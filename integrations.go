package warmbly

import (
	"context"
	"encoding/json"
	"net/url"
	"time"
)

// IntegrationService manages third-party connections: the provider catalog,
// connected accounts, the events each connection reacts to, field mappings,
// sync history and on-demand contact pushes.
//
// Connecting an OAuth provider is a browser flow. Start it with
// [IntegrationService.StartOAuth], send the user to the returned URL, and
// finish it with [IntegrationService.FinishOAuth] once the popup posts the code
// back. Those two calls are session-only.
type IntegrationService service

// Providers available in the integration catalog.
const (
	ProviderHubSpot      = "hubspot"
	ProviderSalesforce   = "salesforce"
	ProviderPipedrive    = "pipedrive"
	ProviderClose        = "close"
	ProviderZapier       = "zapier"
	ProviderMake         = "make"
	ProviderN8N          = "n8n"
	ProviderSlack        = "slack"
	ProviderDiscord      = "discord"
	ProviderCalendly     = "calendly"
	ProviderCalCom       = "cal_com"
	ProviderGoogleSheets = "google_sheets"
)

// Connection states returned in [IntegrationConnection.Status].
const (
	ConnectionPending        = "pending"
	ConnectionAuthorizing    = "authorizing"
	ConnectionConnected      = "connected"
	ConnectionDegraded       = "degraded"
	ConnectionReauthRequired = "reauth_required"
	ConnectionDisconnected   = "disconnected"
)

// Data-flow directions for [IntegrationConnection.SyncDirection].
const (
	SyncPush = "push"
	SyncPull = "pull"
	SyncBoth = "both"
)

// IntegrationCatalogEntry describes one connectable provider.
type IntegrationCatalogEntry struct {
	// Provider is one of the Provider* constants.
	Provider string `json:"provider"`
	Name     string `json:"name"`
	Tagline  string `json:"tagline,omitempty"`
	// Category groups the provider: "crm", "automation", "notifications",
	// "meetings" or "data".
	Category string `json:"category,omitempty"`
	DocsURL  string `json:"docs_url,omitempty"`
	// AuthMethod is "oauth", "api_key" or "webhook".
	AuthMethod string `json:"auth_method"`
	BadgeColor string `json:"badge_color,omitempty"`
	Beta       bool   `json:"beta"`
	// WebhookHint is shown for webhook-URL and inbound providers.
	WebhookHint string `json:"webhook_hint,omitempty"`
	// Highlights are short "what you get" bullets.
	Highlights []string `json:"highlights,omitempty"`
	// Scopes are the OAuth scopes requested at authorize time.
	Scopes []string `json:"scopes,omitempty"`
	// Events are the Warmbly events this provider can react to, and
	// ActionTypes the actions it can actually perform.
	Events      []string `json:"events,omitempty"`
	ActionTypes []string `json:"action_types,omitempty"`
	// SupportsPush reports whether contacts can be pushed to this provider on
	// demand.
	SupportsPush bool `json:"supports_push"`
	// Configured reports whether this deployment has credentials for the
	// provider, so an unconfigured one can be greyed out rather than failing
	// at connect time.
	Configured bool `json:"configured"`
	// Capability is the provider's raw capability descriptor, when it has one.
	Capability json.RawMessage `json:"capability,omitempty"`
}

// IntegrationConnection is one connected third-party account.
type IntegrationConnection struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	// Provider is one of the Provider* constants.
	Provider string `json:"provider"`
	Label    string `json:"label"`
	// Status is one of the Connection* constants.
	Status string `json:"status"`
	// AuthMethod is "oauth", "api_key" or "webhook".
	AuthMethod string `json:"auth_method"`
	// DisplayFields are the non-secret connection details worth showing.
	DisplayFields json.RawMessage `json:"display_fields,omitempty"`
	// ConfigCapabilities is the per-connection capability snapshot: selected
	// objects, enabled use cases, picker selections. It never holds secrets.
	ConfigCapabilities json.RawMessage `json:"config_capabilities,omitempty"`
	// SyncDirection is [SyncPush], [SyncPull] or [SyncBoth].
	SyncDirection string `json:"sync_direction"`

	ConnectedByUserID   *string    `json:"connected_by_user_id,omitempty"`
	ExternalAccountID   string     `json:"external_account_id,omitempty"`
	ExternalAccountName string     `json:"external_account_name,omitempty"`
	GrantedScopes       []string   `json:"granted_scopes,omitempty"`
	TokenExpiresAt      *time.Time `json:"token_expires_at,omitempty"`

	// Health is "unknown", "healthy", "degraded" or "down".
	Health          string     `json:"health"`
	HealthDetail    *string    `json:"health_detail,omitempty"`
	HealthCheckedAt *time.Time `json:"health_checked_at,omitempty"`

	LastSyncedAt *time.Time `json:"last_synced_at,omitempty"`
	LastError    *string    `json:"last_error,omitempty"`
	LastErrorAt  *time.Time `json:"last_error_at,omitempty"`

	// InboundWebhookURL is where an inbound provider should POST. It embeds a
	// rotatable per-connection secret, so treat it as a credential.
	InboundWebhookURL string `json:"inbound_webhook_url,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ConnectionDetail is a connection together with its event subscriptions and
// recent sync runs.
type ConnectionDetail struct {
	Connection *IntegrationConnection `json:"connection"`
	Events     []EventSubscription    `json:"events"`
	Runs       []SyncRun              `json:"runs"`
}

// EventSubscription binds a Warmbly event to an action on a connection.
type EventSubscription struct {
	ID             string `json:"id"`
	ConnectionID   string `json:"connection_id"`
	OrganizationID string `json:"organization_id,omitempty"`
	// EventType is a webhook event key; see the Event* constants.
	EventType string `json:"event_type"`
	// Action is a provider action identifier from
	// [IntegrationCatalogEntry.ActionTypes].
	Action  string          `json:"action"`
	Config  json.RawMessage `json:"config,omitempty"`
	Enabled bool            `json:"enabled"`
	UseCase string          `json:"use_case,omitempty"`
	// AutomationID is set when the subscription is backed by an automation
	// flow rather than a single action.
	AutomationID *string   `json:"automation_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// EventSubscriptionParams creates an event subscription.
type EventSubscriptionParams struct {
	EventType string          `json:"event_type"`
	Action    string          `json:"action"`
	Config    json.RawMessage `json:"config,omitempty"`
	Enabled   *bool           `json:"enabled,omitempty"`
}

// FieldMapping maps a Warmbly field to a field on the provider.
type FieldMapping struct {
	ID             string  `json:"id"`
	ConnectionID   string  `json:"connection_id"`
	OrganizationID string  `json:"organization_id,omitempty"`
	SubscriptionID *string `json:"subscription_id,omitempty"`
	// Direction is [SyncPush], [SyncPull] or [SyncBoth].
	Direction string `json:"direction,omitempty"`
	// ObjectName is the provider object the mapping applies to, for example
	// "contact".
	ObjectName    string `json:"object_name"`
	WarmblyField  string `json:"warmbly_field"`
	ExternalField string `json:"external_field"`
	// Transform names an optional value transformation.
	Transform string `json:"transform,omitempty"`
	// StaticValue writes a constant instead of reading a Warmbly field.
	StaticValue string `json:"static_value,omitempty"`
	// IsDefault marks a mapping the platform supplied rather than the user.
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
}

// FieldMappingInput is one mapping in a wholesale replacement.
type FieldMappingInput struct {
	ExternalField string `json:"external_field"`
	WarmblyField  string `json:"warmbly_field,omitempty"`
	Transform     string `json:"transform,omitempty"`
	StaticValue   string `json:"static_value,omitempty"`
}

// SyncRun is one execution of a connection's sync.
type SyncRun struct {
	ID             string `json:"id"`
	ConnectionID   string `json:"connection_id"`
	OrganizationID string `json:"organization_id,omitempty"`
	// Kind is what ran, for example "push" or "pull".
	Kind string `json:"kind"`
	// Status is the outcome, for example "success" or "error".
	Status           string     `json:"status"`
	Detail           string     `json:"detail,omitempty"`
	RecordsProcessed int        `json:"records_processed"`
	StartedAt        time.Time  `json:"started_at"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
}

// ConnectionWebhookSecret is what an inbound provider needs to sign its
// callbacks to Warmbly.
type ConnectionWebhookSecret struct {
	SigningSecret   string `json:"signing_secret"`
	SignatureHeader string `json:"signature_header"`
	// Scheme names the signing algorithm the provider should use.
	Scheme string `json:"scheme"`
}

// PushResult reports the outcome of pushing contacts to a provider.
type PushResult struct {
	Provider string              `json:"provider"`
	Pushed   int                 `json:"pushed"`
	Failed   int                 `json:"failed"`
	Results  []PushContactResult `json:"results"`
}

// PushContactResult is one contact's outcome in a push.
type PushContactResult struct {
	ContactID string `json:"contact_id"`
	Email     string `json:"email,omitempty"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
}

// ConnectParams connects a provider that authenticates with an API key or a
// webhook URL. Use [IntegrationService.StartOAuth] for OAuth providers.
type ConnectParams struct {
	// Provider is one of the Provider* constants.
	Provider string `json:"provider"`
	Label    string `json:"label,omitempty"`
	// Config carries the provider's credentials, which are sealed server-side.
	Config map[string]any `json:"config,omitempty"`
}

// ConnectionConfigParams updates a connection's non-secret configuration.
type ConnectionConfigParams struct {
	ConfigCapabilities json.RawMessage `json:"config_capabilities,omitempty"`
	// SyncDirection is [SyncPush], [SyncPull] or [SyncBoth].
	SyncDirection string `json:"sync_direction,omitempty"`
}

// OAuthStartResult is the provider consent URL plus the state value that ties
// the round trip together.
type OAuthStartResult struct {
	URL   string `json:"url"`
	State string `json:"state"`
}

// Catalog returns every connectable provider.
func (s *IntegrationService) Catalog(ctx context.Context, opts ...RequestOption) ([]IntegrationCatalogEntry, *Response, error) {
	var out struct {
		Catalog []IntegrationCatalogEntry `json:"catalog"`
	}
	resp, err := s.client.get(ctx, "integrations/catalog", &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Catalog, resp, nil
}

// Connections returns the workspace's connected accounts.
func (s *IntegrationService) Connections(ctx context.Context, opts ...RequestOption) ([]IntegrationConnection, *Response, error) {
	var out struct {
		Connections []IntegrationConnection `json:"connections"`
	}
	resp, err := s.client.get(ctx, "integrations/connections", &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Connections, resp, nil
}

// Connect creates a connection for an API-key or webhook provider.
func (s *IntegrationService) Connect(ctx context.Context, params *ConnectParams, opts ...RequestOption) (*IntegrationConnection, *Response, error) {
	return send[IntegrationConnection](ctx, s.client.post, "integrations/connections", params, opts)
}

// Connection returns one connection with its event subscriptions and recent
// sync runs.
func (s *IntegrationService) Connection(ctx context.Context, id string, opts ...RequestOption) (*ConnectionDetail, *Response, error) {
	return fetch[ConnectionDetail](ctx, s.client, "integrations/connections/"+url.PathEscape(id), opts)
}

// UpdateConfig changes a connection's non-secret configuration.
func (s *IntegrationService) UpdateConfig(ctx context.Context, id string, params *ConnectionConfigParams, opts ...RequestOption) (*IntegrationConnection, *Response, error) {
	var out struct {
		Connection *IntegrationConnection `json:"connection"`
	}
	resp, err := s.client.patch(ctx, "integrations/connections/"+url.PathEscape(id)+"/config", params, &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Connection, resp, nil
}

// Disconnect removes a connection and its stored credentials.
func (s *IntegrationService) Disconnect(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "integrations/connections/"+url.PathEscape(id), opts...)
}

// Events returns a connection's event subscriptions.
func (s *IntegrationService) Events(ctx context.Context, id string, opts ...RequestOption) ([]EventSubscription, *Response, error) {
	var out struct {
		Events []EventSubscription `json:"events"`
	}
	resp, err := s.client.get(ctx, "integrations/connections/"+url.PathEscape(id)+"/events", &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Events, resp, nil
}

// CreateEvent subscribes a connection to a Warmbly event.
func (s *IntegrationService) CreateEvent(ctx context.Context, id string, params *EventSubscriptionParams, opts ...RequestOption) (*EventSubscription, *Response, error) {
	return send[EventSubscription](ctx, s.client.post, "integrations/connections/"+url.PathEscape(id)+"/events", params, opts)
}

// DeleteEvent removes an event subscription.
func (s *IntegrationService) DeleteEvent(ctx context.Context, id, eventID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "integrations/connections/"+url.PathEscape(id)+"/events/"+url.PathEscape(eventID), opts...)
}

// FieldMappings returns a connection's field mappings.
func (s *IntegrationService) FieldMappings(ctx context.Context, id string, opts ...RequestOption) ([]FieldMapping, *Response, error) {
	var out struct {
		Mappings []FieldMapping `json:"mappings"`
	}
	resp, err := s.client.get(ctx, "integrations/connections/"+url.PathEscape(id)+"/field-mappings", &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Mappings, resp, nil
}

// ReplaceFieldMappings replaces the mappings for one provider object
// wholesale.
func (s *IntegrationService) ReplaceFieldMappings(ctx context.Context, id, object string, mappings []FieldMappingInput, opts ...RequestOption) ([]FieldMapping, *Response, error) {
	body := struct {
		Object   string              `json:"object,omitempty"`
		Mappings []FieldMappingInput `json:"mappings"`
	}{Object: object, Mappings: mappings}
	var out struct {
		Mappings []FieldMapping `json:"mappings"`
	}
	resp, err := s.client.put(ctx, "integrations/connections/"+url.PathEscape(id)+"/field-mappings", body, &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Mappings, resp, nil
}

// Runs returns a connection's recent sync runs.
func (s *IntegrationService) Runs(ctx context.Context, id string, opts ...RequestOption) ([]SyncRun, *Response, error) {
	var out struct {
		Runs []SyncRun `json:"runs"`
	}
	resp, err := s.client.get(ctx, "integrations/connections/"+url.PathEscape(id)+"/runs", &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Runs, resp, nil
}

// WebhookSecret returns the signing material an inbound provider needs. Treat
// the secret as a credential.
func (s *IntegrationService) WebhookSecret(ctx context.Context, id string, opts ...RequestOption) (*ConnectionWebhookSecret, *Response, error) {
	return fetch[ConnectionWebhookSecret](ctx, s.client, "integrations/connections/"+url.PathEscape(id)+"/webhook-secret", opts)
}

// Test sends a test message through the connection to confirm it works. For a
// notification provider this posts a real message to the configured channel.
func (s *IntegrationService) Test(ctx context.Context, id string, opts ...RequestOption) (bool, *Response, error) {
	var out struct {
		Sent bool `json:"sent"`
	}
	resp, err := s.client.post(ctx, "integrations/connections/"+url.PathEscape(id)+"/test", nil, &out, opts...)
	if err != nil {
		return false, resp, err
	}
	return out.Sent, resp, nil
}

// Push sends the given contacts to the provider now, rather than waiting for
// an event to fire.
func (s *IntegrationService) Push(ctx context.Context, id string, contactIDs []string, opts ...RequestOption) (*PushResult, *Response, error) {
	body := struct {
		ContactIDs []string `json:"contact_ids"`
	}{ContactIDs: contactIDs}
	return send[PushResult](ctx, s.client.post, "integrations/connections/"+url.PathEscape(id)+"/push", body, opts)
}

// --- OAuth connect flow (session-only) ---

// StartOAuth begins the consent flow for an OAuth provider and returns the URL
// to send the user to. The label, if given, names the resulting connection.
func (s *IntegrationService) StartOAuth(ctx context.Context, provider, label string, opts ...RequestOption) (*OAuthStartResult, *Response, error) {
	body := struct {
		Provider string `json:"provider"`
		Label    string `json:"label,omitempty"`
	}{Provider: provider, Label: label}
	return send[OAuthStartResult](ctx, s.client.post, "integrations/oauth/start", body, opts)
}

// FinishOAuth exchanges the authorization code the provider redirected back
// with for a stored connection.
func (s *IntegrationService) FinishOAuth(ctx context.Context, code, state string, opts ...RequestOption) (*IntegrationConnection, *Response, error) {
	body := struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}{Code: code, State: state}
	return send[IntegrationConnection](ctx, s.client.post, "integrations/oauth/finish", body, opts)
}

// ReauthOAuth restarts consent for a connection whose token expired or was
// revoked, returning a fresh URL to send the user to.
func (s *IntegrationService) ReauthOAuth(ctx context.Context, id string, opts ...RequestOption) (*OAuthStartResult, *Response, error) {
	return send[OAuthStartResult](ctx, s.client.post, "integrations/oauth/reauth/"+url.PathEscape(id), nil, opts)
}

// --- meetings ---

// MeetingService lists meetings booked through a connected scheduling provider
// such as Calendly or Cal.com, and lets one be logged by hand.
type MeetingService service

// Meeting lifecycle states returned in [Meeting.Status].
const (
	MeetingBooked      = "booked"
	MeetingRescheduled = "rescheduled"
	MeetingCanceled    = "canceled"
	MeetingCompleted   = "completed"
	MeetingNoShow      = "no_show"
)

// Meeting is one booked call.
type Meeting struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	// Source is the scheduling provider, or "manual" for one logged by hand.
	Source string `json:"source"`
	// ExternalEventID is the provider's identifier for the booking.
	ExternalEventID string `json:"external_event_id,omitempty"`
	// Status is one of the Meeting* constants.
	Status       string `json:"status"`
	InviteeEmail string `json:"invitee_email"`
	InviteeName  string `json:"invitee_name"`
	EventName    string `json:"event_name"`
	EventType    string `json:"event_type,omitempty"`

	ScheduledFor *time.Time `json:"scheduled_for,omitempty"`
	EndTime      *time.Time `json:"end_time,omitempty"`

	JoinURL       string `json:"join_url,omitempty"`
	Location      string `json:"location,omitempty"`
	CancelURL     string `json:"cancel_url,omitempty"`
	RescheduleURL string `json:"reschedule_url,omitempty"`
	// CanceledReason is set once the booking is canceled.
	CanceledReason string `json:"canceled_reason,omitempty"`

	ContactID  *string `json:"contact_id,omitempty"`
	CampaignID *string `json:"campaign_id,omitempty"`
	// ContactName is joined in for display.
	ContactName string `json:"contact_name,omitempty"`
	// RawPayload is the provider's original webhook body.
	RawPayload json.RawMessage `json:"raw_payload,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MeetingsSummary counts booked meetings for a header row.
type MeetingsSummary struct {
	Upcoming int `json:"upcoming"`
	Today    int `json:"today"`
	Total    int `json:"total"`
	Canceled int `json:"canceled"`
}

// Meeting timeframes accepted by [MeetingListParams.Timeframe].
const (
	MeetingsUpcoming = "upcoming"
	MeetingsPast     = "past"
)

// MeetingListParams filters and paginates the meetings list.
type MeetingListParams struct {
	ListOptions
	// Timeframe is [MeetingsUpcoming] or [MeetingsPast]. Empty returns both.
	Timeframe string
	// Status is one of the Meeting* constants.
	Status string
	// Query is a free-text search over the invitee and the event name.
	Query string
}

func (p *MeetingListParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	setNonEmpty(q, "timeframe", p.Timeframe)
	setNonEmpty(q, "status", p.Status)
	setNonEmpty(q, "q", p.Query)
	return q
}

// MeetingCreateParams logs a meeting that was booked outside a connected
// provider. It is matched to a contact by InviteeEmail unless ContactID says
// otherwise.
type MeetingCreateParams struct {
	Title        string `json:"title,omitempty"`
	InviteeName  string `json:"invitee_name,omitempty"`
	InviteeEmail string `json:"invitee_email"`
	// ScheduledFor is an RFC 3339 timestamp.
	ScheduledFor    string `json:"scheduled_for,omitempty"`
	DurationMinutes int    `json:"duration_minutes,omitempty"`
	Location        string `json:"location,omitempty"`
	JoinURL         string `json:"join_url,omitempty"`
	ContactID       string `json:"contact_id,omitempty"`
}

// List returns a page of booked meetings.
func (s *MeetingService) List(ctx context.Context, params *MeetingListParams, opts ...RequestOption) (*Page[Meeting], error) {
	return listJSON[Meeting](ctx, s.client, "meetings", params.values(), opts...)
}

// Summary returns the meeting counters.
func (s *MeetingService) Summary(ctx context.Context, opts ...RequestOption) (*MeetingsSummary, *Response, error) {
	return fetch[MeetingsSummary](ctx, s.client, "meetings/summary", opts)
}

// Create logs a meeting booked outside a connected provider.
func (s *MeetingService) Create(ctx context.Context, params *MeetingCreateParams, opts ...RequestOption) (*Meeting, *Response, error) {
	return send[Meeting](ctx, s.client.post, "meetings", params, opts)
}

// Delete removes a meeting record.
func (s *MeetingService) Delete(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "meetings/"+url.PathEscape(id), opts...)
}

// Bookings returns meetings from connected scheduling providers. It is the
// integration-scoped view of the same data [MeetingService.List] returns.
func (s *IntegrationService) Bookings(ctx context.Context, opts ...RequestOption) ([]Meeting, *Response, error) {
	var out struct {
		Bookings []Meeting `json:"bookings"`
	}
	resp, err := s.client.get(ctx, "integrations/bookings", &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Bookings, resp, nil
}
