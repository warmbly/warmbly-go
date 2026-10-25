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
// status. "reason" is present on a few codes only, as a finer key.
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
	// Reason is an optional finer key on a few codes, for when one code has
	// more than one next step (for example "certificate_untrusted" on a
	// mailbox TLS failure). Treat an unknown or empty Reason as the plain
	// Code.
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
// service they belong to instead of here — [ErrCodeMailboxAllowanceReached],
// [ErrCodeMailboxWorkerUnreachable], [ErrCodeListBounceRisk] and
// [ErrCodeLeadsUndeliverable].
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
	// ErrCodeReauthRequired is the 403 (401 when the credential is not a
	// signed-in session) from a sensitive action whose session has no recent
	// confirmation. Confirm through [AuthService.Reauth] or another
	// re-authentication flow, then repeat the call. API keys and OAuth app
	// tokens are never asked.
	ErrCodeReauthRequired = "reauth_required"
	// ErrCodeReauthNoFactor is the 400 from [AuthService.Reauth] for an account
	// with no password and no authenticator; use a passkey or linked identity.
	ErrCodeReauthNoFactor = "reauth_no_factor"
	// ErrCodeReauthLimited is the 400 once the hourly confirmation attempts are
	// spent.
	ErrCodeReauthLimited = "reauth_limited"
	// ErrCodeReauthIdentityMismatch and ErrCodeReauthExpired are the 400s from
	// [AuthService.FinishSSOReauth] and [AuthService.ReauthNative].
	ErrCodeReauthIdentityMismatch = "reauth_identity_mismatch"
	ErrCodeReauthExpired          = "reauth_expired"
	// ErrCodeSSOLinkExpired is the 400 from [AuthService.LinkSSO] for an
	// expired challenge or too many wrong passwords.
	ErrCodeSSOLinkExpired = "sso_link_expired"
	// ErrCodeResetLinkInvalid is the 401 from [AuthService.ResetPasswordSession]
	// for an unknown, used or expired emailed token.
	ErrCodeResetLinkInvalid = "reset_link_invalid"
	// ErrCodePasswordChangedSignInAgain is the 409 from a password change that
	// was stored but could not reissue a session: sign in with the new password.
	ErrCodePasswordChangedSignInAgain = "password_changed_sign_in_again"
	// ErrCodeCredentialRevokeUnsupported is the 400 for revoke_api_credentials
	// on a deployment that cannot do it.
	ErrCodeCredentialRevokeUnsupported = "api_credentials_revoke_unsupported"
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

// Codes added since the last catalog sync. The descriptions are condensed
// from the public error reference; see it for the endpoints that raise each.
const (
	// ErrCodeDuplicateQueryParameter (HTTP 400): A query parameter appeared more
	// than once on a route that reads it as a single value.
	ErrCodeDuplicateQueryParameter = "duplicate_query_parameter"
	// ErrCodeDuplicateHeader (HTTP 400): `Authorization`, `X-Warmbly-Token`,
	// `Idempotency-Key` or `Origin` was sent on more than one header line.
	ErrCodeDuplicateHeader = "duplicate_header"
	// ErrCodeInvalidSortBy (HTTP 400): `POST /contacts/search`, `POST
	// /contacts/export` or a bulk action's `all` selection was given a `sort_by`
	// of the form `custom:<key>` whose key could never be a...
	ErrCodeInvalidSortBy = "invalid_sort_by"
	// ErrCodeInvalidColumn (HTTP 400): `PUT /me/views/:view` was given a column id
	// that view cannot render, the same column twice, more than 64 columns, or a
	// sort that names neither a sortable contact...
	ErrCodeInvalidColumn = "invalid_column"
	// ErrCodeDuplicateColumn (HTTP 400): `PUT /me/views/:view` was given a column
	// id that view cannot render, the same column twice, more than 64 columns, or
	// a sort that names neither a sortable contact...
	ErrCodeDuplicateColumn = "duplicate_column"
	// ErrCodeTooManyColumns (HTTP 400): `PUT /me/views/:view` was given a column
	// id that view cannot render, the same column twice, more than 64 columns, or
	// a sort that names neither a sortable contact...
	ErrCodeTooManyColumns = "too_many_columns"
	// ErrCodeInvalidSort (HTTP 400): `PUT /me/views/:view` was given a column id
	// that view cannot render, the same column twice, more than 64 columns, or a
	// sort that names neither a sortable contact...
	ErrCodeInvalidSort = "invalid_sort"
	// ErrCodeInvalidLayout (HTTP 400): `PUT /me/views/:view` was given a `layout`
	// on a view that has none, or a `unibox_rail` layout with an unknown field, an
	// empty or over-long key (200 bytes), more than...
	ErrCodeInvalidLayout = "invalid_layout"
	// ErrCodeInvalidMailHost (HTTP 400): `POST /contacts/search`, `POST
	// /contacts/export` or a bulk action's `all` selection was given a
	// `mail_hosts` entry that is not a documented provider value.
	ErrCodeInvalidMailHost = "invalid_mail_host"
	// ErrCodeEmptyStepBody (HTTP 400): `POST /campaigns/:id/start` found an email
	// step with nothing in either body, so it would send a blank message to every
	// lead it reached.
	ErrCodeEmptyStepBody = "empty_step_body"
	// ErrCodeTooManyTasks (HTTP 400): `PATCH /crm/tasks` or `DELETE /crm/tasks`
	// was given more than `1000` ids in one request, or more than `50,000`
	// exclusions.
	ErrCodeTooManyTasks = "too_many_tasks"
	// ErrCodeTooManyContacts (HTTP 400): A contact bulk action was given more than
	// `10,000` contact ids in one request, or more than `250,000` exclusions.
	ErrCodeTooManyContacts = "too_many_contacts"
	// ErrCodeSelectionTooLarge (HTTP 400): A `"all": true` bulk selection resolved
	// to more than its limit: `250,000` contacts, or `50,000` CRM tasks.
	ErrCodeSelectionTooLarge = "selection_too_large"
	// ErrCodeInvalidFilter (HTTP 400): A task filter carried an id that is not
	// one: `assigned_to`, `contact_id` and `deal_id` name records, and are matched
	// against id columns.
	ErrCodeInvalidFilter = "invalid_filter"
	// ErrCodeInvalidSetting (HTTP 400): `PATCH /outreach/settings` (or a
	// campaign's advanced settings) carried a value outside the documented
	// vocabulary, for example a `reply_intent.crm_task_intents` entry...
	ErrCodeInvalidSetting = "invalid_setting"
	// ErrCodeInvalidSlug (HTTP 400): `PATCH /organization/current` was given a
	// `slug` that is not 2 to 80 lowercase letters, numbers or dashes starting and
	// ending with a letter or number.
	ErrCodeInvalidSlug = "invalid_slug"
	// ErrCodeInvalidSyncFolder (HTTP 400): `PUT /emails/:id/sync` was given a
	// folder the sync always follows (`INBOX`, or a sent, drafts, spam, trash or
	// archive folder by attribute or name), a name that is...
	ErrCodeInvalidSyncFolder = "invalid_sync_folder"
	// ErrCodeInvalidTLSPin (HTTP 400): An SMTP/IMAP connect or reconnect carried a
	// `tls_pin` that is not a SHA-256 fingerprint (64 hexadecimal characters;
	// colons, spaces and a `sha256:` prefix are...
	ErrCodeInvalidTLSPin = "invalid_tls_pin"
	// ErrCodeMailboxCertificateNotPinned (HTTP 400): `POST
	// /emails/onboarding/smtp-imap/:id/certificates/confirm` on a mailbox whose
	// servers are both verified by a certificate authority, so there is no
	// certificate to...
	ErrCodeMailboxCertificateNotPinned = "mailbox_certificate_not_pinned"
	// ErrCodeLeadCCLimit (HTTP 400): Set a lead's CC was given more than `2`
	// contacts.
	ErrCodeLeadCCLimit = "lead_cc_limit"
	// ErrCodeLeadCCSelf (HTTP 400): Set a lead's CC named the lead itself as a
	// copy.
	ErrCodeLeadCCSelf = "lead_cc_self"
	// ErrCodeInvalidRecipient (HTTP 400): `POST /emails/:id/send`, `POST
	// /unibox/reply` or `POST /unibox/compose` was given a `to`, `cc` or `bcc`
	// entry that is not exactly one email address (`ana@example.com`...
	ErrCodeInvalidRecipient = "invalid_recipient"
	// ErrCodeRecipientSuppressed (HTTP 400): `POST /emails/:id/send`, `POST
	// /unibox/reply` or `POST /unibox/compose` named a `to`, `cc` or `bcc` address
	// on the workspace suppression list, directly or through its...
	ErrCodeRecipientSuppressed = "recipient_suppressed"
	// ErrCodeInvalidMessageID (HTTP 400): `POST /emails/:id/send` or `POST
	// /unibox/reply` was given an `in_reply_to` entry that is not one Message-ID:
	// printable characters with no spaces or line breaks, with...
	ErrCodeInvalidMessageID = "invalid_message_id"
	// ErrCodeImageUnreadable (HTTP 400): The file is empty or does not decode as
	// an image: it is damaged, or it is a different format than its name suggests.
	ErrCodeImageUnreadable = "image_unreadable"
	// ErrCodeImageFormatNotAllowed (HTTP 400): The file is an image, but not a
	// format this upload takes.
	ErrCodeImageFormatNotAllowed = "image_format_not_allowed"
	// ErrCodeImageTooLarge (HTTP 400): The file is over the size limit.
	ErrCodeImageTooLarge = "image_too_large"
	// ErrCodeImageDimensionsTooLarge (HTTP 400): The image is wider or taller than
	// the limit.
	ErrCodeImageDimensionsTooLarge = "image_dimensions_too_large"
	// ErrCodeImageTooSmall (HTTP 400): An OAuth app logo is under 32px on a side.
	ErrCodeImageTooSmall = "image_too_small"
	// ErrCodePasswordBreached (HTTP 400): The password appears in a public list of
	// breached passwords and was refused.
	ErrCodePasswordBreached = "password_breached"
	// ErrCodeSessionsNotEnded (HTTP 503): The same two routes could not sign out
	// every device on the account, so the password was not changed.
	ErrCodeSessionsNotEnded = "sessions_not_ended"
	// ErrCodeCloudLinkWorkspaceRequired (HTTP 409): Disconnecting a shared server
	// connection requires explicit confirmation of the server-wide scope; it
	// affects all local workspaces using that connection.
	ErrCodeCloudLinkWorkspaceRequired = "cloud_link_workspace_required"
	// ErrCodePoolLinkWorkspaceRequired (HTTP 409): Cloud refused a new connection
	// request without local workspace scope.
	ErrCodePoolLinkWorkspaceRequired = "pool_link_workspace_required"
	// ErrCodePoolLinkWorkspaceConnected (HTTP 409): The Cloud workspace already
	// has an active self-hosted connection, including a legacy link.
	ErrCodePoolLinkWorkspaceConnected = "pool_link_workspace_connected"
	// ErrCodeCloudLinkUpgradeRequired (HTTP 409): This Cloud deployment does not
	// support workspace-scoped connections.
	ErrCodeCloudLinkUpgradeRequired = "cloud_link_upgrade_required"
	// ErrCodeOrganizationCloudConnected (HTTP 409): Disconnect the workspace's
	// Cloud connection before deleting it or scheduling deletion of it or its
	// owner account.
	ErrCodeOrganizationCloudConnected = "organization_cloud_connected"
	// ErrCodeCloudLinkConnected (HTTP 409): The workspace is already connected to
	// Warmbly Cloud.
	ErrCodeCloudLinkConnected = "cloud_link_connected"
	// ErrCodeCloudLinkNotConnected (HTTP 409): The workspace is not connected to
	// Warmbly Cloud, so there is nothing to change or disconnect.
	ErrCodeCloudLinkNotConnected = "cloud_link_not_connected"
	// ErrCodeCloudLinkNoKey (HTTP 409): The instance has no
	// `CREDENTIALS_ENCRYPTION_KEY`, so it cannot store the Warmbly Cloud token.
	ErrCodeCloudLinkNoKey = "cloud_link_no_key"
	// ErrCodeCloudLinkURL (HTTP 400): The Cloud URL does not start with
	// `https://`.
	ErrCodeCloudLinkURL = "cloud_link_url"
	// ErrCodeCloudLinkNoPending (HTTP 404): No connection is in progress for this
	// workspace.
	ErrCodeCloudLinkNoPending = "cloud_link_no_pending"
	// ErrCodeCloudLinkCodeExpired (HTTP 404): The connection code expired before
	// it was approved in Warmbly Cloud.
	ErrCodeCloudLinkCodeExpired = "cloud_link_code_expired"
	// ErrCodeCloudLinkMailboxInactive (HTTP 422): Only active mailboxes can be
	// enrolled in Warmbly Cloud warmup.
	ErrCodeCloudLinkMailboxInactive = "cloud_link_mailbox_inactive"
	// ErrCodeCloudLinkOAuthMailbox (HTTP 422): A Google or Microsoft mailbox
	// connected with this instance's own OAuth app cannot be warmed by Warmbly
	// Cloud, because its refresh grant is bound to that app.
	ErrCodeCloudLinkOAuthMailbox = "cloud_link_oauth_mailbox"
	// ErrCodeCloudLinkLifecycle (HTTP 400): A pause or resume request named an
	// action other than `pause` or `resume`.
	ErrCodeCloudLinkLifecycle = "cloud_link_lifecycle"
	// ErrCodePoolLinkRequest (HTTP 400): The connection request is missing the
	// instance name.
	ErrCodePoolLinkRequest = "pool_link_request"
	// ErrCodePoolLinkCredential (HTTP 400): Enrolling a mailbox needs a credential
	// that matches the mailbox's provider.
	ErrCodePoolLinkCredential = "pool_link_credential"
	// ErrCodePoolLinkCodeNotFound (HTTP 404): The connection code is unknown or
	// has expired.
	ErrCodePoolLinkCodeNotFound = "pool_link_code_not_found"
	// ErrCodePoolLinkCodeUsed (HTTP 409): The connection code has already been
	// used.
	ErrCodePoolLinkCodeUsed = "pool_link_code_used"
	// ErrCodePoolLinkDenied (HTTP 403): The connection was declined in Warmbly
	// Cloud.
	ErrCodePoolLinkDenied = "pool_link_denied"
	// ErrCodePoolLinkRevoked (HTTP 401): The instance's link has been revoked.
	ErrCodePoolLinkRevoked = "pool_link_revoked"
	// ErrCodePoolLinkInstanceNotFound (HTTP 404): The linked instance no longer
	// exists in Warmbly Cloud.
	ErrCodePoolLinkInstanceNotFound = "pool_link_instance_not_found"
	// ErrCodePoolLinkMailboxNotFound (HTTP 404): The mailbox is not enrolled in
	// Warmbly Cloud.
	ErrCodePoolLinkMailboxNotFound = "pool_link_mailbox_not_found"
	// ErrCodePoolLinkMailboxLimit (HTTP 402): The Cloud workspace has used its
	// free allowance of linked mailboxes.
	ErrCodePoolLinkMailboxLimit = "pool_link_mailbox_limit"
	// ErrCodePoolLinkAlreadyAdopted (HTTP 409): The mailbox is already linked to
	// an instance.
	ErrCodePoolLinkAlreadyAdopted = "pool_link_already_adopted"
	// ErrCodePoolLinkNotAdoptable (HTTP 422): Only active Google and Microsoft
	// mailboxes in the Cloud workspace can be linked to an instance.
	ErrCodePoolLinkNotAdoptable = "pool_link_not_adoptable"
	// ErrCodePoolLinkNotManaged (HTTP 403): The mailbox's credential is held by
	// the instance, not by Warmbly Cloud, so Cloud cannot act on it.
	ErrCodePoolLinkNotManaged = "pool_link_not_managed"
	// ErrCodePoolLinkMailboxBlocked (HTTP 403): Warmbly Cloud blocked the mailbox
	// for hurting the pool.
	ErrCodePoolLinkMailboxBlocked = "pool_link_mailbox_blocked"
	// ErrCodePoolLinkMailboxInactive (HTTP 403): The mailbox is not active in
	// Warmbly Cloud.
	ErrCodePoolLinkMailboxInactive = "pool_link_mailbox_inactive"
	// ErrCodePoolLinkWarmupUnavailable (HTTP 403): Warmup is not available on the
	// Cloud workspace.
	ErrCodePoolLinkWarmupUnavailable = "pool_link_warmup_unavailable"
	// ErrCodePoolLinkOAuthPending (HTTP 409): A mailbox sign-in started through
	// Warmbly Cloud has not completed yet.
	ErrCodePoolLinkOAuthPending = "pool_link_oauth_pending"
	// ErrCodePoolLinkOAuthSession (HTTP 404): The mailbox sign-in session is
	// unknown or has expired.
	ErrCodePoolLinkOAuthSession = "pool_link_oauth_session"
	// ErrCodeCLIAuthRequest (HTTP 400): `POST /auth/cli/poll` was sent without a
	// `device_code`.
	ErrCodeCLIAuthRequest = "cli_auth_request"
	// ErrCodeCLIAuthScopes (HTTP 400): The CLI asked for scopes that include
	// permission bits this instance does not grant.
	ErrCodeCLIAuthScopes = "cli_auth_scopes"
	// ErrCodeCLIAuthCodeNotFound (HTTP 404): The code is unknown or has expired.
	ErrCodeCLIAuthCodeNotFound = "cli_auth_code_not_found"
	// ErrCodeCLIAuthCodeUsed (HTTP 409): The code has already been used.
	ErrCodeCLIAuthCodeUsed = "cli_auth_code_used"
	// ErrCodeCLIAuthForbidden (HTTP 403): Approving a CLI sign-in needs the
	// `manage_api_keys` permission in the workspace, because the approval mints an
	// API key.
	ErrCodeCLIAuthForbidden = "cli_auth_forbidden"
	// ErrCodeCloudWarmupValueUnsupported (HTTP 422): Warmbly Cloud does not take 0
	// from an instance for the starting volume, maximum, daily increase or reply
	// rate.
	ErrCodeCloudWarmupValueUnsupported = "cloud_warmup_value_unsupported"
	// ErrCodeCloudWarmupNotApplied (HTTP 409): Warmbly Cloud answered but did not
	// apply the named settings.
	ErrCodeCloudWarmupNotApplied = "cloud_warmup_not_applied"
	// ErrCodeCloudWarmupManaged (HTTP 409): Participation, the shared daily and
	// recipient limits, and stopping warmup are changed with the mailbox's Warmbly
	// Cloud controls, not on the mailbox.
	ErrCodeCloudWarmupManaged = "cloud_warmup_managed"
	// ErrCodeCloudLinkDisconnecting (HTTP 409): The workspace is disconnecting
	// from Warmbly Cloud.
	ErrCodeCloudLinkDisconnecting = "cloud_link_disconnecting"
	// ErrCodeCloudLinkUnreachable (HTTP 503): Warmbly Cloud could not be reached.
	ErrCodeCloudLinkUnreachable = "cloud_link_unreachable"
	// ErrCodeCloudLinkBadResponse (HTTP 503): Warmbly Cloud answered without the
	// mailbox's warmup settings.
	ErrCodeCloudLinkBadResponse = "cloud_link_bad_response"
	// ErrCodeInvalidName (HTTP 400): A first name, last name, workspace name,
	// company name, OAuth app name or mailbox display name broke the naming rules.
	ErrCodeInvalidName = "invalid_name"
	// ErrCodeRedirectTargetNotOwnDomain (HTTP 400): A form's `redirect_url`, a
	// sending domain's redirect `target_url`, or a vendor forwarding `url` is not
	// on one of the workspace's own domains.
	ErrCodeRedirectTargetNotOwnDomain = "redirect_target_not_own_domain"
	// ErrCodeMailboxImportEmpty (HTTP 400): No file and no pasted text, or an
	// input with no rows, or a header row and nothing under it.
	ErrCodeMailboxImportEmpty = "mailbox_import_empty"
	// ErrCodeMailboxImportTooLarge (HTTP 400): More than 5,000 mailboxes, a file
	// over 10 MB, a pasted list over 2 MB, or a body that is not multipart form
	// data.
	ErrCodeMailboxImportTooLarge = "mailbox_import_too_large"
	// ErrCodeMailboxImportNoEmailColumn (HTTP 400): `POST /emails/imports` with a
	// mapping in which no column is `email`.
	ErrCodeMailboxImportNoEmailColumn = "mailbox_import_no_email_column"
	// ErrCodeMailboxImportRowIncomplete (HTTP 400): `PATCH
	// /emails/imports/:id/rows/:line` left the row without an SMTP and IMAP server
	// or without a password.
	ErrCodeMailboxImportRowIncomplete = "mailbox_import_row_incomplete"
	// ErrCodeInvalidCursor (HTTP 400): `GET /emails/imports` or `GET
	// /emails/imports/:id/rows` was given a `cursor` this API did not issue.
	ErrCodeInvalidCursor = "invalid_cursor"
	// ErrCodeMailboxImportRowNotFailed (HTTP 409): Only a row that did not connect
	// can be fixed; this one is queued, running, connected, updated, skipped,
	// canceled or waiting for sign-in.
	ErrCodeMailboxImportRowNotFailed = "mailbox_import_row_not_failed"
	// ErrCodeMailboxImportCredentialsExpired (HTTP 409): The row's credentials
	// were deleted 7 days after the import finished, so it cannot be retried.
	ErrCodeMailboxImportCredentialsExpired = "mailbox_import_credentials_expired"
	// ErrCodeMailboxImportNothingToRetry (HTTP 409): `POST
	// /emails/imports/:id/retry` matched no failed row that still holds its
	// credentials.
	ErrCodeMailboxImportNothingToRetry = "mailbox_import_nothing_to_retry"
	// ErrCodeMailboxVendorUnauthorized (HTTP 400): The vendor did not accept the
	// API key.
	ErrCodeMailboxVendorUnauthorized = "mailbox_vendor_unauthorized"
	// ErrCodeMailboxVendorNoWorkspace (HTTP 400): The key is accepted but the
	// vendor lists no workspace or organization for it, so there is nothing to
	// read.
	ErrCodeMailboxVendorNoWorkspace = "mailbox_vendor_no_workspace"
	// ErrCodeMailboxVendorInvalidFields (HTTP 400): A field the vendor requires is
	// empty, or a field holds control characters.
	ErrCodeMailboxVendorInvalidFields = "mailbox_vendor_invalid_fields"
	// ErrCodeMailboxVendorUnknown (HTTP 400): `vendor` is not one of the supported
	// vendors.
	ErrCodeMailboxVendorUnknown = "mailbox_vendor_unknown"
	// ErrCodeMailboxVendorRateLimited (HTTP 429): The vendor is rate limiting
	// Warmbly's requests.
	ErrCodeMailboxVendorRateLimited = "mailbox_vendor_rate_limited"
	// ErrCodeMailboxVendorUnavailable (HTTP 400): The vendor answered with
	// something unexpected, did not answer, or no longer has the mailbox.
	ErrCodeMailboxVendorUnavailable = "mailbox_vendor_unavailable"
	// ErrCodeMailboxGrantNotConfigured (HTTP 400): This instance has no Google
	// service account or no Microsoft app for admin grants.
	ErrCodeMailboxGrantNotConfigured = "mailbox_grant_not_configured"
	// ErrCodeGoogleDelegationUnauthorized (HTTP 400): Google refused the domain-
	// wide delegation, or accepted it and refused the call: the client ID or a
	// scope is missing from the Admin console entry, an OAuth client ID...
	ErrCodeGoogleDelegationUnauthorized = "google_delegation_unauthorized"
	// ErrCodeMicrosoftConsentMissing (HTTP 400): The Microsoft 365 organization
	// has not granted admin consent, does not exist, or the consent lacks
	// `Mail.ReadWrite`, `Mail.Send` or `User.Read.All`; or the consent...
	ErrCodeMicrosoftConsentMissing = "microsoft_consent_missing"
	// ErrCodeMailboxGrantStateInvalid (HTTP 400): The Google sign-in or Microsoft
	// consent `state` expired (15 minutes), was already used, or was started by
	// another member or workspace, or Google did not accept the...
	ErrCodeMailboxGrantStateInvalid = "mailbox_grant_state_invalid"
	// ErrCodeMailboxGrantProofMissing (HTTP 400): The workspace has not proved it
	// controls the Google Workspace domain: the `_warmbly.<domain>` `TXT` record
	// does not hold this workspace's `warmbly-verify=` value yet,...
	ErrCodeMailboxGrantProofMissing = "mailbox_grant_proof_missing"
	// ErrCodeMailboxGrantUnavailable (HTTP 503): Google, Microsoft or the DNS
	// lookup for the proof did not answer.
	ErrCodeMailboxGrantUnavailable = "mailbox_grant_unavailable"
	// ErrCodeMailboxGrantDomainMismatch (HTTP 400): The admin address is not on
	// the domain entered, or the mailbox is on a domain the grant does not cover.
	ErrCodeMailboxGrantDomainMismatch = "mailbox_grant_domain_mismatch"
	// ErrCodeMailboxGrantMailboxUnreachable (HTTP 400): Google or Microsoft has no
	// mailbox for the account: it does not exist, is suspended or disabled, or has
	// no Exchange Online license.
	ErrCodeMailboxGrantMailboxUnreachable = "mailbox_grant_mailbox_unreachable"
	// ErrCodeMailboxGrantInactive (HTTP 400): The grant failed its last check, so
	// it connects nothing.
	ErrCodeMailboxGrantInactive = "mailbox_grant_inactive"
	// ErrCodeMailboxVendorDomainNotFound (HTTP 404): `PUT
	// /emails/domains/:domain/vendor-forwarding` or `POST
	// /emails/domains/:domain/vendor-tracking` on a domain none of the workspace's
	// active vendor accounts holds.
	ErrCodeMailboxVendorDomainNotFound = "mailbox_vendor_domain_not_found"
	// ErrCodeMailboxVendorDomainUnsupported (HTTP 400): The vendor holding the
	// domain cannot do this through its API: it has no forwarding removal, writes
	// no DNS records for Warmbly, or the existing record is one the...
	ErrCodeMailboxVendorDomainUnsupported = "mailbox_vendor_domain_unsupported"
	// ErrCodeMailboxReauthDelegated (HTTP 409): `POST
	// /emails/onboarding/oauth/reauth/:id` on a mailbox connected through an admin
	// grant.
	ErrCodeMailboxReauthDelegated = "mailbox_reauth_delegated"
	// ErrCodeSendingDomainNotInWorkspace (HTTP 404): The workspace has no mailbox
	// on this domain.
	ErrCodeSendingDomainNotInWorkspace = "sending_domain_not_in_workspace"
	// ErrCodeSendingDomainSharedProvider (HTTP 400): The domain belongs to a
	// shared email provider, such as `gmail.com`, and cannot be redirected.
	ErrCodeSendingDomainSharedProvider = "sending_domain_shared_provider"
	// ErrCodeDomainRedirectInvalidTarget (HTTP 400): `target_url` (or a vendor
	// forwarding `url`) is not an `http` or `https` address on a host, carries
	// credentials, is longer than 2,048 characters, or points back at the...
	ErrCodeDomainRedirectInvalidTarget = "domain_redirect_invalid_target"
	// ErrCodeDomainRedirectTaken (HTTP 409): Another workspace on this instance
	// already serves a verified redirect for this domain, or already has Warmbly
	// Cloud serve it.
	ErrCodeDomainRedirectTaken = "domain_redirect_taken"
	// ErrCodeDomainRedirectLinked (HTTP 409): On Warmbly Cloud: this domain's
	// redirect is served for a linked self-hosted instance, and only that instance
	// changes it.
	ErrCodeDomainRedirectLinked = "domain_redirect_linked"
	// ErrCodeDomainRedirectCloudUnavailable (HTTP 409): `served_by` is `cloud`,
	// but this instance is not linked to Warmbly Cloud, or Cloud does not serve
	// redirects for it.
	ErrCodeDomainRedirectCloudUnavailable = "domain_redirect_cloud_unavailable"
	// ErrCodeDomainRedirectCloudUnreachable (HTTP 503): Warmbly Cloud could not be
	// reached while saving, moving or removing a redirect it serves, so nothing
	// changed.
	ErrCodeDomainRedirectCloudUnreachable = "domain_redirect_cloud_unreachable"
	// ErrCodeDomainRedirectLimit (HTTP 409): Warmbly Cloud already serves 200
	// redirects for this linked instance.
	ErrCodeDomainRedirectLimit = "domain_redirect_limit"
	// ErrCodePoolLinkRedirectNotFound (HTTP 404): On Warmbly Cloud's linked-
	// instance API: Cloud serves no redirect for this domain for the calling
	// instance.
	ErrCodePoolLinkRedirectNotFound = "pool_link_redirect_not_found"
	// ErrCodePoolLinkCleartextMailbox (HTTP 422): Enrolling a mailbox in Warmbly
	// Cloud, or updating its credential there: SMTP or IMAP uses security `none`.
	ErrCodePoolLinkCleartextMailbox = "pool_link_cleartext_mailbox"
	// ErrCodePoolLinkMailboxConflict (HTTP 409): Enrollment found the address
	// already connected in the linked Cloud workspace and could not attach it.
	ErrCodePoolLinkMailboxConflict = "pool_link_mailbox_conflict"
	// ErrCodePoolLinkInstanceURL (HTTP 422): Starting a Google or Microsoft sign-
	// in through Warmbly Cloud from an instance that registered no address of its
	// own when it was linked, so Cloud has nowhere to return...
	ErrCodePoolLinkInstanceURL = "pool_link_instance_url"
	// ErrCodePoolLinkReturnURL (HTTP 400): The same sign-in named a return address
	// that is not on the instance's registered address (same scheme and host).
	ErrCodePoolLinkReturnURL = "pool_link_return_url"
	// ErrCodePoolLinkOAuthBrowser (HTTP 400): The brokered sign-in was not
	// continued from the Warmbly Cloud page that started it, or returned to a
	// different browser.
	ErrCodePoolLinkOAuthBrowser = "pool_link_oauth_browser"
	// ErrCodeCloudLinkManagedProtocol (HTTP 409): Cloud does not advertise durable
	// managed-consent protocol 1.
	ErrCodeCloudLinkManagedProtocol = "cloud_link_managed_protocol"
	// ErrCodeCloudLinkSMTPAssociationProtocol (HTTP 409): Cloud does not advertise
	// `smtp_association_protocol: 1`.
	ErrCodeCloudLinkSMTPAssociationProtocol = "cloud_link_smtp_association_protocol"
	// ErrCodeCloudLinkSMTPAssociation (HTTP 409): The explicitly selected existing
	// Cloud SMTP/IMAP mailbox could not be associated.
	ErrCodeCloudLinkSMTPAssociation = "cloud_link_smtp_association"
	// ErrCodePoolLinkOAuthUnknown (HTTP 409): The provider exchange has no
	// confirmed committed mailbox.
	ErrCodePoolLinkOAuthUnknown = "pool_link_oauth_unknown"
	// ErrCodeCLIAuthScopesRole (HTTP 403): Approving a CLI sign-in: your role in
	// the chosen workspace allows none of the scopes the CLI asked for.
	ErrCodeCLIAuthScopesRole = "cli_auth_scopes_role"
	// ErrCodeSendingDomainBulkInvalid (HTTP 400): A bulk setup named no domain,
	// more than 100, neither a `tracking_label` nor a `redirect_url`, a
	// `tracking_label` that is not one DNS label, or a `served_by` other...
	ErrCodeSendingDomainBulkInvalid = "sending_domain_bulk_invalid"
	// ErrCodeTrackingDomainTaken (HTTP 409): Another workspace on this instance
	// has already verified this tracking domain, on a mailbox or as a campaign's
	// override.
	ErrCodeTrackingDomainTaken = "tracking_domain_taken"
	// ErrCodeTrackingHostNotConfigured (HTTP 400): This instance has no tracking
	// host (`TRACKING_DOMAIN`), so it cannot serve a tracking domain or a
	// redirect.
	ErrCodeTrackingHostNotConfigured = "tracking_host_not_configured"
	// ErrCodeAppPasswordInvalid (HTTP 400): `app_password` is not 16 letters once
	// spaces are removed.
	ErrCodeAppPasswordInvalid = "app_password_invalid"
	// ErrCodeMailboxNotGoogleSignin (HTTP 409): The mailbox is not connected with
	// per-mailbox Google sign-in: it is already on an app password or an admin
	// grant, is an Outlook or SMTP/IMAP mailbox, or its sign-in...
	ErrCodeMailboxNotGoogleSignin = "mailbox_not_google_signin"
	// ErrCodePlacementNotEntitled (HTTP 402): The workspace has no active trial or
	// subscription, so it cannot start a test.
	ErrCodePlacementNotEntitled = "placement_not_entitled"
	// ErrCodePlacementQuotaExceeded (HTTP 402): The workspace has used its free
	// tests for the month on the instance panel, or the linked Warmbly Cloud
	// workspace has used its free tests on Warmbly Cloud's panel.
	ErrCodePlacementQuotaExceeded = "placement_quota_exceeded"
	// ErrCodePlacementTooManyRunning (HTTP 429): The workspace already has `3`
	// tests running, counting each half of a tracking comparison.
	ErrCodePlacementTooManyRunning = "placement_too_many_running"
	// ErrCodePlacementSenderBusy (HTTP 409): The sending mailbox still has copies
	// of another test waiting to be sent.
	ErrCodePlacementSenderBusy = "placement_sender_busy"
	// ErrCodePlacementSenderUnavailable (HTTP 409): The sending mailbox is not
	// connected and active, or it is itself a seed inbox, which receives tests and
	// cannot send one.
	ErrCodePlacementSenderUnavailable = "placement_sender_unavailable"
	// ErrCodePlacementDailyBudget (HTTP 409): The mailbox has too little of its
	// daily limit left today for a useful test (at least five seeds per test, ten
	// for a comparison), or for every seed chosen with `seed_ids`.
	ErrCodePlacementDailyBudget = "placement_daily_budget"
	// ErrCodePlacementNoSeeds (HTTP 409): The panel has no seed inbox the test
	// could use: none were added, none is connected, or every one is on the
	// sender's own domain, which is always skipped.
	ErrCodePlacementNoSeeds = "placement_no_seeds"
	// ErrCodePlacementPanelUnavailable (HTTP 409): `panel` is `cloud` on an
	// instance that is not linked to Warmbly Cloud.
	ErrCodePlacementPanelUnavailable = "placement_panel_unavailable"
	// ErrCodePlacementInvalidSeeds (HTTP 400): `seed_ids` names a mailbox that is
	// not a seed inbox of the workspace, or one that is not connected and running
	// right now; the `message` names it.
	ErrCodePlacementInvalidSeeds = "placement_invalid_seeds"
	// ErrCodePlacementInvalidTracking (HTTP 400): `tracking` is `on` or `compare`
	// for a campaign that sends plain text, which carries no tracking.
	ErrCodePlacementInvalidTracking = "placement_invalid_tracking"
	// ErrCodePlacementNotRunning (HTTP 409): `POST /placement/tests/:id/cancel` on
	// a test that has already finished or been canceled.
	ErrCodePlacementNotRunning = "placement_not_running"
	// ErrCodePlacementSeedUnavailable (HTTP 409): The mailbox cannot be marked as
	// a seed inbox: it is on the instance panel, or a placement test is still
	// sending from it.
	ErrCodePlacementSeedUnavailable = "placement_seed_unavailable"
	// ErrCodePlacementSeedLimit (HTTP 409): The workspace already has `50` seed
	// inboxes.
	ErrCodePlacementSeedLimit = "placement_seed_limit"
	// ErrCodeMailboxIsSeed (HTTP 409): Warmup was started or resumed on a seed
	// inbox.
	ErrCodeMailboxIsSeed = "mailbox_is_seed"
	// ErrCodePlacementBatchEmpty (HTTP 400): `POST /placement/batches` resolved to
	// no sending mailbox: the scope, its filters and the sample left nothing.
	ErrCodePlacementBatchEmpty = "placement_batch_empty"
	// ErrCodePlacementBatchTooLarge (HTTP 400): The batch would hold more
	// mailboxes than the instance allows in one batch (`10,000` by default, an
	// operator setting).
	ErrCodePlacementBatchTooLarge = "placement_batch_too_large"
	// ErrCodePlacementTooManyBatches (HTTP 429): The workspace already has `5`
	// batches running.
	ErrCodePlacementTooManyBatches = "placement_too_many_batches"
	// ErrCodePlacementBatchNotRunning (HTTP 409): `POST
	// /placement/batches/:id/cancel` on a batch that has already finished or been
	// canceled.
	ErrCodePlacementBatchNotRunning = "placement_batch_not_running"
	// ErrCodeInvalidListing (HTTP 400): A listing field breaks a rule: the `slug`
	// is not 3 to 48 lowercase letters, numbers or single dashes, or is reserved
	// for a built-in integration; the `tagline` is...
	ErrCodeInvalidListing = "invalid_listing"
	// ErrCodeAppNotListable (HTTP 400): The app cannot be listed: it is disabled
	// or suspended, or it is a client created by dynamic registration, which has
	// no publishing workspace.
	ErrCodeAppNotListable = "app_not_listable"
	// ErrCodeDeveloperAccessBlocked (HTTP 403): The instance's operators have
	// blocked this workspace or person from registering and publishing apps.
	ErrCodeDeveloperAccessBlocked = "developer_access_blocked"
	// ErrCodeInvalidWebsite (HTTP 400): `website` on `POST` or `PATCH
	// /oauth/applications` is not an `http` or `https` address on a host without
	// credentials.
	ErrCodeInvalidWebsite = "invalid_website"
	// ErrCodeInvalidLogo (HTTP 400): `logo_url` on `POST` or `PATCH
	// /oauth/applications` is not an address this instance issued for the
	// workspace.
	ErrCodeInvalidLogo = "invalid_logo"
	// ErrCodeListingSlugTaken (HTTP 409): Another app on this instance already
	// uses the `slug`.
	ErrCodeListingSlugTaken = "listing_slug_taken"
	// ErrCodeListingHidden (HTTP 409): The instance's operators hid this listing,
	// so `DELETE /oauth/applications/{id}/listing` cannot remove it.
	ErrCodeListingHidden = "listing_hidden"
	// ErrCodeAppSuspended (HTTP 409): The instance's operators suspended this app.
	ErrCodeAppSuspended = "app_suspended"
	// ErrCodeOAuthTokenNotAllowed (HTTP 403): An OAuth app token called a route
	// that manages credentials: creating, editing, revoking or deleting API keys,
	// or anything under `/oauth/applications` and...
	ErrCodeOAuthTokenNotAllowed = "oauth_token_not_allowed"
	// ErrCodeAPIKeyPermissionsExceedCaller (HTTP 403): `POST /api-keys` or `PATCH
	// /api-keys/{id}` would give a key more than its creator has: a permission the
	// member's role does not cover, or, for an API key caller, a...
	ErrCodeAPIKeyPermissionsExceedCaller = "api_key_permissions_exceed_caller"
	// ErrCodeAPIKeyHolderLeft (HTTP 401): The member who created this API key is
	// no longer in the workspace, so the key no longer works.
	ErrCodeAPIKeyHolderLeft = "api_key_holder_left"
	// ErrCodeAPIKeyMailboxLimited (HTTP 403): An API key limited to some mailboxes
	// called a surface that acts across the workspace: `/ai/tools` or `/mcp`, a
	// campaign sender pool resolved from mailbox tags, or...
	ErrCodeAPIKeyMailboxLimited = "api_key_mailbox_limited"
	// ErrCodeCRMNotConnected (HTTP 409): `GET /crm/metadata`, `POST /crm/sync`,
	// `POST /crm/backfill`, a `/crm/contacts/:id` or a `/crm/lists` call on a
	// workspace that is not in HubSpot or Pipedrive mode.
	ErrCodeCRMNotConnected = "crm_not_connected"
	// ErrCodeCRMReauthRequired (HTTP 409): The provider revoked Warmbly's access,
	// the connection was removed, or the connection lacks a permission provider
	// mode needs (in HubSpot deals, companies, owners, the...
	ErrCodeCRMReauthRequired = "crm_reauth_required"
	// ErrCodeCRMManagedExternally (HTTP 409): A create, update or delete of a
	// pipeline, stage or task type.
	ErrCodeCRMManagedExternally = "crm_managed_externally"
	// ErrCodeCRMStageUnknown (HTTP 400): The `pipeline_id` or `stage_id` given is
	// not a mirrored pipeline or stage of the provider, or (in HubSpot) a deal was
	// marked `won` or `lost` in a pipeline with no...
	ErrCodeCRMStageUnknown = "crm_stage_unknown"
	// ErrCodeCRMOwnerUnmapped (HTTP 400): A deal or task was assigned to a member
	// who is not matched to a HubSpot owner or Pipedrive user.
	ErrCodeCRMOwnerUnmapped = "crm_owner_unmapped"
	// ErrCodeCRMContactMissing (HTTP 404): The contact is not in the provider, and
	// creating contacts there is turned off in the workspace's CRM settings.
	ErrCodeCRMContactMissing = "crm_contact_missing"
	// ErrCodeCRMRecordMissing (HTTP 404): The record no longer exists in the
	// provider; it was deleted there.
	ErrCodeCRMRecordMissing = "crm_record_missing"
	// ErrCodeCRMProviderRejected (HTTP 422): The provider refused the change.
	ErrCodeCRMProviderRejected = "crm_provider_rejected"
	// ErrCodeCRMSyncRunning (HTTP 429): `POST /crm/sync` within 30 seconds
	// (HubSpot) or a minute (Pipedrive) of the last one.
	ErrCodeCRMSyncRunning = "crm_sync_running"
	// ErrCodeCRMUnavailable (HTTP 503): The provider did not answer or failed,
	// including a Pipedrive company out of its daily API budget.
	ErrCodeCRMUnavailable = "crm_unavailable"
	// ErrCodeInvalidSalesforceDomain (HTTP 400): The custom domain given to `POST
	// /integrations/oauth/start` is not a Salesforce My Domain
	// (`*.my.salesforce.com`).
	ErrCodeInvalidSalesforceDomain = "invalid_salesforce_domain"
	// ErrCodeInvalidSalesforceSettings (HTTP 400): `PUT
	// /integrations/salesforce/:id/settings` named something the sync cannot run:
	// an unknown field, a related field to write, an engagement field to read, a
	// field...
	ErrCodeInvalidSalesforceSettings = "invalid_salesforce_settings"
	// ErrCodeSalesforceError (HTTP 400): Salesforce refused the request.
	ErrCodeSalesforceError = "salesforce_error"
	// ErrCodeSalesforceUnreachable (HTTP 400): Salesforce did not answer.
	ErrCodeSalesforceUnreachable = "salesforce_unreachable"
	// ErrCodeSalesforceSyncOff (HTTP 400): `POST
	// /integrations/salesforce/:id/sync-now` on a connection with sync turned off.
	ErrCodeSalesforceSyncOff = "salesforce_sync_off"
	// ErrCodeSalesforceReconnectRequired (HTTP 409): The Salesforce session ended
	// (a revoked app, a password reset, a deactivated user).
	ErrCodeSalesforceReconnectRequired = "salesforce_reconnect_required"
	// ErrCodeImportRunning (HTTP 409): `POST /integrations/salesforce/:id/import-
	// sources/:sourceId/run` or `POST /integrations/apollo/:id/import-
	// sources/:sourceId/run` while that import is already running.
	ErrCodeImportRunning = "import_running"
	// ErrCodeSalesforceRateLimited (HTTP 429): The Salesforce org has used its API
	// requests for today.
	ErrCodeSalesforceRateLimited = "salesforce_rate_limited"
	// ErrCodeApolloEnrichTooMany (HTTP 400): `POST
	// /integrations/apollo/:id/enrichments` selected more than 5,000 contacts.
	ErrCodeApolloEnrichTooMany = "apollo_enrich_too_many"
	// ErrCodeClayWebhookURL (HTTP 400): `PATCH /integrations/clay/:id` set a
	// `webhook_url` that is not a Clay table webhook.
	ErrCodeClayWebhookURL = "clay_webhook_url"
	// ErrCodeClayWebhookMissing (HTTP 400): `PATCH /integrations/clay/:id` chose
	// events to send with no table webhook saved.
	ErrCodeClayWebhookMissing = "clay_webhook_missing"
	// ErrCodeApolloKeyRejected (HTTP 422): Apollo refused the saved API key.
	ErrCodeApolloKeyRejected = "apollo_key_rejected"
	// ErrCodeApolloKeyForbidden (HTTP 422): The API key works but cannot reach an
	// endpoint the integration uses.
	ErrCodeApolloKeyForbidden = "apollo_key_forbidden"
	// ErrCodeApolloKeyMissing (HTTP 422): The Apollo connection has no API key.
	ErrCodeApolloKeyMissing = "apollo_key_missing"
	// ErrCodeApolloRateLimited (HTTP 429): Apollo's rate limit for the account was
	// reached.
	ErrCodeApolloRateLimited = "apollo_rate_limited"
	// ErrCodeActionProviderMismatch (HTTP 400): `POST /automations`, `PATCH
	// /automations/:id` or `POST /integrations/connections/:id/events` paired an
	// action with a connection whose provider does not run it, such...
	ErrCodeActionProviderMismatch = "action_provider_mismatch"
	// ErrCodeInvitationExpired (HTTP 400): `POST /invitations/accept` on an
	// invitation older than 48 hours.
	ErrCodeInvitationExpired = "invitation_expired"
	// ErrCodeInvitationUsedOrExpired (HTTP 400): `POST /invitations/accept` on an
	// invitation that was accepted or expired while the request was in flight.
	ErrCodeInvitationUsedOrExpired = "invitation_used_or_expired"
	// ErrCodeInvitationRolesRemoved (HTTP 400): `POST /invitations/accept` on an
	// invitation whose roles were all deleted since it was sent.
	ErrCodeInvitationRolesRemoved = "invitation_roles_removed"
	// ErrCodeAccountSuspended (HTTP 403): The account is suspended and cannot sign
	// in, by password, passkey or two-factor confirmation.
	ErrCodeAccountSuspended = "account_suspended"
	// ErrCodeAccountSendingSuspended (HTTP 403): The account may not send mail.
	ErrCodeAccountSendingSuspended = "account_sending_suspended"
	// ErrCodeAccountWorkspaceCreateSuspended (HTTP 403): The account may not
	// create workspaces.
	ErrCodeAccountWorkspaceCreateSuspended = "account_workspace_create_suspended"
	// ErrCodeWorkspaceSendingSuspended (HTTP 403): Sending is paused for the whole
	// workspace while it is under review.
	ErrCodeWorkspaceSendingSuspended = "workspace_sending_suspended"
	// ErrCodePasskeyUserVerificationRequired (HTTP 400): A passkey sign-in came
	// from an authenticator that did not verify the user with a PIN or biometric.
	ErrCodePasskeyUserVerificationRequired = "passkey_user_verification_required"
	// ErrCodeTwoFaInvalidCode (HTTP 400): The authenticator or recovery code did
	// not match.
	ErrCodeTwoFaInvalidCode = "two_fa_invalid_code"
	// ErrCodeMemberAccessRestricted (HTTP 403): The caller's selected-resource
	// scope does not cover this route.
	ErrCodeMemberAccessRestricted = "member_access_restricted"
	// ErrCodeInvalidAccessResource (HTTP 400): `PUT
	// /organization/members/:id/access` or an invitation named a campaign folder,
	// campaign or mailbox that is not in this workspace.
	ErrCodeInvalidAccessResource = "invalid_access_resource"
	// ErrCodeInvalidAccessScope (HTTP 400): The scope is neither `workspace` nor
	// `restricted`.
	ErrCodeInvalidAccessScope = "invalid_access_scope"
	// ErrCodeTooManyAccessGrants (HTTP 400): More than 500 folders, campaigns or
	// mailboxes in one list.
	ErrCodeTooManyAccessGrants = "too_many_access_grants"
	// ErrCodeOwnerAccessUnrestricted (HTTP 403): The workspace owner always
	// reaches the entire workspace and cannot be restricted.
	ErrCodeOwnerAccessUnrestricted = "owner_access_unrestricted"
	// ErrCodeUnknownView (HTTP 404): `/me/views/:view` was given a view name other
	// than `contacts`, `campaign_leads` or `unibox_rail`.
	ErrCodeUnknownView = "unknown_view"
	// ErrCodeLeadCCContactNotFound (HTTP 404): Set a lead's CC named a contact
	// that is not in the workspace.
	ErrCodeLeadCCContactNotFound = "lead_cc_contact_not_found"
	// ErrCodeSlackNotConnected (HTTP 404): A Slack route was called for a
	// workspace that has not connected Slack, or whose connection was removed.
	ErrCodeSlackNotConnected = "slack_not_connected"
	// ErrCodeSlackLinkInvalid (HTTP 404): The Slack link code is unknown, expired
	// or already used.
	ErrCodeSlackLinkInvalid = "slack_link_invalid"
	// ErrCodeSlackVerifyFailed (HTTP 404): `POST /integrations/slack/link` with a
	// Sign in with Slack result that is unknown, expired, already used, started by
	// someone else or for another code.
	ErrCodeSlackVerifyFailed = "slack_verify_failed"
	// ErrCodeSlackVerifyWrongAccount (HTTP 404): `POST /integrations/slack/link`
	// after Sign in with Slack signed in a different Slack account than the one
	// the link code names.
	ErrCodeSlackVerifyWrongAccount = "slack_verify_wrong_account"
	// ErrCodeSlackVerifyUnavailable (HTTP 404): `POST
	// /integrations/slack/link/verify` on an instance whose Slack app has no
	// client credentials.
	ErrCodeSlackVerifyUnavailable = "slack_verify_unavailable"
	// ErrCodeSlackNotLinked (HTTP 404): `PATCH /integrations/slack/link` from a
	// member who has not linked a Slack account in this workspace.
	ErrCodeSlackNotLinked = "slack_not_linked"
	// ErrCodeContactEmailTaken (HTTP 409): The address given to update a contact
	// already belongs to another contact.
	ErrCodeContactEmailTaken = "contact_email_taken"
	// ErrCodeLeadCCLeadIsCopied (HTTP 409): Set a lead's CC on a lead that is
	// itself copied on another lead in the campaign, so it sends nothing of its
	// own to copy anyone on.
	ErrCodeLeadCCLeadIsCopied = "lead_cc_lead_is_copied"
	// ErrCodeLeadCCHasCopies (HTTP 409): Set a lead's CC names a contact that has
	// copies of their own in the campaign.
	ErrCodeLeadCCHasCopies = "lead_cc_has_copies"
	// ErrCodeApprovalNotPending (HTTP 409): `POST /ai/sessions/:id/approve` named
	// a tool call the conversation is not waiting on: it was already decided, in
	// another tab or in Slack, or a newer message replaced it.
	ErrCodeApprovalNotPending = "approval_not_pending"
	// ErrCodeMailboxCloudUnenrollFailed (HTTP 409): The mailbox is linked to
	// Warmbly Cloud and its link could not be released, so `DELETE /emails/{id}`
	// would leave Warmbly Cloud holding its credential or its claim on it.
	ErrCodeMailboxCloudUnenrollFailed = "mailbox_cloud_unenroll_failed"
	// ErrCodeSendHoldNotFound (HTTP 404): The mailbox is not held from sending, or
	// does not exist.
	ErrCodeSendHoldNotFound = "send_hold_not_found"
	// ErrCodeSendHoldChanged (HTTP 409): The held send or reason differs from the
	// one submitted, because the hold moved or lifted since the list loaded.
	ErrCodeSendHoldChanged = "send_hold_changed"
	// ErrCodeSendHoldNotResolvable (HTTP 409): The hold cannot take a verdict
	// right now (the reason is in the message and on the row), or the evidence
	// stopped applying while it was submitted.
	ErrCodeSendHoldNotResolvable = "send_hold_not_resolvable"
	// ErrCodeAINotConfigured (HTTP 503): AI analysis is not configured on this
	// deployment.
	ErrCodeAINotConfigured = "ai_not_configured"
	// ErrCodeAttemptCheckUnavailable (HTTP 503): The per-account attempt budget
	// could not be read, so the attempt was not counted.
	ErrCodeAttemptCheckUnavailable = "attempt_check_unavailable"
	// ErrCodeCloudLinkNotManaged (HTTP 404): The mailbox is not managed by Warmbly
	// Cloud.
	ErrCodeCloudLinkNotManaged = "cloud_link_not_managed"
	// ErrCodeCloudLinkOAuthSession (HTTP 404): The cloud sign-in session is
	// unknown or has expired.
	ErrCodeCloudLinkOAuthSession = "cloud_link_oauth_session"
	// ErrCodeCodeLookupLimited (HTTP 429): Too many pairing-code lookups.
	ErrCodeCodeLookupLimited = "code_lookup_limited"
	// ErrCodeCRMOptedOut (HTTP 409): The contact opted out of email in HubSpot.
	ErrCodeCRMOptedOut = "crm_opted_out"
	// ErrCodeMailboxGmailOAuthDisabled (HTTP 403): This deployment connects new
	// Gmail mailboxes with an app password, not Google sign-in.
	ErrCodeMailboxGmailOAuthDisabled = "mailbox_gmail_oauth_disabled"
	// ErrCodeMailboxIdentityUnavailable (HTTP 503): The machine running the
	// mailbox could not be reached, so its sending addresses were not refreshed.
	ErrCodeMailboxIdentityUnavailable = "mailbox_identity_unavailable"
	// ErrCodeMailboxOAuthReturnOrigin (HTTP 400): The dashboard origin is not
	// allowed for mailbox OAuth; add its exact origin to CORS_ALLOW_ORIGINS.
	ErrCodeMailboxOAuthReturnOrigin = "mailbox_oauth_return_origin"
	// ErrCodeMailboxSendAsUnknown (HTTP 400): The address is not one the provider
	// has verified the mailbox to send as.
	ErrCodeMailboxSendAsUnknown = "mailbox_send_as_unknown"
	// ErrCodeMailboxSendAsUnsupported (HTTP 400): The mailbox provider does not
	// expose send-as addresses; only Gmail and Google Workspace do.
	ErrCodeMailboxSendAsUnsupported = "mailbox_send_as_unsupported"
	// ErrCodeMailboxSignatureTooLarge (HTTP 400): The mailbox signature is larger
	// than Warmbly stores; shorten it at the provider and import it again.
	ErrCodeMailboxSignatureTooLarge = "mailbox_signature_too_large"
	// ErrCodeMailboxTLSFailed (HTTP 400): The mail server did not present the
	// expected certificate, or the TLS handshake failed.
	ErrCodeMailboxTLSFailed = "mailbox_tls_failed"
	// ErrCodeMailboxServerDeclined (HTTP 400): The mail server declined the sign-
	// in for a reason other than the credentials (a retry-later reply, no
	// supported sign-in method, or a broken conversation).
	ErrCodeMailboxServerDeclined = "mailbox_server_declined"
	// ErrCodeMailboxValidationTimeout (HTTP 400): No worker reported back on the
	// mailbox credential check in time, so the mail server was not tested.
	ErrCodeMailboxValidationTimeout = "mailbox_validation_timeout"
	// ErrCodeSessionEnded (HTTP 401): The session was signed out or reached its
	// absolute lifetime; only a new sign-in continues.
	ErrCodeSessionEnded = "session_ended"
	// ErrCodeSlackLinkEmailMismatch (HTTP 403): The Slack account email is not the
	// one used to sign in to Warmbly.
	ErrCodeSlackLinkEmailMismatch = "slack_link_email_mismatch"
	// ErrCodeSlackNotConfigured (HTTP 503): Slack is not set up on this Warmbly
	// instance.
	ErrCodeSlackNotConfigured = "slack_not_configured"
)
