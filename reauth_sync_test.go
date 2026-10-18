package warmbly

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReauthRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()

	cases := []struct {
		name       string
		call       func() error
		wantMethod string
		wantPath   string
		wantBody   string
	}{
		{"Reauth password", func() error {
			_, _, e := c.Auth.Reauth(ctx, "hunter2", "")
			return e
		}, "POST", "/v1/auth/reauth", `{"password":"hunter2"}`},
		{"Reauth code", func() error {
			_, _, e := c.Auth.Reauth(ctx, "", "123456")
			return e
		}, "POST", "/v1/auth/reauth", `{"code":"123456"}`},
		{"ReauthMethods", func() error { _, _, e := c.Auth.ReauthMethods(ctx); return e }, "GET", "/v1/auth/reauth/methods", ""},
		{"BeginReauthPasskey", func() error { _, _, e := c.Auth.BeginReauthPasskey(ctx); return e }, "POST", "/v1/auth/reauth/passkey/begin", ""},
		{"FinishReauthPasskey", func() error {
			_, _, e := c.Auth.FinishReauthPasskey(ctx, json.RawMessage(`{"id":"cred"}`))
			return e
		}, "POST", "/v1/auth/reauth/passkey/finish", `{"credential":{"id":"cred"}}`},
		{"BeginReauthSSO", func() error { _, _, e := c.Auth.BeginReauthSSO(ctx, "google"); return e },
			"POST", "/v1/auth/reauth/sso/begin", `{"provider":"google"}`},
		{"FinishReauthSSO", func() error { _, _, e := c.Auth.FinishReauthSSO(ctx, "c1", "b1"); return e },
			"POST", "/v1/auth/reauth/sso/finish", `{"code":"c1","binding":"b1"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got = recordedRequest{}
			if err := tc.call(); err != nil {
				t.Fatalf("call: %v", err)
			}
			if got.method != tc.wantMethod || got.path != tc.wantPath {
				t.Errorf("got %s %s, want %s %s", got.method, got.path, tc.wantMethod, tc.wantPath)
			}
			if tc.wantBody != "" && strings.TrimSpace(got.body) != tc.wantBody {
				t.Errorf("body = %s, want %s", got.body, tc.wantBody)
			}
		})
	}
}

func reauthClient(t *testing.T, status int, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestReauthDecode(t *testing.T) {
	ctx := context.Background()

	c := reauthClient(t, 200, `{"valid_for_seconds":300}`)
	res, _, err := c.Auth.Reauth(ctx, "x", "")
	if err != nil || res != 300*time.Second {
		t.Fatalf("Reauth = %+v, %v", res, err)
	}

	c = reauthClient(t, 200, `{"password":true,"totp":false,"passkey":true,
		"providers":[{"id":"google","label":"Google"}],"native_providers":["apple"],"valid_for_seconds":300}`)
	m, _, err := c.Auth.ReauthMethods(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Password || m.TOTP || !m.Passkey || len(m.Providers) != 1 || m.Providers[0].Label != "Google" ||
		len(m.NativeProviders) != 1 || m.NativeProviders[0] != "apple" || m.ValidForSeconds != 300 {
		t.Errorf("methods = %+v", m)
	}

	c = reauthClient(t, 200, `{"options":{"publicKey":{"challenge":"abc"}}}`)
	opts, _, err := c.Auth.BeginReauthPasskey(ctx)
	if err != nil || string(opts) != `{"publicKey":{"challenge":"abc"}}` {
		t.Fatalf("options = %s, %v", opts, err)
	}

	c = reauthClient(t, 200, `{"url":"https://idp.example/auth","binding":"bind"}`)
	red, _, err := c.Auth.BeginReauthSSO(ctx, "google")
	if err != nil || red.URL != "https://idp.example/auth" || red.Binding != "bind" {
		t.Fatalf("redirect = %+v, %v", red, err)
	}
}

func TestReauthErrorCodes(t *testing.T) {
	ctx := context.Background()
	c := reauthClient(t, 400, `{"error":"Bad Request","message":"expired","code":"reauth_expired"}`)
	_, _, err := c.Auth.FinishReauthSSO(ctx, "c", "b")
	var apiErr *Error
	if !errors.As(err, &apiErr) || !apiErr.HasCode(ErrCodeReauthExpired) {
		t.Fatalf("err = %v, want reauth_expired", err)
	}
	if apiErr.Temporary() {
		t.Error("reauth_expired must not be Temporary")
	}
}
