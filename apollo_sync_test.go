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

// fixedClient answers every request with a fixed status and body and records
// the request, body included.
func fixedClient(t *testing.T, status int, body string, got *recordedRequest) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*got = recordedRequest{
			method: r.Method, path: r.URL.Path, rawQuery: r.URL.RawQuery,
			body: strings.TrimSpace(string(b)), idempotencyKey: r.Header.Get("Idempotency-Key"),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func checkRequest(t *testing.T, got recordedRequest, method, path, query, body string) {
	t.Helper()
	if got.method != method || got.path != path {
		t.Errorf("got %s %s, want %s %s", got.method, got.path, method, path)
	}
	if got.rawQuery != query {
		t.Errorf("query = %q, want %q", got.rawQuery, query)
	}
	if got.body != body {
		t.Errorf("body = %s, want %s", got.body, body)
	}
}

const (
	apolloConn   = "11111111-1111-4111-8111-111111111111"
	apolloSource = "22222222-2222-4222-8222-222222222222"
	apolloRun    = "33333333-3333-4333-8333-333333333333"

	apolloSourceJSON = `{"id":"` + apolloSource + `","organization_id":"44444444-4444-4444-8444-444444444444","connection_id":"` + apolloConn + `","created_by_user_id":"55555555-5555-4555-8555-555555555555","name":"Q4 CFOs","list_id":"lst_9","list_name":"CFOs","campaign_id":"66666666-6666-4666-8666-666666666666","category_ids":["77777777-7777-4777-8777-777777777777"],"verified_only":true,"recurring":true,"enabled":true,"status":"idle","last_run_at":"2026-10-17T08:00:00Z","last_result":{"read":120,"imported":90,"updated":2,"skipped":10,"failed":1,"no_email":8,"unverified":6,"suppressed":3,"truncated":true,"already_seen":0,"next_page":4},"total_imported":90,"created_at":"2026-10-01T08:00:00Z","updated_at":"2026-10-17T08:05:00Z"}`
	apolloRunJSON    = `{"id":"` + apolloRun + `","organization_id":"44444444-4444-4444-8444-444444444444","connection_id":"` + apolloConn + `","overwrite":false,"status":"running","total":50,"processed":20,"enriched":15,"not_found":4,"failed":1,"credits_consumed":15,"started_at":"2026-10-18T09:00:00Z","created_at":"2026-10-18T08:59:00Z","updated_at":"2026-10-18T09:01:00Z"}`
)

func TestApolloOverview(t *testing.T) {
	var got recordedRequest
	c := fixedClient(t, 200, `{"connection_id":"`+apolloConn+`","label":"Apollo","status":"connected","health":"healthy","account_name":"Ada","account_email":"ada@acme.com","sources":2,"total_imported":90,"last_run_at":"2026-10-17T08:00:00Z"}`, &got)
	out, _, err := c.Apollo.Overview(context.Background(), apolloConn)
	if err != nil {
		t.Fatal(err)
	}
	checkRequest(t, got, "GET", "/v1/integrations/apollo/"+apolloConn, "", "")
	if out.Status != ConnectionConnected || out.AccountEmail != "ada@acme.com" || out.Sources != 2 || out.LastRunAt == nil {
		t.Errorf("decoded %+v", out)
	}
}

func TestApolloReplaceKey(t *testing.T) {
	var got recordedRequest
	c := fixedClient(t, 200, `{"connection_id":"`+apolloConn+`","label":"Apollo","status":"connected","health":"healthy","sources":0,"total_imported":0}`, &got)
	out, _, err := c.Apollo.ReplaceKey(context.Background(), apolloConn, "k_123")
	if err != nil {
		t.Fatal(err)
	}
	checkRequest(t, got, "PUT", "/v1/integrations/apollo/"+apolloConn+"/key", "", `{"api_key":"k_123"}`)
	if out.LastRunAt != nil || out.Health != "healthy" {
		t.Errorf("decoded %+v", out)
	}
}

func TestApolloLists(t *testing.T) {
	var got recordedRequest
	c := fixedClient(t, 200, `{"data":[{"id":"lst_9","name":"CFOs","count":340}]}`, &got)
	out, _, err := c.Apollo.Lists(context.Background(), apolloConn)
	if err != nil {
		t.Fatal(err)
	}
	checkRequest(t, got, "GET", "/v1/integrations/apollo/"+apolloConn+"/lists", "", "")
	if len(out) != 1 || out[0].ID != "lst_9" || out[0].Count != 340 {
		t.Errorf("decoded %+v", out)
	}
}

func TestApolloPreviewImport(t *testing.T) {
	var got recordedRequest
	c := fixedClient(t, 200, `{"total":340,"sample":[{"apollo_id":"a1","name":"Ada Lovelace","email":"ada@acme.com","email_status":"verified","title":"CFO","company":"Acme","already_in_warmbly":false,"will_import":true}]}`, &got)
	out, _, err := c.Apollo.PreviewImport(context.Background(), apolloConn, &ApolloPreviewParams{ListID: "lst_9", VerifiedOnly: Bool(false)})
	if err != nil {
		t.Fatal(err)
	}
	checkRequest(t, got, "POST", "/v1/integrations/apollo/"+apolloConn+"/import/preview", "", `{"list_id":"lst_9","verified_only":false}`)
	if out.Total != 340 || len(out.Sample) != 1 || !out.Sample[0].WillImport || out.Sample[0].AlreadyInWarmbly {
		t.Errorf("decoded %+v", out)
	}
}

func TestApolloImportSources(t *testing.T) {
	ctx := context.Background()
	base := "/v1/integrations/apollo/" + apolloConn + "/import-sources"

	t.Run("list", func(t *testing.T) {
		var got recordedRequest
		c := fixedClient(t, 200, `{"data":[`+apolloSourceJSON+`]}`, &got)
		out, _, err := c.Apollo.ListImportSources(ctx, apolloConn)
		if err != nil {
			t.Fatal(err)
		}
		checkRequest(t, got, "GET", base, "", "")
		s := out[0]
		if s.LastResult == nil || s.LastResult.NextPage != 4 || !s.LastResult.Truncated || s.CampaignID == nil || s.Status != ApolloSourceIdle || len(s.CategoryIDs) != 1 {
			t.Errorf("decoded %+v", s)
		}
	})

	t.Run("create 201", func(t *testing.T) {
		var got recordedRequest
		c := fixedClient(t, 201, apolloSourceJSON, &got)
		out, _, err := c.Apollo.CreateImportSource(ctx, apolloConn, &ApolloImportSourceCreateParams{
			ListID: "lst_9", ListName: "CFOs", Recurring: Bool(true), CategoryIDs: []string{"cat_1"},
		})
		if err != nil {
			t.Fatal(err)
		}
		checkRequest(t, got, "POST", base, "", `{"list_id":"lst_9","list_name":"CFOs","category_ids":["cat_1"],"recurring":true}`)
		if out.ID != apolloSource {
			t.Errorf("decoded %+v", out)
		}
	})

	t.Run("update clears campaign", func(t *testing.T) {
		var got recordedRequest
		c := fixedClient(t, 200, apolloSourceJSON, &got)
		_, _, err := c.Apollo.UpdateImportSource(ctx, apolloConn, apolloSource, &ApolloImportSourceUpdateParams{
			ClearCampaign: true, CampaignID: String("ignored"), Enabled: Bool(false),
		})
		if err != nil {
			t.Fatal(err)
		}
		checkRequest(t, got, "PATCH", base+"/"+apolloSource, "", `{"enabled":false,"campaign_id":null}`)
	})

	t.Run("update leaves campaign alone and clears tags", func(t *testing.T) {
		var got recordedRequest
		c := fixedClient(t, 200, apolloSourceJSON, &got)
		_, _, err := c.Apollo.UpdateImportSource(ctx, apolloConn, apolloSource, &ApolloImportSourceUpdateParams{CategoryIDs: &[]string{}})
		if err != nil {
			t.Fatal(err)
		}
		checkRequest(t, got, "PATCH", base+"/"+apolloSource, "", `{"category_ids":[]}`)
	})

	t.Run("run", func(t *testing.T) {
		var got recordedRequest
		c := fixedClient(t, 200, apolloSourceJSON, &got)
		if _, _, err := c.Apollo.RunImportSource(ctx, apolloConn, apolloSource); err != nil {
			t.Fatal(err)
		}
		checkRequest(t, got, "POST", base+"/"+apolloSource+"/run", "", "")
	})

	t.Run("run conflict", func(t *testing.T) {
		var got recordedRequest
		c := fixedClient(t, 409, `{"error":{"code":"import_running","message":"This import is already running."}}`, &got)
		if _, _, err := c.Apollo.RunImportSource(ctx, apolloConn, apolloSource); err == nil {
			t.Fatal("want an error")
		}
	})

	t.Run("delete 204", func(t *testing.T) {
		var got recordedRequest
		c := fixedClient(t, 204, ``, &got)
		resp, err := c.Apollo.DeleteImportSource(ctx, apolloConn, apolloSource)
		if err != nil {
			t.Fatal(err)
		}
		checkRequest(t, got, "DELETE", base+"/"+apolloSource, "", "")
		if resp.StatusCode != 204 {
			t.Errorf("status = %d", resp.StatusCode)
		}
	})
}

func TestApolloEnrichments(t *testing.T) {
	ctx := context.Background()
	base := "/v1/integrations/apollo/" + apolloConn + "/enrichments"

	t.Run("list with limit", func(t *testing.T) {
		var got recordedRequest
		c := fixedClient(t, 200, `{"data":[`+apolloRunJSON+`]}`, &got)
		out, _, err := c.Apollo.ListEnrichments(ctx, apolloConn, &ApolloEnrichmentListParams{Limit: 5})
		if err != nil {
			t.Fatal(err)
		}
		checkRequest(t, got, "GET", base, "limit=5", "")
		if len(out) != 1 || out[0].Status != ApolloEnrichmentRunning || out[0].CreditsConsumed != 15 || out[0].FinishedAt != nil || out[0].StartedAt == nil {
			t.Errorf("decoded %+v", out)
		}
	})

	t.Run("list without params", func(t *testing.T) {
		var got recordedRequest
		c := fixedClient(t, 200, `{"data":null}`, &got)
		out, _, err := c.Apollo.ListEnrichments(ctx, apolloConn, nil)
		if err != nil {
			t.Fatal(err)
		}
		checkRequest(t, got, "GET", base, "", "")
		if out == nil || len(out) != 0 {
			t.Errorf("want an empty non-nil slice, got %#v", out)
		}
	})

	t.Run("start 202", func(t *testing.T) {
		var got recordedRequest
		c := fixedClient(t, 202, apolloRunJSON, &got)
		out, _, err := c.Apollo.StartEnrichment(ctx, apolloConn, &ApolloEnrichParams{Contacts: []string{"ct_1", "ct_2"}, Overwrite: true}, WithIdempotencyKey("idem-1"))
		if err != nil {
			t.Fatal(err)
		}
		checkRequest(t, got, "POST", base, "", `{"contacts":["ct_1","ct_2"],"overwrite":true}`)
		if got.idempotencyKey != "idem-1" || out.ID != apolloRun {
			t.Errorf("key = %q, decoded %+v", got.idempotencyKey, out)
		}
	})

	t.Run("start by filter", func(t *testing.T) {
		var got recordedRequest
		c := fixedClient(t, 202, apolloRunJSON, &got)
		_, _, err := c.Apollo.StartEnrichment(ctx, apolloConn, &ApolloEnrichParams{All: true, Filters: &ContactSearchParams{Query: "acme"}, Exclude: []string{"ct_9"}})
		if err != nil {
			t.Fatal(err)
		}
		checkRequest(t, got, "POST", base, "", `{"all":true,"filters":{"query":"acme"},"exclude":["ct_9"],"overwrite":false}`)
	})

	t.Run("get", func(t *testing.T) {
		var got recordedRequest
		c := fixedClient(t, 200, apolloRunJSON, &got)
		out, _, err := c.Apollo.GetEnrichment(ctx, apolloConn, apolloRun)
		if err != nil {
			t.Fatal(err)
		}
		checkRequest(t, got, "GET", base+"/"+apolloRun, "", "")
		if out.Total != 50 || out.NotFound != 4 {
			t.Errorf("decoded %+v", out)
		}
	})
}

func TestApolloSourceUpdateParamsEmpty(t *testing.T) {
	b, err := json.Marshal(ApolloImportSourceUpdateParams{})
	if err != nil || string(b) != `{}` {
		t.Errorf("got %s, %v", b, err)
	}
}

// Unknown statuses and providers must still decode.
func TestApolloUnknownValuesDecode(t *testing.T) {
	var s ApolloImportSource
	if err := json.Unmarshal([]byte(`{"status":"paused_by_admin"}`), &s); err != nil || s.Status != "paused_by_admin" {
		t.Errorf("source: %v %+v", err, s)
	}
	var conn IntegrationConnection
	if err := json.Unmarshal([]byte(`{"provider":"apollo","status":"connected"}`), &conn); err != nil || conn.Provider != ProviderApollo {
		t.Errorf("connection: %v %+v", err, conn)
	}
	if err := json.Unmarshal([]byte(`{"provider":"a_future_one"}`), &conn); err != nil || conn.Provider != "a_future_one" {
		t.Errorf("future provider: %v %+v", err, conn)
	}
	if ProviderClay != "clay" || ActionClaySendRow != "clay.send_row" {
		t.Error("constants drifted from the server")
	}
}
