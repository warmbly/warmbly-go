package warmbly

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// placementSyncClient records every request and answers with the fixture
// registered for "METHOD /path", falling back to {"data":{}}. A fixture of
// "204" answers 204 with no body.
func placementSyncClient(t *testing.T, fixtures map[string]string, got *recordedRequest) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got != nil {
			body, _ := io.ReadAll(r.Body)
			*got = recordedRequest{method: r.Method, path: r.URL.Path, rawQuery: r.URL.RawQuery, body: string(body)}
		}
		fixture, ok := fixtures[r.Method+" "+r.URL.Path]
		if !ok {
			fixture = `{"data":{}}`
		}
		if fixture == "204" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fixture))
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestPlacementSyncRouting(t *testing.T) {
	const page = `{"data":[],"pagination":{"total":0,"has_more":false,"next_cursor":null}}`
	var got recordedRequest
	c := placementSyncClient(t, map[string]string{
		"GET /v1/placement/tests":                       page,
		"POST /v1/placement/tests":                      `{"data":[]}`,
		"GET /v1/placement/batches":                     page,
		"GET /v1/placement/batches/b_1/senders":         page,
		"GET /v1/placement/seeds":                       `{"data":[]}`,
		"DELETE /v1/campaigns/camp_1/placement-monitor": "204",
		"GET /v1/campaigns/camp_1/send-plan":            `{}`,
		"GET /v1/analytics/direct":                      `{}`,
		"GET /v1/analytics/warmup/placement":            `{}`,
		"GET /v1/analytics/inbox-tagging":               `{"enabled":true,"data":[],"total":0,"summary":{},"pagination":{"has_more":false}}`,
	}, &got)
	ctx := context.Background()
	from := time.Date(2026, 9, 1, 15, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name       string
		call       func() error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string // exact; "" requires an empty body
	}{
		{"Overview", func() error { _, _, e := c.Placement.Overview(ctx); return e },
			"GET", "/v1/placement/overview", "", ""},
		{"ListTests", func() error {
			_, e := c.Placement.ListTests(ctx, &PlacementTestListParams{ListOptions: ListOptions{Limit: 10, Cursor: "MjU="}, CampaignID: "camp_1"})
			return e
		}, "GET", "/v1/placement/tests", "campaign_id=camp_1&cursor=MjU%3D&limit=10", ""},
		{"ListTests nil", func() error { _, e := c.Placement.ListTests(ctx, nil); return e },
			"GET", "/v1/placement/tests", "", ""},
		{"GetTest", func() error { _, _, e := c.Placement.GetTest(ctx, "t_1"); return e },
			"GET", "/v1/placement/tests/t_1", "", ""},
		{"CreateTest", func() error {
			_, _, e := c.Placement.CreateTest(ctx, &PlacementTestCreateParams{
				SenderAccountID: "em_1", CampaignID: "camp_1", SequenceID: "seq_1",
				Tracking: PlacementTrackingCompare, Panel: PlacementPanelWorkspace,
				SeedIDs: []string{"em_9"}, Families: []string{"gmail"}, Pace: PlacementPaceQuick, MaxCredits: 50,
			})
			return e
		}, "POST", "/v1/placement/tests", "",
			`{"sender_account_id":"em_1","campaign_id":"camp_1","sequence_id":"seq_1","tracking":"compare","panel":"workspace","seed_ids":["em_9"],"families":["gmail"],"pace":"quick","max_credits":50}` + "\n"},
		{"CancelTest", func() error { _, _, e := c.Placement.CancelTest(ctx, "t_1"); return e },
			"POST", "/v1/placement/tests/t_1/cancel", "", ""},
		{"ListBatches", func() error {
			_, e := c.Placement.ListBatches(ctx, &ListOptions{Limit: 5})
			return e
		}, "GET", "/v1/placement/batches", "limit=5", ""},
		{"GetBatch", func() error { _, _, e := c.Placement.GetBatch(ctx, "b_1"); return e },
			"GET", "/v1/placement/batches/b_1", "", ""},
		{"ListBatchSenders", func() error {
			_, e := c.Placement.ListBatchSenders(ctx, "b_1", &PlacementBatchSenderListParams{
				Status: PlacementSenderDeferred, Query: "acme", Sort: PlacementSenderSortBest,
			})
			return e
		}, "GET", "/v1/placement/batches/b_1/senders", "q=acme&sort=best&status=deferred", ""},
		{"PreviewBatch", func() error {
			_, _, e := c.Placement.PreviewBatch(ctx, &PlacementBatchParams{
				SenderScope: &PlacementSenderScope{Type: PlacementScopeWorkspace, Providers: []string{"gmail"}, UntestedDays: 14},
				Sample:      &PlacementSample{Mode: PlacementSamplePercent, Percent: 20, Stratify: "provider"},
				Subject:     "Hi", BodyPlain: "Hello",
			})
			return e
		}, "POST", "/v1/placement/batches/preview", "",
			`{"sender_scope":{"type":"workspace","providers":["gmail"],"untested_days":14},"sample":{"mode":"percent","percent":20,"stratify":"provider"},"subject":"Hi","body_plain":"Hello"}` + "\n"},
		{"CreateBatch", func() error {
			_, _, e := c.Placement.CreateBatch(ctx, &PlacementBatchParams{
				SenderAccountIDs: []string{"em_1", "em_2"}, CampaignID: "camp_1",
				OnUnavailable: PlacementUnavailableSkip, MaxCredits: 100,
			})
			return e
		}, "POST", "/v1/placement/batches", "",
			`{"sender_account_ids":["em_1","em_2"],"campaign_id":"camp_1","on_unavailable":"skip","max_credits":100}` + "\n"},
		{"CancelBatch", func() error { _, _, e := c.Placement.CancelBatch(ctx, "b_1"); return e },
			"POST", "/v1/placement/batches/b_1/cancel", "", ""},
		{"Coverage", func() error { _, _, e := c.Placement.Coverage(ctx); return e },
			"GET", "/v1/placement/coverage", "", ""},
		{"ListSeeds", func() error { _, _, e := c.Placement.ListSeeds(ctx); return e },
			"GET", "/v1/placement/seeds", "", ""},
		{"SetSeed off", func() error { _, _, e := c.Placement.SetSeed(ctx, "em_1", false); return e },
			"PUT", "/v1/placement/seeds/em_1", "", `{"seed":false}` + "\n"},
		{"campaigns.PlacementMonitor", func() error { _, _, e := c.Campaigns.PlacementMonitor(ctx, "camp_1"); return e },
			"GET", "/v1/campaigns/camp_1/placement-monitor", "", ""},
		{"campaigns.SetPlacementMonitor", func() error {
			_, _, e := c.Campaigns.SetPlacementMonitor(ctx, "camp_1", &PlacementMonitorParams{
				Enabled: Bool(false), IntervalDays: Int(3), Panel: String(PlacementPanelCloud), AlertBelow: Int(0), PauseOnAlert: Bool(true),
			})
			return e
		}, "PUT", "/v1/campaigns/camp_1/placement-monitor", "",
			`{"enabled":false,"interval_days":3,"panel":"cloud","alert_below":0,"pause_on_alert":true}` + "\n"},
		{"campaigns.SetPlacementMonitor nil", func() error {
			_, _, e := c.Campaigns.SetPlacementMonitor(ctx, "camp_1", nil)
			return e
		}, "PUT", "/v1/campaigns/camp_1/placement-monitor", "", "{}\n"},
		{"campaigns.DeletePlacementMonitor", func() error { _, e := c.Campaigns.DeletePlacementMonitor(ctx, "camp_1"); return e },
			"DELETE", "/v1/campaigns/camp_1/placement-monitor", "", ""},
		{"campaigns.SendPlan", func() error { _, _, e := c.Campaigns.SendPlan(ctx, "camp_1"); return e },
			"GET", "/v1/campaigns/camp_1/send-plan", "", ""},
		{"analytics.DirectMail", func() error { _, _, e := c.Analytics.DirectMail(ctx, Period30Days); return e },
			"GET", "/v1/analytics/direct", "period=30d", ""},
		{"analytics.WarmupPlacement", func() error {
			_, _, e := c.Analytics.WarmupPlacement(ctx, &WarmupPlacementParams{EmailID: "em_1", From: from, To: to})
			return e
		}, "GET", "/v1/analytics/warmup/placement", "email_id=em_1&from=2026-09-01&to=2026-09-30", ""},
		{"analytics.WarmupPlacement nil", func() error { _, _, e := c.Analytics.WarmupPlacement(ctx, nil); return e },
			"GET", "/v1/analytics/warmup/placement", "", ""},
		{"analytics.InboxTagging", func() error {
			_, e := c.Analytics.InboxTagging(ctx, &InboxTaggingParams{ListOptions: ListOptions{Limit: 200}, NeedsReview: true})
			return e
		}, "GET", "/v1/analytics/inbox-tagging", "limit=200&needs_review=true", ""},
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
			if got.rawQuery != tc.wantQuery {
				t.Errorf("query = %q, want %q", got.rawQuery, tc.wantQuery)
			}
			if got.body != tc.wantBody {
				t.Errorf("body = %q, want %q", got.body, tc.wantBody)
			}
		})
	}
}

// placementTestFixture mirrors placement.TestDetail on the server: the
// embedded models.PlacementTest, its summary and families, every result, the
// content check and the compare half.
const placementTestFixture = `{"data":{
	"id":"t_1","sender_account_id":"em_1","sender_email":"sam@acme.com","created_by":null,
	"campaign_id":"camp_1","sequence_id":"seq_1","contact_id":null,"monitor_id":null,"batch_id":null,
	"subject":"Quick question","body_html":"<p>Hi</p>","open_tracking":true,"link_tracking":false,
	"compare_group_id":"g_1","origin":"manual","panel":"instance","status":"completed","pace":"spaced",
	"credits_charged":25,"credits_refunded":0,"credits_settled_at":"2026-10-01T12:00:00Z",
	"created_at":"2026-10-01T10:00:00Z","finished_at":"2026-10-01T11:00:00Z",
	"summary":{"total":20,"pending":0,"inbox":14,"promotions":2,"other":1,"spam":2,"missing":1,"failed":0,
		"unknown":0,"archive":0,"custom":0,"observed_receipts":19,"classified_receipts":19,"unresolved":1,
		"primary_metric":{"version":"observed-v2","population":"seed_panel","denominator_kind":"classified_observed_receipts",
			"unit":"fraction","source":"mailbox_observation","classification_policy":"first_folder_v2_with_legacy_classifications",
			"numerator":14,"denominator":19,"unresolved":1,"window_basis":"report_date_range_utc",
			"missingness_basis":"unclassified_observed_receipts","value":0.7368,
			"wilson_95_independence_interval":{"lower":0.51,"upper":0.88}},
		"non_spam_metric":null,"delivered":20,"inbox_rate":0.7,"tabs_rate":0.15,"spam_rate":0.1,"missing_rate":0.05},
	"families":[{"family":"gmail","label":"Gmail","counts":{"total":5,"inbox":5,"delivered":5,"inbox_rate":1,
		"tabs_rate":0,"spam_rate":0,"missing_rate":0,"primary_metric":null,"non_spam_metric":null}}],
	"results":[
		{"seed":"s***@gmail.com","family":"gmail","family_label":"Gmail","folder":"promotions",
			"scheduled_at":"2026-10-01T10:00:00Z","sent_at":"2026-10-01T10:01:00Z","detected_at":"2026-10-01T10:03:00Z",
			"first_folder":"inbox","observed_at":"2026-10-01T10:03:00Z","late_observation":true,
			"evidence":{"dkim_verification":{"dkim":"pass","alignment":"pass","signing_domain":"acme.com",
				"verifier":"warmbly","observed_at":"2026-10-01T10:03:00Z"},
				"version":"received-v1","source":"authentication_results","trust":"provider","observed_at":"2026-10-01T10:03:00Z",
				"spf":"pass","dkim":"pass","dmarc":"pass","alignment":"pass","tls":"pass","from_domain":"acme.com",
				"envelope_domain_claim":"acme.com","signing_domain_claim":"acme.com","selector_claim":"s1",
				"authentication_results_present":true,"one_click_headers_present":true,"one_click_signing_claim":true,
				"one_click_compliance":"compliant"}},
		{"seed":"x***@yahoo.com","family":"yahoo","family_label":"Yahoo","folder":"brand_new_folder",
			"scheduled_at":null,"sent_at":null,"detected_at":null,"error":"never sent","evidence":null}
	],
	"content":{"score":82,"issues":[{"severity":"warn","code":"spammy_phrase","message":"Avoid \"act now\"",
		"field":"body","spans":[{"field":"body","text":"act now","line":2}],"suggestion":"Say it plainly."}]},
	"compare":{"id":"t_2","sender_account_id":"em_1","sender_email":"sam@acme.com","created_by":null,
		"campaign_id":"camp_1","sequence_id":"seq_1","contact_id":null,"monitor_id":null,"batch_id":null,
		"subject":"Quick question","open_tracking":false,"link_tracking":false,"compare_group_id":"g_1",
		"origin":"manual","panel":"instance","status":"running","pace":"spaced","credits_charged":0,
		"credits_refunded":0,"credits_settled_at":null,"created_at":"2026-10-01T10:00:00Z","finished_at":null,
		"summary":{"total":20,"pending":20,"delivered":0,"inbox_rate":null,"tabs_rate":null,"spam_rate":null,
			"missing_rate":null,"primary_metric":null,"non_spam_metric":null},"families":[]}
}}`

func TestPlacementTestDetailDecode(t *testing.T) {
	c := placementSyncClient(t, map[string]string{"GET /v1/placement/tests/t_1": placementTestFixture}, nil)
	d, _, err := c.Placement.GetTest(context.Background(), "t_1")
	if err != nil {
		t.Fatalf("GetTest: %v", err)
	}
	if d.ID != "t_1" || d.SenderAccountID == nil || *d.SenderAccountID != "em_1" || d.CreatedBy != nil || d.ContactID != nil {
		t.Errorf("ids = %+v", d.PlacementTest)
	}
	if d.CompareGroupID == nil || *d.CompareGroupID != "g_1" || d.Origin != PlacementOriginManual || d.Status != PlacementStatusCompleted {
		t.Errorf("compare/origin/status = %v/%q/%q", d.CompareGroupID, d.Origin, d.Status)
	}
	if d.CreditsCharged != 25 || d.CreditsSettledAt == nil || d.FinishedAt == nil || d.BodyHTML != "<p>Hi</p>" {
		t.Errorf("credits/finished/body = %d/%v/%v/%q", d.CreditsCharged, d.CreditsSettledAt, d.FinishedAt, d.BodyHTML)
	}
	s := d.Summary
	if s.Inbox != 14 || s.Observed != 19 || s.Classified != 19 || s.Unresolved != 1 || s.InboxRate == nil || *s.InboxRate != 0.7 {
		t.Errorf("summary = %+v", s)
	}
	if s.PrimaryMetric == nil || s.PrimaryMetric.PolicyVersion == "" || s.PrimaryMetric.Value == nil ||
		s.PrimaryMetric.Interval == nil || s.PrimaryMetric.Interval.Upper != 0.88 || s.NonSpamMetric != nil {
		t.Errorf("metrics = %+v / %+v", s.PrimaryMetric, s.NonSpamMetric)
	}
	if len(d.Families) != 1 || d.Families[0].Counts.InboxRate == nil || *d.Families[0].Counts.InboxRate != 1 {
		t.Errorf("families = %+v", d.Families)
	}
	if len(d.Results) != 2 {
		t.Fatalf("results = %d", len(d.Results))
	}
	r := d.Results[0]
	if r.Folder != PlacementFolderPromotions || r.FirstFolder == nil || *r.FirstFolder != "inbox" || !r.LateObservation {
		t.Errorf("result = %+v", r)
	}
	if r.Evidence == nil || r.Evidence.DKIMVerification == nil || r.Evidence.DKIMVerification.SigningDomain != "acme.com" ||
		r.Evidence.Selector != "s1" || !r.Evidence.OneClickHeaders || r.Evidence.OneClickCompliance != "compliant" {
		t.Errorf("evidence = %+v", r.Evidence)
	}
	// An unknown folder decodes as-is; a never-sent copy has null times.
	if u := d.Results[1]; u.Folder != "brand_new_folder" || u.SentAt != nil || u.Evidence != nil || u.Error != "never sent" {
		t.Errorf("unsent result = %+v", u)
	}
	if d.Content.Score != 82 || len(d.Content.Issues) != 1 || d.Content.Issues[0].Severity != TemplateIssueWarn {
		t.Errorf("content = %+v", d.Content)
	}
	if d.Compare == nil || d.Compare.ID != "t_2" || d.Compare.FinishedAt != nil || d.Compare.Summary.InboxRate != nil {
		t.Errorf("compare = %+v", d.Compare)
	}
}

func TestPlacementCreateTestDecode(t *testing.T) {
	c := placementSyncClient(t, map[string]string{
		"POST /v1/placement/tests": `{"data":[{"id":"t_1","compare_group_id":"g_1","open_tracking":true,"status":"running",
			"summary":{"total":20,"pending":20,"inbox_rate":null},"families":[]},
			{"id":"t_2","compare_group_id":"g_1","open_tracking":false,"status":"running",
			"summary":{"total":20,"pending":20,"inbox_rate":null},"families":[]}]}`,
	}, nil)
	tests, _, err := c.Placement.CreateTest(context.Background(), &PlacementTestCreateParams{SenderAccountID: "em_1", Tracking: PlacementTrackingCompare})
	if err != nil {
		t.Fatalf("CreateTest: %v", err)
	}
	if len(tests) != 2 || !tests[0].OpenTracking || tests[1].OpenTracking || *tests[1].CompareGroupID != "g_1" {
		t.Errorf("tests = %+v", tests)
	}
}

func TestPlacementOverviewDecode(t *testing.T) {
	c := placementSyncClient(t, map[string]string{
		"GET /v1/placement/overview": `{"data":{"panels":[
			{"panel":"instance","available":true,"seeds":40,"families":[{"family":"gmail","label":"Gmail","seeds":10}],"metered":true},
			{"panel":"cloud","available":false,"reason":"Link this instance to Warmbly Cloud first.","seeds":0,"families":[],"metered":true}],
			"usage":{"used":3,"limit":null,"credits_per_test":25,"credit_balance":null,
				"period_start":"2026-10-01T00:00:00Z","period_end":"2026-11-01T00:00:00Z"},
			"workspace_seeds":2,"seeds_per_test":20,"spacing_seconds":60}}`,
	}, nil)
	o, _, err := c.Placement.Overview(context.Background())
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if len(o.Panels) != 2 || !o.Panels[0].Metered || o.Panels[0].Families[0].Seeds != 10 || o.Panels[1].Available || o.Panels[1].Reason == "" {
		t.Errorf("panels = %+v", o.Panels)
	}
	if o.Usage.Limit != nil || o.Usage.CreditBalance != nil || o.Usage.CreditsPerTest != 25 || o.Usage.PeriodEnd.Month() != time.November {
		t.Errorf("usage = %+v", o.Usage)
	}
	if o.WorkspaceSeeds != 2 || o.SeedsPerTest != 20 || o.SpacingSeconds != 60 {
		t.Errorf("overview = %+v", o)
	}
}

func TestPlacementBatchDecode(t *testing.T) {
	const counts = `{"total":10,"inbox":8,"spam":2,"delivered":10,"inbox_rate":0.8,"tabs_rate":0,"spam_rate":0.2,"missing_rate":0,"primary_metric":null,"non_spam_metric":null}`
	c := placementSyncClient(t, map[string]string{
		"GET /v1/placement/batches/b_1": `{"data":{
			"id":"b_1","created_by":"u_1","campaign_id":null,"sequence_id":null,"contact_id":null,
			"subject":"Hi","tracking":"compare","panel":"instance","pace":"spaced","families":[],"seed_ids":[],
			"on_unavailable":"defer",
			"selection":{"sender_scope":{"type":"workspace","providers":["gmail"],"untested_days":30},
				"sample":{"mode":"percent","percent":25,"stratify":"domain"},"matched":200},
			"sender_count":50,"max_credits":500,"credits_spent":250,"status":"completed_with_warnings",
			"retry_until":"2026-10-02T10:00:00Z","created_at":"2026-10-01T10:00:00Z",
			"started_at":"2026-10-01T10:01:00Z","finished_at":null,
			"progress":{"total":50,"queued":0,"deferred":2,"running":0,"completed":45,"skipped":3,"failed":0},
			"summary":` + counts + `,"untracked":` + counts + `,
			"domains":[{"key":"acme.com","label":"acme.com","senders":10,"tested":9,"counts":` + counts + `}],
			"providers":[{"key":"google_workspace","label":"Google Workspace","senders":10,"tested":9,"counts":` + counts + `}],
			"recipients":[{"family":"outlook","label":"Outlook","counts":` + counts + `}],
			"matrix":[{"domain":"acme.com","recipients":[{"family":"gmail","label":"Gmail","counts":` + counts + `}]}],
			"content":{"score":100,"issues":[]}}}`,
	}, nil)
	b, _, err := c.Placement.GetBatch(context.Background(), "b_1")
	if err != nil {
		t.Fatalf("GetBatch: %v", err)
	}
	if b.Status != PlacementBatchCompletedWithWarnings || b.CreatedBy == nil || b.CampaignID != nil || b.FinishedAt != nil || b.StartedAt == nil {
		t.Errorf("batch = %+v", b.PlacementBatch)
	}
	sel := b.Selection
	if sel.Scope == nil || sel.Scope.Type != PlacementScopeWorkspace || sel.Scope.UntestedDays != 30 ||
		sel.Sample.Mode != PlacementSamplePercent || sel.Sample.Percent != 25 || sel.Matched != 200 {
		t.Errorf("selection = %+v", sel)
	}
	if b.Progress.Completed != 45 || b.Progress.Skipped != 3 || b.CreditsSpent != 250 {
		t.Errorf("progress = %+v", b.Progress)
	}
	if b.Untracked == nil || b.Untracked.Spam != 2 || len(b.Domains) != 1 || b.Domains[0].Tested != 9 ||
		len(b.Providers) != 1 || len(b.Recipients) != 1 || len(b.Matrix) != 1 || b.Matrix[0].Recipients[0].Family != "gmail" {
		t.Errorf("groups = %+v", b)
	}
}

func TestPlacementBatchSendersPaging(t *testing.T) {
	const sender = `{"id":"%s","batch_id":"b_1","email_account_id":%s,"sender_email":"sam@acme.com","sender_domain":"acme.com",
		"sender_family":"google_workspace","sender_family_label":"Google Workspace","status":"skipped","reason":"no_headroom",
		"attempts":2,"next_attempt_at":"2026-10-01T12:00:00Z","started_at":null,"finished_at":"2026-10-01T12:00:00Z",
		"summary":{"total":0,"inbox_rate":null},"test_ids":["t_1"]}`
	var got recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = recordedRequest{method: r.Method, path: r.URL.Path, rawQuery: r.URL.RawQuery}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("cursor") == "" {
			_, _ = w.Write([]byte(`{"data":[` + strings.Replace(strings.Replace(sender, "%s", "bs_1", 1), "%s", `"em_1"`, 1) +
				`],"pagination":{"total":2,"has_more":true,"next_cursor":"MQ=="}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[` + strings.Replace(strings.Replace(sender, "%s", "bs_2", 1), "%s", "null", 1) +
			`],"pagination":{"total":2,"has_more":false,"next_cursor":null}}`))
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatal(err)
	}
	page, err := c.Placement.ListBatchSenders(context.Background(), "b_1", &PlacementBatchSenderListParams{
		ListOptions: ListOptions{Limit: 1}, Status: PlacementSenderSkipped,
	})
	if err != nil {
		t.Fatalf("ListBatchSenders: %v", err)
	}
	if page.Pagination.Total == nil || *page.Pagination.Total != 2 || !page.HasMore() {
		t.Errorf("pagination = %+v", page.Pagination)
	}
	ids := make([]string, 0, 2)
	for s, err := range page.All(context.Background()) {
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		ids = append(ids, s.ID)
		if s.Status != PlacementSenderSkipped || s.Reason != "no_headroom" || s.StartedAt != nil || len(s.TestIDs) != 1 {
			t.Errorf("sender = %+v", s)
		}
	}
	if strings.Join(ids, ",") != "bs_1,bs_2" {
		t.Errorf("ids = %v", ids)
	}
	if got.rawQuery != "cursor=MQ%3D%3D&limit=1&status=skipped" {
		t.Errorf("follow-up query = %q", got.rawQuery)
	}
}

func TestPlacementSeedsAndCoverageDecode(t *testing.T) {
	c := placementSyncClient(t, map[string]string{
		"GET /v1/placement/seeds": `{"data":[{"email_account_id":"em_1","email":"seed@acme.com","family":"gmail","label":"Gmail",
			"status":"active","seed":true},{"email_account_id":"em_2","email":"x@acme.com","family":"outlook","label":"Outlook",
			"status":"paused","seed":false,"blocker":"The mailbox sends campaigns."}]}`,
		"PUT /v1/placement/seeds/em_1": `{"data":{"email_account_id":"em_1","email":"seed@acme.com","family":"gmail",
			"label":"Gmail","status":"active","seed":true}}`,
		"GET /v1/placement/coverage": `{"data":{"mailboxes":40,"tested_7d":10,"tested_30d":25,"never_tested":12}}`,
	}, nil)
	ctx := context.Background()
	seeds, _, err := c.Placement.ListSeeds(ctx)
	if err != nil {
		t.Fatalf("ListSeeds: %v", err)
	}
	if len(seeds) != 2 || !seeds[0].Seed || seeds[1].Blocker == "" {
		t.Errorf("seeds = %+v", seeds)
	}
	seed, _, err := c.Placement.SetSeed(ctx, "em_1", true)
	if err != nil || !seed.Seed || seed.EmailAccountID != "em_1" {
		t.Errorf("SetSeed = %+v, %v", seed, err)
	}
	cov, _, err := c.Placement.Coverage(ctx)
	if err != nil || cov.Mailboxes != 40 || cov.Tested7d != 10 || cov.Tested30d != 25 || cov.NeverTested != 12 {
		t.Errorf("Coverage = %+v, %v", cov, err)
	}
}

func TestPlacementMonitorDecode(t *testing.T) {
	c := placementSyncClient(t, map[string]string{
		"GET /v1/campaigns/none/placement-monitor": `{"data":null}`,
		"GET /v1/campaigns/camp_1/placement-monitor": `{"data":{"id":"m_1","campaign_id":"camp_1","created_by":null,
			"enabled":true,"interval_days":7,"panel":"instance","alert_below":70,"pause_on_alert":false,
			"next_run_at":"2026-10-08T10:00:00Z","last_run_at":"2026-10-01T10:00:00Z","last_test_id":"t_1",
			"last_alert_at":null,"last_error":"no mailbox could send","created_at":"2026-09-01T10:00:00Z",
			"updated_at":"2026-10-01T10:00:00Z"}}`,
	}, nil)
	ctx := context.Background()
	none, _, err := c.Campaigns.PlacementMonitor(ctx, "none")
	if err != nil || none != nil {
		t.Errorf("no monitor = %+v, %v", none, err)
	}
	m, _, err := c.Campaigns.PlacementMonitor(ctx, "camp_1")
	if err != nil {
		t.Fatalf("PlacementMonitor: %v", err)
	}
	if m.ID != "m_1" || m.CreatedBy != nil || m.IntervalDays != 7 || m.AlertBelow != 70 || m.LastTestID == nil ||
		m.LastAlertAt != nil || m.LastRunAt == nil || m.LastError == "" || m.NextRunAt.Day() != 8 {
		t.Errorf("monitor = %+v", m)
	}
}

func TestCampaignSendPlanDecode(t *testing.T) {
	c := placementSyncClient(t, map[string]string{
		"GET /v1/campaigns/camp_1/send-plan": `{"campaign_id":"camp_1","status":"active","day":"2026-10-11",
			"timezone":"Europe/Berlin","computed_at":"2026-10-11T09:00:00Z","stale":true,
			"configured_ceiling":200,"projected_today":120,"sent_today":40,"expected_remaining":80,
			"bottleneck":"warmup_graduation",
			"limits":[{"kind":"warmup_graduation","emails":60,"mailboxes":2},{"kind":"some_future_clamp","emails":20}],
			"window":{"sending_day":true,"open_now":false,"opens_at":"2026-10-11T10:00:00+02:00","minutes_left":420},
			"leads":{"due_now":30,"due_later_today":90,"new_leads_due_today":50,"waiting_on_step":12,
				"waiting_on_condition":3,"held":1,"waiting_on_sender":4,"new_leads_started_today":10,"max_new_leads_per_day":0},
			"mailboxes":[{"id":"em_1","email":"sam@acme.com","provider":"google","configured_cap":50,"cap_today":20,
				"limited_by":"warmup_graduation","sent_today":10,"sent_by_other_campaigns":5,"expected_remaining":5,
				"state":"no_worker","min_gap_seconds":300,
				"graduation":{"ceiling":20,"mailbox_cap":50,"days_to_full_cap":6,"held":false}}],
			"organization":{"daily_limit":1000,"sent_today":300,"remaining":700}}`,
	}, nil)
	p, _, err := c.Campaigns.SendPlan(context.Background(), "camp_1")
	if err != nil {
		t.Fatalf("SendPlan: %v", err)
	}
	if !p.Stale || p.ProjectedToday != 120 || p.ExpectedRemaining != 80 || p.Bottleneck != SendLimitWarmupGraduation || p.NextWakeAt != nil {
		t.Errorf("plan = %+v", p)
	}
	if len(p.Limits) != 2 || p.Limits[1].Kind != "some_future_clamp" || p.Limits[1].Mailboxes != 0 {
		t.Errorf("limits = %+v", p.Limits)
	}
	if p.Window.OpenNow || p.Window.OpensAt == nil || p.Window.ClosesAt != nil || p.Window.MinutesLeft != 420 {
		t.Errorf("window = %+v", p.Window)
	}
	if p.Leads.DueLaterToday != 90 || p.Leads.WaitingOnSender != 4 || p.Leads.NextDueAt != nil {
		t.Errorf("leads = %+v", p.Leads)
	}
	if len(p.Mailboxes) != 1 || p.Mailboxes[0].State != MailboxPlanNoWorker || p.Mailboxes[0].Graduation == nil ||
		p.Mailboxes[0].Graduation.DaysToFullCap != 6 || p.Mailboxes[0].ReopensAt != nil {
		t.Errorf("mailboxes = %+v", p.Mailboxes)
	}
	if p.Organization == nil || p.Organization.Remaining != 700 {
		t.Errorf("organization = %+v", p.Organization)
	}
}

func TestWarmupPlacementReportDecode(t *testing.T) {
	const counts = `"sent":100,"delivered":90,"inbox":70,"tabs":10,"spam":10,"unknown":2,"archived":1,"custom":0,
		"instrumented_receipts":80,"observed_receipts":93,"legacy_uninstrumented_receipts":13,"rescue_requested":10,
		"rescue_confirmed":null,"non_spam_metric":null,"rescued":10,"unconfirmed":5,"inbox_rate":88.89,"spam_rate":11.11`
	const rate = `{"window_days":7,"min_sample":20,"scope":"major","delivered":15,"inbox":12,"tabs":1,"spam":2,
		"inbox_rate":null,"band":"collecting","metric":{"version":"observed-v2","unit":"percent","window_days":7,"value":86.67},
		"other_delivered":0,"other_inbox_rate":null}`
	c := placementSyncClient(t, map[string]string{
		"GET /v1/analytics/warmup/placement": `{"date_range":{"from":"2026-09-01T00:00:00Z","to":"2026-09-30T00:00:00Z"},
			"summary":{` + counts + `},"rate":` + rate + `,
			"daily":[{"date":"2026-09-30",` + counts + `,"rolling_inbox_rate":null,
				"groups":[{"group":"google","inbox":5,"tabs":1,"spam":0,"rescued":0,"unknown":0,"archived":0,"custom":0}]}],
			"providers":[{"group":"microsoft",` + counts + `,"hosts":[{"host":"outlook",` + counts + `}]}],
			"mailboxes":[{"email_account_id":"em_1","email":"sam@acme.com",` + counts + `,"rate":` + rate + `,
				"daily_inbox_rate":[null,95.5]}]}`,
	}, nil)
	r, _, err := c.Analytics.WarmupPlacement(context.Background(), nil)
	if err != nil {
		t.Fatalf("WarmupPlacement: %v", err)
	}
	if r.EmailAccountID != nil || r.DateRange.To.Day() != 30 {
		t.Errorf("scope = %v/%v", r.EmailAccountID, r.DateRange)
	}
	s := r.Summary
	if s.Delivered != 90 || s.Instrumented != 80 || s.LegacyClassified != 13 || s.RescueConfirmed != nil || s.InboxRate == nil || *s.InboxRate != 88.89 {
		t.Errorf("summary = %+v", s)
	}
	if r.Rate.InboxRate != nil || r.Rate.Band != WarmupPlacementBandCollecting || r.Rate.Metric == nil || r.Rate.Metric.WindowDays != 7 {
		t.Errorf("rate = %+v", r.Rate)
	}
	if len(r.Daily) != 1 || r.Daily[0].Date != "2026-09-30" || r.Daily[0].Spam != 10 || r.Daily[0].RollingInboxRate != nil ||
		r.Daily[0].Groups[0].Group != WarmupRecipientGoogle {
		t.Errorf("daily = %+v", r.Daily)
	}
	if len(r.Providers) != 1 || r.Providers[0].Group != WarmupRecipientMicrosoft || r.Providers[0].Hosts[0].Host != "outlook" || r.Providers[0].Hosts[0].Tabs != 10 {
		t.Errorf("providers = %+v", r.Providers)
	}
	if len(r.Mailboxes) != 1 || r.Mailboxes[0].Inbox != 70 || len(r.Mailboxes[0].DailyInboxRate) != 2 ||
		r.Mailboxes[0].DailyInboxRate[0] != nil || *r.Mailboxes[0].DailyInboxRate[1] != 95.5 {
		t.Errorf("mailboxes = %+v", r.Mailboxes)
	}
}

func TestDirectMailDecode(t *testing.T) {
	c := placementSyncClient(t, map[string]string{
		"GET /v1/analytics/direct": `{"period":"7d",
			"volume":{"sent":120,"received":80,"threads_started":60,"replied":20,"reply_rate":33.3,"bounced":2,"median_reply_minutes":95},
			"tracking":{"mailboxes_opted_in":1,"mailboxes_total":3,"tracked_sent":40,"opened":20,"machine_opened":4,
				"clicked":5,"open_rate":50,"click_rate":12.5},
			"daily_trend":[{"date":"2026-10-10T00:00:00Z","sent":20,"received":12}],
			"mailboxes":[{"email_account_id":"em_1","email":"sam@acme.com","track_direct_mail":true,"sent":60,"received":40}],
			"top_contacts":[{"email":"pat@corp.com","sent":8,"received":5,"last_at":"2026-10-10T15:00:00Z"}]}`,
	}, nil)
	d, _, err := c.Analytics.DirectMail(context.Background(), "")
	if err != nil {
		t.Fatalf("DirectMail: %v", err)
	}
	if d.Period != Period7Days || d.Volume.MedianReplyMinutes != 95 || d.Volume.Bounced != 2 || d.Tracking.MachineOpened != 4 {
		t.Errorf("direct = %+v", d)
	}
	if len(d.DailyTrend) != 1 || d.DailyTrend[0].Date.Day() != 10 || len(d.Mailboxes) != 1 || !d.Mailboxes[0].TrackDirectMail ||
		len(d.TopContacts) != 1 || d.TopContacts[0].LastAt.Hour() != 15 {
		t.Errorf("breakdowns = %+v", d)
	}
}

func TestInboxTaggingReviewPaging(t *testing.T) {
	const row = `{"id":"%s","message_id":"msg_1","thread_id":"th_1","kind":"auto_reply_ooo","kind_confidence":1,
		"kind_source":"header","intent":"","intent_confidence":0,"relevance":10,"priority":"low","needs_review":true,
		"review_reason":"low_confidence","labels":["out-of-office"],"answers":{"is_back_date":"yes"},"model":"",
		"input_tokens":0,"actions":["hold"],"return_date":%s,"created_at":"2026-10-10T09:00:00Z"}`
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("cursor") == "" {
			_, _ = w.Write([]byte(`{"enabled":true,"data":[` +
				strings.Replace(strings.Replace(row, "%s", "it_1", 1), "%s", `"2026-10-20"`, 1) +
				`],"total":2,"summary":{"total":5,"needs_review":2,"from_offline":3,"acted":1},
				"pagination":{"next_cursor":"MQ==","has_more":true}}`))
			return
		}
		_, _ = w.Write([]byte(`{"enabled":true,"data":[` +
			strings.Replace(strings.Replace(row, "%s", "it_2", 1), "%s", "null", 1) +
			`],"total":2,"summary":{"total":5,"needs_review":2,"from_offline":3,"acted":1},
			"pagination":{"next_cursor":null,"has_more":false}}`))
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	rev, err := c.Analytics.InboxTagging(ctx, &InboxTaggingParams{ListOptions: ListOptions{Limit: 1}, NeedsReview: true})
	if err != nil {
		t.Fatalf("InboxTagging: %v", err)
	}
	if !rev.Enabled || rev.Total != 2 || rev.Summary.FromOffline != 3 || rev.Summary.Acted != 1 || rev.Response() == nil {
		t.Errorf("review = %+v", rev)
	}
	if rev.Pagination.Total == nil || *rev.Pagination.Total != 2 || !rev.HasMore() {
		t.Errorf("pagination = %+v", rev.Pagination)
	}
	v := rev.Data[0]
	if v.KindSource != "header" || v.ReturnDate == nil || *v.ReturnDate != "2026-10-20" || v.CreatedAt.Day() != 10 {
		t.Errorf("verdict = %+v", v)
	}
	var answers map[string]string
	if err := json.Unmarshal(v.Answers, &answers); err != nil || answers["is_back_date"] != "yes" {
		t.Errorf("answers = %s, %v", v.Answers, err)
	}
	ids := make([]string, 0, 2)
	for verdict, err := range rev.All(ctx) {
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		ids = append(ids, verdict.ID)
	}
	if strings.Join(ids, ",") != "it_1,it_2" {
		t.Errorf("ids = %v", ids)
	}
	if len(queries) != 2 || queries[1] != "cursor=MQ%3D%3D&limit=1&needs_review=true" {
		t.Errorf("queries = %v", queries)
	}
}
