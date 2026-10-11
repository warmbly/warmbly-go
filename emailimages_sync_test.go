package warmbly

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

const emailImageJSON = `{
  "id": "img_1",
  "organization_id": "org_1",
  "user_id": "usr_1",
  "filename": "logo.png",
  "mime_type": "image/png",
  "size": 20480,
  "width": 600,
  "height": 200,
  "url": "https://cdn.example.com/email-images/org_1/abc.png",
  "created_at": "2026-10-01T09:00:00Z"
}`

func TestEmailImageUpload(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusCreated, "application/json", emailImageJSON, &got)

	img, resp, err := c.EmailImages.Upload(context.Background(), &FileUpload{
		Filename:    "logo.png",
		Content:     strings.NewReader("\x89PNG\r\n\x1a\n"),
		ContentType: "image/png",
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", resp.StatusCode)
	}
	assertRequest(t, got, "POST", "/v1/email-images", "")
	if name, data := multipartFile(t, got, "file"); name != "logo.png" || data != "\x89PNG\r\n\x1a\n" {
		t.Errorf("file part = %q %q", name, data)
	}
	if img.ID != "img_1" || img.UserID == nil || img.MimeType != "image/png" || img.Width != 600 || img.Size != 20480 || !strings.HasPrefix(img.URL, "https://") {
		t.Errorf("image = %+v", img)
	}
}

func TestEmailImageUploadQuota(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusBadRequest, "application/json",
		`{"error": "Bad Request", "code": "storage_limit_reached", "message": "Storage limit reached"}`, &got)
	_, _, err := c.EmailImages.Upload(context.Background(), &FileUpload{Filename: "a.png", Content: strings.NewReader("x")})
	var apiErr *Error
	if !errors.As(err, &apiErr) || !apiErr.HasCode(ErrCodeStorageLimitReached) {
		t.Fatalf("err = %v, want %s", err, ErrCodeStorageLimitReached)
	}
}

func TestEmailImageListAndDelete(t *testing.T) {
	var got capturedRequest
	// The handler's pagination object carries no total.
	c := syncServer(t, http.StatusOK, "application/json",
		`{"data": [`+emailImageJSON+`], "pagination": {"next_cursor": "c2", "has_more": true}}`, &got)
	ctx := context.Background()

	page, err := c.EmailImages.List(ctx, &ListOptions{Limit: 40})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	assertRequest(t, got, "GET", "/v1/email-images", "limit=40")
	if len(page.Data) != 1 || page.Data[0].Filename != "logo.png" || !page.HasMore() || page.Pagination.Total != nil {
		t.Errorf("page = %+v", page)
	}
	if _, err := page.Next(ctx); err != nil {
		t.Fatalf("Next: %v", err)
	}
	assertRequest(t, got, "GET", "/v1/email-images", "cursor=c2&limit=40")

	c = syncServer(t, http.StatusNoContent, "", "", &got)
	resp, err := c.EmailImages.Delete(ctx, "img_1")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	assertRequest(t, got, "DELETE", "/v1/email-images/img_1", "")
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want 204", resp.StatusCode)
	}
}

func TestAPIKeyDeletePermanent(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusOK, "application/json", `{"status": "deleted"}`, &got)
	if _, err := c.APIKeys.Delete(context.Background(), "key_1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	assertRequest(t, got, "DELETE", "/v1/api-keys/key_1/permanent", "")

	c = syncServer(t, http.StatusConflict, "application/json",
		`{"error": "Conflict", "code": "conflict", "message": "this key is still active; revoke it before deleting it"}`, &got)
	if _, err := c.APIKeys.Delete(context.Background(), "key_1"); !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v, want a conflict", err)
	}
}

func TestTemplateAnalyze(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusOK, "application/json", `{
  "score": 72,
  "verdict": "Likely to land, but the subject oversells.",
  "findings": [
    {"severity": "high", "field": "subject", "text": "FREE", "line": 1, "excerpt": "FREE demo today", "issue": "All-caps trigger word", "suggestion": "Drop it", "category": "trigger_word"},
    {"severity": "info", "issue": "Long email"}
  ],
  "suggested_subject": "A quick demo?",
  "improvements": ["Shorten the second paragraph"],
  "rules": {"score": 80, "issues": [
    {"severity": "high", "code": "trigger_terms", "message": "Spam trigger terms", "field": "subject",
     "spans": [{"field": "subject", "text": "FREE", "line": 1, "excerpt": "FREE demo today"}], "suggestion": "Remove FREE"}
  ]},
  "judgment": {"reads_as": 0.3, "personalization": 0.6, "ask": "one_clear_ask", "spam_claim": 0.1, "confidence": 0.8, "model": "jev", "input_tokens": 120},
  "model": "claude", "tokens_used": 900, "credits_remaining": 41, "credits_charged": 2
}`, &got)

	a, _, err := c.Templates.Analyze(context.Background(), &TemplateScoreParams{Subject: "FREE demo today", BodyPlain: "Hi"},
		WithIdempotencyKey("k1"))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	assertRequest(t, got, "POST", "/v1/templates/analyze", "", `"subject":"FREE demo today"`, `"body_plain":"Hi"`)
	if a.Score != 72 || len(a.Findings) != 2 || a.Findings[0].Field != TemplateFieldSubject || a.Findings[1].Severity != TemplateFindingInfo {
		t.Errorf("analysis = %+v", a)
	}
	if a.Rules.Score != 80 || len(a.Rules.Issues[0].Spans) != 1 || a.Rules.Issues[0].Spans[0].Excerpt != "FREE demo today" || a.Rules.Issues[0].Suggestion == "" {
		t.Errorf("rules = %+v", a.Rules)
	}
	if a.Judgment == nil || a.Judgment.Ask != CopyAskOneClear || a.Judgment.InputTokens != 120 || a.CreditsCharged != 2 {
		t.Errorf("judgment/credits = %+v %d", a.Judgment, a.CreditsCharged)
	}
}

func TestTemplateAnalyzeNotConfigured(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusServiceUnavailable, "application/json",
		`{"error": "Service Unavailable", "code": "ai_not_configured", "message": "AI analysis is not configured on this deployment."}`, &got)
	_, _, err := c.Templates.Analyze(context.Background(), nil)
	var apiErr *Error
	if !errors.As(err, &apiErr) || !apiErr.HasCode(ErrCodeAINotConfigured) {
		t.Fatalf("err = %v, want %s", err, ErrCodeAINotConfigured)
	}
	if got.body != "{}\n" {
		t.Errorf("nil params body = %q, want an empty object", got.body)
	}
}

func TestTemplateScoreDecodesSpans(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusOK, "application/json", `{"score": 90, "issues": [
  {"severity": "warn", "code": "empty_subject", "message": "No subject"}
]}`, &got)
	s, _, err := c.Templates.Score(context.Background(), &TemplateScoreParams{BodyPlain: "Hi"})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if len(s.Issues) != 1 || s.Issues[0].Field != "" || s.Issues[0].Spans != nil {
		t.Errorf("issues = %+v", s.Issues)
	}
}
