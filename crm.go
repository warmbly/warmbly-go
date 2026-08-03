package warmbly

import (
	"context"
	"net/url"
	"time"
)

// CRMService manages the built-in CRM: deal pipelines and their stages, deals,
// and the task board that hangs off contacts and deals.
//
// The deal and task lists come in two shapes. The plain List methods are cursor
// pages for straightforward browsing; the Search methods take a faceted filter
// body and are aggregated server-side, so a total or a per-stage sum is a true
// count over the whole matching set rather than a reduce over one page.
type CRMService service

// Pipeline is a named deal pipeline: an ordered set of stages deals move
// through.
type Pipeline struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	Position       int    `json:"position"`
	// Stages is populated on reads that hydrate the pipeline.
	Stages    []PipelineStage `json:"stages,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// PipelineStage is one column of a pipeline.
type PipelineStage struct {
	ID         string `json:"id"`
	PipelineID string `json:"pipeline_id"`
	Name       string `json:"name"`
	Color      string `json:"color"`
	Position   int    `json:"position"`
	// DealCount is set on reads that count the stage's deals.
	DealCount int       `json:"deal_count,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PipelineCreateParams creates a pipeline, optionally with its initial stages.
type PipelineCreateParams struct {
	Name   string              `json:"name"`
	Stages []StageCreateParams `json:"stages,omitempty"`
}

// StageCreateParams creates one pipeline stage.
type StageCreateParams struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// PipelineUpdateParams renames a pipeline.
type PipelineUpdateParams struct {
	Name *string `json:"name,omitempty"`
}

// StageUpdateParams updates a stage. Nil fields are left unchanged.
type StageUpdateParams struct {
	Name  *string `json:"name,omitempty"`
	Color *string `json:"color,omitempty"`
}

// Deal states returned in [Deal.Status].
const (
	DealStatusOpen = "open"
	DealStatusWon  = "won"
	DealStatusLost = "lost"
)

// Deal is a sales opportunity moving through a pipeline.
type Deal struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organization_id"`
	PipelineID     string  `json:"pipeline_id"`
	StageID        string  `json:"stage_id"`
	ContactID      *string `json:"contact_id,omitempty"`
	Name           string  `json:"name"`
	// Value is the deal's amount in Currency.
	Value    *float64 `json:"value,omitempty"`
	Currency string   `json:"currency"`
	// Status is [DealStatusOpen], [DealStatusWon] or [DealStatusLost].
	Status            string     `json:"status"`
	ExpectedCloseDate *time.Time `json:"expected_close_date,omitempty"`
	WonAt             *time.Time `json:"won_at,omitempty"`
	LostAt            *time.Time `json:"lost_at,omitempty"`
	LostReason        *string    `json:"lost_reason,omitempty"`
	AssignedTo        *string    `json:"assigned_to,omitempty"`

	// CampaignID and SourceMailboxID attribute the deal to the outreach that
	// produced it. They are a best guess and are editable.
	CampaignID      *string `json:"campaign_id,omitempty"`
	SourceMailboxID *string `json:"source_mailbox_id,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Contact, Stage and CampaignName are joined in by the list and search
	// endpoints, not by single-deal reads.
	Contact      *Contact       `json:"contact,omitempty"`
	Stage        *PipelineStage `json:"stage,omitempty"`
	CampaignName *string        `json:"campaign_name,omitempty"`
}

// DealCreateParams creates a deal. PipelineID, StageID and Name are required.
type DealCreateParams struct {
	PipelineID        string     `json:"pipeline_id"`
	StageID           string     `json:"stage_id"`
	ContactID         *string    `json:"contact_id,omitempty"`
	Name              string     `json:"name"`
	Value             *float64   `json:"value,omitempty"`
	Currency          string     `json:"currency,omitempty"`
	ExpectedCloseDate *time.Time `json:"expected_close_date,omitempty"`
	AssignedTo        *string    `json:"assigned_to,omitempty"`
	CampaignID        *string    `json:"campaign_id,omitempty"`
	SourceMailboxID   *string    `json:"source_mailbox_id,omitempty"`
}

// DealUpdateParams updates a deal. Nil fields are left unchanged. Setting
// Status to [DealStatusWon] or [DealStatusLost] closes it.
type DealUpdateParams struct {
	StageID           *string    `json:"stage_id,omitempty"`
	ContactID         *string    `json:"contact_id,omitempty"`
	Name              *string    `json:"name,omitempty"`
	Value             *float64   `json:"value,omitempty"`
	Currency          *string    `json:"currency,omitempty"`
	Status            *string    `json:"status,omitempty"`
	ExpectedCloseDate *time.Time `json:"expected_close_date,omitempty"`
	LostReason        *string    `json:"lost_reason,omitempty"`
	AssignedTo        *string    `json:"assigned_to,omitempty"`
}

// DealSearchParams is the faceted filter shared by [CRMService.SearchDeals] and
// [CRMService.DealsSummary]. Every facet is optional; an empty body matches
// every deal in the organization. Slice facets match any of their values.
type DealSearchParams struct {
	ListOptions

	// Query matches the deal name.
	Query string `json:"query,omitempty"`
	// Statuses matches any of [DealStatusOpen], [DealStatusWon] or
	// [DealStatusLost].
	Statuses    []string `json:"statuses,omitempty"`
	PipelineIDs []string `json:"pipeline_ids,omitempty"`
	StageIDs    []string `json:"stage_ids,omitempty"`
	// AssignedTo matches any of the given owner user ids.
	AssignedTo  []string `json:"assigned_to,omitempty"`
	CampaignIDs []string `json:"campaign_ids,omitempty"`

	MinValue      *float64   `json:"min_value,omitempty"`
	MaxValue      *float64   `json:"max_value,omitempty"`
	CloseAfter    *time.Time `json:"close_after,omitempty"`
	CloseBefore   *time.Time `json:"close_before,omitempty"`
	CreatedAfter  *time.Time `json:"created_after,omitempty"`
	CreatedBefore *time.Time `json:"created_before,omitempty"`

	// SortBy is "created_at", "updated_at", "value", "expected_close_date" or
	// "name". Reverse switches to ascending; the default is descending.
	SortBy  string `json:"sort_by,omitempty"`
	Reverse bool   `json:"reverse,omitempty"`
}

// DealsSummary aggregates every deal matching a [DealSearchParams], so a
// header total or a kanban column sum is exact rather than a page reduce.
type DealsSummary struct {
	Total     int64              `json:"total"`
	OpenCount int64              `json:"open_count"`
	OpenValue float64            `json:"open_value"`
	WonCount  int64              `json:"won_count"`
	WonValue  float64            `json:"won_value"`
	LostCount int64              `json:"lost_count"`
	LostValue float64            `json:"lost_value"`
	Currency  string             `json:"currency"`
	Stages    []DealStageSummary `json:"stages"`
	// MixedCurrency is true when the matching deals span several currencies,
	// in which case the value sums are not directly comparable.
	MixedCurrency bool `json:"mixed_currency"`
}

// DealStageSummary is one pipeline column's count and open value.
type DealStageSummary struct {
	StageID string  `json:"stage_id"`
	Count   int64   `json:"count"`
	Value   float64 `json:"value"`
}

// CRM task priorities returned in [CRMTask.Priority].
const (
	TaskPriorityLow    = "low"
	TaskPriorityMedium = "medium"
	TaskPriorityHigh   = "high"
	TaskPriorityUrgent = "urgent"
)

// CRM task states returned in [CRMTask.Status].
const (
	TaskStatusPending    = "pending"
	TaskStatusInProgress = "in_progress"
	TaskStatusCompleted  = "completed"
	TaskStatusCancelled  = "cancelled"
)

// CRMTaskType is a user-defined kind of work, for example Call or Meeting.
// Tasks reference their type by name, so deleting a type never orphans the
// tasks that used it; they keep the label and fall back to a neutral color.
type CRMTaskType struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	Color          string    `json:"color"`
	Position       int       `json:"position"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// TaskTypeCreateParams creates a task type.
type TaskTypeCreateParams struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// TaskTypeUpdateParams updates a task type. Nil fields are left unchanged.
type TaskTypeUpdateParams struct {
	Name     *string `json:"name,omitempty"`
	Color    *string `json:"color,omitempty"`
	Position *int    `json:"position,omitempty"`
}

// CRMTask is a unit of follow-up work, optionally attached to a contact or a
// deal and assigned to a member or a whole team.
type CRMTask struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organization_id"`
	ContactID      *string `json:"contact_id,omitempty"`
	DealID         *string `json:"deal_id,omitempty"`
	AssignedTo     *string `json:"assigned_to,omitempty"`
	AssignedTeamID *string `json:"assigned_team_id,omitempty"`
	CreatedBy      string  `json:"created_by"`

	Title       string     `json:"title"`
	Description *string    `json:"description,omitempty"`
	DueDate     *time.Time `json:"due_date,omitempty"`
	// Priority is one of the TaskPriority* constants.
	Priority string `json:"priority"`
	// Type is a [CRMTaskType] name.
	Type string `json:"type"`
	// Status is one of the TaskStatus* constants.
	Status      string     `json:"status"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CRMTaskCreateParams creates a task. Title is required.
type CRMTaskCreateParams struct {
	ContactID      *string    `json:"contact_id,omitempty"`
	DealID         *string    `json:"deal_id,omitempty"`
	AssignedTo     *string    `json:"assigned_to,omitempty"`
	AssignedTeamID *string    `json:"assigned_team_id,omitempty"`
	Title          string     `json:"title"`
	Description    *string    `json:"description,omitempty"`
	DueDate        *time.Time `json:"due_date,omitempty"`
	Priority       string     `json:"priority,omitempty"`
	Type           string     `json:"type,omitempty"`
}

// CRMTaskUpdateParams updates a task. Nil fields are left unchanged.
type CRMTaskUpdateParams struct {
	AssignedTo     *string    `json:"assigned_to,omitempty"`
	AssignedTeamID *string    `json:"assigned_team_id,omitempty"`
	Title          *string    `json:"title,omitempty"`
	Description    *string    `json:"description,omitempty"`
	DueDate        *time.Time `json:"due_date,omitempty"`
	Priority       *string    `json:"priority,omitempty"`
	Type           *string    `json:"type,omitempty"`
	Status         *string    `json:"status,omitempty"`
}

// TaskSearchParams is the faceted filter shared by [CRMService.SearchTasks] and
// [CRMService.TasksSummary]. Every facet is optional; slice facets match any of
// their values.
type TaskSearchParams struct {
	ListOptions

	// Query matches the task title.
	Query string `json:"query,omitempty"`
	// Statuses matches any of the TaskStatus* constants, Priorities any of the
	// TaskPriority* constants, and Types any [CRMTaskType] name.
	Statuses   []string `json:"statuses,omitempty"`
	Priorities []string `json:"priorities,omitempty"`
	Types      []string `json:"types,omitempty"`
	// AssignedTo matches any of the given assignee user ids. TeamIDs matches
	// tasks assigned to any of those teams, or whose assignee belongs to one.
	AssignedTo []string `json:"assigned_to,omitempty"`
	TeamIDs    []string `json:"team_ids,omitempty"`

	ContactID *string    `json:"contact_id,omitempty"`
	DealID    *string    `json:"deal_id,omitempty"`
	DueAfter  *time.Time `json:"due_after,omitempty"`
	DueBefore *time.Time `json:"due_before,omitempty"`
	// Overdue matches tasks past their due date that are neither completed nor
	// canceled.
	Overdue bool `json:"overdue,omitempty"`

	// SortBy is "created_at", "due_date", "priority", "title" or "updated_at".
	// Reverse switches to ascending; the default is descending.
	SortBy  string `json:"sort_by,omitempty"`
	Reverse bool   `json:"reverse,omitempty"`
}

// TasksSummary aggregates every task matching a [TaskSearchParams].
type TasksSummary struct {
	Total          int64 `json:"total"`
	PendingCount   int64 `json:"pending_count"`
	InProgress     int64 `json:"in_progress_count"`
	CompletedCount int64 `json:"completed_count"`
	CancelledCount int64 `json:"cancelled_count"`
	OverdueCount   int64 `json:"overdue_count"`
	HighPriority   int64 `json:"high_priority_count"`
}

// --- pipelines ---

// ListPipelines returns the organization's pipelines with their stages.
func (s *CRMService) ListPipelines(ctx context.Context, opts ...RequestOption) ([]Pipeline, *Response, error) {
	return fetchData[Pipeline](ctx, s.client, "crm/pipelines", opts)
}

// CreatePipeline creates a pipeline, optionally seeding its stages.
func (s *CRMService) CreatePipeline(ctx context.Context, params *PipelineCreateParams, opts ...RequestOption) (*Pipeline, *Response, error) {
	return send[Pipeline](ctx, s.client, s.client.post, "crm/pipelines", params, opts)
}

// GetPipeline retrieves a pipeline and its stages.
func (s *CRMService) GetPipeline(ctx context.Context, id string, opts ...RequestOption) (*Pipeline, *Response, error) {
	return fetch[Pipeline](ctx, s.client, "crm/pipelines/"+url.PathEscape(id), opts)
}

// UpdatePipeline renames a pipeline.
func (s *CRMService) UpdatePipeline(ctx context.Context, id string, params *PipelineUpdateParams, opts ...RequestOption) (*Pipeline, *Response, error) {
	return send[Pipeline](ctx, s.client, s.client.patch, "crm/pipelines/"+url.PathEscape(id), params, opts)
}

// DeletePipeline removes a pipeline.
func (s *CRMService) DeletePipeline(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "crm/pipelines/"+url.PathEscape(id), opts...)
}

// CreateStage appends a stage to a pipeline.
func (s *CRMService) CreateStage(ctx context.Context, pipelineID string, params *StageCreateParams, opts ...RequestOption) (*PipelineStage, *Response, error) {
	return send[PipelineStage](ctx, s.client, s.client.post, "crm/pipelines/"+url.PathEscape(pipelineID)+"/stages", params, opts)
}

// UpdateStage modifies a pipeline stage.
func (s *CRMService) UpdateStage(ctx context.Context, pipelineID, stageID string, params *StageUpdateParams, opts ...RequestOption) (*PipelineStage, *Response, error) {
	return send[PipelineStage](ctx, s.client, s.client.patch, "crm/pipelines/"+url.PathEscape(pipelineID)+"/stages/"+url.PathEscape(stageID), params, opts)
}

// DeleteStage removes a pipeline stage.
func (s *CRMService) DeleteStage(ctx context.Context, pipelineID, stageID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "crm/pipelines/"+url.PathEscape(pipelineID)+"/stages/"+url.PathEscape(stageID), opts...)
}

// --- deals ---

// ListDeals returns a page of deals.
func (s *CRMService) ListDeals(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[Deal], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[Deal](ctx, s.client, "crm/deals", q, opts...)
}

// SearchDeals returns a page of deals matching a faceted filter.
func (s *CRMService) SearchDeals(ctx context.Context, params *DealSearchParams, opts ...RequestOption) (*Page[Deal], error) {
	if params == nil {
		params = &DealSearchParams{}
	}
	q := make(url.Values)
	params.apply(q)
	return listPostJSON[Deal](ctx, s.client, "crm/deals/search", q, params, opts...)
}

// DealsSummary aggregates every deal matching the same filter a search takes.
func (s *CRMService) DealsSummary(ctx context.Context, params *DealSearchParams, opts ...RequestOption) (*DealsSummary, *Response, error) {
	return send[DealsSummary](ctx, s.client, s.client.post, "crm/deals/summary", params, opts)
}

// CreateDeal opens a new deal.
func (s *CRMService) CreateDeal(ctx context.Context, params *DealCreateParams, opts ...RequestOption) (*Deal, *Response, error) {
	return send[Deal](ctx, s.client, s.client.post, "crm/deals", params, opts)
}

// GetDeal retrieves a single deal.
func (s *CRMService) GetDeal(ctx context.Context, id string, opts ...RequestOption) (*Deal, *Response, error) {
	return fetch[Deal](ctx, s.client, "crm/deals/"+url.PathEscape(id), opts)
}

// UpdateDeal modifies a deal, including moving it between stages or closing it.
func (s *CRMService) UpdateDeal(ctx context.Context, id string, params *DealUpdateParams, opts ...RequestOption) (*Deal, *Response, error) {
	return send[Deal](ctx, s.client, s.client.patch, "crm/deals/"+url.PathEscape(id), params, opts)
}

// DeleteDeal removes a deal.
func (s *CRMService) DeleteDeal(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "crm/deals/"+url.PathEscape(id), opts...)
}

// --- task types ---

// ListTaskTypes returns the organization's task types. The first read seeds a
// usable default set.
func (s *CRMService) ListTaskTypes(ctx context.Context, opts ...RequestOption) ([]CRMTaskType, *Response, error) {
	return fetchData[CRMTaskType](ctx, s.client, "crm/task-types", opts)
}

// CreateTaskType adds a task type.
func (s *CRMService) CreateTaskType(ctx context.Context, params *TaskTypeCreateParams, opts ...RequestOption) (*CRMTaskType, *Response, error) {
	return send[CRMTaskType](ctx, s.client, s.client.post, "crm/task-types", params, opts)
}

// UpdateTaskType modifies a task type.
func (s *CRMService) UpdateTaskType(ctx context.Context, id string, params *TaskTypeUpdateParams, opts ...RequestOption) (*CRMTaskType, *Response, error) {
	return send[CRMTaskType](ctx, s.client, s.client.patch, "crm/task-types/"+url.PathEscape(id), params, opts)
}

// DeleteTaskType removes a task type. Tasks that used it keep its name.
func (s *CRMService) DeleteTaskType(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "crm/task-types/"+url.PathEscape(id), opts...)
}

// --- tasks ---

// ListTasks returns a page of CRM tasks.
func (s *CRMService) ListTasks(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[CRMTask], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[CRMTask](ctx, s.client, "crm/tasks", q, opts...)
}

// SearchTasks returns a page of tasks matching a faceted filter.
func (s *CRMService) SearchTasks(ctx context.Context, params *TaskSearchParams, opts ...RequestOption) (*Page[CRMTask], error) {
	if params == nil {
		params = &TaskSearchParams{}
	}
	q := make(url.Values)
	params.apply(q)
	return listPostJSON[CRMTask](ctx, s.client, "crm/tasks/search", q, params, opts...)
}

// TasksSummary aggregates every task matching the same filter a search takes.
func (s *CRMService) TasksSummary(ctx context.Context, params *TaskSearchParams, opts ...RequestOption) (*TasksSummary, *Response, error) {
	return send[TasksSummary](ctx, s.client, s.client.post, "crm/tasks/summary", params, opts)
}

// CreateTask opens a new CRM task.
func (s *CRMService) CreateTask(ctx context.Context, params *CRMTaskCreateParams, opts ...RequestOption) (*CRMTask, *Response, error) {
	return send[CRMTask](ctx, s.client, s.client.post, "crm/tasks", params, opts)
}

// GetTask retrieves a single CRM task.
func (s *CRMService) GetTask(ctx context.Context, id string, opts ...RequestOption) (*CRMTask, *Response, error) {
	return fetch[CRMTask](ctx, s.client, "crm/tasks/"+url.PathEscape(id), opts)
}

// UpdateTask modifies a CRM task.
func (s *CRMService) UpdateTask(ctx context.Context, id string, params *CRMTaskUpdateParams, opts ...RequestOption) (*CRMTask, *Response, error) {
	return send[CRMTask](ctx, s.client, s.client.patch, "crm/tasks/"+url.PathEscape(id), params, opts)
}

// DeleteTask removes a CRM task.
func (s *CRMService) DeleteTask(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "crm/tasks/"+url.PathEscape(id), opts...)
}
