// Package warmbly is the official Go SDK for the Warmbly API.
//
// Warmbly is a cold-outreach and mailbox-warmup platform. This package is a
// typed client for the whole customer-facing v1 surface; the gateway
// subpackage streams the same workspace's events over a websocket.
//
// # Installation
//
//	go get github.com/warmbly/warmbly-go@latest
//
// # Authentication
//
// Three credentials reach three different slices of the API:
//
//   - API keys (prefixed "wmbly_") for server-to-server access scoped to one
//     workspace. Create a client with [WithAPIKey].
//   - OAuth 2.1 access tokens (prefixed "wmblyo_") for an application acting
//     for a user. Run the authorization-code or client-credentials flow with
//     [OAuth2Config] or [ClientCredentialsConfig], then pass the token with
//     [WithAccessToken] or [WithTokenSource].
//   - Session tokens from [AuthService.Login] for the routes a long-lived key
//     deliberately cannot reach: workspace governance, billing and the AI
//     assistant. An API key on one of those gets a clean [ErrUnauthorized].
//
// # Quick start
//
//	client, err := warmbly.New(warmbly.WithAPIKey("wmbly_..."))
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	page, err := client.Campaigns.List(ctx, nil)
//	if err != nil {
//		log.Fatal(err)
//	}
//	for campaign, err := range page.All(ctx) {
//		if err != nil {
//			log.Fatal(err)
//		}
//		fmt.Println(campaign.Name)
//	}
//
// # Shape of the API
//
// Every resource group is a service on the [Client]: [Client.Emails],
// [Client.Campaigns], [Client.Contacts], [Client.Unibox], [Client.CRM] and the
// rest. Each method takes a context, then its parameters, then a variadic list
// of [RequestOption].
//
// Methods that return one record return it alongside the [Response], so
// rate-limit state and the request id stay reachable. List methods return a
// [Page] that auto-pages through [Page.All].
//
// # Retrying safely
//
// The client retries 429 and 5xx responses with jittered exponential backoff,
// honoring Retry-After. That is safe for reads, but a retried send could go
// out twice — so anything that sends mail or spends money accepts an
// idempotency key, and repeating it replays the original response:
//
//	result, resp, err := client.Emails.Send(ctx, id, params,
//		warmbly.WithIdempotencyKey("order-4171-welcome"))
//	if resp.IdempotentReplayed {
//		// The original send already went out.
//	}
//
// # Errors
//
// Every non-2xx response decodes into an [Error] carrying the message, code and
// request id. Match it with errors.Is against the package sentinels
// ([ErrNotFound], [ErrRateLimited] and the rest) rather than comparing status
// codes by hand.
//
// # Reaching something new
//
// The API ships faster than this SDK. [Client.Do] issues a request against any
// path with the same authentication, retries and typed errors, and
// [WithQueryParam] adds a filter no typed parameter carries yet.
//
// # Design
//
// The module depends only on the standard library, including its own RFC 6455
// websocket implementation, so it adds no transitive supply chain.
package warmbly
