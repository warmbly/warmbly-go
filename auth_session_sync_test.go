package warmbly

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

// authTokenPairBody is models.Token as the server marshals it.
const authTokenPairBody = `{
	"access_token": "at_new",
	"access_token_expires_at": "2026-10-11T10:15:00Z",
	"refresh_token": "rt_new",
	"refresh_token_expires_at": "2026-11-10T10:00:00Z"
}`

func TestAuthSessionFieldsSync(t *testing.T) {
	ctx := context.Background()
	runAuthWireCases(t, []authWireCase{
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
			name: "EnrollTwoFA", response: `{"secret": "JBSW", "otpauth_uri": "otpauth://totp/x", "issuer": "Warmbly", "account": "sam@acme.com", "algorithm": "SHA1", "digits": 6, "period": 30}`,
			call: func(c *Client) error {
				e, _, err := c.Auth.EnrollTwoFA(ctx)
				if err == nil && (e.Issuer != "Warmbly" || e.Account != "sam@acme.com" || e.Digits != 6 || e.Period != 30 || e.Algorithm != "SHA1") {
					err = errors.New("enrollment not decoded")
				}
				return err
			},
			method: "POST", path: "/v1/auth/2fa/enroll/start",
		},
		{
			name: "Login with a device token", response: `{"code_required": false, "token": ` + authTokenPairBody + `}`,
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
				"scopes": 7, "scope_names": ["read_emails", "read_campaigns", "read_contacts"],
				"granted_scopes": 3, "granted_scope_names": ["read_emails", "read_campaigns"],
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
			name: "CLIAuthRequest leaves granted scopes nil",
			response: `{"id": "c1", "user_code": "ABCD-EFGH", "scopes": 7, "scope_names": [], "status": "pending",
				"expires_at": "2026-10-11T10:15:00Z", "created_at": "2026-10-11T10:00:00Z"}`,
			call: func(c *Client) error {
				r, _, err := c.Auth.CLIAuthRequest(ctx, "ABCD-EFGH")
				if err == nil && r.GrantedScopes != nil {
					err = errors.New("want nil granted scopes without a workspace")
				}
				return err
			},
			method: "GET", path: "/v1/auth/cli/codes/ABCD-EFGH",
		},
		{
			name: "Me decodes the product tour",
			response: `{"id": "u1", "first_name": "Sam", "last_name": "Lee", "email": "sam@acme.com",
				"product_tour_completed_at": "2026-10-11T09:00:00Z",
				"created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z"}`,
			call: func(c *Client) error {
				u, _, err := c.Auth.Me(ctx)
				if err == nil && u.ProductTourCompletedAt == nil {
					err = errors.New("product tour not decoded")
				}
				return err
			},
			method: "GET", path: "/v1/auth/me",
		},
	})
}

func TestAuthCLIApproveRefusals(t *testing.T) {
	for _, code := range []string{ErrCodeReauthRequired, ErrCodeCLIAuthScopesRole} {
		c := emailFixtureClient(t, http.StatusForbidden, `{"code":"`+code+`","message":"refused"}`, nil)
		_, _, err := c.Auth.ApproveCLIAuth(context.Background(), "ABCD-EFGH", "org_1")
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Code != code {
			t.Fatalf("err = %v, want %q", err, code)
		}
	}
}

func TestAuthReauthLimitedIsTyped(t *testing.T) {
	c := emailFixtureClient(t, http.StatusBadRequest, `{"code":"reauth_limited","message":"Too many confirmation attempts."}`, nil)
	_, _, err := c.Auth.Reauth(context.Background(), "pw", "")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != ErrCodeReauthLimited {
		t.Fatalf("err = %v, want %q", err, ErrCodeReauthLimited)
	}
}

func TestUniboxRailLayoutDecode(t *testing.T) {
	const saved = `{"preferences": {"view": "unibox_rail", "columns": [], "layout": {
		"favorites": [{"key": "folder:inbox", "name": "Main"}],
		"hidden": ["folder:spam"],
		"order": {"mailboxes": ["mailbox:m2", "mailbox:m1"]},
		"section_order": ["favorites", "folders"]
	}, "updated_at": "2026-10-11T09:00:00Z"}}`
	c := emailFixtureClient(t, http.StatusOK, saved, nil)
	p, _, err := c.Auth.ViewPreferences(context.Background(), ViewNameUniboxRail)
	if err != nil {
		t.Fatal(err)
	}
	var rail UniboxRailLayout
	if err := json.Unmarshal(p.Layout, &rail); err != nil {
		t.Fatal(err)
	}
	if len(rail.Favorites) != 1 || rail.Favorites[0].Name != "Main" || rail.Hidden[0] != "folder:spam" ||
		len(rail.Order["mailboxes"]) != 2 || len(rail.SectionOrder) != 2 {
		t.Fatalf("rail = %+v", rail)
	}
}

func TestScopesRoundTrip(t *testing.T) {
	mask, unknown := PermissionsForScopes("read_emails READ_CAMPAIGNS", ScopeWebhooks, "campaigns:read")
	if mask != PermReadEmails|PermReadCampaigns|PermWebhooks {
		t.Fatalf("mask = %d", mask)
	}
	if len(unknown) != 1 || unknown[0] != "campaigns:read" {
		t.Fatalf("unknown = %v", unknown)
	}
	got := ScopesForPermissions(mask)
	want := []string{ScopeReadCampaigns, ScopeReadEmails, ScopeWebhooks}
	if len(got) != len(want) {
		t.Fatalf("scopes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("scopes = %v, want %v", got, want)
		}
	}
	if ScopesForPermissions(PermFullAccess)[0] == "" || len(ScopesForPermissions(PermFullAccess)) != len(scopeBits) {
		t.Fatal("full access does not expand to every scope")
	}
}

func TestAPIPermissionsForMemberDelegation(t *testing.T) {
	if APIPermissionsForMember(0, true) != PermFullAccess {
		t.Fatal("owner should delegate everything")
	}
	got := APIPermissionsForMember(OrgPermViewContacts, false)
	if got&PermReadContacts == 0 || got&PermReadCRM == 0 || got&PermRealtimeSubscribe == 0 {
		t.Fatalf("view-contacts mask = %d", got)
	}
	if got&PermAIResearch != 0 || got&PermWriteContacts != 0 {
		t.Fatalf("view-contacts mask grants too much: %d", got)
	}
	if PermAppGrantable&PermAPIKeys != 0 {
		t.Fatal("apps must never be granted api_keys")
	}
}
