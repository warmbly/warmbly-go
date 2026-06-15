package warmbly

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"
)

// WebhookService manages webhook endpoints and event subscriptions.
//
// Each endpoint receives signed HTTP POST callbacks for the event types it
// subscribes to. Verify the [WebhookSignatureHeader] on every delivery with
// [VerifyWebhookSignature] (or, more conveniently, [WebhookService.ConstructEvent])
// before trusting the payload.
type WebhookService service

// WebhookSignatureHeader is the HTTP header carrying the delivery signature.
// Its value has the form "sha256=<lowercase-hex>", an HMAC-SHA256 computed over
// the raw request body using the endpoint's secret.
const WebhookSignatureHeader = "X-Webhook-Signature"

// ErrInvalidWebhookSignature is returned by [WebhookService.ConstructEvent] when
// the delivery signature does not match the computed HMAC.
var ErrInvalidWebhookSignature = errors.New("warmbly: invalid webhook signature")

// Webhook is a webhook endpoint as returned by the API.
type Webhook struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	// URL is the HTTPS endpoint that receives event deliveries.
	URL string `json:"url"`
	// Secret is the signing secret used to verify deliveries. It is only present
	// on the responses to [WebhookService.Create] and
	// [WebhookService.RotateSecret]; store it securely.
	Secret string `json:"secret,omitempty"`
	// EventTypes are the event keys this endpoint subscribes to.
	EventTypes []string `json:"event_types"`
	// Disabled reports whether deliveries to this endpoint are suspended.
	Disabled  bool      `json:"disabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WebhookEventType describes an event type that endpoints can subscribe to.
type WebhookEventType struct {
	// Key is the stable event identifier, e.g. "campaign.reply_received".
	Key string `json:"key"`
	// Description is a human-readable explanation of when the event fires.
	Description string `json:"description"`
	// Group is the optional category the event belongs to, e.g. "campaign".
	Group string `json:"group,omitempty"`
	// HighVolume flags events that fire frequently and may dominate delivery
	// traffic if subscribed to.
	HighVolume bool `json:"high_volume"`
}

// WebhookDelivery is a single delivery attempt of an event to an endpoint.
type WebhookDelivery struct {
	ID string `json:"id"`
	// EventType is the event key that was delivered.
	EventType string `json:"event_type"`
	// Status is the delivery outcome, e.g. "succeeded", "failed" or "pending".
	Status string `json:"status"`
	// ResponseStatus is the HTTP status code returned by the endpoint, or 0 if
	// the request never completed.
	ResponseStatus int `json:"response_status"`
	// Attempt is the 1-based attempt number for this delivery.
	Attempt   int       `json:"attempt"`
	CreatedAt time.Time `json:"created_at"`
}

// WebhookEvent is the decoded body of a webhook delivery, as produced by
// [WebhookService.ConstructEvent]. Inspect Event to dispatch and unmarshal Data
// into the concrete payload for that event type.
type WebhookEvent struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	// Event is the event type key, e.g. "campaign.reply_received". Compare it
	// against the [WebhookEventName] constants.
	Event          string `json:"event"`
	OrganizationID string `json:"organization_id"`
	// Data is the raw, event-specific payload. Unmarshal it into the appropriate
	// type once Event is known.
	Data json.RawMessage `json:"data"`
	// Attempt is the 1-based delivery attempt number.
	Attempt int `json:"attempt"`
	// MaxAttempts is the maximum number of delivery attempts that will be made.
	MaxAttempts int `json:"max_attempts"`
}

// WebhookEventName is a stable webhook event type key. The exported constants
// cover the most common events; the authoritative, complete list is available
// from [WebhookService.EventTypes].
type WebhookEventName string

// Known webhook event type keys. These are convenience constants for
// subscribing to and dispatching on events; the live catalog from
// [WebhookService.EventTypes] is authoritative.
const (
	// EventEmailAccountConnected fires when an email account finishes connecting.
	EventEmailAccountConnected WebhookEventName = "email_account.connected"
	// EventEmailAccountDisconnected fires when an email account is disconnected.
	EventEmailAccountDisconnected WebhookEventName = "email_account.disconnected"

	// EventCampaignStarted fires when a campaign begins sending.
	EventCampaignStarted WebhookEventName = "campaign.started"
	// EventCampaignCompleted fires when a campaign finishes all of its steps.
	EventCampaignCompleted WebhookEventName = "campaign.completed"
	// EventCampaignReplyReceived fires when a contact replies to a campaign email.
	EventCampaignReplyReceived WebhookEventName = "campaign.reply_received"
	// EventCampaignEmailOpened fires when a campaign email is opened.
	EventCampaignEmailOpened WebhookEventName = "campaign.email_opened"
	// EventCampaignEmailClicked fires when a link in a campaign email is clicked.
	EventCampaignEmailClicked WebhookEventName = "campaign.email_clicked"
	// EventCampaignEmailBounced fires when a campaign email bounces.
	EventCampaignEmailBounced WebhookEventName = "campaign.email_bounced"
	// EventCampaignUnsubscribed fires when a contact unsubscribes from a campaign.
	EventCampaignUnsubscribed WebhookEventName = "campaign.unsubscribed"

	// EventWarmupHealthChanged fires when a mailbox's warmup health score changes.
	EventWarmupHealthChanged WebhookEventName = "warmup.health_changed"
	// EventWarmupBlocked fires when a warming mailbox is blocked by a provider.
	EventWarmupBlocked WebhookEventName = "warmup.blocked"
	// EventWarmupPlacementInSpam fires when a warmup message lands in spam.
	EventWarmupPlacementInSpam WebhookEventName = "warmup.placement_in_spam"

	// EventContactCreated fires when a contact is created.
	EventContactCreated WebhookEventName = "contact.created"
	// EventContactUpdated fires when a contact is updated.
	EventContactUpdated WebhookEventName = "contact.updated"
	// EventContactDeleted fires when a contact is deleted.
	EventContactDeleted WebhookEventName = "contact.deleted"

	// EventInboxReplyReceived fires when any reply lands in a connected inbox.
	EventInboxReplyReceived WebhookEventName = "inbox.reply_received"
	// EventMeetingBooked fires when a meeting is booked from outreach.
	EventMeetingBooked WebhookEventName = "meeting.booked"
	// EventCustom is the catch-all key for user-defined custom events.
	EventCustom WebhookEventName = "custom.event"
)

// WebhookCreateParams are the parameters for creating a webhook endpoint.
type WebhookCreateParams struct {
	// URL is the HTTPS endpoint that will receive event deliveries.
	URL string `json:"url"`
	// EventTypes are the event keys to subscribe to.
	EventTypes []string `json:"event_types"`
	// Description is an optional human-readable note for the endpoint.
	Description string `json:"description,omitempty"`
}

// WebhookUpdateParams are the parameters for updating a webhook endpoint. Only
// non-nil fields are sent, so zero values are not mistaken for "clear this
// field".
type WebhookUpdateParams struct {
	URL        *string   `json:"url,omitempty"`
	EventTypes *[]string `json:"event_types,omitempty"`
	Disabled   *bool     `json:"disabled,omitempty"`
}

// WebhookListParams paginates a list of webhook endpoints.
type WebhookListParams struct {
	ListOptions
}

func (p *WebhookListParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.ListOptions.apply(q)
	return q
}

// List returns a page of webhook endpoints.
func (s *WebhookService) List(ctx context.Context, params *WebhookListParams) (*Page[Webhook], error) {
	return listJSON[Webhook](ctx, s.client, "webhooks", params.values())
}

// Get retrieves a single webhook endpoint by ID.
func (s *WebhookService) Get(ctx context.Context, id string) (*Webhook, *Response, error) {
	hook := new(Webhook)
	resp, err := s.client.get(ctx, "webhooks/"+url.PathEscape(id), hook)
	if err != nil {
		return nil, resp, err
	}
	return hook, resp, nil
}

// Create registers a new webhook endpoint. The returned [Webhook] includes the
// signing Secret, which is only available at creation time; store it securely.
func (s *WebhookService) Create(ctx context.Context, params *WebhookCreateParams) (*Webhook, *Response, error) {
	hook := new(Webhook)
	resp, err := s.client.post(ctx, "webhooks", params, hook)
	if err != nil {
		return nil, resp, err
	}
	return hook, resp, nil
}

// Update modifies an existing webhook endpoint.
func (s *WebhookService) Update(ctx context.Context, id string, params *WebhookUpdateParams) (*Webhook, *Response, error) {
	hook := new(Webhook)
	resp, err := s.client.patch(ctx, "webhooks/"+url.PathEscape(id), params, hook)
	if err != nil {
		return nil, resp, err
	}
	return hook, resp, nil
}

// Delete permanently removes a webhook endpoint.
func (s *WebhookService) Delete(ctx context.Context, id string) (*Response, error) {
	return s.client.delete(ctx, "webhooks/"+url.PathEscape(id), nil)
}

// RotateSecret generates a new signing secret for the endpoint and returns the
// updated [Webhook]. The new Secret is only available on this response; store
// it securely. The previous secret is invalidated.
func (s *WebhookService) RotateSecret(ctx context.Context, id string) (*Webhook, *Response, error) {
	hook := new(Webhook)
	resp, err := s.client.post(ctx, "webhooks/"+url.PathEscape(id)+"/rotate-secret", nil, hook)
	if err != nil {
		return nil, resp, err
	}
	return hook, resp, nil
}

// Verify sends a test event to the endpoint to confirm it is reachable and
// correctly verifying signatures.
func (s *WebhookService) Verify(ctx context.Context, id string) (*Response, error) {
	return s.client.post(ctx, "webhooks/"+url.PathEscape(id)+"/verify", nil, nil)
}

// EventTypes lists the event types that endpoints can subscribe to.
func (s *WebhookService) EventTypes(ctx context.Context) ([]WebhookEventType, *Response, error) {
	var out struct {
		Data []WebhookEventType `json:"data"`
	}
	resp, err := s.client.get(ctx, "webhooks/event-types", &out)
	if err != nil {
		return nil, resp, err
	}
	return out.Data, resp, nil
}

// Deliveries returns a page of recent delivery attempts for an endpoint.
func (s *WebhookService) Deliveries(ctx context.Context, id string, params *ListOptions) (*Page[WebhookDelivery], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[WebhookDelivery](ctx, s.client, "webhooks/"+url.PathEscape(id)+"/deliveries", q)
}

// ConstructEvent verifies the signature on a webhook delivery and decodes the
// payload into a [*WebhookEvent]. It returns [ErrInvalidWebhookSignature] when
// the signature does not match secret, and otherwise reports any JSON decoding
// error. Pass the raw, unmodified request body as payload and the value of the
// [WebhookSignatureHeader] header as signatureHeader.
func (s *WebhookService) ConstructEvent(payload []byte, signatureHeader, secret string) (*WebhookEvent, error) {
	if !VerifyWebhookSignature(payload, signatureHeader, secret) {
		return nil, ErrInvalidWebhookSignature
	}
	event := new(WebhookEvent)
	if err := json.Unmarshal(payload, event); err != nil {
		return nil, err
	}
	return event, nil
}

// ComputeWebhookSignature returns the expected value of the
// [WebhookSignatureHeader] for the given raw payload and signing secret: the
// string "sha256=" followed by the lowercase hex HMAC-SHA256 of payload keyed
// by secret.
func ComputeWebhookSignature(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// VerifyWebhookSignature reports whether signatureHeader is a valid signature
// for payload under secret. The comparison is constant-time. A leading
// "sha256=" prefix on signatureHeader is optional and tolerated. It returns
// false for an empty or malformed signature.
func VerifyWebhookSignature(payload []byte, signatureHeader, secret string) bool {
	if signatureHeader == "" {
		return false
	}
	got := strings.TrimPrefix(signatureHeader, "sha256=")
	want := strings.TrimPrefix(ComputeWebhookSignature(payload, secret), "sha256=")
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
