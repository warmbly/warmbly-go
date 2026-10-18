// Package gateway is a client for the Warmbly realtime gateway: a persistent
// websocket that streams workspace events — campaign progress, engagement
// pulses, inbox arrivals, warmup health, automation runs — as they happen,
// instead of polling the REST API.
//
// # Connecting
//
// Authenticate with an API key holding the REALTIME_SUBSCRIBE permission, an
// OAuth access token with the realtime_subscribe scope, or a connection ticket
// minted for a signed-in session. Keys and OAuth tokens are sent in the
// X-Warmbly-Token handshake header ([CredentialHeader]), never in the URL.
// Register handlers first, then open the connection:
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
// A member restricted to selected campaigns, folders and mailboxes, and any
// key or OAuth app acting for one, is filtered further by those grants on the
// workspace channel: inbox events need a granted mailbox, other events a
// granted campaign or mailbox, and AUDIT_CREATED a granted campaign, folder or
// mailbox or a change to the member themselves. Events that name no resource
// are dropped, and such a connection receives no presence. Access is re-read
// at least every 30 seconds, so a revoked grant stops delivery within that
// window rather than instantly.
package gateway
