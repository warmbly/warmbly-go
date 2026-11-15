package warmbly

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Shape changes the server made to resources the SDK already covered.

func TestLeadEnrichWebSearch(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"id": "run_1", "requested_by_user_id": null, "verify": true, "people": false,
		"web_search": false, "status": "queued", "total": 2, "processed": 0, "emails_found": 0, "people_found": 0,
		"no_website": 0, "failed": 0, "charged_usd": 0, "last_error": "", "started_at": null, "finished_at": null,
		"created_at": "2026-11-15T10:00:00Z", "updated_at": "2026-11-15T10:00:00Z"}`)
	run, _, err := c.Leads.Enrich(context.Background(), &LeadEnrichParams{
		LeadSelection: LeadSelection{LeadIDs: []string{"l_1", "l_2"}},
		People:        Bool(false),
		WebSearch:     Bool(false),
	})
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	checkRequest(t, got, "POST", "/v1/leads/enrich", "", `{"lead_ids":["l_1","l_2"],"people":false,"web_search":false}`)
	if run.WebSearch || !run.Verify {
		t.Errorf("run = %+v", run)
	}
}

func TestLeadSiteUnread(t *testing.T) {
	var site LeadSite
	if err := json.Unmarshal([]byte(`{"reachable": false, "https": true, "read_via": "jina", "unread": "robots"}`), &site); err != nil {
		t.Fatal(err)
	}
	if site.ReadVia != "jina" || site.Unread != LeadSiteRobots {
		t.Errorf("site = %+v", site)
	}
	var email LeadEmail
	if err := json.Unmarshal([]byte(`{"email": "a@b.test", "source": "search", "domain_match": true}`), &email); err != nil {
		t.Fatal(err)
	}
	if email.Source != "search" {
		t.Errorf("email = %+v", email)
	}
}

// The submissions list now answers an opaque cursor; Next and All follow it
// instead of the timestamp and never send both.
func TestFormSubmissionsCursor(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("cursor") == "" {
			_, _ = w.Write([]byte(`{"data":[
				{"id":"s_2","form_id":"f_1","organization_id":"org_1","data":{},"source_url":"","created_at":"2026-09-01T10:00:01Z"}
			],"has_more":true,"pagination":{"next_cursor":"opaque_1","has_more":true}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[
			{"id":"s_1","form_id":"f_1","organization_id":"org_1","data":{},"source_url":"","created_at":"2026-09-01T10:00:00Z"}
		],"has_more":false,"pagination":{"next_cursor":null,"has_more":false}}`))
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	page, err := c.Forms.ListSubmissions(context.Background(), "f_1", &FormSubmissionListParams{Limit: 1})
	if err != nil {
		t.Fatalf("ListSubmissions: %v", err)
	}
	if page.NextCursor() != "opaque_1" {
		t.Fatalf("NextCursor = %q", page.NextCursor())
	}
	ids := make([]string, 0, 2)
	for sub, err := range page.All(context.Background()) {
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		ids = append(ids, sub.ID)
	}
	if strings.Join(ids, ",") != "s_2,s_1" {
		t.Errorf("ids = %v", ids)
	}
	if len(queries) != 2 || queries[0] != "limit=1" || queries[1] != "cursor=opaque_1&limit=1" {
		t.Errorf("queries = %q", queries)
	}

	queries = nil
	if _, err := c.Forms.ListSubmissions(context.Background(), "f_1", &FormSubmissionListParams{Cursor: "opaque_1"}); err != nil {
		t.Fatalf("ListSubmissions: %v", err)
	}
	if len(queries) != 1 || queries[0] != "cursor=opaque_1" {
		t.Errorf("queries = %q", queries)
	}
}

func TestFormsDomainEdgeRecords(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"forms_domain": "forms.acme.test", "forms_domain_verified": false,
		"forms_domain_verified_at": null, "cname_target": "forms.warmbly.test", "status": "pending", "message": "",
		"forms_host_unresolvable": false,
		"edge_records": [{"purpose": "certificate", "type": "TXT", "name": "_acme-challenge.forms.acme.test", "value": "abc", "ok": false}]}`)
	st, _, err := c.Forms.Domain(context.Background())
	if err != nil {
		t.Fatalf("Domain: %v", err)
	}
	if len(st.EdgeRecords) != 1 || st.EdgeRecords[0].Type != "TXT" || st.EdgeRecords[0].OK || st.EdgeRecords[0].Optional {
		t.Errorf("edge records = %+v", st.EdgeRecords)
	}
}

func TestNotificationPreferencesNewCategories(t *testing.T) {
	var res NotificationPreferencesResult
	if err := json.Unmarshal([]byte(`{"preferences": {
		"inbound_reply": {"enabled": false, "channels": {"in_app": true, "email": false, "slack": false, "push": false}},
		"inbound_positive_reply": {"enabled": true, "channels": {"in_app": true, "email": false, "slack": false, "push": true}},
		"placement_alert": {"enabled": true, "channels": {"in_app": true, "email": true, "slack": false, "push": true}},
		"inbox_action_required": {"enabled": true, "channels": {"in_app": true, "email": true, "slack": false, "push": true}},
		"email_digest_minutes": 60
	}}`), &res); err != nil {
		t.Fatal(err)
	}
	p := res.Preferences
	if p.InboundReply.Enabled || !p.InboundPositiveReply.Enabled || !p.InboundPositiveReply.Channels.Push {
		t.Errorf("inbound = %+v / %+v", p.InboundReply, p.InboundPositiveReply)
	}
	if !p.PlacementAlert.Channels.Email || !p.InboxActionRequired.Enabled || p.PlacementFinished.Enabled {
		t.Errorf("placement/inbox = %+v", p)
	}
}

func TestPlanAndSubscriptionAgencyFields(t *testing.T) {
	var plan Plan
	if err := json.Unmarshal([]byte(`{"id": "p_ag", "name": "Agency", "monthly_send_pool": 0, "agency_ai_allowance_usd": 50}`), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.MonthlySendPool == nil || *plan.MonthlySendPool != 0 || plan.AgencyAIAllowanceUSD == nil || *plan.AgencyAIAllowanceUSD != 50 {
		t.Errorf("plan = %+v", plan)
	}
	var ws Plan
	if err := json.Unmarshal([]byte(`{"id": "p_pro", "name": "Pro"}`), &ws); err != nil {
		t.Fatal(err)
	}
	if ws.MonthlySendPool != nil || ws.AgencyAIAllowanceUSD != nil {
		t.Errorf("workspace plan = %+v", ws)
	}

	var sub Subscription
	if err := json.Unmarshal([]byte(`{"id": "sub_1", "status": "active", "is_enterprise": false, "managed_by_agency_id": "ag_1"}`), &sub); err != nil {
		t.Fatal(err)
	}
	if sub.ManagedByAgencyID == nil || *sub.ManagedByAgencyID != "ag_1" {
		t.Errorf("subscription = %+v", sub)
	}
}

func TestSegmentFieldOptionLabels(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"data": [
		{"field": "world_region", "label": "World region", "group": "Location", "kind": "enum",
			"options": ["north_america", "emea", "none"], "option_labels": {"emea": "Europe, Middle East and Africa", "none": "No known region"}},
		{"field": "location_known", "label": "Location known", "group": "Location", "kind": "bool"}
	]}`)
	fields, _, err := c.Segments.Fields(context.Background())
	if err != nil {
		t.Fatalf("Fields: %v", err)
	}
	if len(fields) != 2 || fields[0].OptionLabels[SegmentWorldRegionNone] != "No known region" || fields[1].Kind != SegmentFieldBool {
		t.Errorf("fields = %+v", fields)
	}
}
