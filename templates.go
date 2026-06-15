package warmbly

import (
	"context"
	"net/url"
	"time"
)

// TemplateService manages reusable message templates for the current
// organization. Templates capture a subject line and body that can be reused
// across campaigns and one-off sends.
type TemplateService service

// Template is a reusable message template as returned by the API.
type Template struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Subject string `json:"subject"`
	// BodyPlain is the plain-text body of the template.
	BodyPlain string `json:"body_plain"`
	// BodyHTML is the HTML body of the template.
	BodyHTML string `json:"body_html"`
	// Tags are optional labels used to organize and filter templates.
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TemplateCreateParams are the parameters for creating a template.
type TemplateCreateParams struct {
	Name      string   `json:"name"`
	Subject   string   `json:"subject"`
	BodyPlain string   `json:"body_plain,omitempty"`
	BodyHTML  string   `json:"body_html,omitempty"`
	Tags      []string `json:"tags,omitempty"`
}

// TemplateUpdateParams are the parameters for updating a template. Only non-nil
// fields are sent, so zero values are not mistaken for "clear this field".
type TemplateUpdateParams struct {
	Name      *string   `json:"name,omitempty"`
	Subject   *string   `json:"subject,omitempty"`
	BodyPlain *string   `json:"body_plain,omitempty"`
	BodyHTML  *string   `json:"body_html,omitempty"`
	Tags      *[]string `json:"tags,omitempty"`
}

// TemplateListParams filters and paginates a list of templates.
type TemplateListParams struct {
	ListOptions
	// Search filters by name substring.
	Search string
}

func (p *TemplateListParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	if p.Search != "" {
		q.Set("search", p.Search)
	}
	return q
}

// List returns a page of templates.
func (s *TemplateService) List(ctx context.Context, params *TemplateListParams) (*Page[Template], error) {
	return listJSON[Template](ctx, s.client, "templates", params.values())
}

// Get retrieves a single template by ID.
func (s *TemplateService) Get(ctx context.Context, id string) (*Template, *Response, error) {
	tmpl := new(Template)
	resp, err := s.client.get(ctx, "templates/"+url.PathEscape(id), tmpl)
	if err != nil {
		return nil, resp, err
	}
	return tmpl, resp, nil
}

// Create creates a new template.
func (s *TemplateService) Create(ctx context.Context, params *TemplateCreateParams) (*Template, *Response, error) {
	tmpl := new(Template)
	resp, err := s.client.post(ctx, "templates", params, tmpl)
	if err != nil {
		return nil, resp, err
	}
	return tmpl, resp, nil
}

// Update modifies an existing template.
func (s *TemplateService) Update(ctx context.Context, id string, params *TemplateUpdateParams) (*Template, *Response, error) {
	tmpl := new(Template)
	resp, err := s.client.patch(ctx, "templates/"+url.PathEscape(id), params, tmpl)
	if err != nil {
		return nil, resp, err
	}
	return tmpl, resp, nil
}

// Delete permanently deletes a template.
func (s *TemplateService) Delete(ctx context.Context, id string) (*Response, error) {
	return s.client.delete(ctx, "templates/"+url.PathEscape(id))
}
