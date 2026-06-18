package wsconn

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// wsHandshakeServer starts a TCP listener whose first accepted connection
// performs the server side of the RFC 6455 opening handshake. The supplied
// reply function chooses how the server responds: it receives the parsed
// client request and the connection, and must write the raw HTTP response.
// The function returns the listener address (host:port) and a channel that
// receives the accepted, post-handshake net.Conn (or nil if reply closed it).
func wsHandshakeServer(t *testing.T, reply func(req *http.Request, conn net.Conn)) (string, <-chan net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	connCh := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			connCh <- nil
			return
		}
		br := bufio.NewReader(c)
		req, err := http.ReadRequest(br)
		if err != nil {
			_ = c.Close()
			connCh <- nil
			return
		}
		reply(req, c)
		connCh <- c
	}()
	return ln.Addr().String(), connCh
}

// serverAcceptKey computes Sec-WebSocket-Accept the way an RFC 6455 server
// would, independently of the production acceptKey helper.
func serverAcceptKey(t *testing.T, key string) string {
	t.Helper()
	h := sha1.New()
	_, _ = io.WriteString(h, key+guid)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// writeRaw writes the entire string s to conn, failing the test on error.
func writeRaw(t *testing.T, conn net.Conn, s string) {
	t.Helper()
	if _, err := io.WriteString(conn, s); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

// wsWriteFrameRaw writes a single unmasked frame directly to c's underlying
// connection with an explicit FIN bit and opcode. Server-to-client frames are
// unmasked, which readFrame accepts; this lets a test craft fragmented frames
// (FIN=0) that the production writeFrame never produces.
func wsWriteFrameRaw(t *testing.T, c *Conn, fin bool, opcode byte, payload []byte) {
	t.Helper()
	b0 := opcode
	if fin {
		b0 |= 0x80
	}
	n := len(payload)
	var hdr []byte
	hdr = append(hdr, b0)
	switch {
	case n < 126:
		hdr = append(hdr, byte(n))
	case n < 65536:
		hdr = append(hdr, 126, byte(n>>8), byte(n))
	default:
		hdr = append(hdr, 127, 0, 0, 0, 0, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	}
	if _, err := c.conn.Write(hdr); err != nil {
		t.Errorf("write frame header: %v", err)
		return
	}
	if n > 0 {
		if _, err := c.conn.Write(payload); err != nil {
			t.Errorf("write frame payload: %v", err)
		}
	}
}

func TestWSDialHappyPath(t *testing.T) {
	var sawAuth string
	addr, connCh := wsHandshakeServer(t, func(req *http.Request, conn net.Conn) {
		sawAuth = req.Header.Get("Authorization")
		key := req.Header.Get("Sec-WebSocket-Key")
		writeRaw(t, conn, "HTTP/1.1 101 Switching Protocols\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Accept: "+serverAcceptKey(t, key)+"\r\n\r\n")
	})

	ctx := context.Background()
	conn, resp, err := Dial(ctx, "ws://"+addr+"/gateway", http.Header{"Authorization": {"Bearer x"}})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(1000, "") })

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Errorf("status = %d, want 101", resp.StatusCode)
	}
	if sawAuth != "Bearer x" {
		t.Errorf("server saw Authorization = %q, want %q", sawAuth, "Bearer x")
	}

	// Cover SetMaxMessage and exercise a real message exchange in both
	// directions to prove the connection works.
	conn.SetMaxMessage(1 << 20)

	srv := <-connCh
	if srv == nil {
		t.Fatal("server connection was not established")
	}
	peer := &Conn{conn: srv, br: bufio.NewReader(srv), maxMessage: defaultMaxMessage}

	go func() {
		if err := conn.WriteMessage(ctx, MessageText, []byte("ping-the-server")); err != nil {
			t.Errorf("client write: %v", err)
		}
	}()
	mt, got, err := peer.ReadMessage(ctx)
	if err != nil {
		t.Fatalf("server read: %v", err)
	}
	if mt != MessageText || string(got) != "ping-the-server" {
		t.Errorf("server got mt=%d %q", mt, got)
	}
}

func TestWSDialErrors(t *testing.T) {
	t.Run("unparseable url", func(t *testing.T) {
		_, _, err := Dial(context.Background(), "ht tp://bad url/\x7f", nil)
		if err == nil {
			t.Fatal("expected an error for an unparseable url")
		}
		if !strings.Contains(err.Error(), "parse url") {
			t.Errorf("error = %v, want a parse url error", err)
		}
	})

	t.Run("unsupported scheme", func(t *testing.T) {
		_, _, err := Dial(context.Background(), "http://example.invalid/", nil)
		if err == nil {
			t.Fatal("expected an error for an unsupported scheme")
		}
		if !strings.Contains(err.Error(), "unsupported scheme") {
			t.Errorf("error = %v, want unsupported scheme", err)
		}
	})

	t.Run("non-101 status", func(t *testing.T) {
		addr, _ := wsHandshakeServer(t, func(req *http.Request, conn net.Conn) {
			writeRaw(t, conn, "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
		})
		_, resp, err := Dial(context.Background(), "ws://"+addr+"/", nil)
		if err == nil {
			t.Fatal("expected a handshake error for a non-101 response")
		}
		if !strings.Contains(err.Error(), "handshake failed") {
			t.Errorf("error = %v, want handshake failed", err)
		}
		if resp == nil || resp.StatusCode != http.StatusOK {
			t.Errorf("resp = %v, want a 200 response returned", resp)
		}
	})

	t.Run("missing upgrade and connection headers", func(t *testing.T) {
		addr, _ := wsHandshakeServer(t, func(req *http.Request, conn net.Conn) {
			// 101 but no Upgrade/Connection tokens at all.
			writeRaw(t, conn, "HTTP/1.1 101 Switching Protocols\r\n\r\n")
		})
		_, _, err := Dial(context.Background(), "ws://"+addr+"/", nil)
		if err == nil {
			t.Fatal("expected a handshake error when Upgrade/Connection are absent")
		}
		if !strings.Contains(err.Error(), "handshake failed") {
			t.Errorf("error = %v, want handshake failed", err)
		}
	})

	t.Run("wrong accept key", func(t *testing.T) {
		addr, _ := wsHandshakeServer(t, func(req *http.Request, conn net.Conn) {
			writeRaw(t, conn, "HTTP/1.1 101 Switching Protocols\r\n"+
				"Upgrade: websocket\r\n"+
				"Connection: Upgrade\r\n"+
				"Sec-WebSocket-Accept: not-the-right-key\r\n\r\n")
		})
		_, _, err := Dial(context.Background(), "ws://"+addr+"/", nil)
		if err == nil {
			t.Fatal("expected an error for a wrong Sec-WebSocket-Accept")
		}
		if !strings.Contains(err.Error(), "invalid Sec-WebSocket-Accept") {
			t.Errorf("error = %v, want invalid Sec-WebSocket-Accept", err)
		}
	})
}

func TestWSDialWSSHandshakeFailsOverPlainServer(t *testing.T) {
	// A wss:// dial against a plain (non-TLS) listener: the TCP dial succeeds,
	// then the TLS handshake fails. This covers the "wss" scheme case and the
	// TLS handshake error branch in Dial.
	addr, _ := wsHandshakeServer(t, func(req *http.Request, conn net.Conn) {
		// Never reached for a plain server; included for completeness.
		_ = conn.Close()
	})
	_, _, err := Dial(context.Background(), "wss://"+addr+"/", nil)
	if err == nil {
		t.Fatal("expected a TLS handshake error against a plain server")
	}
	if !strings.Contains(err.Error(), "tls handshake") {
		t.Errorf("error = %v, want a tls handshake error", err)
	}
}

func TestWSDialDefaultPortBranch(t *testing.T) {
	cases := []struct {
		name string
		url  string // no explicit port, so Dial fills in the default
	}{
		// ws:// with no port -> :80 default branch.
		{"ws default port 80", "ws://127.0.0.1/__wsconn_no_such_path__"},
		// wss:// with no port -> :443 default branch.
		{"wss default port 443", "wss://127.0.0.1/__wsconn_no_such_path__"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The default-port branch under test runs before the dial; the dial
			// to the default port on loopback is expected to fail, which is
			// fine.
			_, _, err := Dial(context.Background(), tc.url, nil)
			if err == nil {
				t.Skip("unexpected listener on the default port; branch still executed")
			}
			if !strings.Contains(err.Error(), "dial") && !strings.Contains(err.Error(), "tls handshake") {
				t.Errorf("error = %v, want a dial or tls handshake error", err)
			}
		})
	}
}

func TestWSCloseErrorString(t *testing.T) {
	e := &CloseError{Code: 1011, Reason: "boom"}
	want := "wsconn: connection closed: 1011 boom"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestWSCloseIdempotentAndPeerObservesClose(t *testing.T) {
	a, b := tcpPair(t)

	errCh := make(chan error, 1)
	go func() {
		_, _, err := b.ReadMessage(context.Background())
		errCh <- err
	}()

	if err := a.Close(1000, "bye"); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	// Second Close is a no-op guarded by closeOnce and returns nil.
	if err := a.Close(1000, "again"); err != nil {
		t.Errorf("second Close = %v, want nil (no-op)", err)
	}

	select {
	case err := <-errCh:
		var ce *CloseError
		if !errors.As(err, &ce) {
			t.Fatalf("peer read error = %v, want *CloseError", err)
		}
		if ce.Code != 1000 || ce.Reason != "bye" {
			t.Errorf("close = %d %q, want 1000 bye", ce.Code, ce.Reason)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("peer ReadMessage did not observe the close")
	}
}

func TestWSWriteControlTruncatesPayload(t *testing.T) {
	a, b := tcpPair(t)

	// A control payload longer than 125 bytes must be truncated to 125.
	go func() {
		if err := a.writeControl(opPing, make([]byte, 300)); err != nil {
			t.Errorf("writeControl: %v", err)
		}
	}()

	_ = b.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	fin, op, payload, err := b.readFrame(defaultMaxMessage)
	if err != nil {
		t.Fatalf("readFrame: %v", err)
	}
	if !fin || op != opPing {
		t.Errorf("frame fin=%v op=0x%x, want fin op=ping", fin, op)
	}
	if len(payload) != 125 {
		t.Errorf("payload length = %d, want 125", len(payload))
	}
}

func TestWSParseClose(t *testing.T) {
	cases := []struct {
		name     string
		frame    []byte
		wantCode int
		wantMsg  string
	}{
		{"too short", []byte{0x03}, 1005, ""},
		{"empty", nil, 1005, ""},
		{"code only", []byte{0x03, 0xE8}, 1000, ""},
		{"code and reason", append([]byte{0x03, 0xF3}, []byte("done")...), 1011, "done"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, reason := parseClose(tc.frame)
			if code != tc.wantCode || reason != tc.wantMsg {
				t.Errorf("parseClose = %d %q, want %d %q", code, reason, tc.wantCode, tc.wantMsg)
			}
		})
	}
}

func TestWSWriteMessageInvalidType(t *testing.T) {
	a, _ := tcpPair(t)
	err := a.WriteMessage(context.Background(), MessageType(99), []byte("x"))
	if err == nil {
		t.Fatal("expected an error for an invalid message type")
	}
	if !strings.Contains(err.Error(), "invalid message type") {
		t.Errorf("error = %v, want invalid message type", err)
	}
}

func TestWSDialWithDeadlinePropagation(t *testing.T) {
	// A context deadline drives the handshake-deadline propagation branch in
	// Dial (SetDeadline before the handshake) and applyDeadline's deadline
	// branch on the first read.
	addr, connCh := wsHandshakeServer(t, func(req *http.Request, conn net.Conn) {
		key := req.Header.Get("Sec-WebSocket-Key")
		writeRaw(t, conn, "HTTP/1.1 101 Switching Protocols\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: keep-alive, Upgrade\r\n"+
			"Sec-WebSocket-Accept: "+serverAcceptKey(t, key)+"\r\n\r\n")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, _, err := Dial(ctx, "ws://"+addr+"/", nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(1000, "") })

	srv := <-connCh
	if srv == nil {
		t.Fatal("server connection not established")
	}
	peer := &Conn{conn: srv, br: bufio.NewReader(srv), maxMessage: defaultMaxMessage}

	go func() {
		if err := peer.WriteMessage(ctx, MessageBinary, []byte("xyz")); err != nil {
			t.Errorf("server write: %v", err)
		}
	}()
	mt, got, err := conn.ReadMessage(ctx)
	if err != nil {
		t.Fatalf("client read: %v", err)
	}
	if mt != MessageBinary || string(got) != "xyz" {
		t.Errorf("client got mt=%d %q", mt, got)
	}
}

func TestWSDialConnectionRefused(t *testing.T) {
	// Port 1 on loopback should refuse, exercising the dial error branch.
	_, _, err := Dial(context.Background(), "ws://127.0.0.1:1/", nil)
	if err == nil {
		t.Fatal("expected a dial error for a refused connection")
	}
	if !strings.Contains(err.Error(), "dial") {
		t.Errorf("error = %v, want a dial error", err)
	}
}

func TestWSDialResponseReadError(t *testing.T) {
	// Accept the connection, read the request, then close without writing a
	// valid HTTP response so http.ReadResponse fails.
	addr, _ := wsHandshakeServer(t, func(req *http.Request, conn net.Conn) {
		_ = conn.Close()
	})
	_, _, err := Dial(context.Background(), "ws://"+addr+"/", nil)
	if err == nil {
		t.Fatal("expected an error when the response cannot be read")
	}
	if !strings.Contains(err.Error(), "read handshake response") {
		t.Errorf("error = %v, want read handshake response", err)
	}
}

func TestWSDialConnectionTokenWithoutUpgradeToken(t *testing.T) {
	// 101 with an Upgrade header but a Connection header that does not contain
	// the "upgrade" token forces headerContainsToken to iterate and return
	// false, and exercises the handshake-failed branch.
	addr, _ := wsHandshakeServer(t, func(req *http.Request, conn net.Conn) {
		key := req.Header.Get("Sec-WebSocket-Key")
		writeRaw(t, conn, "HTTP/1.1 101 Switching Protocols\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: keep-alive, close\r\n"+
			"Sec-WebSocket-Accept: "+serverAcceptKey(t, key)+"\r\n\r\n")
	})
	_, _, err := Dial(context.Background(), "ws://"+addr+"/", nil)
	if err == nil {
		t.Fatal("expected a handshake error when Connection lacks the upgrade token")
	}
	if !strings.Contains(err.Error(), "handshake failed") {
		t.Errorf("error = %v, want handshake failed", err)
	}
}

func TestWSWriteMessageContextDeadlineTighter(t *testing.T) {
	a, b := tcpPair(t)
	// A context deadline sooner than writeWait selects the ctx deadline branch.
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(2*time.Second))
	defer cancel()

	go func() {
		if err := a.WriteMessage(ctx, MessageText, []byte("hi")); err != nil {
			t.Errorf("write: %v", err)
		}
	}()
	_, got, err := b.ReadMessage(context.Background())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "hi" {
		t.Errorf("got %q, want hi", got)
	}
}

func TestWSReadMessagePongThenData(t *testing.T) {
	a, b := tcpPair(t)
	go func() {
		// A pong frame must be silently skipped, then the data delivered.
		if err := a.writeFrame(opPong, []byte("p"), time.Time{}); err != nil {
			t.Errorf("write pong: %v", err)
		}
		if err := a.WriteMessage(context.Background(), MessageText, []byte("after-pong")); err != nil {
			t.Errorf("write text: %v", err)
		}
	}()
	_, got, err := b.ReadMessage(context.Background())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "after-pong" {
		t.Errorf("got %q, want after-pong", got)
	}
}

func TestWSReadMessageFragmentReassembly(t *testing.T) {
	a, b := tcpPair(t)
	go func() {
		// First fragment: text, FIN=0; continuation FIN=1.
		wsWriteFrameRaw(t, a, false, opText, []byte("foo"))
		wsWriteFrameRaw(t, a, true, opContinuation, []byte("bar"))
	}()
	mt, got, err := b.ReadMessage(context.Background())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if mt != MessageText || string(got) != "foobar" {
		t.Errorf("got mt=%d %q, want text foobar", mt, got)
	}
}

func TestWSReadMessageProtocolErrors(t *testing.T) {
	t.Run("new data frame mid-message", func(t *testing.T) {
		a, b := tcpPair(t)
		go func() {
			wsWriteFrameRaw(t, a, false, opText, []byte("a"))
			wsWriteFrameRaw(t, a, true, opText, []byte("b"))
		}()
		_, _, err := b.ReadMessage(context.Background())
		if err == nil || !strings.Contains(err.Error(), "unexpected new data frame") {
			t.Fatalf("err = %v, want unexpected new data frame", err)
		}
	})

	t.Run("continuation without start", func(t *testing.T) {
		a, b := tcpPair(t)
		go func() { wsWriteFrameRaw(t, a, true, opContinuation, []byte("x")) }()
		_, _, err := b.ReadMessage(context.Background())
		if err == nil || !strings.Contains(err.Error(), "continuation frame without start") {
			t.Fatalf("err = %v, want continuation frame without start", err)
		}
	})

	t.Run("unknown opcode", func(t *testing.T) {
		a, b := tcpPair(t)
		go func() { wsWriteFrameRaw(t, a, true, 0x3, []byte("x")) }()
		_, _, err := b.ReadMessage(context.Background())
		if err == nil || !strings.Contains(err.Error(), "unknown opcode") {
			t.Fatalf("err = %v, want unknown opcode", err)
		}
	})
}

func TestWSReadFrameReservedBits(t *testing.T) {
	a, b := tcpPair(t)
	// First header byte with a reserved bit (0x40) set, opcode text.
	go func() { _, _ = a.conn.Write([]byte{0x40 | 0x80 | opText, 0x00}) }()
	_, _, err := b.ReadMessage(context.Background())
	if err == nil || !strings.Contains(err.Error(), "reserved bits set") {
		t.Fatalf("err = %v, want reserved bits set", err)
	}
}

func TestWSReadFrameMaskedUnmasking(t *testing.T) {
	a, b := tcpPair(t)
	// A server should not mask, but readFrame must unmask if the MASK bit is
	// set. Send FIN+text, MASK bit set, len 3, a mask key, masked payload.
	go func() {
		mask := []byte{0x01, 0x02, 0x03, 0x04}
		plain := []byte("hey")
		hdr := append([]byte{0x80 | opText, 0x80 | byte(len(plain))}, mask...)
		masked := make([]byte, len(plain))
		for i := range plain {
			masked[i] = plain[i] ^ mask[i%4]
		}
		_, _ = a.conn.Write(hdr)
		_, _ = a.conn.Write(masked)
	}()
	mt, got, err := b.ReadMessage(context.Background())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if mt != MessageText || string(got) != "hey" {
		t.Errorf("got mt=%d %q, want text hey", mt, got)
	}
}

func TestWSReadFrameExtendedLengths(t *testing.T) {
	t.Run("16-bit length", func(t *testing.T) {
		a, b := tcpPair(t)
		payload := make([]byte, 300)
		for i := range payload {
			payload[i] = byte(i)
		}
		go func() {
			if err := a.WriteMessage(context.Background(), MessageBinary, payload); err != nil {
				t.Errorf("write: %v", err)
			}
		}()
		_, got, err := b.ReadMessage(context.Background())
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if len(got) != 300 {
			t.Errorf("len = %d, want 300", len(got))
		}
	})

	t.Run("64-bit length", func(t *testing.T) {
		a, b := tcpPair(t)
		payload := make([]byte, 70000)
		go func() {
			if err := a.WriteMessage(context.Background(), MessageBinary, payload); err != nil {
				t.Errorf("write: %v", err)
			}
		}()
		_, got, err := b.ReadMessage(context.Background())
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if len(got) != 70000 {
			t.Errorf("len = %d, want 70000", len(got))
		}
	})
}

func TestWSReadFrameTruncatedReads(t *testing.T) {
	cases := []struct {
		name  string
		bytes []byte
	}{
		// 126 (16-bit len marker) then EOF before the 2 extension bytes.
		{"16-bit ext eof", []byte{0x80 | opBinary, 126}},
		// 127 (64-bit len marker) then EOF before the 8 extension bytes.
		{"64-bit ext eof", []byte{0x80 | opBinary, 127}},
		// Declares 5 bytes of payload but sends none before EOF.
		{"payload eof", []byte{0x80 | opBinary, 0x05}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, b := tcpPair(t)
			go func() {
				_, _ = a.conn.Write(tc.bytes)
				_ = a.conn.Close()
			}()
			_, _, err := b.ReadMessage(context.Background())
			if err == nil {
				t.Fatal("expected an error from a truncated frame")
			}
		})
	}
}

func TestWSReadFrameMaskKeyEOF(t *testing.T) {
	a, b := tcpPair(t)
	// MASK bit set with a 1-byte payload, but EOF before the 4-byte mask key.
	go func() {
		_, _ = a.conn.Write([]byte{0x80 | opBinary, 0x81})
		_ = a.conn.Close()
	}()
	_, _, err := b.ReadMessage(context.Background())
	if err == nil {
		t.Fatal("expected an error when the mask key cannot be read")
	}
}

func TestWSWriteFrameWriteError(t *testing.T) {
	a, _ := tcpPair(t)
	// Closing the underlying conn makes the header write fail.
	if err := a.conn.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	err := a.writeFrame(opText, []byte("data"), time.Time{})
	if err == nil {
		t.Fatal("expected a write error on a closed connection")
	}
}

// errWrite is a sentinel returned by fakeConn writes that are configured to
// fail.
var errWrite = errors.New("fake write failure")

// fakeConn is an in-memory net.Conn whose Read yields a fixed byte slice (then
// blocks until closed) and whose Write fails after failAfter successful calls.
// It lets tests drive writeFrame and ReadMessage write-error branches without
// real sockets.
type fakeConn struct {
	readData  []byte
	pos       int
	writes    int
	failAfter int // number of Writes that succeed before failures begin
	closed    chan struct{}
	once      sync.Once
}

func newFakeConn(readData []byte, failAfter int) *fakeConn {
	return &fakeConn{readData: readData, failAfter: failAfter, closed: make(chan struct{})}
}

func (f *fakeConn) Read(p []byte) (int, error) {
	if f.pos < len(f.readData) {
		n := copy(p, f.readData[f.pos:])
		f.pos += n
		return n, nil
	}
	<-f.closed
	return 0, io.EOF
}

func (f *fakeConn) Write(p []byte) (int, error) {
	f.writes++
	if f.writes > f.failAfter {
		return 0, errWrite
	}
	return len(p), nil
}

func (f *fakeConn) Close() error {
	f.once.Do(func() { close(f.closed) })
	return nil
}

func (f *fakeConn) LocalAddr() net.Addr              { return fakeAddr{} }
func (f *fakeConn) RemoteAddr() net.Addr             { return fakeAddr{} }
func (f *fakeConn) SetDeadline(time.Time) error      { return nil }
func (f *fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (f *fakeConn) SetWriteDeadline(time.Time) error { return nil }

type fakeAddr struct{}

func (fakeAddr) Network() string { return "fake" }
func (fakeAddr) String() string  { return "fake" }

func TestWSWriteFramePayloadWriteError(t *testing.T) {
	// The header write succeeds (failAfter=1) but the payload write fails,
	// covering the payload-write error branch of writeFrame.
	fc := newFakeConn(nil, 1)
	c := &Conn{conn: fc, br: bufio.NewReader(fc), maxMessage: defaultMaxMessage}
	err := c.writeFrame(opBinary, []byte("payload"), time.Time{})
	if !errors.Is(err, errWrite) {
		t.Fatalf("err = %v, want errWrite", err)
	}
}

func TestWSReadMessagePingPongWriteError(t *testing.T) {
	// A ping frame is read successfully, but echoing the pong fails because the
	// fake conn rejects all writes. This covers the ping write-error branch in
	// ReadMessage.
	ping := []byte{0x80 | opPing, 0x02, 'h', 'b'}
	fc := newFakeConn(ping, 0)
	c := &Conn{conn: fc, br: bufio.NewReader(fc), maxMessage: defaultMaxMessage}
	t.Cleanup(func() { _ = fc.Close() })

	_, _, err := c.ReadMessage(context.Background())
	if !errors.Is(err, errWrite) {
		t.Fatalf("err = %v, want errWrite from the failed pong echo", err)
	}
}

func TestWSReadMessageExceedsBudget(t *testing.T) {
	a, b := tcpPair(t)
	b.SetMaxMessage(8)

	// Writer sends a data frame larger than the reader's budget.
	go func() {
		if err := a.WriteMessage(context.Background(), MessageBinary, make([]byte, 64)); err != nil {
			t.Errorf("write: %v", err)
		}
	}()

	_, _, err := b.ReadMessage(context.Background())
	if err == nil {
		t.Fatal("expected an error when a frame exceeds the budget")
	}
	if !strings.Contains(err.Error(), "exceeds remaining") || !strings.Contains(err.Error(), "budget") {
		t.Errorf("error = %v, want an exceeds remaining ... budget error", err)
	}
}
