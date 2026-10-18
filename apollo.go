package warmbly

import (
	"context"
	"encoding/json"
	"net/url"
	"time"
)

// ApolloService drives a connected Apollo account: saved lists, list imports
// that feed contacts into Warmbly, and background enrichment of existing
// contacts. Every method takes the id of the Apollo connection, which comes
// from [IntegrationService.Connect] with [ProviderApollo] or
// [IntegrationService.Connections].
//
// Each method names the access it needs. Read means an API key with
// [PermIntegrations], or a member who can manage settings or use
// integrations. Manage settings is [PermIntegrations] on a key and the
// manage-settings right on a member. Use integrations is [PermIntegrations]
// on a key and the use-integrations right on a member. Imports and enrichment
// spend the Apollo account's credits, so they sit behind use integrations.
type ApolloService service

// Run states of an [ApolloImportSource], returned in
// [ApolloImportSource.Status]. Treat an unknown value as a new state.
const (
	ApolloSourceIdle    = "idle"
	ApolloSourceRunning = "running"
	ApolloSourceError   = "error"
)

// Run states of an [ApolloEnrichmentRun], returned in
// [ApolloEnrichmentRun.Status]. Treat an unknown value as a new state.
const (
	ApolloEnrichmentQueued  = "queued"
	ApolloEnrichmentRunning = "running"
	ApolloEnrichmentDone    = "done"
	ApolloEnrichmentError   = "error"
)

// ApolloOverview is the Apollo page header: the account the API key acts as
// and what the connection has done.
type ApolloOverview struct {
	ConnectionID string `json:"connection_id"`
	Label        string `json:"label"`
	// Status is one of the Connection* constants.
	Status string `json:"status"`
	// Health is "unknown", "healthy", "degraded" or "down".
	Health       string `json:"health"`
	HealthDetail string `json:"health_detail,omitempty"`
	AccountName  string `json:"account_name,omitempty"`
	AccountEmail string `json:"account_email,omitempty"`
	// Sources is how many import sources the connection has.
	Sources       int        `json:"sources"`
	TotalImported int        `json:"total_imported"`
	LastRunAt     *time.Time `json:"last_run_at,omitempty"`
}

// ApolloList is one saved Apollo list of contacts.
type ApolloList struct {
	// ID is Apollo's list id, the value to pass as a ListID.
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// ApolloPreviewParams selects the list [ApolloService.PreviewImport] reads.
type ApolloPreviewParams struct {
	// ListID is an [ApolloList.ID].
	ListID string `json:"list_id"`
	// VerifiedOnly marks only people with a verified address as importable.
	// Nil means true, which is also what the server assumes.
	VerifiedOnly *bool `json:"verified_only,omitempty"`
}

// ApolloPreviewRow is one sample person of a list preview.
type ApolloPreviewRow struct {
	ApolloID    string `json:"apollo_id"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	EmailStatus string `json:"email_status"`
	Title       string `json:"title"`
	Company     string `json:"company"`
	// AlreadyInWarmbly reports whether a contact with this address exists.
	AlreadyInWarmbly bool `json:"already_in_warmbly"`
	// WillImport is false for a missing, suppressed or (with VerifiedOnly)
	// unverified address.
	WillImport bool `json:"will_import"`
}

// ApolloPreview is the first people of a list and how many it holds.
type ApolloPreview struct {
	Total  int                `json:"total"`
	Sample []ApolloPreviewRow `json:"sample"`
}

// ApolloRunResult is what one import run did.
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
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	ConnectionID   string `json:"connection_id"`
	// CreatedByUserID is nil for a source whose creator is gone.
	CreatedByUserID *string `json:"created_by_user_id,omitempty"`
	Name            string  `json:"name"`
	ListID          string  `json:"list_id"`
	ListName        string  `json:"list_name"`
	// CampaignID is the campaign imported people are enrolled in, or nil.
	CampaignID  *string  `json:"campaign_id,omitempty"`
	CategoryIDs []string `json:"category_ids"`
	// VerifiedOnly skips people without a verified address.
	VerifiedOnly bool `json:"verified_only"`
	// Recurring keeps the source in sync hourly.
	Recurring bool `json:"recurring"`
	Enabled   bool `json:"enabled"`
	// Status is one of the ApolloSource* constants.
	Status     string           `json:"status"`
	LastRunAt  *time.Time       `json:"last_run_at,omitempty"`
	LastResult *ApolloRunResult `json:"last_result,omitempty"`
	LastError  string           `json:"last_error,omitempty"`
	// TotalImported counts people brought in over every run.
	TotalImported int       `json:"total_imported"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ApolloImportSourceCreateParams saves a list import. Creating a source starts
// its first run, so the returned source is normally already running.
type ApolloImportSourceCreateParams struct {
	// ListID is an [ApolloList.ID]. Required, at most 64 characters.
	ListID string `json:"list_id"`
	// ListName is the list's display name, kept for the source's label.
	ListName string `json:"list_name,omitempty"`
	// Name labels the source (at most 200 characters). Empty falls back to
	// ListName, then "Apollo list".
	Name *string `json:"name,omitempty"`
	// CampaignID enrolls imported people in a campaign.
	CampaignID *string `json:"campaign_id,omitempty"`
	// CategoryIDs tags imported people, at most 50.
	CategoryIDs []string `json:"category_ids,omitempty"`
	// VerifiedOnly defaults to true when nil.
	VerifiedOnly *bool `json:"verified_only,omitempty"`
	// Recurring keeps the source in sync hourly. Defaults to false.
	Recurring *bool `json:"recurring,omitempty"`
}

// ApolloImportSourceUpdateParams edits a source's targets, filter and
// schedule. The list itself cannot change; create a new source instead. A nil
// field is left as it is.
type ApolloImportSourceUpdateParams struct {
	Name *string `json:"name,omitempty"`
	// CampaignID enrolls imported people in a campaign. To stop enrolling,
	// set ClearCampaign: a nil pointer means "leave as is".
	CampaignID *string `json:"campaign_id,omitempty"`
	// ClearCampaign sends campaign_id as JSON null, detaching the campaign.
	// It wins over CampaignID when both are set.
	ClearCampaign bool `json:"-"`
	// CategoryIDs replaces the tag list (at most 50). Point at an empty slice
	// to clear it.
	CategoryIDs  *[]string `json:"category_ids,omitempty"`
	VerifiedOnly *bool     `json:"verified_only,omitempty"`
	Recurring    *bool     `json:"recurring,omitempty"`
	Enabled      *bool     `json:"enabled,omitempty"`
}

// MarshalJSON emits campaign_id as an explicit null when ClearCampaign is set,
// which is how the server distinguishes "detach" from "unchanged".
func (p ApolloImportSourceUpdateParams) MarshalJSON() ([]byte, error) {
	type plain ApolloImportSourceUpdateParams
	if !p.ClearCampaign {
		return json.Marshal(plain(p))
	}
	p.CampaignID = nil
	// The outer field shadows the embedded omitempty one, so a nil pointer
	// here serializes as null rather than being dropped.
	return json.Marshal(struct {
		*plain
		CampaignID *string `json:"campaign_id"`
	}{plain: (*plain)(&p)})
}

// ApolloEnrichParams selects the contacts to enrich and starts a run. Name
// either Contacts or All; at most 5,000 contacts can be enriched in one run
// (code "apollo_enrich_too_many").
type ApolloEnrichParams struct {
	// Contacts are contact ids. Used when All is false.
	Contacts []string `json:"contacts,omitempty"`
	// All switches the selection from Contacts to Filters.
	All bool `json:"all,omitempty"`
	// Filters is the same filter [ContactService.Search] takes, so the set
	// enriched is exactly the set a list was showing. Used when All is true.
	Filters *ContactSearchParams `json:"filters,omitempty"`
	// Exclude drops ids from the resolved set. Ignored unless All is set.
	Exclude []string `json:"exclude,omitempty"`
	// Overwrite replaces fields a contact already has. Without it Apollo only
	// fills the blanks.
	Overwrite bool `json:"overwrite"`
}

// ApolloEnrichmentRun is one background request to enrich Warmbly contacts
// through Apollo's people match.
type ApolloEnrichmentRun struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	ConnectionID   string `json:"connection_id"`
	// RequestedByUserID is nil for a run whose requester is gone.
	RequestedByUserID *string `json:"requested_by_user_id,omitempty"`
	Overwrite         bool    `json:"overwrite"`
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

// ApolloEnrichmentListParams narrows [ApolloService.ListEnrichments].
type ApolloEnrichmentListParams struct {
	// Limit is how many recent runs to return, 1 to 100. Zero uses the server
	// default of 20.
	Limit int
}

func (p *ApolloEnrichmentListParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	setPositive(q, "limit", p.Limit)
	return q
}

func apolloPath(connectionID, rest string) string {
	return "integrations/apollo/" + url.PathEscape(connectionID) + rest
}

// Overview returns the account the connection's key acts as and what the
// connection has imported.
//
// GET /integrations/apollo/:id. Needs read access.
func (s *ApolloService) Overview(ctx context.Context, connectionID string, opts ...RequestOption) (*ApolloOverview, *Response, error) {
	return fetch[ApolloOverview](ctx, s.client, apolloPath(connectionID, ""), opts)
}

// ReplaceKey checks a new Apollo API key against Apollo and swaps it in. The
// call is a PUT, so retrying it converges on the same key.
//
// PUT /integrations/apollo/:id/key. Needs manage-settings access.
func (s *ApolloService) ReplaceKey(ctx context.Context, connectionID, apiKey string, opts ...RequestOption) (*ApolloOverview, *Response, error) {
	body := struct {
		APIKey string `json:"api_key"`
	}{apiKey}
	return send[ApolloOverview](ctx, s.client.put, apolloPath(connectionID, "/key"), body, opts)
}

// Lists returns the account's saved contact lists.
//
// GET /integrations/apollo/:id/lists. Needs read access.
func (s *ApolloService) Lists(ctx context.Context, connectionID string, opts ...RequestOption) ([]ApolloList, *Response, error) {
	return fetchData[ApolloList](ctx, s.client, apolloPath(connectionID, "/lists"), opts)
}

// PreviewImport reads the first people of a list and how many it holds,
// without importing anything.
//
// POST /integrations/apollo/:id/import/preview. Needs use-integrations
// access.
func (s *ApolloService) PreviewImport(ctx context.Context, connectionID string, params *ApolloPreviewParams, opts ...RequestOption) (*ApolloPreview, *Response, error) {
	return send[ApolloPreview](ctx, s.client.post, apolloPath(connectionID, "/import/preview"), params, opts)
}

// ListImportSources returns a connection's saved list imports.
//
// GET /integrations/apollo/:id/import-sources. Needs read access.
func (s *ApolloService) ListImportSources(ctx context.Context, connectionID string, opts ...RequestOption) ([]ApolloImportSource, *Response, error) {
	return fetchData[ApolloImportSource](ctx, s.client, apolloPath(connectionID, "/import-sources"), opts)
}

// CreateImportSource saves a list import and starts its first run. A retried
// create makes a second source whose run imports nothing new, since people are
// deduplicated by address.
//
// POST /integrations/apollo/:id/import-sources, answered with 201. Needs
// manage-settings access.
func (s *ApolloService) CreateImportSource(ctx context.Context, connectionID string, params *ApolloImportSourceCreateParams, opts ...RequestOption) (*ApolloImportSource, *Response, error) {
	return send[ApolloImportSource](ctx, s.client.post, apolloPath(connectionID, "/import-sources"), params, opts)
}

// UpdateImportSource edits a list import's targets, filter and schedule.
//
// PATCH /integrations/apollo/:id/import-sources/:sourceId. Needs
// manage-settings access.
func (s *ApolloService) UpdateImportSource(ctx context.Context, connectionID, sourceID string, params *ApolloImportSourceUpdateParams, opts ...RequestOption) (*ApolloImportSource, *Response, error) {
	return send[ApolloImportSource](ctx, s.client.patch, apolloPath(connectionID, "/import-sources/"+url.PathEscape(sourceID)), params, opts)
}

// RunImportSource runs a list import now and returns the source in its
// running state; poll [ApolloService.ListImportSources] for the result. A
// request while the source is already running is refused with a 409 (code
// "import_running").
//
// POST /integrations/apollo/:id/import-sources/:sourceId/run. Needs
// use-integrations access.
func (s *ApolloService) RunImportSource(ctx context.Context, connectionID, sourceID string, opts ...RequestOption) (*ApolloImportSource, *Response, error) {
	return send[ApolloImportSource](ctx, s.client.post, apolloPath(connectionID, "/import-sources/"+url.PathEscape(sourceID)+"/run"), nil, opts)
}

// DeleteImportSource removes a list import. Contacts it brought in stay.
//
// DELETE /integrations/apollo/:id/import-sources/:sourceId, answered with 204.
// Needs manage-settings access.
func (s *ApolloService) DeleteImportSource(ctx context.Context, connectionID, sourceID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, apolloPath(connectionID, "/import-sources/"+url.PathEscape(sourceID)), opts...)
}

// ListEnrichments returns a connection's recent enrichment runs. The list is
// not paginated.
//
// GET /integrations/apollo/:id/enrichments. Needs read access.
func (s *ApolloService) ListEnrichments(ctx context.Context, connectionID string, params *ApolloEnrichmentListParams, opts ...RequestOption) ([]ApolloEnrichmentRun, *Response, error) {
	return fetchData[ApolloEnrichmentRun](ctx, s.client, withQuery(apolloPath(connectionID, "/enrichments"), params.values()), opts)
}

// StartEnrichment queues a run that enriches the selected contacts with
// title, company, LinkedIn and location, spending Apollo credits. It answers
// 202 with the queued run; poll [ApolloService.GetEnrichment] for progress.
// Pass [WithIdempotencyKey] to make a retry safe; without one a retry queues a
// second run.
//
// POST /integrations/apollo/:id/enrichments. Needs both use-integrations
// access and [PermBulkContacts] (the member's manage-contacts right).
func (s *ApolloService) StartEnrichment(ctx context.Context, connectionID string, params *ApolloEnrichParams, opts ...RequestOption) (*ApolloEnrichmentRun, *Response, error) {
	return send[ApolloEnrichmentRun](ctx, s.client.post, apolloPath(connectionID, "/enrichments"), params, opts)
}

// GetEnrichment returns one enrichment run's progress.
//
// GET /integrations/apollo/:id/enrichments/:runId. Needs read access.
func (s *ApolloService) GetEnrichment(ctx context.Context, connectionID, runID string, opts ...RequestOption) (*ApolloEnrichmentRun, *Response, error) {
	return fetch[ApolloEnrichmentRun](ctx, s.client, apolloPath(connectionID, "/enrichments/"+url.PathEscape(runID)), opts)
}
