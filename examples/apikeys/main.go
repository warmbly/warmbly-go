// Command apikeys demonstrates minting, listing and auditing API keys.
//
// Run it with a key that may manage API keys:
//
//	WARMBLY_API_KEY=wmbly_... go run ./examples/apikeys
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/warmbly/warmbly-go"
)

func main() {
	client, err := warmbly.New(warmbly.WithAPIKey(os.Getenv("WARMBLY_API_KEY")))
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	// Permissions are a bitmask: OR together exactly the scopes the key needs.
	created, _, err := client.APIKeys.Create(ctx, &warmbly.APIKeyCreateParams{
		Name:               "ci-bot",
		Description:        "Reads campaigns and contacts from CI",
		Permissions:        warmbly.PermReadCampaigns | warmbly.PermReadContacts,
		RateLimitPerMinute: 120,
	})
	if err != nil {
		log.Fatalf("create key: %v", err)
	}
	// The secret is available only right now — store it securely.
	fmt.Printf("created key %s (secret: %s)\n", created.ID, created.Secret)

	// The catalog is the source of truth for what each bit means, and its
	// presets stay current as new scopes are added.
	catalog, _, err := client.APIKeys.Permissions(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d scopes available; read-only preset = %d\n",
		len(catalog.Permissions), catalog.Presets.ReadOnly)

	page, err := client.APIKeys.List(ctx, nil)
	if err != nil {
		log.Fatalf("list keys: %v", err)
	}
	for key, err := range page.All(ctx) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("- %s (%s…%s) status=%s send=%v\n",
			key.Name, key.KeyPrefix, key.KeySuffix, key.Status, key.Can(warmbly.PermSendCampaigns))
	}

	// Usage tells you whether a key is still in use before you revoke it.
	summary, _, err := client.APIKeys.UsageSummary(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d active keys, %d requests and %d errors in the last day\n",
		summary.ActiveKeys, summary.Requests24h, summary.Errors24h)

	if _, err := client.APIKeys.Revoke(ctx, created.ID, "example cleanup"); err != nil {
		log.Fatal(err)
	}
}
