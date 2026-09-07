package warmbly

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestEmailSyncRouting asserts the method and path (and the body where it
// carries something) of every mailbox route added since the previous server
// sync, so a typo in a resource path cannot survive to a 404 at runtime.
func TestEmailSyncRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()

	cases := []struct {
		name       string
		call       func() error
		wantMethod string
		wantPath   string
		wantBody   string // "" asserts an empty body; otherwise a substring
	}{
		{"Allowance", func() error { _, _, e := c.Emails.Allowance(ctx); return e }, "GET", "/v1/emails/allowance", ""},
		{"GetTrackingDomain", func() error { _, _, e := c.Emails.GetTrackingDomain(ctx, "em_1"); return e }, "GET", "/v1/emails/em_1/track", ""},
		{"VerifyTrackingDomain", func() error { _, _, e := c.Emails.VerifyTrackingDomain(ctx, "em_1"); return e }, "POST", "/v1/emails/em_1/track/verify", ""},
		{"Hold", func() error { _, _, e := c.Emails.Hold(ctx, "em_1"); return e }, "POST", "/v1/emails/em_1/hold", ""},
		{"Release", func() error { _, _, e := c.Emails.Release(ctx, "em_1"); return e }, "POST", "/v1/emails/em_1/release", ""},
		{"RefreshAuthCheck", func() error { _, _, e := c.Emails.RefreshAuthCheck(ctx, "em_1"); return e }, "POST", "/v1/emails/em_1/auth-check", ""},
		{"SyncStatus", func() error { _, _, e := c.Emails.SyncStatus(ctx, "em_1"); return e }, "GET", "/v1/emails/em_1/sync", ""},
		{"Behavior", func() error { _, _, e := c.Emails.Behavior(ctx, "em_1"); return e }, "GET", "/v1/emails/em_1/behavior", ""},
		{"UpdateBehavior", func() error {
			_, _, e := c.Emails.UpdateBehavior(ctx, "em_1", &SendingBehaviorUpdateParams{Enabled: Bool(true), Weekdays: Int(BehaviorWeekdays)})
			return e
		}, "PUT", "/v1/emails/em_1/behavior", `{"enabled":true,"weekdays":31}`},
		{"BehaviorPlan", func() error { _, _, e := c.Emails.BehaviorPlan(ctx, "em_1"); return e }, "GET", "/v1/emails/em_1/behavior/plan", ""},
		{"ConnectSMTPIMAPBulk", func() error {
			_, _, e := c.Emails.ConnectSMTPIMAPBulk(ctx, &SMTPIMAPBulkParams{Accounts: []SMTPIMAPParams{{
				Email: "a@b.com",
				SMTP:  &MailboxCredentials{Host: "smtp.b.com", Port: 2525, Security: MailSecurityStartTLS},
				IMAP:  &MailboxCredentials{Host: "imap.b.com", Port: 993},
			}}})
			return e
		}, "POST", "/v1/emails/onboarding/smtp-imap/bulk", `"accounts":[{"email":"a@b.com","smtp":{"username":"","password":"","host":"smtp.b.com","port":2525,"security":"starttls"},"imap":{"username":"","password":"","host":"imap.b.com","port":993}}]`},
		{"ReauthOAuth", func() error { _, _, e := c.Emails.ReauthOAuth(ctx, "em_1"); return e }, "POST", "/v1/emails/onboarding/oauth/reauth/em_1", ""},
		{"UpdateSMTPIMAPCredentials", func() error {
			_, _, e := c.Emails.UpdateSMTPIMAPCredentials(ctx, "em_1", &SMTPIMAPCredentialsParams{
				SMTP: &MailboxCredentials{Username: "u", Password: "p", Host: "smtp.b.com", Port: 465},
				IMAP: &MailboxCredentials{Username: "u", Password: "p", Host: "imap.b.com", Port: 993},
			})
			return e
		}, "PUT", "/v1/emails/onboarding/smtp-imap/em_1", `"smtp":{"username":"u","password":"p","host":"smtp.b.com","port":465}`},
		// Changed request shapes on existing routes.
		{"Update save_to_sent+timezone", func() error {
			_, _, e := c.Emails.Update(ctx, "em_1", &EmailUpdateParams{SaveToSent: Bool(false), Timezone: String("Europe/London")})
			return e
		}, "PATCH", "/v1/emails/em_1", `{"timezone":"Europe/London","save_to_sent":false}`},
		{"Delete", func() error { _, e := c.Emails.Delete(ctx, "em_1"); return e }, "DELETE", "/v1/emails/em_1", ""},
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
			if tc.wantBody == "" {
				if got.body != "" {
					t.Errorf("body = %q, want it empty", got.body)
				}
			} else if !strings.Contains(got.body, tc.wantBody) {
				t.Errorf("body = %q, want it to contain %q", got.body, tc.wantBody)
			}
		})
	}
}

// emailFixtureClient answers every request with status and body, recording
// the request, for decode tests that need a realistic response rather than
// routingClient's generic envelope.
func emailFixtureClient(t *testing.T, status int, body string, got *recordedRequest) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got != nil {
			got.method = r.Method
			got.path = r.URL.Path
			got.rawQuery = r.URL.RawQuery
			b, _ := io.ReadAll(r.Body)
			got.body = string(b)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"), WithMaxRetries(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestEmailDecodeMailboxDrift(t *testing.T) {
	// A mailbox that has never synced: last_synced_at is null on the wire,
	// which the previous time.Time field could not decode.
	const body = `{
		"id": "0c0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b",
		"email": "sales@acme.com",
		"provider": "smtp_imap",
		"status": "active",
		"last_synced_at": null,
		"last_id": null,
		"campaign_limit": 50,
		"min_wait_time": 600,
		"save_to_sent": true,
		"auth_state": "failing",
		"auth_spf": false,
		"auth_dkim": true,
		"auth_dmarc": true,
		"auth_failing_since": "2026-08-30T03:00:00Z",
		"warmup": null,
		"timezone": "America/Denver",
		"tags": [],
		"created_at": "2026-05-19T18:00:00Z",
		"updated_at": "2026-06-11T09:14:00Z"
	}`
	c := emailFixtureClient(t, http.StatusOK, body, nil)
	em, _, err := c.Emails.Get(context.Background(), "em_1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if em.LastSyncedAt != nil {
		t.Errorf("LastSyncedAt = %v, want nil", em.LastSyncedAt)
	}
	if !em.SaveToSent {
		t.Error("SaveToSent = false, want true")
	}
	if em.AuthState != AuthStateFailing || em.AuthFailingSince == nil || em.AuthFailingSince.Day() != 30 {
		t.Errorf("auth = %q since %v", em.AuthState, em.AuthFailingSince)
	}
	if em.Timezone != "America/Denver" {
		t.Errorf("Timezone = %q", em.Timezone)
	}

	// And one that has synced, to keep the non-null path covered.
	c2 := emailFixtureClient(t, http.StatusOK, `{"id":"x","last_synced_at":"2026-06-11T09:14:00Z","tags":[]}`, nil)
	em2, _, err := c2.Emails.Get(context.Background(), "x")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if em2.LastSyncedAt == nil || em2.LastSyncedAt.Hour() != 9 {
		t.Errorf("LastSyncedAt = %v", em2.LastSyncedAt)
	}
}

func TestEmailDecodeAllowance(t *testing.T) {
	const body = `{
		"used": 148,
		"allowance": 150,
		"remaining": 2,
		"basis": "fair_use",
		"sends_per_mailbox": 1,
		"plan_daily_sends": 150,
		"plan_name": "Starter",
		"paid": true,
		"pending_request": {
			"id": "5e6f7a8b-9c0d-1e2f-3a4b-5c6d7e8f9a0b",
			"organization_id": "f9e8d7c6-0000-0000-0000-000000000000",
			"field": "max_email_accounts",
			"current_effective": 150,
			"requested": 300,
			"reason": "Second sales team onboarding.",
			"status": "pending",
			"submitted_by": "a1b2c3d4-0000-0000-0000-000000000000",
			"submitted_at": "2026-09-01T10:00:00Z",
			"review_notes": ""
		}
	}`
	var got recordedRequest
	c := emailFixtureClient(t, http.StatusOK, body, &got)
	a, _, err := c.Emails.Allowance(context.Background())
	if err != nil {
		t.Fatalf("Allowance: %v", err)
	}
	if a.Used != 148 || a.Allowance == nil || *a.Allowance != 150 || a.Remaining == nil || *a.Remaining != 2 {
		t.Errorf("counts = used %d allowance %v remaining %v", a.Used, a.Allowance, a.Remaining)
	}
	if a.Basis != MailboxAllowanceFairUse || a.SendsPerMailbox != 1 || a.PlanDailySends == nil || *a.PlanDailySends != 150 || a.PlanName != "Starter" || !a.Paid {
		t.Errorf("basis fields = %+v", a)
	}
	if a.Unlimited() || !a.CanAdd(2) || a.CanAdd(3) {
		t.Errorf("Unlimited/CanAdd wrong for %+v", a)
	}
	pr := a.PendingRequest
	if pr == nil || pr.Field != "max_email_accounts" || pr.CurrentEffective != 150 || pr.Requested != 300 || pr.Status != LimitRequestPending || pr.SubmittedAt.Month() != time.September {
		t.Errorf("PendingRequest = %+v", pr)
	}

	// Unlimited: allowance and remaining are null.
	c2 := emailFixtureClient(t, http.StatusOK, `{"used":3,"allowance":null,"remaining":null,"basis":"unlimited","sends_per_mailbox":1,"paid":true}`, nil)
	u, _, err := c2.Emails.Allowance(context.Background())
	if err != nil {
		t.Fatalf("Allowance: %v", err)
	}
	if !u.Unlimited() || !u.CanAdd(1000) || u.Basis != MailboxAllowanceUnlimited {
		t.Errorf("unlimited allowance = %+v", u)
	}
}

func TestEmailAllowanceReachedError(t *testing.T) {
	c := emailFixtureClient(t, http.StatusForbidden, `{"error":"forbidden","message":"This workspace holds 150 of its 150 mailboxes.","code":"mailbox_allowance_reached"}`, nil)
	_, _, err := c.Emails.ConnectSMTPIMAP(context.Background(), &SMTPIMAPParams{Email: "a@b.com"})
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %T %v, want *Error", err, err)
	}
	if apiErr.StatusCode != http.StatusForbidden || apiErr.Code != ErrCodeMailboxAllowanceReached {
		t.Errorf("err = %+v", apiErr)
	}
}

func TestEmailDecodeTrackingDomainStatus(t *testing.T) {
	const body = `{
		"tracking_domain": "t.acme.com",
		"tracking_domain_verified": false,
		"tracking_domain_verified_at": null,
		"cname_target": "t.warmbly.com",
		"status": "wrong_target",
		"message": "t.acme.com points at cdn.acme.com, not t.warmbly.com.",
		"observed": "cdn.acme.com",
		"tracking_host_unresolvable": false
	}`
	c := emailFixtureClient(t, http.StatusOK, body, nil)
	st, _, err := c.Emails.GetTrackingDomain(context.Background(), "em_1")
	if err != nil {
		t.Fatalf("GetTrackingDomain: %v", err)
	}
	if st.CNAMETarget != "t.warmbly.com" || st.Status != TrackingStatusWrongTarget || st.Observed != "cdn.acme.com" || st.Message == "" || st.TrackingHostUnresolvable {
		t.Errorf("status = %+v", st)
	}
	if st.TrackingDomainVerified || st.TrackingDomainVerifiedAt != nil {
		t.Errorf("verified fields = %v %v", st.TrackingDomainVerified, st.TrackingDomainVerifiedAt)
	}
}

func TestEmailDecodeSendLifecycleState(t *testing.T) {
	c := emailFixtureClient(t, http.StatusOK, `{"state":"reserve","since":"2026-08-28T10:15:00Z","reason":"held back by its owner"}`, nil)
	st, _, err := c.Emails.Hold(context.Background(), "em_1")
	if err != nil {
		t.Fatalf("Hold: %v", err)
	}
	if st.State != SendLifecycleReserve || st.Since == nil || st.Since.Day() != 28 || st.Reason != "held back by its owner" {
		t.Errorf("state = %+v", st)
	}
	if st.SendsCold() {
		t.Error("a reserved mailbox must not send cold")
	}

	c2 := emailFixtureClient(t, http.StatusOK, `{"state":"active"}`, nil)
	st2, _, err := c2.Emails.Release(context.Background(), "em_1")
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	if !st2.SendsCold() || st2.Since != nil {
		t.Errorf("state = %+v", st2)
	}
}

func TestEmailDecodeDomainAuthCheckDrift(t *testing.T) {
	const body = `{
		"domain": "mail.acme.com",
		"spf_found": true,
		"spf_record": "v=spf1 include:_spf.google.com ~all",
		"dkim_found": true,
		"dkim_selectors": ["google"],
		"dmarc_found": true,
		"dmarc_policy": "quarantine",
		"dmarc_domain": "acme.com",
		"dmarc_inherited": true,
		"reserved": false,
		"lookup_error": false,
		"all_aligned": true,
		"summary": "SPF, DKIM and DMARC all present"
	}`
	var got recordedRequest
	c := emailFixtureClient(t, http.StatusOK, body, &got)
	res, _, err := c.Emails.RefreshAuthCheck(context.Background(), "em_1")
	if err != nil {
		t.Fatalf("RefreshAuthCheck: %v", err)
	}
	if got.method != "POST" {
		t.Errorf("method = %s, want POST", got.method)
	}
	if res.DMARCDomain != "acme.com" || !res.DMARCInherited || res.Reserved || !res.AllAligned || res.DMARCPolicy != "quarantine" {
		t.Errorf("result = %+v", res)
	}
}

func TestEmailDecodeVerifyResultDrift(t *testing.T) {
	const body = `{
		"email": "jane@example.com",
		"status": "risky",
		"sub_status": "catch_all",
		"reason": "domain accepts every address",
		"is_catch_all": true,
		"has_mx": true,
		"provider": "builtin",
		"confidence": 40,
		"checked_at": "2026-06-11T09:30:00Z"
	}`
	c := emailFixtureClient(t, http.StatusOK, body, nil)
	v, _, err := c.Emails.Verify(context.Background(), "jane@example.com")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if v.Status != VerifyStatusRisky || v.SubStatus != VerifySubStatusCatchAll || v.Provider != "builtin" || v.Confidence != 40 || !v.IsCatchAll {
		t.Errorf("result = %+v", v)
	}
}

func TestEmailDecodeSyncStatus(t *testing.T) {
	const body = `{
		"state": {
			"backfill_status": "running",
			"backfill_cursor": {"folders": {"INBOX/12345": {"uid": 4021}, "Sent/12346": {"done": true}}},
			"backfill_synced": 1830,
			"backfill_since": "2026-06-08T00:00:00Z",
			"backfill_started_at": "2026-09-06T08:00:00Z",
			"throttled_until": "2026-09-07T00:00:00Z",
			"throttle_reason": "daily",
			"deferred": 212,
			"last_synced_at": "2026-09-06T08:41:00Z"
		},
		"policy": {"backfill_days": 90, "backfill_messages": 5000, "daily_messages": 2000, "org_daily_messages": 25000}
	}`
	c := emailFixtureClient(t, http.StatusOK, body, nil)
	s, _, err := c.Emails.SyncStatus(context.Background(), "em_1")
	if err != nil {
		t.Fatalf("SyncStatus: %v", err)
	}
	if s.Policy.BackfillDays != 90 || s.Policy.BackfillMessages != 5000 || s.Policy.DailyMessages != 2000 || s.Policy.OrgDailyMessages != 25000 {
		t.Errorf("policy = %+v", s.Policy)
	}
	st := s.State
	if st == nil {
		t.Fatal("State = nil")
	}
	if st.BackfillStatus != SyncBackfillRunning || st.BackfillSynced != 1830 || st.Deferred != 212 || st.ThrottleReason != SyncThrottleDaily {
		t.Errorf("state = %+v", st)
	}
	if st.BackfillSince == nil || st.BackfillStartedAt == nil || st.BackfillCompletedAt != nil || st.LastSyncedAt == nil {
		t.Errorf("timestamps = %+v", st)
	}
	if st.BackfillCursor.Folders["INBOX/12345"].UID != 4021 || !st.BackfillCursor.Folders["Sent/12346"].Done {
		t.Errorf("cursor = %+v", st.BackfillCursor)
	}
	if !st.Throttled(time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)) || st.Throttled(time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)) {
		t.Error("Throttled did not honor throttled_until")
	}

	// First connect: the worker has not reported yet.
	c2 := emailFixtureClient(t, http.StatusOK, `{"state":null,"policy":{"backfill_days":90,"backfill_messages":5000,"daily_messages":2000,"org_daily_messages":25000}}`, nil)
	s2, _, err := c2.Emails.SyncStatus(context.Background(), "em_1")
	if err != nil {
		t.Fatalf("SyncStatus: %v", err)
	}
	if s2.State != nil {
		t.Errorf("State = %+v, want nil", s2.State)
	}
}

const sendingBehaviorFixture = `{
	"email_account_id": "0c0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b",
	"enabled": true,
	"daily_limit_min": 30, "daily_limit_max": 45,
	"hourly_limit_min": 5, "hourly_limit_max": 9,
	"gap_min_seconds": 90, "gap_max_seconds": 420,
	"work_start_min": 543, "work_start_max": 567,
	"work_end_min": 1038, "work_end_max": 1076,
	"lunch_enabled": true, "lunch_earliest": 720, "lunch_latest": 810,
	"lunch_min_minutes": 30, "lunch_max_minutes": 60,
	"weekdays": 31,
	"timezone": "Europe/London",
	"created_at": "2026-09-01T10:00:00Z",
	"updated_at": "2026-09-05T10:00:00Z"
}`

func TestEmailDecodeSendingBehavior(t *testing.T) {
	c := emailFixtureClient(t, http.StatusOK, sendingBehaviorFixture, nil)
	b, _, err := c.Emails.Behavior(context.Background(), "em_1")
	if err != nil {
		t.Fatalf("Behavior: %v", err)
	}
	if !b.Enabled || b.DailyLimitMin != 30 || b.DailyLimitMax != 45 || b.HourlyLimitMin != 5 || b.HourlyLimitMax != 9 {
		t.Errorf("limits = %+v", b)
	}
	if b.GapMinSeconds != 90 || b.GapMaxSeconds != 420 || b.WorkStartMin != 543 || b.WorkStartMax != 567 || b.WorkEndMin != 1038 || b.WorkEndMax != 1076 {
		t.Errorf("ranges = %+v", b)
	}
	if !b.LunchEnabled || b.LunchEarliest != 720 || b.LunchLatest != 810 || b.LunchMinMinutes != 30 || b.LunchMaxMinutes != 60 {
		t.Errorf("lunch = %+v", b)
	}
	if b.Weekdays != BehaviorWeekdays || b.Timezone != "Europe/London" || b.EmailAccountID == "" {
		t.Errorf("misc = %+v", b)
	}
	if !b.WorksOn(time.Monday) || !b.WorksOn(time.Friday) || b.WorksOn(time.Saturday) || b.WorksOn(time.Sunday) {
		t.Error("WorksOn maps the Monday-indexed mask wrongly")
	}
}

func TestEmailDecodeDailyPlan(t *testing.T) {
	body := `{
		"email_account_id": "0c0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b",
		"plan_date": "2026-09-06",
		"timezone": "Europe/London",
		"is_working_day": true,
		"daily_limit": 38,
		"hourly_limit": 7,
		"work_start_minute": 551,
		"work_end_minute": 1062,
		"lunch_start_minute": 765,
		"lunch_end_minute": 810,
		"gap_min_seconds": 90,
		"gap_max_seconds": 420,
		"created_at": "2026-09-06T00:05:00Z",
		"sent_today": 12,
		"remaining_today": 26,
		"behavior": ` + sendingBehaviorFixture + `
	}`
	c := emailFixtureClient(t, http.StatusOK, body, nil)
	p, _, err := c.Emails.BehaviorPlan(context.Background(), "em_1")
	if err != nil {
		t.Fatalf("BehaviorPlan: %v", err)
	}
	if p.PlanDate != "2026-09-06" || !p.IsWorkingDay || p.DailyLimit != 38 || p.HourlyLimit != 7 || p.WorkStartMinute != 551 || p.WorkEndMinute != 1062 {
		t.Errorf("plan = %+v", p)
	}
	if !p.HasLunch() || *p.LunchStartMinute != 765 || *p.LunchEndMinute != 810 {
		t.Errorf("lunch = %v %v", p.LunchStartMinute, p.LunchEndMinute)
	}
	if p.SentToday != 12 || p.RemainingToday != 26 || p.Behavior.DailyLimitMax != 45 {
		t.Errorf("progress = %d/%d behavior %+v", p.SentToday, p.RemainingToday, p.Behavior)
	}

	// A non-working day carries no break.
	c2 := emailFixtureClient(t, http.StatusOK, `{"plan_date":"2026-09-06","is_working_day":false,"lunch_start_minute":null,"lunch_end_minute":null,"behavior":{}}`, nil)
	p2, _, err := c2.Emails.BehaviorPlan(context.Background(), "em_1")
	if err != nil {
		t.Fatalf("BehaviorPlan: %v", err)
	}
	if p2.IsWorkingDay || p2.HasLunch() {
		t.Errorf("plan = %+v", p2)
	}
}

func TestEmailDecodeBulkConnectResult(t *testing.T) {
	const body = `{
		"data": [
			{"row": 0, "email": "a@acme.com", "status": "connected", "id": "0c0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b"},
			{"row": 1, "email": "b@acme.com", "status": "skipped", "code": "already_connected", "message": "This email account is already connected."},
			{"row": 2, "email": "c@acme.com", "status": "failed", "code": "mailbox_allowance_reached", "message": "This workspace holds 150 of its 150 mailboxes."}
		],
		"summary": {"total": 3, "connected": 1, "skipped": 1, "failed": 1},
		"allowance": {"used": 150, "allowance": 150, "remaining": 0, "basis": "plan", "sends_per_mailbox": 1, "paid": true}
	}`
	c := emailFixtureClient(t, http.StatusOK, body, nil)
	res, _, err := c.Emails.ConnectSMTPIMAPBulk(context.Background(), &SMTPIMAPBulkParams{Accounts: []SMTPIMAPParams{{Email: "a@acme.com"}}})
	if err != nil {
		t.Fatalf("ConnectSMTPIMAPBulk: %v", err)
	}
	if res.Summary != (MailboxBulkSummary{Total: 3, Connected: 1, Skipped: 1, Failed: 1}) {
		t.Errorf("summary = %+v", res.Summary)
	}
	if len(res.Data) != 3 {
		t.Fatalf("rows = %d", len(res.Data))
	}
	if r := res.Data[0]; r.Row != 0 || r.Status != MailboxBulkConnected || r.ID == nil || *r.ID == "" || r.Code != "" {
		t.Errorf("row 0 = %+v", r)
	}
	if r := res.Data[1]; r.Status != MailboxBulkSkipped || r.Code != "already_connected" || r.ID != nil {
		t.Errorf("row 1 = %+v", r)
	}
	if r := res.Data[2]; r.Status != MailboxBulkFailed || r.Code != ErrCodeMailboxAllowanceReached || r.Message == "" {
		t.Errorf("row 2 = %+v", r)
	}
	if res.Allowance == nil || res.Allowance.Remaining == nil || *res.Allowance.Remaining != 0 || res.Allowance.Basis != MailboxAllowancePlan {
		t.Errorf("allowance = %+v", res.Allowance)
	}
}

func TestEmailDecodeReauthAndOAuthStart(t *testing.T) {
	var got recordedRequest
	c := emailFixtureClient(t, http.StatusOK, `{"url":"https://accounts.google.com/o/oauth2/auth?login_hint=sales%40acme.com","state":"n0nc3"}`, &got)
	res, _, err := c.Emails.ReauthOAuth(context.Background(), "em_1")
	if err != nil {
		t.Fatalf("ReauthOAuth: %v", err)
	}
	if got.method != "POST" || got.path != "/v1/emails/onboarding/oauth/reauth/em_1" || got.body != "" {
		t.Errorf("request = %s %s body %q", got.method, got.path, got.body)
	}
	if !strings.HasPrefix(res.URL, "https://accounts.google.com/") || res.State != "n0nc3" {
		t.Errorf("result = %+v", res)
	}
}

func TestEmailFinishOAuthReportsCreatedVersusRenewed(t *testing.T) {
	for _, status := range []int{http.StatusCreated, http.StatusOK} {
		c := emailFixtureClient(t, status, `{"id":"em_1","email":"sales@acme.com","provider":"gmail","tags":[]}`, nil)
		em, resp, err := c.Emails.FinishOAuth(context.Background(), "code", "state")
		if err != nil {
			t.Fatalf("FinishOAuth (%d): %v", status, err)
		}
		if resp.StatusCode != status || em.ID != "em_1" {
			t.Errorf("status %d: resp %d mailbox %+v", status, resp.StatusCode, em)
		}
	}
}

// TestEmailMailboxCredentialsOmitEmptySecurity guards the "let the port
// decide" default: an unset Security must not be sent as "", which the server
// would reject as an unknown mode.
func TestEmailMailboxCredentialsOmitEmptySecurity(t *testing.T) {
	b, err := json.Marshal(MailboxCredentials{Host: "smtp.b.com", Port: 587})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "security") {
		t.Errorf("body = %s, want security omitted", b)
	}
	b, _ = json.Marshal(MailboxCredentials{Host: "smtp.b.com", Port: 465, Security: MailSecurityTLS})
	if !strings.Contains(string(b), `"security":"tls"`) {
		t.Errorf("body = %s, want security=tls", b)
	}
}

// TestEmailBehaviorUpdateOmitsUnsetFields guards the partial-update contract:
// a field the caller did not touch must be absent, not null, or the server
// would treat it as a value.
func TestEmailBehaviorUpdateOmitsUnsetFields(t *testing.T) {
	b, err := json.Marshal(&SendingBehaviorUpdateParams{LunchEnabled: Bool(false)})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"lunch_enabled":false}` {
		t.Errorf("body = %s", b)
	}
}
