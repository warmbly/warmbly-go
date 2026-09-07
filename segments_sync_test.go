package warmbly

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// recordingFixtureClient is routingClient for responses the shared envelope cannot
// stand in for: it records the request and answers with exactly body.
func recordingFixtureClient(t *testing.T, got *recordedRequest, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.path = r.URL.Path
		got.rawQuery = r.URL.RawQuery
		b, _ := io.ReadAll(r.Body)
		got.body = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestSegmentRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()

	cases := []struct {
		name       string
		call       func() error
		wantMethod string
		wantPath   string
		wantBody   string
	}{
		{"List", func() error { _, _, e := c.Segments.List(ctx); return e }, "GET", "/v1/segments", ""},
		{"Fields", func() error { _, _, e := c.Segments.Fields(ctx); return e }, "GET", "/v1/segments/fields", ""},
		{"Preview", func() error {
			_, _, e := c.Segments.Preview(ctx, &SegmentPreviewParams{
				ID:    String("seg_1"),
				Match: SegmentMatchAny,
				Conditions: []SegmentCondition{
					{Field: "subscribed", Operator: SegmentOpIsTrue},
				},
			})
			return e
		}, "POST", "/v1/segments/preview", `{"id":"seg_1","match":"any","conditions":[{"field":"subscribed","operator":"is_true"}]}`},
		{"Create", func() error {
			_, _, e := c.Segments.Create(ctx, &SegmentCreateParams{
				Name:  "Warm leads",
				Color: String("#0284c7"),
				Conditions: []SegmentCondition{
					{Field: "source", Operator: SegmentOpIn, Values: []string{"import", "api"}},
				},
			})
			return e
		}, "POST", "/v1/segments", `{"name":"Warm leads","color":"#0284c7","conditions":[{"field":"source","operator":"in","values":["import","api"]}]}`},
		{"Get", func() error { _, _, e := c.Segments.Get(ctx, "seg_1"); return e }, "GET", "/v1/segments/seg_1", ""},
		{"Update", func() error {
			_, _, e := c.Segments.Update(ctx, "seg_1", &SegmentUpdateParams{Name: String("Renamed")})
			return e
		}, "PATCH", "/v1/segments/seg_1", `{"name":"Renamed","conditions":null}`},
		{"Update clears conditions", func() error {
			_, _, e := c.Segments.Update(ctx, "seg_1", &SegmentUpdateParams{Conditions: []SegmentCondition{}})
			return e
		}, "PATCH", "/v1/segments/seg_1", `{"conditions":[]}`},
		{"Delete", func() error { _, e := c.Segments.Delete(ctx, "seg_1"); return e }, "DELETE", "/v1/segments/seg_1", ""},
		{"SetMembers", func() error {
			_, _, e := c.Segments.SetMembers(ctx, "seg_1", []string{"ct_1", "ct_2"}, SegmentMemberExclude)
			return e
		}, "POST", "/v1/segments/seg_1/members", `{"contacts":["ct_1","ct_2"],"mode":"exclude"}`},
		{"Overrides", func() error { _, _, e := c.Segments.Overrides(ctx, "seg_1"); return e }, "GET", "/v1/segments/seg_1/overrides", ""},
		{"AddToCampaign", func() error {
			_, _, e := c.Segments.AddToCampaign(ctx, "seg_1", "camp_1")
			return e
		}, "POST", "/v1/segments/seg_1/add-to-campaign", `{"campaign_id":"camp_1"}`},
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
			if tc.wantBody != "" && strings.TrimSpace(got.body) != tc.wantBody {
				t.Errorf("body = %s, want %s", got.body, tc.wantBody)
			}
		})
	}
}

// The lookup answers {"data": {...}} with an object, which the shared
// envelope's data array cannot stand in for.
func TestSegmentMemberModes(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"data": {"ct_1": "include", "ct_2": "exclude"}}`)

	modes, _, err := c.Segments.MemberModes(context.Background(), "seg_1", []string{"ct_1", "ct_2", "ct_3"})
	if err != nil {
		t.Fatalf("MemberModes: %v", err)
	}
	if got.method != "POST" || got.path != "/v1/segments/seg_1/members/lookup" {
		t.Errorf("got %s %s", got.method, got.path)
	}
	if strings.TrimSpace(got.body) != `{"contacts":["ct_1","ct_2","ct_3"]}` {
		t.Errorf("body = %s", got.body)
	}
	if modes["ct_1"] != SegmentMemberInclude || modes["ct_2"] != SegmentMemberExclude {
		t.Errorf("modes = %v", modes)
	}
	if _, ok := modes["ct_3"]; ok {
		t.Errorf("a contact without an override must be absent, got %v", modes)
	}
}

func TestSegmentMemberModesEmpty(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"data": {}}`)
	modes, _, err := c.Segments.MemberModes(context.Background(), "seg_1", []string{"ct_1"})
	if err != nil {
		t.Fatalf("MemberModes: %v", err)
	}
	if modes == nil || len(modes) != 0 {
		t.Errorf("want an empty, non-nil map, got %#v", modes)
	}
}

func TestSegmentSetMembersDecodesUpdated(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"updated": 7}`)
	n, _, err := c.Segments.SetMembers(context.Background(), "seg_1", []string{"ct_1"}, SegmentMemberAuto)
	if err != nil {
		t.Fatalf("SetMembers: %v", err)
	}
	if n != 7 {
		t.Errorf("updated = %d, want 7", n)
	}
}

func TestSegmentDecode(t *testing.T) {
	const fixture = `{
	  "id": "0b6f9c3e-2f7a-4c0e-9d8e-1a2b3c4d5e6f",
	  "organization_id": "9b1e0000-0000-4000-8000-000000000001",
	  "created_by": "4e5f0000-0000-4000-8000-000000000002",
	  "name": "Warm fintech leads",
	  "description": "Opened in the last 30 days, not yet replied",
	  "color": "#0284c7",
	  "match": "all",
	  "conditions": [
	    { "field": "custom.industry", "operator": "equals", "value": "fintech" },
	    { "field": "last_opened_at", "operator": "within_days", "value": "30" },
	    { "field": "emails_replied", "operator": "equals", "value": "0" },
	    { "field": "source", "operator": "in", "values": ["import", "api"] },
	    { "field": "subscribed", "operator": "is_true" }
	  ],
	  "contact_count": 412,
	  "included_count": 3,
	  "excluded_count": 1,
	  "created_at": "2026-08-01T09:12:00Z",
	  "updated_at": "2026-08-20T14:03:00Z"
	}`
	var got recordedRequest
	c := recordingFixtureClient(t, &got, fixture)

	seg, _, err := c.Segments.Get(context.Background(), "0b6f9c3e-2f7a-4c0e-9d8e-1a2b3c4d5e6f")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if seg.Name != "Warm fintech leads" || seg.Match != SegmentMatchAll || seg.Color != "#0284c7" {
		t.Errorf("segment = %+v", seg)
	}
	if seg.CreatedBy == nil || *seg.CreatedBy != "4e5f0000-0000-4000-8000-000000000002" {
		t.Errorf("created_by = %v", seg.CreatedBy)
	}
	if seg.ContactCount != 412 || seg.IncludedCount != 3 || seg.ExcludedCount != 1 {
		t.Errorf("counts = %d/%d/%d", seg.ContactCount, seg.IncludedCount, seg.ExcludedCount)
	}
	if len(seg.Conditions) != 5 {
		t.Fatalf("conditions = %d, want 5", len(seg.Conditions))
	}
	if c0 := seg.Conditions[0]; c0.Field != SegmentCustomFieldPrefix+"industry" || c0.Operator != SegmentOpEquals || c0.Value != "fintech" {
		t.Errorf("condition 0 = %+v", c0)
	}
	if c1 := seg.Conditions[1]; c1.Operator != SegmentOpWithinDays || c1.Value != "30" {
		t.Errorf("condition 1 = %+v", c1)
	}
	if c3 := seg.Conditions[3]; c3.Operator != SegmentOpIn || len(c3.Values) != 2 || c3.Values[1] != "api" || c3.Value != "" {
		t.Errorf("condition 3 = %+v", c3)
	}
	if c4 := seg.Conditions[4]; c4.Operator != SegmentOpIsTrue || c4.Value != "" || c4.Values != nil {
		t.Errorf("condition 4 = %+v", c4)
	}
	if seg.UpdatedAt.Year() != 2026 || seg.UpdatedAt.Month() != 8 || seg.UpdatedAt.Day() != 20 {
		t.Errorf("updated_at = %v", seg.UpdatedAt)
	}
}

// A condition round-trips without inventing operands: a valueless operator
// must not serialize an empty value or values key.
func TestSegmentConditionEncoding(t *testing.T) {
	b, err := json.Marshal(SegmentCondition{Field: "email", Operator: SegmentOpIsNotEmpty})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"field":"email","operator":"is_not_empty"}` {
		t.Errorf("encoded = %s", b)
	}
}

func TestSegmentPreviewDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"contact_count": 412}`)
	res, _, err := c.Segments.Preview(context.Background(), &SegmentPreviewParams{
		Conditions: []SegmentCondition{{Field: "company", Operator: SegmentOpContains, Value: "acme"}},
	})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if res.ContactCount != 412 {
		t.Errorf("contact_count = %d, want 412", res.ContactCount)
	}
	// Match is left to the server default when unset; conditions always go.
	if strings.TrimSpace(got.body) != `{"conditions":[{"field":"company","operator":"contains","value":"acme"}]}` {
		t.Errorf("body = %s", got.body)
	}
}

func TestSegmentFieldsDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"data": [
	  {"field": "source", "label": "Source", "group": "Contact", "kind": "enum", "options": ["unknown", "manual", "import"]},
	  {"field": "custom.industry", "label": "industry", "group": "Custom field", "kind": "text"}
	]}`)
	fields, _, err := c.Segments.Fields(context.Background())
	if err != nil {
		t.Fatalf("Fields: %v", err)
	}
	if len(fields) != 2 {
		t.Fatalf("fields = %d, want 2", len(fields))
	}
	if fields[0].Kind != SegmentFieldEnum || len(fields[0].Options) != 3 {
		t.Errorf("field 0 = %+v", fields[0])
	}
	if fields[1].Kind != SegmentFieldText || fields[1].Options != nil || !strings.HasPrefix(fields[1].Field, SegmentCustomFieldPrefix) {
		t.Errorf("field 1 = %+v", fields[1])
	}
}

func TestSegmentOverridesAndAddToCampaignDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"data": [
	  {"contact_id": "ct_1", "first_name": "Dana", "last_name": "Lee", "email": "dana@acme.com", "company": "Acme", "mode": "include", "created_at": "2026-08-02T10:00:00Z"},
	  {"contact_id": "ct_2", "first_name": "", "last_name": "", "email": "x@acme.com", "company": "", "mode": "exclude", "created_at": "2026-08-01T10:00:00Z"}
	]}`)
	overrides, _, err := c.Segments.Overrides(context.Background(), "seg_1")
	if err != nil {
		t.Fatalf("Overrides: %v", err)
	}
	if len(overrides) != 2 || overrides[0].Mode != SegmentMemberInclude || overrides[1].Mode != SegmentMemberExclude {
		t.Errorf("overrides = %+v", overrides)
	}
	if overrides[0].ContactID != "ct_1" || overrides[0].Email != "dana@acme.com" {
		t.Errorf("override 0 = %+v", overrides[0])
	}

	c = recordingFixtureClient(t, &got, `{"campaign_id": "camp_1", "added": 40, "members": 412}`)
	res, _, err := c.Segments.AddToCampaign(context.Background(), "seg_1", "camp_1")
	if err != nil {
		t.Fatalf("AddToCampaign: %v", err)
	}
	if res.CampaignID != "camp_1" || res.Added != 40 || res.Members != 412 {
		t.Errorf("result = %+v", res)
	}
}

func TestSegmentListEmpty(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"data": null}`)
	segs, _, err := c.Segments.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if segs == nil || len(segs) != 0 {
		t.Errorf("want an empty, non-nil slice, got %#v", segs)
	}
}
