// Command campaigns demonstrates building and running a campaign: create it,
// add sequence steps, send a test, start it, and list campaigns with automatic
// pagination.
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

	// Create a campaign.
	camp, _, err := client.Campaigns.Create(ctx, &warmbly.CampaignCreateParams{
		Name:           "Q3 outbound",
		Description:    "Founder-led outreach to mid-market SaaS",
		DailyLimit:     50,
		StopOnReply:    true,
		OpenTracking:   true,
		SenderStrategy: "tags",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("created campaign", camp.ID)

	// Add a first email step and a follow-up two days later.
	immediate := 0
	first, _, err := client.Campaigns.CreateStep(ctx, camp.ID, &warmbly.StepParams{
		Kind:      "email",
		Subject:   "Quick question, {{first_name}}",
		BodyPlain: "Hi {{first_name}}, ...",
		WaitAfter: &immediate,
	})
	if err != nil {
		log.Fatal(err)
	}
	followUp := 2 * 24 * 60 // minutes
	if _, _, err := client.Campaigns.CreateStep(ctx, camp.ID, &warmbly.StepParams{
		Kind:      "email",
		Subject:   "Re: Quick question",
		BodyPlain: "Just floating this back to the top of your inbox.",
		WaitAfter: &followUp,
	}); err != nil {
		log.Fatal(err)
	}
	fmt.Println("added steps, first is", first.ID)

	// Send yourself a test render, then start the campaign.
	if _, err := client.Campaigns.TestEmail(ctx, camp.ID, &warmbly.CampaignTestEmailParams{To: "you@example.com"}); err != nil {
		log.Fatal(err)
	}
	if _, _, err := client.Campaigns.Start(ctx, camp.ID); err != nil {
		log.Fatal(err)
	}

	// List running campaigns, paging automatically.
	page, err := client.Campaigns.List(ctx, &warmbly.CampaignListParams{Status: "running"})
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
