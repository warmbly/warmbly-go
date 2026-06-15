package warmbly

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestAuthCodeURL(t *testing.T) {
	cfg := &OAuth2Config{
		ClientID:    "client_123",
		RedirectURL: "https://app.example.com/cb",
		Scopes:      []string{"campaigns:read", "contacts:read"},
		Endpoint:    OAuth2Endpoint{AuthURL: "https://auth.example.com/oauth/authorize"},
	}
	verifier := GenerateVerifier()
	raw := cfg.AuthCodeURL("state-xyz", S256ChallengeOption(verifier))

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := u.Query()
	checks := map[string]string{
		"response_type":         "code",
		"client_id":             "client_123",
		"redirect_uri":          "https://app.example.com/cb",
		"scope":                 "campaigns:read contacts:read",
		"state":                 "state-xyz",
		"code_challenge_method": "S256",
		"code_challenge":        S256ChallengeFromVerifier(verifier),
	}
	for k, want := range checks {
		if got := q.Get(k); got != want {
			t.Errorf("query %s = %q, want %q", k, got, want)
		}
	}
}

func TestExchange(t *testing.T) {
	fixed := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	old := timeNow
	timeNow = func() time.Time { return fixed }
	defer func() { timeNow = old }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.Form.Get("grant_type") != "authorization_code" {
			t.Errorf("grant_type = %q", r.Form.Get("grant_type"))
		}
		if r.Form.Get("code") != "auth-code" {
			t.Errorf("code = %q", r.Form.Get("code"))
		}
		if r.Form.Get("code_verifier") != "verifier-123" {
			t.Errorf("code_verifier = %q", r.Form.Get("code_verifier"))
		}
		id, secret, ok := r.BasicAuth()
		if !ok || id != "client_123" || secret != "shh" {
			t.Errorf("basic auth = %q/%q ok=%v", id, secret, ok)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"wmblyo_abc","token_type":"Bearer","expires_in":3600,"refresh_token":"rt_1","scope":"campaigns:read"}`))
	}))
	defer srv.Close()

	cfg := &OAuth2Config{
		ClientID:     "client_123",
		ClientSecret: "shh",
		RedirectURL:  "https://app.example.com/cb",
		Endpoint:     OAuth2Endpoint{TokenURL: srv.URL},
	}
	tok, err := cfg.Exchange(context.Background(), "auth-code", VerifierOption("verifier-123"))
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if tok.AccessToken != "wmblyo_abc" || tok.RefreshToken != "rt_1" {
		t.Errorf("unexpected token: %+v", tok)
	}
	if want := fixed.Add(3600 * time.Second); !tok.Expiry.Equal(want) {
		t.Errorf("Expiry = %v, want %v", tok.Expiry, want)
	}
}

func TestClientCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "client_credentials" {
			t.Errorf("grant_type = %q", r.Form.Get("grant_type"))
		}
		if r.Form.Get("scope") != "campaigns:read" {
			t.Errorf("scope = %q", r.Form.Get("scope"))
		}
		_, _ = w.Write([]byte(`{"access_token":"wmblyo_cc","token_type":"Bearer","expires_in":7200}`))
	}))
	defer srv.Close()

	cfg := &ClientCredentialsConfig{
		ClientID:     "client_123",
		ClientSecret: "shh",
		Scopes:       []string{"campaigns:read"},
		TokenURL:     srv.URL,
	}
	tok, err := cfg.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok.AccessToken != "wmblyo_cc" {
		t.Errorf("access token = %q", tok.AccessToken)
	}
}

func TestRefreshingTokenSource(t *testing.T) {
	fixed := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	old := timeNow
	timeNow = func() time.Time { return fixed }
	defer func() { timeNow = old }()

	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "rt_old" {
			t.Errorf("unexpected refresh request: %v", r.Form)
		}
		_, _ = w.Write([]byte(`{"access_token":"wmblyo_new","token_type":"Bearer","expires_in":3600}`))
	}))
	defer srv.Close()

	cfg := &OAuth2Config{
		ClientID: "client_123",
		Endpoint: OAuth2Endpoint{TokenURL: srv.URL},
	}
	expired := &Token{AccessToken: "wmblyo_old", RefreshToken: "rt_old", Expiry: fixed.Add(-time.Hour)}
	ts := cfg.TokenSource(context.Background(), expired)

	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok.AccessToken != "wmblyo_new" {
		t.Errorf("access token = %q, want wmblyo_new", tok.AccessToken)
	}
	// The new token carried no refresh_token, so the old one is preserved.
	if tok.RefreshToken != "rt_old" {
		t.Errorf("refresh token = %q, want rt_old", tok.RefreshToken)
	}
	// A second call should reuse the cached (still-valid) token, not refresh.
	if _, err := ts.Token(); err != nil {
		t.Fatalf("second Token: %v", err)
	}
	if calls != 1 {
		t.Errorf("token endpoint called %d times, want 1", calls)
	}
}

func TestExchangeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"code expired"}`))
	}))
	defer srv.Close()

	cfg := &OAuth2Config{ClientID: "c", Endpoint: OAuth2Endpoint{TokenURL: srv.URL}}
	_, err := cfg.Exchange(context.Background(), "bad")
	if err == nil {
		t.Fatal("expected error")
	}
	var oerr *OAuth2Error
	if !errors.As(err, &oerr) {
		t.Fatalf("expected *OAuth2Error, got %T: %v", err, err)
	}
	if oerr.Code != "invalid_grant" || oerr.StatusCode != 400 {
		t.Errorf("unexpected oauth error: %+v", oerr)
	}
}

// TestExchangeStringExpiresIn ensures a quoted-string expires_in (which some
// servers emit) is tolerated instead of breaking the whole token parse.
func TestExchangeStringExpiresIn(t *testing.T) {
	fixed := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	old := timeNow
	timeNow = func() time.Time { return fixed }
	defer func() { timeNow = old }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"wmblyo_x","token_type":"Bearer","expires_in":"3600"}`))
	}))
	defer srv.Close()

	cfg := &OAuth2Config{ClientID: "c", Endpoint: OAuth2Endpoint{TokenURL: srv.URL}}
	tok, err := cfg.Exchange(context.Background(), "code")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if tok.AccessToken != "wmblyo_x" {
		t.Errorf("access token = %q", tok.AccessToken)
	}
	if want := fixed.Add(3600 * time.Second); !tok.Expiry.Equal(want) {
		t.Errorf("Expiry = %v, want %v", tok.Expiry, want)
	}
}
