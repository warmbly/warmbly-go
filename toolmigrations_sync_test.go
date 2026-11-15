package warmbly

import (
	"context"
	"testing"
)

const toolMigrationFixture = `{
	"id": "mig_1",
	"organization_id": "org_1",
	"created_by_user_id": "usr_1",
	"provider": "instantly",
	"label": "From Instantly",
	"status": "running",
	"accounts": [{"name": "Acme", "email": "ops@acme.test", "plan": "Hypergrowth"}],
	"inventory": [{
		"workspace": {"id": "", "name": "Acme"},
		"inventory": {"mailboxes": 4, "campaigns": 2, "leads": 900, "blocklist": 12,
			"campaign_summaries": [{"id": "c1", "name": "Q3", "status": "active", "steps": 3, "leads": -1}]}
	}],
	"options": {"kinds": ["blocklist", "campaigns", "leads"], "in_flight": "restart", "skip_finished": true, "warmup": false},
	"targets": [{
		"id": "tgt_1", "migration_id": "mig_1", "workspace": {"id": "", "name": "Acme"},
		"organization_id": "org_1", "organization_name": "Acme", "status": "running", "phase": "leads",
		"counts": {"blocklist_imported": 12, "mailboxes_queued": 0, "mailboxes_manual": 0, "campaigns_created": 2,
			"campaigns_skipped": 0, "steps_created": 5, "leads_read": 300, "contacts_imported": 290, "contacts_updated": 10,
			"leads_enrolled": 280, "leads_not_enrolled": 5, "leads_suppressed": 3, "leads_failed": 2},
		"followup": {"reconnect": [{"email": "a@acme.test", "kind": "google"}], "notes": ["LinkedIn steps were left out."],
			"campaigns": [{"source_id": "c1", "campaign_id": "camp_9", "name": "Q3", "missing_senders": ["b@acme.test"]}]},
		"category_id": "cat_1",
		"started_at": "2026-11-15T10:00:00Z",
		"updated_at": "2026-11-15T10:05:00Z"
	}],
	"credentials_held": true,
	"started_at": "2026-11-15T10:00:00Z",
	"created_at": "2026-11-15T09:55:00Z",
	"updated_at": "2026-11-15T10:05:00Z"
}`

func TestToolMigrationRouting(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name                         string
		call                         func(c *Client) error
		method, path, query, reqBody string
	}{
		{"Create", func(c *Client) error {
			_, _, e := c.ToolMigrations.Create(ctx, &ToolMigrationCreateParams{
				Provider:    "instantly",
				Credentials: []ToolMigrationCredentials{{"api_key": "k_1"}},
			})
			return e
		}, "POST", "/v1/migrations", "", `{"provider":"instantly","credentials":[{"api_key":"k_1"}]}`},
		{"Get", func(c *Client) error { _, _, e := c.ToolMigrations.Get(ctx, "mig_1"); return e },
			"GET", "/v1/migrations/mig_1", "", ""},
		{"Start", func(c *Client) error {
			_, _, e := c.ToolMigrations.Start(ctx, "mig_1", &ToolMigrationStartParams{Options: ToolMigrationOptions{
				Kinds:    []string{ToolMigrationKindBlocklist, ToolMigrationKindCampaigns},
				InFlight: ToolMigrationInFlightRestart,
			}})
			return e
		}, "POST", "/v1/migrations/mig_1/start", "",
			`{"options":{"kinds":["blocklist","campaigns"],"in_flight":"restart","skip_finished":false,"warmup":false}}`},
		{"Start with targets", func(c *Client) error {
			_, _, e := c.ToolMigrations.Start(ctx, "mig_1", &ToolMigrationStartParams{
				Options: ToolMigrationOptions{Kinds: []string{ToolMigrationKindLeads}, InFlight: ToolMigrationInFlightSkip},
				Targets: []ToolMigrationTargetChoice{{WorkspaceID: "w1", OrganizationID: String("org_2")}, {WorkspaceID: "w2", Skip: true}},
			})
			return e
		}, "POST", "/v1/migrations/mig_1/start", "",
			`{"options":{"kinds":["leads"],"in_flight":"skip","skip_finished":false,"warmup":false},"targets":[{"workspace_id":"w1","key":0,"organization_id":"org_2"},{"workspace_id":"w2","key":0,"skip":true}]}`},
		{"Cancel", func(c *Client) error { _, _, e := c.ToolMigrations.Cancel(ctx, "mig_1"); return e },
			"POST", "/v1/migrations/mig_1/cancel", "", ""},
		{"LinkSenders", func(c *Client) error { _, _, e := c.ToolMigrations.LinkSenders(ctx, "mig_1"); return e },
			"POST", "/v1/migrations/mig_1/link-senders", "", ""},
		{"Dismiss", func(c *Client) error { _, e := c.ToolMigrations.Dismiss(ctx, "mig_1"); return e },
			"DELETE", "/v1/migrations/mig_1", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got recordedRequest
			c := recordingFixtureClient(t, &got, toolMigrationFixture)
			if err := tc.call(c); err != nil {
				t.Fatalf("call: %v", err)
			}
			checkRequest(t, got, tc.method, tc.path, tc.query, tc.reqBody)
		})
	}
}

func TestToolMigrationDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, toolMigrationFixture)
	m, _, err := c.ToolMigrations.Get(context.Background(), "mig_1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !m.Active() || m.AgencyID != nil || m.OrganizationID == nil || *m.OrganizationID != "org_1" || !m.CredentialsHeld {
		t.Errorf("migration = %+v", m)
	}
	if len(m.Inventory) != 1 || m.Inventory[0].Inventory.Summaries[0].Leads != -1 {
		t.Errorf("inventory = %+v", m.Inventory)
	}
	if m.Options == nil || m.Options.InFlight != ToolMigrationInFlightRestart || !m.Options.SkipFinished {
		t.Errorf("options = %+v", m.Options)
	}
	tg := m.Targets[0]
	if tg.Phase != ToolMigrationKindLeads || tg.Counts.LeadsEnrolled != 280 || tg.MailboxImportID != nil || tg.CategoryID == nil || tg.FinishedAt != nil {
		t.Errorf("target = %+v", tg)
	}
	if tg.Followup.Reconnect[0].Kind != ToolMigrationMailboxGoogle || tg.Followup.Campaigns[0].MissingSenders[0] != "b@acme.test" {
		t.Errorf("followup = %+v", tg.Followup)
	}
}

func TestToolMigrationList(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"data": [`+toolMigrationFixture+`], "pagination": {"total": null, "next_cursor": null, "has_more": false}}`)
	page, err := c.ToolMigrations.List(context.Background(), &ListOptions{Limit: 5})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	checkRequest(t, got, "GET", "/v1/migrations", "limit=5", "")
	if len(page.Data) != 1 || page.HasMore() {
		t.Errorf("page = %+v", page)
	}
}

func TestToolMigrationProviders(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"data": [{
		"id": "smartlead", "label": "Smartlead",
		"fields": [{"key": "api_key", "label": "API key", "secret": true}, {"key": "region", "label": "Region", "secret": false, "optional": true, "options": ["us", "eu"]}],
		"key_help": "Settings > API", "kinds": ["mailboxes", "campaigns", "leads", "blocklist"],
		"workspaces": true, "multi_key": false, "mailbox_secrets": true, "notes": ["Sequences with AI steps come over as plain steps."]
	}]}`)
	ps, _, err := c.ToolMigrations.Providers(context.Background())
	if err != nil {
		t.Fatalf("Providers: %v", err)
	}
	checkRequest(t, got, "GET", "/v1/migrations/providers", "", "")
	if len(ps) != 1 || ps[0].ID != "smartlead" || !ps[0].Fields[0].Secret || ps[0].Fields[1].Options[1] != "eu" || !ps[0].MailboxSecrets {
		t.Errorf("providers = %+v", ps)
	}
}

// An unknown status or kind must decode as-is so a newer server does not
// break an older client.
func TestToolMigrationUnknownEnums(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"id": "mig_2", "provider": "newtool", "label": "", "status": "paused",
		"accounts": [], "inventory": [], "targets": [{"id": "t", "migration_id": "mig_2", "workspace": {"id": "", "name": ""},
		"organization_id": "org_1", "status": "throttled", "phase": "signatures", "counts": {}, "followup": {},
		"updated_at": "2026-11-15T10:05:00Z"}],
		"credentials_held": false, "created_at": "2026-11-15T09:55:00Z", "updated_at": "2026-11-15T10:05:00Z"}`)
	m, _, err := c.ToolMigrations.Get(context.Background(), "mig_2")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if m.Status != "paused" || m.Active() || m.Targets[0].Status != "throttled" || m.Targets[0].Phase != "signatures" || m.Options != nil {
		t.Errorf("migration = %+v", m)
	}
}
