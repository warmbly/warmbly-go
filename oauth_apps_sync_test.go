package warmbly

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// oauthAppBody is an application as models.OAuthApplication marshals it,
// suspended by an operator.
const oauthAppBody = `{
	"id": "5b0c8f0e-6f2a-4f43-9f43-2f7f0b8a1c10",
	"organization_id": "1d2e3f40-1111-4222-8333-944455566677",
	"created_by": "2e3f4051-2222-4333-8444-a55566677788",
	"name": "Acme Sync",
	"description": "Two-way CRM sync",
	"logo_url": "https://cdn.warmbly.test/oauth-app-logos/5b0c8f0e-1.png",
	"website_url": "https://acme.example",
	"client_id": "wmcid_abc",
	"redirect_uris": ["https://acme.example/cb"],
	"allowed_webhook_domains": [],
	"webhook_url": "",
	"webhook_events": [],
	"scopes": 3,
	"status": "disabled",
	"is_public": false,
	"dynamically_registered": false,
	"suspended_at": "2026-10-02T12:00:00Z",
	"suspended_reason": "phishing reports",
	"created_at": "2026-09-01T00:00:00Z",
	"updated_at": "2026-10-02T12:00:00Z"
}`

func TestOAuthAppSync(t *testing.T) {
	ctx := context.Background()
	const listing = `{"listing": {
		"application_id": "5b0c8f0e-6f2a-4f43-9f43-2f7f0b8a1c10",
		"organization_id": "1d2e3f40-1111-4222-8333-944455566677",
		"slug": "acme-sync",
		"tagline": "Sync Warmbly with Acme",
		"description": "",
		"category": "crm",
		"install_url": "https://acme.example/install",
		"support_url": "",
		"privacy_url": "",
		"status": "hidden",
		"status_note": "misleading screenshots",
		"status_at": "2026-10-03T00:00:00Z",
		"submitted_at": "2026-09-20T00:00:00Z",
		"created_at": "2026-09-20T00:00:00Z",
		"updated_at": "2026-10-03T00:00:00Z"
	}}`

	runWireCases(t, []wireCase{
		{
			name: "Get decodes suspension", response: oauthAppBody,
			call: func(c *Client) error {
				app, _, err := c.OAuthApps.Get(ctx, "app_1")
				if err == nil && (app.Status != OAuthAppDisabled || app.SuspendedAt == nil || app.SuspendedReason != "phishing reports") {
					err = errors.New("suspension not decoded")
				}
				return err
			},
			method: "GET", path: "/v1/oauth/applications/app_1",
		},
		{
			name: "DeveloperAccess", response: `{"applications": [], "developer_access": {"blocked": true, "reason": "abuse"}}`,
			call: func(c *Client) error {
				a, _, err := c.OAuthApps.DeveloperAccess(ctx)
				if err == nil && (!a.Blocked || a.Reason != "abuse") {
					err = errors.New("developer_access not decoded")
				}
				return err
			},
			method: "GET", path: "/v1/oauth/applications",
		},
		{
			name: "DeleteLogo", response: oauthAppBody,
			call: func(c *Client) error {
				app, _, err := c.OAuthApps.DeleteLogo(ctx, "app_1")
				if err == nil && app.ClientID != "wmcid_abc" {
					err = errors.New("application not decoded")
				}
				return err
			},
			method: "DELETE", path: "/v1/oauth/applications/app_1/logo",
		},
		{
			name: "Listing", response: listing,
			call: func(c *Client) error {
				l, _, err := c.OAuthApps.Listing(ctx, "app_1")
				if err == nil && (l == nil || l.Status != AppListingHidden || l.StatusAt == nil || l.Category != AppCategoryCRM) {
					err = errors.New("listing not decoded")
				}
				return err
			},
			method: "GET", path: "/v1/oauth/applications/app_1/listing",
		},
		{
			name: "Listing unpublished is nil", response: `{"listing": null}`,
			call: func(c *Client) error {
				l, _, err := c.OAuthApps.Listing(ctx, "app_1")
				if err == nil && l != nil {
					err = errors.New("want nil for an unpublished app")
				}
				return err
			},
			method: "GET", path: "/v1/oauth/applications/app_1/listing",
		},
		{
			name: "PublishListing", response: listing,
			call: func(c *Client) error {
				l, _, err := c.OAuthApps.PublishListing(ctx, "app_1", &AppListingParams{
					Slug: "acme-sync", Tagline: "Sync Warmbly with Acme", Category: AppCategoryCRM, InstallURL: "https://acme.example/install",
				})
				if err == nil && l.Slug != "acme-sync" {
					err = errors.New("listing not decoded")
				}
				return err
			},
			method: "PUT", path: "/v1/oauth/applications/app_1/listing",
			body: `{"slug":"acme-sync","tagline":"Sync Warmbly with Acme","description":"","category":"crm","install_url":"https://acme.example/install"}`,
		},
		{
			name: "UnpublishListing", response: `{"deleted": true}`,
			call: func(c *Client) error {
				_, err := c.OAuthApps.UnpublishListing(ctx, "app_1")
				return err
			},
			method: "DELETE", path: "/v1/oauth/applications/app_1/listing",
		},
		{
			name: "WorkspaceAuthorizations",
			response: `{"authorizations": [{
				"application_id": "5b0c8f0e-6f2a-4f43-9f43-2f7f0b8a1c10", "name": "Acme Sync", "logo_url": "", "website_url": "",
				"user_id": "2e3f4051-2222-4333-8444-a55566677788", "user_email": "sam@acme.com", "user_name": "Sam",
				"scopes": 33, "authorized_at": "2026-09-01T00:00:00Z", "last_used_at": null
			}]}`,
			call: func(c *Client) error {
				list, _, err := c.OAuthApps.WorkspaceAuthorizations(ctx)
				if err == nil && (len(list) != 1 || list[0].UserEmail != "sam@acme.com" || list[0].Scopes != 33 || list[0].LastUsedAt != nil) {
					err = errors.New("authorizations not decoded")
				}
				return err
			},
			method: "GET", path: "/v1/oauth/workspace-authorizations",
		},
		{
			name: "WorkspaceAuthorizations null is empty", response: `{"authorizations": null}`,
			call: func(c *Client) error {
				list, _, err := c.OAuthApps.WorkspaceAuthorizations(ctx)
				if err == nil && list == nil {
					err = errors.New("want a non-nil empty slice")
				}
				return err
			},
			method: "GET", path: "/v1/oauth/workspace-authorizations",
		},
		{
			name: "RevokeWorkspaceAuthorization", response: `{"revoked": true}`,
			call: func(c *Client) error {
				_, err := c.OAuthApps.RevokeWorkspaceAuthorization(ctx, "app_1", "user_9")
				return err
			},
			method: "DELETE", path: "/v1/oauth/workspace-authorizations/app_1/members/user_9",
		},
		{
			name: "AuthorizeDetails decodes the narrowed grant",
			response: `{"client_id": "wmcid_abc", "name": "Acme Sync", "description": "", "logo_url": "", "website_url": "",
				"redirect_uri": "https://acme.example/cb", "scopes": ["emails:read"], "withheld_scopes": ["api_keys"],
				"state": "s1", "organization_name": "Acme", "verified": true, "self_registered": false}`,
			call: func(c *Client) error {
				info, _, err := c.OAuthApps.AuthorizeDetails(ctx, nil)
				if err == nil && (len(info.WithheldScopes) != 1 || info.OrganizationName != "Acme" || !info.Verified || info.SelfRegistered) {
					err = errors.New("consent info not decoded")
				}
				return err
			},
			method: "GET", path: "/v1/oauth/authorize/details",
		},
	})
}

// TestOAuthAppLogoUploads asserts both logo uploads send the image in the
// "file" form field, which is the only one the server reads.
func TestOAuthAppLogoUploads(t *testing.T) {
	var path, field, filename string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		f, hdr, err := r.FormFile("file")
		if err == nil {
			field = "file"
			filename = hdr.Filename
			_, _ = io.Copy(io.Discard, f)
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/application-logo") {
			_, _ = w.Write([]byte(`{"logo_url": "https://cdn.warmbly.test/oauth-app-logos/org/abc.png"}`))
			return
		}
		_, _ = w.Write([]byte(oauthAppBody))
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	u, _, err := c.OAuthApps.UploadLogo(ctx, &FileUpload{Filename: "logo.png", Content: strings.NewReader("png")})
	if err != nil || u == "" || path != "/v1/oauth/application-logo" || field != "file" || filename != "logo.png" {
		t.Fatalf("UploadLogo: url=%q path=%q field=%q err=%v", u, path, field, err)
	}

	field = ""
	app, _, err := c.OAuthApps.SetLogo(ctx, "app_1", &FileUpload{Filename: "logo.jpg", Content: strings.NewReader("jpg")})
	if err != nil || app.ID == "" || path != "/v1/oauth/applications/app_1/logo" || field != "file" || filename != "logo.jpg" {
		t.Fatalf("SetLogo: path=%q field=%q err=%v", path, field, err)
	}
}

func TestOAuthAppSuspendedRefusal(t *testing.T) {
	c := emailFixtureClient(t, http.StatusConflict, `{"code":"app_suspended","message":"this app is suspended"}`, nil)
	_, _, err := c.OAuthApps.DeleteLogo(context.Background(), "app_1")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != ErrCodeAppSuspended || apiErr.StatusCode != http.StatusConflict {
		t.Fatalf("err = %v, want %q", err, ErrCodeAppSuspended)
	}
}
