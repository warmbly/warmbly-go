// Package gateway is a client for the Warmbly realtime gateway: a persistent
// websocket that streams workspace events — campaign progress, engagement
// pulses, inbox arrivals, warmup health, automation runs — as they happen,
// instead of polling the REST API.
//
// # Connecting
//
// Authenticate with an API key holding the REALTIME_SUBSCRIBE scope, or with an
// OAuth access token whose grant includes the realtime scope. Both travel in
// the X-Warmbly-Token handshake header, never the URL: the gateway refuses a
// key or token in the query string with code 4003. A short-lived connection
// token minted for a browser session is also accepted, but it opens one socket
// only, so it cannot survive a reconnect. Register handlers first, then open
// the connection:
//
//	client := gateway.New(apiKey, orgID,
//		gateway.WithIntents(gateway.IntentCampaign, gateway.IntentEmail))
//
//	gateway.On(client, gateway.EventCampaignStarted, func(ctx context.Context, e *gateway.CampaignEvent) {
//		log.Printf("campaign %s started", e.CampaignID)
//	})
//
//	if err := client.Open(ctx); err != nil {
//		log.Fatal(err)
//	}
//	defer client.Close()
//
// [Client.Open] blocks until the workspace channel is joined, then returns.
// Reconnection, heartbeats and resumption run in the background from there.
//
// # Topics
//
// The workspace channel carries the org-scoped stream. Several families are
// only ever delivered on a member's personal channel — the task lifecycle,
// meetings, notifications and bulk-operation progress — so join it alongside:
//
//	client := gateway.New(apiKey, orgID,
//		gateway.WithTopics(gateway.UserTopic(userID)))
//
// [Event.Topic] tells a handler which channel an event arrived on. An extra
// topic that is refused raises [EventJoinFailed] and leaves the rest of the
// session running; a workspace channel that is refused ends it.
//
// # Delivery guarantees
//
// Every event carries a monotonic per-workspace sequence number. The client
// tracks the highest it has seen and replays the gap on reconnect, so a brief
// disconnect does not lose events.
//
// Replay is at-least-once: an event that arrives during the replay window can
// be delivered twice. Deduplicate on [Event.Seq] if your handler is not
// idempotent. When a disconnect outlasts the server's replay buffer the client
// receives [EventResumeFailed] and you should resync from the REST API.
//
// # Permissions
//
// The gateway enforces the same permissions as the REST API. A credential
// without unibox access receives no inbox events, whatever intents it declares;
// intents only ever narrow the stream further.
//
// A key or token reaches only the workspace it was created in, and there only
// the event families its read scopes cover, never more than its holder's own
// member permissions:
//
//	READ_EMAILS     mailbox, warmup and mailbox import events, the account topic
//	READ_CAMPAIGNS  campaign activity, task progress, send, open, click and reply pulses
//	READ_CONTACTS   contact, page hit, form and research events
//	READ_UNIBOX     inbox events and AI reply drafts
//	READ_ANALYTICS  placement tests and direct email events
//
// Team, billing and settings events never reach a key or token. A member
// restricted to selected resources, and any key acting for one, receives only
// events that name a mailbox or campaign they were granted; the rest are
// dropped, including events that name no resource. Authorization is re-checked
// about every 30 seconds, so a revoked grant stops delivery within that window
// rather than instantly.
//
// # Refusals
//
// A refused join or handshake surfaces as a [*JoinError] carrying the server's
// numeric code and reason. Branch on [JoinError.Code], and use
// [JoinError.Permanent] to tell a refusal that retrying cannot fix from one
// that it can. Joining another workspace's topic is refused as not_a_member.
package gateway
