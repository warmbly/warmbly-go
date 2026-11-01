package warmbly

import (
	"context"
	"net/url"
	"time"
)

// The Salesforce methods take the id of a Salesforce connection (see
// [IntegrationService.Connections]). Every one needs [PermIntegrations] for an
// API key. A session member needs the settings permission to change settings
// and import sources, and the integrations permission to read, preview, run an
// import or sync. The Salesforce connection itself is made with
// [IntegrationService.StartOAuthWith], which is session-only.

// Salesforce import source kinds for [SalesforceImportSourceParams.SourceKind].
const (
	SalesforceSourceListView = "list_view"
	SalesforceSourceCampaign = "campaign"
)

// Salesforce objects the sync reads and writes.
const (
	SalesforceObjectLead    = "Lead"
	SalesforceObjectContact = "Contact"
)

// Salesforce activity states returned in [SalesforceActivity.Status].
const (
	SalesforceActivityPending = "pending"
	SalesforceActivitySynced  = "synced"
	SalesforceActivitySkipped = "skipped"
	SalesforceActivityFailed  = "failed"
)

// Salesforce import source run states returned in
// [SalesforceImportSource.Status].
const (
	SalesforceSourceIdle    = "idle"
	SalesforceSourceRunning = "running"
	SalesforceSourceError   = "error"
)

// SalesforceOverviewCheck is one permission or setup probe.
type SalesforceOverviewCheck struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// SalesforceOrg names the connected Salesforce org.
type SalesforceOrg struct {
	ID          string `json:"id,omitempty"`
	InstanceURL string `json:"instance_url"`
	// Environment is "production" or "sandbox".
	Environment string `json:"environment"`
	LoginHost   string `json:"login_host"`
	UserID      string `json:"user_id,omitempty"`
	Account     string `json:"account,omitempty"`
}

// SalesforceAPIUsage is the API call budget picture.
type SalesforceAPIUsage struct {
	Used       int `json:"used"`
	Max        int `json:"max"`
	CallsToday int `json:"calls_today"`
	// Budget is the daily cap on calls Warmbly makes.
	Budget int `json:"budget"`
}

// SalesforceActivityCounts summarizes the activity outbox.
type SalesforceActivityCounts struct {
	Pending     int `json:"pending"`
	Synced24h   int `json:"synced_24h"`
	Failed      int `json:"failed"`
	Skipped24h  int `json:"skipped_24h"`
	LinkedTotal int `json:"linked_records"`
}

// SalesforceOverview is a Salesforce connection's health page.
type SalesforceOverview struct {
	ConnectionID string `json:"connection_id"`
	Label        string `json:"label"`
	// Status is one of the Connection* constants and Health is "unknown",
	// "healthy", "degraded" or "down".
	Status          string                   `json:"status"`
	Health          string                   `json:"health"`
	HealthDetail    string                   `json:"health_detail,omitempty"`
	Org             SalesforceOrg            `json:"org"`
	API             SalesforceAPIUsage       `json:"api"`
	Counts          SalesforceActivityCounts `json:"counts"`
	LastPullAt      *time.Time               `json:"last_pull_at,omitempty"`
	LastPullError   string                   `json:"last_pull_error,omitempty"`
	SettingsEnabled bool                     `json:"settings_enabled"`
	// Checks is filled only when the call asked for them.
	Checks []SalesforceOverviewCheck `json:"checks,omitempty"`
}

// SalesforceMatching decides which record an email is, and what happens when it
// is none.
type SalesforceMatching struct {
	// Prefer is which object wins when an email is both: "contact" or "lead".
	Prefer string `json:"prefer"`
	// CreateWhen is "never", "reply" (on a reply or a meeting) or "send" (on
	// the first logged activity).
	CreateWhen string `json:"create_when"`
	// CreateAs is "lead" or "contact".
	CreateAs string `json:"create_as"`
	// LeadSource stamps created records; empty leaves Salesforce's default.
	LeadSource string `json:"lead_source"`
	// LeadStatus is the status a created Lead starts in; empty is the default.
	LeadStatus string `json:"lead_status"`
	// Owner of created records: "connected_user", "sender" or "fixed". Fixed
	// takes OwnerID.
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
	// RelateToOpportunity puts a Contact's Tasks on the account's open
	// opportunity, else on the account.
	RelateToOpportunity bool `json:"relate_to_opportunity"`
}

// SalesforceWriteback decides which Lead fields Warmbly changes.
type SalesforceWriteback struct {
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

// SalesforceInbound decides what a change in Salesforce does in Warmbly.
type SalesforceInbound struct {
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

// Salesforce field sync directions and conflict policies for
// [SalesforceFieldRule].
const (
	SalesforcePolicyOverwrite = "overwrite"
	SalesforcePolicyIfEmpty   = "if_empty"
)

// SalesforceFieldRule syncs one Warmbly field with one Salesforce field.
type SalesforceFieldRule struct {
	// Object is [SalesforceObjectLead] or [SalesforceObjectContact].
	Object string `json:"object"`
	// Warmbly names a Warmbly field: see [SalesforceSettingsView.WarmblyFields],
	// or "custom:<key>" for a contact custom field.
	Warmbly    string `json:"warmbly"`
	Salesforce string `json:"salesforce"`
	// Direction is [SyncPush], [SyncPull] or [SyncBoth].
	Direction string `json:"direction"`
	// Policy is [SalesforcePolicyOverwrite] or [SalesforcePolicyIfEmpty].
	Policy string `json:"policy"`
}

// SalesforceSettings is one connection's sync configuration. It is saved
// whole, so read it first and send back every part you do not mean to change.
type SalesforceSettings struct {
	// Enabled is the master switch for logging, writeback and the pull loop.
	// Imports and the contact panel work regardless.
	Enabled   bool                       `json:"enabled"`
	Matching  SalesforceMatching         `json:"matching"`
	Activity  SalesforceActivitySettings `json:"activity"`
	Writeback SalesforceWriteback        `json:"writeback"`
	Inbound   SalesforceInbound          `json:"inbound"`
	// FieldMap is per-object field sync, in both directions.
	FieldMap []SalesforceFieldRule `json:"field_map"`
	// DailyAPIBudget caps the calls Warmbly makes per day; 0 means a fifth of
	// the org's daily allocation.
	DailyAPIBudget int `json:"daily_api_budget"`
}

// IntegrationFieldDef is a named field a mapping may use.
type IntegrationFieldDef struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// SalesforceSettingsView is a connection's settings with the vocabulary to edit
// them.
type SalesforceSettingsView struct {
	Settings SalesforceSettings `json:"settings"`
	// WarmblyFields are the fields a rule may name on the Warmbly side.
	WarmblyFields []IntegrationFieldDef `json:"warmbly_fields"`
	// Defaults is what a new connection starts with.
	Defaults SalesforceSettings `json:"defaults"`
}

// SalesforcePicklistValue is one picklist entry.
type SalesforcePicklistValue struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// SalesforceField is one Salesforce field as the mapping editor shows it.
type SalesforceField struct {
	Name       string                    `json:"name"`
	Label      string                    `json:"label"`
	Type       string                    `json:"type"`
	Createable bool                      `json:"createable"`
	Updateable bool                      `json:"updateable"`
	Calculated bool                      `json:"calculated"`
	Custom     bool                      `json:"custom"`
	Picklist   []SalesforcePicklistValue `json:"picklist,omitempty"`
}

// SalesforceMetadata is the catalog of fields and picklists the settings
// screens pick from.
type SalesforceMetadata struct {
	LeadFields    []SalesforceField         `json:"lead_fields"`
	ContactFields []SalesforceField         `json:"contact_fields"`
	LeadStatuses  []SalesforcePicklistValue `json:"lead_statuses"`
	LeadSources   []SalesforcePicklistValue `json:"lead_sources"`
}

// SalesforceUser is an active Salesforce user, for owner pickers.
type SalesforceUser struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// SalesforceListView is a Lead or Contact list view for the import picker.
type SalesforceListView struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
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

// SalesforceImportSourceRef names what an import reads.
type SalesforceImportSourceRef struct {
	// SourceKind is [SalesforceSourceListView] or [SalesforceSourceCampaign].
	SourceKind string `json:"source_kind"`
	// Object is [SalesforceObjectLead] or [SalesforceObjectContact] for a list
	// view. A Campaign import ignores it.
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

// SalesforceImportPreview is the first rows of an import source and how many it
// holds.
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
	// Truncated reports that the source held more than one run reads.
	Truncated bool `json:"truncated,omitempty"`
}

// SalesforceImportSource is a saved list view or Salesforce Campaign that
// feeds contacts into Warmbly.
type SalesforceImportSource struct {
	ID              string  `json:"id"`
	OrganizationID  string  `json:"organization_id"`
	ConnectionID    string  `json:"connection_id"`
	CreatedByUserID *string `json:"created_by_user_id,omitempty"`
	Name            string  `json:"name"`
	// SourceKind is one of the SalesforceSource* kinds.
	SourceKind  string `json:"source_kind"`
	Object      string `json:"object"`
	SourceID    string `json:"source_id"`
	SourceLabel string `json:"source_label"`
	// CampaignID is the Warmbly campaign imported contacts are enrolled in.
	CampaignID *string `json:"campaign_id,omitempty"`
	// CategoryIDs are the tags applied to imported contacts.
	CategoryIDs []string `json:"category_ids"`
	// Recurring keeps the source in sync on a schedule.
	Recurring bool `json:"recurring"`
	Enabled   bool `json:"enabled"`
	// Status is one of the SalesforceSource* run states.
	Status        string               `json:"status"`
	LastRunAt     *time.Time           `json:"last_run_at,omitempty"`
	LastResult    *SalesforceRunResult `json:"last_result,omitempty"`
	LastError     string               `json:"last_error,omitempty"`
	TotalImported int                  `json:"total_imported"`
	CreatedAt     time.Time            `json:"created_at"`
	UpdatedAt     time.Time            `json:"updated_at"`
}

// SalesforceImportSourceParams saves an import source. Saving one starts its
// first run.
type SalesforceImportSourceParams struct {
	// Name defaults to SourceLabel.
	Name *string `json:"name,omitempty"`
	// SourceKind is [SalesforceSourceListView] or [SalesforceSourceCampaign].
	SourceKind string `json:"source_kind"`
	// Object is [SalesforceObjectLead] or [SalesforceObjectContact] for a list
	// view. A Campaign import ignores it.
	Object      string `json:"object,omitempty"`
	SourceID    string `json:"source_id"`
	SourceLabel string `json:"source_label,omitempty"`
	// CampaignID enrolls imported contacts in a Warmbly campaign.
	CampaignID *string `json:"campaign_id,omitempty"`
	// CategoryIDs tags imported contacts. At most 50.
	CategoryIDs []string `json:"category_ids,omitempty"`
	Recurring   *bool    `json:"recurring,omitempty"`
	Enabled     *bool    `json:"enabled,omitempty"`
}

// SalesforceImportSourceUpdate edits an import source's targets and schedule.
// What it reads cannot be changed. Nil fields are left unchanged.
type SalesforceImportSourceUpdate struct {
	Name *string
	// CampaignID enrolls imported contacts in a Warmbly campaign. Set
	// ClearCampaign instead to stop enrolling.
	CampaignID    *string
	ClearCampaign bool
	// CategoryIDs replaces the tags applied to imported contacts. A nil slice
	// leaves them; an empty, non-nil slice removes them all. At most 50.
	CategoryIDs []string
	Recurring   *bool
	Enabled     *bool
}

func (u *SalesforceImportSourceUpdate) body() map[string]any {
	body := map[string]any{}
	if u == nil {
		return body
	}
	if u.Name != nil {
		body["name"] = *u.Name
	}
	switch {
	case u.ClearCampaign:
		body["campaign_id"] = nil
	case u.CampaignID != nil:
		body["campaign_id"] = *u.CampaignID
	}
	if u.CategoryIDs != nil {
		body["category_ids"] = u.CategoryIDs
	}
	if u.Recurring != nil {
		body["recurring"] = *u.Recurring
	}
	if u.Enabled != nil {
		body["enabled"] = *u.Enabled
	}
	return body
}

// SalesforceActivity is one queued or processed activity: a campaign event to
// log as a Task, and what became of it.
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

// SalesforceActivityParams filters and paginates the activity log, which is
// newest first.
type SalesforceActivityParams struct {
	// ListOptions limit is 1 to 200; the server default is 50.
	ListOptions
	// Status is one of the SalesforceActivity* constants.
	Status string
	// ContactID narrows to one contact.
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

// SalesforceOverview returns a connection's health page. Pass checks to also
// probe the permissions the sync needs, which costs Salesforce API calls.
func (s *IntegrationService) SalesforceOverview(ctx context.Context, id string, checks bool, opts ...RequestOption) (*SalesforceOverview, *Response, error) {
	q := make(url.Values)
	if checks {
		q.Set("checks", "1")
	}
	return fetch[SalesforceOverview](ctx, s.client, withQuery("integrations/salesforce/"+url.PathEscape(id)+"/overview", q), opts)
}

// SalesforceSettings returns a connection's sync settings, the Warmbly field
// vocabulary and the defaults a new connection starts with.
func (s *IntegrationService) SalesforceSettings(ctx context.Context, id string, opts ...RequestOption) (*SalesforceSettingsView, *Response, error) {
	return fetch[SalesforceSettingsView](ctx, s.client, "integrations/salesforce/"+url.PathEscape(id)+"/settings", opts)
}

// UpdateSalesforceSettings saves a connection's sync settings and returns what
// was stored. It replaces the whole document, so a retry converges.
func (s *IntegrationService) UpdateSalesforceSettings(ctx context.Context, id string, settings *SalesforceSettings, opts ...RequestOption) (*SalesforceSettings, *Response, error) {
	var out struct {
		Settings *SalesforceSettings `json:"settings"`
	}
	resp, err := s.client.put(ctx, "integrations/salesforce/"+url.PathEscape(id)+"/settings", settings, &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Settings, resp, nil
}

// SalesforceMetadata returns the Lead and Contact fields and the picklists a
// connection's settings choose from.
func (s *IntegrationService) SalesforceMetadata(ctx context.Context, id string, opts ...RequestOption) (*SalesforceMetadata, *Response, error) {
	return fetch[SalesforceMetadata](ctx, s.client, "integrations/salesforce/"+url.PathEscape(id)+"/metadata", opts)
}

// SalesforceUsers searches the org's active Salesforce users by name or email.
// An empty query returns the first 50.
func (s *IntegrationService) SalesforceUsers(ctx context.Context, id, query string, opts ...RequestOption) ([]SalesforceUser, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "q", query)
	return fetchData[SalesforceUser](ctx, s.client, withQuery("integrations/salesforce/"+url.PathEscape(id)+"/users", q), opts)
}

// SalesforceListViews lists the list views of a Salesforce object, which is
// [SalesforceObjectLead] or [SalesforceObjectContact].
func (s *IntegrationService) SalesforceListViews(ctx context.Context, id, object string, opts ...RequestOption) ([]SalesforceListView, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "object", object)
	return fetchData[SalesforceListView](ctx, s.client, withQuery("integrations/salesforce/"+url.PathEscape(id)+"/list-views", q), opts)
}

// SalesforceCampaigns searches the org's Salesforce Campaigns by name.
func (s *IntegrationService) SalesforceCampaigns(ctx context.Context, id, query string, opts ...RequestOption) ([]SalesforceCampaign, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "q", query)
	return fetchData[SalesforceCampaign](ctx, s.client, withQuery("integrations/salesforce/"+url.PathEscape(id)+"/campaigns", q), opts)
}

// PreviewSalesforceImport reads the first rows of a list view or Campaign
// without importing anything.
func (s *IntegrationService) PreviewSalesforceImport(ctx context.Context, id string, source *SalesforceImportSourceRef, opts ...RequestOption) (*SalesforceImportPreview, *Response, error) {
	return send[SalesforceImportPreview](ctx, s.client.post, "integrations/salesforce/"+url.PathEscape(id)+"/import/preview", source, opts)
}

// SalesforceImportSources lists a connection's saved imports.
func (s *IntegrationService) SalesforceImportSources(ctx context.Context, id string, opts ...RequestOption) ([]SalesforceImportSource, *Response, error) {
	return fetchData[SalesforceImportSource](ctx, s.client, "integrations/salesforce/"+url.PathEscape(id)+"/import-sources", opts)
}

// CreateSalesforceImportSource saves an import and starts its first run. A
// retried create makes a second source whose run imports nothing new, since
// people are deduplicated by address.
func (s *IntegrationService) CreateSalesforceImportSource(ctx context.Context, id string, params *SalesforceImportSourceParams, opts ...RequestOption) (*SalesforceImportSource, *Response, error) {
	return send[SalesforceImportSource](ctx, s.client.post, "integrations/salesforce/"+url.PathEscape(id)+"/import-sources", params, opts)
}

// UpdateSalesforceImportSource edits an import's targets and schedule.
func (s *IntegrationService) UpdateSalesforceImportSource(ctx context.Context, id, sourceID string, params *SalesforceImportSourceUpdate, opts ...RequestOption) (*SalesforceImportSource, *Response, error) {
	return send[SalesforceImportSource](ctx, s.client.patch, "integrations/salesforce/"+url.PathEscape(id)+"/import-sources/"+url.PathEscape(sourceID), params.body(), opts)
}

// RunSalesforceImportSource starts an import now and returns the source with
// its status set to running; poll [IntegrationService.SalesforceImportSources]
// for the result. A second request while one runs answers 409.
func (s *IntegrationService) RunSalesforceImportSource(ctx context.Context, id, sourceID string, opts ...RequestOption) (*SalesforceImportSource, *Response, error) {
	return send[SalesforceImportSource](ctx, s.client.post, "integrations/salesforce/"+url.PathEscape(id)+"/import-sources/"+url.PathEscape(sourceID)+"/run", nil, opts)
}

// DeleteSalesforceImportSource removes an import. The contacts it brought in
// stay.
func (s *IntegrationService) DeleteSalesforceImportSource(ctx context.Context, id, sourceID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "integrations/salesforce/"+url.PathEscape(id)+"/import-sources/"+url.PathEscape(sourceID), opts...)
}

// SalesforceActivityLog returns a page of the activity log, newest first.
func (s *IntegrationService) SalesforceActivityLog(ctx context.Context, id string, params *SalesforceActivityParams, opts ...RequestOption) (*Page[SalesforceActivity], error) {
	return listJSON[SalesforceActivity](ctx, s.client, "integrations/salesforce/"+url.PathEscape(id)+"/activity", params.values(), opts...)
}

// RetrySalesforceActivity requeues failed or skipped activity rows and returns
// how many it requeued. At most 500 ids go in one call. Requeuing a row that is
// already pending changes nothing, so a retry is safe.
func (s *IntegrationService) RetrySalesforceActivity(ctx context.Context, id string, activityIDs []string, opts ...RequestOption) (int, *Response, error) {
	body := struct {
		IDs []string `json:"ids"`
	}{IDs: activityIDs}
	var out struct {
		Requeued int `json:"requeued"`
	}
	resp, err := s.client.post(ctx, "integrations/salesforce/"+url.PathEscape(id)+"/activity/retry", body, &out, opts...)
	if err != nil {
		return 0, resp, err
	}
	return out.Requeued, resp, nil
}

// SalesforceSyncNow drains pending activity and pulls Salesforce changes at
// once. Both are idempotent passes.
func (s *IntegrationService) SalesforceSyncNow(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "integrations/salesforce/"+url.PathEscape(id)+"/sync-now", nil, nil, opts...)
}

// --- the contact panel ---

// SalesforcePanelConnection is one connected Salesforce org.
type SalesforcePanelConnection struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Environment string `json:"environment"`
	InstanceURL string `json:"instance_url"`
}

// SalesforceRef names a related record.
type SalesforceRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SalesforceAccount is a Contact's account.
type SalesforceAccount struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// SalesforceOpportunity is an opportunity on the account, or on the converted
// Lead's.
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

// SalesforceTask is a recent activity on a record.
type SalesforceTask struct {
	ID          string `json:"id"`
	Subject     string `json:"subject"`
	Date        string `json:"date,omitempty"`
	Status      string `json:"status"`
	OwnerName   string `json:"owner_name,omitempty"`
	URL         string `json:"url"`
	FromWarmbly bool   `json:"from_warmbly"`
}

// SalesforceSyncCounts is the outbox picture for one contact.
type SalesforceSyncCounts struct {
	Pending   int    `json:"pending"`
	Failed    int    `json:"failed"`
	Synced    int    `json:"synced"`
	LastError string `json:"last_error,omitempty"`
}

// SalesforceRecord is the Lead or Contact a Warmbly contact is linked to in one
// Salesforce org.
type SalesforceRecord struct {
	LinkID          string `json:"link_id"`
	ConnectionID    string `json:"connection_id"`
	ConnectionLabel string `json:"connection_label"`
	// Object is "Lead" or "Contact".
	Object  string `json:"object"`
	ID      string `json:"id"`
	URL     string `json:"url"`
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Company string `json:"company,omitempty"`
	Email   string `json:"email,omitempty"`
	Phone   string `json:"phone,omitempty"`
	Status  string `json:"status,omitempty"`
	// Owner and Account are set when Salesforce reported them.
	Owner         *SalesforceRef          `json:"owner,omitempty"`
	Account       *SalesforceAccount      `json:"account,omitempty"`
	IsConverted   bool                    `json:"is_converted"`
	OptedOut      bool                    `json:"opted_out"`
	LeadSource    string                  `json:"lead_source,omitempty"`
	Opportunities []SalesforceOpportunity `json:"opportunities"`
	Tasks         []SalesforceTask        `json:"tasks"`
	// LinkedBy says how the link was made.
	LinkedBy     string               `json:"linked_by"`
	LastSyncedAt *time.Time           `json:"last_synced_at,omitempty"`
	LastPushedAt *time.Time           `json:"last_pushed_at,omitempty"`
	Sync         SalesforceSyncCounts `json:"sync"`
	// Stale reports that the live read failed or was skipped and the values
	// shown come from the last stored snapshot.
	Stale bool   `json:"stale"`
	Error string `json:"error,omitempty"`
}

// SalesforcePanel is a contact's Salesforce context.
type SalesforcePanel struct {
	// Connections are the connected Salesforce orgs.
	Connections []SalesforcePanelConnection `json:"connections"`
	// Records are the linked records, one per org at most.
	Records []SalesforceRecord `json:"records"`
	// CanSync reports whether the caller may push this contact now.
	CanSync bool `json:"can_sync"`
}

// SalesforceContactSyncParams chooses how a contact is matched or created in
// Salesforce.
type SalesforceContactSyncParams struct {
	// ConnectionID limits the sync to one Salesforce connection. Empty syncs
	// every connected org.
	ConnectionID string `json:"connection_id,omitempty"`
	// CreateAs is "lead" or "contact" and decides what a missing person is
	// created as.
	CreateAs string `json:"create_as,omitempty"`
}

// Salesforce returns the contact's linked Salesforce records, with opportunities
// and recent tasks. It needs [PermReadCRM].
func (s *ContactService) Salesforce(ctx context.Context, contactID string, opts ...RequestOption) (*SalesforcePanel, *Response, error) {
	return fetch[SalesforcePanel](ctx, s.client, "contacts/"+url.PathEscape(contactID)+"/salesforce", opts)
}

// SyncSalesforce matches the contact in Salesforce, creating it when missing,
// pushes its mapped fields and returns the refreshed panel. Matching runs
// first, so a retry finds the record the first attempt created. params may be
// nil. It needs [PermIntegrations].
func (s *ContactService) SyncSalesforce(ctx context.Context, contactID string, params *SalesforceContactSyncParams, opts ...RequestOption) (*SalesforcePanel, *Response, error) {
	return send[SalesforcePanel](ctx, s.client.post, "contacts/"+url.PathEscape(contactID)+"/salesforce/sync", params, opts)
}

// UnlinkSalesforce drops the contact's link to a Salesforce record. It needs
// [PermIntegrations].
func (s *ContactService) UnlinkSalesforce(ctx context.Context, contactID, linkID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "contacts/"+url.PathEscape(contactID)+"/salesforce/links/"+url.PathEscape(linkID), opts...)
}
