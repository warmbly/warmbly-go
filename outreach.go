package warmbly

import "context"

// OutreachService reads and writes the organization-wide advanced outreach
// settings: the bounce pipeline, reply-intent classification, send-time
// optimization, preflight checks, the in-body unsubscribe line and the rest
// of the sending policy that campaigns inherit.
//
// A campaign can override any of these for itself; see
// [CampaignService.AdvancedSettings].
type OutreachService service

// In-body opt-out modes for [UnsubscribeSettings.Mode] and
// [Campaign.UnsubscribeMode]. The List-Unsubscribe header is a separate
// per-campaign flag ([Campaign.UnsubscribeHeader]).
const (
	// UnsubscribeModeInherit is valid only on a campaign: it follows the
	// organization's [UnsubscribeSettings]. Sent as the organization mode it
	// is normalized to [UnsubscribeModeText].
	UnsubscribeModeInherit = "inherit"
	// UnsubscribeModeText appends a plain sentence inviting a reply to opt
	// out. It is the default: it reads as a personal email, and a reply that
	// asks to stop is detected and honored automatically.
	UnsubscribeModeText = "text"
	// UnsubscribeModeLink appends a sentence with a real, signed unsubscribe
	// link, unique to the recipient and campaign and valid for a year.
	UnsubscribeModeLink = "link"
	// UnsubscribeModeOff appends nothing.
	UnsubscribeModeOff = "off"
)

// UnsubscribeSettings is the organization default for the opt-out appended
// after the signature of every campaign email. Copy fields are single lines
// of at most 300 characters; whitespace is collapsed and longer text is cut.
// Blank copy falls back to the server defaults at send time.
type UnsubscribeSettings struct {
	// Mode is [UnsubscribeModeText], [UnsubscribeModeLink] or
	// [UnsubscribeModeOff].
	Mode string `json:"mode"`
	// Text is the sentence appended in text mode. Default: "If this isn't
	// relevant, just reply and let me know and I won't email you again."
	Text string `json:"text"`
	// LinkIntro and LinkText make up the link-mode line, rendered as
	// "<intro> <a>text</a>". Defaults: "Not the right person, or not
	// interested?" and "Unsubscribe".
	LinkIntro string `json:"link_intro"`
	LinkText  string `json:"link_text"`
}

// BouncePipelineSettings controls automatic suppression and the circuit breaker
// that pauses a campaign when bounces or complaints spike.
type BouncePipelineSettings struct {
	Enabled                   bool `json:"enabled"`
	AutoSuppressOnBounce      bool `json:"auto_suppress_on_bounce"`
	AutoSuppressOnComplaint   bool `json:"auto_suppress_on_complaint"`
	AutoSuppressOnUnsubscribe bool `json:"auto_suppress_on_unsubscribe"`
	AutoPauseCampaignOnSpike  bool `json:"auto_pause_campaign_on_spike"`
	// PauseBounceRateThreshold and PauseComplaintRateThreshold are rates in
	// [0,1] at which a campaign is paused.
	PauseBounceRateThreshold    float64 `json:"pause_bounce_rate_threshold"`
	PauseComplaintRateThreshold float64 `json:"pause_complaint_rate_threshold"`
}

// TaskReliabilitySettings tunes send-task retries and the dead-letter queue.
type TaskReliabilitySettings struct {
	Enabled                bool `json:"enabled"`
	DLQEnabled             bool `json:"dlq_enabled"`
	MaxAttempts            int  `json:"max_attempts"`
	ExecutionWindowSeconds int  `json:"execution_window_seconds"`
}

// ABTestingSettings governs how A/B variants are compared and promoted.
type ABTestingSettings struct {
	Enabled bool `json:"enabled"`
	// DefaultWinningRule is the metric a winner is picked on, for example
	// "reply_rate".
	DefaultWinningRule string `json:"default_winning_rule"`
	AutoPromoteWinner  bool   `json:"auto_promote_winner"`
	MinSampleSize      int    `json:"min_sample_size"`
}

// ReplyIntentSettings configures keyword-based classification of inbound
// replies and the actions taken on each class.
type ReplyIntentSettings struct {
	Enabled             bool     `json:"enabled"`
	PositiveKeywords    []string `json:"positive_keywords"`
	NegativeKeywords    []string `json:"negative_keywords"`
	OutOfOfficeKeywords []string `json:"out_of_office_keywords"`
	QuestionKeywords    []string `json:"question_keywords"`
	AutoCreateCRMTask   bool     `json:"auto_create_crm_task"`
	// CRMTaskIntents narrows AutoCreateCRMTask to the reply intents worth a
	// follow-up task: any of the ReplyIntent* constants. A nil list means the
	// server default (every human intent, no automated one); an explicit empty
	// list means none, the same as turning the switch off. An entry that is not
	// a reply intent fails with code "invalid_setting".
	CRMTaskIntents          []string `json:"crm_task_intents"`
	AutoPauseOnNegative     bool     `json:"auto_pause_on_negative"`
	AutoSuppressOnUnsubWord bool     `json:"auto_suppress_on_unsubscribe_keyword"`
	// HoldOnOutOfOffice parks a contact's next step when an auto-reply says
	// they are away, and resumes it when they are back, rather than sending
	// into an empty desk.
	HoldOnOutOfOffice bool `json:"hold_on_out_of_office"`
	// OutOfOfficeHoldDays is the hold used when the auto-reply carries no
	// readable return date, clamped to 1 to 90 (default 7).
	OutOfOfficeHoldDays int `json:"out_of_office_hold_days"`
}

// Reply intents a classified reply can carry, for
// [ReplyIntentSettings.CRMTaskIntents]. The set can grow.
const (
	ReplyIntentPositive    = "positive"
	ReplyIntentNegative    = "negative"
	ReplyIntentOutOfOffice = "out_of_office"
	ReplyIntentQuestion    = "question"
	ReplyIntentNeutral     = "neutral"
	// ReplyIntentAutomated is a machine reply that is not a vacation notice:
	// an autoresponder, a ticket acknowledgement, a bounce or a delivery
	// report.
	ReplyIntentAutomated = "automated"
)

// InboxTaggingSettings are the actions a workspace lets a classified reply
// take. The three reversible ones default on; suppression defaults off because
// it is the one that cannot be undone.
type InboxTaggingSettings struct {
	// HoldOnNotNow parks the contact's sequences for NotNowHoldDays (1 to 90,
	// default 30) when they answer "not now".
	HoldOnNotNow   bool `json:"hold_on_not_now"`
	NotNowHoldDays int  `json:"not_now_hold_days"`
	// StopOnDeclined parks a contact with no end when they decline or say they
	// are the wrong person. The hold shows on the lead and a member lifts it;
	// nothing is unsubscribed or deleted.
	StopOnDeclined bool `json:"stop_on_declined"`
	// TaskOnCallRequest opens a CRM task for the mailbox owner when a reply
	// asks for a call or proposes a time.
	TaskOnCallRequest bool `json:"task_on_call_request"`
	// SuppressOnRemovalRequest adds the sender to the suppression list when a
	// reply asks to be removed and the classifier is strongly sure of it.
	SuppressOnRemovalRequest bool `json:"suppress_on_removal_request"`
	// Questions are the workspace's own tagging questions, asked alongside the
	// built-in set, at most 10.
	Questions []InboxTagQuestion `json:"questions"`
	// Languages are the language codes the workspace's mail is written in (for
	// example "en", "de", "ja"); empty uses the default set. An unsupported
	// code fails with code "invalid_setting".
	Languages []string `json:"languages"`
	// ActionRequiredInInbox keeps automated notifications that need the
	// recipient to act (a failed payment, a suspended account) in the inbox,
	// labelled, instead of the Automated view.
	ActionRequiredInInbox bool `json:"action_required_in_inbox"`
}

// Question types of [InboxTagQuestion.Type].
const (
	// InboxTagQuestionYesNo applies Label on yes.
	InboxTagQuestionYesNo = "yes_no"
	// InboxTagQuestionChoice applies the label of the option it picked.
	InboxTagQuestionChoice = "choice"
)

// Actions a matching reply may take, in [InboxTagAction.Type].
const (
	InboxTagActionNone = ""
	InboxTagActionHold = "hold"
	InboxTagActionStop = "stop"
	InboxTagActionTask = "task"
)

// InboxTagQuestion is one workspace-defined tagging question. The server mints
// ID when it is empty; a question needs text (300 characters at most), a label
// of up to 40 characters, and for a choice question 2 to 8 options.
type InboxTagQuestion struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Question string `json:"question"`
	Label    string `json:"label,omitempty"`
	// Action is what a yes answer does.
	Action  InboxTagAction   `json:"action"`
	Choices []InboxTagChoice `json:"choices,omitempty"`
	// Automated also asks the question of automated notifications; a match
	// labels the conversation and keeps it in the inbox, and never acts.
	Automated bool `json:"automated,omitempty"`
}

// InboxTagChoice is one option of a choice question.
type InboxTagChoice struct {
	Label       string         `json:"label"`
	Description string         `json:"description"`
	Action      InboxTagAction `json:"action"`
}

// InboxTagAction is what a matching reply may do: one of the InboxTagAction*
// constants, with HoldDays (1 to 365, default 30) for a hold.
type InboxTagAction struct {
	Type     string `json:"type"`
	HoldDays int    `json:"hold_days,omitempty"`
}

// SendTimeOptimizationSettings holds each campaign email until the clock
// where the recipient reads mail reaches a preferred hour. It only ever delays
// a send, never brings one forward, and is off by default because turning it
// on changes when everything sends.
type SendTimeOptimizationSettings struct {
	Enabled                bool   `json:"enabled"`
	UseContactTimezone     bool   `json:"use_contact_timezone"`
	DefaultContactTimezone string `json:"default_contact_timezone"`
	// PreferredHours are local hours (0-23) to favor.
	PreferredHours          []int   `json:"preferred_hours"`
	WeekendWeightMultiplier float64 `json:"weekend_weight_multiplier"`
}

// PreflightValidationSettings selects the checks run before a campaign starts.
type PreflightValidationSettings struct {
	Enabled                  bool `json:"enabled"`
	CheckTrackingDomain      bool `json:"check_tracking_domain"`
	CheckUnsubscribeHeader   bool `json:"check_unsubscribe_header"`
	CheckABVariantConfigured bool `json:"check_ab_variant_configured"`
	CheckDailyLimit          bool `json:"check_daily_limit"`
	CheckScheduleWindow      bool `json:"check_schedule_window"`
	// CheckContentScore scores each step's copy for spam signals, at preflight
	// and again per send against the rendered text. It is advisory: it warns
	// and never blocks a send. On by default.
	CheckContentScore bool `json:"check_content_score"`
	// MinContentScore is the 1-100 floor below which copy is flagged (default
	// 60). Values outside the range are clamped server-side.
	MinContentScore int `json:"min_content_score"`
}

// DeliverabilityDashboardSettings toggles the panels on the deliverability view.
type DeliverabilityDashboardSettings struct {
	Enabled            bool `json:"enabled"`
	ShowSuppressionLog bool `json:"show_suppression_log"`
	ShowIntentSummary  bool `json:"show_intent_summary"`
	ShowDLQStats       bool `json:"show_dlq_stats"`
}

// OutreachSettings is the full advanced-outreach policy, either for the
// organization or as a per-campaign override.
type OutreachSettings struct {
	BouncePipeline  BouncePipelineSettings  `json:"bounce_pipeline"`
	TaskReliability TaskReliabilitySettings `json:"task_reliability"`
	ABTesting       ABTestingSettings       `json:"ab_testing"`
	ReplyIntent     ReplyIntentSettings     `json:"reply_intent"`
	// InboxTagging is what a classified reply may do: hold, stop, open a task
	// or suppress, plus the workspace's own tagging questions. Because
	// [OutreachService.Update] replaces the settings wholesale, start from Get
	// so this section is carried back unchanged.
	InboxTagging         InboxTaggingSettings            `json:"inbox_tagging"`
	SendTimeOptimization SendTimeOptimizationSettings    `json:"send_time_optimization"`
	Preflight            PreflightValidationSettings     `json:"preflight"`
	Dashboard            DeliverabilityDashboardSettings `json:"dashboard"`
	// Unsubscribe is the in-body opt-out line campaigns inherit unless they
	// set their own [Campaign.UnsubscribeMode].
	Unsubscribe UnsubscribeSettings `json:"unsubscribe"`
	// Custom carries forward-compatible keys the server understands but this
	// SDK release does not model.
	Custom map[string]any `json:"custom,omitempty"`
}

// Get returns the organization's advanced outreach settings.
func (s *OutreachService) Get(ctx context.Context, opts ...RequestOption) (*OutreachSettings, *Response, error) {
	return fetch[OutreachSettings](ctx, s.client, "outreach/settings", opts)
}

// Update replaces the organization's advanced outreach settings wholesale, so
// start from [OutreachService.Get] rather than a zero value. Out-of-range
// values are clamped rather than refused. The API answers 204 with no body;
// read the stored result back with Get.
func (s *OutreachService) Update(ctx context.Context, settings *OutreachSettings, opts ...RequestOption) (*Response, error) {
	body := struct {
		Settings *OutreachSettings `json:"settings"`
	}{Settings: settings}
	return s.client.patch(ctx, "outreach/settings", body, nil, opts...)
}

// DeliverabilityService ingests deliverability events (bounces, complaints,
// deferrals) from an upstream mail pipeline, so a downstream processor such as
// an SES bounce handler can feed Warmbly's suppression list directly.
type DeliverabilityService service

// Deliverability event types accepted by [DeliverabilityEventParams.EventType].
const (
	DeliverabilityEventBounce      = "bounce"
	DeliverabilityEventComplaint   = "complaint"
	DeliverabilityEventUnsubscribe = "unsubscribe"
	DeliverabilityEventOpen        = "open"
	DeliverabilityEventClick       = "click"
	DeliverabilityEventReply       = "reply"
)

// DeliverabilityEventParams describes a single deliverability event.
type DeliverabilityEventParams struct {
	// EventType is one of the DeliverabilityEvent* constants.
	EventType      string `json:"event_type"`
	RecipientEmail string `json:"recipient_email"`
	// CampaignID, TaskID and ContactID attribute the event when known.
	CampaignID string `json:"campaign_id,omitempty"`
	TaskID     string `json:"task_id,omitempty"`
	ContactID  string `json:"contact_id,omitempty"`
	// Provider names the upstream that reported the event, for example "ses".
	Provider string `json:"provider,omitempty"`
	Reason   string `json:"reason,omitempty"`
	// IdempotencyKey deduplicates the event body-side, independently of the
	// Idempotency-Key header.
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// Ingest records a deliverability event for the organization. The API accepts
// the event asynchronously and answers 202 with no body.
func (s *DeliverabilityService) Ingest(ctx context.Context, params *DeliverabilityEventParams, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "deliverability/events", params, nil, opts...)
}
