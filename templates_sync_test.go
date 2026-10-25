package warmbly

import (
	"context"
	"testing"
)

func TestTemplateAnalyze(t *testing.T) {
	var got syncCapture
	analysis := TemplateAnalysis{
		Score:   62,
		Verdict: "Reads like bulk mail with one risky claim.",
		Findings: []TemplateFinding{{
			Severity: TemplateIssueHigh, Field: "subject", Text: "FREE money", Line: 1, Excerpt: "FREE money now",
			Issue: "All caps and a giveaway claim", Suggestion: "Ask a plain question", Category: "trigger_word",
		}, {
			Severity: TemplateFindingInfo, Issue: "No unsubscribe line",
		}},
		SuggestedSubject: "Quick question about Acme",
		Improvements:     []string{"Cut the second link"},
		Rules: TemplateScore{Score: 70, Issues: []TemplateIssue{{
			Severity: TemplateIssueWarn, Code: "trigger_terms", Message: "2 spam terms", Field: "body",
			Spans:      []TemplateSpan{{Field: "body", Text: "act now", Line: 3, Excerpt: "Please act now"}},
			Suggestion: "Remove them",
		}}},
		Judgment:         &CopyJudgment{ReadsAs: 0.8, Personalization: 0.2, Ask: CopyAskSeveral, SpamClaim: 0.1, Confidence: 0.9, Model: "judge-1", InputTokens: 300},
		Model:            "writer-1",
		TokensUsed:       812,
		CreditsCharged:   2,
		CreditsRemaining: 98,
	}
	c := respondingClient(t, &got, 200, jsonOf(t, analysis))
	out, resp, err := c.Templates.Analyze(context.Background(), &TemplateAnalyzeParams{
		Subject:  "FREE money",
		BodyHTML: "<p>Please act now</p>",
	}, WithIdempotencyKey("idem-1"))
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "POST", "/v1/templates/analyze", "")
	got.wantBody(t, `{"subject":"FREE money","body_html":"<p>Please act now</p>"}`)
	if got.idempotency != "idem-1" {
		t.Errorf("Idempotency-Key = %q, want idem-1", got.idempotency)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
	if out.Score != 62 || out.CreditsCharged != 2 || out.CreditsRemaining != 98 || out.SuggestedSubject == "" {
		t.Errorf("analysis = %+v", out)
	}
	if len(out.Findings) != 2 || out.Findings[0].Field != "subject" || out.Findings[0].Line != 1 || out.Findings[1].Severity != TemplateFindingInfo {
		t.Errorf("findings = %+v", out.Findings)
	}
	if len(out.Rules.Issues) != 1 || out.Rules.Issues[0].Field != "body" || out.Rules.Issues[0].Spans[0].Text != "act now" {
		t.Errorf("rules = %+v", out.Rules)
	}
	if out.Judgment == nil || out.Judgment.Ask != CopyAskSeveral || out.Judgment.ReadsAs != 0.8 {
		t.Errorf("judgment = %+v", out.Judgment)
	}
}

// TestTemplateAnalyzeWithoutModelDecodes covers the deployment that has a
// classifier but no language model: no findings, a verdict, no model fields.
func TestTemplateAnalyzeWithoutModelDecodes(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, `{"score":88,"verdict":"Reads as a personal note.","findings":[],"rules":{"score":88,"issues":[]},"judgment":{"reads_as":0.1,"personalization":0.7,"ask":"some_new_ask","spam_claim":0,"confidence":0.8,"model":"j","input_tokens":10},"model":"j","tokens_used":10,"credits_remaining":0,"credits_charged":0}`)
	out, _, err := c.Templates.Analyze(context.Background(), &TemplateAnalyzeParams{BodyPlain: "Hi Ada"})
	if err != nil {
		t.Fatal(err)
	}
	got.wantBody(t, `{"body_plain":"Hi Ada"}`)
	if len(out.Findings) != 0 || out.Judgment == nil || out.Judgment.Ask != "some_new_ask" || out.CreditsCharged != 0 {
		t.Errorf("analysis = %+v", out)
	}
}

func TestTemplateScoreDecodesLocatedIssues(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, `{"score":70,"issues":[{"severity":"warn","code":"caps","message":"m","field":"subject","spans":[{"field":"subject","text":"BUY","line":1,"excerpt":"BUY NOW"}],"suggestion":"Lowercase it"}]}`)
	out, _, err := c.Templates.Score(context.Background(), &TemplateScoreParams{Subject: "BUY NOW"})
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "POST", "/v1/templates/score", "")
	if i := out.Issues[0]; i.Field != "subject" || i.Spans[0].Text != "BUY" || i.Suggestion != "Lowercase it" {
		t.Errorf("issue = %+v", i)
	}
}
