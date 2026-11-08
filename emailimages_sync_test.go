package warmbly

import (
	"context"
	"strings"
	"testing"
)

func TestEmailImagesRouting(t *testing.T) {
	ctx := context.Background()
	var got recordedRequest

	c := emailFixtureClient(t, 200, `{"data":[{"id":"i1","organization_id":"o1","filename":"logo.png","mime_type":"image/png","size":1234,"width":200,"height":100,"url":"https://x.test/email-images/o1/a.png","created_at":"2026-11-01T10:00:00Z"}],"pagination":{"next_cursor":"nx","has_more":true}}`, &got)
	page, err := c.EmailImages.List(ctx, &EmailImageListParams{ListOptions: ListOptions{Limit: 10, Cursor: "cur"}})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got.method != "GET" || got.path != "/v1/email-images" || got.rawQuery != "cursor=cur&limit=10" {
		t.Errorf("list request = %s %s ?%s", got.method, got.path, got.rawQuery)
	}
	if len(page.Data) != 1 || page.Data[0].Width != 200 || page.Data[0].UserID != nil || !page.HasMore() {
		t.Errorf("page = %+v", page)
	}
	if _, err := page.Next(ctx); err != nil || got.rawQuery != "cursor=nx&limit=10" {
		t.Errorf("Next: %v ?%s", err, got.rawQuery)
	}

	c = emailFixtureClient(t, 201, `{"id":"i2","organization_id":"o1","user_id":"u1","filename":"a.png","mime_type":"image/png","size":10,"width":1,"height":1,"url":"u","created_at":"2026-11-01T10:00:00Z"}`, &got)
	img, _, err := c.EmailImages.Upload(ctx, &FileUpload{Filename: "a.png", Content: strings.NewReader("PNGDATA"), ContentType: "image/png"})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if got.method != "POST" || got.path != "/v1/email-images" || !strings.Contains(got.body, `name="file"; filename="a.png"`) || !strings.Contains(got.body, "PNGDATA") {
		t.Errorf("upload request = %s %s %q", got.method, got.path, got.body)
	}
	if img.ID != "i2" || img.UserID == nil || *img.UserID != "u1" {
		t.Errorf("img = %+v", img)
	}

	c = emailFixtureClient(t, 204, ``, &got)
	if _, err := c.EmailImages.Delete(ctx, "i2"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got.method != "DELETE" || got.path != "/v1/email-images/i2" {
		t.Errorf("delete request = %s %s", got.method, got.path)
	}
}

func TestTemplateAnalyze(t *testing.T) {
	var got recordedRequest
	c := emailFixtureClient(t, 200, `{"score":72,"verdict":"Likely inbox","findings":[{"severity":"high","field":"subject","text":"FREE","line":1,"excerpt":"FREE offer","issue":"trigger word","suggestion":"drop it","category":"trigger_word"}],"suggested_subject":"Quick question","improvements":["shorten"],"rules":{"score":80,"issues":[{"severity":"warn","code":"caps","message":"caps","field":"subject","spans":[{"field":"subject","text":"FREE","line":1,"excerpt":"FREE offer"}],"suggestion":"lowercase"}]},"judgment":{"reads_as":0.2,"personalization":0.7,"ask":"one_ask","spam_claim":0.1,"confidence":0.9,"model":"m","input_tokens":50},"model":"claude","tokens_used":900,"credits_charged":3,"credits_remaining":97,"charged_usd":0.03,"balance_usd":0.97}`, &got)
	a, _, err := c.Templates.Analyze(context.Background(), &TemplateAnalyzeParams{Subject: "FREE offer", BodyPlain: "hi"}, WithIdempotencyKey("k1"))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if got.method != "POST" || got.path != "/v1/templates/analyze" || strings.TrimSpace(got.body) != `{"subject":"FREE offer","body_plain":"hi"}` {
		t.Errorf("request = %s %s %q", got.method, got.path, got.body)
	}
	if a.Score != 72 || a.Findings[0].Field != "subject" || a.Findings[0].Severity != TemplateIssueHigh ||
		a.Rules.Issues[0].Spans[0].Text != "FREE" || a.Rules.Issues[0].Suggestion != "lowercase" ||
		a.Judgment == nil || a.Judgment.Ask != "one_ask" || a.CreditsCharged != 3 || a.BalanceUSD != 0.97 {
		t.Errorf("analysis = %+v", a)
	}

	// With no AI provider the findings are empty and there is no suggestion.
	c = emailFixtureClient(t, 200, `{"score":90,"verdict":"ok","findings":[],"rules":{"score":90,"issues":[]},"model":"","tokens_used":0,"credits_charged":0,"credits_remaining":0,"charged_usd":0,"balance_usd":0}`, nil)
	a, _, err = c.Templates.Analyze(context.Background(), &TemplateAnalyzeParams{Subject: "x"})
	if err != nil || a.Judgment != nil || len(a.Findings) != 0 {
		t.Errorf("no-AI analysis = %+v, %v", a, err)
	}
}
