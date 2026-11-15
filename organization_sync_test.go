package warmbly

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestOrganizationMemberAccess(t *testing.T) {
	var got recordedRequest
	c := agentToolServer(t, http.StatusOK, `{
		"scope": "restricted",
		"campaign_folder_ids": ["fld_1"],
		"campaign_ids": ["camp_1", "camp_2"],
		"email_account_ids": []
	}`, &got)

	a, _, err := c.Organization.MemberAccess(context.Background(), "usr_1")
	if err != nil {
		t.Fatalf("MemberAccess: %v", err)
	}
	checkRequest(t, got, "GET", "/v1/organization/members/usr_1/access", "", "")
	if a.Scope != AccessScopeRestricted || len(a.CampaignFolderIDs) != 1 || len(a.CampaignIDs) != 2 || a.EmailAccountIDs == nil || len(a.EmailAccountIDs) != 0 {
		t.Errorf("access = %+v", a)
	}
}

func TestOrganizationSetMemberAccess(t *testing.T) {
	var got recordedRequest
	c := agentToolServer(t, http.StatusOK, `{
		"id": "mem_1", "organization_id": "org_1", "user_id": "usr_1", "role": "member",
		"roles": [{"id": "role_1", "name": "SDR", "color": "#0284c7"}],
		"permissions": 49, "invited_at": "2026-10-01T10:00:00Z", "accepted_at": "2026-10-02T10:00:00Z",
		"access_scope": "restricted",
		"access": {"scope": "restricted", "campaign_folder_ids": [], "campaign_ids": ["camp_1"], "email_account_ids": ["mbx_1"]},
		"email": "sam@acme.test", "name": "Sam"
	}`, &got)

	m, _, err := c.Organization.SetMemberAccess(context.Background(), "usr_1", &MemberAccessParams{
		Scope:           AccessScopeRestricted,
		CampaignIDs:     []string{"camp_1"},
		EmailAccountIDs: []string{"mbx_1"},
	})
	if err != nil {
		t.Fatalf("SetMemberAccess: %v", err)
	}
	checkRequest(t, got, "PUT", "/v1/organization/members/usr_1/access", "",
		`{"scope":"restricted","campaign_ids":["camp_1"],"email_account_ids":["mbx_1"]}`)
	if m.UserID != "usr_1" || m.AccessScope != AccessScopeRestricted || m.ScopeActions != 0 {
		t.Errorf("member = %+v", m)
	}
	if m.Access == nil || len(m.Access.CampaignIDs) != 1 || m.Access.EmailAccountIDs[0] != "mbx_1" {
		t.Errorf("member access = %+v", m.Access)
	}

	// Widening back to the whole workspace sends only the scope.
	if _, _, err := c.Organization.SetMemberAccess(context.Background(), "usr_1", &MemberAccessParams{Scope: AccessScopeWorkspace}); err != nil {
		t.Fatalf("SetMemberAccess: %v", err)
	}
	checkRequest(t, got, "PUT", "/v1/organization/members/usr_1/access", "", `{"scope":"workspace"}`)
}

func TestOrganizationSetMemberAccessOwner(t *testing.T) {
	c := agentToolServer(t, http.StatusForbidden,
		`{"error": "forbidden", "code": "owner_access_unrestricted", "message": "the workspace owner always has access to the entire workspace"}`, nil)
	_, _, err := c.Organization.SetMemberAccess(context.Background(), "usr_owner", &MemberAccessParams{Scope: AccessScopeRestricted})
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "owner_access_unrestricted" {
		t.Fatalf("err = %v", err)
	}
}

func TestOrganizationSuggestedSenders(t *testing.T) {
	ctx := context.Background()
	var got recordedRequest
	c := agentToolServer(t, http.StatusOK, `{"data": [
		{"id": "mbx_1", "email": "sam@acme.test", "name": "Sam", "campaign_ids": ["camp_1", "camp_2"]}
	]}`, &got)

	senders, _, err := c.Organization.SuggestedSenders(ctx, &SuggestedSendersParams{
		CampaignIDs:       []string{"camp_1", "camp_2"},
		CampaignFolderIDs: []string{"fld_1"},
	})
	if err != nil {
		t.Fatalf("SuggestedSenders: %v", err)
	}
	// One comma-separated value per key: the server reads only the first of
	// a repeated key.
	checkRequest(t, got, "GET", "/v1/organization/access/suggested-senders",
		"campaign_folder_ids=fld_1&campaign_ids=camp_1%2Ccamp_2", "")
	if len(senders) != 1 || senders[0].Email != "sam@acme.test" || len(senders[0].CampaignIDs) != 2 {
		t.Errorf("senders = %+v", senders)
	}

	if _, _, err := c.Organization.SuggestedSenders(ctx, nil); err != nil {
		t.Fatalf("SuggestedSenders(nil): %v", err)
	}
	checkRequest(t, got, "GET", "/v1/organization/access/suggested-senders", "", "")
}
