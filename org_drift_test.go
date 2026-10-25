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
)

// orgDriftReq is one recorded request.
type orgDriftReq struct {
	method, path, query, body string
}

// orgDriftClient serves canned responses keyed by "METHOD path" and records
// every request in order. A key missing from replies answers 200 with `{}`.
func orgDriftClient(t *testing.T, replies map[string]string) (*Client, *[]orgDriftReq) {
	t.Helper()
	var got []orgDriftReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got = append(got, orgDriftReq{r.Method, r.URL.Path, r.URL.RawQuery, string(raw)})
		key := r.Method + " " + r.URL.Path
		body, ok := replies[key]
		if !ok {
			body = `{}`
		}
		if strings.HasPrefix(body, "202:") {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if strings.HasPrefix(body, "204:") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if strings.HasPrefix(body, "sse:") {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, strings.TrimPrefix(body, "sse:"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"), WithMaxRetries(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c, &got
}

func orgDriftExpect(t *testing.T, got orgDriftReq, method, path, query, body string) {
	t.Helper()
	if got.method != method || got.path != "/v1/"+path || got.query != query {
		t.Fatalf("request = %s %s?%s, want %s /v1/%s?%s", got.method, got.path, got.query, method, path, query)
	}
	if body == "" {
		return
	}
	var want, have any
	if err := json.Unmarshal([]byte(body), &want); err != nil {
		t.Fatalf("bad expected body %q: %v", body, err)
	}
	if err := json.Unmarshal([]byte(got.body), &have); err != nil {
		t.Fatalf("body %q is not JSON: %v", got.body, err)
	}
	wb, _ := json.Marshal(want)
	hb, _ := json.Marshal(have)
	if string(wb) != string(hb) {
		t.Fatalf("body = %s, want %s", hb, wb)
	}
}

func TestOrgDriftOrganization(t *testing.T) {
	ctx := context.Background()
	const id = "11111111-1111-1111-1111-111111111111"
	c, got := orgDriftClient(t, map[string]string{
		"POST /v1/organization": `{"id":"` + id + `","name":"Acme","category":"test","timezone":"Europe/Berlin","owner_user_id":"u"}`,
		"GET /v1/organization/current": `{"id":"` + id + `","name":"Acme","category":"standard","timezone":"",
			"counts":{"total_campaigns":1,"active_campaigns":1,"total_contacts":2,"email_accounts":1,"emails_sent_today":0}}`,
		"GET /v1/organization/members/u1/access": `{"scope":"restricted","campaign_folder_ids":["f1"],"campaign_ids":["c1"],"email_account_ids":[]}`,
		"PUT /v1/organization/members/u1/access": `{"id":"m1","user_id":"u1","role":"viewer","access_scope":"restricted",
			"access":{"scope":"restricted","campaign_folder_ids":[],"campaign_ids":["c1"],"email_account_ids":["e1"]}}`,
		"GET /v1/organization/access/suggested-senders": `{"data":[{"id":"e1","email":"a@b.co","name":"A","campaign_ids":["c1"]}]}`,
		"GET /v1/organization/invitations": `{"data":[{"id":"i1","email":"x@y.co","role":"viewer","expires_at":"2099-01-01T00:00:00Z","expired":true,
			"access":{"scope":"workspace","campaign_folder_ids":[],"campaign_ids":[],"email_account_ids":[]}}]}`,
		"POST /v1/organization/members/invite": `{"invitation":{"id":"i2","email":"x@y.co","access":{"scope":"restricted"}}}`,
	})

	org, _, err := c.Organization.Create(ctx, &OrganizationCreateParams{Name: "Acme", Timezone: "Europe/Berlin"})
	if err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[0], "POST", "organization", "", `{"name":"Acme","timezone":"Europe/Berlin"}`)
	if org.Category != "test" || org.Timezone != "Europe/Berlin" {
		t.Fatalf("org = %+v", org)
	}

	tz := ""
	if _, _, err := c.Organization.Update(ctx, &OrganizationUpdateParams{Timezone: &tz}); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[1], "PATCH", "organization/current", "", `{"timezone":""}`)

	cur, _, err := c.Organization.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cur.Category != OrganizationCategoryStandard || cur.Counts.TotalContacts != 2 || cur.Counts.TotalMembers != 0 {
		t.Fatalf("current = %+v", cur)
	}

	acc, _, err := c.Organization.MemberAccess(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[3], "GET", "organization/members/u1/access", "", "")
	if acc.Scope != AccessScopeRestricted || len(acc.CampaignFolderIDs) != 1 || acc.CampaignIDs[0] != "c1" {
		t.Fatalf("access = %+v", acc)
	}

	m, _, err := c.Organization.SetMemberAccess(ctx, "u1", &SetMemberAccessParams{
		Scope: AccessScopeRestricted, CampaignIDs: []string{"c1"}, EmailAccountIDs: []string{"e1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[4], "PUT", "organization/members/u1/access", "",
		`{"scope":"restricted","campaign_folder_ids":null,"campaign_ids":["c1"],"email_account_ids":["e1"]}`)
	if !m.IsRestricted() || m.Access == nil || m.Access.EmailAccountIDs[0] != "e1" {
		t.Fatalf("member = %+v", m)
	}

	senders, _, err := c.Organization.SuggestedSenders(ctx, []string{"c1", "c2"}, []string{"f1"})
	if err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[5], "GET", "organization/access/suggested-senders", "campaign_folder_ids=f1&campaign_ids=c1%2Cc2", "")
	if len(senders) != 1 || senders[0].Email != "a@b.co" || senders[0].CampaignIDs[0] != "c1" {
		t.Fatalf("senders = %+v", senders)
	}

	invs, _, err := c.Organization.ListInvitations(ctx, &InvitationListParams{IncludeExpired: true})
	if err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[6], "GET", "organization/invitations", "include_expired=true", "")
	if len(invs) != 1 || !invs[0].Expired() || invs[0].Access == nil || invs[0].Access.Scope != AccessScopeWorkspace {
		t.Fatalf("invitations = %+v", invs)
	}

	if _, _, err := c.Organization.Invite(ctx, &InviteMemberParams{
		Email: "x@y.co", RoleIDs: []string{"r1"}, Access: &MemberAccess{Scope: AccessScopeRestricted, CampaignIDs: []string{"c1"}},
	}); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[7], "POST", "organization/members/invite", "",
		`{"email":"x@y.co","role_ids":["r1"],"access":{"scope":"restricted","campaign_folder_ids":null,"campaign_ids":["c1"],"email_account_ids":null}}`)
}

func TestOrgDriftBillingAndMeta(t *testing.T) {
	ctx := context.Background()
	c, got := orgDriftClient(t, map[string]string{
		"GET /v1/subscription": `{"id":"s","status":"incomplete","managed":true,"managed_reason":"design partner",
			"managed_until":"2099-01-01T00:00:00Z","managed_plan_id":"p2","plan_id":"p1",
			"plan":{"id":"p2","price":29,"price_yearly":290.5}}`,
		"POST /v1/getaway": `{"url":"wss://rt.example/socket/websocket?token=t","expires_in":300,"proxy_path":"/realtime/socket/websocket?token=t"}`,
		"GET /v1/me/views/contacts": `{"preferences":{"view":"contacts","columns":["name","custom:Industry"],
			"sort":{"by":"created_at","reverse":true},"updated_at":"2026-01-02T03:04:05Z"}}`,
		"PUT /v1/me/views/unibox_rail": `{"preferences":{"view":"unibox_rail","columns":[],"layout":{"hidden":[]}}}`,
		"DELETE /v1/me/views/contacts": `204:`,
	})

	sub, _, err := c.Billing.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !sub.Managed || sub.ManagedReason == nil || *sub.ManagedReason != "design partner" ||
		sub.ManagedUntil == nil || sub.ManagedPlanID == nil || *sub.ManagedPlanID != "p2" {
		t.Fatalf("subscription = %+v", sub)
	}
	if sub.Plan == nil || sub.Plan.PriceYearly == nil || *sub.Plan.PriceYearly != 290.5 {
		t.Fatalf("plan = %+v", sub.Plan)
	}

	tk, _, err := c.Meta.GatewayTicket(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if tk.ProxyPath != "/realtime/socket/websocket?token=t" {
		t.Fatalf("ticket = %+v", tk)
	}

	vp, _, err := c.Meta.ViewPreferences(ctx, ViewContacts)
	if err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[2], "GET", "me/views/contacts", "", "")
	if vp.Sort == nil || !vp.Sort.Reverse || len(vp.Columns) != 2 || vp.UpdatedAt == nil {
		t.Fatalf("view = %+v", vp)
	}

	cols := []string{}
	if _, _, err := c.Meta.UpdateViewPreferences(ctx, ViewUniboxRail, &ViewPreferencesParams{Columns: &cols, Layout: json.RawMessage("null")}); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[3], "PUT", "me/views/unibox_rail", "", `{"columns":[],"layout":null}`)

	if _, err := c.Meta.ResetViewPreferences(ctx, ViewContacts); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[4], "DELETE", "me/views/contacts", "", "")
}

func TestOrgDriftAssistant(t *testing.T) {
	ctx := context.Background()
	c, got := orgDriftClient(t, map[string]string{
		"GET /v1/ai/sessions/s1/messages": `{"title":"t","turns":[],"free_model":false,"pending":{"message_id":"m","tool_call_id":"call_9",
			"tool_name":"send_email","risk":"send","args":{"to":"a@b.co"},"arguments":"{\n  \"to\": \"a@b.co\"\n}",
			"preview":{"from":"me@x.co","to":["a@b.co"],"subject":"Hi","body":"Hello"},"always_allow_offered":true}}`,
		"POST /v1/ai/sessions/s1/approve":  "sse:data: {\"type\":\"tool_result\",\"tool\":\"send_email\"}\n\ndata: {\"type\":\"done\"}\n\n",
		"POST /v1/ai/sessions/s2/messages": "sse:data: {\"type\":\"tool_secret\",\"secrets\":{\"api_key\":\"k\"}}\n\n",
		"GET /v1/ai/sessions/s3/messages":  `{"title":"t","turns":[],"free_model":false}`,
		"GET /v1/ai/tool-policies": `{"data":[{"org_id":"o","tool_name":"create_label","decision":"always_allow","created_by":"u",
			"created_by_name":"Ann","created_at":"2026-01-01T00:00:00Z"}]}`,
		"DELETE /v1/ai/tool-policies/create_label": `{"message":"policy revoked"}`,
	})

	var events []string
	if _, err := c.Assistant.Approve(ctx, "s1", ApprovalApprove, func(ev *AgentEvent) bool {
		events = append(events, ev.Type)
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if len(*got) != 2 {
		t.Fatalf("requests = %d, want transcript read then approve", len(*got))
	}
	orgDriftExpect(t, (*got)[0], "GET", "ai/sessions/s1/messages", "", "")
	orgDriftExpect(t, (*got)[1], "POST", "ai/sessions/s1/approve", "", `{"decision":"approve","tool_call_id":"call_9"}`)
	if len(events) != 2 || events[0] != AgentEventToolResult || events[1] != AgentEventDone {
		t.Fatalf("events = %v", events)
	}

	// The pending card carries the new fields.
	tr, _, err := c.Assistant.Transcript(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	p := tr.Pending
	if p == nil || p.Preview == nil || p.Preview.Subject != "Hi" || !p.AlwaysAllowOffered || !strings.Contains(p.Arguments, "a@b.co") {
		t.Fatalf("pending = %+v", p)
	}

	// With the id in hand there is no transcript round trip.
	*got = (*got)[:0]
	if _, err := c.Assistant.ApproveToolCall(ctx, "s2", "call_1", ApprovalDeny, nil); err != nil {
		t.Fatal(err)
	}
	if len(*got) != 1 {
		t.Fatalf("ApproveToolCall made %d requests", len(*got))
	}
	orgDriftExpect(t, (*got)[0], "POST", "ai/sessions/s2/approve", "", `{"decision":"deny","tool_call_id":"call_1"}`)

	// Nothing pending: the error is typed and nothing is posted.
	*got = (*got)[:0]
	_, err = c.Assistant.Approve(ctx, "s3", ApprovalApprove, nil)
	if !errors.Is(err, ErrNoPendingApproval) {
		t.Fatalf("err = %v, want ErrNoPendingApproval", err)
	}
	if len(*got) != 1 {
		t.Fatalf("requests = %d", len(*got))
	}

	var secret string
	if _, err := c.Assistant.SendMessage(ctx, "s2", &MessageParams{Text: "go"}, func(ev *AgentEvent) bool {
		if ev.Type == AgentEventToolSecret {
			secret = ev.Secrets["api_key"]
		}
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if secret != "k" {
		t.Fatalf("secret = %q", secret)
	}

	pol, _, err := c.Assistant.ToolPolicies(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pol) != 1 || pol[0].ToolName != "create_label" || pol[0].CreatedByName != "Ann" {
		t.Fatalf("policies = %+v", pol)
	}
	if _, err := c.Assistant.RevokeToolPolicy(ctx, "create_label"); err != nil {
		t.Fatal(err)
	}
	last := (*got)[len(*got)-1]
	orgDriftExpect(t, last, "DELETE", "ai/tool-policies/create_label", "", "")
}

func TestOrgDriftLeadSync(t *testing.T) {
	ctx := context.Background()
	c, got := orgDriftClient(t, map[string]string{
		"GET /v1/lead-sync/sources": `{"data":[{"id":"l1","segment_ids":["seg1"],"category_ids":[],"has_header":true}]}`,
	})
	src, _, err := c.LeadSync.SourcesForSegment(ctx, "seg1")
	if err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[0], "GET", "lead-sync/sources", "segment_id=seg1", "")
	if len(src) != 1 || len(src[0].SegmentIDs) != 1 || src[0].SegmentIDs[0] != "seg1" {
		t.Fatalf("sources = %+v", src)
	}
	segs := []string{}
	if _, _, err := c.LeadSync.Update(ctx, "l1", &LeadSyncUpdateParams{SegmentIDs: &segs}); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[1], "PATCH", "lead-sync/sources/l1", "", `{"segment_ids":[]}`)
	if _, _, err := c.LeadSync.Create(ctx, &LeadSyncCreateParams{ConnectionID: "c", SheetID: "s", SegmentIDs: []string{"seg1"}}); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[2], "POST", "lead-sync/sources", "", `{"connection_id":"c","sheet_id":"s","has_header":false,"column_mapping":null,"segment_ids":["seg1"]}`)
}

func TestOrgDriftCRM(t *testing.T) {
	ctx := context.Background()
	c, got := orgDriftClient(t, map[string]string{
		"PATCH /v1/crm/tasks":  `{"affected":3}`,
		"DELETE /v1/crm/tasks": `{"affected":2}`,
		"GET /v1/crm/settings": `{"organization_id":"o","provider":"hubspot","connection_id":"c1","setup_completed_at":"2026-01-01T00:00:00Z",
			"updated_at":"2026-01-02T00:00:00Z","account":{"external_id":"123","name":"Acme","app_url":"https://app.hubspot.com/x","status":"connected","health":"healthy","missing_scopes":["crm.objects.deals.read"]},
			"config":{"activity":{"sent":true,"replies":true,"bounces":true,"unsubscribes":true,"opens":false,"clicks":false,"meetings":true},
			"create_contacts":true,"create_companies":true,"write_properties":true,
			"positive_reply":{"lead_status":"IN_PROGRESS","lifecycle_stage":"","create_deal":true,"deal_pipeline_id":"p1","deal_stage_id":"st1"},
			"exit_rules":{"deal_created":true,"lifecycle_stages":["customer"],"opted_out":true},
			"guards":{"skip_lifecycle_stages":[],"skip_open_deals":true,"skip_other_owners":false,"skip_opted_out":true},
			"deal_pipelines":[],"display_properties":["jobtitle"],"field_map":{"first_name":"firstname"},"field_direction":{"first_name":"both"},"for":"hubspot"}}`,
		"PUT /v1/crm/settings":      `{"organization_id":"o","provider":"pipedrive","config":{"activity":{}},"updated_at":"2026-01-02T00:00:00Z"}`,
		"GET /v1/crm/owners":        `{"data":[{"external_id":"o1","email":"a@b.co","first_name":"A","last_name":"B","user_id":"u1","user_pinned":true,"archived":false}]}`,
		"PUT /v1/crm/owners/o1":     `204:`,
		"GET /v1/crm/sync":          `{"pending":1,"failed":2,"done_24h":3,"cursors":[{"object_type":"deals","last_run_at":"2026-01-01T00:00:00Z"}],"failures":[{"id":"j1","organization_id":"o","provider":"hubspot","kind":"push_deal","subject":"d","status":"failed","attempts":3,"next_attempt_at":"2026-01-01T00:00:00Z","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}],"counts":{"contacts":1,"deals":2,"tasks":3,"pipelines":4,"owners":5}}`,
		"POST /v1/crm/sync":         `202:`,
		"POST /v1/crm/sync/retry":   `{"affected":4}`,
		"POST /v1/crm/sync/discard": `{"affected":1}`,
		"GET /v1/crm/backfill":      `{"deals":1,"tasks":2,"notes":3}`,
		"POST /v1/crm/backfill":     `202:`,
		"GET /v1/crm/contacts/k1":   `{"provider":"hubspot","linked":true,"external_id":"77","owner":{"external_id":"o1","email":"a@b.co","first_name":"","last_name":"","user_pinned":false,"archived":false},"lifecycle_stage":{"value":"lead","label":"Lead"},"opted_out":false,"properties":[{"name":"jobtitle","label":"Job title","value":"CTO"}]}`,
		"PATCH /v1/crm/contacts/k1": `{"provider":"hubspot","linked":true,"opted_out":false,"properties":[]}`,
		"GET /v1/crm/lists":         `{"data":[{"external_id":"9","name":"Hot","size":10,"dynamic":true}],"pagination":{"has_more":false,"next_cursor":null}}`,
		"POST /v1/crm/lists/import": `{"import_id":"imp1","preview":{"list_name":"Hot","total":10,"included":8,"skipped":[{"reason":"opted_out","label":"Opted out","count":2}],"sample":[],"truncated":false}}`,
		"GET /v1/tasks-ext":         `{}`,
	})

	st := TaskStatusCompleted
	n, _, err := c.CRM.BulkUpdateTasks(ctx, &BulkTaskUpdateParams{TaskSelection: TaskSelection{Tasks: []string{"t1", "t2"}}, Status: &st})
	if err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[0], "PATCH", "crm/tasks", "", `{"tasks":["t1","t2"],"status":"completed"}`)
	if n.Affected != 3 {
		t.Fatalf("affected = %d", n.Affected)
	}

	del, _, err := c.CRM.BulkDeleteTasks(ctx, &TaskSelection{All: true, Filters: &TaskSearchParams{Overdue: true}, Exclude: []string{"t9"}})
	if err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[1], "DELETE", "crm/tasks", "", `{"tasks":null,"all":true,"filters":{"overdue":true},"exclude":["t9"]}`)
	if del.Affected != 2 {
		t.Fatalf("affected = %d", del.Affected)
	}

	set, _, err := c.CRM.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if set.Provider != CRMProviderHubSpot || set.Account == nil || set.Account.MissingScopes[0] != "crm.objects.deals.read" ||
		!set.Config.PositiveReply.CreateDeal || *set.Config.PositiveReply.DealPipelineID != "p1" ||
		set.Config.FieldDirection["first_name"] != CRMFieldBoth || set.SetupCompletedAt == nil {
		t.Fatalf("settings = %+v", set)
	}

	prov := CRMProviderPipedrive
	if _, _, err := c.CRM.UpdateSettings(ctx, &CRMSettingsUpdateParams{Provider: &prov, CompleteSetup: true}); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[3], "PUT", "crm/settings", "", `{"provider":"pipedrive","complete_setup":true}`)

	owners, _, err := c.CRM.Owners(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(owners) != 1 || !owners[0].UserPinned || *owners[0].UserID != "u1" {
		t.Fatalf("owners = %+v", owners)
	}
	if _, err := c.CRM.MapOwner(ctx, "o1", nil); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[5], "PUT", "crm/owners/o1", "", `{"user_id":null}`)

	h, _, err := c.CRM.SyncHealth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if h.Failed != 2 || h.Counts.Pipelines != 4 || h.Failures[0].Kind != "push_deal" || h.Cursors[0].LastRunAt == nil {
		t.Fatalf("health = %+v", h)
	}
	if _, err := c.CRM.SyncNow(ctx); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[7], "POST", "crm/sync", "", "")

	r, _, err := c.CRM.RetrySyncFailures(ctx, []string{"j1"})
	if err != nil || r != 4 {
		t.Fatalf("retry = %d, %v", r, err)
	}
	orgDriftExpect(t, (*got)[8], "POST", "crm/sync/retry", "", `{"ids":["j1"]}`)
	d, _, err := c.CRM.DiscardSyncFailures(ctx, nil)
	if err != nil || d != 1 {
		t.Fatalf("discard = %d, %v", d, err)
	}
	orgDriftExpect(t, (*got)[9], "POST", "crm/sync/discard", "", `{"ids":null}`)

	bf, _, err := c.CRM.BackfillPreview(ctx)
	if err != nil || bf.Notes != 3 {
		t.Fatalf("backfill = %+v, %v", bf, err)
	}
	if _, err := c.CRM.StartBackfill(ctx, &CRMBackfillParams{Deals: true, Notes: true}); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[11], "POST", "crm/backfill", "", `{"deals":true,"tasks":false,"notes":true}`)

	cv, _, err := c.CRM.Contact(ctx, "k1")
	if err != nil {
		t.Fatal(err)
	}
	if !cv.Linked || cv.LifecycleStage == nil || cv.LifecycleStage.Label != "Lead" || cv.Properties[0].Value != "CTO" {
		t.Fatalf("contact = %+v", cv)
	}
	stage := "customer"
	if _, _, err := c.CRM.UpdateContact(ctx, "k1", &CRMContactUpdateParams{LifecycleStage: &stage}); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[13], "PATCH", "crm/contacts/k1", "", `{"lifecycle_stage":"customer"}`)

	page, err := c.CRM.Lists(ctx, &CRMListParams{ListOptions: ListOptions{Limit: 10}, Query: "hot"})
	if err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[14], "GET", "crm/lists", "limit=10&q=hot", "")
	if len(page.Data) != 1 || !page.Data[0].Dynamic {
		t.Fatalf("lists = %+v", page.Data)
	}
	imp, _, err := c.CRM.ImportList(ctx, &CRMImportParams{ListID: "9", ApplyGuards: true})
	if err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[15], "POST", "crm/lists/import", "", `{"list_id":"9","apply_guards":true}`)
	if imp.ImportID != "imp1" || imp.Preview.Skipped[0].Count != 2 {
		t.Fatalf("import = %+v", imp)
	}
}

func TestOrgDriftIntegrations(t *testing.T) {
	ctx := context.Background()
	c, got := orgDriftClient(t, map[string]string{
		"POST /v1/integrations/oauth/start":                       `{"url":"https://login.salesforce.com/x","state":"st"}`,
		"POST /v1/integrations/connections/c1/push":               `{"provider":"hubspot","pushed":1,"failed":0,"results":[]}`,
		"PUT /v1/integrations/connections/c1/signing-key":         `{"connection":{"id":"c1","provider":"calendly"}}`,
		"POST /v1/integrations/connections/c1/rotate-inbound-url": `{"inbound_webhook_url":"https://api.example/inbound/calendly/abc"}`,
		"GET /v1/integrations/catalog":                            `{"catalog":[{"provider":"apollo","name":"Apollo","auth_method":"api_key","rank":2,"supports_push":false,"configured":true,"beta":false}]}`,
		"GET /v1/integrations/community": `{"data":[{"application_id":"a1","slug":"acme","name":"Acme","category":"ai","scopes":3,
			"permissions":[{"name":"READ_EMAILS","value":1,"description":"d","category":"read"}],"status":"featured","listed":true,"installs":4,"installed":false,"published_at":"2026-01-01T00:00:00Z"}],
			"pagination":{"total":1,"has_more":false,"next_cursor":null}}`,
		"GET /v1/integrations/salesforce/c2/overview": `{"connection_id":"c2","label":"SF","status":"connected","health":"healthy",
			"org":{"instance_url":"https://x.my.salesforce.com","environment":"sandbox","login_host":"test.salesforce.com"},
			"api":{"used":1,"max":100,"calls_today":5,"budget":20},"counts":{"pending":1,"synced_24h":2,"failed":3,"skipped_24h":4,"linked_records":5},
			"settings_enabled":true,"checks":[{"key":"k","label":"L","ok":false,"detail":"x"}]}`,
		"GET /v1/integrations/salesforce/c2/settings": `{"settings":{"enabled":true,"matching":{"prefer":"contact","create_when":"reply","create_as":"lead","lead_source":"Warmbly","lead_status":"","owner":"connected_user","run_assignment_rules":false},
			"activity":{"sent":true,"replied":true,"opened":false,"clicked":false,"bounced":true,"unsubscribed":true,"meeting_booked":true,"include_body":true,"assign_to":"record_owner","relate_to_opportunity":true},
			"writeback":{"lead_status_on_sent":"","lead_status_on_reply":{},"lead_status_on_meeting":"","never_move_backwards":true},
			"inbound":{"opt_out":"both","pause_on_converted":false,"pause_on_statuses":[],"pause_on_open_opportunity":false},
			"field_map":[{"object":"Lead","warmbly":"first_name","salesforce":"FirstName","direction":"both","policy":"if_empty"}],"daily_api_budget":0},
			"warmbly_fields":[{"key":"first_name","label":"First name"}],"defaults":{"enabled":true}}`,
		"PUT /v1/integrations/salesforce/c2/settings":               `{"settings":{"enabled":false}}`,
		"POST /v1/integrations/salesforce/c2/import-sources":        `{"id":"s1","connection_id":"c2","source_kind":"list_view","object":"Lead","source_id":"00B000000000001","source_label":"All","category_ids":[],"recurring":false,"enabled":true,"status":"running","total_imported":0}`,
		"PATCH /v1/integrations/salesforce/c2/import-sources/s1":    `{"id":"s1","category_ids":[]}`,
		"POST /v1/integrations/salesforce/c2/import-sources/s1/run": `{"id":"s1","last_result":{"read":10,"imported":5,"opted_out":1,"no_email":2},"category_ids":[]}`,
		"DELETE /v1/integrations/salesforce/c2/import-sources/s1":   `204:`,
		"GET /v1/integrations/salesforce/c2/activity":               `{"data":[{"id":"a1","kind":"replied","status":"failed","contact_email":"a@b.co","attempts":2,"next_attempt_at":"2026-01-01T00:00:00Z","occurred_at":"2026-01-01T00:00:00Z","created_at":"2026-01-01T00:00:00Z"}],"pagination":{"next_cursor":"nx","has_more":true}}`,
		"POST /v1/integrations/salesforce/c2/activity/retry":        `{"requeued":2}`,
		"POST /v1/integrations/salesforce/c2/sync-now":              `{"ok":true}`,
		"GET /v1/integrations/apollo/c3":                            `{"connection_id":"c3","label":"A","status":"connected","health":"healthy","account_email":"me@x.co","sources":1,"total_imported":9}`,
		"PUT /v1/integrations/apollo/c3/key":                        `{"connection_id":"c3","label":"A","status":"connected","health":"healthy","sources":0,"total_imported":0}`,
		"POST /v1/integrations/apollo/c3/enrichments":               `{"id":"r1","connection_id":"c3","status":"queued","total":2,"processed":0,"enriched":0,"not_found":0,"failed":0,"credits_consumed":0,"overwrite":true,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
		"GET /v1/integrations/apollo/c3/enrichments":                `{"data":[]}`,
		"POST /v1/integrations/apollo/c3/import-sources":            `{"id":"as1","list_id":"l1","verified_only":true,"category_ids":[]}`,
		"PATCH /v1/integrations/apollo/c3/import-sources/as1":       `{"id":"as1","category_ids":[]}`,
		"PATCH /v1/integrations/clay/c4": `{"connection_id":"c4","label":"Clay","status":"connected","health":"healthy","inbound_url_minted":true,
			"defaults":{"category_ids":[],"segment_ids":["seg"],"dedup":"skip","subscribed":true},
			"outbound":{"webhook_url_set":true,"webhook_host":"app.clay.com","auth_token_set":false,"events":["reply.received"]},
			"stats":{"received":4,"created":2,"updated":1,"skipped":1,"failed":0},"available_events":["reply.received","email.sent"]}`,
		"GET /v1/integrations/slack/status": `{"app_configured":true,"interactive_configured":true,"missing_scopes":["chat:write"],
			"settings":{"channel":"C1","routes":{"replies":"C2"},"inbox_scope":"all"},
			"my_link":{"id":"l1","organization_id":"o","connection_id":"c","slack_team_id":"T","slack_user_id":"U","user_id":"u","dm_notifications":true,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"},"links":[]}`,
		"GET /v1/integrations/slack/channels":     `{"data":[{"id":"C1","name":"general","is_private":false,"is_member":true}],"pagination":{"next_cursor":null,"has_more":false}}`,
		"PUT /v1/integrations/slack/settings":     `{"channel":"C1"}`,
		"POST /v1/integrations/slack/link":        `{"id":"l1","dm_notifications":false}`,
		"POST /v1/integrations/slack/link/verify": `{"url":"https://slack.com/openid"}`,
		"PATCH /v1/integrations/slack/link":       `{"id":"l1","dm_notifications":false}`,
		"DELETE /v1/integrations/slack/link":      `204:`,
		"DELETE /v1/integrations/slack/links/l9":  `204:`,
	})

	if _, _, err := c.Integrations.StartOAuthWith(ctx, &IntegrationOAuthStartParams{Provider: ProviderSalesforce, Environment: SalesforceSandbox, Domain: "acme.my.salesforce.com"}); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[0], "POST", "integrations/oauth/start", "", `{"provider":"salesforce","environment":"sandbox","domain":"acme.my.salesforce.com"}`)

	if _, _, err := c.Integrations.PushSelection(ctx, "c1", &ContactSelection{All: true, Exclude: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[1], "POST", "integrations/connections/c1/push", "", `{"all":true,"exclude":["x"]}`)

	conn, _, err := c.Integrations.SetInboundSigningKey(ctx, "c1", "")
	if err != nil || conn.ID != "c1" {
		t.Fatalf("signing key = %+v, %v", conn, err)
	}
	orgDriftExpect(t, (*got)[2], "PUT", "integrations/connections/c1/signing-key", "", `{"signing_key":""}`)
	u, _, err := c.Integrations.RotateInboundURL(ctx, "c1")
	if err != nil || !strings.HasSuffix(u, "/abc") {
		t.Fatalf("rotate = %q, %v", u, err)
	}
	orgDriftExpect(t, (*got)[3], "POST", "integrations/connections/c1/rotate-inbound-url", "", "")

	cat, _, err := c.Integrations.Catalog(ctx)
	if err != nil || cat[0].Rank != 2 || cat[0].Provider != ProviderApollo {
		t.Fatalf("catalog = %+v, %v", cat, err)
	}

	apps, err := c.Integrations.CommunityApps(ctx, &ListOptions{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[5], "GET", "integrations/community", "limit=5", "")
	if len(apps.Data) != 1 || apps.Data[0].Permissions[0].Name != "READ_EMAILS" || apps.Data[0].Status != CommunityFeatured || !apps.Data[0].Listed {
		t.Fatalf("apps = %+v", apps.Data)
	}

	ov, _, err := c.Integrations.SalesforceOverview(ctx, "c2", true)
	if err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[6], "GET", "integrations/salesforce/c2/overview", "checks=1", "")
	if ov.Org.Environment != SalesforceSandbox || ov.API.Budget != 20 || ov.Counts.LinkedTotal != 5 || len(ov.Checks) != 1 || ov.Checks[0].OK {
		t.Fatalf("overview = %+v", ov)
	}

	doc, _, err := c.Integrations.SalesforceSettings(ctx, "c2")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Settings.FieldMap[0].Salesforce != "FirstName" || doc.Settings.Writeback.LeadStatusOnReply == nil || doc.WarmblyFields[0].Key != "first_name" {
		t.Fatalf("settings = %+v", doc)
	}
	saved, _, err := c.Integrations.UpdateSalesforceSettings(ctx, "c2", &SalesforceSettings{Enabled: false})
	if err != nil || saved.Enabled {
		t.Fatalf("saved = %+v, %v", saved, err)
	}
	if (*got)[8].method != "PUT" || (*got)[8].path != "/v1/integrations/salesforce/c2/settings" {
		t.Fatalf("settings request = %+v", (*got)[8])
	}

	cid := "camp1"
	if _, _, err := c.Integrations.CreateSalesforceImportSource(ctx, "c2", &SalesforceImportSourceParams{
		SourceKind: SalesforceSourceListView, Object: "Lead", SourceID: "00B000000000001", CampaignID: &cid,
	}); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[9], "POST", "integrations/salesforce/c2/import-sources", "",
		`{"source_kind":"list_view","object":"Lead","source_id":"00B000000000001","campaign_id":"camp1"}`)
	if _, _, err := c.Integrations.UpdateSalesforceImportSource(ctx, "c2", "s1", &SalesforceImportSourceParams{ClearCampaign: true, CampaignID: &cid}); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[10], "PATCH", "integrations/salesforce/c2/import-sources/s1", "", `{"campaign_id":null}`)
	run, _, err := c.Integrations.RunSalesforceImportSource(ctx, "c2", "s1")
	if err != nil || run.LastResult == nil || run.LastResult.OptedOut != 1 || run.LastResult.NoEmail != 2 {
		t.Fatalf("run = %+v, %v", run, err)
	}
	if _, err := c.Integrations.DeleteSalesforceImportSource(ctx, "c2", "s1"); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[12], "DELETE", "integrations/salesforce/c2/import-sources/s1", "", "")

	act, err := c.Integrations.SalesforceActivity(ctx, "c2", &SalesforceActivityParams{ListOptions: ListOptions{Limit: 50, Cursor: "cc"}, Status: SalesforceActivityFailed, ContactID: "k1"})
	if err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[13], "GET", "integrations/salesforce/c2/activity", "contact_id=k1&cursor=cc&limit=50&status=failed", "")
	if len(act.Data) != 1 || act.Data[0].Kind != "replied" || !act.HasMore() {
		t.Fatalf("activity = %+v", act)
	}
	n, _, err := c.Integrations.RetrySalesforceActivity(ctx, "c2", []string{"a1"})
	if err != nil || n != 2 {
		t.Fatalf("retry = %d, %v", n, err)
	}
	orgDriftExpect(t, (*got)[14], "POST", "integrations/salesforce/c2/activity/retry", "", `{"ids":["a1"]}`)
	if _, err := c.Integrations.SalesforceSyncNow(ctx, "c2"); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[15], "POST", "integrations/salesforce/c2/sync-now", "", "")

	aov, _, err := c.Integrations.ApolloOverview(ctx, "c3")
	if err != nil || aov.TotalImported != 9 || aov.AccountEmail != "me@x.co" {
		t.Fatalf("apollo = %+v, %v", aov, err)
	}
	orgDriftExpect(t, (*got)[16], "GET", "integrations/apollo/c3", "", "")
	if _, _, err := c.Integrations.ReplaceApolloKey(ctx, "c3", "k"); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[17], "PUT", "integrations/apollo/c3/key", "", `{"api_key":"k"}`)
	run2, _, err := c.Integrations.StartApolloEnrichment(ctx, "c3", &ApolloEnrichParams{ContactSelection: ContactSelection{Contacts: []string{"k1", "k2"}}, Overwrite: true})
	if err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[18], "POST", "integrations/apollo/c3/enrichments", "", `{"contacts":["k1","k2"],"overwrite":true}`)
	if run2.Status != ApolloEnrichmentQueued || !run2.Overwrite {
		t.Fatalf("run = %+v", run2)
	}
	if _, _, err := c.Integrations.ApolloEnrichments(ctx, "c3", 5); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[19], "GET", "integrations/apollo/c3/enrichments", "limit=5", "")
	vo := true
	if _, _, err := c.Integrations.CreateApolloImportSource(ctx, "c3", &ApolloImportSourceParams{ListID: "l1", VerifiedOnly: &vo}); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[20], "POST", "integrations/apollo/c3/import-sources", "", `{"list_id":"l1","verified_only":true}`)
	if _, _, err := c.Integrations.UpdateApolloImportSource(ctx, "c3", "as1", &ApolloImportSourceParams{ClearCampaign: true}); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[21], "PATCH", "integrations/apollo/c3/import-sources/as1", "", `{"campaign_id":null}`)

	wh := "https://app.clay.com/hook"
	clay, _, err := c.Integrations.UpdateClaySettings(ctx, "c4", &ClaySettingsParams{WebhookURL: &wh})
	if err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[22], "PATCH", "integrations/clay/c4", "", `{"defaults":null,"webhook_url":"https://app.clay.com/hook","auth_token":null,"events":null}`)
	if !clay.InboundURLMinted || clay.Defaults.SegmentIDs[0] != "seg" || !clay.Outbound.WebhookURLSet || clay.Stats.Created != 2 || len(clay.AvailableEvents) != 2 {
		t.Fatalf("clay = %+v", clay)
	}

	sl, _, err := c.Integrations.SlackStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !sl.InteractiveConfigured || sl.Settings.Routes["replies"] != "C2" || sl.MyLink == nil || !sl.MyLink.DMNotifications || sl.MissingScopes[0] != "chat:write" {
		t.Fatalf("slack = %+v", sl)
	}
	ch, _, err := c.Integrations.SlackChannels(ctx, "gen")
	if err != nil || ch[0].Name != "general" {
		t.Fatalf("channels = %+v, %v", ch, err)
	}
	orgDriftExpect(t, (*got)[24], "GET", "integrations/slack/channels", "q=gen", "")
	if _, _, err := c.Integrations.UpdateSlackSettings(ctx, &SlackSettings{Channel: "C1", InboxScope: SlackInboxAll}); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[25], "PUT", "integrations/slack/settings", "", `{"channel":"C1","inbox_scope":"all"}`)
	if _, _, err := c.Integrations.ConfirmSlackLink(ctx, "code", "", ""); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[26], "POST", "integrations/slack/link", "", `{"code":"code"}`)
	vurl, _, err := c.Integrations.StartSlackLinkVerify(ctx, "code")
	if err != nil || vurl == "" {
		t.Fatalf("verify = %q, %v", vurl, err)
	}
	if _, _, err := c.Integrations.UpdateMySlackLink(ctx, false); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[28], "PATCH", "integrations/slack/link", "", `{"dm_notifications":false}`)
	if _, err := c.Integrations.DeleteMySlackLink(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Integrations.RemoveSlackLink(ctx, "l9"); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[30], "DELETE", "integrations/slack/links/l9", "", "")
}

func TestOrgDriftPoolAndCloudLink(t *testing.T) {
	ctx := context.Background()
	c, got := orgDriftClient(t, map[string]string{
		"POST /v1/pool-link/codes":    `{"device_code":"d","user_code":"U","verification_url":"https://x","expires_in":900,"interval":5,"workspace_scoped":true}`,
		"GET /v1/pool-link/offer":     `{"available":true,"monthly_available":true,"yearly_available":false,"monthly_usd":19,"yearly_usd":0,"currency":"usd"}`,
		"POST /v1/pool-link/checkout": `{"checkout_url":"https://stripe/x"}`,
		"GET /v1/pool-link/instances": `{"data":[{"id":"i","remote_organization_id":"ro","mailbox_count":2}],"plan":{"tier":"free","mailbox_limit":5,"enrolled":2,"warming":1,"price_usd":19,"manage_url":"https://m","warmup_entitled":true}}`,
		"GET /v1/cloud-link": `{"legacy_connected":true,"connected":true,"reachable":true,"default_cloud_url":"https://c",
			"link":{"organization_id":"o","cloud_url":"https://c","instance_id":"i","organization_name":"N","connected_at":"2026-01-01T00:00:00Z","disconnect_pending":true},
			"info":{"smtp_association_protocol":1,"managed_consent_protocol":1,"instance":{"id":"i","remote_organization_id":"ro","mailbox_count":0},"organization":{"id":"o","name":"N"},
			"plan":{"tier":"paid","mailbox_limit":null,"enrolled":0,"warming":0,"price_usd":0,"warmup_entitled":true},"redirects":{"available":true,"host":"r.example","limit":3,"used":1}}}`,
		"DELETE /v1/cloud-link": `204:`,
		"PATCH /v1/cloud-link/mailboxes/m1/participation": `{"id":"m1","email":"a@b.co","independent_smtp":true,"legacy":false,"enrolled":true,"managed":false,"enrollment_state":"active","standing_observed_at":"2026-01-01T00:00:00Z",
			"cloud":{"remote_id":"m1","participation":{"mode":"diagnostic","send":true,"receive":false,"shared_daily_limit":5},"consent_completed_at":"2026-01-01T00:00:00Z"}}`,
		"GET /v1/cloud-link/workspace-mailboxes": `{"data":[{"id":"w1","email":"a@b.co","provider":"smtp_imap","mail_host":"smtp.b.co"}]}`,
	})

	g, _, err := c.PoolLink.StartCode(ctx, &PoolLinkStartParams{RemoteOrganizationID: "ro", InstanceName: "n", InstanceURL: "https://i", InstanceVersion: "1"})
	if err != nil || !g.WorkspaceScoped {
		t.Fatalf("start = %+v, %v", g, err)
	}
	orgDriftExpect(t, (*got)[0], "POST", "pool-link/codes", "", `{"remote_organization_id":"ro","instance_name":"n","instance_url":"https://i","instance_version":"1"}`)

	o, _, err := c.PoolLink.Offer(ctx)
	if err != nil || !o.MonthlyAvailable || o.YearlyAvailable || o.MonthlyUSD != 19 {
		t.Fatalf("offer = %+v, %v", o, err)
	}
	orgDriftExpect(t, (*got)[1], "GET", "pool-link/offer", "", "")
	url, _, err := c.PoolLink.Checkout(ctx, PoolLinkIntervalYear)
	if err != nil || url != "https://stripe/x" {
		t.Fatalf("checkout = %q, %v", url, err)
	}
	orgDriftExpect(t, (*got)[2], "POST", "pool-link/checkout", "", `{"interval":"year"}`)

	list, _, err := c.PoolLink.ListInstances(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if list.Data[0].RemoteOrganizationID == nil || *list.Data[0].RemoteOrganizationID != "ro" || list.Plan.Warming != 1 || list.Plan.ManageURL != "https://m" {
		t.Fatalf("instances = %+v", list)
	}

	st, _, err := c.CloudLink.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !st.LegacyConnected || st.Link.OrganizationID == nil || !st.Link.DisconnectPending ||
		st.Info.SMTPAssociationProtocol != 1 || st.Info.Redirects == nil || st.Info.Redirects.Host != "r.example" ||
		st.Info.Plan.MailboxLimit != nil {
		t.Fatalf("status = %+v", st)
	}

	if _, err := c.CloudLink.DisconnectLegacy(ctx); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[5], "DELETE", "cloud-link", "legacy=true", "")
	if _, err := c.CloudLink.Disconnect(ctx); err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[6], "DELETE", "cloud-link", "", "")

	lim := 5
	mb, _, err := c.CloudLink.SetParticipation(ctx, "m1", &DiagnosticParticipation{Mode: ParticipationDiagnostic, Send: true, SharedDailyLimit: &lim})
	if err != nil {
		t.Fatal(err)
	}
	orgDriftExpect(t, (*got)[7], "PATCH", "cloud-link/mailboxes/m1/participation", "", `{"mode":"diagnostic","send":true,"receive":false,"shared_daily_limit":5}`)
	if !mb.IndependentSMTP || mb.EnrollmentState != "active" || mb.StandingObservedAt == nil ||
		mb.Cloud == nil || mb.Cloud.Participation == nil || mb.Cloud.Participation.Mode != ParticipationDiagnostic ||
		*mb.Cloud.Participation.SharedDailyLimit != 5 || mb.Cloud.ConsentCompletedAt == nil {
		t.Fatalf("mailbox = %+v", mb)
	}

	ws, _, err := c.CloudLink.ListWorkspaceMailboxes(ctx)
	if err != nil || ws[0].MailHost != "smtp.b.co" {
		t.Fatalf("workspace = %+v, %v", ws, err)
	}
}
