package warmbly

import (
	"context"
	"net/url"
	"time"
)

// CloudLinkService is a self-hosted instance's side of the warmup pool link,
// the "Settings > Warmbly Cloud" page: connect the instance to a Warmbly
// Cloud workspace, choose which local mailboxes warm in the hosted pool, and
// add Google or Microsoft mailboxes through Warmbly's own OAuth apps.
//
// Only warmup moves to the cloud. Campaigns, contacts and the inbox stay on
// the instance. The link itself is a property of the instance, not of a
// workspace, so an instance holds at most one.
//
// Connecting is a device-code handshake driven from here:
// [CloudLinkService.Connect] asks the cloud for a code and returns what to
// show the operator, who approves it on Warmbly Cloud; meanwhile
// [CloudLinkService.WaitForConnect] polls until the instance has its token.
//
// Every route is session-only (a token from [AuthService.Login], never an API
// key) and needs an active workspace on the session. Reads are open to any
// member, since no secret travels; connecting, polling and disconnecting are
// a settings change and need the manage-settings permission; enrolling,
// pausing, resuming, adopting and the OAuth sign-in are mailbox changes and
// need the manage-emails permission.
//
// On a deployment without the cloud link (the hosted product itself, or an
// instance built without it) every route answers 501 Not Implemented.
type CloudLinkService service

// CloudLinkConnection is the instance's link to a Warmbly Cloud workspace.
// The instance token is never serialized.
type CloudLinkConnection struct {
	// CloudURL is the Warmbly Cloud API the instance is linked to.
	CloudURL string `json:"cloud_url"`
	// InstanceID is the instance's ID on the cloud, as
	// [PoolLinkInstance.ID] on the workspace's side.
	InstanceID string `json:"instance_id"`
	// OrganizationName is the cloud workspace's name at link time.
	OrganizationName string `json:"organization_name"`
	// ConnectedBy is the local member who ran the handshake.
	ConnectedBy *string   `json:"connected_by,omitempty"`
	ConnectedAt time.Time `json:"connected_at"`
	// LastSyncedAt is the last time the instance reached the cloud, and
	// LastError the failure from the most recent attempt, if it failed.
	LastSyncedAt *time.Time `json:"last_synced_at,omitempty"`
	LastError    string     `json:"last_error,omitempty"`
}

// CloudLinkStatus is the dashboard's view of the link.
type CloudLinkStatus struct {
	// Connected reports whether the instance holds a link at all. Link is
	// filled from the local row whenever it does.
	Connected bool                 `json:"connected"`
	Link      *CloudLinkConnection `json:"link,omitempty"`
	// Info is the cloud's own status document for this instance, including
	// the pool allowance; it is set only when Reachable.
	Info *PoolLinkInstanceInfo `json:"info,omitempty"`
	// Reachable is false when the cloud could not be contacted just now;
	// Error then carries the reason and Link still comes from the local row.
	Reachable bool   `json:"reachable"`
	Error     string `json:"error,omitempty"`
	// DefaultCloudURL is the cloud [CloudLinkService.Connect] proposes when
	// given no URL: WARMBLY_CLOUD_URL or the hosted API.
	DefaultCloudURL string `json:"default_cloud_url"`
}

// CloudLinkPendingConnect is an in-flight handshake: what to show the operator
// and how to poll. It lives in the instance's memory only, so a restart
// forgets it.
type CloudLinkPendingConnect struct {
	// UserCode is the short code the operator types in on Warmbly Cloud.
	UserCode string `json:"user_code"`
	// VerificationURL is the cloud page that approves the code; it already
	// carries the user code as a query parameter.
	VerificationURL string `json:"verification_url"`
	// CloudURL is the cloud the handshake was opened against.
	CloudURL string `json:"cloud_url"`
	// ExpiresAt is when an unapproved code is retired.
	ExpiresAt time.Time `json:"expires_at"`
	// Interval is the minimum number of seconds between polls.
	Interval int `json:"interval"`
}

// CloudLinkConnectPollResult is one answer to [CloudLinkService.PollConnect].
type CloudLinkConnectPollResult struct {
	// Status is [PoolLinkCodePending] or [PoolLinkCodeApproved]; a denied,
	// expired or forgotten handshake is reported as an error.
	Status string `json:"status"`
	// Link is the stored connection, set once approved.
	Link *CloudLinkConnection `json:"link,omitempty"`
	// Info is the cloud's status document, set once approved when the cloud
	// answered the follow-up call.
	Info *PoolLinkInstanceInfo `json:"info,omitempty"`
}

// CloudLinkMailbox is one of the instance's active mailboxes with its pool
// enrollment, as listed on the Warmbly Cloud settings page.
type CloudLinkMailbox struct {
	// ID is the mailbox's ID on this instance, as [Email.ID].
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	// Provider is [ProviderGmail], [ProviderOutlook] or [ProviderSMTPIMAP].
	Provider string `json:"provider"`
	// Status is the local connection state, for example [MailboxStatusActive].
	Status string `json:"status"`
	// Enrolled reports whether the mailbox warms in the hosted pool; the
	// instance's own warmup stands down for it while it does.
	Enrolled   bool       `json:"enrolled"`
	EnrolledAt *time.Time `json:"enrolled_at,omitempty"`
	// Managed is true for a mailbox signed in through Warmbly Cloud: the
	// credential lives on the cloud and the instance sends with brokered
	// tokens.
	Managed bool `json:"managed"`
	// Cloud is the cloud's view of an enrolled mailbox (health, today's
	// count, 7-day totals). Nil when not enrolled or when the cloud could not
	// be reached for the listing.
	Cloud *PoolLinkMailboxState `json:"cloud,omitempty"`
}

// CloudLinkOAuthStart is the handle on a Google or Microsoft consent brokered
// by the cloud: open URL for the operator, then redeem Session with
// [CloudLinkService.FinishOAuth] once the window returns.
type CloudLinkOAuthStart struct {
	URL string `json:"url"`
	// Session is single-use and expires after about 15 minutes.
	Session string `json:"session"`
}

// Status reports whether the instance is linked, the cloud's view of it when
// reachable, and the default cloud URL. Open to any member.
func (s *CloudLinkService) Status(ctx context.Context, opts ...RequestOption) (*CloudLinkStatus, *Response, error) {
	return fetch[CloudLinkStatus](ctx, s.client, "cloud-link", opts)
}

// Connect opens the handshake against cloudURL ("" for the instance's
// default, see [CloudLinkStatus.DefaultCloudURL]) and returns the code to
// show the operator. Follow up with [CloudLinkService.WaitForConnect].
//
// cloudURL must be https (loopback http is allowed for local development).
// An instance that is already linked fails with "cloud_link_connected":
// disconnect first. Needs the manage-settings permission.
func (s *CloudLinkService) Connect(ctx context.Context, cloudURL string, opts ...RequestOption) (*CloudLinkPendingConnect, *Response, error) {
	body := struct {
		CloudURL string `json:"cloud_url,omitempty"`
	}{CloudURL: cloudURL}
	return send[CloudLinkPendingConnect](ctx, s.client.post, "cloud-link/connect", body, opts)
}

// PollConnect asks the cloud once whether the pending code has been
// approved, and on approval stores the instance token and returns the new
// link. Needs the manage-settings permission.
//
// With no handshake in progress it fails with "cloud_link_no_pending" (or,
// if another session finished the handshake meanwhile, answers approved with
// the stored link); an expired code with "cloud_link_code_expired"; a code
// the member declined with "pool_link_denied". Wait at least
// [CloudLinkPendingConnect.Interval] seconds between calls.
func (s *CloudLinkService) PollConnect(ctx context.Context, opts ...RequestOption) (*CloudLinkConnectPollResult, *Response, error) {
	return send[CloudLinkConnectPollResult](ctx, s.client.post, "cloud-link/connect/poll", nil, opts)
}

// WaitForConnect polls every [CloudLinkPendingConnect.Interval] seconds
// (never faster than once a second) until the operator approves the code,
// the instance reports a terminal error, or ctx is done. Once approved the
// link is stored on the instance and the result carries it.
//
// The cloud retires an unapproved code at [CloudLinkPendingConnect.ExpiresAt],
// after which the loop ends with a "cloud_link_code_expired" error; bound ctx
// yourself to give up sooner.
func (s *CloudLinkService) WaitForConnect(ctx context.Context, pending *CloudLinkPendingConnect, opts ...RequestOption) (*CloudLinkConnectPollResult, error) {
	interval := time.Duration(pending.Interval) * time.Second
	for {
		res, _, err := s.PollConnect(ctx, opts...)
		if err != nil {
			return nil, err
		}
		if res.Status == PoolLinkCodeApproved {
			return res, nil
		}
		if err := poolLinkWait(ctx, interval); err != nil {
			return nil, err
		}
	}
}

// Disconnect ends the link. Every enrolled mailbox leaves the pool, their
// credentials are deleted on the cloud and local warmup takes over; mailboxes
// signed in through Warmbly Cloud are removed from the instance altogether,
// since they cannot send without the link (they stay in the cloud
// workspace). Needs the manage-settings permission; recorded in the audit
// log.
//
// The cloud must confirm (or already have dropped) the link before local
// state goes, so a cloud outage fails the call rather than leaving managed
// mailboxes orphaned there; retry later.
func (s *CloudLinkService) Disconnect(ctx context.Context, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "cloud-link", opts...)
}

// --- enrollment of the instance's own mailboxes ---

// ListMailboxes lists every active mailbox in the session's workspace with
// its pool enrollment and, for enrolled ones, the cloud's view. Open to any
// member. The listing makes one round trip to the cloud; when that fails the
// rows still come back, with [CloudLinkMailbox.Cloud] nil.
func (s *CloudLinkService) ListMailboxes(ctx context.Context, opts ...RequestOption) ([]CloudLinkMailbox, *Response, error) {
	return fetchData[CloudLinkMailbox](ctx, s.client, "cloud-link/mailboxes", opts)
}

// EnrollMailbox sends mailbox id's SMTP/IMAP credential to the cloud and
// starts warming it in the hosted pool with the ramp settings it has on the
// instance; local warmup stands down for it. Idempotent for an enrolled
// mailbox. Needs the manage-emails permission; recorded in the audit log.
//
// Only active SMTP/IMAP mailboxes qualify: one signed in with the instance's
// own Google or Microsoft OAuth app fails with "cloud_link_oauth_mailbox"
// (re-add it through [CloudLinkService.StartOAuth] instead), an inactive one
// with "cloud_link_mailbox_inactive", and one over the workspace's allowance
// with "pool_link_mailbox_limit".
func (s *CloudLinkService) EnrollMailbox(ctx context.Context, id string, opts ...RequestOption) (*CloudLinkMailbox, *Response, error) {
	return send[CloudLinkMailbox](ctx, s.client.post, "cloud-link/mailboxes/"+url.PathEscape(id)+"/enroll", nil, opts)
}

// UnenrollMailbox takes mailbox id out of the pool. An SMTP/IMAP mailbox's
// credential is deleted on the cloud and local warmup takes over; a mailbox
// signed in through Warmbly Cloud is removed from the instance instead and
// stays in the cloud workspace. Succeeds for a mailbox that is not enrolled.
// Needs the manage-emails permission; recorded in the audit log.
func (s *CloudLinkService) UnenrollMailbox(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "cloud-link/mailboxes/"+url.PathEscape(id)+"/enroll", opts...)
}

// PauseMailbox pauses pool warmup for enrolled mailbox id without
// unenrolling it. Needs the manage-emails permission; recorded in the audit
// log. A mailbox that is not enrolled fails with not found.
func (s *CloudLinkService) PauseMailbox(ctx context.Context, id string, opts ...RequestOption) (*CloudLinkMailbox, *Response, error) {
	return s.lifecycle(ctx, id, "pause", opts)
}

// ResumeMailbox resumes pool warmup for a paused mailbox. Needs the
// manage-emails permission; recorded in the audit log.
func (s *CloudLinkService) ResumeMailbox(ctx context.Context, id string, opts ...RequestOption) (*CloudLinkMailbox, *Response, error) {
	return s.lifecycle(ctx, id, "resume", opts)
}

func (s *CloudLinkService) lifecycle(ctx context.Context, id, action string, opts []RequestOption) (*CloudLinkMailbox, *Response, error) {
	return send[CloudLinkMailbox](ctx, s.client.post, "cloud-link/mailboxes/"+url.PathEscape(id)+"/"+action, nil, opts)
}

// --- mailboxes signed in through Warmbly Cloud ---

// StartOAuth begins a Google or Microsoft sign-in on Warmbly's own OAuth
// apps, so the instance needs no client of its own. provider is
// [ProviderGmail] or [ProviderOutlook]. Open the returned URL for the
// operator; the consent window returns to the instance's dashboard, which
// then calls [CloudLinkService.FinishOAuth] with the session. Needs the
// manage-emails permission and a live link.
func (s *CloudLinkService) StartOAuth(ctx context.Context, provider string, opts ...RequestOption) (*CloudLinkOAuthStart, *Response, error) {
	body := struct {
		Provider string `json:"provider"`
	}{Provider: provider}
	return send[CloudLinkOAuthStart](ctx, s.client.post, "cloud-link/oauth/start", body, opts)
}

// FinishOAuth redeems a completed consent. The mailbox is created in the
// cloud workspace, where its sign-in lives, and starts warming in the pool at
// once; the instance gets a credential-free mirror of it, returned here,
// which campaigns and the inbox use through brokered access tokens. Needs the
// manage-emails permission; recorded in the audit log.
//
// The session must belong to the session's workspace and is single-use; an
// unknown or expired one fails with "cloud_link_oauth_session", a consent
// the operator has not finished yet with "pool_link_oauth_pending".
func (s *CloudLinkService) FinishOAuth(ctx context.Context, session string, opts ...RequestOption) (*Email, *Response, error) {
	body := struct {
		Session string `json:"session"`
	}{Session: session}
	return send[Email](ctx, s.client.post, "cloud-link/oauth/finish", body, opts)
}

// ListWorkspaceMailboxes lists the Google and Microsoft mailboxes connected
// directly on the linked cloud workspace that this instance may adopt. Open
// to any member; fails with "cloud_link_not_connected" without a link.
func (s *CloudLinkService) ListWorkspaceMailboxes(ctx context.Context, opts ...RequestOption) ([]PoolLinkWorkspaceMailbox, *Response, error) {
	return fetchData[PoolLinkWorkspaceMailbox](ctx, s.client, "cloud-link/workspace-mailboxes", opts)
}

// AdoptWorkspaceMailbox adds cloud workspace mailbox id
// ([PoolLinkWorkspaceMailbox.ID]) to the instance the same way as
// [CloudLinkService.FinishOAuth]: the cloud keeps the sign-in, the instance
// gets a credential-free mirror, returned here. A workspace mailbox can be
// linked to one instance at a time; a second adoption fails with
// "pool_link_already_adopted". Needs the manage-emails permission; recorded
// in the audit log.
func (s *CloudLinkService) AdoptWorkspaceMailbox(ctx context.Context, id string, opts ...RequestOption) (*Email, *Response, error) {
	return send[Email](ctx, s.client.post, "cloud-link/workspace-mailboxes/"+url.PathEscape(id)+"/adopt", nil, opts)
}
