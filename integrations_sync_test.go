package warmbly

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestIntegrationsSyncRouting(t *testing.T) {
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
		// Integration service additions.
		{"PushSelection", func() error {
			_, _, e := c.Integrations.PushSelection(ctx, "cn_1", &IntegrationContactSelection{
				All: true, Filters: &ContactSearchParams{Query: "acme"}, Exclude: []string{"ct_9"},
			})
			return e
		}, "POST", "/v1/integrations/connections/cn_1/push", "", []string{`"all":true`, `"filters":{"query":"acme"}`, `"exclude":["ct_9"]`}},
		{"RotateInboundURL", func() error { _, _, e := c.Integrations.RotateInboundURL(ctx, "cn_1"); return e },
			"POST", "/v1/integrations/connections/cn_1/rotate-inbound-url", "", nil},
		{"SetSigningKey", func() error { _, _, e := c.Integrations.SetSigningKey(ctx, "cn_1", "s3cret-key"); return e },
			"PUT", "/v1/integrations/connections/cn_1/signing-key", "", []string{`"signing_key":"s3cret-key"`}},
		{"StartOAuthWith", func() error {
			_, _, e := c.Integrations.StartOAuthWith(ctx, &OAuthStartParams{Provider: ProviderSalesforce, Environment: SalesforceEnvironmentSandbox, Domain: "acme.my.salesforce.com"})
			return e
		}, "POST", "/v1/integrations/oauth/start", "", []string{`"provider":"salesforce"`, `"environment":"sandbox"`, `"domain":"acme.my.salesforce.com"`}},
		{"Community", func() error {
			_, e := c.Integrations.Community(ctx, &ListOptions{Limit: 20, Cursor: "o20"})
			return e
		}, "GET", "/v1/integrations/community", "cursor=o20&limit=20", nil},
		{"CommunityApp", func() error { _, _, e := c.Integrations.CommunityApp(ctx, "my-app"); return e },
			"GET", "/v1/integrations/community/my-app", "", nil},

		// Salesforce.
		{"Salesforce.Overview", func() error {
			_, _, e := c.Salesforce.Overview(ctx, "cn_1", &SalesforceOverviewParams{Checks: true})
			return e
		}, "GET", "/v1/integrations/salesforce/cn_1/overview", "checks=1", nil},
		{"Salesforce.Settings", func() error { _, _, e := c.Salesforce.Settings(ctx, "cn_1"); return e },
			"GET", "/v1/integrations/salesforce/cn_1/settings", "", nil},
		{"Salesforce.UpdateSettings", func() error {
			_, _, e := c.Salesforce.UpdateSettings(ctx, "cn_1", &SalesforceSettings{Enabled: true, DailyAPIBudget: 500})
			return e
		}, "PUT", "/v1/integrations/salesforce/cn_1/settings", "", []string{`"enabled":true`, `"daily_api_budget":500`}},
		{"Salesforce.Metadata", func() error { _, _, e := c.Salesforce.Metadata(ctx, "cn_1"); return e },
			"GET", "/v1/integrations/salesforce/cn_1/metadata", "", nil},
		{"Salesforce.Users", func() error { _, _, e := c.Salesforce.Users(ctx, "cn_1", "ada"); return e },
			"GET", "/v1/integrations/salesforce/cn_1/users", "q=ada", nil},
		{"Salesforce.ListViews", func() error { _, _, e := c.Salesforce.ListViews(ctx, "cn_1", SalesforceObjectLead); return e },
			"GET", "/v1/integrations/salesforce/cn_1/list-views", "object=Lead", nil},
		{"Salesforce.Campaigns", func() error { _, _, e := c.Salesforce.Campaigns(ctx, "cn_1", "spring"); return e },
			"GET", "/v1/integrations/salesforce/cn_1/campaigns", "q=spring", nil},
		{"Salesforce.PreviewImport", func() error {
			_, _, e := c.Salesforce.PreviewImport(ctx, "cn_1", &SalesforceImportRef{SourceKind: SalesforceSourceListView, Object: SalesforceObjectContact, SourceID: "00B1"})
			return e
		}, "POST", "/v1/integrations/salesforce/cn_1/import/preview", "", []string{`"source_kind":"list_view"`, `"object":"Contact"`, `"source_id":"00B1"`}},
		{"Salesforce.ImportSources", func() error { _, _, e := c.Salesforce.ImportSources(ctx, "cn_1"); return e },
			"GET", "/v1/integrations/salesforce/cn_1/import-sources", "", nil},
		{"Salesforce.CreateImportSource", func() error {
			_, _, e := c.Salesforce.CreateImportSource(ctx, "cn_1", &SalesforceImportSourceParams{SourceKind: SalesforceSourceCampaign, SourceID: "701x", CampaignID: String("camp_1"), Recurring: Bool(true)})
			return e
		}, "POST", "/v1/integrations/salesforce/cn_1/import-sources", "", []string{`"source_kind":"campaign"`, `"campaign_id":"camp_1"`, `"recurring":true`}},
		{"Salesforce.UpdateImportSource clear", func() error {
			_, _, e := c.Salesforce.UpdateImportSource(ctx, "cn_1", "src_1", &SalesforceImportSourceParams{ClearCampaign: true, Enabled: Bool(false)})
			return e
		}, "PATCH", "/v1/integrations/salesforce/cn_1/import-sources/src_1", "", []string{`"campaign_id":null`, `"enabled":false`}},
		{"Salesforce.RunImportSource", func() error { _, _, e := c.Salesforce.RunImportSource(ctx, "cn_1", "src_1"); return e },
			"POST", "/v1/integrations/salesforce/cn_1/import-sources/src_1/run", "", nil},
		{"Salesforce.DeleteImportSource", func() error { _, e := c.Salesforce.DeleteImportSource(ctx, "cn_1", "src_1"); return e },
			"DELETE", "/v1/integrations/salesforce/cn_1/import-sources/src_1", "", nil},
		{"Salesforce.Activity", func() error {
			_, e := c.Salesforce.Activity(ctx, "cn_1", &SalesforceActivityParams{ListOptions: ListOptions{Limit: 25}, Status: SalesforceActivityFailed, ContactID: "ct_1"})
			return e
		}, "GET", "/v1/integrations/salesforce/cn_1/activity", "contact_id=ct_1&limit=25&status=failed", nil},
		{"Salesforce.RetryActivity", func() error { _, _, e := c.Salesforce.RetryActivity(ctx, "cn_1", []string{"a1"}); return e },
			"POST", "/v1/integrations/salesforce/cn_1/activity/retry", "", []string{`"ids":["a1"]`}},
		{"Salesforce.SyncNow", func() error { _, e := c.Salesforce.SyncNow(ctx, "cn_1"); return e },
			"POST", "/v1/integrations/salesforce/cn_1/sync-now", "", nil},
		{"Salesforce.ContactPanel", func() error { _, _, e := c.Salesforce.ContactPanel(ctx, "ct_1"); return e },
			"GET", "/v1/contacts/ct_1/salesforce", "", nil},
		{"Salesforce.SyncContact", func() error {
			_, _, e := c.Salesforce.SyncContact(ctx, "ct_1", &SalesforceContactSyncParams{ConnectionID: String("cn_1"), CreateAs: "lead"})
			return e
		}, "POST", "/v1/contacts/ct_1/salesforce/sync", "", []string{`"connection_id":"cn_1"`, `"create_as":"lead"`}},
		{"Salesforce.UnlinkContact", func() error { _, e := c.Salesforce.UnlinkContact(ctx, "ct_1", "lk_1"); return e },
			"DELETE", "/v1/contacts/ct_1/salesforce/links/lk_1", "", nil},

		// Apollo.
		{"Apollo.Overview", func() error { _, _, e := c.Apollo.Overview(ctx, "cn_2"); return e },
			"GET", "/v1/integrations/apollo/cn_2", "", nil},
		{"Apollo.ReplaceKey", func() error { _, _, e := c.Apollo.ReplaceKey(ctx, "cn_2", "key-123"); return e },
			"PUT", "/v1/integrations/apollo/cn_2/key", "", []string{`"api_key":"key-123"`}},
		{"Apollo.Lists", func() error { _, _, e := c.Apollo.Lists(ctx, "cn_2"); return e },
			"GET", "/v1/integrations/apollo/cn_2/lists", "", nil},
		{"Apollo.PreviewImport", func() error {
			_, _, e := c.Apollo.PreviewImport(ctx, "cn_2", &ApolloImportPreviewParams{ListID: "l1", VerifiedOnly: Bool(false)})
			return e
		}, "POST", "/v1/integrations/apollo/cn_2/import/preview", "", []string{`"list_id":"l1"`, `"verified_only":false`}},
		{"Apollo.ImportSources", func() error { _, _, e := c.Apollo.ImportSources(ctx, "cn_2"); return e },
			"GET", "/v1/integrations/apollo/cn_2/import-sources", "", nil},
		{"Apollo.CreateImportSource", func() error {
			_, _, e := c.Apollo.CreateImportSource(ctx, "cn_2", &ApolloImportSourceParams{ListID: "l1", ListName: "Hot", CategoryIDs: []string{"cat_1"}})
			return e
		}, "POST", "/v1/integrations/apollo/cn_2/import-sources", "", []string{`"list_id":"l1"`, `"category_ids":["cat_1"]`}},
		{"Apollo.UpdateImportSource", func() error {
			_, _, e := c.Apollo.UpdateImportSource(ctx, "cn_2", "src_2", &ApolloImportSourceParams{ClearCampaign: true})
			return e
		}, "PATCH", "/v1/integrations/apollo/cn_2/import-sources/src_2", "", []string{`"campaign_id":null`}},
		{"Apollo.RunImportSource", func() error { _, _, e := c.Apollo.RunImportSource(ctx, "cn_2", "src_2"); return e },
			"POST", "/v1/integrations/apollo/cn_2/import-sources/src_2/run", "", nil},
		{"Apollo.DeleteImportSource", func() error { _, e := c.Apollo.DeleteImportSource(ctx, "cn_2", "src_2"); return e },
			"DELETE", "/v1/integrations/apollo/cn_2/import-sources/src_2", "", nil},
		{"Apollo.Enrichments", func() error { _, _, e := c.Apollo.Enrichments(ctx, "cn_2", 5); return e },
			"GET", "/v1/integrations/apollo/cn_2/enrichments", "limit=5", nil},
		{"Apollo.Enrichment", func() error { _, _, e := c.Apollo.Enrichment(ctx, "cn_2", "run_1"); return e },
			"GET", "/v1/integrations/apollo/cn_2/enrichments/run_1", "", nil},
		{"Apollo.StartEnrichment", func() error {
			_, _, e := c.Apollo.StartEnrichment(ctx, "cn_2", &ApolloEnrichParams{
				IntegrationContactSelection: IntegrationContactSelection{Contacts: []string{"ct_1"}},
				Overwrite:                   true,
			})
			return e
		}, "POST", "/v1/integrations/apollo/cn_2/enrichments", "", []string{`"contacts":["ct_1"]`, `"overwrite":true`}},

		// Clay.
		{"Clay.Settings", func() error { _, _, e := c.Clay.Settings(ctx, "cn_3"); return e },
			"GET", "/v1/integrations/clay/cn_3", "", nil},
		{"Clay.UpdateSettings", func() error {
			_, _, e := c.Clay.UpdateSettings(ctx, "cn_3", &ClaySettingsUpdateParams{
				Defaults:   &ClayInboundDefaults{Dedup: ImportDedupUpdate, Subscribed: true},
				WebhookURL: String("https://clay.example/hook"),
				Events:     &[]string{"reply.received"},
			})
			return e
		}, "PATCH", "/v1/integrations/clay/cn_3", "", []string{`"dedup":"update"`, `"subscribed":true`, `"webhook_url":"https://clay.example/hook"`, `"events":["reply.received"]`}},
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

func TestSalesforceImportSourceParamsOmitCampaign(t *testing.T) {
	raw, err := json.Marshal(SalesforceImportSourceParams{SourceKind: SalesforceSourceListView, SourceID: "00B1"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "campaign_id") || strings.Contains(string(raw), "ClearCampaign") {
		t.Fatalf("unset campaign leaked into %s", raw)
	}
}

func TestSalesforceSettingsResultDecodes(t *testing.T) {
	var got recordedRequest
	want := struct {
		Settings      SalesforceSettings   `json:"settings"`
		WarmblyFields []SalesforceFieldDef `json:"warmbly_fields"`
		Defaults      SalesforceSettings   `json:"defaults"`
	}{
		Settings: SalesforceSettings{
			Enabled:   true,
			Matching:  SalesforceMatchingSettings{Prefer: "contact", CreateWhen: "reply", CreateAs: "lead", Owner: "connected_user"},
			Activity:  SalesforceActivitySettings{Sent: true, AssignTo: "record_owner"},
			Writeback: SalesforceWritebackSettings{LeadStatusOnReply: map[string]string{"positive": "Working"}, NeverMoveBackwards: true},
			Inbound:   SalesforceInboundSettings{OptOut: "both", PauseOnStatuses: []string{}},
			FieldMap:  []SalesforceFieldRule{{Object: SalesforceObjectLead, Warmbly: "first_name", Salesforce: "FirstName", Direction: SalesforceDirectionBoth, Policy: SalesforcePolicyIfEmpty}},
		},
		WarmblyFields: []SalesforceFieldDef{{Key: "first_name", Label: "First name"}},
	}
	c := crmServe(t, &got, 200, want)
	out, _, err := c.Salesforce.Settings(context.Background(), "cn_1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out.Settings, want.Settings) || !reflect.DeepEqual(out.WarmblyFields, want.WarmblyFields) {
		t.Fatalf("decoded %+v", out)
	}

	// Saving answers {"settings": ...}.
	c = crmServe(t, &got, 200, map[string]any{"settings": want.Settings})
	saved, _, err := c.Salesforce.UpdateSettings(context.Background(), "cn_1", &want.Settings)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*saved, want.Settings) {
		t.Fatalf("saved %+v", saved)
	}
}

func TestSalesforcePanelDecodes(t *testing.T) {
	var got recordedRequest
	amount := 12000.5
	synced := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	want := SalesforcePanel{
		Connections: []SalesforcePanelConnection{{ID: "cn_1", Label: "Prod", Environment: "production", InstanceURL: "https://acme.my.salesforce.com"}},
		Records: []SalesforceRecord{{
			LinkID: "lk_1", ConnectionID: "cn_1", ConnectionLabel: "Prod", Object: SalesforceObjectContact, ID: "003x", URL: "https://acme/003x", Name: "Ada Lovelace",
			Owner:         &SalesforceRef{ID: "005x", Name: "Sam"},
			Account:       &SalesforceAccountRef{ID: "001x", Name: "Acme", URL: "https://acme/001x"},
			Opportunities: []SalesforceOpportunity{{ID: "006x", Name: "Renewal", Stage: "Proposal", Amount: &amount, IsClosed: false}},
			Tasks:         []SalesforceTask{{ID: "00Tx", Subject: "Email: Hi", Status: "Completed", FromWarmbly: true}},
			LinkedBy:      "email", LastSyncedAt: &synced, Sync: SalesforcePanelSync{Synced: 3},
		}},
		CanSync: true,
	}
	c := crmServe(t, &got, 200, want)
	out, _, err := c.Salesforce.SyncContact(context.Background(), "ct_1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*out, want) {
		t.Fatalf("decoded %+v, want %+v", *out, want)
	}
	if got.body != "" {
		t.Fatalf("nil params sent body %q", got.body)
	}
}

func TestSalesforceActivityPages(t *testing.T) {
	var got recordedRequest
	next := "cur2"
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	cid := "ct_1"
	c := crmServe(t, &got, 200, Page[SalesforceActivity]{
		Data:       []SalesforceActivity{{ID: "a1", ContactID: &cid, ContactEmail: "ada@acme.com", Kind: "replied", Status: SalesforceActivityFailed, Attempts: 3, Detail: "FIELD_CUSTOM_VALIDATION", OccurredAt: now, CreatedAt: now, NextAttemptAt: now}},
		Pagination: Pagination{NextCursor: &next, HasMore: true},
	})
	page, err := c.Salesforce.Activity(context.Background(), "cn_1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 1 || page.Data[0].Status != SalesforceActivityFailed || page.NextCursor() != "cur2" {
		t.Fatalf("page = %+v", page)
	}
}

func TestApolloEnrichmentDecodes(t *testing.T) {
	var got recordedRequest
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	want := ApolloEnrichmentRun{
		ID: "run_1", OrganizationID: "org_1", ConnectionID: "cn_2", Status: ApolloEnrichmentRunning,
		Total: 100, Processed: 40, Enriched: 30, NotFound: 10, CreditsConsumed: 30, StartedAt: &now, CreatedAt: now, UpdatedAt: now,
	}
	c := crmServe(t, &got, 202, want)
	out, resp, err := c.Apollo.StartEnrichment(context.Background(), "cn_2", &ApolloEnrichParams{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 202 || !reflect.DeepEqual(*out, want) {
		t.Fatalf("decoded %+v (%d), want %+v", *out, resp.StatusCode, want)
	}
}

func TestApolloImportSourceDecodes(t *testing.T) {
	var got recordedRequest
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	camp := "camp_1"
	want := ApolloImportSource{
		ID: "src_2", ConnectionID: "cn_2", Name: "Hot list", ListID: "l1", ListName: "Hot", CampaignID: &camp,
		CategoryIDs: []string{"cat_1"}, VerifiedOnly: true, Recurring: true, Enabled: true, Status: ImportSourceIdle,
		LastRunAt: &now, LastResult: &ApolloRunResult{Read: 10, Imported: 8, AlreadySeen: 2}, TotalImported: 8, CreatedAt: now, UpdatedAt: now,
	}
	c := crmServe(t, &got, 200, map[string]any{"data": []ApolloImportSource{want}})
	out, _, err := c.Apollo.ImportSources(context.Background(), "cn_2")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || !reflect.DeepEqual(out[0], want) {
		t.Fatalf("decoded %+v", out)
	}
}

func TestClaySettingsDecodes(t *testing.T) {
	var got recordedRequest
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	want := ClaySettings{
		ConnectionID: "cn_3", Label: "Clay", Status: ConnectionConnected, Health: "healthy", InboundURLMinted: true,
		Defaults:        ClayInboundDefaults{CategoryIDs: []string{}, SegmentIDs: []string{"seg_1"}, Dedup: ImportDedupSkip, Subscribed: true},
		Outbound:        ClayOutbound{WebhookURLSet: true, WebhookHost: "clay.example", Events: []string{"reply.received"}},
		Stats:           ClayStats{Received: 5, Created: 4, LastReceivedAt: &now},
		AvailableEvents: []string{"reply.received", "email.bounced"},
	}
	c := crmServe(t, &got, 200, want)
	out, _, err := c.Clay.Settings(context.Background(), "cn_3")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*out, want) {
		t.Fatalf("decoded %+v, want %+v", *out, want)
	}
}

func TestIntegrationCommunityDecodes(t *testing.T) {
	var got recordedRequest
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	app := CommunityApp{
		ApplicationID: "app_1", Slug: "my-app", Name: "My App", Category: "ai", Developer: "Acme",
		Scopes: 3, Permissions: []Permission{{Name: "READ_CONTACTS", Value: 1, Category: "read"}},
		Status: "published", Listed: true, Installs: 12, Installed: false, PublishedAt: now,
	}
	total := int64(1)
	c := crmServe(t, &got, 200, Page[CommunityApp]{Data: []CommunityApp{app}, Pagination: Pagination{Total: &total}})
	page, err := c.Integrations.Community(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 1 || !reflect.DeepEqual(page.Data[0], app) {
		t.Fatalf("page = %+v", page.Data)
	}
}

func TestIntegrationRotateAndSigningKeyResults(t *testing.T) {
	var got recordedRequest
	c := crmServe(t, &got, 200, map[string]string{"inbound_webhook_url": "https://api.example/v1/integrations/inbound/calendly/abc"})
	url, _, err := c.Integrations.RotateInboundURL(context.Background(), "cn_1")
	if err != nil || !strings.HasSuffix(url, "/calendly/abc") {
		t.Fatalf("RotateInboundURL = %q, %v", url, err)
	}

	c = crmServe(t, &got, 200, map[string]any{"connection": IntegrationConnection{ID: "cn_1", Provider: ProviderCalendly, Status: ConnectionConnected}})
	conn, _, err := c.Integrations.SetSigningKey(context.Background(), "cn_1", "")
	if err != nil || conn.ID != "cn_1" {
		t.Fatalf("SetSigningKey = %+v, %v", conn, err)
	}
	if !strings.Contains(got.body, `"signing_key":""`) {
		t.Fatalf("empty key must still be sent to clear it, body %s", got.body)
	}
}

func TestIntegrationCatalogRankDecodes(t *testing.T) {
	var got recordedRequest
	c := crmServe(t, &got, 200, map[string]any{"catalog": []IntegrationCatalogEntry{{Provider: ProviderApollo, Name: "Apollo", AuthMethod: "api_key", Rank: 3}}})
	entries, _, err := c.Integrations.Catalog(context.Background())
	if err != nil || len(entries) != 1 || entries[0].Rank != 3 {
		t.Fatalf("Catalog = %+v, %v", entries, err)
	}
}
