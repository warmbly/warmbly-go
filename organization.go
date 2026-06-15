package warmbly

import (
	"context"
	"net/url"
	"time"
)

// OrganizationService manages the current organization and its members.
//
// Some operations here require a user-context credential (an OAuth access
// token obtained via [WithAccessToken] or [WithTokenSource]) rather than an
// API key, because they act on behalf of a specific user — notably Create,
// List and the member-management methods.
type OrganizationService service

// Organization is an organization (tenant) as returned by the API.
type Organization struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Slug        *string   `json:"slug,omitempty"`
	OwnerUserID string    `json:"owner_user_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// Member is a user's membership in an organization as returned by the API.
type Member struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// OrganizationCreateParams are the parameters for creating an organization.
type OrganizationCreateParams struct {
	Name string `json:"name"`
	Slug string `json:"slug,omitempty"`
}

// OrganizationUpdateParams are the parameters for updating the current
// organization.
type OrganizationUpdateParams struct {
	Name *string `json:"name,omitempty"`
	Slug *string `json:"slug,omitempty"`
}

// InviteMemberParams are the parameters for inviting a member by email.
type InviteMemberParams struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// UpdateMemberParams are the parameters for updating a member.
type UpdateMemberParams struct {
	Role *string `json:"role,omitempty"`
}

// Create provisions a new organization. This requires a user-context
// credential (an OAuth access token).
func (s *OrganizationService) Create(ctx context.Context, params *OrganizationCreateParams) (*Organization, *Response, error) {
	org := new(Organization)
	resp, err := s.client.post(ctx, "organization", params, org)
	if err != nil {
		return nil, resp, err
	}
	return org, resp, nil
}

// List returns the organizations the caller belongs to. This requires a
// user-context credential (an OAuth access token).
func (s *OrganizationService) List(ctx context.Context) ([]Organization, *Response, error) {
	var out struct {
		Data []Organization `json:"data"`
	}
	resp, err := s.client.get(ctx, "organization", &out)
	if err != nil {
		return nil, resp, err
	}
	return out.Data, resp, nil
}

// Current retrieves the organization the current credential acts on.
func (s *OrganizationService) Current(ctx context.Context) (*Organization, *Response, error) {
	org := new(Organization)
	resp, err := s.client.get(ctx, "organization/current", org)
	if err != nil {
		return nil, resp, err
	}
	return org, resp, nil
}

// Update modifies the current organization.
func (s *OrganizationService) Update(ctx context.Context, params *OrganizationUpdateParams) (*Organization, *Response, error) {
	org := new(Organization)
	resp, err := s.client.patch(ctx, "organization/current", params, org)
	if err != nil {
		return nil, resp, err
	}
	return org, resp, nil
}

// Members returns the members of the current organization.
func (s *OrganizationService) Members(ctx context.Context) ([]Member, *Response, error) {
	var out struct {
		Data []Member `json:"data"`
	}
	resp, err := s.client.get(ctx, "organization/members", &out)
	if err != nil {
		return nil, resp, err
	}
	return out.Data, resp, nil
}

// Invite invites a member to the current organization by email. This requires
// a user-context credential (an OAuth access token).
func (s *OrganizationService) Invite(ctx context.Context, params *InviteMemberParams) (*Member, *Response, error) {
	member := new(Member)
	resp, err := s.client.post(ctx, "organization/members/invite", params, member)
	if err != nil {
		return nil, resp, err
	}
	return member, resp, nil
}

// UpdateMember updates a member of the current organization. This requires a
// user-context credential (an OAuth access token).
func (s *OrganizationService) UpdateMember(ctx context.Context, id string, params *UpdateMemberParams) (*Member, *Response, error) {
	member := new(Member)
	resp, err := s.client.patch(ctx, "organization/members/"+url.PathEscape(id), params, member)
	if err != nil {
		return nil, resp, err
	}
	return member, resp, nil
}

// RemoveMember removes a member from the current organization. This requires a
// user-context credential (an OAuth access token).
func (s *OrganizationService) RemoveMember(ctx context.Context, id string) (*Response, error) {
	return s.client.delete(ctx, "organization/members/"+url.PathEscape(id))
}
