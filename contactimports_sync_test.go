package warmbly

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func sampleContactImport() ContactImport {
	started := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	creator := "usr_1"
	pinned := true
	return ContactImport{
		ID:             "imp_1",
		OrganizationID: "org_1",
		CreatedBy:      &creator,
		Filename:       "leads.csv",
		Format:         "csv",
		Status:         ContactImportStatusRunning,
		HasHeader:      true,
		Columns:        []string{"Email", "Name"},
		Total:          120,
		Processed:      40,
		Imported:       30,
		Updated:        5,
		Skipped:        3,
		Failed:         2,
		Options: &ContactImportParams{
			Mapping:   []ImportColumnMapping{{Index: 0, Target: ImportTargetEmail}},
			Dedup:     ImportDedupUpdate,
			HasHeader: true,
		},
		Quality:        &ContactImportQuality{Malformed: 2, BadSharePct: 1.6},
		SegmentsPinned: &pinned,
		Notes:          []string{"2 rows skipped"},
		CreatedAt:      started,
		UpdatedAt:      started,
		StartedAt:      &started,
		Preview: &ContactImportPreview{
			Filename:         "leads.csv",
			Format:           "csv",
			TotalRows:        120,
			Columns:          []string{"Email", "Name"},
			HasHeader:        true,
			SampleRows:       [][]string{{"a@b.com", "Ada"}},
			SuggestedMapping: []ImportColumnMapping{{Index: 0, Target: ImportTargetEmail}},
			InferredColumns:  []int{1},
			ColumnStats:      []ContactImportColumnStats{{Filled: 120, Distinct: 118, Samples: []string{"a@b.com"}}},
			MappingSource:    ContactImportMappingSaved,
		},
		Failures: []ImportRowError{{Line: 7, Email: "nope", Reason: "invalid email"}},
	}
}

func TestContactImportLifecycleRouting(t *testing.T) {
	ctx := context.Background()
	imp := sampleContactImport()
	body := jsonOf(t, imp)
	params := &ContactImportParams{
		Mapping:     []ImportColumnMapping{{Index: 0, Target: ImportTargetEmail}, {Index: 1, Target: ImportTargetCustom, CustomKey: "role"}},
		Dedup:       ImportDedupSkip,
		HasHeader:   true,
		SegmentIDs:  []string{"seg_1"},
		CategoryIDs: []string{"cat_1"},
	}
	wantParams := `{"mapping":[{"index":0,"target":"email"},{"index":1,"target":"custom","custom_key":"role"}],"dedup":"skip","has_header":true,"category_ids":["cat_1"],"segment_ids":["seg_1"]}`

	cases := []struct {
		name       string
		status     int
		call       func(c *Client) (*ContactImport, error)
		wantMethod string
		wantPath   string
		wantBody   string
	}{
		{"GetImport", 200, func(c *Client) (*ContactImport, error) {
			v, _, e := c.Contacts.GetImport(ctx, "imp_1")
			return v, e
		}, "GET", "/v1/contacts/imports/imp_1", ""},
		{"SaveImportDraft", 200, func(c *Client) (*ContactImport, error) {
			v, _, e := c.Contacts.SaveImportDraft(ctx, "imp_1", params)
			return v, e
		}, "PATCH", "/v1/contacts/imports/imp_1", wantParams},
		{"StartImport", 200, func(c *Client) (*ContactImport, error) {
			v, _, e := c.Contacts.StartImport(ctx, "imp_1", params)
			return v, e
		}, "POST", "/v1/contacts/imports/imp_1/start", wantParams},
		{"CancelImport", 200, func(c *Client) (*ContactImport, error) {
			v, _, e := c.Contacts.CancelImport(ctx, "imp_1")
			return v, e
		}, "POST", "/v1/contacts/imports/imp_1/cancel", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got syncCapture
			c := respondingClient(t, &got, tc.status, body)
			v, err := tc.call(c)
			if err != nil {
				t.Fatal(err)
			}
			got.wantRequest(t, tc.wantMethod, tc.wantPath, "")
			got.wantBody(t, tc.wantBody)
			if v.ID != "imp_1" || v.Status != ContactImportStatusRunning || v.Terminal() {
				t.Errorf("import = %+v", v)
			}
			if v.Preview == nil || v.Preview.MappingSource != ContactImportMappingSaved ||
				len(v.Preview.ColumnStats) != 1 || v.Preview.InferredColumns[0] != 1 {
				t.Errorf("preview = %+v", v.Preview)
			}
			if v.Options == nil || v.Options.Dedup != ImportDedupUpdate || v.SegmentsPinned == nil || !*v.SegmentsPinned {
				t.Errorf("options = %+v", v.Options)
			}
			if len(v.Failures) != 1 || v.Failures[0].Line != 7 || v.StartedAt == nil || v.FinishedAt != nil {
				t.Errorf("failures/timestamps = %+v", v)
			}
		})
	}
}

func TestContactImportUnknownStatusDecodes(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, `{"id":"imp_1","status":"archiving","columns":[],"notes":[]}`)
	imp, _, err := c.Contacts.GetImport(context.Background(), "imp_1")
	if err != nil {
		t.Fatalf("an unknown status must decode: %v", err)
	}
	if imp.Status != "archiving" || imp.Terminal() {
		t.Errorf("status = %q terminal=%v", imp.Status, imp.Terminal())
	}
	for _, s := range []string{ContactImportStatusCompleted, ContactImportStatusFailed, ContactImportStatusCancelled} {
		if !(&ContactImport{Status: s}).Terminal() {
			t.Errorf("%s should be terminal", s)
		}
	}
}

func TestContactImportCreateUploadsFileField(t *testing.T) {
	var got syncCapture
	imp := sampleContactImport()
	imp.Status = ContactImportStatusDraft
	c := respondingClient(t, &got, http.StatusCreated, jsonOf(t, imp))
	out, resp, err := c.Contacts.CreateImport(context.Background(), &FileUpload{
		Filename: "leads.csv",
		Content:  strings.NewReader("email,name\na@b.com,Ada\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "POST", "/v1/contacts/imports", "")
	_, files := got.multipartParts(t)
	if files["file"] != "leads.csv|email,name\na@b.com,Ada\n" {
		t.Errorf("multipart file part = %q (server reads field %q)", files["file"], "file")
	}
	if resp.StatusCode != http.StatusCreated || out.Status != ContactImportStatusDraft || out.Preview == nil {
		t.Errorf("response = %d %+v", resp.StatusCode, out)
	}
	if _, _, err := c.Contacts.CreateImport(context.Background(), nil); err == nil {
		t.Error("a nil upload must fail before any request")
	}
}

func TestContactImportListPaginates(t *testing.T) {
	var got syncCapture
	next := "cur_2"
	body := jsonOf(t, map[string]any{
		"data":       []ContactImport{sampleContactImport()},
		"pagination": Pagination{HasMore: true, NextCursor: &next},
	})
	c := respondingClient(t, &got, 200, body)
	page, err := c.Contacts.ListImports(context.Background(), &ListOptions{Limit: 25, Cursor: "cur_1"})
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "GET", "/v1/contacts/imports", "cursor=cur_1&limit=25")
	if len(page.Data) != 1 || page.Data[0].Filename != "leads.csv" || !page.HasMore() || page.NextCursor() != "cur_2" {
		t.Errorf("page = %+v", page)
	}
	if _, err := c.Contacts.ListImports(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "GET", "/v1/contacts/imports", "")
}

func TestContactImportAnalyze(t *testing.T) {
	var got syncCapture
	analysis := ContactImportAnalysis{
		Rows: 100, New: 80, Existing: 10, DuplicatesInFile: 4, Invalid: 5, Conflicts: 1,
		InvalidSamples: []ImportRowError{{Line: 3, Email: "x", Reason: "invalid email"}},
		Quality:        &ContactImportQuality{Malformed: 5},
		Problem:        "this import would pass your plan's contact limit",
	}
	c := respondingClient(t, &got, 200, jsonOf(t, analysis))
	out, _, err := c.Contacts.AnalyzeImport(context.Background(), "imp_1", &ContactImportAnalyzeParams{
		Mapping:   []ImportColumnMapping{{Index: 0, Target: ImportTargetEmail}},
		HasHeader: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "POST", "/v1/contacts/imports/imp_1/analyze", "")
	got.wantBody(t, `{"mapping":[{"index":0,"target":"email"}],"has_header":true}`)
	if out.New != 80 || out.Conflicts != 1 || out.Problem == "" || len(out.InvalidSamples) != 1 || out.Quality.Malformed != 5 {
		t.Errorf("analysis = %+v", out)
	}
}

func TestContactImportDownloadFailures(t *testing.T) {
	var got syncCapture
	csv := "\ufeffEmail,Name,Line,Error\nnope,Ada,7,invalid email\n"
	c := respondingClientType(t, &got, 200, "text/csv; charset=utf-8", csv)
	var buf bytes.Buffer
	resp, err := c.Contacts.DownloadImportFailures(context.Background(), "imp_1", &buf)
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "GET", "/v1/contacts/imports/imp_1/failed.csv", "")
	if buf.String() != csv {
		t.Errorf("downloaded %q, want %q", buf.String(), csv)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
	if _, err := c.Contacts.DownloadImportFailures(context.Background(), "imp_1", nil); err == nil {
		t.Error("a nil writer must fail before any request")
	}
}

func TestContactLookupSender(t *testing.T) {
	var got syncCapture
	contact := Contact{ID: "ct_1", Email: "ada@acme.com", MailHost: MailHostGoogleWorkspace}
	body := jsonOf(t, ContactLookup{Contact: &contact, Match: ContactLookupMatchThread})
	c := respondingClient(t, &got, 200, body)
	out, _, err := c.Contacts.LookupSender(context.Background(), &ContactLookupParams{
		Email:     "Ada <ada@alias.io>",
		ThreadID:  "th_1",
		AccountID: "acc_1",
	})
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "GET", "/v1/contacts/lookup", "account_id=acc_1&email=Ada+%3Cada%40alias.io%3E&thread_id=th_1")
	if out.Contact == nil || out.Contact.ID != "ct_1" || out.Match != ContactLookupMatchThread || out.Contact.MailHost != MailHostGoogleWorkspace {
		t.Errorf("lookup = %+v", out)
	}

	c = respondingClient(t, &got, 200, `{"contact":null}`)
	out, _, err = c.Contacts.LookupSender(context.Background(), &ContactLookupParams{ThreadID: "th_9"})
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "GET", "/v1/contacts/lookup", "thread_id=th_9")
	if out.Contact != nil || out.Match != "" {
		t.Errorf("no match must decode as a nil contact, got %+v", out)
	}

	// A match kind this release does not know still decodes.
	c = respondingClient(t, &got, 200, `{"contact":{"id":"ct_2"},"match":"alias"}`)
	out, _, err = c.Contacts.LookupSender(context.Background(), &ContactLookupParams{Email: "x@y.z"})
	if err != nil || out.Match != "alias" {
		t.Errorf("unknown match = %+v, %v", out, err)
	}
}

func TestContactBulkSelectionBodies(t *testing.T) {
	ctx := context.Background()
	filters := &ContactSearchParams{Query: "acme", MailHosts: []string{MailHostGmail, ""}, SortBy: "custom:Industry", Reverse: true}
	const wantFilters = `{"query":"acme","mail_hosts":["gmail",""],"sort_by":"custom:Industry","reverse":true}`

	t.Run("BulkUpdate select all", func(t *testing.T) {
		var got syncCapture
		c := respondingClient(t, &got, 200, `[]`)
		out, _, err := c.Contacts.BulkUpdate(ctx, &ContactBulkUpdateParams{
			All: true, Filters: filters, Exclude: []string{"ct_9"}, Subscribe: Bool(false),
		})
		if err != nil {
			t.Fatal(err)
		}
		got.wantRequest(t, "PATCH", "/v1/contacts", "")
		got.wantBody(t, `{"all":true,"filters":`+wantFilters+`,"exclude":["ct_9"],"subscribe":false}`)
		if out == nil || len(out) != 0 {
			t.Errorf("a select-all answers an empty list, got %v", out)
		}
	})
	t.Run("BulkUpdate ids", func(t *testing.T) {
		var got syncCapture
		c := respondingClient(t, &got, 200, `[]`)
		if _, _, err := c.Contacts.BulkUpdate(ctx, &ContactBulkUpdateParams{Contacts: []string{"ct_1"}, AddCategories: []string{"cat_1"}}); err != nil {
			t.Fatal(err)
		}
		got.wantBody(t, `{"contacts":["ct_1"],"add_categories":["cat_1"]}`)
	})
	t.Run("RequestVerification select all", func(t *testing.T) {
		var got syncCapture
		resp := ContactVerificationResult{Affected: 3, Action: VerificationActionVerify, Queued: true, Verifier: "builtin", VerifierError: "key rejected"}
		c := respondingClient(t, &got, 200, jsonOf(t, resp))
		out, _, err := c.Contacts.RequestVerification(ctx, &ContactVerificationParams{
			Action: VerificationActionVerify, All: true, Filters: filters,
		})
		if err != nil {
			t.Fatal(err)
		}
		got.wantRequest(t, "POST", "/v1/contacts/verification", "")
		got.wantBody(t, `{"action":"verify","all":true,"filters":`+wantFilters+`}`)
		if !out.Queued || out.Verifier != "builtin" || out.VerifierError != "key rejected" {
			t.Errorf("result = %+v", out)
		}
	})
	t.Run("BulkDeleteSelection", func(t *testing.T) {
		var got syncCapture
		c := respondingClient(t, &got, http.StatusNoContent, "")
		if _, err := c.Contacts.BulkDeleteSelection(ctx, &ContactSelection{All: true, Filters: filters, Exclude: []string{"ct_9"}}); err != nil {
			t.Fatal(err)
		}
		got.wantRequest(t, "DELETE", "/v1/contacts", "")
		got.wantBody(t, `{"all":true,"filters":`+wantFilters+`,"exclude":["ct_9"]}`)
	})
	t.Run("BulkDelete ids stay a bare array", func(t *testing.T) {
		var got syncCapture
		c := respondingClient(t, &got, http.StatusNoContent, "")
		if _, err := c.Contacts.BulkDelete(ctx, []string{"ct_1", "ct_2"}); err != nil {
			t.Fatal(err)
		}
		got.wantBody(t, `["ct_1","ct_2"]`)
	})
	t.Run("Push selection", func(t *testing.T) {
		var got syncCapture
		c := respondingClient(t, &got, 200, `{"provider":"hubspot","pushed":1,"failed":0,"results":[]}`)
		out, _, err := c.Integrations.PushSelection(ctx, "conn_1", &ContactSelection{All: true, Filters: filters})
		if err != nil {
			t.Fatal(err)
		}
		got.wantRequest(t, "POST", "/v1/integrations/connections/conn_1/push", "")
		got.wantBody(t, `{"all":true,"filters":`+wantFilters+`}`)
		if out.Pushed != 1 {
			t.Errorf("push = %+v", out)
		}
	})
}

func TestContactUpdateCarriesEmail(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, jsonOf(t, Contact{ID: "ct_1", Email: "new@acme.com"}))
	out, _, err := c.Contacts.Update(context.Background(), "ct_1", &ContactUpdateParams{Email: String("new@acme.com")})
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "PATCH", "/v1/contacts/ct_1", "")
	got.wantBody(t, `{"email":"new@acme.com"}`)
	if out.Email != "new@acme.com" {
		t.Errorf("contact = %+v", out)
	}
}

func TestContactDetailDecodesNewFields(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	detail := ContactDetail{
		Contact: Contact{
			ID: "ct_1", Email: "ada@acme.com", MailHost: MailHostMicrosoft365, ESPProvider: "outlook",
			VerificationRequestedAt: &now,
			CampaignLead: &ContactCampaignProgress{
				Status: LeadStatusPaused, Sender: "me@mine.io",
				Hold: &LeadHold{Since: now, Source: LeadHoldOutOfOffice},
				CC:   []CampaignLeadCC{{ContactID: "ct_2", Email: "bo@acme.com", Status: "active"}},
			},
		},
		Engagement: ContactEngagement{ReadsOn: []ContactReadingOrigin{{
			Client: "Gmail", ClientType: EngagementClientWebmail, DeviceHidden: true, Opens: 3, LastOpenedAt: now,
		}}},
		Verification: &ContactVerificationDetail{
			Status: VerifyStatusValid, Source: VerificationSourceProvider, Provider: VerificationProviderCleanMyList,
			ProviderLabel: "CleanMyList", CheckStatus: "ok", CheckedAt: &now, RequestedAt: &now,
		},
	}
	var got syncCapture
	c := respondingClient(t, &got, 200, jsonOf(t, detail))
	out, _, err := c.Contacts.Get(context.Background(), "ct_1")
	if err != nil {
		t.Fatal(err)
	}
	if out.MailHost != MailHostMicrosoft365 || out.VerificationRequestedAt == nil {
		t.Errorf("contact = %+v", out.Contact)
	}
	lead := out.CampaignLead
	if lead == nil || lead.Status != LeadStatusPaused || lead.Hold == nil || lead.Hold.Source != LeadHoldOutOfOffice ||
		lead.Hold.Until != nil || len(lead.CC) != 1 || lead.Sender != "me@mine.io" {
		t.Errorf("lead = %+v", lead)
	}
	if len(out.Engagement.ReadsOn) != 1 || !out.Engagement.ReadsOn[0].DeviceHidden || out.Engagement.ReadsOn[0].Opens != 3 {
		t.Errorf("reads_on = %+v", out.Engagement.ReadsOn)
	}
	if v := out.Verification; v == nil || v.Provider != VerificationProviderCleanMyList || v.CheckedAt == nil || v.ProviderLabel != "CleanMyList" {
		t.Errorf("verification = %+v", v)
	}
}

func TestContactImportQueryPreviewKeepsFileField(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, jsonOf(t, ContactImportPreview{Filename: "a.csv", Columns: []string{"Email"}}))
	if _, _, err := c.Contacts.ImportPreview(context.Background(), &FileUpload{Filename: "a.csv", Content: strings.NewReader("Email\n")}); err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "POST", "/v1/contacts/import/preview", "")
	if _, files := got.multipartParts(t); files["file"] == "" {
		t.Errorf("file part missing: %v", files)
	}
}
