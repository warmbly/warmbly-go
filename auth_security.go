package warmbly

import (
	"context"
	"encoding/json"
	"net/url"
	"time"
)

// This file holds the account-security half of [AuthService]: the follow-up
// steps of a sign-in, linked sign-in methods, password and recovery-code
// management, and re-authentication.
//
// Re-authentication is how a session proves the account holder is still at the
// keyboard. Sensitive changes (creating an API key, registering a passkey,
// inviting a member, approving a CLI or pool link, scheduling a deletion)
// refuse a session that has not confirmed recently; the refusal is a 403 with
// code "reauth_required". Read [AuthService.ReauthMethods] to learn which
// factors the account can answer with, confirm with one of the Reauth calls,
// then repeat the change within [ReauthResult.ValidForSeconds].

// ReauthParams confirms the account holder with a password or a code.
type ReauthParams struct {
	// Password is the account password. Leave it empty for an account that has
	// none (passkey or single sign-on only), which confirms with Code.
	Password string `json:"password,omitempty"`
	// Code is a current authenticator code or a recovery code.
	Code string `json:"code,omitempty"`
}

// ReauthResult reports a successful confirmation.
type ReauthResult struct {
	// ValidForSeconds is how long the confirmation lasts, so a client can
	// decide to ask again before a change fails.
	ValidForSeconds int `json:"valid_for_seconds"`
}

// ReauthProvider is one linked sign-in method that can confirm in a browser
// window.
type ReauthProvider struct {
	// ID is the provider identifier to pass to [AuthService.BeginReauthSSO],
	// one of the SSOProvider* values or a deployment-defined one.
	ID    string `json:"id"`
	Label string `json:"label"`
}

// ReauthMethods lists every way the signed-in account can confirm it is the
// holder, so a client offers only those.
type ReauthMethods struct {
	Password bool `json:"password"`
	// TOTP is true when the account has an authenticator; a recovery code also
	// works then.
	TOTP    bool `json:"totp"`
	Passkey bool `json:"passkey"`
	// Providers confirm in a browser window ([AuthService.BeginReauthSSO]);
	// NativeProviders confirm with an ID token from a native app
	// ([AuthService.BeginReauthNative]).
	Providers       []ReauthProvider `json:"providers"`
	NativeProviders []string         `json:"native_providers"`
	// ValidForSeconds is how long a confirmation lasts.
	ValidForSeconds int `json:"valid_for_seconds"`
}

// LinkedIdentity is a sign-in method (social or single sign-on provider)
// attached to the account. The provider's issuer and subject are not exposed.
type LinkedIdentity struct {
	// Provider is the identifier to pass to [AuthService.UnlinkIdentity], for
	// example "google", "apple" or "oidc".
	Provider string `json:"provider"`
	// Label is the display name, for example "Google" or the deployment's own
	// name for its single sign-on.
	Label string `json:"label"`
	// Email is the address the provider asserted, when it gave one.
	Email       string     `json:"email,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

// ChangePasswordParams changes the signed-in account's password.
type ChangePasswordParams struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
	// RevokeAPICredentials also ends the account's OAuth app authorizations
	// and the API keys it created. Use it when the old password may have been
	// exposed.
	RevokeAPICredentials bool `json:"revoke_api_credentials,omitempty"`
}

// ResetPasswordConfirmParams completes a password reset.
type ResetPasswordConfirmParams struct {
	// Session is the handle from [AuthService.ResetPasswordSession].
	Session  string `json:"session"`
	Password string `json:"password"`
	// RevokeAPICredentials also ends the account's OAuth app authorizations
	// and the API keys it created.
	RevokeAPICredentials bool   `json:"revoke_api_credentials,omitempty"`
	Turnstile            string `json:"turnstile,omitempty"`
}

// PasswordResetSession is the short-lived handle the emailed reset link opens.
type PasswordResetSession struct {
	// Session goes to [AuthService.ResetPasswordComplete] (or the older
	// [AuthService.ResetPasswordConfirm]).
	Session   string    `json:"session"`
	ExpiresAt time.Time `json:"expires_at"`
}

// --- sign-in follow-up steps ---

// CompleteInitialPassword finishes a sign-in whose first step answered with
// [AuthStep.PasswordChangeRequired]: the account holds a password someone else
// issued, and this call replaces it with the password the person chose. The
// pendingToken is [AuthStep.PendingToken]. The returned [AuthStep] is the
// ordinary sign-in outcome from then on: a code step (CodeRequired), the
// session in Token, or a 2FA challenge.
//
// The server ends the account's other sessions when it succeeds. The route
// needs no credentials (the pending token is the proof) and shares the per-IP
// sign-in budget. deviceToken is the remembered-device token to carry into the
// emailed-code step and may be empty.
func (s *AuthService) CompleteInitialPassword(ctx context.Context, pendingToken, newPassword, deviceToken string, opts ...RequestOption) (*AuthStep, *Response, error) {
	body := struct {
		PendingToken string `json:"pending_token"`
		Password     string `json:"password"`
		DeviceToken  string `json:"device_token,omitempty"`
	}{PendingToken: pendingToken, Password: newPassword, DeviceToken: deviceToken}
	return send[AuthStep](ctx, s.client.post, "auth/login/initial-password", body, opts)
}

// LinkSSO finishes a provider sign-in that resolved to an existing password
// account ([Session.LinkRequired]): it takes the account's password, attaches
// the provider identity and signs in. pendingToken is [Session.PendingToken].
// Two-factor still applies to the result ([Session.TwoFARequired]).
//
// The route needs no credentials (the pending token is the proof). Wrong
// passwords count against the sign-in failure budget.
func (s *AuthService) LinkSSO(ctx context.Context, pendingToken, password string, opts ...RequestOption) (*Session, *Response, error) {
	body := struct {
		PendingToken string `json:"pending_token"`
		Password     string `json:"password"`
	}{PendingToken: pendingToken, Password: password}
	return send[Session](ctx, s.client.post, "auth/sso/link", body, opts)
}

// ResetPasswordSession exchanges the token from the emailed reset link for the
// reset session [AuthService.ResetPasswordComplete] takes. Sending the token
// in a body keeps it out of URLs and logs. It needs no credentials. An unknown
// or expired token is refused.
func (s *AuthService) ResetPasswordSession(ctx context.Context, linkToken string, opts ...RequestOption) (*PasswordResetSession, *Response, error) {
	body := struct {
		Token string `json:"token"`
	}{Token: linkToken}
	return send[PasswordResetSession](ctx, s.client.post, "auth/reset-password/session", body, opts)
}

// ResetPasswordComplete sets a new password using the session from
// [AuthService.ResetPasswordSession]. It is [AuthService.ResetPasswordConfirm]
// with the option to also end the account's API credentials.
func (s *AuthService) ResetPasswordComplete(ctx context.Context, params *ResetPasswordConfirmParams, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "auth/reset-password/confirm", params, nil, opts...)
}

// LoginWithAppleProfile is [AuthService.LoginWithApple] for a first sign-in:
// Apple shares the person's name only with the app, never inside the identity
// token, so pass it along to prefill the new account. Either name may be empty.
func (s *AuthService) LoginWithAppleProfile(ctx context.Context, identityToken, firstName, lastName string, opts ...RequestOption) (*Session, *Response, error) {
	body := struct {
		IdentityToken string `json:"identity_token"`
		FirstName     string `json:"first_name,omitempty"`
		LastName      string `json:"last_name,omitempty"`
	}{IdentityToken: identityToken, FirstName: firstName, LastName: lastName}
	return send[Session](ctx, s.client.post, "auth/apple", body, opts)
}

// LogoutForgetDevices revokes the current session like [AuthService.Logout]
// and also forgets the given remembered-device tokens (see
// [Session.DeviceToken]), so the next sign-in from this client asks for the
// emailed code again. The server honors at most 16 tokens.
func (s *AuthService) LogoutForgetDevices(ctx context.Context, deviceTokens []string, opts ...RequestOption) (*Response, error) {
	body := struct {
		DeviceTokens []string `json:"device_tokens"`
	}{DeviceTokens: deviceTokens}
	return s.client.post(ctx, "auth/logout", body, nil, opts...)
}

// --- profile and password ---

// CompleteProductTour records that the caller finished or skipped the
// dashboard's product tour and returns when. The call is idempotent for an
// account that already completed it.
func (s *AuthService) CompleteProductTour(ctx context.Context, opts ...RequestOption) (time.Time, *Response, error) {
	var out struct {
		At time.Time `json:"product_tour_completed_at"`
	}
	resp, err := s.client.put(ctx, "auth/me/product-tour", nil, &out, opts...)
	if err != nil {
		return time.Time{}, resp, err
	}
	return out.At, resp, nil
}

// ChangePasswordSession changes the password like [AuthService.ChangePassword]
// and returns the replacement session. The server ends every session on the
// account, this one included, and issues a fresh token pair for the calling
// device; switch to it (or to a [RefreshTokenSource] built from it) at once.
// The returned [Session] has an empty AccessToken in the unlikely case the
// server issued none.
//
// This route is session-only and rate limited as a write. The current
// password is checked on the sign-in failure budget.
func (s *AuthService) ChangePasswordSession(ctx context.Context, params *ChangePasswordParams, opts ...RequestOption) (*Session, *Response, error) {
	return send[Session](ctx, s.client.post, "auth/me/password", params, opts)
}

// SendPasswordLink emails the caller a link to set a password. It is for an
// account that signs in through a provider or passkey and has none. This route
// is session-only and rate limited as a write.
func (s *AuthService) SendPasswordLink(ctx context.Context, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "auth/me/password/link", nil, nil, opts...)
}

// --- linked sign-in methods ---

// LinkedIdentities lists the provider sign-in methods attached to the account
// (the "data" array of the response). Passkeys are listed separately by
// [AuthService.Passkeys]. This route is session-only.
func (s *AuthService) LinkedIdentities(ctx context.Context, opts ...RequestOption) ([]LinkedIdentity, *Response, error) {
	return fetchData[LinkedIdentity](ctx, s.client, "auth/me/identities", opts)
}

// UnlinkIdentity removes a provider sign-in method and ends every other
// session that signed in through it. provider is [LinkedIdentity.Provider].
// It is refused with code "last_sign_in_method" when it would leave the
// account no way to sign in, and with "identity_not_found" for a provider that
// is not linked. A failure to end the provider's other sessions answers 503
// with code "sessions_not_ended" and a Retry-After; repeating the call
// finishes the job.
//
// This route is session-only and needs a recent confirmation
// ([AuthService.Reauth]).
func (s *AuthService) UnlinkIdentity(ctx context.Context, provider string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "auth/me/identities/"+url.PathEscape(provider), opts...)
}

// RegenerateRecoveryCodes replaces the account's two-factor recovery codes
// with a fresh set, shown exactly once, and invalidates the old ones. code is
// a current authenticator code. This route is session-only; wrong codes count
// against the code-attempt budget.
func (s *AuthService) RegenerateRecoveryCodes(ctx context.Context, code string, opts ...RequestOption) (*TwoFARecoveryCodes, *Response, error) {
	body := struct {
		Code string `json:"code"`
	}{Code: code}
	return send[TwoFARecoveryCodes](ctx, s.client.post, "auth/2fa/recovery-codes", body, opts)
}

// --- re-authentication (session-only) ---

// ReauthMethods lists the ways the signed-in account can confirm it is the
// holder.
func (s *AuthService) ReauthMethods(ctx context.Context, opts ...RequestOption) (*ReauthMethods, *Response, error) {
	return fetch[ReauthMethods](ctx, s.client, "auth/reauth/methods", opts)
}

// Reauth confirms the account holder with a password or an authenticator or
// recovery code and stamps the session. Either field is accepted; an account
// with no password confirms with its code. An account with neither a password
// nor an authenticator fails with code "reauth_no_factor" and can confirm with
// a passkey ([AuthService.BeginReauthPasskey]) or provider instead.
//
// Wrong attempts are limited per account; past the limit the call fails with
// code "reauth_limited" until the window passes.
func (s *AuthService) Reauth(ctx context.Context, params *ReauthParams, opts ...RequestOption) (*ReauthResult, *Response, error) {
	return send[ReauthResult](ctx, s.client.post, "auth/reauth", params, opts)
}

// BeginReauthPasskey asks for one of the account's own passkeys and returns
// the WebAuthn request options, to hand to the authenticator. Finish with
// [AuthService.FinishReauthPasskey].
func (s *AuthService) BeginReauthPasskey(ctx context.Context, opts ...RequestOption) (json.RawMessage, *Response, error) {
	var out struct {
		Options json.RawMessage `json:"options"`
	}
	resp, err := s.client.post(ctx, "auth/reauth/passkey/begin", nil, &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Options, resp, nil
}

// FinishReauthPasskey verifies the authenticator's assertion for the options
// from [AuthService.BeginReauthPasskey] and stamps the session.
func (s *AuthService) FinishReauthPasskey(ctx context.Context, assertion json.RawMessage, opts ...RequestOption) (*ReauthResult, *Response, error) {
	body := struct {
		Credential json.RawMessage `json:"credential"`
	}{Credential: assertion}
	return send[ReauthResult](ctx, s.client.post, "auth/reauth/passkey/finish", body, opts)
}

// BeginReauthSSO starts a confirmation with a linked provider and returns the
// authorization URL to open in a browser window, plus the binding to keep for
// [AuthService.FinishReauthSSO]. provider is [ReauthProvider.ID].
func (s *AuthService) BeginReauthSSO(ctx context.Context, provider string, opts ...RequestOption) (*SSORedirect, *Response, error) {
	body := struct {
		Provider string `json:"provider"`
	}{Provider: provider}
	return send[SSORedirect](ctx, s.client.post, "auth/reauth/sso/begin", body, opts)
}

// FinishReauthSSO redeems the single-use code the provider callback handed to
// the dashboard's /reauth/complete page, together with the binding from
// [AuthService.BeginReauthSSO], and stamps the session.
func (s *AuthService) FinishReauthSSO(ctx context.Context, code, binding string, opts ...RequestOption) (*ReauthResult, *Response, error) {
	body := struct {
		Code    string `json:"code"`
		Binding string `json:"binding"`
	}{Code: code, Binding: binding}
	return send[ReauthResult](ctx, s.client.post, "auth/reauth/sso/finish", body, opts)
}

// BeginReauthNative returns the nonce a native app passes to Apple or Google
// when it asks for the ID token [AuthService.ReauthNative] takes. It is for
// native app clients; a server integration normally has no use for it.
func (s *AuthService) BeginReauthNative(ctx context.Context, opts ...RequestOption) (string, *Response, error) {
	var out struct {
		Nonce string `json:"nonce"`
	}
	resp, err := s.client.post(ctx, "auth/reauth/native/begin", nil, &out, opts...)
	if err != nil {
		return "", resp, err
	}
	return out.Nonce, resp, nil
}

// ReauthNative confirms with a native Apple or Google ID token that carries
// the nonce from [AuthService.BeginReauthNative] and stamps the session.
// provider is one of [ReauthMethods.NativeProviders].
func (s *AuthService) ReauthNative(ctx context.Context, provider, idToken string, opts ...RequestOption) (*ReauthResult, *Response, error) {
	body := struct {
		Provider string `json:"provider"`
		IDToken  string `json:"id_token"`
	}{Provider: provider, IDToken: idToken}
	return send[ReauthResult](ctx, s.client.post, "auth/reauth/native", body, opts)
}

// CLIAuthRequestFor is [AuthService.CLIAuthRequest] with a workspace picked:
// the result also carries [CLIAuthRequest.GrantedScopes], what approving into
// that workspace would mint given the caller's role there.
func (s *AuthService) CLIAuthRequestFor(ctx context.Context, userCode, organizationID string, opts ...RequestOption) (*CLIAuthRequest, *Response, error) {
	q := url.Values{}
	q.Set("organization_id", organizationID)
	return fetch[CLIAuthRequest](ctx, s.client, withQuery("auth/cli/codes/"+url.PathEscape(userCode), q), opts)
}
