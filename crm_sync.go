package warmbly

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// A workspace runs its CRM on Warmbly's own pipelines and deals, or on a
// connected HubSpot or Pipedrive account. The methods below configure that
// choice and read the connected side: the owners, the sync queue, the mirrored
// contact records and the provider's contact lists. They answer 501 when the
// deployment has no CRM provider support.
//
// Reading needs [PermReadCRM]. Changing settings, owner mappings or the sync
// queue, and starting a backfill, need [PermIntegrations]. Linking or editing a
// contact's provider record needs [PermWriteCRM]. Importing a provider list
// needs [PermWriteContacts].

// CRM providers returned in [CRMSettings.Provider].
const (
	// CRMProviderNative is Warmbly's own CRM.
	CRMProviderNative = "native"
	// CRMProviderHubSpot runs the CRM on a connected HubSpot account.
	CRMProviderHubSpot = "hubspot"
	// CRMProviderPipedrive runs the CRM on a connected Pipedrive account.
	CRMProviderPipedrive = "pipedrive"
)

// CRMExternalRef says where a CRM record lives outside Warmbly. It is set on
// pipelines, deals and tasks read while a connected CRM is the CRM.
type CRMExternalRef struct {
	// Provider is one of the CRMProvider* constants.
	Provider   string    `json:"provider"`
	ExternalID string    `json:"external_id"`
	URL        string    `json:"url,omitempty"`
	SyncedAt   time.Time `json:"synced_at"`
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

// CRMReplyOutcome is what a positive reply does in the provider.
type CRMReplyOutcome struct {
	// LeadStatus is written to the contact's lead status. Empty leaves it.
	LeadStatus string `json:"lead_status"`
	// LifecycleStage is written to the contact's lifecycle stage. Empty
	// leaves it.
	LifecycleStage string `json:"lifecycle_stage"`
	CreateDeal     bool   `json:"create_deal"`
	// CreateLead adds the person to Pipedrive's Leads Inbox (Pipedrive only).
	CreateLead bool `json:"create_lead,omitempty"`
	// DealPipelineID and DealStageID are Warmbly (mirrored) ids. They are
	// required when CreateDeal is set.
	DealPipelineID *string `json:"deal_pipeline_id,omitempty"`
	DealStageID    *string `json:"deal_stage_id,omitempty"`
}

// CRMExitRules stop a contact's campaigns when the provider says they have
// moved on.
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

// CRM field directions in [CRMConfig.FieldDirection]. They match [SyncPush],
// [SyncPull] and [SyncBoth], where the most recent change wins for both.
const (
	CRMFieldPush = SyncPush
	CRMFieldPull = SyncPull
	CRMFieldBoth = SyncBoth
)

// CRMConfig is every choice the CRM setup wizard asks. It is saved whole, so
// read it first and send back every part you do not mean to change.
type CRMConfig struct {
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
	// FieldMap maps a Warmbly contact field ("first_name", "last_name",
	// "email", "company", "phone" or "custom:<key>") to a provider property,
	// and FieldDirection says which side wins per field with one of the
	// CRMField* values.
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
	Status string `json:"status"`
	Health string `json:"health"`
	// MissingScopes lists permissions the connection lacks for full mode, so
	// the caller can ask for a reconnect.
	MissingScopes []string `json:"missing_scopes,omitempty"`
}

// CRMSettings is the workspace's CRM mode.
type CRMSettings struct {
	OrganizationID string `json:"organization_id"`
	// Provider is one of the CRMProvider* constants.
	Provider     string    `json:"provider"`
	ConnectionID *string   `json:"connection_id,omitempty"`
	Config       CRMConfig `json:"config"`
	// SetupCompletedAt is set once the setup wizard has finished.
	SetupCompletedAt *time.Time `json:"setup_completed_at,omitempty"`
	UpdatedAt        time.Time  `json:"updated_at"`
	// Account describes the connected provider account, when there is one.
	Account *CRMAccount `json:"account,omitempty"`
}

// CRMSettingsUpdate switches the CRM and stores the setup choices. Nil fields
// are left unchanged.
type CRMSettingsUpdate struct {
	// Provider is one of the CRMProvider* constants.
	Provider *string `json:"provider,omitempty"`
	// ConnectionID is the HubSpot or Pipedrive connection to run on.
	ConnectionID *string `json:"connection_id,omitempty"`
	// Config replaces the setup choices as a whole.
	Config *CRMConfig `json:"config,omitempty"`
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

// CRMMetadata is the provider vocabulary the pickers render.
type CRMMetadata struct {
	LifecycleStages []CRMOption   `json:"lifecycle_stages"`
	LeadStatuses    []CRMOption   `json:"lead_statuses"`
	TaskTypes       []CRMOption   `json:"task_types"`
	Properties      []CRMProperty `json:"properties"`
	Pipelines       []CRMOption   `json:"pipelines"`
}

// CRMOwner is a provider user who can own records.
type CRMOwner struct {
	ExternalID string `json:"external_id"`
	Email      string `json:"email"`
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
	// UserID is the workspace member this owner is matched to, or nil.
	UserID *string `json:"user_id,omitempty"`
	// UserPinned reports that the match was set by hand rather than by email.
	UserPinned bool `json:"user_pinned"`
	Archived   bool `json:"archived"`
}

// CRM sync job states returned in [CRMSyncJob.Status].
const (
	CRMJobPending = "pending"
	CRMJobRunning = "running"
	CRMJobDone    = "done"
	CRMJobFailed  = "failed"
)

// CRMSyncJob is one queued write to the provider.
type CRMSyncJob struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	// Provider is one of the CRMProvider* constants.
	Provider string `json:"provider"`
	// Kind is what the job does, for example "log_email", "push_deal" or
	// "backfill".
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
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

// CRMSyncHealth is the sync health panel: counts, failures and freshness.
type CRMSyncHealth struct {
	Pending      int64           `json:"pending"`
	Failed       int64           `json:"failed"`
	Done24h      int64           `json:"done_24h"`
	LastSyncedAt *time.Time      `json:"last_synced_at,omitempty"`
	Cursors      []CRMSyncCursor `json:"cursors"`
	Failures     []CRMSyncJob    `json:"failures"`
	Counts       CRMMirrorCounts `json:"counts"`
}

// CRMBackfillRequest chooses which Warmbly-only records are copied into the
// provider.
type CRMBackfillRequest struct {
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

// CRMContactView is the provider side of a Warmbly contact: owner, lifecycle
// stage, lead status, company and the chosen properties.
type CRMContactView struct {
	// Provider is one of the CRMProvider* constants.
	Provider string `json:"provider"`
	// Linked reports that the contact exists in the provider.
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

// CRMContactUpdate edits a contact's provider record in place. Nil fields are
// left unchanged. Each value is at most 100 characters.
type CRMContactUpdate struct {
	// OwnerExternalID is the provider owner id from [CRMService.ListOwners].
	OwnerExternalID *string `json:"owner_external_id,omitempty"`
	LifecycleStage  *string `json:"lifecycle_stage,omitempty"`
	LeadStatus      *string `json:"lead_status,omitempty"`
}

// CRMList is a provider contact list Warmbly can import from.
type CRMList struct {
	ExternalID string `json:"external_id"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	// Dynamic reports a list the provider fills by rule.
	Dynamic   bool   `json:"dynamic"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// CRMListParams filters and paginates the provider's contact lists.
type CRMListParams struct {
	// ListOptions limit is 1 to 100; the server default is 25. The cursor is
	// the provider's own and is opaque.
	ListOptions
	// Query narrows lists by name. At most 200 characters.
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

// CRMImportParams names a provider list for a preview or an import.
type CRMImportParams struct {
	// ListID is the list's external id from [CRMList.ExternalID].
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

// CRMImportPerson is a sample row of the preview.
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
	// Truncated reports that the list is larger than one import takes.
	Truncated bool `json:"truncated"`
}

// CRMImportResult names the contact import draft a list import produced. The
// draft is finished in the regular import review.
type CRMImportResult struct {
	ImportID string           `json:"import_id"`
	Preview  CRMImportPreview `json:"preview"`
}

// Settings returns the workspace's CRM mode and the connected account.
func (s *CRMService) Settings(ctx context.Context, opts ...RequestOption) (*CRMSettings, *Response, error) {
	return fetch[CRMSettings](ctx, s.client, "crm/settings", opts)
}

// UpdateSettings switches the CRM and stores the setup choices, then returns
// the saved settings. It needs [PermIntegrations].
func (s *CRMService) UpdateSettings(ctx context.Context, params *CRMSettingsUpdate, opts ...RequestOption) (*CRMSettings, *Response, error) {
	return send[CRMSettings](ctx, s.client.put, "crm/settings", params, opts)
}

// Metadata returns the provider's lifecycle stages, lead statuses, task types,
// contact properties and pipelines for the pickers.
func (s *CRMService) Metadata(ctx context.Context, opts ...RequestOption) (*CRMMetadata, *Response, error) {
	return fetch[CRMMetadata](ctx, s.client, "crm/metadata", opts)
}

// ListOwners returns the provider's owners and the workspace member each one
// is matched to.
func (s *CRMService) ListOwners(ctx context.Context, opts ...RequestOption) ([]CRMOwner, *Response, error) {
	return fetchData[CRMOwner](ctx, s.client, "crm/owners", opts)
}

// MapOwner pins a provider owner to a workspace member. A nil userID clears the
// match. It needs [PermIntegrations].
func (s *CRMService) MapOwner(ctx context.Context, externalID string, userID *string, opts ...RequestOption) (*Response, error) {
	body := struct {
		UserID *string `json:"user_id"`
	}{UserID: userID}
	return s.client.put(ctx, "crm/owners/"+url.PathEscape(externalID), body, nil, opts...)
}

// SyncHealth reports what is waiting, what failed and when each pull ran.
func (s *CRMService) SyncHealth(ctx context.Context, opts ...RequestOption) (*CRMSyncHealth, *Response, error) {
	return fetch[CRMSyncHealth](ctx, s.client, "crm/sync", opts)
}

// SyncNow starts a full pull from the provider. The server answers 202 and
// runs the pull in the background; watch [CRMService.SyncHealth] for progress.
// It needs [PermIntegrations].
func (s *CRMService) SyncNow(ctx context.Context, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "crm/sync", nil, nil, opts...)
}

// crmJobIDs is the body of the retry and discard calls.
type crmJobIDs struct {
	IDs []string `json:"ids,omitempty"`
}

// RetrySyncFailures requeues failed sync jobs and returns how many it
// requeued. Pass no ids to requeue every failed job, or at most 500. It needs
// [PermIntegrations].
func (s *CRMService) RetrySyncFailures(ctx context.Context, ids []string, opts ...RequestOption) (int64, *Response, error) {
	return s.affect(ctx, "crm/sync/retry", crmJobIDs{IDs: ids}, opts)
}

// DiscardSyncFailures drops failed sync jobs and returns how many it dropped.
// Pass no ids to drop every failed job, or at most 500. It needs
// [PermIntegrations].
func (s *CRMService) DiscardSyncFailures(ctx context.Context, ids []string, opts ...RequestOption) (int64, *Response, error) {
	return s.affect(ctx, "crm/sync/discard", crmJobIDs{IDs: ids}, opts)
}

func (s *CRMService) affect(ctx context.Context, path string, body any, opts []RequestOption) (int64, *Response, error) {
	var out struct {
		Affected int64 `json:"affected"`
	}
	resp, err := s.client.post(ctx, path, body, &out, opts...)
	if err != nil {
		return 0, resp, err
	}
	return out.Affected, resp, nil
}

// BackfillPreview counts the Warmbly-only deals, tasks and notes a switch to a
// connected CRM would copy. It needs [PermIntegrations].
func (s *CRMService) BackfillPreview(ctx context.Context, opts ...RequestOption) (*CRMBackfillPreview, *Response, error) {
	return fetch[CRMBackfillPreview](ctx, s.client, "crm/backfill", opts)
}

// StartBackfill copies Warmbly's own deals, tasks and notes into the provider
// once. The server answers 202 and runs it in the background. A second call
// while one runs is folded into it, and copied records are never copied twice.
// It needs [PermIntegrations].
func (s *CRMService) StartBackfill(ctx context.Context, params *CRMBackfillRequest, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "crm/backfill", params, nil, opts...)
}

// Contact returns the provider side of a contact.
func (s *CRMService) Contact(ctx context.Context, contactID string, opts ...RequestOption) (*CRMContactView, *Response, error) {
	return fetch[CRMContactView](ctx, s.client, "crm/contacts/"+url.PathEscape(contactID), opts)
}

// RefreshContact pulls a contact's provider record, deals, tasks and notes now.
// The server debounces it per contact, so it is safe to call whenever a panel
// opens.
func (s *CRMService) RefreshContact(ctx context.Context, contactID string, opts ...RequestOption) (*CRMContactView, *Response, error) {
	return send[CRMContactView](ctx, s.client.post, "crm/contacts/"+url.PathEscape(contactID)+"/refresh", nil, opts)
}

// LinkContact finds the contact in the provider, or creates it. It needs
// [PermWriteCRM].
func (s *CRMService) LinkContact(ctx context.Context, contactID string, opts ...RequestOption) (*CRMContactView, *Response, error) {
	return send[CRMContactView](ctx, s.client.post, "crm/contacts/"+url.PathEscape(contactID)+"/link", nil, opts)
}

// UpdateContact edits a contact's owner, lifecycle stage or lead status in the
// provider. It needs [PermWriteCRM].
func (s *CRMService) UpdateContact(ctx context.Context, contactID string, params *CRMContactUpdate, opts ...RequestOption) (*CRMContactView, *Response, error) {
	return send[CRMContactView](ctx, s.client.patch, "crm/contacts/"+url.PathEscape(contactID), params, opts)
}

// ListLists returns a page of the provider's contact lists for import. It needs
// [PermReadCRM], and a session member also needs to manage contacts.
func (s *CRMService) ListLists(ctx context.Context, params *CRMListParams, opts ...RequestOption) (*Page[CRMList], error) {
	return listJSON[CRMList](ctx, s.client, "crm/lists", params.values(), opts...)
}

// PreviewImport counts who importing a provider list would bring in and who it
// would skip. It needs [PermWriteContacts].
func (s *CRMService) PreviewImport(ctx context.Context, params *CRMImportParams, opts ...RequestOption) (*CRMImportPreview, *Response, error) {
	return send[CRMImportPreview](ctx, s.client.post, "crm/lists/preview", params, opts)
}

// ImportList turns a provider list into a contact import draft and answers 201
// with its id. The draft is finished in the regular import review. It needs
// [PermWriteContacts] and a signed-in member, so an API key is refused.
func (s *CRMService) ImportList(ctx context.Context, params *CRMImportParams, opts ...RequestOption) (*CRMImportResult, *Response, error) {
	return send[CRMImportResult](ctx, s.client.post, "crm/lists/import", params, opts)
}

// --- bulk task actions ---

// CRMTaskSelection names the tasks a bulk action applies to: either an explicit
// id list, or every task matching Filters minus the ids in Exclude. A selection
// that names both prefers the filter. A filter selection may cover at most
// 50000 tasks.
type CRMTaskSelection struct {
	// Tasks is an explicit list of task ids.
	Tasks []string `json:"tasks"`
	// All switches the selection from Tasks to Filters.
	All bool `json:"all,omitempty"`
	// Filters is the same body [CRMService.SearchTasks] takes.
	Filters *TaskSearchParams `json:"filters,omitempty"`
	// Exclude drops ids from the resolved set. It is ignored unless All is set.
	Exclude []string `json:"exclude,omitempty"`
}

// CRMTaskBulkUpdate writes one status or priority onto a selection of tasks. At
// least one of Status and Priority is required.
type CRMTaskBulkUpdate struct {
	CRMTaskSelection
	// Status is one of the TaskStatus* constants.
	Status *string `json:"status,omitempty"`
	// Priority is one of the TaskPriority* constants.
	Priority *string `json:"priority,omitempty"`
}

// BulkUpdateTasks writes a status or priority onto a selection of tasks and
// returns how many it changed. It needs [PermWriteCRM].
func (s *CRMService) BulkUpdateTasks(ctx context.Context, params *CRMTaskBulkUpdate, opts ...RequestOption) (int64, *Response, error) {
	var out struct {
		Affected int64 `json:"affected"`
	}
	resp, err := s.client.patch(ctx, "crm/tasks", params, &out, opts...)
	if err != nil {
		return 0, resp, err
	}
	return out.Affected, resp, nil
}

// BulkDeleteTasks deletes a selection of tasks and returns how many it
// deleted. It needs [PermWriteCRM].
func (s *CRMService) BulkDeleteTasks(ctx context.Context, selection *CRMTaskSelection, opts ...RequestOption) (int64, *Response, error) {
	var out struct {
		Affected int64 `json:"affected"`
	}
	resp, err := s.client.call(ctx, http.MethodDelete, "crm/tasks", selection, &out, opts)
	if err != nil {
		return 0, resp, err
	}
	return out.Affected, resp, nil
}
