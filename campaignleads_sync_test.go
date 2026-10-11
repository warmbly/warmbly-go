package warmbly

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCampaignLeadHoldRoutes(t *testing.T) {
	var got capturedRequest
	ctx := context.Background()
	held := `{"campaign_id": "camp_1", "contact_id": "ct_1",
  "hold": {"since": "2026-10-01T10:00:00Z", "until": "2026-10-15T00:00:00Z", "reason": "conference", "source": "manual"}}`

	c := syncServer(t, http.StatusOK, "application/json", held, &got)
	h, _, err := c.Campaigns.LeadHold(ctx, "camp_1", "ct_1")
	if err != nil {
		t.Fatalf("LeadHold: %v", err)
	}
	assertRequest(t, got, "GET", "/v1/campaigns/camp_1/leads/ct_1/hold", "")
	if h.CampaignID != "camp_1" || h.ContactID != "ct_1" || h.Hold == nil || h.Hold.Reason != "conference" || h.Hold.Source != LeadHoldSourceManual {
		t.Errorf("hold = %+v", h)
	}

	until := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	if _, _, err := c.Campaigns.PauseLead(ctx, "camp_1", "ct_1", &LeadPauseParams{Until: &until, Reason: "conference"}); err != nil {
		t.Fatalf("PauseLead: %v", err)
	}
	assertRequest(t, got, "POST", "/v1/campaigns/camp_1/leads/ct_1/pause", "",
		`"until":"2026-10-15T00:00:00Z"`, `"reason":"conference"`)

	// No params is a hold with no end: the handler reads an empty body as that.
	if _, _, err := c.Campaigns.PauseLead(ctx, "camp_1", "ct_1", nil); err != nil {
		t.Fatalf("PauseLead(nil): %v", err)
	}
	assertRequest(t, got, "POST", "/v1/campaigns/camp_1/leads/ct_1/pause", "")
	if got.body != "" {
		t.Errorf("open-ended pause body = %q, want none", got.body)
	}
	if b, _ := json.Marshal(LeadPauseParams{}); string(b) != "{}" {
		t.Errorf("zero pause params = %s, want {}", b)
	}

	c = syncServer(t, http.StatusOK, "application/json", `{"campaign_id": "camp_1", "contact_id": "ct_1"}`, &got)
	r, _, err := c.Campaigns.ResumeLead(ctx, "camp_1", "ct_1")
	if err != nil {
		t.Fatalf("ResumeLead: %v", err)
	}
	assertRequest(t, got, "POST", "/v1/campaigns/camp_1/leads/ct_1/resume", "")
	if r.Hold != nil {
		t.Errorf("resumed hold = %+v, want nil", r.Hold)
	}
}

func TestCampaignLeadHoldNotALead(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusNotFound, "application/json",
		`{"error": "Not Found", "code": "not_found", "message": "contact is not a lead of this campaign"}`, &got)
	_, _, err := c.Campaigns.ResumeLead(context.Background(), "camp_1", "ct_x")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestCampaignLeadCCRoutes(t *testing.T) {
	var got capturedRequest
	ctx := context.Background()
	list := `{"campaign_id": "camp_1", "contact_id": "ct_1", "cc": [
  {"contact_id": "ct_2", "email": "bob@acme.test", "first_name": "Bob", "last_name": "Lee", "company": "Acme", "status": "active"},
  {"contact_id": "ct_3", "email": "eve@acme.test", "first_name": "Eve", "last_name": "", "status": "bounced", "bounced_at": "2026-09-30T08:00:00Z"}
]}`
	c := syncServer(t, http.StatusOK, "application/json", list, &got)

	cc, _, err := c.Campaigns.LeadCC(ctx, "camp_1", "ct_1")
	if err != nil {
		t.Fatalf("LeadCC: %v", err)
	}
	assertRequest(t, got, "GET", "/v1/campaigns/camp_1/leads/ct_1/cc", "")
	if len(cc.CC) != 2 || cc.CC[0].Company != "Acme" || cc.CC[1].Status != LeadCCStatusBounced || cc.CC[1].BouncedAt == nil {
		t.Errorf("cc = %+v", cc)
	}

	if _, _, err := c.Campaigns.SetLeadCC(ctx, "camp_1", "ct_1", []string{"ct_2", "ct_3"}); err != nil {
		t.Fatalf("SetLeadCC: %v", err)
	}
	assertRequest(t, got, "PUT", "/v1/campaigns/camp_1/leads/ct_1/cc", "", `{"contact_ids":["ct_2","ct_3"]}`)

	// Clearing sends an explicit empty list, never null.
	if _, _, err := c.Campaigns.SetLeadCC(ctx, "camp_1", "ct_1", nil); err != nil {
		t.Fatalf("SetLeadCC(nil): %v", err)
	}
	assertRequest(t, got, "PUT", "/v1/campaigns/camp_1/leads/ct_1/cc", "", `{"contact_ids":[]}`)

	c = syncServer(t, http.StatusOK, "application/json", `{"data": [
  {"contact_id": "ct_4", "email": "kim@acme.test", "first_name": "Kim", "last_name": "Park", "company": "Acme", "reason": "company"},
  {"contact_id": "ct_5", "email": "lee@acme.test", "first_name": "", "last_name": "", "reason": "domain"}
]}`, &got)
	sugg, _, err := c.Campaigns.LeadCCSuggestions(ctx, "camp_1", "ct_1")
	if err != nil {
		t.Fatalf("LeadCCSuggestions: %v", err)
	}
	assertRequest(t, got, "GET", "/v1/campaigns/camp_1/leads/ct_1/cc/suggestions", "")
	if len(sugg) != 2 || sugg[0].Reason != LeadCCReasonCompany || sugg[1].Reason != LeadCCReasonDomain {
		t.Errorf("suggestions = %+v", sugg)
	}

	c = syncServer(t, http.StatusOK, "application/json", `{"data": null}`, &got)
	sugg, _, err = c.Campaigns.LeadCCSuggestions(ctx, "camp_1", "ct_1")
	if err != nil || sugg == nil || len(sugg) != 0 {
		t.Errorf("empty suggestions = %v, %v", sugg, err)
	}
}

func TestCampaignLeadCCRefusalCode(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusBadRequest, "application/json",
		`{"error": "Bad Request", "code": "lead_cc_limit", "message": "A lead can have at most 2 contacts copied on their emails"}`, &got)
	_, _, err := c.Campaigns.SetLeadCC(context.Background(), "camp_1", "ct_1", []string{"a", "b", "c"})
	var apiErr *Error
	if !errors.As(err, &apiErr) || !apiErr.HasCode(ErrCodeLeadCCLimit) {
		t.Fatalf("err = %v, want %s", err, ErrCodeLeadCCLimit)
	}
}

func TestCampaignDriftFields(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusOK, "application/json", `{
  "id": "camp_1", "user_id": "usr_1", "name": "Q4", "description": "", "status": "draft",
  "timezone": "", "effective_timezone": "Europe/Berlin", "days": 31, "start_time": "08:00", "end_time": "18:00",
  "schedule_windows": [[],[],[],[],[],[],[]], "entry_delay_minutes": 90,
  "updated_at": "2026-10-01T09:00:00Z", "created_at": "2026-10-01T09:00:00Z"
}`, &got)
	camp, _, err := c.Campaigns.Get(context.Background(), "camp_1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if camp.Timezone != "" || camp.EffectiveTimezone != "Europe/Berlin" || camp.EntryDelayMinutes != 90 || camp.Kind != "" { //nolint:staticcheck // the deprecated field stays empty
		t.Errorf("campaign = %+v", camp)
	}

	if _, _, err := c.Campaigns.Update(context.Background(), "camp_1", &CampaignUpdateParams{EntryDelayMinutes: Int(0)}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	assertRequest(t, got, "PATCH", "/v1/campaigns/camp_1", "", `"entry_delay_minutes":0`)

	if _, _, err := c.Campaigns.Create(context.Background(), &CampaignCreateParams{
		Name:              "Q4",
		EntryDelayMinutes: Int(30),
		Steps:             []StepInput{{Subject: "Hi"}, {ThreadReply: Bool(false), Subject: "New topic"}},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	assertRequest(t, got, "POST", "/v1/campaigns", "", `"entry_delay_minutes":30`, `"thread_reply":false`)
	if strings.Count(got.body, "thread_reply") != 1 {
		t.Errorf("unset thread_reply was sent: %s", got.body)
	}
}

func TestCampaignStepThreadReply(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusOK, "application/json", `{
  "id": "st_1", "name": "Email 2", "subject": "", "body_plain": "", "body_html": "", "body_sync": true, "body_code": false,
  "wait_after": 4320, "thread_reply": true, "position": 1, "x": 0, "y": 0, "kind": "email",
  "updated_at": "2026-10-01T09:00:00Z", "created_at": "2026-10-01T09:00:00Z"
}`, &got)
	ctx := context.Background()

	step, _, err := c.Campaigns.CreateStepWithParams(ctx, "camp_1", &StepUpdateParams{ThreadReply: Bool(true), WaitAfter: Int(4320)})
	if err != nil {
		t.Fatalf("CreateStepWithParams: %v", err)
	}
	assertRequest(t, got, "POST", "/v1/campaigns/camp_1/steps", "", `"thread_reply":true`, `"wait_after":4320`)
	if !step.ThreadReply || step.WaitAfter != 4320 {
		t.Errorf("step = %+v", step)
	}

	if _, _, err := c.Campaigns.CreateStep(ctx, "camp_1"); err != nil {
		t.Fatalf("CreateStep: %v", err)
	}
	if got.body != "" {
		t.Errorf("blank create body = %q, want none", got.body)
	}

	if _, _, err := c.Campaigns.UpdateStep(ctx, "camp_1", "st_1", &StepUpdateParams{ThreadReply: Bool(false)}); err != nil {
		t.Fatalf("UpdateStep: %v", err)
	}
	assertRequest(t, got, "PATCH", "/v1/campaigns/camp_1/steps/st_1", "", `{"thread_reply":false}`)
}

func TestCampaignEstimateDrift(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusOK, "application/json", `{
  "recipients": 300, "mailboxes": 2, "daily_capacity": 60, "remaining_today": 40,
  "sending_days": 12, "estimated_finish_at": "2026-10-20T00:00:00Z",
  "steps": 3, "total_sends": 900, "first_touch_finish_at": "2026-10-09T00:00:00Z",
  "steady_capacity": 100, "full_capacity_at": null, "ramping": 1, "held": 0,
  "warmup": {"mailboxes": 2, "per_day": 20}, "other_campaigns_per_day": 15,
  "bottleneck": "warmup_graduation",
  "timeline": [{"date": "2026-10-12", "sending_day": true, "capacity": 60, "sends": 40, "first_emails": 40, "follow_ups": 0, "warmup": 20}],
  "senders": [{"id": "acc_1", "email": "a@acme.test", "provider": "google", "state": "ramping", "first_day_cap": 20, "steady_cap": 50, "warmup_per_day": 10, "full_cap_at": "2026-10-30T00:00:00Z"}]
}`, &got)
	res, _, err := c.Campaigns.Estimate(context.Background(), &CampaignEstimateParams{
		SegmentIDs: []string{"seg_1"},
		CampaignID: String("camp_1"),
		StartTime:  String("09:00"),
		EndTime:    String("17:00"),
		StepWaits:  []int{3, 4},
	})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}
	assertRequest(t, got, "POST", "/v1/campaigns-estimate", "",
		`"campaign_id":"camp_1"`, `"start_time":"09:00"`, `"end_time":"17:00"`, `"step_waits":[3,4]`)
	if res.TotalSends != 900 || res.Steps != 3 || res.FirstTouchFinishAt == nil || res.FullCapacityAt != nil {
		t.Errorf("result = %+v", res)
	}
	if res.Bottleneck != EstimateBottleneckGraduation || res.Warmup.PerDay != 20 || res.OtherCampaignsPerDay != 15 {
		t.Errorf("limits = %+v", res)
	}
	if len(res.Timeline) != 1 || res.Timeline[0].Date != "2026-10-12" || !res.Timeline[0].SendingDay {
		t.Errorf("timeline = %+v", res.Timeline)
	}
	if len(res.Senders) != 1 || res.Senders[0].State != EstimateSenderRamping || res.Senders[0].FullCapAt == nil {
		t.Errorf("senders = %+v", res.Senders)
	}
}

func TestCampaignSegmentsWithdrawn(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusOK, "application/json", `{"data": [], "added": 0, "withdrawn": 12, "contacted": 3}`, &got)
	res, _, err := c.Campaigns.SetSegments(context.Background(), "camp_1", nil)
	if err != nil {
		t.Fatalf("SetSegments: %v", err)
	}
	assertRequest(t, got, "PUT", "/v1/campaigns/camp_1/segments", "", `"segment_ids":[]`)
	if res.Withdrawn != 12 || res.Contacted != 3 {
		t.Errorf("result = %+v", res)
	}
}

func TestOutreachInboxTaggingRoundTrip(t *testing.T) {
	var got capturedRequest
	settings := `{
  "bounce_pipeline": {}, "task_reliability": {}, "ab_testing": {},
  "reply_intent": {"enabled": true, "auto_create_crm_task": true, "crm_task_intents": ["positive", "question"],
    "hold_on_out_of_office": true, "out_of_office_hold_days": 7},
  "inbox_tagging": {"hold_on_not_now": true, "not_now_hold_days": 30, "stop_on_declined": true,
    "task_on_call_request": true, "suppress_on_removal_request": false, "action_required_in_inbox": true,
    "languages": ["en", "de"],
    "questions": [
      {"id": "budget", "type": "yes_no", "question": "Do they mention a budget?", "label": "Has budget", "action": {"type": "task"}},
      {"id": "timing", "type": "choice", "question": "When?", "action": {"type": ""}, "automated": true,
       "choices": [{"label": "This quarter", "description": "Wants to start now", "action": {"type": ""}},
                   {"label": "Next year", "description": "Later", "action": {"type": "hold", "hold_days": 90}}]}
    ]},
  "send_time_optimization": {}, "preflight": {}, "dashboard": {}, "unsubscribe": {"mode": "text"}
}`
	c := syncServer(t, http.StatusOK, "application/json", settings, &got)
	s, _, err := c.Outreach.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	ri, it := s.ReplyIntent, s.InboxTagging
	if !ri.HoldOnOutOfOffice || ri.OutOfOfficeHoldDays != 7 || len(ri.CRMTaskIntents) != 2 || ri.CRMTaskIntents[1] != ReplyIntentQuestion {
		t.Errorf("reply intent = %+v", ri)
	}
	if !it.HoldOnNotNow || it.NotNowHoldDays != 30 || len(it.Languages) != 2 || len(it.Questions) != 2 {
		t.Fatalf("inbox tagging = %+v", it)
	}
	q := it.Questions[1]
	if q.Type != InboxTagQuestionChoice || !q.Automated || len(q.Choices) != 2 || q.Choices[1].Action.Type != InboxTagActionHold || q.Choices[1].Action.HoldDays != 90 {
		t.Errorf("choice question = %+v", q)
	}

	// Writing back what was read keeps the tagging policy rather than resetting it.
	c = syncServer(t, http.StatusNoContent, "", "", &got)
	if _, err := c.Outreach.Update(context.Background(), s); err != nil {
		t.Fatalf("Update: %v", err)
	}
	assertRequest(t, got, "PATCH", "/v1/outreach/settings", "",
		`"inbox_tagging":{"hold_on_not_now":true`, `"crm_task_intents":["positive","question"]`,
		`"hold_on_out_of_office":true`, `"label":"Has budget","action":{"type":"task"}`)

	// Unset intents travel as null, which the server reads as its default.
	if b, _ := json.Marshal(ReplyIntentSettings{}); !strings.Contains(string(b), `"crm_task_intents":null`) {
		t.Errorf("zero reply intent = %s", b)
	}
}
