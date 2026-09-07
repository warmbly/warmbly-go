package warmbly

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// selfhostServer answers every request with status and body, recording the
// last request, for the pool-link and cloud-link responses the fixed
// routingClient envelope cannot stand in for.
func selfhostServer(t *testing.T, status int, body string, got *recordedRequest) *Client {
	t.Helper()
	return selfhostServerFunc(t, func(w http.ResponseWriter, r *http.Request) {
		if got != nil {
			got.method = r.Method
			got.path = r.URL.Path
			got.rawQuery = r.URL.RawQuery
			raw, _ := io.ReadAll(r.Body)
			got.body = string(raw)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
}

func selfhostServerFunc(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"), WithMaxRetries(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// TestSelfhostSyncRouting asserts method, path and (where one is sent) body
// for every PoolLinkService and CloudLinkService method.
func TestSelfhostSyncRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()

	cases := []struct {
		name       string
		call       func() error
		wantMethod string
		wantPath   string
		wantBody   string // "" skips the body check
	}{
		// Pool link: public handshake.
		{"poolLink.StartCode", func() error {
			_, _, e := c.PoolLink.StartCode(ctx, &PoolLinkStartParams{InstanceName: "box", InstanceURL: "https://box.example", InstanceVersion: "1.2.3"})
			return e
		}, "POST", "/v1/pool-link/codes", `{"instance_name":"box","instance_url":"https://box.example","instance_version":"1.2.3"}`},
		{"poolLink.Poll", func() error { _, _, e := c.PoolLink.Poll(ctx, "dev_1"); return e },
			"POST", "/v1/pool-link/poll", `{"device_code":"dev_1"}`},

		// Pool link: member side.
		{"poolLink.DescribeCode", func() error { _, _, e := c.PoolLink.DescribeCode(ctx, "ABCD-EFGH"); return e },
			"GET", "/v1/pool-link/codes/ABCD-EFGH", ""},
		{"poolLink.ApproveCode", func() error { _, _, e := c.PoolLink.ApproveCode(ctx, "ABCD-EFGH", "org_1"); return e },
			"POST", "/v1/pool-link/codes/ABCD-EFGH/approve", `{"organization_id":"org_1"}`},
		{"poolLink.ApproveCode(session org)", func() error { _, _, e := c.PoolLink.ApproveCode(ctx, "ABCD-EFGH", ""); return e },
			"POST", "/v1/pool-link/codes/ABCD-EFGH/approve", `{}`},
		{"poolLink.DenyCode", func() error { _, e := c.PoolLink.DenyCode(ctx, "ABCD-EFGH"); return e },
			"POST", "/v1/pool-link/codes/ABCD-EFGH/deny", ""},
		{"poolLink.ListInstances", func() error { _, _, e := c.PoolLink.ListInstances(ctx); return e },
			"GET", "/v1/pool-link/instances", ""},
		{"poolLink.RevokeInstance", func() error { _, e := c.PoolLink.RevokeInstance(ctx, "inst_1"); return e },
			"DELETE", "/v1/pool-link/instances/inst_1", ""},

		// Cloud link: the link itself.
		{"cloudLink.Status", func() error { _, _, e := c.CloudLink.Status(ctx); return e },
			"GET", "/v1/cloud-link", ""},
		{"cloudLink.Connect", func() error { _, _, e := c.CloudLink.Connect(ctx, "https://api.warmbly.com"); return e },
			"POST", "/v1/cloud-link/connect", `{"cloud_url":"https://api.warmbly.com"}`},
		{"cloudLink.Connect(default)", func() error { _, _, e := c.CloudLink.Connect(ctx, ""); return e },
			"POST", "/v1/cloud-link/connect", `{}`},
		{"cloudLink.PollConnect", func() error { _, _, e := c.CloudLink.PollConnect(ctx); return e },
			"POST", "/v1/cloud-link/connect/poll", ""},
		{"cloudLink.Disconnect", func() error { _, e := c.CloudLink.Disconnect(ctx); return e },
			"DELETE", "/v1/cloud-link", ""},

		// Cloud link: enrollment.
		{"cloudLink.ListMailboxes", func() error { _, _, e := c.CloudLink.ListMailboxes(ctx); return e },
			"GET", "/v1/cloud-link/mailboxes", ""},
		{"cloudLink.EnrollMailbox", func() error { _, _, e := c.CloudLink.EnrollMailbox(ctx, "em_1"); return e },
			"POST", "/v1/cloud-link/mailboxes/em_1/enroll", ""},
		{"cloudLink.UnenrollMailbox", func() error { _, e := c.CloudLink.UnenrollMailbox(ctx, "em_1"); return e },
			"DELETE", "/v1/cloud-link/mailboxes/em_1/enroll", ""},
		{"cloudLink.PauseMailbox", func() error { _, _, e := c.CloudLink.PauseMailbox(ctx, "em_1"); return e },
			"POST", "/v1/cloud-link/mailboxes/em_1/pause", ""},
		{"cloudLink.ResumeMailbox", func() error { _, _, e := c.CloudLink.ResumeMailbox(ctx, "em_1"); return e },
			"POST", "/v1/cloud-link/mailboxes/em_1/resume", ""},

		// Cloud link: mailboxes signed in through Warmbly Cloud.
		{"cloudLink.StartOAuth", func() error { _, _, e := c.CloudLink.StartOAuth(ctx, ProviderGmail); return e },
			"POST", "/v1/cloud-link/oauth/start", `{"provider":"gmail"}`},
		{"cloudLink.FinishOAuth", func() error { _, _, e := c.CloudLink.FinishOAuth(ctx, "sess_1"); return e },
			"POST", "/v1/cloud-link/oauth/finish", `{"session":"sess_1"}`},
		{"cloudLink.ListWorkspaceMailboxes", func() error { _, _, e := c.CloudLink.ListWorkspaceMailboxes(ctx); return e },
			"GET", "/v1/cloud-link/workspace-mailboxes", ""},
		{"cloudLink.AdoptWorkspaceMailbox", func() error { _, _, e := c.CloudLink.AdoptWorkspaceMailbox(ctx, "em_9"); return e },
			"POST", "/v1/cloud-link/workspace-mailboxes/em_9/adopt", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got = recordedRequest{}
			if err := tc.call(); err != nil {
				t.Fatalf("call: %v", err)
			}
			if got.method != tc.wantMethod || got.path != tc.wantPath {
				t.Errorf("got %s %s, want %s %s", got.method, got.path, tc.wantMethod, tc.wantPath)
			}
			if got.rawQuery != "" {
				t.Errorf("unexpected query %q", got.rawQuery)
			}
			if tc.wantBody != "" && strings.TrimSpace(got.body) != tc.wantBody {
				t.Errorf("body = %s, want %s", got.body, tc.wantBody)
			}
		})
	}
}

func TestSelfhostSyncBodylessPosts(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()

	if _, err := c.PoolLink.DenyCode(ctx, "ABCD-EFGH"); err != nil {
		t.Fatalf("DenyCode: %v", err)
	}
	if got.body != "" {
		t.Errorf("DenyCode sent a body: %q", got.body)
	}
	if _, _, err := c.CloudLink.PollConnect(ctx); err != nil {
		t.Fatalf("PollConnect: %v", err)
	}
	if got.body != "" {
		t.Errorf("PollConnect sent a body: %q", got.body)
	}
	if _, _, err := c.CloudLink.EnrollMailbox(ctx, "em_1"); err != nil {
		t.Fatalf("EnrollMailbox: %v", err)
	}
	if got.body != "" {
		t.Errorf("EnrollMailbox sent a body: %q", got.body)
	}
}

func TestPoolLinkDescribeCodeDecode(t *testing.T) {
	c := selfhostServer(t, http.StatusOK, `{
		"id": "code_1", "user_code": "ABCD-EFGH",
		"instance_name": "box", "instance_url": "https://box.example", "instance_version": "1.2.3",
		"status": "pending",
		"expires_at": "2026-09-06T10:15:00Z", "created_at": "2026-09-06T10:00:00Z"
	}`, nil)

	code, _, err := c.PoolLink.DescribeCode(context.Background(), "ABCD-EFGH")
	if err != nil {
		t.Fatalf("DescribeCode: %v", err)
	}
	if code.ID != "code_1" || code.UserCode != "ABCD-EFGH" || code.InstanceName != "box" ||
		code.InstanceURL != "https://box.example" || code.InstanceVersion != "1.2.3" {
		t.Errorf("code = %+v", code)
	}
	if code.Status != PoolLinkCodePending {
		t.Errorf("Status = %q, want %q", code.Status, PoolLinkCodePending)
	}
	if code.OrganizationID != nil || code.InstanceID != nil {
		t.Errorf("unapproved code carries org/instance: %+v", code)
	}
	if code.ExpiresAt.Sub(code.CreatedAt) != 15*time.Minute {
		t.Errorf("ExpiresAt-CreatedAt = %v, want 15m", code.ExpiresAt.Sub(code.CreatedAt))
	}
}

func TestPoolLinkStartAndPollDecode(t *testing.T) {
	c := selfhostServer(t, http.StatusCreated, `{
		"device_code": "dev_secret", "user_code": "ABCD-EFGH",
		"verification_url": "https://app.warmbly.com/connect?code=ABCD-EFGH",
		"expires_in": 900, "interval": 3
	}`, nil)
	grant, resp, err := c.PoolLink.StartCode(context.Background(), &PoolLinkStartParams{InstanceName: "box"})
	if err != nil {
		t.Fatalf("StartCode: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", resp.StatusCode)
	}
	if grant.DeviceCode != "dev_secret" || grant.UserCode != "ABCD-EFGH" || grant.ExpiresIn != 900 || grant.Interval != 3 ||
		grant.VerificationURL != "https://app.warmbly.com/connect?code=ABCD-EFGH" {
		t.Errorf("grant = %+v", grant)
	}

	c = selfhostServer(t, http.StatusOK, `{
		"status": "approved", "instance_id": "inst_1", "instance_token": "plt_once",
		"organization": {"id": "org_1", "name": "Acme"}
	}`, nil)
	res, _, err := c.PoolLink.Poll(context.Background(), "dev_secret")
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if res.Status != PoolLinkCodeApproved || res.InstanceID == nil || *res.InstanceID != "inst_1" ||
		res.InstanceToken != "plt_once" || res.Organization == nil || res.Organization.Name != "Acme" {
		t.Errorf("poll = %+v", res)
	}
}

func TestPoolLinkInstanceListDecode(t *testing.T) {
	c := selfhostServer(t, http.StatusOK, `{
		"data": [{
			"id": "inst_1", "organization_id": "org_1", "name": "box", "url": "https://box.example",
			"version": "1.2.3", "created_by": "user_1",
			"created_at": "2026-09-01T00:00:00Z", "last_seen_at": "2026-09-06T09:59:00Z",
			"mailbox_count": 4
		}],
		"plan": {"tier": "free", "mailbox_limit": 10, "enrolled": 4, "price_usd": 15,
			"upgrade_url": "https://app.warmbly.com/settings/billing", "warmup_entitled": true}
	}`, nil)

	list, _, err := c.PoolLink.ListInstances(context.Background())
	if err != nil {
		t.Fatalf("ListInstances: %v", err)
	}
	if len(list.Data) != 1 {
		t.Fatalf("len(Data) = %d, want 1", len(list.Data))
	}
	inst := list.Data[0]
	if inst.ID != "inst_1" || inst.Name != "box" || inst.Version != "1.2.3" || inst.MailboxCount != 4 ||
		inst.CreatedBy == nil || *inst.CreatedBy != "user_1" || inst.LastSeenAt == nil || inst.RevokedAt != nil {
		t.Errorf("instance = %+v", inst)
	}
	if list.Plan.Tier != PoolLinkTierFree || list.Plan.MailboxLimit == nil || *list.Plan.MailboxLimit != 10 ||
		list.Plan.Enrolled != 4 || list.Plan.PriceUSD != 15 || !list.Plan.WarmupEntitled {
		t.Errorf("plan = %+v", list.Plan)
	}

	// An unlimited plan reports a null limit, and an empty list is never nil.
	c = selfhostServer(t, http.StatusOK, `{"data": null, "plan": {"tier": "paid", "mailbox_limit": null}}`, nil)
	list, _, err = c.PoolLink.ListInstances(context.Background())
	if err != nil {
		t.Fatalf("ListInstances: %v", err)
	}
	if list.Data == nil || len(list.Data) != 0 {
		t.Errorf("Data = %#v, want empty non-nil", list.Data)
	}
	if list.Plan.Tier != PoolLinkTierPaid || list.Plan.MailboxLimit != nil {
		t.Errorf("plan = %+v, want paid/unlimited", list.Plan)
	}
}

func TestCloudLinkStatusDecode(t *testing.T) {
	c := selfhostServer(t, http.StatusOK, `{
		"connected": true,
		"link": {"cloud_url": "https://api.warmbly.com", "instance_id": "inst_1", "organization_name": "Acme",
			"connected_by": "user_1", "connected_at": "2026-09-01T00:00:00Z",
			"last_synced_at": "2026-09-06T09:59:00Z"},
		"info": {
			"instance": {"id": "inst_1", "organization_id": "org_1", "name": "box", "url": "https://box.example",
				"version": "1.2.3", "created_at": "2026-09-01T00:00:00Z", "mailbox_count": 0},
			"organization": {"id": "org_1", "name": "Acme"},
			"plan": {"tier": "free", "mailbox_limit": 10, "enrolled": 2, "price_usd": 15, "warmup_entitled": true}
		},
		"reachable": true,
		"default_cloud_url": "https://api.warmbly.com"
	}`, nil)

	st, _, err := c.CloudLink.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !st.Connected || !st.Reachable || st.Error != "" || st.DefaultCloudURL != "https://api.warmbly.com" {
		t.Errorf("status = %+v", st)
	}
	if st.Link == nil || st.Link.InstanceID != "inst_1" || st.Link.OrganizationName != "Acme" ||
		st.Link.ConnectedBy == nil || *st.Link.ConnectedBy != "user_1" || st.Link.LastSyncedAt == nil || st.Link.LastError != "" {
		t.Errorf("link = %+v", st.Link)
	}
	if st.Info == nil || st.Info.Instance.ID != "inst_1" || st.Info.Organization.Name != "Acme" ||
		st.Info.Plan.MailboxLimit == nil || *st.Info.Plan.MailboxLimit != 10 || st.Info.Plan.Enrolled != 2 {
		t.Errorf("info = %+v", st.Info)
	}

	// Unreachable: the link still comes from the local row, info is absent.
	c = selfhostServer(t, http.StatusOK, `{
		"connected": true,
		"link": {"cloud_url": "https://api.warmbly.com", "instance_id": "inst_1", "organization_name": "Acme",
			"connected_at": "2026-09-01T00:00:00Z", "last_error": "dial tcp: timeout"},
		"reachable": false, "error": "dial tcp: timeout",
		"default_cloud_url": "https://api.warmbly.com"
	}`, nil)
	st, _, err = c.CloudLink.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !st.Connected || st.Reachable || st.Info != nil || st.Error != "dial tcp: timeout" || st.Link == nil || st.Link.LastError == "" {
		t.Errorf("unreachable status = %+v", st)
	}

	// Not linked at all.
	c = selfhostServer(t, http.StatusOK, `{"connected": false, "reachable": false, "default_cloud_url": "https://api.warmbly.com"}`, nil)
	st, _, err = c.CloudLink.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Connected || st.Link != nil || st.Info != nil {
		t.Errorf("unlinked status = %+v", st)
	}
}

func TestCloudLinkMailboxesDecode(t *testing.T) {
	c := selfhostServer(t, http.StatusOK, `{"data": [
		{"id": "em_1", "email": "a@box.example", "name": "A", "provider": "smtp_imap", "status": "active",
		 "enrolled": true, "enrolled_at": "2026-09-02T00:00:00Z", "managed": false,
		 "cloud": {"remote_id": "em_1", "email_account_id": "cem_1", "email": "a@box.example", "provider": "smtp_imap",
			"status": "active", "enrolled_at": "2026-09-02T00:00:00Z", "managed": false,
			"warmup": {"enabled": true, "paused": false, "started_at": "2026-09-02T00:00:00Z",
				"current_volume": 3, "target_volume": 8, "max_volume": 40, "reply_rate": 30, "days_active": 4,
				"ramp_hold": {"placements": 2, "sends": 30, "volume_cut": true, "resumes_at": "2026-09-07T00:00:00Z"}},
			"health": {"state": "watch", "score": 0.7, "reason": "spam placements", "spam_score": 2},
			"sent_today": 3, "sent_7d": 40, "replied_7d": 12, "spam_placed_7d": 2,
			"errors": [{"id": "err_1", "error_code": "imap_auth", "severity": "error", "title": "IMAP", "message": "denied", "created_at": "2026-09-05T00:00:00Z"}],
			"auth_state": "ok",
			"settings": {"base": 2, "max": 40, "increase": 2, "reply_rate": 30, "start_time": "09:00", "end_time": "17:00", "days": 31, "timezone": "UTC"}}},
		{"id": "em_2", "email": "b@box.example", "name": "B", "provider": "gmail", "status": "active", "enrolled": false, "managed": false}
	]}`, nil)

	rows, _, err := c.CloudLink.ListMailboxes(context.Background())
	if err != nil {
		t.Fatalf("ListMailboxes: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	a := rows[0]
	if a.ID != "em_1" || !a.Enrolled || a.EnrolledAt == nil || a.Managed || a.Provider != ProviderSMTPIMAP || a.Cloud == nil {
		t.Fatalf("row a = %+v", a)
	}
	cl := a.Cloud
	if cl.RemoteID != "em_1" || cl.EmailAccountID != "cem_1" || cl.SentToday != 3 || cl.Sent7d != 40 || cl.SpamPlaced7d != 2 || cl.AuthState != "ok" {
		t.Errorf("cloud state = %+v", cl)
	}
	if cl.Warmup == nil || !cl.Warmup.Enabled || cl.Warmup.TargetVolume != 8 || cl.Warmup.RampHold == nil || !cl.Warmup.RampHold.VolumeCut {
		t.Errorf("warmup = %+v", cl.Warmup)
	}
	if cl.Health == nil || cl.Health.State != BandWatch || cl.Health.Reason != "spam placements" {
		t.Errorf("health = %+v", cl.Health)
	}
	if len(cl.Errors) != 1 || cl.Errors[0].ErrorCode != "imap_auth" {
		t.Errorf("errors = %+v", cl.Errors)
	}
	if cl.Settings.Base != 2 || cl.Settings.Max != 40 || cl.Settings.StartTime != "09:00" || cl.Settings.Days != 31 {
		t.Errorf("settings = %+v", cl.Settings)
	}
	b := rows[1]
	if b.Enrolled || b.EnrolledAt != nil || b.Cloud != nil || b.Provider != ProviderGmail {
		t.Errorf("row b = %+v", b)
	}

	// The list is never nil.
	c = selfhostServer(t, http.StatusOK, `{"data": null}`, nil)
	rows, _, err = c.CloudLink.ListMailboxes(context.Background())
	if err != nil {
		t.Fatalf("ListMailboxes: %v", err)
	}
	if rows == nil || len(rows) != 0 {
		t.Errorf("rows = %#v, want empty non-nil", rows)
	}
}

func TestCloudLinkConnectDecode(t *testing.T) {
	c := selfhostServer(t, http.StatusCreated, `{
		"user_code": "ABCD-EFGH", "verification_url": "https://app.warmbly.com/connect?code=ABCD-EFGH",
		"cloud_url": "https://api.warmbly.com", "expires_at": "2026-09-06T10:15:00Z", "interval": 3
	}`, nil)
	pending, _, err := c.CloudLink.Connect(context.Background(), "")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if pending.UserCode != "ABCD-EFGH" || pending.CloudURL != "https://api.warmbly.com" || pending.Interval != 3 || pending.ExpiresAt.IsZero() {
		t.Errorf("pending = %+v", pending)
	}

	c = selfhostServer(t, http.StatusOK, `{"status": "approved",
		"link": {"cloud_url": "https://api.warmbly.com", "instance_id": "inst_1", "organization_name": "Acme", "connected_at": "2026-09-06T10:01:00Z"},
		"info": {"instance": {"id": "inst_1"}, "organization": {"id": "org_1", "name": "Acme"}, "plan": {"tier": "free", "mailbox_limit": 10}}
	}`, nil)
	res, _, err := c.CloudLink.PollConnect(context.Background())
	if err != nil {
		t.Fatalf("PollConnect: %v", err)
	}
	if res.Status != PoolLinkCodeApproved || res.Link == nil || res.Link.InstanceID != "inst_1" || res.Info == nil || res.Info.Organization.Name != "Acme" {
		t.Errorf("poll = %+v", res)
	}
}

func TestCloudLinkWorkspaceMailboxesDecode(t *testing.T) {
	c := selfhostServer(t, http.StatusOK, `{"data": [
		{"id": "cem_9", "email": "c@acme.com", "name": "C", "provider": "outlook", "status": "active"}
	]}`, nil)
	list, _, err := c.CloudLink.ListWorkspaceMailboxes(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaceMailboxes: %v", err)
	}
	if len(list) != 1 || list[0].ID != "cem_9" || list[0].Provider != ProviderOutlook {
		t.Errorf("list = %+v", list)
	}

	c = selfhostServer(t, http.StatusCreated, `{"id": "em_9", "email": "c@acme.com", "provider": "outlook", "status": "active"}`, nil)
	acc, resp, err := c.CloudLink.AdoptWorkspaceMailbox(context.Background(), "cem_9")
	if err != nil {
		t.Fatalf("AdoptWorkspaceMailbox: %v", err)
	}
	if resp.StatusCode != http.StatusCreated || acc.ID != "em_9" || acc.Provider != ProviderOutlook {
		t.Errorf("adopt = %d %+v", resp.StatusCode, acc)
	}
}

// withFastPoll drops the poll floor for the duration of a test so the wait
// helpers can be exercised without real sleeps.
func withFastPoll(t *testing.T) {
	t.Helper()
	prev := poolLinkPollFloor
	poolLinkPollFloor = time.Millisecond
	t.Cleanup(func() { poolLinkPollFloor = prev })
}

func TestPoolLinkWaitForApproval(t *testing.T) {
	withFastPoll(t)
	var polls atomic.Int32
	c := selfhostServerFunc(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/pool-link/poll" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if polls.Add(1) < 3 {
			_, _ = w.Write([]byte(`{"status": "pending"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status": "approved", "instance_id": "inst_1", "instance_token": "plt_once", "organization": {"id": "org_1", "name": "Acme"}}`))
	})

	grant := &PoolLinkCodeGrant{DeviceCode: "dev_secret", Interval: 0}
	res, err := c.PoolLink.WaitForApproval(context.Background(), grant)
	if err != nil {
		t.Fatalf("WaitForApproval: %v", err)
	}
	if res.InstanceToken != "plt_once" || res.Status != PoolLinkCodeApproved {
		t.Errorf("result = %+v", res)
	}
	if polls.Load() != 3 {
		t.Errorf("polls = %d, want 3", polls.Load())
	}
}

func TestPoolLinkWaitForApprovalTerminalError(t *testing.T) {
	withFastPoll(t)
	c := selfhostServer(t, http.StatusForbidden, `{"error": "forbidden", "message": "The connection was declined.", "code": "pool_link_denied"}`, nil)

	_, err := c.PoolLink.WaitForApproval(context.Background(), &PoolLinkCodeGrant{DeviceCode: "dev_secret", Interval: 1})
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *Error", err)
	}
	if apiErr.Code != "pool_link_denied" || apiErr.StatusCode != http.StatusForbidden {
		t.Errorf("err = %+v", apiErr)
	}
}

func TestPoolLinkWaitForApprovalContextDone(t *testing.T) {
	// A generous floor: the loop must be waiting on the timer when ctx ends.
	prev := poolLinkPollFloor
	poolLinkPollFloor = time.Minute
	t.Cleanup(func() { poolLinkPollFloor = prev })

	c := selfhostServer(t, http.StatusOK, `{"status": "pending"}`, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := c.PoolLink.WaitForApproval(ctx, &PoolLinkCodeGrant{DeviceCode: "dev_secret", Interval: 3})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("loop ignored ctx: took %v", time.Since(start))
	}
}

func TestCloudLinkWaitForConnect(t *testing.T) {
	withFastPoll(t)
	var polls atomic.Int32
	c := selfhostServerFunc(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/cloud-link/connect/poll" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if polls.Add(1) < 2 {
			_, _ = w.Write([]byte(`{"status": "pending"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status": "approved", "link": {"cloud_url": "https://api.warmbly.com", "instance_id": "inst_1", "organization_name": "Acme", "connected_at": "2026-09-06T10:01:00Z"}}`))
	})

	res, err := c.CloudLink.WaitForConnect(context.Background(), &CloudLinkPendingConnect{Interval: 0})
	if err != nil {
		t.Fatalf("WaitForConnect: %v", err)
	}
	if res.Status != PoolLinkCodeApproved || res.Link == nil || res.Link.OrganizationName != "Acme" {
		t.Errorf("result = %+v", res)
	}
	if polls.Load() != 2 {
		t.Errorf("polls = %d, want 2", polls.Load())
	}

	// A retired code ends the loop with the server's error.
	c = selfhostServer(t, http.StatusNotFound, `{"error": "not_found", "message": "The code expired before it was approved. Start again.", "code": "cloud_link_code_expired"}`, nil)
	_, err = c.CloudLink.WaitForConnect(context.Background(), &CloudLinkPendingConnect{Interval: 3})
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "cloud_link_code_expired" {
		t.Errorf("err = %v, want cloud_link_code_expired", err)
	}
}

func TestSelfhostSyncNotImplemented(t *testing.T) {
	c := selfhostServer(t, http.StatusNotImplemented, `{"error": "not_implemented", "message": "cloud link is not enabled on this instance", "code": "not_implemented"}`, nil)
	_, _, err := c.CloudLink.Status(context.Background())
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotImplemented {
		t.Errorf("err = %v, want 501 *Error", err)
	}
}
