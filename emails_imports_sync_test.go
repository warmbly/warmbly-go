package warmbly

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// exactCase asserts a call's method, path, raw query and body exactly, through
// checkRequest. An empty body asserts that none was sent.
type exactCase struct {
	name   string
	call   func() error
	method string
	path   string
	query  string
	body   string
}

func runExactCases(t *testing.T, got *recordedRequest, cases []exactCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			*got = recordedRequest{}
			if err := tc.call(); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			checkRequest(t, *got, tc.method, tc.path, tc.query, tc.body)
		})
	}
}

func TestMailboxImportRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()

	runExactCases(t, &got, []exactCase{
		{"ListImports", func() error {
			_, e := c.Emails.ListImports(ctx, &ListOptions{Limit: 50, Cursor: "cur_1"})
			return e
		}, "GET", "/v1/emails/imports", "cursor=cur_1&limit=50", ""},
		{"ListImports nil", func() error { _, e := c.Emails.ListImports(ctx, nil); return e }, "GET", "/v1/emails/imports", "", ""},
		{"GetImport", func() error { _, _, e := c.Emails.GetImport(ctx, "imp_1"); return e }, "GET", "/v1/emails/imports/imp_1", "", ""},
		{"ImportRows", func() error {
			_, e := c.Emails.ImportRows(ctx, "imp_1", &MailboxImportRowsParams{
				ListOptions: ListOptions{Limit: 200},
				Status:      []string{MailboxImportRowFailed, MailboxImportRowNeedsSignin},
				Cause:       "auth",
			})
			return e
		}, "GET", "/v1/emails/imports/imp_1/rows", "cause=auth&limit=200&status=failed%2Cneeds_signin", ""},
		{"ImportRows nil", func() error { _, e := c.Emails.ImportRows(ctx, "imp_1", nil); return e }, "GET", "/v1/emails/imports/imp_1/rows", "", ""},
		{"FixImportRow", func() error {
			_, _, e := c.Emails.FixImportRow(ctx, "imp_1", 7, &MailboxImportRowFix{
				AppPassword: String("abcd efgh ijkl mnop"),
				SMTP:        &MailboxImportLegFix{Host: String("smtp.acme.io"), Port: Int(587)},
			})
			return e
		}, "PATCH", "/v1/emails/imports/imp_1/rows/7", "",
			`{"password":null,"app_password":"abcd efgh ijkl mnop","username":null,"smtp":{"host":"smtp.acme.io","port":587,"security":null,"username":null,"password":null},"imap":null}`},
		{"RetryImport", func() error {
			_, _, e := c.Emails.RetryImport(ctx, "imp_1", &MailboxImportRetryParams{Cause: "auth", Lines: []int{2, 5}})
			return e
		}, "POST", "/v1/emails/imports/imp_1/retry", "", `{"cause":"auth","lines":[2,5]}`},
		{"RetryImport nil", func() error { _, _, e := c.Emails.RetryImport(ctx, "imp_1", nil); return e }, "POST", "/v1/emails/imports/imp_1/retry", "", ""},
		{"CancelImport", func() error { _, _, e := c.Emails.CancelImport(ctx, "imp_1"); return e }, "POST", "/v1/emails/imports/imp_1/cancel", "", ""},
		{"DismissImport", func() error { _, e := c.Emails.DismissImport(ctx, "imp_1"); return e }, "POST", "/v1/emails/imports/imp_1/dismiss", "", ""},
	})
}

const mailboxImportFixture = `{
	"id": "imp_1",
	"created_by": "usr_1",
	"status": "running",
	"source": "file",
	"filename": "inboxes.csv",
	"vendor": "",
	"on_existing": "update",
	"created_at": "2026-11-14T09:00:00Z",
	"updated_at": "2026-11-14T09:01:00Z",
	"finished_at": null,
	"credentials_expire_at": "2026-11-21T09:00:00Z",
	"total": 3,
	"counts": {"queued": 1, "running": 0, "connected": 1, "updated": 0, "skipped": 0, "failed": 1, "needs_signin": 0},
	"causes": [{"cause": "auth", "title": "Wrong password", "fix": "Use an app password.", "count": 1, "retryable": true, "google": true}],
	"authorizing": {"google": 2}
}`

func TestMailboxImportDecode(t *testing.T) {
	var got recordedRequest
	c := emailFixtureClient(t, http.StatusOK, mailboxImportFixture, &got)
	imp, _, err := c.Emails.GetImport(context.Background(), "imp_1")
	if err != nil {
		t.Fatal(err)
	}
	if imp.Status != MailboxImportRunning || imp.Finished() || imp.CreatedBy == nil || *imp.CreatedBy != "usr_1" {
		t.Errorf("import = %+v", imp)
	}
	if imp.Counts.Connected != 1 || imp.Counts.Failed != 1 || imp.Total != 3 || imp.Authorizing["google"] != 2 {
		t.Errorf("counts = %+v authorizing = %v", imp.Counts, imp.Authorizing)
	}
	if len(imp.Causes) != 1 || !imp.Causes[0].Retryable || !imp.Causes[0].Google {
		t.Errorf("causes = %+v", imp.Causes)
	}
	if imp.CredentialsExpireAt == nil || !imp.CredentialsExpireAt.Equal(time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("credentials_expire_at = %v", imp.CredentialsExpireAt)
	}
}

func TestMailboxImportRowsDecode(t *testing.T) {
	c := emailFixtureClient(t, http.StatusOK, `{
		"data": [{"line": 2, "email": "ana@acme.io", "name": "Ana", "mail_host": "google", "status": "failed",
			"code": "google_app_password_required", "cause": "auth", "message": "Google needs an app password.",
			"retryable": true, "fields": {"email": "ana@acme.io"}, "updated_at": "2026-11-14T09:01:00Z"},
			{"line": 3, "email": "bo@acme.io", "name": "", "mail_host": "other", "status": "connected", "code": "",
			"cause": "", "message": "", "email_account_id": "acc_9", "retryable": false, "fields": {}, "updated_at": "2026-11-14T09:01:00Z"}],
		"pagination": {"next_cursor": "cur_2", "has_more": true}
	}`, nil)
	page, err := c.Emails.ImportRows(context.Background(), "imp_1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 2 || !page.HasMore() || page.NextCursor() != "cur_2" {
		t.Fatalf("page = %+v", page)
	}
	if r := page.Data[0]; r.Code != ErrCodeGoogleAppPasswordRequired || r.EmailAccountID != nil || r.Fields["email"] != "ana@acme.io" {
		t.Errorf("row 0 = %+v", r)
	}
	if r := page.Data[1]; r.EmailAccountID == nil || *r.EmailAccountID != "acc_9" || r.Status != MailboxImportRowConnected {
		t.Errorf("row 1 = %+v", r)
	}
}

func TestMailboxImportPreviewDecode(t *testing.T) {
	c := emailFixtureClient(t, http.StatusOK, `{
		"format": "csv", "has_header": true, "vendor": {"id": "inboxkit", "label": "InboxKit"},
		"columns": [{"index": 0, "header": "Email", "samples": ["ana@acme.io"], "secret": false, "field": "email", "source": "alias", "confidence": 0.9},
			{"index": 1, "header": "Password", "samples": ["••••"], "secret": true, "field": "password", "source": "alias", "confidence": 0.9}],
		"mapping": {"0": "email", "1": "password"},
		"saved_mapping": false,
		"summary": {"total": 2, "ready": 1, "needs_signin": 0, "invalid": 1, "existing": 0, "duplicate": 0},
		"rows": [{"line": 2, "email": "ana@acme.io", "name": "", "mail_host": "google", "auth_method": "app_password", "status": "ready",
			"smtp": {"host": "smtp.gmail.com", "port": 587, "security": "starttls"}, "imap": {"host": "imap.gmail.com", "port": 993, "security": "tls"}}],
		"domains": [{"domain": "acme.io", "mail_host": "google", "label": "Google Workspace", "source": "mx", "rows": 2,
			"password_auth": "app_password", "app_password_url": "https://myaccount.google.com/apppasswords",
			"auth": {"state": "passing", "spf": true, "dkim": true, "dmarc": false},
			"tracking": {"host": "link.acme.io", "status": "suggested", "cname_target": "track.warmbly.test"},
			"vendor_domain": {"vendor": "inboxkit", "connection_id": "vc_1", "can_forward": true, "can_unforward": false, "forwarding_reviewed": false, "can_dns": true, "dns_types": ["CNAME"]}}],
		"issues": [{"cause": "invalid_email", "title": "Not an address", "fix": "Check the column.", "count": 1, "lines": [3]}],
		"allowance": null,
		"max_rows": 5000
	}`, nil)
	p, _, err := c.Emails.PreviewImport(context.Background(), &MailboxImportParams{Text: "ana@acme.io,secret"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Vendor == nil || p.Vendor.ID != "inboxkit" || p.Mapping["1"] != MailboxImportFieldPassword || p.MaxRows != 5000 || p.Allowance != nil {
		t.Errorf("preview = %+v", p)
	}
	if len(p.Columns) != 2 || !p.Columns[1].Secret || p.Summary.Invalid != 1 {
		t.Errorf("columns = %+v summary = %+v", p.Columns, p.Summary)
	}
	if len(p.Rows) != 1 || p.Rows[0].Status != MailboxImportPreviewReady || p.Rows[0].SMTP == nil || p.Rows[0].SMTP.Port != 587 {
		t.Errorf("rows = %+v", p.Rows)
	}
	d := p.Domains[0]
	if d.Auth == nil || d.Auth.State != AuthStatePassing || d.Tracking == nil || d.Tracking.Status != TrackingSuggestionSuggested ||
		d.VendorDomain == nil || d.VendorDomain.ConnectionID != "vc_1" || d.Redirect != nil {
		t.Errorf("domain = %+v", d)
	}
	if len(p.Issues) != 1 || p.Issues[0].Lines[0] != 3 {
		t.Errorf("issues = %+v", p.Issues)
	}
}

// multipartServer records the parsed multipart form of the last request.
func multipartServer(t *testing.T, status int, body string, form **multipart.Form, path *string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*path = r.Method + " " + r.URL.Path
		mt, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mt != "multipart/form-data" {
			t.Errorf("content type = %q", r.Header.Get("Content-Type"))
		} else {
			f, err := multipart.NewReader(r.Body, params["boundary"]).ReadForm(1 << 20)
			if err != nil {
				t.Errorf("read form: %v", err)
			}
			*form = f
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"), WithMaxRetries(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestMailboxImportCreateMultipart(t *testing.T) {
	var form *multipart.Form
	var path string
	c := multipartServer(t, http.StatusCreated, mailboxImportFixture, &form, &path)

	imp, resp, err := c.Emails.CreateImport(context.Background(), &MailboxImportParams{
		File:    &FileUpload{Filename: "inboxes.csv", Content: strings.NewReader("email,password\nana@acme.io,x\n"), ContentType: "text/csv"},
		Mapping: MailboxImportMapping{"0": MailboxImportFieldEmail, "1": MailboxImportFieldPassword},
		Options: &MailboxImportOptions{
			HasHeader:       Bool(true),
			OnExisting:      MailboxImportOnExistingSkip,
			Settings:        MailboxImportSettings{DailyLimit: Int(30), TagIDs: []string{"tag_1"}},
			TrackingDomains: map[string]string{"acme.io": "link.acme.io"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if path != "POST /v1/emails/imports" || resp.StatusCode != http.StatusCreated || imp.ID != "imp_1" {
		t.Errorf("path = %s status = %d id = %s", path, resp.StatusCode, imp.ID)
	}
	files := form.File["file"]
	if len(files) != 1 || files[0].Filename != "inboxes.csv" || files[0].Header.Get("Content-Type") != "text/csv" {
		t.Fatalf("file part = %+v", files)
	}
	fh, _ := files[0].Open()
	data, _ := io.ReadAll(fh)
	_ = fh.Close()
	if !bytes.HasPrefix(data, []byte("email,password")) {
		t.Errorf("file = %q", data)
	}
	if got := form.Value["mapping"]; len(got) != 1 || got[0] != `{"0":"email","1":"password"}` {
		t.Errorf("mapping = %v", got)
	}
	if got := form.Value["options"]; len(got) != 1 || got[0] != `{"has_header":true,"on_existing":"skip","settings":{"tag_ids":["tag_1"],"daily_limit":30},"tracking_domains":{"acme.io":"link.acme.io"}}` {
		t.Errorf("options = %v", got)
	}
	if _, ok := form.Value["text"]; ok {
		t.Error("text sent alongside a file")
	}
}

func TestMailboxImportPreviewPasteMultipart(t *testing.T) {
	var form *multipart.Form
	var path string
	c := multipartServer(t, http.StatusOK, `{"format":"paste","columns":[],"rows":[],"domains":[],"issues":[],"max_rows":5000}`, &form, &path)

	if _, _, err := c.Emails.PreviewImport(context.Background(), &MailboxImportParams{Text: "ana@acme.io:secret"}); err != nil {
		t.Fatal(err)
	}
	if path != "POST /v1/emails/imports/preview" {
		t.Errorf("path = %s", path)
	}
	if got := form.Value["text"]; len(got) != 1 || got[0] != "ana@acme.io:secret" {
		t.Errorf("text = %v", got)
	}
	if _, ok := form.Value["mapping"]; ok {
		t.Error("empty mapping was sent")
	}
	if _, ok := form.Value["options"]; ok {
		t.Error("nil options were sent")
	}
}

func TestMailboxImportParamsRequired(t *testing.T) {
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL("http://127.0.0.1:1/v1/"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for name, p := range map[string]*MailboxImportParams{
		"nil":          nil,
		"empty":        {},
		"file no name": {File: &FileUpload{Content: strings.NewReader("x")}},
	} {
		if _, _, err := c.Emails.CreateImport(ctx, p); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestMailboxImportDownloadFailures(t *testing.T) {
	var got recordedRequest
	c := emailFixtureClient(t, http.StatusOK, "line,email,error\n2,ana@acme.io,Wrong password\n", &got)
	var buf bytes.Buffer
	if _, err := c.Emails.DownloadImportFailures(context.Background(), "imp_1", &buf); err != nil {
		t.Fatal(err)
	}
	checkRequest(t, got, "GET", "/v1/emails/imports/imp_1/failed.csv", "", "")
	if !strings.HasPrefix(buf.String(), "line,email,error\n2,ana@acme.io") {
		t.Errorf("csv = %q", buf.String())
	}
}

func TestMailboxImportOptionsZero(t *testing.T) {
	b, err := json.Marshal(MailboxImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"settings":{}}` {
		t.Errorf("zero options = %s", b)
	}
}
