//go:build e2e

// This file is an end-to-end test that drives the SDK against a live Warmbly
// API (the backend from the warmbly monorepo). It is gated behind the "e2e"
// build tag so it never runs in unit CI.
//
// Bring up the backend locally, then run:
//
//	WARMBLY_BASE_URL=http://localhost:8080/v1/ \
//	WARMBLY_API_KEY=wmbly_seed_acme_owner_full_access_0000000000 \
//	go test -tags e2e -run TestE2E -v
//
// Both env vars have sensible defaults (the local backend address and the dev
// seed key), so plain `go test -tags e2e -v` works against a seeded local stack.
package warmbly_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/warmbly/warmbly-go"
)

func e2eClient(t *testing.T) *warmbly.Client {
	t.Helper()
	base := os.Getenv("WARMBLY_BASE_URL")
	if base == "" {
		base = "http://localhost:8080/v1/"
	}
	key := os.Getenv("WARMBLY_API_KEY")
	if key == "" {
		key = "wmbly_seed_acme_owner_full_access_0000000000"
	}
	c, err := warmbly.New(
		warmbly.WithAPIKey(key),
		warmbly.WithBaseURL(base),
		warmbly.WithUserAgent("warmbly-go-e2e"),
	)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return c
}

func TestE2E(t *testing.T) {
	c := e2eClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// --- read paths across every service ---

	t.Run("Organization requires a user token", func(t *testing.T) {
		// The backend gates the /organization routes behind a user (JWT) session,
		// not an API key, so an API-key client must receive a 401 that the SDK
		// maps to ErrUnauthorized. (With an OAuth user token these calls succeed.)
		_, _, err := c.Organization.Current(ctx)
		if err == nil {
			t.Fatal("expected an error: /organization is not reachable with an API key")
		}
		if !errors.Is(err, warmbly.ErrUnauthorized) {
			t.Fatalf("want ErrUnauthorized for an API key on /organization, got %v", err)
		}
		t.Logf("organization correctly rejected the API key with 401 (needs a user token)")
	})

	t.Run("Campaigns.List", func(t *testing.T) {
		page, err := c.Campaigns.List(ctx, nil)
		if err != nil {
			t.Fatalf("list campaigns: %v", err)
		}
		t.Logf("campaigns first page: %d (has_more=%v)", len(page.Data), page.HasMore())
	})

	t.Run("Contacts.Search", func(t *testing.T) {
		page, err := c.Contacts.Search(ctx, &warmbly.ContactSearchParams{Limit: 5})
		if err != nil {
			t.Fatalf("search contacts: %v", err)
		}
		t.Logf("contacts found: %d", len(page.Data))
	})

	t.Run("Emails.List", func(t *testing.T) {
		page, err := c.Emails.List(ctx, nil)
		if err != nil {
			t.Fatalf("list emails: %v", err)
		}
		t.Logf("email accounts: %d", len(page.Data))
	})

	t.Run("Templates.List", func(t *testing.T) {
		page, err := c.Templates.List(ctx, nil)
		if err != nil {
			t.Fatalf("list templates: %v", err)
		}
		t.Logf("templates: %d", len(page.Data))
	})

	t.Run("Webhooks.List", func(t *testing.T) {
		page, err := c.Webhooks.List(ctx, nil)
		if err != nil {
			t.Fatalf("list webhooks: %v", err)
		}
		t.Logf("webhooks: %d", len(page.Data))
	})

	t.Run("Webhooks.EventTypes", func(t *testing.T) {
		types, _, err := c.Webhooks.EventTypes(ctx)
		if err != nil {
			t.Fatalf("webhook event types: %v", err)
		}
		t.Logf("webhook event types: %d", len(types))
	})

	t.Run("APIKeys.List", func(t *testing.T) {
		page, err := c.APIKeys.List(ctx, nil)
		if err != nil {
			t.Fatalf("list api keys: %v", err)
		}
		t.Logf("api keys: %d", len(page.Data))
	})

	t.Run("OAuthApps.List", func(t *testing.T) {
		page, err := c.OAuthApps.List(ctx, nil)
		if err != nil {
			t.Fatalf("list oauth apps: %v", err)
		}
		t.Logf("oauth apps: %d", len(page.Data))
	})

	t.Run("Analytics.Dashboard", func(t *testing.T) {
		dash, _, err := c.Analytics.Dashboard(ctx, nil)
		if err != nil {
			t.Fatalf("analytics dashboard: %v", err)
		}
		t.Logf("dashboard: sent=%d opens=%d replies=%d active_campaigns=%d",
			dash.EmailsSent, dash.Opens, dash.Replies, dash.ActiveCampaigns)
	})

	// --- typed error handling against the live server ---

	t.Run("typed not-found error", func(t *testing.T) {
		// Use a well-formed but nonexistent UUID so the server performs a real
		// lookup and returns 404 (a malformed id would fail UUID parsing first).
		_, _, err := c.Campaigns.Get(ctx, "00000000-0000-0000-0000-0000000000ff")
		if err == nil {
			t.Fatal("expected an error fetching a nonexistent campaign")
		}
		if !errors.Is(err, warmbly.ErrNotFound) {
			var apiErr *warmbly.Error
			if errors.As(err, &apiErr) {
				t.Logf("got API error status=%d code=%s (not a 404)", apiErr.StatusCode, apiErr.Code)
			}
			t.Fatalf("want ErrNotFound, got %v", err)
		}
	})

	// --- a full write lifecycle: create -> get -> update -> list -> delete ---

	t.Run("Template lifecycle", func(t *testing.T) {
		name := fmt.Sprintf("e2e-template-%d", time.Now().UnixNano())
		created, _, err := c.Templates.Create(ctx, &warmbly.TemplateCreateParams{
			Name:      name,
			Subject:   "Hello {{first_name}}",
			BodyPlain: "This template was created by the warmbly-go e2e test.",
			Tags:      []string{"e2e"},
		})
		if err != nil {
			t.Fatalf("create template: %v", err)
		}
		t.Logf("created template %s", created.ID)
		// Always clean up, even if a later step fails.
		defer func() {
			if _, err := c.Templates.Delete(ctx, created.ID); err != nil {
				t.Errorf("cleanup delete template %s: %v", created.ID, err)
			} else {
				t.Logf("deleted template %s", created.ID)
			}
		}()

		got, _, err := c.Templates.Get(ctx, created.ID)
		if err != nil {
			t.Fatalf("get template: %v", err)
		}
		if got.Name != name {
			t.Errorf("round-tripped name = %q, want %q", got.Name, name)
		}

		newSubject := "Hey {{first_name}}"
		updated, _, err := c.Templates.Update(ctx, created.ID, &warmbly.TemplateUpdateParams{Subject: &newSubject})
		if err != nil {
			t.Fatalf("update template: %v", err)
		}
		if updated.Subject != newSubject {
			t.Errorf("updated subject = %q, want %q", updated.Subject, newSubject)
		}
	})
}
