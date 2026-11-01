package gateway

// An intent narrows the workspace stream to the event families you care about.
// Declare them with [WithIntents]; declaring none delivers everything the
// credential is permitted to see.
//
// Matching is a plain substring test, and the SDK passes your tokens through
// untouched. The server upper-cases each token and the event type and collapses
// ".", ":", "-" and whitespace to "_" before comparing, so case and separators
// do not matter, but nothing else is normalized. [IntentCampaign] therefore
// covers CAMPAIGN_STARTED, CAMPAIGN_PROGRESS and every other CAMPAIGN_* event,
// and an intent can be as narrow as you like: "CAMPAIGN_STARTED" is a valid
// intent, and so is any other fragment of an event type.
//
// The constants are therefore not mutually exclusive. An event matches every
// intent whose token appears in its name: [EventWarmupPlacement] matches both
// [IntentWarmup] and [IntentPlacement], [EventDirectEmailOpened] matches both
// [IntentEmail] and [IntentDirectEmail], and [EventContactImportProgress]
// matches [IntentContact] as well as [IntentImport]. Declaring several intents
// delivers the union of what they match.
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
	// replied, bounced, inbox arrivals and the direct-send pulses. It is the
	// highest-volume family.
	IntentEmail = "EMAIL"
	// IntentDirectEmail covers only the opens and clicks of messages sent
	// directly from a mailbox ([EventDirectEmailOpened] and
	// [EventDirectEmailClicked]).
	IntentDirectEmail = "DIRECT_EMAIL"
	// IntentContact covers contact creation, updates and deletion, and also
	// [EventContactImportProgress] and [EventContactsReload].
	IntentContact = "CONTACT"
	// IntentAccount covers mailbox connection, sync and health transitions.
	IntentAccount = "ACCOUNT"
	// IntentWarmup covers warmup health and placement changes, including
	// [EventWarmupPlacement].
	IntentWarmup = "WARMUP"
	// IntentPlacement covers inbox placement tests
	// ([EventPlacementTestUpdated]) and warmup placement verdicts
	// ([EventWarmupPlacement]).
	IntentPlacement = "PLACEMENT"
	// IntentImport covers mailbox and contact import progress
	// ([EventMailboxImportProgress] and [EventContactImportProgress]).
	IntentImport = "IMPORT"
	// IntentCRM covers connected-CRM sync signals ([EventCRMSynced]). The
	// trailing underscore keeps it clear of any event that merely contains the
	// letters.
	IntentCRM = "CRM_"
	// IntentBulk covers the BULK_* progress events. The server addresses them
	// to the member who started the operation, so they arrive on the user
	// topic and the operation's bulk topic; the workspace channel carries them
	// only when the event names the workspace. It does not match
	// [EventContactsReload], which [IntentContact] does.
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
