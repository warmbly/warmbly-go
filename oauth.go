package warmbly

import (
	"context"
	"net/url"
	"time"
)

// OAuthAppService registers and manages OAuth 2.1 applications: the client
// credentials, redirect URIs and scopes an integration uses to act on behalf of
// a Warmbly user, plus the app-level webhook subscription that comes with them.
//
// This service administers applications. To act as an OAuth client — build an
// authorization URL, exchange a code, refresh a token — use [OAuth2Config] and
// [ClientCredentialsConfig] instead.
type OAuthAppService service

// OAuth application states returned in [OAuthApp.Status].
const (
	OAuthAppActive   = "active"
	OAuthAppInactive = "inactive"
)

// OAuthApp is a registered OAuth 2.1 application. The client secret is never
// included here; it is returned once by [OAuthAppService.Create] and
// [OAuthAppService.RotateSecret].
type OAuthApp struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	CreatedBy      string `json:"created_by"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	LogoURL        string `json:"logo_url,omitempty"`
	WebsiteURL     string `json:"website_url,omitempty"`
	// ClientID is the public client identifier.
	ClientID string `json:"client_id"`
	// RedirectURIs are the exact URIs the authorization code may be returned
	// to. They are matched exactly, not by prefix.
	RedirectURIs []string `json:"redirect_uris"`
	// Scopes is the permission bitmask the app may request, built from the
	// Perm* constants.
	Scopes uint64 `json:"scopes"`

	// AllowedWebhookDomains constrains the host of any webhook endpoint the
	// app registers. A leading dot matches subdomains; without one the match
	// is exact. Empty means the app cannot register webhooks at all.
	AllowedWebhookDomains []string `json:"allowed_webhook_domains,omitempty"`
	// WebhookURL turns on the app-level subscription: every workspace that
	// authorizes the app delivers to this URL, scoped to what that workspace
	// granted. Its host must fall inside AllowedWebhookDomains.
	WebhookURL string `json:"webhook_url,omitempty"`
	// WebhookEvents narrows that subscription. Empty means every non-firehose
	// event the grant's scopes allow.
	WebhookEvents []string `json:"webhook_events,omitempty"`

	// Status is [OAuthAppActive] or [OAuthAppInactive].
	Status string `json:"status"`
	// IsPublic marks a client that authenticates with PKCE and no secret, such
	// as a native app or an MCP client. PKCE is mandatory for these.
	IsPublic bool `json:"is_public"`
	// DynamicallyRegistered is true for clients that self-registered through
	// the RFC 7591 endpoint.
	DynamicallyRegistered bool `json:"dynamically_registered"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OAuthAppWithSecret is an [OAuthApp] together with its plaintext client
// secret, returned only at creation.
type OAuthAppWithSecret struct {
	OAuthApp
	// ClientSecret cannot be retrieved again. Store it securely.
	ClientSecret string `json:"client_secret,omitempty"`
}

// OAuthAppParams registers or replaces an OAuth application. The API rewrites
// the application from what you send rather than merging, so send the complete
// desired state on update.
type OAuthAppParams struct {
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	LogoURL      string   `json:"logo_url,omitempty"`
	WebsiteURL   string   `json:"website_url,omitempty"`
	RedirectURIs []string `json:"redirect_uris"`
	// Scopes is the permission bitmask, built from the Perm* constants.
	Scopes                uint64   `json:"scopes"`
	AllowedWebhookDomains []string `json:"allowed_webhook_domains,omitempty"`
	WebhookURL            string   `json:"webhook_url,omitempty"`
	WebhookEvents         []string `json:"webhook_events,omitempty"`
}

// AuthorizedApp is an application a user has granted access to their workspace.
type AuthorizedApp struct {
	ApplicationID string `json:"application_id"`
	Name          string `json:"name"`
	LogoURL       string `json:"logo_url,omitempty"`
	WebsiteURL    string `json:"website_url,omitempty"`
	// Scopes is what was actually granted, which may be less than the app
	// asked for.
	Scopes       uint64     `json:"scopes"`
	AuthorizedAt time.Time  `json:"authorized_at"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
}

// ConsentInfo is what a consent screen renders: who is asking, for what, and
// where the user will be sent back.
type ConsentInfo struct {
	ClientID    string `json:"client_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	LogoURL     string `json:"logo_url"`
	WebsiteURL  string `json:"website_url"`
	RedirectURI string `json:"redirect_uri"`
	// Scopes are the human-readable scope names being requested.
	Scopes []string `json:"scopes"`
	State  string   `json:"state"`
}

// AuthorizeParams approves a consent request and mints an authorization code.
// The fields mirror the query parameters the application sent the user with.
type AuthorizeParams struct {
	// ResponseType is "code".
	ResponseType string `json:"response_type"`
	ClientID     string `json:"client_id"`
	RedirectURI  string `json:"redirect_uri"`
	// Scope is the space-delimited set being granted.
	Scope string `json:"scope"`
	State string `json:"state"`
	// CodeChallenge and CodeChallengeMethod carry PKCE, which is required for
	// a public client.
	CodeChallenge       string `json:"code_challenge,omitempty"`
	CodeChallengeMethod string `json:"code_challenge_method,omitempty"`
}

// List returns the workspace's registered OAuth applications.
func (s *OAuthAppService) List(ctx context.Context, opts ...RequestOption) ([]OAuthApp, *Response, error) {
	var out struct {
		Applications []OAuthApp `json:"applications"`
	}
	resp, err := s.client.get(ctx, "oauth/applications", &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Applications, resp, nil
}

// Get retrieves a single OAuth application.
func (s *OAuthAppService) Get(ctx context.Context, id string, opts ...RequestOption) (*OAuthApp, *Response, error) {
	return fetch[OAuthApp](ctx, s.client, "oauth/applications/"+url.PathEscape(id), opts)
}

// Create registers a new OAuth application. The returned [OAuthAppWithSecret]
// is the only time the client secret is available.
func (s *OAuthAppService) Create(ctx context.Context, params *OAuthAppParams, opts ...RequestOption) (*OAuthAppWithSecret, *Response, error) {
	return send[OAuthAppWithSecret](ctx, s.client, s.client.post, "oauth/applications", params, opts)
}

// Update replaces an OAuth application's registration. See [OAuthAppParams].
func (s *OAuthAppService) Update(ctx context.Context, id string, params *OAuthAppParams, opts ...RequestOption) (*OAuthApp, *Response, error) {
	return send[OAuthApp](ctx, s.client, s.client.patch, "oauth/applications/"+url.PathEscape(id), params, opts)
}

// Delete permanently removes an OAuth application, revoking every token issued
// under it.
func (s *OAuthAppService) Delete(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "oauth/applications/"+url.PathEscape(id), opts...)
}

// RotateSecret issues a new client secret and returns it. The previous secret
// stops working immediately, so deploy the new one before rotating.
func (s *OAuthAppService) RotateSecret(ctx context.Context, id string, opts ...RequestOption) (string, *Response, error) {
	var out struct {
		ClientSecret string `json:"client_secret"`
	}
	resp, err := s.client.post(ctx, "oauth/applications/"+url.PathEscape(id)+"/rotate-secret", nil, &out, opts...)
	if err != nil {
		return "", resp, err
	}
	return out.ClientSecret, resp, nil
}

// UploadLogo stores an app logo (PNG or JPEG, up to 2 MB) and returns its URL,
// which you then pass in [OAuthAppParams.LogoURL]. It is a separate call so a
// logo can be uploaded during registration, before the app has an id.
func (s *OAuthAppService) UploadLogo(ctx context.Context, file *FileUpload, opts ...RequestOption) (string, *Response, error) {
	var out struct {
		LogoURL string `json:"logo_url"`
	}
	resp, err := s.client.postMultipart(ctx, "oauth/application-logo", "logo", file, nil, &out, opts...)
	if err != nil {
		return "", resp, err
	}
	return out.LogoURL, resp, nil
}

// WebhookSecret returns the signing secret for the app-level webhook
// subscription. Verify deliveries with it exactly as for an ordinary endpoint;
// see [VerifyWebhookSignature].
func (s *OAuthAppService) WebhookSecret(ctx context.Context, id string, opts ...RequestOption) (string, *Response, error) {
	var out struct {
		WebhookSecret string `json:"webhook_secret"`
	}
	resp, err := s.client.get(ctx, "oauth/applications/"+url.PathEscape(id)+"/webhook-secret", &out, opts...)
	if err != nil {
		return "", resp, err
	}
	return out.WebhookSecret, resp, nil
}

// RotateWebhookSecret issues a new app-webhook signing secret and returns it.
// The previous secret stops verifying immediately.
func (s *OAuthAppService) RotateWebhookSecret(ctx context.Context, id string, opts ...RequestOption) (string, *Response, error) {
	var out struct {
		WebhookSecret string `json:"webhook_secret"`
	}
	resp, err := s.client.post(ctx, "oauth/applications/"+url.PathEscape(id)+"/webhook-secret/rotate", nil, &out, opts...)
	if err != nil {
		return "", resp, err
	}
	return out.WebhookSecret, resp, nil
}

// WebhookEndpoints returns the per-workspace endpoints materialized from the
// app-level subscription, one for each workspace that authorized the app.
func (s *OAuthAppService) WebhookEndpoints(ctx context.Context, id string, opts ...RequestOption) ([]Webhook, *Response, error) {
	var out struct {
		Endpoints []Webhook `json:"endpoints"`
	}
	resp, err := s.client.get(ctx, "oauth/applications/"+url.PathEscape(id)+"/webhook-endpoints", &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Endpoints, resp, nil
}

// WebhookDeliveries returns a page of the app's delivery log, across every
// workspace that authorized it.
func (s *OAuthAppService) WebhookDeliveries(ctx context.Context, id string, params *WebhookDeliveryListParams, opts ...RequestOption) (*Page[WebhookDelivery], error) {
	q := params.values()
	q.Del("endpoint_id")
	return listJSON[WebhookDelivery](ctx, s.client, "oauth/applications/"+url.PathEscape(id)+"/webhook-deliveries", q, opts...)
}

// --- consent flow (session-only) ---

// AuthorizeDetails resolves an incoming authorization request into what a
// consent screen should show. Pass the query parameters the application sent
// the user with.
func (s *OAuthAppService) AuthorizeDetails(ctx context.Context, query url.Values, opts ...RequestOption) (*ConsentInfo, *Response, error) {
	return fetch[ConsentInfo](ctx, s.client, withQuery("oauth/authorize/details", query), opts)
}

// Authorize approves a consent request and returns the URL the browser should
// be sent to, carrying the authorization code back to the application.
func (s *OAuthAppService) Authorize(ctx context.Context, params *AuthorizeParams, opts ...RequestOption) (string, *Response, error) {
	var out struct {
		RedirectURL string `json:"redirect_url"`
	}
	resp, err := s.client.post(ctx, "oauth/authorize", params, &out, opts...)
	if err != nil {
		return "", resp, err
	}
	return out.RedirectURL, resp, nil
}

// AuthorizedApps returns the applications the caller has granted access to the
// current workspace.
func (s *OAuthAppService) AuthorizedApps(ctx context.Context, opts ...RequestOption) ([]AuthorizedApp, *Response, error) {
	var out struct {
		AuthorizedApps []AuthorizedApp `json:"authorized_apps"`
	}
	resp, err := s.client.get(ctx, "oauth/authorized-apps", &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.AuthorizedApps, resp, nil
}

// RevokeAuthorizedApp withdraws the caller's grant to an application and
// revokes its tokens.
func (s *OAuthAppService) RevokeAuthorizedApp(ctx context.Context, applicationID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "oauth/authorized-apps/"+url.PathEscape(applicationID), opts...)
}

// --- dynamic client registration (RFC 7591) ---

// DynamicClientParams self-registers a client at runtime, the RFC 7591 way that
// an MCP client uses to get credentials without a human registering an app
// first.
type DynamicClientParams struct {
	ClientName   string   `json:"client_name,omitempty"`
	RedirectURIs []string `json:"redirect_uris"`
	// GrantTypes and ResponseTypes default to the authorization-code flow.
	GrantTypes    []string `json:"grant_types,omitempty"`
	ResponseTypes []string `json:"response_types,omitempty"`
	// TokenEndpointAuthMethod of "none" registers a public client, which
	// authenticates with PKCE and holds no secret.
	TokenEndpointAuthMethod string `json:"token_endpoint_auth_method,omitempty"`
	// Scope is the space-delimited set being asked for.
	Scope     string `json:"scope,omitempty"`
	ClientURI string `json:"client_uri,omitempty"`
	LogoURI   string `json:"logo_uri,omitempty"`
}

// DynamicClient is the RFC 7591 client-information response.
type DynamicClient struct {
	ClientID string `json:"client_id"`
	// ClientSecret is set only for a confidential registration.
	ClientSecret string `json:"client_secret,omitempty"`
	// ClientIDIssuedAt is a Unix timestamp.
	ClientIDIssuedAt int64 `json:"client_id_issued_at"`

	ClientName              string   `json:"client_name,omitempty"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	// Scope is what was actually granted, which is capped below what a
	// human-registered application can hold: a self-registered client can
	// never send mail or mint credentials.
	Scope string `json:"scope"`
}

// RegisterDynamicClient self-registers an OAuth client. The endpoint is open
// and unauthenticated but per-IP rate limited, and registration alone grants no
// access — a human still has to consent.
func (s *OAuthAppService) RegisterDynamicClient(ctx context.Context, params *DynamicClientParams, opts ...RequestOption) (*DynamicClient, *Response, error) {
	return send[DynamicClient](ctx, s.client, s.client.post, "oauth/register", params, opts)
}
