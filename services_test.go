package warmbly

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestServiceRouting exercises one representative call per service against a
// recording server and asserts the HTTP method and path. It guards against
// typos in the resource paths across all service files.
func TestServiceRouting(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		// Universally decodable into *Page[T], *T, or {"data":[...]} shapes.
		_, _ = w.Write([]byte(`{"data":[],"pagination":{"has_more":false}}`))
	}))
	defer srv.Close()

	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()

	cases := []struct {
		name       string
		call       func() error
		wantMethod string
		wantPath   string
	}{
		{"apikeys.List", func() error { _, e := c.APIKeys.List(ctx, nil); return e }, http.MethodGet, "/v1/api-keys"},
		{"oauthapps.List", func() error { _, e := c.OAuthApps.List(ctx, nil); return e }, http.MethodGet, "/v1/oauth/applications"},
		{"emails.List", func() error { _, e := c.Emails.List(ctx, nil); return e }, http.MethodGet, "/v1/emails"},
		{"emails.StartWarmup", func() error { _, _, e := c.Emails.StartWarmup(ctx, "em_1"); return e }, http.MethodPost, "/v1/emails/em_1/warmup/start"},
		{"campaigns.List", func() error { _, e := c.Campaigns.List(ctx, nil); return e }, http.MethodGet, "/v1/campaigns"},
		{"campaigns.ListSteps", func() error { _, _, e := c.Campaigns.ListSteps(ctx, "camp_1"); return e }, http.MethodGet, "/v1/campaigns/camp_1/steps"},
		{"contacts.Search", func() error { _, e := c.Contacts.Search(ctx, &ContactSearchParams{}); return e }, http.MethodPost, "/v1/contacts/search"},
		{"contacts.Get", func() error { _, _, e := c.Contacts.Get(ctx, "ct_1"); return e }, http.MethodGet, "/v1/contacts/ct_1"},
		{"webhooks.List", func() error { _, e := c.Webhooks.List(ctx, nil); return e }, http.MethodGet, "/v1/webhooks"},
		{"webhooks.EventTypes", func() error { _, _, e := c.Webhooks.EventTypes(ctx); return e }, http.MethodGet, "/v1/webhooks/event-types"},
		{"templates.List", func() error { _, e := c.Templates.List(ctx, nil); return e }, http.MethodGet, "/v1/templates"},
		{"analytics.Dashboard", func() error { _, _, e := c.Analytics.Dashboard(ctx, nil); return e }, http.MethodGet, "/v1/analytics/dashboard"},
		{"organization.Current", func() error { _, _, e := c.Organization.Current(ctx); return e }, http.MethodGet, "/v1/organization/current"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotMethod, gotPath = "", ""
			if err := tc.call(); err != nil {
				t.Fatalf("call: %v", err)
			}
			if gotMethod != tc.wantMethod {
				t.Errorf("method = %s, want %s", gotMethod, tc.wantMethod)
			}
			if gotPath != tc.wantPath {
				t.Errorf("path = %s, want %s", gotPath, tc.wantPath)
			}
		})
	}
}
