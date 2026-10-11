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
