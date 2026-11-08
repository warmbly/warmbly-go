package warmbly

import (
	"context"
	"io"
	"net/url"
	"time"
)

// Background contact imports. Unlike the synchronous
// [ContactService.ImportPreview] and [ContactService.ImportCommit] pair, which
// need the file twice, a background import is uploaded once as a draft,
// analyzed under a mapping without writing anything, then started and run in
// chunks on the server. Poll [ContactService.GetImport] (or watch the gateway)
// until [ContactImport.Status] is terminal.
//
// Reads take the READ_CONTACTS scope, drafting and analyzing take
// WRITE_CONTACTS, and starting or canceling takes BULK_CONTACTS.

// Contact import states returned in [ContactImport.Status].
const (
	// ImportStatusDraft is uploaded and waiting for a mapping and a start.
	ImportStatusDraft     = "draft"
	ImportStatusQueued    = "queued"
	ImportStatusRunning   = "running"
	ImportStatusCompleted = "completed"
	ImportStatusFailed    = "failed"
	ImportStatusCancelled = "cancelled" //nolint:misspell // wire value
)

// Mapping sources reported in [ContactImportPreview.MappingSource].
const (
	// ImportMappingSuggested means the mapping was guessed from the file.
	ImportMappingSuggested = "suggested"
	// ImportMappingSaved means the workspace confirmed a mapping for these
	// exact headers before.
	ImportMappingSaved = "saved"
)

// ImportTerminal reports whether a [ContactImport.Status] is final: the
// import will never change again.
func ImportTerminal(status string) bool {
	switch status {
	case ImportStatusCompleted, ImportStatusFailed, ImportStatusCancelled:
		return true
	}
	return false
}

// ContactImportColumnStats is one column's fill across the whole file.
type ContactImportColumnStats struct {
	Filled int `json:"filled"`
	// Distinct counts different non-empty values, capped at 1,000.
	Distinct int `json:"distinct"`
	// Samples are a few distinct non-empty values, in file order.
	Samples []string `json:"samples"`
}

// ContactImport is one background import of a contact file.
type ContactImport struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organization_id"`
	CreatedBy      *string `json:"created_by,omitempty"`
	Filename       string  `json:"filename"`
	Format         string  `json:"format"`
	// Status is one of the ImportStatus* constants.
	Status    string   `json:"status"`
	HasHeader bool     `json:"has_header"`
	Columns   []string `json:"columns"`
	// Total is the number of data rows and Processed how many have settled.
	Total     int `json:"total"`
	Processed int `json:"processed"`
	Imported  int `json:"imported"`
	Updated   int `json:"updated"`
	Skipped   int `json:"skipped"`
	Failed    int `json:"failed"`

	// Options is the saved mapping and settings, once a draft has been
	// saved or the import started.
	Options *ContactImportParams  `json:"options,omitempty"`
	Quality *ContactImportQuality `json:"quality,omitempty"`
	// SegmentsPinned is nil when the import had no segment targets.
	SegmentsPinned *bool    `json:"segments_pinned,omitempty"`
	Notes          []string `json:"notes"`
	Error          string   `json:"error,omitempty"`

	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`

	// Preview is what a mapper shows. Only a draft carries it.
	Preview *ContactImportPreview `json:"preview,omitempty"`
	// Failures are the first 200 failed rows once the import has finished;
	// [ContactService.DownloadImportFailures] returns every one.
	Failures []ImportRowError `json:"failures,omitempty"`
}

// ContactImportAnalyzeParams asks what a draft would do under a mapping.
type ContactImportAnalyzeParams struct {
	Mapping   []ImportColumnMapping `json:"mapping"`
	HasHeader bool                  `json:"has_header"`
}

// ContactImportAnalysis is what an import would do, read over the whole file
// before anything is written. Every row lands in exactly one of New, Existing,
// DuplicatesInFile, Invalid or Conflicts.
type ContactImportAnalysis struct {
	Rows int `json:"rows"`
	// New addresses the workspace does not have yet.
	New int `json:"new"`
	// Existing addresses the workspace already has a contact for.
	Existing int `json:"existing"`
	// DuplicatesInFile repeat an address an earlier row holds.
	DuplicatesInFile int `json:"duplicates_in_file"`
	// Invalid rows have no usable address or an unreadable value.
	Invalid int `json:"invalid"`
	// Conflicts are addresses the importing member already holds as a contact
	// in another workspace, which cannot be created here.
	Conflicts int `json:"conflicts"`
	// InvalidSamples are the first 25 invalid or conflicting rows, with
	// reasons.
	InvalidSamples []ImportRowError      `json:"invalid_samples"`
	Quality        *ContactImportQuality `json:"quality,omitempty"`
	// Problem is why starting would be refused as a whole (the plan's contact
	// limit, too many new categories). Empty when the import would run.
	Problem string `json:"problem,omitempty"`
}

// CreateImport uploads a CSV or XLSX file (at most 50 MB) once and returns a
// draft with the preview a column mapper needs. Nothing is imported until
// [ContactService.StartImport]. It needs a user-backed credential, because
// the draft belongs to the caller.
//
// Requires the WRITE_CONTACTS scope.
func (s *ContactService) CreateImport(ctx context.Context, file *FileUpload, opts ...RequestOption) (*ContactImport, *Response, error) {
	out := new(ContactImport)
	resp, err := s.client.postMultipart(ctx, "contacts/imports", "file", file, nil, out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// ListImports returns a page of background imports, newest first. The page
// limit is 1 to 100 and defaults to 50.
//
// Requires the READ_CONTACTS scope.
func (s *ContactService) ListImports(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[ContactImport], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[ContactImport](ctx, s.client, "contacts/imports", q, opts...)
}

// GetImport returns one background import with its progress.
//
// Requires the READ_CONTACTS scope.
func (s *ContactService) GetImport(ctx context.Context, id string, opts ...RequestOption) (*ContactImport, *Response, error) {
	return fetch[ContactImport](ctx, s.client, "contacts/imports/"+url.PathEscape(id), opts)
}

// SaveImportDraft autosaves a draft's mapping and settings so a reload
// resumes it. Safe to repeat.
//
// Requires the WRITE_CONTACTS scope.
func (s *ContactService) SaveImportDraft(ctx context.Context, id string, params *ContactImportParams, opts ...RequestOption) (*ContactImport, *Response, error) {
	return send[ContactImport](ctx, s.client.patch, "contacts/imports/"+url.PathEscape(id), params, opts)
}

// AnalyzeImport reads the whole draft under a mapping and reports what
// starting it would do, writing nothing. It needs a user-backed credential.
//
// Requires the WRITE_CONTACTS scope.
func (s *ContactService) AnalyzeImport(ctx context.Context, id string, params *ContactImportAnalyzeParams, opts ...RequestOption) (*ContactImportAnalysis, *Response, error) {
	return send[ContactImportAnalysis](ctx, s.client.post, "contacts/imports/"+url.PathEscape(id)+"/analyze", params, opts)
}

// StartImport starts a draft with the given mapping and settings. Starting an
// import that already started returns it unchanged, so a retry is safe. It
// needs a user-backed credential.
//
// Requires the BULK_CONTACTS scope.
func (s *ContactService) StartImport(ctx context.Context, id string, params *ContactImportParams, opts ...RequestOption) (*ContactImport, *Response, error) {
	return send[ContactImport](ctx, s.client.post, "contacts/imports/"+url.PathEscape(id)+"/start", params, opts)
}

// CancelImport stops a queued or running import. Rows already written stay
// written.
//
// Requires the BULK_CONTACTS scope.
func (s *ContactService) CancelImport(ctx context.Context, id string, opts ...RequestOption) (*ContactImport, *Response, error) {
	return send[ContactImport](ctx, s.client.post, "contacts/imports/"+url.PathEscape(id)+"/cancel", nil, opts)
}

// DownloadImportFailures streams the import's failed rows, with the reason
// for each, to w as CSV.
//
// Requires the READ_CONTACTS scope.
func (s *ContactService) DownloadImportFailures(ctx context.Context, id string, w io.Writer, opts ...RequestOption) (*Response, error) {
	return s.client.get(ctx, "contacts/imports/"+url.PathEscape(id)+"/failed.csv", w, opts...)
}
