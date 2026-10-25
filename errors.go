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
//	  "reason":     "certificate_untrusted",
//	  "request_id": "0b6f...",
//	  "retry_after": 30
//	}
//
// "error" and "message" are for people; branch on [Error.Code] and the HTTP
// status. "reason" appears on a few codes only, as a finer key for when one
// code has more than one next step. "retry_after" appears only where the
// endpoint has a wait to quote (rate limiting); the client also fills it from
// the Retry-After header.
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
	// Reason is an optional finer key on a few codes (for example
	// [ErrReasonCertificateUntrusted] on [ErrCodeMailboxTLSFailed]). Treat an
	// unknown or empty Reason as the plain Code.
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
// It is a status-level guess, with one correction: a 503 that says the
// deployment simply lacks a feature ([ErrCodeAINotConfigured],
// [ErrCodeSlackNotConfigured], [ErrCodeMailboxProviderNotConfigured]) is not
// transient and reports false. [ErrCodeMailboxWorkerUnreachable] is transient:
// nothing was removed, and the same call can succeed a moment later. So is
// [ErrCodeSalesforceUnreachable], although the server sends it as a 400.
func (e *Error) Temporary() bool {
	if e.HasCode(ErrCodeAINotConfigured) || e.HasCode(ErrCodeSlackNotConfigured) ||
		e.HasCode(ErrCodeMailboxProviderNotConfigured) {
		return false
	}
	if e.HasCode(ErrCodeSalesforceUnreachable) {
		return true
	}
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
// [ErrCodeLeadsUndeliverable]. The refusals added for sign-in confirmation,
// request validation, mailbox connection and import, sending domains, inbox
// placement, CRM provider mode, Salesforce, integrations and Slack are at the
// end of the block.
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
	// ErrCodeInternalError is the generic 500. Its message is normally the
	// fixed "Something went wrong." (the detail is logged server-side against
	// the request id), so do not parse it. Retry once, then quote
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

	// Sign-in, password and confirmation refusals.
	// ErrCodePasswordBreached is a 400: the password appears in a public list of breached passwords. Choose another.
	ErrCodePasswordBreached = "password_breached"
	// ErrCodeSSOLinkExpired is a 400: the pending token from a link_required single sign-on attempt is unknown, expired, used, or had three wrong passwords. Start the provider sign-in again.
	ErrCodeSSOLinkExpired = "sso_link_expired"
	// ErrCodeInvalidName is a 400: a person, workspace, company, OAuth app or mailbox display name broke the naming rules (no links, markup or control characters). The message names the field and the rule.
	ErrCodeInvalidName = "invalid_name"
	// ErrCodePasskeyUserVerificationRequired is a 400: a passkey sign-in came from an authenticator that did not verify the user with a PIN or biometric.
	ErrCodePasskeyUserVerificationRequired = "passkey_user_verification_required"
	// ErrCodeTwoFAInvalidCode is a 400: the authenticator or recovery code did not match. The login challenge allows five attempts per pending session.
	ErrCodeTwoFAInvalidCode = "two_fa_invalid_code"
	// ErrCodeReauthRequired is a 403: the action needs a proof of identity newer than the session. Confirm with Auth.Reauth, then retry. It is a 401 when the route was reached without a signed-in session at all. API keys and OAuth tokens are never asked for it.
	ErrCodeReauthRequired = "reauth_required"
	// ErrCodeReauthLimited is a 400: too many failed confirmations within the hour. A fresh sign-in counts as a confirmation for five minutes.
	ErrCodeReauthLimited = "reauth_limited"
	// ErrCodeReauthNoFactor is a 400: the account has neither a password nor two-factor authentication, so there is nothing to confirm with.
	ErrCodeReauthNoFactor = "reauth_no_factor"
	// ErrCodeReauthIdentityMismatch is a 400: the Google, Apple or SSO account verified during a confirmation is not the one linked to this account.
	ErrCodeReauthIdentityMismatch = "reauth_identity_mismatch"
	// ErrCodeReauthExpired is a 400: a confirmation's code or nonce expired or was already used. Start the confirmation again.
	ErrCodeReauthExpired = "reauth_expired"
	// ErrCodeAppAttestFailed is a 400: the native app's App Attest proof (used in place of the captcha) did not verify. Fetch a fresh challenge and prove again.
	ErrCodeAppAttestFailed = "app_attest_failed"
	// ErrCodeAppAttestKeyUnknown is a 400: the App Attest key id was never registered on this server. Register the key again, then retry.
	ErrCodeAppAttestKeyUnknown = "app_attest_key_unknown"
	// ErrCodeAppAttestUnavailable is a 404: the App Attest routes on an instance that has App Attest turned off.
	ErrCodeAppAttestUnavailable = "app_attest_unavailable"
	// ErrCodeRecipientSuppressed is a 400: an email send or unibox reply/compose named a to, cc or bcc address on the workspace suppression list, directly or through its domain. Nothing was queued. A unibox forward is never refused for this.
	ErrCodeRecipientSuppressed = "recipient_suppressed"
	// ErrCodePoolLinkMailboxConflict is a 409: enrollment into Warmbly Cloud found the address already connected there without an association for this local mailbox. Do not delete or re-enroll; contact support.
	ErrCodePoolLinkMailboxConflict = "pool_link_mailbox_conflict"
	// ErrCodeAdminMFARequired is a 403: an admin route was reached by a session that did not present a second factor.
	ErrCodeAdminMFARequired = "admin_mfa_required"
	// ErrCodePasswordChangedSignInAgain is a 409: the new password was stored but the calling device could not be given a new session. Every earlier token is invalid; sign in again.
	ErrCodePasswordChangedSignInAgain = "password_changed_sign_in_again"

	// Request validation refusals (400 unless noted).
	// ErrCodeInvalidSlug is a 400: a workspace slug that is not 2 to 80 lowercase letters, numbers or dashes starting and ending with a letter or number.
	ErrCodeInvalidSlug = "invalid_slug"
	// ErrCodeInvalidSetting is a 400: an outreach or campaign advanced setting outside the documented vocabulary.
	ErrCodeInvalidSetting = "invalid_setting"
	// ErrCodeInvalidSyncFolder is a 400: a mailbox sync folder selection the sync cannot honor: a folder it always follows, an empty, over-long or control-character name, more than 50 names, or a mailbox that is not IMAP. The message names the entry refused.
	ErrCodeInvalidSyncFolder = "invalid_sync_folder"
	// ErrCodeInvalidSortBy is a 400: a contact search, export or bulk selection sort_by of the form custom:<key> whose key could never be a custom-field name.
	ErrCodeInvalidSortBy = "invalid_sort_by"
	// ErrCodeInvalidMailHost is a 400: a contact filter mail_hosts entry that is not a documented provider value.
	ErrCodeInvalidMailHost = "invalid_mail_host"
	// ErrCodeInvalidFilter is a 400: a CRM task filter carried an id that is not one.
	ErrCodeInvalidFilter = "invalid_filter"
	// ErrCodeInvalidCursor is a 400: a list cursor this API did not issue. Start again without one.
	ErrCodeInvalidCursor = "invalid_cursor"
	// ErrCodeInvalidColumn is a 400: a view update named a column the view cannot render.
	ErrCodeInvalidColumn = "invalid_column"
	// ErrCodeDuplicateColumn is a 400: a view update named the same column twice.
	ErrCodeDuplicateColumn = "duplicate_column"
	// ErrCodeTooManyColumns is a 400: a view update named more than 64 columns.
	ErrCodeTooManyColumns = "too_many_columns"
	// ErrCodeInvalidSort is a 400: a view update sort that names neither a sortable contact column nor a well-formed custom:<key>.
	ErrCodeInvalidSort = "invalid_sort"
	// ErrCodeInvalidLayout is a 400: a view layout on a view that has none, or one with an unknown or oversized field.
	ErrCodeInvalidLayout = "invalid_layout"
	// ErrCodeUnknownView is a 404: a view name other than contacts, campaign_leads or unibox_rail.
	ErrCodeUnknownView = "unknown_view"
	// ErrCodeTooManyTasks is a 400: a CRM task update or delete named more than 1000 ids, or more than 50,000 exclusions. Split it into batches.
	ErrCodeTooManyTasks = "too_many_tasks"
	// ErrCodeTooManyContacts is a 400: a contact bulk action named more than 10,000 ids, or more than 250,000 exclusions. Split it, or send a filter selection. An integration push answers it past 500 contacts, however they were selected.
	ErrCodeTooManyContacts = "too_many_contacts"
	// ErrCodeSelectionTooLarge is a 400: an all-records bulk selection resolved to more than its limit (250,000 contacts or 50,000 CRM tasks). Nothing was changed.
	ErrCodeSelectionTooLarge = "selection_too_large"
	// ErrCodeEmptyStepBody is a 400: a campaign start found an email step with no body, which would send a blank message to every lead.
	ErrCodeEmptyStepBody = "empty_step_body"
	// ErrCodeLeadCcLimit is a 400: more than two contacts were set as a lead's CC.
	ErrCodeLeadCcLimit = "lead_cc_limit"
	// ErrCodeLeadCcSelf is a 400: a lead was named as a copy on itself.
	ErrCodeLeadCcSelf = "lead_cc_self"
	// ErrCodeLeadCcContactNotFound is a 404: a CC contact that is not in the workspace.
	ErrCodeLeadCcContactNotFound = "lead_cc_contact_not_found"
	// ErrCodeLeadCcLeadIsCopied is a 409: setting CC on a lead that is itself copied on another lead in the campaign.
	ErrCodeLeadCcLeadIsCopied = "lead_cc_lead_is_copied"
	// ErrCodeLeadCcHasCopies is a 409: a CC contact that has copies of their own in the campaign.
	ErrCodeLeadCcHasCopies = "lead_cc_has_copies"
	// ErrCodeContactEmailTaken is a 409: an updated contact email that already belongs to another contact.
	ErrCodeContactEmailTaken = "contact_email_taken"
	// ErrCodeInvalidRecipient is a 400: a to, cc or bcc entry that is not exactly one email address, or a send with no to address at all.
	ErrCodeInvalidRecipient = "invalid_recipient"
	// ErrCodeInvalidMessageID is a 400: an in_reply_to entry that is not one Message-ID.
	ErrCodeInvalidMessageID = "invalid_message_id"

	// Credential and member access refusals.
	// ErrCodeAPIKeyHolderLeft is a 401: the API key's creator is no longer a member of the workspace. The key will not work again; create a new one from a current member. The realtime gateway refuses it too.
	ErrCodeAPIKeyHolderLeft = "api_key_holder_left"
	// ErrCodeAPIKeyMailboxLimited is a 403: an API key limited to some mailboxes called a surface that acts across the workspace, such as the AI tool and MCP endpoints, a campaign sender pool resolved from mailbox tags, or a campaign start whose pool is not an explicit list of the key's own mailboxes.
	ErrCodeAPIKeyMailboxLimited = "api_key_mailbox_limited"
	// ErrCodeAPIKeyPermissionsExceedCaller is a 403: an API key was created or edited with more than its creator holds, a permission the member's roles do not cover or, for an API key caller, a permission, mailbox or IP address the calling key does not have.
	ErrCodeAPIKeyPermissionsExceedCaller = "api_key_permissions_exceed_caller"
	// ErrCodeOAuthTokenNotAllowed is a 403: an OAuth access token called a route that manages credentials (API keys and OAuth applications). Use an API key or the dashboard.
	ErrCodeOAuthTokenNotAllowed = "oauth_token_not_allowed"
	// ErrCodeMemberAccessRestricted is a 403: a member restricted to selected campaigns, folders and mailboxes, or a key or app acting for one, called a route outside that scope. Resource writes and workspace-wide surfaces such as contacts and settings are refused, and so is creating a workspace for an account with only restricted memberships.
	ErrCodeMemberAccessRestricted = "member_access_restricted"
	// ErrCodeInvalidAccessScope is a 400: a member access scope other than workspace or restricted.
	ErrCodeInvalidAccessScope = "invalid_access_scope"
	// ErrCodeInvalidAccessResource is a 400: a member access grant names a campaign folder, campaign or mailbox that is not in the workspace.
	ErrCodeInvalidAccessResource = "invalid_access_resource"
	// ErrCodeTooManyAccessGrants is a 400: a member access scope grants more campaign folders, campaigns or mailboxes than one scope may hold. The message names the limit.
	ErrCodeTooManyAccessGrants = "too_many_access_grants"
	// ErrCodeOwnerAccessUnrestricted is a 403: the workspace owner cannot be restricted to selected resources.
	ErrCodeOwnerAccessUnrestricted = "owner_access_unrestricted"
	// ErrCodeOrganizationCloudConnected is a 409: a workspace or account deletion while the workspace, or one the account owns, is still connected to Warmbly Cloud. Disconnect it first.
	ErrCodeOrganizationCloudConnected = "organization_cloud_connected"

	// Mailbox connect and sync refusals.
	// ErrCodeMailboxGmailOauthDisabled is a 403: new Gmail mailboxes connect with an app password on this deployment, not with Google sign-in.
	ErrCodeMailboxGmailOauthDisabled = "mailbox_gmail_oauth_disabled"
	// ErrCodeMailboxOAuthReturnOrigin is a 400: a mailbox OAuth connect named a dashboard origin this deployment does not allow. Add the exact origin to CORS_ALLOW_ORIGINS.
	ErrCodeMailboxOAuthReturnOrigin = "mailbox_oauth_return_origin"
	// ErrCodeAppPasswordInvalid is a 400: an app_password that is not 16 letters once spaces are removed.
	ErrCodeAppPasswordInvalid = "app_password_invalid"
	// ErrCodeMailboxNotGoogleSignin is a 409: an app-password switch on a mailbox that is not connected with per-mailbox Google sign-in.
	ErrCodeMailboxNotGoogleSignin = "mailbox_not_google_signin"
	// ErrCodeMailboxReauthDelegated is a 409: a re-authorization of a mailbox connected through an admin grant, which has no sign-in of its own.
	ErrCodeMailboxReauthDelegated = "mailbox_reauth_delegated"
	// ErrCodeMailboxValidationTimeout is a 400: the mailbox worker did not report back in time, so the mail server was not tested and nothing was saved. Retry.
	ErrCodeMailboxValidationTimeout = "mailbox_validation_timeout"
	// ErrCodeMailboxAuthRefused is a 400: the mail server refused the credentials.
	ErrCodeMailboxAuthRefused = "mailbox_auth_refused"
	// ErrCodeMailboxUnreachable is a 400: the mail server could not be reached.
	ErrCodeMailboxUnreachable = "mailbox_unreachable"
	// ErrCodeMailboxTLSFailed is a 400: the mail server did not complete a secure connection on the chosen port.
	ErrCodeMailboxTLSFailed = "mailbox_tls_failed"
	// ErrCodeMailboxServerDeclined is a 400: the sign-in was declined for a reason other than the credentials, such as a login rate limit or an unsupported mechanism.
	ErrCodeMailboxServerDeclined = "mailbox_server_declined"
	// ErrCodeMailboxSendAsUnsupported is a 400: a send-as refresh or change on a mailbox whose provider exposes no send-as list (only Gmail and Google Workspace do).
	ErrCodeMailboxSendAsUnsupported = "mailbox_send_as_unsupported"
	// ErrCodeMailboxSendAsUnknown is a 400: a send_as_email the provider has not verified for the mailbox.
	ErrCodeMailboxSendAsUnknown = "mailbox_send_as_unknown"
	// ErrCodeMailboxIdentityUnavailable is a 503: the worker holding the mailbox could not be reached, so its sending addresses were not refreshed. Retry.
	ErrCodeMailboxIdentityUnavailable = "mailbox_identity_unavailable"
	// ErrCodeMailboxSignatureTooLarge is a 400: the provider signature is larger than Warmbly stores.
	ErrCodeMailboxSignatureTooLarge = "mailbox_signature_too_large"
	// ErrCodeMailboxIsSeed is a 409: warmup was started or resumed on a placement seed inbox.
	ErrCodeMailboxIsSeed = "mailbox_is_seed"
	// ErrCodeMailboxCloudUnenrollFailed is a 409: a mailbox linked to Warmbly Cloud could not be released from it, so its deletion was refused.
	ErrCodeMailboxCloudUnenrollFailed = "mailbox_cloud_unenroll_failed"
	// ErrCodePoolLinkCleartextMailbox is a 422: a mailbox with SMTP or IMAP security none cannot be enrolled in Warmbly Cloud.
	ErrCodePoolLinkCleartextMailbox = "pool_link_cleartext_mailbox"
	// ErrCodePoolLinkRedirectNotFound is a 404: Warmbly Cloud serves no redirect for the domain for the calling instance.
	ErrCodePoolLinkRedirectNotFound = "pool_link_redirect_not_found"

	// Mailbox import, vendor and admin grant refusals.
	// ErrCodeMailboxImportEmpty is a 400: a mailbox import with no file or text, no rows, or only a header row.
	ErrCodeMailboxImportEmpty = "mailbox_import_empty"
	// ErrCodeMailboxImportTooLarge is a 400: a mailbox import over 5,000 mailboxes, a file over 10 MB, a pasted list over 2 MB, or a non-multipart body.
	ErrCodeMailboxImportTooLarge = "mailbox_import_too_large"
	// ErrCodeMailboxImportNoEmailColumn is a 400: a mailbox import mapping in which no column is email.
	ErrCodeMailboxImportNoEmailColumn = "mailbox_import_no_email_column"
	// ErrCodeMailboxImportRowIncomplete is a 400: an import row fix that left the row without servers or a password.
	ErrCodeMailboxImportRowIncomplete = "mailbox_import_row_incomplete"
	// ErrCodeMailboxImportRowNotFailed is a 409: an import row fix on a row that did not fail.
	ErrCodeMailboxImportRowNotFailed = "mailbox_import_row_not_failed"
	// ErrCodeMailboxImportCredentialsExpired is a 409: an import row retry after its credentials were deleted, 7 days after the import finished.
	ErrCodeMailboxImportCredentialsExpired = "mailbox_import_credentials_expired"
	// ErrCodeMailboxImportNothingToRetry is a 409: an import retry that matched no failed row still holding its credentials.
	ErrCodeMailboxImportNothingToRetry = "mailbox_import_nothing_to_retry"
	// ErrCodeMailboxVendorUnauthorized is a 400: the inbox vendor did not accept the API key.
	ErrCodeMailboxVendorUnauthorized = "mailbox_vendor_unauthorized"
	// ErrCodeMailboxVendorNoWorkspace is a 400: the vendor accepted the key but lists no workspace for it.
	ErrCodeMailboxVendorNoWorkspace = "mailbox_vendor_no_workspace"
	// ErrCodeMailboxVendorInvalidFields is a 400: a vendor field is empty or holds control characters.
	ErrCodeMailboxVendorInvalidFields = "mailbox_vendor_invalid_fields"
	// ErrCodeMailboxVendorUnknown is a 400: a vendor that is not one of the supported vendors.
	ErrCodeMailboxVendorUnknown = "mailbox_vendor_unknown"
	// ErrCodeMailboxVendorRateLimited is a 429: the vendor is rate limiting Warmbly's requests. Retry in a minute.
	ErrCodeMailboxVendorRateLimited = "mailbox_vendor_rate_limited"
	// ErrCodeMailboxVendorUnavailable is a 400: the vendor answered unexpectedly, did not answer, or no longer has the mailbox.
	ErrCodeMailboxVendorUnavailable = "mailbox_vendor_unavailable"
	// ErrCodeMailboxVendorDomainNotFound is a 404: a vendor forwarding or tracking change on a domain none of the workspace's vendor accounts holds.
	ErrCodeMailboxVendorDomainNotFound = "mailbox_vendor_domain_not_found"
	// ErrCodeMailboxVendorDomainUnsupported is a 400: the vendor holding the domain cannot do this through its API.
	ErrCodeMailboxVendorDomainUnsupported = "mailbox_vendor_domain_unsupported"
	// ErrCodeMailboxGrantNotConfigured is a 400: this instance has no Google service account or Microsoft app for admin grants.
	ErrCodeMailboxGrantNotConfigured = "mailbox_grant_not_configured"
	// ErrCodeGoogleDelegationUnauthorized is a 400: Google refused the domain-wide delegation.
	ErrCodeGoogleDelegationUnauthorized = "google_delegation_unauthorized"
	// ErrCodeMicrosoftConsentMissing is a 400: the Microsoft 365 organization has not granted admin consent, or the consent lacks a required permission.
	ErrCodeMicrosoftConsentMissing = "microsoft_consent_missing"
	// ErrCodeMailboxGrantStateInvalid is a 400: the grant sign-in or consent state expired, was used, or belongs to another member or workspace.
	ErrCodeMailboxGrantStateInvalid = "mailbox_grant_state_invalid"
	// ErrCodeMailboxGrantProofMissing is a 400: the workspace has not proved it controls the domain, or the sign-in was not the domain's administrator.
	ErrCodeMailboxGrantProofMissing = "mailbox_grant_proof_missing"
	// ErrCodeMailboxGrantUnavailable is a 503: Google, Microsoft or the DNS lookup did not answer. Nothing was recorded. Retry.
	ErrCodeMailboxGrantUnavailable = "mailbox_grant_unavailable"
	// ErrCodeMailboxGrantDomainMismatch is a 400: the admin address or mailbox is on a domain the grant does not cover.
	ErrCodeMailboxGrantDomainMismatch = "mailbox_grant_domain_mismatch"
	// ErrCodeMailboxGrantMailboxUnreachable is a 400: Google or Microsoft has no usable mailbox for the account.
	ErrCodeMailboxGrantMailboxUnreachable = "mailbox_grant_mailbox_unreachable"
	// ErrCodeMailboxGrantInactive is a 400: the grant failed its last check, so it connects nothing.
	ErrCodeMailboxGrantInactive = "mailbox_grant_inactive"

	// Sending domain and redirect refusals.
	// ErrCodeSendingDomainNotInWorkspace is a 404: the workspace has no mailbox on the domain (400 when no domain was given).
	ErrCodeSendingDomainNotInWorkspace = "sending_domain_not_in_workspace"
	// ErrCodeSendingDomainSharedProvider is a 400: the domain belongs to a shared email provider, such as gmail.com, and cannot be redirected.
	ErrCodeSendingDomainSharedProvider = "sending_domain_shared_provider"
	// ErrCodeSendingDomainBulkInvalid is a 400: a bulk domain setup that named no domain, more than 100, nothing to set, a bad tracking_label, or a bad served_by.
	ErrCodeSendingDomainBulkInvalid = "sending_domain_bulk_invalid"
	// ErrCodeDomainRedirectInvalidTarget is a 400: a redirect target that is not an http or https address on a host, or points back at the domain.
	ErrCodeDomainRedirectInvalidTarget = "domain_redirect_invalid_target"
	// ErrCodeDomainRedirectTaken is a 409: another workspace on this instance already serves a verified redirect for the domain.
	ErrCodeDomainRedirectTaken = "domain_redirect_taken"
	// ErrCodeDomainRedirectLinked is a 409: the domain's redirect is served for a linked self-hosted instance, and only that instance changes it.
	ErrCodeDomainRedirectLinked = "domain_redirect_linked"
	// ErrCodeDomainRedirectCloudUnavailable is a 409: served_by is cloud, but this instance is not linked to Warmbly Cloud or Cloud does not serve redirects for it.
	ErrCodeDomainRedirectCloudUnavailable = "domain_redirect_cloud_unavailable"
	// ErrCodeDomainRedirectCloudUnreachable is a 503: Warmbly Cloud could not be reached, so nothing changed. Retry.
	ErrCodeDomainRedirectCloudUnreachable = "domain_redirect_cloud_unreachable"
	// ErrCodeDomainRedirectLimit is a 409: Warmbly Cloud already serves 200 redirects for the linked instance.
	ErrCodeDomainRedirectLimit = "domain_redirect_limit"
	// ErrCodeTrackingHostNotConfigured is a 400: this instance has no tracking host, so it cannot serve a tracking domain or redirect.
	ErrCodeTrackingHostNotConfigured = "tracking_host_not_configured"
	// ErrCodeTrackingDomainTaken is a 409: another workspace on this instance already uses the tracking domain. Choose a host on a domain the workspace sends from.
	ErrCodeTrackingDomainTaken = "tracking_domain_taken"

	// Inbox placement refusals.
	// ErrCodePlacementNotEntitled is a 402: the workspace has no active trial or subscription, so it cannot start a placement test.
	ErrCodePlacementNotEntitled = "placement_not_entitled"
	// ErrCodePlacementQuotaExceeded is a 402: the free placement tests for the month are used up. The message may name a price in credits payable with max_credits.
	ErrCodePlacementQuotaExceeded = "placement_quota_exceeded"
	// ErrCodePlacementTooManyRunning is a 429: three placement tests are already running.
	ErrCodePlacementTooManyRunning = "placement_too_many_running"
	// ErrCodePlacementTooManyBatches is a 429: five placement batches are already running.
	ErrCodePlacementTooManyBatches = "placement_too_many_batches"
	// ErrCodePlacementBatchEmpty is a 400: a placement batch resolved to no sending mailbox.
	ErrCodePlacementBatchEmpty = "placement_batch_empty"
	// ErrCodePlacementBatchTooLarge is a 400: a placement batch would hold more mailboxes than the instance allows.
	ErrCodePlacementBatchTooLarge = "placement_batch_too_large"
	// ErrCodePlacementBatchNotRunning is a 409: a cancel on a placement batch that has already finished.
	ErrCodePlacementBatchNotRunning = "placement_batch_not_running"
	// ErrCodePlacementNotRunning is a 409: a cancel on a placement test that has already finished.
	ErrCodePlacementNotRunning = "placement_not_running"
	// ErrCodePlacementDailyBudget is a 409: the mailbox has too little of its daily limit left for a useful test.
	ErrCodePlacementDailyBudget = "placement_daily_budget"
	// ErrCodePlacementInvalidSeeds is a 400: seed_ids names a mailbox that is not a connected, running seed inbox of the workspace.
	ErrCodePlacementInvalidSeeds = "placement_invalid_seeds"
	// ErrCodePlacementInvalidTracking is a 400: tracking is on or compare for a plain-text campaign.
	ErrCodePlacementInvalidTracking = "placement_invalid_tracking"
	// ErrCodePlacementNoSeeds is a 409: the panel has no usable seed inbox.
	ErrCodePlacementNoSeeds = "placement_no_seeds"
	// ErrCodePlacementPanelUnavailable is a 409: panel is cloud on an instance that is not linked to Warmbly Cloud.
	ErrCodePlacementPanelUnavailable = "placement_panel_unavailable"
	// ErrCodePlacementSeedLimit is a 409: the workspace already has 50 seed inboxes.
	ErrCodePlacementSeedLimit = "placement_seed_limit"
	// ErrCodePlacementSeedUnavailable is a 409: the mailbox cannot be marked as a seed inbox right now.
	ErrCodePlacementSeedUnavailable = "placement_seed_unavailable"
	// ErrCodePlacementSenderBusy is a 409: the sending mailbox still has copies of another test waiting to be sent.
	ErrCodePlacementSenderBusy = "placement_sender_busy"
	// ErrCodePlacementSenderUnavailable is a 409: the sending mailbox is not connected and active, or is itself a seed inbox.
	ErrCodePlacementSenderUnavailable = "placement_sender_unavailable"

	// CRM provider mode refusals (HubSpot and Pipedrive).
	// ErrCodeCRMNotConnected is a 409: a provider-mode call while the workspace runs on Warmbly's own CRM. Connect HubSpot or Pipedrive and choose it with CRM.UpdateSettings first. Some calls answer it as a 404 once the mode was switched off.
	ErrCodeCRMNotConnected = "crm_not_connected"
	// ErrCodeCRMManagedExternally is a 409: an edit to something the connected CRM owns, such as its pipelines or task types.
	ErrCodeCRMManagedExternally = "crm_managed_externally"
	// ErrCodeCRMReauthRequired is a 409: the provider connection was revoked or lacks a permission. Reconnect it under Integrations.
	ErrCodeCRMReauthRequired = "crm_reauth_required"
	// ErrCodeCRMContactMissing is a 404: the provider does not have the contact yet. CRM.LinkContactRecord finds or creates it.
	ErrCodeCRMContactMissing = "crm_contact_missing"
	// ErrCodeCRMRecordMissing is a 404: the record was deleted in the provider.
	ErrCodeCRMRecordMissing = "crm_record_missing"
	// ErrCodeCRMOptedOut is a 409: the contact unsubscribed in the provider.
	ErrCodeCRMOptedOut = "crm_opted_out"
	// ErrCodeCRMOwnerUnmapped is a 400: an owner id the provider does not know.
	ErrCodeCRMOwnerUnmapped = "crm_owner_unmapped"
	// ErrCodeCRMStageUnknown is a 400 or 409: a pipeline or stage the provider does not have.
	ErrCodeCRMStageUnknown = "crm_stage_unknown"
	// ErrCodeCRMProviderRejected is a 422: the provider refused the change. The message carries its reason.
	ErrCodeCRMProviderRejected = "crm_provider_rejected"
	// ErrCodeCRMUnavailable is a 503: the provider did not answer. Retry shortly.
	ErrCodeCRMUnavailable = "crm_unavailable"
	// ErrCodeCRMSyncRunning is a 429: a sync was requested while one has just started.
	ErrCodeCRMSyncRunning = "crm_sync_running"

	// Salesforce and integration refusals.
	// ErrCodeSalesforceReconnectRequired is a 409: the Salesforce connection's access expired or was revoked. Reconnect it.
	ErrCodeSalesforceReconnectRequired = "salesforce_reconnect_required"
	// ErrCodeSalesforceRateLimited is a 429: the Salesforce org has used its daily API requests.
	ErrCodeSalesforceRateLimited = "salesforce_rate_limited"
	// ErrCodeSalesforceError is a 400: Salesforce refused the call. The message carries its reason.
	ErrCodeSalesforceError = "salesforce_error"
	// ErrCodeSalesforceUnreachable is a 400: Salesforce did not answer. Retry shortly.
	ErrCodeSalesforceUnreachable = "salesforce_unreachable"
	// ErrCodeSalesforceSyncOff is a 400: Salesforce.SyncNow while the connection's sync is switched off.
	ErrCodeSalesforceSyncOff = "salesforce_sync_off"
	// ErrCodeInvalidSalesforceSettings is a 400: Salesforce sync settings the sync cannot run. The message names the field.
	ErrCodeInvalidSalesforceSettings = "invalid_salesforce_settings"
	// ErrCodeInvalidSalesforceDomain is a 400: an OAuth start whose Salesforce My Domain is not a usable login host.
	ErrCodeInvalidSalesforceDomain = "invalid_salesforce_domain"
	// ErrCodeImportRunning is a 409: a run of a Salesforce import source that is already running.
	ErrCodeImportRunning = "import_running"
	// ErrCodeActionProviderMismatch is a 400: an event subscription or automation step whose action belongs to another provider than the connection it runs on.
	ErrCodeActionProviderMismatch = "action_provider_mismatch"

	// Service availability codes.
	// ErrCodeAINotConfigured is a 503: the deployment has no AI provider set up. Not transient; retrying will not help.
	ErrCodeAINotConfigured = "ai_not_configured"
	// ErrCodeSlackNotConfigured is a 503: this instance is not set up for Slack. Not transient.
	ErrCodeSlackNotConfigured = "slack_not_configured"
	// ErrCodeSlackNotConnected is a 404: a Slack route was called for a workspace that has not connected Slack.
	ErrCodeSlackNotConnected = "slack_not_connected"
	// ErrCodeSlackLinkInvalid is a 404: the Slack link code is unknown, expired or already used.
	ErrCodeSlackLinkInvalid = "slack_link_invalid"
	// ErrCodeSlackNotLinked is a 404: the member has not linked a Slack account in this workspace.
	ErrCodeSlackNotLinked = "slack_not_linked"
)

// Refusals added by the third API sync: sign-in and session state, account and
// workspace standing, mailbox certificates, request hygiene, uploads,
// invitations, redirects, and the Warmbly Cloud link.
const (
	// ErrCodeSessionEnded is a 401: the session behind the token was signed
	// out, revoked, or ended by a password change. Refreshing will not help;
	// sign in again.
	ErrCodeSessionEnded = "session_ended"
	// ErrCodeAttemptCheckUnavailable is a 503 with Retry-After: the attempt
	// budget of a password, code or sign-in check could not be read, so nothing
	// was compared and the attempt was not counted. Retry after
	// [Error.RetryAfter]. A CLI or Warmbly Cloud pairing-code lookup answers
	// the same code when its lookup budget cannot be read.
	ErrCodeAttemptCheckUnavailable = "attempt_check_unavailable"
	// ErrCodeCodeLookupLimited is a 429 with Retry-After: 60: too many
	// pairing-code lookups, approvals or denials for the signed-in member.
	ErrCodeCodeLookupLimited = "code_lookup_limited"
	// ErrCodeResetLinkInvalid is a 401: the reset link or reset session from
	// [AuthService.ResetPasswordSession] or the confirm step expired, was
	// already used, or was issued before the password last changed. Request a
	// new link.
	ErrCodeResetLinkInvalid = "reset_link_invalid"
	// ErrCodeCredentialRevokeUnsupported is a 400: revoke_api_credentials was
	// sent to a server that cannot honor it. The password was not changed;
	// send the request again without it.
	ErrCodeCredentialRevokeUnsupported = "api_credentials_revoke_unsupported"
	// ErrCodeSessionsNotEnded is a 503 with Retry-After: the other sessions
	// could not be signed out, so the password was not changed. Retry.
	ErrCodeSessionsNotEnded = "sessions_not_ended"

	// Account and workspace standing. These are not permission problems and a
	// role change does not lift them.
	// ErrCodeAccountSuspended is a 403: the account is suspended and cannot
	// sign in.
	ErrCodeAccountSuspended = "account_suspended"
	// ErrCodeAccountSendingSuspended is a 403: the account may not send mail;
	// a manual send is refused and nothing else is affected.
	ErrCodeAccountSendingSuspended = "account_sending_suspended"
	// ErrCodeAccountWorkspaceCreateSuspended is a 403: the account may not
	// create workspaces. It can still join one it is invited to.
	ErrCodeAccountWorkspaceCreateSuspended = "account_workspace_create_suspended"
	// ErrCodeWorkspaceSendingSuspended is a 403: sending is paused for the
	// whole workspace while it is under review.
	ErrCodeWorkspaceSendingSuspended = "workspace_sending_suspended"

	// Request hygiene.
	// ErrCodeDuplicateQueryParameter is a 400: a query parameter appeared more
	// than once on a route that reads it as a single value. The message names
	// it.
	ErrCodeDuplicateQueryParameter = "duplicate_query_parameter"
	// ErrCodeDuplicateHeader is a 400: Authorization, X-Warmbly-Token,
	// Idempotency-Key or Origin was sent on more than one header line.
	ErrCodeDuplicateHeader = "duplicate_header"

	// Mailbox certificates.
	// ErrCodeInvalidTLSPin is a 400: a tls_pin that is not a SHA-256
	// fingerprint (64 hexadecimal characters), or a certificate confirmation
	// missing the fingerprint of a server the mailbox trusts on its own
	// certificate.
	ErrCodeInvalidTLSPin = "invalid_tls_pin"
	// ErrCodeMailboxCertificateNotPinned is a 400: a certificate confirmation
	// on a mailbox whose servers are both verified by a certificate authority,
	// so there is nothing to confirm.
	ErrCodeMailboxCertificateNotPinned = "mailbox_certificate_not_pinned"

	// Image uploads (avatars, app logos, form images, email images), checked
	// by content rather than by file name.
	// ErrCodeImageUnreadable is a 400: the file is empty or does not decode as
	// an image.
	ErrCodeImageUnreadable = "image_unreadable"
	// ErrCodeImageFormatNotAllowed is a 400: an image, but not a format this
	// upload takes.
	ErrCodeImageFormatNotAllowed = "image_format_not_allowed"
	// ErrCodeImageTooLarge is a 400: the file is over the size limit.
	ErrCodeImageTooLarge = "image_too_large"
	// ErrCodeImageDimensionsTooLarge is a 400: the image is wider or taller
	// than the limit.
	ErrCodeImageDimensionsTooLarge = "image_dimensions_too_large"
	// ErrCodeImageTooSmall is a 400: an OAuth app logo under 32px on a side.
	ErrCodeImageTooSmall = "image_too_small"

	// ErrCodeRedirectTargetNotOwnDomain is a 400: a form's redirect_url, a
	// sending domain's redirect target_url, or a vendor forwarding url is not
	// on one of the workspace's own domains.
	ErrCodeRedirectTargetNotOwnDomain = "redirect_target_not_own_domain"

	// Invitations accepted late.
	// ErrCodeInvitationExpired is a 400: accepting an invitation older than 48
	// hours. It stays on the team page, where it can be sent again.
	ErrCodeInvitationExpired = "invitation_expired"
	// ErrCodeInvitationUsedOrExpired is a 400: the invitation was accepted or
	// expired while the request was in flight.
	ErrCodeInvitationUsedOrExpired = "invitation_used_or_expired"
	// ErrCodeInvitationRolesRemoved is a 400: every role on the invitation was
	// deleted after it was sent. Ask for a new one.
	ErrCodeInvitationRolesRemoved = "invitation_roles_removed"

	// CLI sign-in (warmbly auth login).
	// ErrCodeCLIAuthRequest is a 400: a CLI poll without a device_code.
	ErrCodeCLIAuthRequest = "cli_auth_request"
	// ErrCodeCLIAuthScopes is a 400: the CLI asked for scopes that include
	// permission bits this instance does not grant.
	ErrCodeCLIAuthScopes = "cli_auth_scopes"
	// ErrCodeCLIAuthCodeNotFound is a 404: the code is unknown or has expired.
	ErrCodeCLIAuthCodeNotFound = "cli_auth_code_not_found"
	// ErrCodeCLIAuthCodeUsed is a 409: the code has already been used.
	ErrCodeCLIAuthCodeUsed = "cli_auth_code_used"
	// ErrCodeCLIAuthForbidden is a 403: approving a CLI sign-in needs the
	// manage-API-keys permission in the workspace, because it mints a key.
	ErrCodeCLIAuthForbidden = "cli_auth_forbidden"

	// Warmbly Cloud link, as raised on a self-hosted instance.
	// ErrCodeCloudLinkConnected is a 409: the workspace is already connected
	// to Warmbly Cloud.
	ErrCodeCloudLinkConnected = "cloud_link_connected"
	// ErrCodeCloudLinkNotConnected is a 409: there is no connection to change
	// or disconnect.
	ErrCodeCloudLinkNotConnected = "cloud_link_not_connected"
	// ErrCodeCloudLinkNoKey is a 409: the instance has no
	// CREDENTIALS_ENCRYPTION_KEY, so it cannot store the Cloud token.
	ErrCodeCloudLinkNoKey = "cloud_link_no_key"
	// ErrCodeCloudLinkURL is a 400: the Cloud URL does not start with https://.
	ErrCodeCloudLinkURL = "cloud_link_url"
	// ErrCodeCloudLinkNoPending is a 404: no connection is in progress for the
	// workspace. Start it again.
	ErrCodeCloudLinkNoPending = "cloud_link_no_pending"
	// ErrCodeCloudLinkCodeExpired is a 404: the connection code expired before
	// it was approved in Warmbly Cloud.
	ErrCodeCloudLinkCodeExpired = "cloud_link_code_expired"
	// ErrCodeCloudLinkMailboxInactive is a 422: only active mailboxes can be
	// enrolled in Warmbly Cloud warmup.
	ErrCodeCloudLinkMailboxInactive = "cloud_link_mailbox_inactive"
	// ErrCodeCloudLinkOAuthMailbox is a 422: a Google or Microsoft mailbox
	// connected with the instance's own OAuth app cannot be warmed by Cloud,
	// because its refresh grant is bound to that app.
	ErrCodeCloudLinkOAuthMailbox = "cloud_link_oauth_mailbox"
	// ErrCodeCloudLinkLifecycle is a 400: a pause or resume request named an
	// action other than pause or resume.
	ErrCodeCloudLinkLifecycle = "cloud_link_lifecycle"
	// ErrCodeCloudLinkDisconnecting is a 409: the workspace is disconnecting
	// from Warmbly Cloud. Retry once it has finished.
	ErrCodeCloudLinkDisconnecting = "cloud_link_disconnecting"
	// ErrCodeCloudLinkUnreachable is a 503: Warmbly Cloud could not be reached.
	ErrCodeCloudLinkUnreachable = "cloud_link_unreachable"
	// ErrCodeCloudLinkBadResponse is a 503: Warmbly Cloud answered without the
	// mailbox's warmup settings.
	ErrCodeCloudLinkBadResponse = "cloud_link_bad_response"
	// ErrCodeCloudLinkSMTPAssociationProtocol is a 409: Cloud does not
	// advertise smtp_association_protocol 1. Nothing was changed.
	ErrCodeCloudLinkSMTPAssociationProtocol = "cloud_link_smtp_association_protocol"
	// ErrCodeCloudLinkSMTPAssociation is a 409: the selected existing Cloud
	// SMTP/IMAP mailbox could not be associated. Do not delete it or replace
	// passwords to repair the association.
	ErrCodeCloudLinkSMTPAssociation = "cloud_link_smtp_association"
	// ErrCodeCloudWarmupValueUnsupported is a 422: Warmbly Cloud does not take
	// 0 from an instance for the starting volume, maximum, daily increase or
	// reply rate.
	ErrCodeCloudWarmupValueUnsupported = "cloud_warmup_value_unsupported"
	// ErrCodeCloudWarmupNotApplied is a 409: Cloud answered but did not apply
	// the named settings. The mailbox now shows the values Cloud reported. The
	// rest of the patch was not applied; retry with a new idempotency key.
	ErrCodeCloudWarmupNotApplied = "cloud_warmup_not_applied"
	// ErrCodeCloudWarmupManaged is a 409: participation, the shared daily and
	// recipient limits, and stopping warmup are changed with the mailbox's
	// Warmbly Cloud controls, not on the mailbox.
	ErrCodeCloudWarmupManaged = "cloud_warmup_managed"

	// Pool link refusals from Warmbly Cloud's side, passed through by a
	// self-hosted instance.
	// ErrCodePoolLinkRequest is a 400: the connection request is missing the
	// instance name.
	ErrCodePoolLinkRequest = "pool_link_request"
	// ErrCodePoolLinkCredential is a 400: enrolling a mailbox needs a
	// credential that matches its provider.
	ErrCodePoolLinkCredential = "pool_link_credential"
	// ErrCodePoolLinkCodeNotFound is a 404: the connection code is unknown or
	// has expired.
	ErrCodePoolLinkCodeNotFound = "pool_link_code_not_found"
	// ErrCodePoolLinkCodeUsed is a 409: the connection code was already used.
	ErrCodePoolLinkCodeUsed = "pool_link_code_used"
	// ErrCodePoolLinkDenied is a 403: the connection was declined in Cloud.
	ErrCodePoolLinkDenied = "pool_link_denied"
	// ErrCodePoolLinkRevoked is a 401: the instance's link was revoked.
	// Reconnect from the instance's settings.
	ErrCodePoolLinkRevoked = "pool_link_revoked"
	// ErrCodePoolLinkInstanceNotFound is a 404: the linked instance no longer
	// exists in Cloud.
	ErrCodePoolLinkInstanceNotFound = "pool_link_instance_not_found"
	// ErrCodePoolLinkMailboxNotFound is a 404: the mailbox is not enrolled.
	ErrCodePoolLinkMailboxNotFound = "pool_link_mailbox_not_found"
	// ErrCodePoolLinkMailboxLimit is a 402: the Cloud workspace used its free
	// allowance of linked mailboxes.
	ErrCodePoolLinkMailboxLimit = "pool_link_mailbox_limit"
	// ErrCodePoolLinkAlreadyAdopted is a 409: the mailbox is already linked to
	// an instance.
	ErrCodePoolLinkAlreadyAdopted = "pool_link_already_adopted"
	// ErrCodePoolLinkNotAdoptable is a 422: the Cloud mailbox cannot be linked
	// (inactive, an unsupported provider, or a Microsoft whole-organization
	// grant).
	ErrCodePoolLinkNotAdoptable = "pool_link_not_adoptable"
	// ErrCodePoolLinkNotManaged is a 403: the credential is held by the
	// instance, not by Cloud, so Cloud cannot act on it.
	ErrCodePoolLinkNotManaged = "pool_link_not_managed"
	// ErrCodePoolLinkMailboxBlocked is a 403: Cloud blocked the mailbox for
	// hurting the pool; sending from it is suspended until reviewed.
	ErrCodePoolLinkMailboxBlocked = "pool_link_mailbox_blocked"
	// ErrCodePoolLinkMailboxInactive is a 403: the mailbox is not active in
	// Cloud. Reconnect it to keep sending.
	ErrCodePoolLinkMailboxInactive = "pool_link_mailbox_inactive"
	// ErrCodePoolLinkWarmupUnavailable is a 403: warmup is not available on
	// the Cloud workspace.
	ErrCodePoolLinkWarmupUnavailable = "pool_link_warmup_unavailable"
	// ErrCodePoolLinkOAuthPending is a 409: a mailbox sign-in started through
	// Cloud has not completed yet. Poll again.
	ErrCodePoolLinkOAuthPending = "pool_link_oauth_pending"
	// ErrCodePoolLinkOAuthSession is a 404: the mailbox sign-in session is
	// unknown or has expired.
	ErrCodePoolLinkOAuthSession = "pool_link_oauth_session"
)

// Values of [Error.Reason].
const (
	// ErrReasonCertificateUntrusted accompanies [ErrCodeMailboxTLSFailed] when
	// the server's own certificate failed verification and the owner can
	// resolve it by trusting that certificate (tls_unverified on the
	// credentials, then [EmailService.ConfirmCertificates]).
	ErrReasonCertificateUntrusted = "certificate_untrusted"
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
