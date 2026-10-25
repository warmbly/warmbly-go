package warmbly

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestResetPasswordSession(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"session":"q3Vh2c","expires_at":"2026-10-23T12:15:00Z"}`)

	out, _, err := c.Auth.ResetPasswordSession(context.Background(), "link-token")
	if err != nil {
		t.Fatal(err)
	}
	if got.method != "POST" || got.path != "/v1/auth/reset-password/session" || got.rawQuery != "" {
		t.Errorf("sent %s %s ?%s", got.method, got.path, got.rawQuery)
	}
	if strings.TrimSpace(got.body) != `{"token":"link-token"}` {
		t.Errorf("body = %s", got.body)
	}
	if out.Session != "q3Vh2c" || !out.ExpiresAt.Equal(time.Date(2026, 10, 23, 12, 15, 0, 0, time.UTC)) {
		t.Errorf("decoded %+v", out)
	}
}

func TestResetPasswordSessionLinkInvalid(t *testing.T) {
	c := reauthClient(t, 401, `{"error":"Unauthorized","message":"This reset link has expired.","code":"reset_link_invalid","request_id":"r"}`)
	_, _, err := c.Auth.ResetPasswordSession(context.Background(), "old")
	var apiErr *Error
	if !errors.As(err, &apiErr) || !apiErr.HasCode(ErrCodeResetLinkInvalid) || !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("error = %v", err)
	}
}

func TestResetPasswordConfirmWithRevoke(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{}`)
	_, err := c.Auth.ResetPasswordConfirmWith(context.Background(), &ResetPasswordConfirmParams{
		Session: "q3Vh2c", Password: "a-new-password", RevokeAPICredentials: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.method != "POST" || got.path != "/v1/auth/reset-password/confirm" || got.rawQuery != "" {
		t.Errorf("sent %s %s ?%s", got.method, got.path, got.rawQuery)
	}
	if want := `{"session":"q3Vh2c","password":"a-new-password","revoke_api_credentials":true}`; strings.TrimSpace(got.body) != want {
		t.Errorf("body = %s, want %s", got.body, want)
	}

	// The existing method still sends no revoke flag.
	if _, err := c.Auth.ResetPasswordConfirm(context.Background(), "s", "p", ""); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.body, "revoke_api_credentials") {
		t.Errorf("ResetPasswordConfirm body = %s", got.body)
	}
}

func TestChangePasswordWithRevoke(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"access_token":"a","refresh_token":"r"}`)
	sess, _, err := c.Auth.ChangePasswordWith(context.Background(), &ChangePasswordParams{
		CurrentPassword: "old", NewPassword: "brand-new-password", RevokeAPICredentials: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.method != "POST" || got.path != "/v1/auth/me/password" {
		t.Errorf("sent %s %s", got.method, got.path)
	}
	if want := `{"current_password":"old","new_password":"brand-new-password","revoke_api_credentials":true}`; strings.TrimSpace(got.body) != want {
		t.Errorf("body = %s, want %s", got.body, want)
	}
	if sess.AccessToken != "a" {
		t.Errorf("session = %+v", sess)
	}

	c = reauthClient(t, 400, `{"error":"Bad Request","message":"x","code":"api_credentials_revoke_unsupported"}`)
	_, _, err = c.Auth.ChangePasswordWith(context.Background(), &ChangePasswordParams{RevokeAPICredentials: true})
	var apiErr *Error
	if !errors.As(err, &apiErr) || !apiErr.HasCode(ErrCodeCredentialRevokeUnsupported) {
		t.Fatalf("error = %v", err)
	}
}
