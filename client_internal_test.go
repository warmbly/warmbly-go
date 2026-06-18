package warmbly

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientHTTPBaseURL(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {})
	got := c.BaseURL()
	if got == nil {
		t.Fatal("BaseURL() = nil")
	}
	if got != c.baseURL {
		t.Errorf("BaseURL() = %p, want %p", got, c.baseURL)
	}
	if !strings.HasSuffix(got.Path, "/v1/") {
		t.Errorf("BaseURL().Path = %q, want it to end with /v1/", got.Path)
	}
}

// TestClientHTTPWriteVerbs exercises the put/patch/delete verbs end-to-end via
// the campaign service, asserting the HTTP method and path reach the server.
func TestClientHTTPWriteVerbs(t *testing.T) {
	t.Run("put_set_senders", func(t *testing.T) {
		var gotMethod, gotPath, gotBody string
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotPath = r.URL.Path
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":["acc_1","acc_2"]}`))
		})
		senders, resp, err := c.Campaigns.SetSenders(context.Background(), "camp_1", []string{"acc_1", "acc_2"})
		if err != nil {
			t.Fatalf("SetSenders: %v", err)
		}
		if gotMethod != http.MethodPut {
			t.Errorf("method = %s, want PUT", gotMethod)
		}
		if gotPath != "/v1/campaigns/camp_1/senders" {
			t.Errorf("path = %q", gotPath)
		}
		if !strings.Contains(gotBody, `"acc_1"`) {
			t.Errorf("body = %q, want it to contain acc_1", gotBody)
		}
		if strings.Join(senders, ",") != "acc_1,acc_2" {
			t.Errorf("senders = %v", senders)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d", resp.StatusCode)
		}
	})

	t.Run("patch_update", func(t *testing.T) {
		var gotMethod, gotPath string
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotPath = r.URL.Path
			_, _ = w.Write([]byte(`{"id":"camp_1","name":"Renamed"}`))
		})
		name := "Renamed"
		camp, _, err := c.Campaigns.Update(context.Background(), "camp_1", &CampaignUpdateParams{Name: &name})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if gotMethod != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", gotMethod)
		}
		if gotPath != "/v1/campaigns/camp_1" {
			t.Errorf("path = %q", gotPath)
		}
		if camp.Name != "Renamed" {
			t.Errorf("name = %q", camp.Name)
		}
	})

	t.Run("delete", func(t *testing.T) {
		var gotMethod, gotPath string
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotPath = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		})
		resp, err := c.Campaigns.Delete(context.Background(), "camp_1")
		if err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if gotMethod != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", gotMethod)
		}
		if gotPath != "/v1/campaigns/camp_1" {
			t.Errorf("path = %q", gotPath)
		}
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("status = %d", resp.StatusCode)
		}
	})
}

// TestClientHTTPDecodeBody drives decodeBody directly across its branches.
func TestClientHTTPDecodeBody(t *testing.T) {
	newResp := func(status int, body string) *http.Response {
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader(body)),
		}
	}

	t.Run("into_io_writer_copies_body", func(t *testing.T) {
		var buf bytes.Buffer
		r := newResp(http.StatusOK, "raw-bytes-payload")
		if err := decodeBody(r, &buf); err != nil {
			t.Fatalf("decodeBody: %v", err)
		}
		if buf.String() != "raw-bytes-payload" {
			t.Errorf("buf = %q", buf.String())
		}
	})

	t.Run("nil_out_drains", func(t *testing.T) {
		r := newResp(http.StatusOK, `{"ignored":true}`)
		if err := decodeBody(r, nil); err != nil {
			t.Fatalf("decodeBody: %v", err)
		}
	})

	t.Run("no_content_with_non_nil_out_is_noop", func(t *testing.T) {
		var dst struct {
			Name string `json:"name"`
		}
		r := newResp(http.StatusNoContent, `{"name":"should-not-be-decoded"}`)
		if err := decodeBody(r, &dst); err != nil {
			t.Fatalf("decodeBody: %v", err)
		}
		if dst.Name != "" {
			t.Errorf("dst.Name = %q, want empty (204 should not decode)", dst.Name)
		}
	})

	t.Run("decodes_json_into_struct", func(t *testing.T) {
		var dst struct {
			Name string `json:"name"`
		}
		r := newResp(http.StatusOK, `{"name":"decoded"}`)
		if err := decodeBody(r, &dst); err != nil {
			t.Fatalf("decodeBody: %v", err)
		}
		if dst.Name != "decoded" {
			t.Errorf("dst.Name = %q", dst.Name)
		}
	})

	t.Run("empty_body_is_eof_noop", func(t *testing.T) {
		var dst struct {
			Name string `json:"name"`
		}
		r := newResp(http.StatusOK, "")
		if err := decodeBody(r, &dst); err != nil {
			t.Fatalf("decodeBody on empty body: %v", err)
		}
	})

	t.Run("invalid_json_returns_wrapped_error", func(t *testing.T) {
		var dst struct {
			Name string `json:"name"`
		}
		r := newResp(http.StatusOK, `{not-json`)
		err := decodeBody(r, &dst)
		if err == nil {
			t.Fatal("expected an error decoding invalid JSON")
		}
		if !strings.Contains(err.Error(), "decode response body") {
			t.Errorf("error = %v, want it to mention decode response body", err)
		}
		var syntaxErr interface{ Error() string }
		if !errors.As(err, &syntaxErr) {
			t.Errorf("errors.As failed for %v", err)
		}
	})
}

// TestClientHTTPNewRequestInvalidPath verifies newRequest reports an error for a
// path that cannot be parsed as a URL reference.
func TestClientHTTPNewRequestInvalidPath(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {})
	_, err := c.newRequest(context.Background(), http.MethodGet, "http://%zz", nil)
	if err == nil {
		t.Fatal("expected an error for an invalid request path")
	}
	if !strings.Contains(err.Error(), "invalid request path") {
		t.Errorf("error = %v, want it to mention invalid request path", err)
	}
}

// TestClientHTTPNewRequestEncodeError covers newRequest's JSON-encode failure
// branch using a body value that cannot be marshaled (a channel).
func TestClientHTTPNewRequestEncodeError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {})
	_, err := c.newRequest(context.Background(), http.MethodPost, "campaigns", make(chan int))
	if err == nil {
		t.Fatal("expected an error encoding an unmarshalable body")
	}
	if !strings.Contains(err.Error(), "encode request body") {
		t.Errorf("error = %v, want it to mention encode request body", err)
	}
	var marshalErr *json.UnsupportedTypeError
	if !errors.As(err, &marshalErr) {
		t.Errorf("errors.As(*json.UnsupportedTypeError) failed for %v", err)
	}
}

// TestClientHTTPNewRequestBadMethod covers newRequest's NewRequestWithContext
// failure branch using a method string containing illegal characters.
func TestClientHTTPNewRequestBadMethod(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {})
	_, err := c.newRequest(context.Background(), "BAD METHOD", "campaigns", nil)
	if err == nil {
		t.Fatal("expected an error for an invalid HTTP method")
	}
	if strings.Contains(err.Error(), "invalid request path") || strings.Contains(err.Error(), "encode request body") {
		t.Errorf("error = %v, want the NewRequestWithContext failure", err)
	}
}

// TestClientHTTPParseRateLimit drives parseRateLimit directly across its
// Retry-After handling branches.
func TestClientHTTPParseRateLimit(t *testing.T) {
	t.Run("retry_after_integer_seconds", func(t *testing.T) {
		h := http.Header{}
		h.Set("Retry-After", "7")
		rl := parseRateLimit(h)
		if rl.RetryAfter != 7*time.Second {
			t.Errorf("RetryAfter = %v, want 7s", rl.RetryAfter)
		}
	})

	t.Run("retry_after_http_date_in_future", func(t *testing.T) {
		future := time.Now().Add(2 * time.Hour).UTC().Format(http.TimeFormat)
		h := http.Header{}
		h.Set("Retry-After", future)
		rl := parseRateLimit(h)
		if rl.RetryAfter <= 0 {
			t.Errorf("RetryAfter = %v, want a positive duration", rl.RetryAfter)
		}
		if rl.RetryAfter > 2*time.Hour {
			t.Errorf("RetryAfter = %v, want it at or below 2h", rl.RetryAfter)
		}
	})

	t.Run("retry_after_http_date_in_past_ignored", func(t *testing.T) {
		past := time.Now().Add(-2 * time.Hour).UTC().Format(http.TimeFormat)
		h := http.Header{}
		h.Set("Retry-After", past)
		rl := parseRateLimit(h)
		if rl.RetryAfter != 0 {
			t.Errorf("RetryAfter = %v, want 0 for a past date", rl.RetryAfter)
		}
	})

	t.Run("retry_after_garbage_ignored", func(t *testing.T) {
		h := http.Header{}
		h.Set("Retry-After", "not-a-number-or-date")
		rl := parseRateLimit(h)
		if rl.RetryAfter != 0 {
			t.Errorf("RetryAfter = %v, want 0 for garbage", rl.RetryAfter)
		}
	})

	t.Run("all_fields", func(t *testing.T) {
		h := http.Header{}
		h.Set("X-RateLimit-Limit", "100")
		h.Set("X-RateLimit-Remaining", "42")
		h.Set("X-RateLimit-Policy", "100;w=60")
		rl := parseRateLimit(h)
		if rl.Limit != 100 || rl.Remaining != 42 || rl.Policy != "100;w=60" {
			t.Errorf("unexpected rate limit: %+v", rl)
		}
	})
}

// TestClientHTTPDecodeError drives decodeError directly: a 503 with a Retry-After
// header populates a rounded-up RetryAfter and falls back to the X-Request-ID
// header for the request id.
func TestClientHTTPDecodeError(t *testing.T) {
	t.Run("retry_after_rounded_up_and_request_id_fallback", func(t *testing.T) {
		// An HTTP-date roughly 1.5s in the future should round up to 2 seconds.
		future := time.Now().Add(1500 * time.Millisecond).UTC().Format(http.TimeFormat)
		h := http.Header{}
		h.Set("Retry-After", future)
		h.Set("X-Request-ID", "req_hdr_fallback")
		r := &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Header:     h,
			Body:       io.NopCloser(strings.NewReader("")),
		}
		apiErr := decodeError(r)
		if apiErr == nil {
			t.Fatal("decodeError returned nil for a 503")
		}
		if apiErr.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("StatusCode = %d", apiErr.StatusCode)
		}
		if apiErr.RetryAfter < 1 {
			t.Errorf("RetryAfter = %d, want a positive whole-second value", apiErr.RetryAfter)
		}
		if apiErr.RequestID != "req_hdr_fallback" {
			t.Errorf("RequestID = %q, want fallback from X-Request-ID", apiErr.RequestID)
		}
	})

	t.Run("integer_retry_after_rounded", func(t *testing.T) {
		h := http.Header{}
		h.Set("Retry-After", "3")
		r := &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     h,
			Body:       io.NopCloser(strings.NewReader(`{"error":"rate_limited","message":"slow down"}`)),
		}
		apiErr := decodeError(r)
		if apiErr == nil {
			t.Fatal("decodeError returned nil")
		}
		if apiErr.RetryAfter != 3 {
			t.Errorf("RetryAfter = %d, want 3", apiErr.RetryAfter)
		}
		if apiErr.Type != "rate_limited" || apiErr.Message != "slow down" {
			t.Errorf("decoded envelope = %+v", apiErr)
		}
	})

	t.Run("body_retry_after_not_overridden", func(t *testing.T) {
		// When the JSON body already supplies retry_after, the header is not used.
		h := http.Header{}
		h.Set("Retry-After", "99")
		r := &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     h,
			Body:       io.NopCloser(strings.NewReader(`{"error":"rate_limited","retry_after":5}`)),
		}
		apiErr := decodeError(r)
		if apiErr.RetryAfter != 5 {
			t.Errorf("RetryAfter = %d, want 5 (from body, not header)", apiErr.RetryAfter)
		}
	})

	t.Run("two_xx_returns_nil", func(t *testing.T) {
		r := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader(`{}`)),
		}
		if apiErr := decodeError(r); apiErr != nil {
			t.Errorf("decodeError on 200 = %+v, want nil", apiErr)
		}
	})
}

// TestClientHTTPNetworkErrorRetry exercises the do() err!=nil non-context retry
// branch. The server hijacks and abruptly closes the connection on the first
// request (producing a transport-level error, not an HTTP status), then responds
// normally; do() must retry and ultimately succeed.
func TestClientHTTPNetworkErrorRetry(t *testing.T) {
	var calls int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			// Force a transport error: hijack and close without writing a reply.
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("ResponseWriter is not a Hijacker")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatalf("Hijack: %v", err)
			}
			_ = conn.Close()
			return
		}
		_, _ = w.Write([]byte(`{"id":"camp_1","name":"recovered"}`))
	})
	// Disable connection reuse so the retry dials a fresh, healthy connection.
	c.httpClient.Transport = &http.Transport{DisableKeepAlives: true}

	camp, _, err := c.Campaigns.Get(context.Background(), "camp_1")
	if err != nil {
		t.Fatalf("Get after transport retry: %v", err)
	}
	if camp.Name != "recovered" {
		t.Errorf("name = %q", camp.Name)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("server received %d requests, want 2", got)
	}
}

// TestClientHTTPNetworkErrorExhausted covers do() returning a wrapped request
// failure once retries are exhausted: every dial fails because the target host
// does not resolve.
func TestClientHTTPNetworkErrorExhausted(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {})
	// Point at an unresolvable host so every attempt errors at the transport.
	bad, perr := url.Parse("http://wmbly.invalid./v1/")
	if perr != nil {
		t.Fatalf("url.Parse: %v", perr)
	}
	c.baseURL = bad
	c.maxRetries = 1
	c.httpClient.Transport = &http.Transport{DisableKeepAlives: true}

	_, _, err := c.Campaigns.Get(context.Background(), "camp_1")
	if err == nil {
		t.Fatal("expected an error once retries are exhausted")
	}
	if !strings.Contains(err.Error(), "request failed") {
		t.Errorf("error = %v, want it to mention request failed", err)
	}
}

// TestClientHTTPWaitRetryHonorsRetryAfter sends a 429 with Retry-After: 1 through
// testClient; the configured bounds clamp the actual wait to 5ms so the retry
// succeeds quickly. This drives the resp!=nil RetryAfter branch in waitRetry.
func TestClientHTTPWaitRetryHonorsRetryAfter(t *testing.T) {
	var calls int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate_limited","message":"slow down"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"camp_1","name":"after-429"}`))
	})

	start := time.Now()
	camp, resp, err := c.Campaigns.Get(context.Background(), "camp_1")
	if err != nil {
		t.Fatalf("Get after 429: %v", err)
	}
	if camp.Name != "after-429" {
		t.Errorf("name = %q", camp.Name)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("server called %d times, want 2", got)
	}
	// The 1s Retry-After must be clamped well below a second by retryWaitMax=5ms.
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("waited %v, want the Retry-After clamped to the 5ms bound", elapsed)
	}
}

// TestClientHTTPWaitRetryContextCancel covers waitRetry returning the context
// error when the context is canceled mid-wait.
func TestClientHTTPWaitRetryContextCancel(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := c.waitRetry(ctx, 0, nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("waitRetry = %v, want context.Canceled", err)
	}
}

// TestClientHTTPWaitRetryClampsHugeRetryAfter ensures an oversized Retry-After is
// clamped to retryWaitMax inside waitRetry.
func TestClientHTTPWaitRetryClampsHugeRetryAfter(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {})
	resp := &Response{RateLimit: RateLimit{RetryAfter: time.Hour}}
	start := time.Now()
	if err := c.waitRetry(context.Background(), 0, resp); err != nil {
		t.Fatalf("waitRetry: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("waited %v, want it clamped to retryWaitMax (5ms)", elapsed)
	}
}

// TestClientHTTPBackoff drives backoff across its bounds: a zero span returns the
// minimum, and large attempts clamp to retryWaitMax.
func TestClientHTTPBackoff(t *testing.T) {
	t.Run("equal_bounds_returns_min", func(t *testing.T) {
		c, err := New(
			WithAPIKey("wmbly_test"),
			WithRetryWaitBounds(2*time.Millisecond, 2*time.Millisecond),
		)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if d := c.backoff(0); d != 2*time.Millisecond {
			t.Errorf("backoff = %v, want 2ms (zero span)", d)
		}
	})

	t.Run("large_attempt_clamps_to_max", func(t *testing.T) {
		c, err := New(
			WithAPIKey("wmbly_test"),
			WithRetryWaitBounds(time.Millisecond, 4*time.Millisecond),
		)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		// A large shift overflows/exceeds max, exercising the clamp branch.
		for attempt := 0; attempt < 70; attempt++ {
			d := c.backoff(attempt)
			if d < time.Millisecond || d > 4*time.Millisecond {
				t.Fatalf("backoff(%d) = %v, out of [1ms,4ms]", attempt, d)
			}
		}
	})
}

// TestClientHTTPNewOptionHandling covers New's nil-option skip and an option
// that returns an error.
func TestClientHTTPNewOptionHandling(t *testing.T) {
	t.Run("nil_option_skipped", func(t *testing.T) {
		c, err := New(nil, WithAPIKey("wmbly_test"), nil)
		if err != nil {
			t.Fatalf("New with nil options: %v", err)
		}
		if c == nil {
			t.Fatal("client is nil")
		}
	})

	t.Run("option_error_propagates", func(t *testing.T) {
		_, err := New(WithAPIKey("wmbly_test"), WithHeader("", "v"))
		if err == nil {
			t.Fatal("expected an error from a failing option")
		}
		if !strings.Contains(err.Error(), "header key must not be empty") {
			t.Errorf("error = %v", err)
		}
	})
}

// TestClientHTTPNewRequestBodyAndHeaders covers newRequest's GetBody closure,
// Content-Type, ContentLength and the default-headers copy loop.
func TestClientHTTPNewRequestBodyAndHeaders(t *testing.T) {
	c, err := New(
		WithAPIKey("wmbly_test"),
		WithBaseURL("https://api.example.test/v1/"),
		WithHeader("X-Custom", "abc"),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req, err := c.newRequest(context.Background(), http.MethodPost, "/campaigns", map[string]string{"name": "x"})
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}
	if req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", req.Header.Get("Content-Type"))
	}
	if req.Header.Get("X-Custom") != "abc" {
		t.Errorf("X-Custom = %q", req.Header.Get("X-Custom"))
	}
	if req.Header.Get("Accept") != "application/json" {
		t.Errorf("Accept = %q", req.Header.Get("Accept"))
	}
	if req.ContentLength <= 0 {
		t.Errorf("ContentLength = %d, want > 0", req.ContentLength)
	}
	if req.GetBody == nil {
		t.Fatal("GetBody is nil; retries would not be able to rewind the body")
	}
	rc, err := req.GetBody()
	if err != nil {
		t.Fatalf("GetBody: %v", err)
	}
	t.Cleanup(func() { _ = rc.Close() })
	got, _ := io.ReadAll(rc)
	if !strings.Contains(string(got), `"name":"x"`) {
		t.Errorf("rewound body = %q", string(got))
	}
}

// TestClientHTTPDoRewindBodyError covers do()'s attempt>0 rewind-error branch:
// the request's GetBody is replaced with one that fails, and the server returns a
// retryable 503 on the first attempt so the rewind is reached on the second.
func TestClientHTTPDoRewindBodyError(t *testing.T) {
	rewindErr := errors.New("cannot rewind")
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"server_error","message":"down"}`))
	})

	req, err := c.newRequest(context.Background(), http.MethodPost, "campaigns", map[string]string{"name": "x"})
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}
	req.GetBody = func() (io.ReadCloser, error) { return nil, rewindErr }

	_, derr := c.do(req, nil)
	if derr == nil {
		t.Fatal("expected a rewind error")
	}
	if !errors.Is(derr, rewindErr) {
		t.Errorf("errors.Is(rewindErr) = false; err = %v", derr)
	}
	if !strings.Contains(derr.Error(), "rewind request body") {
		t.Errorf("error = %v, want it to mention rewind request body", derr)
	}
}

// TestClientHTTPDoRewindsBodyOnRetry exercises do()'s attempt>0 body-rewind path
// by sending a body-bearing POST that the server fails once (503) then accepts,
// asserting both attempts received the identical encoded body.
func TestClientHTTPDoRewindsBodyOnRetry(t *testing.T) {
	var bodies []string
	var calls int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, strings.TrimSpace(string(b)))
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"server_error","message":"retry"}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"camp_new","name":"Created"}`))
	})

	camp, _, err := c.Campaigns.Create(context.Background(), &CampaignCreateParams{Name: "Created", DailyLimit: 50})
	if err != nil {
		t.Fatalf("Create with retry: %v", err)
	}
	if camp.ID != "camp_new" {
		t.Errorf("id = %q", camp.ID)
	}
	if len(bodies) != 2 {
		t.Fatalf("server saw %d bodies, want 2", len(bodies))
	}
	if bodies[0] != bodies[1] {
		t.Errorf("rewound body differs: %q vs %q", bodies[0], bodies[1])
	}
	if !strings.Contains(bodies[1], `"name":"Created"`) {
		t.Errorf("body = %q", bodies[1])
	}
}

// TestClientHTTPDoAuthError covers do() returning a wrapped error when the
// authenticator fails.
func TestClientHTTPDoAuthError(t *testing.T) {
	authErr := errors.New("token refresh failed")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be reached when auth fails")
	}))
	t.Cleanup(srv.Close)
	c, err := New(
		WithBaseURL(srv.URL+"/v1/"),
		WithAuthenticator(AuthenticatorFunc(func(req *http.Request) error { return authErr })),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, _, derr := c.Campaigns.Get(context.Background(), "camp_1")
	if derr == nil {
		t.Fatal("expected an error from a failing authenticator")
	}
	if !errors.Is(derr, authErr) {
		t.Errorf("errors.Is(authErr) = false; err = %v", derr)
	}
	if !strings.Contains(derr.Error(), "authenticate request") {
		t.Errorf("error = %v, want it to mention authenticate request", derr)
	}
}

// TestClientHTTPDoDecodeBodyError covers do() returning the decodeBody error on a
// 2xx response with an undecodable JSON body.
func TestClientHTTPDoDecodeBodyError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"camp_1",`)) // truncated JSON
	})
	_, resp, err := c.Campaigns.Get(context.Background(), "camp_1")
	if err == nil {
		t.Fatal("expected a decode error")
	}
	if !strings.Contains(err.Error(), "decode response body") {
		t.Errorf("error = %v", err)
	}
	if resp == nil {
		t.Error("resp should be non-nil even when decoding fails")
	}
}

// TestClientHTTPDoRetryWaitContextCancel covers do() propagating the context
// error returned by waitRetry on both retry branches (temporary apiErr and a
// transport-level error).
func TestClientHTTPDoRetryWaitContextCancel(t *testing.T) {
	t.Run("temporary_api_error", func(t *testing.T) {
		var calls int32
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&calls, 1)
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"server_error","message":"down"}`))
		})
		// A wide retry-wait window guarantees the wait is still in-flight when the
		// short-lived context expires, deterministically hitting waitRetry's
		// ctx.Done() branch on the second attempt.
		c.retryWaitMin = time.Hour
		c.retryWaitMax = time.Hour
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, _, err := c.Campaigns.Get(ctx, "camp_1")
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("err = %v, want context.DeadlineExceeded", err)
		}
		if atomic.LoadInt32(&calls) < 1 {
			t.Error("server was never reached")
		}
	})

	t.Run("transport_error", func(t *testing.T) {
		var calls int32
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&calls, 1)
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("ResponseWriter is not a Hijacker")
			}
			conn, _, herr := hj.Hijack()
			if herr != nil {
				t.Fatalf("Hijack: %v", herr)
			}
			_ = conn.Close()
		})
		c.httpClient.Transport = &http.Transport{DisableKeepAlives: true}
		c.retryWaitMin = time.Hour
		c.retryWaitMax = time.Hour
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, _, err := c.Campaigns.Get(ctx, "camp_1")
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("err = %v, want context.DeadlineExceeded", err)
		}
		if atomic.LoadInt32(&calls) < 1 {
			t.Error("server was never reached")
		}
	})
}

// TestClientHTTPVerbNewRequestErrors covers the newRequest-error branch of every
// convenience verb (get/post/patch/put/delete) by forcing an invalid path. The
// id is URL-path-escaped by the services, so the invalid character is injected
// via a custom baseURL that ResolveReference cannot combine; instead we call the
// unexported verbs directly with an unparseable refPath.
func TestClientHTTPVerbNewRequestErrors(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be reached when the path is invalid")
	})
	bad := "http://%zz"
	ctx := context.Background()

	if _, err := c.get(ctx, bad, nil); err == nil {
		t.Error("get: expected an error")
	}
	if _, err := c.post(ctx, bad, nil, nil); err == nil {
		t.Error("post: expected an error")
	}
	if _, err := c.patch(ctx, bad, nil, nil); err == nil {
		t.Error("patch: expected an error")
	}
	if _, err := c.put(ctx, bad, nil, nil); err == nil {
		t.Error("put: expected an error")
	}
	if _, err := c.delete(ctx, bad); err == nil {
		t.Error("delete: expected an error")
	}
}
