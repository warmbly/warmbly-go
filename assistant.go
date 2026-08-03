package warmbly

import (
	"context"
	"encoding/json"
	"net/url"
	"time"
)

// AssistantService drives the workspace AI assistant: conversations, their
// transcripts, and the streamed runs that answer a message.
//
// The assistant acts as the calling member. Every tool it runs is re-checked
// against that member's permissions, and anything that sends mail or spends
// money pauses for an explicit approval. Runs spend AI credits.
//
// These routes are session-only — no API key can drive the assistant — and need
// the use-AI permission.
type AssistantService service

// AgentSession is one assistant conversation.
type AgentSession struct {
	ID     string `json:"id"`
	OrgID  string `json:"org_id"`
	UserID string `json:"user_id"`
	Title  string `json:"title"`
	// UserName attributes the conversation in a workspace-shared history; see
	// [Organization.AssistantSharedHistory].
	UserName  string              `json:"user_name,omitempty"`
	Context   AgentSessionContext `json:"context"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
}

// AgentSessionContext is what the assistant knows about the session beyond its
// messages.
type AgentSessionContext struct {
	// Page and Resource are what the caller was looking at, folded into the
	// prompt so "this campaign" resolves.
	Page     string `json:"page,omitempty"`
	Resource string `json:"resource,omitempty"`
	// Model is the provider model resolved for this session.
	Model string `json:"model,omitempty"`
	// FreeModel reports that the run used a free or local backend, in which
	// case no credits were charged.
	FreeModel bool `json:"free_model,omitempty"`
	// Pending is the tool call awaiting approval when a run is paused.
	Pending *PendingTool `json:"pending,omitempty"`
}

// PendingTool is a tool call paused for human approval.
type PendingTool struct {
	MessageID  string `json:"message_id"`
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	// Risk is how consequential the call is, for example "send".
	Risk string `json:"risk"`
	// Args is the exact payload the tool would run with, and ArgsSummary a
	// human-readable rendering of it.
	Args        json.RawMessage `json:"args"`
	ArgsSummary string          `json:"args_summary,omitempty"`
}

// AgentTranscript is a session rehydrated for display.
type AgentTranscript struct {
	Title string      `json:"title"`
	Turns []AgentTurn `json:"turns"`
	// Pending is set when the conversation is waiting on an approval.
	Pending *PendingTool `json:"pending,omitempty"`
	// FreeModel reports that the session ran without charging credits.
	FreeModel bool `json:"free_model"`
}

// AgentTurn is one user or assistant turn. An assistant turn interleaves text
// and tool steps in the order they happened.
type AgentTurn struct {
	// Role is "user" or "assistant".
	Role   string       `json:"role"`
	Blocks []AgentBlock `json:"blocks"`
}

// AgentBlock is one piece of a turn.
type AgentBlock struct {
	// Kind is "text" or "tool".
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`

	Tool        string `json:"tool,omitempty"`
	ArgsSummary string `json:"args_summary,omitempty"`
	Result      string `json:"result,omitempty"`

	// EntityType, EntityID and OpenURL point at something the tool created, so
	// a client can link straight to it.
	EntityType string `json:"entity_type,omitempty"`
	EntityID   string `json:"entity_id,omitempty"`
	OpenURL    string `json:"open_url,omitempty"`

	// Done is false while a tool step is still running.
	Done bool `json:"done"`
}

// Event types streamed during a run, in [AgentEvent.Type].
const (
	// AgentEventTextDelta is an incremental chunk of the reply.
	AgentEventTextDelta = "text_delta"
	// AgentEventText is a complete block of reply text.
	AgentEventText = "text"
	// AgentEventToolStart announces a tool call beginning.
	AgentEventToolStart = "tool_start"
	// AgentEventToolResult carries what a tool returned.
	AgentEventToolResult = "tool_result"
	// AgentEventApprovalRequired pauses the run until
	// [AssistantService.Approve] answers.
	AgentEventApprovalRequired = "approval_required"
	// AgentEventError ends the run with a failure.
	AgentEventError = "error"
	// AgentEventDone ends the run normally.
	AgentEventDone = "done"
)

// Decisions accepted by [AssistantService.Approve].
const (
	// ApprovalApprove runs the paused tool call once.
	ApprovalApprove = "approve"
	// ApprovalDeny abandons it and lets the assistant continue without it.
	ApprovalDeny = "deny"
	// ApprovalAlwaysAllow runs it and stops asking for that tool in this
	// session.
	ApprovalAlwaysAllow = "always_allow"
)

// AgentEvent is one step of a streamed run. Which fields are set depends on
// Type.
type AgentEvent struct {
	// Type is one of the AgentEvent* constants.
	Type string `json:"type"`
	// Text is reply content, whole or incremental.
	Text string `json:"text,omitempty"`

	// Tool, Risk, ArgsSummary and ToolCallID describe a tool step.
	Tool        string `json:"tool,omitempty"`
	Risk        string `json:"risk,omitempty"`
	ArgsSummary string `json:"args_summary,omitempty"`
	ToolCallID  string `json:"tool_call_id,omitempty"`
	Result      string `json:"result,omitempty"`

	// Iteration counts the assistant's turns within the run, and Budget caps
	// them.
	Iteration int `json:"iteration,omitempty"`
	Budget    int `json:"budget,omitempty"`
	// CreditsRemaining is the balance after this step; FreeModel means nothing
	// was charged.
	CreditsRemaining int  `json:"credits_remaining,omitempty"`
	FreeModel        bool `json:"free_model,omitempty"`

	// Code and Message are set on [AgentEventError].
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`

	// EntityType, EntityID and OpenURL point at something a tool created.
	EntityType string `json:"entity_type,omitempty"`
	EntityID   string `json:"entity_id,omitempty"`
	OpenURL    string `json:"open_url,omitempty"`
}

// MessageParams sends a message to the assistant.
type MessageParams struct {
	// MessageID deduplicates a retried send. Leave it empty and the server
	// generates one.
	MessageID string `json:"message_id,omitempty"`
	Text      string `json:"text"`
	// Page and Resource tell the assistant what the user is looking at, so
	// references like "this campaign" resolve.
	Page     string `json:"page,omitempty"`
	Resource string `json:"resource,omitempty"`
}

// CreateSession opens a new conversation, optionally anchored to what the user
// is looking at.
func (s *AssistantService) CreateSession(ctx context.Context, page, resource string, opts ...RequestOption) (*AgentSession, *Response, error) {
	body := struct {
		Page     string `json:"page,omitempty"`
		Resource string `json:"resource,omitempty"`
	}{Page: page, Resource: resource}
	return send[AgentSession](ctx, s.client, s.client.post, "ai/sessions", body, opts)
}

// Sessions returns a page of the caller's conversations, newest first. With
// [Organization.AssistantSharedHistory] on, it returns the whole workspace's.
func (s *AssistantService) Sessions(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[AgentSession], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[AgentSession](ctx, s.client, "ai/sessions", q, opts...)
}

// Transcript returns a conversation rehydrated for display, including any
// pending approval.
func (s *AssistantService) Transcript(ctx context.Context, sessionID string, opts ...RequestOption) (*AgentTranscript, *Response, error) {
	return fetch[AgentTranscript](ctx, s.client, "ai/sessions/"+url.PathEscape(sessionID)+"/messages", opts)
}

// DeleteSession removes one conversation.
func (s *AssistantService) DeleteSession(ctx context.Context, sessionID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "ai/sessions/"+url.PathEscape(sessionID), opts...)
}

// ClearSessions removes every conversation the caller can see.
func (s *AssistantService) ClearSessions(ctx context.Context, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "ai/sessions", opts...)
}

// SendMessage runs the assistant on a message and streams the run, calling fn
// for each step as it arrives. Return false from fn to stop reading early.
//
// The run pauses at [AgentEventApprovalRequired] and ends there; answer it with
// [AssistantService.Approve], which streams the remainder.
//
//	_, err := client.Assistant.SendMessage(ctx, sessionID,
//		&warmbly.MessageParams{Text: "Which campaigns are bouncing?"},
//		func(ev *warmbly.AgentEvent) bool {
//			if ev.Type == warmbly.AgentEventTextDelta {
//				fmt.Print(ev.Text)
//			}
//			return true
//		})
func (s *AssistantService) SendMessage(ctx context.Context, sessionID string, params *MessageParams, fn func(*AgentEvent) bool, opts ...RequestOption) (*Response, error) {
	return s.streamRun(ctx, "ai/sessions/"+url.PathEscape(sessionID)+"/messages", params, fn, opts)
}

// Approve answers a paused approval and streams the rest of the run. The
// decision is [ApprovalApprove], [ApprovalDeny] or [ApprovalAlwaysAllow].
func (s *AssistantService) Approve(ctx context.Context, sessionID, decision string, fn func(*AgentEvent) bool, opts ...RequestOption) (*Response, error) {
	body := struct {
		Decision string `json:"decision"`
	}{Decision: decision}
	return s.streamRun(ctx, "ai/sessions/"+url.PathEscape(sessionID)+"/approve", body, fn, opts)
}

func (s *AssistantService) streamRun(ctx context.Context, path string, body any, fn func(*AgentEvent) bool, opts []RequestOption) (*Response, error) {
	return s.client.stream(ctx, path, body, func(data []byte) bool {
		ev := new(AgentEvent)
		if err := json.Unmarshal(data, ev); err != nil {
			// Skip a frame we cannot read rather than ending the run.
			return true
		}
		if fn == nil {
			return true
		}
		return fn(ev)
	}, opts...)
}

// --- connected MCP servers ---

// MCPServer is an external MCP server whose tools the assistant may call.
type MCPServer struct {
	ID    string `json:"id"`
	OrgID string `json:"org_id"`
	Name  string `json:"name"`
	URL   string `json:"url"`
	// AuthType is "none" or "bearer". The token itself is sealed server-side
	// and never returned.
	AuthType string `json:"auth_type"`
	Enabled  bool   `json:"enabled"`
	// DiscoveredTools is what the server advertised on the last refresh.
	DiscoveredTools []MCPTool `json:"discovered_tools"`
	// LastError is why the most recent refresh failed, when it did.
	LastError string    `json:"last_error,omitempty"`
	CreatedBy *string   `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MCPTool is one tool discovered on a connected MCP server.
type MCPTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// InputSchema is the tool's JSON Schema.
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

// MCPServerParams connects an MCP server.
type MCPServerParams struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	// AuthType is "none" or "bearer".
	AuthType string `json:"auth_type,omitempty"`
	// Token is the bearer credential, sealed server-side on receipt.
	Token string `json:"token,omitempty"`
}

// MCPServerUpdateParams updates a connected server. Nil fields are unchanged.
type MCPServerUpdateParams struct {
	Name    *string `json:"name,omitempty"`
	Enabled *bool   `json:"enabled,omitempty"`
	// Token replaces the stored credential.
	Token *string `json:"token,omitempty"`
}

// MCPServers returns the workspace's connected MCP servers.
func (s *AssistantService) MCPServers(ctx context.Context, opts ...RequestOption) ([]MCPServer, *Response, error) {
	return fetchData[MCPServer](ctx, s.client, "ai/connections", opts)
}

// ConnectMCPServer registers an external MCP server and discovers its tools.
func (s *AssistantService) ConnectMCPServer(ctx context.Context, params *MCPServerParams, opts ...RequestOption) (*MCPServer, *Response, error) {
	return send[MCPServer](ctx, s.client, s.client.post, "ai/connections", params, opts)
}

// UpdateMCPServer changes a connected server's name, credential or enabled
// state.
func (s *AssistantService) UpdateMCPServer(ctx context.Context, id string, params *MCPServerUpdateParams, opts ...RequestOption) (*MCPServer, *Response, error) {
	return send[MCPServer](ctx, s.client, s.client.patch, "ai/connections/"+url.PathEscape(id), params, opts)
}

// DeleteMCPServer disconnects a server and forgets its credential.
func (s *AssistantService) DeleteMCPServer(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "ai/connections/"+url.PathEscape(id), opts...)
}

// RefreshMCPServer re-discovers a connected server's tools.
func (s *AssistantService) RefreshMCPServer(ctx context.Context, id string, opts ...RequestOption) (*MCPServer, *Response, error) {
	return send[MCPServer](ctx, s.client, s.client.post, "ai/connections/"+url.PathEscape(id)+"/refresh", nil, opts)
}
