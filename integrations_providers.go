package warmbly

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"time"
)

// This file holds the provider-specific panels behind
// /integrations/{salesforce,apollo,clay,slack,community}: the pages a
// workspace uses after a provider is connected through [IntegrationService].
// Every method hangs off [IntegrationService].
//
// The connection-scoped routes (Salesforce, Apollo, Clay) are open to the
// integrations scope, manage-settings or use-integrations for reads, need the
// integrations scope with manage-settings for writes, and the integrations
// scope with use-integrations for runs ("operate"). They answer 503 when the
// provider is not wired on the instance and 404 for a connection that is not
// the provider's.

// --- community directory ---

// Community app listing states returned in [CommunityApp.Status].
const (
	// CommunityPublished is reachable by its link, and listed in discovery
	// once enough workspaces use it.
	CommunityPublished = "published"
	// CommunityFeatured was picked by an operator and is always listed.
	CommunityFeatured = "featured"
)

// CommunityApp is a third-party app's page in the community directory, as a
// workspace browsing it sees it. No credential, redirect or webhook field is
// ever included.
type CommunityApp struct {
	ApplicationID string `json:"application_id"`
	// Slug addresses the app in [IntegrationService.CommunityApp].
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Tagline     string `json:"tagline"`
	Description string `json:"description"`
	// Category is a catalog category, "ai" or "other".
	Category   string `json:"category"`
	LogoURL    string `json:"logo_url"`
	WebsiteURL string `json:"website_url"`
	InstallURL string `json:"install_url"`
	SupportURL string `json:"support_url"`
	PrivacyURL string `json:"privacy_url"`
	Developer  string `json:"developer"`
	// Scopes is the API scope bitmask the app asks for and Permissions spells
	// it out.
	Scopes      uint64       `json:"scopes"`
	Permissions []Permission `json:"permissions"`
	// Status is [CommunityPublished] or [CommunityFeatured]; unknown values
	// may appear on newer servers.
	Status string `json:"status"`
	// Listed reports whether the app shows in discovery, because it is
	// featured or widely used.
	Listed    bool `json:"listed"`
	Installs  int  `json:"installs"`
	Installed bool `json:"installed"`
	// PublishedAt is when the listing was published.
	PublishedAt time.Time `json:"published_at"`
}

// CommunityApps returns a page of the apps shown in discovery, featured first
// and then most installed. The server caps Limit at 200 and defaults it to
// 100. Requires the same access as [IntegrationService.Catalog].
func (s *IntegrationService) CommunityApps(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[CommunityApp], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[CommunityApp](ctx, s.client, "integrations/community", q, opts...)
}

// CommunityApp opens a published listing by its slug, listed in discovery or
// not.
func (s *IntegrationService) CommunityApp(ctx context.Context, slug string, opts ...RequestOption) (*CommunityApp, *Response, error) {
	return fetch[CommunityApp](ctx, s.client, "integrations/community/"+url.PathEscape(slug), opts)
}

// --- Salesforce ---

// SalesforceSettings is one connection's sync configuration.
type SalesforceSettings struct {
	// Enabled is the master switch for activity logging, writeback and the
	// pull loop. Imports and the contact panel work regardless.
	Enabled   bool                        `json:"enabled"`
	Matching  SalesforceMatchingSettings  `json:"matching"`
	Activity  SalesforceActivitySettings  `json:"activity"`
	Writeback SalesforceWritebackSettings `json:"writeback"`
	Inbound   SalesforceInboundSettings   `json:"inbound"`
	// FieldMap is per-object field sync, in both directions.
	FieldMap []SalesforceFieldRule `json:"field_map"`
	// DailyAPIBudget caps the calls Warmbly makes per day; 0 means a fifth of
	// the org's daily allocation.
	DailyAPIBudget int `json:"daily_api_budget"`
}

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
	// LeadStatus is the status a created Lead starts in.
	LeadStatus string `json:"lead_status"`
	// Owner of created records: "connected_user", "sender" or "fixed"
	// (OwnerID).
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
	// LeadStatusOnReply maps a reply intent (positive, negative, neutral,
	// question, out_of_office, any) to a Lead status.
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
	// Object is "Lead" or "Contact".
	Object string `json:"object"`
	// Warmbly is a Warmbly field key: first_name, last_name, company, phone,
	// custom:<key>, or a push-only engagement.* field.
	Warmbly    string `json:"warmbly"`
	Salesforce string `json:"salesforce"`
	// Direction is [SyncPush], [SyncPull] or [SyncBoth].
	Direction string `json:"direction"`
	// Policy is "overwrite" (write whenever it changes) or "if_empty".
	Policy string `json:"policy"`
}

// SalesforceSettingsDocument is the answer to
// [IntegrationService.SalesforceSettings]: the saved settings, the Warmbly
// field vocabulary a rule may name, and the defaults.
type SalesforceSettingsDocument struct {
	Settings      SalesforceSettings `json:"settings"`
	WarmblyFields []SalesforceField  `json:"warmbly_fields"`
	Defaults      SalesforceSettings `json:"defaults"`
}

// SalesforceField is a Warmbly-side field a rule may name.
type SalesforceField struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// SalesforceCheck is one entry in a connection's self-test.
type SalesforceCheck struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// SalesforceActivityCounts summarizes the outbox.
type SalesforceActivityCounts struct {
	Pending     int `json:"pending"`
	Synced24h   int `json:"synced_24h"`
	Failed      int `json:"failed"`
	Skipped24h  int `json:"skipped_24h"`
	LinkedTotal int `json:"linked_records"`
}

// SalesforceOverview is the connection's health page.
type SalesforceOverview struct {
	ConnectionID string `json:"connection_id"`
	Label        string `json:"label"`
	Status       string `json:"status"`
	Health       string `json:"health"`
	HealthDetail string `json:"health_detail,omitempty"`
	Org          struct {
		ID          string `json:"id,omitempty"`
		InstanceURL string `json:"instance_url"`
		// Environment is [SalesforceProduction] or [SalesforceSandbox].
		Environment string `json:"environment"`
		LoginHost   string `json:"login_host"`
		UserID      string `json:"user_id,omitempty"`
		Account     string `json:"account,omitempty"`
	} `json:"org"`
	API struct {
		Used       int `json:"used"`
		Max        int `json:"max"`
		CallsToday int `json:"calls_today"`
		Budget     int `json:"budget"`
	} `json:"api"`
	Counts          SalesforceActivityCounts `json:"counts"`
	LastPullAt      *time.Time               `json:"last_pull_at,omitempty"`
	LastPullError   string                   `json:"last_pull_error,omitempty"`
	SettingsEnabled bool                     `json:"settings_enabled"`
	// Checks is set only when the overview was requested with checks.
	Checks []SalesforceCheck `json:"checks,omitempty"`
}

// SalesforcePicklistOption is one picklist value.
type SalesforcePicklistOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// SalesforceFieldInfo describes one Lead or Contact field.
type SalesforceFieldInfo struct {
	Name       string                     `json:"name"`
	Label      string                     `json:"label"`
	Type       string                     `json:"type"`
	Createable bool                       `json:"createable"`
	Updateable bool                       `json:"updateable"`
	Calculated bool                       `json:"calculated"`
	Custom     bool                       `json:"custom"`
	Picklist   []SalesforcePicklistOption `json:"picklist,omitempty"`
}

// SalesforceMetadata is the Lead and Contact vocabulary the settings pickers
// render.
type SalesforceMetadata struct {
	LeadFields    []SalesforceFieldInfo      `json:"lead_fields"`
	ContactFields []SalesforceFieldInfo      `json:"contact_fields"`
	LeadStatuses  []SalesforcePicklistOption `json:"lead_statuses"`
	LeadSources   []SalesforcePicklistOption `json:"lead_sources"`
}

// SalesforceUser is an active Salesforce user.
type SalesforceUser struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// SalesforceListView is a Lead or Contact list view.
type SalesforceListView struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Object string `json:"object"`
}

// SalesforceCampaign is a Salesforce Campaign an import can read members of.
type SalesforceCampaign struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Type        string `json:"type"`
	MemberCount int    `json:"member_count"`
}

// Salesforce import source kinds for [SalesforceImportRef.SourceKind].
const (
	// SalesforceSourceListView reads a Lead or Contact list view.
	SalesforceSourceListView = "list_view"
	// SalesforceSourceCampaign reads a Salesforce Campaign's members.
	SalesforceSourceCampaign = "campaign"
)

// SalesforceImportRef names what to read from Salesforce.
type SalesforceImportRef struct {
	// SourceKind is [SalesforceSourceListView] or [SalesforceSourceCampaign].
	SourceKind string `json:"source_kind"`
	// Object is "Lead" or "Contact"; required for a list view.
	Object string `json:"object,omitempty"`
	// SourceID is the Salesforce id of the list view or Campaign.
	SourceID string `json:"source_id"`
}

// SalesforcePreviewRow is one sample person of an import preview.
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

// SalesforceImportPreview is the first rows of a list view or Campaign and
// how many it holds.
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

// SalesforceImportSource is a saved list view or Salesforce Campaign that
// feeds contacts into Warmbly.
type SalesforceImportSource struct {
	ID              string  `json:"id"`
	OrganizationID  string  `json:"organization_id"`
	ConnectionID    string  `json:"connection_id"`
	CreatedByUserID *string `json:"created_by_user_id,omitempty"`
	Name            string  `json:"name"`
	// SourceKind is a SalesforceSource* constant and Object "Lead" or
	// "Contact".
	SourceKind  string `json:"source_kind"`
	Object      string `json:"object"`
	SourceID    string `json:"source_id"`
	SourceLabel string `json:"source_label"`
	// CampaignID enrolls every imported person in that campaign.
	CampaignID  *string              `json:"campaign_id,omitempty"`
	CategoryIDs []string             `json:"category_ids"`
	Recurring   bool                 `json:"recurring"`
	Enabled     bool                 `json:"enabled"`
	Status      string               `json:"status"`
	LastRunAt   *time.Time           `json:"last_run_at,omitempty"`
	LastResult  *SalesforceRunResult `json:"last_result,omitempty"`
	LastError   string               `json:"last_error,omitempty"`
	// TotalImported is the running count of contacts this source brought in.
	TotalImported int       `json:"total_imported"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// SalesforceImportSourceParams creates or edits a Salesforce import source.
// On update, nil fields are left unchanged and the source reference is
// re-validated when present.
type SalesforceImportSourceParams struct {
	Name *string `json:"name,omitempty"`
	// SourceKind, Object, SourceID and SourceLabel name what to read; see
	// [SalesforceImportRef].
	SourceKind  string `json:"source_kind,omitempty"`
	Object      string `json:"object,omitempty"`
	SourceID    string `json:"source_id,omitempty"`
	SourceLabel string `json:"source_label,omitempty"`
	// CampaignID enrolls imported people in a campaign (at most 50
	// CategoryIDs may be set).
	CampaignID *string `json:"campaign_id,omitempty"`
	// ClearCampaign detaches the target campaign on update by sending
	// campaign_id as null. It overrides CampaignID.
	ClearCampaign bool     `json:"-"`
	CategoryIDs   []string `json:"category_ids,omitempty"`
	Recurring     *bool    `json:"recurring,omitempty"`
	Enabled       *bool    `json:"enabled,omitempty"`
}

// MarshalJSON writes campaign_id as null when ClearCampaign is set.
func (p SalesforceImportSourceParams) MarshalJSON() ([]byte, error) {
	type plain SalesforceImportSourceParams
	if !p.ClearCampaign {
		return json.Marshal(plain(p))
	}
	p.CampaignID = nil
	raw, err := json.Marshal(plain(p))
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	m["campaign_id"] = json.RawMessage("null")
	return json.Marshal(m)
}

// Salesforce activity states for [SalesforceActivity.Status] and
// [SalesforceActivityParams.Status].
const (
	SalesforceActivityPending = "pending"
	SalesforceActivitySynced  = "synced"
	SalesforceActivitySkipped = "skipped"
	SalesforceActivityFailed  = "failed"
)

// SalesforceActivity is one queued or processed campaign event to log as a
// Salesforce Task, and what became of it.
type SalesforceActivity struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organization_id"`
	ConnectionID   string  `json:"connection_id"`
	ContactID      *string `json:"contact_id,omitempty"`
	ContactEmail   string  `json:"contact_email"`
	// Kind is sent, opened, clicked, replied, bounced, unsubscribed or
	// meeting_booked.
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
// first. The server caps Limit at 200 and defaults it to 50.
type SalesforceActivityParams struct {
	ListOptions
	// Status is one of the SalesforceActivity* constants.
	Status string
	// ContactID narrows to one contact's activity.
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

func salesforcePath(id, suffix string) string {
	return "integrations/salesforce/" + url.PathEscape(id) + suffix
}

// SalesforceOverview returns the connection's health page. With checks set the
// server also runs its self-test and fills [SalesforceOverview.Checks]; that
// makes live calls to Salesforce.
func (s *IntegrationService) SalesforceOverview(ctx context.Context, connectionID string, checks bool, opts ...RequestOption) (*SalesforceOverview, *Response, error) {
	q := make(url.Values)
	if checks {
		q.Set("checks", "1")
	}
	return fetch[SalesforceOverview](ctx, s.client, withQuery(salesforcePath(connectionID, "/overview"), q), opts)
}

// SalesforceSettings returns the sync settings, the Warmbly field vocabulary
// and the defaults.
func (s *IntegrationService) SalesforceSettings(ctx context.Context, connectionID string, opts ...RequestOption) (*SalesforceSettingsDocument, *Response, error) {
	return fetch[SalesforceSettingsDocument](ctx, s.client, salesforcePath(connectionID, "/settings"), opts)
}

// UpdateSalesforceSettings replaces the whole settings document, so a retry
// converges, and returns what was saved. An invalid document fails with
// "invalid_salesforce_settings".
func (s *IntegrationService) UpdateSalesforceSettings(ctx context.Context, connectionID string, settings *SalesforceSettings, opts ...RequestOption) (*SalesforceSettings, *Response, error) {
	var out struct {
		Settings *SalesforceSettings `json:"settings"`
	}
	resp, err := s.client.put(ctx, salesforcePath(connectionID, "/settings"), settings, &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Settings, resp, nil
}

// SalesforceMetadata returns the Lead and Contact fields and picklists.
func (s *IntegrationService) SalesforceMetadata(ctx context.Context, connectionID string, opts ...RequestOption) (*SalesforceMetadata, *Response, error) {
	return fetch[SalesforceMetadata](ctx, s.client, salesforcePath(connectionID, "/metadata"), opts)
}

// SalesforceUsers searches active Salesforce users; query may be empty.
func (s *IntegrationService) SalesforceUsers(ctx context.Context, connectionID, query string, opts ...RequestOption) ([]SalesforceUser, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "q", query)
	return fetchData[SalesforceUser](ctx, s.client, withQuery(salesforcePath(connectionID, "/users"), q), opts)
}

// SalesforceListViews lists the Lead or Contact list views; object is "Lead"
// or "Contact".
func (s *IntegrationService) SalesforceListViews(ctx context.Context, connectionID, object string, opts ...RequestOption) ([]SalesforceListView, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "object", object)
	return fetchData[SalesforceListView](ctx, s.client, withQuery(salesforcePath(connectionID, "/list-views"), q), opts)
}

// SalesforceCampaigns searches Salesforce Campaigns; query may be empty.
func (s *IntegrationService) SalesforceCampaigns(ctx context.Context, connectionID, query string, opts ...RequestOption) ([]SalesforceCampaign, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "q", query)
	return fetchData[SalesforceCampaign](ctx, s.client, withQuery(salesforcePath(connectionID, "/campaigns"), q), opts)
}

// PreviewSalesforceImport reads the first rows of a list view or Campaign
// without importing anything. Requires the use-integrations permission.
func (s *IntegrationService) PreviewSalesforceImport(ctx context.Context, connectionID string, ref *SalesforceImportRef, opts ...RequestOption) (*SalesforceImportPreview, *Response, error) {
	return send[SalesforceImportPreview](ctx, s.client.post, salesforcePath(connectionID, "/import/preview"), ref, opts)
}

// SalesforceImportSources lists the connection's saved imports.
func (s *IntegrationService) SalesforceImportSources(ctx context.Context, connectionID string, opts ...RequestOption) ([]SalesforceImportSource, *Response, error) {
	return fetchData[SalesforceImportSource](ctx, s.client, salesforcePath(connectionID, "/import-sources"), opts)
}

// CreateSalesforceImportSource saves an import and starts its first run. A
// retried create makes a second source whose run imports nothing new, because
// people are deduplicated by address.
func (s *IntegrationService) CreateSalesforceImportSource(ctx context.Context, connectionID string, params *SalesforceImportSourceParams, opts ...RequestOption) (*SalesforceImportSource, *Response, error) {
	return send[SalesforceImportSource](ctx, s.client.post, salesforcePath(connectionID, "/import-sources"), params, opts)
}

// UpdateSalesforceImportSource edits an import's targets and schedule.
func (s *IntegrationService) UpdateSalesforceImportSource(ctx context.Context, connectionID, sourceID string, params *SalesforceImportSourceParams, opts ...RequestOption) (*SalesforceImportSource, *Response, error) {
	return send[SalesforceImportSource](ctx, s.client.patch, salesforcePath(connectionID, "/import-sources/"+url.PathEscape(sourceID)), params, opts)
}

// RunSalesforceImportSource runs an import now and returns the source. A
// second request while one runs is refused with 409. Requires the
// use-integrations permission.
func (s *IntegrationService) RunSalesforceImportSource(ctx context.Context, connectionID, sourceID string, opts ...RequestOption) (*SalesforceImportSource, *Response, error) {
	return send[SalesforceImportSource](ctx, s.client.post, salesforcePath(connectionID, "/import-sources/"+url.PathEscape(sourceID)+"/run"), nil, opts)
}

// DeleteSalesforceImportSource removes an import; the contacts it brought in
// stay.
func (s *IntegrationService) DeleteSalesforceImportSource(ctx context.Context, connectionID, sourceID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, salesforcePath(connectionID, "/import-sources/"+url.PathEscape(sourceID)), opts...)
}

// SalesforceActivity returns a page of the activity log, newest first.
func (s *IntegrationService) SalesforceActivity(ctx context.Context, connectionID string, params *SalesforceActivityParams, opts ...RequestOption) (*Page[SalesforceActivity], error) {
	return listJSON[SalesforceActivity](ctx, s.client, salesforcePath(connectionID, "/activity"), params.values(), opts...)
}

// RetrySalesforceActivity re-queues failed or skipped activity rows by id (at
// most 500) and returns how many were requeued. Re-queuing a pending row
// changes nothing, so a retry is safe.
func (s *IntegrationService) RetrySalesforceActivity(ctx context.Context, connectionID string, ids []string, opts ...RequestOption) (int, *Response, error) {
	body := struct {
		IDs []string `json:"ids"`
	}{IDs: ids}
	var out struct {
		Requeued int `json:"requeued"`
	}
	resp, err := s.client.post(ctx, salesforcePath(connectionID, "/activity/retry"), body, &out, opts...)
	if err != nil {
		return 0, resp, err
	}
	return out.Requeued, resp, nil
}

// SalesforceSyncNow drains the pending activity and pulls changes at once.
// Both are idempotent passes. With sync turned off in the settings it fails
// with "salesforce_sync_off". Requires the use-integrations permission.
func (s *IntegrationService) SalesforceSyncNow(ctx context.Context, connectionID string, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, salesforcePath(connectionID, "/sync-now"), nil, nil, opts...)
}

// --- Apollo ---

// ApolloOverview is the Apollo page header: the account the key acts as and
// what the connection has done.
type ApolloOverview struct {
	ConnectionID  string     `json:"connection_id"`
	Label         string     `json:"label"`
	Status        string     `json:"status"`
	Health        string     `json:"health"`
	HealthDetail  string     `json:"health_detail,omitempty"`
	AccountName   string     `json:"account_name,omitempty"`
	AccountEmail  string     `json:"account_email,omitempty"`
	Sources       int        `json:"sources"`
	TotalImported int        `json:"total_imported"`
	LastRunAt     *time.Time `json:"last_run_at,omitempty"`
}

// ApolloList is one saved Apollo list, as the import picker shows it.
type ApolloList struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// ApolloPreviewRow is one sample person of a list preview.
type ApolloPreviewRow struct {
	ApolloID    string `json:"apollo_id"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	EmailStatus string `json:"email_status"`
	Title       string `json:"title"`
	Company     string `json:"company"`
	// AlreadyInWarmbly reports the person is already a contact, and
	// WillImport that a run would bring them in.
	AlreadyInWarmbly bool `json:"already_in_warmbly"`
	WillImport       bool `json:"will_import"`
}

// ApolloImportPreview is the first people of a list and how many it holds.
type ApolloImportPreview struct {
	Total  int                `json:"total"`
	Sample []ApolloPreviewRow `json:"sample"`
}

// ApolloRunResult is what one Apollo list import run did.
type ApolloRunResult struct {
	Read        int  `json:"read"`
	Imported    int  `json:"imported"`
	Updated     int  `json:"updated"`
	Skipped     int  `json:"skipped"`
	Failed      int  `json:"failed"`
	NoEmail     int  `json:"no_email"`
	Unverified  int  `json:"unverified"`
	Suppressed  int  `json:"suppressed"`
	Truncated   bool `json:"truncated,omitempty"`
	AlreadySeen int  `json:"already_seen"`
	// NextPage is where the next run resumes reading a list too long for one.
	NextPage int `json:"next_page,omitempty"`
}

// ApolloImportSource is a saved Apollo list that feeds contacts into Warmbly.
type ApolloImportSource struct {
	ID              string           `json:"id"`
	OrganizationID  string           `json:"organization_id"`
	ConnectionID    string           `json:"connection_id"`
	CreatedByUserID *string          `json:"created_by_user_id,omitempty"`
	Name            string           `json:"name"`
	ListID          string           `json:"list_id"`
	ListName        string           `json:"list_name"`
	CampaignID      *string          `json:"campaign_id,omitempty"`
	CategoryIDs     []string         `json:"category_ids"`
	VerifiedOnly    bool             `json:"verified_only"`
	Recurring       bool             `json:"recurring"`
	Enabled         bool             `json:"enabled"`
	Status          string           `json:"status"`
	LastRunAt       *time.Time       `json:"last_run_at,omitempty"`
	LastResult      *ApolloRunResult `json:"last_result,omitempty"`
	LastError       string           `json:"last_error,omitempty"`
	TotalImported   int              `json:"total_imported"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

// ApolloImportSourceParams creates or edits an Apollo import source. On
// update, nil fields are left unchanged.
type ApolloImportSourceParams struct {
	Name     *string `json:"name,omitempty"`
	ListID   string  `json:"list_id,omitempty"`
	ListName string  `json:"list_name,omitempty"`
	// CampaignID enrolls imported people in a campaign; ClearCampaign detaches
	// it on update by sending campaign_id as null and overrides CampaignID.
	CampaignID    *string  `json:"campaign_id,omitempty"`
	ClearCampaign bool     `json:"-"`
	CategoryIDs   []string `json:"category_ids,omitempty"`
	// VerifiedOnly skips people Apollo has not verified an address for.
	VerifiedOnly *bool `json:"verified_only,omitempty"`
	Recurring    *bool `json:"recurring,omitempty"`
	Enabled      *bool `json:"enabled,omitempty"`
}

// MarshalJSON writes campaign_id as null when ClearCampaign is set.
func (p ApolloImportSourceParams) MarshalJSON() ([]byte, error) {
	type plain ApolloImportSourceParams
	if !p.ClearCampaign {
		return json.Marshal(plain(p))
	}
	p.CampaignID = nil
	raw, err := json.Marshal(plain(p))
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	m["campaign_id"] = json.RawMessage("null")
	return json.Marshal(m)
}

// Apollo enrichment run states for [ApolloEnrichmentRun.Status].
const (
	ApolloEnrichmentQueued  = "queued"
	ApolloEnrichmentRunning = "running"
	ApolloEnrichmentDone    = "done"
	ApolloEnrichmentError   = "error"
)

// ApolloEnrichmentRun is one background request to enrich contacts through
// Apollo's people match.
type ApolloEnrichmentRun struct {
	ID                string  `json:"id"`
	OrganizationID    string  `json:"organization_id"`
	ConnectionID      string  `json:"connection_id"`
	RequestedByUserID *string `json:"requested_by_user_id,omitempty"`
	// Overwrite replaces fields that already have a value.
	Overwrite bool `json:"overwrite"`
	// Status is one of the ApolloEnrichment* constants.
	Status          string     `json:"status"`
	Total           int        `json:"total"`
	Processed       int        `json:"processed"`
	Enriched        int        `json:"enriched"`
	NotFound        int        `json:"not_found"`
	Failed          int        `json:"failed"`
	CreditsConsumed int        `json:"credits_consumed"`
	LastError       string     `json:"last_error,omitempty"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// ApolloEnrichParams starts an enrichment run over a contact selection. The
// selection may name "select all matching" through its filter and may not
// resolve to more than 5,000 contacts.
type ApolloEnrichParams struct {
	ContactSelection
	// Overwrite replaces fields that already have a value.
	Overwrite bool `json:"overwrite,omitempty"`
}

func apolloPath(id, suffix string) string {
	return "integrations/apollo/" + url.PathEscape(id) + suffix
}

// ApolloOverview returns the Apollo page header for a connection.
func (s *IntegrationService) ApolloOverview(ctx context.Context, connectionID string, opts ...RequestOption) (*ApolloOverview, *Response, error) {
	return fetch[ApolloOverview](ctx, s.client, apolloPath(connectionID, ""), opts)
}

// ReplaceApolloKey validates and stores a new Apollo API key for the
// connection and returns the refreshed overview. A key Apollo refuses fails
// with "apollo_key_rejected". Requires manage-settings with the integrations
// scope.
func (s *IntegrationService) ReplaceApolloKey(ctx context.Context, connectionID, apiKey string, opts ...RequestOption) (*ApolloOverview, *Response, error) {
	body := struct {
		APIKey string `json:"api_key"`
	}{APIKey: apiKey}
	return send[ApolloOverview](ctx, s.client.put, apolloPath(connectionID, "/key"), body, opts)
}

// ApolloLists returns the account's saved lists.
func (s *IntegrationService) ApolloLists(ctx context.Context, connectionID string, opts ...RequestOption) ([]ApolloList, *Response, error) {
	return fetchData[ApolloList](ctx, s.client, apolloPath(connectionID, "/lists"), opts)
}

// PreviewApolloImport reads the first people of a list without importing.
// verifiedOnly nil defaults to true on the server. Requires the
// use-integrations permission.
func (s *IntegrationService) PreviewApolloImport(ctx context.Context, connectionID, listID string, verifiedOnly *bool, opts ...RequestOption) (*ApolloImportPreview, *Response, error) {
	body := struct {
		ListID       string `json:"list_id"`
		VerifiedOnly *bool  `json:"verified_only,omitempty"`
	}{ListID: listID, VerifiedOnly: verifiedOnly}
	return send[ApolloImportPreview](ctx, s.client.post, apolloPath(connectionID, "/import/preview"), body, opts)
}

// ApolloImportSources lists the connection's saved list imports.
func (s *IntegrationService) ApolloImportSources(ctx context.Context, connectionID string, opts ...RequestOption) ([]ApolloImportSource, *Response, error) {
	return fetchData[ApolloImportSource](ctx, s.client, apolloPath(connectionID, "/import-sources"), opts)
}

// CreateApolloImportSource saves a list import and starts its first run.
func (s *IntegrationService) CreateApolloImportSource(ctx context.Context, connectionID string, params *ApolloImportSourceParams, opts ...RequestOption) (*ApolloImportSource, *Response, error) {
	return send[ApolloImportSource](ctx, s.client.post, apolloPath(connectionID, "/import-sources"), params, opts)
}

// UpdateApolloImportSource edits a list import's targets and schedule.
func (s *IntegrationService) UpdateApolloImportSource(ctx context.Context, connectionID, sourceID string, params *ApolloImportSourceParams, opts ...RequestOption) (*ApolloImportSource, *Response, error) {
	return send[ApolloImportSource](ctx, s.client.patch, apolloPath(connectionID, "/import-sources/"+url.PathEscape(sourceID)), params, opts)
}

// RunApolloImportSource runs a list import now. Requires the use-integrations
// permission.
func (s *IntegrationService) RunApolloImportSource(ctx context.Context, connectionID, sourceID string, opts ...RequestOption) (*ApolloImportSource, *Response, error) {
	return send[ApolloImportSource](ctx, s.client.post, apolloPath(connectionID, "/import-sources/"+url.PathEscape(sourceID)+"/run"), nil, opts)
}

// DeleteApolloImportSource removes a list import; its contacts stay.
func (s *IntegrationService) DeleteApolloImportSource(ctx context.Context, connectionID, sourceID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, apolloPath(connectionID, "/import-sources/"+url.PathEscape(sourceID)), opts...)
}

// ApolloEnrichments returns the connection's most recent enrichment runs. limit
// 0 uses the server default of 20; the maximum is 100.
func (s *IntegrationService) ApolloEnrichments(ctx context.Context, connectionID string, limit int, opts ...RequestOption) ([]ApolloEnrichmentRun, *Response, error) {
	q := make(url.Values)
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	return fetchData[ApolloEnrichmentRun](ctx, s.client, withQuery(apolloPath(connectionID, "/enrichments"), q), opts)
}

// ApolloEnrichment returns one enrichment run, for polling its progress.
func (s *IntegrationService) ApolloEnrichment(ctx context.Context, connectionID, runID string, opts ...RequestOption) (*ApolloEnrichmentRun, *Response, error) {
	return fetch[ApolloEnrichmentRun](ctx, s.client, apolloPath(connectionID, "/enrichments/"+url.PathEscape(runID)), opts)
}

// StartApolloEnrichment queues an enrichment run over a contact selection and
// returns it (HTTP 202). It spends the Apollo account's credits. Requires the
// use-integrations permission, the manage-contacts permission and the bulk
// contacts scope, and a paid plan. A selection over 5,000 contacts fails with
// "apollo_enrich_too_many".
func (s *IntegrationService) StartApolloEnrichment(ctx context.Context, connectionID string, params *ApolloEnrichParams, opts ...RequestOption) (*ApolloEnrichmentRun, *Response, error) {
	return send[ApolloEnrichmentRun](ctx, s.client.post, apolloPath(connectionID, "/enrichments"), params, opts)
}

// --- Clay ---

// ClayInboundDefaults decide where rows a Clay table sends land. A row may
// name its own campaign.
type ClayInboundDefaults struct {
	CampaignID  *string  `json:"campaign_id,omitempty"`
	CategoryIDs []string `json:"category_ids"`
	SegmentIDs  []string `json:"segment_ids"`
	// Dedup is one of the ImportDedup* constants.
	Dedup      string `json:"dedup"`
	Subscribed bool   `json:"subscribed"`
}

// ClayOutbound is the default Clay table Warmbly sends events to. The URL and
// the token are sealed server-side; only their presence is reported.
type ClayOutbound struct {
	WebhookURLSet bool     `json:"webhook_url_set"`
	WebhookHost   string   `json:"webhook_host,omitempty"`
	AuthTokenSet  bool     `json:"auth_token_set"`
	Events        []string `json:"events"`
}

// ClayStats count what the inbound URL has received.
type ClayStats struct {
	Received       int        `json:"received"`
	Created        int        `json:"created"`
	Updated        int        `json:"updated"`
	Skipped        int        `json:"skipped"`
	Failed         int        `json:"failed"`
	LastReceivedAt *time.Time `json:"last_received_at,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
}

// ClaySettings is everything the Clay page edits and shows.
type ClaySettings struct {
	ConnectionID string `json:"connection_id"`
	Label        string `json:"label"`
	// Status is one of the Connection* constants and Health "unknown",
	// "healthy", "degraded" or "down".
	Status string `json:"status"`
	Health string `json:"health"`
	// InboundURLMinted reports whether the connection has an inbound URL yet.
	InboundURLMinted bool                `json:"inbound_url_minted"`
	Defaults         ClayInboundDefaults `json:"defaults"`
	Outbound         ClayOutbound        `json:"outbound"`
	Stats            ClayStats           `json:"stats"`
	// AvailableEvents are the event types [ClaySettingsParams.Events] may
	// choose from.
	AvailableEvents []string `json:"available_events"`
}

// ClaySettingsParams edits the Clay page. A nil field is left alone; an empty
// WebhookURL or AuthToken clears it. Every set field replaces its value, so a
// retry converges.
type ClaySettingsParams struct {
	Defaults *ClayInboundDefaults `json:"defaults"`
	// WebhookURL is a Clay table webhook URL; anything else fails with
	// "clay_webhook_url". Choosing events with no webhook set fails with
	// "clay_webhook_missing".
	WebhookURL *string   `json:"webhook_url"`
	AuthToken  *string   `json:"auth_token"`
	Events     *[]string `json:"events"`
}

// ClaySettings returns everything the Clay page shows for a connection.
func (s *IntegrationService) ClaySettings(ctx context.Context, connectionID string, opts ...RequestOption) (*ClaySettings, *Response, error) {
	return fetch[ClaySettings](ctx, s.client, "integrations/clay/"+url.PathEscape(connectionID), opts)
}

// UpdateClaySettings saves the inbound defaults, the outbound table and the
// events sent to it, and returns the refreshed page. Requires manage-settings
// with the integrations scope.
func (s *IntegrationService) UpdateClaySettings(ctx context.Context, connectionID string, params *ClaySettingsParams, opts ...RequestOption) (*ClaySettings, *Response, error) {
	return send[ClaySettings](ctx, s.client.patch, "integrations/clay/"+url.PathEscape(connectionID), params, opts)
}

// --- Slack ---
//
// The Slack panel is session-only: it accepts a token from [AuthService.Login]
// and rejects an API key, because a link ties a Slack account to a signed-in
// member.

// Slack inbox scopes for [SlackSettings.InboxScope].
const (
	// SlackInboxReplies mirrors human replies only.
	SlackInboxReplies = "replies"
	// SlackInboxAll mirrors every inbound message.
	SlackInboxAll = "all"
)

// SlackSettings is the non-secret Slack configuration. Channel values are
// Slack channel ids (C... or G...); a legacy "#name" still posts to public
// channels.
type SlackSettings struct {
	// Channel is the default channel for notifications.
	Channel string `json:"channel,omitempty"`
	// Routes sends a notification category to its own channel; a category
	// absent here uses Channel.
	Routes map[string]string `json:"routes,omitempty"`
	// AssistantDisabled turns the assistant off for the whole Slack workspace.
	AssistantDisabled bool `json:"assistant_disabled,omitempty"`
	// AssistantDMOnly keeps the assistant out of channels.
	AssistantDMOnly bool `json:"assistant_dm_only,omitempty"`
	// InboxChannel mirrors unified-inbox arrivals into this channel, one Slack
	// thread per conversation. Empty turns the inbox mirror off.
	InboxChannel string `json:"inbox_channel,omitempty"`
	// InboxScope is [SlackInboxReplies] (the default) or [SlackInboxAll].
	InboxScope string `json:"inbox_scope,omitempty"`
}

// SlackUserLink binds a Slack member to a Warmbly member. Anything the bot
// does for that Slack member runs as the Warmbly member with their current
// permissions.
type SlackUserLink struct {
	ID              string    `json:"id"`
	OrganizationID  string    `json:"organization_id"`
	ConnectionID    string    `json:"connection_id"`
	SlackTeamID     string    `json:"slack_team_id"`
	SlackUserID     string    `json:"slack_user_id"`
	UserID          string    `json:"user_id"`
	DMNotifications bool      `json:"dm_notifications"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	// UserName and UserEmail are joined in for display.
	UserName  string `json:"user_name,omitempty"`
	UserEmail string `json:"user_email,omitempty"`
}

// SlackStatus is the Slack panel's state for the caller.
type SlackStatus struct {
	// AppConfigured reports that the instance has Slack OAuth credentials.
	AppConfigured bool `json:"app_configured"`
	// InteractiveConfigured reports that it also has a signing secret, so
	// events, the assistant, slash commands and buttons work.
	InteractiveConfigured bool `json:"interactive_configured"`
	// Connection is the workspace's Slack connection; nil when not connected.
	Connection *IntegrationConnection `json:"connection,omitempty"`
	// MissingScopes lists bot scopes the install predates; reconnecting
	// grants them.
	MissingScopes []string      `json:"missing_scopes"`
	Settings      SlackSettings `json:"settings"`
	// MyLink is the caller's own link, when they have one. Links lists the
	// workspace's links and is populated only for a member who manages
	// settings or uses integrations.
	MyLink *SlackUserLink  `json:"my_link,omitempty"`
	Links  []SlackUserLink `json:"links"`
}

// SlackChannel is one entry of the channel picker.
type SlackChannel struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsPrivate bool   `json:"is_private"`
	IsMember  bool   `json:"is_member"`
}

// SlackLinkPreview is what a member sees before confirming a link minted by
// the Slack bot.
type SlackLinkPreview struct {
	SlackTeamID   string `json:"slack_team_id"`
	SlackTeamName string `json:"slack_team_name"`
	SlackUserID   string `json:"slack_user_id"`
	// SlackUserName and SlackUserAvatar describe the Slack account being
	// linked; either may be empty.
	SlackUserName   string `json:"slack_user_name"`
	SlackUserAvatar string `json:"slack_user_avatar"`
	// EmailMatches reports that the Slack account's email is the caller's
	// Warmbly email, which confirming requires.
	EmailMatches bool      `json:"email_matches"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// SlackStatus returns the Slack panel state for the caller in the session's
// workspace.
func (s *IntegrationService) SlackStatus(ctx context.Context, opts ...RequestOption) (*SlackStatus, *Response, error) {
	return fetch[SlackStatus](ctx, s.client, "integrations/slack/status", opts)
}

// SlackChannels returns a page of the channel picker, filtered by query.
// Requires the integrations access of a member who manages settings or uses
// integrations. The page carries no cursor; Pagination.HasMore says the list
// was cut off.
func (s *IntegrationService) SlackChannels(ctx context.Context, query string, opts ...RequestOption) ([]SlackChannel, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "q", query)
	return fetchData[SlackChannel](ctx, s.client, withQuery("integrations/slack/channels", q), opts)
}

// UpdateSlackSettings replaces the Slack settings and returns what was saved.
// Requires manage-settings.
func (s *IntegrationService) UpdateSlackSettings(ctx context.Context, settings *SlackSettings, opts ...RequestOption) (*SlackSettings, *Response, error) {
	return send[SlackSettings](ctx, s.client.put, "integrations/slack/settings", settings, opts)
}

// PreviewSlackLink resolves a link code minted by the Slack bot, so the member
// sees which Slack account they are about to link. It needs no workspace on
// the session: the code names it.
func (s *IntegrationService) PreviewSlackLink(ctx context.Context, code string, opts ...RequestOption) (*SlackLinkPreview, *Response, error) {
	return fetch[SlackLinkPreview](ctx, s.client, "integrations/slack/link/"+url.PathEscape(code), opts)
}

// ConfirmSlackLink links the caller to the Slack account behind code and
// returns the link (HTTP 201). slackCode and state are the proof returned by
// Slack's sign-in after [IntegrationService.StartSlackLinkVerify]; leave them
// empty when the Slack email already matches.
func (s *IntegrationService) ConfirmSlackLink(ctx context.Context, code, slackCode, state string, opts ...RequestOption) (*SlackUserLink, *Response, error) {
	body := struct {
		Code      string `json:"code"`
		SlackCode string `json:"slack_code,omitempty"`
		State     string `json:"state,omitempty"`
	}{Code: code, SlackCode: slackCode, State: state}
	return send[SlackUserLink](ctx, s.client.post, "integrations/slack/link", body, opts)
}

// StartSlackLinkVerify returns the Slack sign-in URL that proves the caller
// owns the Slack account behind code, for the case where the Slack email does
// not match the Warmbly email.
func (s *IntegrationService) StartSlackLinkVerify(ctx context.Context, code string, opts ...RequestOption) (string, *Response, error) {
	body := struct {
		Code string `json:"code"`
	}{Code: code}
	var out struct {
		URL string `json:"url"`
	}
	resp, err := s.client.post(ctx, "integrations/slack/link/verify", body, &out, opts...)
	if err != nil {
		return "", resp, err
	}
	return out.URL, resp, nil
}

// UpdateMySlackLink turns the caller's Slack direct-message notifications on
// or off and returns the link.
func (s *IntegrationService) UpdateMySlackLink(ctx context.Context, dmNotifications bool, opts ...RequestOption) (*SlackUserLink, *Response, error) {
	body := struct {
		DMNotifications bool `json:"dm_notifications"`
	}{DMNotifications: dmNotifications}
	return send[SlackUserLink](ctx, s.client.patch, "integrations/slack/link", body, opts)
}

// DeleteMySlackLink removes the caller's own Slack link.
func (s *IntegrationService) DeleteMySlackLink(ctx context.Context, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "integrations/slack/link", opts...)
}

// RemoveSlackLink removes any member's Slack link by its id. Requires
// manage-settings.
func (s *IntegrationService) RemoveSlackLink(ctx context.Context, linkID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "integrations/slack/links/"+url.PathEscape(linkID), opts...)
}
