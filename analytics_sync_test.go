package warmbly

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCampaignLeadAndSendPlan(t *testing.T) {
	ctx := context.Background()
	var got recordedRequest

	t.Run("send plan", func(t *testing.T) {
		c := recordingFixtureClient(t, &got, `{
			"campaign_id": "camp_1", "status": "active", "day": "2026-09-07", "timezone": "Europe/Berlin",
			"computed_at": "2026-09-07T09:00:00Z", "stale": true,
			"configured_ceiling": 100, "projected_today": 60, "sent_today": 20, "expected_remaining": 40,
			"bottleneck": "future_limit",
			"limits": [{"kind": "warmup_graduation", "emails": 30, "mailboxes": 2}, {"kind": "campaign_daily_limit", "emails": 10}],
			"window": {"sending_day": true, "open_now": true, "closes_at": "2026-09-07T17:00:00Z", "minutes_left": 300},
			"leads": {"due_now": 5, "due_later_today": 3, "new_leads_due_today": 2, "waiting_on_step": 9, "waiting_on_condition": 0,
				"held": 1, "waiting_on_sender": 0, "new_leads_started_today": 4, "max_new_leads_per_day": 10},
			"mailboxes": [{"id": "em_1", "email": "a@acme.com", "provider": "google", "configured_cap": 50, "cap_today": 20,
				"limited_by": "warmup_graduation", "sent_today": 10, "sent_by_other_campaigns": 0, "expected_remaining": 10,
				"state": "brand_new_state", "min_gap_seconds": 60,
				"graduation": {"ceiling": 20, "mailbox_cap": 50, "days_to_full_cap": 3, "held": false}}],
			"organization": {"daily_limit": 500, "sent_today": 120, "remaining": 380}
		}`)
		p, _, err := c.Campaigns.SendPlan(ctx, "camp_1")
		if err != nil {
			t.Fatal(err)
		}
		if got.method != "GET" || got.path != "/v1/campaigns/camp_1/send-plan" {
			t.Errorf("request = %s %s", got.method, got.path)
		}
		// Unknown bottleneck and mailbox state values decode as plain strings.
		if p.Bottleneck != "future_limit" || p.Mailboxes[0].State != "brand_new_state" || !p.Stale {
			t.Errorf("plan = %+v", p)
		}
		if p.Projected != 60 || p.Limits[0].Kind != SendLimitWarmupGraduation || p.Limits[0].Mailboxes != 2 ||
			p.Window.ClosesAt == nil || p.Window.OpensAt != nil || p.NextWakeAt != nil {
			t.Errorf("plan = %+v", p)
		}
		if g := p.Mailboxes[0].Graduation; g == nil || g.DaysToFullCap != 3 {
			t.Errorf("graduation = %+v", g)
		}
		if p.Leads.Held != 1 || p.Leads.MaxNewLeadsPerDay != 10 || p.Organization == nil || p.Organization.Remaining != 380 {
			t.Errorf("leads/org = %+v %+v", p.Leads, p.Organization)
		}
	})

	t.Run("hold, pause, resume", func(t *testing.T) {
		c := recordingFixtureClient(t, &got, `{"campaign_id": "camp_1", "contact_id": "ct_1"}`)
		h, _, err := c.Campaigns.LeadHold(ctx, "camp_1", "ct_1")
		if err != nil {
			t.Fatal(err)
		}
		if got.method != "GET" || got.path != "/v1/campaigns/camp_1/leads/ct_1/hold" || h.Hold != nil {
			t.Errorf("hold = %s %s %+v", got.method, got.path, h)
		}

		c = recordingFixtureClient(t, &got, `{"campaign_id": "camp_1", "contact_id": "ct_1",
			"hold": {"since": "2026-09-07T09:00:00Z", "until": "2026-09-20T00:00:00Z", "reason": "On leave", "source": "manual"}}`)
		until := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
		h, _, err = c.Campaigns.PauseLead(ctx, "camp_1", "ct_1", &CampaignLeadPauseParams{Until: &until, Reason: "On leave"})
		if err != nil {
			t.Fatal(err)
		}
		if got.method != "POST" || got.path != "/v1/campaigns/camp_1/leads/ct_1/pause" ||
			strings.TrimSpace(got.body) != `{"until":"2026-09-20T00:00:00Z","reason":"On leave"}` {
			t.Errorf("pause = %s %s %s", got.method, got.path, got.body)
		}
		if h.Hold == nil || h.Hold.Source != LeadHoldManual || h.Hold.Until == nil {
			t.Errorf("hold = %+v", h.Hold)
		}

		// No params: the hold has no end and no body is sent.
		if _, _, err = c.Campaigns.PauseLead(ctx, "camp_1", "ct_1", nil); err != nil {
			t.Fatal(err)
		}
		if got.body != "" {
			t.Errorf("bodyless pause sent %q", got.body)
		}

		c = recordingFixtureClient(t, &got, `{"campaign_id": "camp_1", "contact_id": "ct_1"}`)
		h, _, err = c.Campaigns.ResumeLead(ctx, "camp_1", "ct_1")
		if err != nil {
			t.Fatal(err)
		}
		if got.method != "POST" || got.path != "/v1/campaigns/camp_1/leads/ct_1/resume" || h.Hold != nil {
			t.Errorf("resume = %s %s %+v", got.method, got.path, h)
		}
	})

	t.Run("cc", func(t *testing.T) {
		const list = `{"campaign_id": "camp_1", "contact_id": "ct_1", "cc": [
			{"contact_id": "ct_2", "email": "b@acme.com", "first_name": "Bo", "last_name": "Lee", "status": "bounced", "bounced_at": "2026-09-01T00:00:00Z"},
			{"contact_id": "ct_3", "email": "c@acme.com", "first_name": "Cy", "last_name": "Kim", "company": "Acme", "status": "future_status"}]}`
		c := recordingFixtureClient(t, &got, list)
		l, _, err := c.Campaigns.LeadCC(ctx, "camp_1", "ct_1")
		if err != nil {
			t.Fatal(err)
		}
		if got.method != "GET" || got.path != "/v1/campaigns/camp_1/leads/ct_1/cc" || len(l.CC) != 2 ||
			l.CC[0].Status != LeadCCBounced || l.CC[0].BouncedAt == nil || l.CC[1].Status != "future_status" || l.CC[1].BouncedAt != nil {
			t.Errorf("cc = %s %s %+v", got.method, got.path, l)
		}

		c = recordingFixtureClient(t, &got, list)
		if _, _, err = c.Campaigns.SetLeadCC(ctx, "camp_1", "ct_1", []string{"ct_2"}); err != nil {
			t.Fatal(err)
		}
		if got.method != "PUT" || strings.TrimSpace(got.body) != `{"contact_ids":["ct_2"]}` {
			t.Errorf("set cc = %s %s", got.method, got.body)
		}
		// Clearing must send an explicit empty list, not null.
		if _, _, err = c.Campaigns.SetLeadCC(ctx, "camp_1", "ct_1", nil); err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(got.body) != `{"contact_ids":[]}` {
			t.Errorf("clear cc body = %s", got.body)
		}

		c = recordingFixtureClient(t, &got, `{"data": [{"contact_id": "ct_4", "email": "d@acme.com", "first_name": "Di", "last_name": "Ng", "reason": "domain"}]}`)
		s, _, err := c.Campaigns.SuggestLeadCC(ctx, "camp_1", "ct_1")
		if err != nil {
			t.Fatal(err)
		}
		if got.method != "GET" || got.path != "/v1/campaigns/camp_1/leads/ct_1/cc/suggestions" || len(s) != 1 || s[0].Reason != "domain" {
			t.Errorf("suggestions = %s %s %+v", got.method, got.path, s)
		}
	})
}

func TestAnalyticsSync(t *testing.T) {
	ctx := context.Background()
	var got recordedRequest

	t.Run("dashboard scope", func(t *testing.T) {
		c := recordingFixtureClient(t, &got, `{
			"period": "30d",
			"overall_stats": {"total_emails_sent": 10},
			"recent_activity": [{"type": "opened", "campaign_id": "c1", "campaign_name": "Q3", "contact_email": "j@x.com",
				"contact_id": "ct_1", "timestamp": "2026-09-07T09:00:00Z",
				"origin": {"client": "Gmail", "device_type": "mobile", "country_code": "DE"},
				"sender_id": "em_1", "sender_email": "a@acme.com"}],
			"top_campaigns": [], "account_health": {}, "daily_trend": [],
			"capacity_today": {"capacity": 300, "remaining_today": 120, "configured_ceiling": 500, "mailboxes": 8, "held": 1},
			"scope": {"campaigns": [{"id": "c1", "name": "Q3"}], "folders": [], "campaign_count": 1}
		}`)
		d, _, err := c.Analytics.DashboardFor(ctx, &DashboardParams{Period: Period30Days, CampaignIDs: []string{"c1", "c2"}, FolderIDs: []string{"f1"}})
		if err != nil {
			t.Fatal(err)
		}
		if got.path != "/v1/analytics/dashboard" || got.rawQuery != "campaign_ids=c1%2Cc2&folder_ids=f1&period=30d" {
			t.Errorf("request = %s ?%s", got.path, got.rawQuery)
		}
		if d.CapacityToday == nil || d.CapacityToday.Remaining != 120 || d.Scope == nil || d.Scope.CampaignCount != 1 {
			t.Errorf("dashboard = %+v", d)
		}
		a := d.RecentActivity[0]
		if a.Origin == nil || a.Origin.Client != "Gmail" || a.SenderID == nil || a.SenderEmail != "a@acme.com" {
			t.Errorf("activity = %+v", a)
		}
	})

	t.Run("campaign analytics", func(t *testing.T) {
		c := recordingFixtureClient(t, &got, `{
			"campaign_id": "c1", "name": "Q3", "status": "active",
			"date_range": {"from": "2026-09-01T00:00:00Z", "to": "2026-09-07T00:00:00Z"},
			"summary": {"total_contacts": 100, "emails_sent": 80, "positive_replies": 4, "interested_leads": 3,
				"reply_breakdown": {"positive": 4, "neutral": 1, "negative": 1, "unsubscribe": 0, "unclassified": 0, "out_of_office": 2, "auto_reply": 1},
				"positive_reply_rate": 5.0, "reply_rate": 7.5},
			"steps": [{"step_id": "s1", "name": "Intro", "position": 1, "emails_sent": 80, "opens": 40, "machine_opens": 10,
				"clicks": 5, "machine_clicks": 1, "replies": 6, "positive_replies": 4, "bounces": 1,
				"open_rate": 50, "positive_reply_rate": 5}],
			"daily_stats": [{"date": "2026-09-07", "sent": 10, "opens": 5, "clicks": 1, "replies": 1, "positive_replies": 1}],
			"engagement": {"countries": [], "clients": [], "devices": [], "surfaces": [{"key": "hidden", "opens": 3, "clicks": 0}]}
		}`)
		from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
		a, _, err := c.Analytics.CampaignBetween(ctx, "c1", from, to)
		if err != nil {
			t.Fatal(err)
		}
		if got.path != "/v1/analytics/campaigns/c1" || got.rawQuery != "from=2026-09-01&to=2026-09-07" {
			t.Errorf("request = %s ?%s", got.path, got.rawQuery)
		}
		if a.Summary.PositiveReplies != 4 || a.Summary.InterestedLeads != 3 || a.Summary.ReplyBreakdown.OutOfOffice != 2 ||
			a.Summary.PositiveReplyRate != 5 || a.Steps[0].MachineOpens != 10 || a.Steps[0].PositiveReplies != 4 ||
			a.DailyStats[0].PositiveReplies != 1 || a.Engagement.Surfaces[0].Key != "hidden" {
			t.Errorf("analytics = %+v", a)
		}
	})

	t.Run("warmup summary", func(t *testing.T) {
		c := recordingFixtureClient(t, &got, `{
			"email_account_id": "em_1", "email": "a@acme.com",
			"date_range": {"from": "2026-09-01T00:00:00Z", "to": "2026-09-02T00:00:00Z"},
			"summary": {"total_sent": 30, "total_replied": 10, "total_received": 25, "average_daily": 15, "reply_rate": 33.3, "target_progress": 0.9, "days_active": 2},
			"daily_stats": [{"date": "2026-09-01", "emails_sent": 15, "emails_replied": 5, "emails_received": 12, "target_volume": 16, "active": true}]
		}`)
		w, _, err := c.Analytics.Warmup(ctx, "em_1", time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatal(err)
		}
		if w.Summary.TotalReceived != 25 || w.DailyStats[0].EmailsReceived != 12 || !w.DailyStats[0].Active {
			t.Errorf("warmup = %+v", w)
		}
	})

	t.Run("warmup placement", func(t *testing.T) {
		c := recordingFixtureClient(t, &got, `{
			"email_account_id": "em_1",
			"date_range": {"from": "2026-08-09T00:00:00Z", "to": "2026-09-07T00:00:00Z"},
			"summary": {"sent": 100, "delivered": 90, "inbox": 70, "tabs": 10, "spam": 10, "unknown": 0, "archived": 0, "custom": 0,
				"instrumented_receipts": 90, "observed_receipts": 90, "legacy_uninstrumented_receipts": 0, "rescue_requested": 2,
				"non_spam_metric": null, "rescued": 2, "rescue_confirmed": 1, "rescue_missing": 0, "rescue_failed": 0,
				"rescue_unreported": 1, "unconfirmed": 5, "inbox_rate": 88.89, "spam_rate": 11.11},
			"rate": {"window_days": 7, "min_sample": 20, "scope": "major", "delivered": 30, "inbox": 25, "tabs": 2, "spam": 3,
				"inbox_rate": 90, "band": "good", "metric": null, "other_delivered": 0, "other_inbox_rate": null},
			"daily": [{"date": "2026-09-07", "sent": 5, "delivered": 5, "inbox": 4, "tabs": 0, "spam": 1, "inbox_rate": 80, "spam_rate": 20,
				"rolling_inbox_rate": null, "groups": [{"group": "google", "inbox": 4, "tabs": 0, "spam": 1, "rescued": 0}]}],
			"providers": [{"group": "google", "sent": 5, "delivered": 5, "inbox": 4, "spam": 1, "inbox_rate": 80,
				"hosts": [{"host": "gmail", "sent": 2, "delivered": 2, "inbox": 2, "inbox_rate": 100}]}]
		}`)
		r, _, err := c.Analytics.WarmupPlacement(ctx, &WarmupPlacementParams{
			EmailID: "em_1", From: time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatal(err)
		}
		if got.method != "GET" || got.path != "/v1/analytics/warmup/placement" || got.rawQuery != "email_id=em_1&from=2026-08-09&to=2026-09-07" {
			t.Errorf("request = %s %s ?%s", got.method, got.path, got.rawQuery)
		}
		if r.Rate.Band != WarmupBandGood || r.Rate.InboxRate == nil || r.Rate.OtherInboxRate != nil || r.Summary.RescueUnreported != 1 ||
			r.Summary.NonSpamMetric != nil || r.Summary.InboxRate == nil {
			t.Errorf("report = %+v", r)
		}
		if len(r.Daily) != 1 || r.Daily[0].Inbox != 4 || r.Daily[0].RollingInboxRate != nil || r.Daily[0].Groups[0].Group != WarmupGroupGoogle {
			t.Errorf("daily = %+v", r.Daily)
		}
		if len(r.Providers) != 1 || r.Providers[0].Hosts[0].Host != "gmail" || r.Providers[0].Hosts[0].Delivered != 2 {
			t.Errorf("providers = %+v", r.Providers)
		}

		// With no params no query is sent.
		if _, _, err = c.Analytics.WarmupPlacement(ctx, nil); err != nil {
			t.Fatal(err)
		}
		if got.rawQuery != "" {
			t.Errorf("query = %q", got.rawQuery)
		}
	})

	t.Run("direct", func(t *testing.T) {
		c := recordingFixtureClient(t, &got, `{
			"period": "30d",
			"volume": {"sent": 40, "received": 30, "threads_started": 20, "replied": 8, "reply_rate": 40, "bounced": 1, "median_reply_minutes": 95},
			"tracking": {"mailboxes_opted_in": 1, "mailboxes_total": 3, "tracked_sent": 10, "opened": 6, "machine_opened": 1, "clicked": 2, "open_rate": 60, "click_rate": 20},
			"daily_trend": [{"date": "2026-09-07T00:00:00Z", "sent": 3, "received": 2}],
			"mailboxes": [{"email_account_id": "em_1", "email": "a@acme.com", "track_direct_mail": true, "sent": 40, "received": 30}],
			"top_contacts": [{"email": "j@x.com", "sent": 4, "received": 3, "last_at": "2026-09-06T10:00:00Z"}]
		}`)
		d, _, err := c.Analytics.Direct(ctx, Period30Days)
		if err != nil {
			t.Fatal(err)
		}
		if got.path != "/v1/analytics/direct" || got.rawQuery != "period=30d" {
			t.Errorf("request = %s ?%s", got.path, got.rawQuery)
		}
		if d.Volume.MedianReplyMinutes != 95 || d.Tracking.MailboxesTotal != 3 || len(d.DailyTrend) != 1 || !d.Mailboxes[0].TrackDirectMail || d.TopContacts[0].Sent != 4 {
			t.Errorf("direct = %+v", d)
		}
	})

	t.Run("inbox tagging", func(t *testing.T) {
		c := recordingFixtureClient(t, &got, `{
			"enabled": true,
			"data": [{"id": "v1", "message_id": "m1", "thread_id": "th1", "kind": "reply", "kind_confidence": 0.9, "kind_source": "model",
				"intent": "not_now", "intent_confidence": 0.8, "relevance": 70, "priority": "high", "needs_review": true,
				"review_reason": "low confidence", "labels": ["follow-up"], "answers": {"q1": true}, "model": "m", "input_tokens": 120,
				"actions": ["hold"], "return_date": null, "created_at": "2026-09-07T09:00:00Z"}],
			"total": 41, "summary": {"total": 100, "needs_review": 41, "from_offline": 3, "acted": 12},
			"pagination": {"next_cursor": "c2", "has_more": true}
		}`)
		r, _, err := c.Analytics.InboxTagging(ctx, &InboxTaggingParams{ListOptions: ListOptions{Limit: 20, Cursor: "c1"}, NeedsReview: true})
		if err != nil {
			t.Fatal(err)
		}
		if got.path != "/v1/analytics/inbox-tagging" || got.rawQuery != "cursor=c1&limit=20&needs_review=true" {
			t.Errorf("request = %s ?%s", got.path, got.rawQuery)
		}
		if !r.Enabled || r.Total != 41 || r.Summary.NeedsReview != 41 || r.Data[0].ReturnDate != nil || string(r.Data[0].Answers) != `{"q1": true}` ||
			!r.Pagination.HasMore || r.Pagination.NextCursor == nil || *r.Pagination.NextCursor != "c2" {
			t.Errorf("review = %+v", r)
		}
	})

	t.Run("account statuses page", func(t *testing.T) {
		c := recordingFixtureClient(t, &got, `{"data": [{
			"id": "em_1", "email": "a@acme.com", "provider": "google", "status": "active", "last_synced_at": null,
			"health": {"status": "healthy", "score": 90}, "errors": [{"id": "e1", "error_code": "x", "severity": "warn", "title": "T", "message": "M",
				"created_at": "2026-09-07T09:00:00Z", "operation": "sync"}],
			"daily_usage": {"date": "2026-09-07", "campaign_sent": 1, "campaign_limit": 50},
			"warmup_status": {"enabled": true, "paused": false, "started_at": "2026-08-01T00:00:00Z", "current_volume": 5, "target_volume": 10,
				"max_volume": 40, "reply_rate": 30, "days_active": 30,
				"partner_limit": {"reachable": 4, "ramp_target": 12},
				"send_failure": {"message": "auth failed", "at": "2026-09-07T08:00:00Z", "kind": "authentication"},
				"next_send_at": "2026-09-07T10:00:00Z"},
			"warmup_health": {"pool_type": "premium", "source": "cloud", "state": "healthy", "score": 95, "spam_score": 0,
				"partner_mailboxes_7d": 12, "partner_domains_7d": 9, "partner_organizations_7d": 8, "received_7d": 40, "senders_7d": 11},
			"warmup_placement": {"window_days": 7, "min_sample": 20, "scope": "major", "delivered": 25, "inbox": 24, "tabs": 0, "spam": 1,
				"inbox_rate": 96, "band": "good", "metric": null, "other_delivered": 0, "other_inbox_rate": null},
			"in_campaign": true, "send_recovery_reason": "authentication"
		}], "pagination": {"total": null, "next_cursor": "n", "has_more": true}}`)
		page, err := c.Analytics.ListAccounts(ctx, &AccountStatusListParams{ListOptions: ListOptions{Limit: 100}, EmailIDs: []string{"em_1", "em_2"}})
		if err != nil {
			t.Fatal(err)
		}
		if got.path != "/v1/analytics/accounts" || got.rawQuery != "email_ids=em_1%2Cem_2&limit=100" {
			t.Errorf("request = %s ?%s", got.path, got.rawQuery)
		}
		a := page.Data[0]
		if a.Placement == nil || a.Placement.Band != WarmupBandGood || a.SendRecoveryReason != "authentication" || a.Errors[0].Operation != "sync" {
			t.Errorf("account = %+v", a)
		}
		ws, wh := a.WarmupStatus, a.WarmupHealth
		if ws.PartnerLimit == nil || ws.PartnerLimit.RampTarget != 12 || ws.SendFailure == nil || ws.SendFailure.Kind != "authentication" || ws.NextSendAt == nil {
			t.Errorf("warmup status = %+v", ws)
		}
		if wh.PoolType != "premium" || wh.Source != "cloud" || wh.PartnerDomains7d != 9 || wh.Senders7d != 11 {
			t.Errorf("warmup health = %+v", wh)
		}
		if !page.HasMore() {
			t.Error("HasMore = false")
		}
	})

	t.Run("deliverability", func(t *testing.T) {
		c := recordingFixtureClient(t, &got, `{
			"from": "2026-09-01T00:00:00Z", "to": "2026-09-07T00:00:00Z", "intent_automated": 4, "band": "healthy", "score": 90,
			"by_provider": [{"provider": "gmail", "label": "Gmail", "samples": 10, "inbox": 8, "promotions": 1, "spam": 0, "other": 0, "missing": 1, "inbox_rate": 80, "spam_rate": 0}],
			"warmup_placement": [{"provider": "gmail", "label": "Gmail", "delivered": 50, "spam": 2, "inbox_rate": 96, "spam_rate": 4}]
		}`)
		d, _, err := c.Analytics.Deliverability(ctx, time.Time{}, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if d.IntentAutomated != 4 || d.ByProvider[0].Missing != 1 || d.ByProvider[0].Label != "Gmail" || d.WarmupPlacement[0].Label != "Gmail" {
			t.Errorf("deliverability = %+v", d)
		}
	})
}

func TestEstimateAndStepSync(t *testing.T) {
	ctx := context.Background()
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{
		"recipients": 100, "mailboxes": 3, "daily_capacity": 90, "remaining_today": 80, "sending_days": 2,
		"estimated_finish_at": "2026-09-09T00:00:00Z", "steps": 3, "total_sends": 300, "first_touch_finish_at": "2026-09-09T00:00:00Z",
		"steady_capacity": 150, "full_capacity_at": null, "ramping": 1, "held": 0,
		"warmup": {"mailboxes": 3, "per_day": 30, "per_day_without": 45, "reply_rate": 30},
		"other_campaigns_per_day": 10, "bottleneck": "a_new_clamp",
		"timeline": [{"date": "2026-09-07", "sending_day": true, "capacity": 90, "sends": 80, "first_emails": 80, "follow_ups": 0, "warmup": 30}],
		"senders": [{"id": "em_1", "email": "a@acme.com", "provider": "google", "state": "ramping", "first_day_cap": 20, "steady_cap": 50,
			"warmup_per_day": 10, "warmup_without": 15, "full_cap_at": "2026-09-20T00:00:00Z"}],
		"scenarios": [{"kind": "more_mailboxes", "value": 2, "finish_at": null}]
	}`)
	r, _, err := c.Campaigns.Estimate(ctx, &CampaignEstimateParams{
		SegmentIDs: []string{"seg_1"}, StartTime: String("09:00"), EndTime: String("17:00"), StepWaits: []int{3, 5}, CampaignID: String("camp_1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"segment_ids":["seg_1"],"start_time":"09:00","end_time":"17:00","step_waits":[3,5],"campaign_id":"camp_1"}`
	if strings.TrimSpace(got.body) != want {
		t.Errorf("body = %s", got.body)
	}
	if r.TotalSends != 300 || r.FullCapacityAt != nil || r.Warmup.PerDayWithout != 45 || r.Bottleneck != "a_new_clamp" ||
		len(r.Timeline) != 1 || r.Senders[0].State != EstimateSenderRamping || r.Senders[0].FullCapAt == nil ||
		r.Scenarios[0].Kind != EstimateScenarioMoreMailboxes || r.Scenarios[0].FinishAt != nil {
		t.Errorf("estimate = %+v", r)
	}

	c = recordingFixtureClient(t, &got, `{
		"id": "camp_1", "user_id": "u1", "name": "Q3", "status": "active", "timezone": "", "effective_timezone": "Europe/Berlin",
		"entry_delay_minutes": 30, "schedule_windows": [[],[],[],[],[],[],[]]
	}`)
	camp, _, err := c.Campaigns.Update(ctx, "camp_1", &CampaignUpdateParams{EntryDelayMinutes: Int(30)})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got.body) != `{"entry_delay_minutes":30}` || camp.EffectiveTimezone != "Europe/Berlin" || camp.EntryDelayMinutes != 30 || camp.Timezone != "" {
		t.Errorf("campaign = %s %+v", got.body, camp)
	}

	c = recordingFixtureClient(t, &got, `{"id": "s1", "name": "Follow-up", "thread_reply": true, "kind": "email"}`)
	st, _, err := c.Campaigns.UpdateStep(ctx, "camp_1", "s1", &StepUpdateParams{ThreadReply: Bool(false)})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got.body) != `{"thread_reply":false}` || !st.ThreadReply {
		t.Errorf("step = %s %+v", got.body, st)
	}
}

func TestOutreachSettingsRoundTrip(t *testing.T) {
	// Update replaces the settings wholesale, so a settings object read from
	// the server must serialize back with the inbox tagging and reply intent
	// fields intact.
	const in = `{
		"reply_intent": {"enabled": true, "crm_task_intents": ["positive"], "hold_on_out_of_office": true, "out_of_office_hold_days": 7},
		"inbox_tagging": {"hold_on_not_now": true, "not_now_hold_days": 14, "stop_on_declined": true, "task_on_call_request": true,
			"suppress_on_removal_request": false, "languages": ["de"], "action_required_in_inbox": true,
			"questions": [{"id": "q1", "type": "choice", "question": "Budget?", "action": {"type": ""},
				"choices": [{"label": "high", "description": "d", "action": {"type": "hold", "hold_days": 30}}, {"label": "low", "description": "d", "action": {"type": "task"}}]}]}
	}`
	var s OutreachSettings
	if err := json.Unmarshal([]byte(in), &s); err != nil {
		t.Fatal(err)
	}
	if !s.ReplyIntent.HoldOnOutOfOffice || s.ReplyIntent.OutOfOfficeHoldDays != 7 || len(s.ReplyIntent.CRMTaskIntents) != 1 ||
		s.InboxTagging.NotNowHoldDays != 14 || len(s.InboxTagging.Questions) != 1 || s.InboxTagging.Questions[0].Choices[0].Action.HoldDays != 30 {
		t.Fatalf("settings = %+v", s)
	}
	out, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"crm_task_intents":["positive"]`, `"hold_on_out_of_office":true`, `"inbox_tagging":{"hold_on_not_now":true`, `"hold_days":30`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("marshaled settings lack %s: %s", want, out)
		}
	}
	// An absent intents list stays null so the server applies its default.
	var empty OutreachSettings
	out, _ = json.Marshal(empty)
	if !strings.Contains(string(out), `"crm_task_intents":null`) {
		t.Errorf("empty settings = %s", out)
	}
}

func TestAPIKeyDeletePermanently(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"status": "deleted"}`)
	if _, err := c.APIKeys.DeletePermanently(context.Background(), "key_1"); err != nil {
		t.Fatal(err)
	}
	if got.method != "DELETE" || got.path != "/v1/api-keys/key_1/permanent" || got.rawQuery != "" {
		t.Errorf("request = %s %s ?%s", got.method, got.path, got.rawQuery)
	}
}
