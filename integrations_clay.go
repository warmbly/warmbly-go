package warmbly

import (
	"context"
	"net/url"
	"time"
)

// ClayInboundDefaults decide where rows a Clay table sends land. A row may name
// its own campaign.
type ClayInboundDefaults struct {
	// CampaignID enrolls new contacts in a campaign.
	CampaignID *string `json:"campaign_id,omitempty"`
	// CategoryIDs tags new contacts (at most 50) and SegmentIDs adds them to
	// segments (at most 20).
	CategoryIDs []string `json:"category_ids"`
	SegmentIDs  []string `json:"segment_ids"`
	// Dedup is [ImportDedupSkip] or [ImportDedupUpdate]. Empty on a write means
	// update.
	Dedup string `json:"dedup"`
	// Subscribed marks new contacts as subscribed.
	Subscribed bool `json:"subscribed"`
}

// ClayOutbound describes the Clay table Warmbly sends events to. The table URL
// and token are stored sealed, so only their presence and host are shown.
type ClayOutbound struct {
	WebhookURLSet bool   `json:"webhook_url_set"`
	WebhookHost   string `json:"webhook_host,omitempty"`
	AuthTokenSet  bool   `json:"auth_token_set"`
	// Events are the Warmbly events sent to the table.
	Events []string `json:"events"`
}

// ClayStats counts what the inbound URL has received.
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
	// Status is one of the Connection* constants and Health is "unknown",
	// "healthy", "degraded" or "down".
	Status string `json:"status"`
	Health string `json:"health"`
	// InboundURLMinted reports that the connection has its inbound URL. The
	// URL itself is returned in [IntegrationConnection.InboundWebhookURL] when
	// the connection is created.
	InboundURLMinted bool                `json:"inbound_url_minted"`
	Defaults         ClayInboundDefaults `json:"defaults"`
	Outbound         ClayOutbound        `json:"outbound"`
	Stats            ClayStats           `json:"stats"`
	// AvailableEvents are the events a table can be sent.
	AvailableEvents []string `json:"available_events"`
}

// ClaySettingsUpdate edits the Clay page. Nil leaves a value alone.
type ClaySettingsUpdate struct {
	// Defaults replaces the inbound defaults as a whole.
	Defaults *ClayInboundDefaults `json:"defaults,omitempty"`
	// WebhookURL is the URL of a Clay table's webhook source. A pointer to an
	// empty string clears it.
	WebhookURL *string `json:"webhook_url,omitempty"`
	// AuthToken is the table's auth token. A pointer to an empty string clears
	// it.
	AuthToken *string `json:"auth_token,omitempty"`
	// Events replaces the events sent to the table. A pointer to an empty
	// slice sends none. Choosing events needs a WebhookURL.
	Events *[]string `json:"events,omitempty"`
}

// ClaySettings returns a Clay connection's inbound defaults, outbound table and
// stats. It needs [PermIntegrations], or a session with the integrations or
// settings member permission.
func (s *IntegrationService) ClaySettings(ctx context.Context, id string, opts ...RequestOption) (*ClaySettings, *Response, error) {
	return fetch[ClaySettings](ctx, s.client, "integrations/clay/"+url.PathEscape(id), opts)
}

// UpdateClaySettings saves a Clay connection's inbound defaults, outbound table
// and the events sent to it, and returns the result. Every part replaces its
// value, so a retry converges. It needs [PermIntegrations].
func (s *IntegrationService) UpdateClaySettings(ctx context.Context, id string, params *ClaySettingsUpdate, opts ...RequestOption) (*ClaySettings, *Response, error) {
	return send[ClaySettings](ctx, s.client.patch, "integrations/clay/"+url.PathEscape(id), params, opts)
}
