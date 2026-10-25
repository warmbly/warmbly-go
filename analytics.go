package warmbly

import (
	"context"
	"encoding/json"
	"errors"
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
	// scheduler's own clamps; nil when it could not be computed or the caller
	// is limited to some resources.
	CapacityToday *WorkspaceSendCapacity `json:"capacity_today,omitempty"`
	// Scope echoes the campaign filter the campaign-derived sections were
	// computed for; nil when they cover the whole workspace.
	Scope *DashboardScope `json:"scope,omitempty"`
}

// WorkspaceSendCapacity is how much cold mail the workspace can still send
// today.
type WorkspaceSendCapacity struct {
	// Capacity is today's cold sends across every mailbox that can send;
	// RemainingToday is what is left of it.
	Capacity       int `json:"capacity"`
	RemainingToday int `json:"remaining_today"`
	// ConfiguredCeiling is the same mailboxes' own caps added up.
	ConfiguredCeiling int `json:"configured_ceiling"`
	// Mailboxes is how many mailboxes contribute; Held is how many are
	// attached but cannot send today.
	Mailboxes int `json:"mailboxes"`
	Held      int `json:"held"`
}

// DashboardScope echoes a resolved dashboard filter.
type DashboardScope struct {
	Campaigns []DashboardScopeItem `json:"campaigns"`
	Folders   []DashboardScopeItem `json:"folders"`
	// CampaignCount is the number of distinct campaigns the filter covers.
	CampaignCount int `json:"campaign_count"`
}

// DashboardScopeItem names one campaign or folder in a [DashboardScope].
type DashboardScopeItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// DashboardParams narrows [AnalyticsService.DashboardFor] to some campaigns.
// Each id list takes at most 100 ids; a folder stands in for any number of
// campaigns. The workspace-wide sections (mailbox counts, capacity) are not
// narrowed.
type DashboardParams struct {
	// Period is [Period7Days], [Period30Days] or [Period90Days].
	Period      string
	CampaignIDs []string
	FolderIDs   []string
}

func (p *DashboardParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	setNonEmpty(q, "period", p.Period)
	setNonEmpty(q, "campaign_ids", strings.Join(p.CampaignIDs, ","))
	setNonEmpty(q, "folder_ids", strings.Join(p.FolderIDs, ","))
	return q
}

// OverallStats are the headline counters for the dashboard window.
type OverallStats struct {
	TotalEmailsSent int64 `json:"total_emails_sent"`
	TotalOpens      int64 `json:"total_opens"`
	// MachineOpens are opens attributed to a mail-privacy proxy rather than a
	// human, and are excluded from OpenRate.
	MachineOpens int64 `json:"machine_opens"`
	TotalClicks  int64 `json:"total_clicks"`
	// MachineClicks counts the contacts whose only clicks on a step came from
	// automated fetchers (security gateways walking the links). They are not
	// part of TotalClicks, which only ever counts a person's click.
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
	// SenderID and SenderEmail name the mailbox the step went out from; a
	// caller limited to some mailboxes does not see a sender outside them.
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
	// PositiveReplies is the subset of Replies judged positive. Only the
	// campaign daily series carries it; it decodes as zero elsewhere.
	PositiveReplies int64 `json:"positive_replies,omitempty"`
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
	// Surfaces is keyed by device and app or webmail together: a device type,
	// its "_app" form ("mobile_app", "desktop_app", "tablet_app"), "webmail",
	// or "hidden" for a fetch through a mailbox provider's image proxy.
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
	MachineClicks int64 `json:"machine_clicks"`
	Replies       int64 `json:"replies"`
	// PositiveReplies is the subset of Replies the classifier judged positive,
	// one per answered step; InterestedLeads counts distinct contacts with a
	// positive reply.
	PositiveReplies int64 `json:"positive_replies"`
	InterestedLeads int64 `json:"interested_leads"`
	Bounces         int64 `json:"bounces"`
	Unsubscribes    int64 `json:"unsubscribes"`
	// ReplyBreakdown splits Replies by classification, plus the automated
	// replies that never count as one.
	ReplyBreakdown ReplyBreakdown `json:"reply_breakdown"`
	OpenRate       float64        `json:"open_rate"`
	ClickRate      float64        `json:"click_rate"`
	ReplyRate      float64        `json:"reply_rate"`
	// PositiveReplyRate is a percentage of EmailsSent.
	PositiveReplyRate float64 `json:"positive_reply_rate"`
	BounceRate        float64 `json:"bounce_rate"`
}

// ReplyBreakdown is a cohort's replies by classification. The human classes
// plus Unclassified add up to Replies; OutOfOffice and AutoReply are the
// automated answers, which are not part of Replies.
type ReplyBreakdown struct {
	Positive     int64 `json:"positive"`
	Neutral      int64 `json:"neutral"`
	Negative     int64 `json:"negative"`
	Unsubscribe  int64 `json:"unsubscribe"`
	Unclassified int64 `json:"unclassified"`
	OutOfOffice  int64 `json:"out_of_office"`
	AutoReply    int64 `json:"auto_reply"`
}

// StepAnalytics is one sequence step's engagement.
type StepAnalytics struct {
	StepID     string `json:"step_id"`
	Name       string `json:"name"`
	Position   int    `json:"position"`
	EmailsSent int64  `json:"emails_sent"`
	Opens      int64  `json:"opens"`
	// MachineOpens is the subset of Opens from automated fetchers; human opens
	// are Opens - MachineOpens.
	MachineOpens int64 `json:"machine_opens"`
	Clicks       int64 `json:"clicks"`
	// MachineClicks counts this step's contacts whose only clicks were
	// automated; they are not part of Clicks.
	MachineClicks   int64 `json:"machine_clicks"`
	Replies         int64 `json:"replies"`
	PositiveReplies int64 `json:"positive_replies"`
	Bounces         int64 `json:"bounces"`
	// The rates are percentages of this step's own EmailsSent, so steps that
	// reached different numbers of contacts still compare.
	OpenRate          float64 `json:"open_rate"`
	ClickRate         float64 `json:"click_rate"`
	ReplyRate         float64 `json:"reply_rate"`
	PositiveReplyRate float64 `json:"positive_reply_rate"`
	BounceRate        float64 `json:"bounce_rate"`
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
	// range: the other half of the exchange.
	TotalReceived int64   `json:"total_received"`
	AverageDaily  float64 `json:"average_daily"`
	ReplyRate     float64 `json:"reply_rate"`
	// TargetProgress is actual sends divided by the planned target volume for
	// the active days.
	TargetProgress float64 `json:"target_progress"`
	DaysActive     int     `json:"days_active"`
}

// WarmupDailyStat is one day of warmup volume against its target.
type WarmupDailyStat struct {
	Date          string `json:"date"`
	EmailsSent    int64  `json:"emails_sent"`
	EmailsReplied int64  `json:"emails_replied"`
	// EmailsReceived is verified warmup mail that arrived that day.
	EmailsReceived int64 `json:"emails_received"`
	TargetVolume   int64 `json:"target_volume"`
	// Active is whether the mailbox had a warmup plan that day. A day with
	// arrivals but no plan is listed but does not count as a day active.
	Active bool `json:"active"`
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
	// IntentAutomated counts machine replies that are not vacation notices:
	// autoresponders, ticket acknowledgements, bounces and delivery reports.
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

// WarmupHostPlacement is the name the server gives [WarmupDomainPlacement]: one
// recipient mail host's warmup placement rollup.
type WarmupHostPlacement = WarmupDomainPlacement

// ProviderPlacement is one recipient provider's seed placement rollup: where
// the seed messages landed.
type ProviderPlacement struct {
	// Provider is the seed's host family and Label its display name.
	Provider   string `json:"provider"`
	Label      string `json:"label"`
	Samples    int64  `json:"samples"`
	Inbox      int64  `json:"inbox"`
	Promotions int64  `json:"promotions"`
	Spam       int64  `json:"spam"`
	Other      int64  `json:"other"`
	// Missing counts copies that never arrived.
	Missing   int64   `json:"missing"`
	InboxRate float64 `json:"inbox_rate"`
	SpamRate  float64 `json:"spam_rate"`
}

// WarmupDomainPlacement is one recipient mail host's warmup placement rollup.
// Delivered counts verified warmup arrivals; Spam the ones filed into junk.
//
// The server keys this rollup by mail host, not by domain: Provider is the host
// and Label its display name. Domain is no longer sent and decodes empty.
type WarmupDomainPlacement struct {
	Provider string `json:"provider"`
	Label    string `json:"label"`
	// Deprecated: the server no longer reports a recipient domain; use
	// Provider and Label.
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
	// Placement is where the mailbox's warmup mail landed over the trailing
	// week; it also caps Health.Score. nil when nothing was delivered in the
	// window.
	Placement *WarmupPlacementRate `json:"warmup_placement,omitempty"`
	// SendRecoveryReason names a send safety hold ("authentication",
	// "permanent", "conflict" or "unknown"); empty when sending is not held.
	SendRecoveryReason string `json:"send_recovery_reason,omitempty"`
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
	// PartnerLimit is present while today's target is capped by how many
	// partners the mailbox can still reach. Only the single-mailbox read
	// ([AnalyticsService.Account]) computes it.
	PartnerLimit *WarmupPartnerLimit `json:"partner_limit,omitempty"`
	// SendFailure is present while the newest warmup send failed and no later
	// one was confirmed delivered.
	SendFailure *WarmupSendFailure `json:"send_failure,omitempty"`
	// NextSendAt is when the parked warmup wakeup fires; nil while nothing is
	// queued.
	NextSendAt *time.Time `json:"next_send_at,omitempty"`
}

// WarmupPartnerLimit explains a target held below the ramp because a mailbox
// never writes to the same partner twice in a day.
type WarmupPartnerLimit struct {
	// Reachable is how many partners are available to it today, including any
	// it already wrote to.
	Reachable int `json:"reachable"`
	// RampTarget is what the ramp alone would send today.
	RampTarget int `json:"ramp_target"`
}

// WarmupSendFailure is why a warmup send failed: the server's answer when it
// gave one.
type WarmupSendFailure struct {
	Message string    `json:"message"`
	At      time.Time `json:"at"`
	// Kind classifies the failure; treat it as an open set.
	Kind           string     `json:"kind,omitempty"`
	FirstFailureAt *time.Time `json:"first_failure_at,omitempty"`
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
	// Source is "cloud" when Warmbly Cloud warms the mailbox and reported this
	// standing; empty for this instance's own pool.
	Source string `json:"source,omitempty"`
	// State is one of the Band* constants.
	State string `json:"state"`
	// Score runs 0 to 100, higher being healthier.
	Score  float64 `json:"score"`
	Reason string  `json:"reason,omitempty"`
	// SpamScore is always 0: the server retired the accumulating score and
	// keeps the key only so older clients do not break. Read Score and Reason.
	SpamScore    int        `json:"spam_score"`
	BlockedUntil *time.Time `json:"blocked_until,omitempty"`
	EvaluatedAt  *time.Time `json:"evaluated_at,omitempty"`
	// PartnerMailboxes7d, PartnerDomains7d and PartnerOrganizations7d count the
	// distinct partners of confirmed warmup deliveries over seven days.
	PartnerMailboxes7d     int `json:"partner_mailboxes_7d"`
	PartnerDomains7d       int `json:"partner_domains_7d"`
	PartnerOrganizations7d int `json:"partner_organizations_7d"`
	// Received7d and Senders7d are the receiving side over the same window:
	// verified warmup arrivals and the distinct partners they came from.
	Received7d int `json:"received_7d"`
	Senders7d  int `json:"senders_7d"`
}

// WarmupPlacementRate is a mailbox's headline warmup deliverability: the inbox
// rate over the trailing window, withheld below the sample floor.
type WarmupPlacementRate struct {
	WindowDays int `json:"window_days"`
	MinSample  int `json:"min_sample"`
	// Scope is which recipients the rate is taken over ("major": Google,
	// Microsoft and Yahoo).
	Scope     string `json:"scope"`
	Delivered int    `json:"delivered"`
	Inbox     int    `json:"inbox"`
	Tabs      int    `json:"tabs"`
	Spam      int    `json:"spam"`
	// InboxRate is a percentage, nil until Delivered reaches MinSample.
	InboxRate *float64 `json:"inbox_rate"`
	// Band is one of the WarmupPlacementBand* constants.
	Band   string             `json:"band"`
	Metric *ObservationMetric `json:"metric"`
	// OtherDelivered and OtherInboxRate are the other mail hosts left out of a
	// major-scope rate, shown beside it and never judged.
	OtherDelivered int      `json:"other_delivered"`
	OtherInboxRate *float64 `json:"other_inbox_rate"`
}

// Bands a mailbox's rolling warmup inbox rate falls into, in
// [WarmupPlacementRate.Band]. Treat the set as open.
const (
	WarmupPlacementBandGood       = "good"
	WarmupPlacementBandFair       = "fair"
	WarmupPlacementBandPoor       = "poor"
	WarmupPlacementBandCollecting = "collecting"
	WarmupPlacementBandNone       = "none"
)

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
	// Operation is what the mailbox was doing when it failed: "sync", "send"
	// or "unknown". Empty when the server did not say.
	Operation string `json:"operation,omitempty"`
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
	Total  int `json:"total"`
	Active int `json:"active"`
	Paused int `json:"paused"`
	Draft  int `json:"draft"`
	// EmailsSent counts sent email steps inside the usage Period.
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

// DashboardFor is [AnalyticsService.Dashboard] narrowed to some campaigns or
// folders. The campaign-derived sections (stats, top campaigns, activity,
// trend) are computed for the filter, which Scope echoes; a restricted caller
// may only name campaigns and folders it was granted (a 404 otherwise).
func (s *AnalyticsService) DashboardFor(ctx context.Context, params *DashboardParams, opts ...RequestOption) (*DashboardAnalytics, *Response, error) {
	return fetch[DashboardAnalytics](ctx, s.client, withQuery("analytics/dashboard", params.values()), opts)
}

// Campaign returns one campaign's all-time engagement, broken down by step.
// Use [AnalyticsService.CampaignBetween] for a date range.
func (s *AnalyticsService) Campaign(ctx context.Context, id string, opts ...RequestOption) (*CampaignAnalytics, *Response, error) {
	return fetch[CampaignAnalytics](ctx, s.client, "analytics/campaigns/"+url.PathEscape(id), opts)
}

// CampaignBetween returns one campaign's engagement for the whole days from
// through to (UTC, to included). The server requires both or neither, so a
// zero from or to is a client-side error; use [AnalyticsService.Campaign] for
// all time.
func (s *AnalyticsService) CampaignBetween(ctx context.Context, id string, from, to time.Time, opts ...RequestOption) (*CampaignAnalytics, *Response, error) {
	if from.IsZero() || to.IsZero() {
		return nil, nil, errors.New("warmbly: CampaignBetween needs both from and to; use Campaign for all time")
	}
	q := url.Values{"from": {formatDay(from)}, "to": {formatDay(to)}}
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

// Accounts returns the first page of mailbox statuses, up to the server's
// default of 1000. The endpoint is paginated: a workspace with more mailboxes
// than that has the rest behind a cursor, which this method does not follow.
// Use [AnalyticsService.AccountsPage] to walk every mailbox or to ask for a
// chosen few.
func (s *AnalyticsService) Accounts(ctx context.Context, opts ...RequestOption) ([]AccountStatus, *Response, error) {
	return fetchData[AccountStatus](ctx, s.client, "analytics/accounts", opts)
}

// AccountsParams selects and paginates mailbox statuses. The page size
// defaults to 1000, which is also the cap.
type AccountsParams struct {
	ListOptions
	// EmailIDs limits the page to these mailboxes, at most 200; an invalid or
	// over-limit list is a 400.
	EmailIDs []string
}

func (p *AccountsParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	setNonEmpty(q, "email_ids", strings.Join(p.EmailIDs, ","))
	return q
}

// AccountsPage returns one page of mailbox statuses; iterate it with
// [Page.All] to cover the whole inventory. Per-mailbox reads are batched across
// the page, so its cost does not grow with the total inventory.
func (s *AnalyticsService) AccountsPage(ctx context.Context, params *AccountsParams, opts ...RequestOption) (*Page[AccountStatus], error) {
	return listJSON[AccountStatus](ctx, s.client, "analytics/accounts", params.values(), opts...)
}

// Account returns one mailbox's operational status, including its warmup
// ramp, any cold-ramp ceiling and any lifecycle hold.
func (s *AnalyticsService) Account(ctx context.Context, id string, opts ...RequestOption) (*AccountStatus, *Response, error) {
	return fetch[AccountStatus](ctx, s.client, "analytics/accounts/"+url.PathEscape(id), opts)
}

// WarmupPlacementParams selects a warmup placement report. The window is whole
// UTC days, to included.
type WarmupPlacementParams struct {
	// EmailID limits the report to one mailbox; empty reports the whole
	// workspace, with a per-mailbox table.
	EmailID string
	// From and To bound the report. A zero To is today and a zero From is the
	// 30 days ending on To.
	From time.Time
	To   time.Time
}

func (p *WarmupPlacementParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	setNonEmpty(q, "email_id", p.EmailID)
	setNonEmpty(q, "from", formatDay(p.From))
	setNonEmpty(q, "to", formatDay(p.To))
	return q
}

// WarmupPlacementCounts is where a set of verified warmup deliveries landed.
// The inbox rate counts category tabs as inbox: the mail was delivered and not
// filtered.
type WarmupPlacementCounts struct {
	// Sent is warmup mail the mailbox sent; Delivered is what recipients
	// verifiably received (inbox, tabs and spam).
	Sent      int `json:"sent"`
	Delivered int `json:"delivered"`
	Inbox     int `json:"inbox"`
	Tabs      int `json:"tabs"`
	Spam      int `json:"spam"`
	Unknown   int `json:"unknown"`
	Archived  int `json:"archived"`
	Custom    int `json:"custom"`
	// Instrumented counts receipts classified from a live observation and
	// Observed all receipts seen; LegacyClassified are the receipts classified
	// before observation was recorded.
	Instrumented     int `json:"instrumented_receipts"`
	Observed         int `json:"observed_receipts"`
	LegacyClassified int `json:"legacy_uninstrumented_receipts"`
	// RescueRequested is spam placements the recipient's mailbox was told to
	// move back to the inbox; the outcome counters are what its worker
	// reported, and RescueUnreported those no worker has answered yet.
	RescueRequested  int `json:"rescue_requested"`
	Rescued          int `json:"rescued"`
	RescueConfirmed  int `json:"rescue_confirmed"`
	RescueMissing    int `json:"rescue_missing"`
	RescueFailed     int `json:"rescue_failed"`
	RescueUnreported int `json:"rescue_unreported"`
	// Unconfirmed is mail sent more than 24 hours ago that no recipient has
	// reported seeing.
	Unconfirmed   int                `json:"unconfirmed"`
	NonSpamMetric *ObservationMetric `json:"non_spam_metric"`
	// InboxRate and SpamRate are percentages, nil until something is
	// delivered.
	InboxRate *float64 `json:"inbox_rate"`
	SpamRate  *float64 `json:"spam_rate"`
}

// WarmupPlacementGroupCounts is one recipient group's share of a day.
type WarmupPlacementGroupCounts struct {
	// Group is "google", "microsoft", "yahoo" or "other".
	Group    string `json:"group"`
	Inbox    int    `json:"inbox"`
	Tabs     int    `json:"tabs"`
	Spam     int    `json:"spam"`
	Rescued  int    `json:"rescued"`
	Unknown  int    `json:"unknown"`
	Archived int    `json:"archived"`
	Custom   int    `json:"custom"`

	RescueConfirmed int `json:"rescue_confirmed"`
	RescueMissing   int `json:"rescue_missing"`
	RescueFailed    int `json:"rescue_failed"`
}

// WarmupPlacementDay is one UTC day of warmup placement.
type WarmupPlacementDay struct {
	// Date is a calendar day formatted YYYY-MM-DD.
	Date string `json:"date"`
	WarmupPlacementCounts
	// RollingInboxRate is the trailing-window rate ending on this day, nil
	// below the sample floor.
	RollingInboxRate *float64                     `json:"rolling_inbox_rate"`
	Groups           []WarmupPlacementGroupCounts `json:"groups"`
}

// WarmupPlacementHost is one mail host inside a recipient group.
type WarmupPlacementHost struct {
	Host string `json:"host"`
	WarmupPlacementCounts
}

// WarmupPlacementProvider is the window's placement at one recipient group.
type WarmupPlacementProvider struct {
	// Group is "google", "microsoft", "yahoo" or "other".
	Group string `json:"group"`
	WarmupPlacementCounts
	Hosts []WarmupPlacementHost `json:"hosts"`
}

// WarmupPlacementMailbox is one sender's row in the workspace report.
type WarmupPlacementMailbox struct {
	EmailAccountID string `json:"email_account_id"`
	Email          string `json:"email"`
	WarmupPlacementCounts
	Rate WarmupPlacementRate `json:"rate"`
	// DailyInboxRate follows the report's days; an entry is nil on a day with
	// no deliveries.
	DailyInboxRate []*float64 `json:"daily_inbox_rate"`
}

// WarmupPlacementReport is where warmup mail landed over a date range, for one
// mailbox or for the whole workspace.
type WarmupPlacementReport struct {
	// EmailAccountID is set on a single-mailbox report.
	EmailAccountID *string                   `json:"email_account_id,omitempty"`
	DateRange      DateRange                 `json:"date_range"`
	Summary        WarmupPlacementCounts     `json:"summary"`
	Rate           WarmupPlacementRate       `json:"rate"`
	Daily          []WarmupPlacementDay      `json:"daily"`
	Providers      []WarmupPlacementProvider `json:"providers"`
	// Mailboxes is present on the workspace report only, worst rate first.
	Mailboxes []WarmupPlacementMailbox `json:"mailboxes,omitempty"`
}

// WarmupPlacement reports where warmup mail landed (inbox, category tabs,
// spam) per day and per recipient provider, for one mailbox or the whole
// workspace. Requires an organization to be selected.
func (s *AnalyticsService) WarmupPlacement(ctx context.Context, params *WarmupPlacementParams, opts ...RequestOption) (*WarmupPlacementReport, *Response, error) {
	return fetch[WarmupPlacementReport](ctx, s.client, withQuery("analytics/warmup/placement", params.values()), opts)
}

// DirectMailVolume is the "how much did we send and hear back" half of
// [DirectMailAnalytics], measured from the synced mailbox, so it covers mail
// written in Gmail or on a phone as well.
type DirectMailVolume struct {
	Sent     int `json:"sent"`
	Received int `json:"received"`
	// ThreadsStarted counts outbound threads whose first message was ours and
	// Replied those that got an inbound message back.
	ThreadsStarted int     `json:"threads_started"`
	Replied        int     `json:"replied"`
	ReplyRate      float64 `json:"reply_rate"`
	// Bounced counts the delivery failures that came back; they are excluded
	// from Replied.
	Bounced int `json:"bounced"`
	// MedianReplyMinutes is how long contacts took to answer, across the
	// threads that were answered; zero when none was.
	MedianReplyMinutes int `json:"median_reply_minutes"`
}

// DirectMailTracking is the opt-in half of [DirectMailAnalytics]: opens and
// clicks, for mail sent through Warmbly by mailboxes with tracking switched on
// ([EmailService.SetDirectTracking]), and only from the moment it was
// switched on. TrackedSent is the denominator for both rates.
type DirectMailTracking struct {
	MailboxesOptedIn int     `json:"mailboxes_opted_in"`
	MailboxesTotal   int     `json:"mailboxes_total"`
	TrackedSent      int     `json:"tracked_sent"`
	Opened           int     `json:"opened"`
	MachineOpened    int     `json:"machine_opened"`
	Clicked          int     `json:"clicked"`
	OpenRate         float64 `json:"open_rate"`
	ClickRate        float64 `json:"click_rate"`
}

// DirectMailDay is one day of hand-written mail volume.
type DirectMailDay struct {
	Date     time.Time `json:"date"`
	Sent     int       `json:"sent"`
	Received int       `json:"received"`
}

// DirectMailMailbox is one mailbox's hand-written mail volume.
type DirectMailMailbox struct {
	EmailAccountID  string `json:"email_account_id"`
	Email           string `json:"email"`
	TrackDirectMail bool   `json:"track_direct_mail"`
	Sent            int    `json:"sent"`
	Received        int    `json:"received"`
}

// DirectMailContact is one correspondent, ranked by how much was sent to them.
type DirectMailContact struct {
	Email    string    `json:"email"`
	Sent     int       `json:"sent"`
	Received int       `json:"received"`
	LastAt   time.Time `json:"last_at"`
}

// DirectMailAnalytics reports on mail written by hand rather than sent by a
// campaign. Volume and replies (from the synced mailbox) and opens and clicks
// (from tracked sends) cover different sets of messages, so they are kept
// apart and never blended into one rate.
type DirectMailAnalytics struct {
	// Period is [Period7Days], [Period30Days] or [Period90Days].
	Period      string              `json:"period"`
	Volume      DirectMailVolume    `json:"volume"`
	Tracking    DirectMailTracking  `json:"tracking"`
	DailyTrend  []DirectMailDay     `json:"daily_trend"`
	Mailboxes   []DirectMailMailbox `json:"mailboxes"`
	TopContacts []DirectMailContact `json:"top_contacts"`
}

// Direct returns the hand-written mail overview for a rolling window. Period is
// [Period7Days], [Period30Days] or [Period90Days]; anything else falls back to
// [Period7Days].
func (s *AnalyticsService) Direct(ctx context.Context, period string, opts ...RequestOption) (*DirectMailAnalytics, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "period", period)
	return fetch[DirectMailAnalytics](ctx, s.client, withQuery("analytics/direct", q), opts)
}

// InboxTagRow is one reply the automatic inbox tagging classified, with how
// sure it was and what its verdict was allowed to do.
type InboxTagRow struct {
	ID        string `json:"id"`
	MessageID string `json:"message_id"`
	ThreadID  string `json:"thread_id"`
	// Kind and Intent are the classification; treat them as open sets.
	Kind             string   `json:"kind"`
	KindConfidence   float64  `json:"kind_confidence"`
	KindSource       string   `json:"kind_source"`
	Intent           string   `json:"intent"`
	IntentConfidence float64  `json:"intent_confidence"`
	Relevance        int      `json:"relevance"`
	Priority         string   `json:"priority"`
	NeedsReview      bool     `json:"needs_review"`
	ReviewReason     string   `json:"review_reason"`
	Labels           []string `json:"labels"`
	// Answers holds the answers to the built-in and workspace questions, keyed
	// by question id.
	Answers     json.RawMessage `json:"answers"`
	Model       string          `json:"model"`
	InputTokens int             `json:"input_tokens"`
	// Actions is what the workspace's switches let this verdict do.
	Actions []string `json:"actions"`
	// ReturnDate is the out-of-office return date the model was asked to
	// confirm (YYYY-MM-DD); nil when it was not asked.
	ReturnDate *string `json:"return_date"`
	CreatedAt  string  `json:"created_at"`
}

// InboxTagSummary counts the workspace's tagged replies.
type InboxTagSummary struct {
	Total       int `json:"total"`
	NeedsReview int `json:"needs_review"`
	FromOffline int `json:"from_offline"`
	Acted       int `json:"acted"`
}

// InboxTagReview is one page of automatic inbox tagging verdicts.
type InboxTagReview struct {
	// Enabled says whether tagging is switched on for the instance, so an
	// empty list can be explained.
	Enabled    bool            `json:"enabled"`
	Data       []InboxTagRow   `json:"data"`
	Total      int             `json:"total"`
	Summary    InboxTagSummary `json:"summary"`
	Pagination Pagination      `json:"pagination"`
}

// InboxTagReviewParams paginates [AnalyticsService.InboxTagging]. The page size
// defaults to 50 and is capped at 200.
type InboxTagReviewParams struct {
	ListOptions
	// NeedsReviewOnly keeps only the verdicts flagged for review.
	NeedsReviewOnly bool
}

func (p *InboxTagReviewParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	if p.NeedsReviewOnly {
		q.Set("needs_review", "true")
	}
	return q
}

// InboxTagging returns the review surface of automatic inbox tagging: what it
// decided for each reply, how sure it was and what it was allowed to do.
// Read-only. The result carries its own cursor in Pagination.NextCursor; pass
// it back in [InboxTagReviewParams.Cursor] for the next page.
func (s *AnalyticsService) InboxTagging(ctx context.Context, params *InboxTagReviewParams, opts ...RequestOption) (*InboxTagReview, *Response, error) {
	out, resp, err := fetch[InboxTagReview](ctx, s.client, withQuery("analytics/inbox-tagging", params.values()), opts)
	if err != nil {
		return nil, resp, err
	}
	if out.Data == nil {
		out.Data = []InboxTagRow{}
	}
	return out, resp, nil
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
