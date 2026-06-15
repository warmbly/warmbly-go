# warmbly-go

The official Go SDK for the Warmbly cold-outreach & mailbox-warmup platform.

[![Go Reference](https://pkg.go.dev/badge/github.com/warmbly/warmbly-go.svg)](https://pkg.go.dev/github.com/warmbly/warmbly-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/warmbly/warmbly-go)](https://goreportcard.com/report/github.com/warmbly/warmbly-go)
[![CI](https://github.com/warmbly/warmbly-go/actions/workflows/ci.yml/badge.svg)](https://github.com/warmbly/warmbly-go/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/warmbly/warmbly-go/branch/main/graph/badge.svg)](https://codecov.io/gh/warmbly/warmbly-go)
[![Go Version](https://img.shields.io/github/go-mod/go-version/warmbly/warmbly-go)](https://github.com/warmbly/warmbly-go/blob/main/go.mod)
[![Release](https://img.shields.io/github/v/release/warmbly/warmbly-go?sort=semver)](https://github.com/warmbly/warmbly-go/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

`warmbly-go` is a fully typed client for the Warmbly REST API and its real-time event gateway. It covers everything from API-key and OAuth 2.1 authentication to campaigns, contacts, emails, webhooks, and analytics, plus a persistent gateway connection for live engagement events. **It has zero external dependencies** — the entire module is built on the Go standard library, including a dependency-free RFC 6455 WebSocket implementation, so adding it to your project pulls in nothing but Warmbly itself.

## Features

- **Typed REST client.** Services hang off a single `Client`: `client.APIKeys`, `client.OAuthApps`, `client.Emails`, `client.Campaigns`, `client.Contacts`, `client.Webhooks`, `client.Templates`, `client.Analytics`, and `client.Organization`.
- **Flexible authentication.** API keys via `warmbly.WithAPIKey`, or OAuth 2.1 access tokens via `warmbly.WithAccessToken` / `warmbly.WithTokenSource`. Full OAuth client flows: authorization-code with PKCE and client-credentials, plus OAuth application management.
- **Robust by default.** Automatic retries with exponential backoff and jitter (honouring `Retry-After`), rate-limit header parsing, and cursor pagination with a Go 1.23 auto-paging iterator.
- **Typed errors.** A decoded `*warmbly.Error` carrying the request ID and message, matchable with `errors.Is` against sentinels such as `warmbly.ErrNotFound`, `warmbly.ErrUnauthorized`, and `warmbly.ErrRateLimited`.
- **Webhook signature verification.** HMAC-SHA256 verification via `warmbly.VerifyWebhookSignature` and `client.Webhooks.ConstructEvent`.
- **Real-time gateway.** A persistent connection with intent-based subscriptions, typed event handlers, automatic heartbeat, session resume, and reconnect.
- **Zero dependencies.** Standard library only. No transitive supply chain to audit.

## Installation

```
go get github.com/warmbly/warmbly-go
```

Requires **Go 1.23+** (the auto-paging iterator uses range-over-func).

## Quickstart

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/warmbly/warmbly-go"
)

func main() {
    client, err := warmbly.New(warmbly.WithAPIKey("wmbly_..."))
    if err != nil {
        log.Fatal(err)
    }

    ctx := context.Background()
    page, err := client.Campaigns.List(ctx, nil)
    if err != nil {
        log.Fatal(err)
    }
    for campaign, err := range page.All(ctx) {
        if err != nil {
            log.Fatal(err)
        }
        fmt.Println(campaign.Name)
    }
}
```

## Authentication

### API keys

The simplest way to authenticate is with a Warmbly API key (they start with the `wmbly_` prefix):

```go
client, err := warmbly.New(warmbly.WithAPIKey("wmbly_..."))
if err != nil {
    log.Fatal(err)
}
```

### OAuth 2.1

For applications acting on behalf of users, use the OAuth 2.1 authorization-code flow with PKCE:

```go
cfg := &warmbly.OAuth2Config{
    ClientID:     "...",
    ClientSecret: "...",
    RedirectURL:  "https://app.example.com/callback",
    Scopes:       []string{"campaigns:read", "contacts:read"},
}
verifier := warmbly.GenerateVerifier()
authURL := cfg.AuthCodeURL("state-xyz", warmbly.S256ChallengeOption(verifier))
// redirect the user to authURL; on the callback:
tok, err := cfg.Exchange(ctx, code, warmbly.VerifierOption(verifier))
client, err := cfg.NewClient(ctx, tok)
```

For machine-to-machine access, use the client-credentials flow via `warmbly.ClientCredentialsConfig`. You can register, list, and manage your OAuth applications programmatically through `client.OAuthApps`.

If you already hold an access token, authenticate directly with `warmbly.WithAccessToken`:

```go
client, err := warmbly.New(warmbly.WithAccessToken("..."))
```

> **Transparent refresh.** Pass a token source with `warmbly.WithTokenSource` to have the client fetch and refresh tokens automatically, so requests never fail on an expired access token. The configs returned by the OAuth flows produce clients backed by a refreshing token source out of the box.

## Working with resources

Every resource is exposed as a service on the `Client`. Listing returns a page that you can iterate with the auto-paging iterator; individual records are fetched by ID, and most resources support creation:

```go
ctx := context.Background()

// List with automatic pagination.
page, err := client.Campaigns.List(ctx, nil)
if err != nil {
    log.Fatal(err)
}
for campaign, err := range page.All(ctx) {
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(campaign.Name)
}

// Fetch a single record by ID (single-record calls also return the *Response).
campaign, _, err := client.Campaigns.Get(ctx, "camp_123")
if err != nil {
    log.Fatal(err)
}

// Create a new record.
created, _, err := client.Campaigns.Create(ctx, &warmbly.CampaignCreateParams{
    Name: "Q3 outbound",
})
if err != nil {
    log.Fatal(err)
}
fmt.Println(created.ID)
```

The full set of services: `client.APIKeys`, `client.OAuthApps`, `client.Emails`, `client.Campaigns`, `client.Contacts`, `client.Webhooks`, `client.Templates`, `client.Analytics`, and `client.Organization`.

## Pagination

List endpoints use cursor-based pagination. Each call returns a page object, and rather than threading cursors through your own loop you can range over `page.All(ctx)` — a Go 1.23 range-over-func iterator that transparently fetches subsequent pages as you consume items, stopping on the first error:

```go
page, err := client.Emails.List(ctx, nil)
if err != nil {
    log.Fatal(err)
}
for email, err := range page.All(ctx) {
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(email.Email)
}
```

The iterator yields both a value and an error on each step, so per-page fetch failures surface inline; `break` out of the loop at any time to stop paging.

## Errors

API failures are decoded into a typed `*warmbly.Error`. Match well-known conditions with `errors.Is`, or unwrap the full error with `errors.As` to read details such as the request ID:

```go
if errors.Is(err, warmbly.ErrNotFound) {
    // ...
}
var apiErr *warmbly.Error
if errors.As(err, &apiErr) {
    log.Printf("request %s failed: %s", apiErr.RequestID, apiErr.Message)
}
```

Key sentinels include `warmbly.ErrNotFound`, `warmbly.ErrUnauthorized`, and `warmbly.ErrRateLimited`.

## Retries & rate limits

The client automatically retries transient failures using exponential backoff with jitter, and honours the `Retry-After` header when the server sends one. Rate-limit headers from each response are parsed and exposed on the returned `*Response` (`resp.RateLimit`) so you can observe your remaining quota. Tune retry behaviour with the `warmbly.WithMaxRetries` option:

```go
client, err := warmbly.New(
    warmbly.WithAPIKey("wmbly_..."),
    warmbly.WithMaxRetries(5),
)
```

## Real-time gateway

The `gateway` subpackage maintains a persistent connection to Warmbly's event gateway and delivers typed events as they happen. You subscribe to **intents** (categories of events) and register typed handlers with `gateway.On`; the gateway takes care of heartbeats, session resume, and reconnection automatically.

```go
import "github.com/warmbly/warmbly-go/gateway"

g := gateway.New(apiKey, gateway.WithIntents(gateway.IntentEmailEngagement|gateway.IntentCampaigns))
gateway.On(g, gateway.EventEmailOpened, func(ctx context.Context, e *gateway.EngagementEvent) {
    log.Printf("contact %s opened a message", e.ContactID)
})
if err := g.Open(ctx); err != nil {
    log.Fatal(err)
}
defer g.Close()
<-ctx.Done()
```

The underlying WebSocket transport is a dependency-free RFC 6455 implementation living in `internal/wsconn`, so the gateway adds no third-party packages either. See the [gateway package documentation](https://pkg.go.dev/github.com/warmbly/warmbly-go/gateway) for the full list of intents, events, and handler types.

## Webhooks

Verify inbound webhook payloads before trusting them. `client.Webhooks.ConstructEvent` validates the HMAC-SHA256 signature (sent in the `X-Webhook-Signature` header) and returns the decoded event:

```go
event, err := client.Webhooks.ConstructEvent(body, r.Header.Get(warmbly.WebhookSignatureHeader), endpointSecret)
if err != nil {
    http.Error(w, "invalid signature", http.StatusBadRequest)
    return
}
```

For lower-level use you can call `warmbly.VerifyWebhookSignature` directly.

## Examples

The [`examples/`](examples/) directory has a runnable program for each part of the SDK — API keys, both OAuth flows, campaigns, contacts, emails/warmup, templates, analytics, webhooks, the real-time gateway, and error handling. See [`examples/README.md`](examples/README.md) for the full index.

## Versioning

`warmbly-go` follows semantic versioning. While the module is pre-1.0, the public API may change between minor releases; review the [release notes](https://github.com/warmbly/warmbly-go/releases) before upgrading.

## Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) and our [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) before opening an issue or pull request.

## License

Released under the MIT License. See [LICENSE](LICENSE) for details.
