package warmbly

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Version is the SDK version, reported in the default User-Agent.
const Version = "0.2.0"

const (
	defaultBaseURL   = "https://api.warmbly.com/v1/"
	defaultUserAgent = "warmbly-go/" + Version
	defaultTimeout   = 30 * time.Second

	// idempotencyKeyHeader is the opt-in retry-safety header accepted on every
	// mutating request.
	idempotencyKeyHeader = "Idempotency-Key"
	// idempotentReplayedHeader is set when the server replayed a stored
	// response instead of performing the operation again.
	idempotentReplayedHeader = "X-Idempotent-Replayed"
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

	// Emails manages connected mailboxes, their warmup and one-off sends.
	Emails *EmailService
	// Campaigns manages outreach campaigns, their steps and A/B variants.
	Campaigns *CampaignService
	// Contacts manages contacts, their CRM notes and import and export.
	Contacts *ContactService
	// Unibox is the unified inbox: reading, replying and composing.
	Unibox *UniboxService
	// Templates manages reusable reply templates.
	Templates *TemplateService
	// Analytics reads aggregate analytics and deliverability health.
	Analytics *AnalyticsService
	// Advisor reads and acts on continuous checks of the sending posture.
	Advisor *AdvisorService

	// CRM manages pipelines, deals and the task board.
	CRM *CRMService
	// Teams groups members for CRM assignment.
	Teams *TeamService
	// Meetings lists calls booked through a connected scheduler.
	Meetings *MeetingService

	// Integrations manages third-party connections.
	Integrations *IntegrationService
	// Automations manages event-triggered flows across those connections.
	Automations *AutomationService
	// LeadSync manages Google Sheets to contacts sync.
	LeadSync *LeadSyncService

	// Generation writes and rewrites copy with AI.
	Generation *GenerationService
	// Skills manages the workspace AI playbooks that steer it.
	Skills *SkillService
	// Assistant drives the AI assistant and its connected MCP servers.
	Assistant *AssistantService

	// Webhooks manages webhook endpoints and their delivery log.
	Webhooks *WebhookService
	// APIKeys manages API keys.
	APIKeys *APIKeyService
	// OAuthApps registers and manages OAuth 2.1 applications.
	OAuthApps *OAuthAppService

	// Outreach reads and writes the organization-wide sending policy.
	Outreach *OutreachService
	// Deliverability ingests bounce and complaint events from upstream.
	Deliverability *DeliverabilityService
	// WarmupRouting manages warmup partner-selection rules.
	WarmupRouting *WarmupRoutingService
	// Tasks inspects and replays the send-task dead-letter queue.
	Tasks *TaskService
	// AuditLogs reads the organization audit trail.
	AuditLogs *AuditLogService

	// Folders group campaigns, Tags group mailboxes, and Categories group
	// contacts and double as unified-inbox labels.
	Folders    *GroupService
	Tags       *GroupService
	Categories *GroupService

	// Meta reads the caller's identity, the plan catalog and the timezone list.
	Meta *MetaService

	// Auth signs a user in and manages the resulting session. Its routes, and
	// those of Organization and Billing, are session-only: they need a token
	// from [AuthService.Login] rather than an API key.
	Auth *AuthService
	// Organization manages the workspace, its members and its roles.
	Organization *OrganizationService
	// Billing manages the subscription, AI credits and referrals.
	Billing *BillingService
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
	c.Emails = (*EmailService)(&c.common)
	c.Campaigns = (*CampaignService)(&c.common)
	c.Contacts = (*ContactService)(&c.common)
	c.Unibox = (*UniboxService)(&c.common)
	c.Templates = (*TemplateService)(&c.common)
	c.Analytics = (*AnalyticsService)(&c.common)
	c.Advisor = (*AdvisorService)(&c.common)

	c.CRM = (*CRMService)(&c.common)
	c.Teams = (*TeamService)(&c.common)
	c.Meetings = (*MeetingService)(&c.common)

	c.Integrations = (*IntegrationService)(&c.common)
	c.Automations = (*AutomationService)(&c.common)
	c.LeadSync = (*LeadSyncService)(&c.common)

	c.Generation = (*GenerationService)(&c.common)
	c.Skills = (*SkillService)(&c.common)
	c.Assistant = (*AssistantService)(&c.common)

	c.Webhooks = (*WebhookService)(&c.common)
	c.APIKeys = (*APIKeyService)(&c.common)
	c.OAuthApps = (*OAuthAppService)(&c.common)

	c.Outreach = (*OutreachService)(&c.common)
	c.Deliverability = (*DeliverabilityService)(&c.common)
	c.WarmupRouting = (*WarmupRoutingService)(&c.common)
	c.Tasks = (*TaskService)(&c.common)
	c.AuditLogs = (*AuditLogService)(&c.common)

	c.Folders = &GroupService{client: c, name: "folders"}
	c.Tags = &GroupService{client: c, name: "tags"}
	c.Categories = &GroupService{client: c, name: "categories"}

	c.Meta = (*MetaService)(&c.common)

	c.Auth = (*AuthService)(&c.common)
	c.Organization = (*OrganizationService)(&c.common)
	c.Billing = (*BillingService)(&c.common)

	return c, nil
}

// Do issues a request against an arbitrary API path, decoding a successful JSON
// body into out (which may be nil). It is the escape hatch for endpoints this
// SDK release does not model yet.
//
// The path is resolved relative to the base URL, so pass it without the version
// prefix — "campaigns/123/steps", not "/v1/campaigns/123/steps". Retries,
// authentication, rate-limit parsing and typed errors work exactly as they do
// for the typed methods.
func (c *Client) Do(ctx context.Context, method, path string, body, out any, opts ...RequestOption) (*Response, error) {
	return c.call(ctx, method, path, body, out, opts)
}

// BaseURL returns the configured API base URL.
func (c *Client) BaseURL() *url.URL { return c.baseURL }

// RequestOption customizes a single API call. Every service method accepts a
// variadic list of them.
type RequestOption func(*requestConfig)

// requestConfig accumulates the per-call overrides applied by [RequestOption].
type requestConfig struct {
	header http.Header
	query  url.Values
}

func newRequestConfig(opts []RequestOption) *requestConfig {
	cfg := &requestConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	return cfg
}

// WithIdempotencyKey attaches an Idempotency-Key to a mutating request. Retrying
// the same key with the same method, path and body replays the original
// response instead of performing the operation twice. The key must be 1-255
// visible ASCII characters; a UUID is a good choice.
//
//	_, _, err := client.Emails.Send(ctx, id, params, warmbly.WithIdempotencyKey(key))
func WithIdempotencyKey(key string) RequestOption {
	return WithRequestHeader(idempotencyKeyHeader, key)
}

// WithRequestHeader sets a header on a single request, overriding any client
// default of the same name.
func WithRequestHeader(key, value string) RequestOption {
	return func(cfg *requestConfig) {
		if key == "" {
			return
		}
		if cfg.header == nil {
			cfg.header = make(http.Header)
		}
		cfg.header.Set(key, value)
	}
}

// WithQueryParam sets a query parameter on a single request. Use it to reach
// filters newer than this SDK release without waiting for a typed field.
func WithQueryParam(key, value string) RequestOption {
	return func(cfg *requestConfig) {
		if key == "" {
			return
		}
		if cfg.query == nil {
			cfg.query = make(url.Values)
		}
		cfg.query.Set(key, value)
	}
}

// Response wraps the underlying [*http.Response] with parsed Warmbly metadata.
// The body has already been consumed and closed by the time a Response is
// returned.
type Response struct {
	*http.Response

	// RateLimit holds the parsed X-RateLimit-* headers, when present.
	RateLimit RateLimit
	// RequestID is the server-assigned request identifier (X-Request-ID).
	RequestID string
	// APIVersion is the API surface that served the request (for example
	// "v1"), taken from the API-Version header.
	APIVersion string
	// Deprecation is the raw Deprecation header, set when the endpoint is on
	// its way out.
	Deprecation string
	// Sunset is the raw Sunset header: the date after which a deprecated
	// endpoint stops responding.
	Sunset string
	// Warning is the raw Warning header carrying migration advice.
	Warning string
	// IdempotentReplayed reports whether the server replayed a stored response
	// for a repeated Idempotency-Key rather than acting again.
	IdempotentReplayed bool
}

// Deprecated reports whether the server marked this endpoint as deprecated.
func (r *Response) Deprecated() bool { return r != nil && r.Deprecation != "" }

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
func (c *Client) newRequest(ctx context.Context, method, refPath string, body any, cfg *requestConfig) (*http.Request, error) {
	rel, err := url.Parse(strings.TrimPrefix(refPath, "/"))
	if err != nil {
		return nil, fmt.Errorf("warmbly: invalid request path %q: %w", refPath, err)
	}
	if cfg != nil && len(cfg.query) > 0 {
		q := rel.Query()
		for k, vs := range cfg.query {
			q[k] = vs
		}
		rel.RawQuery = q.Encode()
	}
	u := c.baseURL.ResolveReference(rel)

	var buf *bytes.Buffer
	if !isNilBody(body) {
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
	if cfg != nil {
		for k, vs := range cfg.header {
			req.Header[http.CanonicalHeaderKey(k)] = append([]string(nil), vs...)
		}
	}

	return req, nil
}

// do executes a request with retries and decodes a successful JSON body into
// out (which may be nil). On a non-2xx response it returns a [*Error].
func (c *Client) do(req *http.Request, out any) (*Response, error) {
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
}

// waitRetry sleeps before the next attempt, honoring Retry-After when present
// and otherwise using exponential backoff with full jitter. It returns the
// context error if the context is canceled while waiting.
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
	resp := &Response{
		Response:           r,
		RequestID:          r.Header.Get("X-Request-ID"),
		APIVersion:         r.Header.Get("API-Version"),
		Deprecation:        r.Header.Get("Deprecation"),
		Sunset:             r.Header.Get("Sunset"),
		Warning:            r.Header.Get("Warning"),
		IdempotentReplayed: strings.EqualFold(r.Header.Get(idempotentReplayedHeader), "true"),
	}
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
			// Round up so a sub-second wait is not truncated to 0.
			apiErr.RetryAfter = int((rl.RetryAfter + time.Second - 1) / time.Second)
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

// isNilBody reports whether body should produce a bodyless request. A typed nil
// pointer (the common case of passing an unset *FooParams) counts as nil, so it
// sends no body rather than the JSON literal null.
func isNilBody(body any) bool {
	if body == nil {
		return true
	}
	v := reflect.ValueOf(body)
	switch v.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Interface:
		return v.IsNil()
	default:
		return false
	}
}

// drain consumes the remaining body to EOF and closes it so the underlying
// connection can be returned to the pool and reused.
func drain(r *http.Response) {
	_, _ = io.Copy(io.Discard, r.Body)
	_ = r.Body.Close()
}

// --- convenience verbs used by the resource services ---

func (c *Client) call(ctx context.Context, method, path string, body, out any, opts []RequestOption) (*Response, error) {
	req, err := c.newRequest(ctx, method, path, body, newRequestConfig(opts))
	if err != nil {
		return nil, err
	}
	return c.do(req, out)
}

func (c *Client) get(ctx context.Context, path string, out any, opts ...RequestOption) (*Response, error) {
	return c.call(ctx, http.MethodGet, path, nil, out, opts)
}

func (c *Client) post(ctx context.Context, path string, body, out any, opts ...RequestOption) (*Response, error) {
	return c.call(ctx, http.MethodPost, path, body, out, opts)
}

func (c *Client) patch(ctx context.Context, path string, body, out any, opts ...RequestOption) (*Response, error) {
	return c.call(ctx, http.MethodPatch, path, body, out, opts)
}

func (c *Client) put(ctx context.Context, path string, body, out any, opts ...RequestOption) (*Response, error) {
	return c.call(ctx, http.MethodPut, path, body, out, opts)
}

func (c *Client) delete(ctx context.Context, path string, opts ...RequestOption) (*Response, error) {
	return c.call(ctx, http.MethodDelete, path, nil, nil, opts)
}

// deleteBody issues a DELETE carrying a JSON body, which a few bulk endpoints
// (for example contact bulk delete) require.
func (c *Client) deleteBody(ctx context.Context, path string, body, out any, opts ...RequestOption) (*Response, error) {
	return c.call(ctx, http.MethodDelete, path, body, out, opts)
}

// stream POSTs a request and hands each Server-Sent Event's data payload to fn
// as it arrives. It returns when the stream ends, fn returns false, or the
// context is canceled.
//
// Streaming requests are never retried: a partially consumed stream cannot be
// replayed, and the caller has already seen its early events.
func (c *Client) stream(ctx context.Context, path string, body any, fn func(data []byte) bool, opts ...RequestOption) (*Response, error) {
	cfg := newRequestConfig(opts)
	req, err := c.newRequest(ctx, http.MethodPost, path, body, cfg)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	if err := c.auth.authenticate(req); err != nil {
		return nil, fmt.Errorf("warmbly: authenticate request: %w", err)
	}

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		if cerr := req.Context().Err(); cerr != nil {
			return nil, cerr
		}
		return nil, fmt.Errorf("warmbly: request failed: %w", err)
	}
	resp := newResponse(httpResp)
	if apiErr := decodeError(httpResp); apiErr != nil {
		drain(httpResp)
		return resp, apiErr
	}
	defer drain(httpResp)

	// Each event is a run of "field: value" lines ending at a blank line. Only
	// the data field carries a payload here.
	scanner := bufio.NewScanner(httpResp.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	var data []byte
	for scanner.Scan() {
		line := scanner.Bytes()
		switch {
		case len(line) == 0:
			if len(data) > 0 {
				if !fn(data) {
					return resp, nil
				}
				data = data[:0]
			}
		case bytes.HasPrefix(line, []byte("data:")):
			chunk := bytes.TrimPrefix(line, []byte("data:"))
			data = append(data, bytes.TrimPrefix(chunk, []byte(" "))...)
		}
	}
	// A stream that ends without its final blank line still has one event.
	if len(data) > 0 {
		fn(data)
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		return resp, fmt.Errorf("warmbly: read event stream: %w", err)
	}
	return resp, ctx.Err()
}

// FileUpload is a file being sent to a multipart endpoint such as campaign
// attachments or the organization avatar.
type FileUpload struct {
	// Filename is the name recorded server-side. It is required.
	Filename string
	// Content is read to EOF and buffered so the request stays retryable.
	Content io.Reader
	// ContentType overrides the part's Content-Type. When empty the server
	// sniffs the file instead.
	ContentType string
}

// postMultipart uploads a file (plus any extra form fields) as
// multipart/form-data. The whole body is buffered so retries can replay it.
func (c *Client) postMultipart(ctx context.Context, path, fieldName string, file *FileUpload, fields map[string]string, out any, opts ...RequestOption) (*Response, error) {
	if file == nil || file.Content == nil {
		return nil, errors.New("warmbly: a file is required for this upload")
	}
	if file.Filename == "" {
		return nil, errors.New("warmbly: upload filename must not be empty")
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			return nil, fmt.Errorf("warmbly: build upload form: %w", err)
		}
	}
	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, fieldName, file.Filename))
	if file.ContentType != "" {
		hdr.Set("Content-Type", file.ContentType)
	}
	part, err := mw.CreatePart(hdr)
	if err != nil {
		return nil, fmt.Errorf("warmbly: build upload form: %w", err)
	}
	if _, err := io.Copy(part, file.Content); err != nil {
		return nil, fmt.Errorf("warmbly: read upload content: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("warmbly: build upload form: %w", err)
	}

	req, err := c.newRequest(ctx, http.MethodPost, path, nil, newRequestConfig(opts))
	if err != nil {
		return nil, err
	}
	payload := buf.Bytes()
	req.Body = io.NopCloser(bytes.NewReader(payload))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(payload)), nil
	}
	req.ContentLength = int64(len(payload))
	req.Header.Set("Content-Type", mw.FormDataContentType())

	return c.do(req, out)
}
