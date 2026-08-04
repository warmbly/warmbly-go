// Command gateway streams realtime events from a workspace and logs them until
// interrupted.
//
//	WARMBLY_API_KEY=wmbly_... WARMBLY_ORG_ID=... go run ./examples/gateway
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
	g := gateway.New(os.Getenv("WARMBLY_API_KEY"), os.Getenv("WARMBLY_ORG_ID"),
		// Narrow the stream to the families this process cares about. Omit this
		// to receive everything the credential may see.
		gateway.WithIntents(gateway.IntentCampaign, gateway.IntentEmail, gateway.IntentWarmup),
		gateway.WithLogger(log.Printf),
	)

	gateway.On(g, gateway.EventEmailOpened, func(_ context.Context, e *gateway.EngagementEvent) {
		log.Printf("opened: contact=%s campaign=%s", e.ContactID, e.CampaignID)
	})
	gateway.On(g, gateway.EventEmailClicked, func(_ context.Context, e *gateway.EngagementEvent) {
		log.Printf("clicked: contact=%s url=%s", e.ContactID, e.URL)
	})
	gateway.On(g, gateway.EventAccountHealthChanged, func(_ context.Context, e *gateway.AccountEvent) {
		log.Printf("mailbox %s health is now %s", e.Email, e.Health)
	})

	// A resume failure means the disconnect outlasted the server's replay
	// buffer: resync from the REST API rather than assuming you saw everything.
	gateway.On(g, gateway.EventResumeFailed, func(_ context.Context, e *gateway.ResumeFailed) {
		log.Printf("resume failed (%s); resync from sequence %d", e.Reason, e.CurrentSeq)
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

	log.Printf("connected at sequence %d; streaming (ctrl-c to quit)", g.Ready().Seq)
	<-ctx.Done()

	// Persist the position so a restart can pick up where this left off with
	// gateway.WithResumeFrom.
	log.Printf("last sequence seen: %d", g.LastSeq())
}
