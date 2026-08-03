package warmbly

import (
	"context"
	"encoding/json"
	"net/url"
	"time"
)

// CampaignService manages outreach campaigns: their schedule and sending
// policy, their sequence steps, their sender pool, A/B variants, attachments
// and the preflight checks run before they go live.
//
// A campaign is an ordered sequence of steps (emails, waits and actions)
// delivered to enrolled contacts from one or more connected mailboxes. Steps
// are addressed as a sub-resource under their campaign.
type CampaignService service

// Campaign lifecycle states returned in [Campaign.Status].
const (
	CampaignStatusDraft            = "draft"
	CampaignStatusScheduled        = "scheduled"
	CampaignStatusActive           = "active"
	CampaignStatusPaused           = "paused"
	CampaignStatusPausedNoAccounts = "paused_no_accounts"
	CampaignStatusCompleted        = "completed"
	CampaignStatusStopped          = "stopped"
)

// Sender-selection strategies for [Campaign.SenderStrategy].
const (
	// SenderStrategyTags resolves the sending pool from the mailbox tags in
	// [Campaign.EmailTags].
	SenderStrategyTags = "tags"
	// SenderStrategyExplicit uses the pool managed through
	// [CampaignService.Senders].
	SenderStrategyExplicit = "explicit"
)

// ESP-matching modes for [Campaign.ESPMatchMode], which bias sends towards a
// mailbox on the same provider as the recipient.
const (
	ESPMatchOff    = "off"
	ESPMatchPrefer = "prefer"
	ESPMatchStrict = "strict"
)

// TimeInterval is one sending window inside a day, in minutes since local
// midnight. End is exclusive and must be greater than Start and at most 1440.
type TimeInterval struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// ScheduleWindows is a campaign's per-day sending schedule, indexed by weekday
// with Sunday at 0. An empty day means no sending that day. When any day is
// populated this supersedes the legacy Days/StartTime/EndTime fields.
type ScheduleWindows [7][]TimeInterval

// IsEmpty reports whether no day carries an interval, in which case the legacy
// day/time fields apply.
func (w ScheduleWindows) IsEmpty() bool {
	for _, day := range w {
		if len(day) > 0 {
			return false
		}
	}
	return true
}

// Campaign is an outreach campaign as returned by the API.
type Campaign struct {
	ID             string  `json:"id"`
	UserID         string  `json:"user_id"`
	OrganizationID *string `json:"organization_id,omitempty"`

	Name        string `json:"name"`
	Description string `json:"description"`
	// Status is one of the CampaignStatus* constants.
	Status string `json:"status"`

	// StopOnReply halts a contact's sequence as soon as they reply.
	StopOnReply bool `json:"stop_on_reply"`
	// OpenTracking enables open tracking via a tracking pixel.
	OpenTracking bool `json:"open_tracking"`
	// LinkTracking enables click tracking by rewriting links.
	LinkTracking bool `json:"link_tracking"`
	// TextOnly sends the plain-text body only, with no HTML part.
	TextOnly bool `json:"text_only"`
	// DailyLimit caps new sends per day across the campaign.
	DailyLimit int `json:"daily_limit"`
	// UnsubscribeHeader adds an RFC 8058 List-Unsubscribe header.
	UnsubscribeHeader bool `json:"unsubscribe_header"`
	// RiskyEmails allows sending to addresses verification flagged as risky.
	RiskyEmails bool `json:"risky_emails"`

	// CC and BCC are copied on every send.
	CC  []string `json:"cc"`
	BCC []string `json:"bcc"`

	// StartDate and EndDate bound the active sending window, when set.
	StartDate *time.Time `json:"start_date"`
	EndDate   *time.Time `json:"end_date"`
	// Timezone is the IANA timezone the schedule is interpreted in.
	Timezone string `json:"timezone"`
	// Days is a legacy weekday bitmask, superseded by ScheduleWindows.
	Days uint8 `json:"days"`
	// StartTime and EndTime are legacy "HH:MM" bounds, superseded by
	// ScheduleWindows.
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	// ScheduleWindows is the authoritative per-day schedule when non-empty.
	ScheduleWindows ScheduleWindows `json:"schedule_windows"`

	// EmailTags holds the mailbox tag ids the sending pool is drawn from under
	// [SenderStrategyTags]. Folders holds the campaign's folder ids.
	EmailTags []string `json:"email_tags"`
	Folders   []string `json:"folders"`

	// ContactOrderBy, ContactOrderDir and ContactOrderField control the order
	// contacts are enrolled in.
	ContactOrderBy    string  `json:"contact_order_by"`
	ContactOrderDir   string  `json:"contact_order_dir"`
	ContactOrderField *string `json:"contact_order_field,omitempty"`

	// SenderStrategy is [SenderStrategyTags] or [SenderStrategyExplicit];
	// RotationMode picks how volume spreads across the chosen mailboxes.
	SenderStrategy string `json:"sender_strategy"`
	RotationMode   string `json:"rotation_mode"`
	// Senders is the explicit pool. It is loaded on demand rather than on every
	// read; use [CampaignService.Senders] to fetch it.
	Senders []CampaignSender `json:"senders,omitempty"`

	// RampEnabled gradually increases daily volume for the campaign. The ramp
	// only ever lowers volume: it is applied as a minimum against each
	// mailbox's own cap. RampLevel is server-managed and survives pause/resume.
	RampEnabled   bool       `json:"ramp_enabled"`
	RampStart     int        `json:"ramp_start"`
	RampIncrement int        `json:"ramp_increment"`
	RampCeiling   int        `json:"ramp_ceiling"`
	RampLevel     int        `json:"ramp_level"`
	RampLevelDate *time.Time `json:"ramp_level_date,omitempty"`

	// ESPMatchMode is [ESPMatchOff], [ESPMatchPrefer] or [ESPMatchStrict].
	ESPMatchMode string `json:"esp_match_mode"`

	// MaxNewLeadsPerDay throttles newly enrolled contacts; 0 means unlimited.
	MaxNewLeadsPerDay  int  `json:"max_new_leads_per_day"`
	PrioritizeNewLeads bool `json:"prioritize_new_leads"`

	// TrackingDomain overrides the mailbox tracking domain for this campaign.
	// It is honored only once verified.
	TrackingDomain           string     `json:"tracking_domain"`
	TrackingDomainVerified   bool       `json:"tracking_domain_verified"`
	TrackingDomainVerifiedAt *time.Time `json:"tracking_domain_verified_at,omitempty"`

	LastStatusChangeAt *time.Time `json:"last_status_change_at,omitempty"`

	UpdatedAt time.Time `json:"updated_at"`
	CreatedAt time.Time `json:"created_at"`
}

// CampaignSender is one mailbox in an explicit-strategy campaign's sender pool.
type CampaignSender struct {
	EmailAccountID string     `json:"email_account_id"`
	Weight         int        `json:"weight"`
	LastSentAt     *time.Time `json:"last_sent_at,omitempty"`
	Enabled        bool       `json:"enabled"`
}

// CampaignSenderInput adds or updates one mailbox in the sender pool.
type CampaignSenderInput struct {
	EmailAccountID string `json:"email_account_id"`
	Weight         *int   `json:"weight,omitempty"`
	Enabled        *bool  `json:"enabled,omitempty"`
}

// Step kinds returned in [Step.Kind].
const (
	// StepKindEmail renders and sends the step's subject and body.
	StepKindEmail = "email"
	// StepKindAction runs a side effect described by [Step.Action].
	StepKindAction = "action"
	// StepKindWait delays without sending.
	StepKindWait = "wait"
)

// Step is a single step in a campaign's sequence.
type Step struct {
	ID   string `json:"id"`
	Name string `json:"name"`

	Subject   string `json:"subject"`
	BodyPlain string `json:"body_plain"`
	BodyHTML  string `json:"body_html"`
	// BodySync keeps the plain-text body derived from the HTML body.
	BodySync bool `json:"body_sync"`
	// BodyCode reports whether the HTML body is edited as raw markup.
	BodyCode bool `json:"body_code"`

	// WaitAfter is the delay in minutes before the next step runs.
	WaitAfter int `json:"wait_after"`
	// Position is the step's zero-based index in the sequence.
	Position int `json:"position"`

	// X and Y are the step's coordinates on the sequence canvas. They are
	// written only through [CampaignService.UpdateStepLayout].
	X float64 `json:"x"`
	Y float64 `json:"y"`

	// Conditions is the step's branching tree, evaluated against the contact's
	// engagement to pick the next step. It is left as raw JSON because the
	// branch grammar evolves independently of this SDK; an empty value means
	// plain linear progression.
	Conditions json.RawMessage `json:"conditions,omitempty"`

	// Kind is [StepKindEmail], [StepKindAction] or [StepKindWait].
	Kind string `json:"kind"`
	// Action is the typed configuration for a non-email node, as raw JSON. It
	// is an empty object for email steps.
	Action json.RawMessage `json:"action,omitempty"`

	UpdatedAt time.Time `json:"updated_at"`
	CreatedAt time.Time `json:"created_at"`
}

// StepUpdateParams updates a campaign step. Nil fields are left unchanged.
type StepUpdateParams struct {
	Name      *string `json:"name,omitempty"`
	Subject   *string `json:"subject,omitempty"`
	BodyPlain *string `json:"body_plain,omitempty"`
	BodyHTML  *string `json:"body_html,omitempty"`
	BodySync  *bool   `json:"body_sync,omitempty"`
	BodyCode  *bool   `json:"body_code,omitempty"`
	WaitAfter *int    `json:"wait_after,omitempty"`

	// Conditions replaces the branching tree when non-nil. Send an empty
	// object to clear branching and fall back to linear progression.
	Conditions json.RawMessage `json:"conditions,omitempty"`

	// Kind and Action switch the node between an email and an action or wait.
	Kind   *string         `json:"kind,omitempty"`
	Action json.RawMessage `json:"action,omitempty"`
}

// StepPosition is one step's coordinates on the sequence canvas.
type StepPosition struct {
	ID string  `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
}

// CampaignLogEntry is a single entry from a campaign's activity log.
type CampaignLogEntry struct {
	ID         string `json:"id"`
	CampaignID string `json:"campaign_id"`
	// EventType names what happened, for example "campaign_started".
	EventType string         `json:"event_type"`
	Message   string         `json:"message"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// CampaignsOverview backs the campaigns browser: status-bucket counts across
// the organization plus per-folder totals. Paused sums every paused variant.
type CampaignsOverview struct {
	Total     int64                 `json:"total"`
	Active    int64                 `json:"active"`
	Paused    int64                 `json:"paused"`
	Draft     int64                 `json:"draft"`
	Completed int64                 `json:"completed"`
	Folders   []CampaignFolderCount `json:"folders"`
}

// CampaignFolderCount is one folder's campaign total.
type CampaignFolderCount struct {
	FolderID string `json:"folder_id"`
	Total    int64  `json:"total"`
}

// CampaignAdvancedSettings is a campaign's override of the organization-wide
// outreach policy.
type CampaignAdvancedSettings struct {
	CampaignID string           `json:"campaign_id"`
	Overrides  OutreachSettings `json:"overrides"`
	UpdatedAt  time.Time        `json:"updated_at"`
}

// ABVariant is one arm of a campaign's A/B test.
type ABVariant struct {
	ID         string `json:"id"`
	CampaignID string `json:"campaign_id"`
	// StepID scopes the variant to a single step; nil applies it campaign-wide.
	StepID *string `json:"step_id"`
	Name   string  `json:"name"`
	// Weight is the variant's share of the split.
	Weight    int    `json:"weight"`
	Subject   string `json:"subject"`
	BodyHTML  string `json:"body_html"`
	BodyPlain string `json:"body_plain"`
	// IsControl marks the baseline arm the others are measured against.
	IsControl bool           `json:"is_control"`
	IsActive  bool           `json:"is_active"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// ABVariantCreateParams creates an A/B variant. Only Name is required.
type ABVariantCreateParams struct {
	Name      string         `json:"name"`
	StepID    string         `json:"step_id,omitempty"`
	Weight    *int           `json:"weight,omitempty"`
	Subject   string         `json:"subject,omitempty"`
	BodyHTML  string         `json:"body_html,omitempty"`
	BodyPlain string         `json:"body_plain,omitempty"`
	IsControl *bool          `json:"is_control,omitempty"`
	IsActive  *bool          `json:"is_active,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// ABVariantUpdateParams updates an A/B variant. Nil fields are unchanged.
type ABVariantUpdateParams struct {
	Name      *string        `json:"name,omitempty"`
	Weight    *int           `json:"weight,omitempty"`
	Subject   *string        `json:"subject,omitempty"`
	BodyHTML  *string        `json:"body_html,omitempty"`
	BodyPlain *string        `json:"body_plain,omitempty"`
	IsControl *bool          `json:"is_control,omitempty"`
	IsActive  *bool          `json:"is_active,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// ABAnalysis compares the arms of a campaign's A/B test.
type ABAnalysis struct {
	CampaignID string             `json:"campaign_id"`
	Variants   []ABVariantMetrics `json:"variants"`
	// WinnerID and WinnerName are nil until a winner clears the sample-size
	// and confidence bar.
	WinnerID   *string `json:"winner_id"`
	WinnerName *string `json:"winner_name"`
	// WinningRule is the metric the comparison was decided on.
	WinningRule string `json:"winning_rule"`
	// Confidence is a qualitative reading, for example "high" or "low".
	Confidence string `json:"confidence"`
}

// ABVariantMetrics is one variant's engagement in an [ABAnalysis].
type ABVariantMetrics struct {
	VariantID   string  `json:"variant_id"`
	VariantName string  `json:"variant_name"`
	TotalSent   int64   `json:"total_sent"`
	Opened      int64   `json:"opened"`
	Clicked     int64   `json:"clicked"`
	Replied     int64   `json:"replied"`
	Bounced     int64   `json:"bounced"`
	OpenRate    float64 `json:"open_rate"`
	ClickRate   float64 `json:"click_rate"`
	ReplyRate   float64 `json:"reply_rate"`
	BounceRate  float64 `json:"bounce_rate"`
}

// CampaignAttachment is a file attached to a campaign, optionally scoped to a
// single step.
type CampaignAttachment struct {
	ID         string  `json:"id"`
	CampaignID string  `json:"campaign_id"`
	StepID     *string `json:"step_id"`
	Filename   string  `json:"filename"`
	Size       int64   `json:"size"`
	MimeType   string  `json:"mime_type"`
	// URL is where the stored file can be downloaded.
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

// PreflightResult is the outcome of the pre-launch checks for a campaign.
type PreflightResult struct {
	ID             string `json:"id,omitempty"`
	OrganizationID string `json:"organization_id,omitempty"`
	CampaignID     string `json:"campaign_id"`
	// Passed is false when any blocking check failed.
	Passed bool `json:"passed"`
	// Score is a 0-100 readiness score.
	Score           int              `json:"score"`
	Checks          []PreflightCheck `json:"checks"`
	Recommendations []string         `json:"recommendations,omitempty"`
	CreatedAt       time.Time        `json:"created_at,omitempty"`
}

// PreflightCheck is a single pre-launch check.
type PreflightCheck struct {
	Key    string `json:"key"`
	Passed bool   `json:"passed"`
	// Severity is how much a failure matters, for example "error" or "warning".
	Severity    string `json:"severity"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

// TestEmailParams sends a rendered preview of a campaign step to one address.
type TestEmailParams struct {
	// AccountID is the mailbox the test is sent from.
	AccountID string `json:"account_id"`
	Recipient string `json:"recipient"`
	// StepID selects which step to render; the first step is used when empty.
	StepID string `json:"step_id,omitempty"`
}

// TestEmailResult confirms a test send.
type TestEmailResult struct {
	Message   string `json:"message"`
	Recipient string `json:"recipient"`
	Subject   string `json:"subject"`
	AccountID string `json:"account_id"`
}

// TemplatePreviewParams renders campaign copy against a sample contact so merge
// tags can be checked without sending anything.
type TemplatePreviewParams struct {
	Subject   string                `json:"subject,omitempty"`
	BodyHTML  string                `json:"body_html,omitempty"`
	BodyPlain string                `json:"body_plain,omitempty"`
	Contact   *TemplatePreviewInput `json:"contact,omitempty"`
}

// TemplatePreviewInput is the sample contact merge tags are resolved against.
type TemplatePreviewInput struct {
	FirstName    string            `json:"first_name,omitempty"`
	LastName     string            `json:"last_name,omitempty"`
	Email        string            `json:"email,omitempty"`
	Company      string            `json:"company,omitempty"`
	Phone        string            `json:"phone,omitempty"`
	CustomFields map[string]string `json:"custom_fields,omitempty"`
}

// TemplatePreviewResult is the rendered copy plus anything that failed to
// resolve.
type TemplatePreviewResult struct {
	Subject   string `json:"subject"`
	BodyHTML  string `json:"body_html"`
	BodyPlain string `json:"body_plain"`
	// Errors are template syntax problems.
	Errors []string `json:"errors,omitempty"`
	// Unresolved lists merge tags with no value on the sample contact.
	Unresolved []string `json:"unresolved,omitempty"`
}

// CampaignCreateParams creates a campaign. Only Name is required; every other
// field falls back to a server default. The wizard sends everything at once,
// while a simple create can send just a name and description.
type CampaignCreateParams struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`

	StopOnReply       *bool `json:"stop_on_reply,omitempty"`
	OpenTracking      *bool `json:"open_tracking,omitempty"`
	LinkTracking      *bool `json:"link_tracking,omitempty"`
	TextOnly          *bool `json:"text_only,omitempty"`
	DailyLimit        *int  `json:"daily_limit,omitempty"`
	UnsubscribeHeader *bool `json:"unsubscribe_header,omitempty"`
	RiskyEmails       *bool `json:"risky_emails,omitempty"`

	CC  []string `json:"cc,omitempty"`
	BCC []string `json:"bcc,omitempty"`

	StartDate       *time.Time       `json:"start_date,omitempty"`
	EndDate         *time.Time       `json:"end_date,omitempty"`
	Timezone        *string          `json:"timezone,omitempty"`
	Days            *uint8           `json:"days,omitempty"`
	StartTime       *string          `json:"start_time,omitempty"`
	EndTime         *string          `json:"end_time,omitempty"`
	ScheduleWindows *ScheduleWindows `json:"schedule_windows,omitempty"`

	// EmailTagIDs and FolderIDs reference tags and folders that already exist.
	EmailTagIDs []string `json:"email_tag_ids,omitempty"`
	FolderIDs   []string `json:"folder_ids,omitempty"`

	SenderStrategy *string               `json:"sender_strategy,omitempty"`
	RotationMode   *string               `json:"rotation_mode,omitempty"`
	Senders        []CampaignSenderInput `json:"senders,omitempty"`

	RampEnabled   *bool `json:"ramp_enabled,omitempty"`
	RampStart     *int  `json:"ramp_start,omitempty"`
	RampIncrement *int  `json:"ramp_increment,omitempty"`
	RampCeiling   *int  `json:"ramp_ceiling,omitempty"`

	ESPMatchMode       *string `json:"esp_match_mode,omitempty"`
	MaxNewLeadsPerDay  *int    `json:"max_new_leads_per_day,omitempty"`
	PrioritizeNewLeads *bool   `json:"prioritize_new_leads,omitempty"`
	TrackingDomain     *string `json:"tracking_domain,omitempty"`

	// Steps seeds the sequence in order. Steps can equally be added afterwards
	// with [CampaignService.CreateStep].
	Steps []StepInput `json:"steps,omitempty"`
	// Variants seeds A/B variants for the first step.
	Variants []ABVariantCreateParams `json:"variants,omitempty"`
	// AdvancedOverrides seeds this campaign's outreach-policy overrides.
	AdvancedOverrides *OutreachSettings `json:"advanced_overrides,omitempty"`
}

// StepInput is one step seeded during campaign creation.
type StepInput struct {
	Name      string `json:"name,omitempty"`
	Subject   string `json:"subject,omitempty"`
	BodyPlain string `json:"body_plain,omitempty"`
	BodyHTML  string `json:"body_html,omitempty"`
	BodySync  *bool  `json:"body_sync,omitempty"`
	BodyCode  *bool  `json:"body_code,omitempty"`
	WaitAfter *int   `json:"wait_after,omitempty"`
}

// CampaignUpdateParams updates a campaign. Nil fields are left unchanged, so a
// zero value is never mistaken for "clear this".
//
// The explicit sender list is not editable here; use
// [CampaignService.ReplaceSenders].
type CampaignUpdateParams struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Status      *string `json:"status,omitempty"`

	StopOnReply       *bool `json:"stop_on_reply,omitempty"`
	OpenTracking      *bool `json:"open_tracking,omitempty"`
	LinkTracking      *bool `json:"link_tracking,omitempty"`
	TextOnly          *bool `json:"text_only,omitempty"`
	DailyLimit        *int  `json:"daily_limit,omitempty"`
	UnsubscribeHeader *bool `json:"unsubscribe_header,omitempty"`
	RiskyEmails       *bool `json:"risky_emails,omitempty"`

	CC  []string `json:"cc,omitempty"`
	BCC []string `json:"bcc,omitempty"`

	StartDate       *time.Time       `json:"start_date,omitempty"`
	EndDate         *time.Time       `json:"end_date,omitempty"`
	Timezone        *string          `json:"timezone,omitempty"`
	Days            *uint8           `json:"days,omitempty"`
	StartTime       *string          `json:"start_time,omitempty"`
	EndTime         *string          `json:"end_time,omitempty"`
	ScheduleWindows *ScheduleWindows `json:"schedule_windows,omitempty"`

	EmailTags []string `json:"email_tags,omitempty"`
	Folders   []string `json:"folders,omitempty"`

	ContactOrderBy    *string `json:"contact_order_by,omitempty"`
	ContactOrderDir   *string `json:"contact_order_dir,omitempty"`
	ContactOrderField *string `json:"contact_order_field,omitempty"`

	SenderStrategy *string `json:"sender_strategy,omitempty"`
	RotationMode   *string `json:"rotation_mode,omitempty"`

	RampEnabled   *bool `json:"ramp_enabled,omitempty"`
	RampStart     *int  `json:"ramp_start,omitempty"`
	RampIncrement *int  `json:"ramp_increment,omitempty"`
	RampCeiling   *int  `json:"ramp_ceiling,omitempty"`

	ESPMatchMode       *string `json:"esp_match_mode,omitempty"`
	MaxNewLeadsPerDay  *int    `json:"max_new_leads_per_day,omitempty"`
	PrioritizeNewLeads *bool   `json:"prioritize_new_leads,omitempty"`
	TrackingDomain     *string `json:"tracking_domain,omitempty"`
}

// CampaignListParams filters and paginates a list of campaigns.
type CampaignListParams struct {
	ListOptions
	// Query is a free-text filter on campaign name.
	Query string
	// Folder restricts the list to a single folder id.
	Folder string
}

func (p *CampaignListParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	setNonEmpty(q, "q", p.Query)
	setNonEmpty(q, "folder", p.Folder)
	return q
}

// List returns a page of campaigns.
func (s *CampaignService) List(ctx context.Context, params *CampaignListParams, opts ...RequestOption) (*Page[Campaign], error) {
	return listJSON[Campaign](ctx, s.client, "campaigns", params.values(), opts...)
}

// Overview returns status-bucket and per-folder campaign counts for the
// organization.
func (s *CampaignService) Overview(ctx context.Context, opts ...RequestOption) (*CampaignsOverview, *Response, error) {
	return fetch[CampaignsOverview](ctx, s.client, "campaigns-overview", opts)
}

// Create creates a new campaign.
func (s *CampaignService) Create(ctx context.Context, params *CampaignCreateParams, opts ...RequestOption) (*Campaign, *Response, error) {
	return send[Campaign](ctx, s.client, s.client.post, "campaigns", params, opts)
}

// Get retrieves a single campaign by ID.
func (s *CampaignService) Get(ctx context.Context, id string, opts ...RequestOption) (*Campaign, *Response, error) {
	return fetch[Campaign](ctx, s.client, "campaigns/"+url.PathEscape(id), opts)
}

// Update modifies a campaign's settings.
func (s *CampaignService) Update(ctx context.Context, id string, params *CampaignUpdateParams, opts ...RequestOption) (*Campaign, *Response, error) {
	return send[Campaign](ctx, s.client, s.client.patch, "campaigns/"+url.PathEscape(id), params, opts)
}

// Delete permanently removes a campaign.
func (s *CampaignService) Delete(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "campaigns/"+url.PathEscape(id), opts...)
}

// Start begins (or resumes) sending for a campaign.
func (s *CampaignService) Start(ctx context.Context, id string, opts ...RequestOption) (*Campaign, *Response, error) {
	return send[Campaign](ctx, s.client, s.client.post, "campaigns/"+url.PathEscape(id)+"/start", nil, opts)
}

// Stop halts sending for a campaign.
func (s *CampaignService) Stop(ctx context.Context, id string, opts ...RequestOption) (*Campaign, *Response, error) {
	return send[Campaign](ctx, s.client, s.client.post, "campaigns/"+url.PathEscape(id)+"/stop", nil, opts)
}

// Logs returns a page of a campaign's activity log.
func (s *CampaignService) Logs(ctx context.Context, id string, params *ListOptions, opts ...RequestOption) (*Page[CampaignLogEntry], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[CampaignLogEntry](ctx, s.client, "campaigns/"+url.PathEscape(id)+"/logs", q, opts...)
}

// Preflight runs the pre-launch checks for a campaign without starting it.
func (s *CampaignService) Preflight(ctx context.Context, id string, opts ...RequestOption) (*PreflightResult, *Response, error) {
	return send[PreflightResult](ctx, s.client, s.client.post, "campaigns/"+url.PathEscape(id)+"/preflight", nil, opts)
}

// SendTestEmail sends a rendered preview of a step to a single address.
func (s *CampaignService) SendTestEmail(ctx context.Context, id string, params *TestEmailParams, opts ...RequestOption) (*TestEmailResult, *Response, error) {
	return send[TestEmailResult](ctx, s.client, s.client.post, "campaigns/"+url.PathEscape(id)+"/test-email", params, opts)
}

// PreviewTemplate renders campaign copy against a sample contact. It is not
// scoped to a campaign and sends nothing.
func (s *CampaignService) PreviewTemplate(ctx context.Context, params *TemplatePreviewParams, opts ...RequestOption) (*TemplatePreviewResult, *Response, error) {
	return send[TemplatePreviewResult](ctx, s.client, s.client.post, "campaign-template-preview", params, opts)
}

// VerifyTrackingDomain re-resolves the campaign's tracking-domain override.
func (s *CampaignService) VerifyTrackingDomain(ctx context.Context, id string, opts ...RequestOption) (*TrackingDomainStatus, *Response, error) {
	return send[TrackingDomainStatus](ctx, s.client, s.client.post, "campaigns/"+url.PathEscape(id)+"/tracking-domain/verify", nil, opts)
}

// --- sender pool ---

// Senders returns the campaign's explicit sender pool.
func (s *CampaignService) Senders(ctx context.Context, id string, opts ...RequestOption) ([]CampaignSender, *Response, error) {
	return fetchData[CampaignSender](ctx, s.client, "campaigns/"+url.PathEscape(id)+"/senders", opts)
}

// ReplaceSenders replaces the campaign's explicit sender pool wholesale.
func (s *CampaignService) ReplaceSenders(ctx context.Context, id string, senders []CampaignSenderInput, opts ...RequestOption) ([]CampaignSender, *Response, error) {
	body := struct {
		Senders []CampaignSenderInput `json:"senders"`
	}{Senders: senders}
	return sendData[CampaignSender](ctx, s.client, s.client.put, "campaigns/"+url.PathEscape(id)+"/senders", body, opts)
}

// --- advanced settings ---

// AdvancedSettings returns the campaign's overrides of the organization
// outreach policy.
func (s *CampaignService) AdvancedSettings(ctx context.Context, id string, opts ...RequestOption) (*CampaignAdvancedSettings, *Response, error) {
	return fetch[CampaignAdvancedSettings](ctx, s.client, "campaigns/"+url.PathEscape(id)+"/advanced", opts)
}

// UpdateAdvancedSettings replaces the campaign's outreach-policy overrides.
func (s *CampaignService) UpdateAdvancedSettings(ctx context.Context, id string, overrides *OutreachSettings, opts ...RequestOption) (*CampaignAdvancedSettings, *Response, error) {
	body := struct {
		Overrides *OutreachSettings `json:"overrides"`
	}{Overrides: overrides}
	return send[CampaignAdvancedSettings](ctx, s.client, s.client.patch, "campaigns/"+url.PathEscape(id)+"/advanced", body, opts)
}

// --- steps ---

// ListSteps returns the campaign's sequence steps in order.
func (s *CampaignService) ListSteps(ctx context.Context, id string, opts ...RequestOption) ([]Step, *Response, error) {
	return fetchSlice[Step](ctx, s.client, "campaigns/"+url.PathEscape(id)+"/steps", opts)
}

// CreateStep appends a blank step to the campaign's sequence. Fill it in with
// [CampaignService.UpdateStep].
func (s *CampaignService) CreateStep(ctx context.Context, id string, opts ...RequestOption) (*Step, *Response, error) {
	return send[Step](ctx, s.client, s.client.post, "campaigns/"+url.PathEscape(id)+"/steps", nil, opts)
}

// UpdateStep modifies a step's content, delay or branching.
func (s *CampaignService) UpdateStep(ctx context.Context, id, stepID string, params *StepUpdateParams, opts ...RequestOption) (*Step, *Response, error) {
	return send[Step](ctx, s.client, s.client.patch, "campaigns/"+url.PathEscape(id)+"/steps/"+url.PathEscape(stepID), params, opts)
}

// DeleteStep removes a step from the campaign's sequence.
func (s *CampaignService) DeleteStep(ctx context.Context, id, stepID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "campaigns/"+url.PathEscape(id)+"/steps/"+url.PathEscape(stepID), opts...)
}

// UpdateStepLayout persists the canvas coordinates of a campaign's steps. It is
// cosmetic: it does not audit, does not bump the campaign's updated_at, and is
// last-write-wins, so retries are safe.
func (s *CampaignService) UpdateStepLayout(ctx context.Context, id string, positions []StepPosition, opts ...RequestOption) (*Response, error) {
	body := struct {
		Positions []StepPosition `json:"positions"`
	}{Positions: positions}
	return s.client.patch(ctx, "campaigns/"+url.PathEscape(id)+"/step-layout", body, nil, opts...)
}

// --- A/B variants ---

// ListABVariants returns the campaign's A/B variants.
func (s *CampaignService) ListABVariants(ctx context.Context, id string, opts ...RequestOption) ([]ABVariant, *Response, error) {
	return fetchData[ABVariant](ctx, s.client, "campaigns/"+url.PathEscape(id)+"/ab-variants", opts)
}

// CreateABVariant adds an A/B variant to the campaign.
func (s *CampaignService) CreateABVariant(ctx context.Context, id string, params *ABVariantCreateParams, opts ...RequestOption) (*ABVariant, *Response, error) {
	return send[ABVariant](ctx, s.client, s.client.post, "campaigns/"+url.PathEscape(id)+"/ab-variants", params, opts)
}

// UpdateABVariant modifies an A/B variant.
func (s *CampaignService) UpdateABVariant(ctx context.Context, id, variantID string, params *ABVariantUpdateParams, opts ...RequestOption) (*ABVariant, *Response, error) {
	return send[ABVariant](ctx, s.client, s.client.patch, "campaigns/"+url.PathEscape(id)+"/ab-variants/"+url.PathEscape(variantID), params, opts)
}

// DeleteABVariant removes an A/B variant.
func (s *CampaignService) DeleteABVariant(ctx context.Context, id, variantID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "campaigns/"+url.PathEscape(id)+"/ab-variants/"+url.PathEscape(variantID), opts...)
}

// ABAnalysis compares the campaign's A/B arms and names a winner once the
// result is significant.
func (s *CampaignService) ABAnalysis(ctx context.Context, id string, opts ...RequestOption) (*ABAnalysis, *Response, error) {
	return fetch[ABAnalysis](ctx, s.client, "campaigns/"+url.PathEscape(id)+"/ab-analysis", opts)
}

// --- attachments ---

// ListAttachments returns the campaign's attachments.
func (s *CampaignService) ListAttachments(ctx context.Context, id string, opts ...RequestOption) ([]CampaignAttachment, *Response, error) {
	return fetchData[CampaignAttachment](ctx, s.client, "campaigns/"+url.PathEscape(id)+"/attachments", opts)
}

// UploadAttachment attaches a file to the campaign, optionally scoped to a
// single step.
func (s *CampaignService) UploadAttachment(ctx context.Context, id string, file *FileUpload, stepID string, opts ...RequestOption) (*CampaignAttachment, *Response, error) {
	fields := map[string]string{}
	if stepID != "" {
		fields["step_id"] = stepID
	}
	out := new(CampaignAttachment)
	resp, err := s.client.postMultipart(ctx, "campaigns/"+url.PathEscape(id)+"/attachments", "file", file, fields, out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// DeleteAttachment removes an attachment from the campaign.
func (s *CampaignService) DeleteAttachment(ctx context.Context, id, attachmentID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "campaigns/"+url.PathEscape(id)+"/attachments/"+url.PathEscape(attachmentID), opts...)
}
