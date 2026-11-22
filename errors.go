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
	if msg == "" && e.StatusCode == statusClientClosedRequest {
		msg = "Client Closed Request"
	}
	if msg == "" {
		msg = "unexpected status"
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

// statusClientClosedRequest is the status the server records, and answers
// with no body, when the client gave up on the request before it finished.
const statusClientClosedRequest = 499

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
	// allowance; retrying changes nothing. An AI action can also answer 402
	// with [ErrCodeAIAllowanceUsed] or [ErrCodeAIOverageLimitReached].
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

	// ErrCodePaymentMethodRequired is the 409 for a top-up with no saved card,
	// and the 402 for a store order the balance does not cover with no saved
	// card.
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

	// Codes added with the agency, Partnership connect, client report, tool
	// migration and branding surfaces. The trailing comment is the HTTP status.

	// Agency endpoints and agency API keys.
	ErrCodeAgencyAIBalanceInsufficient    = "agency_ai_balance_insufficient"    // 402
	ErrCodeAgencyAlreadySubscribed        = "agency_already_subscribed"         // 409
	ErrCodeAgencyAPIKeyNotFound           = "agency_api_key_not_found"          // 404
	ErrCodeAgencyBilled                   = "agency_billed"                     // 409
	ErrCodeAgencyBillingDisabled          = "agency_billing_disabled"           // 503
	ErrCodeAgencyClientHasSubscription    = "agency_client_has_subscription"    // 409
	ErrCodeAgencyClientInboxForbidden     = "agency_client_inbox_forbidden"     // 403
	ErrCodeAgencyClientMailboxesForbidden = "agency_client_mailboxes_forbidden" // 403
	ErrCodeAgencyClientNotFound           = "agency_client_not_found"           // 404
	ErrCodeAgencyClientNotOwner           = "agency_client_not_owner"           // 403
	ErrCodeAgencyClientTaken              = "agency_client_taken"               // 409
	ErrCodeAgencyCredentialRequired       = "agency_credential_required"        // 403
	ErrCodeAgencyForbidden                = "agency_forbidden"                  // 403
	ErrCodeAgencyKeyHolderLeft            = "agency_key_holder_left"            // 401
	ErrCodeAgencyKeyIPNotAllowed          = "agency_key_ip_not_allowed"         // 403
	ErrCodeAgencyKeyNotAllowed            = "agency_key_not_allowed"            // 403
	ErrCodeAgencyKeyNotClient             = "agency_key_not_client"             // 403
	ErrCodeAgencyKeyOrganizationRequired  = "agency_key_organization_required"  // 400
	ErrCodeAgencyLeaveAgencyOwner         = "agency_leave_agency_owner"         // 409
	ErrCodeAgencyLeaveNotOwner            = "agency_leave_not_owner"            // 403
	ErrCodeAgencyLimitReached             = "agency_limit_reached"              // 403
	ErrCodeAgencyNoPaymentMethod          = "agency_no_payment_method"          // 409
	ErrCodeAgencyNoSubscription           = "agency_no_subscription"            // 400
	ErrCodeAgencyNotFound                 = "agency_not_found"                  // 404
	ErrCodeAgencyOwnerFixed               = "agency_owner_fixed"                // 403
	ErrCodeAgencyPlanPreviewExpired       = "agency_plan_preview_expired"       // 409
	ErrCodeAgencyPlanUnavailable          = "agency_plan_unavailable"           // 400
	ErrCodeAgencyPlanUnchanged            = "agency_plan_unchanged"             // 409
	ErrCodeAgencyPoolExceedsPlan          = "agency_pool_exceeds_plan"          // 409
	ErrCodeAgencyPoolUnlimited            = "agency_pool_unlimited"             // 400
	ErrCodeAgencyStaffNotFound            = "agency_staff_not_found"            // 404
	ErrCodeAgencySubscriptionActive       = "agency_subscription_active"        // 409
	ErrCodeAgencyVendorManaged            = "agency_vendor_managed"             // 409
	ErrCodeAgencyVendorStaffOnly          = "agency_vendor_staff_only"          // 403
	ErrCodeInvalidAgencyKeyPermissions    = "invalid_agency_key_permissions"    // 400
	ErrCodeInvalidAgencyRole              = "invalid_agency_role"               // 400
	ErrCodeInvalidAIAmount                = "invalid_ai_amount"                 // 400
	ErrCodeInvalidAIAutoTopup             = "invalid_ai_auto_topup"             // 400
	ErrCodeInvalidAutoTopup               = "invalid_auto_topup"                // 400
	ErrCodeInvalidMonthlyAILimit          = "invalid_monthly_ai_limit"          // 400
	ErrCodeInvalidMonthlySendLimit        = "invalid_monthly_send_limit"        // 400

	// Client connect links (/emails/client-links) and the pages they open.
	ErrCodeClientLinkCaptchaFailed       = "client_link_captcha_failed"        // 400
	ErrCodeClientLinkExhausted           = "client_link_exhausted"             // 409
	ErrCodeClientLinkExpired             = "client_link_expired"               // 403
	ErrCodeClientLinkInvalidExpiry       = "client_link_invalid_expiry"        // 400
	ErrCodeClientLinkInvalidMaxMailboxes = "client_link_invalid_max_mailboxes" // 400
	ErrCodeClientLinkInvalidProviders    = "client_link_invalid_providers"     // 400
	ErrCodeClientLinkInvalidTags         = "client_link_invalid_tags"          // 400
	ErrCodeClientLinkNoMailboxes         = "client_link_no_mailboxes"          // 400
	ErrCodeClientLinkNotFound            = "client_link_not_found"             // 404
	ErrCodeClientLinkProviderNotAllowed  = "client_link_provider_not_allowed"  // 400
	ErrCodeClientLinkProviderUnavailable = "client_link_provider_unavailable"  // 400
	ErrCodeClientLinkRateLimited         = "client_link_rate_limited"          // 429
	ErrCodeClientLinkRevoked             = "client_link_revoked"               // 403
	ErrCodeClientLinkSigninDeclined      = "client_link_signin_declined"       // 400
	ErrCodeClientLinkSigninExpired       = "client_link_signin_expired"        // 400
	ErrCodeClientLinkTooManyMailboxes    = "client_link_too_many_mailboxes"    // 409
	ErrCodeClientLinksUnavailable        = "client_links_unavailable"          // 503

	// Warmbly Cloud pool link enrollment.
	ErrCodeCloudLinkCredentialRequired = "cloud_link_credential_required" // 409
	ErrCodeCloudLinkExecutor           = "cloud_link_executor"            // 400
	ErrCodePoolLinkExecutor            = "pool_link_executor"             // 400
	ErrCodePoolLinkExecutorConflict    = "pool_link_executor_conflict"    // 409

	// The mailbox connect API (/emails/connect/...), Partnership plan only.
	ErrCodeConnectGrantAppRequired       = "connect_grant_app_required"        // 422
	ErrCodeConnectGrantInput             = "connect_grant_input"               // 400
	ErrCodeConnectGrantMailboxNotAllowed = "connect_grant_mailbox_not_allowed" // 403
	ErrCodeConnectGrantOffline           = "connect_grant_offline"             // 422
	ErrCodeConnectGrantRedirectURI       = "connect_grant_redirect_uri"        // 400
	ErrCodeConnectGrantRefused           = "connect_grant_refused"             // 422
	ErrCodeConnectGrantWrongMailbox      = "connect_grant_wrong_mailbox"       // 403
	ErrCodeConnectLinkBrowser            = "connect_link_browser"              // 403
	ErrCodeConnectLinkCreatorLeft        = "connect_link_creator_left"         // 403
	ErrCodeConnectLinkDeclined           = "connect_link_declined"             // 400
	ErrCodeConnectLinkEmail              = "connect_link_email"                // 400
	ErrCodeConnectLinkExpiry             = "connect_link_expiry"               // 400
	ErrCodeConnectLinkNotFound           = "connect_link_not_found"            // 404
	ErrCodeConnectLinkProvider           = "connect_link_provider"             // 400
	ErrCodeConnectLinkReturnURL          = "connect_link_return_url"           // 400
	ErrCodeConnectLinkUnavailable        = "connect_link_unavailable"          // 404
	ErrCodeConnectLinkWrongMailbox       = "connect_link_wrong_mailbox"        // 403
	ErrCodeConnectWarmup                 = "connect_warmup"                    // 400

	// Copying campaigns and templates into other workspaces.
	ErrCodeCopyDestinationsLimit             = "copy_destinations_limit"               // 400
	ErrCodeCopyTemplatesLimit                = "copy_templates_limit"                  // 400
	ErrCodeCrossWorkspaceCopyRequiresSession = "cross_workspace_copy_requires_session" // 403
	// ErrCodeCopyDestinationForbidden is not an HTTP refusal: it is the code
	// on one destination's entry in a copy result.
	ErrCodeCopyDestinationForbidden = "copy_destination_forbidden"

	// Balance top-up, automatic reload and plan offers.
	ErrCodeCardDeclined           = "card_declined"           // 402
	ErrCodeAuthenticationRequired = "authentication_required" // 402
	ErrCodeChargeInProgress       = "charge_in_progress"      // 409
	ErrCodeMonthlyLimitTooSmall   = "monthly_limit_too_small" // 422
	ErrCodePlanOfferRequired      = "plan_offer_required"     // 403
	ErrCodeWarmupPlanRetired      = "warmup_plan_retired"     // 409

	// Member access restrictions.
	ErrCodeOwnerAccessUnrestricted = "owner_access_unrestricted" // 403
	ErrCodeInvalidAccessScope      = "invalid_access_scope"      // 400
	ErrCodeTooManyAccessGrants     = "too_many_access_grants"    // 400
	ErrCodeInvalidAccessResource   = "invalid_access_resource"   // 400

	// The Slack panel.
	ErrCodeSlackNotConfigured      = "slack_not_configured"       // 503
	ErrCodeSlackNotConnected       = "slack_not_connected"        // 404
	ErrCodeSlackNotLinked          = "slack_not_linked"           // 404
	ErrCodeSlackLinkInvalid        = "slack_link_invalid"         // 404
	ErrCodeSlackLinkEmailMismatch  = "slack_link_email_mismatch"  // 403
	ErrCodeSlackVerifyUnavailable  = "slack_verify_unavailable"   // 503
	ErrCodeSlackVerifyFailed       = "slack_verify_failed"        // 403
	ErrCodeSlackVerifyWrongAccount = "slack_verify_wrong_account" // 403

	// Saved list views.
	ErrCodeUnknownView     = "unknown_view"     // 404
	ErrCodeInvalidColumn   = "invalid_column"   // 400
	ErrCodeDuplicateColumn = "duplicate_column" // 400
	ErrCodeTooManyColumns  = "too_many_columns" // 400
	ErrCodeInvalidSort     = "invalid_sort"     // 400
	ErrCodeInvalidLayout   = "invalid_layout"   // 400

	// Other refusals.
	ErrCodeFormsDomainTaken          = "forms_domain_taken"          // 409
	ErrCodeIdempotencyOutcomeUnknown = "idempotency_outcome_unknown" // 409
	// ErrCodeContactLimitReached is the 403 for new contacts that would take
	// the workspace past its contact limit.
	ErrCodeContactLimitReached = "contact_limit_reached"
	// ErrCodeConnectWarmupFailed and ErrCodeConnectWarmupSettingsFailed are
	// not HTTP refusals: they are set as the error code on a connect link that
	// connected its mailbox but could not start warmup, or started it on the
	// default ramp.
	ErrCodeConnectWarmupFailed         = "connect_warmup_failed"
	ErrCodeConnectWarmupSettingsFailed = "connect_warmup_settings_failed"
	ErrCodeInitialPasswordWebOnly      = "initial_password_web_only"   // 403
	ErrCodeInvalidCursor               = "invalid_cursor"              // 400
	ErrCodeInvalidEmail                = "invalid_email"               // 400
	ErrCodeInvalidLimit                = "invalid_limit"               // 400
	ErrCodeInvalidName                 = "invalid_name"                // 400
	ErrCodeInvalidReportLink           = "invalid_report_link"         // 400
	ErrCodeInvalidReportRange          = "invalid_report_range"        // 400
	ErrCodeInvalidReportSchedule       = "invalid_report_schedule"     // 400
	ErrCodeInvalidReportScope          = "invalid_report_scope"        // 400
	ErrCodeInvalidReportSections       = "invalid_report_sections"     // 400
	ErrCodeMailboxVendorHoldExpired    = "mailbox_vendor_hold_expired" // 400
	ErrCodeScheduledSendNotPending     = "scheduled_send_not_pending"  // 409

	// Workspace and agency branding, portal domains and portal sessions.
	ErrCodeInvalidAccentColor       = "invalid_accent_color"       // 400
	ErrCodeInvalidBrandLink         = "invalid_brand_link"         // 400
	ErrCodeInvalidBrandText         = "invalid_brand_text"         // 400
	ErrCodeInvalidPortalDomain      = "invalid_portal_domain"      // 400
	ErrCodeInvalidPortalSignIn      = "invalid_portal_sign_in"     // 400
	ErrCodeInvalidTheme             = "invalid_theme"              // 400
	ErrCodeInvalidWebsiteURL        = "invalid_website_url"        // 400
	ErrCodePortalDomainTaken        = "portal_domain_taken"        // 409
	ErrCodePortalNotFound           = "portal_not_found"           // 404
	ErrCodePortalRegistrationClosed = "portal_registration_closed" // 403
	ErrCodePortalSessionRestricted  = "portal_session_restricted"  // 403
	ErrCodePortalSignInElsewhere    = "portal_sign_in_elsewhere"   // 403
	ErrCodeWhiteLabelNotEntitled    = "white_label_not_entitled"   // 403

	// A workspace's own mailbox OAuth apps (/emails/oauth-apps).
	ErrCodeOAuthAppClientID            = "oauth_app_client_id"            // 400
	ErrCodeOAuthAppClientSecret        = "oauth_app_client_secret"        // 400
	ErrCodeOAuthAppInUse               = "oauth_app_in_use"               // 409
	ErrCodeOAuthAppNotFound            = "oauth_app_not_found"            // 404
	ErrCodeOAuthAppProvider            = "oauth_app_provider"             // 400
	ErrCodeOAuthAppRedirectURL         = "oauth_app_redirect_url"         // 400
	ErrCodeOAuthAppRejected            = "oauth_app_rejected"             // 422
	ErrCodeOAuthAppSecretMissing       = "oauth_app_secret_missing"       // 422
	ErrCodePartnershipOAuthAppRequired = "partnership_oauth_app_required" // 422
	ErrCodePartnershipPlanRequired     = "partnership_plan_required"      // 403

	// Client reports (/reports).
	ErrCodeReportEmailsUnavailable = "report_emails_unavailable" // 422
	ErrCodeReportLimitReached      = "report_limit_reached"      // 409
	ErrCodeReportLinksUnavailable  = "report_links_unavailable"  // 422
	ErrCodeReportNotFound          = "report_not_found"          // 404

	// Tool migrations (/migrations).
	ErrCodeToolMigrationInFlightUnsupported = "tool_migration_in_flight_unsupported" // 400
	ErrCodeToolMigrationKeyRejected         = "tool_migration_key_rejected"          // 422
	ErrCodeToolMigrationLimit               = "tool_migration_limit"                 // 409
	ErrCodeToolMigrationNotFound            = "tool_migration_not_found"             // 404
	ErrCodeToolMigrationNotReady            = "tool_migration_not_ready"             // 409
	ErrCodeToolMigrationPermission          = "tool_migration_permission"            // 403
	ErrCodeToolMigrationProviderUnavailable = "tool_migration_provider_unavailable"  // 503
	ErrCodeToolMigrationProviderUnknown     = "tool_migration_provider_unknown"      // 400
	ErrCodeToolMigrationRunning             = "tool_migration_running"               // 409
	ErrCodeToolMigrationTargetInvalid       = "tool_migration_target_invalid"        // 400

	// The store (/store): domains and mailboxes bought for the workspace.
	// [ErrCodePaymentMethodRequired], [ErrCodeCardDeclined],
	// [ErrCodeAuthenticationRequired], [ErrCodeChargeInProgress],
	// [ErrCodeInvalidCursor], [ErrCodeInvalidLimit], [ErrCodeInvalidName] and
	// [ErrCodeReauthRequired] also answer store routes.
	ErrCodeContactInvalid            = "contact_invalid"             // 400
	ErrCodeStoreItemsInvalid         = "store_items_invalid"         // 400
	ErrCodeInvalidQuery              = "invalid_query"               // 400
	ErrCodeForwardingURLInvalid      = "forwarding_url_invalid"      // 400
	ErrCodeLocalPartInvalid          = "local_part_invalid"          // 400
	ErrCodeDNSRecordInvalid          = "dns_record_invalid"          // 400
	ErrCodePhotoInvalid              = "photo_invalid"               // 400
	ErrCodePhotoNotFound             = "photo_not_found"             // 400
	ErrCodePaymentFailed             = "payment_failed"              // 402
	ErrCodeInsufficientBalance       = "insufficient_balance"        // 402
	ErrCodePlanRequired              = "plan_required"               // 402
	ErrCodeStoreManagedByPartner     = "store_managed_by_partner"    // 403
	ErrCodePurchasedMailbox          = "purchased_mailbox"           // 403
	ErrCodeStoreDisabled             = "store_disabled"              // 404
	ErrCodeQuoteNotFound             = "quote_not_found"             // 404
	ErrCodeProfilePhotoMissing       = "profile_photo_missing"       // 404
	ErrCodePriceChanged              = "price_changed"               // 409
	ErrCodePaymentProcessing         = "payment_processing"          // 409
	ErrCodeStorePurchasesActive      = "store_purchases_active"      // 409
	ErrCodeDomainUnavailable         = "domain_unavailable"          // 409
	ErrCodeDomainRegisteredElsewhere = "domain_registered_elsewhere" // 409
	ErrCodeDomainRegisteredHere      = "domain_registered_here"      // 409
	ErrCodeDomainInUse               = "domain_in_use"               // 409
	ErrCodeDNSRecordConflict         = "dns_record_conflict"         // 409
	ErrCodeDNSRecordManaged          = "dns_record_managed"          // 409
	ErrCodeDNSNotHosted              = "dns_not_hosted"              // 409
	ErrCodeSignInUnavailable         = "sign_in_unavailable"         // 409
	ErrCodeStoreStateConflict        = "store_state_conflict"        // 409
	ErrCodeMailboxExists             = "mailbox_exists"              // 409
	ErrCodeDomainNotReady            = "domain_not_ready"            // 409
	ErrCodeDomainNotRenewable        = "domain_not_renewable"        // 409
	ErrCodeRenewalInProgress         = "renewal_in_progress"         // 409
	ErrCodeTransferLocked            = "transfer_locked"             // 409
	ErrCodeMailboxNotCancellable     = "mailbox_not_cancellable"     // 409
	ErrCodeRegistrantRequired        = "registrant_required"         // 422
	ErrCodeDomainPremium             = "domain_premium"              // 422
	ErrCodeTLDUnsupported            = "tld_unsupported"             // 422
	ErrCodeProviderMixed             = "provider_mixed"              // 422
	ErrCodeProviderUnavailable       = "provider_unavailable"        // 422
	ErrCodeMailboxLimit              = "mailbox_limit"               // 422
	ErrCodeRenewalNotDue             = "renewal_not_due"             // 422
	ErrCodeRenewalWindowClosed       = "renewal_window_closed"       // 422
	ErrCodePhotoLimitReached         = "photo_limit_reached"         // 429
	ErrCodeStoreRateLimited          = "rate_limited"                // 429
	ErrCodePhotoStorageUnavailable   = "photo_storage_unavailable"   // 503
	ErrCodeStoreUnavailable          = "store_unavailable"           // 503
	// ErrCodeInvalidCategory is the 400 from [BillingService.CreditTransactionsInCategory]
	// with a category the server does not know.
	ErrCodeInvalidCategory = "invalid_category" // 400

	// AI refusals (402) beside [ErrCodeInsufficientCredits]: the plan's
	// included usage is spent and AI may not use added money
	// ([CreditSettings.AIOverageEnabled] is off), or AI took its monthly limit
	// from added money ([CreditSettings.AIOverageMonthlyLimitUSD]).
	ErrCodeAIAllowanceUsed       = "ai_allowance_used"        // 402
	ErrCodeAIOverageLimitReached = "ai_overage_limit_reached" // 402

	// A workspace a partner (reseller) manages. A suspended or canceled one
	// is read-only; billing changes are made with the partner; an account the
	// partner signs in cannot set a password, passkey or second factor; and a
	// mailbox the partner holds is removed or reconnected through it.
	ErrCodeOrganizationSuspended = "organization_suspended"  // 403
	ErrCodeOrganizationCancelled = "organization_cancelled"  //nolint:misspell // 403; wire value
	ErrCodePartnerManaged        = "partner_managed"         // 403
	ErrCodePartnerAccount        = "partner_account"         // 403
	ErrCodePartnerManagedMailbox = "partner_managed_mailbox" // 403

	// ErrCodeLeadStepSent is the 409 for personalizing, resetting or
	// rewriting one lead's email after the lead already received it.
	ErrCodeLeadStepSent = "lead_step_sent" // 409
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
