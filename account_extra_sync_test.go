package warmbly

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAccountExtraRouting(t *testing.T) {
	ctx := context.Background()
	day := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		body       string
		call       func(c *Client) error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string
	}{
		// Unibox.
		{"Unibox.MoveFolder", `{"email_ids":["m1"],"thread_ids":["t1"],"folder":"archive"}`,
			func(c *Client) error {
				r, _, e := c.Unibox.MoveFolder(ctx, &UniboxMoveFolderParams{EmailIDs: []string{"m1"}, ThreadIDs: []string{"t1"}, Folder: FolderArchive})
				if e == nil && (r.Folder != FolderArchive || r.ThreadIDs[0] != "t1") {
					t.Errorf("result = %+v", r)
				}
				return e
			},
			"PATCH", "/v1/unibox/folder", "", `{"email_ids":["m1"],"thread_ids":["t1"],"folder":"archive"}`},
		{"Unibox.UpsertThreadDraft", `{"id":"d1","status":"pending","created":true}`,
			func(c *Client) error {
				r, _, e := c.Unibox.UpsertThreadDraft(ctx, &ThreadDraftParams{ThreadID: "th1", Body: "Thanks!", Source: "my-agent", Confidence: Float64(0.9)})
				if e == nil && (r.ID != "d1" || !r.Created || r.Status != "pending") {
					t.Errorf("result = %+v", r)
				}
				return e
			},
			"PUT", "/v1/unibox/ai-drafts/by-thread", "", `{"thread_id":"th1","body":"Thanks!","source":"my-agent","confidence":0.9}`},
		// API keys.
		{"APIKeys.Delete", `{"status":"deleted"}`,
			func(c *Client) error { _, e := c.APIKeys.Delete(ctx, "key_1"); return e },
			"DELETE", "/v1/api-keys/key_1/permanent", "", ""},
		// OAuth apps.
		{"OAuthApps.Listing", `{"listing":null}`,
			func(c *Client) error {
				l, _, e := c.OAuthApps.Listing(ctx, "app_1")
				if e == nil && l != nil {
					t.Errorf("listing = %+v, want nil", l)
				}
				return e
			},
			"GET", "/v1/oauth/applications/app_1/listing", "", ""},
		{"OAuthApps.PutListing", `{"listing":{"application_id":"app_1","slug":"acme","status":"featured","submitted_at":"2026-11-01T00:00:00Z"}}`,
			func(c *Client) error {
				l, _, e := c.OAuthApps.PutListing(ctx, "app_1", &AppListingParams{Slug: "acme", Tagline: "t", Category: "crm"})
				if e == nil && (l.Slug != "acme" || l.Status != AppListingFeatured) {
					t.Errorf("listing = %+v", l)
				}
				return e
			},
			"PUT", "/v1/oauth/applications/app_1/listing", "", `{"slug":"acme","tagline":"t","description":"","category":"crm","install_url":"","support_url":"","privacy_url":""}`},
		{"OAuthApps.DeleteListing", `{"deleted":true}`,
			func(c *Client) error { _, e := c.OAuthApps.DeleteListing(ctx, "app_1"); return e },
			"DELETE", "/v1/oauth/applications/app_1/listing", "", ""},
		{"OAuthApps.RemoveLogo", `{"id":"app_1","logo_url":"","status":"disabled","suspended_at":"2026-11-01T00:00:00Z","suspended_reason":"abuse"}`,
			func(c *Client) error {
				a, _, e := c.OAuthApps.RemoveLogo(ctx, "app_1")
				if e == nil && (a.SuspendedAt == nil || a.SuspendedReason != "abuse" || a.Status != OAuthAppDisabled) {
					t.Errorf("app = %+v", a)
				}
				return e
			},
			"DELETE", "/v1/oauth/applications/app_1/logo", "", ""},
		// Auth.
		{"Auth.LinkedIdentities", `{"data":[{"provider":"google","label":"Google","email":"a@b.co","created_at":"2026-11-01T00:00:00Z"}]}`,
			func(c *Client) error {
				ids, _, e := c.Auth.LinkedIdentities(ctx)
				if e == nil && (len(ids) != 1 || ids[0].Provider != "google") {
					t.Errorf("ids = %+v", ids)
				}
				return e
			},
			"GET", "/v1/auth/me/identities", "", ""},
		{"Auth.UnlinkIdentity", ``,
			func(c *Client) error { _, e := c.Auth.UnlinkIdentity(ctx, "google"); return e },
			"DELETE", "/v1/auth/me/identities/google", "", ""},
		{"Auth.SendPasswordLink", ``,
			func(c *Client) error { _, e := c.Auth.SendPasswordLink(ctx); return e },
			"POST", "/v1/auth/me/password/link", "", ""},
		{"Auth.RegenerateRecoveryCodes", `{"recovery_codes":["a","b"]}`,
			func(c *Client) error {
				r, _, e := c.Auth.RegenerateRecoveryCodes(ctx, "123456")
				if e == nil && len(r.RecoveryCodes) != 2 {
					t.Errorf("codes = %+v", r)
				}
				return e
			},
			"POST", "/v1/auth/2fa/recovery-codes", "", `{"code":"123456"}`},
		{"Auth.LoginInitialPassword", `{"code_required":true,"session":"s1"}`,
			func(c *Client) error {
				s, _, e := c.Auth.LoginInitialPassword(ctx, "pend", "new-pass", "dev")
				if e == nil && (!s.CodeRequired || s.Done()) {
					t.Errorf("step = %+v", s)
				}
				return e
			},
			"POST", "/v1/auth/login/initial-password", "", `{"pending_token":"pend","password":"new-pass","device_token":"dev"}`},
		{"Auth.LinkSSO", `{"access_token":"a","refresh_token":"r"}`,
			func(c *Client) error {
				s, _, e := c.Auth.LinkSSO(ctx, "pend", "pw")
				if e == nil && s.AccessToken != "a" {
					t.Errorf("session = %+v", s)
				}
				return e
			},
			"POST", "/v1/auth/sso/link", "", `{"pending_token":"pend","password":"pw"}`},
		{"Auth.ResetPasswordSession", `{"session":"rs","expires_at":"2026-11-08T12:00:00Z"}`,
			func(c *Client) error {
				s, _, e := c.Auth.ResetPasswordSession(ctx, "link-token")
				if e == nil && s.Session != "rs" {
					t.Errorf("reset = %+v", s)
				}
				return e
			},
			"POST", "/v1/auth/reset-password/session", "", `{"token":"link-token"}`},
		{"Auth.ReauthMethods", `{"password":true,"totp":false,"passkey":true,"providers":[{"id":"google","label":"Google"}],"native_providers":["apple"],"valid_for_seconds":300}`,
			func(c *Client) error {
				m, _, e := c.Auth.ReauthMethods(ctx)
				if e == nil && (!m.Password || m.Providers[0].ID != "google" || m.NativeProviders[0] != "apple" || m.ValidForSeconds != 300) {
					t.Errorf("methods = %+v", m)
				}
				return e
			},
			"GET", "/v1/auth/reauth/methods", "", ""},
		{"Auth.Reauth", `{"valid_for_seconds":300}`,
			func(c *Client) error {
				r, _, e := c.Auth.Reauth(ctx, "pw", "")
				if e == nil && r.ValidForSeconds != 300 {
					t.Errorf("result = %+v", r)
				}
				return e
			},
			"POST", "/v1/auth/reauth", "", `{"password":"pw"}`},
		{"Auth.ReauthPasskeyBegin", `{"options":{"publicKey":{"challenge":"x"}}}`,
			func(c *Client) error {
				o, _, e := c.Auth.ReauthPasskeyBegin(ctx)
				if e == nil && !strings.Contains(string(o), "challenge") {
					t.Errorf("options = %s", o)
				}
				return e
			},
			"POST", "/v1/auth/reauth/passkey/begin", "", ""},
		{"Auth.ReauthPasskeyFinish", `{"valid_for_seconds":300}`,
			func(c *Client) error {
				_, _, e := c.Auth.ReauthPasskeyFinish(ctx, json.RawMessage(`{"id":"cred"}`))
				return e
			},
			"POST", "/v1/auth/reauth/passkey/finish", "", `{"credential":{"id":"cred"}}`},
		{"Auth.ReauthSSOBegin", `{"url":"https://idp/authorize","binding":"b"}`,
			func(c *Client) error {
				r, _, e := c.Auth.ReauthSSOBegin(ctx, "google")
				if e == nil && r.Binding != "b" {
					t.Errorf("redirect = %+v", r)
				}
				return e
			},
			"POST", "/v1/auth/reauth/sso/begin", "", `{"provider":"google"}`},
		{"Auth.ReauthSSOFinish", `{"valid_for_seconds":300}`,
			func(c *Client) error { _, _, e := c.Auth.ReauthSSOFinish(ctx, "code", "b"); return e },
			"POST", "/v1/auth/reauth/sso/finish", "", `{"code":"code","binding":"b"}`},
		{"Auth.ReauthNativeBegin", `{"nonce":"n1"}`,
			func(c *Client) error {
				n, _, e := c.Auth.ReauthNativeBegin(ctx)
				if e == nil && n != "n1" {
					t.Errorf("nonce = %q", n)
				}
				return e
			},
			"POST", "/v1/auth/reauth/native/begin", "", ""},
		{"Auth.ReauthNative", `{"valid_for_seconds":300}`,
			func(c *Client) error { _, _, e := c.Auth.ReauthNative(ctx, "apple", "idtok"); return e },
			"POST", "/v1/auth/reauth/native", "", `{"provider":"apple","id_token":"idtok"}`},
		{"Auth.ChangePassword", `{"access_token":"new","refresh_token":"newr"}`,
			func(c *Client) error {
				s, _, e := c.Auth.ChangePasswordWith(ctx, &ChangePasswordParams{CurrentPassword: "a", NewPassword: "b", RevokeAPICredentials: true})
				if e == nil && s.AccessToken != "new" {
					t.Errorf("session = %+v", s)
				}
				return e
			},
			"POST", "/v1/auth/me/password", "", `{"current_password":"a","new_password":"b","revoke_api_credentials":true}`},
		// Analytics.
		{"Analytics.DirectMail", `{"period":"30d","volume":{"sent":10,"received":4,"threads_started":6,"replied":2,"reply_rate":33.3,"bounced":1,"median_reply_minutes":90},"tracking":{"mailboxes_opted_in":1,"mailboxes_total":3,"tracked_sent":5,"opened":2,"machine_opened":1,"clicked":1,"open_rate":40,"click_rate":20},"daily_trend":[{"date":"2026-11-01T00:00:00Z","sent":2,"received":1}],"mailboxes":[{"email_account_id":"em_1","email":"a@b.co","track_direct_mail":true,"sent":10,"received":4}],"top_contacts":[{"email":"x@y.co","sent":3,"received":1,"last_at":"2026-11-02T00:00:00Z"}]}`,
			func(c *Client) error {
				d, _, e := c.Analytics.DirectMail(ctx, Period30Days)
				if e == nil && (d.Volume.MedianReplyMinutes != 90 || d.Tracking.MailboxesTotal != 3 || !d.Mailboxes[0].TrackDirectMail || d.TopContacts[0].Email != "x@y.co") {
					t.Errorf("direct = %+v", d)
				}
				return e
			},
			"GET", "/v1/analytics/direct", "period=30d", ""},
		{"Analytics.InboxTagging", `{"enabled":true,"data":[{"id":"r1","message_id":"m","thread_id":"t","kind":"human","kind_confidence":0.9,"kind_source":"model","intent":"interested","intent_confidence":0.8,"relevance":3,"priority":"high","needs_review":true,"review_reason":"low","labels":["a"],"answers":{"q":"a"},"model":"x","input_tokens":10,"actions":["hold"],"return_date":"2026-12-01","created_at":"2026-11-01T10:00:00Z"}],"total":1,"summary":{"total":1,"needs_review":1,"from_offline":0,"acted":0},"pagination":{"next_cursor":"c2","has_more":true}}`,
			func(c *Client) error {
				r, _, e := c.Analytics.InboxTagging(ctx, &InboxTaggingParams{ListOptions: ListOptions{Limit: 20, Cursor: "c1"}, NeedsReview: true})
				if e == nil && (!r.Enabled || r.NextCursor() != "c2" || r.Data[0].ReturnDate == nil || *r.Data[0].ReturnDate != "2026-12-01" || r.Summary.NeedsReview != 1 || !strings.Contains(string(r.Data[0].Answers), `"q"`)) {
					t.Errorf("review = %+v", r)
				}
				return e
			},
			"GET", "/v1/analytics/inbox-tagging", "cursor=c1&limit=20&needs_review=true", ""},
		{"Analytics.WarmupPlacement", `{"email_account_id":"em_1","date_range":{"from":"2026-10-10T00:00:00Z","to":"2026-11-08T00:00:00Z"},"summary":{"sent":10,"delivered":8,"inbox":6,"tabs":1,"spam":1,"inbox_rate":87.5,"spam_rate":12.5,"non_spam_metric":null},"rate":{"window_days":7,"min_sample":20,"scope":"major","delivered":8,"inbox":6,"tabs":1,"spam":1,"inbox_rate":null,"band":"collecting","metric":null,"other_delivered":0,"other_inbox_rate":null},"daily":[{"date":"2026-11-07","sent":3,"delivered":2,"inbox":2,"tabs":0,"spam":0,"rolling_inbox_rate":null,"groups":[{"group":"google","inbox":2,"tabs":0,"spam":0}]}],"providers":[{"group":"google","delivered":8,"inbox":6,"hosts":[{"host":"gmail.com","delivered":8,"inbox":6}]}]}`,
			func(c *Client) error {
				r, _, e := c.Analytics.WarmupPlacement(ctx, "em_1", day, day.AddDate(0, 0, 7))
				if e == nil && (*r.EmailAccountID != "em_1" || r.Rate.Band != WarmupPlacementBandCollecting || r.Rate.InboxRate != nil || *r.Summary.InboxRate != 87.5 ||
					r.Daily[0].Groups[0].Group != WarmupRecipientGoogle || r.Providers[0].Hosts[0].Host != "gmail.com" || r.Daily[0].RollingInboxRate != nil) {
					t.Errorf("report = %+v", r)
				}
				return e
			},
			"GET", "/v1/analytics/warmup/placement", "email_id=em_1&from=2026-11-01&to=2026-11-08", ""},
		{"Analytics.DashboardFiltered", `{"period":"7d","capacity_today":{"capacity":100,"remaining_today":40,"configured_ceiling":120,"mailboxes":3,"held":1},"scope":{"campaigns":[{"id":"c1","name":"A"}],"folders":[],"campaign_count":1},"recent_activity":[{"type":"reply","sender_id":"em_1","sender_email":"a@b.co","origin":{"client":"Gmail"}}]}`,
			func(c *Client) error {
				d, _, e := c.Analytics.DashboardFiltered(ctx, Period7Days, &DashboardFilter{CampaignIDs: []string{"c1", "c2"}, FolderIDs: []string{"f1"}})
				if e == nil && (d.CapacityToday.Remaining != 40 || d.Scope.CampaignCount != 1 || *d.RecentActivity[0].SenderID != "em_1" || d.RecentActivity[0].Origin.Client != "Gmail") {
					t.Errorf("dashboard = %+v", d)
				}
				return e
			},
			"GET", "/v1/analytics/dashboard", "campaign_ids=c1%2Cc2&folder_ids=f1&period=7d", ""},
		{"Analytics.CampaignRange", `{"campaign_id":"c1","summary":{"positive_replies":3,"interested_leads":2,"reply_breakdown":{"positive":3,"out_of_office":1}},"steps":[{"step_id":"s1","machine_opens":2,"positive_reply_rate":1.5}],"engagement":{"countries":[],"clients":[],"devices":[],"surfaces":[{"key":"webmail","opens":1,"clicks":0}]}}`,
			func(c *Client) error {
				a, _, e := c.Analytics.CampaignRange(ctx, "c1", day, day.AddDate(0, 0, 7))
				if e == nil && (a.Summary.PositiveReplies != 3 || a.Summary.ReplyBreakdown.OutOfOffice != 1 || a.Steps[0].MachineOpens != 2 || a.Engagement.Surfaces[0].Key != "webmail") {
					t.Errorf("analytics = %+v", a)
				}
				return e
			},
			"GET", "/v1/analytics/campaigns/c1", "from=2026-11-01&to=2026-11-08", ""},
		{"Analytics.AccountsPage", `{"data":[{"id":"em_1","send_pause":{"reason":"rate_limited","held":false,"resumable":true},"send_recovery_reason":"domain_auth","warmup_placement":{"band":"good","delivered":30},"errors":[{"id":"e1","operation":"send"}],"warmup_status":{"partner_limit":{"reachable":3,"ramp_target":10},"next_send_at":"2026-11-08T12:00:00Z"},"warmup_health":{"pool_type":"premium","received_7d":4}}],"pagination":{"has_more":true,"next_cursor":"n"}}`,
			func(c *Client) error {
				p, e := c.Analytics.AccountsPage(ctx, &AccountsParams{ListOptions: ListOptions{Limit: 50}, EmailIDs: []string{"em_1", "em_2"}})
				if e == nil {
					a := p.Data[0]
					if !a.SendPause.Resumable || a.SendRecoveryReason != "domain_auth" || a.Placement.Band != WarmupPlacementBandGood || a.Errors[0].Operation != "send" ||
						a.WarmupStatus.PartnerLimit.RampTarget != 10 || a.WarmupStatus.NextSendAt == nil || a.WarmupHealth.PoolType != "premium" || a.WarmupHealth.Received7d != 4 || !p.HasMore() {
						t.Errorf("account = %+v", a)
					}
				}
				return e
			},
			"GET", "/v1/analytics/accounts", "email_ids=em_1%2Cem_2&limit=50", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got recordedRequest
			c := recordingFixtureClient(t, &got, tc.body)
			if err := tc.call(c); err != nil {
				t.Fatalf("call: %v", err)
			}
			if got.method != tc.wantMethod || got.path != tc.wantPath {
				t.Errorf("request = %s %s, want %s %s", got.method, got.path, tc.wantMethod, tc.wantPath)
			}
			if got.rawQuery != tc.wantQuery {
				t.Errorf("query = %q, want %q", got.rawQuery, tc.wantQuery)
			}
			if strings.TrimSpace(got.body) != tc.wantBody {
				t.Errorf("body = %q, want %q", got.body, tc.wantBody)
			}
		})
	}
}

func TestAnalyticsAccountsFollowsPages(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("cursor") == "" {
			_, _ = w.Write([]byte(`{"data":[{"id":"em_1"}],"pagination":{"has_more":true,"next_cursor":"n2"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"em_2"}],"pagination":{"has_more":false,"next_cursor":null}}`))
	}))
	defer srv.Close()
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatal(err)
	}
	all, _, err := c.Analytics.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].ID != "em_1" || all[1].ID != "em_2" {
		t.Errorf("accounts = %+v", all)
	}
	if len(queries) != 2 || queries[1] != "cursor=n2" {
		t.Errorf("queries = %v", queries)
	}
}

func TestSessionAndAuthStepNewFields(t *testing.T) {
	var s Session
	if err := json.Unmarshal([]byte(`{"device_token":"dt","link_required":true,"link_email":"a@b.co","link_provider":"google","pending_token":"p"}`), &s); err != nil {
		t.Fatal(err)
	}
	if s.DeviceToken != "dt" || !s.LinkRequired || s.LinkEmail != "a@b.co" || s.LinkProvider != "google" {
		t.Errorf("session = %+v", s)
	}
	var step AuthStep
	if err := json.Unmarshal([]byte(`{"password_change_required":true,"pending_token":"p","code_required":false}`), &step); err != nil {
		t.Fatal(err)
	}
	if step.Done() || !step.PasswordChangeRequired {
		t.Errorf("step = %+v", step)
	}
	b, _ := json.Marshal(ConfirmParams{Session: "s", Code: "1", RememberDevice: true})
	if !strings.Contains(string(b), `"remember_device":true`) {
		t.Errorf("confirm = %s", b)
	}
	b, _ = json.Marshal(LoginParams{Email: "a@b.co", DeviceToken: "dt"})
	if !strings.Contains(string(b), `"device_token":"dt"`) {
		t.Errorf("login = %s", b)
	}
}

func TestContactCampaignStateHoldAndCC(t *testing.T) {
	var st ContactCampaignState
	body := `{"campaign_id":"c1","sender_id":"em_1","sender_email":"a@b.co","follow_up_sender":"rotate",
		"hold":{"since":"2026-11-01T00:00:00Z","source":"out_of_office"},
		"cc":[{"contact_id":"ct_2","email":"x@y.co","first_name":"X","last_name":"Y","status":"active"}],
		"steps":[{"id":"s1","sender_email":"a@b.co"}]}`
	if err := json.Unmarshal([]byte(body), &st); err != nil {
		t.Fatal(err)
	}
	if *st.SenderID != "em_1" || st.FollowUpSender != FollowUpRotate || st.Hold.Source != LeadHoldOutOfOffice || st.Hold.Until != nil || !st.CC[0].Copied() || st.Steps[0].SenderEmail != "a@b.co" {
		t.Errorf("state = %+v", st)
	}
}

func TestOAuthAppLogoUploadsUseFileField(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"id":"app_1","logo_url":"https://cdn/x.png","status":"active"}`)
	app, _, err := c.OAuthApps.SetLogo(context.Background(), "app_1", &FileUpload{Filename: "logo.png", Content: strings.NewReader("png")})
	if err != nil {
		t.Fatal(err)
	}
	if got.method != "POST" || got.path != "/v1/oauth/applications/app_1/logo" || app.LogoURL != "https://cdn/x.png" {
		t.Errorf("request = %s %s app %+v", got.method, got.path, app)
	}
	if !strings.Contains(got.body, `name="file"`) {
		t.Errorf("multipart body lacks the file part: %q", got.body)
	}
	c = recordingFixtureClient(t, &got, `{"logo_url":"https://cdn/y.png"}`)
	if _, _, err := c.OAuthApps.UploadLogo(context.Background(), &FileUpload{Filename: "logo.png", Content: strings.NewReader("png")}); err != nil {
		t.Fatal(err)
	}
	if got.path != "/v1/oauth/application-logo" || !strings.Contains(got.body, `name="file"`) {
		t.Errorf("request = %s body %q", got.path, got.body)
	}
}
