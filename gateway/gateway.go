package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/warmbly/warmbly-go/internal/wsconn"
)

// DefaultURL is the production gateway endpoint.
const DefaultURL = "wss://gateway.warmbly.com/?v=1&encoding=json"

const (
	libName    = "warmbly-go"
	libVersion = "0.1.0"
)

// Close codes that indicate the client must not keep retrying.
var fatalCloseCodes = map[int]bool{
	4004: true, // authentication failed
	4013: true, // invalid intents
	4014: true, // disallowed intents
}

// Client is a real-time gateway client. It maintains a persistent connection
// that authenticates, heartbeats, dispatches typed events to registered
// handlers, and transparently resumes or reconnects on failure.
//
// Register handlers with [Client.Handle], [Client.HandleAny] or [On], then call
// [Client.Open]. A Client must not be reused after [Client.Close].
type Client struct {
	token      string
	url        string
	intents    Intent
	backoffMin time.Duration
	backoffMax time.Duration
	maxMessage int64
	logf       func(format string, args ...any)

	mu          sync.RWMutex
	handlers    map[EventName][]HandlerFunc
	anyHandlers []HandlerFunc
	conn        *wsconn.Conn
	sessionID   string
	seq         int
	hbInterval  time.Duration
	cancel      context.CancelFunc
	running     bool

	ackPending atomic.Bool

	events chan *Event
	wg     sync.WaitGroup

	readyOnce sync.Once
	readyCh   chan struct{}
	readyErr  error
}

// Option configures a [Client].
type Option func(*Client)

// WithURL overrides the gateway endpoint (default [DefaultURL]). Useful for a
// self-hosted instance or a staging environment.
func WithURL(url string) Option {
	return func(c *Client) {
		if url != "" {
			c.url = url
		}
	}
}

// WithIntents sets the event categories the session subscribes to (default
// [IntentsDefault]).
func WithIntents(intents Intent) Option {
	return func(c *Client) { c.intents = intents }
}

// WithLogger sets a logging function for non-fatal diagnostics (reconnects,
// dropped events). The default discards logs.
func WithLogger(logf func(format string, args ...any)) Option {
	return func(c *Client) {
		if logf != nil {
			c.logf = logf
		}
	}
}

// WithReconnectBackoff sets the minimum and maximum delay between reconnection
// attempts. Delays grow exponentially with jitter between these bounds.
func WithReconnectBackoff(min, max time.Duration) Option {
	return func(c *Client) {
		if min > 0 {
			c.backoffMin = min
		}
		if max > 0 {
			c.backoffMax = max
		}
	}
}

// WithEventBuffer sets the size of the internal dispatch queue (default 64).
func WithEventBuffer(n int) Option {
	return func(c *Client) {
		if n > 0 {
			c.events = make(chan *Event, n)
		}
	}
}

// New creates a gateway client authenticated with token (a Warmbly API key or
// OAuth access token). It does not connect; call [Client.Open] to start.
func New(token string, opts ...Option) *Client {
	c := &Client{
		token:      token,
		url:        DefaultURL,
		intents:    IntentsDefault,
		backoffMin: 1 * time.Second,
		backoffMax: 60 * time.Second,
		maxMessage: 8 << 20,
		logf:       func(string, ...any) {},
		handlers:   make(map[EventName][]HandlerFunc),
		events:     make(chan *Event, 64),
		readyCh:    make(chan struct{}),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Open connects and runs the session until ctx is cancelled or [Client.Close]
// is called. It blocks until the first session is established (the READY event)
// or a fatal error occurs, returning that error. Reconnection and resumption
// happen automatically in the background after Open returns.
func (c *Client) Open(ctx context.Context) error {
	if c.token == "" {
		return errors.New("gateway: token is required")
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
		// Cancelled before the first READY.
		c.signalReady(sctx.Err())
		return sctx.Err()
	}
}

// Close shuts the session down and waits briefly for background goroutines to
// stop. It is safe to call more than once.
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

// SessionID returns the current session identifier, or "" if no session is
// established.
func (c *Client) SessionID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionID
}

func (c *Client) signalReady(err error) {
	c.readyOnce.Do(func() {
		c.readyErr = err
		close(c.readyCh)
	})
}

// manage runs the connect/reconnect loop for the lifetime of the session.
func (c *Client) manage(ctx context.Context) {
	defer c.wg.Done()
	var attempt int
	for {
		if ctx.Err() != nil {
			return
		}
		resumable, hadReady, err := c.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if hadReady {
			attempt = 0
		}
		if err != nil {
			c.logf("gateway: connection ended: %v (resumable=%v)", err, resumable)
			if isFatalClose(err) {
				c.signalReady(err)
				return
			}
		}
		if !resumable {
			c.mu.Lock()
			c.sessionID, c.seq = "", 0
			c.mu.Unlock()
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

// runOnce establishes a single connection and reads until it ends. It reports
// whether the disconnect is resumable and whether the session reached READY.
func (c *Client) runOnce(ctx context.Context) (resumable bool, hadReady bool, err error) {
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	conn, _, derr := wsconn.Dial(connCtx, c.url, nil)
	if derr != nil {
		return true, false, fmt.Errorf("dial: %w", derr)
	}
	conn.SetMaxMessage(c.maxMessage)
	c.setConn(conn)
	defer func() {
		_ = conn.Close(1000, "")
		c.setConn(nil)
	}()

	// The first frame must be Hello.
	hello, herr := c.readHello(connCtx, conn)
	if herr != nil {
		return true, false, herr
	}
	interval := time.Duration(hello.HeartbeatInterval) * time.Millisecond
	if interval <= 0 {
		interval = 30 * time.Second
	}
	c.setHeartbeatInterval(interval)
	c.ackPending.Store(false)

	// Resume an existing session if we have one, otherwise identify afresh.
	c.mu.RLock()
	sid, seq := c.sessionID, c.seq
	c.mu.RUnlock()
	if sid != "" {
		err = c.send(connCtx, OpResume, resumeData{Token: c.token, SessionID: sid, Seq: seq})
	} else {
		err = c.send(connCtx, OpIdentify, identifyData{Token: c.token, Intents: c.intents, Properties: c.props()})
	}
	if err != nil {
		return true, false, fmt.Errorf("handshake: %w", err)
	}

	// Heartbeat on its own goroutine, tied to this connection.
	hbDone := make(chan struct{})
	go c.heartbeatLoop(connCtx, conn, interval, hbDone)
	defer func() {
		cancel()
		<-hbDone
	}()

	for {
		_, data, rerr := conn.ReadMessage(connCtx)
		if rerr != nil {
			return isResumable(rerr), hadReady, rerr
		}
		ready, perr := c.process(ctx, data)
		if ready {
			hadReady = true
		}
		if perr != nil {
			return isResumable(perr), hadReady, perr
		}
	}
}

func (c *Client) readHello(ctx context.Context, conn *wsconn.Conn) (*helloData, error) {
	_, data, err := conn.ReadMessage(ctx)
	if err != nil {
		return nil, fmt.Errorf("read hello: %w", err)
	}
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("decode hello: %w", err)
	}
	if env.Op != OpHello {
		return nil, fmt.Errorf("expected Hello, got %s", env.Op)
	}
	hello := new(helloData)
	if len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, hello); err != nil {
			return nil, fmt.Errorf("decode hello payload: %w", err)
		}
	}
	return hello, nil
}

// process handles a single received frame. It reports whether the frame put the
// session into the ready state and returns a non-nil error to end the
// connection (triggering reconnect).
func (c *Client) process(ctx context.Context, data []byte) (ready bool, err error) {
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		c.logf("gateway: skipping malformed frame: %v", err)
		return false, nil
	}
	if env.Seq != nil {
		c.mu.Lock()
		c.seq = *env.Seq
		c.mu.Unlock()
	}

	switch env.Op {
	case OpHeartbeat:
		// Server asked for an immediate heartbeat.
		c.mu.RLock()
		seq := c.seq
		c.mu.RUnlock()
		_ = c.send(ctx, OpHeartbeat, seq)
		return false, nil
	case OpHeartbeatACK:
		c.ackPending.Store(false)
		return false, nil
	case OpReconnect:
		return false, &disconnectError{resumable: true, reason: "server requested reconnect"}
	case OpInvalidSession:
		var resumable bool
		_ = json.Unmarshal(env.Data, &resumable)
		return false, &disconnectError{resumable: resumable, reason: "invalid session"}
	case OpDispatch:
		return c.dispatch(ctx, &env), nil
	default:
		c.logf("gateway: ignoring unexpected op %s", env.Op)
		return false, nil
	}
}

func (c *Client) dispatch(ctx context.Context, env *envelope) (ready bool) {
	name := EventName(env.Type)
	seq := 0
	if env.Seq != nil {
		seq = *env.Seq
	}
	if name == EventReady {
		var r Ready
		if err := json.Unmarshal(env.Data, &r); err == nil && r.SessionID != "" {
			c.mu.Lock()
			c.sessionID = r.SessionID
			c.mu.Unlock()
		}
		c.signalReady(nil)
		ready = true
	}
	if name == EventResumed {
		c.signalReady(nil)
		ready = true
	}

	ev := &Event{Type: name, Seq: seq, Raw: append(json.RawMessage(nil), env.Data...)}
	select {
	case c.events <- ev:
	case <-ctx.Done():
	}
	return ready
}

// heartbeatLoop sends periodic heartbeats and detects a zombied connection: if
// the previous heartbeat went unacknowledged, it closes the connection to force
// a reconnect.
func (c *Client) heartbeatLoop(ctx context.Context, conn *wsconn.Conn, interval time.Duration, done chan<- struct{}) {
	defer close(done)

	// Jitter the first beat to avoid synchronised reconnect storms.
	first := time.Duration(float64(interval) * rand.Float64())
	timer := time.NewTimer(first)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		if c.ackPending.Load() {
			c.logf("gateway: heartbeat not acknowledged; forcing reconnect")
			_ = conn.Close(1001, "heartbeat timeout")
			return
		}

		c.mu.RLock()
		seq := c.seq
		c.mu.RUnlock()
		c.ackPending.Store(true)
		if err := c.send(ctx, OpHeartbeat, seq); err != nil {
			if ctx.Err() == nil {
				c.logf("gateway: heartbeat send failed: %v", err)
				_ = conn.Close(1001, "heartbeat send failed")
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
	any := append([]HandlerFunc(nil), c.anyHandlers...)
	c.mu.RUnlock()

	for _, h := range any {
		c.safeCall(ctx, h, ev)
	}
	for _, h := range named {
		c.safeCall(ctx, h, ev)
	}
}

func (c *Client) safeCall(ctx context.Context, h HandlerFunc, ev *Event) {
	defer func() {
		if r := recover(); r != nil {
			c.logf("gateway: handler for %s panicked: %v", ev.Type, r)
		}
	}()
	h(ctx, ev)
}

// send marshals and writes a frame. Writes are serialised by the underlying
// connection, so send is safe to call from multiple goroutines.
func (c *Client) send(ctx context.Context, op Opcode, data any) error {
	var raw json.RawMessage
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return err
		}
		raw = b
	}
	b, err := json.Marshal(envelope{Op: op, Data: raw})
	if err != nil {
		return err
	}
	conn := c.getConn()
	if conn == nil {
		return errors.New("gateway: not connected")
	}
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return conn.WriteMessage(wctx, wsconn.MessageText, b)
}

func (c *Client) props() properties {
	return properties{OS: runtime.GOOS, Lib: libName, Version: libVersion}
}

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

func (c *Client) getConn() *wsconn.Conn {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn
}

func (c *Client) setHeartbeatInterval(d time.Duration) {
	c.mu.Lock()
	c.hbInterval = d
	c.mu.Unlock()
}

// disconnectError signals a protocol-level disconnect from a received control
// op, carrying whether the session may be resumed.
type disconnectError struct {
	resumable bool
	reason    string
}

func (e *disconnectError) Error() string { return "gateway: " + e.reason }

// isResumable reports whether err represents a disconnect after which the
// session can be resumed (rather than re-identified).
func isResumable(err error) bool {
	var de *disconnectError
	if errors.As(err, &de) {
		return de.resumable
	}
	var ce *wsconn.CloseError
	if errors.As(err, &ce) {
		// Normal/away closures and most application codes are resumable;
		// the explicitly fatal codes are not.
		return !fatalCloseCodes[ce.Code]
	}
	// Network errors are resumable.
	return true
}

// isFatalClose reports whether err is a close that must stop the client (for
// example an authentication failure), so it should not keep retrying.
func isFatalClose(err error) bool {
	var ce *wsconn.CloseError
	if errors.As(err, &ce) {
		return fatalCloseCodes[ce.Code]
	}
	return false
}
