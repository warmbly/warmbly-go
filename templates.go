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

// Parts of a message, returned in [TemplateIssue.Field], [TemplateSpan.Field]
// and [TemplateFinding.Field].
const (
	TemplateFieldSubject = "subject"
	TemplateFieldBody    = "body"
)

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
	// order. Empty when there is nothing to point at, such as an empty
	// subject.
	Spans []TemplateSpan `json:"spans,omitempty"`
	// Suggestion is the concrete fix, in one line.
	Suggestion string `json:"suggestion,omitempty"`
}

// TemplateSpan is one fragment of the copy an issue points at.
type TemplateSpan struct {
	// Field is [TemplateFieldSubject] or [TemplateFieldBody].
	Field string `json:"field"`
	// Text is the fragment as it is written in the copy.
	Text string `json:"text"`
	// Line is the 1-based line the fragment sits on within that field.
	Line int `json:"line,omitempty"`
	// Excerpt is that whole line, to show the fragment in context.
	Excerpt string `json:"excerpt,omitempty"`
}

// Severities returned in [TemplateFinding.Severity].
const (
	TemplateFindingHigh = "high"
	TemplateFindingWarn = "warn"
	TemplateFindingInfo = "info"
)

// TemplateFinding is one located problem from the AI half of
// [TemplateService.Analyze].
type TemplateFinding struct {
	// Severity is one of the TemplateFinding* constants.
	Severity string `json:"severity"`
	// Field is [TemplateFieldSubject] or [TemplateFieldBody], empty when the
	// finding could not be anchored in the copy.
	Field string `json:"field,omitempty"`
	// Text is the exact fragment quoted from the copy, empty when the finding
	// is about the email as a whole. Line and Excerpt locate it.
	Text    string `json:"text,omitempty"`
	Line    int    `json:"line,omitempty"`
	Excerpt string `json:"excerpt,omitempty"`
	// Issue is why it hurts deliverability; Suggestion is what to write
	// instead.
	Issue      string `json:"issue"`
	Suggestion string `json:"suggestion,omitempty"`
	// Category is a coarse grouping, for example "trigger_word", "tone",
	// "formatting", "links", "structure" or "authenticity".
	Category string `json:"category,omitempty"`
}

// How many things copy asks the reader for, returned in [CopyJudgment.Ask].
const (
	CopyAskOneClear = "one_clear_ask"
	CopyAskSeveral  = "several_asks"
	CopyAskNone     = "no_ask"
)

// CopyJudgment is a model's reading of how copy lands with its recipient,
// scored without spending AI credits. The scores run 0 to 1.
type CopyJudgment struct {
	// ReadsAs is how much the copy reads as bulk mail rather than a personal
	// note.
	ReadsAs         float64 `json:"reads_as"`
	Personalization float64 `json:"personalization"`
	// Ask is one of the CopyAsk* constants.
	Ask string `json:"ask"`
	// SpamClaim is how strongly the copy makes a claim a filter objects to.
	SpamClaim float64 `json:"spam_claim"`
	// Confidence is the lowest confidence across the answers.
	Confidence  float64 `json:"confidence"`
	Model       string  `json:"model"`
	InputTokens int     `json:"input_tokens"`
}

// ErrCodeAINotConfigured is returned (503) by [TemplateService.Analyze] when
// the deployment has no AI analysis configured at all, which unlike an outage
// is permanent for that deployment.
const ErrCodeAINotConfigured = "ai_not_configured"

// TemplateAnalysis is the result of [TemplateService.Analyze]: located
// findings with suggested rewrites, the rules score of the same copy, and a
// judgment of how it reads.
type TemplateAnalysis struct {
	// Score is the overall 0-100 deliverability score, higher being safer.
	Score   int    `json:"score"`
	Verdict string `json:"verdict"`
	// Findings are located problems, most severe first. Empty when the
	// deployment has no AI provider and only the judgment ran.
	Findings []TemplateFinding `json:"findings"`
	// SuggestedSubject is a rewritten subject, empty when the current one is
	// fine.
	SuggestedSubject string `json:"suggested_subject,omitempty"`
	// Improvements are copy-level suggestions with nothing specific to quote.
	Improvements []string `json:"improvements,omitempty"`
	// Rules is the rules-based score ([TemplateService.Score]) of the same
	// copy.
	Rules TemplateScore `json:"rules"`
	// Judgment is nil when the judging service is unavailable.
	Judgment         *CopyJudgment `json:"judgment,omitempty"`
	Model            string        `json:"model"`
	TokensUsed       int           `json:"tokens_used"`
	CreditsRemaining int           `json:"credits_remaining"`
	CreditsCharged   int           `json:"credits_charged"`
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

// Analyze runs the AI half of the spam check on arbitrary copy: which word or
// sentence, in the subject or the body, would push it to spam, and what to
// write instead, together with the rules score of the same copy. At least one
// part must be non-empty and the three together at most 60,000 bytes.
//
// It spends AI credits ([ErrCodeInsufficientCredits] when the balance is
// short) and needs an active plan or trial; a provider failure refunds them.
// Pass [WithIdempotencyKey] so a retried request is not charged twice. A
// self-hosted deployment with only the judging service configured answers
// with the rules score and the judgment and charges nothing; one with neither
// answers 503 with [ErrCodeAINotConfigured].
//
// Requires [PermWriteTemplates]: it writes no template, but a read-only key
// must not be able to spend credits.
func (s *TemplateService) Analyze(ctx context.Context, params *TemplateScoreParams, opts ...RequestOption) (*TemplateAnalysis, *Response, error) {
	if params == nil {
		params = &TemplateScoreParams{}
	}
	return send[TemplateAnalysis](ctx, s.client.post, "templates/analyze", params, opts)
}
