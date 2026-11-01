package warmbly

import (
	"context"
	"strings"
	"testing"
)

// TestCRMCoverageRouting asserts method, path, raw query and body of the CRM
// provider-mode and bulk-task methods.
func TestCRMCoverageRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()

	cases := []struct {
		name       string
		call       func() error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   []string
	}{
		{"Settings", func() error { _, _, e := c.CRM.Settings(ctx); return e },
			"GET", "/v1/crm/settings", "", nil},
		{"UpdateSettings", func() error {
			_, _, e := c.CRM.UpdateSettings(ctx, &CRMSettingsUpdate{
				Provider: String(CRMProviderHubSpot), ConnectionID: String("cn_1"), CompleteSetup: true,
				Config: &CRMConfig{CreateContacts: true, FieldMap: map[string]string{"company": "company"}},
			})
			return e
		}, "PUT", "/v1/crm/settings", "", []string{
			`"provider":"hubspot"`, `"connection_id":"cn_1"`, `"complete_setup":true`,
			`"create_contacts":true`, `"write_properties":false`, `"field_map":{"company":"company"}`,
		}},
		{"Metadata", func() error { _, _, e := c.CRM.Metadata(ctx); return e },
			"GET", "/v1/crm/metadata", "", nil},
		{"ListOwners", func() error { _, _, e := c.CRM.ListOwners(ctx); return e },
			"GET", "/v1/crm/owners", "", nil},
		{"MapOwner", func() error { _, e := c.CRM.MapOwner(ctx, "ow 1", String("u_1")); return e },
			"PUT", "/v1/crm/owners/ow 1", "", []string{`{"user_id":"u_1"}`}},
		{"MapOwner clear", func() error { _, e := c.CRM.MapOwner(ctx, "ow_1", nil); return e },
			"PUT", "/v1/crm/owners/ow_1", "", []string{`{"user_id":null}`}},
		{"SyncHealth", func() error { _, _, e := c.CRM.SyncHealth(ctx); return e },
			"GET", "/v1/crm/sync", "", nil},
		{"SyncNow", func() error { _, e := c.CRM.SyncNow(ctx); return e },
			"POST", "/v1/crm/sync", "", nil},
		{"RetrySyncFailures", func() error { _, _, e := c.CRM.RetrySyncFailures(ctx, []string{"j1"}); return e },
			"POST", "/v1/crm/sync/retry", "", []string{`{"ids":["j1"]}`}},
		{"DiscardSyncFailures all", func() error { _, _, e := c.CRM.DiscardSyncFailures(ctx, nil); return e },
			"POST", "/v1/crm/sync/discard", "", []string{`{}`}},
		{"BackfillPreview", func() error { _, _, e := c.CRM.BackfillPreview(ctx); return e },
			"GET", "/v1/crm/backfill", "", nil},
		{"StartBackfill", func() error {
			_, e := c.CRM.StartBackfill(ctx, &CRMBackfillRequest{Deals: true, Notes: true})
			return e
		}, "POST", "/v1/crm/backfill", "", []string{`{"deals":true,"tasks":false,"notes":true}`}},
		{"Contact", func() error { _, _, e := c.CRM.Contact(ctx, "ct_1"); return e },
			"GET", "/v1/crm/contacts/ct_1", "", nil},
		{"RefreshContact", func() error { _, _, e := c.CRM.RefreshContact(ctx, "ct_1"); return e },
			"POST", "/v1/crm/contacts/ct_1/refresh", "", nil},
		{"LinkContact", func() error { _, _, e := c.CRM.LinkContact(ctx, "ct_1"); return e },
			"POST", "/v1/crm/contacts/ct_1/link", "", nil},
		{"UpdateContact", func() error {
			_, _, e := c.CRM.UpdateContact(ctx, "ct_1", &CRMContactUpdate{LifecycleStage: String("lead")})
			return e
		}, "PATCH", "/v1/crm/contacts/ct_1", "", []string{`{"lifecycle_stage":"lead"}`}},
		{"ListLists", func() error {
			_, e := c.CRM.ListLists(ctx, &CRMListParams{ListOptions: ListOptions{Limit: 10, Cursor: "pc"}, Query: "vip"})
			return e
		}, "GET", "/v1/crm/lists", "cursor=pc&limit=10&q=vip", nil},
		{"PreviewImport", func() error {
			_, _, e := c.CRM.PreviewImport(ctx, &CRMImportParams{ListID: "l1", ApplyGuards: true})
			return e
		}, "POST", "/v1/crm/lists/preview", "", []string{`{"list_id":"l1","apply_guards":true}`}},
		{"ImportList", func() error {
			_, _, e := c.CRM.ImportList(ctx, &CRMImportParams{ListID: "l1"})
			return e
		}, "POST", "/v1/crm/lists/import", "", []string{`{"list_id":"l1","apply_guards":false}`}},
		{"BulkUpdateTasks ids", func() error {
			_, _, e := c.CRM.BulkUpdateTasks(ctx, &CRMTaskBulkUpdate{
				CRMTaskSelection: CRMTaskSelection{Tasks: []string{"t1", "t2"}},
				Status:           String(TaskStatusCompleted),
			})
			return e
		}, "PATCH", "/v1/crm/tasks", "", []string{`"tasks":["t1","t2"]`, `"status":"completed"`}},
		{"BulkUpdateTasks filter", func() error {
			_, _, e := c.CRM.BulkUpdateTasks(ctx, &CRMTaskBulkUpdate{
				CRMTaskSelection: CRMTaskSelection{
					All: true, Exclude: []string{"t9"},
					Filters: &TaskSearchParams{Statuses: []string{TaskStatusPending}, Overdue: true},
				},
				Priority: String(TaskPriorityHigh),
			})
			return e
		}, "PATCH", "/v1/crm/tasks", "", []string{
			`"all":true`, `"exclude":["t9"]`, `"filters":{"statuses":["pending"],"overdue":true}`, `"priority":"high"`,
		}},
		{"BulkDeleteTasks", func() error {
			_, _, e := c.CRM.BulkDeleteTasks(ctx, &CRMTaskSelection{Tasks: []string{"t1"}})
			return e
		}, "DELETE", "/v1/crm/tasks", "", []string{`{"tasks":["t1"]}`}},
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
			for _, frag := range tc.wantBody {
				if !strings.Contains(got.body, frag) {
					t.Errorf("body %s lacks %s", got.body, frag)
				}
			}
			if len(tc.wantBody) == 0 && strings.TrimSpace(got.body) != "" {
				t.Errorf("unexpected body %s", got.body)
			}
		})
	}
}

func TestCRMPipelinesBareArray(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `[{
	  "id": "pl_1", "organization_id": "org_1", "name": "Sales", "position": 0,
	  "stages": [
	    {"id": "st_1", "pipeline_id": "pl_1", "name": "Won", "color": "#0a0", "position": 3, "closed": true, "won": true, "probability": 100, "created_at": "2026-10-01T09:00:00Z", "updated_at": "2026-10-01T09:00:00Z"}
	  ],
	  "created_at": "2026-10-01T09:00:00Z", "updated_at": "2026-10-01T09:00:00Z",
	  "external": {"provider": "pipedrive", "external_id": "7", "url": "https://x", "synced_at": "2026-10-01T09:00:00Z"}
	}]`)
	ps, _, err := c.CRM.ListPipelines(context.Background())
	if err != nil {
		t.Fatalf("ListPipelines: %v", err)
	}
	st := ps[0].Stages[0]
	if !st.Closed || !st.Won || st.Probability == nil || *st.Probability != 100 {
		t.Errorf("stage = %+v", st)
	}
	if ps[0].External == nil || ps[0].External.Provider != CRMProviderPipedrive {
		t.Errorf("pipeline = %+v", ps[0])
	}

	// A workspace with no pipelines answers null, which must still range.
	c = recordingFixtureClient(t, &got, `null`)
	ps, _, err = c.CRM.ListPipelines(context.Background())
	if err != nil || ps == nil || len(ps) != 0 {
		t.Errorf("ps = %v, err = %v", ps, err)
	}
}

func TestCRMSettingsDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{
	  "organization_id": "org_1", "provider": "future_crm", "connection_id": "cn_1",
	  "config": {
	    "activity": {"sent": true, "replies": true, "bounces": true, "unsubscribes": true, "opens": false, "clicks": false, "meetings": true},
	    "create_contacts": true, "create_companies": true, "write_properties": true,
	    "positive_reply": {"lead_status": "IN_PROGRESS", "lifecycle_stage": "", "create_deal": true, "deal_pipeline_id": "pl_1", "deal_stage_id": "st_1"},
	    "exit_rules": {"deal_created": true, "lifecycle_stages": ["customer"], "opted_out": true},
	    "guards": {"skip_lifecycle_stages": [], "skip_open_deals": true, "skip_other_owners": false, "skip_opted_out": true},
	    "deal_pipelines": [], "display_properties": [],
	    "field_map": {"first_name": "firstname"}, "field_direction": {"first_name": "both"}
	  },
	  "setup_completed_at": "2026-10-01T09:00:00Z", "updated_at": "2026-10-01T09:00:00Z",
	  "account": {"external_id": "123", "name": "Acme", "app_url": "https://app.hubspot.com/contacts/123", "status": "connected", "health": "healthy", "missing_scopes": ["crm.lists.read"]}
	}`)
	s, _, err := c.CRM.Settings(context.Background())
	if err != nil {
		t.Fatalf("Settings: %v", err)
	}
	// A provider this release has never heard of still decodes.
	if s.Provider != "future_crm" || s.ConnectionID == nil || s.SetupCompletedAt == nil {
		t.Errorf("settings = %+v", s)
	}
	if s.Config.PositiveReply.DealPipelineID == nil || s.Config.FieldDirection["first_name"] != CRMFieldBoth {
		t.Errorf("config = %+v", s.Config)
	}
	if s.Account == nil || s.Account.MissingScopes[0] != "crm.lists.read" {
		t.Errorf("account = %+v", s.Account)
	}
}

func TestCRMSyncHealthDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{
	  "pending": 3, "failed": 1, "done_24h": 40, "last_synced_at": "2026-10-01T09:00:00Z",
	  "cursors": [{"object_type": "deal", "cursor_at": "2026-10-01T08:00:00Z", "last_run_at": "2026-10-01T09:00:00Z"}],
	  "failures": [{"id": "j1", "organization_id": "org_1", "provider": "hubspot", "kind": "push_deal", "subject": "Big deal", "status": "failed", "attempts": 5, "next_attempt_at": "2026-10-01T09:30:00Z", "last_error": "429", "created_at": "2026-10-01T08:00:00Z", "updated_at": "2026-10-01T09:00:00Z"}],
	  "counts": {"contacts": 100, "deals": 9, "tasks": 4, "pipelines": 1, "owners": 3}
	}`)
	h, _, err := c.CRM.SyncHealth(context.Background())
	if err != nil {
		t.Fatalf("SyncHealth: %v", err)
	}
	if h.Pending != 3 || h.Failures[0].Status != CRMJobFailed || h.Failures[0].FinishedAt != nil || h.Counts.Owners != 3 {
		t.Errorf("health = %+v", h)
	}
	if h.Cursors[0].CursorAt == nil || h.Cursors[0].LastError != "" {
		t.Errorf("cursor = %+v", h.Cursors[0])
	}
}

func TestCRMOwnersAndAffectedDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"data": [
	  {"external_id": "o1", "email": "a@acme.com", "first_name": "Ann", "last_name": "Lee", "user_id": "u_1", "user_pinned": true, "archived": false},
	  {"external_id": "o2", "email": "b@acme.com", "first_name": "", "last_name": "", "user_pinned": false, "archived": true}
	]}`)
	owners, _, err := c.CRM.ListOwners(context.Background())
	if err != nil {
		t.Fatalf("ListOwners: %v", err)
	}
	if owners[0].UserID == nil || !owners[0].UserPinned || owners[1].UserID != nil || !owners[1].Archived {
		t.Errorf("owners = %+v", owners)
	}

	c = recordingFixtureClient(t, &got, `{"affected": 7}`)
	n, _, err := c.CRM.RetrySyncFailures(context.Background(), nil)
	if err != nil || n != 7 {
		t.Errorf("affected = %d, err = %v", n, err)
	}
	n, _, err = c.CRM.BulkUpdateTasks(context.Background(), &CRMTaskBulkUpdate{Status: String(TaskStatusCompleted)})
	if err != nil || n != 7 {
		t.Errorf("bulk affected = %d, err = %v", n, err)
	}
}

func TestCRMContactViewDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{
	  "provider": "hubspot", "linked": true, "external_id": "501", "url": "https://x",
	  "owner": {"external_id": "o1", "email": "a@acme.com", "first_name": "Ann", "last_name": "Lee", "user_pinned": false, "archived": false},
	  "lifecycle_stage": {"value": "lead", "label": "Lead"},
	  "company": {"external_id": "c1", "name": "Acme", "domain": "acme.com"},
	  "opted_out": false,
	  "properties": [{"name": "jobtitle", "label": "Job title", "value": "CTO"}],
	  "synced_at": "2026-10-01T09:00:00Z"
	}`)
	v, _, err := c.CRM.Contact(context.Background(), "ct_1")
	if err != nil {
		t.Fatalf("Contact: %v", err)
	}
	if !v.Linked || v.Owner == nil || v.LifecycleStage == nil || v.LeadStatus != nil || v.Company.Domain != "acme.com" {
		t.Errorf("view = %+v", v)
	}

	// An unlinked contact carries no provider fields.
	c = recordingFixtureClient(t, &got, `{"provider": "hubspot", "linked": false, "opted_out": false, "properties": []}`)
	v, _, err = c.CRM.Contact(context.Background(), "ct_2")
	if err != nil || v.Linked || v.Owner != nil || v.SyncedAt != nil {
		t.Errorf("view = %+v, err = %v", v, err)
	}
}

func TestCRMListsPage(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{
	  "data": [{"external_id": "l1", "name": "VIPs", "size": 120, "dynamic": true, "updated_at": "2026-10-01"}],
	  "pagination": {"total": null, "next_cursor": "pc2", "has_more": true}
	}`)
	page, err := c.CRM.ListLists(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListLists: %v", err)
	}
	if page.Data[0].Size != 120 || !page.Data[0].Dynamic || page.Pagination.Total != nil || page.NextCursor() != "pc2" {
		t.Errorf("page = %+v", page)
	}
}

func TestCRMImportDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{
	  "import_id": "imp_1",
	  "preview": {
	    "list_name": "VIPs", "total": 120, "included": 100,
	    "skipped": [{"reason": "opted_out", "label": "Opted out", "count": 20}],
	    "sample": [{"email": "a@acme.com", "first_name": "Ann", "last_name": "Lee", "company": "Acme"}],
	    "truncated": false
	  }
	}`)
	res, _, err := c.CRM.ImportList(context.Background(), &CRMImportParams{ListID: "l1"})
	if err != nil {
		t.Fatalf("ImportList: %v", err)
	}
	if res.ImportID != "imp_1" || res.Preview.Included != 100 || res.Preview.Skipped[0].Count != 20 {
		t.Errorf("result = %+v", res)
	}
}

func TestCRMExternalRefOnDealAndTask(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{
	  "id": "d1", "organization_id": "org_1", "pipeline_id": "pl_1", "stage_id": "st_1", "name": "Big", "currency": "USD", "status": "open",
	  "created_at": "2026-10-01T09:00:00Z", "updated_at": "2026-10-01T09:00:00Z",
	  "external": {"provider": "hubspot", "external_id": "9", "synced_at": "2026-10-01T09:00:00Z", "owner_name": "Pat"}
	}`)
	d, _, err := c.CRM.GetDeal(context.Background(), "d1")
	if err != nil {
		t.Fatalf("GetDeal: %v", err)
	}
	if d.External == nil || d.External.OwnerName != "Pat" {
		t.Errorf("deal = %+v", d)
	}

	c = recordingFixtureClient(t, &got, `{
	  "id": "t1", "organization_id": "org_1", "created_by": "u_1", "title": "Call", "priority": "future_priority", "type": "Call", "status": "pending",
	  "created_at": "2026-10-01T09:00:00Z", "updated_at": "2026-10-01T09:00:00Z",
	  "external": {"provider": "pipedrive", "external_id": "3", "synced_at": "2026-10-01T09:00:00Z"}
	}`)
	task, _, err := c.CRM.GetTask(context.Background(), "t1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.External == nil || task.Priority != "future_priority" {
		t.Errorf("task = %+v", task)
	}
}
