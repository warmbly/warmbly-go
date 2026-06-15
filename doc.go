// Package warmbly is the official Go SDK for the Warmbly API.
//
// Warmbly is a cold-outreach and mailbox-warmup platform. This SDK provides a
// typed, idiomatic client for the REST API together with a real-time gateway
// client for streaming events as they happen.
//
// # Installation
//
//	go get github.com/warmbly/warmbly-go@latest
//
// # Authentication
//
// The SDK supports the two programmatic authentication schemes exposed by the
// Warmbly API:
//
//   - API keys (prefixed "wmbly_"), for server-to-server access scoped to a
//     single organization. Create a client with [WithAPIKey].
//   - OAuth 2.1 access tokens (prefixed "wmblyo_"), for applications acting on
//     behalf of a user. Use the helpers in oauth_flow.go to run the
//     authorization-code (with PKCE) or client-credentials flow, then pass the
//     resulting token with [WithAccessToken] or [WithTokenSource].
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
//	for _, c := range page.Data {
//		fmt.Println(c.Name)
//	}
//
// # Real-time gateway
//
// The gateway subpackage (github.com/warmbly/warmbly-go/gateway) maintains a
// persistent connection that streams events such as opened emails, replies and
// warmup health changes. See that package for details.
//
// # Design
//
// The root package depends only on the Go standard library. Errors returned by
// the API are decoded into a typed [*Error] that can be matched with errors.Is
// against the package sentinels (for example [ErrNotFound]). List endpoints
// return a generic [Page] that transparently auto-paginates.
package warmbly
