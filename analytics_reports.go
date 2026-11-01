package warmbly

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Where a verified warmup delivery landed ([WarmupPlacementCounts] fields).
// Category tabs count as delivered and outside spam, so they are inbox for the
// rate.
const (
	// WarmupGroupGoogle, WarmupGroupMicrosoft, WarmupGroupYahoo and
	// WarmupGroupOther are the recipient groups a placement is broken down by
	// ([WarmupPlacementProvider].Group). New groups may appear.
	WarmupGroupGoogle    = "google"
	WarmupGroupMicrosoft = "microsoft"
	WarmupGroupYahoo     = "yahoo"
	WarmupGroupOther     = "other"
)

// Bands of a [WarmupPlacementRate]. New bands may appear.
const (
	WarmupBandGood       = "good"
	WarmupBandFair       = "fair"
	WarmupBandPoor       = "poor"
	WarmupBandCollecting = "collecting"
	WarmupBandNone       = "none"
)

// WarmupPlacementCounts is where a set of verified warmup deliveries landed.
// The inbox rate counts category tabs as inbox: the mail was delivered and not
// filtered. Rates are percentages and nil until something is delivered.
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
	// InstrumentedReceipts and ObservedReceipts count the receipts seen, and
	// LegacyUninstrumentedReceipts those classified before instrumentation.
	InstrumentedReceipts         int                `json:"instrumented_receipts"`
	ObservedReceipts             int                `json:"observed_receipts"`
	LegacyUninstrumentedReceipts int                `json:"legacy_uninstrumented_receipts"`
	RescueRequested              int                `json:"rescue_requested"`
	NonSpamMetric                *ObservationMetric `json:"non_spam_metric"`
	// Rescued is spam placements the recipient's mailbox was told to move back
	// to the inbox; the outcome counters are what its worker reported.
	Rescued         int `json:"rescued"`
	RescueConfirmed int `json:"rescue_confirmed"`
	RescueMissing   int `json:"rescue_missing"`
	RescueFailed    int `json:"rescue_failed"`
	// RescueUnreported is requested rescues no worker has answered yet.
	RescueUnreported int `json:"rescue_unreported"`
	// Unconfirmed is mail sent more than 24 hours ago that no recipient has
	// reported seeing.
	Unconfirmed int      `json:"unconfirmed"`
	InboxRate   *float64 `json:"inbox_rate"`
	SpamRate    *float64 `json:"spam_rate"`
}

// WarmupPlacementGroup is one recipient group's share of a day.
type WarmupPlacementGroup struct {
	Group           string `json:"group"`
	Inbox           int    `json:"inbox"`
	Tabs            int    `json:"tabs"`
	Spam            int    `json:"spam"`
	Rescued         int    `json:"rescued"`
	Unknown         int    `json:"unknown"`
	Archived        int    `json:"archived"`
	Custom          int    `json:"custom"`
	RescueConfirmed int    `json:"rescue_confirmed"`
	RescueMissing   int    `json:"rescue_missing"`
	RescueFailed    int    `json:"rescue_failed"`
}

// WarmupPlacementDay is one UTC day of warmup placement.
type WarmupPlacementDay struct {
	// Date is a calendar day formatted YYYY-MM-DD.
	Date string `json:"date"`
	WarmupPlacementCounts
	// RollingInboxRate is the trailing-window rate ending on this day, nil
	// below the sample floor.
	RollingInboxRate *float64               `json:"rolling_inbox_rate"`
	Groups           []WarmupPlacementGroup `json:"groups"`
}

// WarmupPlacementHost is one mail host inside a recipient group.
type WarmupPlacementHost struct {
	Host string `json:"host"`
	WarmupPlacementCounts
}

// WarmupPlacementProvider is the window's placement at one recipient group.
type WarmupPlacementProvider struct {
	// Group is one of the WarmupGroup constants.
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
	// DailyInboxRate follows the report's days; nil on a day with no
	// deliveries.
	DailyInboxRate []*float64 `json:"daily_inbox_rate"`
}

// WarmupPlacementReport is where warmup mail landed over a date range, for one
// mailbox or for the whole workspace.
type WarmupPlacementReport struct {
	// EmailAccountID is set when the report covers one mailbox.
	EmailAccountID string                    `json:"email_account_id,omitempty"`
	DateRange      DateRange                 `json:"date_range"`
	Summary        WarmupPlacementCounts     `json:"summary"`
	Rate           WarmupPlacementRate       `json:"rate"`
	Daily          []WarmupPlacementDay      `json:"daily"`
	Providers      []WarmupPlacementProvider `json:"providers"`
	// Mailboxes is present on the workspace report only, worst rate first.
	Mailboxes []WarmupPlacementMailbox `json:"mailboxes,omitempty"`
}

// WarmupPlacementParams selects the mailbox and window of
// [AnalyticsService.WarmupPlacement]. Every field is optional.
type WarmupPlacementParams struct {
	// EmailID restricts the report to one mailbox; empty covers the workspace.
	EmailID string
	// From and To bound the window as calendar days (UTC). To defaults to
	// today and From to the 30 days ending on To.
	From, To time.Time
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

// DirectMailVolume is the "how much did we actually send and hear back" half
// of [DirectMailAnalytics], measured from the synced mailbox.
type DirectMailVolume struct {
	Sent     int `json:"sent"`
	Received int `json:"received"`
	// ThreadsStarted counts outbound threads whose first message was ours and
	// Replied those that got an inbound message back.
	ThreadsStarted int     `json:"threads_started"`
	Replied        int     `json:"replied"`
	ReplyRate      float64 `json:"reply_rate"`
	// Bounced counts delivery failures that came back; they are excluded from
	// Replied.
	Bounced int `json:"bounced"`
	// MedianReplyMinutes is how long contacts took to answer, across answered
	// threads. Zero when nothing has been answered.
	MedianReplyMinutes int `json:"median_reply_minutes"`
}

// DirectMailTracking is the opt-in half of [DirectMailAnalytics]. TrackedSent
// is the denominator for both rates: untracked sends were never asked, so they
// are not failures to open.
type DirectMailTracking struct {
	// MailboxesOptedIn out of MailboxesTotal says how much of the picture this
	// covers.
	MailboxesOptedIn int `json:"mailboxes_opted_in"`
	MailboxesTotal   int `json:"mailboxes_total"`
	TrackedSent      int `json:"tracked_sent"`
	Opened           int `json:"opened"`
	MachineOpened    int `json:"machine_opened"`
	Clicked          int `json:"clicked"`
	// OpenRate and ClickRate are over TrackedSent.
	OpenRate  float64 `json:"open_rate"`
	ClickRate float64 `json:"click_rate"`
}

// DirectMailDay is one day of hand-written mail volume.
type DirectMailDay struct {
	Date     time.Time `json:"date"`
	Sent     int       `json:"sent"`
	Received int       `json:"received"`
}

// DirectMailMailbox is one mailbox's hand-written mail volume.
type DirectMailMailbox struct {
	EmailAccountID string `json:"email_account_id"`
	Email          string `json:"email"`
	// TrackDirectMail reports whether the mailbox opted into open and click
	// tracking on hand-written mail.
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

// DirectMailAnalytics reports on mail written by hand rather than sent by a
// campaign. Volume and replies come from the synced mailbox and cover
// everything it sent; opens and clicks cover only mail sent through Warmbly by
// a mailbox with tracking switched on. The two are kept apart because blending
// them into one rate would mislead.
type DirectMailAnalytics struct {
	// Period is the window, one of the Period constants.
	Period      string              `json:"period"`
	Volume      DirectMailVolume    `json:"volume"`
	Tracking    DirectMailTracking  `json:"tracking"`
	DailyTrend  []DirectMailDay     `json:"daily_trend"`
	Mailboxes   []DirectMailMailbox `json:"mailboxes"`
	TopContacts []DirectMailContact `json:"top_contacts"`
}

// InboxTaggingSummary counts the workspace's automatic inbox tagging verdicts.
type InboxTaggingSummary struct {
	Total       int `json:"total"`
	NeedsReview int `json:"needs_review"`
	FromOffline int `json:"from_offline"`
	Acted       int `json:"acted"`
}

// InboxTaggingVerdict is one message's automatic tagging verdict: what the
// classifier decided, how sure it was and what the workspace's switches let it
// do.
type InboxTaggingVerdict struct {
	ID               string   `json:"id"`
	MessageID        string   `json:"message_id"`
	ThreadID         string   `json:"thread_id"`
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
	// Answers holds the answers to the workspace's own tagging questions,
	// keyed by question id.
	Answers     json.RawMessage `json:"answers"`
	Model       string          `json:"model"`
	InputTokens int             `json:"input_tokens"`
	// Actions is what the workspace's switches let this verdict do.
	Actions []string `json:"actions"`
	// ReturnDate is the out-of-office return date (YYYY-MM-DD) the model was
	// asked to confirm, nil when it was not asked.
	ReturnDate *string   `json:"return_date"`
	CreatedAt  time.Time `json:"created_at"`
}

// InboxTaggingReview is the read-only review surface for automatic inbox
// tagging: a page of verdicts plus the totals. Walk further pages by passing
// Pagination.NextCursor back in [InboxTaggingParams].
type InboxTaggingReview struct {
	// Enabled says whether tagging is switched on for this instance, so an
	// empty list can be explained.
	Enabled bool                  `json:"enabled"`
	Data    []InboxTaggingVerdict `json:"data"`
	// Total counts the verdicts matching the filter.
	Total      int                 `json:"total"`
	Summary    InboxTaggingSummary `json:"summary"`
	Pagination Pagination          `json:"pagination"`
}

// InboxTaggingParams filters [AnalyticsService.InboxTagging].
type InboxTaggingParams struct {
	// Limit is the page size, 1 to 200 (default 50) and Cursor the token from
	// the previous page's Pagination.NextCursor.
	ListOptions
	// NeedsReview keeps only the verdicts a person should look at.
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

// AccountStatusListParams pages [AnalyticsService.ListAccounts].
type AccountStatusListParams struct {
	// Limit is the page size, 1 to 1000 (default 1000) and Cursor the token
	// from the previous page.
	ListOptions
	// EmailIDs restricts the page to these mailboxes (at most 200), for a view
	// that shows a known set.
	EmailIDs []string
}

func (p *AccountStatusListParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	if len(p.EmailIDs) > 0 {
		q.Set("email_ids", strings.Join(p.EmailIDs, ","))
	}
	return q
}

// DashboardFor is [AnalyticsService.Dashboard] with a campaign and folder
// scope. params may be nil. Needs [PermReadAnalytics].
func (s *AnalyticsService) DashboardFor(ctx context.Context, params *DashboardParams, opts ...RequestOption) (*DashboardAnalytics, *Response, error) {
	return fetch[DashboardAnalytics](ctx, s.client, withQuery("analytics/dashboard", params.values()), opts)
}

// CampaignBetween is [AnalyticsService.Campaign] restricted to a date range
// (calendar days, UTC, both included). Without a range the report covers all
// time. Needs [PermReadAnalytics].
func (s *AnalyticsService) CampaignBetween(ctx context.Context, id string, from, to time.Time, opts ...RequestOption) (*CampaignAnalytics, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "from", formatDay(from))
	setNonEmpty(q, "to", formatDay(to))
	return fetch[CampaignAnalytics](ctx, s.client, withQuery("analytics/campaigns/"+url.PathEscape(id), q), opts)
}

// ListAccounts returns a page of mailbox statuses. Unlike
// [AnalyticsService.Accounts], which reads only the first page, it walks the
// whole inventory through [Page.All]. Needs [PermReadAnalytics].
func (s *AnalyticsService) ListAccounts(ctx context.Context, params *AccountStatusListParams, opts ...RequestOption) (*Page[AccountStatus], error) {
	return listJSON[AccountStatus](ctx, s.client, "analytics/accounts", params.values(), opts...)
}

// Direct returns the hand-written-mail overview: volume and replies from the
// synced mailboxes, plus opens and clicks for the mailboxes that opted into
// tracking them. Period is [Period7Days] (default), [Period30Days] or
// [Period90Days]. Needs [PermReadAnalytics].
func (s *AnalyticsService) Direct(ctx context.Context, period string, opts ...RequestOption) (*DirectMailAnalytics, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "period", period)
	return fetch[DirectMailAnalytics](ctx, s.client, withQuery("analytics/direct", q), opts)
}

// InboxTagging returns the review surface for automatic inbox tagging: what
// the classifier decided for recent messages and how sure it was. It is
// read-only. Needs [PermReadAnalytics].
func (s *AnalyticsService) InboxTagging(ctx context.Context, params *InboxTaggingParams, opts ...RequestOption) (*InboxTaggingReview, *Response, error) {
	return fetch[InboxTaggingReview](ctx, s.client, withQuery("analytics/inbox-tagging", params.values()), opts)
}

// WarmupPlacement reports where warmup mail landed (inbox, category tabs,
// spam) per day and per recipient provider, for one mailbox or the workspace.
// params may be nil. Needs [PermReadAnalytics].
func (s *AnalyticsService) WarmupPlacement(ctx context.Context, params *WarmupPlacementParams, opts ...RequestOption) (*WarmupPlacementReport, *Response, error) {
	return fetch[WarmupPlacementReport](ctx, s.client, withQuery("analytics/warmup/placement", params.values()), opts)
}
