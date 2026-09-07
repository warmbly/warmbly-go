package warmbly

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"time"
)

// ContactService manages contacts (leads): search and bulk editing, the
// hydrated contact 360 view, address verification, per-campaign state, CRM
// notes and activities, import and export, and AI-assisted research.
type ContactService service

// MiniCampaign is a campaign reference embedded in another resource.
type MiniCampaign struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// MiniCategory is a contact category (also surfaced as a conversation label in
// the unified inbox) embedded in another resource.
type MiniCategory struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Color string `json:"color"`
}

// Who produced a contact's verification verdict, returned in
// [Contact.VerificationSource]. Empty means the address was never checked.
const (
	// VerificationSourceProbe is the built-in check (syntax, MX, SMTP probe).
	VerificationSourceProbe = "probe"
	// VerificationSourceProvider is a connected verification service.
	VerificationSourceProvider = "provider"
	// VerificationSourceImported is a verdict that arrived with the contact,
	// through [ContactInput.VerificationStatus] or an import column. The
	// background check leaves it alone until it ages out.
	VerificationSourceImported = "imported"
	// VerificationSourceManual is a verdict a member set through
	// [ContactService.RequestVerification]. It is never re-checked
	// automatically.
	VerificationSourceManual = "manual"
)

// Refinements of a verification status, returned in
// [Contact.VerificationSubStatus]. Empty when the status needs no qualifier.
const (
	VerificationSubStatusCatchAll    = "catch_all"
	VerificationSubStatusDisposable  = "disposable"
	VerificationSubStatusRole        = "role"
	VerificationSubStatusSpamtrap    = "spamtrap"
	VerificationSubStatusMailboxFull = "mailbox_full"
	VerificationSubStatusNoMX        = "no_mx"
	VerificationSubStatusSyntax      = "syntax"
	VerificationSubStatusUndisclosed = "undisclosed"
)

// Verification vocabularies accepted in [ContactInput.VerificationProvider]
// and [ImportColumnMapping.VerificationProvider], plus the two verifiers a
// workspace can run ([ContactVerificationOverview.Provider]).
const (
	// VerificationProviderBuiltin is Warmbly's own check.
	VerificationProviderBuiltin = "builtin"
	// VerificationProviderWarmbly names Warmbly's vocabulary (valid, risky,
	// invalid, unknown) when supplying a verdict you already hold.
	VerificationProviderWarmbly         = "warmbly"
	VerificationProviderMillionVerifier = "millionverifier"
	VerificationProviderZeroBounce      = "zerobounce"
	VerificationProviderNeverBounce     = "neverbounce"
	VerificationProviderBouncer         = "bouncer"
	VerificationProviderKickbox         = "kickbox"
	VerificationProviderEmailable       = "emailable"
	VerificationProviderDebounce        = "debounce"
	VerificationProviderClearout        = "clearout"
	VerificationProviderEmailListVerify = "emaillistverify"
)

// Contact is a contact (lead) as returned by the API.
type Contact struct {
	ID string `json:"id"`

	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Company   string `json:"company"`
	Phone     string `json:"phone"`

	CustomFields map[string]string `json:"custom_fields"`

	// Subscribed is the marketing-consent flag. Campaigns never send to an
	// unsubscribed contact.
	Subscribed bool           `json:"subscribed"`
	Campaigns  []MiniCampaign `json:"campaigns"`
	Categories []MiniCategory `json:"categories"`

	// VerificationStatus is the pre-send address check: [VerifyStatusValid],
	// [VerifyStatusRisky], [VerifyStatusInvalid] or [VerifyStatusUnknown].
	// Campaigns never send to invalid addresses, and send to risky ones only
	// when their risky-emails setting is on.
	VerificationStatus string `json:"verification_status"`
	// VerificationReason is a short explanation of the verdict.
	VerificationReason string `json:"verification_reason"`
	// VerificationSubStatus refines the status: one of the
	// VerificationSubStatus* constants, or empty.
	VerificationSubStatus string `json:"verification_sub_status"`
	// VerificationSource says who produced the verdict: one of the
	// VerificationSource* constants, or empty when never checked.
	VerificationSource string `json:"verification_source"`
	// VerificationProvider names the verifier or vocabulary behind the
	// verdict, for example [VerificationProviderBuiltin] or
	// [VerificationProviderZeroBounce].
	VerificationProvider string `json:"verification_provider"`
	// VerificationConfidence is 0 to 100, scored from the last check plus what
	// real mail to the address showed (deliveries, opens, replies, bounces).
	VerificationConfidence int        `json:"verification_confidence"`
	IsCatchAll             bool       `json:"is_catch_all"`
	VerificationCheckedAt  *time.Time `json:"verification_checked_at,omitempty"`

	// ESPProvider is the recipient's mail provider, derived from the domain:
	// "", "gmail", "outlook" or "other". Campaign ESP matching keys off it.
	ESPProvider   string     `json:"esp_provider"`
	ESPResolvedAt *time.Time `json:"esp_resolved_at,omitempty"`

	// CampaignLead is the contact's state within a single campaign. It is
	// populated only when a search filters by exactly one campaign.
	CampaignLead *ContactCampaignProgress `json:"campaign_lead,omitempty"`

	UpdatedAt time.Time `json:"updated_at"`
	CreatedAt time.Time `json:"created_at"`
}

// Derived lead states returned in [ContactCampaignProgress.Status],
// [ContactCampaignState.LeadStatus] and accepted by
// [ContactSearchParams.LeadStatus]. Highest priority first: unsubscribed,
// bounced, replied, failed, completed, active, undeliverable, pending.
const (
	// LeadStatusPending means the contact is enrolled but nothing has been sent.
	LeadStatusPending = "pending"
	// LeadStatusActive means some but not all steps have been sent.
	LeadStatusActive = "active"
	// LeadStatusCompleted means every step was sent with no reply.
	LeadStatusCompleted = "completed"
	// LeadStatusReplied is terminal: the contact replied.
	LeadStatusReplied = "replied"
	// LeadStatusBounced is terminal: a send hard-bounced.
	LeadStatusBounced = "bounced"
	// LeadStatusFailed is terminal: the mailbox could not send a step after
	// every retry. [ContactCampaignProgress.FailureReason] says why.
	LeadStatusFailed = "failed"
	// LeadStatusUnsubscribed is terminal: the contact is suppressed.
	LeadStatusUnsubscribed = "unsubscribed"
	// LeadStatusUndeliverable is a lead the campaign skips because address
	// verification refused it (invalid, or risky with the campaign's risky
	// toggle off). Re-verifying or marking it deliverable
	// ([ContactService.RequestVerification]) puts it back in the queue.
	LeadStatusUndeliverable = "undeliverable"
)

// Engagement filters accepted by [ContactSearchParams.Engagement]. Each is a
// predicate over the contact's progress in one campaign; the Not* forms match
// only leads that were sent at least one step, so a lead never emailed is
// neither opened nor not-opened. Opens are human opens only.
const (
	LeadEngagementOpened     = "opened"
	LeadEngagementNotOpened  = "not_opened"
	LeadEngagementClicked    = "clicked"
	LeadEngagementNotClicked = "not_clicked"
	LeadEngagementReplied    = "replied"
	LeadEngagementNotReplied = "not_replied"
	LeadEngagementBounced    = "bounced"
)

// ContactCampaignProgress is a contact's aggregate state inside one campaign.
type ContactCampaignProgress struct {
	// Status is one of the LeadStatus* constants.
	Status string `json:"status"`
	// Sent counts steps the worker delivered to the mailbox provider; a send
	// that could not complete is retried later and never shows here.
	Sent int `json:"sent"`
	// Opened counts steps opened by a person. Automated fetches (mail privacy
	// proxies and similar) are counted in MachineOpened instead.
	Opened         int        `json:"opened"`
	MachineOpened  int        `json:"machine_opened"`
	Clicked        int        `json:"clicked"`
	Replied        int        `json:"replied"`
	Bounced        int        `json:"bounced"`
	LastActivityAt *time.Time `json:"last_activity_at,omitempty"`
	// CurrentStep labels the latest step actually sent. It is empty while the
	// status is [LeadStatusPending].
	CurrentStep string `json:"current_step,omitempty"`
	// FailureReason is the sending worker's reason for the last failed send.
	// Set only when Status is [LeadStatusFailed].
	FailureReason string `json:"failure_reason,omitempty"`
}

// ContactEngagement summarizes every email touchpoint recorded for a contact.
// Opens count people only: fetches by a mail client or security gateway are
// left out, as they are in campaign analytics. A person's click counts as an
// open too.
type ContactEngagement struct {
	TotalSent       int `json:"total_sent"`
	TotalOpened     int `json:"total_opened"`
	TotalClicked    int `json:"total_clicked"`
	TotalReplied    int `json:"total_replied"`
	TotalBounced    int `json:"total_bounced"`
	TotalComplained int `json:"total_complained"`

	LastSentAt    *time.Time `json:"last_sent_at,omitempty"`
	LastOpenedAt  *time.Time `json:"last_opened_at,omitempty"`
	LastClickedAt *time.Time `json:"last_clicked_at,omitempty"`
	LastRepliedAt *time.Time `json:"last_replied_at,omitempty"`
	LastBouncedAt *time.Time `json:"last_bounced_at,omitempty"`
}

// ContactSuppression records why a contact's address is suppressed. It is nil
// when the contact is deliverable.
type ContactSuppression struct {
	// ID is the suppression-list entry; DELETE /suppressions/:id lifts it.
	ID string `json:"id"`
	// Kind is "email" when the contact's own address is on the list, or
	// "domain" when its whole domain is. Value is the matching entry.
	Kind   string `json:"kind"`
	Value  string `json:"value"`
	Reason string `json:"reason"`
	// Source is "bounce", "complaint", "unsubscribe", "manual" or "import".
	Source    string     `json:"source"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// Kinds of observation in [ContactVerificationEvidence.Kind].
const (
	VerificationEvidenceDelivered        = "delivered"
	VerificationEvidenceOpened           = "opened"
	VerificationEvidenceClicked          = "clicked"
	VerificationEvidenceReplied          = "replied"
	VerificationEvidenceAutoReplied      = "auto_replied"
	VerificationEvidenceBouncedRecipient = "bounced_recipient"
	VerificationEvidenceBouncedOther     = "bounced_other"
)

// ContactVerificationEvidence is one observed fact about the mailbox that fed
// the verification confidence.
type ContactVerificationEvidence struct {
	// Kind is one of the VerificationEvidence* constants.
	Kind       string    `json:"kind"`
	Detail     string    `json:"detail,omitempty"`
	ObservedAt time.Time `json:"observed_at"`
}

// ContactVerificationDetail is the "why" behind a contact's verdict, returned
// on the contact 360 view.
type ContactVerificationDetail struct {
	// Status is one of the VerifyStatus* constants.
	Status     string `json:"status"`
	Confidence int    `json:"confidence"`
	// Reasons are sentences, strongest first.
	Reasons []string `json:"reasons"`
	// Decisive is true when real mail, rather than a check, decided the status.
	Decisive bool `json:"decisive"`
	// Evidence lists the observations the score came from, newest first.
	Evidence []ContactVerificationEvidence `json:"evidence"`
}

// Where a contact first came from, returned in [ContactDetail.Source] and on
// [TimelineContactCreated] events. It never changes after creation.
const (
	// ContactSourceUnknown is the honest value for contacts created before
	// attribution existed.
	ContactSourceUnknown = "unknown"
	// ContactSourceManual is a contact added by hand in the dashboard.
	ContactSourceManual = "manual"
	// ContactSourceCampaign is a contact added from a campaign's Leads tab.
	ContactSourceCampaign = "campaign"
	// ContactSourceImport is a file import; the detail is the file name.
	ContactSourceImport = "import"
	// ContactSourceSheetSync is a Google Sheets sync; the detail is the sheet.
	ContactSourceSheetSync = "sheet_sync"
	// ContactSourceAPI is an API-key request; the detail is the key's name.
	ContactSourceAPI = "api"
	// ContactSourceAIAssistant is a contact the AI assistant created.
	ContactSourceAIAssistant = "ai_assistant"
	// ContactSourceForm is a hosted form submission; the detail is the form.
	ContactSourceForm = "form"
	// ContactSourceAutomation is an automation's "create or update contact"
	// action; the detail is the automation's name.
	ContactSourceAutomation = "automation"
)

// ContactDetail is the hydrated contact 360 view returned by
// [ContactService.Get]: the contact plus its engagement rollup, suppression
// state, verification explanation and first-touch attribution in one payload.
type ContactDetail struct {
	Contact
	Engagement  ContactEngagement   `json:"engagement"`
	Suppression *ContactSuppression `json:"suppression,omitempty"`
	// Verification explains the verdict: the reasons behind it and the
	// observations it was scored from.
	Verification *ContactVerificationDetail `json:"verification,omitempty"`

	// Source is one of the ContactSource* constants; SourceDetail names the
	// file, campaign, sheet, form, automation or API key behind it. Both are
	// fixed at creation.
	Source       string    `json:"source"`
	SourceDetail string    `json:"source_detail"`
	FirstSeenAt  time.Time `json:"first_seen_at"`
}

// ContactSentEmail is one row in the list of emails sent to a contact.
type ContactSentEmail struct {
	TaskID    string    `json:"task_id"`
	Status    string    `json:"status"`
	MessageID string    `json:"message_id"`
	Subject   string    `json:"subject"`
	SentAt    time.Time `json:"sent_at"`

	EmailAccountID    *string `json:"email_account_id,omitempty"`
	EmailAccountEmail *string `json:"email_account_email,omitempty"`
	EmailAccountName  *string `json:"email_account_name,omitempty"`

	CampaignID   *string `json:"campaign_id,omitempty"`
	CampaignName *string `json:"campaign_name,omitempty"`
	StepID       *string `json:"step_id,omitempty"`
	StepName     *string `json:"step_name,omitempty"`

	OpenedAt  *time.Time `json:"opened_at,omitempty"`
	ClickedAt *time.Time `json:"clicked_at,omitempty"`
	RepliedAt *time.Time `json:"replied_at,omitempty"`
	BouncedAt *time.Time `json:"bounced_at,omitempty"`
}

// Timeline event types returned in [TimelineEvent.Type].
const (
	TimelineEmailSent          = "email_sent"
	TimelineEmailOpened        = "email_opened"
	TimelineEmailClicked       = "email_clicked"
	TimelineEmailReplied       = "email_replied"
	TimelineEmailBounced       = "email_bounced"
	TimelineReplyReceived      = "reply_received"
	TimelineDeliverability     = "deliverability"
	TimelineSuppressed         = "suppressed"
	TimelineNote               = "note"
	TimelineMeetingBooked      = "meeting_booked"
	TimelineMeetingRescheduled = "meeting_rescheduled"
	TimelineMeetingCanceled    = "meeting_canceled"

	// Lifecycle events. TimelineContactCreated carries Source and
	// SourceDetail; the campaign and category events carry the name as it was
	// at the time, so a later rename does not rewrite history.
	TimelineContactCreated  = "contact_created"
	TimelineCampaignAdded   = "campaign_added"
	TimelineCampaignRemoved = "campaign_removed"
	TimelineCategoryAdded   = "category_added"
	TimelineCategoryRemoved = "category_removed"
	// TimelineFormSubmitted carries FormID and FormName.
	TimelineFormSubmitted = "form_submitted"
	// TimelinePageHit is a page view on your own site by a browser tied to the
	// contact through an email-link ticket. Subject is the page title (or its
	// path) and PageHit carries the full view.
	TimelinePageHit = "page_hit"
)

// Why an open or click was classified as automated, returned in
// [TimelineEvent.MachineReason].
const (
	// MachineReasonPrefetch is a mail privacy proxy or security gateway.
	MachineReasonPrefetch = "prefetch"
	// MachineReasonInstant is a fetch within ten seconds of the send.
	MachineReasonInstant = "instant"
	// MachineReasonBurst is several links followed within seconds (clicks only).
	MachineReasonBurst = "burst"
)

// ContactLinkClick names the link behind an [TimelineEmailClicked] event:
// where it went, the anchor text it was minted from, and the UTM parameters
// the destination carried. Every link in an email is tracked on its own, so
// each link clicked is its own event. Clicks recorded before per-link
// attribution have no link.
type ContactLinkClick struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	Label       string `json:"label,omitempty"`
	UTMSource   string `json:"utm_source,omitempty"`
	UTMMedium   string `json:"utm_medium,omitempty"`
	UTMCampaign string `json:"utm_campaign,omitempty"`
	UTMTerm     string `json:"utm_term,omitempty"`
	UTMContent  string `json:"utm_content,omitempty"`
	UserAgent   string `json:"user_agent,omitempty"`
}

// EngagementOrigin is what an open or click said about where it came from.
// Client names the mail client or image proxy when the user agent does
// (Gmail, Apple Mail, Outlook); the browser fields describe the rest. The
// location is resolved from the source network; the address itself is never
// stored. Every field is empty when unknown.
type EngagementOrigin struct {
	Client         string `json:"client,omitempty"`
	DeviceType     string `json:"device_type,omitempty"`
	OS             string `json:"os,omitempty"`
	Browser        string `json:"browser,omitempty"`
	BrowserVersion string `json:"browser_version,omitempty"`
	CountryCode    string `json:"country_code,omitempty"`
	Region         string `json:"region,omitempty"`
	City           string `json:"city,omitempty"`
}

// ContactPageHit is one page view from the website tracking snippet, as
// carried by a [TimelinePageHit] event. Landing marks the first view of a
// session; the device, language, screen and location fields are empty or zero
// when unknown.
type ContactPageHit struct {
	ID             string    `json:"id"`
	VisitorID      string    `json:"visitor_id"`
	SessionKey     string    `json:"session_key"`
	OccurredAt     time.Time `json:"occurred_at"`
	URL            string    `json:"url"`
	Path           string    `json:"path"`
	Title          string    `json:"title"`
	Referrer       string    `json:"referrer"`
	ReferrerDomain string    `json:"referrer_domain"`
	Landing        bool      `json:"landing"`
	UTMSource      string    `json:"utm_source"`
	UTMMedium      string    `json:"utm_medium"`
	UTMCampaign    string    `json:"utm_campaign"`
	UTMTerm        string    `json:"utm_term"`
	UTMContent     string    `json:"utm_content"`
	DeviceType     string    `json:"device_type"`
	OS             string    `json:"os"`
	Browser        string    `json:"browser"`
	BrowserVersion string    `json:"browser_version"`
	DeviceBrand    string    `json:"device_brand"`
	Language       string    `json:"language"`
	Timezone       string    `json:"timezone"`
	ScreenWidth    int       `json:"screen_width"`
	ScreenHeight   int       `json:"screen_height"`
	CountryCode    string    `json:"country_code"`
	Region         string    `json:"region"`
	City           string    `json:"city"`
}

// TimelineEvent is one entry in a contact's merged activity feed. Which
// optional fields are populated depends on Type.
type TimelineEvent struct {
	// Type is one of the Timeline* constants.
	Type string    `json:"type"`
	At   time.Time `json:"at"`

	EmailAccountID    *string `json:"email_account_id,omitempty"`
	EmailAccountEmail *string `json:"email_account_email,omitempty"`
	EmailAccountName  *string `json:"email_account_name,omitempty"`

	CampaignID   *string `json:"campaign_id,omitempty"`
	CampaignName *string `json:"campaign_name,omitempty"`
	StepID       *string `json:"step_id,omitempty"`
	StepName     *string `json:"step_name,omitempty"`

	TaskID *string `json:"task_id,omitempty"`
	// Subject is the email subject, or the page title for [TimelinePageHit].
	Subject *string `json:"subject,omitempty"`

	Reason *string `json:"reason,omitempty"`
	// Source is the suppression source on [TimelineSuppressed] events and the
	// first-touch ContactSource* value on [TimelineContactCreated] events.
	Source   *string `json:"source,omitempty"`
	Provider *string `json:"provider,omitempty"`
	// Intent is the reply-intent classification, for example "positive".
	Intent *string `json:"intent,omitempty"`
	// Content is the note body for [TimelineNote] events.
	Content *string `json:"content,omitempty"`

	// ScheduledFor, JoinURL and MeetingState are set on meeting events.
	ScheduledFor *time.Time `json:"scheduled_for,omitempty"`
	JoinURL      *string    `json:"join_url,omitempty"`
	MeetingState *string    `json:"meeting_state,omitempty"`

	// CategoryID and CategoryTitle are set on category lifecycle events;
	// SourceDetail on [TimelineContactCreated] (the file, campaign, sheet,
	// form, automation or API key name).
	CategoryID    *string `json:"category_id,omitempty"`
	CategoryTitle *string `json:"category_title,omitempty"`
	SourceDetail  *string `json:"source_detail,omitempty"`

	// FormID and FormName are set on [TimelineFormSubmitted] events.
	FormID   *string `json:"form_id,omitempty"`
	FormName *string `json:"form_name,omitempty"`

	// PageHit is the full page view behind a [TimelinePageHit] event.
	PageHit *ContactPageHit `json:"page_hit,omitempty"`

	// Machine is set on opens and clicks: true when an automated fetcher did
	// it rather than the person. MachineReason is one of the MachineReason*
	// constants when the event was logged individually. Automated clicks stay
	// on the feed for the record but never count the step as clicked.
	Machine       *bool   `json:"machine,omitempty"`
	MachineReason *string `json:"machine_reason,omitempty"`

	// Link is the exact link behind an [TimelineEmailClicked] event.
	Link *ContactLinkClick `json:"link,omitempty"`

	// Origin is the mail client, device and rough location of an open or
	// click, when the event was logged individually. Opens appear once per
	// event, so a contact who opened from two devices has two rows.
	Origin *EngagementOrigin `json:"origin,omitempty"`

	// UserID is the author of a note or the member behind a lifecycle event.
	UserID *string `json:"user_id,omitempty"`
}

// TimelineParams paginates [ContactService.ListTimeline].
type TimelineParams struct {
	// Limit is 1 to 200 (default 50). Anything else is a 400.
	ListOptions
	// Before is deprecated: it returns events strictly older than the
	// timestamp and can skip events sharing an instant with the page
	// boundary. It is ignored when Cursor is set. Prefer paging by cursor.
	Before *time.Time `json:"-"`
}

func (p *TimelineParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	setTime(q, "before", p.Before)
	return q
}

// ContactNote is a free-text CRM note on a contact.
type ContactNote struct {
	ID             string    `json:"id"`
	ContactID      string    `json:"contact_id"`
	OrganizationID string    `json:"organization_id"`
	UserID         string    `json:"user_id"`
	Content        string    `json:"content"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	// User is the note's author, joined in on read.
	User *User `json:"user,omitempty"`
}

// Contact activity types returned in [ContactActivity.ActivityType].
const (
	ActivityEmailSent       = "email_sent"
	ActivityEmailOpened     = "email_opened"
	ActivityEmailClicked    = "email_clicked"
	ActivityEmailReplied    = "email_replied"
	ActivityEmailBounced    = "email_bounced"
	ActivityNoteAdded       = "note_added"
	ActivityNoteUpdated     = "note_updated"
	ActivityDealCreated     = "deal_created"
	ActivityDealStageChange = "deal_stage_changed"
	ActivityDealWon         = "deal_won"
	ActivityDealLost        = "deal_lost"
	ActivityTaskCreated     = "task_created"
	ActivityTaskCompleted   = "task_completed"
	ActivityContactCreated  = "contact_created"
	ActivityContactUpdated  = "contact_updated"
	ActivityFormSubmitted   = "form_submitted"
	ActivityCampaignAdded   = "campaign_added"
	ActivityCampaignRemoved = "campaign_removed"
	ActivityCategoryAdded   = "category_added"
	ActivityCategoryRemoved = "category_removed"
)

// ContactActivity is one machine-recorded event on a contact's CRM record.
type ContactActivity struct {
	ID             string  `json:"id"`
	ContactID      string  `json:"contact_id"`
	OrganizationID string  `json:"organization_id"`
	UserID         *string `json:"user_id,omitempty"`
	// ActivityType is one of the Activity* constants.
	ActivityType string         `json:"activity_type"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"created_at"`
	User         *User          `json:"user,omitempty"`
}

// ContactVerificationCounts is how many of the organization's contacts carry
// each verification status. Pending is the subset of Unknown nobody has
// checked yet.
type ContactVerificationCounts struct {
	Valid   int `json:"valid"`
	Risky   int `json:"risky"`
	Invalid int `json:"invalid"`
	Unknown int `json:"unknown"`
	Pending int `json:"pending"`
}

// ContactsCounts are organization-wide contact facet totals, returned on the
// first page of a search for the browse sidebar. They are independent of the
// search's own filters.
type ContactsCounts struct {
	Total        int                       `json:"total"`
	Subscribed   int                       `json:"subscribed"`
	Unsubscribed int                       `json:"unsubscribed"`
	InCampaign   int                       `json:"in_campaign"`
	NotContacted int                       `json:"not_contacted"`
	Categories   []ContactCategoryCount    `json:"categories"`
	Verification ContactVerificationCounts `json:"verification"`
}

// ContactCategoryCount is how many contacts carry one category.
type ContactCategoryCount struct {
	CategoryID string `json:"category_id"`
	Count      int    `json:"count"`
}

// CampaignLeadCounts are per-status lead totals within a single campaign,
// returned on the first page of a search that filters by exactly one campaign.
// They ignore the search's own lead-status and engagement filters, so every
// bucket shows its real total.
type CampaignLeadCounts struct {
	Total int `json:"total"`
	// Queued is the pending bucket; Processing is the active bucket.
	Queued       int `json:"queued"`
	Processing   int `json:"processing"`
	Completed    int `json:"completed"`
	Replied      int `json:"replied"`
	Bounced      int `json:"bounced"`
	Failed       int `json:"failed"`
	Unsubscribed int `json:"unsubscribed"`
	// Undeliverable leads were refused by address verification.
	Undeliverable int `json:"undeliverable"`

	// Engagement totals matching the LeadEngagement* filters: leads sent at
	// least one step, and of those the ones with a human open, a click, or a
	// reply on any step (whatever their derived status).
	Contacted  int `json:"contacted"`
	Opened     int `json:"opened"`
	Clicked    int `json:"clicked"`
	RepliedAny int `json:"replied_any"`
}

// Custom-field match modes for [ContactFieldFilter.Type].
const (
	FilterEqual      = "equal"
	FilterStartsWith = "starts_with"
	FilterEndsWith   = "ends_with"
	FilterContains   = "contains"
)

// ContactFieldFilter matches one custom field.
type ContactFieldFilter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	// Type is one of the Filter* constants.
	Type string `json:"type"`
}

// ContactSearchParams filters and paginates a contact search. Cursor, Limit and
// Category travel in the query string; every other field is sent in the body.
type ContactSearchParams struct {
	ListOptions
	// Category is a convenience filter for a single category id.
	Category string `json:"-"`

	// Query is a free-text search across the core contact fields.
	Query              string               `json:"query,omitempty"`
	CustomFieldFilters []ContactFieldFilter `json:"custom_field_filters,omitempty"`
	// CampaignIDs matches contacts enrolled in every listed campaign.
	CampaignIDs []string `json:"campaign_ids,omitempty"`
	// LeadStatus narrows to one derived lead status (a LeadStatus* constant)
	// inside the single campaign in CampaignIDs. Any other number of campaigns
	// is rejected with code "lead_filter_requires_campaign"; an unknown value
	// with "invalid_lead_status".
	LeadStatus string `json:"lead_status,omitempty"`
	// Engagement narrows by engagement inside that same single campaign (a
	// LeadEngagement* constant), ANDed with LeadStatus. It has the same
	// single-campaign requirement; an unknown value is rejected with
	// "invalid_engagement".
	Engagement string `json:"engagement,omitempty"`
	// CategoryIDs matches contacts carrying every listed category.
	CategoryIDs []string `json:"category_ids,omitempty"`
	// SegmentIDs matches contacts that are members of every listed segment
	// (conditions plus manual overrides). A malformed id is a 400; an unknown
	// segment matches nothing.
	SegmentIDs   []string `json:"segment_ids,omitempty"`
	MinCampaigns *int     `json:"min_campaigns,omitempty"`
	MaxCampaigns *int     `json:"max_campaigns,omitempty"`
	Subscribed   *bool    `json:"subscribed,omitempty"`
	// VerificationStatus filters by verdict: one of the VerifyStatus*
	// constants.
	VerificationStatus string     `json:"verification_status,omitempty"`
	CreatedAfter       *time.Time `json:"created_after,omitempty"`
	CreatedBefore      *time.Time `json:"created_before,omitempty"`
	UpdatedAfter       *time.Time `json:"updated_after,omitempty"`
	UpdatedBefore      *time.Time `json:"updated_before,omitempty"`
	// SortBy names the column to order by, for example "first_name" or
	// "campaign_count".
	SortBy string `json:"sort_by,omitempty"`
	// Reverse switches the sort to descending.
	Reverse bool `json:"reverse,omitempty"`
}

func (p *ContactSearchParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	setNonEmpty(q, "category", p.Category)
	return q
}

// ContactPage is one page of contact search results. Beyond the page itself it
// carries the facet counts the API returns on the first page.
type ContactPage struct {
	Page[Contact]

	// Counts are organization-wide facet totals, present on the first page only.
	Counts *ContactsCounts `json:"counts,omitempty"`
	// LeadCounts are per-status totals for a single-campaign search, present on
	// the first page only.
	LeadCounts *CampaignLeadCounts `json:"lead_counts,omitempty"`
}

// ContactInput creates one contact. Email is required.
//
// An address the organization already has is matched (case-insensitively) and
// enriched rather than duplicated: fields you send replace what is stored,
// fields you omit or leave empty are kept, and CustomFields is merged key by
// key. Use [ContactService.Update] to clear a value.
type ContactInput struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	Company   string `json:"company,omitempty"`
	Phone     string `json:"phone,omitempty"`
	// Campaigns and Categories are ids to enroll the new contact in.
	Campaigns  []string `json:"campaigns,omitempty"`
	Categories []string `json:"categories,omitempty"`
	// Segments pins the contact into these segments as a manual include
	// override, so it belongs whether or not the conditions match it. An
	// unknown id is a 400 before any contact is written.
	Segments []string `json:"segments,omitempty"`
	// CustomFields keys may use letters, numbers, underscores, spaces and
	// dashes.
	CustomFields map[string]string `json:"custom_fields,omitempty"`
	// Subscribed is the marketing-consent flag. Nil lets a new contact default
	// to subscribed and an existing one keep what it had.
	Subscribed *bool `json:"subscribed,omitempty"`
	// VerificationStatus is a verdict you already hold for the address, in
	// Warmbly's vocabulary (the VerifyStatus* constants) or any known
	// service's ("ok", "catch-all", "do_not_mail", "deliverable", ...). It is
	// stored as an imported verdict the background check leaves alone. A
	// value no known service writes is rejected with code
	// "unknown_verification_status".
	VerificationStatus string `json:"verification_status,omitempty"`
	// VerificationProvider names the vocabulary VerificationStatus is written
	// in (a VerificationProvider* constant). Empty recognizes the value by
	// itself; an unknown name is rejected with "unknown_verification_provider".
	VerificationProvider string `json:"verification_provider,omitempty"`
	// Source is the first-touch attribution stamped on a new contact. Only
	// [ContactSourceManual] and [ContactSourceCampaign] may be claimed, and
	// only by a user-scoped (OAuth) caller; a request authenticated with an
	// API key is always recorded as [ContactSourceAPI] under the key's name.
	// An existing contact keeps its original source.
	Source string `json:"source,omitempty"`
}

// ContactUpdateParams updates a single contact. Nil fields are unchanged.
//
// Categories replaces the contact's categories wholesale, while AddCategories
// and RemoveCategories adjust them incrementally. Use one form or the other.
type ContactUpdateParams struct {
	FirstName    *string           `json:"first_name,omitempty"`
	LastName     *string           `json:"last_name,omitempty"`
	Company      *string           `json:"company,omitempty"`
	Phone        *string           `json:"phone,omitempty"`
	CustomFields map[string]string `json:"custom_fields,omitempty"`
	Subscribed   *bool             `json:"subscribed,omitempty"`

	Campaigns        []string `json:"campaigns,omitempty"`
	Categories       []string `json:"categories,omitempty"`
	AddCategories    []string `json:"add_categories,omitempty"`
	RemoveCategories []string `json:"remove_categories,omitempty"`
}

// Bulk custom-field operations for [ContactFieldEdit.Type].
const (
	// FieldOpAdd sets the field only where it is currently absent.
	FieldOpAdd = "ADD"
	// FieldOpEdit overwrites the field's value.
	FieldOpEdit = "EDIT"
	// FieldOpDelete removes the field.
	FieldOpDelete = "DELETE"
	// FieldOpRename renames the key to Value, keeping each contact's value.
	FieldOpRename = "RENAME"
)

// ContactFieldEdit is one custom-field change in a bulk update.
type ContactFieldEdit struct {
	// Type is one of the FieldOp* constants.
	Type  string `json:"type"`
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

// ContactBulkUpdateParams edits many contacts at once.
type ContactBulkUpdateParams struct {
	// Contacts are the contact ids to edit, at most 1,000 per request.
	Contacts []string `json:"contacts"`

	AddCampaigns     []string           `json:"add_campaigns,omitempty"`
	RemoveCampaigns  []string           `json:"remove_campaigns,omitempty"`
	AddCategories    []string           `json:"add_categories,omitempty"`
	RemoveCategories []string           `json:"remove_categories,omitempty"`
	Fields           []ContactFieldEdit `json:"fields,omitempty"`
	// Subscribe sets the subscription flag on every listed contact.
	Subscribe *bool `json:"subscribe,omitempty"`
}

// Actions accepted by [ContactVerificationParams.Action].
const (
	// VerificationActionVerify queues a fresh check. Each contact updates as
	// its verdict lands.
	VerificationActionVerify = "verify"
	// VerificationActionMarkDeliverable records a manual "valid" verdict and
	// resumes any campaign of the workspace paused for verification.
	VerificationActionMarkDeliverable = "mark_deliverable"
	// VerificationActionMarkUndeliverable records a manual "invalid" verdict.
	VerificationActionMarkUndeliverable = "mark_undeliverable"
)

// ContactVerificationParams selects contacts for
// [ContactService.RequestVerification]. Contacts and CampaignID combine; at
// least one contact must be selected or the request is rejected with code
// "no_contacts".
type ContactVerificationParams struct {
	// Action is one of the VerificationAction* constants. Anything else is
	// rejected with code "invalid_action".
	Action string `json:"action"`
	// Contacts are contact ids, at most 1,000 per request
	// ("too_many_contacts").
	Contacts []string `json:"contacts,omitempty"`
	// CampaignID selects every lead of the campaign that verification refused
	// (the [LeadStatusUndeliverable] ones), instead of or as well as Contacts.
	CampaignID string `json:"campaign_id,omitempty"`
}

// ContactVerificationResult reports how many contacts a verification action
// touched.
type ContactVerificationResult struct {
	Affected int    `json:"affected"`
	Action   string `json:"action"`
	// Queued is true for [VerificationActionVerify]: the check runs in the
	// background rather than in the request.
	Queued bool `json:"queued"`
}

// ContactVerificationOverview says who checks the workspace's addresses and how
// its contacts split by verdict.
type ContactVerificationOverview struct {
	// Provider is the verifier in use: [VerificationProviderBuiltin] or
	// [VerificationProviderMillionVerifier].
	Provider string `json:"provider"`
	// ConnectionID is the integration connection behind a paid provider.
	ConnectionID *string `json:"connection_id,omitempty"`
	// Credits is the paid provider's remaining balance, when it could be read.
	Credits *int `json:"credits,omitempty"`
	// ProviderError is set when a paid provider is connected but unusable (a
	// rejected key, no credits); the built-in check is in use meanwhile.
	ProviderError string `json:"provider_error,omitempty"`
	// BuiltinReady says whether the built-in mailbox probe can reach mail
	// servers from this instance. Off, it still checks syntax, MX and
	// disposable domains.
	BuiltinReady bool                      `json:"builtin_ready"`
	Counts       ContactVerificationCounts `json:"counts"`
}

// How firm a [ContactNextAction]'s timing is, in [ContactNextAction.State].
const (
	// NextActionDue carries ScheduledAt, the slot the scheduler would give the
	// step on its next pass. Leads ahead in the queue can still push it later.
	NextActionDue = "due"
	// NextActionWaiting carries NotBefore and a Constraint.
	NextActionWaiting = "waiting"
	// NextActionPaused and NextActionBlocked carry only the Constraint.
	NextActionPaused  = "paused"
	NextActionBlocked = "blocked"
)

// ContactNextAction is what happens next to a contact in a campaign. It is
// derived on read by the scheduler through the same constraints a real send
// goes through; nothing per contact is stored.
type ContactNextAction struct {
	// StepID is nil while a branch condition is still undecided, in which
	// case StepLabel says the step depends on the contact's response.
	StepID    *string `json:"step_id,omitempty"`
	StepLabel string  `json:"step_label"`
	Kind      string  `json:"kind,omitempty"`
	Subject   string  `json:"subject,omitempty"`
	// State is one of the NextAction* constants.
	State string `json:"state"`
	// ScheduledAt is set only when due; NotBefore is the earliest the hard
	// constraints allow; Constraint names the gate in user-facing words.
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
	NotBefore   *time.Time `json:"not_before,omitempty"`
	Constraint  string     `json:"constraint,omitempty"`
}

// ContactCampaignStep is one flow node with the contact's progress on it.
type ContactCampaignStep struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Kind     string `json:"kind"`
	Position int    `json:"position"`
	Subject  string `json:"subject,omitempty"`

	SentAt    *time.Time `json:"sent_at,omitempty"`
	OpenedAt  *time.Time `json:"opened_at,omitempty"`
	ClickedAt *time.Time `json:"clicked_at,omitempty"`
	RepliedAt *time.Time `json:"replied_at,omitempty"`
	BouncedAt *time.Time `json:"bounced_at,omitempty"`
	FailedAt  *time.Time `json:"failed_at,omitempty"`
	// Attempts counts failed sends; InFlight means a worker holds a
	// reservation whose result has not come back yet.
	Attempts int  `json:"attempts,omitempty"`
	InFlight bool `json:"in_flight,omitempty"`
}

// ContactCampaignState is one campaign a contact is a lead of: the flow with
// the contact's progress on each step, the derived lead status, the last
// thing that happened and what happens next.
type ContactCampaignState struct {
	CampaignID     string `json:"campaign_id"`
	CampaignName   string `json:"campaign_name"`
	CampaignStatus string `json:"campaign_status"`
	// LeadStatus is one of the LeadStatus* constants.
	LeadStatus string `json:"lead_status"`
	// FailureReason is the worker's reason for the last failed send.
	FailureReason string `json:"failure_reason,omitempty"`

	Steps          []ContactCampaignStep `json:"steps"`
	CompletedSteps int                   `json:"completed_steps"`
	TotalSteps     int                   `json:"total_steps"`

	// CurrentStep is the latest step sent.
	CurrentStep  *ContactCampaignStep `json:"current_step,omitempty"`
	LastAction   string               `json:"last_action,omitempty"`
	LastActionAt *time.Time           `json:"last_action_at,omitempty"`

	// Next is nil once the flow has ended for the contact; EndedReason says
	// why.
	Next        *ContactNextAction `json:"next,omitempty"`
	EndedReason string             `json:"ended_reason,omitempty"`
}

// ContactSegmentMembership is one segment of the organization seen from a
// contact: whether the contact is in it right now and any manual override.
type ContactSegmentMembership struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	// Mode is "include" or "exclude" when the contact carries a manual
	// override, and empty when the conditions alone decide.
	Mode string `json:"mode,omitempty"`
	// Member reports whether the contact is currently in the segment.
	Member bool `json:"member"`
}

// Export formats accepted by [ContactExportParams.Format].
const (
	ExportFormatCSV  = "csv"
	ExportFormatXLSX = "xlsx"
	ExportFormatJSON = "json"
)

// Export scopes accepted by [ContactExportParams.Scope].
const (
	// ExportScopeAll exports every contact in the organization.
	ExportScopeAll = "all"
	// ExportScopeFiltered exports whatever [ContactExportParams.Filters] match.
	ExportScopeFiltered = "filtered"
	// ExportScopeSelected exports the ids in [ContactExportParams.ContactIDs].
	ExportScopeSelected = "selected"
)

// Built-in export column identifiers. A custom field is addressed as
// "custom:<key>".
const (
	ExportFieldID         = "id"
	ExportFieldEmail      = "email"
	ExportFieldFirstName  = "first_name"
	ExportFieldLastName   = "last_name"
	ExportFieldCompany    = "company"
	ExportFieldPhone      = "phone"
	ExportFieldSubscribed = "subscribed"
	ExportFieldCategories = "categories"
	ExportFieldCampaigns  = "campaigns"
	ExportFieldCreatedAt  = "created_at"
	ExportFieldUpdatedAt  = "updated_at"

	// The lead columns are the contact's engagement inside the one campaign
	// named in [ContactExportParams.Filters].CampaignIDs. They are blank when
	// the filters do not name exactly one campaign.
	ExportFieldLeadStatus  = "lead_status"
	ExportFieldLeadOpened  = "lead_opened"
	ExportFieldLeadClicked = "lead_clicked"
	ExportFieldLeadReplied = "lead_replied"
)

// ContactExportParams describes an export. A single export is capped at 50,000
// rows server-side.
type ContactExportParams struct {
	// Format is [ExportFormatCSV], [ExportFormatXLSX] or [ExportFormatJSON].
	Format string `json:"format"`
	// Scope is [ExportScopeAll], [ExportScopeFiltered] or [ExportScopeSelected].
	Scope string `json:"scope"`
	// ContactIDs is required for [ExportScopeSelected].
	ContactIDs []string `json:"contact_ids,omitempty"`
	// Filters is required for [ExportScopeFiltered]. With
	// [ExportScopeSelected] it is optional and applied on top of ContactIDs;
	// name the campaign there to populate the ExportFieldLead* columns.
	Filters *ContactSearchParams `json:"filters,omitempty"`
	// Fields are column identifiers in display order. Empty means the
	// recommended default set.
	Fields []string `json:"fields,omitempty"`
	// Filename is the download name without an extension. Empty falls back to
	// "contacts-<date>".
	Filename string `json:"filename,omitempty"`
}

// Where an imported column lands, for [ImportColumnMapping.Target].
const (
	ImportTargetIgnore     = "ignore"
	ImportTargetEmail      = "email"
	ImportTargetFirstName  = "first_name"
	ImportTargetLastName   = "last_name"
	ImportTargetCompany    = "company"
	ImportTargetPhone      = "phone"
	ImportTargetSubscribed = "subscribed"
	ImportTargetCategories = "categories"
	// ImportTargetVerificationStatus reads a verdict column written by Warmbly
	// or another verification service. A cell nobody recognizes leaves that
	// contact unverified rather than failing the row.
	ImportTargetVerificationStatus = "verification_status"
	// ImportTargetCustom routes the column into a custom field named by
	// [ImportColumnMapping.CustomKey]. The older "custom:<key>" spelling is
	// still accepted.
	ImportTargetCustom = "custom"
)

// How an import treats a row whose address already exists, for
// [ContactImportParams.Dedup].
const (
	ImportDedupSkip            = "skip"
	ImportDedupUpdate          = "update"
	ImportDedupCreateDuplicate = "create_duplicate"
)

// ImportColumnMapping maps the column at Index to a contact field. The index is
// zero-based and matches [ContactImportPreview.Columns].
type ImportColumnMapping struct {
	Index int `json:"index"`
	// Target is one of the ImportTarget* constants, or "custom:<key>".
	Target string `json:"target"`
	// CustomKey is the custom field an [ImportTargetCustom] column writes to.
	// It may use letters, numbers, underscores, spaces and dashes.
	CustomKey string `json:"custom_key,omitempty"`
	// VerificationProvider names the vocabulary of an
	// [ImportTargetVerificationStatus] column (a VerificationProvider*
	// constant). Empty recognizes each value by itself.
	VerificationProvider string `json:"verification_provider,omitempty"`
}

// ContactImportPreview describes an uploaded file before anything is written.
type ContactImportPreview struct {
	Filename  string `json:"filename"`
	Format    string `json:"format"`
	TotalRows int    `json:"total_rows"`
	// Columns are the detected headers. For a headerless file the server
	// synthesizes "Column 1", "Column 2" and so on.
	Columns   []string `json:"columns"`
	HasHeader bool     `json:"has_header"`
	// SampleRows are the first rows verbatim.
	SampleRows [][]string `json:"sample_rows"`
	// SuggestedMapping is a heuristic default the caller may override. It
	// proposes [ImportTargetVerificationStatus] itself when a column's header
	// or values look like another service's results.
	SuggestedMapping []ImportColumnMapping `json:"suggested_mapping"`
}

// ContactImportParams is the configuration committed alongside the file.
//
// Exactly one column must map to [ImportTargetEmail]. A mapping with no email
// column, a custom column with no CustomKey, or a custom key with characters
// outside letters, numbers, underscores, spaces and dashes is a 400 on the
// whole request, raised before any row is written.
type ContactImportParams struct {
	Mapping []ImportColumnMapping `json:"mapping"`
	// Dedup is one of the ImportDedup* constants.
	Dedup     string `json:"dedup,omitempty"`
	HasHeader bool   `json:"has_header"`
	// CategoryIDs and CampaignIDs are applied to every imported contact.
	CategoryIDs []string `json:"category_ids,omitempty"`
	CampaignIDs []string `json:"campaign_ids,omitempty"`
	// SegmentIDs pins every row into these segments as a manual include
	// override: imported, updated and skipped-but-matched contacts alike.
	SegmentIDs []string `json:"segment_ids,omitempty"`
	// SubscribedDefault is what new contacts inherit when no subscribed column
	// was mapped. It defaults to true server-side.
	SubscribedDefault *bool `json:"subscribed_default,omitempty"`
}

// ImportRowError is a row that could not be imported. Line is the 1-based index
// into the source file, after the header when there was one.
type ImportRowError struct {
	Line   int      `json:"line"`
	Email  string   `json:"email,omitempty"`
	Values []string `json:"values,omitempty"`
	Reason string   `json:"reason"`
}

// ContactImportQuality is what the uploaded addresses looked like, measured
// at import. It is advisory: a flagged import still stores every row it could
// parse. A list bad enough to matter is refused at campaign launch instead.
type ContactImportQuality struct {
	// Malformed rows are not addresses at all; Disposable are on known
	// throwaway domains.
	Malformed  int `json:"malformed"`
	Disposable int `json:"disposable"`
	// Role counts shared inboxes such as info@. Reported but not counted as
	// bad, since mailing a shared inbox is a choice.
	Role int `json:"role"`
	// BadSharePct is malformed plus disposable as a percentage of the file.
	BadSharePct float64 `json:"bad_share_pct"`
	// Flagged is set above 25% on files of at least 20 rows, with a Summary
	// sentence.
	Flagged bool   `json:"flagged"`
	Summary string `json:"summary,omitempty"`
}

// ContactImportResult summarizes a committed import. Every row lands in
// exactly one of Imported, Updated, Skipped or Failed, so those always sum to
// Total.
type ContactImportResult struct {
	Total     int       `json:"total"`
	Imported  int       `json:"imported"`
	Updated   int       `json:"updated"`
	Skipped   int       `json:"skipped"`
	Failed    int       `json:"failed"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	// Errors holds at most the first 1,000 per-row failures. Past that
	// ErrorsTruncated is true and the counters, not the list, are the real
	// totals.
	Errors          []ImportRowError `json:"errors,omitempty"`
	ErrorsTruncated bool             `json:"errors_truncated,omitempty"`
	// Quality is the file's address-level assessment.
	Quality *ContactImportQuality `json:"quality,omitempty"`
}

// Research run states returned in [ResearchRun.Status]. [ResearchNothingFound]
// is a billable success: the agent looked and honestly found nothing.
const (
	ResearchQueued       = "queued"
	ResearchRunning      = "running"
	ResearchSucceeded    = "succeeded"
	ResearchFailed       = "failed"
	ResearchNothingFound = "nothing_found"
)

// ResearchRun is one AI research attempt against a contact. Runs spend AI
// credits.
type ResearchRun struct {
	ID          string  `json:"id"`
	OrgID       string  `json:"org_id"`
	ContactID   string  `json:"contact_id"`
	RequestedBy *string `json:"requested_by,omitempty"`
	// Status is one of the Research* constants.
	Status         string         `json:"status"`
	Objective      string         `json:"objective"`
	Result         ResearchResult `json:"result"`
	Error          string         `json:"error,omitempty"`
	CreditsCharged int            `json:"credits_charged"`
	ModelUsed      string         `json:"model_used"`
	TokensUsed     int            `json:"tokens_used"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// ResearchResult is what a run found. Every signal and artifact carries a
// source URL; the server rejects anything uncited.
type ResearchResult struct {
	Company *ResearchCompany `json:"company,omitempty"`
	Person  *ResearchPerson  `json:"person,omitempty"`
	Signals []ResearchSignal `json:"signals,omitempty"`
	Hooks   []ResearchHook   `json:"hooks,omitempty"`
	// CustomFieldUpdates are contact custom fields the run proposes.
	CustomFieldUpdates map[string]string `json:"custom_field_updates,omitempty"`
	ResearchNotes      string            `json:"research_notes,omitempty"`
	NothingFound       bool              `json:"nothing_found"`
}

// ResearchCompany is what a run learned about the contact's company.
type ResearchCompany struct {
	Summary            string   `json:"summary,omitempty"`
	Industry           string   `json:"industry,omitempty"`
	SizeEstimate       string   `json:"size_estimate,omitempty"`
	SellsTo            string   `json:"sells_to,omitempty"`
	TechOrStackSignals []string `json:"tech_or_stack_signals,omitempty"`
}

// ResearchPerson is what a run learned about the contact themselves.
type ResearchPerson struct {
	RoleConfirmed   bool               `json:"role_confirmed"`
	Title           string             `json:"title,omitempty"`
	PublicArtifacts []ResearchArtifact `json:"public_artifacts,omitempty"`
}

// ResearchArtifact is something public the contact produced.
type ResearchArtifact struct {
	What  string `json:"what"`
	Where string `json:"where"`
	When  string `json:"when,omitempty"`
	URL   string `json:"url"`
}

// ResearchSignal is a cited fact. Confidence is "high", "medium" or "low".
type ResearchSignal struct {
	Type       string `json:"type"`
	Fact       string `json:"fact"`
	When       string `json:"when,omitempty"`
	URL        string `json:"url"`
	Confidence string `json:"confidence"`
}

// ResearchHook is a suggested opener grounded in a signal.
type ResearchHook struct {
	BasedOn     string `json:"based_on"`
	WhyRelevant string `json:"why_relevant"`
	OpenerLine  string `json:"opener_line"`
}

// Search returns a page of contacts matching the filters.
func (s *ContactService) Search(ctx context.Context, params *ContactSearchParams, opts ...RequestOption) (*ContactPage, error) {
	return searchContacts(ctx, s.client, params, opts)
}

func searchContacts(ctx context.Context, c *Client, params *ContactSearchParams, opts []RequestOption) (*ContactPage, error) {
	page := &ContactPage{}
	resp, err := c.post(ctx, withQuery("contacts/search", params.values()), params, page, opts...)
	if err != nil {
		return nil, err
	}
	page.resp = resp
	page.fetch = func(ctx context.Context, cursor string) (*Page[Contact], error) {
		var next ContactSearchParams
		if params != nil {
			next = *params
		}
		next.Cursor = cursor
		p, err := searchContacts(ctx, c, &next, opts)
		if err != nil {
			return nil, err
		}
		return &p.Page, nil
	}
	return page, nil
}

// Create adds contacts to the organization and returns the resulting records,
// index-aligned with the input. An address that already exists is enriched
// rather than duplicated (see [ContactInput]). New addresses are queued for
// verification right away.
//
// A contact.created webhook (and any automation it triggers) fires for each
// genuinely new row, but only when the request carries 100 contacts or fewer:
// a larger batch is treated as a bulk arrival, like a file import, and stays
// silent so one call cannot flood the organization's automations.
func (s *ContactService) Create(ctx context.Context, contacts []ContactInput, opts ...RequestOption) ([]Contact, *Response, error) {
	return sendSlice[Contact](ctx, s.client.post, "contacts", contacts, opts)
}

// BulkUpdate edits many contacts at once and returns the updated records. It
// takes at most 1,000 contacts per request and never creates contacts, so it
// never raises contact.created.
func (s *ContactService) BulkUpdate(ctx context.Context, params *ContactBulkUpdateParams, opts ...RequestOption) ([]Contact, *Response, error) {
	return sendSlice[Contact](ctx, s.client.patch, "contacts", params, opts)
}

// BulkDelete permanently removes the given contacts, at most 1,000 per request.
func (s *ContactService) BulkDelete(ctx context.Context, ids []string, opts ...RequestOption) (*Response, error) {
	return s.client.deleteBody(ctx, "contacts", ids, opts...)
}

// Get retrieves the hydrated contact 360 view.
func (s *ContactService) Get(ctx context.Context, id string, opts ...RequestOption) (*ContactDetail, *Response, error) {
	return fetch[ContactDetail](ctx, s.client, "contacts/"+url.PathEscape(id), opts)
}

// Update modifies a single contact.
func (s *ContactService) Update(ctx context.Context, id string, params *ContactUpdateParams, opts ...RequestOption) (*Contact, *Response, error) {
	return send[Contact](ctx, s.client.patch, "contacts/"+url.PathEscape(id), params, opts)
}

// Delete permanently removes a contact.
func (s *ContactService) Delete(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "contacts/"+url.PathEscape(id), opts...)
}

// Lookup resolves an email address to a contact. A "Display Name <addr>" form
// is accepted and reduced to the bare address. The contact is nil when no
// contact in the organization owns that address.
func (s *ContactService) Lookup(ctx context.Context, email string, opts ...RequestOption) (*ContactDetail, *Response, error) {
	var env struct {
		Contact *ContactDetail `json:"contact"`
	}
	q := url.Values{"email": {email}}
	resp, err := s.client.get(ctx, withQuery("contacts/lookup", q), &env, opts...)
	if err != nil {
		return nil, resp, err
	}
	return env.Contact, resp, nil
}

// CustomFields returns the organization's distinct contact custom-field keys,
// most common first, capped at 200. Use it to populate a merge-tag picker.
func (s *ContactService) CustomFields(ctx context.Context, opts ...RequestOption) ([]string, *Response, error) {
	return fetchData[string](ctx, s.client, "contacts/custom-fields", opts)
}

// VerificationOverview reports which verifier checks the workspace's
// addresses, its remaining credits, and the contacts by verdict.
func (s *ContactService) VerificationOverview(ctx context.Context, opts ...RequestOption) (*ContactVerificationOverview, *Response, error) {
	return fetch[ContactVerificationOverview](ctx, s.client, "contacts/verification", opts)
}

// RequestVerification queues a fresh check of the selected contacts or records
// a manual verdict on them, depending on [ContactVerificationParams.Action].
// Verification runs in the background: poll the contacts or subscribe to the
// gateway to see verdicts land.
func (s *ContactService) RequestVerification(ctx context.Context, params *ContactVerificationParams, opts ...RequestOption) (*ContactVerificationResult, *Response, error) {
	return send[ContactVerificationResult](ctx, s.client.post, "contacts/verification", params, opts)
}

// Emails returns the emails sent to a contact.
func (s *ContactService) Emails(ctx context.Context, id string, opts ...RequestOption) ([]ContactSentEmail, *Response, error) {
	return fetchData[ContactSentEmail](ctx, s.client, "contacts/"+url.PathEscape(id)+"/emails", opts)
}

// Timeline returns the first page of the contact's merged activity feed, newest
// first, using the server default of 50 events. Use
// [ContactService.ListTimeline] to page through the whole feed.
func (s *ContactService) Timeline(ctx context.Context, id string, opts ...RequestOption) ([]TimelineEvent, *Response, error) {
	return fetchData[TimelineEvent](ctx, s.client, "contacts/"+url.PathEscape(id)+"/timeline", opts)
}

// ListTimeline returns a page of the contact's merged activity feed: sends,
// opens, clicks (one per link), replies, bounces, deliverability and
// suppression events, notes, meetings, lifecycle events and page views. Pages
// resume at the exact position of the last event, so events sharing a
// timestamp are never skipped or repeated. Pagination.Total is always nil: the
// feed is merged from several tables and never counted.
func (s *ContactService) ListTimeline(ctx context.Context, id string, params *TimelineParams, opts ...RequestOption) (*Page[TimelineEvent], error) {
	return listJSON[TimelineEvent](ctx, s.client, "contacts/"+url.PathEscape(id)+"/timeline", params.values(), opts...)
}

// CampaignStates returns, for every campaign the contact is a lead of, the
// flow with the contact's progress on each step, the derived lead status, and
// the scheduler's next action.
func (s *ContactService) CampaignStates(ctx context.Context, id string, opts ...RequestOption) ([]ContactCampaignState, *Response, error) {
	return fetchData[ContactCampaignState](ctx, s.client, "contacts/"+url.PathEscape(id)+"/campaigns", opts)
}

// Segments returns every segment in the organization with whether the contact
// is currently a member and any manual override on it. Membership is
// evaluated live, so the answer reflects the contact as it is now.
func (s *ContactService) Segments(ctx context.Context, id string, opts ...RequestOption) ([]ContactSegmentMembership, *Response, error) {
	return fetchData[ContactSegmentMembership](ctx, s.client, "contacts/"+url.PathEscape(id)+"/segments", opts)
}

// Notes returns a page of the contact's CRM notes.
func (s *ContactService) Notes(ctx context.Context, id string, params *ListOptions, opts ...RequestOption) (*Page[ContactNote], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[ContactNote](ctx, s.client, "contacts/"+url.PathEscape(id)+"/notes", q, opts...)
}

// AddNote appends a CRM note to the contact.
func (s *ContactService) AddNote(ctx context.Context, id, content string, opts ...RequestOption) (*ContactNote, *Response, error) {
	body := struct {
		Content string `json:"content"`
	}{Content: content}
	return send[ContactNote](ctx, s.client.post, "contacts/"+url.PathEscape(id)+"/notes", body, opts)
}

// UpdateNote rewrites a CRM note.
func (s *ContactService) UpdateNote(ctx context.Context, id, noteID, content string, opts ...RequestOption) (*ContactNote, *Response, error) {
	body := struct {
		Content *string `json:"content,omitempty"`
	}{Content: &content}
	return send[ContactNote](ctx, s.client.patch, "contacts/"+url.PathEscape(id)+"/notes/"+url.PathEscape(noteID), body, opts)
}

// DeleteNote removes a CRM note.
func (s *ContactService) DeleteNote(ctx context.Context, id, noteID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "contacts/"+url.PathEscape(id)+"/notes/"+url.PathEscape(noteID), opts...)
}

// Activities returns a page of the contact's recorded CRM activities.
func (s *ContactService) Activities(ctx context.Context, id string, params *ListOptions, opts ...RequestOption) (*Page[ContactActivity], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[ContactActivity](ctx, s.client, "contacts/"+url.PathEscape(id)+"/activities", q, opts...)
}

// Deals returns the CRM deals attached to the contact.
func (s *ContactService) Deals(ctx context.Context, id string, opts ...RequestOption) ([]Deal, *Response, error) {
	return fetchData[Deal](ctx, s.client, "contacts/"+url.PathEscape(id)+"/deals", opts)
}

// Export streams the organization's contacts to w in the requested format. The
// response's X-Total-Rows header carries the row count.
func (s *ContactService) Export(ctx context.Context, params *ContactExportParams, w io.Writer, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "contacts/export", params, w, opts...)
}

// ImportPreview parses an uploaded CSV or XLSX and returns the detected columns
// and a suggested mapping, without writing anything.
func (s *ContactService) ImportPreview(ctx context.Context, file *FileUpload, opts ...RequestOption) (*ContactImportPreview, *Response, error) {
	out := new(ContactImportPreview)
	resp, err := s.client.postMultipart(ctx, "contacts/import/preview", "file", file, nil, out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// ImportCommit imports the file using the given mapping and options. Pass the
// same file that was used for [ContactService.ImportPreview]. Imports are
// capped at 50,000 rows, and new addresses are queued for verification right
// away.
//
// An import is a bulk arrival: it never raises contact.created, however few
// rows it has, so one upload cannot flood the organization's automations and
// webhooks.
func (s *ContactService) ImportCommit(ctx context.Context, file *FileUpload, params *ContactImportParams, opts ...RequestOption) (*ContactImportResult, *Response, error) {
	encoded, err := json.Marshal(params)
	if err != nil {
		return nil, nil, fmt.Errorf("warmbly: encode import options: %w", err)
	}
	out := new(ContactImportResult)
	resp, err := s.client.postMultipart(ctx, "contacts/import/commit", "file", file, map[string]string{"options": string(encoded)}, out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// Research runs AI research against a single contact, in the request. It spends
// AI credits.
func (s *ContactService) Research(ctx context.Context, id, objective string, opts ...RequestOption) (*ResearchRun, *Response, error) {
	body := struct {
		Objective string `json:"objective,omitempty"`
	}{Objective: objective}
	return send[ResearchRun](ctx, s.client.post, "contacts/"+url.PathEscape(id)+"/research", body, opts)
}

// ListResearch returns the contact's most recent research runs, newest first. A
// limit of 0 uses the server default of 20; the cap is 100.
func (s *ContactService) ListResearch(ctx context.Context, id string, limit int, opts ...RequestOption) ([]ResearchRun, *Response, error) {
	q := make(url.Values)
	setPositive(q, "limit", limit)
	return fetchData[ResearchRun](ctx, s.client, withQuery("contacts/"+url.PathEscape(id)+"/research", q), opts)
}

// BatchResearch queues AI research for many contacts and returns how many runs
// were enqueued. They drain in the background; poll
// [ContactService.ListResearch] or subscribe to the gateway for progress.
func (s *ContactService) BatchResearch(ctx context.Context, contactIDs []string, objective string, opts ...RequestOption) (int, *Response, error) {
	body := struct {
		ContactIDs []string `json:"contact_ids"`
		Objective  string   `json:"objective,omitempty"`
	}{ContactIDs: contactIDs, Objective: objective}
	var out struct {
		Queued int `json:"queued"`
	}
	resp, err := s.client.post(ctx, "contacts/research/batch", body, &out, opts...)
	if err != nil {
		return 0, resp, err
	}
	return out.Queued, resp, nil
}
