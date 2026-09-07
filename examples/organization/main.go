// Command organization demonstrates managing the workspace and its people: read
// its settings and limits, list members and roles, invite a teammate, then
// remove them.
//
// These routes are session-only — they reject a long-lived API key, because
// workspace governance should not sit behind a static credential. Sign in
// first, then pass the resulting access token.
//
//	WARMBLY_EMAIL=you@example.com WARMBLY_PASSWORD=... go run ./examples/organization
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/warmbly/warmbly-go"
)

func main() {
	ctx := context.Background()

	// Sign in. The first call emails a code; the second exchanges it for
	// tokens. An account with two-factor on takes one more step, through
	// Auth.VerifyTwoFA.
	anon, err := warmbly.New(warmbly.WithAPIKey("unused"))
	if err != nil {
		log.Fatal(err)
	}
	step, _, err := anon.Auth.Login(ctx, &warmbly.LoginParams{
		Email:    os.Getenv("WARMBLY_EMAIL"),
		Password: os.Getenv("WARMBLY_PASSWORD"),
	})
	if err != nil {
		log.Fatalf("login: %v", err)
	}

	// Whether a code step follows is the deployment's choice, and it is skipped
	// on a device this account has signed in from before. Branch on the answer
	// rather than assuming it.
	tokens := step.Token
	if step.CodeRequired {
		fmt.Print("emailed verification code: ")
		code, _ := bufio.NewReader(os.Stdin).ReadString('\n')

		tokens, _, err = anon.Auth.LoginConfirm(ctx, &warmbly.ConfirmParams{
			Session: step.Session,
			Code:    strings.TrimSpace(code),
		})
		if err != nil {
			log.Fatalf("confirm: %v", err)
		}
	} else if step.TwoFARequired {
		log.Fatal("this account has two-factor enabled; finish with Auth.VerifyTwoFA")
	}
	if tokens == nil || tokens.TwoFARequired {
		log.Fatal("this account has two-factor enabled; finish with Auth.VerifyTwoFA")
	}

	client, err := warmbly.New(warmbly.WithAccessToken(tokens.AccessToken))
	if err != nil {
		log.Fatal(err)
	}

	// The workspace the session acts on, with its plan ceilings and usage.
	org, _, err := client.Organization.Current(ctx)
	if err != nil {
		log.Fatalf("current org: %v", err)
	}
	fmt.Printf("organization %s (%s)\n", org.Name, org.ID)
	if org.Counts != nil {
		fmt.Printf("  %d campaigns, %d contacts, %d mailboxes\n",
			org.Counts.TotalCampaigns, org.Counts.TotalContacts, org.Counts.EmailAccounts)
	}

	// Only what you set is sent, so this renames without touching anything else.
	if _, _, err := client.Organization.Update(ctx, &warmbly.OrganizationUpdateParams{
		Name: warmbly.String(org.Name),
		// The voice profile grounds every AI writing surface in the workspace.
		ProductDescription: warmbly.String("Deliverability tooling for outbound teams."),
	}); err != nil {
		log.Fatalf("update org: %v", err)
	}

	members, _, err := client.Organization.Members(ctx)
	if err != nil {
		log.Fatalf("list members: %v", err)
	}
	for _, m := range members {
		fmt.Printf("- %s <%s> role=%s billing=%v\n",
			m.Name, m.Email, m.Role, m.Can(warmbly.OrgPermManageBilling))
	}

	// Invitations land in a role, so pick one first.
	roles, _, err := client.Organization.Roles(ctx)
	if err != nil {
		log.Fatalf("list roles: %v", err)
	}
	if len(roles) == 0 {
		return
	}

	invite, _, err := client.Organization.Invite(ctx, &warmbly.InviteMemberParams{
		Email:   "teammate@example.com",
		RoleIDs: []string{roles[0].ID},
	})
	if err != nil {
		log.Fatalf("invite member: %v", err)
	}
	fmt.Printf("invited %s as %s\n", invite.Email, roles[0].Name)

	if _, err := client.Organization.CancelInvitation(ctx, invite.ID); err != nil {
		log.Fatalf("cancel invitation: %v", err)
	}
}
