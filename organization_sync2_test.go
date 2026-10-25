package warmbly

import (
	"context"
	"testing"
	"time"
)

func TestListInvitationsIncludeExpired(t *testing.T) {
	ctx := context.Background()
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"data":[
	  {"id":"inv_1","organization_id":"org_1","email":"new@example.com","role":"member","permissions":3,"invited_by":"u_1",
	   "expires_at":"2999-01-01T00:00:00Z","created_at":"2026-10-01T00:00:00Z"},
	  {"id":"inv_2","organization_id":"org_1","email":"late@example.com","role":"member","permissions":3,"invited_by":"u_1",
	   "expires_at":"2999-01-01T00:00:00Z","created_at":"2026-09-01T00:00:00Z","expired":true}
	]}`)

	out, _, err := c.Organization.ListInvitations(ctx, &InvitationListParams{IncludeExpired: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.method != "GET" || got.path != "/v1/organization/invitations" || got.rawQuery != "include_expired=true" || got.body != "" {
		t.Errorf("sent %s %s ?%s %q", got.method, got.path, got.rawQuery, got.body)
	}
	if len(out) != 2 || out[0].Lapsed || out[0].Expired() || !out[1].Lapsed || !out[1].Expired() {
		t.Errorf("decoded %+v", out)
	}

	got = recordedRequest{}
	if _, _, err := c.Organization.ListInvitations(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if got.path != "/v1/organization/invitations" || got.rawQuery != "" {
		t.Errorf("default list sent ?%s", got.rawQuery)
	}
}

func TestInvitationExpiredStillFollowsTheClock(t *testing.T) {
	inv := Invitation{ExpiresAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}
	if !inv.Expired() {
		t.Error("an invitation past its expiry must report Expired")
	}
}
