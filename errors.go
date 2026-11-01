package warmbly

import (
	"fmt"
	"net/http"
)

// Error is the typed error returned for any non-2xx API response. It mirrors
// the JSON error envelope returned by the Warmbly API, which is the same four
// fields on every endpoint — there is no nested "details" or per-field
// validation object to dig through:
//
//	{
//	  "error":      "Not Found",
//	  "message":    "Resource not found.",
//	  "code":       "not_found",
//	  "reason":     "...",
//	  "request_id": "0b6f...",
//	  "retry_after": 30
//	}
//
// "error" and "message" are for people; branch on [Error.Code] and the HTTP
// status. "reason" is an optional finer key that only a few codes carry.
// "retry_after" appears only where the endpoint has a wait to quote (rate
// limiting); the client also fills it from the Retry-After header.
//
// The package sentinels ([ErrNotFound], [ErrUnauthorized], ...) can be matched
// with errors.Is to branch on the HTTP status without parsing the body, and
// [Error.HasCode] on the stable code for the specific condition:
//
//	if errors.Is(err, warmbly.ErrNotFound) {
//		// handle a 404
//	}
//
//	var apiErr *warmbly.Error
//	if errors.As(err, &apiErr) && apiErr.HasCode(warmbly.ErrCodeStorageLimitReached) {
//		// the quota, not a malformed request
//	}
type Error struct {
	// StatusCode is the HTTP status code of the response.
	StatusCode int `json:"-"`
	// Type is the machine-readable error family (the API "error" field).
	Type string `json:"error"`
	// Message is the human-readable description.
	Message string `json:"message"`
	// Code is a stable, machine-readable error code.
	Code string `json:"code"`
	// Reason is an optional finer key some codes carry when one code has more
	// than one next step. Treat an empty or unknown Reason as the plain Code.
	Reason string `json:"reason,omitempty"`
	// RequestID identifies the request server-side and should be included in
	// any support correspondence.
	RequestID string `json:"request_id"`
	// RetryAfter is the number of seconds to wait before retrying, when the
	// server provides it (typically on 429 responses).
	RetryAfter int `json:"retry_after,omitempty"`
	// Header is the full set of response headers, for inspection.
	Header http.Header `json:"-"`
}

// Error implements the error interface.
func (e *Error) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}
	parts := fmt.Sprintf("warmbly: %d", e.StatusCode)
	if e.Code != "" {
		parts += " (" + e.Code + ")"
	}
	parts += ": " + msg
	if e.RequestID != "" {
		parts += fmt.Sprintf(" [request_id=%s]", e.RequestID)
	}
	return parts
}

// Is reports whether the error matches target. It enables matching against the
// package sentinels by HTTP status. The [ErrServer] sentinel matches any 5xx
// status.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	if t == ErrServer {
		return e.StatusCode >= 500
	}
	return e.StatusCode == t.StatusCode
}

// HasCode reports whether the response carried the given stable error code. It
// is nil-safe, so it can be used on the result of an errors.As that did not
// match:
//
//	var apiErr *warmbly.Error
//	errors.As(err, &apiErr)
//	if apiErr.HasCode(warmbly.ErrCodeNoOrganization) { ... }
//
// Compare against the ErrCode* constants rather than literals: a code is stable
// where the human-readable message is not.
func (e *Error) HasCode(code string) bool {
	return e != nil && code != "" && e.Code == code
}

// Temporary reports whether the error is likely transient and worth retrying:
// rate limiting (429) or server errors (5xx).
//
// It is a status-level guess. Two 5xx codes are not transient at all —
// [ErrCodeMailboxProviderNotConfigured] and, on the mailbox delete path,
// [ErrCodeMailboxWorkerUnreachable] — so check [Error.HasCode] before building
// a retry loop around them.
func (e *Error) Temporary() bool {
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= 500
}

// Stable machine-readable values of the "code" field in the error envelope, for
// callers that need to branch on the specific condition rather than the HTTP
// status. Match them with [Error.HasCode].
//
// Every response carries one: an endpoint that has nothing more specific to say
// answers with the generic code for its status class. A few codes live with the
// service they belong to instead of here, such as
// [ErrCodeMailboxAllowanceReached], [ErrCodeMailboxWorkerUnreachable],
// [ErrCodeListBounceRisk] and [ErrCodeLeadsUndeliverable]; the second block
// below lists the area-specific codes.
const (
	// ErrCodeBadRequest is the generic 400: malformed JSON, a missing required
	// field, a value out of range.
	ErrCodeBadRequest = "bad_request"
	// ErrCodeUnauthorized is the generic 401: no credential, or one the server
	// would not accept.
	ErrCodeUnauthorized = "unauthorized"
	// ErrCodeForbidden is the generic 403: authenticated, but the credential
	// lacks the permission (or the IP is outside the key's allowlist).
	ErrCodeForbidden = "forbidden"
	// ErrCodeNotFound is the generic 404. It is also what a resource in
	// another workspace returns, so it does not prove non-existence.
	ErrCodeNotFound = "not_found"
	// ErrCodeConflict is the generic 409: the resource already exists, or a
	// unique value collided.
	ErrCodeConflict = "conflict"
	// ErrCodeUnprocessable is the generic 422: the request parsed but failed
	// validation.
	ErrCodeUnprocessable = "unprocessable"
	// ErrCodeRateLimitExceeded is the 429 for the per-key request rate. Honor
	// [Error.RetryAfter]; the client's own retry does.
	ErrCodeRateLimitExceeded = "rate_limit_exceeded"
	// ErrCodeInternalError is the generic 500. Retry once, then quote
	// [Error.RequestID] to support.
	ErrCodeInternalError = "internal_error"
	// ErrCodeNotImplemented is the 501 for a feature this deployment does not
	// build in.
	ErrCodeNotImplemented = "not_implemented"
	// ErrCodeServiceUnavailable is the generic 503, and the only genuinely
	// transient one of the three 503 codes.
	ErrCodeServiceUnavailable = "service_unavailable"

	// ErrCodeInsufficientCredits is the 402 every AI action returns once the
	// workspace's credit balance is spent. Top up or wait for the monthly
	// allowance; retrying changes nothing.
	ErrCodeInsufficientCredits = "insufficient_credits"
	// ErrCodeUsageCapExceeded is a 429 from the AI endpoints: a short-term
	// usage cap, not the request-rate limiter. Unlike
	// [ErrCodeInsufficientCredits] it clears on its own, so retry later.
	ErrCodeUsageCapExceeded = "usage_cap_exceeded"

	// ErrCodeNoOrganization is a 400 raised when a request needs a workspace
	// and the session has none selected. API keys always carry theirs, so this
	// is a dashboard-session condition: every entitlement, limit and
	// suppression rule is workspace-scoped, and a write that would run
	// unscoped is refused rather than run without those checks.
	ErrCodeNoOrganization = "no_organization"
	// ErrCodeStorageLimitReached is the 400 from campaign attachment uploads
	// (and from duplicating a campaign that has them) when the workspace's
	// total attachment storage would pass its quota. The check and the write
	// happen under one lock, so nothing was stored. GET
	// organization/current/limits reports the quota and what is used.
	ErrCodeStorageLimitReached = "storage_limit_reached"
	// ErrCodeMailboxProviderNotConfigured is a 503 that is not transient:
	// the deployment has no OAuth client for the mailbox provider the request
	// named. Self-hosted only. Set the provider's credentials, or connect the
	// mailbox over SMTP and IMAP instead.
	ErrCodeMailboxProviderNotConfigured = "mailbox_provider_not_configured"

	// ErrCodeInvalidLeadStatus and ErrCodeInvalidEngagement are 400s from the
	// contact search and export endpoints for a filter value outside the
	// documented set.
	ErrCodeInvalidLeadStatus = "invalid_lead_status"
	ErrCodeInvalidEngagement = "invalid_engagement"
	// ErrCodeLeadFilterRequiresCampaign is the 400 for setting a lead status
	// or engagement filter without exactly one campaign id. Both describe a
	// contact's standing inside one campaign, so they are meaningless without
	// it.
	ErrCodeLeadFilterRequiresCampaign = "lead_filter_requires_campaign"
	// ErrCodeUnknownVerificationStatus and ErrCodeUnknownVerificationProvider
	// are 400s raised when a stored contact carries a verification status or
	// provider this platform version cannot read — a row written by a newer
	// deployment, or by a service that has since been removed.
	ErrCodeUnknownVerificationStatus   = "unknown_verification_status"
	ErrCodeUnknownVerificationProvider = "unknown_verification_provider"
	// ErrCodeInvalidAction is the 400 from the contact verification endpoint
	// for an action other than verify, mark_deliverable or
	// mark_undeliverable.
	ErrCodeInvalidAction = "invalid_action"
	// ErrCodeNoContacts is the 400 from the contact verification endpoint when
	// the selection resolved to nothing: no explicit contacts, and no campaign
	// with refused leads.
	ErrCodeNoContacts = "no_contacts"

	// ErrCodeRegistrationInviteOnly and ErrCodeRegistrationClosed are 403s
	// describing a deployment's signup policy. They never reveal whether an
	// address already has an account.
	ErrCodeRegistrationInviteOnly = "registration_invite_only"
	ErrCodeRegistrationClosed     = "registration_closed"
	// ErrCodeInvitationInvalid is the 403 for an invitation that is expired,
	// canceled, already used, or was issued for a different address.
	ErrCodeInvitationInvalid = "invitation_invalid"
	// ErrCodeSetupTokenInvalid is the 401 from the first-run claim endpoint
	// for a setup link that is invalid, already used or expired.
	ErrCodeSetupTokenInvalid = "setup_token_invalid"
	// ErrCodeSetupAlreadyComplete is the 403 for claiming an instance that
	// already has an account.
	ErrCodeSetupAlreadyComplete = "setup_already_complete"
	// ErrCodeSSOWrongBrowser is the 401 from the SSO exchange when the handoff
	// code arrives without the binding secret the browser that started the
	// sign-in was given. The handoff is deliberately non-transferable: a
	// forwarded sign-in link cannot sign the recipient in.
	ErrCodeSSOWrongBrowser = "sso_wrong_browser"
)

// Stable "code" values for the specific conditions the API reports, grouped by
// the area they come from. Match them with [Error.HasCode]. The status each one
// carries is noted; a code never changes status, but the server adds codes over
// time, so treat an unknown code as its HTTP status class.
const (
	// Request validation.

	// ErrCodeDuplicateQueryParameter is a 400: a query parameter appeared more
	// than once on a route that reads it as a single value. The message names
	// it. Send lists as one comma-separated value.
	ErrCodeDuplicateQueryParameter = "duplicate_query_parameter"
	// ErrCodeDuplicateHeader is a 400: Authorization, X-Warmbly-Token,
	// Idempotency-Key or Origin was sent on more than one header line. Send each
	// of them once.
	ErrCodeDuplicateHeader = "duplicate_header"
	// ErrCodeInvalidSortBy is a 400: POST /contacts/search, POST
	// /contacts/export or a bulk action's all selection was given a sort_by of
	// the form custom:<key> whose key could never be a custom-field name
	// (letters, numbers, underscores, spaces or dashes).
	ErrCodeInvalidSortBy = "invalid_sort_by"
	// ErrCodeInvalidColumn is a 400: PUT /me/views/:view was given a column id
	// that view cannot render.
	ErrCodeInvalidColumn = "invalid_column"
	// ErrCodeDuplicateColumn is a 400: PUT /me/views/:view named the same column
	// twice.
	ErrCodeDuplicateColumn = "duplicate_column"
	// ErrCodeTooManyColumns is a 400: PUT /me/views/:view was given more than 64
	// columns.
	ErrCodeTooManyColumns = "too_many_columns"
	// ErrCodeInvalidSort is a 400: PUT /me/views/:view was given a sort that
	// names neither a sortable contact column nor a well-formed custom:<key>.
	ErrCodeInvalidSort = "invalid_sort"
	// ErrCodeInvalidLayout is a 400: PUT /me/views/:view was given a layout on a
	// view that has none, or a unibox_rail layout with an unknown field, an
	// empty or over-long key (200 bytes), more than 500 keys in a list or 32
	// sections, or a favorite name over 40 characters.
	ErrCodeInvalidLayout = "invalid_layout"
	// ErrCodeInvalidMailHost is a 400: POST /contacts/search, POST
	// /contacts/export or a bulk action's all selection was given a mail_hosts
	// entry that is not a documented provider value.
	ErrCodeInvalidMailHost = "invalid_mail_host"
	// ErrCodeEmptyStepBody is a 400: POST /campaigns/:id/start found an email
	// step with nothing in either body, so it would send a blank message to
	// every lead it reached. Write the step's body and start again.
	ErrCodeEmptyStepBody = "empty_step_body"
	// ErrCodeTooManyTasks is a 400: PATCH /crm/tasks or DELETE /crm/tasks was
	// given more than 1000 ids in one request, or more than 50,000 exclusions.
	// Split it into batches.
	ErrCodeTooManyTasks = "too_many_tasks"
	// ErrCodeSelectionTooLarge is a 400: a "all": true bulk selection resolved
	// to more than its limit: 250,000 contacts, or 50,000 CRM tasks. Narrow the
	// filter and run it in parts; nothing was changed.
	ErrCodeSelectionTooLarge = "selection_too_large"
	// ErrCodeInvalidFilter is a 400: a task filter carried an id that is not
	// one: assigned_to, contact_id and deal_id name records, and are matched
	// against id columns. Sent by POST /crm/tasks/search, POST
	// /crm/tasks/summary, and the filters of a "all": true bulk selection.
	ErrCodeInvalidFilter = "invalid_filter"
	// ErrCodeInvalidSetting is a 400: PATCH /outreach/settings (or a campaign's
	// advanced settings) carried a value outside the documented vocabulary, for
	// example a reply_intent.crm_task_intents entry that is not a reply intent,
	// or an inbox_tagging.questions entry with no question text, a label that is
	// missing, repeated or built in, or a choice question with fewer than two
	// options, or an inbox_tagging.languages entry that is not a supported
	// language code.
	ErrCodeInvalidSetting = "invalid_setting"
	// ErrCodeInvalidSlug is a 400: PATCH /organization/current was given a slug
	// that is not 2 to 80 lowercase letters, numbers or dashes starting and
	// ending with a letter or number.
	ErrCodeInvalidSlug = "invalid_slug"
	// ErrCodeInvalidSyncFolder is a 400: PUT /emails/:id/sync was given a folder
	// the sync always follows (INBOX, or a sent, drafts, spam, trash or archive
	// folder by attribute or name), a name that is empty after trimming, longer
	// than 255 characters or carrying a control character, more than 50 names,
	// or a mailbox that is not IMAP.
	ErrCodeInvalidSyncFolder = "invalid_sync_folder"
	// ErrCodeInvalidTLSPin is a 400: an SMTP/IMAP connect or reconnect carried a
	// tls_pin that is not a SHA-256 fingerprint (64 hexadecimal characters;
	// colons, spaces and a sha256: prefix are ignored), or POST
	// /emails/onboarding/smtp-imap/:id/certificates/confirm was missing the
	// fingerprint of a server the mailbox trusts on its own certificate.
	ErrCodeInvalidTLSPin = "invalid_tls_pin"
	// ErrCodeMailboxCertificateNotPinned is a 400: POST /emails/onboarding/smtp-
	// imap/:id/certificates/confirm on a mailbox whose servers are both verified
	// by a certificate authority, so there is no certificate to confirm.
	ErrCodeMailboxCertificateNotPinned = "mailbox_certificate_not_pinned"
	// ErrCodeLeadCcLimit is a 400: Set a lead's CC was given more than 2
	// contacts.
	ErrCodeLeadCcLimit = "lead_cc_limit"
	// ErrCodeLeadCcSelf is a 400: Set a lead's CC named the lead itself as a
	// copy.
	ErrCodeLeadCcSelf = "lead_cc_self"
	// ErrCodeInvalidRecipient is a 400: POST /emails/:id/send, POST
	// /unibox/reply or POST /unibox/compose was given a to, cc or bcc entry that
	// is not exactly one email address (ana@example.com or Ana
	// <ana@example.com>), or no to address at all. Put each address in its own
	// entry.
	ErrCodeInvalidRecipient = "invalid_recipient"
	// ErrCodeRecipientSuppressed is a 400: POST /emails/:id/send, POST
	// /unibox/reply or POST /unibox/compose named a to, cc or bcc address on the
	// workspace suppression list, directly or through its domain. The message
	// names the address and the entry. Nothing was queued.
	ErrCodeRecipientSuppressed = "recipient_suppressed"
	// ErrCodeInvalidMessageID is a 400: POST /emails/:id/send or POST
	// /unibox/reply was given an in_reply_to entry that is not one Message-ID:
	// printable characters with no spaces or line breaks, with or without the
	// surrounding < and >.
	ErrCodeInvalidMessageID = "invalid_message_id"

	// Image upload refusals.

	// ErrCodeImageUnreadable is a 400: the file is empty or does not decode as
	// an image: it is damaged, or it is a different format than its name
	// suggests.
	ErrCodeImageUnreadable = "image_unreadable"
	// ErrCodeImageFormatNotAllowed is a 400: the file is an image, but not a
	// format this upload takes.
	ErrCodeImageFormatNotAllowed = "image_format_not_allowed"
	// ErrCodeImageTooLarge is a 400: the file is over the size limit.
	ErrCodeImageTooLarge = "image_too_large"
	// ErrCodeImageDimensionsTooLarge is a 400: the image is wider or taller than
	// the limit.
	ErrCodeImageDimensionsTooLarge = "image_dimensions_too_large"
	// ErrCodeImageTooSmall is a 400: an OAuth app logo is under 32px on a side.
	ErrCodeImageTooSmall = "image_too_small"

	// Password refusals.

	// ErrCodePasswordBreached is a 400: the password appears in a public list of
	// breached passwords and was refused. Choose one that does not.
	ErrCodePasswordBreached = "password_breached"
	// ErrCodeSSOLinkExpired is a 400: the pending token from a link_required
	// sign-in is unknown, expired (ten minutes), already used, or has had three
	// wrong passwords. Start the provider sign-in again.
	ErrCodeSSOLinkExpired = "sso_link_expired"
	// ErrCodePasswordReused is a 400: POST /auth/login/initial-password was
	// given the issued initial password itself. Choose a password of your own.
	ErrCodePasswordReused = "password_reused"
	// ErrCodePasswordChangeExpired is a 400: the pending token from a
	// password_change_required sign-in is unknown, expired (ten minutes) or
	// already used, or the issued password it was for has run out.
	ErrCodePasswordChangeExpired = "password_change_expired"
	// ErrCodePasswordChangeRequired is a 403: POST /auth/sso/link was given an
	// issued initial password. Sign in with email and password first, which asks
	// you to choose your own, then link the provider.
	ErrCodePasswordChangeRequired = "password_change_required"
	// ErrCodeResetLinkInvalid is a 401: POST /auth/reset-password/session or
	// /confirm was given a reset link or reset session that expired, was already
	// used, or was issued before the password last changed. Request a new link.
	ErrCodeResetLinkInvalid = "reset_link_invalid"
	// ErrCodeAPICredentialsRevokeUnsupported is a 400: POST /auth/reset-
	// password/confirm or POST /auth/me/password asked to end connected apps and
	// API keys (revoke_api_credentials) on a server that cannot. The password
	// was not changed. Send it again without that option, then remove apps and
	// keys from Settings.
	ErrCodeAPICredentialsRevokeUnsupported = "api_credentials_revoke_unsupported"
	// ErrCodeSessionsNotEnded is a 503: the same two routes could not sign out
	// every device on the account, so the password was not changed.
	ErrCodeSessionsNotEnded = "sessions_not_ended"
	// ErrCodeIdentityNotFound is a 404: DELETE /auth/me/identities/:provider
	// named something other than google, apple or oidc.
	ErrCodeIdentityNotFound = "identity_not_found"
	// ErrCodeLastSignInMethod is a 409: DELETE /auth/me/identities/:provider or
	// DELETE /auth/passkey/credentials/:id would leave the account with no
	// password, no passkey and no other provider. Nothing was removed. Set a
	// password (POST /auth/me/password/link) or add a passkey first.
	ErrCodeLastSignInMethod = "last_sign_in_method"
	// ErrCodePasswordLoginDisabled is a 403: POST /auth/me/password/link was
	// called on a server that turned password sign-in off
	// (DISABLE_PASSWORD_LOGIN), so a password could never be used. No link was
	// sent.
	ErrCodePasswordLoginDisabled = "password_login_disabled"

	// Cloud connection refusals.

	// ErrCodeCloudLinkWorkspaceRequired is a 409: Disconnecting a shared server
	// connection requires explicit confirmation of the server-wide scope; it
	// affects all local workspaces using that connection.
	ErrCodeCloudLinkWorkspaceRequired = "cloud_link_workspace_required"
	// ErrCodePoolLinkWorkspaceRequired is a 409: cloud refused a new connection
	// request without local workspace scope. Update the self-hosted instance
	// before creating a new connection; existing connections can still enroll
	// mailboxes and start placement tests.
	ErrCodePoolLinkWorkspaceRequired = "pool_link_workspace_required"
	// ErrCodePoolLinkWorkspaceConnected is a 409: the Cloud workspace already
	// has an active self-hosted connection, including a legacy link. Use another
	// Cloud workspace and plan, or disconnect the existing link first.
	ErrCodePoolLinkWorkspaceConnected = "pool_link_workspace_connected"
	// ErrCodeCloudLinkUpgradeRequired is a 409: this Cloud deployment does not
	// support workspace-scoped connections. Upgrade Cloud first.
	ErrCodeCloudLinkUpgradeRequired = "cloud_link_upgrade_required"
	// ErrCodeOrganizationCloudConnected is a 409: disconnect the workspace's
	// Cloud connection before deleting it or scheduling deletion of it or its
	// owner account.
	ErrCodeOrganizationCloudConnected = "organization_cloud_connected"
	// ErrCodeCloudLinkNoKey is a 409: the instance has no
	// CREDENTIALS_ENCRYPTION_KEY, so it cannot store the Warmbly Cloud token.
	// Set one and restart the backend before connecting.
	ErrCodeCloudLinkNoKey = "cloud_link_no_key"
	// ErrCodeCloudLinkURL is a 400: the Cloud URL does not start with https://.
	ErrCodeCloudLinkURL = "cloud_link_url"
	// ErrCodeCloudLinkLifecycle is a 400: a pause or resume request named an
	// action other than pause or resume.
	ErrCodeCloudLinkLifecycle = "cloud_link_lifecycle"
	// ErrCodeCloudLinkExecutor is a 400: an enrollment named an executor other
	// than cloud or instance.
	ErrCodeCloudLinkExecutor = "cloud_link_executor"
	// ErrCodeCloudLinkSelfExecutionNotGranted is a 403: keeping a mailbox's
	// credential on this server needs a permission Warmbly grants to the linked
	// Cloud workspace, and it has none or it expired. Ask Warmbly support, or
	// enroll with executor cloud.
	ErrCodeCloudLinkSelfExecutionNotGranted = "cloud_link_self_execution_not_granted"
	// ErrCodeCloudLinkSelfExecutionUnavailable is a 409: the connected Warmbly
	// Cloud cannot run warmup for a mailbox whose credential stays on this
	// server. Upgrade Cloud, or enroll with executor cloud.
	ErrCodeCloudLinkSelfExecutionUnavailable = "cloud_link_self_execution_unavailable"
	// ErrCodeCloudLinkSelfExecutionManaged is a 422: the mailbox signs in
	// through Warmbly Cloud or is associated with its app-password copy there,
	// so its credential already lives on Cloud and cannot stay on this server.
	ErrCodeCloudLinkSelfExecutionManaged = "cloud_link_self_execution_managed"

	// Warmbly Cloud pool link.

	// ErrCodePoolLinkRequest is a 400: the connection request is missing the
	// instance name.
	ErrCodePoolLinkRequest = "pool_link_request"
	// ErrCodePoolLinkCredential is a 400: enrolling a mailbox needs a credential
	// that matches the mailbox's provider.
	ErrCodePoolLinkCredential = "pool_link_credential"
	// ErrCodePoolLinkInstanceNotFound is a 404: the linked instance no longer
	// exists in Warmbly Cloud.
	ErrCodePoolLinkInstanceNotFound = "pool_link_instance_not_found"
	// ErrCodePoolLinkMailboxNotFound is a 404: the mailbox is not enrolled in
	// Warmbly Cloud.
	ErrCodePoolLinkMailboxNotFound = "pool_link_mailbox_not_found"
	// ErrCodePoolLinkNotAdoptable is a 422: only active Google and Microsoft
	// mailboxes in the Cloud workspace can be linked to an instance.
	ErrCodePoolLinkNotAdoptable = "pool_link_not_adoptable"
	// ErrCodePoolLinkNotManaged is a 403: the mailbox's credential is held by
	// the instance, not by Warmbly Cloud, so Cloud cannot act on it.
	ErrCodePoolLinkNotManaged = "pool_link_not_managed"
	// ErrCodePoolLinkMailboxBlocked is a 403: warmbly Cloud blocked the mailbox
	// for hurting the pool. Sending from it is suspended until it is reviewed.
	ErrCodePoolLinkMailboxBlocked = "pool_link_mailbox_blocked"
	// ErrCodePoolLinkMailboxInactive is a 403: the mailbox is not active in
	// Warmbly Cloud. Reconnect it to keep sending.
	ErrCodePoolLinkMailboxInactive = "pool_link_mailbox_inactive"
	// ErrCodePoolLinkWarmupUnavailable is a 403: Warmup is not available on the
	// Cloud workspace.
	ErrCodePoolLinkWarmupUnavailable = "pool_link_warmup_unavailable"
	// ErrCodePoolLinkOAuthSession is a 404: the mailbox sign-in session is
	// unknown or has expired. Start the sign-in again.
	ErrCodePoolLinkOAuthSession = "pool_link_oauth_session"

	// CLI sign-in.

	// ErrCodeCLIAuthRequest is a 400: POST /auth/cli/poll was sent without a
	// device_code.
	ErrCodeCLIAuthRequest = "cli_auth_request"
	// ErrCodeCLIAuthScopes is a 400: the CLI asked for scopes that include
	// permission bits this instance does not grant.
	ErrCodeCLIAuthScopes = "cli_auth_scopes"
	// ErrCodeCLIAuthCodeNotFound is a 404: the code is unknown or has expired.
	// Run warmbly auth login again for a fresh one.
	ErrCodeCLIAuthCodeNotFound = "cli_auth_code_not_found"
	// ErrCodeCLIAuthCodeUsed is a 409: the code has already been used.
	ErrCodeCLIAuthCodeUsed = "cli_auth_code_used"
	// ErrCodeCLIAuthForbidden is a 403: approving a CLI sign-in needs the
	// manage_api_keys permission in the workspace, because the approval mints an
	// API key.
	ErrCodeCLIAuthForbidden = "cli_auth_forbidden"

	// Warmbly Cloud warmup refusals.

	// ErrCodeCloudWarmupValueUnsupported is a 422: warmbly Cloud does not take 0
	// from an instance for the starting volume, maximum, daily increase or reply
	// rate. Use 1 or more, or change it in Warmbly Cloud.
	ErrCodeCloudWarmupValueUnsupported = "cloud_warmup_value_unsupported"
	// ErrCodeCloudWarmupNotApplied is a 409: warmbly Cloud answered but did not
	// apply the named settings. The mailbox now shows the values Warmbly Cloud
	// reported.
	ErrCodeCloudWarmupNotApplied = "cloud_warmup_not_applied"
	// ErrCodeCloudWarmupManaged is a 409: Participation, the shared daily and
	// recipient limits, and stopping warmup are changed with the mailbox's
	// Warmbly Cloud controls, not on the mailbox.
	ErrCodeCloudWarmupManaged = "cloud_warmup_managed"
	// ErrCodeCloudLinkDisconnecting is a 409: the workspace is disconnecting
	// from Warmbly Cloud. Retry once it has finished.
	ErrCodeCloudLinkDisconnecting = "cloud_link_disconnecting"
	// ErrCodeCloudLinkUnreachable is a 503: warmbly Cloud could not be reached.
	ErrCodeCloudLinkUnreachable = "cloud_link_unreachable"
	// ErrCodeCloudLinkBadResponse is a 503: warmbly Cloud answered without the
	// mailbox's warmup settings.
	ErrCodeCloudLinkBadResponse = "cloud_link_bad_response"

	// Name refusals.

	// ErrCodeInvalidName is a 400: a first name, last name, workspace name,
	// company name, OAuth app name or mailbox display name broke the naming
	// rules. The message names the field and the rule.
	ErrCodeInvalidName = "invalid_name"

	// Redirect destination refusals.

	// ErrCodeRedirectTargetNotOwnDomain is a 400: a form's redirect_url, a
	// sending domain's redirect target_url, or a vendor forwarding url is not on
	// one of the workspace's own domains. Bulk setup answers it on the refused
	// domain's row.
	ErrCodeRedirectTargetNotOwnDomain = "redirect_target_not_own_domain"

	// Mailbox import refusals.

	// ErrCodeMailboxImportEmpty is a 400: No file and no pasted text, or an
	// input with no rows, or a header row and nothing under it.
	ErrCodeMailboxImportEmpty = "mailbox_import_empty"
	// ErrCodeMailboxImportTooLarge is a 400: more than 5,000 mailboxes, a file
	// over 10 MB, a pasted list over 2 MB, or a body that is not multipart form
	// data. Split the file and import the rest separately.
	ErrCodeMailboxImportTooLarge = "mailbox_import_too_large"
	// ErrCodeMailboxImportNoEmailColumn is a 400: POST /emails/imports with a
	// mapping in which no column is email.
	ErrCodeMailboxImportNoEmailColumn = "mailbox_import_no_email_column"
	// ErrCodeMailboxImportRowIncomplete is a 400: PATCH
	// /emails/imports/:id/rows/:line left the row without an SMTP and IMAP
	// server or without a password. The message says which.
	ErrCodeMailboxImportRowIncomplete = "mailbox_import_row_incomplete"
	// ErrCodeInvalidCursor is a 400: GET /emails/imports or GET
	// /emails/imports/:id/rows was given a cursor this API did not issue. Start
	// again without one.
	ErrCodeInvalidCursor = "invalid_cursor"
	// ErrCodeMailboxImportRowNotFailed is a 409: only a row that did not connect
	// can be fixed; this one is queued, running, connected, updated, skipped,
	// canceled or waiting for sign-in.
	ErrCodeMailboxImportRowNotFailed = "mailbox_import_row_not_failed"
	// ErrCodeMailboxImportCredentialsExpired is a 409: the row's credentials
	// were deleted 7 days after the import finished, so it cannot be retried.
	// Import it again.
	ErrCodeMailboxImportCredentialsExpired = "mailbox_import_credentials_expired"
	// ErrCodeMailboxImportNothingToRetry is a 409: POST
	// /emails/imports/:id/retry matched no failed row that still holds its
	// credentials.
	ErrCodeMailboxImportNothingToRetry = "mailbox_import_nothing_to_retry"

	// Mailbox source refusals.

	// ErrCodeMailboxVendorUnauthorized is a 400: the vendor did not accept the
	// API key. A saved connection is marked invalid until the key is updated.
	ErrCodeMailboxVendorUnauthorized = "mailbox_vendor_unauthorized"
	// ErrCodeMailboxVendorNoWorkspace is a 400: the key is accepted but the
	// vendor lists no workspace or organization for it, so there is nothing to
	// read. Create one at the vendor, then connect again.
	ErrCodeMailboxVendorNoWorkspace = "mailbox_vendor_no_workspace"
	// ErrCodeMailboxVendorInvalidFields is a 400: a field the vendor requires is
	// empty, or a field holds control characters. GET /emails/vendors/catalog
	// lists each vendor's fields.
	ErrCodeMailboxVendorInvalidFields = "mailbox_vendor_invalid_fields"
	// ErrCodeMailboxVendorUnknown is a 400: vendor is not one of the supported
	// vendors.
	ErrCodeMailboxVendorUnknown = "mailbox_vendor_unknown"
	// ErrCodeMailboxVendorRateLimited is a 429: the vendor is rate limiting
	// Warmbly's requests. Try again in a minute.
	ErrCodeMailboxVendorRateLimited = "mailbox_vendor_rate_limited"
	// ErrCodeMailboxVendorUnavailable is a 400: the vendor answered with
	// something unexpected, did not answer, or no longer has the mailbox. Try
	// again shortly.
	ErrCodeMailboxVendorUnavailable = "mailbox_vendor_unavailable"
	// ErrCodeMailboxGrantNotConfigured is a 400: this instance has no Google
	// service account or no Microsoft app for admin grants. An operator sets
	// them up; see configuration. GET /emails/grants/config names the settings
	// still unset in google_missing and microsoft_missing.
	ErrCodeMailboxGrantNotConfigured = "mailbox_grant_not_configured"
	// ErrCodeGoogleDelegationUnauthorized is a 400: google refused the domain-
	// wide delegation, or accepted it and refused the call: the client ID or a
	// scope is missing from the Admin console entry, an OAuth client ID was
	// entered instead of the service account's numeric one, or the admin address
	// is not a super administrator.
	ErrCodeGoogleDelegationUnauthorized = "google_delegation_unauthorized"
	// ErrCodeMicrosoftConsentMissing is a 400: the Microsoft 365 organization
	// has not granted admin consent, does not exist, or the consent lacks
	// Mail.ReadWrite, Mail.Send or User.Read.All; or the consent sign-in did not
	// complete or name the organization.
	ErrCodeMicrosoftConsentMissing = "microsoft_consent_missing"
	// ErrCodeMailboxGrantStateInvalid is a 400: the Google sign-in or Microsoft
	// consent state expired (15 minutes), was already used, or was started by
	// another member or workspace, or Google did not accept the sign-in. Start
	// again.
	ErrCodeMailboxGrantStateInvalid = "mailbox_grant_state_invalid"
	// ErrCodeMailboxGrantProofMissing is a 400: the workspace has not proved it
	// controls the Google Workspace domain: the _warmbly.<domain> TXT record
	// does not hold this workspace's warmbly-verify= value yet, or the Google
	// sign-in was not the admin address entered on that domain.
	ErrCodeMailboxGrantProofMissing = "mailbox_grant_proof_missing"
	// ErrCodeMailboxGrantUnavailable is a 503: Google, Microsoft or the DNS
	// lookup for the proof did not answer. Nothing was recorded or changed, and
	// a grant's status is left as it was. Try again in a minute.
	ErrCodeMailboxGrantUnavailable = "mailbox_grant_unavailable"
	// ErrCodeMailboxGrantDomainMismatch is a 400: the admin address is not on
	// the domain entered, or the mailbox is on a domain the grant does not
	// cover.
	ErrCodeMailboxGrantDomainMismatch = "mailbox_grant_domain_mismatch"
	// ErrCodeMailboxGrantMailboxUnreachable is a 400: google or Microsoft has no
	// mailbox for the account: it does not exist, is suspended or disabled, or
	// has no Exchange Online license.
	ErrCodeMailboxGrantMailboxUnreachable = "mailbox_grant_mailbox_unreachable"
	// ErrCodeMailboxGrantInactive is a 400: the grant failed its last check, so
	// it connects nothing. Check the grant (POST /emails/grants/:id/check), then
	// retry.
	ErrCodeMailboxGrantInactive = "mailbox_grant_inactive"
	// ErrCodeMailboxVendorDomainNotFound is a 404: PUT
	// /emails/domains/:domain/vendor-forwarding or POST
	// /emails/domains/:domain/vendor-tracking on a domain none of the
	// workspace's active vendor accounts holds.
	ErrCodeMailboxVendorDomainNotFound = "mailbox_vendor_domain_not_found"
	// ErrCodeMailboxVendorDomainUnsupported is a 400: the vendor holding the
	// domain cannot do this through its API: it has no forwarding removal,
	// writes no DNS records for Warmbly, or the existing record is one the
	// vendor manages itself. See domains held by an inbox vendor.
	ErrCodeMailboxVendorDomainUnsupported = "mailbox_vendor_domain_unsupported"
	// ErrCodeMailboxReauthDelegated is a 409: POST
	// /emails/onboarding/oauth/reauth/:id on a mailbox connected through an
	// admin grant. It has no sign-in of its own to renew; check its grant
	// instead, from **Add account > Google or Microsoft > Whole domain** or with
	// POST /emails/grants/:id/check.
	ErrCodeMailboxReauthDelegated = "mailbox_reauth_delegated"
	// ErrCodeSendingDomainNotInWorkspace is a 404: the workspace has no mailbox
	// on this domain. Also 400 when no domain was given.
	ErrCodeSendingDomainNotInWorkspace = "sending_domain_not_in_workspace"
	// ErrCodeSendingDomainSharedProvider is a 400: the domain belongs to a
	// shared email provider, such as gmail.com, and cannot be redirected.
	ErrCodeSendingDomainSharedProvider = "sending_domain_shared_provider"
	// ErrCodeDomainRedirectInvalidTarget is a 400: target_url (or a vendor
	// forwarding url) is not an http or https address on a host, carries
	// credentials, is longer than 2,048 characters, or points back at the same
	// domain or its www. An import's redirects option answers the same.
	ErrCodeDomainRedirectInvalidTarget = "domain_redirect_invalid_target"
	// ErrCodeDomainRedirectLinked is a 409: On Warmbly Cloud: this domain's
	// redirect is served for a linked self-hosted instance, and only that
	// instance changes it.
	ErrCodeDomainRedirectLinked = "domain_redirect_linked"
	// ErrCodeDomainRedirectCloudUnavailable is a 409: served_by is cloud, but
	// this instance is not linked to Warmbly Cloud, or Cloud does not serve
	// redirects for it.
	ErrCodeDomainRedirectCloudUnavailable = "domain_redirect_cloud_unavailable"
	// ErrCodeDomainRedirectCloudUnreachable is a 503: warmbly Cloud could not be
	// reached while saving, moving or removing a redirect it serves, so nothing
	// changed. Retry.
	ErrCodeDomainRedirectCloudUnreachable = "domain_redirect_cloud_unreachable"
	// ErrCodeDomainRedirectLimit is a 409: warmbly Cloud already serves 200
	// redirects for this linked instance.
	ErrCodeDomainRedirectLimit = "domain_redirect_limit"
	// ErrCodePoolLinkCleartextMailbox is a 422: enrolling a mailbox in Warmbly
	// Cloud, or updating its credential there: SMTP or IMAP uses security none.
	// Cloud reaches the mail server over the internet and needs TLS or STARTTLS
	// on both legs. Switch the mailbox to TLS, or keep warming it on the
	// instance.
	ErrCodePoolLinkCleartextMailbox = "pool_link_cleartext_mailbox"
	// ErrCodePoolLinkMailboxConflict is a 409: Enrollment found the address
	// already connected in the linked Cloud workspace and could not attach it.
	ErrCodePoolLinkMailboxConflict = "pool_link_mailbox_conflict"
	// ErrCodePoolLinkInstanceURL is a 422: starting a Google or Microsoft sign-
	// in through Warmbly Cloud from an instance that registered no address of
	// its own when it was linked, so Cloud has nowhere to return the sign-in to.
	// Set APP_URL on the instance, disconnect it and link it again.
	ErrCodePoolLinkInstanceURL = "pool_link_instance_url"
	// ErrCodePoolLinkReturnURL is a 400: the same sign-in named a return address
	// that is not on the instance's registered address (same scheme and host).
	ErrCodePoolLinkReturnURL = "pool_link_return_url"
	// ErrCodePoolLinkOAuthBrowser is a 400: the brokered sign-in was not
	// continued from the Warmbly Cloud page that started it, or returned to a
	// different browser. Nothing was connected. Start again from the instance.
	ErrCodePoolLinkOAuthBrowser = "pool_link_oauth_browser"
	// ErrCodeCloudLinkManagedProtocol is a 409: cloud does not advertise durable
	// managed-consent protocol 1. Upgrade all Cloud control-plane replicas
	// before opening a new managed connection or adoption. Existing mailboxes
	// are unchanged.
	ErrCodeCloudLinkManagedProtocol = "cloud_link_managed_protocol"
	// ErrCodeCloudLinkSMTPAssociationProtocol is a 409: cloud does not advertise
	// smtp_association_protocol: 1. Update all Cloud replicas before connecting
	// existing app-password copies. No local association intent or credentials
	// have changed.
	ErrCodeCloudLinkSMTPAssociationProtocol = "cloud_link_smtp_association_protocol"
	// ErrCodeCloudLinkSMTPAssociation is a 409: the explicitly selected existing
	// Cloud SMTP/IMAP mailbox could not be associated. Check the linked
	// workspace and whether an active Cloud campaign or another instance uses
	// that mailbox. Do not delete it or replace passwords to repair the
	// association.
	ErrCodeCloudLinkSMTPAssociation = "cloud_link_smtp_association"
	// ErrCodePoolLinkOAuthUnknown is a 409: the provider exchange has no
	// confirmed committed mailbox. Do not retry the authorization code or infer
	// success; remove the pending connection before starting new explicit
	// consent.
	ErrCodePoolLinkOAuthUnknown = "pool_link_oauth_unknown"
	// ErrCodeCLIAuthScopesRole is a 403: approving a CLI sign-in: your role in
	// the chosen workspace allows none of the scopes the CLI asked for. Pick
	// another workspace, or ask for narrower scopes with --scopes.
	ErrCodeCLIAuthScopesRole = "cli_auth_scopes_role"
	// ErrCodeSendingDomainBulkInvalid is a 400: a bulk setup named no domain,
	// more than 100, neither a tracking_label nor a redirect_url, a
	// tracking_label that is not one DNS label, or a served_by other than
	// instance or cloud.
	ErrCodeSendingDomainBulkInvalid = "sending_domain_bulk_invalid"
	// ErrCodeFormsDomainTaken is a 409: another workspace on this instance has
	// already verified this domain, as its forms domain, a tracking domain or a
	// redirect. Returned when saving or verifying the workspace's custom forms
	// domain. Nothing changes.
	ErrCodeFormsDomainTaken = "forms_domain_taken"
	// ErrCodeLeadSyncTargetNotFound is a 400: creating or editing a lead sync
	// source named a target campaign or a category that does not exist in this
	// workspace. Nothing is saved.
	ErrCodeLeadSyncTargetNotFound = "lead_sync_target_not_found"
	// ErrCodeTrackingDomainTaken is a 409: another workspace on this instance
	// has already verified this tracking domain, on a mailbox, as a campaign's
	// override, or as its forms domain.
	ErrCodeTrackingDomainTaken = "tracking_domain_taken"
	// ErrCodeTrackingHostNotConfigured is a 400: this instance has no tracking
	// host (TRACKING_DOMAIN), so it cannot serve a tracking domain or a
	// redirect.
	ErrCodeTrackingHostNotConfigured = "tracking_host_not_configured"
	// ErrCodeMailboxGmailOAuthDisabled is a 403: New Gmail mailboxes connect
	// with an app password on this deployment, not Google sign-in. Existing
	// sign-in mailboxes keep working.
	ErrCodeMailboxGmailOAuthDisabled = "mailbox_gmail_oauth_disabled"
	// ErrCodeMailboxIdentityUnavailable is a 503: the machine running the
	// mailbox could not be reached, so its sending addresses were not refreshed.
	// Nothing changed; retry.
	ErrCodeMailboxIdentityUnavailable = "mailbox_identity_unavailable"
	// ErrCodeMailboxOAuthReturnOrigin is a 400: the dashboard origin is not
	// allowed for mailbox sign-in. Add its exact origin to the instance CORS
	// allow-list.
	ErrCodeMailboxOAuthReturnOrigin = "mailbox_oauth_return_origin"
	// ErrCodeMailboxSendAsUnknown is a 400: the address is not one the provider
	// has verified the mailbox to send as. Refresh the list, or verify the
	// address at the provider first.
	ErrCodeMailboxSendAsUnknown = "mailbox_send_as_unknown"
	// ErrCodeMailboxSendAsUnsupported is a 400: the mailbox's provider does not
	// expose send-as addresses. Only Gmail and Google Workspace mailboxes do.
	ErrCodeMailboxSendAsUnsupported = "mailbox_send_as_unsupported"
	// ErrCodeMailboxSignatureTooLarge is a 400: the signature is larger than
	// Warmbly stores. Shorten it at the provider and import it again.
	ErrCodeMailboxSignatureTooLarge = "mailbox_signature_too_large"
	// ErrCodeMailboxTLSFailed is a 400: the server did not present the
	// certificate whose fingerprint was confirmed, so nothing changed. Run the
	// certificate check again.
	ErrCodeMailboxTLSFailed = "mailbox_tls_failed"
	// ErrCodeMailboxValidationTimeout is a 400: No worker reported back on the
	// mailbox in time, so the mail server was not tested and nothing was saved.
	// Retry.
	ErrCodeMailboxValidationTimeout = "mailbox_validation_timeout"

	// App password switch refusals.

	// ErrCodeAppPasswordInvalid is a 400: app_password is not 16 letters once
	// spaces are removed. Create one at myaccount.google.com/apppasswords and
	// send it as Google shows it.
	ErrCodeAppPasswordInvalid = "app_password_invalid"
	// ErrCodeMailboxNotGoogleSignin is a 409: the mailbox is not connected with
	// per-mailbox Google sign-in: it is already on an app password or an admin
	// grant, is an Outlook or SMTP/IMAP mailbox, or its sign-in is held by
	// Warmbly Cloud. A repeat of a switch that succeeded answers this.
	ErrCodeMailboxNotGoogleSignin = "mailbox_not_google_signin"

	// Placement test refusals.

	// ErrCodePlacementNotEntitled is a 402: the workspace has no active trial or
	// subscription, so it cannot start a test. Only the hosted product answers
	// this.
	ErrCodePlacementNotEntitled = "placement_not_entitled"
	// ErrCodePlacementQuotaExceeded is a 402: the workspace has used its free
	// tests for the month on the instance panel, or the linked Warmbly Cloud
	// workspace has used its free tests on Warmbly Cloud's panel. A tracking
	// comparison needs two.
	ErrCodePlacementQuotaExceeded = "placement_quota_exceeded"
	// ErrCodePlacementTooManyRunning is a 429: the workspace already has 3 tests
	// running, counting each half of a tracking comparison. Wait for one to
	// finish.
	ErrCodePlacementTooManyRunning = "placement_too_many_running"
	// ErrCodePlacementSenderBusy is a 409: the sending mailbox still has copies
	// of another test waiting to be sent. One test sends from a mailbox at a
	// time.
	ErrCodePlacementSenderBusy = "placement_sender_busy"
	// ErrCodePlacementSenderUnavailable is a 409: the sending mailbox is not
	// connected and active, or it is itself a seed inbox, which receives tests
	// and cannot send one.
	ErrCodePlacementSenderUnavailable = "placement_sender_unavailable"
	// ErrCodePlacementDailyBudget is a 409: the mailbox has too little of its
	// daily limit left today for a useful test (at least five seeds per test,
	// ten for a comparison), or for every seed chosen with seed_ids. A test that
	// fits otherwise is sized down to what is left rather than refused.
	ErrCodePlacementDailyBudget = "placement_daily_budget"
	// ErrCodePlacementNoSeeds is a 409: the panel has no seed inbox the test
	// could use: none were added, none is connected, or every one is on the
	// sender's own domain, which is always skipped.
	ErrCodePlacementNoSeeds = "placement_no_seeds"
	// ErrCodePlacementPanelUnavailable is a 409: panel is cloud on an instance
	// that is not linked to Warmbly Cloud.
	ErrCodePlacementPanelUnavailable = "placement_panel_unavailable"
	// ErrCodePlacementInvalidSeeds is a 400: seed_ids names a mailbox that is
	// not a seed inbox of the workspace, or one that is not connected and
	// running right now; the message names it.
	ErrCodePlacementInvalidSeeds = "placement_invalid_seeds"
	// ErrCodePlacementInvalidTracking is a 400: tracking is on or compare for a
	// campaign that sends plain text, which carries no tracking.
	ErrCodePlacementInvalidTracking = "placement_invalid_tracking"
	// ErrCodePlacementNotRunning is a 409: POST /placement/tests/:id/cancel on a
	// test that has already finished or been canceled.
	ErrCodePlacementNotRunning = "placement_not_running"
	// ErrCodePlacementSeedUnavailable is a 409: the mailbox cannot be marked as
	// a seed inbox: it is on the instance panel, or a placement test is still
	// sending from it.
	ErrCodePlacementSeedUnavailable = "placement_seed_unavailable"
	// ErrCodePlacementSeedLimit is a 409: the workspace already has 50 seed
	// inboxes. Unmark one first.
	ErrCodePlacementSeedLimit = "placement_seed_limit"
	// ErrCodeMailboxIsSeed is a 409: Warmup was started or resumed on a seed
	// inbox. A seed never warms up; unmark it first with PUT
	// /placement/seeds/:email_account_id.
	ErrCodeMailboxIsSeed = "mailbox_is_seed"
	// ErrCodePlacementBatchEmpty is a 400: POST /placement/batches resolved to
	// no sending mailbox: the scope, its filters and the sample left nothing.
	ErrCodePlacementBatchEmpty = "placement_batch_empty"
	// ErrCodePlacementBatchTooLarge is a 400: the batch would hold more
	// mailboxes than the instance allows in one batch (10,000 by default, an
	// operator setting). The message gives both numbers; narrow the selection or
	// take a sample.
	ErrCodePlacementBatchTooLarge = "placement_batch_too_large"
	// ErrCodePlacementTooManyBatches is a 429: the workspace already has 5
	// batches running. Wait for one to finish, or cancel one.
	ErrCodePlacementTooManyBatches = "placement_too_many_batches"
	// ErrCodePlacementBatchNotRunning is a 409: POST
	// /placement/batches/:id/cancel on a batch that has already finished or been
	// canceled.
	ErrCodePlacementBatchNotRunning = "placement_batch_not_running"

	// OAuth app and community directory refusals.

	// ErrCodeInvalidListing is a 400: a listing field breaks a rule: the slug is
	// not 3 to 48 lowercase letters, numbers or single dashes, or is reserved
	// for a built-in integration; the tagline is empty or over 120 characters;
	// the description is over 2,000; either holds control, invisible or
	// bidirectional-override characters; the category is unknown; or
	// install_url, support_url or privacy_url is not an https address on a host
	// without credentials.
	ErrCodeInvalidListing = "invalid_listing"
	// ErrCodeAppNotListable is a 400: the app cannot be listed: it is disabled
	// or suspended, or it is a client created by dynamic registration, which has
	// no publishing workspace.
	ErrCodeAppNotListable = "app_not_listable"
	// ErrCodeDeveloperAccessBlocked is a 403: the instance's operators have
	// blocked this workspace or person from registering and publishing apps. GET
	// /oauth/applications reports the reason in developer_access. Also returned
	// by POST /oauth/applications.
	ErrCodeDeveloperAccessBlocked = "developer_access_blocked"
	// ErrCodeInvalidWebsite is a 400: website on POST or PATCH
	// /oauth/applications is not an http or https address on a host without
	// credentials.
	ErrCodeInvalidWebsite = "invalid_website"
	// ErrCodeInvalidLogo is a 400: logo_url on POST or PATCH /oauth/applications
	// is not an address this instance issued for the workspace. Upload the logo
	// with POST /oauth/applications/{id}/logo instead.
	ErrCodeInvalidLogo = "invalid_logo"
	// ErrCodeListingSlugTaken is a 409: another app on this instance already
	// uses the slug. Choose another.
	ErrCodeListingSlugTaken = "listing_slug_taken"
	// ErrCodeListingHidden is a 409: the instance's operators hid this listing,
	// so DELETE /oauth/applications/{id}/listing cannot remove it. The hide note
	// is on the listing.
	ErrCodeListingHidden = "listing_hidden"
	// ErrCodeAppSuspended is a 409: the instance's operators suspended this app.
	// It cannot be edited or given a new logo until it is unsuspended.
	ErrCodeAppSuspended = "app_suspended"

	// Credential management refusals.

	// ErrCodeOAuthTokenNotAllowed is a 403: an OAuth app token called a route
	// that manages credentials: creating, editing, revoking or deleting API
	// keys, or anything under /oauth/applications and /oauth/application-logo.
	// Use the dashboard or an API key.
	ErrCodeOAuthTokenNotAllowed = "oauth_token_not_allowed"
	// ErrCodeAPIKeyPermissionsExceedCaller is a 403: POST /api-keys or PATCH
	// /api-keys/{id} would give a key more than its creator has: a permission
	// the member's role does not cover, or, for an API key caller, a permission,
	// mailbox or IP address the calling key does not have (a mailbox- or IP-
	// limited key can only manage keys limited to a subset of its own).
	ErrCodeAPIKeyPermissionsExceedCaller = "api_key_permissions_exceed_caller"
	// ErrCodeAPIKeyHolderLeft is a 401: the member who created this API key is
	// no longer in the workspace, so the key no longer works. Create a new key
	// from a current member.
	ErrCodeAPIKeyHolderLeft = "api_key_holder_left"
	// ErrCodeAPIKeyMailboxLimited is a 403: an API key limited to some mailboxes
	// called a surface that acts across the workspace: /ai/tools or /mcp, a
	// campaign sender pool resolved from mailbox tags, or starting a campaign
	// whose pool is not an explicit list of the key's mailboxes.
	ErrCodeAPIKeyMailboxLimited = "api_key_mailbox_limited"

	// CRM provider refusals.

	// ErrCodeCRMNotConnected is a 409: GET /crm/metadata, POST /crm/sync, POST
	// /crm/backfill, a /crm/contacts/:id or a /crm/lists call on a workspace
	// that is not in HubSpot or Pipedrive mode. Connect the provider and choose
	// it as the CRM with PUT /crm/settings first.
	ErrCodeCRMNotConnected = "crm_not_connected"
	// ErrCodeCRMReauthRequired is a 409: the provider revoked Warmbly's access,
	// the connection was removed, or the connection lacks a permission provider
	// mode needs (in HubSpot deals, companies, owners, the contact schema, or
	// the optional list permission for a list import; in Pipedrive deals,
	// people, activities and users, or the connected user may not make the
	// change).
	ErrCodeCRMReauthRequired = "crm_reauth_required"
	// ErrCodeCRMManagedExternally is a 409: a create, update or delete of a
	// pipeline, stage or task type. They are managed in HubSpot or Pipedrive
	// while it is the CRM; change them there and they update in Warmbly within
	// minutes.
	ErrCodeCRMManagedExternally = "crm_managed_externally"
	// ErrCodeCRMStageUnknown is a 400: the pipeline_id or stage_id given is not
	// a mirrored pipeline or stage of the provider, or (in HubSpot) a deal was
	// marked won or lost in a pipeline with no closed stage of that kind.
	ErrCodeCRMStageUnknown = "crm_stage_unknown"
	// ErrCodeCRMOwnerUnmapped is a 400: a deal or task was assigned to a member
	// who is not matched to a HubSpot owner or Pipedrive user. Match them with
	// PUT /crm/owners/:externalId.
	ErrCodeCRMOwnerUnmapped = "crm_owner_unmapped"
	// ErrCodeCRMContactMissing is a 404: the contact is not in the provider, and
	// creating contacts there is turned off in the workspace's CRM settings.
	ErrCodeCRMContactMissing = "crm_contact_missing"
	// ErrCodeCRMRecordMissing is a 404: the record no longer exists in the
	// provider; it was deleted there.
	ErrCodeCRMRecordMissing = "crm_record_missing"
	// ErrCodeCRMProviderRejected is a 422: the provider refused the change. The
	// message carries its reason, such as a required property HubSpot enforces
	// or a Pipedrive plan without the feature.
	ErrCodeCRMProviderRejected = "crm_provider_rejected"
	// ErrCodeCRMSyncRunning is a 429: POST /crm/sync within 30 seconds (HubSpot)
	// or a minute (Pipedrive) of the last one. The running sync finishes in a
	// minute or two.
	ErrCodeCRMSyncRunning = "crm_sync_running"
	// ErrCodeCRMUnavailable is a 503: the provider did not answer or failed,
	// including a Pipedrive company out of its daily API budget. Nothing was
	// changed; retry shortly.
	ErrCodeCRMUnavailable = "crm_unavailable"
	// ErrCodeCRMOptedOut is a 409: the contact opted out of email in the
	// connected CRM, so the write was refused.
	ErrCodeCRMOptedOut = "crm_opted_out"

	// Salesforce sync refusals.

	// ErrCodeInvalidSalesforceDomain is a 400: the custom domain given to POST
	// /integrations/oauth/start is not a Salesforce My Domain
	// (*.my.salesforce.com).
	ErrCodeInvalidSalesforceDomain = "invalid_salesforce_domain"
	// ErrCodeInvalidSalesforceSettings is a 400: PUT
	// /integrations/salesforce/:id/settings named something the sync cannot run:
	// an unknown field, a related field to write, an engagement field to read, a
	// field mapped twice, or a fixed owner with no user.
	ErrCodeInvalidSalesforceSettings = "invalid_salesforce_settings"
	// ErrCodeSalesforceError is a 400: salesforce refused the request. The
	// message is Salesforce's own.
	ErrCodeSalesforceError = "salesforce_error"
	// ErrCodeSalesforceUnreachable is a 400: salesforce did not answer. Retry
	// later.
	ErrCodeSalesforceUnreachable = "salesforce_unreachable"
	// ErrCodeSalesforceSyncOff is a 400: POST /integrations/salesforce/:id/sync-
	// now on a connection with sync turned off.
	ErrCodeSalesforceSyncOff = "salesforce_sync_off"
	// ErrCodeSalesforceReconnectRequired is a 409: the Salesforce session ended
	// (a revoked app, a password reset, a deactivated user). Reconnect the
	// integration.
	ErrCodeSalesforceReconnectRequired = "salesforce_reconnect_required"
	// ErrCodeImportRunning is a 409: POST /integrations/salesforce/:id/import-
	// sources/:sourceId/run or POST /integrations/apollo/:id/import-
	// sources/:sourceId/run while that import is already running.
	ErrCodeImportRunning = "import_running"
	// ErrCodeSalesforceRateLimited is a 429: the Salesforce org has used its API
	// requests for today.
	ErrCodeSalesforceRateLimited = "salesforce_rate_limited"

	// Apollo and Clay refusals.

	// ErrCodeApolloEnrichTooMany is a 400: POST
	// /integrations/apollo/:id/enrichments selected more than 5,000 contacts.
	ErrCodeApolloEnrichTooMany = "apollo_enrich_too_many"
	// ErrCodeClayWebhookURL is a 400: PATCH /integrations/clay/:id set a
	// webhook_url that is not a Clay table webhook.
	ErrCodeClayWebhookURL = "clay_webhook_url"
	// ErrCodeClayWebhookMissing is a 400: PATCH /integrations/clay/:id chose
	// events to send with no table webhook saved.
	ErrCodeClayWebhookMissing = "clay_webhook_missing"
	// ErrCodeApolloKeyRejected is a 422: apollo refused the saved API key.
	// Replace it with PUT /integrations/apollo/:id/key.
	ErrCodeApolloKeyRejected = "apollo_key_rejected"
	// ErrCodeApolloKeyForbidden is a 422: the API key works but cannot reach an
	// endpoint the integration uses. Use a master key, or one with lists,
	// contact search and people enrichment.
	ErrCodeApolloKeyForbidden = "apollo_key_forbidden"
	// ErrCodeApolloKeyMissing is a 422: the Apollo connection has no API key.
	ErrCodeApolloKeyMissing = "apollo_key_missing"
	// ErrCodeApolloRateLimited is a 429: Apollo's rate limit for the account was
	// reached. Retry later.
	ErrCodeApolloRateLimited = "apollo_rate_limited"

	// Automation refusals.

	// ErrCodeActionProviderMismatch is a 400: POST /automations, PATCH
	// /automations/:id or POST /integrations/connections/:id/events paired an
	// action with a connection whose provider does not run it, such as a Slack
	// message on a HubSpot connection.
	ErrCodeActionProviderMismatch = "action_provider_mismatch"

	// Registration and invitation refusals.

	// ErrCodeInvitationExpired is a 400: POST /invitations/accept on an
	// invitation older than 48 hours. It stays on the team page, where it can be
	// sent again.
	ErrCodeInvitationExpired = "invitation_expired"
	// ErrCodeInvitationUsedOrExpired is a 400: POST /invitations/accept on an
	// invitation that was accepted or expired while the request was in flight.
	// Ask for it to be sent again.
	ErrCodeInvitationUsedOrExpired = "invitation_used_or_expired"
	// ErrCodeInvitationRolesRemoved is a 400: POST /invitations/accept on an
	// invitation whose roles were all deleted since it was sent. Ask for a new
	// invitation.
	ErrCodeInvitationRolesRemoved = "invitation_roles_removed"

	// Account and workspace standing.

	// ErrCodeAccountSuspended is a 403: the account is suspended and cannot sign
	// in, by password, passkey or two-factor confirmation.
	ErrCodeAccountSuspended = "account_suspended"
	// ErrCodeAccountSendingSuspended is a 403: the account may not send mail. A
	// manual send (POST /emails/:id/send, POST /unibox/*) is refused; nothing
	// else is affected.
	ErrCodeAccountSendingSuspended = "account_sending_suspended"
	// ErrCodeAccountWorkspaceCreateSuspended is a 403: the account may not
	// create workspaces. It can still join one it is invited to.
	ErrCodeAccountWorkspaceCreateSuspended = "account_workspace_create_suspended"
	// ErrCodeWorkspaceSendingSuspended is a 403: sending is paused for the whole
	// workspace while it is under review. Contact support to resolve the review.
	ErrCodeWorkspaceSendingSuspended = "workspace_sending_suspended"
	// ErrCodeSessionEnded is a 401: the session was signed out or reached its
	// absolute lifetime. Only a new sign-in continues.
	ErrCodeSessionEnded = "session_ended"

	// Confirmation required.

	// ErrCodeReauthLimited is a 400: Too many failed confirmations for this
	// account within the hour. Wait, or sign out and sign in again: a sign-in
	// counts as a confirmation for five minutes.
	ErrCodeReauthLimited = "reauth_limited"
	// ErrCodeReauthNoFactor is a 400: POST /auth/reauth was sent a password or
	// code for an account that has neither.
	ErrCodeReauthNoFactor = "reauth_no_factor"
	// ErrCodeReauthIdentityMismatch is a 400: the Google, Apple or single sign-
	// on account verified during a confirmation is not the one linked to this
	// Warmbly account. Continue with the account you sign in with.
	ErrCodeReauthIdentityMismatch = "reauth_identity_mismatch"
	// ErrCodeReauthExpired is a 400: the confirmation's code or nonce expired or
	// was already used. Start the confirmation again.
	ErrCodeReauthExpired = "reauth_expired"
	// ErrCodePasskeyUserVerificationRequired is a 400: a passkey sign-in came
	// from an authenticator that did not verify the user with a PIN or
	// biometric. Set a PIN on the security key, or sign in another way.
	ErrCodePasskeyUserVerificationRequired = "passkey_user_verification_required"
	// ErrCodeTwoFAInvalidCode is a 400: the authenticator or recovery code did
	// not match. Wait for a fresh code and try again; the login challenge allows
	// five attempts per pending session.
	ErrCodeTwoFAInvalidCode = "two_fa_invalid_code"
	// ErrCodePasswordChangedSignInAgain is a 409: POST /auth/me/password stored
	// the new password but could not issue the calling device a new session.
	// Every earlier token is invalid; discard them and sign in again with the
	// new password.
	ErrCodePasswordChangedSignInAgain = "password_changed_sign_in_again"

	// Member access scope.

	// ErrCodeMemberAccessRestricted is a 403: the caller's selected-resource
	// scope does not cover this route. Resource writes and workspace-wide
	// surfaces such as contacts and settings are refused.
	ErrCodeMemberAccessRestricted = "member_access_restricted"
	// ErrCodeInvalidAccessResource is a 400: PUT
	// /organization/members/:id/access or an invitation named a campaign folder,
	// campaign or mailbox that is not in this workspace.
	ErrCodeInvalidAccessResource = "invalid_access_resource"
	// ErrCodeInvalidAccessScope is a 400: the scope is neither workspace nor
	// restricted.
	ErrCodeInvalidAccessScope = "invalid_access_scope"
	// ErrCodeTooManyAccessGrants is a 400: more than 500 folders, campaigns or
	// mailboxes in one list.
	ErrCodeTooManyAccessGrants = "too_many_access_grants"
	// ErrCodeOwnerAccessUnrestricted is a 403: the workspace owner always
	// reaches the entire workspace and cannot be restricted.
	ErrCodeOwnerAccessUnrestricted = "owner_access_unrestricted"

	// Lookups.

	// ErrCodeUnknownView is a 404: /me/views/:view was given a view name other
	// than contacts, campaign_leads or unibox_rail.
	ErrCodeUnknownView = "unknown_view"
	// ErrCodeLeadCcContactNotFound is a 404: Set a lead's CC named a contact
	// that is not in the workspace.
	ErrCodeLeadCcContactNotFound = "lead_cc_contact_not_found"
	// ErrCodeSlackNotConnected is a 404: a Slack route was called for a
	// workspace that has not connected Slack, or whose connection was removed.
	// Connect it under **Integrations > Slack**.
	ErrCodeSlackNotConnected = "slack_not_connected"
	// ErrCodeSlackLinkInvalid is a 404: the Slack link code is unknown, expired
	// or already used. Mention or message the bot in Slack for a new link
	// button.
	ErrCodeSlackLinkInvalid = "slack_link_invalid"
	// ErrCodeSlackVerifyFailed is a 404: POST /integrations/slack/link with a
	// Sign in with Slack result that is unknown, expired, already used, started
	// by someone else or for another code. Start again from POST
	// /integrations/slack/link/verify.
	ErrCodeSlackVerifyFailed = "slack_verify_failed"
	// ErrCodeSlackVerifyWrongAccount is a 404: POST /integrations/slack/link
	// after Sign in with Slack signed in a different Slack account than the one
	// the link code names.
	ErrCodeSlackVerifyWrongAccount = "slack_verify_wrong_account"
	// ErrCodeSlackVerifyUnavailable is a 404: POST
	// /integrations/slack/link/verify on an instance whose Slack app has no
	// client credentials.
	ErrCodeSlackVerifyUnavailable = "slack_verify_unavailable"
	// ErrCodeSlackNotLinked is a 404: PATCH /integrations/slack/link from a
	// member who has not linked a Slack account in this workspace.
	ErrCodeSlackNotLinked = "slack_not_linked"

	// Conflicts.

	// ErrCodeContactEmailTaken is a 409: the address given to update a contact
	// already belongs to another contact.
	ErrCodeContactEmailTaken = "contact_email_taken"
	// ErrCodeLeadCcLeadIsCopied is a 409: Set a lead's CC on a lead that is
	// itself copied on another lead in the campaign, so it sends nothing of its
	// own to copy anyone on.
	ErrCodeLeadCcLeadIsCopied = "lead_cc_lead_is_copied"
	// ErrCodeLeadCcHasCopies is a 409: Set a lead's CC names a contact that has
	// copies of their own in the campaign.
	ErrCodeLeadCcHasCopies = "lead_cc_has_copies"
	// ErrCodeApprovalNotPending is a 409: POST /ai/sessions/:id/approve named a
	// tool call the conversation is not waiting on: it was already decided, in
	// another tab or in Slack, or a newer message replaced it.
	ErrCodeApprovalNotPending = "approval_not_pending"
	// ErrCodeMailboxCloudUnenrollFailed is a 409: the mailbox is linked to
	// Warmbly Cloud and its link could not be released, so DELETE /emails/{id}
	// would leave Warmbly Cloud holding its credential or its claim on it. The
	// mailbox record remains, and restoration onto its worker is attempted.
	ErrCodeMailboxCloudUnenrollFailed = "mailbox_cloud_unenroll_failed"

	// AI.

	// ErrCodeAINotConfigured is a 503: the deployment has no AI provider
	// configured, as opposed to a provider outage. Nothing is wrong with the
	// request.
	ErrCodeAINotConfigured = "ai_not_configured"

	// General.

	// ErrCodeAttemptCheckUnavailable is a 503: the attempt budget could not be
	// read, so the attempt was not counted or evaluated. Wait a few seconds and
	// retry.
	ErrCodeAttemptCheckUnavailable = "attempt_check_unavailable"

	// Billing.

	// ErrCodePlanOfferRequired is a 403: a private plan was requested by a
	// workspace without an active offer for it.
	ErrCodePlanOfferRequired = "plan_offer_required"

	// Slack.

	// ErrCodeSlackLinkEmailMismatch is a 403: the Slack account's email is not
	// the one the member signs in to Warmbly with. Confirm the account with Sign
	// in with Slack.
	ErrCodeSlackLinkEmailMismatch = "slack_link_email_mismatch"
	// ErrCodeSlackNotConfigured is a 503: Slack is not set up on this instance.
	ErrCodeSlackNotConfigured = "slack_not_configured"
)

// Codes the SDK's own documentation refers to, and a few the generic tables
// above do not cover.
const (
	// ErrCodeReauthRequired is a 401 or 403 from the routes that change
	// credentials or secrets (creating a webhook, rotating its secret) when
	// called with a dashboard session that has not recently confirmed the
	// account holder. API keys and OAuth tokens are never asked to confirm, so
	// an SDK client does not meet it.
	ErrCodeReauthRequired = "reauth_required"
	// ErrCodeTooManyContacts is a 400: a contact bulk action was given more
	// than 10,000 contact ids in one request, or more than 250,000 exclusions.
	// Split it into batches, or send a filter selection instead of ids.
	ErrCodeTooManyContacts = "too_many_contacts"
	// ErrCodeDomainRedirectTaken is a 409: another workspace on this instance
	// already serves a verified redirect for the domain, or already has Warmbly
	// Cloud serve it. The redirect is saved but does not verify.
	ErrCodeDomainRedirectTaken = "domain_redirect_taken"

	// Connecting a self-hosted instance to Warmbly Cloud and enrolling its
	// mailboxes in the shared warmup pool.

	// ErrCodeCloudLinkConnected is a 409: the workspace is already connected to
	// Warmbly Cloud. Disconnect first to link a different Cloud workspace.
	ErrCodeCloudLinkConnected = "cloud_link_connected"
	// ErrCodeCloudLinkNotConnected is a 409: the workspace is not connected to
	// Warmbly Cloud.
	ErrCodeCloudLinkNotConnected = "cloud_link_not_connected"
	// ErrCodeCloudLinkNoPending is a 404: no connection is in progress. Start
	// again.
	ErrCodeCloudLinkNoPending = "cloud_link_no_pending"
	// ErrCodeCloudLinkCodeExpired is a 404: the connection code expired before
	// it was approved. Start again.
	ErrCodeCloudLinkCodeExpired = "cloud_link_code_expired"
	// ErrCodeCloudLinkOAuthSession is a 404: the mailbox sign-in session is
	// unknown or has expired. Start the sign-in again.
	ErrCodeCloudLinkOAuthSession = "cloud_link_oauth_session"
	// ErrCodeCloudLinkOAuthMailbox is a 422: a mailbox that signs in with the
	// instance's own Google or Microsoft OAuth app cannot be warmed by Warmbly
	// Cloud, because its refresh grant is bound to that app. Connect it over
	// SMTP and IMAP with an app password to enroll it.
	ErrCodeCloudLinkOAuthMailbox = "cloud_link_oauth_mailbox"
	// ErrCodeCloudLinkMailboxInactive is a 422: only active mailboxes can be
	// enrolled.
	ErrCodeCloudLinkMailboxInactive = "cloud_link_mailbox_inactive"
	// ErrCodePoolLinkCodeNotFound is a 404: the connection code is unknown or
	// has expired. Start the connection again from the instance.
	ErrCodePoolLinkCodeNotFound = "pool_link_code_not_found"
	// ErrCodePoolLinkCodeUsed is a 409: the connection code has already been
	// used or is no longer pending.
	ErrCodePoolLinkCodeUsed = "pool_link_code_used"
	// ErrCodePoolLinkDenied is returned when the member declined the
	// connection. The code cannot be approved afterwards.
	ErrCodePoolLinkDenied = "pool_link_denied"
	// ErrCodePoolLinkOAuthPending is a 409: the mailbox sign-in has not
	// completed yet. Poll again.
	ErrCodePoolLinkOAuthPending = "pool_link_oauth_pending"
	// ErrCodePoolLinkAlreadyAdopted is a 409: the mailbox is already linked to
	// an instance.
	ErrCodePoolLinkAlreadyAdopted = "pool_link_already_adopted"
	// ErrCodePoolLinkMailboxLimit is a 402: the workspace has reached the free
	// allowance of linked mailboxes. Upgrade to the pool plan for unlimited
	// mailboxes.
	ErrCodePoolLinkMailboxLimit = "pool_link_mailbox_limit"
	// ErrCodePoolLinkRevoked is returned when the instance's link has been
	// revoked. The instance's local link state is cleared on its next call.
	ErrCodePoolLinkRevoked = "pool_link_revoked"
)

// Sentinel errors for matching API failures with errors.Is. They carry only a
// status code; the concrete error returned from a call carries the full body.
var (
	// ErrBadRequest is returned for HTTP 400 responses.
	ErrBadRequest = &Error{StatusCode: http.StatusBadRequest}
	// ErrUnauthorized is returned for HTTP 401 responses (missing or invalid
	// credentials).
	ErrUnauthorized = &Error{StatusCode: http.StatusUnauthorized}
	// ErrForbidden is returned for HTTP 403 responses (authenticated but not
	// permitted).
	ErrForbidden = &Error{StatusCode: http.StatusForbidden}
	// ErrNotFound is returned for HTTP 404 responses.
	ErrNotFound = &Error{StatusCode: http.StatusNotFound}
	// ErrConflict is returned for HTTP 409 responses.
	ErrConflict = &Error{StatusCode: http.StatusConflict}
	// ErrUnprocessable is returned for HTTP 422 responses (validation failed).
	ErrUnprocessable = &Error{StatusCode: http.StatusUnprocessableEntity}
	// ErrRateLimited is returned for HTTP 429 responses.
	ErrRateLimited = &Error{StatusCode: http.StatusTooManyRequests}
	// ErrServer matches any HTTP 5xx response.
	ErrServer = &Error{StatusCode: http.StatusInternalServerError}
)
