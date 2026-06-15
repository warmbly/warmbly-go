package gateway

import "context"

// HandlerFunc handles a dispatched [Event]. Handlers for a session run on a
// single dispatch goroutine in the order events arrive, so a slow handler
// delays subsequent events; offload long work to your own goroutine if needed.
// The context is cancelled when the session closes.
type HandlerFunc func(ctx context.Context, e *Event)

// Handle registers fn to be called for every event of the given name. Multiple
// handlers may be registered for the same event; they run in registration
// order. Register handlers before calling [Client.Open].
func (c *Client) Handle(name EventName, fn HandlerFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.handlers == nil {
		c.handlers = make(map[EventName][]HandlerFunc)
	}
	c.handlers[name] = append(c.handlers[name], fn)
}

// HandleAny registers fn to be called for every dispatched event, regardless of
// name. These handlers run before the name-specific handlers for each event.
func (c *Client) HandleAny(fn HandlerFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.anyHandlers = append(c.anyHandlers, fn)
}

// On registers a typed handler for an event whose payload decodes into T. It is
// a thin wrapper over [Client.Handle] that unmarshals [Event.Raw] into a *T
// before invoking fn:
//
//	gateway.On(client, gateway.EventEmailOpened, func(ctx context.Context, e *gateway.EngagementEvent) {
//		log.Printf("opened by %s", e.ContactID)
//	})
//
// Events whose payload fails to decode are dropped (and logged via the client's
// logger).
func On[T any](c *Client, name EventName, fn func(ctx context.Context, payload *T)) {
	c.Handle(name, func(ctx context.Context, e *Event) {
		v := new(T)
		if err := e.Into(v); err != nil {
			c.logf("gateway: decode %s payload: %v", name, err)
			return
		}
		fn(ctx, v)
	})
}
