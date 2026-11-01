package warmbly

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestContactImportRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()
	params := &ContactImportParams{
		Mapping:   []ImportColumnMapping{{Index: 0, Target: ImportTargetEmail}},
		Dedup:     ImportDedupUpdate,
		HasHeader: true,
	}
	const paramsJSON = `{"mapping":[{"index":0,"target":"email"}],"dedup":"update","has_header":true}`

	cases := []struct {
		name       string
		call       func() error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string
	}{
		{"ListImports", func() error {
			_, e := c.Contacts.ListImports(ctx, &ListOptions{Limit: 10, Cursor: "c"})
			return e
		}, "GET", "/v1/contacts/imports", "cursor=c&limit=10", ""},
		{"GetImport", func() error { _, _, e := c.Contacts.GetImport(ctx, "ci_1"); return e }, "GET", "/v1/contacts/imports/ci_1", "", ""},
		{"SaveImportDraft", func() error { _, _, e := c.Contacts.SaveImportDraft(ctx, "ci_1", params); return e }, "PATCH", "/v1/contacts/imports/ci_1", "", paramsJSON},
		{"AnalyzeImport", func() error {
			_, _, e := c.Contacts.AnalyzeImport(ctx, "ci_1", &ContactImportAnalyzeParams{Mapping: params.Mapping, HasHeader: true})
			return e
		}, "POST", "/v1/contacts/imports/ci_1/analyze", "", `{"mapping":[{"index":0,"target":"email"}],"has_header":true}`},
		{"StartImport", func() error { _, _, e := c.Contacts.StartImport(ctx, "ci_1", params); return e }, "POST", "/v1/contacts/imports/ci_1/start", "", paramsJSON},
		{"CancelImport", func() error { _, _, e := c.Contacts.CancelImport(ctx, "ci_1"); return e }, "POST", "/v1/contacts/imports/ci_1/cancel", "", ""},
		{"DownloadImportFailures", func() error {
			var buf bytes.Buffer
			_, e := c.Contacts.DownloadImportFailures(ctx, "ci_1", &buf)
			return e
		}, "GET", "/v1/contacts/imports/ci_1/failed.csv", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got = recordedRequest{}
			if err := tc.call(); err != nil {
				t.Fatalf("call: %v", err)
			}
			if got.method != tc.wantMethod || got.path != tc.wantPath || got.rawQuery != tc.wantQuery {
				t.Errorf("got %s %s?%s, want %s %s?%s", got.method, got.path, got.rawQuery, tc.wantMethod, tc.wantPath, tc.wantQuery)
			}
			if strings.TrimSpace(got.body) != tc.wantBody {
				t.Errorf("body = %q, want %q", got.body, tc.wantBody)
			}
		})
	}

	// Options are required on the calls that save or start.
	if _, _, err := c.Contacts.StartImport(ctx, "ci_1", nil); err == nil {
		t.Error("StartImport with nil options succeeded")
	}
	if _, _, err := c.Contacts.SaveImportDraft(ctx, "ci_1", nil); err == nil {
		t.Error("SaveImportDraft with nil options succeeded")
	}
}

func TestContactImportCreateAndDecode(t *testing.T) {
	var gotCT, gotPath, gotBody string
	srv := newCaptureServer(t, func(r *http.Request, body string) {
		gotCT, gotPath, gotBody = r.Header.Get("Content-Type"), r.URL.Path, body
	}, http.StatusCreated, `{
		"id": "0c0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b",
		"organization_id": "f9e8d7c6-0000-0000-0000-000000000000",
		"created_by": "a1b2c3d4-0000-0000-0000-000000000000",
		"filename": "leads.csv",
		"format": "csv",
		"status": "draft",
		"has_header": true,
		"columns": ["Email", "First name"],
		"total": 2,
		"processed": 0, "imported": 0, "updated": 0, "skipped": 0, "failed": 0,
		"options": {"mapping": [{"index": 0, "target": "email"}], "dedup": "skip", "has_header": true, "segment_ids": ["s1"]},
		"notes": [],
		"created_at": "2026-09-10T08:00:00Z",
		"updated_at": "2026-09-10T08:00:00Z",
		"preview": {
			"filename": "leads.csv", "format": "csv", "total_rows": 2,
			"columns": ["Email", "First name"], "has_header": true,
			"sample_rows": [["a@acme.io", "A"]],
			"suggested_mapping": [{"index": 0, "target": "email"}, {"index": 1, "target": "first_name"}],
			"inferred_columns": [1],
			"column_stats": [{"filled": 2, "distinct": 2, "samples": ["a@acme.io"]}, {"filled": 1, "distinct": 1, "samples": ["A"]}],
			"mapping_source": "saved"
		}
	}`)
	c := newClientFor(t, srv)

	imp, resp, err := c.Contacts.CreateImport(context.Background(), &FileUpload{Filename: "leads.csv", Content: strings.NewReader("Email,First name\na@acme.io,A\n")})
	if err != nil {
		t.Fatalf("CreateImport: %v", err)
	}
	if resp.StatusCode != http.StatusCreated || gotPath != "/v1/contacts/imports" {
		t.Errorf("got %d %s", resp.StatusCode, gotPath)
	}
	_, field, name := parseForm(t, gotCT, gotBody)
	if field != "file" || name != "leads.csv" {
		t.Errorf("file part = %q %q", field, name)
	}
	if imp.Status != ContactImportStatusDraft || imp.Finished() || imp.Total != 2 || imp.CreatedBy == nil {
		t.Errorf("import = %+v", imp)
	}
	if imp.Options == nil || imp.Options.Dedup != ImportDedupSkip || len(imp.Options.SegmentIDs) != 1 {
		t.Errorf("options = %+v", imp.Options)
	}
	p := imp.Preview
	if p == nil || p.MappingSource != "saved" || len(p.InferredColumns) != 1 || p.InferredColumns[0] != 1 || len(p.ColumnStats) != 2 || p.ColumnStats[0].Filled != 2 {
		t.Errorf("preview = %+v", p)
	}
}

func TestContactImportFinishedDecode(t *testing.T) {
	const body = `{
		"id": "0c0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b",
		"organization_id": "f9e8d7c6-0000-0000-0000-000000000000",
		"filename": "leads.csv", "format": "csv", "status": "a_future_status", "has_header": true, "columns": [],
		"total": 100, "processed": 100, "imported": 90, "updated": 4, "skipped": 3, "failed": 3,
		"quality": {"malformed": 2, "disposable": 1, "role": 4, "bad_share_pct": 3, "flagged": false},
		"segments_pinned": false,
		"notes": ["A segment could not be written."],
		"error": "",
		"created_at": "2026-09-10T08:00:00Z", "updated_at": "2026-09-10T08:05:00Z",
		"started_at": "2026-09-10T08:01:00Z", "finished_at": "2026-09-10T08:05:00Z",
		"failures": [{"line": 5, "email": "bad", "reason": "Not an email address."}]
	}`
	var got recordedRequest
	c := emailFixtureClient(t, http.StatusOK, body, &got)
	imp, _, err := c.Contacts.GetImport(context.Background(), "ci_1")
	if err != nil {
		t.Fatalf("GetImport: %v", err)
	}
	if imp.Status != "a_future_status" || imp.Finished() {
		t.Errorf("unknown status handling: %+v", imp)
	}
	if imp.SegmentsPinned == nil || *imp.SegmentsPinned || len(imp.Notes) != 1 || imp.FinishedAt == nil || imp.Quality == nil || imp.Quality.Role != 4 {
		t.Errorf("import = %+v", imp)
	}
	if len(imp.Failures) != 1 || imp.Failures[0].Line != 5 || imp.Preview != nil || imp.Options != nil {
		t.Errorf("failures = %+v", imp.Failures)
	}

	c = emailFixtureClient(t, http.StatusOK, `{"id":"x","status":"completed","columns":[],"notes":[]}`, &got)
	done, _, err := c.Contacts.CancelImport(context.Background(), "x")
	if err != nil || !done.Finished() {
		t.Errorf("completed import = %+v %v", done, err)
	}
}

func TestContactImportListDecode(t *testing.T) {
	// The list's pagination carries no total, and a single page has no cursor.
	var got recordedRequest
	c := emailFixtureClient(t, http.StatusOK, `{
		"data": [{"id": "ci_1", "status": "running", "columns": [], "notes": []}],
		"pagination": {"total": null, "next_cursor": "cur", "has_more": true}
	}`, &got)
	page, err := c.Contacts.ListImports(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListImports: %v", err)
	}
	if len(page.Data) != 1 || !page.HasMore() || page.NextCursor() != "cur" || page.Pagination.Total != nil {
		t.Errorf("page = %+v", page.Pagination)
	}

	c = emailFixtureClient(t, http.StatusOK, `{"data":[],"pagination":{"total":null,"next_cursor":null,"has_more":false}}`, &got)
	last, err := c.Contacts.ListImports(context.Background(), nil)
	if err != nil || last.HasMore() {
		t.Errorf("last page = %+v %v", last, err)
	}
}

func TestContactImportAnalysisDecode(t *testing.T) {
	var got recordedRequest
	c := emailFixtureClient(t, http.StatusOK, `{
		"rows": 100, "new": 80, "existing": 10, "duplicates_in_file": 4, "invalid": 5, "conflicts": 1,
		"invalid_samples": [{"line": 7, "email": "x@", "values": ["x@", "X"], "reason": "Not an email address."}],
		"quality": {"malformed": 5, "disposable": 0, "role": 2, "bad_share_pct": 5, "flagged": false, "summary": ""},
		"problem": "This import would exceed your contact limit."
	}`, &got)
	a, _, err := c.Contacts.AnalyzeImport(context.Background(), "ci_1", &ContactImportAnalyzeParams{HasHeader: true})
	if err != nil {
		t.Fatalf("AnalyzeImport: %v", err)
	}
	if a.Rows != 100 || a.New != 80 || a.Existing != 10 || a.DuplicatesInFile != 4 || a.Invalid != 5 || a.Conflicts != 1 {
		t.Errorf("analysis = %+v", a)
	}
	if len(a.InvalidSamples) != 1 || a.InvalidSamples[0].Line != 7 || a.Quality == nil || a.Problem == "" {
		t.Errorf("samples = %+v", a)
	}
}

func TestContactDecodeSecondSyncFields(t *testing.T) {
	const body = `{
		"id": "0c0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b",
		"email": "a@acme.io",
		"verification_requested_at": "2026-09-10T08:00:00Z",
		"mail_host": "microsoft365",
		"outreach": {"sent": 5, "opened": 3, "clicked": 1, "replied": 1, "bounced": 0, "last_sent_at": "2026-09-09T08:00:00Z"},
		"campaign_lead": {"status": "active", "sent": 1, "opened": 0, "machine_opened": 0, "clicked": 0, "replied": 0, "bounced": 0,
			"sender": "sales@acme.io",
			"hold": {"since": "2026-09-09T08:00:00Z", "until": null, "reason": "Out of office", "source": "out_of_office"},
			"cc": [{"contact_id": "5e6f7a8b-9c0d-1e2f-3a4b-5c6d7e8f9a0b", "email": "b@acme.io", "first_name": "B", "last_name": "C", "status": "bounced", "bounced_at": "2026-09-09T09:00:00Z"}]}
	}`
	var got recordedRequest
	c := emailFixtureClient(t, http.StatusOK, body, &got)
	ct, _, err := c.Contacts.Get(context.Background(), "ct_1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ct.MailHost != "microsoft365" || ct.VerificationRequestedAt == nil || ct.Outreach == nil || ct.Outreach.Sent != 5 || ct.Outreach.LastSentAt == nil {
		t.Errorf("contact = %+v", ct)
	}
	cl := ct.CampaignLead
	if cl == nil || cl.Sender != "sales@acme.io" || cl.Hold == nil || cl.Hold.Until != nil || cl.Hold.Source != "out_of_office" || len(cl.CC) != 1 || cl.CC[0].Status != "bounced" || cl.CC[0].BouncedAt == nil {
		t.Errorf("campaign lead = %+v", cl)
	}

	_, _, err = c.Contacts.Update(context.Background(), "ct_1", &ContactUpdateParams{Email: String("new@acme.io")})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !strings.Contains(got.body, `"email":"new@acme.io"`) {
		t.Errorf("update body = %q", got.body)
	}

	_, err = c.Contacts.Search(context.Background(), &ContactSearchParams{MailHosts: []string{"google_workspace", ""}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !strings.Contains(got.body, `"mail_hosts":["google_workspace",""]`) {
		t.Errorf("search body = %q", got.body)
	}
}
