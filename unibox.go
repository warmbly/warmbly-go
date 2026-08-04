package warmbly

import (
	"context"
	"net/url"
	"strings"
	"time"
)

// UniboxService is the unified inbox: every connected mailbox's mail in one
// place, plus replying, composing, snoozing, conversation labels, scheduled
// sends, autosaved drafts and the AI drafting surfaces.
type UniboxService service

// UniboxThread is one conversation in the unified inbox list.
type UniboxThread struct {
	ID       string   `json:"id"`
	EmailID  string   `json:"email_id"`
	ThreadID string   `json:"thread_id"`
	FromAddr []string `json:"from_addr"`
	ToAddr   []string `json:"to_addr"`
	Subject  string   `json:"subject"`
	Snippet  string   `json:"snippet"`
	// InternalDate is the provider's timestamp for the newest message.
	InternalDate time.Time `json:"internal_date"`
	Seen         bool      `json:"seen"`
	MessageCount int       `json:"message_count"`
	HasUnread    bool      `json:"has_unread"`
	// Labels are the conversation labels on the thread. They share the
	// category registry with contact tags.
	Labels []MiniCategory `json:"labels,omitempty"`
}

// UniboxMessage is a single message inside a thread.
type UniboxMessage struct {
	ID      string `json:"id"`
	EmailID string `json:"email_id"`
	// Mailbox is the provider-side folder id.
	Mailbox   int      `json:"mailbox,omitempty"`
	ThreadID  string   `json:"thread_id"`
	MessageID string   `json:"message_id,omitempty"`
	GmailID   string   `json:"gmail_id,omitempty"`
	ParentID  string   `json:"parent_id,omitempty"`
	UID       int      `json:"uid,omitempty"`
	ModSeq    int      `json:"mod_seq,omitempty"`
	Flags     []string `json:"flags,omitempty"`

	BCC       []string `json:"bcc,omitempty"`
	CC        []string `json:"cc,omitempty"`
	FromAddr  []string `json:"from_addr,omitempty"`
	InReplyTo []string `json:"in_reply_to,omitempty"`
	ReplyTo   []string `json:"reply_to,omitempty"`
	ToAddr    []string `json:"to_addr,omitempty"`

	Subject      string    `json:"subject"`
	Size         int       `json:"size,omitempty"`
	InternalDate time.Time `json:"internal_date"`
	SentDate     time.Time `json:"sent_date,omitempty"`
	Snippet      string    `json:"snippet,omitempty"`
	Seen         bool      `json:"seen"`
	BodyPlain    string    `json:"body_plain,omitempty"`
	BodyHTML     string    `json:"body_html,omitempty"`

	UpdatedAt time.Time `json:"updated_at,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// UniboxOverview is the inbox sidebar: totals across the workspace, and per
// mailbox and per label.
type UniboxOverview struct {
	Total         int `json:"total"`
	Unread        int `json:"unread"`
	Today         int `json:"today"`
	Week          int `json:"week"`
	Snoozed       int `json:"snoozed"`
	AwaitingReply int `json:"awaiting_reply"`
	// ScheduledPending is how many sends are queued but not yet out, capped
	// for display at ScheduledPendingMax.
	ScheduledPending    int `json:"scheduled_pending"`
	ScheduledPendingMax int `json:"scheduled_pending_max,omitempty"`

	Mailboxes  []UniboxMailboxCount `json:"mailboxes,omitempty"`
	Tags       []UniboxLabelCount   `json:"tags,omitempty"`
	Categories []UniboxLabelCount   `json:"categories,omitempty"`

	GeneratedAt      time.Time `json:"generated_at,omitempty"`
	WindowTodayStart time.Time `json:"window_today_start,omitempty"`
	WindowWeekStart  time.Time `json:"window_week_start,omitempty"`
}

// UniboxMailboxCount is one mailbox's share of the inbox.
type UniboxMailboxCount struct {
	ID     string `json:"id"`
	Email  string `json:"email"`
	Name   string `json:"name,omitempty"`
	Unread int    `json:"unread"`
	Total  int    `json:"total"`
}

// UniboxLabelCount is one tag's or label's share of the inbox.
type UniboxLabelCount struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Color  string `json:"color,omitempty"`
	Unread int    `json:"unread"`
	Total  int    `json:"total"`
}

// UniboxSnooze hides a thread until SnoozedUntil, at which point it returns to
// the inbox.
type UniboxSnooze struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	ThreadID     string    `json:"thread_id"`
	SnoozedUntil time.Time `json:"snoozed_until"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
}

// ScheduledSend is a queued outbound message that has not left yet.
type ScheduledSend struct {
	TaskID       string    `json:"task_id"`
	ScheduledAt  time.Time `json:"scheduled_at"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
	AccountID    string    `json:"account_id"`
	AccountEmail string    `json:"account_email,omitempty"`
	AccountName  string    `json:"account_name,omitempty"`
	To           []string  `json:"to,omitempty"`
	Subject      string    `json:"subject"`
	Snippet      string    `json:"snippet,omitempty"`
	ThreadID     string    `json:"thread_id,omitempty"`
}

// UniboxListParams filters and paginates the unified inbox.
type UniboxListParams struct {
	ListOptions
	// From and Subject are substring filters.
	From    string
	Subject string
	// Unseen restricts to threads with unread messages.
	Unseen *bool
	// AwaitingReply restricts to threads whose latest message you sent.
	AwaitingReply *bool
	// Snoozed set to true returns only snoozed threads. Leave it nil to
	// exclude snoozed threads entirely.
	Snoozed *bool
	// Since and Until bound the thread date.
	Since time.Time
	Until time.Time
	// EmailID restricts to one mailbox; EmailIDs matches any of several.
	EmailID  string
	EmailIDs []string
	// CategoryIDs matches threads carrying any of the given conversation
	// labels.
	CategoryIDs []string
}

func (p *UniboxListParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	setNonEmpty(q, "from", p.From)
	setNonEmpty(q, "subject", p.Subject)
	setBool(q, "unseen", p.Unseen)
	setBool(q, "awaiting_reply", p.AwaitingReply)
	setBool(q, "snoozed", p.Snoozed)
	setNonEmpty(q, "since", formatDay(p.Since))
	setNonEmpty(q, "until", formatDay(p.Until))
	setNonEmpty(q, "email_id", p.EmailID)
	setNonEmpty(q, "email_ids", strings.Join(p.EmailIDs, ","))
	setNonEmpty(q, "category_ids", strings.Join(p.CategoryIDs, ","))
	return q
}

// UniboxReplyParams sends a reply from a mailbox into an existing thread.
type UniboxReplyParams struct {
	EmailAccountID string   `json:"email_account_id"`
	To             []string `json:"to"`
	CC             []string `json:"cc,omitempty"`
	BCC            []string `json:"bcc,omitempty"`
	Subject        string   `json:"subject"`
	BodyHTML       string   `json:"body_html,omitempty"`
	BodyPlain      string   `json:"body_plain,omitempty"`
	// InReplyTo holds the Message-IDs being replied to, and ThreadID keeps the
	// message in the provider's conversation.
	InReplyTo []string `json:"in_reply_to,omitempty"`
	ThreadID  string   `json:"thread_id,omitempty"`
	// SendMode is [SendModeInstant] (the default), [SendModeSmart] or
	// [SendModeScheduled].
	SendMode    string     `json:"send_mode,omitempty"`
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
}

// UniboxComposeParams sends a brand-new outbound email. Unlike a reply it is
// checked against the workspace suppression list before being queued.
type UniboxComposeParams struct {
	// EmailAccountID picks the sending mailbox. Leave it empty (or set it to
	// "auto") to let the server choose the best mailbox for the first
	// recipient; see [UniboxService.ComposeCandidates].
	EmailAccountID string `json:"email_account_id,omitempty"`
	// FromTagID scopes an automatic pick to mailboxes carrying that tag. It is
	// ignored when EmailAccountID names a mailbox.
	FromTagID string   `json:"from_tag_id,omitempty"`
	To        []string `json:"to"`
	CC        []string `json:"cc,omitempty"`
	BCC       []string `json:"bcc,omitempty"`
	Subject   string   `json:"subject"`
	BodyHTML  string   `json:"body_html,omitempty"`
	BodyPlain string   `json:"body_plain,omitempty"`
	// SendMode is [SendModeInstant] (the default), [SendModeSmart] or
	// [SendModeScheduled].
	SendMode    string     `json:"send_mode,omitempty"`
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
}

// ComposeCandidates is the compose mailbox picker: every active mailbox scored
// against the recipient, the automatic recommendation and why, plus what is
// known about the address.
type ComposeCandidates struct {
	Accounts []ComposeCandidate `json:"accounts"`
	// RecommendedAccountID is what automatic selection would pick.
	RecommendedAccountID *string `json:"recommended_account_id"`
	RecommendedReason    string  `json:"recommended_reason"`
	// Contact is the resolved contact for the address, when there is one.
	Contact *Contact `json:"contact"`
	// Suppression is non-nil when the address is suppressed workspace-wide, in
	// which case a compose to it will be rejected.
	Suppression *ComposeSuppression `json:"suppression"`
}

// ComposeSuppression explains why an address cannot be mailed.
type ComposeSuppression struct {
	Reason string `json:"reason"`
}

// ComposeCandidate is one mailbox scored as a sender for a recipient.
type ComposeCandidate struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	// AuthState is the sending domain's SPF/DKIM/DMARC verdict.
	AuthState    string `json:"auth_state"`
	WarmupActive bool   `json:"warmup_active"`
	DailyLimit   int    `json:"daily_limit"`
	SentToday    int    `json:"sent_today"`
	// RemainingToday is how much of the daily allowance is left.
	RemainingToday int `json:"remaining_today"`
	// HistoryMessages counts prior traffic between this mailbox and the
	// recipient, which is the strongest affinity signal.
	HistoryMessages int        `json:"history_messages"`
	LastContactAt   *time.Time `json:"last_contact_at,omitempty"`
	// Score ranks the candidate; Reasons explains the ranking in words.
	Score       int      `json:"score"`
	Reasons     []string `json:"reasons"`
	Recommended bool     `json:"recommended"`
}

// ComposeDraft is an autosaved compose draft. Ids are client-generated, which
// makes the upsert idempotent under debounced autosave.
type ComposeDraft struct {
	ID             string    `json:"id"`
	EmailAccountID *string   `json:"email_account_id,omitempty"`
	To             []string  `json:"to"`
	CC             []string  `json:"cc"`
	BCC            []string  `json:"bcc"`
	Subject        string    `json:"subject"`
	Body           string    `json:"body"`
	UpdatedAt      time.Time `json:"updated_at"`
	CreatedAt      time.Time `json:"created_at"`
}

// ComposeDraftParams is the body of an autosave.
type ComposeDraftParams struct {
	EmailAccountID string   `json:"email_account_id,omitempty"`
	To             []string `json:"to,omitempty"`
	CC             []string `json:"cc,omitempty"`
	BCC            []string `json:"bcc,omitempty"`
	Subject        string   `json:"subject,omitempty"`
	Body           string   `json:"body,omitempty"`
}

// AIDraft is a generated draft. It is never sent by the call that produced it.
type AIDraft struct {
	// Text is the drafted message. It is empty when the model asked a
	// clarifying Question instead.
	Text string `json:"text,omitempty"`
	// Question is a single clarifying question, returned when the request gave
	// the model too little to work with.
	Question string `json:"question,omitempty"`
	// CreditsCharged is what this draft cost; CreditsRemaining is the balance
	// afterwards.
	CreditsCharged   int    `json:"credits_charged"`
	CreditsRemaining int    `json:"credits_remaining"`
	TokensUsed       int    `json:"tokens_used"`
	Model            string `json:"model"`
	// Grounding reports what context the draft was written against. It is only
	// returned by [UniboxService.DraftCompose].
	Grounding *AIDraftGrounding `json:"grounding,omitempty"`
}

// AIDraftGrounding reports which context sources fed a compose draft.
type AIDraftGrounding struct {
	// Contact is true when a contact record was found for the recipient.
	Contact bool `json:"contact"`
	// History is how many prior messages with the address were included.
	History int `json:"history"`
	// VoiceProfile is true when the workspace voice profile was applied.
	VoiceProfile bool `json:"voice_profile"`
}

// Inbox-agent draft states returned in [AgentDraft.Status].
const (
	AgentDraftPending   = "pending"
	AgentDraftApproved  = "approved"
	AgentDraftDiscarded = "discarded"
)

// AgentDraft is a reply the inbox agent suggested for an inbound human reply.
// It is never sent until someone approves it.
type AgentDraft struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	EmailAccountID string `json:"email_account_id"`
	// OwnerUserID is the mailbox owner; an approved send goes out as them.
	OwnerUserID string `json:"owner_user_id"`
	ThreadID    string `json:"thread_id"`
	// SourceMessageID is the inbound message being replied to.
	SourceMessageID *string `json:"source_message_id,omitempty"`
	ContactID       *string `json:"contact_id,omitempty"`
	CampaignID      *string `json:"campaign_id,omitempty"`

	ToAddr  string `json:"to_addr"`
	Subject string `json:"subject"`
	// InReplyTo is the Message-ID referenced on send.
	InReplyTo string `json:"in_reply_to"`
	Body      string `json:"body"`
	// IntentClass is how the agent read the inbound reply, and Confidence how
	// sure it was.
	IntentClass string  `json:"intent_class"`
	Confidence  float64 `json:"confidence"`
	Model       string  `json:"model"`
	// Status is one of the AgentDraft* constants.
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// List returns a page of inbox threads, newest first.
func (s *UniboxService) List(ctx context.Context, params *UniboxListParams, opts ...RequestOption) (*Page[UniboxThread], error) {
	return listJSON[UniboxThread](ctx, s.client, "unibox", params.values(), opts...)
}

// UnseenCount returns how many unread messages there are, across the workspace
// or in a single mailbox when emailID is set.
func (s *UniboxService) UnseenCount(ctx context.Context, emailID string, opts ...RequestOption) (int, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "email_id", emailID)
	var out struct {
		Count int `json:"count"`
	}
	resp, err := s.client.get(ctx, withQuery("unibox/count", q), &out, opts...)
	if err != nil {
		return 0, resp, err
	}
	return out.Count, resp, nil
}

// Overview returns the inbox sidebar counts.
func (s *UniboxService) Overview(ctx context.Context, opts ...RequestOption) (*UniboxOverview, *Response, error) {
	return fetch[UniboxOverview](ctx, s.client, "unibox/overview", opts)
}

// Thread returns a page of the messages in one conversation, oldest first.
// Pass an empty emailID to search every mailbox.
func (s *UniboxService) Thread(ctx context.Context, threadID, emailID string, params *ListOptions, opts ...RequestOption) (*Page[UniboxMessage], error) {
	q := make(url.Values)
	params.apply(q)
	q.Set("thread_id", threadID)
	setNonEmpty(q, "email_id", emailID)
	return listJSON[UniboxMessage](ctx, s.client, "unibox/thread", q, opts...)
}

// Get returns a single message by id, including its bodies.
func (s *UniboxService) Get(ctx context.Context, id string, opts ...RequestOption) (*UniboxMessage, *Response, error) {
	return fetch[UniboxMessage](ctx, s.client, "unibox/"+url.PathEscape(id), opts)
}

// ThreadLabels returns the conversation labels on a thread.
func (s *UniboxService) ThreadLabels(ctx context.Context, threadID string, opts ...RequestOption) ([]MiniCategory, *Response, error) {
	q := url.Values{"thread_id": {threadID}}
	return fetchData[MiniCategory](ctx, s.client, withQuery("unibox/thread/labels", q), opts)
}

// SetThreadLabels replaces a thread's conversation labels wholesale, which
// makes it idempotent. Labels are contact categories; manage them with
// [GroupService].
func (s *UniboxService) SetThreadLabels(ctx context.Context, threadID string, categoryIDs []string, opts ...RequestOption) ([]MiniCategory, *Response, error) {
	body := struct {
		ThreadID    string   `json:"thread_id"`
		CategoryIDs []string `json:"category_ids"`
	}{ThreadID: threadID, CategoryIDs: categoryIDs}
	return sendData[MiniCategory](ctx, s.client.put, "unibox/thread/labels", body, opts)
}

// MarkSeen marks messages read or unread.
func (s *UniboxService) MarkSeen(ctx context.Context, emailIDs []string, seen bool, opts ...RequestOption) (*Response, error) {
	body := struct {
		EmailIDs []string `json:"email_ids"`
		Seen     bool     `json:"seen"`
	}{EmailIDs: emailIDs, Seen: seen}
	return s.client.patch(ctx, "unibox/seen", body, nil, opts...)
}

// Reply sends a reply into an existing thread.
func (s *UniboxService) Reply(ctx context.Context, params *UniboxReplyParams, opts ...RequestOption) (*SendResult, *Response, error) {
	return send[SendResult](ctx, s.client.post, "unibox/reply", params, opts)
}

// Compose sends a brand-new outbound email.
func (s *UniboxService) Compose(ctx context.Context, params *UniboxComposeParams, opts ...RequestOption) (*SendResult, *Response, error) {
	return send[SendResult](ctx, s.client.post, "unibox/compose", params, opts)
}

// ComposeCandidates scores the workspace's mailboxes as senders for one
// recipient, so a picker can explain the automatic choice.
func (s *UniboxService) ComposeCandidates(ctx context.Context, to string, opts ...RequestOption) (*ComposeCandidates, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "to", to)
	return fetch[ComposeCandidates](ctx, s.client, withQuery("unibox/compose/candidates", q), opts)
}

// DraftCompose writes a new email grounded in the contact record, prior
// correspondence and the workspace voice profile. It spends AI credits and
// never sends. When the request gives it too little to work with it returns a
// clarifying [AIDraft.Question] instead of a draft.
func (s *UniboxService) DraftCompose(ctx context.Context, to, subject, instruction string, opts ...RequestOption) (*AIDraft, *Response, error) {
	body := struct {
		To          string `json:"to,omitempty"`
		Subject     string `json:"subject,omitempty"`
		Instruction string `json:"instruction,omitempty"`
	}{To: to, Subject: subject, Instruction: instruction}
	return send[AIDraft](ctx, s.client.post, "unibox/compose/draft", body, opts)
}

// DraftReply writes a reply grounded in the thread's history. It spends AI
// credits and never sends.
func (s *UniboxService) DraftReply(ctx context.Context, threadID, instruction string, opts ...RequestOption) (*AIDraft, *Response, error) {
	body := struct {
		ThreadID    string `json:"thread_id"`
		Instruction string `json:"instruction,omitempty"`
	}{ThreadID: threadID, Instruction: instruction}
	return send[AIDraft](ctx, s.client.post, "unibox/reply/draft", body, opts)
}

// --- autosaved compose drafts ---

// Drafts returns the caller's autosaved compose drafts.
func (s *UniboxService) Drafts(ctx context.Context, opts ...RequestOption) ([]ComposeDraft, *Response, error) {
	return fetchData[ComposeDraft](ctx, s.client, "unibox/drafts", opts)
}

// SaveDraft creates or replaces an autosaved compose draft. The id is
// client-generated, so repeating the call with the same id is an update rather
// than a new draft.
func (s *UniboxService) SaveDraft(ctx context.Context, id string, params *ComposeDraftParams, opts ...RequestOption) (*Response, error) {
	return s.client.put(ctx, "unibox/drafts/"+url.PathEscape(id), params, nil, opts...)
}

// DeleteDraft discards an autosaved compose draft.
func (s *UniboxService) DeleteDraft(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "unibox/drafts/"+url.PathEscape(id), opts...)
}

// --- inbox-agent drafts ---

// AgentDrafts returns the workspace's pending inbox-agent drafts, newest
// first. The agent only runs when the workspace has opted in via
// [Organization.InboxAgentEnabled].
func (s *UniboxService) AgentDrafts(ctx context.Context, opts ...RequestOption) ([]AgentDraft, *Response, error) {
	return fetchData[AgentDraft](ctx, s.client, "unibox/agent-drafts", opts)
}

// ApproveAgentDraft sends an agent draft through the normal reply path,
// optionally replacing its body first. Approval is claimed before the send, so
// two concurrent approvals can never double-send; the loser gets a 409.
func (s *UniboxService) ApproveAgentDraft(ctx context.Context, id, body string, opts ...RequestOption) (*SendResult, *Response, error) {
	req := struct {
		Body string `json:"body,omitempty"`
	}{Body: body}
	return send[SendResult](ctx, s.client.post, "unibox/agent-drafts/"+url.PathEscape(id)+"/approve", req, opts)
}

// DiscardAgentDraft throws an agent draft away unsent.
func (s *UniboxService) DiscardAgentDraft(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "unibox/agent-drafts/"+url.PathEscape(id)+"/discard", nil, nil, opts...)
}

// --- snoozes ---

// Snoozes returns the caller's active snoozes.
func (s *UniboxService) Snoozes(ctx context.Context, opts ...RequestOption) ([]UniboxSnooze, *Response, error) {
	return fetchData[UniboxSnooze](ctx, s.client, "unibox/snoozes", opts)
}

// Snooze hides a thread until the given time.
func (s *UniboxService) Snooze(ctx context.Context, threadID string, until time.Time, opts ...RequestOption) (*UniboxSnooze, *Response, error) {
	body := struct {
		ThreadID     string    `json:"thread_id"`
		SnoozedUntil time.Time `json:"snoozed_until"`
	}{ThreadID: threadID, SnoozedUntil: until}
	return send[UniboxSnooze](ctx, s.client.post, "unibox/snooze", body, opts)
}

// Unsnooze returns a snoozed thread to the inbox immediately.
func (s *UniboxService) Unsnooze(ctx context.Context, threadID string, opts ...RequestOption) (*Response, error) {
	q := url.Values{"thread_id": {threadID}}
	return s.client.delete(ctx, withQuery("unibox/snooze", q), opts...)
}

// --- scheduled sends ---

// Scheduled returns queued sends that have not left yet. Pass a threadID to
// narrow it to one conversation.
func (s *UniboxService) Scheduled(ctx context.Context, threadID string, opts ...RequestOption) ([]ScheduledSend, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "thread_id", threadID)
	return fetchData[ScheduledSend](ctx, s.client, withQuery("unibox/scheduled", q), opts)
}

// CancelScheduled cancels a queued send. The cancellation is recorded
// immediately and the task short-circuits when its turn comes.
func (s *UniboxService) CancelScheduled(ctx context.Context, taskID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "unibox/scheduled/"+url.PathEscape(taskID), opts...)
}
