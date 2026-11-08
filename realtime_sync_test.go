package warmbly

import (
	"encoding/json"
	"testing"
)

func TestSyncPermissionValues(t *testing.T) {
	// Values must equal the server's bit positions.
	if PermReadEmails != 1 || PermRealtimeSubscribe != 1<<11 || PermAPIKeys != 1<<13 ||
		PermAIAgent != 1<<22 || PermAIResearch != 1<<23 {
		t.Fatal("permission bit values drifted from the server")
	}
	if PermAppGrantable&PermAPIKeys != 0 || PermAppGrantable|PermAPIKeys != PermFullAccess {
		t.Errorf("PermAppGrantable = %d", PermAppGrantable)
	}
}

func TestSyncCatalogDecodesNewFields(t *testing.T) {
	var c PermissionCatalog
	in := `{"permissions":[],"presets":{"read_only":688159,"full_access":16777215},"grantable":5,"app_scopes":16769023}`
	if err := json.Unmarshal([]byte(in), &c); err != nil || c.Grantable != 5 || c.AppScopes != 16769023 {
		t.Fatalf("%+v %v", c, err)
	}
	var d WebhookEventDescriptor
	if err := json.Unmarshal([]byte(`{"type":"campaign.action","category":"campaign","description":"x","firehose":false,"deprecated":true}`), &d); err != nil || !d.Deprecated {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestSyncErrorReasonAndOAuthSuspension(t *testing.T) {
	var e Error
	if err := json.Unmarshal([]byte(`{"error":"Bad Request","message":"m","code":"bad_request","reason":"why","request_id":"r"}`), &e); err != nil || e.Reason != "why" {
		t.Fatalf("%+v %v", e, err)
	}
	var a OAuthApp
	if err := json.Unmarshal([]byte(`{"id":"1","status":"active","suspended_at":"2026-10-01T00:00:00Z","suspended_reason":"abuse"}`), &a); err != nil || a.SuspendedAt == nil || a.SuspendedReason != "abuse" {
		t.Fatalf("%+v %v", a, err)
	}
}

func TestSyncWebhookPayloads(t *testing.T) {
	var r ReplyReceivedPayload
	in := `{"contact_email":"a@b.c","intent":"interested","confidence":0.9,"thread_id":"t","trigger":"inbox_reply","ai_labels":["Meeting"],"ai_kind":"","ai_relevance":82,"ai_needs_review":false}`
	if err := json.Unmarshal([]byte(in), &r); err != nil || r.AIKind == nil || *r.AIKind != "" || *r.AIRelevance != 82 || r.Trigger != "inbox_reply" {
		t.Fatalf("%+v %v", r, err)
	}
	var b BulkOperationPayload
	if err := json.Unmarshal([]byte(`{"operation_id":"o","operation_type":"contact_import","entity_type":"contact","filename":"f.csv","total":5,"imported":4,"failed":1}`), &b); err != nil || b.Imported != 4 {
		t.Fatalf("%+v %v", b, err)
	}
}
