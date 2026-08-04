package warmbly

import (
	"context"
	"net/url"
	"time"
)

// LeadSyncService manages on-demand Google Sheets to contacts sync: a saved
// binding between a spreadsheet tab and the contact importer, re-runnable with
// "sync now". New rows create contacts and rows matching an existing contact by
// email update it.
//
// The Google account itself is connected through the ordinary integration OAuth
// flow with provider [ProviderGoogleSheets]; see
// [IntegrationService.StartOAuth].
//
// Setting up a source runs in three steps: [LeadSyncService.Spreadsheet] to
// list a workbook's tabs, [LeadSyncService.Preview] to read the columns and get
// a suggested mapping, then [LeadSyncService.Create] to save it. Preview
// returns the same shape as the contact importer, so one column mapper serves
// both.
type LeadSyncService service

// Source states returned in [LeadSyncSource.Status].
const (
	// LeadSyncIdle means never synced, or the last sync succeeded.
	LeadSyncIdle = "idle"
	// LeadSyncSyncing means a sync is in flight.
	LeadSyncSyncing = "syncing"
	// LeadSyncError means the last sync failed; see
	// [LeadSyncSource.LastError].
	LeadSyncError = "error"
)

// LeadSyncConnection reports whether the workspace has a Google account
// connected for lead sync.
type LeadSyncConnection struct {
	Connected  bool                       `json:"connected"`
	Connection *LeadSyncConnectionSummary `json:"connection"`
}

// LeadSyncConnectionSummary identifies the connected Google account.
type LeadSyncConnectionSummary struct {
	ID                  string `json:"id"`
	ExternalAccountName string `json:"external_account_name"`
	// Status is one of the Connection* constants.
	Status string `json:"status"`
}

// Spreadsheet is a workbook's title and its tabs.
type Spreadsheet struct {
	SheetID string           `json:"sheet_id"`
	Title   string           `json:"title"`
	Tabs    []SpreadsheetTab `json:"tabs"`
}

// SpreadsheetTab is one tab in a workbook.
type SpreadsheetTab struct {
	Title string `json:"title"`
	Index int    `json:"index"`
}

// LeadSyncSource is a saved sheet-to-contacts binding.
type LeadSyncSource struct {
	ID              string `json:"id"`
	OrganizationID  string `json:"organization_id"`
	CreatedByUserID string `json:"created_by_user_id"`
	// Provider is the source system; today always [ProviderGoogleSheets].
	Provider string `json:"provider"`
	// ConnectionID is the integration connection the sheet is read through.
	ConnectionID string `json:"connection_id"`

	SheetID    string `json:"sheet_id"`
	SheetTitle string `json:"sheet_title,omitempty"`
	TabTitle   string `json:"tab_title,omitempty"`
	// A1Range narrows the read to a range within the tab.
	A1Range   string `json:"a1_range,omitempty"`
	HasHeader bool   `json:"has_header"`

	// ColumnMapping and Dedup are the contact-importer settings the rows flow
	// through, so a sheet sync behaves exactly like a file import.
	ColumnMapping []ImportColumnMapping `json:"column_mapping"`
	Dedup         string                `json:"dedup"`

	// TargetCampaignID, when set, enrolls every new or updated lead in that
	// campaign on each sync.
	TargetCampaignID  *string  `json:"target_campaign_id,omitempty"`
	CategoryIDs       []string `json:"category_ids"`
	SubscribedDefault bool     `json:"subscribed_default"`

	Label string `json:"label,omitempty"`
	// Status is one of the LeadSync* constants.
	Status string `json:"status"`

	LastSyncedAt *time.Time           `json:"last_synced_at,omitempty"`
	LastResult   *ContactImportResult `json:"last_result,omitempty"`
	LastError    string               `json:"last_error,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LeadSyncCreateParams saves a new source.
type LeadSyncCreateParams struct {
	ConnectionID string `json:"connection_id"`
	SheetID      string `json:"sheet_id"`
	SheetTitle   string `json:"sheet_title,omitempty"`
	TabTitle     string `json:"tab_title,omitempty"`
	HasHeader    bool   `json:"has_header"`

	ColumnMapping []ImportColumnMapping `json:"column_mapping"`
	// Dedup is one of the ImportDedup* constants.
	Dedup string `json:"dedup,omitempty"`

	TargetCampaignID  *string  `json:"target_campaign_id,omitempty"`
	CategoryIDs       []string `json:"category_ids,omitempty"`
	SubscribedDefault *bool    `json:"subscribed_default,omitempty"`
	Label             string   `json:"label,omitempty"`
}

// LeadSyncUpdateParams edits a saved source. Nil fields are left unchanged.
type LeadSyncUpdateParams struct {
	SheetID    *string `json:"sheet_id,omitempty"`
	SheetTitle *string `json:"sheet_title,omitempty"`
	TabTitle   *string `json:"tab_title,omitempty"`
	HasHeader  *bool   `json:"has_header,omitempty"`

	ColumnMapping *[]ImportColumnMapping `json:"column_mapping,omitempty"`
	Dedup         *string                `json:"dedup,omitempty"`

	TargetCampaignID *string `json:"target_campaign_id,omitempty"`
	// ClearCampaign detaches the target campaign. Use it instead of a nil
	// TargetCampaignID, which means "leave unchanged".
	ClearCampaign     bool      `json:"clear_campaign,omitempty"`
	CategoryIDs       *[]string `json:"category_ids,omitempty"`
	SubscribedDefault *bool     `json:"subscribed_default,omitempty"`
	Label             *string   `json:"label,omitempty"`
}

// LeadSyncResult is what a manual sync returns: the import counts plus the
// source they belong to.
type LeadSyncResult struct {
	SourceID string               `json:"source_id"`
	Result   *ContactImportResult `json:"result"`
}

// Connection reports whether a Google account is connected for lead sync.
func (s *LeadSyncService) Connection(ctx context.Context, opts ...RequestOption) (*LeadSyncConnection, *Response, error) {
	return fetch[LeadSyncConnection](ctx, s.client, "lead-sync/google/connection", opts)
}

// Spreadsheet returns a workbook's title and tabs, so a tab can be chosen
// before mapping columns.
func (s *LeadSyncService) Spreadsheet(ctx context.Context, connectionID, sheetID string, opts ...RequestOption) (*Spreadsheet, *Response, error) {
	body := struct {
		ConnectionID string `json:"connection_id"`
		SheetID      string `json:"sheet_id"`
	}{ConnectionID: connectionID, SheetID: sheetID}
	return send[Spreadsheet](ctx, s.client.post, "lead-sync/google/spreadsheet", body, opts)
}

// Preview reads the top rows of a tab and returns the same shape as
// [ContactService.ImportPreview], including a suggested column mapping.
func (s *LeadSyncService) Preview(ctx context.Context, connectionID, sheetID, tabTitle string, opts ...RequestOption) (*ContactImportPreview, *Response, error) {
	body := struct {
		ConnectionID string `json:"connection_id"`
		SheetID      string `json:"sheet_id"`
		TabTitle     string `json:"tab_title,omitempty"`
	}{ConnectionID: connectionID, SheetID: sheetID, TabTitle: tabTitle}
	return send[ContactImportPreview](ctx, s.client.post, "lead-sync/google/preview", body, opts)
}

// Sources returns the workspace's saved sync sources.
func (s *LeadSyncService) Sources(ctx context.Context, opts ...RequestOption) ([]LeadSyncSource, *Response, error) {
	return fetchData[LeadSyncSource](ctx, s.client, "lead-sync/sources", opts)
}

// Create saves a new sync source.
func (s *LeadSyncService) Create(ctx context.Context, params *LeadSyncCreateParams, opts ...RequestOption) (*LeadSyncSource, *Response, error) {
	return send[LeadSyncSource](ctx, s.client.post, "lead-sync/sources", params, opts)
}

// Get retrieves a saved sync source.
func (s *LeadSyncService) Get(ctx context.Context, id string, opts ...RequestOption) (*LeadSyncSource, *Response, error) {
	return fetch[LeadSyncSource](ctx, s.client, "lead-sync/sources/"+url.PathEscape(id), opts)
}

// Update edits a saved sync source.
func (s *LeadSyncService) Update(ctx context.Context, id string, params *LeadSyncUpdateParams, opts ...RequestOption) (*LeadSyncSource, *Response, error) {
	return send[LeadSyncSource](ctx, s.client.patch, "lead-sync/sources/"+url.PathEscape(id), params, opts)
}

// Delete removes a saved sync source. The contacts it imported stay.
func (s *LeadSyncService) Delete(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "lead-sync/sources/"+url.PathEscape(id), opts...)
}

// Sync runs a source now and returns the import counts. It runs in the
// request, so a large sheet takes a while.
func (s *LeadSyncService) Sync(ctx context.Context, id string, opts ...RequestOption) (*LeadSyncResult, *Response, error) {
	return send[LeadSyncResult](ctx, s.client.post, "lead-sync/sources/"+url.PathEscape(id)+"/sync", nil, opts)
}
