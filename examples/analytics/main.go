// Command analytics reads aggregate analytics over the last 30 days: the
// account dashboard and warmup health.
//
//	WARMBLY_API_KEY=wmbly_... go run ./examples/analytics
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/warmbly/warmbly-go"
)

func main() {
	client, err := warmbly.New(warmbly.WithAPIKey(os.Getenv("WARMBLY_API_KEY")))
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	to := time.Now()
	from := to.AddDate(0, 0, -30)
	window := &warmbly.AnalyticsRange{From: &from, To: &to, Granularity: "day"}

	dash, _, err := client.Analytics.Dashboard(ctx, window)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("sent=%d delivered=%d opens=%d replies=%d bounces=%d\n",
		dash.EmailsSent, dash.Delivered, dash.Opens, dash.Replies, dash.Bounces)
	fmt.Printf("open rate=%.1f%%  reply rate=%.1f%%  active campaigns=%d\n",
		dash.OpenRate*100, dash.ReplyRate*100, dash.ActiveCampaigns)

	warmup, _, err := client.Analytics.Warmup(ctx, window)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("warming=%d  health=%.2f  inbox=%.1f%%  spam=%.1f%%\n",
		warmup.AccountsWarming, warmup.HealthScore, warmup.InboxRate*100, warmup.SpamRate*100)
}
