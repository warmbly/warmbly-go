package warmbly

import (
	"context"
	"net/url"
	"strings"
	"time"
)

// Bands a mailbox's rolling warmup inbox rate falls into
// ([WarmupPlacementRate.Band]). Unknown bands decode unchanged.
const (
	WarmupPlacementBandGood = "good"
	WarmupPlacementBandFair = "fair"
	WarmupPlacementBandPoor = "poor"
	// WarmupPlacementBandCollecting means some mail was delivered but not
	// enough yet for a rate.
	WarmupPlacementBandCollecting = "collecting"
	// WarmupPlacementBandNone means nothing was delivered in the window.
	WarmupPlacementBandNone = "none"
)

// Recipient groups ([WarmupPlacementDayGroup.Group],
// [WarmupPlacementProvider.Group]) a warmup recipient is bucketed into.
// Unknown groups decode unchanged.
const (
	WarmupRecipientGoogle    = "google"
	WarmupRecipientMicrosoft = "microsoft"
	WarmupRecipientYahoo     = "yahoo"
	WarmupRecipientOther     = "other"
)

// WarmupPlacementMaxDays is the longest window [AnalyticsService.WarmupPlacement]
// covers; a longer one is a 400.
const WarmupPlacementMaxDays = 366

// WarmupPlacementCounts is where a set of verified warmup deliveries landed.
// Category tabs count as inbox in the rates: the mail was delivered and not
// filtered. The rates are percentages from 0 to 100 and nil until something
// was delivered.
type WarmupPlacementCounts struct {
	// Sent is warmup mail the mailbox sent; Delivered is what recipients
	// verifiably received (Inbox plus Tabs plus Spam).
	Sent      int `json:"sent"`
	Delivered int `json:"delivered"`
	Inbox     int `json:"inbox"`
	// Tabs is mail in a Gmail category tab (Promotions, Updates, Social,
	// Forums).
	Tabs int `json:"tabs"`
	Spam int `json:"spam"`
	// Rescued is spam placements the recipient's mailbox was told to move
	// back to the inbox; the move itself is not confirmed back.
	Rescued int `json:"rescued"`
	// Unconfirmed is mail sent more than 24 hours ago that no recipient has
	// reported seeing.
	Unconfirmed int      `json:"unconfirmed"`
	InboxRate   *float64 `json:"inbox_rate"`
	SpamRate    *float64 `json:"spam_rate"`
}

// WarmupPlacementRate is a mailbox's headline deliverability: the inbox rate
// over the trailing window, withheld below the sample floor. It is taken over
// the major providers only (Google, Microsoft and Yahoo), because a small
// host's own filter says nothing about the sender; other hosts ride beside it
// and are never judged.
type WarmupPlacementRate struct {
	// WindowDays is the trailing window (7) and MinSample the deliveries
	// needed before InboxRate is reported.
	WindowDays int `json:"window_days"`
	MinSample  int `json:"min_sample"`
	// Scope is which recipients the rate is taken over, "major".
	Scope     string `json:"scope"`
	Delivered int    `json:"delivered"`
	Inbox     int    `json:"inbox"`
	Tabs      int    `json:"tabs"`
	Spam      int    `json:"spam"`
	// InboxRate is a percentage, nil until Delivered reaches MinSample.
	InboxRate *float64 `json:"inbox_rate"`
	// Band is one of the WarmupPlacementBand* constants.
	Band string `json:"band"`
	// OtherDelivered and OtherInboxRate are the other mail hosts left out of
	// the major-scope rate; the rate is nil when there are none.
	OtherDelivered int      `json:"other_delivered"`
	OtherInboxRate *float64 `json:"other_inbox_rate"`
}

// WarmupPlacementDayGroup is one recipient group's share of a day.
type WarmupPlacementDayGroup struct {
	// Group is one of the WarmupRecipient* constants.
	Group   string `json:"group"`
	Inbox   int    `json:"inbox"`
	Tabs    int    `json:"tabs"`
	Spam    int    `json:"spam"`
	Rescued int    `json:"rescued"`
}

// WarmupPlacementDay is one UTC day of placement.
type WarmupPlacementDay struct {
	// Date is a calendar day formatted YYYY-MM-DD.
	Date string `json:"date"`
	WarmupPlacementCounts
	// RollingInboxRate is the trailing-window rate ending on this day, nil
	// below the sample floor.
	RollingInboxRate *float64                  `json:"rolling_inbox_rate"`
	Groups           []WarmupPlacementDayGroup `json:"groups"`
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
	// DailyInboxRate follows the report's days, nil on a day with no
	// deliveries.
	DailyInboxRate []*float64 `json:"daily_inbox_rate"`
}

// WarmupPlacementReport is where warmup mail landed over a date range, for
// one mailbox or for the whole workspace.
type WarmupPlacementReport struct {
	// EmailAccountID is set on a single-mailbox report.
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

// WarmupPlacement reports where warmup mail landed (inbox, Gmail tab, spam)
// per day and recipient provider. Pass an empty emailID for the workspace
// report, which also ranks every mailbox. A zero to defaults to today and a
// zero from to 29 days before to, so the default window is 30 days; a window
// longer than [WarmupPlacementMaxDays] days, or from after to, is a 400.
//
// Requires [PermReadAnalytics].
func (s *AnalyticsService) WarmupPlacement(ctx context.Context, emailID string, from, to time.Time, opts ...RequestOption) (*WarmupPlacementReport, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "email_id", emailID)
	setNonEmpty(q, "from", formatDay(from))
	setNonEmpty(q, "to", formatDay(to))
	return fetch[WarmupPlacementReport](ctx, s.client, withQuery("analytics/warmup/placement", q), opts)
}

// DirectMailAnalytics reports on mail written by hand rather than sent by a
// campaign. Volume and replies come from the synced mailbox, so they cover
// everything the mailbox sent (including mail written in Gmail or on a phone)
// and history from before tracking existed. Opens and clicks come from the
// send records, so they cover only mail sent through Warmbly by a mailbox
// with direct-mail tracking switched on, and only from then on. The two are
// reported apart on purpose, never as one blended rate.
type DirectMailAnalytics struct {
	// Period is "7d", "30d" or "90d".
	Period      string                   `json:"period"`
	Volume      DirectMailVolume         `json:"volume"`
	Tracking    DirectMailTracking       `json:"tracking"`
	DailyTrend  []DirectMailDay          `json:"daily_trend"`
	Mailboxes   []DirectMailMailboxStats `json:"mailboxes"`
	TopContacts []DirectMailContact      `json:"top_contacts"`
}

// DirectMailVolume is how much was actually sent and heard back, measured
// from the synced mailbox.
type DirectMailVolume struct {
	Sent     int `json:"sent"`
	Received int `json:"received"`
	// ThreadsStarted counts outbound threads whose first message was ours;
	// Replied those that got an inbound message back.
	ThreadsStarted int     `json:"threads_started"`
	Replied        int     `json:"replied"`
	ReplyRate      float64 `json:"reply_rate"`
	// Bounced counts the delivery failures that came back; they are not
	// counted as replies.
	Bounced int `json:"bounced"`
	// MedianReplyMinutes is how long the contact took to answer across the
	// answered threads, 0 when nothing has been.
	MedianReplyMinutes int `json:"median_reply_minutes"`
}

// DirectMailTracking is the opt-in half. TrackedSent is the denominator of
// both rates: untracked sends were never asked, so they are not failures to
// open.
type DirectMailTracking struct {
	// MailboxesOptedIn out of MailboxesTotal says how much of the picture
	// this covers.
	MailboxesOptedIn int     `json:"mailboxes_opted_in"`
	MailboxesTotal   int     `json:"mailboxes_total"`
	TrackedSent      int     `json:"tracked_sent"`
	Opened           int     `json:"opened"`
	MachineOpened    int     `json:"machine_opened"`
	Clicked          int     `json:"clicked"`
	OpenRate         float64 `json:"open_rate"`
	ClickRate        float64 `json:"click_rate"`
}

// DirectMailDay is one day of direct-mail volume.
type DirectMailDay struct {
	Date     time.Time `json:"date"`
	Sent     int       `json:"sent"`
	Received int       `json:"received"`
}

// DirectMailMailboxStats is one mailbox's direct-mail volume.
type DirectMailMailboxStats struct {
	EmailAccountID string `json:"email_account_id"`
	Email          string `json:"email"`
	// TrackDirectMail reports whether the mailbox opted in to open and click
	// tracking on hand-written mail.
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

// DirectMail returns analytics for mail written by hand over a rolling window.
// Period is [Period7Days], [Period30Days] or [Period90Days]; anything else
// falls back to [Period7Days].
//
// Requires [PermReadAnalytics].
func (s *AnalyticsService) DirectMail(ctx context.Context, period string, opts ...RequestOption) (*DirectMailAnalytics, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "period", period)
	return fetch[DirectMailAnalytics](ctx, s.client, withQuery("analytics/direct", q), opts)
}

// AccountStatusMaxLimit is the most mailboxes one page of
// [AnalyticsService.AccountsPage] holds (also its default), and
// AccountStatusMaxIDs the most [AccountListParams.EmailIDs] one call accepts.
const (
	AccountStatusMaxLimit = 1000
	AccountStatusMaxIDs   = 200
)

// AccountListParams selects and paginates [AnalyticsService.AccountsPage].
type AccountListParams struct {
	// Limit is 1 to [AccountStatusMaxLimit]; anything else is a 400.
	ListOptions
	// EmailIDs scopes the page to these mailboxes (at most
	// [AccountStatusMaxIDs]); an invalid or over-long list is a 400.
	EmailIDs []string
}

// AccountsPage returns one page of mailbox statuses, so a workspace with
// thousands of mailboxes can be read in bounded pieces. Page.All iterates
// every mailbox across pages.
func (s *AnalyticsService) AccountsPage(ctx context.Context, params *AccountListParams, opts ...RequestOption) (*Page[AccountStatus], error) {
	q := make(url.Values)
	if params != nil {
		params.apply(q)
		if len(params.EmailIDs) > 0 {
			q.Set("email_ids", strings.Join(params.EmailIDs, ","))
		}
	}
	return listJSON[AccountStatus](ctx, s.client, "analytics/accounts", q, opts...)
}
