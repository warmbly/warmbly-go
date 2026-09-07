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
