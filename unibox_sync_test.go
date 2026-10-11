package warmbly

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestUniboxMoveToFolder(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusOK, "application/json",
		`{"email_ids": null, "thread_ids": ["th_1", "th_2"], "folder": "archive"}`, &got)
	ctx := context.Background()

	res, _, err := c.Unibox.MoveToFolder(ctx, &UniboxFolderMove{ThreadIDs: []string{"th_1", "th_2"}, Folder: FolderArchive})
	if err != nil {
		t.Fatalf("MoveToFolder: %v", err)
	}
	assertRequest(t, got, "PATCH", "/v1/unibox/folder", "",
		`{"email_ids":[],"thread_ids":["th_1","th_2"],"folder":"archive"}`)
	if res.Folder != FolderArchive || len(res.ThreadIDs) != 2 || res.EmailIDs != nil {
		t.Errorf("echo = %+v", res)
	}

	if _, _, err := c.Unibox.MoveToFolder(ctx, &UniboxFolderMove{EmailIDs: []string{"m_1"}, Folder: FolderTrash}); err != nil {
		t.Fatalf("MoveToFolder ids: %v", err)
	}
	assertRequest(t, got, "PATCH", "/v1/unibox/folder", "", `{"email_ids":["m_1"],"folder":"trash"}`)
}

func TestUniboxMarkThreadsSeen(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusOK, "application/json", `{"email_ids": [], "thread_ids": ["th_1"], "seen": true}`, &got)
	if _, err := c.Unibox.MarkThreadsSeen(context.Background(), []string{"th_1"}, true); err != nil {
		t.Fatalf("MarkThreadsSeen: %v", err)
	}
	assertRequest(t, got, "PATCH", "/v1/unibox/seen", "", `{"email_ids":[],"thread_ids":["th_1"],"seen":true}`)
}

func TestUniboxSnoozeThreads(t *testing.T) {
	var got capturedRequest
	until := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	row := `{"id": "sn_1", "user_id": "usr_1", "thread_id": "th_1", "snoozed_until": "2026-10-20T09:00:00Z", "created_at": "2026-10-11T09:00:00Z", "updated_at": "2026-10-11T09:00:00Z"}`
	ctx := context.Background()

	c := syncServer(t, http.StatusOK, "application/json", `{"data": [`+row+`, {"id": "sn_2", "user_id": "usr_1", "thread_id": "th_2", "snoozed_until": "2026-10-20T09:00:00Z"}]}`, &got)
	rows, _, err := c.Unibox.SnoozeThreads(ctx, []string{"th_1", "th_2"}, until)
	if err != nil {
		t.Fatalf("SnoozeThreads: %v", err)
	}
	assertRequest(t, got, "POST", "/v1/unibox/snooze", "",
		`"thread_ids":["th_1","th_2"]`, `"snoozed_until":"2026-10-20T09:00:00Z"`)
	if len(rows) != 2 || rows[1].ThreadID != "th_2" {
		t.Errorf("rows = %+v", rows)
	}

	// One thread is answered with the bare snooze.
	c = syncServer(t, http.StatusOK, "application/json", row, &got)
	rows, _, err = c.Unibox.SnoozeThreads(ctx, []string{"th_1"}, until)
	if err != nil {
		t.Fatalf("SnoozeThreads one: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "sn_1" || !rows[0].SnoozedUntil.Equal(until) {
		t.Errorf("rows = %+v", rows)
	}

	c = syncServer(t, http.StatusNoContent, "", "", &got)
	if _, err := c.Unibox.UnsnoozeThreads(ctx, []string{"th_1", "th_2"}); err != nil {
		t.Fatalf("UnsnoozeThreads: %v", err)
	}
	assertRequest(t, got, "DELETE", "/v1/unibox/snooze", "thread_id=th_1%2Cth_2")
}

func TestUniboxListAutomatedAndArchived(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusOK, "application/json", `{"data": [], "pagination": {"has_more": false}}`, &got)
	if _, err := c.Unibox.List(context.Background(), &UniboxListParams{IncludeArchived: true, Automated: Bool(false)}); err != nil {
		t.Fatalf("List: %v", err)
	}
	assertRequest(t, got, "GET", "/v1/unibox", "automated=false&include_archived=true")
}

func TestUniboxReplyForward(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusOK, "application/json", `{"task_id": "t_1"}`, &got)
	if _, _, err := c.Unibox.Reply(context.Background(), &UniboxReplyParams{
		EmailAccountID:   "acc_1",
		To:               []string{"bob@acme.test"},
		Subject:          "Fwd: pricing",
		BodyPlain:        "See below",
		ForwardMessageID: "m_9",
	}); err != nil {
		t.Fatalf("Reply: %v", err)
	}
	assertRequest(t, got, "POST", "/v1/unibox/reply", "", `"forward_message_id":"m_9"`)
}

func TestUniboxDriftDecode(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusOK, "application/json", `{
  "total": 40, "unread": 5, "today": 2, "week": 9, "snoozed": 1, "awaiting_reply": 3,
  "automated": 12, "automated_unread": 4, "awaiting_agent_draft": 0, "scheduled_pending": 0, "scheduled_pending_max": 500,
  "folders": [], "mailboxes": [], "tags": [], "categories": []
}`, &got)
	o, _, err := c.Unibox.Overview(context.Background())
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if o.Automated != 12 || o.AutomatedUnread != 4 {
		t.Errorf("overview = %+v", o)
	}

	c = syncServer(t, http.StatusOK, "application/json", `{"data": [
  {"id": "m_1", "email_id": "acc_2", "thread_id": "th_1", "from_addr": ["ada@example.com"], "subject": "Re: hi",
   "internal_date": "2026-10-01T09:00:00Z", "sent_date": "2026-10-01T09:00:00Z", "seen": true, "labels": [],
   "answers_mailbox_id": "acc_1"}
], "pagination": {"has_more": false, "next_cursor": null}}`, &got)
	page, err := c.Unibox.Thread(context.Background(), "th_1", "", nil)
	if err != nil {
		t.Fatalf("Thread: %v", err)
	}
	if m := page.Data[0]; m.AnswersMailboxID == nil || *m.AnswersMailboxID != "acc_1" {
		t.Errorf("message = %+v", m)
	}
}
