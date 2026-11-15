package warmbly

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

func TestAuthViewPreferences(t *testing.T) {
	ctx := context.Background()
	var got recordedRequest

	c := agentToolServer(t, http.StatusOK, `{"preferences": {"view": "contacts", "columns": ["name", "custom:Industry"],
		"sort": {"by": "custom:Industry", "reverse": true}, "updated_at": "2026-11-14T08:00:00Z"}}`, &got)
	p, _, err := c.Auth.ViewPreferences(ctx, ViewContacts)
	if err != nil {
		t.Fatalf("ViewPreferences: %v", err)
	}
	checkRequest(t, got, "GET", "/v1/me/views/contacts", "", "")
	if p.View != ViewContacts || len(p.Columns) != 2 || p.Sort == nil || !p.Sort.Reverse || p.UpdatedAt == nil || p.Layout != nil {
		t.Errorf("preferences = %+v", p)
	}

	// Nothing saved: empty columns, no sort, no timestamp.
	c = agentToolServer(t, http.StatusOK, `{"preferences": {"view": "campaign_leads", "columns": []}}`, &got)
	p, _, err = c.Auth.ViewPreferences(ctx, ViewCampaignLeads)
	if err != nil {
		t.Fatalf("ViewPreferences: %v", err)
	}
	checkRequest(t, got, "GET", "/v1/me/views/campaign_leads", "", "")
	if p.Sort != nil || p.UpdatedAt != nil || p.Columns == nil || len(p.Columns) != 0 {
		t.Errorf("empty preferences = %+v", p)
	}
}

func TestAuthSaveViewPreferences(t *testing.T) {
	ctx := context.Background()
	var got recordedRequest
	c := agentToolServer(t, http.StatusOK, `{"preferences": {"view": "contacts", "columns": [], "updated_at": "2026-11-15T08:00:00Z"}}`, &got)

	cols := []string{}
	p, _, err := c.Auth.SaveViewPreferences(ctx, ViewContacts, &ViewPreferencesParams{Columns: &cols, Sort: &ViewSort{}})
	if err != nil {
		t.Fatalf("SaveViewPreferences: %v", err)
	}
	checkRequest(t, got, "PUT", "/v1/me/views/contacts", "", `{"columns":[],"sort":{"by":"","reverse":false}}`)
	if p.View != ViewContacts || p.UpdatedAt == nil {
		t.Errorf("preferences = %+v", p)
	}

	layout, err := json.Marshal(UniboxRailLayout{
		Favorites:    []UniboxRailFavorite{{Key: "mailbox:mbx_1", Name: "Main"}},
		Hidden:       []string{"folder:spam"},
		Order:        map[string][]string{"mailboxes": {"mailbox:mbx_1"}},
		SectionOrder: []string{"favorites", "mailboxes"},
	})
	if err != nil {
		t.Fatal(err)
	}
	c = agentToolServer(t, http.StatusOK, `{"preferences": {"view": "unibox_rail", "columns": [],
		"layout": {"favorites": [{"key": "mailbox:mbx_1", "name": "Main"}], "hidden": ["folder:spam"],
		"order": {"mailboxes": ["mailbox:mbx_1"]}, "section_order": ["favorites", "mailboxes"]},
		"updated_at": "2026-11-15T08:00:00Z"}}`, &got)
	p, _, err = c.Auth.SaveViewPreferences(ctx, ViewUniboxRail, &ViewPreferencesParams{Layout: layout})
	if err != nil {
		t.Fatalf("SaveViewPreferences: %v", err)
	}
	checkRequest(t, got, "PUT", "/v1/me/views/unibox_rail", "",
		`{"layout":{"favorites":[{"key":"mailbox:mbx_1","name":"Main"}],"hidden":["folder:spam"],"order":{"mailboxes":["mailbox:mbx_1"]},"section_order":["favorites","mailboxes"]}}`)
	var rail UniboxRailLayout
	if err := json.Unmarshal(p.Layout, &rail); err != nil {
		t.Fatalf("decode layout: %v", err)
	}
	if len(rail.Favorites) != 1 || rail.Favorites[0].Name != "Main" || rail.SectionOrder[1] != "mailboxes" {
		t.Errorf("rail = %+v", rail)
	}

	// A JSON null restores the default layout.
	if _, _, err := c.Auth.SaveViewPreferences(ctx, ViewUniboxRail, &ViewPreferencesParams{Layout: json.RawMessage("null")}); err != nil {
		t.Fatalf("SaveViewPreferences: %v", err)
	}
	checkRequest(t, got, "PUT", "/v1/me/views/unibox_rail", "", `{"layout":null}`)
}

func TestAuthSaveViewPreferencesRefused(t *testing.T) {
	c := agentToolServer(t, http.StatusBadRequest, `{"error": "bad_request", "code": "invalid_column", "message": "invalid column: nope"}`, nil)
	cols := []string{"nope"}
	_, _, err := c.Auth.SaveViewPreferences(context.Background(), ViewContacts, &ViewPreferencesParams{Columns: &cols})
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "invalid_column" {
		t.Fatalf("err = %v", err)
	}
}

func TestAuthResetViewPreferences(t *testing.T) {
	var got recordedRequest
	c := agentToolServer(t, http.StatusNoContent, "", &got)
	if _, err := c.Auth.ResetViewPreferences(context.Background(), ViewContacts); err != nil {
		t.Fatalf("ResetViewPreferences: %v", err)
	}
	checkRequest(t, got, "DELETE", "/v1/me/views/contacts", "", "")
}
