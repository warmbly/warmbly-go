package warmbly

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestMiscSyncRouting asserts the method, path and raw query of the routes
// added or reshaped in this sync.
func TestMiscSyncRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()

	cases := []struct {
		name       string
		call       func() error
		wantMethod string
		wantPath   string
		wantQuery  string
	}{
		{"assistant.ToolPolicies", func() error { _, _, e := c.Assistant.ToolPolicies(ctx); return e }, "GET", "/v1/ai/tool-policies", ""},
		{"assistant.RevokeToolPolicy", func() error { _, e := c.Assistant.RevokeToolPolicy(ctx, "update_campaign"); return e }, "DELETE", "/v1/ai/tool-policies/update_campaign", ""},
		{"organization.MemberAccess", func() error { _, _, e := c.Organization.MemberAccess(ctx, "u_1"); return e }, "GET", "/v1/organization/members/u_1/access", ""},
		{"organization.SuggestedSenders", func() error {
			_, _, e := c.Organization.SuggestedSenders(ctx, []string{"c_1", "c_2"}, []string{"f_1"})
			return e
		}, "GET", "/v1/organization/access/suggested-senders", "campaign_folder_ids=f_1&campaign_ids=c_1%2Cc_2"},
		{"organization.SuggestedSenders.empty", func() error { _, _, e := c.Organization.SuggestedSenders(ctx, nil, nil); return e }, "GET", "/v1/organization/access/suggested-senders", ""},
		{"leadSync.SourcesForSegment", func() error { _, _, e := c.LeadSync.SourcesForSegment(ctx, "s_1"); return e }, "GET", "/v1/lead-sync/sources", "segment_id=s_1"},
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
		})
	}
}

func TestAssistantApproveSendsToolCallID(t *testing.T) {
	var gotBody, gotPath string
	c := accountServerFunc(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"tool_secret\",\"secrets\":{\"api_key\":\"wmbly_x\"}}\n\ndata: {\"type\":\"done\"}\n\n")
	})
	var events []*AgentEvent
	_, err := c.Assistant.Approve(context.Background(), "sess_1", "call_7", ApprovalAlwaysAllow, func(ev *AgentEvent) bool {
		events = append(events, ev)
		return true
	})
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if gotPath != "/v1/ai/sessions/sess_1/approve" {
		t.Errorf("path = %s", gotPath)
	}
	var body map[string]string
	if err := json.Unmarshal([]byte(gotBody), &body); err != nil {
		t.Fatalf("body %q: %v", gotBody, err)
	}
	if body["decision"] != "always_allow" || body["tool_call_id"] != "call_7" {
		t.Errorf("body = %v", body)
	}
	if len(events) != 2 || events[0].Type != AgentEventToolSecret || events[0].Secrets["api_key"] != "wmbly_x" {
		t.Errorf("events = %+v", events)
	}
}

func TestAssistantTranscriptPendingDecode(t *testing.T) {
	const fixture = `{
	  "title": "Bounce check",
	  "turns": [{"role": "user", "blocks": [{"kind": "text", "text": "hi", "done": true}]}],
	  "pending": {
	    "message_id": "m_1",
	    "tool_call_id": "call_7",
	    "tool_name": "send_reply",
	    "risk": "send",
	    "args": {"thread_id": "t_1"},
	    "args_summary": "Reply to t_1",
	    "arguments": "{\n  \"thread_id\": \"t_1\"\n}",
	    "arguments_truncated": true,
	    "preview": {"from": "a@x.io", "to": ["b@y.io"], "subject": "Re: hi", "body": "Thanks", "body_html": "<p>Thanks</p>"},
	    "always_allow_offered": false
	  },
	  "free_model": false
	}`
	c := accountServer(t, fixture, nil)
	tr, _, err := c.Assistant.Transcript(context.Background(), "sess_1")
	if err != nil {
		t.Fatalf("Transcript: %v", err)
	}
	p := tr.Pending
	if p == nil || p.ToolCallID != "call_7" || !p.ArgumentsTruncated || p.AlwaysAllowOffered ||
		!strings.Contains(p.Arguments, "thread_id") || p.Preview == nil || p.Preview.To[0] != "b@y.io" || p.Preview.BodyHTML == "" {
		t.Errorf("pending = %+v", p)
	}
}

func TestAIToolPolicyDecode(t *testing.T) {
	const fixture = `{"data":[{"org_id":"o_1","tool_name":"update_campaign","decision":"always_allow","created_by":"u_1","created_by_name":"Ada Lovelace","created_at":"2026-09-01T10:00:00Z"}]}`
	c := accountServer(t, fixture, nil)
	ps, _, err := c.Assistant.ToolPolicies(context.Background())
	if err != nil {
		t.Fatalf("ToolPolicies: %v", err)
	}
	if len(ps) != 1 || ps[0].ToolName != "update_campaign" || ps[0].CreatedByName != "Ada Lovelace" || ps[0].CreatedBy == nil || ps[0].CreatedAt.IsZero() {
		t.Errorf("policies = %+v", ps)
	}
}

func TestPermissionCatalogGrantableDecode(t *testing.T) {
	const fixture = `{
	  "permissions": [{"name": "READ_EMAILS", "value": 1, "description": "Read mailboxes", "category": "read"}],
	  "presets": {"read_only": 1, "full_access": 33554431},
	  "grantable": 31,
	  "app_scopes": 33546239
	}`
	c := accountServer(t, fixture, nil)
	cat, _, err := c.APIKeys.Permissions(context.Background())
	if err != nil {
		t.Fatalf("Permissions: %v", err)
	}
	if cat.Grantable != 31 || cat.AppScopes != 33546239 || cat.AppScopes&PermAPIKeys != 0 {
		t.Errorf("catalog = %+v", cat)
	}
}

func TestSubscriptionManagedDecode(t *testing.T) {
	const fixture = `{
	  "id": "s_1", "user_id": "u_1", "organization_id": "o_1", "plan_id": "p_free",
	  "stripe_customer_id": "", "status": "incomplete", "cancel_at_period_end": false,
	  "managed_at": "2026-09-01T00:00:00Z", "managed_by": "u_admin", "managed_reason": "design partner",
	  "managed_until": "2027-01-01T00:00:00Z", "managed_plan_id": "p_pro", "managed": true,
	  "is_enterprise": false, "created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-09-01T00:00:00Z"
	}`
	c := accountServer(t, fixture, nil)
	sub, _, err := c.Billing.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !sub.Managed || sub.ManagedPlanID == nil || *sub.ManagedPlanID != "p_pro" || sub.ManagedUntil == nil || sub.ManagedReason == nil || sub.Status != "incomplete" {
		t.Errorf("subscription = %+v", sub)
	}
}

func TestPlanPriceYearlyAndGatewayProxyDecode(t *testing.T) {
	c := accountServer(t, `{"plans":[{"id":"p_1","price":49,"price_yearly":470,"discounted_price":0,"duration":"month"},{"id":"p_2","price":9,"duration":"month"}]}`, nil)
	plans, _, err := c.Meta.Plans(context.Background())
	if err != nil {
		t.Fatalf("Plans: %v", err)
	}
	if len(plans) != 2 || plans[0].PriceYearly == nil || *plans[0].PriceYearly != 470 || plans[1].PriceYearly != nil {
		t.Errorf("plans = %+v", plans)
	}

	c = accountServer(t, `{"url":"wss://rt.example/socket/websocket?token=abc","expires_in":60,"proxy_path":"/realtime/socket/websocket?token=abc"}`, nil)
	tk, _, err := c.Meta.GatewayTicket(context.Background())
	if err != nil {
		t.Fatalf("GatewayTicket: %v", err)
	}
	if tk.ProxyPath != "/realtime/socket/websocket?token=abc" || tk.ExpiresIn != 60 {
		t.Errorf("ticket = %+v", tk)
	}
}

func TestOrganizationCurrentRestrictedDecode(t *testing.T) {
	// A restricted member's view: no voice profile, no member count, and an
	// unknown category that must still decode.
	const fixture = `{
	  "id": "o_1", "name": "Acme", "owner_user_id": "u_owner",
	  "category": "sandbox", "timezone": "Europe/Berlin",
	  "presence_show_online": true, "presence_show_activity": true,
	  "inbox_agent_enabled": false, "assistant_shared_history": false,
	  "created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-09-01T00:00:00Z",
	  "limits": {"max_campaigns": 10},
	  "counts": {"total_campaigns": 2, "active_campaigns": 1, "total_contacts": 40, "email_accounts": 1, "emails_sent_today": 3}
	}`
	c := accountServer(t, fixture, nil)
	org, _, err := c.Organization.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if org.Category != "sandbox" || org.Timezone != "Europe/Berlin" || org.ProductDescription != "" {
		t.Errorf("org = %+v", org.Organization)
	}
	if org.Counts == nil || org.Counts.TotalMembers != 0 || org.Counts.TotalCampaigns != 2 {
		t.Errorf("counts = %+v", org.Counts)
	}
}

func TestMemberAccessDecodeAndBodies(t *testing.T) {
	const member = `{
	  "id": "m_1", "organization_id": "o_1", "user_id": "u_2", "role": "viewer",
	  "permissions": 1152, "invited_at": "2026-08-01T00:00:00Z",
	  "access_scope": "restricted",
	  "access": {"scope": "restricted", "campaign_folder_ids": ["f_1"], "campaign_ids": ["c_1"], "email_account_ids": ["e_1"]},
	  "email": "viewer@acme.io", "name": "Vee"
	}`
	var got recordedRequest
	c := accountServer(t, member, &got)
	ctx := context.Background()

	m, _, err := c.Organization.SetMemberAccess(ctx, "u_2", &MemberAccess{
		Scope:             AccessScopeRestricted,
		CampaignFolderIDs: []string{"f_1"},
		CampaignIDs:       []string{"c_1"},
		EmailAccountIDs:   []string{"e_1"},
	})
	if err != nil {
		t.Fatalf("SetMemberAccess: %v", err)
	}
	if got.method != http.MethodPut || got.path != "/v1/organization/members/u_2/access" {
		t.Errorf("got %s %s", got.method, got.path)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(got.body), &body); err != nil {
		t.Fatalf("body: %v", err)
	}
	if body["scope"] != "restricted" || len(body["campaign_folder_ids"].([]any)) != 1 || len(body["email_account_ids"].([]any)) != 1 {
		t.Errorf("body = %v", body)
	}
	if !m.IsRestricted() || m.Access == nil || !m.Access.Restricted() || m.Access.CampaignFolderIDs[0] != "f_1" {
		t.Errorf("member = %+v", m)
	}
	if m.Permissions&^OrgPermRestrictedMask != 0 {
		t.Errorf("fixture permissions %d exceed the restricted mask", m.Permissions)
	}

	// The invite carries the access scope the invitee lands with.
	c = accountServer(t, `{"invitation":{"id":"i_1","organization_id":"o_1","email":"new@acme.io","role":"viewer","permissions":1024,"invited_by":"u_1","expires_at":"2026-10-20T00:00:00Z","created_at":"2026-10-13T00:00:00Z","access":{"scope":"restricted","campaign_folder_ids":[],"campaign_ids":["c_1"],"email_account_ids":[]}}}`, &got)
	inv, _, err := c.Organization.Invite(ctx, &InviteMemberParams{
		Email:   "new@acme.io",
		RoleIDs: []string{"r_viewer"},
		Access:  &MemberAccess{Scope: AccessScopeRestricted, CampaignIDs: []string{"c_1"}},
	})
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}
	if !strings.Contains(got.body, `"access":{"scope":"restricted"`) || !strings.Contains(got.body, `"campaign_ids":["c_1"]`) {
		t.Errorf("invite body = %s", got.body)
	}
	if inv.Access == nil || inv.Access.CampaignIDs[0] != "c_1" {
		t.Errorf("invitation = %+v", inv)
	}

	// The workspace timezone travels on create.
	c = accountServer(t, `{"id":"o_2","name":"Beta","owner_user_id":"u_1","category":"standard","timezone":"America/New_York","created_at":"2026-10-13T00:00:00Z","updated_at":"2026-10-13T00:00:00Z"}`, &got)
	org, _, err := c.Organization.Create(ctx, &OrganizationCreateParams{Name: "Beta", Timezone: "America/New_York"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if strings.TrimSpace(got.body) != `{"name":"Beta","timezone":"America/New_York"}` {
		t.Errorf("create body = %s", got.body)
	}
	if org.Category != OrganizationCategoryStandard {
		t.Errorf("category = %q", org.Category)
	}
}

func TestSuggestedSendersDecode(t *testing.T) {
	c := accountServer(t, `{"data":[{"id":"e_1","email":"sales@acme.io","name":"Sales","campaign_ids":["c_1","c_2"]}]}`, nil)
	ss, _, err := c.Organization.SuggestedSenders(context.Background(), []string{"c_1"}, nil)
	if err != nil {
		t.Fatalf("SuggestedSenders: %v", err)
	}
	if len(ss) != 1 || ss[0].Email != "sales@acme.io" || len(ss[0].CampaignIDs) != 2 {
		t.Errorf("senders = %+v", ss)
	}
}

func TestLeadSyncSegmentIDs(t *testing.T) {
	var got recordedRequest
	c := accountServer(t, `{"id":"ls_1","organization_id":"o_1","created_by_user_id":"u_1","provider":"google_sheets","connection_id":"conn_1","sheet_id":"sh_1","has_header":true,"column_mapping":[],"dedup":"update","category_ids":[],"segment_ids":["seg_1"],"subscribed_default":true,"status":"idle","created_at":"2026-10-01T00:00:00Z","updated_at":"2026-10-01T00:00:00Z"}`, &got)
	src, _, err := c.LeadSync.Create(context.Background(), &LeadSyncCreateParams{
		ConnectionID: "conn_1", SheetID: "sh_1", SegmentIDs: []string{"seg_1"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.Contains(got.body, `"segment_ids":["seg_1"]`) {
		t.Errorf("body = %s", got.body)
	}
	if len(src.SegmentIDs) != 1 || src.SegmentIDs[0] != "seg_1" {
		t.Errorf("source = %+v", src)
	}

	// An empty, non-nil slice clears the targets on update.
	clear := []string{}
	if _, _, err := c.LeadSync.Update(context.Background(), "ls_1", &LeadSyncUpdateParams{SegmentIDs: &clear}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if strings.TrimSpace(got.body) != `{"segment_ids":[]}` {
		t.Errorf("update body = %s", got.body)
	}
}

func TestCreateStepWithParamsBody(t *testing.T) {
	var got recordedRequest
	c := accountServer(t, `{"id":"st_1","campaign_id":"camp_1","subject":"Hi","thread_reply":true,"position":1}`, &got)
	st, _, err := c.Campaigns.CreateStepWithParams(context.Background(), "camp_1", &StepUpdateParams{
		Subject:     String("Hi"),
		ThreadReply: Bool(true),
	})
	if err != nil {
		t.Fatalf("CreateStepWithParams: %v", err)
	}
	if got.method != http.MethodPost || got.path != "/v1/campaigns/camp_1/steps" || strings.TrimSpace(got.body) != `{"subject":"Hi","thread_reply":true}` {
		t.Errorf("got %s %s", got.method, got.body)
	}
	if !st.ThreadReply || st.Subject != "Hi" {
		t.Errorf("step = %+v", st)
	}

	// The blank form still sends no body.
	if _, _, err := c.Campaigns.CreateStep(context.Background(), "camp_1"); err != nil {
		t.Fatalf("CreateStep: %v", err)
	}
	if got.body != "" {
		t.Errorf("blank step body = %q", got.body)
	}
}

func TestFormsConfigTriageAvailableDecode(t *testing.T) {
	c := accountServer(t, `{"captcha_available":true,"triage_available":true}`, nil)
	cfg, _, err := c.Forms.Config(context.Background())
	if err != nil {
		t.Fatalf("Config: %v", err)
	}
	if !cfg.CaptchaAvailable || !cfg.TriageAvailable {
		t.Errorf("config = %+v", cfg)
	}
}
