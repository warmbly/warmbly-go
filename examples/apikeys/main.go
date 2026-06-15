// Command apikeys demonstrates creating and listing API keys with warmbly-go.
//
// Run it with a key that has permission to manage API keys:
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

	created, _, err := client.APIKeys.Create(ctx, &warmbly.APIKeyCreateParams{
		Name:               "ci-bot",
		Scopes:             []string{"campaigns:read", "contacts:read"},
		RateLimitPerMinute: 120,
	})
	if err != nil {
		log.Fatalf("create key: %v", err)
	}
	// The secret is only available right now — store it securely.
	fmt.Printf("created key %s (secret: %s)\n", created.ID, created.Secret)

	page, err := client.APIKeys.List(ctx, nil)
	if err != nil {
		log.Fatalf("list keys: %v", err)
	}
	for key, err := range page.All(ctx) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("- %s (%s…%s)\n", key.Name, key.Prefix, key.Suffix)
	}
}
