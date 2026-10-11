package warmbly

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

// tokenPairBody is models.Token as the server marshals it.
const tokenPairBody = `{
	"access_token": "at_new",
	"access_token_expires_at": "2026-10-11T10:15:00Z",
	"refresh_token": "rt_new",
	"refresh_token_expires_at": "2026-11-10T10:00:00Z"
}`

func TestAuthSessionSync(t *testing.T) {
	ctx := context.Background()
	runWireCases(t, []wireCase{
		{
			name: "ChangePassword returns the new session", response: tokenPairBody,
			call: func(c *Client) error {
				s, _, err := c.Auth.ChangePassword(ctx, "old", "new")
				if err == nil && (s == nil || s.AccessToken != "at_new" || s.RefreshToken != "rt_new") {
					err = errors.New("new session not decoded")
				}
				return err
			},
			method: "POST", path: "/v1/auth/me/password", body: `{"current_password":"old","new_password":"new"}`,
		},
		{
			name: "ChangePassword without a new session", response: ``,
			call: func(c *Client) error {
				s, _, err := c.Auth.ChangePassword(ctx, "old", "new")
				if err == nil && s != nil {
					err = errors.New("want nil when the server issued no session")
				}
				return err
			},
			method: "POST", path: "/v1/auth/me/password", body: `{"current_password":"old","new_password":"new"}`,
		},
		{
			name: "CompleteProductTour", response: `{"product_tour_completed_at": "2026-10-11T09:00:00Z"}`,
			call: func(c *Client) error {
				at, _, err := c.Auth.CompleteProductTour(ctx)
				if err == nil && !at.Equal(time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)) {
					err = errors.New("completion time not decoded")
				}
				return err
			},
			method: "PUT", path: "/v1/auth/me/product-tour",
		},
		{
			name: "Reauth with a password", response: `{"valid_for_seconds": 300}`,
			call: func(c *Client) error {
				r, _, err := c.Auth.Reauth(ctx, &ReauthParams{Password: "pw"})
				if err == nil && r.ValidForSeconds != 300 {
					err = errors.New("valid_for_seconds not decoded")
				}
				return err
			},
			method: "POST", path: "/v1/auth/reauth", body: `{"password":"pw"}`,
		},
		{
			name: "Reauth with a code", response: `{"valid_for_seconds": 300}`,
			call: func(c *Client) error {
				_, _, err := c.Auth.Reauth(ctx, &ReauthParams{Code: "123456"})
				return err
			},
			method: "POST", path: "/v1/auth/reauth", body: `{"code":"123456"}`,
		},
		{
			name: "RegenerateRecoveryCodes", response: `{"recovery_codes": ["aaaa-bbbb", "cccc-dddd"]}`,
			call: func(c *Client) error {
				r, _, err := c.Auth.RegenerateRecoveryCodes(ctx, "123456")
				if err == nil && len(r.RecoveryCodes) != 2 {
					err = errors.New("recovery codes not decoded")
				}
				return err
			},
			method: "POST", path: "/v1/auth/2fa/recovery-codes", body: `{"code":"123456"}`,
		},
		{
			name: "TwoFAStatus", response: `{"enabled": true, "confirmed_at": "2026-01-02T00:00:00Z", "recovery_codes_remaining": 3, "recovery_codes_total": 10}`,
			call: func(c *Client) error {
				st, _, err := c.Auth.TwoFAStatus(ctx)
				if err == nil && (!st.Enabled || st.ConfirmedAt == nil || st.RecoveryCodesRemaining != 3 || st.RecoveryCodesTotal != 10) {
					err = errors.New("status not decoded")
				}
				return err
			},
			method: "GET", path: "/v1/auth/2fa/status",
		},
		{
			name: "EnrollTwoFA", response: `{"secret": "JBSW", "otpauth_uri": "otpauth://totp/x", "issuer": "Warmbly", "account": "sam@acme.com", "algorithm": "SHA1", "digits": 6, "period": 30}`,
			call: func(c *Client) error {
				e, _, err := c.Auth.EnrollTwoFA(ctx)
				if err == nil && (e.Issuer != "Warmbly" || e.Digits != 6 || e.Period != 30 || e.Algorithm != "SHA1") {
					err = errors.New("enrollment not decoded")
				}
				return err
			},
			method: "POST", path: "/v1/auth/2fa/enroll/start",
		},
		{
			name:     "ExchangeSSO link required",
			response: `{"link_required": true, "pending_token": "pt_1", "expires_in": 600, "link_email": "sam@acme.com", "link_provider": "google"}`,
			call: func(c *Client) error {
				s, _, err := c.Auth.ExchangeSSO(ctx, "code", "bind")
				if err == nil && (!s.LinkRequired || s.PendingToken != "pt_1" || s.LinkEmail != "sam@acme.com" || s.LinkProvider != "google" || s.AccessToken != "") {
					err = errors.New("link challenge not decoded")
				}
				return err
			},
			method: "POST", path: "/v1/auth/sso/exchange", body: `{"code":"code","binding":"bind"}`,
		},
		{
			name: "LinkSSO", response: tokenPairBody,
			call: func(c *Client) error {
				s, _, err := c.Auth.LinkSSO(ctx, "pt_1", "pw")
				if err == nil && s.AccessToken != "at_new" {
					err = errors.New("session not decoded")
				}
				return err
			},
			method: "POST", path: "/v1/auth/sso/link", body: `{"pending_token":"pt_1","password":"pw"}`,
		},
		{
			name: "Login with a device token", response: `{"code_required": false, "token": ` + tokenPairBody + `}`,
			call: func(c *Client) error {
				st, _, err := c.Auth.Login(ctx, &LoginParams{Email: "sam@acme.com", Password: "pw", DeviceToken: "dt_1"})
				if err == nil && (st.Token == nil || st.Token.AccessToken != "at_new") {
					err = errors.New("one-step session not decoded")
				}
				return err
			},
			method: "POST", path: "/v1/auth/login", body: `{"email":"sam@acme.com","password":"pw","device_token":"dt_1"}`,
		},
		{
			name:     "LoginConfirm remembering the device",
			response: `{"access_token": "at_new", "access_token_expires_at": "2026-10-11T10:15:00Z", "refresh_token": "rt_new", "refresh_token_expires_at": "2026-11-10T10:00:00Z", "device_token": "dt_2"}`,
			call: func(c *Client) error {
				s, _, err := c.Auth.LoginConfirm(ctx, &ConfirmParams{Session: "s", Code: "123456", RememberDevice: true})
				if err == nil && s.DeviceToken != "dt_2" {
					err = errors.New("device token not decoded")
				}
				return err
			},
			method: "POST", path: "/v1/auth/login/confirm", body: `{"session":"s","code":"123456","remember_device":true}`,
		},
		{
			name: "Register with attribution", response: `{"session": "s", "code_required": true}`,
			call: func(c *Client) error {
				_, _, err := c.Auth.Register(ctx, &LoginParams{Email: "a@b.com", Password: "pw", Acquisition: &SignupAcquisition{UTMSource: "news", LandingPath: "/pricing"}})
				return err
			},
			method: "POST", path: "/v1/auth/register",
			body: `{"email":"a@b.com","password":"pw","acquisition":{"landing_path":"/pricing","utm_source":"news"}}`,
		},
		{
			name: "CLIAuthRequestIn",
			response: `{"id": "c1", "user_code": "ABCD-EFGH", "client_name": "Warmbly CLI", "hostname": "laptop", "cli_version": "1.0",
				"scopes": 7, "scope_names": ["emails:read", "campaigns:read", "contacts:read"],
				"granted_scopes": 3, "granted_scope_names": ["emails:read", "campaigns:read"],
				"status": "pending", "expires_at": "2026-10-11T10:15:00Z", "created_at": "2026-10-11T10:00:00Z"}`,
			call: func(c *Client) error {
				r, _, err := c.Auth.CLIAuthRequestIn(ctx, "ABCD-EFGH", "org_1")
				if err == nil && (r.GrantedScopes == nil || *r.GrantedScopes != 3 || len(r.GrantedScopeNames) != 2) {
					err = errors.New("granted scopes not decoded")
				}
				return err
			},
			method: "GET", path: "/v1/auth/cli/codes/ABCD-EFGH", rawQuery: "organization_id=org_1",
		},
		{
			name: "Me decodes the product tour and MFA flag",
			response: `{"id": "u1", "first_name": "Sam", "last_name": "Lee", "email": "sam@acme.com",
				"product_tour_completed_at": "2026-10-11T09:00:00Z", "session_mfa_verified": true,
				"folders": [], "tags": [], "categories": [], "created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z"}`,
			call: func(c *Client) error {
				u, _, err := c.Auth.Me(ctx)
				if err == nil && (u.ProductTourCompletedAt == nil || !u.SessionMFAVerified) {
					err = errors.New("profile fields not decoded")
				}
				return err
			},
			method: "GET", path: "/v1/auth/me",
		},
	})
}

func TestAuthReauthRequired(t *testing.T) {
	c := emailFixtureClient(t, http.StatusForbidden, `{"code":"reauth_required","message":"Confirm it is you before making this change."}`, nil)
	_, _, err := c.Auth.ApproveCLIAuth(context.Background(), "ABCD-EFGH", "org_1")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != ErrCodeReauthRequired {
		t.Fatalf("err = %v, want %q", err, ErrCodeReauthRequired)
	}
}
