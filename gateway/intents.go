package gateway

// An intent narrows the workspace stream to the event families you care about.
// Declare them with [WithIntents]; declaring none delivers everything the
// credential is permitted to see.
//
// Matching is a substring test against the event type, so [IntentCampaign]
// covers CAMPAIGN_STARTED, CAMPAIGN_PROGRESS and every other CAMPAIGN_* event.
// That also means an intent can be as narrow as you like: "CAMPAIGN_STARTED" is
// a valid intent, and so is any other fragment of an event type.
//
// Intents only ever subtract. They cannot widen what a credential may see: a
// key without unibox access receives no inbox events however it asks.
const (
	// IntentCampaign covers campaign lifecycle and progress.
	IntentCampaign = "CAMPAIGN"
	// IntentEmail covers the per-message pulse — sent, opened, clicked,
	// replied, bounced — and inbox arrivals. It is the highest-volume family.
	IntentEmail = "EMAIL"
	// IntentContact covers contact creation, updates and deletion.
	IntentContact = "CONTACT"
	// IntentAccount covers mailbox connection and health transitions.
	IntentAccount = "ACCOUNT"
	// IntentWarmup covers warmup health and placement changes.
	IntentWarmup = "WARMUP"
	// IntentBulk covers bulk import and export progress.
	IntentBulk = "BULK"
	// IntentTask covers send-task progress.
	IntentTask = "TASK"
	// IntentAudit covers the audit-trail signal that something changed.
	IntentAudit = "AUDIT"
	// IntentAutomation covers automation lifecycle and runs.
	IntentAutomation = "AUTOMATION"
	// IntentMeeting covers meetings booked through a connected scheduler.
	IntentMeeting = "MEETING"
	// IntentBilling covers subscription and credit-balance changes.
	IntentBilling = "BILLING"
	// IntentNotification covers in-app notifications.
	IntentNotification = "NOTIFICATION"
	// IntentAI covers AI research progress and inbox-agent drafts.
	IntentAI = "AI"
	// IntentCustom covers developer-fired custom events.
	IntentCustom = "CUSTOM"
)
