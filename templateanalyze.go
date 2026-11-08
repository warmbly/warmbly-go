package warmbly

import "context"

// TemplateSpan is the exact fragment of copy behind a [TemplateIssue].
type TemplateSpan struct {
	// Field is "subject" or "body".
	Field string `json:"field"`
	// Text is the fragment as written in the copy.
	Text string `json:"text"`
	// Line is the 1-based line of that field the fragment sits on.
	Line int `json:"line,omitempty"`
	// Excerpt is the whole line, for context around the fragment.
	Excerpt string `json:"excerpt,omitempty"`
}

// TemplateFindingInfo is the informational severity of a [TemplateFinding],
// next to [TemplateIssueWarn] and [TemplateIssueHigh].
const TemplateFindingInfo = "info"

// TemplateFinding is one located problem found by [TemplateService.Analyze].
type TemplateFinding struct {
	// Severity is [TemplateIssueHigh], [TemplateIssueWarn] or
	// [TemplateFindingInfo].
	Severity string `json:"severity"`
	// Field is "subject" or "body"; empty when the finding is about the
	// email as a whole.
	Field string `json:"field,omitempty"`
	// Text is the exact fragment quoted from the copy; empty for a
	// whole-email finding.
	Text string `json:"text,omitempty"`
	// Line is the 1-based line of the field the fragment sits on.
	Line int `json:"line,omitempty"`
	// Excerpt is the whole line, for context.
	Excerpt string `json:"excerpt,omitempty"`
	// Issue is why it hurts deliverability.
	Issue string `json:"issue"`
	// Suggestion is what to write instead.
	Suggestion string `json:"suggestion,omitempty"`
	// Category is a coarse grouping such as "trigger_word", "tone",
	// "formatting", "links", "structure" or "authenticity".
	Category string `json:"category,omitempty"`
}

// TemplateJudgment is how the copy reads to its recipient, as judged by the
// instance's copy judge. It costs no credits.
type TemplateJudgment struct {
	// ReadsAs scores how bulk-sent the copy reads, 0 to 1.
	ReadsAs float64 `json:"reads_as"`
	// Personalization scores how personal the copy reads, 0 to 1.
	Personalization float64 `json:"personalization"`
	// Ask says what the copy asks for, for example "no_ask" or
	// "several_asks"; other values mean a single clear ask.
	Ask string `json:"ask"`
	// SpamClaim scores how much the copy reads as an explicit promotion.
	SpamClaim float64 `json:"spam_claim"`
	// Confidence is the lowest confidence across the judge's answers.
	Confidence  float64 `json:"confidence"`
	Model       string  `json:"model"`
	InputTokens int     `json:"input_tokens"`
}

// TemplateAnalyzeParams is the copy to analyze. Pass whichever parts exist;
// at least one must be non-blank and the total is capped at 60,000 bytes.
type TemplateAnalyzeParams struct {
	Subject   string `json:"subject,omitempty"`
	BodyHTML  string `json:"body_html,omitempty"`
	BodyPlain string `json:"body_plain,omitempty"`
}

// TemplateAnalysis is the AI spam analysis of a piece of copy.
type TemplateAnalysis struct {
	// Score is the overall 0 to 100 deliverability score; higher is safer.
	Score int `json:"score"`
	// Verdict is one plain sentence on how the email will land.
	Verdict string `json:"verdict"`
	// Findings are located problems, most severe first. Empty on an instance
	// with no AI provider, where only Rules and Judgment are filled.
	Findings []TemplateFinding `json:"findings"`
	// SuggestedSubject is a rewritten subject line, empty when the current
	// one is fine.
	SuggestedSubject string `json:"suggested_subject,omitempty"`
	// Improvements are copy-level suggestions with nothing specific to quote.
	Improvements []string `json:"improvements,omitempty"`
	// Rules is the rules-based score of the same copy, as
	// [TemplateService.Score] returns it.
	Rules TemplateScore `json:"rules"`
	// Judgment is how the copy reads to its recipient; nil when the instance
	// has no copy judge or it was unavailable.
	Judgment   *TemplateJudgment `json:"judgment,omitempty"`
	Model      string            `json:"model"`
	TokensUsed int               `json:"tokens_used"`
	// CreditsCharged is what this call cost and CreditsRemaining the
	// workspace balance afterwards; ChargedUSD and BalanceUSD are the same
	// amounts in dollars.
	CreditsCharged   int     `json:"credits_charged"`
	CreditsRemaining int     `json:"credits_remaining"`
	ChargedUSD       float64 `json:"charged_usd"`
	BalanceUSD       float64 `json:"balance_usd"`
}

// Analyze runs an AI spam analysis of the copy: which word or sentence hurts,
// whether it is in the subject or the body, and what to write instead. The
// rules-based score of the same copy comes back under Rules.
//
// It spends AI credits (reserved first, refunded on a provider failure) and
// needs both the template write scope and AI access. It answers 403 without
// an active plan or trial and 503 with the identifier "ai_not_configured" on
// an instance with no AI provider. Pass [WithIdempotencyKey] so a retry
// charges once.
func (s *TemplateService) Analyze(ctx context.Context, params *TemplateAnalyzeParams, opts ...RequestOption) (*TemplateAnalysis, *Response, error) {
	return send[TemplateAnalysis](ctx, s.client.post, "templates/analyze", params, opts)
}
