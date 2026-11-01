package warmbly

import (
	"context"
	"net/url"
	"time"
)

// The Slack panel routes below are session-only: an API key is refused, and so
// is a request with no organization selected. They back the dashboard's Slack
// page (status, channel picker, settings and member links). The Slack app's own
// callbacks (events, interactivity) are Slack-to-Warmbly traffic and are not
// part of this client.

// Slack inbox scopes for [SlackSettings.InboxScope].
const (
	// SlackInboxReplies mirrors human replies only. It is the default.
	SlackInboxReplies = "replies"
	// SlackInboxAll mirrors every inbound message.
	SlackInboxAll = "all"
)

// SlackSettings is the non-secret Slack configuration of a workspace. Channel
// values are Slack channel ids (C... or G...); a "#name" still posts for public
// channels.
type SlackSettings struct {
	// Channel is the default channel for notifications.
	Channel string `json:"channel,omitempty"`
	// Routes sends a notification category (for example "inbound_reply") to
	// its own channel. A category absent here uses Channel.
	Routes map[string]string `json:"routes,omitempty"`
	// AssistantDisabled turns the assistant off for the whole Slack workspace.
	AssistantDisabled bool `json:"assistant_disabled,omitempty"`
	// AssistantDMOnly keeps the assistant out of channels: it answers in DMs
	// and the assistant pane only.
	AssistantDMOnly bool `json:"assistant_dm_only,omitempty"`
	// InboxChannel mirrors unified-inbox arrivals into this channel, one
	// thread per conversation. Empty turns the mirror off.
	InboxChannel string `json:"inbox_channel,omitempty"`
	// InboxScope is [SlackInboxReplies] or [SlackInboxAll].
	InboxScope string `json:"inbox_scope,omitempty"`
}

// SlackUserLink binds a Slack member to a Warmbly member. Anything the Slack
// assistant does for that member runs as them, with their current permissions.
type SlackUserLink struct {
	ID              string    `json:"id"`
	OrganizationID  string    `json:"organization_id"`
	ConnectionID    string    `json:"connection_id"`
	SlackTeamID     string    `json:"slack_team_id"`
	SlackUserID     string    `json:"slack_user_id"`
	UserID          string    `json:"user_id"`
	DMNotifications bool      `json:"dm_notifications"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	// UserName and UserEmail describe the Warmbly member, joined in for
	// display.
	UserName  string `json:"user_name,omitempty"`
	UserEmail string `json:"user_email,omitempty"`
}

// SlackStatus is the Slack panel's state. What it fills in depends on the
// caller: everyone sees readiness and their own link, members who use
// integrations also see the connection and settings, and members who manage
// settings also see every link.
type SlackStatus struct {
	// AppConfigured reports that this deployment has Slack OAuth credentials.
	AppConfigured bool `json:"app_configured"`
	// InteractiveConfigured reports that events, the assistant, slash commands
	// and buttons work as well.
	InteractiveConfigured bool `json:"interactive_configured"`
	// Connection is the workspace's Slack connection, nil when not connected.
	Connection *IntegrationConnection `json:"connection,omitempty"`
	// MissingScopes lists bot scopes the install predates; reconnecting grants
	// them.
	MissingScopes []string      `json:"missing_scopes"`
	Settings      SlackSettings `json:"settings"`
	// MyLink is the caller's own link, nil when they have none.
	MyLink *SlackUserLink  `json:"my_link,omitempty"`
	Links  []SlackUserLink `json:"links"`
}

// SlackChannel is one entry of the channel picker.
type SlackChannel struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsPrivate bool   `json:"is_private"`
	IsMember  bool   `json:"is_member"`
}

// SlackLinkPreview is what a member sees before confirming a link to a Slack
// account.
type SlackLinkPreview struct {
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name"`
	// IsMember reports whether the caller belongs to that organization.
	IsMember bool `json:"is_member"`
	// UserEmail is the signed-in Warmbly account the link would be made for.
	UserEmail string `json:"user_email"`
	// VerifyAvailable reports that Sign in with Slack can confirm the link
	// when the two emails differ.
	VerifyAvailable bool   `json:"verify_available"`
	SlackTeamID     string `json:"slack_team_id"`
	SlackTeamName   string `json:"slack_team_name"`
	SlackUserID     string `json:"slack_user_id"`
	// SlackUserName and SlackUserAvatar describe the Slack account; either
	// may be empty.
	SlackUserName   string `json:"slack_user_name"`
	SlackUserAvatar string `json:"slack_user_avatar"`
	// EmailMatches reports that the Slack account's email is the caller's
	// Warmbly email, which confirming requires unless verified.
	EmailMatches bool      `json:"email_matches"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// SlackLinkConfirmParams confirms a link code minted by the Slack bot.
type SlackLinkConfirmParams struct {
	// Code is the link code from the Slack bot.
	Code string `json:"code"`
	// SlackCode and State are the result of Sign in with Slack, from
	// [IntegrationService.StartSlackLinkVerify]. Send them when the emails do
	// not match.
	SlackCode string `json:"slack_code,omitempty"`
	State     string `json:"state,omitempty"`
}

// SlackChannelParams filters the channel picker.
type SlackChannelParams struct {
	// Query narrows channels by name.
	Query string
}

func (p *SlackChannelParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	setNonEmpty(q, "q", p.Query)
	return q
}

// SlackStatus returns the Slack panel state for the current organization.
// Session-only.
func (s *IntegrationService) SlackStatus(ctx context.Context, opts ...RequestOption) (*SlackStatus, *Response, error) {
	return fetch[SlackStatus](ctx, s.client, "integrations/slack/status", opts)
}

// SlackChannels returns the Slack channels the workspace can post to, for a
// channel picker. The server returns one capped page: Pagination.HasMore says
// more channels exist than were returned, and there is no cursor, so narrow
// the list with [SlackChannelParams.Query]. Session-only, and the member needs
// the integrations or settings permission.
func (s *IntegrationService) SlackChannels(ctx context.Context, params *SlackChannelParams, opts ...RequestOption) (*Page[SlackChannel], error) {
	return listJSON[SlackChannel](ctx, s.client, "integrations/slack/channels", params.values(), opts...)
}

// UpdateSlackSettings replaces the workspace's Slack settings and returns what
// was saved. It is a PUT of the whole document, so omit nothing you want kept.
// Session-only, and the member needs the settings permission.
func (s *IntegrationService) UpdateSlackSettings(ctx context.Context, settings *SlackSettings, opts ...RequestOption) (*SlackSettings, *Response, error) {
	return send[SlackSettings](ctx, s.client.put, "integrations/slack/settings", settings, opts)
}

// PreviewSlackLink shows what a link code would connect, before the member
// confirms it. The code needs no organization; confirming requires membership
// of the one it names. Session-only.
func (s *IntegrationService) PreviewSlackLink(ctx context.Context, code string, opts ...RequestOption) (*SlackLinkPreview, *Response, error) {
	return fetch[SlackLinkPreview](ctx, s.client, "integrations/slack/link/"+url.PathEscape(code), opts)
}

// ConfirmSlackLink links the signed-in member to the Slack account that
// minted the code. Session-only.
func (s *IntegrationService) ConfirmSlackLink(ctx context.Context, params *SlackLinkConfirmParams, opts ...RequestOption) (*SlackUserLink, *Response, error) {
	return send[SlackUserLink](ctx, s.client.post, "integrations/slack/link", params, opts)
}

// StartSlackLinkVerify begins Sign in with Slack for a link code and returns
// the URL to send the member to. Session-only.
func (s *IntegrationService) StartSlackLinkVerify(ctx context.Context, code string, opts ...RequestOption) (string, *Response, error) {
	body := struct {
		Code string `json:"code"`
	}{Code: code}
	var out struct {
		URL string `json:"url"`
	}
	resp, err := s.client.post(ctx, "integrations/slack/link/verify", body, &out, opts...)
	if err != nil {
		return "", resp, err
	}
	return out.URL, resp, nil
}

// SetSlackDMNotifications switches the signed-in member's Slack direct-message
// notifications on or off and returns their link. Session-only.
func (s *IntegrationService) SetSlackDMNotifications(ctx context.Context, enabled bool, opts ...RequestOption) (*SlackUserLink, *Response, error) {
	body := struct {
		DMNotifications bool `json:"dm_notifications"`
	}{DMNotifications: enabled}
	return send[SlackUserLink](ctx, s.client.patch, "integrations/slack/link", body, opts)
}

// UnlinkSlack removes the signed-in member's own Slack link. Session-only.
func (s *IntegrationService) UnlinkSlack(ctx context.Context, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "integrations/slack/link", opts...)
}

// RemoveSlackLink removes any member's Slack link by its id. Session-only, and
// the member needs the settings permission.
func (s *IntegrationService) RemoveSlackLink(ctx context.Context, linkID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "integrations/slack/links/"+url.PathEscape(linkID), opts...)
}
