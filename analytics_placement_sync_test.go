package warmbly

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestAnalyticsPlacementRouting(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"data": [], "pagination": {"has_more": false}}`)
	ctx := context.Background()
	from := time.Date(2026, 8, 16, 23, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)

	cases := []struct {
		name      string
		call      func() error
		wantPath  string
		wantQuery string
	}{
		{"WarmupPlacement workspace defaults", func() error {
			_, _, e := c.Analytics.WarmupPlacement(ctx, "", time.Time{}, time.Time{})
			return e
		}, "/v1/analytics/warmup/placement", ""},
		{"WarmupPlacement mailbox window", func() error {
			_, _, e := c.Analytics.WarmupPlacement(ctx, "em_1", from, to)
			return e
		}, "/v1/analytics/warmup/placement", "email_id=em_1&from=2026-08-16&to=2026-09-14"},
		{"CampaignRange", func() error {
			_, _, e := c.Analytics.CampaignRange(ctx, "camp_1", from, to)
			return e
		}, "/v1/analytics/campaigns/camp_1", "from=2026-08-16&to=2026-09-14"},
		{"Campaign all time", func() error { _, _, e := c.Analytics.Campaign(ctx, "camp_1"); return e }, "/v1/analytics/campaigns/camp_1", ""},
		{"DirectMail", func() error { _, _, e := c.Analytics.Direct(ctx, Period30Days); return e }, "/v1/analytics/direct", "period=30d"},
		{"AccountsPage", func() error {
			_, e := c.Analytics.AccountsPage(ctx, &AccountListParams{ListOptions: ListOptions{Limit: 200, Cursor: "abc"}, EmailIDs: []string{"em_1", "em_2"}})
			return e
		}, "/v1/analytics/accounts", "cursor=abc&email_ids=em_1%2Cem_2&limit=200"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got = recordedRequest{}
			if err := tc.call(); err != nil {
				// The list shape cannot stand in for the object endpoints; only
				// the request matters here.
				if got.path == "" {
					t.Fatalf("call: %v", err)
				}
			}
			if got.method != "GET" || got.path != tc.wantPath {
				t.Errorf("got %s %s, want GET %s", got.method, got.path, tc.wantPath)
			}
			if got.rawQuery != tc.wantQuery {
				t.Errorf("query = %q, want %q", got.rawQuery, tc.wantQuery)
			}
		})
	}
}

func TestWarmupPlacementReportDecode(t *testing.T) {
	counts := WarmupPlacementCounts{Sent: 40, Delivered: 36, Inbox: 30, Tabs: 4, Spam: 2, Rescued: 2, Unconfirmed: 1, InboxRate: f64(94.44), SpamRate: f64(5.56)}
	want := WarmupPlacementReport{
		EmailAccountID: String("em_1"),
		DateRange:      DateRange{From: time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)},
		Summary:        counts,
		Rate: WarmupPlacementRate{
			WindowDays: 7, MinSample: 20, Scope: "major", Delivered: 30, Inbox: 25, Tabs: 3, Spam: 2, InboxRate: f64(93.33),
			Band: WarmupPlacementBandGood, OtherDelivered: 6, OtherInboxRate: f64(100),
		},
		Daily: []WarmupPlacementDay{
			{Date: "2026-09-13", WarmupPlacementCounts: counts, RollingInboxRate: f64(92.5), Groups: []WarmupPlacementDayGroup{{Group: WarmupRecipientGoogle, Inbox: 20, Tabs: 4, Spam: 1, Rescued: 1}}},
			{Date: "2026-09-14", Groups: []WarmupPlacementDayGroup{}},
		},
		Providers: []WarmupPlacementProvider{{
			Group: WarmupRecipientMicrosoft, WarmupPlacementCounts: counts,
			Hosts: []WarmupPlacementHost{{Host: "outlook", WarmupPlacementCounts: counts}},
		}},
		Mailboxes: []WarmupPlacementMailbox{{
			EmailAccountID: "em_1", Email: "dana@acme.com", WarmupPlacementCounts: counts,
			Rate: WarmupPlacementRate{WindowDays: 7, MinSample: 20, Scope: "major", Band: WarmupPlacementBandCollecting}, DailyInboxRate: []*float64{f64(90), nil, f64(100)},
		}},
	}
	var got recordedRequest
	c := recordingFixtureClient(t, &got, syncJSON(t, want))
	have, _, err := c.Analytics.WarmupPlacement(context.Background(), "em_1", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("WarmupPlacement: %v", err)
	}
	if !reflect.DeepEqual(*have, want) {
		t.Errorf("report mismatch\n got  %+v\n want %+v", *have, want)
	}
	// The embedded counts flatten onto the day, so the wire stays flat.
	var raw map[string]any
	if err := json.Unmarshal([]byte(syncJSON(t, want.Daily[0])), &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["inbox"]; !ok || raw["date"] != "2026-09-13" {
		t.Errorf("day wire shape = %v", raw)
	}

	// A workspace report names no mailbox, and an unknown band decodes as is.
	c = recordingFixtureClient(t, &got, `{"date_range":{"from":"2026-08-16T00:00:00Z","to":"2026-09-14T00:00:00Z"},
	  "summary":{"sent":0,"delivered":0,"inbox":0,"tabs":0,"spam":0,"rescued":0,"unconfirmed":0,"inbox_rate":null,"spam_rate":null},
	  "rate":{"window_days":7,"min_sample":20,"scope":"major","delivered":0,"inbox":0,"tabs":0,"spam":0,"inbox_rate":null,"band":"shiny","other_delivered":0,"other_inbox_rate":null},
	  "daily":[],"providers":[]}`)
	empty, _, err := c.Analytics.WarmupPlacement(context.Background(), "", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("WarmupPlacement workspace: %v", err)
	}
	if empty.EmailAccountID != nil || empty.Rate.Band != "shiny" || empty.Rate.InboxRate != nil || empty.Mailboxes != nil {
		t.Errorf("report = %+v", empty)
	}
}

func TestDirectMailDecode(t *testing.T) {
	want := DirectMailAnalytics{
		Period:   "30d",
		Volume:   DirectMailVolume{Sent: 120, Received: 80, ThreadsStarted: 40, Replied: 12, ReplyRate: 30, Bounced: 3, MedianReplyMinutes: 95},
		Tracking: DirectMailTracking{MailboxesOptedIn: 1, MailboxesTotal: 4, TrackedSent: 50, Opened: 20, MachineOpened: 5, Clicked: 4, OpenRate: 40, ClickRate: 8},
		DailyTrend: []DirectMailDailyStat{
			{Date: time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC), Sent: 5, Received: 3},
		},
		Mailboxes:   []DirectMailMailboxStats{{EmailAccountID: "em_1", Email: "dana@acme.com", TrackDirectMail: true, Sent: 100, Received: 70}},
		TopContacts: []DirectMailContact{{Email: "ana@acme.com", Sent: 9, Received: 7, LastAt: syncTime(8)}},
	}
	var got recordedRequest
	c := recordingFixtureClient(t, &got, syncJSON(t, want))
	have, _, err := c.Analytics.Direct(context.Background(), "")
	if err != nil {
		t.Fatalf("DirectMail: %v", err)
	}
	if !reflect.DeepEqual(*have, want) {
		t.Errorf("direct mail mismatch\n got  %+v\n want %+v", *have, want)
	}
	if got.rawQuery != "" {
		t.Errorf("query = %q, want none for an empty period", got.rawQuery)
	}
}

// Step rates, machine counters and the engagement surfaces arrived on
// existing responses; a campaign analytics body carries them all.
func TestCampaignAnalyticsPeriodDecode(t *testing.T) {
	const body = `{"campaign_id":"camp_1","name":"Launch","status":"active",
	  "date_range":{"from":"2026-08-16T00:00:00Z","to":"2026-09-14T00:00:00Z"},
	  "summary":{"total_contacts":100,"emails_sent":80,"emails_pending":20,"unique_opens":40,"machine_opens":10,"unique_clicks":8,"machine_clicks":2,"replies":5,"bounces":1,"unsubscribes":0,"open_rate":50,"click_rate":10,"reply_rate":6.25,"bounce_rate":1.25},
	  "steps":[{"step_id":"st_1","name":"Email 1","position":1,"emails_sent":50,"opens":30,"machine_opens":8,"clicks":5,"machine_clicks":1,"replies":4,"bounces":1,"open_rate":60,"click_rate":10,"reply_rate":8,"bounce_rate":2}],
	  "engagement":{"countries":[],"clients":[],"devices":[],"surfaces":[{"key":"mobile_app","opens":12,"clicks":3},{"key":"hidden","opens":9,"clicks":0}]}}`
	var got recordedRequest
	c := recordingFixtureClient(t, &got, body)
	a, _, err := c.Analytics.CampaignRange(context.Background(), "camp_1", time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("CampaignRange: %v", err)
	}
	st := a.Steps[0]
	if st.Position != 1 || st.MachineOpens != 8 || st.MachineClicks != 1 || st.OpenRate != 60 || st.ReplyRate != 8 || st.BounceRate != 2 || st.ClickRate != 10 {
		t.Errorf("step = %+v", st)
	}
	if a.Engagement == nil || len(a.Engagement.Surfaces) != 2 || a.Engagement.Surfaces[1].Key != "hidden" {
		t.Errorf("engagement = %+v", a.Engagement)
	}
	if a.DateRange.From.Day() != 16 || a.DateRange.To.Day() != 14 {
		t.Errorf("date_range = %+v", a.DateRange)
	}
}

func TestAccountStatusPlacementAndHealthDecode(t *testing.T) {
	const body = `{"id":"em_1","email":"dana@acme.com","provider":"google","status":"active","last_synced_at":null,
	  "health":{"status":"warning","score":85,"issues":["Warmup inbox rate is 85% over the last 7 days"]},
	  "daily_usage":{"date":"2026-09-14","campaign_sent":1,"campaign_limit":50,"warmup_sent":2,"warmup_limit":20},
	  "warmup_status":{"enabled":true,"paused":false,"started_at":"2026-08-01T00:00:00Z","current_volume":4,"target_volume":12,"max_volume":40,"reply_rate":30,"days_active":20,
	    "partner_limit":{"reachable":9,"ramp_target":15},"send_failure":{"message":"smtp 550","at":"2026-09-14T08:30:00Z"}},
	  "warmup_health":{"pool_type":"premium","source":"cloud","state":"watch","score":71.5,"spam_score":0,
	    "partner_mailboxes_7d":18,"partner_domains_7d":11,"partner_organizations_7d":9,"received_7d":40,"senders_7d":15},
	  "warmup_placement":{"window_days":7,"min_sample":20,"scope":"major","delivered":30,"inbox":25,"tabs":3,"spam":2,"inbox_rate":93.33,"band":"good","other_delivered":0,"other_inbox_rate":null},
	  "in_campaign":true}`
	var got recordedRequest
	c := recordingFixtureClient(t, &got, body)
	a, _, err := c.Analytics.Account(context.Background(), "em_1")
	if err != nil {
		t.Fatalf("Account: %v", err)
	}
	if a.WarmupPlacement == nil || a.WarmupPlacement.Band != WarmupPlacementBandGood || a.WarmupPlacement.InboxRate == nil || *a.WarmupPlacement.InboxRate != 93.33 {
		t.Errorf("placement = %+v", a.WarmupPlacement)
	}
	if a.WarmupHealth == nil || a.WarmupHealth.PoolType != "premium" || a.WarmupHealth.Source != "cloud" || a.WarmupHealth.PartnerOrganizations7d != 9 || a.WarmupHealth.Senders7d != 15 {
		t.Errorf("warmup health = %+v", a.WarmupHealth)
	}
	ws := a.WarmupStatus
	if ws == nil || ws.PartnerLimit == nil || ws.PartnerLimit.Reachable != 9 || ws.PartnerLimit.RampTarget != 15 || ws.SendFailure == nil || ws.SendFailure.Message != "smtp 550" {
		t.Errorf("warmup status = %+v", ws)
	}
}

func TestDashboardCapacityAndActivityDecode(t *testing.T) {
	const body = `{"period":"7d","overall_stats":{},"recent_activity":[
	  {"type":"open","campaign_id":"camp_1","campaign_name":"Launch","contact_email":"ana@acme.com","contact_id":"ct_1","timestamp":"2026-09-14T08:30:00Z",
	   "origin":{"client":"Apple Mail","device_type":"mobile","country_code":"DE"},"sender_id":"em_1","sender_email":"dana@acme.com"}],
	  "top_campaigns":[],"account_health":{},"daily_trend":[],
	  "capacity_today":{"capacity":300,"remaining_today":180,"configured_ceiling":400,"mailboxes":8,"held":1}}`
	var got recordedRequest
	c := recordingFixtureClient(t, &got, body)
	d, _, err := c.Analytics.Dashboard(context.Background(), Period7Days)
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if d.CapacityToday == nil || *d.CapacityToday != (WorkspaceSendCapacity{Capacity: 300, Remaining: 180, ConfiguredCeiling: 400, Mailboxes: 8, Held: 1}) {
		t.Errorf("capacity = %+v", d.CapacityToday)
	}
	ev := d.RecentActivity[0]
	if ev.Origin == nil || ev.Origin.Client != "Apple Mail" || ev.SenderID == nil || *ev.SenderID != "em_1" || ev.SenderEmail != "dana@acme.com" {
		t.Errorf("event = %+v", ev)
	}
}

func TestWarmupAnalyticsReceivedDecode(t *testing.T) {
	const body = `{"email_account_id":"em_1","email":"dana@acme.com","date_range":{"from":"2026-09-01T00:00:00Z","to":"2026-09-14T00:00:00Z"},
	  "summary":{"total_sent":100,"total_replied":30,"total_received":90,"average_daily":7.1,"reply_rate":30,"target_progress":95.5,"days_active":14},
	  "daily_stats":[{"date":"2026-09-14","emails_sent":8,"emails_replied":2,"emails_received":7,"target_volume":9}]}`
	var got recordedRequest
	c := recordingFixtureClient(t, &got, body)
	w, _, err := c.Analytics.Warmup(context.Background(), "em_1", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("Warmup: %v", err)
	}
	if w.Summary.TotalReceived != 90 || w.Summary.TargetProgress != 95.5 || w.DailyStats[0].EmailsReceived != 7 {
		t.Errorf("warmup = %+v", w)
	}
}

// Accounts reads every page, so a workspace past one page of mailboxes is
// not silently truncated.
func TestAccountsFollowsCursor(t *testing.T) {
	var mu sync.Mutex
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		queries = append(queries, r.URL.RawQuery)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("cursor") == "" {
			_, _ = w.Write([]byte(`{"data":[{"id":"em_1"},{"id":"em_2"}],"pagination":{"total":3,"has_more":true,"next_cursor":"c2"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"em_3"}],"pagination":{"total":3,"has_more":false,"next_cursor":null}}`))
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatal(err)
	}

	accounts, resp, err := c.Analytics.Accounts(context.Background())
	if err != nil {
		t.Fatalf("Accounts: %v", err)
	}
	if len(accounts) != 3 || accounts[2].ID != "em_3" || resp == nil {
		t.Errorf("accounts = %+v", accounts)
	}
	if !reflect.DeepEqual(queries, []string{"", "cursor=c2"}) {
		t.Errorf("queries = %q", queries)
	}

	page, err := c.Analytics.AccountsPage(context.Background(), &AccountListParams{ListOptions: ListOptions{Limit: 2}})
	if err != nil {
		t.Fatalf("AccountsPage: %v", err)
	}
	ids := make([]string, 0, 3)
	for a, err := range page.All(context.Background()) {
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		ids = append(ids, a.ID)
	}
	if !reflect.DeepEqual(ids, []string{"em_1", "em_2", "em_3"}) {
		t.Errorf("ids = %v", ids)
	}
}
