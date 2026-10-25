package warmbly

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestAnalyticsDirect(t *testing.T) {
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	fixture := DirectMailAnalytics{
		Period: Period30Days,
		Volume: DirectMailVolume{Sent: 40, Received: 12, ThreadsStarted: 30, Replied: 9, ReplyRate: 0.3, Bounced: 2, MedianReplyMinutes: 95},
		Tracking: DirectMailTracking{
			MailboxesOptedIn: 1, MailboxesTotal: 3, TrackedSent: 10, Opened: 6, MachineOpened: 2, Clicked: 1, OpenRate: 0.6, ClickRate: 0.1,
		},
		DailyTrend:  []DirectMailDailyStat{{Date: day, Sent: 4, Received: 1}},
		Mailboxes:   []DirectMailMailboxStats{{EmailAccountID: "acc_1", Email: "sam@acme.com", TrackDirectMail: true, Sent: 40, Received: 12}},
		TopContacts: []DirectMailContact{{Email: "ada@x.io", Sent: 5, Received: 3, LastAt: day}},
	}
	var got syncCapture
	c := respondingClient(t, &got, 200, jsonOf(t, fixture))
	out, _, err := c.Analytics.Direct(context.Background(), Period30Days)
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "GET", "/v1/analytics/direct", "period=30d")
	if out.Period != "30d" || out.Volume.MedianReplyMinutes != 95 || out.Tracking.MailboxesOptedIn != 1 || out.Tracking.MachineOpened != 2 ||
		len(out.DailyTrend) != 1 || !out.DailyTrend[0].Date.Equal(day) || !out.Mailboxes[0].TrackDirectMail ||
		out.TopContacts[0].Email != "ada@x.io" || !out.TopContacts[0].LastAt.Equal(day) {
		t.Errorf("analytics = %+v", out)
	}

	if _, _, err := c.Analytics.Direct(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "GET", "/v1/analytics/direct", "")
}

func TestAnalyticsInboxTagging(t *testing.T) {
	created := time.Date(2026, 9, 2, 10, 30, 0, 0, time.UTC)
	ret := "2026-10-01"
	next := "off_50"
	rows := []InboxTagResult{{
		ID: "tag_1", MessageID: "m_1", ThreadID: "th_1",
		Kind: InboxKindAutoReplyOOO, KindConfidence: 0.99, KindSource: "header",
		Intent: InboxIntentUnclear, IntentConfidence: 0.4, Relevance: 20, Priority: InboxPriorityWhenever,
		NeedsReview: true, ReviewReason: "low intent confidence",
		Labels: []string{"Out of office"}, Answers: json.RawMessage(`{"signal":true}`),
		Model: "tagger-1", InputTokens: 120, Actions: []string{"hold"}, ReturnDate: &ret, CreatedAt: created,
	}}
	body := jsonOf(t, map[string]any{
		"enabled":    true,
		"data":       rows,
		"total":      130,
		"summary":    InboxTagSummary{Total: 130, NeedsReview: 12, FromOffline: 40, Acted: 5},
		"pagination": Pagination{HasMore: true, NextCursor: &next},
	})
	var got syncCapture
	c := respondingClient(t, &got, 200, body)
	page, err := c.Analytics.InboxTagging(context.Background(), &InboxTaggingParams{
		ListOptions: ListOptions{Limit: 50, Cursor: "off_0"},
		NeedsReview: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "GET", "/v1/analytics/inbox-tagging", "cursor=off_0&limit=50&needs_review=true")
	if !page.Enabled || page.Total != 130 || page.Summary.NeedsReview != 12 || page.Summary.FromOffline != 40 || !page.HasMore() || page.NextCursor() != "off_50" {
		t.Errorf("page = %+v", page)
	}
	r := page.Data[0]
	if r.Kind != InboxKindAutoReplyOOO || r.KindSource != "header" || !r.NeedsReview || r.ReturnDate == nil || *r.ReturnDate != "2026-10-01" ||
		!r.CreatedAt.Equal(created) || string(r.Answers) != `{"signal":true}` || r.Actions[0] != "hold" {
		t.Errorf("row = %+v", r)
	}
}

func TestAnalyticsInboxTaggingDisabledAndNextPage(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, `{"enabled":false,"data":[],"total":0,"summary":{"total":0,"needs_review":0,"from_offline":0,"acted":0},"pagination":{"next_cursor":null,"has_more":false}}`)
	page, err := c.Analytics.InboxTagging(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "GET", "/v1/analytics/inbox-tagging", "")
	if page.Enabled || len(page.Data) != 0 || page.HasMore() {
		t.Errorf("page = %+v", page)
	}
	if _, err := page.Next(context.Background()); !errors.Is(err, ErrNoMorePages) {
		t.Errorf("Next = %v, want ErrNoMorePages", err)
	}

	// Following a cursor keeps the filter and moves the cursor.
	c = respondingClient(t, &got, 200, `{"enabled":true,"data":[{"id":"tag_1","answers":{},"labels":[],"actions":[],"return_date":null,"created_at":"2026-09-02T10:30:00Z"}],"total":2,"summary":{"total":2,"needs_review":1,"from_offline":0,"acted":0},"pagination":{"next_cursor":"off_1","has_more":true}}`)
	page, err = c.Analytics.InboxTagging(context.Background(), &InboxTaggingParams{NeedsReview: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Next(context.Background()); err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "GET", "/v1/analytics/inbox-tagging", "cursor=off_1&needs_review=true")
}

func TestAnalyticsInboxTaggingUnknownKindDecodes(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, `{"enabled":true,"data":[{"id":"x","kind":"brand_new_kind","intent":"new_intent","priority":"urgent","labels":null,"answers":{},"actions":null,"created_at":"2026-09-02T10:30:00Z"}],"total":1,"summary":{},"pagination":{"has_more":false}}`)
	page, err := c.Analytics.InboxTagging(context.Background(), nil)
	if err != nil {
		t.Fatalf("unknown enum values must decode: %v", err)
	}
	if r := page.Data[0]; r.Kind != "brand_new_kind" || r.Priority != "urgent" {
		t.Errorf("row = %+v", r)
	}
}
