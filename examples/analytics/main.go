// Command analytics reads aggregate analytics: the workspace dashboard,
// deliverability health over the last 30 days, and per-mailbox status.
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

	dash, _, err := client.Analytics.Dashboard(ctx, warmbly.Period30Days)
	if err != nil {
		log.Fatal(err)
	}
	s := dash.OverallStats
	fmt.Printf("sent=%d opens=%d (machine %d) clicks=%d replies=%d bounces=%d\n",
		s.TotalEmailsSent, s.TotalOpens, s.MachineOpens, s.TotalClicks, s.TotalReplies, s.TotalBounces)
	fmt.Printf("open=%.1f%% reply=%.1f%% bounce=%.1f%%  active campaigns=%d\n",
		s.OpenRate*100, s.ReplyRate*100, s.BounceRate*100, s.ActiveCampaigns)

	// Deliverability health tells you whether the sending itself is in trouble,
	// which the engagement numbers alone will not.
	to := time.Now()
	from := to.AddDate(0, 0, -30)
	deliv, _, err := client.Analytics.Deliverability(ctx, from, to)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("band=%s bounce=%.2f%% complaint=%.2f%% inbox placement=%.1f%%\n",
		deliv.Band, deliv.BounceRate*100, deliv.ComplaintRate*100, deliv.InboxPlacementRate*100)
	for _, mb := range deliv.ByMailbox {
		if mb.Band != warmbly.BandHealthy {
			fmt.Printf("  %s is %s (%.2f%% bounces over %d sends)\n",
				mb.Email, mb.Band, mb.BounceRate*100, mb.Sent)
		}
	}

	// Per-mailbox operational status: health score, recent errors, and how much
	// of today's allowance is spent.
	accounts, _, err := client.Analytics.Accounts(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for _, a := range accounts {
		fmt.Printf("%s: health=%s(%d) campaign %d/%d warmup %d/%d\n",
			a.Email, a.Health.Status, a.Health.Score,
			a.DailyUsage.CampaignSent, a.DailyUsage.CampaignLimit,
			a.DailyUsage.WarmupSent, a.DailyUsage.WarmupLimit)
	}
}
