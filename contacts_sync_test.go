package warmbly

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestContactSyncRouting asserts method, path and, where it matters, the
// query and body of every contact method added or changed in the sync against
// the current server API.
func TestContactSyncRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()
	before := time.Date(2026, 6, 9, 11, 42, 0, 0, time.UTC)

	cases := []struct {
		name       string
		call       func() error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   []string
	}{
		{"VerificationOverview", func() error {
			_, _, e := c.Contacts.VerificationOverview(ctx)
			return e
		}, "GET", "/v1/contacts/verification", "", nil},
		{"RequestVerification", func() error {
			_, _, e := c.Contacts.RequestVerification(ctx, &ContactVerificationParams{
				Action:     VerificationActionVerify,
				Contacts:   []string{"ct_1"},
				CampaignID: "camp_1",
			})
			return e
		}, "POST", "/v1/contacts/verification", "", []string{
			`"action":"verify"`, `"contacts":["ct_1"]`, `"campaign_id":"camp_1"`,
		}},
		{"CampaignStates", func() error {
			_, _, e := c.Contacts.CampaignStates(ctx, "ct_1")
			return e
		}, "GET", "/v1/contacts/ct_1/campaigns", "", nil},
		{"Segments", func() error {
			_, _, e := c.Contacts.Segments(ctx, "ct_1")
			return e
		}, "GET", "/v1/contacts/ct_1/segments", "", nil},
		{"ListTimeline cursor", func() error {
			_, e := c.Contacts.ListTimeline(ctx, "ct_1", &TimelineParams{
				ListOptions: ListOptions{Limit: 25, Cursor: "abc"},
			})
			return e
		}, "GET", "/v1/contacts/ct_1/timeline", "cursor=abc&limit=25", nil},
		{"ListTimeline before", func() error {
			_, e := c.Contacts.ListTimeline(ctx, "ct_1", &TimelineParams{Before: &before})
			return e
		}, "GET", "/v1/contacts/ct_1/timeline", "before=2026-06-09T11%3A42%3A00Z", nil},
		{"ListTimeline nil params", func() error {
			_, e := c.Contacts.ListTimeline(ctx, "ct_1", nil)
			return e
		}, "GET", "/v1/contacts/ct_1/timeline", "", nil},
		{"Timeline (legacy)", func() error {
			_, _, e := c.Contacts.Timeline(ctx, "ct_1")
			return e
		}, "GET", "/v1/contacts/ct_1/timeline", "", nil},
		{"Search with new filters", func() error {
			_, e := c.Contacts.Search(ctx, &ContactSearchParams{
				CampaignIDs:        []string{"camp_1"},
				LeadStatus:         LeadStatusUndeliverable,
				Engagement:         LeadEngagementNotOpened,
				SegmentIDs:         []string{"seg_1"},
				VerificationStatus: VerifyStatusRisky,
			})
			return e
		}, "POST", "/v1/contacts/search", "", []string{
			`"lead_status":"undeliverable"`, `"engagement":"not_opened"`,
			`"segment_ids":["seg_1"]`, `"verification_status":"risky"`,
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got = recordedRequest{}
			if err := tc.call(); err != nil {
				t.Fatalf("call: %v", err)
			}
			if got.method != tc.wantMethod || got.path != tc.wantPath {
				t.Fatalf("got %s %s, want %s %s", got.method, got.path, tc.wantMethod, tc.wantPath)
			}
			if got.rawQuery != tc.wantQuery {
				t.Fatalf("query = %q, want %q", got.rawQuery, tc.wantQuery)
			}
			for _, frag := range tc.wantBody {
				if !strings.Contains(got.body, frag) {
					t.Errorf("body %s missing %s", got.body, frag)
				}
			}
		})
	}
}

// arrayRecordingClient is routingClient for the endpoints that answer with a
// bare JSON array (POST /contacts), which the shared envelope fixture cannot
// satisfy.
func arrayRecordingClient(t *testing.T, got *recordedRequest) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.path = r.URL.Path
		got.rawQuery = r.URL.RawQuery
		body, _ := io.ReadAll(r.Body)
		got.body = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// TestContactCreateSendsNewFields asserts the create body carries the fields
// the server gained: segments, subscribed, an imported verification verdict
// with its vocabulary, and a claimed source.
func TestContactCreateSendsNewFields(t *testing.T) {
	var got recordedRequest
	c := arrayRecordingClient(t, &got)
	_, _, err := c.Contacts.Create(context.Background(), []ContactInput{{
		Email:                "ada@example.com",
		Segments:             []string{"seg_1"},
		Subscribed:           Bool(false),
		VerificationStatus:   "ok",
		VerificationProvider: VerificationProviderZeroBounce,
		Source:               ContactSourceCampaign,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got.method != "POST" || got.path != "/v1/contacts" {
		t.Fatalf("got %s %s, want POST /v1/contacts", got.method, got.path)
	}
	for _, frag := range []string{
		`"segments":["seg_1"]`, `"subscribed":false`, `"verification_status":"ok"`,
		`"verification_provider":"zerobounce"`, `"source":"campaign"`,
	} {
		if !strings.Contains(got.body, frag) {
			t.Errorf("body %s missing %s", got.body, frag)
		}
	}
}

// TestContactInputOmitsUnsetOptionalFields guards the pointer/omitempty
// contract: a minimal input must not send explicit nulls or zero values the
// server would act on (a false "subscribed" would unsubscribe).
func TestContactInputOmitsUnsetOptionalFields(t *testing.T) {
	var got recordedRequest
	c := arrayRecordingClient(t, &got)
	if _, _, err := c.Contacts.Create(context.Background(), []ContactInput{{Email: "a@b.com"}}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"subscribed", "segments", "verification_status", "verification_provider", "source"} {
		if strings.Contains(got.body, `"`+key+`"`) {
			t.Errorf("body %s should not carry %q when unset", got.body, key)
		}
	}
}

// fixtureClient serves one canned JSON body per path.
func fixtureClient(t *testing.T, fixtures map[string]string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := fixtures[r.URL.Path]
		if !ok {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

const timelineFixture = `{
  "data": [
    {
      "type": "email_clicked",
      "at": "2026-06-09T11:42:00Z",
      "email_account_id": "e1",
      "campaign_id": "c1",
      "campaign_name": "Q3 Outbound",
      "step_id": "s1",
      "step_name": "Intro",
      "subject": "Quick question",
      "machine": true,
      "machine_reason": "burst",
      "link": {
        "id": "7c0f",
        "url": "https://yourco.com/pricing?utm_source=warmbly",
        "label": "Pricing",
        "utm_source": "warmbly",
        "utm_medium": "email",
        "utm_campaign": "q3_outbound",
        "utm_content": "pricing",
        "user_agent": "Mozilla/5.0"
      },
      "origin": {
        "client": "Gmail",
        "device_type": "desktop",
        "os": "macOS",
        "browser": "Chrome",
        "browser_version": "126",
        "country_code": "US",
        "region": "CA",
        "city": "San Francisco"
      }
    },
    {
      "type": "page_hit",
      "at": "2026-06-09T11:43:10Z",
      "subject": "Pricing",
      "page_hit": {
        "id": "ph1",
        "visitor_id": "v1",
        "session_key": "sess",
        "occurred_at": "2026-06-09T11:43:10Z",
        "url": "https://yourco.com/pricing",
        "path": "/pricing",
        "title": "Pricing",
        "referrer": "https://mail.google.com/",
        "referrer_domain": "mail.google.com",
        "landing": true,
        "utm_source": "warmbly",
        "utm_medium": "email",
        "utm_campaign": "q3_outbound",
        "utm_term": "",
        "utm_content": "pricing",
        "device_type": "desktop",
        "os": "macOS",
        "browser": "Chrome",
        "browser_version": "126",
        "device_brand": "Apple",
        "language": "en-US",
        "timezone": "America/Los_Angeles",
        "screen_width": 1440,
        "screen_height": 900,
        "country_code": "US",
        "region": "CA",
        "city": "San Francisco"
      }
    },
    {
      "type": "category_added",
      "at": "2026-05-02T09:00:00Z",
      "category_id": "cat1",
      "category_title": "VIP",
      "user_id": "u1"
    },
    {
      "type": "form_submitted",
      "at": "2026-05-01T10:00:00Z",
      "form_id": "f1",
      "form_name": "Demo request"
    },
    {
      "type": "contact_created",
      "at": "2026-05-01T09:30:00Z",
      "source": "import",
      "source_detail": "q3-leads.csv",
      "user_id": "u1"
    }
  ],
  "has_more": true,
  "pagination": { "total": null, "next_cursor": "opaque-1", "has_more": true }
}`

func TestListTimelineDecodesEventDetail(t *testing.T) {
	c := fixtureClient(t, map[string]string{"/v1/contacts/ct_1/timeline": timelineFixture})
	page, err := c.Contacts.ListTimeline(context.Background(), "ct_1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 5 {
		t.Fatalf("got %d events, want 5", len(page.Data))
	}
	if !page.HasMore() || page.NextCursor() != "opaque-1" {
		t.Fatalf("pagination not decoded: has_more=%v cursor=%q", page.HasMore(), page.NextCursor())
	}
	if page.Pagination.Total != nil {
		t.Errorf("timeline total should be nil, got %v", *page.Pagination.Total)
	}

	click := page.Data[0]
	if click.Type != TimelineEmailClicked {
		t.Fatalf("type = %q", click.Type)
	}
	if click.Machine == nil || !*click.Machine || click.MachineReason == nil || *click.MachineReason != MachineReasonBurst {
		t.Errorf("machine classification not decoded: %+v", click)
	}
	if click.Link == nil || click.Link.ID != "7c0f" || click.Link.Label != "Pricing" || click.Link.UTMCampaign != "q3_outbound" || click.Link.UserAgent != "Mozilla/5.0" {
		t.Errorf("link not decoded: %+v", click.Link)
	}
	if click.Origin == nil || click.Origin.Client != "Gmail" || click.Origin.City != "San Francisco" || click.Origin.BrowserVersion != "126" {
		t.Errorf("origin not decoded: %+v", click.Origin)
	}

	hit := page.Data[1]
	if hit.Type != TimelinePageHit || hit.PageHit == nil {
		t.Fatalf("page_hit not decoded: %+v", hit)
	}
	if hit.PageHit.Path != "/pricing" || !hit.PageHit.Landing || hit.PageHit.ScreenWidth != 1440 || hit.PageHit.ReferrerDomain != "mail.google.com" || hit.PageHit.Timezone != "America/Los_Angeles" {
		t.Errorf("page_hit fields: %+v", hit.PageHit)
	}
	if hit.Subject == nil || *hit.Subject != "Pricing" {
		t.Errorf("page_hit subject should be the page title")
	}

	cat := page.Data[2]
	if cat.Type != TimelineCategoryAdded || cat.CategoryID == nil || *cat.CategoryID != "cat1" || cat.CategoryTitle == nil || *cat.CategoryTitle != "VIP" || cat.UserID == nil || *cat.UserID != "u1" {
		t.Errorf("category event: %+v", cat)
	}
	form := page.Data[3]
	if form.Type != TimelineFormSubmitted || form.FormID == nil || *form.FormID != "f1" || form.FormName == nil || *form.FormName != "Demo request" {
		t.Errorf("form event: %+v", form)
	}
	created := page.Data[4]
	if created.Type != TimelineContactCreated || created.Source == nil || *created.Source != ContactSourceImport || created.SourceDetail == nil || *created.SourceDetail != "q3-leads.csv" {
		t.Errorf("contact_created event: %+v", created)
	}
	// Absent optional detail must stay nil, not zero-valued.
	if created.Link != nil || created.Origin != nil || created.PageHit != nil || created.Machine != nil {
		t.Errorf("absent detail should be nil: %+v", created)
	}
}

const contactDetailFixture = `{
  "id": "ct_1",
  "first_name": "Ada",
  "last_name": "Lovelace",
  "email": "ada@example.com",
  "company": "Analytical Engines",
  "phone": "",
  "custom_fields": {"plan": "pro"},
  "subscribed": true,
  "campaigns": [{"id": "c1", "name": "Q3 Outbound"}],
  "categories": [],
  "verification_status": "valid",
  "verification_reason": "recipient accepted",
  "verification_sub_status": "",
  "verification_source": "probe",
  "verification_provider": "builtin",
  "verification_confidence": 92,
  "verification_checked_at": "2026-06-10T11:58:00Z",
  "is_catch_all": false,
  "esp_provider": "gmail",
  "updated_at": "2026-06-10T12:00:00Z",
  "created_at": "2026-05-01T09:30:00Z",
  "source": "import",
  "source_detail": "q3-leads.csv",
  "first_seen_at": "2026-05-01T09:30:00Z",
  "engagement": {"total_sent": 4, "total_opened": 3, "total_clicked": 1, "total_replied": 1, "total_bounced": 0, "total_complained": 0},
  "suppression": {
    "id": "sup_1",
    "kind": "domain",
    "value": "example.com",
    "reason": "hard bounce",
    "source": "manual",
    "created_at": "2026-06-01T00:00:00Z"
  },
  "verification": {
    "status": "valid",
    "confidence": 92,
    "reasons": ["The mailbox accepted the probe.", "A reply was received."],
    "decisive": true,
    "evidence": [
      {"kind": "replied", "detail": "Re: Quick question", "observed_at": "2026-06-09T11:02:00Z"},
      {"kind": "delivered", "observed_at": "2026-06-09T08:00:00Z"}
    ]
  }
}`

func TestGetDecodesVerificationAndAttribution(t *testing.T) {
	c := fixtureClient(t, map[string]string{"/v1/contacts/ct_1": contactDetailFixture})
	d, _, err := c.Contacts.Get(context.Background(), "ct_1")
	if err != nil {
		t.Fatal(err)
	}
	if d.VerificationSource != VerificationSourceProbe || d.VerificationProvider != VerificationProviderBuiltin || d.VerificationConfidence != 92 {
		t.Errorf("contact verification fields: source=%q provider=%q confidence=%d", d.VerificationSource, d.VerificationProvider, d.VerificationConfidence)
	}
	if d.Source != ContactSourceImport || d.SourceDetail != "q3-leads.csv" || d.FirstSeenAt.IsZero() {
		t.Errorf("attribution: %q %q %v", d.Source, d.SourceDetail, d.FirstSeenAt)
	}
	if d.Verification == nil {
		t.Fatal("verification detail missing")
	}
	if d.Verification.Status != VerifyStatusValid || !d.Verification.Decisive || len(d.Verification.Reasons) != 2 {
		t.Errorf("verification detail: %+v", d.Verification)
	}
	if len(d.Verification.Evidence) != 2 || d.Verification.Evidence[0].Kind != VerificationEvidenceReplied || d.Verification.Evidence[0].Detail != "Re: Quick question" {
		t.Errorf("evidence: %+v", d.Verification.Evidence)
	}
	if d.Suppression == nil || d.Suppression.ID != "sup_1" || d.Suppression.Kind != "domain" || d.Suppression.Value != "example.com" || d.Suppression.Source != "manual" {
		t.Errorf("suppression: %+v", d.Suppression)
	}
}

func TestVerificationOverviewDecodes(t *testing.T) {
	c := fixtureClient(t, map[string]string{"/v1/contacts/verification": `{
	  "provider": "millionverifier",
	  "connection_id": "conn_1",
	  "credits": 48210,
	  "builtin_ready": true,
	  "counts": {"valid": 11240, "risky": 380, "invalid": 512, "unknown": 1890, "pending": 120}
	}`})
	o, _, err := c.Contacts.VerificationOverview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if o.Provider != VerificationProviderMillionVerifier || !o.BuiltinReady || o.ProviderError != "" {
		t.Errorf("overview: %+v", o)
	}
	if o.ConnectionID == nil || *o.ConnectionID != "conn_1" || o.Credits == nil || *o.Credits != 48210 {
		t.Errorf("provider details: conn=%v credits=%v", o.ConnectionID, o.Credits)
	}
	if o.Counts.Valid != 11240 || o.Counts.Pending != 120 {
		t.Errorf("counts: %+v", o.Counts)
	}

	// The builtin-only shape omits the provider fields entirely.
	c2 := fixtureClient(t, map[string]string{"/v1/contacts/verification": `{
	  "provider": "builtin", "builtin_ready": false,
	  "counts": {"valid": 1, "risky": 0, "invalid": 0, "unknown": 3, "pending": 3}
	}`})
	o2, _, err := c2.Contacts.VerificationOverview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if o2.Provider != VerificationProviderBuiltin || o2.ConnectionID != nil || o2.Credits != nil {
		t.Errorf("builtin overview: %+v", o2)
	}
}

func TestRequestVerificationDecodes(t *testing.T) {
	c := fixtureClient(t, map[string]string{"/v1/contacts/verification": `{"affected": 512, "action": "verify", "queued": true}`})
	r, _, err := c.Contacts.RequestVerification(context.Background(), &ContactVerificationParams{Action: VerificationActionVerify, CampaignID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Affected != 512 || r.Action != VerificationActionVerify || !r.Queued {
		t.Errorf("result: %+v", r)
	}
}

func TestCampaignStatesDecodes(t *testing.T) {
	c := fixtureClient(t, map[string]string{"/v1/contacts/ct_1/campaigns": `{"data": [{
	  "campaign_id": "c1",
	  "campaign_name": "Q3 Outbound",
	  "campaign_status": "active",
	  "lead_status": "active",
	  "steps": [
	    {"id": "s1", "label": "Email 1", "kind": "email", "position": 0, "subject": "Quick question", "sent_at": "2026-06-09T08:00:00Z", "opened_at": "2026-06-09T08:14:00Z"},
	    {"id": "s2", "label": "Email 2", "kind": "email", "position": 1, "subject": "Following up", "attempts": 2, "in_flight": true}
	  ],
	  "completed_steps": 1,
	  "total_steps": 2,
	  "current_step": {"id": "s1", "label": "Email 1", "kind": "email", "position": 0},
	  "last_action": "Opened",
	  "last_action_at": "2026-06-09T08:14:00Z",
	  "next": {
	    "step_id": "s2", "step_label": "Email 2", "kind": "email", "subject": "Following up",
	    "state": "waiting", "not_before": "2026-06-12T08:00:00Z", "constraint": "Waiting 3 days after Email 1"
	  }
	}, {
	  "campaign_id": "c2", "campaign_name": "Old", "campaign_status": "completed",
	  "lead_status": "failed", "failure_reason": "mailbox refused the message",
	  "steps": [], "completed_steps": 0, "total_steps": 1,
	  "ended_reason": "The mailbox could not send Email 1."
	}]}`})
	states, _, err := c.Contacts.CampaignStates(context.Background(), "ct_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 {
		t.Fatalf("got %d states", len(states))
	}
	s := states[0]
	if s.LeadStatus != LeadStatusActive || s.CompletedSteps != 1 || len(s.Steps) != 2 {
		t.Errorf("state: %+v", s)
	}
	if s.Steps[0].OpenedAt == nil || s.Steps[1].Attempts != 2 || !s.Steps[1].InFlight {
		t.Errorf("steps: %+v", s.Steps)
	}
	if s.CurrentStep == nil || s.CurrentStep.ID != "s1" {
		t.Errorf("current step: %+v", s.CurrentStep)
	}
	if s.Next == nil || s.Next.State != NextActionWaiting || s.Next.StepID == nil || *s.Next.StepID != "s2" || s.Next.NotBefore == nil || s.Next.ScheduledAt != nil {
		t.Errorf("next: %+v", s.Next)
	}
	ended := states[1]
	if ended.Next != nil || ended.LeadStatus != LeadStatusFailed || ended.FailureReason == "" || ended.EndedReason == "" {
		t.Errorf("ended state: %+v", ended)
	}
}

func TestSegmentsDecodes(t *testing.T) {
	c := fixtureClient(t, map[string]string{"/v1/contacts/ct_1/segments": `{"data": [
	  {"id": "seg_1", "name": "Warm fintech leads", "color": "#0284c7", "mode": "include", "member": true},
	  {"id": "seg_2", "name": "Cold", "color": "#999999", "member": false}
	]}`})
	segs, _, err := c.Contacts.Segments(context.Background(), "ct_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 2 || segs[0].Mode != "include" || !segs[0].Member || segs[1].Mode != "" || segs[1].Member {
		t.Errorf("segments: %+v", segs)
	}
}

func TestSearchDecodesVerificationAndLeadCounts(t *testing.T) {
	c := fixtureClient(t, map[string]string{"/v1/contacts/search": `{
	  "data": [{
	    "id": "ct_1", "email": "ada@example.com", "subscribed": true,
	    "campaigns": [], "categories": [], "custom_fields": {},
	    "verification_status": "risky", "verification_sub_status": "catch_all",
	    "verification_source": "provider", "verification_provider": "millionverifier",
	    "verification_confidence": 40, "is_catch_all": true, "esp_provider": "other",
	    "campaign_lead": {
	      "status": "failed", "sent": 2, "opened": 1, "machine_opened": 1, "clicked": 0,
	      "replied": 0, "bounced": 0, "current_step": "Email 2",
	      "failure_reason": "mailbox refused the message"
	    },
	    "updated_at": "2026-06-10T12:00:00Z", "created_at": "2026-05-01T09:30:00Z"
	  }],
	  "pagination": {"total": 1, "next_cursor": null, "has_more": false},
	  "counts": {
	    "total": 14022, "subscribed": 13900, "unsubscribed": 122, "in_campaign": 9000, "not_contacted": 2000,
	    "categories": [{"category_id": "cat1", "count": 3}],
	    "verification": {"valid": 11240, "risky": 380, "invalid": 512, "unknown": 1890, "pending": 120}
	  },
	  "lead_counts": {
	    "total": 3140, "queued": 1980, "processing": 910, "completed": 27, "replied": 180,
	    "bounced": 22, "failed": 3, "unsubscribed": 18, "undeliverable": 40,
	    "contacted": 1160, "opened": 600, "clicked": 90, "replied_any": 185
	  }
	}`})
	page, err := c.Contacts.Search(context.Background(), &ContactSearchParams{CampaignIDs: []string{"c1"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 1 {
		t.Fatalf("got %d contacts", len(page.Data))
	}
	ct := page.Data[0]
	if ct.VerificationSubStatus != VerificationSubStatusCatchAll || ct.VerificationSource != VerificationSourceProvider || ct.VerificationConfidence != 40 {
		t.Errorf("contact verification: %+v", ct)
	}
	if ct.CampaignLead == nil || ct.CampaignLead.Status != LeadStatusFailed || ct.CampaignLead.MachineOpened != 1 || ct.CampaignLead.FailureReason == "" {
		t.Errorf("campaign_lead: %+v", ct.CampaignLead)
	}
	if page.Counts == nil || page.Counts.Verification.Pending != 120 || page.Counts.Verification.Invalid != 512 {
		t.Errorf("counts.verification: %+v", page.Counts)
	}
	lc := page.LeadCounts
	if lc == nil || lc.Failed != 3 || lc.Undeliverable != 40 || lc.Contacted != 1160 || lc.RepliedAny != 185 {
		t.Errorf("lead_counts: %+v", lc)
	}
}

func TestImportCommitDecodesQuality(t *testing.T) {
	c := fixtureClient(t, map[string]string{"/v1/contacts/import/commit": `{
	  "total": 1200, "imported": 1100, "updated": 50, "skipped": 40, "failed": 10,
	  "started_at": "2026-06-11T10:10:00Z", "ended_at": "2026-06-11T10:10:07Z",
	  "errors": [{"line": 57, "email": "not-an-email", "reason": "invalid email"}],
	  "errors_truncated": true,
	  "quality": {"malformed": 4, "disposable": 0, "role": 62, "bad_share_pct": 0.3, "flagged": false}
	}`})
	file := &FileUpload{Filename: "leads.csv", Content: strings.NewReader("email\na@b.com\n")}
	res, _, err := c.Contacts.ImportCommit(context.Background(), file, &ContactImportParams{
		Mapping:    []ImportColumnMapping{{Index: 0, Target: ImportTargetEmail}},
		Dedup:      ImportDedupUpdate,
		HasHeader:  true,
		SegmentIDs: []string{"seg_1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.ErrorsTruncated || res.Quality == nil || res.Quality.Role != 62 || res.Quality.BadSharePct != 0.3 {
		t.Errorf("import result: %+v quality=%+v", res, res.Quality)
	}
}
