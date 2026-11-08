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
//	  "request_id": "0b6f...",
//	  "retry_after": 30
//	}
//
// "error" and "message" are for people; branch on [Error.Code] and the HTTP
// status. "retry_after" appears only where the endpoint has a wait to quote
// (rate limiting); the client also fills it from the Retry-After header.
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
	// Reason is an optional finer key a few codes carry when the same code has
	// more than one next step. Treat an unknown or empty Reason as the plain
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
)

// Further stable codes, grouped by where they surface. Codes that belong to a
// single resource family (CRM sync, mailbox vendors, placement internals, pool
// and cloud links, lead providers) are documented on the service that returns
// them; branch on [Error.HasCode] with the literal there. Treat any code not
// listed as opaque: the server adds them over time.
const (
	// ErrCodeDuplicateQueryParameter and ErrCodeDuplicateHeader are 400s for a
	// query parameter, or one of Authorization, X-Warmbly-Token,
	// Idempotency-Key and Origin, sent more than once.
	ErrCodeDuplicateQueryParameter = "duplicate_query_parameter"
	ErrCodeDuplicateHeader         = "duplicate_header"
	// ErrCodeInvalidFilter, ErrCodeInvalidSortBy, ErrCodeInvalidSetting,
	// ErrCodeInvalidTemplate and ErrCodeInvalidRecipient are 400s for a
	// filter, sort, setting, step template or recipient the server cannot
	// read. The message names the field.
	ErrCodeInvalidFilter    = "invalid_filter"
	ErrCodeInvalidSortBy    = "invalid_sort_by"
	ErrCodeInvalidSetting   = "invalid_setting"
	ErrCodeInvalidTemplate  = "invalid_template"
	ErrCodeInvalidRecipient = "invalid_recipient"
	// ErrCodeRecipientSuppressed is the 400 from a send to an address on the
	// workspace suppression list, directly or through its domain.
	ErrCodeRecipientSuppressed = "recipient_suppressed"
	// ErrCodeEmptyStepBody is the 400 from starting a campaign with an email
	// step that has no body.
	ErrCodeEmptyStepBody = "empty_step_body"
	// ErrCodeTooManyContacts, ErrCodeTooManyTasks and ErrCodeSelectionTooLarge
	// are 400s for a bulk request over its id, exclusion or selection limit.
	// Split the work into batches or narrow the filter.
	ErrCodeTooManyContacts   = "too_many_contacts"
	ErrCodeTooManyTasks      = "too_many_tasks"
	ErrCodeSelectionTooLarge = "selection_too_large"

	// Image upload refusals (400): the file is checked by its contents.
	ErrCodeImageUnreadable         = "image_unreadable"
	ErrCodeImageFormatNotAllowed   = "image_format_not_allowed"
	ErrCodeImageTooLarge           = "image_too_large"
	ErrCodeImageDimensionsTooLarge = "image_dimensions_too_large"
	ErrCodeImageTooSmall           = "image_too_small"

	// ErrCodePasswordBreached is the 400 for a password found in a public
	// breach list.
	ErrCodePasswordBreached = "password_breached"
	// ErrCodePasswordReused is the 400 for choosing the issued initial
	// password as the new one.
	ErrCodePasswordReused = "password_reused"
	// ErrCodePasswordChangeRequired is the 403 for a sign-in that must first
	// replace an issued initial password.
	ErrCodePasswordChangeRequired = "password_change_required"
	// ErrCodeLastSignInMethod is the 409 for removing the only way into an
	// account.
	ErrCodeLastSignInMethod = "last_sign_in_method"
	// ErrCodePasswordLoginDisabled is the 403 on a server that turned
	// password sign-in off.
	ErrCodePasswordLoginDisabled = "password_login_disabled"

	// ErrCodeReauthRequired is the 403 for an action that needs a recent proof
	// of identity. Confirm with the reauth endpoint, then retry.
	ErrCodeReauthRequired = "reauth_required"
	// ErrCodeMemberAccessRestricted is the 403 when the caller's
	// selected-resource scope does not cover the route.
	ErrCodeMemberAccessRestricted = "member_access_restricted"
	// ErrCodeAPIKeyPermissionsExceedCaller is the 403 for creating or editing
	// an API key with more power than its creator has.
	ErrCodeAPIKeyPermissionsExceedCaller = "api_key_permissions_exceed_caller"
	// ErrCodeAPIKeyHolderLeft is the 401 for a key whose creator is no longer
	// a member of the workspace.
	ErrCodeAPIKeyHolderLeft = "api_key_holder_left"
	// ErrCodeAPIKeyMailboxLimited is the 403 for a mailbox-limited key calling
	// a surface that acts across the whole workspace.
	ErrCodeAPIKeyMailboxLimited = "api_key_mailbox_limited"

	// ErrCodePaymentMethodRequired is the 409 for a top-up with no saved card.
	ErrCodePaymentMethodRequired = "payment_method_required"
	// ErrCodeRechargeTooSmall is the 422 for an auto-reload amount that would
	// not lift the balance past its threshold.
	ErrCodeRechargeTooSmall = "recharge_too_small"
	// ErrCodePlacementQuotaExceeded and ErrCodePlacementNotEntitled are 402s:
	// no free placement tests left this month, or a plan without them.
	ErrCodePlacementQuotaExceeded = "placement_quota_exceeded"
	ErrCodePlacementNotEntitled   = "placement_not_entitled"
	// ErrCodePlacementTooManyRunning and ErrCodePlacementTooManyBatches are
	// 429s for too many tests or batches running at once.
	ErrCodePlacementTooManyRunning = "placement_too_many_running"
	ErrCodePlacementTooManyBatches = "placement_too_many_batches"
)

// Send-admission diagnostics. These appear in campaign activity and task
// failure metadata, not as HTTP response codes.
const (
	// SendCodeCampaignDailyLimitReached: the campaign hit its UTC-day maximum
	// for this sending mailbox.
	SendCodeCampaignDailyLimitReached = "CAMPAIGN_DAILY_LIMIT_REACHED"
	// SendCodeMailboxSendingPlanDenied: the mailbox's sending plan is
	// unavailable, outside working hours or its quota is spent. The scheduler
	// retries when capacity opens.
	SendCodeMailboxSendingPlanDenied = "MAILBOX_SENDING_PLAN_DENIED"
	// SendCodeManualSendLimitReached: a unibox send would exceed the
	// mailbox's fair-use allowance.
	SendCodeManualSendLimitReached = "MANUAL_SEND_LIMIT_REACHED"
	// SendCodeRecipientSuppressed: a recipient is on the suppression list.
	SendCodeRecipientSuppressed = "RECIPIENT_SUPPRESSED"
	// SendCodeSendAdmissionDenied: another send-authority, recipient or
	// capacity check denied admission.
	SendCodeSendAdmissionDenied = "SEND_ADMISSION_DENIED"
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
