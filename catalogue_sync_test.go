package warmbly

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestSyncedErrorCodes(t *testing.T) {
	err := &Error{StatusCode: 409, Code: "contact_email_taken"}
	if !err.HasCode(ErrCodeContactEmailTaken) {
		t.Errorf("HasCode(%q) = false", ErrCodeContactEmailTaken)
	}
	cases := map[string]string{
		ErrCodeSessionEnded:              "session_ended",
		ErrCodeDeveloperAccessBlocked:    "developer_access_blocked",
		ErrCodeAppSuspended:              "app_suspended",
		ErrCodePlacementQuotaExceeded:    "placement_quota_exceeded",
		ErrCodeCRMNotConnected:           "crm_not_connected",
		ErrCodeMailboxTLSFailed:          "mailbox_tls_failed",
		ErrCodeRecipientSuppressed:       "recipient_suppressed",
		ErrCodeTrackingDomainTaken:       "tracking_domain_taken",
		ErrCodeInvalidTLSPin:             "invalid_tls_pin",
		ErrCodeMailboxServerDeclined:     "mailbox_server_declined",
		ErrCodeWorkspaceSendingSuspended: "workspace_sending_suspended",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("constant = %q, want %q", got, want)
		}
	}
}

func TestErrorReasonDecodes(t *testing.T) {
	var e Error
	if err := json.Unmarshal([]byte(`{"error":"Bad Request","message":"m","code":"mailbox_tls_failed","reason":"certificate_untrusted","request_id":"r"}`), &e); err != nil {
		t.Fatal(err)
	}
	if e.Reason != "certificate_untrusted" || !e.HasCode(ErrCodeMailboxTLSFailed) {
		t.Errorf("%+v", e)
	}
}

func TestPermAppGrantableExcludesAPIKeys(t *testing.T) {
	if PermAppGrantable&PermAPIKeys != 0 {
		t.Error("PermAppGrantable must exclude PermAPIKeys")
	}
	if PermAppGrantable|PermAPIKeys != PermFullAccess {
		t.Error("PermAppGrantable should be PermFullAccess less PermAPIKeys")
	}
}

func TestOAuthAppUsable(t *testing.T) {
	var nilApp *OAuthApp
	if nilApp.Usable() {
		t.Error("nil app is not usable")
	}
	now := time.Now()
	cases := []struct {
		app  OAuthApp
		want bool
	}{
		{OAuthApp{Status: OAuthAppActive}, true},
		{OAuthApp{Status: OAuthAppDisabled}, false},
		{OAuthApp{Status: OAuthAppActive, SuspendedAt: &now}, false},
	}
	for _, tc := range cases {
		if got := tc.app.Usable(); got != tc.want {
			t.Errorf("%+v Usable() = %v", tc.app, got)
		}
	}
	if !errors.Is(&Error{StatusCode: 404}, ErrNotFound) {
		t.Error("sanity")
	}
}
