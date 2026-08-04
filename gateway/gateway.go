package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/warmbly/warmbly-go/internal/wsconn"
)

// DefaultURL is the production gateway endpoint.
const DefaultURL = "wss://realtime.warmbly.com/socket/websocket"

// protocolVersion is the channel wire format this client speaks. Frames are
// JSON arrays: [join_ref, ref, topic, event, payload].
const protocolVersion = "2.0.0"

// heartbeatTopic is the reserved topic client heartbeats are sent on.
const heartbeatTopic = "phoenix"

// Channel control events.
const (
	evJoin      = "phx_join"
	evReply     = "phx_reply"
	evError     = "phx_error"
	evClose     = "phx_close"
	evHeartbeat = "heartbeat"
)

// ErrUnauthorized is returned by [Client.Open] when the credential is rejected.
// The client does not retry it: a bad key will not become good.
var ErrUnauthorized = errors.New("gateway: credential rejected")

// ErrForbidden is returned by [Client.Open] when the credential is valid but
// may not join the workspace, which is also not retried.
var ErrForbidden = errors.New("gateway: not permitted to join this workspace")

// Client is a realtime gateway client. It holds one connection open,
// heartbeats, dispatches events to registered handlers, and reconnects and
// replays missed events after a drop.
//
// Register handlers with [Client.Handle], [Client.HandleAny] or [On], then call
// [Client.Open]. A Client must not be reused after [Client.Close].
type Client struct {
	token   string
	orgID   string
	url     string
	intents []string

	backoffMin time.Duration
	backoffMax time.Duration
	maxMessage int64
	logf       func(format string, args ...any)

	mu          sync.RWMutex
	handlers    map[EventName][]HandlerFunc
	anyHandlers []HandlerFunc
	conn        *wsconn.Conn
	lastSeq     int
	ready       Ready
	cancel      context.CancelFunc
	running     bool

	ref     atomic.Int64
	joinRef atomic.Int64

	events chan *Event
	wg     sync.WaitGroup

	readyOnce sync.Once
	readyCh   chan struct{}
	readyErr  error

	finishOnce sync.Once
	done       chan struct{}
	termErr    error
}

// Option configures a [Client].
type Option func(*Client)

// WithURL overrides the gateway endpoint, for a self-hosted instance or a
// staging environment. It should be the full websocket path, for example
// "wss://realtime.example.com/socket/websocket".
func WithURL(rawURL string) Option {
	return func(c *Client) {
		if rawURL != "" {
			c.url = rawURL
		}
	}
}

// WithIntents narrows the stream to the given event families. Declaring none
// delivers everything the credential may see. See the Intent* constants.
func WithIntents(intents ...string) Option {
	return func(c *Client) {
		for _, in := range intents {
			if in = strings.TrimSpace(in); in != "" {
				c.intents = append(c.intents, in)
			}
		}
	}
}

// WithLogger sets a logging function for non-fatal diagnostics such as
// reconnects and dropped events. The default discards them.
func WithLogger(logf func(format string, args ...any)) Option {
	return func(c *Client) {
		if logf != nil {
			c.logf = logf
		}
	}
}

// WithReconnectBackoff sets the bounds on the delay between reconnection
// attempts. The delay grows exponentially with jitter between them.
func WithReconnectBackoff(minDelay, maxDelay time.Duration) Option {
	return func(c *Client) {
		if minDelay > 0 {
			c.backoffMin = minDelay
		}
		if maxDelay > 0 {
			c.backoffMax = maxDelay
		}
	}
}

// WithEventBuffer sets the size of the internal dispatch queue, 64 by default.
// When the queue fills, further events are dropped and logged rather than
// stalling the connection, so raise it if your handlers are slow.
func WithEventBuffer(n int) Option {
	return func(c *Client) {
		if n > 0 {
			c.events = make(chan *Event, n)
		}
	}
}

// WithResumeFrom starts the session from a known sequence number instead of
// live, replaying anything after it that is still in the server's buffer. Use
// it to pick up where a previous process left off.
func WithResumeFrom(seq int) Option {
	return func(c *Client) {
		if seq > 0 {
			c.lastSeq = seq
		}
	}
}

// New creates a gateway client for one workspace, authenticated with token: an
// API key holding the realtime-subscribe scope, or a session access token. It
// does not connect; call [Client.Open].
func New(token, orgID string, opts ...Option) *Client {
	c := &Client{
		token:      token,
		orgID:      orgID,
		url:        DefaultURL,
		backoffMin: 1 * time.Second,
		backoffMax: 60 * time.Second,
		maxMessage: 8 << 20,
		logf:       func(string, ...any) {},
		handlers:   make(map[EventName][]HandlerFunc),
		events:     make(chan *Event, 64),
		readyCh:    make(chan struct{}),
		done:       make(chan struct{}),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Open connects and runs the session until ctx is canceled or [Client.Close] is
// called. It blocks until the workspace channel is joined, or until a
// credential error ends the attempt, and returns that error. Reconnection and
// replay continue in the background afterwards.
func (c *Client) Open(ctx context.Context) error {
	if c.token == "" {
		return errors.New("gateway: token is required")
	}
	if c.orgID == "" {
		return errors.New("gateway: organization id is required")
	}

	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return errors.New("gateway: already open")
	}
	c.running = true
	sctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.mu.Unlock()

	c.wg.Add(2)
	go c.dispatchLoop(sctx)
	go c.manage(sctx)

	select {
	case <-c.readyCh:
		if c.readyErr != nil {
			cancel()
		}
		return c.readyErr
	case <-sctx.Done():
		c.signalReady(sctx.Err())
		return sctx.Err()
	}
}

// Close shuts the session down and waits briefly for the background goroutines
// to stop. It is safe to call more than once.
func (c *Client) Close() error {
	c.mu.Lock()
	cancel := c.cancel
	conn := c.conn
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if conn != nil {
		_ = conn.Close(1000, "client closing")
	}

	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
	return nil
}

// LastSeq returns the highest sequence number seen so far. Persist it to resume
// a later process with [WithResumeFrom].
func (c *Client) LastSeq() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastSeq
}

// Ready returns the join details of the current session. It is zero until the
// channel has been joined.
func (c *Client) Ready() Ready {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready
}

// Done returns a channel closed when the session has permanently stopped:
// because [Client.Close] was called or the context was canceled, or because the
// credential was rejected and no further attempt will be made. [Client.Err]
// then reports the terminal error.
func (c *Client) Done() <-chan struct{} { return c.done }

// Err returns the terminal error once [Client.Done] is closed: nil for a clean
// shutdown, or the error that stopped the session. It is nil before then.
func (c *Client) Err() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.termErr
}

func (c *Client) signalReady(err error) {
	c.readyOnce.Do(func() {
		c.readyErr = err
		close(c.readyCh)
	})
}

func (c *Client) finish(err error) {
	c.finishOnce.Do(func() {
		c.mu.Lock()
		c.termErr = err
		c.mu.Unlock()
		close(c.done)
	})
}

// manage runs the connect and reconnect loop for the session's lifetime.
func (c *Client) manage(ctx context.Context) {
	defer c.wg.Done()
	var termErr error
	defer func() { c.finish(termErr) }()

	var attempt int
	for {
		if ctx.Err() != nil {
			return
		}
		joined, err := c.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if joined {
			attempt = 0
		}
		if err != nil {
			// A rejected credential will not become valid on retry.
			if errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrForbidden) {
				c.logf("gateway: %v; not reconnecting", err)
				termErr = err
				c.signalReady(err)
				c.emitLifecycle(EventDisconnected, Disconnected{Err: err.Error()})
				return
			}
			c.logf("gateway: connection ended: %v", err)
			c.emitLifecycle(EventDisconnected, Disconnected{Err: err.Error(), Reconnecting: true})
		}

		delay := c.backoff(attempt)
		attempt++
		c.logf("gateway: reconnecting in %s", delay)
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

// runOnce dials, joins the workspace channel and reads until the connection
// ends. It reports whether the channel was successfully joined.
func (c *Client) runOnce(ctx context.Context) (joined bool, err error) {
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	conn, resp, derr := wsconn.Dial(connCtx, c.socketURL(), nil)
	if derr != nil {
		// The socket refuses the upgrade with 403 when the token is bad, so a
		// dial failure can be terminal rather than transient.
		if resp != nil {
			_ = resp.Body.Close()
			switch resp.StatusCode {
			case 401:
				return false, ErrUnauthorized
			case 403:
				return false, ErrForbidden
			}
		}
		return false, fmt.Errorf("dial: %w", derr)
	}
	if resp != nil {
		_ = resp.Body.Close()
	}
	conn.SetMaxMessage(c.maxMessage)
	c.setConn(conn)
	defer func() {
		_ = conn.Close(1000, "")
		c.setConn(nil)
	}()

	joinRef := strconv.FormatInt(c.joinRef.Add(1), 10)
	if err := c.sendJoin(connCtx, conn, joinRef); err != nil {
		return false, fmt.Errorf("join: %w", err)
	}

	// Heartbeats start only once the join is acknowledged, since the reply
	// carries the cadence the server expects.
	var hbDone chan struct{}
	defer func() {
		cancel()
		if hbDone != nil {
			<-hbDone
		}
	}()

	for {
		_, data, rerr := conn.ReadMessage(connCtx)
		if rerr != nil {
			return joined, rerr
		}

		msg, perr := decodeFrame(data)
		if perr != nil {
			c.logf("gateway: skipping malformed frame: %v", perr)
			continue
		}

		// A reply to our join either starts the session or ends the attempt.
		if msg.Event == evReply && msg.JoinRef == joinRef && !joined {
			ready, jerr := c.handleJoinReply(msg)
			if jerr != nil {
				return false, jerr
			}
			joined = true
			c.mu.Lock()
			c.ready = ready
			c.mu.Unlock()
			c.signalReady(nil)
			c.emitLifecycle(EventReady, ready)

			hbDone = make(chan struct{})
			go c.heartbeatLoop(connCtx, conn, heartbeatInterval(ready), hbDone)
			continue
		}

		if err := c.handleFrame(msg); err != nil {
			return joined, err
		}
	}
}

// socketURL builds the dial URL, carrying the credential and wire version.
func (c *Client) socketURL() string {
	u, err := url.Parse(c.url)
	if err != nil {
		return c.url
	}
	q := u.Query()
	q.Set("token", c.token)
	q.Set("vsn", protocolVersion)
	u.RawQuery = q.Encode()
	return u.String()
}

// sendJoin joins the workspace channel, declaring intents and, when resuming,
// the last sequence seen.
func (c *Client) sendJoin(ctx context.Context, conn *wsconn.Conn, joinRef string) error {
	params := map[string]any{}
	if len(c.intents) > 0 {
		params["intents"] = c.intents
	}
	c.mu.RLock()
	seq := c.lastSeq
	c.mu.RUnlock()
	if seq > 0 {
		params["resume"] = map[string]any{"last_seq": seq}
	}
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return c.write(ctx, conn, frame{
		JoinRef: joinRef,
		Ref:     c.nextRef(),
		Topic:   c.topic(),
		Event:   evJoin,
		Payload: body,
	})
}

func (c *Client) handleJoinReply(msg *frame) (Ready, error) {
	var reply struct {
		Status   string          `json:"status"`
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(msg.Payload, &reply); err != nil {
		return Ready{}, fmt.Errorf("decode join reply: %w", err)
	}
	if reply.Status != "ok" {
		return Ready{}, joinError(reply.Response)
	}
	var ready Ready
	if err := json.Unmarshal(reply.Response, &ready); err != nil {
		return Ready{}, fmt.Errorf("decode join response: %w", err)
	}
	return ready, nil
}

// joinError maps a rejected join to a typed error, so the caller can tell a
// permission problem from a transient one.
func joinError(payload json.RawMessage) error {
	var e struct {
		Code   int    `json:"code"`
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(payload, &e)
	reason := e.Reason
	if reason == "" {
		reason = "join rejected"
	}
	switch e.Code {
	case 4001, 4004:
		return fmt.Errorf("%w: %s", ErrUnauthorized, reason)
	case 4003, 4005:
		return fmt.Errorf("%w: %s", ErrForbidden, reason)
	}
	return fmt.Errorf("gateway: %s", reason)
}

// handleFrame processes a non-join frame, returning an error to end the
// connection and trigger a reconnect.
func (c *Client) handleFrame(msg *frame) error {
	switch msg.Event {
	case evReply:
		// Heartbeat and other acknowledgements need no action.
		return nil
	case evError:
		return errors.New("gateway: channel error")
	case evClose:
		return errors.New("gateway: channel closed by server")
	}

	// Everything else is a workspace event.
	seq := extractSeq(msg.Payload)
	if seq > 0 {
		c.mu.Lock()
		if seq > c.lastSeq {
			c.lastSeq = seq
		}
		c.mu.Unlock()
	}
	c.emit(&Event{
		Type: msg.Event,
		Seq:  seq,
		Raw:  append(json.RawMessage(nil), msg.Payload...),
	})
	return nil
}

// emit queues an event for the dispatch goroutine. It never blocks the read
// loop: a stalled send would stop us reading heartbeat replies and trip a false
// timeout, so a full queue drops the event instead.
func (c *Client) emit(ev *Event) {
	select {
	case c.events <- ev:
	default:
		c.logf("gateway: event buffer full; dropping %s", ev.Type)
	}
}

// emitLifecycle queues a client-side lifecycle event.
func (c *Client) emitLifecycle(name EventName, payload any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	c.emit(&Event{Type: name, Raw: raw})
}

// heartbeatLoop keeps the connection alive at the cadence the server asked for.
// The server closes a connection that goes silent past its own timeout, so a
// failed heartbeat closes the connection immediately to reconnect sooner.
func (c *Client) heartbeatLoop(ctx context.Context, conn *wsconn.Conn, interval time.Duration, done chan<- struct{}) {
	defer close(done)

	// Jitter the first beat so a fleet of clients does not synchronize.
	timer := time.NewTimer(time.Duration(float64(interval) * rand.Float64()))
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		err := c.write(ctx, conn, frame{
			Ref:   c.nextRef(),
			Topic: heartbeatTopic,
			Event: evHeartbeat,
		})
		if err != nil {
			if ctx.Err() == nil {
				c.logf("gateway: heartbeat failed: %v", err)
				_ = conn.Close(1001, "heartbeat failed")
			}
			return
		}
		timer.Reset(interval)
	}
}

func (c *Client) dispatchLoop(ctx context.Context) {
	defer c.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-c.events:
			c.fire(ctx, ev)
		}
	}
}

func (c *Client) fire(ctx context.Context, ev *Event) {
	c.mu.RLock()
	named := append([]HandlerFunc(nil), c.handlers[ev.Type]...)
	wildcards := append([]HandlerFunc(nil), c.anyHandlers...)
	c.mu.RUnlock()

	for _, h := range wildcards {
		c.safeCall(ctx, h, ev)
	}
	for _, h := range named {
		c.safeCall(ctx, h, ev)
	}
}

// safeCall isolates a panicking handler so it cannot take the session down.
func (c *Client) safeCall(ctx context.Context, h HandlerFunc, ev *Event) {
	defer func() {
		if r := recover(); r != nil {
			c.logf("gateway: handler for %s panicked: %v", ev.Type, r)
		}
	}()
	h(ctx, ev)
}

func (c *Client) write(ctx context.Context, conn *wsconn.Conn, f frame) error {
	b, err := f.encode()
	if err != nil {
		return err
	}
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return conn.WriteMessage(wctx, wsconn.MessageText, b)
}

func (c *Client) topic() string { return "org:" + c.orgID }

func (c *Client) nextRef() string { return strconv.FormatInt(c.ref.Add(1), 10) }

func (c *Client) backoff(attempt int) time.Duration {
	d := c.backoffMin << attempt
	if d <= 0 || d > c.backoffMax {
		d = c.backoffMax
	}
	half := d / 2
	if half <= 0 {
		return d
	}
	return half + time.Duration(rand.Int64N(int64(half)+1))
}

func (c *Client) setConn(conn *wsconn.Conn) {
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
}

// heartbeatInterval picks the cadence from the join reply, falling back to a
// safe default when the server did not advertise one.
func heartbeatInterval(r Ready) time.Duration {
	if r.HeartbeatIntervalMS > 0 {
		return time.Duration(r.HeartbeatIntervalMS) * time.Millisecond
	}
	return 25 * time.Second
}

// extractSeq pulls the sequence number out of an event payload without
// decoding the whole thing.
func extractSeq(payload json.RawMessage) int {
	var probe struct {
		Seq int `json:"seq"`
	}
	if err := json.Unmarshal(payload, &probe); err != nil {
		return 0
	}
	return probe.Seq
}

// frame is one channel message. On the wire it is a five-element JSON array:
// [join_ref, ref, topic, event, payload], where the first two may be null.
type frame struct {
	JoinRef string
	Ref     string
	Topic   string
	Event   string
	Payload json.RawMessage
}

func (f frame) encode() ([]byte, error) {
	// join_ref and ref are null when the frame does not correlate to a reply.
	var joinRef, ref any
	if f.JoinRef != "" {
		joinRef = f.JoinRef
	}
	if f.Ref != "" {
		ref = f.Ref
	}
	payload := f.Payload
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}
	return json.Marshal([]any{joinRef, ref, f.Topic, f.Event, payload})
}

func decodeFrame(data []byte) (*frame, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if len(raw) != 5 {
		return nil, fmt.Errorf("expected 5 frame elements, got %d", len(raw))
	}
	f := &frame{Payload: raw[4]}
	// The first two elements are null on a server push.
	_ = json.Unmarshal(raw[0], &f.JoinRef)
	_ = json.Unmarshal(raw[1], &f.Ref)
	if err := json.Unmarshal(raw[2], &f.Topic); err != nil {
		return nil, fmt.Errorf("decode topic: %w", err)
	}
	if err := json.Unmarshal(raw[3], &f.Event); err != nil {
		return nil, fmt.Errorf("decode event: %w", err)
	}
	return f, nil
}
