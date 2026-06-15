// Command gateway streams real-time events from the Warmbly gateway and logs
// them until interrupted.
//
//	WARMBLY_API_KEY=wmbly_... go run ./examples/gateway
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/warmbly/warmbly-go/gateway"
)

func main() {
	g := gateway.New(os.Getenv("WARMBLY_API_KEY"),
		gateway.WithIntents(gateway.IntentEmailEngagement|gateway.IntentCampaigns|gateway.IntentWarmup),
		gateway.WithLogger(log.Printf),
	)

	gateway.On(g, gateway.EventEmailOpened, func(_ context.Context, e *gateway.EngagementEvent) {
		log.Printf("opened: contact=%s campaign=%s", e.ContactID, e.CampaignID)
	})
	gateway.On(g, gateway.EventEmailClicked, func(_ context.Context, e *gateway.EngagementEvent) {
		log.Printf("clicked: contact=%s url=%s", e.ContactID, e.URL)
	})
	gateway.On(g, gateway.EventWarmupHealthChanged, func(_ context.Context, e *gateway.WarmupEvent) {
		log.Printf("warmup health for %s is now %s (%.0f)", e.EmailAccountID, e.Health, e.Score)
	})
	g.HandleAny(func(_ context.Context, e *gateway.Event) {
		log.Printf("event %s (seq %d)", e.Type, e.Seq)
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := g.Open(ctx); err != nil {
		log.Fatal(err)
	}
	defer g.Close()

	log.Println("connected; streaming events (ctrl-c to quit)")
	<-ctx.Done()
}
