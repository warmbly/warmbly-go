package warmbly

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

// crmServe answers every request with status and the JSON encoding of v, and
// records the request it saw.
func crmServe(t *testing.T, got *recordedRequest, status int, v any) *Client {
	t.Helper()
	payload, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.path = r.URL.Path
		got.rawQuery = r.URL.RawQuery
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r.Body)
		got.body = buf.String()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(payload)
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestCRMProviderRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()
	user := "usr_1"

	cases := []struct {
		name       string
		call       func() error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   []string
	}{
		{"Settings", func() error { _, _, e := c.CRM.Settings(ctx); return e }, "GET", "/v1/crm/settings", "", nil},
		{"UpdateSettings", func() error {
			_, _, e := c.CRM.UpdateSettings(ctx, &CRMSettingsUpdateParams{Provider: String(CRMProviderHubSpot), CompleteSetup: true})
			return e
		}, "PUT", "/v1/crm/settings", "", []string{`"provider":"hubspot"`, `"complete_setup":true`}},
		{"Metadata", func() error { _, _, e := c.CRM.Metadata(ctx); return e }, "GET", "/v1/crm/metadata", "", nil},
		{"Owners", func() error { _, _, e := c.CRM.Owners(ctx); return e }, "GET", "/v1/crm/owners", "", nil},
		{"MapOwner", func() error { _, e := c.CRM.MapOwner(ctx, "own_9", &user); return e },
			"PUT", "/v1/crm/owners/own_9", "", []string{`"user_id":"usr_1"`}},
		{"MapOwner clear", func() error { _, e := c.CRM.MapOwner(ctx, "own_9", nil); return e },
			"PUT", "/v1/crm/owners/own_9", "", []string{`"user_id":null`}},
		{"SyncHealth", func() error { _, _, e := c.CRM.SyncHealth(ctx); return e }, "GET", "/v1/crm/sync", "", nil},
		{"SyncNow", func() error { _, e := c.CRM.SyncNow(ctx); return e }, "POST", "/v1/crm/sync", "", nil},
		{"RetrySyncFailures", func() error { _, _, e := c.CRM.RetrySyncFailures(ctx, []string{"j1"}); return e },
			"POST", "/v1/crm/sync/retry", "", []string{`"ids":["j1"]`}},
		{"DiscardSyncFailures", func() error { _, _, e := c.CRM.DiscardSyncFailures(ctx, nil); return e },
			"POST", "/v1/crm/sync/discard", "", []string{`{}`}},
		{"Backfill", func() error { _, _, e := c.CRM.Backfill(ctx); return e }, "GET", "/v1/crm/backfill", "", nil},
		{"StartBackfill", func() error { _, e := c.CRM.StartBackfill(ctx, &CRMBackfillParams{Deals: true, Notes: true}); return e },
			"POST", "/v1/crm/backfill", "", []string{`"deals":true`, `"tasks":false`, `"notes":true`}},
		{"Contact", func() error { _, _, e := c.CRM.Contact(ctx, "ct_1"); return e }, "GET", "/v1/crm/contacts/ct_1", "", nil},
		{"RefreshContact", func() error { _, _, e := c.CRM.RefreshContact(ctx, "ct_1"); return e },
			"POST", "/v1/crm/contacts/ct_1/refresh", "", nil},
		{"LinkContact", func() error { _, _, e := c.CRM.LinkContact(ctx, "ct_1"); return e },
			"POST", "/v1/crm/contacts/ct_1/link", "", nil},
		{"UpdateContact", func() error {
			_, _, e := c.CRM.UpdateContact(ctx, "ct_1", &CRMContactUpdateParams{LeadStatus: String("OPEN")})
			return e
		}, "PATCH", "/v1/crm/contacts/ct_1", "", []string{`"lead_status":"OPEN"`}},
		{"Lists", func() error {
			_, e := c.CRM.Lists(ctx, &CRMListParams{ListOptions: ListOptions{Limit: 10, Cursor: "cur"}, Query: "vip"})
			return e
		}, "GET", "/v1/crm/lists", "cursor=cur&limit=10&q=vip", nil},
		{"PreviewListImport", func() error {
			_, _, e := c.CRM.PreviewListImport(ctx, &CRMListImportParams{ListID: "L1", ApplyGuards: true})
			return e
		}, "POST", "/v1/crm/lists/preview", "", []string{`"list_id":"L1"`, `"apply_guards":true`}},
		{"ImportList", func() error {
			_, _, e := c.CRM.ImportList(ctx, &CRMListImportParams{ListID: "L1"})
			return e
		}, "POST", "/v1/crm/lists/import", "", []string{`"list_id":"L1"`}},
		{"BulkUpdateTasks ids", func() error {
			_, _, e := c.CRM.BulkUpdateTasks(ctx, &CRMTaskBulkUpdateParams{
				CRMTaskSelection: CRMTaskSelection{Tasks: []string{"t1", "t2"}},
				Status:           String(TaskStatusCompleted),
			})
			return e
		}, "PATCH", "/v1/crm/tasks", "", []string{`"tasks":["t1","t2"]`, `"status":"completed"`}},
		{"BulkUpdateTasks filter", func() error {
			_, _, e := c.CRM.BulkUpdateTasks(ctx, &CRMTaskBulkUpdateParams{
				CRMTaskSelection: CRMTaskSelection{All: true, Filters: &TaskSearchParams{Overdue: true}, Exclude: []string{"t9"}},
				Priority:         String(TaskPriorityHigh),
			})
			return e
		}, "PATCH", "/v1/crm/tasks", "", []string{`"all":true`, `"filters":{"overdue":true}`, `"exclude":["t9"]`, `"priority":"high"`}},
		{"BulkDeleteTasks", func() error {
			_, _, e := c.CRM.BulkDeleteTasks(ctx, &CRMTaskSelection{Tasks: []string{"t1"}})
			return e
		}, "DELETE", "/v1/crm/tasks", "", []string{`"tasks":["t1"]`}},
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

func TestCRMSettingsDecodes(t *testing.T) {
	var got recordedRequest
	connID := "conn_1"
	completed := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	want := CRMSettings{
		OrganizationID: "org_1",
		Provider:       CRMProviderPipedrive,
		ConnectionID:   &connID,
		Config: CRMProviderConfig{
			Activity:       CRMActivityLog{Sent: true, Replies: true},
			CreateContacts: true,
			PositiveReply:  CRMReplyOutcome{CreateDeal: true, DealPipelineID: String("pl_1"), DealStageID: String("st_1")},
			ExitRules:      CRMExitRules{DealCreated: true, LifecycleStages: []string{"customer"}},
			Guards:         CRMEnrollmentGuards{SkipOpenDeals: true, SkipLifecycleStages: []string{}},
			DealPipelines:  []string{},
			FieldMap:       map[string]string{"company": "org_name"},
			FieldDirection: map[string]string{"company": CRMFieldBoth},
			For:            CRMProviderPipedrive,
		},
		SetupCompletedAt: &completed,
		UpdatedAt:        completed,
		Account:          &CRMAccount{ExternalID: "42", Name: "Acme", AppURL: "https://acme.pipedrive.com", Status: "connected", Health: "healthy"},
	}
	c := crmServe(t, &got, 200, want)
	out, _, err := c.CRM.Settings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*out, want) {
		t.Fatalf("decoded %+v, want %+v", *out, want)
	}
}

func TestCRMSyncHealthDecodes(t *testing.T) {
	var got recordedRequest
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	want := CRMSyncHealth{
		Pending: 3, Failed: 1, Done24h: 40, LastSyncedAt: &now,
		Cursors:  []CRMSyncCursor{{ObjectType: "deal", CursorAt: &now, LastRunAt: &now}},
		Failures: []CRMSyncJob{{ID: "j1", Provider: CRMProviderHubSpot, Kind: "push_deal", Status: CRMJobFailed, Attempts: 5, LastError: "429", NextAttemptAt: now, CreatedAt: now, UpdatedAt: now}},
		Counts:   CRMMirrorCounts{Contacts: 10, Deals: 2},
	}
	c := crmServe(t, &got, 200, want)
	out, _, err := c.CRM.SyncHealth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*out, want) {
		t.Fatalf("decoded %+v, want %+v", *out, want)
	}

	// The retry and discard calls answer {"affected": n}.
	c = crmServe(t, &got, 200, map[string]int{"affected": 4})
	n, _, err := c.CRM.RetrySyncFailures(context.Background(), nil)
	if err != nil || n != 4 {
		t.Fatalf("RetrySyncFailures = %d, %v", n, err)
	}
}

func TestCRMNoContentCalls(t *testing.T) {
	var got recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path = r.Method, r.URL.Path
		if r.URL.Path == "/v1/crm/sync" || r.URL.Path == "/v1/crm/backfill" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if resp, err := c.CRM.MapOwner(ctx, "own_1", nil); err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("MapOwner: %v %v", resp, err)
	}
	if resp, err := c.CRM.SyncNow(ctx); err != nil || resp.StatusCode != http.StatusAccepted {
		t.Fatalf("SyncNow: %v %v", resp, err)
	}
	if resp, err := c.CRM.StartBackfill(ctx, &CRMBackfillParams{}); err != nil || resp.StatusCode != http.StatusAccepted {
		t.Fatalf("StartBackfill: %v %v", resp, err)
	}
}

func TestCRMContactViewDecodes(t *testing.T) {
	var got recordedRequest
	synced := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	owner := "usr_2"
	want := CRMContactView{
		Provider: CRMProviderHubSpot, Linked: true, ExternalID: "501", URL: "https://app.hubspot.com/c/501",
		Owner:          &CRMOwner{ExternalID: "7", Email: "a@acme.com", FirstName: "Ada", UserID: &owner, UserPinned: true},
		LifecycleStage: &CRMOption{Value: "lead", Label: "Lead"},
		Company:        &CRMCompanyRef{ExternalID: "9", Name: "Acme"},
		Properties:     []CRMPropertyView{{Name: "phone", Label: "Phone", Value: "555"}},
		SyncedAt:       &synced,
	}
	c := crmServe(t, &got, 200, want)
	out, _, err := c.CRM.LinkContact(context.Background(), "ct_1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*out, want) {
		t.Fatalf("decoded %+v, want %+v", *out, want)
	}
}

func TestCRMListsPages(t *testing.T) {
	var got recordedRequest
	next := "c2"
	total := int64(1)
	c := crmServe(t, &got, 200, Page[CRMList]{
		Data:       []CRMList{{ExternalID: "L1", Name: "VIP", Size: 12, Dynamic: true}},
		Pagination: Pagination{Total: &total, NextCursor: &next, HasMore: true},
	})
	page, err := c.CRM.Lists(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 1 || page.Data[0].Name != "VIP" || !page.HasMore() || page.NextCursor() != "c2" {
		t.Fatalf("page = %+v", page)
	}
}

func TestCRMExternalRefOnRecords(t *testing.T) {
	var got recordedRequest
	synced := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	c := crmServe(t, &got, 200, map[string]any{
		"id": "d1", "name": "Big deal", "status": "open",
		"external": CRMExternalRef{Provider: CRMProviderPipedrive, ExternalID: "88", URL: "https://x/88", SyncedAt: synced, OwnerName: "Sam"},
	})
	deal, _, err := c.CRM.GetDeal(context.Background(), "d1")
	if err != nil {
		t.Fatal(err)
	}
	if deal.External == nil || deal.External.Provider != CRMProviderPipedrive || deal.External.OwnerName != "Sam" {
		t.Fatalf("external = %+v", deal.External)
	}

	prob := 0.4
	c = crmServe(t, &got, 200, PipelineStage{ID: "s1", Closed: true, Won: true, Probability: &prob})
	stage, _, err := c.CRM.UpdateStage(context.Background(), "p1", "s1", &StageUpdateParams{})
	if err != nil {
		t.Fatal(err)
	}
	if !stage.Closed || !stage.Won || stage.Probability == nil || *stage.Probability != 0.4 {
		t.Fatalf("stage = %+v", stage)
	}
}

func TestCRMBulkTasksResult(t *testing.T) {
	var got recordedRequest
	c := crmServe(t, &got, 200, CRMTaskBulkResult{Affected: 12})
	out, _, err := c.CRM.BulkDeleteTasks(context.Background(), &CRMTaskSelection{All: true})
	if err != nil || out.Affected != 12 {
		t.Fatalf("BulkDeleteTasks = %+v, %v", out, err)
	}
	if got.method != "DELETE" || !strings.Contains(got.body, `"all":true`) {
		t.Fatalf("request = %+v", got)
	}
}
