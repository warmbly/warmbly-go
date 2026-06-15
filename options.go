package warmbly

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Option configures a [Client]. Options are applied in order by [New].
type Option func(*Client) error

// WithAPIKey authenticates the client with a Warmbly API key (prefixed
// "wmbly_"). This is the recommended scheme for server-to-server access.
func WithAPIKey(key string) Option {
	return func(c *Client) error {
		key = strings.TrimSpace(key)
		if key == "" {
			return fmt.Errorf("warmbly: API key must not be empty")
		}
		c.auth = apiKeyAuth{key: key}
		return nil
	}
}

// WithAccessToken authenticates the client with a static OAuth 2.1 access token
// (prefixed "wmblyo_"). Prefer [WithTokenSource] when you have a refresh token
// and want transparent renewal.
func WithAccessToken(token string) Option {
	return func(c *Client) error {
		token = strings.TrimSpace(token)
		if token == "" {
			return fmt.Errorf("warmbly: access token must not be empty")
		}
		c.auth = accessTokenAuth{token: token}
		return nil
	}
}

// WithTokenSource authenticates the client with a [TokenSource], refreshing the
// access token transparently as it expires.
func WithTokenSource(src TokenSource) Option {
	return func(c *Client) error {
		if src == nil {
			return fmt.Errorf("warmbly: token source must not be nil")
		}
		c.auth = tokenSourceAuth{src: src}
		return nil
	}
}

// WithAuthenticator sets a custom [Authenticator], for advanced scenarios not
// covered by the built-in schemes.
func WithAuthenticator(a Authenticator) Option {
	return func(c *Client) error {
		if a == nil {
			return fmt.Errorf("warmbly: authenticator must not be nil")
		}
		c.auth = a
		return nil
	}
}

// WithBaseURL overrides the API base URL (default https://api.warmbly.com/v1).
// Useful for targeting a self-hosted instance or a staging environment. The URL
// should include the version path segment.
func WithBaseURL(raw string) Option {
	return func(c *Client) error {
		u, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("warmbly: invalid base URL %q: %w", raw, err)
		}
		if u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("warmbly: base URL %q must be absolute", raw)
		}
		// Ensure the path ends with a slash so relative refs resolve cleanly.
		if !strings.HasSuffix(u.Path, "/") {
			u.Path += "/"
		}
		c.baseURL = u
		return nil
	}
}

// WithHTTPClient sets the underlying [*http.Client]. Use this to customize
// transport, proxies, TLS or timeouts. The client's Timeout, if any, applies to
// each individual request.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) error {
		if hc == nil {
			return fmt.Errorf("warmbly: HTTP client must not be nil")
		}
		c.httpClient = hc
		return nil
	}
}

// WithUserAgent appends a product token to the default User-Agent, so requests
// from your application are identifiable in Warmbly's logs.
func WithUserAgent(product string) Option {
	return func(c *Client) error {
		product = strings.TrimSpace(product)
		if product != "" {
			c.userAgent = product + " " + defaultUserAgent
		}
		return nil
	}
}

// WithMaxRetries sets how many times the client retries a request that fails
// with a 429 or 5xx response. Retries use exponential backoff with jitter and
// honor any Retry-After header. The default is 2; set 0 to disable retries.
func WithMaxRetries(n int) Option {
	return func(c *Client) error {
		if n < 0 {
			return fmt.Errorf("warmbly: max retries must be >= 0")
		}
		c.maxRetries = n
		return nil
	}
}

// WithRetryWaitBounds sets the minimum and maximum backoff between retries.
func WithRetryWaitBounds(minWait, maxWait time.Duration) Option {
	return func(c *Client) error {
		if minWait <= 0 || maxWait <= 0 || minWait > maxWait {
			return fmt.Errorf("warmbly: invalid retry wait bounds")
		}
		c.retryWaitMin = minWait
		c.retryWaitMax = maxWait
		return nil
	}
}

// WithHeader sets a default header sent on every request. It may be called
// multiple times to set several headers.
func WithHeader(key, value string) Option {
	return func(c *Client) error {
		if key == "" {
			return fmt.Errorf("warmbly: header key must not be empty")
		}
		c.defaultHeaders.Set(key, value)
		return nil
	}
}
