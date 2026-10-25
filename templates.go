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
	// Field is "subject" or "body" when the issue sits in one of them.
	Field string `json:"field,omitempty"`
	// Spans locate the offending text in the copy, when the rule can.
	Spans []TemplateSpan `json:"spans,omitempty"`
	// Suggestion is what to write instead, when the rule has one.
	Suggestion string `json:"suggestion,omitempty"`
}

// TemplateSpan locates the text behind a [TemplateIssue].
type TemplateSpan struct {
	// Field is "subject" or "body".
	Field string `json:"field"`
	// Text is the exact fragment.
	Text string `json:"text"`
	// Line is the 1-based line of that field it sits on; Excerpt is the
	// whole line for context.
	Line    int    `json:"line,omitempty"`
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

// Score rates arbitrary copy for spam risk. It works on any subject and body,
// not just a stored template.
func (s *TemplateService) Score(ctx context.Context, params *TemplateScoreParams, opts ...RequestOption) (*TemplateScore, *Response, error) {
	return send[TemplateScore](ctx, s.client.post, "templates/score", params, opts)
}

// TemplateIssueInfo is the severity of a [TemplateFinding] that is advice
// rather than a problem.
const TemplateIssueInfo = "info"

// TemplateFinding is one problem an AI analysis found in copy.
type TemplateFinding struct {
	// Severity is "high", "warn" or "info".
	Severity string `json:"severity"`
	// Field is "subject" or "body"; empty when the finding is about the email
	// as a whole.
	Field string `json:"field,omitempty"`
	// Text is the exact fragment quoted from the copy; empty when the finding
	// is about the email as a whole.
	Text string `json:"text,omitempty"`
	// Line is the 1-based line of that field the fragment sits on; Excerpt is
	// the whole line.
	Line    int    `json:"line,omitempty"`
	Excerpt string `json:"excerpt,omitempty"`
	// Issue is why it hurts deliverability and Suggestion is what to write
	// instead.
	Issue      string `json:"issue"`
	Suggestion string `json:"suggestion,omitempty"`
	// Category is a coarse grouping such as "trigger_word", "tone",
	// "formatting", "links", "structure" or "authenticity".
	Category string `json:"category,omitempty"`
}

// TemplateJudgment is how the copy reads to its recipient, scored on two 0 to
// 1 scales where 0 is the personal end.
type TemplateJudgment struct {
	// ReadsAs runs from a personal note (0) to bulk mail (1).
	ReadsAs         float64 `json:"reads_as"`
	Personalization float64 `json:"personalization"`
	// Ask is what the email asks the reader for, for example "none" or
	// "several"; the set grows, so treat it as an open string.
	Ask       string  `json:"ask"`
	SpamClaim float64 `json:"spam_claim"`
	// Confidence is the lowest confidence across the scores and the choice.
	Confidence  float64 `json:"confidence"`
	Model       string  `json:"model"`
	InputTokens int     `json:"input_tokens"`
}

// TemplateAnalysis is the AI spam analysis of a piece of copy: which word,
// which sentence, whether in the subject or the body, and what to write
// instead.
type TemplateAnalysis struct {
	// Score runs 0 to 100, higher being safer.
	Score   int    `json:"score"`
	Verdict string `json:"verdict"`
	// Findings is never nil.
	Findings []TemplateFinding `json:"findings"`
	// SuggestedSubject and Improvements are absent when only the judgment
	// could run (no language model configured).
	SuggestedSubject string   `json:"suggested_subject,omitempty"`
	Improvements     []string `json:"improvements,omitempty"`
	// Rules is the rules-based score of the same copy, from the same request.
	Rules TemplateScore `json:"rules"`
	// Judgment is how the copy reads to its recipient; nil when unavailable.
	Judgment *TemplateJudgment `json:"judgment,omitempty"`
	// Model and TokensUsed describe the analysis call.
	Model      string `json:"model"`
	TokensUsed int    `json:"tokens_used"`
	// CreditsCharged is what this call cost and CreditsRemaining the balance
	// afterwards. A self-hosted model charges nothing.
	CreditsCharged   int `json:"credits_charged"`
	CreditsRemaining int `json:"credits_remaining"`
}

// Analyze runs an AI spam analysis of arbitrary copy, which spends AI credits
// (use an idempotency key to make a retry safe). Pass at least one of subject
// and bodies; the three together may be at most 60,000 bytes. It needs the
// templates write scope and AI access, and is refused with a 503 identified
// "ai_not_configured" on a deployment without an AI provider, and a 403 without
// an active plan or trial. [TemplateService.Score] is the free rules-only check.
func (s *TemplateService) Analyze(ctx context.Context, params *TemplateScoreParams, opts ...RequestOption) (*TemplateAnalysis, *Response, error) {
	out, resp, err := send[TemplateAnalysis](ctx, s.client.post, "templates/analyze", params, opts)
	if err != nil {
		return nil, resp, err
	}
	if out.Findings == nil {
		out.Findings = []TemplateFinding{}
	}
	return out, resp, nil
}
