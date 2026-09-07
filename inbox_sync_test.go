package warmbly

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// inboxFixtureClient answers every request with one fixed body, for the decode
// assertions below that routingClient's generic envelope cannot satisfy.
func inboxFixtureClient(t *testing.T, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

// syncCase is one method/path (and optionally query/body) assertion.
type syncCase struct {
	name       string
	call       func() error
	wantMethod string
	wantPath   string
	wantQuery  string
	wantBody   []string
}

func runSyncCases(t *testing.T, got *recordedRequest, cases []syncCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			*got = recordedRequest{}
			if err := tc.call(); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if got.method != tc.wantMethod || got.path != tc.wantPath {
				t.Errorf("%s = %s %s, want %s %s", tc.name, got.method, got.path, tc.wantMethod, tc.wantPath)
			}
			if tc.wantQuery != "" && got.rawQuery != tc.wantQuery {
				t.Errorf("%s query = %q, want %q", tc.name, got.rawQuery, tc.wantQuery)
			}
			for _, want := range tc.wantBody {
				if !strings.Contains(got.body, want) {
					t.Errorf("%s body = %s, want it to contain %s", tc.name, got.body, want)
				}
			}
		})
	}
}

// TestUniboxSyncRouting covers every unibox route, including the folder scope
// added to the list and the folder sweep on PATCH /unibox/seen.
func TestUniboxSyncRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()
	until := time.Date(2026, 9, 8, 9, 0, 0, 0, time.UTC)

	runSyncCases(t, &got, []syncCase{
		{"List folder", func() error {
			_, e := c.Unibox.List(ctx, &UniboxListParams{Folder: FolderSpam, Unseen: Bool(true)})
			return e
		}, "GET", "/v1/unibox", "folder=spam&unseen=true", nil},
		{"List filters", func() error {
			_, e := c.Unibox.List(ctx, &UniboxListParams{
				ListOptions: ListOptions{Limit: 25},
				Direction:   DirectionReceived,
				EmailIDs:    []string{"em_1", "em_2"},
				CategoryIDs: []string{"cat_1"},
				SnoozedAny:  true,
			})
			return e
		}, "GET", "/v1/unibox", "category_ids=cat_1&direction=received&email_ids=em_1%2Cem_2&limit=25&snoozed=any", nil},
		{"UnseenCount", func() error {
			_, _, e := c.Unibox.UnseenCount(ctx, "em_1")
			return e
		}, "GET", "/v1/unibox/count", "email_id=em_1", nil},
		{"Overview", func() error {
			_, _, e := c.Unibox.Overview(ctx)
			return e
		}, "GET", "/v1/unibox/overview", "", nil},
		{"Thread", func() error {
			_, e := c.Unibox.Thread(ctx, "th_1", "em_1", &ListOptions{Limit: 10})
			return e
		}, "GET", "/v1/unibox/thread", "email_id=em_1&limit=10&thread_id=th_1", nil},
		{"Get", func() error {
			_, _, e := c.Unibox.Get(ctx, "msg_1")
			return e
		}, "GET", "/v1/unibox/msg_1", "", nil},
		{"ThreadLabels", func() error {
			_, _, e := c.Unibox.ThreadLabels(ctx, "th_1")
			return e
		}, "GET", "/v1/unibox/thread/labels", "thread_id=th_1", nil},
		{"SetThreadLabels", func() error {
			_, _, e := c.Unibox.SetThreadLabels(ctx, "th_1", []string{"cat_1"})
			return e
		}, "PUT", "/v1/unibox/thread/labels", "", []string{`"thread_id":"th_1"`, `"category_ids":["cat_1"]`}},
		{"MarkSeen", func() error {
			_, e := c.Unibox.MarkSeen(ctx, []string{"msg_1", "msg_2"}, true)
			return e
		}, "PATCH", "/v1/unibox/seen", "", []string{`"email_ids":["msg_1","msg_2"]`, `"seen":true`}},
		{"MarkFolderSeen", func() error {
			_, e := c.Unibox.MarkFolderSeen(ctx, FolderInbox, true)
			return e
		}, "PATCH", "/v1/unibox/seen", "", []string{`"folder":"inbox"`, `"seen":true`}},
		{"Reply", func() error {
			_, _, e := c.Unibox.Reply(ctx, &UniboxReplyParams{
				EmailAccountID: "em_1", To: []string{"jane@example.com"}, ThreadID: "th_1",
			})
			return e
		}, "POST", "/v1/unibox/reply", "", []string{`"thread_id":"th_1"`}},
		{"Compose", func() error {
			_, _, e := c.Unibox.Compose(ctx, &UniboxComposeParams{To: []string{"jane@example.com"}, Subject: "Hi"})
			return e
		}, "POST", "/v1/unibox/compose", "", []string{`"subject":"Hi"`}},
		{"ComposeCandidates", func() error {
			_, _, e := c.Unibox.ComposeCandidates(ctx, "jane@example.com")
			return e
		}, "GET", "/v1/unibox/compose/candidates", "to=jane%40example.com", nil},
		{"DraftCompose", func() error {
			_, _, e := c.Unibox.DraftCompose(ctx, "jane@example.com", "Intro", "keep it short")
			return e
		}, "POST", "/v1/unibox/compose/draft", "", []string{`"instruction":"keep it short"`}},
		{"DraftReply", func() error {
			_, _, e := c.Unibox.DraftReply(ctx, "th_1", "say yes")
			return e
		}, "POST", "/v1/unibox/reply/draft", "", []string{`"thread_id":"th_1"`}},
		{"Drafts", func() error {
			_, _, e := c.Unibox.Drafts(ctx)
			return e
		}, "GET", "/v1/unibox/drafts", "", nil},
		{"SaveDraft", func() error {
			_, e := c.Unibox.SaveDraft(ctx, "d_1", &ComposeDraftParams{Subject: "Draft"})
			return e
		}, "PUT", "/v1/unibox/drafts/d_1", "", []string{`"subject":"Draft"`}},
		{"DeleteDraft", func() error {
			_, e := c.Unibox.DeleteDraft(ctx, "d_1")
			return e
		}, "DELETE", "/v1/unibox/drafts/d_1", "", nil},
		{"AgentDrafts", func() error {
			_, _, e := c.Unibox.AgentDrafts(ctx)
			return e
		}, "GET", "/v1/unibox/agent-drafts", "", nil},
		{"ApproveAgentDraft", func() error {
			_, _, e := c.Unibox.ApproveAgentDraft(ctx, "ad_1", "edited body")
			return e
		}, "POST", "/v1/unibox/agent-drafts/ad_1/approve", "", []string{`"body":"edited body"`}},
		{"DiscardAgentDraft", func() error {
			_, e := c.Unibox.DiscardAgentDraft(ctx, "ad_1")
			return e
		}, "POST", "/v1/unibox/agent-drafts/ad_1/discard", "", nil},
		{"Snoozes", func() error {
			_, _, e := c.Unibox.Snoozes(ctx)
			return e
		}, "GET", "/v1/unibox/snoozes", "", nil},
		{"Snooze", func() error {
			_, _, e := c.Unibox.Snooze(ctx, "th_1", until)
			return e
		}, "POST", "/v1/unibox/snooze", "", []string{`"thread_id":"th_1"`, `"snoozed_until":"2026-09-08T09:00:00Z"`}},
		{"Unsnooze", func() error {
			_, e := c.Unibox.Unsnooze(ctx, "th_1")
			return e
		}, "DELETE", "/v1/unibox/snooze", "thread_id=th_1", nil},
		{"Scheduled", func() error {
			_, _, e := c.Unibox.Scheduled(ctx, "th_1")
			return e
		}, "GET", "/v1/unibox/scheduled", "thread_id=th_1", nil},
		{"CancelScheduled", func() error {
			_, e := c.Unibox.CancelScheduled(ctx, "task_1")
			return e
		}, "DELETE", "/v1/unibox/scheduled/task_1", "", nil},
	})
}

// TestUniboxOverviewDecode asserts the folder rail added to the overview.
func TestUniboxOverviewDecode(t *testing.T) {
	c := inboxFixtureClient(t, `{
		"total": 812, "unread": 34, "today": 9, "week": 61,
		"snoozed": 3, "awaiting_reply": 12, "awaiting_agent_draft": 2,
		"scheduled_pending": 4, "scheduled_pending_max": 50,
		"folders": [
			{"folder": "inbox", "unread": 30, "total": 500},
			{"folder": "sent", "unread": 0, "total": 250},
			{"folder": "drafts", "unread": 0, "total": 2},
			{"folder": "archive", "unread": 0, "total": 40},
			{"folder": "spam", "unread": 4, "total": 19},
			{"folder": "trash", "unread": 0, "total": 1}
		],
		"mailboxes": [{"id": "em_1", "email": "sam@example.com", "name": "Sam", "unread": 30, "total": 700}],
		"tags": [{"id": "t_1", "title": "VIP", "color": "#8b5cf6", "unread": 1, "total": 8}],
		"categories": [{"id": "c_1", "title": "Interested", "color": "#0ea5e9", "unread": 2, "total": 15}],
		"generated_at": "2026-09-07T10:00:00Z",
		"window_today_start": "2026-09-07T00:00:00Z",
		"window_week_start": "2026-09-01T00:00:00Z"
	}`)

	ov, _, err := c.Unibox.Overview(context.Background())
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if len(ov.Folders) != 6 {
		t.Fatalf("Folders = %d, want all six canonical folders", len(ov.Folders))
	}
	if ov.Folders[0].Folder != FolderInbox || ov.Folders[0].Unread != 30 || ov.Folders[0].Total != 500 {
		t.Errorf("Folders[0] = %+v", ov.Folders[0])
	}
	if ov.Folders[4].Folder != FolderSpam || ov.Folders[4].Unread != 4 {
		t.Errorf("Folders[4] = %+v", ov.Folders[4])
	}
	if ov.AwaitingAgentDraft != 2 || ov.ScheduledPendingMax != 50 {
		t.Errorf("agent draft/scheduled counters = %d/%d", ov.AwaitingAgentDraft, ov.ScheduledPendingMax)
	}
	if ov.WindowWeekStart.IsZero() {
		t.Error("WindowWeekStart did not decode")
	}
}

// TestUniboxMessageDecode covers the truncated-body flag and the folder a
// message is filed in.
func TestUniboxMessageDecode(t *testing.T) {
	c := inboxFixtureClient(t, `{
		"id": "msg_1", "email_id": "em_1", "thread_id": "th_1",
		"folder": "archive", "message_id": "<a@example.com>",
		"from": ["jane@example.com"], "to": ["sam@example.com"],
		"subject": "Re: pricing", "date": "2026-09-06T08:30:00Z",
		"snippet": "Sounds good", "seen": true,
		"body_plain": "Sounds good", "body_html": "<p>Sounds good</p>",
		"body_truncated": true
	}`)

	msg, _, err := c.Unibox.Get(context.Background(), "msg_1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !msg.BodyTruncated {
		t.Error("BodyTruncated = false, want true")
	}
	if msg.Folder != FolderArchive {
		t.Errorf("Folder = %q, want %q", msg.Folder, FolderArchive)
	}
	// The single-message read spells the envelope "from"/"to"/"date".
	if len(msg.FromAddr) != 1 || msg.FromAddr[0] != "jane@example.com" {
		t.Errorf("FromAddr = %v", msg.FromAddr)
	}
	if len(msg.ToAddr) != 1 || msg.ToAddr[0] != "sam@example.com" {
		t.Errorf("ToAddr = %v", msg.ToAddr)
	}
	if msg.SentDate.IsZero() {
		t.Error("SentDate did not decode from \"date\"")
	}
}

// TestAnalyticsSyncRouting covers every analytics route.
func TestAnalyticsSyncRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)

	runSyncCases(t, &got, []syncCase{
		{"Dashboard", func() error {
			_, _, e := c.Analytics.Dashboard(ctx, "30d")
			return e
		}, "GET", "/v1/analytics/dashboard", "period=30d", nil},
		{"Campaign", func() error {
			_, _, e := c.Analytics.Campaign(ctx, "camp_1")
			return e
		}, "GET", "/v1/analytics/campaigns/camp_1", "", nil},
		{"CampaignDaily", func() error {
			_, _, e := c.Analytics.CampaignDaily(ctx, "camp_1", from, to)
			return e
		}, "GET", "/v1/analytics/campaigns/camp_1/daily", "from=2026-08-01&to=2026-08-31", nil},
		{"CampaignHourly", func() error {
			_, _, e := c.Analytics.CampaignHourly(ctx, "camp_1", from)
			return e
		}, "GET", "/v1/analytics/campaigns/camp_1/hourly", "date=2026-08-01", nil},
		{"CompareCampaigns", func() error {
			_, _, e := c.Analytics.CompareCampaigns(ctx, []string{"camp_1", "camp_2"}, from, to)
			return e
		}, "GET", "/v1/analytics/campaigns/compare", "from=2026-08-01&ids=camp_1%2Ccamp_2&to=2026-08-31", nil},
		{"Warmup", func() error {
			_, _, e := c.Analytics.Warmup(ctx, "em_1", from, to)
			return e
		}, "GET", "/v1/analytics/warmup", "email_id=em_1&from=2026-08-01&to=2026-08-31", nil},
		{"Deliverability", func() error {
			_, _, e := c.Analytics.Deliverability(ctx, from, to)
			return e
		}, "GET", "/v1/analytics/deliverability", "from=2026-08-01T00%3A00%3A00Z&to=2026-08-31T00%3A00%3A00Z", nil},
		{"Accounts", func() error {
			_, _, e := c.Analytics.Accounts(ctx)
			return e
		}, "GET", "/v1/analytics/accounts", "", nil},
		{"Account", func() error {
			_, _, e := c.Analytics.Account(ctx, "em_1")
			return e
		}, "GET", "/v1/analytics/accounts/em_1", "", nil},
		{"Usage", func() error {
			_, _, e := c.Analytics.Usage(ctx, "month")
			return e
		}, "GET", "/v1/analytics/usage", "period=month", nil},
	})
}

// TestCampaignAnalyticsEngagementDecode covers the engagement breakdown and
// the machine-click counter added to a campaign's analytics.
func TestCampaignAnalyticsEngagementDecode(t *testing.T) {
	c := inboxFixtureClient(t, `{
		"campaign_id": "camp_1", "name": "Q3 outbound", "status": "active",
		"date_range": {"from": "2026-08-01T00:00:00Z", "to": "2026-08-31T00:00:00Z"},
		"summary": {
			"total_contacts": 1200, "emails_sent": 900, "emails_pending": 300,
			"unique_opens": 410, "machine_opens": 88,
			"unique_clicks": 96, "machine_clicks": 14,
			"replies": 37, "bounces": 6, "unsubscribes": 2,
			"open_rate": 35.8, "click_rate": 10.7, "reply_rate": 4.1, "bounce_rate": 0.7
		},
		"steps": [],
		"engagement": {
			"countries": [{"key": "US", "opens": 220, "clicks": 51}, {"key": "", "opens": 12, "clicks": 1}],
			"clients": [{"key": "Gmail", "opens": 180, "clicks": 44}],
			"devices": [{"key": "desktop", "opens": 260, "clicks": 70}, {"key": "mobile", "opens": 150, "clicks": 26}]
		}
	}`)

	a, _, err := c.Analytics.Campaign(context.Background(), "camp_1")
	if err != nil {
		t.Fatalf("Campaign: %v", err)
	}
	if a.Summary.MachineClicks != 14 {
		t.Errorf("MachineClicks = %d, want 14", a.Summary.MachineClicks)
	}
	if a.Engagement == nil {
		t.Fatal("Engagement is nil")
	}
	if len(a.Engagement.Countries) != 2 || a.Engagement.Countries[0].Key != "US" || a.Engagement.Countries[0].Clicks != 51 {
		t.Errorf("Countries = %+v", a.Engagement.Countries)
	}
	// The empty key is the "unknown" bucket, not a missing field.
	if a.Engagement.Countries[1].Key != "" || a.Engagement.Countries[1].Opens != 12 {
		t.Errorf("unknown bucket = %+v", a.Engagement.Countries[1])
	}
	if len(a.Engagement.Clients) != 1 || a.Engagement.Clients[0].Key != "Gmail" {
		t.Errorf("Clients = %+v", a.Engagement.Clients)
	}
	if len(a.Engagement.Devices) != 2 || a.Engagement.Devices[1].Key != "mobile" {
		t.Errorf("Devices = %+v", a.Engagement.Devices)
	}
}

// TestAccountStatusColdRampDecode covers the cold-ramp ceiling, the lifecycle
// hold and the warmup ramp hold on a mailbox's operational status.
func TestAccountStatusColdRampDecode(t *testing.T) {
	c := inboxFixtureClient(t, `{
		"id": "em_1", "email": "sam@example.com", "name": "Sam",
		"provider": "google", "status": "active",
		"health": {"score": 82, "band": "watch"},
		"errors": [{
			"id": "err_1", "error_code": "auth_expired", "severity": "high",
			"title": "Reconnect needed", "message": "The token expired",
			"action_required": "Reconnect the mailbox",
			"created_at": "2026-09-01T10:00:00Z"
		}],
		"daily_usage": {"date": "2026-09-07", "campaign_sent": 12, "campaign_limit": 20},
		"in_campaign": true,
		"send_lifecycle": {"state": "resting", "since": "2026-09-05T08:00:00Z", "reason": "warmup health throttled"},
		"cold_ramp": {"ceiling": 20, "mailbox_cap": 60, "days_to_full_cap": 4, "held": true},
		"warmup_status": {
			"enabled": true, "paused": false, "started_at": "2026-08-01T00:00:00Z",
			"current_volume": 8, "target_volume": 12, "max_volume": 40,
			"reply_rate": 30, "days_active": 37,
			"ramp_hold": {"placements": 3, "sends": 120, "volume_cut": true, "resumes_at": "2026-09-09T00:00:00Z"}
		},
		"warmup_health": {"state": "throttled", "score": 61.5, "reason": "spam placements rising", "spam_score": 22}
	}`)

	acc, _, err := c.Analytics.Account(context.Background(), "em_1")
	if err != nil {
		t.Fatalf("Account: %v", err)
	}
	if acc.ColdRamp == nil {
		t.Fatal("ColdRamp is nil")
	}
	if acc.ColdRamp.Ceiling != 20 || acc.ColdRamp.MailboxCap != 60 || acc.ColdRamp.DaysToFullCap != 4 || !acc.ColdRamp.Held {
		t.Errorf("ColdRamp = %+v", *acc.ColdRamp)
	}
	if acc.SendLifecycle == nil || acc.SendLifecycle.State != SendLifecycleResting {
		t.Fatalf("SendLifecycle = %+v", acc.SendLifecycle)
	}
	if acc.SendLifecycle.SendsCold() {
		t.Error("SendsCold() = true for a resting mailbox")
	}
	if acc.WarmupStatus == nil || acc.WarmupStatus.RampHold == nil {
		t.Fatal("WarmupStatus.RampHold is nil")
	}
	hold := acc.WarmupStatus.RampHold
	if hold.Placements != 3 || hold.Sends != 120 || !hold.VolumeCut || hold.ResumesAt.IsZero() {
		t.Errorf("RampHold = %+v", *hold)
	}
	if acc.WarmupHealth == nil || acc.WarmupHealth.State != BandThrottled {
		t.Errorf("WarmupHealth = %+v", acc.WarmupHealth)
	}
	if len(acc.Errors) != 1 || acc.Errors[0].ActionRequired == nil || *acc.Errors[0].ActionRequired != "Reconnect the mailbox" {
		t.Errorf("Errors = %+v", acc.Errors)
	}
}

// TestDeliverabilityDashboardDecode covers the score and the per-provider and
// warmup placement breakdowns.
func TestDeliverabilityDashboardDecode(t *testing.T) {
	c := inboxFixtureClient(t, `{
		"from": "2026-08-01T00:00:00Z", "to": "2026-08-31T00:00:00Z",
		"events_total": 900, "bounce_count": 8, "complaint_count": 1,
		"emails_sent": 4000, "bounce_rate": 0.2, "complaint_rate": 0.02,
		"placement_samples": 40, "spam_placement_rate": 12.5, "inbox_placement_rate": 82.5,
		"band": "watch", "score": 74,
		"by_provider": [{
			"provider": "google", "samples": 20, "inbox": 17, "promotions": 1,
			"spam": 2, "other": 0, "inbox_rate": 85, "spam_rate": 10
		}],
		"warmup_placement": [{
			"provider": "microsoft", "domain": "outlook.com",
			"delivered": 140, "spam": 12, "inbox_rate": 92.1, "spam_rate": 7.9
		}]
	}`)

	d, _, err := c.Analytics.Deliverability(context.Background(), time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("Deliverability: %v", err)
	}
	if d.Score != 74 || d.Band != BandWatch {
		t.Errorf("score/band = %d/%q", d.Score, d.Band)
	}
	if len(d.ByProvider) != 1 || d.ByProvider[0].Provider != "google" || d.ByProvider[0].Spam != 2 {
		t.Errorf("ByProvider = %+v", d.ByProvider)
	}
	if len(d.WarmupPlacement) != 1 || d.WarmupPlacement[0].Domain != "outlook.com" || d.WarmupPlacement[0].Delivered != 140 {
		t.Errorf("WarmupPlacement = %+v", d.WarmupPlacement)
	}
}

// TestDashboardMachineClicksDecode covers the machine-click counter added to
// the dashboard totals.
func TestDashboardMachineClicksDecode(t *testing.T) {
	c := inboxFixtureClient(t, `{
		"period": "30d",
		"overall_stats": {
			"total_emails_sent": 4000, "total_opens": 1800, "machine_opens": 400,
			"total_clicks": 320, "machine_clicks": 45, "total_replies": 120,
			"total_bounces": 8, "open_rate": 35, "click_rate": 8,
			"reply_rate": 3, "bounce_rate": 0.2,
			"active_campaigns": 3, "active_accounts": 6
		},
		"recent_activity": [{
			"type": "clicked", "campaign_id": "camp_1", "campaign_name": "Q3",
			"contact_email": "jane@example.com", "contact_id": "ct_1",
			"timestamp": "2026-09-07T09:00:00Z", "link": "https://example.com/pricing"
		}],
		"top_campaigns": [], "account_health": {}, "daily_trend": []
	}`)

	d, _, err := c.Analytics.Dashboard(context.Background(), "30d")
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if d.OverallStats.MachineClicks != 45 {
		t.Errorf("MachineClicks = %d, want 45", d.OverallStats.MachineClicks)
	}
	if len(d.RecentActivity) != 1 || d.RecentActivity[0].Link != "https://example.com/pricing" {
		t.Errorf("RecentActivity = %+v", d.RecentActivity)
	}
}

// TestAutomationSyncRouting covers the automation routes, including the dry
// run's optional skip list.
func TestAutomationSyncRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()

	runSyncCases(t, &got, []syncCase{
		{"List", func() error {
			_, _, e := c.Automations.List(ctx)
			return e
		}, "GET", "/v1/automations", "", nil},
		{"Create", func() error {
			cfg, err := (&UpsertContactConfig{
				Email:        "{{.data.email}}",
				FirstName:    "{{.data.first_name}}",
				CustomFields: []TemplateField{{Key: "industry", Value: "{{.data.industry}}"}},
				CategoryIDs:  []string{"cat_1"},
				CampaignID:   "camp_1",
				IfExists:     IfExistsSkip,
			}).Config()
			if err != nil {
				return err
			}
			_, _, e := c.Automations.Create(ctx, &AutomationCreateParams{
				Name:         "Lead intake",
				Enabled:      true,
				TriggerEvent: TriggerFormSubmitted,
				Graph: AutomationGraph{
					Nodes: []AutomationNode{
						{ID: "trigger", Type: NodeTrigger},
						{ID: "a1", Type: NodeAction, Action: ActionUpsertContact, Config: cfg},
					},
					Edges: []AutomationEdge{{ID: "e1", Source: "trigger", Target: "a1"}},
				},
			})
			return e
		}, "POST", "/v1/automations", "", []string{
			`"trigger_event":"form.submitted"`,
			`"action":"warmbly.upsert_contact"`,
			`"if_exists":"skip"`,
			`"custom_fields":[{"key":"industry","value":"{{.data.industry}}"}]`,
		}},
		{"Get", func() error {
			_, _, e := c.Automations.Get(ctx, "au_1")
			return e
		}, "GET", "/v1/automations/au_1", "", nil},
		{"Update", func() error {
			_, _, e := c.Automations.Update(ctx, "au_1", &AutomationUpdateParams{
				Name: "Lead intake v2", Enabled: true, TriggerEvent: TriggerContactCreated,
			})
			return e
		}, "PATCH", "/v1/automations/au_1", "", []string{`"trigger_event":"contact.created"`}},
		{"UpdateLayout", func() error {
			_, e := c.Automations.UpdateLayout(ctx, "au_1", []NodePosition{{ID: "a1", X: 10, Y: 20}})
			return e
		}, "PATCH", "/v1/automations/au_1/layout", "", []string{`"positions":[{"id":"a1","x":10,"y":20}]`}},
		{"Delete", func() error {
			_, e := c.Automations.Delete(ctx, "au_1")
			return e
		}, "DELETE", "/v1/automations/au_1", "", nil},
		{"DryRun", func() error {
			_, _, e := c.Automations.DryRun(ctx, "au_1", &AutomationTestParams{
				Data:        map[string]any{"email": "jane@example.com"},
				SkipNodeIDs: []string{"a2"},
			})
			return e
		}, "POST", "/v1/automations/au_1/test", "", []string{`"skip_node_ids":["a2"]`, `"email":"jane@example.com"`}},
		{"Test nil data", func() error {
			_, _, e := c.Automations.Test(ctx, "au_1", nil)
			return e
		}, "POST", "/v1/automations/au_1/test", "", []string{`{}`}},
		{"Runs", func() error {
			_, _, e := c.Automations.Runs(ctx, "au_1", 10)
			return e
		}, "GET", "/v1/automations/au_1/runs", "limit=10", nil},
	})
}

// TestAutomationUpsertContactDecode covers an automation carrying the
// lead-intake action, its inbound URL and a stop node.
func TestAutomationUpsertContactDecode(t *testing.T) {
	c := inboxFixtureClient(t, `{"automation": {
		"id": "au_1", "organization_id": "org_1", "name": "Facebook leads",
		"enabled": true, "trigger_event": "inbound.webhook",
		"inbound_url": "https://api.warmbly.com/api/v1/integrations/inbound/automation/tok_secret",
		"graph": {
			"nodes": [
				{"id": "trigger", "type": "trigger", "x": 0, "y": 0},
				{"id": "c1", "type": "condition", "x": 0, "y": 100,
				 "condition": {"field": "field", "key": "form_name", "operator": "contains", "value": "Demo"}},
				{"id": "a1", "type": "action", "action": "warmbly.upsert_contact", "x": 0, "y": 200,
				 "config": {
					"email": "{{.data.email}}",
					"first_name": "{{.data.first_name}}",
					"custom_fields": [{"key": "industry", "value": "{{.data.industry}}"}],
					"category_ids": ["cat_1"],
					"campaign_id": "camp_1",
					"if_exists": "update"
				 }},
				{"id": "s1", "type": "stop", "x": 0, "y": 300}
			],
			"edges": [
				{"id": "e1", "source": "trigger", "target": "c1"},
				{"id": "e2", "source": "c1", "target": "a1", "when": "true"},
				{"id": "e3", "source": "c1", "target": "s1", "when": "false"},
				{"id": "e4", "source": "a1", "target": "s1", "when": "error"}
			]
		},
		"created_at": "2026-09-01T00:00:00Z", "updated_at": "2026-09-06T00:00:00Z"
	}}`)

	a, _, err := c.Automations.Get(context.Background(), "au_1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if a.TriggerEvent != TriggerInboundWebhook || a.InboundURL == "" {
		t.Errorf("trigger/inbound URL = %q/%q", a.TriggerEvent, a.InboundURL)
	}
	if len(a.Graph.Nodes) != 4 {
		t.Fatalf("Nodes = %d, want 4", len(a.Graph.Nodes))
	}
	if a.Graph.Nodes[3].Type != NodeStop {
		t.Errorf("terminal node type = %q, want %q", a.Graph.Nodes[3].Type, NodeStop)
	}
	cond := a.Graph.Nodes[1].Condition
	if cond == nil || cond.Field != ConditionField || cond.Key != "form_name" || cond.Operator != ConditionOpContains {
		t.Errorf("condition = %+v", cond)
	}
	action := a.Graph.Nodes[2]
	if action.Action != ActionUpsertContact || action.ConnectionID != nil {
		t.Errorf("action = %q, connection = %v (built-ins carry no connection)", action.Action, action.ConnectionID)
	}
	var cfg UpsertContactConfig
	if err := json.Unmarshal(action.Config, &cfg); err != nil {
		t.Fatalf("decode action config: %v", err)
	}
	if cfg.Email != "{{.data.email}}" || cfg.IfExists != IfExistsUpdate || cfg.CampaignID != "camp_1" {
		t.Errorf("config = %+v", cfg)
	}
	if len(cfg.CustomFields) != 1 || cfg.CustomFields[0].Key != "industry" {
		t.Errorf("CustomFields = %+v", cfg.CustomFields)
	}
	if len(cfg.CategoryIDs) != 1 || cfg.CategoryIDs[0] != "cat_1" {
		t.Errorf("CategoryIDs = %v", cfg.CategoryIDs)
	}
	if a.Graph.Edges[1].When != EdgeWhenTrue || a.Graph.Edges[3].When != EdgeWhenError {
		t.Errorf("edges = %+v", a.Graph.Edges)
	}
}

// TestAutomationDryRunDecode covers the trace an upsert-contact dry run
// returns, including what the node wrote back into the event data.
func TestAutomationDryRunDecode(t *testing.T) {
	c := inboxFixtureClient(t, `{
		"trace": [
			{"node_id": "trigger", "type": "trigger", "status": "success"},
			{"node_id": "c1", "type": "condition", "label": "field contains", "status": "branch_true"},
			{"node_id": "a1", "type": "action", "action": "warmbly.upsert_contact",
			 "label": "Create or update contact", "status": "success",
			 "preview": {"email": "jane@example.com", "custom:industry": "SaaS", "contact_created": "true"}},
			{"node_id": "a2", "type": "action", "action": "warmbly.add_to_campaign", "status": "skipped"}
		],
		"data": {"email": "jane@example.com", "contact_id": "ct_1", "contact_created": true}
	}`)

	res, _, err := c.Automations.DryRun(context.Background(), "au_1", nil)
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}
	if len(res.Trace) != 4 {
		t.Fatalf("Trace = %d nodes, want 4", len(res.Trace))
	}
	if res.Trace[1].Status != NodeStatusBranchTrue {
		t.Errorf("condition status = %q", res.Trace[1].Status)
	}
	if res.Trace[2].Action != ActionUpsertContact || len(res.Trace[2].Preview) == 0 {
		t.Errorf("action node = %+v", res.Trace[2])
	}
	if res.Trace[3].Status != NodeStatusSkipped {
		t.Errorf("skipped node status = %q", res.Trace[3].Status)
	}
	var data map[string]any
	if err := json.Unmarshal(res.Data, &data); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if data["contact_id"] != "ct_1" {
		t.Errorf("data[contact_id] = %v, want the contact the action wrote", data["contact_id"])
	}
}

// TestIntegrationSyncRouting covers the integration and meeting routes.
func TestIntegrationSyncRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()

	runSyncCases(t, &got, []syncCase{
		{"Catalog", func() error {
			_, _, e := c.Integrations.Catalog(ctx)
			return e
		}, "GET", "/v1/integrations/catalog", "", nil},
		{"Connections", func() error {
			_, _, e := c.Integrations.Connections(ctx)
			return e
		}, "GET", "/v1/integrations/connections", "", nil},
		{"Connect verifier", func() error {
			_, _, e := c.Integrations.Connect(ctx, &ConnectParams{
				Provider: ProviderMillionVerifier,
				Label:    "Verification",
				Config:   map[string]any{"api_key": "mv_key"},
			})
			return e
		}, "POST", "/v1/integrations/connections", "", []string{
			`"provider":"millionverifier"`, `"api_key":"mv_key"`,
		}},
		{"Connection", func() error {
			_, _, e := c.Integrations.Connection(ctx, "conn_1")
			return e
		}, "GET", "/v1/integrations/connections/conn_1", "", nil},
		{"UpdateConfig", func() error {
			_, _, e := c.Integrations.UpdateConfig(ctx, "conn_1", &ConnectionConfigParams{SyncDirection: SyncPush})
			return e
		}, "PATCH", "/v1/integrations/connections/conn_1/config", "", []string{`"sync_direction":"push"`}},
		{"Disconnect", func() error {
			_, e := c.Integrations.Disconnect(ctx, "conn_1")
			return e
		}, "DELETE", "/v1/integrations/connections/conn_1", "", nil},
		{"Events", func() error {
			_, _, e := c.Integrations.Events(ctx, "conn_1")
			return e
		}, "GET", "/v1/integrations/connections/conn_1/events", "", nil},
		{"CreateEvent", func() error {
			_, _, e := c.Integrations.CreateEvent(ctx, "conn_1", &EventSubscriptionParams{
				EventType: TriggerContactCreated, Action: ActionSlackNotify,
			})
			return e
		}, "POST", "/v1/integrations/connections/conn_1/events", "", []string{`"event_type":"contact.created"`}},
		{"DeleteEvent", func() error {
			_, e := c.Integrations.DeleteEvent(ctx, "conn_1", "ev_1")
			return e
		}, "DELETE", "/v1/integrations/connections/conn_1/events/ev_1", "", nil},
		{"FieldMappings", func() error {
			_, _, e := c.Integrations.FieldMappings(ctx, "conn_1")
			return e
		}, "GET", "/v1/integrations/connections/conn_1/field-mappings", "", nil},
		{"ReplaceFieldMappings", func() error {
			_, _, e := c.Integrations.ReplaceFieldMappings(ctx, "conn_1", "contact", []FieldMappingInput{
				{ExternalField: "email", WarmblyField: "email"},
			})
			return e
		}, "PUT", "/v1/integrations/connections/conn_1/field-mappings", "", []string{`"object":"contact"`, `"external_field":"email"`}},
		{"Runs", func() error {
			_, _, e := c.Integrations.Runs(ctx, "conn_1")
			return e
		}, "GET", "/v1/integrations/connections/conn_1/runs", "", nil},
		{"WebhookSecret", func() error {
			_, _, e := c.Integrations.WebhookSecret(ctx, "conn_1")
			return e
		}, "GET", "/v1/integrations/connections/conn_1/webhook-secret", "", nil},
		{"Test", func() error {
			_, _, e := c.Integrations.Test(ctx, "conn_1")
			return e
		}, "POST", "/v1/integrations/connections/conn_1/test", "", nil},
		{"Push", func() error {
			_, _, e := c.Integrations.Push(ctx, "conn_1", []string{"ct_1"})
			return e
		}, "POST", "/v1/integrations/connections/conn_1/push", "", []string{`"contact_ids":["ct_1"]`}},
		{"Bookings", func() error {
			_, _, e := c.Integrations.Bookings(ctx)
			return e
		}, "GET", "/v1/integrations/bookings", "", nil},
		{"StartOAuth", func() error {
			_, _, e := c.Integrations.StartOAuth(ctx, ProviderGoogleSheets, "Sheets")
			return e
		}, "POST", "/v1/integrations/oauth/start", "", []string{`"provider":"google_sheets"`}},
		{"FinishOAuth", func() error {
			_, _, e := c.Integrations.FinishOAuth(ctx, "code_1", "state_1")
			return e
		}, "POST", "/v1/integrations/oauth/finish", "", []string{`"code":"code_1"`, `"state":"state_1"`}},
		{"ReauthOAuth", func() error {
			_, _, e := c.Integrations.ReauthOAuth(ctx, "conn_1")
			return e
		}, "POST", "/v1/integrations/oauth/reauth/conn_1", "", nil},

		{"Meetings.List", func() error {
			_, e := c.Meetings.List(ctx, &MeetingListParams{
				ListOptions: ListOptions{Limit: 20},
				Timeframe:   MeetingsUpcoming,
				Status:      MeetingBooked,
				Query:       "jane",
			})
			return e
		}, "GET", "/v1/meetings", "limit=20&q=jane&status=booked&timeframe=upcoming", nil},
		{"Meetings.Summary", func() error {
			_, _, e := c.Meetings.Summary(ctx)
			return e
		}, "GET", "/v1/meetings/summary", "", nil},
		{"Meetings.Create", func() error {
			_, _, e := c.Meetings.Create(ctx, &MeetingCreateParams{
				Title: "Intro call", InviteeEmail: "jane@example.com",
				ScheduledFor: "2026-09-10T15:00:00Z", DurationMinutes: 30,
			})
			return e
		}, "POST", "/v1/meetings", "", []string{`"scheduled_for":"2026-09-10T15:00:00Z"`, `"duration_minutes":30`}},
		{"Meetings.Delete", func() error {
			_, e := c.Meetings.Delete(ctx, "mt_1")
			return e
		}, "DELETE", "/v1/meetings/mt_1", "", nil},
	})
}

// TestMeetingCreateDecode covers the {"meeting": ...} envelope the create
// endpoint answers with, which is not the shape the list returns.
func TestMeetingCreateDecode(t *testing.T) {
	c := inboxFixtureClient(t, `{"meeting": {
		"id": "mt_1", "organization_id": "org_1", "source": "manual",
		"external_event_id": "8f0e1f6c-1f2c-4d3a-9f1a-6c2d5e4b3a21",
		"status": "booked", "invitee_email": "jane@example.com", "invitee_name": "Jane",
		"event_name": "Intro call", "scheduled_for": "2026-09-10T15:00:00Z",
		"end_time": "2026-09-10T15:30:00Z", "contact_id": "ct_1",
		"created_at": "2026-09-07T10:00:00Z", "updated_at": "2026-09-07T10:00:00Z"
	}}`)

	m, _, err := c.Meetings.Create(context.Background(), &MeetingCreateParams{
		InviteeEmail: "jane@example.com", ScheduledFor: "2026-09-10T15:00:00Z",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if m == nil {
		t.Fatal("Create returned no meeting; the {\"meeting\": ...} envelope was not unwrapped")
	}
	if m.ID != "mt_1" || m.Source != "manual" || m.Status != MeetingBooked {
		t.Errorf("meeting = %+v", *m)
	}
	if m.ScheduledFor == nil || m.EndTime == nil {
		t.Errorf("scheduled_for/end_time = %v/%v", m.ScheduledFor, m.EndTime)
	}
}

// TestIntegrationCatalogVerificationDecode covers the verification category
// added for the pay-as-you-go address verifier.
func TestIntegrationCatalogVerificationDecode(t *testing.T) {
	c := inboxFixtureClient(t, `{"catalog": [{
		"provider": "millionverifier", "name": "MillionVerifier",
		"tagline": "Pay-as-you-go address verification for every contact you import.",
		"category": "verification", "auth_method": "api_key",
		"docs_url": "https://www.millionverifier.com/",
		"highlights": ["Every new contact is checked automatically; nothing to run"],
		"supports_push": false, "configured": true
	}]}`)

	entries, _, err := c.Integrations.Catalog(context.Background())
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("Catalog = %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.Provider != ProviderMillionVerifier || e.Category != "verification" || e.AuthMethod != "api_key" {
		t.Errorf("entry = %+v", e)
	}
}

// TestTemplateSyncRouting covers the reply-template routes.
func TestTemplateSyncRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()

	runSyncCases(t, &got, []syncCase{
		{"List", func() error {
			_, _, e := c.Templates.List(ctx, "follow up")
			return e
		}, "GET", "/v1/templates", "q=follow+up", nil},
		{"Create", func() error {
			_, _, e := c.Templates.Create(ctx, &TemplateCreateParams{Name: "Follow up", Subject: "Re: {{company}}"})
			return e
		}, "POST", "/v1/templates", "", []string{`"name":"Follow up"`}},
		{"Get", func() error {
			_, _, e := c.Templates.Get(ctx, "tpl_1")
			return e
		}, "GET", "/v1/templates/tpl_1", "", nil},
		{"Update", func() error {
			_, _, e := c.Templates.Update(ctx, "tpl_1", &TemplateUpdateParams{Name: String("Follow up v2")})
			return e
		}, "PATCH", "/v1/templates/tpl_1", "", []string{`"name":"Follow up v2"`}},
		{"Delete", func() error {
			_, e := c.Templates.Delete(ctx, "tpl_1")
			return e
		}, "DELETE", "/v1/templates/tpl_1", "", nil},
		{"Reorder", func() error {
			_, _, e := c.Templates.Reorder(ctx, []string{"tpl_2", "tpl_1"})
			return e
		}, "PATCH", "/v1/templates/reorder", "", []string{`"ids":["tpl_2","tpl_1"]`}},
		{"Duplicate", func() error {
			_, _, e := c.Templates.Duplicate(ctx, "tpl_1")
			return e
		}, "POST", "/v1/templates/tpl_1/duplicate", "", nil},
		{"Render", func() error {
			_, _, e := c.Templates.Render(ctx, "tpl_1", map[string]string{"company": "Example"})
			return e
		}, "POST", "/v1/templates/tpl_1/render", "", []string{`"company":"Example"`}},
		{"Score", func() error {
			_, _, e := c.Templates.Score(ctx, &TemplateScoreParams{Subject: "FREE!!!", BodyPlain: "buy now"})
			return e
		}, "POST", "/v1/templates/score", "", []string{`"subject":"FREE!!!"`}},
	})
}

// TestLeadSyncRouting covers the Google Sheets lead-sync routes.
func TestLeadSyncRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()
	mapping := []ImportColumnMapping{{Index: 0, Target: "email"}, {Index: 1, Target: "first_name"}}

	runSyncCases(t, &got, []syncCase{
		{"Connection", func() error {
			_, _, e := c.LeadSync.Connection(ctx)
			return e
		}, "GET", "/v1/lead-sync/google/connection", "", nil},
		{"Spreadsheet", func() error {
			_, _, e := c.LeadSync.Spreadsheet(ctx, "conn_1", "sheet_1")
			return e
		}, "POST", "/v1/lead-sync/google/spreadsheet", "", []string{`"sheet_id":"sheet_1"`}},
		{"Preview", func() error {
			_, _, e := c.LeadSync.Preview(ctx, "conn_1", "sheet_1", "Leads")
			return e
		}, "POST", "/v1/lead-sync/google/preview", "", []string{`"tab_title":"Leads"`}},
		{"Sources", func() error {
			_, _, e := c.LeadSync.Sources(ctx)
			return e
		}, "GET", "/v1/lead-sync/sources", "", nil},
		{"SourcesForCampaign", func() error {
			_, _, e := c.LeadSync.SourcesForCampaign(ctx, "camp_1")
			return e
		}, "GET", "/v1/lead-sync/sources", "campaign_id=camp_1", nil},
		{"Create", func() error {
			_, _, e := c.LeadSync.Create(ctx, &LeadSyncCreateParams{
				ConnectionID: "conn_1", SheetID: "sheet_1", TabTitle: "Leads",
				HasHeader: true, ColumnMapping: mapping, Dedup: "update",
			})
			return e
		}, "POST", "/v1/lead-sync/sources", "", []string{
			`"column_mapping":[{"index":0,"target":"email"},{"index":1,"target":"first_name"}]`,
			`"dedup":"update"`,
		}},
		{"Get", func() error {
			_, _, e := c.LeadSync.Get(ctx, "src_1")
			return e
		}, "GET", "/v1/lead-sync/sources/src_1", "", nil},
		{"Update mapping", func() error {
			_, _, e := c.LeadSync.Update(ctx, "src_1", &LeadSyncUpdateParams{ColumnMapping: &mapping})
			return e
		}, "PATCH", "/v1/lead-sync/sources/src_1", "", []string{`"column_mapping":[{"index":0,"target":"email"}`}},
		{"Delete", func() error {
			_, e := c.LeadSync.Delete(ctx, "src_1")
			return e
		}, "DELETE", "/v1/lead-sync/sources/src_1", "", nil},
		{"Sync", func() error {
			_, _, e := c.LeadSync.Sync(ctx, "src_1")
			return e
		}, "POST", "/v1/lead-sync/sources/src_1/sync", "", nil},
	})
}

// TestLeadSyncSourceDecode covers a saved source, whose column mapping the
// server now validates on write.
func TestLeadSyncSourceDecode(t *testing.T) {
	c := inboxFixtureClient(t, `{
		"id": "src_1", "organization_id": "org_1", "created_by_user_id": "u_1",
		"provider": "google_sheets", "connection_id": "conn_1",
		"sheet_id": "sheet_1", "sheet_title": "Leads Q3", "tab_title": "Sheet1",
		"has_header": true,
		"column_mapping": [
			{"index": 0, "target": "email"},
			{"index": 3, "target": "custom", "custom_key": "industry"}
		],
		"dedup": "update", "target_campaign_id": "camp_1",
		"category_ids": ["cat_1"], "subscribed_default": true,
		"label": "Ads leads", "status": "idle",
		"last_synced_at": "2026-09-06T12:00:00Z",
		"last_result": {"total": 40, "imported": 31, "updated": 9, "skipped": 0, "failed": 0,
			"started_at": "2026-09-06T12:00:00Z", "ended_at": "2026-09-06T12:00:04Z"},
		"created_at": "2026-08-01T00:00:00Z", "updated_at": "2026-09-06T12:00:04Z"
	}`)

	src, _, err := c.LeadSync.Get(context.Background(), "src_1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if src.Status != LeadSyncIdle || src.Provider != ProviderGoogleSheets {
		t.Errorf("status/provider = %q/%q", src.Status, src.Provider)
	}
	if len(src.ColumnMapping) != 2 || src.ColumnMapping[1].CustomKey != "industry" {
		t.Errorf("ColumnMapping = %+v", src.ColumnMapping)
	}
	if src.TargetCampaignID == nil || *src.TargetCampaignID != "camp_1" {
		t.Errorf("TargetCampaignID = %v", src.TargetCampaignID)
	}
	if src.LastResult == nil || src.LastResult.Imported != 31 {
		t.Errorf("LastResult = %+v", src.LastResult)
	}
}

// TestCRMSyncRouting covers every CRM route, including the filters the plain
// deal and task lists accept.
func TestCRMSyncRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()
	closeBy := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

	runSyncCases(t, &got, []syncCase{
		{"ListPipelines", func() error {
			_, _, e := c.CRM.ListPipelines(ctx)
			return e
		}, "GET", "/v1/crm/pipelines", "", nil},
		{"CreatePipeline", func() error {
			_, _, e := c.CRM.CreatePipeline(ctx, &PipelineCreateParams{
				Name:   "Sales",
				Stages: []StageCreateParams{{Name: "New", Color: "#8b5cf6"}},
			})
			return e
		}, "POST", "/v1/crm/pipelines", "", []string{`"name":"Sales"`, `"stages":[{"name":"New"`}},
		{"GetPipeline", func() error {
			_, _, e := c.CRM.GetPipeline(ctx, "pl_1")
			return e
		}, "GET", "/v1/crm/pipelines/pl_1", "", nil},
		{"UpdatePipeline", func() error {
			_, _, e := c.CRM.UpdatePipeline(ctx, "pl_1", &PipelineUpdateParams{Name: String("Sales EU")})
			return e
		}, "PATCH", "/v1/crm/pipelines/pl_1", "", []string{`"name":"Sales EU"`}},
		{"DeletePipeline", func() error {
			_, e := c.CRM.DeletePipeline(ctx, "pl_1")
			return e
		}, "DELETE", "/v1/crm/pipelines/pl_1", "", nil},
		{"CreateStage", func() error {
			_, _, e := c.CRM.CreateStage(ctx, "pl_1", &StageCreateParams{Name: "Won", Color: "#22c55e"})
			return e
		}, "POST", "/v1/crm/pipelines/pl_1/stages", "", []string{`"name":"Won"`}},
		{"UpdateStage", func() error {
			_, _, e := c.CRM.UpdateStage(ctx, "pl_1", "st_1", &StageUpdateParams{Color: String("#ef4444")})
			return e
		}, "PATCH", "/v1/crm/pipelines/pl_1/stages/st_1", "", []string{`"color":"#ef4444"`}},
		{"DeleteStage", func() error {
			_, e := c.CRM.DeleteStage(ctx, "pl_1", "st_1")
			return e
		}, "DELETE", "/v1/crm/pipelines/pl_1/stages/st_1", "", nil},

		{"ListDeals", func() error {
			_, e := c.CRM.ListDeals(ctx, &DealListParams{
				ListOptions: ListOptions{Limit: 50, Cursor: "cur_1"},
				PipelineID:  "pl_1", StageID: "st_1", Status: DealStatusOpen,
			})
			return e
		}, "GET", "/v1/crm/deals", "cursor=cur_1&limit=50&pipeline_id=pl_1&stage_id=st_1&status=open", nil},
		{"ListDeals no filter", func() error {
			_, e := c.CRM.ListDeals(ctx, nil)
			return e
		}, "GET", "/v1/crm/deals", "", nil},
		{"SearchDeals", func() error {
			_, e := c.CRM.SearchDeals(ctx, &DealSearchParams{
				ListOptions: ListOptions{Limit: 200},
				Query:       "acme",
				Statuses:    []string{DealStatusOpen, DealStatusWon},
				PipelineIDs: []string{"pl_1"},
				MinValue:    Float64(1000),
				CloseBefore: &closeBy,
				SortBy:      "value",
			})
			return e
		}, "POST", "/v1/crm/deals/search", "limit=200", []string{
			`"query":"acme"`, `"statuses":["open","won"]`, `"min_value":1000`, `"sort_by":"value"`,
		}},
		{"DealsSummary", func() error {
			_, _, e := c.CRM.DealsSummary(ctx, &DealSearchParams{PipelineIDs: []string{"pl_1"}})
			return e
		}, "POST", "/v1/crm/deals/summary", "", []string{`"pipeline_ids":["pl_1"]`}},
		{"CreateDeal", func() error {
			_, _, e := c.CRM.CreateDeal(ctx, &DealCreateParams{
				PipelineID: "pl_1", StageID: "st_1", Name: "Acme", Value: Float64(5000), Currency: "EUR",
			})
			return e
		}, "POST", "/v1/crm/deals", "", []string{`"pipeline_id":"pl_1"`, `"currency":"EUR"`}},
		{"GetDeal", func() error {
			_, _, e := c.CRM.GetDeal(ctx, "dl_1")
			return e
		}, "GET", "/v1/crm/deals/dl_1", "", nil},
		{"UpdateDeal", func() error {
			_, _, e := c.CRM.UpdateDeal(ctx, "dl_1", &DealUpdateParams{
				Status: String(DealStatusLost), LostReason: String("price"),
			})
			return e
		}, "PATCH", "/v1/crm/deals/dl_1", "", []string{`"status":"lost"`, `"lost_reason":"price"`}},
		{"DeleteDeal", func() error {
			_, e := c.CRM.DeleteDeal(ctx, "dl_1")
			return e
		}, "DELETE", "/v1/crm/deals/dl_1", "", nil},

		{"ListTaskTypes", func() error {
			_, _, e := c.CRM.ListTaskTypes(ctx)
			return e
		}, "GET", "/v1/crm/task-types", "", nil},
		{"CreateTaskType", func() error {
			_, _, e := c.CRM.CreateTaskType(ctx, &TaskTypeCreateParams{Name: "Call", Color: "#8b5cf6"})
			return e
		}, "POST", "/v1/crm/task-types", "", []string{`"name":"Call"`}},
		{"UpdateTaskType", func() error {
			_, _, e := c.CRM.UpdateTaskType(ctx, "tt_1", &TaskTypeUpdateParams{Position: Int(0)})
			return e
		}, "PATCH", "/v1/crm/task-types/tt_1", "", []string{`"position":0`}},
		{"DeleteTaskType", func() error {
			_, e := c.CRM.DeleteTaskType(ctx, "tt_1")
			return e
		}, "DELETE", "/v1/crm/task-types/tt_1", "", nil},

		{"ListTasks", func() error {
			_, e := c.CRM.ListTasks(ctx, &CRMTaskListParams{
				ContactID: "ct_1", DealID: "dl_1", AssignedTo: "u_1", Status: TaskStatusPending,
			})
			return e
		}, "GET", "/v1/crm/tasks", "assigned_to=u_1&contact_id=ct_1&deal_id=dl_1&status=pending", nil},
		{"SearchTasks", func() error {
			_, e := c.CRM.SearchTasks(ctx, &TaskSearchParams{
				Statuses: []string{TaskStatusPending}, Priorities: []string{TaskPriorityHigh},
				TeamIDs: []string{"tm_1"}, Overdue: true,
			})
			return e
		}, "POST", "/v1/crm/tasks/search", "", []string{
			`"statuses":["pending"]`, `"priorities":["high"]`, `"team_ids":["tm_1"]`, `"overdue":true`,
		}},
		{"TasksSummary", func() error {
			_, _, e := c.CRM.TasksSummary(ctx, &TaskSearchParams{Overdue: true})
			return e
		}, "POST", "/v1/crm/tasks/summary", "", []string{`"overdue":true`}},
		{"CreateTask", func() error {
			_, _, e := c.CRM.CreateTask(ctx, &CRMTaskCreateParams{
				Title: "Call Jane", Priority: TaskPriorityUrgent, Type: "Call", ContactID: String("ct_1"),
			})
			return e
		}, "POST", "/v1/crm/tasks", "", []string{`"title":"Call Jane"`, `"priority":"urgent"`}},
		{"GetTask", func() error {
			_, _, e := c.CRM.GetTask(ctx, "tk_1")
			return e
		}, "GET", "/v1/crm/tasks/tk_1", "", nil},
		{"UpdateTask", func() error {
			_, _, e := c.CRM.UpdateTask(ctx, "tk_1", &CRMTaskUpdateParams{Status: String(TaskStatusCompleted)})
			return e
		}, "PATCH", "/v1/crm/tasks/tk_1", "", []string{`"status":"completed"`}},
		{"DeleteTask", func() error {
			_, e := c.CRM.DeleteTask(ctx, "tk_1")
			return e
		}, "DELETE", "/v1/crm/tasks/tk_1", "", nil},
	})
}

// TestDealsSummaryDecode covers the server-side aggregate a kanban header
// reads instead of reducing a page.
func TestDealsSummaryDecode(t *testing.T) {
	c := inboxFixtureClient(t, `{
		"total": 120, "open_count": 80, "open_value": 240000,
		"won_count": 30, "won_value": 150000, "lost_count": 10, "lost_value": 20000,
		"currency": "USD", "mixed_currency": true,
		"stages": [
			{"stage_id": "st_1", "count": 40, "value": 90000},
			{"stage_id": "st_2", "count": 40, "value": 150000}
		]
	}`)

	s, _, err := c.CRM.DealsSummary(context.Background(), nil)
	if err != nil {
		t.Fatalf("DealsSummary: %v", err)
	}
	if s.Total != 120 || s.OpenValue != 240000 || !s.MixedCurrency {
		t.Errorf("summary = %+v", *s)
	}
	if len(s.Stages) != 2 || s.Stages[1].StageID != "st_2" || s.Stages[1].Count != 40 {
		t.Errorf("Stages = %+v", s.Stages)
	}
}

// TestCRMTaskDecode covers a task assigned to a team rather than a person.
func TestCRMTaskDecode(t *testing.T) {
	c := inboxFixtureClient(t, `{
		"id": "tk_1", "organization_id": "org_1", "contact_id": "ct_1",
		"assigned_team_id": "tm_1", "created_by": "u_1",
		"title": "Call Jane", "description": "About the renewal",
		"due_date": "2026-09-09T09:00:00Z", "priority": "urgent",
		"type": "Call", "status": "in_progress",
		"created_at": "2026-09-07T09:00:00Z", "updated_at": "2026-09-07T09:30:00Z"
	}`)

	task, _, err := c.CRM.GetTask(context.Background(), "tk_1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Priority != TaskPriorityUrgent || task.Status != TaskStatusInProgress {
		t.Errorf("priority/status = %q/%q", task.Priority, task.Status)
	}
	if task.AssignedTeamID == nil || *task.AssignedTeamID != "tm_1" {
		t.Errorf("AssignedTeamID = %v", task.AssignedTeamID)
	}
	if task.AssignedTo != nil {
		t.Errorf("AssignedTo = %v, want nil for a team-assigned task", task.AssignedTo)
	}
	if task.Type != "Call" {
		t.Errorf("Type = %q, want the task type's name", task.Type)
	}
}

// TestTeamSyncRouting covers the team routes.
func TestTeamSyncRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()

	runSyncCases(t, &got, []syncCase{
		{"List", func() error {
			_, _, e := c.Teams.List(ctx)
			return e
		}, "GET", "/v1/teams", "", nil},
		{"Create", func() error {
			_, _, e := c.Teams.Create(ctx, &TeamCreateParams{Name: "AE team", Color: "#0ea5e9"})
			return e
		}, "POST", "/v1/teams", "", []string{`"name":"AE team"`, `"color":"#0ea5e9"`}},
		{"Get", func() error {
			_, _, e := c.Teams.Get(ctx, "tm_1")
			return e
		}, "GET", "/v1/teams/tm_1", "", nil},
		{"Update", func() error {
			_, _, e := c.Teams.Update(ctx, "tm_1", &TeamUpdateParams{Name: String("AE team EU")})
			return e
		}, "PATCH", "/v1/teams/tm_1", "", []string{`"name":"AE team EU"`}},
		{"Delete", func() error {
			_, e := c.Teams.Delete(ctx, "tm_1")
			return e
		}, "DELETE", "/v1/teams/tm_1", "", nil},
		{"AddMember", func() error {
			_, _, e := c.Teams.AddMember(ctx, "tm_1", "u_1")
			return e
		}, "POST", "/v1/teams/tm_1/members", "", []string{`"user_id":"u_1"`}},
		{"RemoveMember", func() error {
			_, e := c.Teams.RemoveMember(ctx, "tm_1", "u_1")
			return e
		}, "DELETE", "/v1/teams/tm_1/members/u_1", "", nil},
	})
}

// TestTeamDecode covers a team with its joined members.
func TestTeamDecode(t *testing.T) {
	c := inboxFixtureClient(t, `{
		"id": "tm_1", "organization_id": "org_1", "name": "AE team", "color": "#0ea5e9",
		"members": [
			{"user_id": "u_1", "email": "sam@example.com", "name": "Sam", "added_at": "2026-08-01T00:00:00Z"},
			{"user_id": "u_2", "email": "alex@example.com", "name": "Alex", "added_at": "2026-08-02T00:00:00Z"}
		],
		"created_at": "2026-08-01T00:00:00Z", "updated_at": "2026-08-02T00:00:00Z"
	}`)

	team, _, err := c.Teams.Get(context.Background(), "tm_1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(team.Members) != 2 || team.Members[1].Email != "alex@example.com" {
		t.Errorf("Members = %+v", team.Members)
	}
	if team.Members[0].AddedAt.IsZero() {
		t.Error("AddedAt did not decode")
	}
}
