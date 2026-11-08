package warmbly

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestEmailMailboxSyncRouting(t *testing.T) {
	ctx := context.Background()
	var got recordedRequest

	cases := []struct {
		name       string
		resp       string
		call       func(c *Client) error
		wantMethod string
		wantPath   string
		wantBody   string
	}{
		{"BulkDelete", `{"succeeded":1,"failed":0,"results":[]}`, func(c *Client) error { _, _, e := c.Emails.BulkDelete(ctx, []string{"a", "b"}); return e }, "POST", "/v1/emails/bulk/delete", `{"email_ids":["a","b"]}`},
		{"BulkSetStatus", `{}`, func(c *Client) error {
			_, _, e := c.Emails.BulkSetStatus(ctx, []string{"a"}, MailboxStatusInactive)
			return e
		}, "POST", "/v1/emails/bulk/status", `{"email_ids":["a"],"status":"inactive"}`},
		{"BulkWarmup", `{}`, func(c *Client) error { _, _, e := c.Emails.BulkWarmup(ctx, []string{"a"}, BulkWarmupPause); return e }, "POST", "/v1/emails/bulk/warmup", `{"email_ids":["a"],"action":"pause"}`},
		{"Identity", `{}`, func(c *Client) error { _, _, e := c.Emails.Identity(ctx, "em_1"); return e }, "GET", "/v1/emails/em_1/identity", ""},
		{"RefreshIdentity", `{}`, func(c *Client) error { _, _, e := c.Emails.RefreshIdentity(ctx, "em_1", true); return e }, "POST", "/v1/emails/em_1/identity/refresh", `{"import_signature":true}`},
		{"Certificates", `{}`, func(c *Client) error { _, _, e := c.Emails.Certificates(ctx, "em_1"); return e }, "GET", "/v1/emails/em_1/certificates", ""},
		{"ResumeSending", `{}`, func(c *Client) error { _, _, e := c.Emails.ResumeSending(ctx, "em_1"); return e }, "POST", "/v1/emails/em_1/resume-sending", ""},
		{"UpdateSyncSettings", `{"skip_folders":["Junk"]}`, func(c *Client) error {
			_, _, e := c.Emails.UpdateSyncSettings(ctx, "em_1", &SyncSettingsParams{SkipFolders: []string{"Junk"}})
			return e
		}, "PUT", "/v1/emails/em_1/sync", `{"skip_folders":["Junk"]}`},
		{"SetDirectTracking", `{"track_direct_mail":true}`, func(c *Client) error { _, _, e := c.Emails.SetDirectTracking(ctx, "em_1", true); return e }, "PATCH", "/v1/emails/em_1/direct-tracking", `{"enabled":true}`},
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
			if strings.TrimSpace(got.body) != tc.wantBody {
				t.Errorf("body = %q, want %q", got.body, tc.wantBody)
			}
		})
	}
}

func TestEmailMailboxSyncDecode(t *testing.T) {
	ctx := context.Background()

	c := emailFixtureClient(t, 200, `{"succeeded":1,"failed":1,"results":[{"id":"a","ok":true},{"id":"b","ok":false,"code":"not_found","message":"gone"}]}`, nil)
	r, _, err := c.Emails.BulkDelete(ctx, []string{"a", "b"})
	if err != nil || r.Succeeded != 1 || r.Failed != 1 || r.Results[1].Code != "not_found" || r.Results[0].OK != true {
		t.Fatalf("BulkDelete = %+v, %v", r, err)
	}

	c = emailFixtureClient(t, 200, `{"supported":true,"provider":"gmail","mailbox_email":"a@x.test","send_as_email":"b@x.test","identities":[{"email":"b@x.test","name":"B","is_primary":false,"is_default":true,"verified":true}],"synced_at":"2026-11-01T10:00:00Z","signature_source":"provider"}`, nil)
	id, _, err := c.Emails.Identity(ctx, "em_1")
	if err != nil || !id.Supported || id.Identities[0].Email != "b@x.test" || id.SyncedAt == nil || id.SignatureImportedAt != nil || id.SignatureSource != SignatureSourceProvider {
		t.Fatalf("Identity = %+v, %v", id, err)
	}

	c = emailFixtureClient(t, 200, `{"smtp":{"host":"smtp.x.test","port":465,"tls_unverified":true,"tls_pin":"ab","observed":{"fingerprint":"ff","self_signed":true}},"imap":{"host":"imap.x.test","port":993,"tls_unverified":false}}`, nil)
	cert, _, err := c.Emails.Certificates(ctx, "em_1")
	if err != nil || !cert.SMTP.TLSUnverified || cert.SMTP.Observed == nil || !cert.SMTP.Observed.SelfSigned || cert.IMAP.Observed != nil {
		t.Fatalf("Certificates = %+v, %v", cert, err)
	}

	c = emailFixtureClient(t, 200, `{"hold_released":true,"cooldown_cleared":false}`, nil)
	rs, _, err := c.Emails.ResumeSending(ctx, "em_1")
	if err != nil || !rs.HoldReleased || rs.CooldownCleared {
		t.Fatalf("ResumeSending = %+v, %v", rs, err)
	}

	c = emailFixtureClient(t, 200, `{"state":null,"policy":{"backfill_days":90,"backfill_messages":5000,"daily_messages":500,"org_daily_messages":5000,"skip_folders":["Junk"]},"skip_folders":["Junk"],"folders":[{"name":"INBOX","folder":"inbox"},{"name":"Junk","folder":"spam"}]}`, nil)
	sy, _, err := c.Emails.SyncStatus(ctx, "em_1")
	if err != nil || sy.State != nil || sy.SkipFolders[0] != "Junk" || len(sy.Folders) != 2 || sy.Folders[1].Folder != "spam" || sy.Policy.SkipFolders[0] != "Junk" {
		t.Fatalf("SyncStatus = %+v, %v", sy, err)
	}

	c = emailFixtureClient(t, 200, `{"skip_folders":null}`, nil)
	sk, _, err := c.Emails.UpdateSyncSettings(ctx, "em_1", &SyncSettingsParams{})
	if err != nil || sk == nil || len(sk) != 0 {
		t.Fatalf("UpdateSyncSettings = %v, %v", sk, err)
	}

	c = emailFixtureClient(t, 200, `{"track_direct_mail":false}`, nil)
	if v, _, err := c.Emails.SetDirectTracking(ctx, "em_1", false); err != nil || v {
		t.Fatalf("SetDirectTracking = %v, %v", v, err)
	}
}

func TestEmailStructShape(t *testing.T) {
	const body = `{"id":"e1","user_id":"u","worker_id":null,"email":"a@x.test","send_as_email":"b@x.test","provider":"gmail","status":"active",
		"mail_host":"google_workspace","auth_method":"delegated","domain_grant_id":"g1","vendor_connection_id":"v1","vendor":"acme","avatar_url":"",
		"campaign_limit":50,"test_mode":null,"test_send_enabled":true,"test_receive_enabled":false,"shared_daily_limit":100,"rolling_recipient_limit":null,
		"min_wait_time":600,"reply_to":"","track_direct_mail":true,"warmup_placement":"folder","warmup_folder":"Warmup","warmup_retention_days":14,
		"relay_folder_moves":true,"tags":[]}`
	var e Email
	if err := json.Unmarshal([]byte(body), &e); err != nil {
		t.Fatal(err)
	}
	if e.SendAsEmail != "b@x.test" || e.AuthMethod != AuthMethodDelegated || e.DomainGrantID == nil || *e.DomainGrantID != "g1" || e.TestMode != nil ||
		!e.TestSendEnabled || e.SharedDailyLimit == nil || *e.SharedDailyLimit != 100 || e.RollingRecipientLimit != nil || !e.TrackDirectMail ||
		e.WarmupPlacement != WarmupPlacementFolder || e.WarmupRetentionDays != 14 || !e.RelayFolderMoves || e.Vendor != "acme" {
		t.Errorf("email = %+v", e)
	}

	out, err := json.Marshal(EmailUpdateParams{SendAsEmail: String("b@x.test"), TestMode: String(TestModeOff), SharedDailyLimit: Int(0), WarmupPlacement: String(WarmupPlacementInbox), RelayFolderMoves: Bool(false)})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"send_as_email":"b@x.test","test_mode":"off","shared_daily_limit":0,"warmup_placement":"inbox","relay_folder_moves":false}` {
		t.Errorf("update params = %s", out)
	}
}
