package warmbly

import (
	"context"
	"encoding/json"
	"fmt"
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
	// CapacityToday is what the workspace's mailboxes can send today under the
	// scheduler's clamps, the denominator of a "sent today" meter. Nil when it
	// could not be computed.
	CapacityToday *WorkspaceSendCapacity `json:"capacity_today,omitempty"`
}

// WorkspaceSendCapacity is what a workspace's mailboxes can send today between
// them, under the same clamps a campaign's send plan applies.
type WorkspaceSendCapacity struct {
	// Capacity is today's cold sends across every mailbox that can send;
	// Remaining is what is left of it after what has already gone out.
	Capacity  int `json:"capacity"`
	Remaining int `json:"remaining_today"`
	// ConfiguredCeiling is the same mailboxes' own caps added up.
	ConfiguredCeiling int `json:"configured_ceiling"`
	// Mailboxes is how many mailboxes contribute; Held is how many are
	// attached but cannot send today.
	Mailboxes int `json:"mailboxes"`
	Held      int `json:"held"`
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
	// Origin is the client, device and location of a person's open or click,
	// when it was logged per event.
	Origin *EngagementOrigin `json:"origin,omitempty"`
	// SenderID and SenderEmail name the mailbox the step went out from, which a
	// reply credits even when it landed in a shared reply inbox.
	SenderID    *string `json:"sender_id,omitempty"`
	SenderEmail string  `json:"sender_email,omitempty"`
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
	// Surfaces combines device and app or webmail, for example "mobile_app" or
	// "webmail". "hidden" is a fetch by a mailbox provider's image proxy, which
	// hides the reader's device.
	Surfaces []EngagementBucket `json:"surfaces"`
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
	// MachineClicks counts the contacts whose only clicks on a step came from
	// automated fetchers. They are not part of UniqueClicks, which only ever
	// counts a person's click.
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
	// MachineOpens is the subset of Opens from automated fetchers; human opens
	// are Opens - MachineOpens. MachineClicks counts contacts whose only clicks
	// were automated, and are not part of Clicks.
	MachineOpens  int64 `json:"machine_opens"`
	MachineClicks int64 `json:"machine_clicks"`
	// The rates are percentages of this step's own EmailsSent, so steps that
	// reached different numbers of contacts still compare.
	OpenRate   float64 `json:"open_rate"`
	ClickRate  float64 `json:"click_rate"`
	ReplyRate  float64 `json:"reply_rate"`
	BounceRate float64 `json:"bounce_rate"`
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
	TotalSent    int64 `json:"total_sent"`
	TotalReplied int64 `json:"total_replied"`
	// TotalReceived is verified warmup mail that arrived from partners in the
	// range: the other half of the exchange, so a mailbox that sends and is
	// never written to shows in the numbers.
	TotalReceived int64   `json:"total_received"`
	AverageDaily  float64 `json:"average_daily"`
	ReplyRate     float64 `json:"reply_rate"`
	// TargetProgress is actual sends divided by planned target volume over
	// the active days, as a percentage (100 means on target).
	TargetProgress float64 `json:"target_progress"`
	DaysActive     int     `json:"days_active"`
}

// WarmupDailyStat is one day of warmup volume against its target.
type WarmupDailyStat struct {
	Date          string `json:"date"`
	EmailsSent    int64  `json:"emails_sent"`
	EmailsReplied int64  `json:"emails_replied"`
	TargetVolume  int64  `json:"target_volume"`
	// EmailsReceived is verified warmup mail that arrived that day.
	EmailsReceived int64 `json:"emails_received"`
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
	// IntentAutomated counts replies classified as automated mail.
	IntentAutomated int64 `json:"intent_automated"`

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
	Provider string `json:"provider"`
	// Label is the provider's display name.
	Label      string `json:"label,omitempty"`
	Samples    int64  `json:"samples"`
	Inbox      int64  `json:"inbox"`
	Promotions int64  `json:"promotions"`
	Spam       int64  `json:"spam"`
	Other      int64  `json:"other"`
	// Missing is copies that never arrived.
	Missing   int64   `json:"missing"`
	InboxRate float64 `json:"inbox_rate"`
	SpamRate  float64 `json:"spam_rate"`
}

// WarmupDomainPlacement is one recipient mail host's warmup placement rollup.
// Delivered counts verified warmup arrivals; Spam the ones flagged into junk.
// The server keys it by host, never by recipient domain, so Domain is empty on
// current servers; read Label.
type WarmupDomainPlacement struct {
	Provider string `json:"provider"`
	// Label is the host's display name.
	Label string `json:"label,omitempty"`
	// Domain is no longer sent.
	//
	// Deprecated: use Label.
	Domain    string  `json:"domain,omitempty"`
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
	// WarmupPlacement is where the mailbox's warmup mail landed over the
	// trailing week; it also caps Health.Score. Nil when nothing was delivered
	// in the window.
	WarmupPlacement *WarmupPlacementRate `json:"warmup_placement,omitempty"`
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
	// ReplyRate is the configured share of warmup sends that receive
	// synthetic replies, as a percentage.
	ReplyRate  int `json:"reply_rate"`
	DaysActive int `json:"days_active"`
	// RampHold explains a ramp that is not climbing, so a TargetVolume below
	// the plain ramp is never an unexplained drop. Nil while the ramp is free
	// to climb.
	RampHold *WarmupRampHold `json:"ramp_hold,omitempty"`
	// PartnerLimit is present while today's target is capped by how many
	// partners the mailbox can still reach.
	PartnerLimit *WarmupPartnerLimit `json:"partner_limit,omitempty"`
	// SendFailure is present while the newest warmup send failed and no later
	// one was confirmed delivered.
	SendFailure *WarmupSendFailure `json:"send_failure,omitempty"`
}

// WarmupPartnerLimit explains a target held below the ramp because a mailbox
// never writes to the same partner twice in a day.
type WarmupPartnerLimit struct {
	// Reachable is how many partners are available to it today, including any
	// it already wrote to; those at their inbound limit are left out.
	Reachable int `json:"reachable"`
	// RampTarget is what the ramp alone would send today.
	RampTarget int `json:"ramp_target"`
}

// WarmupSendFailure is why a warmup send failed: the server's answer when it
// gave one.
type WarmupSendFailure struct {
	Message string    `json:"message"`
	At      time.Time `json:"at"`
}

// WarmupPlacementRate is a mailbox's headline deliverability: the inbox rate
// over the trailing window, withheld below the sample floor.
type WarmupPlacementRate struct {
	WindowDays int `json:"window_days"`
	MinSample  int `json:"min_sample"`
	// Scope is which recipients the rate is taken over.
	Scope     string `json:"scope"`
	Delivered int    `json:"delivered"`
	Inbox     int    `json:"inbox"`
	Tabs      int    `json:"tabs"`
	Spam      int    `json:"spam"`
	// InboxRate is nil until Delivered reaches MinSample.
	InboxRate *float64 `json:"inbox_rate"`
	// Band is the health band the rate falls in.
	Band string `json:"band"`
	// OtherDelivered and OtherInboxRate are the other mail hosts left out of a
	// major-scope rate, shown beside it and never judged; nil with none.
	OtherDelivered int      `json:"other_delivered"`
	OtherInboxRate *float64 `json:"other_inbox_rate"`
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
	// PoolType is the pool the mailbox warms in: "premium" or "free".
	PoolType string `json:"pool_type,omitempty"`
	// Source is "cloud" when Warmbly Cloud warms the mailbox and reported
	// this standing, empty for the instance's own pool.
	Source string `json:"source,omitempty"`
	// State is one of the Band* constants.
	State string `json:"state"`
	// Score runs 0 to 100, higher being healthier.
	Score  float64 `json:"score"`
	Reason string  `json:"reason,omitempty"`
	// SpamScore is always 0 on current servers: the accumulating score was
	// retired and the key stays for compatibility. Read Score and Reason.
	SpamScore    int        `json:"spam_score"`
	BlockedUntil *time.Time `json:"blocked_until,omitempty"`
	EvaluatedAt  *time.Time `json:"evaluated_at,omitempty"`
	// Partner diversity counts confirmed warmup deliveries over seven days.
	PartnerMailboxes7d     int `json:"partner_mailboxes_7d"`
	PartnerDomains7d       int `json:"partner_domains_7d"`
	PartnerOrganizations7d int `json:"partner_organizations_7d"`
	// Received7d and Senders7d are the receiving side over the same window:
	// verified warmup arrivals and the distinct partners they came from.
	Received7d int `json:"received_7d"`
	Senders7d  int `json:"senders_7d"`
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

// Campaign returns one campaign's engagement over its whole life, broken down
// by step. Use [AnalyticsService.CampaignRange] to scope it to a period.
func (s *AnalyticsService) Campaign(ctx context.Context, id string, opts ...RequestOption) (*CampaignAnalytics, *Response, error) {
	return fetch[CampaignAnalytics](ctx, s.client, "analytics/campaigns/"+url.PathEscape(id), opts)
}

// CampaignRange is [AnalyticsService.Campaign] scoped to the emails sent
// between two calendar days (UTC, both included). The summary, the step
// performance, the engagement breakdown and the daily series all read the
// same days; TotalContacts and EmailsPending stay campaign-wide, and
// [CampaignAnalytics.DateRange] reports the resolved period. The server needs
// both days or neither (a lone one is a 400), and from must not be after to.
func (s *AnalyticsService) CampaignRange(ctx context.Context, id string, from, to time.Time, opts ...RequestOption) (*CampaignAnalytics, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "from", formatDay(from))
	setNonEmpty(q, "to", formatDay(to))
	return fetch[CampaignAnalytics](ctx, s.client, withQuery("analytics/campaigns/"+url.PathEscape(id), q), opts)
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

// Accounts returns the operational status of every mailbox. The server
// answers in pages of up to [AccountStatusMaxLimit], so this follows the
// cursor until the whole inventory is read; the returned [Response] is the
// last page's. For a very large inventory, or to read only some mailboxes,
// use [AnalyticsService.AccountsPage].
func (s *AnalyticsService) Accounts(ctx context.Context, opts ...RequestOption) ([]AccountStatus, *Response, error) {
	page, err := s.AccountsPage(ctx, nil, opts...)
	if err != nil {
		return nil, nil, err
	}
	out := []AccountStatus{}
	for {
		out = append(out, page.Data...)
		if !page.HasMore() {
			return out, page.Response(), nil
		}
		cursor := page.NextCursor()
		next, err := page.Next(ctx)
		if err != nil {
			return nil, page.Response(), err
		}
		if next.HasMore() && next.NextCursor() == cursor {
			return nil, next.Response(), fmt.Errorf("warmbly: account status pagination repeated cursor %q", cursor)
		}
		page = next
	}
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

// DirectMailAnalytics reports the mail sent by hand from the unified inbox, as
// opposed to campaign mail. Volume is measured from the synced mailboxes and
// Tracking is the opt-in half ([EmailService.SetDirectTracking]); they are kept
// apart because one blended rate would be a lie.
type DirectMailAnalytics struct {
	// Period is [Period7Days], [Period30Days] or [Period90Days].
	Period      string                   `json:"period"`
	Volume      DirectMailVolume         `json:"volume"`
	Tracking    DirectMailTracking       `json:"tracking"`
	DailyTrend  []DirectMailDailyStat    `json:"daily_trend"`
	Mailboxes   []DirectMailMailboxStats `json:"mailboxes"`
	TopContacts []DirectMailContact      `json:"top_contacts"`
}

// DirectMailVolume is how much was actually sent and answered, measured from
// the synced mailbox.
type DirectMailVolume struct {
	Sent     int `json:"sent"`
	Received int `json:"received"`
	// ThreadsStarted counts outbound threads whose first message was yours, and
	// Replied those that got an inbound message back.
	ThreadsStarted int     `json:"threads_started"`
	Replied        int     `json:"replied"`
	ReplyRate      float64 `json:"reply_rate"`
	// Bounced counts the delivery failures that came back. They are excluded
	// from Replied.
	Bounced int `json:"bounced"`
	// MedianReplyMinutes is how long contacts took to answer, across the
	// threads that were answered. Zero when none was.
	MedianReplyMinutes int `json:"median_reply_minutes"`
}

// DirectMailTracking covers the mailboxes that opted into open and click
// tracking. TrackedSent is the denominator of both rates: an untracked send is
// not a failure to open, it is a message nobody asked about.
type DirectMailTracking struct {
	// MailboxesOptedIn of MailboxesTotal says how much of the picture this
	// covers.
	MailboxesOptedIn int `json:"mailboxes_opted_in"`
	MailboxesTotal   int `json:"mailboxes_total"`
	TrackedSent      int `json:"tracked_sent"`
	Opened           int `json:"opened"`
	// MachineOpened are opens by automated fetchers, excluded from Opened and
	// OpenRate.
	MachineOpened int     `json:"machine_opened"`
	Clicked       int     `json:"clicked"`
	OpenRate      float64 `json:"open_rate"`
	ClickRate     float64 `json:"click_rate"`
}

// DirectMailDailyStat is one day of direct-mail volume.
type DirectMailDailyStat struct {
	Date     time.Time `json:"date"`
	Sent     int       `json:"sent"`
	Received int       `json:"received"`
}

// DirectMailMailboxStats is one mailbox's direct-mail volume.
type DirectMailMailboxStats struct {
	EmailAccountID string `json:"email_account_id"`
	Email          string `json:"email"`
	// TrackDirectMail is whether the mailbox opted into tracking.
	TrackDirectMail bool `json:"track_direct_mail"`
	Sent            int  `json:"sent"`
	Received        int  `json:"received"`
}

// DirectMailContact is one correspondent, ranked by how much was sent to them.
type DirectMailContact struct {
	Email    string    `json:"email"`
	Sent     int       `json:"sent"`
	Received int       `json:"received"`
	LastAt   time.Time `json:"last_at"`
}

// Direct returns analytics for the mail sent by hand from the unified inbox over
// a rolling window. Period is [Period7Days], [Period30Days] or [Period90Days];
// anything else, including empty, silently becomes [Period7Days]. Requires the
// view-analytics permission ([PermReadAnalytics] for an API key).
func (s *AnalyticsService) Direct(ctx context.Context, period string, opts ...RequestOption) (*DirectMailAnalytics, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "period", period)
	return fetch[DirectMailAnalytics](ctx, s.client, withQuery("analytics/direct", q), opts)
}

// Kinds of message the automatic inbox tagging recognizes, in
// [InboxTagResult.Kind]. The set may grow, so a value not listed here still
// decodes.
const (
	InboxKindBounceHard      = "bounce_hard"
	InboxKindBounceSoft      = "bounce_soft"
	InboxKindAutoReplyOOO    = "auto_reply_ooo"
	InboxKindAutoReplyTicket = "auto_reply_ticket"
	InboxKindHumanReply      = "human_reply"
	InboxKindColdInbound     = "cold_inbound"
	InboxKindNotification    = "notification"
	InboxKindInternal        = "internal"
)

// Intents of a human reply, in [InboxTagResult.Intent]. Only read when the kind
// is [InboxKindHumanReply]. The set may grow.
const (
	InboxIntentAgreed           = "agreed"
	InboxIntentWantsInfo        = "wants_info"
	InboxIntentWantsPricing     = "wants_pricing"
	InboxIntentNotNow           = "not_now"
	InboxIntentNotInterested    = "not_interested"
	InboxIntentWrongPerson      = "wrong_person"
	InboxIntentOptOut           = "opt_out"
	InboxIntentScheduling       = "scheduling"
	InboxIntentInProgress       = "in_progress"
	InboxIntentQuestionAnswered = "question_answered"
	InboxIntentUnclear          = "unclear"
)

// Priorities in [InboxTagResult.Priority]. The set may grow.
const (
	InboxPriorityNow      = "now"
	InboxPriorityToday    = "today"
	InboxPriorityWhenever = "whenever"
	InboxPriorityIgnore   = "ignore"
)

// InboxTagResult is what automatic inbox tagging decided about one inbound
// message, and how sure it was.
type InboxTagResult struct {
	ID        string `json:"id"`
	MessageID string `json:"message_id"`
	ThreadID  string `json:"thread_id"`
	// Kind is one of the InboxKind* constants and KindSource says who decided:
	// "header" (a standard auto-reply or bounce header) or "model".
	Kind           string  `json:"kind"`
	KindConfidence float64 `json:"kind_confidence"`
	KindSource     string  `json:"kind_source"`
	// Intent is one of the InboxIntent* constants, meaningful for a human
	// reply.
	Intent           string  `json:"intent"`
	IntentConfidence float64 `json:"intent_confidence"`
	// Relevance is a 0 to 100 score and Priority one of the InboxPriority*
	// constants.
	Relevance int    `json:"relevance"`
	Priority  string `json:"priority"`
	// NeedsReview is set when the verdict was not confident enough to trust,
	// with ReviewReason saying why.
	NeedsReview  bool   `json:"needs_review"`
	ReviewReason string `json:"review_reason"`
	// Labels are the conversation labels the verdict applied.
	Labels []string `json:"labels"`
	// Answers are the model's raw per-question answers, an object whose keys
	// are not part of this SDK's contract.
	Answers json.RawMessage `json:"answers"`
	Model   string          `json:"model"`
	// InputTokens is what the judgment consumed.
	InputTokens int `json:"input_tokens"`
	// Actions are what the workspace's switches let this verdict do: "hold",
	// "stop", "task" or "suppress".
	Actions []string `json:"actions"`
	// ReturnDate is the out-of-office return date (YYYY-MM-DD) the model was
	// asked to confirm, nil when it was not asked.
	ReturnDate *string   `json:"return_date"`
	CreatedAt  time.Time `json:"created_at"`
}

// InboxTagSummary counts the verdicts behind an [InboxTaggingPage].
type InboxTagSummary struct {
	Total       int `json:"total"`
	NeedsReview int `json:"needs_review"`
	// FromOffline counts verdicts made without the model, from headers alone.
	FromOffline int `json:"from_offline"`
	// Acted counts verdicts that were allowed to take an action.
	Acted int `json:"acted"`
}

// InboxTaggingPage is one page of the automatic-tagging review list. Beyond the
// rows it says whether the feature is on and carries the totals.
type InboxTaggingPage struct {
	Page[InboxTagResult]

	// Enabled says whether automatic tagging is switched on for this instance,
	// so an empty list can be explained rather than read as "nothing found".
	Enabled bool `json:"enabled"`
	// Total is the number of rows matching the filter, across every page.
	Total   int             `json:"total"`
	Summary InboxTagSummary `json:"summary"`
}

// InboxTaggingParams filters and paginates the review list. Limit may be 1 to
// 200 and defaults to 50. The cursor is opaque.
type InboxTaggingParams struct {
	ListOptions
	// NeedsReview restricts the list to verdicts flagged for a person's review.
	NeedsReview bool
}

func (p *InboxTaggingParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	if p.NeedsReview {
		q.Set("needs_review", "true")
	}
	return q
}

// InboxTagging returns the review list of automatic inbox tagging: what it
// decided about each inbound message and how sure it was, newest first. It is
// read-only; the point of the review phase is that a person watches it decide
// before it is allowed to act. Pages are cursor-based, and the filter and the
// totals ride the first and every following page. Requires the view-analytics
// permission ([PermReadAnalytics] for an API key).
func (s *AnalyticsService) InboxTagging(ctx context.Context, params *InboxTaggingParams, opts ...RequestOption) (*InboxTaggingPage, error) {
	return fetchInboxTagging(ctx, s.client, params, opts)
}

func fetchInboxTagging(ctx context.Context, c *Client, params *InboxTaggingParams, opts []RequestOption) (*InboxTaggingPage, error) {
	page := &InboxTaggingPage{}
	resp, err := c.get(ctx, withQuery("analytics/inbox-tagging", params.values()), page, opts...)
	if err != nil {
		return nil, err
	}
	page.resp = resp
	page.fetch = func(ctx context.Context, cursor string) (*Page[InboxTagResult], error) {
		var next InboxTaggingParams
		if params != nil {
			next = *params
		}
		next.Cursor = cursor
		p, err := fetchInboxTagging(ctx, c, &next, opts)
		if err != nil {
			return nil, err
		}
		return &p.Page, nil
	}
	return page, nil
}

// formatDay renders a calendar day the way the analytics endpoints expect. A
// zero time renders empty so the caller can omit the parameter.
func formatDay(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}
