package warmbly

import (
	"context"
	"net/url"
	"strconv"
	"time"
)

// ApolloService manages a connected Apollo account: its saved contact lists,
// scheduled list imports into Warmbly, and enrichment of existing contacts
// through Apollo's people match.
//
// Connect Apollo first with [IntegrationService.Connect] and
// [ProviderApollo], passing the API key as config.api_key. Every method takes
// the connection id and answers 404 for a connection that is not an Apollo
// one, and 503 on an instance without Apollo.
//
// All methods take the INTEGRATIONS scope. Starting an enrichment also takes
// BULK_CONTACTS, because it writes contacts and spends your Apollo credits.
type ApolloService service

// Apollo enrichment run states returned in [ApolloEnrichmentRun.Status].
const (
	ApolloEnrichmentQueued  = "queued"
	ApolloEnrichmentRunning = "running"
	ApolloEnrichmentDone    = "done"
	ApolloEnrichmentError   = "error"
)

// ApolloOverview is the Apollo page header: the account the key acts as and
// what the connection has done.
type ApolloOverview struct {
	ConnectionID string `json:"connection_id"`
	Label        string `json:"label"`
	// Status is one of the Connection* constants.
	Status       string `json:"status"`
	Health       string `json:"health"`
	HealthDetail string `json:"health_detail,omitempty"`
	AccountName  string `json:"account_name,omitempty"`
	AccountEmail string `json:"account_email,omitempty"`
	// Sources counts the saved list imports.
	Sources       int        `json:"sources"`
	TotalImported int        `json:"total_imported"`
	LastRunAt     *time.Time `json:"last_run_at,omitempty"`
}

// ApolloList is one saved Apollo list of contacts.
type ApolloList struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// ApolloImportPreviewParams names the list to preview.
type ApolloImportPreviewParams struct {
	ListID string `json:"list_id"`
	// VerifiedOnly keeps only people whose address Apollo has verified. Nil
	// means true.
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
	// AlreadyInWarmbly is set when the address is already a contact.
	AlreadyInWarmbly bool `json:"already_in_warmbly"`
	// WillImport is set when a run would import this person.
	WillImport bool `json:"will_import"`
}

// ApolloImportPreview is the first people of a list and how many it holds.
type ApolloImportPreview struct {
	Total  int                `json:"total"`
	Sample []ApolloPreviewRow `json:"sample"`
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
	// Truncated is set when the list held more than one run reads.
	Truncated   bool `json:"truncated,omitempty"`
	AlreadySeen int  `json:"already_seen"`
	// NextPage is where the next run resumes reading a list too long for one.
	NextPage int `json:"next_page,omitempty"`
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
	// CampaignID is the campaign new contacts are enrolled in, if any.
	CampaignID   *string  `json:"campaign_id,omitempty"`
	CategoryIDs  []string `json:"category_ids"`
	VerifiedOnly bool     `json:"verified_only"`
	Recurring    bool     `json:"recurring"`
	Enabled      bool     `json:"enabled"`
	// Status is one of the ImportSource* constants.
	Status        string           `json:"status"`
	LastRunAt     *time.Time       `json:"last_run_at,omitempty"`
	LastResult    *ApolloRunResult `json:"last_result,omitempty"`
	LastError     string           `json:"last_error,omitempty"`
	TotalImported int              `json:"total_imported"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

// ApolloImportSourceParams creates or edits a list import. On create, ListID
// is required (at most 64 characters) and the first run starts at once. On
// edit, empty strings and nil pointers are left unchanged.
type ApolloImportSourceParams struct {
	Name     *string `json:"name,omitempty"`
	ListID   string  `json:"list_id,omitempty"`
	ListName string  `json:"list_name,omitempty"`
	// CampaignID enrolls newly imported contacts in a campaign.
	CampaignID *string `json:"-"`
	// ClearCampaign removes the campaign target on edit.
	ClearCampaign bool     `json:"-"`
	CategoryIDs   []string `json:"category_ids,omitempty"`
	VerifiedOnly  *bool    `json:"verified_only,omitempty"`
	Recurring     *bool    `json:"recurring,omitempty"`
	Enabled       *bool    `json:"enabled,omitempty"`
}

// MarshalJSON sends campaign_id as null when ClearCampaign is set, which the
// server reads as "remove the target", and omits it when it is unset.
func (p ApolloImportSourceParams) MarshalJSON() ([]byte, error) {
	type plain ApolloImportSourceParams
	return marshalWithCampaign(plain(p), p.CampaignID, p.ClearCampaign)
}

// ApolloEnrichmentRun is one background request to enrich Warmbly contacts
// through Apollo's people match.
type ApolloEnrichmentRun struct {
	ID                string  `json:"id"`
	OrganizationID    string  `json:"organization_id"`
	ConnectionID      string  `json:"connection_id"`
	RequestedByUserID *string `json:"requested_by_user_id,omitempty"`
	// Overwrite replaces existing values instead of only filling blanks.
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

// ApolloEnrichParams chooses the contacts to enrich: an id list, or every
// contact a search matches ("select all"). At most 5,000 contacts per run.
type ApolloEnrichParams struct {
	IntegrationContactSelection
	// Overwrite replaces existing values instead of only filling blanks.
	Overwrite bool `json:"overwrite,omitempty"`
}

func (s *ApolloService) base(connID string) string {
	return "integrations/apollo/" + url.PathEscape(connID)
}

// Overview returns the Apollo page header.
func (s *ApolloService) Overview(ctx context.Context, connectionID string, opts ...RequestOption) (*ApolloOverview, *Response, error) {
	return fetch[ApolloOverview](ctx, s.client, s.base(connectionID), opts)
}

// ReplaceKey checks a new API key against Apollo and swaps it in, returning
// the refreshed overview. A mistyped key is a 422 and the old key stays. Safe
// to repeat. Needs the settings-management right as well as the INTEGRATIONS
// scope.
func (s *ApolloService) ReplaceKey(ctx context.Context, connectionID, apiKey string, opts ...RequestOption) (*ApolloOverview, *Response, error) {
	body := struct {
		APIKey string `json:"api_key"`
	}{APIKey: apiKey}
	return send[ApolloOverview](ctx, s.client.put, s.base(connectionID)+"/key", body, opts)
}

// Lists returns the account's saved contact lists.
func (s *ApolloService) Lists(ctx context.Context, connectionID string, opts ...RequestOption) ([]ApolloList, *Response, error) {
	return fetchData[ApolloList](ctx, s.client, s.base(connectionID)+"/lists", opts)
}

// PreviewImport reads the first people of a list and how many it holds. It
// writes nothing.
func (s *ApolloService) PreviewImport(ctx context.Context, connectionID string, params *ApolloImportPreviewParams, opts ...RequestOption) (*ApolloImportPreview, *Response, error) {
	return send[ApolloImportPreview](ctx, s.client.post, s.base(connectionID)+"/import/preview", params, opts)
}

// ImportSources lists the connection's saved list imports.
func (s *ApolloService) ImportSources(ctx context.Context, connectionID string, opts ...RequestOption) ([]ApolloImportSource, *Response, error) {
	return fetchData[ApolloImportSource](ctx, s.client, s.base(connectionID)+"/import-sources", opts)
}

// CreateImportSource saves a list import and starts its first run. A retried
// create makes a second source whose run imports nothing new, since people
// are deduplicated by address.
func (s *ApolloService) CreateImportSource(ctx context.Context, connectionID string, params *ApolloImportSourceParams, opts ...RequestOption) (*ApolloImportSource, *Response, error) {
	return send[ApolloImportSource](ctx, s.client.post, s.base(connectionID)+"/import-sources", params, opts)
}

// UpdateImportSource edits a list import's targets, filter and schedule.
func (s *ApolloService) UpdateImportSource(ctx context.Context, connectionID, sourceID string, params *ApolloImportSourceParams, opts ...RequestOption) (*ApolloImportSource, *Response, error) {
	return send[ApolloImportSource](ctx, s.client.patch, s.base(connectionID)+"/import-sources/"+url.PathEscape(sourceID), params, opts)
}

// RunImportSource runs a list import now and returns it as running. A second
// call while one runs is refused with 409, so a retry cannot double-run it.
func (s *ApolloService) RunImportSource(ctx context.Context, connectionID, sourceID string, opts ...RequestOption) (*ApolloImportSource, *Response, error) {
	return send[ApolloImportSource](ctx, s.client.post, s.base(connectionID)+"/import-sources/"+url.PathEscape(sourceID)+"/run", nil, opts)
}

// DeleteImportSource removes a list import. Contacts it brought in stay.
func (s *ApolloService) DeleteImportSource(ctx context.Context, connectionID, sourceID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, s.base(connectionID)+"/import-sources/"+url.PathEscape(sourceID), opts...)
}

// Enrichments lists the connection's recent enrichment runs, newest first.
// limit is 1 to 100 and defaults to 20 when zero.
func (s *ApolloService) Enrichments(ctx context.Context, connectionID string, limit int, opts ...RequestOption) ([]ApolloEnrichmentRun, *Response, error) {
	q := make(url.Values)
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	return fetchData[ApolloEnrichmentRun](ctx, s.client, withQuery(s.base(connectionID)+"/enrichments", q), opts)
}

// Enrichment returns one enrichment run's progress.
func (s *ApolloService) Enrichment(ctx context.Context, connectionID, runID string, opts ...RequestOption) (*ApolloEnrichmentRun, *Response, error) {
	return fetch[ApolloEnrichmentRun](ctx, s.client, s.base(connectionID)+"/enrichments/"+url.PathEscape(runID), opts)
}

// StartEnrichment queues an enrichment run over a contact selection and
// answers 202 with the run, which proceeds in the background; poll
// [ApolloService.Enrichment]. It spends Apollo credits. Send an
// Idempotency-Key to make a retry safe; without one a retry queues a second
// run, which only rewrites the same fields.
//
// Requires the INTEGRATIONS and BULK_CONTACTS scopes.
func (s *ApolloService) StartEnrichment(ctx context.Context, connectionID string, params *ApolloEnrichParams, opts ...RequestOption) (*ApolloEnrichmentRun, *Response, error) {
	return send[ApolloEnrichmentRun](ctx, s.client.post, s.base(connectionID)+"/enrichments", params, opts)
}
