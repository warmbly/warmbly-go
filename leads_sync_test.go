package warmbly

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestLeadsRouting(t *testing.T) {
	var got recordedRequest
	ctx := context.Background()

	cases := []struct {
		name       string
		status     int
		resp       string
		call       func(c *Client) error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string
	}{
		{"Config", 200, `{"providers":[],"categories":[],"map":{}}`, func(c *Client) error { _, _, e := c.Leads.Config(ctx); return e }, "GET", "/v1/leads/config", "", ""},
		{"Geocode", 200, `{"data":[]}`, func(c *Client) error {
			_, _, e := c.Leads.Geocode(ctx, &LeadGeocodeParams{Query: "Berlin", Near: &LatLng{Lat: 52.5, Lng: 13.4}, Language: "de"})
			return e
		}, "GET", "/v1/leads/geocode", "lang=de&lat=52.5&lng=13.4&q=Berlin", ""},
		{"Search", 200, `{"search":null,"leads":[]}`, func(c *Client) error {
			_, _, e := c.Leads.Search(ctx, &LeadSearchParams{Provider: LeadProviderOSM, Category: "cafe", Area: LeadArea{Kind: LeadAreaBBox, Bounds: LeadBounds{South: 1, West: 2, North: 3, East: 4}}})
			return e
		}, "POST", "/v1/leads/search", "", `{"provider":"osm","category":"cafe","location_label":"","area":{"kind":"bbox","bounds":{"south":1,"west":2,"north":3,"east":4}}}`},
		{"Query", 200, `{"data":[],"pagination":{"next_cursor":null,"has_more":false,"total":0}}`, func(c *Client) error {
			_, e := c.Leads.Query(ctx, &LeadQueryParams{Filter: &LeadFilter{HasEmail: Bool(true)}, Sort: LeadSortScore, Limit: 10})
			return e
		}, "POST", "/v1/leads/query", "", `{"filter":{"has_email":true},"sort":"score","limit":10}`},
		{"Points", 200, `{"data":[],"truncated":false}`, func(c *Client) error { _, _, e := c.Leads.Points(ctx, &LeadFilter{Tag: "x"}); return e }, "POST", "/v1/leads/points", "", `{"filter":{"tag":"x"}}`},
		{"Facets", 200, `{"categories":[],"tags":[],"cities":[]}`, func(c *Client) error { _, _, e := c.Leads.Facets(ctx, nil); return e }, "POST", "/v1/leads/facets", "", `{}`},
		{"Get", 200, `{"id":"l1"}`, func(c *Client) error { _, _, e := c.Leads.Get(ctx, "l1"); return e }, "GET", "/v1/leads/l1", "", ""},
		{"Update", 200, `{"id":"l1"}`, func(c *Client) error {
			_, _, e := c.Leads.Update(ctx, "l1", &LeadPatch{Status: String(LeadFinderStatusSaved), AddTags: []string{"a"}})
			return e
		}, "PATCH", "/v1/leads/l1", "", `{"status":"saved","add_tags":["a"]}`},
		{"BulkUpdate", 200, `{"updated":2}`, func(c *Client) error {
			_, _, e := c.Leads.BulkUpdate(ctx, &LeadBulkUpdateParams{LeadSelection: LeadSelection{LeadIDs: []string{"l1", "l2"}}, LeadPatch: LeadPatch{Notes: String("n")}})
			return e
		}, "PATCH", "/v1/leads", "", `{"lead_ids":["l1","l2"],"notes":"n"}`},
		{"BulkDelete", 200, `{"deleted":1}`, func(c *Client) error {
			_, _, e := c.Leads.BulkDelete(ctx, &LeadSelection{All: true, Filter: &LeadFilter{Q: "x"}})
			return e
		}, "DELETE", "/v1/leads", "", `{"all":true,"filter":{"q":"x"}}`},
		{"Enrich", 202, `{"id":"r1"}`, func(c *Client) error {
			_, _, e := c.Leads.Enrich(ctx, &LeadEnrichParams{LeadSelection: LeadSelection{LeadIDs: []string{"l1"}}, People: Bool(false)})
			return e
		}, "POST", "/v1/leads/enrich", "", `{"lead_ids":["l1"],"people":false}`},
		{"EnrichmentRuns", 200, `{"data":[]}`, func(c *Client) error { _, _, e := c.Leads.EnrichmentRuns(ctx); return e }, "GET", "/v1/leads/enrichments", "", ""},
		{"EnrichmentRun", 200, `{"id":"r1"}`, func(c *Client) error { _, _, e := c.Leads.EnrichmentRun(ctx, "r1"); return e }, "GET", "/v1/leads/enrichments/r1", "", ""},
		{"Export", 200, `id,name`, func(c *Client) error {
			var b bytes.Buffer
			_, e := c.Leads.Export(ctx, &LeadExportParams{LeadSelection: LeadSelection{All: true}, Format: LeadExportCSV, Columns: []string{"name"}}, &b)
			return e
		}, "POST", "/v1/leads/export", "", `{"all":true,"format":"csv","columns":["name"]}`},
		{"AddToContacts", 200, `{"selected":1}`, func(c *Client) error {
			_, _, e := c.Leads.AddToContacts(ctx, &LeadAddToContactsParams{LeadSelection: LeadSelection{LeadIDs: []string{"l1"}}, Who: LeadWhoBoth, SegmentIDs: []string{"s1"}})
			return e
		}, "POST", "/v1/leads/contacts", "", `{"lead_ids":["l1"],"segment_ids":["s1"],"who":"both"}`},
		{"Searches", 200, `{"data":[]}`, func(c *Client) error { _, _, e := c.Leads.Searches(ctx, true); return e }, "GET", "/v1/leads/searches", "saved=true", ""},
		{"UpdateSearch", 200, `{"id":"s1"}`, func(c *Client) error {
			_, _, e := c.Leads.UpdateSearch(ctx, "s1", &LeadSearchUpdateParams{Saved: Bool(true)})
			return e
		}, "PATCH", "/v1/leads/searches/s1", "", `{"saved":true}`},
		{"DeleteSearch", 204, ``, func(c *Client) error { _, e := c.Leads.DeleteSearch(ctx, "s1"); return e }, "DELETE", "/v1/leads/searches/s1", "", ""},
		{"AddPerson", 200, `{"id":"p1"}`, func(c *Client) error {
			_, _, e := c.Leads.AddPerson(ctx, "l1", &LeadPersonInput{Name: "Ada Lovelace", Title: "CEO"})
			return e
		}, "POST", "/v1/leads/l1/people", "", `{"name":"Ada Lovelace","title":"CEO"}`},
		{"UpdatePerson", 200, `{"id":"p1"}`, func(c *Client) error {
			_, _, e := c.Leads.UpdatePerson(ctx, "l1", "p1", &LeadPersonUpdateParams{Email: String("")})
			return e
		}, "PATCH", "/v1/leads/l1/people/p1", "", `{"email":""}`},
		{"DeletePerson", 204, ``, func(c *Client) error { _, e := c.Leads.DeletePerson(ctx, "l1", "p1"); return e }, "DELETE", "/v1/leads/l1/people/p1", "", ""},
		{"FindPersonEmail", 200, `{"id":"p1"}`, func(c *Client) error { _, _, e := c.Leads.FindPersonEmail(ctx, "l1", "p1"); return e }, "POST", "/v1/leads/l1/people/p1/find-email", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got = recordedRequest{}
			c := emailFixtureClient(t, tc.status, tc.resp, &got)
			if err := tc.call(c); err != nil {
				t.Fatalf("call: %v", err)
			}
			if got.method != tc.wantMethod || got.path != tc.wantPath {
				t.Errorf("got %s %s, want %s %s", got.method, got.path, tc.wantMethod, tc.wantPath)
			}
			if got.rawQuery != tc.wantQuery {
				t.Errorf("query = %q, want %q", got.rawQuery, tc.wantQuery)
			}
			if strings.TrimSpace(got.body) != tc.wantBody {
				t.Errorf("body = %q, want %q", got.body, tc.wantBody)
			}
		})
	}
}

func TestLeadsDecode(t *testing.T) {
	rating := 4.5
	reviews := 120
	contact := "c1"
	now := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	lead := Lead{
		ID: "11111111-1111-1111-1111-111111111111", Provider: LeadProviderOSM, Name: "Cafe Nord", Category: "cafe",
		Categories: []string{"cafe"}, Latitude: 52.5, Longitude: 13.4, Website: "https://nord.example", Domain: "nord.example",
		Email: "hi@nord.example", Emails: []LeadEmail{{Email: "hi@nord.example", Source: "website", DomainMatch: true, Status: "valid"}},
		Socials: map[string]string{"instagram": "https://instagram.com/nord"}, Rating: &rating, ReviewCount: &reviews,
		Site: LeadSite{Reachable: true, HTTPS: true, EmailPattern: "first.last", CheckedAt: &now}, Score: 80,
		Status: LeadFinderStatusNew, EnrichmentStatus: LeadEnrichmentDone, EnrichedAt: &now, ContactID: &contact,
		Tags: []string{"berlin"}, PeopleCount: 1,
		People:      []LeadPerson{{ID: "p1", LeadID: "l1", Name: "Ada Lovelace", Seniority: LeadSeniorityOwner, Email: "ada@nord.example", EmailSource: LeadEmailPattern, EmailStatus: LeadPersonStatusGuessed, EmailConfidence: 60, Source: LeadPersonPage}},
		FirstSeenAt: now, LastSeenAt: now, CreatedAt: now, UpdatedAt: now,
	}
	body, err := json.Marshal(map[string]any{
		"data":       []Lead{lead},
		"pagination": map[string]any{"next_cursor": "abc", "has_more": true, "total": 41},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got recordedRequest
	c := emailFixtureClient(t, http.StatusOK, string(body), &got)
	page, err := c.Leads.Query(context.Background(), &LeadQueryParams{Cursor: "prev"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if !strings.Contains(got.body, `"cursor":"prev"`) {
		t.Errorf("body = %q, want cursor", got.body)
	}
	if len(page.Data) != 1 || page.Data[0].Name != "Cafe Nord" || *page.Data[0].Rating != 4.5 || page.Data[0].People[0].Seniority != LeadSeniorityOwner {
		t.Fatalf("decoded = %+v", page.Data)
	}
	if page.Pagination.Total == nil || *page.Pagination.Total != 41 || !page.HasMore() || page.NextCursor() != "abc" {
		t.Errorf("pagination = %+v", page.Pagination)
	}

	// The next page re-posts the same query with the returned cursor.
	if _, err := page.Next(context.Background()); err != nil {
		t.Fatalf("Next: %v", err)
	}
	if !strings.Contains(got.body, `"cursor":"abc"`) {
		t.Errorf("next body = %q, want cursor abc", got.body)
	}
}

func TestLeadsDecodeResponses(t *testing.T) {
	ctx := context.Background()

	// A lead with nulls where the server has pointers.
	c := emailFixtureClient(t, 200, `{"id":"l1","rating":null,"review_count":null,"price_level":null,"enriched_at":null,"contact_id":null,"tags":null,"unknown_new_field":1,"status":"brand_new"}`, nil)
	l, _, err := c.Leads.Get(ctx, "l1")
	if err != nil || l.Rating != nil || l.ContactID != nil || l.Status != "brand_new" {
		t.Fatalf("Get = %+v, %v", l, err)
	}

	c = emailFixtureClient(t, 200, `{"search":{"id":"s1","area":{"kind":"circle","bounds":{"south":1,"west":2,"north":3,"east":4},"center":{"lat":1,"lng":2},"radius_m":500},"has_more":true,"created_by_user_id":null},"leads":[],"found":20,"outside_area":3,"charged_usd":0.04}`, nil)
	sr, _, err := c.Leads.Search(ctx, &LeadSearchParams{})
	if err != nil || sr.Found != 20 || sr.OutsideArea != 3 || !sr.Search.HasMore || sr.Search.Area.Center.Lng != 2 || sr.ChargedUSD != 0.04 {
		t.Fatalf("Search = %+v, %v", sr, err)
	}

	c = emailFixtureClient(t, 200, `{"data":[{"id":"a","name":"A","category":"cafe","latitude":1,"longitude":2,"status":"new","score":3,"has_email":true,"has_website":false,"has_phone":true,"rating":null,"review_count":7,"in_contacts":false}],"truncated":true}`, nil)
	pts, _, err := c.Leads.Points(ctx, nil)
	if err != nil || !pts.Truncated || len(pts.Data) != 1 || !pts.Data[0].HasEmail || *pts.Data[0].Reviews != 7 {
		t.Fatalf("Points = %+v, %v", pts, err)
	}

	c = emailFixtureClient(t, 200, `{"categories":[{"value":"cafe","count":3}],"tags":[],"cities":[{"value":"Berlin","count":2}]}`, nil)
	fc, _, err := c.Leads.Facets(ctx, nil)
	if err != nil || fc.Categories[0].Count != 3 || fc.Cities[0].Value != "Berlin" {
		t.Fatalf("Facets = %+v, %v", fc, err)
	}

	c = emailFixtureClient(t, 200, `{"providers":[{"id":"osm","name":"OpenStreetMap","description":"d","available":true,"key_source":"none","needs_key":false,"page_size":100,"max_pages":5,"cost_usd":0,"fields":["name"]}],"categories":[{"id":"cafe","label":"Cafes","group":"Food"}],"default_provider":"osm","enrich_cost_usd":0.01,"person_email_cost_usd":0.02,"export_columns":[{"key":"name","label":"Name"}],"person_columns":[],"default_export_columns":["name"],"map":{"style_light":"l","style_dark":"d"}}`, nil)
	cfg, _, err := c.Leads.Config(ctx)
	if err != nil || cfg.Providers[0].MaxPages != 5 || cfg.Categories[0].ID != "cafe" || cfg.EnrichCostUSD != 0.01 || cfg.Map.StyleDark != "d" || cfg.DefaultExportColumns[0] != "name" {
		t.Fatalf("Config = %+v, %v", cfg, err)
	}

	c = emailFixtureClient(t, 200, `{"data":[{"label":"Berlin","detail":"Germany","kind":"city","center":{"lat":52.5,"lng":13.4},"bounds":{"south":1,"west":2,"north":3,"east":4}}]}`, nil)
	geo, _, err := c.Leads.Geocode(ctx, &LeadGeocodeParams{Query: "Berlin"})
	if err != nil || len(geo) != 1 || geo[0].Center.Lat != 52.5 || geo[0].Bounds.East != 4 {
		t.Fatalf("Geocode = %+v, %v", geo, err)
	}

	c = emailFixtureClient(t, 202, `{"id":"r1","verify":true,"people":false,"status":"running","total":10,"processed":3,"emails_found":2,"charged_usd":0.1,"started_at":"2026-11-01T10:00:00Z","finished_at":null,"requested_by_user_id":null}`, nil)
	run, _, err := c.Leads.Enrich(ctx, &LeadEnrichParams{})
	if err != nil || run.Status != LeadRunRunning || run.Done() || run.StartedAt == nil || run.FinishedAt != nil || run.Total != 10 {
		t.Fatalf("Enrich = %+v, %v", run, err)
	}

	c = emailFixtureClient(t, 200, `{"updated":3}`, nil)
	if n, _, err := c.Leads.BulkUpdate(ctx, &LeadBulkUpdateParams{}); err != nil || n != 3 {
		t.Fatalf("BulkUpdate = %d, %v", n, err)
	}
	c = emailFixtureClient(t, 200, `{"deleted":4}`, nil)
	if n, _, err := c.Leads.BulkDelete(ctx, &LeadSelection{LeadIDs: []string{"a"}}); err != nil || n != 4 {
		t.Fatalf("BulkDelete = %d, %v", n, err)
	}

	c = emailFixtureClient(t, 200, `{"selected":5,"imported":3,"updated":1,"skipped":1,"no_email":2,"errors":[{"line":2,"reason":"bad"}]}`, nil)
	res, _, err := c.Leads.AddToContacts(ctx, &LeadAddToContactsParams{})
	if err != nil || res.Imported != 3 || res.NoEmail != 2 || res.Errors[0].Reason != "bad" {
		t.Fatalf("AddToContacts = %+v, %v", res, err)
	}

	c = emailFixtureClient(t, 200, `{"data":[{"id":"s1","name":"Cafes","provider":"osm","saved":true,"result_count":20,"pages_fetched":1,"has_more":false,"area":{"kind":"bbox","bounds":{}}}]}`, nil)
	ss, _, err := c.Leads.Searches(ctx, false)
	if err != nil || len(ss) != 1 || !ss[0].Saved {
		t.Fatalf("Searches = %+v, %v", ss, err)
	}
}
