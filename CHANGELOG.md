# Changelog

All notable changes to this project are documented in this file. Entries are
grouped by release and version numbers use semantic versioning.

## [Unreleased]

## [0.1.0] - 2026-06-15

Initial release of the official Go SDK for Warmbly.

### Added

- Typed REST client with services hanging off the client for all API surfaces:
  API keys (`client.APIKeys`), OAuth applications (`client.OAuthApps`), email
  accounts and mailbox warmup (`client.Emails`), campaigns and steps
  (`client.Campaigns`), contacts (`client.Contacts`), webhooks
  (`client.Webhooks`), templates (`client.Templates`), analytics
  (`client.Analytics`), and organization (`client.Organization`).
- API-key authentication via `warmbly.WithAPIKey` and OAuth 2.1 authentication
  via `warmbly.WithAccessToken` / `warmbly.WithTokenSource`.
- Full OAuth 2.1 client flows: authorization-code with PKCE
  (`warmbly.OAuth2Config`) and client-credentials
  (`warmbly.ClientCredentialsConfig`).
- Automatic retries with jittered exponential backoff that honours the
  `Retry-After` header, plus rate-limit header parsing exposed on the client.
- Cursor pagination with a Go 1.23 auto-paging iterator via `page.All(ctx)`.
- Typed errors decoded from the API as `*warmbly.Error`, matchable with
  `errors.Is` against sentinels such as `warmbly.ErrNotFound`,
  `warmbly.ErrUnauthorized`, and `warmbly.ErrRateLimited`.
- Webhook HMAC-SHA256 signature verification via
  `warmbly.VerifyWebhookSignature` and `client.Webhooks.ConstructEvent`.
- Real-time gateway client (subpackage `gateway`) with a persistent
  connection, intent-based subscriptions, typed event handlers, automatic
  heartbeat, session resume, and automatic reconnect. Session termination is
  observable via `Done()` and `Err()`.
- Zero external dependencies: the entire module uses only the Go standard
  library, with the RFC 6455 WebSocket protocol implemented in an internal
  package.

[Unreleased]: https://github.com/warmbly/warmbly-go/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/warmbly/warmbly-go/releases/tag/v0.1.0
