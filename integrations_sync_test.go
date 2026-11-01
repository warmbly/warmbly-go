package warmbly

import (
	"context"
	"strings"
	"testing"
)

// TestIntegrationCoverageRouting asserts method, path, raw query and body of every
// integration method added in the sync with the current server API.
func TestIntegrationCoverageRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()
	yes := true

	cases := []struct {
		name       string
		call       func() error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   []string
	}{
		{"StartOAuthWith", func() error {
			_, _, e := c.Integrations.StartOAuthWith(ctx, &OAuthStartParams{
				Provider: ProviderSalesforce, Environment: SalesforceSandbox, Domain: "acme.my.salesforce.com",
			})
			return e
		}, "POST", "/v1/integrations/oauth/start", "", []string{
			`"provider":"salesforce"`, `"environment":"sandbox"`, `"domain":"acme.my.salesforce.com"`,
		}},
		{"StartOAuth keeps its body", func() error {
			_, _, e := c.Integrations.StartOAuth(ctx, ProviderHubSpot, "Main")
			return e
		}, "POST", "/v1/integrations/oauth/start", "", []string{`"provider":"hubspot"`, `"label":"Main"`}},
		{"CommunityApps", func() error {
			_, e := c.Integrations.CommunityApps(ctx, &ListOptions{Limit: 20, Cursor: "off"})
			return e
		}, "GET", "/v1/integrations/community", "cursor=off&limit=20", nil},
		{"CommunityApp", func() error {
			_, _, e := c.Integrations.CommunityApp(ctx, "acme-sync")
			return e
		}, "GET", "/v1/integrations/community/acme-sync", "", nil},
		{"SetSigningKey", func() error {
			_, _, e := c.Integrations.SetSigningKey(ctx, "cn_1", "0123456789")
			return e
		}, "PUT", "/v1/integrations/connections/cn_1/signing-key", "", []string{`{"signing_key":"0123456789"}`}},
		{"SetSigningKey clear", func() error {
			_, _, e := c.Integrations.SetSigningKey(ctx, "cn_1", "")
			return e
		}, "PUT", "/v1/integrations/connections/cn_1/signing-key", "", []string{`{"signing_key":""}`}},
		{"RotateInboundURL", func() error {
			_, _, e := c.Integrations.RotateInboundURL(ctx, "cn_1")
			return e
		}, "POST", "/v1/integrations/connections/cn_1/rotate-inbound-url", "", nil},

		// Slack.
		{"SlackStatus", func() error { _, _, e := c.Integrations.SlackStatus(ctx); return e },
			"GET", "/v1/integrations/slack/status", "", nil},
		{"SlackChannels", func() error {
			_, e := c.Integrations.SlackChannels(ctx, &SlackChannelParams{Query: "sales"})
			return e
		}, "GET", "/v1/integrations/slack/channels", "q=sales", nil},
		{"UpdateSlackSettings", func() error {
			_, _, e := c.Integrations.UpdateSlackSettings(ctx, &SlackSettings{
				Channel: "C01", Routes: map[string]string{"inbound_reply": "C02"}, InboxChannel: "C03", InboxScope: SlackInboxAll,
			})
			return e
		}, "PUT", "/v1/integrations/slack/settings", "", []string{
			`"channel":"C01"`, `"routes":{"inbound_reply":"C02"}`, `"inbox_channel":"C03"`, `"inbox_scope":"all"`,
		}},
		{"PreviewSlackLink", func() error {
			_, _, e := c.Integrations.PreviewSlackLink(ctx, "abc_DEF-123")
			return e
		}, "GET", "/v1/integrations/slack/link/abc_DEF-123", "", nil},
		{"ConfirmSlackLink", func() error {
			_, _, e := c.Integrations.ConfirmSlackLink(ctx, &SlackLinkConfirmParams{Code: "abc", SlackCode: "sc", State: "st"})
			return e
		}, "POST", "/v1/integrations/slack/link", "", []string{`"code":"abc"`, `"slack_code":"sc"`, `"state":"st"`}},
		{"StartSlackLinkVerify", func() error {
			_, _, e := c.Integrations.StartSlackLinkVerify(ctx, "abc")
			return e
		}, "POST", "/v1/integrations/slack/link/verify", "", []string{`{"code":"abc"}`}},
		{"SetSlackDMNotifications off", func() error {
			_, _, e := c.Integrations.SetSlackDMNotifications(ctx, false)
			return e
		}, "PATCH", "/v1/integrations/slack/link", "", []string{`{"dm_notifications":false}`}},
		{"UnlinkSlack", func() error { _, e := c.Integrations.UnlinkSlack(ctx); return e },
			"DELETE", "/v1/integrations/slack/link", "", nil},
		{"RemoveSlackLink", func() error { _, e := c.Integrations.RemoveSlackLink(ctx, "ln_1"); return e },
			"DELETE", "/v1/integrations/slack/links/ln_1", "", nil},

		// Salesforce.
		{"SalesforceOverview", func() error {
			_, _, e := c.Integrations.SalesforceOverview(ctx, "cn_1", false)
			return e
		}, "GET", "/v1/integrations/salesforce/cn_1/overview", "", nil},
		{"SalesforceOverview checks", func() error {
			_, _, e := c.Integrations.SalesforceOverview(ctx, "cn_1", true)
			return e
		}, "GET", "/v1/integrations/salesforce/cn_1/overview", "checks=1", nil},
		{"SalesforceSettings", func() error {
			_, _, e := c.Integrations.SalesforceSettings(ctx, "cn_1")
			return e
		}, "GET", "/v1/integrations/salesforce/cn_1/settings", "", nil},
		{"UpdateSalesforceSettings", func() error {
			_, _, e := c.Integrations.UpdateSalesforceSettings(ctx, "cn_1", &SalesforceSettings{
				Matching: SalesforceMatching{Prefer: "contact", CreateWhen: "reply", CreateAs: "lead", Owner: "connected_user"},
				Activity: SalesforceActivitySettings{AssignTo: "record_owner"},
				FieldMap: []SalesforceFieldRule{{Object: SalesforceObjectLead, Warmbly: "company", Salesforce: "Company", Direction: SyncBoth, Policy: SalesforcePolicyIfEmpty}},
			})
			return e
		}, "PUT", "/v1/integrations/salesforce/cn_1/settings", "", []string{
			`"enabled":false`, `"prefer":"contact"`, `"assign_to":"record_owner"`, `"sent":false`,
			`"field_map":[{"object":"Lead","warmbly":"company","salesforce":"Company","direction":"both","policy":"if_empty"}]`,
			`"daily_api_budget":0`,
		}},
		{"SalesforceMetadata", func() error {
			_, _, e := c.Integrations.SalesforceMetadata(ctx, "cn_1")
			return e
		}, "GET", "/v1/integrations/salesforce/cn_1/metadata", "", nil},
		{"SalesforceUsers", func() error {
			_, _, e := c.Integrations.SalesforceUsers(ctx, "cn_1", "ann smith")
			return e
		}, "GET", "/v1/integrations/salesforce/cn_1/users", "q=ann+smith", nil},
		{"SalesforceListViews", func() error {
			_, _, e := c.Integrations.SalesforceListViews(ctx, "cn_1", SalesforceObjectLead)
			return e
		}, "GET", "/v1/integrations/salesforce/cn_1/list-views", "object=Lead", nil},
		{"SalesforceCampaigns", func() error {
			_, _, e := c.Integrations.SalesforceCampaigns(ctx, "cn_1", "webinar")
			return e
		}, "GET", "/v1/integrations/salesforce/cn_1/campaigns", "q=webinar", nil},
		{"PreviewSalesforceImport", func() error {
			_, _, e := c.Integrations.PreviewSalesforceImport(ctx, "cn_1", &SalesforceImportSourceRef{
				SourceKind: SalesforceSourceListView, Object: SalesforceObjectContact, SourceID: "00B000000000001AAA",
			})
			return e
		}, "POST", "/v1/integrations/salesforce/cn_1/import/preview", "", []string{
			`"source_kind":"list_view"`, `"object":"Contact"`, `"source_id":"00B000000000001AAA"`,
		}},
		{"SalesforceImportSources", func() error {
			_, _, e := c.Integrations.SalesforceImportSources(ctx, "cn_1")
			return e
		}, "GET", "/v1/integrations/salesforce/cn_1/import-sources", "", nil},
		{"CreateSalesforceImportSource", func() error {
			_, _, e := c.Integrations.CreateSalesforceImportSource(ctx, "cn_1", &SalesforceImportSourceParams{
				SourceKind: SalesforceSourceCampaign, SourceID: "701000000000001AAA", SourceLabel: "Webinar", Recurring: &yes,
				CategoryIDs: []string{"cat_1"},
			})
			return e
		}, "POST", "/v1/integrations/salesforce/cn_1/import-sources", "", []string{
			`"source_kind":"campaign"`, `"source_label":"Webinar"`, `"recurring":true`, `"category_ids":["cat_1"]`,
		}},
		{"UpdateSalesforceImportSource clears the campaign", func() error {
			_, _, e := c.Integrations.UpdateSalesforceImportSource(ctx, "cn_1", "src_1", &SalesforceImportSourceUpdate{
				ClearCampaign: true, CategoryIDs: []string{}, Enabled: &yes,
			})
			return e
		}, "PATCH", "/v1/integrations/salesforce/cn_1/import-sources/src_1", "", []string{
			`"campaign_id":null`, `"category_ids":[]`, `"enabled":true`,
		}},
		{"RunSalesforceImportSource", func() error {
			_, _, e := c.Integrations.RunSalesforceImportSource(ctx, "cn_1", "src_1")
			return e
		}, "POST", "/v1/integrations/salesforce/cn_1/import-sources/src_1/run", "", nil},
		{"DeleteSalesforceImportSource", func() error {
			_, e := c.Integrations.DeleteSalesforceImportSource(ctx, "cn_1", "src_1")
			return e
		}, "DELETE", "/v1/integrations/salesforce/cn_1/import-sources/src_1", "", nil},
		{"SalesforceActivityLog", func() error {
			_, e := c.Integrations.SalesforceActivityLog(ctx, "cn_1", &SalesforceActivityParams{
				ListOptions: ListOptions{Limit: 50, Cursor: "cur"}, Status: SalesforceActivityFailed, ContactID: "ct_1",
			})
			return e
		}, "GET", "/v1/integrations/salesforce/cn_1/activity", "contact_id=ct_1&cursor=cur&limit=50&status=failed", nil},
		{"RetrySalesforceActivity", func() error {
			_, _, e := c.Integrations.RetrySalesforceActivity(ctx, "cn_1", []string{"a1", "a2"})
			return e
		}, "POST", "/v1/integrations/salesforce/cn_1/activity/retry", "", []string{`{"ids":["a1","a2"]}`}},
		{"SalesforceSyncNow", func() error {
			_, e := c.Integrations.SalesforceSyncNow(ctx, "cn_1")
			return e
		}, "POST", "/v1/integrations/salesforce/cn_1/sync-now", "", nil},
		{"Contacts.Salesforce", func() error {
			_, _, e := c.Contacts.Salesforce(ctx, "ct_1")
			return e
		}, "GET", "/v1/contacts/ct_1/salesforce", "", nil},
		{"Contacts.SyncSalesforce", func() error {
			_, _, e := c.Contacts.SyncSalesforce(ctx, "ct_1", &SalesforceContactSyncParams{ConnectionID: "cn_1", CreateAs: "lead"})
			return e
		}, "POST", "/v1/contacts/ct_1/salesforce/sync", "", []string{`"connection_id":"cn_1"`, `"create_as":"lead"`}},
		{"Contacts.SyncSalesforce no params", func() error {
			_, _, e := c.Contacts.SyncSalesforce(ctx, "ct_1", nil)
			return e
		}, "POST", "/v1/contacts/ct_1/salesforce/sync", "", nil},
		{"Contacts.UnlinkSalesforce", func() error {
			_, e := c.Contacts.UnlinkSalesforce(ctx, "ct_1", "ln_1")
			return e
		}, "DELETE", "/v1/contacts/ct_1/salesforce/links/ln_1", "", nil},

		// Apollo.
		{"ApolloOverview", func() error { _, _, e := c.Integrations.ApolloOverview(ctx, "cn_1"); return e },
			"GET", "/v1/integrations/apollo/cn_1", "", nil},
		{"ReplaceApolloKey", func() error {
			_, _, e := c.Integrations.ReplaceApolloKey(ctx, "cn_1", "key-123")
			return e
		}, "PUT", "/v1/integrations/apollo/cn_1/key", "", []string{`{"api_key":"key-123"}`}},
		{"ApolloLists", func() error { _, _, e := c.Integrations.ApolloLists(ctx, "cn_1"); return e },
			"GET", "/v1/integrations/apollo/cn_1/lists", "", nil},
		{"PreviewApolloImport", func() error {
			no := false
			_, _, e := c.Integrations.PreviewApolloImport(ctx, "cn_1", &ApolloPreviewParams{ListID: "l1", VerifiedOnly: &no})
			return e
		}, "POST", "/v1/integrations/apollo/cn_1/import/preview", "", []string{`"list_id":"l1"`, `"verified_only":false`}},
		{"ApolloImportSources", func() error {
			_, _, e := c.Integrations.ApolloImportSources(ctx, "cn_1")
			return e
		}, "GET", "/v1/integrations/apollo/cn_1/import-sources", "", nil},
		{"CreateApolloImportSource", func() error {
			_, _, e := c.Integrations.CreateApolloImportSource(ctx, "cn_1", &ApolloImportSourceParams{
				ListID: "l1", ListName: "ICP", Recurring: &yes, CampaignID: String("camp_1"),
			})
			return e
		}, "POST", "/v1/integrations/apollo/cn_1/import-sources", "", []string{
			`"list_id":"l1"`, `"list_name":"ICP"`, `"recurring":true`, `"campaign_id":"camp_1"`,
		}},
		{"UpdateApolloImportSource", func() error {
			_, _, e := c.Integrations.UpdateApolloImportSource(ctx, "cn_1", "src_1", &ApolloImportSourceUpdate{
				CampaignID: String("camp_2"), VerifiedOnly: Bool(false),
			})
			return e
		}, "PATCH", "/v1/integrations/apollo/cn_1/import-sources/src_1", "", []string{
			`"campaign_id":"camp_2"`, `"verified_only":false`,
		}},
		{"RunApolloImportSource", func() error {
			_, _, e := c.Integrations.RunApolloImportSource(ctx, "cn_1", "src_1")
			return e
		}, "POST", "/v1/integrations/apollo/cn_1/import-sources/src_1/run", "", nil},
		{"DeleteApolloImportSource", func() error {
			_, e := c.Integrations.DeleteApolloImportSource(ctx, "cn_1", "src_1")
			return e
		}, "DELETE", "/v1/integrations/apollo/cn_1/import-sources/src_1", "", nil},
		{"ApolloEnrichments", func() error {
			_, _, e := c.Integrations.ApolloEnrichments(ctx, "cn_1", 10)
			return e
		}, "GET", "/v1/integrations/apollo/cn_1/enrichments", "limit=10", nil},
		{"ApolloEnrichments default limit", func() error {
			_, _, e := c.Integrations.ApolloEnrichments(ctx, "cn_1", 0)
			return e
		}, "GET", "/v1/integrations/apollo/cn_1/enrichments", "", nil},
		{"StartApolloEnrichment", func() error {
			_, _, e := c.Integrations.StartApolloEnrichment(ctx, "cn_1", &ApolloEnrichParams{ContactIDs: []string{"ct_1"}, Overwrite: true})
			return e
		}, "POST", "/v1/integrations/apollo/cn_1/enrichments", "", []string{`"contact_ids":["ct_1"]`, `"overwrite":true`}},
		{"ApolloEnrichment", func() error {
			_, _, e := c.Integrations.ApolloEnrichment(ctx, "cn_1", "run_1")
			return e
		}, "GET", "/v1/integrations/apollo/cn_1/enrichments/run_1", "", nil},

		// Clay.
		{"ClaySettings", func() error { _, _, e := c.Integrations.ClaySettings(ctx, "cn_1"); return e },
			"GET", "/v1/integrations/clay/cn_1", "", nil},
		{"UpdateClaySettings", func() error {
			events := []string{}
			_, _, e := c.Integrations.UpdateClaySettings(ctx, "cn_1", &ClaySettingsUpdate{
				Defaults:   &ClayInboundDefaults{Dedup: ImportDedupSkip, Subscribed: true},
				WebhookURL: String(""),
				Events:     &events,
			})
			return e
		}, "PATCH", "/v1/integrations/clay/cn_1", "", []string{
			`"dedup":"skip"`, `"subscribed":true`, `"webhook_url":""`, `"events":[]`,
		}},
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

func TestSalesforceSettingsView(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{
	  "settings": {
	    "enabled": true,
	    "matching": {"prefer": "contact", "create_when": "reply", "create_as": "lead", "lead_source": "Warmbly", "lead_status": "", "owner": "connected_user", "run_assignment_rules": false},
	    "activity": {"sent": true, "replied": true, "opened": false, "clicked": false, "bounced": true, "unsubscribed": true, "meeting_booked": true, "include_body": true, "assign_to": "record_owner", "relate_to_opportunity": true},
	    "writeback": {"lead_status_on_sent": "", "lead_status_on_reply": {"positive": "Working"}, "lead_status_on_meeting": "", "never_move_backwards": true},
	    "inbound": {"opt_out": "both", "pause_on_converted": false, "pause_on_statuses": [], "pause_on_open_opportunity": false},
	    "field_map": [{"object": "Lead", "warmbly": "first_name", "salesforce": "FirstName", "direction": "future_direction", "policy": "if_empty"}],
	    "daily_api_budget": 0
	  },
	  "warmbly_fields": [{"key": "first_name", "label": "First name"}],
	  "defaults": {"enabled": true, "matching": {"prefer": "contact"}, "activity": {}, "writeback": {"lead_status_on_reply": {}}, "inbound": {"pause_on_statuses": []}, "field_map": [], "daily_api_budget": 0}
	}`)

	v, _, err := c.Integrations.SalesforceSettings(context.Background(), "cn_1")
	if err != nil {
		t.Fatalf("SalesforceSettings: %v", err)
	}
	if !v.Settings.Enabled || v.Settings.Matching.CreateAs != "lead" || v.Settings.Writeback.LeadStatusOnReply["positive"] != "Working" {
		t.Errorf("settings = %+v", v.Settings)
	}
	// An enum value this release has never seen decodes as a plain string.
	if v.Settings.FieldMap[0].Direction != "future_direction" {
		t.Errorf("field rule = %+v", v.Settings.FieldMap[0])
	}
	if len(v.WarmblyFields) != 1 || v.WarmblyFields[0].Key != "first_name" || !v.Defaults.Enabled {
		t.Errorf("view = %+v", v)
	}
}

func TestSalesforceOverviewDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{
	  "connection_id": "cn_1", "label": "Prod", "status": "connected", "health": "healthy",
	  "org": {"id": "00D1", "instance_url": "https://acme.my.salesforce.com", "environment": "production", "login_host": "login.salesforce.com", "user_id": "005", "account": "Acme"},
	  "api": {"used": 10, "max": 100000, "calls_today": 4, "budget": 20000},
	  "counts": {"pending": 2, "synced_24h": 5, "failed": 1, "skipped_24h": 0, "linked_records": 40},
	  "last_pull_at": "2026-10-01T10:00:00Z",
	  "settings_enabled": true,
	  "checks": [{"key": "task_create", "label": "Create Tasks", "ok": false, "detail": "missing"}]
	}`)
	o, _, err := c.Integrations.SalesforceOverview(context.Background(), "cn_1", true)
	if err != nil {
		t.Fatalf("SalesforceOverview: %v", err)
	}
	if o.Org.Environment != "production" || o.API.Budget != 20000 || o.Counts.LinkedTotal != 40 {
		t.Errorf("overview = %+v", o)
	}
	if o.LastPullAt == nil || len(o.Checks) != 1 || o.Checks[0].OK {
		t.Errorf("overview = %+v", o)
	}
}

func TestSalesforceActivityPage(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{
	  "data": [{
	    "id": "act_1", "organization_id": "org_1", "connection_id": "cn_1", "contact_id": "ct_1",
	    "contact_email": "a@acme.com", "kind": "replied", "payload": {"subject": "Hi"},
	    "status": "failed", "attempts": 3, "next_attempt_at": "2026-10-01T10:00:00Z",
	    "detail": "REQUIRED_FIELD_MISSING", "occurred_at": "2026-10-01T09:00:00Z", "created_at": "2026-10-01T09:00:01Z"
	  }],
	  "pagination": {"next_cursor": "c2", "has_more": true}
	}`)
	page, err := c.Integrations.SalesforceActivityLog(context.Background(), "cn_1", nil)
	if err != nil {
		t.Fatalf("SalesforceActivityLog: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].Status != SalesforceActivityFailed || page.Data[0].ContactID == nil {
		t.Errorf("page = %+v", page.Data)
	}
	if page.Pagination.Total != nil || !page.HasMore() || page.NextCursor() != "c2" {
		t.Errorf("pagination = %+v", page.Pagination)
	}
}

func TestSalesforcePanelDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{
	  "connections": [{"id": "cn_1", "label": "Prod", "environment": "production", "instance_url": "https://acme.my.salesforce.com"}],
	  "records": [{
	    "link_id": "ln_1", "connection_id": "cn_1", "connection_label": "Prod", "object": "Lead", "id": "00Q1", "url": "https://x",
	    "name": "Ann", "status": "Working", "owner": {"id": "005", "name": "Bob"},
	    "is_converted": false, "opted_out": false,
	    "opportunities": [{"id": "006", "name": "Big", "stage": "Prospecting", "amount": 1200.5, "is_closed": false, "is_won": false, "url": "u"}],
	    "tasks": [{"id": "00T", "subject": "Email: Hi", "status": "Completed", "url": "u", "from_warmbly": true}],
	    "linked_by": "match", "sync": {"pending": 0, "failed": 1, "synced": 2, "last_error": "boom"}, "stale": true
	  }],
	  "can_sync": true
	}`)
	p, _, err := c.Contacts.Salesforce(context.Background(), "ct_1")
	if err != nil {
		t.Fatalf("Salesforce: %v", err)
	}
	r := p.Records[0]
	if !p.CanSync || r.Object != "Lead" || r.Owner == nil || r.Owner.Name != "Bob" || r.Account != nil {
		t.Errorf("panel = %+v", p)
	}
	if r.Opportunities[0].Amount == nil || *r.Opportunities[0].Amount != 1200.5 || !r.Tasks[0].FromWarmbly || !r.Stale {
		t.Errorf("record = %+v", r)
	}
}

func TestSalesforceImportSourceDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{
	  "id": "src_1", "organization_id": "org_1", "connection_id": "cn_1", "created_by_user_id": "u_1",
	  "name": "Webinar", "source_kind": "campaign", "object": "CampaignMember", "source_id": "701", "source_label": "Webinar",
	  "campaign_id": null, "category_ids": ["cat_1"], "recurring": true, "enabled": true, "status": "running",
	  "last_result": {"read": 10, "imported": 6, "updated": 1, "linked": 2, "skipped": 0, "failed": 0, "no_email": 1, "opted_out": 0},
	  "total_imported": 6, "created_at": "2026-10-01T09:00:00Z", "updated_at": "2026-10-01T09:00:00Z"
	}`)
	src, _, err := c.Integrations.RunSalesforceImportSource(context.Background(), "cn_1", "src_1")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if src.Status != SalesforceSourceRunning || src.CampaignID != nil || src.LastResult == nil || src.LastResult.Imported != 6 {
		t.Errorf("source = %+v", src)
	}
}

func TestApolloDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{
	  "id": "run_1", "organization_id": "org_1", "connection_id": "cn_1", "requested_by_user_id": "u_1",
	  "overwrite": false, "status": "paused_by_future", "total": 100, "processed": 10, "enriched": 8,
	  "not_found": 1, "failed": 1, "credits_consumed": 8,
	  "created_at": "2026-10-01T09:00:00Z", "updated_at": "2026-10-01T09:00:00Z"
	}`)
	run, _, err := c.Integrations.StartApolloEnrichment(context.Background(), "cn_1", &ApolloEnrichParams{ContactIDs: []string{"ct_1"}})
	if err != nil {
		t.Fatalf("StartApolloEnrichment: %v", err)
	}
	if run.Status != "paused_by_future" || run.CreditsConsumed != 8 || run.StartedAt != nil {
		t.Errorf("run = %+v", run)
	}
}

func TestApolloSourceDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"data": [{
	  "id": "src_1", "organization_id": "org_1", "connection_id": "cn_1", "name": "ICP", "list_id": "l1", "list_name": "ICP",
	  "category_ids": [], "verified_only": true, "recurring": false, "enabled": true, "status": "idle",
	  "last_result": {"read": 5, "imported": 3, "updated": 0, "skipped": 0, "failed": 0, "no_email": 1, "unverified": 1, "suppressed": 0, "already_seen": 0, "next_page": 3, "truncated": true},
	  "total_imported": 3, "created_at": "2026-10-01T09:00:00Z", "updated_at": "2026-10-01T09:00:00Z"
	}]}`)
	list, _, err := c.Integrations.ApolloImportSources(context.Background(), "cn_1")
	if err != nil {
		t.Fatalf("ApolloImportSources: %v", err)
	}
	if len(list) != 1 || list[0].LastResult == nil || list[0].LastResult.NextPage != 3 || !list[0].VerifiedOnly {
		t.Errorf("list = %+v", list)
	}
}

func TestClaySettingsDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{
	  "connection_id": "cn_1", "label": "Clay", "status": "connected", "health": "healthy", "inbound_url_minted": true,
	  "defaults": {"campaign_id": "camp_1", "category_ids": ["cat_1"], "segment_ids": [], "dedup": "update", "subscribed": true},
	  "outbound": {"webhook_url_set": true, "webhook_host": "api.clay.com", "auth_token_set": false, "events": ["reply.received"]},
	  "stats": {"received": 9, "created": 5, "updated": 3, "skipped": 1, "failed": 0, "last_received_at": "2026-10-01T09:00:00Z"},
	  "available_events": ["reply.received", "future.event"]
	}`)
	s, _, err := c.Integrations.ClaySettings(context.Background(), "cn_1")
	if err != nil {
		t.Fatalf("ClaySettings: %v", err)
	}
	if s.Defaults.CampaignID == nil || s.Defaults.Dedup != ImportDedupUpdate || !s.Outbound.WebhookURLSet || s.Stats.LastReceivedAt == nil {
		t.Errorf("settings = %+v", s)
	}
}

func TestSlackStatusDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{
	  "app_configured": true, "interactive_configured": true,
	  "connection": {"id": "cn_1", "organization_id": "org_1", "provider": "slack", "label": "Acme", "status": "connected", "auth_method": "oauth", "sync_direction": "push", "health": "healthy", "created_at": "2026-10-01T09:00:00Z", "updated_at": "2026-10-01T09:00:00Z"},
	  "missing_scopes": ["im:history"],
	  "settings": {"channel": "C01", "routes": {"inbound_reply": "C02"}, "inbox_scope": "replies"},
	  "my_link": {"id": "ln_1", "organization_id": "org_1", "connection_id": "cn_1", "slack_team_id": "T1", "slack_user_id": "U1", "user_id": "u_1", "dm_notifications": true, "created_at": "2026-10-01T09:00:00Z", "updated_at": "2026-10-01T09:00:00Z"},
	  "links": []
	}`)
	st, _, err := c.Integrations.SlackStatus(context.Background())
	if err != nil {
		t.Fatalf("SlackStatus: %v", err)
	}
	if st.Connection == nil || st.Connection.Provider != ProviderSlack || st.Settings.Routes["inbound_reply"] != "C02" {
		t.Errorf("status = %+v", st)
	}
	if st.MyLink == nil || !st.MyLink.DMNotifications || len(st.MissingScopes) != 1 {
		t.Errorf("status = %+v", st)
	}
}

func TestSlackStatusOwnLinkOnly(t *testing.T) {
	var got recordedRequest
	// A caller without integrations access gets readiness and their own link
	// only, so the connection is absent and the lists are empty.
	c := recordingFixtureClient(t, &got, `{"app_configured": true, "interactive_configured": false, "missing_scopes": [], "settings": {}, "links": []}`)
	st, _, err := c.Integrations.SlackStatus(context.Background())
	if err != nil {
		t.Fatalf("SlackStatus: %v", err)
	}
	if st.Connection != nil || st.MyLink != nil || st.InteractiveConfigured {
		t.Errorf("status = %+v", st)
	}
}

func TestSlackChannelsPage(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{
	  "data": [{"id": "C01", "name": "sales", "is_private": false, "is_member": true}],
	  "pagination": {"next_cursor": null, "has_more": true}
	}`)
	page, err := c.Integrations.SlackChannels(context.Background(), nil)
	if err != nil {
		t.Fatalf("SlackChannels: %v", err)
	}
	if len(page.Data) != 1 || !page.Data[0].IsMember || !page.Pagination.HasMore {
		t.Errorf("page = %+v", page)
	}
	// No cursor accompanies has_more, so the page cannot be advanced.
	if page.HasMore() {
		t.Error("HasMore must be false without a cursor")
	}
}

func TestSlackLinkPreviewDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{
	  "organization_id": "org_1", "organization_name": "Acme", "is_member": true, "user_email": "a@acme.com",
	  "verify_available": true, "slack_team_id": "T1", "slack_team_name": "Acme HQ", "slack_user_id": "U1",
	  "slack_user_name": "ann", "slack_user_avatar": "", "email_matches": false, "expires_at": "2026-10-01T10:00:00Z"
	}`)
	p, _, err := c.Integrations.PreviewSlackLink(context.Background(), "code")
	if err != nil {
		t.Fatalf("PreviewSlackLink: %v", err)
	}
	if p.OrganizationName != "Acme" || !p.VerifyAvailable || p.EmailMatches || p.SlackTeamName != "Acme HQ" {
		t.Errorf("preview = %+v", p)
	}
}

func TestCommunityAppDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{
	  "data": [{
	    "application_id": "app_1", "slug": "acme-sync", "name": "Acme Sync", "tagline": "t", "description": "d",
	    "category": "future_category", "logo_url": "", "website_url": "", "install_url": "https://i", "support_url": "", "privacy_url": "",
	    "developer": "Acme", "scopes": 5,
	    "permissions": [{"name": "READ_CONTACTS", "value": 4, "description": "View contacts", "category": "read"}],
	    "status": "featured", "listed": true, "installs": 12, "installed": false, "published_at": "2026-09-01T00:00:00Z"
	  }],
	  "pagination": {"total": 31, "next_cursor": "o2", "has_more": true}
	}`)
	page, err := c.Integrations.CommunityApps(context.Background(), nil)
	if err != nil {
		t.Fatalf("CommunityApps: %v", err)
	}
	app := page.Data[0]
	if app.Status != CommunityFeatured || app.Category != "future_category" || app.Permissions[0].Name != "READ_CONTACTS" {
		t.Errorf("app = %+v", app)
	}
	if page.Pagination.Total == nil || *page.Pagination.Total != 31 || !page.HasMore() {
		t.Errorf("pagination = %+v", page.Pagination)
	}
}

func TestSigningKeyAndRotateDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"connection": {"id": "cn_1", "provider": "calendly", "status": "connected", "health": "healthy"}}`)
	conn, _, err := c.Integrations.SetSigningKey(context.Background(), "cn_1", "0123456789")
	if err != nil {
		t.Fatalf("SetSigningKey: %v", err)
	}
	if conn == nil || conn.Provider != ProviderCalendly {
		t.Errorf("connection = %+v", conn)
	}

	c = recordingFixtureClient(t, &got, `{"inbound_webhook_url": "https://api.example/inbound/calendly/s3cret"}`)
	u, _, err := c.Integrations.RotateInboundURL(context.Background(), "cn_1")
	if err != nil {
		t.Fatalf("RotateInboundURL: %v", err)
	}
	if u != "https://api.example/inbound/calendly/s3cret" {
		t.Errorf("url = %q", u)
	}
}

func TestCatalogEntryRank(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"catalog": [{"provider": "apollo", "name": "Apollo", "auth_method": "api_key", "beta": false, "supports_push": false, "configured": true, "rank": 4}]}`)
	cat, _, err := c.Integrations.Catalog(context.Background())
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	if cat[0].Provider != ProviderApollo || cat[0].Rank != 4 {
		t.Errorf("entry = %+v", cat[0])
	}
}

func TestSalesforceRetryAndSyncNow(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"requeued": 2}`)
	n, _, err := c.Integrations.RetrySalesforceActivity(context.Background(), "cn_1", []string{"a1", "a2"})
	if err != nil || n != 2 {
		t.Errorf("requeued = %d, err = %v", n, err)
	}

	// sync-now answers {"ok": true}; the method returns only the response.
	c = recordingFixtureClient(t, &got, `{"ok": true}`)
	if _, err := c.Integrations.SalesforceSyncNow(context.Background(), "cn_1"); err != nil {
		t.Fatalf("SalesforceSyncNow: %v", err)
	}
}
