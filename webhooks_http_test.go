package warmbly

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

func TestWebhookSvcListParamsValues(t *testing.T) {
	t.Run("nil is empty", func(t *testing.T) {
		var p *WebhookListParams
		if q := p.values(); len(q) != 0 {
			t.Errorf("nil params produced %v, want empty", q)
		}
	})
	t.Run("limit set", func(t *testing.T) {
		p := &WebhookListParams{ListOptions: ListOptions{Limit: 25}}
		q := p.values()
		if got := q.Get("limit"); got != "25" {
			t.Errorf("limit = %q, want 25", got)
		}
	})
}

func TestWebhookSvcList(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/v1/webhooks" {
			t.Errorf("path = %q, want /v1/webhooks", r.URL.Path)
		}
		if got := r.URL.Query().Get("limit"); got != "5" {
			t.Errorf("limit query = %q, want 5", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "req_list")
		_, _ = w.Write([]byte(`{"data":[{"id":"wh_1","url":"https://example.com/hook","event_types":["campaign.started"]}],"pagination":{"has_more":false,"next_cursor":null}}`))
	})

	page, err := c.Webhooks.List(context.Background(), &WebhookListParams{ListOptions: ListOptions{Limit: 5}})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].ID != "wh_1" {
		t.Errorf("unexpected page data: %+v", page.Data)
	}
	if page.Data[0].URL != "https://example.com/hook" {
		t.Errorf("url = %q", page.Data[0].URL)
	}
	if resp := page.Response(); resp == nil || resp.RequestID != "req_list" {
		t.Errorf("unexpected response metadata: %+v", resp)
	}
}

func TestWebhookSvcGet(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/v1/webhooks/wh_42" {
			t.Errorf("path = %q, want /v1/webhooks/wh_42", r.URL.Path)
		}
		w.Header().Set("X-Request-ID", "req_get")
		_, _ = w.Write([]byte(`{"id":"wh_42","url":"https://example.com/h","event_types":["contact.created"],"disabled":true}`))
	})

	hook, resp, err := c.Webhooks.Get(context.Background(), "wh_42")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if hook.ID != "wh_42" || !hook.Disabled {
		t.Errorf("unexpected hook: %+v", hook)
	}
	if resp.StatusCode != http.StatusOK || resp.RequestID != "req_get" {
		t.Errorf("unexpected response: status=%d id=%q", resp.StatusCode, resp.RequestID)
	}
}

func TestWebhookSvcGetError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not_found","message":"missing"}`))
	})

	hook, resp, err := c.Webhooks.Get(context.Background(), "nope")
	if err == nil {
		t.Fatal("expected error")
	}
	if hook != nil {
		t.Errorf("hook = %+v, want nil", hook)
	}
	if resp == nil || resp.StatusCode != http.StatusNotFound {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestWebhookSvcCreate(t *testing.T) {
	var body WebhookCreateParams
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/webhooks" {
			t.Errorf("path = %q, want /v1/webhooks", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"wh_new","url":"https://example.com/h","event_types":["campaign.started"],"secret":"whsec_xyz"}`))
	})

	hook, resp, err := c.Webhooks.Create(context.Background(), &WebhookCreateParams{
		URL:         "https://example.com/h",
		EventTypes:  []string{"campaign.started"},
		Description: "primary",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if hook.ID != "wh_new" || hook.Secret != "whsec_xyz" {
		t.Errorf("unexpected hook: %+v", hook)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", resp.StatusCode)
	}
	if body.URL != "https://example.com/h" || body.Description != "primary" || len(body.EventTypes) != 1 {
		t.Errorf("unexpected request body: %+v", body)
	}
}

func TestWebhookSvcCreateError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad_request","message":"invalid url"}`))
	})

	hook, resp, err := c.Webhooks.Create(context.Background(), &WebhookCreateParams{URL: "ftp://nope"})
	if err == nil {
		t.Fatal("expected error")
	}
	if hook != nil {
		t.Errorf("hook = %+v, want nil", hook)
	}
	if resp == nil || resp.StatusCode != http.StatusBadRequest {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestWebhookSvcUpdate(t *testing.T) {
	var body WebhookUpdateParams
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		if r.URL.Path != "/v1/webhooks/wh_7" {
			t.Errorf("path = %q, want /v1/webhooks/wh_7", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"id":"wh_7","url":"https://example.com/new","event_types":["contact.created"],"disabled":true}`))
	})

	disabled := true
	newURL := "https://example.com/new"
	hook, resp, err := c.Webhooks.Update(context.Background(), "wh_7", &WebhookUpdateParams{
		URL:      &newURL,
		Disabled: &disabled,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if hook.ID != "wh_7" || hook.URL != "https://example.com/new" || !hook.Disabled {
		t.Errorf("unexpected hook: %+v", hook)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d", resp.StatusCode)
	}
	if body.URL == nil || *body.URL != "https://example.com/new" || body.Disabled == nil || !*body.Disabled {
		t.Errorf("unexpected request body: %+v", body)
	}
}

func TestWebhookSvcUpdateError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"conflict","message":"nope"}`))
	})

	hook, resp, err := c.Webhooks.Update(context.Background(), "wh_7", &WebhookUpdateParams{})
	if err == nil {
		t.Fatal("expected error")
	}
	if hook != nil {
		t.Errorf("hook = %+v, want nil", hook)
	}
	if resp == nil || resp.StatusCode != http.StatusConflict {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestWebhookSvcDelete(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		if r.URL.Path != "/v1/webhooks/wh_9" {
			t.Errorf("path = %q, want /v1/webhooks/wh_9", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	resp, err := c.Webhooks.Delete(context.Background(), "wh_9")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want 204", resp.StatusCode)
	}
}

func TestWebhookSvcRotateSecret(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/webhooks/wh_3/rotate-secret" {
			t.Errorf("path = %q, want /v1/webhooks/wh_3/rotate-secret", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":"wh_3","url":"https://example.com/h","event_types":[],"secret":"whsec_rotated"}`))
	})

	hook, resp, err := c.Webhooks.RotateSecret(context.Background(), "wh_3")
	if err != nil {
		t.Fatalf("RotateSecret: %v", err)
	}
	if hook.Secret != "whsec_rotated" {
		t.Errorf("secret = %q, want whsec_rotated", hook.Secret)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestWebhookSvcRotateSecretError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"server_error","message":"boom"}`))
	})

	hook, resp, err := c.Webhooks.RotateSecret(context.Background(), "wh_3")
	if err == nil {
		t.Fatal("expected error")
	}
	if hook != nil {
		t.Errorf("hook = %+v, want nil", hook)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}
}

func TestWebhookSvcVerify(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/webhooks/wh_5/verify" {
			t.Errorf("path = %q, want /v1/webhooks/wh_5/verify", r.URL.Path)
		}
		w.Header().Set("X-Request-ID", "req_verify")
		w.WriteHeader(http.StatusAccepted)
	})

	resp, err := c.Webhooks.Verify(context.Background(), "wh_5")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted || resp.RequestID != "req_verify" {
		t.Errorf("unexpected response: status=%d id=%q", resp.StatusCode, resp.RequestID)
	}
}

func TestWebhookSvcEventTypes(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/v1/webhooks/event-types" {
			t.Errorf("path = %q, want /v1/webhooks/event-types", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"key":"campaign.started","description":"Campaign began sending","group":"campaign","high_volume":false},{"key":"campaign.email_opened","description":"Email opened","high_volume":true}]}`))
	})

	types, resp, err := c.Webhooks.EventTypes(context.Background())
	if err != nil {
		t.Fatalf("EventTypes: %v", err)
	}
	if len(types) != 2 {
		t.Fatalf("got %d types, want 2", len(types))
	}
	if types[0].Key != "campaign.started" || types[0].Group != "campaign" || types[0].HighVolume {
		t.Errorf("unexpected first type: %+v", types[0])
	}
	if !types[1].HighVolume {
		t.Errorf("second type should be high volume: %+v", types[1])
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestWebhookSvcEventTypesError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden","message":"no access"}`))
	})

	types, resp, err := c.Webhooks.EventTypes(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if types != nil {
		t.Errorf("types = %+v, want nil", types)
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestWebhookSvcDeliveries(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/v1/webhooks/wh_8/deliveries" {
			t.Errorf("path = %q, want /v1/webhooks/wh_8/deliveries", r.URL.Path)
		}
		if r.URL.RawQuery != "" {
			t.Errorf("query = %q, want empty (nil ListOptions)", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"del_1","event_type":"campaign.started","status":"succeeded","response_status":200,"attempt":1}],"pagination":{"has_more":false,"next_cursor":null}}`))
	})

	page, err := c.Webhooks.Deliveries(context.Background(), "wh_8", nil)
	if err != nil {
		t.Fatalf("Deliveries: %v", err)
	}
	if len(page.Data) != 1 {
		t.Fatalf("got %d deliveries, want 1", len(page.Data))
	}
	d := page.Data[0]
	if d.ID != "del_1" || d.EventType != "campaign.started" || d.Status != "succeeded" || d.ResponseStatus != 200 || d.Attempt != 1 {
		t.Errorf("unexpected delivery: %+v", d)
	}
}

// TestWebhookSvcConstructEventDecodeError covers ConstructEvent's JSON-decode
// branch: a correctly signed payload whose body is not valid JSON must surface
// the decode error (and not be mistaken for a signature failure).
func TestWebhookSvcConstructEventDecodeError(t *testing.T) {
	c := testClient(t, func(http.ResponseWriter, *http.Request) {})
	const secret = "whsec_decode"
	payload := []byte(`{"id":"evt_1",`) // valid prefix, truncated -> invalid JSON
	sig := ComputeWebhookSignature(payload, secret)

	_, err := c.Webhooks.ConstructEvent(payload, sig, secret)
	if err == nil {
		t.Fatal("expected a decode error for a malformed payload")
	}
	if errors.Is(err, ErrInvalidWebhookSignature) {
		t.Errorf("got signature error, want a JSON decode error: %v", err)
	}
}
