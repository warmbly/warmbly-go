package warmbly

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestContactImportsSyncRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()
	mapping := []ImportColumnMapping{{Index: 0, Target: ImportTargetEmail}}

	cases := []struct {
		name       string
		call       func() error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   []string
	}{
		{"CreateImport", func() error {
			_, _, e := c.Contacts.CreateImport(ctx, &FileUpload{Filename: "leads.csv", Content: strings.NewReader("email\na@b.co\n")})
			return e
		}, "POST", "/v1/contacts/imports", "", []string{`name="file"`, `filename="leads.csv"`, "a@b.co"}},
		{"ListImports", func() error {
			_, e := c.Contacts.ListImports(ctx, &ListOptions{Limit: 10, Cursor: "cur"})
			return e
		}, "GET", "/v1/contacts/imports", "cursor=cur&limit=10", nil},
		{"GetImport", func() error { _, _, e := c.Contacts.GetImport(ctx, "imp_1"); return e },
			"GET", "/v1/contacts/imports/imp_1", "", nil},
		{"SaveImportDraft", func() error {
			_, _, e := c.Contacts.SaveImportDraft(ctx, "imp_1", &ContactImportParams{Mapping: mapping, HasHeader: true, SkipActiveInOtherCampaigns: true})
			return e
		}, "PATCH", "/v1/contacts/imports/imp_1", "", []string{`"has_header":true`, `"skip_active_in_other_campaigns":true`, `"target":"email"`}},
		{"AnalyzeImport", func() error {
			_, _, e := c.Contacts.AnalyzeImport(ctx, "imp_1", &ContactImportAnalyzeParams{Mapping: mapping, HasHeader: true})
			return e
		}, "POST", "/v1/contacts/imports/imp_1/analyze", "", []string{`"mapping":[{"index":0,"target":"email"}]`, `"has_header":true`}},
		{"StartImport", func() error {
			_, _, e := c.Contacts.StartImport(ctx, "imp_1", &ContactImportParams{Mapping: mapping, Dedup: ImportDedupUpdate})
			return e
		}, "POST", "/v1/contacts/imports/imp_1/start", "", []string{`"dedup":"update"`}},
		{"CancelImport", func() error { _, _, e := c.Contacts.CancelImport(ctx, "imp_1"); return e },
			"POST", "/v1/contacts/imports/imp_1/cancel", "", nil},
		{"DownloadImportFailures", func() error {
			var buf bytes.Buffer
			_, e := c.Contacts.DownloadImportFailures(ctx, "imp_1", &buf)
			return e
		}, "GET", "/v1/contacts/imports/imp_1/failed.csv", "", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got = recordedRequest{}
			if err := tc.call(); err != nil {
				t.Fatalf("call: %v", err)
			}
			if got.method != tc.wantMethod || got.path != tc.wantPath {
				t.Fatalf("got %s %s, want %s %s", got.method, got.path, tc.wantMethod, tc.wantPath)
			}
			if got.rawQuery != tc.wantQuery {
				t.Fatalf("query = %q, want %q", got.rawQuery, tc.wantQuery)
			}
			for _, frag := range tc.wantBody {
				if !strings.Contains(got.body, frag) {
					t.Errorf("body %s missing %s", got.body, frag)
				}
			}
		})
	}
}

func TestContactImportDecodes(t *testing.T) {
	var got recordedRequest
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	creator := "usr_1"
	pinned := false
	want := ContactImport{
		ID: "imp_1", OrganizationID: "org_1", CreatedBy: &creator, Filename: "leads.csv", Format: "csv",
		Status: ImportStatusDraft, HasHeader: true, Columns: []string{"Email", "Name"},
		Total: 2, Notes: []string{},
		Options:        &ContactImportParams{Mapping: []ImportColumnMapping{{Index: 0, Target: ImportTargetEmail}}, Dedup: ImportDedupSkip, HasHeader: true},
		SegmentsPinned: &pinned,
		CreatedAt:      now, UpdatedAt: now,
		Preview: &ContactImportPreview{
			Filename: "leads.csv", Format: "csv", TotalRows: 2, Columns: []string{"Email", "Name"}, HasHeader: true,
			SampleRows: [][]string{{"a@b.co", "A"}}, SuggestedMapping: []ImportColumnMapping{{Index: 0, Target: ImportTargetEmail}},
			InferredColumns: []int{1},
			ColumnStats:     []ContactImportColumnStats{{Filled: 2, Distinct: 2, Samples: []string{"a@b.co"}}},
			MappingSource:   ImportMappingSaved,
		},
	}
	c := crmServe(t, &got, 201, want)
	out, resp, err := c.Contacts.CreateImport(context.Background(), &FileUpload{Filename: "leads.csv", Content: strings.NewReader("x")})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 || !reflect.DeepEqual(*out, want) {
		t.Fatalf("decoded %+v, want %+v", *out, want)
	}
	if ImportTerminal(out.Status) {
		t.Fatal("a draft is not terminal")
	}
	for _, s := range []string{ImportStatusCompleted, ImportStatusFailed, ImportStatusCancelled} {
		if !ImportTerminal(s) {
			t.Fatalf("%s should be terminal", s)
		}
	}
}

func TestContactImportAnalysisDecodes(t *testing.T) {
	var got recordedRequest
	want := ContactImportAnalysis{
		Rows: 10, New: 6, Existing: 2, DuplicatesInFile: 1, Invalid: 1,
		InvalidSamples: []ImportRowError{{Line: 4, Email: "nope", Reason: "invalid address"}},
		Problem:        "The plan allows 5 more contacts.",
	}
	c := crmServe(t, &got, 200, want)
	out, _, err := c.Contacts.AnalyzeImport(context.Background(), "imp_1", &ContactImportAnalyzeParams{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*out, want) {
		t.Fatalf("decoded %+v, want %+v", *out, want)
	}
}

func TestContactImportListPages(t *testing.T) {
	var got recordedRequest
	next := "c2"
	c := crmServe(t, &got, 200, Page[ContactImport]{
		Data:       []ContactImport{{ID: "imp_1", Status: ImportStatusRunning}},
		Pagination: Pagination{NextCursor: &next, HasMore: true},
	})
	page, err := c.Contacts.ListImports(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 1 || page.Data[0].Status != ImportStatusRunning || !page.HasMore() {
		t.Fatalf("page = %+v", page)
	}
}

func TestDownloadImportFailuresStreamsCSV(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		_, _ = w.Write([]byte("line,email,reason\n3,bad,invalid\n"))
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := c.Contacts.DownloadImportFailures(context.Background(), "imp_1", &buf); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "line,email,reason\n3,bad,invalid\n" {
		t.Fatalf("csv = %q", buf.String())
	}
}

func TestContactImportResultSegmentsPinned(t *testing.T) {
	var got recordedRequest
	pinned := false
	c := crmServe(t, &got, 200, ContactImportResult{Total: 1, Imported: 1, SegmentsPinned: &pinned})
	out, _, err := c.Contacts.ImportCommit(context.Background(), &FileUpload{Filename: "a.csv", Content: strings.NewReader("x")}, &ContactImportParams{})
	if err != nil {
		t.Fatal(err)
	}
	if out.SegmentsPinned == nil || *out.SegmentsPinned {
		t.Fatalf("SegmentsPinned = %v, want pointer to false", out.SegmentsPinned)
	}
}
