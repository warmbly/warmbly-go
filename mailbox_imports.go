package warmbly

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// --- mailbox imports ---
//
// A mailbox import connects many SMTP/IMAP mailboxes from one file or pasted
// list as a background job: preview it, create it, then follow its rows and
// retry or fix the ones that failed. Every route here is session-only and
// needs the manage-emails organization permission ([OrgPermManageEmails]),
// because an import carries mailbox passwords.

// MailboxImportField says what a column of an import file means, for
// [MailboxImportMapping]. The server reports its own guess in
// [MailboxImportColumn.Field]. The list is open: a value this SDK does not
// name is passed through unchanged.
type MailboxImportField string

// Values for [MailboxImportField].
const (
	ImportFieldIgnore          MailboxImportField = "ignore"
	ImportFieldEmail           MailboxImportField = "email"
	ImportFieldName            MailboxImportField = "name"
	ImportFieldFirstName       MailboxImportField = "first_name"
	ImportFieldLastName        MailboxImportField = "last_name"
	ImportFieldPassword        MailboxImportField = "password"
	ImportFieldAppPassword     MailboxImportField = "app_password"
	ImportFieldSMTPPassword    MailboxImportField = "smtp_password"
	ImportFieldIMAPPassword    MailboxImportField = "imap_password"
	ImportFieldUsername        MailboxImportField = "username"
	ImportFieldSMTPUsername    MailboxImportField = "smtp_username"
	ImportFieldIMAPUsername    MailboxImportField = "imap_username"
	ImportFieldSMTPHost        MailboxImportField = "smtp_host"
	ImportFieldSMTPPort        MailboxImportField = "smtp_port"
	ImportFieldSMTPSecurity    MailboxImportField = "smtp_security"
	ImportFieldIMAPHost        MailboxImportField = "imap_host"
	ImportFieldIMAPPort        MailboxImportField = "imap_port"
	ImportFieldIMAPSecurity    MailboxImportField = "imap_security"
	ImportFieldDailyLimit      MailboxImportField = "daily_limit"
	ImportFieldMinWait         MailboxImportField = "min_wait"
	ImportFieldWarmup          MailboxImportField = "warmup"
	ImportFieldWarmupStart     MailboxImportField = "warmup_start"
	ImportFieldWarmupMax       MailboxImportField = "warmup_max"
	ImportFieldWarmupIncrease  MailboxImportField = "warmup_increase"
	ImportFieldWarmupReplyRate MailboxImportField = "warmup_reply_rate"
	ImportFieldReplyTo         MailboxImportField = "reply_to"
	ImportFieldSignature       MailboxImportField = "signature"
	ImportFieldTags            MailboxImportField = "tags"
	ImportFieldTimezone        MailboxImportField = "timezone"
)

// Row states in [MailboxImportRow.Status] and the keys of
// [MailboxImportCounts]. The set is open; compare against these but tolerate
// others.
const (
	MailboxImportRowQueued      = "queued"
	MailboxImportRowRunning     = "running"
	MailboxImportRowConnected   = "connected"
	MailboxImportRowUpdated     = "updated"
	MailboxImportRowSkipped     = "skipped"
	MailboxImportRowFailed      = "failed"
	MailboxImportRowNeedsSignin = "needs_signin"
	MailboxImportRowCancelled   = "cancelled" //nolint:misspell // wire value: the API sends "cancelled" here
)

// Import states in [MailboxImport.Status].
const (
	MailboxImportRunning   = "running"
	MailboxImportCompleted = "completed"
	MailboxImportCancelled = "cancelled" //nolint:misspell // wire value: the API sends "cancelled" here
)

// Per-row states a preview reports in [MailboxImportPreviewRow.Status]: what
// would happen, before anything is tried.
const (
	MailboxImportPreviewReady       = "ready"
	MailboxImportPreviewNeedsSignin = "needs_signin"
	MailboxImportPreviewInvalid     = "invalid"
	MailboxImportPreviewExisting    = "existing"
	MailboxImportPreviewDuplicate   = "duplicate"
)

// What an import does with an address that is already connected, for
// [MailboxImportOptions.OnExisting].
const (
	// MailboxImportOnExistingUpdate applies the row's settings to the existing
	// mailbox. The default.
	MailboxImportOnExistingUpdate = "update"
	// MailboxImportOnExistingSkip leaves the existing mailbox untouched.
	MailboxImportOnExistingSkip = "skip"
)

// MailboxImportMapping maps a column index, written as a decimal string, to a
// [MailboxImportField], for example {"0": ImportFieldEmail}. Leave it nil to
// let the server detect the columns.
type MailboxImportMapping map[string]MailboxImportField

// MailboxImportSettings are applied to every mailbox an import connects or
// updates; a mapped column overrides them per row. Nil fields leave the
// mailbox as it is.
type MailboxImportSettings struct {
	TagIDs          []string `json:"tag_ids,omitempty"`
	DailyLimit      *int     `json:"daily_limit,omitempty"`
	MinWait         *int     `json:"min_wait,omitempty"`
	Warmup          *bool    `json:"warmup,omitempty"`
	WarmupStart     *int     `json:"warmup_start,omitempty"`
	WarmupMax       *int     `json:"warmup_max,omitempty"`
	WarmupIncrease  *int     `json:"warmup_increase,omitempty"`
	WarmupReplyRate *int     `json:"warmup_reply_rate,omitempty"`
	ReplyTo         *string  `json:"reply_to,omitempty"`
	Signature       *string  `json:"signature,omitempty"`
	Timezone        *string  `json:"timezone,omitempty"`
}

// MailboxImportOptions are the non-file inputs of a preview or an import. It
// is also the options block of [EmailService.ConnectGrantUsers] and
// [EmailService.ImportVendorMailboxes].
type MailboxImportOptions struct {
	// HasHeader says whether the first row is a header. Nil lets the server
	// decide.
	HasHeader *bool `json:"has_header"`
	// SharedPassword is used for every row without a password of its own.
	SharedPassword string `json:"shared_password"`
	// OnExisting is [MailboxImportOnExistingUpdate] (the default) or
	// [MailboxImportOnExistingSkip].
	OnExisting string `json:"on_existing"`
	// SaveMapping remembers the column mapping for the next file with the same
	// headers.
	SaveMapping *bool                 `json:"save_mapping"`
	Settings    MailboxImportSettings `json:"settings"`
	// TrackingDomains picks a tracking host per sending domain, for example
	// {"acme.io": "link.acme.io"}.
	TrackingDomains map[string]string `json:"tracking_domains,omitempty"`
	// Redirects sends a sending domain's root to a website, for example
	// {"acme.io": "https://acme.com"}.
	Redirects map[string]string `json:"redirects,omitempty"`
}

// MailboxImportParams is the input of [EmailService.PreviewImport] and
// [EmailService.CreateImport]: a file or a pasted list, plus how to read it.
type MailboxImportParams struct {
	// File is the upload (CSV, TSV or XLSX, up to 10 MB). Set File or Text.
	File *FileUpload
	// Text is a pasted list, used when File is nil. Up to 2 MB.
	Text string
	// Mapping overrides the detected column meanings.
	Mapping MailboxImportMapping
	// Options carries the import's settings; nil uses the defaults.
	Options *MailboxImportOptions
}

// MailboxImportVendor is the inbox vendor an uploaded file looks like it came
// from.
type MailboxImportVendor struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// MailboxImportColumn is one detected column of an uploaded file.
type MailboxImportColumn struct {
	Index   int      `json:"index"`
	Header  string   `json:"header"`
	Samples []string `json:"samples"`
	// Secret marks a column holding passwords, whose samples are masked.
	Secret bool `json:"secret"`
	// Field is the meaning the server guessed or was told.
	Field MailboxImportField `json:"field"`
	// Source says how Field was chosen, for example "saved", "user", "vendor",
	// "alias", "shape" or "none". The set is open.
	Source     string  `json:"source"`
	Confidence float64 `json:"confidence"`
}

// MailboxImportEndpoint is one server (SMTP or IMAP) a row would connect to.
type MailboxImportEndpoint struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Security string `json:"security"`
	Username string `json:"username,omitempty"`
}

// MailboxImportPreviewRow is one row of a preview.
type MailboxImportPreviewRow struct {
	Line     int    `json:"line"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	MailHost string `json:"mail_host"`
	// AuthMethod is how the row would sign in, for example "password" or
	// "app_password".
	AuthMethod string `json:"auth_method"`
	// Status is one of the MailboxImportPreview* constants.
	Status string `json:"status"`
	// Cause and Problem say why a row is not ready.
	Cause   string                 `json:"cause,omitempty"`
	Problem string                 `json:"problem,omitempty"`
	SMTP    *MailboxImportEndpoint `json:"smtp,omitempty"`
	IMAP    *MailboxImportEndpoint `json:"imap,omitempty"`
}

// MailboxImportDomainAuth is a sending domain's authentication state as the
// preview found it.
type MailboxImportDomainAuth struct {
	// State is [AuthStatePassing], [AuthStateFailing] or [AuthStateUnknown].
	State string `json:"state"`
	SPF   bool   `json:"spf"`
	DKIM  bool   `json:"dkim"`
	DMARC bool   `json:"dmarc"`
}

// MailboxImportDomain is one sending domain found in an uploaded file.
type MailboxImportDomain struct {
	Domain   string `json:"domain"`
	MailHost string `json:"mail_host"`
	Label    string `json:"label"`
	Source   string `json:"source"`
	Rows     int    `json:"rows"`
	// PasswordAuth says how the host takes passwords, for example "password",
	// "app_password" or "oauth". The set is open.
	PasswordAuth   string                   `json:"password_auth"`
	AppPasswordURL string                   `json:"app_password_url,omitempty"`
	SMTP           *MailboxImportEndpoint   `json:"smtp,omitempty"`
	IMAP           *MailboxImportEndpoint   `json:"imap,omitempty"`
	Auth           *MailboxImportDomainAuth `json:"auth,omitempty"`
	// Tracking is the tracking host this domain's mailboxes can use; nil when
	// tracking is off on this instance.
	Tracking *TrackingSuggestion `json:"tracking,omitempty"`
	// Redirect is the domain's root redirect, when one exists.
	Redirect *DomainRedirect `json:"redirect,omitempty"`
	// VendorDomain is set when a connected vendor account holds the domain.
	VendorDomain *VendorDomainLink `json:"vendor_domain,omitempty"`
}

// MailboxImportIssue is one cause shared by several rows, with its fix.
type MailboxImportIssue struct {
	Cause string `json:"cause"`
	Title string `json:"title"`
	Fix   string `json:"fix"`
	Count int    `json:"count"`
	Lines []int  `json:"lines"`
}

// MailboxImportSummary counts a preview's rows by state.
type MailboxImportSummary struct {
	Total       int `json:"total"`
	Ready       int `json:"ready"`
	NeedsSignin int `json:"needs_signin"`
	Invalid     int `json:"invalid"`
	Existing    int `json:"existing"`
	Duplicate   int `json:"duplicate"`
}

// MailboxImportPreview is what an import of a file would do, row by row and
// domain by domain, without doing any of it.
type MailboxImportPreview struct {
	// Format is the detected file kind, for example "csv", "xlsx" or "paste".
	Format    string `json:"format"`
	HasHeader bool   `json:"has_header"`
	// Vendor is set when the file looks like an inbox vendor's export.
	Vendor  *MailboxImportVendor  `json:"vendor"`
	Columns []MailboxImportColumn `json:"columns"`
	Mapping MailboxImportMapping  `json:"mapping"`
	// SavedMapping reports that Mapping came from a mapping saved earlier.
	SavedMapping bool                      `json:"saved_mapping"`
	Summary      MailboxImportSummary      `json:"summary"`
	Rows         []MailboxImportPreviewRow `json:"rows"`
	Domains      []MailboxImportDomain     `json:"domains"`
	Issues       []MailboxImportIssue      `json:"issues"`
	// Allowance is the workspace's mailbox allowance, so a preview can say how
	// many rows will fit.
	Allowance *MailboxAllowance `json:"allowance,omitempty"`
	// MaxRows is the most rows one import accepts.
	MaxRows int `json:"max_rows"`
}

// MailboxImportCounts counts an import's rows by [MailboxImportRow.Status].
type MailboxImportCounts struct {
	Queued      int `json:"queued"`
	Running     int `json:"running"`
	Connected   int `json:"connected"`
	Updated     int `json:"updated"`
	Skipped     int `json:"skipped"`
	Failed      int `json:"failed"`
	NeedsSignin int `json:"needs_signin"`
	Cancelled   int `json:"cancelled"` //nolint:misspell // wire value: the API sends "cancelled" here
}

// MailboxImportCause is one reason rows failed, with the fix and whether
// [EmailService.RetryImport] can requeue them.
type MailboxImportCause struct {
	Cause     string `json:"cause"`
	Title     string `json:"title"`
	Fix       string `json:"fix"`
	Count     int    `json:"count"`
	Retryable bool   `json:"retryable"`
}

// MailboxImport is a background mailbox import.
type MailboxImport struct {
	ID        string  `json:"id"`
	CreatedBy *string `json:"created_by,omitempty"`
	// Status is [MailboxImportRunning], [MailboxImportCompleted] or
	// [MailboxImportCancelled].
	Status string `json:"status"`
	// Source is where the rows came from: "file", "paste", "vendor" or
	// "workspace". The set is open.
	Source   string `json:"source"`
	Filename string `json:"filename"`
	// Vendor is the inbox vendor or provider the rows came from, when any.
	Vendor     string     `json:"vendor"`
	OnExisting string     `json:"on_existing"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	// CredentialsExpireAt is when the stored credentials of unfinished rows are
	// discarded.
	CredentialsExpireAt *time.Time           `json:"credentials_expire_at,omitempty"`
	Total               int                  `json:"total"`
	Counts              MailboxImportCounts  `json:"counts"`
	Causes              []MailboxImportCause `json:"causes"`
	// Authorizing counts the rows an inbox vendor is still authorizing, by
	// provider ("google", "microsoft").
	Authorizing map[string]int `json:"authorizing"`
}

// Finished reports whether the import has stopped, completed or canceled.
func (i *MailboxImport) Finished() bool { return i.FinishedAt != nil }

// MailboxImportRow is one row of an import and how it fared.
type MailboxImportRow struct {
	Line     int    `json:"line"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	MailHost string `json:"mail_host"`
	// Status is one of the MailboxImportRow* constants.
	Status string `json:"status"`
	// Code is the stable error code of a failed row.
	Code string `json:"code"`
	// Cause groups failures; see [MailboxImportCause].
	Cause   string `json:"cause"`
	Message string `json:"message"`
	// EmailAccountID is the mailbox the row connected or updated.
	EmailAccountID *string `json:"email_account_id,omitempty"`
	Retryable      bool    `json:"retryable"`
	// Fields are the row's non-secret values as uploaded.
	Fields    map[string]string `json:"fields"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// MailboxImportRowsParams filters and pages [EmailService.ImportRows].
type MailboxImportRowsParams struct {
	ListOptions
	// Status keeps rows in these states (MailboxImportRow* constants). The
	// server rejects an unknown state with a 400.
	Status []string
	// Cause keeps rows that failed for this cause.
	Cause string
}

func (p *MailboxImportRowsParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	if len(p.Status) > 0 {
		q.Set("status", strings.Join(p.Status, ","))
	}
	setNonEmpty(q, "cause", p.Cause)
	return q
}

// MailboxImportLegFix corrects one leg (SMTP or IMAP) of a failed row. Nil
// fields keep what was uploaded.
type MailboxImportLegFix struct {
	Host     *string `json:"host"`
	Port     *int    `json:"port"`
	Security *string `json:"security"`
	Username *string `json:"username"`
	Password *string `json:"password"`
}

// MailboxImportRowFix corrects one failed row. Nil fields keep what was
// uploaded.
type MailboxImportRowFix struct {
	Password    *string              `json:"password"`
	AppPassword *string              `json:"app_password"`
	Username    *string              `json:"username"`
	SMTP        *MailboxImportLegFix `json:"smtp"`
	IMAP        *MailboxImportLegFix `json:"imap"`
}

// MailboxImportRetryParams narrows [EmailService.RetryImport]. With every
// field empty all retryable failed rows are requeued.
type MailboxImportRetryParams struct {
	// Cause requeues only the rows that failed for this cause.
	Cause string `json:"cause"`
	// Lines requeues only these row numbers.
	Lines []int `json:"lines"`
	// Password is used for rows that failed for lack of one.
	Password string `json:"password"`
}

// PreviewImport reports what importing a file or pasted list would do, row by
// row and domain by domain, without connecting anything. It is session-only
// and needs [OrgPermManageEmails].
func (s *EmailService) PreviewImport(ctx context.Context, params *MailboxImportParams, opts ...RequestOption) (*MailboxImportPreview, *Response, error) {
	out := new(MailboxImportPreview)
	resp, err := s.client.postImportForm(ctx, "emails/imports/preview", params, out, opts)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// CreateImport stores the rows of a file or pasted list and connects them in
// the background, answering 201 at once with the running import. Follow it
// with [EmailService.GetImport] and [EmailService.ImportRows].
//
// Retrying the call creates a second import, whose rows find the first one's
// mailboxes already connected and update them (or skip them with
// [MailboxImportOnExistingSkip]). It is session-only and needs
// [OrgPermManageEmails]; rows past the workspace's mailbox allowance fail with
// the allowance cause.
func (s *EmailService) CreateImport(ctx context.Context, params *MailboxImportParams, opts ...RequestOption) (*MailboxImport, *Response, error) {
	out := new(MailboxImport)
	resp, err := s.client.postImportForm(ctx, "emails/imports", params, out, opts)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// ListImports returns a page of the workspace's recent mailbox imports, newest
// first. Dismissed imports are left out. It is session-only and needs
// [OrgPermManageEmails].
func (s *EmailService) ListImports(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[MailboxImport], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[MailboxImport](ctx, s.client, "emails/imports", q, opts...)
}

// GetImport retrieves one mailbox import with its row counts and failure
// causes. It is session-only and needs [OrgPermManageEmails].
func (s *EmailService) GetImport(ctx context.Context, id string, opts ...RequestOption) (*MailboxImport, *Response, error) {
	return fetch[MailboxImport](ctx, s.client, "emails/imports/"+url.PathEscape(id), opts)
}

// ImportRows returns a page of an import's rows, optionally narrowed by state
// and cause. The page size is capped at 200. It is session-only and needs
// [OrgPermManageEmails].
func (s *EmailService) ImportRows(ctx context.Context, id string, params *MailboxImportRowsParams, opts ...RequestOption) (*Page[MailboxImportRow], error) {
	return listJSON[MailboxImportRow](ctx, s.client, "emails/imports/"+url.PathEscape(id)+"/rows", params.values(), opts...)
}

// FixImportRow corrects one failed row (a password, a server, a username) and
// queues it again, returning the row. A row that already connected is refused,
// so repeating the call is safe. Line is the row number from
// [MailboxImportRow.Line]. It is session-only and needs [OrgPermManageEmails].
func (s *EmailService) FixImportRow(ctx context.Context, id string, line int, fix *MailboxImportRowFix, opts ...RequestOption) (*MailboxImportRow, *Response, error) {
	return send[MailboxImportRow](ctx, s.client.patch, "emails/imports/"+url.PathEscape(id)+"/rows/"+strconv.Itoa(line), fix, opts)
}

// RetryImport requeues failed rows of an import (all of them, one cause, or
// named lines) and returns the import. Only failed rows are requeued, so
// repeating the call cannot connect anything twice. Params may be nil. It is
// session-only and needs [OrgPermManageEmails].
func (s *EmailService) RetryImport(ctx context.Context, id string, params *MailboxImportRetryParams, opts ...RequestOption) (*MailboxImport, *Response, error) {
	return send[MailboxImport](ctx, s.client.post, "emails/imports/"+url.PathEscape(id)+"/retry", params, opts)
}

// CancelImport stops an import. Rows already connected stay connected;
// canceling twice changes nothing. It is session-only and needs
// [OrgPermManageEmails].
func (s *EmailService) CancelImport(ctx context.Context, id string, opts ...RequestOption) (*MailboxImport, *Response, error) {
	return send[MailboxImport](ctx, s.client.post, "emails/imports/"+url.PathEscape(id)+"/cancel", nil, opts)
}

// DismissImport hides an import from the recent list, stopping it first when
// it is still running. A repeat changes nothing. The server answers 204. It is
// session-only and needs [OrgPermManageEmails].
func (s *EmailService) DismissImport(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "emails/imports/"+url.PathEscape(id)+"/dismiss", nil, nil, opts...)
}

// DownloadImportFailures streams the import's failed rows as CSV to w, ready
// to fix and upload again. It is session-only and needs [OrgPermManageEmails].
func (s *EmailService) DownloadImportFailures(ctx context.Context, id string, w io.Writer, opts ...RequestOption) (*Response, error) {
	return s.client.get(ctx, "emails/imports/"+url.PathEscape(id)+"/failed.csv", w, opts...)
}

// postImportForm sends an import's multipart body: the file or pasted text
// plus the mapping and options as JSON fields.
func (c *Client) postImportForm(ctx context.Context, path string, params *MailboxImportParams, out any, opts []RequestOption) (*Response, error) {
	if params == nil {
		return nil, errors.New("warmbly: import parameters are required")
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	switch {
	case params.File != nil:
		if params.File.Content == nil || params.File.Filename == "" {
			return nil, errors.New("warmbly: an import file needs a filename and content")
		}
		hdr := make(textproto.MIMEHeader)
		hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, "file", params.File.Filename))
		if params.File.ContentType != "" {
			hdr.Set("Content-Type", params.File.ContentType)
		}
		part, err := mw.CreatePart(hdr)
		if err != nil {
			return nil, fmt.Errorf("warmbly: build import form: %w", err)
		}
		if _, err := io.Copy(part, params.File.Content); err != nil {
			return nil, fmt.Errorf("warmbly: read import file: %w", err)
		}
	case params.Text != "":
		if err := mw.WriteField("text", params.Text); err != nil {
			return nil, fmt.Errorf("warmbly: build import form: %w", err)
		}
	default:
		return nil, errors.New("warmbly: an import needs a file or pasted text")
	}
	if len(params.Mapping) > 0 {
		enc, err := json.Marshal(params.Mapping)
		if err != nil {
			return nil, fmt.Errorf("warmbly: encode import mapping: %w", err)
		}
		if err := mw.WriteField("mapping", string(enc)); err != nil {
			return nil, fmt.Errorf("warmbly: build import form: %w", err)
		}
	}
	if params.Options != nil {
		enc, err := json.Marshal(params.Options)
		if err != nil {
			return nil, fmt.Errorf("warmbly: encode import options: %w", err)
		}
		if err := mw.WriteField("options", string(enc)); err != nil {
			return nil, fmt.Errorf("warmbly: build import form: %w", err)
		}
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("warmbly: build import form: %w", err)
	}

	req, err := c.newRequest(ctx, http.MethodPost, path, nil, newRequestConfig(opts))
	if err != nil {
		return nil, err
	}
	payload := buf.Bytes()
	req.Body = io.NopCloser(bytes.NewReader(payload))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(payload)), nil
	}
	req.ContentLength = int64(len(payload))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return c.do(req, out)
}
