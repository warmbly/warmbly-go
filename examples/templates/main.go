// Command templates demonstrates the full lifecycle of a message template:
// create, update, list, and delete.
//
//	WARMBLY_API_KEY=wmbly_... go run ./examples/templates
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

	tpl, _, err := client.Templates.Create(ctx, &warmbly.TemplateCreateParams{
		Name:      "Intro",
		Subject:   "Hello {{first_name}}",
		BodyPlain: "Hi {{first_name}}, I noticed {{company}} is hiring...",
		Tags:      []string{"outbound"},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("created template", tpl.ID)

	// Tweak the subject (pointer field: only what you set is sent).
	newSubject := "Hey {{first_name}}"
	if _, _, err := client.Templates.Update(ctx, tpl.ID, &warmbly.TemplateUpdateParams{Subject: &newSubject}); err != nil {
		log.Fatal(err)
	}

	page, err := client.Templates.List(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}
	for t, err := range page.All(ctx) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("- %s: %q\n", t.Name, t.Subject)
	}

	if _, err := client.Templates.Delete(ctx, tpl.ID); err != nil {
		log.Fatal(err)
	}
	fmt.Println("deleted template", tpl.ID)
}
