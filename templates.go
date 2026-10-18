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
	// Field is "subject" or "body" when the issue lives in exactly one of
	// them, and empty when it spans both or describes the send as a whole.
	Field string `json:"field,omitempty"`
	// Spans are the exact fragments that triggered the issue, in reading
	// order. Empty for an issue with nothing to point at.
	Spans []TemplateSpan `json:"spans,omitempty"`
	// Suggestion is the concrete fix, in one line.
	Suggestion string `json:"suggestion,omitempty"`
}

// TemplateSpan locates one exact fragment of the copy.
type TemplateSpan struct {
	// Field is "subject" or "body".
	Field string `json:"field"`
	// Text is the fragment as it is written in the copy.
	Text string `json:"text"`
	// Line is the 1-based line of that field the fragment sits on.
	Line int `json:"line,omitempty"`
	// Excerpt is that whole line, so the fragment can be shown in context.
	Excerpt string `json:"excerpt,omitempty"`
}

// TemplateFindingInfo is a [TemplateFinding.Severity] for a note rather than a
// risk, on top of [TemplateIssueWarn] and [TemplateIssueHigh].
const TemplateFindingInfo = "info"

// TemplateFinding is one located problem an AI analysis found in the copy.
type TemplateFinding struct {
	// Severity is [TemplateIssueHigh], [TemplateIssueWarn] or
	// [TemplateFindingInfo].
	Severity string `json:"severity"`
	// Field is "subject" or "body", empty when nothing in the finding could be
	// anchored in the copy.
	Field string `json:"field,omitempty"`
	// Text is the exact fragment quoted from the copy, empty when the finding
	// is about the email as a whole.
	Text string `json:"text,omitempty"`
	// Line is the 1-based line of that field the fragment sits on, and Excerpt
	// the whole line for context.
	Line    int    `json:"line,omitempty"`
	Excerpt string `json:"excerpt,omitempty"`
	// Issue is why it hurts deliverability or replies.
	Issue string `json:"issue"`
	// Suggestion is what to write instead.
	Suggestion string `json:"suggestion,omitempty"`
	// Category is a coarse grouping, for example "trigger_word", "tone",
	// "formatting", "links", "structure" or "authenticity". Treat it as
	// open-ended.
	Category string `json:"category,omitempty"`
}

// How a [CopyJudgment] found the copy's ask, in [CopyJudgment.Ask]. The set may
// grow.
const (
	CopyAskOneClear = "one_clear_ask"
	CopyAskSeveral  = "several_asks"
	CopyAskNone     = "no_ask"
)

// CopyJudgment is how the copy reads to its recipient, from a classifier that
// needs no AI credits. ReadsAs, Personalization and SpamClaim are positions
// from 0 to 1 on their rubric, with 0 the personal end.
type CopyJudgment struct {
	// ReadsAs runs from a personal note (0) to bulk mail (1).
	ReadsAs         float64 `json:"reads_as"`
	Personalization float64 `json:"personalization"`
	// Ask is one of the CopyAsk* constants.
	Ask string `json:"ask"`
	// SpamClaim is how likely the copy carries a claim a filter would object
	// to.
	SpamClaim float64 `json:"spam_claim"`
	// Confidence is the lowest confidence across the answers, so one shaky
	// answer makes the whole judgment shaky.
	Confidence  float64 `json:"confidence"`
	Model       string  `json:"model"`
	InputTokens int     `json:"input_tokens"`
}

// TemplateAnalysis is an AI spam analysis of one piece of copy: which word and
// which sentence would hurt, whether it is in the subject or the body, and what
// to write instead, next to the rules-based score of the same reading.
type TemplateAnalysis struct {
	// Score is the overall 0 to 100 deliverability score, higher being safer.
	Score int `json:"score"`
	// Verdict is one plain sentence on how the email will land.
	Verdict string `json:"verdict"`
	// Findings are located problems, most severe first. Empty for a clean
	// email, and on a deployment with no language model.
	Findings []TemplateFinding `json:"findings"`
	// SuggestedSubject is a rewritten subject line, empty when the current one
	// is fine.
	SuggestedSubject string `json:"suggested_subject,omitempty"`
	// Improvements are copy-level suggestions with nothing specific to quote.
	Improvements []string `json:"improvements,omitempty"`
	// Rules is the rules-based score from the same request, the one
	// [TemplateService.Score] returns.
	Rules TemplateScore `json:"rules"`
	// Judgment is how the copy reads to its recipient. Nil when the
	// deployment has no classifier or it was unavailable.
	Judgment *CopyJudgment `json:"judgment,omitempty"`
	// Model names the model behind the analysis, TokensUsed what it consumed.
	Model      string `json:"model"`
	TokensUsed int    `json:"tokens_used"`
	// CreditsCharged is the AI credits this call spent (0 when the instance
	// runs a local model or no language model) and CreditsRemaining the
	// balance after it.
	CreditsCharged   int `json:"credits_charged"`
	CreditsRemaining int `json:"credits_remaining"`
}

// TemplateAnalyzeParams is the copy to analyze. Pass whichever parts exist; at
// least one must hold text, and the three together may not exceed 60,000
// bytes.
type TemplateAnalyzeParams struct {
	Subject   string `json:"subject,omitempty"`
	BodyHTML  string `json:"body_html,omitempty"`
	BodyPlain string `json:"body_plain,omitempty"`
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

// Analyze asks an AI model what in the copy would push it to spam, where that
// is, and what to write instead. It works on any subject and body, not just a
// stored template, and runs the rules score of [TemplateService.Score] in the
// same request so both halves come from one reading.
//
// It spends AI credits (reserved first, refunded when the provider fails, and
// settled on real token use), so pass [WithIdempotencyKey] to make a retry
// charge once. An out-of-credits workspace gets a 402; a deployment with no AI
// configured answers 503 with code "ai_not_configured", which is permanent,
// unlike a provider outage; a workspace without an active plan or trial gets a
// 403. Requires the view-campaigns and use-ai permissions ([PermWriteTemplates]
// for an API key).
func (s *TemplateService) Analyze(ctx context.Context, params *TemplateAnalyzeParams, opts ...RequestOption) (*TemplateAnalysis, *Response, error) {
	return send[TemplateAnalysis](ctx, s.client.post, "templates/analyze", params, opts)
}
