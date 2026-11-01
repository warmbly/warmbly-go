package warmbly

import (
	"context"
	"encoding/json"
	"net/url"
	"time"
)

// Dashboard lists whose layout a member can save with
// [AuthService.SaveViewPreferences]. The server adds a name here before the
// dashboard can persist a layout for a new list; an unknown name is refused.
const (
	// ViewContacts is the contacts list.
	ViewContacts = "contacts"
	// ViewCampaignLeads is a campaign's lead list.
	ViewCampaignLeads = "campaign_leads"
	// ViewUniboxRail is the unified inbox scope rail. It has no columns or
	// sort; its layout is a [UniboxRailLayout] in [ViewPreferences.Layout].
	ViewUniboxRail = "unibox_rail"
)

// ViewSort is the saved ordering of a list, in the contacts search's terms.
type ViewSort struct {
	// By is a sort key such as "created_at", or "custom:<field>" for a custom
	// field. Empty selects the default sort when saving.
	By      string `json:"by"`
	Reverse bool   `json:"reverse"`
}

// ViewPreferences is one member's saved layout for one list in one workspace.
// It is personal: other members never see it.
type ViewPreferences struct {
	View string `json:"view"`
	// Columns are the visible column ids in order: built-in ids or
	// "custom:<field>". Empty means the default layout.
	Columns []string `json:"columns"`
	// Sort is nil for the default sort.
	Sort *ViewSort `json:"sort,omitempty"`
	// Layout is the saved document of a view that keeps one ([ViewUniboxRail]
	// holds a [UniboxRailLayout]); absent when none is saved.
	Layout json.RawMessage `json:"layout,omitempty"`
	// UpdatedAt is when it was last saved; nil when nothing is saved.
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// ViewPreferencesParams is a partial write: a field left nil keeps what is
// saved, so changing the sort cannot erase the columns.
type ViewPreferencesParams struct {
	// Columns replaces the column list. A pointer to an empty slice restores
	// the default layout.
	Columns *[]string `json:"columns,omitempty"`
	// Sort replaces the ordering. A [ViewSort] with an empty By restores the
	// default sort.
	Sort *ViewSort `json:"sort,omitempty"`
	// Layout replaces the layout document of a view that keeps one. Send the
	// JSON value null to restore the default layout.
	Layout json.RawMessage `json:"layout,omitempty"`
}

// UniboxRailLayout is how one member arranges the unified inbox scope rail.
// Keys are rail scope keys such as "folder:inbox" or "mailbox:<id>"; the rail
// ignores keys it no longer shows. Marshal it into
// [ViewPreferencesParams.Layout] for [ViewUniboxRail].
type UniboxRailLayout struct {
	Favorites    []UniboxRailFavorite `json:"favorites"`
	Hidden       []string             `json:"hidden"`
	Order        map[string][]string  `json:"order"`
	SectionOrder []string             `json:"section_order"`
}

// UniboxRailFavorite is a scope pinned to the rail's Favorites section, with
// an optional name the member gave it.
type UniboxRailFavorite struct {
	Key  string `json:"key"`
	Name string `json:"name,omitempty"`
}

// ViewPreferences returns the caller's saved layout for one dashboard list in
// the current workspace, or the empty layout when none is saved. view is one
// of the View* constants. Session-only; the session needs a current workspace.
func (s *AuthService) ViewPreferences(ctx context.Context, view string, opts ...RequestOption) (*ViewPreferences, *Response, error) {
	var out struct {
		Preferences ViewPreferences `json:"preferences"`
	}
	resp, err := s.client.get(ctx, "me/views/"+url.PathEscape(view), &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return &out.Preferences, resp, nil
}

// SaveViewPreferences writes the caller's saved layout for one list: the
// columns, the sort, the layout document, or any of them. Fields left out
// keep their saved value. Column ids and sort keys are validated against the
// list's built-in columns and the workspace's custom fields. Session-only.
func (s *AuthService) SaveViewPreferences(ctx context.Context, view string, params *ViewPreferencesParams, opts ...RequestOption) (*ViewPreferences, *Response, error) {
	var out struct {
		Preferences ViewPreferences `json:"preferences"`
	}
	resp, err := s.client.put(ctx, "me/views/"+url.PathEscape(view), params, &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return &out.Preferences, resp, nil
}

// ResetViewPreferences forgets the caller's saved layout for one list, so the
// dashboard's defaults apply again. Session-only.
func (s *AuthService) ResetViewPreferences(ctx context.Context, view string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "me/views/"+url.PathEscape(view), opts...)
}
