# Changelog

All notable changes to this project are documented in this file. Entries are
grouped by release and version numbers use semantic versioning.

## [Unreleased]

Reconciled the SDK with the current v1 API. The previous release was written
against an earlier draft of the API and had drifted: several models, query
parameters and response envelopes no longer matched what the server sends, and
the webhook and gateway protocols were wrong outright. This release rebuilds the
SDK against the live surface.

Because there has been no public release, breaking changes are made directly
rather than deprecated. Expect to touch call sites.

### Added

- New services covering the rest of the customer-facing v1 surface:
  `client.Unibox`, `client.Advisor`, `client.CRM`, `client.Teams`,
  `client.Meetings`, `client.Integrations`, `client.Automations`,
  `client.LeadSync`, `client.Generation`, `client.Skills`, `client.Outreach`,
  `client.Deliverability`, `client.WarmupRouting`, `client.Tasks`,
  `client.AuditLogs`, `client.Folders` / `Tags` / `Categories`, `client.Meta`,
  `client.Auth` and `client.Billing`.
- Per-request options on every method: `warmbly.WithIdempotencyKey` for
  retry-safe sends, plus `WithRequestHeader` and `WithQueryParam`.
- `client.Do` as an escape hatch for endpoints this release does not model,
  with the same authentication, retries and typed errors as a generated method.
- Sign-in and session management through `client.Auth`, including two-factor,
  passkey listing, notification preferences and device tokens. These reach the
  routes an API key deliberately cannot.
- Multipart uploads: campaign attachments, contact import, avatars and OAuth
  application logos, via the new `warmbly.FileUpload`.
- Response metadata for `API-Version`, `Deprecation`, `Sunset`, `Warning` and
  `X-Idempotent-Replayed`.
- Pointer helpers (`warmbly.String`, `Int`, `Bool`, `Time`, …) for filling the
  optional fields of parameter structs inline.
- Many endpoints on existing services: mailbox bulk tagging, domain
  authentication checks and address verification; campaign advanced settings,
  A/B variants, attachments, preflight and sender pools; contact import and
  export, lookup, custom fields and AI research; template scoring, rendering,
  duplication and reordering; deliverability, comparison and per-mailbox
  analytics; API-key usage analytics and request logs; the webhook event
  catalog, delivery log, redelivery and throttle drops.

### Changed

- **Webhook signatures.** The header is `X-Warmbly-Signature`, not
  `X-Webhook-Signature`, and its value is `t=<unix>,v1=<hex>` over
  `"<timestamp>." + body` rather than a bare `sha256=` digest. The old scheme
  never validated a real delivery. `ConstructEvent` now also rejects a stale
  signature, which defeats replay; `ComputeWebhookSignature` takes the signing
  timestamp. `WebhookEvent.Event` is now `EventType`.
- **Gateway protocol.** The realtime gateway speaks Phoenix Channels over
  `/socket/websocket`, not the opcode protocol the previous release
  implemented. `gateway.New` now takes an organization id, intents are event
  family strings rather than a bitmask, and resumption is by sequence number.
  Nothing in the previous gateway package could connect.
- **API-key and OAuth-application scopes** are a `uint64` bitmask built from the
  new `warmbly.Perm*` constants, not a string slice. `APIKey` gained
  `Can`/`CanAny`, and `APIKeyService.Permissions` returns the server's catalog
  and presets.
- **List filters.** Mailbox and campaign search use `q` (`Query`), not
  `search`; the mailbox `provider`/`status` filters and the campaign `status`
  filter never existed and are gone. Campaign lists filter by `Folder`.
- **Contact search** carries pagination in the query string and filters in the
  body, and returns a `ContactPage` with the facet counts the API sends on the
  first page. `Contacts.Get` returns the hydrated `ContactDetail`.
- **Response shapes** corrected throughout, most consequentially: `SendResult`
  is `{task_id, scheduled_at, send_mode}`; `WarmupBanStatus` reports
  `Blocked`/`CanAppeal`/`HealthState`; `Webhooks.List` returns endpoints plus
  the event catalog and is not paginated; `Templates.List` is not paginated;
  `Campaigns.Senders` returns `[]CampaignSender`, not `[]string`.
- **Analytics** takes explicit periods and date ranges rather than an
  `AnalyticsRange`, and its models match the server's actual payloads.
- `Organization.Current` returns `OrganizationWithLimits`; members, roles and
  invitations are modelled properly, and permissions are a bitmask with
  `Member.Can`.
- `Campaigns.CreateStep` takes no body — it appends a blank step, which
  `UpdateStep` then fills in.

### Removed

- `Webhooks.Get`, which has no route on the server.
- `gateway.Opcode`, `gateway.Intent` as a bitmask, and the `IntentsAll` /
  `IntentsDefault` presets.
- `AnalyticsRange`, `StepParams`, `CampaignTestEmailParams` and the other
  parameter types replaced by ones matching the API.

### Notes

Every route on the customer-facing v1 surface is now reachable through a typed
method. Deliberately out of scope: the platform admin API (`/admin`), the
internal worker API, the signed task-queue endpoints, and `POST /v1/mcp` — the
last is an MCP transport rather than a REST resource, and is meant to be driven
by an MCP client.

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
