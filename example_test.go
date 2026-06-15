package warmbly_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/warmbly/warmbly-go"
)

// Example shows the basic flow: construct a client with an API key and list a
// resource.
func Example() {
	client, err := warmbly.New(warmbly.WithAPIKey("wmbly_..."))
	if err != nil {
		log.Fatal(err)
	}

	page, err := client.Campaigns.List(context.Background(), nil)
	if err != nil {
		log.Fatal(err)
	}
	for _, c := range page.Data {
		fmt.Println(c.Name)
	}
}

// Example_pagination iterates every item across all pages using the auto-paging
// iterator.
func Example_pagination() {
	client, _ := warmbly.New(warmbly.WithAPIKey("wmbly_..."))

	ctx := context.Background()
	page, err := client.Contacts.Search(ctx, &warmbly.ContactSearchParams{Query: "acme.com"})
	if err != nil {
		log.Fatal(err)
	}
	for contact, err := range page.All(ctx) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(contact.Email)
	}
}

// Example_oauth runs the authorization-code flow with PKCE.
func Example_oauth() {
	cfg := &warmbly.OAuth2Config{
		ClientID:     "client_id",
		ClientSecret: "client_secret",
		RedirectURL:  "https://app.example.com/callback",
		Scopes:       []string{"campaigns:read", "contacts:read"},
	}

	verifier := warmbly.GenerateVerifier()
	authURL := cfg.AuthCodeURL("opaque-state", warmbly.S256ChallengeOption(verifier))
	fmt.Println("visit:", authURL)

	// ... after the user authorizes and you receive ?code=... on the callback:
	ctx := context.Background()
	tok, err := cfg.Exchange(ctx, "the-code", warmbly.VerifierOption(verifier))
	if err != nil {
		log.Fatal(err)
	}
	client, err := cfg.NewClient(ctx, tok)
	if err != nil {
		log.Fatal(err)
	}
	_ = client
}

// Example_errorHandling distinguishes API errors with errors.Is and errors.As.
func Example_errorHandling() {
	client, _ := warmbly.New(warmbly.WithAPIKey("wmbly_..."))

	_, _, err := client.Campaigns.Get(context.Background(), "nonexistent")
	switch {
	case errors.Is(err, warmbly.ErrNotFound):
		fmt.Println("no such campaign")
	case errors.Is(err, warmbly.ErrRateLimited):
		fmt.Println("slow down")
	case err != nil:
		var apiErr *warmbly.Error
		if errors.As(err, &apiErr) {
			fmt.Printf("request %s failed: %s\n", apiErr.RequestID, apiErr.Message)
		}
	}
}

// Example_webhookHandler verifies an incoming webhook delivery before acting on
// it.
func Example_webhookHandler() {
	client, _ := warmbly.New(warmbly.WithAPIKey("wmbly_..."))
	const endpointSecret = "whsec_..."

	http.HandleFunc("/webhooks/warmbly", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		event, err := client.Webhooks.ConstructEvent(body, r.Header.Get(warmbly.WebhookSignatureHeader), endpointSecret)
		if err != nil {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		fmt.Println("received", event.Event)
		w.WriteHeader(http.StatusOK)
	})
}
