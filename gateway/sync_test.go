package gateway

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestNewEventPayloads decodes the payload shapes added with the placement,
// import and CRM events.
func TestNewEventPayloads(t *testing.T) {
	t.Run("warmup placement", func(t *testing.T) {
		var e AccountEvent
		decodeInto(t, `{"event_type":"WARMUP_PLACEMENT","org_id":"o_1","email_account_id":"a_1","email":"ana@example.com","status":"rescue_moved","timestamp":"2026-10-01T09:00:00Z"}`, &e)
		if e.EventType != EventWarmupPlacement || e.EmailAccountID != "a_1" || e.Email != "ana@example.com" {
			t.Fatalf("decoded = %+v", e)
		}
		if e.Status != WarmupPlacementRescueMoved {
			t.Errorf("Status = %q, want %q", e.Status, WarmupPlacementRescueMoved)
		}
	})

	t.Run("placement test", func(t *testing.T) {
		var e PlacementTestEvent
		decodeInto(t, `{"event_type":"PLACEMENT_TEST_UPDATED","org_id":"o_1","test_id":"t_1","campaign_id":"c_1","status":"running","seq":9}`, &e)
		if e.TestID != "t_1" || e.CampaignID != "c_1" || e.Status != PlacementTestRunning || e.BatchID != "" || e.Seq != 9 {
			t.Errorf("decoded = %+v", e)
		}

		var batch PlacementTestEvent
		decodeInto(t, `{"event_type":"PLACEMENT_TEST_UPDATED","org_id":"o_1","batch_id":"b_1","status":"completed_with_warnings"}`, &batch)
		if batch.BatchID != "b_1" || batch.TestID != "" || batch.Status != PlacementBatchCompletedWithWarnings {
			t.Errorf("batch decoded = %+v", batch)
		}
	})

	t.Run("import progress", func(t *testing.T) {
		for _, tc := range []struct{ name, status string }{
			{EventMailboxImportProgress, ImportStatusCompleted},
			{EventContactImportProgress, ImportStatusQueued},
		} {
			var e ImportProgressEvent
			decodeInto(t, `{"event_type":"`+tc.name+`","org_id":"o_1","import_id":"i_1","status":"`+tc.status+`"}`, &e)
			if e.EventType != tc.name || e.ImportID != "i_1" || e.Status != tc.status {
				t.Errorf("%s decoded = %+v", tc.name, e)
			}
		}
	})

	t.Run("crm synced", func(t *testing.T) {
		var e CRMSyncedEvent
		decodeInto(t, `{"event_type":"CRM_SYNCED","org_id":"o_1","objects":["deal","task","note"],"contact_id":"ct_1"}`, &e)
		if len(e.Objects) != 3 || e.Objects[0] != CRMObjectDeal || e.ContactID != "ct_1" {
			t.Errorf("decoded = %+v", e)
		}

		var bare CRMSyncedEvent
		decodeInto(t, `{"event_type":"CRM_SYNCED","objects":["owner"]}`, &bare)
		if bare.ContactID != "" || bare.Objects[0] != CRMObjectOwner {
			t.Errorf("decoded = %+v", bare)
		}
	})

	t.Run("direct email opened", func(t *testing.T) {
		var e EngagementEvent
		decodeInto(t, `{"event_type":"DIRECT_EMAIL_OPENED","org_id":"o_1","email_account_id":"a_1","machine":false,
			"occurred_at":"2026-10-01T08:59:00Z","timestamp":"2026-10-01T09:00:00Z",
			"client":"Gmail","client_type":"webmail","device_hidden":true,"device_type":"desktop",
			"os":"macOS","browser":"Safari","country_code":"GB","city":"London"}`, &e)
		if e.CampaignID != "" {
			t.Errorf("CampaignID = %q, want empty on a direct-send event", e.CampaignID)
		}
		if e.EmailAccountID != "a_1" || e.ClientType != "webmail" || !e.DeviceHidden {
			t.Errorf("origin = %+v", e)
		}
		if e.OS != "macOS" || e.Browser != "Safari" || e.DeviceType != "desktop" {
			t.Errorf("device = %+v", e)
		}
	})

	t.Run("task progress total emails", func(t *testing.T) {
		var e TaskProgressEvent
		decodeInto(t, `{"event_type":"TASK_PROGRESS","campaign_id":"c_1","task_id":"k_1","progress":50,"total_contacts":10,"total_emails":30,"processed_count":15}`, &e)
		if e.TotalContacts != 10 || e.TotalEmails != 30 || e.ProcessedCount != 15 {
			t.Errorf("decoded = %+v", e)
		}
	})
}

// TestUnknownEventsStillDispatch: a server adds event types without notice, and
// a handler registered with HandleAny must receive one this SDK has never heard
// of, with its payload intact.
func TestUnknownEventsStillDispatch(t *testing.T) {
	srv := newPhxServer(t)
	srv.onJoin = func(int, string) (string, string) { return "ok", helloResponse }

	got := make(chan *Event, 4)
	c := srv.client(t)
	c.HandleAny(func(_ context.Context, e *Event) {
		if !strings.HasPrefix(e.Type, "__") {
			got <- e
		}
	})
	typed := make(chan *PlacementTestEvent, 1)
	On(c, EventPlacementTestUpdated, func(_ context.Context, p *PlacementTestEvent) { typed <- p })

	if err := c.Open(testContext(t)); err != nil {
		t.Fatalf("Open: %v", err)
	}
	srv.push("org:org_1", "SOMETHING_NEW_ENTIRELY", `{"event_type":"SOMETHING_NEW_ENTIRELY","seq":7,"extra":{"a":1}}`)
	srv.push("org:org_1", EventPlacementTestUpdated, `{"event_type":"PLACEMENT_TEST_UPDATED","test_id":"t_1","status":"completed","seq":8}`)

	select {
	case e := <-got:
		if e.Type != "SOMETHING_NEW_ENTIRELY" || e.Seq != 7 || !strings.Contains(string(e.Raw), `"extra"`) {
			t.Errorf("unknown event = %+v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("unknown event never reached HandleAny")
	}
	select {
	case p := <-typed:
		if p.TestID != "t_1" || p.Status != PlacementTestCompleted {
			t.Errorf("typed payload = %+v", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("placement event never reached its typed handler")
	}
}

// serverIntentMatch mirrors the gateway's intent test: both sides are
// normalized, then compared as a substring.
func serverIntentMatch(intent, eventType string) bool {
	norm := func(s string) string {
		return strings.NewReplacer(".", "_", ":", "_", "-", "_", " ", "_").Replace(strings.ToUpper(s))
	}
	return strings.Contains(norm(eventType), norm(intent))
}

// TestIntentsMatchTheirFamilies pins which events each new intent selects, and
// that none of them leaks into an unrelated family. The server matches by
// substring, so an intent token that is too short quietly widens the stream.
func TestIntentsMatchTheirFamilies(t *testing.T) {
	all := []string{
		EventEmailSent, EventEmailFailed, EventEmailOpened, EventEmailClicked, EventEmailReplied,
		EventDirectEmailOpened, EventDirectEmailClicked,
		EventEmailReceived, EventEmailUpdated, EventEmailDeleted,
		EventCampaignCreated, EventCampaignStarted, EventCampaignIdle,
		EventContactCreated, EventContactsReload, EventContactImportProgress,
		EventAccountConnected, EventAccountHealthChanged, EventAccountSyncState,
		EventWarmupPlacement, EventPlacementTestUpdated,
		EventMailboxImportProgress, EventCRMSynced,
		EventBulkStarted, EventBulkProgress,
		EventTaskProgress, EventAutomationRun, EventMeetingBooked,
		EventAIResearchProgress, EventAIDraftReady, EventBillingCreditsLow,
		EventNotificationCreated, EventAuditCreated, EventPageHit,
		EventFormSubmissionCreated, EventCustom,
	}
	cases := []struct {
		intent string
		want   []string
	}{
		{IntentPlacement, []string{EventWarmupPlacement, EventPlacementTestUpdated}},
		{IntentImport, []string{EventContactImportProgress, EventMailboxImportProgress}},
		{IntentCRM, []string{EventCRMSynced}},
		{IntentDirectEmail, []string{EventDirectEmailOpened, EventDirectEmailClicked}},
		{IntentWarmup, []string{EventWarmupPlacement}},
		{IntentAI, []string{EventAIResearchProgress, EventAIDraftReady}},
	}
	for _, tc := range cases {
		t.Run(tc.intent, func(t *testing.T) {
			var got []string
			for _, e := range all {
				if serverIntentMatch(tc.intent, e) {
					got = append(got, e)
				}
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("intent %q selects %v, want %v", tc.intent, got, tc.want)
			}
		})
	}

	// The wider families keep covering the new events they always implied.
	for intent, event := range map[string]string{
		IntentEmail:   EventDirectEmailOpened,
		IntentContact: EventContactImportProgress,
		IntentAccount: EventAccountSyncState,
	} {
		if !serverIntentMatch(intent, event) {
			t.Errorf("intent %q should select %s", intent, event)
		}
	}
}

// TestHandshakeCarriesCredentialInHeader: a key or token in the URL is refused
// by the gateway, so the dial must put it in the header and keep it out of the
// query.
func TestHandshakeCarriesCredentialInHeader(t *testing.T) {
	srv := newPhxServer(t)
	srv.onJoin = func(int, string) (string, string) { return "ok", helloResponse }

	c := srv.client(t)
	if err := c.Open(testContext(t)); err != nil {
		t.Fatalf("Open: %v", err)
	}
	srv.mu.Lock()
	header, query := srv.lastHeader, srv.lastQuery
	srv.mu.Unlock()

	if got := header.Get("X-Warmbly-Token"); got != "wmbly_test" {
		t.Errorf("X-Warmbly-Token = %q, want wmbly_test", got)
	}
	if strings.Contains(query, "wmbly_test") || strings.Contains(query, "token=") {
		t.Errorf("query = %q, want the credential kept out of the URL", query)
	}
	if !strings.Contains(query, "vsn="+protocolVersion) {
		t.Errorf("query = %q, want the wire version", query)
	}
}

// TestHandshakeRefusalCarriesTheServersReason: a refused upgrade answers 403
// with {code, reason}, which must reach the caller rather than collapsing into
// a bare status sentinel.
func TestHandshakeRefusalCarriesTheServersReason(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantCode   int
		wantReason string
		sentinel   error
		permanent  bool
	}{
		{"credential in the URL", `{"code":4003,"reason":"Send API keys and OAuth tokens in the x-warmbly-token header, not the URL"}`, JoinCodeUnauthenticated, "x-warmbly-token", ErrUnauthorized, true},
		{"ticket already used", `{"code":4004,"reason":"This connection token was already used; mint a new one for each connection"}`, JoinCodeAuthFailed, "already used", ErrUnauthorized, true},
		{"ip not allowed", `{"code":4010,"reason":"IP address not allowed"}`, JoinCodePermissionDenied, "IP address", ErrForbidden, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newPhxServer(t)
			srv.upgradeStatus = http.StatusForbidden
			srv.upgradeBody = tc.body

			err := srv.client(t, WithReconnectBackoff(time.Hour, time.Hour)).Open(testContext(t))
			var je *JoinError
			if !errors.As(err, &je) {
				t.Fatalf("Open error = %v, want a *JoinError", err)
			}
			if je.Code != tc.wantCode || !strings.Contains(je.Reason, tc.wantReason) {
				t.Errorf("refusal = code %d reason %q", je.Code, je.Reason)
			}
			if tc.sentinel != nil && !errors.Is(err, tc.sentinel) {
				t.Errorf("errors.Is(%v, %v) = false", err, tc.sentinel)
			}
			if je.Permanent() != tc.permanent {
				t.Errorf("Permanent() = %v, want %v", je.Permanent(), tc.permanent)
			}
		})
	}
}

// TestRateLimitedHandshakeIsRetried: a handshake refused for rate limiting is
// not fatal, and the wait the server quoted is honored.
func TestRateLimitedHandshakeIsRetried(t *testing.T) {
	je := parseHandshakeRefusal([]byte(`{"code":4007,"reason":"Rate limited","retry_after_ms":21000}`))
	if je == nil || je.Code != JoinCodeRateLimited || je.RetryAfter != 21*time.Second {
		t.Fatalf("parsed = %+v", je)
	}
	fatal, cooldown := New("wmbly_k", "org_1").classify(je)
	if fatal || cooldown != 21*time.Second {
		t.Errorf("classify = fatal %v cooldown %v, want a retry after 21s", fatal, cooldown)
	}
}

// TestHandshakeRefusalWithoutBodyFallsBack keeps the old behavior for a
// gateway or proxy that sends no body.
func TestHandshakeRefusalWithoutBodyFallsBack(t *testing.T) {
	if err := handshakeError(&http.Response{StatusCode: http.StatusForbidden}); !errors.Is(err, ErrForbidden) {
		t.Errorf("403 without a body = %v, want ErrForbidden", err)
	}
	if err := handshakeError(&http.Response{StatusCode: http.StatusUnauthorized}); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("401 without a body = %v, want ErrUnauthorized", err)
	}
	if err := handshakeError(&http.Response{StatusCode: http.StatusBadGateway}); err != nil {
		t.Errorf("502 = %v, want nil so the transport error is reported", err)
	}
	if je := parseHandshakeRefusal([]byte(`<html>forbidden</html>`)); je != nil {
		t.Errorf("html body parsed as %+v", je)
	}
}
