package warmbly

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Version is the SDK version, reported in the default User-Agent.
const Version = "0.1.0"

const (
	defaultBaseURL   = "https://api.warmbly.com/v1/"
	defaultUserAgent = "warmbly-go/" + Version
	defaultTimeout   = 30 * time.Second
)

// Client is a Warmbly API client. Create one with [New]. A Client is safe for
// concurrent use by multiple goroutines.
//
// Resource groups are exposed as services hanging off the client, for example
// client.Campaigns and client.APIKeys.
type Client struct {
	httpClient     *http.Client
	baseURL        *url.URL
	auth           Authenticator
	userAgent      string
	defaultHeaders http.Header
	maxRetries     int
	retryWaitMin   time.Duration
	retryWaitMax   time.Duration

	common service // reused for all services to avoid allocating per-service

	// APIKeys manages API keys (create, list, rotate, revoke).
	APIKeys *APIKeyService
	// OAuthApps manages OAuth 2.1 applications (client registration).
	OAuthApps *OAuthAppService
	// Emails manages connected email accounts and their warmup.
	Emails *EmailService
	// Campaigns manages outreach campaigns and their steps.
	Campaigns *CampaignService
	// Contacts manages contacts (leads).
	Contacts *ContactService
	// Webhooks manages webhook endpoints and event subscriptions.
	Webhooks *WebhookService
	// Templates manages reusable message templates.
	Templates *TemplateService
	// Analytics reads aggregate analytics.
	Analytics *AnalyticsService
	// Organization manages the current organization and its members.
	Organization *OrganizationService
}

// service is embedded (by conversion) into every resource service so they all
// share a single back-reference to the client.
type service struct {
	client *Client
}

// New creates a Client. Exactly one credential option is required: [WithAPIKey],
// [WithAccessToken], [WithTokenSource] or [WithAuthenticator].
func New(opts ...Option) (*Client, error) {
	base, err := url.Parse(defaultBaseURL)
	if err != nil {
		return nil, fmt.Errorf("warmbly: invalid default base URL: %w", err)
	}

	c := &Client{
		httpClient:     &http.Client{Timeout: defaultTimeout},
		baseURL:        base,
		userAgent:      defaultUserAgent,
		defaultHeaders: make(http.Header),
		maxRetries:     2,
		retryWaitMin:   500 * time.Millisecond,
		retryWaitMax:   30 * time.Second,
	}

	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(c); err != nil {
			return nil, err
		}
	}

	if c.auth == nil {
		return nil, errors.New("warmbly: no credentials configured; use WithAPIKey, WithAccessToken, WithTokenSource or WithAuthenticator")
	}

	c.common.client = c
	c.APIKeys = (*APIKeyService)(&c.common)
	c.OAuthApps = (*OAuthAppService)(&c.common)
	c.Emails = (*EmailService)(&c.common)
	c.Campaigns = (*CampaignService)(&c.common)
	c.Contacts = (*ContactService)(&c.common)
	c.Webhooks = (*WebhookService)(&c.common)
	c.Templates = (*TemplateService)(&c.common)
	c.Analytics = (*AnalyticsService)(&c.common)
	c.Organization = (*OrganizationService)(&c.common)

	return c, nil
}

// BaseURL returns the configured API base URL.
func (c *Client) BaseURL() *url.URL { return c.baseURL }

// Response wraps the underlying [*http.Response] with parsed Warmbly metadata.
// The body has already been consumed and closed by the time a Response is
// returned.
type Response struct {
	*http.Response

	// RateLimit holds the parsed X-RateLimit-* headers, when present.
	RateLimit RateLimit
	// RequestID is the server-assigned request identifier (X-Request-ID).
	RequestID string
}

// RateLimit is the per-key rate-limit state reported on each response.
type RateLimit struct {
	// Limit is the ceiling of requests permitted in the current window.
	Limit int
	// Remaining is the number of requests left in the current window.
	Remaining int
	// Policy is the raw policy string, e.g. "60;w=60".
	Policy string
	// RetryAfter is how long to wait before retrying, when provided.
	RetryAfter time.Duration
}

// newRequest builds a request relative to the base URL. The body, if non-nil,
// is JSON-encoded and buffered so the request can be safely retried.
func (c *Client) newRequest(ctx context.Context, method, refPath string, body any) (*http.Request, error) {
	rel, err := url.Parse(strings.TrimPrefix(refPath, "/"))
	if err != nil {
		return nil, fmt.Errorf("warmbly: invalid request path %q: %w", refPath, err)
	}
	u := c.baseURL.ResolveReference(rel)

	var buf *bytes.Buffer
	if body != nil {
		buf = &bytes.Buffer{}
		enc := json.NewEncoder(buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(body); err != nil {
			return nil, fmt.Errorf("warmbly: encode request body: %w", err)
		}
	}

	var reqBody io.Reader
	if buf != nil {
		reqBody = buf
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), reqBody)
	if err != nil {
		return nil, err
	}

	// Enable safe retries by letting net/http rebuild the body.
	if buf != nil {
		payload := buf.Bytes()
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(payload)), nil
		}
		req.ContentLength = int64(len(payload))
		req.Header.Set("Content-Type", "application/json")
	}

	for k, vs := range c.defaultHeaders {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)

	return req, nil
}

// do executes a request with retries and decodes a successful JSON body into
// out (which may be nil). On a non-2xx response it returns a [*Error].
func (c *Client) do(req *http.Request, out any) (*Response, error) {
	var lastErr error

	for attempt := 0; ; attempt++ {
		// Rewind the body for retries.
		if attempt > 0 && req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, fmt.Errorf("warmbly: rewind request body: %w", err)
			}
			req.Body = body
		}

		// Apply (possibly refreshed) credentials on every attempt.
		if err := c.auth.authenticate(req); err != nil {
			return nil, fmt.Errorf("warmbly: authenticate request: %w", err)
		}

		httpResp, err := c.httpClient.Do(req)
		if err != nil {
			// Context cancellation is terminal; never retry it.
			if cerr := req.Context().Err(); cerr != nil {
				return nil, cerr
			}
			lastErr = err
			if attempt < c.maxRetries {
				if werr := c.waitRetry(req.Context(), attempt, nil); werr != nil {
					return nil, werr
				}
				continue
			}
			return nil, fmt.Errorf("warmbly: request failed: %w", err)
		}

		resp := newResponse(httpResp)
		apiErr := decodeError(httpResp)

		if apiErr != nil && apiErr.Temporary() && attempt < c.maxRetries {
			drain(httpResp)
			if werr := c.waitRetry(req.Context(), attempt, resp); werr != nil {
				return resp, werr
			}
			lastErr = apiErr
			continue
		}

		if apiErr != nil {
			drain(httpResp)
			return resp, apiErr
		}

		if err := decodeBody(httpResp, out); err != nil {
			return resp, err
		}
		return resp, nil
	}

	// Unreachable: the loop always returns. Kept for the compiler if edited.
	_ = lastErr
}

// waitRetry sleeps before the next attempt, honouring Retry-After when present
// and otherwise using exponential backoff with full jitter. It returns the
// context error if the context is cancelled while waiting.
func (c *Client) waitRetry(ctx context.Context, attempt int, resp *Response) error {
	wait := c.backoff(attempt)
	if resp != nil && resp.RateLimit.RetryAfter > 0 {
		wait = resp.RateLimit.RetryAfter
		if wait > c.retryWaitMax {
			wait = c.retryWaitMax
		}
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// backoff returns an exponential backoff duration with full jitter, clamped to
// the configured bounds.
func (c *Client) backoff(attempt int) time.Duration {
	d := c.retryWaitMin << attempt
	if d <= 0 || d > c.retryWaitMax {
		d = c.retryWaitMax
	}
	// Full jitter: random in [retryWaitMin, d].
	span := d - c.retryWaitMin
	if span <= 0 {
		return c.retryWaitMin
	}
	return c.retryWaitMin + time.Duration(rand.Int64N(int64(span)+1))
}

func newResponse(r *http.Response) *Response {
	resp := &Response{Response: r, RequestID: r.Header.Get("X-Request-ID")}
	resp.RateLimit = parseRateLimit(r.Header)
	return resp
}

func parseRateLimit(h http.Header) RateLimit {
	var rl RateLimit
	if v := h.Get("X-RateLimit-Limit"); v != "" {
		rl.Limit, _ = strconv.Atoi(v)
	}
	if v := h.Get("X-RateLimit-Remaining"); v != "" {
		rl.Remaining, _ = strconv.Atoi(v)
	}
	rl.Policy = h.Get("X-RateLimit-Policy")
	if v := h.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			rl.RetryAfter = time.Duration(secs) * time.Second
		} else if t, err := http.ParseTime(v); err == nil {
			if d := time.Until(t); d > 0 {
				rl.RetryAfter = d
			}
		}
	}
	return rl
}

// decodeError reads and returns a [*Error] for non-2xx responses, or nil for a
// 2xx response. It does not close the body.
func decodeError(r *http.Response) *Error {
	if r.StatusCode >= 200 && r.StatusCode < 300 {
		return nil
	}
	apiErr := &Error{StatusCode: r.StatusCode, Header: r.Header}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if len(bytes.TrimSpace(body)) > 0 {
		// Best-effort decode of the JSON envelope; fields stay zero on failure.
		_ = json.Unmarshal(body, apiErr)
	}
	if apiErr.RequestID == "" {
		apiErr.RequestID = r.Header.Get("X-Request-ID")
	}
	if apiErr.RetryAfter == 0 {
		if rl := parseRateLimit(r.Header); rl.RetryAfter > 0 {
			apiErr.RetryAfter = int(rl.RetryAfter.Seconds())
		}
	}
	return apiErr
}

// decodeBody decodes a successful response body into out and closes the body.
func decodeBody(r *http.Response, out any) error {
	defer r.Body.Close()
	if out == nil || r.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, r.Body)
		return nil
	}
	if w, ok := out.(io.Writer); ok {
		_, err := io.Copy(w, r.Body)
		return err
	}
	if err := json.NewDecoder(r.Body).Decode(out); err != nil && err != io.EOF {
		return fmt.Errorf("warmbly: decode response body: %w", err)
	}
	return nil
}

// drain consumes and closes a response body so the connection can be reused.
func drain(r *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 4<<10))
	_ = r.Body.Close()
}

// --- convenience verbs used by the resource services ---

func (c *Client) get(ctx context.Context, path string, out any) (*Response, error) {
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	return c.do(req, out)
}

func (c *Client) post(ctx context.Context, path string, body, out any) (*Response, error) {
	req, err := c.newRequest(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, err
	}
	return c.do(req, out)
}

func (c *Client) patch(ctx context.Context, path string, body, out any) (*Response, error) {
	req, err := c.newRequest(ctx, http.MethodPatch, path, body)
	if err != nil {
		return nil, err
	}
	return c.do(req, out)
}

func (c *Client) put(ctx context.Context, path string, body, out any) (*Response, error) {
	req, err := c.newRequest(ctx, http.MethodPut, path, body)
	if err != nil {
		return nil, err
	}
	return c.do(req, out)
}

func (c *Client) delete(ctx context.Context, path string, out any) (*Response, error) {
	req, err := c.newRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}
	return c.do(req, out)
}
