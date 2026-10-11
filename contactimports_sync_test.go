package warmbly

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// capturedRequest is one request a syncServer saw, including the headers a
// multipart upload is judged by.
type capturedRequest struct {
	method      string
	path        string
	rawQuery    string
	body        string
	contentType string
}

// syncServer answers every request with one status, content type and body,
// and records the last request into got.
func syncServer(t *testing.T, status int, contentType, body string, got *capturedRequest) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		*got = capturedRequest{
			method:      r.Method,
			path:        r.URL.Path,
			rawQuery:    r.URL.RawQuery,
			body:        string(raw),
			contentType: r.Header.Get("Content-Type"),
		}
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
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

// assertRequest checks method, path, exact raw query and body fragments.
func assertRequest(t *testing.T, got capturedRequest, method, path, query string, body ...string) {
	t.Helper()
	if got.method != method || got.path != path {
		t.Fatalf("got %s %s, want %s %s", got.method, got.path, method, path)
	}
	if got.rawQuery != query {
		t.Errorf("query = %q, want %q", got.rawQuery, query)
	}
	for _, frag := range body {
		if !strings.Contains(got.body, frag) {
			t.Errorf("body %s missing %s", got.body, frag)
		}
	}
}

// multipartFile parses a captured multipart body and returns the named file
// part's filename and content.
func multipartFile(t *testing.T, got capturedRequest, field string) (string, string) {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(got.contentType)
	if err != nil || mediaType != "multipart/form-data" {
		t.Fatalf("content type = %q, want multipart/form-data", got.contentType)
	}
	mr := multipart.NewReader(strings.NewReader(got.body), params["boundary"])
	for {
		part, err := mr.NextPart()
		if err != nil {
			t.Fatalf("no %q part: %v", field, err)
		}
		if part.FormName() == field {
			data, _ := io.ReadAll(part)
			return part.FileName(), string(data)
		}
	}
}

// draftImportJSON is a draft as POST /contacts/imports answers it: no options
// yet, the preview with the column stats and a remembered mapping, and notes
// as an empty array.
const draftImportJSON = `{
  "id": "imp_1",
  "organization_id": "org_1",
  "created_by": "usr_1",
  "filename": "leads.csv",
  "format": "csv",
  "status": "draft",
  "has_header": true,
  "columns": ["Email", "First name", "Plan"],
  "total": 3,
  "processed": 0,
  "imported": 0,
  "updated": 0,
  "skipped": 0,
  "failed": 0,
  "notes": [],
  "created_at": "2026-10-01T09:00:00Z",
  "updated_at": "2026-10-01T09:00:00Z",
  "preview": {
    "filename": "leads.csv",
    "format": "csv",
    "total_rows": 3,
    "columns": ["Email", "First name", "Plan"],
    "has_header": true,
    "sample_rows": [["ada@example.com", "Ada", "pro"]],
    "suggested_mapping": [
      {"index": 0, "target": "email"},
      {"index": 1, "target": "first_name"},
      {"index": 2, "target": "custom", "custom_key": "plan"}
    ],
    "inferred_columns": [2],
    "column_stats": [
      {"filled": 3, "distinct": 3, "samples": ["ada@example.com"]},
      {"filled": 2, "distinct": 2, "samples": ["Ada", "Grace"]},
      {"filled": 3, "distinct": 1, "samples": ["pro"]}
    ],
    "mapping_source": "saved"
  }
}`

// finishedImportJSON is a completed import with its options, quality, a
// failed segment pin and the first failed rows.
const finishedImportJSON = `{
  "id": "imp_2",
  "organization_id": "org_1",
  "filename": "leads.xlsx",
  "format": "xlsx",
  "status": "completed",
  "has_header": true,
  "columns": ["Email"],
  "total": 4,
  "processed": 4,
  "imported": 2,
  "updated": 1,
  "skipped": 0,
  "failed": 1,
  "options": {
    "mapping": [{"index": 0, "target": "email"}],
    "dedup": "update",
    "has_header": true,
    "segment_ids": ["seg_1"]
  },
  "quality": {"malformed": 1, "disposable": 0, "role": 1, "bad_share_pct": 25, "flagged": false},
  "segments_pinned": false,
  "notes": ["segment seg_1 could not be updated"],
  "created_at": "2026-10-01T09:00:00Z",
  "updated_at": "2026-10-01T09:05:00Z",
  "started_at": "2026-10-01T09:01:00Z",
  "finished_at": "2026-10-01T09:05:00Z",
  "failures": [{"line": 4, "email": "nope", "values": ["nope"], "reason": "invalid email"}]
}`

func TestContactImportCreateUploadsMultipart(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusCreated, "application/json", draftImportJSON, &got)

	imp, resp, err := c.Contacts.CreateImport(context.Background(), &FileUpload{
		Filename: "leads.csv",
		Content:  strings.NewReader("Email\nada@example.com\n"),
	})
	if err != nil {
		t.Fatalf("CreateImport: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", resp.StatusCode)
	}
	assertRequest(t, got, "POST", "/v1/contacts/imports", "")
	name, content := multipartFile(t, got, "file")
	if name != "leads.csv" || content != "Email\nada@example.com\n" {
		t.Errorf("file part = %q %q", name, content)
	}

	if imp.ID != "imp_1" || imp.Status != ContactImportStatusDraft || imp.CreatedBy == nil || *imp.CreatedBy != "usr_1" {
		t.Errorf("import = %+v", imp)
	}
	if imp.Notes == nil || len(imp.Notes) != 0 || imp.Options != nil || imp.StartedAt != nil {
		t.Errorf("draft fields = notes %v options %v started %v", imp.Notes, imp.Options, imp.StartedAt)
	}
	p := imp.Preview
	if p == nil || p.MappingSource != ImportMappingSaved || len(p.InferredColumns) != 1 || p.InferredColumns[0] != 2 {
		t.Fatalf("preview = %+v", p)
	}
	if len(p.ColumnStats) != 3 || p.ColumnStats[1].Filled != 2 || p.ColumnStats[1].Samples[1] != "Grace" {
		t.Errorf("column stats = %+v", p.ColumnStats)
	}
	if p.SuggestedMapping[2].Target != ImportTargetCustom || p.SuggestedMapping[2].CustomKey != "plan" {
		t.Errorf("mapping = %+v", p.SuggestedMapping)
	}
}

func TestContactImportCreateRequiresFile(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusCreated, "application/json", draftImportJSON, &got)
	if _, _, err := c.Contacts.CreateImport(context.Background(), nil); err == nil {
		t.Fatal("CreateImport(nil) succeeded")
	}
	if got.method != "" {
		t.Errorf("a request was sent: %s %s", got.method, got.path)
	}
}

func TestContactImportGetDecodesFinished(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusOK, "application/json", finishedImportJSON, &got)

	imp, _, err := c.Contacts.GetImport(context.Background(), "imp 2")
	if err != nil {
		t.Fatalf("GetImport: %v", err)
	}
	assertRequest(t, got, "GET", "/v1/contacts/imports/imp 2", "")
	if imp.Status != ContactImportStatusCompleted || imp.Processed != 4 || imp.Imported+imp.Updated+imp.Skipped+imp.Failed != imp.Total {
		t.Errorf("counters = %+v", imp)
	}
	if imp.SegmentsPinned == nil || *imp.SegmentsPinned {
		t.Errorf("segments_pinned = %v, want false", imp.SegmentsPinned)
	}
	if imp.Options == nil || imp.Options.Dedup != ImportDedupUpdate || len(imp.Options.SegmentIDs) != 1 {
		t.Errorf("options = %+v", imp.Options)
	}
	if imp.Quality == nil || imp.Quality.BadSharePct != 25 || imp.FinishedAt == nil {
		t.Errorf("quality/finished = %+v %v", imp.Quality, imp.FinishedAt)
	}
	if len(imp.Failures) != 1 || imp.Failures[0].Line != 4 || imp.Failures[0].Reason != "invalid email" {
		t.Errorf("failures = %+v", imp.Failures)
	}
}

func TestContactImportListPaginates(t *testing.T) {
	var got capturedRequest
	body := `{"data": [` + finishedImportJSON + `], "pagination": {"total": null, "next_cursor": "c2", "has_more": true}}`
	c := syncServer(t, http.StatusOK, "application/json", body, &got)

	page, err := c.Contacts.ListImports(context.Background(), &ListOptions{Limit: 10, Cursor: "c1"})
	if err != nil {
		t.Fatalf("ListImports: %v", err)
	}
	assertRequest(t, got, "GET", "/v1/contacts/imports", "cursor=c1&limit=10")
	if len(page.Data) != 1 || page.Data[0].ID != "imp_2" || !page.HasMore() || page.NextCursor() != "c2" {
		t.Errorf("page = %+v", page)
	}
	if page.Pagination.Total != nil {
		t.Errorf("total = %v, want nil", *page.Pagination.Total)
	}
	if _, err := page.Next(context.Background()); err != nil {
		t.Fatalf("Next: %v", err)
	}
	assertRequest(t, got, "GET", "/v1/contacts/imports", "cursor=c2&limit=10")

	if _, err := c.Contacts.ListImports(context.Background(), nil); err != nil {
		t.Fatalf("ListImports(nil): %v", err)
	}
	assertRequest(t, got, "GET", "/v1/contacts/imports", "")
}

func TestContactImportWrites(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusOK, "application/json", finishedImportJSON, &got)
	ctx := context.Background()
	params := &ContactImportParams{
		Mapping:           []ImportColumnMapping{{Index: 0, Target: ImportTargetEmail}},
		Dedup:             ImportDedupSkip,
		HasHeader:         true,
		CampaignIDs:       []string{"camp_1"},
		SubscribedDefault: Bool(false),
	}

	if _, _, err := c.Contacts.SaveImportDraft(ctx, "imp_1", params); err != nil {
		t.Fatalf("SaveImportDraft: %v", err)
	}
	assertRequest(t, got, "PATCH", "/v1/contacts/imports/imp_1", "",
		`"mapping":[{"index":0,"target":"email"}]`, `"dedup":"skip"`, `"has_header":true`,
		`"campaign_ids":["camp_1"]`, `"subscribed_default":false`)

	if _, _, err := c.Contacts.StartImport(ctx, "imp_1", params); err != nil {
		t.Fatalf("StartImport: %v", err)
	}
	assertRequest(t, got, "POST", "/v1/contacts/imports/imp_1/start", "", `"dedup":"skip"`)

	// A nil configuration still sends a JSON object, which the handler needs.
	if _, _, err := c.Contacts.StartImport(ctx, "imp_1", nil); err != nil {
		t.Fatalf("StartImport(nil): %v", err)
	}
	assertRequest(t, got, "POST", "/v1/contacts/imports/imp_1/start", "", `"mapping":null`)

	if _, _, err := c.Contacts.CancelImport(ctx, "imp_1"); err != nil {
		t.Fatalf("CancelImport: %v", err)
	}
	assertRequest(t, got, "POST", "/v1/contacts/imports/imp_1/cancel", "")
	if got.body != "" {
		t.Errorf("cancel body = %q, want none", got.body)
	}
}

func TestContactImportAnalyze(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusOK, "application/json", `{
  "rows": 10, "new": 5, "existing": 2, "duplicates_in_file": 1, "invalid": 1, "conflicts": 1,
  "invalid_samples": [{"line": 3, "email": "x@", "values": ["x@"], "reason": "invalid email"}],
  "quality": {"malformed": 1, "disposable": 0, "role": 0, "bad_share_pct": 10, "flagged": false},
  "problem": "this import would pass the plan's contact limit"
}`, &got)

	a, _, err := c.Contacts.AnalyzeImport(context.Background(), "imp_1", &ContactImportAnalyzeParams{
		Mapping:   []ImportColumnMapping{{Index: 0, Target: ImportTargetEmail}},
		HasHeader: true,
	})
	if err != nil {
		t.Fatalf("AnalyzeImport: %v", err)
	}
	assertRequest(t, got, "POST", "/v1/contacts/imports/imp_1/analyze", "",
		`"mapping":[{"index":0,"target":"email"}]`, `"has_header":true`)
	if a.Rows != a.New+a.Existing+a.DuplicatesInFile+a.Invalid+a.Conflicts {
		t.Errorf("buckets do not sum: %+v", a)
	}
	if len(a.InvalidSamples) != 1 || a.InvalidSamples[0].Line != 3 || a.Quality == nil || a.Problem == "" {
		t.Errorf("analysis = %+v", a)
	}
}

func TestContactImportFailuresStreamsCSV(t *testing.T) {
	var got capturedRequest
	csv := "\xef\xbb\xbfEmail,Line,Error\r\nnope,4,invalid email\r\n"
	c := syncServer(t, http.StatusOK, "text/csv; charset=utf-8", csv, &got)

	var buf bytes.Buffer
	resp, err := c.Contacts.ImportFailures(context.Background(), "imp_2", &buf)
	if err != nil {
		t.Fatalf("ImportFailures: %v", err)
	}
	assertRequest(t, got, "GET", "/v1/contacts/imports/imp_2/failed.csv", "")
	if buf.String() != csv {
		t.Errorf("csv = %q, want %q", buf.String(), csv)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("content type = %q", ct)
	}
}

func TestContactImportConflictIsTyped(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusConflict, "application/json",
		`{"error": "Conflict", "code": "conflict", "message": "this import has already started"}`, &got)
	_, _, err := c.Contacts.AnalyzeImport(context.Background(), "imp_1", nil)
	var apiErr *Error
	if !errors.Is(err, ErrConflict) || !errors.As(err, &apiErr) || !apiErr.HasCode(ErrCodeConflict) {
		t.Fatalf("err = %v, want a conflict", err)
	}
	assertRequest(t, got, "POST", "/v1/contacts/imports/imp_1/analyze", "", `"mapping":null`, `"has_header":false`)
}

func TestImportCommitDecodesSegmentsPinned(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusOK, "application/json", `{
  "total": 1, "imported": 1, "updated": 0, "skipped": 0, "failed": 0,
  "started_at": "2026-10-01T09:00:00Z", "ended_at": "2026-10-01T09:00:01Z",
  "segments_pinned": true
}`, &got)
	res, _, err := c.Contacts.ImportCommit(context.Background(), &FileUpload{Filename: "a.csv", Content: strings.NewReader("Email\na@b.co\n")},
		&ContactImportParams{Mapping: []ImportColumnMapping{{Index: 0, Target: ImportTargetEmail}}})
	if err != nil {
		t.Fatalf("ImportCommit: %v", err)
	}
	if res.SegmentsPinned == nil || !*res.SegmentsPinned {
		t.Errorf("segments_pinned = %v", res.SegmentsPinned)
	}
}

// TestContactSelectionRequests covers the select-all shape the bulk routes
// gained, and the sender lookup by thread.
func TestContactSelectionRequests(t *testing.T) {
	var got capturedRequest
	ctx := context.Background()
	filters := &ContactSearchParams{
		ListOptions: ListOptions{Limit: 5, Cursor: "ignored"},
		Query:       "acme",
		MailHosts:   []string{MailHostGoogleWorkspace, ""},
	}

	c := syncServer(t, http.StatusOK, "application/json", `[]`, &got)
	if _, _, err := c.Contacts.BulkUpdate(ctx, &ContactBulkUpdateParams{
		All: true, Filters: filters, Exclude: []string{"ct_9"}, AddCategories: []string{"cat_1"},
	}); err != nil {
		t.Fatalf("BulkUpdate: %v", err)
	}
	assertRequest(t, got, "PATCH", "/v1/contacts", "",
		`"contacts":null`, `"all":true`, `"filters":{"query":"acme","mail_hosts":["google_workspace",""]}`,
		`"exclude":["ct_9"]`, `"add_categories":["cat_1"]`)

	c = syncServer(t, http.StatusNoContent, "", "", &got)
	if _, err := c.Contacts.BulkDeleteSelection(ctx, &ContactSelection{All: true, Filters: filters}); err != nil {
		t.Fatalf("BulkDeleteSelection: %v", err)
	}
	assertRequest(t, got, "DELETE", "/v1/contacts", "", `"all":true`, `"filters":{"query":"acme"`)
	if strings.Contains(got.body, "cursor") || strings.Contains(got.body, "limit") {
		t.Errorf("pagination leaked into the selection: %s", got.body)
	}

	c = syncServer(t, http.StatusOK, "application/json", `{"affected": 3, "action": "verify", "queued": true,
  "verifier": "millionverifier", "verifier_label": "MillionVerifier", "verifier_error": "out of credits"}`, &got)
	res, _, err := c.Contacts.RequestVerification(ctx, &ContactVerificationParams{
		Action: VerificationActionVerify, All: true, Filters: &ContactSearchParams{VerificationStatus: VerifyStatusUnknown},
	})
	if err != nil {
		t.Fatalf("RequestVerification: %v", err)
	}
	assertRequest(t, got, "POST", "/v1/contacts/verification", "", `"all":true`, `"filters":{"verification_status":"unknown"}`)
	if res.Verifier != VerificationProviderMillionVerifier || res.VerifierLabel != "MillionVerifier" || res.VerifierError == "" {
		t.Errorf("result = %+v", res)
	}

	c = syncServer(t, http.StatusOK, "application/json",
		`{"contact": {"id": "ct_1", "email": "ada@example.com", "mail_host": "microsoft365", "esp_provider": "outlook", "updated_at": "2026-10-01T09:00:00Z", "created_at": "2026-10-01T09:00:00Z"}, "match": "thread"}`, &got)
	look, _, err := c.Contacts.LookupSender(ctx, &ContactLookupParams{Email: "Ada <alias@example.com>", ThreadID: "th_1", AccountID: "acc_1"})
	if err != nil {
		t.Fatalf("LookupSender: %v", err)
	}
	assertRequest(t, got, "GET", "/v1/contacts/lookup", "account_id=acc_1&email=Ada+%3Calias%40example.com%3E&thread_id=th_1")
	if look.Match != ContactMatchThread || look.Contact == nil || look.Contact.MailHost != MailHostMicrosoft365 {
		t.Errorf("lookup = %+v", look)
	}

	c = syncServer(t, http.StatusOK, "application/json", `{"contact": null}`, &got)
	look, _, err = c.Contacts.LookupSender(ctx, &ContactLookupParams{Email: "nobody@example.com"})
	if err != nil || look.Contact != nil || look.Match != "" {
		t.Errorf("unmatched lookup = %+v, %v", look, err)
	}

	c = syncServer(t, http.StatusOK, "application/json", `{"id": "ct_1", "updated_at": "2026-10-01T09:00:00Z", "created_at": "2026-10-01T09:00:00Z"}`, &got)
	if _, _, err := c.Contacts.Update(ctx, "ct_1", &ContactUpdateParams{Email: String("new@example.com")}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	assertRequest(t, got, "PATCH", "/v1/contacts/ct_1", "", `"email":"new@example.com"`)
}

// contactDriftJSON is a contact 360 view carrying every field the server
// added since the last sync.
const contactDriftJSON = `{
  "id": "ct_1",
  "email": "ada@example.com",
  "verification_status": "valid",
  "verification_requested_at": "2026-10-02T08:00:00Z",
  "mail_host": "google_workspace",
  "esp_provider": "gmail",
  "campaign_lead": {
    "status": "paused", "sent": 1, "opened": 0, "machine_opened": 0, "clicked": 0, "replied": 0, "bounced": 0,
    "sender": "sales@acme.test",
    "hold": {"since": "2026-10-01T10:00:00Z", "until": "2026-10-08T10:00:00Z", "reason": "back next week", "source": "out_of_office"},
    "cc": [{"contact_id": "ct_2", "email": "bob@example.com", "first_name": "Bob", "last_name": "", "status": "active"}]
  },
  "updated_at": "2026-10-01T09:00:00Z",
  "created_at": "2026-10-01T09:00:00Z",
  "engagement": {
    "total_sent": 2, "total_opened": 1, "total_clicked": 0, "total_replied": 0, "total_bounced": 0, "total_complained": 0,
    "reads_on": [{"client": "Gmail", "client_type": "webmail", "device_hidden": true, "opens": 3, "last_opened_at": "2026-10-01T11:00:00Z"}]
  },
  "verification": {
    "status": "valid", "confidence": 90, "reasons": ["a person replied"], "decisive": true, "evidence": [],
    "source": "provider", "provider": "millionverifier", "provider_label": "MillionVerifier",
    "check_status": "risky", "checked_at": "2026-09-30T08:00:00Z", "requested_at": "2026-10-02T08:00:00Z"
  },
  "source": "crm_sync",
  "source_detail": "HubSpot: Q4 list",
  "first_seen_at": "2026-10-01T09:00:00Z"
}`

func TestContactDecodesDriftFields(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusOK, "application/json", contactDriftJSON, &got)
	d, _, err := c.Contacts.Get(context.Background(), "ct_1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if d.MailHost != MailHostGoogleWorkspace || d.VerificationRequestedAt == nil || d.Source != ContactSourceCRMSync {
		t.Errorf("contact = %+v", d.Contact)
	}
	lead := d.CampaignLead
	if lead == nil || lead.Status != LeadStatusPaused || lead.Sender != "sales@acme.test" {
		t.Fatalf("lead = %+v", lead)
	}
	if lead.Hold == nil || lead.Hold.Source != LeadHoldSourceOutOfOffice || lead.Hold.Until == nil || !lead.Hold.Until.Equal(time.Date(2026, 10, 8, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("hold = %+v", lead.Hold)
	}
	if len(lead.CC) != 1 || lead.CC[0].Status != LeadCCStatusActive || lead.CC[0].BouncedAt != nil {
		t.Errorf("cc = %+v", lead.CC)
	}
	if len(d.Engagement.ReadsOn) != 1 || d.Engagement.ReadsOn[0].ClientType != EngagementClientWebmail || !d.Engagement.ReadsOn[0].DeviceHidden || d.Engagement.ReadsOn[0].Opens != 3 {
		t.Errorf("reads_on = %+v", d.Engagement.ReadsOn)
	}
	v := d.Verification
	if v == nil || v.Provider != VerificationProviderMillionVerifier || v.ProviderLabel != "MillionVerifier" || v.CheckStatus != VerifyStatusRisky || v.CheckedAt == nil || v.RequestedAt == nil {
		t.Errorf("verification = %+v", v)
	}
}

func TestContactCampaignStatesDecodeHoldAndCC(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusOK, "application/json", `{"data": [{
  "campaign_id": "camp_1", "campaign_name": "Q4", "campaign_status": "active", "lead_status": "paused",
  "sender_id": "acc_1", "sender_email": "sales@acme.test",
  "hold": {"since": "2026-10-01T10:00:00Z", "source": "manual"},
  "cc": [],
  "steps": [], "completed_steps": 1, "total_steps": 3,
  "next": {"step_label": "Email 2", "state": "paused", "constraint": "Lead paused"}
}]}`, &got)
	states, _, err := c.Contacts.CampaignStates(context.Background(), "ct_1")
	if err != nil {
		t.Fatalf("CampaignStates: %v", err)
	}
	s := states[0]
	if s.SenderID == nil || *s.SenderID != "acc_1" || s.SenderEmail != "sales@acme.test" {
		t.Errorf("sender = %v %q", s.SenderID, s.SenderEmail)
	}
	if s.Hold == nil || s.Hold.Until != nil || s.Hold.Source != LeadHoldSourceManual || s.CC == nil {
		t.Errorf("hold/cc = %+v %v", s.Hold, s.CC)
	}
}

func TestContactSearchDecodesLeadCountDrift(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusOK, "application/json", `{
  "data": [], "pagination": {"has_more": false, "next_cursor": null, "total": 0},
  "lead_counts": {"total": 10, "queued": 1, "processing": 2, "completed": 1, "replied": 1, "bounced": 0, "failed": 0,
    "unsubscribed": 0, "paused": 3, "undeliverable": 2, "contacted": 7, "opened": 3, "clicked": 1, "replied_any": 1,
    "providers": {"gmail": 4, "outlook": 3, "other": 2, "undetected": 1}}
}`, &got)
	page, err := c.Contacts.Search(context.Background(), &ContactSearchParams{CampaignIDs: []string{"camp_1"}, LeadStatus: LeadStatusPaused})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	assertRequest(t, got, "POST", "/v1/contacts/search", "", `"lead_status":"paused"`)
	lc := page.LeadCounts
	if lc == nil || lc.Paused != 3 || lc.Providers.Google != 4 || lc.Providers.Microsoft != 3 || lc.Providers.Undetected != 1 {
		t.Errorf("lead counts = %+v", lc)
	}
}

func TestContactSentEmailMachineOpen(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusOK, "application/json",
		`{"data": [{"task_id": "t1", "status": "sent", "message_id": "<m@x>", "subject": "Hi", "sent_at": "2026-10-01T09:00:00Z", "machine_opened_at": "2026-10-01T09:00:03Z"}]}`, &got)
	rows, _, err := c.Contacts.Emails(context.Background(), "ct_1")
	if err != nil {
		t.Fatalf("Emails: %v", err)
	}
	if rows[0].OpenedAt != nil || rows[0].MachineOpenedAt == nil {
		t.Errorf("opens = %v %v", rows[0].OpenedAt, rows[0].MachineOpenedAt)
	}
}
