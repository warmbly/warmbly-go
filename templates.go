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
	// Position is the template's place in the organization's ordered list.
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
	// Field is [TemplateFieldSubject] or [TemplateFieldBody] when the issue
	// lives in exactly one of them, and empty when it spans both or describes
	// the send as a whole.
	Field string `json:"field,omitempty"`
	// Spans are the exact fragments that triggered the issue, in reading
	// order. Empty when there is nothing to point at.
	Spans []TemplateSpan `json:"spans,omitempty"`
	// Suggestion is the concrete fix, in one line.
	Suggestion string `json:"suggestion,omitempty"`
}

// Fields a [TemplateIssue], [TemplateSpan] or [TemplateFinding] can point at.
const (
	TemplateFieldSubject = "subject"
	TemplateFieldBody    = "body"
)

// TemplateSpan is a fragment of the copy that triggered a [TemplateIssue].
type TemplateSpan struct {
	// Field is [TemplateFieldSubject] or [TemplateFieldBody].
	Field string `json:"field"`
	// Text is the fragment as it is written in the copy.
	Text string `json:"text"`
	// Line is the 1-based line of that field the fragment sits on.
	Line int `json:"line,omitempty"`
	// Excerpt is the whole line, so the fragment can be shown in context.
	Excerpt string `json:"excerpt,omitempty"`
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

// Reorder repositions the given templates in the listed order and returns the
// full reordered list. Templates not in ids keep their positions.
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

// TemplateIssueInfo is the lowest severity, used by [TemplateFinding].
const TemplateIssueInfo = "info"

// TemplateFinding is a located problem from [TemplateService.Analyze], most
// severe first.
type TemplateFinding struct {
	// Severity is [TemplateIssueHigh], [TemplateIssueWarn] or
	// [TemplateIssueInfo].
	Severity string `json:"severity"`
	// Field is [TemplateFieldSubject] or [TemplateFieldBody]; empty when the
	// finding is about the email as a whole.
	Field string `json:"field,omitempty"`
	// Text is the exact fragment quoted from the copy, empty for a whole-email
	// finding.
	Text string `json:"text,omitempty"`
	// Line is the 1-based line of that field the fragment sits on.
	Line int `json:"line,omitempty"`
	// Excerpt is the whole line, for context.
	Excerpt string `json:"excerpt,omitempty"`
	// Issue is why it hurts deliverability.
	Issue string `json:"issue"`
	// Suggestion is what to write instead.
	Suggestion string `json:"suggestion,omitempty"`
}

// TemplateJudgment is how the copy reads to its recipient, as judged by a
// language model. Scores run 0 to 1.
type TemplateJudgment struct {
	// ReadsAs and Personalization are normalized rubric positions: how much
	// the tone reads as bulk mail and how little it is tailored to the
	// recipient. From 0.75 the copy counts as bulk.
	ReadsAs         float64 `json:"reads_as"`
	Personalization float64 `json:"personalization"`
	// Ask is "one_clear_ask", "several_asks" or "no_ask". The set is open.
	Ask string `json:"ask"`
	// SpamClaim is the probability the copy makes an explicit promotional
	// claim; from 0.70 one counts as present.
	SpamClaim float64 `json:"spam_claim"`
	// Confidence is the lowest confidence across the scores and the choice.
	Confidence  float64 `json:"confidence"`
	Model       string  `json:"model"`
	InputTokens int     `json:"input_tokens"`
}

// TemplateAnalysis is an AI spam analysis of a piece of copy: which word and
// sentence hurts deliverability, whether it is in the subject or the body, and
// what to write instead.
type TemplateAnalysis struct {
	// Score is the overall 0 to 100 deliverability score, higher being safer.
	Score   int    `json:"score"`
	Verdict string `json:"verdict"`
	// Findings are located problems, most severe first. It is empty when only
	// the rules and the judgment answered.
	Findings []TemplateFinding `json:"findings"`
	// SuggestedSubject is a rewritten subject line; empty when the current one
	// is fine.
	SuggestedSubject string `json:"suggested_subject,omitempty"`
	// Improvements are copy-level suggestions with nothing specific to quote.
	Improvements []string `json:"improvements,omitempty"`
	// Rules is the rules-based score of the same copy.
	Rules TemplateScore `json:"rules"`
	// Judgment is set when the instance has a copy judge configured.
	Judgment *TemplateJudgment `json:"judgment,omitempty"`
	Model    string            `json:"model"`
	// TokensUsed is what the analysis consumed.
	TokensUsed int `json:"tokens_used"`
	// CreditsRemaining is the workspace's AI credit balance after the call and
	// CreditsCharged what the call cost (0 on a local model or when only the
	// copy judge answered).
	CreditsRemaining int `json:"credits_remaining"`
	CreditsCharged   int `json:"credits_charged"`
}

// Analyze runs an AI spam analysis over arbitrary copy and returns located
// findings, a suggested subject and the rules score, in one reading of the
// copy. It spends AI credits (see [TemplateAnalysis.CreditsCharged]); pass
// [WithIdempotencyKey] when you may retry so a retried call is not charged
// twice. At least one of the three fields must be non-empty and together they
// may not exceed 60,000 bytes. An instance with no AI provider answers 503
// with the code "ai_not_configured", and a workspace without an active plan or
// trial 403. Requires the write_templates scope and an organization.
func (s *TemplateService) Analyze(ctx context.Context, params *TemplateScoreParams, opts ...RequestOption) (*TemplateAnalysis, *Response, error) {
	return send[TemplateAnalysis](ctx, s.client.post, "templates/analyze", params, opts)
}

// Score rates arbitrary copy for spam risk. It works on any subject and body,
// not just a stored template.
func (s *TemplateService) Score(ctx context.Context, params *TemplateScoreParams, opts ...RequestOption) (*TemplateScore, *Response, error) {
	return send[TemplateScore](ctx, s.client.post, "templates/score", params, opts)
}
