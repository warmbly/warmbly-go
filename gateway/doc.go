// Package gateway is a real-time event client for the Warmbly platform.
//
// It maintains a persistent WebSocket connection that streams events — opened
// and clicked emails, replies, campaign lifecycle changes, warmup health
// updates and more — as they happen, so you can react without polling the REST
// API.
//
// # Protocol
//
// Every message is an envelope tagged with an [Opcode]. After connecting, the
// server sends [OpHello] with a heartbeat interval; the client then sends
// [OpIdentify] (with its token and an [Intent] bitmask) to start a session, or
// [OpResume] to replay missed events on an existing one. The server then streams
// [OpDispatch] frames, each carrying an event name and a sequence number. The
// client heartbeats on the interval and the server acknowledges with
// [OpHeartbeatACK]; a missed acknowledgement is treated as a dead connection and
// triggers an automatic reconnect.
//
// All of this is handled for you: connection, authentication, heartbeating,
// sequence tracking, session resumption and reconnection with backoff.
//
// # Usage
//
//	g := gateway.New(apiKey, gateway.WithIntents(gateway.IntentCampaigns|gateway.IntentEmailEngagement))
//
//	gateway.On(g, gateway.EventEmailOpened, func(ctx context.Context, e *gateway.EngagementEvent) {
//		log.Printf("contact %s opened a message", e.ContactID)
//	})
//	gateway.On(g, gateway.EventCampaignCompleted, func(ctx context.Context, e *gateway.CampaignEvent) {
//		log.Printf("campaign %s finished", e.CampaignID)
//	})
//
//	if err := g.Open(ctx); err != nil {
//		log.Fatal(err)
//	}
//	defer g.Close()
//	<-ctx.Done()
//
// # Intents
//
// Declare only the [Intent] categories you need. The server dispatches events
// solely for the requested intents, which keeps high-volume streams (such as
// inbox messages) off the wire unless you ask for them.
//
// This package depends only on the standard library; the WebSocket protocol is
// implemented internally.
package gateway
