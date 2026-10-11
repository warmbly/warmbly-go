package warmbly

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// crmSyncServer answers every request with status and body and records the
// last request into got.
func crmSyncServer(t *testing.T, status int, body string, got *recordedRequest) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.path = r.URL.Path
		got.rawQuery = r.URL.RawQuery
		b, _ := io.ReadAll(r.Body)
		got.body = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// exactCase asserts method, path, the exact raw query, and fragments of (or
// the absence of) a body.
type exactCase struct {
	name       string
	call       func() error
	wantMethod string
	wantPath   string
	wantQuery  string
	wantBody   []string
	// wantNoBody asserts the request carried no body at all.
	wantNoBody bool
}

func runExactCases(t *testing.T, got *recordedRequest, cases []exactCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			*got = recordedRequest{}
			if err := tc.call(); err != nil {
				t.Fatalf("call: %v", err)
			}
			if got.method != tc.wantMethod || got.path != tc.wantPath {
				t.Errorf("got %s %s, want %s %s", got.method, got.path, tc.wantMethod, tc.wantPath)
			}
			if got.rawQuery != tc.wantQuery {
				t.Errorf("query = %q, want %q", got.rawQuery, tc.wantQuery)
			}
			if tc.wantNoBody && got.body != "" {
				t.Errorf("body = %s, want none", got.body)
			}
			for _, frag := range tc.wantBody {
				if !strings.Contains(got.body, frag) {
					t.Errorf("body %s missing %s", got.body, frag)
				}
			}
		})
	}
}

const envelopeBody = `{"data": [], "pagination": {"has_more": false}}`

func TestCRMProviderSyncRouting(t *testing.T) {
	var got recordedRequest
	c := crmSyncServer(t, http.StatusOK, envelopeBody, &got)
	ctx := context.Background()

	runExactCases(t, &got, []exactCase{
		{"GetSettings", func() error {
			_, _, e := c.CRM.GetSettings(ctx)
			return e
		}, "GET", "/v1/crm/settings", "", nil, true},
		{"UpdateSettings", func() error {
			_, _, e := c.CRM.UpdateSettings(ctx, &CRMSettingsUpdateParams{
				Provider:      String(CRMProviderHubSpot),
				ConnectionID:  String("conn_1"),
				CompleteSetup: true,
				Config: &CRMProviderConfig{
					Activity:       CRMActivityLog{Sent: true, Replies: true},
					FieldMap:       map[string]string{"first_name": "firstname"},
					FieldDirection: map[string]string{"first_name": CRMFieldBoth},
					PositiveReply: CRMReplyOutcome{
						CreateDeal: true, DealPipelineID: String("pl_1"), DealStageID: String("st_1"),
					},
				},
			})
			return e
		}, "PUT", "/v1/crm/settings", "", []string{
			`"provider":"hubspot"`, `"connection_id":"conn_1"`, `"complete_setup":true`,
			`"activity":{"sent":true,"replies":true,`, `"field_map":{"first_name":"firstname"}`,
			`"field_direction":{"first_name":"both"}`, `"deal_pipeline_id":"pl_1"`, `"deal_stage_id":"st_1"`,
		}, false},
		{"GetMetadata", func() error {
			_, _, e := c.CRM.GetMetadata(ctx)
			return e
		}, "GET", "/v1/crm/metadata", "", nil, true},
		{"ListOwners", func() error {
			_, _, e := c.CRM.ListOwners(ctx)
			return e
		}, "GET", "/v1/crm/owners", "", nil, true},
		{"MapOwner", func() error {
			_, e := c.CRM.MapOwner(ctx, "own_1", String("usr_1"))
			return e
		}, "PUT", "/v1/crm/owners/own_1", "", []string{`{"user_id":"usr_1"}`}, false},
		{"MapOwner clear", func() error {
			_, e := c.CRM.MapOwner(ctx, "own_1", nil)
			return e
		}, "PUT", "/v1/crm/owners/own_1", "", []string{`{"user_id":null}`}, false},
		{"GetSyncHealth", func() error {
			_, _, e := c.CRM.GetSyncHealth(ctx)
			return e
		}, "GET", "/v1/crm/sync", "", nil, true},
		{"SyncNow", func() error {
			_, e := c.CRM.SyncNow(ctx)
			return e
		}, "POST", "/v1/crm/sync", "", nil, true},
		{"RetrySyncFailures ids", func() error {
			_, _, e := c.CRM.RetrySyncFailures(ctx, []string{"job_1"})
			return e
		}, "POST", "/v1/crm/sync/retry", "", []string{`{"ids":["job_1"]}`}, false},
		{"RetrySyncFailures all", func() error {
			_, _, e := c.CRM.RetrySyncFailures(ctx, nil)
			return e
		}, "POST", "/v1/crm/sync/retry", "", []string{`{}`}, false},
		{"DiscardSyncFailures", func() error {
			_, _, e := c.CRM.DiscardSyncFailures(ctx, []string{"job_1", "job_2"})
			return e
		}, "POST", "/v1/crm/sync/discard", "", []string{`{"ids":["job_1","job_2"]}`}, false},
		{"GetBackfill", func() error {
			_, _, e := c.CRM.GetBackfill(ctx)
			return e
		}, "GET", "/v1/crm/backfill", "", nil, true},
		{"StartBackfill", func() error {
			_, e := c.CRM.StartBackfill(ctx, &CRMBackfillParams{Deals: true, Notes: true})
			return e
		}, "POST", "/v1/crm/backfill", "", []string{`{"deals":true,"tasks":false,"notes":true}`}, false},
		{"StartBackfill nil", func() error {
			_, e := c.CRM.StartBackfill(ctx, nil)
			return e
		}, "POST", "/v1/crm/backfill", "", []string{`{"deals":false,"tasks":false,"notes":false}`}, false},
		{"GetContactRecord", func() error {
			_, _, e := c.CRM.GetContactRecord(ctx, "ct_1")
			return e
		}, "GET", "/v1/crm/contacts/ct_1", "", nil, true},
		{"RefreshContactRecord", func() error {
			_, _, e := c.CRM.RefreshContactRecord(ctx, "ct_1")
			return e
		}, "POST", "/v1/crm/contacts/ct_1/refresh", "", nil, true},
		{"LinkContactRecord", func() error {
			_, _, e := c.CRM.LinkContactRecord(ctx, "ct_1")
			return e
		}, "POST", "/v1/crm/contacts/ct_1/link", "", nil, true},
		{"UpdateContactRecord", func() error {
			_, _, e := c.CRM.UpdateContactRecord(ctx, "ct_1", &CRMContactUpdateParams{
				OwnerExternalID: String("own_1"), LifecycleStage: String("lead"),
			})
			return e
		}, "PATCH", "/v1/crm/contacts/ct_1", "", []string{`{"owner_external_id":"own_1","lifecycle_stage":"lead"}`}, false},
		{"ListLists", func() error {
			_, e := c.CRM.ListLists(ctx, &CRMListParams{ListOptions: ListOptions{Limit: 50, Cursor: "cur"}, Query: "q3"})
			return e
		}, "GET", "/v1/crm/lists", "cursor=cur&limit=50&q=q3", nil, true},
		{"ListLists nil", func() error {
			_, e := c.CRM.ListLists(ctx, nil)
			return e
		}, "GET", "/v1/crm/lists", "", nil, true},
		{"PreviewListImport", func() error {
			_, _, e := c.CRM.PreviewListImport(ctx, &CRMListImportParams{ListID: "42", ApplyGuards: true})
			return e
		}, "POST", "/v1/crm/lists/preview", "", []string{`{"list_id":"42","apply_guards":true}`}, false},
		{"ImportList", func() error {
			_, _, e := c.CRM.ImportList(ctx, &CRMListImportParams{ListID: "42"})
			return e
		}, "POST", "/v1/crm/lists/import", "", []string{`{"list_id":"42","apply_guards":false}`}, false},
	})
}

func TestCRMTaskBulkSyncRouting(t *testing.T) {
	var got recordedRequest
	c := crmSyncServer(t, http.StatusOK, `{"affected": 3}`, &got)
	ctx := context.Background()
	due := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	runExactCases(t, &got, []exactCase{
		{"BulkUpdateTasks ids", func() error {
			res, _, e := c.CRM.BulkUpdateTasks(ctx, &CRMTaskBulkUpdateParams{
				CRMTaskSelection: CRMTaskSelection{Tasks: []string{"tk_1", "tk_2"}},
				Status:           String(TaskStatusCompleted),
			})
			if e == nil && res.Affected != 3 {
				e = errors.New("affected not decoded")
			}
			return e
		}, "PATCH", "/v1/crm/tasks", "", []string{`{"tasks":["tk_1","tk_2"],"status":"completed"}`}, false},
		{"BulkUpdateTasks filter", func() error {
			_, _, e := c.CRM.BulkUpdateTasks(ctx, &CRMTaskBulkUpdateParams{
				CRMTaskSelection: CRMTaskSelection{
					All:     true,
					Filters: &TaskSearchParams{ListOptions: ListOptions{Limit: 10}, Statuses: []string{TaskStatusPending}, DueBefore: &due},
					Exclude: []string{"tk_9"},
				},
				Priority: String(TaskPriorityHigh),
			})
			return e
		}, "PATCH", "/v1/crm/tasks", "", []string{
			`"all":true`, `"filters":{"statuses":["pending"],"due_before":"2026-10-01T00:00:00Z"}`,
			`"exclude":["tk_9"]`, `"priority":"high"`,
		}, false},
		{"BulkDeleteTasks", func() error {
			res, _, e := c.CRM.BulkDeleteTasks(ctx, &CRMTaskSelection{Tasks: []string{"tk_1"}})
			if e == nil && res.Affected != 3 {
				e = errors.New("affected not decoded")
			}
			return e
		}, "DELETE", "/v1/crm/tasks", "", []string{`{"tasks":["tk_1"]}`}, false},
		{"BulkDeleteTasks nil", func() error {
			_, _, e := c.CRM.BulkDeleteTasks(ctx, nil)
			return e
		}, "DELETE", "/v1/crm/tasks", "", []string{`{}`}, false},
	})
}

// TestListPipelinesBareArray guards the shape GET /crm/pipelines answers
// with: a top-level array, not a {"data": ...} envelope.
func TestListPipelinesBareArray(t *testing.T) {
	var got recordedRequest
	body := `[{
		"id": "pl_1", "organization_id": "org_1", "name": "Sales", "position": 0,
		"created_at": "2026-09-01T10:00:00Z", "updated_at": "2026-09-01T10:00:00Z",
		"stages": [{
			"id": "st_1", "pipeline_id": "pl_1", "name": "Won", "color": "#22c55e", "position": 3,
			"created_at": "2026-09-01T10:00:00Z", "updated_at": "2026-09-01T10:00:00Z",
			"closed": true, "won": true, "probability": 1
		}],
		"external": {"provider": "hubspot", "external_id": "default", "url": "https://app.hubspot.com/x", "synced_at": "2026-09-02T08:00:00Z"}
	}]`
	c := crmSyncServer(t, http.StatusOK, body, &got)
	pls, _, err := c.CRM.ListPipelines(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(pls) != 1 || len(pls[0].Stages) != 1 {
		t.Fatalf("pipelines = %+v", pls)
	}
	st := pls[0].Stages[0]
	if !st.Closed || !st.Won || st.Probability == nil || *st.Probability != 1 {
		t.Errorf("stage = %+v", st)
	}
	ext := pls[0].External
	if ext == nil || ext.Provider != CRMProviderHubSpot || ext.ExternalID != "default" || ext.SyncedAt.IsZero() {
		t.Errorf("external = %+v", ext)
	}
}

func TestCRMSettingsDecode(t *testing.T) {
	var got recordedRequest
	body := `{
		"organization_id": "org_1",
		"provider": "pipedrive",
		"connection_id": "conn_1",
		"config": {
			"activity": {"sent": true, "replies": true, "bounces": true, "unsubscribes": true, "opens": false, "clicks": false, "meetings": true},
			"create_contacts": true, "create_companies": true, "write_properties": true,
			"positive_reply": {"lead_status": "", "lifecycle_stage": "", "create_deal": true, "create_lead": true, "deal_pipeline_id": "pl_1", "deal_stage_id": "st_1"},
			"exit_rules": {"deal_created": true, "lifecycle_stages": [], "opted_out": true},
			"guards": {"skip_lifecycle_stages": ["customer"], "skip_open_deals": true, "skip_other_owners": false, "skip_opted_out": true},
			"deal_pipelines": ["1"],
			"display_properties": null,
			"field_map": {"company": "org_name"},
			"field_direction": {"company": "both"},
			"for": "pipedrive"
		},
		"setup_completed_at": "2026-09-03T12:00:00Z",
		"updated_at": "2026-09-03T12:00:00Z",
		"account": {"external_id": "123", "name": "Acme", "app_url": "https://acme.pipedrive.com", "status": "connected", "health": "healthy", "missing_scopes": ["deals:full"]}
	}`
	c := crmSyncServer(t, http.StatusOK, body, &got)
	st, _, err := c.CRM.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Provider != CRMProviderPipedrive || st.ConnectionID == nil || *st.ConnectionID != "conn_1" || st.SetupCompletedAt == nil {
		t.Errorf("settings = %+v", st)
	}
	cfg := st.Config
	if !cfg.PositiveReply.CreateLead || cfg.PositiveReply.DealStageID == nil || cfg.For != CRMProviderPipedrive {
		t.Errorf("config = %+v", cfg)
	}
	if cfg.FieldDirection["company"] != CRMFieldBoth || len(cfg.Guards.SkipLifecycleStages) != 1 {
		t.Errorf("config maps = %+v", cfg)
	}
	if st.Account == nil || st.Account.AppURL == "" || len(st.Account.MissingScopes) != 1 {
		t.Errorf("account = %+v", st.Account)
	}
}

func TestCRMContactViewDecode(t *testing.T) {
	var got recordedRequest
	body := `{
		"provider": "hubspot", "linked": true, "external_id": "501", "url": "https://app.hubspot.com/contacts/1/record/0-1/501",
		"owner": {"external_id": "77", "email": "ae@acme.com", "first_name": "Ada", "last_name": "Byron", "user_id": "usr_1", "user_pinned": true, "archived": false},
		"lifecycle_stage": {"value": "lead", "label": "Lead"},
		"lead_status": {"value": "IN_PROGRESS", "label": "In progress"},
		"company": {"external_id": "9", "name": "Acme", "domain": "acme.com"},
		"opted_out": false,
		"properties": [{"name": "jobtitle", "label": "Job title", "value": "CTO"}],
		"synced_at": "2026-09-03T12:00:00Z"
	}`
	c := crmSyncServer(t, http.StatusOK, body, &got)
	v, _, err := c.CRM.LinkContactRecord(context.Background(), "ct_1")
	if err != nil {
		t.Fatal(err)
	}
	if !v.Linked || v.Owner == nil || v.Owner.UserID == nil || !v.Owner.UserPinned {
		t.Errorf("view = %+v", v)
	}
	if v.LeadStatus == nil || v.LeadStatus.Value != "IN_PROGRESS" || v.Company == nil || v.Company.Domain != "acme.com" {
		t.Errorf("view enums = %+v", v)
	}
	if len(v.Properties) != 1 || v.Properties[0].Value != "CTO" || v.SyncedAt == nil {
		t.Errorf("properties = %+v", v.Properties)
	}
}

func TestCRMSyncHealthDecode(t *testing.T) {
	var got recordedRequest
	body := `{
		"pending": 4, "failed": 1, "done_24h": 120, "last_synced_at": "2026-09-03T12:00:00Z",
		"cursors": [{"object_type": "deal", "cursor_at": "2026-09-03T11:00:00Z", "last_run_at": "2026-09-03T12:00:00Z", "last_error": ""}],
		"failures": [{
			"id": "job_1", "organization_id": "org_1", "provider": "hubspot", "kind": "push_deal",
			"subject": "Acme renewal", "status": "failed", "attempts": 5,
			"next_attempt_at": "2026-09-03T13:00:00Z", "last_error": "property missing",
			"created_at": "2026-09-03T10:00:00Z", "updated_at": "2026-09-03T12:00:00Z"
		}],
		"counts": {"contacts": 1000, "deals": 50, "tasks": 20, "pipelines": 2, "owners": 5}
	}`
	c := crmSyncServer(t, http.StatusOK, body, &got)
	h, _, err := c.CRM.GetSyncHealth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if h.Done24h != 120 || len(h.Cursors) != 1 || h.Cursors[0].CursorAt == nil {
		t.Errorf("health = %+v", h)
	}
	if len(h.Failures) != 1 || h.Failures[0].Status != CRMSyncJobFailed || h.Failures[0].FinishedAt != nil {
		t.Errorf("failures = %+v", h.Failures)
	}
	if h.Counts.Owners != 5 {
		t.Errorf("counts = %+v", h.Counts)
	}
}

func TestCRMListsAndImportDecode(t *testing.T) {
	var got recordedRequest
	lists := `{"data": [{"external_id": "42", "name": "Q3 targets", "size": 310, "dynamic": true, "updated_at": "2026-09-01T00:00:00.000Z"}],
		"pagination": {"next_cursor": "n1", "has_more": true}}`
	c := crmSyncServer(t, http.StatusOK, lists, &got)
	page, err := c.CRM.ListLists(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 1 || !page.Data[0].Dynamic || page.Data[0].Size != 310 || page.NextCursor() != "n1" {
		t.Errorf("lists = %+v", page)
	}

	imp := `{"import_id": "imp_1", "preview": {
		"list_name": "Q3 targets", "total": 310, "included": 300,
		"skipped": [{"reason": "opted_out", "label": "Opted out", "count": 10}],
		"sample": [{"email": "a@acme.com", "first_name": "Ada", "last_name": "B", "company": "Acme"}],
		"truncated": false}}`
	c = crmSyncServer(t, http.StatusCreated, imp, &got)
	res, _, err := c.CRM.ImportList(context.Background(), &CRMListImportParams{ListID: "42"})
	if err != nil {
		t.Fatal(err)
	}
	if res.ImportID != "imp_1" || res.Preview.Included != 300 || len(res.Preview.Skipped) != 1 || res.Preview.Sample[0].Company != "Acme" {
		t.Errorf("import = %+v", res)
	}
}

func TestCRMNotConnectedError(t *testing.T) {
	var got recordedRequest
	body := `{"error": "Conflict", "code": "crm_not_connected", "message": "Connect HubSpot or Pipedrive and choose it as your CRM first."}`
	c := crmSyncServer(t, http.StatusConflict, body, &got)
	_, _, err := c.CRM.GetMetadata(context.Background())
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
		t.Fatalf("err = %v", err)
	}
	if apiErr.Code != ErrCodeCRMNotConnected {
		t.Errorf("code = %q (raw %s)", apiErr.Code, mustJSON(t, apiErr))
	}
}

// TestCRMRecordsCarryExternal covers the provider reference on the deal and
// task shapes, and that an unknown provider decodes without error.
func TestCRMRecordsCarryExternal(t *testing.T) {
	var got recordedRequest
	body := `{"id": "dl_1", "organization_id": "org_1", "pipeline_id": "pl_1", "stage_id": "st_1", "name": "Acme",
		"currency": "USD", "status": "open", "created_at": "2026-09-01T10:00:00Z", "updated_at": "2026-09-01T10:00:00Z",
		"external": {"provider": "some_future_crm", "external_id": "d9", "synced_at": "2026-09-02T08:00:00Z", "owner_name": "Grace"}}`
	c := crmSyncServer(t, http.StatusOK, body, &got)
	d, _, err := c.CRM.GetDeal(context.Background(), "dl_1")
	if err != nil {
		t.Fatal(err)
	}
	if d.External == nil || d.External.Provider != "some_future_crm" || d.External.OwnerName != "Grace" {
		t.Errorf("deal external = %+v", d.External)
	}

	body = `{"id": "tk_1", "organization_id": "org_1", "created_by": "usr_1", "title": "Call", "priority": "high", "type": "Call",
		"status": "pending", "created_at": "2026-09-01T10:00:00Z", "updated_at": "2026-09-01T10:00:00Z",
		"external": {"provider": "pipedrive", "external_id": "a1", "url": "https://acme.pipedrive.com/activities/a1", "synced_at": "2026-09-02T08:00:00Z"}}`
	c = crmSyncServer(t, http.StatusOK, body, &got)
	tk, _, err := c.CRM.GetTask(context.Background(), "tk_1")
	if err != nil {
		t.Fatal(err)
	}
	if tk.External == nil || tk.External.URL == "" {
		t.Errorf("task external = %+v", tk.External)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
