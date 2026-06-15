package warmbly

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := New(
		WithAPIKey("wmbly_test"),
		WithBaseURL(srv.URL+"/v1/"),
		WithMaxRetries(3),
		WithRetryWaitBounds(time.Millisecond, 5*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestNewRequiresCredentials(t *testing.T) {
	if _, err := New(); err == nil {
		t.Fatal("expected error when no credentials are configured")
	}
	if _, err := New(WithAPIKey("wmbly_x")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthHeaderAndUserAgent(t *testing.T) {
	var gotAuth, gotUA, gotAccept string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		gotAccept = r.Header.Get("Accept")
		_, _ = w.Write([]byte(`{"id":"camp_1","name":"Q3 Launch"}`))
	})

	camp, _, err := c.Campaigns.Get(context.Background(), "camp_1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if camp.Name != "Q3 Launch" {
		t.Errorf("name = %q, want %q", camp.Name, "Q3 Launch")
	}
	if gotAuth != "Bearer wmbly_test" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if !strings.Contains(gotUA, "warmbly-go/") {
		t.Errorf("User-Agent = %q, want it to contain warmbly-go/", gotUA)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q", gotAccept)
	}
}

func TestErrorDecoding(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "req_abc")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not_found","message":"Resource not found.","code":"resource_not_found"}`))
	})

	_, _, err := c.Campaigns.Get(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false; err = %v", err)
	}
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As(*Error) failed; err = %v", err)
	}
	if apiErr.StatusCode != 404 || apiErr.Code != "resource_not_found" || apiErr.RequestID != "req_abc" {
		t.Errorf("unexpected error fields: %+v", apiErr)
	}
}

func TestRetryOnServerError(t *testing.T) {
	var calls int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"service_down","message":"try again"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"camp_1","name":"ok"}`))
	})

	camp, _, err := c.Campaigns.Get(context.Background(), "camp_1")
	if err != nil {
		t.Fatalf("Get after retries: %v", err)
	}
	if camp.Name != "ok" {
		t.Errorf("name = %q", camp.Name)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("server received %d calls, want 3", got)
	}
}

func TestRetryRespectsContextCancellation(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled
	if _, _, err := c.Campaigns.Get(ctx, "x"); !errors.Is(err, context.Canceled) {
		t.Errorf("want context.Canceled, got %v", err)
	}
}

func TestRateLimitParsing(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "60")
		w.Header().Set("X-RateLimit-Remaining", "59")
		w.Header().Set("X-RateLimit-Policy", "60;w=60")
		_, _ = w.Write([]byte(`{"id":"camp_1","name":"x"}`))
	})

	_, resp, err := c.Campaigns.Get(context.Background(), "camp_1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.RateLimit.Limit != 60 || resp.RateLimit.Remaining != 59 || resp.RateLimit.Policy != "60;w=60" {
		t.Errorf("unexpected rate limit: %+v", resp.RateLimit)
	}
}

func TestPaginationAutoPaging(t *testing.T) {
	var sawCursor string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/campaigns" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		cursor := r.URL.Query().Get("cursor")
		w.Header().Set("Content-Type", "application/json")
		if cursor == "" {
			_, _ = w.Write([]byte(`{"data":[{"id":"c1","name":"one"},{"id":"c2","name":"two"}],"pagination":{"has_more":true,"next_cursor":"CURSOR2"}}`))
			return
		}
		sawCursor = cursor
		_, _ = w.Write([]byte(`{"data":[{"id":"c3","name":"three"}],"pagination":{"has_more":false,"next_cursor":null}}`))
	})

	page, err := c.Campaigns.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var names []string
	for camp, err := range page.All(context.Background()) {
		if err != nil {
			t.Fatalf("iterating: %v", err)
		}
		names = append(names, camp.Name)
	}
	want := []string{"one", "two", "three"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("collected %v, want %v", names, want)
	}
	if sawCursor != "CURSOR2" {
		t.Errorf("second page cursor = %q, want CURSOR2", sawCursor)
	}
}

func TestPostEncodesJSONBody(t *testing.T) {
	var body map[string]any
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q", ct)
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"camp_new","name":"Created"}`))
	})

	camp, resp, err := c.Campaigns.Create(context.Background(), &CampaignCreateParams{Name: "Created", DailyLimit: 50})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d", resp.StatusCode)
	}
	if camp.ID != "camp_new" {
		t.Errorf("id = %q", camp.ID)
	}
	if body["name"] != "Created" {
		t.Errorf("request body name = %v", body["name"])
	}
}
