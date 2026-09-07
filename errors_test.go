package warmbly

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestErrorIsSentinels(t *testing.T) {
	tests := []struct {
		status   int
		sentinel error
		want     bool
	}{
		{http.StatusNotFound, ErrNotFound, true},
		{http.StatusNotFound, ErrUnauthorized, false},
		{http.StatusUnauthorized, ErrUnauthorized, true},
		{http.StatusForbidden, ErrForbidden, true},
		{http.StatusConflict, ErrConflict, true},
		{http.StatusTooManyRequests, ErrRateLimited, true},
		{http.StatusUnprocessableEntity, ErrUnprocessable, true},
		{http.StatusInternalServerError, ErrServer, true},
		{http.StatusServiceUnavailable, ErrServer, true}, // any 5xx matches ErrServer
		{http.StatusNotFound, ErrServer, false},
	}
	for _, tt := range tests {
		err := error(&Error{StatusCode: tt.status})
		if got := errors.Is(err, tt.sentinel); got != tt.want {
			t.Errorf("errors.Is(Error{%d}, sentinel) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestErrorTemporary(t *testing.T) {
	cases := map[int]bool{
		http.StatusTooManyRequests:     true,
		http.StatusInternalServerError: true,
		http.StatusBadGateway:          true,
		http.StatusNotFound:            false,
		http.StatusBadRequest:          false,
	}
	for status, want := range cases {
		e := &Error{StatusCode: status}
		if got := e.Temporary(); got != want {
			t.Errorf("Error{%d}.Temporary() = %v, want %v", status, got, want)
		}
	}
}

func TestErrorString(t *testing.T) {
	e := &Error{StatusCode: 404, Code: "resource_not_found", Message: "Resource not found.", RequestID: "req_123"}
	got := e.Error()
	for _, want := range []string{"404", "resource_not_found", "Resource not found.", "req_123"} {
		if !strings.Contains(got, want) {
			t.Errorf("Error() = %q, missing %q", got, want)
		}
	}
}

// TestErrorHasCode covers the branch callers use to tell one 400 from another.
// It must not panic on the nil *Error an unmatched errors.As leaves behind.
func TestErrorHasCode(t *testing.T) {
	err := &Error{StatusCode: http.StatusBadRequest, Code: ErrCodeStorageLimitReached}

	if !err.HasCode(ErrCodeStorageLimitReached) {
		t.Error("HasCode did not match the code it carries")
	}
	if err.HasCode(ErrCodeNoOrganization) {
		t.Error("HasCode matched a different code")
	}
	// An empty argument must never match, or a caller comparing against an
	// unset variable would take the branch for every error.
	if err.HasCode("") {
		t.Error(`HasCode("") should be false`)
	}
	if (&Error{StatusCode: 400}).HasCode("") {
		t.Error(`HasCode("") should be false even when the envelope carried no code`)
	}

	var missing *Error
	if missing.HasCode(ErrCodeStorageLimitReached) {
		t.Error("HasCode on a nil *Error should be false")
	}
}

// TestErrorHasCodeAfterAs is the shape the documentation recommends: pull the
// typed error out, then branch on the code.
func TestErrorHasCodeAfterAs(t *testing.T) {
	var err error = &Error{StatusCode: http.StatusForbidden, Code: ErrCodeRegistrationInviteOnly, Message: "invite only"}

	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatal("errors.As did not yield an *Error")
	}
	if !apiErr.HasCode(ErrCodeRegistrationInviteOnly) {
		t.Errorf("code = %q", apiErr.Code)
	}
	// Status matching keeps working alongside code matching.
	if !errors.Is(err, ErrForbidden) {
		t.Error("errors.Is(err, ErrForbidden) = false")
	}
}

// TestErrorCodesAreDistinct guards the catalog against a copy/paste
// collision, which would make two unrelated conditions indistinguishable.
func TestErrorCodesAreDistinct(t *testing.T) {
	codes := []string{
		ErrCodeBadRequest, ErrCodeUnauthorized, ErrCodeForbidden, ErrCodeNotFound,
		ErrCodeConflict, ErrCodeUnprocessable, ErrCodeRateLimitExceeded,
		ErrCodeInternalError, ErrCodeNotImplemented, ErrCodeServiceUnavailable,
		ErrCodeInsufficientCredits, ErrCodeUsageCapExceeded,
		ErrCodeNoOrganization, ErrCodeStorageLimitReached, ErrCodeMailboxProviderNotConfigured,
		ErrCodeInvalidLeadStatus, ErrCodeInvalidEngagement, ErrCodeLeadFilterRequiresCampaign,
		ErrCodeUnknownVerificationStatus, ErrCodeUnknownVerificationProvider,
		ErrCodeInvalidAction, ErrCodeNoContacts,
		ErrCodeRegistrationInviteOnly, ErrCodeRegistrationClosed, ErrCodeInvitationInvalid,
		ErrCodeSetupTokenInvalid, ErrCodeSetupAlreadyComplete, ErrCodeSSOWrongBrowser,
		// Declared alongside the services that raise them.
		ErrCodeMailboxAllowanceReached, ErrCodeMailboxWorkerUnreachable,
		ErrCodeListBounceRisk, ErrCodeLeadsUndeliverable,
	}
	seen := make(map[string]bool, len(codes))
	for _, c := range codes {
		if c == "" {
			t.Error("an error code constant is empty")
		}
		if seen[c] {
			t.Errorf("duplicate error code %q", c)
		}
		seen[c] = true
		if c != strings.ToLower(c) || strings.ContainsAny(c, " -.") {
			t.Errorf("error code %q is not a lower_snake_case slug", c)
		}
	}
}

// TestErrorDecodesTheEnvelope checks that the struct still matches the wire
// shape the API returns, including the retry_after only some endpoints set.
func TestErrorDecodesTheEnvelope(t *testing.T) {
	body := `{
		"error":"Too Many Requests",
		"message":"API key exceeded 60 requests per minute",
		"code":"rate_limit_exceeded",
		"request_id":"4bbbd1b2-8f86-47dd-8a7f-9476501ad20e",
		"retry_after":42
	}`
	var e Error
	if err := json.Unmarshal([]byte(body), &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if e.Type != "Too Many Requests" || !e.HasCode(ErrCodeRateLimitExceeded) {
		t.Errorf("decoded = %+v", e)
	}
	if e.RequestID == "" || e.RetryAfter != 42 {
		t.Errorf("request id/retry_after lost: %+v", e)
	}

	// The common case: no retry_after at all.
	var plain Error
	if err := json.Unmarshal([]byte(`{"error":"Not Found","message":"Resource not found.","code":"not_found","request_id":"r_1"}`), &plain); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if plain.RetryAfter != 0 || !plain.HasCode(ErrCodeNotFound) {
		t.Errorf("decoded = %+v", plain)
	}
}
