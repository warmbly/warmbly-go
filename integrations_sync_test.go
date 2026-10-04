package warmbly

import (
	"context"
	"testing"
	"time"
)

func TestIntegrationRotateInboundURL(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, `{"inbound_webhook_url":"https://api.example.com/v1/inbound/calendly/s3cr3t"}`)
	url, _, err := c.Integrations.RotateInboundURL(context.Background(), "conn_1")
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "POST", "/v1/integrations/connections/conn_1/rotate-inbound-url", "")
	got.wantBody(t, "")
	if url != "https://api.example.com/v1/inbound/calendly/s3cr3t" {
		t.Errorf("url = %q", url)
	}
}

func TestIntegrationSetSigningKey(t *testing.T) {
	updated := time.Date(2026, 9, 4, 7, 0, 0, 0, time.UTC)
	conn := IntegrationConnection{
		ID: "conn_1", OrganizationID: "org_1", Provider: ProviderCalendly, Status: ConnectionConnected,
		AuthMethod: "webhook", SyncDirection: SyncPull, Health: "healthy",
		InboundWebhookURL: "https://api.example.com/v1/inbound/calendly/s3cr3t", CreatedAt: updated, UpdatedAt: updated,
	}
	var got syncCapture
	c := respondingClient(t, &got, 200, jsonOf(t, map[string]any{"connection": conn}))
	out, _, err := c.Integrations.SetSigningKey(context.Background(), "conn_1", "whsec_0123456789")
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "PUT", "/v1/integrations/connections/conn_1/signing-key", "")
	got.wantBody(t, `{"signing_key":"whsec_0123456789"}`)
	if out.ID != "conn_1" || out.Provider != ProviderCalendly || out.InboundWebhookURL == "" || !out.UpdatedAt.Equal(updated) {
		t.Errorf("connection = %+v", out)
	}

	// An empty key is sent as an empty string: it clears the key.
	if _, _, err := c.Integrations.SetSigningKey(context.Background(), "conn_1", ""); err != nil {
		t.Fatal(err)
	}
	got.wantBody(t, `{"signing_key":""}`)
}

func TestIntegrationDecodesNewProviderAndRedactedConfig(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, `{"catalog":[],"connections":[{"id":"c1","provider":"cleanmylist","status":"connected","config_capabilities":{"use_cases":["verify"]},"sync_direction":"pull","health":"healthy","created_at":"2026-09-04T07:00:00Z","updated_at":"2026-09-04T07:00:00Z"}]}`)
	conns, _, err := c.Integrations.Connections(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 1 || conns[0].Provider != ProviderCleanMyList {
		t.Errorf("connections = %+v", conns)
	}
}

func TestIntegrationPushStillSendsContactIDs(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, `{"provider":"hubspot","pushed":2,"failed":0,"results":[]}`)
	if _, _, err := c.Integrations.Push(context.Background(), "conn_1", []string{"ct_1", "ct_2"}); err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "POST", "/v1/integrations/connections/conn_1/push", "")
	got.wantBody(t, `{"contact_ids":["ct_1","ct_2"]}`)
}
