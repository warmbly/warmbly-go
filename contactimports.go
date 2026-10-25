package warmbly

import (
	"context"
	"errors"
	"io"
	"net/url"
	"time"
)

// Where a [ContactImportPreview.SuggestedMapping] came from, in
// [ContactImportPreview.MappingSource]. Empty when the server did not say.
const (
	// ContactImportMappingSuggested is the server's own header and value
	// heuristics.
	ContactImportMappingSuggested = "suggested"
	// ContactImportMappingSaved is a mapping the workspace confirmed before for
	// a file with exactly these headers.
	ContactImportMappingSaved = "saved"
)

// Lifecycle states returned in [ContactImport.Status]. A status this SDK
// release does not know still decodes; compare against the constants rather
// than switching exhaustively.
const (
	// ContactImportStatusDraft is uploaded and waiting for a mapping and a
	// [ContactService.StartImport].
	ContactImportStatusDraft = "draft"
	// ContactImportStatusQueued is started and waiting for the runner.
	ContactImportStatusQueued = "queued"
	// ContactImportStatusRunning is being applied in chunks.
	ContactImportStatusRunning = "running"
	// ContactImportStatusCompleted finished; some rows may still have failed.
	ContactImportStatusCompleted = "completed"
	// ContactImportStatusFailed stopped on an error ([ContactImport.Error]).
	ContactImportStatusFailed = "failed"
	// ContactImportStatusCancelled was stopped by [ContactService.CancelImport].
	ContactImportStatusCancelled = "cancelled" //nolint:misspell // the server's spelling on the wire
)

// ContactImportColumnStats is one column's fill across the whole file, not just
// the sample rows.
type ContactImportColumnStats struct {
	// Filled counts rows with a non-empty value in the column.
	Filled int `json:"filled"`
	// Distinct counts different non-empty values, capped at 1,000.
	Distinct int `json:"distinct"`
	// Samples are a few distinct non-empty values, in file order.
	Samples []string `json:"samples"`
}

// ContactImport is one background import of a contact file: uploaded once as a
// draft, analyzed, started, and applied in the background so it survives a
// closed tab. Follow it with [ContactService.GetImport] or the gateway's
// contact-import progress event.
type ContactImport struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	// CreatedBy is the member who uploaded the file, nil when unknown.
	CreatedBy *string `json:"created_by,omitempty"`
	Filename  string  `json:"filename"`
	Format    string  `json:"format"`
	// Status is one of the ContactImportStatus* constants.
	Status    string   `json:"status"`
	HasHeader bool     `json:"has_header"`
	Columns   []string `json:"columns"`

	// Total is the number of data rows and Processed how many have settled.
	// Imported, Updated, Skipped and Failed split the settled rows.
	Total     int `json:"total"`
	Processed int `json:"processed"`
	Imported  int `json:"imported"`
	Updated   int `json:"updated"`
	Skipped   int `json:"skipped"`
	Failed    int `json:"failed"`

	// Options are the mapping and settings the import was saved or started
	// with; nil until a draft has been saved.
	Options *ContactImportParams `json:"options,omitempty"`
	// Quality is the file's address-level assessment, once measured.
	Quality *ContactImportQuality `json:"quality,omitempty"`
	// SegmentsPinned is nil when the import had no segment targets. With
	// targets it is true when every membership write landed.
	SegmentsPinned *bool `json:"segments_pinned,omitempty"`
	// Notes are per-import remarks the runner recorded.
	Notes []string `json:"notes"`
	// Error is why the import ended in [ContactImportStatusFailed].
	Error string `json:"error,omitempty"`

	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`

	// Preview is what the column mapper shows. Only a draft carries it, so a
	// reload resumes the mapping step.
	Preview *ContactImportPreview `json:"preview,omitempty"`
	// Failures are the first 200 failed rows of a finished import;
	// [ContactService.DownloadImportFailures] has every one.
	Failures []ImportRowError `json:"failures,omitempty"`
}

// Terminal reports whether the import will never change again.
func (i *ContactImport) Terminal() bool {
	switch i.Status {
	case ContactImportStatusCompleted, ContactImportStatusFailed, ContactImportStatusCancelled:
		return true
	}
	return false
}

// ContactImportAnalyzeParams asks what a draft would do under a mapping.
type ContactImportAnalyzeParams struct {
	Mapping []ImportColumnMapping `json:"mapping"`
	// HasHeader says whether the first row is a header rather than data.
	HasHeader bool `json:"has_header"`
}

// ContactImportAnalysis is what an import would do, measured over the whole
// file before anything is written. Every row lands in exactly one of New,
// Existing, DuplicatesInFile, Invalid or Conflicts.
type ContactImportAnalysis struct {
	// Rows is every data row.
	Rows int `json:"rows"`
	// New are addresses the workspace does not have yet.
	New int `json:"new"`
	// Existing are addresses the workspace already holds a contact for.
	Existing int `json:"existing"`
	// DuplicatesInFile are rows repeating an address an earlier row holds.
	DuplicatesInFile int `json:"duplicates_in_file"`
	// Invalid rows have no usable address or a value that cannot be read.
	Invalid int `json:"invalid"`
	// Conflicts are addresses the importing member already holds as a contact
	// in another workspace, which cannot be created here.
	Conflicts int `json:"conflicts"`
	// InvalidSamples are the first 25 invalid or conflicting rows, with
	// reasons.
	InvalidSamples []ImportRowError      `json:"invalid_samples"`
	Quality        *ContactImportQuality `json:"quality,omitempty"`
	// Problem says why starting this import would be refused as a whole (the
	// plan's contact limit, too many new labels). Empty when it would run.
	Problem string `json:"problem,omitempty"`
}

// CreateImport uploads a CSV or XLSX file as a background-import draft and
// returns it with the preview the column mapper needs (201). The multipart
// field is "file". Files are capped at 50 MB and 50,000 data rows, and a
// workspace may hold only a few imports waiting or running at once (409
// otherwise). Nothing is written to the workspace's contacts yet; continue with
// [ContactService.AnalyzeImport] and [ContactService.StartImport].
//
// Requires the manage-contacts permission ([PermWriteContacts] for an API key).
func (s *ContactService) CreateImport(ctx context.Context, file *FileUpload, opts ...RequestOption) (*ContactImport, *Response, error) {
	out := new(ContactImport)
	resp, err := s.client.postMultipart(ctx, "contacts/imports", "file", file, nil, out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// ListImports returns a page of the workspace's background imports, newest
// first. The page size defaults to 50 and may be 1 to 100. Rows do not carry
// the draft preview or the failed-row list; fetch one with
// [ContactService.GetImport] for those.
//
// Requires the view-contacts permission ([PermReadContacts] for an API key).
func (s *ContactService) ListImports(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[ContactImport], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[ContactImport](ctx, s.client, "contacts/imports", q, opts...)
}

// GetImport returns one background import, including its first failed rows once
// it has finished.
//
// Requires the view-contacts permission ([PermReadContacts] for an API key).
func (s *ContactService) GetImport(ctx context.Context, id string, opts ...RequestOption) (*ContactImport, *Response, error) {
	return fetch[ContactImport](ctx, s.client, "contacts/imports/"+url.PathEscape(id), opts)
}

// SaveImportDraft autosaves a draft's mapping and settings so a reload resumes
// it. Nothing is validated until [ContactService.StartImport]. A draft that has
// already started answers 409.
//
// Requires the manage-contacts permission ([PermWriteContacts] for an API key).
func (s *ContactService) SaveImportDraft(ctx context.Context, id string, params *ContactImportParams, opts ...RequestOption) (*ContactImport, *Response, error) {
	if params == nil {
		params = &ContactImportParams{}
	}
	return send[ContactImport](ctx, s.client.patch, "contacts/imports/"+url.PathEscape(id), params, opts)
}

// AnalyzeImport reads the whole file under a mapping and reports what the
// import would do, writing nothing. Only a draft can be analyzed; one that has
// started answers 409.
//
// Requires the manage-contacts permission ([PermWriteContacts] for an API key).
func (s *ContactService) AnalyzeImport(ctx context.Context, id string, params *ContactImportAnalyzeParams, opts ...RequestOption) (*ContactImportAnalysis, *Response, error) {
	if params == nil {
		params = &ContactImportAnalyzeParams{}
	}
	return send[ContactImportAnalysis](ctx, s.client.post, "contacts/imports/"+url.PathEscape(id)+"/analyze", params, opts)
}

// StartImport validates the options and queues the draft to run in the
// background. Starting an import that already started returns it unchanged, so
// a retry is safe; one that was canceled answers 409. A started import is
// audited and, like [ContactService.ImportCommit], never raises
// contact.created.
//
// Requires the manage-contacts permission ([PermBulkContacts] for an API key).
func (s *ContactService) StartImport(ctx context.Context, id string, params *ContactImportParams, opts ...RequestOption) (*ContactImport, *Response, error) {
	if params == nil {
		params = &ContactImportParams{}
	}
	return send[ContactImport](ctx, s.client.post, "contacts/imports/"+url.PathEscape(id)+"/start", params, opts)
}

// CancelImport stops an import. Rows already written stay written. It answers
// with the import whatever state it was in, so canceling a finished one is not
// an error.
//
// Requires the manage-contacts permission ([PermBulkContacts] for an API key).
func (s *ContactService) CancelImport(ctx context.Context, id string, opts ...RequestOption) (*ContactImport, *Response, error) {
	return send[ContactImport](ctx, s.client.post, "contacts/imports/"+url.PathEscape(id)+"/cancel", nil, opts)
}

// DownloadImportFailures streams every failed row of an import into w as a CSV
// (UTF-8 with a byte-order mark) under the uploaded file's own headers, with
// "Line" and "Error" columns appended, so it can be corrected and imported
// again. The file name the server suggests is in the response's
// Content-Disposition header. The download is not retried, so a failure
// mid-stream leaves w partially written.
//
// Requires the view-contacts permission ([PermReadContacts] for an API key).
func (s *ContactService) DownloadImportFailures(ctx context.Context, id string, w io.Writer, opts ...RequestOption) (*Response, error) {
	if w == nil {
		return nil, errors.New("warmbly: a writer is required to download the failed rows")
	}
	return s.client.get(ctx, "contacts/imports/"+url.PathEscape(id)+"/failed.csv", w, opts...)
}
