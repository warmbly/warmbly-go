package warmbly

import (
	"context"
	"encoding/json"
	"net/url"
	"time"
)

// CampaignService manages outreach campaigns: their schedule and sending
// policy, their sequence steps, their sender pool, linked segments, A/B
// variants, attachments and the preflight checks run before they go live.
//
// A campaign is an ordered sequence of steps (emails, waits and actions)
// delivered to enrolled contacts from one or more connected mailboxes. Steps
// are addressed as a sub-resource under their campaign.
type CampaignService service

// Campaign lifecycle states returned in [Campaign.Status].
//
// The server stores draft, active, paused, completed and the paused_* variants.
// Every paused_* variant is a self-inflicted pause with a specific cause and
// is restartable with [CampaignService.Start] once the cause is fixed.
// "Waiting for leads" is not a status: a continuous campaign that has run out
// of leads stays active and sets [Campaign.IdleSince] instead.
const (
	CampaignStatusDraft     = "draft"
	CampaignStatusScheduled = "scheduled"
	CampaignStatusActive    = "active"
	CampaignStatusPaused    = "paused"
	// CampaignStatusPausedNoAccounts is set when the campaign loses every
	// sender, or no sender can send under its settings (a failing SPF/DMARC
	// domain, a sending-behavior profile with no working days).
	CampaignStatusPausedNoAccounts = "paused_no_accounts"
	// CampaignStatusPausedTrialExpired is set when the organization's trial
	// ends while the campaign is running.
	CampaignStatusPausedTrialExpired = "paused_trial_expired"
	// CampaignStatusPausedGuardrail is set when an auto-pause guardrail trips;
	// [Campaign.GuardrailReason] says which one.
	CampaignStatusPausedGuardrail = "paused_guardrail"
	// CampaignStatusPausedUndeliverable is set when address verification has
	// refused every remaining lead. Re-verify them or mark them deliverable
	// (POST /contacts/verification) to resume; starting again from this
	// status is not gated by the list bounce-risk check.
	CampaignStatusPausedUndeliverable = "paused_undeliverable"
	CampaignStatusCompleted           = "completed"
	CampaignStatusStopped             = "stopped"
)

// Campaign kinds returned in [Campaign.Kind]. The kind is fixed at creation.
const (
	// CampaignKindSequence is the multi-step default.
	CampaignKindSequence = "sequence"
	// CampaignKindOneTime is a single message with no follow-ups. It is a
	// normal campaign underneath (same pool, caps, suppression and analytics)
	// but accepts at most one email step and refuses further ones.
	CampaignKindOneTime = "one_time"
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

// Error codes ([Error.Code]) a [CampaignService.Start] can refuse with.
const (
	// ErrCodeListBounceRisk is returned (400) when the list's projected bounce
	// rate is too high: above 4% of known-invalid addresses among deliverable
	// leads, on lists of at least 50. Launch anyway with
	// [CampaignStartParams.AcknowledgeListRisk].
	ErrCodeListBounceRisk = "list_bounce_risk"
	// ErrCodeLeadsUndeliverable is returned (400) when every remaining lead was
	// refused by address verification. The campaign is parked at
	// [CampaignStatusPausedUndeliverable].
	ErrCodeLeadsUndeliverable = "leads_undeliverable"
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
	// Kind is [CampaignKindSequence] or [CampaignKindOneTime].
	Kind string `json:"kind"`

	// StopOnReply halts a contact's sequence as soon as they reply.
	StopOnReply bool `json:"stop_on_reply"`
	// OpenTracking enables open tracking via a tracking pixel.
	OpenTracking bool `json:"open_tracking"`
	// LinkTracking enables click tracking by rewriting links.
	LinkTracking bool `json:"link_tracking"`
	// TextOnly sends the plain-text body only, with no HTML part.
	TextOnly bool `json:"text_only"`
	// DailyLimit caps sends per mailbox per day for this campaign; each
	// mailbox sends the smaller of this and its own cap.
	DailyLimit int `json:"daily_limit"`
	// UnsubscribeHeader adds RFC 8058 List-Unsubscribe headers.
	UnsubscribeHeader bool `json:"unsubscribe_header"`
	// RiskyEmails allows sending to addresses verification flagged as risky.
	RiskyEmails bool `json:"risky_emails"`
	// UnsubscribeMode picks the in-body opt-out appended after the signature:
	// [UnsubscribeModeInherit] follows the organization setting in
	// [OutreachSettings.Unsubscribe]; the other UnsubscribeMode* constants
	// override it for this campaign.
	UnsubscribeMode string `json:"unsubscribe_mode"`

	// CC and BCC are copied on every send.
	CC  []string `json:"cc"`
	BCC []string `json:"bcc"`

	// StartDate and EndDate bound the active sending window. Both are nullable:
	// a nil StartDate means "start now" and a nil EndDate means open-ended.
	StartDate *time.Time `json:"start_date"`
	EndDate   *time.Time `json:"end_date"`
	// Timezone is the IANA timezone the schedule is interpreted in.
	Timezone string `json:"timezone"`
	// Days is a legacy weekday bitmask (bit 0 is Monday), superseded by
	// ScheduleWindows.
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

	// Continuous keeps the campaign active when it runs out of leads: instead
	// of completing it waits, with IdleSince set, and sends the sequence to
	// each lead as they arrive (from a linked segment, a form, the API or an
	// automation). Linking a segment turns it on. A continuous campaign can be
	// started with no leads at all; only its end date finishes it.
	Continuous bool `json:"continuous"`
	// IdleSince is set while a continuous campaign is waiting for leads and
	// cleared as soon as it has something to send again.
	IdleSince *time.Time `json:"idle_since,omitempty"`

	// Auto-pause guardrails. Rates are percentages (0-100) evaluated over a
	// rolling GuardrailWindowDays window every 15 minutes, and the campaign is
	// moved to [CampaignStatusPausedGuardrail] the moment a band is breached.
	// A rate of 0 disables that rule; GuardrailMinSample is the number of sends
	// in the window below which no rule fires. GuardrailWindowDays 0 measures
	// the campaign's whole history.
	//
	// Bounce and complaint rates are ceilings (pause at or above); the reply
	// rate is a floor (pause below). Off by default.
	GuardrailEnabled          bool    `json:"guardrail_enabled"`
	GuardrailBounceRateMax    float64 `json:"guardrail_bounce_rate_max"`
	GuardrailComplaintRateMax float64 `json:"guardrail_complaint_rate_max"`
	GuardrailReplyRateMin     float64 `json:"guardrail_reply_rate_min"`
	GuardrailMinSample        int     `json:"guardrail_min_sample"`
	GuardrailWindowDays       int     `json:"guardrail_window_days"`
	// GuardrailTrippedAt and GuardrailReason are server-owned: set when a
	// guardrail pauses the campaign and cleared when it is started again.
	GuardrailTrippedAt *time.Time `json:"guardrail_tripped_at,omitempty"`
	GuardrailReason    string     `json:"guardrail_reason,omitempty"`

	// TrackingDomain overrides the mailbox tracking domain for this campaign.
	// It is honored only once verified.
	TrackingDomain           string     `json:"tracking_domain"`
	TrackingDomainVerified   bool       `json:"tracking_domain_verified"`
	TrackingDomainVerifiedAt *time.Time `json:"tracking_domain_verified_at,omitempty"`

	// UTMTracking tags every http(s) link in the body with utm_* parameters at
	// send time; a link that already carries one keeps its own value. Empty
	// UTMSource, UTMMedium and UTMCampaign mean the defaults ("warmbly",
	// "email" and the campaign name as a slug); utm_content is always the
	// link's own text. Off for campaigns created through the API unless sent.
	UTMTracking bool   `json:"utm_tracking"`
	UTMSource   string `json:"utm_source"`
	UTMMedium   string `json:"utm_medium"`
	UTMCampaign string `json:"utm_campaign"`

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
	// Position is the step's zero-based index in the sequence. It orders the
	// canvas and picks the entry step; it never advances a contact by itself.
	Position int `json:"position"`

	// X and Y are the step's coordinates on the sequence canvas. They are
	// written only through [CampaignService.UpdateStepLayout].
	X float64 `json:"x"`
	Y float64 `json:"y"`

	// Conditions is the step's routing: the connections out of it, evaluated
	// against the contact's engagement to pick the next step. Routing follows
	// connections only, so a step with an empty tree (no branches) has no
	// outgoing path and the contact's flow ends there; a plain "go there next"
	// link is a branch with no conditions. It is left as raw JSON because the
	// branch grammar evolves independently of this SDK.
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

	// Conditions replaces the step's outgoing connections when non-nil. Send
	// an empty object to remove them all, after which the contact's flow ends
	// at this step.
	Conditions json.RawMessage `json:"conditions,omitempty"`

	// Kind and Action switch the node between an email and an action or wait.
	// A one-time campaign refuses a second email step.
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
	// EventType names what happened, for example "started", "created" (also
	// written for a duplicate, with source_campaign_id in Metadata) or "idle"
	// when a continuous campaign runs out of leads and waits.
	EventType string         `json:"event_type"`
	Message   string         `json:"message"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// CampaignsOverview backs the campaigns browser: status-bucket counts across
// the organization plus per-folder totals. Paused sums every paused_* variant;
// OneTime counts campaigns of [CampaignKindOneTime] whatever their status.
type CampaignsOverview struct {
	Total     int64                 `json:"total"`
	Active    int64                 `json:"active"`
	Paused    int64                 `json:"paused"`
	Draft     int64                 `json:"draft"`
	Completed int64                 `json:"completed"`
	OneTime   int64                 `json:"one_time"`
	Folders   []CampaignFolderCount `json:"folders"`
}

// CampaignFolderCount is one folder's campaign total.
type CampaignFolderCount struct {
	FolderID string `json:"folder_id"`
	Total    int64  `json:"total"`
}

// CampaignEstimateParams projects an audience against a sender pool before a
// campaign exists. Only SegmentIDs is required. Nothing is written.
type CampaignEstimateParams struct {
	// SegmentIDs make up the audience (at most 20). A contact in several of
	// them is counted once.
	SegmentIDs []string `json:"segment_ids"`
	// EmailTagIDs resolve the mailbox pool. Empty means every active mailbox
	// in the organization.
	EmailTagIDs []string `json:"email_tag_ids,omitempty"`
	// DailyLimit is the per-mailbox campaign cap to apply (default 50). Each
	// mailbox counts the smaller of this and its own cap.
	DailyLimit *int `json:"daily_limit,omitempty"`
	// Days is the weekday bitmask of sending days, bit 0 being Monday.
	// Defaults to weekdays.
	Days *uint8 `json:"days,omitempty"`
	// Timezone is the IANA timezone the days are counted in. Defaults to UTC.
	Timezone *string `json:"timezone,omitempty"`
	// StartDate is when sending begins. Omit for now.
	StartDate *time.Time `json:"start_date,omitempty"`
}

// CampaignEstimateResult is the projection from [CampaignService.Estimate]. It
// applies the scheduler's cap rule but none of its pacing, so it is the
// earliest the last send can land, not a promise.
type CampaignEstimateResult struct {
	Recipients int `json:"recipients"`
	Mailboxes  int `json:"mailboxes"`
	// DailyCapacity is the pool's per-day ceiling under the campaign limit;
	// RemainingToday subtracts what the mailboxes already sent today.
	DailyCapacity  int `json:"daily_capacity"`
	RemainingToday int `json:"remaining_today"`
	// SendingDays is how many sending days the audience needs and
	// EstimatedFinishAt the calendar day the last send lands on. Both are nil
	// when the audience is empty, the pool has no capacity, or the send would
	// take longer than two years.
	SendingDays       *int       `json:"sending_days"`
	EstimatedFinishAt *time.Time `json:"estimated_finish_at"`
}

// CampaignDuplicateParams is the optional body of [CampaignService.Duplicate].
type CampaignDuplicateParams struct {
	// Name is the copy's name, 3 to 50 characters. Empty defaults to the source
	// name with " (copy)" appended.
	Name string `json:"name,omitempty"`
}

// CampaignStartParams qualifies a [CampaignService.StartWithOptions] request.
type CampaignStartParams struct {
	// AcknowledgeListRisk launches past the bounce-risk gate
	// ([ErrCodeListBounceRisk]), for a list verified elsewhere.
	AcknowledgeListRisk bool `json:"acknowledge_list_risk"`
}

// CampaignStatusChange confirms a start or stop. Status is "started" or
// "stopped"; fetch the campaign for its resulting state.
type CampaignStatusChange struct {
	Status string `json:"status"`
}

// CampaignAdvancedSettings is a campaign's override of the organization-wide
// outreach policy.
type CampaignAdvancedSettings struct {
	CampaignID string           `json:"campaign_id"`
	Overrides  OutreachSettings `json:"overrides"`
	UpdatedAt  time.Time        `json:"updated_at"`
}

// CampaignSegmentLink is one segment linked to a campaign as a live audience
// source. The counts are evaluated when asked: ContactCount is how many
// contacts the segment matches now, LeadCount how many of them are leads of
// this campaign, and HeldOutCount how many are not leads because they were
// removed from the campaign by hand and are never re-added automatically.
type CampaignSegmentLink struct {
	SegmentID    string    `json:"segment_id"`
	Name         string    `json:"name"`
	Color        string    `json:"color"`
	Description  string    `json:"description"`
	ContactCount int       `json:"contact_count"`
	LeadCount    int       `json:"lead_count"`
	HeldOutCount int       `json:"held_out_count"`
	LinkedAt     time.Time `json:"linked_at"`
}

// CampaignSegmentsResult is the outcome of [CampaignService.SetSegments]: the
// resulting links plus how many leads the call enrolled. Added is 0 when every
// member was already a lead, the segments match no contacts yet, or the only
// members are held out; the per-link counts tell these apart.
type CampaignSegmentsResult struct {
	Segments []CampaignSegmentLink `json:"data"`
	Added    int                   `json:"added"`
}

// CampaignFormStats is one form the campaign's emails link to, with what the
// campaign's recipients did with it. LinksSent counts personalized links
// minted for recipients; Viewers, Starters and Submissions are distinct
// recipients who opened, began and completed it.
type CampaignFormStats struct {
	FormID   string `json:"form_id"`
	FormName string `json:"form_name"`
	// PublicID is the form's public slug.
	PublicID string `json:"public_id"`
	// Status is the form's publication status.
	Status      string `json:"status"`
	LinksSent   int64  `json:"links_sent"`
	Viewers     int64  `json:"viewers"`
	Starters    int64  `json:"starters"`
	Submissions int64  `json:"submissions"`
	// ShareURL is the form's public URL, when it is published.
	ShareURL string `json:"share_url,omitempty"`
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
	ID         string `json:"id"`
	CampaignID string `json:"campaign_id"`
	// StepID is the step the file is sent with; nil means it rides every step
	// of the campaign. The field is always present.
	StepID   *string `json:"step_id"`
	Filename string  `json:"filename"`
	Size     int64   `json:"size"`
	MimeType string  `json:"mime_type"`
	// URL is a presigned download link, valid for about 15 minutes after the
	// response; fetch the attachment again for a fresh one.
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
// The test carries the step's attachments, the mailbox signature and the
// opt-out footer, rendered from what is saved (not unsaved edits). Opens and
// clicks on a test are not tracked and its opt-out link names nobody.
type TestEmailParams struct {
	// AccountID is the mailbox the test is sent from.
	AccountID string `json:"account_id"`
	Recipient string `json:"recipient"`
	// StepID selects which step to render; the first step is used when empty.
	StepID string `json:"step_id,omitempty"`
	// ContactID renders the copy for a real contact of the organization
	// instead of the built-in sample one. It requires contact read access on
	// top of the campaign permission.
	ContactID string `json:"contact_id,omitempty"`
}

// TestEmailResult confirms a test send.
type TestEmailResult struct {
	Message   string `json:"message"`
	Recipient string `json:"recipient"`
	Subject   string `json:"subject"`
	AccountID string `json:"account_id"`
	// StepID is the step that was rendered.
	StepID string `json:"step_id"`
	// ContactID echoes the contact rendered for, when one was given.
	ContactID string `json:"contact_id,omitempty"`
}

// TemplatePreviewParams renders campaign copy exactly as the send path would,
// so merge tags can be checked without sending anything. With no context it
// renders against a built-in sample contact; CampaignID, AccountID, ContactID
// and StepID add the pieces of a real send.
type TemplatePreviewParams struct {
	Subject   string `json:"subject,omitempty"`
	BodyHTML  string `json:"body_html,omitempty"`
	BodyPlain string `json:"body_plain,omitempty"`
	// Contact overrides individual fields of the contact rendered for (the
	// sample one, or the one named by ContactID).
	Contact *TemplatePreviewInput `json:"contact,omitempty"`
	// ContactID renders for a real contact of the organization. It requires
	// contact read access on top of the campaign permission.
	ContactID string `json:"contact_id,omitempty"`
	// CampaignID applies the campaign's opt-out footer and plain-text rule and
	// lists its campaign-wide attachments in the result.
	CampaignID string `json:"campaign_id,omitempty"`
	// AccountID applies that mailbox's signature and fills
	// [TemplatePreviewResult.From].
	AccountID string `json:"account_id,omitempty"`
	// StepID adds that step's own attachments to the campaign-wide ones. It
	// only means something with CampaignID.
	StepID string `json:"step_id,omitempty"`
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
// resolve. Tracking rewrites are left out; everything else the send path adds
// (signature, opt-out footer, sender, attachments) is included when the
// request named the campaign and mailbox.
type TemplatePreviewResult struct {
	Subject   string `json:"subject"`
	BodyHTML  string `json:"body_html"`
	BodyPlain string `json:"body_plain"`
	// Errors are template syntax problems. They block sending.
	Errors []string `json:"errors,omitempty"`
	// Unresolved lists merge tags with no value on the contact.
	Unresolved []string `json:"unresolved,omitempty"`
	// From is the sender as the recipient will see it, when AccountID was
	// given.
	From *TemplatePreviewFrom `json:"from,omitempty"`
	// Attachments are the files the send would carry, metadata only.
	Attachments []TemplatePreviewAttachment `json:"attachments,omitempty"`
}

// TemplatePreviewFrom is the sender line of a rendered preview.
type TemplatePreviewFrom struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// TemplatePreviewAttachment is one attachment a previewed send would carry.
type TemplatePreviewAttachment struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	MimeType string `json:"mime_type"`
}

// CampaignCreateParams creates a campaign. Only Name is required; every other
// field falls back to a server default. The wizard sends everything at once,
// while a simple create can send just a name and description.
//
// Segments are linked after creation with [CampaignService.SetSegments], and
// auto-pause guardrails are configured with [CampaignService.Update].
type CampaignCreateParams struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Kind is [CampaignKindSequence] (the default) or [CampaignKindOneTime].
	// It is fixed at creation. A one-time email accepts at most one entry in
	// Steps.
	Kind *string `json:"kind,omitempty"`

	StopOnReply       *bool `json:"stop_on_reply,omitempty"`
	OpenTracking      *bool `json:"open_tracking,omitempty"`
	LinkTracking      *bool `json:"link_tracking,omitempty"`
	TextOnly          *bool `json:"text_only,omitempty"`
	DailyLimit        *int  `json:"daily_limit,omitempty"`
	UnsubscribeHeader *bool `json:"unsubscribe_header,omitempty"`
	RiskyEmails       *bool `json:"risky_emails,omitempty"`
	// UnsubscribeMode is one of the UnsubscribeMode* constants; the default
	// is [UnsubscribeModeInherit].
	UnsubscribeMode *string `json:"unsubscribe_mode,omitempty"`

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
	// Continuous keeps the campaign active and waiting when it runs out of
	// leads; see [Campaign.Continuous].
	Continuous     *bool   `json:"continuous,omitempty"`
	TrackingDomain *string `json:"tracking_domain,omitempty"`

	// UTMTracking is off unless sent; empty UTMSource, UTMMedium and
	// UTMCampaign keep the defaults. See [Campaign.UTMTracking].
	UTMTracking *bool   `json:"utm_tracking,omitempty"`
	UTMSource   *string `json:"utm_source,omitempty"`
	UTMMedium   *string `json:"utm_medium,omitempty"`
	UTMCampaign *string `json:"utm_campaign,omitempty"`

	// Steps seeds the sequence in order, each connected to the previous one
	// with its wait set. Steps can equally be added afterwards with
	// [CampaignService.CreateStep].
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
// Changing any schedule field (StartDate, EndDate, Timezone, Days, StartTime,
// EndTime, ScheduleWindows) on an active campaign reschedules its next send
// immediately. The explicit sender list is not editable here; use
// [CampaignService.ReplaceSenders]. Linked segments live under
// [CampaignService.SetSegments].
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
	// UnsubscribeMode is one of the UnsubscribeMode* constants.
	UnsubscribeMode *string `json:"unsubscribe_mode,omitempty"`

	CC  []string `json:"cc,omitempty"`
	BCC []string `json:"bcc,omitempty"`

	// StartDate and EndDate set the sending window. To clear a stored date
	// (start now / run open-ended) leave the pointer nil and set
	// ClearStartDate or ClearEndDate, which sends an explicit null.
	StartDate *time.Time `json:"start_date,omitempty"`
	EndDate   *time.Time `json:"end_date,omitempty"`
	// ClearStartDate and ClearEndDate null out the stored dates. They take
	// precedence over StartDate and EndDate.
	ClearStartDate bool `json:"-"`
	ClearEndDate   bool `json:"-"`

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
	// Continuous keeps the campaign active and waiting when it runs out of
	// leads; see [Campaign.Continuous]. Turning it off has the campaign finish
	// once every lead is done.
	Continuous     *bool   `json:"continuous,omitempty"`
	TrackingDomain *string `json:"tracking_domain,omitempty"`

	// UTM tagging; see [Campaign.UTMTracking].
	UTMTracking *bool   `json:"utm_tracking,omitempty"`
	UTMSource   *string `json:"utm_source,omitempty"`
	UTMMedium   *string `json:"utm_medium,omitempty"`
	UTMCampaign *string `json:"utm_campaign,omitempty"`

	// Auto-pause guardrails; see [Campaign.GuardrailEnabled]. Rates are
	// percentages in [0,100] with 0 disabling the rule, GuardrailMinSample is
	// 1-100000 and GuardrailWindowDays 0-365. The tripped-at marker and reason
	// are server-owned and cleared by the next start.
	GuardrailEnabled          *bool    `json:"guardrail_enabled,omitempty"`
	GuardrailBounceRateMax    *float64 `json:"guardrail_bounce_rate_max,omitempty"`
	GuardrailComplaintRateMax *float64 `json:"guardrail_complaint_rate_max,omitempty"`
	GuardrailReplyRateMin     *float64 `json:"guardrail_reply_rate_min,omitempty"`
	GuardrailMinSample        *int     `json:"guardrail_min_sample,omitempty"`
	GuardrailWindowDays       *int     `json:"guardrail_window_days,omitempty"`
}

// MarshalJSON emits an explicit null for start_date/end_date when
// ClearStartDate/ClearEndDate is set, since the API distinguishes an absent
// field (unchanged) from a null one (cleared).
func (p CampaignUpdateParams) MarshalJSON() ([]byte, error) {
	type plain CampaignUpdateParams
	out := struct {
		plain
		StartDate json.RawMessage `json:"start_date,omitempty"`
		EndDate   json.RawMessage `json:"end_date,omitempty"`
	}{plain: plain(p)}
	var err error
	if out.StartDate, err = nullableDateJSON(p.StartDate, p.ClearStartDate); err != nil {
		return nil, err
	}
	if out.EndDate, err = nullableDateJSON(p.EndDate, p.ClearEndDate); err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

// nullableDateJSON renders a PATCH date field: null when clearing, the time
// when set, nothing (nil) when untouched.
func nullableDateJSON(t *time.Time, clearIt bool) (json.RawMessage, error) {
	if clearIt {
		return json.RawMessage("null"), nil
	}
	if t == nil {
		return nil, nil
	}
	return json.Marshal(t)
}

// CampaignListParams filters and paginates a list of campaigns.
type CampaignListParams struct {
	ListOptions
	// Query is a free-text filter on campaign name.
	Query string
	// Folder restricts the list to a single folder id.
	Folder string
	// Status is a bucket filter: [CampaignStatusDraft], [CampaignStatusActive],
	// [CampaignStatusPaused] (matches every paused_* variant) or
	// [CampaignStatusCompleted]. Any other value is a 400.
	Status string
	// Kind is [CampaignKindSequence] or [CampaignKindOneTime]. Any other value
	// is a 400.
	Kind string
}

func (p *CampaignListParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	setNonEmpty(q, "q", p.Query)
	setNonEmpty(q, "folder", p.Folder)
	setNonEmpty(q, "status", p.Status)
	setNonEmpty(q, "kind", p.Kind)
	return q
}

// List returns a page of campaigns. The page total counts campaigns matching
// the Query, Folder and Status filters.
func (s *CampaignService) List(ctx context.Context, params *CampaignListParams, opts ...RequestOption) (*Page[Campaign], error) {
	return listJSON[Campaign](ctx, s.client, "campaigns", params.values(), opts...)
}

// Overview returns status-bucket and per-folder campaign counts for the
// organization.
func (s *CampaignService) Overview(ctx context.Context, opts ...RequestOption) (*CampaignsOverview, *Response, error) {
	return fetch[CampaignsOverview](ctx, s.client, "campaigns-overview", opts)
}

// Estimate projects an audience of segments against a sender pool before a
// campaign exists: how many contacts it resolves to, the pool's daily
// capacity and the day the last send is expected to land. It writes nothing,
// so it needs no idempotency key.
func (s *CampaignService) Estimate(ctx context.Context, params *CampaignEstimateParams, opts ...RequestOption) (*CampaignEstimateResult, *Response, error) {
	return send[CampaignEstimateResult](ctx, s.client.post, "campaigns-estimate", params, opts)
}

// Create creates a new campaign. It starts as a draft and sends nothing until
// started.
func (s *CampaignService) Create(ctx context.Context, params *CampaignCreateParams, opts ...RequestOption) (*Campaign, *Response, error) {
	return send[Campaign](ctx, s.client.post, "campaigns", params, opts)
}

// Get retrieves a single campaign by ID.
func (s *CampaignService) Get(ctx context.Context, id string, opts ...RequestOption) (*Campaign, *Response, error) {
	return fetch[Campaign](ctx, s.client, "campaigns/"+url.PathEscape(id), opts)
}

// Update modifies a campaign's settings.
func (s *CampaignService) Update(ctx context.Context, id string, params *CampaignUpdateParams, opts ...RequestOption) (*Campaign, *Response, error) {
	return send[Campaign](ctx, s.client.patch, "campaigns/"+url.PathEscape(id), params, opts)
}

// Delete permanently removes a campaign with its steps, lead progress and
// activity. A running campaign can be deleted directly: its pending sends are
// canceled with it. Contacts and emails already sent stay.
func (s *CampaignService) Delete(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "campaigns/"+url.PathEscape(id), opts...)
}

// Duplicate creates a draft copy of a campaign's configuration: every
// setting, the steps with their branch graph and canvas positions, tags,
// folders, the explicit sender list, A/B variants, advanced settings and
// attachments. Leads, progress, statistics, the activity log, the ramp level,
// a guardrail trip and any start or end date already in the past are not
// copied. The copy is owned by the caller, counts against the daily
// new-campaign throttle, and answers 201. params may be nil.
func (s *CampaignService) Duplicate(ctx context.Context, id string, params *CampaignDuplicateParams, opts ...RequestOption) (*Campaign, *Response, error) {
	return send[Campaign](ctx, s.client.post, "campaigns/"+url.PathEscape(id)+"/duplicate", params, opts)
}

// Start begins (or resumes) sending for a campaign. It works from draft, any
// paused status or completed; a completed campaign with nothing left to send
// re-completes with a 400, unless it is continuous, in which case it starts
// and waits for leads with [Campaign.IdleSince] set. Status changes are
// rate-limited to one per minute per campaign.
//
// The start can be refused with [ErrCodeListBounceRisk] or
// [ErrCodeLeadsUndeliverable] in [Error.Code]; see [CampaignService.StartWithOptions]
// to launch past the bounce-risk gate.
func (s *CampaignService) Start(ctx context.Context, id string, opts ...RequestOption) (*CampaignStatusChange, *Response, error) {
	return s.StartWithOptions(ctx, id, nil, opts...)
}

// StartWithOptions is [CampaignService.Start] with a body; params may be nil,
// in which case no body is sent.
func (s *CampaignService) StartWithOptions(ctx context.Context, id string, params *CampaignStartParams, opts ...RequestOption) (*CampaignStatusChange, *Response, error) {
	return send[CampaignStatusChange](ctx, s.client.post, "campaigns/"+url.PathEscape(id)+"/start", params, opts)
}

// Stop pauses sending for a campaign. Leads keep their place and resume from
// it on the next start.
func (s *CampaignService) Stop(ctx context.Context, id string, opts ...RequestOption) (*CampaignStatusChange, *Response, error) {
	return send[CampaignStatusChange](ctx, s.client.post, "campaigns/"+url.PathEscape(id)+"/stop", nil, opts)
}

// Logs returns a page of a campaign's activity log, newest first. Limit is
// capped at 100.
func (s *CampaignService) Logs(ctx context.Context, id string, params *ListOptions, opts ...RequestOption) (*Page[CampaignLogEntry], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[CampaignLogEntry](ctx, s.client, "campaigns/"+url.PathEscape(id)+"/logs", q, opts...)
}

// Preflight runs the pre-launch checks for a campaign without starting it.
func (s *CampaignService) Preflight(ctx context.Context, id string, opts ...RequestOption) (*PreflightResult, *Response, error) {
	return send[PreflightResult](ctx, s.client.post, "campaigns/"+url.PathEscape(id)+"/preflight", nil, opts)
}

// SendTestEmail sends a rendered preview of a step to a single address.
func (s *CampaignService) SendTestEmail(ctx context.Context, id string, params *TestEmailParams, opts ...RequestOption) (*TestEmailResult, *Response, error) {
	return send[TestEmailResult](ctx, s.client.post, "campaigns/"+url.PathEscape(id)+"/test-email", params, opts)
}

// PreviewTemplate renders campaign copy the way the send path would. It is
// not scoped to a campaign in the path (name one in the params to include its
// footer and attachments) and sends nothing.
func (s *CampaignService) PreviewTemplate(ctx context.Context, params *TemplatePreviewParams, opts ...RequestOption) (*TemplatePreviewResult, *Response, error) {
	return send[TemplatePreviewResult](ctx, s.client.post, "campaign-template-preview", params, opts)
}

// VerifyTrackingDomain re-resolves the campaign's tracking-domain override.
func (s *CampaignService) VerifyTrackingDomain(ctx context.Context, id string, opts ...RequestOption) (*TrackingDomainStatus, *Response, error) {
	return send[TrackingDomainStatus](ctx, s.client.post, "campaigns/"+url.PathEscape(id)+"/tracking-domain/verify", nil, opts)
}

// Forms returns the forms the campaign's emails link to, with what the
// campaign's recipients did with each.
func (s *CampaignService) Forms(ctx context.Context, id string, opts ...RequestOption) ([]CampaignFormStats, *Response, error) {
	return fetchData[CampaignFormStats](ctx, s.client, "campaigns/"+url.PathEscape(id)+"/forms", opts)
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
	return sendData[CampaignSender](ctx, s.client.put, "campaigns/"+url.PathEscape(id)+"/senders", body, opts)
}

// --- linked segments ---

// ListSegments returns the segments linked to the campaign as live audience
// sources, with their current member and lead counts.
func (s *CampaignService) ListSegments(ctx context.Context, id string, opts ...RequestOption) ([]CampaignSegmentLink, *Response, error) {
	return fetchData[CampaignSegmentLink](ctx, s.client, "campaigns/"+url.PathEscape(id)+"/segments", opts)
}

// SetSegments atomically replaces the campaign's linked segments (up to 20)
// and turns [Campaign.Continuous] on. Every current member of a newly linked
// segment is enrolled as a lead immediately, and contacts who enter a linked
// segment later are enrolled automatically within about two minutes.
// Enrolment is additive: a contact who leaves a segment keeps their lead row,
// and unlinking a segment stops future enrolment without touching existing
// leads. An active campaign wakes to send to the new leads; a completed one
// restarts through the launch checks when a linked segment grows.
//
// An empty segmentIDs detaches every segment (the SDK sends an explicit empty
// array, which the API requires). The links and the enrolment are written in
// one transaction, so retries are safe.
func (s *CampaignService) SetSegments(ctx context.Context, id string, segmentIDs []string, opts ...RequestOption) (*CampaignSegmentsResult, *Response, error) {
	if segmentIDs == nil {
		segmentIDs = []string{}
	}
	body := struct {
		SegmentIDs []string `json:"segment_ids"`
	}{SegmentIDs: segmentIDs}
	return send[CampaignSegmentsResult](ctx, s.client.put, "campaigns/"+url.PathEscape(id)+"/segments", body, opts)
}

// --- advanced settings ---

// AdvancedSettings returns the campaign's overrides of the organization
// outreach policy.
func (s *CampaignService) AdvancedSettings(ctx context.Context, id string, opts ...RequestOption) (*CampaignAdvancedSettings, *Response, error) {
	return fetch[CampaignAdvancedSettings](ctx, s.client, "campaigns/"+url.PathEscape(id)+"/advanced", opts)
}

// UpdateAdvancedSettings replaces the campaign's outreach-policy overrides.
// The API answers 204 with no body; read them back with
// [CampaignService.AdvancedSettings].
func (s *CampaignService) UpdateAdvancedSettings(ctx context.Context, id string, overrides *OutreachSettings, opts ...RequestOption) (*Response, error) {
	body := struct {
		Settings *OutreachSettings `json:"settings"`
	}{Settings: overrides}
	return s.client.patch(ctx, "campaigns/"+url.PathEscape(id)+"/advanced", body, nil, opts...)
}

// --- steps ---

// ListSteps returns the campaign's sequence steps in order.
func (s *CampaignService) ListSteps(ctx context.Context, id string, opts ...RequestOption) ([]Step, *Response, error) {
	return fetchSlice[Step](ctx, s.client, "campaigns/"+url.PathEscape(id)+"/steps", opts)
}

// CreateStep appends a blank step to the campaign's sequence. Fill it in with
// [CampaignService.UpdateStep]. New steps are not connected to anything: wire
// them in through the previous step's Conditions. A one-time campaign refuses
// a second email step.
func (s *CampaignService) CreateStep(ctx context.Context, id string, opts ...RequestOption) (*Step, *Response, error) {
	return send[Step](ctx, s.client.post, "campaigns/"+url.PathEscape(id)+"/steps", nil, opts)
}

// UpdateStep modifies a step's content, delay, routing or kind.
func (s *CampaignService) UpdateStep(ctx context.Context, id, stepID string, params *StepUpdateParams, opts ...RequestOption) (*Step, *Response, error) {
	return send[Step](ctx, s.client.patch, "campaigns/"+url.PathEscape(id)+"/steps/"+url.PathEscape(stepID), params, opts)
}

// DeleteStep removes a step from the campaign's sequence, along with the
// attachments scoped to it.
func (s *CampaignService) DeleteStep(ctx context.Context, id, stepID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "campaigns/"+url.PathEscape(id)+"/steps/"+url.PathEscape(stepID), opts...)
}

// UpdateStepLayout persists the canvas coordinates of a campaign's steps. It is
// cosmetic: it does not audit, does not bump the campaign's updated_at, and is
// last-write-wins, so retries are safe. At most 1000 positions per call.
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
	return send[ABVariant](ctx, s.client.post, "campaigns/"+url.PathEscape(id)+"/ab-variants", params, opts)
}

// UpdateABVariant modifies an A/B variant.
func (s *CampaignService) UpdateABVariant(ctx context.Context, id, variantID string, params *ABVariantUpdateParams, opts ...RequestOption) (*ABVariant, *Response, error) {
	return send[ABVariant](ctx, s.client.patch, "campaigns/"+url.PathEscape(id)+"/ab-variants/"+url.PathEscape(variantID), params, opts)
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

// ListAttachments returns the campaign's attachments, campaign-wide and
// per-step alike.
func (s *CampaignService) ListAttachments(ctx context.Context, id string, opts ...RequestOption) ([]CampaignAttachment, *Response, error) {
	return fetchData[CampaignAttachment](ctx, s.client, "campaigns/"+url.PathEscape(id)+"/attachments", opts)
}

// UploadAttachment attaches a file to the campaign. With stepID it is sent
// only with that step (which must belong to the campaign); without, it rides
// every step. Files are capped at 15 MB, executable and script types are
// refused, and the upload counts against the organization's storage quota
// (a 4xx with code "storage_limit_reached" when it would pass it). Answers
// 201.
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

// DeleteAttachment removes an attachment from the campaign and from storage.
func (s *CampaignService) DeleteAttachment(ctx context.Context, id, attachmentID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "campaigns/"+url.PathEscape(id)+"/attachments/"+url.PathEscape(attachmentID), opts...)
}
