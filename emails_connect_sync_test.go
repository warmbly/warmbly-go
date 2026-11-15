package warmbly

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestEmailConnectRouting(t *testing.T) {
	ctx := context.Background()
	var got recordedRequest
	creds := &MailboxCredentials{Username: "u", Password: "p", Host: "mail.example.com", Port: 465}

	cases := []struct {
		name       string
		resp       string
		call       func(c *Client) error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string
	}{
		{"ConnectMailboxSMTPIMAP", `{"mailbox":{},"warmup_note":""}`, func(c *Client) error {
			_, _, e := c.Emails.ConnectMailboxSMTPIMAP(ctx, &ConnectSMTPIMAPParams{
				Email: "a@example.com", SMTP: creds, IMAP: creds,
				Warmup: &MailboxConnectWarmup{Enabled: true, Max: Int(40)},
			})
			return e
		}, "POST", "/v1/emails/connect/smtp-imap", "",
			`{"email":"a@example.com","smtp":{"username":"u","password":"p","host":"mail.example.com","port":465},"imap":{"username":"u","password":"p","host":"mail.example.com","port":465},"warmup":{"enabled":true,"max":40}}`},
		{"ConnectMailboxSMTPIMAPBulk", `{"data":[],"summary":{}}`, func(c *Client) error {
			_, _, e := c.Emails.ConnectMailboxSMTPIMAPBulk(ctx, &ConnectSMTPIMAPBulkParams{
				Accounts: []SMTPIMAPParams{{Email: "a@example.com", Name: "A", SMTP: creds, IMAP: creds}},
			})
			return e
		}, "POST", "/v1/emails/connect/smtp-imap/bulk", "",
			`{"accounts":[{"email":"a@example.com","name":"A","smtp":{"username":"u","password":"p","host":"mail.example.com","port":465},"imap":{"username":"u","password":"p","host":"mail.example.com","port":465}}]}`},
		{"ConnectMailboxOAuth", `{"mailbox":{},"renewed":false,"warmup_note":""}`, func(c *Client) error {
			_, _, e := c.Emails.ConnectMailboxOAuth(ctx, &ConnectOAuthParams{
				MailboxOAuthGrant: MailboxOAuthGrant{Provider: ProviderGmail, RefreshToken: "rt", Email: "a@example.com"},
				Warmup:            &MailboxConnectWarmup{Enabled: false},
			})
			return e
		}, "POST", "/v1/emails/connect/oauth", "",
			`{"provider":"gmail","refresh_token":"rt","email":"a@example.com","warmup":{"enabled":false}}`},
		{"ConnectMailboxOAuthBulk", `{"data":[],"summary":{}}`, func(c *Client) error {
			_, _, e := c.Emails.ConnectMailboxOAuthBulk(ctx, &ConnectOAuthBulkParams{
				Grants: []MailboxOAuthGrant{{Provider: ProviderOutlook, Code: "c", RedirectURI: "https://x.example/cb", CodeVerifier: "v"}},
			})
			return e
		}, "POST", "/v1/emails/connect/oauth/bulk", "",
			`{"grants":[{"provider":"outlook","code":"c","redirect_uri":"https://x.example/cb","code_verifier":"v"}]}`},
		{"CreateConnectLink", `{}`, func(c *Client) error {
			_, _, e := c.Emails.CreateConnectLink(ctx, &MailboxConnectLinkParams{
				Provider: ProviderGmail, Email: "a@example.com", ReturnURL: "https://x.example/done", ExpiresInHours: 24,
			})
			return e
		}, "POST", "/v1/emails/connect/links", "",
			`{"provider":"gmail","email":"a@example.com","return_url":"https://x.example/done","expires_in_hours":24}`},
		{"ListConnectLinks", `{"data":[],"pagination":{"next_cursor":null,"has_more":false}}`, func(c *Client) error {
			_, e := c.Emails.ListConnectLinks(ctx, &ListOptions{Limit: 10, Cursor: "abc"})
			return e
		}, "GET", "/v1/emails/connect/links", "cursor=abc&limit=10", ""},
		{"GetConnectLink", `{}`, func(c *Client) error { _, _, e := c.Emails.GetConnectLink(ctx, "ln_1"); return e }, "GET", "/v1/emails/connect/links/ln_1", "", ""},
		{"RevokeConnectLink", `{"status":"revoked"}`, func(c *Client) error { _, _, e := c.Emails.RevokeConnectLink(ctx, "ln_1"); return e }, "DELETE", "/v1/emails/connect/links/ln_1", "", ""},
		{"ListMailboxOAuthApps", `{"data":[],"callbacks":{}}`, func(c *Client) error { _, _, e := c.Emails.ListMailboxOAuthApps(ctx); return e }, "GET", "/v1/emails/oauth-apps", "", ""},
		{"PutMailboxOAuthApp", `{}`, func(c *Client) error {
			_, _, e := c.Emails.PutMailboxOAuthApp(ctx, ProviderGmail, &MailboxOAuthAppParams{ClientID: "id.apps.googleusercontent.com", ClientSecret: "s", AppName: "Acme"})
			return e
		}, "PUT", "/v1/emails/oauth-apps/gmail", "", `{"client_id":"id.apps.googleusercontent.com","client_secret":"s","app_name":"Acme"}`},
		{"DeleteMailboxOAuthApp", ``, func(c *Client) error { _, e := c.Emails.DeleteMailboxOAuthApp(ctx, ProviderOutlook); return e }, "DELETE", "/v1/emails/oauth-apps/outlook", "", ""},
		{"CreateClientLink", `{"link":{},"url":"https://p.example/c/x"}`, func(c *Client) error {
			_, _, e := c.Emails.CreateClientLink(ctx, &ClientConnectLinkParams{
				Label: "Acme", Providers: []string{ProviderGmail, ConnectLinkProviderVendor}, MaxMailboxes: 10, ExpiresInDays: 14,
				TagIDs: []string{"tg_1"}, Warmup: true,
			})
			return e
		}, "POST", "/v1/emails/client-links", "",
			`{"label":"Acme","providers":["gmail","vendor"],"max_mailboxes":10,"expires_in_days":14,"tag_ids":["tg_1"],"warmup":true}`},
		{"ListClientLinks", `{"data":[],"pagination":{"next_cursor":null,"has_more":false}}`, func(c *Client) error {
			_, e := c.Emails.ListClientLinks(ctx, &ListOptions{Limit: 5})
			return e
		}, "GET", "/v1/emails/client-links", "limit=5", ""},
		{"ListClientLinks nil", `{"data":[],"pagination":{"next_cursor":null,"has_more":false}}`, func(c *Client) error {
			_, e := c.Emails.ListClientLinks(ctx, nil)
			return e
		}, "GET", "/v1/emails/client-links", "", ""},
		{"ClientLinkUses", `{"data":[]}`, func(c *Client) error { _, _, e := c.Emails.ClientLinkUses(ctx, "cl_1"); return e }, "GET", "/v1/emails/client-links/cl_1/uses", "", ""},
		{"RevokeClientLink", `{}`, func(c *Client) error { _, _, e := c.Emails.RevokeClientLink(ctx, "cl_1"); return e }, "POST", "/v1/emails/client-links/cl_1/revoke", "", ""},
		{"WarmupFilterTag", `{"tag":null}`, func(c *Client) error { _, _, e := c.Emails.WarmupFilterTag(ctx, "em_1"); return e }, "GET", "/v1/emails/em_1/warmup-filter-tag", "", ""},
		{"SetWarmupFilterTag random", `{"tag":"abcd-efgh-jkmn"}`, func(c *Client) error { _, _, e := c.Emails.SetWarmupFilterTag(ctx, "em_1", ""); return e }, "PUT", "/v1/emails/em_1/warmup-filter-tag", "", ""},
		{"SetWarmupFilterTag chosen", `{"tag":"acme 2026"}`, func(c *Client) error { _, _, e := c.Emails.SetWarmupFilterTag(ctx, "em_1", "acme 2026"); return e }, "PUT", "/v1/emails/em_1/warmup-filter-tag", "", `{"tag":"acme 2026"}`},
		{"ClearWarmupFilterTag", `{"tag":null}`, func(c *Client) error { _, e := c.Emails.ClearWarmupFilterTag(ctx, "em_1"); return e }, "DELETE", "/v1/emails/em_1/warmup-filter-tag", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got = recordedRequest{}
			c := emailFixtureClient(t, 200, tc.resp, &got)
			if err := tc.call(c); err != nil {
				t.Fatalf("call: %v", err)
			}
			if got.method != tc.wantMethod || got.path != tc.wantPath {
				t.Errorf("got %s %s, want %s %s", got.method, got.path, tc.wantMethod, tc.wantPath)
			}
			if got.rawQuery != tc.wantQuery {
				t.Errorf("query = %q, want %q", got.rawQuery, tc.wantQuery)
			}
			if strings.TrimSpace(got.body) != tc.wantBody {
				t.Errorf("body = %q, want %q", got.body, tc.wantBody)
			}
		})
	}
}

func TestEmailConnectDecode(t *testing.T) {
	ctx := context.Background()

	t.Run("ConnectMailboxOAuth renewed", func(t *testing.T) {
		c := emailFixtureClient(t, 200, `{
			"mailbox": {"id": "0c0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b", "email": "a@example.com", "provider": "gmail", "status": "active"},
			"renewed": true,
			"warmup_note": "Warmup was left off: the domain fails SPF."
		}`, nil)
		r, resp, err := c.Emails.ConnectMailboxOAuth(ctx, &ConnectOAuthParams{MailboxOAuthGrant: MailboxOAuthGrant{Provider: ProviderGmail, RefreshToken: "rt"}})
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK || !r.Renewed || r.Mailbox.Email != "a@example.com" || r.Mailbox.Provider != ProviderGmail || !strings.HasPrefix(r.WarmupNote, "Warmup was left off") {
			t.Errorf("decoded %+v", r)
		}
	})

	t.Run("ConnectMailboxOAuthBulk", func(t *testing.T) {
		c := emailFixtureClient(t, 200, `{
			"data": [
				{"row": 0, "email": "a@example.com", "status": "connected", "id": "11111111-1111-1111-1111-111111111111"},
				{"row": 1, "email": "b@example.com", "status": "renewed", "id": "22222222-2222-2222-2222-222222222222"},
				{"row": 2, "email": "", "status": "failed", "code": "oauth_grant_invalid", "message": "The provider refused the refresh token."},
				{"row": 3, "email": "d@example.com", "status": "something_new"}
			],
			"summary": {"total": 4, "connected": 1, "renewed": 1, "failed": 1}
		}`, nil)
		r, _, err := c.Emails.ConnectMailboxOAuthBulk(ctx, &ConnectOAuthBulkParams{Grants: []MailboxOAuthGrant{{Provider: ProviderGmail}}})
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Data) != 4 || r.Data[1].Status != MailboxBulkRenewed || r.Data[2].Code != "oauth_grant_invalid" || r.Data[0].ID == nil ||
			r.Data[3].Status != "something_new" || r.Summary.Renewed != 1 || r.Summary.Total != 4 {
			t.Errorf("decoded %+v", r)
		}
	})

	t.Run("ConnectMailboxSMTPIMAPBulk", func(t *testing.T) {
		c := emailFixtureClient(t, 200, `{
			"data": [{"row": 0, "email": "a@example.com", "status": "skipped", "code": "already_connected"}],
			"summary": {"total": 1, "connected": 0, "skipped": 1, "failed": 0}
		}`, nil)
		r, _, err := c.Emails.ConnectMailboxSMTPIMAPBulk(ctx, &ConnectSMTPIMAPBulkParams{})
		if err != nil || r.Summary.Skipped != 1 || r.Data[0].Status != MailboxBulkSkipped || r.Allowance != nil {
			t.Errorf("decoded %+v, %v", r, err)
		}
	})

	t.Run("CreateConnectLink", func(t *testing.T) {
		c := emailFixtureClient(t, 201, `{
			"id": "33333333-3333-3333-3333-333333333333",
			"organization_id": "44444444-4444-4444-4444-444444444444",
			"created_by": "55555555-5555-5555-5555-555555555555",
			"provider": "outlook",
			"email": "owner@example.com",
			"warmup": {"enabled": true, "start": 2, "reply_rate": 30},
			"return_url": "https://partner.example/done",
			"status": "pending",
			"expires_at": "2026-11-18T10:00:00Z",
			"created_at": "2026-11-15T10:00:00Z",
			"url": "https://api.example/addresses/link/tok"
		}`, nil)
		l, resp, err := c.Emails.CreateConnectLink(ctx, &MailboxConnectLinkParams{Provider: ProviderOutlook})
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusCreated || l.Status != MailboxConnectLinkPending || l.Provider != ProviderOutlook ||
			l.Email != "owner@example.com" || !l.Warmup.Enabled || l.Warmup.Start == nil || *l.Warmup.Start != 2 || l.Warmup.Max != nil ||
			l.URL == "" || l.CompletedAt != nil || l.EmailAccountID != nil || l.ExpiresAt.Day() != 18 {
			t.Errorf("decoded %+v", l)
		}
	})

	t.Run("ListConnectLinks pages", func(t *testing.T) {
		var got recordedRequest
		c := recordingFixtureClient(t, &got, `{
			"data": [{"id": "l1", "provider": "gmail", "warmup": {"enabled": false}, "status": "connected",
				"email_account_id": "66666666-6666-6666-6666-666666666666", "completed_at": "2026-11-15T11:00:00Z",
				"expires_at": "2026-11-18T10:00:00Z", "created_at": "2026-11-15T10:00:00Z"}],
			"pagination": {"next_cursor": "next1", "has_more": true}
		}`)
		p, err := c.Emails.ListConnectLinks(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Data) != 1 || p.Data[0].EmailAccountID == nil || p.Data[0].CompletedAt == nil || !p.HasMore() || p.NextCursor() != "next1" {
			t.Errorf("decoded %+v", p)
		}
		if _, err := p.Next(ctx); err != nil {
			t.Fatal(err)
		}
		if got.rawQuery != "cursor=next1" {
			t.Errorf("next query = %q", got.rawQuery)
		}
	})

	t.Run("ListMailboxOAuthApps", func(t *testing.T) {
		c := emailFixtureClient(t, 200, `{
			"data": [{
				"id": "77777777-7777-7777-7777-777777777777",
				"organization_id": "44444444-4444-4444-4444-444444444444",
				"provider": "gmail",
				"client_id": "123.apps.googleusercontent.com",
				"has_secret": false,
				"app_name": "Acme Mail",
				"redirect_url": "https://api.example/v1/oauth/callback/gmail",
				"custom_redirect": false,
				"created_at": "2026-11-01T10:00:00Z",
				"updated_at": "2026-11-02T10:00:00Z",
				"mailboxes": 12
			}],
			"callbacks": {"gmail": "https://api.example/v1/oauth/callback/gmail", "outlook": "https://api.example/v1/oauth/callback/outlook"}
		}`, nil)
		r, _, err := c.Emails.ListMailboxOAuthApps(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Data) != 1 || r.Data[0].HasSecret || r.Data[0].Mailboxes != 12 || r.Data[0].CreatedBy != nil ||
			r.Callbacks[ProviderOutlook] != "https://api.example/v1/oauth/callback/outlook" {
			t.Errorf("decoded %+v", r)
		}
	})

	t.Run("ListMailboxOAuthApps null data", func(t *testing.T) {
		c := emailFixtureClient(t, 200, `{"data": null, "callbacks": {}}`, nil)
		r, _, err := c.Emails.ListMailboxOAuthApps(ctx)
		if err != nil || r.Data == nil {
			t.Errorf("decoded %+v, %v", r, err)
		}
	})

	t.Run("CreateClientLink", func(t *testing.T) {
		c := emailFixtureClient(t, 201, `{
			"link": {
				"id": "88888888-8888-8888-8888-888888888888",
				"organization_id": "44444444-4444-4444-4444-444444444444",
				"created_by": "55555555-5555-5555-5555-555555555555",
				"label": "Acme",
				"allowed_providers": ["gmail", "smtp_imap", "vendor"],
				"max_mailboxes": 25,
				"mailboxes_connected": 0,
				"tag_ids": [],
				"warmup": true,
				"expires_at": "2026-11-22T10:00:00Z",
				"revoked_at": null,
				"created_at": "2026-11-15T10:00:00Z",
				"last_used_at": null,
				"status": "active"
			},
			"url": "https://connect.example/c/tok"
		}`, nil)
		r, _, err := c.Emails.CreateClientLink(ctx, &ClientConnectLinkParams{Label: "Acme"})
		if err != nil {
			t.Fatal(err)
		}
		if r.URL == "" || r.Link.Status != ClientConnectLinkActive || len(r.Link.AllowedProviders) != 3 || r.Link.CreatedBy == nil ||
			r.Link.RevokedAt != nil || r.Link.LastUsedAt != nil || r.Link.MaxMailboxes != 25 || !r.Link.Warmup {
			t.Errorf("decoded %+v", r)
		}
	})

	t.Run("RevokeClientLink orphaned creator", func(t *testing.T) {
		c := emailFixtureClient(t, 200, `{"id": "l", "created_by": null, "revoked_at": "2026-11-15T12:00:00Z", "status": "revoked", "allowed_providers": ["outlook"], "tag_ids": null}`, nil)
		r, _, err := c.Emails.RevokeClientLink(ctx, "l")
		if err != nil || r.CreatedBy != nil || r.RevokedAt == nil || r.Status != ClientConnectLinkRevoked {
			t.Errorf("decoded %+v, %v", r, err)
		}
	})

	t.Run("ClientLinkUses", func(t *testing.T) {
		c := emailFixtureClient(t, 200, `{"data": [
			{"id": "u1", "link_id": "l", "email_account_id": "99999999-9999-9999-9999-999999999999", "email": "a@client.example", "provider": "smtp_imap", "connected_at": "2026-11-15T12:00:00Z"},
			{"id": "u2", "link_id": "l", "email_account_id": null, "email": "b@client.example", "provider": "gmail", "connected_at": "2026-11-15T12:05:00Z"}
		]}`, nil)
		uses, _, err := c.Emails.ClientLinkUses(ctx, "l")
		if err != nil || len(uses) != 2 || uses[0].EmailAccountID == nil || uses[1].EmailAccountID != nil || uses[0].Provider != ProviderSMTPIMAP {
			t.Errorf("decoded %+v, %v", uses, err)
		}
	})

	t.Run("WarmupFilterTag", func(t *testing.T) {
		c := emailFixtureClient(t, 200, `{"tag": "k7pq-3mzr-x2bd"}`, nil)
		r, _, err := c.Emails.WarmupFilterTag(ctx, "em_1")
		if err != nil || r.Tag == nil || *r.Tag != "k7pq-3mzr-x2bd" {
			t.Errorf("decoded %+v, %v", r, err)
		}
		c = emailFixtureClient(t, 200, `{"tag": null}`, nil)
		r, _, err = c.Emails.WarmupFilterTag(ctx, "em_1")
		if err != nil || r.Tag != nil {
			t.Errorf("decoded %+v, %v", r, err)
		}
	})

	t.Run("SetWarmupFilterTag refused", func(t *testing.T) {
		c := emailFixtureClient(t, 400, `{"error": "bad_request", "code": "invalid_filter_tag", "message": "a filter tag cannot describe the mail as warmup, automated or a test; use a code instead"}`, nil)
		_, _, err := c.Emails.SetWarmupFilterTag(ctx, "em_1", "warmup mail")
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest || apiErr.Code != "invalid_filter_tag" {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("WarmupSendFailure kinds", func(t *testing.T) {
		if WarmupFailureKindMailboxLoading != "mailbox_loading" || WarmupFailureKindProviderThrottle != "provider_throttle" {
			t.Error("kind constants drifted")
		}
	})
}
