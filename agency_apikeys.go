package warmbly

import (
	"context"
	"net/url"
	"time"
)

// AgencyAPIKey is an agency API key as returned by the API. The secret is
// never included; see [AgencyAPIKeyWithSecret].
type AgencyAPIKey struct {
	ID       string `json:"id"`
	AgencyID string `json:"agency_id"`
	// CreatedBy is the staff member the key acts as in client workspaces. The
	// key stops working in a workspace that person leaves.
	CreatedBy string `json:"created_by"`
	Name      string `json:"name"`
	// KeyPrefix and KeySuffix are the non-secret ends of the key.
	KeyPrefix string `json:"key_prefix"`
	KeySuffix string `json:"key_suffix"`
	// Permissions is the scope bitmask: workspace bits (see
	// [PermReadEmails]) and agency bits (see [PermAgencyRead]).
	Permissions uint64 `json:"permissions"`
	// AllowedIPs restricts use to these IPs or CIDR ranges. Empty means any.
	AllowedIPs         []string `json:"allowed_ips"`
	RateLimitPerMinute int      `json:"rate_limit_per_minute"`
	// Status is [APIKeyStatusActive], [APIKeyStatusRevoked] or
	// [APIKeyStatusExpired].
	Status        string     `json:"status"`
	LastUsedAt    *time.Time `json:"last_used_at,omitempty"`
	LastRequestIP *string    `json:"last_request_ip,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
	RevokedReason *string    `json:"revoked_reason,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// Can reports whether the key holds every bit in perm.
func (k *AgencyAPIKey) Can(perm uint64) bool {
	return k != nil && perm != 0 && k.Permissions&perm == perm
}

// AgencyAPIKeyWithSecret is a newly created agency key. Secret (prefixed
// "wmagency_") is returned only here; store it now.
type AgencyAPIKeyWithSecret struct {
	AgencyAPIKey
	Secret string `json:"secret"`
}

// AgencyAPIKeyCreateParams creates an agency API key.
type AgencyAPIKeyCreateParams struct {
	// Name is required, at most 255 characters.
	Name string `json:"name"`
	// Permissions is a non-zero set of bits from
	// [AgencyAPIKeyPermissions.Grantable].
	Permissions uint64 `json:"permissions"`
	// AllowedIPs restricts use to these IPs or CIDR ranges (at most 64).
	AllowedIPs []string `json:"allowed_ips,omitempty"`
	// RateLimitPerMinute is 1 to 10000; nil takes the server default.
	RateLimitPerMinute *int `json:"rate_limit_per_minute,omitempty"`
	// ExpiresAt must be in the future; nil never expires.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// AgencyAPIKeyUpdateParams renames a key or replaces its IP allowlist. Set at
// least one field.
type AgencyAPIKeyUpdateParams struct {
	Name *string `json:"name,omitempty"`
	// AllowedIPs replaces the allowlist; a pointer to an empty slice clears
	// it.
	AllowedIPs *[]string `json:"allowed_ips,omitempty"`
}

// AgencyAPIKeyPermissions is what an agency key may hold.
type AgencyAPIKeyPermissions struct {
	// Workspace are the workspace scopes an agency key may carry into client
	// workspaces (every workspace scope except API key management, realtime
	// and mailbox credentials).
	Workspace []Permission `json:"workspace"`
	// Agency are the agency scopes (the PermAgency* bits).
	Agency  []Permission      `json:"agency"`
	Presets PermissionPresets `json:"presets"`
	// Grantable is every bit an agency key may carry.
	Grantable uint64 `json:"grantable"`
}

// APIKeys returns a page of the agency's API keys, newest first. Limit is 1
// to 100 (default 50). Owner or admin.
//
// Session only.
func (s *AgencyService) APIKeys(ctx context.Context, agencyID string, params *ListOptions, opts ...RequestOption) (*Page[AgencyAPIKey], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[AgencyAPIKey](ctx, s.client, agencyPath(agencyID, "api-keys"), q, opts...)
}

// APIKeyPermissions returns the scopes an agency key may hold and the preset
// masks.
//
// Session only.
func (s *AgencyService) APIKeyPermissions(ctx context.Context, agencyID string, opts ...RequestOption) (*AgencyAPIKeyPermissions, *Response, error) {
	return fetch[AgencyAPIKeyPermissions](ctx, s.client, agencyPath(agencyID, "api-keys", "permissions"), opts)
}

// CreateAPIKey creates an agency API key. The secret is in this response
// only. Owner or admin. Pass [WithIdempotencyKey] to make a retry safe.
//
// Session only; needs a recent sign-in.
func (s *AgencyService) CreateAPIKey(ctx context.Context, agencyID string, params *AgencyAPIKeyCreateParams, opts ...RequestOption) (*AgencyAPIKeyWithSecret, *Response, error) {
	return send[AgencyAPIKeyWithSecret](ctx, s.client.post, agencyPath(agencyID, "api-keys"), params, opts)
}

// UpdateAPIKey renames an active key or replaces its IP allowlist.
//
// Session only; needs a recent sign-in.
func (s *AgencyService) UpdateAPIKey(ctx context.Context, agencyID, keyID string, params *AgencyAPIKeyUpdateParams, opts ...RequestOption) (*AgencyAPIKey, *Response, error) {
	return send[AgencyAPIKey](ctx, s.client.patch, agencyPath(agencyID, "api-keys", url.PathEscape(keyID)), params, opts)
}

// RevokeAPIKey revokes an agency key. Revoking twice succeeds, so a retry is
// safe.
//
// Session only; needs a recent sign-in.
func (s *AgencyService) RevokeAPIKey(ctx context.Context, agencyID, keyID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, agencyPath(agencyID, "api-keys", url.PathEscape(keyID)), opts...)
}
