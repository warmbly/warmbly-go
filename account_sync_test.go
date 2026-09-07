package warmbly

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// accountServer answers every request with one fixed body, for the decode
// tests below. routingClient cannot stand in for it: it serves a single
// envelope shaped for the list endpoints, which says nothing about whether a
// concrete payload lands in the right fields.
func accountServer(t *testing.T, body string, got *recordedRequest) *Client {
	t.Helper()
	return accountServerFunc(t, func(w http.ResponseWriter, r *http.Request) {
		if got != nil {
			got.method = r.Method
			got.path = r.URL.Path
			got.rawQuery = r.URL.RawQuery
			raw, _ := io.ReadAll(r.Body)
			got.body = string(raw)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	})
}

func accountServerFunc(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// TestAccountSyncRouting asserts the method and path of every account,
// workspace, billing, API-key, meta and website-tracking route this SDK
// reaches. A path typo would otherwise only surface as a 404 at runtime.
func TestAccountSyncRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()

	cases := []struct {
		name       string
		call       func() error
		wantMethod string
		wantPath   string
	}{
		// --- sign-in and deployment capabilities ---
		{"auth.Config", func() error { _, _, e := c.Auth.Config(ctx); return e }, "GET", "/v1/auth/config"},
		{"auth.Instance", func() error { _, _, e := c.Auth.Instance(ctx); return e }, "GET", "/v1/auth/instance"},
		{"auth.Providers", func() error { _, _, e := c.Auth.Providers(ctx); return e }, "GET", "/v1/auth/providers"},
		{"auth.Setup", func() error { _, _, e := c.Auth.Setup(ctx, &SetupParams{Token: "st_1"}); return e }, "POST", "/v1/auth/setup"},
		{"auth.Login", func() error { _, _, e := c.Auth.Login(ctx, &LoginParams{Email: "a@b.com"}); return e }, "POST", "/v1/auth/login"},
		{"auth.LoginConfirm", func() error { _, _, e := c.Auth.LoginConfirm(ctx, &ConfirmParams{Session: "s_1"}); return e }, "POST", "/v1/auth/login/confirm"},
		{"auth.Register", func() error { _, _, e := c.Auth.Register(ctx, &LoginParams{Email: "a@b.com"}); return e }, "POST", "/v1/auth/register"},
		{"auth.RegisterConfirm", func() error { _, _, e := c.Auth.RegisterConfirm(ctx, &ConfirmParams{Session: "s_1"}); return e }, "POST", "/v1/auth/register/confirm"},
		{"auth.ResetPassword", func() error { _, e := c.Auth.ResetPassword(ctx, "a@b.com", ""); return e }, "POST", "/v1/auth/reset-password"},
		{"auth.ResetPasswordConfirm", func() error { _, e := c.Auth.ResetPasswordConfirm(ctx, "s_1", "pw", ""); return e }, "POST", "/v1/auth/reset-password/confirm"},
		{"auth.Refresh", func() error { _, _, e := c.Auth.Refresh(ctx, "rt_1"); return e }, "POST", "/v1/auth/refresh"},
		{"auth.LoginWithApple", func() error { _, _, e := c.Auth.LoginWithApple(ctx, "tok"); return e }, "POST", "/v1/auth/apple"},
		{"auth.LoginWithGoogle", func() error { _, _, e := c.Auth.LoginWithGoogle(ctx, "tok"); return e }, "POST", "/v1/auth/google"},

		// --- browser SSO ---
		{"auth.BeginSSO(oidc)", func() error { _, _, e := c.Auth.BeginSSO(ctx, SSOProviderOIDC); return e }, "POST", "/v1/auth/oidc/begin"},
		{"auth.BeginSSO(google)", func() error { _, _, e := c.Auth.BeginSSO(ctx, SSOProviderGoogle); return e }, "POST", "/v1/auth/google/begin"},
		{"auth.BeginSSO(apple)", func() error { _, _, e := c.Auth.BeginSSO(ctx, SSOProviderApple); return e }, "POST", "/v1/auth/apple/begin"},
		{"auth.ExchangeSSO", func() error { _, _, e := c.Auth.ExchangeSSO(ctx, "hc_1", "bind"); return e }, "POST", "/v1/auth/sso/exchange"},

		// --- CLI device flow ---
		{"auth.StartCLIAuth", func() error { _, _, e := c.Auth.StartCLIAuth(ctx, nil); return e }, "POST", "/v1/auth/cli/code"},
		{"auth.PollCLIAuth", func() error { _, _, e := c.Auth.PollCLIAuth(ctx, "dc_1"); return e }, "POST", "/v1/auth/cli/poll"},
		{"auth.CLIAuthRequest", func() error { _, _, e := c.Auth.CLIAuthRequest(ctx, "ABCD-1234"); return e }, "GET", "/v1/auth/cli/codes/ABCD-1234"},
		{"auth.ApproveCLIAuth", func() error { _, _, e := c.Auth.ApproveCLIAuth(ctx, "ABCD-1234", "org_1"); return e }, "POST", "/v1/auth/cli/codes/ABCD-1234/approve"},
		{"auth.DenyCLIAuth", func() error { _, e := c.Auth.DenyCLIAuth(ctx, "ABCD-1234"); return e }, "POST", "/v1/auth/cli/codes/ABCD-1234/deny"},

		// --- sessions ---
		{"auth.Logout", func() error { _, e := c.Auth.Logout(ctx); return e }, "POST", "/v1/auth/logout"},
		{"auth.LogoutAll", func() error { _, e := c.Auth.LogoutAll(ctx); return e }, "POST", "/v1/auth/logout-all"},
		{"auth.RevokeSession", func() error { _, e := c.Auth.RevokeSession(ctx, "sess_1"); return e }, "DELETE", "/v1/auth/sessions/sess_1"},
		{"auth.RevokeOtherSessions", func() error { _, e := c.Auth.RevokeOtherSessions(ctx); return e }, "DELETE", "/v1/auth/sessions"},

		// --- profile ---
		{"auth.UpdateProfile", func() error { _, _, e := c.Auth.UpdateProfile(ctx, &ProfileUpdateParams{FirstName: "Ada"}); return e }, "PATCH", "/v1/auth/me"},
		{"auth.CompleteOnboarding", func() error {
			_, _, e := c.Auth.CompleteOnboarding(ctx, &OnboardingParams{FirstName: "Ada", LastName: "L"})
			return e
		}, "PATCH", "/v1/auth/me/onboarding"},
		{"auth.UploadAvatar", func() error {
			_, _, e := c.Auth.UploadAvatar(ctx, &FileUpload{Filename: "a.png", Content: strings.NewReader("x")})
			return e
		}, "POST", "/v1/auth/me/avatar"},
		{"auth.DeleteAvatar", func() error { _, e := c.Auth.DeleteAvatar(ctx); return e }, "DELETE", "/v1/auth/me/avatar"},
		{"auth.ChangePassword", func() error { _, e := c.Auth.ChangePassword(ctx, "old", "new"); return e }, "POST", "/v1/auth/me/password"},
		{"auth.SetUndoSendSeconds", func() error { _, e := c.Auth.SetUndoSendSeconds(ctx, 30); return e }, "PUT", "/v1/auth/me/send-preferences"},

		// --- two-factor ---
		{"auth.EnrollTwoFA", func() error { _, _, e := c.Auth.EnrollTwoFA(ctx); return e }, "POST", "/v1/auth/2fa/enroll/start"},
		{"auth.ConfirmTwoFA", func() error { _, _, e := c.Auth.ConfirmTwoFA(ctx, "123456"); return e }, "POST", "/v1/auth/2fa/enroll/confirm"},
		{"auth.DisableTwoFA", func() error { _, e := c.Auth.DisableTwoFA(ctx, "123456"); return e }, "DELETE", "/v1/auth/2fa"},
		{"auth.VerifyTwoFA", func() error { _, _, e := c.Auth.VerifyTwoFA(ctx, "pt_1", "123456"); return e }, "POST", "/v1/auth/2fa/verify"},

		// --- passkeys ---
		{"auth.Passkeys", func() error { _, _, e := c.Auth.Passkeys(ctx); return e }, "GET", "/v1/auth/passkey/credentials"},
		{"auth.BeginPasskeyLogin", func() error { _, _, e := c.Auth.BeginPasskeyLogin(ctx); return e }, "POST", "/v1/auth/passkey/login/begin"},
		{"auth.FinishPasskeyLogin", func() error {
			_, _, e := c.Auth.FinishPasskeyLogin(ctx, "s_1", json.RawMessage(`{}`))
			return e
		}, "POST", "/v1/auth/passkey/login/finish"},
		{"auth.BeginPasskeyRegistration", func() error { _, _, e := c.Auth.BeginPasskeyRegistration(ctx); return e }, "POST", "/v1/auth/passkey/register/begin"},
		{"auth.FinishPasskeyRegistration", func() error {
			_, _, e := c.Auth.FinishPasskeyRegistration(ctx, "laptop", json.RawMessage(`{}`))
			return e
		}, "POST", "/v1/auth/passkey/register/finish"},
		{"auth.RenamePasskey", func() error { _, _, e := c.Auth.RenamePasskey(ctx, "pk_1", "laptop"); return e }, "PATCH", "/v1/auth/passkey/credentials/pk_1"},
		{"auth.DeletePasskey", func() error { _, e := c.Auth.DeletePasskey(ctx, "pk_1"); return e }, "DELETE", "/v1/auth/passkey/credentials/pk_1"},

		// --- notifications and devices ---
		{"auth.NotificationPreferences", func() error { _, _, e := c.Auth.NotificationPreferences(ctx); return e }, "GET", "/v1/auth/me/notification-preferences"},
		{"auth.UpdateNotificationPreferences", func() error {
			_, _, e := c.Auth.UpdateNotificationPreferences(ctx, &NotificationPreferences{EmailDigestMinutes: 30})
			return e
		}, "PUT", "/v1/auth/me/notification-preferences"},
		{"auth.MarkNotificationRead", func() error { _, e := c.Auth.MarkNotificationRead(ctx, "n_1"); return e }, "POST", "/v1/auth/me/notifications/n_1/read"},
		{"auth.MarkAllNotificationsRead", func() error { _, e := c.Auth.MarkAllNotificationsRead(ctx); return e }, "PUT", "/v1/auth/me/notifications"},
		{"auth.RegisterDeviceToken", func() error { _, e := c.Auth.RegisterDeviceToken(ctx, "abcdef0123456789", "ios", ""); return e }, "POST", "/v1/auth/me/device-tokens"},
		{"auth.DeleteDeviceToken", func() error { _, e := c.Auth.DeleteDeviceToken(ctx, "abcdef0123456789"); return e }, "DELETE", "/v1/auth/me/device-tokens/abcdef0123456789"},

		// --- account danger zone (outside the /auth group) ---
		{"auth.DangerZone", func() error { _, _, e := c.Auth.DangerZone(ctx); return e }, "GET", "/v1/me/danger-zone"},
		{"auth.ScheduleDeletion", func() error {
			_, _, e := c.Auth.ScheduleDeletion(ctx, &ScheduleDeletionParams{Confirmation: "a@b.com"})
			return e
		}, "POST", "/v1/me/danger-zone/delete"},
		{"auth.CancelDeletion", func() error { _, e := c.Auth.CancelDeletion(ctx, "changed my mind"); return e }, "DELETE", "/v1/me/danger-zone/delete"},

		// --- workspace ---
		{"organization.Create", func() error { _, _, e := c.Organization.Create(ctx, &OrganizationCreateParams{Name: "Acme"}); return e }, "POST", "/v1/organization"},
		{"organization.List", func() error { _, _, e := c.Organization.List(ctx); return e }, "GET", "/v1/organization"},
		{"organization.Switch", func() error { _, e := c.Organization.Switch(ctx, "org_1"); return e }, "POST", "/v1/organization/switch/org_1"},
		{"organization.Update", func() error {
			_, _, e := c.Organization.Update(ctx, &OrganizationUpdateParams{Name: String("Acme")})
			return e
		}, "PATCH", "/v1/organization/current"},
		{"organization.Risk", func() error { _, _, e := c.Organization.Risk(ctx); return e }, "GET", "/v1/organization/current/risk"},
		{"organization.UploadAvatar", func() error {
			_, _, e := c.Organization.UploadAvatar(ctx, &FileUpload{Filename: "logo.png", Content: strings.NewReader("x")})
			return e
		}, "POST", "/v1/organization/avatar"},
		{"organization.DeleteAvatar", func() error { _, e := c.Organization.DeleteAvatar(ctx); return e }, "DELETE", "/v1/organization/avatar"},
		{"organization.Members", func() error { _, _, e := c.Organization.Members(ctx); return e }, "GET", "/v1/organization/members"},
		{"organization.Invite", func() error {
			_, _, e := c.Organization.Invite(ctx, &InviteMemberParams{Email: "a@b.com", RoleIDs: []string{"r_1"}})
			return e
		}, "POST", "/v1/organization/members/invite"},
		{"organization.RemoveMember", func() error { _, e := c.Organization.RemoveMember(ctx, "m_1"); return e }, "DELETE", "/v1/organization/members/m_1"},
		{"organization.TransferOwnership", func() error { _, e := c.Organization.TransferOwnership(ctx, "u_2"); return e }, "POST", "/v1/organization/transfer-ownership"},
		{"organization.DeleteRole", func() error { _, e := c.Organization.DeleteRole(ctx, "r_1"); return e }, "DELETE", "/v1/organization/roles/r_1"},
		{"organization.Invitations", func() error { _, _, e := c.Organization.Invitations(ctx); return e }, "GET", "/v1/organization/invitations"},
		{"organization.CancelInvitation", func() error { _, e := c.Organization.CancelInvitation(ctx, "inv_1"); return e }, "DELETE", "/v1/organization/invitations/inv_1"},
		{"organization.InvitationLink", func() error { _, _, e := c.Organization.InvitationLink(ctx, "inv_1"); return e }, "GET", "/v1/organization/invitations/inv_1/link"},
		{"organization.PreviewInvitation", func() error { _, _, e := c.Organization.PreviewInvitation(ctx, "tok_1"); return e }, "GET", "/v1/invitations/lookup"},
		{"organization.AcceptInvitation", func() error { _, _, e := c.Organization.AcceptInvitation(ctx, "tok_1"); return e }, "POST", "/v1/invitations/accept"},
		{"organization.RequestLimitIncrease", func() error {
			_, _, e := c.Organization.RequestLimitIncrease(ctx, "org_1", &LimitRequestParams{Field: LimitFieldMaxEmailAccounts, Requested: 25})
			return e
		}, "POST", "/v1/organization/org_1/limit-requests"},
		{"organization.ScheduleDeletion", func() error {
			_, _, e := c.Organization.ScheduleDeletion(ctx, &ScheduleDeletionParams{Confirmation: "Acme"})
			return e
		}, "POST", "/v1/organization/current/danger-zone/delete"},
		{"organization.CancelDeletion", func() error { _, e := c.Organization.CancelDeletion(ctx, "oops"); return e }, "DELETE", "/v1/organization/current/danger-zone/delete"},

		// --- workspace archives ---
		{"organization.TransferGroups", func() error { _, _, e := c.Organization.TransferGroups(ctx); return e }, "GET", "/v1/organization/current/transfer/groups"},
		{"organization.CreateExport", func() error { _, _, e := c.Organization.CreateExport(ctx, nil); return e }, "POST", "/v1/organization/current/export"},
		{"organization.Exports", func() error { _, _, e := c.Organization.Exports(ctx); return e }, "GET", "/v1/organization/current/export"},
		{"organization.Export", func() error { _, _, e := c.Organization.Export(ctx, "ex_1"); return e }, "GET", "/v1/organization/current/export/ex_1"},
		{"organization.DeleteExport", func() error { _, e := c.Organization.DeleteExport(ctx, "ex_1"); return e }, "DELETE", "/v1/organization/current/export/ex_1"},
		{"organization.PreflightImport", func() error {
			_, _, e := c.Organization.PreflightImport(ctx, &FileUpload{Filename: "a.zip", Content: strings.NewReader("x")}, "pw")
			return e
		}, "POST", "/v1/organization/current/import/preflight"},
		{"organization.CreateImport", func() error {
			_, _, e := c.Organization.CreateImport(ctx, &FileUpload{Filename: "a.zip", Content: strings.NewReader("x")}, nil)
			return e
		}, "POST", "/v1/organization/current/import"},
		{"organization.Imports", func() error { _, _, e := c.Organization.Imports(ctx); return e }, "GET", "/v1/organization/current/import"},
		{"organization.Import", func() error { _, _, e := c.Organization.Import(ctx, "im_1"); return e }, "GET", "/v1/organization/current/import/im_1"},

		// --- billing ---
		{"billing.Limits", func() error { _, _, e := c.Billing.Limits(ctx); return e }, "GET", "/v1/subscription/limits"},
		{"billing.Trial", func() error { _, _, e := c.Billing.Trial(ctx); return e }, "GET", "/v1/subscription/trial"},
		{"billing.Features", func() error { _, _, e := c.Billing.Features(ctx); return e }, "GET", "/v1/subscription/features"},
		{"billing.Checkout", func() error {
			_, _, e := c.Billing.Checkout(ctx, &CheckoutParams{PriceID: "price_1"})
			return e
		}, "POST", "/v1/subscription/checkout"},
		{"billing.Portal", func() error { _, _, e := c.Billing.Portal(ctx, "https://acme.test/back"); return e }, "POST", "/v1/subscription/portal"},
		{"billing.Cancel", func() error { _, e := c.Billing.Cancel(ctx, true); return e }, "POST", "/v1/subscription/cancel"},
		{"billing.ChangePlan", func() error {
			_, _, e := c.Billing.ChangePlan(ctx, &ChangePlanParams{PlanID: "plan_1"})
			return e
		}, "POST", "/v1/subscription/change-plan"},
		{"billing.PreviewPlanChange", func() error { _, _, e := c.Billing.PreviewPlanChange(ctx, "plan_2"); return e }, "GET", "/v1/subscription/preview-change"},
		{"billing.ValidateDiscount", func() error { _, _, e := c.Billing.ValidateDiscount(ctx, "SAVE20", ""); return e }, "POST", "/v1/subscription/discount/validate"},
		{"billing.AppliedDiscounts", func() error { _, _, e := c.Billing.AppliedDiscounts(ctx); return e }, "GET", "/v1/subscription/discounts"},
		{"billing.EnterpriseInquiry", func() error {
			_, _, e := c.Billing.EnterpriseInquiry(ctx, &EnterpriseInquiryParams{CompanyName: "Acme"})
			return e
		}, "POST", "/v1/subscription/enterprise-inquiry"},
		{"billing.CreditTransactions", func() error { _, e := c.Billing.CreditTransactions(ctx, nil); return e }, "GET", "/v1/subscription/credits/transactions"},
		{"billing.BuyCredits", func() error {
			_, _, e := c.Billing.BuyCredits(ctx, "small", "https://a.test/ok", "https://a.test/no")
			return e
		}, "POST", "/v1/subscription/credits/checkout"},
		{"billing.CreditSettings", func() error { _, _, e := c.Billing.CreditSettings(ctx); return e }, "GET", "/v1/subscription/credits/settings"},
		{"billing.UpdateCreditSettings", func() error {
			_, _, e := c.Billing.UpdateCreditSettings(ctx, &CreditSettingsParams{AutoTopupPack: "small"})
			return e
		}, "PATCH", "/v1/subscription/credits/settings"},
		{"billing.EnsureReferralCode", func() error { _, _, e := c.Billing.EnsureReferralCode(ctx); return e }, "POST", "/v1/subscription/referral"},
		{"billing.ReferralAttributions", func() error { _, e := c.Billing.ReferralAttributions(ctx, nil); return e }, "GET", "/v1/subscription/referral/attributions"},
		{"billing.ReferralEarnings", func() error { _, e := c.Billing.ReferralEarnings(ctx, nil); return e }, "GET", "/v1/subscription/referral/earnings"},

		// --- API keys ---
		{"apikeys.Revoke", func() error { _, e := c.APIKeys.Revoke(ctx, "k_1", "rotated"); return e }, "DELETE", "/v1/api-keys/k_1"},
		{"apikeys.RevokeSelf", func() error { _, e := c.APIKeys.RevokeSelf(ctx, "logout"); return e }, "DELETE", "/v1/api-keys/self"},
		{"apikeys.UsageSummary", func() error { _, _, e := c.APIKeys.UsageSummary(ctx); return e }, "GET", "/v1/api-keys/usage/summary"},
		{"apikeys.Analytics", func() error { _, _, e := c.APIKeys.Analytics(ctx, "k_1", nil); return e }, "GET", "/v1/api-keys/k_1/analytics"},

		// --- meta ---
		{"meta.Realtime", func() error { _, _, e := c.Meta.Realtime(ctx); return e }, "GET", "/v1/realtime/info"},
		{"meta.GatewayTicket", func() error { _, _, e := c.Meta.GatewayTicket(ctx); return e }, "POST", "/v1/getaway"},

		// --- website tracking ---
		{"websitetracking.Settings", func() error { _, _, e := c.WebsiteTracking.Settings(ctx); return e }, "GET", "/v1/website-tracking/settings"},
		{"websitetracking.UpdateSettings", func() error {
			_, _, e := c.WebsiteTracking.UpdateSettings(ctx, &WebsiteTrackingUpdateParams{Enabled: Bool(true)})
			return e
		}, "PATCH", "/v1/website-tracking/settings"},
		{"websitetracking.RotateKey", func() error { _, _, e := c.WebsiteTracking.RotateKey(ctx); return e }, "POST", "/v1/website-tracking/settings/rotate-key"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got = recordedRequest{}
			if err := tc.call(); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if got.method != tc.wantMethod || got.path != tc.wantPath {
				t.Errorf("%s = %s %s, want %s %s", tc.name, got.method, got.path, tc.wantMethod, tc.wantPath)
			}
		})
	}
}

// TestAccountSyncRoutingPermissionObjects covers the routes whose payload
// carries a "permissions" field. The shared routing fixture answers with a
// permissions array (for the API scope catalog), which cannot decode into the
// bitmask these carry, so they get a server of their own.
func TestAccountSyncRoutingPermissionObjects(t *testing.T) {
	var got recordedRequest
	c := accountServer(t, `{}`, &got)
	ctx := context.Background()

	cases := []struct {
		name       string
		call       func() error
		wantMethod string
		wantPath   string
	}{
		{"organization.UpdateMember", func() error {
			_, _, e := c.Organization.UpdateMember(ctx, "m_1", &UpdateMemberParams{RoleIDs: []string{"r_1"}})
			return e
		}, "PATCH", "/v1/organization/members/m_1"},
		{"organization.CreateRole", func() error {
			_, _, e := c.Organization.CreateRole(ctx, &RoleCreateParams{Name: "Ops", Permissions: 3})
			return e
		}, "POST", "/v1/organization/roles"},
		{"organization.UpdateRole", func() error {
			_, _, e := c.Organization.UpdateRole(ctx, "r_1", &RoleUpdateParams{Name: String("Ops")})
			return e
		}, "PATCH", "/v1/organization/roles/r_1"},
		{"apikeys.Get", func() error { _, _, e := c.APIKeys.Get(ctx, "k_1"); return e }, "GET", "/v1/api-keys/k_1"},
		{"apikeys.Create", func() error {
			_, _, e := c.APIKeys.Create(ctx, &APIKeyCreateParams{Name: "ci", Permissions: PermReadOnly})
			return e
		}, "POST", "/v1/api-keys"},
		{"apikeys.Update", func() error {
			_, _, e := c.APIKeys.Update(ctx, "k_1", &APIKeyUpdateParams{Permissions: func() *uint64 { p := PermFullAccess; return &p }()})
			return e
		}, "PATCH", "/v1/api-keys/k_1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got = recordedRequest{}
			if err := tc.call(); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if got.method != tc.wantMethod || got.path != tc.wantPath {
				t.Errorf("%s = %s %s, want %s %s", tc.name, got.method, got.path, tc.wantMethod, tc.wantPath)
			}
		})
	}
}

// TestAccountSyncQueries pins the query strings the server actually reads.
func TestAccountSyncQueries(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
		want string
	}{
		{"apikeys.Revoke reason", func() error { _, e := c.APIKeys.Revoke(ctx, "k_1", "rotated"); return e }, "reason=rotated"},
		{"apikeys.RevokeSelf reason", func() error { _, e := c.APIKeys.RevokeSelf(ctx, "cli logout"); return e }, "reason=cli+logout"},
		{"apikeys.Analytics window", func() error {
			_, _, e := c.APIKeys.Analytics(ctx, "k_1", &APIKeyAnalyticsParams{
				From:     time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
				To:       time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC),
				Interval: IntervalHour,
			})
			return e
		}, "from=2026-08-01T00%3A00%3A00Z&interval=hour&to=2026-08-02T00%3A00%3A00Z"},
		{"billing.PreviewPlanChange", func() error { _, _, e := c.Billing.PreviewPlanChange(ctx, "plan_2"); return e }, "new_plan_id=plan_2"},
		{"billing.CreditUsage days", func() error { _, _, e := c.Billing.CreditUsage(ctx, 14); return e }, "days=14"},
		{"organization.PreviewInvitation", func() error { _, _, e := c.Organization.PreviewInvitation(ctx, "tok 1"); return e }, "token=tok+1"},
		{"apikeys.Logs limit", func() error { _, e := c.APIKeys.Logs(ctx, "k_1", &ListOptions{Limit: 200}); return e }, "limit=200"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got = recordedRequest{}
			if err := tc.call(); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if got.rawQuery != tc.want {
				t.Errorf("%s query = %q, want %q", tc.name, got.rawQuery, tc.want)
			}
		})
	}
}

// TestAccountSyncBodies pins the request bodies for the calls whose wire shape
// is built inside the SDK rather than handed in by the caller. Each of these is
// a field name the server binds on and would 400 without.
func TestAccountSyncBodies(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
		want string
	}{
		{"auth.ExchangeSSO", func() error { _, _, e := c.Auth.ExchangeSSO(ctx, "hc_1", "bind_1"); return e },
			`{"code":"hc_1","binding":"bind_1"}`},
		{"auth.PollCLIAuth", func() error { _, _, e := c.Auth.PollCLIAuth(ctx, "dc_1"); return e },
			`{"device_code":"dc_1"}`},
		{"auth.ApproveCLIAuth", func() error { _, _, e := c.Auth.ApproveCLIAuth(ctx, "ABCD-1234", "org_1"); return e },
			`{"organization_id":"org_1"}`},
		{"auth.LoginWithApple", func() error { _, _, e := c.Auth.LoginWithApple(ctx, "at_1"); return e },
			`{"id_token":"at_1"}`},
		{"auth.SetUndoSendSeconds", func() error { _, e := c.Auth.SetUndoSendSeconds(ctx, 15); return e },
			`{"undo_send_seconds":15}`},
		{"auth.RegisterDeviceToken", func() error {
			_, e := c.Auth.RegisterDeviceToken(ctx, "abcdef0123456789", "ios", "development")
			return e
		}, `{"token":"abcdef0123456789","platform":"ios","environment":"development"}`},
		{"auth.CancelDeletion", func() error { _, e := c.Auth.CancelDeletion(ctx, "stay"); return e },
			`{"reason":"stay"}`},
		// The cancel route binds a JSON body: a bodyless POST is a 400 there,
		// and false is how a scheduled cancellation is undone.
		{"billing.Cancel(true)", func() error { _, e := c.Billing.Cancel(ctx, true); return e },
			`{"cancel_at_period_end":true}`},
		{"billing.Cancel(false)", func() error { _, e := c.Billing.Cancel(ctx, false); return e },
			`{"cancel_at_period_end":false}`},
		{"billing.Portal", func() error { _, _, e := c.Billing.Portal(ctx, "https://acme.test/back"); return e },
			`{"return_url":"https://acme.test/back"}`},
		{"billing.ValidateDiscount without plan", func() error { _, _, e := c.Billing.ValidateDiscount(ctx, "SAVE20", ""); return e },
			`{"code":"SAVE20"}`},
		{"billing.ValidateDiscount with plan", func() error { _, _, e := c.Billing.ValidateDiscount(ctx, "SAVE20", "plan_1"); return e },
			`{"code":"SAVE20","plan_id":"plan_1"}`},
		{"billing.EnterpriseInquiry", func() error {
			_, _, e := c.Billing.EnterpriseInquiry(ctx, &EnterpriseInquiryParams{
				CompanyName: "Acme", ContactName: "Ada", ContactEmail: "ada@acme.test",
			})
			return e
		}, `{"company_name":"Acme","contact_name":"Ada","contact_email":"ada@acme.test"}`},
		{"organization.TransferOwnership", func() error { _, e := c.Organization.TransferOwnership(ctx, "u_2"); return e },
			`{"new_owner_user_id":"u_2"}`},
		{"organization.AcceptInvitation", func() error { _, _, e := c.Organization.AcceptInvitation(ctx, "tok_1"); return e },
			`{"token":"tok_1"}`},
		{"websitetracking.UpdateSettings", func() error {
			_, _, e := c.WebsiteTracking.UpdateSettings(ctx, &WebsiteTrackingUpdateParams{
				ConsentMode:  String(WebsiteConsentImplicit),
				AllowedHosts: &[]string{},
			})
			return e
		}, `{"consent_mode":"implicit","allowed_hosts":[]}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got = recordedRequest{}
			if err := tc.call(); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if strings.TrimSpace(got.body) != tc.want {
				t.Errorf("%s body = %s, want %s", tc.name, strings.TrimSpace(got.body), tc.want)
			}
		})
	}
}

// TestAuthConfigDecode checks that everything a login screen branches on
// survives the round trip.
func TestAuthConfigDecode(t *testing.T) {
	c := accountServer(t, `{
		"captcha": false,
		"password_login": true,
		"login_code": "new_device",
		"registration": "invite_only",
		"email_verification": true,
		"mail_delivers": false,
		"passkeys": true,
		"providers": ["oidc", "google"],
		"provider_labels": {"oidc": "Authentik"},
		"self_hosted": true,
		"billing_enabled": false,
		"setup_required": false,
		"invites_required": true,
		"docs_url": "https://docs.warmbly.com/development/accounts-and-access/",
		"websocket_url": "wss://acme.test/realtime",
		"app_url": "https://acme.test"
	}`, nil)

	cfg, _, err := c.Auth.Config(context.Background())
	if err != nil {
		t.Fatalf("Config: %v", err)
	}
	if cfg.LoginCode != LoginCodeNewDevice || cfg.Registration != RegistrationInviteOnly || !cfg.InvitesRequired {
		t.Errorf("policy = %q/%q/%v", cfg.LoginCode, cfg.Registration, cfg.InvitesRequired)
	}
	if cfg.Captcha || cfg.MailDelivers || cfg.BillingEnabled || !cfg.SelfHosted || !cfg.Passkeys {
		t.Errorf("capability flags = %+v", cfg)
	}
	if len(cfg.Providers) != 2 || cfg.Providers[0] != SSOProviderOIDC {
		t.Errorf("providers = %v", cfg.Providers)
	}
	if cfg.ProviderLabels["oidc"] != "Authentik" {
		t.Errorf("provider labels = %v", cfg.ProviderLabels)
	}
	if cfg.WebsocketURL != "wss://acme.test/realtime" || cfg.AppURL != "https://acme.test" {
		t.Errorf("urls = %q %q", cfg.WebsocketURL, cfg.AppURL)
	}
}

func TestInstanceInfoDecode(t *testing.T) {
	c := accountServer(t, `{
		"self_hosted": true,
		"version": "v0.9.2",
		"commit": "50f50e6",
		"update_available": true,
		"latest": {
			"tag": "v0.9.3",
			"html_url": "https://github.com/warmbly/warmbly/releases/tag/v0.9.3",
			"published_at": "2026-08-30T09:00:00Z"
		},
		"checked_at": "2026-09-01T00:00:00Z"
	}`, nil)

	info, _, err := c.Auth.Instance(context.Background())
	if err != nil {
		t.Fatalf("Instance: %v", err)
	}
	if !info.SelfHosted || info.Version != "v0.9.2" || info.Commit != "50f50e6" || !info.UpdateAvailable {
		t.Errorf("instance = %+v", info)
	}
	if info.Latest == nil || info.Latest.Tag != "v0.9.3" || info.Latest.PublishedAt.IsZero() {
		t.Fatalf("latest = %+v", info.Latest)
	}
	if info.CheckedAt == nil {
		t.Error("checked_at did not decode")
	}
}

// TestAuthStepDecode covers both shapes the first sign-in step can take: a
// handle to confirm, or a finished session.
func TestAuthStepDecode(t *testing.T) {
	t.Run("code required", func(t *testing.T) {
		c := accountServer(t, `{"session":"ls_1","code_required":true}`, nil)
		step, _, err := c.Auth.Login(context.Background(), &LoginParams{Email: "a@b.com", Password: "pw"})
		if err != nil {
			t.Fatalf("Login: %v", err)
		}
		if !step.CodeRequired || step.Session != "ls_1" || step.Done() {
			t.Errorf("step = %+v", step)
		}
	})

	t.Run("finished in one step", func(t *testing.T) {
		c := accountServer(t, `{
			"code_required": false,
			"token": {
				"access_token": "wmblyo_at",
				"access_token_expires_at": "2026-09-07T12:00:00Z",
				"refresh_token": "wmblyo_rt",
				"refresh_token_expires_at": "2026-10-07T12:00:00Z"
			}
		}`, nil)
		step, _, err := c.Auth.Login(context.Background(), &LoginParams{Email: "a@b.com", Password: "pw"})
		if err != nil {
			t.Fatalf("Login: %v", err)
		}
		if !step.Done() || step.Token == nil || step.Token.AccessToken != "wmblyo_at" {
			t.Fatalf("step = %+v", step)
		}
		if step.Token.RefreshTokenExpiresAt.IsZero() {
			t.Error("refresh expiry did not decode")
		}
	})

	t.Run("two-factor challenge", func(t *testing.T) {
		c := accountServer(t, `{"code_required":false,"two_fa_required":true,"pending_token":"pt_1","expires_in":300}`, nil)
		step, _, err := c.Auth.Login(context.Background(), &LoginParams{Email: "a@b.com", Password: "pw"})
		if err != nil {
			t.Fatalf("Login: %v", err)
		}
		if !step.Done() || !step.TwoFARequired || step.PendingToken != "pt_1" || step.ExpiresIn != 300 {
			t.Errorf("step = %+v", step)
		}
	})
}

func TestSSOBeginAndExchangeDecode(t *testing.T) {
	var got recordedRequest
	c := accountServer(t, `{"url":"https://idp.test/authorize?state=abc","binding":"bind_1"}`, &got)

	redirect, _, err := c.Auth.BeginSSO(context.Background(), SSOProviderOIDC)
	if err != nil {
		t.Fatalf("BeginSSO: %v", err)
	}
	if got.path != "/v1/auth/oidc/begin" || got.method != http.MethodPost {
		t.Errorf("begin = %s %s", got.method, got.path)
	}
	if redirect.URL != "https://idp.test/authorize?state=abc" || redirect.Binding != "bind_1" {
		t.Errorf("redirect = %+v", redirect)
	}
}

// TestSSOExchangeWrongBrowser is the refusal a forwarded handoff link earns.
func TestSSOExchangeWrongBrowser(t *testing.T) {
	c := accountServerFunc(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"Unauthorized","message":"start the sign-in again in this browser","code":"sso_wrong_browser","request_id":"req_1"}`)
	})

	_, _, err := c.Auth.ExchangeSSO(context.Background(), "hc_1", "")
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("ExchangeSSO error = %v, want *Error", err)
	}
	if apiErr.Code != "sso_wrong_browser" || apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("error = %+v", apiErr)
	}
}

// TestWaitForCLIAuthApproval drives the poll loop the way a CLI does: pending
// twice, then the one poll that carries the minted key.
func TestWaitForCLIAuthApproval(t *testing.T) {
	prev := cliAuthPollInterval
	cliAuthPollInterval = time.Millisecond
	t.Cleanup(func() { cliAuthPollInterval = prev })

	var polls atomic.Int32
	c := accountServerFunc(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/auth/cli/poll" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if polls.Add(1) <= 2 {
			_, _ = io.WriteString(w, `{"status":"pending"}`)
			return
		}
		_, _ = io.WriteString(w, `{
			"status": "approved",
			"token": "wmbly_minted",
			"api_key_id": "key_1",
			"scopes": 31,
			"scope_names": ["READ_EMAILS"],
			"user_id": "u_1",
			"user_email": "ada@acme.test",
			"user_name": "Ada Lovelace",
			"organization_id": "org_1",
			"organization_name": "Acme"
		}`)
	})

	grant, err := c.Auth.WaitForCLIAuth(context.Background(), &CLIAuthHandshake{DeviceCode: "dc_1"})
	if err != nil {
		t.Fatalf("WaitForCLIAuth: %v", err)
	}
	if n := polls.Load(); n != 3 {
		t.Errorf("polled %d times, want 3", n)
	}
	if grant.Status != CLIAuthApproved || grant.Token != "wmbly_minted" {
		t.Fatalf("grant = %+v", grant)
	}
	if grant.APIKeyID == nil || *grant.APIKeyID != "key_1" {
		t.Errorf("api key id = %v", grant.APIKeyID)
	}
	if grant.OrganizationName != "Acme" || grant.UserEmail != "ada@acme.test" || grant.Scopes != 31 {
		t.Errorf("identity = %+v", grant)
	}
}

// TestWaitForCLIAuthDenied checks the denial is reported as a sentinel rather
// than a generic error, so a CLI can tell "no" apart from "broke".
func TestWaitForCLIAuthDenied(t *testing.T) {
	prev := cliAuthPollInterval
	cliAuthPollInterval = time.Millisecond
	t.Cleanup(func() { cliAuthPollInterval = prev })

	c := accountServer(t, `{"status":"denied"}`, nil)
	poll, err := c.Auth.WaitForCLIAuth(context.Background(), &CLIAuthHandshake{DeviceCode: "dc_1"})
	if !errors.Is(err, ErrCLIAuthDenied) {
		t.Fatalf("err = %v, want ErrCLIAuthDenied", err)
	}
	if poll == nil || poll.Status != CLIAuthDenied {
		t.Errorf("poll = %+v", poll)
	}
}

func TestWaitForCLIAuthNeedsHandshake(t *testing.T) {
	c := accountServer(t, `{}`, nil)
	if _, err := c.Auth.WaitForCLIAuth(context.Background(), nil); err == nil {
		t.Error("WaitForCLIAuth(nil) should refuse before making a request")
	}
	if _, err := c.Auth.WaitForCLIAuth(context.Background(), &CLIAuthHandshake{}); err == nil {
		t.Error("WaitForCLIAuth without a device code should refuse")
	}
}

func TestNotificationPreferencesDecode(t *testing.T) {
	c := accountServer(t, `{
		"preferences": {
			"inbound_reply": {"enabled": false, "channels": {"in_app": true, "email": false, "slack": false, "push": false}},
			"campaign_paused": {"enabled": true, "channels": {"in_app": true, "email": true, "slack": false, "push": true}},
			"health_domain_auth": {"enabled": true, "channels": {"in_app": true, "email": true, "slack": false, "push": true}},
			"email_digest_minutes": 30
		},
		"email_delivery": {"min_minutes": 5, "max_minutes": 240, "daily_cap": 20}
	}`, nil)

	res, _, err := c.Auth.NotificationPreferences(context.Background())
	if err != nil {
		t.Fatalf("NotificationPreferences: %v", err)
	}
	if res.Preferences.InboundReply.Enabled {
		t.Error("inbound_reply should be off")
	}
	if !res.Preferences.CampaignPaused.Enabled || !res.Preferences.CampaignPaused.Channels.Email {
		t.Errorf("campaign_paused = %+v", res.Preferences.CampaignPaused)
	}
	if !res.Preferences.DomainAuth.Enabled || !res.Preferences.DomainAuth.Channels.Push {
		t.Errorf("health_domain_auth = %+v", res.Preferences.DomainAuth)
	}
	if res.EmailDelivery == nil || res.EmailDelivery.MaxMinutes != 240 || res.EmailDelivery.DailyCap != 20 {
		t.Errorf("email delivery = %+v", res.EmailDelivery)
	}
}

func TestAPIKeyDecode(t *testing.T) {
	c := accountServer(t, `{
		"id": "key_1",
		"user_id": "u_1",
		"organization_id": "org_1",
		"name": "ci",
		"description": "build pipeline",
		"key_prefix": "wmbly_ab",
		"key_suffix": "9f2c",
		"permissions": 7,
		"allowed_ips": ["203.0.113.4", "198.51.100.0/24"],
		"allowed_email_accounts": ["em_1"],
		"rate_limit_per_minute": 600,
		"status": "active",
		"last_used_at": "2026-09-06T10:00:00Z",
		"last_request_ip": "203.0.113.4",
		"expires_at": null,
		"revoked_at": null,
		"revoked_reason": null,
		"created_at": "2026-08-01T00:00:00Z",
		"updated_at": "2026-09-06T10:00:00Z"
	}`, nil)

	key, _, err := c.APIKeys.Get(context.Background(), "key_1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if key.Status != APIKeyStatusActive || key.Revoked() {
		t.Errorf("status = %q revoked=%v", key.Status, key.Revoked())
	}
	// rate_limit_per_minute is the only per-key ceiling the API carries; the
	// per-category limits (read/write/bulk/websocket) are an admin-surface
	// concept and never ride an API key.
	if key.RateLimitPerMinute != 600 {
		t.Errorf("rate limit = %d, want 600", key.RateLimitPerMinute)
	}
	if key.KeyPrefix != "wmbly_ab" || key.KeySuffix != "9f2c" {
		t.Errorf("key ends = %q/%q", key.KeyPrefix, key.KeySuffix)
	}
	if len(key.AllowedIPs) != 2 || len(key.AllowedEmailAccounts) != 1 {
		t.Errorf("allow lists = %v %v", key.AllowedIPs, key.AllowedEmailAccounts)
	}
	if !key.Can(PermReadEmails|PermReadCampaigns|PermReadContacts) || key.Can(PermWriteEmails) {
		t.Errorf("permissions = %d", key.Permissions)
	}
	if !key.CanAny(PermReadEmails | PermWriteEmails) {
		t.Error("CanAny should hold for a mask sharing one bit")
	}
	if key.LastUsedAt == nil || key.ExpiresAt != nil || key.RevokedReason != nil {
		t.Errorf("nullable timestamps = %v %v %v", key.LastUsedAt, key.ExpiresAt, key.RevokedReason)
	}
}

func TestAPIKeyPermissionCatalogDecode(t *testing.T) {
	c := accountServer(t, `{
		"permissions": [
			{"name": "READ_CONTACTS", "value": 4, "description": "View contact lists, segments, notes, and activities", "category": "read"},
			{"name": "WRITE_CONTACTS", "value": 128, "description": "Create and modify contacts, segments, notes, and activities", "category": "write"}
		],
		"presets": {"read_only": 8552739, "full_access": 16777215}
	}`, nil)

	cat, _, err := c.APIKeys.Permissions(context.Background())
	if err != nil {
		t.Fatalf("Permissions: %v", err)
	}
	if len(cat.Permissions) != 2 {
		t.Fatalf("permissions = %+v", cat.Permissions)
	}
	// The bit values are positional, so a reordering upstream would show here.
	if cat.Permissions[0].Value != PermReadContacts || cat.Permissions[1].Value != PermWriteContacts {
		t.Errorf("bit values = %d %d, want %d %d",
			cat.Permissions[0].Value, cat.Permissions[1].Value, PermReadContacts, PermWriteContacts)
	}
	if !strings.Contains(cat.Permissions[0].Description, "segments") {
		t.Errorf("READ_CONTACTS description = %q, want it to mention segments", cat.Permissions[0].Description)
	}
	if cat.Presets.FullAccess == 0 || cat.Presets.ReadOnly == 0 {
		t.Errorf("presets = %+v", cat.Presets)
	}
}

func TestOrgRiskDecode(t *testing.T) {
	c := accountServer(t, `{
		"state": "restricted",
		"restricted": true,
		"suspended": false,
		"reason": "Complaint rate above threshold for 3 days."
	}`, nil)

	risk, _, err := c.Organization.Risk(context.Background())
	if err != nil {
		t.Fatalf("Risk: %v", err)
	}
	if risk.State != OrgRiskRestricted || !risk.Restricted || risk.Suspended {
		t.Errorf("risk = %+v", risk)
	}
	if risk.Reason == "" {
		t.Error("reason should survive")
	}
}

func TestOrganizationLimitsDecode(t *testing.T) {
	c := accountServer(t, `{
		"limits": {"max_campaigns": 50, "max_email_accounts": null, "max_contacts": 100000},
		"counts": {"total_campaigns": 4, "active_campaigns": 1, "total_contacts": 812, "total_members": 3, "email_accounts": 6, "emails_sent_today": 120},
		"mailboxes": {"used": 6, "allowance": 10, "remaining": 4, "basis": "fair_use", "sends_per_mailbox": 50, "plan_daily_sends": 500, "plan_name": "Pro", "paid": true},
		"storage": {"used_bytes": 6442450944, "limit_bytes": 5368709120, "over_quota": true}
	}`, nil)

	res, _, err := c.Organization.Limits(context.Background())
	if err != nil {
		t.Fatalf("Limits: %v", err)
	}
	if res.Limits == nil || res.Limits.MaxCampaigns == nil || *res.Limits.MaxCampaigns != 50 {
		t.Fatalf("limits = %+v", res.Limits)
	}
	// A null ceiling is unmetered, not zero.
	if res.Limits.MaxEmailAccounts != nil {
		t.Errorf("max_email_accounts = %v, want nil for unmetered", res.Limits.MaxEmailAccounts)
	}
	if res.Counts == nil || res.Counts.EmailsSentToday != 120 {
		t.Errorf("counts = %+v", res.Counts)
	}
	if res.Storage == nil || !res.Storage.OverQuota || res.Storage.UsedBytes <= res.Storage.LimitBytes {
		t.Errorf("storage = %+v", res.Storage)
	}
	if res.Mailboxes == nil || res.Mailboxes.Basis != MailboxAllowanceFairUse || !res.Mailboxes.CanAdd(4) {
		t.Fatalf("mailboxes = %+v", res.Mailboxes)
	}
	if res.Mailboxes.CanAdd(5) {
		t.Error("CanAdd should refuse more than the remaining allowance")
	}
}

func TestOrgExportJobDecode(t *testing.T) {
	c := accountServer(t, `{
		"id": "ex_1",
		"organization_id": "org_1",
		"requested_by": "u_1",
		"status": "completed",
		"groups": ["core", "contacts", "campaigns"],
		"include_secrets": true,
		"format_version": 1,
		"progress_percent": 100,
		"progress_stage": "done",
		"archive_bytes": 20971520,
		"archive_sha256": "9f2c1b",
		"row_counts": {"contacts": 812, "campaigns": 4},
		"error_message": null,
		"started_at": "2026-09-06T10:00:00Z",
		"completed_at": "2026-09-06T10:02:00Z",
		"expires_at": "2026-09-13T10:02:00Z",
		"created_at": "2026-09-06T09:59:00Z",
		"updated_at": "2026-09-06T10:02:00Z"
	}`, nil)

	job, _, err := c.Organization.Export(context.Background(), "ex_1")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if job.Status != OrgTransferCompleted || !job.Terminal() {
		t.Errorf("status = %q terminal=%v", job.Status, job.Terminal())
	}
	if !job.IncludeSecrets || job.FormatVersion != 1 || len(job.Groups) != 3 || job.Groups[0] != OrgDataGroupCore {
		t.Errorf("job = %+v", job)
	}
	if job.TotalRows() != 816 {
		t.Errorf("TotalRows = %d, want 816", job.TotalRows())
	}
	if job.ArchiveBytes == nil || *job.ArchiveBytes != 20971520 || job.ExpiresAt == nil {
		t.Errorf("archive = %v %v", job.ArchiveBytes, job.ExpiresAt)
	}

	running := &OrgExportJob{Status: OrgTransferRunning}
	if running.Terminal() {
		t.Error("a running job is not terminal")
	}
}

// TestDownloadExportStreamsToWriter covers the one route that answers with
// bytes rather than JSON.
func TestDownloadExportStreamsToWriter(t *testing.T) {
	const archive = "PK\x03\x04 pretend this is a zip"
	var gotPath string
	c := accountServerFunc(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("X-Archive-SHA256", "9f2c1b")
		_, _ = io.WriteString(w, archive)
	})

	var buf bytes.Buffer
	resp, err := c.Organization.DownloadExport(context.Background(), "ex_1", &buf)
	if err != nil {
		t.Fatalf("DownloadExport: %v", err)
	}
	if gotPath != "/v1/organization/current/export/ex_1/download" {
		t.Errorf("path = %q", gotPath)
	}
	if buf.String() != archive {
		t.Errorf("archive = %q", buf.String())
	}
	if resp.Header.Get("X-Archive-SHA256") != "9f2c1b" {
		t.Errorf("digest header = %q", resp.Header.Get("X-Archive-SHA256"))
	}

	if _, err := c.Organization.DownloadExport(context.Background(), "ex_1", nil); err == nil {
		t.Error("DownloadExport with no writer should refuse")
	}
}

// TestOrgTransferCatalogDecode also pins the required-object shape
// routingClient cannot answer.
func TestOrgTransferCatalogDecode(t *testing.T) {
	c := accountServer(t, `{
		"groups": [
			{"key": "core", "label": "Workspace", "description": "Members, roles, settings", "required": true, "heavy": false},
			{"key": "inbox", "label": "Inbox", "description": "Threads and messages", "required": false, "heavy": true, "requires": ["contacts"]}
		],
		"format_version": 1,
		"min_passphrase": 12,
		"retention_days": 7
	}`, nil)

	cat, _, err := c.Organization.TransferGroups(context.Background())
	if err != nil {
		t.Fatalf("TransferGroups: %v", err)
	}
	if cat.FormatVersion != 1 || cat.MinPassphrase != 12 || cat.RetentionDays != 7 {
		t.Errorf("catalog = %+v", cat)
	}
	if len(cat.Groups) != 2 || !cat.Groups[0].Required || !cat.Groups[1].Heavy {
		t.Fatalf("groups = %+v", cat.Groups)
	}
	if len(cat.Groups[1].Requires) != 1 || cat.Groups[1].Requires[0] != OrgDataGroupContacts {
		t.Errorf("inbox requires = %v", cat.Groups[1].Requires)
	}
}

// TestInvitationPreviewDecode pins the public preview shape, which is not the
// workspace-side invitation row.
func TestInvitationPreviewDecode(t *testing.T) {
	c := accountServer(t, `{
		"organization_name": "Acme",
		"organization_avatar": "https://acme.test/logo.png",
		"inviter_name": "Ada Lovelace",
		"email": "grace@acme.test",
		"roles": [{"id": "r_1", "name": "Ops", "color": "#123456"}],
		"expired": false
	}`, nil)

	preview, _, err := c.Organization.PreviewInvitation(context.Background(), "tok_1")
	if err != nil {
		t.Fatalf("PreviewInvitation: %v", err)
	}
	if preview.OrganizationName != "Acme" || preview.InviterName != "Ada Lovelace" || preview.Expired {
		t.Errorf("preview = %+v", preview)
	}
	if len(preview.Roles) != 1 || preview.Roles[0].Name != "Ops" {
		t.Errorf("roles = %+v", preview.Roles)
	}
}

func TestAvatarUploadReturnsURL(t *testing.T) {
	var gotField, gotFilename string
	c := accountServerFunc(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		// The server reads the part named "file"; anything else is a 400.
		for name, files := range r.MultipartForm.File {
			gotField = name
			if len(files) > 0 {
				gotFilename = files[0].Filename
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"avatar_url":"https://acme.test/avatars/u_1.png"}`)
	})

	url, _, err := c.Auth.UploadAvatar(context.Background(), &FileUpload{
		Filename: "me.png", Content: strings.NewReader("png-bytes"), ContentType: "image/png",
	})
	if err != nil {
		t.Fatalf("UploadAvatar: %v", err)
	}
	if gotField != "file" {
		t.Errorf("multipart field = %q, want %q", gotField, "file")
	}
	if gotFilename != "me.png" {
		t.Errorf("filename = %q", gotFilename)
	}
	if url != "https://acme.test/avatars/u_1.png" {
		t.Errorf("avatar url = %q", url)
	}

	orgURL, _, err := c.Organization.UploadAvatar(context.Background(), &FileUpload{
		Filename: "logo.png", Content: strings.NewReader("png-bytes"),
	})
	if err != nil {
		t.Fatalf("Organization.UploadAvatar: %v", err)
	}
	if gotField != "file" || orgURL == "" {
		t.Errorf("org avatar field=%q url=%q", gotField, orgURL)
	}
}

func TestSubscriptionDecode(t *testing.T) {
	c := accountServer(t, `{
		"id": "sub_1",
		"user_id": "u_1",
		"organization_id": "org_1",
		"plan_id": "plan_1",
		"stripe_customer_id": "cus_1",
		"status": "active",
		"current_period_end": "2026-10-01T00:00:00Z",
		"cancel_at_period_end": true,
		"is_enterprise": false,
		"plan": {"id": "plan_1", "name": "Pro", "daily_emails": 500, "monthly_credits": 2000, "duration": "month", "max_email_accounts": 20},
		"created_at": "2026-01-01T00:00:00Z",
		"updated_at": "2026-09-01T00:00:00Z"
	}`, nil)

	sub, _, err := c.Billing.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sub.Status != "active" || !sub.CancelAtPeriodEnd || sub.IsEnterprise {
		t.Errorf("subscription = %+v", sub)
	}
	// The plan rides the subscription, so a client does not have to look it up.
	if sub.Plan == nil || sub.Plan.Name == nil || *sub.Plan.Name != "Pro" {
		t.Fatalf("plan = %+v", sub.Plan)
	}
	if sub.Plan.Duration != DurationMonth || sub.Plan.MonthlyCredits != 2000 {
		t.Errorf("plan detail = %+v", sub.Plan)
	}
	if sub.Plan.MaxEmailAccounts == nil || *sub.Plan.MaxEmailAccounts != 20 {
		t.Errorf("plan mailbox ceiling = %v", sub.Plan.MaxEmailAccounts)
	}
}

func TestCreditBalanceUnlimitedDecode(t *testing.T) {
	c := accountServer(t, `{
		"unlimited": true,
		"balance": 0,
		"monthly_balance": 0,
		"purchased_balance": 0,
		"monthly_allowance": 0,
		"total_purchased": 0,
		"monthly_reset_at": null,
		"next_reset_at": null,
		"packs": []
	}`, nil)

	bal, _, err := c.Billing.Credits(context.Background())
	if err != nil {
		t.Fatalf("Credits: %v", err)
	}
	// Without a billing provider AI is unmetered: zero here means "not
	// counted", not "nothing left".
	if !bal.Unlimited || bal.Balance != 0 || len(bal.Packs) != 0 {
		t.Errorf("balance = %+v", bal)
	}
	if bal.MonthlyResetAt != nil || bal.NextResetAt != nil {
		t.Errorf("reset timestamps = %v %v", bal.MonthlyResetAt, bal.NextResetAt)
	}
}

func TestPlanChangePreviewDecode(t *testing.T) {
	c := accountServer(t, `{
		"current_plan": {"id": "plan_1", "name": "Starter"},
		"new_plan": {"id": "plan_2", "name": "Pro"},
		"proration_amount": -1200,
		"amount_due": 8800,
		"next_billing_date": "2026-10-01T00:00:00Z",
		"currency": "usd"
	}`, nil)

	preview, _, err := c.Billing.PreviewPlanChange(context.Background(), "plan_2")
	if err != nil {
		t.Fatalf("PreviewPlanChange: %v", err)
	}
	if preview.CurrentPlan == nil || preview.NewPlan == nil || preview.NewPlan.ID != "plan_2" {
		t.Fatalf("plans = %+v %+v", preview.CurrentPlan, preview.NewPlan)
	}
	if preview.ProrationAmount != -1200 || preview.AmountDue != 8800 || preview.Currency != "usd" {
		t.Errorf("amounts = %+v", preview)
	}
	if preview.NextBillingDate.IsZero() {
		t.Error("next_billing_date did not decode")
	}
}

func TestDiscountPreviewDecode(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		c := accountServer(t, `{
			"valid": true,
			"code": "SAVE20",
			"type": "percent",
			"percent_off": 20,
			"duration": "repeating",
			"duration_in_months": 3,
			"original_amount": 99,
			"discounted_amount": 79.2,
			"savings_amount": 19.8
		}`, nil)
		p, _, err := c.Billing.ValidateDiscount(context.Background(), "SAVE20", "plan_1")
		if err != nil {
			t.Fatalf("ValidateDiscount: %v", err)
		}
		if !p.Valid || p.Type != DiscountTypePercent || p.Duration != DiscountDurationRepeating {
			t.Errorf("preview = %+v", p)
		}
		if p.PercentOff == nil || *p.PercentOff != 20 || p.DurationInMonths == nil || *p.DurationInMonths != 3 {
			t.Errorf("percent/duration = %v %v", p.PercentOff, p.DurationInMonths)
		}
		if p.SavingsAmount == nil || *p.SavingsAmount != 19.8 {
			t.Errorf("savings = %v", p.SavingsAmount)
		}
	})

	t.Run("refused", func(t *testing.T) {
		// An unusable code is a 200 with valid:false, not an error.
		c := accountServer(t, `{"valid":false,"reason":"This code has expired."}`, nil)
		p, _, err := c.Billing.ValidateDiscount(context.Background(), "OLD", "")
		if err != nil {
			t.Fatalf("ValidateDiscount: %v", err)
		}
		if p.Valid || p.Reason == "" {
			t.Errorf("preview = %+v", p)
		}
	})
}

func TestReferralCodeAndEarningsDecode(t *testing.T) {
	t.Run("ensure code", func(t *testing.T) {
		// POST /subscription/referral answers with the code row, not the
		// summary that GET returns.
		c := accountServer(t, `{
			"id": "rc_1",
			"owner_user_id": "u_1",
			"owner_org_id": "org_1",
			"code": "ADA-2026",
			"discount_code_id": "dc_1",
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-01T00:00:00Z"
		}`, nil)
		code, _, err := c.Billing.EnsureReferralCode(context.Background())
		if err != nil {
			t.Fatalf("EnsureReferralCode: %v", err)
		}
		if code.Code != "ADA-2026" || code.OwnerOrgID != "org_1" {
			t.Errorf("code = %+v", code)
		}
		if code.DiscountCodeID == nil || *code.DiscountCodeID != "dc_1" {
			t.Errorf("discount code = %v", code.DiscountCodeID)
		}
	})

	t.Run("earnings ledger", func(t *testing.T) {
		c := accountServer(t, `{
			"data": [{
				"id": "re_1",
				"org_id": "org_1",
				"attribution_id": "ra_1",
				"amount_cents": 4900,
				"currency": "usd",
				"reason": "referral_reward",
				"balance_after_cents": 4900,
				"stripe_customer_balance_txn_id": "cbtxn_1",
				"created_at": "2026-09-01T00:00:00Z"
			}],
			"pagination": {"has_more": false, "next_cursor": null}
		}`, nil)
		page, err := c.Billing.ReferralEarnings(context.Background(), nil)
		if err != nil {
			t.Fatalf("ReferralEarnings: %v", err)
		}
		if len(page.Data) != 1 {
			t.Fatalf("data = %+v", page.Data)
		}
		e := page.Data[0]
		if e.Reason != "referral_reward" || e.AmountCents != 4900 || e.BalanceAfterCents != 4900 {
			t.Errorf("earning = %+v", e)
		}
		if e.AttributionID == nil || *e.AttributionID != "ra_1" {
			t.Errorf("attribution = %v", e.AttributionID)
		}
		if page.HasMore() {
			t.Error("HasMore should be false on the last page")
		}
	})
}

func TestReferralSummaryAndAttributionDecode(t *testing.T) {
	c := accountServer(t, `{
		"code": "ADA-2026",
		"share_url": "https://warmbly.com/?ref=ADA-2026",
		"currency": "usd",
		"invitee_percent_off": 20,
		"invitee_months": 3,
		"balance_cents": 9800,
		"lifetime_earned_cents": 14700,
		"total_referred": 5,
		"pending": 2,
		"qualified": 1,
		"rewarded": 2
	}`, nil)

	summary, _, err := c.Billing.Referral(context.Background())
	if err != nil {
		t.Fatalf("Referral: %v", err)
	}
	if summary.Code != "ADA-2026" || summary.BalanceCents != 9800 || summary.LifetimeEarnedCents != 14700 {
		t.Errorf("summary = %+v", summary)
	}
	if summary.TotalReferred != 5 || summary.Rewarded != 2 {
		t.Errorf("counts = %+v", summary)
	}
}

func TestWebsiteTrackingSettingsDecode(t *testing.T) {
	c := accountServer(t, `{
		"organization_id": "org_1",
		"enabled": true,
		"site_key": "wsk_abc123",
		"consent_mode": "explicit",
		"location_precision": "country",
		"allowed_hosts": ["acme.test"],
		"retention_days": 90,
		"updated_at": "2026-09-01T00:00:00Z",
		"tracking_host": "t.warmbly.com"
	}`, nil)

	s, _, err := c.WebsiteTracking.Settings(context.Background())
	if err != nil {
		t.Fatalf("Settings: %v", err)
	}
	if !s.Enabled || s.SiteKey != "wsk_abc123" {
		t.Errorf("settings = %+v", s)
	}
	if s.ConsentMode != WebsiteConsentExplicit || s.LocationPrecision != WebsiteLocationCountry {
		t.Errorf("privacy = %q/%q", s.ConsentMode, s.LocationPrecision)
	}
	if s.RetentionDays != WebsiteRetentionDefaultDays || s.TrackingHost != "t.warmbly.com" {
		t.Errorf("retention/host = %d %q", s.RetentionDays, s.TrackingHost)
	}
	if len(s.AllowedHosts) != 1 || s.AllowedHosts[0] != "acme.test" {
		t.Errorf("allowed hosts = %v", s.AllowedHosts)
	}
}

// TestMetaTimezonesDecode covers the bare-array response, which the shared
// envelope fixture cannot represent.
func TestMetaTimezonesDecode(t *testing.T) {
	c := accountServer(t, `[
		{"name": "Europe/Budapest", "display_name": "(UTC+02:00) Budapest"},
		{"name": "America/New_York", "display_name": "(UTC-04:00) New York"}
	]`, nil)

	zones, _, err := c.Meta.Timezones(context.Background())
	if err != nil {
		t.Fatalf("Timezones: %v", err)
	}
	if len(zones) != 2 || zones[0].Name != "Europe/Budapest" || zones[0].DisplayName == "" {
		t.Errorf("timezones = %+v", zones)
	}
}

func TestMetaIdentityDecode(t *testing.T) {
	c := accountServer(t, `{
		"user_id": "u_1",
		"email": "ada@acme.test",
		"name": "Ada Lovelace",
		"first_name": "Ada",
		"last_name": "Lovelace",
		"organization_id": "org_1",
		"organization_name": "Acme",
		"auth_type": "api_key",
		"scopes": ["READ_EMAILS", "READ_CAMPAIGNS"]
	}`, nil)

	id, _, err := c.Meta.Identity(context.Background())
	if err != nil {
		t.Fatalf("Identity: %v", err)
	}
	if id.AuthType != AuthTypeAPIKey || len(id.Scopes) != 2 {
		t.Errorf("identity = %+v", id)
	}
	if id.OrganizationID == nil || *id.OrganizationID != "org_1" || id.OrganizationName != "Acme" {
		t.Errorf("workspace = %v %q", id.OrganizationID, id.OrganizationName)
	}
}

// TestMetaPlansDecode unwraps the {"plans": [...]} envelope.
func TestMetaPlansDecode(t *testing.T) {
	c := accountServer(t, `{"plans":[
		{"id": "plan_1", "name": "Pro", "price": 99, "duration": "month", "public": true, "monthly_credits": 2000, "referral_reward_percent": 100, "daily_emails": 500},
		{"id": "plan_2", "name": "Pro Annual", "price": 990, "duration": "year", "savings": 17, "public": true}
	]}`, nil)

	plans, _, err := c.Meta.Plans(context.Background())
	if err != nil {
		t.Fatalf("Plans: %v", err)
	}
	if len(plans) != 2 {
		t.Fatalf("plans = %+v", plans)
	}
	if plans[0].Duration != DurationMonth || plans[1].Duration != DurationYear {
		t.Errorf("durations = %q %q", plans[0].Duration, plans[1].Duration)
	}
	if plans[0].ReferralRewardPercent != 100 || plans[1].Savings != 17 {
		t.Errorf("plan detail = %+v %+v", plans[0], plans[1])
	}
}

// TestAccountSyncNoBillingProvider checks the shape a deployment without a
// billing provider answers with, so a client can tell "off" from "broken".
func TestAccountSyncNoBillingProvider(t *testing.T) {
	c := accountServerFunc(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"Not Found","message":"404 page not found","code":"not_found"}`)
	})

	_, _, err := c.Meta.Plans(context.Background())
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Plans on a billing-less deployment = %v, want ErrNotFound", err)
	}
}
