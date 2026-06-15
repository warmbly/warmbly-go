package warmbly

import (
	"context"
	"net/url"
	"time"
)

// AnalyticsService reads aggregate analytics for the current organization. All
// of its operations are read-only.
type AnalyticsService service

// AnalyticsRange optionally restricts an analytics query to a time window and
// selects the bucket size of any returned time series.
type AnalyticsRange struct {
	// From is the inclusive start of the window. Nil leaves it unbounded.
	From *time.Time
	// To is the inclusive end of the window. Nil leaves it unbounded.
	To *time.Time
	// Granularity is the time-series bucket size: "day", "week" or "month".
	// Empty uses the server default.
	Granularity string
}

func (r *AnalyticsRange) values() url.Values {
	q := make(url.Values)
	if r == nil {
		return q
	}
	if r.From != nil {
		q.Set("from", r.From.Format(time.RFC3339))
	}
	if r.To != nil {
		q.Set("to", r.To.Format(time.RFC3339))
	}
	if r.Granularity != "" {
		q.Set("granularity", r.Granularity)
	}
	return q
}

// withQuery appends an encoded query string to path, omitting the "?" entirely
// when there are no parameters to send.
func withQuery(path string, q url.Values) string {
	if len(q) == 0 {
		return path
	}
	return path + "?" + q.Encode()
}

// TimeseriesPoint is a single bucket of time-series analytics.
type TimeseriesPoint struct {
	Date    time.Time `json:"date"`
	Sent    int64     `json:"sent"`
	Opens   int64     `json:"opens"`
	Clicks  int64     `json:"clicks"`
	Replies int64     `json:"replies"`
}

// DashboardAnalytics is the organization-wide analytics summary.
type DashboardAnalytics struct {
	EmailsSent      int64             `json:"emails_sent"`
	Delivered       int64             `json:"delivered"`
	Opens           int64             `json:"opens"`
	Clicks          int64             `json:"clicks"`
	Replies         int64             `json:"replies"`
	Bounces         int64             `json:"bounces"`
	Unsubscribes    int64             `json:"unsubscribes"`
	OpenRate        float64           `json:"open_rate"`
	ReplyRate       float64           `json:"reply_rate"`
	BounceRate      float64           `json:"bounce_rate"`
	ActiveCampaigns int               `json:"active_campaigns"`
	Series          []TimeseriesPoint `json:"series,omitempty"`
}

// CampaignAnalytics is the analytics summary for a single campaign.
type CampaignAnalytics struct {
	CampaignID   string            `json:"campaign_id"`
	Sent         int64             `json:"sent"`
	Delivered    int64             `json:"delivered"`
	Opens        int64             `json:"opens"`
	UniqueOpens  int64             `json:"unique_opens"`
	Clicks       int64             `json:"clicks"`
	Replies      int64             `json:"replies"`
	Bounces      int64             `json:"bounces"`
	Unsubscribes int64             `json:"unsubscribes"`
	OpenRate     float64           `json:"open_rate"`
	ReplyRate    float64           `json:"reply_rate"`
	Series       []TimeseriesPoint `json:"series,omitempty"`
}

// WarmupAnalytics is the analytics summary for mailbox warmup activity.
type WarmupAnalytics struct {
	AccountsWarming int               `json:"accounts_warming"`
	HealthScore     float64           `json:"health_score"`
	SentToday       int64             `json:"sent_today"`
	InboxRate       float64           `json:"inbox_rate"`
	SpamRate        float64           `json:"spam_rate"`
	Series          []TimeseriesPoint `json:"series,omitempty"`
}

// Dashboard retrieves the organization-wide analytics summary, optionally scoped
// to a time range.
func (s *AnalyticsService) Dashboard(ctx context.Context, params *AnalyticsRange) (*DashboardAnalytics, *Response, error) {
	out := new(DashboardAnalytics)
	resp, err := s.client.get(ctx, withQuery("analytics/dashboard", params.values()), out)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// Campaign retrieves the analytics summary for a single campaign, optionally
// scoped to a time range.
func (s *AnalyticsService) Campaign(ctx context.Context, id string, params *AnalyticsRange) (*CampaignAnalytics, *Response, error) {
	out := new(CampaignAnalytics)
	resp, err := s.client.get(ctx, withQuery("analytics/campaigns/"+url.PathEscape(id), params.values()), out)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// Warmup retrieves the mailbox-warmup analytics summary, optionally scoped to a
// time range.
func (s *AnalyticsService) Warmup(ctx context.Context, params *AnalyticsRange) (*WarmupAnalytics, *Response, error) {
	out := new(WarmupAnalytics)
	resp, err := s.client.get(ctx, withQuery("analytics/warmup", params.values()), out)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}
