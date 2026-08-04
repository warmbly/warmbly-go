package warmbly

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPageListOptionsApply(t *testing.T) {
	t.Run("nil receiver leaves query empty", func(t *testing.T) {
		var o *ListOptions
		q := make(url.Values)
		o.apply(q)
		if len(q) != 0 {
			t.Errorf("expected no keys for nil receiver, got %v", q)
		}
	})

	t.Run("limit and cursor set", func(t *testing.T) {
		o := &ListOptions{Limit: 25, Cursor: "CUR"}
		q := make(url.Values)
		o.apply(q)
		if got := q.Get("limit"); got != "25" {
			t.Errorf("limit = %q, want 25", got)
		}
		if got := q.Get("cursor"); got != "CUR" {
			t.Errorf("cursor = %q, want CUR", got)
		}
	})

	t.Run("zero limit and empty cursor add nothing", func(t *testing.T) {
		o := &ListOptions{}
		q := make(url.Values)
		o.apply(q)
		if len(q) != 0 {
			t.Errorf("expected no keys, got %v", q)
		}
	})
}

func TestPageResponse(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/campaigns" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("X-Request-ID", "req_page")
		w.Header().Set("X-RateLimit-Limit", "100")
		w.Header().Set("X-RateLimit-Remaining", "97")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"c1","name":"one"}],"pagination":{"has_more":false,"next_cursor":null}}`))
	})

	page, err := c.Campaigns.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	resp := page.Response()
	if resp == nil {
		t.Fatal("Response() returned nil")
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
	if resp.RequestID != "req_page" {
		t.Errorf("RequestID = %q, want req_page", resp.RequestID)
	}
	if resp.RateLimit.Limit != 100 || resp.RateLimit.Remaining != 97 {
		t.Errorf("unexpected rate limit: %+v", resp.RateLimit)
	}
}

func TestPageNextCursorNil(t *testing.T) {
	t.Run("nil pointer yields empty string", func(t *testing.T) {
		p := &Page[Campaign]{Pagination: Pagination{NextCursor: nil}}
		if got := p.NextCursor(); got != "" {
			t.Errorf("NextCursor() = %q, want empty", got)
		}
	})

	t.Run("set pointer yields value", func(t *testing.T) {
		cur := "CURSOR9"
		p := &Page[Campaign]{Pagination: Pagination{NextCursor: &cur}}
		if got := p.NextCursor(); got != "CURSOR9" {
			t.Errorf("NextCursor() = %q, want CURSOR9", got)
		}
	})
}

func TestPageHasMore(t *testing.T) {
	cur := "C"
	empty := ""
	tests := []struct {
		name string
		page Page[Campaign]
		want bool
	}{
		{
			name: "has_more true and cursor set",
			page: Page[Campaign]{Pagination: Pagination{HasMore: true, NextCursor: &cur}},
			want: true,
		},
		{
			name: "has_more true but nil cursor",
			page: Page[Campaign]{Pagination: Pagination{HasMore: true, NextCursor: nil}},
			want: false,
		},
		{
			name: "has_more true but empty cursor",
			page: Page[Campaign]{Pagination: Pagination{HasMore: true, NextCursor: &empty}},
			want: false,
		},
		{
			name: "has_more false with cursor set",
			page: Page[Campaign]{Pagination: Pagination{HasMore: false, NextCursor: &cur}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.page
			if got := p.HasMore(); got != tt.want {
				t.Errorf("HasMore() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPageNext(t *testing.T) {
	t.Run("no more pages when not HasMore", func(t *testing.T) {
		p := &Page[Campaign]{Pagination: Pagination{HasMore: false}}
		next, err := p.Next(context.Background())
		if !errors.Is(err, ErrNoMorePages) {
			t.Errorf("err = %v, want ErrNoMorePages", err)
		}
		if next != nil {
			t.Errorf("next = %v, want nil", next)
		}
	})

	t.Run("no more pages when fetch is nil", func(t *testing.T) {
		cur := "C"
		p := &Page[Campaign]{Pagination: Pagination{HasMore: true, NextCursor: &cur}}
		next, err := p.Next(context.Background())
		if !errors.Is(err, ErrNoMorePages) {
			t.Errorf("err = %v, want ErrNoMorePages", err)
		}
		if next != nil {
			t.Errorf("next = %v, want nil", next)
		}
	})

	t.Run("fetches the following page", func(t *testing.T) {
		var sawCursor string
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/campaigns" {
				t.Errorf("unexpected path %q", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			switch cursor := r.URL.Query().Get("cursor"); cursor {
			case "":
				_, _ = w.Write([]byte(`{"data":[{"id":"c1","name":"one"}],"pagination":{"has_more":true,"next_cursor":"CURSOR2"}}`))
			default:
				sawCursor = cursor
				_, _ = w.Write([]byte(`{"data":[{"id":"c2","name":"two"}],"pagination":{"has_more":false,"next_cursor":null}}`))
			}
		})

		page, err := c.Campaigns.List(context.Background(), nil)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		next, err := page.Next(context.Background())
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if sawCursor != "CURSOR2" {
			t.Errorf("requested cursor = %q, want CURSOR2", sawCursor)
		}
		if len(next.Data) != 1 || next.Data[0].Name != "two" {
			t.Errorf("unexpected second page data: %+v", next.Data)
		}
		if next.HasMore() {
			t.Errorf("second page should report no more pages")
		}
		if _, err := next.Next(context.Background()); !errors.Is(err, ErrNoMorePages) {
			t.Errorf("third Next err = %v, want ErrNoMorePages", err)
		}
	})
}

func TestPageAll(t *testing.T) {
	t.Run("iterates across pages", func(t *testing.T) {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch cursor := r.URL.Query().Get("cursor"); cursor {
			case "":
				_, _ = w.Write([]byte(`{"data":[{"id":"c1","name":"one"},{"id":"c2","name":"two"}],"pagination":{"has_more":true,"next_cursor":"CURSOR2"}}`))
			default:
				_, _ = w.Write([]byte(`{"data":[{"id":"c3","name":"three"}],"pagination":{"has_more":false,"next_cursor":null}}`))
			}
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
		if got := strings.Join(names, ","); got != "one,two,three" {
			t.Errorf("collected %q, want one,two,three", got)
		}
	})

	t.Run("early break stops iteration", func(t *testing.T) {
		var calls int32
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&calls, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"c1","name":"one"},{"id":"c2","name":"two"}],"pagination":{"has_more":true,"next_cursor":"CURSOR2"}}`))
		})

		page, err := c.Campaigns.List(context.Background(), nil)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		var n int
		for camp, err := range page.All(context.Background()) {
			if err != nil {
				t.Fatalf("iterating: %v", err)
			}
			n++
			if camp.Name == "one" {
				break
			}
		}
		if n != 1 {
			t.Errorf("iterated %d items, want 1 before break", n)
		}
		if got := atomic.LoadInt32(&calls); got != 1 {
			t.Errorf("server hit %d times, want 1 (no extra fetch after break)", got)
		}
	})

	t.Run("propagates error from a later page", func(t *testing.T) {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch cursor := r.URL.Query().Get("cursor"); cursor {
			case "":
				_, _ = w.Write([]byte(`{"data":[{"id":"c1","name":"one"}],"pagination":{"has_more":true,"next_cursor":"CURSOR2"}}`))
			default:
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"not_found","message":"gone","code":"resource_not_found"}`))
			}
		})

		page, err := c.Campaigns.List(context.Background(), nil)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		var names []string
		var iterErr error
		for camp, err := range page.All(context.Background()) {
			if err != nil {
				iterErr = err
				if camp.Name != "" {
					t.Errorf("expected zero value on error, got %+v", camp)
				}
				break
			}
			names = append(names, camp.Name)
		}
		if iterErr == nil {
			t.Fatal("expected an error from the second page fetch")
		}
		if !errors.Is(iterErr, ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", iterErr)
		}
		if got := strings.Join(names, ","); got != "one" {
			t.Errorf("collected %q before error, want one", got)
		}
	})

	t.Run("single page with no fetch closure", func(t *testing.T) {
		cur := "C"
		// fetch is nil and HasMore is true: All must stop after the first page.
		p := &Page[Campaign]{
			Data:       []Campaign{{ID: "c1", Name: "only"}},
			Pagination: Pagination{HasMore: true, NextCursor: &cur},
		}
		var names []string
		for camp, err := range p.All(context.Background()) {
			if err != nil {
				t.Fatalf("iterating: %v", err)
			}
			names = append(names, camp.Name)
		}
		if got := strings.Join(names, ","); got != "only" {
			t.Errorf("collected %q, want only", got)
		}
	})
}

func TestPageCloneValues(t *testing.T) {
	orig := url.Values{
		"status": {"running"},
		"tags":   {"a", "b"},
	}
	clone := cloneValues(orig)

	if len(clone) != len(orig) {
		t.Fatalf("clone has %d keys, want %d", len(clone), len(orig))
	}
	if clone.Get("status") != "running" {
		t.Errorf("clone status = %q, want running", clone.Get("status"))
	}

	// Mutating the clone must not touch the original.
	clone.Set("status", "stopped")
	clone.Add("tags", "c")
	clone["new"] = []string{"x"}

	if orig.Get("status") != "running" {
		t.Errorf("original status mutated to %q", orig.Get("status"))
	}
	if len(orig["tags"]) != 2 {
		t.Errorf("original tags mutated to %v", orig["tags"])
	}
	if _, ok := orig["new"]; ok {
		t.Errorf("original gained a key from clone mutation: %v", orig)
	}

	// Mutating a slice element of the clone must not alias the original.
	clone["tags"][0] = "MUT"
	if orig["tags"][0] != "a" {
		t.Errorf("original tags[0] mutated to %q via shared backing array", orig["tags"][0])
	}
}

func TestPageEmptyClone(t *testing.T) {
	out := cloneValues(nil)
	if out == nil {
		t.Fatal("cloneValues(nil) returned nil map")
	}
	if len(out) != 0 {
		t.Errorf("cloneValues(nil) = %v, want empty", out)
	}
}
