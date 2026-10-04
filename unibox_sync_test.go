package warmbly

import (
	"context"
	"testing"
	"time"
)

func TestUniboxMoveFolder(t *testing.T) {
	var got syncCapture
	echo := MoveFolderResult{
		EmailIDs:  []string{"msg_1"},
		ThreadIDs: []string{"th_1", "th_2"},
		Folder:    FolderArchive,
	}
	c := respondingClient(t, &got, 200, jsonOf(t, echo))
	out, resp, err := c.Unibox.MoveFolder(context.Background(), &MoveFolderParams{
		EmailIDs:  []string{"msg_1"},
		ThreadIDs: []string{"th_1", "th_2"},
		Folder:    FolderArchive,
	})
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "PATCH", "/v1/unibox/folder", "")
	got.wantBody(t, `{"email_ids":["msg_1"],"thread_ids":["th_1","th_2"],"folder":"archive"}`)
	if resp.StatusCode != 200 || out.Folder != FolderArchive || len(out.ThreadIDs) != 2 || out.EmailIDs[0] != "msg_1" {
		t.Errorf("result = %+v", out)
	}

	// An echo with no message ids (a thread-only filing) serializes null.
	c = respondingClient(t, &got, 200, `{"email_ids":null,"thread_ids":["th_1"],"folder":"trash"}`)
	out, _, err = c.Unibox.MoveFolder(context.Background(), &MoveFolderParams{ThreadIDs: []string{"th_1"}, Folder: FolderTrash})
	if err != nil {
		t.Fatal(err)
	}
	got.wantBody(t, `{"thread_ids":["th_1"],"folder":"trash"}`)
	if len(out.EmailIDs) != 0 || out.Folder != FolderTrash {
		t.Errorf("result = %+v", out)
	}
}

func TestUniboxMarkThreadsSeen(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, `{}`)
	if _, err := c.Unibox.MarkThreadsSeen(context.Background(), []string{"th_1"}, true); err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "PATCH", "/v1/unibox/seen", "")
	got.wantBody(t, `{"thread_ids":["th_1"],"seen":true}`)
}

func TestUniboxSnoozeMany(t *testing.T) {
	until := time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC)
	row := func(id string) UniboxSnooze { return UniboxSnooze{ID: "sn_" + id, ThreadID: id, SnoozedUntil: until} }

	t.Run("several threads answer a data list", func(t *testing.T) {
		var got syncCapture
		c := respondingClient(t, &got, 200, jsonOf(t, map[string]any{"data": []UniboxSnooze{row("th_1"), row("th_2")}}))
		out, _, err := c.Unibox.SnoozeMany(context.Background(), []string{"th_1", "th_2"}, until)
		if err != nil {
			t.Fatal(err)
		}
		got.wantRequest(t, "POST", "/v1/unibox/snooze", "")
		got.wantBody(t, `{"thread_ids":["th_1","th_2"],"snoozed_until":"2026-09-10T09:00:00Z"}`)
		if len(out) != 2 || out[1].ThreadID != "th_2" {
			t.Errorf("rows = %+v", out)
		}
	})
	t.Run("one thread answers the bare row", func(t *testing.T) {
		var got syncCapture
		c := respondingClient(t, &got, 200, jsonOf(t, row("th_1")))
		out, _, err := c.Unibox.SnoozeMany(context.Background(), []string{"th_1"}, until)
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != 1 || out[0].ThreadID != "th_1" {
			t.Errorf("rows = %+v", out)
		}
	})
	t.Run("unsnooze joins ids", func(t *testing.T) {
		var got syncCapture
		c := respondingClient(t, &got, 204, "")
		if _, err := c.Unibox.UnsnoozeMany(context.Background(), []string{"th_1", "th_2"}); err != nil {
			t.Fatal(err)
		}
		got.wantRequest(t, "DELETE", "/v1/unibox/snooze", "thread_id=th_1%2Cth_2")
	})
}

func TestUniboxListNewFilters(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, `{"data":[],"pagination":{"next_cursor":null,"has_more":false}}`)
	_, err := c.Unibox.List(context.Background(), &UniboxListParams{
		IncludeArchived: true,
		Automated:       Bool(false),
	})
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "GET", "/v1/unibox", "automated=false&include_archived=true")

	if _, err := c.Unibox.List(context.Background(), &UniboxListParams{Automated: Bool(true)}); err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "GET", "/v1/unibox", "automated=true")
}

func TestUniboxReplyForwardsAMessage(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, `{"task_id":"t_1","scheduled_at":"2026-09-03T12:00:00Z","send_mode":"instant"}`)
	_, _, err := c.Unibox.Reply(context.Background(), &UniboxReplyParams{
		EmailAccountID:   "acc_1",
		To:               []string{"a@b.com"},
		Subject:          "Fwd: hi",
		BodyHTML:         "<p>fyi</p>",
		ForwardMessageID: "msg_9",
	})
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "POST", "/v1/unibox/reply", "")
	got.wantBody(t, `{"email_account_id":"acc_1","to":["a@b.com"],"subject":"Fwd: hi","body_html":"<p>fyi</p>","forward_message_id":"msg_9"}`)
}

func TestUniboxOverviewAndMessageDecodeNewFields(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, jsonOf(t, UniboxOverview{Total: 10, Automated: 4, AutomatedUnread: 1}))
	ov, _, err := c.Unibox.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ov.Automated != 4 || ov.AutomatedUnread != 1 {
		t.Errorf("overview = %+v", ov)
	}

	answers := "acc_7"
	c = respondingClient(t, &got, 200, jsonOf(t, map[string]any{
		"data":       []UniboxMessage{{ID: "m1", EmailID: "acc_1", AnswersMailboxID: &answers}},
		"pagination": Pagination{},
	}))
	page, err := c.Unibox.Thread(context.Background(), "th_1", "acc_1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 1 || page.Data[0].AnswersMailboxID == nil || *page.Data[0].AnswersMailboxID != "acc_7" {
		t.Errorf("messages = %+v", page.Data)
	}
}
