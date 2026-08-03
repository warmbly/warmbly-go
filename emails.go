package warmbly

import (
	"context"
	"net/url"
	"time"
)

// EmailService manages connected email accounts (mailboxes), their warmup
// configuration, their sending-domain authentication and the one-off messages
// sent through them.
type EmailService service

// Mailbox provider values returned in [Email.Provider].
const (
	ProviderGmail    = "gmail"
	ProviderOutlook  = "outlook"
	ProviderSMTPIMAP = "smtp_imap"
)

// Mailbox connection states returned in [Email.Status].
const (
	MailboxStatusActive   = "active"
	MailboxStatusInactive = "inactive"
	MailboxStatusRevoked  = "revoked"
)

// Email is a connected email account (mailbox) as returned by the API.
type Email struct {
	ID             string  `json:"id"`
	UserID         string  `json:"user_id"`
	OrganizationID *string `json:"organization_id,omitempty"`
	// WorkerID identifies the worker currently handling this mailbox, when assigned.
	WorkerID *string `json:"worker_id"`
	Email    string  `json:"email"`

	Name           string `json:"name"`
	SignaturePlain string `json:"signature_plain"`
	SignatureHTML  string `json:"signature_html"`
	// SignatureSync mirrors the signature configured at the provider instead of
	// the one stored here.
	SignatureSync bool `json:"signature_sync"`
	// SignatureCode reports whether the HTML signature is edited as raw markup.
	SignatureCode bool `json:"signature_code"`

	// Provider is the mailbox backend: [ProviderGmail], [ProviderOutlook] or
	// [ProviderSMTPIMAP].
	Provider string `json:"provider"`
	// Status is the connection state: [MailboxStatusActive],
	// [MailboxStatusInactive] or [MailboxStatusRevoked].
	Status string `json:"status"`

	// LastSyncedAt is when the mailbox was last polled for inbound mail.
	LastSyncedAt time.Time `json:"last_synced_at"`
	// LastID is the provider-side sync watermark.
	LastID *int64 `json:"last_id"`

	// CampaignLimit caps how many campaign messages a day this mailbox sends.
	CampaignLimit int `json:"campaign_limit"`
	// MinWaitTime is the minimum gap, in minutes, between two sends.
	MinWaitTime int    `json:"min_wait_time"`
	ReplyTo     string `json:"reply_to"`

	TrackingDomain           string     `json:"tracking_domain"`
	TrackingDomainVerified   bool       `json:"tracking_domain_verified"`
	TrackingDomainVerifiedAt *time.Time `json:"tracking_domain_verified_at"`

	// AuthState summarizes SPF/DKIM/DMARC for the sending domain, as refreshed
	// by the background sweep: "passing", "failing" or "unknown". It is
	// observational — a failing domain is not blocked from sending.
	AuthState       string     `json:"auth_state"`
	AuthSPF         bool       `json:"auth_spf"`
	AuthDKIM        bool       `json:"auth_dkim"`
	AuthDMARC       bool       `json:"auth_dmarc"`
	AuthDMARCPolicy string     `json:"auth_dmarc_policy,omitempty"`
	AuthReason      string     `json:"auth_reason,omitempty"`
	AuthCheckedAt   *time.Time `json:"auth_checked_at,omitempty"`

	// Warmup is the warmup ramp anchor; a nil value means warmup is disabled.
	Warmup *time.Time `json:"warmup"`
	// WarmupPausedAt is when warmup was paused, when it is currently paused.
	WarmupPausedAt  *time.Time `json:"warmup_paused_at"`
	WarmupBase      int        `json:"warmup_base"`
	WarmupMax       int        `json:"warmup_max"`
	WarmupIncrease  int        `json:"warmup_increase"`
	WarmupReplyRate int        `json:"warmup_reply_rate"`
	WarmupTag       string     `json:"warmup_tag"`
	// WarmupPoolType is the partner pool this mailbox warms in: "free" or
	// "premium".
	WarmupPoolType string `json:"warmup_pool_type"`
	// WarmupStartTime and WarmupEndTime bound the daily warmup window as
	// "HH:MM" in the mailbox timezone.
	WarmupStartTime string `json:"warmup_start_time"`
	WarmupEndTime   string `json:"warmup_end_time"`
	// WarmupDays is a bitmask of the weekdays warmup runs on.
	WarmupDays int `json:"warmup_days"`

	Timezone string `json:"timezone"`

	// Tags holds the ids of the tags applied to this mailbox.
	Tags []string `json:"tags"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WarmupEnabled reports whether warmup is enabled for this mailbox, whether or
// not it is currently paused.
func (e *Email) WarmupEnabled() bool { return e.Warmup != nil }

// WarmupActive reports whether warmup is enabled and running.
func (e *Email) WarmupActive() bool { return e.Warmup != nil && e.WarmupPausedAt == nil }

// WarmupPaused reports whether warmup is enabled but paused. A paused mailbox
// keeps its ramp progress.
func (e *Email) WarmupPaused() bool { return e.Warmup != nil && e.WarmupPausedAt != nil }

// WarmupBanStatus describes whether a mailbox has been blocked from the warmup
// pool and whether that block can be appealed.
type WarmupBanStatus struct {
	EmailAccountID string `json:"email_account_id"`
	Blocked        bool   `json:"blocked"`
	// HealthState is the mailbox's warmup health: "healthy", "watch",
	// "throttled", "quarantined" or "blocked".
	HealthState   string     `json:"health_state"`
	Reason        string     `json:"reason,omitempty"`
	BlockedAt     *time.Time `json:"blocked_at,omitempty"`
	BlockedUntil  *time.Time `json:"blocked_until,omitempty"`
	CanAppeal     bool       `json:"can_appeal"`
	PendingAppeal bool       `json:"pending_appeal"`
}

// Send modes accepted by [SendEmailParams.SendMode].
const (
	// SendModeInstant enqueues the message immediately.
	SendModeInstant = "instant"
	// SendModeSmart places the message in the next gap in the mailbox's
	// sending schedule.
	SendModeSmart = "smart"
	// SendModeScheduled sends at [SendEmailParams.ScheduledAt].
	SendModeScheduled = "scheduled"
)

// SendEmailParams are the parameters for sending a one-off message from a mailbox.
type SendEmailParams struct {
	To        []string `json:"to"`
	CC        []string `json:"cc,omitempty"`
	BCC       []string `json:"bcc,omitempty"`
	Subject   string   `json:"subject"`
	BodyHTML  string   `json:"body_html,omitempty"`
	BodyPlain string   `json:"body_plain,omitempty"`
	// InReplyTo holds the RFC 5322 Message-IDs this message replies to.
	InReplyTo []string `json:"in_reply_to,omitempty"`
	// ThreadID continues an existing provider thread.
	ThreadID string `json:"thread_id,omitempty"`
	// SendMode is [SendModeInstant] (the default), [SendModeSmart] or
	// [SendModeScheduled].
	SendMode string `json:"send_mode,omitempty"`
	// ScheduledAt is the send time, required when SendMode is
	// [SendModeScheduled] and ignored otherwise.
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
}

// SendResult is returned when a message has been accepted for delivery.
type SendResult struct {
	// TaskID identifies the queued send.
	TaskID string `json:"task_id"`
	// ScheduledAt is when the message will actually go out, which for
	// [SendModeSmart] is chosen by the scheduler.
	ScheduledAt time.Time `json:"scheduled_at"`
	SendMode    string    `json:"send_mode"`
}

// WarmupAppealParams are the parameters for appealing a warmup ban.
type WarmupAppealParams struct {
	Reason string `json:"reason"`
}

// WarmupAppealResult identifies the appeal that was filed.
type WarmupAppealResult struct {
	AppealID string `json:"appeal_id"`
}

// EmailUpdateParams are the parameters for updating a mailbox. Unset (nil)
// fields are left unchanged.
type EmailUpdateParams struct {
	Name *string `json:"name,omitempty"`

	SignaturePlain *string `json:"signature_plain,omitempty"`
	SignatureHTML  *string `json:"signature_html,omitempty"`
	SignatureSync  *bool   `json:"signature_sync,omitempty"`
	SignatureCode  *bool   `json:"signature_code,omitempty"`

	// Status is [MailboxStatusActive], [MailboxStatusInactive] or
	// [MailboxStatusRevoked].
	Status *string `json:"status,omitempty"`

	CampaignLimit *int    `json:"campaign_limit,omitempty"`
	MinWaitTime   *int    `json:"min_wait_time,omitempty"`
	ReplyTo       *string `json:"reply_to,omitempty"`

	// Warmup enables or disables warmup. Prefer the explicit lifecycle methods
	// ([EmailService.StartWarmup] and friends) unless you are changing warmup
	// settings in the same call.
	Warmup          *bool   `json:"warmup,omitempty"`
	WarmupBase      *int    `json:"warmup_base,omitempty"`
	WarmupMax       *int    `json:"warmup_max,omitempty"`
	WarmupIncrease  *int    `json:"warmup_increase,omitempty"`
	WarmupReplyRate *int    `json:"warmup_reply_rate,omitempty"`
	WarmupTag       *string `json:"warmup_tag,omitempty"`
	WarmupStartTime *string `json:"warmup_start_time,omitempty"`
	WarmupEndTime   *string `json:"warmup_end_time,omitempty"`
	WarmupDays      *int    `json:"warmup_days,omitempty"`

	// Tags replaces the mailbox's tag ids wholesale when non-nil.
	Tags []string `json:"tags,omitempty"`
}

// EmailListParams filters and paginates a list of connected mailboxes.
type EmailListParams struct {
	ListOptions
	// Query is a free-text search over the mailbox address and name.
	Query string
	// Tag filters to mailboxes carrying the given tag id.
	Tag string
}

func (p *EmailListParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	setNonEmpty(q, "q", p.Query)
	setNonEmpty(q, "tag", p.Tag)
	return q
}

// BulkTagParams adds and removes tags across many mailboxes at once. Mailbox
// and tag ids the caller cannot see are ignored rather than failing the batch,
// and the operation is set-based, so retries are naturally safe.
type BulkTagParams struct {
	EmailIDs   []string `json:"email_ids"`
	AddTags    []string `json:"add_tags,omitempty"`
	RemoveTags []string `json:"remove_tags,omitempty"`
}

// BulkTagResult reports how many mailboxes the tag change touched.
type BulkTagResult struct {
	Updated int `json:"updated"`
}

// TrackingDomainStatus is the state of a mailbox's custom click/open tracking
// domain. Verified turns true once the customer's subdomain CNAMEs to the
// shared tracking host.
type TrackingDomainStatus struct {
	TrackingDomain           string     `json:"tracking_domain"`
	TrackingDomainVerified   bool       `json:"tracking_domain_verified"`
	TrackingDomainVerifiedAt *time.Time `json:"tracking_domain_verified_at"`
}

// DomainAuthCheck is the live SPF/DKIM/DMARC lookup for a sending domain.
type DomainAuthCheck struct {
	Domain        string   `json:"domain"`
	SPFFound      bool     `json:"spf_found"`
	SPFRecord     string   `json:"spf_record,omitempty"`
	DKIMFound     bool     `json:"dkim_found"`
	DKIMSelectors []string `json:"dkim_selectors,omitempty"`
	DMARCFound    bool     `json:"dmarc_found"`
	DMARCPolicy   string   `json:"dmarc_policy,omitempty"`
	AllAligned    bool     `json:"all_aligned"`
	// LookupError is true when an authoritative lookup failed transiently
	// (timeout, SERVFAIL, network) rather than the record being absent. Treat
	// the result as unknown, not as a misconfiguration.
	LookupError bool   `json:"lookup_error"`
	Summary     string `json:"summary"`
}

// Address verification outcomes returned in [VerifyResult.Status].
const (
	VerifyStatusValid   = "valid"
	VerifyStatusRisky   = "risky"
	VerifyStatusInvalid = "invalid"
	VerifyStatusUnknown = "unknown"
)

// VerifyResult is the outcome of verifying a single recipient address.
type VerifyResult struct {
	Email string `json:"email"`
	// Status is [VerifyStatusValid], [VerifyStatusRisky],
	// [VerifyStatusInvalid] or [VerifyStatusUnknown].
	Status     string    `json:"status"`
	Reason     string    `json:"reason"`
	IsCatchAll bool      `json:"is_catch_all"`
	HasMX      bool      `json:"has_mx"`
	CheckedAt  time.Time `json:"checked_at"`
}

// MailboxCredentials are the host, port and login for one leg of an SMTP/IMAP
// connection.
type MailboxCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
}

// SMTPIMAPParams connects a mailbox by its own SMTP and IMAP credentials, for
// providers with no OAuth integration. The passwords are sealed server-side on
// receipt and never returned.
type SMTPIMAPParams struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
	// SMTP is used for sending and IMAP for reading. Both are required.
	SMTP *MailboxCredentials `json:"smtp"`
	IMAP *MailboxCredentials `json:"imap"`
}

// List returns a page of connected mailboxes, newest first.
func (s *EmailService) List(ctx context.Context, params *EmailListParams, opts ...RequestOption) (*Page[Email], error) {
	return listJSON[Email](ctx, s.client, "emails", params.values(), opts...)
}

// Get retrieves a single mailbox by ID.
func (s *EmailService) Get(ctx context.Context, id string, opts ...RequestOption) (*Email, *Response, error) {
	return fetch[Email](ctx, s.client, "emails/"+url.PathEscape(id), opts)
}

// Update modifies a mailbox's settings.
func (s *EmailService) Update(ctx context.Context, id string, params *EmailUpdateParams, opts ...RequestOption) (*Email, *Response, error) {
	return send[Email](ctx, s.client, s.client.patch, "emails/"+url.PathEscape(id), params, opts)
}

// Delete disconnects and removes a mailbox.
func (s *EmailService) Delete(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "emails/"+url.PathEscape(id), opts...)
}

// BulkTag adds and removes tags across many mailboxes in one call.
func (s *EmailService) BulkTag(ctx context.Context, params *BulkTagParams, opts ...RequestOption) (*BulkTagResult, *Response, error) {
	return send[BulkTagResult](ctx, s.client, s.client.patch, "emails/tags", params, opts)
}

// UpdateTrackingDomain sets the mailbox's custom tracking subdomain (for
// example "t.acme.com") and re-resolves it. An empty domain clears it.
func (s *EmailService) UpdateTrackingDomain(ctx context.Context, id, domain string, opts ...RequestOption) (*TrackingDomainStatus, *Response, error) {
	q := url.Values{"domain": {domain}}
	return send[TrackingDomainStatus](ctx, s.client, s.client.patch, withQuery("emails/"+url.PathEscape(id)+"/track", q), nil, opts)
}

// AuthCheck runs a live SPF/DKIM/DMARC lookup against the mailbox's sending
// domain.
func (s *EmailService) AuthCheck(ctx context.Context, id string, opts ...RequestOption) (*DomainAuthCheck, *Response, error) {
	return fetch[DomainAuthCheck](ctx, s.client, "emails/"+url.PathEscape(id)+"/auth-check", opts)
}

// Verify checks whether a recipient address is deliverable, before anything is
// ever sent to it.
func (s *EmailService) Verify(ctx context.Context, email string, opts ...RequestOption) (*VerifyResult, *Response, error) {
	body := struct {
		Email string `json:"email"`
	}{Email: email}
	return send[VerifyResult](ctx, s.client, s.client.post, "emails/verify", body, opts)
}

// Send sends a one-off message from the given mailbox.
func (s *EmailService) Send(ctx context.Context, id string, params *SendEmailParams, opts ...RequestOption) (*SendResult, *Response, error) {
	return send[SendResult](ctx, s.client, s.client.post, "emails/"+url.PathEscape(id)+"/send", params, opts)
}

// StartWarmup enables warmup for a mailbox, resuming from the existing ramp
// progress when it was previously paused.
func (s *EmailService) StartWarmup(ctx context.Context, id string, opts ...RequestOption) (*Email, *Response, error) {
	return s.warmupLifecycle(ctx, id, "start", opts)
}

// PauseWarmup pauses warmup without losing ramp progress.
func (s *EmailService) PauseWarmup(ctx context.Context, id string, opts ...RequestOption) (*Email, *Response, error) {
	return s.warmupLifecycle(ctx, id, "pause", opts)
}

// ResumeWarmup resumes a paused warmup, continuing at the same daily volume.
func (s *EmailService) ResumeWarmup(ctx context.Context, id string, opts ...RequestOption) (*Email, *Response, error) {
	return s.warmupLifecycle(ctx, id, "resume", opts)
}

// StopWarmup disables warmup and clears ramp progress; a later start begins a
// fresh ramp. Use [EmailService.PauseWarmup] to keep the progress.
func (s *EmailService) StopWarmup(ctx context.Context, id string, opts ...RequestOption) (*Email, *Response, error) {
	return s.warmupLifecycle(ctx, id, "stop", opts)
}

func (s *EmailService) warmupLifecycle(ctx context.Context, id, action string, opts []RequestOption) (*Email, *Response, error) {
	return send[Email](ctx, s.client, s.client.post, "emails/"+url.PathEscape(id)+"/warmup/"+action, nil, opts)
}

// WarmupBanStatus reports whether a mailbox has been blocked from the warmup pool.
func (s *EmailService) WarmupBanStatus(ctx context.Context, id string, opts ...RequestOption) (*WarmupBanStatus, *Response, error) {
	return fetch[WarmupBanStatus](ctx, s.client, "emails/"+url.PathEscape(id)+"/warmup/ban-status", opts)
}

// AppealWarmupBan submits an appeal against a mailbox's warmup ban.
func (s *EmailService) AppealWarmupBan(ctx context.Context, id string, params *WarmupAppealParams, opts ...RequestOption) (*WarmupAppealResult, *Response, error) {
	return send[WarmupAppealResult](ctx, s.client, s.client.post, "emails/"+url.PathEscape(id)+"/warmup/appeal", params, opts)
}

// --- connecting a mailbox ---
//
// These routes are session-only. Connecting a mailbox writes a refresh token
// encrypted to the connecting user, which is not something a long-lived API key
// should be able to trigger.

// StartOAuth begins connecting a Gmail or Outlook mailbox and returns the URL
// to send the user to. Provider is [ProviderGmail] or [ProviderOutlook].
//
// The provider redirects to a Warmbly-hosted page that posts the code and state
// back to the opener; pass those to [EmailService.FinishOAuth].
func (s *EmailService) StartOAuth(ctx context.Context, provider string, opts ...RequestOption) (*OAuthStartResult, *Response, error) {
	body := struct {
		Provider string `json:"provider"`
	}{Provider: provider}
	return send[OAuthStartResult](ctx, s.client, s.client.post, "emails/onboarding/oauth/start", body, opts)
}

// FinishOAuth exchanges the authorization code for a connected mailbox.
func (s *EmailService) FinishOAuth(ctx context.Context, code, state string, opts ...RequestOption) (*Email, *Response, error) {
	body := struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}{Code: code, State: state}
	return send[Email](ctx, s.client, s.client.post, "emails/onboarding/oauth/finish", body, opts)
}

// ConnectSMTPIMAP connects a mailbox by its own credentials. The server dials
// both legs to validate them before storing anything, so a bad password fails
// here rather than silently at send time.
func (s *EmailService) ConnectSMTPIMAP(ctx context.Context, params *SMTPIMAPParams, opts ...RequestOption) (*Email, *Response, error) {
	return send[Email](ctx, s.client, s.client.post, "emails/onboarding/smtp-imap", params, opts)
}
