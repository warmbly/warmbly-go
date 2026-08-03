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
// hydrated contact 360 view, CRM notes and activities, import and export, and
// AI-assisted research.
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

// Contact is a contact (lead) as returned by the API.
type Contact struct {
	ID string `json:"id"`

	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Company   string `json:"company"`
	Phone     string `json:"phone"`

	CustomFields map[string]string `json:"custom_fields"`

	Subscribed bool           `json:"subscribed"`
	Campaigns  []MiniCampaign `json:"campaigns"`
	Categories []MiniCategory `json:"categories"`

	// VerificationStatus is the pre-send address check: [VerifyStatusValid],
	// [VerifyStatusRisky], [VerifyStatusInvalid] or [VerifyStatusUnknown].
	// Campaigns drop invalid addresses before a worker ever sends to them.
	VerificationStatus    string     `json:"verification_status"`
	VerificationReason    string     `json:"verification_reason"`
	IsCatchAll            bool       `json:"is_catch_all"`
	VerificationCheckedAt *time.Time `json:"verification_checked_at,omitempty"`

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

// Derived lead states returned in [ContactCampaignProgress.Status].
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
	// LeadStatusUnsubscribed is terminal: the contact is suppressed.
	LeadStatusUnsubscribed = "unsubscribed"
)

// ContactCampaignProgress is a contact's aggregate state inside one campaign.
type ContactCampaignProgress struct {
	// Status is one of the LeadStatus* constants.
	Status         string     `json:"status"`
	Sent           int        `json:"sent"`
	Opened         int        `json:"opened"`
	Clicked        int        `json:"clicked"`
	Replied        int        `json:"replied"`
	Bounced        int        `json:"bounced"`
	LastActivityAt *time.Time `json:"last_activity_at,omitempty"`
	// CurrentStep labels the latest step actually sent. It is empty while the
	// status is [LeadStatusPending].
	CurrentStep string `json:"current_step,omitempty"`
}

// ContactEngagement summarizes every email touchpoint recorded for a contact.
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
	Reason string `json:"reason"`
	// Source is "bounce", "complaint" or "unsubscribe".
	Source    string     `json:"source"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// ContactDetail is the hydrated contact 360 view returned by
// [ContactService.Get]: the contact plus its engagement rollup and suppression
// state in one payload.
type ContactDetail struct {
	Contact
	Engagement  ContactEngagement   `json:"engagement"`
	Suppression *ContactSuppression `json:"suppression,omitempty"`
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
)

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

	TaskID  *string `json:"task_id,omitempty"`
	Subject *string `json:"subject,omitempty"`

	Reason   *string `json:"reason,omitempty"`
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

// ContactsCounts are organization-wide contact facet totals, returned on the
// first page of a search for the browse sidebar. They are independent of the
// search's own filters.
type ContactsCounts struct {
	Total        int                    `json:"total"`
	Subscribed   int                    `json:"subscribed"`
	Unsubscribed int                    `json:"unsubscribed"`
	InCampaign   int                    `json:"in_campaign"`
	NotContacted int                    `json:"not_contacted"`
	Categories   []ContactCategoryCount `json:"categories"`
}

// ContactCategoryCount is how many contacts carry one category.
type ContactCategoryCount struct {
	CategoryID string `json:"category_id"`
	Count      int    `json:"count"`
}

// CampaignLeadCounts are per-status lead totals within a single campaign,
// returned on the first page of a search that filters by exactly one campaign.
// They ignore the search's own lead-status filter, so every bucket shows its
// real total.
type CampaignLeadCounts struct {
	Total        int `json:"total"`
	Queued       int `json:"queued"`
	Processing   int `json:"processing"`
	Completed    int `json:"completed"`
	Replied      int `json:"replied"`
	Bounced      int `json:"bounced"`
	Unsubscribed int `json:"unsubscribed"`
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
	// LeadStatus filters by derived lead status and requires exactly one entry
	// in CampaignIDs. Use one of the LeadStatus* constants.
	LeadStatus string `json:"lead_status,omitempty"`
	// CategoryIDs matches contacts carrying every listed category.
	CategoryIDs   []string   `json:"category_ids,omitempty"`
	MinCampaigns  *int       `json:"min_campaigns,omitempty"`
	MaxCampaigns  *int       `json:"max_campaigns,omitempty"`
	Subscribed    *bool      `json:"subscribed,omitempty"`
	CreatedAfter  *time.Time `json:"created_after,omitempty"`
	CreatedBefore *time.Time `json:"created_before,omitempty"`
	UpdatedAfter  *time.Time `json:"updated_after,omitempty"`
	UpdatedBefore *time.Time `json:"updated_before,omitempty"`
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
type ContactInput struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	Company   string `json:"company,omitempty"`
	Phone     string `json:"phone,omitempty"`
	// Campaigns and Categories are ids to enrol the new contact in.
	Campaigns    []string          `json:"campaigns,omitempty"`
	Categories   []string          `json:"categories,omitempty"`
	CustomFields map[string]string `json:"custom_fields,omitempty"`
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
	// Contacts are the contact ids to edit.
	Contacts []string `json:"contacts"`

	AddCampaigns     []string           `json:"add_campaigns,omitempty"`
	RemoveCampaigns  []string           `json:"remove_campaigns,omitempty"`
	AddCategories    []string           `json:"add_categories,omitempty"`
	RemoveCategories []string           `json:"remove_categories,omitempty"`
	Fields           []ContactFieldEdit `json:"fields,omitempty"`
	// Subscribe sets the subscription flag on every listed contact.
	Subscribe *bool `json:"subscribe,omitempty"`
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
	// Filters is required for [ExportScopeFiltered].
	Filters *ContactSearchParams `json:"filters,omitempty"`
	// Fields are column identifiers in display order. Empty means the
	// recommended default set.
	Fields []string `json:"fields,omitempty"`
	// Filename is the download name without an extension. Empty falls back to
	// "contacts-<date>".
	Filename string `json:"filename,omitempty"`
}

// Where an imported column lands, for [ImportColumnMapping.Target]. A custom
// field uses the "custom:<key>" form.
const (
	ImportTargetIgnore     = "ignore"
	ImportTargetEmail      = "email"
	ImportTargetFirstName  = "first_name"
	ImportTargetLastName   = "last_name"
	ImportTargetCompany    = "company"
	ImportTargetPhone      = "phone"
	ImportTargetSubscribed = "subscribed"
	ImportTargetCategories = "categories"
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
	// CustomKey is the JSON key a "custom:" target writes to. It is split out
	// so a client can label the column without parsing Target.
	CustomKey string `json:"custom_key,omitempty"`
}

// ContactImportPreview describes an uploaded file before anything is written.
type ContactImportPreview struct {
	Filename  string `json:"filename"`
	Format    string `json:"format"`
	TotalRows int    `json:"total_rows"`
	// Columns are the detected headers. For a headerless file the server
	// synthesises "Column 1", "Column 2" and so on.
	Columns   []string `json:"columns"`
	HasHeader bool     `json:"has_header"`
	// SampleRows are the first rows verbatim.
	SampleRows [][]string `json:"sample_rows"`
	// SuggestedMapping is a heuristic default the caller may override.
	SuggestedMapping []ImportColumnMapping `json:"suggested_mapping"`
}

// ContactImportParams is the configuration committed alongside the file.
type ContactImportParams struct {
	Mapping []ImportColumnMapping `json:"mapping"`
	// Dedup is one of the ImportDedup* constants.
	Dedup     string `json:"dedup,omitempty"`
	HasHeader bool   `json:"has_header"`
	// CategoryIDs and CampaignIDs are applied to every imported contact.
	CategoryIDs []string `json:"category_ids,omitempty"`
	CampaignIDs []string `json:"campaign_ids,omitempty"`
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

// ContactImportResult summarizes a committed import.
type ContactImportResult struct {
	Total     int              `json:"total"`
	Imported  int              `json:"imported"`
	Updated   int              `json:"updated"`
	Skipped   int              `json:"skipped"`
	Failed    int              `json:"failed"`
	StartedAt time.Time        `json:"started_at"`
	EndedAt   time.Time        `json:"ended_at"`
	Errors    []ImportRowError `json:"errors,omitempty"`
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

// Create adds contacts to the organization and returns the created records.
func (s *ContactService) Create(ctx context.Context, contacts []ContactInput, opts ...RequestOption) ([]Contact, *Response, error) {
	return sendSlice[Contact](ctx, s.client, s.client.post, "contacts", contacts, opts)
}

// BulkUpdate edits many contacts at once and returns the updated records.
func (s *ContactService) BulkUpdate(ctx context.Context, params *ContactBulkUpdateParams, opts ...RequestOption) ([]Contact, *Response, error) {
	return sendSlice[Contact](ctx, s.client, s.client.patch, "contacts", params, opts)
}

// BulkDelete permanently removes the given contacts.
func (s *ContactService) BulkDelete(ctx context.Context, ids []string, opts ...RequestOption) (*Response, error) {
	return s.client.deleteBody(ctx, "contacts", ids, nil, opts...)
}

// Get retrieves the hydrated contact 360 view.
func (s *ContactService) Get(ctx context.Context, id string, opts ...RequestOption) (*ContactDetail, *Response, error) {
	return fetch[ContactDetail](ctx, s.client, "contacts/"+url.PathEscape(id), opts)
}

// Update modifies a single contact.
func (s *ContactService) Update(ctx context.Context, id string, params *ContactUpdateParams, opts ...RequestOption) (*Contact, *Response, error) {
	return send[Contact](ctx, s.client, s.client.patch, "contacts/"+url.PathEscape(id), params, opts)
}

// Delete permanently removes a contact.
func (s *ContactService) Delete(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "contacts/"+url.PathEscape(id), opts...)
}

// Lookup resolves an email address to a contact. The contact is nil when no
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

// Emails returns the emails sent to a contact.
func (s *ContactService) Emails(ctx context.Context, id string, opts ...RequestOption) ([]ContactSentEmail, *Response, error) {
	return fetchData[ContactSentEmail](ctx, s.client, "contacts/"+url.PathEscape(id)+"/emails", opts)
}

// Timeline returns the contact's merged activity feed.
func (s *ContactService) Timeline(ctx context.Context, id string, opts ...RequestOption) ([]TimelineEvent, *Response, error) {
	return fetchData[TimelineEvent](ctx, s.client, "contacts/"+url.PathEscape(id)+"/timeline", opts)
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
	return send[ContactNote](ctx, s.client, s.client.post, "contacts/"+url.PathEscape(id)+"/notes", body, opts)
}

// UpdateNote rewrites a CRM note.
func (s *ContactService) UpdateNote(ctx context.Context, id, noteID, content string, opts ...RequestOption) (*ContactNote, *Response, error) {
	body := struct {
		Content *string `json:"content,omitempty"`
	}{Content: &content}
	return send[ContactNote](ctx, s.client, s.client.patch, "contacts/"+url.PathEscape(id)+"/notes/"+url.PathEscape(noteID), body, opts)
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

// Export streams the organization's contacts to w in the requested format.
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
// same file that was used for [ContactService.ImportPreview].
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
	return send[ResearchRun](ctx, s.client, s.client.post, "contacts/"+url.PathEscape(id)+"/research", body, opts)
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
