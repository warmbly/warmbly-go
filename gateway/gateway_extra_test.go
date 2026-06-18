package gateway

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/warmbly/warmbly-go/internal/wsconn"
)

// gwCapture is a small thread-safe logger sink used to assert that the client's
// logger was invoked for non-fatal diagnostics.
type gwCapture struct {
	mu   sync.Mutex
	msgs []string
}

func (c *gwCapture) logf(format string, args ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Render eagerly so the slice owns no shared backing arrays.
	c.msgs = append(c.msgs, fmt.Sprintf(format, args...))
}

func (c *gwCapture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.msgs)
}

func TestGWWithEventBuffer(t *testing.T) {
	// A positive buffer size is honored.
	c := New("tok", WithEventBuffer(8))
	if got := cap(c.events); got != 8 {
		t.Errorf("cap(events) = %d, want 8", got)
	}
	// Zero leaves the default 64 untouched.
	d := New("tok", WithEventBuffer(0))
	if got := cap(d.events); got != 64 {
		t.Errorf("cap(events) with 0 = %d, want default 64", got)
	}
}

func TestGWDisconnectErrorMessage(t *testing.T) {
	de := &disconnectError{resumable: true, reason: "server requested reconnect"}
	if got, want := de.Error(), "gateway: server requested reconnect"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestGWIsFatalCloseNonClose(t *testing.T) {
	if isFatalClose(errors.New("boom")) {
		t.Error("a generic error must not be classified as a fatal close")
	}
}

func TestGWFireOrdering(t *testing.T) {
	c := New("tok")

	var order []string
	var mu sync.Mutex
	record := func(s string) {
		mu.Lock()
		order = append(order, s)
		mu.Unlock()
	}

	c.HandleAny(func(_ context.Context, _ *Event) { record("any1") })
	c.HandleAny(func(_ context.Context, _ *Event) { record("any2") })
	c.Handle(EventEmailOpened, func(_ context.Context, _ *Event) { record("named1") })
	c.Handle(EventEmailOpened, func(_ context.Context, _ *Event) { record("named2") })

	c.fire(context.Background(), &Event{Type: EventEmailOpened})

	want := []string{"any1", "any2", "named1", "named2"}
	if len(order) != len(want) {
		t.Fatalf("fired %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("fired %v, want %v", order, want)
		}
	}
}

func TestGWHandleNilMap(t *testing.T) {
	c := New("tok")
	// Force the nil-map branch in Handle (New normally initializes it).
	c.mu.Lock()
	c.handlers = nil
	c.mu.Unlock()

	called := false
	c.Handle(EventEmailOpened, func(_ context.Context, _ *Event) { called = true })
	c.fire(context.Background(), &Event{Type: EventEmailOpened})
	if !called {
		t.Error("handler registered against a nil map was not invoked")
	}
}

func TestGWOnSuccess(t *testing.T) {
	c := New("tok")
	got := make(chan *EngagementEvent, 1)
	On(c, EventEmailOpened, func(_ context.Context, e *EngagementEvent) { got <- e })

	c.fire(context.Background(), &Event{
		Type: EventEmailOpened,
		Raw:  json.RawMessage(`{"contact_id":"ct_42","url":"https://x"}`),
	})

	select {
	case e := <-got:
		if e.ContactID != "ct_42" || e.URL != "https://x" {
			t.Errorf("payload = %+v", e)
		}
	default:
		t.Fatal("typed handler was not called")
	}
}

func TestGWOnDecodeError(t *testing.T) {
	cl := &gwCapture{}
	c := New("tok", WithLogger(cl.logf))

	called := false
	On(c, EventEmailOpened, func(_ context.Context, _ *EngagementEvent) { called = true })

	c.fire(context.Background(), &Event{
		Type: EventEmailOpened,
		Raw:  json.RawMessage(`{not json`),
	})

	if called {
		t.Error("handler must not run when the payload fails to decode")
	}
	if cl.count() == 0 {
		t.Error("decode failure should be logged")
	}
}

func TestGWEventIntoEmptyRaw(t *testing.T) {
	e := &Event{Type: EventEmailOpened}
	v := &EngagementEvent{ContactID: "keep"}
	if err := e.Into(v); err != nil {
		t.Fatalf("Into with empty Raw: %v", err)
	}
	if v.ContactID != "keep" {
		t.Errorf("target mutated: %+v", v)
	}
}

func TestGWOpcodeStringDefault(t *testing.T) {
	if got := Opcode(99).String(); got != "Opcode(99)" {
		t.Errorf("Opcode(99).String() = %q", got)
	}
}

func TestGWOpcodeStringAll(t *testing.T) {
	cases := map[Opcode]string{
		OpDispatch:       "Dispatch",
		OpHeartbeat:      "Heartbeat",
		OpIdentify:       "Identify",
		OpResume:         "Resume",
		OpReconnect:      "Reconnect",
		OpInvalidSession: "InvalidSession",
		OpHello:          "Hello",
		OpHeartbeatACK:   "HeartbeatACK",
	}
	for op, want := range cases {
		if got := op.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", int(op), got, want)
		}
	}
}

func TestGWItoa(t *testing.T) {
	cases := map[int]string{0: "0", -7: "-7", 123: "123"}
	for in, want := range cases {
		if got := itoa(in); got != want {
			t.Errorf("itoa(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestGWProcessMalformedFrame(t *testing.T) {
	cl := &gwCapture{}
	c := New("tok", WithLogger(cl.logf))
	ready, err := c.process(context.Background(), []byte(`{not json`))
	if ready || err != nil {
		t.Errorf("malformed frame -> (%v, %v), want (false, nil)", ready, err)
	}
	if cl.count() == 0 {
		t.Error("malformed frame should be logged")
	}
}

func TestGWProcessUpdatesSeq(t *testing.T) {
	c := New("tok")
	// A heartbeat-ACK frame carrying a sequence: process must record the seq.
	ready, err := c.process(context.Background(), []byte(`{"op":11,"s":7}`))
	if ready || err != nil {
		t.Errorf("process -> (%v, %v), want (false, nil)", ready, err)
	}
	c.mu.RLock()
	seq := c.seq
	c.mu.RUnlock()
	if seq != 7 {
		t.Errorf("c.seq = %d, want 7", seq)
	}
}

func TestGWProcessHeartbeatNoConn(t *testing.T) {
	c := New("tok")
	// OpHeartbeat with no connection: the inner send fails but is swallowed.
	ready, err := c.process(context.Background(), []byte(`{"op":1}`))
	if ready || err != nil {
		t.Errorf("heartbeat with no conn -> (%v, %v), want (false, nil)", ready, err)
	}
}

func TestGWProcessUnexpectedOp(t *testing.T) {
	cl := &gwCapture{}
	c := New("tok", WithLogger(cl.logf))
	ready, err := c.process(context.Background(), []byte(`{"op":5}`))
	if ready || err != nil {
		t.Errorf("unexpected op -> (%v, %v), want (false, nil)", ready, err)
	}
	if cl.count() == 0 {
		t.Error("unexpected op should be logged")
	}
}

func TestGWDispatchResumed(t *testing.T) {
	c := New("tok")
	env := &envelope{Op: OpDispatch, Type: string(EventResumed)}
	if !c.dispatch(env) {
		t.Error("RESUMED event should report ready==true")
	}
	select {
	case ev := <-c.events:
		if ev.Type != EventResumed {
			t.Errorf("event type = %q, want RESUMED", ev.Type)
		}
	default:
		t.Error("expected a queued RESUMED event")
	}
}

func TestGWDispatchBufferFullDrop(t *testing.T) {
	cl := &gwCapture{}
	c := New("tok", WithEventBuffer(1), WithLogger(cl.logf))

	// Fill the single-slot buffer.
	if c.dispatch(&envelope{Op: OpDispatch, Type: string(EventEmailOpened)}) {
		t.Error("a non-READY event should not report ready")
	}
	// The next dispatch cannot enqueue and must be dropped (and logged).
	if c.dispatch(&envelope{Op: OpDispatch, Type: string(EventEmailClicked)}) {
		t.Error("a dropped event should not report ready")
	}
	if cl.count() == 0 {
		t.Error("a full buffer should log a dropped event")
	}
	if got := len(c.events); got != 1 {
		t.Errorf("buffer length = %d, want 1 (second event dropped)", got)
	}
}

func TestGWSafeCallRecoversPanic(t *testing.T) {
	cl := &gwCapture{}
	c := New("tok", WithLogger(cl.logf))
	c.Handle(EventEmailOpened, func(_ context.Context, _ *Event) { panic("boom") })

	// fire must return normally despite the panicking handler.
	c.fire(context.Background(), &Event{Type: EventEmailOpened})

	if cl.count() == 0 {
		t.Error("a recovered panic should be logged")
	}
}

func TestGWBackoffTinyDelay(t *testing.T) {
	// A 1ns minimum makes the first delay's half == 0, exercising the branch
	// that returns d directly instead of adding jitter.
	c := New("tok", WithReconnectBackoff(time.Nanosecond, time.Second))
	if got := c.backoff(0); got != time.Nanosecond {
		t.Errorf("backoff(0) = %v, want 1ns (no jitter when half==0)", got)
	}
	// A larger attempt overflows past backoffMax and clamps to it.
	if got := c.backoff(80); got <= 0 || got > c.backoffMax {
		t.Errorf("backoff(80) = %v, want within (0, %v]", got, c.backoffMax)
	}
}

func TestGWSendNotConnected(t *testing.T) {
	c := New("tok")
	err := c.send(context.Background(), OpHeartbeat, 1)
	if err == nil {
		t.Fatal("send without a connection should error")
	}
	if got := err.Error(); got != "gateway: not connected" {
		t.Errorf("send error = %q, want %q", got, "gateway: not connected")
	}
}

func TestGWSendMarshalError(t *testing.T) {
	c := New("tok")
	// A channel cannot be marshaled to JSON: send must surface that error
	// before reaching the connection check.
	err := c.send(context.Background(), OpHeartbeat, make(chan int))
	var ute *json.UnsupportedTypeError
	if !errors.As(err, &ute) {
		t.Errorf("send with unmarshalable data = %v, want json.UnsupportedTypeError", err)
	}
}

func TestGWDefaultLoggerNoop(t *testing.T) {
	// New without WithLogger installs a no-op logger; exercise its body so the
	// default-logger statement is covered. A malformed frame triggers a log.
	c := New("tok")
	if _, err := c.process(context.Background(), []byte(`{bad`)); err != nil {
		t.Fatalf("process: %v", err)
	}
}

func TestGWRunOnceDialError(t *testing.T) {
	// An unsupported URL scheme makes the dial fail synchronously with no
	// network I/O, driving runOnce's dial-error branch.
	c := New("tok", WithURL("http://127.0.0.1:1/"), WithLogger(t.Logf))
	resumable, hadReady, err := c.runOnce(context.Background())
	if err == nil {
		t.Fatal("runOnce against an unsupported scheme should error")
	}
	if !resumable {
		t.Error("a dial error should be classified resumable")
	}
	if hadReady {
		t.Error("a dial error must not report hadReady")
	}
}

func TestGWOpenTokenRequired(t *testing.T) {
	err := New("").Open(context.Background())
	if err == nil || err.Error() != "gateway: token is required" {
		t.Errorf("Open with empty token = %v, want token-required error", err)
	}
}

func TestGWOpenAlreadyOpen(t *testing.T) {
	c := New("tok")
	// Flip the running flag through the mutex (white-box) to simulate an
	// already-open client without starting any goroutines.
	c.mu.Lock()
	c.running = true
	c.mu.Unlock()

	err := c.Open(context.Background())
	if err == nil || err.Error() != "gateway: already open" {
		t.Errorf("Open while running = %v, want already-open error", err)
	}
}

// TestGWOpenCanceledBeforeReady drives Open's ctx-canceled branch: the context
// is already canceled, so Open returns the context error without ever reaching
// READY, exercising signalReady on the cancellation path.
func TestGWOpenCanceledBeforeReady(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	// Accept and immediately drop connections so no READY ever arrives.
	go func() {
		for {
			conn, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	c := New("tok", WithURL("ws://"+ln.Addr().String()+"/"), WithLogger(t.Logf))
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before Open observes READY

	if err := c.Open(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Open with canceled ctx = %v, want context.Canceled", err)
	}
	_ = c.Close()
}

// gwWriteCloseFrame writes a server close frame carrying a 2-byte status code.
func gwWriteCloseFrame(conn net.Conn, code int) error {
	payload := []byte{byte(code >> 8), byte(code)}
	return writeServerFrame(conn, 0x8, payload)
}

// TestGWFatalClose drives the authentication-failure path: the server completes
// the handshake then closes with code 4004. manage classifies it as fatal,
// records the terminal error, and signals Open with that error, which in turn
// cancels its context (the readyErr branch in Open). Done is then closed with
// the fatal error reported by Err.
func TestGWFatalClose(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, aerr := ln.Accept()
		if aerr != nil {
			return
		}
		defer conn.Close()
		br := bufio.NewReader(conn)
		if herr := serverHandshake(conn, br); herr != nil {
			return
		}
		_ = sendEnvelope(conn, 10, "", nil, map[string]any{"heartbeat_interval": 500})
		// Read the Identify, then reject with a fatal auth-failure close.
		_, _, _ = readClientFrame(br)
		_ = gwWriteCloseFrame(conn, 4004)
	}()

	c := New("tok", WithURL("ws://"+ln.Addr().String()+"/"), WithLogger(t.Logf))
	openErr := c.Open(context.Background())
	if openErr == nil {
		t.Fatal("Open should return the fatal close error")
	}
	var ce *wsconn.CloseError
	if !errors.As(openErr, &ce) || ce.Code != 4004 {
		t.Errorf("Open error = %v, want CloseError 4004", openErr)
	}

	select {
	case <-c.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("Done was not closed after a fatal close")
	}
	if terr := c.Err(); !errors.As(terr, &ce) || ce.Code != 4004 {
		t.Errorf("Err = %v, want CloseError 4004", terr)
	}
	_ = c.Close()
}

// TestGWReconnectResume drives the reconnect-and-resume loop: a first
// connection reaches READY, the server then requests a reconnect (op 7); the
// client reconnects, the server observes a Resume (op 6) carrying the prior
// session id, and replays a RESUMED event. This exercises manage's reconnect
// branch and runOnce's resume handshake.
func TestGWReconnectResume(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	resumeSeen := make(chan int, 1)
	go func() {
		// First connection: identify -> READY -> request reconnect.
		conn1, aerr := ln.Accept()
		if aerr != nil {
			return
		}
		br1 := bufio.NewReader(conn1)
		if serverHandshake(conn1, br1) != nil {
			conn1.Close()
			return
		}
		_ = sendEnvelope(conn1, 10, "", nil, map[string]any{"heartbeat_interval": 10000})
		_, _, _ = readClientFrame(br1) // Identify
		one := 1
		_ = sendEnvelope(conn1, 0, "READY", &one, map[string]any{"session_id": "sess_resume"})
		_ = sendEnvelope(conn1, 7, "", nil, nil) // ask the client to reconnect
		conn1.Close()

		// Second connection: expect a Resume frame, then send RESUMED.
		conn2, aerr := ln.Accept()
		if aerr != nil {
			return
		}
		defer conn2.Close()
		br2 := bufio.NewReader(conn2)
		if serverHandshake(conn2, br2) != nil {
			return
		}
		_ = sendEnvelope(conn2, 10, "", nil, map[string]any{"heartbeat_interval": 10000})
		op, payload, rerr := readClientFrame(br2)
		if rerr == nil && op == 0x1 {
			var env struct {
				Op int `json:"op"`
			}
			_ = json.Unmarshal(payload, &env)
			resumeSeen <- env.Op
		}
		two := 2
		_ = sendEnvelope(conn2, 0, "RESUMED", &two, map[string]any{})
		// Keep the connection open until the client tears it down.
		for {
			if _, _, e := readClientFrame(br2); e != nil {
				return
			}
		}
	}()

	resumed := make(chan struct{}, 1)
	c := New("tok",
		WithURL("ws://"+ln.Addr().String()+"/"),
		WithReconnectBackoff(time.Millisecond, 10*time.Millisecond),
		WithLogger(t.Logf),
	)
	c.Handle(EventResumed, func(_ context.Context, _ *Event) {
		select {
		case resumed <- struct{}{}:
		default:
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if oerr := c.Open(ctx); oerr != nil {
		t.Fatalf("Open: %v", oerr)
	}
	t.Cleanup(func() { _ = c.Close() })

	select {
	case op := <-resumeSeen:
		if op != int(OpResume) {
			t.Errorf("server saw op %d on reconnect, want Resume (%d)", op, int(OpResume))
		}
	case <-time.After(4 * time.Second):
		t.Fatal("server never observed a Resume frame")
	}
	select {
	case <-resumed:
	case <-time.After(4 * time.Second):
		t.Fatal("client never dispatched RESUMED")
	}
}

// gwDialPair starts a listener, accepts one connection and performs the server
// side of the WebSocket handshake, then dials it as a wsconn client. It returns
// the raw server conn (with a buffered reader for reading client frames) and the
// client wsconn.Conn. Both are closed on cleanup.
func gwDialPair(t *testing.T) (srv net.Conn, br *bufio.Reader, cli *wsconn.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	type accepted struct {
		conn net.Conn
		br   *bufio.Reader
		err  error
	}
	ch := make(chan accepted, 1)
	go func() {
		conn, aerr := ln.Accept()
		if aerr != nil {
			ch <- accepted{err: aerr}
			return
		}
		r := bufio.NewReader(conn)
		ch <- accepted{conn: conn, br: r, err: serverHandshake(conn, r)}
	}()

	cli, resp, err := wsconn.Dial(context.Background(), "ws://"+ln.Addr().String()+"/", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if resp != nil {
		_ = resp.Body.Close()
	}
	a := <-ch
	if a.err != nil {
		t.Fatalf("server handshake: %v", a.err)
	}
	t.Cleanup(func() {
		_ = cli.Close(1000, "")
		_ = a.conn.Close()
	})
	return a.conn, a.br, cli
}

func TestGWReadHelloErrors(t *testing.T) {
	t.Run("wrong op", func(t *testing.T) {
		srv, _, cli := gwDialPair(t)
		// Send a heartbeat-ACK instead of Hello.
		if err := sendEnvelope(srv, 11, "", nil, nil); err != nil {
			t.Fatalf("send: %v", err)
		}
		c := New("tok")
		if _, err := c.readHello(context.Background(), cli); err == nil {
			t.Error("readHello should reject a non-Hello first frame")
		}
	})

	t.Run("decode envelope", func(t *testing.T) {
		srv, _, cli := gwDialPair(t)
		// A frame that is not valid JSON.
		if err := writeServerFrame(srv, 0x1, []byte(`{not json`)); err != nil {
			t.Fatalf("write: %v", err)
		}
		c := New("tok")
		if _, err := c.readHello(context.Background(), cli); err == nil {
			t.Error("readHello should fail to decode a malformed envelope")
		}
	})

	t.Run("decode payload", func(t *testing.T) {
		srv, _, cli := gwDialPair(t)
		// Valid Hello envelope but a non-object payload that cannot decode into
		// helloData.
		if err := writeServerFrame(srv, 0x1, []byte(`{"op":10,"d":"oops"}`)); err != nil {
			t.Fatalf("write: %v", err)
		}
		c := New("tok")
		if _, err := c.readHello(context.Background(), cli); err == nil {
			t.Error("readHello should fail to decode a bad Hello payload")
		}
	})

	t.Run("read error", func(t *testing.T) {
		srv, _, cli := gwDialPair(t)
		// Close the server side so the client read fails.
		_ = srv.Close()
		c := New("tok")
		if _, err := c.readHello(context.Background(), cli); err == nil {
			t.Error("readHello should surface a read error")
		}
	})

	t.Run("zero interval defaults", func(t *testing.T) {
		srv, _, cli := gwDialPair(t)
		// A Hello with no payload yields a zero interval; readHello returns a
		// helloData whose interval is 0 (runOnce then applies the default).
		if err := sendEnvelope(srv, 10, "", nil, nil); err != nil {
			t.Fatalf("send: %v", err)
		}
		c := New("tok")
		hello, err := c.readHello(context.Background(), cli)
		if err != nil {
			t.Fatalf("readHello: %v", err)
		}
		if hello.HeartbeatInterval != 0 {
			t.Errorf("interval = %d, want 0", hello.HeartbeatInterval)
		}
	})
}

// TestGWRunOnceZeroInterval drives runOnce against a server that advertises a
// zero heartbeat interval, exercising the interval<=0 default branch, and then
// reaches READY before requesting a reconnect.
func TestGWRunOnceZeroInterval(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, aerr := ln.Accept()
		if aerr != nil {
			return
		}
		defer conn.Close()
		br := bufio.NewReader(conn)
		if serverHandshake(conn, br) != nil {
			return
		}
		// heartbeat_interval 0 -> runOnce falls back to its 30s default.
		_ = sendEnvelope(conn, 10, "", nil, map[string]any{"heartbeat_interval": 0})
		_, _, _ = readClientFrame(br) // Identify
		one := 1
		_ = sendEnvelope(conn, 0, "READY", &one, map[string]any{"session_id": "sess_zero"})
		_ = sendEnvelope(conn, 7, "", nil, nil) // reconnect to end runOnce
	}()

	c := New("tok", WithURL("ws://"+ln.Addr().String()+"/"), WithLogger(t.Logf))
	resumable, hadReady, rerr := c.runOnce(context.Background())
	if rerr == nil {
		t.Fatal("runOnce should end with the reconnect disconnect error")
	}
	if !hadReady {
		t.Error("runOnce should report hadReady after READY")
	}
	if !resumable {
		t.Error("a server-requested reconnect is resumable")
	}
	c.mu.RLock()
	hb := c.hbInterval
	c.mu.RUnlock()
	if hb != 30*time.Second {
		t.Errorf("heartbeat interval = %v, want 30s default", hb)
	}
}

// TestGWRunOnceHelloError drives runOnce's readHello-error branch: the server
// completes the WS handshake but sends a non-Hello first frame.
func TestGWRunOnceHelloError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, aerr := ln.Accept()
		if aerr != nil {
			return
		}
		defer conn.Close()
		br := bufio.NewReader(conn)
		if serverHandshake(conn, br) != nil {
			return
		}
		// Wrong first op (heartbeat-ACK instead of Hello).
		_ = sendEnvelope(conn, 11, "", nil, nil)
		for {
			if _, _, e := readClientFrame(br); e != nil {
				return
			}
		}
	}()

	c := New("tok", WithURL("ws://"+ln.Addr().String()+"/"), WithLogger(t.Logf))
	resumable, hadReady, rerr := c.runOnce(context.Background())
	if rerr == nil {
		t.Fatal("runOnce should fail when the first frame is not Hello")
	}
	if !resumable {
		t.Error("a hello error is resumable")
	}
	if hadReady {
		t.Error("hadReady must be false when hello fails")
	}
}

// TestGWHeartbeatLoopBeatsAndAcks runs heartbeatLoop directly with a tiny
// interval against a live connection that acknowledges beats, covering the
// normal send path. The connection is wired via the client's send.
func TestGWHeartbeatLoopBeatsAndAcks(t *testing.T) {
	srv, br, cli := gwDialPair(t)
	c := New("tok", WithLogger(t.Logf))
	c.setConn(cli)

	beat := make(chan struct{}, 4)
	// Drain client frames; reply with an ACK to each heartbeat so ackPending
	// stays clear and the loop keeps beating.
	go func() {
		for {
			op, payload, err := readClientFrame(br)
			if err != nil {
				return
			}
			if op == 0x1 {
				var e struct {
					Op int `json:"op"`
				}
				_ = json.Unmarshal(payload, &e)
				if e.Op == int(OpHeartbeat) {
					select {
					case beat <- struct{}{}:
					default:
					}
					_ = sendEnvelope(srv, 11, "", nil, nil)
					c.ackPending.Store(false)
				}
			}
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go c.heartbeatLoop(ctx, cli, 5*time.Millisecond, done)

	// Wait for a couple of beats to flow through the send path.
	for i := 0; i < 2; i++ {
		select {
		case <-beat:
		case <-time.After(3 * time.Second):
			cancel()
			<-done
			t.Fatal("heartbeatLoop did not send heartbeats")
		}
	}
	cancel()
	<-done
}

// TestGWHeartbeatLoopZombie covers the zombie-detection branch: with no ACKs,
// ackPending stays set after the first beat, so the next tick closes the
// connection and the loop returns.
func TestGWHeartbeatLoopZombie(t *testing.T) {
	_, br, cli := gwDialPair(t)
	cl := &gwCapture{}
	c := New("tok", WithLogger(cl.logf))
	c.setConn(cli)

	// Drain client frames so writes never block, but never ACK.
	go func() {
		for {
			if _, _, err := readClientFrame(br); err != nil {
				return
			}
		}
	}()

	// Pre-set ackPending so the very first tick trips zombie detection
	// deterministically.
	c.ackPending.Store(true)

	done := make(chan struct{})
	go c.heartbeatLoop(context.Background(), cli, 5*time.Millisecond, done)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("zombie heartbeat did not force the loop to return")
	}
	if cl.count() == 0 {
		t.Error("an unacknowledged heartbeat should be logged")
	}
}

// TestGWHeartbeatLoopSendFail covers heartbeatLoop's send-failure branch: the
// connection is closed before the first beat, so send fails while the context
// is still live, prompting a log and an explicit connection close.
func TestGWHeartbeatLoopSendFail(t *testing.T) {
	_, br, cli := gwDialPair(t)
	cl := &gwCapture{}
	c := New("tok", WithLogger(cl.logf))
	c.setConn(cli)

	// Drain frames until the connection drops.
	go func() {
		for {
			if _, _, err := readClientFrame(br); err != nil {
				return
			}
		}
	}()

	// Close the client connection so the heartbeat write fails.
	_ = cli.Close(1000, "")

	done := make(chan struct{})
	// ackPending stays false, so the first tick goes straight to send, which
	// fails on the closed connection.
	go c.heartbeatLoop(context.Background(), cli, 5*time.Millisecond, done)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("heartbeatLoop did not return after a send failure")
	}
	if cl.count() == 0 {
		t.Error("a heartbeat send failure should be logged")
	}
}

// TestGWManageNonResumable covers manage's session-reset branch: an
// invalid-session(false) control op ends the connection non-resumably, so
// manage clears the session id before backing off. The context is then canceled
// during backoff to exercise the ctx.Done() path.
func TestGWManageNonResumable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, aerr := ln.Accept()
		if aerr != nil {
			return
		}
		defer conn.Close()
		br := bufio.NewReader(conn)
		if serverHandshake(conn, br) != nil {
			return
		}
		_ = sendEnvelope(conn, 10, "", nil, map[string]any{"heartbeat_interval": 10000})
		_, _, _ = readClientFrame(br) // Identify
		one := 1
		_ = sendEnvelope(conn, 0, "READY", &one, map[string]any{"session_id": "sess_x"})
		// Then declare the session invalid and non-resumable.
		_ = sendEnvelope(conn, 9, "", nil, false)
		// Drain until the client disconnects.
		for {
			if _, _, e := readClientFrame(br); e != nil {
				return
			}
		}
	}()

	cl := &gwCapture{}
	c := New("tok",
		WithURL("ws://"+ln.Addr().String()+"/"),
		WithReconnectBackoff(2*time.Second, 5*time.Second),
		WithLogger(cl.logf),
	)
	ctx, cancel := context.WithCancel(context.Background())
	if oerr := c.Open(ctx); oerr != nil {
		cancel()
		t.Fatalf("Open: %v", oerr)
	}

	// After READY the server sends invalid-session(false). manage clears the
	// session and enters a long backoff; cancel during it to take the
	// ctx.Done() path. Poll until the session id is cleared.
	deadline := time.After(4 * time.Second)
	for c.SessionID() != "" {
		select {
		case <-deadline:
			cancel()
			_ = c.Close()
			t.Fatal("session id was not cleared after a non-resumable disconnect")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel() // unblock the backoff select via ctx.Done()
	select {
	case <-c.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("Done not closed after cancellation")
	}
	_ = c.Close()
}
