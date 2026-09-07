package warmbly

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"time"
)

// AuthService handles user sign-in and the session it produces: the email
// flow, browser SSO, the CLI device flow, two-factor verification, session
// management, the caller's own profile, notification preferences and device
// tokens. It also reads what the deployment supports ([AuthService.Config])
// and, on a self-hosted instance, what version it runs ([AuthService.Instance]).
//
// This is the credential path for anything an API key cannot reach — workspace
// governance, billing, the AI assistant. Sign in, then pass the returned
// [Session.AccessToken] to [WithAccessToken] (or wrap it in a [TokenSource]
// that refreshes it).
//
// Sign-in is usually two steps: [AuthService.Login] emails a code and returns
// an opaque session handle, and [AuthService.LoginConfirm] exchanges the handle
// and code for tokens. Whether the code step happens is deployment policy
// ([AuthConfig.LoginCode]): with it off, or on a device the account has used
// before, the first call already carries the tokens, which is why it answers
// with an [AuthStep] rather than a bare handle. When the account has 2FA on,
// whichever call completes the sign-in returns [Session.TwoFARequired] with a
// [Session.PendingToken] to finish through [AuthService.VerifyTwoFA].
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

// LoginParams starts a sign-in or a signup. Turnstile carries a bot-check
// token when the deployment requires one ([AuthConfig.Captcha]); the last two
// fields only matter to [AuthService.Register].
type LoginParams struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	Turnstile string `json:"turnstile,omitempty"`

	// ReferralCode is the ?ref= code a signup arrived with, so the new
	// workspace is attributed to its referrer.
	ReferralCode string `json:"referral_code,omitempty"`
	// Invite is a team-invitation token. On a deployment that runs
	// invite-only registration ([AuthConfig.InvitesRequired]) it is what
	// permits the signup, and the account lands in the inviting workspace
	// rather than a new one. It must resolve to a live invitation for the same
	// email, or the request fails with code "invitation_invalid"; omitting it
	// on a closed deployment fails with "registration_invite_only" or
	// "registration_closed".
	Invite string `json:"invite,omitempty"`
}

// AuthStep is the outcome of the first step of a sign-in or signup. Either
// another step is needed — CodeRequired is true and Session is the handle to
// pass to the confirm call — or the flow finished in one call and Token holds
// the session (with the usual 2FA challenge fields when the account has
// two-factor on).
//
// A deployment decides which: an emailed login code can be off entirely, or
// skipped on a device the account has signed in from before; a signup skips
// verification when the deployment does not require it or cannot deliver
// mail. Branch on CodeRequired rather than assuming the code step.
type AuthStep struct {
	// Session is the opaque handle for the confirm step. Empty when the flow
	// already finished.
	Session string `json:"session,omitempty"`
	// CodeRequired is true when an emailed code must be confirmed next.
	CodeRequired bool `json:"code_required"`
	// Token is the session when the flow finished in one step. It is nil while
	// a code is still required, and also nil after a completed signup that
	// could not be signed in immediately: the account exists, so sign in with
	// [AuthService.Login].
	Token *Session `json:"token,omitempty"`

	// TwoFARequired, PendingToken and ExpiresIn are the 2FA challenge, set
	// instead of Token when the flow finished but the account has two-factor
	// on. Finish with [AuthService.VerifyTwoFA].
	TwoFARequired bool   `json:"two_fa_required,omitempty"`
	PendingToken  string `json:"pending_token,omitempty"`
	ExpiresIn     int    `json:"expires_in,omitempty"`
}

// Done reports whether the flow finished in this step, with either a session
// or a 2FA challenge to complete.
func (a *AuthStep) Done() bool { return !a.CodeRequired }

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

// Login-code policies returned in [AuthConfig.LoginCode].
const (
	// LoginCodeAlways emails a code on every password sign-in.
	LoginCodeAlways = "always"
	// LoginCodeNewDevice emails a code only from a browser the account has
	// not signed in from before, or when the sign-in looks anomalous.
	LoginCodeNewDevice = "new_device"
	// LoginCodeOff never emails a code: [AuthService.Login] returns the
	// tokens directly.
	LoginCodeOff = "off"
)

// Registration policies returned in [AuthConfig.Registration].
const (
	// RegistrationOpen means anyone may create an account.
	RegistrationOpen = "true"
	// RegistrationInviteOnly means a signup needs an invitation token
	// ([LoginParams.Invite]).
	RegistrationInviteOnly = "invite_only"
	// RegistrationClosed means signups are off and invitations do not
	// override it.
	RegistrationClosed = "false"
)

// Browser SSO providers accepted by [AuthService.BeginSSO] and listed in
// [AuthConfig.Providers].
const (
	SSOProviderOIDC   = "oidc"
	SSOProviderGoogle = "google"
	SSOProviderApple  = "apple"
)

// AuthConfig is what a deployment supports, so one client binary adapts to
// hosted and self-hosted backends instead of guessing. Everything here is
// public, non-secret configuration; read it before rendering a sign-in.
type AuthConfig struct {
	// Captcha reports whether a Turnstile token is verified. When false do not
	// collect one: an air-gapped install cannot reach the challenge service.
	Captcha bool `json:"captcha"`
	// PasswordLogin is false when the deployment authenticates only through
	// SSO or passkeys; [AuthService.Login] and [AuthService.Register] are then
	// refused.
	PasswordLogin bool `json:"password_login"`
	// LoginCode is [LoginCodeAlways], [LoginCodeNewDevice] or [LoginCodeOff].
	LoginCode string `json:"login_code"`
	// Registration is [RegistrationOpen], [RegistrationInviteOnly] or
	// [RegistrationClosed], already resolved through the first-launch
	// exemption (a brand new instance reports open signups).
	Registration string `json:"registration"`
	// EmailVerification reports whether a signup must confirm an emailed code.
	EmailVerification bool `json:"email_verification"`
	// MailDelivers is false when the platform's mail transport writes to a log
	// instead of the wire, so emailed codes never arrive and the operator has
	// to read them from the server.
	MailDelivers bool `json:"mail_delivers"`
	// Passkeys reports whether WebAuthn can work here: it needs a secure
	// context, so a plain-http origin disables it.
	Passkeys bool `json:"passkeys"`
	// Providers are the browser SSO providers this backend can complete a
	// sign-in with (the SSOProvider* values). Native-app token sign-in is
	// separate; see [AuthService.Providers].
	Providers []string `json:"providers"`
	// ProviderLabels is what each provider's button should say, keyed by the
	// same identifiers, so a deployment behind Authentik says so.
	ProviderLabels map[string]string `json:"provider_labels,omitempty"`
	// SelfHosted lets a client drop hosted-only affordances.
	SelfHosted bool `json:"self_hosted"`
	// BillingEnabled is false when the deployment runs without a billing
	// provider: every feature is then unlocked and there is no trial or plan
	// to show. SelfHosted alone does not imply it.
	BillingEnabled bool `json:"billing_enabled"`
	// SetupRequired is true while the instance has no accounts at all; claim
	// it with [AuthService.Setup] before any sign-in can work.
	SetupRequired bool `json:"setup_required"`
	// InvitesRequired mirrors Registration == [RegistrationInviteOnly].
	InvitesRequired bool `json:"invites_required"`
	// DocsURL is where to send someone whose signup was refused by deployment
	// policy.
	DocsURL string `json:"docs_url"`
	// WebsocketURL is the realtime gateway; empty when the instance runs no
	// realtime service. AppURL is the dashboard origin, for building links to
	// pages. Both are served here because on a self-hosted instance the host
	// layout is whatever the operator chose.
	WebsocketURL string `json:"websocket_url,omitempty"`
	AppURL       string `json:"app_url,omitempty"`
}

// InstanceInfo is which Warmbly a self-hosted instance runs and whether a
// newer release exists. A hosted deployment answers SelfHosted false and
// nothing else. Applying an update is an admin-panel action, not an API call.
type InstanceInfo struct {
	SelfHosted bool `json:"self_hosted"`
	// Version is the release tag and Commit the short commit it was built
	// from.
	Version string `json:"version,omitempty"`
	Commit  string `json:"commit,omitempty"`
	// UpdateAvailable is true when Latest is newer than Version.
	UpdateAvailable bool `json:"update_available"`
	// Latest is the newest published release, when the instance has checked.
	Latest *InstanceRelease `json:"latest,omitempty"`
	// CheckedAt is when the instance last looked for a release.
	CheckedAt *time.Time `json:"checked_at,omitempty"`
}

// InstanceRelease is one published release.
type InstanceRelease struct {
	Tag         string    `json:"tag"`
	HTMLURL     string    `json:"html_url,omitempty"`
	PublishedAt time.Time `json:"published_at,omitempty"`
}

// SetupParams claims a fresh self-hosted instance. Token is the one-time
// setup token printed at first boot (or by warmblyctl setup-link); the rest
// becomes the owner account.
type SetupParams struct {
	Token     string `json:"token"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// SSORedirect is where to send the browser to start a provider sign-in, plus
// the binding secret that must come back at the exchange.
type SSORedirect struct {
	// URL is the provider's authorization URL. Navigate the browser there.
	URL string `json:"url"`
	// Binding is the secret this client must keep (never in a URL) and hand
	// to [AuthService.ExchangeSSO]. It ties the handoff to the browser that
	// started the sign-in, so a forwarded handoff link cannot sign anyone in.
	Binding string `json:"binding"`
}

// CLI device-flow handshake states returned in [CLIAuthPoll.Status] and
// [CLIAuthRequest.Status].
const (
	// CLIAuthPending means no member has decided yet.
	CLIAuthPending = "pending"
	// CLIAuthApproved means a member approved; the poll that sees it carries
	// the key.
	CLIAuthApproved = "approved"
	// CLIAuthClaimed means the key has already been handed out and the code
	// is spent.
	CLIAuthClaimed = "claimed"
	// CLIAuthDenied means a member declined.
	CLIAuthDenied = "denied"
)

// ErrCLIAuthDenied is returned by [AuthService.WaitForCLIAuth] when a member
// declined the request.
var ErrCLIAuthDenied = errors.New("warmbly: CLI sign-in was denied")

// CLIAuthStartParams opens a device-flow handshake. Everything but Scopes is
// display-only: it is what the approving member sees. Scopes is the API
// permission mask the minted key will hold (the Perm* constants); zero asks
// for the server default.
type CLIAuthStartParams struct {
	ClientName string `json:"client_name,omitempty"`
	Hostname   string `json:"hostname,omitempty"`
	CLIVersion string `json:"cli_version,omitempty"`
	Scopes     uint64 `json:"scopes,omitempty"`
}

// CLIAuthHandshake is an open device-flow handshake, shaped like RFC 8628 so
// a generic device-flow client works against it.
type CLIAuthHandshake struct {
	// DeviceCode is the secret this client keeps and polls with.
	DeviceCode string `json:"device_code"`
	// UserCode is what to show the person: eight unambiguous characters they
	// match against the approval screen.
	UserCode string `json:"user_code"`
	// VerificationURI is the approval page; VerificationURIComplete carries
	// the code already, so opening it needs no typing.
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	// ExpiresIn is the handshake's lifetime in seconds; Interval is how many
	// seconds to wait between polls.
	ExpiresIn int `json:"expires_in"`
	Interval  int `json:"interval"`
}

// CLIAuthPoll answers one poll. Status is the only field always set; the key
// fields arrive exactly once, on the poll that finds the code approved.
type CLIAuthPoll struct {
	// Status is [CLIAuthPending], [CLIAuthApproved] or [CLIAuthDenied].
	Status string `json:"status"`
	// Token is the minted API key (prefixed "wmbly_"). It is shown here and
	// never again: store it before returning.
	Token string `json:"token,omitempty"`
	// APIKeyID identifies the key under Settings > API keys, which is where
	// it can be revoked (or with [APIKeyService.RevokeSelf]).
	APIKeyID *string `json:"api_key_id,omitempty"`
	// Scopes is the granted permission mask; ScopeNames the same as names.
	Scopes     uint64   `json:"scopes,omitempty"`
	ScopeNames []string `json:"scope_names,omitempty"`
	// The signed-in identity, for labeling the credential.
	UserID           *string `json:"user_id,omitempty"`
	UserEmail        string  `json:"user_email,omitempty"`
	UserName         string  `json:"user_name,omitempty"`
	OrganizationID   *string `json:"organization_id,omitempty"`
	OrganizationName string  `json:"organization_name,omitempty"`
}

// CLIAuthRequest is what the approving member is shown before deciding: who
// is asking, from where, and for which scopes.
type CLIAuthRequest struct {
	ID         string   `json:"id"`
	UserCode   string   `json:"user_code"`
	ClientName string   `json:"client_name"`
	Hostname   string   `json:"hostname"`
	CLIVersion string   `json:"cli_version"`
	Scopes     uint64   `json:"scopes"`
	ScopeNames []string `json:"scope_names"`
	// Status is one of the CLIAuth* constants.
	Status string `json:"status"`
	// OrganizationID is the workspace the key was minted in, once approved.
	OrganizationID *string `json:"organization_id,omitempty"`
	// APIKeyID is set on the approval response only: the key that was minted.
	APIKeyID  *string   `json:"api_key_id,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// PasskeyLoginChallenge is the start of a passkey sign-in: the WebAuthn
// request options for the authenticator and the handle to return with its
// assertion.
type PasskeyLoginChallenge struct {
	// Session is the opaque handle for [AuthService.FinishPasskeyLogin].
	Session string `json:"session"`
	// Options is the raw WebAuthn credential-request options, for the
	// authenticator.
	Options json.RawMessage `json:"options"`
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
	// NotifCampaignPaused fires when the platform pauses a campaign on its
	// own, for example when an auto-pause guardrail is breached.
	NotifCampaignPaused = "campaign_paused"
	// NotifDomainAuth fires when a sending domain starts failing SPF or
	// DMARC: the warning before the send gate applies.
	NotifDomainAuth = "health_domain_auth"
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
	// CampaignPaused and DomainAuth default to on with email: both mean the
	// platform will (or did) stop sending, so they must reach someone who can
	// act.
	CampaignPaused CategoryPref `json:"campaign_paused"`
	DomainAuth     CategoryPref `json:"health_domain_auth"`

	// EmailDigestMinutes bundles pending notification emails into one send.
	// It must fall within [NotificationEmailDelivery.MinMinutes] and
	// [NotificationEmailDelivery.MaxMinutes]; zero on an update means the
	// server default. Security sign-in alerts always go out immediately
	// regardless.
	EmailDigestMinutes int `json:"email_digest_minutes"`
}

// NotificationEmailDelivery is the deployment's bounds for the email channel,
// so a digest-window control renders the right range.
type NotificationEmailDelivery struct {
	MinMinutes int `json:"min_minutes"`
	MaxMinutes int `json:"max_minutes"`
	// DailyCap is the most notification emails one account receives a day.
	DailyCap int `json:"daily_cap"`
}

// NotificationPreferencesResult is the preferences plus how email delivery is
// configured on this deployment.
type NotificationPreferencesResult struct {
	Preferences   NotificationPreferences    `json:"preferences"`
	EmailDelivery *NotificationEmailDelivery `json:"email_delivery,omitempty"`
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

// AccountDangerZone and its scheduling mirror the workspace danger zone; see
// [DangerZoneStatus] and [ScheduleDeletionParams].

// Login starts a password sign-in. Usually it emails a code and returns the
// session handle for [AuthService.LoginConfirm]; when the deployment's
// login-code policy does not demand one for this device, the returned
// [AuthStep] already carries the tokens. Branch on [AuthStep.CodeRequired].
// It is refused when [AuthConfig.PasswordLogin] is false.
func (s *AuthService) Login(ctx context.Context, params *LoginParams, opts ...RequestOption) (*AuthStep, *Response, error) {
	return send[AuthStep](ctx, s.client.post, "auth/login", params, opts)
}

// LoginConfirm exchanges the session handle and emailed code for tokens. When
// the account has two-factor enabled the result carries
// [Session.TwoFARequired] instead.
func (s *AuthService) LoginConfirm(ctx context.Context, params *ConfirmParams, opts ...RequestOption) (*Session, *Response, error) {
	return send[Session](ctx, s.client.post, "auth/login/confirm", params, opts)
}

// Register starts account creation. With email verification on it emails a
// code and returns the handle for [AuthService.RegisterConfirm]; otherwise the
// account is created at once and the [AuthStep] carries its session. On an
// invite-only deployment [LoginParams.Invite] is required; see the codes it
// documents.
func (s *AuthService) Register(ctx context.Context, params *LoginParams, opts ...RequestOption) (*AuthStep, *Response, error) {
	return send[AuthStep](ctx, s.client.post, "auth/register", params, opts)
}

// RegisterConfirm completes account creation with the emailed code and signs
// the new account in: the result's [AuthStep.Token] is the session. It is nil
// only when the account was created but could not be signed in on the spot,
// in which case [AuthService.Login] works.
func (s *AuthService) RegisterConfirm(ctx context.Context, params *ConfirmParams, opts ...RequestOption) (*AuthStep, *Response, error) {
	return send[AuthStep](ctx, s.client.post, "auth/register/confirm", params, opts)
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

// Config reports what this deployment supports: which sign-in methods are
// on, whether a login code follows, whether signups are open, whether the
// instance still needs claiming. It needs no credentials and is the first call
// a sign-in screen should make.
func (s *AuthService) Config(ctx context.Context, opts ...RequestOption) (*AuthConfig, *Response, error) {
	return fetch[AuthConfig](ctx, s.client, "auth/config", opts)
}

// Instance reports the running version of a self-hosted instance and whether
// a newer release exists. This route is session-only; any signed-in member may
// call it. A hosted deployment answers with SelfHosted false and nothing else.
func (s *AuthService) Instance(ctx context.Context, opts ...RequestOption) (*InstanceInfo, *Response, error) {
	return fetch[InstanceInfo](ctx, s.client, "auth/instance", opts)
}

// Setup claims a fresh self-hosted instance: it exchanges the one-time setup
// token for the owner account and returns its session, so there is no second
// sign-in. It needs no credentials — the token is the protection — and works
// only while [AuthConfig.SetupRequired] is true. An invalid, used or expired
// token fails with code "setup_token_invalid"; an instance that already has an
// account fails with "setup_already_complete".
func (s *AuthService) Setup(ctx context.Context, params *SetupParams, opts ...RequestOption) (*Session, *Response, error) {
	return send[Session](ctx, s.client.post, "auth/setup", params, opts)
}

// BeginSSO starts a browser sign-in with one of the SSOProvider* providers
// listed in [AuthConfig.Providers] and returns the authorization URL to send
// the browser to. Keep the [SSORedirect.Binding]: the exchange needs it.
//
// The provider sends the browser back to the API's callback, which redirects
// to the dashboard's /auth/sso page with a single-use "code" query parameter.
// Those callbacks are browser redirects, not JSON, so this SDK does not model
// them; collect the code from the redirect and pass it to
// [AuthService.ExchangeSSO]. A refused consent screen redirects back to the
// login page with no code.
func (s *AuthService) BeginSSO(ctx context.Context, provider string, opts ...RequestOption) (*SSORedirect, *Response, error) {
	return send[SSORedirect](ctx, s.client.post, "auth/"+url.PathEscape(provider)+"/begin", nil, opts)
}

// ExchangeSSO swaps the handoff code from a provider callback, together with
// the binding from [AuthService.BeginSSO], for a session. The code is single
// use. A code presented without the binding of the browser that began the
// sign-in fails with code "sso_wrong_browser", by design: a forwarded handoff
// link cannot sign its recipient in. Two-factor applies here like everywhere
// else; see [Session.TwoFARequired].
//
// The route is POST /auth/sso/exchange; the server also keeps the older
// POST /auth/oidc/exchange as an alias for the same handler.
func (s *AuthService) ExchangeSSO(ctx context.Context, code, binding string, opts ...RequestOption) (*Session, *Response, error) {
	body := struct {
		Code    string `json:"code"`
		Binding string `json:"binding"`
	}{Code: code, Binding: binding}
	return send[Session](ctx, s.client.post, "auth/sso/exchange", body, opts)
}

// --- CLI device flow ---

// StartCLIAuth opens a device-flow handshake for a client that holds no
// credential yet. Show the person [CLIAuthHandshake.UserCode], open
// [CLIAuthHandshake.VerificationURIComplete], then poll with
// [AuthService.PollCLIAuth] or let [AuthService.WaitForCLIAuth] do it. It
// needs no credentials and is per-IP rate limited; the handshake expires
// after [CLIAuthHandshake.ExpiresIn] seconds. A deployment without CLI
// sign-in answers 501.
func (s *AuthService) StartCLIAuth(ctx context.Context, params *CLIAuthStartParams, opts ...RequestOption) (*CLIAuthHandshake, *Response, error) {
	if params == nil {
		params = &CLIAuthStartParams{}
	}
	return send[CLIAuthHandshake](ctx, s.client.post, "auth/cli/code", params, opts)
}

// PollCLIAuth asks once whether a member has decided. The answer is
// [CLIAuthPending] until they do, then [CLIAuthApproved] carrying the minted
// key exactly once (a later poll finds the code spent), or [CLIAuthDenied].
// An unknown or expired device code is a 404 ([ErrNotFound]): the two are
// indistinguishable on purpose, so a poller cannot probe for live handshakes.
// Respect [CLIAuthHandshake.Interval] between polls; polling faster is rate
// limited per IP.
func (s *AuthService) PollCLIAuth(ctx context.Context, deviceCode string, opts ...RequestOption) (*CLIAuthPoll, *Response, error) {
	body := struct {
		DeviceCode string `json:"device_code"`
	}{DeviceCode: deviceCode}
	return send[CLIAuthPoll](ctx, s.client.post, "auth/cli/poll", body, opts)
}

// WaitForCLIAuth polls a handshake until a member decides, the handshake
// expires, or ctx is done. It sleeps [CLIAuthHandshake.Interval] between
// polls (five seconds when the server sent none) and, when the server asks it
// to slow down with a 429, waits the Retry-After it was given — or five more
// seconds — before continuing rather than failing.
//
// On approval it returns the poll carrying the key. A denial returns
// [ErrCLIAuthDenied]; an expired handshake surfaces as [ErrNotFound] from the
// poll; and a canceled context returns its error. Typical use:
//
//	hs, _, err := client.Auth.StartCLIAuth(ctx, &warmbly.CLIAuthStartParams{Hostname: host})
//	fmt.Println("Approve at", hs.VerificationURIComplete)
//	grant, err := client.Auth.WaitForCLIAuth(ctx, hs)
//	// grant.Token is the API key
func (s *AuthService) WaitForCLIAuth(ctx context.Context, hs *CLIAuthHandshake, opts ...RequestOption) (*CLIAuthPoll, error) {
	if hs == nil || hs.DeviceCode == "" {
		return nil, errors.New("warmbly: a started handshake with a device code is required")
	}
	interval := time.Duration(hs.Interval) * time.Second
	if interval <= 0 {
		interval = cliAuthPollInterval
	}
	const slowDown = 5 * time.Second

	wait := interval
	for {
		if err := sleepCtx(ctx, wait); err != nil {
			return nil, err
		}
		wait = interval

		poll, resp, err := s.PollCLIAuth(ctx, hs.DeviceCode, opts...)
		if err != nil {
			var apiErr *Error
			if errors.As(err, &apiErr) && apiErr.StatusCode == 429 {
				// The server wants a slower poller. Honor what it asked for,
				// and stretch the interval so the next poll is not another
				// 429.
				extra := slowDown
				if resp != nil && resp.RateLimit.RetryAfter > 0 {
					extra = resp.RateLimit.RetryAfter
				} else if apiErr.RetryAfter > 0 {
					extra = time.Duration(apiErr.RetryAfter) * time.Second
				}
				interval += slowDown
				wait = extra
				continue
			}
			return nil, err
		}
		switch poll.Status {
		case CLIAuthPending:
			continue
		case CLIAuthDenied:
			return poll, ErrCLIAuthDenied
		default:
			return poll, nil
		}
	}
}

// cliAuthPollInterval is how long [AuthService.WaitForCLIAuth] waits between
// polls when the handshake reported no interval of its own. The poll route
// shares a per-IP budget, so the fallback is deliberately unhurried.
var cliAuthPollInterval = 5 * time.Second

// sleepCtx waits for d or until ctx is done, whichever comes first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// CLIAuthRequest describes a pending handshake by its user code, for an
// approval screen: which client is asking, from which machine, and for which
// scopes. This route is session-only.
func (s *AuthService) CLIAuthRequest(ctx context.Context, userCode string, opts ...RequestOption) (*CLIAuthRequest, *Response, error) {
	return fetch[CLIAuthRequest](ctx, s.client, "auth/cli/codes/"+url.PathEscape(userCode), opts)
}

// ApproveCLIAuth approves a handshake and mints an ordinary API key into the
// given workspace — not necessarily the session's, because a member of
// several workspaces picks one on screen. An empty organizationID uses the
// session's workspace. The key shows up under Settings > API keys and the
// result's [CLIAuthRequest.APIKeyID] names it. This route is session-only and
// needs the manage-API-keys organization permission ([OrgPermManageAPIKeys]):
// an API key must not be able to mint another one this way. A code that is no
// longer pending answers 409.
func (s *AuthService) ApproveCLIAuth(ctx context.Context, userCode, organizationID string, opts ...RequestOption) (*CLIAuthRequest, *Response, error) {
	body := struct {
		OrganizationID string `json:"organization_id,omitempty"`
	}{OrganizationID: organizationID}
	return send[CLIAuthRequest](ctx, s.client.post, "auth/cli/codes/"+url.PathEscape(userCode)+"/approve", body, opts)
}

// DenyCLIAuth declines a handshake; the polling client sees
// [CLIAuthDenied]. Nothing is created, so nothing is audited. This route is
// session-only and needs [OrgPermManageAPIKeys].
func (s *AuthService) DenyCLIAuth(ctx context.Context, userCode string, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "auth/cli/codes/"+url.PathEscape(userCode)+"/deny", nil, nil, opts...)
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

// UploadAvatar sets the caller's profile picture and returns the URL it is now
// served from. The image must be a PNG or JPEG, at most 2 MB and 1024 pixels
// on a side; anything else is refused. The previous avatar is deleted.
func (s *AuthService) UploadAvatar(ctx context.Context, file *FileUpload, opts ...RequestOption) (string, *Response, error) {
	var out struct {
		AvatarURL string `json:"avatar_url"`
	}
	resp, err := s.client.postMultipart(ctx, "auth/me/avatar", "file", file, nil, &out, opts...)
	if err != nil {
		return "", resp, err
	}
	return out.AvatarURL, resp, nil
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
// WebAuthn request options together with the handle to finish with. It needs
// no credentials: there is no account context until the assertion resolves,
// and the challenge and signature are the protection.
func (s *AuthService) BeginPasskeyLogin(ctx context.Context, opts ...RequestOption) (*PasskeyLoginChallenge, *Response, error) {
	return send[PasskeyLoginChallenge](ctx, s.client.post, "auth/passkey/login/begin", nil, opts)
}

// FinishPasskeyLogin completes a passkey sign-in with the handle from
// [PasskeyLoginChallenge.Session] and the authenticator's assertion, and
// returns the session. It is a single step with no emailed code; two-factor
// still applies ([Session.TwoFARequired]).
func (s *AuthService) FinishPasskeyLogin(ctx context.Context, session string, assertion json.RawMessage, opts ...RequestOption) (*Session, *Response, error) {
	body := struct {
		Session    string          `json:"session"`
		Credential json.RawMessage `json:"credential"`
	}{Session: session, Credential: assertion}
	return send[Session](ctx, s.client.post, "auth/passkey/login/finish", body, opts)
}

// BeginPasskeyRegistration starts enrolling a passkey on the signed-in account
// and returns the WebAuthn creation options.
func (s *AuthService) BeginPasskeyRegistration(ctx context.Context, opts ...RequestOption) (json.RawMessage, *Response, error) {
	return s.passkeyStep(ctx, "auth/passkey/register/begin", nil, opts)
}

// FinishPasskeyRegistration completes enrollment with the authenticator's
// attestation and returns the stored credential. Name is the label shown in
// the credential list; empty lets the server pick one.
func (s *AuthService) FinishPasskeyRegistration(ctx context.Context, name string, attestation json.RawMessage, opts ...RequestOption) (*PasskeyCredential, *Response, error) {
	body := struct {
		Name       string          `json:"name,omitempty"`
		Credential json.RawMessage `json:"credential"`
	}{Name: name, Credential: attestation}
	return send[PasskeyCredential](ctx, s.client.post, "auth/passkey/register/finish", body, opts)
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

// Notifications returns the caller's in-app feed and unread count, newest
// first. The feed is capped at 50 entries unless a "limit" is given, and
// "unread=true" narrows it to what has not been read:
//
//	feed, _, err := client.Auth.Notifications(ctx,
//		warmbly.WithQueryParam("unread", "true"))
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

// RegisterDeviceToken registers a device for push notifications. The token is
// an APNs token (hex); platform must be "ios" or empty, and environment
// "production", "development" or empty, which defaults to production.
// Registering the same token again is harmless: it refreshes the record rather
// than creating a second one. The registration is not readable back — there is
// no route that lists a user's devices.
func (s *AuthService) RegisterDeviceToken(ctx context.Context, token, platform, environment string, opts ...RequestOption) (*Response, error) {
	body := struct {
		Token       string `json:"token"`
		Platform    string `json:"platform,omitempty"`
		Environment string `json:"environment,omitempty"`
	}{Token: token, Platform: platform, Environment: environment}
	return s.client.post(ctx, "auth/me/device-tokens", body, nil, opts...)
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
