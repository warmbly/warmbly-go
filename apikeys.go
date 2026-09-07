package warmbly

import (
	"context"
	"net/url"
	"time"
)

// APIKeyService manages the organization's API keys.
//
// API keys authenticate server-to-server requests and are prefixed "wmbly_".
// The secret is shown exactly once, at creation; afterwards only a prefix and
// suffix are retrievable.
type APIKeyService service

// API permission bits. A key's grant is the bitwise OR of the scopes it holds,
// which is what travels in [APIKey.Permissions]:
//
//	perms := warmbly.PermReadCampaigns | warmbly.PermWriteCampaigns
//
// These are distinct from the organization role permissions that gate a human
// session.
const (
	// PermReadEmails grants reading mailboxes and their settings.
	PermReadEmails uint64 = 1 << iota
	// PermReadCampaigns grants reading campaigns and their steps.
	PermReadCampaigns
	// PermReadContacts grants reading contacts, segments, notes and
	// activities.
	PermReadContacts
	// PermReadUnibox grants reading the unified inbox.
	PermReadUnibox
	// PermReadAnalytics grants reading analytics and statistics.
	PermReadAnalytics

	// PermWriteEmails grants modifying mailbox settings.
	PermWriteEmails
	// PermWriteCampaigns grants creating and editing campaigns and steps.
	PermWriteCampaigns
	// PermWriteContacts grants creating and editing contacts, segments, notes
	// and activities.
	PermWriteContacts
	// PermWriteUnibox grants marking messages seen and sending replies.
	PermWriteUnibox

	// PermBulkContacts grants bulk contact import, export and delete. It is
	// separate so a key can read and write without bulk power.
	PermBulkContacts
	// PermBulkCampaigns grants bulk campaign operations.
	PermBulkCampaigns

	// PermRealtimeSubscribe grants subscribing to the realtime gateway.
	PermRealtimeSubscribe
	// PermWebhooks grants managing webhook endpoints.
	PermWebhooks

	// PermAPIKeys grants creating, listing and revoking API keys, so an
	// integration can rotate its own credentials.
	PermAPIKeys

	// PermSendCampaigns grants starting and stopping campaigns. It is separate
	// from PermWriteCampaigns because starting one actually sends mail.
	PermSendCampaigns

	// PermReadTemplates grants reading reply templates.
	PermReadTemplates
	// PermWriteTemplates grants creating and editing reply templates.
	PermWriteTemplates
	// PermReadCRM grants reading pipelines, deals and CRM tasks.
	PermReadCRM
	// PermWriteCRM grants creating and editing pipelines, deals and CRM tasks.
	PermWriteCRM

	// PermReadAuditLogs grants reading the organization audit trail.
	PermReadAuditLogs

	// PermIntegrations grants connecting and managing third-party integrations
	// and automations.
	PermIntegrations
	// PermWarmupRouting grants managing warmup routing rules.
	PermWarmupRouting

	// PermAIAgent grants running the AI assistant and the MCP tool surface.
	PermAIAgent
	// PermAIResearch grants running AI contact research.
	PermAIResearch
)

// Preset permission masks matching the presets the API advertises.
const (
	// PermReadOnly grants every read scope and nothing else.
	PermReadOnly = PermReadEmails | PermReadCampaigns | PermReadContacts |
		PermReadUnibox | PermReadAnalytics | PermReadTemplates |
		PermReadCRM | PermReadAuditLogs

	// PermFullAccess grants every scope this SDK release knows about. A key
	// minted with it will not pick up scopes added later.
	PermFullAccess = PermReadOnly | PermWriteEmails | PermWriteCampaigns |
		PermWriteContacts | PermWriteUnibox | PermBulkContacts |
		PermBulkCampaigns | PermSendCampaigns | PermWriteTemplates |
		PermWriteCRM | PermRealtimeSubscribe | PermWebhooks | PermAPIKeys |
		PermIntegrations | PermWarmupRouting | PermAIAgent | PermAIResearch
)

// API key lifecycle states returned in [APIKey.Status].
const (
	APIKeyStatusActive  = "active"
	APIKeyStatusRevoked = "revoked"
	APIKeyStatusExpired = "expired"
)

// APIKey is an API key as returned by the API. The full secret is never
// included; see [APIKeyWithSecret], returned only by [APIKeyService.Create].
type APIKey struct {
	ID             string `json:"id"`
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	// KeyPrefix and KeySuffix are the non-secret ends of the key, shown so a
	// human can tell two keys apart.
	KeyPrefix string `json:"key_prefix"`
	KeySuffix string `json:"key_suffix"`
	// Permissions is the granted scope bitmask; test it with [APIKey.Can].
	Permissions uint64 `json:"permissions"`
	// AllowedIPs restricts use to these IPs or CIDR ranges. Empty means any.
	AllowedIPs []string `json:"allowed_ips,omitempty"`
	// AllowedEmailAccounts restricts the key to specific mailbox ids. Empty
	// means every mailbox in the organization.
	AllowedEmailAccounts []string `json:"allowed_email_accounts,omitempty"`
	// RateLimitPerMinute is the per-key request ceiling, enforced as a
	// sliding window. Zero means the server default (60).
	RateLimitPerMinute int `json:"rate_limit_per_minute"`
	// Status is [APIKeyStatusActive], [APIKeyStatusRevoked] or
	// [APIKeyStatusExpired].
	Status        string     `json:"status"`
	LastUsedAt    *time.Time `json:"last_used_at"`
	LastRequestIP *string    `json:"last_request_ip"`
	ExpiresAt     *time.Time `json:"expires_at"`
	RevokedAt     *time.Time `json:"revoked_at"`
	RevokedReason *string    `json:"revoked_reason"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// Can reports whether the key holds every bit in perms.
//
//	if key.Can(warmbly.PermSendCampaigns) { ... }
func (k *APIKey) Can(perms uint64) bool { return k.Permissions&perms == perms }

// CanAny reports whether the key holds at least one bit in perms.
func (k *APIKey) CanAny(perms uint64) bool { return k.Permissions&perms != 0 }

// Revoked reports whether the key has been revoked.
func (k *APIKey) Revoked() bool { return k.RevokedAt != nil }

// APIKeyWithSecret is an API key together with its plaintext secret, returned
// only by [APIKeyService.Create].
type APIKeyWithSecret struct {
	APIKey
	// Secret is the full plaintext credential (prefixed "wmbly_"). Capture it
	// now; it cannot be retrieved again.
	Secret string `json:"secret"`
}

// APIKeyCreateParams provisions an API key. Name (at most 255 characters) and
// Permissions are required.
type APIKeyCreateParams struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Permissions is the scope bitmask, for example
	// [PermReadOnly] or PermReadCampaigns|PermSendCampaigns. It must name at
	// least one scope, and only scopes the server knows: a mask carrying an
	// unrecognized bit is refused rather than silently narrowed, so a stale
	// client cannot grant a scope it does not understand.
	Permissions uint64 `json:"permissions"`
	// AllowedIPs restricts the key to these IPs or CIDR ranges (at most
	// [MaxAllowedIPs]); AllowedEmailAccounts to these mailbox ids (at most
	// [MaxAllowedEmailAccounts]). Empty means no restriction.
	AllowedIPs           []string `json:"allowed_ips,omitempty"`
	AllowedEmailAccounts []string `json:"allowed_email_accounts,omitempty"`
	// RateLimitPerMinute caps requests per minute for this key, between
	// [MinRateLimitPerMinute] and [MaxRateLimitPerMinute]; zero keeps the
	// server default of [DefaultRateLimitPerMinute].
	RateLimitPerMinute int        `json:"rate_limit_per_minute,omitempty"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
}

// Bounds the server enforces on [APIKeyCreateParams] and
// [APIKeyUpdateParams].
const (
	// MinRateLimitPerMinute and MaxRateLimitPerMinute bound an explicit
	// per-key request ceiling; DefaultRateLimitPerMinute is what a key gets
	// when it names none.
	MinRateLimitPerMinute     = 1
	MaxRateLimitPerMinute     = 10000
	DefaultRateLimitPerMinute = 60
	// MaxAllowedIPs is the most entries an IP allow-list may hold, and
	// MaxAllowedEmailAccounts the most mailboxes a key may be pinned to.
	MaxAllowedIPs           = 64
	MaxAllowedEmailAccounts = 128
)

// APIKeyUpdateParams updates an API key. Nil fields are left unchanged; an
// empty (non-nil) AllowedIPs or AllowedEmailAccounts clears the restriction.
// Permissions is validated the same way as on create: non-zero, and only
// scopes the server knows.
type APIKeyUpdateParams struct {
	Name                 *string   `json:"name,omitempty"`
	Description          *string   `json:"description,omitempty"`
	Permissions          *uint64   `json:"permissions,omitempty"`
	AllowedIPs           *[]string `json:"allowed_ips,omitempty"`
	AllowedEmailAccounts *[]string `json:"allowed_email_accounts,omitempty"`
	RateLimitPerMinute   *int      `json:"rate_limit_per_minute,omitempty"`
}

// Permission describes one scope bit the API advertises.
type Permission struct {
	// Name is the stable identifier, for example "READ_CAMPAIGNS".
	Name  string `json:"name"`
	Value uint64 `json:"value"`
	// Description is human-readable copy for a permission picker.
	Description string `json:"description"`
	// Category groups the bit as "read", "write", "bulk" or "special".
	Category string `json:"category"`
}

// PermissionCatalog is the full set of API scopes plus the server's preset
// masks. Prefer the presets over hard-coding a mask when you want "everything":
// the server's value stays current as scopes are added.
type PermissionCatalog struct {
	Permissions []Permission      `json:"permissions"`
	Presets     PermissionPresets `json:"presets"`
}

// PermissionPresets are the server-side preset masks.
type PermissionPresets struct {
	ReadOnly   uint64 `json:"read_only"`
	FullAccess uint64 `json:"full_access"`
}

// APIKeyUsageSummary is a rollup of the organization's key usage.
type APIKeyUsageSummary struct {
	ActiveKeys  int `json:"active_keys"`
	RevokedKeys int `json:"revoked_keys"`
	ExpiredKeys int `json:"expired_keys"`
	// Requests24h and Errors24h cover the last rolling day.
	Requests24h     int64      `json:"requests_24h"`
	Errors24h       int64      `json:"errors_24h"`
	AvgLatencyMS24h float64    `json:"avg_latency_ms_24h"`
	LastCallAt      *time.Time `json:"last_call_at"`
}

// Bucket granularities accepted by [APIKeyAnalyticsParams.Interval].
const (
	IntervalMinute = "minute"
	IntervalHour   = "hour"
	IntervalDay    = "day"
)

// APIKeyAnalyticsParams selects the window and granularity of a usage report.
type APIKeyAnalyticsParams struct {
	// From defaults to 24 hours before To; To defaults to now. The window may
	// not exceed 90 days.
	From time.Time
	To   time.Time
	// Interval is [IntervalMinute], [IntervalHour] or [IntervalDay]. Leave it
	// empty to let the server pick from the window's span: minutes up to two
	// hours, hours up to a week, days beyond that. The interval it settled on
	// comes back in [APIKeyAnalytics.Interval].
	Interval string
}

func (p *APIKeyAnalyticsParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	setTime(q, "from", &p.From)
	setTime(q, "to", &p.To)
	setNonEmpty(q, "interval", p.Interval)
	return q
}

// APIKeyAnalytics is call volume, latency and error rate over a window, both
// bucketed over time and broken down by endpoint.
type APIKeyAnalytics struct {
	// APIKeyID is the key the report covers, or the zero UUID for the
	// organization-wide report.
	APIKeyID string    `json:"api_key_id"`
	From     time.Time `json:"from"`
	To       time.Time `json:"to"`
	// Interval is one of the Interval* constants.
	Interval  string               `json:"interval"`
	Buckets   []APIKeyUsageBucket  `json:"buckets"`
	Endpoints []APIKeyEndpointStat `json:"endpoints"`
	Total     int64                `json:"total"`
	Errors    int64                `json:"errors"`
}

// APIKeyUsageBucket is one time bucket of API traffic.
type APIKeyUsageBucket struct {
	Bucket       time.Time `json:"bucket"`
	Total        int64     `json:"total"`
	Success      int64     `json:"success"`
	ClientErrors int64     `json:"client_errors"`
	ServerErrors int64     `json:"server_errors"`
	AvgLatencyMS float64   `json:"avg_latency_ms"`
}

// APIKeyEndpointStat is one endpoint's share of the traffic.
type APIKeyEndpointStat struct {
	Endpoint     string  `json:"endpoint"`
	Method       string  `json:"method"`
	Count        int64   `json:"count"`
	ErrorCount   int64   `json:"error_count"`
	AvgLatencyMS float64 `json:"avg_latency_ms"`
}

// APIKeyUsageLog is a single recorded API request made with a key.
type APIKeyUsageLog struct {
	ID             string    `json:"id"`
	APIKeyID       string    `json:"api_key_id"`
	Endpoint       string    `json:"endpoint"`
	Method         string    `json:"method"`
	IPAddress      string    `json:"ip_address"`
	UserAgent      string    `json:"user_agent"`
	ResponseCode   int       `json:"response_code"`
	ResponseTimeMS int       `json:"response_time_ms"`
	CreatedAt      time.Time `json:"created_at"`
}

// List returns a page of API keys.
func (s *APIKeyService) List(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[APIKey], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[APIKey](ctx, s.client, "api-keys", q, opts...)
}

// Get retrieves a single API key by ID.
func (s *APIKeyService) Get(ctx context.Context, id string, opts ...RequestOption) (*APIKey, *Response, error) {
	return fetch[APIKey](ctx, s.client, "api-keys/"+url.PathEscape(id), opts)
}

// Create provisions a new API key. The returned [APIKeyWithSecret] is the only
// time the plaintext credential is available.
func (s *APIKeyService) Create(ctx context.Context, params *APIKeyCreateParams, opts ...RequestOption) (*APIKeyWithSecret, *Response, error) {
	return send[APIKeyWithSecret](ctx, s.client.post, "api-keys", params, opts)
}

// Update modifies an existing API key.
func (s *APIKeyService) Update(ctx context.Context, id string, params *APIKeyUpdateParams, opts ...RequestOption) (*APIKey, *Response, error) {
	return send[APIKey](ctx, s.client.patch, "api-keys/"+url.PathEscape(id), params, opts)
}

// Revoke permanently revokes an API key. The reason is stored on the key and
// may be empty.
func (s *APIKeyService) Revoke(ctx context.Context, id, reason string, opts ...RequestOption) (*Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "reason", reason)
	return s.client.delete(ctx, withQuery("api-keys/"+url.PathEscape(id), q), opts...)
}

// RevokeSelf revokes the key this client is authenticated with. It is the one
// key route that needs no scope: a credential must always be able to end
// itself, which is what a CLI logout promises. The reason is stored on the key
// and may be empty (the server records that the key revoked itself). A session
// caller gets a 400 — there is no key in that request to end; use
// [AuthService.Logout]. Every request after this one fails with 401.
func (s *APIKeyService) RevokeSelf(ctx context.Context, reason string, opts ...RequestOption) (*Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "reason", reason)
	return s.client.delete(ctx, withQuery("api-keys/self", q), opts...)
}

// Permissions lists every scope bit the API exposes, together with the preset
// masks.
func (s *APIKeyService) Permissions(ctx context.Context, opts ...RequestOption) (*PermissionCatalog, *Response, error) {
	return fetch[PermissionCatalog](ctx, s.client, "api-keys/permissions", opts)
}

// UsageSummary returns a rollup of the organization's key usage.
func (s *APIKeyService) UsageSummary(ctx context.Context, opts ...RequestOption) (*APIKeyUsageSummary, *Response, error) {
	return fetch[APIKeyUsageSummary](ctx, s.client, "api-keys/usage/summary", opts)
}

// UsageAnalytics returns call volume and latency across every key in the
// organization.
func (s *APIKeyService) UsageAnalytics(ctx context.Context, params *APIKeyAnalyticsParams, opts ...RequestOption) (*APIKeyAnalytics, *Response, error) {
	return fetch[APIKeyAnalytics](ctx, s.client, withQuery("api-keys/usage/analytics", params.values()), opts)
}

// Analytics returns call volume and latency for a single key.
func (s *APIKeyService) Analytics(ctx context.Context, id string, params *APIKeyAnalyticsParams, opts ...RequestOption) (*APIKeyAnalytics, *Response, error) {
	return fetch[APIKeyAnalytics](ctx, s.client, withQuery("api-keys/"+url.PathEscape(id)+"/analytics", params.values()), opts)
}

// Logs returns a page of individual requests made with a key, most recent
// first. The server caps [ListOptions.Limit] at 200 and defaults to 50.
func (s *APIKeyService) Logs(ctx context.Context, id string, params *ListOptions, opts ...RequestOption) (*Page[APIKeyUsageLog], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[APIKeyUsageLog](ctx, s.client, "api-keys/"+url.PathEscape(id)+"/logs", q, opts...)
}
