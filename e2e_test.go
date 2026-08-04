//go:build e2e

// This file drives the SDK against a live Warmbly API. It is gated behind the
// "e2e" build tag so it never runs in unit CI.
//
// Bring up the backend locally, then run:
//
//	WARMBLY_BASE_URL=http://localhost:8080/v1/ \
//	WARMBLY_API_KEY=wmbly_seed_acme_owner_full_access_0000000000 \
//	go test -tags e2e -run TestE2E -v
//
// Both variables default to the local backend and the dev seed key, so a plain
// `go test -tags e2e -v` works against a seeded local stack.
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
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// --- who am I ---

	t.Run("Meta.Identity", func(t *testing.T) {
		me, resp, err := c.Meta.Identity(ctx)
		if err != nil {
			t.Fatalf("identity: %v", err)
		}
		if me.AuthType != warmbly.AuthTypeAPIKey {
			t.Errorf("auth_type = %q, want %q", me.AuthType, warmbly.AuthTypeAPIKey)
		}
		t.Logf("acting as %s in org %v with %d scopes (API-Version %s)",
			me.Email, me.OrganizationID, len(me.Scopes), resp.APIVersion)
	})

	// --- session-only routes reject an API key ---

	t.Run("Organization requires a session token", func(t *testing.T) {
		// Workspace governance is deliberately unreachable with a long-lived
		// key, so the SDK should surface a clean 401.
		_, _, err := c.Organization.Current(ctx)
		if err == nil {
			t.Fatal("expected an error: /organization is not reachable with an API key")
		}
		if !errors.Is(err, warmbly.ErrUnauthorized) {
			t.Fatalf("want ErrUnauthorized, got %v", err)
		}
	})

	// --- read paths across every service ---

	reads := []struct {
		name string
		call func() (string, error)
	}{
		{"Campaigns.List", func() (string, error) {
			p, err := c.Campaigns.List(ctx, nil)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d campaigns (has_more=%v)", len(p.Data), p.HasMore()), nil
		}},
		{"Campaigns.Overview", func() (string, error) {
			o, _, err := c.Campaigns.Overview(ctx)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("total=%d active=%d draft=%d", o.Total, o.Active, o.Draft), nil
		}},
		{"Contacts.Search", func() (string, error) {
			p, err := c.Contacts.Search(ctx, &warmbly.ContactSearchParams{
				ListOptions: warmbly.ListOptions{Limit: 5},
			})
			if err != nil {
				return "", err
			}
			total := 0
			if p.Counts != nil {
				total = p.Counts.Total
			}
			return fmt.Sprintf("%d on page, %d in workspace", len(p.Data), total), nil
		}},
		{"Contacts.CustomFields", func() (string, error) {
			keys, _, err := c.Contacts.CustomFields(ctx)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d custom-field keys", len(keys)), nil
		}},
		{"Emails.List", func() (string, error) {
			p, err := c.Emails.List(ctx, nil)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d mailboxes", len(p.Data)), nil
		}},
		{"Unibox.Overview", func() (string, error) {
			o, _, err := c.Unibox.Overview(ctx)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d threads, %d unread", o.Total, o.Unread), nil
		}},
		{"Templates.List", func() (string, error) {
			ts, _, err := c.Templates.List(ctx, "")
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d templates", len(ts)), nil
		}},
		{"Webhooks.List", func() (string, error) {
			l, _, err := c.Webhooks.List(ctx)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d endpoints, %d event types", len(l.Endpoints), len(l.EventTypes)), nil
		}},
		{"APIKeys.Permissions", func() (string, error) {
			cat, _, err := c.APIKeys.Permissions(ctx)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d scopes, read-only preset=%d", len(cat.Permissions), cat.Presets.ReadOnly), nil
		}},
		{"OAuthApps.List", func() (string, error) {
			apps, _, err := c.OAuthApps.List(ctx)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d applications", len(apps)), nil
		}},
		{"Analytics.Dashboard", func() (string, error) {
			d, _, err := c.Analytics.Dashboard(ctx, warmbly.Period7Days)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("sent=%d opens=%d replies=%d",
				d.OverallStats.TotalEmailsSent, d.OverallStats.TotalOpens, d.OverallStats.TotalReplies), nil
		}},
		{"Analytics.Accounts", func() (string, error) {
			a, _, err := c.Analytics.Accounts(ctx)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d mailbox statuses", len(a)), nil
		}},
		{"Advisor.Summary", func() (string, error) {
			s, _, err := c.Advisor.Summary(ctx)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("score=%d total=%d critical=%d", s.Score, s.Total, s.Critical), nil
		}},
		{"CRM.ListPipelines", func() (string, error) {
			p, _, err := c.CRM.ListPipelines(ctx)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d pipelines", len(p)), nil
		}},
		{"Teams.List", func() (string, error) {
			teams, _, err := c.Teams.List(ctx)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d teams", len(teams)), nil
		}},
		{"Integrations.Catalog", func() (string, error) {
			cat, _, err := c.Integrations.Catalog(ctx)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d providers", len(cat)), nil
		}},
		{"Automations.List", func() (string, error) {
			a, _, err := c.Automations.List(ctx)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d automations", len(a)), nil
		}},
		{"Meetings.Summary", func() (string, error) {
			s, _, err := c.Meetings.Summary(ctx)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("upcoming=%d today=%d", s.Upcoming, s.Today), nil
		}},
		{"Outreach.Get", func() (string, error) {
			s, _, err := c.Outreach.Get(ctx)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("bounce pipeline enabled=%v", s.BouncePipeline.Enabled), nil
		}},
		{"WarmupRouting.List", func() (string, error) {
			r, _, err := c.WarmupRouting.List(ctx)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d routing rules", len(r)), nil
		}},
		{"AuditLogs.List", func() (string, error) {
			p, err := c.AuditLogs.List(ctx, &warmbly.AuditLogListParams{
				ListOptions: warmbly.ListOptions{Limit: 10},
			})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d audit entries", len(p.Data)), nil
		}},
		{"Meta.Timezones", func() (string, error) {
			tz, _, err := c.Meta.Timezones(ctx)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d timezones", len(tz)), nil
		}},
	}

	for _, r := range reads {
		t.Run(r.name, func(t *testing.T) {
			summary, err := r.call()
			if err != nil {
				t.Fatalf("%s: %v", r.name, err)
			}
			t.Log(summary)
		})
	}

	// --- typed error handling against the live server ---

	t.Run("typed not-found error", func(t *testing.T) {
		// A well-formed but nonexistent id makes the server do a real lookup and
		// answer 404; a malformed one would fail id parsing first.
		_, _, err := c.Campaigns.Get(ctx, "00000000-0000-0000-0000-0000000000ff")
		if err == nil {
			t.Fatal("expected an error fetching a nonexistent campaign")
		}
		if !errors.Is(err, warmbly.ErrNotFound) {
			var apiErr *warmbly.Error
			if errors.As(err, &apiErr) {
				t.Logf("got status=%d code=%s", apiErr.StatusCode, apiErr.Code)
			}
			t.Fatalf("want ErrNotFound, got %v", err)
		}
	})

	// --- a full write lifecycle ---

	t.Run("Template lifecycle", func(t *testing.T) {
		name := fmt.Sprintf("e2e-template-%d", time.Now().UnixNano())
		created, _, err := c.Templates.Create(ctx, &warmbly.TemplateCreateParams{
			Name:      name,
			Subject:   "Hello {{first_name}}",
			BodyPlain: "This template was created by the warmbly-go e2e test.",
		})
		if err != nil {
			t.Fatalf("create template: %v", err)
		}
		t.Logf("created template %s", created.ID)
		defer func() {
			if _, err := c.Templates.Delete(ctx, created.ID); err != nil {
				t.Errorf("cleanup delete template %s: %v", created.ID, err)
			}
		}()

		got, _, err := c.Templates.Get(ctx, created.ID)
		if err != nil {
			t.Fatalf("get template: %v", err)
		}
		if got.Name != name {
			t.Errorf("round-tripped name = %q, want %q", got.Name, name)
		}

		updated, _, err := c.Templates.Update(ctx, created.ID, &warmbly.TemplateUpdateParams{
			Subject: warmbly.String("Hey {{first_name}}"),
		})
		if err != nil {
			t.Fatalf("update template: %v", err)
		}
		if updated.Subject != "Hey {{first_name}}" {
			t.Errorf("updated subject = %q", updated.Subject)
		}

		rendered, _, err := c.Templates.Render(ctx, created.ID, map[string]string{"first_name": "Ada"})
		if err != nil {
			t.Fatalf("render template: %v", err)
		}
		t.Logf("rendered subject: %q", rendered.Subject)
	})

	// --- idempotency is honored end to end ---

	t.Run("Idempotency-Key replays", func(t *testing.T) {
		key := fmt.Sprintf("e2e-idem-%d", time.Now().UnixNano())
		name := fmt.Sprintf("e2e-idem-template-%d", time.Now().UnixNano())
		params := &warmbly.TemplateCreateParams{Name: name, Subject: "Idempotent"}

		first, _, err := c.Templates.Create(ctx, params, warmbly.WithIdempotencyKey(key))
		if err != nil {
			t.Fatalf("first create: %v", err)
		}
		defer func() { _, _ = c.Templates.Delete(ctx, first.ID) }()

		second, resp, err := c.Templates.Create(ctx, params, warmbly.WithIdempotencyKey(key))
		if err != nil {
			t.Fatalf("replayed create: %v", err)
		}
		if second.ID != first.ID {
			t.Errorf("replay created a second template: %s vs %s", second.ID, first.ID)
		}
		if !resp.IdempotentReplayed {
			t.Error("expected the response to be marked as a replay")
		}
	})
}
