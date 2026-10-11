package warmbly

import (
	"context"
	"net/url"
	"time"
)

// CRM providers returned in [CRMSettings.Provider] and [CRMExternalRef.Provider].
// A workspace runs on Warmbly's own CRM ([CRMProviderNative]) until it
// connects HubSpot or Pipedrive and chooses it with [CRMService.UpdateSettings].
const (
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

// CRM sync job states returned in [CRMSyncJob.Status].
const (
	CRMSyncJobPending = "pending"
	CRMSyncJobRunning = "running"
	CRMSyncJobDone    = "done"
	CRMSyncJobFailed  = "failed"
)

// CRM provider-mode error codes, returned in [Error.Code].
const (
	// ErrCodeCRMNotConnected is the 409 every provider-mode call answers
	// while the workspace runs on Warmbly's own CRM. Connect HubSpot or
	// Pipedrive and choose it with [CRMService.UpdateSettings] first. Some
	// provider calls answer it as a 404 when the mode was switched off.
	ErrCodeCRMNotConnected = "crm_not_connected"
	// ErrCodeCRMManagedExternally is the 409 for editing something the
	// connected CRM owns, such as its pipelines or task types.
	ErrCodeCRMManagedExternally = "crm_managed_externally"
	// ErrCodeCRMReauthRequired is the 409 for a provider connection that was
	// revoked or lacks a permission. Reconnect it under Integrations.
	ErrCodeCRMReauthRequired = "crm_reauth_required"
	// ErrCodeCRMContactMissing is the 404 for a contact the provider does
	// not have yet. [CRMService.LinkContactRecord] finds or creates it.
	ErrCodeCRMContactMissing = "crm_contact_missing"
	// ErrCodeCRMRecordMissing is the 404 for a record deleted in the provider.
	ErrCodeCRMRecordMissing = "crm_record_missing"
	// ErrCodeCRMOptedOut is the 409 for a contact who unsubscribed in the
	// provider.
	ErrCodeCRMOptedOut = "crm_opted_out"
	// ErrCodeCRMOwnerUnmapped is the 400 for an owner id the provider does
	// not know.
	ErrCodeCRMOwnerUnmapped = "crm_owner_unmapped"
	// ErrCodeCRMStageUnknown is a 400 or 409 for a pipeline or stage the
	// provider does not have.
	ErrCodeCRMStageUnknown = "crm_stage_unknown"
	// ErrCodeCRMProviderRejected is the 422 for a change the provider
	// refused. The message carries the provider's reason.
	ErrCodeCRMProviderRejected = "crm_provider_rejected"
	// ErrCodeCRMUnavailable is the 503 for a provider that did not answer.
	// It is transient; retry shortly.
	ErrCodeCRMUnavailable = "crm_unavailable"
	// ErrCodeCRMSyncRunning is the 429 for a sync requested while one has
	// just started.
	ErrCodeCRMSyncRunning = "crm_sync_running"
)

// CRMExternalRef says where a CRM record lives outside Warmbly. It is
// attached to deals, tasks, notes and pipelines read in provider mode.
type CRMExternalRef struct {
	// Provider is [CRMProviderHubSpot] or [CRMProviderPipedrive].
	Provider   string `json:"provider"`
	ExternalID string `json:"external_id"`
	// URL opens the record in the provider's web app.
	URL      string    `json:"url,omitempty"`
	SyncedAt time.Time `json:"synced_at"`
	// OwnerName is the provider owner when that owner is not a workspace
	// member.
	OwnerName string `json:"owner_name,omitempty"`
}

// CRMActivityLog chooses which Warmbly activity is written to the provider's
// timeline. Opens and clicks default off, because machine opens make them
// noise.
type CRMActivityLog struct {
	Sent         bool `json:"sent"`
	Replies      bool `json:"replies"`
	Bounces      bool `json:"bounces"`
	Unsubscribes bool `json:"unsubscribes"`
	Opens        bool `json:"opens"`
	Clicks       bool `json:"clicks"`
	Meetings     bool `json:"meetings"`
}

// CRMReplyOutcome is what a positive ("interested") reply does in the
// provider.
type CRMReplyOutcome struct {
	// LeadStatus is written to the contact's lead status; empty leaves it.
	LeadStatus string `json:"lead_status"`
	// LifecycleStage is written to the contact's lifecycle stage; empty
	// leaves it.
	LifecycleStage string `json:"lifecycle_stage"`
	// CreateDeal opens a deal in DealPipelineID and DealStageID, which are
	// then both required.
	CreateDeal bool `json:"create_deal"`
	// CreateLead adds the person to the Leads Inbox. Pipedrive only.
	CreateLead bool `json:"create_lead,omitempty"`
	// DealPipelineID and DealStageID are local (mirrored) [Pipeline] and
	// [PipelineStage] ids.
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

// CRMEnrollmentGuards skip contacts at import and enrollment time.
type CRMEnrollmentGuards struct {
	SkipLifecycleStages []string `json:"skip_lifecycle_stages"`
	SkipOpenDeals       bool     `json:"skip_open_deals"`
	SkipOtherOwners     bool     `json:"skip_other_owners"`
	SkipOptedOut        bool     `json:"skip_opted_out"`
}

// CRMProviderConfig is every choice the provider setup asks. The defaults
// work without changing anything.
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
	// DealPipelines limits the mirrored deals to these provider pipeline ids.
	// Empty mirrors every pipeline.
	DealPipelines []string `json:"deal_pipelines"`
	// DisplayProperties are extra contact properties shown in the dashboard.
	DisplayProperties []string `json:"display_properties"`
	// FieldMap maps a Warmbly contact field (first_name, last_name, email,
	// company, phone or custom:<key>) to a provider property, and
	// FieldDirection says which side wins per field: [CRMFieldPush],
	// [CRMFieldPull] or [CRMFieldBoth].
	FieldMap       map[string]string `json:"field_map"`
	FieldDirection map[string]string `json:"field_direction"`
	// For is the provider these choices were made for. Empty means HubSpot.
	For string `json:"for,omitempty"`
}

// CRMAccount identifies the connected provider account.
type CRMAccount struct {
	ExternalID string `json:"external_id"`
	Name       string `json:"name"`
	// AppURL is the provider's web app root for this account, for "Open in"
	// links.
	AppURL string `json:"app_url"`
	// Status is one of the Connection* constants.
	Status string `json:"status"`
	// Health is "unknown", "healthy", "degraded" or "down".
	Health string `json:"health"`
	// MissingScopes lists permissions the connection lacks for full provider
	// mode. Reconnecting the integration grants them.
	MissingScopes []string `json:"missing_scopes,omitempty"`
}

// CRMSettings is the workspace's CRM mode.
type CRMSettings struct {
	OrganizationID string `json:"organization_id"`
	// Provider is one of the CRMProvider* constants.
	Provider string `json:"provider"`
	// ConnectionID is the [IntegrationConnection] the provider runs on.
	ConnectionID     *string           `json:"connection_id,omitempty"`
	Config           CRMProviderConfig `json:"config"`
	SetupCompletedAt *time.Time        `json:"setup_completed_at,omitempty"`
	UpdatedAt        time.Time         `json:"updated_at"`
	// Account describes the connected provider account, when there is one.
	Account *CRMAccount `json:"account,omitempty"`
}

// CRMSettingsUpdateParams switches the workspace's CRM or stores its setup
// choices. Nil fields are left unchanged.
type CRMSettingsUpdateParams struct {
	// Provider is one of the CRMProvider* constants.
	Provider *string `json:"provider,omitempty"`
	// ConnectionID picks the integration connection to run the provider on.
	ConnectionID *string `json:"connection_id,omitempty"`
	// Config replaces the setup choices as a whole.
	Config *CRMProviderConfig `json:"config,omitempty"`
	// CompleteSetup stamps the setup as finished.
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

// CRMMetadata is the provider vocabulary a setup picks from.
type CRMMetadata struct {
	LifecycleStages []CRMOption   `json:"lifecycle_stages"`
	LeadStatuses    []CRMOption   `json:"lead_statuses"`
	TaskTypes       []CRMOption   `json:"task_types"`
	Properties      []CRMProperty `json:"properties"`
	Pipelines       []CRMOption   `json:"pipelines"`
}

// CRMOwner is a provider user who can own records, and the workspace member
// it maps to.
type CRMOwner struct {
	ExternalID string `json:"external_id"`
	Email      string `json:"email"`
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
	// UserID is the matched workspace member, matched by address unless
	// UserPinned says it was set with [CRMService.MapOwner].
	UserID     *string `json:"user_id,omitempty"`
	UserPinned bool    `json:"user_pinned"`
	Archived   bool    `json:"archived"`
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

// CRMContactView is the provider side of one Warmbly contact: its owner,
// lifecycle stage, lead status, company and the chosen display properties.
type CRMContactView struct {
	// Provider is one of the CRMProvider* constants.
	Provider string `json:"provider"`
	// Linked reports whether the contact exists in the provider. The other
	// fields are empty until it does.
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

// CRMContactUpdateParams edits a contact's sales fields in the provider. Nil
// fields are left unchanged; each value is at most 100 characters.
type CRMContactUpdateParams struct {
	OwnerExternalID *string `json:"owner_external_id,omitempty"`
	LifecycleStage  *string `json:"lifecycle_stage,omitempty"`
	LeadStatus      *string `json:"lead_status,omitempty"`
}

// CRMSyncJob is one queued write to the provider.
type CRMSyncJob struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	// Provider is one of the CRMProvider* constants.
	Provider string `json:"provider"`
	// Kind is what the job does, for example "log_email" or "push_deal".
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	// Status is one of the CRMSyncJob* constants.
	Status        string     `json:"status"`
	Attempts      int        `json:"attempts"`
	NextAttemptAt time.Time  `json:"next_attempt_at"`
	LastError     string     `json:"last_error,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
}

// CRMSyncCursor reports one incremental pull from the provider.
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

// CRMSyncHealth is what is waiting, what failed and when each pull ran.
type CRMSyncHealth struct {
	Pending      int64           `json:"pending"`
	Failed       int64           `json:"failed"`
	Done24h      int64           `json:"done_24h"`
	LastSyncedAt *time.Time      `json:"last_synced_at,omitempty"`
	Cursors      []CRMSyncCursor `json:"cursors"`
	Failures     []CRMSyncJob    `json:"failures"`
	Counts       CRMMirrorCounts `json:"counts"`
}

// CRMList is a provider contact list Warmbly can import from.
type CRMList struct {
	ExternalID string `json:"external_id"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	// Dynamic reports a list the provider keeps current from a filter.
	Dynamic bool `json:"dynamic"`
	// UpdatedAt is the provider's own timestamp, passed through as text.
	UpdatedAt string `json:"updated_at,omitempty"`
}

// CRMListParams filters and paginates the provider's lists. Limit is between
// 1 and 100 and defaults to 25.
type CRMListParams struct {
	ListOptions
	// Query matches the list name; at most 200 characters.
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

// CRMListImportParams names a provider list to preview or import.
type CRMListImportParams struct {
	// ListID is a [CRMList.ExternalID] and is required.
	ListID string `json:"list_id"`
	// ApplyGuards drops contacts the workspace's [CRMEnrollmentGuards]
	// exclude.
	ApplyGuards bool `json:"apply_guards"`
}

// CRMImportSkip is one skip reason and how many contacts it removed.
type CRMImportSkip struct {
	Reason string `json:"reason"`
	Label  string `json:"label"`
	Count  int    `json:"count"`
}

// CRMImportPerson is one sample row of a list preview.
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

// CRMImportResult names the contact import draft a list import produced.
// Finish it like any other contact import.
type CRMImportResult struct {
	ImportID string           `json:"import_id"`
	Preview  CRMImportPreview `json:"preview"`
}

// CRMBackfillParams chooses what Warmbly's own CRM copies into the provider
// once, when a workspace switches.
type CRMBackfillParams struct {
	Deals bool `json:"deals"`
	Tasks bool `json:"tasks"`
	Notes bool `json:"notes"`
}

// CRMBackfillPreview counts the Warmbly-only records a backfill would copy.
type CRMBackfillPreview struct {
	Deals int64 `json:"deals"`
	Tasks int64 `json:"tasks"`
	Notes int64 `json:"notes"`
}

// --- settings ---

// GetSettings returns the workspace's CRM mode and the connected account. It
// answers on Warmbly's own CRM too, with Provider [CRMProviderNative].
//
// Requires the READ_CRM API permission.
func (s *CRMService) GetSettings(ctx context.Context, opts ...RequestOption) (*CRMSettings, *Response, error) {
	return fetch[CRMSettings](ctx, s.client, "crm/settings", opts)
}

// UpdateSettings switches the workspace's CRM and stores the setup choices.
// Switching away from a provider stops its sync; the records mirrored from it
// stay.
//
// Requires the INTEGRATIONS API permission.
func (s *CRMService) UpdateSettings(ctx context.Context, params *CRMSettingsUpdateParams, opts ...RequestOption) (*CRMSettings, *Response, error) {
	return send[CRMSettings](ctx, s.client.put, "crm/settings", params, opts)
}

// GetMetadata returns the provider's lifecycle stages, lead statuses, task
// types, contact properties and pipelines.
//
// Requires the READ_CRM API permission.
func (s *CRMService) GetMetadata(ctx context.Context, opts ...RequestOption) (*CRMMetadata, *Response, error) {
	return fetch[CRMMetadata](ctx, s.client, "crm/metadata", opts)
}

// ListOwners returns the provider's owners and the member each one maps to.
//
// Requires the READ_CRM API permission.
func (s *CRMService) ListOwners(ctx context.Context, opts ...RequestOption) ([]CRMOwner, *Response, error) {
	return fetchData[CRMOwner](ctx, s.client, "crm/owners", opts)
}

// MapOwner pins a provider owner to a workspace member. A nil userID clears
// the pin, so the owner is matched by address again.
//
// Requires the INTEGRATIONS API permission.
func (s *CRMService) MapOwner(ctx context.Context, externalID string, userID *string, opts ...RequestOption) (*Response, error) {
	body := struct {
		UserID *string `json:"user_id"`
	}{UserID: userID}
	return s.client.put(ctx, "crm/owners/"+url.PathEscape(externalID), body, nil, opts...)
}

// --- sync ---

// GetSyncHealth reports what is waiting, what failed and when each pull ran.
//
// Requires the READ_CRM API permission.
func (s *CRMService) GetSyncHealth(ctx context.Context, opts ...RequestOption) (*CRMSyncHealth, *Response, error) {
	return fetch[CRMSyncHealth](ctx, s.client, "crm/sync", opts)
}

// SyncNow starts a full pull from the provider. It answers 202 at once; the
// pull runs in the background and [CRMService.GetSyncHealth] reports it.
//
// Requires the INTEGRATIONS API permission.
func (s *CRMService) SyncNow(ctx context.Context, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "crm/sync", nil, nil, opts...)
}

// RetrySyncFailures requeues failed sync jobs: the given ids, or every failed
// job when ids is empty. At most 500 ids per call. It returns how many jobs
// were requeued; requeueing a job twice changes nothing.
//
// Requires the INTEGRATIONS API permission.
func (s *CRMService) RetrySyncFailures(ctx context.Context, ids []string, opts ...RequestOption) (int64, *Response, error) {
	return s.syncFailures(ctx, "crm/sync/retry", ids, opts)
}

// DiscardSyncFailures drops failed sync jobs: the given ids, or every failed
// job when ids is empty. At most 500 ids per call. It returns how many jobs
// were dropped.
//
// Requires the INTEGRATIONS API permission.
func (s *CRMService) DiscardSyncFailures(ctx context.Context, ids []string, opts ...RequestOption) (int64, *Response, error) {
	return s.syncFailures(ctx, "crm/sync/discard", ids, opts)
}

func (s *CRMService) syncFailures(ctx context.Context, path string, ids []string, opts []RequestOption) (int64, *Response, error) {
	body := struct {
		IDs []string `json:"ids,omitempty"`
	}{IDs: ids}
	var out struct {
		Affected int64 `json:"affected"`
	}
	resp, err := s.client.post(ctx, path, body, &out, opts...)
	if err != nil {
		return 0, resp, err
	}
	return out.Affected, resp, nil
}

// GetBackfill counts the Warmbly-only deals, tasks and notes a backfill would
// copy into the provider.
//
// Requires the INTEGRATIONS API permission.
func (s *CRMService) GetBackfill(ctx context.Context, opts ...RequestOption) (*CRMBackfillPreview, *Response, error) {
	return fetch[CRMBackfillPreview](ctx, s.client, "crm/backfill", opts)
}

// StartBackfill copies Warmbly's own deals, tasks and notes into the provider
// once. It answers 202 at once. It is idempotent: a second call while one
// runs is folded into it, and copied records are never copied twice.
//
// Requires the INTEGRATIONS API permission.
func (s *CRMService) StartBackfill(ctx context.Context, params *CRMBackfillParams, opts ...RequestOption) (*Response, error) {
	if params == nil {
		params = &CRMBackfillParams{}
	}
	return s.client.post(ctx, "crm/backfill", params, nil, opts...)
}

// --- contact records ---

func crmContactPath(contactID string) string {
	return "crm/contacts/" + url.PathEscape(contactID)
}

// GetContactRecord returns the provider side of a Warmbly contact.
//
// Requires the READ_CRM API permission.
func (s *CRMService) GetContactRecord(ctx context.Context, contactID string, opts ...RequestOption) (*CRMContactView, *Response, error) {
	return fetch[CRMContactView](ctx, s.client, crmContactPath(contactID), opts)
}

// RefreshContactRecord pulls a contact's provider record, deals, tasks and
// notes now. It is debounced per contact, so calling it often is safe.
//
// Requires the READ_CRM API permission.
func (s *CRMService) RefreshContactRecord(ctx context.Context, contactID string, opts ...RequestOption) (*CRMContactView, *Response, error) {
	return send[CRMContactView](ctx, s.client.post, crmContactPath(contactID)+"/refresh", nil, opts)
}

// LinkContactRecord finds the contact in the provider, or creates it there.
//
// Requires the WRITE_CRM API permission.
func (s *CRMService) LinkContactRecord(ctx context.Context, contactID string, opts ...RequestOption) (*CRMContactView, *Response, error) {
	return send[CRMContactView](ctx, s.client.post, crmContactPath(contactID)+"/link", nil, opts)
}

// UpdateContactRecord edits a contact's owner, lifecycle stage or lead status
// in the provider.
//
// Requires the WRITE_CRM API permission.
func (s *CRMService) UpdateContactRecord(ctx context.Context, contactID string, params *CRMContactUpdateParams, opts ...RequestOption) (*CRMContactView, *Response, error) {
	return send[CRMContactView](ctx, s.client.patch, crmContactPath(contactID), params, opts)
}

// --- list import ---

// ListLists returns a page of the provider's contact lists.
//
// Requires the READ_CRM API permission.
func (s *CRMService) ListLists(ctx context.Context, params *CRMListParams, opts ...RequestOption) (*Page[CRMList], error) {
	return listJSON[CRMList](ctx, s.client, "crm/lists", params.values(), opts...)
}

// PreviewListImport counts who importing a provider list brings in and who it
// skips, with a sample of the people. It imports nothing.
//
// Requires the WRITE_CONTACTS API permission.
func (s *CRMService) PreviewListImport(ctx context.Context, params *CRMListImportParams, opts ...RequestOption) (*CRMImportPreview, *Response, error) {
	return send[CRMImportPreview](ctx, s.client.post, "crm/lists/preview", params, opts)
}

// ImportList turns a provider list into a contact import draft and returns
// its id. The credential must act as a workspace member, or the call answers
// 401.
//
// Requires the WRITE_CONTACTS API permission.
func (s *CRMService) ImportList(ctx context.Context, params *CRMListImportParams, opts ...RequestOption) (*CRMImportResult, *Response, error) {
	return send[CRMImportResult](ctx, s.client.post, "crm/lists/import", params, opts)
}
