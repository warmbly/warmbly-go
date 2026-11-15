package warmbly

import (
	"context"
	"strings"
	"testing"
)

// checkRequest asserts the method, path, raw query and body of a recorded
// request. An empty wantBody asserts an empty body.
func checkRequest(t *testing.T, got recordedRequest, method, path, rawQuery, wantBody string) {
	t.Helper()
	if got.method != method || got.path != path {
		t.Errorf("got %s %s, want %s %s", got.method, got.path, method, path)
	}
	if got.rawQuery != rawQuery {
		t.Errorf("query = %q, want %q", got.rawQuery, rawQuery)
	}
	if strings.TrimSpace(got.body) != wantBody {
		t.Errorf("body = %s, want %s", got.body, wantBody)
	}
}

const reportFixture = `{
	"id": "rep_1",
	"created_by": null,
	"name": "Acme monthly",
	"access": {"scope": "restricted", "campaign_folder_ids": [], "campaign_ids": ["camp_1"], "email_account_ids": []},
	"sections": {"overview": true, "campaigns": true, "replies": false, "positive_replies": true, "mailboxes": false, "daily_chart": true},
	"show_reply_snippets": false,
	"date_range": "last_month",
	"link": {"hint": "Xy7a", "created_at": "2026-11-01T10:00:00Z", "expires_at": null, "expired": false},
	"schedule": {"recipients": ["cto@acme.test"], "cadence": "monthly", "weekday": 0, "month_day": 3, "hour": 9,
		"timezone": "Europe/London", "paused": false, "last_sent_at": null, "next_run_at": "2026-12-03T09:00:00Z"},
	"created_at": "2026-10-01T10:00:00Z",
	"updated_at": "2026-11-01T10:00:00Z"
}`

func TestReportRouting(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name                         string
		call                         func(c *Client) error
		method, path, query, reqBody string
	}{
		{"Get", func(c *Client) error { _, _, e := c.Reports.Get(ctx, "rep_1"); return e },
			"GET", "/v1/reports/rep_1", "", ""},
		{"Create", func(c *Client) error {
			_, _, e := c.Reports.Create(ctx, &ReportCreateParams{
				Name:      "Acme monthly",
				Access:    &ReportAccess{Scope: ReportAccessRestricted, CampaignIDs: []string{"camp_1"}},
				DateRange: String(ReportLastMonth),
			})
			return e
		}, "POST", "/v1/reports", "",
			`{"name":"Acme monthly","access":{"scope":"restricted","campaign_folder_ids":null,"campaign_ids":["camp_1"],"email_account_ids":null},"date_range":"last_month"}`},
		{"Update", func(c *Client) error {
			_, _, e := c.Reports.Update(ctx, "rep_1", &ReportUpdateParams{ShowReplySnippets: Bool(true)})
			return e
		}, "PATCH", "/v1/reports/rep_1", "", `{"show_reply_snippets":true}`},
		{"RevokeLink", func(c *Client) error { _, _, e := c.Reports.RevokeLink(ctx, "rep_1"); return e },
			"DELETE", "/v1/reports/rep_1/link", "", ""},
		{"SetSchedule", func(c *Client) error {
			_, _, e := c.Reports.SetSchedule(ctx, "rep_1", &ReportScheduleParams{
				Recipients: []string{"cto@acme.test"}, Cadence: ReportMonthly, MonthDay: 3, Hour: 9, Timezone: "Europe/London",
			})
			return e
		}, "PUT", "/v1/reports/rep_1/schedule", "",
			`{"recipients":["cto@acme.test"],"cadence":"monthly","weekday":0,"month_day":3,"hour":9,"timezone":"Europe/London","paused":false}`},
		{"DeleteSchedule", func(c *Client) error { _, _, e := c.Reports.DeleteSchedule(ctx, "rep_1"); return e },
			"DELETE", "/v1/reports/rep_1/schedule", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got recordedRequest
			c := recordingFixtureClient(t, &got, reportFixture)
			if err := tc.call(c); err != nil {
				t.Fatalf("call: %v", err)
			}
			checkRequest(t, got, tc.method, tc.path, tc.query, tc.reqBody)
		})
	}
}

func TestReportDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, reportFixture)
	rep, _, err := c.Reports.Get(context.Background(), "rep_1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rep.CreatedBy != nil || rep.Access.Scope != ReportAccessRestricted || len(rep.Access.CampaignIDs) != 1 {
		t.Errorf("report = %+v", rep)
	}
	if !rep.Sections.PositiveReplies || rep.Sections.Replies {
		t.Errorf("sections = %+v", rep.Sections)
	}
	if rep.Link == nil || rep.Link.Hint != "Xy7a" || rep.Link.ExpiresAt != nil {
		t.Errorf("link = %+v", rep.Link)
	}
	if rep.Schedule == nil || rep.Schedule.Cadence != ReportMonthly || rep.Schedule.LastSentAt != nil || rep.Schedule.NextRunAt.IsZero() {
		t.Errorf("schedule = %+v", rep.Schedule)
	}
}

func TestReportList(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"data": [`+reportFixture+`], "pagination": {"next_cursor": "cur_2", "has_more": true}}`)
	page, err := c.Reports.List(context.Background(), &ListOptions{Limit: 10, Cursor: "cur_1"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	checkRequest(t, got, "GET", "/v1/reports", "cursor=cur_1&limit=10", "")
	if len(page.Data) != 1 || page.Data[0].Name != "Acme monthly" || !page.HasMore() || page.NextCursor() != "cur_2" {
		t.Errorf("page = %+v", page)
	}
	if page.Pagination.Total != nil {
		t.Errorf("total should be absent, got %v", *page.Pagination.Total)
	}
}

func TestReportPreview(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{
		"name": "Acme monthly", "date_range": "last_7_days", "from": "2026-11-09", "to": "2026-11-15",
		"generated_at": "2026-11-15T12:00:00Z",
		"sections": {"overview": true, "campaigns": true, "replies": true, "positive_replies": false, "mailboxes": true, "daily_chart": true},
		"show_reply_snippets": true,
		"overview": {"sent": 120, "opened": 40, "open_tracked_sent": 100, "open_rate": 0.4, "clicked": 3, "replied": 6, "reply_rate": 0.05, "bounced": 1, "bounce_rate": 0.008},
		"daily": [{"date": "2026-11-09", "sent": 20, "opens": 7, "replies": 1}],
		"campaigns": [{"name": "Q4", "status": "running", "sent": 120, "opens": 40, "open_tracked_sent": 100, "open_rate": 0.4, "clicks": 3, "replies": 6, "reply_rate": 0.05, "bounces": 1, "bounce_rate": 0.008}],
		"replies": [{"name": "Ana", "email": "ana@x.test", "campaign": "Q4", "subject": "Re: hi", "replied_at": "2026-11-10T08:00:00Z", "classification": "interested", "snippet": "Sounds good"}],
		"mailboxes": {"total": 4, "healthy": 3, "warning": 1, "error": 0, "warmup_sent": 50, "warmup_received": 48, "warmup_replied": 12}
	}`)
	view, _, err := c.Reports.Preview(context.Background(), "rep_1", ReportLast7Days)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	checkRequest(t, got, "GET", "/v1/reports/rep_1/preview", "range=last_7_days", "")
	if view.Overview == nil || view.Overview.Sent != 120 || view.Overview.Positive != nil {
		t.Errorf("overview = %+v", view.Overview)
	}
	if len(view.Campaigns) != 1 || view.Campaigns[0].Positive != nil || len(view.Replies) != 1 || view.Replies[0].Snippet != "Sounds good" {
		t.Errorf("view = %+v", view)
	}
	if view.Mailboxes == nil || view.Mailboxes.Warning != 1 {
		t.Errorf("mailboxes = %+v", view.Mailboxes)
	}

	got = recordedRequest{}
	if _, _, err := c.Reports.Preview(context.Background(), "rep_1", ""); err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if got.rawQuery != "" {
		t.Errorf("an empty range must send no query, got %q", got.rawQuery)
	}
}

func TestReportCreateLink(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{
		"url": "https://portal.acme.test/r/tok_abc", "token": "tok_abc",
		"link": {"hint": "tok_", "created_at": "2026-11-15T12:00:00Z", "expires_at": "2027-02-13T12:00:00Z", "expired": false},
		"report": `+reportFixture+`
	}`)
	res, _, err := c.Reports.CreateLink(context.Background(), "rep_1", &ReportLinkParams{ExpiresInDays: Int(90)})
	if err != nil {
		t.Fatalf("CreateLink: %v", err)
	}
	checkRequest(t, got, "POST", "/v1/reports/rep_1/link", "", `{"expires_in_days":90}`)
	if res.URL == "" || res.Token != "tok_abc" || res.Link.ExpiresAt == nil || res.Report == nil || res.Report.ID != "rep_1" {
		t.Errorf("result = %+v", res)
	}

	got = recordedRequest{}
	if _, _, err := c.Reports.CreateLink(context.Background(), "rep_1", nil); err != nil {
		t.Fatalf("CreateLink: %v", err)
	}
	checkRequest(t, got, "POST", "/v1/reports/rep_1/link", "", `{}`)
}

func TestReportDelete(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	if _, err := c.Reports.Delete(context.Background(), "rep_1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	checkRequest(t, got, "DELETE", "/v1/reports/rep_1", "", "")
}
