package warmbly

import (
	"errors"
	"strings"
	"testing"
)

func TestWebhookSignatureRoundTrip(t *testing.T) {
	secret := "whsec_test"
	payload := []byte(`{"id":"evt_1","event":"campaign.started","data":{}}`)

	sig := ComputeWebhookSignature(payload, secret)
	if !strings.HasPrefix(sig, "sha256=") {
		t.Fatalf("signature missing sha256= prefix: %q", sig)
	}

	if !VerifyWebhookSignature(payload, sig, secret) {
		t.Error("valid signature did not verify")
	}
	// The sha256= prefix is optional on the incoming header.
	if !VerifyWebhookSignature(payload, strings.TrimPrefix(sig, "sha256="), secret) {
		t.Error("valid signature without prefix did not verify")
	}
}

func TestWebhookSignatureRejects(t *testing.T) {
	secret := "whsec_test"
	payload := []byte(`{"id":"evt_1"}`)
	sig := ComputeWebhookSignature(payload, secret)

	cases := []struct {
		name    string
		payload []byte
		sig     string
		secret  string
	}{
		{"wrong secret", payload, sig, "other"},
		{"tampered payload", []byte(`{"id":"evt_2"}`), sig, secret},
		{"empty signature", payload, "", secret},
		{"garbage signature", payload, "sha256=deadbeef", secret},
	}
	for _, tt := range cases {
		if VerifyWebhookSignature(tt.payload, tt.sig, tt.secret) {
			t.Errorf("%s: expected verification to fail but it passed", tt.name)
		}
	}
}

func TestConstructEvent(t *testing.T) {
	client, err := New(WithAPIKey("wmbly_test"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	secret := "whsec_abc"
	payload := []byte(`{"id":"evt_9","event":"campaign.reply_received","organization_id":"org_1","data":{"campaign_id":"camp_1"},"attempt":1,"max_attempts":3}`)
	sig := ComputeWebhookSignature(payload, secret)

	event, err := client.Webhooks.ConstructEvent(payload, sig, secret)
	if err != nil {
		t.Fatalf("ConstructEvent: %v", err)
	}
	if event.Event != string(EventCampaignReplyReceived) {
		t.Errorf("event type = %q, want %q", event.Event, EventCampaignReplyReceived)
	}
	if event.ID != "evt_9" || event.OrganizationID != "org_1" || event.MaxAttempts != 3 {
		t.Errorf("unexpected decoded event: %+v", event)
	}

	if _, err := client.Webhooks.ConstructEvent(payload, "sha256=bad", secret); !errors.Is(err, ErrInvalidWebhookSignature) {
		t.Errorf("expected ErrInvalidWebhookSignature, got %v", err)
	}
}
