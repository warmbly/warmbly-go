package warmbly

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCampaignLeadRouting(t *testing.T) {
	var got recordedRequest
	// The CC and hold endpoints answer bare objects; suggestions a data list.
	obj := recordingFixtureClient(t, &got, `{"campaign_id":"camp_1","contact_id":"ct_1"}`)
	list := recordingFixtureClient(t, &got, `{"data": []}`)
	ctx := context.Background()
	until := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

	cases := []struct {
		name       string
		call       func() error
		wantMethod string
		wantPath   string
		wantBody   string
	}{
		{"SendPlan", func() error { _, _, e := obj.Campaigns.SendPlan(ctx, "camp_1"); return e }, "GET", "/v1/campaigns/camp_1/send-plan", ""},
		{"LeadHold", func() error { _, _, e := obj.Campaigns.LeadHold(ctx, "camp_1", "ct_1"); return e }, "GET", "/v1/campaigns/camp_1/leads/ct_1/hold", ""},
		{"PauseLead until", func() error {
			_, _, e := obj.Campaigns.PauseLead(ctx, "camp_1", "ct_1", &LeadPauseParams{Until: &until, Reason: "on holiday"})
			return e
		}, "POST", "/v1/campaigns/camp_1/leads/ct_1/pause", `{"until":"2026-10-01T09:00:00Z","reason":"on holiday"}`},
		{"PauseLead open ended", func() error { _, _, e := obj.Campaigns.PauseLead(ctx, "camp_1", "ct_1", nil); return e }, "POST", "/v1/campaigns/camp_1/leads/ct_1/pause", ""},
		{"ResumeLead", func() error { _, e := obj.Campaigns.ResumeLead(ctx, "camp_1", "ct_1"); return e }, "POST", "/v1/campaigns/camp_1/leads/ct_1/resume", ""},
		{"LeadCC", func() error { _, _, e := obj.Campaigns.LeadCC(ctx, "camp_1", "ct_1"); return e }, "GET", "/v1/campaigns/camp_1/leads/ct_1/cc", ""},
		{"SetLeadCC", func() error {
			_, _, e := obj.Campaigns.SetLeadCC(ctx, "camp_1", "ct_1", []string{"ct_2", "ct_3"})
			return e
		}, "PUT", "/v1/campaigns/camp_1/leads/ct_1/cc", `{"contact_ids":["ct_2","ct_3"]}`},
		{"SetLeadCC clears", func() error { _, _, e := obj.Campaigns.SetLeadCC(ctx, "camp_1", "ct_1", nil); return e }, "PUT", "/v1/campaigns/camp_1/leads/ct_1/cc", `{"contact_ids":[]}`},
		{"LeadCCSuggestions", func() error {
			_, _, e := list.Campaigns.LeadCCSuggestions(ctx, "camp_1", "ct_1")
			return e
		}, "GET", "/v1/campaigns/camp_1/leads/ct_1/cc/suggestions", ""},
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
			if got.rawQuery != "" {
				t.Errorf("query = %q, want none", got.rawQuery)
			}
			if body := strings.TrimSpace(got.body); body != tc.wantBody {
				t.Errorf("body = %q, want %q", body, tc.wantBody)
			}
		})
	}
}

func TestLeadHoldDecode(t *testing.T) {
	until := syncTimePtr(18)
	held := LeadHold{Since: syncTime(8), Until: until, Reason: "back on the 20th", Source: LeadHoldOutOfOffice}
	body := syncJSON(t, map[string]any{"campaign_id": "camp_1", "contact_id": "ct_1", "hold": held})
	var got recordedRequest
	c := recordingFixtureClient(t, &got, body)

	hold, _, err := c.Campaigns.LeadHold(context.Background(), "camp_1", "ct_1")
	if err != nil {
		t.Fatalf("LeadHold: %v", err)
	}
	if hold == nil || !reflect.DeepEqual(*hold, held) {
		t.Errorf("hold = %+v, want %+v", hold, held)
	}

	// No end: until is absent. Pausing answers the same shape.
	open := LeadHold{Since: syncTime(8), Source: LeadHoldManual}
	c = recordingFixtureClient(t, &got, syncJSON(t, map[string]any{"campaign_id": "camp_1", "contact_id": "ct_1", "hold": open}))
	hold, _, err = c.Campaigns.PauseLead(context.Background(), "camp_1", "ct_1", &LeadPauseParams{})
	if err != nil {
		t.Fatalf("PauseLead: %v", err)
	}
	if hold == nil || hold.Until != nil || hold.Source != LeadHoldManual {
		t.Errorf("hold = %+v", hold)
	}
	if strings.TrimSpace(got.body) != `{}` {
		t.Errorf("an empty pause must send an empty object, got %s", got.body)
	}

	// A lead that is not held has no hold key at all.
	c = recordingFixtureClient(t, &got, `{"campaign_id":"camp_1","contact_id":"ct_1"}`)
	hold, _, err = c.Campaigns.LeadHold(context.Background(), "camp_1", "ct_1")
	if err != nil || hold != nil {
		t.Errorf("hold = %+v, err = %v; want nil, nil", hold, err)
	}

	// An unknown source decodes unchanged.
	c = recordingFixtureClient(t, &got, `{"campaign_id":"a","contact_id":"b","hold":{"since":"2026-09-14T08:30:00Z","source":"teleport"}}`)
	hold, _, err = c.Campaigns.LeadHold(context.Background(), "camp_1", "ct_1")
	if err != nil || hold.Source != "teleport" {
		t.Errorf("hold = %+v, err = %v", hold, err)
	}
}

func TestLeadCCDecode(t *testing.T) {
	cc := []CampaignLeadCC{
		{ContactID: "ct_2", Email: "ana@acme.com", FirstName: "Ana", LastName: "Ruiz", Company: "Acme", Status: LeadCCStatusActive},
		{ContactID: "ct_3", Email: "bo@acme.com", FirstName: "Bo", Status: LeadCCStatusBounced, BouncedAt: syncTimePtr(11)},
	}
	body := syncJSON(t, map[string]any{"campaign_id": "camp_1", "contact_id": "ct_1", "cc": cc})
	var got recordedRequest
	c := recordingFixtureClient(t, &got, body)

	have, _, err := c.Campaigns.LeadCC(context.Background(), "camp_1", "ct_1")
	if err != nil {
		t.Fatalf("LeadCC: %v", err)
	}
	if !reflect.DeepEqual(have, cc) {
		t.Errorf("cc = %+v, want %+v", have, cc)
	}
	if !have[0].Copied() || have[1].Copied() {
		t.Errorf("Copied() = %v/%v", have[0].Copied(), have[1].Copied())
	}

	have, _, err = c.Campaigns.SetLeadCC(context.Background(), "camp_1", "ct_1", []string{"ct_2", "ct_3"})
	if err != nil || len(have) != 2 {
		t.Errorf("SetLeadCC = %+v, %v", have, err)
	}

	// The server answers an empty list as [], but a null must read as empty too.
	c = recordingFixtureClient(t, &got, `{"campaign_id":"camp_1","contact_id":"ct_1","cc":null}`)
	have, _, err = c.Campaigns.LeadCC(context.Background(), "camp_1", "ct_1")
	if err != nil || have == nil || len(have) != 0 {
		t.Errorf("want an empty, non-nil slice, got %#v, %v", have, err)
	}

	// An unknown status decodes unchanged and is not copied.
	c = recordingFixtureClient(t, &got, `{"cc":[{"contact_id":"x","email":"x@y.z","first_name":"","last_name":"","status":"quarantined"}]}`)
	have, _, err = c.Campaigns.LeadCC(context.Background(), "camp_1", "ct_1")
	if err != nil || have[0].Status != "quarantined" || have[0].Copied() {
		t.Errorf("cc = %+v, err = %v", have, err)
	}
}

func TestLeadCCSuggestionsDecode(t *testing.T) {
	want := []LeadCCSuggestion{
		{ContactID: "ct_2", Email: "ana@acme.com", FirstName: "Ana", LastName: "Ruiz", Company: "Acme", Reason: LeadCCReasonCompany},
		{ContactID: "ct_4", Email: "cy@acme.com", Reason: LeadCCReasonDomain},
	}
	var got recordedRequest
	c := recordingFixtureClient(t, &got, dataEnv(t, want))
	have, _, err := c.Campaigns.LeadCCSuggestions(context.Background(), "camp_1", "ct_1")
	if err != nil {
		t.Fatalf("LeadCCSuggestions: %v", err)
	}
	if !reflect.DeepEqual(have, want) {
		t.Errorf("suggestions = %+v", have)
	}
}

func TestResumeLeadIgnoresBody(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"campaign_id":"camp_1","contact_id":"ct_1"}`)
	resp, err := c.Campaigns.ResumeLead(context.Background(), "camp_1", "ct_1")
	if err != nil || resp == nil || resp.StatusCode != 200 {
		t.Errorf("resp = %+v, err = %v", resp, err)
	}
}

func TestSendPlanDecode(t *testing.T) {
	want := CampaignSendPlan{
		CampaignID: "camp_1", Status: CampaignStatusActive, Day: "2026-09-14", Timezone: "Europe/Berlin",
		ComputedAt: syncTime(8), Stale: true,
		ConfiguredCeiling: 300, ProjectedToday: 180, SentToday: 60, ExpectedRemaining: 120,
		Bottleneck: SendLimitWarmupGraduation,
		Limits: []CampaignSendLimit{
			{Kind: SendLimitCampaignDailyLimit, Emails: 40, Mailboxes: 0},
			{Kind: SendLimitWarmupGraduation, Emails: 80, Mailboxes: 3},
			{Kind: "something_new", Emails: 0},
		},
		Window: CampaignSendWindow{SendingDay: true, OpenNow: true, ClosesAt: syncTimePtr(17), MinutesLeft: 420, EndsAt: syncTimePtr(23)},
		Leads: CampaignLeadSupply{
			DueNow: 50, DueLaterToday: 20, NewLeadsDueToday: 30, WaitingOnStep: 400, WaitingOnCondition: 5, Held: 2,
			WaitingOnSender: 7, NewLeadsStartedToday: 12, MaxNewLeadsPerDay: 100, NextDueAt: syncTimePtr(9),
		},
		Mailboxes: []CampaignMailboxPlan{
			{
				ID: "em_1", Email: "dana@acme.com", Provider: "google", ConfiguredCap: 100, CapToday: 40, LimitedBy: SendLimitWarmupGraduation,
				SentToday: 30, SentByOtherCampaigns: 5, ExpectedRemaining: 10, State: MailboxPlanSending, Health: "watch", MinGapSeconds: 60,
				Graduation: &ColdRamp{Ceiling: 40, MailboxCap: 100, DaysToFullCap: 6, Held: true},
			},
			{ID: "em_2", Email: "ops@acme.com", Provider: "outlook", State: MailboxPlanNoWorker, ReopensAt: syncTimePtr(22)},
		},
		Organization: &CampaignOrgAllowance{DailyLimit: 500, SentToday: 100, Remaining: 400},
		NextWakeAt:   syncTimePtr(9),
	}
	var got recordedRequest
	c := recordingFixtureClient(t, &got, syncJSON(t, want))
	have, _, err := c.Campaigns.SendPlan(context.Background(), "camp_1")
	if err != nil {
		t.Fatalf("SendPlan: %v", err)
	}
	if !reflect.DeepEqual(*have, want) {
		t.Errorf("plan mismatch\n got  %+v\n want %+v", *have, want)
	}
	// The waterfall adds up in the fixture, and the field names stay the wire's.
	total := have.ConfiguredCeiling - have.SentToday
	for _, l := range have.Limits {
		total -= l.Emails
	}
	if total != have.ExpectedRemaining || !strings.Contains(syncJSON(t, want), `"projected_today":180`) {
		t.Errorf("waterfall = %d", total)
	}
}

// A campaign that is not running has no window, nothing due and no optional
// blocks; every optional field decodes to nil.
func TestSendPlanMinimalDecode(t *testing.T) {
	const body = `{"campaign_id":"camp_1","status":"draft","day":"2026-09-14","timezone":"UTC","computed_at":"2026-09-14T08:30:00Z",
	  "stale":false,"configured_ceiling":0,"projected_today":0,"sent_today":0,"expected_remaining":0,"bottleneck":"",
	  "limits":[],"window":{"sending_day":false,"open_now":false,"minutes_left":0},"leads":{"due_now":0,"due_later_today":0,
	  "new_leads_due_today":0,"waiting_on_step":0,"waiting_on_condition":0,"held":0,"waiting_on_sender":0,
	  "new_leads_started_today":0,"max_new_leads_per_day":0},"mailboxes":[]}`
	var got recordedRequest
	c := recordingFixtureClient(t, &got, body)
	plan, _, err := c.Campaigns.SendPlan(context.Background(), "camp_1")
	if err != nil {
		t.Fatalf("SendPlan: %v", err)
	}
	if plan.Organization != nil || plan.NextWakeAt != nil || plan.Window.OpensAt != nil || plan.Leads.NextDueAt != nil || plan.Bottleneck != "" {
		t.Errorf("plan = %+v", plan)
	}
}

func TestContactCampaignProgressHoldAndCC(t *testing.T) {
	const body = `{"status":"replied","sent":2,"opened":1,"machine_opened":0,"clicked":0,"replied":1,"bounced":0,
	  "sender":"dana@acme.com",
	  "hold":{"since":"2026-09-14T08:30:00Z","source":"cc","reason":"copied on ct_9"},
	  "cc":[{"contact_id":"ct_2","email":"ana@acme.com","first_name":"Ana","last_name":"Ruiz","status":"active"}]}`
	var p ContactCampaignProgress
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.Sender != "dana@acme.com" || p.Hold == nil || p.Hold.Source != LeadHoldCC || len(p.CC) != 1 || !p.CC[0].Copied() {
		t.Errorf("progress = %+v", p)
	}
}
