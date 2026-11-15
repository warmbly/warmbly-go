package warmbly

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestMailboxGrantRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()

	runExactCases(t, &got, []exactCase{
		{"GrantConfig", func() error { _, _, e := c.Emails.GrantConfig(ctx); return e }, "GET", "/v1/emails/grants/config", "", ""},
		{"ListGrants", func() error { _, _, e := c.Emails.ListGrants(ctx); return e }, "GET", "/v1/emails/grants", "", ""},
		{"SigninMigration", func() error { _, _, e := c.Emails.SigninMigration(ctx); return e }, "GET", "/v1/emails/grants/migration", "", ""},
		{"StartGoogleGrant", func() error {
			_, _, e := c.Emails.StartGoogleGrant(ctx, "acme.io", "admin@acme.io")
			return e
		}, "POST", "/v1/emails/grants/google/start", "", `{"domain":"acme.io","admin_email":"admin@acme.io"}`},
		{"FinishGoogleGrant signin", func() error {
			_, _, e := c.Emails.FinishGoogleGrant(ctx, &GoogleGrantFinishParams{State: "st_1", Code: "code_1"})
			return e
		}, "POST", "/v1/emails/grants/google/finish", "", `{"state":"st_1","code":"code_1"}`},
		{"FinishGoogleGrant dns", func() error {
			_, _, e := c.Emails.FinishGoogleGrant(ctx, &GoogleGrantFinishParams{Domain: "acme.io", AdminEmail: "admin@acme.io"})
			return e
		}, "POST", "/v1/emails/grants/google/finish", "", `{"domain":"acme.io","admin_email":"admin@acme.io"}`},
		{"StartMicrosoftGrant", func() error { _, _, e := c.Emails.StartMicrosoftGrant(ctx); return e }, "POST", "/v1/emails/grants/microsoft/start", "", ""},
		{"FinishMicrosoftGrant", func() error {
			_, _, e := c.Emails.FinishMicrosoftGrant(ctx, "st_2", "code_2")
			return e
		}, "POST", "/v1/emails/grants/microsoft/finish", "", `{"state":"st_2","code":"code_2"}`},
		{"GetGrant", func() error { _, _, e := c.Emails.GetGrant(ctx, "gr_1"); return e }, "GET", "/v1/emails/grants/gr_1", "", ""},
		{"CheckGrant", func() error { _, _, e := c.Emails.CheckGrant(ctx, "gr_1"); return e }, "POST", "/v1/emails/grants/gr_1/check", "", ""},
		{"DeleteGrant", func() error { _, e := c.Emails.DeleteGrant(ctx, "gr_1"); return e }, "DELETE", "/v1/emails/grants/gr_1", "", ""},
		{"GrantUsers", func() error { _, _, e := c.Emails.GrantUsers(ctx, "gr_1"); return e }, "GET", "/v1/emails/grants/gr_1/users", "", ""},
		{"ConnectGrantUsers", func() error {
			_, _, e := c.Emails.ConnectGrantUsers(ctx, "gr_1", &GrantConnectParams{
				UserIDs: []string{"ana@acme.io"},
				Options: MailboxImportOptions{Settings: MailboxImportSettings{Warmup: Bool(true)}},
			})
			return e
		}, "POST", "/v1/emails/grants/gr_1/connect", "", `{"user_ids":["ana@acme.io"],"options":{"settings":{"warmup":true}}}`},
		{"ConnectGrantUsers all", func() error {
			_, _, e := c.Emails.ConnectGrantUsers(ctx, "gr_1", &GrantConnectParams{All: true})
			return e
		}, "POST", "/v1/emails/grants/gr_1/connect", "", `{"all":true,"options":{"settings":{}}}`},
		{"SwitchToAppPassword", func() error {
			_, _, e := c.Emails.SwitchToAppPassword(ctx, "acc_1", "abcd efgh ijkl mnop")
			return e
		}, "POST", "/v1/emails/onboarding/app-password/acc_1", "", `{"app_password":"abcd efgh ijkl mnop"}`},
	})
}

func TestMailboxGrantDecode(t *testing.T) {
	c := emailFixtureClient(t, http.StatusOK, `{"data": [{
		"id": "gr_1", "provider": "microsoft", "tenant": "6f1c0c1e-0000-4000-8000-000000000001",
		"admin_email": "admin@acme.io", "domains": ["acme.io", "acme.co"], "status": "active",
		"verified_at": "2026-11-12T10:00:00Z", "created_at": "2026-11-12T09:59:00Z", "mailboxes": 12
	}]}`, nil)
	list, _, err := c.Emails.ListGrants(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Provider != GrantProviderMicrosoft || len(list[0].Domains) != 2 || list[0].VerifiedAt == nil || list[0].Mailboxes != 12 {
		t.Errorf("grants = %+v", list)
	}
}

func TestMailboxGrantConfigDecode(t *testing.T) {
	c := emailFixtureClient(t, http.StatusOK, `{"google_enabled": false, "google_scopes": ["https://mail.google.com/"],
		"microsoft_enabled": true, "google_missing": ["GOOGLE_WORKSPACE_DELEGATION_KEY"], "microsoft_missing": [],
		"google_mailbox_client_id": "123.apps.googleusercontent.com"}`, nil)
	cfg, _, err := c.Emails.GrantConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GoogleEnabled || !cfg.MicrosoftEnabled || cfg.GoogleClientID != "" || cfg.GoogleMissing[0] != "GOOGLE_WORKSPACE_DELEGATION_KEY" || cfg.GoogleMailboxClientID == "" {
		t.Errorf("config = %+v", cfg)
	}
}

func TestMailboxGrantStartDecode(t *testing.T) {
	c := emailFixtureClient(t, http.StatusOK, `{"method": "dns", "txt_name": "_warmbly.acme.io", "txt_value": "wm=abc"}`, nil)
	s, _, err := c.Emails.StartGoogleGrant(context.Background(), "acme.io", "admin@acme.io")
	if err != nil {
		t.Fatal(err)
	}
	if s.Method != GrantMethodDNS || s.URL != "" || s.TXTValue != "wm=abc" {
		t.Errorf("start = %+v", s)
	}
}

func TestMailboxGrantUsersDecode(t *testing.T) {
	c := emailFixtureClient(t, http.StatusOK, `{"data": [
		{"id": "ana@acme.io", "email": "ana@acme.io", "name": "Ana", "enabled": true, "connected": true, "email_account_id": "acc_1", "upgrade": true},
		{"id": "bo@acme.io", "email": "bo@acme.io", "name": "Bo", "enabled": false, "connected": false, "upgrade": false}
	]}`, nil)
	users, _, err := c.Emails.GrantUsers(context.Background(), "gr_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 || users[0].EmailAccountID == nil || !users[0].Upgrade || users[1].EmailAccountID != nil || users[1].Enabled {
		t.Errorf("users = %+v", users)
	}
}

func TestSigninMigrationDecode(t *testing.T) {
	c := emailFixtureClient(t, http.StatusOK, `{"data": [
		{"domain": "acme.io", "kind": "workspace", "grant_id": "gr_1", "mailboxes": [
			{"id": "acc_1", "email": "ana@acme.io", "name": "Ana", "status": "active", "vendor_connection_id": "vc_1", "vendor": "inboxkit"}]},
		{"domain": "gmail.com", "kind": "personal", "mailboxes": [{"id": "acc_2", "email": "bo@gmail.com", "name": "Bo", "status": "active"}]}
	], "total": 2}`, nil)
	m, _, err := c.Emails.SigninMigration(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if m.Total != 2 || len(m.Data) != 2 || m.Data[0].GrantID == nil || m.Data[1].Kind != SigninMigrationPersonal || m.Data[1].GrantID != nil {
		t.Fatalf("migration = %+v", m)
	}
	if mb := m.Data[0].Mailboxes[0]; mb.VendorConnectionID == nil || *mb.VendorConnectionID != "vc_1" || mb.Vendor != "inboxkit" {
		t.Errorf("mailbox = %+v", mb)
	}
	if m.Data[1].Mailboxes[0].VendorConnectionID != nil {
		t.Error("vendor connection on a personal mailbox")
	}
}

func TestSigninMigrationEmpty(t *testing.T) {
	c := emailFixtureClient(t, http.StatusOK, `{"data": null, "total": 0}`, nil)
	m, _, err := c.Emails.SigninMigration(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if m.Data == nil || len(m.Data) != 0 {
		t.Errorf("data = %#v", m.Data)
	}
}

func TestSwitchToAppPasswordConflict(t *testing.T) {
	c := emailFixtureClient(t, http.StatusConflict, `{"error": "conflict", "code": "mailbox_not_google_signin", "message": "Only a Google mailbox can switch."}`, nil)
	_, resp, err := c.Emails.SwitchToAppPassword(context.Background(), "acc_1", "abcdefghijklmnop")
	var apiErr *Error
	if !errors.As(err, &apiErr) || !apiErr.HasCode(ErrCodeMailboxNotGoogleSignin) || resp.StatusCode != http.StatusConflict {
		t.Errorf("err = %v", err)
	}
}
