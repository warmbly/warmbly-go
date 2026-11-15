package warmbly

import (
	"context"
	"net/url"
)

// The agency migration methods share their types with [ToolMigrationService]:
// an agency migration is a [ToolMigration] whose AgencyID is set and whose
// targets name client workspaces.

// MigrationProviders returns the outreach tools the agency can migrate from.
//
// Scope: [PermAgencyMigrate].
func (s *AgencyService) MigrationProviders(ctx context.Context, agencyID string, opts ...RequestOption) ([]ToolMigrationProvider, *Response, error) {
	return fetchData[ToolMigrationProvider](ctx, s.client, agencyPath(agencyID, "migrations", "providers"), opts)
}

// Migrations returns a page of the agency's migrations, newest first. Limit
// is 1 to 100 (default 20).
//
// Scope: [PermAgencyMigrate].
func (s *AgencyService) Migrations(ctx context.Context, agencyID string, params *ListOptions, opts ...RequestOption) (*Page[ToolMigration], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[ToolMigration](ctx, s.client, agencyPath(agencyID, "migrations"), q, opts...)
}

// CreateMigration checks the credentials and starts scanning the source. Poll
// [AgencyService.GetMigration] until the status is [ToolMigrationReady],
// then call [AgencyService.StartMigration]. Pass [WithIdempotencyKey] to make
// a retry safe.
//
// Scope: [PermAgencyMigrate].
func (s *AgencyService) CreateMigration(ctx context.Context, agencyID string, params *ToolMigrationCreateParams, opts ...RequestOption) (*ToolMigration, *Response, error) {
	return send[ToolMigration](ctx, s.client.post, agencyPath(agencyID, "migrations"), params, opts)
}

// GetMigration retrieves one migration.
//
// Scope: [PermAgencyMigrate].
func (s *AgencyService) GetMigration(ctx context.Context, agencyID, id string, opts ...RequestOption) (*ToolMigration, *Response, error) {
	return fetch[ToolMigration](ctx, s.client, agencyPath(agencyID, "migrations", url.PathEscape(id)), opts)
}

// StartMigration starts bringing data over from a ready migration. An agency
// key also needs [PermWriteCampaigns] to bring campaigns, [PermWriteEmails]
// to bring mailboxes and [PermAgencyManageClients] to create client
// workspaces. Pass
// [WithIdempotencyKey] to make a retry safe.
//
// Scope: [PermAgencyMigrate].
func (s *AgencyService) StartMigration(ctx context.Context, agencyID, id string, params *ToolMigrationStartParams, opts ...RequestOption) (*ToolMigration, *Response, error) {
	return send[ToolMigration](ctx, s.client.post, agencyPath(agencyID, "migrations", url.PathEscape(id), "start"), params, opts)
}

// CancelMigration stops a scanning or running migration and wipes its keys.
//
// Scope: [PermAgencyMigrate].
func (s *AgencyService) CancelMigration(ctx context.Context, agencyID, id string, opts ...RequestOption) (*ToolMigration, *Response, error) {
	return send[ToolMigration](ctx, s.client.post, agencyPath(agencyID, "migrations", url.PathEscape(id), "cancel"), nil, opts)
}

// LinkMigrationSenders attaches mailboxes connected since the migration ran
// to the draft campaigns that were missing them
// ([ToolMigrationCampaign.MissingSenders]). Running it twice changes
// nothing more. An agency key also needs [PermWriteCampaigns].
//
// Scope: [PermAgencyMigrate].
func (s *AgencyService) LinkMigrationSenders(ctx context.Context, agencyID, id string, opts ...RequestOption) (*ToolMigration, *Response, error) {
	return send[ToolMigration](ctx, s.client.post, agencyPath(agencyID, "migrations", url.PathEscape(id), "link-senders"), nil, opts)
}

// DismissMigration removes a migration that is not running from the list
// and wipes any keys it still holds.
//
// Scope: [PermAgencyMigrate].
func (s *AgencyService) DismissMigration(ctx context.Context, agencyID, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, agencyPath(agencyID, "migrations", url.PathEscape(id)), opts...)
}
