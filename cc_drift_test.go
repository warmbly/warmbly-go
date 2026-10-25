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

type ccReq struct {
	method, path, query, body, ctype string
}

// ccClient answers every request with status and body and records the last one.
func ccClient(t *testing.T, status int, ctype, body string, got *ccReq) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*got = ccReq{r.Method, r.URL.Path, r.URL.RawQuery, string(b), r.Header.Get("Content-Type")}
		w.Header().Set("Content-Type", ctype)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func ccExpect(t *testing.T, got ccReq, method, path, query, bodyHas string) {
	t.Helper()
	if got.method != method || got.path != path || got.query != query {
		t.Errorf("request = %s %s?%s, want %s %s?%s", got.method, got.path, got.query, method, path, query)
	}
	if bodyHas == "" && got.body != "" {
		t.Errorf("unexpected body %q", got.body)
	}
	if bodyHas != "" && !strings.Contains(got.body, bodyHas) {
		t.Errorf("body %q lacks %q", got.body, bodyHas)
	}
}

func TestCCLeadHoldAndCC(t *testing.T) {
	ctx := context.Background()
	var got ccReq
	hold := `{"campaign_id":"c1","contact_id":"k1","hold":{"since":"2026-10-01T10:00:00Z","until":"2026-10-08T10:00:00Z","reason":"ooo","source":"out_of_office"}}`
	c := ccClient(t, 200, "application/json", hold, &got)

	h, _, err := c.Campaigns.GetLeadHold(ctx, "c1", "k1")
	if err != nil {
		t.Fatal(err)
	}
	ccExpect(t, got, "GET", "/v1/campaigns/c1/leads/k1/hold", "", "")
	if h.Hold == nil || h.Hold.Source != LeadHoldSourceOutOfOffice || h.Hold.Until == nil || h.Hold.Reason != "ooo" {
		t.Errorf("hold = %+v", h.Hold)
	}

	until := time.Date(2026, 10, 8, 10, 0, 0, 0, time.UTC)
	if _, _, err := c.Campaigns.PauseLead(ctx, "c1", "k1", &LeadPauseParams{Until: &until, Reason: "ooo"}); err != nil {
		t.Fatal(err)
	}
	ccExpect(t, got, "POST", "/v1/campaigns/c1/leads/k1/pause", "", `"until":"2026-10-08T10:00:00Z"`)

	if _, _, err := c.Campaigns.PauseLead(ctx, "c1", "k1", nil); err != nil {
		t.Fatal(err)
	}
	ccExpect(t, got, "POST", "/v1/campaigns/c1/leads/k1/pause", "", "")

	c = ccClient(t, 200, "application/json", `{"campaign_id":"c1","contact_id":"k1"}`, &got)
	r, _, err := c.Campaigns.ResumeLead(ctx, "c1", "k1")
	if err != nil || r.Hold != nil {
		t.Fatalf("resume: %v %+v", err, r)
	}
	ccExpect(t, got, "POST", "/v1/campaigns/c1/leads/k1/resume", "", "")

	cc := `{"campaign_id":"c1","contact_id":"k1","cc":[{"contact_id":"k2","email":"a@b.co","first_name":"A","last_name":"B","status":"bounced","bounced_at":"2026-10-02T00:00:00Z"}]}`
	c = ccClient(t, 200, "application/json", cc, &got)
	l, _, err := c.Campaigns.LeadCC(ctx, "c1", "k1")
	if err != nil {
		t.Fatal(err)
	}
	ccExpect(t, got, "GET", "/v1/campaigns/c1/leads/k1/cc", "", "")
	if len(l.CC) != 1 || l.CC[0].Copied() || l.CC[0].BouncedAt == nil {
		t.Errorf("cc = %+v", l.CC)
	}
	if _, _, err := c.Campaigns.SetLeadCC(ctx, "c1", "k1", nil); err != nil {
		t.Fatal(err)
	}
	ccExpect(t, got, "PUT", "/v1/campaigns/c1/leads/k1/cc", "", `{"contact_ids":[]}`)

	c = ccClient(t, 200, "application/json", `{"data":[{"contact_id":"k3","email":"x@y.co","first_name":"X","last_name":"Y","reason":"company"}]}`, &got)
	sug, _, err := c.Campaigns.SuggestLeadCC(ctx, "c1", "k1")
	if err != nil || len(sug) != 1 || sug[0].Reason != "company" {
		t.Fatalf("suggest: %v %+v", err, sug)
	}
	ccExpect(t, got, "GET", "/v1/campaigns/c1/leads/k1/cc/suggestions", "", "")
}

func TestCCSendPlanAndPlacementMonitor(t *testing.T) {
	ctx := context.Background()
	var got ccReq
	plan := `{"campaign_id":"c1","status":"active","day":"2026-10-25","timezone":"UTC","computed_at":"2026-10-25T08:00:00Z","stale":true,
"configured_ceiling":100,"projected_today":60,"sent_today":10,"expected_remaining":50,"bottleneck":"warmup_graduation",
"limits":[{"kind":"warmup_graduation","emails":30,"mailboxes":2}],
"window":{"sending_day":true,"open_now":true,"closes_at":"2026-10-25T18:00:00Z","minutes_left":300},
"leads":{"due_now":5,"due_later_today":2,"new_leads_due_today":3,"waiting_on_step":9,"waiting_on_condition":1,"held":2,"waiting_on_sender":0,"new_leads_started_today":1,"max_new_leads_per_day":20},
"mailboxes":[{"id":"m1","email":"a@x.co","provider":"google","configured_cap":50,"cap_today":30,"limited_by":"warmup_graduation","sent_today":10,"sent_by_other_campaigns":0,"expected_remaining":20,"state":"sending","min_gap_seconds":60,"graduation":{"ceiling":30,"mailbox_cap":50,"days_to_full_cap":4,"held":false}}],
"organization":{"daily_limit":500,"sent_today":10,"remaining":490}}`
	c := ccClient(t, 200, "application/json", plan, &got)
	p, _, err := c.Campaigns.SendPlan(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	ccExpect(t, got, "GET", "/v1/campaigns/c1/send-plan", "", "")
	if !p.Stale || p.Projected != 60 || p.Limits[0].Kind != SendLimitWarmupGraduation || p.Mailboxes[0].Graduation == nil ||
		p.Mailboxes[0].Graduation.DaysToFullCap != 4 || p.Organization == nil || p.Leads.Held != 2 || p.Window.ClosesAt == nil {
		t.Errorf("plan = %+v", p)
	}

	mon := `{"data":{"id":"m1","campaign_id":"c1","created_by":null,"enabled":true,"interval_days":7,"panel":"instance","alert_below":70,"pause_on_alert":false,"next_run_at":"2026-10-26T00:00:00Z","last_run_at":null,"last_test_id":null,"last_alert_at":null,"created_at":"2026-10-25T00:00:00Z","updated_at":"2026-10-25T00:00:00Z"}}`
	c = ccClient(t, 200, "application/json", mon, &got)
	m, _, err := c.Campaigns.SetPlacementMonitor(ctx, "c1", &CampaignPlacementMonitorParams{IntervalDays: Int(7), PauseOnAlert: Bool(false)})
	if err != nil || m == nil || m.IntervalDays != 7 || m.Panel != PlacementMonitorPanelInstance {
		t.Fatalf("set: %v %+v", err, m)
	}
	ccExpect(t, got, "PUT", "/v1/campaigns/c1/placement-monitor", "", `"interval_days":7,"pause_on_alert":false`)

	c = ccClient(t, 200, "application/json", `{"data":null}`, &got)
	m, _, err = c.Campaigns.PlacementMonitor(ctx, "c1")
	if err != nil || m != nil {
		t.Fatalf("get none: %v %+v", err, m)
	}
	ccExpect(t, got, "GET", "/v1/campaigns/c1/placement-monitor", "", "")

	c = ccClient(t, 204, "application/json", ``, &got)
	if _, err := c.Campaigns.DeletePlacementMonitor(ctx, "c1"); err != nil {
		t.Fatal(err)
	}
	ccExpect(t, got, "DELETE", "/v1/campaigns/c1/placement-monitor", "", "")
}

func TestCCCampaignShapeAndEstimate(t *testing.T) {
	ctx := context.Background()
	var got ccReq
	camp := `{"id":"c1","name":"N","status":"active","timezone":"","effective_timezone":"Europe/Berlin","entry_delay_minutes":90}`
	c := ccClient(t, 200, "application/json", camp, &got)
	cp, _, err := c.Campaigns.Update(ctx, "c1", &CampaignUpdateParams{EntryDelayMinutes: Int(90)})
	if err != nil || cp.EffectiveTimezone != "Europe/Berlin" || cp.EntryDelayMinutes != 90 || cp.Kind != "" {
		t.Fatalf("update: %v %+v", err, cp)
	}
	ccExpect(t, got, "PATCH", "/v1/campaigns/c1", "", `"entry_delay_minutes":90`)

	step := `{"id":"s1","name":"","subject":"Hi","thread_reply":true,"kind":"email"}`
	c = ccClient(t, 201, "application/json", step, &got)
	st, _, err := c.Campaigns.CreateStepWith(ctx, "c1", &StepUpdateParams{Subject: String("Hi"), ThreadReply: Bool(false)})
	if err != nil || !st.ThreadReply {
		t.Fatalf("step: %v %+v", err, st)
	}
	ccExpect(t, got, "POST", "/v1/campaigns/c1/steps", "", `"subject":"Hi","thread_reply":false`)
	if _, _, err := c.Campaigns.CreateStep(ctx, "c1"); err != nil {
		t.Fatal(err)
	}
	ccExpect(t, got, "POST", "/v1/campaigns/c1/steps", "", "")

	est := `{"recipients":100,"mailboxes":2,"daily_capacity":40,"remaining_today":40,"sending_days":3,"estimated_finish_at":"2026-10-28T00:00:00Z",
"steps":3,"total_sends":300,"first_touch_finish_at":"2026-10-27T00:00:00Z","steady_capacity":80,"full_capacity_at":null,"ramping":1,"held":0,
"warmup":{"mailboxes":2,"per_day":10},"other_campaigns_per_day":5,"bottleneck":"warmup_graduation",
"timeline":[{"date":"2026-10-25","sending_day":true,"capacity":40,"sends":40,"first_emails":30,"follow_ups":10,"warmup":5}],
"senders":[{"id":"m1","email":"a@x.co","provider":"google","state":"ramping","first_day_cap":20,"steady_cap":40,"warmup_per_day":5,"full_cap_at":"2026-11-01T00:00:00Z"}]}`
	c = ccClient(t, 200, "application/json", est, &got)
	e, _, err := c.Campaigns.Estimate(ctx, &CampaignEstimateParams{CampaignID: String("c1"), StepWaits: []int{2, 3}, StartTime: String("09:00"), EndTime: String("17:00")})
	if err != nil {
		t.Fatal(err)
	}
	ccExpect(t, got, "POST", "/v1/campaigns-estimate", "", `"start_time":"09:00","end_time":"17:00","step_waits":[2,3],"campaign_id":"c1"`)
	if e.Steps != 3 || e.TotalSends != 300 || e.Bottleneck != EstimateBottleneckGraduation || len(e.Timeline) != 1 ||
		e.Timeline[0].FollowUps != 10 || e.Senders[0].State != EstimateSenderRamping || e.Senders[0].FullCapAt == nil || e.FullCapacityAt != nil || e.Warmup.PerDay != 10 {
		t.Errorf("estimate = %+v", e)
	}

	c = ccClient(t, 200, "application/json", `{"data":[],"added":0,"withdrawn":4,"contacted":2}`, &got)
	r, _, err := c.Campaigns.SetSegments(ctx, "c1", nil)
	if err != nil || r.Withdrawn != 4 || r.Contacted != 2 {
		t.Fatalf("segments: %v %+v", err, r)
	}
}

func TestCCContactsDecodeAndBulk(t *testing.T) {
	ctx := context.Background()
	var got ccReq
	search := `{"data":[{"id":"k1","email":"a@b.co","mail_host":"google","esp_provider":"gmail","verification_requested_at":"2026-10-01T00:00:00Z",
"outreach":{"sent":3,"opened":2,"clicked":1,"replied":0,"bounced":0,"last_sent_at":"2026-10-02T00:00:00Z"},
"campaign_lead":{"status":"paused","sent":1,"opened":0,"machine_opened":0,"clicked":0,"replied":0,"bounced":0,"sender":"s@x.co","hold":{"since":"2026-10-01T00:00:00Z","source":"manual"},"cc":[{"contact_id":"k2","email":"c@d.co","first_name":"","last_name":"","status":"active"}]}}],
"pagination":{"total":1,"next_cursor":null,"has_more":false},
"lead_counts":{"total":1,"paused":1,"providers":{"gmail":1,"outlook":0,"other":0,"undetected":0}}}`
	c := ccClient(t, 200, "application/json", search, &got)
	pg, err := c.Contacts.Search(ctx, &ContactSearchParams{MailHosts: []string{"google", ""}, SortBy: "custom:tier"})
	if err != nil {
		t.Fatal(err)
	}
	ccExpect(t, got, "POST", "/v1/contacts/search", "", `"mail_hosts":["google",""],"sort_by":"custom:tier"`)
	k := pg.Data[0]
	if k.MailHost != "google" || k.Outreach == nil || k.Outreach.Opened != 2 || k.CampaignLead.Status != LeadStatusPaused ||
		k.CampaignLead.Hold == nil || k.CampaignLead.Sender != "s@x.co" || len(k.CampaignLead.CC) != 1 || k.VerificationRequestedAt == nil {
		t.Errorf("contact = %+v", k)
	}
	if pg.LeadCounts == nil || pg.LeadCounts.Paused != 1 || pg.LeadCounts.Providers.Google != 1 {
		t.Errorf("lead counts = %+v", pg.LeadCounts)
	}

	c = ccClient(t, 200, "application/json", `[]`, &got)
	_, _, err = c.Contacts.BulkUpdate(ctx, &ContactBulkUpdateParams{All: true, Filters: &ContactSearchParams{Query: "acme"}, Exclude: []string{"k9"}, Subscribe: Bool(false)})
	if err != nil {
		t.Fatal(err)
	}
	ccExpect(t, got, "PATCH", "/v1/contacts", "", `"all":true,"filters":{"query":"acme"},"exclude":["k9"]`)

	c = ccClient(t, 204, "application/json", ``, &got)
	if _, err := c.Contacts.BulkDeleteMatching(ctx, &ContactSelection{All: true, Filters: &ContactSearchParams{Subscribed: Bool(false)}}); err != nil {
		t.Fatal(err)
	}
	ccExpect(t, got, "DELETE", "/v1/contacts", "", `{"all":true,"filters":{"subscribed":false}}`)

	c = ccClient(t, 200, "application/json", `{"affected":2,"action":"verify","queued":true,"verifier":"builtin","verifier_label":"Built-in","verifier_error":"key rejected"}`, &got)
	v, _, err := c.Contacts.RequestVerification(ctx, &ContactVerificationParams{Action: VerificationActionVerify, All: true, Filters: &ContactSearchParams{VerificationStatus: "unknown"}})
	if err != nil || v.Verifier != "builtin" || v.VerifierError == "" {
		t.Fatalf("verify: %v %+v", err, v)
	}
	ccExpect(t, got, "POST", "/v1/contacts/verification", "", `"all":true,"filters":{"verification_status":"unknown"}`)

	c = ccClient(t, 200, "application/json", `{"contact":{"id":"k1","email":"a@b.co"},"match":"thread"}`, &got)
	lk, _, err := c.Contacts.LookupSender(ctx, ContactLookupParams{ThreadID: "t1", AccountID: "m1"})
	if err != nil || lk.Match != ContactLookupMatchThread || lk.Contact == nil {
		t.Fatalf("lookup: %v %+v", err, lk)
	}
	ccExpect(t, got, "GET", "/v1/contacts/lookup", "account_id=m1&thread_id=t1", "")

	c = ccClient(t, 200, "application/json", `{"id":"k1"}`, &got)
	if _, _, err := c.Contacts.Update(ctx, "k1", &ContactUpdateParams{Email: String("new@b.co")}); err != nil {
		t.Fatal(err)
	}
	ccExpect(t, got, "PATCH", "/v1/contacts/k1", "", `"email":"new@b.co"`)

	c = ccClient(t, 200, "application/json", `{"data":[{"task_id":"t1","status":"sent","message_id":"m","subject":"s","sent_at":"2026-10-01T00:00:00Z","machine_opened_at":"2026-10-01T00:00:05Z"}]}`, &got)
	em, _, err := c.Contacts.Emails(ctx, "k1")
	if err != nil || em[0].MachineOpenedAt == nil {
		t.Fatalf("emails: %v %+v", err, em)
	}

	c = ccClient(t, 200, "application/json", `{"data":[{"campaign_id":"c1","campaign_name":"N","campaign_status":"active","lead_status":"paused","sender_id":"m1","sender_email":"s@x.co","hold":{"since":"2026-10-01T00:00:00Z","source":"crm"},"cc":[],"steps":[]}]}`, &got)
	cs, _, err := c.Contacts.CampaignStates(ctx, "k1")
	if err != nil || cs[0].SenderID == nil || cs[0].Hold == nil || cs[0].Hold.Source != LeadHoldSourceCRM {
		t.Fatalf("states: %v %+v", err, cs)
	}
}

func TestCCContactImports(t *testing.T) {
	ctx := context.Background()
	var got ccReq
	imp := `{"id":"i1","organization_id":"o1","filename":"f.csv","format":"csv","status":"draft","has_header":true,"columns":["Email"],"total":2,"processed":0,"imported":0,"updated":0,"skipped":0,"failed":0,"notes":[],
"preview":{"filename":"f.csv","format":"csv","total_rows":2,"columns":["Email"],"has_header":true,"sample_rows":[["a@b.co"]],"suggested_mapping":[{"index":0,"target":"email"}],"inferred_columns":[0],"column_stats":[{"filled":2,"distinct":2,"samples":["a@b.co"]}],"mapping_source":"saved"},
"created_at":"2026-10-25T00:00:00Z","updated_at":"2026-10-25T00:00:00Z"}`
	c := ccClient(t, 201, "application/json", imp, &got)
	i, _, err := c.Contacts.CreateImport(ctx, &FileUpload{Filename: "f.csv", Content: strings.NewReader("Email\na@b.co\n")})
	if err != nil {
		t.Fatal(err)
	}
	ccExpect(t, got, "POST", "/v1/contacts/imports", "", `name="file"`)
	if i.Status != ContactImportDraft || i.Terminal() || i.Preview == nil || i.Preview.MappingSource != "saved" || i.Preview.ColumnStats[0].Filled != 2 {
		t.Errorf("import = %+v", i)
	}

	c = ccClient(t, 200, "application/json", `{"data":[`+imp+`],"pagination":{"total":null,"next_cursor":"n","has_more":true}}`, &got)
	pg, err := c.Contacts.ListImports(ctx, &ContactImportListParams{ListOptions: ListOptions{Limit: 10}})
	if err != nil || len(pg.Data) != 1 || !pg.HasMore() {
		t.Fatalf("list: %v %+v", err, pg)
	}
	ccExpect(t, got, "GET", "/v1/contacts/imports", "limit=10", "")

	c = ccClient(t, 200, "application/json", imp, &got)
	if _, _, err = c.Contacts.GetImport(ctx, "i1"); err != nil {
		t.Fatal(err)
	}
	ccExpect(t, got, "GET", "/v1/contacts/imports/i1", "", "")
	params := &ContactImportParams{Mapping: []ImportColumnMapping{{Index: 0, Target: ImportTargetEmail}}, Dedup: ImportDedupUpdate, HasHeader: true}
	if _, _, err = c.Contacts.SaveImportDraft(ctx, "i1", params); err != nil {
		t.Fatal(err)
	}
	ccExpect(t, got, "PATCH", "/v1/contacts/imports/i1", "", `"dedup":"update"`)
	if _, _, err = c.Contacts.StartImport(ctx, "i1", params); err != nil {
		t.Fatal(err)
	}
	ccExpect(t, got, "POST", "/v1/contacts/imports/i1/start", "", `"mapping":[{"index":0,"target":"email"}]`)
	if _, _, err = c.Contacts.CancelImport(ctx, "i1"); err != nil {
		t.Fatal(err)
	}
	ccExpect(t, got, "POST", "/v1/contacts/imports/i1/cancel", "", "")

	c = ccClient(t, 200, "application/json", `{"rows":10,"new":6,"existing":2,"duplicates_in_file":1,"invalid":1,"conflicts":0,"invalid_samples":null,"problem":"limit"}`, &got)
	an, _, err := c.Contacts.AnalyzeImport(ctx, "i1", &ContactImportAnalyzeParams{Mapping: params.Mapping, HasHeader: true})
	if err != nil || an.New != 6 || an.Problem != "limit" || an.InvalidSamples == nil {
		t.Fatalf("analyze: %v %+v", err, an)
	}
	ccExpect(t, got, "POST", "/v1/contacts/imports/i1/analyze", "", `"has_header":true`)

	c = ccClient(t, 200, "text/csv; charset=utf-8", "line,email,reason\n3,x,bad\n", &got)
	var sb strings.Builder
	if _, err = c.Contacts.DownloadImportFailures(ctx, "i1", &sb); err != nil {
		t.Fatal(err)
	}
	ccExpect(t, got, "GET", "/v1/contacts/imports/i1/failed.csv", "", "")
	if !strings.HasPrefix(sb.String(), "line,email") {
		t.Errorf("csv = %q", sb.String())
	}
}

func TestCCSegmentsFormsTemplates(t *testing.T) {
	ctx := context.Background()
	var got ccReq
	c := ccClient(t, 200, "application/json", `{"updated":5}`, &got)
	n, _, err := c.Segments.SetMembersMatching(ctx, "s1", &ContactSelection{All: true, Filters: &ContactSearchParams{Query: "x"}}, SegmentMemberInclude)
	if err != nil || n != 5 {
		t.Fatalf("members: %v %d", err, n)
	}
	ccExpect(t, got, "POST", "/v1/segments/s1/members", "", `"all":true,"filters":{"query":"x"},"mode":"include"`)

	c = ccClient(t, 200, "application/json", `{"data":[{"field":"mail_host","label":"Email provider","group":"Contact","kind":"enum","options":["google"],"option_labels":{"google":"Google"}}]}`, &got)
	f, _, err := c.Segments.Fields(ctx)
	if err != nil || f[0].OptionLabels["google"] != "Google" {
		t.Fatalf("fields: %v %+v", err, f)
	}

	form := `{"id":"f1","name":"F","status":"published","redirect_url":"https://x.co","redirect_url_active":false,"triage_enabled":true,"fields":[],"category_ids":[],"allowed_domains":[]}`
	c = ccClient(t, 200, "application/json", form, &got)
	fm, _, err := c.Forms.Update(ctx, "f1", &FormUpdateParams{TriageEnabled: Bool(true)})
	if err != nil || !fm.TriageEnabled || fm.RedirectURLActive == nil || *fm.RedirectURLActive {
		t.Fatalf("form: %v %+v", err, fm)
	}
	ccExpect(t, got, "PATCH", "/v1/forms/f1", "", `"triage_enabled":true`)

	c = ccClient(t, 200, "application/json", `{"base_url":"https://f.x","captcha_available":true,"triage_available":true}`, &got)
	cfg, _, err := c.Forms.Config(ctx)
	if err != nil || !cfg.TriageAvailable {
		t.Fatalf("config: %v %+v", err, cfg)
	}

	c = ccClient(t, 200, "application/json", `{"data":[{"id":"s1","form_id":"f1","data":{},"triage":"junk","triage_confidence":0.93,"created_at":"2026-10-01T00:00:00Z"}],"pagination":{"total":null,"next_cursor":null,"has_more":false}}`, &got)
	sp, err := c.Forms.ListSubmissions(ctx, "f1", nil)
	if err != nil || sp.Data[0].Triage != FormTriageJunk || sp.Data[0].TriageConfidence != 0.93 {
		t.Fatalf("submissions: %v %+v", err, sp)
	}

	analysis := `{"score":55,"verdict":"Reads as bulk mail.","findings":[{"severity":"high","field":"subject","text":"FREE","line":1,"excerpt":"FREE now","issue":"trigger word","suggestion":"drop it","category":"trigger_word"}],
"suggested_subject":"Quick question","improvements":["shorter"],
"rules":{"score":70,"issues":[{"severity":"warn","code":"caps","message":"caps","field":"subject","spans":[{"field":"subject","text":"FREE","line":1}],"suggestion":"lowercase"}]},
"judgment":{"reads_as":0.8,"personalization":0.4,"ask":"none","spam_claim":0.1,"confidence":0.9,"model":"m","input_tokens":120},
"model":"m","tokens_used":300,"credits_remaining":40,"credits_charged":2}`
	c = ccClient(t, 200, "application/json", analysis, &got)
	a, _, err := c.Templates.Analyze(ctx, &TemplateScoreParams{Subject: "FREE now", BodyPlain: "hi"}, WithIdempotencyKey("k"))
	if err != nil {
		t.Fatal(err)
	}
	ccExpect(t, got, "POST", "/v1/templates/analyze", "", `"subject":"FREE now","body_plain":"hi"`)
	if a.Score != 55 || a.Findings[0].Category != "trigger_word" || a.Rules.Issues[0].Spans[0].Text != "FREE" ||
		a.Judgment == nil || a.Judgment.Ask != "none" || a.CreditsCharged != 2 || a.SuggestedSubject == "" {
		t.Errorf("analysis = %+v", a)
	}
}
