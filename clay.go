package warmbly

import (
	"context"
	"net/url"
	"time"
)

// ClayService manages a connected Clay workspace: where rows a Clay table
// sends into Warmbly land, and the default Clay table Warmbly sends events to.
//
// Connect Clay first with [IntegrationService.Connect] and [ProviderClay]. The
// connection's InboundWebhookURL is the URL a Clay HTTP API column posts to;
// rotate it with [IntegrationService.RotateInboundURL]. Every method takes the
// connection id and answers 404 for a connection that is not a Clay one, and
// 503 on an instance without Clay.
//
// All methods take the INTEGRATIONS scope.
type ClayService service

// ClayInboundDefaults decide where rows a Clay table sends land. A row may
// name its own campaign.
type ClayInboundDefaults struct {
	// CampaignID enrolls new contacts in a campaign.
	CampaignID  *string  `json:"campaign_id,omitempty"`
	CategoryIDs []string `json:"category_ids"`
	SegmentIDs  []string `json:"segment_ids"`
	// Dedup is one of the ImportDedup* constants.
	Dedup      string `json:"dedup"`
	Subscribed bool   `json:"subscribed"`
}

// ClayOutbound is the default Clay table Warmbly sends events to. The URL and
// token are sealed server-side; only whether they are set is readable.
type ClayOutbound struct {
	WebhookURLSet bool   `json:"webhook_url_set"`
	WebhookHost   string `json:"webhook_host,omitempty"`
	AuthTokenSet  bool   `json:"auth_token_set"`
	// Events are the Warmbly event types sent to the table.
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
	Status           string              `json:"status"`
	Health           string              `json:"health"`
	InboundURLMinted bool                `json:"inbound_url_minted"`
	Defaults         ClayInboundDefaults `json:"defaults"`
	Outbound         ClayOutbound        `json:"outbound"`
	Stats            ClayStats           `json:"stats"`
	// AvailableEvents are the event types Outbound.Events may name.
	AvailableEvents []string `json:"available_events"`
}

// ClaySettingsUpdateParams edits the Clay page. Nil leaves a value alone, and
// an empty WebhookURL or AuthToken clears it. Every value it carries replaces
// the stored one, so a retry converges.
type ClaySettingsUpdateParams struct {
	Defaults   *ClayInboundDefaults `json:"defaults,omitempty"`
	WebhookURL *string              `json:"webhook_url,omitempty"`
	AuthToken  *string              `json:"auth_token,omitempty"`
	Events     *[]string            `json:"events,omitempty"`
}

// Settings returns everything the Clay page shows.
func (s *ClayService) Settings(ctx context.Context, connectionID string, opts ...RequestOption) (*ClaySettings, *Response, error) {
	return fetch[ClaySettings](ctx, s.client, "integrations/clay/"+url.PathEscape(connectionID), opts)
}

// UpdateSettings saves the inbound defaults, the outbound table and the
// events sent to it, and returns the settings as stored. Needs the
// settings-management right as well as the INTEGRATIONS scope.
func (s *ClayService) UpdateSettings(ctx context.Context, connectionID string, params *ClaySettingsUpdateParams, opts ...RequestOption) (*ClaySettings, *Response, error) {
	return send[ClaySettings](ctx, s.client.patch, "integrations/clay/"+url.PathEscape(connectionID), params, opts)
}
