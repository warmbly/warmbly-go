package warmbly

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// ReportService manages client reports: saved, scoped views of the
// workspace's analytics that are shared outside it, either by a link that
// opens without signing in or by a scheduled email.
//
// A report covers the whole workspace or only some campaigns, campaign
// folders and mailboxes ([ReportAccess]), and shows the sections turned on in
// [ReportSections]. [ReportService.Preview] renders it as its reader will see
// it.
//
// Reading (List, Get, Preview) needs the read_analytics scope. Every change,
// including minting a share link and setting a schedule, needs the
// manage_reports scope, because a link shows the numbers to anyone who holds
// it.
type ReportService service

// Report date ranges, for [Report.DateRange] and the range argument of
// [ReportService.Preview]. Ranges are whole UTC days with today included.
// Other values may appear.
const (
	ReportLast7Days  = "last_7_days"
	ReportLast30Days = "last_30_days"
	ReportThisMonth  = "this_month"
	ReportLastMonth  = "last_month"
)

// Report email cadences, for [ReportSchedule.Cadence]. A weekly email covers
// the last 7 days, a monthly one the last month.
const (
	ReportWeekly  = "weekly"
	ReportMonthly = "monthly"
)

// Report access scopes, for [ReportAccess.Scope].
const (
	// ReportAccessWorkspace covers every campaign and mailbox in the
	// workspace.
	ReportAccessWorkspace = "workspace"
	// ReportAccessRestricted covers only the listed folders, campaigns and
	// mailboxes.
	ReportAccessRestricted = "restricted"
)

// Report limits enforced by the server.
const (
	// ReportMaxPerWorkspace is the most reports one workspace keeps.
	ReportMaxPerWorkspace = 200
	// ReportMaxRecipients is the most recipients one schedule sends to.
	ReportMaxRecipients = 10
	// ReportLinkDefaultDays is a share link's lifetime when none is asked for.
	ReportLinkDefaultDays = 90
	// ReportLinkMaxDays is the longest lifetime a share link may be given.
	ReportLinkMaxDays = 365
	// ReportMaxAccessGrants is the most of each of folders, campaigns and
	// mailboxes a restricted report may list.
	ReportMaxAccessGrants = 500
)

// ReportAccess is which campaigns, folders and mailboxes a report covers.
type ReportAccess struct {
	// Scope is [ReportAccessWorkspace] or [ReportAccessRestricted]. Empty on
	// a write means workspace.
	Scope string `json:"scope"`
	// CampaignFolderIDs cover every campaign the folder holds when the report
	// is read.
	CampaignFolderIDs []string `json:"campaign_folder_ids"`
	CampaignIDs       []string `json:"campaign_ids"`
	EmailAccountIDs   []string `json:"email_account_ids"`
}

// ReportSections are the parts of a report that are shown. At least one must
// be on.
type ReportSections struct {
	Overview        bool `json:"overview"`
	Campaigns       bool `json:"campaigns"`
	Replies         bool `json:"replies"`
	PositiveReplies bool `json:"positive_replies"`
	Mailboxes       bool `json:"mailboxes"`
	DailyChart      bool `json:"daily_chart"`
}

// ReportLink is the state of a report's share link. The URL itself is only
// returned once, by [ReportService.CreateLink].
type ReportLink struct {
	// Hint is the token's first characters, so links can be told apart.
	Hint      string    `json:"hint"`
	CreatedAt time.Time `json:"created_at"`
	// ExpiresAt is when the link stops opening; nil never expires.
	ExpiresAt *time.Time `json:"expires_at"`
	Expired   bool       `json:"expired"`
}

// ReportSchedule is when a report is emailed and to whom.
type ReportSchedule struct {
	Recipients []string `json:"recipients"`
	// Cadence is [ReportWeekly] or [ReportMonthly].
	Cadence string `json:"cadence"`
	// Weekday is 0 (Sunday) to 6, used by weekly schedules.
	Weekday int `json:"weekday"`
	// MonthDay is 1 to 28, used by monthly schedules.
	MonthDay int `json:"month_day"`
	// Hour is 0 to 23 in Timezone.
	Hour     int    `json:"hour"`
	Timezone string `json:"timezone"`
	Paused   bool   `json:"paused"`
	// LastSentAt is nil until the first email goes out.
	LastSentAt *time.Time `json:"last_sent_at"`
	NextRunAt  time.Time  `json:"next_run_at"`
}

// Report is a saved, scoped report a workspace shares with its client.
type Report struct {
	ID string `json:"id"`
	// CreatedBy is the user who created it; nil when unknown.
	CreatedBy *string        `json:"created_by"`
	Name      string         `json:"name"`
	Access    ReportAccess   `json:"access"`
	Sections  ReportSections `json:"sections"`
	// ShowReplySnippets includes the text of each reply in the replies list.
	ShowReplySnippets bool `json:"show_reply_snippets"`
	// DateRange is the period a link opens on, one of the Report* range
	// constants.
	DateRange string `json:"date_range"`
	// Link is the current share link; nil when none has been minted or it was
	// revoked.
	Link *ReportLink `json:"link"`
	// Schedule is the email schedule; nil when the report is not emailed.
	Schedule  *ReportSchedule `json:"schedule"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// ReportCreateParams creates a report. Name is required; the rest defaults to
// the whole workspace, every section on, no reply snippets and
// [ReportLast30Days].
type ReportCreateParams struct {
	Name              string          `json:"name"`
	Access            *ReportAccess   `json:"access,omitempty"`
	Sections          *ReportSections `json:"sections,omitempty"`
	ShowReplySnippets *bool           `json:"show_reply_snippets,omitempty"`
	DateRange         *string         `json:"date_range,omitempty"`
}

// ReportUpdateParams changes a report. Nil fields keep their value; Access and
// Sections replace the stored value whole.
type ReportUpdateParams struct {
	Name              *string         `json:"name,omitempty"`
	Access            *ReportAccess   `json:"access,omitempty"`
	Sections          *ReportSections `json:"sections,omitempty"`
	ShowReplySnippets *bool           `json:"show_reply_snippets,omitempty"`
	DateRange         *string         `json:"date_range,omitempty"`
}

// ReportLinkParams is the optional body of [ReportService.CreateLink].
type ReportLinkParams struct {
	// ExpiresInDays is 1 to [ReportLinkMaxDays]; nil uses
	// [ReportLinkDefaultDays].
	ExpiresInDays *int `json:"expires_in_days,omitempty"`
}

// ReportLinkResult is a freshly minted share link. URL and Token are shown
// this once and cannot be read back later.
type ReportLinkResult struct {
	URL    string     `json:"url"`
	Token  string     `json:"token"`
	Link   ReportLink `json:"link"`
	Report *Report    `json:"report"`
}

// ReportScheduleParams sets when a report is emailed. It replaces any earlier
// schedule whole.
type ReportScheduleParams struct {
	// Recipients are 1 to [ReportMaxRecipients] plain email addresses.
	Recipients []string `json:"recipients"`
	// Cadence is [ReportWeekly] or [ReportMonthly].
	Cadence string `json:"cadence"`
	// Weekday is 0 (Sunday) to 6; read by weekly schedules.
	Weekday int `json:"weekday"`
	// MonthDay is 1 to 28; read by monthly schedules. Zero means 1.
	MonthDay int `json:"month_day"`
	// Hour is 0 to 23 in Timezone.
	Hour int `json:"hour"`
	// Timezone is an IANA zone; empty means UTC.
	Timezone string `json:"timezone"`
	Paused   bool   `json:"paused"`
}

// ReportView is a rendered report: what the preview and the shared page show.
// Sections that are off are absent.
type ReportView struct {
	Name      string `json:"name"`
	DateRange string `json:"date_range"`
	// From and To are the covered days, YYYY-MM-DD.
	From              string           `json:"from"`
	To                string           `json:"to"`
	GeneratedAt       time.Time        `json:"generated_at"`
	Sections          ReportSections   `json:"sections"`
	ShowReplySnippets bool             `json:"show_reply_snippets"`
	Overview          *ReportOverview  `json:"overview,omitempty"`
	Daily             []ReportDay      `json:"daily,omitempty"`
	Campaigns         []ReportCampaign `json:"campaigns,omitempty"`
	Replies           []ReportReply    `json:"replies,omitempty"`
	Mailboxes         *ReportMailboxes `json:"mailboxes,omitempty"`
}

// ReportOverview is the KPI row. The positive figures are nil when the
// positive replies section is off.
type ReportOverview struct {
	Sent   int `json:"sent"`
	Opened int `json:"opened"`
	// OpenTrackedSent is the sends that carried an open pixel; OpenRate is
	// of these.
	OpenTrackedSent int      `json:"open_tracked_sent"`
	OpenRate        float64  `json:"open_rate"`
	Clicked         int      `json:"clicked"`
	Replied         int      `json:"replied"`
	ReplyRate       float64  `json:"reply_rate"`
	Positive        *int     `json:"positive,omitempty"`
	PositiveRate    *float64 `json:"positive_rate,omitempty"`
	Bounced         int      `json:"bounced"`
	BounceRate      float64  `json:"bounce_rate"`
}

// ReportDay is one day of the chart.
type ReportDay struct {
	// Date is YYYY-MM-DD.
	Date    string `json:"date"`
	Sent    int    `json:"sent"`
	Opens   int    `json:"opens"`
	Replies int    `json:"replies"`
}

// ReportCampaign is one row of the campaign table.
type ReportCampaign struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Sent   int    `json:"sent"`
	Opens  int    `json:"opens"`
	// OpenTrackedSent is the sends that carried an open pixel; OpenRate is
	// of these.
	OpenTrackedSent int     `json:"open_tracked_sent"`
	OpenRate        float64 `json:"open_rate"`
	Clicks          int     `json:"clicks"`
	Replies         int     `json:"replies"`
	ReplyRate       float64 `json:"reply_rate"`
	// Positive is nil when the positive replies section is off.
	Positive   *int    `json:"positive,omitempty"`
	Bounces    int     `json:"bounces"`
	BounceRate float64 `json:"bounce_rate"`
}

// ReportReply is one reply in the list.
type ReportReply struct {
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Campaign  string    `json:"campaign"`
	Subject   string    `json:"subject"`
	RepliedAt time.Time `json:"replied_at"`
	// Classification is the reply's category, for example "interested".
	Classification string `json:"classification"`
	// Snippet is set only when the report shows reply snippets.
	Snippet string `json:"snippet,omitempty"`
}

// ReportMailboxes is the mailbox health and warmup summary.
type ReportMailboxes struct {
	Total          int `json:"total"`
	Healthy        int `json:"healthy"`
	Warning        int `json:"warning"`
	Error          int `json:"error"`
	WarmupSent     int `json:"warmup_sent"`
	WarmupReceived int `json:"warmup_received"`
	WarmupReplied  int `json:"warmup_replied"`
}

func reportPath(id string) string { return "reports/" + url.PathEscape(id) }

// deleteFor issues a body-less DELETE and decodes the answer into a freshly
// allocated T, for the DELETE routes that return the resource as it now
// stands.
func deleteFor[T any](ctx context.Context, c *Client, path string, opts []RequestOption) (*T, *Response, error) {
	out := new(T)
	resp, err := c.call(ctx, http.MethodDelete, path, nil, out, opts)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// List returns a page of the workspace's reports, newest first. Limit is 1 to
// 100 (default 50). Needs the read_analytics scope.
func (s *ReportService) List(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[Report], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[Report](ctx, s.client, "reports", q, opts...)
}

// Get returns one report. Needs the read_analytics scope.
func (s *ReportService) Get(ctx context.Context, id string, opts ...RequestOption) (*Report, *Response, error) {
	return fetch[Report](ctx, s.client, reportPath(id), opts)
}

// Preview renders the report as its reader sees it. dateRange is one of the
// Report* range constants; empty uses the report's own range. Needs the
// read_analytics scope.
func (s *ReportService) Preview(ctx context.Context, id, dateRange string, opts ...RequestOption) (*ReportView, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "range", dateRange)
	return fetch[ReportView](ctx, s.client, withQuery(reportPath(id)+"/preview", q), opts)
}

// Create saves a report (201). A workspace keeps at most
// [ReportMaxPerWorkspace]; past that it is a 409 "report_limit_reached".
// Needs the manage_reports scope.
func (s *ReportService) Create(ctx context.Context, params *ReportCreateParams, opts ...RequestOption) (*Report, *Response, error) {
	return send[Report](ctx, s.client.post, "reports", params, opts)
}

// Update changes a report. Needs the manage_reports scope.
func (s *ReportService) Update(ctx context.Context, id string, params *ReportUpdateParams, opts ...RequestOption) (*Report, *Response, error) {
	return send[Report](ctx, s.client.patch, reportPath(id), params, opts)
}

// Delete removes a report, its link and its schedule. Needs the
// manage_reports scope. Answers 204.
func (s *ReportService) Delete(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, reportPath(id), opts...)
}

// CreateLink mints a share link, replacing any earlier one so the old URL
// stops opening. The answer (201) is the only time the URL is shown. params
// may be nil. An instance with no portal service configured answers 422
// "report_links_unavailable". Needs the manage_reports scope.
func (s *ReportService) CreateLink(ctx context.Context, id string, params *ReportLinkParams, opts ...RequestOption) (*ReportLinkResult, *Response, error) {
	if params == nil {
		params = &ReportLinkParams{}
	}
	return send[ReportLinkResult](ctx, s.client.post, reportPath(id)+"/link", params, opts)
}

// RevokeLink stops the report's share link from opening and returns the
// report. Needs the manage_reports scope.
func (s *ReportService) RevokeLink(ctx context.Context, id string, opts ...RequestOption) (*Report, *Response, error) {
	return deleteFor[Report](ctx, s.client, reportPath(id)+"/link", opts)
}

// SetSchedule emails the report on a schedule, replacing any earlier one, and
// returns the report. An invalid schedule is a 400
// "invalid_report_schedule"; an instance without outgoing mail answers 422
// "report_emails_unavailable". Needs the manage_reports scope.
func (s *ReportService) SetSchedule(ctx context.Context, id string, params *ReportScheduleParams, opts ...RequestOption) (*Report, *Response, error) {
	return send[Report](ctx, s.client.put, reportPath(id)+"/schedule", params, opts)
}

// DeleteSchedule stops emailing the report and returns it. Needs the
// manage_reports scope.
func (s *ReportService) DeleteSchedule(ctx context.Context, id string, opts ...RequestOption) (*Report, *Response, error) {
	return deleteFor[Report](ctx, s.client, reportPath(id)+"/schedule", opts)
}
