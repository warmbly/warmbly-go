package warmbly

import (
	"context"
	"net/url"
	"strings"
)

// Member access scopes, in [Member.AccessScope] and [MemberAccess.Scope]. A
// restricted member's roles apply only to the campaigns, campaign folders and
// mailboxes granted to them, and inside those grants a role gives them at
// most campaign and analytics viewing, the unified inbox, and managing and
// sending campaigns. Treat an unknown value as restricted.
const (
	// AccessScopeWorkspace is every resource in the workspace.
	AccessScopeWorkspace = "workspace"
	// AccessScopeRestricted is only the resources granted to the member.
	AccessScopeRestricted = "restricted"
)

// MemberAccess is the resource scope set on a member.
type MemberAccess struct {
	// Scope is [AccessScopeWorkspace] or [AccessScopeRestricted].
	Scope string `json:"scope"`
	// CampaignFolderIDs grant every campaign the folder holds at the time of
	// the request. The three lists are empty for a workspace scope.
	CampaignFolderIDs []string `json:"campaign_folder_ids"`
	CampaignIDs       []string `json:"campaign_ids"`
	// EmailAccountIDs are the mailboxes the member may use. Campaign access
	// never grants a mailbox.
	EmailAccountIDs []string `json:"email_account_ids"`
}

// MemberAccessParams replaces a member's resource scope. The server replaces
// the whole scope, so send every grant that should remain; the lists are
// ignored for a workspace scope. Each list holds at most 500 ids.
type MemberAccessParams struct {
	// Scope is [AccessScopeWorkspace] or [AccessScopeRestricted]. Required.
	Scope             string   `json:"scope"`
	CampaignFolderIDs []string `json:"campaign_folder_ids,omitempty"`
	CampaignIDs       []string `json:"campaign_ids,omitempty"`
	EmailAccountIDs   []string `json:"email_account_ids,omitempty"`
}

// SuggestedSender is a mailbox that sends for campaigns an administrator is
// about to grant, offered for review before granting the mailbox itself.
type SuggestedSender struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	// CampaignIDs are the requested campaigns this mailbox sends for.
	CampaignIDs []string `json:"campaign_ids"`
}

// SuggestedSendersParams names the campaigns and campaign folders to find
// senders for. At most 1,000 ids may be named across both lists.
type SuggestedSendersParams struct {
	CampaignIDs       []string
	CampaignFolderIDs []string
}

// values joins each list with commas: the server reads one comma-separated
// value per key, not a repeated key.
func (p *SuggestedSendersParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	setNonEmpty(q, "campaign_ids", strings.Join(p.CampaignIDs, ","))
	setNonEmpty(q, "campaign_folder_ids", strings.Join(p.CampaignFolderIDs, ","))
	return q
}

// MemberAccess returns which resources a member's roles apply to and, for a
// restricted member, the grants. userID is the member's user ID
// ([Member.UserID]). Session-only; needs the manage-team organization
// permission.
func (s *OrganizationService) MemberAccess(ctx context.Context, userID string, opts ...RequestOption) (*MemberAccess, *Response, error) {
	return fetch[MemberAccess](ctx, s.client, "organization/members/"+url.PathEscape(userID)+"/access", opts)
}

// SetMemberAccess replaces a member's resource scope and returns the updated
// member, whose [Member.Access] carries the stored grants. userID is the
// member's user ID ([Member.UserID]). A retry lands on the same state.
//
// The caller must hold every permission the member's roles carry and may not
// change their own access (403). The workspace owner cannot be restricted
// (403 "owner_access_unrestricted"). An unknown scope fails with 400
// "invalid_access_scope", a list over 500 ids with 400
// "too_many_access_grants", and a campaign, folder or mailbox outside the
// workspace with 400 "invalid_access_resource". Session-only; needs the manage-team
// organization permission and a recently confirmed session (see
// [AuthService.Reauth]).
func (s *OrganizationService) SetMemberAccess(ctx context.Context, userID string, params *MemberAccessParams, opts ...RequestOption) (*Member, *Response, error) {
	return send[Member](ctx, s.client.put, "organization/members/"+url.PathEscape(userID)+"/access", params, opts)
}

// SuggestedSenders lists the mailboxes that send for the given campaigns and
// folders, for an administrator to review before granting them with
// [OrganizationService.SetMemberAccess]. Session-only; needs the manage-team
// organization permission.
func (s *OrganizationService) SuggestedSenders(ctx context.Context, params *SuggestedSendersParams, opts ...RequestOption) ([]SuggestedSender, *Response, error) {
	return fetchData[SuggestedSender](ctx, s.client, withQuery("organization/access/suggested-senders", params.values()), opts)
}
