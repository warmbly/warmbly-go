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
	Enabled                 bool     `json:"enabled"`
	PositiveKeywords        []string `json:"positive_keywords"`
	NegativeKeywords        []string `json:"negative_keywords"`
	OutOfOfficeKeywords     []string `json:"out_of_office_keywords"`
	QuestionKeywords        []string `json:"question_keywords"`
	AutoCreateCRMTask       bool     `json:"auto_create_crm_task"`
	AutoPauseOnNegative     bool     `json:"auto_pause_on_negative"`
	AutoSuppressOnUnsubWord bool     `json:"auto_suppress_on_unsubscribe_keyword"`
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
	BouncePipeline       BouncePipelineSettings          `json:"bounce_pipeline"`
	TaskReliability      TaskReliabilitySettings         `json:"task_reliability"`
	ABTesting            ABTestingSettings               `json:"ab_testing"`
	ReplyIntent          ReplyIntentSettings             `json:"reply_intent"`
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
