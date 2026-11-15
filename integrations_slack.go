package warmbly

import (
	"context"
	"net/url"
	"time"
)

// The Slack panel routes below back the dashboard's Slack page: status, the
// channel picker, settings and member links. They are session-only (an API
// key is refused). The Slack app's own callbacks (events, interactivity) are
// Slack-to-Warmbly traffic signed by Slack and are not part of this client.

// Slack inbox scopes for [SlackSettings.InboxScope].
const (
	// SlackInboxReplies mirrors human replies only. It is the default.
	SlackInboxReplies = "replies"
	// SlackInboxAll mirrors every inbound message.
	SlackInboxAll = "all"
)

// SlackSettings is the non-secret Slack configuration of a workspace. Channel
// values are Slack channel ids (C... or G...); a "#name" still posts for
// public channels, except for InboxChannel, which must be an id picked from
// [IntegrationService.SlackChannels].
type SlackSettings struct {
	// Channel is the default channel for notifications.
	Channel string `json:"channel,omitempty"`
	// Routes sends a notification category (one of the Notif* constants,
	// such as [NotifInboundReply]) to its own channel. A category absent here
	// uses Channel; an unknown category is refused.
	Routes map[string]string `json:"routes,omitempty"`
	// AssistantDisabled turns the assistant off for the whole Slack workspace.
	AssistantDisabled bool `json:"assistant_disabled,omitempty"`
	// AssistantDMOnly keeps the assistant out of channels: it answers in DMs
	// and the assistant pane only.
	AssistantDMOnly bool `json:"assistant_dm_only,omitempty"`
	// InboxChannel mirrors unified-inbox arrivals into this channel, one
	// thread per conversation. Empty turns the mirror off. The bot must be
	// in the channel, and a channel shared through Slack Connect is refused.
	InboxChannel string `json:"inbox_channel,omitempty"`
	// InboxScope is [SlackInboxReplies] or [SlackInboxAll].
	InboxScope string `json:"inbox_scope,omitempty"`
}

// SlackUserLink binds a Slack member to a Warmbly member. Anything the Slack
// assistant does for that member runs as them, with their current
// permissions.
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
// integrations also see the connection, settings and missing scopes, and
// members who manage settings also see every link.
type SlackStatus struct {
	// AppConfigured reports that this deployment has Slack OAuth credentials.
	AppConfigured bool `json:"app_configured"`
	// InteractiveConfigured reports that events, the assistant, slash
	// commands and buttons work as well.
	InteractiveConfigured bool `json:"interactive_configured"`
	// Connection is the workspace's Slack connection, nil when not connected
	// or not visible to the caller.
	Connection *IntegrationConnection `json:"connection,omitempty"`
	// MissingScopes lists bot scopes the install predates; reconnecting
	// grants them.
	MissingScopes []string      `json:"missing_scopes"`
	Settings      SlackSettings `json:"settings"`
	// MyLink is the caller's own link, nil when they have none.
	MyLink *SlackUserLink `json:"my_link,omitempty"`
	// Links is every member's link, empty unless the caller manages settings.
	Links []SlackUserLink `json:"links"`
}

// SlackChannel is one entry of the channel picker.
type SlackChannel struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsPrivate bool   `json:"is_private"`
	// IsMember reports whether the bot is in the channel.
	IsMember bool `json:"is_member"`
}

// SlackLinkPreview is what a member sees before confirming a link to a Slack
// account.
type SlackLinkPreview struct {
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name"`
	// IsMember reports whether the caller belongs to that organization;
	// confirming requires it.
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
	// Warmbly email. When it is false, confirming needs a Sign in with Slack
	// result ([IntegrationService.StartSlackLinkVerify]).
	EmailMatches bool      `json:"email_matches"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// SlackLinkConfirmParams confirms a link code minted by the Slack bot.
type SlackLinkConfirmParams struct {
	// Code is the link code from the Slack bot. Required.
	Code string `json:"code"`
	// SlackCode and State are the Sign in with Slack result, from the
	// redirect after [IntegrationService.StartSlackLinkVerify]. Send them
	// when the emails do not match.
	SlackCode string `json:"slack_code,omitempty"`
	State     string `json:"state,omitempty"`
}

// SlackChannelParams filters the channel picker.
type SlackChannelParams struct {
	// Query narrows channels by name; a leading "#" is ignored.
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

// SlackStatus returns the Slack panel state for the current workspace.
// Session-only; any member may read it.
func (s *IntegrationService) SlackStatus(ctx context.Context, opts ...RequestOption) (*SlackStatus, *Response, error) {
	return fetch[SlackStatus](ctx, s.client, "integrations/slack/status", opts)
}

// SlackChannels returns the Slack channels the bot can see, for a channel
// picker. The server returns one page of at most 200: Pagination.HasMore says
// more matched than were returned, and there is no cursor, so narrow the list
// with [SlackChannelParams.Query]. A workspace without Slack fails with 404
// "slack_not_connected". Session-only; needs the use-integrations or
// manage-settings organization permission.
func (s *IntegrationService) SlackChannels(ctx context.Context, params *SlackChannelParams, opts ...RequestOption) (*Page[SlackChannel], error) {
	return listJSON[SlackChannel](ctx, s.client, "integrations/slack/channels", params.values(), opts...)
}

// UpdateSlackSettings replaces the workspace's Slack settings and returns
// what was saved. It is a PUT of the whole document, so send every field you
// want kept. Session-only; needs the manage-settings organization
// permission.
func (s *IntegrationService) UpdateSlackSettings(ctx context.Context, settings *SlackSettings, opts ...RequestOption) (*SlackSettings, *Response, error) {
	return send[SlackSettings](ctx, s.client.put, "integrations/slack/settings", settings, opts)
}

// PreviewSlackLink shows what a link code from the Slack bot would connect,
// before the member confirms it. It needs no current workspace. An expired or
// used code fails with 404 "slack_link_invalid". Session-only.
func (s *IntegrationService) PreviewSlackLink(ctx context.Context, code string, opts ...RequestOption) (*SlackLinkPreview, *Response, error) {
	return fetch[SlackLinkPreview](ctx, s.client, "integrations/slack/link/"+url.PathEscape(code), opts)
}

// ConfirmSlackLink links the signed-in member to the Slack account that
// minted the code, in the workspace the code names. When the emails differ
// it fails with 403 "slack_link_email_mismatch" unless the params carry a
// Sign in with Slack result. Session-only; needs no current workspace.
func (s *IntegrationService) ConfirmSlackLink(ctx context.Context, params *SlackLinkConfirmParams, opts ...RequestOption) (*SlackUserLink, *Response, error) {
	return send[SlackUserLink](ctx, s.client.post, "integrations/slack/link", params, opts)
}

// StartSlackLinkVerify begins Sign in with Slack for a link code and returns
// the URL to send the member to in a browser. Slack redirects back with the
// code and state to pass in [SlackLinkConfirmParams]. Session-only; needs no
// current workspace.
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

// SetSlackDMNotifications switches the signed-in member's Slack
// direct-message notifications on or off and returns their link. A member
// with no link fails with 404 "slack_not_linked". Session-only.
func (s *IntegrationService) SetSlackDMNotifications(ctx context.Context, enabled bool, opts ...RequestOption) (*SlackUserLink, *Response, error) {
	body := struct {
		DMNotifications bool `json:"dm_notifications"`
	}{DMNotifications: enabled}
	return send[SlackUserLink](ctx, s.client.patch, "integrations/slack/link", body, opts)
}

// UnlinkSlack removes the signed-in member's own Slack link in the current
// workspace. Removing a link that does not exist also succeeds. Answers
// 204. Session-only.
func (s *IntegrationService) UnlinkSlack(ctx context.Context, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "integrations/slack/link", opts...)
}

// RemoveSlackLink removes any member's Slack link by its id
// ([SlackUserLink.ID]). Answers 204. Session-only; needs the manage-settings
// organization permission.
func (s *IntegrationService) RemoveSlackLink(ctx context.Context, linkID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "integrations/slack/links/"+url.PathEscape(linkID), opts...)
}
