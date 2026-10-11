package warmbly

import (
	"context"
	"encoding/json"
	"net/url"
	"time"
)

// SalesforceService runs the native Salesforce sync on a connected org: which
// campaign activity is logged as Salesforce Tasks, which Lead and Contact
// fields are written back, list view and Campaign imports, and each contact's
// Salesforce panel.
//
// Every connection-scoped method takes the id of an [IntegrationConnection]
// whose provider is [ProviderSalesforce]; connect one with
// [IntegrationService.StartOAuthWithParams]. On a deployment without the sync
// every call answers 503.
type SalesforceService service

// Salesforce objects named by the sync.
const (
	SalesforceObjectLead           = "Lead"
	SalesforceObjectContact        = "Contact"
	SalesforceObjectCampaignMember = "CampaignMember"
)

// Salesforce import source kinds for [SalesforceImportSource.SourceKind].
const (
	SalesforceSourceListView = "list_view"
	SalesforceSourceCampaign = "campaign"
)

// Salesforce import source states returned in [SalesforceImportSource.Status].
const (
	SalesforceSourceIdle    = "idle"
	SalesforceSourceRunning = "running"
	SalesforceSourceError   = "error"
)

// Salesforce activity states returned in [SalesforceActivity.Status] and
// accepted by [SalesforceActivityListParams.Status].
const (
	SalesforceActivityPending = "pending"
	SalesforceActivitySynced  = "synced"
	SalesforceActivitySkipped = "skipped"
	SalesforceActivityFailed  = "failed"
)

// Salesforce activity kinds returned in [SalesforceActivity.Kind].
const (
	SalesforceKindSent          = "sent"
	SalesforceKindOpened        = "opened"
	SalesforceKindClicked       = "clicked"
	SalesforceKindReplied       = "replied"
	SalesforceKindBounced       = "bounced"
	SalesforceKindUnsubscribed  = "unsubscribed"
	SalesforceKindMeetingBooked = "meeting_booked"
)

// Field rule directions and conflict policies for [SalesforceFieldRule].
const (
	SalesforceDirectionPush = "push"
	SalesforceDirectionPull = "pull"
	SalesforceDirectionBoth = "both"

	// SalesforcePolicyOverwrite writes the value whenever it changes.
	SalesforcePolicyOverwrite = "overwrite"
	// SalesforcePolicyIfEmpty only fills a field that is blank on the
	// receiving side.
	SalesforcePolicyIfEmpty = "if_empty"
)

// SalesforceMatchingSettings decides which record an email is, and what
// happens when it is none.
type SalesforceMatchingSettings struct {
	// Prefer is which object wins when an email is both: "contact" or "lead".
	Prefer string `json:"prefer"`
	// CreateWhen is when a missing person is created: "never", "reply" (on a
	// reply or a meeting) or "send" (on the first logged activity).
	CreateWhen string `json:"create_when"`
	// CreateAs is "lead" or "contact". A created Contact needs no Account.
	CreateAs string `json:"create_as"`
	// LeadSource stamps created records; empty leaves Salesforce's default.
	LeadSource string `json:"lead_source"`
	// LeadStatus is the status a created Lead starts in; empty is the default.
	LeadStatus string `json:"lead_status"`
	// Owner of created records: "connected_user", "sender" (the Salesforce
	// user with the sending mailbox's address) or "fixed", which needs
	// OwnerID.
	Owner   string `json:"owner"`
	OwnerID string `json:"owner_id,omitempty"`
	// RunAssignmentRules lets the org's lead assignment rules pick the owner.
	RunAssignmentRules bool `json:"run_assignment_rules"`
}

// SalesforceActivitySettings decides which campaign events become Tasks.
type SalesforceActivitySettings struct {
	Sent          bool `json:"sent"`
	Replied       bool `json:"replied"`
	Opened        bool `json:"opened"`
	Clicked       bool `json:"clicked"`
	Bounced       bool `json:"bounced"`
	Unsubscribed  bool `json:"unsubscribed"`
	MeetingBooked bool `json:"meeting_booked"`
	// IncludeBody writes the email or reply text into the Task description.
	IncludeBody bool `json:"include_body"`
	// AssignTo owns the Task: "record_owner", "sender" or "connected_user".
	AssignTo string `json:"assign_to"`
	// RelateToOpportunity puts a Contact's Tasks on their account's open
	// opportunity, else the account.
	RelateToOpportunity bool `json:"relate_to_opportunity"`
}

// SalesforceWritebackSettings decides which Lead and Contact fields Warmbly
// changes.
type SalesforceWritebackSettings struct {
	// LeadStatusOnSent is the status a Lead moves to when first emailed.
	LeadStatusOnSent string `json:"lead_status_on_sent"`
	// LeadStatusOnReply maps a reply intent ("any", "positive", "negative",
	// "neutral", "question" or "out_of_office") to a Lead status.
	LeadStatusOnReply map[string]string `json:"lead_status_on_reply"`
	// LeadStatusOnMeeting is the status a booked meeting moves a Lead to.
	LeadStatusOnMeeting string `json:"lead_status_on_meeting"`
	// NeverMoveBackwards keeps a Lead that is further along the status
	// picklist where it is.
	NeverMoveBackwards bool `json:"never_move_backwards"`
}

// SalesforceInboundSettings decides what a change in Salesforce does in
// Warmbly.
type SalesforceInboundSettings struct {
	// OptOut keeps do-not-email in step: "both", "to_salesforce",
	// "from_salesforce" or "off".
	OptOut string `json:"opt_out"`
	// PauseOnConverted holds a Lead's outreach once it is converted.
	PauseOnConverted bool `json:"pause_on_converted"`
	// PauseOnStatuses holds a Lead's outreach once it reaches one of these.
	PauseOnStatuses []string `json:"pause_on_statuses"`
	// PauseOnOpenOpportunity holds a Contact whose account has an open
	// opportunity.
	PauseOnOpenOpportunity bool `json:"pause_on_open_opportunity"`
}

// SalesforceFieldRule syncs one Warmbly field with one Salesforce field.
type SalesforceFieldRule struct {
	// Object is [SalesforceObjectLead] or [SalesforceObjectContact].
	Object string `json:"object"`
	// Warmbly is a [SalesforceSettingsResult.WarmblyFields] key or
	// custom:<key>. Engagement fields only push.
	Warmbly string `json:"warmbly"`
	// Salesforce is the field's API name. A related field such as
	// Account.Name can only be pulled.
	Salesforce string `json:"salesforce"`
	// Direction is one of the SalesforceDirection* constants.
	Direction string `json:"direction"`
	// Policy is [SalesforcePolicyOverwrite] or [SalesforcePolicyIfEmpty].
	Policy string `json:"policy"`
}

// SalesforceSettings is one connection's sync configuration. It is saved
// whole: read it, change what you need, and pass it back to
// [SalesforceService.UpdateSettings].
type SalesforceSettings struct {
	// Enabled is the master switch for activity logging, writeback and the
	// pull loop. Imports and the contact panel work regardless.
	Enabled   bool                        `json:"enabled"`
	Matching  SalesforceMatchingSettings  `json:"matching"`
	Activity  SalesforceActivitySettings  `json:"activity"`
	Writeback SalesforceWritebackSettings `json:"writeback"`
	Inbound   SalesforceInboundSettings   `json:"inbound"`
	// FieldMap is per-object field sync, in both directions. At most 100
	// rules.
	FieldMap []SalesforceFieldRule `json:"field_map"`
	// DailyAPIBudget caps the calls Warmbly makes per day; 0 means a fifth of
	// the org's daily allocation.
	DailyAPIBudget int `json:"daily_api_budget"`
}

// SalesforceWarmblyField is one Warmbly field a rule may name.
type SalesforceWarmblyField struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// SalesforceSettingsResult is a connection's settings with the vocabulary to
// edit them.
type SalesforceSettingsResult struct {
	Settings SalesforceSettings `json:"settings"`
	// WarmblyFields are the Warmbly fields a rule may name besides
	// custom:<key>.
	WarmblyFields []SalesforceWarmblyField `json:"warmbly_fields"`
	// Defaults is what a new connection starts with.
	Defaults SalesforceSettings `json:"defaults"`
}

// SalesforceCheck is one permission or setup probe.
type SalesforceCheck struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	OK    bool   `json:"ok"`
	// Detail explains a failed or degraded check in words an admin can act on.
	Detail string `json:"detail,omitempty"`
}

// SalesforceOrg names the connected org.
type SalesforceOrg struct {
	ID          string `json:"id,omitempty"`
	InstanceURL string `json:"instance_url"`
	// Environment is [SalesforceProduction] or [SalesforceSandbox].
	Environment string `json:"environment"`
	LoginHost   string `json:"login_host"`
	UserID      string `json:"user_id,omitempty"`
	Account     string `json:"account,omitempty"`
}

// SalesforceAPIUsage is the org's API budget picture.
type SalesforceAPIUsage struct {
	Used       int `json:"used"`
	Max        int `json:"max"`
	CallsToday int `json:"calls_today"`
	Budget     int `json:"budget"`
}

// SalesforceActivityCounts summarizes the activity queue.
type SalesforceActivityCounts struct {
	Pending       int `json:"pending"`
	Synced24h     int `json:"synced_24h"`
	Failed        int `json:"failed"`
	Skipped24h    int `json:"skipped_24h"`
	LinkedRecords int `json:"linked_records"`
}

// SalesforceOverview is a connection's health page.
type SalesforceOverview struct {
	ConnectionID string `json:"connection_id"`
	Label        string `json:"label"`
	// Status is one of the Connection* constants.
	Status string `json:"status"`
	// Health is "unknown", "healthy", "degraded" or "down".
	Health          string                   `json:"health"`
	HealthDetail    string                   `json:"health_detail,omitempty"`
	Org             SalesforceOrg            `json:"org"`
	API             SalesforceAPIUsage       `json:"api"`
	Counts          SalesforceActivityCounts `json:"counts"`
	LastPullAt      *time.Time               `json:"last_pull_at,omitempty"`
	LastPullError   string                   `json:"last_pull_error,omitempty"`
	SettingsEnabled bool                     `json:"settings_enabled"`
	// Checks is set when the overview was asked to probe permissions.
	Checks []SalesforceCheck `json:"checks,omitempty"`
}

// SalesforceOverviewParams tunes [SalesforceService.Overview].
type SalesforceOverviewParams struct {
	// Checks also probes what the connected user can do. It costs a few
	// Salesforce API calls.
	Checks bool
}

func (p *SalesforceOverviewParams) values() url.Values {
	q := make(url.Values)
	if p != nil && p.Checks {
		q.Set("checks", "1")
	}
	return q
}

// SalesforcePickOption is a picklist value.
type SalesforcePickOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// SalesforceFieldInfo is one Lead or Contact field.
type SalesforceFieldInfo struct {
	Name       string                 `json:"name"`
	Label      string                 `json:"label"`
	Type       string                 `json:"type"`
	Createable bool                   `json:"createable"`
	Updateable bool                   `json:"updateable"`
	Calculated bool                   `json:"calculated"`
	Custom     bool                   `json:"custom"`
	Picklist   []SalesforcePickOption `json:"picklist,omitempty"`
}

// SalesforceMetadata is the field and picklist catalog of a connected org.
type SalesforceMetadata struct {
	LeadFields    []SalesforceFieldInfo  `json:"lead_fields"`
	ContactFields []SalesforceFieldInfo  `json:"contact_fields"`
	LeadStatuses  []SalesforcePickOption `json:"lead_statuses"`
	LeadSources   []SalesforcePickOption `json:"lead_sources"`
}

// SalesforceUser is an active Salesforce user, for owner pickers.
type SalesforceUser struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// SalesforceListView is a Lead or Contact list view an import can read.
type SalesforceListView struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Object string `json:"object"`
}

// SalesforceCampaign is a Salesforce Campaign an import can read.
type SalesforceCampaign struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Type        string `json:"type"`
	MemberCount int    `json:"member_count"`
}

// SalesforceSearchParams narrows a Salesforce user or Campaign search. At most
// 50 results come back.
type SalesforceSearchParams struct {
	// Query matches the name, and for users the email too.
	Query string
}

func (p *SalesforceSearchParams) values() url.Values {
	q := make(url.Values)
	if p != nil {
		setNonEmpty(q, "q", p.Query)
	}
	return q
}

// SalesforceImportSourceRef names what an import reads: a list view of
// Object, or a Campaign.
type SalesforceImportSourceRef struct {
	// SourceKind is [SalesforceSourceListView] or [SalesforceSourceCampaign].
	SourceKind string `json:"source_kind"`
	// Object is [SalesforceObjectLead] or [SalesforceObjectContact] for a list
	// view. A Campaign reads its members, whatever is sent.
	Object string `json:"object,omitempty"`
	// SourceID is the list view or Campaign id.
	SourceID string `json:"source_id"`
}

// SalesforceImportPreviewRow is one sample row of a preview.
type SalesforceImportPreviewRow struct {
	RecordID  string `json:"record_id"`
	Object    string `json:"object"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Company   string `json:"company"`
	Title     string `json:"title"`
	OwnerName string `json:"owner_name"`
	Status    string `json:"status"`
	// AlreadyLinked reports a record already linked to a Warmbly contact.
	AlreadyLinked bool `json:"already_linked"`
}

// SalesforceImportPreview is the first rows of a source and how many it holds.
type SalesforceImportPreview struct {
	Total  int                          `json:"total"`
	Sample []SalesforceImportPreviewRow `json:"sample"`
}

// SalesforceRunResult is what one import run did.
type SalesforceRunResult struct {
	Read     int `json:"read"`
	Imported int `json:"imported"`
	Updated  int `json:"updated"`
	Linked   int `json:"linked"`
	Skipped  int `json:"skipped"`
	Failed   int `json:"failed"`
	// NoEmail counts records with no address, which cannot be contacts.
	NoEmail int `json:"no_email"`
	// OptedOut counts people with Email Opt Out set, suppressed rather than
	// imported.
	OptedOut int `json:"opted_out"`
	// Truncated is set when the source held more than one run reads.
	Truncated bool `json:"truncated,omitempty"`
}

// SalesforceImportSource is a saved list view or Campaign that feeds contacts
// into Warmbly.
type SalesforceImportSource struct {
	ID              string  `json:"id"`
	OrganizationID  string  `json:"organization_id"`
	ConnectionID    string  `json:"connection_id"`
	CreatedByUserID *string `json:"created_by_user_id,omitempty"`
	Name            string  `json:"name"`
	// SourceKind is [SalesforceSourceListView] or [SalesforceSourceCampaign].
	SourceKind string `json:"source_kind"`
	// Object is one of the SalesforceObject* constants.
	Object      string `json:"object"`
	SourceID    string `json:"source_id"`
	SourceLabel string `json:"source_label"`
	// CampaignID is the Warmbly campaign imported people are enrolled in.
	CampaignID *string `json:"campaign_id,omitempty"`
	// CategoryIDs tag imported people.
	CategoryIDs []string `json:"category_ids"`
	// Recurring re-reads the source every 30 minutes.
	Recurring bool `json:"recurring"`
	Enabled   bool `json:"enabled"`
	// Status is one of the SalesforceSource* state constants.
	Status        string               `json:"status"`
	LastRunAt     *time.Time           `json:"last_run_at,omitempty"`
	LastResult    *SalesforceRunResult `json:"last_result,omitempty"`
	LastError     string               `json:"last_error,omitempty"`
	TotalImported int                  `json:"total_imported"`
	CreatedAt     time.Time            `json:"created_at"`
	UpdatedAt     time.Time            `json:"updated_at"`
}

// SalesforceImportSourceCreateParams saves an import. SourceKind, SourceID and,
// for a list view, Object are required.
type SalesforceImportSourceCreateParams struct {
	SalesforceImportSourceRef
	// Name defaults to SourceLabel.
	Name *string `json:"name,omitempty"`
	// SourceLabel is the list view or Campaign name, for display.
	SourceLabel string `json:"source_label,omitempty"`
	// CampaignID enrolls imported people in a Warmbly campaign.
	CampaignID *string `json:"campaign_id,omitempty"`
	// CategoryIDs tag imported people; at most 50.
	CategoryIDs []string `json:"category_ids,omitempty"`
	// Recurring re-reads the source every 30 minutes.
	Recurring *bool `json:"recurring,omitempty"`
}

// SalesforceImportSourceUpdateParams edits an import's targets and schedule.
// Nil fields are left unchanged.
type SalesforceImportSourceUpdateParams struct {
	Name *string
	// CampaignID retargets the enrollment campaign. ClearCampaign removes it
	// instead, and wins over CampaignID.
	CampaignID    *string
	ClearCampaign bool
	// CategoryIDs replaces the tags; a non-nil empty slice clears them.
	CategoryIDs []string
	Recurring   *bool
	Enabled     *bool
}

// MarshalJSON sends campaign_id as null when ClearCampaign is set, which is
// how the API tells clearing the campaign from leaving it alone.
func (p SalesforceImportSourceUpdateParams) MarshalJSON() ([]byte, error) {
	out := map[string]any{}
	if p.Name != nil {
		out["name"] = *p.Name
	}
	switch {
	case p.ClearCampaign:
		out["campaign_id"] = nil
	case p.CampaignID != nil:
		out["campaign_id"] = *p.CampaignID
	}
	if p.CategoryIDs != nil {
		out["category_ids"] = p.CategoryIDs
	}
	if p.Recurring != nil {
		out["recurring"] = *p.Recurring
	}
	if p.Enabled != nil {
		out["enabled"] = *p.Enabled
	}
	return json.Marshal(out)
}

// SalesforceActivity is one queued or processed campaign event to log as a
// Salesforce Task, and what became of it.
type SalesforceActivity struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organization_id"`
	ConnectionID   string  `json:"connection_id"`
	ContactID      *string `json:"contact_id,omitempty"`
	ContactEmail   string  `json:"contact_email"`
	// Kind is one of the SalesforceKind* constants.
	Kind    string         `json:"kind"`
	Payload map[string]any `json:"payload,omitempty"`
	// Status is one of the SalesforceActivity* constants.
	Status        string    `json:"status"`
	Attempts      int       `json:"attempts"`
	NextAttemptAt time.Time `json:"next_attempt_at"`
	// RecordID is the Lead or Contact the Task went on, and TaskID the Task.
	RecordID string `json:"record_id,omitempty"`
	TaskID   string `json:"task_id,omitempty"`
	// Detail says why a row was skipped or failed.
	Detail      string     `json:"detail,omitempty"`
	OccurredAt  time.Time  `json:"occurred_at"`
	CreatedAt   time.Time  `json:"created_at"`
	ProcessedAt *time.Time `json:"processed_at,omitempty"`
}

// SalesforceActivityListParams filters and paginates the activity log, newest
// first. Limit is between 1 and 200 and defaults to 50.
type SalesforceActivityListParams struct {
	ListOptions
	// Status is one of the SalesforceActivity* constants.
	Status string
	// ContactID narrows to one Warmbly contact.
	ContactID string
}

func (p *SalesforceActivityListParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	setNonEmpty(q, "status", p.Status)
	setNonEmpty(q, "contact_id", p.ContactID)
	return q
}

// SalesforcePanelConnection is one connected org in a contact's panel.
type SalesforcePanelConnection struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Environment string `json:"environment"`
	InstanceURL string `json:"instance_url"`
}

// SalesforcePanelRef names a related Salesforce user.
type SalesforcePanelRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SalesforcePanelAccount is a Contact's account.
type SalesforcePanelAccount struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// SalesforcePanelOpportunity is an opportunity on the account, or on the
// converted Lead's account.
type SalesforcePanelOpportunity struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Stage  string   `json:"stage"`
	Amount *float64 `json:"amount,omitempty"`
	// CloseDate is Salesforce's date, as YYYY-MM-DD.
	CloseDate string `json:"close_date,omitempty"`
	IsClosed  bool   `json:"is_closed"`
	IsWon     bool   `json:"is_won"`
	URL       string `json:"url"`
}

// SalesforcePanelTask is a recent activity on the record.
type SalesforcePanelTask struct {
	ID        string `json:"id"`
	Subject   string `json:"subject"`
	Date      string `json:"date,omitempty"`
	Status    string `json:"status"`
	OwnerName string `json:"owner_name,omitempty"`
	URL       string `json:"url"`
	// FromWarmbly marks a Task the sync logged.
	FromWarmbly bool `json:"from_warmbly"`
}

// SalesforcePanelSync is the activity queue picture for one record.
type SalesforcePanelSync struct {
	Pending   int    `json:"pending"`
	Failed    int    `json:"failed"`
	Synced    int    `json:"synced"`
	LastError string `json:"last_error,omitempty"`
}

// SalesforcePanelRecord is the Lead or Contact a contact is linked to in one
// org.
type SalesforcePanelRecord struct {
	// LinkID identifies the link, for [SalesforceService.UnlinkContact].
	LinkID          string `json:"link_id"`
	ConnectionID    string `json:"connection_id"`
	ConnectionLabel string `json:"connection_label"`
	// Object is [SalesforceObjectLead] or [SalesforceObjectContact].
	Object        string                       `json:"object"`
	ID            string                       `json:"id"`
	URL           string                       `json:"url"`
	Name          string                       `json:"name"`
	Title         string                       `json:"title,omitempty"`
	Company       string                       `json:"company,omitempty"`
	Email         string                       `json:"email,omitempty"`
	Phone         string                       `json:"phone,omitempty"`
	Status        string                       `json:"status,omitempty"`
	Owner         *SalesforcePanelRef          `json:"owner,omitempty"`
	Account       *SalesforcePanelAccount      `json:"account,omitempty"`
	IsConverted   bool                         `json:"is_converted"`
	OptedOut      bool                         `json:"opted_out"`
	LeadSource    string                       `json:"lead_source,omitempty"`
	Opportunities []SalesforcePanelOpportunity `json:"opportunities"`
	Tasks         []SalesforcePanelTask        `json:"tasks"`
	// LinkedBy is how the link was made: "match", "created", "import" or
	// "manual".
	LinkedBy     string              `json:"linked_by"`
	LastSyncedAt *time.Time          `json:"last_synced_at,omitempty"`
	LastPushedAt *time.Time          `json:"last_pushed_at,omitempty"`
	Sync         SalesforcePanelSync `json:"sync"`
	// Stale is set when the live read failed and the stored copy is shown.
	Stale bool   `json:"stale"`
	Error string `json:"error,omitempty"`
}

// SalesforceContactPanel is a contact's Salesforce context across every
// connected org.
type SalesforceContactPanel struct {
	Connections []SalesforcePanelConnection `json:"connections"`
	Records     []SalesforcePanelRecord     `json:"records"`
	// CanSync reports whether a sync can run for this contact.
	CanSync bool `json:"can_sync"`
}

// SalesforceContactSyncParams tunes [SalesforceService.SyncContact].
type SalesforceContactSyncParams struct {
	// ConnectionID limits the sync to one org; nil syncs every connected org.
	ConnectionID *string `json:"connection_id,omitempty"`
	// CreateAs creates a missing person as "lead" or "contact". Empty only
	// matches.
	CreateAs string `json:"create_as,omitempty"`
}

func salesforcePath(connID string) string {
	return "integrations/salesforce/" + url.PathEscape(connID)
}

// Overview returns a connection's health page: the org, its API budget, the
// activity queue and the last pull.
//
// Requires the INTEGRATIONS API permission.
func (s *SalesforceService) Overview(ctx context.Context, connID string, params *SalesforceOverviewParams, opts ...RequestOption) (*SalesforceOverview, *Response, error) {
	return fetch[SalesforceOverview](ctx, s.client, withQuery(salesforcePath(connID)+"/overview", params.values()), opts)
}

// Settings returns a connection's sync settings, the defaults and the Warmbly
// field list.
//
// Requires the INTEGRATIONS API permission.
func (s *SalesforceService) Settings(ctx context.Context, connID string, opts ...RequestOption) (*SalesforceSettingsResult, *Response, error) {
	return fetch[SalesforceSettingsResult](ctx, s.client, salesforcePath(connID)+"/settings", opts)
}

// UpdateSettings validates and saves a connection's settings as a whole and
// returns what was stored. Invalid settings are refused with
// [ErrCodeInvalidSalesforceSettings].
//
// Requires the INTEGRATIONS API permission.
func (s *SalesforceService) UpdateSettings(ctx context.Context, connID string, settings *SalesforceSettings, opts ...RequestOption) (*SalesforceSettings, *Response, error) {
	var out struct {
		Settings *SalesforceSettings `json:"settings"`
	}
	resp, err := s.client.put(ctx, salesforcePath(connID)+"/settings", settings, &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Settings, resp, nil
}

// Metadata returns the org's Lead and Contact fields and picklists.
//
// Requires the INTEGRATIONS API permission.
func (s *SalesforceService) Metadata(ctx context.Context, connID string, opts ...RequestOption) (*SalesforceMetadata, *Response, error) {
	return fetch[SalesforceMetadata](ctx, s.client, salesforcePath(connID)+"/metadata", opts)
}

// Users searches the org's active users.
//
// Requires the INTEGRATIONS API permission.
func (s *SalesforceService) Users(ctx context.Context, connID string, params *SalesforceSearchParams, opts ...RequestOption) ([]SalesforceUser, *Response, error) {
	return fetchData[SalesforceUser](ctx, s.client, withQuery(salesforcePath(connID)+"/users", params.values()), opts)
}

// ListViews lists the list views of object, [SalesforceObjectLead] or
// [SalesforceObjectContact].
//
// Requires the INTEGRATIONS API permission.
func (s *SalesforceService) ListViews(ctx context.Context, connID, object string, opts ...RequestOption) ([]SalesforceListView, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "object", object)
	return fetchData[SalesforceListView](ctx, s.client, withQuery(salesforcePath(connID)+"/list-views", q), opts)
}

// Campaigns searches the org's Salesforce Campaigns, active ones first.
//
// Requires the INTEGRATIONS API permission.
func (s *SalesforceService) Campaigns(ctx context.Context, connID string, params *SalesforceSearchParams, opts ...RequestOption) ([]SalesforceCampaign, *Response, error) {
	return fetchData[SalesforceCampaign](ctx, s.client, withQuery(salesforcePath(connID)+"/campaigns", params.values()), opts)
}

// PreviewImport reads the first 25 rows of a list view or Campaign without
// importing anything.
//
// Requires the INTEGRATIONS API permission.
func (s *SalesforceService) PreviewImport(ctx context.Context, connID string, source *SalesforceImportSourceRef, opts ...RequestOption) (*SalesforceImportPreview, *Response, error) {
	return send[SalesforceImportPreview](ctx, s.client.post, salesforcePath(connID)+"/import/preview", source, opts)
}

// ImportSources lists a connection's saved imports.
//
// Requires the INTEGRATIONS API permission.
func (s *SalesforceService) ImportSources(ctx context.Context, connID string, opts ...RequestOption) ([]SalesforceImportSource, *Response, error) {
	return fetchData[SalesforceImportSource](ctx, s.client, salesforcePath(connID)+"/import-sources", opts)
}

// CreateImportSource saves an import and starts its first run, returning the
// source with Status [SalesforceSourceRunning]. People are deduplicated by
// address, so a retried create imports nothing new.
//
// Requires the INTEGRATIONS API permission.
func (s *SalesforceService) CreateImportSource(ctx context.Context, connID string, params *SalesforceImportSourceCreateParams, opts ...RequestOption) (*SalesforceImportSource, *Response, error) {
	return send[SalesforceImportSource](ctx, s.client.post, salesforcePath(connID)+"/import-sources", params, opts)
}

// UpdateImportSource edits an import's targets and schedule.
//
// Requires the INTEGRATIONS API permission.
func (s *SalesforceService) UpdateImportSource(ctx context.Context, connID, sourceID string, params *SalesforceImportSourceUpdateParams, opts ...RequestOption) (*SalesforceImportSource, *Response, error) {
	if params == nil {
		params = &SalesforceImportSourceUpdateParams{}
	}
	return send[SalesforceImportSource](ctx, s.client.patch, salesforcePath(connID)+"/import-sources/"+url.PathEscape(sourceID), params, opts)
}

// RunImportSource runs an import now, in the background. A second run while
// one is going is refused with [ErrCodeImportRunning], so a retry cannot
// double-run it.
//
// Requires the INTEGRATIONS API permission.
func (s *SalesforceService) RunImportSource(ctx context.Context, connID, sourceID string, opts ...RequestOption) (*SalesforceImportSource, *Response, error) {
	return send[SalesforceImportSource](ctx, s.client.post, salesforcePath(connID)+"/import-sources/"+url.PathEscape(sourceID)+"/run", nil, opts)
}

// DeleteImportSource removes an import. The contacts it brought in stay.
//
// Requires the INTEGRATIONS API permission.
func (s *SalesforceService) DeleteImportSource(ctx context.Context, connID, sourceID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, salesforcePath(connID)+"/import-sources/"+url.PathEscape(sourceID), opts...)
}

// Activity returns a page of the activity log, newest first.
//
// Requires the INTEGRATIONS API permission.
func (s *SalesforceService) Activity(ctx context.Context, connID string, params *SalesforceActivityListParams, opts ...RequestOption) (*Page[SalesforceActivity], error) {
	return listJSON[SalesforceActivity](ctx, s.client, salesforcePath(connID)+"/activity", params.values(), opts...)
}

// RetryActivity requeues the given failed or skipped rows, or every failed
// row when ids is empty, and returns how many were requeued. At most 500 ids
// per call; requeueing a pending row changes nothing.
//
// Requires the INTEGRATIONS API permission.
func (s *SalesforceService) RetryActivity(ctx context.Context, connID string, ids []string, opts ...RequestOption) (int, *Response, error) {
	if ids == nil {
		ids = []string{}
	}
	body := struct {
		IDs []string `json:"ids"`
	}{IDs: ids}
	var out struct {
		Requeued int `json:"requeued"`
	}
	resp, err := s.client.post(ctx, salesforcePath(connID)+"/activity/retry", body, &out, opts...)
	if err != nil {
		return 0, resp, err
	}
	return out.Requeued, resp, nil
}

// SyncNow drains pending activity and pulls changes from Salesforce at once,
// in the background. It is refused with [ErrCodeSalesforceSyncOff] while the
// sync is switched off.
//
// Requires the INTEGRATIONS API permission.
func (s *SalesforceService) SyncNow(ctx context.Context, connID string, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, salesforcePath(connID)+"/sync-now", nil, nil, opts...)
}

// --- contact panel ---

func contactSalesforcePath(contactID string) string {
	return "contacts/" + url.PathEscape(contactID) + "/salesforce"
}

// ContactPanel returns a contact's Salesforce panel, linking it on the way
// when its address matches a record nobody has linked yet.
//
// Requires the READ_CRM API permission.
func (s *SalesforceService) ContactPanel(ctx context.Context, contactID string, opts ...RequestOption) (*SalesforceContactPanel, *Response, error) {
	return fetch[SalesforceContactPanel](ctx, s.client, contactSalesforcePath(contactID), opts)
}

// SyncContact matches the contact in Salesforce, or creates it when
// CreateAs says so, pushes its mapped fields and returns the refreshed panel.
// Matching runs first, so a retry finds the record the first attempt created.
//
// Requires the INTEGRATIONS API permission.
func (s *SalesforceService) SyncContact(ctx context.Context, contactID string, params *SalesforceContactSyncParams, opts ...RequestOption) (*SalesforceContactPanel, *Response, error) {
	if params == nil {
		params = &SalesforceContactSyncParams{}
	}
	return send[SalesforceContactPanel](ctx, s.client.post, contactSalesforcePath(contactID)+"/sync", params, opts)
}

// UnlinkContact drops a contact's link to a Salesforce record. Neither record
// is deleted.
//
// Requires the INTEGRATIONS API permission.
func (s *SalesforceService) UnlinkContact(ctx context.Context, contactID, linkID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, contactSalesforcePath(contactID)+"/links/"+url.PathEscape(linkID), opts...)
}
