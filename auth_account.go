package warmbly

import (
	"context"
	"encoding/json"
	"net/url"
	"time"
)

// LinkedIdentity is a provider the signed-in person can sign in with.
type LinkedIdentity struct {
	// Provider is "google", "apple" or "oidc" (single sign-on). Pass it to
	// [AuthService.UnlinkIdentity].
	Provider string `json:"provider"`
	// Label is the provider's display name.
	Label       string     `json:"label"`
	Email       string     `json:"email,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

// LinkedIdentities lists the providers the signed-in person can sign in with.
// It is a session call (a person's login), not an API key one.
func (s *AuthService) LinkedIdentities(ctx context.Context, opts ...RequestOption) ([]LinkedIdentity, *Response, error) {
	return fetchData[LinkedIdentity](ctx, s.client, "auth/me/identities", opts)
}

// UnlinkIdentity removes one provider from the signed-in person's sign-in
// methods. It needs a recently confirmed session (see [AuthService.Reauth])
// and is refused with a 4xx when it would leave the account with no way to
// sign in. Linking happens through the provider sign-in itself, never here.
func (s *AuthService) UnlinkIdentity(ctx context.Context, provider string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "auth/me/identities/"+url.PathEscape(provider), opts...)
}

// SendPasswordLink emails the signed-in person a link to set a password, for
// an account that signs in through a provider or a passkey and has none.
func (s *AuthService) SendPasswordLink(ctx context.Context, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "auth/me/password/link", nil, nil, opts...)
}

// RegenerateRecoveryCodes replaces the account's two-factor recovery codes and
// returns the new set, shown once. It requires a current authenticator code or
// an unused recovery code, and counts against the same attempt budget as
// [AuthService.DisableTwoFA].
func (s *AuthService) RegenerateRecoveryCodes(ctx context.Context, code string, opts ...RequestOption) (*TwoFARecoveryCodes, *Response, error) {
	body := struct {
		Code string `json:"code"`
	}{Code: code}
	return send[TwoFARecoveryCodes](ctx, s.client.post, "auth/2fa/recovery-codes", body, opts)
}

// LoginInitialPassword answers a sign-in that returned
// [AuthStep.PasswordChangeRequired]: pendingToken is the challenge it carried
// and password the person's own new password. The sign-in then continues as
// [AuthService.Login] would (emailed code, two-factor or a session). It needs
// no credentials. deviceToken is the optional remembered-browser token, see
// [Session.DeviceToken].
func (s *AuthService) LoginInitialPassword(ctx context.Context, pendingToken, password, deviceToken string, opts ...RequestOption) (*AuthStep, *Response, error) {
	body := struct {
		PendingToken string `json:"pending_token"`
		Password     string `json:"password"`
		DeviceToken  string `json:"device_token,omitempty"`
	}{PendingToken: pendingToken, Password: password, DeviceToken: deviceToken}
	return send[AuthStep](ctx, s.client.post, "auth/login/initial-password", body, opts)
}

// LinkSSO completes a provider sign-in that returned [Session.LinkRequired]:
// pendingToken is the challenge it carried and password the password of the
// existing account it resolved to. The identity is attached and the session
// issued (or the two-factor challenge, see [Session.TwoFARequired]). It needs
// no credentials.
func (s *AuthService) LinkSSO(ctx context.Context, pendingToken, password string, opts ...RequestOption) (*Session, *Response, error) {
	body := struct {
		PendingToken string `json:"pending_token"`
		Password     string `json:"password"`
	}{PendingToken: pendingToken, Password: password}
	return send[Session](ctx, s.client.post, "auth/sso/link", body, opts)
}

// ResetSession is the session an emailed reset link opens, for
// [AuthService.ResetPasswordConfirm].
type ResetSession struct {
	// Session is the handle to pass to ResetPasswordConfirm.
	Session   string    `json:"session"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ResetPasswordSession exchanges the token from an emailed reset link for the
// reset session [AuthService.ResetPasswordConfirm] takes, so the token never
// has to travel in a URL again. It needs no credentials.
func (s *AuthService) ResetPasswordSession(ctx context.Context, linkToken string, opts ...RequestOption) (*ResetSession, *Response, error) {
	body := struct {
		Token string `json:"token"`
	}{Token: linkToken}
	return send[ResetSession](ctx, s.client.post, "auth/reset-password/session", body, opts)
}

// ReauthProvider is a linked identity a person can re-verify with in a
// browser window.
type ReauthProvider struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// ReauthMethods lists the ways the signed-in account can confirm it is the
// holder, so a client offers only those.
type ReauthMethods struct {
	Password bool `json:"password"`
	TOTP     bool `json:"totp"`
	Passkey  bool `json:"passkey"`
	// Providers run in a browser window ([AuthService.ReauthSSOBegin]);
	// NativeProviders take an ID token from the native app
	// ([AuthService.ReauthNative]).
	Providers       []ReauthProvider `json:"providers"`
	NativeProviders []string         `json:"native_providers"`
	// ValidForSeconds is how long a confirmation lasts.
	ValidForSeconds int `json:"valid_for_seconds"`
}

// ReauthResult is the outcome of a confirmation.
type ReauthResult struct {
	// ValidForSeconds is how long the confirmation lasts, so a client can
	// decide whether to ask again rather than letting the next call fail.
	ValidForSeconds int `json:"valid_for_seconds"`
}

// ReauthMethods lists the ways the signed-in account can confirm it is the
// holder.
//
// Sensitive account changes (minting an API key, registering a passkey,
// unlinking an identity, scheduling a deletion) need a recent confirmation of
// the session, which the Reauth* calls give. They are session calls: an API
// key has no session to stamp.
func (s *AuthService) ReauthMethods(ctx context.Context, opts ...RequestOption) (*ReauthMethods, *Response, error) {
	return fetch[ReauthMethods](ctx, s.client, "auth/reauth/methods", opts)
}

// Reauth confirms the account holder with a password, a current authenticator
// code or recovery code, or both. An account with no password confirms with
// its code, and an account with no two-factor with its password. Too many
// misses within an hour fail with code "reauth_limited".
func (s *AuthService) Reauth(ctx context.Context, password, code string, opts ...RequestOption) (*ReauthResult, *Response, error) {
	body := struct {
		Password string `json:"password,omitempty"`
		Code     string `json:"code,omitempty"`
	}{Password: password, Code: code}
	return send[ReauthResult](ctx, s.client.post, "auth/reauth", body, opts)
}

// ReauthPasskeyBegin asks for one of the account's own passkeys and returns
// the WebAuthn request options to hand to the authenticator.
func (s *AuthService) ReauthPasskeyBegin(ctx context.Context, opts ...RequestOption) (json.RawMessage, *Response, error) {
	var out struct {
		Options json.RawMessage `json:"options"`
	}
	resp, err := s.client.post(ctx, "auth/reauth/passkey/begin", nil, &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Options, resp, nil
}

// ReauthPasskeyFinish verifies the authenticator's assertion over the options
// from [AuthService.ReauthPasskeyBegin] and confirms the session.
func (s *AuthService) ReauthPasskeyFinish(ctx context.Context, credential json.RawMessage, opts ...RequestOption) (*ReauthResult, *Response, error) {
	body := struct {
		Credential json.RawMessage `json:"credential"`
	}{Credential: credential}
	return send[ReauthResult](ctx, s.client.post, "auth/reauth/passkey/finish", body, opts)
}

// ReauthSSOBegin starts a provider authorization for the account's linked
// identity. Open [SSORedirect.URL] in a browser window and keep the binding
// for [AuthService.ReauthSSOFinish].
func (s *AuthService) ReauthSSOBegin(ctx context.Context, provider string, opts ...RequestOption) (*SSORedirect, *Response, error) {
	body := struct {
		Provider string `json:"provider"`
	}{Provider: provider}
	return send[SSORedirect](ctx, s.client.post, "auth/reauth/sso/begin", body, opts)
}

// ReauthSSOFinish redeems the provider callback's code with the binding from
// [AuthService.ReauthSSOBegin] and confirms the session.
func (s *AuthService) ReauthSSOFinish(ctx context.Context, code, binding string, opts ...RequestOption) (*ReauthResult, *Response, error) {
	body := struct {
		Code    string `json:"code"`
		Binding string `json:"binding"`
	}{Code: code, Binding: binding}
	return send[ReauthResult](ctx, s.client.post, "auth/reauth/sso/finish", body, opts)
}

// ReauthNativeBegin returns the nonce a native app passes to Apple or Google
// when it asks for the ID token for [AuthService.ReauthNative].
func (s *AuthService) ReauthNativeBegin(ctx context.Context, opts ...RequestOption) (string, *Response, error) {
	var out struct {
		Nonce string `json:"nonce"`
	}
	resp, err := s.client.post(ctx, "auth/reauth/native/begin", nil, &out, opts...)
	if err != nil {
		return "", resp, err
	}
	return out.Nonce, resp, nil
}

// ReauthNative confirms the session with a native Apple or Google ID token
// carrying the nonce from [AuthService.ReauthNativeBegin]. provider is "apple"
// or "google".
func (s *AuthService) ReauthNative(ctx context.Context, provider, idToken string, opts ...RequestOption) (*ReauthResult, *Response, error) {
	body := struct {
		Provider string `json:"provider"`
		IDToken  string `json:"id_token"`
	}{Provider: provider, IDToken: idToken}
	return send[ReauthResult](ctx, s.client.post, "auth/reauth/native", body, opts)
}
