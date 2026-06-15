package gateway

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/warmbly/warmbly-go/internal/wsconn"
)

// --- pure logic tests (no network) ---

func TestOpcodeString(t *testing.T) {
	if OpHello.String() != "Hello" || OpDispatch.String() != "Dispatch" || OpHeartbeatACK.String() != "HeartbeatACK" {
		t.Error("unexpected opcode strings")
	}
	if Opcode(42).String() != "Opcode(42)" {
		t.Errorf("unknown opcode string = %q", Opcode(42).String())
	}
}

func TestIntentHas(t *testing.T) {
	i := IntentCampaigns | IntentWarmup
	if !i.Has(IntentCampaigns) || !i.Has(IntentWarmup) {
		t.Error("Has should report set bits")
	}
	if i.Has(IntentInbox) {
		t.Error("Has should not report unset bits")
	}
	if !IntentsAll.Has(IntentInbox) {
		t.Error("IntentsAll should include IntentInbox")
	}
	if IntentsDefault.Has(IntentInbox) {
		t.Error("IntentsDefault should exclude the high-volume IntentInbox")
	}
}

func TestEventInto(t *testing.T) {
	e := &Event{Type: EventEmailClicked, Raw: json.RawMessage(`{"contact_id":"ct_9","url":"https://x"}`)}
	var payload EngagementEvent
	if err := e.Into(&payload); err != nil {
		t.Fatalf("Into: %v", err)
	}
	if payload.ContactID != "ct_9" || payload.URL != "https://x" {
		t.Errorf("decoded %+v", payload)
	}
}

func TestBackoffBounds(t *testing.T) {
	c := New("tok", WithReconnectBackoff(time.Second, 30*time.Second))
	for attempt := 0; attempt < 12; attempt++ {
		d := c.backoff(attempt)
		if d < 0 || d > c.backoffMax {
			t.Errorf("attempt %d: backoff %v out of bounds (max %v)", attempt, d, c.backoffMax)
		}
	}
}

func TestResumeClassification(t *testing.T) {
	if isResumable(&disconnectError{resumable: false}) {
		t.Error("non-resumable disconnect classified resumable")
	}
	if !isResumable(&disconnectError{resumable: true}) {
		t.Error("resumable disconnect classified non-resumable")
	}
	if isResumable(&wsconn.CloseError{Code: 4004}) {
		t.Error("auth-failure close should not be resumable")
	}
	if !isResumable(&wsconn.CloseError{Code: 1001}) {
		t.Error("going-away close should be resumable")
	}
	if !isResumable(errors.New("boom")) {
		t.Error("network errors should be resumable")
	}
	if !isFatalClose(&wsconn.CloseError{Code: 4004}) {
		t.Error("4004 should be fatal")
	}
	if isFatalClose(&wsconn.CloseError{Code: 1001}) {
		t.Error("1001 should not be fatal")
	}
}

func TestProcessDispatchReady(t *testing.T) {
	c := New("tok")
	ready, err := c.process(context.Background(), []byte(`{"op":0,"t":"READY","s":1,"d":{"session_id":"sx"}}`))
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if !ready {
		t.Error("READY should put the session into the ready state")
	}
	if c.SessionID() != "sx" {
		t.Errorf("session id = %q, want sx", c.SessionID())
	}
	select {
	case ev := <-c.events:
		if ev.Type != EventReady || ev.Seq != 1 {
			t.Errorf("event = %+v", ev)
		}
	default:
		t.Error("expected a dispatched event")
	}
}

func TestProcessControlOps(t *testing.T) {
	c := New("tok")
	c.ackPending.Store(true)
	if _, err := c.process(context.Background(), []byte(`{"op":11}`)); err != nil {
		t.Fatalf("process ack: %v", err)
	}
	if c.ackPending.Load() {
		t.Error("HeartbeatACK should clear ackPending")
	}

	_, err := c.process(context.Background(), []byte(`{"op":7}`))
	var de *disconnectError
	if !errors.As(err, &de) || !de.resumable {
		t.Errorf("reconnect op should yield a resumable disconnect, got %v", err)
	}

	_, err = c.process(context.Background(), []byte(`{"op":9,"d":false}`))
	if !errors.As(err, &de) || de.resumable {
		t.Errorf("invalid-session(false) should yield a non-resumable disconnect, got %v", err)
	}
}

// --- integration test against a hand-rolled gateway server ---

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func TestGatewaySession(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go runTestGateway(t, ln)

	g := New("tok_test",
		WithURL("ws://"+ln.Addr().String()+"/"),
		WithIntents(IntentEmailEngagement),
		WithLogger(t.Logf),
	)

	opened := make(chan *EngagementEvent, 1)
	On(g, EventEmailOpened, func(_ context.Context, e *EngagementEvent) { opened <- e })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := g.Open(ctx); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer g.Close()

	if g.SessionID() != "sess_test" {
		t.Errorf("session id = %q, want sess_test", g.SessionID())
	}
	select {
	case e := <-opened:
		if e.ContactID != "ct_1" {
			t.Errorf("contact id = %q, want ct_1", e.ContactID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("did not receive email.opened event")
	}
}

// TestGatewayDoneOnClose verifies that Close terminates the session, closes
// Done, and reports a nil terminal error for a clean shutdown.
func TestGatewayDoneOnClose(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go runTestGateway(t, ln)

	g := New("tok_test", WithURL("ws://"+ln.Addr().String()+"/"), WithLogger(t.Logf))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := g.Open(ctx); err != nil {
		t.Fatalf("Open: %v", err)
	}

	_ = g.Close()
	select {
	case <-g.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("Done was not closed after Close")
	}
	if err := g.Err(); err != nil {
		t.Errorf("Err after clean shutdown = %v, want nil", err)
	}
}

func runTestGateway(t *testing.T, ln net.Listener) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	br := bufio.NewReader(conn)

	if err := serverHandshake(conn, br); err != nil {
		t.Logf("server handshake: %v", err)
		return
	}

	// Hello with a heartbeat interval.
	_ = sendEnvelope(conn, 10, "", nil, map[string]any{"heartbeat_interval": 500})

	// Expect Identify.
	op, payload, err := readClientFrame(br)
	if err != nil {
		t.Logf("read identify: %v", err)
		return
	}
	if op == 0x1 {
		var env struct {
			Op      int `json:"op"`
			Intents int `json:"-"`
		}
		_ = json.Unmarshal(payload, &env)
		if env.Op != 2 {
			t.Errorf("expected Identify (op 2), got op %d", env.Op)
		}
	}

	// READY, then a dispatched event.
	one, two := 1, 2
	_ = sendEnvelope(conn, 0, "READY", &one, map[string]any{"session_id": "sess_test"})
	_ = sendEnvelope(conn, 0, "email.opened", &two, map[string]any{"contact_id": "ct_1"})

	// Acknowledge heartbeats until the client disconnects.
	for {
		op, payload, err := readClientFrame(br)
		if err != nil {
			return
		}
		if op == 0x8 { // close
			return
		}
		if op == 0x1 {
			var e struct {
				Op int `json:"op"`
			}
			_ = json.Unmarshal(payload, &e)
			if e.Op == 1 {
				_ = sendEnvelope(conn, 11, "", nil, nil)
			}
		}
	}
}

func serverHandshake(conn net.Conn, br *bufio.Reader) error {
	req, err := http.ReadRequest(br)
	if err != nil {
		return err
	}
	key := req.Header.Get("Sec-WebSocket-Key")
	h := sha1.New()
	_, _ = io.WriteString(h, key+wsGUID)
	accept := base64.StdEncoding.EncodeToString(h.Sum(nil))
	_, err = io.WriteString(conn,
		"HTTP/1.1 101 Switching Protocols\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Accept: "+accept+"\r\n\r\n")
	return err
}

// sendEnvelope writes a server (unmasked) text frame carrying a gateway envelope.
func sendEnvelope(conn net.Conn, op int, typ string, seq *int, data any) error {
	m := map[string]any{"op": op}
	if typ != "" {
		m["t"] = typ
	}
	if seq != nil {
		m["s"] = *seq
	}
	if data != nil {
		b, _ := json.Marshal(data)
		m["d"] = json.RawMessage(b)
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return writeServerFrame(conn, 0x1, b)
}

func writeServerFrame(conn net.Conn, opcode byte, payload []byte) error {
	hdr := []byte{0x80 | opcode}
	n := len(payload)
	switch {
	case n < 126:
		hdr = append(hdr, byte(n))
	case n < 65536:
		hdr = append(hdr, 126)
		var e [2]byte
		binary.BigEndian.PutUint16(e[:], uint16(n))
		hdr = append(hdr, e[:]...)
	default:
		hdr = append(hdr, 127)
		var e [8]byte
		binary.BigEndian.PutUint64(e[:], uint64(n))
		hdr = append(hdr, e[:]...)
	}
	_, err := conn.Write(append(hdr, payload...))
	return err
}

func readClientFrame(br *bufio.Reader) (opcode byte, payload []byte, err error) {
	var h [2]byte
	if _, err = io.ReadFull(br, h[:]); err != nil {
		return 0, nil, err
	}
	opcode = h[0] & 0x0F
	masked := h[1]&0x80 != 0
	n := int64(h[1] & 0x7F)
	switch n {
	case 126:
		var e [2]byte
		if _, err = io.ReadFull(br, e[:]); err != nil {
			return 0, nil, err
		}
		n = int64(binary.BigEndian.Uint16(e[:]))
	case 127:
		var e [8]byte
		if _, err = io.ReadFull(br, e[:]); err != nil {
			return 0, nil, err
		}
		n = int64(binary.BigEndian.Uint64(e[:]))
	}
	var mask [4]byte
	if masked {
		if _, err = io.ReadFull(br, mask[:]); err != nil {
			return 0, nil, err
		}
	}
	payload = make([]byte, n)
	if _, err = io.ReadFull(br, payload); err != nil {
		return 0, nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return opcode, payload, nil
}
