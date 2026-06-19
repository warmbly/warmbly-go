package warmbly

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestAnalyticsRangeValues(t *testing.T) {
	t.Run("nil receiver yields empty values", func(t *testing.T) {
		var r *AnalyticsRange
		got := r.values()
		if got == nil {
			t.Fatal("values() = nil, want non-nil empty url.Values")
		}
		if len(got) != 0 {
			t.Errorf("values() = %v, want empty", got)
		}
	})

	t.Run("all fields formatted RFC3339 and granularity set", func(t *testing.T) {
		from := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		to := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
		r := &AnalyticsRange{From: &from, To: &to, Granularity: "week"}
		got := r.values()
		if want := from.Format(time.RFC3339); got.Get("from") != want {
			t.Errorf("from = %q, want %q", got.Get("from"), want)
		}
		if want := to.Format(time.RFC3339); got.Get("to") != want {
			t.Errorf("to = %q, want %q", got.Get("to"), want)
		}
		if got.Get("granularity") != "week" {
			t.Errorf("granularity = %q, want %q", got.Get("granularity"), "week")
		}
		if len(got) != 3 {
			t.Errorf("values() has %d keys, want 3: %v", len(got), got)
		}
	})

	t.Run("empty range yields no keys", func(t *testing.T) {
		r := &AnalyticsRange{}
		got := r.values()
		if len(got) != 0 {
			t.Errorf("values() = %v, want empty", got)
		}
	})
}

func TestAnalyticsWithQuery(t *testing.T) {
	t.Run("empty values leaves path unchanged", func(t *testing.T) {
		got := withQuery("analytics/dashboard", url.Values{})
		if got != "analytics/dashboard" {
			t.Errorf("withQuery = %q, want %q", got, "analytics/dashboard")
		}
	})

	t.Run("nil values leaves path unchanged", func(t *testing.T) {
		got := withQuery("analytics/dashboard", nil)
		if got != "analytics/dashboard" {
			t.Errorf("withQuery = %q, want %q", got, "analytics/dashboard")
		}
	})

	t.Run("non-empty values appends encoded query", func(t *testing.T) {
		q := url.Values{}
		q.Set("granularity", "day")
		got := withQuery("analytics/warmup", q)
		if want := "analytics/warmup?granularity=day"; got != want {
			t.Errorf("withQuery = %q, want %q", got, want)
		}
	})
}

func TestAnalyticsDashboard(t *testing.T) {
	t.Run("nil range sends no query parameters", func(t *testing.T) {
		var gotPath, gotRawQuery string
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotRawQuery = r.URL.RawQuery
			w.Header().Set("X-Request-ID", "req_dash")
			_, _ = w.Write([]byte(`{
				"emails_sent": 100,
				"delivered": 95,
				"opens": 40,
				"clicks": 12,
				"replies": 5,
				"bounces": 3,
				"unsubscribes": 1,
				"open_rate": 0.42,
				"reply_rate": 0.05,
				"bounce_rate": 0.03,
				"active_campaigns": 7,
				"series": [
					{"date":"2026-01-01T00:00:00Z","sent":10,"opens":4,"clicks":1,"replies":0}
				]
			}`))
		})

		dash, resp, err := c.Analytics.Dashboard(context.Background(), nil)
		if err != nil {
			t.Fatalf("Dashboard: %v", err)
		}
		if gotPath != "/v1/analytics/dashboard" {
			t.Errorf("path = %q, want %q", gotPath, "/v1/analytics/dashboard")
		}
		if gotRawQuery != "" {
			t.Errorf("raw query = %q, want empty", gotRawQuery)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want 200", resp.StatusCode)
		}
		if resp.RequestID != "req_dash" {
			t.Errorf("RequestID = %q, want %q", resp.RequestID, "req_dash")
		}
		if dash.EmailsSent != 100 || dash.Delivered != 95 || dash.Opens != 40 {
			t.Errorf("unexpected counters: %+v", dash)
		}
		if dash.OpenRate != 0.42 || dash.ReplyRate != 0.05 || dash.BounceRate != 0.03 {
			t.Errorf("unexpected rates: %+v", dash)
		}
		if dash.ActiveCampaigns != 7 {
			t.Errorf("active_campaigns = %d, want 7", dash.ActiveCampaigns)
		}
		if len(dash.Series) != 1 {
			t.Fatalf("series len = %d, want 1", len(dash.Series))
		}
		wantDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		if !dash.Series[0].Date.Equal(wantDate) {
			t.Errorf("series[0].Date = %v, want %v", dash.Series[0].Date, wantDate)
		}
		if dash.Series[0].Sent != 10 || dash.Series[0].Opens != 4 || dash.Series[0].Clicks != 1 {
			t.Errorf("unexpected series point: %+v", dash.Series[0])
		}
	})

	t.Run("range sends from/to/granularity query", func(t *testing.T) {
		from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 3, 31, 23, 59, 59, 0, time.UTC)
		var gotQuery url.Values
		var gotPath string
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotQuery = r.URL.Query()
			w.Header().Set("X-RateLimit-Limit", "100")
			w.Header().Set("X-RateLimit-Remaining", "99")
			_, _ = w.Write([]byte(`{"emails_sent":1}`))
		})

		dash, resp, err := c.Analytics.Dashboard(context.Background(), &AnalyticsRange{
			From:        &from,
			To:          &to,
			Granularity: "month",
		})
		if err != nil {
			t.Fatalf("Dashboard: %v", err)
		}
		if gotPath != "/v1/analytics/dashboard" {
			t.Errorf("path = %q", gotPath)
		}
		if got := gotQuery.Get("from"); got != from.Format(time.RFC3339) {
			t.Errorf("from = %q, want %q", got, from.Format(time.RFC3339))
		}
		if got := gotQuery.Get("to"); got != to.Format(time.RFC3339) {
			t.Errorf("to = %q, want %q", got, to.Format(time.RFC3339))
		}
		if got := gotQuery.Get("granularity"); got != "month" {
			t.Errorf("granularity = %q, want %q", got, "month")
		}
		if dash.EmailsSent != 1 {
			t.Errorf("emails_sent = %d, want 1", dash.EmailsSent)
		}
		if resp.RateLimit.Limit != 100 || resp.RateLimit.Remaining != 99 {
			t.Errorf("unexpected rate limit: %+v", resp.RateLimit)
		}
	})

	t.Run("error propagates with nil summary", func(t *testing.T) {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom","message":"explosion"}`))
		})
		dash, resp, err := c.Analytics.Dashboard(context.Background(), nil)
		if err == nil {
			t.Fatal("expected an error")
		}
		if dash != nil {
			t.Errorf("dash = %+v, want nil", dash)
		}
		if resp == nil {
			t.Error("resp = nil, want non-nil")
		}
	})
}

func TestAnalyticsCampaign(t *testing.T) {
	t.Run("escapes id and decodes summary", func(t *testing.T) {
		var gotPath, gotRawQuery string
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.EscapedPath()
			gotRawQuery = r.URL.RawQuery
			_, _ = w.Write([]byte(`{
				"campaign_id": "camp/1",
				"sent": 50,
				"delivered": 48,
				"opens": 20,
				"unique_opens": 18,
				"clicks": 6,
				"replies": 2,
				"bounces": 1,
				"unsubscribes": 0,
				"open_rate": 0.4,
				"reply_rate": 0.04
			}`))
		})

		camp, resp, err := c.Analytics.Campaign(context.Background(), "camp/1", nil)
		if err != nil {
			t.Fatalf("Campaign: %v", err)
		}
		if gotPath != "/v1/analytics/campaigns/camp%2F1" {
			t.Errorf("path = %q, want %q", gotPath, "/v1/analytics/campaigns/camp%2F1")
		}
		if gotRawQuery != "" {
			t.Errorf("raw query = %q, want empty", gotRawQuery)
		}
		if camp.CampaignID != "camp/1" {
			t.Errorf("campaign_id = %q", camp.CampaignID)
		}
		if camp.Sent != 50 || camp.Delivered != 48 || camp.UniqueOpens != 18 {
			t.Errorf("unexpected counters: %+v", camp)
		}
		if camp.OpenRate != 0.4 || camp.ReplyRate != 0.04 {
			t.Errorf("unexpected rates: %+v", camp)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want 200", resp.StatusCode)
		}
	})

	t.Run("range sends query parameters", func(t *testing.T) {
		from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
		var gotQuery url.Values
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotQuery = r.URL.Query()
			_, _ = w.Write([]byte(`{"campaign_id":"camp_2"}`))
		})

		camp, _, err := c.Analytics.Campaign(context.Background(), "camp_2", &AnalyticsRange{
			From:        &from,
			Granularity: "day",
		})
		if err != nil {
			t.Fatalf("Campaign: %v", err)
		}
		if got := gotQuery.Get("from"); got != from.Format(time.RFC3339) {
			t.Errorf("from = %q, want %q", got, from.Format(time.RFC3339))
		}
		if got := gotQuery.Get("granularity"); got != "day" {
			t.Errorf("granularity = %q, want %q", got, "day")
		}
		if gotQuery.Has("to") {
			t.Errorf("unexpected to param: %q", gotQuery.Get("to"))
		}
		if camp.CampaignID != "camp_2" {
			t.Errorf("campaign_id = %q", camp.CampaignID)
		}
	})

	t.Run("error propagates with nil summary", func(t *testing.T) {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not_found","message":"missing"}`))
		})
		camp, resp, err := c.Analytics.Campaign(context.Background(), "missing", nil)
		if err == nil {
			t.Fatal("expected an error")
		}
		if camp != nil {
			t.Errorf("camp = %+v, want nil", camp)
		}
		if resp == nil {
			t.Error("resp = nil, want non-nil")
		}
	})
}

func TestAnalyticsWarmup(t *testing.T) {
	t.Run("nil range decodes summary", func(t *testing.T) {
		var gotPath, gotRawQuery string
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotRawQuery = r.URL.RawQuery
			_, _ = w.Write([]byte(`{
				"accounts_warming": 12,
				"health_score": 0.91,
				"sent_today": 340,
				"inbox_rate": 0.97,
				"spam_rate": 0.02,
				"series": [
					{"date":"2026-02-02T00:00:00Z","sent":30,"opens":12,"clicks":3,"replies":1}
				]
			}`))
		})

		warm, resp, err := c.Analytics.Warmup(context.Background(), nil)
		if err != nil {
			t.Fatalf("Warmup: %v", err)
		}
		if gotPath != "/v1/analytics/warmup" {
			t.Errorf("path = %q, want %q", gotPath, "/v1/analytics/warmup")
		}
		if gotRawQuery != "" {
			t.Errorf("raw query = %q, want empty", gotRawQuery)
		}
		if warm.AccountsWarming != 12 || warm.SentToday != 340 {
			t.Errorf("unexpected counters: %+v", warm)
		}
		if warm.HealthScore != 0.91 || warm.InboxRate != 0.97 || warm.SpamRate != 0.02 {
			t.Errorf("unexpected rates: %+v", warm)
		}
		if len(warm.Series) != 1 {
			t.Fatalf("series len = %d, want 1", len(warm.Series))
		}
		if warm.Series[0].Sent != 30 || warm.Series[0].Replies != 1 {
			t.Errorf("unexpected series point: %+v", warm.Series[0])
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want 200", resp.StatusCode)
		}
	})

	t.Run("range sends query parameters", func(t *testing.T) {
		to := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
		var gotQuery url.Values
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotQuery = r.URL.Query()
			_, _ = w.Write([]byte(`{"accounts_warming":1}`))
		})

		warm, _, err := c.Analytics.Warmup(context.Background(), &AnalyticsRange{To: &to})
		if err != nil {
			t.Fatalf("Warmup: %v", err)
		}
		if got := gotQuery.Get("to"); got != to.Format(time.RFC3339) {
			t.Errorf("to = %q, want %q", got, to.Format(time.RFC3339))
		}
		if gotQuery.Has("from") || gotQuery.Has("granularity") {
			t.Errorf("unexpected params: %v", gotQuery)
		}
		if warm.AccountsWarming != 1 {
			t.Errorf("accounts_warming = %d, want 1", warm.AccountsWarming)
		}
	})

	t.Run("error propagates with nil summary", func(t *testing.T) {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"down","message":"unavailable"}`))
		})
		warm, resp, err := c.Analytics.Warmup(context.Background(), nil)
		if err == nil {
			t.Fatal("expected an error")
		}
		if warm != nil {
			t.Errorf("warm = %+v, want nil", warm)
		}
		if resp == nil {
			t.Error("resp = nil, want non-nil")
		}
	})
}
