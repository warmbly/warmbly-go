package warmbly

import (
	"context"
	"net/url"
	"time"
)

// ToolMigrationService moves a workspace in from another outreach tool
// (Instantly, Smartlead, lemlist and the others [ToolMigrationService.Providers]
// lists). A migration runs in two steps:
//
//  1. [ToolMigrationService.Create] checks the pasted API keys and scans the
//     source account. The migration is "scanning" until the scan finishes,
//     then "ready" with an inventory of what each source workspace holds.
//  2. [ToolMigrationService.Start] picks what to bring over (blocklist,
//     mailboxes, campaigns, leads) and the migration runs in the background
//     until it is "completed", "failed" or "canceled". Poll
//     [ToolMigrationService.Get] to follow it.
//
// Every route needs the import_tools scope, and the caller must be able to
// manage contacts. Bringing mailboxes over also needs write_emails, and
// bringing campaigns over (or linking their senders) needs write_campaigns,
// on the key as well as on the member. A key or role limited to particular
// mailboxes or campaigns is refused with code "tool_migration_permission",
// because a migration writes across the whole workspace. The POST routes honor
// [WithIdempotencyKey].
//
// The source keys are held only while they are needed: they are wiped when the
// migration finishes, is canceled or is dismissed ([ToolMigration.CredentialsHeld]).
type ToolMigrationService service

// Migration states, for [ToolMigration.Status]. Other values may appear.
const (
	// ToolMigrationScanning: the source account is being read to show what it
	// holds.
	ToolMigrationScanning = "scanning"
	// ToolMigrationReady: the preview is ready and waits for
	// [ToolMigrationService.Start].
	ToolMigrationReady = "ready"
	// ToolMigrationRunning: data is being brought over.
	ToolMigrationRunning   = "running"
	ToolMigrationCompleted = "completed"
	ToolMigrationFailed    = "failed"
	ToolMigrationCanceled  = "canceled"
)

// Target states, for [ToolMigrationTarget.Status]. Other values may appear.
const (
	ToolMigrationTargetPending   = "pending"
	ToolMigrationTargetRunning   = "running"
	ToolMigrationTargetCompleted = "completed"
	ToolMigrationTargetFailed    = "failed"
	ToolMigrationTargetCanceled  = "canceled"
)

// What a migration can bring over, for [ToolMigrationOptions.Kinds],
// [ToolMigrationProvider.Kinds] and [ToolMigrationTarget.Phase]. They are
// applied in this order: the blocklist first, so nobody who opted out
// elsewhere is enrolled by a later step.
const (
	ToolMigrationKindBlocklist = "blocklist"
	ToolMigrationKindMailboxes = "mailboxes"
	ToolMigrationKindCampaigns = "campaigns"
	ToolMigrationKindLeads     = "leads"
)

// What happens to a lead the old tool was part way through a sequence with,
// for [ToolMigrationOptions.InFlight].
const (
	// ToolMigrationInFlightRestart enrolls the lead from the first step.
	ToolMigrationInFlightRestart = "restart"
	// ToolMigrationInFlightSkip imports the contact without enrolling it.
	ToolMigrationInFlightSkip = "skip"
	// ToolMigrationInFlightContinue is refused for now (code
	// "tool_migration_in_flight_unsupported"): picking a lead up mid-sequence
	// would need Warmbly to record sends it never made.
	ToolMigrationInFlightContinue = "continue"
)

// How a mailbox signs in, for [ToolMigrationMailbox.Kind]. Other values may
// appear.
const (
	ToolMigrationMailboxGoogle    = "google"
	ToolMigrationMailboxMicrosoft = "microsoft"
	ToolMigrationMailboxSMTP      = "smtp_imap"
	ToolMigrationMailboxUnknown   = "unknown"
)

// ToolMigrationProvider describes one tool a migration can read from.
type ToolMigrationProvider struct {
	// ID is the value for [ToolMigrationCreateParams.Provider], for example
	// "instantly".
	ID    string `json:"id"`
	Label string `json:"label"`
	// Fields are the credentials to send, in the order they are asked.
	Fields []ToolMigrationField `json:"fields"`
	// KeyHelp tells a person where the key lives in that tool.
	KeyHelp string `json:"key_help"`
	KeyURL  string `json:"key_url,omitempty"`
	// Kinds are what the provider's API can read, ToolMigrationKind* values.
	Kinds []string `json:"kinds"`
	// Workspaces is true when one key can see several client accounts or
	// workspaces.
	Workspaces bool `json:"workspaces"`
	// MultiKey is true when the tool issues one key per workspace, so a
	// migration takes one credential set for each.
	MultiKey bool `json:"multi_key"`
	// MailboxSecrets is true when the API returns mailbox passwords, so SMTP
	// and IMAP mailboxes connect without being asked for again.
	MailboxSecrets bool `json:"mailbox_secrets"`
	// Notes are short facts to know before starting.
	Notes []string `json:"notes,omitempty"`
}

// ToolMigrationField is one credential input a provider needs.
type ToolMigrationField struct {
	// Key is the key to use in a [ToolMigrationCredentials] map.
	Key         string `json:"key"`
	Label       string `json:"label"`
	Help        string `json:"help,omitempty"`
	Secret      bool   `json:"secret"`
	Optional    bool   `json:"optional,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	// Options limits the value to a fixed set, such as a data region.
	Options []string `json:"options,omitempty"`
}

// ToolMigrationCredentials are the values for a provider's
// [ToolMigrationProvider.Fields], keyed by [ToolMigrationField.Key].
type ToolMigrationCredentials map[string]string

// ToolMigrationAccount is who one pasted key acts as in the source tool.
type ToolMigrationAccount struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	// Plan is the provider's plan name when it reports one.
	Plan string `json:"plan,omitempty"`
}

// ToolMigrationSourceWorkspace is one client account or workspace inside the
// source tool.
type ToolMigrationSourceWorkspace struct {
	// ID is the provider's id, empty for the account itself.
	ID   string `json:"id"`
	Name string `json:"name"`
	// Key is set when this workspace is read with a key of its own: an index
	// into the migration's credential sets.
	Key int `json:"key,omitempty"`
}

// ToolMigrationInventory is what one source workspace holds.
type ToolMigrationInventory struct {
	Mailboxes int `json:"mailboxes"`
	Campaigns int `json:"campaigns"`
	Leads     int `json:"leads"`
	Blocklist int `json:"blocklist"`
	// Summaries lists the campaigns, for choosing
	// [ToolMigrationOptions.CampaignIDs].
	Summaries []ToolMigrationCampaignSummary `json:"campaign_summaries,omitempty"`
}

// ToolMigrationCampaignSummary is one source campaign as the preview lists it.
type ToolMigrationCampaignSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Status is the source tool's state, normalized: "draft", "active",
	// "paused", "completed" or "archived".
	Status string `json:"status"`
	// Steps and Leads are -1 when the source API cannot give them cheaply.
	Steps int `json:"steps"`
	Leads int `json:"leads"`
}

// ToolMigrationWorkspace is one source workspace and what the scan found in
// it.
type ToolMigrationWorkspace struct {
	Workspace ToolMigrationSourceWorkspace `json:"workspace"`
	Inventory ToolMigrationInventory       `json:"inventory"`
	// Error is why this workspace could not be read; empty when it was.
	Error string `json:"error,omitempty"`
}

// ToolMigrationOptions are the choices made when a migration starts.
type ToolMigrationOptions struct {
	// Kinds are what to bring over, ToolMigrationKind* values.
	Kinds []string `json:"kinds"`
	// CampaignIDs limits campaigns (and their leads) to these source ids;
	// empty brings every campaign.
	CampaignIDs []string `json:"campaign_ids,omitempty"`
	// InFlight is one of the ToolMigrationInFlight* constants.
	InFlight string `json:"in_flight"`
	// SkipFinished leaves out campaigns the source marks completed or
	// archived.
	SkipFinished bool `json:"skip_finished"`
	// Warmup starts warmup on mailboxes that connect, unless the source said
	// it was off.
	Warmup bool `json:"warmup"`
}

// ToolMigrationCounts is what one target brought over.
type ToolMigrationCounts struct {
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

// ToolMigrationMailbox is a mailbox someone has to connect by hand.
type ToolMigrationMailbox struct {
	Email string `json:"email"`
	// Kind is one of the ToolMigrationMailbox* constants.
	Kind string `json:"kind"`
}

// ToolMigrationCampaign is one draft campaign a migration created.
type ToolMigrationCampaign struct {
	SourceID   string `json:"source_id"`
	CampaignID string `json:"campaign_id"`
	Name       string `json:"name"`
	// MissingSenders are source sender addresses with no mailbox in the
	// workspace yet; [ToolMigrationService.LinkSenders] attaches them once
	// they are connected.
	MissingSenders []string `json:"missing_senders,omitempty"`
}

// ToolMigrationFollowup is what a person still has to do after a target ran.
type ToolMigrationFollowup struct {
	// Reconnect are mailboxes that sign in with Google or Microsoft, or whose
	// password the source does not return, and need connecting by their
	// owner.
	Reconnect []ToolMigrationMailbox `json:"reconnect,omitempty"`
	// Notes are things that did not carry over, such as LinkedIn steps.
	Notes []string `json:"notes,omitempty"`
	// Campaigns are the draft campaigns created, for review before launch.
	Campaigns []ToolMigrationCampaign `json:"campaigns,omitempty"`
}

// ToolMigrationTarget is one source workspace brought into one Warmbly
// workspace.
type ToolMigrationTarget struct {
	ID               string                       `json:"id"`
	MigrationID      string                       `json:"migration_id"`
	Workspace        ToolMigrationSourceWorkspace `json:"workspace"`
	OrganizationID   string                       `json:"organization_id"`
	OrganizationName string                       `json:"organization_name,omitempty"`
	// Status is one of the ToolMigrationTarget* constants.
	Status string `json:"status"`
	// Phase is the kind being imported now, empty before the first.
	Phase    string                `json:"phase,omitempty"`
	Counts   ToolMigrationCounts   `json:"counts"`
	Followup ToolMigrationFollowup `json:"followup"`
	Error    string                `json:"error,omitempty"`
	// MailboxImportID is the mailbox import that connects the mailboxes the
	// source returned passwords for.
	MailboxImportID *string `json:"mailbox_import_id,omitempty"`
	// CategoryID is the contact category every imported contact is filed
	// under.
	CategoryID *string    `json:"category_id,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// ToolMigration is one import from another outreach tool.
type ToolMigration struct {
	ID string `json:"id"`
	// OrganizationID is the workspace the migration belongs to.
	OrganizationID *string `json:"organization_id,omitempty"`
	// AgencyID is set instead on an agency's migration, which writes into
	// the client workspaces its targets name.
	AgencyID        *string `json:"agency_id,omitempty"`
	CreatedByUserID *string `json:"created_by_user_id,omitempty"`
	// Provider is the source tool's [ToolMigrationProvider.ID].
	Provider string `json:"provider"`
	Label    string `json:"label"`
	// Status is one of the ToolMigration* state constants.
	Status string `json:"status"`
	// Accounts are who each pasted key acts as, in the order they were added.
	Accounts []ToolMigrationAccount `json:"accounts"`
	// Inventory is what the scan found, one entry per source workspace.
	Inventory []ToolMigrationWorkspace `json:"inventory"`
	// Options are the choices the migration was started with; nil before
	// it starts.
	Options *ToolMigrationOptions `json:"options,omitempty"`
	Targets []ToolMigrationTarget `json:"targets"`
	Error   string                `json:"error,omitempty"`
	// CredentialsHeld is false once the source keys have been wiped.
	CredentialsHeld bool       `json:"credentials_held"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Active reports whether the migration still has work in flight.
func (m *ToolMigration) Active() bool {
	return m.Status == ToolMigrationScanning || m.Status == ToolMigrationRunning
}

// ToolMigrationCreateParams starts a migration: the server checks every key
// and scans the source.
type ToolMigrationCreateParams struct {
	// Provider is a [ToolMigrationProvider.ID]. Required.
	Provider string `json:"provider"`
	// Label names the migration; optional.
	Label string `json:"label,omitempty"`
	// Credentials holds one set per key. Most tools need one; a tool with a
	// key per workspace ([ToolMigrationProvider.MultiKey]) takes one for each.
	Credentials []ToolMigrationCredentials `json:"credentials"`
}

// ToolMigrationTargetChoice is where one source workspace goes. A workspace's
// own migration sends no targets: every source workspace lands in it.
type ToolMigrationTargetChoice struct {
	WorkspaceID string `json:"workspace_id"`
	Key         int    `json:"key"`
	// OrganizationID is an existing workspace to bring it into.
	OrganizationID *string `json:"organization_id,omitempty"`
	// CreateName creates a new workspace with this name.
	CreateName string `json:"create_name,omitempty"`
	// Skip leaves this source workspace out.
	Skip bool `json:"skip,omitempty"`
}

// ToolMigrationStartParams is the choice of what to bring over, and where.
type ToolMigrationStartParams struct {
	Options ToolMigrationOptions `json:"options"`
	// Targets maps source workspaces onto Warmbly workspaces. Leave it empty
	// for a workspace's own migration.
	Targets []ToolMigrationTargetChoice `json:"targets,omitempty"`
}

// Providers lists the tools a migration can read from, with the credentials
// each needs. Needs the import_tools scope. It answers an empty list on an
// instance where migrations are off.
func (s *ToolMigrationService) Providers(ctx context.Context, opts ...RequestOption) ([]ToolMigrationProvider, *Response, error) {
	return fetchData[ToolMigrationProvider](ctx, s.client, "migrations/providers", opts)
}

// List returns a page of the workspace's migrations, newest first. Limit is 1
// to 100 (default 20). Needs the import_tools scope.
func (s *ToolMigrationService) List(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[ToolMigration], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[ToolMigration](ctx, s.client, "migrations", q, opts...)
}

// Create checks the keys and starts scanning the source account. The
// migration comes back "scanning" (201); poll [ToolMigrationService.Get]
// until it is "ready". A rejected key is a 422 "tool_migration_key_rejected",
// a source tool that did not answer a 503
// "tool_migration_provider_unavailable", an unknown provider a 400
// "tool_migration_provider_unknown", and too many migrations at once a 409
// "tool_migration_limit". Needs the import_tools scope.
func (s *ToolMigrationService) Create(ctx context.Context, params *ToolMigrationCreateParams, opts ...RequestOption) (*ToolMigration, *Response, error) {
	return send[ToolMigration](ctx, s.client.post, "migrations", params, opts)
}

// Get returns one migration with its inventory and per-target progress.
// Needs the import_tools scope.
func (s *ToolMigrationService) Get(ctx context.Context, id string, opts ...RequestOption) (*ToolMigration, *Response, error) {
	return fetch[ToolMigration](ctx, s.client, "migrations/"+url.PathEscape(id), opts)
}

// Start begins bringing data over. The migration must be "ready" (otherwise a
// 409 "tool_migration_not_ready"); starting one that is already running
// answers with it as it stands, so a retry is safe. Needs the import_tools
// scope, plus write_emails for mailboxes and write_campaigns for campaigns.
func (s *ToolMigrationService) Start(ctx context.Context, id string, params *ToolMigrationStartParams, opts ...RequestOption) (*ToolMigration, *Response, error) {
	return send[ToolMigration](ctx, s.client.post, "migrations/"+url.PathEscape(id)+"/start", params, opts)
}

// Cancel stops a migration and wipes its keys. What was already brought over
// stays. Needs the import_tools scope.
func (s *ToolMigrationService) Cancel(ctx context.Context, id string, opts ...RequestOption) (*ToolMigration, *Response, error) {
	return send[ToolMigration](ctx, s.client.post, "migrations/"+url.PathEscape(id)+"/cancel", nil, opts)
}

// LinkSenders attaches mailboxes connected since the migration ran to the
// draft campaigns that named them as senders
// ([ToolMigrationCampaign.MissingSenders]). Running it again changes nothing
// more, so a retry is safe. Needs the import_tools and write_campaigns
// scopes.
func (s *ToolMigrationService) LinkSenders(ctx context.Context, id string, opts ...RequestOption) (*ToolMigration, *Response, error) {
	return send[ToolMigration](ctx, s.client.post, "migrations/"+url.PathEscape(id)+"/link-senders", nil, opts)
}

// Dismiss deletes a migration that is not running, with any keys it still
// holds; what it brought over stays. A running migration must be canceled
// first (409
// "tool_migration_running"). Needs the import_tools scope. Answers 204.
func (s *ToolMigrationService) Dismiss(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "migrations/"+url.PathEscape(id), opts...)
}
