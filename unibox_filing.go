package warmbly

import "context"

// UniboxMoveFolderParams re-files messages into one canonical folder.
type UniboxMoveFolderParams struct {
	// EmailIDs are messages to file (at most 500).
	EmailIDs []string `json:"email_ids"`
	// ThreadIDs file whole conversations (at most 500), which is what a list
	// row names: filing half a conversation leaves it in the view it was
	// filed out of. Each entry is a thread id, or a message id for mail that
	// never got one.
	ThreadIDs []string `json:"thread_ids,omitempty"`
	// Folder is where the mail goes: [FolderInbox], [FolderArchive] or
	// [FolderTrash]. Sent, drafts and spam are verdicts the provider reaches
	// and are refused.
	Folder string `json:"folder"`
}

// UniboxMoveFolderResult echoes the filing that was applied.
type UniboxMoveFolderResult struct {
	EmailIDs  []string `json:"email_ids"`
	ThreadIDs []string `json:"thread_ids,omitempty"`
	Folder    string   `json:"folder"`
}

// MoveFolder re-files messages and conversations into the inbox, the archive
// or the trash. The store changes first; the move is then relayed to each
// mailbox that has folder-move relay switched on. The body names the
// destination, not a delta, so a retry is safe and needs no idempotency key.
// A credential limited to some mailboxes may only name messages and
// conversations in them. Needs the write_unibox scope.
func (s *UniboxService) MoveFolder(ctx context.Context, params *UniboxMoveFolderParams, opts ...RequestOption) (*UniboxMoveFolderResult, *Response, error) {
	return send[UniboxMoveFolderResult](ctx, s.client.patch, "unibox/folder", params, opts)
}

// ThreadDraftParams writes a reply draft for a conversation.
type ThreadDraftParams struct {
	// ThreadID is the conversation (at most 998 characters). The draft
	// answers its newest inbound message.
	ThreadID string `json:"thread_id"`
	// Body is the reply text, at most 8000 characters.
	Body string `json:"body"`
	// Source names the writer: letters, digits, dot, dash or underscore, at
	// most 40 characters. It is stored as the draft's model label.
	Source string `json:"source"`
	// IntentClass optionally classifies the inbound message (free text, at
	// most 40 characters).
	IntentClass string `json:"intent_class,omitempty"`
	// Confidence is the writer's confidence from 0 to 1.
	Confidence *float64 `json:"confidence,omitempty"`
}

// ThreadDraftResult is the outcome of [UniboxService.UpsertThreadDraft].
type ThreadDraftResult struct {
	// ID is the draft's id, usable with [UniboxService.ApproveAgentDraft] and
	// [UniboxService.DiscardAgentDraft].
	ID string `json:"id"`
	// Status is "pending": the draft waits for review.
	Status string `json:"status"`
	// Created is true when a new draft was written and false when the
	// conversation's pending draft was replaced.
	Created bool `json:"created"`
}

// UpsertThreadDraft writes a reply draft for the newest inbound message on a
// conversation, replacing the conversation's pending draft or creating one,
// for review in the drafts view. It never sends. The call states the whole
// draft, so a retry lands on the same one.
//
// A conversation with no inbound message is a 404 (code "thread_not_found"). A
// message that already has an approved reply is a 409 ("already_sent") and one
// whose draft was discarded is a 409 ("draft_discarded"). A credential limited
// to some mailboxes reaches only drafts in them. Needs the write_unibox scope.
func (s *UniboxService) UpsertThreadDraft(ctx context.Context, params *ThreadDraftParams, opts ...RequestOption) (*ThreadDraftResult, *Response, error) {
	return send[ThreadDraftResult](ctx, s.client.put, "unibox/ai-drafts/by-thread", params, opts)
}
