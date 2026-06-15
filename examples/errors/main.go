// Command errors demonstrates error handling, retry configuration, and reading
// rate-limit state from a response.
//
//	WARMBLY_API_KEY=wmbly_... go run ./examples/errors
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/warmbly/warmbly-go"
)

func main() {
	client, err := warmbly.New(
		warmbly.WithAPIKey(os.Getenv("WARMBLY_API_KEY")),
		warmbly.WithMaxRetries(5),
		warmbly.WithRetryWaitBounds(200*time.Millisecond, 10*time.Second),
	)
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	campaign, resp, err := client.Campaigns.Get(ctx, "camp_does_not_exist")
	switch {
	case errors.Is(err, warmbly.ErrNotFound):
		fmt.Println("no such campaign")
	case errors.Is(err, warmbly.ErrUnauthorized):
		fmt.Println("check your API key")
	case errors.Is(err, warmbly.ErrRateLimited):
		fmt.Println("rate limited — the client already retried with backoff")
	case err != nil:
		// Any other API failure: read the typed error for details.
		var apiErr *warmbly.Error
		if errors.As(err, &apiErr) {
			fmt.Printf("request %s failed (%d %s): %s\n",
				apiErr.RequestID, apiErr.StatusCode, apiErr.Code, apiErr.Message)
		} else {
			log.Fatal(err) // transport-level error
		}
	default:
		fmt.Println("got campaign", campaign.Name)
	}

	// Rate-limit state is parsed from every response.
	if resp != nil {
		fmt.Printf("rate limit: %d/%d remaining\n", resp.RateLimit.Remaining, resp.RateLimit.Limit)
	}
}
