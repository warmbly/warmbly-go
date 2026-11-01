package warmbly

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// idCall runs fn against a server that answers every request with body and
// returns what the server saw.
func idCall(t *testing.T, body string, fn func(c *Client) error) recordedRequest {
	t.Helper()
	var got recordedRequest
	c := accountServer(t, body, &got)
	if err := fn(c); err != nil {
		t.Fatalf("call failed: %v", err)
	}
	return got
}

func wantReq(t *testing.T, got recordedRequest, method, path, query, body string) {
	t.Helper()
	if got.method != method || got.path != path || got.rawQuery != query {
		t.Fatalf("request = %s %s?%s, want %s %s?%s", got.method, got.path, got.rawQuery, method, path, query)
	}
	if body == "" {
		return
	}
	if !jsonEqual(t, got.body, body) {
		t.Fatalf("body = %s, want %s", got.body, body)
	}
}

func jsonEqual(t *testing.T, a, b string) bool {
	t.Helper()
	var x, y any
	if err := json.Unmarshal([]byte(a), &x); err != nil {
		t.Fatalf("bad JSON %q: %v", a, err)
	}
	if err := json.Unmarshal([]byte(b), &y); err != nil {
		t.Fatalf("bad JSON %q: %v", b, err)
	}
	xa, _ := json.Marshal(x)
	ya, _ := json.Marshal(y)
	return string(xa) == string(ya)
}

func TestAuthSecurityRouting(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		resp   string
		call   func(c *Client) error
		method string
		path   string
		query  string
		body   string
	}{
		{"CompleteInitialPassword", `{}`, func(c *Client) error {
			_, _, e := c.Auth.CompleteInitialPassword(ctx, "pt", "newpw", "dev")
			return e
		}, "POST", "/v1/auth/login/initial-password", "", `{"pending_token":"pt","password":"newpw","device_token":"dev"}`},
		{"LinkSSO", `{}`, func(c *Client) error { _, _, e := c.Auth.LinkSSO(ctx, "pt", "pw"); return e },
			"POST", "/v1/auth/sso/link", "", `{"pending_token":"pt","password":"pw"}`},
		{"ResetPasswordSession", `{"session":"s","expires_at":"2026-01-02T03:04:05Z"}`, func(c *Client) error {
			_, _, e := c.Auth.ResetPasswordSession(ctx, "tok")
			return e
		}, "POST", "/v1/auth/reset-password/session", "", `{"token":"tok"}`},
		{"ResetPasswordComplete", ``, func(c *Client) error {
			_, e := c.Auth.ResetPasswordComplete(ctx, &ResetPasswordConfirmParams{Session: "s", Password: "pw", RevokeAPICredentials: true})
			return e
		}, "POST", "/v1/auth/reset-password/confirm", "", `{"session":"s","password":"pw","revoke_api_credentials":true}`},
		{"LoginWithAppleProfile", `{}`, func(c *Client) error {
			_, _, e := c.Auth.LoginWithAppleProfile(ctx, "it", "Ada", "Lovelace")
			return e
		}, "POST", "/v1/auth/apple", "", `{"identity_token":"it","first_name":"Ada","last_name":"Lovelace"}`},
		{"LogoutForgetDevices", ``, func(c *Client) error {
			_, e := c.Auth.LogoutForgetDevices(ctx, []string{"a", "b"})
			return e
		}, "POST", "/v1/auth/logout", "", `{"device_tokens":["a","b"]}`},
		{"CompleteProductTour", `{"product_tour_completed_at":"2026-01-02T03:04:05Z"}`, func(c *Client) error {
			_, _, e := c.Auth.CompleteProductTour(ctx)
			return e
		}, "PUT", "/v1/auth/me/product-tour", "", ""},
		{"ChangePasswordSession", `{}`, func(c *Client) error {
			_, _, e := c.Auth.ChangePasswordSession(ctx, &ChangePasswordParams{CurrentPassword: "a", NewPassword: "b", RevokeAPICredentials: true})
			return e
		}, "POST", "/v1/auth/me/password", "", `{"current_password":"a","new_password":"b","revoke_api_credentials":true}`},
		{"SendPasswordLink", ``, func(c *Client) error { _, e := c.Auth.SendPasswordLink(ctx); return e },
			"POST", "/v1/auth/me/password/link", "", ""},
		{"LinkedIdentities", `{"data":[]}`, func(c *Client) error { _, _, e := c.Auth.LinkedIdentities(ctx); return e },
			"GET", "/v1/auth/me/identities", "", ""},
		{"UnlinkIdentity", ``, func(c *Client) error { _, e := c.Auth.UnlinkIdentity(ctx, "google"); return e },
			"DELETE", "/v1/auth/me/identities/google", "", ""},
		{"RegenerateRecoveryCodes", `{"recovery_codes":[]}`, func(c *Client) error {
			_, _, e := c.Auth.RegenerateRecoveryCodes(ctx, "123456")
			return e
		}, "POST", "/v1/auth/2fa/recovery-codes", "", `{"code":"123456"}`},
		{"ReauthMethods", `{}`, func(c *Client) error { _, _, e := c.Auth.ReauthMethods(ctx); return e },
			"GET", "/v1/auth/reauth/methods", "", ""},
		{"Reauth", `{"valid_for_seconds":300}`, func(c *Client) error {
			_, _, e := c.Auth.Reauth(ctx, &ReauthParams{Password: "pw"})
			return e
		}, "POST", "/v1/auth/reauth", "", `{"password":"pw"}`},
		{"BeginReauthPasskey", `{"options":{"publicKey":{}}}`, func(c *Client) error {
			_, _, e := c.Auth.BeginReauthPasskey(ctx)
			return e
		}, "POST", "/v1/auth/reauth/passkey/begin", "", ""},
		{"FinishReauthPasskey", `{"valid_for_seconds":300}`, func(c *Client) error {
			_, _, e := c.Auth.FinishReauthPasskey(ctx, json.RawMessage(`{"id":"x"}`))
			return e
		}, "POST", "/v1/auth/reauth/passkey/finish", "", `{"credential":{"id":"x"}}`},
		{"BeginReauthSSO", `{"url":"https://idp","binding":"b"}`, func(c *Client) error {
			_, _, e := c.Auth.BeginReauthSSO(ctx, "google")
			return e
		}, "POST", "/v1/auth/reauth/sso/begin", "", `{"provider":"google"}`},
		{"FinishReauthSSO", `{"valid_for_seconds":300}`, func(c *Client) error {
			_, _, e := c.Auth.FinishReauthSSO(ctx, "code", "bind")
			return e
		}, "POST", "/v1/auth/reauth/sso/finish", "", `{"code":"code","binding":"bind"}`},
		{"BeginReauthNative", `{"nonce":"n"}`, func(c *Client) error { _, _, e := c.Auth.BeginReauthNative(ctx); return e },
			"POST", "/v1/auth/reauth/native/begin", "", ""},
		{"ReauthNative", `{"valid_for_seconds":300}`, func(c *Client) error {
			_, _, e := c.Auth.ReauthNative(ctx, "apple", "idt")
			return e
		}, "POST", "/v1/auth/reauth/native", "", `{"provider":"apple","id_token":"idt"}`},
		{"CLIAuthRequestFor", `{}`, func(c *Client) error {
			_, _, e := c.Auth.CLIAuthRequestFor(ctx, "ABCD-1234", "org_1")
			return e
		}, "GET", "/v1/auth/cli/codes/ABCD-1234", "organization_id=org_1", ""},
		{"ViewPreferences", `{"preferences":{"view":"contacts","columns":[]}}`, func(c *Client) error {
			_, _, e := c.Auth.ViewPreferences(ctx, ViewContacts)
			return e
		}, "GET", "/v1/me/views/contacts", "", ""},
		{"ResetViewPreferences", ``, func(c *Client) error { _, e := c.Auth.ResetViewPreferences(ctx, ViewContacts); return e },
			"DELETE", "/v1/me/views/contacts", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := idCall(t, tc.resp, tc.call)
			wantReq(t, got, tc.method, tc.path, tc.query, tc.body)
		})
	}
}

func TestSaveViewPreferencesPartialBodies(t *testing.T) {
	ctx := context.Background()
	empty := []string{}
	cols := []string{"name", "custom:Industry"}
	cases := []struct {
		name   string
		params *ViewPreferencesParams
		body   string
	}{
		{"sort only keeps columns", &ViewPreferencesParams{Sort: &ViewSort{By: "created_at", Reverse: true}}, `{"sort":{"by":"created_at","reverse":true}}`},
		{"empty columns restore default", &ViewPreferencesParams{Columns: &empty}, `{"columns":[]}`},
		{"columns", &ViewPreferencesParams{Columns: &cols}, `{"columns":["name","custom:Industry"]}`},
		{"null layout restores default", &ViewPreferencesParams{Layout: json.RawMessage("null")}, `{"layout":null}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var vp *ViewPreferences
			got := idCall(t, `{"preferences":{"view":"unibox_rail","columns":[],"layout":{"hidden":["a"]},"updated_at":"2026-01-02T03:04:05Z"}}`, func(c *Client) error {
				var err error
				vp, _, err = c.Auth.SaveViewPreferences(ctx, ViewUniboxRail, tc.params)
				return err
			})
			wantReq(t, got, "PUT", "/v1/me/views/unibox_rail", "", tc.body)
			if vp.View != "unibox_rail" || vp.UpdatedAt == nil || !strings.Contains(string(vp.Layout), "hidden") {
				t.Fatalf("decoded = %+v", vp)
			}
		})
	}
}

func TestSessionDecodesLinkAndDeviceFields(t *testing.T) {
	var s *Session
	idCall(t, `{"link_required":true,"link_email":"a@b.com","link_provider":"google","pending_token":"pt","expires_in":900}`, func(c *Client) error {
		var err error
		s, _, err = c.Auth.ExchangeSSO(context.Background(), "code", "bind")
		return err
	})
	if !s.LinkRequired || s.LinkEmail != "a@b.com" || s.LinkProvider != "google" || s.PendingToken != "pt" || s.AccessToken != "" {
		t.Fatalf("session = %+v", s)
	}

	idCall(t, `{"access_token":"at","access_token_expires_at":"2026-01-02T03:04:05Z","refresh_token":"rt","refresh_token_expires_at":"2026-02-02T03:04:05Z","device_token":"dev"}`, func(c *Client) error {
		var err error
		s, _, err = c.Auth.LoginConfirm(context.Background(), &ConfirmParams{Session: "s", Code: "1", RememberDevice: true})
		return err
	})
	if s.DeviceToken != "dev" || s.AccessToken != "at" {
		t.Fatalf("session = %+v", s)
	}
}

func TestLoginSendsRememberedDeviceAndAcquisition(t *testing.T) {
	var step *AuthStep
	got := idCall(t, `{"password_change_required":true,"pending_token":"pt","expires_in":900,"code_required":false}`, func(c *Client) error {
		var err error
		step, _, err = c.Auth.Login(context.Background(), &LoginParams{
			Email: "a@b.com", Password: "pw", DeviceToken: "dev",
			Acquisition: &Acquisition{UTMSource: "news", LandingPath: "/pricing"},
		})
		return err
	})
	wantReq(t, got, "POST", "/v1/auth/login", "",
		`{"email":"a@b.com","password":"pw","device_token":"dev","acquisition":{"landing_path":"/pricing","utm_source":"news"}}`)
	if !step.PasswordChangeRequired || step.PendingToken != "pt" || !step.Done() {
		t.Fatalf("step = %+v", step)
	}
}

func TestChangePasswordSessionDecodesReplacement(t *testing.T) {
	var s *Session
	idCall(t, `{"access_token":"new","access_token_expires_at":"2026-01-02T03:04:05Z","refresh_token":"newr","refresh_token_expires_at":"2026-02-02T03:04:05Z"}`, func(c *Client) error {
		var err error
		s, _, err = c.Auth.ChangePasswordSession(context.Background(), &ChangePasswordParams{CurrentPassword: "a", NewPassword: "b"})
		return err
	})
	if s.AccessToken != "new" || s.RefreshToken != "newr" {
		t.Fatalf("session = %+v", s)
	}
	// An empty 200 (no replacement issued) still succeeds.
	idCall(t, ``, func(c *Client) error {
		s, _, _ = c.Auth.ChangePasswordSession(context.Background(), &ChangePasswordParams{CurrentPassword: "a", NewPassword: "b"})
		return nil
	})
	if s == nil || s.AccessToken != "" {
		t.Fatalf("empty response = %+v", s)
	}
}

func TestReauthAndIdentityDecoding(t *testing.T) {
	var m *ReauthMethods
	idCall(t, `{"password":true,"totp":false,"passkey":true,"providers":[{"id":"oidc","label":"Authentik"}],"native_providers":["apple"],"valid_for_seconds":300}`, func(c *Client) error {
		var err error
		m, _, err = c.Auth.ReauthMethods(context.Background())
		return err
	})
	if !m.Password || m.TOTP || !m.Passkey || len(m.Providers) != 1 || m.Providers[0].Label != "Authentik" || m.NativeProviders[0] != "apple" || m.ValidForSeconds != 300 {
		t.Fatalf("methods = %+v", m)
	}

	var ids []LinkedIdentity
	idCall(t, `{"data":[{"provider":"google","label":"Google","email":"a@b.com","created_at":"2026-01-02T03:04:05Z","last_login_at":"2026-01-03T03:04:05Z"},{"provider":"novel","label":"New","created_at":"2026-01-02T03:04:05Z"}]}`, func(c *Client) error {
		var err error
		ids, _, err = c.Auth.LinkedIdentities(context.Background())
		return err
	})
	if len(ids) != 2 || ids[0].LastLoginAt == nil || ids[1].LastLoginAt != nil || ids[1].Provider != "novel" {
		t.Fatalf("identities = %+v", ids)
	}

	var at json.RawMessage
	idCall(t, `{"options":{"publicKey":{"challenge":"abc"}}}`, func(c *Client) error {
		var err error
		at, _, err = c.Auth.BeginReauthPasskey(context.Background())
		return err
	})
	if !strings.Contains(string(at), "challenge") || strings.Contains(string(at), `"options"`) {
		t.Fatalf("options = %s", at)
	}
}

func TestAuthConfigNewFields(t *testing.T) {
	var cfg *AuthConfig
	idCall(t, `{"captcha":false,"app_attest":true,"password_login":true,"login_code":"new_device","registration":"invite_only","email_verification":true,"mail_delivers":true,"passkeys":true,"providers":["oidc"],"self_hosted":true,"gmail_oauth_connect":true,"billing_enabled":false,"setup_required":false,"invites_required":true,"docs_url":"https://d","api_url":"https://api.example.com","brand":{"name":"Acme","support_email":"s@acme.test"}}`, func(c *Client) error {
		var err error
		cfg, _, err = c.Auth.Config(context.Background())
		return err
	})
	if !cfg.AppAttest || !cfg.GmailOAuthConnect || cfg.APIURL != "https://api.example.com" || cfg.Brand.Name != "Acme" || cfg.Brand.SupportEmail != "s@acme.test" {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestNotificationPreferencesRoundTripKeepsPlacementCategories(t *testing.T) {
	var prefs *NotificationPreferencesResult
	idCall(t, `{"preferences":{"inbound_reply":{"enabled":false,"channels":{"in_app":true,"email":false,"slack":false,"push":true}},"placement_finished":{"enabled":true,"channels":{"in_app":true,"email":false,"slack":false,"push":true}},"placement_alert":{"enabled":true,"channels":{"in_app":true,"email":true,"slack":false,"push":true}},"inbox_action_required":{"enabled":true,"channels":{"in_app":true,"email":true,"slack":false,"push":true}},"email_digest_minutes":30}}`, func(c *Client) error {
		var err error
		prefs, _, err = c.Auth.NotificationPreferences(context.Background())
		return err
	})
	p := prefs.Preferences
	if !p.PlacementFinished.Enabled || !p.PlacementAlert.Channels.Email || !p.InboxActionRequired.Enabled {
		t.Fatalf("prefs = %+v", p)
	}
	got := idCall(t, `{"preferences":{}}`, func(c *Client) error {
		_, _, err := c.Auth.UpdateNotificationPreferences(context.Background(), &p)
		return err
	})
	for _, key := range []string{`"placement_finished"`, `"placement_alert"`, `"inbox_action_required"`} {
		if !strings.Contains(got.body, key) {
			t.Fatalf("update body drops %s: %s", key, got.body)
		}
	}
	if !strings.Contains(got.body, `"placement_alert":{"enabled":true,"channels":{"in_app":true,"email":true,"slack":false,"push":true}}`) {
		t.Fatalf("placement_alert not preserved: %s", got.body)
	}
}

func TestUserSessionAndUserDecodeNewFields(t *testing.T) {
	var sess []UserSession
	idCall(t, `[{"id":"s1","current":true,"browser":"Firefox","os":"Linux","location_city":"Berlin","location_region":"BE","location_country":"Germany","country_code":"DE","auth_provider":"webauthn","mfa_verified":true,"created_at":"2026-01-02T03:04:05Z","last_active_at":"2026-01-02T04:04:05Z"}]`, func(c *Client) error {
		var err error
		sess, _, err = c.Auth.Sessions(context.Background())
		return err
	})
	if !sess[0].MFAVerified || sess[0].AuthProvider != "webauthn" {
		t.Fatalf("session = %+v", sess[0])
	}

	var u *User
	idCall(t, `{"id":"u1","first_name":"A","last_name":"B","email":"a@b.com","roles":[],"referral_source":null,"onboarding_completed_at":null,"product_tour_completed_at":"2026-01-02T03:04:05Z","session_mfa_verified":true,"folders":[],"tags":[],"categories":[],"created_at":"2026-01-02T03:04:05Z","updated_at":"2026-01-02T03:04:05Z"}`, func(c *Client) error {
		var err error
		u, _, err = c.Auth.Me(context.Background())
		return err
	})
	if u.ProductTourCompletedAt == nil || !u.SessionMFAVerified {
		t.Fatalf("user = %+v", u)
	}
}

func TestCLIAuthRequestDecodesGrantedScopes(t *testing.T) {
	var r *CLIAuthRequest
	idCall(t, `{"id":"c1","user_code":"ABCD-1234","client_name":"warmbly","hostname":"h","cli_version":"1","scopes":7,"scope_names":["a"],"granted_scopes":3,"granted_scope_names":["a"],"status":"pending","expires_at":"2026-01-02T03:04:05Z","created_at":"2026-01-02T03:04:05Z"}`, func(c *Client) error {
		var err error
		r, _, err = c.Auth.CLIAuthRequestFor(context.Background(), "ABCD-1234", "org")
		return err
	})
	if r.GrantedScopes == nil || *r.GrantedScopes != 3 || r.GrantedScopeNames[0] != "a" {
		t.Fatalf("request = %+v", r)
	}
}

// --- organization ---

func TestMemberAccessRoutesAndDecoding(t *testing.T) {
	ctx := context.Background()
	var a *MemberAccess
	got := idCall(t, `{"scope":"restricted","campaign_folder_ids":["f1"],"campaign_ids":["c1","c2"],"email_account_ids":[]}`, func(c *Client) error {
		var err error
		a, _, err = c.Organization.MemberAccess(ctx, "user_1")
		return err
	})
	wantReq(t, got, "GET", "/v1/organization/members/user_1/access", "", "")
	if a.Scope != AccessScopeRestricted || len(a.CampaignIDs) != 2 || len(a.CampaignFolderIDs) != 1 {
		t.Fatalf("access = %+v", a)
	}

	var m *Member
	got = idCall(t, `{"id":"m1","organization_id":"o1","user_id":"user_1","role":"viewer","permissions":1024,"invited_at":"2026-01-02T03:04:05Z","access_scope":"restricted","access":{"scope":"restricted","campaign_folder_ids":[],"campaign_ids":["c1"],"email_account_ids":["e1"]},"email":"a@b.com","name":"A B"}`, func(c *Client) error {
		var err error
		m, _, err = c.Organization.SetMemberAccess(ctx, "user_1", &MemberAccessParams{Scope: AccessScopeRestricted, CampaignIDs: []string{"c1"}, EmailAccountIDs: []string{"e1"}})
		return err
	})
	wantReq(t, got, "PUT", "/v1/organization/members/user_1/access", "", `{"scope":"restricted","campaign_ids":["c1"],"email_account_ids":["e1"]}`)
	if m.AccessScope != AccessScopeRestricted || m.Access == nil || m.Access.EmailAccountIDs[0] != "e1" {
		t.Fatalf("member = %+v", m)
	}

	var senders []SuggestedSender
	got = idCall(t, `{"data":[{"id":"e1","email":"s@x.com","name":"S","campaign_ids":["c1"]}]}`, func(c *Client) error {
		var err error
		senders, _, err = c.Organization.SuggestedSenders(ctx, &SuggestedSendersParams{CampaignIDs: []string{"c1", "c2"}, CampaignFolderIDs: []string{"f1"}})
		return err
	})
	wantReq(t, got, "GET", "/v1/organization/access/suggested-senders", "campaign_folder_ids=f1&campaign_ids=c1%2Cc2", "")
	if len(senders) != 1 || senders[0].CampaignIDs[0] != "c1" {
		t.Fatalf("senders = %+v", senders)
	}

	got = idCall(t, `{"data":null}`, func(c *Client) error {
		var err error
		senders, _, err = c.Organization.SuggestedSenders(ctx, nil)
		return err
	})
	wantReq(t, got, "GET", "/v1/organization/access/suggested-senders", "", "")
	if senders == nil {
		t.Fatal("null data must decode to an empty slice")
	}
}

func TestMemberUnknownAccessScopeDecodes(t *testing.T) {
	var m *Member
	idCall(t, `{"id":"m1","organization_id":"o1","user_id":"u","role":"x","permissions":0,"invited_at":"2026-01-02T03:04:05Z","access_scope":"future_scope","email":"","name":""}`, func(c *Client) error {
		var err error
		m, _, err = c.Organization.SetMemberAccess(context.Background(), "u", &MemberAccessParams{Scope: AccessScopeWorkspace})
		return err
	})
	if m.AccessScope != "future_scope" {
		t.Fatalf("member = %+v", m)
	}
}

func TestOrganizationTimezoneAndInvitationAccess(t *testing.T) {
	ctx := context.Background()
	tz := "Europe/Berlin"
	got := idCall(t, `{"id":"o1","name":"W","category":"test","owner_user_id":"u","timezone":"Europe/Berlin","created_at":"2026-01-02T03:04:05Z","updated_at":"2026-01-02T03:04:05Z","presence_show_online":true,"presence_show_activity":true,"product_description":"","icp_notes":"","voice_profile":"","inbox_agent_enabled":false,"assistant_shared_history":false}`, func(c *Client) error {
		o, _, err := c.Organization.Update(ctx, &OrganizationUpdateParams{Timezone: &tz})
		if err == nil && (o.Timezone != tz || o.Category != "test") {
			t.Errorf("org = %+v", o)
		}
		return err
	})
	wantReq(t, got, "PATCH", "/v1/organization/current", "", `{"timezone":"Europe/Berlin"}`)

	got = idCall(t, `{"id":"o1","name":"W","owner_user_id":"u","created_at":"2026-01-02T03:04:05Z","updated_at":"2026-01-02T03:04:05Z"}`, func(c *Client) error {
		_, _, err := c.Organization.Create(ctx, &OrganizationCreateParams{Name: "W", Timezone: tz})
		return err
	})
	wantReq(t, got, "POST", "/v1/organization", "", `{"name":"W","timezone":"Europe/Berlin"}`)

	got = idCall(t, `{"invitation":{"id":"i1","organization_id":"o1","email":"a@b.com","role":"viewer","permissions":1024,"invited_by":"u","expires_at":"2026-01-02T03:04:05Z","created_at":"2026-01-02T03:04:05Z","access":{"scope":"restricted","campaign_folder_ids":[],"campaign_ids":["c1"],"email_account_ids":[]},"expired":true}}`, func(c *Client) error {
		_, _, err := c.Organization.Invite(ctx, &InviteMemberParams{Email: "a@b.com", RoleIDs: []string{"r1"}, Access: &MemberAccessParams{Scope: AccessScopeRestricted, CampaignIDs: []string{"c1"}}})
		return err
	})
	wantReq(t, got, "POST", "/v1/organization/members/invite", "", `{"email":"a@b.com","role_ids":["r1"],"access":{"scope":"restricted","campaign_ids":["c1"]}}`)

	var inv []Invitation
	idCall(t, `{"data":[{"id":"i1","organization_id":"o1","email":"a@b.com","role":"viewer","permissions":0,"invited_by":"u","expires_at":"2026-01-02T03:04:05Z","created_at":"2026-01-02T03:04:05Z","access":{"scope":"restricted","campaign_folder_ids":[],"campaign_ids":["c1"],"email_account_ids":[]},"expired":true}]}`, func(c *Client) error {
		var err error
		inv, _, err = c.Organization.Invitations(ctx)
		return err
	})
	if inv[0].Access == nil || inv[0].Access.CampaignIDs[0] != "c1" || !inv[0].Lapsed {
		t.Fatalf("invitation = %+v", inv[0])
	}
}

// --- OAuth apps ---

func TestOAuthAppSuspensionAndDisabledStatus(t *testing.T) {
	var app *OAuthApp
	idCall(t, `{"id":"a1","organization_id":"o","created_by":"u","name":"N","description":"","logo_url":"","website_url":"","client_id":"wmcid_x","redirect_uris":[],"scopes":3,"allowed_webhook_domains":[],"webhook_url":"","webhook_events":[],"status":"disabled","is_public":false,"dynamically_registered":false,"suspended_at":"2026-01-02T03:04:05Z","suspended_reason":"abuse","created_at":"2026-01-02T03:04:05Z","updated_at":"2026-01-02T03:04:05Z"}`, func(c *Client) error {
		var err error
		app, _, err = c.OAuthApps.Get(context.Background(), "a1")
		return err
	})
	if app.Status != OAuthAppDisabled || app.SuspendedAt == nil || app.SuspendedReason != "abuse" || app.Usable() {
		t.Fatalf("app = %+v", app)
	}
	app.Status, app.SuspendedAt = OAuthAppActive, nil
	if !app.Usable() {
		t.Fatal("active, unsuspended app should be usable")
	}
}

func TestOAuthListWithAccess(t *testing.T) {
	var l *OAuthAppList
	got := idCall(t, `{"applications":null,"developer_access":{"blocked":true,"reason":"review"}}`, func(c *Client) error {
		var err error
		l, _, err = c.OAuthApps.ListWithAccess(context.Background())
		return err
	})
	wantReq(t, got, "GET", "/v1/oauth/applications", "", "")
	if !l.DeveloperAccess.Blocked || l.DeveloperAccess.Reason != "review" || l.Applications == nil {
		t.Fatalf("list = %+v", l)
	}
}

func TestOAuthLogoUploadsUseFileField(t *testing.T) {
	var field, path, method string
	c := accountServerFunc(t, func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("parse: %v", err)
			}
			for name := range r.MultipartForm.File {
				field = name
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"a1","logo_url":"https://cdn.example/logo.png","logo":"x"}`)
	})
	ctx := context.Background()
	file := func() *FileUpload {
		return &FileUpload{Filename: "logo.png", ContentType: "image/png", Content: strings.NewReader("png")}
	}

	url, _, err := c.OAuthApps.UploadLogo(ctx, file())
	if err != nil || url != "https://cdn.example/logo.png" {
		t.Fatalf("UploadLogo = %q, %v", url, err)
	}
	if field != "file" || path != "/v1/oauth/application-logo" || method != "POST" {
		t.Fatalf("UploadLogo sent %s %s field %q", method, path, field)
	}

	field = ""
	app, _, err := c.OAuthApps.SetLogo(ctx, "a1", file())
	if err != nil || app.LogoURL != "https://cdn.example/logo.png" {
		t.Fatalf("SetLogo = %+v, %v", app, err)
	}
	if field != "file" || path != "/v1/oauth/applications/a1/logo" || method != "POST" {
		t.Fatalf("SetLogo sent %s %s field %q", method, path, field)
	}

	app, _, err = c.OAuthApps.RemoveLogo(ctx, "a1")
	if err != nil || app.ID != "a1" || method != "DELETE" || path != "/v1/oauth/applications/a1/logo" {
		t.Fatalf("RemoveLogo = %+v, %v (%s %s)", app, err, method, path)
	}
}

func TestOAuthListingRoutes(t *testing.T) {
	ctx := context.Background()
	listing := `{"listing":{"application_id":"a1","organization_id":"o1","slug":"acme","tagline":"t","description":"d","category":"crm","install_url":"https://i","support_url":"https://s","privacy_url":"https://p","status":"featured","status_note":"n","status_at":"2026-01-02T03:04:05Z","submitted_at":"2026-01-02T03:04:05Z","created_at":"2026-01-02T03:04:05Z","updated_at":"2026-01-02T03:04:05Z"}}`

	var l *AppListing
	got := idCall(t, listing, func(c *Client) error {
		var err error
		l, _, err = c.OAuthApps.Listing(ctx, "a1")
		return err
	})
	wantReq(t, got, "GET", "/v1/oauth/applications/a1/listing", "", "")
	if l.Slug != "acme" || l.Status != AppListingFeatured || l.StatusAt == nil {
		t.Fatalf("listing = %+v", l)
	}

	_ = idCall(t, `{"listing":null}`, func(c *Client) error {
		var err error
		l, _, err = c.OAuthApps.Listing(ctx, "a1")
		return err
	})
	if l != nil {
		t.Fatalf("unpublished app should have a nil listing, got %+v", l)
	}

	got = idCall(t, listing, func(c *Client) error {
		_, _, err := c.OAuthApps.PublishListing(ctx, "a1", &AppListingParams{Slug: "acme", Tagline: "t", Description: "d", Category: "crm", InstallURL: "https://i", SupportURL: "https://s", PrivacyURL: "https://p"})
		return err
	})
	wantReq(t, got, "PUT", "/v1/oauth/applications/a1/listing", "", `{"slug":"acme","tagline":"t","description":"d","category":"crm","install_url":"https://i","support_url":"https://s","privacy_url":"https://p"}`)

	got = idCall(t, `{"deleted":true}`, func(c *Client) error {
		_, err := c.OAuthApps.UnpublishListing(ctx, "a1")
		return err
	})
	wantReq(t, got, "DELETE", "/v1/oauth/applications/a1/listing", "", "")
}

func TestWorkspaceAuthorizations(t *testing.T) {
	ctx := context.Background()
	var auths []WorkspaceAuthorization
	got := idCall(t, `{"authorizations":[{"application_id":"a1","name":"App","logo_url":"","website_url":"https://w","user_id":"u1","user_email":"m@x.com","user_name":"M","scopes":65,"authorized_at":"2026-01-02T03:04:05Z"}]}`, func(c *Client) error {
		var err error
		auths, _, err = c.OAuthApps.WorkspaceAuthorizations(ctx)
		return err
	})
	wantReq(t, got, "GET", "/v1/oauth/workspace-authorizations", "", "")
	if len(auths) != 1 || auths[0].UserEmail != "m@x.com" || auths[0].Scopes != 65 || auths[0].LastUsedAt != nil {
		t.Fatalf("auths = %+v", auths)
	}

	got = idCall(t, `{"revoked":true}`, func(c *Client) error {
		_, err := c.OAuthApps.RevokeWorkspaceAuthorization(ctx, "a1", "u1")
		return err
	})
	wantReq(t, got, "DELETE", "/v1/oauth/workspace-authorizations/a1/members/u1", "", "")

	idCall(t, `{}`, func(c *Client) error {
		var err error
		auths, _, err = c.OAuthApps.WorkspaceAuthorizations(ctx)
		return err
	})
	if auths == nil {
		t.Fatal("missing list must decode to an empty slice")
	}
}

// --- assistant ---

func TestAssistantToolPoliciesAndApproval(t *testing.T) {
	ctx := context.Background()
	var pol []ToolPolicy
	got := idCall(t, `{"data":[{"org_id":"o1","tool_name":"update_campaign","decision":"always_allow","created_by":"u1","created_by_name":"Ada","created_at":"2026-01-02T03:04:05Z"}]}`, func(c *Client) error {
		var err error
		pol, _, err = c.Assistant.ToolPolicies(ctx)
		return err
	})
	wantReq(t, got, "GET", "/v1/ai/tool-policies", "", "")
	if len(pol) != 1 || pol[0].ToolName != "update_campaign" || pol[0].Decision != ApprovalAlwaysAllow || pol[0].CreatedBy == nil {
		t.Fatalf("policies = %+v", pol)
	}

	got = idCall(t, `{"message":"policy revoked"}`, func(c *Client) error {
		_, err := c.Assistant.RevokeToolPolicy(ctx, "update_campaign")
		return err
	})
	wantReq(t, got, "DELETE", "/v1/ai/tool-policies/update_campaign", "", "")

	got = idCall(t, "data: {\"type\":\"done\"}\n\n", func(c *Client) error {
		_, err := c.Assistant.ApproveToolCall(ctx, "s1", "call_9", ApprovalApprove, nil)
		return err
	})
	wantReq(t, got, "POST", "/v1/ai/sessions/s1/approve", "", `{"decision":"approve","tool_call_id":"call_9"}`)
}

func TestAssistantApproveFindsPendingToolCall(t *testing.T) {
	var posted string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/ai/sessions/s1/messages":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"title":"t","turns":[],"free_model":false,"pending":{"message_id":"m","tool_call_id":"call_7","tool_name":"send_email","risk":"send","args":{"to":"x"},"arguments":"{\n  \"to\": \"x\"\n}","preview":{"from":"a@x.com","to":["b@y.com"],"subject":"Hi","body":"Hello"},"always_allow_offered":false}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/ai/sessions/s2/messages":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"title":"t","turns":[],"free_model":false,"pending":null}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/ai/sessions/s1/approve":
			b, _ := io.ReadAll(r.Body)
			posted = string(b)
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"type\":\"done\"}\n\n")
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatal(err)
	}

	var types []string
	_, err = c.Assistant.Approve(context.Background(), "s1", ApprovalDeny, func(ev *AgentEvent) bool {
		types = append(types, ev.Type)
		return true
	})
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if !jsonEqual(t, posted, `{"decision":"deny","tool_call_id":"call_7"}`) || len(types) != 1 || types[0] != AgentEventDone {
		t.Fatalf("posted %s, events %v", posted, types)
	}

	if _, err = c.Assistant.Approve(context.Background(), "s2", ApprovalApprove, nil); !errors.Is(err, ErrNoPendingApproval) {
		t.Fatalf("no pending approval: err = %v", err)
	}

	tr, _, err := c.Assistant.Transcript(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	p := tr.Pending
	if p.Preview == nil || p.Preview.Subject != "Hi" || p.Preview.To[0] != "b@y.com" || p.AlwaysAllowOffered || !strings.Contains(p.Arguments, `"to"`) {
		t.Fatalf("pending = %+v", p)
	}
}

func TestAgentEventApprovalCard(t *testing.T) {
	var ev AgentEvent
	if err := json.Unmarshal([]byte(`{"type":"approval_required","tool":"send_email","risk":"send","tool_call_id":"c1","arguments":"{}","arguments_truncated":true,"preview":{"from":"a","to":["b"],"subject":"s","body":"x","body_html":"<p>x</p>"},"always_allow_offered":true,"secrets":{"api_key":"k"},"brand_new_field":1}`), &ev); err != nil {
		t.Fatal(err)
	}
	if !ev.ArgumentsTruncated || !ev.AlwaysAllowOffered || ev.Preview.BodyHTML != "<p>x</p>" || ev.Secrets["api_key"] != "k" {
		t.Fatalf("event = %+v", ev)
	}
}

// --- billing ---

func TestPlanOffers(t *testing.T) {
	ctx := context.Background()
	var offers []PlanOffer
	got := idCall(t, `{"offers":[{"plan_id":"p1","name":"Partner","currency":"usd","monthly_usd":49,"yearly_usd":490,"monthly_available":true,"yearly_available":true,"available":true,"warmup_only":true,"expires_at":"2026-12-01T00:00:00Z"},{"plan_id":"p2","name":"Custom","currency":"usd","monthly_usd":99,"monthly_available":false,"yearly_available":false,"available":false,"warmup_only":false}]}`, func(c *Client) error {
		var err error
		offers, _, err = c.Billing.PlanOffers(ctx)
		return err
	})
	wantReq(t, got, "GET", "/v1/subscription/offers", "", "")
	if len(offers) != 2 || !offers[0].WarmupOnly || offers[0].ExpiresAt == nil || offers[1].ExpiresAt != nil || offers[1].YearlyUSD != 0 || offers[0].YearlyUSD != 490 {
		t.Fatalf("offers = %+v", offers)
	}

	var co *PlanOfferCheckout
	got = idCall(t, `{"checkout_url":"https://pay.example/c/1"}`, func(c *Client) error {
		var err error
		co, _, err = c.Billing.CheckoutPlanOffer(ctx, "p1", DurationYear)
		return err
	})
	wantReq(t, got, "POST", "/v1/subscription/offers/p1/checkout", "", `{"interval":"year"}`)
	if co.CheckoutURL == "" || co.Changed {
		t.Fatalf("checkout = %+v", co)
	}

	got = idCall(t, `{"changed":true}`, func(c *Client) error {
		var err error
		co, _, err = c.Billing.CheckoutPlanOffer(ctx, "p1", "")
		return err
	})
	wantReq(t, got, "POST", "/v1/subscription/offers/p1/checkout", "", `{}`)
	if !co.Changed || co.CheckoutURL != "" {
		t.Fatalf("checkout = %+v", co)
	}
}

func TestSubscriptionManagedGrantDecodes(t *testing.T) {
	var sub *Subscription
	idCall(t, `{"id":"s","user_id":"u","organization_id":"o","plan_id":"p","stripe_customer_id":"","status":"incomplete","cancel_at_period_end":false,"is_enterprise":false,"managed":true,"managed_at":"2026-01-02T03:04:05Z","managed_by":"admin","managed_reason":"design partner","managed_until":"2026-06-01T00:00:00Z","managed_plan_id":"p2","plan":{"id":"p","price":10,"price_yearly":100,"discounted_price":10,"duration":"month","savings":0,"public":true,"dedicated_workers":0,"monthly_credits":0,"referral_reward_percent":100,"max_contacts":1,"daily_emails":1,"ai_generation":false,"account_limit":1},"created_at":"2026-01-02T03:04:05Z","updated_at":"2026-01-02T03:04:05Z"}`, func(c *Client) error {
		var err error
		sub, _, err = c.Billing.Get(context.Background())
		return err
	})
	if !sub.Managed || sub.ManagedPlanID == nil || *sub.ManagedPlanID != "p2" || sub.ManagedUntil == nil || sub.Plan.PriceYearly == nil || *sub.Plan.PriceYearly != 100 {
		t.Fatalf("subscription = %+v", sub)
	}
}

// --- pool link and cloud link ---

func TestPoolLinkOfferAndCheckout(t *testing.T) {
	ctx := context.Background()
	var off *PoolLinkOffer
	got := idCall(t, `{"available":true,"monthly_available":true,"yearly_available":false,"monthly_usd":9,"yearly_usd":0,"currency":"usd"}`, func(c *Client) error {
		var err error
		off, _, err = c.PoolLink.Offer(ctx)
		return err
	})
	wantReq(t, got, "GET", "/v1/pool-link/offer", "", "")
	if !off.Available || off.MonthlyUSD != 9 || off.YearlyAvailable {
		t.Fatalf("offer = %+v", off)
	}

	var u string
	got = idCall(t, `{"checkout_url":"https://pay.example/c/2"}`, func(c *Client) error {
		var err error
		u, _, err = c.PoolLink.Checkout(ctx, DurationYear)
		return err
	})
	wantReq(t, got, "POST", "/v1/pool-link/checkout", "", `{"interval":"year"}`)
	if u != "https://pay.example/c/2" {
		t.Fatalf("url = %q", u)
	}
}

func TestPoolLinkStartSendsRemoteWorkspace(t *testing.T) {
	var grant *PoolLinkCodeGrant
	got := idCall(t, `{"workspace_scoped":true,"device_code":"dc","user_code":"ABCD1234","verification_url":"https://app/connect?code=ABCD1234","expires_in":900,"interval":5}`, func(c *Client) error {
		var err error
		grant, _, err = c.PoolLink.StartCode(context.Background(), &PoolLinkStartParams{RemoteOrganizationID: "r-org", InstanceName: "box", InstanceURL: "https://box", InstanceVersion: "1.0"})
		return err
	})
	wantReq(t, got, "POST", "/v1/pool-link/codes", "", `{"remote_organization_id":"r-org","instance_name":"box","instance_url":"https://box","instance_version":"1.0"}`)
	if !grant.WorkspaceScoped {
		t.Fatalf("grant = %+v", grant)
	}
}

func TestPoolLinkInstanceListDecodesNewFields(t *testing.T) {
	var l *PoolLinkInstanceList
	idCall(t, `{"data":[{"remote_organization_id":"r1","id":"i1","organization_id":"o1","name":"box","url":"https://box","version":"1","created_at":"2026-01-02T03:04:05Z","mailbox_count":3}],"plan":{"tier":"paid","mailbox_limit":null,"enrolled":3,"warming":2,"price_usd":9,"manage_url":"https://app/billing","warmup_entitled":true}}`, func(c *Client) error {
		var err error
		l, _, err = c.PoolLink.ListInstances(context.Background())
		return err
	})
	if l.Data[0].RemoteOrganizationID == nil || *l.Data[0].RemoteOrganizationID != "r1" || l.Plan.Warming != 2 || l.Plan.ManageURL == "" || l.Plan.MailboxLimit != nil {
		t.Fatalf("list = %+v", l)
	}
}

func TestCloudLinkStatusAndMailboxDecodeNewFields(t *testing.T) {
	ctx := context.Background()
	var st *CloudLinkStatus
	idCall(t, `{"legacy_connected":true,"connected":true,"link":{"organization_id":"o1","cloud_url":"https://cloud","instance_id":"i1","organization_name":"W","connected_at":"2026-01-02T03:04:05Z","disconnect_pending":true},"info":{"smtp_association_protocol":1,"managed_consent_protocol":1,"instance":{"id":"i1","organization_id":"o","name":"n","url":"u","version":"v","created_at":"2026-01-02T03:04:05Z","mailbox_count":0},"organization":{"id":"o","name":"W"},"plan":{"tier":"free","mailbox_limit":5,"enrolled":1,"warming":1,"price_usd":0,"warmup_entitled":true},"redirects":{"available":true,"host":"r.example","limit":3,"used":1},"self_execution":{"protocol":1,"granted":true,"expires_at":"2026-12-01T00:00:00Z"}},"reachable":true,"default_cloud_url":"https://api.example"}`, func(c *Client) error {
		var err error
		st, _, err = c.CloudLink.Status(ctx)
		return err
	})
	if !st.LegacyConnected || !st.Link.DisconnectPending || st.Link.OrganizationID == nil || *st.Link.OrganizationID != "o1" {
		t.Fatalf("link = %+v", st.Link)
	}
	info := st.Info
	if info.SMTPAssociationProtocol != 1 || info.Redirects == nil || info.Redirects.Limit != 3 || info.SelfExecution == nil || !info.SelfExecution.Granted || info.SelfExecution.ExpiresAt == nil {
		t.Fatalf("info = %+v", info)
	}

	var rows []CloudLinkMailbox
	idCall(t, `{"data":[{"id":"e1","email":"a@x.com","name":"A","provider":"smtp_imap","status":"active","enrolled":true,"managed":false,"legacy":false,"independent_smtp":true,"enrollment_state":"pending_remove","standing_observed_at":"2026-01-02T03:04:05Z","executor":"instance","commands_pending":2,"commands_failed":1,"cloud":{"remote_id":"e1","email_account_id":"c1","email":"a@x.com","name":"A","provider":"smtp_imap","status":"active","enrolled_at":"2026-01-02T03:04:05Z","managed":false,"sent_today":1,"sent_7d":2,"replied_7d":0,"spam_placed_7d":0,"auth_state":"ok","settings":{"base":2,"max":40,"increase":1,"reply_rate":30,"start_time":"09:00","end_time":"17:00","days":127,"timezone":"UTC"},"participation":{"mode":"future_mode","send":true,"receive":false,"shared_daily_limit":50},"consent_completed_at":"2026-01-02T03:04:05Z","executor":"instance","warmup":{"enabled":true,"paused":false,"started_at":"2026-01-02T03:04:05Z","current_volume":1,"target_volume":2,"max_volume":40,"reply_rate":30,"days_active":3,"partner_limit":{"reachable":4,"ramp_target":9},"send_failure":{"message":"boom","at":"2026-01-02T03:04:05Z","kind":"auth"},"next_send_at":"2026-01-02T05:04:05Z"},"health":{"state":"healthy","score":0.9,"spam_score":0,"pool_type":"premium","source":"cloud","partner_mailboxes_7d":4,"partner_domains_7d":3,"partner_organizations_7d":2,"received_7d":8,"senders_7d":5}}}]}`, func(c *Client) error {
		var err error
		rows, _, err = c.CloudLink.ListMailboxes(ctx)
		return err
	})
	r := rows[0]
	if !r.IndependentSMTP || r.EnrollmentState != "pending_remove" || r.StandingObservedAt == nil || r.Executor != PoolLinkExecutorInstance || r.CommandsPending != 2 || r.CommandsFailed != 1 {
		t.Fatalf("row = %+v", r)
	}
	cl := r.Cloud
	if cl.Participation == nil || cl.Participation.Mode != "future_mode" || *cl.Participation.SharedDailyLimit != 50 || cl.ConsentCompletedAt == nil || cl.Executor != "instance" {
		t.Fatalf("cloud state = %+v", cl)
	}
	if cl.Warmup.PartnerLimit.Reachable != 4 || cl.Warmup.SendFailure.Kind != "auth" || cl.Warmup.NextSendAt == nil {
		t.Fatalf("warmup = %+v", cl.Warmup)
	}
	if cl.Health.PoolType != "premium" || cl.Health.Source != "cloud" || cl.Health.Received7d != 8 || cl.Health.PartnerOrganizations7d != 2 {
		t.Fatalf("health = %+v", cl.Health)
	}
}

func TestCloudLinkNewRoutes(t *testing.T) {
	ctx := context.Background()

	got := idCall(t, ``, func(c *Client) error { _, err := c.CloudLink.DisconnectLegacy(ctx); return err })
	wantReq(t, got, "DELETE", "/v1/cloud-link", "legacy=true", "")

	got = idCall(t, `{"id":"e1"}`, func(c *Client) error {
		_, _, err := c.CloudLink.EnrollMailboxAs(ctx, "e1", PoolLinkExecutorInstance)
		return err
	})
	wantReq(t, got, "POST", "/v1/cloud-link/mailboxes/e1/enroll", "", `{"executor":"instance"}`)

	got = idCall(t, `{"id":"e1"}`, func(c *Client) error {
		_, _, err := c.CloudLink.EnrollMailboxAs(ctx, "e1", "")
		return err
	})
	wantReq(t, got, "POST", "/v1/cloud-link/mailboxes/e1/enroll", "", "")
	if got.body != "" {
		t.Fatalf("empty executor must send no body, sent %q", got.body)
	}

	limit := 40
	got = idCall(t, `{"id":"e1","enrolled":true}`, func(c *Client) error {
		_, _, err := c.CloudLink.SetMailboxParticipation(ctx, "e1", &DiagnosticParticipation{Mode: TestParticipationDiagnostic, Send: true, Receive: true, SharedDailyLimit: &limit})
		return err
	})
	wantReq(t, got, "PATCH", "/v1/cloud-link/mailboxes/e1/participation", "", `{"mode":"diagnostic","send":true,"receive":true,"shared_daily_limit":40}`)
}
