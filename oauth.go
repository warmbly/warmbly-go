package warmbly

import (
	"context"
	"net/url"
	"time"
)

// OAuthAppService manages OAuth 2.1 applications (client registrations) for the
// current organization. An application owns a client_id/client_secret pair and
// a set of redirect URIs and scopes; use it together with the flows in
// oauth_flow.go to authenticate end users.
//
// This service registers and manages applications. To act as an OAuth client
// (build an authorization URL, exchange a code, refresh tokens) use
// [OAuth2Config] and [ClientCredentialsConfig].
type OAuthAppService service

// OAuthApp is a registered OAuth 2.1 application. The client secret is never
// included here; it is returned exactly once by [OAuthAppService.Create] and
// [OAuthAppService.RotateSecret] as an [OAuthAppWithSecret].
type OAuthApp struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	CreatedBy      string `json:"created_by"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	LogoURL        string `json:"logo_url,omitempty"`
	WebsiteURL     string `json:"website_url,omitempty"`
	// ClientID is the public client identifier (prefixed by the platform).
	ClientID string `json:"client_id"`
	// RedirectURIs are the exact redirect URIs permitted for this app.
	RedirectURIs []string `json:"redirect_uris"`
	// Scopes are the permission keys the app may request.
	Scopes []string `json:"scopes"`
	// AllowedWebhookDomains restricts the domains the app may register webhook
	// URLs under.
	AllowedWebhookDomains []string `json:"allowed_webhook_domains,omitempty"`
	// WebhookURL and WebhookEvents configure an app-level webhook subscription.
	WebhookURL    string   `json:"webhook_url,omitempty"`
	WebhookEvents []string `json:"webhook_events,omitempty"`
	// Status is "active" or "inactive".
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OAuthAppWithSecret is an [OAuthApp] together with its plaintext client secret.
// The ClientSecret is only available at creation and after rotation; store it
// securely.
type OAuthAppWithSecret struct {
	OAuthApp
	// ClientSecret is the plaintext secret. It cannot be retrieved again.
	ClientSecret string `json:"client_secret"`
}

// OAuthAppCreateParams are the parameters for registering an OAuth application.
type OAuthAppCreateParams struct {
	Name                  string   `json:"name"`
	Description           string   `json:"description,omitempty"`
	LogoURL               string   `json:"logo_url,omitempty"`
	WebsiteURL            string   `json:"website_url,omitempty"`
	RedirectURIs          []string `json:"redirect_uris"`
	Scopes                []string `json:"scopes,omitempty"`
	AllowedWebhookDomains []string `json:"allowed_webhook_domains,omitempty"`
	WebhookURL            string   `json:"webhook_url,omitempty"`
	WebhookEvents         []string `json:"webhook_events,omitempty"`
}

// OAuthAppUpdateParams are the parameters for updating an OAuth application.
// Only non-nil fields are sent.
type OAuthAppUpdateParams struct {
	Name                  *string   `json:"name,omitempty"`
	Description           *string   `json:"description,omitempty"`
	LogoURL               *string   `json:"logo_url,omitempty"`
	WebsiteURL            *string   `json:"website_url,omitempty"`
	RedirectURIs          *[]string `json:"redirect_uris,omitempty"`
	Scopes                *[]string `json:"scopes,omitempty"`
	AllowedWebhookDomains *[]string `json:"allowed_webhook_domains,omitempty"`
	WebhookURL            *string   `json:"webhook_url,omitempty"`
	WebhookEvents         *[]string `json:"webhook_events,omitempty"`
	Status                *string   `json:"status,omitempty"`
}

// OAuthAppListParams paginates a list of OAuth applications.
type OAuthAppListParams struct {
	ListOptions
}

func (p *OAuthAppListParams) values() url.Values {
	q := make(url.Values)
	if p != nil {
		p.ListOptions.apply(q)
	}
	return q
}

// List returns a page of registered OAuth applications.
func (s *OAuthAppService) List(ctx context.Context, params *OAuthAppListParams) (*Page[OAuthApp], error) {
	return listJSON[OAuthApp](ctx, s.client, "oauth/applications", params.values())
}

// Get retrieves a single OAuth application by ID.
func (s *OAuthAppService) Get(ctx context.Context, id string) (*OAuthApp, *Response, error) {
	app := new(OAuthApp)
	resp, err := s.client.get(ctx, "oauth/applications/"+url.PathEscape(id), app)
	if err != nil {
		return nil, resp, err
	}
	return app, resp, nil
}

// Create registers a new OAuth application. The returned [OAuthAppWithSecret]
// is the only time the client secret is available.
func (s *OAuthAppService) Create(ctx context.Context, params *OAuthAppCreateParams) (*OAuthAppWithSecret, *Response, error) {
	app := new(OAuthAppWithSecret)
	resp, err := s.client.post(ctx, "oauth/applications", params, app)
	if err != nil {
		return nil, resp, err
	}
	return app, resp, nil
}

// Update modifies an existing OAuth application.
func (s *OAuthAppService) Update(ctx context.Context, id string, params *OAuthAppUpdateParams) (*OAuthApp, *Response, error) {
	app := new(OAuthApp)
	resp, err := s.client.patch(ctx, "oauth/applications/"+url.PathEscape(id), params, app)
	if err != nil {
		return nil, resp, err
	}
	return app, resp, nil
}

// Delete permanently removes an OAuth application.
func (s *OAuthAppService) Delete(ctx context.Context, id string) (*Response, error) {
	return s.client.delete(ctx, "oauth/applications/"+url.PathEscape(id), nil)
}

// RotateSecret generates a new client secret for the application, invalidating
// the previous one. The new secret is returned exactly once.
func (s *OAuthAppService) RotateSecret(ctx context.Context, id string) (*OAuthAppWithSecret, *Response, error) {
	app := new(OAuthAppWithSecret)
	resp, err := s.client.post(ctx, "oauth/applications/"+url.PathEscape(id)+"/rotate-secret", nil, app)
	if err != nil {
		return nil, resp, err
	}
	return app, resp, nil
}
