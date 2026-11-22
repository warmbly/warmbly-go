package gateway

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStoreEventsDecode(t *testing.T) {
	// The server publishes pubsub.StoreEvent: a BaseEvent plus org_id, id, status and optional links.
	raw := `{"event_type":"STORE_MAILBOX_UPDATED","user_id":"00000000-0000-0000-0000-000000000000","timestamp":"2026-11-20T10:00:00Z","org_id":"o","id":"m","status":"active","order_id":"ord","domain_id":"d","email_account_id":"e","connect_state":"connected","seq":7}`
	e := Event{Type: EventStoreMailboxUpdated, Raw: json.RawMessage(raw)}
	var se StoreEvent
	if err := e.Into(&se); err != nil {
		t.Fatal(err)
	}
	if se.OrgID != "o" || se.ID != "m" || se.Status != "active" || se.OrderID != "ord" || se.DomainID != "d" ||
		se.EmailAccountID != "e" || se.ConnectState != "connected" || se.Seq != 7 || se.EventType != EventStoreMailboxUpdated {
		t.Fatalf("store event = %+v", se)
	}
	var order StoreEvent
	if err := (&Event{Type: EventStoreOrderUpdated, Raw: json.RawMessage(`{"event_type":"STORE_ORDER_UPDATED","org_id":"o","id":"ord","status":"completed"}`)}).Into(&order); err != nil || order.ID != "ord" || order.DomainID != "" {
		t.Fatalf("order event = %+v, %v", order, err)
	}
	for name, want := range map[EventName]string{
		EventStoreOrderUpdated:   "STORE_ORDER_UPDATED",
		EventStoreDomainUpdated:  "STORE_DOMAIN_UPDATED",
		EventStoreMailboxUpdated: "STORE_MAILBOX_UPDATED",
	} {
		if name != want {
			t.Errorf("wire name %q, want %q", name, want)
		}
	}
	if IntentStore != "STORE_" {
		t.Fatalf("IntentStore = %q", IntentStore)
	}
	// The server substring-matches intent tokens: the store intent covers each
	// store event and none of the other families the SDK names.
	for _, name := range []EventName{EventStoreOrderUpdated, EventStoreDomainUpdated, EventStoreMailboxUpdated} {
		if !strings.Contains(name, IntentStore) {
			t.Errorf("%s not covered by IntentStore", name)
		}
	}
	for _, name := range []EventName{EventCampaignUpdated, EventEmailSent, EventAccountConnected, EventBillingCreditsChanged, EventCRMSynced, EventPlacementTestUpdated} {
		if strings.Contains(name, IntentStore) {
			t.Errorf("IntentStore also matches %s", name)
		}
	}
}

func TestUnknownEventStillDecodes(t *testing.T) {
	// A type this release does not name still decodes into BaseEvent and a raw map.
	e := Event{Type: "STORE_SOMETHING_NEW", Raw: json.RawMessage(`{"event_type":"STORE_SOMETHING_NEW","org_id":"o","extra":{"a":1}}`)}
	var b BaseEvent
	if err := e.Into(&b); err != nil || b.EventType != "STORE_SOMETHING_NEW" || b.OrgID != "o" {
		t.Fatalf("%+v %v", b, err)
	}
	var m map[string]any
	if err := e.Into(&m); err != nil || m["extra"] == nil {
		t.Fatalf("%v %v", m, err)
	}
}
