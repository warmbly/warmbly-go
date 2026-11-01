package warmbly

import (
	"context"
	"net/url"
	"strings"
)

// Member access scopes returned in [Member.AccessScope] and
// [MemberAccess.Scope]. A restricted member's roles apply only to the campaigns,
// campaign folders and mailboxes granted to them, and a role cannot grant a
// restricted member more than campaign and analytics viewing and the unified
// inbox. The server may add scopes; treat an unknown one as restricted.
const (
	// AccessScopeWorkspace is every resource in the workspace.
	AccessScopeWorkspace = "workspace"
	// AccessScopeRestricted is only the resources granted to the member.
	AccessScopeRestricted = "restricted"
)

// MemberAccess is the resource scope on a member or an invitation.
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

// MemberAccessParams sets a member's (or an invitee's) resource scope. The
// server replaces the whole scope, so send every grant that should remain.
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

// SuggestedSendersParams selects the campaigns and folders to find senders for.
type SuggestedSendersParams struct {
	CampaignIDs       []string
	CampaignFolderIDs []string
}

func (p *SuggestedSendersParams) values() url.Values {
	q := url.Values{}
	if p == nil {
		return q
	}
	if len(p.CampaignIDs) > 0 {
		q.Set("campaign_ids", strings.Join(p.CampaignIDs, ","))
	}
	if len(p.CampaignFolderIDs) > 0 {
		q.Set("campaign_folder_ids", strings.Join(p.CampaignFolderIDs, ","))
	}
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
// member's user ID.
//
// Session-only; needs the manage-team organization permission and a recent
// confirmation ([AuthService.Reauth]). The caller must hold every permission
// the member's roles carry, may not change their own access (403), and may
// not restrict the workspace owner (403, code "owner_access_unrestricted").
// An unknown scope fails with code "invalid_access_scope". Each grant list
// holds at most 500 entries. Recorded in the audit log.
func (s *OrganizationService) SetMemberAccess(ctx context.Context, userID string, params *MemberAccessParams, opts ...RequestOption) (*Member, *Response, error) {
	return send[Member](ctx, s.client.put, "organization/members/"+url.PathEscape(userID)+"/access", params, opts)
}

// SuggestedSenders lists the mailboxes that send for the given campaigns and
// folders, for an administrator to review before granting them to a
// restricted member with [OrganizationService.SetMemberAccess]. At most 1000
// ids may be named in total. Session-only; needs the manage-team organization
// permission.
func (s *OrganizationService) SuggestedSenders(ctx context.Context, params *SuggestedSendersParams, opts ...RequestOption) ([]SuggestedSender, *Response, error) {
	return fetchData[SuggestedSender](ctx, s.client, withQuery("organization/access/suggested-senders", params.values()), opts)
}
