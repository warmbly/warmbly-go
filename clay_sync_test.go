package warmbly

import (
	"context"
	"testing"
)

const claySettingsJSON = `{"connection_id":"11111111-1111-4111-8111-111111111111","label":"Clay","status":"connected","health":"healthy","inbound_url_minted":true,"defaults":{"campaign_id":"66666666-6666-4666-8666-666666666666","category_ids":["77777777-7777-4777-8777-777777777777"],"segment_ids":[],"dedup":"update","subscribed":true},"outbound":{"webhook_url_set":true,"webhook_host":"app.clay.com","auth_token_set":false,"events":["campaign.reply_received"]},"stats":{"received":12,"created":8,"updated":2,"skipped":1,"failed":1,"last_received_at":"2026-10-17T10:00:00Z","last_error":"bad row"},"available_events":["campaign.reply_received","campaign.email_sent"]}`

func TestClayGet(t *testing.T) {
	var got recordedRequest
	c := fixedClient(t, 200, claySettingsJSON, &got)
	out, _, err := c.Clay.Get(context.Background(), apolloConn)
	if err != nil {
		t.Fatal(err)
	}
	checkRequest(t, got, "GET", "/v1/integrations/clay/"+apolloConn, "", "")
	if !out.InboundURLMinted || out.Defaults.Dedup != ImportDedupUpdate || out.Defaults.CampaignID == nil ||
		out.Outbound.WebhookHost != "app.clay.com" || out.Outbound.AuthTokenSet || out.Stats.Received != 12 ||
		out.Stats.LastReceivedAt == nil || len(out.AvailableEvents) != 2 {
		t.Errorf("decoded %+v", out)
	}
}

func TestClayUpdate(t *testing.T) {
	ctx := context.Background()

	t.Run("partial", func(t *testing.T) {
		var got recordedRequest
		c := fixedClient(t, 200, claySettingsJSON, &got)
		_, _, err := c.Clay.Update(ctx, apolloConn, &ClaySettingsUpdateParams{WebhookURL: String(""), Events: &[]string{}})
		if err != nil {
			t.Fatal(err)
		}
		checkRequest(t, got, "PATCH", "/v1/integrations/clay/"+apolloConn, "", `{"webhook_url":"","events":[]}`)
	})

	t.Run("defaults", func(t *testing.T) {
		var got recordedRequest
		c := fixedClient(t, 200, claySettingsJSON, &got)
		_, _, err := c.Clay.Update(ctx, apolloConn, &ClaySettingsUpdateParams{Defaults: &ClayInboundDefaults{
			CategoryIDs: []string{"cat_1"}, SegmentIDs: []string{}, Dedup: ImportDedupSkip, Subscribed: true,
		}})
		if err != nil {
			t.Fatal(err)
		}
		checkRequest(t, got, "PATCH", "/v1/integrations/clay/"+apolloConn, "", `{"defaults":{"category_ids":["cat_1"],"segment_ids":[],"dedup":"skip","subscribed":true}}`)
	})
}
