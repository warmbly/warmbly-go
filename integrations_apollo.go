package warmbly

import (
	"context"
	"net/url"
	"strconv"
	"time"
)

// The Apollo methods take the id of an Apollo connection, which is made with
// [IntegrationService.Connect] and an API key. Every one needs
// [PermIntegrations] for an API key. A session member needs the settings
// permission to replace the key or change import sources, and the integrations
// permission to read, preview, run an import or enrich contacts.

// Apollo enrichment run states returned in [ApolloEnrichmentRun.Status].
const (
	ApolloEnrichmentQueued  = "queued"
	ApolloEnrichmentRunning = "running"
	ApolloEnrichmentDone    = "done"
	ApolloEnrichmentError   = "error"
)

// ApolloOverview is the Apollo page header: the account a key acts as and what
// the connection has done.
type ApolloOverview struct {
	ConnectionID string `json:"connection_id"`
	Label        string `json:"label"`
	// Status is one of the Connection* constants and Health is "unknown",
	// "healthy", "degraded" or "down".
	Status        string     `json:"status"`
	Health        string     `json:"health"`
	HealthDetail  string     `json:"health_detail,omitempty"`
	AccountName   string     `json:"account_name,omitempty"`
	AccountEmail  string     `json:"account_email,omitempty"`
	Sources       int        `json:"sources"`
	TotalImported int        `json:"total_imported"`
	LastRunAt     *time.Time `json:"last_run_at,omitempty"`
}

// ApolloList is a saved Apollo list of contacts, as the import picker shows it.
type ApolloList struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// ApolloPreviewRow is one sample person of a list preview.
type ApolloPreviewRow struct {
	ApolloID string `json:"apollo_id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	// EmailStatus is Apollo's verdict on the address, for example "verified".
	EmailStatus string `json:"email_status"`
	Title       string `json:"title"`
	Company     string `json:"company"`
	// AlreadyInWarmbly reports that the address is already a contact and
	// WillImport that an import would bring this person in.
	AlreadyInWarmbly bool `json:"already_in_warmbly"`
	WillImport       bool `json:"will_import"`
}

// ApolloPreview is the first people of a list and how many it holds.
type ApolloPreview struct {
	Total  int                `json:"total"`
	Sample []ApolloPreviewRow `json:"sample"`
}

// ApolloPreviewParams chooses the list to preview.
type ApolloPreviewParams struct {
	ListID string `json:"list_id"`
	// VerifiedOnly counts only people with a verified address. The server
	// treats an omitted value as true.
	VerifiedOnly *bool `json:"verified_only,omitempty"`
}

// ApolloRunResult is what one list import run did.
type ApolloRunResult struct {
	Read       int `json:"read"`
	Imported   int `json:"imported"`
	Updated    int `json:"updated"`
	Skipped    int `json:"skipped"`
	Failed     int `json:"failed"`
	NoEmail    int `json:"no_email"`
	Unverified int `json:"unverified"`
	Suppressed int `json:"suppressed"`
	// AlreadySeen counts people a previous run already handled.
	AlreadySeen int `json:"already_seen"`
	// Truncated reports that the list held more than one run reads, and
	// NextPage is where the next run resumes.
	Truncated bool `json:"truncated,omitempty"`
	NextPage  int  `json:"next_page,omitempty"`
}

// ApolloImportSource is a saved Apollo list that feeds contacts into Warmbly.
type ApolloImportSource struct {
	ID              string  `json:"id"`
	OrganizationID  string  `json:"organization_id"`
	ConnectionID    string  `json:"connection_id"`
	CreatedByUserID *string `json:"created_by_user_id,omitempty"`
	Name            string  `json:"name"`
	ListID          string  `json:"list_id"`
	ListName        string  `json:"list_name"`
	// CampaignID is the Warmbly campaign imported contacts are enrolled in.
	CampaignID *string `json:"campaign_id,omitempty"`
	// CategoryIDs are the tags applied to imported contacts.
	CategoryIDs []string `json:"category_ids"`
	// VerifiedOnly skips people whose address Apollo has not verified.
	VerifiedOnly bool `json:"verified_only"`
	// Recurring keeps the source in sync on a schedule.
	Recurring bool `json:"recurring"`
	Enabled   bool `json:"enabled"`
	// Status is "idle", "running" or "error".
	Status        string           `json:"status"`
	LastRunAt     *time.Time       `json:"last_run_at,omitempty"`
	LastResult    *ApolloRunResult `json:"last_result,omitempty"`
	LastError     string           `json:"last_error,omitempty"`
	TotalImported int              `json:"total_imported"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

// ApolloImportSourceParams saves an Apollo list import. Saving one starts its
// first run.
type ApolloImportSourceParams struct {
	// Name defaults to ListName.
	Name     *string `json:"name,omitempty"`
	ListID   string  `json:"list_id"`
	ListName string  `json:"list_name,omitempty"`
	// CampaignID enrolls imported contacts in a Warmbly campaign.
	CampaignID *string `json:"campaign_id,omitempty"`
	// CategoryIDs tags imported contacts. At most 50.
	CategoryIDs []string `json:"category_ids,omitempty"`
	// VerifiedOnly defaults to true on the server when omitted.
	VerifiedOnly *bool `json:"verified_only,omitempty"`
	Recurring    *bool `json:"recurring,omitempty"`
}

// ApolloImportSourceUpdate edits an Apollo import's targets, filter and
// schedule. The list it reads cannot be changed. Nil fields are left unchanged.
type ApolloImportSourceUpdate struct {
	Name *string
	// CampaignID enrolls imported contacts in a Warmbly campaign. Set
	// ClearCampaign instead to stop enrolling.
	CampaignID    *string
	ClearCampaign bool
	// CategoryIDs replaces the tags applied to imported contacts. A nil slice
	// leaves them; an empty, non-nil slice removes them all. At most 50.
	CategoryIDs  []string
	VerifiedOnly *bool
	Recurring    *bool
	Enabled      *bool
}

func (u *ApolloImportSourceUpdate) body() map[string]any {
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
	if u.VerifiedOnly != nil {
		body["verified_only"] = *u.VerifiedOnly
	}
	if u.Recurring != nil {
		body["recurring"] = *u.Recurring
	}
	if u.Enabled != nil {
		body["enabled"] = *u.Enabled
	}
	return body
}

// ApolloEnrichmentRun is one background request to enrich Warmbly contacts
// through Apollo's people match. Each match spends the customer's Apollo
// credits.
type ApolloEnrichmentRun struct {
	ID                string  `json:"id"`
	OrganizationID    string  `json:"organization_id"`
	ConnectionID      string  `json:"connection_id"`
	RequestedByUserID *string `json:"requested_by_user_id,omitempty"`
	// Overwrite replaces fields a contact already has.
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

// ApolloEnrichParams starts an enrichment run.
type ApolloEnrichParams struct {
	// ContactIDs are the contacts to enrich. At most 5000 per run.
	ContactIDs []string `json:"contact_ids"`
	// Overwrite replaces fields a contact already has instead of filling
	// blanks.
	Overwrite bool `json:"overwrite"`
}

// ApolloOverview returns the Apollo page header for a connection.
func (s *IntegrationService) ApolloOverview(ctx context.Context, id string, opts ...RequestOption) (*ApolloOverview, *Response, error) {
	return fetch[ApolloOverview](ctx, s.client, "integrations/apollo/"+url.PathEscape(id), opts)
}

// ReplaceApolloKey checks a new API key against Apollo and swaps it in, then
// returns the refreshed overview. A key Apollo refuses answers 422. It is a
// PUT, so a retry converges.
func (s *IntegrationService) ReplaceApolloKey(ctx context.Context, id, apiKey string, opts ...RequestOption) (*ApolloOverview, *Response, error) {
	body := struct {
		APIKey string `json:"api_key"`
	}{APIKey: apiKey}
	return send[ApolloOverview](ctx, s.client.put, "integrations/apollo/"+url.PathEscape(id)+"/key", body, opts)
}

// ApolloLists returns the account's saved contact lists.
func (s *IntegrationService) ApolloLists(ctx context.Context, id string, opts ...RequestOption) ([]ApolloList, *Response, error) {
	return fetchData[ApolloList](ctx, s.client, "integrations/apollo/"+url.PathEscape(id)+"/lists", opts)
}

// PreviewApolloImport reads the first people of a list without importing.
func (s *IntegrationService) PreviewApolloImport(ctx context.Context, id string, params *ApolloPreviewParams, opts ...RequestOption) (*ApolloPreview, *Response, error) {
	return send[ApolloPreview](ctx, s.client.post, "integrations/apollo/"+url.PathEscape(id)+"/import/preview", params, opts)
}

// ApolloImportSources lists a connection's saved list imports.
func (s *IntegrationService) ApolloImportSources(ctx context.Context, id string, opts ...RequestOption) ([]ApolloImportSource, *Response, error) {
	return fetchData[ApolloImportSource](ctx, s.client, "integrations/apollo/"+url.PathEscape(id)+"/import-sources", opts)
}

// CreateApolloImportSource saves a list import and starts its first run. A
// retried create makes a second source whose run imports nothing new, since
// people are deduplicated by address.
func (s *IntegrationService) CreateApolloImportSource(ctx context.Context, id string, params *ApolloImportSourceParams, opts ...RequestOption) (*ApolloImportSource, *Response, error) {
	return send[ApolloImportSource](ctx, s.client.post, "integrations/apollo/"+url.PathEscape(id)+"/import-sources", params, opts)
}

// UpdateApolloImportSource edits a list import's targets, filter and schedule.
func (s *IntegrationService) UpdateApolloImportSource(ctx context.Context, id, sourceID string, params *ApolloImportSourceUpdate, opts ...RequestOption) (*ApolloImportSource, *Response, error) {
	return send[ApolloImportSource](ctx, s.client.patch, "integrations/apollo/"+url.PathEscape(id)+"/import-sources/"+url.PathEscape(sourceID), params.body(), opts)
}

// RunApolloImportSource starts a list import now and returns the source with
// its status set to running. A second request while one runs answers 409.
func (s *IntegrationService) RunApolloImportSource(ctx context.Context, id, sourceID string, opts ...RequestOption) (*ApolloImportSource, *Response, error) {
	return send[ApolloImportSource](ctx, s.client.post, "integrations/apollo/"+url.PathEscape(id)+"/import-sources/"+url.PathEscape(sourceID)+"/run", nil, opts)
}

// DeleteApolloImportSource removes a list import. The contacts it brought in
// stay.
func (s *IntegrationService) DeleteApolloImportSource(ctx context.Context, id, sourceID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "integrations/apollo/"+url.PathEscape(id)+"/import-sources/"+url.PathEscape(sourceID), opts...)
}

// ApolloEnrichments lists a connection's recent enrichment runs. limit is 1 to
// 100; zero uses the server default of 20.
func (s *IntegrationService) ApolloEnrichments(ctx context.Context, id string, limit int, opts ...RequestOption) ([]ApolloEnrichmentRun, *Response, error) {
	q := make(url.Values)
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	return fetchData[ApolloEnrichmentRun](ctx, s.client, withQuery("integrations/apollo/"+url.PathEscape(id)+"/enrichments", q), opts)
}

// StartApolloEnrichment queues an enrichment run over the given contacts and
// returns it with the server's 202 answer. Each match spends the customer's
// Apollo credits, so it needs [PermBulkContacts] as well as [PermIntegrations].
// Pass an [WithIdempotencyKey] option to keep a retry from queuing a second
// run. A run covers at most 5000 contacts.
func (s *IntegrationService) StartApolloEnrichment(ctx context.Context, id string, params *ApolloEnrichParams, opts ...RequestOption) (*ApolloEnrichmentRun, *Response, error) {
	return send[ApolloEnrichmentRun](ctx, s.client.post, "integrations/apollo/"+url.PathEscape(id)+"/enrichments", params, opts)
}

// ApolloEnrichment returns one enrichment run's progress.
func (s *IntegrationService) ApolloEnrichment(ctx context.Context, id, runID string, opts ...RequestOption) (*ApolloEnrichmentRun, *Response, error) {
	return fetch[ApolloEnrichmentRun](ctx, s.client, "integrations/apollo/"+url.PathEscape(id)+"/enrichments/"+url.PathEscape(runID), opts)
}
