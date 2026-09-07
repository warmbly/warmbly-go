package warmbly

import (
	"context"
	"net/url"
	"strings"
	"time"
)

// AnalyticsService reads aggregate analytics: the organization dashboard,
// per-campaign engagement, warmup progress, deliverability health, mailbox
// status and plan usage. Every operation is read-only.
//
// The date-range endpoints take plain calendar days, which the SDK formats as
// YYYY-MM-DD in UTC.
type AnalyticsService service

// Rolling windows accepted by [AnalyticsService.Dashboard].
const (
	Period7Days  = "7d"
	Period30Days = "30d"
	Period90Days = "90d"
)

// Usage windows accepted by [AnalyticsService.Usage].
const (
	UsagePeriodDay   = "day"
	UsagePeriodWeek  = "week"
	UsagePeriodMonth = "month"
)

// DateRange is the window an analytics response covers.
type DateRange struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// DashboardAnalytics is the organization-wide engagement summary.
type DashboardAnalytics struct {
	// Period is [Period7Days], [Period30Days] or [Period90Days].
	Period         string              `json:"period"`
	OverallStats   OverallStats        `json:"overall_stats"`
	RecentActivity []ActivityEvent     `json:"recent_activity"`
	TopCampaigns   []CampaignSummary   `json:"top_campaigns"`
	AccountHealth  AccountHealthTotals `json:"account_health"`
	DailyTrend     []DailyStat         `json:"daily_trend"`
}

// OverallStats are the headline counters for the dashboard window.
type OverallStats struct {
	TotalEmailsSent int64 `json:"total_emails_sent"`
	TotalOpens      int64 `json:"total_opens"`
	// MachineOpens are opens attributed to a mail-privacy proxy rather than a
	// human, and are excluded from OpenRate.
	MachineOpens int64 `json:"machine_opens"`
	TotalClicks  int64 `json:"total_clicks"`
	// MachineClicks counts steps whose only clicks came from automated
	// fetchers (security gateways walking the links). They are not part of
	// TotalClicks, which only ever counts a person's click.
	MachineClicks   int64   `json:"machine_clicks"`
	TotalReplies    int64   `json:"total_replies"`
	TotalBounces    int64   `json:"total_bounces"`
	OpenRate        float64 `json:"open_rate"`
	ClickRate       float64 `json:"click_rate"`
	ReplyRate       float64 `json:"reply_rate"`
	BounceRate      float64 `json:"bounce_rate"`
	ActiveCampaigns int     `json:"active_campaigns"`
	ActiveAccounts  int     `json:"active_accounts"`
}

// ActivityEvent is one recent engagement event on the dashboard feed.
type ActivityEvent struct {
	// Type is the engagement, for example "open", "click" or "reply".
	Type         string    `json:"type"`
	CampaignID   string    `json:"campaign_id"`
	CampaignName string    `json:"campaign_name"`
	ContactEmail string    `json:"contact_email"`
	ContactID    string    `json:"contact_id"`
	Timestamp    time.Time `json:"timestamp"`
	// Link is the URL that was clicked, on click events.
	Link string `json:"link,omitempty"`
}

// CampaignSummary is one campaign's headline engagement.
type CampaignSummary struct {
	CampaignID string  `json:"campaign_id"`
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	EmailsSent int64   `json:"emails_sent"`
	OpenRate   float64 `json:"open_rate"`
	ClickRate  float64 `json:"click_rate"`
	ReplyRate  float64 `json:"reply_rate"`
	BounceRate float64 `json:"bounce_rate,omitempty"`
}

// AccountHealthTotals counts mailboxes by health band.
type AccountHealthTotals struct {
	TotalAccounts   int `json:"total_accounts"`
	HealthyAccounts int `json:"healthy_accounts"`
	WarningAccounts int `json:"warning_accounts"`
	ErrorAccounts   int `json:"error_accounts"`
}

// DailyStat is one day of engagement on a trend line.
type DailyStat struct {
	// Date is a calendar day formatted YYYY-MM-DD.
	Date    string `json:"date"`
	Sent    int64  `json:"sent"`
	Opens   int64  `json:"opens"`
	Clicks  int64  `json:"clicks"`
	Replies int64  `json:"replies"`
}

// HourlyStat is one hour of engagement within a day.
type HourlyStat struct {
	// Hour is 0-23 in the organization timezone.
	Hour    int   `json:"hour"`
	Sent    int64 `json:"sent"`
	Opens   int64 `json:"opens"`
	Clicks  int64 `json:"clicks"`
	Replies int64 `json:"replies"`
}

// CampaignAnalytics is one campaign's engagement, broken down by step and, for
// human opens and clicks, by where and on what they happened.
type CampaignAnalytics struct {
	CampaignID string                  `json:"campaign_id"`
	Name       string                  `json:"name"`
	Status     string                  `json:"status"`
	DateRange  DateRange               `json:"date_range"`
	Summary    CampaignAnalyticsTotals `json:"summary"`
	Steps      []StepAnalytics         `json:"steps"`
	DailyStats []DailyStat             `json:"daily_stats,omitempty"`
	// Engagement is the country, mail-client and device breakdown of human
	// opens and clicks. It is best-effort: nil when the breakdown could not be
	// computed, in which case Summary still stands on its own.
	Engagement *CampaignEngagement `json:"engagement,omitempty"`
}

// CampaignEngagement is the "where from, on what" view of a campaign's human
// opens and clicks. Each list is ordered by activity and capped at the busiest
// buckets; an empty key means unknown.
type CampaignEngagement struct {
	// Countries is keyed by ISO 3166-1 alpha-2 country code.
	Countries []EngagementBucket `json:"countries"`
	// Clients is keyed by mail client or browser name.
	Clients []EngagementBucket `json:"clients"`
	// Devices is keyed by device type, for example "desktop" or "mobile".
	Devices []EngagementBucket `json:"devices"`
}

// EngagementBucket is one slice of an engagement breakdown: how many distinct
// contacts opened and clicked from that country, client or device.
type EngagementBucket struct {
	Key    string `json:"key"`
	Opens  int64  `json:"opens"`
	Clicks int64  `json:"clicks"`
}

// CampaignAnalyticsTotals are a campaign's headline counters.
type CampaignAnalyticsTotals struct {
	TotalContacts int64 `json:"total_contacts"`
	EmailsSent    int64 `json:"emails_sent"`
	// EmailsPending are queued sends not yet dispatched.
	EmailsPending int64 `json:"emails_pending"`
	UniqueOpens   int64 `json:"unique_opens"`
	// MachineOpens is the subset of UniqueOpens from automated fetchers
	// (mail-privacy prefetch, UA-less clients). Human opens are
	// UniqueOpens - MachineOpens.
	MachineOpens int64 `json:"machine_opens"`
	UniqueClicks int64 `json:"unique_clicks"`
	// MachineClicks counts steps whose only clicks came from automated
	// fetchers. They are not part of UniqueClicks, which only ever counts a
	// person's click.
	MachineClicks int64   `json:"machine_clicks"`
	Replies       int64   `json:"replies"`
	Bounces       int64   `json:"bounces"`
	Unsubscribes  int64   `json:"unsubscribes"`
	OpenRate      float64 `json:"open_rate"`
	ClickRate     float64 `json:"click_rate"`
	ReplyRate     float64 `json:"reply_rate"`
	BounceRate    float64 `json:"bounce_rate"`
}

// StepAnalytics is one sequence step's engagement.
type StepAnalytics struct {
	StepID     string `json:"step_id"`
	Name       string `json:"name"`
	Position   int    `json:"position"`
	EmailsSent int64  `json:"emails_sent"`
	Opens      int64  `json:"opens"`
	Clicks     int64  `json:"clicks"`
	Replies    int64  `json:"replies"`
	Bounces    int64  `json:"bounces"`
}

// CampaignComparison compares several campaigns over one window.
type CampaignComparison struct {
	Campaigns []CampaignSummary `json:"campaigns"`
	Period    DateRange         `json:"period"`
}

// WarmupAnalytics is warmup progress for one mailbox, or for the organization
// when no mailbox was named.
type WarmupAnalytics struct {
	EmailAccountID string            `json:"email_account_id"`
	Email          string            `json:"email"`
	DateRange      DateRange         `json:"date_range"`
	Summary        WarmupSummary     `json:"summary"`
	DailyStats     []WarmupDailyStat `json:"daily_stats"`
}

// WarmupSummary is the rollup for a warmup window.
type WarmupSummary struct {
	TotalSent    int64   `json:"total_sent"`
	TotalReplied int64   `json:"total_replied"`
	AverageDaily float64 `json:"average_daily"`
	ReplyRate    float64 `json:"reply_rate"`
	// TargetProgress is how far the ramp has come, from 0 to 1.
	TargetProgress float64 `json:"target_progress"`
	DaysActive     int     `json:"days_active"`
}

// WarmupDailyStat is one day of warmup volume against its target.
type WarmupDailyStat struct {
	Date          string `json:"date"`
	EmailsSent    int64  `json:"emails_sent"`
	EmailsReplied int64  `json:"emails_replied"`
	TargetVolume  int64  `json:"target_volume"`
}

// Deliverability health bands returned in [DeliverabilityDashboard.Band] and in
// the per-mailbox and per-campaign breakdowns.
const (
	BandHealthy     = "healthy"
	BandWatch       = "watch"
	BandThrottled   = "throttled"
	BandQuarantined = "quarantined"
	BandBlocked     = "blocked"
)

// DeliverabilityDashboard is the organization's sending health over a window:
// bounce and complaint pressure, inbox placement, reply intent, and the
// mailboxes and campaigns driving it.
type DeliverabilityDashboard struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`

	EventsTotal          int64 `json:"events_total"`
	BounceCount          int64 `json:"bounce_count"`
	ComplaintCount       int64 `json:"complaint_count"`
	UnsubscribeCount     int64 `json:"unsubscribe_count"`
	ReplyCount           int64 `json:"reply_count"`
	OpenCount            int64 `json:"open_count"`
	ClickCount           int64 `json:"click_count"`
	SuppressedRecipients int64 `json:"suppressed_recipients"`
	// DLQPending is how many send tasks are parked in the dead-letter queue.
	DLQPending int64 `json:"dlq_pending"`

	IntentPositive    int64 `json:"intent_positive"`
	IntentNegative    int64 `json:"intent_negative"`
	IntentOutOfOffice int64 `json:"intent_out_of_office"`
	IntentQuestion    int64 `json:"intent_question"`
	IntentNeutral     int64 `json:"intent_neutral"`

	EmailsSent    int64   `json:"emails_sent"`
	BounceRate    float64 `json:"bounce_rate"`
	ComplaintRate float64 `json:"complaint_rate"`
	OpenRate      float64 `json:"open_rate"`
	ClickRate     float64 `json:"click_rate"`
	ReplyRate     float64 `json:"reply_rate"`

	// SpamPlacementRate and InboxPlacementRate come from seed-inbox testing;
	// PlacementSamples is how many seeds backed them. Both rates are omitted
	// (and decode as zero) when the window has no seed samples.
	SpamPlacementRate  float64 `json:"spam_placement_rate"`
	InboxPlacementRate float64 `json:"inbox_placement_rate"`
	PlacementSamples   int64   `json:"placement_samples"`

	// Band is the overall health verdict: one of the Band* constants. Score
	// folds the same bounce, complaint and spam-placement rates into a 0 to
	// 100 composite, higher being healthier.
	Band  string `json:"band"`
	Score int    `json:"score"`

	Timeseries []DeliverabilityDay       `json:"timeseries,omitempty"`
	ByMailbox  []DeliverabilityBreakdown `json:"by_mailbox,omitempty"`
	ByCampaign []DeliverabilityBreakdown `json:"by_campaign,omitempty"`
	// ByProvider breaks the seed placement results down per recipient
	// provider.
	ByProvider []ProviderPlacement `json:"by_provider,omitempty"`
	// WarmupPlacement is the continuous warmup-derived placement signal per
	// recipient domain.
	WarmupPlacement []WarmupDomainPlacement `json:"warmup_placement,omitempty"`
}

// ProviderPlacement is one recipient provider's seed placement rollup: where
// the seed messages landed.
type ProviderPlacement struct {
	Provider   string  `json:"provider"`
	Samples    int64   `json:"samples"`
	Inbox      int64   `json:"inbox"`
	Promotions int64   `json:"promotions"`
	Spam       int64   `json:"spam"`
	Other      int64   `json:"other"`
	InboxRate  float64 `json:"inbox_rate"`
	SpamRate   float64 `json:"spam_rate"`
}

// WarmupDomainPlacement is one recipient domain's warmup placement rollup.
// Delivered counts verified warmup arrivals; Spam the ones flagged into junk.
type WarmupDomainPlacement struct {
	Provider  string  `json:"provider"`
	Domain    string  `json:"domain"`
	Delivered int64   `json:"delivered"`
	Spam      int64   `json:"spam"`
	InboxRate float64 `json:"inbox_rate"`
	SpamRate  float64 `json:"spam_rate"`
}

// DeliverabilityDay is one day on the deliverability trend line.
type DeliverabilityDay struct {
	Date         string `json:"date"`
	Sent         int64  `json:"sent"`
	Bounces      int64  `json:"bounces"`
	Complaints   int64  `json:"complaints"`
	Opens        int64  `json:"opens"`
	Clicks       int64  `json:"clicks"`
	Replies      int64  `json:"replies"`
	Unsubscribes int64  `json:"unsubscribes"`
}

// DeliverabilityBreakdown is one mailbox's or campaign's contribution to
// deliverability. EmailAccountID and Email are set on the mailbox breakdown;
// CampaignID and Name on the campaign breakdown.
type DeliverabilityBreakdown struct {
	EmailAccountID string `json:"email_account_id,omitempty"`
	Email          string `json:"email,omitempty"`
	CampaignID     string `json:"campaign_id,omitempty"`
	Name           string `json:"name,omitempty"`

	Sent          int64   `json:"sent"`
	Bounces       int64   `json:"bounces"`
	Complaints    int64   `json:"complaints"`
	BounceRate    float64 `json:"bounce_rate"`
	ComplaintRate float64 `json:"complaint_rate"`
	// Band is one of the Band* constants.
	Band string `json:"band"`
}

// AccountStatus is one mailbox's operational state: health, recent errors,
// how much of its daily allowance it has used, and anything currently holding
// its volume down.
type AccountStatus struct {
	ID           string            `json:"id"`
	Email        string            `json:"email"`
	Provider     string            `json:"provider"`
	Status       string            `json:"status"`
	LastSyncedAt *time.Time        `json:"last_synced_at"`
	Health       AccountHealth     `json:"health"`
	Errors       []AccountError    `json:"errors,omitempty"`
	DailyUsage   AccountDailyUsage `json:"daily_usage"`
	// WarmupStatus is the warmup ramp, present once warmup has ever been
	// enabled (running or paused).
	WarmupStatus *WarmupStatus `json:"warmup_status,omitempty"`
	// WarmupHealth is the mailbox's standing in the warmup pool. It is folded
	// into Health.Score and nil when the mailbox is not in a pool.
	WarmupHealth *WarmupHealth `json:"warmup_health,omitempty"`
	// InCampaign reports whether the mailbox is attached to a running campaign.
	// When true a low-volume health-check warmup keeps running even if warmup
	// is paused or off.
	InCampaign bool `json:"in_campaign"`
	// SendLifecycle is present only when the mailbox is NOT in cold rotation
	// (resting or held in reserve); an active mailbox needs no explanation.
	SendLifecycle *SendLifecycleState `json:"send_lifecycle,omitempty"`
	// ColdRamp is present only while the warmup-to-cold graduation ceiling
	// holds today's cold allowance below the mailbox's own campaign limit.
	ColdRamp *ColdRamp `json:"cold_ramp,omitempty"`
}

// WarmupStatus is a mailbox's warmup ramp as the scheduler will act on it.
type WarmupStatus struct {
	Enabled   bool       `json:"enabled"`
	Paused    bool       `json:"paused"`
	PausedAt  *time.Time `json:"paused_at,omitempty"`
	StartedAt time.Time  `json:"started_at"`
	// CurrentVolume is today's warmup sends so far; TargetVolume is today's
	// target after any hold; MaxVolume is the configured ceiling.
	CurrentVolume int `json:"current_volume"`
	TargetVolume  int `json:"target_volume"`
	MaxVolume     int `json:"max_volume"`
	// ReplyRate is the configured warmup reply percentage.
	ReplyRate  int `json:"reply_rate"`
	DaysActive int `json:"days_active"`
	// RampHold explains a ramp that is not climbing, so a TargetVolume below
	// the plain ramp is never an unexplained drop. Nil while the ramp is free
	// to climb.
	RampHold *WarmupRampHold `json:"ramp_hold,omitempty"`
}

// WarmupRampHold explains a frozen warmup ramp. It is present for the whole
// freeze; VolumeCut says whether today's volume is also reduced, which lasts a
// shorter window.
type WarmupRampHold struct {
	// Placements is how many warmup messages landed in spam in the last 48
	// hours, out of Sends.
	Placements int  `json:"placements"`
	Sends      int  `json:"sends"`
	VolumeCut  bool `json:"volume_cut"`
	// ResumesAt is when the ramp climbs again if nothing else lands in spam.
	ResumesAt time.Time `json:"resumes_at"`
}

// WarmupHealth is a mailbox's standing in the warmup pool.
type WarmupHealth struct {
	// State is one of the Band* constants.
	State string `json:"state"`
	// Score runs 0 to 100, higher being healthier.
	Score  float64 `json:"score"`
	Reason string  `json:"reason,omitempty"`
	// SpamScore is the last content spam score, 0 to 100, lower being safer.
	SpamScore    int        `json:"spam_score"`
	BlockedUntil *time.Time `json:"blocked_until,omitempty"`
	EvaluatedAt  *time.Time `json:"evaluated_at,omitempty"`
}

// The SendLifecycle* constants and [SendLifecycleState] are declared in
// emails.go, where the hold/release endpoints that drive them live.

// ColdRamp explains a cold sending cap held below the mailbox's configured
// campaign limit while it graduates from warmup.
type ColdRamp struct {
	// Ceiling is today's cold allowance; MailboxCap is what the owner
	// configured.
	Ceiling    int `json:"ceiling"`
	MailboxCap int `json:"mailbox_cap"`
	// DaysToFullCap is how many clean days remain before Ceiling reaches
	// MailboxCap, 0 when it arrives today.
	DaysToFullCap int `json:"days_to_full_cap"`
	// Held is true when a recent spam placement is pausing the climb.
	Held bool `json:"held"`
}

// AccountHealth is a mailbox's health verdict.
type AccountHealth struct {
	Status string `json:"status"`
	// Score runs 0 to 100, higher being healthier.
	Score  int      `json:"score"`
	Issues []string `json:"issues,omitempty"`
}

// AccountError is a recent error recorded against a mailbox.
type AccountError struct {
	ID        string `json:"id"`
	ErrorCode string `json:"error_code"`
	Severity  string `json:"severity"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	// ActionRequired tells the mailbox owner what to do about it, when there
	// is something to do.
	ActionRequired *string   `json:"action_required,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// AccountDailyUsage is a mailbox's sending against today's caps. The warmup
// pair is omitted (zero) for a mailbox that is not warming.
type AccountDailyUsage struct {
	Date          string `json:"date"`
	CampaignSent  int64  `json:"campaign_sent"`
	CampaignLimit int64  `json:"campaign_limit"`
	WarmupSent    int64  `json:"warmup_sent"`
	WarmupLimit   int64  `json:"warmup_limit"`
}

// UsageOverview is the organization's consumption against its plan.
type UsageOverview struct {
	UserID string `json:"user_id"`
	// Period is [UsagePeriodDay], [UsagePeriodWeek] or [UsagePeriodMonth].
	Period        string            `json:"period"`
	EmailAccounts EmailAccountUsage `json:"email_accounts"`
	Campaigns     CampaignUsage     `json:"campaigns"`
	Contacts      ContactUsage      `json:"contacts"`
	API           APIUsage          `json:"api"`
}

// EmailAccountUsage counts mailboxes by state.
type EmailAccountUsage struct {
	Total      int `json:"total"`
	Active     int `json:"active"`
	InWarmup   int `json:"in_warmup"`
	WithErrors int `json:"with_errors"`
}

// CampaignUsage counts campaigns by state plus total volume.
type CampaignUsage struct {
	Total      int   `json:"total"`
	Active     int   `json:"active"`
	Paused     int   `json:"paused"`
	Draft      int   `json:"draft"`
	EmailsSent int64 `json:"emails_sent"`
}

// ContactUsage counts contacts.
type ContactUsage struct {
	Total      int64 `json:"total"`
	Subscribed int64 `json:"subscribed"`
	AddedToday int64 `json:"added_today"`
}

// APIUsage is API call volume against the plan's daily limit.
type APIUsage struct {
	TotalCalls int64 `json:"total_calls"`
	DailyLimit int64 `json:"daily_limit"`
	// TopEndpoints are the busiest endpoints, each carrying its own endpoint
	// and count keys.
	TopEndpoints []map[string]any `json:"top_endpoints,omitempty"`
}

// Dashboard returns the organization-wide engagement summary. Period is
// [Period7Days], [Period30Days] or [Period90Days]; anything else falls back to
// [Period7Days].
func (s *AnalyticsService) Dashboard(ctx context.Context, period string, opts ...RequestOption) (*DashboardAnalytics, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "period", period)
	return fetch[DashboardAnalytics](ctx, s.client, withQuery("analytics/dashboard", q), opts)
}

// Campaign returns one campaign's engagement, broken down by step.
func (s *AnalyticsService) Campaign(ctx context.Context, id string, opts ...RequestOption) (*CampaignAnalytics, *Response, error) {
	return fetch[CampaignAnalytics](ctx, s.client, "analytics/campaigns/"+url.PathEscape(id), opts)
}

// CampaignDaily returns a campaign's day-by-day engagement over a date range.
func (s *AnalyticsService) CampaignDaily(ctx context.Context, id string, from, to time.Time, opts ...RequestOption) ([]DailyStat, *Response, error) {
	q := url.Values{"from": {formatDay(from)}, "to": {formatDay(to)}}
	return fetchData[DailyStat](ctx, s.client, withQuery("analytics/campaigns/"+url.PathEscape(id)+"/daily", q), opts)
}

// CampaignHourly returns a campaign's hour-by-hour engagement for one day. A
// zero date reports today.
func (s *AnalyticsService) CampaignHourly(ctx context.Context, id string, date time.Time, opts ...RequestOption) ([]HourlyStat, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "date", formatDay(date))
	return fetchData[HourlyStat](ctx, s.client, withQuery("analytics/campaigns/"+url.PathEscape(id)+"/hourly", q), opts)
}

// CompareCampaigns compares up to ten campaigns over one date range.
func (s *AnalyticsService) CompareCampaigns(ctx context.Context, ids []string, from, to time.Time, opts ...RequestOption) (*CampaignComparison, *Response, error) {
	q := url.Values{
		"ids":  {strings.Join(ids, ",")},
		"from": {formatDay(from)},
		"to":   {formatDay(to)},
	}
	return fetch[CampaignComparison](ctx, s.client, withQuery("analytics/campaigns/compare", q), opts)
}

// Warmup returns warmup progress over a date range. Pass an empty emailID for
// the whole organization.
func (s *AnalyticsService) Warmup(ctx context.Context, emailID string, from, to time.Time, opts ...RequestOption) (*WarmupAnalytics, *Response, error) {
	q := url.Values{"from": {formatDay(from)}, "to": {formatDay(to)}}
	setNonEmpty(q, "email_id", emailID)
	return fetch[WarmupAnalytics](ctx, s.client, withQuery("analytics/warmup", q), opts)
}

// Deliverability returns the organization's sending health over a window. Zero
// times default to the last seven days.
func (s *AnalyticsService) Deliverability(ctx context.Context, from, to time.Time, opts ...RequestOption) (*DeliverabilityDashboard, *Response, error) {
	q := make(url.Values)
	setTime(q, "from", &from)
	setTime(q, "to", &to)
	return fetch[DeliverabilityDashboard](ctx, s.client, withQuery("analytics/deliverability", q), opts)
}

// Accounts returns the operational status of every mailbox.
func (s *AnalyticsService) Accounts(ctx context.Context, opts ...RequestOption) ([]AccountStatus, *Response, error) {
	return fetchData[AccountStatus](ctx, s.client, "analytics/accounts", opts)
}

// Account returns one mailbox's operational status, including its warmup
// ramp, any cold-ramp ceiling and any lifecycle hold.
func (s *AnalyticsService) Account(ctx context.Context, id string, opts ...RequestOption) (*AccountStatus, *Response, error) {
	return fetch[AccountStatus](ctx, s.client, "analytics/accounts/"+url.PathEscape(id), opts)
}

// Usage returns the organization's consumption against its plan. Period is
// [UsagePeriodDay], [UsagePeriodWeek] or [UsagePeriodMonth].
func (s *AnalyticsService) Usage(ctx context.Context, period string, opts ...RequestOption) (*UsageOverview, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "period", period)
	return fetch[UsageOverview](ctx, s.client, withQuery("analytics/usage", q), opts)
}

// formatDay renders a calendar day the way the analytics endpoints expect. A
// zero time renders empty so the caller can omit the parameter.
func formatDay(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}
