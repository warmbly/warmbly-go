// Command oauthapps demonstrates registering and managing an OAuth 2.1
// application, then starting the authorization-code flow a user completes to
// grant it access. See examples/oauth for the full callback and code exchange.
//
//	WARMBLY_API_KEY=wmbly_... go run ./examples/oauthapps
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/warmbly/warmbly-go"
)

func main() {
	// Applications are managed with a key that may manage API credentials.
	admin, err := warmbly.New(warmbly.WithAPIKey(os.Getenv("WARMBLY_API_KEY")))
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	// Scopes are the same bitmask API keys use.
	app, _, err := admin.OAuthApps.Create(ctx, &warmbly.OAuthAppParams{
		Name:         "Reporting Bot",
		Description:  "Reads analytics on a schedule",
		RedirectURIs: []string{"https://app.example.com/callback"},
		Scopes:       warmbly.PermReadAnalytics | warmbly.PermReadCampaigns,

		// An app-level webhook subscription delivers to every workspace that
		// authorizes the app. Its host must sit inside the allowed domains.
		AllowedWebhookDomains: []string{"app.example.com"},
		WebhookURL:            "https://app.example.com/webhooks/warmbly",
		WebhookEvents:         []string{warmbly.EventCampaignCompleted},
	})
	if err != nil {
		log.Fatal(err)
	}
	// The client secret is available only now.
	fmt.Println("registered app", app.ClientID)

	// The app-webhook secret verifies deliveries exactly like an ordinary
	// endpoint's; see VerifyWebhookSignature.
	if hookSecret, _, err := admin.OAuthApps.WebhookSecret(ctx, app.ID); err == nil {
		fmt.Printf("app webhook secret is %d characters\n", len(hookSecret))
	}

	// The app acts for a user who authorizes it. Warmbly supports the
	// authorization-code grant (with PKCE) and refresh tokens; for
	// machine-to-machine access with no user, use an API key instead.
	cfg := &warmbly.OAuth2Config{
		ClientID:     app.ClientID,
		ClientSecret: app.ClientSecret,
		RedirectURL:  "https://app.example.com/callback",
		Scopes:       []string{warmbly.ScopeReadAnalytics},
	}
	verifier := warmbly.GenerateVerifier()
	fmt.Println("send the user to", cfg.AuthCodeURL("state-123", warmbly.S256ChallengeOption(verifier)))
	// On the callback: tok, err := cfg.Exchange(ctx, code, warmbly.VerifierOption(verifier))
	// then client, err := cfg.NewClient(ctx, tok).

	// Rotate the secret if it may have leaked. The old one stops working at
	// once, so deploy the new one first.
	rotated, _, err := admin.OAuthApps.RotateSecret(ctx, app.ID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("rotated secret (%d characters)\n", len(rotated))
}
