package gateway

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestLongLivedCredentialsTravelInHeader(t *testing.T) {
	for _, tok := range []string{"wmbly_abc123", "wmat_abc123"} {
		c := New(tok, "org_1", WithURL("wss://realtime.example.com/socket/websocket?token=stale"))
		u, err := url.Parse(c.socketURL())
		if err != nil {
			t.Fatal(err)
		}
		if u.Query().Has("token") {
			t.Errorf("%s: token leaked into the URL: %s", tok, u)
		}
		if got := u.Query().Get("vsn"); got != protocolVersion {
			t.Errorf("%s: vsn = %q", tok, got)
		}
		if got := c.handshakeHeader().Get(CredentialHeader); got != tok {
			t.Errorf("%s: header = %q", tok, got)
		}
	}
}

func TestConnectionTokenStaysInQuery(t *testing.T) {
	c := New("eyJ.conn.token", "org_1")
	if h := c.handshakeHeader(); h != nil {
		t.Errorf("handshakeHeader() = %v, want nil", h)
	}
	if !strings.Contains(c.socketURL(), "token=eyJ.conn.token") {
		t.Errorf("socketURL() = %q", c.socketURL())
	}
}

func TestSyncEventPayloads(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		into any
		chk  func(t *testing.T, v any)
	}{
		{"opened", `{"event_type":"EMAIL_OPENED","org_id":"o","email_account_id":"a","client":"Gmail","client_type":"webmail","device_hidden":true,"device_type":"mobile","os":"iOS","browser":"Safari"}`,
			&EngagementEvent{}, func(t *testing.T, v any) {
				e := v.(*EngagementEvent)
				if e.ClientType != "webmail" || !e.DeviceHidden || e.OS != "iOS" || e.Browser != "Safari" || e.EmailAccountID != "a" || e.OrgID != "o" {
					t.Errorf("%+v", e)
				}
			}},
		{"progress", `{"event_type":"TASK_PROGRESS","campaign_id":"c","total_contacts":3,"total_emails":9,"processed_count":4}`,
			&TaskProgressEvent{}, func(t *testing.T, v any) {
				if e := v.(*TaskProgressEvent); e.TotalEmails != 9 || e.TotalContacts != 3 {
					t.Errorf("%+v", e)
				}
			}},
		{"import", `{"event_type":"MAILBOX_IMPORT_PROGRESS","org_id":"o","import_id":"i","status":"running"}`,
			&ImportProgressEvent{}, func(t *testing.T, v any) {
				if e := v.(*ImportProgressEvent); e.ImportID != "i" || e.Status != "running" {
					t.Errorf("%+v", e)
				}
			}},
		{"placement", `{"event_type":"PLACEMENT_TEST_UPDATED","org_id":"o","batch_id":"b","status":"completed"}`,
			&PlacementTestEvent{}, func(t *testing.T, v any) {
				if e := v.(*PlacementTestEvent); e.BatchID != "b" || e.Status != "completed" || e.TestID != "" {
					t.Errorf("%+v", e)
				}
			}},
		{"crm", `{"event_type":"CRM_SYNCED","org_id":"o","objects":["deals","tasks"],"contact_id":"c"}`,
			&CRMSyncedEvent{}, func(t *testing.T, v any) {
				if e := v.(*CRMSyncedEvent); len(e.Objects) != 2 || e.ContactID != "c" {
					t.Errorf("%+v", e)
				}
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(tc.raw), tc.into); err != nil {
				t.Fatal(err)
			}
			tc.chk(t, tc.into)
		})
	}
}

// Intents are matched by the server as substrings of the normalized event type;
// the new constants must hit their own events and stay off unrelated ones.
func TestNewIntentsMatchTheirEvents(t *testing.T) {
	match := func(intent, event string) bool { return strings.Contains(event, intent) }
	cases := []struct {
		intent string
		event  EventName
		want   bool
	}{
		{IntentWarmup, EventWarmupPlacement, true},
		{IntentEmail, EventDirectEmailOpened, true},
		{IntentDirectEmail, EventDirectEmailClicked, true},
		{IntentDirectEmail, EventEmailOpened, false},
		{IntentPlacementTest, EventPlacementTestUpdated, true},
		{IntentPlacementTest, EventWarmupPlacement, false},
		{IntentMailboxImport, EventMailboxImportProgress, true},
		{IntentAccount, EventMailboxImportProgress, false},
		{IntentContact, EventContactImportProgress, true},
		{IntentContactImport, EventContactImportProgress, true},
		{IntentContactImport, EventContactCreated, false},
		{IntentCRM, EventCRMSynced, true},
		{IntentCRM, EventCampaignStarted, false},
	}
	for _, tc := range cases {
		if got := match(tc.intent, tc.event); got != tc.want {
			t.Errorf("intent %q vs %s = %v, want %v", tc.intent, tc.event, got, tc.want)
		}
	}
}
