package warmbly

import (
	"context"
	"net/url"
	"time"
)

// Community directory listing states returned in [CommunityApp.Status].
const (
	// CommunityPublished is reachable by its link, and listed in discovery
	// once enough workspaces use it.
	CommunityPublished = "published"
	// CommunityFeatured was picked by an operator and is always listed.
	CommunityFeatured = "featured"
	// CommunityHidden was taken down by an operator.
	CommunityHidden = "hidden"
)

// CommunityApp is an OAuth app published in the community directory, as a
// workspace browsing the directory sees it. It never carries a credential,
// redirect or webhook field.
type CommunityApp struct {
	ApplicationID string `json:"application_id"`
	// Slug is the listing's link name.
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Tagline     string `json:"tagline"`
	Description string `json:"description"`
	// Category is one of the catalog categories, or "ai" or "other".
	Category   string `json:"category"`
	LogoURL    string `json:"logo_url"`
	WebsiteURL string `json:"website_url"`
	InstallURL string `json:"install_url"`
	SupportURL string `json:"support_url"`
	PrivacyURL string `json:"privacy_url"`
	// Developer names the publisher.
	Developer string `json:"developer"`
	// Scopes is the permission bitmask the app asks for and Permissions spells
	// it out for a viewer who cannot read the permission catalog.
	Scopes      uint64       `json:"scopes"`
	Permissions []Permission `json:"permissions"`
	// Status is one of the Community* constants.
	Status string `json:"status"`
	// Listed reports whether discovery shows the app: it is featured or
	// widely used.
	Listed bool `json:"listed"`
	// Installs is how many workspaces use the app and Installed whether this
	// one does.
	Installs    int       `json:"installs"`
	Installed   bool      `json:"installed"`
	PublishedAt time.Time `json:"published_at"`
}

// CommunityApps returns a page of the community directory: featured apps first,
// then the most installed. [Pagination.Total] is exact. It needs
// [PermIntegrations], or a session with the integrations or settings member
// permission.
func (s *IntegrationService) CommunityApps(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[CommunityApp], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[CommunityApp](ctx, s.client, "integrations/community", q, opts...)
}

// CommunityApp opens a published listing by its slug, listed or not. It needs
// the same access as [IntegrationService.CommunityApps].
func (s *IntegrationService) CommunityApp(ctx context.Context, slug string, opts ...RequestOption) (*CommunityApp, *Response, error) {
	return fetch[CommunityApp](ctx, s.client, "integrations/community/"+url.PathEscape(slug), opts)
}

// SetSigningKey sets the key a Calendly or Cal.com connection's deliveries must
// be signed with. An empty key goes back to relying on the secret in the
// inbound URL alone. A key of 8 to 512 characters is accepted; anything else
// answers 400, as does a connection that is not Calendly or Cal.com. It needs
// [PermIntegrations].
func (s *IntegrationService) SetSigningKey(ctx context.Context, id, signingKey string, opts ...RequestOption) (*IntegrationConnection, *Response, error) {
	body := struct {
		SigningKey string `json:"signing_key"`
	}{SigningKey: signingKey}
	var out struct {
		Connection *IntegrationConnection `json:"connection"`
	}
	resp, err := s.client.put(ctx, "integrations/connections/"+url.PathEscape(id)+"/signing-key", body, &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Connection, resp, nil
}

// RotateInboundURL mints a new inbound webhook URL for a Calendly or Cal.com
// connection and returns it. The previous URL stops working immediately, so
// update the provider before relying on it. The URL embeds a secret: treat it
// as a credential. It needs [PermIntegrations].
func (s *IntegrationService) RotateInboundURL(ctx context.Context, id string, opts ...RequestOption) (string, *Response, error) {
	var out struct {
		InboundWebhookURL string `json:"inbound_webhook_url"`
	}
	resp, err := s.client.post(ctx, "integrations/connections/"+url.PathEscape(id)+"/rotate-inbound-url", nil, &out, opts...)
	if err != nil {
		return "", resp, err
	}
	return out.InboundWebhookURL, resp, nil
}
