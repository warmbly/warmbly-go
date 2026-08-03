package warmbly

import "context"

// OutreachService reads and writes the organization-wide advanced outreach
// settings: the bounce pipeline, reply-intent classification, send-time
// optimization, preflight checks and the rest of the sending policy that
// campaigns inherit.
//
// A campaign can override any of these for itself; see
// [CampaignService.AdvancedSettings].
type OutreachService service

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

// SendTimeOptimizationSettings shifts sends towards the hours a recipient is
// most likely to engage.
type SendTimeOptimizationSettings struct {
	Enabled                bool   `json:"enabled"`
	UseContactTimezone     bool   `json:"use_contact_timezone"`
	DefaultContactTimezone string `json:"default_contact_timezone"`
	// PreferredHours are local hours (0-23) to favour.
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
	// Custom carries forward-compatible keys the server understands but this
	// SDK release does not model.
	Custom map[string]any `json:"custom,omitempty"`
}

// Get returns the organization's advanced outreach settings.
func (s *OutreachService) Get(ctx context.Context, opts ...RequestOption) (*OutreachSettings, *Response, error) {
	return fetch[OutreachSettings](ctx, s.client, "outreach/settings", opts)
}

// Update replaces the organization's advanced outreach settings.
func (s *OutreachService) Update(ctx context.Context, settings *OutreachSettings, opts ...RequestOption) (*OutreachSettings, *Response, error) {
	body := struct {
		Settings *OutreachSettings `json:"settings"`
	}{Settings: settings}
	return send[OutreachSettings](ctx, s.client, s.client.patch, "outreach/settings", body, opts)
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
