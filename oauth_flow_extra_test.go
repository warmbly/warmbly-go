package warmbly

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOAuthFlowRevoke(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var gotMethod, gotToken, gotCT string
		var gotID, gotSecret string
		var gotOK bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotCT = r.Header.Get("Content-Type")
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse form: %v", err)
			}
			gotToken = r.Form.Get("token")
			gotID, gotSecret, gotOK = r.BasicAuth()
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		cfg := &OAuth2Config{
			ClientID:     "client_123",
			ClientSecret: "shh",
			Endpoint:     OAuth2Endpoint{RevokeURL: srv.URL},
		}
		if err := cfg.Revoke(context.Background(), "tok_to_revoke"); err != nil {
			t.Fatalf("Revoke: %v", err)
		}
		if gotMethod != http.MethodPost {
			t.Errorf("method = %q, want POST", gotMethod)
		}
		if gotToken != "tok_to_revoke" {
			t.Errorf("token form value = %q", gotToken)
		}
		if gotCT != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q", gotCT)
		}
		if !gotOK || gotID != "client_123" || gotSecret != "shh" {
			t.Errorf("basic auth = %q/%q ok=%v", gotID, gotSecret, gotOK)
		}
	})

	t.Run("error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_token","error_description":"unknown token"}`))
		}))
		t.Cleanup(srv.Close)

		cfg := &OAuth2Config{
			ClientID:     "client_123",
			ClientSecret: "shh",
			Endpoint:     OAuth2Endpoint{RevokeURL: srv.URL},
		}
		err := cfg.Revoke(context.Background(), "tok")
		if err == nil {
			t.Fatal("expected error")
		}
		var oerr *OAuth2Error
		if !errors.As(err, &oerr) {
			t.Fatalf("expected *OAuth2Error, got %T: %v", err, err)
		}
		if oerr.StatusCode != http.StatusBadRequest || oerr.Code != "invalid_token" {
			t.Errorf("unexpected oauth error: %+v", oerr)
		}
	})
}

func TestOAuthFlowNewClient(t *testing.T) {
	fixed := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	old := timeNow
	timeNow = func() time.Time { return fixed }
	t.Cleanup(func() { timeNow = old })

	// Token endpoint returns a fresh bearer token for the refresh exchange.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "refresh_token" {
			t.Errorf("grant_type = %q", r.Form.Get("grant_type"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"wmblyo_fresh","token_type":"Bearer","expires_in":3600}`))
	}))
	t.Cleanup(tokenSrv.Close)

	var gotAuth string
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"id":"camp_1","name":"Q3"}`))
	}))
	t.Cleanup(apiSrv.Close)

	cfg := &OAuth2Config{
		ClientID: "client_123",
		Endpoint: OAuth2Endpoint{TokenURL: tokenSrv.URL},
	}
	// Seed with an expired token bearing a refresh token so the source refreshes.
	expired := &Token{AccessToken: "wmblyo_old", RefreshToken: "rt_1", Expiry: fixed.Add(-time.Hour)}
	c, err := cfg.NewClient(context.Background(), expired, WithBaseURL(apiSrv.URL+"/v1/"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, _, err := c.Campaigns.Get(context.Background(), "camp_1"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotAuth != "Bearer wmblyo_fresh" {
		t.Errorf("Authorization = %q, want Bearer wmblyo_fresh", gotAuth)
	}
}

func TestOAuthFlowClientCredentialsNewClient(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "client_credentials" {
			t.Errorf("grant_type = %q", r.Form.Get("grant_type"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"wmblyo_m2m","token_type":"Bearer","expires_in":7200}`))
	}))
	t.Cleanup(tokenSrv.Close)

	var gotAuth string
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"id":"camp_1","name":"Q3"}`))
	}))
	t.Cleanup(apiSrv.Close)

	cfg := &ClientCredentialsConfig{
		ClientID:     "client_123",
		ClientSecret: "shh",
		TokenURL:     tokenSrv.URL,
	}
	c, err := cfg.NewClient(context.Background(), WithBaseURL(apiSrv.URL+"/v1/"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, _, err := c.Campaigns.Get(context.Background(), "camp_1"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotAuth != "Bearer wmblyo_m2m" {
		t.Errorf("Authorization = %q, want Bearer wmblyo_m2m", gotAuth)
	}
}

func TestOAuthFlowClientCredentialsTokenSourceCaches(t *testing.T) {
	fixed := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	old := timeNow
	timeNow = func() time.Time { return fixed }
	t.Cleanup(func() { timeNow = old })

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"wmblyo_cc","token_type":"Bearer","expires_in":7200}`))
	}))
	t.Cleanup(srv.Close)

	cfg := &ClientCredentialsConfig{
		ClientID: "client_123",
		TokenURL: srv.URL,
	}
	ts := cfg.TokenSource(context.Background())

	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("first Token: %v", err)
	}
	if tok.AccessToken != "wmblyo_cc" {
		t.Errorf("access token = %q", tok.AccessToken)
	}
	// Second call reuses the cached, still-valid token.
	tok2, err := ts.Token()
	if err != nil {
		t.Fatalf("second Token: %v", err)
	}
	if tok2.AccessToken != "wmblyo_cc" {
		t.Errorf("second access token = %q", tok2.AccessToken)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("token endpoint called %d times, want 1", got)
	}
}

func TestOAuthFlowClientCredentialsTokenSourceError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
	}))
	t.Cleanup(srv.Close)

	cfg := &ClientCredentialsConfig{ClientID: "client_123", TokenURL: srv.URL}
	ts := cfg.TokenSource(context.Background())
	_, err := ts.Token()
	if err == nil {
		t.Fatal("expected error")
	}
	var oerr *OAuth2Error
	if !errors.As(err, &oerr) {
		t.Fatalf("expected *OAuth2Error, got %T: %v", err, err)
	}
}

func TestOAuthFlowErrorFormatting(t *testing.T) {
	tests := []struct {
		name string
		err  *OAuth2Error
		want string
	}{
		{
			name: "description",
			err:  &OAuth2Error{StatusCode: 400, Code: "invalid_grant", Description: "code expired"},
			want: "warmbly: oauth2: 400 invalid_grant: code expired",
		},
		{
			name: "message only",
			err:  &OAuth2Error{StatusCode: 400, Code: "invalid_grant", Message: "the code expired"},
			want: "warmbly: oauth2: 400 invalid_grant: the code expired",
		},
		{
			name: "empty code falls back to status text",
			err:  &OAuth2Error{StatusCode: http.StatusBadRequest},
			want: "warmbly: oauth2: 400 Bad Request",
		},
		{
			name: "request id appended",
			err:  &OAuth2Error{StatusCode: 401, Code: "invalid_client", Description: "bad creds", RequestID: "req_99"},
			want: "warmbly: oauth2: 401 invalid_client: bad creds [request_id=req_99]",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.err.Error(); got != tc.want {
				t.Errorf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOAuthFlowSetAuthURLParam(t *testing.T) {
	cfg := &OAuth2Config{
		ClientID: "client_123",
		Endpoint: OAuth2Endpoint{AuthURL: "https://auth.example.com/oauth/authorize"},
	}
	raw := cfg.AuthCodeURL("st", SetAuthURLParam("prompt", "consent"))
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := u.Query().Get("prompt"); got != "consent" {
		t.Errorf("prompt = %q, want consent", got)
	}
}

func TestOAuthFlowParseFormEncodedToken(t *testing.T) {
	fixed := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	old := timeNow
	timeNow = func() time.Time { return fixed }
	t.Cleanup(func() { timeNow = old })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-www-form-urlencoded")
		form := url.Values{
			"access_token":  {"wmblyo_form"},
			"token_type":    {"Bearer"},
			"expires_in":    {"3600"},
			"refresh_token": {"rt_form"},
			"scope":         {"campaigns:read contacts:read"},
		}
		_, _ = w.Write([]byte(form.Encode()))
	}))
	t.Cleanup(srv.Close)

	cfg := &OAuth2Config{ClientID: "c", Endpoint: OAuth2Endpoint{TokenURL: srv.URL}}
	tok, err := cfg.Exchange(context.Background(), "code")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if tok.AccessToken != "wmblyo_form" {
		t.Errorf("access token = %q", tok.AccessToken)
	}
	if tok.RefreshToken != "rt_form" {
		t.Errorf("refresh token = %q", tok.RefreshToken)
	}
	if tok.Scope != "campaigns:read contacts:read" {
		t.Errorf("scope = %q", tok.Scope)
	}
	if want := fixed.Add(3600 * time.Second); !tok.Expiry.Equal(want) {
		t.Errorf("Expiry = %v, want %v", tok.Expiry, want)
	}
}

func TestOAuthFlowRefreshNoRefreshToken(t *testing.T) {
	fixed := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	old := timeNow
	timeNow = func() time.Time { return fixed }
	t.Cleanup(func() { timeNow = old })

	cfg := &OAuth2Config{ClientID: "c", Endpoint: OAuth2Endpoint{TokenURL: "http://unused.example"}}
	// Expired token with no refresh token: Token() must error without a request.
	expired := &Token{AccessToken: "wmblyo_old", Expiry: fixed.Add(-time.Hour)}
	ts := cfg.TokenSource(context.Background(), expired)
	_, err := ts.Token()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no refresh token") {
		t.Errorf("error = %q, want it to mention no refresh token", err.Error())
	}
}

func TestOAuthFlowRetrieveTokenClientIDNoSecret(t *testing.T) {
	var gotClientID string
	var sawBasicAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		gotClientID = r.Form.Get("client_id")
		_, _, sawBasicAuth = r.BasicAuth()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"wmblyo_pub","token_type":"Bearer"}`))
	}))
	t.Cleanup(srv.Close)

	// ClientID set, ClientSecret empty -> client_id in body, no HTTP Basic auth.
	cfg := &OAuth2Config{ClientID: "public_client", Endpoint: OAuth2Endpoint{TokenURL: srv.URL}}
	tok, err := cfg.Exchange(context.Background(), "code")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if tok.AccessToken != "wmblyo_pub" {
		t.Errorf("access token = %q", tok.AccessToken)
	}
	if gotClientID != "public_client" {
		t.Errorf("client_id form value = %q, want public_client", gotClientID)
	}
	if sawBasicAuth {
		t.Error("did not expect HTTP Basic auth when ClientSecret is empty")
	}
}

func TestOAuthFlowDefaultEndpointFallbacks(t *testing.T) {
	t.Run("oauth2config", func(t *testing.T) {
		cfg := &OAuth2Config{} // zero Endpoint, nil HTTPClient
		if got := cfg.authURL(); got != DefaultEndpoint.AuthURL {
			t.Errorf("authURL() = %q, want %q", got, DefaultEndpoint.AuthURL)
		}
		if got := cfg.tokenURL(); got != DefaultEndpoint.TokenURL {
			t.Errorf("tokenURL() = %q, want %q", got, DefaultEndpoint.TokenURL)
		}
		if got := cfg.revokeURL(); got != DefaultEndpoint.RevokeURL {
			t.Errorf("revokeURL() = %q, want %q", got, DefaultEndpoint.RevokeURL)
		}
		if cfg.httpClient() == nil {
			t.Error("httpClient() = nil, want non-nil")
		}
	})

	t.Run("clientcredentialsconfig", func(t *testing.T) {
		cfg := &ClientCredentialsConfig{} // zero TokenURL, nil HTTPClient
		if got := cfg.tokenURL(); got != DefaultEndpoint.TokenURL {
			t.Errorf("tokenURL() = %q, want %q", got, DefaultEndpoint.TokenURL)
		}
		if cfg.httpClient() == nil {
			t.Error("httpClient() = nil, want non-nil")
		}
	})
}

func TestOAuthFlowCustomHTTPClient(t *testing.T) {
	custom := &http.Client{Timeout: 7 * time.Second}

	t.Run("oauth2config", func(t *testing.T) {
		cfg := &OAuth2Config{HTTPClient: custom}
		if cfg.httpClient() != custom {
			t.Error("httpClient() did not return the configured client")
		}
	})

	t.Run("clientcredentialsconfig", func(t *testing.T) {
		cfg := &ClientCredentialsConfig{HTTPClient: custom}
		if cfg.httpClient() != custom {
			t.Error("httpClient() did not return the configured client")
		}
	})
}

func TestOAuthFlowAuthCodeURLMinimal(t *testing.T) {
	// No RedirectURL, no Scopes, empty state, and an AuthURL already carrying a
	// query so the separator becomes "&".
	cfg := &OAuth2Config{
		ClientID: "client_123",
		Endpoint: OAuth2Endpoint{AuthURL: "https://auth.example.com/oauth/authorize?foo=bar"},
	}
	raw := cfg.AuthCodeURL("")
	if !strings.Contains(raw, "?foo=bar&") {
		t.Errorf("expected & separator after existing query, got %q", raw)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := u.Query()
	if q.Get("response_type") != "code" || q.Get("client_id") != "client_123" {
		t.Errorf("unexpected base params: %v", q)
	}
	if q.Has("redirect_uri") || q.Has("scope") || q.Has("state") {
		t.Errorf("did not expect redirect_uri/scope/state, got %v", q)
	}
}

func TestOAuthFlowRefreshWithScopes(t *testing.T) {
	var gotScope string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotScope = r.Form.Get("scope")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"wmblyo_r","token_type":"Bearer"}`))
	}))
	t.Cleanup(srv.Close)

	cfg := &OAuth2Config{
		ClientID: "c",
		Scopes:   []string{"campaigns:read", "contacts:read"},
		Endpoint: OAuth2Endpoint{TokenURL: srv.URL},
	}
	if _, err := cfg.Refresh(context.Background(), "rt_1"); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if gotScope != "campaigns:read contacts:read" {
		t.Errorf("scope = %q", gotScope)
	}
}

func TestOAuthFlowRefreshSourceRotatesRefreshToken(t *testing.T) {
	fixed := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	old := timeNow
	timeNow = func() time.Time { return fixed }
	t.Cleanup(func() { timeNow = old })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Server rotates the refresh token: the new one must be kept as-is.
		_, _ = w.Write([]byte(`{"access_token":"wmblyo_new","token_type":"Bearer","expires_in":3600,"refresh_token":"rt_rotated"}`))
	}))
	t.Cleanup(srv.Close)

	cfg := &OAuth2Config{ClientID: "c", Endpoint: OAuth2Endpoint{TokenURL: srv.URL}}
	expired := &Token{AccessToken: "wmblyo_old", RefreshToken: "rt_old", Expiry: fixed.Add(-time.Hour)}
	ts := cfg.TokenSource(context.Background(), expired)
	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok.RefreshToken != "rt_rotated" {
		t.Errorf("refresh token = %q, want rt_rotated", tok.RefreshToken)
	}
}

func TestOAuthFlowRefreshSourceError(t *testing.T) {
	fixed := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	old := timeNow
	timeNow = func() time.Time { return fixed }
	t.Cleanup(func() { timeNow = old })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	t.Cleanup(srv.Close)

	cfg := &OAuth2Config{ClientID: "c", Endpoint: OAuth2Endpoint{TokenURL: srv.URL}}
	expired := &Token{AccessToken: "old", RefreshToken: "rt_old", Expiry: fixed.Add(-time.Hour)}
	ts := cfg.TokenSource(context.Background(), expired)
	if _, err := ts.Token(); err == nil {
		t.Fatal("expected error from refresh")
	}
}

func TestOAuthFlowClientCredentialsEndpointParams(t *testing.T) {
	var gotAudience, gotResource string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotAudience = r.Form.Get("audience")
		gotResource = r.Form.Get("resource")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"wmblyo_cc","token_type":"Bearer"}`))
	}))
	t.Cleanup(srv.Close)

	cfg := &ClientCredentialsConfig{
		ClientID:       "c",
		Scopes:         []string{"campaigns:read"},
		TokenURL:       srv.URL,
		EndpointParams: url.Values{"audience": {"https://api.warmbly.com"}, "resource": {"res_1"}},
	}
	if _, err := cfg.Token(context.Background()); err != nil {
		t.Fatalf("Token: %v", err)
	}
	if gotAudience != "https://api.warmbly.com" {
		t.Errorf("audience = %q", gotAudience)
	}
	if gotResource != "res_1" {
		t.Errorf("resource = %q", gotResource)
	}
}

func TestOAuthFlowRetrieveTokenNoAccessToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// 2xx but no access_token -> a server_error OAuth2Error with no Body.
		_, _ = w.Write([]byte(`{"token_type":"Bearer"}`))
	}))
	t.Cleanup(srv.Close)

	cfg := &OAuth2Config{ClientID: "c", Endpoint: OAuth2Endpoint{TokenURL: srv.URL}}
	_, err := cfg.Exchange(context.Background(), "code")
	if err == nil {
		t.Fatal("expected error")
	}
	var oerr *OAuth2Error
	if !errors.As(err, &oerr) {
		t.Fatalf("expected *OAuth2Error, got %T: %v", err, err)
	}
	if oerr.Code != "server_error" {
		t.Errorf("code = %q, want server_error", oerr.Code)
	}
	if oerr.Body != nil {
		t.Errorf("Body = %q, want nil (no issued credentials in error)", oerr.Body)
	}
}

func TestOAuthFlowRetrieveTokenDoError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // closed immediately -> connection refused

	cfg := &OAuth2Config{ClientID: "c", Endpoint: OAuth2Endpoint{TokenURL: srv.URL}}
	if _, err := cfg.Exchange(context.Background(), "code"); err == nil {
		t.Fatal("expected transport error")
	}
}

func TestOAuthFlowRetrieveTokenBadURL(t *testing.T) {
	// A control character in the URL makes http.NewRequestWithContext fail.
	cfg := &OAuth2Config{ClientID: "c", Endpoint: OAuth2Endpoint{TokenURL: "http://\x7f.example"}}
	if _, err := cfg.Exchange(context.Background(), "code"); err == nil {
		t.Fatal("expected request-build error")
	}
}

func TestOAuthFlowRevokeBadURL(t *testing.T) {
	cfg := &OAuth2Config{ClientID: "c", Endpoint: OAuth2Endpoint{RevokeURL: "http://\x7f.example"}}
	if err := cfg.Revoke(context.Background(), "tok"); err == nil {
		t.Fatal("expected request-build error")
	}
}

func TestOAuthFlowRevokeDoError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // connection refused

	cfg := &OAuth2Config{ClientID: "c", Endpoint: OAuth2Endpoint{RevokeURL: srv.URL}}
	if err := cfg.Revoke(context.Background(), "tok"); err == nil {
		t.Fatal("expected transport error")
	}
}

func TestOAuthFlowRevokeClientIDNoSecret(t *testing.T) {
	var gotClientID string
	var sawBasicAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotClientID = r.Form.Get("client_id")
		_, _, sawBasicAuth = r.BasicAuth()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	cfg := &OAuth2Config{ClientID: "public_client", Endpoint: OAuth2Endpoint{RevokeURL: srv.URL}}
	if err := cfg.Revoke(context.Background(), "tok"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if gotClientID != "public_client" {
		t.Errorf("client_id form value = %q", gotClientID)
	}
	if sawBasicAuth {
		t.Error("did not expect HTTP Basic auth when ClientSecret is empty")
	}
}

func TestOAuthFlowRetrieveTokenBodyReadError(t *testing.T) {
	// Promise more bytes than we send, then hijack and close the connection so
	// the client's io.ReadAll fails with an unexpected EOF.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatalf("ResponseWriter is not a Hijacker")
		}
		conn, buf, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack: %v", err)
		}
		_, _ = buf.WriteString("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n{}")
		_ = buf.Flush()
		_ = conn.Close()
	}))
	t.Cleanup(srv.Close)

	cfg := &OAuth2Config{ClientID: "c", Endpoint: OAuth2Endpoint{TokenURL: srv.URL}}
	if _, err := cfg.Exchange(context.Background(), "code"); err == nil {
		t.Fatal("expected body read error")
	}
}

func TestOAuthFlowParseTokenDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	t.Cleanup(srv.Close)

	cfg := &OAuth2Config{ClientID: "c", Endpoint: OAuth2Endpoint{TokenURL: srv.URL}}
	_, err := cfg.Exchange(context.Background(), "code")
	if err == nil {
		t.Fatal("expected decode error")
	}
	if !strings.Contains(err.Error(), "decode token response") {
		t.Errorf("error = %q, want it to mention decode token response", err.Error())
	}
}

func TestOAuthFlowParseTokenFormParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-www-form-urlencoded")
		// Invalid percent-encoding in a form body triggers url.ParseQuery to fail.
		_, _ = w.Write([]byte("access_token=%zz"))
	}))
	t.Cleanup(srv.Close)

	cfg := &OAuth2Config{ClientID: "c", Endpoint: OAuth2Endpoint{TokenURL: srv.URL}}
	_, err := cfg.Exchange(context.Background(), "code")
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "parse token response") {
		t.Errorf("error = %q, want it to mention parse token response", err.Error())
	}
}
