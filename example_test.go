package warmbly_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/warmbly/warmbly-go"
)

func ExampleNew() {
	client, err := warmbly.New(warmbly.WithAPIKey("wmbly_..."))
	if err != nil {
		log.Fatal(err)
	}

	page, err := client.Campaigns.List(context.Background(), nil)
	if err != nil {
		log.Fatal(err)
	}
	for _, c := range page.Data {
		fmt.Println(c.Name, c.Status)
	}
}

// Auto-paging walks every page for you, fetching the next one only when the
// current one runs out.
func ExamplePage_All() {
	client, _ := warmbly.New(warmbly.WithAPIKey("wmbly_..."))
	ctx := context.Background()

	page, err := client.Emails.List(ctx, &warmbly.EmailListParams{Query: "acme.com"})
	if err != nil {
		log.Fatal(err)
	}
	for mailbox, err := range page.All(ctx) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(mailbox.Email, mailbox.WarmupActive())
	}
}

// Errors decode into a typed *Error that matches the package sentinels, so you
// can branch on the status without parsing the body.
func ExampleError() {
	client, _ := warmbly.New(warmbly.WithAPIKey("wmbly_..."))

	_, _, err := client.Campaigns.Get(context.Background(), "camp_missing")
	switch {
	case errors.Is(err, warmbly.ErrNotFound):
		fmt.Println("no such campaign")
	case errors.Is(err, warmbly.ErrRateLimited):
		var apiErr *warmbly.Error
		errors.As(err, &apiErr)
		fmt.Println("slow down for", apiErr.RetryAfter, "seconds")
	case err != nil:
		var apiErr *warmbly.Error
		if errors.As(err, &apiErr) {
			// The request id is what support needs to find the request.
			fmt.Printf("%s (request %s)\n", apiErr.Message, apiErr.RequestID)
		}
	}
}

// An idempotency key makes a retry safe on anything that sends mail or spends
// money: repeating the key replays the original response instead of acting
// twice.
func ExampleWithIdempotencyKey() {
	client, _ := warmbly.New(warmbly.WithAPIKey("wmbly_..."))

	result, resp, err := client.Emails.Send(context.Background(), "mailbox_id", &warmbly.SendEmailParams{
		To:        []string{"prospect@example.com"},
		Subject:   "Hello",
		BodyPlain: "Hi there.",
	}, warmbly.WithIdempotencyKey("order-4171-welcome"))
	if err != nil {
		log.Fatal(err)
	}
	if resp.IdempotentReplayed {
		fmt.Println("already sent; replayed the original response")
	}
	fmt.Println("queued", result.TaskID, "for", result.ScheduledAt.Format(time.RFC3339))
}

// Verify every webhook delivery before trusting its payload.
func ExampleWebhookService_ConstructEvent() {
	client, _ := warmbly.New(warmbly.WithAPIKey("wmbly_..."))

	var body []byte            // the raw request body, unmodified
	var signatureHeader string // r.Header.Get(warmbly.WebhookSignatureHeader)
	const secret = "whsec_..."

	event, err := client.Webhooks.ConstructEvent(body, signatureHeader, secret)
	if err != nil {
		// Either the signature did not match or it was too old to trust.
		log.Fatal(err)
	}

	switch event.EventType {
	case warmbly.EventCampaignReplyReceived:
		fmt.Println("someone replied")
	case warmbly.EventWarmupBlocked:
		fmt.Println("a mailbox was blocked from the warmup pool")
	}
}

// An agency key ("wmagency_...") reads across the agency's clients, and
// [warmbly.WithAgencyOrganization] points any workspace call at one client.
func ExampleAgencyService_Clients() {
	client, _ := warmbly.New(warmbly.WithAPIKey("wmagency_..."))
	ctx := context.Background()

	clients, _, err := client.Agency.Clients(ctx, "agency-id")
	if err != nil {
		log.Fatal(err)
	}
	for _, c := range clients {
		page, err := client.Campaigns.List(ctx, nil, warmbly.WithAgencyOrganization(c.OrganizationID))
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(c.OrganizationID, len(page.Data))
	}
}

// A single-use sign-in link lets a mailbox owner connect their own Google or
// Microsoft mailbox without handing over a password. Partnership plan only.
func ExampleEmailService_CreateConnectLink() {
	client, _ := warmbly.New(warmbly.WithAPIKey("wmbly_..."))

	link, _, err := client.Emails.CreateConnectLink(context.Background(), &warmbly.MailboxConnectLinkParams{
		Provider:       "gmail",
		Email:          "owner@example.com",
		ExpiresInHours: 48,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(link.URL)
}

func ExampleStoreService_CreateOrder() {
	client, _ := warmbly.New(warmbly.WithAPIKey("wmbly_..."))
	ctx := context.Background()

	quote, _, err := client.Store.CreateQuote(ctx, &warmbly.StoreQuoteParams{
		Items: []warmbly.StoreQuoteItem{
			{Type: warmbly.StoreItemDomain, Domain: "acme-mail.com", Years: 1},
			{Type: warmbly.StoreItemMailboxes, Domain: "acme-mail.com", Provider: warmbly.StoreProviderGoogle,
				Mailboxes: []warmbly.StoreNewMailbox{{LocalPart: "sam", FirstName: "Sam", LastName: "Seller", Admin: true}}},
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	due := quote.DueNowMicros
	order, _, err := client.Store.CreateOrder(ctx, &warmbly.StoreOrderParams{QuoteID: quote.ID, ExpectedDueNowMicros: &due})
	if fresh, ok := warmbly.StorePriceChangedQuote(err); ok {
		// Nothing was charged. Show fresh.DueNowMicros and order again with fresh.ID once confirmed.
		fmt.Println("new total:", fresh.DueNowMicros)
		return
	}
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(order.ID, order.Status)
}
