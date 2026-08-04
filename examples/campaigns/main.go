// Command campaigns demonstrates building and running a campaign: create it
// with its sequence, preflight it, send a test, start it, and list campaigns
// with automatic pagination.
//
//	WARMBLY_API_KEY=wmbly_... go run ./examples/campaigns
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

	// Create the campaign with its sequence in one call. Steps can equally be
	// added afterwards with CreateStep and UpdateStep.
	camp, _, err := client.Campaigns.Create(ctx, &warmbly.CampaignCreateParams{
		Name:           "Q3 outbound",
		Description:    "Founder-led outreach to mid-market SaaS",
		DailyLimit:     warmbly.Int(50),
		StopOnReply:    warmbly.Bool(true),
		OpenTracking:   warmbly.Bool(true),
		SenderStrategy: warmbly.String(warmbly.SenderStrategyTags),
		Steps: []warmbly.StepInput{
			{
				Subject:   "Quick question, {{first_name}}",
				BodyPlain: "Hi {{first_name}}, ...",
				WaitAfter: warmbly.Int(0),
			},
			{
				Subject:   "Re: Quick question",
				BodyPlain: "Just floating this back to the top of your inbox.",
				WaitAfter: warmbly.Int(2 * 24 * 60), // two days, in minutes
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("created campaign", camp.ID)

	steps, _, err := client.Campaigns.ListSteps(ctx, camp.ID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("sequence has %d steps\n", len(steps))

	// Run the pre-launch checks and stop if anything blocking failed.
	pre, _, err := client.Campaigns.Preflight(ctx, camp.ID)
	if err != nil {
		log.Fatal(err)
	}
	if !pre.Passed {
		for _, check := range pre.Checks {
			if !check.Passed {
				fmt.Printf("preflight %s (%s): %s — %s\n",
					check.Key, check.Severity, check.Message, check.Remediation)
			}
		}
		return
	}

	// Send yourself a test render from a specific mailbox, then start sending.
	mailboxes, err := client.Emails.List(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}
	if len(mailboxes.Data) > 0 {
		if _, _, err := client.Campaigns.SendTestEmail(ctx, camp.ID, &warmbly.TestEmailParams{
			AccountID: mailboxes.Data[0].ID,
			Recipient: "you@example.com",
		}); err != nil {
			log.Fatal(err)
		}
	}
	if _, _, err := client.Campaigns.Start(ctx, camp.ID); err != nil {
		log.Fatal(err)
	}

	// List campaigns, paging automatically.
	page, err := client.Campaigns.List(ctx, &warmbly.CampaignListParams{Query: "outbound"})
	if err != nil {
		log.Fatal(err)
	}
	for c, err := range page.All(ctx) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("- %s (%s)\n", c.Name, c.Status)
	}
}
