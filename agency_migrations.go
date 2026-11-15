package warmbly

import (
	"context"
	"net/url"
	"time"
)

// Where an agency migration stands, in [AgencyMigration.Status].
const (
	// AgencyMigrationScanning reads the source account to show what it holds.
	AgencyMigrationScanning = "scanning"
	// AgencyMigrationReady has a preview and waits for
	// [AgencyService.StartMigration].
	AgencyMigrationReady = "ready"
	// AgencyMigrationRunning is bringing data over.
	AgencyMigrationRunning = "running"
	// AgencyMigrationCompleted finished.
	AgencyMigrationCompleted = "completed"
	// AgencyMigrationFailed stopped on an error; see [AgencyMigration.Error].
	AgencyMigrationFailed = "failed"
	// AgencyMigrationCanceled was canceled.
	AgencyMigrationCanceled = "canceled"
)

// What happens to a lead the old tool was part way through a sequence with,
// in [AgencyMigrationOptions.InFlight].
const (
	// AgencyMigrationRestart enrolls the lead from the first step.
	AgencyMigrationRestart = "restart"
	// AgencyMigrationSkip imports the contact without enrolling it.
	AgencyMigrationSkip = "skip"
)

// Kinds of data a migration can bring over, in
// [AgencyMigrationOptions.Kinds].
const (
	// AgencyMigrationKindBlocklist is the source's blocklist. Applied first.
	AgencyMigrationKindBlocklist = "blocklist"
	// AgencyMigrationKindMailboxes is the sending mailboxes.
	AgencyMigrationKindMailboxes = "mailboxes"
	// AgencyMigrationKindCampaigns is the campaigns, created as drafts.
	AgencyMigrationKindCampaigns = "campaigns"
	// AgencyMigrationKindLeads is the campaigns' leads.
	AgencyMigrationKindLeads = "leads"
)

// AgencyMigrationProvider is one outreach tool the agency can migrate from.
type AgencyMigrationProvider struct {
	// ID is the provider identifier, for example "smartlead".
	ID    string `json:"id"`
	Label string `json:"label"`
	// Fields are the credentials, in the order they are asked.
	Fields []AgencyMigrationField `json:"fields"`
	// KeyHelp tells a person where the key lives in that tool.
	KeyHelp string `json:"key_help"`
	KeyURL  string `json:"key_url,omitempty"`
	// Kinds are what the provider's API can read.
	Kinds []string `json:"kinds"`
	// Workspaces is true when one key sees several client accounts, which an
	// agency maps onto client workspaces.
	Workspaces bool `json:"workspaces"`
	// MultiKey is true when the tool issues one key per workspace, so an
	// agency adds a credential set for each.
	MultiKey bool `json:"multi_key"`
	// MailboxSecrets is true when the API returns mailbox passwords, so SMTP
	// and IMAP mailboxes connect without being asked for again.
	MailboxSecrets bool     `json:"mailbox_secrets"`
	Notes          []string `json:"notes,omitempty"`
}

// AgencyMigrationField is one credential input a provider needs.
type AgencyMigrationField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Help        string `json:"help,omitempty"`
	Secret      bool   `json:"secret"`
	Optional    bool   `json:"optional,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	// Options limits the value to a fixed set, such as a data region.
	Options []string `json:"options,omitempty"`
}

// AgencyMigration is one import from another outreach tool into the agency's
// client workspaces. It is the same shape the workspace migrations endpoints
// return, with AgencyID set.
type AgencyMigration struct {
	ID              string  `json:"id"`
	OrganizationID  *string `json:"organization_id,omitempty"`
	AgencyID        *string `json:"agency_id,omitempty"`
	CreatedByUserID *string `json:"created_by_user_id,omitempty"`
	Provider        string  `json:"provider"`
	Label           string  `json:"label"`
	// Status is one of the AgencyMigration* status constants.
	Status string `json:"status"`
	// Accounts are who each credential set acts as, in the order added.
	Accounts []AgencyMigrationAccount `json:"accounts"`
	// Inventory is what the scan found, one entry per source workspace.
	Inventory []AgencyMigrationInventory `json:"inventory"`
	Options   *AgencyMigrationOptions    `json:"options,omitempty"`
	Targets   []AgencyMigrationTarget    `json:"targets"`
	Error     string                     `json:"error,omitempty"`
	// CredentialsHeld is false once the keys have been wiped, which happens
	// when the migration finishes, is canceled or is dismissed.
	CredentialsHeld bool       `json:"credentials_held"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// AgencyMigrationAccount is who one credential set acts as in the source.
type AgencyMigrationAccount struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	Plan  string `json:"plan,omitempty"`
}

// AgencyMigrationSourceWorkspace is one workspace or client account in the
// source tool.
type AgencyMigrationSourceWorkspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Key is the index of the credential set that sees it.
	Key int `json:"key,omitempty"`
}

// AgencyMigrationInventory is one source workspace and what it holds.
type AgencyMigrationInventory struct {
	Workspace AgencyMigrationSourceWorkspace `json:"workspace"`
	Inventory AgencyMigrationCounts          `json:"inventory"`
	Error     string                         `json:"error,omitempty"`
}

// AgencyMigrationCounts is what a source workspace holds.
type AgencyMigrationCounts struct {
	Mailboxes int                             `json:"mailboxes"`
	Campaigns int                             `json:"campaigns"`
	Leads     int                             `json:"leads"`
	Blocklist int                             `json:"blocklist"`
	Summaries []AgencyMigrationCampaignSource `json:"campaign_summaries,omitempty"`
}

// AgencyMigrationCampaignSource is one campaign in the source tool.
type AgencyMigrationCampaignSource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Status is "draft", "active", "paused", "completed" or "archived".
	Status string `json:"status"`
	Steps  int    `json:"steps"`
	Leads  int    `json:"leads"`
}

// AgencyMigrationOptions are the choices made before a migration starts.
type AgencyMigrationOptions struct {
	// Kinds are the AgencyMigrationKind* constants to bring over.
	Kinds []string `json:"kinds"`
	// CampaignIDs limits campaigns (and their leads) to these source IDs;
	// empty brings every campaign.
	CampaignIDs []string `json:"campaign_ids,omitempty"`
	// InFlight is [AgencyMigrationRestart] or [AgencyMigrationSkip].
	InFlight string `json:"in_flight"`
	// SkipFinished leaves out campaigns the source marks completed or
	// archived.
	SkipFinished bool `json:"skip_finished"`
	// Warmup starts warmup on mailboxes that connect, unless the source said
	// off.
	Warmup bool `json:"warmup"`
}

// AgencyMigrationTarget is one source workspace brought into one client
// workspace.
type AgencyMigrationTarget struct {
	ID               string                         `json:"id"`
	MigrationID      string                         `json:"migration_id"`
	Workspace        AgencyMigrationSourceWorkspace `json:"workspace"`
	OrganizationID   string                         `json:"organization_id"`
	OrganizationName string                         `json:"organization_name,omitempty"`
	// Status is "pending", "running", "completed", "failed" or "canceled".
	Status string `json:"status"`
	// Phase is the kind being imported now; empty before the first.
	Phase    string                  `json:"phase,omitempty"`
	Counts   AgencyMigrationProgress `json:"counts"`
	Followup AgencyMigrationFollowup `json:"followup"`
	Error    string                  `json:"error,omitempty"`
	// MailboxImportID is the mailbox import that connects mailboxes the
	// source returned passwords for.
	MailboxImportID *string `json:"mailbox_import_id,omitempty"`
	// CategoryID is the contact category every imported contact is filed
	// under.
	CategoryID *string    `json:"category_id,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// AgencyMigrationProgress is what one target brought over.
type AgencyMigrationProgress struct {
	BlocklistImported int `json:"blocklist_imported"`
	MailboxesQueued   int `json:"mailboxes_queued"`
	MailboxesManual   int `json:"mailboxes_manual"`
	CampaignsCreated  int `json:"campaigns_created"`
	CampaignsSkipped  int `json:"campaigns_skipped"`
	StepsCreated      int `json:"steps_created"`
	LeadsRead         int `json:"leads_read"`
	ContactsImported  int `json:"contacts_imported"`
	ContactsUpdated   int `json:"contacts_updated"`
	LeadsEnrolled     int `json:"leads_enrolled"`
	LeadsNotEnrolled  int `json:"leads_not_enrolled"`
	LeadsSuppressed   int `json:"leads_suppressed"`
	LeadsFailed       int `json:"leads_failed"`
}

// AgencyMigrationFollowup is what a person still has to do after a target
// ran.
type AgencyMigrationFollowup struct {
	// Reconnect are mailboxes that need connecting by their owner.
	Reconnect []AgencyMigrationMailbox `json:"reconnect,omitempty"`
	// Notes are things that did not carry over, such as LinkedIn steps.
	Notes []string `json:"notes,omitempty"`
	// Campaigns are the draft campaigns created, for review before launch.
	Campaigns []AgencyMigrationCampaign `json:"campaigns,omitempty"`
}

// AgencyMigrationMailbox is a mailbox to reconnect.
type AgencyMigrationMailbox struct {
	Email string `json:"email"`
	// Kind is "google", "microsoft", "smtp_imap" or "unknown".
	Kind string `json:"kind"`
}

// AgencyMigrationCampaign is one draft campaign a migration created.
type AgencyMigrationCampaign struct {
	SourceID   string `json:"source_id"`
	CampaignID string `json:"campaign_id"`
	Name       string `json:"name"`
	// MissingSenders are source sender addresses with no mailbox in the
	// workspace yet; [AgencyService.LinkMigrationSenders] links them once
	// they are connected.
	MissingSenders []string `json:"missing_senders,omitempty"`
}

// AgencyMigrationCreateParams starts a migration: every key is checked and
// the source is scanned.
type AgencyMigrationCreateParams struct {
	// Provider is an [AgencyMigrationProvider.ID]. Required.
	Provider string `json:"provider"`
	Label    string `json:"label,omitempty"`
	// Credentials holds one set per key, keyed by
	// [AgencyMigrationField.Key]. Most tools need one; a tool with a key per
	// workspace ([AgencyMigrationProvider.MultiKey]) takes one for each.
	Credentials []map[string]string `json:"credentials"`
}

// AgencyMigrationStartParams chooses what to bring over and where.
type AgencyMigrationStartParams struct {
	Options AgencyMigrationOptions `json:"options"`
	// Targets maps each source workspace onto a client workspace, existing
	// or new.
	Targets []AgencyMigrationTargetChoice `json:"targets,omitempty"`
}

// AgencyMigrationTargetChoice is where one source workspace goes. Set exactly
// one of OrganizationID, CreateName and Skip.
type AgencyMigrationTargetChoice struct {
	// WorkspaceID and Key name the source workspace
	// ([AgencyMigrationSourceWorkspace]).
	WorkspaceID string `json:"workspace_id"`
	Key         int    `json:"key"`
	// OrganizationID is an existing client workspace.
	OrganizationID *string `json:"organization_id,omitempty"`
	// CreateName creates a new client workspace with this name.
	CreateName string `json:"create_name,omitempty"`
	// Skip leaves this source workspace out.
	Skip bool `json:"skip,omitempty"`
}

// MigrationProviders returns the outreach tools the agency can migrate from.
//
// Scope: [PermAgencyMigrate].
func (s *AgencyService) MigrationProviders(ctx context.Context, agencyID string, opts ...RequestOption) ([]AgencyMigrationProvider, *Response, error) {
	return fetchData[AgencyMigrationProvider](ctx, s.client, agencyPath(agencyID, "migrations", "providers"), opts)
}

// Migrations returns a page of the agency's migrations, newest first. Limit
// is 1 to 100 (default 20).
//
// Scope: [PermAgencyMigrate].
func (s *AgencyService) Migrations(ctx context.Context, agencyID string, params *ListOptions, opts ...RequestOption) (*Page[AgencyMigration], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[AgencyMigration](ctx, s.client, agencyPath(agencyID, "migrations"), q, opts...)
}

// CreateMigration checks the credentials and starts scanning the source. Poll
// [AgencyService.GetMigration] until the status is [AgencyMigrationReady],
// then call [AgencyService.StartMigration]. Pass [WithIdempotencyKey] to make
// a retry safe.
//
// Scope: [PermAgencyMigrate].
func (s *AgencyService) CreateMigration(ctx context.Context, agencyID string, params *AgencyMigrationCreateParams, opts ...RequestOption) (*AgencyMigration, *Response, error) {
	return send[AgencyMigration](ctx, s.client.post, agencyPath(agencyID, "migrations"), params, opts)
}

// GetMigration retrieves one migration.
//
// Scope: [PermAgencyMigrate].
func (s *AgencyService) GetMigration(ctx context.Context, agencyID, id string, opts ...RequestOption) (*AgencyMigration, *Response, error) {
	return fetch[AgencyMigration](ctx, s.client, agencyPath(agencyID, "migrations", url.PathEscape(id)), opts)
}

// StartMigration starts bringing data over from a ready migration. An agency
// key also needs [PermWriteCampaigns] to bring campaigns, [PermWriteEmails]
// to bring mailboxes and [PermAgencyManageClients] to create client
// workspaces. Pass
// [WithIdempotencyKey] to make a retry safe.
//
// Scope: [PermAgencyMigrate].
func (s *AgencyService) StartMigration(ctx context.Context, agencyID, id string, params *AgencyMigrationStartParams, opts ...RequestOption) (*AgencyMigration, *Response, error) {
	return send[AgencyMigration](ctx, s.client.post, agencyPath(agencyID, "migrations", url.PathEscape(id), "start"), params, opts)
}

// CancelMigration stops a scanning or running migration and wipes its keys.
//
// Scope: [PermAgencyMigrate].
func (s *AgencyService) CancelMigration(ctx context.Context, agencyID, id string, opts ...RequestOption) (*AgencyMigration, *Response, error) {
	return send[AgencyMigration](ctx, s.client.post, agencyPath(agencyID, "migrations", url.PathEscape(id), "cancel"), nil, opts)
}

// LinkMigrationSenders attaches mailboxes connected since the migration ran
// to the draft campaigns that were missing them
// ([AgencyMigrationCampaign.MissingSenders]). Running it twice changes
// nothing more. An agency key also needs [PermWriteCampaigns].
//
// Scope: [PermAgencyMigrate].
func (s *AgencyService) LinkMigrationSenders(ctx context.Context, agencyID, id string, opts ...RequestOption) (*AgencyMigration, *Response, error) {
	return send[AgencyMigration](ctx, s.client.post, agencyPath(agencyID, "migrations", url.PathEscape(id), "link-senders"), nil, opts)
}

// DismissMigration removes a migration that is not running from the list
// and wipes any keys it still holds.
//
// Scope: [PermAgencyMigrate].
func (s *AgencyService) DismissMigration(ctx context.Context, agencyID, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, agencyPath(agencyID, "migrations", url.PathEscape(id)), opts...)
}
