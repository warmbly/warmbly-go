package warmbly

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// WebhookService manages webhook endpoints and their delivery log.
//
// Each endpoint receives signed HTTP POSTs for the event types it subscribes
// to. Verify the [WebhookSignatureHeader] on every delivery with
// [VerifyWebhookSignature] — or, more conveniently,
// [WebhookService.ConstructEvent] — before trusting the payload.
//
// A new endpoint does not receive the real event stream until it has proved
// ownership of its URL: call [WebhookService.Verify], then echo the challenge
// value back from your handler.
type WebhookService service

// Headers set on every webhook delivery.
const (
	// WebhookSignatureHeader carries the signature as "t=<unix>,v1=<hex>",
	// where the hex digest is an HMAC-SHA256 over "<unix>." followed by the raw
	// request body, keyed by the endpoint's secret.
	WebhookSignatureHeader = "X-Warmbly-Signature"
	// WebhookEventHeader carries the delivered event type.
	WebhookEventHeader = "X-Warmbly-Event"
	// WebhookEventIDHeader carries the event's id, which is stable across
	// retries and so is the right key for receiver-side deduplication.
	WebhookEventIDHeader = "X-Warmbly-Event-Id"
	// WebhookChallengeHeader carries the ownership-verification challenge. Echo
	// its value back in your response body to confirm the endpoint.
	WebhookChallengeHeader = "X-Warmbly-Webhook-Challenge"
)

// Errors returned when a delivery fails verification.
var (
	// ErrInvalidWebhookSignature is returned when the delivery signature does
	// not match the computed HMAC.
	ErrInvalidWebhookSignature = errors.New("warmbly: invalid webhook signature")
	// ErrWebhookSignatureExpired is returned when the signature is valid but
	// its timestamp falls outside the tolerance, which defeats replay.
	ErrWebhookSignatureExpired = errors.New("warmbly: webhook signature timestamp outside tolerance")
)

// DefaultWebhookTolerance is how far a delivery's signature timestamp may drift
// from local time before [WebhookService.ConstructEvent] rejects it.
const DefaultWebhookTolerance = 5 * time.Minute

// Webhook is a webhook endpoint as returned by the API.
type Webhook struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	// URL is the HTTPS endpoint that receives deliveries.
	URL         string `json:"url"`
	Description string `json:"description"`
	// EventTypes are the event keys this endpoint subscribes to. An empty list
	// means every non-firehose event.
	EventTypes []string `json:"event_types"`
	// Enabled reports whether deliveries are being attempted.
	Enabled bool `json:"enabled"`

	LastSuccessAt     *time.Time `json:"last_success_at,omitempty"`
	LastFailureAt     *time.Time `json:"last_failure_at,omitempty"`
	LastFailureReason *string    `json:"last_failure_reason,omitempty"`
	// ConsecutiveFailures drives the auto-disable circuit breaker.
	ConsecutiveFailures int `json:"consecutive_failures"`

	// OAuthApplicationID is set for app-scoped endpoints, whose URL host must
	// stay inside the owning application's allowed webhook domains.
	OAuthApplicationID *string `json:"oauth_application_id,omitempty"`
	CreatedBy          *string `json:"created_by,omitempty"`

	// VerifiedAt is non-nil once the endpoint passed ownership verification.
	// Only verified endpoints receive the real event stream.
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
	// OwnershipConfirmed is true once the receiver echoed the challenge back,
	// which proves control of the URL rather than mere reachability.
	OwnershipConfirmed bool `json:"ownership_confirmed"`

	// AutoDisabledAt is set when sustained failures tripped the breaker.
	AutoDisabledAt *time.Time `json:"auto_disabled_at,omitempty"`
	DisabledReason *string    `json:"disabled_reason,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Verified reports whether the endpoint has passed ownership verification.
func (w *Webhook) Verified() bool { return w.VerifiedAt != nil }

// WebhookWithSecret is a newly created endpoint together with its signing
// secret, which is returned only at creation. Store it securely; afterwards it
// can only be replaced with [WebhookService.RotateSecret].
type WebhookWithSecret struct {
	Webhook
	Secret string `json:"secret"`
}

// WebhookEndpointList is the response from [WebhookService.List]: the
// organization's endpoints plus the full event catalog, so a picker can be
// rendered without a second call.
type WebhookEndpointList struct {
	Endpoints  []Webhook                `json:"endpoints"`
	EventTypes []WebhookEventDescriptor `json:"event_types"`
}

// Event categories used to group [WebhookEventDescriptor] entries.
const (
	WebhookCatCampaign       = "Campaign"
	WebhookCatWarmup         = "Warmup"
	WebhookCatDeliverability = "Deliverability"
	WebhookCatInbox          = "Inbox"
	WebhookCatEmailAccount   = "Mailbox"
	WebhookCatContact        = "Contact"
	WebhookCatCRM            = "CRM"
	WebhookCatAutomation     = "Automation"
	WebhookCatMeeting        = "Meeting"
	WebhookCatTeam           = "Team & access"
	WebhookCatBulk           = "Bulk operations"
	WebhookCatWorkspace      = "Workspace"
	WebhookCatDeveloper      = "Developer"
)

// WebhookEventDescriptor is one entry in the public event catalog.
type WebhookEventDescriptor struct {
	// Type is the event key, for example [EventCampaignReplyReceived].
	Type string `json:"type"`
	// Category groups the event for a picker; one of the WebhookCat* values.
	Category    string `json:"category"`
	Description string `json:"description"`
	// Firehose marks high-volume events. They are opt-in only: an endpoint
	// with an empty event filter does not receive them.
	Firehose bool `json:"firehose"`
}

// WebhookEventName is a stable webhook event key. The constants below are the
// full catalog at this SDK release; [WebhookService.EventTypes] is
// authoritative and stays current.
type WebhookEventName = string

// Webhook event keys.
const (
	// Mailbox lifecycle and health.
	EventEmailAccountConnected     WebhookEventName = "email_account.connected"
	EventEmailAccountRemoved       WebhookEventName = "email_account.removed"
	EventEmailAccountDisconnected  WebhookEventName = "email_account.disconnected"
	EventEmailAccountError         WebhookEventName = "email_account.error"
	EventEmailAccountSynced        WebhookEventName = "email_account.synced"
	EventEmailAccountHealthChanged WebhookEventName = "email_account.health_changed"

	// Campaign send pipeline.
	EventCampaignEmailSent      WebhookEventName = "campaign.email_sent"
	EventCampaignEmailDelivered WebhookEventName = "campaign.email_delivered"
	EventCampaignEmailOpened    WebhookEventName = "campaign.email_opened"
	EventCampaignEmailClicked   WebhookEventName = "campaign.email_clicked"
	EventCampaignEmailBounced   WebhookEventName = "campaign.email_bounced"
	EventCampaignReplyReceived  WebhookEventName = "campaign.reply_received"
	EventCampaignUnsubscribed   WebhookEventName = "campaign.unsubscribed"

	// Campaign lifecycle and configuration.
	EventCampaignStarted   WebhookEventName = "campaign.started"
	EventCampaignPaused    WebhookEventName = "campaign.paused"
	EventCampaignCompleted WebhookEventName = "campaign.completed"
	EventCampaignCreated   WebhookEventName = "campaign.created"
	EventCampaignUpdated   WebhookEventName = "campaign.updated"
	EventCampaignDeleted   WebhookEventName = "campaign.deleted"
	// EventCampaignDeliverabilityWarning fires when a campaign's rolling
	// bounce or complaint rate enters the early-warning band, short of an
	// auto-pause.
	EventCampaignDeliverabilityWarning WebhookEventName = "campaign.deliverability_warning"
	// EventCampaignAction fires from a notify action node in a sequence.
	EventCampaignAction WebhookEventName = "campaign.action"

	// Warmup.
	EventWarmupEmailSent       WebhookEventName = "warmup.email_sent"
	EventWarmupHealthChanged   WebhookEventName = "warmup.health_changed"
	EventWarmupPlacementInSpam WebhookEventName = "warmup.placement_in_spam"
	EventWarmupQuarantined     WebhookEventName = "warmup.quarantined"
	EventWarmupBlocked         WebhookEventName = "warmup.blocked"

	// Deliverability.
	EventDeliverabilityBounce    WebhookEventName = "deliverability.bounce"
	EventDeliverabilityComplaint WebhookEventName = "deliverability.complaint"

	// Meetings booked through a connected scheduling provider.
	EventMeetingBooked      WebhookEventName = "meeting.booked"
	EventMeetingRescheduled WebhookEventName = "meeting.rescheduled"
	EventMeetingCanceled    WebhookEventName = "meeting.canceled"

	// Inbox mail. These are firehose events: subscribe explicitly.
	EventInboxEmailReceived WebhookEventName = "inbox.email_received"
	EventInboxEmailUpdated  WebhookEventName = "inbox.email_updated"
	EventInboxEmailDeleted  WebhookEventName = "inbox.email_deleted"
	EventInboxReplyReceived WebhookEventName = "inbox.reply_received"

	// Contacts.
	EventContactCreated WebhookEventName = "contact.created"
	EventContactUpdated WebhookEventName = "contact.updated"
	EventContactDeleted WebhookEventName = "contact.deleted"

	// Bulk import and export.
	EventBulkOperationStarted   WebhookEventName = "bulk_operation.started"
	EventBulkOperationCompleted WebhookEventName = "bulk_operation.completed"
	EventBulkOperationFailed    WebhookEventName = "bulk_operation.failed"

	// Automations.
	EventAutomationCreated WebhookEventName = "automation.created"
	EventAutomationUpdated WebhookEventName = "automation.updated"
	EventAutomationDeleted WebhookEventName = "automation.deleted"
	EventAutomationRun     WebhookEventName = "automation.run"

	// Templates.
	EventTemplateCreated WebhookEventName = "template.created"
	EventTemplateUpdated WebhookEventName = "template.updated"
	EventTemplateDeleted WebhookEventName = "template.deleted"

	// Team and access governance.
	EventTeamMemberInvited WebhookEventName = "team.member_invited"
	EventTeamMemberRemoved WebhookEventName = "team.member_removed"
	EventRoleCreated       WebhookEventName = "role.created"
	EventRoleUpdated       WebhookEventName = "role.updated"
	EventRoleDeleted       WebhookEventName = "role.deleted"

	// CRM.
	EventCRMDealCreated    WebhookEventName = "crm.deal_created"
	EventCRMDealUpdated    WebhookEventName = "crm.deal_updated"
	EventCRMDealDeleted    WebhookEventName = "crm.deal_deleted"
	EventCRMTaskCreated    WebhookEventName = "crm.task_created"
	EventCRMTaskUpdated    WebhookEventName = "crm.task_updated"
	EventCRMNoteCreated    WebhookEventName = "crm.note_created"
	EventCRMPipelineUpdate WebhookEventName = "crm.pipeline_updated"

	// Workspace.
	EventLeadSyncSourceUpdated WebhookEventName = "lead_sync_source.updated"
	EventSettingsUpdated       WebhookEventName = "settings.updated"
	EventSubscriptionUpdated   WebhookEventName = "subscription.updated"

	// EventCustom is the developer-defined event published by a fire-event
	// sequence node.
	EventCustom WebhookEventName = "custom.event"
	// EventEndpointTest is the verification ping, delivered only to the
	// endpoint being verified.
	EventEndpointTest WebhookEventName = "webhook.test"
)

// Delivery states returned in [WebhookDelivery.Status].
const (
	// DeliveryPending is queued for its next attempt.
	DeliveryPending = "pending"
	// DeliveryInFlight is being attempted right now.
	DeliveryInFlight = "in_flight"
	// DeliveryDelivered succeeded.
	DeliveryDelivered = "delivered"
	// DeliveryFailed will be retried.
	DeliveryFailed = "failed"
	// DeliveryAbandoned exhausted its attempts.
	DeliveryAbandoned = "abandoned"
)

// WebhookDelivery is one delivery record, updated in place as attempts
// progress.
type WebhookDelivery struct {
	ID             string `json:"id"`
	EndpointID     string `json:"endpoint_id"`
	OrganizationID string `json:"organization_id"`
	EventType      string `json:"event_type"`
	// EventID is stable across retries of the same event.
	EventID string `json:"event_id"`
	// Payload is the exact body POSTed to the endpoint.
	Payload json.RawMessage `json:"payload"`
	// Status is one of the Delivery* constants.
	Status        string     `json:"status"`
	AttemptCount  int        `json:"attempt_count"`
	MaxAttempts   int        `json:"max_attempts"`
	NextAttemptAt time.Time  `json:"next_attempt_at"`
	LastAttemptAt *time.Time `json:"last_attempt_at,omitempty"`

	ResponseStatus      *int    `json:"response_status,omitempty"`
	ResponseBodyExcerpt *string `json:"response_body_excerpt,omitempty"`
	ErrorReason         *string `json:"error_reason,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WebhookDrop is a daily rollup of events the dispatch throttle dropped, so
// rate limiting is visible rather than silent.
type WebhookDrop struct {
	EventType      string    `json:"event_type"`
	Day            time.Time `json:"day"`
	DroppedWindows int       `json:"dropped_windows"`
	LastDroppedAt  time.Time `json:"last_dropped_at"`
}

// WebhookEvent is the decoded body of a delivery, as produced by
// [WebhookService.ConstructEvent]. Switch on EventType, then unmarshal Data
// into the concrete payload for that event.
type WebhookEvent struct {
	// ID is the event id, stable across retries. Deduplicate on it.
	ID string `json:"id"`
	// EventType is the event key; compare it against the Event* constants.
	EventType      string    `json:"event_type"`
	OrganizationID string    `json:"organization_id"`
	CreatedAt      time.Time `json:"created_at"`
	// Data is the event-specific payload, left raw.
	Data json.RawMessage `json:"data"`
}

// WebhookCreateParams registers a webhook endpoint.
type WebhookCreateParams struct {
	// URL is the HTTPS endpoint that will receive deliveries. Private and
	// loopback addresses are rejected.
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
	// EventTypes are the keys to subscribe to. An empty list subscribes to
	// every non-firehose event.
	EventTypes []string `json:"event_types,omitempty"`
	// Enabled defaults to true when nil.
	Enabled *bool `json:"enabled,omitempty"`
}

// WebhookUpdateParams replaces an endpoint's configuration. Unlike most update
// params these are not merged: the API replaces URL, description, event types
// and enabled state wholesale, so send the complete desired state.
//
// The secret is not changed here; use [WebhookService.RotateSecret]. Changing
// the URL host re-arms ownership verification.
type WebhookUpdateParams struct {
	URL         string   `json:"url"`
	Description string   `json:"description,omitempty"`
	EventTypes  []string `json:"event_types,omitempty"`
	Enabled     *bool    `json:"enabled,omitempty"`
}

// WebhookDeliveryListParams filters the delivery log.
type WebhookDeliveryListParams struct {
	ListOptions
	// EndpointID narrows to one endpoint. It is ignored by
	// [WebhookService.EndpointDeliveries], which takes the id in the path.
	EndpointID string
	// Status is one of the Delivery* constants.
	Status string
	// EventType is one of the Event* constants.
	EventType string
}

func (p *WebhookDeliveryListParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	setNonEmpty(q, "endpoint_id", p.EndpointID)
	setNonEmpty(q, "status", p.Status)
	setNonEmpty(q, "event_type", p.EventType)
	return q
}

// List returns the organization's webhook endpoints alongside the full event
// catalog. The list is not paginated.
func (s *WebhookService) List(ctx context.Context, opts ...RequestOption) (*WebhookEndpointList, *Response, error) {
	return fetch[WebhookEndpointList](ctx, s.client, "webhooks", opts)
}

// Create registers a new endpoint. The returned [WebhookWithSecret] carries the
// signing secret, which is only ever available here.
//
// The endpoint receives no real events until it passes verification: call
// [WebhookService.Verify] and echo the challenge back.
func (s *WebhookService) Create(ctx context.Context, params *WebhookCreateParams, opts ...RequestOption) (*WebhookWithSecret, *Response, error) {
	return send[WebhookWithSecret](ctx, s.client, s.client.post, "webhooks", params, opts)
}

// Update replaces an endpoint's configuration.
func (s *WebhookService) Update(ctx context.Context, id string, params *WebhookUpdateParams, opts ...RequestOption) (*Webhook, *Response, error) {
	return send[Webhook](ctx, s.client, s.client.patch, "webhooks/"+url.PathEscape(id), params, opts)
}

// Delete permanently removes an endpoint.
func (s *WebhookService) Delete(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "webhooks/"+url.PathEscape(id), opts...)
}

// RotateSecret issues a new signing secret and returns it. The previous secret
// stops verifying immediately, so deploy the new one before rotating.
func (s *WebhookService) RotateSecret(ctx context.Context, id string, opts ...RequestOption) (string, *Response, error) {
	var out struct {
		Secret string `json:"secret"`
	}
	resp, err := s.client.post(ctx, "webhooks/"+url.PathEscape(id)+"/rotate-secret", nil, &out, opts...)
	if err != nil {
		return "", resp, err
	}
	return out.Secret, resp, nil
}

// Verify sends an ownership challenge to the endpoint. Echo the value of the
// [WebhookChallengeHeader] (or the challenge field in the body) back in your
// response to confirm the endpoint and start receiving events.
func (s *WebhookService) Verify(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "webhooks/"+url.PathEscape(id)+"/verify", nil, nil, opts...)
}

// EventTypes returns the full catalog of deliverable events.
func (s *WebhookService) EventTypes(ctx context.Context, opts ...RequestOption) ([]WebhookEventDescriptor, *Response, error) {
	var out struct {
		EventTypes []WebhookEventDescriptor `json:"event_types"`
	}
	resp, err := s.client.get(ctx, "webhooks/event-types", &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.EventTypes, resp, nil
}

// Deliveries returns a page of the delivery log across every endpoint.
func (s *WebhookService) Deliveries(ctx context.Context, params *WebhookDeliveryListParams, opts ...RequestOption) (*Page[WebhookDelivery], error) {
	return listJSON[WebhookDelivery](ctx, s.client, "webhooks/deliveries", params.values(), opts...)
}

// EndpointDeliveries returns a page of one endpoint's delivery log.
func (s *WebhookService) EndpointDeliveries(ctx context.Context, id string, params *WebhookDeliveryListParams, opts ...RequestOption) (*Page[WebhookDelivery], error) {
	q := params.values()
	q.Del("endpoint_id")
	return listJSON[WebhookDelivery](ctx, s.client, "webhooks/"+url.PathEscape(id)+"/deliveries", q, opts...)
}

// Redeliver re-queues a delivery for a fresh attempt cycle, keeping the same
// event id so a receiver deduplicating on it stays correct.
func (s *WebhookService) Redeliver(ctx context.Context, deliveryID string, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "webhooks/deliveries/"+url.PathEscape(deliveryID)+"/redeliver", nil, nil, opts...)
}

// Drops returns the daily rollup of events the dispatch throttle dropped.
func (s *WebhookService) Drops(ctx context.Context, opts ...RequestOption) ([]WebhookDrop, *Response, error) {
	var out struct {
		Drops []WebhookDrop `json:"drops"`
	}
	resp, err := s.client.get(ctx, "webhooks/throttle-drops", &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Drops, resp, nil
}

// ConstructEvent verifies a delivery's signature and decodes its payload.
//
// Pass the raw, unmodified request body and the value of the
// [WebhookSignatureHeader]. It returns [ErrInvalidWebhookSignature] when the
// signature does not match, or [ErrWebhookSignatureExpired] when the timestamp
// falls outside [DefaultWebhookTolerance].
//
//	func handler(w http.ResponseWriter, r *http.Request) {
//		body, _ := io.ReadAll(r.Body)
//		event, err := client.Webhooks.ConstructEvent(body, r.Header.Get(warmbly.WebhookSignatureHeader), secret)
//		if err != nil {
//			http.Error(w, "bad signature", http.StatusBadRequest)
//			return
//		}
//		switch event.EventType {
//		case warmbly.EventCampaignReplyReceived:
//			// ...
//		}
//	}
func (s *WebhookService) ConstructEvent(payload []byte, signatureHeader, secret string) (*WebhookEvent, error) {
	return ConstructWebhookEvent(payload, signatureHeader, secret, DefaultWebhookTolerance)
}

// ConstructWebhookEvent is [WebhookService.ConstructEvent] with an explicit
// timestamp tolerance. A tolerance of zero or less disables the replay check.
func ConstructWebhookEvent(payload []byte, signatureHeader, secret string, tolerance time.Duration) (*WebhookEvent, error) {
	ts, ok := verifyWebhookSignature(payload, signatureHeader, secret)
	if !ok {
		return nil, ErrInvalidWebhookSignature
	}
	if tolerance > 0 {
		if drift := timeNow().Sub(ts); drift > tolerance || drift < -tolerance {
			return nil, ErrWebhookSignatureExpired
		}
	}
	event := new(WebhookEvent)
	if err := json.Unmarshal(payload, event); err != nil {
		return nil, err
	}
	return event, nil
}

// ComputeWebhookSignature returns the expected [WebhookSignatureHeader] value
// for a payload, signing timestamp and secret: "t=<unix>,v1=<hex>", where the
// digest is HMAC-SHA256 over "<unix>." followed by the raw payload.
func ComputeWebhookSignature(payload []byte, secret string, timestamp time.Time) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.", timestamp.Unix())
	mac.Write(payload)
	return fmt.Sprintf("t=%d,v1=%s", timestamp.Unix(), hex.EncodeToString(mac.Sum(nil)))
}

// VerifyWebhookSignature reports whether signatureHeader is a valid signature
// for payload under secret. The digest comparison is constant-time.
//
// It does not check the timestamp for freshness; use
// [ConstructWebhookEvent] (or [VerifyWebhookSignatureAt]) to also defeat
// replay.
func VerifyWebhookSignature(payload []byte, signatureHeader, secret string) bool {
	_, ok := verifyWebhookSignature(payload, signatureHeader, secret)
	return ok
}

// VerifyWebhookSignatureAt verifies a signature and returns the timestamp it
// was signed at, so a caller can apply its own freshness policy.
func VerifyWebhookSignatureAt(payload []byte, signatureHeader, secret string) (time.Time, bool) {
	return verifyWebhookSignature(payload, signatureHeader, secret)
}

func verifyWebhookSignature(payload []byte, signatureHeader, secret string) (time.Time, bool) {
	unix, digest, ok := parseSignatureHeader(signatureHeader)
	if !ok {
		return time.Time{}, false
	}
	ts := time.Unix(unix, 0).UTC()
	want := ComputeWebhookSignature(payload, secret, ts)
	_, wantDigest, ok := parseSignatureHeader(want)
	if !ok {
		return time.Time{}, false
	}
	if subtle.ConstantTimeCompare([]byte(digest), []byte(wantDigest)) != 1 {
		return time.Time{}, false
	}
	return ts, true
}

// parseSignatureHeader splits a "t=<unix>,v1=<hex>" header into its parts.
func parseSignatureHeader(header string) (unix int64, digest string, ok bool) {
	var sawTimestamp bool
	for _, part := range strings.Split(header, ",") {
		key, value, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found {
			continue
		}
		switch key {
		case "t":
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return 0, "", false
			}
			unix, sawTimestamp = n, true
		case "v1":
			digest = value
		}
	}
	return unix, digest, sawTimestamp && digest != ""
}
