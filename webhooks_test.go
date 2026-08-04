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
