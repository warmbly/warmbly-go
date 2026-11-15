package gateway

import (
	"encoding/json"
	"testing"
)

func TestEmailBouncedDecodes(t *testing.T) {
	// The server publishes EMAIL_BOUNCED as a TrackingEventPayload.
	raw := `{"event_type":"EMAIL_BOUNCED","user_id":"","timestamp":"2026-11-14T10:00:00Z","org_id":"o","campaign_id":"c","contact_id":"k","contact_email":"a@b.c","step_id":"s"}`
	e := Event{Type: EventEmailBounced, Raw: json.RawMessage(raw)}
	var en EngagementEvent
	if err := e.Into(&en); err != nil || en.CampaignID != "c" || en.ContactID != "k" || en.ContactEmail != "a@b.c" || en.StepID != "s" {
		t.Fatalf("bounced = %+v, %v", en, err)
	}
	if EventEmailBounced != "EMAIL_BOUNCED" {
		t.Fatal("wire name drifted")
	}
}
