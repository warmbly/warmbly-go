// Command webhooks demonstrates both sides of webhooks: registering an endpoint
// with the API, and running an HTTP receiver that verifies the signature on
// every delivery before acting on it.
//
//	WARMBLY_API_KEY=wmbly_... go run ./examples/webhooks
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/warmbly/warmbly-go"
)

func main() {
	client, err := warmbly.New(warmbly.WithAPIKey(os.Getenv("WARMBLY_API_KEY")))
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	// Discover the available event types.
	types, _, err := client.Webhooks.EventTypes(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d event types available\n", len(types))

	// Register an endpoint subscribed to replies and bounces.
	hook, _, err := client.Webhooks.Create(ctx, &warmbly.WebhookCreateParams{
		URL: "https://app.example.com/webhooks/warmbly",
		EventTypes: []string{
			string(warmbly.EventCampaignReplyReceived),
			string(warmbly.EventCampaignEmailBounced),
		},
		Description: "reply + bounce notifications",
	})
	if err != nil {
		log.Fatal(err)
	}
	// The signing secret is only returned now — store it securely.
	secret := hook.Secret
	fmt.Println("created webhook", hook.ID)

	// Receiver: verify every delivery before trusting its payload.
	http.HandleFunc("/webhooks/warmbly", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		event, err := client.Webhooks.ConstructEvent(body, r.Header.Get(warmbly.WebhookSignatureHeader), secret)
		if err != nil {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		switch warmbly.WebhookEventName(event.Event) {
		case warmbly.EventCampaignReplyReceived:
			log.Printf("reply received (delivery %s)", event.ID)
		case warmbly.EventCampaignEmailBounced:
			log.Printf("email bounced (delivery %s)", event.ID)
		default:
			log.Printf("event %s (delivery %s)", event.Event, event.ID)
		}
		w.WriteHeader(http.StatusOK)
	})

	log.Println("receiver listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
