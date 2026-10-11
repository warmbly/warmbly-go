package warmbly

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"time"
)

// Recipient groups a warmup placement report is broken down by, returned in
// [WarmupPlacementProvider.Group] and [WarmupPlacementGroupCounts.Group].
const (
	WarmupRecipientGoogle    = "google"
	WarmupRecipientMicrosoft = "microsoft"
	WarmupRecipientYahoo     = "yahoo"
	WarmupRecipientOther     = "other"
)

// Bands a mailbox's rolling warmup inbox rate falls into, returned in
// [WarmupPlacementRate.Band].
const (
	WarmupPlacementBandGood = "good"
	WarmupPlacementBandFair = "fair"
	WarmupPlacementBandPoor = "poor"
	// WarmupPlacementBandCollecting has deliveries but fewer than the sample
	// floor, so no rate yet.
	WarmupPlacementBandCollecting = "collecting"
	WarmupPlacementBandNone       = "none"
)

// WarmupPlacementScopeMajor is the [WarmupPlacementRate.Scope] of a headline
// rate taken over Google, Microsoft and Yahoo recipients only.
const WarmupPlacementScopeMajor = "major"

// WarmupPlacementParams selects a warmup placement report. Zero values use
// the server defaults: the whole workspace, over the 30 days ending today.
type WarmupPlacementParams struct {
	// EmailID narrows the report to one mailbox.
	EmailID string
	// From and To are whole UTC days. A zero To is today; a zero From is 29
	// days before To.
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

// WarmupPlacementReport is where warmup mail landed (inbox, Gmail tabs,
// spam) over a date range, for one mailbox or for the whole workspace.
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

// WarmupPlacementCounts is where a set of verified warmup deliveries landed.
// The inbox rate counts Gmail tabs as inbox: the mail was delivered and not
// filtered.
type WarmupPlacementCounts struct {
	// Sent is warmup mail the mailbox sent; Delivered is what recipients
	// verifiably received (inbox + tabs + spam).
	Sent      int `json:"sent"`
	Delivered int `json:"delivered"`
	Inbox     int `json:"inbox"`
	Tabs      int `json:"tabs"`
	Spam      int `json:"spam"`
	Unknown   int `json:"unknown"`
	Archived  int `json:"archived"`
	Custom    int `json:"custom"`
	// Instrumented, Observed and LegacyClassified split the observed
	// receipts by how they were recorded.
	Instrumented     int `json:"instrumented_receipts"`
	Observed         int `json:"observed_receipts"`
	LegacyClassified int `json:"legacy_uninstrumented_receipts"`
	// RescueRequested is spam placements the recipient's mailbox was told to
	// move back to the inbox; RescueConfirmed is how many were confirmed
	// moved, nil when that is not tracked.
	RescueRequested int                `json:"rescue_requested"`
	RescueConfirmed *int               `json:"rescue_confirmed"`
	NonSpamMetric   *ObservationMetric `json:"non_spam_metric"`
	// Rescued is the same count as RescueRequested, kept for older clients.
	Rescued int `json:"rescued"`
	// Unconfirmed is mail sent more than 24 hours ago that no recipient has
	// reported seeing.
	Unconfirmed int `json:"unconfirmed"`
	// InboxRate and SpamRate are percentages (0 to 100), nil until something
	// is delivered.
	InboxRate *float64 `json:"inbox_rate"`
	SpamRate  *float64 `json:"spam_rate"`
}

// WarmupPlacementRate is a mailbox's headline warmup deliverability: the
// inbox rate over the trailing window, withheld below the sample floor.
type WarmupPlacementRate struct {
	WindowDays int `json:"window_days"`
	MinSample  int `json:"min_sample"`
	// Scope is which recipients the rate is taken over, normally
	// [WarmupPlacementScopeMajor].
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
	// OtherDelivered and OtherInboxRate are the other mail hosts left out of
	// a major-scope rate, shown beside it and never judged; the rate is nil
	// with none.
	OtherDelivered int      `json:"other_delivered"`
	OtherInboxRate *float64 `json:"other_inbox_rate"`
}

// WarmupPlacementDay is one UTC day of warmup placement.
type WarmupPlacementDay struct {
	// Date is YYYY-MM-DD.
	Date string `json:"date"`
	WarmupPlacementCounts
	// RollingInboxRate is the trailing-window rate ending on this day, nil
	// below the sample floor.
	RollingInboxRate *float64                     `json:"rolling_inbox_rate"`
	Groups           []WarmupPlacementGroupCounts `json:"groups"`
}

// WarmupPlacementGroupCounts is one recipient group's share of a day.
type WarmupPlacementGroupCounts struct {
	// Group is one of the WarmupRecipient* constants.
	Group    string `json:"group"`
	Inbox    int    `json:"inbox"`
	Tabs     int    `json:"tabs"`
	Spam     int    `json:"spam"`
	Rescued  int    `json:"rescued"`
	Unknown  int    `json:"unknown"`
	Archived int    `json:"archived"`
	Custom   int    `json:"custom"`
}

// WarmupPlacementProvider is the window's placement at one recipient group,
// with the mail hosts inside it.
type WarmupPlacementProvider struct {
	// Group is one of the WarmupRecipient* constants.
	Group string `json:"group"`
	WarmupPlacementCounts
	Hosts []WarmupPlacementHost `json:"hosts"`
}

// WarmupPlacementHost is one mail host inside a recipient group.
type WarmupPlacementHost struct {
	Host string `json:"host"`
	WarmupPlacementCounts
}

// WarmupPlacementMailbox is one sender's row in the workspace report.
type WarmupPlacementMailbox struct {
	EmailAccountID string `json:"email_account_id"`
	Email          string `json:"email"`
	WarmupPlacementCounts
	Rate WarmupPlacementRate `json:"rate"`
	// DailyInboxRate follows the report's days; an entry is nil on a day
	// with no deliveries.
	DailyInboxRate []*float64 `json:"daily_inbox_rate"`
}

// WarmupPlacement reports where warmup mail landed (inbox, Gmail tabs, spam)
// per day and per recipient provider, for one mailbox or the workspace.
// Requires [PermReadAnalytics].
func (s *AnalyticsService) WarmupPlacement(ctx context.Context, params *WarmupPlacementParams, opts ...RequestOption) (*WarmupPlacementReport, *Response, error) {
	return fetch[WarmupPlacementReport](ctx, s.client, withQuery("analytics/warmup/placement", params.values()), opts)
}

// DirectMailAnalytics is the overview of hand-written mail sent from the
// workspace's mailboxes outside campaigns: volume and replies from the
// synced mailbox, plus opens and clicks for the mailboxes that opted into
// tracking them.
type DirectMailAnalytics struct {
	// Period is one of the Period* constants.
	Period      string                   `json:"period"`
	Volume      DirectMailVolume         `json:"volume"`
	Tracking    DirectMailTracking       `json:"tracking"`
	DailyTrend  []DirectMailDay          `json:"daily_trend"`
	Mailboxes   []DirectMailMailboxStats `json:"mailboxes"`
	TopContacts []DirectMailContact      `json:"top_contacts"`
}

// DirectMailVolume is how much was sent and heard back, measured from the
// synced mailbox.
type DirectMailVolume struct {
	Sent     int `json:"sent"`
	Received int `json:"received"`
	// ThreadsStarted counts outbound threads whose first message was ours;
	// Replied those that got an inbound message back.
	ThreadsStarted int     `json:"threads_started"`
	Replied        int     `json:"replied"`
	ReplyRate      float64 `json:"reply_rate"`
	// Bounced counts the delivery failures that came back, excluded from
	// Replied.
	Bounced int `json:"bounced"`
	// MedianReplyMinutes is how long contacts took to answer, zero when
	// nothing has been answered.
	MedianReplyMinutes int `json:"median_reply_minutes"`
}

// DirectMailTracking is the opt-in half. TrackedSent is the denominator for
// both rates: untracked sends are not counted as unopened.
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

// DirectMailDay is one day of direct mail volume.
type DirectMailDay struct {
	Date     time.Time `json:"date"`
	Sent     int       `json:"sent"`
	Received int       `json:"received"`
}

// DirectMailMailboxStats is one mailbox's direct mail volume.
type DirectMailMailboxStats struct {
	EmailAccountID  string `json:"email_account_id"`
	Email           string `json:"email"`
	TrackDirectMail bool   `json:"track_direct_mail"`
	Sent            int    `json:"sent"`
	Received        int    `json:"received"`
}

// DirectMailContact is one correspondent, ranked by how much was sent to
// them.
type DirectMailContact struct {
	Email    string    `json:"email"`
	Sent     int       `json:"sent"`
	Received int       `json:"received"`
	LastAt   time.Time `json:"last_at"`
}

// DirectMail returns the overview of hand-written mail. Period is
// [Period7Days], [Period30Days] or [Period90Days]; anything else falls back
// to [Period7Days]. Requires [PermReadAnalytics].
func (s *AnalyticsService) DirectMail(ctx context.Context, period string, opts ...RequestOption) (*DirectMailAnalytics, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "period", period)
	return fetch[DirectMailAnalytics](ctx, s.client, withQuery("analytics/direct", q), opts)
}

// InboxTaggingParams filters [AnalyticsService.InboxTagging].
type InboxTaggingParams struct {
	// ListOptions.Limit is 1 to 200 (default 50).
	ListOptions
	// NeedsReview keeps only the verdicts flagged for a person to check.
	NeedsReview bool
}

func (p *InboxTaggingParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	if p.NeedsReview {
		q.Set("needs_review", strconv.FormatBool(true))
	}
	return q
}

// InboxTaggingReview is one page of automatic inbox tagging verdicts, newest
// first, with the workspace-wide summary. The embedded [Page] pages through
// the verdicts as usual; following pages carry only the verdicts.
type InboxTaggingReview struct {
	Page[InboxTagVerdict]
	// Enabled says whether automatic tagging is switched on for the
	// instance, so an empty list can be told apart from nothing found.
	Enabled bool `json:"enabled"`
	// Total is how many verdicts match the filter.
	Total   int                 `json:"total"`
	Summary InboxTaggingSummary `json:"summary"`
}

// InboxTaggingSummary counts the workspace's tagging verdicts.
type InboxTaggingSummary struct {
	Total       int `json:"total"`
	NeedsReview int `json:"needs_review"`
	// FromOffline is how many the header rules decided without the model.
	FromOffline int `json:"from_offline"`
	// Acted is how many led to an action (a hold, stop, task or
	// suppression).
	Acted int `json:"acted"`
}

// InboxTagVerdict is what automatic tagging decided about one inbound
// message, and how sure it was.
type InboxTagVerdict struct {
	ID        string `json:"id"`
	MessageID string `json:"message_id"`
	ThreadID  string `json:"thread_id"`
	// Kind is the message kind (for example a human reply or an
	// out-of-office auto reply). KindSource is "header" when the
	// deterministic header rules decided it, "model" when the model did.
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
	// Answers holds the model's answers to the tagging questions, as a JSON
	// object.
	Answers     json.RawMessage `json:"answers"`
	Model       string          `json:"model"`
	InputTokens int             `json:"input_tokens"`
	// Actions is what the workspace's switches let this verdict do.
	Actions []string `json:"actions"`
	// ReturnDate is the out-of-office return date (YYYY-MM-DD) the model
	// was asked to confirm, nil when it was not asked.
	ReturnDate *string   `json:"return_date"`
	CreatedAt  time.Time `json:"created_at"`
}

// InboxTagging returns a page of automatic inbox tagging verdicts for review,
// newest first. Requires [PermReadAnalytics].
func (s *AnalyticsService) InboxTagging(ctx context.Context, params *InboxTaggingParams, opts ...RequestOption) (*InboxTaggingReview, error) {
	query := params.values()
	path := "analytics/inbox-tagging"
	out := &InboxTaggingReview{}
	resp, err := s.client.get(ctx, withQuery(path, query), out, opts...)
	if err != nil {
		return nil, err
	}
	if out.Data == nil {
		out.Data = []InboxTagVerdict{}
	}
	if out.Pagination.Total == nil {
		total := int64(out.Total)
		out.Pagination.Total = &total
	}
	out.resp = resp
	out.fetch = func(ctx context.Context, cursor string) (*Page[InboxTagVerdict], error) {
		q := cloneValues(query)
		q.Set("cursor", cursor)
		return listJSON[InboxTagVerdict](ctx, s.client, path, q, opts...)
	}
	return out, nil
}
