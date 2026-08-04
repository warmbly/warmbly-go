package warmbly

import (
	"context"
	"encoding/json"
	"net/url"
	"time"
)

// AutomationService manages automations: a trigger event plus a graph of
// condition and action nodes that runs across connected integrations.
//
// An automation can also be triggered from outside Warmbly. Each one gets a
// token-addressed inbound URL that runs it with the POSTed JSON body as the
// event payload.
type AutomationService service

// Node kinds in an automation graph.
const (
	// NodeTrigger is the entry point; there is exactly one.
	NodeTrigger = "trigger"
	// NodeCondition branches on the event data.
	NodeCondition = "condition"
	// NodeAction performs work, usually against a connection.
	NodeAction = "action"
)

// Per-node outcomes returned in [AutomationNodeResult.Status].
const (
	NodeStatusSuccess     = "success"
	NodeStatusError       = "error"
	NodeStatusSkipped     = "skipped"
	NodeStatusBranchTrue  = "branch_true"
	NodeStatusBranchFalse = "branch_false"
)

// Automation run outcomes returned in [AutomationRun.Status].
const (
	AutomationRunning = "running"
	AutomationSuccess = "success"
	AutomationError   = "error"
)

// Automation is a trigger plus the graph that runs when it fires.
type Automation struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	Enabled        bool   `json:"enabled"`
	// TriggerEvent is the event key that starts the run; see the Event*
	// constants.
	TriggerEvent string `json:"trigger_event"`
	// Filter narrows which occurrences of the trigger event actually run it.
	Filter json.RawMessage `json:"filter,omitempty"`
	Graph  AutomationGraph `json:"graph"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AutomationGraph is the node graph, as drawn on the builder canvas.
type AutomationGraph struct {
	Nodes []AutomationNode `json:"nodes"`
	Edges []AutomationEdge `json:"edges"`
}

// AutomationNode is one step. Which fields matter depends on Type.
type AutomationNode struct {
	ID string `json:"id"`
	// Type is [NodeTrigger], [NodeCondition] or [NodeAction].
	Type string `json:"type"`
	// Action is the provider action identifier for an action node.
	Action string `json:"action,omitempty"`
	// ConnectionID is the integration the action runs against.
	ConnectionID *string `json:"connection_id,omitempty"`
	// Config is the action's parameters.
	Config json.RawMessage `json:"config,omitempty"`
	// Condition is the test a condition node applies.
	Condition *AutomationCondition `json:"condition,omitempty"`
	// X and Y are the node's canvas coordinates.
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// AutomationCondition is a condition node's test.
type AutomationCondition struct {
	// Field names the event field to read, and Key indexes into it when the
	// field is a map.
	Field string `json:"field"`
	Key   string `json:"key,omitempty"`
	// Operator is the comparison, for example "equals" or "contains".
	Operator string `json:"operator"`
	Value    any    `json:"value,omitempty"`
	// Expression is an alternative to Field/Operator/Value for conditions that
	// need more than a single comparison.
	Expression string `json:"expression,omitempty"`
}

// AutomationEdge connects two nodes. When is set on the outgoing edges of a
// condition node to pick the true or false branch.
type AutomationEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	When   string `json:"when,omitempty"`
}

// AutomationRun is one execution, with a per-node trace.
type AutomationRun struct {
	ID             string `json:"id"`
	AutomationID   string `json:"automation_id"`
	OrganizationID string `json:"organization_id,omitempty"`
	TriggerEvent   string `json:"trigger_event"`
	// Status is [AutomationRunning], [AutomationSuccess] or [AutomationError].
	Status      string                 `json:"status"`
	NodeResults []AutomationNodeResult `json:"node_results"`
	ErrorDetail string                 `json:"error_detail,omitempty"`
	StartedAt   time.Time              `json:"started_at"`
	FinishedAt  *time.Time             `json:"finished_at,omitempty"`
}

// AutomationNodeResult is what one node did during a run.
type AutomationNodeResult struct {
	NodeID string `json:"node_id"`
	// Type is [NodeTrigger], [NodeCondition] or [NodeAction].
	Type   string `json:"type"`
	Action string `json:"action,omitempty"`
	Label  string `json:"label,omitempty"`
	// Status is one of the NodeStatus* constants.
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	// Preview is a redacted sample of what the node produced.
	Preview json.RawMessage `json:"preview,omitempty"`
}

// AutomationTestResult is a dry run: the node-by-node trace plus the event data
// it ran against.
type AutomationTestResult struct {
	Trace []AutomationNodeResult `json:"trace"`
	Data  json.RawMessage        `json:"data"`
}

// AutomationCreateParams creates an automation.
type AutomationCreateParams struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	// TriggerEvent is the event key that starts it.
	TriggerEvent string          `json:"trigger_event"`
	Filter       json.RawMessage `json:"filter,omitempty"`
	Graph        AutomationGraph `json:"graph"`
}

// AutomationUpdateParams replaces an automation's definition. Unlike most
// update params these are not merged: the API rewrites the name, enabled flag,
// trigger, filter and node graph from what you send, so send the complete
// desired state. Read the current one with [AutomationService.Get] first if you
// only mean to change part of it.
type AutomationUpdateParams = AutomationCreateParams

// NodePosition is one node's coordinates on the builder canvas.
type NodePosition struct {
	ID string  `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
}

// List returns the workspace's automations.
func (s *AutomationService) List(ctx context.Context, opts ...RequestOption) ([]Automation, *Response, error) {
	var out struct {
		Automations []Automation `json:"automations"`
	}
	resp, err := s.client.get(ctx, "automations", &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Automations, resp, nil
}

// Create creates an automation.
func (s *AutomationService) Create(ctx context.Context, params *AutomationCreateParams, opts ...RequestOption) (*Automation, *Response, error) {
	return s.oneAutomation(ctx, s.client.post, "automations", params, opts)
}

// Get retrieves a single automation, including its graph.
func (s *AutomationService) Get(ctx context.Context, id string, opts ...RequestOption) (*Automation, *Response, error) {
	var out struct {
		Automation *Automation `json:"automation"`
	}
	resp, err := s.client.get(ctx, "automations/"+url.PathEscape(id), &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Automation, resp, nil
}

// Update replaces an automation's definition. See [AutomationUpdateParams].
func (s *AutomationService) Update(ctx context.Context, id string, params *AutomationUpdateParams, opts ...RequestOption) (*Automation, *Response, error) {
	return s.oneAutomation(ctx, s.client.patch, "automations/"+url.PathEscape(id), params, opts)
}

func (s *AutomationService) oneAutomation(ctx context.Context, verb bodyVerb, path string, body any, opts []RequestOption) (*Automation, *Response, error) {
	var out struct {
		Automation *Automation `json:"automation"`
	}
	resp, err := verb(ctx, path, body, &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Automation, resp, nil
}

// UpdateLayout persists node coordinates only. It is cosmetic: unaudited,
// last-write-wins, and safe to call continuously as nodes are dragged.
func (s *AutomationService) UpdateLayout(ctx context.Context, id string, positions []NodePosition, opts ...RequestOption) (*Response, error) {
	body := struct {
		Positions []NodePosition `json:"positions"`
	}{Positions: positions}
	return s.client.patch(ctx, "automations/"+url.PathEscape(id)+"/layout", body, nil, opts...)
}

// Delete removes an automation.
func (s *AutomationService) Delete(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "automations/"+url.PathEscape(id), opts...)
}

// Test dry-runs an automation against sample event data and returns the trace.
// Action nodes are not actually executed against the provider.
func (s *AutomationService) Test(ctx context.Context, id string, data any, opts ...RequestOption) (*AutomationTestResult, *Response, error) {
	body := struct {
		Data any `json:"data,omitempty"`
	}{Data: data}
	return send[AutomationTestResult](ctx, s.client.post, "automations/"+url.PathEscape(id)+"/test", body, opts)
}

// Runs returns an automation's recent runs, newest first. A limit of 0 uses the
// server default of 50.
func (s *AutomationService) Runs(ctx context.Context, id string, limit int, opts ...RequestOption) ([]AutomationRun, *Response, error) {
	q := make(url.Values)
	setPositive(q, "limit", limit)
	var out struct {
		Runs []AutomationRun `json:"runs"`
	}
	resp, err := s.client.get(ctx, withQuery("automations/"+url.PathEscape(id)+"/runs", q), &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Runs, resp, nil
}
