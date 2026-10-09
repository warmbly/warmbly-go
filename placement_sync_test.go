package warmbly

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// syncJSON marshals v for use as a fixture body.
func syncJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return string(b)
}

// dataEnv wraps v as the {"data": v} envelope placement answers use.
func dataEnv(t *testing.T, v any) string {
	t.Helper()
	return syncJSON(t, map[string]any{"data": v})
}

func f64(v float64) *float64 { return &v }

func syncTime(h int) time.Time { return time.Date(2026, 9, 14, h, 30, 0, 0, time.UTC) }

func syncTimePtr(h int) *time.Time { t := syncTime(h); return &t }

// TestPlacementRouting asserts method, path, raw query and body of every
// placement method. The client answers {"data": {}} for the single-object
// endpoints and the routing fixture's list shape for the rest.
func TestPlacementRouting(t *testing.T) {
	var got recordedRequest
	obj := recordingFixtureClient(t, &got, `{"data": {}}`)
	list := routingClient(t, &got)
	ctx := context.Background()

	cases := []struct {
		name       string
		call       func() error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string
	}{
		{"Overview", func() error { _, _, e := obj.Placement.Overview(ctx); return e }, "GET", "/v1/placement/overview", "", ""},
		{"Coverage", func() error { _, _, e := obj.Placement.Coverage(ctx); return e }, "GET", "/v1/placement/coverage", "", ""},
		{"CreateTests", func() error {
			_, _, e := list.Placement.CreateTests(ctx, &PlacementTestCreateParams{
				SenderAccountID: "em_1", CampaignID: "camp_1", SequenceID: "st_1",
				Tracking: PlacementTrackingCompare, Panel: PlacementPanelWorkspace,
				SeedIDs: []string{"s1", "s2"}, Families: []string{"gmail"}, Pace: PlacementPaceQuick, MaxCredits: 4,
			})
			return e
		}, "POST", "/v1/placement/tests", "",
			`{"sender_account_id":"em_1","campaign_id":"camp_1","sequence_id":"st_1","tracking":"compare","panel":"workspace","seed_ids":["s1","s2"],"families":["gmail"],"pace":"quick","max_credits":4}`},
		{"CreateTests ad hoc", func() error {
			_, _, e := list.Placement.CreateTests(ctx, &PlacementTestCreateParams{SenderAccountID: "em_1", Subject: "Hi", BodyPlain: "Body"})
			return e
		}, "POST", "/v1/placement/tests", "", `{"sender_account_id":"em_1","subject":"Hi","body_plain":"Body"}`},
		{"ListTests", func() error { _, e := list.Placement.ListTests(ctx, nil); return e }, "GET", "/v1/placement/tests", "", ""},
		{"ListTests filtered", func() error {
			_, e := list.Placement.ListTests(ctx, &PlacementTestListParams{ListOptions: ListOptions{Limit: 50, Cursor: "MjU="}, CampaignID: "camp_1"})
			return e
		}, "GET", "/v1/placement/tests", "campaign_id=camp_1&cursor=MjU%3D&limit=50", ""},
		{"GetTest", func() error { _, _, e := obj.Placement.GetTest(ctx, "t 1"); return e }, "GET", "/v1/placement/tests/t 1", "", ""},
		{"CancelTest", func() error { _, _, e := obj.Placement.CancelTest(ctx, "t_1"); return e }, "POST", "/v1/placement/tests/t_1/cancel", "", ""},
		{"PreviewBatch", func() error {
			_, _, e := obj.Placement.PreviewBatch(ctx, &PlacementBatchParams{
				SenderScope: &PlacementSenderScope{Type: PlacementScopeCampaign, CampaignID: String("camp_1"), UntestedDays: 14},
				Sample:      &PlacementSample{Mode: PlacementSamplePercent, Percent: 10, Stratify: "provider"},
			})
			return e
		}, "POST", "/v1/placement/batches/preview", "",
			`{"sender_scope":{"type":"campaign","campaign_id":"camp_1","untested_days":14},"sample":{"mode":"percent","percent":10,"stratify":"provider"}}`},
		{"CreateBatch", func() error {
			_, _, e := obj.Placement.CreateBatch(ctx, &PlacementBatchParams{
				SenderAccountIDs: []string{"em_1", "em_2"}, Subject: "Hi", BodyPlain: "Body",
				OnUnavailable: PlacementUnavailableSkip, MaxCredits: 10,
			})
			return e
		}, "POST", "/v1/placement/batches", "",
			`{"sender_account_ids":["em_1","em_2"],"subject":"Hi","body_plain":"Body","on_unavailable":"skip","max_credits":10}`},
		{"ListBatches", func() error {
			_, e := list.Placement.ListBatches(ctx, &ListOptions{Limit: 20})
			return e
		}, "GET", "/v1/placement/batches", "limit=20", ""},
		{"GetBatch", func() error { _, _, e := obj.Placement.GetBatch(ctx, "b_1"); return e }, "GET", "/v1/placement/batches/b_1", "", ""},
		{"BatchSenders", func() error {
			_, e := list.Placement.BatchSenders(ctx, "b_1", &PlacementBatchSendersParams{
				ListOptions: ListOptions{Limit: 100}, Status: PlacementSenderDeferred, Query: "acme", Sort: PlacementSortBest,
			})
			return e
		}, "GET", "/v1/placement/batches/b_1/senders", "limit=100&q=acme&sort=best&status=deferred", ""},
		{"CancelBatch", func() error { _, _, e := obj.Placement.CancelBatch(ctx, "b_1"); return e }, "POST", "/v1/placement/batches/b_1/cancel", "", ""},
		{"Seeds", func() error { _, _, e := list.Placement.Seeds(ctx); return e }, "GET", "/v1/placement/seeds", "", ""},
		{"SetSeed", func() error { _, _, e := obj.Placement.SetSeed(ctx, "em_1", false); return e }, "PUT", "/v1/placement/seeds/em_1", "", `{"seed":false}`},
		{"PlacementMonitor", func() error { _, _, e := obj.Campaigns.PlacementMonitor(ctx, "camp_1"); return e }, "GET", "/v1/campaigns/camp_1/placement-monitor", "", ""},
		{"SetPlacementMonitor", func() error {
			_, _, e := obj.Campaigns.SetPlacementMonitor(ctx, "camp_1", &PlacementMonitorParams{
				Enabled: Bool(true), IntervalDays: Int(3), Panel: ptrTo(PlacementPanelWorkspace), AlertBelow: Int(80), PauseOnAlert: Bool(true),
			})
			return e
		}, "PUT", "/v1/campaigns/camp_1/placement-monitor", "",
			`{"enabled":true,"interval_days":3,"panel":"workspace","alert_below":80,"pause_on_alert":true}`},
		{"SetPlacementMonitor partial", func() error {
			_, _, e := obj.Campaigns.SetPlacementMonitor(ctx, "camp_1", &PlacementMonitorParams{Enabled: Bool(false)})
			return e
		}, "PUT", "/v1/campaigns/camp_1/placement-monitor", "",
			`{"enabled":false,"interval_days":null,"panel":null,"alert_below":null,"pause_on_alert":null}`},
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
			if body := strings.TrimSpace(got.body); body != tc.wantBody {
				t.Errorf("body = %q, want %q", body, tc.wantBody)
			}
		})
	}
}

func ptrTo[T any](v T) *T { return &v }

func TestDeletePlacementMonitor(t *testing.T) {
	var got recordedRequest
	c := campaignSyncClient(t, 204, "", &got)
	resp, err := c.Campaigns.DeletePlacementMonitor(context.Background(), "camp_1")
	if err != nil {
		t.Fatalf("DeletePlacementMonitor: %v", err)
	}
	if resp == nil || resp.StatusCode != 204 {
		t.Errorf("response = %+v", resp)
	}
	if got.method != "DELETE" || got.path != "/v1/campaigns/camp_1/placement-monitor" {
		t.Errorf("got %s %s", got.method, got.path)
	}
}

func TestPlacementCancelledMonitorIsGone(t *testing.T) {
	var got recordedRequest
	c := campaignSyncClient(t, 404, `{"error":{"code":"not_found","message":"this campaign has no placement monitor"}}`, &got)
	_, err := c.Campaigns.DeletePlacementMonitor(context.Background(), "camp_1")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 404 {
		t.Fatalf("err = %v, want a 404 *Error", err)
	}
}

func placementTestFixture() PlacementTest {
	return PlacementTest{
		ID:               "t_1",
		SenderAccountID:  String("em_1"),
		SenderEmail:      "dana@acme.com",
		CreatedBy:        String("u_1"),
		CampaignID:       String("camp_1"),
		SequenceID:       String("st_1"),
		MonitorID:        String("mon_1"),
		BatchID:          String("b_1"),
		Subject:          "Quick question",
		BodyHTML:         "<p>Hi</p>",
		BodyPlain:        "Hi",
		OpenTracking:     true,
		LinkTracking:     false,
		CompareGroupID:   String("g_1"),
		Origin:           PlacementOriginMonitor,
		Panel:            PlacementPanelWorkspace,
		Status:           PlacementStatusCompleted,
		Pace:             PlacementPaceSpaced,
		CreditsCharged:   3,
		CreditsRefunded:  1,
		CreditsSettledAt: syncTimePtr(10),
		CreatedAt:        syncTime(8),
		FinishedAt:       syncTimePtr(9),
		Summary: PlacementCounts{
			Total: 20, Inbox: 12, Promotions: 3, Other: 1, Spam: 2, Missing: 1, Failed: 1, Delivered: 19,
			InboxRate: f64(0.63), TabsRate: f64(0.21), SpamRate: f64(0.1), MissingRate: f64(0.05),
		},
		Families: []PlacementFamilyCounts{{Family: "gmail", Label: "Gmail", Counts: PlacementCounts{Total: 10, Pending: 10}}},
	}
}

func TestPlacementTestDetailDecode(t *testing.T) {
	want := PlacementTestDetail{
		PlacementTest: placementTestFixture(),
		Results: []PlacementResult{
			{Seed: "s***@gmail.com", Family: "gmail", FamilyLabel: "Gmail", Folder: PlacementFolderPromotions, ScheduledAt: syncTimePtr(8), SentAt: syncTimePtr(8), DetectedAt: syncTimePtr(9)},
			{Seed: "t***@yahoo.com", Family: "yahoo", FamilyLabel: "Yahoo", Folder: PlacementFolderFailed, Error: "smtp 550"},
		},
		Content: PlacementContentCheck{Score: 82, Issues: []PlacementContentIssue{{
			Severity: "warn", Code: "trigger_term", Message: "Spam-trigger wording", Field: "subject",
			Spans:      []PlacementContentSpan{{Field: "subject", Text: "FREE", Line: 1, Excerpt: "FREE offer"}},
			Suggestion: "Reword it.",
		}}},
		Compare: ptrTo(placementTestFixture()),
	}
	var got recordedRequest
	c := recordingFixtureClient(t, &got, dataEnv(t, want))
	have, _, err := c.Placement.GetTest(context.Background(), "t_1")
	if err != nil {
		t.Fatalf("GetTest: %v", err)
	}
	if !reflect.DeepEqual(*have, want) {
		t.Errorf("detail mismatch\n got  %+v\n want %+v", *have, want)
	}
	if have.Summary.InboxRate == nil || *have.Summary.InboxRate != 0.63 || have.Compare == nil {
		t.Errorf("summary/compare = %+v / %+v", have.Summary, have.Compare)
	}
}

// A test that has just started has nulls where a finished one has times, and
// no probe has reported, so every rate is nil.
func TestPlacementTestRunningDecode(t *testing.T) {
	const body = `{"data": {"id":"t_2","sender_account_id":null,"sender_email":"gone@acme.com","created_by":null,
	  "campaign_id":null,"sequence_id":null,"contact_id":null,"monitor_id":null,"batch_id":null,"subject":"Hi",
	  "open_tracking":false,"link_tracking":false,"compare_group_id":null,"origin":"manual","panel":"instance",
	  "status":"running","pace":"quick","credits_charged":0,"credits_refunded":0,"credits_settled_at":null,
	  "created_at":"2026-09-14T08:30:00Z","finished_at":null,
	  "summary":{"total":5,"pending":5,"inbox":0,"promotions":0,"other":0,"spam":0,"missing":0,"failed":0,"delivered":0,
	    "inbox_rate":null,"tabs_rate":null,"spam_rate":null,"missing_rate":null},"families":[]}}`
	var got recordedRequest
	c := recordingFixtureClient(t, &got, body)
	test, _, err := c.Placement.CancelTest(context.Background(), "t_2")
	if err != nil {
		t.Fatalf("CancelTest: %v", err)
	}
	if test.SenderAccountID != nil || test.FinishedAt != nil || test.Summary.InboxRate != nil || test.Status != PlacementStatusRunning {
		t.Errorf("test = %+v", test)
	}
}

func TestPlacementUnknownEnumsDecode(t *testing.T) {
	const body = `{"data": {"id":"b_1","subject":"s","tracking":"hologram","panel":"moon","pace":"warp",
	  "on_unavailable":"shrug","status":"quantum","selection":{"sample":{"mode":"vibes"},"matched":1},
	  "retry_until":"2026-09-21T00:00:00Z","created_at":"2026-09-14T00:00:00Z",
	  "progress":{},"summary":{},"domains":[],"providers":[],"recipients":[],"matrix":[],"content":{"score":100,"issues":[]}}}`
	var got recordedRequest
	c := recordingFixtureClient(t, &got, body)
	b, _, err := c.Placement.GetBatch(context.Background(), "b_1")
	if err != nil {
		t.Fatalf("an unknown enum value must decode: %v", err)
	}
	if b.Status != "quantum" || b.Panel != "moon" || b.Selection.Sample.Mode != "vibes" || b.Status.Finished() {
		t.Errorf("batch = %+v", b)
	}
}

func TestPlacementListTestsPaginates(t *testing.T) {
	var got recordedRequest
	first := PlacementTest{ID: "t_1", Status: PlacementStatusRunning, Summary: PlacementCounts{Total: 1, Pending: 1}}
	body := syncJSON(t, map[string]any{
		"data":       []PlacementTest{first},
		"pagination": map[string]any{"total": 51, "has_more": true, "next_cursor": "MjU="},
	})
	c := recordingFixtureClient(t, &got, body)
	page, err := c.Placement.ListTests(context.Background(), &PlacementTestListParams{CampaignID: "camp_1"})
	if err != nil {
		t.Fatalf("ListTests: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].ID != "t_1" || page.Pagination.Total == nil || *page.Pagination.Total != 51 || !page.HasMore() {
		t.Fatalf("page = %+v", page)
	}
	if _, err := page.Next(context.Background()); err != nil {
		t.Fatalf("Next: %v", err)
	}
	if got.rawQuery != "campaign_id=camp_1&cursor=MjU%3D" {
		t.Errorf("next query = %q", got.rawQuery)
	}
}

func TestPlacementCreateTestsDecodesTwoHalves(t *testing.T) {
	a, b := placementTestFixture(), placementTestFixture()
	b.ID = "t_2"
	b.OpenTracking = false
	var got recordedRequest
	c := recordingFixtureClient(t, &got, dataEnv(t, []PlacementTest{a, b}))
	tests, _, err := c.Placement.CreateTests(context.Background(), &PlacementTestCreateParams{SenderAccountID: "em_1", CampaignID: "camp_1", Tracking: PlacementTrackingCompare})
	if err != nil {
		t.Fatalf("CreateTests: %v", err)
	}
	if !reflect.DeepEqual(tests, []PlacementTest{a, b}) {
		t.Errorf("tests = %+v", tests)
	}
}

func TestPlacementOverviewDecode(t *testing.T) {
	want := PlacementOverview{
		Panels: []PlacementPanelInfo{
			{Panel: PlacementPanelInstance, Available: true, Seeds: 40, Metered: true,
				Families: []PlacementPanelFamily{{Family: "gmail", Label: "Gmail", Seeds: 12}}},
			{Panel: PlacementPanelCloud, Available: false, Reason: "Link this instance to Warmbly Cloud.", Families: []PlacementPanelFamily{}},
		},
		Usage:          PlacementUsage{Used: 4, Limit: Int(10), CreditsPerTest: 2, CreditBalance: Int(50), PeriodStart: syncTime(0), PeriodEnd: syncTime(23)},
		WorkspaceSeeds: 3, SeedsPerTest: 20, SpacingSeconds: 60,
	}
	var got recordedRequest
	c := recordingFixtureClient(t, &got, dataEnv(t, want))
	have, _, err := c.Placement.Overview(context.Background())
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if !reflect.DeepEqual(*have, want) {
		t.Errorf("overview mismatch\n got  %+v\n want %+v", *have, want)
	}
	if have.Usage.Remaining() != 6 {
		t.Errorf("Remaining = %d, want 6", have.Usage.Remaining())
	}
	if (PlacementUsage{}).Remaining() != -1 {
		t.Errorf("an unmetered usage must report -1 remaining")
	}
}

func TestPlacementBatchDecode(t *testing.T) {
	batch := PlacementBatch{
		ID: "b_1", CreatedBy: String("u_1"), CampaignID: String("camp_1"), Subject: "Hi", BodyPlain: "Body",
		Tracking: PlacementTrackingCompare, Panel: PlacementPanelInstance, Pace: PlacementPaceSpaced,
		Families: []string{"gmail"}, SeedIDs: []string{"s1"}, OnUnavailable: PlacementUnavailableDefer,
		Selection: PlacementBatchSelection{
			Scope:   &PlacementSenderScope{Type: PlacementScopeWorkspace, Providers: []string{"gmail"}, IncludeInactive: true, UntestedDays: 30},
			Sample:  PlacementSample{Mode: PlacementSampleRandom, Count: 50, Stratify: "domain"},
			Matched: 400,
		},
		SenderCount: 50, MaxCredits: 10, CreditsSpent: 4, Status: PlacementBatchRunning,
		RetryUntil: syncTime(23), CreatedAt: syncTime(8), StartedAt: syncTimePtr(9),
		Progress: PlacementBatchProgress{Total: 50, Queued: 10, Deferred: 5, Running: 5, Completed: 25, Failed: 5},
		Summary:  PlacementCounts{Total: 100, Inbox: 60, Delivered: 60, InboxRate: f64(1)},
	}
	want := PlacementBatchDetail{
		PlacementBatch: batch,
		Untracked:      &PlacementCounts{Total: 100, Spam: 10, Delivered: 10, SpamRate: f64(1)},
		Domains:        []PlacementBatchGroup{{Key: "acme.com", Label: "acme.com", Senders: 4, Tested: 3, Counts: PlacementCounts{Total: 6}}},
		Providers:      []PlacementBatchGroup{{Key: "gmail", Label: "Gmail", Senders: 4, Tested: 3}},
		Recipients:     []PlacementFamilyCounts{{Family: "outlook", Label: "Outlook"}},
		Matrix:         []PlacementBatchMatrixRow{{Domain: "acme.com", Recipients: []PlacementFamilyCounts{{Family: "gmail", Label: "Gmail"}}}},
		Content:        PlacementContentCheck{Score: 90, Issues: []PlacementContentIssue{}},
	}
	var got recordedRequest
	c := recordingFixtureClient(t, &got, dataEnv(t, want))
	have, _, err := c.Placement.GetBatch(context.Background(), "b_1")
	if err != nil {
		t.Fatalf("GetBatch: %v", err)
	}
	if !reflect.DeepEqual(*have, want) {
		t.Errorf("batch mismatch\n got  %+v\n want %+v", *have, want)
	}
	if have.Progress.Open() != 20 || have.Status.Finished() {
		t.Errorf("open = %d finished = %v", have.Progress.Open(), have.Status.Finished())
	}

	// CancelBatch answers the batch without the detail breakdown.
	c = recordingFixtureClient(t, &got, dataEnv(t, batch))
	stopped, _, err := c.Placement.CancelBatch(context.Background(), "b_1")
	if err != nil {
		t.Fatalf("CancelBatch: %v", err)
	}
	if !reflect.DeepEqual(*stopped, batch) {
		t.Errorf("stopped = %+v", stopped)
	}
}

func TestPlacementBatchFinished(t *testing.T) {
	for status, want := range map[PlacementBatchStatus]bool{
		PlacementBatchQueued: false, PlacementBatchRunning: false, PlacementBatchCompleted: true,
		PlacementBatchCompletedWithWarnings: true, PlacementBatchCancelled: true, PlacementBatchFailed: true, "future": false,
	} {
		if status.Finished() != want {
			t.Errorf("%q.Finished() = %v, want %v", status, status.Finished(), want)
		}
	}
}

func TestPlacementBatchPreviewDecode(t *testing.T) {
	want := PlacementBatchPreview{
		Matched: 400, Selected: 50, Inactive: 2, Domains: 7,
		Providers: []PlacementGroupCount{{Key: "gmail", Label: "Gmail", Senders: 30}},
		Variants:  2, Tests: 100, SeedsPerTest: 20, MaxSends: 2000, Metered: true, FreeTests: 6, PaidTests: 94, Credits: 188,
		Usage: PlacementUsage{Used: 4, Limit: Int(10), PeriodStart: syncTime(0), PeriodEnd: syncTime(23)}, SendersMax: 10000, Concurrency: 20,
	}
	var got recordedRequest
	c := recordingFixtureClient(t, &got, dataEnv(t, want))
	have, _, err := c.Placement.PreviewBatch(context.Background(), &PlacementBatchParams{SenderScope: &PlacementSenderScope{Type: PlacementScopeWorkspace}})
	if err != nil {
		t.Fatalf("PreviewBatch: %v", err)
	}
	if !reflect.DeepEqual(*have, want) {
		t.Errorf("preview mismatch\n got  %+v\n want %+v", *have, want)
	}
}

func TestPlacementBatchSendersDecode(t *testing.T) {
	sender := PlacementBatchSender{
		ID: "bs_1", BatchID: "b_1", EmailAccountID: String("em_1"), SenderEmail: "dana@acme.com", SenderDomain: "acme.com", SenderFamily: "google_workspace",
		Status: PlacementSenderDeferred, Reason: "no_headroom", Detail: "daily limit reached", Attempts: 2, NextAttemptAt: syncTime(12),
		SenderFamilyLabel: "Google Workspace", Summary: PlacementCounts{Total: 20, Inbox: 18, Delivered: 18, InboxRate: f64(1)}, TestIDs: []string{"t_1", "t_2"},
	}
	gone := PlacementBatchSender{ID: "bs_2", BatchID: "b_1", SenderEmail: "old@acme.com", Status: PlacementSenderSkipped, NextAttemptAt: syncTime(12), TestIDs: []string{}}
	body := syncJSON(t, map[string]any{
		"data":       []PlacementBatchSender{sender, gone},
		"pagination": map[string]any{"total": 2, "has_more": false, "next_cursor": nil},
	})
	var got recordedRequest
	c := recordingFixtureClient(t, &got, body)
	page, err := c.Placement.BatchSenders(context.Background(), "b_1", nil)
	if err != nil {
		t.Fatalf("BatchSenders: %v", err)
	}
	if !reflect.DeepEqual(page.Data, []PlacementBatchSender{sender, gone}) {
		t.Errorf("senders = %+v", page.Data)
	}
	if page.Data[1].EmailAccountID != nil || page.HasMore() {
		t.Errorf("deleted mailbox must decode to a nil id, page = %+v", page.Pagination)
	}
}

func TestPlacementSeedsAndCoverageDecode(t *testing.T) {
	seeds := []PlacementSeed{
		{EmailAccountID: "em_1", Email: "seed@acme.com", Family: "gmail", Label: "Gmail", Status: "active", Seed: true},
		{EmailAccountID: "em_2", Email: "x@acme.com", Family: "other", Label: "Other provider", Status: "error", Blocker: "not connected"},
	}
	var got recordedRequest
	c := recordingFixtureClient(t, &got, dataEnv(t, seeds))
	have, _, err := c.Placement.Seeds(context.Background())
	if err != nil {
		t.Fatalf("Seeds: %v", err)
	}
	if !reflect.DeepEqual(have, seeds) {
		t.Errorf("seeds = %+v", have)
	}

	c = recordingFixtureClient(t, &got, dataEnv(t, seeds[0]))
	one, _, err := c.Placement.SetSeed(context.Background(), "em_1", true)
	if err != nil {
		t.Fatalf("SetSeed: %v", err)
	}
	if !reflect.DeepEqual(*one, seeds[0]) || strings.TrimSpace(got.body) != `{"seed":true}` {
		t.Errorf("seed = %+v body = %s", one, got.body)
	}

	c = recordingFixtureClient(t, &got, dataEnv(t, PlacementCoverage{Mailboxes: 100, Tested7d: 40, Tested30d: 75, NeverTested: 25}))
	cov, _, err := c.Placement.Coverage(context.Background())
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	if *cov != (PlacementCoverage{Mailboxes: 100, Tested7d: 40, Tested30d: 75, NeverTested: 25}) {
		t.Errorf("coverage = %+v", cov)
	}

	c = recordingFixtureClient(t, &got, `{"data": null}`)
	empty, _, err := c.Placement.Seeds(context.Background())
	if err != nil {
		t.Fatalf("Seeds empty: %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("want an empty, non-nil slice, got %#v", empty)
	}
}

func TestPlacementMonitorDecode(t *testing.T) {
	want := PlacementMonitor{
		ID: "mon_1", CampaignID: "camp_1", CreatedBy: String("u_1"), Enabled: true, IntervalDays: 7, Panel: PlacementPanelInstance,
		AlertBelow: 70, PauseOnAlert: true, NextRunAt: syncTime(8), LastRunAt: syncTimePtr(7), LastTestID: String("t_1"),
		LastAlertAt: syncTimePtr(6), LastError: "no sending mailbox", CreatedAt: syncTime(1), UpdatedAt: syncTime(2),
	}
	var got recordedRequest
	c := recordingFixtureClient(t, &got, dataEnv(t, want))
	have, _, err := c.Campaigns.PlacementMonitor(context.Background(), "camp_1")
	if err != nil {
		t.Fatalf("PlacementMonitor: %v", err)
	}
	if !reflect.DeepEqual(*have, want) {
		t.Errorf("monitor mismatch\n got  %+v\n want %+v", *have, want)
	}

	// A campaign with no monitor answers {"data": null}: no monitor, no error.
	c = recordingFixtureClient(t, &got, `{"data": null}`)
	none, _, err := c.Campaigns.PlacementMonitor(context.Background(), "camp_1")
	if err != nil || none != nil {
		t.Errorf("monitor = %+v, err = %v; want nil, nil", none, err)
	}

	// A nil params value still sends a body the server can bind.
	c = recordingFixtureClient(t, &got, dataEnv(t, want))
	if _, _, err := c.Campaigns.SetPlacementMonitor(context.Background(), "camp_1", nil); err != nil {
		t.Fatalf("SetPlacementMonitor: %v", err)
	}
	if got.method != "PUT" || strings.TrimSpace(got.body) != `{"enabled":null,"interval_days":null,"panel":null,"alert_below":null,"pause_on_alert":null}` {
		t.Errorf("got %s %s", got.method, got.body)
	}
}

func TestPlacementNullDataNeverNil(t *testing.T) {
	ctx := context.Background()
	var got syncCapture
	c := respondingClient(t, &got, 200, `{"data":null}`)
	batch, _, err := c.Placement.GetBatch(ctx, "pb_1")
	if err != nil || batch == nil {
		t.Fatalf("GetBatch = %v, %v; want a non-nil result", batch, err)
	}
	monitor, _, err := c.Campaigns.PlacementMonitor(ctx, "camp_1")
	if err != nil || monitor != nil {
		t.Fatalf("PlacementMonitor = %v, %v; want nil for an absent monitor", monitor, err)
	}
}
