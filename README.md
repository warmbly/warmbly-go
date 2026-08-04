# warmbly-go

The official Go SDK for the Warmbly cold-outreach & mailbox-warmup platform.

[![Go Reference](https://pkg.go.dev/badge/github.com/warmbly/warmbly-go.svg)](https://pkg.go.dev/github.com/warmbly/warmbly-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/warmbly/warmbly-go)](https://goreportcard.com/report/github.com/warmbly/warmbly-go)
[![CI](https://github.com/warmbly/warmbly-go/actions/workflows/ci.yml/badge.svg)](https://github.com/warmbly/warmbly-go/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/warmbly/warmbly-go/branch/main/graph/badge.svg)](https://codecov.io/gh/warmbly/warmbly-go)
[![Go Version](https://img.shields.io/github/go-mod/go-version/warmbly/warmbly-go)](https://github.com/warmbly/warmbly-go/blob/main/go.mod)
[![Release](https://img.shields.io/github/v/release/warmbly/warmbly-go?sort=semver)](https://github.com/warmbly/warmbly-go/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

`warmbly-go` is a fully typed client for the Warmbly REST API and its realtime event gateway. It covers the whole customer-facing v1 surface — mailboxes and warmup, campaigns and sequences, contacts and CRM, the unified inbox, integrations and automations, AI generation and the Advisor, analytics, webhooks, keys and billing — plus a persistent gateway connection for live events. **It has zero external dependencies**: the entire module is built on the Go standard library, including a dependency-free RFC 6455 WebSocket implementation, so adding it pulls in nothing but Warmbly itself.

## Features

- **The whole API, typed.** Every service hangs off one `Client`; see [Services](#services) for the map.
- **Flexible authentication.** API keys via `warmbly.WithAPIKey`, OAuth 2.1 access tokens via `warmbly.WithAccessToken` / `warmbly.WithTokenSource`, and session tokens from `client.Auth.Login` for the routes an API key deliberately cannot reach. Full OAuth client flows: authorization-code with PKCE and client-credentials.
- **Safe retries.** Exponential backoff with jitter honoring `Retry-After`, plus per-request `Idempotency-Key` support so retrying a send never sends twice.
- **Typed errors.** A decoded `*warmbly.Error` carrying the request ID and message, matchable with `errors.Is` against sentinels such as `warmbly.ErrNotFound` and `warmbly.ErrRateLimited`.
- **Cursor pagination.** A generic `Page[T]` with a Go 1.23 auto-paging iterator.
- **Webhook verification.** Signature and replay checking via `client.Webhooks.ConstructEvent`.
- **Realtime gateway.** A persistent connection with intent filtering, typed handlers, heartbeats, reconnection and sequence-based replay of missed events.
- **Forward compatible.** `client.Do` reaches an endpoint this release does not model yet, and `WithQueryParam` adds a filter that landed after it shipped.
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

### Session tokens

Some of the API is deliberately unreachable with a long-lived key: workspace governance, billing, and the AI assistant all act as a named person rather than an integration. `client.Auth` signs a user in and yields a session token for those.

Sign-in is two steps. The first emails a code; the second exchanges it for tokens. An account with two-factor enabled takes one more, through `Auth.VerifyTwoFA`.

```go
session, _, err := client.Auth.Login(ctx, &warmbly.LoginParams{Email: email, Password: password})
tokens, _, err := client.Auth.LoginConfirm(ctx, &warmbly.ConfirmParams{Session: session, Code: emailedCode})

authed, err := warmbly.New(warmbly.WithAccessToken(tokens.AccessToken))
org, _, err := authed.Organization.Current(ctx)
```

An API key used on one of these routes gets a clean `warmbly.ErrUnauthorized` rather than a confusing failure.

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

## Services

| Service | What it covers |
| --- | --- |
| `client.Emails` | Connected mailboxes, warmup lifecycle, domain authentication, address verification, one-off sends |
| `client.Campaigns` | Campaigns, sequence steps, A/B variants, attachments, senders, preflight, template preview |
| `client.Contacts` | Contacts and the 360 view, faceted search, CRM notes, import and export, AI research |
| `client.Unibox` | Unified inbox: reading, replying, composing, labels, snoozes, scheduled sends, AI drafts |
| `client.Templates` | Reply templates, spam scoring, rendering, ordering |
| `client.Analytics` | Dashboard, per-campaign engagement, warmup progress, deliverability health, plan usage |
| `client.Advisor` | Continuous checks on sending posture, with one-click and agent fixes |
| `client.CRM` | Pipelines, deals, task types and the task board |
| `client.Teams` | Named groups of members for CRM assignment |
| `client.Meetings` | Calls booked through a connected scheduler |
| `client.Integrations` | Third-party connections, event subscriptions, field mappings, contact pushes |
| `client.Automations` | Event-triggered flows across those connections |
| `client.LeadSync` | Google Sheets to contacts sync |
| `client.Generation` | AI writing and rewriting |
| `client.Skills` | Workspace AI playbooks that steer it |
| `client.Webhooks` | Endpoints, the event catalog, and the delivery log |
| `client.APIKeys` | Keys, scopes and usage analytics |
| `client.OAuthApps` | OAuth 2.1 application registration and the consent flow |
| `client.Outreach` | Organization-wide sending policy |
| `client.Deliverability` | Bounce and complaint ingestion from an upstream pipeline |
| `client.WarmupRouting` | Warmup partner-selection rules |
| `client.Tasks` | Send-task dead-letter queue |
| `client.AuditLogs` | The organization audit trail |
| `client.Folders` / `Tags` / `Categories` | The label sets that organize campaigns, mailboxes and contacts |
| `client.Meta` | Caller identity, plan catalog, timezones |
| `client.Auth` | Sign-in, sessions, profile, two-factor, passkeys, notifications |
| `client.Organization` | Workspace settings, members, roles, invitations, danger zone |
| `client.Billing` | Subscription, plan changes, AI credits, referrals |

`Auth`, `Organization` and `Billing` are session-only; see [Session tokens](#session-tokens).

### Reaching something new

The API moves faster than this SDK's release cadence. `client.Do` issues a request against any path, with the same authentication, retries and typed errors as a generated method:

```go
var out map[string]any
_, err := client.Do(ctx, http.MethodGet, "some/new/endpoint", nil, &out)
```

`warmbly.WithQueryParam` does the same for a filter that a typed parameter struct does not carry yet.

## Idempotency

Anything that sends mail or spends money accepts an `Idempotency-Key`. Retrying with the same key replays the original response instead of acting twice, which turns an ambiguous timeout into a safe retry:

```go
result, resp, err := client.Emails.Send(ctx, mailboxID, params,
    warmbly.WithIdempotencyKey("order-4171-welcome"))
if resp.IdempotentReplayed {
    // The original send already went out; this was a replay.
}
```

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

## Realtime gateway

The `gateway` subpackage holds a websocket open to a workspace and delivers typed events as they happen. Declare the **intents** you want, register handlers with `gateway.On`, and the client handles heartbeats, reconnection and replay of missed events.

```go
import "github.com/warmbly/warmbly-go/gateway"

g := gateway.New(apiKey, orgID,
    gateway.WithIntents(gateway.IntentCampaign, gateway.IntentEmail))

gateway.On(g, gateway.EventEmailOpened, func(ctx context.Context, e *gateway.EngagementEvent) {
    log.Printf("contact %s opened a message", e.ContactID)
})

if err := g.Open(ctx); err != nil {
    log.Fatal(err)
}
defer g.Close()
<-ctx.Done()
```

Every event carries a monotonic per-workspace sequence number. The client replays the gap after a reconnect, so a brief drop loses nothing; replay is at-least-once, so deduplicate on `Event.Seq` if your handler is not idempotent. A disconnect that outlasts the server's buffer surfaces as `EventResumeFailed`, your cue to resync from the REST API.

Intents only ever narrow the stream — a credential without unibox access receives no inbox events however it asks. The underlying transport is the dependency-free RFC 6455 implementation in `internal/wsconn`, so the gateway adds no third-party packages either.

## Webhooks

Verify every inbound delivery before trusting it. `client.Webhooks.ConstructEvent` checks the HMAC-SHA256 signature from the `X-Warmbly-Signature` header and rejects a stale one, which defeats replay:

```go
event, err := client.Webhooks.ConstructEvent(body, r.Header.Get(warmbly.WebhookSignatureHeader), endpointSecret)
switch {
case errors.Is(err, warmbly.ErrWebhookSignatureExpired):
    http.Error(w, "stale signature", http.StatusUnauthorized)
    return
case err != nil:
    http.Error(w, "invalid signature", http.StatusUnauthorized)
    return
}
```

A new endpoint receives nothing until it proves it owns its URL. Call `client.Webhooks.Verify`; Warmbly then sends a signed `webhook.test` delivery carrying a challenge token, which you echo back in the `X-Warmbly-Webhook-Challenge` response header. Take the token from the *verified payload*, not from the copy in the request header — that copy is attacker-controllable, the signed body is not. See [`examples/webhooks`](examples/webhooks) for the full handler.

Deliveries retry, so deduplicate on `event.ID`.

For lower-level use, `warmbly.VerifyWebhookSignature` and `warmbly.ConstructWebhookEvent` expose the same checks with an explicit tolerance.

## Examples

The [`examples/`](examples/) directory has a runnable program for each part of the SDK — API keys, both OAuth flows, campaigns, contacts, emails/warmup, templates, analytics, webhooks, the real-time gateway, and error handling. See [`examples/README.md`](examples/README.md) for the full index.

## Versioning

`warmbly-go` follows semantic versioning. While the module is pre-1.0, the public API may change between minor releases; review the [release notes](https://github.com/warmbly/warmbly-go/releases) before upgrading.

## Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) and our [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) before opening an issue or pull request.

## License

Released under the MIT License. See [LICENSE](LICENSE) for details.
