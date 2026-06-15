// Command contacts demonstrates importing contacts, searching them (a POST with
// manual cursor pagination), and applying a bulk update.
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

	// Import a couple of contacts.
	res, _, err := client.Contacts.Create(ctx, []warmbly.ContactInput{
		{FirstName: "Ada", LastName: "Lovelace", Email: "ada@example.com", Company: "Analytical Engines", Tags: []string{"vip"}},
		{FirstName: "Alan", LastName: "Turing", Email: "alan@example.com"},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("created=%d updated=%d skipped=%d\n", res.Created, res.Updated, res.Skipped)

	// Search is a POST; paginate by carrying the cursor forward.
	params := &warmbly.ContactSearchParams{Query: "example.com", Limit: 50}
	for {
		page, err := client.Contacts.Search(ctx, params)
		if err != nil {
			log.Fatal(err)
		}
		for _, c := range page.Data {
			fmt.Printf("%s %s <%s> [%s]\n", c.FirstName, c.LastName, c.Email, c.VerificationStatus)
		}
		if !page.HasMore() {
			break
		}
		params.Cursor = page.NextCursor()
	}

	// Bulk subscribe a set of contacts and tag them.
	subscribed := true
	if _, err := client.Contacts.BulkUpdate(ctx, &warmbly.ContactBulkUpdateParams{
		IDs:        []string{"ct_1", "ct_2"},
		Subscribed: &subscribed,
		AddTags:    []string{"newsletter"},
	}); err != nil {
		log.Fatal(err)
	}
}
