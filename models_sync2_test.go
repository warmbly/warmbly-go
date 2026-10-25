package warmbly

import (
	"context"
	"testing"
	"time"
)

func TestAccountStatusSendHoldAndNextSend(t *testing.T) {
	const body = `{"id":"em_1","email":"dana@acme.com","provider":"smtp_imap","status":"active","last_synced_at":null,
	  "health":{"status":"warning","score":80,"issues":[]},
	  "daily_usage":{"date":"2026-10-24","campaign_sent":0,"campaign_limit":50,"warmup_sent":2,"warmup_limit":20},
	  "warmup_status":{"enabled":true,"paused":false,"started_at":"2026-08-01T00:00:00Z","current_volume":4,"target_volume":12,"max_volume":40,"reply_rate":30,"days_active":23,
	    "next_send_at":"2026-06-12T14:37:12Z"},
	  "warmup_health":{"state":"blocked","score":0,"reason":"cloud_evidence_unavailable","source":"cloud","spam_score":0,
	    "partner_mailboxes_7d":0,"partner_domains_7d":0,"partner_organizations_7d":0,"received_7d":0,"senders_7d":0},
	  "send_recovery_reason":"authentication","in_campaign":false}`
	var got recordedRequest
	c := recordingFixtureClient(t, &got, body)
	a, _, err := c.Analytics.Account(context.Background(), "em_1")
	if err != nil {
		t.Fatal(err)
	}
	if got.method != "GET" || got.path != "/v1/analytics/accounts/em_1" || got.rawQuery != "" {
		t.Errorf("sent %s %s ?%s", got.method, got.path, got.rawQuery)
	}
	if a.SendRecoveryReason != "authentication" {
		t.Errorf("SendRecoveryReason = %q", a.SendRecoveryReason)
	}
	if want := time.Date(2026, 6, 12, 14, 37, 12, 0, time.UTC); a.WarmupStatus == nil || a.WarmupStatus.NextSendAt == nil || !a.WarmupStatus.NextSendAt.Equal(want) {
		t.Errorf("NextSendAt = %+v", a.WarmupStatus)
	}
	if a.WarmupHealth == nil || a.WarmupHealth.Reason != WarmupReasonCloudEvidenceUnavailable || a.WarmupHealth.Source != "cloud" {
		t.Errorf("warmup health = %+v", a.WarmupHealth)
	}

	// Absent while nothing is queued or held, and a new reason decodes as is.
	c = recordingFixtureClient(t, &got, `{"id":"em_2","email":"x@acme.com","provider":"google","status":"active","last_synced_at":null,
	  "health":{"status":"healthy","score":100,"issues":[]},"daily_usage":{},"warmup_status":{"enabled":false,"paused":true,"started_at":"2026-08-01T00:00:00Z"},
	  "send_recovery_reason":"quarantine","in_campaign":false}`)
	b, _, err := c.Analytics.Account(context.Background(), "em_2")
	if err != nil {
		t.Fatal(err)
	}
	if b.WarmupStatus.NextSendAt != nil || b.SendRecoveryReason != "quarantine" {
		t.Errorf("got %+v / %+v", b.WarmupStatus, b.SendRecoveryReason)
	}
}

func TestWarmupPlacementRescueOutcomesDecode(t *testing.T) {
	const body = `{"date_range":{"from":"2026-10-18T00:00:00Z","to":"2026-10-24T00:00:00Z"},
	  "summary":{"sent":50,"delivered":47,"inbox":40,"tabs":1,"spam":6,"rescued":44,"rescue_confirmed":41,"rescue_missing":1,"rescue_failed":0,"rescue_unreported":2,
	    "unconfirmed":12,"inbox_rate":93.93,"spam_rate":6.07},
	  "rate":{"window_days":7,"min_sample":20,"scope":"major","delivered":47,"inbox":40,"tabs":1,"spam":6,"inbox_rate":87,"band":"good","other_delivered":0,"other_inbox_rate":null},
	  "daily":[{"date":"2026-10-24","sent":10,"delivered":9,"inbox":8,"tabs":0,"spam":1,"rescued":1,"rescue_confirmed":1,"rescue_missing":0,"rescue_failed":0,"rescue_unreported":0,
	    "unconfirmed":0,"inbox_rate":88.9,"spam_rate":11.1,"rolling_inbox_rate":null,
	    "groups":[{"group":"microsoft","inbox":12,"tabs":0,"spam":1,"rescued":1,"rescue_confirmed":1,"rescue_missing":0,"rescue_failed":0}]}],
	  "providers":[]}`
	var got recordedRequest
	c := recordingFixtureClient(t, &got, body)
	r, _, err := c.Analytics.WarmupPlacement(context.Background(), "", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	s := r.Summary
	if s.Rescued != 44 || s.RescueConfirmed != 41 || s.RescueMissing != 1 || s.RescueFailed != 0 || s.RescueUnreported != 2 {
		t.Errorf("summary = %+v", s)
	}
	g := r.Daily[0].Groups[0]
	if g.Group != WarmupRecipientMicrosoft || g.RescueConfirmed != 1 || g.RescueMissing != 0 || g.RescueFailed != 0 {
		t.Errorf("group = %+v", g)
	}
}

func TestFormRedirectURLActiveDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"id":"f_1","name":"Signup","redirect_url":"https://acme.com/thanks","redirect_url_active":false}`)
	f, _, err := c.Forms.Get(context.Background(), "f_1")
	if err != nil {
		t.Fatal(err)
	}
	if f.RedirectURLActive == nil || *f.RedirectURLActive {
		t.Errorf("RedirectURLActive = %v, want false", f.RedirectURLActive)
	}
	c = recordingFixtureClient(t, &got, `{"id":"f_2","name":"Plain","redirect_url":""}`)
	f, _, err = c.Forms.Get(context.Background(), "f_2")
	if err != nil {
		t.Fatal(err)
	}
	if f.RedirectURLActive != nil {
		t.Errorf("RedirectURLActive = %v, want nil when no redirect is set", *f.RedirectURLActive)
	}
}

func TestCloudLinkMailboxRowSyncFields(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"data":[{"id":"em_1","email":"dana@acme.com","name":"Dana","provider":"smtp_imap","status":"active",
	  "enrolled":true,"managed":false,"legacy":false,"independent_smtp":true,"enrollment_state":"active",
	  "settings_observed_at":"2026-10-24T09:00:00Z"}]}`)
	rows, _, err := c.CloudLink.ListMailboxes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.method != "GET" || got.path != "/v1/cloud-link/mailboxes" {
		t.Errorf("sent %s %s", got.method, got.path)
	}
	if len(rows) != 1 || !rows[0].IndependentSMTP || rows[0].SettingsObservedAt == nil || rows[0].SettingsObservedAt.Hour() != 9 {
		t.Errorf("rows = %+v", rows)
	}

	c = recordingFixtureClient(t, &got, `{"data":[{"id":"cw_1","email":"ops@acme.com","name":"Ops","provider":"smtp_imap","status":"active","mail_host":"mail.acme.com"}]}`)
	ws, _, err := c.CloudLink.ListWorkspaceMailboxes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 1 || ws[0].MailHost != "mail.acme.com" || ws[0].Provider != ProviderSMTPIMAP {
		t.Errorf("workspace mailboxes = %+v", ws)
	}
}

func TestSyncCatalogueValues(t *testing.T) {
	for _, tc := range [][2]string{
		{SendLimitCloudStatusHold, "cloud_status_hold"},
		{SendLimitRecovery, "send_recovery"},
		{SendLimitCooldown, "send_cooldown"},
		{MailboxPlanCloudStatusHold, "cloud_status_hold"},
		{MailboxPlanRecovery, "send_recovery"},
		{MailboxPlanCooldown, "send_cooldown"},
		{MailboxPlanAdmission, "send_authority"},
		{AuditEntityOAuthApplication, "oauth_application"},
		{AccountErrorTLSCertificateChanged, "TLS_CERTIFICATE_CHANGED"},
		{AccountErrorTLSCertificateUnconfirmed, "TLS_CERTIFICATE_UNCONFIRMED"},
		{ErrCodeSessionEnded, "session_ended"},
		{ErrCodeInvalidTLSPin, "invalid_tls_pin"},
		{ErrCodeMailboxCertificateNotPinned, "mailbox_certificate_not_pinned"},
		{ErrCodeRedirectTargetNotOwnDomain, "redirect_target_not_own_domain"},
		{ErrCodeAccountSuspended, "account_suspended"},
		{ErrCodeWorkspaceSendingSuspended, "workspace_sending_suspended"},
		{ErrCodeDuplicateQueryParameter, "duplicate_query_parameter"},
		{ErrCodeImageUnreadable, "image_unreadable"},
		{ErrCodeInvitationExpired, "invitation_expired"},
		{ErrCodeCloudWarmupManaged, "cloud_warmup_managed"},
		{ErrCodePoolLinkAlreadyAdopted, "pool_link_already_adopted"},
		{ErrCodeSessionsNotEnded, "sessions_not_ended"},
		{ErrCodeCodeLookupLimited, "code_lookup_limited"},
		{ErrReasonCertificateUntrusted, "certificate_untrusted"},
	} {
		if tc[0] != tc[1] {
			t.Errorf("constant = %q, want %q", tc[0], tc[1])
		}
	}
}
