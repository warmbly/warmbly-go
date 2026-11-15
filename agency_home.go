package warmbly

import (
	"context"
	"net/url"
	"strings"
	"time"
)

// AgencyClientAccess is how much of a client workspace the caller reaches
// through their own membership in it.
type AgencyClientAccess string

const (
	// AgencyClientAccessFull is a member across the whole workspace.
	AgencyClientAccessFull AgencyClientAccess = "full"
	// AgencyClientAccessRestricted is a member limited to granted campaigns
	// and mailboxes; the overview leaves its numbers out.
	AgencyClientAccessRestricted AgencyClientAccess = "restricted"
	// AgencyClientAccessNone is not a member of the workspace.
	AgencyClientAccessNone AgencyClientAccess = "none"
)

// AgencyAttentionKind names a problem on the agency overview's attention
// list. Unknown kinds may appear as the server adds them.
type AgencyAttentionKind string

const (
	// AgencyAttentionMailboxDisconnected counts disconnected mailboxes.
	AgencyAttentionMailboxDisconnected AgencyAttentionKind = "mailbox_disconnected"
	// AgencyAttentionMailboxHeld counts mailboxes quarantined or blocked from
	// warmup.
	AgencyAttentionMailboxHeld AgencyAttentionKind = "mailbox_held"
	// AgencyAttentionBounceSpike is a client bouncing too much; Rate is the
	// bounce rate.
	AgencyAttentionBounceSpike AgencyAttentionKind = "bounce_spike"
	// AgencyAttentionCampaignsPausedLimits counts campaigns paused by limits.
	AgencyAttentionCampaignsPausedLimits AgencyAttentionKind = "campaigns_paused_limits"
	// AgencyAttentionCampaignsPausedBilling counts campaigns paused by
	// billing.
	AgencyAttentionCampaignsPausedBilling AgencyAttentionKind = "campaigns_paused_billing"
	// AgencyAttentionClientCapNear is a client close to its monthly send cap;
	// Rate is the share of the cap used.
	AgencyAttentionClientCapNear AgencyAttentionKind = "client_cap_near"
	// AgencyAttentionPoolLow is the agency's send pool running low.
	AgencyAttentionPoolLow AgencyAttentionKind = "pool_low"
)

// AgencyOverview is the agency home: the pool, totals, problems first, then
// every client. Numbers only cover what the caller can see in each client.
type AgencyOverview struct {
	GeneratedAt time.Time `json:"generated_at"`
	// Usage is the send pool; nil when it could not be read.
	Usage     *AgencyUsage           `json:"usage"`
	Totals    AgencyOverviewTotals   `json:"totals"`
	Attention []AgencyAttention      `json:"attention"`
	Clients   []AgencyOverviewClient `json:"clients"`
}

// AgencyOverviewTotals adds up what the caller can see across clients.
type AgencyOverviewTotals struct {
	Clients           int   `json:"clients"`
	SentToday         int64 `json:"sent_today"`
	RepliesWaiting    int64 `json:"replies_waiting"`
	PositiveReplies7d int64 `json:"positive_replies_7d"`
	ActiveCampaigns   int   `json:"active_campaigns"`
	Mailboxes         int   `json:"mailboxes"`
	MailboxProblems   int   `json:"mailbox_problems"`
}

// AgencyAttention is one item on the attention list.
type AgencyAttention struct {
	Kind AgencyAttentionKind `json:"kind"`
	// Severity is "critical" or "warning".
	Severity string `json:"severity"`
	// OrganizationID and ClientName name the client; nil for the agency
	// itself.
	OrganizationID *string `json:"organization_id,omitempty"`
	ClientName     string  `json:"client_name,omitempty"`
	Count          int64   `json:"count"`
	// Rate is the bounce rate for a bounce spike and the share of the cap
	// used for a client near its cap.
	Rate float64 `json:"rate,omitempty"`
}

// AgencyOverviewClient is one client row of the agency home. A section is nil
// when the caller lacks the workspace permission behind it.
type AgencyOverviewClient struct {
	OrganizationID string             `json:"organization_id"`
	Name           string             `json:"name"`
	AvatarURL      *string            `json:"avatar_url,omitempty"`
	Access         AgencyClientAccess `json:"access"`
	// ManageMailboxes reports whether the caller can connect and change
	// mailboxes in the workspace.
	ManageMailboxes  bool  `json:"manage_mailboxes"`
	PoolUsed         int64 `json:"pool_used"`
	MonthlySendLimit *int  `json:"monthly_send_limit,omitempty"`
	// Campaigns needs view_campaigns in the workspace.
	Campaigns *AgencyClientCampaigns `json:"campaigns,omitempty"`
	// Inbox needs access_unibox.
	Inbox *AgencyClientInbox `json:"inbox,omitempty"`
	// Mailboxes needs view_campaigns, like the mailbox list.
	Mailboxes *AgencyClientMailboxes `json:"mailboxes,omitempty"`
	// Advisor needs view_analytics.
	Advisor *AgencyClientAdvisor `json:"advisor,omitempty"`
}

// AgencyClientCampaigns is a client's campaign activity.
type AgencyClientCampaigns struct {
	Active            int     `json:"active"`
	PausedByLimits    int     `json:"paused_by_limits"`
	PausedByBilling   int     `json:"paused_by_billing"`
	SentToday         int64   `json:"sent_today"`
	Sent7d            int64   `json:"sent_7d"`
	Bounced7d         int64   `json:"bounced_7d"`
	BounceRate7d      float64 `json:"bounce_rate_7d"`
	PositiveReplies7d int64   `json:"positive_replies_7d"`
}

// AgencyClientInbox is a client's unanswered human mail.
type AgencyClientInbox struct {
	RepliesWaiting int64 `json:"replies_waiting"`
}

// AgencyClientMailboxes is a client's mailbox health.
type AgencyClientMailboxes struct {
	Total        int `json:"total"`
	Disconnected int `json:"disconnected"`
	Erroring     int `json:"erroring"`
	Watch        int `json:"watch"`
	Throttled    int `json:"throttled"`
	Quarantined  int `json:"quarantined"`
	Blocked      int `json:"blocked"`
}

// AgencyClientAdvisor counts a client's open Advisor findings by severity.
type AgencyClientAdvisor struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
}

// AgencyMailbox is one client mailbox in the agency's mailbox list.
type AgencyMailbox struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	// Client is the client workspace's name.
	Client   string `json:"client"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Status   string `json:"status"`
	// Erroring is a connected mailbox with an unresolved critical error.
	Erroring bool `json:"erroring"`
	Warmup   bool `json:"warmup"`
	// WarmupState is "watch", "throttled", "quarantined" or "blocked";
	// empty in good standing.
	WarmupState string `json:"warmup_state,omitempty"`
	// Vendor is the inbox vendor the mailbox was imported from, if any.
	Vendor        string    `json:"vendor,omitempty"`
	CampaignLimit int       `json:"campaign_limit"`
	CreatedAt     time.Time `json:"created_at"`
}

// AgencyMailboxListParams filters the agency's mailbox list. Limit is 1 to
// 200 (default 50).
type AgencyMailboxListParams struct {
	ListOptions
	// Clients keeps these client workspace IDs; empty keeps every client
	// whose mailbox list the caller reads.
	Clients []string
	// Query searches mailbox addresses and names.
	Query string
	// Attention keeps only disconnected or erroring mailboxes and those
	// quarantined or blocked from warmup.
	Attention bool
}

// AgencyInboxThread is one conversation in the cross-client inbox: its
// newest message the caller can see.
type AgencyInboxThread struct {
	OrganizationID string `json:"organization_id"`
	ID             string `json:"id"`
	// EmailID is the client mailbox the conversation is in.
	EmailID      string    `json:"email_id"`
	ThreadID     string    `json:"thread_id"`
	FromAddr     []string  `json:"from_addr"`
	ToAddr       []string  `json:"to_addr"`
	Subject      string    `json:"subject"`
	Snippet      string    `json:"snippet"`
	InternalDate time.Time `json:"internal_date"`
	Seen         bool      `json:"seen"`
	MessageCount int64     `json:"message_count"`
	HasUnread    bool      `json:"has_unread"`
}

// AgencyInboxListParams filters the cross-client inbox. Limit is 1 to 100
// (default 30).
type AgencyInboxListParams struct {
	ListOptions
	// Clients keeps these client workspace IDs; empty keeps every client
	// whose inbox the caller reads.
	Clients []string
	// Unread keeps conversations with unread mail.
	Unread bool
	// NeedsReply keeps conversations waiting on a reply.
	NeedsReply bool
	// Positive keeps conversations with a positive reply.
	Positive bool
	// Query searches the conversations.
	Query string
}

func setClients(q url.Values, clients []string) {
	ids := make([]string, 0, len(clients))
	for _, c := range clients {
		if c != "" {
			ids = append(ids, c)
		}
	}
	// The server reads one comma-separated value, not a repeated parameter.
	if len(ids) > 0 {
		q.Set("client", strings.Join(ids, ","))
	}
}

// Overview returns the agency home: the send pool, totals, an attention list
// with problems first, then every client. Each client's numbers are read
// through the caller's own membership in it.
//
// Scope: [PermAgencyRead].
func (s *AgencyService) Overview(ctx context.Context, agencyID string, opts ...RequestOption) (*AgencyOverview, *Response, error) {
	return fetch[AgencyOverview](ctx, s.client, agencyPath(agencyID, "overview"), opts)
}

// Mailboxes returns a page of mailboxes across the clients whose mailbox
// list the caller reads, newest first. A client where the caller is limited
// to selected mailboxes is left out.
//
// Scope: [PermAgencyRead].
func (s *AgencyService) Mailboxes(ctx context.Context, agencyID string, params *AgencyMailboxListParams, opts ...RequestOption) (*Page[AgencyMailbox], error) {
	q := make(url.Values)
	if params != nil {
		params.apply(q)
		setClients(q, params.Clients)
		setNonEmpty(q, "q", params.Query)
		if params.Attention {
			q.Set("attention", "true")
		}
	}
	return listJSON[AgencyMailbox](ctx, s.client, agencyPath(agencyID, "mailboxes"), q, opts...)
}

// Inbox returns a page of conversations across the clients whose inbox the
// caller reads, newest activity first.
//
// Scope: [PermAgencyRead] and [PermReadUnibox].
func (s *AgencyService) Inbox(ctx context.Context, agencyID string, params *AgencyInboxListParams, opts ...RequestOption) (*Page[AgencyInboxThread], error) {
	q := make(url.Values)
	if params != nil {
		params.apply(q)
		setClients(q, params.Clients)
		if params.Unread {
			q.Set("unread", "true")
		}
		if params.NeedsReply {
			q.Set("needs_reply", "true")
		}
		if params.Positive {
			q.Set("positive", "true")
		}
		setNonEmpty(q, "q", params.Query)
	}
	return listJSON[AgencyInboxThread](ctx, s.client, agencyPath(agencyID, "inbox"), q, opts...)
}

// InboxThread returns a page of one client conversation's messages, without
// marking anything read. Rows carry the snippet, not the body. Only
// params.Cursor is read; the server picks the page size.
//
// Scope: [PermAgencyRead] and [PermReadUnibox].
func (s *AgencyService) InboxThread(ctx context.Context, agencyID, organizationID, threadID string, params *ListOptions, opts ...RequestOption) (*Page[UniboxMessage], error) {
	q := url.Values{"organization_id": {organizationID}, "thread_id": {threadID}}
	if params != nil {
		setNonEmpty(q, "cursor", params.Cursor)
	}
	return listJSON[UniboxMessage](ctx, s.client, agencyPath(agencyID, "inbox", "thread"), q, opts...)
}

// InboxMessage returns one client message with its body, leaving it unread.
//
// Scope: [PermAgencyRead] and [PermReadUnibox].
func (s *AgencyService) InboxMessage(ctx context.Context, agencyID, organizationID, messageID string, opts ...RequestOption) (*UniboxMessage, *Response, error) {
	q := url.Values{"organization_id": {organizationID}}
	return fetch[UniboxMessage](ctx, s.client, withQuery(agencyPath(agencyID, "inbox", "messages", url.PathEscape(messageID)), q), opts)
}
