// Command emails demonstrates managing connected mailboxes: listing them,
// driving warmup, checking whether a mailbox is blocked from the warmup pool,
// and sending a one-off message.
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

	page, err := client.Emails.List(ctx, &warmbly.EmailListParams{Query: "acme.com"})
	if err != nil {
		log.Fatal(err)
	}

	for mailbox, err := range page.All(ctx) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s (%s) status=%s warmup=%v auth=%s\n",
			mailbox.Email, mailbox.Provider, mailbox.Status, mailbox.WarmupActive(), mailbox.AuthState)

		// Start warmup where it is not already running. A paused mailbox keeps
		// its ramp progress, so starting it resumes rather than restarts.
		if !mailbox.WarmupActive() {
			if _, _, err := client.Emails.StartWarmup(ctx, mailbox.ID); err != nil {
				log.Printf("start warmup for %s: %v", mailbox.Email, err)
			}
		}

		// Appeal a warmup block when one is in effect and can be appealed.
		ban, _, err := client.Emails.WarmupBanStatus(ctx, mailbox.ID)
		if err == nil && ban.Blocked && ban.CanAppeal && !ban.PendingAppeal {
			appeal, _, err := client.Emails.AppealWarmupBan(ctx, mailbox.ID, &warmbly.WarmupAppealParams{
				Reason: "This mailbox follows sending best practices; please review.",
			})
			if err != nil {
				log.Printf("appeal for %s: %v", mailbox.Email, err)
			} else {
				fmt.Println("  filed appeal", appeal.AppealID)
			}
		}
	}

	if len(page.Data) == 0 {
		return
	}
	first := page.Data[0]

	// Check the sending domain's authentication before trusting deliverability.
	if auth, _, err := client.Emails.AuthCheck(ctx, first.ID); err == nil && !auth.AllAligned {
		fmt.Printf("%s: %s\n", auth.Domain, auth.Summary)
	}

	// Send a one-off message. The idempotency key makes a retry safe: repeating
	// it replays the original response instead of sending twice.
	result, _, err := client.Emails.Send(ctx, first.ID, &warmbly.SendEmailParams{
		To:        []string{"prospect@example.com"},
		Subject:   "Hello from Warmbly",
		BodyPlain: "Hi there — sending this directly from a connected mailbox.",
		SendMode:  warmbly.SendModeSmart,
	}, warmbly.WithIdempotencyKey("example-send-1"))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("queued %s, going out %s (%s)\n",
		result.TaskID, result.ScheduledAt.Format("15:04:05"), result.SendMode)
}
