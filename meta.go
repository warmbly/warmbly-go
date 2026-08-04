package warmbly

import (
	"context"
	"time"
)

// MetaService reads the small, cross-cutting endpoints: who the current
// credential is, the plan catalog, and the timezone list.
type MetaService service

// Authentication types returned in [Identity.AuthType].
const (
	AuthTypeAPIKey = "api_key"
	AuthTypeOAuth  = "oauth"
	AuthTypeJWT    = "jwt"
)

// Identity is who the current credential is and what workspace it acts on.
// Every credential can read it, which makes it the right call for an
// integration to validate a connection and label it.
type Identity struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`

	OrganizationID   *string `json:"organization_id,omitempty"`
	OrganizationName string  `json:"organization_name,omitempty"`

	// AuthType is [AuthTypeAPIKey], [AuthTypeOAuth] or [AuthTypeJWT].
	AuthType string `json:"auth_type"`
	// Scopes lists the granted API scope names for an API key or OAuth token.
	// It is empty for a session, which carries organization role permissions
	// instead.
	Scopes []string `json:"scopes"`
}

// Billing periods returned in [Plan.Duration].
const (
	DurationMonth = "month"
	DurationYear  = "year"
)

// Plan is one purchasable plan and the ceilings it carries.
type Plan struct {
	ID   string  `json:"id"`
	Name *string `json:"name,omitempty"`

	MaxContacts  uint `json:"max_contacts"`
	DailyEmails  uint `json:"daily_emails"`
	AIGeneration bool `json:"ai_generation"`
	AccountLimit uint `json:"account_limit"`

	Price           float32 `json:"price"`
	DiscountedPrice float32 `json:"discounted_price"`
	// Duration is [DurationMonth] or [DurationYear].
	Duration string `json:"duration"`
	// Savings is the percentage saved against the monthly price.
	Savings uint8 `json:"savings"`
	// Public reports whether the plan is offered on the pricing page.
	Public bool `json:"public"`

	StripePriceID       *string `json:"stripe_price_id,omitempty"`
	StripePriceIDYearly *string `json:"stripe_price_id_yearly,omitempty"`
	StripeProductID     *string `json:"stripe_product_id,omitempty"`

	DedicatedWorkers   int  `json:"dedicated_workers"`
	DailyCampaignLimit *int `json:"daily_campaign_limit,omitempty"`

	MaxCampaigns       *int `json:"max_campaigns,omitempty"`
	MaxActiveCampaigns *int `json:"max_active_campaigns,omitempty"`
	MaxTeamMembers     *int `json:"max_team_members,omitempty"`
	MaxEmailAccounts   *int `json:"max_email_accounts,omitempty"`

	// MonthlyCredits is the AI credit grant included each month.
	MonthlyCredits int `json:"monthly_credits"`

	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// Timezone is one selectable IANA timezone.
type Timezone struct {
	// Name is the IANA identifier, for example "Europe/Berlin".
	Name string `json:"name"`
	// DisplayName is how to label it in a picker.
	DisplayName string `json:"display_name"`
}

// Identity returns who the current credential is and which workspace it acts
// on. Any valid credential can call it, whatever its scopes.
func (s *MetaService) Identity(ctx context.Context, opts ...RequestOption) (*Identity, *Response, error) {
	return fetch[Identity](ctx, s.client, "me", opts)
}

// Plans returns the plan catalog.
func (s *MetaService) Plans(ctx context.Context, opts ...RequestOption) ([]Plan, *Response, error) {
	var out struct {
		Plans []Plan `json:"plans"`
	}
	resp, err := s.client.get(ctx, "plans", &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Plans, resp, nil
}

// Timezones returns the timezones a campaign schedule or mailbox can use.
func (s *MetaService) Timezones(ctx context.Context, opts ...RequestOption) ([]Timezone, *Response, error) {
	return fetchSlice[Timezone](ctx, s.client, "timezones", opts)
}

// RealtimeInfo tells a client where the realtime gateway lives.
type RealtimeInfo struct {
	// WebsocketURL is the endpoint to dial.
	WebsocketURL string `json:"websocket_url"`
	// Topics are the channels this caller may subscribe to.
	Topics []string `json:"topics"`
}

// Realtime returns the gateway's connection details. This route is
// session-only. For a programmatic gateway connection, authenticate with an API
// key holding [PermRealtimeSubscribe]; see the gateway subpackage.
func (s *MetaService) Realtime(ctx context.Context, opts ...RequestOption) (*RealtimeInfo, *Response, error) {
	return fetch[RealtimeInfo](ctx, s.client, "realtime/info", opts)
}

// GatewayTicket is a short-lived, single-use credential for opening a gateway
// connection from a browser session.
type GatewayTicket struct {
	// URL is the websocket endpoint with the ticket already embedded, so it can
	// be dialed as-is.
	URL string `json:"url"`
	// ExpiresIn is the ticket's lifetime in seconds.
	ExpiresIn float64 `json:"expires_in"`
}

// GatewayTicket mints a single-use websocket ticket for the current session.
// This route is session-only.
func (s *MetaService) GatewayTicket(ctx context.Context, opts ...RequestOption) (*GatewayTicket, *Response, error) {
	return send[GatewayTicket](ctx, s.client.post, "getaway", nil, opts)
}
