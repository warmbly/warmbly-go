// Command oauthapps demonstrates registering and managing an OAuth 2.1
// application, then using its credentials for machine-to-machine access via the
// client-credentials grant.
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
	// Manage applications with an admin API key.
	admin, err := warmbly.New(warmbly.WithAPIKey(os.Getenv("WARMBLY_API_KEY")))
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	app, _, err := admin.OAuthApps.Create(ctx, &warmbly.OAuthAppCreateParams{
		Name:         "Reporting Bot",
		Description:  "Reads analytics on a schedule",
		RedirectURIs: []string{"https://app.example.com/callback"},
		Scopes:       []string{"analytics:read", "campaigns:read"},
	})
	if err != nil {
		log.Fatal(err)
	}
	// ClientID and ClientSecret are available now; the secret won't be shown again.
	fmt.Println("registered app", app.ClientID)

	// Use the app's own credentials (no user) to call the API.
	cc := &warmbly.ClientCredentialsConfig{
		ClientID:     app.ClientID,
		ClientSecret: app.ClientSecret,
		Scopes:       []string{"analytics:read"},
	}
	client, err := cc.NewClient(ctx)
	if err != nil {
		log.Fatal(err)
	}
	dash, _, err := client.Analytics.Dashboard(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("dashboard: %d emails sent, %.1f%% reply rate\n", dash.EmailsSent, dash.ReplyRate*100)

	// Rotate the secret if it may have leaked.
	rotated, _, err := admin.OAuthApps.RotateSecret(ctx, app.ID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("rotated secret for", rotated.ClientID)
}
