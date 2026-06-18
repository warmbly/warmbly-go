// Command organization demonstrates managing the current organization and its
// members with warmbly-go: read and rename the org, list members, then invite,
// re-role, and remove a teammate.
//
// Some operations act on behalf of a user rather than an organization —
// notably Create, List, and inviting members — so they require an OAuth user
// token (see the oauth example) instead of an API key.
//
//	WARMBLY_API_KEY=wmbly_... go run ./examples/organization
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

	// The organization the current credential acts on.
	org, _, err := client.Organization.Current(ctx)
	if err != nil {
		log.Fatalf("current org: %v", err)
	}
	fmt.Printf("organization %s (%s)\n", org.Name, org.ID)

	// Rename it (pointer field: only what you set is sent).
	newName := org.Name + " (renamed)"
	if _, _, err := client.Organization.Update(ctx, &warmbly.OrganizationUpdateParams{Name: &newName}); err != nil {
		log.Fatalf("update org: %v", err)
	}

	// List the members of the current organization.
	members, _, err := client.Organization.Members(ctx)
	if err != nil {
		log.Fatalf("list members: %v", err)
	}
	for _, m := range members {
		fmt.Printf("- %s %s <%s> [%s]\n", m.FirstName, m.LastName, m.Email, m.Role)
	}

	// Invite a teammate, promote them to admin, then remove them again.
	invited, _, err := client.Organization.Invite(ctx, &warmbly.InviteMemberParams{
		Email: "teammate@example.com",
		Role:  "member",
	})
	if err != nil {
		log.Fatalf("invite member: %v", err)
	}
	fmt.Println("invited", invited.Email)

	admin := "admin"
	if _, _, err := client.Organization.UpdateMember(ctx, invited.ID, &warmbly.UpdateMemberParams{Role: &admin}); err != nil {
		log.Fatalf("update member: %v", err)
	}

	if _, err := client.Organization.RemoveMember(ctx, invited.ID); err != nil {
		log.Fatalf("remove member: %v", err)
	}
	fmt.Println("removed", invited.Email)

	// Create and List act on behalf of a user, so they need an OAuth user token
	// rather than an API key. With such a client they look like this:
	//
	//	orgs, _, err := client.Organization.List(ctx)        // organizations you belong to
	//	created, _, err := client.Organization.Create(ctx, &warmbly.OrganizationCreateParams{
	//		Name: "Acme, Inc.",
	//		Slug: "acme",
	//	})
}
