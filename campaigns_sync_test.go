package warmbly

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// campaignSyncClient answers every request with a fixed status and body, for
// endpoints whose response the shared routing fixture cannot stand in for.
func campaignSyncClient(t *testing.T, status int, body string, got *recordedRequest) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got != nil {
			got.method = r.Method
			got.path = r.URL.Path
			got.rawQuery = r.URL.RawQuery
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// TestCampaignSyncRouting asserts method, path and, where the body carries
// meaning, the wire shape of every campaign and outreach method added or
// changed by the sync.
func TestCampaignSyncRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()
	start := time.Date(2026, 9, 7, 9, 0, 0, 0, time.UTC)

	cases := []struct {
		name       string
		call       func() error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string // substring; "" skips the check
		wantNoBody bool
	}{
		{"campaigns.List filters", func() error {
			_, e := c.Campaigns.List(ctx, &CampaignListParams{Status: CampaignStatusPaused, Kind: CampaignKindOneTime})
			return e
		}, "GET", "/v1/campaigns", "kind=one_time&status=paused", "", false},
		{"campaigns.Estimate", func() error {
			_, _, e := c.Campaigns.Estimate(ctx, &CampaignEstimateParams{
				SegmentIDs: []string{"seg_1"}, DailyLimit: Int(40), StartDate: &start,
			})
			return e
		}, "POST", "/v1/campaigns-estimate", "", `"segment_ids":["seg_1"],"daily_limit":40,"start_date":"2026-09-07T09:00:00Z"`, false},
		{"campaigns.Duplicate named", func() error {
			_, _, e := c.Campaigns.Duplicate(ctx, "camp_1", &CampaignDuplicateParams{Name: "Copy"})
			return e
		}, "POST", "/v1/campaigns/camp_1/duplicate", "", `{"name":"Copy"}`, false},
		{"campaigns.Duplicate bodyless", func() error {
			_, _, e := c.Campaigns.Duplicate(ctx, "camp_1", nil)
			return e
		}, "POST", "/v1/campaigns/camp_1/duplicate", "", "", true},
		{"campaigns.Start bodyless", func() error {
			_, _, e := c.Campaigns.Start(ctx, "camp_1")
			return e
		}, "POST", "/v1/campaigns/camp_1/start", "", "", true},
		{"campaigns.StartWithOptions", func() error {
			_, _, e := c.Campaigns.StartWithOptions(ctx, "camp_1", &CampaignStartParams{AcknowledgeListRisk: true})
			return e
		}, "POST", "/v1/campaigns/camp_1/start", "", `{"acknowledge_list_risk":true}`, false},
		{"campaigns.Stop", func() error {
			_, _, e := c.Campaigns.Stop(ctx, "camp_1")
			return e
		}, "POST", "/v1/campaigns/camp_1/stop", "", "", true},
		{"campaigns.Forms", func() error {
			_, _, e := c.Campaigns.Forms(ctx, "camp_1")
			return e
		}, "GET", "/v1/campaigns/camp_1/forms", "", "", false},
		{"campaigns.ListSegments", func() error {
			_, _, e := c.Campaigns.ListSegments(ctx, "camp_1")
			return e
		}, "GET", "/v1/campaigns/camp_1/segments", "", "", false},
		{"campaigns.SetSegments", func() error {
			_, _, e := c.Campaigns.SetSegments(ctx, "camp_1", []string{"seg_1", "seg_2"})
			return e
		}, "PUT", "/v1/campaigns/camp_1/segments", "", `{"segment_ids":["seg_1","seg_2"]}`, false},
		// The API refuses an omitted segment_ids, so nil must reach the wire
		// as an explicit empty array.
		{"campaigns.SetSegments detach all", func() error {
			_, _, e := c.Campaigns.SetSegments(ctx, "camp_1", nil)
			return e
		}, "PUT", "/v1/campaigns/camp_1/segments", "", `{"segment_ids":[]}`, false},
		{"campaigns.UpdateAdvancedSettings", func() error {
			_, e := c.Campaigns.UpdateAdvancedSettings(ctx, "camp_1", &OutreachSettings{})
			return e
		}, "PATCH", "/v1/campaigns/camp_1/advanced", "", `{"settings":{`, false},
		{"campaigns.SendTestEmail", func() error {
			_, _, e := c.Campaigns.SendTestEmail(ctx, "camp_1", &TestEmailParams{AccountID: "em_1", Recipient: "me@acme.com", ContactID: "ct_1"})
			return e
		}, "POST", "/v1/campaigns/camp_1/test-email", "", `"contact_id":"ct_1"`, false},
		{"campaigns.PreviewTemplate", func() error {
			_, _, e := c.Campaigns.PreviewTemplate(ctx, &TemplatePreviewParams{Subject: "Hi", CampaignID: "camp_1", AccountID: "em_1", StepID: "st_1", ContactID: "ct_1"})
			return e
		}, "POST", "/v1/campaign-template-preview", "", `"contact_id":"ct_1","campaign_id":"camp_1","account_id":"em_1","step_id":"st_1"`, false},
		{"campaigns.Update guardrails", func() error {
			_, _, e := c.Campaigns.Update(ctx, "camp_1", &CampaignUpdateParams{
				Continuous: Bool(true), GuardrailEnabled: Bool(true), GuardrailBounceRateMax: Float64(5),
				UTMTracking: Bool(true), UnsubscribeMode: String(UnsubscribeModeLink),
			})
			return e
		}, "PATCH", "/v1/campaigns/camp_1", "", `"unsubscribe_mode":"link","continuous":true,"utm_tracking":true,"guardrail_enabled":true,"guardrail_bounce_rate_max":5`, false},
		{"campaigns.Create kind", func() error {
			_, _, e := c.Campaigns.Create(ctx, &CampaignCreateParams{Name: "Launch", Kind: String(CampaignKindOneTime), Continuous: Bool(false)})
			return e
		}, "POST", "/v1/campaigns", "", `{"name":"Launch","kind":"one_time","continuous":false}`, false},
		{"outreach.Update", func() error {
			_, e := c.Outreach.Update(ctx, &OutreachSettings{Unsubscribe: UnsubscribeSettings{Mode: UnsubscribeModeText}})
			return e
		}, "PATCH", "/v1/outreach/settings", "", `"unsubscribe":{"mode":"text","text":"","link_intro":"","link_text":""}`, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got = recordedRequest{}
			if err := tc.call(); err != nil {
				t.Fatalf("call: %v", err)
			}
			if got.method != tc.wantMethod || got.path != tc.wantPath {
				t.Errorf("got %s %s, want %s %s", got.method, got.path, tc.wantMethod, tc.wantPath)
			}
			if tc.wantQuery != "" && got.rawQuery != tc.wantQuery {
				t.Errorf("query = %q, want %q", got.rawQuery, tc.wantQuery)
			}
			if tc.wantNoBody && got.body != "" {
				t.Errorf("body = %q, want it empty", got.body)
			}
			if tc.wantBody != "" && !strings.Contains(got.body, tc.wantBody) {
				t.Errorf("body = %s, want it to contain %s", got.body, tc.wantBody)
			}
		})
	}
}

// TestCampaignUpdateParamsClearDates covers the PATCH date semantics: absent
// leaves the stored date alone, an explicit null clears it.
func TestCampaignUpdateParamsClearDates(t *testing.T) {
	at := time.Date(2026, 9, 7, 9, 0, 0, 0, time.UTC)

	cases := []struct {
		name   string
		params CampaignUpdateParams
		want   string
	}{
		{"untouched", CampaignUpdateParams{Name: String("x")}, `{"name":"x"}`},
		{"set", CampaignUpdateParams{StartDate: &at}, `{"start_date":"2026-09-07T09:00:00Z"}`},
		{"clear", CampaignUpdateParams{ClearStartDate: true, ClearEndDate: true}, `{"start_date":null,"end_date":null}`},
		{"clear wins over set", CampaignUpdateParams{EndDate: &at, ClearEndDate: true}, `{"end_date":null}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(&tc.params)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(b) != tc.want {
				t.Errorf("got %s, want %s", b, tc.want)
			}
		})
	}
}

func TestCampaignDecodeSyncFields(t *testing.T) {
	const fixture = `{
		"id": "c1", "user_id": "u1", "organization_id": "o1",
		"name": "Q3 outbound", "description": "", "status": "active", "kind": "sequence",
		"stop_on_reply": true, "open_tracking": true, "link_tracking": true, "text_only": false,
		"daily_limit": 50, "unsubscribe_header": true, "risky_emails": false,
		"unsubscribe_mode": "inherit",
		"cc": [], "bcc": [],
		"start_date": null, "end_date": null, "timezone": "Europe/Berlin", "days": 31,
		"start_time": "09:00", "end_time": "17:00",
		"schedule_windows": [[], [{"start": 540, "end": 1020}], [], [], [], [], []],
		"email_tags": ["t1"], "folders": [],
		"contact_order_by": "created_at", "contact_order_dir": "asc",
		"sender_strategy": "tags", "rotation_mode": "round_robin",
		"ramp_enabled": false, "ramp_start": 0, "ramp_increment": 0, "ramp_ceiling": 0, "ramp_level": 0,
		"esp_match_mode": "off", "max_new_leads_per_day": 0, "prioritize_new_leads": false,
		"continuous": true, "idle_since": "2026-09-01T10:00:00Z",
		"guardrail_enabled": true, "guardrail_bounce_rate_max": 5, "guardrail_complaint_rate_max": 0.1,
		"guardrail_reply_rate_min": 0, "guardrail_min_sample": 50, "guardrail_window_days": 7,
		"guardrail_tripped_at": "2026-09-02T10:00:00Z", "guardrail_reason": "bounce_rate",
		"tracking_domain": "", "tracking_domain_verified": false,
		"utm_tracking": true, "utm_source": "", "utm_medium": "email", "utm_campaign": "q3",
		"updated_at": "2026-09-02T10:00:00Z", "created_at": "2026-08-01T10:00:00Z"
	}`
	var c Campaign
	if err := json.Unmarshal([]byte(fixture), &c); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if c.Kind != CampaignKindSequence || c.UnsubscribeMode != UnsubscribeModeInherit {
		t.Errorf("kind/unsubscribe_mode = %q/%q", c.Kind, c.UnsubscribeMode)
	}
	if !c.Continuous || c.IdleSince == nil || c.IdleSince.Day() != 1 {
		t.Errorf("continuous/idle_since = %v/%v", c.Continuous, c.IdleSince)
	}
	if c.StartDate != nil || c.EndDate != nil {
		t.Errorf("null dates decoded as %v/%v", c.StartDate, c.EndDate)
	}
	if !c.GuardrailEnabled || c.GuardrailBounceRateMax != 5 || c.GuardrailComplaintRateMax != 0.1 ||
		c.GuardrailMinSample != 50 || c.GuardrailWindowDays != 7 || c.GuardrailReason != "bounce_rate" || c.GuardrailTrippedAt == nil {
		t.Errorf("guardrails = %+v", c)
	}
	if !c.UTMTracking || c.UTMMedium != "email" || c.UTMCampaign != "q3" {
		t.Errorf("utm = %v/%q/%q", c.UTMTracking, c.UTMMedium, c.UTMCampaign)
	}
	if c.ScheduleWindows.IsEmpty() || c.ScheduleWindows[1][0].End != 1020 {
		t.Errorf("schedule_windows = %v", c.ScheduleWindows)
	}
}

func TestCampaignEstimateResultDecode(t *testing.T) {
	var got recordedRequest
	c := campaignSyncClient(t, http.StatusOK, `{
		"recipients": 1000, "mailboxes": 4, "daily_capacity": 200, "remaining_today": 140,
		"sending_days": 5, "estimated_finish_at": "2026-09-09T00:00:00+02:00"
	}`, &got)
	res, _, err := c.Campaigns.Estimate(context.Background(), &CampaignEstimateParams{SegmentIDs: []string{"seg_1"}})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}
	if res.Recipients != 1000 || res.Mailboxes != 4 || res.DailyCapacity != 200 || res.RemainingToday != 140 {
		t.Errorf("counts = %+v", res)
	}
	if res.SendingDays == nil || *res.SendingDays != 5 || res.EstimatedFinishAt == nil || res.EstimatedFinishAt.Day() != 9 {
		t.Errorf("projection = %v/%v", res.SendingDays, res.EstimatedFinishAt)
	}

	// No capacity: both projections are null.
	var empty CampaignEstimateResult
	if err := json.Unmarshal([]byte(`{"recipients":0,"mailboxes":0,"daily_capacity":0,"remaining_today":0,"sending_days":null,"estimated_finish_at":null}`), &empty); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if empty.SendingDays != nil || empty.EstimatedFinishAt != nil {
		t.Errorf("null projection decoded as %v/%v", empty.SendingDays, empty.EstimatedFinishAt)
	}
}

func TestCampaignStatusChangeDecode(t *testing.T) {
	c := campaignSyncClient(t, http.StatusOK, `{"status":"started"}`, nil)
	res, _, err := c.Campaigns.Start(context.Background(), "camp_1")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if res.Status != "started" {
		t.Errorf("status = %q, want started", res.Status)
	}
}

// TestCampaignStartRefusalCode checks the bounce-risk refusal surfaces its
// stable code, since that is what a caller branches on before retrying with
// AcknowledgeListRisk.
func TestCampaignStartRefusalCode(t *testing.T) {
	c := campaignSyncClient(t, http.StatusBadRequest, `{"error":"bad_request","message":"projected bounce rate 6%","code":"list_bounce_risk"}`, nil)
	_, _, err := c.Campaigns.Start(context.Background(), "camp_1")
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %T (%v), want *Error", err, err)
	}
	if apiErr.Code != ErrCodeListBounceRisk {
		t.Errorf("code = %q, want %q", apiErr.Code, ErrCodeListBounceRisk)
	}
}

func TestCampaignSegmentsDecode(t *testing.T) {
	var got recordedRequest
	c := campaignSyncClient(t, http.StatusOK, `{
		"data": [{
			"segment_id": "seg_1", "name": "Warm leads", "color": "#0ea5e9",
			"description": "Replied or clicked in the last 30 days",
			"contact_count": 412, "lead_count": 409, "held_out_count": 3,
			"linked_at": "2026-06-10T12:00:00Z"
		}],
		"added": 7
	}`, &got)
	res, _, err := c.Campaigns.SetSegments(context.Background(), "camp_1", []string{"seg_1"})
	if err != nil {
		t.Fatalf("SetSegments: %v", err)
	}
	if res.Added != 7 || len(res.Segments) != 1 {
		t.Fatalf("result = %+v", res)
	}
	link := res.Segments[0]
	if link.SegmentID != "seg_1" || link.ContactCount != 412 || link.LeadCount != 409 || link.HeldOutCount != 3 || link.LinkedAt.Month() != time.June {
		t.Errorf("link = %+v", link)
	}

	links, _, err := c.Campaigns.ListSegments(context.Background(), "camp_1")
	if err != nil {
		t.Fatalf("ListSegments: %v", err)
	}
	if len(links) != 1 || links[0].Name != "Warm leads" {
		t.Errorf("links = %+v", links)
	}
}

func TestCampaignFormsDecode(t *testing.T) {
	c := campaignSyncClient(t, http.StatusOK, `{"data":[{
		"form_id": "f1", "form_name": "Book a demo", "public_id": "book-a-demo", "status": "published",
		"links_sent": 120, "viewers": 40, "starters": 25, "submissions": 12,
		"share_url": "https://forms.example.com/book-a-demo"
	}]}`, nil)
	forms, _, err := c.Campaigns.Forms(context.Background(), "camp_1")
	if err != nil {
		t.Fatalf("Forms: %v", err)
	}
	if len(forms) != 1 {
		t.Fatalf("forms = %+v", forms)
	}
	f := forms[0]
	if f.FormID != "f1" || f.PublicID != "book-a-demo" || f.LinksSent != 120 || f.Viewers != 40 || f.Starters != 25 || f.Submissions != 12 || f.ShareURL == "" {
		t.Errorf("form = %+v", f)
	}
}

func TestTemplatePreviewResultDecode(t *testing.T) {
	const fixture = `{
		"subject": "Hi Alex", "body_html": "<p>Hi Alex</p>", "body_plain": "Hi Alex",
		"unresolved": ["{{.Industry}}"],
		"from": {"name": "Sam Seller", "email": "sam@acme.com"},
		"attachments": [{"id": "att_1", "filename": "deck.pdf", "size": 20480, "mime_type": "application/pdf"}]
	}`
	var res TemplatePreviewResult
	if err := json.Unmarshal([]byte(fixture), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.From == nil || res.From.Email != "sam@acme.com" {
		t.Errorf("from = %+v", res.From)
	}
	if len(res.Attachments) != 1 || res.Attachments[0].Filename != "deck.pdf" || res.Attachments[0].Size != 20480 {
		t.Errorf("attachments = %+v", res.Attachments)
	}
	if len(res.Unresolved) != 1 {
		t.Errorf("unresolved = %v", res.Unresolved)
	}
}

func TestTestEmailResultDecode(t *testing.T) {
	var res TestEmailResult
	if err := json.Unmarshal([]byte(`{"message":"test email sent","recipient":"me@example.com","subject":"Quick question","account_id":"em_1","step_id":"st_1","contact_id":"ct_1"}`), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.StepID != "st_1" || res.ContactID != "ct_1" {
		t.Errorf("result = %+v", res)
	}
}

func TestCampaignsOverviewOneTimeDecode(t *testing.T) {
	var o CampaignsOverview
	if err := json.Unmarshal([]byte(`{"total":12,"active":3,"paused":2,"draft":4,"completed":3,"one_time":2,"folders":[{"folder_id":"f1","total":5}]}`), &o); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if o.OneTime != 2 || len(o.Folders) != 1 {
		t.Errorf("overview = %+v", o)
	}
}

func TestCampaignAttachmentStepIDDecode(t *testing.T) {
	var atts []CampaignAttachment
	if err := json.Unmarshal([]byte(`[
		{"id":"a1","campaign_id":"c1","step_id":null,"filename":"deck.pdf","size":1,"mime_type":"application/pdf","url":"https://s3/x","created_at":"2026-09-01T00:00:00Z"},
		{"id":"a2","campaign_id":"c1","step_id":"st_1","filename":"one.png","size":2,"mime_type":"image/png","url":"","created_at":"2026-09-01T00:00:00Z"}
	]`), &atts); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if atts[0].StepID != nil {
		t.Errorf("campaign-wide step_id = %v, want nil", *atts[0].StepID)
	}
	if atts[1].StepID == nil || *atts[1].StepID != "st_1" {
		t.Errorf("step-scoped step_id = %v", atts[1].StepID)
	}
}

func TestOutreachSettingsDecodeSyncFields(t *testing.T) {
	const fixture = `{
		"bounce_pipeline": {"enabled": true},
		"task_reliability": {"enabled": true},
		"ab_testing": {"enabled": true},
		"reply_intent": {"enabled": true},
		"send_time_optimization": {"enabled": false, "use_contact_timezone": true, "default_contact_timezone": "UTC", "preferred_hours": [9, 10], "weekend_weight_multiplier": 0.5},
		"preflight": {"enabled": true, "check_content_score": true, "min_content_score": 60},
		"dashboard": {"enabled": true},
		"unsubscribe": {"mode": "link", "text": "", "link_intro": "Not the right person?", "link_text": "Unsubscribe"},
		"custom": {}
	}`
	var s OutreachSettings
	if err := json.Unmarshal([]byte(fixture), &s); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !s.Preflight.CheckContentScore || s.Preflight.MinContentScore != 60 {
		t.Errorf("preflight = %+v", s.Preflight)
	}
	if s.Unsubscribe.Mode != UnsubscribeModeLink || s.Unsubscribe.LinkText != "Unsubscribe" || s.Unsubscribe.LinkIntro == "" {
		t.Errorf("unsubscribe = %+v", s.Unsubscribe)
	}
	if s.SendTimeOptimization.Enabled {
		t.Errorf("send-time optimization decoded as enabled")
	}
}

// TestOutreachUpdateNoContent checks the 204 the settings endpoints answer with
// is a success and not a decode error.
func TestOutreachUpdateNoContent(t *testing.T) {
	var got recordedRequest
	c := campaignSyncClient(t, http.StatusNoContent, ``, &got)
	resp, err := c.Outreach.Update(context.Background(), &OutreachSettings{})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if resp == nil || resp.StatusCode != http.StatusNoContent {
		t.Errorf("resp = %+v", resp)
	}
	if _, err := c.Campaigns.UpdateAdvancedSettings(context.Background(), "camp_1", &OutreachSettings{}); err != nil {
		t.Fatalf("UpdateAdvancedSettings: %v", err)
	}
	if got.method != "PATCH" || got.path != "/v1/campaigns/camp_1/advanced" {
		t.Errorf("got %s %s", got.method, got.path)
	}
}
