# Examples

Runnable programs demonstrating `warmbly-go`. Each reads its credentials from
the environment, so set `WARMBLY_API_KEY` (and, for the user OAuth flow,
`WARMBLY_CLIENT_ID` / `WARMBLY_CLIENT_SECRET`) and then run:

```
go run ./examples/<name>
```

| Example | What it shows |
| --- | --- |
| [`apikeys`](apikeys) | Create, list, and inspect API keys |
| [`oauth`](oauth) | OAuth 2.1 authorization-code flow with PKCE (browser login) |
| [`oauthapps`](oauthapps) | Register/manage OAuth apps + client-credentials grant |
| [`organization`](organization) | Read/rename the org, list members, invite/re-role/remove |
| [`campaigns`](campaigns) | Create a campaign, add sequence steps, test, start, list |
| [`contacts`](contacts) | Import contacts, search with pagination, bulk update |
| [`emails`](emails) | List mailboxes, drive warmup, check ban status, send |
| [`templates`](templates) | Create, update, list, and delete message templates |
| [`analytics`](analytics) | Read dashboard and warmup analytics over a date range |
| [`webhooks`](webhooks) | Register an endpoint and verify inbound deliveries |
| [`gateway`](gateway) | Stream real-time events with typed handlers |
| [`errors`](errors) | Error handling, retry configuration, rate-limit state |
