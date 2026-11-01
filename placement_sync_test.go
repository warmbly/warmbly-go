package warmbly

import (
	"context"
	"strings"
	"testing"
)

const placementTestJSON = `{
	"id": "11111111-1111-1111-1111-111111111111",
	"sender_account_id": "22222222-2222-2222-2222-222222222222",
	"sender_email": "sam@acme.com",
	"created_by": null,
	"campaign_id": "33333333-3333-3333-3333-333333333333",
	"sequence_id": null,
	"contact_id": null,
	"monitor_id": null,
	"batch_id": null,
	"subject": "Quick question",
	"open_tracking": true,
	"link_tracking": false,
	"compare_group_id": null,
	"origin": "manual",
	"panel": "instance",
	"status": "running",
	"pace": "spaced",
	"credits_charged": 3,
	"credits_refunded": 0,
	"credits_settled_at": null,
	"created_at": "2026-09-07T09:00:00Z",
	"finished_at": null,
	"summary": {
		"total": 10, "pending": 4, "inbox": 4, "promotions": 1, "other": 0, "spam": 1,
		"missing": 0, "failed": 0, "unknown": 0, "archive": 0, "custom": 0,
		"observed_receipts": 6, "classified_receipts": 6, "unresolved": 4,
		"primary_metric": {
			"version": "observed-v2", "population": "seed_panel",
			"denominator_kind": "classified_observed_receipts", "unit": "fraction",
			"source": "mailbox_observation", "classification_policy": "first_folder_v2_with_legacy_classifications",
			"numerator": 4, "denominator": 6, "unresolved": 4,
			"window_basis": "placement_test_classification_window",
			"missingness_basis": "pending_missing_and_unclassified_copies",
			"value": 0.6667,
			"wilson_95_independence_interval": {"lower": 0.3, "upper": 0.9}
		},
		"non_spam_metric": null,
		"delivered": 6, "inbox_rate": 0.6667, "tabs_rate": 0.1667, "spam_rate": 0.1667, "missing_rate": 0
	},
	"families": [{"family": "gmail", "label": "Gmail", "counts": {"total": 3, "inbox": 2, "delivered": 3, "inbox_rate": 0.6667, "tabs_rate": null, "spam_rate": null, "missing_rate": null}}]
}`

func TestPlacementRouting(t *testing.T) {
	ctx := context.Background()

	t.Run("overview", func(t *testing.T) {
		var got recordedRequest
		c := recordingFixtureClient(t, &got, `{"data": {
			"panels": [
				{"panel": "instance", "available": true, "seeds": 12, "families": [{"family": "gmail", "label": "Gmail", "seeds": 6}], "metered": true},
				{"panel": "workspace", "available": false, "reason": "Mark one of this workspace's mailboxes as a seed inbox to test on it.", "seeds": 0, "families": [], "metered": false}
			],
			"usage": {"used": 2, "limit": 10, "credits_per_test": 5, "credit_balance": null,
				"period_start": "2026-09-01T00:00:00Z", "period_end": "2026-10-01T00:00:00Z"},
			"workspace_seeds": 0, "seeds_per_test": 12, "spacing_seconds": 60
		}}`)
		o, _, err := c.Placement.Overview(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got.method != "GET" || got.path != "/v1/placement/overview" {
			t.Errorf("request = %s %s", got.method, got.path)
		}
		if len(o.Panels) != 2 || o.Panels[0].Panel != PlacementPanelInstance || !o.Panels[0].Metered || o.Panels[1].Reason == "" {
			t.Errorf("panels = %+v", o.Panels)
		}
		if o.Usage.Limit == nil || *o.Usage.Limit != 10 || o.Usage.CreditBalance != nil || o.Usage.CreditsPerTest != 5 {
			t.Errorf("usage = %+v", o.Usage)
		}
		if o.SeedsPerTest != 12 || o.SpacingSeconds != 60 {
			t.Errorf("overview = %+v", o)
		}
	})

	t.Run("list tests", func(t *testing.T) {
		var got recordedRequest
		c := recordingFixtureClient(t, &got, `{"data": [`+placementTestJSON+`], "pagination": {"total": 1, "next_cursor": null, "has_more": false}}`)
		page, err := c.Placement.ListTests(ctx, &PlacementTestListParams{
			ListOptions: ListOptions{Limit: 5, Cursor: "abc"}, CampaignID: "camp_1",
		})
		if err != nil {
			t.Fatal(err)
		}
		if got.method != "GET" || got.path != "/v1/placement/tests" || got.rawQuery != "campaign_id=camp_1&cursor=abc&limit=5" {
			t.Errorf("request = %s %s ?%s", got.method, got.path, got.rawQuery)
		}
		if len(page.Data) != 1 {
			t.Fatalf("data = %d", len(page.Data))
		}
		tst := page.Data[0]
		if tst.CampaignID == nil || tst.SequenceID != nil || tst.CreatedBy != nil || !tst.OpenTracking || tst.Status != PlacementStatusRunning {
			t.Errorf("test = %+v", tst)
		}
		if tst.Summary.Inbox != 4 || tst.Summary.Archived != 0 || tst.Summary.InboxRate == nil || tst.Summary.PrimaryMetric == nil ||
			tst.Summary.PrimaryMetric.Interval == nil || tst.Summary.NonSpamMetric != nil {
			t.Errorf("summary = %+v", tst.Summary)
		}
		if len(tst.Families) != 1 || tst.Families[0].Counts.TabsRate != nil {
			t.Errorf("families = %+v", tst.Families)
		}
	})

	t.Run("get test", func(t *testing.T) {
		var got recordedRequest
		c := recordingFixtureClient(t, &got, `{"data": {
			"id": "11111111-1111-1111-1111-111111111111", "sender_email": "sam@acme.com", "subject": "Hi",
			"body_plain": "Hello", "origin": "monitor", "panel": "workspace", "status": "completed", "pace": "quick",
			"open_tracking": false, "link_tracking": false, "credits_charged": 0, "credits_refunded": 0,
			"created_at": "2026-09-07T09:00:00Z", "finished_at": "2026-09-07T09:20:00Z",
			"summary": {"total": 1}, "families": [],
			"results": [{
				"seed": "j***@gmail.com", "family": "future_host", "family_label": "Other provider",
				"folder": "quarantine", "scheduled_at": null, "sent_at": "2026-09-07T09:01:00Z", "detected_at": null,
				"first_folder": "inbox", "observed_at": "2026-09-07T09:05:00Z", "late_observation": true,
				"evidence": {"version": "received-v1", "source": "imap", "trust": "unverified_headers",
					"observed_at": "2026-09-07T09:05:00Z", "spf": "unknown", "dkim": "unknown", "dmarc": "unknown",
					"alignment": "unknown", "tls": "unknown", "authentication_results_present": false,
					"one_click_headers_present": true, "one_click_signing_claim": false, "one_click_compliance": "unknown"}
			}],
			"content": {"score": 82, "issues": [{"severity": "warn", "code": "trigger_terms", "message": "2 spam-trigger terms",
				"field": "subject", "spans": [{"field": "subject", "text": "FREE", "line": 1, "excerpt": "FREE offer"}], "suggestion": "Drop them."}]},
			"compare": `+placementTestJSON+`
		}}`)
		d, _, err := c.Placement.GetTest(ctx, "11111111-1111-1111-1111-111111111111")
		if err != nil {
			t.Fatal(err)
		}
		if got.method != "GET" || got.path != "/v1/placement/tests/11111111-1111-1111-1111-111111111111" {
			t.Errorf("request = %s %s", got.method, got.path)
		}
		if d.BodyPlain != "Hello" || d.Origin != PlacementOriginMonitor || d.Content.Score != 82 {
			t.Errorf("detail = %+v", d)
		}
		r := d.Results[0]
		// An unknown folder and family must decode: both are plain strings.
		if r.Folder != "quarantine" || r.Family != "future_host" || r.FirstFolder == nil || !r.LateObservation || r.ScheduledAt != nil {
			t.Errorf("result = %+v", r)
		}
		if r.Evidence == nil || r.Evidence.Trust != "unverified_headers" || !r.Evidence.OneClickHeadersPresent || r.Evidence.DKIMVerification != nil {
			t.Errorf("evidence = %+v", r.Evidence)
		}
		if len(d.Content.Issues) != 1 || d.Content.Issues[0].Spans[0].Text != "FREE" || d.Content.Issues[0].Field != "subject" {
			t.Errorf("issues = %+v", d.Content.Issues)
		}
		if d.Compare == nil || d.Compare.SenderEmail != "sam@acme.com" {
			t.Errorf("compare = %+v", d.Compare)
		}
	})

	t.Run("create test", func(t *testing.T) {
		var got recordedRequest
		c := recordingFixtureClient(t, &got, `{"data": [`+placementTestJSON+`]}`)
		tests, _, err := c.Placement.CreateTest(ctx, &PlacementTestCreateParams{
			SenderAccountID: "em_1", CampaignID: "camp_1", Tracking: PlacementTrackingCompare,
			Panel: PlacementPanelWorkspace, SeedIDs: []string{"s1"}, Pace: PlacementPaceQuick, MaxCredits: 6,
		})
		if err != nil {
			t.Fatal(err)
		}
		if got.method != "POST" || got.path != "/v1/placement/tests" {
			t.Errorf("request = %s %s", got.method, got.path)
		}
		want := `{"sender_account_id":"em_1","campaign_id":"camp_1","tracking":"compare","panel":"workspace","seed_ids":["s1"],"pace":"quick","max_credits":6}`
		if strings.TrimSpace(got.body) != want {
			t.Errorf("body = %s", got.body)
		}
		if len(tests) != 1 || tests[0].CreditsCharged != 3 {
			t.Errorf("tests = %+v", tests)
		}
	})

	t.Run("cancel test", func(t *testing.T) {
		var got recordedRequest
		c := recordingFixtureClient(t, &got, `{"data": `+placementTestJSON+`}`)
		tst, _, err := c.Placement.CancelTest(ctx, "t1")
		if err != nil {
			t.Fatal(err)
		}
		if got.method != "POST" || got.path != "/v1/placement/tests/t1/cancel" || got.body != "" {
			t.Errorf("request = %s %s %q", got.method, got.path, got.body)
		}
		if tst.ID == "" {
			t.Error("empty test")
		}
	})

	t.Run("batches", func(t *testing.T) {
		const batchJSON = `{
			"id": "b1", "created_by": null, "campaign_id": "c1", "sequence_id": null, "contact_id": null,
			"subject": "Hi", "tracking": "campaign", "panel": "instance", "pace": "spaced",
			"families": [], "seed_ids": [], "on_unavailable": "defer",
			"selection": {"sender_account_ids": 0, "sender_scope": {"type": "campaign", "campaign_id": "c1", "untested_days": 7},
				"sample": {"mode": "percent", "percent": 50, "stratify": "domain"}, "matched": 40},
			"sender_count": 20, "max_credits": 0, "credits_spent": 0, "status": "completed_with_warnings",
			"retry_until": "2026-09-08T09:00:00Z", "created_at": "2026-09-07T09:00:00Z",
			"started_at": "2026-09-07T09:01:00Z", "finished_at": null,
			"progress": {"total": 20, "queued": 0, "deferred": 2, "running": 0, "completed": 17, "skipped": 1, "failed": 0},
			"summary": {"total": 100, "inbox": 80, "delivered": 100}
		}`
		var got recordedRequest
		c := recordingFixtureClient(t, &got, `{"data": [`+batchJSON+`], "pagination": {"total": 1, "next_cursor": "x", "has_more": true}}`)
		page, err := c.Placement.ListBatches(ctx, &ListOptions{Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if got.method != "GET" || got.path != "/v1/placement/batches" || got.rawQuery != "limit=10" {
			t.Errorf("request = %s %s ?%s", got.method, got.path, got.rawQuery)
		}
		b := page.Data[0]
		if b.Status != PlacementBatchCompletedWithWarnings || b.Selection.Scope == nil || b.Selection.Scope.UntestedDays != 7 ||
			b.Selection.Sample.Mode != PlacementSamplePercent || b.Progress.Deferred != 2 || b.FinishedAt != nil || b.StartedAt == nil {
			t.Errorf("batch = %+v", b)
		}
		if !page.HasMore() {
			t.Error("HasMore = false")
		}

		c = recordingFixtureClient(t, &got, `{"data": {"id": "b1", "status": "running", "selection": {"sample": {"mode": "all"}, "matched": 1},
			"retry_until": "2026-09-08T09:00:00Z", "created_at": "2026-09-07T09:00:00Z", "progress": {}, "summary": {},
			"untracked": {"total": 5, "inbox": 5},
			"domains": [{"key": "acme.com", "label": "acme.com", "senders": 3, "tested": 2, "counts": {"total": 4, "inbox": 4}}],
			"providers": [{"key": "google_workspace", "label": "Google Workspace", "senders": 3, "tested": 2, "counts": {"total": 4}}],
			"recipients": [{"family": "gmail", "label": "Gmail", "counts": {"total": 2}}],
			"matrix": [{"domain": "acme.com", "recipients": [{"family": "gmail", "label": "Gmail", "counts": {"total": 2}}]}],
			"content": {"score": 90, "issues": []}
		}}`)
		d, _, err := c.Placement.GetBatch(ctx, "b1")
		if err != nil {
			t.Fatal(err)
		}
		if got.path != "/v1/placement/batches/b1" || d.Untracked == nil || d.Untracked.Inbox != 5 ||
			len(d.Domains) != 1 || d.Domains[0].Counts.Inbox != 4 || len(d.Matrix) != 1 || d.Matrix[0].Recipients[0].Family != "gmail" || d.Content.Score != 90 {
			t.Errorf("detail = %+v (path %s)", d, got.path)
		}

		c = recordingFixtureClient(t, &got, `{"data": [{
			"id": "s1", "batch_id": "b1", "email_account_id": null, "sender_email": "a@acme.com", "sender_domain": "acme.com",
			"sender_family": "google_workspace", "status": "deferred", "reason": "no_headroom", "attempts": 2,
			"next_attempt_at": "2026-09-07T10:00:00Z", "started_at": null, "finished_at": null,
			"sender_family_label": "Google Workspace", "summary": {"total": 12, "inbox": 9}, "test_ids": ["t1", "t2"]
		}], "pagination": {"total": 1, "next_cursor": null, "has_more": false}}`)
		senders, err := c.Placement.ListBatchSenders(ctx, "b1", &PlacementBatchSenderListParams{
			ListOptions: ListOptions{Limit: 50}, Status: PlacementSenderDeferred, Search: "acme", Sort: PlacementSortBest,
		})
		if err != nil {
			t.Fatal(err)
		}
		if got.method != "GET" || got.path != "/v1/placement/batches/b1/senders" || got.rawQuery != "limit=50&q=acme&sort=best&status=deferred" {
			t.Errorf("request = %s %s ?%s", got.method, got.path, got.rawQuery)
		}
		s := senders.Data[0]
		if s.EmailAccountID != nil || s.Status != PlacementSenderDeferred || len(s.TestIDs) != 2 || s.Summary.Inbox != 9 {
			t.Errorf("sender = %+v", s)
		}

		c = recordingFixtureClient(t, &got, `{"data": `+batchJSON+`}`)
		if _, _, err := c.Placement.CancelBatch(ctx, "b1"); err != nil {
			t.Fatal(err)
		}
		if got.method != "POST" || got.path != "/v1/placement/batches/b1/cancel" {
			t.Errorf("request = %s %s", got.method, got.path)
		}
	})

	t.Run("preview and create batch", func(t *testing.T) {
		var got recordedRequest
		c := recordingFixtureClient(t, &got, `{"data": {
			"matched": 40, "selected": 20, "inactive": 3, "domains": 5,
			"providers": [{"key": "google_workspace", "label": "Google Workspace", "senders": 12}],
			"variants": 2, "tests": 40, "seeds_per_test": 12, "max_sends": 480,
			"metered": true, "free_tests": 8, "paid_tests": 32, "credits": 160,
			"usage": {"used": 2, "limit": 10, "credits_per_test": 5, "credit_balance": 400,
				"period_start": "2026-09-01T00:00:00Z", "period_end": "2026-10-01T00:00:00Z"},
			"senders_max": 500, "concurrency": 5
		}}`)
		params := &PlacementBatchCreateParams{
			SenderScope: &PlacementSenderScope{Type: PlacementScopeWorkspace, Providers: []string{"gmail"}, IncludeInactive: true},
			Sample:      &PlacementSample{Mode: PlacementSampleRandom, Count: 20},
			CampaignID:  "c1", Tracking: PlacementTrackingCompare, OnUnavailable: PlacementUnavailableSkip, MaxCredits: 160,
		}
		p, _, err := c.Placement.PreviewBatch(ctx, params)
		if err != nil {
			t.Fatal(err)
		}
		want := `{"sender_scope":{"type":"workspace","providers":["gmail"],"include_inactive":true},"sample":{"mode":"random","count":20},"campaign_id":"c1","tracking":"compare","on_unavailable":"skip","max_credits":160}`
		if got.method != "POST" || got.path != "/v1/placement/batches/preview" || strings.TrimSpace(got.body) != want {
			t.Errorf("request = %s %s %s", got.method, got.path, got.body)
		}
		if p.Selected != 20 || p.Variants != 2 || p.Credits != 160 || p.Usage.CreditBalance == nil || *p.Usage.CreditBalance != 400 || len(p.Providers) != 1 {
			t.Errorf("preview = %+v", p)
		}

		c = recordingFixtureClient(t, &got, `{"data": {"id": "b9", "status": "queued", "selection": {"sample": {"mode": "random", "count": 20}, "matched": 40},
			"retry_until": "2026-09-08T09:00:00Z", "created_at": "2026-09-07T09:00:00Z", "progress": {"total": 20, "queued": 20}, "summary": {}}}`)
		b, _, err := c.Placement.CreateBatch(ctx, params)
		if err != nil {
			t.Fatal(err)
		}
		if got.method != "POST" || got.path != "/v1/placement/batches" || b.Status != PlacementBatchQueued || b.Progress.Queued != 20 {
			t.Errorf("create = %s %s %+v", got.method, got.path, b)
		}
	})

	t.Run("coverage", func(t *testing.T) {
		var got recordedRequest
		c := recordingFixtureClient(t, &got, `{"data": {"mailboxes": 50, "tested_7d": 20, "tested_30d": 35, "never_tested": 15}}`)
		cov, _, err := c.Placement.Coverage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got.path != "/v1/placement/coverage" || cov.Mailboxes != 50 || cov.NeverTested != 15 {
			t.Errorf("coverage = %s %+v", got.path, cov)
		}
	})

	t.Run("seeds", func(t *testing.T) {
		var got recordedRequest
		c := recordingFixtureClient(t, &got, `{"data": [
			{"email_account_id": "e1", "email": "seed@acme.com", "family": "gmail", "label": "Gmail", "status": "active", "seed": true},
			{"email_account_id": "e2", "email": "sam@acme.com", "family": "outlook", "label": "Outlook", "status": "error", "seed": false, "blocker": "Not connected."}
		]}`)
		seeds, _, err := c.Placement.ListSeeds(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got.method != "GET" || got.path != "/v1/placement/seeds" || len(seeds) != 2 || !seeds[0].Seed || seeds[1].Blocker == "" {
			t.Errorf("seeds = %s %s %+v", got.method, got.path, seeds)
		}

		c = recordingFixtureClient(t, &got, `{"data": {"email_account_id": "e1", "email": "seed@acme.com", "family": "gmail", "label": "Gmail", "status": "active", "seed": false}}`)
		s, _, err := c.Placement.SetSeed(ctx, "e1", false)
		if err != nil {
			t.Fatal(err)
		}
		if got.method != "PUT" || got.path != "/v1/placement/seeds/e1" || strings.TrimSpace(got.body) != `{"seed":false}` || s.Seed {
			t.Errorf("set seed = %s %s %s %+v", got.method, got.path, got.body, s)
		}
	})
}

func TestCampaignPlacementMonitor(t *testing.T) {
	ctx := context.Background()
	var got recordedRequest

	// A campaign with no monitor answers {"data": null}.
	c := recordingFixtureClient(t, &got, `{"data": null}`)
	m, _, err := c.Campaigns.PlacementMonitor(ctx, "camp_1")
	if err != nil {
		t.Fatal(err)
	}
	if got.method != "GET" || got.path != "/v1/campaigns/camp_1/placement-monitor" || m != nil {
		t.Errorf("get = %s %s %+v", got.method, got.path, m)
	}

	c = recordingFixtureClient(t, &got, `{"data": {
		"id": "m1", "campaign_id": "camp_1", "created_by": null, "enabled": true, "interval_days": 7,
		"panel": "instance", "alert_below": 80, "pause_on_alert": false,
		"next_run_at": "2026-09-14T09:00:00Z", "last_run_at": null, "last_test_id": null, "last_alert_at": null,
		"created_at": "2026-09-07T09:00:00Z", "updated_at": "2026-09-07T09:00:00Z"
	}}`)
	m, _, err = c.Campaigns.SetPlacementMonitor(ctx, "camp_1", &PlacementMonitorParams{
		Enabled: Bool(true), IntervalDays: Int(7), Panel: String(PlacementPanelInstance), AlertBelow: Int(80), PauseOnAlert: Bool(false),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"enabled":true,"interval_days":7,"panel":"instance","alert_below":80,"pause_on_alert":false}`
	if got.method != "PUT" || got.path != "/v1/campaigns/camp_1/placement-monitor" || strings.TrimSpace(got.body) != want {
		t.Errorf("put = %s %s %s", got.method, got.path, got.body)
	}
	if m == nil || m.IntervalDays != 7 || m.LastRunAt != nil || m.PauseOnAlert {
		t.Errorf("monitor = %+v", m)
	}

	c = recordingFixtureClient(t, &got, ``)
	if _, err := c.Campaigns.DeletePlacementMonitor(ctx, "camp_1"); err != nil {
		t.Fatal(err)
	}
	if got.method != "DELETE" || got.path != "/v1/campaigns/camp_1/placement-monitor" {
		t.Errorf("delete = %s %s", got.method, got.path)
	}
}
