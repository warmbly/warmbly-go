package warmbly

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const workspaceBrandFixture = `{
	"brand": {"display_name": "", "logo_url": "", "accent_color": "", "website_url": "", "hide_powered_by": false, "updated_at": "0001-01-01T00:00:00Z"},
	"inherited": {"display_name": "Northwind Agency", "logo_url": "https://cdn.test/brand-assets/a/logo.png", "accent_color": "#123456",
		"website_url": "https://northwind.test", "hide_powered_by": true, "updated_at": "2026-11-01T10:00:00Z", "updated_by": "usr_9"},
	"effective": {
		"brand": {"name": "Northwind Agency", "logo_url": "https://cdn.test/brand-assets/a/logo.png", "accent_color": "#123456",
			"icon_url": "https://cdn.test/brand-assets/a/icon.png", "theme": {"font": "serif", "corners": "round", "support_email": "help@northwind.test"}},
		"source": "agency", "agency_id": "ag_1", "portal_host": "portal.northwind.test", "full_white_label": true
	},
	"portal_host": "portal.northwind.test",
	"portal_source": "agency",
	"portal_base_url": "https://portal.northwind.test",
	"powered_by": {"name": "Warmbly", "url": "https://warmbly.test"}
}`

func TestOrganizationBrandRouting(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name                         string
		call                         func(c *Client) error
		method, path, query, reqBody string
	}{
		{"Brand", func(c *Client) error { _, _, e := c.Organization.Brand(ctx); return e },
			"GET", "/v1/organization/brand", "", ""},
		{"UpdateBrand", func(c *Client) error {
			_, _, e := c.Organization.UpdateBrand(ctx, &WorkspaceBrandParams{DisplayName: "Acme", AccentColor: "#0284c7"})
			return e
		}, "PUT", "/v1/organization/brand", "",
			`{"display_name":"Acme","accent_color":"#0284c7","website_url":"","hide_powered_by":false}`},
		{"ResetBrand", func(c *Client) error { _, _, e := c.Organization.ResetBrand(ctx); return e },
			"DELETE", "/v1/organization/brand", "", ""},
		{"DeleteBrandLogo", func(c *Client) error { _, _, e := c.Organization.DeleteBrandLogo(ctx); return e },
			"DELETE", "/v1/organization/brand/logo", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got recordedRequest
			c := recordingFixtureClient(t, &got, workspaceBrandFixture)
			if err := tc.call(c); err != nil {
				t.Fatalf("call: %v", err)
			}
			checkRequest(t, got, tc.method, tc.path, tc.query, tc.reqBody)
		})
	}
}

func TestOrganizationBrandDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, workspaceBrandFixture)
	b, _, err := c.Organization.Brand(context.Background())
	if err != nil {
		t.Fatalf("Brand: %v", err)
	}
	if b.Brand.DisplayName != "" || b.Brand.UpdatedBy != nil {
		t.Errorf("brand = %+v", b.Brand)
	}
	if b.Inherited == nil || !b.Inherited.HidePoweredBy || b.Inherited.UpdatedBy == nil {
		t.Errorf("inherited = %+v", b.Inherited)
	}
	e := b.Effective
	if e.Source != BrandSourceAgency || e.AgencyID == nil || !e.FullWhiteLabel || e.Brand.PoweredBy != nil {
		t.Errorf("effective = %+v", e)
	}
	if e.Brand.Theme == nil || e.Brand.Theme.Font != "serif" || e.Brand.Theme.SupportEmail != "help@northwind.test" {
		t.Errorf("theme = %+v", e.Brand.Theme)
	}
	if b.PortalSource != "agency" || b.PoweredBy == nil || b.PoweredBy.Name != "Warmbly" {
		t.Errorf("view = %+v", b)
	}
}

func TestOrganizationUploadBrandLogo(t *testing.T) {
	var gotField, gotFilename, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.Method + " " + r.URL.Path
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
			return
		}
		for name, files := range r.MultipartForm.File {
			gotField = name
			if len(files) > 0 {
				gotFilename = files[0].Filename
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, workspaceBrandFixture)
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAccessToken("jwt"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	b, _, err := c.Organization.UploadBrandLogo(context.Background(), &FileUpload{
		Filename: "logo.png", Content: strings.NewReader("png-bytes"), ContentType: "image/png",
	})
	if err != nil {
		t.Fatalf("UploadBrandLogo: %v", err)
	}
	if gotPath != "POST /v1/organization/brand/logo" || gotField != "file" || gotFilename != "logo.png" {
		t.Errorf("path=%q field=%q filename=%q", gotPath, gotField, gotFilename)
	}
	if b.PortalHost != "portal.northwind.test" {
		t.Errorf("brand = %+v", b)
	}
}

func TestOrganizationLeaveAgency(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	if _, err := c.Organization.LeaveAgency(context.Background()); err != nil {
		t.Fatalf("LeaveAgency: %v", err)
	}
	checkRequest(t, got, "POST", "/v1/organization/agency/leave", "", "")
}

func TestOrganizationCurrentAgency(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"id": "org_1", "name": "Client B", "agency": {"id": "ag_1", "name": "Northwind", "can_leave": true}}`)
	org, _, err := c.Organization.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if org.Agency == nil || org.Agency.ID != "ag_1" || !org.Agency.CanLeave {
		t.Errorf("agency = %+v", org.Agency)
	}

	c = recordingFixtureClient(t, &got, `{"data": [{"id": "m_1", "organization_id": "org_1", "user_id": "u_1", "role": "owner",
		"permissions": 0, "invited_at": "2026-01-01T00:00:00Z", "email": "", "name": "", "agency": {"id": "ag_1", "name": "Northwind", "can_leave": false}}]}`)
	ms, _, err := c.Organization.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ms) != 1 || ms[0].Agency == nil || ms[0].Agency.CanLeave {
		t.Errorf("members = %+v", ms)
	}
}
