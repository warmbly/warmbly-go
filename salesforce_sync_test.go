package warmbly

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestSalesforceSyncRouting(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, http.StatusOK, envelopeBody)
	ctx := context.Background()
	const base = "/v1/integrations/salesforce/conn_1"

	runExactCases(t, &got, []exactCase{
		{"Overview", func() error {
			_, _, e := c.Salesforce.Overview(ctx, "conn_1", nil)
			return e
		}, "GET", base + "/overview", "", nil, true},
		{"Overview checks", func() error {
			_, _, e := c.Salesforce.Overview(ctx, "conn_1", &SalesforceOverviewParams{Checks: true})
			return e
		}, "GET", base + "/overview", "checks=1", nil, true},
		{"Settings", func() error {
			_, _, e := c.Salesforce.Settings(ctx, "conn_1")
			return e
		}, "GET", base + "/settings", "", nil, true},
		{"UpdateSettings", func() error {
			_, _, e := c.Salesforce.UpdateSettings(ctx, "conn_1", &SalesforceSettings{
				Enabled:  true,
				Matching: SalesforceMatchingSettings{Prefer: "contact", CreateWhen: "reply", CreateAs: "lead", Owner: "fixed", OwnerID: "005xx"},
				Activity: SalesforceActivitySettings{Sent: true, AssignTo: "record_owner"},
				Inbound:  SalesforceInboundSettings{OptOut: "both", PauseOnStatuses: []string{}},
				FieldMap: []SalesforceFieldRule{{
					Object: SalesforceObjectLead, Warmbly: "first_name", Salesforce: "FirstName",
					Direction: SalesforceDirectionBoth, Policy: SalesforcePolicyIfEmpty,
				}},
			})
			return e
		}, "PUT", base + "/settings", "", []string{
			`"enabled":true`, `"owner":"fixed","owner_id":"005xx"`, `"assign_to":"record_owner"`,
			`"field_map":[{"object":"Lead","warmbly":"first_name","salesforce":"FirstName","direction":"both","policy":"if_empty"}]`,
			`"daily_api_budget":0`,
		}, false},
		{"Metadata", func() error {
			_, _, e := c.Salesforce.Metadata(ctx, "conn_1")
			return e
		}, "GET", base + "/metadata", "", nil, true},
		{"Users", func() error {
			_, _, e := c.Salesforce.Users(ctx, "conn_1", &SalesforceSearchParams{Query: "ada"})
			return e
		}, "GET", base + "/users", "q=ada", nil, true},
		{"Users nil", func() error {
			_, _, e := c.Salesforce.Users(ctx, "conn_1", nil)
			return e
		}, "GET", base + "/users", "", nil, true},
		{"ListViews", func() error {
			_, _, e := c.Salesforce.ListViews(ctx, "conn_1", SalesforceObjectContact)
			return e
		}, "GET", base + "/list-views", "object=Contact", nil, true},
		{"Campaigns", func() error {
			_, _, e := c.Salesforce.Campaigns(ctx, "conn_1", &SalesforceSearchParams{Query: "webinar"})
			return e
		}, "GET", base + "/campaigns", "q=webinar", nil, true},
		{"PreviewImport", func() error {
			_, _, e := c.Salesforce.PreviewImport(ctx, "conn_1", &SalesforceImportSourceRef{
				SourceKind: SalesforceSourceListView, Object: SalesforceObjectLead, SourceID: "00Bxx",
			})
			return e
		}, "POST", base + "/import/preview", "", []string{`{"source_kind":"list_view","object":"Lead","source_id":"00Bxx"}`}, false},
		{"ImportSources", func() error {
			_, _, e := c.Salesforce.ImportSources(ctx, "conn_1")
			return e
		}, "GET", base + "/import-sources", "", nil, true},
		{"CreateImportSource", func() error {
			_, _, e := c.Salesforce.CreateImportSource(ctx, "conn_1", &SalesforceImportSourceCreateParams{
				SalesforceImportSourceRef: SalesforceImportSourceRef{SourceKind: SalesforceSourceCampaign, SourceID: "701xx"},
				SourceLabel:               "Webinar",
				CampaignID:                String("camp_1"),
				CategoryIDs:               []string{"cat_1"},
				Recurring:                 Bool(true),
			})
			return e
		}, "POST", base + "/import-sources", "", []string{
			`"source_kind":"campaign"`, `"source_id":"701xx"`, `"source_label":"Webinar"`,
			`"campaign_id":"camp_1"`, `"category_ids":["cat_1"]`, `"recurring":true`,
		}, false},
		{"UpdateImportSource", func() error {
			_, _, e := c.Salesforce.UpdateImportSource(ctx, "conn_1", "src_1", &SalesforceImportSourceUpdateParams{
				Name: String("Webinar list"), Enabled: Bool(false),
			})
			return e
		}, "PATCH", base + "/import-sources/src_1", "", []string{`{"enabled":false,"name":"Webinar list"}`}, false},
		{"UpdateImportSource clear campaign", func() error {
			_, _, e := c.Salesforce.UpdateImportSource(ctx, "conn_1", "src_1", &SalesforceImportSourceUpdateParams{
				CampaignID: String("ignored"), ClearCampaign: true,
			})
			return e
		}, "PATCH", base + "/import-sources/src_1", "", []string{`{"campaign_id":null}`}, false},
		{"RunImportSource", func() error {
			_, _, e := c.Salesforce.RunImportSource(ctx, "conn_1", "src_1")
			return e
		}, "POST", base + "/import-sources/src_1/run", "", nil, true},
		{"DeleteImportSource", func() error {
			_, e := c.Salesforce.DeleteImportSource(ctx, "conn_1", "src_1")
			return e
		}, "DELETE", base + "/import-sources/src_1", "", nil, true},
		{"Activity", func() error {
			_, e := c.Salesforce.Activity(ctx, "conn_1", &SalesforceActivityListParams{
				ListOptions: ListOptions{Limit: 100, Cursor: "c1"}, Status: SalesforceActivityFailed, ContactID: "ct_1",
			})
			return e
		}, "GET", base + "/activity", "contact_id=ct_1&cursor=c1&limit=100&status=failed", nil, true},
		{"RetryActivity ids", func() error {
			_, _, e := c.Salesforce.RetryActivity(ctx, "conn_1", []string{"act_1"})
			return e
		}, "POST", base + "/activity/retry", "", []string{`{"ids":["act_1"]}`}, false},
		{"RetryActivity all", func() error {
			_, _, e := c.Salesforce.RetryActivity(ctx, "conn_1", nil)
			return e
		}, "POST", base + "/activity/retry", "", []string{`{"ids":[]}`}, false},
		{"SyncNow", func() error {
			_, e := c.Salesforce.SyncNow(ctx, "conn_1")
			return e
		}, "POST", base + "/sync-now", "", nil, true},
		{"ContactPanel", func() error {
			_, _, e := c.Salesforce.ContactPanel(ctx, "ct_1")
			return e
		}, "GET", "/v1/contacts/ct_1/salesforce", "", nil, true},
		{"SyncContact", func() error {
			_, _, e := c.Salesforce.SyncContact(ctx, "ct_1", &SalesforceContactSyncParams{ConnectionID: String("conn_1"), CreateAs: "lead"})
			return e
		}, "POST", "/v1/contacts/ct_1/salesforce/sync", "", []string{`{"connection_id":"conn_1","create_as":"lead"}`}, false},
		{"SyncContact nil", func() error {
			_, _, e := c.Salesforce.SyncContact(ctx, "ct_1", nil)
			return e
		}, "POST", "/v1/contacts/ct_1/salesforce/sync", "", []string{`{}`}, false},
		{"UnlinkContact", func() error {
			_, e := c.Salesforce.UnlinkContact(ctx, "ct_1", "lnk_1")
			return e
		}, "DELETE", "/v1/contacts/ct_1/salesforce/links/lnk_1", "", nil, true},
	})
}

func TestSalesforceUpdateParamsOmitUnset(t *testing.T) {
	b, err := json.Marshal(SalesforceImportSourceUpdateParams{})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{}` {
		t.Errorf("empty update = %s, want {}", b)
	}
	b, _ = json.Marshal(SalesforceImportSourceUpdateParams{CategoryIDs: []string{}})
	if string(b) != `{"category_ids":[]}` {
		t.Errorf("clear tags = %s", b)
	}
}

func TestSalesforceOverviewDecode(t *testing.T) {
	var got syncCapture
	body := `{
		"connection_id": "conn_1", "label": "Acme prod", "status": "connected", "health": "healthy",
		"org": {"id": "00Dxx", "instance_url": "https://acme.my.salesforce.com", "environment": "sandbox", "login_host": "test.salesforce.com", "user_id": "005xx", "account": "ada@acme.com"},
		"api": {"used": 1200, "max": 15000, "calls_today": 300, "budget": 3000},
		"counts": {"pending": 2, "synced_24h": 140, "failed": 1, "skipped_24h": 3, "linked_records": 900},
		"last_pull_at": "2026-09-03T12:00:00Z",
		"settings_enabled": true,
		"checks": [{"key": "task_create", "label": "Log activity as Tasks", "ok": true, "detail": "Tasks are logged without the email icon"}]
	}`
	c := respondingClient(t, &got, http.StatusOK, body)
	o, _, err := c.Salesforce.Overview(context.Background(), "conn_1", &SalesforceOverviewParams{Checks: true})
	if err != nil {
		t.Fatal(err)
	}
	if o.Org.Environment != SalesforceSandbox || o.API.Budget != 3000 || o.Counts.LinkedRecords != 900 || o.Counts.Synced24h != 140 {
		t.Errorf("overview = %+v", o)
	}
	if len(o.Checks) != 1 || !o.Checks[0].OK || o.LastPullAt == nil || !o.SettingsEnabled {
		t.Errorf("overview checks = %+v", o)
	}
}

func TestSalesforceSettingsDecode(t *testing.T) {
	var got syncCapture
	settings := `{"enabled": true,
		"matching": {"prefer": "contact", "create_when": "reply", "create_as": "lead", "lead_source": "Warmbly", "lead_status": "", "owner": "connected_user", "run_assignment_rules": false},
		"activity": {"sent": true, "replied": true, "opened": false, "clicked": false, "bounced": true, "unsubscribed": true, "meeting_booked": true, "include_body": true, "assign_to": "record_owner", "relate_to_opportunity": true},
		"writeback": {"lead_status_on_sent": "Working", "lead_status_on_reply": {"positive": "Qualified"}, "lead_status_on_meeting": "", "never_move_backwards": true},
		"inbound": {"opt_out": "both", "pause_on_converted": false, "pause_on_statuses": [], "pause_on_open_opportunity": true},
		"field_map": [{"object": "Contact", "warmbly": "company", "salesforce": "Account.Name", "direction": "pull", "policy": "if_empty"}],
		"daily_api_budget": 0}`
	body := `{"settings": ` + settings + `, "warmbly_fields": [{"key": "first_name", "label": "First name"}], "defaults": ` + settings + `}`
	c := respondingClient(t, &got, http.StatusOK, body)
	res, _, err := c.Salesforce.Settings(context.Background(), "conn_1")
	if err != nil {
		t.Fatal(err)
	}
	st := res.Settings
	if !st.Enabled || st.Writeback.LeadStatusOnReply["positive"] != "Qualified" || !st.Inbound.PauseOnOpenOpportunity {
		t.Errorf("settings = %+v", st)
	}
	if len(st.FieldMap) != 1 || st.FieldMap[0].Direction != SalesforceDirectionPull || len(res.WarmblyFields) != 1 || !res.Defaults.Enabled {
		t.Errorf("settings result = %+v", res)
	}

	c = respondingClient(t, &got, http.StatusOK, `{"settings": `+settings+`}`)
	saved, _, err := c.Salesforce.UpdateSettings(context.Background(), "conn_1", &st)
	if err != nil {
		t.Fatal(err)
	}
	if saved == nil || saved.Matching.LeadSource != "Warmbly" {
		t.Errorf("saved = %+v", saved)
	}
	if !strings.Contains(got.body, `"lead_status_on_reply":{"positive":"Qualified"}`) {
		t.Errorf("body = %s", got.body)
	}
}

func TestSalesforceImportSourceDecode(t *testing.T) {
	var got syncCapture
	body := `{"data": [{
		"id": "src_1", "organization_id": "org_1", "connection_id": "conn_1", "created_by_user_id": "usr_1",
		"name": "Webinar", "source_kind": "campaign", "object": "CampaignMember", "source_id": "701xx", "source_label": "Webinar",
		"campaign_id": null, "category_ids": ["cat_1"], "recurring": true, "enabled": true, "status": "idle",
		"last_run_at": "2026-09-03T12:00:00Z",
		"last_result": {"read": 120, "imported": 100, "updated": 5, "linked": 110, "skipped": 10, "failed": 0, "no_email": 4, "opted_out": 6, "truncated": true},
		"total_imported": 100, "created_at": "2026-09-01T10:00:00Z", "updated_at": "2026-09-03T12:00:00Z"
	}]}`
	c := respondingClient(t, &got, http.StatusOK, body)
	srcs, _, err := c.Salesforce.ImportSources(context.Background(), "conn_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(srcs) != 1 {
		t.Fatalf("sources = %+v", srcs)
	}
	s := srcs[0]
	if s.Object != SalesforceObjectCampaignMember || s.CampaignID != nil || s.Status != SalesforceSourceIdle || s.CreatedByUserID == nil {
		t.Errorf("source = %+v", s)
	}
	if s.LastResult == nil || s.LastResult.NoEmail != 4 || !s.LastResult.Truncated {
		t.Errorf("last result = %+v", s.LastResult)
	}
}

func TestSalesforceActivityPageDecode(t *testing.T) {
	var got syncCapture
	body := `{"data": [{
		"id": "act_1", "organization_id": "org_1", "connection_id": "conn_1", "contact_id": "ct_1", "contact_email": "a@acme.com",
		"kind": "replied", "payload": {"campaign_id": "camp_1"}, "status": "failed", "attempts": 3,
		"next_attempt_at": "2026-09-03T13:00:00Z", "record_id": "00Qxx", "detail": "FIELD_CUSTOM_VALIDATION_EXCEPTION",
		"occurred_at": "2026-09-03T11:00:00Z", "created_at": "2026-09-03T11:00:01Z", "processed_at": "2026-09-03T11:05:00Z"
	}], "pagination": {"next_cursor": "cur_2", "has_more": true}}`
	c := respondingClient(t, &got, http.StatusOK, body)
	page, err := c.Salesforce.Activity(context.Background(), "conn_1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !page.HasMore() || page.NextCursor() != "cur_2" || len(page.Data) != 1 {
		t.Fatalf("page = %+v", page)
	}
	a := page.Data[0]
	if a.Kind != SalesforceKindReplied || a.Status != SalesforceActivityFailed || a.ContactID == nil || a.ProcessedAt == nil || a.Payload["campaign_id"] != "camp_1" {
		t.Errorf("activity = %+v", a)
	}
}

func TestSalesforceContactPanelDecode(t *testing.T) {
	var got syncCapture
	body := `{
		"connections": [{"id": "conn_1", "label": "Acme prod", "environment": "production", "instance_url": "https://acme.my.salesforce.com"}],
		"records": [{
			"link_id": "lnk_1", "connection_id": "conn_1", "connection_label": "Acme prod", "object": "Contact",
			"id": "003xx", "url": "https://acme.my.salesforce.com/003xx", "name": "Ada Byron", "title": "CTO",
			"status": "", "owner": {"id": "005xx", "name": "Grace"},
			"account": {"id": "001xx", "name": "Acme", "url": "https://acme.my.salesforce.com/001xx"},
			"is_converted": false, "opted_out": false,
			"opportunities": [{"id": "006xx", "name": "Renewal", "stage": "Proposal", "amount": 12000, "close_date": "2026-12-01", "is_closed": false, "is_won": false, "url": "https://acme.my.salesforce.com/006xx"}],
			"tasks": [{"id": "00Txx", "subject": "Email: Hi", "date": "2026-09-01", "status": "Completed", "url": "https://acme.my.salesforce.com/00Txx", "from_warmbly": true}],
			"linked_by": "match", "last_synced_at": "2026-09-03T12:00:00Z",
			"sync": {"pending": 0, "failed": 1, "synced": 12, "last_error": "rate limited"},
			"stale": true, "error": "Reconnect Salesforce to see live data"
		}],
		"can_sync": true
	}`
	c := respondingClient(t, &got, http.StatusOK, body)
	p, _, err := c.Salesforce.SyncContact(context.Background(), "ct_1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !p.CanSync || len(p.Connections) != 1 || len(p.Records) != 1 {
		t.Fatalf("panel = %+v", p)
	}
	r := p.Records[0]
	if r.LinkID != "lnk_1" || r.Owner == nil || r.Account == nil || !r.Stale || r.Sync.Failed != 1 {
		t.Errorf("record = %+v", r)
	}
	if len(r.Opportunities) != 1 || r.Opportunities[0].Amount == nil || *r.Opportunities[0].Amount != 12000 || !r.Tasks[0].FromWarmbly {
		t.Errorf("record children = %+v", r)
	}
}
