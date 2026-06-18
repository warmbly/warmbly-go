package warmbly

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOptionWithAPIKey(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if _, err := New(WithAPIKey("")); err == nil {
			t.Fatal("expected error for empty API key")
		}
	})
	t.Run("whitespace only", func(t *testing.T) {
		if _, err := New(WithAPIKey("  ")); err == nil {
			t.Fatal("expected error for whitespace-only API key")
		}
	})
	t.Run("trimmed value used on requests", func(t *testing.T) {
		var gotAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"id":"camp_1","name":"x"}`))
		}))
		t.Cleanup(srv.Close)

		c, err := New(WithAPIKey("  wmbly_padded  "), WithBaseURL(srv.URL+"/v1/"))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if _, _, err := c.Campaigns.Get(context.Background(), "camp_1"); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if gotAuth != "Bearer wmbly_padded" {
			t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer wmbly_padded")
		}
	})
}

func TestOptionWithAccessToken(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if _, err := New(WithAccessToken("   ")); err == nil {
			t.Fatal("expected error for empty access token")
		}
	})
	t.Run("token carried as bearer", func(t *testing.T) {
		var gotAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"id":"camp_1","name":"x"}`))
		}))
		t.Cleanup(srv.Close)

		c, err := New(WithAccessToken(" wmblyo_abc "), WithBaseURL(srv.URL+"/v1/"))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if _, _, err := c.Campaigns.Get(context.Background(), "camp_1"); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if gotAuth != "Bearer wmblyo_abc" {
			t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer wmblyo_abc")
		}
	})
}

func TestOptionWithTokenSource(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if _, err := New(WithTokenSource(nil)); err == nil {
			t.Fatal("expected error for nil token source")
		}
	})
	t.Run("source used on requests", func(t *testing.T) {
		var gotAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"id":"camp_1","name":"x"}`))
		}))
		t.Cleanup(srv.Close)

		src := StaticTokenSource(&Token{AccessToken: "tok_xyz"})
		c, err := New(WithTokenSource(src), WithBaseURL(srv.URL+"/v1/"))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if _, _, err := c.Campaigns.Get(context.Background(), "camp_1"); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if gotAuth != "Bearer tok_xyz" {
			t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer tok_xyz")
		}
	})
}

func TestOptionWithAuthenticator(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if _, err := New(WithAuthenticator(nil)); err == nil {
			t.Fatal("expected error for nil authenticator")
		}
	})
	t.Run("custom authenticator applied", func(t *testing.T) {
		var gotHeader string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotHeader = r.Header.Get("X-Custom-Auth")
			_, _ = w.Write([]byte(`{"id":"camp_1","name":"x"}`))
		}))
		t.Cleanup(srv.Close)

		auth := AuthenticatorFunc(func(req *http.Request) error {
			req.Header.Set("X-Custom-Auth", "custom-value")
			return nil
		})
		c, err := New(WithAuthenticator(auth), WithBaseURL(srv.URL+"/v1/"))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if _, _, err := c.Campaigns.Get(context.Background(), "camp_1"); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if gotHeader != "custom-value" {
			t.Errorf("X-Custom-Auth = %q, want %q", gotHeader, "custom-value")
		}
	})
}

func TestOptionWithBaseURL(t *testing.T) {
	t.Run("invalid url", func(t *testing.T) {
		if _, err := New(WithAPIKey("wmbly_x"), WithBaseURL("http://%zz")); err == nil {
			t.Fatal("expected error for invalid base URL")
		}
	})
	t.Run("non-absolute url", func(t *testing.T) {
		if _, err := New(WithAPIKey("wmbly_x"), WithBaseURL("foo")); err == nil {
			t.Fatal("expected error for non-absolute base URL")
		}
	})
	t.Run("scheme only, no host", func(t *testing.T) {
		if _, err := New(WithAPIKey("wmbly_x"), WithBaseURL("http://")); err == nil {
			t.Fatal("expected error for base URL with no host")
		}
	})
	t.Run("trailing slash appended", func(t *testing.T) {
		c, err := New(WithAPIKey("wmbly_x"), WithBaseURL("https://api.example.com/v2"))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if got := c.BaseURL().Path; got != "/v2/" {
			t.Errorf("BaseURL().Path = %q, want %q", got, "/v2/")
		}
	})
	t.Run("existing trailing slash preserved", func(t *testing.T) {
		c, err := New(WithAPIKey("wmbly_x"), WithBaseURL("https://api.example.com/v2/"))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if got := c.BaseURL().Path; got != "/v2/" {
			t.Errorf("BaseURL().Path = %q, want %q", got, "/v2/")
		}
	})
}

func TestOptionWithHTTPClient(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if _, err := New(WithAPIKey("wmbly_x"), WithHTTPClient(nil)); err == nil {
			t.Fatal("expected error for nil HTTP client")
		}
	})
	t.Run("custom client used", func(t *testing.T) {
		hc := &http.Client{Timeout: 7 * time.Second}
		c, err := New(WithAPIKey("wmbly_x"), WithHTTPClient(hc))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if c.httpClient != hc {
			t.Errorf("httpClient = %p, want %p", c.httpClient, hc)
		}
	})
}

func TestOptionWithUserAgent(t *testing.T) {
	t.Run("empty leaves default", func(t *testing.T) {
		var gotUA string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotUA = r.Header.Get("User-Agent")
			_, _ = w.Write([]byte(`{"id":"camp_1","name":"x"}`))
		}))
		t.Cleanup(srv.Close)

		c, err := New(WithAPIKey("wmbly_x"), WithBaseURL(srv.URL+"/v1/"), WithUserAgent("  "))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if _, _, err := c.Campaigns.Get(context.Background(), "camp_1"); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if gotUA != defaultUserAgent {
			t.Errorf("User-Agent = %q, want default %q", gotUA, defaultUserAgent)
		}
	})
	t.Run("product token prepended", func(t *testing.T) {
		var gotUA string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotUA = r.Header.Get("User-Agent")
			_, _ = w.Write([]byte(`{"id":"camp_1","name":"x"}`))
		}))
		t.Cleanup(srv.Close)

		c, err := New(WithAPIKey("wmbly_x"), WithBaseURL(srv.URL+"/v1/"), WithUserAgent("myapp/2.0"))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if _, _, err := c.Campaigns.Get(context.Background(), "camp_1"); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !strings.HasPrefix(gotUA, "myapp/2.0 ") {
			t.Errorf("User-Agent = %q, want it to start with product token", gotUA)
		}
		if !strings.Contains(gotUA, "warmbly-go/") {
			t.Errorf("User-Agent = %q, want it to contain warmbly-go/", gotUA)
		}
	})
}

func TestOptionWithMaxRetries(t *testing.T) {
	t.Run("negative", func(t *testing.T) {
		if _, err := New(WithAPIKey("wmbly_x"), WithMaxRetries(-1)); err == nil {
			t.Fatal("expected error for negative max retries")
		}
	})
	t.Run("zero ok", func(t *testing.T) {
		c, err := New(WithAPIKey("wmbly_x"), WithMaxRetries(0))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if c.maxRetries != 0 {
			t.Errorf("maxRetries = %d, want 0", c.maxRetries)
		}
	})
}

func TestOptionWithRetryWaitBounds(t *testing.T) {
	cases := []struct {
		name             string
		minWait, maxWait time.Duration
		wantErr          bool
	}{
		{"min zero", 0, time.Second, true},
		{"min negative", -time.Second, time.Second, true},
		{"max zero", time.Second, 0, true},
		{"max negative", time.Second, -time.Second, true},
		{"min greater than max", 2 * time.Second, time.Second, true},
		{"valid", time.Millisecond, 5 * time.Millisecond, false},
		{"valid equal bounds", time.Second, time.Second, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := New(WithAPIKey("wmbly_x"), WithRetryWaitBounds(tc.minWait, tc.maxWait))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if c.retryWaitMin != tc.minWait || c.retryWaitMax != tc.maxWait {
				t.Errorf("bounds = (%v, %v), want (%v, %v)", c.retryWaitMin, c.retryWaitMax, tc.minWait, tc.maxWait)
			}
		})
	}
}

func TestOptionWithHeader(t *testing.T) {
	t.Run("empty key", func(t *testing.T) {
		if _, err := New(WithAPIKey("wmbly_x"), WithHeader("", "v")); err == nil {
			t.Fatal("expected error for empty header key")
		}
	})
	t.Run("header sent on requests", func(t *testing.T) {
		var gotA, gotB string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotA = r.Header.Get("X-Header-One")
			gotB = r.Header.Get("X-Header-Two")
			_, _ = w.Write([]byte(`{"id":"camp_1","name":"x"}`))
		}))
		t.Cleanup(srv.Close)

		c, err := New(
			WithAPIKey("wmbly_x"),
			WithBaseURL(srv.URL+"/v1/"),
			WithHeader("X-Header-One", "one"),
			WithHeader("X-Header-Two", "two"),
		)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if _, _, err := c.Campaigns.Get(context.Background(), "camp_1"); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if gotA != "one" {
			t.Errorf("X-Header-One = %q, want %q", gotA, "one")
		}
		if gotB != "two" {
			t.Errorf("X-Header-Two = %q, want %q", gotB, "two")
		}
	})
}

func TestOptionNewSkipsNilOption(t *testing.T) {
	c, err := New(nil, WithAPIKey("wmbly_x"))
	if err != nil {
		t.Fatalf("New with leading nil option: %v", err)
	}
	if c == nil {
		t.Fatal("New returned nil client")
	}
}
