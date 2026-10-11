package warmbly

import (
	"context"
	"encoding/json"
	"net/url"
	"time"
)

// Dashboard lists whose layout a member can save, for
// [AuthService.ViewPreferences] and [ViewPreferences.View]. The server refuses
// any other name.
const (
	ViewContacts      = "contacts"
	ViewCampaignLeads = "campaign_leads"
	// ViewUniboxRail is the unified-inbox scope rail. It has no columns or
	// sort; its saved shape is a [UniboxRailLayout] in [ViewPreferences.Layout].
	ViewUniboxRail = "unibox_rail"
)

// ViewSort is the saved ordering of a list, in the contacts search's terms:
// a built-in sort key or "custom:<field>".
type ViewSort struct {
	By      string `json:"by"`
	Reverse bool   `json:"reverse"`
}

// ViewPreferences is one member's saved layout for one dashboard list in the
// current workspace. Empty Columns means the default layout and a nil Sort the
// default sort.
type ViewPreferences struct {
	// View is one of the View* constants.
	View    string    `json:"view"`
	Columns []string  `json:"columns"`
	Sort    *ViewSort `json:"sort,omitempty"`
	// Layout is the saved document of a view that has one ([ViewUniboxRail]);
	// absent when none is saved. Decode it into a [UniboxRailLayout].
	Layout json.RawMessage `json:"layout,omitempty"`
	// UpdatedAt is nil when nothing is saved.
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// UniboxRailLayout is how one member arranges the unified-inbox scope rail.
// Every key is a rail scope key such as "folder:inbox" or "mailbox:<id>".
type UniboxRailLayout struct {
	Favorites    []UniboxRailFavorite `json:"favorites"`
	Hidden       []string             `json:"hidden"`
	Order        map[string][]string  `json:"order"`
	SectionOrder []string             `json:"section_order"`
}

// UniboxRailFavorite is a scope pinned to the rail's Favorites section, with an
// optional name of at most 40 characters.
type UniboxRailFavorite struct {
	Key  string `json:"key"`
	Name string `json:"name,omitempty"`
}

// ViewPreferencesParams is a partial write of a saved layout: a nil field keeps
// what is saved. A non-nil empty Columns restores the default columns, a Sort
// with an empty By the default sort, and a Layout of json.RawMessage("null")
// the default layout of a view that has one.
type ViewPreferencesParams struct {
	// Columns are built-in column ids or "custom:<field>", at most 64.
	Columns *[]string `json:"columns,omitempty"`
	Sort    *ViewSort `json:"sort,omitempty"`
	// Layout is the document for [ViewUniboxRail], for example a marshaled
	// [UniboxRailLayout].
	Layout json.RawMessage `json:"layout,omitempty"`
}

// ViewPreferences returns the caller's saved layout for one dashboard list in
// the current workspace, or the empty layout when none is saved. view is one of
// the View* constants. The route is session-only: an API key has no screen to
// lay out. It needs no organization permission beyond membership.
func (s *AuthService) ViewPreferences(ctx context.Context, view string, opts ...RequestOption) (*ViewPreferences, *Response, error) {
	var out struct {
		Preferences *ViewPreferences `json:"preferences"`
	}
	resp, err := s.client.get(ctx, "me/views/"+url.PathEscape(view), &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Preferences, resp, nil
}

// UpdateViewPreferences saves the caller's layout for one list (the columns,
// the sort, the layout document, or any of them) and returns what is now saved.
// It is session-only.
func (s *AuthService) UpdateViewPreferences(ctx context.Context, view string, params *ViewPreferencesParams, opts ...RequestOption) (*ViewPreferences, *Response, error) {
	if params == nil {
		params = &ViewPreferencesParams{}
	}
	var out struct {
		Preferences *ViewPreferences `json:"preferences"`
	}
	resp, err := s.client.put(ctx, "me/views/"+url.PathEscape(view), params, &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Preferences, resp, nil
}

// ResetViewPreferences forgets the caller's saved layout for one list, so the
// dashboard shows the default again. The server answers 204. It is
// session-only.
func (s *AuthService) ResetViewPreferences(ctx context.Context, view string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "me/views/"+url.PathEscape(view), opts...)
}
