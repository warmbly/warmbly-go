package gateway

import (
	"encoding/json"
	"time"
)

// EventName is the type of a dispatched event. The constants below cover the
// events the platform emits today; any other name still reaches handlers
// registered with [Client.HandleAny].
type EventName = string

// Client-side lifecycle events. These are synthesized by this package rather
// than sent by the server, so a handler can react to the connection itself.
const (
	// EventReady fires once the workspace channel is joined and events are
	// flowing. Payload: [Ready].
	EventReady EventName = "__ready"
	// EventDisconnected fires when the connection drops, before the client
	// reconnects. Payload: [Disconnected].
	EventDisconnected EventName = "__disconnected"
	// EventJoinFailed fires when an extra topic declared with [WithTopics] is
	// refused for good (any code other than rate limiting). The session stays
	// up on the workspace channel; the refused topic is not retried until the
	// next reconnect. Payload: [JoinFailed].
	EventJoinFailed EventName = "__join_failed"
)

// Server lifecycle events, pushed on the workspace channel.
const (
	// EventResumed fires after a reconnect once the missed events have been
	// replayed. Payload: [Resumed].
	EventResumed EventName = "resumed"
	// EventResumeFailed fires when the gap could not be replayed, because the
	// disconnect outlasted the server's buffer. Resync from the REST API.
	// Payload: [ResumeFailed].
	EventResumeFailed EventName = "resume_failed"
	// EventRateLimited fires when outbound events are being dropped because the
	// credential exceeded its message rate, and also (synthesized by the
	// client, with Category [RateLimitCategoryJoin]) when a channel join was
	// refused for rate limiting and is being retried. Payload: [RateLimited].
	EventRateLimited EventName = "rate_limited"
)

// Mail events. The engagement pulse is the highest-volume family on the
// gateway; subscribe to it deliberately.
const (
	// EventEmailSent fires when a campaign email is handed off for delivery.
	// Payload: [TaskProgressEvent], which also decodes into [EngagementEvent].
	EventEmailSent EventName = "EMAIL_SENT"
	// EventEmailFailed fires on the sender's user channel when a send fails
	// outright. Payload: [StatusEvent].
	EventEmailFailed EventName = "EMAIL_FAILED"
	// EventEmailOpened fires when a recipient opens a tracked email. Check
	// [EngagementEvent.Machine] before counting it as a person.
	EventEmailOpened EventName = "EMAIL_OPENED"
	// EventEmailClicked fires when a recipient clicks a tracked link. Check
	// [EngagementEvent.Machine] before counting it as a person.
	EventEmailClicked EventName = "EMAIL_CLICKED"
	// EventEmailReplied fires when a human reply lands for a campaign contact.
	EventEmailReplied EventName = "EMAIL_REPLIED"

	// EventEmailReceived fires when a message lands in a connected mailbox.
	// Payload: [InboxEvent].
	EventEmailReceived EventName = "EMAIL_RECEIVED"
	// EventEmailUpdated fires when a synced message changes, for example when
	// it is marked read. Payload: [InboxEvent].
	EventEmailUpdated EventName = "EMAIL_UPDATED"
	// EventEmailDeleted fires when a synced message is removed.
	// Payload: [InboxEvent].
	EventEmailDeleted EventName = "EMAIL_DELETED"
)

// Campaign events. Payload: [CampaignEvent].
const (
	EventCampaignCreated   EventName = "CAMPAIGN_CREATED"
	EventCampaignUpdated   EventName = "CAMPAIGN_UPDATED"
	EventCampaignDeleted   EventName = "CAMPAIGN_DELETED"
	EventCampaignStarted   EventName = "CAMPAIGN_STARTED"
	EventCampaignPaused    EventName = "CAMPAIGN_PAUSED"
	EventCampaignCompleted EventName = "CAMPAIGN_COMPLETED"
	// EventCampaignIdle fires when a continuous campaign runs out of leads. The
	// campaign is not finished: it stays active with [CampaignEvent.Status]
	// "active" and resumes on its own when new leads arrive. The idle mark is
	// on the campaign resource ("idle_since"), so refetch it if you need the
	// timestamp.
	EventCampaignIdle EventName = "CAMPAIGN_IDLE"
)

// Contact events. Payload: [ContactEvent].
const (
	EventContactCreated EventName = "CONTACT_CREATED"
	EventContactUpdated EventName = "CONTACT_UPDATED"
	EventContactDeleted EventName = "CONTACT_DELETED"
	// EventContactsReload asks a client to refetch its contact list wholesale,
	// after a bulk change too large to stream row by row. It is pushed on the
	// user channel of the member who ran the operation and decodes as a
	// [BulkEvent] carrying the operation id.
	EventContactsReload EventName = "CONTACTS_RELOAD"
)

// Mailbox events. Payload: [AccountEvent].
const (
	EventAccountConnected     EventName = "ACCOUNT_CONNECTED"
	EventAccountDisconnected  EventName = "ACCOUNT_DISCONNECTED"
	EventAccountError         EventName = "ACCOUNT_ERROR"
	EventAccountSynced        EventName = "ACCOUNT_SYNCED"
	EventAccountHealthChanged EventName = "ACCOUNT_HEALTH_CHANGED"
	// EventAccountSyncState fires when a mailbox's initial import finishes, or
	// when the fair-use throttle starts or stops holding it.
	// [AccountEvent.Status] is the backfill status (one of the SyncBackfill*
	// constants) and [AccountEvent.Reason] the throttle reason (a SyncThrottle*
	// constant), empty once the throttle is released.
	EventAccountSyncState EventName = "ACCOUNT_SYNC_STATE"
)

// Backfill statuses carried in [AccountEvent.Status] on [EventAccountSyncState].
const (
	SyncBackfillPending  = "pending"
	SyncBackfillRunning  = "running"
	SyncBackfillComplete = "complete"
)

// Throttle reasons carried in [AccountEvent.Reason] on [EventAccountSyncState]
// while the fair-use throttle holds a mailbox. Each names the exhausted budget.
const (
	SyncThrottleBurst    = "burst"
	SyncThrottleHourly   = "hourly"
	SyncThrottleDaily    = "daily"
	SyncThrottleOrgDaily = "org_daily"
	// SyncThrottlePriorityDaily is the per-day budget for prioritized syncs.
	SyncThrottlePriorityDaily = "priority_daily"
)

// Bulk operation events. Payload: [BulkEvent].
//
// These events carry no workspace id, so the org channel never sees them. They
// are delivered on the user channel of the member who started the operation,
// and on the operation's own bulk:<operation_id> topic ([BulkTopic]). The bulk
// topic is joinable by any member but pushes an event only to the socket of
// the user who started the operation; joining someone else's operation yields
// an open channel that never receives anything.
const (
	EventBulkStarted   EventName = "BULK_STARTED"
	EventBulkProgress  EventName = "BULK_PROGRESS"
	EventBulkCompleted EventName = "BULK_COMPLETED"
	EventBulkFailed    EventName = "BULK_FAILED"
)

// Send-task events.
const (
	// EventTaskProgress reports a campaign send task advancing, on the org and
	// campaign channels. Payload: [TaskProgressEvent].
	EventTaskProgress EventName = "TASK_PROGRESS"

	// The coarse task lifecycle is pushed on the owning member's user channel
	// only. Payload: [StatusEvent].
	EventTaskCreated   EventName = "TASK_CREATED"
	EventTaskStarted   EventName = "TASK_STARTED"
	EventTaskCompleted EventName = "TASK_COMPLETED"
	EventTaskFailed    EventName = "TASK_FAILED"

	// EventError and EventWarning are mailbox-level faults surfaced to the
	// owning member on their user channel, for example a provider refusing a
	// send. Payload: [StatusEvent], with the title and message in Data.
	EventError   EventName = "ERROR"
	EventWarning EventName = "WARNING"
)

// Automation events. Payload: [AutomationEvent].
const (
	EventAutomationCreated EventName = "AUTOMATION_CREATED"
	EventAutomationUpdated EventName = "AUTOMATION_UPDATED"
	EventAutomationDeleted EventName = "AUTOMATION_DELETED"
	// EventAutomationRun fires each time an automation executes.
	EventAutomationRun EventName = "AUTOMATION_RUN"
)

// Meeting events, from a connected scheduling provider. Payload:
// [MeetingEvent]. They are pushed on the lead owner's user channel, not the
// org channel, so join [UserTopic] to receive them.
const (
	EventMeetingBooked      EventName = "MEETING_BOOKED"
	EventMeetingRescheduled EventName = "MEETING_RESCHEDULED"
	EventMeetingCanceled    EventName = "MEETING_CANCELED"
)

// AI, billing, notification and workspace events.
const (
	// EventAIResearchProgress fires as each run in a contact-research batch
	// completes. Payload: [ResearchProgress].
	EventAIResearchProgress EventName = "AI_RESEARCH_PROGRESS"
	// EventAIDraftReady fires when the inbox agent has a reply awaiting
	// review. It requires unibox access. Payload: [DraftReadyEvent].
	EventAIDraftReady EventName = "AI_DRAFT_READY"

	// EventBillingCreditsLow fires when the AI credit balance crosses the
	// configured alert threshold, at most once a day. Payload: [BillingEvent].
	EventBillingCreditsLow EventName = "BILLING_CREDITS_LOW"
	// EventBillingCreditsChanged fires after every credit debit.
	// Payload: [BillingEvent].
	EventBillingCreditsChanged EventName = "BILLING_CREDITS_CHANGED"

	// EventNotificationCreated fires when a new in-app notification arrives.
	// It is user-scoped: join [UserTopic] to receive it.
	// Payload: [NotificationEvent].
	EventNotificationCreated EventName = "NOTIFICATION_CREATED"
	// EventAuditCreated signals that something in the workspace changed, so a
	// dashboard can refetch. Payload: [AuditEvent].
	EventAuditCreated EventName = "AUDIT_CREATED"

	// EventPageHit fires when an identified contact views a page on a site
	// with website tracking installed. Payload: [PageHitEvent].
	EventPageHit EventName = "PAGE_HIT"
	// EventFormSubmissionCreated fires when a hosted form receives a
	// submission. The payload carries ids only; the answers stay behind the
	// forms REST endpoint and its permission. Payload: [FormSubmissionEvent].
	EventFormSubmissionCreated EventName = "FORM_SUBMISSION_CREATED"

	// EventCustom is the developer-fired event published by a fire-event
	// sequence node or automation action. Payload: [CustomEvent].
	EventCustom EventName = "CUSTOM_EVENT"
)

// Presence events, pushed on the workspace channel for human sessions. An
// API-key connection receives events but is never tracked as a teammate.
const (
	// EventPresenceState is the full roster, sent right after joining.
	// Payload: [PresenceState].
	EventPresenceState EventName = "presence_state"
	// EventPresenceDiff carries subsequent joins and leaves.
	// Payload: [PresenceDiff].
	EventPresenceDiff EventName = "presence_diff"
)

// Event is a dispatched gateway event. Raw holds the whole event object;
// decode it into a concrete type with [Event.Into], or register a typed handler
// with [On].
type Event struct {
	// Type is the event name.
	Type EventName
	// Topic is the channel the event arrived on: the workspace channel, or one
	// of the extra topics declared with [WithTopics]. It is empty for the
	// client-side lifecycle events.
	Topic string
	// Seq is the event's per-workspace sequence number, used for resumption.
	// It is 0 for events the server does not sequence, such as presence
	// updates, anything arriving on a non-workspace topic and the client-side
	// lifecycle events.
	Seq int
	// Raw is the undecoded event payload.
	Raw json.RawMessage
}

// Into decodes the event payload into v, which should be a pointer to the type
// matching [Event.Type].
func (e *Event) Into(v any) error {
	if len(e.Raw) == 0 {
		return nil
	}
	return json.Unmarshal(e.Raw, v)
}

// BaseEvent is the envelope every server event shares.
type BaseEvent struct {
	// EventType is the event name, repeated inside the payload.
	EventType string `json:"event_type"`
	// UserID is the member the event concerns, when it concerns one. It is
	// empty (or the nil UUID) for events fired by the system rather than a
	// member: form submissions, page hits, credit alerts.
	UserID string `json:"user_id,omitempty"`
	// OrgID is the workspace the event belongs to.
	OrgID string `json:"org_id,omitempty"`
	// Timestamp is when the event was published.
	Timestamp time.Time `json:"timestamp"`
	// Seq is the per-workspace sequence number, stamped by the gateway on
	// events delivered over the workspace channel. It is absent on events the
	// server does not sequence, which are delivered live but not replayable.
	Seq int `json:"seq,omitempty"`
}

// Ready is the payload of [EventReady]: the workspace channel is joined and
// events are flowing.
type Ready struct {
	OrgID string `json:"org_id"`
	// Role is the workspace role of the member the credential belongs to — for
	// an API key, the role of the member who created it, which is what bounds
	// the events the connection receives.
	Role string `json:"role,omitempty"`
	// HeartbeatIntervalMS is how often the client must heartbeat, and
	// ServerTimeoutMS how long the server waits before closing a silent
	// connection. The client honors both automatically.
	HeartbeatIntervalMS int `json:"heartbeat_interval_ms"`
	ServerTimeoutMS     int `json:"server_timeout_ms"`
	// Seq is the workspace's current sequence number at join time.
	Seq int `json:"seq"`
	// ResumeSupported reports whether the server will replay a gap.
	ResumeSupported bool `json:"resume_supported"`
}

// Disconnected is the payload of [EventDisconnected].
type Disconnected struct {
	// Err is why the connection dropped.
	Err string `json:"error,omitempty"`
	// Reconnecting is false only when the client has given up, which happens
	// when the credential itself was rejected or the workspace join was
	// refused for good.
	Reconnecting bool `json:"reconnecting"`
}

// JoinFailed is the payload of [EventJoinFailed]: an extra topic from
// [WithTopics] was refused.
type JoinFailed struct {
	Topic string `json:"topic"`
	// Code and Reason are the refusal as the server sent it; see [JoinError].
	Code   int    `json:"code"`
	Reason string `json:"reason"`
}

// Resumed is the payload of [EventResumed].
type Resumed struct {
	// From is the sequence the client resumed at, and CurrentSeq the
	// workspace's sequence now.
	From       int `json:"from"`
	CurrentSeq int `json:"current_seq"`
	// Replayed is how many missed events were delivered.
	Replayed int `json:"replayed"`
}

// Reasons carried in [ResumeFailed.Reason].
const (
	// ResumeReasonBufferEvicted means the disconnect outlasted the replay
	// buffer, which holds roughly the last 2,000 events per workspace for up
	// to an hour.
	ResumeReasonBufferEvicted = "buffer_evicted"
	// ResumeReasonInvalid means the client sent a malformed position.
	ResumeReasonInvalid = "invalid_resume"
)

// ResumeFailed is the payload of [EventResumeFailed]. Resync from the REST API:
// the gap is no longer replayable.
type ResumeFailed struct {
	// Reason is [ResumeReasonBufferEvicted] or [ResumeReasonInvalid].
	Reason string `json:"reason"`
	// CurrentSeq is where the stream is now; resume from it going forward.
	CurrentSeq int `json:"current_seq"`
}

// Rate-limiter categories carried in [RateLimited.Category]. Each is a
// separate per-minute budget, allowing a burst of 1.5x its figure.
const (
	// RateLimitCategoryMessage is server-to-client delivery (120/minute by
	// default). Events over budget are dropped, not queued.
	RateLimitCategoryMessage = "ws_message"
	// RateLimitCategoryJoin is channel joins (30/minute by default). The
	// client waits out the hint and rejoins on the same socket.
	RateLimitCategoryJoin = "ws_join"
	// RateLimitCategoryEvent is client-sent events (60/minute by default).
	RateLimitCategoryEvent = "ws_event"
	// RateLimitCategoryConnect is socket handshakes (30/minute by default),
	// budgeted separately from joins so a reconnect does not spend the
	// allowance needed to rejoin.
	RateLimitCategoryConnect = "ws_connect"
)

// RateLimited is the payload of [EventRateLimited]. For
// [RateLimitCategoryMessage] events are being dropped, not queued, for the
// stated window. For [RateLimitCategoryJoin] the client is waiting out the
// window before it sends the join again.
type RateLimited struct {
	// Category names the limiter that tripped, one of the RateLimitCategory*
	// constants.
	Category string `json:"category"`
	// RetryAfterMS is how long until the limiter resets: the remainder of the
	// current minute's window, so retrying earlier only spends another refusal.
	RetryAfterMS int `json:"retry_after_ms"`
	// Topic is the channel whose join was refused. It is set only on the
	// client-synthesized join variant.
	Topic string `json:"topic,omitempty"`
}

// EngagementEvent is the payload of the per-message pulse events
// ([EventEmailOpened], [EventEmailClicked], [EventEmailReplied]). It also
// decodes the fields [EventEmailSent] shares; see [TaskProgressEvent] for the
// rest of that payload.
type EngagementEvent struct {
	BaseEvent
	CampaignID string `json:"campaign_id,omitempty"`
	ContactID  string `json:"contact_id,omitempty"`
	// ContactEmail is the recipient's address.
	ContactEmail string `json:"contact_email,omitempty"`
	// StepID is the sequence step the message belongs to.
	StepID string `json:"step_id,omitempty"`
	// TaskID is the send task, set on [EventEmailSent].
	TaskID string `json:"task_id,omitempty"`

	// URL is the clicked link's original destination, set on
	// [EventEmailClicked]. On the wire it is "original_url".
	URL string `json:"original_url,omitempty"`
	// LinkLabel is the anchor text of the clicked link.
	LinkLabel string `json:"link_label,omitempty"`

	// Machine marks an automated open or click rather than a person's: a mail
	// client prefetch (Apple Mail Privacy Protection), a fetch with no user
	// agent, or a security gateway walking the links. Live views should badge
	// these instead of counting them as engagement.
	Machine bool `json:"machine,omitempty"`
	// OccurredAt is when the tracking service saw the open or click.
	// [BaseEvent.Timestamp] is when the event was published, which can be
	// later.
	OccurredAt time.Time `json:"occurred_at,omitempty"`

	// Client, DeviceType, CountryCode and City describe where and on what the
	// engagement happened, when the consumer could tell. Any of them may be
	// empty.
	Client      string `json:"client,omitempty"`
	DeviceType  string `json:"device_type,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
	City        string `json:"city,omitempty"`

	// EmailAccountID and MessageID are reserved for the sending mailbox and
	// the provider message id. The current server does not populate them on
	// pulse events; use [InboxEvent] for mailbox-level identifiers.
	EmailAccountID string `json:"email_account_id,omitempty"`
	MessageID      string `json:"message_id,omitempty"`
}

// TaskProgressEvent is the payload of [EventTaskProgress] and
// [EventEmailSent]: which contact and step a campaign just fired, and how far
// along the campaign is, so a dashboard can update without a refetch.
type TaskProgressEvent struct {
	BaseEvent
	CampaignID string `json:"campaign_id"`
	TaskID     string `json:"task_id"`
	// Status is the task state: "pending", "active", "completed" or "failed".
	Status       string `json:"status,omitempty"`
	ContactID    string `json:"contact_id,omitempty"`
	ContactEmail string `json:"contact_email,omitempty"`
	ContactName  string `json:"contact_name,omitempty"`
	// StepID, StepName and StepIndex identify the sequence step.
	StepID    string `json:"step_id,omitempty"`
	StepName  string `json:"step_name,omitempty"`
	StepIndex int    `json:"step_index,omitempty"`
	// Progress is a percentage from 0 to 100.
	Progress       int `json:"progress,omitempty"`
	TotalContacts  int `json:"total_contacts,omitempty"`
	ProcessedCount int `json:"processed_count,omitempty"`
}

// StatusEvent is the payload of the user-channel task lifecycle events
// ([EventTaskCreated] through [EventTaskFailed]), [EventEmailFailed],
// [EventError] and [EventWarning].
type StatusEvent struct {
	BaseEvent
	TaskID string `json:"task_id,omitempty"`
	// EmailID is the mailbox concerned.
	EmailID string `json:"email_id,omitempty"`
	// Message is a short human-readable summary.
	Message string `json:"message,omitempty"`
	// Data is event-specific detail. For [EventError] and [EventWarning] it is
	// an object with "title" and "message".
	Data json.RawMessage `json:"data,omitempty"`
}

// InboxEvent is the payload of the mailbox-sync events.
type InboxEvent struct {
	BaseEvent
	EmailAccountID string `json:"email_account_id"`
	MessageID      string `json:"message_id"`
	ThreadID       string `json:"thread_id,omitempty"`
	Subject        string `json:"subject,omitempty"`
	From           string `json:"from,omitempty"`
	// Preview is a short snippet of the body.
	Preview string `json:"preview,omitempty"`
	// Folder is where the message sits: "inbox", "sent", "drafts", "archive",
	// "spam" or "trash".
	Folder string `json:"folder,omitempty"`
}

// CampaignEvent is the payload of the campaign events.
type CampaignEvent struct {
	BaseEvent
	CampaignID string `json:"campaign_id"`
	Name       string `json:"name,omitempty"`
	// Status is the campaign's state after the event. On [EventCampaignIdle]
	// it is still "active".
	Status string `json:"status,omitempty"`
	// Progress is set on progress events.
	Progress *CampaignProgress `json:"progress,omitempty"`
}

// CampaignProgress is a campaign's running totals.
type CampaignProgress struct {
	TotalContacts int `json:"total_contacts"`
	EmailsSent    int `json:"emails_sent"`
	EmailsOpened  int `json:"emails_opened"`
	EmailsClicked int `json:"emails_clicked"`
	EmailsReplied int `json:"emails_replied"`
	EmailsBounced int `json:"emails_bounced"`
}

// ContactEvent is the payload of the contact events.
type ContactEvent struct {
	BaseEvent
	ContactID string `json:"contact_id"`
	Email     string `json:"email,omitempty"`
	ListID    string `json:"list_id,omitempty"`
}

// AccountEvent is the payload of the mailbox events.
type AccountEvent struct {
	BaseEvent
	EmailAccountID string `json:"email_account_id"`
	Email          string `json:"email,omitempty"`
	// Provider is the mailbox provider, for example "gmail", "outlook" or
	// "smtp_imap".
	Provider string `json:"provider,omitempty"`
	// Status is the mailbox state after the event. On
	// [EventAccountHealthChanged] it repeats the new health state; on
	// [EventAccountSyncState] it is the backfill status.
	Status string `json:"status,omitempty"`
	// ErrorCode and ErrorMessage describe the fault on [EventAccountError].
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	// Health is the new warmup health state on [EventAccountHealthChanged],
	// and PreviousState the one it left. On the wire Health is "health_state".
	Health        string `json:"health_state,omitempty"`
	PreviousState string `json:"previous_state,omitempty"`
	// Reason is why the state changed: the health-transition reason on
	// [EventAccountHealthChanged], or the throttle reason on
	// [EventAccountSyncState] (empty once the throttle is released).
	Reason string `json:"reason,omitempty"`
}

// BulkEvent is the payload of the bulk-operation events and of
// [EventContactsReload].
type BulkEvent struct {
	BaseEvent
	OperationID   string `json:"operation_id"`
	OperationType string `json:"operation_type,omitempty"`
	EntityType    string `json:"entity_type,omitempty"`

	TotalItems     int `json:"total_items,omitempty"`
	ProcessedItems int `json:"processed_items,omitempty"`
	FailedItems    int `json:"failed_items,omitempty"`
	// Progress runs from 0 to 1.
	Progress     float64 `json:"progress,omitempty"`
	ErrorMessage string  `json:"error_message,omitempty"`
}

// AutomationEvent is the payload of the automation events.
type AutomationEvent struct {
	BaseEvent
	AutomationID   string `json:"automation_id,omitempty"`
	AutomationName string `json:"automation_name,omitempty"`
	// Status is set on [EventAutomationRun].
	Status string `json:"status,omitempty"`
}

// MeetingEvent is the payload of the meeting events.
type MeetingEvent struct {
	BaseEvent
	BookingID    string `json:"booking_id"`
	ContactID    string `json:"contact_id,omitempty"`
	InviteeEmail string `json:"invitee_email,omitempty"`
	EventName    string `json:"event_name,omitempty"`
	// ScheduledFor is the meeting time as the provider formatted it.
	ScheduledFor string `json:"scheduled_for,omitempty"`
	// Source is the scheduling provider: "calendly" or "cal_com".
	Source string `json:"source,omitempty"`
	// State is "booked", "rescheduled" or "canceled".
	State string `json:"state,omitempty"`
}

// ResearchProgress is the payload of [EventAIResearchProgress]: one contact
// research run completed. A batch produces one event per run, so count them
// client-side to show a batch advancing.
type ResearchProgress struct {
	BaseEvent
	ContactID string `json:"contact_id,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	// Status is the run's outcome, for example "completed" or "failed".
	Status string `json:"status,omitempty"`
}

// DraftReadyEvent is the payload of [EventAIDraftReady].
type DraftReadyEvent struct {
	BaseEvent
	ThreadID string `json:"thread_id,omitempty"`
	DraftID  string `json:"draft_id,omitempty"`
	// EmailID is the inbound message the draft answers.
	EmailID string `json:"email_id,omitempty"`
}

// BillingEvent is the payload of [EventBillingCreditsLow] and
// [EventBillingCreditsChanged].
type BillingEvent struct {
	BaseEvent
	// Balance is the spendable AI credit balance after the change.
	Balance int `json:"balance"`
	// Threshold is the workspace's alert threshold, set on
	// [EventBillingCreditsLow].
	Threshold int `json:"threshold,omitempty"`
}

// NotificationEvent is the payload of [EventNotificationCreated]. The feed
// table is the source of truth; this is the signal to refresh it.
type NotificationEvent struct {
	BaseEvent
	NotificationID string `json:"notification_id"`
	Category       string `json:"category"`
	Title          string `json:"title"`
	// Link is where the notification points, when it points somewhere.
	Link string `json:"link,omitempty"`
}

// AuditEvent is the payload of [EventAuditCreated]. It carries no sensitive
// detail, only enough to know what kind of thing changed.
type AuditEvent struct {
	BaseEvent
	// Action is what happened, for example "update", and EntityType what it
	// happened to, for example "campaign".
	Action     string `json:"action"`
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id,omitempty"`
}

// PageHitEvent is the payload of [EventPageHit].
type PageHitEvent struct {
	BaseEvent
	ContactID string `json:"contact_id"`
	URL       string `json:"url"`
	Title     string `json:"title,omitempty"`
}

// FormSubmissionEvent is the payload of [EventFormSubmissionCreated]. Fetch the
// submission over the REST API for its answers.
type FormSubmissionEvent struct {
	BaseEvent
	FormID       string `json:"form_id"`
	SubmissionID string `json:"submission_id,omitempty"`
	// ContactID is the contact the submission created or matched, when it did.
	ContactID string `json:"contact_id,omitempty"`
}

// CustomEvent is the payload of [EventCustom]. Match on Name: it is the event
// name chosen in the fire-event step.
type CustomEvent struct {
	BaseEvent
	Name string `json:"name"`
	// Payload is the step's key/value fields, each templated against the
	// contact and event data when the step ran.
	Payload map[string]string `json:"payload,omitempty"`
	// Source is "automation" or "campaign", and SourceID the id of the one
	// that fired the event.
	Source   string `json:"source,omitempty"`
	SourceID string `json:"source_id,omitempty"`
}

// PresenceMeta is one session of a member on the workspace channel.
type PresenceMeta struct {
	// OnlineAt is a Unix timestamp in seconds.
	OnlineAt int64  `json:"online_at"`
	Name     string `json:"name,omitempty"`
	Avatar   string `json:"avatar,omitempty"`
	// Page, Resource and Action are the member's live activity. They are
	// empty when the workspace hides activity, or before the member's first
	// presence update.
	Page     string `json:"page,omitempty"`
	Resource string `json:"resource,omitempty"`
	Action   string `json:"action,omitempty"`
	// UpdatedAt is a Unix timestamp in seconds of the last activity update.
	UpdatedAt int64 `json:"updated_at,omitempty"`
	// PhxRef is Phoenix's identifier for this session.
	PhxRef string `json:"phx_ref,omitempty"`
}

// PresenceEntry is every session of one member.
type PresenceEntry struct {
	Metas []PresenceMeta `json:"metas"`
}

// PresenceState is the payload of [EventPresenceState]: the roster keyed by
// member id. It is empty when the workspace has turned off "show who's
// online".
type PresenceState map[string]PresenceEntry

// PresenceDiff is the payload of [EventPresenceDiff].
type PresenceDiff struct {
	Joins  PresenceState `json:"joins"`
	Leaves PresenceState `json:"leaves"`
}
