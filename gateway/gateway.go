package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
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

// handshakeMinInterval is the shortest gap this client leaves between socket
// handshakes. The server budgets handshakes at 30 a minute per credential, so
// dialing faster than one every two seconds eventually earns a refusal that
// looks exactly like a permission error.
const handshakeMinInterval = 2 * time.Second

// handshakeCooldown is how long the client waits after a refused handshake on a
// credential that had already connected. The handshake budget is a fixed
// one-minute window, so a full minute is the only wait guaranteed to clear it.
const handshakeCooldown = time.Minute

// stableSession is how long a connection must survive before the reconnect
// backoff is considered recovered. Without it a socket that joins and
// immediately drops resets the backoff on every attempt and reconnects in a
// tight loop, which is precisely what the handshake budget refuses.
const stableSession = 30 * time.Second

// maxJoinRetryDelay caps how long the client will wait out a rate-limited join
// before trying again. The server's hint is the remainder of the current minute,
// so anything beyond this is a server bug rather than a wait worth honoring.
const maxJoinRetryDelay = 2 * time.Minute

// Codes carried in [JoinError.Code]. The gateway uses the same set for a
// refused socket handshake and for a refused channel join, though only the join
// path delivers them in a machine-readable body today.
const (
	// JoinCodeUnauthenticated means no credential was presented at all.
	JoinCodeUnauthenticated = 4003
	// JoinCodeAuthFailed means the credential was presented and rejected:
	// expired token, unknown or revoked key, bad signature. Re-mint it.
	JoinCodeAuthFailed = 4004
	// JoinCodeInvalidTopic means the topic itself is malformed, typically an
	// id that is not a UUID. Retrying cannot help; fix the topic.
	JoinCodeInvalidTopic = 4005
	// JoinCodeRateLimited means the join budget is spent.
	// [JoinError.RetryAfter] says how long until it resets. The socket stays
	// open, so the fix is to wait and re-send the join, not to reconnect.
	JoinCodeRateLimited = 4007
	// JoinCodeConnectionLimit means the credential already holds as many
	// concurrent sockets as its plan allows. Close one, or wait for one to
	// drop.
	JoinCodeConnectionLimit = 4009
	// JoinCodePermissionDenied covers permission denied, not a member of the
	// workspace, an IP outside the allowlist, and "no such record". They share
	// one code deliberately: distinguishing them would tell an unauthorized
	// caller whether a record exists. Do not retry the topic.
	JoinCodePermissionDenied = 4010
)

// Stable reason slugs carried in [JoinError.Reason]. The slug names the
// specific refusal, where [JoinError.Code] is only the coarse class; branch on
// the code unless you need to tell, say, a missing campaign from a campaign you
// may not see. Any other slug is possible: the server adds them over time.
const (
	// JoinReasonRateLimited accompanies [JoinCodeRateLimited].
	JoinReasonRateLimited = "rate_limited"
	// JoinReasonNotAMember means the credential's owner is not a member of the
	// workspace in the topic.
	JoinReasonNotAMember = "not_a_member"
	// JoinReasonForbidden means the member exists but lacks the permission the
	// topic requires.
	JoinReasonForbidden = "forbidden"
	// JoinReasonUnauthorized is returned by the user channel when a socket
	// tries to join a user topic that is not its own.
	JoinReasonUnauthorized = "unauthorized"
	// JoinReasonCampaignNotFound means no campaign with that id is visible to
	// this credential. It carries [JoinCodePermissionDenied], not a 404-shaped
	// code, so it cannot be used to probe for ids.
	JoinReasonCampaignNotFound = "campaign_not_found"
	// JoinReasonAccountNotFound is the mailbox equivalent.
	JoinReasonAccountNotFound = "account_not_found"
	// JoinReasonInvalidCampaignID, JoinReasonInvalidAccountID and
	// JoinReasonInvalidOperationID accompany [JoinCodeInvalidTopic]: the id in
	// the topic is not a UUID.
	JoinReasonInvalidCampaignID  = "invalid_campaign_id"
	JoinReasonInvalidAccountID   = "invalid_account_id"
	JoinReasonInvalidOperationID = "invalid_operation_id"
)

// ErrUnauthorized is returned by [Client.Open] when the credential is rejected.
// The client does not retry it: a bad key will not become good.
var ErrUnauthorized = errors.New("gateway: credential rejected")

// ErrForbidden is returned by [Client.Open] when the credential is valid but
// may not join the workspace.
//
// The client stops on it during the first connection, when it almost always
// means a permission problem. Once a session has joined successfully, a later
// refused handshake is far more likely to be the handshake budget or the
// concurrent-connection cap — which look identical from the client, because the
// gateway refuses a handshake with a plain HTTP 403 and no machine-readable
// body — so the client backs off and retries instead of giving up.
var ErrForbidden = errors.New("gateway: not permitted to join this workspace")

// JoinError is a refused join, or a refused socket handshake.
//
// It implements Unwrap, so the coarse classes still match the package
// sentinels:
//
//	if errors.Is(err, gateway.ErrUnauthorized) { /* re-mint the credential */ }
//
//	var je *gateway.JoinError
//	if errors.As(err, &je) && je.Code == gateway.JoinCodeRateLimited {
//		time.Sleep(je.RetryAfter)
//	}
//
// The client already honors [JoinCodeRateLimited] itself, so a JoinError with
// that code only reaches you if the whole session was torn down for another
// reason first.
type JoinError struct {
	// Topic is the channel that was refused. It is empty when the refusal was
	// the socket handshake rather than a join.
	Topic string
	// Code is one of the JoinCode* constants: the coarse class to branch on.
	Code int
	// Reason is the server's stable slug for the specific refusal, for example
	// "not_a_member". See the JoinReason* constants.
	Reason string
	// Category names the exhausted budget on a rate-limited refusal, matching
	// the RateLimitCategory* constants. It is empty otherwise.
	Category string
	// RetryAfter is how long until the limiter resets, on a rate-limited
	// refusal. It is the remainder of the current minute's fixed window, so
	// retrying earlier only spends another refusal. Zero otherwise.
	RetryAfter time.Duration
}

// Error implements the error interface.
func (e *JoinError) Error() string {
	var b strings.Builder
	b.WriteString("gateway: join refused")
	if e.Topic != "" {
		b.WriteString(" for " + e.Topic)
	}
	b.WriteString(": " + e.Reason)
	if e.Code != 0 {
		b.WriteString(" (code " + strconv.Itoa(e.Code))
		if e.RetryAfter > 0 {
			b.WriteString(", retry after " + e.RetryAfter.String())
		}
		b.WriteString(")")
	}
	return b.String()
}

// Unwrap maps the refusal onto the package sentinels so errors.Is keeps
// working: [ErrUnauthorized] for a missing or rejected credential,
// [ErrForbidden] for a permission refusal. The codes that are neither — a
// malformed topic, a rate limit, the connection cap — unwrap to nil, because
// calling them either would send a caller down the wrong branch.
func (e *JoinError) Unwrap() error {
	switch e.Code {
	case JoinCodeUnauthenticated, JoinCodeAuthFailed:
		return ErrUnauthorized
	case JoinCodePermissionDenied:
		return ErrForbidden
	}
	return nil
}

// Permanent reports whether re-sending the same join is pointless: the
// credential, the permission or the topic itself must change first. A rate
// limit is not permanent, and neither is the connection cap (a socket will free
// up) or a code this client does not recognize.
func (e *JoinError) Permanent() bool {
	switch e.Code {
	case JoinCodeUnauthenticated, JoinCodeAuthFailed, JoinCodeInvalidTopic, JoinCodePermissionDenied:
		return true
	}
	return false
}

// OrgTopic is the workspace channel: every org-scoped event, filtered by the
// member's permissions. [Client.Open] joins it automatically.
func OrgTopic(orgID string) string { return "org:" + orgID }

// UserTopic is a member's personal channel. Several event families are only
// ever delivered here and never reach the workspace channel: the task
// lifecycle, meetings, notifications, bulk-operation progress and
// [EventContactsReload]. A socket may only join its own user topic; any other
// is refused with [JoinReasonUnauthorized].
func UserTopic(userID string) string { return "user:" + userID }

// CampaignTopic is one campaign's activity. Joining requires view_campaigns.
func CampaignTopic(campaignID string) string { return "campaign:" + campaignID }

// AccountTopic is one mailbox's sync and warmup events. Joining requires
// manage_emails.
func AccountTopic(accountID string) string { return "account:" + accountID }

// BulkTopic is one bulk operation's progress.
//
// The join only checks that the id is a UUID — there is no record for the
// gateway to check ownership against — but delivery is gated: an event is
// pushed only to the socket of the member who started the operation. Joining
// someone else's operation id therefore yields an open channel that never
// receives anything, not a view of their import.
func BulkTopic(operationID string) string { return "bulk:" + operationID }

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
	topics  []string

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
	everJoined  bool

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

// WithIntents narrows the workspace stream to the given event families.
// Declaring none delivers everything the credential may see. See the Intent*
// constants.
//
// Intents apply to the workspace channel only. The extra topics from
// [WithTopics] are already narrow, and the server ignores intents on them.
func WithIntents(intents ...string) Option {
	return func(c *Client) {
		for _, in := range intents {
			if in = strings.TrimSpace(in); in != "" {
				c.intents = append(c.intents, in)
			}
		}
	}
}

// WithTopics joins extra channels alongside the workspace channel, over the
// same socket. Build them with [UserTopic], [CampaignTopic], [AccountTopic] or
// [BulkTopic].
//
// Use it to reach the families the workspace channel never carries — task
// lifecycle, meetings, notifications and bulk progress all arrive on
// [UserTopic] only. [Event.Topic] tells handlers which channel an event came
// from.
//
// Extra topics are joined after the workspace channel and rejoined on every
// reconnect. A refused one does not stop the session: it raises
// [EventJoinFailed] and is left alone until the next reconnect, so a typo in
// one campaign id cannot cost you the whole stream. Each topic spends one unit
// of the per-minute join budget on every connect, so subscribe to a handful,
// not hundreds.
func WithTopics(topics ...string) Option {
	return func(c *Client) {
		for _, t := range topics {
			if t = strings.TrimSpace(t); t != "" {
				c.topics = append(c.topics, t)
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
// attempts, 1s to 60s by default. The delay grows exponentially with jitter
// between them, and resets only once a connection has stayed up for a while —
// a socket that joins and drops immediately keeps backing off rather than
// hammering the endpoint.
//
// Two separate per-minute budgets sit behind this, both per credential and both
// allowing a burst of 1.5x before refusing:
//
//   - Socket handshakes, 30 a minute. This is what reconnecting spends. The
//     client never dials more than once every two seconds regardless of the
//     bounds set here, so a very small minDelay does not turn into a storm.
//   - Channel joins, 30 a minute, spent once per topic per connect. Reconnects
//     cannot eat the allowance a client then needs to rejoin its topics with,
//     because the two budgets are separate.
//
// A handshake refused for rate limiting is indistinguishable from a permission
// refusal on the wire (both are a plain HTTP 403), so after a session has
// connected once the client waits a full minute — the length of the budget's
// fixed window — before dialing again rather than treating it as fatal.
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
//
// A workspace join refused for rate limiting is waited out and re-sent on the
// same socket, so Open can block for the remainder of the current minute before
// returning. Cancel ctx to give up sooner.
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
// workspace channel has been joined.
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
	var lastDial time.Time
	for {
		if ctx.Err() != nil {
			return
		}

		// Never dial faster than the handshake budget allows, however short the
		// configured backoff is.
		if gap := handshakeMinInterval - time.Since(lastDial); !lastDial.IsZero() && gap > 0 {
			if !sleepCtx(ctx, gap) {
				return
			}
		}
		lastDial = time.Now()

		joined, err := c.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if joined && time.Since(lastDial) >= stableSession {
			attempt = 0
		}

		delay := c.backoff(attempt)
		attempt++

		if err != nil {
			if fatal, cooldown := c.classify(err); fatal {
				c.logf("gateway: %v; not reconnecting", err)
				termErr = err
				c.signalReady(err)
				c.emitLifecycle(EventDisconnected, Disconnected{Err: err.Error()})
				return
			} else if cooldown > delay {
				delay = cooldown
			}
			c.logf("gateway: connection ended: %v", err)
			c.emitLifecycle(EventDisconnected, Disconnected{Err: err.Error(), Reconnecting: true})
		}

		c.logf("gateway: reconnecting in %s", delay)
		if !sleepCtx(ctx, delay) {
			return
		}
	}
}

// classify decides what a failed connection means for the reconnect loop: give
// up, or wait at least cooldown and try again.
func (c *Client) classify(err error) (fatal bool, cooldown time.Duration) {
	// A rejected credential will not become valid on retry.
	if errors.Is(err, ErrUnauthorized) {
		return true, 0
	}

	var je *JoinError
	if errors.As(err, &je) {
		if je.Permanent() {
			return true, 0
		}
		// A refusal that quoted a wait is worth honoring over the backoff: it
		// is the moment the budget resets, and retrying earlier only spends
		// another refusal.
		return false, je.RetryAfter
	}

	if errors.Is(err, ErrForbidden) {
		// A refused handshake carries no machine-readable body, so a 403 on a
		// credential that has already connected is more likely the handshake
		// budget or the connection cap than a permission that vanished
		// mid-session. Wait out the budget's window instead of giving up.
		c.mu.RLock()
		seen := c.everJoined
		c.mu.RUnlock()
		if seen {
			return false, handshakeCooldown
		}
		return true, 0
	}
	return false, 0
}

// runOnce dials, joins the workspace channel and any extra topics, then reads
// until the connection ends. It reports whether the workspace channel was
// successfully joined.
func (c *Client) runOnce(ctx context.Context) (joined bool, err error) {
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	conn, resp, derr := wsconn.Dial(connCtx, c.socketURL(), nil)
	if derr != nil {
		// The socket refuses the upgrade at the HTTP layer when the credential
		// is bad, so a dial failure can be terminal rather than transient.
		if herr := handshakeError(resp); herr != nil {
			return false, herr
		}
		return false, fmt.Errorf("dial: %w", derr)
	}
	if resp != nil {
		_ = resp.Body.Close()
	}
	conn.SetMaxMessage(c.maxMessage)
	c.setConn(conn)

	// Rejoin timers and the heartbeat goroutine share the socket with this read
	// loop; wsconn serializes writes, so they only need to be stopped before it
	// is closed.
	var timers sync.WaitGroup
	var hbDone chan struct{}
	defer func() {
		cancel()
		timers.Wait()
		if hbDone != nil {
			<-hbDone
		}
		_ = conn.Close(1000, "")
		c.setConn(nil)
	}()

	primary := c.topic()
	book := newJoinBook()
	if jerr := c.sendJoin(connCtx, conn, book, primary, true); jerr != nil {
		return false, fmt.Errorf("join: %w", jerr)
	}
	for _, topic := range c.topics {
		if topic == primary {
			continue
		}
		if jerr := c.sendJoin(connCtx, conn, book, topic, false); jerr != nil {
			return false, fmt.Errorf("join %s: %w", topic, jerr)
		}
	}

	pong := make(chan struct{}, 1)

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

		// Heartbeat replies ride the reserved topic and only feed the watchdog.
		if msg.Topic == heartbeatTopic {
			select {
			case pong <- struct{}{}:
			default:
			}
			continue
		}

		// A reply to a join we are waiting on either opens that channel or
		// refuses it.
		if msg.Event == evReply {
			cj := book.pending(msg.JoinRef)
			if cj == nil {
				// An acknowledgement for something else; nothing to do.
				continue
			}
			ready, jerr := c.handleJoinReply(msg, cj.topic)
			if jerr != nil {
				stop, serr := c.handleRefusal(connCtx, conn, book, cj, jerr, &timers)
				if stop {
					return joined, serr
				}
				continue
			}
			book.markJoined(cj)
			if !cj.primary {
				c.logf("gateway: joined %s", cj.topic)
				continue
			}

			joined = true
			c.mu.Lock()
			c.ready = ready
			c.everJoined = true
			// Adopt the workspace's current position only on a fresh start. A
			// resuming client keeps its own, so a replay interrupted halfway
			// still resumes from the last event it actually handled.
			if c.lastSeq == 0 && ready.Seq > 0 {
				c.lastSeq = ready.Seq
			}
			c.mu.Unlock()
			c.signalReady(nil)
			c.emitLifecycle(EventReady, ready)

			hbDone = make(chan struct{})
			go c.heartbeatLoop(connCtx, conn, heartbeatInterval(ready), pong, hbDone)
			continue
		}

		if ferr := c.handleFrame(msg, msg.Topic == primary); ferr != nil {
			return joined, ferr
		}
	}
}

// handleRefusal reacts to a refused join. It reports whether the connection
// should be torn down, and with what error.
func (c *Client) handleRefusal(ctx context.Context, conn *wsconn.Conn, book *joinBook, cj *channelJoin, jerr error, timers *sync.WaitGroup) (stop bool, err error) {
	var je *JoinError
	if errors.As(jerr, &je) && je.Code == JoinCodeRateLimited {
		delay := je.RetryAfter
		if delay <= 0 {
			delay = time.Second
		}
		if delay > maxJoinRetryDelay {
			delay = maxJoinRetryDelay
		}
		// The socket is still good: the join budget is spent, not the
		// handshake one. Wait out the window and re-send on this connection;
		// reconnecting here would spend a handshake and change nothing.
		c.logf("gateway: join of %s rate limited; retrying in %s", cj.topic, delay)
		c.emitLifecycle(EventRateLimited, RateLimited{
			Category:     RateLimitCategoryJoin,
			RetryAfterMS: int(delay / time.Millisecond),
			Topic:        cj.topic,
		})
		timers.Add(1)
		go func() {
			defer timers.Done()
			if !sleepCtx(ctx, delay) {
				return
			}
			if err := c.sendJoin(ctx, conn, book, cj.topic, cj.primary); err != nil {
				c.logf("gateway: rejoin of %s failed: %v", cj.topic, err)
				_ = conn.Close(1001, "rejoin failed")
			}
		}()
		return false, nil
	}

	// The workspace channel is the session; losing it ends the connection so
	// the manage loop can decide whether to try again.
	if cj.primary {
		return true, jerr
	}

	// An extra topic is not worth the session. Report it and leave it alone
	// until the next reconnect.
	c.logf("gateway: %v", jerr)
	failed := JoinFailed{Topic: cj.topic}
	if je != nil {
		failed.Code, failed.Reason = je.Code, je.Reason
	}
	c.emitLifecycle(EventJoinFailed, failed)
	return false, nil
}

// handshakeError classifies a refused websocket upgrade. The gateway answers a
// connect-level refusal with an HTTP status and no machine-readable body — the
// reason never reaches the client — so this can only separate "bad credential"
// from "not permitted", and returns nil for anything else so the caller reports
// the transport error instead.
func handshakeError(resp *http.Response) error {
	if resp == nil {
		return nil
	}
	// wsconn has already closed the body on a failed handshake.
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusForbidden:
		return ErrForbidden
	}
	return nil
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

// sendJoin joins one topic and records the attempt so the reply can be matched
// back to it. Intents and the resume position ride the workspace join only; no
// other channel reads them.
func (c *Client) sendJoin(ctx context.Context, conn *wsconn.Conn, book *joinBook, topic string, primary bool) error {
	params := map[string]any{}
	if primary {
		if len(c.intents) > 0 {
			params["intents"] = c.intents
		}
		c.mu.RLock()
		seq := c.lastSeq
		c.mu.RUnlock()
		if seq > 0 {
			params["resume"] = map[string]any{"last_seq": seq}
		}
	}
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	ref := strconv.FormatInt(c.joinRef.Add(1), 10)
	book.track(topic, primary, ref)
	return c.write(ctx, conn, frame{
		JoinRef: ref,
		Ref:     ref,
		Topic:   topic,
		Event:   evJoin,
		Payload: body,
	})
}

func (c *Client) handleJoinReply(msg *frame, topic string) (Ready, error) {
	var reply struct {
		Status   string          `json:"status"`
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(msg.Payload, &reply); err != nil {
		return Ready{}, fmt.Errorf("decode join reply: %w", err)
	}
	if reply.Status != "ok" {
		return Ready{}, newJoinError(topic, reply.Response)
	}
	var ready Ready
	// Only the workspace channel answers with a HELLO; the others reply with an
	// empty object, which decodes to the zero value.
	if err := json.Unmarshal(reply.Response, &ready); err != nil {
		return Ready{}, fmt.Errorf("decode join response: %w", err)
	}
	return ready, nil
}

// newJoinError decodes a refusal body into a typed error. The body is
// {code, reason, category?, retry_after_ms?}; a server that answered with a
// bare string still yields a usable reason.
func newJoinError(topic string, payload json.RawMessage) *JoinError {
	var body struct {
		Code         int    `json:"code"`
		Reason       string `json:"reason"`
		Category     string `json:"category"`
		RetryAfterMS int    `json:"retry_after_ms"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		_ = json.Unmarshal(payload, &body.Reason)
	}
	if body.Reason == "" {
		body.Reason = "join rejected"
	}
	return &JoinError{
		Topic:      topic,
		Code:       body.Code,
		Reason:     body.Reason,
		Category:   body.Category,
		RetryAfter: time.Duration(body.RetryAfterMS) * time.Millisecond,
	}
}

// handleFrame processes a non-join frame, returning an error to end the
// connection and trigger a reconnect. primary reports whether the frame arrived
// on the workspace channel, which is the only one that carries sequence numbers
// and the only one whose loss ends the session.
func (c *Client) handleFrame(msg *frame, primary bool) error {
	switch msg.Event {
	case evReply:
		// Acknowledgements need no action.
		return nil
	case evError, evClose:
		if primary {
			return fmt.Errorf("gateway: channel %s: %s", msg.Topic, msg.Event)
		}
		// An extra topic's channel died. The workspace stream is unaffected, so
		// log it and pick the topic up again on the next reconnect.
		c.logf("gateway: channel %s: %s; not rejoining until the next reconnect", msg.Topic, msg.Event)
		return nil
	}

	// Everything else is a workspace event.
	seq := extractSeq(msg.Payload)
	if primary {
		c.advanceSeq(seq)
		// The resume markers carry where the stream is now. Adopting it matters
		// most on a failure: a client that keeps asking for an evicted position
		// would never resume again.
		switch msg.Event {
		case EventResumed, EventResumeFailed:
			var probe struct {
				CurrentSeq int `json:"current_seq"`
			}
			if json.Unmarshal(msg.Payload, &probe) == nil {
				c.advanceSeq(probe.CurrentSeq)
			}
		}
	}
	c.emit(&Event{
		Type:  msg.Event,
		Topic: msg.Topic,
		Seq:   seq,
		Raw:   append(json.RawMessage(nil), msg.Payload...),
	})
	return nil
}

// advanceSeq moves the resume position forward, never back.
func (c *Client) advanceSeq(seq int) {
	if seq <= 0 {
		return
	}
	c.mu.Lock()
	if seq > c.lastSeq {
		c.lastSeq = seq
	}
	c.mu.Unlock()
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
//
// Each beat also arms a watchdog: a half-open TCP connection accepts writes
// forever and never delivers a reply, so waiting for the server's own timeout
// would leave the session silently dead for a minute. A missed reply closes the
// connection instead.
func (c *Client) heartbeatLoop(ctx context.Context, conn *wsconn.Conn, interval time.Duration, pong <-chan struct{}, done chan<- struct{}) {
	defer close(done)

	// Jitter the first beat so a fleet of clients does not synchronize.
	timer := time.NewTimer(time.Duration(float64(interval) * rand.Float64()))
	defer timer.Stop()

	watchdog := time.NewTimer(interval)
	watchdog.Stop()
	defer watchdog.Stop()

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

		watchdog.Reset(interval)
		select {
		case <-ctx.Done():
			return
		case <-pong:
			if !watchdog.Stop() {
				<-watchdog.C
			}
		case <-watchdog.C:
			c.logf("gateway: heartbeat unanswered after %s; reconnecting", interval)
			_ = conn.Close(1001, "heartbeat timeout")
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

func (c *Client) topic() string { return OrgTopic(c.orgID) }

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

// sleepCtx waits for d, reporting false if ctx was canceled first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// channelJoin is one topic's join on the current connection.
type channelJoin struct {
	topic   string
	primary bool
	joinRef string
	joined  bool
}

// joinBook matches join replies back to the topic that asked for them. A rejoin
// timer registers a new ref from its own goroutine while the read loop looks
// refs up, so it carries a lock.
type joinBook struct {
	mu      sync.Mutex
	byRef   map[string]*channelJoin
	byTopic map[string]*channelJoin
}

func newJoinBook() *joinBook {
	return &joinBook{
		byRef:   make(map[string]*channelJoin),
		byTopic: make(map[string]*channelJoin),
	}
}

// track records a join attempt under ref, retiring any earlier ref for the same
// topic so a late reply to a superseded attempt is ignored.
func (b *joinBook) track(topic string, primary bool, ref string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	cj := b.byTopic[topic]
	if cj == nil {
		cj = &channelJoin{topic: topic, primary: primary}
		b.byTopic[topic] = cj
	}
	delete(b.byRef, cj.joinRef)
	cj.joinRef = ref
	cj.joined = false
	b.byRef[ref] = cj
}

// pending returns the join still awaiting a reply on ref, or nil when the ref
// belongs to something else: a heartbeat, a superseded attempt, or a channel
// that is already open.
func (b *joinBook) pending(ref string) *channelJoin {
	if ref == "" {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	cj := b.byRef[ref]
	if cj == nil || cj.joined || cj.joinRef != ref {
		return nil
	}
	return cj
}

func (b *joinBook) markJoined(cj *channelJoin) {
	b.mu.Lock()
	defer b.mu.Unlock()
	cj.joined = true
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
