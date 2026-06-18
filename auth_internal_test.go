package warmbly

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAuthnAuthenticatorFunc(t *testing.T) {
	t.Run("sets header and returns nil", func(t *testing.T) {
		var called bool
		var auth Authenticator = AuthenticatorFunc(func(req *http.Request) error {
			called = true
			req.Header.Set("Authorization", "Bearer from-func")
			return nil
		})

		req, err := http.NewRequest(http.MethodGet, "https://example.test", nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		if err := auth.authenticate(req); err != nil {
			t.Fatalf("authenticate: %v", err)
		}
		if !called {
			t.Error("wrapped function was not called")
		}
		if got := req.Header.Get("Authorization"); got != "Bearer from-func" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer from-func")
		}
	})

	t.Run("propagates the wrapped error", func(t *testing.T) {
		sentinel := errors.New("func boom")
		var auth Authenticator = AuthenticatorFunc(func(req *http.Request) error {
			return sentinel
		})

		req, err := http.NewRequest(http.MethodGet, "https://example.test", nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		if err := auth.authenticate(req); !errors.Is(err, sentinel) {
			t.Errorf("authenticate error = %v, want %v", err, sentinel)
		}
	})
}

func TestAuthnAPIKeyAuth(t *testing.T) {
	t.Run("direct authenticate sets bearer header", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "https://example.test", nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		if err := (apiKeyAuth{key: "wmbly_direct"}).authenticate(req); err != nil {
			t.Fatalf("authenticate: %v", err)
		}
		if got := req.Header.Get("Authorization"); got != "Bearer wmbly_direct" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer wmbly_direct")
		}
	})

	t.Run("end-to-end header reaches server", func(t *testing.T) {
		var gotAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"id":"camp_1","name":"x"}`))
		}))
		t.Cleanup(srv.Close)

		c, err := New(
			WithAPIKey("wmbly_e2e"),
			WithBaseURL(srv.URL+"/v1/"),
		)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if _, _, err := c.Campaigns.Get(context.Background(), "camp_1"); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if gotAuth != "Bearer wmbly_e2e" {
			t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer wmbly_e2e")
		}
	})
}

func TestAuthnAccessTokenAuth(t *testing.T) {
	t.Run("direct authenticate sets bearer header", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "https://example.test", nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		if err := (accessTokenAuth{token: "wmblyo_direct"}).authenticate(req); err != nil {
			t.Fatalf("authenticate: %v", err)
		}
		if got := req.Header.Get("Authorization"); got != "Bearer wmblyo_direct" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer wmblyo_direct")
		}
	})

	t.Run("end-to-end header reaches server", func(t *testing.T) {
		var gotAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"id":"camp_1","name":"x"}`))
		}))
		t.Cleanup(srv.Close)

		c, err := New(
			WithAccessToken("wmblyo_e2e"),
			WithBaseURL(srv.URL+"/v1/"),
		)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if _, _, err := c.Campaigns.Get(context.Background(), "camp_1"); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if gotAuth != "Bearer wmblyo_e2e" {
			t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer wmblyo_e2e")
		}
	})
}

// TestAuthnErrTokenSource is a TokenSource whose Token method always fails. The
// name carries the mandated TestAuthn prefix; it is a type, not a test, so the
// go test runner ignores it.
type TestAuthnErrTokenSource struct{ err error }

func (s TestAuthnErrTokenSource) Token() (*Token, error) { return nil, s.err }

func TestAuthnTokenSourceAuth(t *testing.T) {
	t.Run("success yields scheme and token", func(t *testing.T) {
		var gotAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"id":"camp_1","name":"x"}`))
		}))
		t.Cleanup(srv.Close)

		c, err := New(
			WithTokenSource(StaticTokenSource(&Token{AccessToken: "t", TokenType: "Bearer"})),
			WithBaseURL(srv.URL+"/v1/"),
		)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if _, _, err := c.Campaigns.Get(context.Background(), "camp_1"); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if gotAuth != "Bearer t" {
			t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer t")
		}
	})

	t.Run("direct authenticate uses token Type for the scheme", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "https://example.test", nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		auth := tokenSourceAuth{src: StaticTokenSource(&Token{AccessToken: "mac-tok", TokenType: "MAC"})}
		if err := auth.authenticate(req); err != nil {
			t.Fatalf("authenticate: %v", err)
		}
		if got := req.Header.Get("Authorization"); got != "MAC mac-tok" {
			t.Errorf("Authorization = %q, want %q", got, "MAC mac-tok")
		}
	})

	t.Run("error from source surfaces through do", func(t *testing.T) {
		var hits int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			_, _ = w.Write([]byte(`{"id":"camp_1","name":"x"}`))
		}))
		t.Cleanup(srv.Close)

		sentinel := errors.New("token source down")
		c, err := New(
			WithTokenSource(TestAuthnErrTokenSource{err: sentinel}),
			WithBaseURL(srv.URL+"/v1/"),
			WithMaxRetries(3),
			WithRetryWaitBounds(time.Millisecond, 5*time.Millisecond),
		)
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		_, _, err = c.Campaigns.Get(context.Background(), "camp_1")
		if err == nil {
			t.Fatal("expected an error from a failing token source")
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("errors.Is(err, sentinel) = false; err = %v", err)
		}
		if !strings.Contains(err.Error(), "authenticate request") {
			t.Errorf("error message %q does not wrap %q", err.Error(), "authenticate request")
		}
		if hits != 0 {
			t.Errorf("server was hit %d times; authentication must fail before any request", hits)
		}
	})

	t.Run("direct authenticate propagates the source error", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "https://example.test", nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		sentinel := errors.New("nope")
		auth := tokenSourceAuth{src: TestAuthnErrTokenSource{err: sentinel}}
		if err := auth.authenticate(req); !errors.Is(err, sentinel) {
			t.Errorf("authenticate error = %v, want %v", err, sentinel)
		}
		if got := req.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization should be unset on error, got %q", got)
		}
	})
}
