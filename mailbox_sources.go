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
	"net/url"
	"strconv"
	"strings"
	"time"
)

// This file covers the ways mailboxes arrive in bulk and the domain-level
// setup around them, all on [EmailService]:
//
//   - CSV and pasted-list imports ([EmailService.PreviewImport] and friends);
//   - administrator grants over a whole Google Workspace domain or Microsoft
//     365 tenant ([EmailService.Grants] and friends);
//   - inbox vendor accounts to import from ([EmailService.Vendors] and
//     friends);
//   - sending domains, their tracking hosts and root redirects
//     ([EmailService.Domains] and friends).
//
// Every route here is session-only: imports carry mailbox passwords and grants
// reach a whole domain's mail, so an API key cannot call them. They require an
// organization and the manage-emails permission, and the grant routes that
// record, remove or use a grant additionally need a recent sign-in.

// Row statuses carried in [MailboxImportRow.Status] and used to filter
// [MailboxImportRowsParams.Status].
const (
	ImportRowQueued      = "queued"
	ImportRowRunning     = "running"
	ImportRowConnected   = "connected"
	ImportRowUpdated     = "updated"
	ImportRowSkipped     = "skipped"
	ImportRowFailed      = "failed"
	ImportRowNeedsSignin = "needs_signin"
	ImportRowCancelled   = "cancelled" //nolint:misspell // wire value: the API sends "cancelled" here
)

// Import statuses carried in [MailboxImport.Status].
const (
	ImportRunning   = "running"
	ImportCompleted = "completed"
	ImportCancelled = "cancelled" //nolint:misspell // wire value: the API sends "cancelled" here
)

// Preview row states carried in [MailboxImportPreviewRow.Status]: what would
// happen, before anything is tried.
const (
	ImportPreviewReady       = "ready"
	ImportPreviewNeedsSignin = "needs_signin"
	ImportPreviewInvalid     = "invalid"
	ImportPreviewExisting    = "existing"
	ImportPreviewDuplicate   = "duplicate"
)

// What an import does with a mailbox that is already connected, for
// [MailboxImportOptions.OnExisting].
const (
	// ImportOnExistingUpdate applies the import's settings to the existing
	// mailbox. The default.
	ImportOnExistingUpdate = "update"
	// ImportOnExistingSkip leaves the existing mailbox alone.
	ImportOnExistingSkip = "skip"
)

// Fields a column of an import file can be mapped to, for
// [MailboxImportMapping]. The server may add more; treat the set as open.
const (
	ImportFieldIgnore          = "ignore"
	ImportFieldEmail           = "email"
	ImportFieldName            = "name"
	ImportFieldFirstName       = "first_name"
	ImportFieldLastName        = "last_name"
	ImportFieldPassword        = "password"
	ImportFieldAppPassword     = "app_password"
	ImportFieldSMTPPassword    = "smtp_password"
	ImportFieldIMAPPassword    = "imap_password"
	ImportFieldUsername        = "username"
	ImportFieldSMTPUsername    = "smtp_username"
	ImportFieldIMAPUsername    = "imap_username"
	ImportFieldSMTPHost        = "smtp_host"
	ImportFieldSMTPPort        = "smtp_port"
	ImportFieldSMTPSecurity    = "smtp_security"
	ImportFieldIMAPHost        = "imap_host"
	ImportFieldIMAPPort        = "imap_port"
	ImportFieldIMAPSecurity    = "imap_security"
	ImportFieldDailyLimit      = "daily_limit"
	ImportFieldMinWait         = "min_wait"
	ImportFieldWarmup          = "warmup"
	ImportFieldWarmupStart     = "warmup_start"
	ImportFieldWarmupMax       = "warmup_max"
	ImportFieldWarmupIncrease  = "warmup_increase"
	ImportFieldWarmupReplyRate = "warmup_reply_rate"
	ImportFieldReplyTo         = "reply_to"
	ImportFieldSignature       = "signature"
	ImportFieldTags            = "tags"
	ImportFieldTimezone        = "timezone"
)

// MailboxImportMapping maps a column index (as a string, since it travels as
// a JSON object key) to one of the ImportField* values.
type MailboxImportMapping map[string]string

// MailboxImportSettings are applied to every mailbox an import connects or
// updates; a mapped column overrides them per row. A nil field leaves that
// setting as it is.
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

// MailboxImportOptions are the non-file inputs of an import or its preview.
// Nil fields take the server default.
type MailboxImportOptions struct {
	// HasHeader says whether the first row is a header; nil detects it.
	HasHeader *bool `json:"has_header,omitempty"`
	// SharedPassword is used for every row that has no password of its own.
	SharedPassword string `json:"shared_password,omitempty"`
	// OnExisting is [ImportOnExistingUpdate] (the default) or
	// [ImportOnExistingSkip].
	OnExisting string `json:"on_existing,omitempty"`
	// SaveMapping remembers the column mapping for the next import.
	SaveMapping *bool                 `json:"save_mapping,omitempty"`
	Settings    MailboxImportSettings `json:"settings"`
	// TrackingDomains picks a tracking host per sending domain, for example
	// {"acme.io": "link.acme.io"}.
	TrackingDomains map[string]string `json:"tracking_domains,omitempty"`
	// Redirects sends a sending domain's root to a website, for example
	// {"acme.io": "https://acme.com"}.
	Redirects map[string]string `json:"redirects,omitempty"`
}

// MailboxImportInput is the file or pasted list to import, with its mapping
// and options. It is sent as multipart form data. Set File for an upload
// (up to 10 MB) or Text for a pasted list (up to 2 MB); File wins when both
// are set.
type MailboxImportInput struct {
	File *FileUpload
	Text string
	// Mapping overrides the detected column mapping.
	Mapping MailboxImportMapping
	Options *MailboxImportOptions
}

// MailboxImportVendor is the inbox vendor an import file was recognized as
// coming from.
type MailboxImportVendor struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// MailboxImportColumn is one detected column of an import file.
type MailboxImportColumn struct {
	Index   int      `json:"index"`
	Header  string   `json:"header"`
	Samples []string `json:"samples"`
	// Secret marks a column that holds credentials; its samples are masked.
	Secret bool `json:"secret"`
	// Field is the ImportField* value the column was mapped to, and Source
	// how that was decided.
	Field      string  `json:"field"`
	Source     string  `json:"source"`
	Confidence float64 `json:"confidence"`
}

// MailboxImportEndpoint is the SMTP or IMAP server a row will connect to.
type MailboxImportEndpoint struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Security string `json:"security"`
	Username string `json:"username,omitempty"`
}

// MailboxImportPreviewRow is what one row of the file would do.
type MailboxImportPreviewRow struct {
	Line       int    `json:"line"`
	Email      string `json:"email"`
	Name       string `json:"name"`
	MailHost   string `json:"mail_host"`
	AuthMethod string `json:"auth_method"`
	// Status is one of the ImportPreview* values.
	Status  string                 `json:"status"`
	Cause   string                 `json:"cause,omitempty"`
	Problem string                 `json:"problem,omitempty"`
	SMTP    *MailboxImportEndpoint `json:"smtp,omitempty"`
	IMAP    *MailboxImportEndpoint `json:"imap,omitempty"`
}

// MailboxImportDomainAuth is the SPF, DKIM and DMARC state found for an
// imported domain.
type MailboxImportDomainAuth struct {
	State string `json:"state"`
	SPF   bool   `json:"spf"`
	DKIM  bool   `json:"dkim"`
	DMARC bool   `json:"dmarc"`
}

// MailboxImportDomain is one sending domain found in an import file.
type MailboxImportDomain struct {
	Domain       string `json:"domain"`
	MailHost     string `json:"mail_host"`
	Label        string `json:"label"`
	Source       string `json:"source"`
	Rows         int    `json:"rows"`
	PasswordAuth string `json:"password_auth"`
	// AppPasswordURL is where the owner creates an app password, when that is
	// how the host signs in.
	AppPasswordURL string                   `json:"app_password_url,omitempty"`
	SMTP           *MailboxImportEndpoint   `json:"smtp,omitempty"`
	IMAP           *MailboxImportEndpoint   `json:"imap,omitempty"`
	Auth           *MailboxImportDomainAuth `json:"auth,omitempty"`
	// Tracking is the tracking host this domain's mailboxes can use; nil when
	// tracking is off on the instance.
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

// MailboxImportPreviewSummary counts a preview's rows by state.
type MailboxImportPreviewSummary struct {
	Total       int `json:"total"`
	Ready       int `json:"ready"`
	NeedsSignin int `json:"needs_signin"`
	Invalid     int `json:"invalid"`
	Existing    int `json:"existing"`
	Duplicate   int `json:"duplicate"`
}

// MailboxImportPreview is what an import would do, row by row and domain by
// domain, without doing any of it.
type MailboxImportPreview struct {
	Format    string `json:"format"`
	HasHeader bool   `json:"has_header"`
	// Vendor is nil unless the file was recognized as a vendor export.
	Vendor       *MailboxImportVendor        `json:"vendor"`
	Columns      []MailboxImportColumn       `json:"columns"`
	Mapping      MailboxImportMapping        `json:"mapping"`
	SavedMapping bool                        `json:"saved_mapping"`
	Summary      MailboxImportPreviewSummary `json:"summary"`
	Rows         []MailboxImportPreviewRow   `json:"rows"`
	Domains      []MailboxImportDomain       `json:"domains"`
	Issues       []MailboxImportIssue        `json:"issues"`
	Allowance    *MailboxAllowance           `json:"allowance,omitempty"`
	// MaxRows is the most rows one import accepts.
	MaxRows int `json:"max_rows"`
}

// MailboxImportCounts counts an import's rows by status.
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

// MailboxImportCause groups failed rows by cause, with the fix and whether a
// retry can help.
type MailboxImportCause struct {
	Cause     string `json:"cause"`
	Title     string `json:"title"`
	Fix       string `json:"fix"`
	Count     int    `json:"count"`
	Retryable bool   `json:"retryable"`
}

// MailboxImport is a background import of mailboxes.
type MailboxImport struct {
	ID        string  `json:"id"`
	CreatedBy *string `json:"created_by,omitempty"`
	// Status is [ImportRunning], [ImportCompleted] or [ImportCancelled].
	Status   string `json:"status"`
	Source   string `json:"source"`
	Filename string `json:"filename"`
	Vendor   string `json:"vendor"`
	// OnExisting is [ImportOnExistingUpdate] or [ImportOnExistingSkip].
	OnExisting string     `json:"on_existing"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	// CredentialsExpireAt is when the uploaded credentials are discarded.
	CredentialsExpireAt *time.Time           `json:"credentials_expire_at,omitempty"`
	Total               int                  `json:"total"`
	Counts              MailboxImportCounts  `json:"counts"`
	Causes              []MailboxImportCause `json:"causes"`
	// Authorizing counts the rows an inbox vendor is still authorizing, by
	// provider ("google", "microsoft").
	Authorizing map[string]int `json:"authorizing"`
}

// MailboxImportRow is one row of an import and how it went.
type MailboxImportRow struct {
	Line     int    `json:"line"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	MailHost string `json:"mail_host"`
	// Status is one of the ImportRow* values.
	Status  string `json:"status"`
	Code    string `json:"code"`
	Cause   string `json:"cause"`
	Message string `json:"message"`
	// EmailAccountID is the mailbox the row connected or updated.
	EmailAccountID *string           `json:"email_account_id,omitempty"`
	Retryable      bool              `json:"retryable"`
	Fields         map[string]string `json:"fields"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

// MailboxImportLegFix corrects one leg (SMTP or IMAP) of a failed row. A nil
// field keeps what was uploaded.
type MailboxImportLegFix struct {
	Host     *string `json:"host"`
	Port     *int    `json:"port"`
	Security *string `json:"security"`
	Username *string `json:"username"`
	Password *string `json:"password"`
}

// MailboxImportRowFix corrects one failed row. A nil field keeps what was
// uploaded.
type MailboxImportRowFix struct {
	Password    *string              `json:"password"`
	AppPassword *string              `json:"app_password"`
	Username    *string              `json:"username"`
	SMTP        *MailboxImportLegFix `json:"smtp"`
	IMAP        *MailboxImportLegFix `json:"imap"`
}

// MailboxImportRetry chooses which failed rows to requeue: all of them when
// empty, or one cause, or the named lines.
type MailboxImportRetry struct {
	Cause string `json:"cause,omitempty"`
	Lines []int  `json:"lines,omitempty"`
	// Password is a new shared password to use for the retried rows.
	Password string `json:"password,omitempty"`
}

// MailboxImportRowsParams filters and paginates an import's rows. The page
// size defaults to the server default and is capped at 200.
type MailboxImportRowsParams struct {
	ListOptions
	// Status keeps only rows in these ImportRow* statuses.
	Status []string
	// Cause keeps only rows with this cause.
	Cause string
}

func (p *MailboxImportRowsParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	setNonEmpty(q, "status", strings.Join(p.Status, ","))
	setNonEmpty(q, "cause", p.Cause)
	return q
}

// importForm builds the multipart body shared by preview and create.
func importForm(in *MailboxImportInput) (body []byte, contentType string, err error) {
	if in == nil {
		return nil, "", errors.New("warmbly: an import input is required")
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	switch {
	case in.File != nil:
		if in.File.Content == nil || in.File.Filename == "" {
			return nil, "", errors.New("warmbly: an import file needs a filename and content")
		}
		part, err := mw.CreateFormFile("file", in.File.Filename)
		if err != nil {
			return nil, "", fmt.Errorf("warmbly: build import form: %w", err)
		}
		if _, err := io.Copy(part, in.File.Content); err != nil {
			return nil, "", fmt.Errorf("warmbly: read import file: %w", err)
		}
	case in.Text != "":
		if err := mw.WriteField("text", in.Text); err != nil {
			return nil, "", fmt.Errorf("warmbly: build import form: %w", err)
		}
	default:
		return nil, "", errors.New("warmbly: an import needs a file or pasted text")
	}
	if len(in.Mapping) > 0 {
		raw, err := json.Marshal(in.Mapping)
		if err != nil {
			return nil, "", fmt.Errorf("warmbly: encode import mapping: %w", err)
		}
		if err := mw.WriteField("mapping", string(raw)); err != nil {
			return nil, "", fmt.Errorf("warmbly: build import form: %w", err)
		}
	}
	if in.Options != nil {
		raw, err := json.Marshal(in.Options)
		if err != nil {
			return nil, "", fmt.Errorf("warmbly: encode import options: %w", err)
		}
		if err := mw.WriteField("options", string(raw)); err != nil {
			return nil, "", fmt.Errorf("warmbly: build import form: %w", err)
		}
	}
	if err := mw.Close(); err != nil {
		return nil, "", fmt.Errorf("warmbly: build import form: %w", err)
	}
	return buf.Bytes(), mw.FormDataContentType(), nil
}

func (s *EmailService) postImportForm(ctx context.Context, path string, in *MailboxImportInput, out any, opts []RequestOption) (*Response, error) {
	payload, contentType, err := importForm(in)
	if err != nil {
		return nil, err
	}
	req, err := s.client.newRequest(ctx, http.MethodPost, path, nil, newRequestConfig(opts))
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(payload))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(payload)), nil
	}
	req.ContentLength = int64(len(payload))
	req.Header.Set("Content-Type", contentType)
	return s.client.do(req, out)
}

// PreviewImport reports what importing the file or pasted list would do, row
// by row and domain by domain, without connecting anything. Feed its Mapping
// back into [MailboxImportInput.Mapping] after correcting it.
func (s *EmailService) PreviewImport(ctx context.Context, in *MailboxImportInput, opts ...RequestOption) (*MailboxImportPreview, *Response, error) {
	out := new(MailboxImportPreview)
	resp, err := s.postImportForm(ctx, "emails/imports/preview", in, out, opts)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// CreateImport stores the rows and connects them in the background; poll
// [EmailService.Import] for progress. Retrying the call creates a second
// import whose rows find the first one's mailboxes already connected and
// update them (or skip them with [ImportOnExistingSkip]).
func (s *EmailService) CreateImport(ctx context.Context, in *MailboxImportInput, opts ...RequestOption) (*MailboxImport, *Response, error) {
	out := new(MailboxImport)
	resp, err := s.postImportForm(ctx, "emails/imports", in, out, opts)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// Imports lists recent imports, newest first. The page size is capped at 100.
func (s *EmailService) Imports(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[MailboxImport], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[MailboxImport](ctx, s.client, "emails/imports", q, opts...)
}

// Import retrieves one import with its row counts and failure causes.
func (s *EmailService) Import(ctx context.Context, id string, opts ...RequestOption) (*MailboxImport, *Response, error) {
	return fetch[MailboxImport](ctx, s.client, "emails/imports/"+url.PathEscape(id), opts)
}

// ImportRows lists an import's rows, optionally filtered by status and cause.
func (s *EmailService) ImportRows(ctx context.Context, id string, params *MailboxImportRowsParams, opts ...RequestOption) (*Page[MailboxImportRow], error) {
	return listJSON[MailboxImportRow](ctx, s.client, "emails/imports/"+url.PathEscape(id)+"/rows", params.values(), opts...)
}

// FixImportRow corrects one failed row and queues it again. line is the row's
// line number (1 or more). Retrying is safe: a row that already connected is
// refused.
func (s *EmailService) FixImportRow(ctx context.Context, id string, line int, fix *MailboxImportRowFix, opts ...RequestOption) (*MailboxImportRow, *Response, error) {
	return send[MailboxImportRow](ctx, s.client.patch, "emails/imports/"+url.PathEscape(id)+"/rows/"+strconv.Itoa(line), fix, opts)
}

// RetryImport requeues the import's failed rows (all of them, one cause, or
// the named lines). Only failed rows are requeued, so repeating the call
// cannot connect anything twice. params may be nil.
func (s *EmailService) RetryImport(ctx context.Context, id string, params *MailboxImportRetry, opts ...RequestOption) (*MailboxImport, *Response, error) {
	return send[MailboxImport](ctx, s.client.post, "emails/imports/"+url.PathEscape(id)+"/retry", params, opts)
}

// CancelImport stops an import; rows not yet tried become canceled.
// Canceling twice is a no-op.
func (s *EmailService) CancelImport(ctx context.Context, id string, opts ...RequestOption) (*MailboxImport, *Response, error) {
	return send[MailboxImport](ctx, s.client.post, "emails/imports/"+url.PathEscape(id)+"/cancel", nil, opts)
}

// DismissImport hides the import from the recent list, stopping it first when
// it is still running. A repeat changes nothing.
func (s *EmailService) DismissImport(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "emails/imports/"+url.PathEscape(id)+"/dismiss", nil, nil, opts...)
}

// ImportFailuresCSV downloads the import's failed rows as a CSV file that can
// be corrected and imported again.
func (s *EmailService) ImportFailuresCSV(ctx context.Context, id string, opts ...RequestOption) ([]byte, *Response, error) {
	var buf bytes.Buffer
	resp, err := s.client.get(ctx, "emails/imports/"+url.PathEscape(id)+"/failed.csv", &buf, opts...)
	if err != nil {
		return nil, resp, err
	}
	return buf.Bytes(), resp, nil
}

// --- administrator grants ---

// Grant providers carried in [DomainGrant.Provider].
const (
	GrantProviderGoogle    = "google"
	GrantProviderMicrosoft = "microsoft"
)

// MailboxGrantConfig says which administrator connections the instance can
// take and what a Google administrator has to authorize.
type MailboxGrantConfig struct {
	GoogleEnabled  bool     `json:"google_enabled"`
	GoogleClientID string   `json:"google_client_id,omitempty"`
	GoogleScopes   []string `json:"google_scopes"`
	// MicrosoftEnabled reports that Microsoft 365 tenant grants are available.
	MicrosoftEnabled bool `json:"microsoft_enabled"`
	// GoogleMissing and MicrosoftMissing name the settings an operator still
	// has to set; empty when the provider is enabled.
	GoogleMissing    []string `json:"google_missing"`
	MicrosoftMissing []string `json:"microsoft_missing"`
}

// DomainGrant is an administrator's grant over a Google Workspace domain or a
// Microsoft 365 tenant.
type DomainGrant struct {
	ID string `json:"id"`
	// Provider is [GrantProviderGoogle] or [GrantProviderMicrosoft].
	Provider   string     `json:"provider"`
	Tenant     string     `json:"tenant"`
	AdminEmail string     `json:"admin_email,omitempty"`
	Domains    []string   `json:"domains"`
	Status     string     `json:"status"`
	LastError  string     `json:"last_error,omitempty"`
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	// Mailboxes counts the mailboxes connected through the grant.
	Mailboxes int `json:"mailboxes"`
}

// GoogleGrantStart says how the workspace proves it controls a domain.
type GoogleGrantStart struct {
	// Method is "signin" (the administrator signs in with Google; URL and
	// State are set) or "dns".
	Method string `json:"method"`
	URL    string `json:"url,omitempty"`
	State  string `json:"state,omitempty"`
	// TXTName and TXTValue are the DNS proof, which always works.
	TXTName  string `json:"txt_name"`
	TXTValue string `json:"txt_value"`
}

// GoogleGrantFinish completes a Google Workspace grant, either with the
// sign-in State and Code, or (DNS proof) with Domain and AdminEmail alone.
type GoogleGrantFinish struct {
	State      string `json:"state,omitempty"`
	Code       string `json:"code,omitempty"`
	Domain     string `json:"domain,omitempty"`
	AdminEmail string `json:"admin_email,omitempty"`
}

// MicrosoftGrantStart is the admin-consent link for a Microsoft 365 tenant.
type MicrosoftGrantStart struct {
	URL   string `json:"url"`
	State string `json:"state"`
}

// DirectoryUser is one account in a granted domain or tenant.
type DirectoryUser struct {
	// ID is the subject tokens are minted for: the address (Google) or the
	// Graph user id (Microsoft). Pass it in [GrantConnectParams.UserIDs].
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	Connected bool   `json:"connected"`
	// EmailAccountID is the connected mailbox, when there is one.
	EmailAccountID *string `json:"email_account_id,omitempty"`
	// Upgrade marks a connected mailbox that signs in on its own; connecting
	// it moves it onto the grant and keeps its history.
	Upgrade bool `json:"upgrade"`
}

// GrantConnectParams picks the directory users to import through a grant.
type GrantConnectParams struct {
	// UserIDs are [DirectoryUser.ID] values.
	UserIDs []string `json:"user_ids,omitempty"`
	// All picks every enabled user not yet connected (and every upgradable
	// one).
	All     bool                 `json:"all,omitempty"`
	Options MailboxImportOptions `json:"options"`
}

// SigninMigrationMailbox is one mailbox still on per-mailbox Google sign-in.
type SigninMigrationMailbox struct {
	ID     string `json:"id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// SigninMigration groups a workspace's mailboxes on the retiring per-mailbox
// Google sign-in by domain.
type SigninMigration struct {
	Domain string `json:"domain"`
	// Kind is "workspace" (moves to an administrator's grant) or "personal"
	// (a shared address such as gmail.com, moves to an app password).
	Kind string `json:"kind"`
	// GrantID is the workspace's active grant covering the domain, if any.
	GrantID   *string                  `json:"grant_id,omitempty"`
	Mailboxes []SigninMigrationMailbox `json:"mailboxes"`
}

// SigninMigrationResult is the answer to [EmailService.SigninMigration].
type SigninMigrationResult struct {
	Data []SigninMigration `json:"data"`
	// Total counts the mailboxes across all groups.
	Total int `json:"total"`
}

// GrantConfig reports which administrator connections this instance supports.
func (s *EmailService) GrantConfig(ctx context.Context, opts ...RequestOption) (*MailboxGrantConfig, *Response, error) {
	return fetch[MailboxGrantConfig](ctx, s.client, "emails/grants/config", opts)
}

// Grants lists the workspace's administrator grants.
func (s *EmailService) Grants(ctx context.Context, opts ...RequestOption) ([]DomainGrant, *Response, error) {
	return fetchData[DomainGrant](ctx, s.client, "emails/grants", opts)
}

// Grant retrieves one administrator grant.
func (s *EmailService) Grant(ctx context.Context, id string, opts ...RequestOption) (*DomainGrant, *Response, error) {
	return fetch[DomainGrant](ctx, s.client, "emails/grants/"+url.PathEscape(id), opts)
}

// StartGoogleGrant begins a Google Workspace grant for domain, administered
// by adminEmail, and says how to prove control of the domain.
func (s *EmailService) StartGoogleGrant(ctx context.Context, domain, adminEmail string, opts ...RequestOption) (*GoogleGrantStart, *Response, error) {
	body := struct {
		Domain     string `json:"domain"`
		AdminEmail string `json:"admin_email"`
	}{Domain: domain, AdminEmail: adminEmail}
	return send[GoogleGrantStart](ctx, s.client.post, "emails/grants/google/start", body, opts)
}

// FinishGoogleGrant records the grant once the domain is proven. A sign-in
// State is single-use; repeating a DNS finish re-verifies the same grant.
// Needs a recent sign-in.
func (s *EmailService) FinishGoogleGrant(ctx context.Context, params *GoogleGrantFinish, opts ...RequestOption) (*DomainGrant, *Response, error) {
	return send[DomainGrant](ctx, s.client.post, "emails/grants/google/finish", params, opts)
}

// StartMicrosoftGrant returns the admin-consent URL for a Microsoft 365
// tenant.
func (s *EmailService) StartMicrosoftGrant(ctx context.Context, opts ...RequestOption) (*MicrosoftGrantStart, *Response, error) {
	return send[MicrosoftGrantStart](ctx, s.client.post, "emails/grants/microsoft/start", nil, opts)
}

// FinishMicrosoftGrant records the tenant grant. The state is single-use, so
// a repeat is refused rather than recorded twice. Needs a recent sign-in.
func (s *EmailService) FinishMicrosoftGrant(ctx context.Context, state, code string, opts ...RequestOption) (*DomainGrant, *Response, error) {
	body := struct {
		State string `json:"state"`
		Code  string `json:"code"`
	}{State: state, Code: code}
	return send[DomainGrant](ctx, s.client.post, "emails/grants/microsoft/finish", body, opts)
}

// CheckGrant verifies the grant again and puts its mailboxes back to work if
// it recovered.
func (s *EmailService) CheckGrant(ctx context.Context, id string, opts ...RequestOption) (*DomainGrant, *Response, error) {
	return send[DomainGrant](ctx, s.client.post, "emails/grants/"+url.PathEscape(id)+"/check", nil, opts)
}

// DeleteGrant removes the grant; the mailboxes connected through it stop.
// Needs a recent sign-in.
func (s *EmailService) DeleteGrant(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "emails/grants/"+url.PathEscape(id), opts...)
}

// GrantUsers lists the accounts in the granted domain or tenant.
func (s *EmailService) GrantUsers(ctx context.Context, id string, opts ...RequestOption) ([]DirectoryUser, *Response, error) {
	return fetchData[DirectoryUser](ctx, s.client, "emails/grants/"+url.PathEscape(id)+"/users", opts)
}

// ConnectGrantUsers imports the picked directory users (or every enabled one
// not yet here) as a background import; track it with [EmailService.Import].
// Needs a recent sign-in.
func (s *EmailService) ConnectGrantUsers(ctx context.Context, id string, params *GrantConnectParams, opts ...RequestOption) (*MailboxImport, *Response, error) {
	return send[MailboxImport](ctx, s.client.post, "emails/grants/"+url.PathEscape(id)+"/connect", params, opts)
}

// SigninMigration lists the workspace's mailboxes still on per-mailbox Google
// sign-in, grouped by domain, with where each one moves.
func (s *EmailService) SigninMigration(ctx context.Context, opts ...RequestOption) (*SigninMigrationResult, *Response, error) {
	out, resp, err := fetch[SigninMigrationResult](ctx, s.client, "emails/grants/migration", opts)
	if err != nil {
		return nil, resp, err
	}
	if out.Data == nil {
		out.Data = []SigninMigration{}
	}
	return out, resp, nil
}

// --- inbox vendors ---

// VendorField is one value connecting an inbox vendor asks for.
type VendorField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Secret   bool   `json:"secret"`
	Required bool   `json:"required"`
	Help     string `json:"help,omitempty"`
}

// MailboxVendor is one supported inbox vendor and what connecting it asks for.
type MailboxVendor struct {
	ID         string        `json:"id"`
	Label      string        `json:"label"`
	Website    string        `json:"website"`
	KeyHelpURL string        `json:"key_help_url"`
	Fields     []VendorField `json:"fields"`
}

// VendorConnection is an inbox vendor account the workspace imports from. Its
// credentials never leave the server.
type VendorConnection struct {
	ID     string `json:"id"`
	Vendor string `json:"vendor"`
	Label  string `json:"label"`
	Status string `json:"status"`
	// LastError is the vendor's last refusal, when there was one.
	LastError  string     `json:"last_error,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	// Mailboxes counts the mailboxes imported from the account.
	Mailboxes int `json:"mailboxes"`
}

// VendorConnectionParams connects (or, on update, renames or re-keys) an
// inbox vendor account. The key is verified with the vendor before it is
// stored sealed.
type VendorConnectionParams struct {
	// Vendor is a [MailboxVendor.ID].
	Vendor string `json:"vendor"`
	Label  string `json:"label,omitempty"`
	// Fields holds the values the vendor's [MailboxVendor.Fields] ask for,
	// keyed by [VendorField.Key].
	Fields map[string]string `json:"fields,omitempty"`
}

// VendorMailbox is one mailbox in a vendor account.
type VendorMailbox struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Domain    string `json:"domain"`
	Provider  string `json:"provider"`
	Status    string `json:"status"`
	Workspace string `json:"workspace,omitempty"`
	Connected bool   `json:"connected"`
	// EmailAccountID is the connected mailbox, when there is one.
	EmailAccountID *string `json:"email_account_id,omitempty"`
}

// VendorImportParams picks the vendor mailboxes to import.
type VendorImportParams struct {
	// MailboxIDs are [VendorMailbox.ID] values.
	MailboxIDs []string `json:"mailbox_ids,omitempty"`
	// All picks every mailbox in the account.
	All     bool                 `json:"all,omitempty"`
	Options MailboxImportOptions `json:"options"`
}

// VendorCatalog lists the supported inbox vendors and what each asks for.
func (s *EmailService) VendorCatalog(ctx context.Context, opts ...RequestOption) ([]MailboxVendor, *Response, error) {
	return fetchData[MailboxVendor](ctx, s.client, "emails/vendors/catalog", opts)
}

// Vendors lists the workspace's vendor accounts (never their keys).
func (s *EmailService) Vendors(ctx context.Context, opts ...RequestOption) ([]VendorConnection, *Response, error) {
	return fetchData[VendorConnection](ctx, s.client, "emails/vendors", opts)
}

// CreateVendor verifies a key with the vendor and stores it sealed.
func (s *EmailService) CreateVendor(ctx context.Context, params *VendorConnectionParams, opts ...RequestOption) (*VendorConnection, *Response, error) {
	return send[VendorConnection](ctx, s.client.post, "emails/vendors", params, opts)
}

// UpdateVendor renames a vendor account, or replaces its key after verifying
// it.
func (s *EmailService) UpdateVendor(ctx context.Context, id string, params *VendorConnectionParams, opts ...RequestOption) (*VendorConnection, *Response, error) {
	return send[VendorConnection](ctx, s.client.patch, "emails/vendors/"+url.PathEscape(id), params, opts)
}

// DeleteVendor removes a vendor account. The mailboxes it brought in stay
// connected.
func (s *EmailService) DeleteVendor(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "emails/vendors/"+url.PathEscape(id), opts...)
}

// VendorMailboxes lists the mailboxes held in a vendor account.
func (s *EmailService) VendorMailboxes(ctx context.Context, id string, opts ...RequestOption) ([]VendorMailbox, *Response, error) {
	return fetchData[VendorMailbox](ctx, s.client, "emails/vendors/"+url.PathEscape(id)+"/mailboxes", opts)
}

// ImportVendorMailboxes starts a background import of the picked vendor
// mailboxes; their credentials are read from the vendor as each row is worked.
// Repeating it updates what the first call connected. Track it with
// [EmailService.Import].
func (s *EmailService) ImportVendorMailboxes(ctx context.Context, id string, params *VendorImportParams, opts ...RequestOption) (*MailboxImport, *Response, error) {
	return send[MailboxImport](ctx, s.client.post, "emails/vendors/"+url.PathEscape(id)+"/import", params, opts)
}

// --- sending domains ---

// Where a root redirect is served from, carried in [DomainRedirect.ServedBy]
// and accepted in [DomainRedirectParams.ServedBy].
const (
	RedirectServedByInstance = "instance"
	RedirectServedByCloud    = "cloud"
)

// Outcomes of the HTTP check of a redirect, carried in
// [RedirectReach.Status]. Treat the set as open.
const (
	RedirectReachOK          = "ok"
	RedirectReachNotReaching = "not_reaching"
	RedirectReachHTTPSError  = "https_error"
	RedirectReachUnreachable = "unreachable"
)

// DomainDNSRecord is one DNS record the customer adds at their DNS provider,
// and whether it is in place.
type DomainDNSRecord struct {
	Purpose string `json:"purpose"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Value   string `json:"value"`
	OK      bool   `json:"ok"`
	// Optional records improve the result but are not needed to verify.
	Optional bool `json:"optional,omitempty"`
}

// RedirectReach is one HTTP check of a verified redirect's domain.
type RedirectReach struct {
	// Status is one of the RedirectReach* values.
	Status string `json:"status"`
	// Hint names the likely cause: "not_routed", "host_header",
	// "wrong_target", "certificate", "no_listener" or "settling".
	Hint string `json:"hint,omitempty"`
	// Detail is one sentence describing what the check saw.
	Detail string `json:"detail,omitempty"`
	// Proxy is the web server that answered in Warmbly's place, when it could
	// be told.
	Proxy     string     `json:"proxy,omitempty"`
	CheckedAt *time.Time `json:"checked_at,omitempty"`
}

// DomainRedirect sends a sending domain's root to the workspace's main
// website.
type DomainRedirect struct {
	ID            string            `json:"id"`
	Domain        string            `json:"domain"`
	TargetURL     string            `json:"target_url"`
	IncludeWWW    bool              `json:"include_www"`
	Verified      bool              `json:"verified"`
	VerifiedAt    *time.Time        `json:"verified_at,omitempty"`
	LastCheckedAt *time.Time        `json:"last_checked_at,omitempty"`
	LastError     string            `json:"last_error,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	Records       []DomainDNSRecord `json:"records"`
	// ServedBy is who answers visitors: [RedirectServedByInstance] or
	// [RedirectServedByCloud].
	ServedBy string `json:"served_by"`
	// ServeHost is the tracking host that answers, which a proxy routes the
	// domain to.
	ServeHost string `json:"serve_host,omitempty"`
	// Reach is the last HTTP check; nil until DNS verifies.
	Reach *RedirectReach `json:"reach,omitempty"`
	// TargetActive is false when the destination is not on one of the
	// workspace's own domains, so the redirect is not served; the reason is
	// in TargetInactiveReason. nil means the server did not say.
	TargetActive         *bool  `json:"target_active,omitempty"`
	TargetInactiveReason string `json:"target_inactive_reason,omitempty"`
}

// TrackingSuggestion is the tracking host a domain's mailboxes can use.
type TrackingSuggestion struct {
	Host string `json:"host"`
	// Status is "active" (a mailbox here already uses it, verified), "found"
	// (DNS already points it at this instance) or "suggested".
	Status      string `json:"status"`
	CNAMETarget string `json:"cname_target"`
	// VendorDomain is set when a connected vendor account holds the domain,
	// so its API may write the CNAME.
	VendorDomain *VendorDomainLink `json:"vendor_domain,omitempty"`
}

// VendorDomainLink is a sending domain held by an inbox vendor account, with
// what the vendor's API can do for it.
type VendorDomainLink struct {
	Vendor       string `json:"vendor"`
	ConnectionID string `json:"connection_id"`
	Forwarding   string `json:"forwarding,omitempty"`
	CanForward   bool   `json:"can_forward"`
	// CanUnforward is whether an empty target removes the vendor's
	// forwarding.
	CanUnforward bool `json:"can_unforward"`
	// ForwardingReviewed means the vendor's staff apply a change later.
	ForwardingReviewed bool     `json:"forwarding_reviewed"`
	CanDNS             bool     `json:"can_dns"`
	DNSTypes           []string `json:"dns_types"`
}

// TrackingDomainUse is one custom tracking host a domain's mailboxes use.
type TrackingDomainUse struct {
	Host      string `json:"host"`
	Verified  bool   `json:"verified"`
	Mailboxes int    `json:"mailboxes"`
}

// SendingDomain is one domain the workspace sends from, with what makes it
// look like a real business.
type SendingDomain struct {
	Domain          string              `json:"domain"`
	Mailboxes       int                 `json:"mailboxes"`
	MailHosts       []string            `json:"mail_hosts"`
	AuthState       string              `json:"auth_state"`
	AuthSPF         bool                `json:"auth_spf"`
	AuthDKIM        bool                `json:"auth_dkim"`
	AuthDMARC       bool                `json:"auth_dmarc"`
	TrackingDomains []TrackingDomainUse `json:"tracking_domains"`
	Redirect        *DomainRedirect     `json:"redirect,omitempty"`
	// Vendors are the inbox vendors this domain's mailboxes were imported
	// from, and VendorConnectionIDs their vendor accounts.
	Vendors             []string `json:"vendors"`
	VendorConnectionIDs []string `json:"vendor_connection_ids"`
	// VendorDomain is set when a connected vendor account holds this domain.
	VendorDomain *VendorDomainLink `json:"vendor_domain,omitempty"`
}

// DomainBulkParams sets one tracking subdomain and one root redirect for up to
// 100 domains, each through its inbox vendor when the vendor can.
type DomainBulkParams struct {
	Domains []string `json:"domains"`
	// TrackingLabel sets <label>.<domain> as every listed domain's tracking
	// host.
	TrackingLabel string `json:"tracking_label,omitempty"`
	// RedirectURL sends every listed domain's root to one website.
	RedirectURL string `json:"redirect_url,omitempty"`
	// TrackingHosts and RedirectURLs override the shared value for the domains
	// they name.
	TrackingHosts map[string]string `json:"tracking_hosts,omitempty"`
	RedirectURLs  map[string]string `json:"redirect_urls,omitempty"`
	// ServedBy picks where a redirect the vendor does not forward is served
	// from; empty keeps each domain's current choice.
	ServedBy string `json:"served_by,omitempty"`
}

// DomainBulkTracking is one domain's tracking host after a bulk setup.
type DomainBulkTracking struct {
	Host string `json:"host"`
	// Via is "vendor" when the vendor wrote the CNAME and "dns" when it is the
	// customer's to add.
	Via         string `json:"via"`
	Verified    bool   `json:"verified"`
	Mailboxes   int    `json:"mailboxes"`
	CNAMETarget string `json:"cname_target,omitempty"`
	// Note says why the vendor did not write the record when it was asked.
	Note  string `json:"note,omitempty"`
	Error string `json:"error,omitempty"`
	Code  string `json:"code,omitempty"`
}

// DomainBulkRedirect is one domain's root redirect after a bulk setup.
type DomainBulkRedirect struct {
	TargetURL string `json:"target_url,omitempty"`
	// Via is "vendor", "dns" (this instance serves it) or "cloud".
	Via      string `json:"via"`
	Verified bool   `json:"verified"`
	// Reviewed means the vendor's staff apply the forwarding later.
	Reviewed bool   `json:"reviewed,omitempty"`
	Error    string `json:"error,omitempty"`
	Code     string `json:"code,omitempty"`
}

// DomainBulkResult is what one domain got from a bulk setup. A refusal is
// reported on the row and never stops the others.
type DomainBulkResult struct {
	Domain   string              `json:"domain"`
	Tracking *DomainBulkTracking `json:"tracking,omitempty"`
	Redirect *DomainBulkRedirect `json:"redirect,omitempty"`
}

// DomainTrackingResult is the answer to setting a domain's tracking host.
type DomainTrackingResult struct {
	Tracking *TrackingDomainStatus `json:"tracking"`
	// Mailboxes is how many mailboxes on the domain now use the host.
	Mailboxes int `json:"mailboxes"`
}

// DomainRedirectParams sets a domain's root redirect.
type DomainRedirectParams struct {
	TargetURL string `json:"target_url"`
	// IncludeWWW also redirects www.<domain>; nil takes the server default
	// (true).
	IncludeWWW *bool `json:"include_www,omitempty"`
	// ServedBy is [RedirectServedByInstance] or [RedirectServedByCloud]; empty
	// keeps the current choice.
	ServedBy string `json:"served_by,omitempty"`
}

// Domains lists every domain the workspace sends from, with its
// authentication, tracking hosts and redirect.
func (s *EmailService) Domains(ctx context.Context, opts ...RequestOption) ([]SendingDomain, *Response, error) {
	return fetchData[SendingDomain](ctx, s.client, "emails/domains", opts)
}

// BulkDomainSetup applies a tracking subdomain and a root redirect to up to
// 100 domains at once. Every row reports its own outcome and the call is safe
// to retry, since each setting is set, not added.
func (s *EmailService) BulkDomainSetup(ctx context.Context, params *DomainBulkParams, opts ...RequestOption) ([]DomainBulkResult, *Response, error) {
	return sendData[DomainBulkResult](ctx, s.client.post, "emails/domains/bulk", params, opts)
}

// DomainTrackingSuggestion returns the tracking host a domain's mailboxes can
// use. It is a 404 for a domain with no mailbox in the workspace and a 400
// when the instance has no tracking host.
func (s *EmailService) DomainTrackingSuggestion(ctx context.Context, domain string, opts ...RequestOption) (*TrackingSuggestion, *Response, error) {
	return fetch[TrackingSuggestion](ctx, s.client, "emails/domains/"+url.PathEscape(domain)+"/tracking-suggestion", opts)
}

// SetDomainTracking sets one tracking host for every mailbox on the domain; an
// empty host clears it. It is idempotent.
func (s *EmailService) SetDomainTracking(ctx context.Context, domain, host string, opts ...RequestOption) (*DomainTrackingResult, *Response, error) {
	body := struct {
		Host string `json:"host"`
	}{Host: host}
	return send[DomainTrackingResult](ctx, s.client.put, "emails/domains/"+url.PathEscape(domain)+"/tracking", body, opts)
}

// SetDomainVendorTracking has the inbox vendor holding the domain write the
// tracking CNAME, then points every mailbox on the domain at host. host is
// required. Safe to retry.
func (s *EmailService) SetDomainVendorTracking(ctx context.Context, domain, host string, opts ...RequestOption) (*DomainTrackingResult, *Response, error) {
	body := struct {
		Host string `json:"host"`
	}{Host: host}
	return send[DomainTrackingResult](ctx, s.client.post, "emails/domains/"+url.PathEscape(domain)+"/vendor-tracking", body, opts)
}

// SetDomainRedirect creates or updates the domain's root redirect and checks
// it once. It is idempotent: the domain keeps its verification token, so the
// TXT record stays valid.
func (s *EmailService) SetDomainRedirect(ctx context.Context, domain string, params *DomainRedirectParams, opts ...RequestOption) (*DomainRedirect, *Response, error) {
	return send[DomainRedirect](ctx, s.client.put, "emails/domains/"+url.PathEscape(domain)+"/redirect", params, opts)
}

// VerifyDomainRedirect checks the redirect's DNS now.
func (s *EmailService) VerifyDomainRedirect(ctx context.Context, domain string, opts ...RequestOption) (*DomainRedirect, *Response, error) {
	return send[DomainRedirect](ctx, s.client.post, "emails/domains/"+url.PathEscape(domain)+"/redirect/verify", nil, opts)
}

// DeleteDomainRedirect removes the domain's root redirect.
func (s *EmailService) DeleteDomainRedirect(ctx context.Context, domain string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "emails/domains/"+url.PathEscape(domain)+"/redirect", opts...)
}

// SetDomainVendorForwarding has the inbox vendor holding the domain forward
// its root to target. An empty target removes the forwarding where the vendor
// allows it. It is idempotent.
func (s *EmailService) SetDomainVendorForwarding(ctx context.Context, domain, target string, opts ...RequestOption) (*VendorDomainLink, *Response, error) {
	body := struct {
		URL string `json:"url"`
	}{URL: target}
	return send[VendorDomainLink](ctx, s.client.put, "emails/domains/"+url.PathEscape(domain)+"/vendor-forwarding", body, opts)
}
