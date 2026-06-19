package warmbly

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestAPIKeyRevoked(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		key  APIKey
		want bool
	}{
		{name: "not revoked", key: APIKey{}, want: false},
		{name: "revoked", key: APIKey{RevokedAt: &now}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.key.Revoked(); got != tt.want {
				t.Errorf("Revoked() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAPIKeyListParamsValues(t *testing.T) {
	t.Run("nil receiver yields empty", func(t *testing.T) {
		var p *APIKeyListParams
		q := p.values()
		if len(q) != 0 {
			t.Errorf("values() = %v, want empty", q)
		}
	})

	t.Run("fields populate query", func(t *testing.T) {
		p := &APIKeyListParams{
			ListOptions: ListOptions{Limit: 25, Cursor: "CUR"},
			Search:      "prod",
		}
		q := p.values()
		if got := q.Get("limit"); got != "25" {
			t.Errorf("limit = %q, want 25", got)
		}
		if got := q.Get("cursor"); got != "CUR" {
			t.Errorf("cursor = %q, want CUR", got)
		}
		if got := q.Get("search"); got != "prod" {
			t.Errorf("search = %q, want prod", got)
		}
	})

	t.Run("empty fields omit query keys", func(t *testing.T) {
		p := &APIKeyListParams{}
		q := p.values()
		if len(q) != 0 {
			t.Errorf("values() = %v, want empty", q)
		}
	})
}

func TestAPIKeyList(t *testing.T) {
	var gotPath, gotMethod, gotSearch, gotLimit string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotSearch = r.URL.Query().Get("search")
		gotLimit = r.URL.Query().Get("limit")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"key_1","name":"primary"},{"id":"key_2","name":"secondary"}],"pagination":{"has_more":false,"next_cursor":null}}`))
	})

	page, err := c.APIKeys.List(context.Background(), &APIKeyListParams{
		ListOptions: ListOptions{Limit: 10},
		Search:      "prim",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/v1/api-keys" {
		t.Errorf("path = %q, want /v1/api-keys", gotPath)
	}
	if gotSearch != "prim" {
		t.Errorf("search = %q, want prim", gotSearch)
	}
	if gotLimit != "10" {
		t.Errorf("limit = %q, want 10", gotLimit)
	}
	if len(page.Data) != 2 {
		t.Fatalf("len(Data) = %d, want 2", len(page.Data))
	}
	if page.Data[0].ID != "key_1" || page.Data[1].Name != "secondary" {
		t.Errorf("unexpected data: %+v", page.Data)
	}
}

func TestAPIKeyListNilParams(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.RawQuery; got != "" {
			t.Errorf("RawQuery = %q, want empty", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"pagination":{"has_more":false,"next_cursor":null}}`))
	})

	page, err := c.APIKeys.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(page.Data) != 0 {
		t.Errorf("len(Data) = %d, want 0", len(page.Data))
	}
}

func TestAPIKeyGet(t *testing.T) {
	var gotPath, gotMethod string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("X-Request-ID", "req_get")
		w.Header().Set("X-RateLimit-Limit", "60")
		w.Header().Set("X-RateLimit-Remaining", "59")
		_, _ = w.Write([]byte(`{"id":"key_1","name":"primary","prefix":"wmbly_ab","suffix":"cd12"}`))
	})

	key, resp, err := c.APIKeys.Get(context.Background(), "key_1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/v1/api-keys/key_1" {
		t.Errorf("path = %q, want /v1/api-keys/key_1", gotPath)
	}
	if key.ID != "key_1" || key.Name != "primary" {
		t.Errorf("unexpected key: %+v", key)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if resp.RequestID != "req_get" {
		t.Errorf("RequestID = %q, want req_get", resp.RequestID)
	}
	if resp.RateLimit.Limit != 60 || resp.RateLimit.Remaining != 59 {
		t.Errorf("unexpected rate limit: %+v", resp.RateLimit)
	}
}

func TestAPIKeyGetEscapesID(t *testing.T) {
	var gotPath, gotRawPath string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotRawPath = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"id":"a/b","name":"weird"}`))
	})

	key, _, err := c.APIKeys.Get(context.Background(), "a/b")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotPath != "/v1/api-keys/a/b" {
		t.Errorf("decoded path = %q, want /v1/api-keys/a/b", gotPath)
	}
	if gotRawPath != "/v1/api-keys/a%2Fb" {
		t.Errorf("escaped path = %q, want /v1/api-keys/a%%2Fb", gotRawPath)
	}
	if key.ID != "a/b" {
		t.Errorf("id = %q, want a/b", key.ID)
	}
}

func TestAPIKeyGetError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "req_404")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not_found","message":"No such key.","code":"resource_not_found"}`))
	})

	key, resp, err := c.APIKeys.Get(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected an error")
	}
	if key != nil {
		t.Errorf("key = %+v, want nil", key)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false; err = %v", err)
	}
	if resp == nil || resp.StatusCode != http.StatusNotFound {
		t.Errorf("resp = %+v, want status 404", resp)
	}
}

func TestAPIKeyCreate(t *testing.T) {
	var gotMethod, gotPath string
	var body map[string]any
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"key_new","name":"deploy","prefix":"wmbly_zz","secret":"wmbly_supersecret"}`))
	})

	key, resp, err := c.APIKeys.Create(context.Background(), &APIKeyCreateParams{
		Name:               "deploy",
		Scopes:             []string{"campaigns:read"},
		RateLimitPerMinute: 120,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/v1/api-keys" {
		t.Errorf("path = %q, want /v1/api-keys", gotPath)
	}
	if body["name"] != "deploy" {
		t.Errorf("body name = %v, want deploy", body["name"])
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", resp.StatusCode)
	}
	if key.ID != "key_new" {
		t.Errorf("id = %q, want key_new", key.ID)
	}
	if key.Secret != "wmbly_supersecret" {
		t.Errorf("secret = %q, want wmbly_supersecret", key.Secret)
	}
}

func TestAPIKeyCreateError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad_request","message":"name required","code":"invalid_request"}`))
	})

	key, _, err := c.APIKeys.Create(context.Background(), &APIKeyCreateParams{})
	if err == nil {
		t.Fatal("expected an error")
	}
	if key != nil {
		t.Errorf("key = %+v, want nil", key)
	}
}

func TestAPIKeyUpdate(t *testing.T) {
	var gotMethod, gotPath string
	var raw map[string]json.RawMessage
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&raw)
		_, _ = w.Write([]byte(`{"id":"key_1","name":"renamed"}`))
	})

	newName := "renamed"
	key, resp, err := c.APIKeys.Update(context.Background(), "key_1", &APIKeyUpdateParams{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("method = %q, want PATCH", gotMethod)
	}
	if gotPath != "/v1/api-keys/key_1" {
		t.Errorf("path = %q, want /v1/api-keys/key_1", gotPath)
	}
	// Only the non-nil Name field should be present in the body.
	if _, ok := raw["name"]; !ok {
		t.Errorf("body missing name; got %v", raw)
	}
	for _, k := range []string{"scopes", "allowed_email_accounts", "allowed_ips", "rate_limit_per_minute", "expires_at"} {
		if _, ok := raw[k]; ok {
			t.Errorf("body unexpectedly contains %q for nil field; got %v", k, raw)
		}
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if key.Name != "renamed" {
		t.Errorf("name = %q, want renamed", key.Name)
	}
}

func TestAPIKeyUpdateError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not_found","message":"missing","code":"resource_not_found"}`))
	})

	key, _, err := c.APIKeys.Update(context.Background(), "missing", &APIKeyUpdateParams{})
	if err == nil {
		t.Fatal("expected an error")
	}
	if key != nil {
		t.Errorf("key = %+v, want nil", key)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false; err = %v", err)
	}
}

func TestAPIKeyRevoke(t *testing.T) {
	var gotMethod, gotPath string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	})

	resp, err := c.APIKeys.Revoke(context.Background(), "key_1")
	if err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/v1/api-keys/key_1" {
		t.Errorf("path = %q, want /v1/api-keys/key_1", gotPath)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want 204", resp.StatusCode)
	}
}

func TestAPIKeyRevokeError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not_found","message":"missing","code":"resource_not_found"}`))
	})

	resp, err := c.APIKeys.Revoke(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false; err = %v", err)
	}
	if resp == nil || resp.StatusCode != http.StatusNotFound {
		t.Errorf("resp = %+v, want status 404", resp)
	}
}

func TestAPIKeyPermissions(t *testing.T) {
	var gotMethod, gotPath string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"data":[{"key":"campaigns:read","description":"Read campaigns","group":"campaigns"},{"key":"contacts:write","description":"Write contacts"}]}`))
	})

	perms, resp, err := c.APIKeys.Permissions(context.Background())
	if err != nil {
		t.Fatalf("Permissions: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/v1/api-keys/permissions" {
		t.Errorf("path = %q, want /v1/api-keys/permissions", gotPath)
	}
	if len(perms) != 2 {
		t.Fatalf("len(perms) = %d, want 2", len(perms))
	}
	if perms[0].Key != "campaigns:read" || perms[0].Group != "campaigns" {
		t.Errorf("perms[0] = %+v", perms[0])
	}
	if perms[1].Key != "contacts:write" || perms[1].Description != "Write contacts" {
		t.Errorf("perms[1] = %+v", perms[1])
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestAPIKeyPermissionsError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"server_error","message":"boom"}`))
	})

	perms, _, err := c.APIKeys.Permissions(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if perms != nil {
		t.Errorf("perms = %+v, want nil", perms)
	}
}

func TestAPIKeyUsageSummary(t *testing.T) {
	var gotMethod, gotPath string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"total_requests":1000,"throttled_requests":12,"window_start":"2026-01-01T00:00:00Z","window_end":"2026-01-31T23:59:59Z"}`))
	})

	summary, resp, err := c.APIKeys.UsageSummary(context.Background())
	if err != nil {
		t.Fatalf("UsageSummary: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/v1/api-keys/usage/summary" {
		t.Errorf("path = %q, want /v1/api-keys/usage/summary", gotPath)
	}
	if summary.TotalRequests != 1000 || summary.ThrottledRequests != 12 {
		t.Errorf("unexpected summary: %+v", summary)
	}
	if summary.WindowStart == nil || summary.WindowEnd == nil {
		t.Fatalf("expected non-nil window bounds: %+v", summary)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestAPIKeyUsageSummaryError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden","message":"nope"}`))
	})

	summary, _, err := c.APIKeys.UsageSummary(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if summary != nil {
		t.Errorf("summary = %+v, want nil", summary)
	}
}
