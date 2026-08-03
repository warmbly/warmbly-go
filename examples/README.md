# Examples

Runnable programs demonstrating `warmbly-go`. Each reads its credentials from
the environment, so set `WARMBLY_API_KEY` (and, for the user OAuth flow,
`WARMBLY_CLIENT_ID` / `WARMBLY_CLIENT_SECRET`) and then run:

```
go run ./examples/<name>
```

| Example | What it shows |
| --- | --- |
| [`apikeys`](apikeys) | Mint keys with scope bitmasks, list them, read usage |
| [`oauth`](oauth) | OAuth 2.1 authorization-code flow with PKCE (browser login) |
| [`oauthapps`](oauthapps) | Register and manage OAuth apps, client-credentials grant |
| [`organization`](organization) | Sign in for a session token, then manage the workspace and its people |
| [`campaigns`](campaigns) | Create a campaign with its sequence, preflight it, start it |
| [`contacts`](contacts) | Add contacts, faceted search, the 360 view, bulk edits |
| [`emails`](emails) | List mailboxes, drive warmup, check domain auth, send |
| [`templates`](templates) | Create, score, render, reorder and delete templates |
| [`analytics`](analytics) | Engagement, deliverability health and per-mailbox status |
| [`webhooks`](webhooks) | Register an endpoint, answer its challenge, verify deliveries |
| [`gateway`](gateway) | Stream realtime events with typed handlers and resume |
| [`errors`](errors) | Error handling, retry configuration, rate-limit state |

The `organization` example signs in with `WARMBLY_EMAIL` / `WARMBLY_PASSWORD`
rather than an API key, because workspace governance is session-only. The
`gateway` example also needs `WARMBLY_ORG_ID`.
