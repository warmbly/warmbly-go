package warmbly

import (
	"context"
	"net/url"
	"time"
)

// EmailService manages connected email accounts (mailboxes), their warmup
// configuration, their sending-domain authentication, their humanlike sending
// behavior and the one-off messages sent through them.
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

// Sending-domain authentication states returned in [Email.AuthState].
const (
	// AuthStateUnknown means the domain has not been checked yet, the DNS
	// lookup could not complete, or the domain is special-use and cannot
	// resolve. It never gates sending.
	AuthStateUnknown = "unknown"
	// AuthStatePassing means SPF and DMARC are both published.
	AuthStatePassing = "passing"
	// AuthStateFailing means SPF or DMARC is missing. A domain that stays
	// failing past the instance grace period (72 hours by default, measured
	// from [Email.AuthFailingSince]) stops cold sending and warmup from every
	// mailbox on it.
	AuthStateFailing = "failing"
)

// Error codes carried in [Error.Code] by mailbox operations.
const (
	// ErrCodeMailboxAllowanceReached is the 403 returned by every connect path
	// once the workspace's mailbox allowance is full. See
	// [EmailService.Allowance].
	ErrCodeMailboxAllowanceReached = "mailbox_allowance_reached"
	// ErrCodeMailboxWorkerUnreachable is the 503 returned by
	// [EmailService.Delete] when the worker syncing the mailbox could not be
	// told to drop it. Nothing was removed; retry in a moment.
	ErrCodeMailboxWorkerUnreachable = "mailbox_worker_unreachable"
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

	// LastSyncedAt is when the mailbox was last polled for inbound mail; nil
	// until the first sync has run. [EmailService.SyncStatus] has the detail.
	LastSyncedAt *time.Time `json:"last_synced_at"`
	// LastID is the provider-side sync watermark.
	LastID *int64 `json:"last_id"`

	// CampaignLimit caps how many campaign messages a day this mailbox sends
	// (0 to 5000; the default is 50). It is a ceiling only: the campaign's
	// daily limit, the warmup ramp, sending behavior and the workspace's daily
	// send limit all still apply and the smallest wins.
	CampaignLimit int `json:"campaign_limit"`
	// MinWaitTime is the minimum gap, in seconds, between two sends (default
	// 600). Jitter is added on top. While sending behavior is enabled the
	// behavior's gap range replaces it.
	MinWaitTime int `json:"min_wait_time"`
	// ReplyTo is the Reply-To address; empty uses the mailbox address.
	ReplyTo string `json:"reply_to"`

	// SaveToSent applies to SMTP/IMAP mailboxes only: after a send the worker
	// files a copy in the mailbox's Sent folder, since SMTP submission puts
	// nothing in the account by itself. Gmail and Outlook file their own copy
	// and ignore the flag. Warmup mail is never filed.
	SaveToSent bool `json:"save_to_sent"`

	// TrackingDomain is the custom open/click tracking subdomain, or "" for
	// the shared host. Only a verified domain is used at send time; see
	// [EmailService.GetTrackingDomain].
	TrackingDomain           string     `json:"tracking_domain"`
	TrackingDomainVerified   bool       `json:"tracking_domain_verified"`
	TrackingDomainVerifiedAt *time.Time `json:"tracking_domain_verified_at"`

	// AuthState summarizes SPF/DKIM/DMARC for the sending domain, as refreshed
	// by the background sweep: [AuthStatePassing], [AuthStateFailing] or
	// [AuthStateUnknown]. A sustained failing state gates cold sending and
	// warmup; see AuthFailingSince.
	AuthState string `json:"auth_state"`
	AuthSPF   bool   `json:"auth_spf"`
	// AuthDKIM is advisory: DKIM selectors are not discoverable from DNS, so a
	// missing DKIM never forces a failing verdict on its own.
	AuthDKIM        bool       `json:"auth_dkim"`
	AuthDMARC       bool       `json:"auth_dmarc"`
	AuthDMARCPolicy string     `json:"auth_dmarc_policy,omitempty"`
	AuthReason      string     `json:"auth_reason,omitempty"`
	AuthCheckedAt   *time.Time `json:"auth_checked_at,omitempty"`
	// AuthFailingSince is when the domain entered [AuthStateFailing], and nil
	// otherwise. The grace period runs from here, so a resolver hiccup or a
	// record broken minutes ago never stops sending immediately. Fix the DNS
	// and call [EmailService.RefreshAuthCheck] to clear it right away.
	AuthFailingSince *time.Time `json:"auth_failing_since,omitempty"`

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

	// Timezone is the mailbox's own IANA zone, which its sending behavior and
	// business-hours window are evaluated in. Empty means not configured, so
	// only the campaign's own window applies.
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
	// "throttled", "quarantined" or "blocked". Health is judged only on what
	// happened while the mailbox was in the pool.
	HealthState string `json:"health_state"`
	// Reason, BlockedAt and BlockedUntil are omitted when the mailbox is not
	// blocked.
	Reason       string     `json:"reason,omitempty"`
	BlockedAt    *time.Time `json:"blocked_at,omitempty"`
	BlockedUntil *time.Time `json:"blocked_until,omitempty"`
	// CanAppeal reports whether [EmailService.AppealWarmupBan] is open to the
	// owner; PendingAppeal reports that one has already been filed.
	CanAppeal     bool `json:"can_appeal"`
	PendingAppeal bool `json:"pending_appeal"`
}

// Send modes accepted by [SendEmailParams.SendMode].
const (
	// SendModeInstant enqueues the message immediately.
	SendModeInstant = "instant"
	// SendModeSmart places the message in the next gap in the mailbox's
	// sending schedule. It follows the mailbox's workday when sending behavior
	// is enabled but is never charged against the cold-send budgets.
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
	// [SendModeScheduled] and ignored otherwise. It must be in the future and
	// within 29 days.
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
}

// SendResult is returned when a message has been accepted for delivery.
type SendResult struct {
	// TaskID identifies the queued send.
	TaskID string `json:"task_id"`
	// ScheduledAt is when the message will actually go out: immediately for
	// [SendModeInstant], the next gap for [SendModeSmart], or the requested
	// time for [SendModeScheduled].
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
	// [MailboxStatusRevoked]. Setting a mailbox inactive stops its syncing and
	// sending within seconds while keeping its settings, history and worker
	// assignment; switching it back on resumes where it stopped.
	Status *string `json:"status,omitempty"`

	// CampaignLimit is the daily cold-campaign cap, 0 to 5000. 30 to 50 a day
	// is the safe band; a fresh mailbox should start at 10 to 20 and ramp.
	CampaignLimit *int `json:"campaign_limit,omitempty"`
	// MinWaitTime is the minimum gap between sends, in seconds.
	MinWaitTime *int    `json:"min_wait_time,omitempty"`
	ReplyTo     *string `json:"reply_to,omitempty"`

	// Timezone is the mailbox's own IANA zone, such as "America/Denver". Send
	// an empty string to clear it, which leaves only the campaign's own window
	// applying.
	Timezone *string `json:"timezone,omitempty"`

	// SaveToSent controls the Sent-folder copy on SMTP/IMAP mailboxes. Turn it
	// off when the submission server already files its own copy (Gmail,
	// Fastmail and Zoho do), or the folder ends up with two of everything.
	// Ignored for OAuth mailboxes.
	SaveToSent *bool `json:"save_to_sent,omitempty"`

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

// EmailListParams filters and paginates a list of connected mailboxes. The
// page size defaults to 50 when Limit is zero.
type EmailListParams struct {
	ListOptions
	// Query is a free-text search over the mailbox address and name.
	Query string
	// Tag filters to mailboxes carrying the given tag id. It must be a UUID;
	// anything else is rejected with a 400.
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

// Machine-readable verdicts returned in [TrackingDomainStatus.Status].
const (
	// TrackingStatusVerified: the subdomain resolves to the tracking host and
	// new sends use it.
	TrackingStatusVerified = "verified"
	// TrackingStatusUnset: no custom domain is configured.
	TrackingStatusUnset = "unset"
	// TrackingStatusNoTarget: this install has no tracking host, so there is
	// nothing to point at and nothing can verify.
	TrackingStatusNoTarget = "no_target"
	// TrackingStatusNotFound: the name does not exist yet. The record has not
	// been added or has not propagated (usually minutes, up to an hour).
	TrackingStatusNotFound = "not_found"
	// TrackingStatusWrongTarget: the record exists but points at another
	// host; [TrackingDomainStatus.Observed] says which.
	TrackingStatusWrongTarget = "wrong_target"
	// TrackingStatusLookupError: DNS could not answer. A transient failure
	// never revokes an already verified domain.
	TrackingStatusLookupError = "lookup_error"
	// TrackingStatusPending: stored state that has not been re-resolved, as
	// reported by the read-only [EmailService.GetTrackingDomain].
	TrackingStatusPending = "pending"
)

// TrackingDomainStatus is the state of a mailbox's custom click/open tracking
// domain. Verified turns true once the customer's subdomain CNAMEs to this
// install's tracking host. Until then opens and clicks go through the shared
// host, so an unverified domain never blocks sending.
//
// Everything below TrackingDomainVerifiedAt is diagnostic, so a "pending"
// badge always comes with something to act on.
type TrackingDomainStatus struct {
	TrackingDomain           string     `json:"tracking_domain"`
	TrackingDomainVerified   bool       `json:"tracking_domain_verified"`
	TrackingDomainVerifiedAt *time.Time `json:"tracking_domain_verified_at"`

	// CNAMETarget is the value to put in the CNAME record: this install's
	// tracking host. Empty when the deployment has no tracking host.
	CNAMETarget string `json:"cname_target"`
	// Status is one of the TrackingStatus* constants.
	Status string `json:"status"`
	// Message explains Status in one sentence and is safe to show as-is.
	Message string `json:"message"`
	// Observed is what DNS actually returned, when it differs from the target.
	Observed string `json:"observed,omitempty"`
	// TrackingHostUnresolvable reports that the record is correct but this
	// install's own tracking host has no DNS record, so nothing will be
	// recorded. That is an operator fault, not a caller one.
	TrackingHostUnresolvable bool `json:"tracking_host_unresolvable"`
}

// DomainAuthCheck is the live SPF/DKIM/DMARC lookup for a sending domain.
type DomainAuthCheck struct {
	Domain    string `json:"domain"`
	SPFFound  bool   `json:"spf_found"`
	SPFRecord string `json:"spf_record,omitempty"`
	// DKIMFound is advisory: selectors are not discoverable from DNS, so a
	// missing DKIM never fails the domain on its own.
	DKIMFound     bool     `json:"dkim_found"`
	DKIMSelectors []string `json:"dkim_selectors,omitempty"`
	DMARCFound    bool     `json:"dmarc_found"`
	// DMARCPolicy is the record's p= value, or its sp= value when the policy
	// is inherited from the organizational domain.
	DMARCPolicy string `json:"dmarc_policy,omitempty"`
	// DMARCDomain is where the DMARC record was actually found. It differs
	// from Domain when the policy is inherited.
	DMARCDomain string `json:"dmarc_domain,omitempty"`
	// DMARCInherited reports that the sending domain has no DMARC record of
	// its own and is covered by its organizational domain's policy, which is
	// how a dedicated sending subdomain normally works. SPF never inherits.
	DMARCInherited bool `json:"dmarc_inherited"`
	// Reserved marks a special-use domain (.test, .invalid, .localhost,
	// .example, .local) that cannot resolve by definition. It is recorded as
	// [AuthStateUnknown] rather than failing.
	Reserved   bool `json:"reserved"`
	AllAligned bool `json:"all_aligned"`
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

// Reason classes returned in [VerifyResult.SubStatus] when the verifier can
// name one.
const (
	VerifySubStatusCatchAll    = "catch_all"
	VerifySubStatusDisposable  = "disposable"
	VerifySubStatusRole        = "role"
	VerifySubStatusSpamTrap    = "spamtrap"
	VerifySubStatusMailboxFull = "mailbox_full"
	VerifySubStatusNoMX        = "no_mx"
	VerifySubStatusSyntax      = "syntax"
	// VerifySubStatusUndisclosed marks a provider (Microsoft, Yahoo) that
	// answers every RCPT the same way, so a probe cannot judge the mailbox.
	VerifySubStatusUndisclosed = "undisclosed"
)

// VerifyResult is the outcome of verifying a single recipient address.
type VerifyResult struct {
	Email string `json:"email"`
	// Status is [VerifyStatusValid], [VerifyStatusRisky],
	// [VerifyStatusInvalid] or [VerifyStatusUnknown]. Unknown covers
	// greylisting, timeouts and blocked outbound port 25; it is never a reason
	// to drop an address.
	Status string `json:"status"`
	// SubStatus refines Status with one of the VerifySubStatus* constants, or
	// "" when nothing more specific is known.
	SubStatus  string `json:"sub_status,omitempty"`
	Reason     string `json:"reason"`
	IsCatchAll bool   `json:"is_catch_all"`
	HasMX      bool   `json:"has_mx"`
	// Provider is who produced the verdict: "builtin" for the in-house probe,
	// or the workspace's connected verification provider.
	Provider string `json:"provider,omitempty"`
	// Confidence is the scored certainty of Status, when the verifier
	// provides one.
	Confidence int       `json:"confidence,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
}

// Connection security modes accepted in [MailboxCredentials.Security]. TLS is
// mandatory either way; the difference is whether it is negotiated before the
// protocol greeting or upgraded in-band after it.
const (
	// MailSecurityTLS is implicit TLS: encrypted from the first byte. The
	// convention for SMTP 465 and IMAP 993.
	MailSecurityTLS = "tls"
	// MailSecurityStartTLS is a plaintext greeting upgraded in place with
	// STARTTLS. The convention for SMTP 587, 25 and 2525, and IMAP 143.
	MailSecurityStartTLS = "starttls"
)

// MailboxCredentials are the host, port and login for one leg of an SMTP/IMAP
// connection.
type MailboxCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Host     string `json:"host"`
	// Port is any port from 1 to 65535.
	Port int `json:"port"`
	// Security is [MailSecurityTLS] or [MailSecurityStartTLS]. Leave it empty
	// to let the port decide (tls for SMTP 465 and IMAP 993, starttls for SMTP
	// 587 and IMAP 143); set it for anything non-standard, such as a
	// submission relay on 2525. A server expecting STARTTLS looks unreachable
	// to a client attempting implicit TLS, and vice versa.
	Security string `json:"security,omitempty"`
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

// SMTPIMAPCredentialsParams carries replacement credentials for an existing
// SMTP/IMAP mailbox. The address and display name never change on a reconnect.
type SMTPIMAPCredentialsParams struct {
	SMTP *MailboxCredentials `json:"smtp"`
	IMAP *MailboxCredentials `json:"imap"`
}

// MaxSMTPIMAPBulkRows is the most rows one [EmailService.ConnectSMTPIMAPBulk]
// call accepts.
const MaxSMTPIMAPBulkRows = 50

// SMTPIMAPBulkParams connects up to [MaxSMTPIMAPBulkRows] SMTP/IMAP mailboxes
// in one call.
type SMTPIMAPBulkParams struct {
	Accounts []SMTPIMAPParams `json:"accounts"`
}

// Per-row outcomes returned in [MailboxBulkRow.Status].
const (
	// MailboxBulkConnected: the credentials were validated and the mailbox
	// connected.
	MailboxBulkConnected = "connected"
	// MailboxBulkSkipped: the mailbox was already connected, so re-sending a
	// batch is safe.
	MailboxBulkSkipped = "skipped"
	// MailboxBulkFailed: the row was refused; [MailboxBulkRow.Code] says why.
	MailboxBulkFailed = "failed"
)

// MailboxBulkRow is one row's answer from a bulk connect.
type MailboxBulkRow struct {
	// Row is the zero-based index of the row in the request, so failed lines
	// can be handed back to whoever supplied them.
	Row   int    `json:"row"`
	Email string `json:"email"`
	// Status is [MailboxBulkConnected], [MailboxBulkSkipped] or
	// [MailboxBulkFailed].
	Status string `json:"status"`
	// Code is the stable error code for a skipped or failed row:
	// "already_connected" for a skip, [ErrCodeMailboxAllowanceReached] for a
	// row past the allowance, otherwise the code the single connect would have
	// returned.
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	// ID is the new mailbox id for a connected row.
	ID *string `json:"id,omitempty"`
}

// MailboxBulkSummary counts a bulk connect's rows by outcome.
type MailboxBulkSummary struct {
	Total     int `json:"total"`
	Connected int `json:"connected"`
	Skipped   int `json:"skipped"`
	Failed    int `json:"failed"`
}

// MailboxBulkResult is the per-row answer to a bulk connect.
type MailboxBulkResult struct {
	Data    []MailboxBulkRow   `json:"data"`
	Summary MailboxBulkSummary `json:"summary"`
	// Allowance is the workspace's mailbox allowance after the batch, so a
	// caller can tell how many more rows will fit without another call.
	Allowance *MailboxAllowance `json:"allowance,omitempty"`
}

// Sources of a workspace's mailbox allowance, returned in
// [MailboxAllowance.Basis].
const (
	// MailboxAllowanceUnlimited: no billing provider, or a plan with no daily
	// send cap. Allowance is nil.
	MailboxAllowanceUnlimited = "unlimited"
	// MailboxAllowanceFree: an unsubscribed workspace's fixed limit.
	MailboxAllowanceFree = "free"
	// MailboxAllowanceOverride: an operator-approved limit-increase request.
	MailboxAllowanceOverride = "override"
	// MailboxAllowancePlan: the plan carries an explicit mailbox limit.
	MailboxAllowancePlan = "plan"
	// MailboxAllowanceFairUse: the plan's daily sends divided by
	// [MailboxAllowance.SendsPerMailbox].
	MailboxAllowanceFairUse = "fair_use"
)

// MailboxAllowance is how many mailboxes a workspace holds, how many it may
// hold, and why. Mailboxes are unlimited on every paid plan in principle; the
// fair-use allowance is one mailbox per daily send the plan includes, which is
// far more than safe sending ever needs. Nothing is ever removed for being
// over the allowance: a workspace that moves to a smaller plan keeps every
// mailbox and simply cannot add more.
type MailboxAllowance struct {
	// Used is the number of mailboxes connected right now.
	Used int `json:"used"`
	// Allowance is the cap; nil means unlimited.
	Allowance *int `json:"allowance"`
	// Remaining is Allowance minus Used, never negative; nil when unlimited.
	Remaining *int `json:"remaining"`
	// Basis is one of the MailboxAllowance* constants.
	Basis string `json:"basis"`
	// SendsPerMailbox is the fair-use divisor.
	SendsPerMailbox int `json:"sends_per_mailbox"`
	// PlanDailySends is the plan's daily send cap when it has one.
	PlanDailySends *int   `json:"plan_daily_sends,omitempty"`
	PlanName       string `json:"plan_name,omitempty"`
	// Paid is false for a free workspace, whose path to more mailboxes is a
	// plan rather than a limit-increase request.
	Paid bool `json:"paid"`
	// PendingRequest is the open limit-increase request for mailboxes (a
	// [LimitRequest] on the "max_email_accounts" field), if any. File one with
	// [OrganizationService.RequestLimitIncrease].
	PendingRequest *LimitRequest `json:"pending_request,omitempty"`
}

// Unlimited reports whether the workspace may connect any number of mailboxes.
func (a *MailboxAllowance) Unlimited() bool { return a.Allowance == nil }

// CanAdd reports whether n more mailboxes fit.
func (a *MailboxAllowance) CanAdd(n int) bool {
	if a.Allowance == nil {
		return true
	}
	return a.Used+n <= *a.Allowance
}

// Cold-rotation states returned in [SendLifecycleState.State].
const (
	// SendLifecycleActive: in campaign rotation. The default.
	SendLifecycleActive = "active"
	// SendLifecycleResting: pulled out of campaign rotation to recover on
	// warmup traffic alone. Entered automatically when warmup health reaches
	// "throttled" or worse and left on its own once the mailbox has been
	// healthy for three days, or earlier through [EmailService.Release].
	SendLifecycleResting = "resting"
	// SendLifecycleReserve: held back deliberately by the owner through
	// [EmailService.Hold]. Never entered or left automatically.
	SendLifecycleReserve = "reserve"
)

// SendLifecycleState is whether a mailbox is offered to campaign sending,
// separate from its warmup health and from which worker hosts it.
type SendLifecycleState struct {
	// State is [SendLifecycleActive], [SendLifecycleResting] or
	// [SendLifecycleReserve].
	State string `json:"state"`
	// Since is when the mailbox entered State.
	Since *time.Time `json:"since,omitempty"`
	// Reason explains the transition, for example "held back by its owner".
	Reason string `json:"reason,omitempty"`
}

// SendsCold reports whether the mailbox is in campaign rotation.
func (s *SendLifecycleState) SendsCold() bool {
	return s.State == SendLifecycleActive || s.State == ""
}

// Backfill stages returned in [MailboxSyncState.BackfillStatus].
const (
	// SyncBackfillPending: the mailbox is loaded but the import has not run.
	SyncBackfillPending = "pending"
	// SyncBackfillRunning: the import is walking history, newest first, and
	// may be paced.
	SyncBackfillRunning = "running"
	// SyncBackfillComplete: the window is exhausted or the cap was reached.
	SyncBackfillComplete = "complete"
)

// Exhausted budgets named in [MailboxSyncState.ThrottleReason].
const (
	SyncThrottleBurst        = "burst"
	SyncThrottleHourly       = "hourly"
	SyncThrottleDaily        = "daily"
	SyncThrottleOrgDaily     = "org_daily"
	SyncThrottlePriorityFull = "priority_daily"
)

// MailboxSyncPolicy is the fair-use budget a mailbox syncs under. Mail over a
// budget is not dropped: it waits on the server with the cursor held and comes
// in when the window rolls.
type MailboxSyncPolicy struct {
	// BackfillDays is how far back the initial import reaches (90 by default).
	BackfillDays int `json:"backfill_days"`
	// BackfillMessages caps how many messages the initial import stores
	// (5000 by default).
	BackfillMessages int `json:"backfill_messages"`
	// DailyMessages caps new (live) messages stored per UTC day. Replies to
	// the mailbox's own sends have a separate budget of the same size.
	DailyMessages int `json:"daily_messages"`
	// OrgDailyMessages caps new plus backfilled messages stored across the
	// whole organization per UTC day.
	OrgDailyMessages int `json:"org_daily_messages"`
}

// MailboxSyncFolderCursor is the resumable position inside one folder of a
// backfill. It is opaque operational detail, surfaced for diagnostics only.
type MailboxSyncFolderCursor struct {
	// Next is an opaque continuation (Microsoft Graph nextLink).
	Next string `json:"next,omitempty"`
	// UID is the lowest IMAP UID already imported; the walk continues below it.
	UID uint32 `json:"uid,omitempty"`
	// Done marks the folder exhausted for this window.
	Done bool `json:"done,omitempty"`
}

// MailboxSyncCursor is the resumable position of a mailbox's backfill.
type MailboxSyncCursor struct {
	// PageToken is Gmail's messages.list continuation.
	PageToken string `json:"page_token,omitempty"`
	// Folders is the per-folder position for IMAP and Microsoft Graph.
	Folders map[string]MailboxSyncFolderCursor `json:"folders,omitempty"`
}

// MailboxSyncState is what the platform knows about a mailbox's sync: backfill
// progress, whether fair use is holding it, and when it last ran.
type MailboxSyncState struct {
	// BackfillStatus is [SyncBackfillPending], [SyncBackfillRunning] or
	// [SyncBackfillComplete].
	BackfillStatus string            `json:"backfill_status"`
	BackfillCursor MailboxSyncCursor `json:"backfill_cursor"`
	// BackfillSynced counts messages the import has stored so far.
	BackfillSynced int `json:"backfill_synced"`
	// BackfillSince is the cutoff the running import uses, fixed at start so
	// a later settings change does not move the goalposts mid-walk.
	BackfillSince       *time.Time `json:"backfill_since,omitempty"`
	BackfillStartedAt   *time.Time `json:"backfill_started_at,omitempty"`
	BackfillCompletedAt *time.Time `json:"backfill_completed_at,omitempty"`

	// ThrottledUntil is set while fair use is deferring live mail; nil when
	// the mailbox is within budget.
	ThrottledUntil *time.Time `json:"throttled_until,omitempty"`
	// ThrottleReason names the exhausted budget, one of the SyncThrottle*
	// constants.
	ThrottleReason string `json:"throttle_reason,omitempty"`
	// Deferred counts live messages currently waiting on budget: seen on the
	// server but not yet stored. It drops back to zero once they are admitted.
	Deferred int `json:"deferred"`

	LastSyncedAt *time.Time `json:"last_synced_at,omitempty"`
}

// Throttled reports whether fair use is currently deferring live mail.
func (s *MailboxSyncState) Throttled(now time.Time) bool {
	return s.ThrottledUntil != nil && now.Before(*s.ThrottledUntil)
}

// MailboxSync is a mailbox's sync progress together with the budget it runs
// under.
type MailboxSync struct {
	// State is nil until the worker has reported once.
	State  *MailboxSyncState `json:"state"`
	Policy MailboxSyncPolicy `json:"policy"`
}

// Weekday bits for [SendingBehavior.Weekdays]. The mask is Monday-indexed
// (bit 0 = Monday), matching the campaign week grid.
const (
	BehaviorMonday    = 1 << 0
	BehaviorTuesday   = 1 << 1
	BehaviorWednesday = 1 << 2
	BehaviorThursday  = 1 << 3
	BehaviorFriday    = 1 << 4
	BehaviorSaturday  = 1 << 5
	BehaviorSunday    = 1 << 6
	// BehaviorWeekdays is Monday to Friday, the default.
	BehaviorWeekdays = BehaviorMonday | BehaviorTuesday | BehaviorWednesday | BehaviorThursday | BehaviorFriday
	// BehaviorEveryDay is all seven days.
	BehaviorEveryDay = BehaviorWeekdays | BehaviorSaturday | BehaviorSunday
)

// SendingBehavior is a mailbox's humanlike sending profile: the ranges a
// workday is rolled from, not the rolled values (see [DailyPlan]). Each local
// day the mailbox rolls one start, finish, break, daily target and hourly
// ceiling from these ranges and keeps to them, so it behaves like one person
// having one workday rather than a process re-deciding its schedule.
//
// Every minute-of-day value is minutes since local midnight in the mailbox's
// own timezone. Behavior can only ever lower volume or delay a send: the
// rolled daily target is applied as a minimum against the mailbox and campaign
// caps, and the gap range replaces the mailbox's fixed minimum wait.
//
// Disabled profiles are still stored, so the ranges can be tuned before
// switching the profile on. A mailbox that has never been configured reads
// back the defaults.
type SendingBehavior struct {
	EmailAccountID string `json:"email_account_id"`
	// Enabled is off by default; a mailbox that has not opted in keeps its
	// fixed daily cap and minimum gap exactly as before.
	Enabled bool `json:"enabled"`

	// DailyLimitMin and DailyLimitMax bound the day's cold-send target
	// (defaults 30 to 45; 1 to 500).
	DailyLimitMin int `json:"daily_limit_min"`
	DailyLimitMax int `json:"daily_limit_max"`

	// HourlyLimitMin and HourlyLimitMax bound the hourly ceiling (defaults 5
	// to 9; 1 to 200), which stops the whole day landing in one burst.
	HourlyLimitMin int `json:"hourly_limit_min"`
	HourlyLimitMax int `json:"hourly_limit_max"`

	// GapMinSeconds and GapMaxSeconds bound the delay between sends, drawn
	// fresh for every send (defaults 90 to 420; 30 seconds to 24 hours).
	GapMinSeconds int `json:"gap_min_seconds"`
	GapMaxSeconds int `json:"gap_max_seconds"`

	// WorkStartMin and WorkStartMax bound when the mailbox opens (defaults
	// 09:03 to 09:27); WorkEndMin and WorkEndMax bound when it closes
	// (defaults 17:18 to 17:56). The latest start must precede the earliest
	// end.
	WorkStartMin int `json:"work_start_min"`
	WorkStartMax int `json:"work_start_max"`
	WorkEndMin   int `json:"work_end_min"`
	WorkEndMax   int `json:"work_end_max"`

	// LunchEnabled (on by default) carves a quiet gap out of the day that
	// starts between LunchEarliest and LunchLatest (defaults 12:00 to 13:30)
	// and lasts LunchMinMinutes to LunchMaxMinutes (defaults 30 to 60, at most
	// 240). The break must fit inside the shortest workday the ranges can
	// produce.
	LunchEnabled    bool `json:"lunch_enabled"`
	LunchEarliest   int  `json:"lunch_earliest"`
	LunchLatest     int  `json:"lunch_latest"`
	LunchMinMinutes int  `json:"lunch_min_minutes"`
	LunchMaxMinutes int  `json:"lunch_max_minutes"`

	// Weekdays is the Monday-indexed bitmask of sending days, cold and warmup
	// alike (default [BehaviorWeekdays]). An enabled profile needs at least
	// one day.
	Weekdays int `json:"weekdays"`

	// Timezone is a read-only echo of the mailbox's timezone.
	Timezone string `json:"timezone,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WorksOn reports whether the profile sends on the given weekday.
func (b *SendingBehavior) WorksOn(wd time.Weekday) bool {
	return b.Weekdays&(1<<uint((int(wd)+6)%7)) != 0
}

// SendingBehaviorUpdateParams is a partial update to a [SendingBehavior]. Nil
// fields keep their stored value, so Enabled can be toggled without resending
// every range. A range that breaks a cross-field rule is rejected with a 400
// naming the field. Editing the ranges does not change today's rolled plan;
// the new ranges take effect on the next roll.
type SendingBehaviorUpdateParams struct {
	Enabled *bool `json:"enabled,omitempty"`

	DailyLimitMin *int `json:"daily_limit_min,omitempty"`
	DailyLimitMax *int `json:"daily_limit_max,omitempty"`

	HourlyLimitMin *int `json:"hourly_limit_min,omitempty"`
	HourlyLimitMax *int `json:"hourly_limit_max,omitempty"`

	GapMinSeconds *int `json:"gap_min_seconds,omitempty"`
	GapMaxSeconds *int `json:"gap_max_seconds,omitempty"`

	WorkStartMin *int `json:"work_start_min,omitempty"`
	WorkStartMax *int `json:"work_start_max,omitempty"`
	WorkEndMin   *int `json:"work_end_min,omitempty"`
	WorkEndMax   *int `json:"work_end_max,omitempty"`

	LunchEnabled    *bool `json:"lunch_enabled,omitempty"`
	LunchEarliest   *int  `json:"lunch_earliest,omitempty"`
	LunchLatest     *int  `json:"lunch_latest,omitempty"`
	LunchMinMinutes *int  `json:"lunch_min_minutes,omitempty"`
	LunchMaxMinutes *int  `json:"lunch_max_minutes,omitempty"`

	Weekdays *int `json:"weekdays,omitempty"`
}

// DailyPlan is the workday a mailbox actually rolled for the current local
// date, plus how much of it is already spent. It is rolled once per local day
// and never updated, so every scheduling pass reads the same numbers. This is
// the read that answers "why is nothing sending right now".
type DailyPlan struct {
	EmailAccountID string `json:"email_account_id"`
	// PlanDate is the local calendar date in Timezone, as YYYY-MM-DD.
	PlanDate string `json:"plan_date"`
	Timezone string `json:"timezone"`

	// IsWorkingDay is false on a day the profile does not send at all.
	IsWorkingDay bool `json:"is_working_day"`

	// DailyLimit and HourlyLimit are the day's rolled cold-send target and
	// hourly ceiling.
	DailyLimit  int `json:"daily_limit"`
	HourlyLimit int `json:"hourly_limit"`
	// WorkStartMinute and WorkEndMinute are the rolled workday as minutes
	// since local midnight.
	WorkStartMinute int `json:"work_start_minute"`
	WorkEndMinute   int `json:"work_end_minute"`
	// LunchStartMinute and LunchEndMinute bound the rolled break; nil when
	// the day carries none.
	LunchStartMinute *int `json:"lunch_start_minute"`
	LunchEndMinute   *int `json:"lunch_end_minute"`
	GapMinSeconds    int  `json:"gap_min_seconds"`
	GapMaxSeconds    int  `json:"gap_max_seconds"`

	CreatedAt time.Time `json:"created_at"`

	// SentToday is completed cold sends from this mailbox on this local date.
	SentToday int `json:"sent_today"`
	// RemainingToday is what the plan still allows, floored at zero.
	RemainingToday int `json:"remaining_today"`
	// Behavior is the profile the plan was rolled from.
	Behavior SendingBehavior `json:"behavior"`
}

// HasLunch reports whether this day carries a break.
func (p *DailyPlan) HasLunch() bool {
	return p.LunchStartMinute != nil && p.LunchEndMinute != nil
}

// List returns a page of connected mailboxes, newest first.
func (s *EmailService) List(ctx context.Context, params *EmailListParams, opts ...RequestOption) (*Page[Email], error) {
	return listJSON[Email](ctx, s.client, "emails", params.values(), opts...)
}

// Get retrieves a single mailbox by ID.
func (s *EmailService) Get(ctx context.Context, id string, opts ...RequestOption) (*Email, *Response, error) {
	return fetch[Email](ctx, s.client, "emails/"+url.PathEscape(id), opts)
}

// Update modifies a mailbox's settings. Changes apply to the next scheduled
// send, not retroactively.
func (s *EmailService) Update(ctx context.Context, id string, params *EmailUpdateParams, opts ...RequestOption) (*Email, *Response, error) {
	return send[Email](ctx, s.client.patch, "emails/"+url.PathEscape(id), params, opts)
}

// Delete disconnects and removes a mailbox for good, together with its
// imported mail, warmup history, credentials and any send still scheduled for
// it. A campaign that was using it keeps running on its remaining senders.
// Set the mailbox inactive with [EmailService.Update] when you only want it to
// stop.
//
// The worker syncing the mailbox is told to drop it before the record is
// removed. If that instruction cannot be delivered the call fails with a 503
// carrying [ErrCodeMailboxWorkerUnreachable] and nothing is removed; the
// client's default retry policy already retries 5xx responses, so retry again
// in a moment if it still fails rather than assuming it worked.
func (s *EmailService) Delete(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "emails/"+url.PathEscape(id), opts...)
}

// BulkTag adds and removes tags across many mailboxes in one call.
func (s *EmailService) BulkTag(ctx context.Context, params *BulkTagParams, opts ...RequestOption) (*BulkTagResult, *Response, error) {
	return send[BulkTagResult](ctx, s.client.patch, "emails/tags", params, opts)
}

// Allowance reports how many mailboxes the workspace holds, how many it may
// hold and why, and any open request for more. Read it before a connect so a
// refusal is never a surprise after the credentials were typed: every connect
// path fails with a 403 carrying [ErrCodeMailboxAllowanceReached] once
// Remaining is zero. Requires an organization to be selected.
func (s *EmailService) Allowance(ctx context.Context, opts ...RequestOption) (*MailboxAllowance, *Response, error) {
	return fetch[MailboxAllowance](ctx, s.client, "emails/allowance", opts)
}

// GetTrackingDomain returns the mailbox's stored tracking-domain state plus
// the CNAME target this install expects. It is read-only and does no DNS
// work, so it is safe to call on every render; Status is
// [TrackingStatusPending] for a domain that has not been re-resolved. Use
// [EmailService.VerifyTrackingDomain] for the live check.
func (s *EmailService) GetTrackingDomain(ctx context.Context, id string, opts ...RequestOption) (*TrackingDomainStatus, *Response, error) {
	return fetch[TrackingDomainStatus](ctx, s.client, "emails/"+url.PathEscape(id)+"/track", opts)
}

// UpdateTrackingDomain sets the mailbox's custom tracking subdomain (for
// example "t.acme.com"), resolves it once and records the verdict. An empty
// domain clears it and falls back to the shared tracking host.
//
// The value is normalized before it is stored (scheme, path, trailing dot and
// case are stripped); anything that is still not a bare hostname (an IP, a
// host with a port, "localhost", a single label) is rejected with a 400. DNS
// can lag a freshly added record, so a miss is reported as unverified with a
// reason, not as an error. The record must point at
// [TrackingDomainStatus.CNAMETarget].
func (s *EmailService) UpdateTrackingDomain(ctx context.Context, id, domain string, opts ...RequestOption) (*TrackingDomainStatus, *Response, error) {
	q := url.Values{"domain": {domain}}
	return send[TrackingDomainStatus](ctx, s.client.patch, withQuery("emails/"+url.PathEscape(id)+"/track", q), nil, opts)
}

// VerifyTrackingDomain re-resolves the mailbox's saved tracking domain and
// records the verdict without changing the domain itself. This is how a
// record that has finished propagating starts being used straight away; the
// backend also re-checks every custom domain hourly, so this is the impatient
// path rather than the only one. A transient resolver failure never revokes a
// verified domain. Bodyless and derived from public DNS, so it needs no
// idempotency key.
func (s *EmailService) VerifyTrackingDomain(ctx context.Context, id string, opts ...RequestOption) (*TrackingDomainStatus, *Response, error) {
	return send[TrackingDomainStatus](ctx, s.client.post, "emails/"+url.PathEscape(id)+"/track/verify", nil, opts)
}

// AuthCheck runs a live SPF/DKIM/DMARC lookup against the mailbox's sending
// domain. It is read-only: it reports what DNS says right now and leaves the
// mailbox's stored [Email.AuthState] alone. Use
// [EmailService.RefreshAuthCheck] to record the verdict.
func (s *EmailService) AuthCheck(ctx context.Context, id string, opts ...RequestOption) (*DomainAuthCheck, *Response, error) {
	return fetch[DomainAuthCheck](ctx, s.client, "emails/"+url.PathEscape(id)+"/auth-check", opts)
}

// RefreshAuthCheck runs the same lookup as [EmailService.AuthCheck] and
// records the verdict against every active mailbox on the sending domain,
// since authentication is a property of the domain rather than of one
// mailbox. This is how a mailbox blocked by the send gate is unblocked: fix
// the DNS records, call this, and cold sending and warmup resume on the next
// scheduled send instead of waiting for the daily background check.
//
// It needs the write scope because recording the verdict is what lifts the
// gate. Bodyless and derived from public DNS with no caller input, so
// repeating it converges on the same stored state.
func (s *EmailService) RefreshAuthCheck(ctx context.Context, id string, opts ...RequestOption) (*DomainAuthCheck, *Response, error) {
	return send[DomainAuthCheck](ctx, s.client.post, "emails/"+url.PathEscape(id)+"/auth-check", nil, opts)
}

// SyncStatus reports where the mailbox's import stands, whether fair use is
// holding it, and the budget it runs under. State is nil until the worker has
// reported once.
func (s *EmailService) SyncStatus(ctx context.Context, id string, opts ...RequestOption) (*MailboxSync, *Response, error) {
	return fetch[MailboxSync](ctx, s.client, "emails/"+url.PathEscape(id)+"/sync", opts)
}

// Behavior returns the mailbox's sending-behavior profile, substituting the
// defaults for a mailbox that has never been configured so the result is
// always a complete profile. Deployments without the behavior engine answer
// with a 400.
func (s *EmailService) Behavior(ctx context.Context, id string, opts ...RequestOption) (*SendingBehavior, *Response, error) {
	return fetch[SendingBehavior](ctx, s.client, "emails/"+url.PathEscape(id)+"/behavior", opts)
}

// UpdateBehavior applies a partial update to the mailbox's sending-behavior
// profile and returns the whole profile. Nil fields keep their stored value.
// The body is the desired state, so a retry converges on the same profile and
// no idempotency key is needed.
func (s *EmailService) UpdateBehavior(ctx context.Context, id string, params *SendingBehaviorUpdateParams, opts ...RequestOption) (*SendingBehavior, *Response, error) {
	return send[SendingBehavior](ctx, s.client.put, "emails/"+url.PathEscape(id)+"/behavior", params, opts)
}

// BehaviorPlan returns the workday the mailbox rolled for the current local
// date and how much of it is already spent.
func (s *EmailService) BehaviorPlan(ctx context.Context, id string, opts ...RequestOption) (*DailyPlan, *Response, error) {
	return fetch[DailyPlan](ctx, s.client, "emails/"+url.PathEscape(id)+"/behavior/plan", opts)
}

// Hold takes the mailbox out of campaign sending until [EmailService.Release]
// puts it back, moving it to [SendLifecycleReserve]. Warmup keeps running.
// This is the owner's decision: the automatic rest-and-resume logic never
// touches a held mailbox. Bodyless and idempotent, so holding an already held
// mailbox simply returns its current state. Installs without the lifecycle
// engine answer with a 409.
func (s *EmailService) Hold(ctx context.Context, id string, opts ...RequestOption) (*SendLifecycleState, *Response, error) {
	return send[SendLifecycleState](ctx, s.client.post, "emails/"+url.PathEscape(id)+"/hold", nil, opts)
}

// Release puts a held or resting mailbox back into automatic management. It
// lands in [SendLifecycleActive], or straight in [SendLifecycleResting] when
// warmup is running and still reports the mailbox as throttled or worse, so a
// release never sends cold mail from a mailbox that warmup can see is
// struggling; the returned Reason says which. It is also the manual exit from
// an automatic rest. Bodyless and idempotent.
func (s *EmailService) Release(ctx context.Context, id string, opts ...RequestOption) (*SendLifecycleState, *Response, error) {
	return send[SendLifecycleState](ctx, s.client.post, "emails/"+url.PathEscape(id)+"/release", nil, opts)
}

// Verify checks whether a recipient address is deliverable, before anything is
// ever sent to it, through whichever verifier the workspace uses (its
// connected provider, else the built-in syntax, MX and SMTP probe). Nothing
// is stored; use the contacts verification endpoints to record verdicts.
func (s *EmailService) Verify(ctx context.Context, email string, opts ...RequestOption) (*VerifyResult, *Response, error) {
	body := struct {
		Email string `json:"email"`
	}{Email: email}
	return send[VerifyResult](ctx, s.client.post, "emails/verify", body, opts)
}

// Send sends a one-off message from the given mailbox, dispatched through the
// mailbox's assigned worker. It requires the send-campaigns scope and an
// organization. Pass [WithIdempotencyKey] when you may retry, so a retried
// call cannot send twice.
func (s *EmailService) Send(ctx context.Context, id string, params *SendEmailParams, opts ...RequestOption) (*SendResult, *Response, error) {
	return send[SendResult](ctx, s.client.post, "emails/"+url.PathEscape(id)+"/send", params, opts)
}

// StartWarmup enables warmup for a mailbox, resuming from the existing ramp
// progress when it was previously paused, and seeds the warmup task chain
// immediately rather than waiting for the next reconciler pass.
func (s *EmailService) StartWarmup(ctx context.Context, id string, opts ...RequestOption) (*Email, *Response, error) {
	return s.warmupLifecycle(ctx, id, "start", opts)
}

// PauseWarmup pauses warmup without losing ramp progress. The mailbox stays in
// its pool, so its health keeps being tracked.
func (s *EmailService) PauseWarmup(ctx context.Context, id string, opts ...RequestOption) (*Email, *Response, error) {
	return s.warmupLifecycle(ctx, id, "pause", opts)
}

// ResumeWarmup resumes a paused warmup, shifting the ramp anchor forward so
// progress continues at the same daily volume.
func (s *EmailService) ResumeWarmup(ctx context.Context, id string, opts ...RequestOption) (*Email, *Response, error) {
	return s.warmupLifecycle(ctx, id, "resume", opts)
}

// StopWarmup disables warmup and clears ramp progress; a later start begins a
// fresh ramp. Use [EmailService.PauseWarmup] to keep the progress.
func (s *EmailService) StopWarmup(ctx context.Context, id string, opts ...RequestOption) (*Email, *Response, error) {
	return s.warmupLifecycle(ctx, id, "stop", opts)
}

func (s *EmailService) warmupLifecycle(ctx context.Context, id, action string, opts []RequestOption) (*Email, *Response, error) {
	return send[Email](ctx, s.client.post, "emails/"+url.PathEscape(id)+"/warmup/"+action, nil, opts)
}

// WarmupBanStatus reports whether a mailbox has been blocked from the warmup
// pool, why, and whether the owner can appeal.
func (s *EmailService) WarmupBanStatus(ctx context.Context, id string, opts ...RequestOption) (*WarmupBanStatus, *Response, error) {
	return fetch[WarmupBanStatus](ctx, s.client, "emails/"+url.PathEscape(id)+"/warmup/ban-status", opts)
}

// AppealWarmupBan submits an appeal against a mailbox's warmup ban.
func (s *EmailService) AppealWarmupBan(ctx context.Context, id string, params *WarmupAppealParams, opts ...RequestOption) (*WarmupAppealResult, *Response, error) {
	return send[WarmupAppealResult](ctx, s.client.post, "emails/"+url.PathEscape(id)+"/warmup/appeal", params, opts)
}

// --- connecting a mailbox ---
//
// These routes are session-only. Connecting a mailbox writes a refresh token
// encrypted to the connecting user, which is not something a long-lived API key
// should be able to trigger. Every first connect counts against the workspace's
// mailbox allowance ([EmailService.Allowance]) and fails with a 403 carrying
// [ErrCodeMailboxAllowanceReached] once it is full; reconnecting an existing
// mailbox never does.

// StartOAuth begins connecting a Gmail or Outlook mailbox and returns the URL
// to send the user to. Provider is [ProviderGmail] or [ProviderOutlook].
//
// The provider redirects to a Warmbly-hosted page that posts the code and state
// back to the opener; pass those to [EmailService.FinishOAuth].
func (s *EmailService) StartOAuth(ctx context.Context, provider string, opts ...RequestOption) (*OAuthStartResult, *Response, error) {
	body := struct {
		Provider string `json:"provider"`
	}{Provider: provider}
	return send[OAuthStartResult](ctx, s.client.post, "emails/onboarding/oauth/start", body, opts)
}

// FinishOAuth exchanges the authorization code for a connected mailbox. It
// completes both a first connect (answered 201 with the new mailbox) and a
// [EmailService.ReauthOAuth] round trip (answered 200 with the renewed one);
// check the Response status code to tell them apart.
func (s *EmailService) FinishOAuth(ctx context.Context, code, state string, opts ...RequestOption) (*Email, *Response, error) {
	body := struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}{Code: code, State: state}
	return send[Email](ctx, s.client.post, "emails/onboarding/oauth/finish", body, opts)
}

// ReauthOAuth starts an OAuth round trip that renews the tokens of an existing
// Gmail or Outlook mailbox after the provider invalidated them (a password
// change, a revoked grant). The finish leg is the ordinary
// [EmailService.FinishOAuth]; the consent must be for the mailbox's own
// address, and signing in with a different account is refused rather than
// quietly connecting the wrong mailbox. A successful reconnect keeps every
// setting, its history and its warmup progress, and never counts against the
// allowance.
//
// An SMTP/IMAP mailbox is refused with a 400 (use
// [EmailService.UpdateSMTPIMAPCredentials]), and a mailbox whose sign-in is
// held by Warmbly Cloud with a 409. Requires the manage-emails permission.
func (s *EmailService) ReauthOAuth(ctx context.Context, id string, opts ...RequestOption) (*OAuthStartResult, *Response, error) {
	return send[OAuthStartResult](ctx, s.client.post, "emails/onboarding/oauth/reauth/"+url.PathEscape(id), nil, opts)
}

// ConnectSMTPIMAP connects a mailbox by its own credentials. The server dials
// both legs to validate them before storing anything, so a bad password fails
// here rather than silently at send time. An address that is already
// connected is refused with a 409.
func (s *EmailService) ConnectSMTPIMAP(ctx context.Context, params *SMTPIMAPParams, opts ...RequestOption) (*Email, *Response, error) {
	return send[Email](ctx, s.client.post, "emails/onboarding/smtp-imap", params, opts)
}

// ConnectSMTPIMAPBulk connects up to [MaxSMTPIMAPBulkRows] SMTP/IMAP
// mailboxes in one call, answered per row so one bad row never hides the
// others: the call itself succeeds with a 200 even when every row failed, so
// read [MailboxBulkResult.Summary] and each row's Status. Rows past the
// workspace's allowance fail with [ErrCodeMailboxAllowanceReached] before any
// credential is dialed. Re-sending a batch is safe: an already connected
// mailbox is [MailboxBulkSkipped], never doubled, so no idempotency key is
// needed. Every credential is validated against its own server, so a large
// batch takes a few seconds per mailbox.
func (s *EmailService) ConnectSMTPIMAPBulk(ctx context.Context, params *SMTPIMAPBulkParams, opts ...RequestOption) (*MailboxBulkResult, *Response, error) {
	return send[MailboxBulkResult](ctx, s.client.post, "emails/onboarding/smtp-imap/bulk", params, opts)
}

// UpdateSMTPIMAPCredentials replaces an existing SMTP/IMAP mailbox's
// credentials after a password or server change, validating them live before
// storing, then clears the authentication error and reactivates the mailbox
// on its existing worker so it resumes syncing from where it stopped. The
// address and display name never change. An OAuth mailbox is refused with a
// 400 (use [EmailService.ReauthOAuth]). Requires the manage-emails permission.
func (s *EmailService) UpdateSMTPIMAPCredentials(ctx context.Context, id string, params *SMTPIMAPCredentialsParams, opts ...RequestOption) (*Email, *Response, error) {
	return send[Email](ctx, s.client.put, "emails/onboarding/smtp-imap/"+url.PathEscape(id), params, opts)
}
