package gateway

// Intent is a bitmask that subscribes a session to a category of events. Declare
// the intents you need in [WithIntents]; the server only dispatches events that
// fall under a requested intent. Subscribing narrowly reduces traffic and avoids
// receiving high-volume streams you do not need.
type Intent uint64

const (
	// IntentCampaigns subscribes to campaign lifecycle events
	// (started, paused, completed).
	IntentCampaigns Intent = 1 << iota
	// IntentEmailEngagement subscribes to recipient engagement events
	// (sent, opened, clicked, replied, bounced). This can be high volume.
	IntentEmailEngagement
	// IntentWarmup subscribes to mailbox warmup events
	// (health changes, blocks, spam placement).
	IntentWarmup
	// IntentContacts subscribes to contact change events.
	IntentContacts
	// IntentInbox subscribes to inbox events (incoming replies and messages).
	// This can be very high volume.
	IntentInbox
	// IntentCRM subscribes to CRM events (deals, tasks, pipelines).
	IntentCRM
	// IntentMeetings subscribes to meeting events (booked, rescheduled,
	// canceled).
	IntentMeetings
	// IntentDeliverability subscribes to deliverability events
	// (bounces and complaints reported by providers).
	IntentDeliverability
)

const (
	// IntentsNone subscribes to no event categories.
	IntentsNone Intent = 0

	// IntentsAll subscribes to every event category, including the high-volume
	// engagement and inbox streams.
	IntentsAll = IntentCampaigns | IntentEmailEngagement | IntentWarmup |
		IntentContacts | IntentInbox | IntentCRM | IntentMeetings |
		IntentDeliverability

	// IntentsDefault is a sensible default: everything except the high-volume
	// inbox stream.
	IntentsDefault = IntentCampaigns | IntentEmailEngagement | IntentWarmup |
		IntentContacts | IntentCRM | IntentMeetings | IntentDeliverability
)

// Has reports whether i includes all the bits of other.
func (i Intent) Has(other Intent) bool {
	return i&other == other
}
