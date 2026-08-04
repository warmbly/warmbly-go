// Command templates demonstrates the lifecycle of a reply template: create,
// score the copy for spam risk, update, render, reorder and delete.
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
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("created template", tpl.ID)

	// Score the copy before it ever goes out.
	score, _, err := client.Templates.Score(ctx, &warmbly.TemplateScoreParams{
		Subject:   tpl.Subject,
		BodyPlain: tpl.BodyPlain,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("spam score %d/100\n", score.Score)
	for _, issue := range score.Issues {
		fmt.Printf("  [%s] %s\n", issue.Severity, issue.Message)
	}

	// Update only what you set; nil fields are left alone.
	if _, _, err := client.Templates.Update(ctx, tpl.ID, &warmbly.TemplateUpdateParams{
		Subject: warmbly.String("Hey {{first_name}}"),
	}); err != nil {
		log.Fatal(err)
	}

	// Render it against sample values to check the merge tags resolve.
	rendered, _, err := client.Templates.Render(ctx, tpl.ID, map[string]string{
		"first_name": "Ada",
		"company":    "Analytical Engines",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("rendered subject: %q\n", rendered.Subject)

	templates, _, err := client.Templates.List(ctx, "")
	if err != nil {
		log.Fatal(err)
	}
	ids := make([]string, len(templates))
	for i, t := range templates {
		fmt.Printf("- %d. %s: %q\n", t.Position, t.Name, t.Subject)
		ids[i] = t.ID
	}

	// Reorder by sending the ids in the order you want them.
	if len(ids) > 1 {
		ids[0], ids[len(ids)-1] = ids[len(ids)-1], ids[0]
		if _, _, err := client.Templates.Reorder(ctx, ids); err != nil {
			log.Fatal(err)
		}
	}

	if _, err := client.Templates.Delete(ctx, tpl.ID); err != nil {
		log.Fatal(err)
	}
	fmt.Println("deleted template", tpl.ID)
}
