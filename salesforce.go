package warmbly

import (
	"context"
	"encoding/json"
	"net/url"
	"time"
)

// SalesforceService manages the native Salesforce sync of a connected org:
// what is logged and written back, what an email matches, saved list-view and
// Campaign imports, the activity log, and the Salesforce panel of a contact.
//
// Connect a Salesforce org first with [IntegrationService.StartOAuth] (pass
// the login environment through [OAuthStartParams]). Every method here takes
// the connection id, and every call answers 404 for a connection that is not
// a Salesforce one, and 503 on an instance without Salesforce sync.
//
// All methods take the INTEGRATIONS scope. The contact panel reads under
// READ_CRM; syncing and unlinking a contact take INTEGRATIONS.
type SalesforceService service

// Salesforce objects the sync reads and writes.
const (
	SalesforceObjectLead    = "Lead"
	SalesforceObjectContact = "Contact"
)

// Import source kinds for [SalesforceImportSourceParams.SourceKind].
const (
	// SalesforceSourceListView reads a saved Lead or Contact list view.
	SalesforceSourceListView = "list_view"
	// SalesforceSourceCampaign reads the members of a Salesforce Campaign.
	SalesforceSourceCampaign = "campaign"
)

// Activity statuses returned in [SalesforceActivity.Status] and accepted by
// [SalesforceActivityParams.Status].
const (
	SalesforceActivityPending = "pending"
	SalesforceActivitySynced  = "synced"
	SalesforceActivitySkipped = "skipped"
	SalesforceActivityFailed  = "failed"
)

// Import source states returned in the Status of a Salesforce, Apollo import
// source.
const (
	ImportSourceIdle    = "idle"
	ImportSourceRunning = "running"
	ImportSourceError   = "error"
)

// SalesforceMatchingSettings decides which record an email is, and what
// happens when it is none.
type SalesforceMatchingSettings struct {
	// Prefer is which object wins when an email is both: "contact" or "lead".
	Prefer string `json:"prefer"`
	// CreateWhen is when a missing person is created: "never", "reply" (on a
	// reply or a meeting) or "send" (on the first logged activity).
	CreateWhen string `json:"create_when"`
	// CreateAs is "lead" or "contact".
	CreateAs string `json:"create_as"`
	// LeadSource stamps created records; empty leaves Salesforce's default.
	LeadSource string `json:"lead_source"`
	// LeadStatus is the status a created Lead starts in; empty is the default.
	LeadStatus string `json:"lead_status"`
	// Owner of created records: "connected_user", "sender" or "fixed". Fixed
	// needs OwnerID.
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
	// opportunity, else on the account.
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

// Field directions and conflict policies for [SalesforceFieldRule].
const (
	SalesforceDirectionPush = "push"
	SalesforceDirectionPull = "pull"
	SalesforceDirectionBoth = "both"

	// SalesforcePolicyOverwrite writes the value whenever it changes.
	SalesforcePolicyOverwrite = "overwrite"
	// SalesforcePolicyIfEmpty only fills a field blank on the receiving side.
	SalesforcePolicyIfEmpty = "if_empty"
)

// SalesforceFieldRule syncs one Warmbly field with one Salesforce field.
type SalesforceFieldRule struct {
	// Object is [SalesforceObjectLead] or [SalesforceObjectContact].
	Object string `json:"object"`
	// Warmbly is a field key from [SalesforceSettingsResult.WarmblyFields], or
	// custom:<key> for a contact custom field. Engagement fields only push.
	Warmbly string `json:"warmbly"`
	// Salesforce is the field's API name. A related field such as
	// Account.Name can only be pulled.
	Salesforce string `json:"salesforce"`
	// Direction is one of the SalesforceDirection* constants.
	Direction string `json:"direction"`
	// Policy is one of the SalesforcePolicy* constants.
	Policy string `json:"policy"`
}

// SalesforceSettings is one connection's sync configuration. Updating it
// replaces the whole document.
type SalesforceSettings struct {
	// Enabled is the master switch for logging, writeback and the pull loop.
	// Imports and the contact panel work regardless.
	Enabled   bool                        `json:"enabled"`
	Matching  SalesforceMatchingSettings  `json:"matching"`
	Activity  SalesforceActivitySettings  `json:"activity"`
	Writeback SalesforceWritebackSettings `json:"writeback"`
	Inbound   SalesforceInboundSettings   `json:"inbound"`
	// FieldMap is per-object field sync, in both directions.
	FieldMap []SalesforceFieldRule `json:"field_map"`
	// DailyAPIBudget caps the calls Warmbly makes per day. Zero means a fifth
	// of the org's daily allocation.
	DailyAPIBudget int `json:"daily_api_budget"`
}

// SalesforceFieldDef names a field of the Warmbly side of a field rule.
type SalesforceFieldDef struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// SalesforceSettingsResult is the settings with the vocabulary and defaults an
// editor needs.
type SalesforceSettingsResult struct {
	Settings SalesforceSettings `json:"settings"`
	// WarmblyFields are the Warmbly fields a rule may name.
	WarmblyFields []SalesforceFieldDef `json:"warmbly_fields"`
	// Defaults is what a new connection starts with.
	Defaults SalesforceSettings `json:"defaults"`
}

// SalesforceCheck is one permission or setup probe of the overview.
type SalesforceCheck struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// SalesforceOrg names the connected org.
type SalesforceOrg struct {
	ID          string `json:"id,omitempty"`
	InstanceURL string `json:"instance_url"`
	// Environment is "production" or "sandbox".
	Environment string `json:"environment"`
	LoginHost   string `json:"login_host"`
	UserID      string `json:"user_id,omitempty"`
	Account     string `json:"account,omitempty"`
}

// SalesforceAPIUsage is the API budget picture.
type SalesforceAPIUsage struct {
	Used       int `json:"used"`
	Max        int `json:"max"`
	CallsToday int `json:"calls_today"`
	Budget     int `json:"budget"`
}

// SalesforceActivityCounts summarizes the activity outbox.
type SalesforceActivityCounts struct {
	Pending    int `json:"pending"`
	Synced24h  int `json:"synced_24h"`
	Failed     int `json:"failed"`
	Skipped24h int `json:"skipped_24h"`
	// LinkedTotal is how many Warmbly contacts are linked to a Salesforce
	// record.
	LinkedTotal int `json:"linked_records"`
}

// SalesforceOverview is the connection's health page.
type SalesforceOverview struct {
	ConnectionID string `json:"connection_id"`
	Label        string `json:"label"`
	// Status is a Connection* constant.
	Status          string                   `json:"status"`
	Health          string                   `json:"health"`
	HealthDetail    string                   `json:"health_detail,omitempty"`
	Org             SalesforceOrg            `json:"org"`
	API             SalesforceAPIUsage       `json:"api"`
	Counts          SalesforceActivityCounts `json:"counts"`
	LastPullAt      *time.Time               `json:"last_pull_at,omitempty"`
	LastPullError   string                   `json:"last_pull_error,omitempty"`
	SettingsEnabled bool                     `json:"settings_enabled"`
	// Checks is set only when requested with [SalesforceOverviewParams].
	Checks []SalesforceCheck `json:"checks,omitempty"`
}

// SalesforceOverviewParams controls the overview.
type SalesforceOverviewParams struct {
	// Checks runs the live permission and setup probes, which cost API calls.
	Checks bool
}

// SalesforcePickOption is a picklist value.
type SalesforcePickOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// SalesforceFieldInfo is one Salesforce field as a mapping editor shows it.
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

// SalesforceMetadata is what the settings screens pick from.
type SalesforceMetadata struct {
	LeadFields    []SalesforceFieldInfo  `json:"lead_fields"`
	ContactFields []SalesforceFieldInfo  `json:"contact_fields"`
	LeadStatuses  []SalesforcePickOption `json:"lead_statuses"`
	LeadSources   []SalesforcePickOption `json:"lead_sources"`
}

// SalesforceUser is a Salesforce user for owner pickers.
type SalesforceUser struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// SalesforceListView is a list view for the import picker.
type SalesforceListView struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Object is [SalesforceObjectLead] or [SalesforceObjectContact].
	Object string `json:"object"`
}

// SalesforceCampaign is a Salesforce Campaign for the import picker.
type SalesforceCampaign struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Type        string `json:"type"`
	MemberCount int    `json:"member_count"`
}

// SalesforceImportRef names what an import reads.
type SalesforceImportRef struct {
	// SourceKind is [SalesforceSourceListView] or [SalesforceSourceCampaign].
	SourceKind string `json:"source_kind"`
	// Object is [SalesforceObjectLead] or [SalesforceObjectContact] for a
	// list view; ignored for a Campaign.
	Object string `json:"object,omitempty"`
	// SourceID is the Salesforce id of the list view or Campaign.
	SourceID string `json:"source_id"`
}

// SalesforcePreviewRow is one sample row of an import preview.
type SalesforcePreviewRow struct {
	RecordID      string `json:"record_id"`
	Object        string `json:"object"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	Company       string `json:"company"`
	Title         string `json:"title"`
	OwnerName     string `json:"owner_name"`
	Status        string `json:"status"`
	AlreadyLinked bool   `json:"already_linked"`
}

// SalesforceImportPreview is the first rows of a source and how many it holds.
type SalesforceImportPreview struct {
	Total  int                    `json:"total"`
	Sample []SalesforcePreviewRow `json:"sample"`
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
	// SourceKind is a SalesforceSource* constant.
	SourceKind  string `json:"source_kind"`
	Object      string `json:"object"`
	SourceID    string `json:"source_id"`
	SourceLabel string `json:"source_label"`
	// CampaignID is the campaign new contacts are enrolled in, if any.
	CampaignID  *string  `json:"campaign_id,omitempty"`
	CategoryIDs []string `json:"category_ids"`
	// Recurring re-reads the source every 30 minutes.
	Recurring bool `json:"recurring"`
	Enabled   bool `json:"enabled"`
	// Status is one of the ImportSource* constants.
	Status        string               `json:"status"`
	LastRunAt     *time.Time           `json:"last_run_at,omitempty"`
	LastResult    *SalesforceRunResult `json:"last_result,omitempty"`
	LastError     string               `json:"last_error,omitempty"`
	TotalImported int                  `json:"total_imported"`
	CreatedAt     time.Time            `json:"created_at"`
	UpdatedAt     time.Time            `json:"updated_at"`
}

// SalesforceImportSourceParams creates or edits an import source. On create,
// SourceKind and SourceID are required, and the first run starts at once. On
// edit, empty strings and nil pointers are left unchanged.
type SalesforceImportSourceParams struct {
	Name *string `json:"name,omitempty"`
	// SourceKind is a SalesforceSource* constant.
	SourceKind  string `json:"source_kind,omitempty"`
	Object      string `json:"object,omitempty"`
	SourceID    string `json:"source_id,omitempty"`
	SourceLabel string `json:"source_label,omitempty"`
	// CampaignID enrolls newly imported contacts in a campaign.
	CampaignID *string `json:"-"`
	// ClearCampaign removes the campaign target on edit.
	ClearCampaign bool     `json:"-"`
	CategoryIDs   []string `json:"category_ids,omitempty"`
	Recurring     *bool    `json:"recurring,omitempty"`
	Enabled       *bool    `json:"enabled,omitempty"`
}

// MarshalJSON sends campaign_id as null when ClearCampaign is set, which the
// server reads as "remove the target", and omits it when it is unset.
func (p SalesforceImportSourceParams) MarshalJSON() ([]byte, error) {
	type plain SalesforceImportSourceParams
	return marshalWithCampaign(plain(p), p.CampaignID, p.ClearCampaign)
}

// marshalWithCampaign encodes v and then adds campaign_id: the id, or null to
// clear it.
func marshalWithCampaign(v any, campaignID *string, clearIt bool) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if campaignID == nil && !clearIt {
		return raw, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if clearIt {
		m["campaign_id"] = json.RawMessage("null")
	} else {
		id, err := json.Marshal(*campaignID)
		if err != nil {
			return nil, err
		}
		m["campaign_id"] = id
	}
	return json.Marshal(m)
}

// SalesforceActivity is one queued or processed activity: a campaign event to
// log as a Salesforce Task, and what became of it.
type SalesforceActivity struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organization_id"`
	ConnectionID   string  `json:"connection_id"`
	ContactID      *string `json:"contact_id,omitempty"`
	ContactEmail   string  `json:"contact_email"`
	// Kind is "sent", "opened", "clicked", "replied", "bounced",
	// "unsubscribed" or "meeting_booked".
	Kind    string         `json:"kind"`
	Payload map[string]any `json:"payload,omitempty"`
	// Status is one of the SalesforceActivity* constants.
	Status        string     `json:"status"`
	Attempts      int        `json:"attempts"`
	NextAttemptAt time.Time  `json:"next_attempt_at"`
	RecordID      string     `json:"record_id,omitempty"`
	TaskID        string     `json:"task_id,omitempty"`
	Detail        string     `json:"detail,omitempty"`
	OccurredAt    time.Time  `json:"occurred_at"`
	CreatedAt     time.Time  `json:"created_at"`
	ProcessedAt   *time.Time `json:"processed_at,omitempty"`
}

// SalesforceActivityParams filters and paginates the activity log, newest
// first. The page limit is 1 to 200 and defaults to 50.
type SalesforceActivityParams struct {
	ListOptions
	// Status is one of the SalesforceActivity* constants.
	Status string
	// ContactID narrows to one Warmbly contact.
	ContactID string
}

func (p *SalesforceActivityParams) values() url.Values {
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

// SalesforceRef names a related Salesforce user.
type SalesforceRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SalesforceAccountRef is a Contact's account.
type SalesforceAccountRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// SalesforceOpportunity is an opportunity on the account.
type SalesforceOpportunity struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Stage     string   `json:"stage"`
	Amount    *float64 `json:"amount,omitempty"`
	CloseDate string   `json:"close_date,omitempty"`
	IsClosed  bool     `json:"is_closed"`
	IsWon     bool     `json:"is_won"`
	URL       string   `json:"url"`
}

// SalesforceTask is a recent activity on the record.
type SalesforceTask struct {
	ID          string `json:"id"`
	Subject     string `json:"subject"`
	Date        string `json:"date,omitempty"`
	Status      string `json:"status"`
	OwnerName   string `json:"owner_name,omitempty"`
	URL         string `json:"url"`
	FromWarmbly bool   `json:"from_warmbly"`
}

// SalesforcePanelSync is the outbox picture for a contact.
type SalesforcePanelSync struct {
	Pending   int    `json:"pending"`
	Failed    int    `json:"failed"`
	Synced    int    `json:"synced"`
	LastError string `json:"last_error,omitempty"`
}

// SalesforceRecord is the Lead or Contact a Warmbly contact is linked to in
// one org.
type SalesforceRecord struct {
	LinkID          string `json:"link_id"`
	ConnectionID    string `json:"connection_id"`
	ConnectionLabel string `json:"connection_label"`
	// Object is [SalesforceObjectLead] or [SalesforceObjectContact].
	Object        string                  `json:"object"`
	ID            string                  `json:"id"`
	URL           string                  `json:"url"`
	Name          string                  `json:"name"`
	Title         string                  `json:"title,omitempty"`
	Company       string                  `json:"company,omitempty"`
	Email         string                  `json:"email,omitempty"`
	Phone         string                  `json:"phone,omitempty"`
	Status        string                  `json:"status,omitempty"`
	Owner         *SalesforceRef          `json:"owner,omitempty"`
	Account       *SalesforceAccountRef   `json:"account,omitempty"`
	IsConverted   bool                    `json:"is_converted"`
	OptedOut      bool                    `json:"opted_out"`
	LeadSource    string                  `json:"lead_source,omitempty"`
	Opportunities []SalesforceOpportunity `json:"opportunities"`
	Tasks         []SalesforceTask        `json:"tasks"`
	// LinkedBy says how the link was made, for example by a match on email.
	LinkedBy     string              `json:"linked_by"`
	LastSyncedAt *time.Time          `json:"last_synced_at,omitempty"`
	LastPushedAt *time.Time          `json:"last_pushed_at,omitempty"`
	Sync         SalesforcePanelSync `json:"sync"`
	// Stale is set when the record shown is older than a live read could give.
	Stale bool   `json:"stale"`
	Error string `json:"error,omitempty"`
}

// SalesforcePanel is a contact's Salesforce context.
type SalesforcePanel struct {
	Connections []SalesforcePanelConnection `json:"connections"`
	Records     []SalesforceRecord          `json:"records"`
	// CanSync is set when the contact can be synced to a connected org.
	CanSync bool `json:"can_sync"`
}

// SalesforceContactSyncParams chooses where and how to sync one contact.
type SalesforceContactSyncParams struct {
	// ConnectionID picks the org when several are connected.
	ConnectionID *string `json:"connection_id,omitempty"`
	// CreateAs is "lead" or "contact": what to create when the person is not
	// found.
	CreateAs string `json:"create_as,omitempty"`
}

func (s *SalesforceService) base(connID string) string {
	return "integrations/salesforce/" + url.PathEscape(connID)
}

// Overview returns the connection's health page.
func (s *SalesforceService) Overview(ctx context.Context, connectionID string, params *SalesforceOverviewParams, opts ...RequestOption) (*SalesforceOverview, *Response, error) {
	q := make(url.Values)
	if params != nil && params.Checks {
		q.Set("checks", "1")
	}
	return fetch[SalesforceOverview](ctx, s.client, withQuery(s.base(connectionID)+"/overview", q), opts)
}

// Settings returns the sync settings with the Warmbly field vocabulary and
// the defaults.
func (s *SalesforceService) Settings(ctx context.Context, connectionID string, opts ...RequestOption) (*SalesforceSettingsResult, *Response, error) {
	return fetch[SalesforceSettingsResult](ctx, s.client, s.base(connectionID)+"/settings", opts)
}

// UpdateSettings validates and saves the whole settings document and returns
// what was stored. Safe to repeat. Needs the settings-management right as
// well as the INTEGRATIONS scope.
func (s *SalesforceService) UpdateSettings(ctx context.Context, connectionID string, settings *SalesforceSettings, opts ...RequestOption) (*SalesforceSettings, *Response, error) {
	var out struct {
		Settings SalesforceSettings `json:"settings"`
	}
	resp, err := s.client.put(ctx, s.base(connectionID)+"/settings", settings, &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return &out.Settings, resp, nil
}

// Metadata returns the Lead and Contact fields and picklists the org has.
func (s *SalesforceService) Metadata(ctx context.Context, connectionID string, opts ...RequestOption) (*SalesforceMetadata, *Response, error) {
	return fetch[SalesforceMetadata](ctx, s.client, s.base(connectionID)+"/metadata", opts)
}

// Users searches active Salesforce users by name or email, for owner
// pickers.
func (s *SalesforceService) Users(ctx context.Context, connectionID, query string, opts ...RequestOption) ([]SalesforceUser, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "q", query)
	return fetchData[SalesforceUser](ctx, s.client, withQuery(s.base(connectionID)+"/users", q), opts)
}

// ListViews lists the Lead or Contact list views the connected user can see.
// object is [SalesforceObjectLead] or [SalesforceObjectContact].
func (s *SalesforceService) ListViews(ctx context.Context, connectionID, object string, opts ...RequestOption) ([]SalesforceListView, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "object", object)
	return fetchData[SalesforceListView](ctx, s.client, withQuery(s.base(connectionID)+"/list-views", q), opts)
}

// Campaigns searches Salesforce Campaigns by name.
func (s *SalesforceService) Campaigns(ctx context.Context, connectionID, query string, opts ...RequestOption) ([]SalesforceCampaign, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "q", query)
	return fetchData[SalesforceCampaign](ctx, s.client, withQuery(s.base(connectionID)+"/campaigns", q), opts)
}

// PreviewImport reads the first rows of a list view or Campaign and how many
// it holds. It writes nothing.
func (s *SalesforceService) PreviewImport(ctx context.Context, connectionID string, ref *SalesforceImportRef, opts ...RequestOption) (*SalesforceImportPreview, *Response, error) {
	return send[SalesforceImportPreview](ctx, s.client.post, s.base(connectionID)+"/import/preview", ref, opts)
}

// ImportSources lists the connection's saved imports.
func (s *SalesforceService) ImportSources(ctx context.Context, connectionID string, opts ...RequestOption) ([]SalesforceImportSource, *Response, error) {
	return fetchData[SalesforceImportSource](ctx, s.client, s.base(connectionID)+"/import-sources", opts)
}

// CreateImportSource saves an import and starts its first run. A retried
// create makes a second source whose run imports nothing new, since people
// are deduplicated by address.
func (s *SalesforceService) CreateImportSource(ctx context.Context, connectionID string, params *SalesforceImportSourceParams, opts ...RequestOption) (*SalesforceImportSource, *Response, error) {
	return send[SalesforceImportSource](ctx, s.client.post, s.base(connectionID)+"/import-sources", params, opts)
}

// UpdateImportSource edits an import's targets and schedule.
func (s *SalesforceService) UpdateImportSource(ctx context.Context, connectionID, sourceID string, params *SalesforceImportSourceParams, opts ...RequestOption) (*SalesforceImportSource, *Response, error) {
	return send[SalesforceImportSource](ctx, s.client.patch, s.base(connectionID)+"/import-sources/"+url.PathEscape(sourceID), params, opts)
}

// RunImportSource runs an import now and returns it as running. A second call
// while one runs is refused with 409, so a retry cannot double-run it.
func (s *SalesforceService) RunImportSource(ctx context.Context, connectionID, sourceID string, opts ...RequestOption) (*SalesforceImportSource, *Response, error) {
	return send[SalesforceImportSource](ctx, s.client.post, s.base(connectionID)+"/import-sources/"+url.PathEscape(sourceID)+"/run", nil, opts)
}

// DeleteImportSource removes an import. Contacts it brought in stay.
func (s *SalesforceService) DeleteImportSource(ctx context.Context, connectionID, sourceID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, s.base(connectionID)+"/import-sources/"+url.PathEscape(sourceID), opts...)
}

// Activity returns a page of the activity log, newest first.
func (s *SalesforceService) Activity(ctx context.Context, connectionID string, params *SalesforceActivityParams, opts ...RequestOption) (*Page[SalesforceActivity], error) {
	return listJSON[SalesforceActivity](ctx, s.client, s.base(connectionID)+"/activity", params.values(), opts...)
}

// RetryActivity re-queues failed or skipped activity rows, at most 500 ids per
// call, and returns how many it re-queued. Re-queuing a pending row changes
// nothing, so it is safe to repeat.
func (s *SalesforceService) RetryActivity(ctx context.Context, connectionID string, ids []string, opts ...RequestOption) (int, *Response, error) {
	body := struct {
		IDs []string `json:"ids"`
	}{IDs: ids}
	var out struct {
		Requeued int `json:"requeued"`
	}
	resp, err := s.client.post(ctx, s.base(connectionID)+"/activity/retry", body, &out, opts...)
	if err != nil {
		return 0, resp, err
	}
	return out.Requeued, resp, nil
}

// SyncNow drains pending activity and pulls changes at once. Both are
// idempotent passes, so a retry only repeats a no-op.
func (s *SalesforceService) SyncNow(ctx context.Context, connectionID string, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, s.base(connectionID)+"/sync-now", nil, nil, opts...)
}

// --- the contact panel ---

// ContactPanel returns a contact's Salesforce context: the Lead or Contact it
// is linked to in each connected org, with opportunities and recent tasks.
//
// Requires the READ_CRM scope.
func (s *SalesforceService) ContactPanel(ctx context.Context, contactID string, opts ...RequestOption) (*SalesforcePanel, *Response, error) {
	return fetch[SalesforcePanel](ctx, s.client, "contacts/"+url.PathEscape(contactID)+"/salesforce", opts)
}

// SyncContact matches the contact in Salesforce (or creates it) and pushes its
// mapped fields, then returns the refreshed panel. Matching runs first, so a
// retry finds the record the first attempt created. params may be nil when one
// org is connected.
//
// Requires the INTEGRATIONS scope.
func (s *SalesforceService) SyncContact(ctx context.Context, contactID string, params *SalesforceContactSyncParams, opts ...RequestOption) (*SalesforcePanel, *Response, error) {
	return send[SalesforcePanel](ctx, s.client.post, "contacts/"+url.PathEscape(contactID)+"/salesforce/sync", params, opts)
}

// UnlinkContact drops a contact's link to a Salesforce record. The record
// itself is untouched.
//
// Requires the INTEGRATIONS scope.
func (s *SalesforceService) UnlinkContact(ctx context.Context, contactID, linkID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "contacts/"+url.PathEscape(contactID)+"/salesforce/links/"+url.PathEscape(linkID), opts...)
}
