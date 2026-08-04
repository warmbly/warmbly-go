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
	// credential exceeded its message rate. Payload: [RateLimited].
	EventRateLimited EventName = "rate_limited"
)

// Mail events. The engagement pulse is the highest-volume family on the
// gateway; subscribe to it deliberately.
const (
	// EventEmailSent fires when a campaign email is handed off for delivery.
	EventEmailSent EventName = "EMAIL_SENT"
	// EventEmailFailed fires when a send fails outright.
	EventEmailFailed EventName = "EMAIL_FAILED"
	// EventEmailOpened fires when a recipient opens a tracked email.
	EventEmailOpened EventName = "EMAIL_OPENED"
	// EventEmailClicked fires when a recipient clicks a tracked link.
	EventEmailClicked EventName = "EMAIL_CLICKED"
	// EventEmailReplied fires when a recipient replies.
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
)

// Contact events. Payload: [ContactEvent].
const (
	EventContactCreated EventName = "CONTACT_CREATED"
	EventContactUpdated EventName = "CONTACT_UPDATED"
	EventContactDeleted EventName = "CONTACT_DELETED"
	// EventContactsReload asks a client to refetch its contact list wholesale,
	// after a bulk change too large to stream row by row.
	EventContactsReload EventName = "CONTACTS_RELOAD"
)

// Mailbox events. Payload: [AccountEvent].
const (
	EventAccountConnected     EventName = "ACCOUNT_CONNECTED"
	EventAccountDisconnected  EventName = "ACCOUNT_DISCONNECTED"
	EventAccountError         EventName = "ACCOUNT_ERROR"
	EventAccountSynced        EventName = "ACCOUNT_SYNCED"
	EventAccountHealthChanged EventName = "ACCOUNT_HEALTH_CHANGED"
)

// Bulk operation events. Payload: [BulkEvent].
const (
	EventBulkStarted   EventName = "BULK_STARTED"
	EventBulkProgress  EventName = "BULK_PROGRESS"
	EventBulkCompleted EventName = "BULK_COMPLETED"
	EventBulkFailed    EventName = "BULK_FAILED"
)

// Send-task events.
const (
	EventTaskCreated   EventName = "TASK_CREATED"
	EventTaskStarted   EventName = "TASK_STARTED"
	EventTaskProgress  EventName = "TASK_PROGRESS"
	EventTaskCompleted EventName = "TASK_COMPLETED"
	EventTaskFailed    EventName = "TASK_FAILED"
)

// Automation events.
const (
	EventAutomationCreated EventName = "AUTOMATION_CREATED"
	EventAutomationUpdated EventName = "AUTOMATION_UPDATED"
	EventAutomationDeleted EventName = "AUTOMATION_DELETED"
	// EventAutomationRun fires each time an automation executes.
	EventAutomationRun EventName = "AUTOMATION_RUN"
)

// Meeting events, from a connected scheduling provider.
const (
	EventMeetingBooked      EventName = "MEETING_BOOKED"
	EventMeetingRescheduled EventName = "MEETING_RESCHEDULED"
	EventMeetingCanceled    EventName = "MEETING_CANCELED"
)

// AI, billing, notification and workspace events.
const (
	// EventAIResearchProgress reports a contact-research batch advancing.
	EventAIResearchProgress EventName = "AI_RESEARCH_PROGRESS"
	// EventAIDraftReady fires when the inbox agent has a reply awaiting review.
	EventAIDraftReady EventName = "AI_DRAFT_READY"

	// EventBillingCreditsLow fires when the AI credit balance crosses the
	// configured alert threshold.
	EventBillingCreditsLow EventName = "BILLING_CREDITS_LOW"
	// EventBillingCreditsChanged fires when the balance moves.
	EventBillingCreditsChanged EventName = "BILLING_CREDITS_CHANGED"

	// EventNotificationCreated fires when a new in-app notification arrives.
	EventNotificationCreated EventName = "NOTIFICATION_CREATED"
	// EventAuditCreated signals that something in the workspace changed, so a
	// dashboard can refetch.
	EventAuditCreated EventName = "AUDIT_CREATED"

	// EventCustom is the developer-fired event published by a fire-event
	// sequence node.
	EventCustom EventName = "CUSTOM_EVENT"
)

// Presence events, pushed on the workspace channel for human sessions. An
// API-key connection receives events but is never tracked as a teammate.
const (
	// EventPresenceState is the full roster, sent right after joining.
	EventPresenceState EventName = "presence_state"
	// EventPresenceDiff carries subsequent joins and leaves.
	EventPresenceDiff EventName = "presence_diff"
)

// Event is a dispatched gateway event. Raw holds the whole event object;
// decode it into a concrete type with [Event.Into], or register a typed handler
// with [On].
type Event struct {
	// Type is the event name.
	Type EventName
	// Seq is the event's per-workspace sequence number, used for resumption.
	// It is 0 for events the server does not sequence, such as presence
	// updates and the client-side lifecycle events.
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
	// UserID is the member the event concerns, when it concerns one.
	UserID string `json:"user_id,omitempty"`
	// OrgID is the workspace the event belongs to.
	OrgID     string    `json:"org_id,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	// Seq is the per-workspace sequence number. It is absent on events the
	// server could not sequence, which are delivered live but not replayable.
	Seq int `json:"seq,omitempty"`
}

// Ready is the payload of [EventReady]: the workspace channel is joined and
// events are flowing.
type Ready struct {
	OrgID string `json:"org_id"`
	// Role is the joining member's workspace role. It is empty for an API-key
	// connection.
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
	// when the credential itself was rejected.
	Reconnecting bool `json:"reconnecting"`
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

// ResumeFailed is the payload of [EventResumeFailed]. Resync from the REST API:
// the gap is no longer replayable.
type ResumeFailed struct {
	// Reason is "buffer_evicted" when the disconnect outlasted the replay
	// buffer, or "invalid_resume" when the client sent a malformed position.
	Reason string `json:"reason"`
	// CurrentSeq is where the stream is now; resume from it going forward.
	CurrentSeq int `json:"current_seq"`
}

// RateLimited is the payload of [EventRateLimited]. Events are being dropped,
// not queued, for the stated window.
type RateLimited struct {
	// Category names the limiter that tripped, for example "ws_message".
	Category string `json:"category"`
	// RetryAfterMS is how long until the limiter resets.
	RetryAfterMS int `json:"retry_after_ms"`
}

// EngagementEvent is the payload of the per-message pulse events.
type EngagementEvent struct {
	BaseEvent
	EmailAccountID string `json:"email_account_id,omitempty"`
	CampaignID     string `json:"campaign_id,omitempty"`
	ContactID      string `json:"contact_id,omitempty"`
	TaskID         string `json:"task_id,omitempty"`
	MessageID      string `json:"message_id,omitempty"`
	// URL is the clicked link, set on [EventEmailClicked].
	URL string `json:"url,omitempty"`
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
}

// CampaignEvent is the payload of the campaign events.
type CampaignEvent struct {
	BaseEvent
	CampaignID string `json:"campaign_id"`
	Name       string `json:"name,omitempty"`
	Status     string `json:"status,omitempty"`
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
	Status         string `json:"status,omitempty"`
	// Health is the mailbox health band, and ErrorMessage the reason it
	// changed, when there is one.
	Health       string `json:"health,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// BulkEvent is the payload of the bulk-operation events.
type BulkEvent struct {
	BaseEvent
	OperationID   string `json:"operation_id"`
	OperationType string `json:"operation_type"`
	EntityType    string `json:"entity_type"`

	TotalItems     int `json:"total_items,omitempty"`
	ProcessedItems int `json:"processed_items,omitempty"`
	FailedItems    int `json:"failed_items,omitempty"`
	// Progress runs from 0 to 1.
	Progress     float64 `json:"progress,omitempty"`
	ErrorMessage string  `json:"error_message,omitempty"`
}

// ResearchProgress is the payload of [EventAIResearchProgress].
type ResearchProgress struct {
	BaseEvent
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}
