package warmbly

import (
	"context"
	"strings"
	"testing"
)

func TestSuppressionRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()

	cases := []struct {
		name       string
		call       func() error
		wantMethod string
		wantPath   string
		wantQuery  []string
		wantBody   string
	}{
		{"List", func() error { _, e := c.Suppressions.List(ctx, nil); return e }, "GET", "/v1/suppressions", nil, ""},
		{"List filtered", func() error {
			_, e := c.Suppressions.List(ctx, &SuppressionListParams{
				ListOptions: ListOptions{Limit: 100, Cursor: "cur_1"},
				Query:       "acme",
			})
			return e
		}, "GET", "/v1/suppressions", []string{"limit=100", "cursor=cur_1", "q=acme"}, ""},
		{"Add", func() error {
			_, _, e := c.Suppressions.Add(ctx, &SuppressionAddParams{
				Entries: []SuppressionEntry{
					{Value: "dana@acme.com", Reason: "Asked us by phone"},
					{Value: "acme.com"},
					{Value: "@partner.io"},
				},
				Reason: "Existing customers",
			})
			return e
		}, "POST", "/v1/suppressions", nil, `{"entries":[{"value":"dana@acme.com","reason":"Asked us by phone"},{"value":"acme.com"},{"value":"@partner.io"}],"reason":"Existing customers"}`},
		{"Delete", func() error { _, e := c.Suppressions.Delete(ctx, "sup_1"); return e }, "DELETE", "/v1/suppressions/sup_1", nil, ""},
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
			for _, want := range tc.wantQuery {
				if !strings.Contains(got.rawQuery, want) {
					t.Errorf("query = %q, want it to contain %q", got.rawQuery, want)
				}
			}
			if tc.wantBody != "" && strings.TrimSpace(got.body) != tc.wantBody {
				t.Errorf("body = %s, want %s", got.body, tc.wantBody)
			}
		})
	}
}

func TestSuppressionListDecode(t *testing.T) {
	const fixture = `{
	  "data": [
	    {
	      "id": "5a0c0000-0000-4000-8000-000000000001",
	      "organization_id": "9b1e0000-0000-4000-8000-000000000002",
	      "email": "dana@acme.com",
	      "kind": "email",
	      "reason": "clicked the unsubscribe link",
	      "source": "unsubscribe",
	      "campaign_id": "c1000000-0000-4000-8000-000000000003",
	      "metadata": { "via": "link" },
	      "created_at": "2026-09-01T10:12:00Z",
	      "updated_at": "2026-09-01T10:12:00Z"
	    },
	    {
	      "id": "7d2f0000-0000-4000-8000-000000000004",
	      "organization_id": "9b1e0000-0000-4000-8000-000000000002",
	      "email": "competitor.io",
	      "kind": "domain",
	      "reason": "Competitor",
	      "source": "manual",
	      "expires_at": "2027-01-01T00:00:00Z",
	      "metadata": { "added_by": "4e5f0000-0000-4000-8000-000000000005" },
	      "created_at": "2026-08-20T09:00:00Z",
	      "updated_at": "2026-08-20T09:00:00Z"
	    }
	  ],
	  "pagination": { "next_cursor": "MjAyNi0wOC0yMFQwOTowMDowMFp8N2QyZg", "has_more": true }
	}`
	var got recordedRequest
	c := recordingFixtureClient(t, &got, fixture)

	page, err := c.Suppressions.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(page.Data) != 2 {
		t.Fatalf("data = %d, want 2", len(page.Data))
	}
	first := page.Data[0]
	if first.Kind != SuppressionKindEmail || first.Source != SuppressionSourceUnsubscribe || first.Email != "dana@acme.com" {
		t.Errorf("first = %+v", first)
	}
	if first.CampaignID == nil || *first.CampaignID != "c1000000-0000-4000-8000-000000000003" {
		t.Errorf("first.campaign_id = %v", first.CampaignID)
	}
	if first.ExpiresAt != nil {
		t.Errorf("first.expires_at = %v, want nil", first.ExpiresAt)
	}
	if first.Metadata["via"] != "link" {
		t.Errorf("first.metadata = %v", first.Metadata)
	}
	second := page.Data[1]
	if second.Kind != SuppressionKindDomain || second.Source != SuppressionSourceManual || second.Email != "competitor.io" {
		t.Errorf("second = %+v", second)
	}
	if second.CampaignID != nil {
		t.Errorf("second.campaign_id = %v, want nil", second.CampaignID)
	}
	if second.ExpiresAt == nil || second.ExpiresAt.Year() != 2027 {
		t.Errorf("second.expires_at = %v", second.ExpiresAt)
	}
	if !page.HasMore() || page.NextCursor() != "MjAyNi0wOC0yMFQwOTowMDowMFp8N2QyZg" {
		t.Errorf("pagination = %+v", page.Pagination)
	}
	if page.Pagination.Total != nil {
		t.Errorf("the suppression envelope carries no total, got %v", *page.Pagination.Total)
	}

	// The next page must carry the cursor back on the same path.
	if _, err := page.Next(context.Background()); err != nil {
		t.Fatalf("Next: %v", err)
	}
	if got.path != "/v1/suppressions" || !strings.Contains(got.rawQuery, "cursor=MjAyNi0wOC0yMFQwOTowMDowMFp8N2QyZg") {
		t.Errorf("next page request = %s?%s", got.path, got.rawQuery)
	}
}

func TestSuppressionAddDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"added": 2, "skipped": ["not an address"]}`)
	res, _, err := c.Suppressions.Add(context.Background(), &SuppressionAddParams{
		Entries: []SuppressionEntry{{Value: "a@b.com"}, {Value: "b.com"}, {Value: "not an address"}},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if res.Added != 2 || len(res.Skipped) != 1 || res.Skipped[0] != "not an address" {
		t.Errorf("result = %+v", res)
	}
	// A batch-level reason is optional and must not be sent when empty.
	if strings.Contains(got.body, `"reason"`) {
		t.Errorf("body = %s, want no reason key", got.body)
	}

	c = recordingFixtureClient(t, &got, `{"added": 1, "skipped": null}`)
	res, _, err = c.Suppressions.Add(context.Background(), &SuppressionAddParams{Entries: []SuppressionEntry{{Value: "a@b.com"}}})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if res.Skipped == nil || len(res.Skipped) != 0 {
		t.Errorf("want an empty, non-nil skipped, got %#v", res.Skipped)
	}
}
