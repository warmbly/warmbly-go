package warmbly

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// OAuth2Endpoint holds the URLs of the Warmbly authorization server. The zero
// value falls back to [DefaultEndpoint].
type OAuth2Endpoint struct {
	// AuthURL is the user-facing authorization (consent) URL.
	AuthURL string
	// TokenURL is the token endpoint (code exchange, refresh, client creds).
	TokenURL string
	// RevokeURL is the token revocation endpoint.
	RevokeURL string
}

// DefaultEndpoint is the production Warmbly authorization server.
var DefaultEndpoint = OAuth2Endpoint{
	AuthURL:   "https://app.warmbly.com/oauth/authorize",
	TokenURL:  "https://api.warmbly.com/v1/oauth/token",
	RevokeURL: "https://api.warmbly.com/v1/oauth/revoke",
}

// OAuth2Config configures the OAuth 2.1 authorization-code flow (with optional
// PKCE) for an application acting on behalf of a user.
//
// A typical web flow:
//
//	cfg := &warmbly.OAuth2Config{
//		ClientID:     "...",
//		ClientSecret: "...",
//		RedirectURL:  "https://app.example.com/callback",
//		Scopes:       []string{"campaigns:read", "contacts:read"},
//	}
//	verifier := warmbly.GenerateVerifier()
//	url := cfg.AuthCodeURL(state, warmbly.S256ChallengeOption(verifier))
//	// ... redirect the user, receive ?code=... on the callback ...
//	tok, err := cfg.Exchange(ctx, code, warmbly.VerifierOption(verifier))
//	client, err := cfg.NewClient(ctx, tok)
type OAuth2Config struct {
	// ClientID is the application's public client identifier.
	ClientID string
	// ClientSecret is the confidential client secret. Leave empty for public
	// clients that rely solely on PKCE.
	ClientSecret string
	// RedirectURL must exactly match one of the application's registered URIs.
	RedirectURL string
	// Scopes are the permission keys to request.
	Scopes []string
	// Endpoint overrides the authorization server URLs. Zero uses
	// [DefaultEndpoint].
	Endpoint OAuth2Endpoint
	// HTTPClient is used for token requests. Nil uses a sensible default.
	HTTPClient *http.Client
}

func (c *OAuth2Config) authURL() string {
	if c.Endpoint.AuthURL != "" {
		return c.Endpoint.AuthURL
	}
	return DefaultEndpoint.AuthURL
}

func (c *OAuth2Config) tokenURL() string {
	if c.Endpoint.TokenURL != "" {
		return c.Endpoint.TokenURL
	}
	return DefaultEndpoint.TokenURL
}

func (c *OAuth2Config) revokeURL() string {
	if c.Endpoint.RevokeURL != "" {
		return c.Endpoint.RevokeURL
	}
	return DefaultEndpoint.RevokeURL
}

func (c *OAuth2Config) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: defaultTimeout}
}

// AuthCodeURL builds the URL to which the user should be redirected to grant
// authorization. state is an opaque, unguessable value echoed back to the
// redirect URI; verify it to defend against CSRF. Pass [S256ChallengeOption] to
// use PKCE (strongly recommended, and required for public clients).
func (c *OAuth2Config) AuthCodeURL(state string, opts ...AuthCodeOption) string {
	v := url.Values{
		"response_type": {"code"},
		"client_id":     {c.ClientID},
	}
	if c.RedirectURL != "" {
		v.Set("redirect_uri", c.RedirectURL)
	}
	if len(c.Scopes) > 0 {
		v.Set("scope", strings.Join(c.Scopes, " "))
	}
	if state != "" {
		v.Set("state", state)
	}
	for _, o := range opts {
		o.setValue(v)
	}
	base := c.authURL()
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return base + sep + v.Encode()
}

// Exchange trades an authorization code for a token. When PKCE was used, pass
// [VerifierOption] with the same verifier supplied to [S256ChallengeOption].
func (c *OAuth2Config) Exchange(ctx context.Context, code string, opts ...AuthCodeOption) (*Token, error) {
	v := url.Values{
		"grant_type": {"authorization_code"},
		"code":       {code},
	}
	if c.RedirectURL != "" {
		v.Set("redirect_uri", c.RedirectURL)
	}
	for _, o := range opts {
		o.setValue(v)
	}
	return retrieveToken(ctx, c.httpClient(), c.tokenURL(), c.ClientID, c.ClientSecret, v)
}

// Refresh exchanges a refresh token for a fresh access token.
func (c *OAuth2Config) Refresh(ctx context.Context, refreshToken string) (*Token, error) {
	v := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}
	if len(c.Scopes) > 0 {
		v.Set("scope", strings.Join(c.Scopes, " "))
	}
	return retrieveToken(ctx, c.httpClient(), c.tokenURL(), c.ClientID, c.ClientSecret, v)
}

// Revoke revokes an access or refresh token.
func (c *OAuth2Config) Revoke(ctx context.Context, token string) error {
	return revokeToken(ctx, c.httpClient(), c.revokeURL(), c.ClientID, c.ClientSecret, token)
}

// TokenSource returns a [TokenSource] that starts from t and transparently
// refreshes the access token using its refresh token as it expires. It is safe
// for concurrent use.
func (c *OAuth2Config) TokenSource(ctx context.Context, t *Token) TokenSource {
	return &refreshTokenSource{ctx: ctx, cfg: c, t: t}
}

// NewClient builds a [*Client] authenticated with a refreshing token source
// seeded from t.
func (c *OAuth2Config) NewClient(ctx context.Context, t *Token, opts ...Option) (*Client, error) {
	all := append([]Option{WithTokenSource(c.TokenSource(ctx, t))}, opts...)
	return New(all...)
}

// refreshTokenSource refreshes via the authorization-code refresh token.
type refreshTokenSource struct {
	ctx context.Context
	cfg *OAuth2Config
	mu  sync.Mutex
	t   *Token
}

func (s *refreshTokenSource) Token() (*Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.t.Valid() {
		return s.t, nil
	}
	if s.t == nil || s.t.RefreshToken == "" {
		return nil, errors.New("warmbly: access token expired and no refresh token is available")
	}
	nt, err := s.cfg.Refresh(s.ctx, s.t.RefreshToken)
	if err != nil {
		return nil, err
	}
	// Carry the previous refresh token forward if the server did not rotate it.
	if nt.RefreshToken == "" {
		nt.RefreshToken = s.t.RefreshToken
	}
	s.t = nt
	return nt, nil
}

// ClientCredentialsConfig configures the OAuth 2.1 client-credentials grant for
// machine-to-machine access (no user involved).
type ClientCredentialsConfig struct {
	ClientID     string
	ClientSecret string
	Scopes       []string
	// TokenURL overrides the token endpoint. Empty uses [DefaultEndpoint].
	TokenURL string
	// EndpointParams are extra parameters added to the token request.
	EndpointParams url.Values
	// HTTPClient is used for token requests. Nil uses a sensible default.
	HTTPClient *http.Client
}

func (c *ClientCredentialsConfig) tokenURL() string {
	if c.TokenURL != "" {
		return c.TokenURL
	}
	return DefaultEndpoint.TokenURL
}

func (c *ClientCredentialsConfig) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: defaultTimeout}
}

// Token fetches a new access token using the client-credentials grant.
func (c *ClientCredentialsConfig) Token(ctx context.Context) (*Token, error) {
	v := url.Values{"grant_type": {"client_credentials"}}
	if len(c.Scopes) > 0 {
		v.Set("scope", strings.Join(c.Scopes, " "))
	}
	for k, vs := range c.EndpointParams {
		for _, val := range vs {
			v.Add(k, val)
		}
	}
	return retrieveToken(ctx, c.httpClient(), c.tokenURL(), c.ClientID, c.ClientSecret, v)
}

// TokenSource returns a [TokenSource] that fetches and caches a token, renewing
// it as it expires. It is safe for concurrent use.
func (c *ClientCredentialsConfig) TokenSource(ctx context.Context) TokenSource {
	return &clientCredentialsSource{ctx: ctx, cfg: c}
}

// NewClient builds a [*Client] authenticated with the client-credentials grant.
func (c *ClientCredentialsConfig) NewClient(ctx context.Context, opts ...Option) (*Client, error) {
	all := append([]Option{WithTokenSource(c.TokenSource(ctx))}, opts...)
	return New(all...)
}

type clientCredentialsSource struct {
	ctx context.Context
	cfg *ClientCredentialsConfig
	mu  sync.Mutex
	t   *Token
}

func (s *clientCredentialsSource) Token() (*Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.t.Valid() {
		return s.t, nil
	}
	nt, err := s.cfg.Token(s.ctx)
	if err != nil {
		return nil, err
	}
	s.t = nt
	return nt, nil
}

// --- PKCE helpers ---

// GenerateVerifier returns a high-entropy PKCE code verifier (RFC 7636): the
// base64url encoding of 32 random bytes (43 characters). Generate one per
// authorization request, pass it to [S256ChallengeOption], and supply the same
// value to [Exchange] via [VerifierOption].
func GenerateVerifier() string {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		// crypto/rand should never fail; panic mirrors the standard library.
		panic("warmbly: failed to read random bytes: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// S256ChallengeFromVerifier derives the S256 code challenge from a verifier.
func S256ChallengeFromVerifier(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// AuthCodeOption customizes the parameters of an authorization or token request.
type AuthCodeOption interface {
	setValue(url.Values)
}

type setParam struct{ kvs map[string]string }

func (p setParam) setValue(v url.Values) {
	for k, val := range p.kvs {
		v.Set(k, val)
	}
}

// SetAuthURLParam sets an arbitrary key/value parameter on the request.
func SetAuthURLParam(key, value string) AuthCodeOption {
	return setParam{kvs: map[string]string{key: value}}
}

// S256ChallengeOption adds an S256 PKCE challenge derived from verifier to an
// [OAuth2Config.AuthCodeURL] call.
func S256ChallengeOption(verifier string) AuthCodeOption {
	return setParam{kvs: map[string]string{
		"code_challenge_method": "S256",
		"code_challenge":        S256ChallengeFromVerifier(verifier),
	}}
}

// VerifierOption supplies the PKCE code verifier to an [OAuth2Config.Exchange]
// call.
func VerifierOption(verifier string) AuthCodeOption {
	return setParam{kvs: map[string]string{"code_verifier": verifier}}
}

// OAuth2Error is returned when the token or revocation endpoint responds with an
// error. It carries both the RFC 6749 fields (Code/Description) and the
// Warmbly error envelope fields (Message/RequestID).
type OAuth2Error struct {
	StatusCode  int    `json:"-"`
	Code        string `json:"error"`
	Description string `json:"error_description"`
	URI         string `json:"error_uri"`
	Message     string `json:"message"`
	RequestID   string `json:"request_id"`
	// Body is the raw error-response body for diagnostics. It is only populated
	// for non-2xx responses (never a successful token payload), so it does not
	// carry issued credentials.
	Body []byte `json:"-"`
}

func (e *OAuth2Error) Error() string {
	msg := e.Description
	if msg == "" {
		msg = e.Message
	}
	code := e.Code
	if code == "" {
		code = http.StatusText(e.StatusCode)
	}
	s := fmt.Sprintf("warmbly: oauth2: %d %s", e.StatusCode, code)
	if msg != "" {
		s += ": " + msg
	}
	if e.RequestID != "" {
		s += fmt.Sprintf(" [request_id=%s]", e.RequestID)
	}
	return s
}

func retrieveToken(ctx context.Context, hc *http.Client, tokenURL, clientID, clientSecret string, v url.Values) (*Token, error) {
	basic := clientID != "" && clientSecret != ""
	if !basic && clientID != "" {
		v.Set("client_id", clientID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(v.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if basic {
		// RFC 6749 §2.3.1: form-urlencode the credentials before Basic auth.
		req.SetBasicAuth(url.QueryEscape(clientID), url.QueryEscape(clientSecret))
	}

	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseOAuth2Error(resp.StatusCode, body)
	}

	tok, err := parseToken(body, resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}
	if tok.AccessToken == "" {
		// Do not attach the raw body: this is a 2xx response that may carry
		// other credentials (e.g. a refresh_token), and OAuth2Error values are
		// often logged.
		return nil, &OAuth2Error{StatusCode: resp.StatusCode, Code: "server_error", Description: "token endpoint returned no access_token"}
	}
	return tok, nil
}

func revokeToken(ctx context.Context, hc *http.Client, revokeURL, clientID, clientSecret, token string) error {
	v := url.Values{"token": {token}}
	basic := clientID != "" && clientSecret != ""
	if !basic && clientID != "" {
		v.Set("client_id", clientID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, revokeURL, strings.NewReader(v.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if basic {
		req.SetBasicAuth(url.QueryEscape(clientID), url.QueryEscape(clientSecret))
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseOAuth2Error(resp.StatusCode, body)
	}
	return nil
}

func parseToken(body []byte, contentType string) (*Token, error) {
	tok := &Token{}

	// Token endpoints return JSON in practice. Detect it by content type or by
	// the body shape, since some servers omit (or mis-sniff) the header. Fall
	// back to a form-encoded body only when it is clearly not JSON.
	trimmed := strings.TrimSpace(string(body))
	isJSON := strings.Contains(contentType, "json") || strings.HasPrefix(trimmed, "{")
	isForm := strings.Contains(contentType, "application/x-www-form-urlencoded")
	// Detect a form body even when the Content-Type is missing or generic, while
	// keeping JSON the default when the shape is ambiguous.
	looksForm := !isJSON && (isForm || (trimmed != "" && !strings.HasPrefix(trimmed, "{") && strings.Contains(trimmed, "=")))

	if looksForm {
		vals, err := url.ParseQuery(trimmed)
		if err != nil {
			return nil, fmt.Errorf("warmbly: oauth2: parse token response: %w", err)
		}
		tok.AccessToken = vals.Get("access_token")
		tok.TokenType = vals.Get("token_type")
		tok.RefreshToken = vals.Get("refresh_token")
		tok.Scope = vals.Get("scope")
		if e := vals.Get("expires_in"); e != "" {
			if secs, perr := strconv.ParseInt(e, 10, 64); perr == nil {
				tok.ExpiresIn = secs
			}
		}
	} else {
		// Decode via a shadow struct so expires_in tolerates both numeric and
		// quoted-string forms (some servers emit "expires_in":"3600").
		var raw struct {
			AccessToken  string      `json:"access_token"`
			TokenType    string      `json:"token_type"`
			RefreshToken string      `json:"refresh_token"`
			Scope        string      `json:"scope"`
			ExpiresIn    json.Number `json:"expires_in"`
		}
		if err := json.Unmarshal(body, &raw); err != nil {
			return nil, fmt.Errorf("warmbly: oauth2: decode token response: %w", err)
		}
		tok.AccessToken = raw.AccessToken
		tok.TokenType = raw.TokenType
		tok.RefreshToken = raw.RefreshToken
		tok.Scope = raw.Scope
		if raw.ExpiresIn != "" {
			if secs, perr := raw.ExpiresIn.Int64(); perr == nil {
				tok.ExpiresIn = secs
			}
		}
	}
	if tok.ExpiresIn > 0 {
		tok.Expiry = timeNow().Add(time.Duration(tok.ExpiresIn) * time.Second)
	}
	return tok, nil
}

func parseOAuth2Error(status int, body []byte) *OAuth2Error {
	e := &OAuth2Error{StatusCode: status, Body: body}
	if len(strings.TrimSpace(string(body))) > 0 {
		_ = json.Unmarshal(body, e)
	}
	return e
}
