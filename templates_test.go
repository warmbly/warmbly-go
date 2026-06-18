package warmbly

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

func TestTemplateListParamsValues(t *testing.T) {
	t.Run("nil receiver yields empty query", func(t *testing.T) {
		var p *TemplateListParams
		q := p.values()
		if len(q) != 0 {
			t.Errorf("nil params produced %d query keys, want 0: %v", len(q), q)
		}
	})

	t.Run("search and limit set", func(t *testing.T) {
		p := &TemplateListParams{
			ListOptions: ListOptions{Limit: 25, Cursor: "CUR"},
			Search:      "welcome",
		}
		q := p.values()
		if got := q.Get("search"); got != "welcome" {
			t.Errorf("search = %q, want %q", got, "welcome")
		}
		if got := q.Get("limit"); got != "25" {
			t.Errorf("limit = %q, want %q", got, "25")
		}
		if got := q.Get("cursor"); got != "CUR" {
			t.Errorf("cursor = %q, want %q", got, "CUR")
		}
	})

	t.Run("empty search omits the key", func(t *testing.T) {
		p := &TemplateListParams{}
		q := p.values()
		if _, ok := q["search"]; ok {
			t.Errorf("empty search should not set the key: %v", q)
		}
	})
}

func TestTemplateList(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/v1/templates" {
			t.Errorf("path = %q, want /v1/templates", r.URL.Path)
		}
		if got := r.URL.Query().Get("search"); got != "promo" {
			t.Errorf("search = %q, want promo", got)
		}
		if got := r.URL.Query().Get("limit"); got != "2" {
			t.Errorf("limit = %q, want 2", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "req_list")
		_, _ = w.Write([]byte(`{"data":[{"id":"tmpl_1","name":"Promo One","subject":"Hi"},{"id":"tmpl_2","name":"Promo Two","subject":"Yo"}],"pagination":{"has_more":false,"next_cursor":null}}`))
	})

	page, err := c.Templates.List(context.Background(), &TemplateListParams{
		ListOptions: ListOptions{Limit: 2},
		Search:      "promo",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(page.Data) != 2 {
		t.Fatalf("len(Data) = %d, want 2", len(page.Data))
	}
	if page.Data[0].ID != "tmpl_1" || page.Data[1].Name != "Promo Two" {
		t.Errorf("unexpected data: %+v", page.Data)
	}
	if resp := page.Response(); resp == nil || resp.RequestID != "req_list" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestTemplateListNilParams(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("expected no query, got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"pagination":{"has_more":false}}`))
	})

	page, err := c.Templates.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(page.Data) != 0 {
		t.Errorf("len(Data) = %d, want 0", len(page.Data))
	}
}

func TestTemplateListError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom","message":"explode"}`))
	})

	page, err := c.Templates.List(context.Background(), nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if page != nil {
		t.Errorf("page = %+v, want nil on error", page)
	}
}

func TestTemplateGet(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.EscapedPath() != "/v1/templates/tmpl%20space" {
			t.Errorf("escaped path = %q, want /v1/templates/tmpl%%20space", r.URL.EscapedPath())
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "req_get")
		w.Header().Set("X-RateLimit-Limit", "100")
		w.Header().Set("X-RateLimit-Remaining", "97")
		_, _ = w.Write([]byte(`{"id":"tmpl_space","name":"Spaced","subject":"S","body_plain":"hi","tags":["a","b"]}`))
	})

	tmpl, resp, err := c.Templates.Get(context.Background(), "tmpl space")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if tmpl.ID != "tmpl_space" || tmpl.Name != "Spaced" {
		t.Errorf("unexpected template: %+v", tmpl)
	}
	if len(tmpl.Tags) != 2 || tmpl.Tags[0] != "a" {
		t.Errorf("tags = %v, want [a b]", tmpl.Tags)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if resp.RequestID != "req_get" {
		t.Errorf("request id = %q, want req_get", resp.RequestID)
	}
	if resp.RateLimit.Limit != 100 || resp.RateLimit.Remaining != 97 {
		t.Errorf("unexpected rate limit: %+v", resp.RateLimit)
	}
}

func TestTemplateGetError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "req_404")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not_found","message":"nope","code":"resource_not_found"}`))
	})

	tmpl, resp, err := c.Templates.Get(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected an error")
	}
	if tmpl != nil {
		t.Errorf("tmpl = %+v, want nil on error", tmpl)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false; err = %v", err)
	}
	if resp == nil || resp.RequestID != "req_404" {
		t.Errorf("unexpected response on error: %+v", resp)
	}
}

func TestTemplateCreate(t *testing.T) {
	var body map[string]any
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/templates" {
			t.Errorf("path = %q, want /v1/templates", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"tmpl_new","name":"Welcome","subject":"Hello there"}`))
	})

	tmpl, resp, err := c.Templates.Create(context.Background(), &TemplateCreateParams{
		Name:      "Welcome",
		Subject:   "Hello there",
		BodyPlain: "Body text",
		Tags:      []string{"onboarding"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tmpl.ID != "tmpl_new" || tmpl.Name != "Welcome" {
		t.Errorf("unexpected template: %+v", tmpl)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", resp.StatusCode)
	}
	if body["name"] != "Welcome" || body["subject"] != "Hello there" || body["body_plain"] != "Body text" {
		t.Errorf("unexpected request body: %v", body)
	}
	tags, ok := body["tags"].([]any)
	if !ok || len(tags) != 1 || tags[0] != "onboarding" {
		t.Errorf("tags in body = %v, want [onboarding]", body["tags"])
	}
}

func TestTemplateCreateError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid","message":"bad name","code":"validation_error"}`))
	})

	tmpl, _, err := c.Templates.Create(context.Background(), &TemplateCreateParams{Name: ""})
	if err == nil {
		t.Fatal("expected an error")
	}
	if tmpl != nil {
		t.Errorf("tmpl = %+v, want nil on error", tmpl)
	}
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As(*Error) failed; err = %v", err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", apiErr.StatusCode)
	}
}

func TestTemplateUpdate(t *testing.T) {
	var body map[string]any
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		if r.URL.Path != "/v1/templates/tmpl_1" {
			t.Errorf("path = %q, want /v1/templates/tmpl_1", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"tmpl_1","name":"Renamed","subject":"New"}`))
	})

	newName := "Renamed"
	newTags := []string{"x", "y"}
	tmpl, resp, err := c.Templates.Update(context.Background(), "tmpl_1", &TemplateUpdateParams{
		Name: &newName,
		Tags: &newTags,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if tmpl.Name != "Renamed" {
		t.Errorf("name = %q, want Renamed", tmpl.Name)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if body["name"] != "Renamed" {
		t.Errorf("body name = %v, want Renamed", body["name"])
	}
	if _, ok := body["subject"]; ok {
		t.Errorf("nil subject should be omitted from body: %v", body)
	}
	tags, ok := body["tags"].([]any)
	if !ok || len(tags) != 2 {
		t.Errorf("tags in body = %v, want [x y]", body["tags"])
	}
}

func TestTemplateUpdateError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"conflict","message":"name taken"}`))
	})

	name := "dup"
	tmpl, _, err := c.Templates.Update(context.Background(), "tmpl_1", &TemplateUpdateParams{Name: &name})
	if err == nil {
		t.Fatal("expected an error")
	}
	if tmpl != nil {
		t.Errorf("tmpl = %+v, want nil on error", tmpl)
	}
}

func TestTemplateDelete(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		if r.URL.Path != "/v1/templates/tmpl_1" {
			t.Errorf("path = %q, want /v1/templates/tmpl_1", r.URL.Path)
		}
		w.Header().Set("X-Request-ID", "req_del")
		w.WriteHeader(http.StatusNoContent)
	})

	resp, err := c.Templates.Delete(context.Background(), "tmpl_1")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want 204", resp.StatusCode)
	}
	if resp.RequestID != "req_del" {
		t.Errorf("request id = %q, want req_del", resp.RequestID)
	}
}

func TestTemplateDeleteError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden","message":"no access"}`))
	})

	resp, err := c.Templates.Delete(context.Background(), "tmpl_1")
	if err == nil {
		t.Fatal("expected an error")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Errorf("unexpected response on error: %+v", resp)
	}
}
