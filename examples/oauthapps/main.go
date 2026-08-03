// Command oauthapps demonstrates registering and managing an OAuth 2.1
// application, then using its credentials for machine-to-machine access through
// the client-credentials grant.
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

	// Act as the application itself, with no user in the loop.
	cc := &warmbly.ClientCredentialsConfig{
		ClientID:     app.ClientID,
		ClientSecret: app.ClientSecret,
		Scopes:       []string{"analytics:read"},
	}
	client, err := cc.NewClient(ctx)
	if err != nil {
		log.Fatal(err)
	}
	dash, _, err := client.Analytics.Dashboard(ctx, warmbly.Period7Days)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("dashboard: %d emails sent, %.1f%% reply rate\n",
		dash.OverallStats.TotalEmailsSent, dash.OverallStats.ReplyRate*100)

	// Rotate the secret if it may have leaked. The old one stops working at
	// once, so deploy the new one first.
	rotated, _, err := admin.OAuthApps.RotateSecret(ctx, app.ID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("rotated secret (%d characters)\n", len(rotated))
}
