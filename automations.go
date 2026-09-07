package warmbly

import (
	"context"
	"encoding/json"
	"net/url"
	"time"
)

// AutomationService manages automations: a trigger event plus a graph of
// condition and action nodes that runs across connected integrations and the
// built-in Warmbly actions.
//
// An automation can also be triggered from outside Warmbly. Each one whose
// trigger is [TriggerInboundWebhook] gets a token-addressed inbound URL
// ([Automation.InboundURL]) that runs it with the POSTed JSON body as the
// event payload.
//
// Chains are bounded. Everything an action does (a contact it creates, a deal
// it opens, an automation it launches) fires events one hop deeper, and past
// [MaxAutomationDepth] hops those events still reach webhooks but run no more
// automations, so "on contact created, create a contact" cannot loop.
type AutomationService service

// MaxAutomationDepth is how many automations may run in a chain before the
// server stops following it.
const MaxAutomationDepth = 5

// Trigger events accepted in [Automation.TriggerEvent]. Any Event* constant
// from the webhook catalog can be a trigger; these are the ones with
// dedicated semantics or no webhook twin.
const (
	// TriggerInboundWebhook runs the automation when an external system POSTs
	// JSON to its [Automation.InboundURL]. It is a trigger only and is never
	// delivered outbound.
	TriggerInboundWebhook = "inbound.webhook"
	// TriggerContactCreated fires for every new contact, whatever created it
	// (import, form, API, another automation). The event carries the contact
	// fields plus source, source_detail, custom_fields, campaign_ids and
	// category_ids, and no campaign.
	TriggerContactCreated = "contact.created"
	// TriggerFormSubmitted fires when a hosted form is submitted. The event
	// carries form_id, form_name, submission_id, source_url and the raw
	// answers under data, alongside the contact fields the answers mapped to.
	TriggerFormSubmitted = "form.submitted"
	// TriggerReplyReceived fires on an inbound campaign reply, with intent
	// and confidence from the classifier.
	TriggerReplyReceived = "campaign.reply_received"
)

// Action identifiers for [AutomationNode.Action]. Provider actions run against
// the node's ConnectionID; the built-in "warmbly." actions need no connection
// and run against the contact the event resolved (contact_id or contact_email
// in the event data).
const (
	ActionSlackNotify      = "slack.notify"
	ActionDiscordNotify    = "discord.notify"
	ActionHubSpotUpsert    = "hubspot.upsert_contact"
	ActionPipedriveUpsert  = "pipedrive.upsert_person"
	ActionSalesforceUpsert = "salesforce.upsert_contact"
	ActionCloseUpsert      = "close.upsert_lead"
	ActionWebhookPing      = "webhook.ping"

	ActionAddTag        = "warmbly.add_tag"
	ActionRemoveTag     = "warmbly.remove_tag"
	ActionCreateTask    = "warmbly.create_task"
	ActionCreateDeal    = "warmbly.create_deal"
	ActionMoveDealStage = "warmbly.move_deal_stage"
	ActionUnsubscribe   = "warmbly.unsubscribe"
	// ActionLabelEmail applies conversation labels to the thread a reply
	// event belongs to; on triggers without a thread it is a logged no-op.
	ActionLabelEmail = "warmbly.label_email"
	// ActionRunAutomation launches another automation with the current event
	// data, one hop deeper in the chain.
	ActionRunAutomation = "warmbly.run_automation"
	// ActionSetVariables computes named values from templates and writes them
	// into the event data for later nodes.
	ActionSetVariables = "warmbly.set_variables"
	// ActionFireEvent publishes a custom event to the realtime gateway.
	ActionFireEvent = "warmbly.fire_event"
	// ActionUpsertContact creates a contact from templated event fields, or
	// enriches the one already holding that email, then tags it and enrolls
	// it in a campaign. It is the one action that can run before the event has
	// a contact: the written contact becomes the event's contact for every
	// node after it. Configure it with [UpsertContactConfig].
	ActionUpsertContact = "warmbly.upsert_contact"
	// ActionAddToCampaign enrolls the event's contact in a campaign and wakes
	// it. Configure it with [AddToCampaignConfig].
	ActionAddToCampaign = "warmbly.add_to_campaign"
	// ActionAIStep is the unified AI node (classify, extract, generate or a
	// bounded tool-using agent); ActionAISwitch routes to exactly one named
	// case. Both spend AI credits, including on a dry run.
	ActionAIStep   = "warmbly.ai_step"
	ActionAISwitch = "warmbly.ai_switch"
)

// Policies accepted in [UpsertContactConfig.IfExists].
const (
	// IfExistsUpdate enriches the existing contact: rendered fields that are
	// empty never erase what the contact already has. The default.
	IfExistsUpdate = "update"
	// IfExistsSkip leaves the existing contact untouched but still makes it
	// the event's contact for the nodes that follow.
	IfExistsSkip = "skip"
)

// TemplateField is one templated key/value pair in an action config. Value is
// a Go template rendered against the event data, so "{{.data.company}}" reads
// a form answer and "{{.first_name}}" a contact field.
type TemplateField struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// UpsertContactConfig is the config of an [ActionUpsertContact] node. Every
// contact field is a Go template rendered against the event data. Email is
// required and is validated when the automation is saved; a rendered email
// that is empty or has no "@" fails the node at run time.
type UpsertContactConfig struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	Company   string `json:"company,omitempty"`
	Phone     string `json:"phone,omitempty"`
	// CustomFields are written into the contact's custom fields. A field
	// whose value renders empty is left out rather than cleared.
	CustomFields []TemplateField `json:"custom_fields,omitempty"`
	// CategoryIDs tags the contact and SegmentIDs pins it into segments. Ids
	// are validated on save.
	CategoryIDs []string `json:"category_ids,omitempty"`
	SegmentIDs  []string `json:"segment_ids,omitempty"`
	// CampaignID, when set, enrolls the contact in that campaign.
	CampaignID string `json:"campaign_id,omitempty"`
	// IfExists is [IfExistsUpdate] (the default) or [IfExistsSkip].
	IfExists string `json:"if_exists,omitempty"`
}

// Config renders the struct as an [AutomationNode.Config] value.
func (c *UpsertContactConfig) Config() (json.RawMessage, error) {
	return json.Marshal(c)
}

// AddToCampaignConfig is the config of an [ActionAddToCampaign] node.
// CampaignID is required and validated on save.
type AddToCampaignConfig struct {
	CampaignID string `json:"campaign_id"`
}

// Config renders the struct as an [AutomationNode.Config] value.
func (c *AddToCampaignConfig) Config() (json.RawMessage, error) {
	return json.Marshal(c)
}

// Node kinds in an automation graph.
const (
	// NodeTrigger is the entry point; there is exactly one, conventionally
	// with the id "trigger".
	NodeTrigger = "trigger"
	// NodeCondition branches on the event data.
	NodeCondition = "condition"
	// NodeAction performs work, against a connection or built in.
	NodeAction = "action"
	// NodeStop is a terminal marker: a path routed into it ends. It carries
	// no action or condition.
	NodeStop = "stop"
)

// Condition kinds accepted in [AutomationCondition.Field].
const (
	// ConditionField tests one event-data key, named by Key, with Operator
	// and Value.
	ConditionField = "field"
	// ConditionExpression evaluates Expression, a Go-template predicate, and
	// takes the true branch when it renders non-empty and not "false".
	ConditionExpression = "expression"
	// ConditionAI asks the model the yes/no question in Prompt about the
	// event. It costs one AI credit per evaluation.
	ConditionAI = "ai"
	// ConditionRandom is a deterministic percentage split; use
	// [ConditionOpChance] with Value as the percentage.
	ConditionRandom = "random"

	// The four below are semantic shortcuts kept valid for automations saved
	// before [ConditionField] could test any key. They read a fixed key of the
	// event data, so Key is ignored.

	// ConditionIntent tests the reply classifier's intent.
	ConditionIntent = "intent"
	// ConditionConfidence tests the classifier's confidence, a float.
	ConditionConfidence = "confidence"
	// ConditionSource tests the campaign or provider the event came from.
	ConditionSource = "source"
	// ConditionHasContact tests whether the event resolved a contact email.
	ConditionHasContact = "has_contact"
)

// Operators accepted in [AutomationCondition.Operator].
const (
	ConditionOpEquals    = "equals"
	ConditionOpNotEquals = "not_equals"
	ConditionOpContains  = "contains"
	ConditionOpGte       = "gte"
	ConditionOpLte       = "lte"
	ConditionOpExists    = "exists"
	ConditionOpIsTrue    = "is_true"
	ConditionOpChance    = "chance"
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
	// TriggerEvent is the event key that starts the run: a Trigger* constant
	// or any Event* constant.
	TriggerEvent string `json:"trigger_event"`
	// Filter narrows which occurrences of the trigger event actually run it.
	Filter json.RawMessage `json:"filter,omitempty"`
	Graph  AutomationGraph `json:"graph"`
	// InboundURL is the public POST URL that fires the automation, set only
	// when TriggerEvent is [TriggerInboundWebhook]. It embeds a per-automation
	// secret, so treat it as a credential.
	InboundURL string `json:"inbound_url,omitempty"`

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
	// Type is [NodeTrigger], [NodeCondition], [NodeAction] or [NodeStop].
	Type string `json:"type"`
	// Action is the action identifier for an action node; see the Action*
	// constants.
	Action string `json:"action,omitempty"`
	// ConnectionID is the integration a provider action runs against. The
	// built-in "warmbly." actions leave it nil.
	ConnectionID *string `json:"connection_id,omitempty"`
	// Config is the action's parameters. Its shape depends on Action; the
	// built-in [UpsertContactConfig] and [AddToCampaignConfig] render into it,
	// and the server validates it when the automation is saved (400).
	Config json.RawMessage `json:"config,omitempty"`
	// Condition is the test a condition node applies.
	Condition *AutomationCondition `json:"condition,omitempty"`
	// X and Y are the node's canvas coordinates.
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// AutomationCondition is a condition node's test. Field picks the kind of
// test; the other fields apply according to the Condition* constant.
type AutomationCondition struct {
	// Field is one of the Condition* constants.
	Field string `json:"field"`
	// Key names the event-data key to read for [ConditionField].
	Key string `json:"key,omitempty"`
	// Operator is one of the ConditionOp* constants.
	Operator string `json:"operator"`
	Value    any    `json:"value,omitempty"`
	// Expression is the Go-template predicate for [ConditionExpression].
	Expression string `json:"expression,omitempty"`
	// Prompt is the yes/no question for [ConditionAI].
	Prompt string `json:"prompt,omitempty"`
}

// Branch labels for [AutomationEdge.When]. A condition node's two outgoing
// edges must be [EdgeWhenTrue] and [EdgeWhenFalse]. An action node takes a
// plain edge (empty When) plus an optional [EdgeWhenError] branch, and an
// [ActionAISwitch] node may additionally carry one [EdgeCasePrefix] edge per
// configured case. Anything else is rejected when the automation is saved.
const (
	EdgeWhenTrue  = "true"
	EdgeWhenFalse = "false"
	// EdgeWhenError is the path taken when the source action fails.
	EdgeWhenError = "error"
	// EdgeCasePrefix prefixes an AI switch case name, as in "label:interested".
	EdgeCasePrefix = "label:"
)

// AutomationEdge connects two nodes. When is the branch label; see the
// EdgeWhen* constants. The graph may not contain a cycle: a flow that loops
// back on itself is a 400 when saved.
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

// AutomationNodeResult is what one node did during a run or a dry run.
type AutomationNodeResult struct {
	NodeID string `json:"node_id"`
	// Type is [NodeTrigger], [NodeCondition] or [NodeAction].
	Type   string `json:"type"`
	Action string `json:"action,omitempty"`
	Label  string `json:"label,omitempty"`
	// Status is one of the NodeStatus* constants.
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	// Preview is a JSON object of what the action sent or would send: the
	// rendered templates of a dry run, plus run-time output such as the
	// contact_id and contact_created of an [ActionUpsertContact] node.
	Preview json.RawMessage `json:"preview,omitempty"`
}

// AutomationTestResult is a dry run: the node-by-node trace plus the event data
// it ran against, including anything the nodes wrote into it.
type AutomationTestResult struct {
	Trace []AutomationNodeResult `json:"trace"`
	Data  json.RawMessage        `json:"data"`
}

// AutomationTestParams configures a dry run.
type AutomationTestParams struct {
	// Data is the sample event payload. Leave it nil to let the server build
	// one from the trigger.
	Data any `json:"data,omitempty"`
	// SkipNodeIDs are action nodes to leave out of this test. They are
	// recorded as [NodeStatusSkipped] and never previewed, but the walk still
	// follows their edges so later steps are shown.
	SkipNodeIDs []string `json:"skip_node_ids,omitempty"`
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

// Create creates an automation. Node configs are validated, so a built-in
// action missing its required field fails here with a 400.
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
// last-write-wins, and safe to call continuously as nodes are dragged. At most
// 1,000 positions are accepted per call.
func (s *AutomationService) UpdateLayout(ctx context.Context, id string, positions []NodePosition, opts ...RequestOption) (*Response, error) {
	body := struct {
		Positions []NodePosition `json:"positions"`
	}{Positions: positions}
	return s.client.patch(ctx, "automations/"+url.PathEscape(id)+"/layout", body, nil, opts...)
}

// Delete removes an automation. It fails with a 409 while a campaign step
// still runs it; remove the step first.
func (s *AutomationService) Delete(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "automations/"+url.PathEscape(id), opts...)
}

// Test dry-runs an automation against sample event data and returns the
// trace. Pass nil data to use the server's sample for the trigger. It is
// [AutomationService.DryRun] without the option to skip nodes.
func (s *AutomationService) Test(ctx context.Context, id string, data any, opts ...RequestOption) (*AutomationTestResult, *Response, error) {
	return s.DryRun(ctx, id, &AutomationTestParams{Data: data}, opts...)
}

// DryRun tests an automation without side effects. Conditions evaluate for
// real and action nodes are previewed rather than executed, with one
// exception: AI nodes run for real, and are charged, so the trace shows the
// model's actual output.
func (s *AutomationService) DryRun(ctx context.Context, id string, params *AutomationTestParams, opts ...RequestOption) (*AutomationTestResult, *Response, error) {
	if params == nil {
		params = &AutomationTestParams{}
	}
	return send[AutomationTestResult](ctx, s.client.post, "automations/"+url.PathEscape(id)+"/test", params, opts)
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
