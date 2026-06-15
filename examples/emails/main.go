// Command emails demonstrates managing connected mailboxes: listing them,
// driving warmup, checking ban status, and sending a one-off message.
//
//	WARMBLY_API_KEY=wmbly_... go run ./examples/emails
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

	page, err := client.Emails.List(ctx, &warmbly.EmailListParams{Status: "active"})
	if err != nil {
		log.Fatal(err)
	}

	for mailbox, err := range page.All(ctx) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s (%s) warmup=%v\n", mailbox.Email, mailbox.Provider, mailbox.WarmupEnabled())

		// Make sure warmup is running.
		if !mailbox.WarmupEnabled() {
			if _, _, err := client.Emails.StartWarmup(ctx, mailbox.ID); err != nil {
				log.Printf("start warmup for %s: %v", mailbox.Email, err)
			}
		}

		// Appeal a warmup ban when one is in effect and appealable.
		if ban, _, err := client.Emails.WarmupBanStatus(ctx, mailbox.ID); err == nil && ban.Banned && ban.Appealable {
			if _, err := client.Emails.AppealWarmupBan(ctx, mailbox.ID, &warmbly.WarmupAppealParams{
				Message: "This mailbox follows sending best practices; please review.",
			}); err != nil {
				log.Printf("appeal for %s: %v", mailbox.Email, err)
			}
		}
	}

	// Send a one-off message from the first mailbox.
	if len(page.Data) > 0 {
		result, _, err := client.Emails.Send(ctx, page.Data[0].ID, &warmbly.SendEmailParams{
			To:        []string{"prospect@example.com"},
			Subject:   "Hello from Warmbly",
			BodyPlain: "Hi there — sending this directly from a connected mailbox.",
		})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("queued message", result.MessageID, "at", result.QueuedAt.Format("15:04:05"))
	}
}
