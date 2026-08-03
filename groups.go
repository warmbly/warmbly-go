package warmbly

import (
	"context"
	"net/url"
)

// GroupService manages the three ordered label sets that organize the rest of
// the workspace:
//
//   - Folders group campaigns.
//   - Tags group mailboxes, and are what a campaign's tag-based sender
//     strategy selects on.
//   - Categories group contacts, and double as unified-inbox conversation
//     labels.
//
// Reach them as client.Folders, client.Tags and client.Categories. They share
// this type because the three endpoints are identical apart from their path.
//
// There is no list endpoint: the current set rides along on the caller's
// profile, in [User.Folders], [User.Tags] and [User.Categories]. Read it with
// [AuthService.Me].
type GroupService struct {
	client *Client
	// name is the resource path segment: "folders", "tags" or "categories".
	name string
}

// GroupOrder is one group's position after a reorder.
type GroupOrder struct {
	ID       string `json:"id"`
	Position int32  `json:"position"`
}

// GroupCreateParams creates a group.
type GroupCreateParams struct {
	Title string `json:"title"`
	Color string `json:"color,omitempty"`
}

// GroupUpdateParams renames or recolors a group. Nil fields are unchanged.
type GroupUpdateParams struct {
	Title *string `json:"title,omitempty"`
	Color *string `json:"color,omitempty"`
}

// Create adds a group, appended to the end of the set.
func (s *GroupService) Create(ctx context.Context, params *GroupCreateParams, opts ...RequestOption) (*Group, *Response, error) {
	return send[Group](ctx, s.client, s.client.post, s.name, params, opts)
}

// Update renames or recolors a group.
func (s *GroupService) Update(ctx context.Context, id string, params *GroupUpdateParams, opts ...RequestOption) (*Group, *Response, error) {
	return send[Group](ctx, s.client, s.client.patch, s.name+"/"+url.PathEscape(id), params, opts)
}

// Move reorders a group to the given zero-based position and returns the new
// order of the whole set.
func (s *GroupService) Move(ctx context.Context, id string, position int32, opts ...RequestOption) ([]GroupOrder, *Response, error) {
	body := struct {
		Position int32 `json:"position"`
	}{Position: position}
	return sendSlice[GroupOrder](ctx, s.client, s.client.patch, s.name+"/"+url.PathEscape(id)+"/move", body, opts)
}

// Delete removes a group. What it labelled keeps existing, unlabelled.
func (s *GroupService) Delete(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, s.name+"/"+url.PathEscape(id), opts...)
}
