package warmbly

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// fixtureJSON renders v the way the server would send it.
func fixtureJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return string(b)
}

func wrapData(t *testing.T, v any) string {
	t.Helper()
	return `{"data":` + fixtureJSON(t, v) + `}`
}

func samplePlacementCounts() PlacementCounts {
	inbox, spam := 75.0, 5.0
	return PlacementCounts{
		Total: 10, Delivered: 8, Inbox: 6, Promotions: 1, Spam: 1, Pending: 2,
		InboxRate: &inbox, SpamRate: &spam,
		PrimaryMetric: &ObservationMetric{Numerator: 6, Denominator: 8, Value: &inbox, Interval: &ObservationInterval{Lower: 40, Upper: 93}},
	}
}

func TestPlacementRouting(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name       string
		body       string
		call       func(c *Client) error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string // exact; "" means no body
	}{
		{"Overview", `{"data":{"panels":[],"usage":{"used":1,"limit":null,"price_per_test_usd":0,"balance_usd":null,"credits_per_test":0,"credit_balance":null,"period_start":"2026-11-01T00:00:00Z","period_end":"2026-12-01T00:00:00Z"},"workspace_seeds":0,"seeds_per_test":20,"spacing_seconds":60}}`,
			func(c *Client) error { _, _, e := c.Placement.Overview(ctx); return e },
			"GET", "/v1/placement/overview", "", ""},
		{"Coverage", `{"data":{"mailboxes":10,"tested_7d":4,"tested_30d":6,"never_tested":4}}`,
			func(c *Client) error { _, _, e := c.Placement.Coverage(ctx); return e },
			"GET", "/v1/placement/coverage", "", ""},
		{"ListTests", `{"data":[],"pagination":{"total":0,"has_more":false,"next_cursor":null}}`,
			func(c *Client) error {
				_, e := c.Placement.ListTests(ctx, &PlacementListParams{ListOptions: ListOptions{Limit: 10, Cursor: "abc"}, CampaignID: "cmp_1"})
				return e
			},
			"GET", "/v1/placement/tests", "campaign_id=cmp_1&cursor=abc&limit=10", ""},
		{"GetTest", wrapData(t, PlacementTestDetail{}),
			func(c *Client) error { _, _, e := c.Placement.GetTest(ctx, "t1"); return e },
			"GET", "/v1/placement/tests/t1", "", ""},
		{"CreateTest", wrapData(t, []PlacementTest{{ID: "t1"}}),
			func(c *Client) error {
				_, _, e := c.Placement.CreateTest(ctx, &PlacementTestParams{
					SenderAccountID: "em_1", CampaignID: "cmp_1", SequenceID: "st_1",
					Tracking: PlacementTrackingCompare, Panel: PlacementPanelWorkspace, MaxUSD: Float64(1.5),
				})
				return e
			},
			"POST", "/v1/placement/tests", "",
			`{"sender_account_id":"em_1","campaign_id":"cmp_1","sequence_id":"st_1","tracking":"compare","panel":"workspace","max_usd":1.5}`},
		{"CancelTest", wrapData(t, PlacementTest{ID: "t1"}),
			func(c *Client) error { _, _, e := c.Placement.CancelTest(ctx, "t1"); return e },
			"POST", "/v1/placement/tests/t1/cancel", "", ""},
		{"ListBatches", `{"data":[],"pagination":{"total":0,"has_more":false,"next_cursor":null}}`,
			func(c *Client) error { _, e := c.Placement.ListBatches(ctx, &ListOptions{Limit: 5}); return e },
			"GET", "/v1/placement/batches", "limit=5", ""},
		{"GetBatch", wrapData(t, PlacementBatchDetail{}),
			func(c *Client) error { _, _, e := c.Placement.GetBatch(ctx, "b1"); return e },
			"GET", "/v1/placement/batches/b1", "", ""},
		{"BatchSenders", `{"data":[],"pagination":{"total":0,"has_more":false,"next_cursor":null}}`,
			func(c *Client) error {
				_, e := c.Placement.BatchSenders(ctx, "b1", &PlacementBatchSendersParams{Status: PlacementSenderFailed, Query: "acme", Sort: PlacementSortBest})
				return e
			},
			"GET", "/v1/placement/batches/b1/senders", "q=acme&sort=best&status=failed", ""},
		{"PreviewBatch", wrapData(t, PlacementBatchPreview{}),
			func(c *Client) error {
				_, _, e := c.Placement.PreviewBatch(ctx, &PlacementBatchParams{
					SenderScope: &PlacementSenderScope{Type: PlacementScopeWorkspace, Providers: []string{"gmail"}, UntestedDays: 14},
					Sample:      PlacementSample{Mode: PlacementSamplePercent, Percent: 10, Stratify: "domain"},
					Subject:     "Hi", BodyPlain: "Hello",
				})
				return e
			},
			"POST", "/v1/placement/batches/preview", "",
			`{"sender_scope":{"type":"workspace","providers":["gmail"],"untested_days":14},"sample":{"mode":"percent","percent":10,"stratify":"domain"},"subject":"Hi","body_plain":"Hello"}`},
		{"CreateBatch", wrapData(t, PlacementBatchView{}),
			func(c *Client) error {
				_, _, e := c.Placement.CreateBatch(ctx, &PlacementBatchParams{
					SenderAccountIDs: []string{"em_1", "em_2"}, Sample: PlacementSample{Mode: PlacementSampleAll},
					OnUnavailable: PlacementUnavailableDefer, MaxUSD: Float64(5),
				})
				return e
			},
			"POST", "/v1/placement/batches", "",
			`{"sender_account_ids":["em_1","em_2"],"sample":{"mode":"all"},"on_unavailable":"defer","max_usd":5}`},
		{"CancelBatch", wrapData(t, PlacementBatchView{}),
			func(c *Client) error { _, _, e := c.Placement.CancelBatch(ctx, "b1"); return e },
			"POST", "/v1/placement/batches/b1/cancel", "", ""},
		{"Seeds", `{"data":[{"email_account_id":"em_1","email":"a@b.co","family":"gmail","label":"Gmail","status":"active","seed":true}]}`,
			func(c *Client) error {
				seeds, _, e := c.Placement.Seeds(ctx)
				if e == nil && (len(seeds) != 1 || !seeds[0].Seed) {
					t.Errorf("seeds = %+v", seeds)
				}
				return e
			},
			"GET", "/v1/placement/seeds", "", ""},
		{"SetSeed", wrapData(t, PlacementSeed{EmailAccountID: "em_1", Seed: true}),
			func(c *Client) error { _, _, e := c.Placement.SetSeed(ctx, "em_1", true); return e },
			"PUT", "/v1/placement/seeds/em_1", "", `{"seed":true}`},
		{"Campaigns.PlacementMonitor", `{"data":null}`,
			func(c *Client) error {
				m, _, e := c.Campaigns.PlacementMonitor(ctx, "cmp_1")
				if e == nil && m != nil {
					t.Errorf("monitor = %+v, want nil", m)
				}
				return e
			},
			"GET", "/v1/campaigns/cmp_1/placement-monitor", "", ""},
		{"Campaigns.SetPlacementMonitor", wrapData(t, PlacementMonitor{ID: "mon_1", CampaignID: "cmp_1"}),
			func(c *Client) error {
				_, _, e := c.Campaigns.SetPlacementMonitor(ctx, "cmp_1", &PlacementMonitorParams{Enabled: Bool(true), IntervalDays: Int(7), AlertBelow: Int(80), PauseOnAlert: Bool(true)})
				return e
			},
			"PUT", "/v1/campaigns/cmp_1/placement-monitor", "", `{"enabled":true,"interval_days":7,"alert_below":80,"pause_on_alert":true}`},
		{"Campaigns.DeletePlacementMonitor", ``,
			func(c *Client) error { _, e := c.Campaigns.DeletePlacementMonitor(ctx, "cmp_1"); return e },
			"DELETE", "/v1/campaigns/cmp_1/placement-monitor", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got recordedRequest
			c := recordingFixtureClient(t, &got, tc.body)
			if err := tc.call(c); err != nil {
				t.Fatalf("call: %v", err)
			}
			if got.method != tc.wantMethod || got.path != tc.wantPath {
				t.Errorf("request = %s %s, want %s %s", got.method, got.path, tc.wantMethod, tc.wantPath)
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

func TestPlacementTestDetailDecode(t *testing.T) {
	sent := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	first := PlacementFolderSpam
	want := PlacementTestDetail{
		PlacementTest: PlacementTest{
			ID: "t1", SenderEmail: "a@b.co", Origin: PlacementOriginBatch, Panel: PlacementPanelInstance,
			Status: PlacementStatusRunning, Pace: PlacementPaceQuick, ChargedUSD: 0.25, CreatedAt: sent,
			BatchID: String("b1"), Summary: samplePlacementCounts(),
			Families: []PlacementFamilyCounts{{Family: "gmail", Label: "Gmail", Counts: samplePlacementCounts()}},
		},
		Results: []PlacementResult{{
			Seed: "s***@gmail.com", Family: "gmail", FamilyLabel: "Gmail", Folder: PlacementFolderInbox,
			SentAt: &sent, FirstFolder: &first, LateObservation: true,
			Evidence: &ReceivedEvidence{SPF: "pass", DKIM: "pass", DMARC: "pass", ObservedAt: sent, DKIMVerification: &DiagnosticDKIMResult{DKIM: "pass", Verifier: "x", ObservedAt: sent}},
		}},
		Content: PlacementContentCheck{Score: 80, Issues: []ContentIssue{{Severity: "warn", Code: "spam_words", Message: "m", Spans: []ContentSpan{{Field: "subject", Text: "free", Line: 1}}}}},
	}
	var got recordedRequest
	c := recordingFixtureClient(t, &got, wrapData(t, want))
	d, _, err := c.Placement.GetTest(context.Background(), "t1")
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != "t1" || d.Origin != PlacementOriginBatch || *d.BatchID != "b1" {
		t.Errorf("test = %+v", d.PlacementTest)
	}
	if d.Summary.InboxRate == nil || *d.Summary.InboxRate != 75 || d.Summary.PrimaryMetric.Interval.Upper != 93 {
		t.Errorf("summary = %+v", d.Summary)
	}
	r := d.Results[0]
	if r.Folder != PlacementFolderInbox || r.FirstFolder == nil || *r.FirstFolder != "spam" || !r.LateObservation || r.Evidence.DKIMVerification.DKIM != "pass" {
		t.Errorf("result = %+v", r)
	}
	if d.Content.Score != 80 || d.Content.Issues[0].Spans[0].Text != "free" {
		t.Errorf("content = %+v", d.Content)
	}
}

func TestPlacementUnknownEnumsDecode(t *testing.T) {
	var got recordedRequest
	body := `{"data":{"id":"b1","status":"brand_new_status","panel":"future_panel","tracking":"x","selection":{"sample":{"mode":"future"},"matched":3},"progress":{"total":3},"summary":{"total":0}}}`
	c := recordingFixtureClient(t, &got, body)
	b, _, err := c.Placement.CancelBatch(context.Background(), "b1")
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != "brand_new_status" || b.Panel != "future_panel" || b.Selection.Sample.Mode != "future" || b.Progress.Total != 3 {
		t.Errorf("batch = %+v", b)
	}
}

func TestPlacementPageIterates(t *testing.T) {
	var got recordedRequest
	next := "cursor2"
	body := `{"data":[{"id":"t1","summary":{"total":1},"families":[]}],"pagination":{"total":2,"has_more":true,"next_cursor":"` + next + `"}}`
	c := recordingFixtureClient(t, &got, body)
	page, err := c.Placement.ListTests(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !page.HasMore() || page.NextCursor() != next || page.Data[0].ID != "t1" || *page.Pagination.Total != 2 {
		t.Errorf("page = %+v", page)
	}
}

func TestCampaignLeadRouting(t *testing.T) {
	ctx := context.Background()
	until := time.Date(2026, 12, 1, 9, 0, 0, 0, time.UTC)
	holdBody := `{"campaign_id":"cmp_1","contact_id":"ct_1","hold":{"since":"2026-11-08T10:00:00Z","until":"2026-12-01T09:00:00Z","reason":"vacation","source":"manual"}}`
	cases := []struct {
		name       string
		body       string
		call       func(c *Client) error
		wantMethod string
		wantPath   string
		wantBody   string
	}{
		{"LeadHold", holdBody,
			func(c *Client) error {
				h, _, e := c.Campaigns.LeadHold(ctx, "cmp_1", "ct_1")
				if e == nil && (h.Hold == nil || h.Hold.Source != LeadHoldManual || h.Hold.Until == nil || !h.Hold.Until.Equal(until)) {
					t.Errorf("hold = %+v", h)
				}
				return e
			},
			"GET", "/v1/campaigns/cmp_1/leads/ct_1/hold", ""},
		{"PauseLead", holdBody,
			func(c *Client) error {
				_, _, e := c.Campaigns.PauseLead(ctx, "cmp_1", "ct_1", &CampaignLeadPauseParams{Until: &until, Reason: "vacation"})
				return e
			},
			"POST", "/v1/campaigns/cmp_1/leads/ct_1/pause", `{"until":"2026-12-01T09:00:00Z","reason":"vacation"}`},
		{"PauseLead no body", holdBody,
			func(c *Client) error { _, _, e := c.Campaigns.PauseLead(ctx, "cmp_1", "ct_1", nil); return e },
			"POST", "/v1/campaigns/cmp_1/leads/ct_1/pause", ""},
		{"ResumeLead", `{"campaign_id":"cmp_1","contact_id":"ct_1"}`,
			func(c *Client) error {
				h, _, e := c.Campaigns.ResumeLead(ctx, "cmp_1", "ct_1")
				if e == nil && h.Hold != nil {
					t.Errorf("hold = %+v, want nil", h.Hold)
				}
				return e
			},
			"POST", "/v1/campaigns/cmp_1/leads/ct_1/resume", ""},
		{"LeadCC", `{"campaign_id":"cmp_1","contact_id":"ct_1","cc":[{"contact_id":"ct_2","email":"x@y.co","first_name":"X","last_name":"Y","status":"bounced","bounced_at":"2026-11-01T00:00:00Z"}]}`,
			func(c *Client) error {
				l, _, e := c.Campaigns.LeadCC(ctx, "cmp_1", "ct_1")
				if e == nil && (len(l.CC) != 1 || l.CC[0].Copied() || l.CC[0].BouncedAt == nil) {
					t.Errorf("cc = %+v", l)
				}
				return e
			},
			"GET", "/v1/campaigns/cmp_1/leads/ct_1/cc", ""},
		{"SetLeadCC", `{"campaign_id":"cmp_1","contact_id":"ct_1","cc":[]}`,
			func(c *Client) error {
				_, _, e := c.Campaigns.SetLeadCC(ctx, "cmp_1", "ct_1", []string{"ct_2", "ct_3"})
				return e
			},
			"PUT", "/v1/campaigns/cmp_1/leads/ct_1/cc", `{"contact_ids":["ct_2","ct_3"]}`},
		{"SetLeadCC clear", `{"campaign_id":"cmp_1","contact_id":"ct_1","cc":[]}`,
			func(c *Client) error { _, _, e := c.Campaigns.SetLeadCC(ctx, "cmp_1", "ct_1", nil); return e },
			"PUT", "/v1/campaigns/cmp_1/leads/ct_1/cc", `{"contact_ids":[]}`},
		{"SuggestLeadCC", `{"data":[{"contact_id":"ct_2","email":"x@y.co","first_name":"X","last_name":"Y","reason":"domain"}]}`,
			func(c *Client) error {
				s, _, e := c.Campaigns.SuggestLeadCC(ctx, "cmp_1", "ct_1")
				if e == nil && (len(s) != 1 || s[0].Reason != "domain") {
					t.Errorf("suggestions = %+v", s)
				}
				return e
			},
			"GET", "/v1/campaigns/cmp_1/leads/ct_1/cc/suggestions", ""},
		{"SendPlan", fixtureJSON(t, CampaignSendPlan{CampaignID: "cmp_1", Bottleneck: SendLimitSpacing, Limits: []CampaignSendLimit{{Kind: SendLimitSpacing, Emails: 5}}, Mailboxes: []CampaignMailboxPlan{{ID: "em_1", State: MailboxPlanSending, Graduation: &ColdRamp{Ceiling: 5, MailboxCap: 50}}}}),
			func(c *Client) error {
				p, _, e := c.Campaigns.SendPlan(ctx, "cmp_1")
				if e == nil && (p.Bottleneck != SendLimitSpacing || p.Mailboxes[0].Graduation.MailboxCap != 50) {
					t.Errorf("plan = %+v", p)
				}
				return e
			},
			"GET", "/v1/campaigns/cmp_1/send-plan", ""},
		{"WarmupPlacement", `{"data":[{"sequence_id":"st_1","variant_id":null,"placement":{"sent":10,"reported":8,"inbox":6,"tabs":1,"spam":1,"rescued":0},"by_group":[{"group":"google","sent":10,"reported":8,"inbox":6,"tabs":1,"spam":1,"rescued":0}]}],"days":30}`,
			func(c *Client) error {
				p, _, e := c.Campaigns.WarmupPlacement(ctx, "cmp_1")
				if e == nil && (p.Days != 30 || p.Data[0].VariantID != nil || p.Data[0].ByGroup[0].Group != "google" || p.Data[0].ByGroup[0].Inbox != 6) {
					t.Errorf("placement = %+v", p)
				}
				return e
			},
			"GET", "/v1/campaigns/cmp_1/warmup-placement", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got recordedRequest
			c := recordingFixtureClient(t, &got, tc.body)
			if err := tc.call(c); err != nil {
				t.Fatalf("call: %v", err)
			}
			if got.method != tc.wantMethod || got.path != tc.wantPath {
				t.Errorf("request = %s %s, want %s %s", got.method, got.path, tc.wantMethod, tc.wantPath)
			}
			if strings.TrimSpace(got.body) != tc.wantBody {
				t.Errorf("body = %q, want %q", got.body, tc.wantBody)
			}
		})
	}
}

func TestCampaignShapeDecode(t *testing.T) {
	fixture := `{"id":"cmp_1","name":"Q4","status":"active","stop_on_meeting":true,
		"timezone":"","effective_timezone":"Europe/Berlin","follow_up_sender":"same_domain","entry_delay_minutes":30,
		"sender_strategy":"tags","rotation_mode":"even"}`
	var c Campaign
	if err := json.Unmarshal([]byte(fixture), &c); err != nil {
		t.Fatal(err)
	}
	if !c.StopOnMeeting || c.EffectiveTimezone != "Europe/Berlin" || c.FollowUpSender != FollowUpSameDomain || c.EntryDelayMinutes != 30 {
		t.Errorf("campaign = %+v", c)
	}
	var st Step
	if err := json.Unmarshal([]byte(`{"id":"st_1","thread_reply":true,"kind":"email"}`), &st); err != nil || !st.ThreadReply {
		t.Errorf("step = %+v err %v", st, err)
	}
	var est CampaignEstimateResult
	body := `{"recipients":100,"mailboxes":2,"daily_capacity":40,"remaining_today":40,"sending_days":3,"estimated_finish_at":"2026-11-12T00:00:00Z",
		"steps":3,"total_sends":300,"first_touch_finish_at":null,"steady_capacity":60,"full_capacity_at":"2026-11-20T00:00:00Z","ramping":1,"held":0,
		"warmup":{"mailboxes":2,"per_day":10,"per_day_without":20,"reply_rate":30},"other_campaigns_per_day":5,"bottleneck":"warmup_graduation",
		"timeline":[{"date":"2026-11-09","sending_day":true,"capacity":40,"sends":40,"first_emails":40,"follow_ups":0,"warmup":10}],
		"senders":[{"id":"em_1","email":"a@b.co","provider":"gmail","state":"ramping","first_day_cap":10,"steady_cap":30,"warmup_per_day":5,"warmup_without":10,"full_cap_at":null}],
		"scenarios":[{"kind":"every_day","value":7,"finish_at":"2026-11-11T00:00:00Z"}]}`
	if err := json.Unmarshal([]byte(body), &est); err != nil {
		t.Fatal(err)
	}
	if est.Bottleneck != CampaignEstimateBottleneckGraduation || est.Timeline[0].Warmup != 10 || est.Senders[0].State != CampaignEstimateSenderRamping || est.Scenarios[0].Kind != CampaignEstimateScenarioEveryDay || est.FirstTouchFinishAt != nil {
		t.Errorf("estimate = %+v", est)
	}
	// The estimate body carries the new projection inputs.
	var got recordedRequest
	cl := recordingFixtureClient(t, &got, `{}`)
	_, _, err := cl.Campaigns.Estimate(context.Background(), &CampaignEstimateParams{SegmentIDs: []string{"s"}, StartTime: String("09:00"), EndTime: String("17:00"), StepWaits: []int{2, 3}, CampaignID: String("cmp_1")})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"start_time":"09:00"`, `"end_time":"17:00"`, `"step_waits":[2,3]`, `"campaign_id":"cmp_1"`} {
		if !strings.Contains(got.body, want) {
			t.Errorf("body %q lacks %s", got.body, want)
		}
	}
}
