package warmbly

import (
	"context"
	"net/url"
	"time"
)

// APIKeyService manages API keys for the current organization.
//
// API keys authenticate server-to-server requests and are prefixed "wmbly_".
// The secret is shown exactly once, at creation time; only a prefix/suffix is
// retrievable afterwards.
type APIKeyService service

// APIKey is an API key as returned by the API. The full secret is never
// included; see [APIKeyWithSecret], returned only by [APIKeyService.Create].
type APIKey struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	UserID         string `json:"user_id"`
	Name           string `json:"name"`
	// Prefix is the leading, non-secret portion shown in listings (e.g. the
	// first 8 characters).
	Prefix string `json:"prefix"`
	// Suffix is the trailing portion shown in listings (e.g. the last 4
	// characters), to help users disambiguate keys.
	Suffix string `json:"suffix"`
	// Scopes are the permission keys granted to the key.
	Scopes []string `json:"scopes"`
	// AllowedEmailAccounts optionally restricts the key to specific email
	// account IDs. Empty means all accounts in the organization.
	AllowedEmailAccounts []string `json:"allowed_email_accounts,omitempty"`
	// AllowedIPs optionally restricts use to these IPs or CIDR ranges.
	AllowedIPs []string `json:"allowed_ips,omitempty"`
	// RateLimitPerMinute is the per-key request ceiling per minute.
	RateLimitPerMinute int        `json:"rate_limit_per_minute"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	RevokedAt          *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt         *time.Time `json:"last_used_at,omitempty"`
	LastUsedIP         string     `json:"last_used_ip,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// Revoked reports whether the key has been revoked.
func (k *APIKey) Revoked() bool { return k.RevokedAt != nil }

// APIKeyWithSecret is an API key together with its plaintext secret. The Secret
// is only ever returned by [APIKeyService.Create]; store it securely.
type APIKeyWithSecret struct {
	APIKey
	// Secret is the full plaintext key (prefixed "wmbly_"). Capture it now; it
	// cannot be retrieved again.
	Secret string `json:"secret"`
}

// APIKeyCreateParams are the parameters for creating an API key.
type APIKeyCreateParams struct {
	Name                 string     `json:"name"`
	Scopes               []string   `json:"scopes,omitempty"`
	AllowedEmailAccounts []string   `json:"allowed_email_accounts,omitempty"`
	AllowedIPs           []string   `json:"allowed_ips,omitempty"`
	RateLimitPerMinute   int        `json:"rate_limit_per_minute,omitempty"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
}

// APIKeyUpdateParams are the parameters for updating an API key. Only non-nil
// fields are sent, so zero values are not mistaken for "clear this field".
type APIKeyUpdateParams struct {
	Name                 *string    `json:"name,omitempty"`
	Scopes               *[]string  `json:"scopes,omitempty"`
	AllowedEmailAccounts *[]string  `json:"allowed_email_accounts,omitempty"`
	AllowedIPs           *[]string  `json:"allowed_ips,omitempty"`
	RateLimitPerMinute   *int       `json:"rate_limit_per_minute,omitempty"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
}

// APIKeyListParams filters and paginates a list of API keys.
type APIKeyListParams struct {
	ListOptions
	// Search filters by name substring.
	Search string
}

func (p *APIKeyListParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.ListOptions.apply(q)
	if p.Search != "" {
		q.Set("search", p.Search)
	}
	return q
}

// Permission describes a permission key that can be granted to an API key or
// OAuth application.
type Permission struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Group       string `json:"group,omitempty"`
}

// APIKeyUsageSummary is an aggregate usage report for the organization's keys.
type APIKeyUsageSummary struct {
	TotalRequests     int64      `json:"total_requests"`
	ThrottledRequests int64      `json:"throttled_requests"`
	WindowStart       *time.Time `json:"window_start,omitempty"`
	WindowEnd         *time.Time `json:"window_end,omitempty"`
}

// List returns a page of API keys.
func (s *APIKeyService) List(ctx context.Context, params *APIKeyListParams) (*Page[APIKey], error) {
	return listJSON[APIKey](ctx, s.client, "api-keys", params.values())
}

// Get retrieves a single API key by ID.
func (s *APIKeyService) Get(ctx context.Context, id string) (*APIKey, *Response, error) {
	key := new(APIKey)
	resp, err := s.client.get(ctx, "api-keys/"+url.PathEscape(id), key)
	if err != nil {
		return nil, resp, err
	}
	return key, resp, nil
}

// Create provisions a new API key. The returned [APIKeyWithSecret] is the only
// time the plaintext secret is available.
func (s *APIKeyService) Create(ctx context.Context, params *APIKeyCreateParams) (*APIKeyWithSecret, *Response, error) {
	key := new(APIKeyWithSecret)
	resp, err := s.client.post(ctx, "api-keys", params, key)
	if err != nil {
		return nil, resp, err
	}
	return key, resp, nil
}

// Update modifies an existing API key.
func (s *APIKeyService) Update(ctx context.Context, id string, params *APIKeyUpdateParams) (*APIKey, *Response, error) {
	key := new(APIKey)
	resp, err := s.client.patch(ctx, "api-keys/"+url.PathEscape(id), params, key)
	if err != nil {
		return nil, resp, err
	}
	return key, resp, nil
}

// Revoke permanently revokes an API key.
func (s *APIKeyService) Revoke(ctx context.Context, id string) (*Response, error) {
	return s.client.delete(ctx, "api-keys/"+url.PathEscape(id), nil)
}

// Permissions lists the permission keys that may be granted to API keys.
func (s *APIKeyService) Permissions(ctx context.Context) ([]Permission, *Response, error) {
	var out struct {
		Data []Permission `json:"data"`
	}
	resp, err := s.client.get(ctx, "api-keys/permissions", &out)
	if err != nil {
		return nil, resp, err
	}
	return out.Data, resp, nil
}

// UsageSummary returns an aggregate usage summary for the organization's keys.
func (s *APIKeyService) UsageSummary(ctx context.Context) (*APIKeyUsageSummary, *Response, error) {
	summary := new(APIKeyUsageSummary)
	resp, err := s.client.get(ctx, "api-keys/usage/summary", summary)
	if err != nil {
		return nil, resp, err
	}
	return summary, resp, nil
}
