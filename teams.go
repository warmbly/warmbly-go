package warmbly

import (
	"context"
	"net/url"
	"time"
)

// TeamService groups existing workspace members into named teams, which CRM
// tasks and deals can be assigned to instead of a single person.
type TeamService service

// Team is a named group of workspace members.
type Team struct {
	ID             string       `json:"id"`
	OrganizationID string       `json:"organization_id"`
	Name           string       `json:"name"`
	Color          string       `json:"color"`
	Members        []TeamMember `json:"members"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// TeamMember is one member of a team.
type TeamMember struct {
	UserID  string    `json:"user_id"`
	Email   string    `json:"email"`
	Name    string    `json:"name"`
	AddedAt time.Time `json:"added_at"`
}

// TeamCreateParams creates a team.
type TeamCreateParams struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// TeamUpdateParams renames or recolors a team.
type TeamUpdateParams struct {
	Name  *string `json:"name,omitempty"`
	Color *string `json:"color,omitempty"`
}

// List returns the workspace's teams with their members.
func (s *TeamService) List(ctx context.Context, opts ...RequestOption) ([]Team, *Response, error) {
	return fetchData[Team](ctx, s.client, "teams", opts)
}

// Create creates a team.
func (s *TeamService) Create(ctx context.Context, params *TeamCreateParams, opts ...RequestOption) (*Team, *Response, error) {
	return send[Team](ctx, s.client.post, "teams", params, opts)
}

// Get retrieves a single team.
func (s *TeamService) Get(ctx context.Context, id string, opts ...RequestOption) (*Team, *Response, error) {
	return fetch[Team](ctx, s.client, "teams/"+url.PathEscape(id), opts)
}

// Update renames or recolors a team.
func (s *TeamService) Update(ctx context.Context, id string, params *TeamUpdateParams, opts ...RequestOption) (*Team, *Response, error) {
	return send[Team](ctx, s.client.patch, "teams/"+url.PathEscape(id), params, opts)
}

// Delete removes a team. Its members keep their workspace membership.
func (s *TeamService) Delete(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "teams/"+url.PathEscape(id), opts...)
}

// AddMember adds an existing workspace member to a team.
func (s *TeamService) AddMember(ctx context.Context, id, userID string, opts ...RequestOption) (*Team, *Response, error) {
	body := struct {
		UserID string `json:"user_id"`
	}{UserID: userID}
	return send[Team](ctx, s.client.post, "teams/"+url.PathEscape(id)+"/members", body, opts)
}

// RemoveMember removes someone from a team.
func (s *TeamService) RemoveMember(ctx context.Context, id, userID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "teams/"+url.PathEscape(id)+"/members/"+url.PathEscape(userID), opts...)
}
