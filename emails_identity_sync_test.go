package warmbly

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

// wireCase is one call against a fixture server: the request it must make
// and the response the server sends back.
type wireCase struct {
	name      string
	status    int    // 0 means 200
	response  string // the body the server answers with
	call      func(c *Client) error
	method    string
	path      string
	rawQuery  string
	body      string // exact JSON, or "" for an empty body
	jsonEqual bool   // compare body as JSON rather than byte for byte
}

// runWireCases runs every case against its own fixture server and asserts
// the method, path, raw query and body of the request it made.
func runWireCases(t *testing.T, cases []wireCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got recordedRequest
			status := tc.status
			if status == 0 {
				status = http.StatusOK
			}
			c := emailFixtureClient(t, status, tc.response, &got)
			if err := tc.call(c); err != nil {
				t.Fatalf("call: %v", err)
			}
			if got.method != tc.method || got.path != tc.path {
				t.Errorf("got %s %s, want %s %s", got.method, got.path, tc.method, tc.path)
			}
			if got.rawQuery != tc.rawQuery {
				t.Errorf("raw query = %q, want %q", got.rawQuery, tc.rawQuery)
			}
			switch {
			case tc.body == "":
				if got.body != "" {
					t.Errorf("body = %q, want it empty", got.body)
				}
			case tc.jsonEqual:
				var a, b any
				if err := json.Unmarshal([]byte(got.body), &a); err != nil {
					t.Fatalf("request body %q is not JSON: %v", got.body, err)
				}
				_ = json.Unmarshal([]byte(tc.body), &b)
				ja, _ := json.Marshal(a)
				jb, _ := json.Marshal(b)
				if string(ja) != string(jb) {
					t.Errorf("body = %s, want %s", ja, jb)
				}
			default:
				if got.body != tc.body+"\n" && got.body != tc.body {
					t.Errorf("body = %q, want %q", got.body, tc.body)
				}
			}
		})
	}
}

func TestEmailIdentitySync(t *testing.T) {
	ctx := context.Background()
	// The shape models.SendIdentity marshals to.
	const identity = `{
		"supported": true,
		"provider": "gmail",
		"mailbox_email": "sam@acme.com",
		"send_as_email": "sales@acme.com",
		"identities": [
			{"email": "sam@acme.com", "name": "Sam", "is_primary": true, "is_default": false, "verified": true},
			{"email": "sales@acme.com", "name": "Acme Sales", "is_primary": false, "is_default": true, "verified": true},
			{"email": "old@acme.com", "name": "", "is_primary": false, "is_default": false, "verified": false}
		],
		"synced_at": "2026-10-01T08:00:00Z",
		"signature_source": "provider",
		"signature_imported_at": "2026-10-01T08:00:01Z"
	}`
	check := func(id *SendIdentity) error {
		if !id.Supported || id.SendAsEmail != "sales@acme.com" || len(id.Identities) != 3 ||
			!id.Identities[0].IsPrimary || !id.Identities[1].IsDefault || id.Identities[2].Verified ||
			id.SignatureSource != SignatureSourceProvider || id.SyncedAt == nil || id.SignatureImportedAt == nil {
			return errors.New("identity decoded wrong")
		}
		return nil
	}

	runWireCases(t, []wireCase{
		{
			name: "SendIdentity", response: identity,
			call: func(c *Client) error {
				id, _, err := c.Emails.SendIdentity(ctx, "em_1")
				if err != nil {
					return err
				}
				return check(id)
			},
			method: "GET", path: "/v1/emails/em_1/identity",
		},
		{
			name: "RefreshSendIdentity addresses only", response: identity,
			call: func(c *Client) error {
				id, _, err := c.Emails.RefreshSendIdentity(ctx, "em_1", false)
				if err != nil {
					return err
				}
				return check(id)
			},
			method: "POST", path: "/v1/emails/em_1/identity/refresh",
		},
		{
			name: "RefreshSendIdentity with signature", response: identity,
			call: func(c *Client) error {
				_, _, err := c.Emails.RefreshSendIdentity(ctx, "em_1", true)
				return err
			},
			method: "POST", path: "/v1/emails/em_1/identity/refresh", body: `{"import_signature":true}`,
		},
		{
			name: "SetDirectTracking", response: `{"track_direct_mail": true}`,
			call: func(c *Client) error {
				on, _, err := c.Emails.SetDirectTracking(ctx, "em_1", true)
				if err == nil && !on {
					err = errors.New("track_direct_mail not decoded")
				}
				return err
			},
			method: "PATCH", path: "/v1/emails/em_1/direct-tracking", body: `{"enabled":true}`,
		},
		{
			name: "SetDirectTracking off sends false", response: `{"track_direct_mail": false}`,
			call: func(c *Client) error {
				_, _, err := c.Emails.SetDirectTracking(ctx, "em_1", false)
				return err
			},
			method: "PATCH", path: "/v1/emails/em_1/direct-tracking", body: `{"enabled":false}`,
		},
		{
			name: "UpdateSyncFolders", response: `{"skip_folders": ["Archive/2019", "Newsletters"]}`,
			call: func(c *Client) error {
				got, _, err := c.Emails.UpdateSyncFolders(ctx, "em_1", []string{"archive/2019", "Newsletters"})
				if err == nil && (len(got) != 2 || got[0] != "Archive/2019") {
					err = errors.New("skip_folders not decoded")
				}
				return err
			},
			method: "PUT", path: "/v1/emails/em_1/sync", body: `{"skip_folders":["archive/2019","Newsletters"]}`,
		},
		{
			name: "UpdateSyncFolders nil clears with an empty list", response: `{"skip_folders": []}`,
			call: func(c *Client) error {
				got, _, err := c.Emails.UpdateSyncFolders(ctx, "em_1", nil)
				if err == nil && (got == nil || len(got) != 0) {
					err = errors.New("want a non-nil empty slice")
				}
				return err
			},
			method: "PUT", path: "/v1/emails/em_1/sync", body: `{"skip_folders":[]}`,
		},
		{
			name: "Update new fields", response: `{"id": "em_1"}`,
			call: func(c *Client) error {
				_, _, err := c.Emails.Update(ctx, "em_1", &EmailUpdateParams{
					SendAsEmail:         String(""),
					TestMode:            String(TestParticipationDiagnostic),
					TestSendEnabled:     Bool(true),
					TestReceiveEnabled:  Bool(false),
					SharedDailyLimit:    Int(80),
					WarmupPlacement:     String(WarmupPlacementArchive),
					WarmupRetentionDays: Int(0),
					RelayFolderMoves:    Bool(false),
					SendRecoveryResolution: &SendRecoveryResolution{
						HeldTaskID:            "task_1",
						HeldReason:            SendHeldUnknown,
						EvidenceType:          SendEvidenceConfirmedSent,
						ConfirmationReference: "ticket 42",
						MessageID:             "<a@b>",
					},
				})
				return err
			},
			method: "PATCH", path: "/v1/emails/em_1", jsonEqual: true,
			body: `{"send_as_email":"","test_mode":"diagnostic","test_send_enabled":true,"test_receive_enabled":false,
				"shared_daily_limit":80,"warmup_placement":"archive","warmup_retention_days":0,"relay_folder_moves":false,
				"send_recovery_resolution":{"held_task_id":"task_1","held_reason":"unknown","evidence_type":"operator_confirmed_sent",
				"confirmation_reference":"ticket 42","message_id":"<a@b>"}}`,
		},
	})
}

func TestEmailSyncFolderDecode(t *testing.T) {
	// The shape of the handler's emailSyncResponse.
	const body = `{
		"state": {
			"backfill_status": "running",
			"backfill_cursor": {"folders": {"INBOX": {"uid": 4100}}, "google_recovery": {"history_id": 991, "since": "2026-10-01T00:00:00Z", "messages_done": false}},
			"backfill_synced": 1200,
			"deferred": 0,
			"folders_skipped_cap": 3,
			"folders_skipped_conflict": 1
		},
		"policy": {"backfill_days": 90, "backfill_messages": 5000, "daily_messages": 2000, "org_daily_messages": 20000, "skip_folders": ["Newsletters"]},
		"skip_folders": ["Newsletters"],
		"folders": [{"name": "INBOX", "folder": "inbox"}, {"name": "Newsletters", "folder": ""}]
	}`
	c := emailFixtureClient(t, http.StatusOK, body, nil)
	s, _, err := c.Emails.SyncStatus(context.Background(), "em_1")
	if err != nil {
		t.Fatalf("SyncStatus: %v", err)
	}
	if s.State == nil || s.State.FoldersSkippedCap != 3 || s.State.FoldersSkippedConflict != 1 ||
		s.State.BackfillCursor.GoogleRecovery == nil || s.State.BackfillCursor.GoogleRecovery.HistoryID != 991 {
		t.Errorf("state = %+v", s.State)
	}
	if len(s.SkipFolders) != 1 || len(s.Policy.SkipFolders) != 1 || len(s.Folders) != 2 || s.Folders[0].Folder != "inbox" {
		t.Errorf("folders = %+v / %+v / %+v", s.SkipFolders, s.Policy.SkipFolders, s.Folders)
	}
}

func TestEmailDecodeNewMailboxFields(t *testing.T) {
	// A delegated Google Workspace mailbox in diagnostic test mode, with the
	// fields models.Email gained since the previous sync.
	const body = `{
		"id": "0c0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b",
		"email": "sam@acme.com",
		"send_as_email": "sales@acme.com",
		"provider": "gmail",
		"status": "active",
		"mail_host": "google_workspace",
		"auth_method": "delegated",
		"domain_grant_id": "6a1d1c56-0e2f-4b1d-9c55-5f7d0f7f1a00",
		"vendor_connection_id": "7b2e2d67-1f30-4c2e-8d66-6a8e1a8a2b11",
		"vendor": "acme_inboxes",
		"avatar_url": "https://cdn.example/a.png",
		"last_synced_at": "2026-10-10T10:00:00Z",
		"campaign_limit": 40,
		"test_mode": "diagnostic",
		"test_send_enabled": true,
		"test_receive_enabled": false,
		"shared_daily_limit": null,
		"rolling_recipient_limit": 120,
		"track_direct_mail": true,
		"warmup": null,
		"warmup_placement": "folder",
		"warmup_folder": "Warmup",
		"warmup_retention_days": 14,
		"relay_folder_moves": true,
		"tags": [],
		"created_at": "2026-05-19T18:00:00Z",
		"updated_at": "2026-06-11T09:14:00Z"
	}`
	c := emailFixtureClient(t, http.StatusOK, body, nil)
	em, _, err := c.Emails.Get(context.Background(), "em_1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if em.SendAsEmail != "sales@acme.com" || em.AuthMethod != MailAuthDelegated || em.MailHost != "google_workspace" ||
		em.DomainGrantID == nil || em.VendorConnectionID == nil || em.Vendor != "acme_inboxes" || em.AvatarURL == "" {
		t.Errorf("identity fields = %+v", em)
	}
	if em.TestMode == nil || *em.TestMode != TestParticipationDiagnostic || !em.TestSendEnabled || em.TestReceiveEnabled ||
		em.SharedDailyLimit != nil || em.RollingRecipientLimit == nil || *em.RollingRecipientLimit != 120 {
		t.Errorf("test participation fields = %+v", em)
	}
	if !em.TrackDirectMail || em.WarmupPlacement != WarmupPlacementFolder || em.WarmupFolder != "Warmup" ||
		em.WarmupRetentionDays != 14 || !em.RelayFolderMoves {
		t.Errorf("filing fields = %+v", em)
	}
	if em.LastSyncedAt == nil || !em.LastSyncedAt.Equal(time.Date(2026, 10, 10, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("last_synced_at = %v", em.LastSyncedAt)
	}
}

func TestEmailSendAsRefusal(t *testing.T) {
	c := emailFixtureClient(t, http.StatusBadRequest,
		`{"code":"mailbox_send_as_unknown","message":"That address is not one your provider has verified this mailbox to send as."}`, nil)
	_, _, err := c.Emails.Update(context.Background(), "em_1", &EmailUpdateParams{SendAsEmail: String("x@acme.com")})
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != ErrCodeSendAsUnknown {
		t.Fatalf("err = %v, want code %q", err, ErrCodeSendAsUnknown)
	}
}
