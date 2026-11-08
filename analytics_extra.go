package warmbly

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"time"
)

// DashboardFilter narrows the campaign-derived sections of
// [AnalyticsService.DashboardFiltered] to some campaigns and folders. A
// folder stands in for every campaign in it. Each list takes at most 100
// ids. A caller limited to some campaigns may only name its own.
type DashboardFilter struct {
	CampaignIDs []string
	FolderIDs   []string
}

// Warmup recipient groups ([WarmupPlacementProvider.Group]).
const (
	WarmupRecipientGoogle    = "google"
	WarmupRecipientMicrosoft = "microsoft"
	WarmupRecipientYahoo     = "yahoo"
	WarmupRecipientOther     = "other"
)

// Bands of a mailbox's rolling inbox rate ([WarmupPlacementRate.Band]).
const (
	WarmupPlacementBandGood       = "good"
	WarmupPlacementBandFair       = "fair"
	WarmupPlacementBandPoor       = "poor"
	WarmupPlacementBandCollecting = "collecting"
	WarmupPlacementBandNone       = "none"
)

// WarmupPlacementRate is a mailbox's headline deliverability: the inbox rate
// over a trailing window, withheld below the sample floor.
type WarmupPlacementRate struct {
	WindowDays int `json:"window_days"`
	MinSample  int `json:"min_sample"`
	// Scope is which recipients the rate is taken over: "major" (Google,
	// Microsoft and Yahoo recipients only).
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
	// a major-scope rate, shown beside it and never judged; nil with none.
	OtherDelivered int      `json:"other_delivered"`
	OtherInboxRate *float64 `json:"other_inbox_rate"`
}

// WarmupPlacementCounts is where a set of verified warmup deliveries landed.
// The inbox rate counts category tabs as inbox: the mail was delivered and
// not filtered.
type WarmupPlacementCounts struct {
	// Sent is warmup mail the mailbox sent; Delivered is what recipients
	// verifiably received (inbox, tabs and spam).
	Sent             int                `json:"sent"`
	Delivered        int                `json:"delivered"`
	Inbox            int                `json:"inbox"`
	Tabs             int                `json:"tabs"`
	Spam             int                `json:"spam"`
	Unknown          int                `json:"unknown"`
	Archived         int                `json:"archived"`
	Custom           int                `json:"custom"`
	Instrumented     int                `json:"instrumented_receipts"`
	Observed         int                `json:"observed_receipts"`
	LegacyClassified int                `json:"legacy_uninstrumented_receipts"`
	RescueRequested  int                `json:"rescue_requested"`
	NonSpamMetric    *ObservationMetric `json:"non_spam_metric"`
	// Rescued is spam placements the recipient's mailbox was told to move
	// back to the inbox; the outcome counters are what its worker reported.
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

// WarmupPlacementGroupCounts is one recipient group's share of a day.
type WarmupPlacementGroupCounts struct {
	// Group is one of the WarmupRecipient* constants.
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

// WarmupPlacementDay is one UTC day of placement.
type WarmupPlacementDay struct {
	// Date is YYYY-MM-DD.
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
	// Group is one of the WarmupRecipient* constants.
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

// WarmupPlacementReport is where warmup mail landed over a date range, for
// one mailbox or for the whole workspace.
type WarmupPlacementReport struct {
	// EmailAccountID is set when the report is for one mailbox.
	EmailAccountID *string               `json:"email_account_id,omitempty"`
	DateRange      DateRange             `json:"date_range"`
	Summary        WarmupPlacementCounts `json:"summary"`
	Rate           WarmupPlacementRate   `json:"rate"`
	Daily          []WarmupPlacementDay  `json:"daily"`
	// Providers is the window's placement per recipient group.
	Providers []WarmupPlacementProvider `json:"providers"`
	// Mailboxes is present on the workspace report only, worst rate first.
	Mailboxes []WarmupPlacementMailbox `json:"mailboxes,omitempty"`
}

// WarmupPlacement reports where warmup mail landed (inbox, category tabs,
// spam) per day and per recipient provider, for one mailbox or, with an empty
// emailID, the whole workspace. A zero to is today (UTC) and a zero from is
// the 29 days before it. Needs the read_analytics scope.
func (s *AnalyticsService) WarmupPlacement(ctx context.Context, emailID string, from, to time.Time, opts ...RequestOption) (*WarmupPlacementReport, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "email_id", emailID)
	setNonEmpty(q, "from", formatDay(from))
	setNonEmpty(q, "to", formatDay(to))
	return fetch[WarmupPlacementReport](ctx, s.client, withQuery("analytics/warmup/placement", q), opts)
}

// Direct mail periods are the same as the dashboard's: [Period7Days],
// [Period30Days] and [Period90Days].

// DirectMailAnalytics reports on mail written by hand rather than sent by a
// campaign. Volume and replies come from the synced mailbox, so they cover
// everything the mailbox sent, including mail written in Gmail or on a phone,
// and history from before the feature shipped. Opens and clicks come from the
// send records, so they cover only mail sent through Warmbly by a mailbox with
// tracking switched on, and only from the moment it was switched on. The two
// are deliberately not blended into one rate.
type DirectMailAnalytics struct {
	Period      string                   `json:"period"`
	Volume      DirectMailVolume         `json:"volume"`
	Tracking    DirectMailTracking       `json:"tracking"`
	DailyTrend  []DirectMailDailyStats   `json:"daily_trend"`
	Mailboxes   []DirectMailMailboxStats `json:"mailboxes"`
	TopContacts []DirectMailContact      `json:"top_contacts"`
}

// DirectMailVolume is how much was sent and heard back, measured from the
// synced mailbox.
type DirectMailVolume struct {
	Sent     int `json:"sent"`
	Received int `json:"received"`
	// ThreadsStarted counts outbound threads whose first message was ours and
	// Replied those that got an inbound message back.
	ThreadsStarted int `json:"threads_started"`
	Replied        int `json:"replied"`
	// ReplyRate is a percentage.
	ReplyRate float64 `json:"reply_rate"`
	// Bounced counts the delivery failures that came back; they are excluded
	// from Replied.
	Bounced int `json:"bounced"`
	// MedianReplyMinutes is how long the contact took to answer, across the
	// threads that were answered. Zero when nothing has been.
	MedianReplyMinutes int `json:"median_reply_minutes"`
}

// DirectMailTracking is the opt-in half. TrackedSent is the denominator for
// both rates: untracked sends are not failures to open, they were never asked.
type DirectMailTracking struct {
	// MailboxesOptedIn of MailboxesTotal says how much of the picture this
	// covers.
	MailboxesOptedIn int `json:"mailboxes_opted_in"`
	MailboxesTotal   int `json:"mailboxes_total"`
	TrackedSent      int `json:"tracked_sent"`
	Opened           int `json:"opened"`
	MachineOpened    int `json:"machine_opened"`
	Clicked          int `json:"clicked"`
	// OpenRate and ClickRate are percentages of TrackedSent.
	OpenRate  float64 `json:"open_rate"`
	ClickRate float64 `json:"click_rate"`
}

// DirectMailDailyStats is one day of hand-written mail.
type DirectMailDailyStats struct {
	Date     time.Time `json:"date"`
	Sent     int       `json:"sent"`
	Received int       `json:"received"`
}

// DirectMailMailboxStats is one mailbox's hand-written mail.
type DirectMailMailboxStats struct {
	EmailAccountID string `json:"email_account_id"`
	Email          string `json:"email"`
	// TrackDirectMail says whether the mailbox has opted in to tracking its
	// hand-written mail.
	TrackDirectMail bool `json:"track_direct_mail"`
	Sent            int  `json:"sent"`
	Received        int  `json:"received"`
}

// DirectMailContact is one correspondent, ranked by how much was sent to
// them.
type DirectMailContact struct {
	Email    string    `json:"email"`
	Sent     int       `json:"sent"`
	Received int       `json:"received"`
	LastAt   time.Time `json:"last_at"`
}

// DirectMail reports on mail written by hand rather than sent by a campaign.
// Period is [Period7Days] (the default), [Period30Days] or [Period90Days];
// anything else falls back to the default. Needs the read_analytics scope.
func (s *AnalyticsService) DirectMail(ctx context.Context, period string, opts ...RequestOption) (*DirectMailAnalytics, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "period", period)
	return fetch[DirectMailAnalytics](ctx, s.client, withQuery("analytics/direct", q), opts)
}

// Intent and kind labels are free-form strings the classifier chooses from;
// treat unknown values as opaque.

// InboxTagResult is one classified inbound message: what automatic inbox
// tagging decided and how sure it was.
type InboxTagResult struct {
	ID        string `json:"id"`
	MessageID string `json:"message_id"`
	ThreadID  string `json:"thread_id"`
	// Kind says what the message is (a person's reply, an automated notice and
	// so on); KindSource says what decided it.
	Kind           string  `json:"kind"`
	KindConfidence float64 `json:"kind_confidence"`
	KindSource     string  `json:"kind_source"`
	// Intent is the sender's intent for a human reply.
	Intent           string  `json:"intent"`
	IntentConfidence float64 `json:"intent_confidence"`
	Relevance        int     `json:"relevance"`
	Priority         string  `json:"priority"`
	// NeedsReview marks a verdict a person should look at, and ReviewReason
	// says why.
	NeedsReview  bool     `json:"needs_review"`
	ReviewReason string   `json:"review_reason"`
	Labels       []string `json:"labels"`
	// Answers holds the structured answers to the questions the model was
	// asked, as raw JSON.
	Answers     json.RawMessage `json:"answers"`
	Model       string          `json:"model"`
	InputTokens int             `json:"input_tokens"`
	// Actions is what the workspace's switches let this verdict do: "hold",
	// "stop", "task" or "suppress". Empty for a verdict that only labelled.
	Actions []string `json:"actions"`
	// ReturnDate is the out-of-office return date the model was asked to
	// confirm (YYYY-MM-DD); nil when it was not asked.
	ReturnDate *string   `json:"return_date"`
	CreatedAt  time.Time `json:"created_at"`
}

// InboxTaggingSummary counts the workspace's tagging verdicts.
type InboxTaggingSummary struct {
	Total       int `json:"total"`
	NeedsReview int `json:"needs_review"`
	FromOffline int `json:"from_offline"`
	Acted       int `json:"acted"`
}

// InboxTaggingReview is one page of the automatic inbox tagging review: what
// was decided about inbound mail and how sure it was. It is read-only.
type InboxTaggingReview struct {
	// Enabled says whether the feature is switched on for this instance, so
	// an empty list can be explained rather than implying nothing was found.
	Enabled bool `json:"enabled"`
	// Data is the page's verdicts, newest first.
	Data    []InboxTagResult    `json:"data"`
	Total   int                 `json:"total"`
	Summary InboxTaggingSummary `json:"summary"`
	// Pagination carries the cursor for the next page.
	Pagination Pagination `json:"pagination"`
}

// NextCursor returns the opaque cursor for the next page, or "" on the last.
func (r *InboxTaggingReview) NextCursor() string {
	if r.Pagination.NextCursor != nil {
		return *r.Pagination.NextCursor
	}
	return ""
}

// InboxTaggingParams paginates and filters the tagging review.
type InboxTaggingParams struct {
	// Limit is 1 to 200 (default 50) and Cursor the NextCursor of the
	// previous page.
	ListOptions
	// NeedsReview keeps only verdicts flagged for review.
	NeedsReview bool
}

// InboxTagging returns one page of the automatic inbox tagging review. Pass
// [InboxTaggingReview.NextCursor] back in [ListOptions.Cursor] for the next
// page. Needs the read_analytics scope.
func (s *AnalyticsService) InboxTagging(ctx context.Context, params *InboxTaggingParams, opts ...RequestOption) (*InboxTaggingReview, *Response, error) {
	q := make(url.Values)
	if params != nil {
		params.apply(q)
		if params.NeedsReview {
			q.Set("needs_review", "true")
		}
	}
	return fetch[InboxTaggingReview](ctx, s.client, withQuery("analytics/inbox-tagging", q), opts)
}

// DashboardFiltered is [AnalyticsService.Dashboard] with the campaign-derived
// sections narrowed to some campaigns and folders; the filter that was applied
// is echoed in [DashboardAnalytics.Scope]. Account counts stay workspace-wide.
func (s *AnalyticsService) DashboardFiltered(ctx context.Context, period string, filter *DashboardFilter, opts ...RequestOption) (*DashboardAnalytics, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "period", period)
	if filter != nil {
		if len(filter.CampaignIDs) > 0 {
			q.Set("campaign_ids", strings.Join(filter.CampaignIDs, ","))
		}
		if len(filter.FolderIDs) > 0 {
			q.Set("folder_ids", strings.Join(filter.FolderIDs, ","))
		}
	}
	return fetch[DashboardAnalytics](ctx, s.client, withQuery("analytics/dashboard", q), opts)
}

// CampaignRange is [AnalyticsService.Campaign] over a date range. Both ends
// are whole UTC days and the end is included; zero times for both read all
// time. Giving only one end is a 400.
func (s *AnalyticsService) CampaignRange(ctx context.Context, id string, from, to time.Time, opts ...RequestOption) (*CampaignAnalytics, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "from", formatDay(from))
	setNonEmpty(q, "to", formatDay(to))
	return fetch[CampaignAnalytics](ctx, s.client, withQuery("analytics/campaigns/"+url.PathEscape(id), q), opts)
}

// AccountsParams pages the mailbox status list.
type AccountsParams struct {
	// Limit is 1 to 1000 (default 1000) and Cursor the NextCursor of the
	// previous page.
	ListOptions
	// EmailIDs asks only for the named mailboxes (at most 200), which is what
	// a page view showing some of them wants.
	EmailIDs []string
}

// AccountsPage returns one page of mailbox status. The per-mailbox reads are
// batched across the page, so its cost does not grow with the inventory. A
// mailbox-limited caller sees only its own mailboxes.
func (s *AnalyticsService) AccountsPage(ctx context.Context, params *AccountsParams, opts ...RequestOption) (*Page[AccountStatus], error) {
	q := make(url.Values)
	if params != nil {
		params.apply(q)
		if len(params.EmailIDs) > 0 {
			q.Set("email_ids", strings.Join(params.EmailIDs, ","))
		}
	}
	return listJSON[AccountStatus](ctx, s.client, "analytics/accounts", q, opts...)
}
