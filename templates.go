package warmbly

import (
	"context"
	"net/url"
	"time"
)

// TemplateService manages the organization's reply templates: reusable subject
// and body snippets that can be rendered into a campaign step or a one-off
// reply. Templates are ordered, and the order is editable.
type TemplateService service

// Template is a reusable message template as returned by the API.
type Template struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	UserID         string `json:"user_id"`
	Name           string `json:"name"`
	Subject        string `json:"subject"`
	BodyHTML       string `json:"body_html"`
	BodyPlain      string `json:"body_plain"`
	// Position is the template's zero-based index in the organization's list.
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TemplateCreateParams creates a template. Only Name is required.
type TemplateCreateParams struct {
	Name      string `json:"name"`
	Subject   string `json:"subject,omitempty"`
	BodyHTML  string `json:"body_html,omitempty"`
	BodyPlain string `json:"body_plain,omitempty"`
}

// TemplateUpdateParams updates a template. Nil fields are left unchanged.
type TemplateUpdateParams struct {
	Name      *string `json:"name,omitempty"`
	Subject   *string `json:"subject,omitempty"`
	BodyHTML  *string `json:"body_html,omitempty"`
	BodyPlain *string `json:"body_plain,omitempty"`
}

// TemplateRenderResult is a template rendered against a set of variables.
type TemplateRenderResult struct {
	Subject   string `json:"subject"`
	BodyHTML  string `json:"body_html"`
	BodyPlain string `json:"body_plain"`
}

// Severities returned in [TemplateIssue.Severity].
const (
	// TemplateIssueWarn is worth fixing but will not sink the message.
	TemplateIssueWarn = "warn"
	// TemplateIssueHigh materially risks landing in spam.
	TemplateIssueHigh = "high"
)

// TemplateScore rates copy for spam risk before it is ever sent. Score runs 0
// to 100, higher being safer.
type TemplateScore struct {
	Score  int             `json:"score"`
	Issues []TemplateIssue `json:"issues"`
}

// TemplateIssue is one problem found while scoring copy.
type TemplateIssue struct {
	// Severity is [TemplateIssueWarn] or [TemplateIssueHigh].
	Severity string `json:"severity"`
	// Code is the stable identifier for the rule that fired.
	Code    string `json:"code"`
	Message string `json:"message"`
}

// TemplateScoreParams is the copy to score. Pass whichever parts exist.
type TemplateScoreParams struct {
	Subject   string `json:"subject,omitempty"`
	BodyHTML  string `json:"body_html,omitempty"`
	BodyPlain string `json:"body_plain,omitempty"`
}

// List returns the organization's templates in their configured order. Query
// is an optional case-insensitive search over name and subject.
func (s *TemplateService) List(ctx context.Context, query string, opts ...RequestOption) ([]Template, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "q", query)
	return fetchData[Template](ctx, s.client, withQuery("templates", q), opts)
}

// Get retrieves a single template by ID.
func (s *TemplateService) Get(ctx context.Context, id string, opts ...RequestOption) (*Template, *Response, error) {
	return fetch[Template](ctx, s.client, "templates/"+url.PathEscape(id), opts)
}

// Create creates a new template, appended to the end of the list.
func (s *TemplateService) Create(ctx context.Context, params *TemplateCreateParams, opts ...RequestOption) (*Template, *Response, error) {
	return send[Template](ctx, s.client.post, "templates", params, opts)
}

// Update modifies an existing template.
func (s *TemplateService) Update(ctx context.Context, id string, params *TemplateUpdateParams, opts ...RequestOption) (*Template, *Response, error) {
	return send[Template](ctx, s.client.patch, "templates/"+url.PathEscape(id), params, opts)
}

// Delete permanently deletes a template.
func (s *TemplateService) Delete(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "templates/"+url.PathEscape(id), opts...)
}

// Reorder sets the template order to exactly the given ids and returns the
// reordered list.
func (s *TemplateService) Reorder(ctx context.Context, ids []string, opts ...RequestOption) ([]Template, *Response, error) {
	body := struct {
		IDs []string `json:"ids"`
	}{IDs: ids}
	return sendData[Template](ctx, s.client.patch, "templates/reorder", body, opts)
}

// Duplicate copies a template, appending the copy to the end of the list.
func (s *TemplateService) Duplicate(ctx context.Context, id string, opts ...RequestOption) (*Template, *Response, error) {
	return send[Template](ctx, s.client.post, "templates/"+url.PathEscape(id)+"/duplicate", nil, opts)
}

// Render fills a template's merge tags from variables and returns the result.
// Nothing is sent.
func (s *TemplateService) Render(ctx context.Context, id string, variables map[string]string, opts ...RequestOption) (*TemplateRenderResult, *Response, error) {
	body := struct {
		Variables map[string]string `json:"variables,omitempty"`
	}{Variables: variables}
	return send[TemplateRenderResult](ctx, s.client.post, "templates/"+url.PathEscape(id)+"/render", body, opts)
}

// Score rates arbitrary copy for spam risk. It works on any subject and body,
// not just a stored template.
func (s *TemplateService) Score(ctx context.Context, params *TemplateScoreParams, opts ...RequestOption) (*TemplateScore, *Response, error) {
	return send[TemplateScore](ctx, s.client.post, "templates/score", params, opts)
}
