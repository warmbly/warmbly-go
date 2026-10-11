package warmbly

import (
	"context"
	"io"
	"net/url"
	"time"
)

// Background contact imports: upload a file once as a draft
// ([ContactService.CreateImport]), check a mapping against the whole file
// ([ContactService.AnalyzeImport]), start it ([ContactService.StartImport])
// and follow it ([ContactService.GetImport]) while a server-side runner works
// through it in chunks. Unlike [ContactService.ImportCommit] nothing is held
// open while rows are written, so a large file never races a request timeout.

// Background import states returned in [ContactImport.Status]. Completed,
// failed and canceled are terminal: the import never changes again.
const (
	// ContactImportStatusDraft is uploaded and waiting for a mapping and a
	// start.
	ContactImportStatusDraft     = "draft"
	ContactImportStatusQueued    = "queued"
	ContactImportStatusRunning   = "running"
	ContactImportStatusCompleted = "completed"
	ContactImportStatusFailed    = "failed"
	ContactImportStatusCancelled = "cancelled" //nolint:misspell // wire value: the API sends "cancelled" here
)

// Where a draft's [ContactImportPreview.SuggestedMapping] came from, returned
// in [ContactImportPreview.MappingSource].
const (
	// ImportMappingSuggested is the server's own guess from the headers and
	// values.
	ImportMappingSuggested = "suggested"
	// ImportMappingSaved is the mapping the workspace confirmed the last time
	// it imported a file with these exact headers.
	ImportMappingSaved = "saved"
)

// ImportColumnStats is one column's fill across the whole file, not just the
// preview sample.
type ImportColumnStats struct {
	// Filled counts rows with a non-empty value in the column.
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
	// Format is the detected file format, for example "csv" or "xlsx".
	Format string `json:"format"`
	// Status is one of the ContactImportStatus* constants.
	Status    string   `json:"status"`
	HasHeader bool     `json:"has_header"`
	Columns   []string `json:"columns"`

	// Total is the number of data rows; Processed is how many have settled.
	// Every settled row lands in exactly one of Imported, Updated, Skipped or
	// Failed.
	Total     int `json:"total"`
	Processed int `json:"processed"`
	Imported  int `json:"imported"`
	Updated   int `json:"updated"`
	Skipped   int `json:"skipped"`
	Failed    int `json:"failed"`

	// Options is the mapping and configuration saved on the draft or started
	// with.
	Options *ContactImportParams  `json:"options,omitempty"`
	Quality *ContactImportQuality `json:"quality,omitempty"`
	// SegmentsPinned is nil when the import had no segment targets. Otherwise
	// it reports whether every membership write landed; a failed one leaves
	// its reason in Notes.
	SegmentsPinned *bool    `json:"segments_pinned,omitempty"`
	Notes          []string `json:"notes"`
	// Error is why a failed import stopped.
	Error string `json:"error,omitempty"`

	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`

	// Preview is what the column mapper shows. Only a draft carries it.
	Preview *ContactImportPreview `json:"preview,omitempty"`
	// Failures are the first 200 failed rows, on a finished import only. Use
	// [ContactService.ImportFailures] for every one of them.
	Failures []ImportRowError `json:"failures,omitempty"`
}

// ContactImportAnalyzeParams is the mapping [ContactService.AnalyzeImport]
// reads the draft under.
type ContactImportAnalyzeParams struct {
	Mapping   []ImportColumnMapping `json:"mapping"`
	HasHeader bool                  `json:"has_header"`
}

// ContactImportAnalysis is what an import would do, read over the whole file
// before anything is written. Every data row lands in exactly one of New,
// Existing, DuplicatesInFile, Invalid or Conflicts.
type ContactImportAnalysis struct {
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
	// plan's contact limit, too many new categories). Empty when it would run.
	Problem string `json:"problem,omitempty"`
}

// CreateImport uploads a CSV or XLSX file (at most 50 MB and 50,000 rows) and
// stores it as a draft, answering with the preview the column mapper needs.
// When the workspace imported a file with the same headers before, the
// preview suggests that confirmed mapping ([ImportMappingSaved]). Nothing is
// written to contacts until [ContactService.StartImport].
//
// A workspace may hold at most 10 imports that are drafts, queued or running
// at once; past that the upload is refused with a 409, so cancel abandoned
// drafts. The client's default 30 second
// timeout covers the upload, so raise it with [WithHTTPClient] for a large
// file on a slow link.
//
// Requires [PermWriteContacts].
func (s *ContactService) CreateImport(ctx context.Context, file *FileUpload, opts ...RequestOption) (*ContactImport, *Response, error) {
	out := new(ContactImport)
	resp, err := s.client.postMultipart(ctx, "contacts/imports", "file", file, nil, out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// ListImports returns a page of the workspace's background imports, newest
// first. [ListOptions.Limit] is 1 to 100 (default 50); anything else is a 400.
//
// Requires [PermReadContacts].
func (s *ContactService) ListImports(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[ContactImport], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[ContactImport](ctx, s.client, "contacts/imports", q, opts...)
}

// GetImport returns one background import. Poll it to follow a running
// import: Processed climbs towards Total as chunks settle. A
// finished import with failed rows carries the first of them in
// [ContactImport.Failures].
//
// Requires [PermReadContacts].
func (s *ContactService) GetImport(ctx context.Context, id string, opts ...RequestOption) (*ContactImport, *Response, error) {
	return fetch[ContactImport](ctx, s.client, contactImportPath(id), opts)
}

// SaveImportDraft stores a draft's in-progress mapping and options so the
// import can be resumed later; nothing is validated until
// [ContactService.StartImport]. It answers 409 once the import has started.
// The request states the whole draft, so a retry is harmless.
//
// Requires [PermWriteContacts].
func (s *ContactService) SaveImportDraft(ctx context.Context, id string, params *ContactImportParams, opts ...RequestOption) (*ContactImport, *Response, error) {
	if params == nil {
		params = &ContactImportParams{}
	}
	return send[ContactImport](ctx, s.client.patch, contactImportPath(id), params, opts)
}

// AnalyzeImport reports what a draft would do under the given mapping, read
// over the whole file: new and existing addresses, repeats within the file,
// invalid rows and whether the start would be refused. Nothing is written. It
// answers 409 once the import has started.
//
// Requires [PermWriteContacts].
func (s *ContactService) AnalyzeImport(ctx context.Context, id string, params *ContactImportAnalyzeParams, opts ...RequestOption) (*ContactImportAnalysis, *Response, error) {
	if params == nil {
		params = &ContactImportAnalyzeParams{}
	}
	return send[ContactImportAnalysis](ctx, s.client.post, contactImportPath(id)+"/analyze", params, opts)
}

// StartImport validates the options and queues a draft for the background
// runner, returning it in [ContactImportStatusQueued]. The same mapping rules
// as [ContactImportParams] apply. Starting an import that already started
// returns it unchanged, so a retry is safe; a canceled one answers 409 and
// has to be uploaded again. Like [ContactService.ImportCommit] it never
// raises contact.created.
//
// Requires [PermBulkContacts].
func (s *ContactService) StartImport(ctx context.Context, id string, params *ContactImportParams, opts ...RequestOption) (*ContactImport, *Response, error) {
	if params == nil {
		params = &ContactImportParams{}
	}
	return send[ContactImport](ctx, s.client.post, contactImportPath(id)+"/start", params, opts)
}

// CancelImport stops a draft, queued or running import and returns it. Rows
// already written stay written. Canceling an import that has already ended
// returns it unchanged.
//
// Requires [PermBulkContacts].
func (s *ContactService) CancelImport(ctx context.Context, id string, opts ...RequestOption) (*ContactImport, *Response, error) {
	return send[ContactImport](ctx, s.client.post, contactImportPath(id)+"/cancel", nil, opts)
}

// ImportFailures streams every failed row of an import to w as CSV, under the
// file's own headers with the line number and the reason last, so the file
// can be fixed and imported again. The body starts with a UTF-8 byte order
// mark for spreadsheet apps, and the Content-Disposition header names the
// file.
//
// Requires [PermReadContacts].
func (s *ContactService) ImportFailures(ctx context.Context, id string, w io.Writer, opts ...RequestOption) (*Response, error) {
	return s.client.get(ctx, contactImportPath(id)+"/failed.csv", w, opts...)
}

func contactImportPath(id string) string {
	return "contacts/imports/" + url.PathEscape(id)
}
