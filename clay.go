package warmbly

import (
	"context"
	"net/url"
	"time"
)

// ClayService reads and edits the settings of a connected Clay workspace: where
// rows a Clay table sends land, and which Warmbly events are sent back to a
// Clay table. Every method takes the id of the Clay connection, which comes
// from [IntegrationService.Connect] with [ProviderClay] or
// [IntegrationService.Connections].
//
// Rows themselves arrive through the connection's inbound URL
// ([IntegrationConnection.InboundWebhookURL]), which Clay calls with its HTTP
// API column. That endpoint is authenticated by the secret in its path, not
// by an API key, so this SDK does not wrap it.
//
// Reading needs an API key with [PermIntegrations], or a member who can manage
// settings or use integrations. Editing needs [PermIntegrations] on a key and
// the manage-settings right on a member.
type ClayService service

// ClayInboundDefaults decide where rows a Clay table sends land. A row may
// name its own campaign.
type ClayInboundDefaults struct {
	// CampaignID enrolls new contacts in a campaign. Nil enrolls none.
	CampaignID  *string  `json:"campaign_id,omitempty"`
	CategoryIDs []string `json:"category_ids"`
	SegmentIDs  []string `json:"segment_ids"`
	// Dedup is one of the ImportDedup* constants.
	Dedup      string `json:"dedup"`
	Subscribed bool   `json:"subscribed"`
}

// ClayOutbound is the default Clay table Warmbly sends events to. The webhook
// URL and the token are sealed server-side; only their presence is returned.
type ClayOutbound struct {
	WebhookURLSet bool   `json:"webhook_url_set"`
	WebhookHost   string `json:"webhook_host,omitempty"`
	AuthTokenSet  bool   `json:"auth_token_set"`
	// Events are the Warmbly event types sent to the table, a subset of
	// [ClaySettings.AvailableEvents].
	Events []string `json:"events"`
}

// ClayStats count what the inbound URL has received.
type ClayStats struct {
	Received       int        `json:"received"`
	Created        int        `json:"created"`
	Updated        int        `json:"updated"`
	Skipped        int        `json:"skipped"`
	Failed         int        `json:"failed"`
	LastReceivedAt *time.Time `json:"last_received_at,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
}

// ClaySettings is everything the Clay page edits and shows.
type ClaySettings struct {
	ConnectionID string `json:"connection_id"`
	Label        string `json:"label"`
	// Status is one of the Connection* constants.
	Status string `json:"status"`
	// Health is "unknown", "healthy", "degraded" or "down".
	Health string `json:"health"`
	// InboundURLMinted reports whether the connection has an inbound URL yet.
	InboundURLMinted bool                `json:"inbound_url_minted"`
	Defaults         ClayInboundDefaults `json:"defaults"`
	Outbound         ClayOutbound        `json:"outbound"`
	Stats            ClayStats           `json:"stats"`
	// AvailableEvents are the event types Outbound.Events can name.
	AvailableEvents []string `json:"available_events"`
}

// ClaySettingsUpdateParams edits the Clay page. A nil field is left alone.
type ClaySettingsUpdateParams struct {
	// Defaults replaces the inbound defaults as a whole.
	Defaults *ClayInboundDefaults `json:"defaults,omitempty"`
	// WebhookURL is the webhook URL of a Clay table's webhook source. It must
	// point at a Clay host (code "clay_webhook_url"). Point at an empty string
	// to clear it.
	WebhookURL *string `json:"webhook_url,omitempty"`
	// AuthToken is the table's auth token, at most 512 characters. Point at an
	// empty string to clear it.
	AuthToken *string `json:"auth_token,omitempty"`
	// Events replaces the events sent to the table. Point at an empty slice to
	// send none.
	Events *[]string `json:"events,omitempty"`
}

// Get returns everything the Clay page shows.
//
// GET /integrations/clay/:id. Needs read access.
func (s *ClayService) Get(ctx context.Context, connectionID string, opts ...RequestOption) (*ClaySettings, *Response, error) {
	return fetch[ClaySettings](ctx, s.client, "integrations/clay/"+url.PathEscape(connectionID), opts)
}

// Update saves the inbound defaults, the outbound table and the events sent to
// it, and returns the settings as stored. Every field replaces its value, so a
// retry converges.
//
// PATCH /integrations/clay/:id. Needs manage-settings access.
func (s *ClayService) Update(ctx context.Context, connectionID string, params *ClaySettingsUpdateParams, opts ...RequestOption) (*ClaySettings, *Response, error) {
	return send[ClaySettings](ctx, s.client.patch, "integrations/clay/"+url.PathEscape(connectionID), params, opts)
}
