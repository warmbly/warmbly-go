// Command webhooks demonstrates both sides of webhooks: registering an endpoint
// with the API, and running an HTTP receiver that verifies the signature on
// every delivery before acting on it.
//
//	WARMBLY_API_KEY=wmbly_... go run ./examples/webhooks
package main

import (
	"context"
	"errors"
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

	// The catalog is the authoritative list of deliverable events, and marks
	// the high-volume ones you have to opt into explicitly.
	types, _, err := client.Webhooks.EventTypes(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d event types available\n", len(types))

	// Register an endpoint subscribed to replies and bounces.
	hook, _, err := client.Webhooks.Create(ctx, &warmbly.WebhookCreateParams{
		URL:         "https://app.example.com/webhooks/warmbly",
		Description: "reply + bounce notifications",
		EventTypes: []string{
			warmbly.EventCampaignReplyReceived,
			warmbly.EventCampaignEmailBounced,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	// The signing secret is only returned now — store it securely.
	secret := hook.Secret
	fmt.Println("created webhook", hook.ID)

	// A new endpoint receives nothing until it proves it owns its URL. This
	// sends the challenge; the handler below echoes it back.
	if _, err := client.Webhooks.Verify(ctx, hook.ID); err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/webhooks/warmbly", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Answer the ownership challenge by echoing it back.
		if challenge := r.Header.Get(warmbly.WebhookChallengeHeader); challenge != "" {
			_, _ = w.Write([]byte(challenge))
			return
		}

		// Verify before trusting anything in the payload. ConstructEvent also
		// rejects a stale signature, which defeats replay.
		event, err := client.Webhooks.ConstructEvent(body, r.Header.Get(warmbly.WebhookSignatureHeader), secret)
		switch {
		case errors.Is(err, warmbly.ErrWebhookSignatureExpired):
			http.Error(w, "stale signature", http.StatusUnauthorized)
			return
		case err != nil:
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}

		// Deliveries retry, so deduplicate on the event id before acting.
		switch event.EventType {
		case warmbly.EventCampaignReplyReceived:
			log.Printf("reply received (event %s)", event.ID)
		case warmbly.EventCampaignEmailBounced:
			log.Printf("email bounced (event %s)", event.ID)
		default:
			log.Printf("event %s (%s)", event.EventType, event.ID)
		}
		w.WriteHeader(http.StatusOK)
	})

	log.Println("receiver listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
