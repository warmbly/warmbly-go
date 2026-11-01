package warmbly

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newCaptureServer answers every request with status and body after handing
// the request and its raw body to onReq.
func newCaptureServer(t *testing.T, onReq func(r *http.Request, body string), status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		onReq(r, string(b))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newClientFor returns a client pointed at srv with retries off.
func newClientFor(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"), WithMaxRetries(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// TestMailboxSyncRouting asserts the method, path, raw query and body of every
// mailbox route added in the second API sync: imports, grants, sending
// domains, inbox vendors, per-mailbox settings, the email image library and
// the unibox folder move.
func TestMailboxSyncRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()

	cases := []struct {
		name       string
		call       func() error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string // "" asserts an empty body; otherwise exact JSON
	}{
		// Imports.
		{"ListImports", func() error {
			_, e := c.Emails.ListImports(ctx, &ListOptions{Limit: 20, Cursor: "c1"})
			return e
		}, "GET", "/v1/emails/imports", "cursor=c1&limit=20", ""},
		{"GetImport", func() error { _, _, e := c.Emails.GetImport(ctx, "imp_1"); return e }, "GET", "/v1/emails/imports/imp_1", "", ""},
		{"ImportRows", func() error {
			_, e := c.Emails.ImportRows(ctx, "imp_1", &MailboxImportRowsParams{
				ListOptions: ListOptions{Limit: 200},
				Status:      []string{MailboxImportRowFailed, MailboxImportRowNeedsSignin},
				Cause:       "missing_password",
			})
			return e
		}, "GET", "/v1/emails/imports/imp_1/rows", "cause=missing_password&limit=200&status=failed%2Cneeds_signin", ""},
		{"FixImportRow", func() error {
			_, _, e := c.Emails.FixImportRow(ctx, "imp_1", 7, &MailboxImportRowFix{
				Password: String("pw"),
				IMAP:     &MailboxImportLegFix{Host: String("imap.acme.io"), Port: Int(993)},
			})
			return e
		}, "PATCH", "/v1/emails/imports/imp_1/rows/7", "",
			`{"password":"pw","app_password":null,"username":null,"smtp":null,"imap":{"host":"imap.acme.io","port":993,"security":null,"username":null,"password":null}}`},
		{"RetryImport", func() error {
			_, _, e := c.Emails.RetryImport(ctx, "imp_1", &MailboxImportRetryParams{Cause: "no_worker", Lines: []int{2, 3}})
			return e
		}, "POST", "/v1/emails/imports/imp_1/retry", "", `{"cause":"no_worker","lines":[2,3],"password":""}`},
		{"RetryImport all", func() error { _, _, e := c.Emails.RetryImport(ctx, "imp_1", nil); return e }, "POST", "/v1/emails/imports/imp_1/retry", "", ""},
		{"CancelImport", func() error { _, _, e := c.Emails.CancelImport(ctx, "imp_1"); return e }, "POST", "/v1/emails/imports/imp_1/cancel", "", ""},
		{"DismissImport", func() error { _, e := c.Emails.DismissImport(ctx, "imp_1"); return e }, "POST", "/v1/emails/imports/imp_1/dismiss", "", ""},
		{"DownloadImportFailures", func() error {
			var buf bytes.Buffer
			_, e := c.Emails.DownloadImportFailures(ctx, "imp_1", &buf)
			return e
		}, "GET", "/v1/emails/imports/imp_1/failed.csv", "", ""},

		// Grants.
		{"GrantConfig", func() error { _, _, e := c.Emails.GrantConfig(ctx); return e }, "GET", "/v1/emails/grants/config", "", ""},
		{"ListGrants", func() error { _, _, e := c.Emails.ListGrants(ctx); return e }, "GET", "/v1/emails/grants", "", ""},
		{"SigninMigration", func() error { _, _, e := c.Emails.SigninMigration(ctx); return e }, "GET", "/v1/emails/grants/migration", "", ""},
		{"StartGoogleGrant", func() error {
			_, _, e := c.Emails.StartGoogleGrant(ctx, "acme.io", "admin@acme.io")
			return e
		}, "POST", "/v1/emails/grants/google/start", "", `{"domain":"acme.io","admin_email":"admin@acme.io"}`},
		{"FinishGoogleGrant", func() error {
			_, _, e := c.Emails.FinishGoogleGrant(ctx, &GoogleGrantFinishParams{Domain: "acme.io", AdminEmail: "admin@acme.io"})
			return e
		}, "POST", "/v1/emails/grants/google/finish", "", `{"domain":"acme.io","admin_email":"admin@acme.io"}`},
		{"StartMicrosoftGrant", func() error { _, _, e := c.Emails.StartMicrosoftGrant(ctx); return e }, "POST", "/v1/emails/grants/microsoft/start", "", ""},
		{"FinishMicrosoftGrant", func() error {
			_, _, e := c.Emails.FinishMicrosoftGrant(ctx, "st", "cd")
			return e
		}, "POST", "/v1/emails/grants/microsoft/finish", "", `{"state":"st","code":"cd"}`},
		{"GetGrant", func() error { _, _, e := c.Emails.GetGrant(ctx, "g_1"); return e }, "GET", "/v1/emails/grants/g_1", "", ""},
		{"CheckGrant", func() error { _, _, e := c.Emails.CheckGrant(ctx, "g_1"); return e }, "POST", "/v1/emails/grants/g_1/check", "", ""},
		{"DeleteGrant", func() error { _, e := c.Emails.DeleteGrant(ctx, "g_1"); return e }, "DELETE", "/v1/emails/grants/g_1", "", ""},
		{"GrantUsers", func() error { _, _, e := c.Emails.GrantUsers(ctx, "g_1"); return e }, "GET", "/v1/emails/grants/g_1/users", "", ""},
		{"ConnectGrantUsers", func() error {
			_, _, e := c.Emails.ConnectGrantUsers(ctx, "g_1", &GrantConnectParams{UserIDs: []string{"a@acme.io"}})
			return e
		}, "POST", "/v1/emails/grants/g_1/connect", "",
			`{"user_ids":["a@acme.io"],"all":false,"options":{"has_header":null,"shared_password":"","on_existing":"","save_mapping":null,"settings":{}}}`},

		// Sending domains.
		{"ListDomains", func() error { _, _, e := c.Emails.ListDomains(ctx); return e }, "GET", "/v1/emails/domains", "", ""},
		{"BulkSetupDomains", func() error {
			_, _, e := c.Emails.BulkSetupDomains(ctx, &BulkDomainSetupParams{
				Domains: []string{"a.io", "b.io"}, TrackingLabel: "link", RedirectURL: "https://acme.com",
			})
			return e
		}, "POST", "/v1/emails/domains/bulk", "", `{"domains":["a.io","b.io"],"tracking_label":"link","redirect_url":"https://acme.com"}`},
		{"TrackingSuggestion", func() error { _, _, e := c.Emails.TrackingSuggestion(ctx, "a.io"); return e }, "GET", "/v1/emails/domains/a.io/tracking-suggestion", "", ""},
		{"SetDomainTracking", func() error { _, _, e := c.Emails.SetDomainTracking(ctx, "a.io", "link.a.io"); return e }, "PUT", "/v1/emails/domains/a.io/tracking", "", `{"host":"link.a.io"}`},
		{"SetDomainRedirect", func() error {
			_, _, e := c.Emails.SetDomainRedirect(ctx, "a.io", &DomainRedirectParams{TargetURL: "https://acme.com", IncludeWWW: Bool(false), ServedBy: RedirectServedByCloud})
			return e
		}, "PUT", "/v1/emails/domains/a.io/redirect", "", `{"target_url":"https://acme.com","include_www":false,"served_by":"cloud"}`},
		{"VerifyDomainRedirect", func() error { _, _, e := c.Emails.VerifyDomainRedirect(ctx, "a.io"); return e }, "POST", "/v1/emails/domains/a.io/redirect/verify", "", ""},
		{"VerifyRedirectWebsite", func() error {
			_, _, e := c.Emails.VerifyRedirectWebsite(ctx, "a.io", "https://acme.com")
			return e
		}, "POST", "/v1/emails/domains/a.io/redirect/website", "", `{"target_url":"https://acme.com"}`},
		{"DeleteDomainRedirect", func() error { _, e := c.Emails.DeleteDomainRedirect(ctx, "a.io"); return e }, "DELETE", "/v1/emails/domains/a.io/redirect", "", ""},
		{"SetDomainVendorForwarding", func() error {
			_, _, e := c.Emails.SetDomainVendorForwarding(ctx, "a.io", "")
			return e
		}, "PUT", "/v1/emails/domains/a.io/vendor-forwarding", "", `{"url":""}`},
		{"SetDomainVendorTracking", func() error {
			_, _, e := c.Emails.SetDomainVendorTracking(ctx, "a.io", "link.a.io")
			return e
		}, "POST", "/v1/emails/domains/a.io/vendor-tracking", "", `{"host":"link.a.io"}`},

		// Inbox vendors.
		{"VendorCatalog", func() error { _, _, e := c.Emails.VendorCatalog(ctx); return e }, "GET", "/v1/emails/vendors/catalog", "", ""},
		{"ListVendors", func() error { _, _, e := c.Emails.ListVendors(ctx); return e }, "GET", "/v1/emails/vendors", "", ""},
		{"CreateVendor", func() error {
			_, _, e := c.Emails.CreateVendor(ctx, &VendorCreateParams{Vendor: "inboxkit", Label: "Main", Fields: map[string]string{"api_key": "k"}})
			return e
		}, "POST", "/v1/emails/vendors", "", `{"vendor":"inboxkit","label":"Main","fields":{"api_key":"k"}}`},
		{"UpdateVendor", func() error {
			_, _, e := c.Emails.UpdateVendor(ctx, "v_1", &VendorCreateParams{Label: "Renamed"})
			return e
		}, "PATCH", "/v1/emails/vendors/v_1", "", `{"vendor":"","label":"Renamed","fields":null}`},
		{"DeleteVendor", func() error { _, e := c.Emails.DeleteVendor(ctx, "v_1"); return e }, "DELETE", "/v1/emails/vendors/v_1", "", ""},
		{"VendorMailboxes", func() error { _, _, e := c.Emails.VendorMailboxes(ctx, "v_1"); return e }, "GET", "/v1/emails/vendors/v_1/mailboxes", "", ""},
		{"ImportVendorMailboxes", func() error {
			_, _, e := c.Emails.ImportVendorMailboxes(ctx, "v_1", &VendorImportParams{All: true})
			return e
		}, "POST", "/v1/emails/vendors/v_1/import", "",
			`{"mailbox_ids":null,"all":true,"options":{"has_header":null,"shared_password":"","on_existing":"","save_mapping":null,"settings":{}}}`},

		// Per-mailbox settings.
		{"SetDirectTracking", func() error { _, _, e := c.Emails.SetDirectTracking(ctx, "em_1", true); return e }, "PATCH", "/v1/emails/em_1/direct-tracking", "", `{"enabled":true}`},
		{"UpdateSyncFolders", func() error {
			_, _, e := c.Emails.UpdateSyncFolders(ctx, "em_1", []string{"Archive/Old"})
			return e
		}, "PUT", "/v1/emails/em_1/sync", "", `{"skip_folders":["Archive/Old"]}`},
		{"UpdateSyncFolders clear", func() error { _, _, e := c.Emails.UpdateSyncFolders(ctx, "em_1", nil); return e }, "PUT", "/v1/emails/em_1/sync", "", `{"skip_folders":[]}`},
		{"Certificates", func() error { _, _, e := c.Emails.Certificates(ctx, "em_1"); return e }, "GET", "/v1/emails/em_1/certificates", "", ""},
		{"CheckCertificates", func() error { _, _, e := c.Emails.CheckCertificates(ctx, "em_1"); return e }, "POST", "/v1/emails/onboarding/smtp-imap/em_1/certificates/check", "", ""},
		{"ConfirmCertificates", func() error {
			_, _, e := c.Emails.ConfirmCertificates(ctx, "em_1", &ConfirmCertificatesParams{SMTPPin: "aa", IMAPPin: "bb"})
			return e
		}, "POST", "/v1/emails/onboarding/smtp-imap/em_1/certificates/confirm", "", `{"smtp_pin":"aa","imap_pin":"bb"}`},
		{"SendIdentity", func() error { _, _, e := c.Emails.SendIdentity(ctx, "em_1"); return e }, "GET", "/v1/emails/em_1/identity", "", ""},
		{"RefreshSendIdentity", func() error { _, _, e := c.Emails.RefreshSendIdentity(ctx, "em_1", false); return e }, "POST", "/v1/emails/em_1/identity/refresh", "", ""},
		{"RefreshSendIdentity signature", func() error { _, _, e := c.Emails.RefreshSendIdentity(ctx, "em_1", true); return e }, "POST", "/v1/emails/em_1/identity/refresh", "", `{"import_signature":true}`},
		{"SwitchToAppPassword", func() error { _, _, e := c.Emails.SwitchToAppPassword(ctx, "em_1", "abcd efgh"); return e }, "POST", "/v1/emails/onboarding/app-password/em_1", "", `{"app_password":"abcd efgh"}`},
		{"StartOAuthWithParams", func() error {
			_, _, e := c.Emails.StartOAuthWithParams(ctx, &MailboxOAuthStartParams{Provider: ProviderOutlook, LoginHint: "a@acme.io", Return: "web"})
			return e
		}, "POST", "/v1/emails/onboarding/oauth/start", "", `{"provider":"outlook","login_hint":"a@acme.io","return":"web"}`},

		// Email images.
		{"ListImages", func() error { _, e := c.Emails.ListImages(ctx, &ListOptions{Limit: 5}); return e }, "GET", "/v1/email-images", "limit=5", ""},
		{"DeleteImage", func() error { _, e := c.Emails.DeleteImage(ctx, "img_1"); return e }, "DELETE", "/v1/email-images/img_1", "", ""},

		// Template analysis and unibox folder move.
		{"Unibox.MoveToFolder", func() error {
			_, _, e := c.Unibox.MoveToFolder(ctx, &UniboxMoveParams{ThreadIDs: []string{"t1"}, Folder: FolderArchive})
			return e
		}, "PATCH", "/v1/unibox/folder", "", `{"email_ids":null,"thread_ids":["t1"],"folder":"archive"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got = recordedRequest{}
			if err := tc.call(); err != nil {
				t.Fatalf("call: %v", err)
			}
			if got.method != tc.wantMethod || got.path != tc.wantPath {
				t.Errorf("got %s %s, want %s %s", got.method, got.path, tc.wantMethod, tc.wantPath)
			}
			if got.rawQuery != tc.wantQuery {
				t.Errorf("query = %q, want %q", got.rawQuery, tc.wantQuery)
			}
			if got.body != tc.wantBody {
				// The JSON encoder ends its output with a newline.
				if strings.TrimSpace(got.body) != tc.wantBody {
					t.Errorf("body = %q, want %q", got.body, tc.wantBody)
				}
			}
		})
	}
}

func TestTemplateAnalyzeDecode(t *testing.T) {
	var gotKey, gotPath, gotBody string
	srv := newCaptureServer(t, func(r *http.Request, body string) {
		gotKey, gotPath, gotBody = r.Header.Get("Idempotency-Key"), r.URL.Path, body
	}, http.StatusOK, `{
		"score": 62,
		"verdict": "Reads like a mass mailing.",
		"findings": [
			{"severity": "high", "field": "subject", "text": "FREE", "line": 1, "excerpt": "FREE audit", "issue": "Trigger word.", "suggestion": "Say what you offer."},
			{"severity": "info", "issue": "No clear ask."}
		],
		"suggested_subject": "A quick idea for Acme",
		"improvements": ["Shorten the first paragraph."],
		"rules": {"score": 70, "issues": [
			{"severity": "warn", "code": "all_caps", "message": "ALL CAPS word.", "field": "subject",
			 "spans": [{"field": "subject", "text": "FREE", "line": 1, "excerpt": "FREE audit"}], "suggestion": "Use sentence case."}
		]},
		"judgment": {"reads_as": 0.8, "personalization": 0.9, "ask": "no_ask", "spam_claim": 0.1, "confidence": 0.9, "model": "m", "input_tokens": 120},
		"model": "claude",
		"tokens_used": 900,
		"credits_remaining": 41,
		"credits_charged": 2
	}`)
	c := newClientFor(t, srv)

	a, _, err := c.Templates.Analyze(context.Background(), &TemplateScoreParams{Subject: "FREE audit"}, WithIdempotencyKey("k1"))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if gotKey != "k1" || gotPath != "/v1/templates/analyze" || strings.TrimSpace(gotBody) != `{"subject":"FREE audit"}` {
		t.Errorf("request = key %q path %q body %q", gotKey, gotPath, gotBody)
	}
	if a.Score != 62 || len(a.Findings) != 2 || a.Findings[0].Field != TemplateFieldSubject || a.Findings[0].Line != 1 {
		t.Errorf("findings = %+v", a.Findings)
	}
	if a.Findings[1].Severity != TemplateIssueInfo || a.SuggestedSubject == "" || len(a.Improvements) != 1 {
		t.Errorf("analysis = %+v", a)
	}
	if a.Rules.Score != 70 || len(a.Rules.Issues) != 1 || len(a.Rules.Issues[0].Spans) != 1 || a.Rules.Issues[0].Spans[0].Text != "FREE" || a.Rules.Issues[0].Suggestion == "" {
		t.Errorf("rules = %+v", a.Rules)
	}
	if a.Judgment == nil || a.Judgment.Ask != "no_ask" || a.Judgment.InputTokens != 120 {
		t.Errorf("judgment = %+v", a.Judgment)
	}
	if a.CreditsCharged != 2 || a.CreditsRemaining != 41 || a.TokensUsed != 900 {
		t.Errorf("credits = %+v", a)
	}
}

// parseForm decodes a recorded multipart body into its text fields and the
// name of its file part.
func parseForm(t *testing.T, contentType, body string) (fields map[string]string, fileField, fileName string) {
	t.Helper()
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("content type %q: %v", contentType, err)
	}
	mr := multipart.NewReader(strings.NewReader(body), params["boundary"])
	fields = map[string]string{}
	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(p)
		if p.FileName() != "" {
			fileField, fileName = p.FormName(), p.FileName()
			continue
		}
		fields[p.FormName()] = buf.String()
	}
	return fields, fileField, fileName
}

func TestMailboxImportMultipart(t *testing.T) {
	var gotCT, gotPath, gotBody string
	srv := newCaptureServer(t, func(r *http.Request, body string) {
		gotCT, gotPath, gotBody = r.Header.Get("Content-Type"), r.URL.Path, body
	}, http.StatusCreated, `{"id":"imp_1","status":"running","total":2}`)
	c := newClientFor(t, srv)
	ctx := context.Background()

	// A file with a mapping and options.
	imp, resp, err := c.Emails.CreateImport(ctx, &MailboxImportParams{
		File:    &FileUpload{Filename: "boxes.csv", Content: strings.NewReader("email,password\na@acme.io,pw\n")},
		Mapping: MailboxImportMapping{"0": ImportFieldEmail, "1": ImportFieldPassword},
		Options: &MailboxImportOptions{OnExisting: MailboxImportOnExistingSkip, SharedPassword: "x"},
	})
	if err != nil {
		t.Fatalf("CreateImport: %v", err)
	}
	if resp.StatusCode != http.StatusCreated || imp.ID != "imp_1" || imp.Status != MailboxImportRunning {
		t.Errorf("response = %d %+v", resp.StatusCode, imp)
	}
	if gotPath != "/v1/emails/imports" {
		t.Errorf("path = %q", gotPath)
	}
	fields, fileField, fileName := parseForm(t, gotCT, gotBody)
	if fileField != "file" || fileName != "boxes.csv" {
		t.Errorf("file part = %q %q", fileField, fileName)
	}
	if fields["mapping"] != `{"0":"email","1":"password"}` {
		t.Errorf("mapping = %q", fields["mapping"])
	}
	if !strings.Contains(fields["options"], `"on_existing":"skip"`) || !strings.Contains(fields["options"], `"shared_password":"x"`) {
		t.Errorf("options = %q", fields["options"])
	}

	// Pasted text goes in the text field and sends no file part.
	_, _, err = c.Emails.PreviewImport(ctx, &MailboxImportParams{Text: "a@acme.io,pw"})
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	if gotPath != "/v1/emails/imports/preview" {
		t.Errorf("preview path = %q", gotPath)
	}
	fields, fileField, _ = parseForm(t, gotCT, gotBody)
	if fileField != "" || fields["text"] != "a@acme.io,pw" {
		t.Errorf("text form = %q file=%q", fields["text"], fileField)
	}

	// Neither a file nor text is a client-side error, before any request.
	if _, _, err := c.Emails.CreateImport(ctx, &MailboxImportParams{}); err == nil {
		t.Error("CreateImport with no input succeeded")
	}
	if _, _, err := c.Emails.CreateImport(ctx, nil); err == nil {
		t.Error("CreateImport with nil params succeeded")
	}
}

func TestEmailImageUploadMultipart(t *testing.T) {
	var gotCT, gotPath, gotBody string
	srv := newCaptureServer(t, func(r *http.Request, body string) {
		gotCT, gotPath, gotBody = r.Header.Get("Content-Type"), r.URL.Path, body
	}, http.StatusCreated, `{
		"id": "0c0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b",
		"organization_id": "f9e8d7c6-0000-0000-0000-000000000000",
		"user_id": "a1b2c3d4-0000-0000-0000-000000000000",
		"filename": "logo.png",
		"mime_type": "image/png",
		"size": 2048,
		"width": 400,
		"height": 120,
		"url": "https://cdn.acme.io/email-images/f9e8/abc.png",
		"created_at": "2026-09-10T08:00:00Z"
	}`)
	c := newClientFor(t, srv)

	img, resp, err := c.Emails.UploadImage(context.Background(), &FileUpload{Filename: "logo.png", Content: strings.NewReader("png")})
	if err != nil {
		t.Fatalf("UploadImage: %v", err)
	}
	if resp.StatusCode != http.StatusCreated || gotPath != "/v1/email-images" {
		t.Errorf("got %d %s", resp.StatusCode, gotPath)
	}
	_, fileField, fileName := parseForm(t, gotCT, gotBody)
	if fileField != "file" || fileName != "logo.png" {
		t.Errorf("file part = %q %q", fileField, fileName)
	}
	if img.Size != 2048 || img.Width != 400 || img.UserID == nil || img.MimeType != "image/png" || img.CreatedAt.Year() != 2026 {
		t.Errorf("image = %+v", img)
	}
}
