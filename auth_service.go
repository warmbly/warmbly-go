package warmbly

import (
	"context"
	"encoding/json"
	"net/url"
	"time"
)

// AuthService handles user sign-in and the session it produces: the two-step
// email flow, two-factor verification, session management, the caller's own
// profile, notification preferences and device tokens.
//
// This is the credential path for anything an API key cannot reach — workspace
// governance, billing, the AI assistant. Sign in, then pass the returned
// [Session.AccessToken] to [WithAccessToken] (or wrap it in a [TokenSource]
// that refreshes it).
//
// Sign-in is two steps by design: [AuthService.Login] emails a code and returns
// an opaque session handle, and [AuthService.LoginConfirm] exchanges the handle
// and code for tokens. When the account has 2FA on, that second call returns
// [Session.TwoFARequired] with a [Session.PendingToken] to finish through
// [AuthService.VerifyTwoFA].
type AuthService service

// Session is a signed-in session's token pair.
type Session struct {
	AccessToken           string    `json:"access_token"`
	AccessTokenExpiresAt  time.Time `json:"access_token_expires_at"`
	RefreshToken          string    `json:"refresh_token"`
	RefreshTokenExpiresAt time.Time `json:"refresh_token_expires_at"`

	// TwoFARequired is true when the account has two-factor enabled. The token
	// fields are then empty; finish with [AuthService.VerifyTwoFA] using
	// PendingToken.
	TwoFARequired bool `json:"two_fa_required,omitempty"`
	// PendingToken is the single-use handle for the 2FA step.
	PendingToken string `json:"pending_token,omitempty"`
	// ExpiresIn is how long PendingToken remains valid, in seconds.
	ExpiresIn int `json:"expires_in,omitempty"`
}

// LoginParams starts a sign-in. Turnstile carries a bot-check token when the
// deployment requires one.
type LoginParams struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	Turnstile string `json:"turnstile,omitempty"`
}

// ConfirmParams completes a two-step flow with the emailed code.
type ConfirmParams struct {
	// Session is the handle returned by the first step.
	Session   string `json:"session"`
	Code      string `json:"code"`
	Turnstile string `json:"turnstile,omitempty"`
}

// UserSession is one active session on the account.
type UserSession struct {
	ID string `json:"id"`
	// Current marks the session making the request.
	Current bool   `json:"current"`
	Browser string `json:"browser,omitempty"`
	OS      string `json:"os,omitempty"`

	LocationCity    string `json:"location_city,omitempty"`
	LocationRegion  string `json:"location_region,omitempty"`
	LocationCountry string `json:"location_country,omitempty"`
	CountryCode     string `json:"country_code,omitempty"`

	// AuthProvider is how the session was created, for example "password",
	// "passkey", "apple" or "google".
	AuthProvider string    `json:"auth_provider"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
}

// ProfileUpdateParams updates the caller's own name.
type ProfileUpdateParams struct {
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
}

// OnboardingParams completes the first-run questions. FirstName and LastName
// are required; the rest are validated against fixed sets, so an unexpected
// value is rejected rather than stored.
type OnboardingParams struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	// ReferralSource is how they found Warmbly.
	ReferralSource string `json:"referral_source,omitempty"`
	Role           string `json:"role,omitempty"`
	TeamSize       string `json:"team_size,omitempty"`
}

// TwoFAStatus reports whether the account has two-factor enabled.
type TwoFAStatus struct {
	Enabled bool `json:"enabled"`
}

// TwoFAEnrollment is what a new authenticator needs to be set up.
type TwoFAEnrollment struct {
	// Secret is the shared TOTP secret, and OTPAuthURI the same thing as a
	// scannable otpauth:// URL.
	Secret     string `json:"secret"`
	OTPAuthURI string `json:"otpauth_uri"`
}

// TwoFARecoveryCodes are the single-use codes that get an account back when the
// authenticator is lost. They are shown once.
type TwoFARecoveryCodes struct {
	RecoveryCodes []string `json:"recovery_codes"`
}

// PasskeyCredential is one registered WebAuthn credential.
type PasskeyCredential struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Provider is the authenticator's origin, for example "icloud" or
	// "android".
	Provider string `json:"provider,omitempty"`
	// CredentialID is the WebAuthn credential identifier.
	CredentialID string `json:"credential_id"`
	// Transports are how the authenticator can be reached, for example
	// "internal" or "hybrid".
	Transports []string `json:"transports"`
	// BackupState reports whether the credential is synced to a cloud keychain
	// rather than bound to one device.
	BackupState bool       `json:"backup_state"`
	CreatedAt   time.Time  `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}

// AuthProviders reports which social sign-in options the deployment supports,
// so one client binary adapts to hosted and self-hosted backends.
type AuthProviders struct {
	Apple  AuthProvider `json:"apple"`
	Google AuthProvider `json:"google"`
}

// AuthProvider is one social sign-in option. The client id is public.
type AuthProvider struct {
	Enabled  bool   `json:"enabled"`
	ClientID string `json:"client_id,omitempty"`
}

// Notification categories used in [Notification.Category] and the preference
// map.
const (
	NotifInboundReply    = "inbound_reply"
	NotifInboundOOO      = "inbound_out_of_office"
	NotifHealthBounce    = "health_bounce"
	NotifHealthComplaint = "health_complaint"
	NotifWorkerDowntime  = "health_worker_downtime"
	NotifSecuritySignIn  = "security_new_signin"
	NotifBillingAlert    = "billing_alert"
	NotifTeamActivity    = "team_activity"
)

// ChannelPrefs are the delivery toggles for one notification category.
type ChannelPrefs struct {
	InApp bool `json:"in_app"`
	Email bool `json:"email"`
	Slack bool `json:"slack"`
	Push  bool `json:"push"`
}

// CategoryPref is one category's enable flag and channel toggles.
type CategoryPref struct {
	Enabled  bool         `json:"enabled"`
	Channels ChannelPrefs `json:"channels"`
}

// NotificationPreferences is the caller's full notification configuration. It
// is always returned fully populated.
type NotificationPreferences struct {
	InboundReply    CategoryPref `json:"inbound_reply"`
	InboundOOO      CategoryPref `json:"inbound_out_of_office"`
	HealthBounce    CategoryPref `json:"health_bounce"`
	HealthComplaint CategoryPref `json:"health_complaint"`
	WorkerDowntime  CategoryPref `json:"health_worker_downtime"`
	SecuritySignIn  CategoryPref `json:"security_new_signin"`
	BillingAlert    CategoryPref `json:"billing_alert"`
	TeamActivity    CategoryPref `json:"team_activity"`

	// EmailDigestMinutes bundles pending notification emails into one send.
	// Security sign-in alerts always go out immediately regardless.
	EmailDigestMinutes int `json:"email_digest_minutes"`
}

// NotificationPreferencesResult is the preferences plus how email delivery is
// configured on this deployment.
type NotificationPreferencesResult struct {
	Preferences NotificationPreferences `json:"preferences"`
	// EmailDelivery describes the deployment's email-channel setup.
	EmailDelivery map[string]any `json:"email_delivery,omitempty"`
}

// Notification is one entry in the in-app feed.
type Notification struct {
	ID             string  `json:"id"`
	UserID         string  `json:"user_id"`
	OrganizationID *string `json:"organization_id,omitempty"`
	// Category is one of the Notif* constants.
	Category string         `json:"category"`
	Title    string         `json:"title"`
	Body     string         `json:"body,omitempty"`
	Link     string         `json:"link,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	// ReadAt is nil while the notification is unread.
	ReadAt    *time.Time `json:"read_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// NotificationFeed is a page of the in-app feed plus the unread count.
type NotificationFeed struct {
	Notifications []Notification `json:"notifications"`
	Unread        int            `json:"unread"`
}

// DeviceToken is one registered push-capable device.
type DeviceToken struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	// Platform is the device family, for example "ios".
	Platform string `json:"platform"`
	Token    string `json:"token"`
	// Environment distinguishes a sandbox registration from production.
	Environment string    `json:"environment"`
	CreatedAt   time.Time `json:"created_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
}

// AccountDangerZone and its scheduling mirror the workspace danger zone; see
// [DangerZoneStatus] and [ScheduleDeletionParams].

// Login starts a sign-in and emails a verification code. It returns the opaque
// session handle to pass to [AuthService.LoginConfirm].
func (s *AuthService) Login(ctx context.Context, params *LoginParams, opts ...RequestOption) (string, *Response, error) {
	return s.startFlow(ctx, "auth/login", params, opts)
}

// LoginConfirm exchanges the session handle and emailed code for tokens. When
// the account has two-factor enabled the result carries
// [Session.TwoFARequired] instead.
func (s *AuthService) LoginConfirm(ctx context.Context, params *ConfirmParams, opts ...RequestOption) (*Session, *Response, error) {
	return send[Session](ctx, s.client.post, "auth/login/confirm", params, opts)
}

// Register starts account creation and emails a verification code.
func (s *AuthService) Register(ctx context.Context, params *LoginParams, opts ...RequestOption) (string, *Response, error) {
	return s.startFlow(ctx, "auth/register", params, opts)
}

// RegisterConfirm completes account creation. Sign in afterwards with
// [AuthService.Login].
func (s *AuthService) RegisterConfirm(ctx context.Context, params *ConfirmParams, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "auth/register/confirm", params, nil, opts...)
}

// ResetPassword emails a password-reset code. It answers the same way whether
// or not the address has an account.
func (s *AuthService) ResetPassword(ctx context.Context, email, turnstile string, opts ...RequestOption) (*Response, error) {
	body := struct {
		Email     string `json:"email"`
		Turnstile string `json:"turnstile,omitempty"`
	}{Email: email, Turnstile: turnstile}
	return s.client.post(ctx, "auth/reset-password", body, nil, opts...)
}

// ResetPasswordConfirm sets a new password using the emailed session handle.
func (s *AuthService) ResetPasswordConfirm(ctx context.Context, session, password, turnstile string, opts ...RequestOption) (*Response, error) {
	body := struct {
		Session   string `json:"session"`
		Password  string `json:"password"`
		Turnstile string `json:"turnstile,omitempty"`
	}{Session: session, Password: password, Turnstile: turnstile}
	return s.client.post(ctx, "auth/reset-password/confirm", body, nil, opts...)
}

func (s *AuthService) startFlow(ctx context.Context, path string, params *LoginParams, opts []RequestOption) (string, *Response, error) {
	var out struct {
		Session string `json:"session"`
	}
	resp, err := s.client.post(ctx, path, params, &out, opts...)
	if err != nil {
		return "", resp, err
	}
	return out.Session, resp, nil
}

// Refresh exchanges a refresh token for a fresh token pair. See
// [RefreshTokenSource] to have the client do this automatically.
func (s *AuthService) Refresh(ctx context.Context, refreshToken string, opts ...RequestOption) (*Session, *Response, error) {
	body := struct {
		RefreshToken string `json:"refresh_token"`
	}{RefreshToken: refreshToken}
	return send[Session](ctx, s.client.post, "auth/refresh", body, opts)
}

// Providers reports which social sign-in options this deployment supports. It
// needs no credentials.
func (s *AuthService) Providers(ctx context.Context, opts ...RequestOption) (*AuthProviders, *Response, error) {
	return fetch[AuthProviders](ctx, s.client, "auth/providers", opts)
}

// LoginWithApple exchanges an Apple-signed identity token for a session. The
// token's signature is the credential, so this needs no prior sign-in.
func (s *AuthService) LoginWithApple(ctx context.Context, identityToken string, opts ...RequestOption) (*Session, *Response, error) {
	return s.tokenLogin(ctx, "auth/apple", identityToken, opts)
}

// LoginWithGoogle exchanges a Google-signed ID token for a session.
func (s *AuthService) LoginWithGoogle(ctx context.Context, idToken string, opts ...RequestOption) (*Session, *Response, error) {
	return s.tokenLogin(ctx, "auth/google", idToken, opts)
}

func (s *AuthService) tokenLogin(ctx context.Context, path, token string, opts []RequestOption) (*Session, *Response, error) {
	body := struct {
		IDToken string `json:"id_token"`
	}{IDToken: token}
	return send[Session](ctx, s.client.post, path, body, opts)
}

// Logout revokes the current session.
func (s *AuthService) Logout(ctx context.Context, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "auth/logout", nil, nil, opts...)
}

// LogoutAll revokes every session on the account, including this one.
func (s *AuthService) LogoutAll(ctx context.Context, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "auth/logout-all", nil, nil, opts...)
}

// Sessions returns the account's active sessions.
func (s *AuthService) Sessions(ctx context.Context, opts ...RequestOption) ([]UserSession, *Response, error) {
	return fetchSlice[UserSession](ctx, s.client, "auth/sessions", opts)
}

// RevokeSession signs one session out.
func (s *AuthService) RevokeSession(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "auth/sessions/"+url.PathEscape(id), opts...)
}

// RevokeOtherSessions signs out every session except the current one.
func (s *AuthService) RevokeOtherSessions(ctx context.Context, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "auth/sessions", opts...)
}

// --- profile ---

// Me returns the caller's own profile, including their folder, tag and
// category sets.
func (s *AuthService) Me(ctx context.Context, opts ...RequestOption) (*User, *Response, error) {
	return fetch[User](ctx, s.client, "auth/me", opts)
}

// UpdateProfile changes the caller's name.
func (s *AuthService) UpdateProfile(ctx context.Context, params *ProfileUpdateParams, opts ...RequestOption) (*User, *Response, error) {
	return send[User](ctx, s.client.patch, "auth/me", params, opts)
}

// CompleteOnboarding answers the first-run questions.
func (s *AuthService) CompleteOnboarding(ctx context.Context, params *OnboardingParams, opts ...RequestOption) (*User, *Response, error) {
	return send[User](ctx, s.client.patch, "auth/me/onboarding", params, opts)
}

// UploadAvatar sets the caller's profile picture.
func (s *AuthService) UploadAvatar(ctx context.Context, file *FileUpload, opts ...RequestOption) (*User, *Response, error) {
	out := new(User)
	resp, err := s.client.postMultipart(ctx, "auth/me/avatar", "avatar", file, nil, out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// DeleteAvatar removes the caller's profile picture.
func (s *AuthService) DeleteAvatar(ctx context.Context, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "auth/me/avatar", opts...)
}

// ChangePassword sets a new password, which requires the current one.
func (s *AuthService) ChangePassword(ctx context.Context, currentPassword, newPassword string, opts ...RequestOption) (*Response, error) {
	body := struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}{CurrentPassword: currentPassword, NewPassword: newPassword}
	return s.client.post(ctx, "auth/me/password", body, nil, opts...)
}

// SetUndoSendSeconds sets how long an instant send is held, and stays
// cancelable, before it leaves. The server bounds the value.
func (s *AuthService) SetUndoSendSeconds(ctx context.Context, seconds int, opts ...RequestOption) (*Response, error) {
	body := struct {
		UndoSendSeconds int `json:"undo_send_seconds"`
	}{UndoSendSeconds: seconds}
	return s.client.put(ctx, "auth/me/send-preferences", body, nil, opts...)
}

// --- two-factor ---

// TwoFAStatus reports whether two-factor is enabled on the account.
func (s *AuthService) TwoFAStatus(ctx context.Context, opts ...RequestOption) (*TwoFAStatus, *Response, error) {
	return fetch[TwoFAStatus](ctx, s.client, "auth/2fa/status", opts)
}

// EnrollTwoFA begins two-factor setup and returns the shared secret. Confirm it
// with [AuthService.ConfirmTwoFA] before it takes effect.
func (s *AuthService) EnrollTwoFA(ctx context.Context, opts ...RequestOption) (*TwoFAEnrollment, *Response, error) {
	return send[TwoFAEnrollment](ctx, s.client.post, "auth/2fa/enroll/start", nil, opts)
}

// ConfirmTwoFA finishes setup with a code from the authenticator and returns
// the recovery codes, which are shown exactly once.
func (s *AuthService) ConfirmTwoFA(ctx context.Context, code string, opts ...RequestOption) (*TwoFARecoveryCodes, *Response, error) {
	body := struct {
		Code string `json:"code"`
	}{Code: code}
	return send[TwoFARecoveryCodes](ctx, s.client.post, "auth/2fa/enroll/confirm", body, opts)
}

// DisableTwoFA turns two-factor off. It requires a current code or a recovery
// code.
func (s *AuthService) DisableTwoFA(ctx context.Context, code string, opts ...RequestOption) (*Response, error) {
	body := struct {
		Code string `json:"code"`
	}{Code: code}
	return s.client.deleteBody(ctx, "auth/2fa", body, opts...)
}

// VerifyTwoFA completes a sign-in that stopped at the two-factor step, using
// the pending token from [AuthService.LoginConfirm] and a TOTP or recovery
// code.
func (s *AuthService) VerifyTwoFA(ctx context.Context, pendingToken, code string, opts ...RequestOption) (*Session, *Response, error) {
	body := struct {
		PendingToken string `json:"pending_token"`
		Code         string `json:"code"`
	}{PendingToken: pendingToken, Code: code}
	return send[Session](ctx, s.client.post, "auth/2fa/verify", body, opts)
}

// --- passkeys ---

// Passkeys returns the account's registered WebAuthn credentials.
func (s *AuthService) Passkeys(ctx context.Context, opts ...RequestOption) ([]PasskeyCredential, *Response, error) {
	return fetchSlice[PasskeyCredential](ctx, s.client, "auth/passkey/credentials", opts)
}

// The passkey begin and finish calls carry raw WebAuthn payloads. Producing and
// consuming those needs an authenticator, which lives in the browser or on the
// device rather than in this SDK — so these methods pass the JSON straight
// through in both directions. Hand the options to your WebAuthn client, and
// hand its response back.

// BeginPasskeyLogin starts a discoverable passkey sign-in and returns the
// WebAuthn request options. It needs no credentials: there is no account
// context until the assertion resolves, and the challenge and signature are the
// protection.
func (s *AuthService) BeginPasskeyLogin(ctx context.Context, opts ...RequestOption) (json.RawMessage, *Response, error) {
	return s.passkeyStep(ctx, "auth/passkey/login/begin", nil, opts)
}

// FinishPasskeyLogin completes a passkey sign-in with the authenticator's
// assertion and returns the session.
func (s *AuthService) FinishPasskeyLogin(ctx context.Context, assertion json.RawMessage, opts ...RequestOption) (*Session, *Response, error) {
	return send[Session](ctx, s.client.post, "auth/passkey/login/finish", assertion, opts)
}

// BeginPasskeyRegistration starts enrolling a passkey on the signed-in account
// and returns the WebAuthn creation options.
func (s *AuthService) BeginPasskeyRegistration(ctx context.Context, opts ...RequestOption) (json.RawMessage, *Response, error) {
	return s.passkeyStep(ctx, "auth/passkey/register/begin", nil, opts)
}

// FinishPasskeyRegistration completes enrollment with the authenticator's
// attestation and returns the stored credential.
func (s *AuthService) FinishPasskeyRegistration(ctx context.Context, attestation json.RawMessage, opts ...RequestOption) (*PasskeyCredential, *Response, error) {
	return send[PasskeyCredential](ctx, s.client.post, "auth/passkey/register/finish", attestation, opts)
}

func (s *AuthService) passkeyStep(ctx context.Context, path string, body any, opts []RequestOption) (json.RawMessage, *Response, error) {
	var out json.RawMessage
	resp, err := s.client.post(ctx, path, body, &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// RenamePasskey relabels a registered credential.
func (s *AuthService) RenamePasskey(ctx context.Context, id, name string, opts ...RequestOption) (*PasskeyCredential, *Response, error) {
	body := struct {
		Name string `json:"name"`
	}{Name: name}
	return send[PasskeyCredential](ctx, s.client.patch, "auth/passkey/credentials/"+url.PathEscape(id), body, opts)
}

// DeletePasskey removes a registered credential.
func (s *AuthService) DeletePasskey(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "auth/passkey/credentials/"+url.PathEscape(id), opts...)
}

// --- notifications ---

// NotificationPreferences returns the caller's notification configuration.
func (s *AuthService) NotificationPreferences(ctx context.Context, opts ...RequestOption) (*NotificationPreferencesResult, *Response, error) {
	return fetch[NotificationPreferencesResult](ctx, s.client, "auth/me/notification-preferences", opts)
}

// UpdateNotificationPreferences replaces the caller's notification
// configuration.
func (s *AuthService) UpdateNotificationPreferences(ctx context.Context, prefs *NotificationPreferences, opts ...RequestOption) (*NotificationPreferencesResult, *Response, error) {
	body := struct {
		Preferences *NotificationPreferences `json:"preferences"`
	}{Preferences: prefs}
	return send[NotificationPreferencesResult](ctx, s.client.put, "auth/me/notification-preferences", body, opts)
}

// Notifications returns the caller's in-app feed and unread count.
func (s *AuthService) Notifications(ctx context.Context, opts ...RequestOption) (*NotificationFeed, *Response, error) {
	return fetch[NotificationFeed](ctx, s.client, "auth/me/notifications", opts)
}

// MarkNotificationRead marks one notification read.
func (s *AuthService) MarkNotificationRead(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "auth/me/notifications/"+url.PathEscape(id)+"/read", nil, nil, opts...)
}

// MarkAllNotificationsRead marks the whole feed read.
func (s *AuthService) MarkAllNotificationsRead(ctx context.Context, opts ...RequestOption) (*Response, error) {
	return s.client.put(ctx, "auth/me/notifications", nil, nil, opts...)
}

// RegisterDeviceToken registers a device for push notifications.
func (s *AuthService) RegisterDeviceToken(ctx context.Context, token, platform, environment string, opts ...RequestOption) (*DeviceToken, *Response, error) {
	body := struct {
		Token       string `json:"token"`
		Platform    string `json:"platform,omitempty"`
		Environment string `json:"environment,omitempty"`
	}{Token: token, Platform: platform, Environment: environment}
	return send[DeviceToken](ctx, s.client.post, "auth/me/device-tokens", body, opts)
}

// DeleteDeviceToken unregisters a device from push notifications.
func (s *AuthService) DeleteDeviceToken(ctx context.Context, token string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "auth/me/device-tokens/"+url.PathEscape(token), opts...)
}

// --- account danger zone ---

// DangerZone returns the account's deletion state and the confirmation phrase
// required to schedule one.
func (s *AuthService) DangerZone(ctx context.Context, opts ...RequestOption) (*DangerZoneStatus, *Response, error) {
	return fetch[DangerZoneStatus](ctx, s.client, "me/danger-zone", opts)
}

// ScheduleDeletion schedules the caller's own account for a delayed hard
// delete. Confirmation must match [DangerZoneStatus.ConfirmationHint], which
// for an account is their email address.
func (s *AuthService) ScheduleDeletion(ctx context.Context, params *ScheduleDeletionParams, opts ...RequestOption) (*ScheduledDeletion, *Response, error) {
	return send[ScheduledDeletion](ctx, s.client.post, "me/danger-zone/delete", params, opts)
}

// CancelDeletion cancels a pending account deletion.
func (s *AuthService) CancelDeletion(ctx context.Context, reason string, opts ...RequestOption) (*Response, error) {
	body := struct {
		Reason string `json:"reason,omitempty"`
	}{Reason: reason}
	return s.client.deleteBody(ctx, "me/danger-zone/delete", body, opts...)
}
