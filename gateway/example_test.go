package gateway_test

import (
	"context"
	"log"
	"os"

	"github.com/warmbly/warmbly-go/gateway"
)

// Example connects to the gateway, subscribes to engagement and campaign events,
// and dispatches them to typed handlers.
func Example() {
	g := gateway.New(os.Getenv("WARMBLY_API_KEY"),
		gateway.WithIntents(gateway.IntentEmailEngagement|gateway.IntentCampaigns),
	)

	gateway.On(g, gateway.EventEmailOpened, func(_ context.Context, e *gateway.EngagementEvent) {
		log.Printf("contact %s opened a message", e.ContactID)
	})
	gateway.On(g, gateway.EventCampaignCompleted, func(_ context.Context, e *gateway.CampaignEvent) {
		log.Printf("campaign %s completed", e.CampaignID)
	})

	ctx := context.Background()
	if err := g.Open(ctx); err != nil {
		log.Fatal(err)
	}
	defer g.Close()

	<-ctx.Done()
}
