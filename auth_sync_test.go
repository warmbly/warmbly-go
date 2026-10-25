package warmbly

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestAuthSyncRoutes(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		body   string
		method string
		path   string
		want   string
		call   func(c *Client) error
	}{
		{"ResetPasswordSession", `{"session":"h1","expires_at":"2026-01-01T00:00:00Z"}`, "POST", "/v1/auth/reset-password/session", `{"token":"tok"}`,
			func(c *Client) error {
				r, _, e := c.Auth.ResetPasswordSession(ctx, "tok")
				if e == nil && (r.Session != "h1" || r.ExpiresAt.IsZero()) {
					t.Errorf("decode: %+v", r)
				}
				return e
			}},
		{"LinkSSO", `{"access_token":"a","refresh_token":"r"}`, "POST", "/v1/auth/sso/link", `{"pending_token":"p","password":"pw"}`,
			func(c *Client) error {
				r, _, e := c.Auth.LinkSSO(ctx, "p", "pw")
				if e == nil && r.AccessToken != "a" {
					t.Errorf("decode: %+v", r)
				}
				return e
			}},
		{"CompleteProductTour", `{"product_tour_completed_at":"2026-05-01T10:00:00Z"}`, "PUT", "/v1/auth/me/product-tour", ``,
			func(c *Client) error {
				at, _, e := c.Auth.CompleteProductTour(ctx)
				if e == nil && at.Year() != 2026 {
					t.Errorf("at = %v", at)
				}
				return e
			}},
		{"RegenerateRecoveryCodes", `{"recovery_codes":["a","b"]}`, "POST", "/v1/auth/2fa/recovery-codes", `{"code":"123456"}`,
			func(c *Client) error {
				r, _, e := c.Auth.RegenerateRecoveryCodes(ctx, "123456")
				if e == nil && len(r.RecoveryCodes) != 2 {
					t.Errorf("decode: %+v", r)
				}
				return e
			}},
		{"Reauth", `{"valid_for_seconds":300}`, "POST", "/v1/auth/reauth", `{"password":"pw"}`,
			func(c *Client) error {
				r, _, e := c.Auth.Reauth(ctx, &ReauthParams{Password: "pw"})
				if e == nil && r.ValidForSeconds != 300 {
					t.Errorf("decode: %+v", r)
				}
				return e
			}},
		{"ReauthMethods", `{"password":true,"totp":false,"passkey":true,"providers":[{"id":"google","label":"Google"}],"native_providers":["apple"],"valid_for_seconds":300}`, "GET", "/v1/auth/reauth/methods", ``,
			func(c *Client) error {
				r, _, e := c.Auth.ReauthMethods(ctx)
				if e == nil && (!r.Passkey || r.Providers[0].ID != "google" || r.NativeProviders[0] != "apple") {
					t.Errorf("decode: %+v", r)
				}
				return e
			}},
		{"BeginPasskeyReauth", `{"options":{"publicKey":{"challenge":"x"}}}`, "POST", "/v1/auth/reauth/passkey/begin", ``,
			func(c *Client) error {
				r, _, e := c.Auth.BeginPasskeyReauth(ctx)
				if e == nil && !strings.Contains(string(r), "challenge") {
					t.Errorf("options = %s", r)
				}
				return e
			}},
		{"FinishPasskeyReauth", `{"valid_for_seconds":300}`, "POST", "/v1/auth/reauth/passkey/finish", `{"credential":{"id":"c"}}`,
			func(c *Client) error {
				_, _, e := c.Auth.FinishPasskeyReauth(ctx, json.RawMessage(`{"id":"c"}`))
				return e
			}},
		{"BeginSSOReauth", `{"url":"https://idp/x","binding":"b"}`, "POST", "/v1/auth/reauth/sso/begin", `{"provider":"google"}`,
			func(c *Client) error {
				r, _, e := c.Auth.BeginSSOReauth(ctx, "google")
				if e == nil && r.Binding != "b" {
					t.Errorf("decode: %+v", r)
				}
				return e
			}},
		{"FinishSSOReauth", `{"valid_for_seconds":300}`, "POST", "/v1/auth/reauth/sso/finish", `{"code":"c","binding":"b"}`,
			func(c *Client) error { _, _, e := c.Auth.FinishSSOReauth(ctx, "c", "b"); return e }},
		{"BeginNativeReauth", `{"nonce":"n1"}`, "POST", "/v1/auth/reauth/native/begin", ``,
			func(c *Client) error {
				n, _, e := c.Auth.BeginNativeReauth(ctx)
				if e == nil && n != "n1" {
					t.Errorf("nonce = %q", n)
				}
				return e
			}},
		{"ReauthNative", `{"valid_for_seconds":300}`, "POST", "/v1/auth/reauth/native", `{"provider":"apple","id_token":"jwt"}`,
			func(c *Client) error { _, _, e := c.Auth.ReauthNative(ctx, "apple", "jwt"); return e }},
		{"ChangePasswordSession", `{"access_token":"new","refresh_token":"r2"}`, "POST", "/v1/auth/me/password", `{"current_password":"a","new_password":"b","revoke_api_credentials":true}`,
			func(c *Client) error {
				r, _, e := c.Auth.ChangePasswordSession(ctx, &ChangePasswordParams{CurrentPassword: "a", NewPassword: "b", RevokeAPICredentials: true})
				if e == nil && r.AccessToken != "new" {
					t.Errorf("decode: %+v", r)
				}
				return e
			}},
		{"ResetPasswordConfirmWith", ``, "POST", "/v1/auth/reset-password/confirm", `{"session":"s","password":"p","revoke_api_credentials":true}`,
			func(c *Client) error {
				_, e := c.Auth.ResetPasswordConfirmWith(ctx, &ResetPasswordConfirmParams{Session: "s", Password: "p", RevokeAPICredentials: true})
				return e
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got recordedRequest
			c := accountServer(t, tc.body, &got)
			if err := tc.call(c); err != nil {
				t.Fatal(err)
			}
			if got.method != tc.method || got.path != tc.path || got.rawQuery != "" {
				t.Errorf("got %s %s ?%s", got.method, got.path, got.rawQuery)
			}
			if tc.want != "" && strings.TrimSpace(got.body) != tc.want {
				t.Errorf("body = %s, want %s", got.body, tc.want)
			}
		})
	}
}

func TestAuthSyncChangePasswordSessionEmpty(t *testing.T) {
	c := accountServer(t, ``, nil)
	s, _, err := c.Auth.ChangePasswordSession(context.Background(), &ChangePasswordParams{CurrentPassword: "a", NewPassword: "b"})
	if err != nil || s != nil {
		t.Fatalf("s=%v err=%v", s, err)
	}
}

func TestAuthSyncDecodeConfigAndSession(t *testing.T) {
	c := accountServer(t, `{"captcha":false,"app_attest":true,"gmail_oauth_connect":true,"api_url":"https://api.x","brand":{"name":"X","support_email":"s@x"}}`, nil)
	cfg, _, err := c.Auth.Config(context.Background())
	if err != nil || !cfg.AppAttest || !cfg.GmailOAuthConnect || cfg.APIURL != "https://api.x" || cfg.Brand.Name != "X" {
		t.Fatalf("cfg=%+v err=%v", cfg, err)
	}
	c = accountServer(t, `{"link_required":true,"pending_token":"p","expires_in":300,"link_email":"a@b","link_provider":"google"}`, nil)
	s, _, err := c.Auth.ExchangeSSO(context.Background(), "c", "b")
	if err != nil || !s.LinkRequired || s.LinkProvider != "google" || s.PendingToken != "p" {
		t.Fatalf("s=%+v err=%v", s, err)
	}
	c = accountServer(t, `{"inbound_reply":{"enabled":true},"placement_alert":{"enabled":true,"channels":{"email":true}},"inbox_action_required":{"enabled":true}}`, nil)
	p, _, err := c.Auth.NotificationPreferences(context.Background())
	if err != nil || p == nil {
		t.Fatalf("err=%v", err)
	}
}
