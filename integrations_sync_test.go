package warmbly

import (
	"context"
	"net/http"
	"testing"
)

func TestIntegrationConnectionSyncRouting(t *testing.T) {
	var got recordedRequest
	c := crmSyncServer(t, http.StatusOK, envelopeBody, &got)
	ctx := context.Background()

	runExactCases(t, &got, []exactCase{
		{"SetSigningKey", func() error {
			_, _, e := c.Integrations.SetSigningKey(ctx, "conn_1", "whsec_abcdefgh")
			return e
		}, "PUT", "/v1/integrations/connections/conn_1/signing-key", "", []string{`{"signing_key":"whsec_abcdefgh"}`}, false},
		{"SetSigningKey clear", func() error {
			_, _, e := c.Integrations.SetSigningKey(ctx, "conn_1", "")
			return e
		}, "PUT", "/v1/integrations/connections/conn_1/signing-key", "", []string{`{"signing_key":""}`}, false},
		{"RotateInboundURL", func() error {
			_, _, e := c.Integrations.RotateInboundURL(ctx, "conn_1")
			return e
		}, "POST", "/v1/integrations/connections/conn_1/rotate-inbound-url", "", nil, true},
		{"PushSelection ids", func() error {
			_, _, e := c.Integrations.PushSelection(ctx, "conn_1", &IntegrationPushParams{Contacts: []string{"ct_1"}})
			return e
		}, "POST", "/v1/integrations/connections/conn_1/push", "", []string{`{"contacts":["ct_1"]}`}, false},
		{"PushSelection filter", func() error {
			_, _, e := c.Integrations.PushSelection(ctx, "conn_1", &IntegrationPushParams{
				All:     true,
				Filters: &ContactSearchParams{ListOptions: ListOptions{Limit: 5}, Category: "cat_1", Query: "acme"},
				Exclude: []string{"ct_9"},
			})
			return e
		}, "POST", "/v1/integrations/connections/conn_1/push", "", []string{
			`{"all":true,"filters":{"query":"acme"},"exclude":["ct_9"]}`,
		}, false},
		{"StartOAuthWithParams", func() error {
			_, _, e := c.Integrations.StartOAuthWithParams(ctx, &OAuthStartParams{
				Provider: ProviderSalesforce, Label: "Sandbox", Environment: SalesforceSandbox, Domain: "acme--dev.sandbox.my.salesforce.com",
			})
			return e
		}, "POST", "/v1/integrations/oauth/start", "", []string{
			`{"provider":"salesforce","label":"Sandbox","environment":"sandbox","domain":"acme--dev.sandbox.my.salesforce.com"}`,
		}, false},
		{"StartOAuth", func() error {
			_, _, e := c.Integrations.StartOAuth(ctx, ProviderHubSpot, "")
			return e
		}, "POST", "/v1/integrations/oauth/start", "", []string{`{"provider":"hubspot"}`}, false},
		{"CommunityApps", func() error {
			_, e := c.Integrations.CommunityApps(ctx, &CommunityAppListParams{ListOptions: ListOptions{Limit: 20, Cursor: "o20"}})
			return e
		}, "GET", "/v1/integrations/community", "cursor=o20&limit=20", nil, true},
		{"CommunityApps nil", func() error {
			_, e := c.Integrations.CommunityApps(ctx, nil)
			return e
		}, "GET", "/v1/integrations/community", "", nil, true},
		{"CommunityApp", func() error {
			_, _, e := c.Integrations.CommunityApp(ctx, "lead-scorer")
			return e
		}, "GET", "/v1/integrations/community/lead-scorer", "", nil, true},
	})
}

func TestIntegrationInboundDecode(t *testing.T) {
	var got recordedRequest
	c := crmSyncServer(t, http.StatusOK, `{"inbound_webhook_url": "https://api.warmbly.com/api/v1/integrations/inbound/calendly/s3cr3t"}`, &got)
	u, _, err := c.Integrations.RotateInboundURL(context.Background(), "conn_1")
	if err != nil {
		t.Fatal(err)
	}
	if u != "https://api.warmbly.com/api/v1/integrations/inbound/calendly/s3cr3t" {
		t.Errorf("url = %q", u)
	}

	body := `{"connection": {"id": "conn_1", "organization_id": "org_1", "provider": "calendly", "label": "Calendly",
		"status": "connected", "auth_method": "webhook", "display_fields": {}, "sync_direction": "pull", "health": "healthy",
		"created_at": "2026-09-01T10:00:00Z", "updated_at": "2026-09-03T12:00:00Z"}}`
	c = crmSyncServer(t, http.StatusOK, body, &got)
	conn, _, err := c.Integrations.SetSigningKey(context.Background(), "conn_1", "whsec_abcdefgh")
	if err != nil {
		t.Fatal(err)
	}
	if conn == nil || conn.Provider != ProviderCalendly || conn.SyncDirection != SyncPull {
		t.Errorf("connection = %+v", conn)
	}
}

func TestCommunityAppDecode(t *testing.T) {
	var got recordedRequest
	app := `{
		"application_id": "app_1", "slug": "lead-scorer", "name": "Lead Scorer", "tagline": "Score replies",
		"description": "Scores every reply.", "category": "ai", "logo_url": "https://cdn.example.com/l.png",
		"website_url": "https://example.com", "install_url": "https://example.com/install",
		"support_url": "https://example.com/help", "privacy_url": "https://example.com/privacy",
		"developer": "Example Inc", "scopes": 5,
		"permissions": [{"name": "READ_EMAILS", "value": 1, "description": "View email accounts and settings", "category": "read"},
			{"name": "READ_CONTACTS", "value": 4, "description": "View contacts", "category": "read"}],
		"status": "featured", "listed": true, "installs": 42, "installed": false,
		"published_at": "2026-08-01T00:00:00Z"
	}`
	c := crmSyncServer(t, http.StatusOK, `{"data": [`+app+`], "pagination": {"total": 1, "next_cursor": null, "has_more": false}}`, &got)
	page, err := c.Integrations.CommunityApps(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 1 || page.Pagination.Total == nil || *page.Pagination.Total != 1 || page.HasMore() {
		t.Fatalf("page = %+v", page)
	}
	a := page.Data[0]
	if a.Status != CommunityAppFeatured || !a.Listed || a.Installs != 42 || a.Scopes != 5 || a.PublishedAt.IsZero() {
		t.Errorf("app = %+v", a)
	}
	if len(a.Permissions) != 2 || a.Permissions[1].Name != "READ_CONTACTS" || a.Permissions[1].Category != "read" {
		t.Errorf("permissions = %+v", a.Permissions)
	}

	c = crmSyncServer(t, http.StatusOK, app, &got)
	one, _, err := c.Integrations.CommunityApp(context.Background(), "lead-scorer")
	if err != nil {
		t.Fatal(err)
	}
	if one.Slug != "lead-scorer" || one.InstallURL == "" {
		t.Errorf("app = %+v", one)
	}
}

func TestIntegrationCatalogRankDecode(t *testing.T) {
	var got recordedRequest
	body := `{"catalog": [{"provider": "cleanmylist", "name": "CleanMyList", "category": "verification",
		"auth_method": "api_key", "beta": false, "supports_push": false, "configured": true, "rank": 3}]}`
	c := crmSyncServer(t, http.StatusOK, body, &got)
	cat, _, err := c.Integrations.Catalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cat) != 1 || cat[0].Provider != ProviderCleanMyList || cat[0].Rank != 3 {
		t.Errorf("catalog = %+v", cat)
	}
}
