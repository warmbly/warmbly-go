package gateway

import (
	"encoding/json"
	"time"
)

// EventName is the type key of a dispatched event, carried in the envelope's
// "t" field. The exported constants cover the common events; any other event
// name is delivered to handlers registered with [Client.HandleAny].
type EventName string

// Lifecycle and domain event names.
const (
	// EventReady is dispatched once a freshly identified session is established.
	// Its payload is a [Ready].
	EventReady EventName = "READY"
	// EventResumed is dispatched when a session has been successfully resumed
	// after a reconnect and missed events have been replayed.
	EventResumed EventName = "RESUMED"

	// EventEmailSent is dispatched when a campaign email is handed off for
	// delivery. Payload: [EngagementEvent].
	EventEmailSent EventName = "email.sent"
	// EventEmailOpened is dispatched when a recipient opens an email.
	// Payload: [EngagementEvent].
	EventEmailOpened EventName = "email.opened"
	// EventEmailClicked is dispatched when a recipient clicks a tracked link.
	// Payload: [EngagementEvent] (with URL set).
	EventEmailClicked EventName = "email.clicked"
	// EventEmailReplied is dispatched when a recipient replies.
	// Payload: [EngagementEvent].
	EventEmailReplied EventName = "email.replied"
	// EventEmailBounced is dispatched when an email bounces.
	// Payload: [EngagementEvent].
	EventEmailBounced EventName = "email.bounced"

	// EventCampaignStarted is dispatched when a campaign begins sending.
	// Payload: [CampaignEvent].
	EventCampaignStarted EventName = "campaign.started"
	// EventCampaignPaused is dispatched when a campaign is paused.
	// Payload: [CampaignEvent].
	EventCampaignPaused EventName = "campaign.paused"
	// EventCampaignCompleted is dispatched when a campaign finishes.
	// Payload: [CampaignEvent].
	EventCampaignCompleted EventName = "campaign.completed"

	// EventWarmupHealthChanged is dispatched when a mailbox's warmup health
	// changes. Payload: [WarmupEvent].
	EventWarmupHealthChanged EventName = "warmup.health_changed"
	// EventWarmupBlocked is dispatched when a warming mailbox is blocked.
	// Payload: [WarmupEvent].
	EventWarmupBlocked EventName = "warmup.blocked"

	// EventContactUpdated is dispatched when a contact record changes.
	// Payload: [ContactEvent].
	EventContactUpdated EventName = "contact.updated"

	// EventAutomationRun is dispatched when an automation executes.
	EventAutomationRun EventName = "automation.run"
	// EventCustom is the catch-all name for developer-fired custom events.
	EventCustom EventName = "custom.event"
)

// Event is a dispatched gateway event. Raw holds the event-specific payload;
// decode it into a concrete type with [Event.Into] (or use the typed
// registration helper [On]).
type Event struct {
	// Type is the event name from the envelope's "t" field.
	Type EventName
	// Seq is the sequence number of this event, used for session resumption.
	Seq int
	// Raw is the undecoded event payload (the envelope's "d" field).
	Raw json.RawMessage
}

// Into decodes the event payload into v, which should be a pointer to the
// payload type matching Event.Type.
func (e *Event) Into(v any) error {
	if len(e.Raw) == 0 {
		return nil
	}
	return json.Unmarshal(e.Raw, v)
}

// Ready is the payload of [EventReady]. It identifies the live session.
type Ready struct {
	// SessionID identifies the session and is required to resume it later.
	SessionID string `json:"session_id"`
	// ResumeURL is the URL to use when resuming this session, if the server
	// provides one.
	ResumeURL string `json:"resume_gateway_url,omitempty"`
	// Organization is the organization the session is scoped to.
	Organization struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"organization,omitempty"`
}

// EngagementEvent is the payload of the email engagement events
// ([EventEmailOpened], [EventEmailClicked], [EventEmailReplied], ...).
type EngagementEvent struct {
	EmailAccountID string `json:"email_account_id,omitempty"`
	CampaignID     string `json:"campaign_id,omitempty"`
	ContactID      string `json:"contact_id,omitempty"`
	MessageID      string `json:"message_id,omitempty"`
	// URL is the clicked link, set for [EventEmailClicked].
	URL        string    `json:"url,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

// CampaignEvent is the payload of the campaign lifecycle events.
type CampaignEvent struct {
	CampaignID string    `json:"campaign_id"`
	Status     string    `json:"status,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

// WarmupEvent is the payload of the warmup events.
type WarmupEvent struct {
	EmailAccountID string `json:"email_account_id"`
	// Health is the warmup health state, e.g. "healthy", "warning" or "blocked".
	Health     string    `json:"health,omitempty"`
	Score      float64   `json:"score,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

// ContactEvent is the payload of [EventContactUpdated].
type ContactEvent struct {
	ContactID  string    `json:"contact_id"`
	OccurredAt time.Time `json:"occurred_at"`
}
