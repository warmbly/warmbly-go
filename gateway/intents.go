package gateway

// An intent narrows the workspace stream to the event families you care about.
// Declare them with [WithIntents]; declaring none delivers everything the
// credential is permitted to see.
//
// Matching is a plain substring test. The server upper-cases each token and the
// event type and collapses ".", ":", "-" and whitespace to "_" before
// comparing, so case and separators do not matter, but nothing else is
// normalized. [IntentCampaign] therefore covers CAMPAIGN_STARTED,
// CAMPAIGN_PROGRESS and every other CAMPAIGN_* event, and an intent can be as
// narrow as you like: "CAMPAIGN_STARTED" is a valid intent, and so is any other
// fragment of an event type.
//
// The flip side of a substring test is that a short token matches more than it
// looks like it should — "AI" is a substring of both EMAIL_SENT and
// CAMPAIGN_PAUSED. The constants below are chosen to avoid that; prefer them to
// hand-written tokens, and include the trailing underscore if you write your
// own short one.
//
// Intents only ever subtract. They cannot widen what a credential may see: a
// key without unibox access receives no inbox events however it asks. They also
// apply to the workspace channel only — the extra topics from [WithTopics] are
// already scoped to a single resource, and the server ignores intents on them.
const (
	// IntentCampaign covers campaign lifecycle and progress, including
	// [EventCampaignIdle].
	IntentCampaign = "CAMPAIGN"
	// IntentEmail covers the per-message pulse — sent, opened, clicked,
	// replied, bounced — and inbox arrivals. It is the highest-volume family.
	IntentEmail = "EMAIL"
	// IntentContact covers contact creation, updates and deletion.
	IntentContact = "CONTACT"
	// IntentAccount covers mailbox connection, sync and health transitions.
	IntentAccount = "ACCOUNT"
	// IntentWarmup covers warmup health and placement changes.
	IntentWarmup = "WARMUP"
	// IntentBulk covers bulk import and export progress. Those events are
	// user-scoped and never reach the workspace channel, so this only narrows
	// a [UserTopic] stream if you also declare it there — which the server
	// does not read. Kept for completeness.
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
	// IntentAI covers AI research progress and inbox-agent drafts. The
	// trailing underscore is load-bearing: a bare "AI" is a substring of
	// EMAIL and CAMPAIGN and would let almost the whole stream through.
	IntentAI = "AI_"
	// IntentPage covers website-tracking page views ([EventPageHit]).
	IntentPage = "PAGE"
	// IntentForm covers hosted form submissions
	// ([EventFormSubmissionCreated]).
	IntentForm = "FORM"
	// IntentCustom covers developer-fired custom events.
	IntentCustom = "CUSTOM"
)
