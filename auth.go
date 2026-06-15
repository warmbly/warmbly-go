package warmbly

import (
	"net/http"
	"time"
)

// Authenticator attaches credentials to an outgoing request. The SDK ships with
// API-key and OAuth access-token authenticators; you may also supply your own
// via [WithAuthenticator].
type Authenticator interface {
	// authenticate adds the credentials to req. It may be called once per
	// request (including retries), so implementations that refresh tokens
	// should cache aggressively.
	authenticate(req *http.Request) error
}

// AuthenticatorFunc adapts an ordinary function to an [Authenticator].
type AuthenticatorFunc func(req *http.Request) error

func (f AuthenticatorFunc) authenticate(req *http.Request) error { return f(req) }

// apiKeyAuth authenticates with a Warmbly API key (prefixed "wmbly_").
type apiKeyAuth struct{ key string }

func (a apiKeyAuth) authenticate(req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+a.key)
	return nil
}

// accessTokenAuth authenticates with a static OAuth 2.1 access token
// (prefixed "wmblyo_").
type accessTokenAuth struct{ token string }

func (a accessTokenAuth) authenticate(req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+a.token)
	return nil
}

// tokenSourceAuth authenticates with a [TokenSource], refreshing as needed.
type tokenSourceAuth struct{ src TokenSource }

func (a tokenSourceAuth) authenticate(req *http.Request) error {
	tok, err := a.src.Token()
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", tok.Type()+" "+tok.AccessToken)
	return nil
}

// Token is an OAuth 2.1 token as returned by the Warmbly token endpoint.
type Token struct {
	// AccessToken is the bearer token used to authenticate requests.
	AccessToken string `json:"access_token"`
	// TokenType is the token type; the API returns "Bearer".
	TokenType string `json:"token_type"`
	// RefreshToken, when present, can be exchanged for a fresh access token.
	RefreshToken string `json:"refresh_token,omitempty"`
	// Scope is the space-delimited set of granted scopes.
	Scope string `json:"scope,omitempty"`
	// ExpiresIn is the lifetime in seconds as returned by the server. It is
	// used to compute Expiry and is not otherwise meaningful afterwards.
	ExpiresIn int64 `json:"expires_in,omitempty"`
	// Expiry is the absolute time at which AccessToken expires. A zero value
	// means the token never expires (or expiry is unknown).
	Expiry time.Time `json:"-"`
}

// Type returns the token's HTTP Authorization scheme, defaulting to "Bearer".
func (t *Token) Type() string {
	if t.TokenType == "" {
		return "Bearer"
	}
	// Normalise common casings to the canonical "Bearer".
	switch t.TokenType {
	case "bearer", "BEARER", "Bearer":
		return "Bearer"
	default:
		return t.TokenType
	}
}

// expiryDelta is the safety margin treated as "already expired" so callers
// refresh slightly ahead of the true expiry.
const expiryDelta = 10 * time.Second

// Valid reports whether the token is non-empty and not (about to be) expired.
func (t *Token) Valid() bool {
	if t == nil || t.AccessToken == "" {
		return false
	}
	if t.Expiry.IsZero() {
		return true
	}
	return t.Expiry.Round(0).After(timeNow().Add(expiryDelta))
}

// TokenSource yields valid tokens on demand, refreshing transparently. It is
// the same contract as golang.org/x/oauth2.TokenSource, so existing sources
// interoperate via a thin adapter.
type TokenSource interface {
	// Token returns a valid token or an error.
	Token() (*Token, error)
}

// StaticTokenSource returns a [TokenSource] that always yields tok. It never
// refreshes and is mainly useful in tests or when a token is injected from the
// environment.
func StaticTokenSource(tok *Token) TokenSource {
	return staticTokenSource{tok}
}

type staticTokenSource struct{ tok *Token }

func (s staticTokenSource) Token() (*Token, error) { return s.tok, nil }

// timeNow is overridable in tests.
var timeNow = time.Now
