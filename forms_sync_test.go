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
	"time"
)

// TestFormServiceRouting asserts the method, path, and where it matters the
// query string and body, of every FormService call.
func TestFormServiceRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()
	before := time.Date(2026, 9, 1, 12, 30, 45, 123456789, time.UTC)

	cases := []struct {
		name       string
		call       func() error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string
	}{
		{"List", func() error { _, _, e := c.Forms.List(ctx); return e }, "GET", "/v1/forms", "", ""},
		{"Config", func() error { _, _, e := c.Forms.Config(ctx); return e }, "GET", "/v1/forms/config", "", ""},
		{"Create", func() error {
			_, _, e := c.Forms.Create(ctx, &FormCreateParams{Name: "Newsletter"})
			return e
		}, "POST", "/v1/forms", "", `{"name":"Newsletter"}`},
		{"Get", func() error { _, _, e := c.Forms.Get(ctx, "f_1"); return e }, "GET", "/v1/forms/f_1", "", ""},
		{"Update", func() error {
			_, _, e := c.Forms.Update(ctx, "f_1", &FormUpdateParams{Status: String(FormStatusPublished)})
			return e
		}, "PATCH", "/v1/forms/f_1", "", `{"status":"published"}`},
		{"Delete", func() error { _, e := c.Forms.Delete(ctx, "f_1"); return e }, "DELETE", "/v1/forms/f_1", "", ""},

		{"ListSubmissions", func() error {
			_, e := c.Forms.ListSubmissions(ctx, "f_1", nil)
			return e
		}, "GET", "/v1/forms/f_1/submissions", "", ""},
		{"ListSubmissions.params", func() error {
			_, e := c.Forms.ListSubmissions(ctx, "f_1", &FormSubmissionListParams{Limit: 25, Before: &before})
			return e
		}, "GET", "/v1/forms/f_1/submissions", "before=2026-09-01T12%3A30%3A45.123456789Z&limit=25", ""},
		{"DeleteSubmission", func() error {
			_, e := c.Forms.DeleteSubmission(ctx, "f_1", "sub_1")
			return e
		}, "DELETE", "/v1/forms/f_1/submissions/sub_1", "", ""},

		{"Stats", func() error { _, _, e := c.Forms.Stats(ctx, "f_1", ""); return e }, "GET", "/v1/forms/f_1/stats", "", ""},
		{"Stats.range", func() error {
			_, _, e := c.Forms.Stats(ctx, "f_1", FormStatsRange90Days)
			return e
		}, "GET", "/v1/forms/f_1/stats", "range=90d", ""},

		{"MintLink", func() error { _, _, e := c.Forms.MintLink(ctx, "f_1", "ct_1"); return e }, "GET", "/v1/forms/f_1/links/ct_1", "", ""},

		{"DeleteAsset", func() error {
			_, _, e := c.Forms.DeleteAsset(ctx, "f_1", FormAssetCover)
			return e
		}, "DELETE", "/v1/forms/f_1/assets/cover", "", ""},

		{"Domain", func() error { _, _, e := c.Forms.Domain(ctx); return e }, "GET", "/v1/forms/domain", "", ""},
		{"SetDomain", func() error {
			_, _, e := c.Forms.SetDomain(ctx, "forms.acme.com")
			return e
		}, "PUT", "/v1/forms/domain", "", `{"forms_domain":"forms.acme.com"}`},
		{"SetDomain.clear", func() error {
			_, _, e := c.Forms.SetDomain(ctx, "")
			return e
		}, "PUT", "/v1/forms/domain", "", `{"forms_domain":""}`},
		{"VerifyDomain", func() error { _, _, e := c.Forms.VerifyDomain(ctx); return e }, "POST", "/v1/forms/domain/verify", "", ""},
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
			if tc.wantBody != "" && strings.TrimSpace(got.body) != tc.wantBody {
				t.Errorf("body = %s, want %s", strings.TrimSpace(got.body), tc.wantBody)
			}
		})
	}
}

// TestFormUploadAssetIsMultipart checks the asset upload goes out as
// multipart/form-data with the file under the "file" field the server reads.
func TestFormUploadAssetIsMultipart(t *testing.T) {
	var (
		gotMethod, gotPath, gotFilename, gotContent, gotContentType string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f, fh, err := r.FormFile("file")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer f.Close()
		b, _ := io.ReadAll(f)
		gotFilename, gotContent = fh.Filename, string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"f_1","name":"Newsletter","logo_url":"https://cdn.example.com/logo.png","fields":[],"category_ids":[],"allowed_domains":[]}`))
	}))
	t.Cleanup(srv.Close)

	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	form, _, err := c.Forms.UploadAsset(context.Background(), "f_1", FormAssetLogo, &FileUpload{
		Filename:    "logo.png",
		Content:     strings.NewReader("png-bytes"),
		ContentType: "image/png",
	})
	if err != nil {
		t.Fatalf("UploadAsset: %v", err)
	}
	if gotMethod != "POST" || gotPath != "/v1/forms/f_1/assets/logo" {
		t.Errorf("got %s %s, want POST /v1/forms/f_1/assets/logo", gotMethod, gotPath)
	}
	if !strings.HasPrefix(gotContentType, "multipart/form-data") {
		t.Errorf("Content-Type = %q, want multipart/form-data", gotContentType)
	}
	if gotFilename != "logo.png" || gotContent != "png-bytes" {
		t.Errorf("file part = %q/%q, want logo.png/png-bytes", gotFilename, gotContent)
	}
	if form.LogoURL != "https://cdn.example.com/logo.png" {
		t.Errorf("LogoURL = %q", form.LogoURL)
	}

	if _, _, err := c.Forms.UploadAsset(context.Background(), "f_1", FormAssetLogo, nil); err == nil {
		t.Error("expected an error for a nil upload")
	}
}

// TestFormUpdateParamsCampaignEncoding covers the three campaign_id states:
// untouched (key absent), set, and detached (explicit null).
func TestFormUpdateParamsCampaignEncoding(t *testing.T) {
	cases := []struct {
		name   string
		params FormUpdateParams
		want   string
	}{
		{"absent", FormUpdateParams{Name: String("x")}, `{"name":"x"}`},
		{"set", FormUpdateParams{CampaignID: String("camp_1")}, `{"campaign_id":"camp_1"}`},
		{"clear", FormUpdateParams{ClearCampaign: true}, `{"campaign_id":null}`},
		{"clear wins", FormUpdateParams{CampaignID: String("camp_1"), ClearCampaign: true, Name: String("x")}, `{"name":"x","campaign_id":null}`},
		{"empty list replaces", FormUpdateParams{CategoryIDs: &[]string{}}, `{"category_ids":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(&tc.params)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(b) != tc.want {
				t.Errorf("got %s, want %s", b, tc.want)
			}
		})
	}
}

// TestFormListSubmissionsPaginatesByBefore walks two pages and checks the
// second request carries the last row's created_at, at full precision, as
// the before parameter.
func TestFormListSubmissionsPaginatesByBefore(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("before") == "" {
			_, _ = w.Write([]byte(`{"data":[
				{"id":"s_3","form_id":"f_1","organization_id":"org_1","data":{},"source_url":"","created_at":"2026-09-01T10:00:02Z"},
				{"id":"s_2","form_id":"f_1","organization_id":"org_1","data":{},"source_url":"","created_at":"2026-09-01T10:00:01.5Z"}
			],"has_more":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[
			{"id":"s_1","form_id":"f_1","organization_id":"org_1","data":{},"source_url":"","created_at":"2026-09-01T10:00:00Z"}
		],"has_more":false}`))
	}))
	t.Cleanup(srv.Close)

	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	page, err := c.Forms.ListSubmissions(ctx, "f_1", &FormSubmissionListParams{Limit: 2})
	if err != nil {
		t.Fatalf("ListSubmissions: %v", err)
	}
	if !page.HasMore() || page.NextCursor() != "2026-09-01T10:00:01.5Z" {
		t.Fatalf("first page: HasMore=%v NextCursor=%q", page.HasMore(), page.NextCursor())
	}
	if page.Response() == nil {
		t.Error("page.Response() is nil")
	}

	ids := make([]string, 0, 3)
	for sub, err := range page.All(ctx) {
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		ids = append(ids, sub.ID)
	}
	if strings.Join(ids, ",") != "s_3,s_2,s_1" {
		t.Errorf("ids = %v", ids)
	}
	if len(queries) != 2 || queries[0] != "limit=2" || queries[1] != "before=2026-09-01T10%3A00%3A01.5Z&limit=2" {
		t.Errorf("queries = %q", queries)
	}

	last, err := page.Next(ctx)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if last.HasMore() {
		t.Error("last page reports more")
	}
	if _, err := last.Next(ctx); !errors.Is(err, ErrNoMorePages) {
		t.Errorf("Next on last page = %v, want ErrNoMorePages", err)
	}
}

func TestFormDecode(t *testing.T) {
	const fixture = `{
		"id": "3f9c4a6e-6d2b-4d1e-9a0f-1b2c3d4e5f60",
		"organization_id": "org_1",
		"created_by": "user_1",
		"public_id": "k7x2m9q4",
		"name": "Newsletter signup",
		"status": "published",
		"fields": [
			{"id": "first_name", "type": "text", "label": "First name", "map_to": "first_name", "width": "half", "required": false},
			{"id": "email", "type": "email", "label": "Email", "map_to": "email", "required": true},
			{"id": "size", "type": "select", "label": "Company size", "options": ["1-10", "11-50"], "required": false},
			{"id": "p2", "type": "page_break", "label": "About you", "required": false},
			{"id": "src", "type": "hidden", "label": "", "value": "pricing-page", "required": false},
			{"id": "bio", "type": "textarea", "label": "Bio", "rows": 4, "required": false}
		],
		"design": {
			"font_family": "inter", "layout": "split", "mode": "focus",
			"page_background": "#ffffff", "page_background_end": "#f3f4f6",
			"accent_color": "#3b82f6", "button_text": "Join",
			"border_radius": 12, "max_width": 640, "shadow": true, "show_progress": true,
			"header_enabled": true, "header_title": "Acme", "header_placement": "page", "header_align": "between",
			"cover_title": "Stay in the loop", "logo_size": "md", "logo_position": "page",
			"background_size": "cover", "background_overlay": 40, "theme": "ocean"
		},
		"success_message": "Thanks!",
		"redirect_url": "https://acme.com/thanks",
		"campaign_id": "camp_1",
		"category_ids": ["cat_1", "cat_2"],
		"allowed_domains": ["acme.com"],
		"captcha_enabled": true,
		"logo_url": "https://cdn.example.com/logo.png",
		"cover_url": "",
		"background_url": "",
		"views_count": 1200,
		"submissions_count": 84,
		"last_submission_at": "2026-09-05T08:00:00Z",
		"published_at": "2026-08-01T09:00:00Z",
		"starts_count": 300,
		"identified_count": 12,
		"trend": [0,1,2,3,4,5,6,7,8,9,10,11,12,13],
		"share_url": "https://forms.acme.com/f/k7x2m9q4",
		"created_at": "2026-07-30T09:00:00Z",
		"updated_at": "2026-09-05T08:00:00Z"
	}`
	var f Form
	if err := json.Unmarshal([]byte(fixture), &f); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if f.Status != FormStatusPublished || f.PublicID != "k7x2m9q4" || f.ShareURL != "https://forms.acme.com/f/k7x2m9q4" {
		t.Errorf("unexpected form: %+v", f)
	}
	if f.CreatedBy == nil || *f.CreatedBy != "user_1" || f.CampaignID == nil || *f.CampaignID != "camp_1" {
		t.Errorf("created_by/campaign_id not decoded: %+v", f)
	}
	if len(f.Fields) != 6 {
		t.Fatalf("fields = %d, want 6", len(f.Fields))
	}
	if f.Fields[1].Type != FormFieldTypeEmail || f.Fields[1].MapTo != FormMapToEmail || !f.Fields[1].Required {
		t.Errorf("email field = %+v", f.Fields[1])
	}
	if f.Fields[2].Type != FormFieldTypeSelect || len(f.Fields[2].Options) != 2 {
		t.Errorf("select field = %+v", f.Fields[2])
	}
	if f.Fields[3].Type != FormFieldTypePageBreak || f.Fields[4].Type != FormFieldTypeHidden || f.Fields[4].Value != "pricing-page" {
		t.Errorf("page break / hidden fields = %+v %+v", f.Fields[3], f.Fields[4])
	}
	if f.Fields[0].Width != FormFieldWidthHalf || f.Fields[5].Rows != 4 {
		t.Errorf("width/rows = %+v %+v", f.Fields[0], f.Fields[5])
	}
	d := f.Design
	if d.Layout != FormLayoutSplit || d.Mode != FormModeFocus || d.FontFamily != FormFontInter || d.HeaderAlign != FormAlignBetween {
		t.Errorf("design enums = %+v", d)
	}
	if d.BorderRadius == nil || *d.BorderRadius != 12 || d.MaxWidth == nil || *d.MaxWidth != 640 || d.Shadow == nil || !*d.Shadow {
		t.Errorf("design numbers = %+v", d)
	}
	if d.BackgroundOverlay == nil || *d.BackgroundOverlay != 40 || d.BackgroundSize != FormBackgroundSizeCover || d.LogoPosition != FormLogoPositionPage {
		t.Errorf("design background/logo = %+v", d)
	}
	if len(f.CategoryIDs) != 2 || len(f.AllowedDomains) != 1 || !f.CaptchaEnabled {
		t.Errorf("settings = %+v", f)
	}
	if f.ViewsCount != 1200 || f.SubmissionsCount != 84 || f.StartsCount != 300 || f.IdentifiedCount != 12 || len(f.Trend) != 14 {
		t.Errorf("counters = %+v", f)
	}
	if f.LastSubmissionAt == nil || f.PublishedAt == nil || f.PublishedAt.Year() != 2026 {
		t.Errorf("timestamps = %+v", f)
	}

	// A sparse form (fresh draft) must decode with nil optionals.
	var draft Form
	if err := json.Unmarshal([]byte(`{"id":"f_2","status":"draft","fields":[],"design":{},"category_ids":[],"allowed_domains":[]}`), &draft); err != nil {
		t.Fatalf("decode draft: %v", err)
	}
	if draft.CampaignID != nil || draft.PublishedAt != nil || draft.Trend != nil || draft.Design.BorderRadius != nil {
		t.Errorf("draft optionals not nil: %+v", draft)
	}
}

func TestFormSubmissionDecode(t *testing.T) {
	const fixture = `{
		"id": "sub_1",
		"form_id": "f_1",
		"organization_id": "org_1",
		"contact_id": "ct_1",
		"campaign_id": "camp_1",
		"data": {
			"first_name": "Ada",
			"email": "ada@example.com",
			"interests": ["api", "sdk"],
			"consent": "true"
		},
		"source_url": "https://acme.com/pricing",
		"created_at": "2026-09-05T08:00:00.25Z",
		"contact_email": "ada@example.com",
		"contact_name": "Ada Lovelace",
		"campaign_name": "Q3 launch"
	}`
	var s FormSubmission
	if err := json.Unmarshal([]byte(fixture), &s); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if s.ContactID == nil || *s.ContactID != "ct_1" || s.CampaignID == nil || *s.CampaignID != "camp_1" {
		t.Errorf("links = %+v", s)
	}
	if s.Data["email"] != "ada@example.com" {
		t.Errorf("data.email = %v", s.Data["email"])
	}
	interests, ok := s.Data["interests"].([]any)
	if !ok || len(interests) != 2 || interests[0] != "api" {
		t.Errorf("data.interests = %#v", s.Data["interests"])
	}
	if s.SourceURL != "https://acme.com/pricing" || s.ContactName != "Ada Lovelace" || s.CampaignName != "Q3 launch" {
		t.Errorf("summaries = %+v", s)
	}
	if s.CreatedAt.Nanosecond() != 250_000_000 {
		t.Errorf("created_at lost precision: %v", s.CreatedAt)
	}

	var anon FormSubmission
	if err := json.Unmarshal([]byte(`{"id":"sub_2","form_id":"f_1","organization_id":"org_1","data":{},"source_url":"","created_at":"2026-09-05T08:00:00Z"}`), &anon); err != nil {
		t.Fatalf("decode anonymous: %v", err)
	}
	if anon.ContactID != nil || anon.CampaignID != nil || anon.ContactEmail != "" {
		t.Errorf("anonymous submission carries links: %+v", anon)
	}
}

func TestFormStatsDecode(t *testing.T) {
	const fixture = `{
		"totals": {"views": 1000, "starts": 400, "submissions": 120, "completion_rate": 0.12, "identified_visitors": 30},
		"daily": [
			{"date": "2026-09-04", "views": 500, "starts": 200, "submissions": 60},
			{"date": "2026-09-05", "views": 500, "starts": 200, "submissions": 60}
		],
		"pages": [
			{"page_index": 0, "title": "Page 1", "reached": 400, "completed_from": 120},
			{"page_index": 1, "title": "About you", "reached": 250, "completed_from": 120}
		],
		"sources": [{"key": "acme.com", "count": 700}, {"key": "", "count": 300}],
		"countries": [{"key": "US", "count": 600}],
		"devices": [{"key": "desktop", "count": 800}, {"key": "mobile", "count": 200}],
		"campaigns": [{"key": "Q3 launch", "count": 30}],
		"identified": [
			{"contact_id": "ct_1", "name": "Ada Lovelace", "email": "ada@example.com", "last_seen": "2026-09-05T08:00:00Z", "furthest_page": 1, "completed": true, "campaign": "Q3 launch"},
			{"contact_id": "ct_2", "name": "", "email": "bob@example.com", "last_seen": "2026-09-04T08:00:00Z", "furthest_page": 0, "completed": false}
		]
	}`
	var st FormStats
	if err := json.Unmarshal([]byte(fixture), &st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if st.Totals.Views != 1000 || st.Totals.Submissions != 120 || st.Totals.CompletionRate != 0.12 || st.Totals.IdentifiedVisitors != 30 {
		t.Errorf("totals = %+v", st.Totals)
	}
	if len(st.Daily) != 2 || st.Daily[0].Date != "2026-09-04" || st.Daily[1].Starts != 200 {
		t.Errorf("daily = %+v", st.Daily)
	}
	if len(st.Pages) != 2 || st.Pages[1].PageIndex != 1 || st.Pages[1].Title != "About you" || st.Pages[1].Reached != 250 || st.Pages[1].CompletedFrom != 120 {
		t.Errorf("pages = %+v", st.Pages)
	}
	if len(st.Sources) != 2 || st.Sources[0].Key != "acme.com" || st.Devices[0].Key != "desktop" || st.Campaigns[0].Count != 30 {
		t.Errorf("buckets = %+v", st)
	}
	if len(st.Identified) != 2 || !st.Identified[0].Completed || st.Identified[0].Campaign != "Q3 launch" || st.Identified[1].FurthestPage != 0 {
		t.Errorf("identified = %+v", st.Identified)
	}
	if st.Identified[0].LastSeen.IsZero() {
		t.Error("last_seen not decoded")
	}
}

func TestFormsDomainStatusDecode(t *testing.T) {
	var st FormsDomainStatus
	if err := json.Unmarshal([]byte(`{
		"forms_domain": "forms.acme.com",
		"forms_domain_verified": false,
		"forms_domain_verified_at": null,
		"cname_target": "forms.warmbly.com",
		"status": "wrong_target",
		"message": "forms.acme.com points at cdn.acme.com, not forms.warmbly.com",
		"observed": "cdn.acme.com",
		"forms_host_unresolvable": false
	}`), &st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if st.Status != FormsDomainStatusWrongTarget || st.Observed != "cdn.acme.com" || st.CNAMETarget != "forms.warmbly.com" {
		t.Errorf("status = %+v", st)
	}
	if st.FormsDomainVerified || st.FormsDomainVerifiedAt != nil {
		t.Errorf("unverified domain reports verification: %+v", st)
	}

	var link FormLink
	if err := json.Unmarshal([]byte(`{"url":"https://forms.acme.com/f/k7x2m9q4?t=8c1b"}`), &link); err != nil {
		t.Fatalf("decode link: %v", err)
	}
	if !strings.HasSuffix(link.URL, "?t=8c1b") {
		t.Errorf("link = %+v", link)
	}

	var cfg FormsConfig
	if err := json.Unmarshal([]byte(`{"base_url":"https://forms.warmbly.com","captcha_available":true}`), &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if cfg.BaseURL != "https://forms.warmbly.com" || !cfg.CaptchaAvailable {
		t.Errorf("config = %+v", cfg)
	}
}
