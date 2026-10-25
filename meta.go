package warmbly

import (
	"context"
	"encoding/json"
	"net/url"
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

	Price float32 `json:"price"`
	// PriceYearly is what StripePriceIDYearly charges. Nil means the plan is
	// monthly only, so nothing may offer it yearly.
	PriceYearly     *float32 `json:"price_yearly,omitempty"`
	DiscountedPrice float32  `json:"discounted_price"`
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
	// ReferralRewardPercent is the share of this plan's month-equivalent
	// price a referrer earns when an invitee converts to it; 100 is a full
	// month.
	ReferralRewardPercent int `json:"referral_reward_percent"`

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

// Plans returns the public plan catalog. On a deployment without a billing
// provider ([AuthConfig.BillingEnabled] false) the route is absent.
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
	// ProxyPath is the same ticket as a path on the API origin
	// ("/realtime/socket/websocket?token=..."), for a browser whose content
	// security policy only permits connections to the API host. Dial it against
	// the API's own host instead of URL. Empty when the server has no
	// websocket endpoint configured.
	ProxyPath string `json:"proxy_path,omitempty"`
}

// GatewayTicket mints a single-use websocket ticket for the current session.
// This route is session-only.
func (s *MetaService) GatewayTicket(ctx context.Context, opts ...RequestOption) (*GatewayTicket, *Response, error) {
	return send[GatewayTicket](ctx, s.client.post, "getaway", nil, opts)
}

// Dashboard lists whose layout a member can save, for
// [MetaService.ViewPreferences]. An unknown view fails with a 400.
const (
	ViewContacts      = "contacts"
	ViewCampaignLeads = "campaign_leads"
	// ViewUniboxRail is the unibox scope rail. It has no columns or sort; its
	// layout is a JSON document in [ViewPreferences.Layout].
	ViewUniboxRail = "unibox_rail"
)

// ViewSort is the saved ordering of a list, in the contacts search's terms.
type ViewSort struct {
	// By is a contact search sort key or "custom:<field>"; an empty By in an
	// update selects the default sort.
	By      string `json:"by"`
	Reverse bool   `json:"reverse"`
}

// ViewPreferences is one member's saved layout for one list in one workspace.
// An empty Columns means the default layout and a nil Sort the default sort.
type ViewPreferences struct {
	// View is one of the View* constants.
	View    string    `json:"view"`
	Columns []string  `json:"columns"`
	Sort    *ViewSort `json:"sort,omitempty"`
	// Layout is the saved document of a view that keeps one
	// ([ViewUniboxRail]); absent when none is saved.
	Layout    json.RawMessage `json:"layout,omitempty"`
	UpdatedAt *time.Time      `json:"updated_at,omitempty"`
}

// ViewPreferencesParams is a partial write of a saved layout: a nil field
// keeps what is saved, so a sort click made before the columns have loaded
// cannot erase them.
type ViewPreferencesParams struct {
	// Columns is the ordered column ids: built-in ids for the list, or
	// "custom:<field>". A pointer to an empty slice restores the default
	// layout; at most 64 columns, each at most 300 characters.
	Columns *[]string `json:"columns,omitempty"`
	// Sort is the saved ordering; a Sort with an empty By restores the default.
	Sort *ViewSort `json:"sort,omitempty"`
	// Layout replaces a layout document; pass json.RawMessage("null") to
	// restore the default of a view that has one.
	Layout json.RawMessage `json:"layout,omitempty"`
}

// ViewPreferences returns the caller's saved layout for one dashboard list in
// the session's workspace, or the empty layout when none is saved. The
// layout is per member and per workspace; it is session-only and needs a
// workspace on the session.
func (s *MetaService) ViewPreferences(ctx context.Context, view string, opts ...RequestOption) (*ViewPreferences, *Response, error) {
	var out struct {
		Preferences *ViewPreferences `json:"preferences"`
	}
	resp, err := s.client.get(ctx, "me/views/"+url.PathEscape(view), &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Preferences, resp, nil
}

// UpdateViewPreferences writes the caller's saved layout for one list: the
// columns, the sort, the layout, or any of them, and returns what is saved. A
// column id that is not valid for the list fails with "invalid_column", a
// repeated one with "duplicate_column" and too many with "too_many_columns".
func (s *MetaService) UpdateViewPreferences(ctx context.Context, view string, params *ViewPreferencesParams, opts ...RequestOption) (*ViewPreferences, *Response, error) {
	var out struct {
		Preferences *ViewPreferences `json:"preferences"`
	}
	resp, err := s.client.put(ctx, "me/views/"+url.PathEscape(view), params, &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Preferences, resp, nil
}

// ResetViewPreferences forgets the caller's saved layout for one list.
func (s *MetaService) ResetViewPreferences(ctx context.Context, view string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "me/views/"+url.PathEscape(view), opts...)
}
