package warmbly

import (
	"context"
	"errors"
	"io"
	"net/url"
	"time"
)

// --- background contact imports ---
//
// [ContactService.ImportPreview] and [ContactService.ImportCommit] import a
// file inside one request. A background import splits the work so large files
// survive: upload the file once as a draft ([ContactService.CreateImport]),
// optionally save a mapping as you edit it ([ContactService.SaveImportDraft]),
// dry-run it ([ContactService.AnalyzeImport]), start it
// ([ContactService.StartImport]) and follow it ([ContactService.GetImport]).
// Imports are capped at 50,000 rows, and every read and write is scoped to the
// workspace.

// Import states returned in [ContactImport.Status]. The set is open; a finished
// import is one of [ContactImportStatusCompleted], [ContactImportStatusFailed]
// and [ContactImportStatusCancelled].
const (
	// ContactImportStatusDraft is uploaded and waiting for a mapping and a
	// start.
	ContactImportStatusDraft = "draft"
	// ContactImportStatusQueued is started and waiting for a worker.
	ContactImportStatusQueued    = "queued"
	ContactImportStatusRunning   = "running"
	ContactImportStatusCompleted = "completed"
	ContactImportStatusFailed    = "failed"
	ContactImportStatusCancelled = "cancelled" //nolint:misspell // wire value: the API sends "cancelled" here
)

// ContactImport is one background import of a contact file.
type ContactImport struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organization_id"`
	CreatedBy      *string `json:"created_by,omitempty"`
	Filename       string  `json:"filename"`
	// Format is the detected file kind, for example "csv" or "xlsx".
	Format string `json:"format"`
	// Status is one of the ContactImportStatus* constants.
	Status    string   `json:"status"`
	HasHeader bool     `json:"has_header"`
	Columns   []string `json:"columns"`
	// Total is the number of data rows; Processed how many have settled. Every
	// settled row lands in exactly one of Imported, Updated, Skipped or Failed.
	Total     int `json:"total"`
	Processed int `json:"processed"`
	Imported  int `json:"imported"`
	Updated   int `json:"updated"`
	Skipped   int `json:"skipped"`
	Failed    int `json:"failed"`

	// Options is the mapping and settings saved on the draft or used to start.
	Options *ContactImportParams  `json:"options,omitempty"`
	Quality *ContactImportQuality `json:"quality,omitempty"`
	// SegmentsPinned is nil when the import had no segment targets; see
	// [ContactImportResult.SegmentsPinned].
	SegmentsPinned *bool `json:"segments_pinned,omitempty"`
	// Notes are per-import remarks, for example why a segment write failed.
	Notes []string `json:"notes"`
	// Error is why a failed import stopped.
	Error string `json:"error,omitempty"`

	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`

	// Preview is what a column mapper shows, carried by a draft only so a
	// reload can resume it.
	Preview *ContactImportPreview `json:"preview,omitempty"`
	// Failures are the first 200 failed rows once the import has finished;
	// [ContactService.DownloadImportFailures] carries every one.
	Failures []ImportRowError `json:"failures,omitempty"`
}

// Finished reports whether the import will never change again.
func (i *ContactImport) Finished() bool {
	switch i.Status {
	case ContactImportStatusCompleted, ContactImportStatusFailed, ContactImportStatusCancelled:
		return true
	}
	return false
}

// ContactImportAnalyzeParams asks what a draft would do under a mapping.
type ContactImportAnalyzeParams struct {
	Mapping   []ImportColumnMapping `json:"mapping"`
	HasHeader bool                  `json:"has_header"`
}

// ContactImportAnalysis is what an import would do, read over the whole file
// before anything is written. Each row lands in exactly one bucket.
type ContactImportAnalysis struct {
	// Rows is every data row.
	Rows int `json:"rows"`
	// New addresses the workspace does not have yet.
	New int `json:"new"`
	// Existing addresses the workspace already has a contact for.
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
	// Problem is why starting this import would be refused as a whole (the
	// plan's contact limit, too many new categories); empty when it would run.
	Problem string `json:"problem,omitempty"`
}

// CreateImport uploads a CSV or XLSX file once as a draft import and returns it
// with the preview a column mapper needs (the detected columns, a suggested
// mapping and sample rows), answering 201. Nothing is written to the
// workspace's contacts yet. Requires the write_contacts scope.
func (s *ContactService) CreateImport(ctx context.Context, file *FileUpload, opts ...RequestOption) (*ContactImport, *Response, error) {
	out := new(ContactImport)
	resp, err := s.client.postMultipart(ctx, "contacts/imports", "file", file, nil, out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// ListImports returns a page of the workspace's background contact imports,
// newest first. The page size defaults to 50 and is capped at 100. Requires the
// read_contacts scope.
func (s *ContactService) ListImports(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[ContactImport], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[ContactImport](ctx, s.client, "contacts/imports", q, opts...)
}

// GetImport retrieves one import. A finished import carries its first failed
// rows. Requires the read_contacts scope.
func (s *ContactService) GetImport(ctx context.Context, id string, opts ...RequestOption) (*ContactImport, *Response, error) {
	return fetch[ContactImport](ctx, s.client, "contacts/imports/"+url.PathEscape(id), opts)
}

// SaveImportDraft autosaves a draft's mapping and options so a reload resumes
// it, and returns the import. The body is the desired state, so repeating the
// call is safe. A started import is refused with a 409. Requires the
// write_contacts scope.
func (s *ContactService) SaveImportDraft(ctx context.Context, id string, params *ContactImportParams, opts ...RequestOption) (*ContactImport, *Response, error) {
	if params == nil {
		return nil, nil, errors.New("warmbly: import options are required")
	}
	return send[ContactImport](ctx, s.client.patch, "contacts/imports/"+url.PathEscape(id), params, opts)
}

// AnalyzeImport reports what the draft would do under a mapping, read over the
// whole file, without writing anything. Requires the write_contacts scope.
func (s *ContactService) AnalyzeImport(ctx context.Context, id string, params *ContactImportAnalyzeParams, opts ...RequestOption) (*ContactImportAnalysis, *Response, error) {
	if params == nil {
		return nil, nil, errors.New("warmbly: an import analysis needs a mapping")
	}
	return send[ContactImportAnalysis](ctx, s.client.post, "contacts/imports/"+url.PathEscape(id)+"/analyze", params, opts)
}

// StartImport queues the draft to import under the given mapping and options
// and returns it. Starting an import that already started returns it
// unchanged, so repeating the call is safe. As with [ContactService.ImportCommit],
// an import is a bulk arrival: it never raises contact.created. Requires the
// bulk_contacts scope.
func (s *ContactService) StartImport(ctx context.Context, id string, params *ContactImportParams, opts ...RequestOption) (*ContactImport, *Response, error) {
	if params == nil {
		return nil, nil, errors.New("warmbly: import options are required")
	}
	return send[ContactImport](ctx, s.client.post, "contacts/imports/"+url.PathEscape(id)+"/start", params, opts)
}

// CancelImport stops an import. Rows already written stay written. Requires the
// bulk_contacts scope.
func (s *ContactService) CancelImport(ctx context.Context, id string, opts ...RequestOption) (*ContactImport, *Response, error) {
	return send[ContactImport](ctx, s.client.post, "contacts/imports/"+url.PathEscape(id)+"/cancel", nil, opts)
}

// DownloadImportFailures streams every failed row of an import as CSV to w.
// Requires the read_contacts scope.
func (s *ContactService) DownloadImportFailures(ctx context.Context, id string, w io.Writer, opts ...RequestOption) (*Response, error) {
	return s.client.get(ctx, "contacts/imports/"+url.PathEscape(id)+"/failed.csv", w, opts...)
}
