package warmbly

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"testing"
)

func TestOAuthAppListParamsValues(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var p *OAuthAppListParams
		q := p.values()
		if len(q) != 0 {
			t.Errorf("nil params produced %d query values, want 0", len(q))
		}
	})

	t.Run("limit", func(t *testing.T) {
		p := &OAuthAppListParams{ListOptions: ListOptions{Limit: 25, Cursor: "cur_abc"}}
		q := p.values()
		if got := q.Get("limit"); got != "25" {
			t.Errorf("limit = %q, want %q", got, "25")
		}
		if got := q.Get("cursor"); got != "cur_abc" {
			t.Errorf("cursor = %q, want %q", got, "cur_abc")
		}
	})
}

func TestOAuthAppList(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/v1/oauth/applications" {
			t.Errorf("path = %q, want /v1/oauth/applications", r.URL.Path)
		}
		if got := r.URL.Query().Get("limit"); got != "2" {
			t.Errorf("limit query = %q, want 2", got)
		}
		w.Header().Set("X-Request-ID", "req_list")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"app_1","name":"First","client_id":"cid_1"},{"id":"app_2","name":"Second","client_id":"cid_2"}],"pagination":{"has_more":false,"next_cursor":null}}`))
	})

	page, err := c.OAuthApps.List(context.Background(), &OAuthAppListParams{ListOptions: ListOptions{Limit: 2}})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(page.Data) != 2 {
		t.Fatalf("len(Data) = %d, want 2", len(page.Data))
	}
	if page.Data[0].ID != "app_1" || page.Data[1].Name != "Second" {
		t.Errorf("unexpected data: %+v", page.Data)
	}
	if resp := page.Response(); resp == nil || resp.RequestID != "req_list" {
		t.Errorf("Response RequestID = %v, want req_list", resp)
	}
}

func TestOAuthAppGet(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		// url.PathEscape encodes the slash; net/http decodes Path back, but the
		// escaped form is preserved in EscapedPath / RawPath.
		if r.URL.EscapedPath() != "/v1/oauth/applications/app%2F1" {
			t.Errorf("escaped path = %q, want /v1/oauth/applications/app%%2F1", r.URL.EscapedPath())
		}
		if r.URL.Path != "/v1/oauth/applications/app/1" {
			t.Errorf("decoded path = %q, want /v1/oauth/applications/app/1", r.URL.Path)
		}
		w.Header().Set("X-Request-ID", "req_get")
		w.Header().Set("X-RateLimit-Limit", "100")
		w.Header().Set("X-RateLimit-Remaining", "99")
		_, _ = w.Write([]byte(`{"id":"app/1","name":"App One","client_id":"cid_1","redirect_uris":["https://x.test/cb"],"scopes":["campaigns:read"],"status":"active"}`))
	})

	// id contains a slash to exercise url.PathEscape.
	app, resp, err := c.OAuthApps.Get(context.Background(), "app/1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if app.ID != "app/1" || app.Name != "App One" || app.ClientID != "cid_1" {
		t.Errorf("unexpected app: %+v", app)
	}
	if len(app.RedirectURIs) != 1 || app.RedirectURIs[0] != "https://x.test/cb" {
		t.Errorf("redirect URIs = %v", app.RedirectURIs)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if resp.RequestID != "req_get" {
		t.Errorf("RequestID = %q, want req_get", resp.RequestID)
	}
	if resp.RateLimit.Limit != 100 || resp.RateLimit.Remaining != 99 {
		t.Errorf("rate limit = %+v", resp.RateLimit)
	}
}

func TestOAuthAppGetError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "req_404")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not_found","message":"missing","code":"resource_not_found"}`))
	})

	app, resp, err := c.OAuthApps.Get(context.Background(), "nope")
	if err == nil {
		t.Fatal("expected error")
	}
	if app != nil {
		t.Errorf("app = %+v, want nil on error", app)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("errors.Is ErrNotFound = false; err = %v", err)
	}
	if resp == nil || resp.StatusCode != http.StatusNotFound {
		t.Errorf("resp = %v, want non-nil with 404", resp)
	}
}

func TestOAuthAppCreate(t *testing.T) {
	var body map[string]any
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/oauth/applications" {
			t.Errorf("path = %q, want /v1/oauth/applications", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"app_new","name":"My App","client_id":"cid_new","redirect_uris":["https://app.test/cb"],"scopes":["campaigns:read"],"status":"active","client_secret":"secret_xyz"}`))
	})

	app, resp, err := c.OAuthApps.Create(context.Background(), &OAuthAppCreateParams{
		Name:         "My App",
		RedirectURIs: []string{"https://app.test/cb"},
		Scopes:       []string{"campaigns:read"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", resp.StatusCode)
	}
	if app.ID != "app_new" || app.ClientID != "cid_new" {
		t.Errorf("unexpected app: %+v", app)
	}
	if app.ClientSecret != "secret_xyz" {
		t.Errorf("ClientSecret = %q, want secret_xyz", app.ClientSecret)
	}
	if body["name"] != "My App" {
		t.Errorf("request body name = %v, want My App", body["name"])
	}
	uris, ok := body["redirect_uris"].([]any)
	if !ok || len(uris) != 1 || uris[0] != "https://app.test/cb" {
		t.Errorf("request body redirect_uris = %v", body["redirect_uris"])
	}
}

func TestOAuthAppCreateError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":"validation","message":"name required","code":"validation_error"}`))
	})

	app, resp, err := c.OAuthApps.Create(context.Background(), &OAuthAppCreateParams{})
	if err == nil {
		t.Fatal("expected error")
	}
	if app != nil {
		t.Errorf("app = %+v, want nil", app)
	}
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As(*Error) failed; err = %v", err)
	}
	if resp == nil || resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("resp = %v, want 422", resp)
	}
}

func TestOAuthAppUpdate(t *testing.T) {
	var raw map[string]any
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		if r.URL.Path != "/v1/oauth/applications/app_1" {
			t.Errorf("path = %q, want /v1/oauth/applications/app_1", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("X-Request-ID", "req_patch")
		_, _ = w.Write([]byte(`{"id":"app_1","name":"Renamed","client_id":"cid_1","status":"inactive"}`))
	})

	name := "Renamed"
	status := "inactive"
	app, resp, err := c.OAuthApps.Update(context.Background(), "app_1", &OAuthAppUpdateParams{
		Name:   &name,
		Status: &status,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if app.Name != "Renamed" || app.Status != "inactive" {
		t.Errorf("unexpected app: %+v", app)
	}
	if resp.RequestID != "req_patch" {
		t.Errorf("RequestID = %q, want req_patch", resp.RequestID)
	}
	if raw["name"] != "Renamed" || raw["status"] != "inactive" {
		t.Errorf("request body = %v", raw)
	}
	// Only non-nil fields should be sent.
	if _, present := raw["description"]; present {
		t.Errorf("description should be omitted, got body %v", raw)
	}
}

func TestOAuthAppUpdateError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden","message":"denied","code":"permission_denied"}`))
	})

	app, resp, err := c.OAuthApps.Update(context.Background(), "app_1", &OAuthAppUpdateParams{})
	if err == nil {
		t.Fatal("expected error")
	}
	if app != nil {
		t.Errorf("app = %+v, want nil", app)
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Errorf("resp = %v, want 403", resp)
	}
}

func TestOAuthAppDelete(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		if r.URL.Path != "/v1/oauth/applications/app_1" {
			t.Errorf("path = %q, want /v1/oauth/applications/app_1", r.URL.Path)
		}
		w.Header().Set("X-Request-ID", "req_del")
		w.WriteHeader(http.StatusNoContent)
	})

	resp, err := c.OAuthApps.Delete(context.Background(), "app_1")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want 204", resp.StatusCode)
	}
	if resp.RequestID != "req_del" {
		t.Errorf("RequestID = %q, want req_del", resp.RequestID)
	}
}

func TestOAuthAppDeleteError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not_found","message":"gone","code":"resource_not_found"}`))
	})

	resp, err := c.OAuthApps.Delete(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("errors.Is ErrNotFound = false; err = %v", err)
	}
	if resp == nil || resp.StatusCode != http.StatusNotFound {
		t.Errorf("resp = %v, want 404", resp)
	}
}

func TestOAuthAppRotateSecret(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/oauth/applications/app_1/rotate-secret" {
			t.Errorf("path = %q, want /v1/oauth/applications/app_1/rotate-secret", r.URL.Path)
		}
		// rotate-secret sends a nil body.
		b, _ := io.ReadAll(r.Body)
		if len(b) != 0 {
			t.Errorf("body = %q, want empty", b)
		}
		w.Header().Set("X-Request-ID", "req_rotate")
		_, _ = w.Write([]byte(`{"id":"app_1","name":"App One","client_id":"cid_1","status":"active","client_secret":"rotated_secret"}`))
	})

	app, resp, err := c.OAuthApps.RotateSecret(context.Background(), "app_1")
	if err != nil {
		t.Fatalf("RotateSecret: %v", err)
	}
	if app.ID != "app_1" || app.ClientSecret != "rotated_secret" {
		t.Errorf("unexpected app: %+v", app)
	}
	if resp.RequestID != "req_rotate" {
		t.Errorf("RequestID = %q, want req_rotate", resp.RequestID)
	}
}

func TestOAuthAppRotateSecretError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Escapes the id and returns an error to drive the err branch.
		if r.URL.EscapedPath() != "/v1/oauth/applications/a%20b/rotate-secret" {
			t.Errorf("escaped path = %q, want 'a%%20b'", r.URL.EscapedPath())
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"server","message":"boom","code":"internal_error"}`))
	})

	app, resp, err := c.OAuthApps.RotateSecret(context.Background(), "a b")
	if err == nil {
		t.Fatal("expected error")
	}
	if app != nil {
		t.Errorf("app = %+v, want nil", app)
	}
	if resp == nil || resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("resp = %v, want 500", resp)
	}
}

// TestOAuthAppListEscapesQuery is a guard that values() flows through to the
// request query unchanged for url.Values encoding.
func TestOAuthAppListEscapesQuery(t *testing.T) {
	var gotRaw string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotRaw = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"data":[],"pagination":{"has_more":false}}`))
	})

	if _, err := c.OAuthApps.List(context.Background(), &OAuthAppListParams{ListOptions: ListOptions{Limit: 5, Cursor: "a b"}}); err != nil {
		t.Fatalf("List: %v", err)
	}
	q, err := url.ParseQuery(gotRaw)
	if err != nil {
		t.Fatalf("ParseQuery(%q): %v", gotRaw, err)
	}
	if q.Get("limit") != "5" || q.Get("cursor") != "a b" {
		t.Errorf("query = %v", q)
	}
}
