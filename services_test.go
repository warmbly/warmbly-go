package warmbly

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// bareArrayPaths are the endpoints that answer with a top-level JSON array
// rather than an envelope, so the fixture server has to match.
var bareArrayPaths = map[string]bool{
	"/v1/timezones":                true,
	"/v1/auth/sessions":            true,
	"/v1/auth/passkey/credentials": true,
}

// routingClient records the method, path and query of the last request and
// answers with a body every envelope shape in the SDK can decode: a page, a
// data list, and the handful of single-key wrappers.
func routingClient(t *testing.T, got *recordedRequest) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.path = r.URL.Path
		got.rawQuery = r.URL.RawQuery
		body, _ := io.ReadAll(r.Body)
		got.body = string(body)
		got.idempotencyKey = r.Header.Get("Idempotency-Key")
		w.Header().Set("Content-Type", "application/json")

		// A few endpoints return a bare array; everything else is an envelope.
		if bareArrayPaths[r.URL.Path] ||
			strings.HasSuffix(r.URL.Path, "/steps") ||
			strings.HasSuffix(r.URL.Path, "/move") {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_, _ = w.Write([]byte(`{
			"data": [], "pagination": {"has_more": false},
			"endpoints": [], "event_types": [], "deliveries": [], "drops": [],
			"applications": [], "authorized_apps": [], "connections": [],
			"catalog": [], "events": [], "mappings": [], "runs": [],
			"bookings": [], "automations": [], "rules": [], "plans": [],
			"permissions": [], "notifications": []
		}`))
	}))
	t.Cleanup(srv.Close)

	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

type recordedRequest struct {
	method         string
	path           string
	rawQuery       string
	body           string
	idempotencyKey string
}

// TestServiceRouting exercises at least one call per service against a
// recording server and asserts the method and path. It is the guard against a
// typo in a resource path going unnoticed, since a wrong path would otherwise
// only show up as a 404 at runtime.
func TestServiceRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()
	day := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name       string
		call       func() error
		wantMethod string
		wantPath   string
	}{
		// Mailboxes.
		{"emails.List", func() error { _, e := c.Emails.List(ctx, nil); return e }, "GET", "/v1/emails"},
		{"emails.Get", func() error { _, _, e := c.Emails.Get(ctx, "em_1"); return e }, "GET", "/v1/emails/em_1"},
		{"emails.BulkTag", func() error {
			_, _, e := c.Emails.BulkTag(ctx, &BulkTagParams{EmailIDs: []string{"em_1"}})
			return e
		}, "PATCH", "/v1/emails/tags"},
		{"emails.StartWarmup", func() error { _, _, e := c.Emails.StartWarmup(ctx, "em_1"); return e }, "POST", "/v1/emails/em_1/warmup/start"},
		{"emails.AuthCheck", func() error { _, _, e := c.Emails.AuthCheck(ctx, "em_1"); return e }, "GET", "/v1/emails/em_1/auth-check"},
		{"emails.Verify", func() error { _, _, e := c.Emails.Verify(ctx, "a@b.com"); return e }, "POST", "/v1/emails/verify"},
		{"emails.Send", func() error { _, _, e := c.Emails.Send(ctx, "em_1", &SendEmailParams{}); return e }, "POST", "/v1/emails/em_1/send"},
		{"emails.UpdateTrackingDomain", func() error {
			_, _, e := c.Emails.UpdateTrackingDomain(ctx, "em_1", "t.acme.com")
			return e
		}, "PATCH", "/v1/emails/em_1/track"},
		{"emails.StartOAuth", func() error {
			_, _, e := c.Emails.StartOAuth(ctx, ProviderGmail)
			return e
		}, "POST", "/v1/emails/onboarding/oauth/start"},
		{"emails.ConnectSMTPIMAP", func() error {
			_, _, e := c.Emails.ConnectSMTPIMAP(ctx, &SMTPIMAPParams{Email: "a@b.com"})
			return e
		}, "POST", "/v1/emails/onboarding/smtp-imap"},

		// Campaigns.
		{"campaigns.List", func() error { _, e := c.Campaigns.List(ctx, nil); return e }, "GET", "/v1/campaigns"},
		{"campaigns.Overview", func() error { _, _, e := c.Campaigns.Overview(ctx); return e }, "GET", "/v1/campaigns-overview"},
		{"campaigns.ListSteps", func() error { _, _, e := c.Campaigns.ListSteps(ctx, "camp_1"); return e }, "GET", "/v1/campaigns/camp_1/steps"},
		{"campaigns.UpdateStepLayout", func() error {
			_, e := c.Campaigns.UpdateStepLayout(ctx, "camp_1", nil)
			return e
		}, "PATCH", "/v1/campaigns/camp_1/step-layout"},
		{"campaigns.AdvancedSettings", func() error { _, _, e := c.Campaigns.AdvancedSettings(ctx, "camp_1"); return e }, "GET", "/v1/campaigns/camp_1/advanced"},
		{"campaigns.ListABVariants", func() error { _, _, e := c.Campaigns.ListABVariants(ctx, "camp_1"); return e }, "GET", "/v1/campaigns/camp_1/ab-variants"},
		{"campaigns.ABAnalysis", func() error { _, _, e := c.Campaigns.ABAnalysis(ctx, "camp_1"); return e }, "GET", "/v1/campaigns/camp_1/ab-analysis"},
		{"campaigns.Preflight", func() error { _, _, e := c.Campaigns.Preflight(ctx, "camp_1"); return e }, "POST", "/v1/campaigns/camp_1/preflight"},
		{"campaigns.ListAttachments", func() error { _, _, e := c.Campaigns.ListAttachments(ctx, "camp_1"); return e }, "GET", "/v1/campaigns/camp_1/attachments"},
		{"campaigns.VerifyTrackingDomain", func() error {
			_, _, e := c.Campaigns.VerifyTrackingDomain(ctx, "camp_1")
			return e
		}, "POST", "/v1/campaigns/camp_1/tracking-domain/verify"},
		{"campaigns.PreviewTemplate", func() error {
			_, _, e := c.Campaigns.PreviewTemplate(ctx, &TemplatePreviewParams{})
			return e
		}, "POST", "/v1/campaign-template-preview"},

		// Contacts.
		{"contacts.Search", func() error { _, e := c.Contacts.Search(ctx, &ContactSearchParams{}); return e }, "POST", "/v1/contacts/search"},
		{"contacts.Get", func() error { _, _, e := c.Contacts.Get(ctx, "ct_1"); return e }, "GET", "/v1/contacts/ct_1"},
		{"contacts.Lookup", func() error { _, _, e := c.Contacts.Lookup(ctx, "a@b.com"); return e }, "GET", "/v1/contacts/lookup"},
		{"contacts.CustomFields", func() error { _, _, e := c.Contacts.CustomFields(ctx); return e }, "GET", "/v1/contacts/custom-fields"},
		{"contacts.Timeline", func() error { _, _, e := c.Contacts.Timeline(ctx, "ct_1"); return e }, "GET", "/v1/contacts/ct_1/timeline"},
		{"contacts.Deals", func() error { _, _, e := c.Contacts.Deals(ctx, "ct_1"); return e }, "GET", "/v1/contacts/ct_1/deals"},
		{"contacts.BulkDelete", func() error { _, e := c.Contacts.BulkDelete(ctx, []string{"ct_1"}); return e }, "DELETE", "/v1/contacts"},
		{"contacts.ListResearch", func() error { _, _, e := c.Contacts.ListResearch(ctx, "ct_1", 0); return e }, "GET", "/v1/contacts/ct_1/research"},

		// Unified inbox.
		{"unibox.List", func() error { _, e := c.Unibox.List(ctx, nil); return e }, "GET", "/v1/unibox"},
		{"unibox.Overview", func() error { _, _, e := c.Unibox.Overview(ctx); return e }, "GET", "/v1/unibox/overview"},
		{"unibox.Thread", func() error { _, e := c.Unibox.Thread(ctx, "th_1", "", nil); return e }, "GET", "/v1/unibox/thread"},
		{"unibox.ThreadLabels", func() error { _, _, e := c.Unibox.ThreadLabels(ctx, "th_1"); return e }, "GET", "/v1/unibox/thread/labels"},
		{"unibox.SetThreadLabels", func() error { _, _, e := c.Unibox.SetThreadLabels(ctx, "th_1", nil); return e }, "PUT", "/v1/unibox/thread/labels"},
		{"unibox.MarkSeen", func() error { _, e := c.Unibox.MarkSeen(ctx, []string{"m_1"}, true); return e }, "PATCH", "/v1/unibox/seen"},
		{"unibox.Reply", func() error { _, _, e := c.Unibox.Reply(ctx, &UniboxReplyParams{}); return e }, "POST", "/v1/unibox/reply"},
		{"unibox.Compose", func() error { _, _, e := c.Unibox.Compose(ctx, &UniboxComposeParams{}); return e }, "POST", "/v1/unibox/compose"},
		{"unibox.ComposeCandidates", func() error { _, _, e := c.Unibox.ComposeCandidates(ctx, "a@b.com"); return e }, "GET", "/v1/unibox/compose/candidates"},
		{"unibox.DraftReply", func() error { _, _, e := c.Unibox.DraftReply(ctx, "th_1", ""); return e }, "POST", "/v1/unibox/reply/draft"},
		{"unibox.AgentDrafts", func() error { _, _, e := c.Unibox.AgentDrafts(ctx); return e }, "GET", "/v1/unibox/agent-drafts"},
		{"unibox.Scheduled", func() error { _, _, e := c.Unibox.Scheduled(ctx, ""); return e }, "GET", "/v1/unibox/scheduled"},
		{"unibox.Snoozes", func() error { _, _, e := c.Unibox.Snoozes(ctx); return e }, "GET", "/v1/unibox/snoozes"},

		// Templates.
		{"templates.List", func() error { _, _, e := c.Templates.List(ctx, ""); return e }, "GET", "/v1/templates"},
		{"templates.Reorder", func() error { _, _, e := c.Templates.Reorder(ctx, []string{"t_1"}); return e }, "PATCH", "/v1/templates/reorder"},
		{"templates.Score", func() error { _, _, e := c.Templates.Score(ctx, &TemplateScoreParams{}); return e }, "POST", "/v1/templates/score"},
		{"templates.Render", func() error { _, _, e := c.Templates.Render(ctx, "t_1", nil); return e }, "POST", "/v1/templates/t_1/render"},
		{"templates.Duplicate", func() error { _, _, e := c.Templates.Duplicate(ctx, "t_1"); return e }, "POST", "/v1/templates/t_1/duplicate"},

		// Analytics.
		{"analytics.Dashboard", func() error { _, _, e := c.Analytics.Dashboard(ctx, Period7Days); return e }, "GET", "/v1/analytics/dashboard"},
		{"analytics.Deliverability", func() error {
			_, _, e := c.Analytics.Deliverability(ctx, time.Time{}, time.Time{})
			return e
		}, "GET", "/v1/analytics/deliverability"},
		{"analytics.CampaignDaily", func() error {
			_, _, e := c.Analytics.CampaignDaily(ctx, "camp_1", day, day)
			return e
		}, "GET", "/v1/analytics/campaigns/camp_1/daily"},
		{"analytics.CompareCampaigns", func() error {
			_, _, e := c.Analytics.CompareCampaigns(ctx, []string{"a", "b"}, day, day)
			return e
		}, "GET", "/v1/analytics/campaigns/compare"},
		{"analytics.Accounts", func() error { _, _, e := c.Analytics.Accounts(ctx); return e }, "GET", "/v1/analytics/accounts"},
		{"analytics.Usage", func() error { _, _, e := c.Analytics.Usage(ctx, UsagePeriodDay); return e }, "GET", "/v1/analytics/usage"},

		// Advisor.
		{"advisor.Recommendations", func() error { _, _, e := c.Advisor.Recommendations(ctx, nil); return e }, "GET", "/v1/advisor/recommendations"},
		{"advisor.Summary", func() error { _, _, e := c.Advisor.Summary(ctx); return e }, "GET", "/v1/advisor/summary"},
		{"advisor.Apply", func() error { _, _, e := c.Advisor.Apply(ctx, "f_1"); return e }, "POST", "/v1/advisor/recommendations/f_1/apply"},
		{"advisor.Snooze", func() error { _, e := c.Advisor.Snooze(ctx, "f_1", 7); return e }, "POST", "/v1/advisor/recommendations/f_1/snooze"},
		{"advisor.Settings", func() error { _, _, e := c.Advisor.Settings(ctx); return e }, "GET", "/v1/advisor/settings"},

		// CRM.
		{"crm.ListPipelines", func() error { _, _, e := c.CRM.ListPipelines(ctx); return e }, "GET", "/v1/crm/pipelines"},
		{"crm.CreateStage", func() error {
			_, _, e := c.CRM.CreateStage(ctx, "p_1", &StageCreateParams{Name: "New"})
			return e
		}, "POST", "/v1/crm/pipelines/p_1/stages"},
		{"crm.SearchDeals", func() error { _, e := c.CRM.SearchDeals(ctx, nil); return e }, "POST", "/v1/crm/deals/search"},
		{"crm.DealsSummary", func() error { _, _, e := c.CRM.DealsSummary(ctx, nil); return e }, "POST", "/v1/crm/deals/summary"},
		{"crm.ListTaskTypes", func() error { _, _, e := c.CRM.ListTaskTypes(ctx); return e }, "GET", "/v1/crm/task-types"},
		{"crm.SearchTasks", func() error { _, e := c.CRM.SearchTasks(ctx, nil); return e }, "POST", "/v1/crm/tasks/search"},

		// Teams and meetings.
		{"teams.List", func() error { _, _, e := c.Teams.List(ctx); return e }, "GET", "/v1/teams"},
		{"teams.AddMember", func() error { _, _, e := c.Teams.AddMember(ctx, "t_1", "u_1"); return e }, "POST", "/v1/teams/t_1/members"},
		{"meetings.List", func() error { _, e := c.Meetings.List(ctx, nil); return e }, "GET", "/v1/meetings"},
		{"meetings.Summary", func() error { _, _, e := c.Meetings.Summary(ctx); return e }, "GET", "/v1/meetings/summary"},

		// Integrations and automations.
		{"integrations.Catalog", func() error { _, _, e := c.Integrations.Catalog(ctx); return e }, "GET", "/v1/integrations/catalog"},
		{"integrations.Connections", func() error { _, _, e := c.Integrations.Connections(ctx); return e }, "GET", "/v1/integrations/connections"},
		{"integrations.FieldMappings", func() error { _, _, e := c.Integrations.FieldMappings(ctx, "cn_1"); return e }, "GET", "/v1/integrations/connections/cn_1/field-mappings"},
		{"integrations.Push", func() error { _, _, e := c.Integrations.Push(ctx, "cn_1", nil); return e }, "POST", "/v1/integrations/connections/cn_1/push"},
		{"integrations.Bookings", func() error { _, _, e := c.Integrations.Bookings(ctx); return e }, "GET", "/v1/integrations/bookings"},
		{"automations.List", func() error { _, _, e := c.Automations.List(ctx); return e }, "GET", "/v1/automations"},
		{"automations.Runs", func() error { _, _, e := c.Automations.Runs(ctx, "a_1", 0); return e }, "GET", "/v1/automations/a_1/runs"},
		{"automations.UpdateLayout", func() error { _, e := c.Automations.UpdateLayout(ctx, "a_1", nil); return e }, "PATCH", "/v1/automations/a_1/layout"},

		// Lead sync.
		{"leadsync.Connection", func() error { _, _, e := c.LeadSync.Connection(ctx); return e }, "GET", "/v1/lead-sync/google/connection"},
		{"leadsync.Preview", func() error { _, _, e := c.LeadSync.Preview(ctx, "cn_1", "s_1", "Tab"); return e }, "POST", "/v1/lead-sync/google/preview"},
		{"leadsync.Sources", func() error { _, _, e := c.LeadSync.Sources(ctx); return e }, "GET", "/v1/lead-sync/sources"},
		{"leadsync.Sync", func() error { _, _, e := c.LeadSync.Sync(ctx, "s_1"); return e }, "POST", "/v1/lead-sync/sources/s_1/sync"},

		// AI.
		{"generation.Write", func() error { _, _, e := c.Generation.Write(ctx, &WriteParams{}); return e }, "POST", "/v1/generation/write"},
		{"generation.Edit", func() error { _, _, e := c.Generation.Edit(ctx, &EditParams{}); return e }, "POST", "/v1/generation/edit"},
		{"generation.AIVariable", func() error { _, _, e := c.Generation.AIVariable(ctx, &AIVariableParams{}); return e }, "POST", "/v1/generation/ai-variable"},
		{"skills.List", func() error { _, _, e := c.Skills.List(ctx); return e }, "GET", "/v1/ai/skills"},
		{"assistant.Sessions", func() error { _, e := c.Assistant.Sessions(ctx, nil); return e }, "GET", "/v1/ai/sessions"},
		{"assistant.CreateSession", func() error {
			_, _, e := c.Assistant.CreateSession(ctx, "/app/campaigns", "")
			return e
		}, "POST", "/v1/ai/sessions"},
		{"assistant.Transcript", func() error { _, _, e := c.Assistant.Transcript(ctx, "s_1"); return e }, "GET", "/v1/ai/sessions/s_1/messages"},
		{"assistant.MCPServers", func() error { _, _, e := c.Assistant.MCPServers(ctx); return e }, "GET", "/v1/ai/connections"},
		{"assistant.RefreshMCPServer", func() error {
			_, _, e := c.Assistant.RefreshMCPServer(ctx, "cn_1")
			return e
		}, "POST", "/v1/ai/connections/cn_1/refresh"},

		// Webhooks, keys and OAuth apps.
		{"webhooks.List", func() error { _, _, e := c.Webhooks.List(ctx); return e }, "GET", "/v1/webhooks"},
		{"webhooks.EventTypes", func() error { _, _, e := c.Webhooks.EventTypes(ctx); return e }, "GET", "/v1/webhooks/event-types"},
		{"webhooks.Deliveries", func() error { _, e := c.Webhooks.Deliveries(ctx, nil); return e }, "GET", "/v1/webhooks/deliveries"},
		{"webhooks.Redeliver", func() error { _, e := c.Webhooks.Redeliver(ctx, "d_1"); return e }, "POST", "/v1/webhooks/deliveries/d_1/redeliver"},
		{"webhooks.Drops", func() error { _, _, e := c.Webhooks.Drops(ctx); return e }, "GET", "/v1/webhooks/throttle-drops"},
		{"webhooks.Verify", func() error { _, e := c.Webhooks.Verify(ctx, "wh_1"); return e }, "POST", "/v1/webhooks/wh_1/verify"},
		{"apikeys.List", func() error { _, e := c.APIKeys.List(ctx, nil); return e }, "GET", "/v1/api-keys"},
		{"apikeys.Permissions", func() error { _, _, e := c.APIKeys.Permissions(ctx); return e }, "GET", "/v1/api-keys/permissions"},
		{"apikeys.UsageAnalytics", func() error { _, _, e := c.APIKeys.UsageAnalytics(ctx, nil); return e }, "GET", "/v1/api-keys/usage/analytics"},
		{"apikeys.Logs", func() error { _, e := c.APIKeys.Logs(ctx, "k_1", nil); return e }, "GET", "/v1/api-keys/k_1/logs"},
		{"oauthapps.List", func() error { _, _, e := c.OAuthApps.List(ctx); return e }, "GET", "/v1/oauth/applications"},
		{"oauthapps.WebhookEndpoints", func() error { _, _, e := c.OAuthApps.WebhookEndpoints(ctx, "app_1"); return e }, "GET", "/v1/oauth/applications/app_1/webhook-endpoints"},
		{"oauthapps.AuthorizedApps", func() error { _, _, e := c.OAuthApps.AuthorizedApps(ctx); return e }, "GET", "/v1/oauth/authorized-apps"},
		{"oauthapps.RegisterDynamicClient", func() error {
			_, _, e := c.OAuthApps.RegisterDynamicClient(ctx, &DynamicClientParams{ClientName: "mcp"})
			return e
		}, "POST", "/v1/oauth/register"},

		// Operations.
		{"outreach.Get", func() error { _, _, e := c.Outreach.Get(ctx); return e }, "GET", "/v1/outreach/settings"},
		{"deliverability.Ingest", func() error {
			_, e := c.Deliverability.Ingest(ctx, &DeliverabilityEventParams{})
			return e
		}, "POST", "/v1/deliverability/events"},
		{"warmuprouting.List", func() error { _, _, e := c.WarmupRouting.List(ctx); return e }, "GET", "/v1/warmup/routing"},
		{"tasks.DeadLetters", func() error { _, _, e := c.Tasks.DeadLetters(ctx, nil); return e }, "GET", "/v1/tasks/dlq"},
		{"tasks.Replay", func() error { _, e := c.Tasks.Replay(ctx, "dl_1"); return e }, "POST", "/v1/tasks/dlq/dl_1/replay"},
		{"auditlogs.List", func() error { _, e := c.AuditLogs.List(ctx, nil); return e }, "GET", "/v1/audit-logs"},

		// Groups.
		{"folders.Create", func() error { _, _, e := c.Folders.Create(ctx, &GroupCreateParams{Title: "F"}); return e }, "POST", "/v1/folders"},
		{"tags.Move", func() error { _, _, e := c.Tags.Move(ctx, "g_1", 2); return e }, "PATCH", "/v1/tags/g_1/move"},
		{"categories.Delete", func() error { _, e := c.Categories.Delete(ctx, "g_1"); return e }, "DELETE", "/v1/categories/g_1"},

		// Meta.
		{"meta.Identity", func() error { _, _, e := c.Meta.Identity(ctx); return e }, "GET", "/v1/me"},
		{"meta.Plans", func() error { _, _, e := c.Meta.Plans(ctx); return e }, "GET", "/v1/plans"},
		{"meta.Timezones", func() error { _, _, e := c.Meta.Timezones(ctx); return e }, "GET", "/v1/timezones"},

		// Session-scoped services.
		{"auth.Me", func() error { _, _, e := c.Auth.Me(ctx); return e }, "GET", "/v1/auth/me"},
		{"auth.Sessions", func() error { _, _, e := c.Auth.Sessions(ctx); return e }, "GET", "/v1/auth/sessions"},
		{"auth.TwoFAStatus", func() error { _, _, e := c.Auth.TwoFAStatus(ctx); return e }, "GET", "/v1/auth/2fa/status"},
		{"auth.Notifications", func() error { _, _, e := c.Auth.Notifications(ctx); return e }, "GET", "/v1/auth/me/notifications"},
		{"organization.Current", func() error { _, _, e := c.Organization.Current(ctx); return e }, "GET", "/v1/organization/current"},
		{"organization.Limits", func() error { _, _, e := c.Organization.Limits(ctx); return e }, "GET", "/v1/organization/current/limits"},
		{"organization.Roles", func() error { _, _, e := c.Organization.Roles(ctx); return e }, "GET", "/v1/organization/roles"},
		{"organization.MyInvitations", func() error { _, _, e := c.Organization.MyInvitations(ctx); return e }, "GET", "/v1/invitations"},
		{"organization.DangerZone", func() error { _, _, e := c.Organization.DangerZone(ctx); return e }, "GET", "/v1/organization/current/danger-zone"},
		{"organization.LimitRequests", func() error {
			_, _, e := c.Organization.LimitRequests(ctx, "org_1")
			return e
		}, "GET", "/v1/organization/org_1/limit-requests"},
		{"organization.CancelLimitRequest", func() error {
			_, e := c.Organization.CancelLimitRequest(ctx, "lr_1")
			return e
		}, "DELETE", "/v1/limit-requests/lr_1"},
		{"billing.Get", func() error { _, _, e := c.Billing.Get(ctx); return e }, "GET", "/v1/subscription"},
		{"billing.Credits", func() error { _, _, e := c.Billing.Credits(ctx); return e }, "GET", "/v1/subscription/credits"},
		{"billing.CreditUsage", func() error { _, _, e := c.Billing.CreditUsage(ctx, 30); return e }, "GET", "/v1/subscription/credits/usage"},
		{"billing.Referral", func() error { _, _, e := c.Billing.Referral(ctx); return e }, "GET", "/v1/subscription/referral"},

		// Audiences, the do-not-contact list and hosted forms.
		{"segments.List", func() error { _, _, e := c.Segments.List(ctx); return e }, "GET", "/v1/segments"},
		{"suppressions.List", func() error { _, e := c.Suppressions.List(ctx, nil); return e }, "GET", "/v1/suppressions"},
		{"forms.List", func() error { _, _, e := c.Forms.List(ctx); return e }, "GET", "/v1/forms"},

		// The AI tool registry over plain HTTP.
		{"agentTools.List", func() error { _, _, e := c.AgentTools.List(ctx); return e }, "GET", "/v1/ai/tools"},

		// Session-only workspace surfaces.
		{"websiteTracking.Settings", func() error { _, _, e := c.WebsiteTracking.Settings(ctx); return e }, "GET", "/v1/website-tracking/settings"},
		{"poolLink.ListInstances", func() error { _, _, e := c.PoolLink.ListInstances(ctx); return e }, "GET", "/v1/pool-link/instances"},
		{"cloudLink.Status", func() error { _, _, e := c.CloudLink.Status(ctx); return e }, "GET", "/v1/cloud-link"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got = recordedRequest{}
			if err := tc.call(); err != nil {
				t.Fatalf("call: %v", err)
			}
			if got.method != tc.wantMethod {
				t.Errorf("method = %s, want %s", got.method, tc.wantMethod)
			}
			if got.path != tc.wantPath {
				t.Errorf("path = %s, want %s", got.path, tc.wantPath)
			}
		})
	}
}

// TestListParamsQueryEncoding checks that the list parameter types put their
// filters where the API expects them. A silently dropped filter reads as
// "no results" rather than as an error, so it is worth asserting directly.
func TestListParamsQueryEncoding(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()

	cases := []struct {
		name  string
		call  func() error
		wants []string
	}{
		{
			name: "emails uses q and tag",
			call: func() error {
				_, e := c.Emails.List(ctx, &EmailListParams{
					ListOptions: ListOptions{Limit: 25},
					Query:       "sales",
					Tag:         "tag_1",
				})
				return e
			},
			wants: []string{"q=sales", "tag=tag_1", "limit=25"},
		},
		{
			name: "campaigns uses q and folder",
			call: func() error {
				_, e := c.Campaigns.List(ctx, &CampaignListParams{Query: "launch", Folder: "f_1"})
				return e
			},
			wants: []string{"q=launch", "folder=f_1"},
		},
		{
			name: "unibox flattens id lists",
			call: func() error {
				_, e := c.Unibox.List(ctx, &UniboxListParams{
					EmailIDs:    []string{"a", "b"},
					CategoryIDs: []string{"c"},
					Unseen:      Bool(true),
				})
				return e
			},
			wants: []string{"email_ids=a%2Cb", "category_ids=c", "unseen=true"},
		},
		{
			name: "advisor repeats status",
			call: func() error {
				_, _, e := c.Advisor.Recommendations(ctx, &AdvisorListParams{
					Statuses: []string{AdvisorStatusOpen, AdvisorStatusSnoozed},
					Surface:  AdvisorSurfaceCampaigns,
				})
				return e
			},
			wants: []string{"status=open", "status=snoozed", "surface=campaigns"},
		},
		{
			name: "audit logs pass filters through",
			call: func() error {
				_, e := c.AuditLogs.List(ctx, &AuditLogListParams{
					ActorID:    "u_1",
					EntityType: AuditEntityCampaign,
					Action:     AuditActionUpdate,
				})
				return e
			},
			wants: []string{"actor_id=u_1", "entity_type=campaign", "action=update"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got = recordedRequest{}
			if err := tc.call(); err != nil {
				t.Fatalf("call: %v", err)
			}
			for _, want := range tc.wants {
				if !strings.Contains(got.rawQuery, want) {
					t.Errorf("query = %q, want it to contain %q", got.rawQuery, want)
				}
			}
		})
	}
}

// TestContactSearchSendsFiltersInBody asserts the split the search endpoint
// makes: pagination in the query string, filters in the body. Embedding
// ListOptions must not leak Limit and Cursor into the JSON body.
func TestContactSearchSendsFiltersInBody(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)

	_, err := c.Contacts.Search(context.Background(), &ContactSearchParams{
		ListOptions: ListOptions{Limit: 25, Cursor: "cur_1"},
		Category:    "cat_1",
		Query:       "acme",
		Subscribed:  Bool(true),
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	for _, want := range []string{"limit=25", "cursor=cur_1", "category=cat_1"} {
		if !strings.Contains(got.rawQuery, want) {
			t.Errorf("query = %q, want it to contain %q", got.rawQuery, want)
		}
	}
	if !strings.Contains(got.body, `"query":"acme"`) || !strings.Contains(got.body, `"subscribed":true`) {
		t.Errorf("body = %q, want the filters in it", got.body)
	}
	if strings.Contains(got.body, "Limit") || strings.Contains(got.body, "Cursor") {
		t.Errorf("body = %q, want pagination kept out of it", got.body)
	}
}

// TestNilParamsSendNoBody covers the typed-nil trap: passing an unset *Params
// must send no body at all rather than the JSON literal null, which some
// handlers reject.
func TestNilParamsSendNoBody(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)

	if _, _, err := c.Campaigns.Update(context.Background(), "camp_1", nil); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.body != "" {
		t.Errorf("body = %q, want it empty", got.body)
	}
}

// TestIdempotencyKeyOption checks the per-request key reaches the wire, since
// it is the only thing standing between a retried send and a duplicate email.
func TestIdempotencyKeyOption(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)

	_, _, err := c.Emails.Send(context.Background(), "em_1", &SendEmailParams{
		To: []string{"a@b.com"}, Subject: "hi",
	}, WithIdempotencyKey("key-123"))
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got.idempotencyKey != "key-123" {
		t.Errorf("Idempotency-Key = %q, want key-123", got.idempotencyKey)
	}
}

// TestWithQueryParamReachesWire covers the forward-compatibility escape hatch
// for filters newer than this SDK release.
func TestWithQueryParamReachesWire(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)

	if _, err := c.Campaigns.List(context.Background(), &CampaignListParams{Query: "x"}, WithQueryParam("future", "1")); err != nil {
		t.Fatalf("List: %v", err)
	}
	if !strings.Contains(got.rawQuery, "future=1") || !strings.Contains(got.rawQuery, "q=x") {
		t.Errorf("query = %q, want both the typed and the extra parameter", got.rawQuery)
	}
}

// TestDoEscapeHatch covers reaching an unmodeled endpoint through the generic
// verb, including that the path resolves under the versioned base URL.
func TestDoEscapeHatch(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)

	var out map[string]any
	resp, err := c.Do(context.Background(), http.MethodGet, "some/new/endpoint", nil, &out)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got.path != "/v1/some/new/endpoint" {
		t.Errorf("path = %q", got.path)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d", resp.StatusCode)
	}
}
