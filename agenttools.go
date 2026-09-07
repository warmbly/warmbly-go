package warmbly

import (
	"context"
	"encoding/json"
	"net/url"
)

// AgentToolService is the AI tool registry over plain HTTP, for
// function-calling agents that do not speak MCP: Hermes-style models,
// OpenAI-compatible frameworks, LangChain executors, plain scripts. It is the
// same registry that POST /v1/mcp exposes, gated the same way, so a tool
// behaves identically whichever door it came through.
//
// There is no route-level scope on these endpoints. Each tool enforces its own
// permission — the API-key scope for key and OAuth callers, the organization
// permission for session callers — and [AgentToolService.List] only ever shows
// what the caller may use, so the whole list can be handed to a model as-is.
// Send-class tools (anything that puts real mail on the wire) are never listed
// and never callable here: an agent wired through this surface can read,
// search, label and draft, but a human presses send.
//
// A call runs as the caller with their permissions, exactly as the matching
// REST route would; a tool can never do more than the credential could through
// the normal API. Write-class tools audit like the routes they wrap.
type AgentToolService service

// AgentTool is one registry tool the caller is permitted to use, as returned
// by [AgentToolService.List].
//
// The server does not report a tool's risk class or the scope it checks; the
// list is already filtered to what the credential allows, so everything in it
// is callable with the same credential.
type AgentTool struct {
	// Name is the identifier to pass to [AgentToolService.Call].
	Name string `json:"name"`
	// Description is the model-facing explanation of what the tool does and
	// when to use it.
	Description string `json:"description"`
	// InputSchema is the JSON Schema of the argument object, kept verbatim so
	// it drops into a function-calling client unchanged. It is always an
	// object schema; a tool with no arguments has an empty properties map.
	InputSchema json.RawMessage `json:"input_schema"`
}

// Function converts the tool to the OpenAI function-calling shape, without a
// round trip to the server. It is what [AgentToolService.ListFunctions]
// returns, built locally.
func (t AgentTool) Function() AgentToolFunction {
	return AgentToolFunction{
		Type: "function",
		Function: AgentToolFunctionSpec{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.InputSchema,
		},
	}
}

// AgentToolFunctions converts a tool list to OpenAI function-calling objects,
// for a client that fetched the default shape and also wants the manifest.
func AgentToolFunctions(tools []AgentTool) []AgentToolFunction {
	out := make([]AgentToolFunction, 0, len(tools))
	for _, t := range tools {
		out = append(out, t.Function())
	}
	return out
}

// AgentToolFunction is a tool in OpenAI function-calling form:
// {"type":"function","function":{...}}. The objects go verbatim into an
// OpenAI-compatible tools array or a Hermes <tools> block.
type AgentToolFunction struct {
	// Type is always "function".
	Type     string                `json:"type"`
	Function AgentToolFunctionSpec `json:"function"`
}

// AgentToolFunctionSpec is the function half of an [AgentToolFunction].
type AgentToolFunctionSpec struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Parameters is the argument JSON Schema, the same bytes as
	// [AgentTool.InputSchema].
	Parameters json.RawMessage `json:"parameters"`
}

// AgentToolCallResult is what one tool call returned.
type AgentToolCallResult struct {
	// Name echoes the tool that ran.
	Name string `json:"name"`
	// Result is the tool's output. It is the tool's JSON embedded directly
	// when the tool produced JSON (every registry tool does today) and a JSON
	// string otherwise, so it is always valid JSON and can be fed back to the
	// model as the tool-call result unchanged.
	Result json.RawMessage `json:"result"`
}

// Decode unmarshals the tool's output into v, for callers that know the shape
// a particular tool produces.
func (r *AgentToolCallResult) Decode(v any) error {
	return json.Unmarshal(r.Result, v)
}

// List returns the tools the caller's credential may use, in registry order,
// in the default {name, description, input_schema} shape. Use [AgentTool.Function]
// or [AgentToolFunctions] to convert locally, or [AgentToolService.ListFunctions]
// to have the server do it.
//
// Fetch it once at session start: the set only changes when the credential's
// scopes do. The read rate limit applies.
func (s *AgentToolService) List(ctx context.Context, opts ...RequestOption) ([]AgentTool, *Response, error) {
	return fetchData[AgentTool](ctx, s.client, "ai/tools", opts)
}

// ListFunctions is [AgentToolService.List] with the server rendering OpenAI
// function-calling objects (GET /ai/tools?format=openai), ready for an
// OpenAI-compatible tools array or a Hermes <tools> block. The filtering is
// identical; only the shape differs.
func (s *AgentToolService) ListFunctions(ctx context.Context, opts ...RequestOption) ([]AgentToolFunction, *Response, error) {
	opts = append([]RequestOption{WithQueryParam("format", "openai")}, opts...)
	return fetchData[AgentToolFunction](ctx, s.client, "ai/tools", opts)
}

// Call runs one tool by name. args is the tool's argument object exactly as the
// model produced it — a map, a struct, or the raw [json.RawMessage] from the
// model's tool call — and must encode to a JSON object matching the tool's
// [AgentTool.InputSchema]. A nil args sends no body, which the server treats as
// no arguments.
//
// Failures map onto the package sentinels:
//
//   - [ErrNotFound] (404, code "not_found"): no such tool. A send-class tool
//     answers the same way on purpose, so a name a model saw elsewhere (for
//     example the dashboard assistant's send_reply) is indistinguishable from
//     a typo here.
//   - [ErrForbidden] (403, code "forbidden"): the credential lacks the tool's
//     permission. It would not have appeared in [AgentToolService.List].
//   - [ErrBadRequest] (400, code "bad_request"): the body was not valid JSON.
//   - [ErrUnprocessable] (422, code "unprocessable"): the tool itself failed —
//     arguments that decoded but did not validate, a missing record, an
//     entitlement the workspace lacks. The [Error.Message] is written for the
//     model to read and react to; feed it back as the tool result rather than
//     treating it as a transport error.
//
// Calls run under the write rate limit and honor [WithIdempotencyKey] like
// every other mutating request, so a retried call with the same key replays
// the stored result instead of running the tool again.
func (s *AgentToolService) Call(ctx context.Context, name string, args any, opts ...RequestOption) (*AgentToolCallResult, *Response, error) {
	var env struct {
		Data AgentToolCallResult `json:"data"`
	}
	resp, err := s.client.post(ctx, "ai/tools/"+url.PathEscape(name)+"/call", args, &env, opts...)
	if err != nil {
		return nil, resp, err
	}
	return &env.Data, resp, nil
}
