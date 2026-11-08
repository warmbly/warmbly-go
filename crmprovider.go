package warmbly

import (
	"context"
	"net/url"
	"time"
)

// CRM mode: a workspace runs on Warmbly's own CRM or on a connected one
// (HubSpot, Pipedrive). The methods below read and change that choice, the
// provider's vocabulary and owners, sync health, backfill, the provider side
// of a contact, and list imports. They live on [CRMService] next to the
// pipelines, deals and tasks, which show the connected CRM's records when the
// workspace runs on one (see [CRMExternalRef]).
//
// Every method answers 501 (not implemented) when the instance has no CRM
// providers, and most answer 4xx until a provider is connected.

// CRM providers returned in [CRMSettings.Provider] and accepted in
// [CRMSettingsUpdateParams.Provider].
const (
	// CRMProviderNative is Warmbly's own CRM.
	CRMProviderNative    = "native"
	CRMProviderHubSpot   = "hubspot"
	CRMProviderPipedrive = "pipedrive"
)

// Field directions for [CRMProviderConfig.FieldDirection].
const (
	// CRMFieldPush means Warmbly writes the provider.
	CRMFieldPush = "push"
	// CRMFieldPull means the provider writes Warmbly.
	CRMFieldPull = "pull"
	// CRMFieldBoth means the most recent change wins.
	CRMFieldBoth = "both"
)

// CRM sync job statuses returned in [CRMSyncJob.Status].
const (
	CRMJobPending = "pending"
	CRMJobRunning = "running"
	CRMJobDone    = "done"
	CRMJobFailed  = "failed"
)

// CRMExternalRef says where a record lives outside Warmbly. It is attached to
// pipelines, deals, tasks and notes read while the workspace runs on a
// connected CRM.
type CRMExternalRef struct {
	// Provider is one of the CRMProvider* constants.
	Provider   string `json:"provider"`
	ExternalID string `json:"external_id"`
	// URL opens the record in the provider's app, when known.
	URL      string    `json:"url,omitempty"`
	SyncedAt time.Time `json:"synced_at"`
	// OwnerName is the provider's owner when that owner is not a workspace
	// member.
	OwnerName string `json:"owner_name,omitempty"`
}

// CRMActivityLog chooses which Warmbly activity is written to the provider's
// timeline. Opens and clicks default off because machine opens make them noise.
type CRMActivityLog struct {
	Sent         bool `json:"sent"`
	Replies      bool `json:"replies"`
	Bounces      bool `json:"bounces"`
	Unsubscribes bool `json:"unsubscribes"`
	Opens        bool `json:"opens"`
	Clicks       bool `json:"clicks"`
	Meetings     bool `json:"meetings"`
}

// CRMReplyOutcome is what a positive reply does in the provider.
type CRMReplyOutcome struct {
	// LeadStatus is written to the contact's lead status; empty leaves it.
	LeadStatus string `json:"lead_status"`
	// LifecycleStage is written to the contact's lifecycle stage; empty
	// leaves it.
	LifecycleStage string `json:"lifecycle_stage"`
	CreateDeal     bool   `json:"create_deal"`
	// CreateLead adds the person to Pipedrive's Leads Inbox (Pipedrive only).
	CreateLead bool `json:"create_lead,omitempty"`
	// DealPipelineID and DealStageID are Warmbly (mirrored) ids. Both are
	// required when CreateDeal is set.
	DealPipelineID *string `json:"deal_pipeline_id,omitempty"`
	DealStageID    *string `json:"deal_stage_id,omitempty"`
}

// CRMExitRules stop a contact's campaigns when the provider says they have
// moved on, so nobody is followed up after becoming a deal or a customer.
type CRMExitRules struct {
	DealCreated bool `json:"deal_created"`
	// LifecycleStages stops a contact whose lifecycle stage becomes any of
	// these.
	LifecycleStages []string `json:"lifecycle_stages"`
	OptedOut        bool     `json:"opted_out"`
}

// CRMEnrollmentGuards skip contacts at import and enrolment time.
type CRMEnrollmentGuards struct {
	SkipLifecycleStages []string `json:"skip_lifecycle_stages"`
	SkipOpenDeals       bool     `json:"skip_open_deals"`
	SkipOtherOwners     bool     `json:"skip_other_owners"`
	SkipOptedOut        bool     `json:"skip_opted_out"`
}

// CRMProviderConfig is every choice the CRM setup wizard asks, with defaults
// that work without changing anything.
type CRMProviderConfig struct {
	Activity CRMActivityLog `json:"activity"`
	// CreateContacts lets Warmbly create a provider contact for someone it
	// emails who is not in the CRM yet.
	CreateContacts bool `json:"create_contacts"`
	// CreateCompanies creates and associates a company from the contact's
	// domain.
	CreateCompanies bool `json:"create_companies"`
	// WriteProperties keeps the "Warmbly" property group on contacts current.
	WriteProperties bool                `json:"write_properties"`
	PositiveReply   CRMReplyOutcome     `json:"positive_reply"`
	ExitRules       CRMExitRules        `json:"exit_rules"`
	Guards          CRMEnrollmentGuards `json:"guards"`
	// DealPipelines limits the mirrored deals to these provider pipeline ids;
	// empty mirrors every pipeline.
	DealPipelines []string `json:"deal_pipelines"`
	// DisplayProperties are extra contact properties shown in the dashboard.
	DisplayProperties []string `json:"display_properties"`
	// FieldMap maps a Warmbly contact field (first_name, last_name, email,
	// company, phone, or custom:<key>) to a provider property, and
	// FieldDirection says which side wins per field (a CRMField* constant).
	FieldMap       map[string]string `json:"field_map"`
	FieldDirection map[string]string `json:"field_direction"`
	// For is the provider these choices were made for. Empty means HubSpot.
	For string `json:"for,omitempty"`
}

// CRMAccount identifies the connected provider account.
type CRMAccount struct {
	ExternalID string `json:"external_id"`
	Name       string `json:"name"`
	// AppURL is the provider's web app root for this account.
	AppURL string `json:"app_url"`
	Status string `json:"status"`
	Health string `json:"health"`
	// MissingScopes lists permissions the connection lacks for full mode, so
	// a reconnect can be requested.
	MissingScopes []string `json:"missing_scopes,omitempty"`
}

// CRMSettings is the workspace's CRM mode.
type CRMSettings struct {
	OrganizationID string `json:"organization_id"`
	// Provider is one of the CRMProvider* constants.
	Provider     string            `json:"provider"`
	ConnectionID *string           `json:"connection_id,omitempty"`
	Config       CRMProviderConfig `json:"config"`
	// SetupCompletedAt is set once the setup wizard is finished.
	SetupCompletedAt *time.Time `json:"setup_completed_at,omitempty"`
	UpdatedAt        time.Time  `json:"updated_at"`
	// Account describes the connected provider account, when one is.
	Account *CRMAccount `json:"account,omitempty"`
}

// CRMSettingsUpdateParams switches the CRM and stores the setup choices. Nil
// fields are left unchanged.
type CRMSettingsUpdateParams struct {
	// Provider is one of the CRMProvider* constants.
	Provider *string `json:"provider,omitempty"`
	// ConnectionID is the integration connection of the provider to run on.
	ConnectionID *string            `json:"connection_id,omitempty"`
	Config       *CRMProviderConfig `json:"config,omitempty"`
	// CompleteSetup stamps the setup wizard as finished.
	CompleteSetup bool `json:"complete_setup,omitempty"`
}

// CRMOption is a provider enum value with its label.
type CRMOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// CRMProperty is a provider contact property a field can map to.
type CRMProperty struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	Type      string `json:"type"`
	GroupName string `json:"group_name"`
	ReadOnly  bool   `json:"read_only"`
}

// CRMMetadata is the provider vocabulary pickers render.
type CRMMetadata struct {
	LifecycleStages []CRMOption   `json:"lifecycle_stages"`
	LeadStatuses    []CRMOption   `json:"lead_statuses"`
	TaskTypes       []CRMOption   `json:"task_types"`
	Properties      []CRMProperty `json:"properties"`
	Pipelines       []CRMOption   `json:"pipelines"`
}

// CRMOwner is a provider user who can own records, and the workspace member
// they are matched to.
type CRMOwner struct {
	ExternalID string `json:"external_id"`
	Email      string `json:"email"`
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
	// UserID is the matched workspace member, when there is one.
	UserID *string `json:"user_id,omitempty"`
	// UserPinned is set when the match was made by hand with
	// [CRMService.MapOwner] rather than by email.
	UserPinned bool `json:"user_pinned"`
	Archived   bool `json:"archived"`
}

// CRMSyncJob is one queued, running or failed piece of sync work.
type CRMSyncJob struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Provider       string `json:"provider"`
	Kind           string `json:"kind"`
	Subject        string `json:"subject"`
	// Status is one of the CRMJob* constants.
	Status        string     `json:"status"`
	Attempts      int        `json:"attempts"`
	NextAttemptAt time.Time  `json:"next_attempt_at"`
	LastError     string     `json:"last_error,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
}

// CRMSyncCursor reports one incremental pull.
type CRMSyncCursor struct {
	ObjectType string     `json:"object_type"`
	CursorAt   *time.Time `json:"cursor_at,omitempty"`
	LastRunAt  *time.Time `json:"last_run_at,omitempty"`
	LastError  string     `json:"last_error,omitempty"`
}

// CRMMirrorCounts is how much provider data the workspace mirrors.
type CRMMirrorCounts struct {
	Contacts  int64 `json:"contacts"`
	Deals     int64 `json:"deals"`
	Tasks     int64 `json:"tasks"`
	Pipelines int64 `json:"pipelines"`
	Owners    int64 `json:"owners"`
}

// CRMSyncHealth reports what is waiting, what failed and when each pull ran.
type CRMSyncHealth struct {
	Pending      int64           `json:"pending"`
	Failed       int64           `json:"failed"`
	Done24h      int64           `json:"done_24h"`
	LastSyncedAt *time.Time      `json:"last_synced_at,omitempty"`
	Cursors      []CRMSyncCursor `json:"cursors"`
	Failures     []CRMSyncJob    `json:"failures"`
	Counts       CRMMirrorCounts `json:"counts"`
}

// CRMCompanyRef is a contact's primary company in the provider.
type CRMCompanyRef struct {
	ExternalID string `json:"external_id"`
	Name       string `json:"name"`
	Domain     string `json:"domain,omitempty"`
	URL        string `json:"url,omitempty"`
}

// CRMPropertyView is one displayed provider property, labeled.
type CRMPropertyView struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Value string `json:"value"`
}

// CRMContactView is the provider side of a contact: owner, lifecycle stage,
// lead status, company and the chosen properties.
type CRMContactView struct {
	// Provider is one of the CRMProvider* constants.
	Provider string `json:"provider"`
	// Linked is false until the contact exists in the provider; see
	// [CRMService.LinkContact].
	Linked         bool              `json:"linked"`
	ExternalID     string            `json:"external_id,omitempty"`
	URL            string            `json:"url,omitempty"`
	Owner          *CRMOwner         `json:"owner,omitempty"`
	LifecycleStage *CRMOption        `json:"lifecycle_stage,omitempty"`
	LeadStatus     *CRMOption        `json:"lead_status,omitempty"`
	Company        *CRMCompanyRef    `json:"company,omitempty"`
	OptedOut       bool              `json:"opted_out"`
	Properties     []CRMPropertyView `json:"properties"`
	SyncedAt       *time.Time        `json:"synced_at,omitempty"`
}

// CRMContactUpdateParams edits the sales fields of a contact in place in the
// provider. Nil fields are left unchanged; each value is at most 100
// characters.
type CRMContactUpdateParams struct {
	// OwnerExternalID is the provider owner's id (see [CRMOwner.ExternalID]).
	OwnerExternalID *string `json:"owner_external_id,omitempty"`
	LifecycleStage  *string `json:"lifecycle_stage,omitempty"`
	LeadStatus      *string `json:"lead_status,omitempty"`
}

// CRMList is a provider contact list Warmbly can import from.
type CRMList struct {
	ExternalID string `json:"external_id"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	// Dynamic is set for lists the provider fills by rule.
	Dynamic   bool   `json:"dynamic"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// CRMListParams filters and paginates the provider's lists. The page limit is
// 1 to 100 and defaults to 25.
type CRMListParams struct {
	ListOptions
	// Query matches list names, at most 200 characters.
	Query string
}

func (p *CRMListParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	setNonEmpty(q, "q", p.Query)
	return q
}

// CRMListImportParams names the provider list to preview or import.
type CRMListImportParams struct {
	// ListID is a [CRMList.ExternalID], at most 64 characters.
	ListID string `json:"list_id"`
	// ApplyGuards drops contacts the workspace's enrolment guards exclude.
	ApplyGuards bool `json:"apply_guards"`
}

// CRMImportSkip is one skip reason and how many contacts it removed.
type CRMImportSkip struct {
	Reason string `json:"reason"`
	Label  string `json:"label"`
	Count  int    `json:"count"`
}

// CRMImportPerson is a sample row of an import preview.
type CRMImportPerson struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Company   string `json:"company"`
}

// CRMImportPreview counts who a list import would bring in and who it skips.
type CRMImportPreview struct {
	ListName string            `json:"list_name"`
	Total    int               `json:"total"`
	Included int               `json:"included"`
	Skipped  []CRMImportSkip   `json:"skipped"`
	Sample   []CRMImportPerson `json:"sample"`
	// Truncated is set when the list is larger than one import takes.
	Truncated bool `json:"truncated"`
}

// CRMImportResult names the contact import draft a list import produced. It
// is finished like any file import: review it with [ContactService.GetImport]
// and run it with [ContactService.StartImport].
type CRMImportResult struct {
	ImportID string           `json:"import_id"`
	Preview  CRMImportPreview `json:"preview"`
}

// CRMBackfillParams chooses what to copy from Warmbly's own CRM into the
// provider once, when a workspace switches.
type CRMBackfillParams struct {
	Deals bool `json:"deals"`
	Tasks bool `json:"tasks"`
	Notes bool `json:"notes"`
}

// CRMBackfillPreview counts what a backfill would copy.
type CRMBackfillPreview struct {
	Deals int64 `json:"deals"`
	Tasks int64 `json:"tasks"`
	Notes int64 `json:"notes"`
}

// --- settings and vocabulary ---

// Settings returns the workspace's CRM mode and the connected account.
//
// Requires the READ_CRM scope.
func (s *CRMService) Settings(ctx context.Context, opts ...RequestOption) (*CRMSettings, *Response, error) {
	return fetch[CRMSettings](ctx, s.client, "crm/settings", opts)
}

// UpdateSettings switches the CRM and stores the setup choices. Switching
// away from a provider tidies up its webhooks in the background.
//
// Requires the INTEGRATIONS scope.
func (s *CRMService) UpdateSettings(ctx context.Context, params *CRMSettingsUpdateParams, opts ...RequestOption) (*CRMSettings, *Response, error) {
	return send[CRMSettings](ctx, s.client.put, "crm/settings", params, opts)
}

// Metadata returns the provider's lifecycle stages, lead statuses, task types,
// contact properties and pipelines, for pickers.
//
// Requires the READ_CRM scope.
func (s *CRMService) Metadata(ctx context.Context, opts ...RequestOption) (*CRMMetadata, *Response, error) {
	return fetch[CRMMetadata](ctx, s.client, "crm/metadata", opts)
}

// Owners lists the provider's owners and the workspace member each one is.
//
// Requires the READ_CRM scope.
func (s *CRMService) Owners(ctx context.Context, opts ...RequestOption) ([]CRMOwner, *Response, error) {
	return fetchData[CRMOwner](ctx, s.client, "crm/owners", opts)
}

// MapOwner pins a provider owner to a workspace member. A nil userID clears
// the match, so the owner is matched by email again.
//
// Requires the INTEGRATIONS scope.
func (s *CRMService) MapOwner(ctx context.Context, externalID string, userID *string, opts ...RequestOption) (*Response, error) {
	body := struct {
		UserID *string `json:"user_id"`
	}{UserID: userID}
	return s.client.put(ctx, "crm/owners/"+url.PathEscape(externalID), body, nil, opts...)
}

// --- sync health and backfill ---

// SyncHealth reports what is waiting, what failed and when each pull ran.
//
// Requires the READ_CRM scope.
func (s *CRMService) SyncHealth(ctx context.Context, opts ...RequestOption) (*CRMSyncHealth, *Response, error) {
	return fetch[CRMSyncHealth](ctx, s.client, "crm/sync", opts)
}

// SyncNow starts a full pull from the provider. It answers 202 and the pull
// runs in the background; watch [CRMService.SyncHealth] for progress.
//
// Requires the INTEGRATIONS scope.
func (s *CRMService) SyncNow(ctx context.Context, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "crm/sync", nil, nil, opts...)
}

// crmJobSelection is the body of the retry and discard calls.
type crmJobSelection struct {
	IDs []string `json:"ids,omitempty"`
}

// RetrySyncFailures requeues failed sync jobs, all of them when ids is empty
// (at most 500 ids per call), and returns how many it requeued. Safe to
// repeat: requeueing a requeued job changes nothing.
//
// Requires the INTEGRATIONS scope.
func (s *CRMService) RetrySyncFailures(ctx context.Context, ids []string, opts ...RequestOption) (int64, *Response, error) {
	return s.jobAction(ctx, "crm/sync/retry", ids, opts)
}

// DiscardSyncFailures drops failed sync jobs, all of them when ids is empty
// (at most 500 ids per call), and returns how many it dropped.
//
// Requires the INTEGRATIONS scope.
func (s *CRMService) DiscardSyncFailures(ctx context.Context, ids []string, opts ...RequestOption) (int64, *Response, error) {
	return s.jobAction(ctx, "crm/sync/discard", ids, opts)
}

func (s *CRMService) jobAction(ctx context.Context, path string, ids []string, opts []RequestOption) (int64, *Response, error) {
	var out struct {
		Affected int64 `json:"affected"`
	}
	resp, err := s.client.post(ctx, path, crmJobSelection{IDs: ids}, &out, opts...)
	if err != nil {
		return 0, resp, err
	}
	return out.Affected, resp, nil
}

// Backfill counts the Warmbly-only deals, tasks and notes a switch to a
// connected CRM would copy.
//
// Requires the INTEGRATIONS scope.
func (s *CRMService) Backfill(ctx context.Context, opts ...RequestOption) (*CRMBackfillPreview, *Response, error) {
	return fetch[CRMBackfillPreview](ctx, s.client, "crm/backfill", opts)
}

// StartBackfill copies Warmbly's own deals, tasks and notes into the provider
// once. It answers 202 and runs in the background. Safe to repeat: a second
// call while one runs is folded into it, and copied records are never copied
// twice.
//
// Requires the INTEGRATIONS scope.
func (s *CRMService) StartBackfill(ctx context.Context, params *CRMBackfillParams, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "crm/backfill", params, nil, opts...)
}

// --- the provider side of a contact ---

// Contact returns the provider side of a Warmbly contact.
//
// Requires the READ_CRM scope.
func (s *CRMService) Contact(ctx context.Context, contactID string, opts ...RequestOption) (*CRMContactView, *Response, error) {
	return fetch[CRMContactView](ctx, s.client, "crm/contacts/"+url.PathEscape(contactID), opts)
}

// RefreshContact pulls the contact's provider record, deals, tasks and notes
// now. It is debounced per contact, so calling it whenever a panel opens is
// safe.
//
// Requires the READ_CRM scope.
func (s *CRMService) RefreshContact(ctx context.Context, contactID string, opts ...RequestOption) (*CRMContactView, *Response, error) {
	return send[CRMContactView](ctx, s.client.post, "crm/contacts/"+url.PathEscape(contactID)+"/refresh", nil, opts)
}

// LinkContact finds the contact in the provider, or creates it there.
//
// Requires the WRITE_CRM scope.
func (s *CRMService) LinkContact(ctx context.Context, contactID string, opts ...RequestOption) (*CRMContactView, *Response, error) {
	return send[CRMContactView](ctx, s.client.post, "crm/contacts/"+url.PathEscape(contactID)+"/link", nil, opts)
}

// UpdateContact edits a contact's owner, lifecycle stage or lead status in
// place in the provider.
//
// Requires the WRITE_CRM scope.
func (s *CRMService) UpdateContact(ctx context.Context, contactID string, params *CRMContactUpdateParams, opts ...RequestOption) (*CRMContactView, *Response, error) {
	return send[CRMContactView](ctx, s.client.patch, "crm/contacts/"+url.PathEscape(contactID), params, opts)
}

// --- list import ---

// Lists returns a page of the provider's contact lists for import.
//
// Requires the READ_CRM scope and contact management rights.
func (s *CRMService) Lists(ctx context.Context, params *CRMListParams, opts ...RequestOption) (*Page[CRMList], error) {
	return listJSON[CRMList](ctx, s.client, "crm/lists", params.values(), opts...)
}

// PreviewListImport counts who importing a provider list would bring in and
// who it would skip. It writes nothing.
//
// Requires the WRITE_CONTACTS scope.
func (s *CRMService) PreviewListImport(ctx context.Context, params *CRMListImportParams, opts ...RequestOption) (*CRMImportPreview, *Response, error) {
	return send[CRMImportPreview](ctx, s.client.post, "crm/lists/preview", params, opts)
}

// ImportList turns a provider list into a contact import draft, which is then
// finished like a file import ([ContactService.AnalyzeImport] and
// [ContactService.StartImport]). The draft belongs to the calling user, and a
// credential that resolves to no user is refused with 401.
//
// Requires the WRITE_CONTACTS scope.
func (s *CRMService) ImportList(ctx context.Context, params *CRMListImportParams, opts ...RequestOption) (*CRMImportResult, *Response, error) {
	return send[CRMImportResult](ctx, s.client.post, "crm/lists/import", params, opts)
}
