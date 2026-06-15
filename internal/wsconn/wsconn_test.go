package wsconn

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"time"
)

func tcpPair(t *testing.T) (*Conn, *Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	type res struct {
		c   net.Conn
		err error
	}
	ch := make(chan res, 1)
	go func() {
		c, err := ln.Accept()
		ch <- res{c, err}
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	r := <-ch
	if r.err != nil {
		t.Fatalf("accept: %v", r.err)
	}

	a := &Conn{conn: client, br: bufio.NewReader(client), maxMessage: defaultMaxMessage}
	b := &Conn{conn: r.c, br: bufio.NewReader(r.c), maxMessage: defaultMaxMessage}
	t.Cleanup(func() { client.Close(); r.c.Close() })
	return a, b
}

func TestAcceptKeyVector(t *testing.T) {
	// RFC 6455 §1.3 worked example.
	if got := acceptKey("dGhlIHNhbXBsZSBub25jZQ=="); got != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Errorf("acceptKey = %q", got)
	}
}

func TestMessageRoundTrip(t *testing.T) {
	a, b := tcpPair(t)
	ctx := context.Background()

	cases := []struct {
		name    string
		mt      MessageType
		payload []byte
	}{
		{"small text", MessageText, []byte("hello, gateway")},
		{"empty", MessageText, []byte("")},
		{"medium (16-bit len)", MessageBinary, bytes.Repeat([]byte{0xAB}, 200)},
		{"large (64-bit len)", MessageBinary, bytes.Repeat([]byte{0xCD}, 70000)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			go func() {
				if err := a.WriteMessage(ctx, tc.mt, tc.payload); err != nil {
					t.Errorf("write: %v", err)
				}
			}()
			mt, got, err := b.ReadMessage(ctx)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if mt != tc.mt {
				t.Errorf("type = %d, want %d", mt, tc.mt)
			}
			if !bytes.Equal(got, tc.payload) {
				t.Errorf("payload mismatch (len got %d, want %d)", len(got), len(tc.payload))
			}
		})
	}
}

func TestPingAutoPong(t *testing.T) {
	a, b := tcpPair(t)
	ctx := context.Background()

	go func() {
		if err := a.writeFrame(opPing, []byte("hb"), time.Time{}); err != nil {
			t.Errorf("write ping: %v", err)
		}
		if err := a.WriteMessage(ctx, MessageText, []byte("after-ping")); err != nil {
			t.Errorf("write text: %v", err)
		}
	}()

	// b should silently answer the ping with a pong and then deliver the text.
	_, got, err := b.ReadMessage(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "after-ping" {
		t.Errorf("got %q", got)
	}

	// a should have received a pong carrying the ping payload.
	_ = a.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, op, payload, err := a.readFrame(defaultMaxMessage)
	if err != nil {
		t.Fatalf("read pong: %v", err)
	}
	if op != opPong || string(payload) != "hb" {
		t.Errorf("expected pong 'hb', got op=0x%x payload=%q", op, payload)
	}
}

func TestReadClose(t *testing.T) {
	a, b := tcpPair(t)
	ctx := context.Background()

	go func() {
		payload := make([]byte, 2+len("bye"))
		binary.BigEndian.PutUint16(payload[:2], 1000)
		copy(payload[2:], "bye")
		_ = a.writeFrame(opClose, payload, time.Time{})
	}()

	_, _, err := b.ReadMessage(ctx)
	var ce *CloseError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *CloseError, got %v", err)
	}
	if ce.Code != 1000 || ce.Reason != "bye" {
		t.Errorf("close = %d %q, want 1000 bye", ce.Code, ce.Reason)
	}
}

func TestReadContextCancel(t *testing.T) {
	_, b := tcpPair(t)
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		_, _, err := b.ReadMessage(ctx)
		errCh <- err
	}()

	cancel()
	select {
	case err := <-errCh:
		if err == nil {
			t.Error("expected an error after cancel")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ReadMessage did not return after context cancel")
	}
}

func TestRejectFragmentedControlFrame(t *testing.T) {
	a, b := tcpPair(t)
	// FIN=0 (fragmented), opcode=ping, unmasked, length 0 — invalid per RFC 6455.
	go func() { _, _ = a.conn.Write([]byte{0x09, 0x00}) }()
	if _, _, err := b.ReadMessage(context.Background()); err == nil {
		t.Fatal("expected an error for a fragmented control frame")
	}
}

func TestRejectOversizedControlFrame(t *testing.T) {
	a, b := tcpPair(t)
	// FIN=1, opcode=ping, length=200 via the 16-bit form — control frames may
	// not exceed 125 bytes.
	go func() {
		_, _ = a.conn.Write([]byte{0x89, 0x7E, 0x00, 0xC8})
		_, _ = a.conn.Write(make([]byte, 200))
	}()
	if _, _, err := b.ReadMessage(context.Background()); err == nil {
		t.Fatal("expected an error for an oversized control frame")
	}
}
