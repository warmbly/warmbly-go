package warmbly

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestWebhookSignatureRoundTrip(t *testing.T) {
	secret := "whsec_test"
	payload := []byte(`{"id":"evt_1","event_type":"campaign.started","data":{}}`)
	now := time.Now()

	sig := ComputeWebhookSignature(payload, secret, now)
	if !strings.HasPrefix(sig, "t=") || !strings.Contains(sig, ",v1=") {
		t.Fatalf("signature is not in t=<unix>,v1=<hex> form: %q", sig)
	}
	if !strings.Contains(sig, "t="+strconv.FormatInt(now.Unix(), 10)) {
		t.Errorf("signature %q does not carry the signing timestamp", sig)
	}

	if !VerifyWebhookSignature(payload, sig, secret) {
		t.Error("valid signature did not verify")
	}

	ts, ok := VerifyWebhookSignatureAt(payload, sig, secret)
	if !ok {
		t.Fatal("VerifyWebhookSignatureAt rejected a valid signature")
	}
	if ts.Unix() != now.Unix() {
		t.Errorf("timestamp = %v, want %v", ts.Unix(), now.Unix())
	}
}

// TestWebhookSignatureBindsTimestamp is the property that makes replay
// detection possible: the digest covers the timestamp, so a signature cannot be
// lifted onto a different one.
func TestWebhookSignatureBindsTimestamp(t *testing.T) {
	secret := "whsec_test"
	payload := []byte(`{"id":"evt_1"}`)
	now := time.Now()

	sig := ComputeWebhookSignature(payload, secret, now)
	_, digest, _ := strings.Cut(sig, ",v1=")
	forged := "t=" + strconv.FormatInt(now.Add(time.Hour).Unix(), 10) + ",v1=" + digest

	if VerifyWebhookSignature(payload, forged, secret) {
		t.Error("a signature re-stamped with a different timestamp verified")
	}
}

func TestWebhookSignatureRejects(t *testing.T) {
	secret := "whsec_test"
	payload := []byte(`{"id":"evt_1"}`)
	sig := ComputeWebhookSignature(payload, secret, time.Now())

	cases := []struct {
		name    string
		payload []byte
		sig     string
		secret  string
	}{
		{"wrong secret", payload, sig, "other"},
		{"tampered payload", []byte(`{"id":"evt_2"}`), sig, secret},
		{"empty signature", payload, "", secret},
		{"garbage digest", payload, "t=1,v1=deadbeef", secret},
		{"missing timestamp", payload, "v1=deadbeef", secret},
		{"missing digest", payload, "t=1", secret},
		{"non-numeric timestamp", payload, "t=abc,v1=deadbeef", secret},
		{"unstructured header", payload, "deadbeef", secret},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if VerifyWebhookSignature(tt.payload, tt.sig, tt.secret) {
				t.Error("expected verification to fail but it passed")
			}
		})
	}
}

func TestConstructEvent(t *testing.T) {
	client, err := New(WithAPIKey("wmbly_test"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	secret := "whsec_abc"
	payload := []byte(`{
		"id":"evt_9",
		"event_type":"campaign.reply_received",
		"organization_id":"org_1",
		"created_at":"2026-08-01T10:00:00Z",
		"data":{"campaign_id":"camp_1"}
	}`)
	sig := ComputeWebhookSignature(payload, secret, time.Now())

	event, err := client.Webhooks.ConstructEvent(payload, sig, secret)
	if err != nil {
		t.Fatalf("ConstructEvent: %v", err)
	}
	if event.EventType != EventCampaignReplyReceived {
		t.Errorf("event type = %q, want %q", event.EventType, EventCampaignReplyReceived)
	}
	if event.ID != "evt_9" || event.OrganizationID != "org_1" {
		t.Errorf("unexpected decoded event: %+v", event)
	}

	var data struct {
		CampaignID string `json:"campaign_id"`
	}
	if err := json.Unmarshal(event.Data, &data); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if data.CampaignID != "camp_1" {
		t.Errorf("campaign_id = %q", data.CampaignID)
	}

	if _, err := client.Webhooks.ConstructEvent(payload, "t=1,v1=bad", secret); !errors.Is(err, ErrInvalidWebhookSignature) {
		t.Errorf("expected ErrInvalidWebhookSignature, got %v", err)
	}
}

// TestConstructEventRejectsStaleSignature covers replay defense: a correctly
// signed but old delivery must not be accepted.
func TestConstructEventRejectsStaleSignature(t *testing.T) {
	client, err := New(WithAPIKey("wmbly_test"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	secret := "whsec_abc"
	payload := []byte(`{"id":"evt_1","event_type":"campaign.started"}`)

	stale := ComputeWebhookSignature(payload, secret, time.Now().Add(-time.Hour))
	if _, err := client.Webhooks.ConstructEvent(payload, stale, secret); !errors.Is(err, ErrWebhookSignatureExpired) {
		t.Errorf("expected ErrWebhookSignatureExpired, got %v", err)
	}

	// A caller that genuinely wants to accept old deliveries can widen or
	// disable the window.
	if _, err := ConstructWebhookEvent(payload, stale, secret, 0); err != nil {
		t.Errorf("tolerance of 0 should disable the check, got %v", err)
	}

	// A future timestamp is just as suspect as an old one.
	future := ComputeWebhookSignature(payload, secret, time.Now().Add(time.Hour))
	if _, err := client.Webhooks.ConstructEvent(payload, future, secret); !errors.Is(err, ErrWebhookSignatureExpired) {
		t.Errorf("expected a far-future signature to be rejected, got %v", err)
	}
}

// TestConstructEventFormSubmitted covers the newest event key end to end:
// signature, envelope, and the flat contact columns that ride alongside the
// raw answers so an automation never has to walk `data`.
func TestConstructEventFormSubmitted(t *testing.T) {
	client, err := New(WithAPIKey("wmbly_test"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	secret := "whsec_abc"
	payload := []byte(`{
		"id":"evt_form_1",
		"event_type":"form.submitted",
		"organization_id":"org_1",
		"created_at":"2026-08-01T10:00:00Z",
		"data":{
			"form_id":"f_1",
			"form_name":"Demo request",
			"submission_id":"s_1",
			"source_url":"https://example.com/demo",
			"data":{"field_1":"jane@example.com","field_2":["a","b"]},
			"contact_id":"ct_1",
			"contact_email":"jane@example.com",
			"first_name":"Jane",
			"company":"Acme",
			"campaign_id":"camp_1"
		}
	}`)
	sig := ComputeWebhookSignature(payload, secret, time.Now())

	event, err := client.Webhooks.ConstructEvent(payload, sig, secret)
	if err != nil {
		t.Fatalf("ConstructEvent: %v", err)
	}
	if event.EventType != EventFormSubmitted {
		t.Fatalf("event type = %q, want %q", event.EventType, EventFormSubmitted)
	}

	var data FormSubmittedPayload
	if err := event.Into(&data); err != nil {
		t.Fatalf("Into: %v", err)
	}
	if data.FormID != "f_1" || data.FormName != "Demo request" || data.SubmissionID != "s_1" {
		t.Errorf("form identity = %+v", data)
	}
	if data.ContactID != "ct_1" || data.ContactEmail != "jane@example.com" || data.FirstName != "Jane" || data.Company != "Acme" {
		t.Errorf("mapped contact columns = %+v", data)
	}
	if data.CampaignID != "camp_1" || data.SourceURL != "https://example.com/demo" {
		t.Errorf("enrolment/source = %+v", data)
	}
	// Answers keep the type the field collected, so a multi-select stays a
	// list rather than being flattened into a string.
	if got, ok := data.Data["field_2"].([]any); !ok || len(got) != 2 {
		t.Errorf("data[field_2] = %#v, want a two-element list", data.Data["field_2"])
	}

	// An anonymous submission carries no contact at all.
	anon := []byte(`{"id":"evt_form_2","event_type":"form.submitted","data":{"form_id":"f_1","form_name":"Newsletter","submission_id":"s_2","data":{}}}`)
	event, err = client.Webhooks.ConstructEvent(anon, ComputeWebhookSignature(anon, secret, time.Now()), secret)
	if err != nil {
		t.Fatalf("ConstructEvent: %v", err)
	}
	data = FormSubmittedPayload{}
	if err := event.Into(&data); err != nil {
		t.Fatalf("Into: %v", err)
	}
	if data.ContactID != "" || data.ContactEmail != "" {
		t.Errorf("a form that collected no email must not report a contact: %+v", data)
	}
}

// TestConstructEventContactCreated covers the richer contact.created body that
// replaced the generic audit-bridged shape.
func TestConstructEventContactCreated(t *testing.T) {
	client, err := New(WithAPIKey("wmbly_test"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	secret := "whsec_abc"
	payload := []byte(`{
		"id":"evt_c_1",
		"event_type":"contact.created",
		"organization_id":"org_1",
		"created_at":"2026-08-01T10:00:00Z",
		"data":{
			"contact_id":"ct_1",
			"contact_email":"jane@example.com",
			"first_name":"Jane",
			"last_name":"Doe",
			"company":"Acme",
			"phone":"+441234567890",
			"subscribed":true,
			"custom_fields":{"tier":"gold"},
			"source":"form",
			"source_detail":"Demo request",
			"campaign_ids":["camp_1"],
			"category_ids":["cat_1","cat_2"],
			"created_at":"2026-08-01T09:59:59Z"
		}
	}`)
	sig := ComputeWebhookSignature(payload, secret, time.Now())

	event, err := client.Webhooks.ConstructEvent(payload, sig, secret)
	if err != nil {
		t.Fatalf("ConstructEvent: %v", err)
	}
	if event.EventType != EventContactCreated {
		t.Fatalf("event type = %q, want %q", event.EventType, EventContactCreated)
	}

	var data ContactCreatedPayload
	if err := event.Into(&data); err != nil {
		t.Fatalf("Into: %v", err)
	}
	if data.ContactID != "ct_1" || data.ContactEmail != "jane@example.com" {
		t.Errorf("identity = %+v", data)
	}
	if data.FirstName != "Jane" || data.LastName != "Doe" || data.Company != "Acme" || data.Phone != "+441234567890" {
		t.Errorf("contact columns = %+v", data)
	}
	if !data.Subscribed || data.CustomFields["tier"] != "gold" {
		t.Errorf("consent/custom fields = %+v", data)
	}
	// The first-touch attribution is the point of the richer payload.
	if data.Source != ContactSourceForm || data.SourceDetail != "Demo request" {
		t.Errorf("source = %q/%q, want form/Demo request", data.Source, data.SourceDetail)
	}
	if len(data.CampaignIDs) != 1 || len(data.CategoryIDs) != 2 {
		t.Errorf("memberships = %+v", data)
	}
	if data.CreatedAt.IsZero() {
		t.Error("CreatedAt did not decode")
	}
}

// TestConstructEventChallenge covers the verification ping. The signed body is
// the only trustworthy place to read the token from.
func TestConstructEventChallenge(t *testing.T) {
	client, err := New(WithAPIKey("wmbly_test"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	secret := "whsec_abc"
	payload := []byte(`{
		"id":"evt_test_1",
		"event_type":"webhook.test",
		"organization_id":"org_1",
		"data":{"challenge":"tok_123","endpoint_id":"wh_1","message":"Echo the challenge value"}
	}`)
	event, err := client.Webhooks.ConstructEvent(payload, ComputeWebhookSignature(payload, secret, time.Now()), secret)
	if err != nil {
		t.Fatalf("ConstructEvent: %v", err)
	}
	if event.EventType != EventEndpointTest {
		t.Fatalf("event type = %q, want %q", event.EventType, EventEndpointTest)
	}

	var data WebhookChallengePayload
	if err := event.Into(&data); err != nil {
		t.Fatalf("Into: %v", err)
	}
	if data.Challenge != "tok_123" || data.EndpointID != "wh_1" {
		t.Errorf("challenge = %+v", data)
	}
}

// TestWebhookEventInto tolerates an event with no data at all, which a caller
// switching on EventType will otherwise trip over.
func TestWebhookEventInto(t *testing.T) {
	var data ContactCreatedPayload
	if err := (&WebhookEvent{}).Into(&data); err != nil {
		t.Errorf("Into on an empty payload = %v, want nil", err)
	}
	if err := (&WebhookEvent{Data: json.RawMessage(`"not an object"`)}).Into(&data); err == nil {
		t.Error("Into should surface a decode failure")
	}
}

// TestWebhookEventCatalogIsDistinct guards against a copy/paste collision in
// the event-key constants, which would silently route one event's deliveries
// into another's handler.
func TestWebhookEventCatalogIsDistinct(t *testing.T) {
	keys := []string{
		EventEmailAccountConnected, EventEmailAccountRemoved, EventEmailAccountDisconnected,
		EventEmailAccountError, EventEmailAccountSynced, EventEmailAccountHealthChanged,
		EventCampaignEmailSent, EventCampaignEmailDelivered, EventCampaignEmailOpened,
		EventCampaignEmailClicked, EventCampaignEmailBounced, EventCampaignReplyReceived,
		EventCampaignUnsubscribed, EventCampaignStarted, EventCampaignPaused,
		EventCampaignCompleted, EventCampaignCreated, EventCampaignUpdated, EventCampaignDeleted,
		EventCampaignDeliverabilityWarning, EventCampaignAction,
		EventWarmupEmailSent, EventWarmupHealthChanged, EventWarmupPlacementInSpam,
		EventWarmupQuarantined, EventWarmupBlocked,
		EventDeliverabilityBounce, EventDeliverabilityComplaint,
		EventMeetingBooked, EventMeetingRescheduled, EventMeetingCanceled,
		EventInboxEmailReceived, EventInboxEmailUpdated, EventInboxEmailDeleted, EventInboxReplyReceived,
		EventContactCreated, EventContactUpdated, EventContactDeleted, EventFormSubmitted,
		EventBulkOperationStarted, EventBulkOperationCompleted, EventBulkOperationFailed,
		EventAutomationCreated, EventAutomationUpdated, EventAutomationDeleted, EventAutomationRun,
		EventTemplateCreated, EventTemplateUpdated, EventTemplateDeleted,
		EventTeamMemberInvited, EventTeamMemberRemoved,
		EventRoleCreated, EventRoleUpdated, EventRoleDeleted,
		EventCRMDealCreated, EventCRMDealUpdated, EventCRMDealDeleted,
		EventCRMTaskCreated, EventCRMTaskUpdated, EventCRMNoteCreated, EventCRMPipelineUpdate,
		EventLeadSyncSourceUpdated, EventSettingsUpdated, EventSubscriptionUpdated,
		EventCustom, EventEndpointTest,
	}
	seen := make(map[string]bool, len(keys))
	for _, k := range keys {
		if k == "" {
			t.Error("an event key is empty")
		}
		if seen[k] {
			t.Errorf("duplicate event key %q", k)
		}
		seen[k] = true
		if !strings.Contains(k, ".") {
			t.Errorf("event key %q is not in <resource>.<action> form", k)
		}
	}
}
