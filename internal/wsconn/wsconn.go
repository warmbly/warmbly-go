// Package wsconn is a minimal, dependency-free RFC 6455 WebSocket client.
//
// It implements only what the warmbly gateway needs: a client-side handshake
// over ws:// or wss://, reading and writing text/binary messages, automatic
// responses to ping and close control frames, and context-aware deadlines.
// It is intentionally not a general-purpose WebSocket library.
package wsconn

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// MessageType identifies the kind of a data message.
type MessageType int

const (
	// MessageText is a UTF-8 text message (opcode 0x1).
	MessageText MessageType = 1
	// MessageBinary is a binary message (opcode 0x2).
	MessageBinary MessageType = 2
)

// WebSocket frame opcodes (RFC 6455 §5.2).
const (
	opContinuation = 0x0
	opText         = 0x1
	opBinary       = 0x2
	opClose        = 0x8
	opPing         = 0x9
	opPong         = 0xA
)

// guid is the RFC 6455 magic value used to compute Sec-WebSocket-Accept.
const guid = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// defaultMaxMessage caps the size of a reassembled message to guard memory.
const defaultMaxMessage = 8 << 20 // 8 MiB

// CloseError reports that the peer sent a close frame.
type CloseError struct {
	Code   int
	Reason string
}

func (e *CloseError) Error() string {
	return fmt.Sprintf("wsconn: connection closed: %d %s", e.Code, e.Reason)
}

// Conn is a client WebSocket connection. A Conn supports one concurrent reader
// and one concurrent writer; writes are serialised internally so the caller may
// write from a separate goroutine than the one reading.
type Conn struct {
	conn net.Conn
	br   *bufio.Reader

	writeMu    sync.Mutex
	maxMessage int64

	closeOnce sync.Once
}

// Dial performs the opening handshake against urlStr (ws:// or wss://) and
// returns an established connection. Extra request headers (for example
// Authorization) may be supplied; Host and the WebSocket handshake headers are
// managed automatically. The handshake honours ctx's deadline and cancellation.
func Dial(ctx context.Context, urlStr string, header http.Header) (*Conn, *http.Response, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, nil, fmt.Errorf("wsconn: parse url: %w", err)
	}

	var useTLS bool
	switch strings.ToLower(u.Scheme) {
	case "ws":
		useTLS = false
	case "wss":
		useTLS = true
	default:
		return nil, nil, fmt.Errorf("wsconn: unsupported scheme %q", u.Scheme)
	}

	hostPort := u.Host
	if u.Port() == "" {
		if useTLS {
			hostPort = net.JoinHostPort(u.Hostname(), "443")
		} else {
			hostPort = net.JoinHostPort(u.Hostname(), "80")
		}
	}

	dialer := &net.Dialer{}
	rawConn, err := dialer.DialContext(ctx, "tcp", hostPort)
	if err != nil {
		return nil, nil, fmt.Errorf("wsconn: dial: %w", err)
	}

	conn := rawConn
	if useTLS {
		tlsConn := tls.Client(rawConn, &tls.Config{ServerName: u.Hostname()})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			rawConn.Close()
			return nil, nil, fmt.Errorf("wsconn: tls handshake: %w", err)
		}
		conn = tlsConn
	}

	// Propagate any handshake deadline to the raw connection.
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}

	keyRaw := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, keyRaw); err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("wsconn: read random key: %w", err)
	}
	key := base64.StdEncoding.EncodeToString(keyRaw)

	req := buildHandshake(u, key, header)
	if err := req.Write(conn); err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("wsconn: write handshake: %w", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("wsconn: read handshake response: %w", err)
	}

	if resp.StatusCode != http.StatusSwitchingProtocols ||
		!strings.EqualFold(resp.Header.Get("Upgrade"), "websocket") ||
		!headerContainsToken(resp.Header, "Connection", "upgrade") {
		resp.Body.Close()
		conn.Close()
		return nil, resp, fmt.Errorf("wsconn: handshake failed: status %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Sec-WebSocket-Accept"); got != acceptKey(key) {
		resp.Body.Close()
		conn.Close()
		return nil, resp, errors.New("wsconn: invalid Sec-WebSocket-Accept")
	}

	// Clear the handshake deadline; per-call deadlines take over.
	_ = conn.SetDeadline(time.Time{})

	return &Conn{conn: conn, br: br, maxMessage: defaultMaxMessage}, resp, nil
}

func buildHandshake(u *url.URL, key string, header http.Header) *http.Request {
	reqURL := *u
	switch strings.ToLower(u.Scheme) {
	case "wss":
		reqURL.Scheme = "https"
	default:
		reqURL.Scheme = "http"
	}
	req := &http.Request{
		Method:     http.MethodGet,
		URL:        &reqURL,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Host:       u.Host,
		Header:     make(http.Header),
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", key)
	req.Header.Set("Sec-WebSocket-Version", "13")
	return req
}

func acceptKey(key string) string {
	h := sha1.New()
	_, _ = io.WriteString(h, key+guid)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func headerContainsToken(h http.Header, name, token string) bool {
	for _, v := range h.Values(name) {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

// SetMaxMessage sets the maximum reassembled message size in bytes.
func (c *Conn) SetMaxMessage(n int64) {
	if n > 0 {
		c.maxMessage = n
	}
}

// ReadMessage reads the next data message, transparently reassembling
// fragments and answering ping and close control frames. It returns a
// [*CloseError] when the peer initiates a close. The deadline and cancellation
// of ctx interrupt a blocked read.
func (c *Conn) ReadMessage(ctx context.Context) (MessageType, []byte, error) {
	stop := c.applyDeadline(ctx, c.conn.SetReadDeadline)
	defer stop()

	var (
		payload  []byte
		dataType MessageType
		started  bool
	)
	for {
		fin, opcode, frame, err := c.readFrame()
		if err != nil {
			return 0, nil, err
		}

		switch opcode {
		case opPing:
			if err := c.writeControl(opPong, frame); err != nil {
				return 0, nil, err
			}
			continue
		case opPong:
			continue
		case opClose:
			code, reason := parseClose(frame)
			_ = c.writeControl(opClose, frame) // echo the close
			return 0, nil, &CloseError{Code: code, Reason: reason}
		case opText, opBinary:
			if started {
				return 0, nil, errors.New("wsconn: unexpected new data frame mid-message")
			}
			started = true
			if opcode == opText {
				dataType = MessageText
			} else {
				dataType = MessageBinary
			}
		case opContinuation:
			if !started {
				return 0, nil, errors.New("wsconn: continuation frame without start")
			}
		default:
			return 0, nil, fmt.Errorf("wsconn: unknown opcode 0x%x", opcode)
		}

		if int64(len(payload))+int64(len(frame)) > c.maxMessage {
			return 0, nil, fmt.Errorf("wsconn: message exceeds %d bytes", c.maxMessage)
		}
		payload = append(payload, frame...)

		if fin {
			return dataType, payload, nil
		}
	}
}

// readFrame reads a single frame and returns its FIN bit, opcode and unmasked
// payload. Server-to-client frames must not be masked.
func (c *Conn) readFrame() (fin bool, opcode byte, payload []byte, err error) {
	var hdr [2]byte
	if _, err = io.ReadFull(c.br, hdr[:]); err != nil {
		return
	}
	fin = hdr[0]&0x80 != 0
	if hdr[0]&0x70 != 0 {
		return false, 0, nil, errors.New("wsconn: reserved bits set")
	}
	opcode = hdr[0] & 0x0F
	masked := hdr[1]&0x80 != 0
	length := int64(hdr[1] & 0x7F)

	switch length {
	case 126:
		var ext [2]byte
		if _, err = io.ReadFull(c.br, ext[:]); err != nil {
			return
		}
		length = int64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err = io.ReadFull(c.br, ext[:]); err != nil {
			return
		}
		length = int64(binary.BigEndian.Uint64(ext[:]))
	}

	if length < 0 || length > c.maxMessage {
		return false, 0, nil, fmt.Errorf("wsconn: frame length %d out of bounds", length)
	}

	var maskKey [4]byte
	if masked {
		// A compliant server never masks; tolerate but unmask anyway.
		if _, err = io.ReadFull(c.br, maskKey[:]); err != nil {
			return
		}
	}

	payload = make([]byte, length)
	if _, err = io.ReadFull(c.br, payload); err != nil {
		return
	}
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}
	return fin, opcode, payload, nil
}

// WriteMessage writes data as a single (unfragmented) masked message. Writes
// are serialised, so it is safe to call concurrently with [Conn.ReadMessage].
func (c *Conn) WriteMessage(ctx context.Context, mt MessageType, data []byte) error {
	var opcode byte
	switch mt {
	case MessageText:
		opcode = opText
	case MessageBinary:
		opcode = opBinary
	default:
		return fmt.Errorf("wsconn: invalid message type %d", mt)
	}
	stop := c.applyDeadline(ctx, c.conn.SetWriteDeadline)
	defer stop()
	return c.writeFrame(opcode, data)
}

func (c *Conn) writeControl(opcode byte, payload []byte) error {
	if len(payload) > 125 {
		payload = payload[:125]
	}
	return c.writeFrame(opcode, payload)
}

// writeFrame writes one complete, client-masked frame with FIN set.
func (c *Conn) writeFrame(opcode byte, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	var hdr []byte
	b0 := byte(0x80) | opcode // FIN + opcode
	hdr = append(hdr, b0)

	n := len(payload)
	switch {
	case n < 126:
		hdr = append(hdr, byte(0x80)|byte(n)) // MASK bit + length
	case n < 65536:
		hdr = append(hdr, byte(0x80)|126)
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], uint16(n))
		hdr = append(hdr, ext[:]...)
	default:
		hdr = append(hdr, byte(0x80)|127)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(n))
		hdr = append(hdr, ext[:]...)
	}

	var maskKey [4]byte
	if _, err := io.ReadFull(rand.Reader, maskKey[:]); err != nil {
		return fmt.Errorf("wsconn: read mask key: %w", err)
	}
	hdr = append(hdr, maskKey[:]...)

	masked := make([]byte, n)
	for i := 0; i < n; i++ {
		masked[i] = payload[i] ^ maskKey[i%4]
	}

	if _, err := c.conn.Write(hdr); err != nil {
		return err
	}
	if n > 0 {
		if _, err := c.conn.Write(masked); err != nil {
			return err
		}
	}
	return nil
}

func parseClose(frame []byte) (int, string) {
	if len(frame) < 2 {
		return 1005, "" // 1005: no status received
	}
	code := int(binary.BigEndian.Uint16(frame[:2]))
	return code, string(frame[2:])
}

// Close sends a close frame with the given status code and reason, then closes
// the underlying connection. Subsequent calls are no-ops.
func (c *Conn) Close(code int, reason string) error {
	var err error
	c.closeOnce.Do(func() {
		payload := make([]byte, 2+len(reason))
		binary.BigEndian.PutUint16(payload[:2], uint16(code))
		copy(payload[2:], reason)
		_ = c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_ = c.writeControl(opClose, payload)
		err = c.conn.Close()
	})
	return err
}

// applyDeadline wires ctx's deadline and cancellation to the connection by
// setting deadlines via set. The returned stop function clears the deadline and
// releases the watcher goroutine.
func (c *Conn) applyDeadline(ctx context.Context, set func(time.Time) error) func() {
	if dl, ok := ctx.Deadline(); ok {
		_ = set(dl)
	}
	if ctx.Done() == nil {
		return func() { _ = set(time.Time{}) }
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			// Force the in-flight read/write to fail promptly.
			_ = set(time.Unix(0, 0))
		case <-done:
		}
	}()
	return func() {
		close(done)
		_ = set(time.Time{})
	}
}
