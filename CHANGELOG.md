# Changelog

All notable changes to this project are documented in this file. Entries are
grouped by release and version numbers use semantic versioning.

## [0.3.1] - 2026-09-07

Picks up the API changes that landed while 0.3.0 was being prepared. Additive,
so nothing needs touching at a call site.

### Added

- `CampaignStatusChange.WaitingForLeads`, which `POST /campaigns/:id/start` now
  returns. It is true when the start found nothing left to send, so a caller
  learns the campaign is parked and waiting without a second fetch.
- `ErrCodeNoLeads` and `ErrCodeNoRemainingLeads`, the two refusals a start can
  answer with.

### Changed

- Starting a campaign whose every lead has finished no longer re-completes it
  with a 400. The start turns `Continuous` on, leaves the campaign active and
  idle, and reports `WaitingForLeads`. `CampaignService.Start` documented that
  older behaviour, which is now wrong.
- A form linked to a campaign, or an automation that enrolls into one, turns
  `Continuous` on, as linking a segment already did.

## [0.3.0] - 2026-09-07

Reconciled the SDK with the current v1 API, covering the 484 server commits
since the 0.2.0 sync. Whole resources landed on the server in that window and
several existing shapes moved underneath the SDK. This release also fixes a
handful of methods that could never have worked.

Breaking changes are made directly rather than deprecated, as there is still no
public release. Expect to touch call sites.

### Added

- New services for resources the API grew: `client.Segments` (saved contact
  audiences, evaluated live, with per-contact overrides), `client.Suppressions`
  (the workspace do-not-contact list), `client.Forms` (hosted lead-capture
  forms, their submissions, assets and custom domain), `client.AgentTools` (the
  AI tool registry over plain HTTP, for function-calling agents that do not
  speak MCP), `client.WebsiteTracking`, and `client.PoolLink` /
  `client.CloudLink` for the self-hosted warmup pool link.
- Mailboxes: `Allowance` reports how many mailboxes a workspace may hold and
  why; `Hold` and `Release` take a mailbox in and out of campaign rotation;
  `SyncStatus` exposes the backfill and fair-use throttle state; `Behavior`,
  `UpdateBehavior` and `BehaviorPlan` drive humanlike sending; `RefreshAuthCheck`
  records a domain verdict rather than only reading one; `GetTrackingDomain` and
  `VerifyTrackingDomain` complete the custom tracking domain flow; bulk
  SMTP/IMAP connect and the two reconnect routes.
- Campaigns: `Estimate` projects recipients, capacity and finish date before a
  launch; `Duplicate`; `Forms`; `ListSegments` and `SetSegments`;
  `StartWithOptions` carries the acknowledgement that clears a list-risk
  refusal.
- Contacts: address verification (`VerificationOverview`, `RequestVerification`),
  `CampaignStates`, `Segments`, and a cursor-paginated `ListTimeline`.
- Sign-in: the browser single-sign-on start and exchange, the first-run claim,
  deployment capabilities via `AuthConfig`, instance version reporting, and the
  full CLI device-code flow with a `WaitForCLIAuth` helper.
- `APIKeys.RevokeSelf`, so a credential can always end itself.
- Workspace archives: export and import, with the download streaming to an
  `io.Writer`, plus the workspace risk posture.
- `Error.HasCode` and the `ErrCode*` constants, so a caller can branch on the
  specific refusal rather than on a status several refusals share.
- Gateway: typed `JoinError` with the server's code and reason slug, a
  `Permanent` method separating retryable refusals from final ones, automatic
  retry of a rate-limited join on the same socket, topic builders and
  `WithTopics`, a heartbeat watchdog, and the `CAMPAIGN_IDLE`,
  `ACCOUNT_SYNC_STATE`, `PAGE_HIT` and `FORM_SUBMISSION_CREATED` events.
- Webhooks: the `form.submitted` event, the richer dedicated `contact.created`
  payload, the `webhook.test` challenge payload, and `WebhookEvent.Into`.

### Fixed

- **`gateway.IntentAI` was `"AI"`**, which is a substring of `EMAIL` and
  `CAMPAIGN`. Intent matching is a substring test, so anyone filtering for AI
  events received nearly the entire stream. It is now `"AI_"`.
- **Gateway join-refusal codes were mapped wrongly.** 4003 and 4005 both
  resolved to `ErrForbidden`, and 4001, which the server never sends, was
  mapped at all. Corrected against the gateway's own `error_code/1`.
- **A client could get permanently stuck after a failed resume.** The
  `resume_failed` path left the sequence at the evicted position, so every
  later resume failed too. It now advances to the server's current sequence.
- **`Meetings.Create` decoded nothing.** The endpoint answers a wrapped
  envelope, so every field of the returned meeting was silently zero.
- **`Billing.Cancel` was unusable.** The endpoint binds a JSON body and the SDK
  sent none, so every call was refused with a 400.
- **Avatar uploads were wrong twice over.** Both endpoints read a part named
  `file`, not `avatar`, and answer a bare URL rather than the object.
- `EnsureReferralCode`, `PreviewInvitation` and `RegisterDeviceToken` each
  decoded a type the endpoint does not return.
- `ReferralEarning` carried invented fields; it now matches the wire.
- `Campaigns.UpdateAdvancedSettings` sent the wrong body key, so it silently
  changed nothing.
- The audit catalogue was badly stale: 5 actions and 8 entity types against the
  server's 30 and 49, with `AuditEntitySequence` carrying the wrong value.

### Changed

- **`Campaigns.Start` and `Campaigns.Stop`** return a `*CampaignStatusChange`.
  The endpoints answer a status envelope, never a campaign, so the previous
  `*Campaign` was always zero.
- **`Campaigns.UpdateAdvancedSettings` and `Outreach.Update`** return only a
  `*Response`; both endpoints answer 204 with no body.
- **`Email.LastSyncedAt` is now `*time.Time`.** It is null until the first sync,
  which previously failed to decode.
- **`Auth.Login` and `Auth.Register` return an `*AuthStep`.** Whether an emailed
  code step follows is the deployment's choice and is skipped on a known device,
  so the flow can finish in one call. Branch on `CodeRequired`.
- **`CRM.ListDeals` and `CRM.ListTasks`** take their own parameter types,
  exposing the pipeline, stage, status, contact, deal and assignee filters the
  server always accepted and the SDK never sent.
- `Billing.ValidateDiscount`, `PreviewPlanChange` and `AppliedDiscounts` return
  typed results instead of `map[string]any`.
- `Auth.UploadAvatar` and `Organization.UploadAvatar` return the avatar URL.
- Campaigns carry continuous sending (`Continuous`, `IdleSince`), guardrails,
  UTM tagging, unsubscribe mode and a `kind`; a campaign that runs out of leads
  now stays active and idle rather than finishing.
- Contact search and export filter by segment, engagement and verification
  status; the timeline carries page hits, link clicks with UTM parameters, and
  the client, device and location an engagement came from.
- Mailboxes carry `SaveToSent`, and the sending-domain authentication gate is
  modelled with `AuthFailingSince` and the `AuthState*` constants.

### Removed

- `DeviceToken`, which no route ever returned.

## [0.2.0] - 2026-08-05

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

[Unreleased]: https://github.com/warmbly/warmbly-go/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/warmbly/warmbly-go/releases/tag/v0.2.0
[0.1.0]: https://github.com/warmbly/warmbly-go/releases/tag/v0.1.0
