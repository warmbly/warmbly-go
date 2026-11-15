package warmbly

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// Partnership connect tools. The routes under emails/connect and
// emails/oauth-apps belong to the Partnership plan: a workspace on another
// plan is refused with a 403 carrying the code "partnership_plan_required".
// They connect mailboxes by credentials a partner already holds, hand out
// single-use sign-in links, and keep the workspace's own Google and Microsoft
// OAuth clients. Client connect links (emails/client-links) are open to every
// plan and let a client connect mailboxes into the workspace without an
// account.

// MaxMailboxGrantBulkRows is the most sign-ins one
// [EmailService.ConnectMailboxOAuthBulk] call accepts.
const MaxMailboxGrantBulkRows = 50

// MailboxBulkRenewed is the [MailboxBulkRow.Status] of a handed-over sign-in
// that renewed a mailbox the workspace already had. Only
// [EmailService.ConnectMailboxOAuthBulk] reports it.
const MailboxBulkRenewed = "renewed"

// Warmup send failure kinds returned in [WarmupSendFailure.Kind]. An empty
// Kind is an ordinary failure; treat any other value as one too.
const (
	// WarmupFailureKindMailboxLoading: the sending worker has not loaded the
	// mailbox yet. It clears on its own shortly after a connect.
	WarmupFailureKindMailboxLoading = "mailbox_loading"
	// WarmupFailureKindProviderThrottle: the provider is pacing the mailbox,
	// which the scheduler absorbs on its own.
	WarmupFailureKindProviderThrottle = "provider_throttle"
)

// MailboxConnectWarmup starts warmup on a mailbox as it connects. Nil fields
// keep the mailbox defaults; the values must be positive and ReplyRate is 1 to
// 100.
type MailboxConnectWarmup struct {
	Enabled   bool `json:"enabled"`
	Start     *int `json:"start,omitempty"`
	Max       *int `json:"max,omitempty"`
	Increase  *int `json:"increase,omitempty"`
	ReplyRate *int `json:"reply_rate,omitempty"`
}

// ConnectSMTPIMAPParams connects one SMTP/IMAP mailbox through the
// Partnership connect API and optionally starts its warmup.
type ConnectSMTPIMAPParams struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
	// SMTP is used for sending and IMAP for reading. Both are required.
	SMTP   *MailboxCredentials   `json:"smtp"`
	IMAP   *MailboxCredentials   `json:"imap"`
	Warmup *MailboxConnectWarmup `json:"warmup,omitempty"`
}

// ConnectSMTPIMAPBulkParams connects up to [MaxSMTPIMAPBulkRows] SMTP/IMAP
// mailboxes with one warmup setting.
type ConnectSMTPIMAPBulkParams struct {
	Accounts []SMTPIMAPParams      `json:"accounts"`
	Warmup   *MailboxConnectWarmup `json:"warmup,omitempty"`
}

// MailboxOAuthGrant hands Warmbly a sign-in the workspace's own OAuth app
// already completed: either a RefreshToken, or an authorization Code with the
// RedirectURI (and PKCE CodeVerifier) it was issued for.
type MailboxOAuthGrant struct {
	// Provider is [ProviderGmail] or [ProviderOutlook].
	Provider     string `json:"provider"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Code         string `json:"code,omitempty"`
	RedirectURI  string `json:"redirect_uri,omitempty"`
	CodeVerifier string `json:"code_verifier,omitempty"`
	// Email, when set, must be the address the sign-in belongs to.
	Email string `json:"email,omitempty"`
}

// ConnectOAuthParams hands over one sign-in and optionally starts warmup.
type ConnectOAuthParams struct {
	MailboxOAuthGrant
	Warmup *MailboxConnectWarmup `json:"warmup,omitempty"`
}

// ConnectOAuthBulkParams hands over up to [MaxMailboxGrantBulkRows] sign-ins
// with one warmup setting.
type ConnectOAuthBulkParams struct {
	Grants []MailboxOAuthGrant   `json:"grants"`
	Warmup *MailboxConnectWarmup `json:"warmup,omitempty"`
}

// ConnectedMailbox is the answer to a single Partnership connect.
type ConnectedMailbox struct {
	Mailbox Email `json:"mailbox"`
	// Renewed is true when a handed-over sign-in renewed a mailbox the
	// workspace already had. Only [EmailService.ConnectMailboxOAuth] sets it.
	Renewed bool `json:"renewed,omitempty"`
	// WarmupNote explains a requested warmup that could not start, empty when
	// it started or none was asked for.
	WarmupNote string `json:"warmup_note"`
}

// MailboxGrantBulkSummary counts a batch of handed-over sign-ins by outcome.
type MailboxGrantBulkSummary struct {
	Total     int `json:"total"`
	Connected int `json:"connected"`
	Renewed   int `json:"renewed"`
	Failed    int `json:"failed"`
}

// MailboxGrantBulkResult is the per-sign-in answer to
// [EmailService.ConnectMailboxOAuthBulk]. Each row's Status is
// [MailboxBulkConnected], [MailboxBulkRenewed] or [MailboxBulkFailed].
type MailboxGrantBulkResult struct {
	Data    []MailboxBulkRow        `json:"data"`
	Summary MailboxGrantBulkSummary `json:"summary"`
}

// Sign-in link states returned in [MailboxConnectLink.Status].
const (
	MailboxConnectLinkPending    = "pending"
	MailboxConnectLinkConnecting = "connecting"
	MailboxConnectLinkConnected  = "connected"
	MailboxConnectLinkFailed     = "failed"
	MailboxConnectLinkRevoked    = "revoked"
	MailboxConnectLinkExpired    = "expired"
)

// MailboxConnectLink is a single-use sign-in link a mailbox owner opens to
// connect one Gmail or Outlook mailbox through the workspace's OAuth app.
type MailboxConnectLink struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	CreatedBy      string `json:"created_by"`
	// Provider is [ProviderGmail] or [ProviderOutlook].
	Provider string `json:"provider"`
	// Email is the address the sign-in must belong to, empty for any.
	Email     string               `json:"email,omitempty"`
	Warmup    MailboxConnectWarmup `json:"warmup"`
	ReturnURL string               `json:"return_url,omitempty"`
	// Status is one of the MailboxConnectLink* constants; it reads
	// [MailboxConnectLinkExpired] once a pending link is past ExpiresAt.
	Status    string `json:"status"`
	ErrorCode string `json:"error_code,omitempty"`
	// EmailAccountID is the mailbox the link connected.
	EmailAccountID *string    `json:"email_account_id,omitempty"`
	ExpiresAt      time.Time  `json:"expires_at"`
	CreatedAt      time.Time  `json:"created_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	// URL is returned once, by [EmailService.CreateConnectLink], and never
	// again.
	URL string `json:"url,omitempty"`
}

// MailboxConnectLinkParams asks for a sign-in link.
type MailboxConnectLinkParams struct {
	// Provider is [ProviderGmail] or [ProviderOutlook].
	Provider string `json:"provider"`
	// Email, when set, is the only address the link will connect.
	Email  string                `json:"email,omitempty"`
	Warmup *MailboxConnectWarmup `json:"warmup,omitempty"`
	// ReturnURL is an https address the owner is sent to afterwards.
	ReturnURL string `json:"return_url,omitempty"`
	// ExpiresInHours is 1 to 168; zero takes the default of 72.
	ExpiresInHours int `json:"expires_in_hours,omitempty"`
}

// MailboxOAuthApp is a workspace's own OAuth client for one mailbox provider.
// The client secret is never returned.
type MailboxOAuthApp struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	// Provider is [ProviderGmail] or [ProviderOutlook].
	Provider string `json:"provider"`
	ClientID string `json:"client_id"`
	// HasSecret is false after an import that carried no secrets; save the
	// secret again before the app can sign mailboxes in.
	HasSecret bool   `json:"has_secret"`
	AppName   string `json:"app_name"`
	// RedirectURL is the callback the client must register; CustomRedirect
	// says the workspace chose it.
	RedirectURL    string    `json:"redirect_url"`
	CustomRedirect bool      `json:"custom_redirect"`
	CreatedBy      *string   `json:"created_by,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	// Mailboxes counts mailboxes whose grant this client issued.
	Mailboxes int `json:"mailboxes"`
}

// MailboxOAuthApps is the workspace's own OAuth clients together with the
// default callback URL to register for each provider.
type MailboxOAuthApps struct {
	Data []MailboxOAuthApp `json:"data"`
	// Callbacks maps a provider ([ProviderGmail], [ProviderOutlook]) to the
	// redirect URL this deployment expects by default.
	Callbacks map[string]string `json:"callbacks"`
}

// MailboxOAuthAppParams saves a workspace's OAuth client.
type MailboxOAuthAppParams struct {
	ClientID string `json:"client_id"`
	// ClientSecret is required for a new app; empty keeps the stored one.
	ClientSecret string `json:"client_secret,omitempty"`
	AppName      string `json:"app_name,omitempty"`
	// RedirectURL overrides the default callback; empty uses the default.
	RedirectURL string `json:"redirect_url,omitempty"`
}

// Client connect link states returned in [ClientConnectLink.Status].
const (
	ClientConnectLinkActive    = "active"
	ClientConnectLinkExpired   = "expired"
	ClientConnectLinkRevoked   = "revoked"
	ClientConnectLinkExhausted = "exhausted"
)

// ConnectLinkProviderVendor is the [ClientConnectLinkParams.Providers] value
// that lets a client connect an inbox vendor account and pick its mailboxes.
// The other values are [ProviderGmail], [ProviderOutlook] and
// [ProviderSMTPIMAP].
const ConnectLinkProviderVendor = "vendor"

// ClientConnectLink is a link a client opens to connect mailboxes into the
// workspace without an account.
type ClientConnectLink struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	// CreatedBy is nil once the member who made the link has left; such a
	// link reads revoked.
	CreatedBy          *string    `json:"created_by"`
	Label              string     `json:"label"`
	AllowedProviders   []string   `json:"allowed_providers"`
	MaxMailboxes       int        `json:"max_mailboxes"`
	MailboxesConnected int        `json:"mailboxes_connected"`
	TagIDs             []string   `json:"tag_ids"`
	Warmup             bool       `json:"warmup"`
	ExpiresAt          time.Time  `json:"expires_at"`
	RevokedAt          *time.Time `json:"revoked_at"`
	CreatedAt          time.Time  `json:"created_at"`
	LastUsedAt         *time.Time `json:"last_used_at"`
	// Status is one of the ClientConnectLink* constants.
	Status string `json:"status"`
}

// ClientConnectLinkParams is what a member chooses when creating a client
// connect link.
type ClientConnectLinkParams struct {
	Label string `json:"label"`
	// Providers is at least one of [ProviderGmail], [ProviderOutlook],
	// [ProviderSMTPIMAP] and [ConnectLinkProviderVendor].
	Providers []string `json:"providers"`
	// MaxMailboxes is 1 to 500.
	MaxMailboxes int `json:"max_mailboxes"`
	// ExpiresInDays is 1 to 90; zero takes the default of 7.
	ExpiresInDays int `json:"expires_in_days,omitempty"`
	// TagIDs are applied to every mailbox the link connects, at most 20.
	TagIDs []string `json:"tag_ids,omitempty"`
	// Warmup starts warmup on every mailbox the link connects.
	Warmup bool `json:"warmup,omitempty"`
}

// CreatedClientConnectLink carries a new link and its URL, which is shown
// once and never again.
type CreatedClientConnectLink struct {
	Link ClientConnectLink `json:"link"`
	URL  string            `json:"url"`
}

// ClientConnectLinkUse is one mailbox a client connected through a link.
type ClientConnectLinkUse struct {
	ID     string `json:"id"`
	LinkID string `json:"link_id"`
	// EmailAccountID is nil once the mailbox was removed.
	EmailAccountID *string   `json:"email_account_id"`
	Email          string    `json:"email"`
	Provider       string    `json:"provider"`
	ConnectedAt    time.Time `json:"connected_at"`
}

// WarmupFilterTag is the code appended to the warmup mail a mailbox receives,
// so its owner can filter that mail. Tag is nil when the tag is off.
type WarmupFilterTag struct {
	Tag *string `json:"tag"`
}

// ConnectMailboxSMTPIMAP connects one SMTP/IMAP mailbox through the
// Partnership connect API, validating both legs live, and starts its warmup
// when params.Warmup asks for it. It answers 201; read
// [ConnectedMailbox.WarmupNote] for a warmup that could not start. Requires
// the CONNECT_EMAILS scope, the manage-emails permission and a selected
// organization on the Partnership plan.
func (s *EmailService) ConnectMailboxSMTPIMAP(ctx context.Context, params *ConnectSMTPIMAPParams, opts ...RequestOption) (*ConnectedMailbox, *Response, error) {
	return send[ConnectedMailbox](ctx, s.client.post, "emails/connect/smtp-imap", params, opts)
}

// ConnectMailboxSMTPIMAPBulk connects up to [MaxSMTPIMAPBulkRows] SMTP/IMAP
// mailboxes with one warmup setting. Like
// [EmailService.ConnectSMTPIMAPBulk] it always answers 200 with a status per
// row. Requires the CONNECT_EMAILS scope and the Partnership plan.
func (s *EmailService) ConnectMailboxSMTPIMAPBulk(ctx context.Context, params *ConnectSMTPIMAPBulkParams, opts ...RequestOption) (*MailboxBulkResult, *Response, error) {
	return send[MailboxBulkResult](ctx, s.client.post, "emails/connect/smtp-imap/bulk", params, opts)
}

// ConnectMailboxOAuth hands over one sign-in the workspace's own OAuth app
// completed. It answers 201 for a new mailbox and 200 with
// [ConnectedMailbox.Renewed] set when it renewed one the workspace already
// had; check the Response status code or Renewed to tell them apart. Requires
// the CONNECT_EMAILS scope and the Partnership plan.
func (s *EmailService) ConnectMailboxOAuth(ctx context.Context, params *ConnectOAuthParams, opts ...RequestOption) (*ConnectedMailbox, *Response, error) {
	return send[ConnectedMailbox](ctx, s.client.post, "emails/connect/oauth", params, opts)
}

// ConnectMailboxOAuthBulk hands over up to [MaxMailboxGrantBulkRows]
// sign-ins with one warmup setting. It always answers 200 with a status per
// sign-in, so read [MailboxGrantBulkResult.Summary] and each row. Requires the
// CONNECT_EMAILS scope and the Partnership plan.
func (s *EmailService) ConnectMailboxOAuthBulk(ctx context.Context, params *ConnectOAuthBulkParams, opts ...RequestOption) (*MailboxGrantBulkResult, *Response, error) {
	return send[MailboxGrantBulkResult](ctx, s.client.post, "emails/connect/oauth/bulk", params, opts)
}

// CreateConnectLink mints a single-use sign-in link for one Gmail or Outlook
// mailbox. [MailboxConnectLink.URL] is in this response only. Requires the
// CONNECT_EMAILS scope and the Partnership plan.
func (s *EmailService) CreateConnectLink(ctx context.Context, params *MailboxConnectLinkParams, opts ...RequestOption) (*MailboxConnectLink, *Response, error) {
	return send[MailboxConnectLink](ctx, s.client.post, "emails/connect/links", params, opts)
}

// ListConnectLinks returns a page of the workspace's sign-in links, newest
// first. Limit is 1 to 100 (default 25). Requires the CONNECT_EMAILS scope
// and the Partnership plan.
func (s *EmailService) ListConnectLinks(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[MailboxConnectLink], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[MailboxConnectLink](ctx, s.client, "emails/connect/links", q, opts...)
}

// GetConnectLink returns one sign-in link, without its URL. Requires the
// CONNECT_EMAILS scope and the Partnership plan.
func (s *EmailService) GetConnectLink(ctx context.Context, id string, opts ...RequestOption) (*MailboxConnectLink, *Response, error) {
	return fetch[MailboxConnectLink](ctx, s.client, "emails/connect/links/"+url.PathEscape(id), opts)
}

// RevokeConnectLink ends a sign-in link and returns it; a link that already
// connected its mailbox is returned as it is. Requires the CONNECT_EMAILS
// scope and the Partnership plan.
func (s *EmailService) RevokeConnectLink(ctx context.Context, id string, opts ...RequestOption) (*MailboxConnectLink, *Response, error) {
	out := new(MailboxConnectLink)
	resp, err := s.client.call(ctx, http.MethodDelete, "emails/connect/links/"+url.PathEscape(id), nil, out, opts)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// ListMailboxOAuthApps returns the workspace's own Google and Microsoft OAuth
// clients, secrets never included, and the default callback URL for each
// provider. Requires the CONNECT_EMAILS scope and a selected organization.
func (s *EmailService) ListMailboxOAuthApps(ctx context.Context, opts ...RequestOption) (*MailboxOAuthApps, *Response, error) {
	out, resp, err := fetch[MailboxOAuthApps](ctx, s.client, "emails/oauth-apps", opts)
	if out != nil && out.Data == nil {
		out.Data = []MailboxOAuthApp{}
	}
	return out, resp, err
}

// PutMailboxOAuthApp saves the workspace's OAuth client for provider
// ([ProviderGmail] or [ProviderOutlook]); the provider checks the client ID
// and secret before anything is stored. An empty ClientSecret keeps the
// stored one. Requires the CONNECT_EMAILS scope and the Partnership plan (or
// an app already saved for that provider).
func (s *EmailService) PutMailboxOAuthApp(ctx context.Context, provider string, params *MailboxOAuthAppParams, opts ...RequestOption) (*MailboxOAuthApp, *Response, error) {
	return send[MailboxOAuthApp](ctx, s.client.put, "emails/oauth-apps/"+url.PathEscape(provider), params, opts)
}

// DeleteMailboxOAuthApp removes the workspace's OAuth client for provider.
// It is refused with a 409 while mailboxes hold grants it issued. Requires
// the CONNECT_EMAILS scope.
func (s *EmailService) DeleteMailboxOAuthApp(ctx context.Context, provider string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "emails/oauth-apps/"+url.PathEscape(provider), opts...)
}

// CreateClientLink creates a client connect link. The URL in the answer is
// shown once and never again; pass [WithIdempotencyKey] so a retry replays the
// same link instead of minting a second one. Requires the WRITE_EMAILS scope
// and the manage-emails permission.
func (s *EmailService) CreateClientLink(ctx context.Context, params *ClientConnectLinkParams, opts ...RequestOption) (*CreatedClientConnectLink, *Response, error) {
	return send[CreatedClientConnectLink](ctx, s.client.post, "emails/client-links", params, opts)
}

// ListClientLinks returns a page of the workspace's client connect links,
// newest first. Limit is 1 to 100 (default 50). Requires the READ_EMAILS scope
// and the manage-emails permission.
func (s *EmailService) ListClientLinks(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[ClientConnectLink], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[ClientConnectLink](ctx, s.client, "emails/client-links", q, opts...)
}

// ClientLinkUses lists the mailboxes a client connect link connected.
// Requires the READ_EMAILS scope and the manage-emails permission.
func (s *EmailService) ClientLinkUses(ctx context.Context, id string, opts ...RequestOption) ([]ClientConnectLinkUse, *Response, error) {
	return fetchData[ClientConnectLinkUse](ctx, s.client, "emails/client-links/"+url.PathEscape(id)+"/uses", opts)
}

// RevokeClientLink turns a client connect link off for good; mailboxes it
// already connected stay connected. A repeat returns the already revoked
// link, so a retry is safe. Requires the WRITE_EMAILS scope and the
// manage-emails permission.
func (s *EmailService) RevokeClientLink(ctx context.Context, id string, opts ...RequestOption) (*ClientConnectLink, *Response, error) {
	return send[ClientConnectLink](ctx, s.client.post, "emails/client-links/"+url.PathEscape(id)+"/revoke", nil, opts)
}

// WarmupFilterTag returns the code appended to the warmup mail the mailbox
// receives; [WarmupFilterTag.Tag] is nil when the tag is off. Requires the
// READ_EMAILS scope and the manage-emails permission.
func (s *EmailService) WarmupFilterTag(ctx context.Context, id string, opts ...RequestOption) (*WarmupFilterTag, *Response, error) {
	return fetch[WarmupFilterTag](ctx, s.client, "emails/"+url.PathEscape(id)+"/warmup-filter-tag", opts)
}

// SetWarmupFilterTag turns the warmup filter tag on. An empty tag issues a
// new random code in the form xxxx-xxxx-xxxx (pass [WithIdempotencyKey] so a
// retry returns the first one); otherwise tag is 6 to 40 letters, digits,
// spaces, hyphens or underscores and must not describe the mail as warmup,
// automated or a test (refused with a 400 carrying "invalid_filter_tag").
// Requires the WRITE_EMAILS scope and the manage-emails permission.
func (s *EmailService) SetWarmupFilterTag(ctx context.Context, id, tag string, opts ...RequestOption) (*WarmupFilterTag, *Response, error) {
	var body any
	if tag != "" {
		body = struct {
			Tag string `json:"tag"`
		}{Tag: tag}
	}
	return send[WarmupFilterTag](ctx, s.client.put, "emails/"+url.PathEscape(id)+"/warmup-filter-tag", body, opts)
}

// ClearWarmupFilterTag turns the warmup filter tag off, so warmup mail carries
// no visible marker again. Requires the WRITE_EMAILS scope and the
// manage-emails permission.
func (s *EmailService) ClearWarmupFilterTag(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "emails/"+url.PathEscape(id)+"/warmup-filter-tag", opts...)
}
