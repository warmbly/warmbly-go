package warmbly

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

const slackLinkFixture = `{
	"id": "lnk_1", "organization_id": "org_1", "connection_id": "conn_1",
	"slack_team_id": "T0001", "slack_user_id": "U0001", "user_id": "usr_1",
	"dm_notifications": true,
	"created_at": "2026-11-01T10:00:00Z", "updated_at": "2026-11-02T10:00:00Z",
	"user_name": "Sam", "user_email": "sam@acme.test"
}`

func TestSlackStatus(t *testing.T) {
	var got recordedRequest
	c := agentToolServer(t, http.StatusOK, `{
		"app_configured": true,
		"interactive_configured": true,
		"connection": {"id": "conn_1", "organization_id": "org_1", "provider": "slack", "label": "Acme",
			"status": "connected", "auth_method": "oauth", "sync_direction": "push", "health": "healthy",
			"granted_scopes": ["chat:write"], "created_at": "2026-10-01T10:00:00Z", "updated_at": "2026-10-01T10:00:00Z"},
		"missing_scopes": ["channels:history"],
		"settings": {"channel": "C0001", "routes": {"inbound_reply": "C0002"}, "assistant_dm_only": true,
			"inbox_channel": "C0003", "inbox_scope": "all"},
		"my_link": `+slackLinkFixture+`,
		"links": [`+slackLinkFixture+`]
	}`, &got)

	st, _, err := c.Integrations.SlackStatus(context.Background())
	if err != nil {
		t.Fatalf("SlackStatus: %v", err)
	}
	checkRequest(t, got, "GET", "/v1/integrations/slack/status", "", "")
	if !st.AppConfigured || !st.InteractiveConfigured || st.Connection == nil || st.Connection.Provider != ProviderSlack {
		t.Errorf("status = %+v", st)
	}
	if len(st.MissingScopes) != 1 || st.Settings.Routes[NotifInboundReply] != "C0002" || st.Settings.InboxScope != SlackInboxAll || !st.Settings.AssistantDMOnly {
		t.Errorf("settings = %+v, missing %v", st.Settings, st.MissingScopes)
	}
	if st.MyLink == nil || !st.MyLink.DMNotifications || st.MyLink.UserEmail != "sam@acme.test" || len(st.Links) != 1 {
		t.Errorf("links = %+v %+v", st.MyLink, st.Links)
	}
}

// A member without integrations access sees readiness only.
func TestSlackStatusOwnLinkOnly(t *testing.T) {
	c := agentToolServer(t, http.StatusOK, `{"app_configured": true, "interactive_configured": false,
		"missing_scopes": [], "settings": {}, "links": []}`, nil)
	st, _, err := c.Integrations.SlackStatus(context.Background())
	if err != nil {
		t.Fatalf("SlackStatus: %v", err)
	}
	if st.Connection != nil || st.MyLink != nil || st.Settings.Channel != "" || st.Links == nil {
		t.Errorf("status = %+v", st)
	}
}

func TestSlackChannels(t *testing.T) {
	var got recordedRequest
	c := agentToolServer(t, http.StatusOK, `{"data": [
		{"id": "C0001", "name": "sales", "is_private": false, "is_member": true},
		{"id": "G0002", "name": "sales-leads", "is_private": true, "is_member": false}
	], "pagination": {"next_cursor": null, "has_more": true}}`, &got)

	page, err := c.Integrations.SlackChannels(context.Background(), &SlackChannelParams{Query: "#sales"})
	if err != nil {
		t.Fatalf("SlackChannels: %v", err)
	}
	checkRequest(t, got, "GET", "/v1/integrations/slack/channels", "q=%23sales", "")
	if len(page.Data) != 2 || !page.Data[0].IsMember || !page.Data[1].IsPrivate || !page.Pagination.HasMore || page.HasMore() || page.NextCursor() != "" {
		t.Errorf("page = %+v", page)
	}

	if _, err := c.Integrations.SlackChannels(context.Background(), nil); err != nil {
		t.Fatalf("SlackChannels(nil): %v", err)
	}
	checkRequest(t, got, "GET", "/v1/integrations/slack/channels", "", "")
}

func TestSlackChannelsNotConnected(t *testing.T) {
	c := agentToolServer(t, http.StatusNotFound,
		`{"error": "not_found", "code": "slack_not_connected", "message": "This workspace has not connected Slack."}`, nil)
	_, err := c.Integrations.SlackChannels(context.Background(), nil)
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "slack_not_connected" {
		t.Fatalf("err = %v", err)
	}
}

func TestSlackUpdateSettings(t *testing.T) {
	var got recordedRequest
	c := agentToolServer(t, http.StatusOK, `{"channel": "C0001", "routes": {"health_bounce": "C0009"}, "inbox_scope": "replies"}`, &got)

	saved, _, err := c.Integrations.UpdateSlackSettings(context.Background(), &SlackSettings{
		Channel:    "C0001",
		Routes:     map[string]string{NotifHealthBounce: "C0009"},
		InboxScope: SlackInboxReplies,
	})
	if err != nil {
		t.Fatalf("UpdateSlackSettings: %v", err)
	}
	checkRequest(t, got, "PUT", "/v1/integrations/slack/settings", "",
		`{"channel":"C0001","routes":{"health_bounce":"C0009"},"inbox_scope":"replies"}`)
	if saved.Channel != "C0001" || saved.Routes[NotifHealthBounce] != "C0009" || saved.InboxChannel != "" {
		t.Errorf("saved = %+v", saved)
	}
}

func TestSlackLinkFlow(t *testing.T) {
	ctx := context.Background()
	var got recordedRequest

	c := agentToolServer(t, http.StatusOK, `{
		"organization_id": "org_1", "organization_name": "Acme", "is_member": true,
		"user_email": "sam@acme.test", "verify_available": true,
		"slack_team_id": "T0001", "slack_team_name": "Acme", "slack_user_id": "U0001",
		"slack_user_name": "", "slack_user_avatar": "",
		"email_matches": false, "expires_at": "2026-11-15T10:15:00Z"
	}`, &got)
	p, _, err := c.Integrations.PreviewSlackLink(ctx, "abc_DEF-123")
	if err != nil {
		t.Fatalf("PreviewSlackLink: %v", err)
	}
	checkRequest(t, got, "GET", "/v1/integrations/slack/link/abc_DEF-123", "", "")
	if p.OrganizationName != "Acme" || !p.IsMember || p.EmailMatches || !p.VerifyAvailable || p.SlackTeamID != "T0001" || p.ExpiresAt.IsZero() {
		t.Errorf("preview = %+v", p)
	}

	c = agentToolServer(t, http.StatusOK, `{"url": "https://slack.com/openid/connect/authorize?state=s1"}`, &got)
	u, _, err := c.Integrations.StartSlackLinkVerify(ctx, "abc_DEF-123")
	if err != nil {
		t.Fatalf("StartSlackLinkVerify: %v", err)
	}
	checkRequest(t, got, "POST", "/v1/integrations/slack/link/verify", "", `{"code":"abc_DEF-123"}`)
	if u != "https://slack.com/openid/connect/authorize?state=s1" {
		t.Errorf("url = %q", u)
	}

	c = agentToolServer(t, http.StatusCreated, slackLinkFixture, &got)
	link, resp, err := c.Integrations.ConfirmSlackLink(ctx, &SlackLinkConfirmParams{Code: "abc_DEF-123", SlackCode: "sc_1", State: "s1"})
	if err != nil {
		t.Fatalf("ConfirmSlackLink: %v", err)
	}
	checkRequest(t, got, "POST", "/v1/integrations/slack/link", "", `{"code":"abc_DEF-123","slack_code":"sc_1","state":"s1"}`)
	if link.ID != "lnk_1" || link.SlackUserID != "U0001" || resp.StatusCode != http.StatusCreated {
		t.Errorf("link = %+v", link)
	}

	if _, _, err := c.Integrations.ConfirmSlackLink(ctx, &SlackLinkConfirmParams{Code: "abc_DEF-123"}); err != nil {
		t.Fatalf("ConfirmSlackLink: %v", err)
	}
	checkRequest(t, got, "POST", "/v1/integrations/slack/link", "", `{"code":"abc_DEF-123"}`)
}

func TestSlackLinkMismatch(t *testing.T) {
	c := agentToolServer(t, http.StatusForbidden, `{"error": "forbidden", "code": "slack_link_email_mismatch", "message": "mismatch"}`, nil)
	_, _, err := c.Integrations.ConfirmSlackLink(context.Background(), &SlackLinkConfirmParams{Code: "abc"})
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "slack_link_email_mismatch" {
		t.Fatalf("err = %v", err)
	}
}

func TestSlackMemberLinks(t *testing.T) {
	ctx := context.Background()
	var got recordedRequest

	c := agentToolServer(t, http.StatusOK, slackLinkFixture, &got)
	link, _, err := c.Integrations.SetSlackDMNotifications(ctx, false)
	if err != nil {
		t.Fatalf("SetSlackDMNotifications: %v", err)
	}
	checkRequest(t, got, "PATCH", "/v1/integrations/slack/link", "", `{"dm_notifications":false}`)
	if link.ID != "lnk_1" {
		t.Errorf("link = %+v", link)
	}

	c = agentToolServer(t, http.StatusNoContent, "", &got)
	if _, err := c.Integrations.UnlinkSlack(ctx); err != nil {
		t.Fatalf("UnlinkSlack: %v", err)
	}
	checkRequest(t, got, "DELETE", "/v1/integrations/slack/link", "", "")

	if _, err := c.Integrations.RemoveSlackLink(ctx, "lnk_1"); err != nil {
		t.Fatalf("RemoveSlackLink: %v", err)
	}
	checkRequest(t, got, "DELETE", "/v1/integrations/slack/links/lnk_1", "", "")
}
