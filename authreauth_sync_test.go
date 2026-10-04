package warmbly

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestAuthReauth(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, `{"valid_for_seconds":300}`)
	out, _, err := c.Auth.Reauth(context.Background(), &ReauthParams{Password: "hunter2hunter2"})
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "POST", "/v1/auth/reauth", "")
	got.wantBody(t, `{"password":"hunter2hunter2"}`)
	if out.ValidForSeconds != 300 {
		t.Errorf("result = %+v", out)
	}

	// An authenticator code travels alone, with no empty password.
	if _, _, err := c.Auth.Reauth(context.Background(), &ReauthParams{Code: "123456"}); err != nil {
		t.Fatal(err)
	}
	got.wantBody(t, `{"code":"123456"}`)
}

func TestAuthReauthNoFactorIsTyped(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 400, `{"error":"Bad Request","message":"nothing to confirm with","code":"reauth_no_factor"}`)
	_, _, err := c.Auth.Reauth(context.Background(), &ReauthParams{Password: "x"})
	var apiErr *Error
	if !errors.As(err, &apiErr) || !apiErr.HasCode("reauth_no_factor") || apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("err = %v", err)
	}
}

func TestAuthRegenerateTwoFARecoveryCodes(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, jsonOf(t, TwoFARecoveryCodes{RecoveryCodes: []string{"aaaa-bbbb", "cccc-dddd"}}))
	out, _, err := c.Auth.RegenerateTwoFARecoveryCodes(context.Background(), "654321")
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "POST", "/v1/auth/2fa/recovery-codes", "")
	got.wantBody(t, `{"code":"654321"}`)
	if len(out.RecoveryCodes) != 2 || out.RecoveryCodes[0] != "aaaa-bbbb" {
		t.Errorf("codes = %+v", out)
	}
}

func TestAuthTwoFAStatusDecodesRecoveryCounts(t *testing.T) {
	confirmed := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	var got syncCapture
	c := respondingClient(t, &got, 200, jsonOf(t, TwoFAStatus{Enabled: true, ConfirmedAt: &confirmed, RecoveryCodesRemaining: 7, RecoveryCodesTotal: 10}))
	out, _, err := c.Auth.TwoFAStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !out.Enabled || out.ConfirmedAt == nil || !out.ConfirmedAt.Equal(confirmed) || out.RecoveryCodesRemaining != 7 || out.RecoveryCodesTotal != 10 {
		t.Errorf("status = %+v", out)
	}

	c = respondingClient(t, &got, 200, `{"enabled":false,"recovery_codes_remaining":0,"recovery_codes_total":0}`)
	out, _, err = c.Auth.TwoFAStatus(context.Background())
	if err != nil || out.Enabled || out.ConfirmedAt != nil {
		t.Errorf("status = %+v, %v", out, err)
	}
}

func TestAuthLinkSSO(t *testing.T) {
	expires := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
	session := Session{
		AccessToken: "at", AccessTokenExpiresAt: expires, RefreshToken: "rt", RefreshTokenExpiresAt: expires,
	}
	var got syncCapture
	c := respondingClient(t, &got, 200, jsonOf(t, session))
	out, _, err := c.Auth.LinkSSO(context.Background(), "pending-jwt", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "POST", "/v1/auth/sso/link", "")
	got.wantBody(t, `{"pending_token":"pending-jwt","password":"correct horse"}`)
	if out.AccessToken != "at" || out.LinkRequired || out.TwoFARequired {
		t.Errorf("session = %+v", out)
	}

	// A linked account with two-factor on finishes through the 2FA step.
	c = respondingClient(t, &got, 200, `{"two_fa_required":true,"pending_token":"2fa-jwt","expires_in":300}`)
	out, _, err = c.Auth.LinkSSO(context.Background(), "pending-jwt", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !out.TwoFARequired || out.PendingToken != "2fa-jwt" || out.ExpiresIn != 300 || out.AccessToken != "" {
		t.Errorf("session = %+v", out)
	}
}

func TestSessionDecodesLinkRequired(t *testing.T) {
	body := `{"link_required":true,"pending_token":"link-jwt","expires_in":600,"link_email":"ada@acme.com","link_provider":"google"}`
	for name, call := range map[string]func(c *Client) (*Session, error){
		"ExchangeSSO": func(c *Client) (*Session, error) {
			s, _, e := c.Auth.ExchangeSSO(context.Background(), "code", "binding")
			return s, e
		},
		"LoginWithGoogle": func(c *Client) (*Session, error) {
			s, _, e := c.Auth.LoginWithGoogle(context.Background(), "id-token")
			return s, e
		},
		"LoginWithApple": func(c *Client) (*Session, error) {
			s, _, e := c.Auth.LoginWithApple(context.Background(), "id-token")
			return s, e
		},
		"LoginConfirm": func(c *Client) (*Session, error) {
			s, _, e := c.Auth.LoginConfirm(context.Background(), &ConfirmParams{Session: "s", Code: "1"})
			return s, e
		},
	} {
		t.Run(name, func(t *testing.T) {
			var got syncCapture
			c := respondingClient(t, &got, 200, body)
			s, err := call(c)
			if err != nil {
				t.Fatal(err)
			}
			if !s.LinkRequired || s.PendingToken != "link-jwt" || s.ExpiresIn != 600 || s.LinkEmail != "ada@acme.com" ||
				s.LinkProvider != SSOProviderGoogle || s.AccessToken != "" {
				t.Errorf("session = %+v", s)
			}
		})
	}
}

func TestAuthChangePasswordReturnsTheNewSession(t *testing.T) {
	expires := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
	var got syncCapture
	c := respondingClient(t, &got, 200, jsonOf(t, Session{AccessToken: "new-at", RefreshToken: "new-rt", AccessTokenExpiresAt: expires, RefreshTokenExpiresAt: expires}))
	out, _, err := c.Auth.ChangePassword(context.Background(), "old", "brand-new-password")
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "POST", "/v1/auth/me/password", "")
	got.wantBody(t, `{"current_password":"old","new_password":"brand-new-password"}`)
	if out.AccessToken != "new-at" || out.RefreshToken != "new-rt" {
		t.Errorf("session = %+v", out)
	}

	// A server that answers with no body leaves an empty session, not an error.
	c = respondingClient(t, &got, 200, ``)
	out, _, err = c.Auth.ChangePassword(context.Background(), "old", "brand-new-password")
	if err != nil || out.AccessToken != "" {
		t.Errorf("empty body: %+v, %v", out, err)
	}

	c = respondingClient(t, &got, 409, `{"error":"Conflict","message":"sign in again","code":"password_changed_sign_in_again"}`)
	_, _, err = c.Auth.ChangePassword(context.Background(), "old", "brand-new-password")
	var apiErr *Error
	if !errors.As(err, &apiErr) || !apiErr.HasCode("password_changed_sign_in_again") {
		t.Errorf("err = %v", err)
	}
}

func TestAuthConfigDecodesNewFields(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, jsonOf(t, AuthConfig{
		Providers: []string{SSOProviderGoogle}, GmailOAuthConnect: true, APIURL: "https://api.acme.io",
		Brand: AuthBrand{Name: "Acme Mail", SupportEmail: "help@acme.io"},
	}))
	out, _, err := c.Auth.Config(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !out.GmailOAuthConnect || out.APIURL != "https://api.acme.io" || out.Brand.Name != "Acme Mail" || out.Brand.SupportEmail != "help@acme.io" {
		t.Errorf("config = %+v", out)
	}
}

func TestAuthViewPreferences(t *testing.T) {
	updated := time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC)
	saved := ViewPreferences{
		View:      ViewNameContacts,
		Columns:   []string{"name", "company", "custom:Industry"},
		Sort:      &ViewSort{By: "created_at", Reverse: true},
		UpdatedAt: &updated,
	}
	envelope := func(p ViewPreferences) string { return jsonOf(t, map[string]any{"preferences": p}) }

	t.Run("get", func(t *testing.T) {
		var got syncCapture
		c := respondingClient(t, &got, 200, envelope(saved))
		out, _, err := c.Auth.ViewPreferences(context.Background(), ViewNameContacts)
		if err != nil {
			t.Fatal(err)
		}
		got.wantRequest(t, "GET", "/v1/me/views/contacts", "")
		if out.View != "contacts" || len(out.Columns) != 3 || out.Sort == nil || !out.Sort.Reverse || out.UpdatedAt == nil {
			t.Errorf("prefs = %+v", out)
		}
	})
	t.Run("nothing saved", func(t *testing.T) {
		var got syncCapture
		c := respondingClient(t, &got, 200, `{"preferences":{"view":"campaign_leads","columns":[]}}`)
		out, _, err := c.Auth.ViewPreferences(context.Background(), ViewNameCampaignLeads)
		if err != nil {
			t.Fatal(err)
		}
		if len(out.Columns) != 0 || out.Sort != nil || out.UpdatedAt != nil || out.Layout != nil {
			t.Errorf("prefs = %+v", out)
		}
	})
	t.Run("put keeps what is left nil", func(t *testing.T) {
		var got syncCapture
		c := respondingClient(t, &got, 200, envelope(saved))
		cols := []string{"name", "custom:Industry"}
		out, _, err := c.Auth.SaveViewPreferences(context.Background(), ViewNameContacts, &ViewPreferencesUpdateParams{Columns: &cols})
		if err != nil {
			t.Fatal(err)
		}
		got.wantRequest(t, "PUT", "/v1/me/views/contacts", "")
		got.wantBody(t, `{"columns":["name","custom:Industry"]}`)
		if out.View != "contacts" {
			t.Errorf("prefs = %+v", out)
		}
	})
	t.Run("put resets columns and sort to defaults", func(t *testing.T) {
		var got syncCapture
		c := respondingClient(t, &got, 200, envelope(ViewPreferences{View: ViewNameContacts, Columns: []string{}}))
		none := []string{}
		if _, _, err := c.Auth.SaveViewPreferences(context.Background(), ViewNameContacts, &ViewPreferencesUpdateParams{Columns: &none, Sort: &ViewSort{}}); err != nil {
			t.Fatal(err)
		}
		got.wantBody(t, `{"columns":[],"sort":{"by":"","reverse":false}}`)
	})
	t.Run("put null layout resets the rail", func(t *testing.T) {
		var got syncCapture
		layout := json.RawMessage(`{"favorites":[{"key":"folder:inbox"}],"hidden":[],"order":{},"section_order":[]}`)
		c := respondingClient(t, &got, 200, envelope(ViewPreferences{View: ViewNameUniboxRail, Columns: []string{}, Layout: layout}))
		out, _, err := c.Auth.SaveViewPreferences(context.Background(), ViewNameUniboxRail, &ViewPreferencesUpdateParams{Layout: layout})
		if err != nil {
			t.Fatal(err)
		}
		got.wantRequest(t, "PUT", "/v1/me/views/unibox_rail", "")
		got.wantBody(t, `{"layout":{"favorites":[{"key":"folder:inbox"}],"hidden":[],"order":{},"section_order":[]}}`)
		if string(out.Layout) != string(layout) {
			t.Errorf("layout = %s", out.Layout)
		}
		if _, _, err := c.Auth.SaveViewPreferences(context.Background(), ViewNameUniboxRail, &ViewPreferencesUpdateParams{Layout: json.RawMessage("null")}); err != nil {
			t.Fatal(err)
		}
		got.wantBody(t, `{"layout":null}`)
	})
	t.Run("delete", func(t *testing.T) {
		var got syncCapture
		c := respondingClient(t, &got, http.StatusNoContent, "")
		resp, err := c.Auth.ResetViewPreferences(context.Background(), ViewNameContacts)
		if err != nil {
			t.Fatal(err)
		}
		got.wantRequest(t, "DELETE", "/v1/me/views/contacts", "")
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("status = %d", resp.StatusCode)
		}
	})
}
