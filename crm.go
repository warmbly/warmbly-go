package warmbly

import (
	"context"
	"net/http"
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
	// External is set when the pipeline is managed in a connected CRM.
	External *CRMExternalRef `json:"external,omitempty"`
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
	// Closed, Won and Probability come from a connected CRM's stage metadata.
	Closed      bool     `json:"closed,omitempty"`
	Won         bool     `json:"won,omitempty"`
	Probability *float64 `json:"probability,omitempty"`
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
	// External is set when the deal lives in a connected CRM.
	External *CRMExternalRef `json:"external,omitempty"`
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

// DealListParams filters and paginates the plain deal list. Every filter is
// optional and matches a single value; use [DealSearchParams] with
// [CRMService.SearchDeals] when you need several values per facet, a value
// range or an exact total.
type DealListParams struct {
	ListOptions
	// PipelineID and StageID narrow to one pipeline or one of its columns.
	PipelineID string
	StageID    string
	// Status is [DealStatusOpen], [DealStatusWon] or [DealStatusLost].
	Status string
}

func (p *DealListParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	setNonEmpty(q, "pipeline_id", p.PipelineID)
	setNonEmpty(q, "stage_id", p.StageID)
	setNonEmpty(q, "status", p.Status)
	return q
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
	TaskStatusCancelled  = "cancelled" //nolint:misspell // wire value: the API sends "cancelled" here
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
	// External is set when the task lives in a connected CRM.
	External *CRMExternalRef `json:"external,omitempty"`
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

// CRMTaskListParams filters and paginates the plain task list. Every filter is
// optional and matches a single value; use [TaskSearchParams] with
// [CRMService.SearchTasks] when you need several values per facet, due-date
// bounds or an exact total.
type CRMTaskListParams struct {
	ListOptions
	// ContactID and DealID narrow to the tasks hanging off one record.
	ContactID string
	DealID    string
	// AssignedTo is an assignee user id.
	AssignedTo string
	// Status is one of the TaskStatus* constants.
	Status string
}

func (p *CRMTaskListParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	setNonEmpty(q, "contact_id", p.ContactID)
	setNonEmpty(q, "deal_id", p.DealID)
	setNonEmpty(q, "assigned_to", p.AssignedTo)
	setNonEmpty(q, "status", p.Status)
	return q
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
	CancelledCount int64 `json:"cancelled_count"` //nolint:misspell // wire value: the API sends "cancelled" here
	OverdueCount   int64 `json:"overdue_count"`
	HighPriority   int64 `json:"high_priority_count"`
}

// TaskSelection names the tasks a bulk action applies to: either an explicit
// id list (Tasks), or every task matching a search (All with Filters) minus
// the rows unticked afterwards (Exclude). A selection that names both prefers
// the filter. A selection larger than [MaxTaskBulkSelection] is refused.
type TaskSelection struct {
	// Tasks are the ids to act on.
	Tasks []string `json:"tasks"`
	// All switches the selection from Tasks to Filters.
	All bool `json:"all,omitempty"`
	// Filters is the body [CRMService.SearchTasks] takes, so the set resolved
	// is exactly the set a list was showing. Its pagination fields are ignored.
	Filters *TaskSearchParams `json:"filters,omitempty"`
	// Exclude drops ids from the resolved set. Ignored unless All is set.
	Exclude []string `json:"exclude,omitempty"`
}

// MaxTaskBulkSelection bounds how many tasks one bulk action may touch.
const MaxTaskBulkSelection = 50000

// BulkTaskUpdateParams writes a status and/or priority onto every task in a
// selection. At least one of Status and Priority is required.
type BulkTaskUpdateParams struct {
	TaskSelection
	// Status is one of the TaskStatus* constants.
	Status *string `json:"status,omitempty"`
	// Priority is one of the TaskPriority* constants.
	Priority *string `json:"priority,omitempty"`
}

// BulkTasksResult is how many tasks (or, for the sync-job routes, jobs) a
// bulk action touched.
type BulkTasksResult struct {
	Affected int64 `json:"affected"`
}

// --- pipelines ---

// ListPipelines returns the organization's pipelines with their stages.
func (s *CRMService) ListPipelines(ctx context.Context, opts ...RequestOption) ([]Pipeline, *Response, error) {
	return fetchData[Pipeline](ctx, s.client, "crm/pipelines", opts)
}

// CreatePipeline creates a pipeline, optionally seeding its stages.
func (s *CRMService) CreatePipeline(ctx context.Context, params *PipelineCreateParams, opts ...RequestOption) (*Pipeline, *Response, error) {
	return send[Pipeline](ctx, s.client.post, "crm/pipelines", params, opts)
}

// GetPipeline retrieves a pipeline and its stages.
func (s *CRMService) GetPipeline(ctx context.Context, id string, opts ...RequestOption) (*Pipeline, *Response, error) {
	return fetch[Pipeline](ctx, s.client, "crm/pipelines/"+url.PathEscape(id), opts)
}

// UpdatePipeline renames a pipeline.
func (s *CRMService) UpdatePipeline(ctx context.Context, id string, params *PipelineUpdateParams, opts ...RequestOption) (*Pipeline, *Response, error) {
	return send[Pipeline](ctx, s.client.patch, "crm/pipelines/"+url.PathEscape(id), params, opts)
}

// DeletePipeline removes a pipeline.
func (s *CRMService) DeletePipeline(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "crm/pipelines/"+url.PathEscape(id), opts...)
}

// CreateStage appends a stage to a pipeline.
func (s *CRMService) CreateStage(ctx context.Context, pipelineID string, params *StageCreateParams, opts ...RequestOption) (*PipelineStage, *Response, error) {
	return send[PipelineStage](ctx, s.client.post, "crm/pipelines/"+url.PathEscape(pipelineID)+"/stages", params, opts)
}

// UpdateStage modifies a pipeline stage.
func (s *CRMService) UpdateStage(ctx context.Context, pipelineID, stageID string, params *StageUpdateParams, opts ...RequestOption) (*PipelineStage, *Response, error) {
	return send[PipelineStage](ctx, s.client.patch, "crm/pipelines/"+url.PathEscape(pipelineID)+"/stages/"+url.PathEscape(stageID), params, opts)
}

// DeleteStage removes a pipeline stage.
func (s *CRMService) DeleteStage(ctx context.Context, pipelineID, stageID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "crm/pipelines/"+url.PathEscape(pipelineID)+"/stages/"+url.PathEscape(stageID), opts...)
}

// --- deals ---

// ListDeals returns a page of deals, optionally narrowed to one pipeline,
// stage or status. Rows carry their joined contact and stage.
func (s *CRMService) ListDeals(ctx context.Context, params *DealListParams, opts ...RequestOption) (*Page[Deal], error) {
	return listJSON[Deal](ctx, s.client, "crm/deals", params.values(), opts...)
}

// SearchDeals returns a page of deals matching a faceted filter. Its page
// limit is capped at 200, twice what [CRMService.ListDeals] allows, and
// [Pagination.Total] is an exact count over the whole matching set.
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
	return send[DealsSummary](ctx, s.client.post, "crm/deals/summary", params, opts)
}

// CreateDeal opens a new deal.
func (s *CRMService) CreateDeal(ctx context.Context, params *DealCreateParams, opts ...RequestOption) (*Deal, *Response, error) {
	return send[Deal](ctx, s.client.post, "crm/deals", params, opts)
}

// GetDeal retrieves a single deal.
func (s *CRMService) GetDeal(ctx context.Context, id string, opts ...RequestOption) (*Deal, *Response, error) {
	return fetch[Deal](ctx, s.client, "crm/deals/"+url.PathEscape(id), opts)
}

// UpdateDeal modifies a deal, including moving it between stages or closing it.
func (s *CRMService) UpdateDeal(ctx context.Context, id string, params *DealUpdateParams, opts ...RequestOption) (*Deal, *Response, error) {
	return send[Deal](ctx, s.client.patch, "crm/deals/"+url.PathEscape(id), params, opts)
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
	return send[CRMTaskType](ctx, s.client.post, "crm/task-types", params, opts)
}

// UpdateTaskType modifies a task type.
func (s *CRMService) UpdateTaskType(ctx context.Context, id string, params *TaskTypeUpdateParams, opts ...RequestOption) (*CRMTaskType, *Response, error) {
	return send[CRMTaskType](ctx, s.client.patch, "crm/task-types/"+url.PathEscape(id), params, opts)
}

// DeleteTaskType removes a task type. Tasks that used it keep its name.
func (s *CRMService) DeleteTaskType(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "crm/task-types/"+url.PathEscape(id), opts...)
}

// --- tasks ---

// ListTasks returns a page of CRM tasks, optionally narrowed to one contact,
// deal, assignee or status.
func (s *CRMService) ListTasks(ctx context.Context, params *CRMTaskListParams, opts ...RequestOption) (*Page[CRMTask], error) {
	return listJSON[CRMTask](ctx, s.client, "crm/tasks", params.values(), opts...)
}

// SearchTasks returns a page of tasks matching a faceted filter. Its page
// limit is capped at 200, twice what [CRMService.ListTasks] allows, and
// [Pagination.Total] is an exact count over the whole matching set.
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
	return send[TasksSummary](ctx, s.client.post, "crm/tasks/summary", params, opts)
}

// CreateTask opens a new CRM task.
func (s *CRMService) CreateTask(ctx context.Context, params *CRMTaskCreateParams, opts ...RequestOption) (*CRMTask, *Response, error) {
	return send[CRMTask](ctx, s.client.post, "crm/tasks", params, opts)
}

// GetTask retrieves a single CRM task.
func (s *CRMService) GetTask(ctx context.Context, id string, opts ...RequestOption) (*CRMTask, *Response, error) {
	return fetch[CRMTask](ctx, s.client, "crm/tasks/"+url.PathEscape(id), opts)
}

// UpdateTask modifies a CRM task.
func (s *CRMService) UpdateTask(ctx context.Context, id string, params *CRMTaskUpdateParams, opts ...RequestOption) (*CRMTask, *Response, error) {
	return send[CRMTask](ctx, s.client.patch, "crm/tasks/"+url.PathEscape(id), params, opts)
}

// DeleteTask removes a CRM task.
func (s *CRMService) DeleteTask(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "crm/tasks/"+url.PathEscape(id), opts...)
}

// BulkUpdateTasks writes one status and/or priority onto a whole selection and
// returns how many tasks changed. The response carries the count rather than
// the rows, because a select-all can cover tens of thousands. Requires the
// write-CRM scope.
func (s *CRMService) BulkUpdateTasks(ctx context.Context, params *BulkTaskUpdateParams, opts ...RequestOption) (*BulkTasksResult, *Response, error) {
	return send[BulkTasksResult](ctx, s.client.patch, "crm/tasks", params, opts)
}

// BulkDeleteTasks deletes a whole selection and returns how many tasks were
// removed. Requires the write-CRM scope.
func (s *CRMService) BulkDeleteTasks(ctx context.Context, selection *TaskSelection, opts ...RequestOption) (*BulkTasksResult, *Response, error) {
	var out BulkTasksResult
	resp, err := s.client.call(ctx, http.MethodDelete, "crm/tasks", selection, &out, opts)
	if err != nil {
		return nil, resp, err
	}
	return &out, resp, nil
}

// --- connected CRM (HubSpot, Pipedrive) ---

// CRM providers returned in [CRMSettings.Provider]. Unknown values may appear
// on newer servers.
const (
	// CRMProviderNative is Warmbly's own CRM, the pipelines, deals and tasks
	// above.
	CRMProviderNative = "native"
	// CRMProviderHubSpot runs the workspace's CRM on a connected HubSpot
	// account.
	CRMProviderHubSpot = "hubspot"
	// CRMProviderPipedrive runs it on a connected Pipedrive account.
	CRMProviderPipedrive = "pipedrive"
)

// CRM field directions in [CRMProviderConfig.FieldDirection].
const (
	// CRMFieldPush means Warmbly writes the provider.
	CRMFieldPush = "push"
	// CRMFieldPull means the provider writes Warmbly.
	CRMFieldPull = "pull"
	// CRMFieldBoth means the most recent change wins.
	CRMFieldBoth = "both"
)

// CRMExternalRef says where a CRM record lives outside Warmbly. It is attached
// to deals, tasks, notes and pipelines read while a connected CRM is active.
type CRMExternalRef struct {
	// Provider is one of the CRMProvider* constants.
	Provider   string    `json:"provider"`
	ExternalID string    `json:"external_id"`
	URL        string    `json:"url,omitempty"`
	SyncedAt   time.Time `json:"synced_at"`
	// OwnerName is the provider owner when that owner is not a workspace
	// member.
	OwnerName string `json:"owner_name,omitempty"`
}

// CRMActivityLog chooses which Warmbly activity is written to the provider's
// timeline. Opens and clicks default off, because machine opens make them
// noise.
type CRMActivityLog struct {
	Sent         bool `json:"sent"`
	Replies      bool `json:"replies"`
	Bounces      bool `json:"bounces"`
	Unsubscribes bool `json:"unsubscribes"`
	Opens        bool `json:"opens"`
	Clicks       bool `json:"clicks"`
	Meetings     bool `json:"meetings"`
}

// CRMReplyOutcome is what a positive ("interested") reply does in the provider.
type CRMReplyOutcome struct {
	// LeadStatus and LifecycleStage are written to the contact; empty leaves
	// them alone.
	LeadStatus     string `json:"lead_status"`
	LifecycleStage string `json:"lifecycle_stage"`
	CreateDeal     bool   `json:"create_deal"`
	// CreateLead adds the person to Pipedrive's Leads Inbox (Pipedrive only).
	CreateLead bool `json:"create_lead,omitempty"`
	// DealPipelineID and DealStageID are local (mirrored) ids, required when
	// CreateDeal is set.
	DealPipelineID *string `json:"deal_pipeline_id,omitempty"`
	DealStageID    *string `json:"deal_stage_id,omitempty"`
}

// CRMExitRules stop a contact's campaigns when the provider says they have
// moved on, so nobody is followed up after becoming a deal or a customer.
type CRMExitRules struct {
	DealCreated bool `json:"deal_created"`
	// LifecycleStages stops a contact whose lifecycle stage becomes any of
	// these.
	LifecycleStages []string `json:"lifecycle_stages"`
	OptedOut        bool     `json:"opted_out"`
}

// CRMEnrollmentGuards skip contacts at import and enrollment time.
type CRMEnrollmentGuards struct {
	SkipLifecycleStages []string `json:"skip_lifecycle_stages"`
	SkipOpenDeals       bool     `json:"skip_open_deals"`
	SkipOtherOwners     bool     `json:"skip_other_owners"`
	SkipOptedOut        bool     `json:"skip_opted_out"`
}

// CRMProviderConfig is every choice the CRM setup wizard asks, with defaults
// that work without changing anything.
type CRMProviderConfig struct {
	Activity CRMActivityLog `json:"activity"`
	// CreateContacts lets Warmbly create a provider contact for someone it
	// emails who is not in the CRM yet.
	CreateContacts bool `json:"create_contacts"`
	// CreateCompanies creates and associates a company from the contact's
	// domain.
	CreateCompanies bool `json:"create_companies"`
	// WriteProperties keeps the "Warmbly" property group on contacts current.
	WriteProperties bool                `json:"write_properties"`
	PositiveReply   CRMReplyOutcome     `json:"positive_reply"`
	ExitRules       CRMExitRules        `json:"exit_rules"`
	Guards          CRMEnrollmentGuards `json:"guards"`
	// DealPipelines limits the mirrored deals to these provider pipeline ids;
	// empty means every pipeline.
	DealPipelines []string `json:"deal_pipelines"`
	// DisplayProperties are extra contact properties shown in the dashboard.
	DisplayProperties []string `json:"display_properties"`
	// FieldMap maps a Warmbly contact field (first_name, last_name, email,
	// company, phone or custom:x) to a provider property; FieldDirection says
	// which side wins per field (a CRMField* constant).
	FieldMap       map[string]string `json:"field_map"`
	FieldDirection map[string]string `json:"field_direction"`
	// For is the provider these choices were made for. Empty means HubSpot.
	For string `json:"for,omitempty"`
}

// CRMAccount identifies the connected provider account.
type CRMAccount struct {
	ExternalID string `json:"external_id"`
	Name       string `json:"name"`
	// AppURL is the provider's web app root for this account, for "Open in"
	// links.
	AppURL string `json:"app_url"`
	Status string `json:"status"`
	Health string `json:"health"`
	// MissingScopes lists permissions the connection lacks for full mode, so a
	// client can ask for a reconnect.
	MissingScopes []string `json:"missing_scopes,omitempty"`
}

// CRMSettings is the workspace's CRM mode.
type CRMSettings struct {
	OrganizationID string `json:"organization_id"`
	// Provider is one of the CRMProvider* constants.
	Provider     string            `json:"provider"`
	ConnectionID *string           `json:"connection_id,omitempty"`
	Config       CRMProviderConfig `json:"config"`
	// SetupCompletedAt is when the setup wizard was finished.
	SetupCompletedAt *time.Time `json:"setup_completed_at,omitempty"`
	UpdatedAt        time.Time  `json:"updated_at"`
	// Account is read-only facts about the connected account.
	Account *CRMAccount `json:"account,omitempty"`
}

// CRMSettingsUpdateParams switches the CRM and stores the setup choices. Nil
// fields are left unchanged.
type CRMSettingsUpdateParams struct {
	// Provider is one of the CRMProvider* constants.
	Provider *string `json:"provider,omitempty"`
	// ConnectionID is the integration connection the provider runs on.
	ConnectionID *string            `json:"connection_id,omitempty"`
	Config       *CRMProviderConfig `json:"config,omitempty"`
	// CompleteSetup stamps the wizard as finished.
	CompleteSetup bool `json:"complete_setup,omitempty"`
}

// CRMOption is a provider enum value with its label.
type CRMOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// CRMProperty is a provider contact property a field can map to.
type CRMProperty struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	Type      string `json:"type"`
	GroupName string `json:"group_name"`
	ReadOnly  bool   `json:"read_only"`
}

// CRMMetadata is the provider vocabulary the pickers render.
type CRMMetadata struct {
	LifecycleStages []CRMOption   `json:"lifecycle_stages"`
	LeadStatuses    []CRMOption   `json:"lead_statuses"`
	TaskTypes       []CRMOption   `json:"task_types"`
	Properties      []CRMProperty `json:"properties"`
	Pipelines       []CRMOption   `json:"pipelines"`
}

// CRMOwner is a provider user who can own records.
type CRMOwner struct {
	ExternalID string `json:"external_id"`
	Email      string `json:"email"`
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
	// UserID is the workspace member this owner maps to, when one does.
	UserID *string `json:"user_id,omitempty"`
	// UserPinned is true when the mapping was set by hand with
	// [CRMService.MapOwner] rather than matched by email.
	UserPinned bool `json:"user_pinned"`
	Archived   bool `json:"archived"`
}

// CRMCompanyRef is a contact's primary company in the provider.
type CRMCompanyRef struct {
	ExternalID string `json:"external_id"`
	Name       string `json:"name"`
	Domain     string `json:"domain,omitempty"`
	URL        string `json:"url,omitempty"`
}

// CRMPropertyView is one displayed provider property, labeled.
type CRMPropertyView struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Value string `json:"value"`
}

// CRMContactView is the provider side of a contact: what a contact drawer or
// inbox panel shows while a connected CRM is active.
type CRMContactView struct {
	// Provider is one of the CRMProvider* constants.
	Provider string `json:"provider"`
	// Linked is false until the contact is found or created in the provider
	// ([CRMService.LinkContact]).
	Linked         bool              `json:"linked"`
	ExternalID     string            `json:"external_id,omitempty"`
	URL            string            `json:"url,omitempty"`
	Owner          *CRMOwner         `json:"owner,omitempty"`
	LifecycleStage *CRMOption        `json:"lifecycle_stage,omitempty"`
	LeadStatus     *CRMOption        `json:"lead_status,omitempty"`
	Company        *CRMCompanyRef    `json:"company,omitempty"`
	OptedOut       bool              `json:"opted_out"`
	Properties     []CRMPropertyView `json:"properties"`
	SyncedAt       *time.Time        `json:"synced_at,omitempty"`
}

// CRMContactUpdateParams edits a contact's sales fields in the provider. Nil
// fields are left unchanged; each value is at most 100 characters.
type CRMContactUpdateParams struct {
	OwnerExternalID *string `json:"owner_external_id,omitempty"`
	LifecycleStage  *string `json:"lifecycle_stage,omitempty"`
	LeadStatus      *string `json:"lead_status,omitempty"`
}

// CRMSyncJob is one queued or failed provider sync job.
type CRMSyncJob struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Provider       string `json:"provider"`
	// Kind names the work, for example "push_deal" or "log_email".
	Kind          string     `json:"kind"`
	Subject       string     `json:"subject"`
	Status        string     `json:"status"`
	Attempts      int        `json:"attempts"`
	NextAttemptAt time.Time  `json:"next_attempt_at"`
	LastError     string     `json:"last_error,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
}

// CRMSyncCursor reports one incremental pull from the provider.
type CRMSyncCursor struct {
	ObjectType string     `json:"object_type"`
	CursorAt   *time.Time `json:"cursor_at,omitempty"`
	LastRunAt  *time.Time `json:"last_run_at,omitempty"`
	LastError  string     `json:"last_error,omitempty"`
}

// CRMMirrorCounts is how much provider data the workspace mirrors.
type CRMMirrorCounts struct {
	Contacts  int64 `json:"contacts"`
	Deals     int64 `json:"deals"`
	Tasks     int64 `json:"tasks"`
	Pipelines int64 `json:"pipelines"`
	Owners    int64 `json:"owners"`
}

// CRMSyncHealth is the sync health panel: counts, failures and freshness.
type CRMSyncHealth struct {
	Pending      int64           `json:"pending"`
	Failed       int64           `json:"failed"`
	Done24h      int64           `json:"done_24h"`
	LastSyncedAt *time.Time      `json:"last_synced_at,omitempty"`
	Cursors      []CRMSyncCursor `json:"cursors"`
	Failures     []CRMSyncJob    `json:"failures"`
	Counts       CRMMirrorCounts `json:"counts"`
}

// CRMList is a provider contact list Warmbly can import from.
type CRMList struct {
	ExternalID string `json:"external_id"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	Dynamic    bool   `json:"dynamic"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

// CRMListParams filters and paginates [CRMService.Lists]. The server caps
// Limit at 100 and defaults it to 25.
type CRMListParams struct {
	ListOptions
	// Query narrows by list name (at most 200 characters).
	Query string
}

// CRMImportParams previews or imports a provider list.
type CRMImportParams struct {
	// ListID is the provider list's id (at most 64 characters).
	ListID string `json:"list_id"`
	// ApplyGuards drops contacts the workspace's enrollment guards exclude.
	ApplyGuards bool `json:"apply_guards"`
}

// CRMImportSkip is one skip reason and how many contacts it removed.
type CRMImportSkip struct {
	Reason string `json:"reason"`
	Label  string `json:"label"`
	Count  int    `json:"count"`
}

// CRMImportPerson is a sample row of an import preview.
type CRMImportPerson struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Company   string `json:"company"`
}

// CRMImportPreview counts who a list import would bring in and who it skips.
type CRMImportPreview struct {
	ListName string            `json:"list_name"`
	Total    int               `json:"total"`
	Included int               `json:"included"`
	Skipped  []CRMImportSkip   `json:"skipped"`
	Sample   []CRMImportPerson `json:"sample"`
	// Truncated is set when the list is larger than one import takes.
	Truncated bool `json:"truncated"`
}

// CRMImportResult names the contact import draft a list import produced. Finish
// it through the contact importer ([ContactService] import methods) using
// ImportID.
type CRMImportResult struct {
	ImportID string           `json:"import_id"`
	Preview  CRMImportPreview `json:"preview"`
}

// CRMBackfillParams chooses which of Warmbly's own CRM data is copied into the
// provider once, when a workspace switches.
type CRMBackfillParams struct {
	Deals bool `json:"deals"`
	Tasks bool `json:"tasks"`
	Notes bool `json:"notes"`
}

// CRMBackfillPreview counts what a backfill would copy.
type CRMBackfillPreview struct {
	Deals int64 `json:"deals"`
	Tasks int64 `json:"tasks"`
	Notes int64 `json:"notes"`
}

// The connected-CRM routes answer 501 when the instance has no CRM providers
// configured, and 4xx when the workspace is still on [CRMProviderNative] and
// the call needs a provider. Reads need the read-CRM scope (or view-contacts),
// contact edits the write-CRM scope, and changing the mode, owners, sync or
// backfill the integrations scope (or manage-settings).

// Settings returns the workspace's CRM mode and the connected account.
func (s *CRMService) Settings(ctx context.Context, opts ...RequestOption) (*CRMSettings, *Response, error) {
	return fetch[CRMSettings](ctx, s.client, "crm/settings", opts)
}

// UpdateSettings switches the CRM and stores the setup choices. Requires the
// integrations scope.
func (s *CRMService) UpdateSettings(ctx context.Context, params *CRMSettingsUpdateParams, opts ...RequestOption) (*CRMSettings, *Response, error) {
	return send[CRMSettings](ctx, s.client.put, "crm/settings", params, opts)
}

// Metadata returns the provider's lifecycle stages, lead statuses, task types,
// contact properties and pipelines, for pickers.
func (s *CRMService) Metadata(ctx context.Context, opts ...RequestOption) (*CRMMetadata, *Response, error) {
	return fetch[CRMMetadata](ctx, s.client, "crm/metadata", opts)
}

// Owners returns the provider's owners and the member each maps to.
func (s *CRMService) Owners(ctx context.Context, opts ...RequestOption) ([]CRMOwner, *Response, error) {
	return fetchData[CRMOwner](ctx, s.client, "crm/owners", opts)
}

// MapOwner pins a provider owner to a workspace member. A nil userID clears
// the pin. Requires the integrations scope.
func (s *CRMService) MapOwner(ctx context.Context, externalID string, userID *string, opts ...RequestOption) (*Response, error) {
	body := struct {
		UserID *string `json:"user_id"`
	}{UserID: userID}
	return s.client.put(ctx, "crm/owners/"+url.PathEscape(externalID), body, nil, opts...)
}

// SyncHealth reports what is waiting, what failed and when each pull ran.
func (s *CRMService) SyncHealth(ctx context.Context, opts ...RequestOption) (*CRMSyncHealth, *Response, error) {
	return fetch[CRMSyncHealth](ctx, s.client, "crm/sync", opts)
}

// SyncNow starts a full pull from the provider; the server answers 202 and
// finishes in the background. Requires the integrations scope.
func (s *CRMService) SyncNow(ctx context.Context, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "crm/sync", nil, nil, opts...)
}

// RetrySyncFailures requeues failed sync jobs: the given ids, or every failed
// job when ids is empty (at most 500 ids). It returns how many were requeued.
// Requires the integrations scope.
func (s *CRMService) RetrySyncFailures(ctx context.Context, ids []string, opts ...RequestOption) (int64, *Response, error) {
	return s.syncJobs(ctx, "crm/sync/retry", ids, opts)
}

// DiscardSyncFailures drops failed sync jobs: the given ids, or every failed
// job when ids is empty (at most 500 ids). It returns how many were dropped.
// Requires the integrations scope.
func (s *CRMService) DiscardSyncFailures(ctx context.Context, ids []string, opts ...RequestOption) (int64, *Response, error) {
	return s.syncJobs(ctx, "crm/sync/discard", ids, opts)
}

func (s *CRMService) syncJobs(ctx context.Context, path string, ids []string, opts []RequestOption) (int64, *Response, error) {
	body := struct {
		IDs []string `json:"ids"`
	}{IDs: ids}
	out, resp, err := send[BulkTasksResult](ctx, s.client.post, path, body, opts)
	if err != nil {
		return 0, resp, err
	}
	return out.Affected, resp, nil
}

// BackfillPreview counts the Warmbly-only deals, tasks and notes a switch to a
// connected CRM would copy. Requires the integrations scope.
func (s *CRMService) BackfillPreview(ctx context.Context, opts ...RequestOption) (*CRMBackfillPreview, *Response, error) {
	return fetch[CRMBackfillPreview](ctx, s.client, "crm/backfill", opts)
}

// StartBackfill copies Warmbly's own deals, tasks and notes into the provider
// once. It is idempotent: a second call while one runs is folded into it, and
// copied records are never copied twice. The server answers 202. Requires the
// integrations scope.
func (s *CRMService) StartBackfill(ctx context.Context, params *CRMBackfillParams, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "crm/backfill", params, nil, opts...)
}

// Contact returns the provider side of a contact: owner, lifecycle stage, lead
// status, company and the chosen properties.
func (s *CRMService) Contact(ctx context.Context, contactID string, opts ...RequestOption) (*CRMContactView, *Response, error) {
	return fetch[CRMContactView](ctx, s.client, "crm/contacts/"+url.PathEscape(contactID), opts)
}

// RefreshContact pulls a contact's provider record, deals, tasks and notes now.
// The server debounces it per contact, so it is safe to call whenever a panel
// opens.
func (s *CRMService) RefreshContact(ctx context.Context, contactID string, opts ...RequestOption) (*CRMContactView, *Response, error) {
	return send[CRMContactView](ctx, s.client.post, "crm/contacts/"+url.PathEscape(contactID)+"/refresh", nil, opts)
}

// LinkContact finds or creates the contact in the provider. Requires the
// write-CRM scope.
func (s *CRMService) LinkContact(ctx context.Context, contactID string, opts ...RequestOption) (*CRMContactView, *Response, error) {
	return send[CRMContactView](ctx, s.client.post, "crm/contacts/"+url.PathEscape(contactID)+"/link", nil, opts)
}

// UpdateContact edits a contact's owner, lifecycle stage or lead status in the
// provider. Requires the write-CRM scope.
func (s *CRMService) UpdateContact(ctx context.Context, contactID string, params *CRMContactUpdateParams, opts ...RequestOption) (*CRMContactView, *Response, error) {
	return send[CRMContactView](ctx, s.client.patch, "crm/contacts/"+url.PathEscape(contactID), params, opts)
}

// Lists returns a page of the provider's contact lists for import. Requires
// the manage-contacts permission and the read-CRM scope.
func (s *CRMService) Lists(ctx context.Context, params *CRMListParams, opts ...RequestOption) (*Page[CRMList], error) {
	q := make(url.Values)
	if params != nil {
		params.apply(q)
		setNonEmpty(q, "q", params.Query)
	}
	return listJSON[CRMList](ctx, s.client, "crm/lists", q, opts...)
}

// PreviewListImport counts who importing a provider list would bring in and
// who it would skip. Requires the manage-contacts permission and the
// write-contacts scope.
func (s *CRMService) PreviewListImport(ctx context.Context, params *CRMImportParams, opts ...RequestOption) (*CRMImportPreview, *Response, error) {
	return send[CRMImportPreview](ctx, s.client.post, "crm/lists/preview", params, opts)
}

// ImportList turns a provider list into a contact import draft (HTTP 201) that
// is finished through the contact importer. It needs a signed-in member, so an
// API key is refused.
func (s *CRMService) ImportList(ctx context.Context, params *CRMImportParams, opts ...RequestOption) (*CRMImportResult, *Response, error) {
	return send[CRMImportResult](ctx, s.client.post, "crm/lists/import", params, opts)
}
