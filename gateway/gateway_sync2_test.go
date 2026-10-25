package gateway

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"
)

// refusingServer answers every upgrade with HTTP 403 and body.
func refusingServer(t *testing.T, body string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				if _, err := http.ReadRequest(bufio.NewReader(conn)); err != nil {
					return
				}
				fmt.Fprintf(conn, "HTTP/1.1 403 Forbidden\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
			}()
		}
	}()
	return "ws://" + ln.Addr().String() + "/socket/websocket"
}

func TestHandshakeRefusalReasonIsRead(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		wantCode  int
		wantWrap  error
		wantRetry time.Duration
	}{
		{"ticket reused", `{"code":4004,"reason":"This connection token was already used; mint a new one for each connection"}`, JoinCodeAuthFailed, ErrUnauthorized, 0},
		{"credential in the url", `{"code":4003,"reason":"Send API keys and OAuth tokens in the x-warmbly-token header, not the URL"}`, JoinCodeUnauthenticated, ErrUnauthorized, 0},
		{"ip not allowed", `{"code":4010,"reason":"IP address not allowed"}`, JoinCodePermissionDenied, ErrForbidden, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := New("ticket", "org_1", WithURL(refusingServer(t, tc.body)))
			err := c.Open(testContext(t))
			var je *JoinError
			if !errors.As(err, &je) {
				t.Fatalf("Open error = %v, want a *JoinError", err)
			}
			if je.Code != tc.wantCode || je.Reason == "" || je.Topic != "" {
				t.Errorf("JoinError = %+v", je)
			}
			if !errors.Is(err, tc.wantWrap) {
				t.Errorf("error %v does not match %v", err, tc.wantWrap)
			}
		})
	}
}

func TestHandshakeRefusalRateLimitQuotesItsWait(t *testing.T) {
	err := handshakeErrorFor(t, `{"code":4007,"reason":"Rate limited","retry_after_ms":12000}`)
	var je *JoinError
	if !errors.As(err, &je) {
		t.Fatalf("error = %v, want a *JoinError", err)
	}
	if je.Code != JoinCodeRateLimited || je.RetryAfter != 12*time.Second || je.Permanent() {
		t.Errorf("JoinError = %+v", je)
	}
}

func TestHandshakeRefusalWithoutBodyKeepsTheStatusFallback(t *testing.T) {
	c := New("ticket", "org_1", WithURL(refusingServer(t, "")))
	if err := c.Open(testContext(t)); !errors.Is(err, ErrForbidden) {
		t.Fatalf("Open error = %v, want ErrForbidden", err)
	}
	if err := handshakeErrorFor(t, `<html>forbidden</html>`); !errors.Is(err, ErrForbidden) {
		t.Fatalf("error = %v, want ErrForbidden for a body that is not the refusal JSON", err)
	}
}

// handshakeErrorFor dials a server that refuses with body and returns the
// classification of the refusal.
func handshakeErrorFor(t *testing.T, body string) error {
	t.Helper()
	c := New("ticket", "org_1", WithURL(refusingServer(t, body)))
	_, err := c.runOnce(testContext(t))
	return err
}

func TestSyncConstants(t *testing.T) {
	if LandedRescueMoved != "rescue_moved" || LandedRescueAlreadyOut != "rescue_already_out" ||
		LandedRescueGone != "rescue_gone" || LandedRescueFailed != "rescue_failed" {
		t.Error("rescue outcome constants drifted from the wire values")
	}
	if JoinReasonAdminMFARequired != "admin_mfa_required" || JoinReasonJWTRequired != "jwt_required" {
		t.Error("admin join reasons drifted from the wire values")
	}
}
