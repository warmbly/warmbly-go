// Command contacts demonstrates adding contacts, searching them with the
// faceted filter, reading the contact 360 view, and applying a bulk update.
//
//	WARMBLY_API_KEY=wmbly_... go run ./examples/contacts
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

	created, _, err := client.Contacts.Create(ctx, []warmbly.ContactInput{
		{FirstName: "Ada", LastName: "Lovelace", Email: "ada@example.com", Company: "Analytical Engines"},
		{FirstName: "Alan", LastName: "Turing", Email: "alan@example.com"},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("created %d contacts\n", len(created))

	// Search takes its filters in the body and pages like every other list. The
	// first page also carries workspace-wide facet counts.
	page, err := client.Contacts.Search(ctx, &warmbly.ContactSearchParams{
		ListOptions: warmbly.ListOptions{Limit: 50},
		Query:       "example.com",
		Subscribed:  warmbly.Bool(true),
	})
	if err != nil {
		log.Fatal(err)
	}
	if page.Counts != nil {
		fmt.Printf("%d contacts total, %d subscribed\n", page.Counts.Total, page.Counts.Subscribed)
	}
	for c, err := range page.All(ctx) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s %s <%s> [%s]\n", c.FirstName, c.LastName, c.Email, c.VerificationStatus)
	}

	if len(created) == 0 {
		return
	}

	// The 360 view bundles the contact with its engagement rollup and
	// suppression state in one round trip.
	detail, _, err := client.Contacts.Get(ctx, created[0].ID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s: %d sent, %d replied\n",
		detail.Email, detail.Engagement.TotalSent, detail.Engagement.TotalReplied)
	if detail.Suppression != nil {
		fmt.Printf("  suppressed (%s): %s\n", detail.Suppression.Source, detail.Suppression.Reason)
	}

	// Bulk edits take contact ids plus the changes to apply.
	ids := make([]string, len(created))
	for i, c := range created {
		ids[i] = c.ID
	}
	if _, _, err := client.Contacts.BulkUpdate(ctx, &warmbly.ContactBulkUpdateParams{
		Contacts:  ids,
		Subscribe: warmbly.Bool(true),
		Fields: []warmbly.ContactFieldEdit{
			{Type: warmbly.FieldOpAdd, Key: "source", Value: "sdk-example"},
		},
	}); err != nil {
		log.Fatal(err)
	}
}
