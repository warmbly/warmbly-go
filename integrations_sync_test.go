package warmbly

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestIntegrationRotateInboundURL(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, `{"inbound_webhook_url":"https://api.example.com/v1/inbound/calendly/s3cr3t"}`)
	url, _, err := c.Integrations.RotateInboundURL(context.Background(), "conn_1")
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "POST", "/v1/integrations/connections/conn_1/rotate-inbound-url", "")
	got.wantBody(t, "")
	if url != "https://api.example.com/v1/inbound/calendly/s3cr3t" {
		t.Errorf("url = %q", url)
	}
}

func TestIntegrationSetInboundSigningKey(t *testing.T) {
	updated := time.Date(2026, 9, 4, 7, 0, 0, 0, time.UTC)
	conn := IntegrationConnection{
		ID: "conn_1", OrganizationID: "org_1", Provider: ProviderCalendly, Status: ConnectionConnected,
		AuthMethod: "webhook", SyncDirection: SyncPull, Health: "healthy",
		InboundWebhookURL: "https://api.example.com/v1/inbound/calendly/s3cr3t", CreatedAt: updated, UpdatedAt: updated,
	}
	var got syncCapture
	c := respondingClient(t, &got, 200, jsonOf(t, map[string]any{"connection": conn}))
	out, _, err := c.Integrations.SetInboundSigningKey(context.Background(), "conn_1", "whsec_0123456789")
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "PUT", "/v1/integrations/connections/conn_1/signing-key", "")
	got.wantBody(t, `{"signing_key":"whsec_0123456789"}`)
	if out.ID != "conn_1" || out.Provider != ProviderCalendly || out.InboundWebhookURL == "" || !out.UpdatedAt.Equal(updated) {
		t.Errorf("connection = %+v", out)
	}

	// An empty key is sent as an empty string: it clears the key.
	if _, _, err := c.Integrations.SetInboundSigningKey(context.Background(), "conn_1", ""); err != nil {
		t.Fatal(err)
	}
	got.wantBody(t, `{"signing_key":""}`)
}

func TestIntegrationDecodesNewProviderAndRedactedConfig(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, `{"catalog":[],"connections":[{"id":"c1","provider":"cleanmylist","status":"connected","config_capabilities":{"use_cases":["verify"]},"sync_direction":"pull","health":"healthy","created_at":"2026-09-04T07:00:00Z","updated_at":"2026-09-04T07:00:00Z"}]}`)
	conns, _, err := c.Integrations.Connections(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 1 || conns[0].Provider != ProviderCleanMyList {
		t.Errorf("connections = %+v", conns)
	}
}

func TestIntegrationPushStillSendsContactIDs(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, `{"provider":"hubspot","pushed":2,"failed":0,"results":[]}`)
	if _, _, err := c.Integrations.Push(context.Background(), "conn_1", []string{"ct_1", "ct_2"}); err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "POST", "/v1/integrations/connections/conn_1/push", "")
	got.wantBody(t, `{"contact_ids":["ct_1","ct_2"]}`)
}

func TestIntegrationOAuthAndCommunityRouting(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, http.StatusOK, envelopeBody)
	ctx := context.Background()

	runExactCases(t, &got, []exactCase{
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

func TestCommunityAppDecode(t *testing.T) {
	var got syncCapture
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
	c := respondingClient(t, &got, http.StatusOK, `{"data": [`+app+`], "pagination": {"total": 1, "next_cursor": null, "has_more": false}}`)
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

	c = respondingClient(t, &got, http.StatusOK, app)
	one, _, err := c.Integrations.CommunityApp(context.Background(), "lead-scorer")
	if err != nil {
		t.Fatal(err)
	}
	if one.Slug != "lead-scorer" || one.InstallURL == "" {
		t.Errorf("app = %+v", one)
	}
}

func TestIntegrationCatalogRankDecode(t *testing.T) {
	var got syncCapture
	body := `{"catalog": [{"provider": "cleanmylist", "name": "CleanMyList", "category": "verification",
		"auth_method": "api_key", "beta": false, "supports_push": false, "configured": true, "rank": 3}]}`
	c := respondingClient(t, &got, http.StatusOK, body)
	cat, _, err := c.Integrations.Catalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cat) != 1 || cat[0].Provider != ProviderCleanMyList || cat[0].Rank != 3 {
		t.Errorf("catalog = %+v", cat)
	}
}
