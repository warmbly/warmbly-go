//nolint:misspell // fixtures carry the API's "cancelled" wire spelling
package warmbly

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// mbServer answers every request with the given status and body and records
// the last request, including a parsed multipart form when there is one.
type mbRecorded struct {
	method, path, rawQuery, body, contentType string
	form                                      map[string]string
	files                                     map[string]string
}

func mbClient(t *testing.T, status int, response string) (*Client, *mbRecorded) {
	t.Helper()
	got := &mbRecorded{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = mbRecorded{method: r.Method, path: r.URL.Path, rawQuery: r.URL.RawQuery, contentType: r.Header.Get("Content-Type")}
		raw, _ := io.ReadAll(r.Body)
		got.body = string(raw)
		if mt, params, err := mime.ParseMediaType(got.contentType); err == nil && strings.HasPrefix(mt, "multipart/") {
			got.form, got.files = map[string]string{}, map[string]string{}
			mr := multipart.NewReader(strings.NewReader(got.body), params["boundary"])
			for {
				p, err := mr.NextPart()
				if err != nil {
					break
				}
				b, _ := io.ReadAll(p)
				if p.FileName() != "" {
					got.files[p.FormName()] = p.FileName() + "|" + string(b)
				} else {
					got.form[p.FormName()] = string(b)
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status != http.StatusNoContent {
			_, _ = w.Write([]byte(response))
		}
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	return c, got
}

func (g *mbRecorded) want(t *testing.T, method, path, query, body string) {
	t.Helper()
	if g.method != method || g.path != path {
		t.Errorf("got %s %s, want %s %s", g.method, g.path, method, path)
	}
	if g.rawQuery != query {
		t.Errorf("query = %q, want %q", g.rawQuery, query)
	}
	if body == "-" {
		return
	}
	if body == "" {
		if g.body != "" {
			t.Errorf("body = %q, want empty", g.body)
		}
	} else if !strings.Contains(g.body, body) {
		t.Errorf("body = %q, want it to contain %q", g.body, body)
	}
}

func TestMBEmailDrift(t *testing.T) {
	ctx := context.Background()

	t.Run("Email decodes new fields", func(t *testing.T) {
		c, got := mbClient(t, 200, `{"id":"e1","user_id":"u1","email":"a@b.com","send_as_email":"alias@b.com","provider":"gmail","status":"active",
			"mail_host":"google_workspace","auth_method":"delegated","domain_grant_id":"g1","vendor_connection_id":"v1","vendor":"inboxkit","avatar_url":"https://x/a.png",
			"last_synced_at":null,"last_id":null,"campaign_limit":50,"test_mode":"diagnostic","test_send_enabled":true,"test_receive_enabled":false,
			"shared_daily_limit":120,"rolling_recipient_limit":null,"min_wait_time":600,"reply_to":"","save_to_sent":true,"relay_folder_moves":true,
			"track_direct_mail":true,"tracking_domain":"","tracking_domain_verified":false,"tracking_domain_verified_at":null,
			"auth_state":"passing","warmup":null,"warmup_paused_at":null,"warmup_placement":"archive","warmup_folder":"Warm","warmup_retention_days":14,
			"timezone":"","tags":[],"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`)
		e, _, err := c.Emails.Get(ctx, "e1")
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "GET", "/v1/emails/e1", "", "")
		if e.SendAsEmail != "alias@b.com" || e.MailHost != "google_workspace" || e.AuthMethod != MailAuthDelegated ||
			e.DomainGrantID == nil || *e.DomainGrantID != "g1" || e.Vendor != "inboxkit" || e.AvatarURL == "" {
			t.Errorf("identity fields not decoded: %+v", e)
		}
		if e.TestMode == nil || *e.TestMode != TestModeDiagnostic || !e.TestSendEnabled || e.TestReceiveEnabled {
			t.Errorf("test participation not decoded: %+v", e)
		}
		if e.SharedDailyLimit == nil || *e.SharedDailyLimit != 120 || e.RollingRecipientLimit != nil {
			t.Errorf("nullable limits wrong: %v %v", e.SharedDailyLimit, e.RollingRecipientLimit)
		}
		if e.WarmupPlacement != WarmupPlacementArchive || e.WarmupFolder != "Warm" || e.WarmupRetentionDays != 14 || !e.RelayFolderMoves || !e.TrackDirectMail {
			t.Errorf("warmup filing / relay fields wrong: %+v", e)
		}
	})

	t.Run("Update carries new keys", func(t *testing.T) {
		c, got := mbClient(t, 200, `{}`)
		_, _, err := c.Emails.Update(ctx, "e1", &EmailUpdateParams{
			SendAsEmail: String(""), TestMode: String(TestModeOff), WarmupPlacement: String(WarmupPlacementFolder),
			WarmupFolder: String("W"), WarmupRetentionDays: Int(0), RelayFolderMoves: Bool(false), SharedDailyLimit: Int(10),
		})
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "PATCH", "/v1/emails/e1", "", `"send_as_email":""`)
		for _, frag := range []string{`"test_mode":"off"`, `"warmup_placement":"folder"`, `"warmup_folder":"W"`, `"warmup_retention_days":0`, `"relay_folder_moves":false`, `"shared_daily_limit":10`} {
			if !strings.Contains(got.body, frag) {
				t.Errorf("body %q missing %s", got.body, frag)
			}
		}
	})

	t.Run("Sync folders", func(t *testing.T) {
		c, got := mbClient(t, 200, `{"state":null,"policy":{"backfill_days":90,"backfill_messages":5000,"daily_messages":1000,"org_daily_messages":5000,"skip_folders":["Junk"]},
			"skip_folders":["Junk"],"folders":[{"name":"INBOX","folder":"inbox"},{"name":"Junk","folder":"spam"}]}`)
		s, _, err := c.Emails.SyncStatus(ctx, "e1")
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "GET", "/v1/emails/e1/sync", "", "")
		if s.State != nil || len(s.Folders) != 2 || s.Folders[1].Folder != "spam" || s.SkipFolders[0] != "Junk" || s.Policy.SkipFolders[0] != "Junk" {
			t.Errorf("sync not decoded: %+v", s)
		}

		c, got = mbClient(t, 200, `{"skip_folders":["Junk","Spam"]}`)
		skip, _, err := c.Emails.UpdateSyncFolders(ctx, "e1", []string{"Junk", "Spam"})
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "PUT", "/v1/emails/e1/sync", "", `{"skip_folders":["Junk","Spam"]}`)
		if len(skip) != 2 {
			t.Errorf("skip = %v", skip)
		}
		// A nil list clears; it must travel as [] not null.
		c, got = mbClient(t, 200, `{"skip_folders":[]}`)
		if _, _, err := c.Emails.UpdateSyncFolders(ctx, "e1", nil); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got.body, `"skip_folders":[]`) {
			t.Errorf("body = %q", got.body)
		}
	})

	t.Run("Direct tracking", func(t *testing.T) {
		c, got := mbClient(t, 200, `{"track_direct_mail":true}`)
		on, _, err := c.Emails.SetDirectTracking(ctx, "e1", true)
		if err != nil || !on {
			t.Fatalf("on=%v err=%v", on, err)
		}
		got.want(t, "PATCH", "/v1/emails/e1/direct-tracking", "", `{"enabled":true}`)
	})

	t.Run("Identity", func(t *testing.T) {
		c, got := mbClient(t, 200, `{"supported":true,"provider":"google","mailbox_email":"a@b.com","send_as_email":"","identities":[{"email":"a@b.com","name":"A","is_primary":true,"is_default":true,"verified":true}],
			"synced_at":"2026-02-01T00:00:00Z","signature_source":"provider","signature_imported_at":null}`)
		id, _, err := c.Emails.Identity(ctx, "e1")
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "GET", "/v1/emails/e1/identity", "", "")
		if !id.Supported || len(id.Identities) != 1 || !id.Identities[0].IsPrimary || id.SyncedAt == nil || id.SignatureSource != SignatureSourceProvider {
			t.Errorf("identity wrong: %+v", id)
		}
		if _, _, err := c.Emails.RefreshIdentity(ctx, "e1", true); err != nil {
			t.Fatal(err)
		}
		got.want(t, "POST", "/v1/emails/e1/identity/refresh", "", `{"import_signature":true}`)
	})

	t.Run("Certificates", func(t *testing.T) {
		c, got := mbClient(t, 200, `{"smtp":{"host":"s","port":465,"tls_unverified":true,"tls_pin":"ab","observed":{"fingerprint":"ab","self_signed":true}},"imap":{"host":"i","port":993,"tls_unverified":false}}`)
		cert, _, err := c.Emails.Certificates(ctx, "e1")
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "GET", "/v1/emails/e1/certificates", "", "")
		if !cert.SMTP.TLSUnverified || cert.SMTP.Observed == nil || !cert.SMTP.Observed.SelfSigned || cert.IMAP.Observed != nil {
			t.Errorf("certs wrong: %+v", cert)
		}
		if _, _, err := c.Emails.CheckCertificates(ctx, "e1"); err != nil {
			t.Fatal(err)
		}
		got.want(t, "POST", "/v1/emails/onboarding/smtp-imap/e1/certificates/check", "", "")
		if _, _, err := c.Emails.ConfirmCertificates(ctx, "e1", "aa", "bb"); err != nil {
			t.Fatal(err)
		}
		got.want(t, "POST", "/v1/emails/onboarding/smtp-imap/e1/certificates/confirm", "", `{"smtp_pin":"aa","imap_pin":"bb"}`)
	})

	t.Run("App password, OAuth hint and TLS pin credentials", func(t *testing.T) {
		c, got := mbClient(t, 200, `{"id":"e1"}`)
		if _, _, err := c.Emails.SwitchToAppPassword(ctx, "e1", "abcd"); err != nil {
			t.Fatal(err)
		}
		got.want(t, "POST", "/v1/emails/onboarding/app-password/e1", "", `{"app_password":"abcd"}`)

		c, got = mbClient(t, 200, `{"url":"u","state":"s","admin_consent_url":"https://admin"}`)
		st, _, err := c.Emails.StartOAuthWith(ctx, &OAuthStartParams{Provider: ProviderOutlook, LoginHint: "a@b.com", Return: "web"})
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "POST", "/v1/emails/onboarding/oauth/start", "", `{"provider":"outlook","login_hint":"a@b.com","return":"web"}`)
		if st.AdminConsentURL != "https://admin" {
			t.Errorf("admin consent url = %q", st.AdminConsentURL)
		}

		c, got = mbClient(t, 201, `{"id":"e1"}`)
		_, _, err = c.Emails.ConnectSMTPIMAP(ctx, &SMTPIMAPParams{Email: "a@b.com",
			SMTP: &MailboxCredentials{Host: "127.0.0.1", Port: 1025, Security: MailSecurityNone},
			IMAP: &MailboxCredentials{Host: "i", Port: 993, TLSUnverified: true, TLSPin: "ab"}})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got.body, `"security":"none"`) || !strings.Contains(got.body, `"tls_unverified":true,"tls_pin":"ab"`) {
			t.Errorf("body = %q", got.body)
		}
	})
}

func TestMBImportsGrantsVendorsDomains(t *testing.T) {
	ctx := context.Background()

	t.Run("PreviewImport multipart text", func(t *testing.T) {
		c, got := mbClient(t, 200, `{"format":"csv","has_header":true,"vendor":null,"columns":[{"index":0,"header":"email","samples":["a@b.com"],"secret":false,"field":"email","source":"header","confidence":1}],
			"mapping":{"0":"email"},"saved_mapping":false,"summary":{"total":1,"ready":1,"needs_signin":0,"invalid":0,"existing":0,"duplicate":0},
			"rows":[{"line":2,"email":"a@b.com","name":"A","mail_host":"gmail","auth_method":"app_password","status":"ready"}],
			"domains":[{"domain":"b.com","mail_host":"gmail","label":"Gmail","source":"mx","rows":1,"password_auth":"app_password","tracking":{"host":"t.b.com","status":"suggested","cname_target":"track.x"}}],
			"issues":[],"max_rows":5000}`)
		p, _, err := c.Emails.PreviewImport(ctx, &MailboxImportInput{
			Text:    "email,password\na@b.com,x",
			Mapping: MailboxImportMapping{"0": ImportFieldEmail},
			Options: &MailboxImportOptions{OnExisting: ImportOnExistingSkip, HasHeader: Bool(true)},
		})
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "POST", "/v1/emails/imports/preview", "", "-")
		if !strings.HasPrefix(got.contentType, "multipart/form-data") {
			t.Fatalf("content type = %q", got.contentType)
		}
		if got.form["text"] != "email,password\na@b.com,x" || got.form["mapping"] != `{"0":"email"}` || !strings.Contains(got.form["options"], `"on_existing":"skip"`) {
			t.Errorf("form = %v", got.form)
		}
		if p.Vendor != nil || p.Summary.Ready != 1 || p.Rows[0].Status != ImportPreviewReady || p.Domains[0].Tracking == nil || p.MaxRows != 5000 {
			t.Errorf("preview wrong: %+v", p)
		}
	})

	t.Run("CreateImport file", func(t *testing.T) {
		c, got := mbClient(t, 201, `{"id":"i1","status":"running","source":"file","filename":"m.csv","vendor":"","on_existing":"update","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z",
			"total":2,"counts":{"queued":2},"causes":[],"authorizing":{"google":1}}`)
		imp, _, err := c.Emails.CreateImport(ctx, &MailboxImportInput{File: &FileUpload{Filename: "m.csv", Content: strings.NewReader("a,b")}})
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "POST", "/v1/emails/imports", "", "-")
		if got.files["file"] != "m.csv|a,b" {
			t.Errorf("files = %v", got.files)
		}
		if imp.Status != ImportRunning || imp.Counts.Queued != 2 || imp.Authorizing["google"] != 1 {
			t.Errorf("import wrong: %+v", imp)
		}
		if _, _, err := c.Emails.CreateImport(ctx, &MailboxImportInput{}); err == nil {
			t.Error("an empty import input must be refused client-side")
		}
	})

	t.Run("Import reads and actions", func(t *testing.T) {
		c, got := mbClient(t, 200, `{"data":[{"id":"i1","status":"completed"}],"pagination":{"next_cursor":"c2","has_more":true}}`)
		pg, err := c.Emails.Imports(ctx, &ListOptions{Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "GET", "/v1/emails/imports", "limit=10", "")
		if len(pg.Data) != 1 || !pg.HasMore() {
			t.Errorf("page = %+v", pg)
		}

		c, got = mbClient(t, 200, `{"data":[{"line":3,"email":"a@b.com","status":"failed","code":"auth","cause":"bad_password","message":"no","retryable":true,"fields":{"smtp_host":"h"},"updated_at":"2026-01-01T00:00:00Z"}],"pagination":{"has_more":false}}`)
		rows, err := c.Emails.ImportRows(ctx, "i1", &MailboxImportRowsParams{ListOptions: ListOptions{Limit: 5}, Status: []string{ImportRowFailed, ImportRowNeedsSignin}, Cause: "bad_password"})
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "GET", "/v1/emails/imports/i1/rows", "cause=bad_password&limit=5&status=failed%2Cneeds_signin", "")
		if rows.Data[0].Line != 3 || !rows.Data[0].Retryable || rows.Data[0].EmailAccountID != nil {
			t.Errorf("row = %+v", rows.Data[0])
		}

		c, got = mbClient(t, 200, `{"line":3,"status":"queued"}`)
		if _, _, err := c.Emails.FixImportRow(ctx, "i1", 3, &MailboxImportRowFix{Password: String("p"), SMTP: &MailboxImportLegFix{Port: Int(587)}}); err != nil {
			t.Fatal(err)
		}
		got.want(t, "PATCH", "/v1/emails/imports/i1/rows/3", "", `"password":"p"`)

		c, got = mbClient(t, 200, `{"id":"i1"}`)
		if _, _, err := c.Emails.RetryImport(ctx, "i1", &MailboxImportRetry{Lines: []int{3, 4}}); err != nil {
			t.Fatal(err)
		}
		got.want(t, "POST", "/v1/emails/imports/i1/retry", "", `{"lines":[3,4]}`)
		if _, _, err := c.Emails.RetryImport(ctx, "i1", nil); err != nil {
			t.Fatal(err)
		}
		got.want(t, "POST", "/v1/emails/imports/i1/retry", "", "")
		if _, _, err := c.Emails.CancelImport(ctx, "i1"); err != nil {
			t.Fatal(err)
		}
		got.want(t, "POST", "/v1/emails/imports/i1/cancel", "", "")

		c, got = mbClient(t, 204, ``)
		if _, err := c.Emails.DismissImport(ctx, "i1"); err != nil {
			t.Fatal(err)
		}
		got.want(t, "POST", "/v1/emails/imports/i1/dismiss", "", "")

		c, got = mbClient(t, 200, "email,error\na@b.com,bad\n")
		csv, _, err := c.Emails.ImportFailuresCSV(ctx, "i1")
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "GET", "/v1/emails/imports/i1/failed.csv", "", "")
		if !strings.HasPrefix(string(csv), "email,error") {
			t.Errorf("csv = %q", csv)
		}
	})

	t.Run("Grants", func(t *testing.T) {
		c, got := mbClient(t, 200, `{"google_enabled":true,"google_client_id":"cid","google_scopes":["s"],"microsoft_enabled":false,"google_missing":[],"microsoft_missing":["BOX_OUTLOOK_CLIENT_ID"]}`)
		cfg, _, err := c.Emails.GrantConfig(ctx)
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "GET", "/v1/emails/grants/config", "", "")
		if !cfg.GoogleEnabled || cfg.MicrosoftMissing[0] != "BOX_OUTLOOK_CLIENT_ID" {
			t.Errorf("cfg = %+v", cfg)
		}

		c, got = mbClient(t, 200, `{"data":[{"id":"g1","provider":"google","tenant":"b.com","admin_email":"a@b.com","domains":["b.com"],"status":"active","verified_at":"2026-01-01T00:00:00Z","created_at":"2026-01-01T00:00:00Z","mailboxes":3}]}`)
		gs, _, err := c.Emails.Grants(ctx)
		if err != nil || len(gs) != 1 || gs[0].Mailboxes != 3 || gs[0].Provider != GrantProviderGoogle {
			t.Fatalf("grants = %+v err=%v", gs, err)
		}
		got.want(t, "GET", "/v1/emails/grants", "", "")

		c, got = mbClient(t, 200, `{"method":"dns","txt_name":"_warmbly.b.com","txt_value":"warmbly-verify=x"}`)
		st, _, err := c.Emails.StartGoogleGrant(ctx, "b.com", "a@b.com")
		if err != nil || st.Method != "dns" || st.TXTName == "" {
			t.Fatalf("start = %+v err=%v", st, err)
		}
		got.want(t, "POST", "/v1/emails/grants/google/start", "", `{"domain":"b.com","admin_email":"a@b.com"}`)

		c, got = mbClient(t, 201, `{"id":"g1","provider":"google","tenant":"b.com","domains":["b.com"],"status":"active","created_at":"2026-01-01T00:00:00Z","mailboxes":0}`)
		if _, _, err := c.Emails.FinishGoogleGrant(ctx, &GoogleGrantFinish{Domain: "b.com", AdminEmail: "a@b.com"}); err != nil {
			t.Fatal(err)
		}
		got.want(t, "POST", "/v1/emails/grants/google/finish", "", `{"domain":"b.com","admin_email":"a@b.com"}`)

		c, got = mbClient(t, 200, `{"url":"https://login","state":"s"}`)
		if ms, _, err := c.Emails.StartMicrosoftGrant(ctx); err != nil || ms.State != "s" {
			t.Fatalf("ms = %+v err=%v", ms, err)
		}
		got.want(t, "POST", "/v1/emails/grants/microsoft/start", "", "")

		c, got = mbClient(t, 201, `{"id":"g2","provider":"microsoft","tenant":"t","domains":[],"status":"active","created_at":"2026-01-01T00:00:00Z","mailboxes":0}`)
		if _, _, err := c.Emails.FinishMicrosoftGrant(ctx, "s", "code"); err != nil {
			t.Fatal(err)
		}
		got.want(t, "POST", "/v1/emails/grants/microsoft/finish", "", `{"state":"s","code":"code"}`)

		c, got = mbClient(t, 200, `{"id":"g1","status":"failing","last_error":"nope"}`)
		g, _, err := c.Emails.CheckGrant(ctx, "g1")
		if err != nil || g.LastError != "nope" {
			t.Fatalf("check = %+v err=%v", g, err)
		}
		got.want(t, "POST", "/v1/emails/grants/g1/check", "", "")

		c, got = mbClient(t, 204, ``)
		if _, err := c.Emails.DeleteGrant(ctx, "g1"); err != nil {
			t.Fatal(err)
		}
		got.want(t, "DELETE", "/v1/emails/grants/g1", "", "")

		c, got = mbClient(t, 200, `{"data":[{"id":"u1","email":"u@b.com","name":"U","enabled":true,"connected":true,"email_account_id":"e9","upgrade":true}]}`)
		us, _, err := c.Emails.GrantUsers(ctx, "g1")
		if err != nil || !us[0].Upgrade || us[0].EmailAccountID == nil {
			t.Fatalf("users = %+v err=%v", us, err)
		}
		got.want(t, "GET", "/v1/emails/grants/g1/users", "", "")

		c, got = mbClient(t, 201, `{"id":"i1","status":"running"}`)
		if _, _, err := c.Emails.ConnectGrantUsers(ctx, "g1", &GrantConnectParams{All: true}); err != nil {
			t.Fatal(err)
		}
		got.want(t, "POST", "/v1/emails/grants/g1/connect", "", `"all":true`)

		c, got = mbClient(t, 200, `{"data":[{"domain":"gmail.com","kind":"personal","mailboxes":[{"id":"e1","email":"a@gmail.com","name":"A","status":"active"}]}],"total":1}`)
		mig, _, err := c.Emails.SigninMigration(ctx)
		if err != nil || mig.Total != 1 || mig.Data[0].GrantID != nil {
			t.Fatalf("migration = %+v err=%v", mig, err)
		}
		got.want(t, "GET", "/v1/emails/grants/migration", "", "")
	})

	t.Run("Vendors", func(t *testing.T) {
		c, got := mbClient(t, 200, `{"data":[{"id":"inboxkit","label":"InboxKit","website":"w","key_help_url":"k","fields":[{"key":"api_key","label":"API key","secret":true,"required":true}]}]}`)
		cat, _, err := c.Emails.VendorCatalog(ctx)
		if err != nil || !cat[0].Fields[0].Secret {
			t.Fatalf("catalog = %+v err=%v", cat, err)
		}
		got.want(t, "GET", "/v1/emails/vendors/catalog", "", "")

		c, got = mbClient(t, 201, `{"id":"v1","vendor":"inboxkit","label":"Main","status":"active","created_at":"2026-01-01T00:00:00Z","mailboxes":0}`)
		v, _, err := c.Emails.CreateVendor(ctx, &VendorConnectionParams{Vendor: "inboxkit", Label: "Main", Fields: map[string]string{"api_key": "k"}})
		if err != nil || v.ID != "v1" {
			t.Fatalf("vendor = %+v err=%v", v, err)
		}
		got.want(t, "POST", "/v1/emails/vendors", "", `{"vendor":"inboxkit","label":"Main","fields":{"api_key":"k"}}`)

		c, got = mbClient(t, 200, `{"id":"v1"}`)
		if _, _, err := c.Emails.UpdateVendor(ctx, "v1", &VendorConnectionParams{Vendor: "inboxkit", Label: "New"}); err != nil {
			t.Fatal(err)
		}
		got.want(t, "PATCH", "/v1/emails/vendors/v1", "", `"label":"New"`)

		c, got = mbClient(t, 204, ``)
		if _, err := c.Emails.DeleteVendor(ctx, "v1"); err != nil {
			t.Fatal(err)
		}
		got.want(t, "DELETE", "/v1/emails/vendors/v1", "", "")

		c, got = mbClient(t, 200, `{"data":[{"id":"m1","email":"a@b.com","name":"A","domain":"b.com","provider":"google","status":"active","connected":false}]}`)
		bx, _, err := c.Emails.VendorMailboxes(ctx, "v1")
		if err != nil || bx[0].Connected || bx[0].EmailAccountID != nil {
			t.Fatalf("mailboxes = %+v err=%v", bx, err)
		}
		got.want(t, "GET", "/v1/emails/vendors/v1/mailboxes", "", "")

		c, got = mbClient(t, 201, `{"id":"i1","status":"running"}`)
		if _, _, err := c.Emails.ImportVendorMailboxes(ctx, "v1", &VendorImportParams{MailboxIDs: []string{"m1"}}); err != nil {
			t.Fatal(err)
		}
		got.want(t, "POST", "/v1/emails/vendors/v1/import", "", `"mailbox_ids":["m1"]`)
	})

	t.Run("Domains", func(t *testing.T) {
		c, got := mbClient(t, 200, `{"data":[{"domain":"b.com","mailboxes":2,"mail_hosts":["gmail"],"auth_state":"passing","auth_spf":true,"auth_dkim":false,"auth_dmarc":true,
			"tracking_domains":[{"host":"t.b.com","verified":true,"mailboxes":2}],
			"redirect":{"id":"r1","domain":"b.com","target_url":"https://b.io","include_www":true,"verified":false,"created_at":"2026-01-01T00:00:00Z",
				"records":[{"purpose":"verify","type":"TXT","name":"_x.b.com","value":"v","ok":false}],"served_by":"cloud","reach":{"status":"not_reaching","hint":"not_routed"},"target_active":false,"target_inactive_reason":"off-workspace"},
			"vendors":["inboxkit"],"vendor_connection_ids":["v1"],"vendor_domain":{"vendor":"inboxkit","connection_id":"v1","can_forward":true,"can_unforward":false,"forwarding_reviewed":true,"can_dns":true,"dns_types":["CNAME"]}}]}`)
		ds, _, err := c.Emails.Domains(ctx)
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "GET", "/v1/emails/domains", "", "")
		d := ds[0]
		if d.Redirect == nil || d.Redirect.ServedBy != RedirectServedByCloud || d.Redirect.Reach.Status != RedirectReachNotReaching ||
			d.Redirect.TargetActive == nil || *d.Redirect.TargetActive || d.Redirect.Records[0].OK || d.VendorDomain == nil || !d.VendorDomain.CanForward || d.TrackingDomains[0].Mailboxes != 2 {
			t.Errorf("domain wrong: %+v", d)
		}

		c, got = mbClient(t, 200, `{"data":[{"domain":"b.com","tracking":{"host":"t.b.com","via":"vendor","verified":true,"mailboxes":2},"redirect":{"target_url":"https://b.io","via":"dns","verified":false,"error":"x","code":"c"}}]}`)
		rows, _, err := c.Emails.BulkDomainSetup(ctx, &DomainBulkParams{Domains: []string{"b.com"}, TrackingLabel: "t", ServedBy: RedirectServedByInstance})
		if err != nil || rows[0].Tracking.Via != "vendor" || rows[0].Redirect.Code != "c" {
			t.Fatalf("bulk = %+v err=%v", rows, err)
		}
		got.want(t, "POST", "/v1/emails/domains/bulk", "", `{"domains":["b.com"],"tracking_label":"t","served_by":"instance"}`)

		c, got = mbClient(t, 200, `{"host":"t.b.com","status":"suggested","cname_target":"track.x"}`)
		if sg, _, err := c.Emails.DomainTrackingSuggestion(ctx, "b.com"); err != nil || sg.CNAMETarget != "track.x" {
			t.Fatalf("suggestion = %+v err=%v", sg, err)
		}
		got.want(t, "GET", "/v1/emails/domains/b.com/tracking-suggestion", "", "")

		c, got = mbClient(t, 200, `{"tracking":{"tracking_domain":"t.b.com","tracking_domain_verified":true,"tracking_domain_verified_at":null,"cname_target":"track.x","status":"verified","message":"ok"},"mailboxes":2}`)
		tr, _, err := c.Emails.SetDomainTracking(ctx, "b.com", "t.b.com")
		if err != nil || tr.Mailboxes != 2 || tr.Tracking.TrackingDomain != "t.b.com" {
			t.Fatalf("tracking = %+v err=%v", tr, err)
		}
		got.want(t, "PUT", "/v1/emails/domains/b.com/tracking", "", `{"host":"t.b.com"}`)
		if _, _, err := c.Emails.SetDomainVendorTracking(ctx, "b.com", "t.b.com"); err != nil {
			t.Fatal(err)
		}
		got.want(t, "POST", "/v1/emails/domains/b.com/vendor-tracking", "", `{"host":"t.b.com"}`)

		c, got = mbClient(t, 200, `{"id":"r1","domain":"b.com","target_url":"https://b.io","include_www":false,"verified":true,"created_at":"2026-01-01T00:00:00Z","records":[],"served_by":"instance"}`)
		r, _, err := c.Emails.SetDomainRedirect(ctx, "b.com", &DomainRedirectParams{TargetURL: "https://b.io", IncludeWWW: Bool(false)})
		if err != nil || r.IncludeWWW || !r.Verified || r.TargetActive != nil {
			t.Fatalf("redirect = %+v err=%v", r, err)
		}
		got.want(t, "PUT", "/v1/emails/domains/b.com/redirect", "", `{"target_url":"https://b.io","include_www":false}`)
		if _, _, err := c.Emails.VerifyDomainRedirect(ctx, "b.com"); err != nil {
			t.Fatal(err)
		}
		got.want(t, "POST", "/v1/emails/domains/b.com/redirect/verify", "", "")

		c, got = mbClient(t, 204, ``)
		if _, err := c.Emails.DeleteDomainRedirect(ctx, "b.com"); err != nil {
			t.Fatal(err)
		}
		got.want(t, "DELETE", "/v1/emails/domains/b.com/redirect", "", "")

		c, got = mbClient(t, 200, `{"vendor":"inboxkit","connection_id":"v1","forwarding":"https://b.io","can_forward":true,"can_unforward":true,"forwarding_reviewed":false,"can_dns":false,"dns_types":[]}`)
		l, _, err := c.Emails.SetDomainVendorForwarding(ctx, "b.com", "https://b.io")
		if err != nil || l.Forwarding != "https://b.io" {
			t.Fatalf("link = %+v err=%v", l, err)
		}
		got.want(t, "PUT", "/v1/emails/domains/b.com/vendor-forwarding", "", `{"url":"https://b.io"}`)
	})
}

func TestMBPlacement(t *testing.T) {
	ctx := context.Background()
	svc := func(c *Client) *PlacementService { return (*PlacementService)(&c.common) }

	testJSON := `{"id":"t1","sender_account_id":"e1","sender_email":"a@b.com","created_by":null,"campaign_id":null,"sequence_id":null,"contact_id":null,"monitor_id":null,"batch_id":null,
		"subject":"Hi","open_tracking":true,"link_tracking":false,"compare_group_id":null,"origin":"manual","panel":"instance","status":"running","pace":"spaced",
		"credits_charged":0,"credits_refunded":0,"credits_settled_at":null,"created_at":"2026-01-01T00:00:00Z","finished_at":null,
		"summary":{"total":2,"pending":1,"inbox":1,"promotions":0,"other":0,"spam":0,"missing":0,"failed":0,"cancelled":0,"unknown":0,"archive":0,"custom":0,"observed_receipts":1,"classified_receipts":1,"unresolved":1,
			"primary_metric":{"version":"observed-v2","population":"seed_panel","denominator_kind":"classified_observed_receipts","unit":"fraction","source":"mailbox_observation","classification_policy":"p","numerator":1,"denominator":1,"unresolved":1,"window_basis":"w","missingness_basis":"m","value":1,"wilson_95_independence_interval":{"lower":0.2,"upper":1}},
			"non_spam_metric":null,"delivered":1,"inbox_rate":1,"tabs_rate":0,"spam_rate":0,"missing_rate":0},
		"families":[{"family":"gmail","label":"Gmail","counts":{"total":1,"inbox":1,"inbox_rate":1,"tabs_rate":null,"spam_rate":null,"missing_rate":null,"primary_metric":null,"non_spam_metric":null}}]}`

	t.Run("Start and decode", func(t *testing.T) {
		c, got := mbClient(t, 201, `{"data":[`+testJSON+`]}`)
		tests, _, err := svc(c).Start(ctx, &PlacementTestParams{SenderAccountID: "e1", Subject: "Hi", Tracking: PlacementTrackingCompare, Panel: PlacementPanelInstance})
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "POST", "/v1/placement/tests", "", `{"sender_account_id":"e1","subject":"Hi","tracking":"compare","panel":"instance"}`)
		if len(tests) != 1 || tests[0].Status != PlacementStatusRunning || tests[0].Summary.InboxRate == nil || tests[0].Summary.PrimaryMetric == nil ||
			tests[0].Summary.PrimaryMetric.Interval == nil || tests[0].Summary.NonSpamMetric != nil || tests[0].Families[0].Counts.TabsRate != nil {
			t.Errorf("test wrong: %+v", tests)
		}
	})

	t.Run("List", func(t *testing.T) {
		c, got := mbClient(t, 200, `{"data":[`+testJSON+`],"pagination":{"total":40,"next_cursor":"b2Zmc2V0OjI1","has_more":true}}`)
		pg, err := svc(c).List(ctx, &PlacementListParams{ListOptions: ListOptions{Limit: 25, Cursor: "x"}, CampaignID: "c1"})
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "GET", "/v1/placement/tests", "campaign_id=c1&cursor=x&limit=25", "")
		if pg.Pagination.Total == nil || *pg.Pagination.Total != 40 || !pg.HasMore() {
			t.Errorf("pagination = %+v", pg.Pagination)
		}
	})

	t.Run("Get detail", func(t *testing.T) {
		detail := strings.TrimSuffix(testJSON, "}") + `,"results":[{"seed":"s***@g.com","family":"gmail","family_label":"Gmail","folder":"promotions","scheduled_at":null,"sent_at":"2026-01-01T00:00:00Z","detected_at":null,
			"first_folder":"inbox","late_observation":true,"evidence":{"version":"received-v1","source":"x","trust":"t","observed_at":"2026-01-01T00:00:00Z","spf":"pass","dkim":"pass","dmarc":"pass","alignment":"pass","tls":"pass",
			"authentication_results_present":true,"one_click_headers_present":false,"one_click_signing_claim":false,"one_click_compliance":"unknown","dkim_verification":{"dkim":"pass","alignment":"pass","signing_domain":"b.com","verifier":"v","observed_at":"2026-01-01T00:00:00Z"}}}],
			"content":{"score":80,"issues":[{"severity":"warn","code":"c","message":"m","field":"body","spans":[{"field":"body","text":"free","line":1}],"suggestion":"s"}]}}`
		c, got := mbClient(t, 200, `{"data":`+detail+`}`)
		d, _, err := svc(c).Get(ctx, "t1")
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "GET", "/v1/placement/tests/t1", "", "")
		if d.ID != "t1" || len(d.Results) != 1 || d.Results[0].Folder != PlacementFolderPromotions || d.Results[0].Evidence == nil ||
			d.Results[0].Evidence.DKIMVerification == nil || d.Results[0].FirstFolder == nil || d.Content.Score != 80 || d.Content.Issues[0].Spans[0].Text != "free" || d.Compare != nil {
			t.Errorf("detail wrong: %+v", d)
		}
	})

	t.Run("Cancel and overview", func(t *testing.T) {
		c, got := mbClient(t, 200, `{"data":`+testJSON+`}`)
		tt, _, err := svc(c).Cancel(ctx, "t1")
		if err != nil || tt.ID != "t1" {
			t.Fatalf("cancel = %+v err=%v", tt, err)
		}
		got.want(t, "POST", "/v1/placement/tests/t1/cancel", "", "")

		c, got = mbClient(t, 200, `{"data":{"panels":[{"panel":"workspace","available":false,"reason":"no seeds","seeds":0,"families":[],"metered":false}],
			"usage":{"used":1,"limit":null,"credits_per_test":2,"credit_balance":null,"period_start":"2026-01-01T00:00:00Z","period_end":"2026-02-01T00:00:00Z"},
			"workspace_seeds":0,"seeds_per_test":10,"spacing_seconds":60}}`)
		ov, _, err := svc(c).Overview(ctx)
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "GET", "/v1/placement/overview", "", "")
		if ov.Usage.Limit != nil || ov.Usage.CreditBalance != nil || ov.Panels[0].Available || ov.SpacingSeconds != 60 {
			t.Errorf("overview wrong: %+v", ov)
		}
	})

	t.Run("Batches", func(t *testing.T) {
		batch := `{"id":"b1","created_by":null,"campaign_id":"c1","sequence_id":null,"contact_id":null,"subject":"S","tracking":"campaign","panel":"workspace","pace":"quick","families":[],"seed_ids":[],
			"on_unavailable":"defer","selection":{"sender_scope":{"type":"workspace","providers":["gmail"],"untested_days":7},"sample":{"mode":"percent","percent":10,"stratify":"domain"},"matched":100},
			"sender_count":10,"max_credits":0,"credits_spent":0,"status":"completed_with_warnings","retry_until":"2026-01-02T00:00:00Z","created_at":"2026-01-01T00:00:00Z","started_at":null,"finished_at":null,
			"progress":{"total":10,"queued":0,"deferred":1,"running":0,"completed":8,"skipped":1,"failed":0,"cancelled":0},"summary":{"total":0}}`
		c, got := mbClient(t, 201, `{"data":`+batch+`}`)
		b, _, err := svc(c).StartBatch(ctx, &PlacementBatchParams{
			SenderScope: &PlacementSenderScope{Type: PlacementScopeWorkspace, Providers: []string{"gmail"}, UntestedDays: 7},
			Sample:      PlacementSample{Mode: PlacementSamplePercent, Percent: 10, Stratify: "domain"},
			CampaignID:  "c1", OnUnavailable: PlacementUnavailableDefer,
		})
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "POST", "/v1/placement/batches", "", `"sender_scope":{"type":"workspace","providers":["gmail"],"untested_days":7}`)
		if !strings.Contains(got.body, `"sample":{"mode":"percent","percent":10,"stratify":"domain"}`) || strings.Contains(got.body, "sender_account_ids") {
			t.Errorf("body = %q", got.body)
		}
		if b.Status != PlacementBatchCompletedWithWarnings || b.Selection.Scope == nil || b.Selection.Scope.UntestedDays != 7 || b.Progress.Deferred != 1 || b.Selection.Matched != 100 {
			t.Errorf("batch wrong: %+v", b)
		}

		c, got = mbClient(t, 200, `{"data":{"matched":100,"selected":10,"inactive":0,"domains":3,"providers":[{"key":"gmail","label":"Gmail","senders":10}],"variants":2,"tests":20,"seeds_per_test":10,
			"max_sends":200,"metered":true,"free_tests":5,"paid_tests":15,"credits":30,"usage":{"used":0,"limit":5,"credits_per_test":2,"credit_balance":100,"period_start":"2026-01-01T00:00:00Z","period_end":"2026-02-01T00:00:00Z"},"senders_max":500,"concurrency":3}}`)
		pv, _, err := svc(c).PreviewBatch(ctx, &PlacementBatchParams{SenderAccountIDs: []string{"e1"}})
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "POST", "/v1/placement/batches/preview", "", `{"sender_account_ids":["e1"]`)
		if pv.Variants != 2 || pv.Credits != 30 || pv.Providers[0].Key != "gmail" || pv.Usage.CreditBalance == nil {
			t.Errorf("preview wrong: %+v", pv)
		}

		c, got = mbClient(t, 200, `{"data":[`+batch+`],"pagination":{"total":1,"has_more":false,"next_cursor":null}}`)
		bp, err := svc(c).Batches(ctx, &ListOptions{Limit: 5})
		if err != nil || len(bp.Data) != 1 {
			t.Fatalf("batches err=%v", err)
		}
		got.want(t, "GET", "/v1/placement/batches", "limit=5", "")

		detail := strings.TrimSuffix(batch, "}") + `,"untracked":null,"domains":[{"key":"b.com","label":"b.com","senders":3,"tested":3,"counts":{"total":3}}],"providers":[],"recipients":[{"family":"gmail","label":"Gmail","counts":{"total":3}}],
			"matrix":[{"domain":"b.com","recipients":[{"family":"gmail","label":"Gmail","counts":{"total":3}}]}],"content":{"score":90,"issues":[]}}`
		c, got = mbClient(t, 200, `{"data":`+detail+`}`)
		bd, _, err := svc(c).Batch(ctx, "b1")
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "GET", "/v1/placement/batches/b1", "", "")
		if bd.ID != "b1" || bd.Untracked != nil || bd.Domains[0].Tested != 3 || bd.Matrix[0].Recipients[0].Family != "gmail" || bd.Content.Score != 90 {
			t.Errorf("batch detail wrong: %+v", bd)
		}

		c, got = mbClient(t, 200, `{"data":[{"id":"s1","batch_id":"b1","email_account_id":null,"sender_email":"a@b.com","sender_domain":"b.com","sender_family":"gmail","status":"deferred","reason":"busy",
			"attempts":2,"next_attempt_at":"2026-01-01T00:00:00Z","started_at":null,"finished_at":null,"sender_family_label":"Gmail","summary":{"total":0},"test_ids":["t1"]}],"pagination":{"total":1,"has_more":false,"next_cursor":null}}`)
		sp, err := svc(c).BatchSenders(ctx, "b1", &PlacementBatchSendersParams{Status: PlacementSenderDeferred, Query: "a@", Sort: "best"})
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "GET", "/v1/placement/batches/b1/senders", "q=a%40&sort=best&status=deferred", "")
		if sp.Data[0].EmailAccountID != nil || sp.Data[0].Status != PlacementSenderDeferred || sp.Data[0].TestIDs[0] != "t1" {
			t.Errorf("sender wrong: %+v", sp.Data[0])
		}

		c, got = mbClient(t, 200, `{"data":`+batch+`}`)
		if _, _, err := svc(c).CancelBatch(ctx, "b1"); err != nil {
			t.Fatal(err)
		}
		got.want(t, "POST", "/v1/placement/batches/b1/cancel", "", "")

		c, got = mbClient(t, 200, `{"data":{"mailboxes":10,"tested_7d":4,"tested_30d":6,"never_tested":4}}`)
		cov, _, err := svc(c).Coverage(ctx)
		if err != nil || cov.NeverTested != 4 {
			t.Fatalf("coverage = %+v err=%v", cov, err)
		}
		got.want(t, "GET", "/v1/placement/coverage", "", "")
	})

	t.Run("Seeds and monitor", func(t *testing.T) {
		c, got := mbClient(t, 200, `{"data":[{"email_account_id":"e1","email":"a@b.com","family":"gmail","label":"Gmail","status":"active","seed":false,"blocker":"in a campaign"}]}`)
		ss, _, err := svc(c).Seeds(ctx)
		if err != nil || ss[0].Blocker == "" {
			t.Fatalf("seeds = %+v err=%v", ss, err)
		}
		got.want(t, "GET", "/v1/placement/seeds", "", "")

		c, got = mbClient(t, 200, `{"data":{"email_account_id":"e1","email":"a@b.com","family":"gmail","label":"Gmail","status":"active","seed":true}}`)
		s, _, err := svc(c).SetSeed(ctx, "e1", true)
		if err != nil || !s.Seed {
			t.Fatalf("seed = %+v err=%v", s, err)
		}
		got.want(t, "PUT", "/v1/placement/seeds/e1", "", `{"seed":true}`)

		c, got = mbClient(t, 200, `{"data":null}`)
		m, _, err := svc(c).Monitor(ctx, "c1")
		if err != nil || m != nil {
			t.Fatalf("monitor = %+v err=%v", m, err)
		}
		got.want(t, "GET", "/v1/campaigns/c1/placement-monitor", "", "")

		c, got = mbClient(t, 200, `{"data":{"id":"m1","campaign_id":"c1","created_by":null,"enabled":true,"interval_days":7,"panel":"instance","alert_below":80,"pause_on_alert":true,
			"next_run_at":"2026-01-01T00:00:00Z","last_run_at":null,"last_test_id":null,"last_alert_at":null,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}}`)
		m, _, err = svc(c).SetMonitor(ctx, "c1", &PlacementMonitorParams{Enabled: Bool(true), IntervalDays: Int(7), AlertBelow: Int(80), PauseOnAlert: Bool(true)})
		if err != nil || m.AlertBelow != 80 || m.LastRunAt != nil {
			t.Fatalf("monitor = %+v err=%v", m, err)
		}
		got.want(t, "PUT", "/v1/campaigns/c1/placement-monitor", "", `{"enabled":true,"interval_days":7,"alert_below":80,"pause_on_alert":true}`)

		c, got = mbClient(t, 204, ``)
		if _, err := svc(c).DeleteMonitor(ctx, "c1"); err != nil {
			t.Fatal(err)
		}
		got.want(t, "DELETE", "/v1/campaigns/c1/placement-monitor", "", "")
	})
}

func TestMBAnalyticsUniboxOutreach(t *testing.T) {
	ctx := context.Background()
	day := func(s string) time.Time {
		d, _ := time.Parse("2006-01-02", s)
		return d
	}

	t.Run("Dashboard filter and new fields", func(t *testing.T) {
		c, got := mbClient(t, 200, `{"period":"30d","overall_stats":{},"recent_activity":[{"type":"reply","campaign_id":"c1","campaign_name":"C","contact_email":"x@y.com","contact_id":"k1","timestamp":"2026-01-01T00:00:00Z",
			"origin":{"client":"Gmail","device_type":"mobile"},"sender_id":"e1","sender_email":"a@b.com"}],"top_campaigns":[],"account_health":{},"daily_trend":[],
			"capacity_today":{"capacity":100,"remaining_today":40,"configured_ceiling":150,"mailboxes":3,"held":1},"scope":{"campaigns":[{"id":"c1","name":"C"}],"folders":[],"campaign_count":1}}`)
		d, _, err := c.Analytics.DashboardFor(ctx, &DashboardParams{Period: Period30Days, CampaignIDs: []string{"c1", "c2"}, FolderIDs: []string{"f1"}})
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "GET", "/v1/analytics/dashboard", "campaign_ids=c1%2Cc2&folder_ids=f1&period=30d", "")
		a := d.RecentActivity[0]
		if d.CapacityToday == nil || d.CapacityToday.RemainingToday != 40 || d.Scope == nil || d.Scope.CampaignCount != 1 || a.SenderID == nil || *a.SenderID != "e1" || a.Origin == nil || a.Origin.Client != "Gmail" {
			t.Errorf("dashboard wrong: %+v", d)
		}
	})

	t.Run("Campaign period", func(t *testing.T) {
		c, got := mbClient(t, 200, `{"campaign_id":"c1","name":"C","status":"active","date_range":{"from":"2026-01-01T00:00:00Z","to":"2026-01-31T00:00:00Z"},
			"summary":{"emails_sent":10,"positive_replies":2,"interested_leads":1,"reply_breakdown":{"positive":2,"out_of_office":3},"positive_reply_rate":20},
			"steps":[{"step_id":"s1","name":"S","position":1,"emails_sent":10,"opens":5,"machine_opens":2,"clicks":1,"machine_clicks":1,"replies":2,"positive_replies":1,"bounces":0,"open_rate":50,"positive_reply_rate":10}],
			"engagement":{"countries":[],"clients":[],"devices":[],"surfaces":[{"key":"webmail","opens":3,"clicks":1}]}}`)
		a, _, err := c.Analytics.CampaignBetween(ctx, "c1", day("2026-01-01"), day("2026-01-31"))
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "GET", "/v1/analytics/campaigns/c1", "from=2026-01-01&to=2026-01-31", "")
		if a.Summary.PositiveReplies != 2 || a.Summary.ReplyBreakdown.OutOfOffice != 3 || a.Steps[0].MachineOpens != 2 || a.Steps[0].PositiveReplyRate != 10 || a.Engagement.Surfaces[0].Key != "webmail" {
			t.Errorf("analytics wrong: %+v", a)
		}
		if _, _, err := c.Analytics.CampaignBetween(ctx, "c1", time.Time{}, day("2026-01-31")); err == nil {
			t.Error("CampaignBetween must refuse a half-open range")
		}
		if _, _, err := c.Analytics.Campaign(ctx, "c1"); err != nil {
			t.Fatal(err)
		}
		got.want(t, "GET", "/v1/analytics/campaigns/c1", "", "")
	})

	t.Run("Accounts page and status fields", func(t *testing.T) {
		c, got := mbClient(t, 200, `{"data":[{"id":"e1","email":"a@b.com","provider":"google","status":"active","last_synced_at":null,"health":{"status":"ok","score":90},"daily_usage":{},
			"warmup_status":{"enabled":true,"paused":false,"started_at":"2026-01-01T00:00:00Z","current_volume":1,"target_volume":5,"max_volume":40,"reply_rate":30,"days_active":3,
				"partner_limit":{"reachable":4,"ramp_target":8},"send_failure":{"message":"550","at":"2026-01-02T00:00:00Z","kind":"auth"},"next_send_at":"2026-01-03T00:00:00Z"},
			"warmup_health":{"pool_type":"premium","source":"cloud","state":"healthy","score":95,"spam_score":0,"partner_mailboxes_7d":9,"received_7d":12,"senders_7d":4},
			"in_campaign":false,"warmup_placement":{"window_days":7,"min_sample":20,"scope":"major","delivered":30,"inbox":25,"tabs":3,"spam":2,"inbox_rate":93.33,"band":"good","metric":null,"other_delivered":0,"other_inbox_rate":null},
			"send_recovery_reason":"authentication","errors":[{"id":"x","error_code":"E","severity":"high","title":"t","message":"m","created_at":"2026-01-01T00:00:00Z","operation":"send"}]}],
			"pagination":{"next_cursor":"nc","has_more":true}}`)
		pg, err := c.Analytics.AccountsPage(ctx, &AccountsParams{ListOptions: ListOptions{Limit: 200}, EmailIDs: []string{"e1", "e2"}})
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "GET", "/v1/analytics/accounts", "email_ids=e1%2Ce2&limit=200", "")
		a := pg.Data[0]
		if !pg.HasMore() || a.Placement == nil || a.Placement.Band != WarmupPlacementBandGood || a.Placement.InboxRate == nil || a.SendRecoveryReason != "authentication" ||
			a.WarmupStatus.PartnerLimit == nil || a.WarmupStatus.SendFailure.Kind != "auth" || a.WarmupStatus.NextSendAt == nil || a.WarmupHealth.PoolType != "premium" ||
			a.WarmupHealth.Received7d != 12 || a.Errors[0].Operation != "send" {
			t.Errorf("account wrong: %+v", a)
		}
	})

	t.Run("Warmup placement, direct, inbox tagging", func(t *testing.T) {
		c, got := mbClient(t, 200, `{"email_account_id":null,"date_range":{"from":"2026-01-01T00:00:00Z","to":"2026-01-30T00:00:00Z"},
			"summary":{"sent":50,"delivered":40,"inbox":30,"tabs":5,"spam":5,"unknown":1,"archived":0,"custom":0,"instrumented_receipts":40,"observed_receipts":41,"legacy_uninstrumented_receipts":1,
				"rescue_requested":2,"rescued":2,"rescue_confirmed":1,"rescue_missing":0,"rescue_failed":0,"rescue_unreported":1,"unconfirmed":3,"non_spam_metric":null,"inbox_rate":87.5,"spam_rate":12.5},
			"rate":{"window_days":7,"min_sample":20,"scope":"major","delivered":0,"inbox":0,"tabs":0,"spam":0,"inbox_rate":null,"band":"none","metric":null,"other_delivered":0,"other_inbox_rate":null},
			"daily":[{"date":"2026-01-01","sent":5,"delivered":4,"inbox":4,"tabs":0,"spam":0,"rolling_inbox_rate":null,"groups":[{"group":"google","inbox":4,"tabs":0,"spam":0,"rescued":0,"unknown":0,"archived":0,"custom":0,"rescue_confirmed":0,"rescue_missing":0,"rescue_failed":0}]}],
			"providers":[{"group":"google","sent":5,"delivered":4,"hosts":[{"host":"gmail","sent":5,"delivered":4}]}],
			"mailboxes":[{"email_account_id":"e1","email":"a@b.com","sent":5,"delivered":4,"rate":{"window_days":7,"min_sample":20,"scope":"major","band":"collecting"},"daily_inbox_rate":[null,92.5]}]}`)
		rep, _, err := c.Analytics.WarmupPlacement(ctx, &WarmupPlacementParams{EmailID: "e1", From: day("2026-01-01"), To: day("2026-01-30")})
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "GET", "/v1/analytics/warmup/placement", "email_id=e1&from=2026-01-01&to=2026-01-30", "")
		if rep.EmailAccountID != nil || rep.Summary.InboxRate == nil || rep.Summary.RescueUnreported != 1 || rep.Daily[0].RollingInboxRate != nil || rep.Daily[0].Date != "2026-01-01" ||
			rep.Daily[0].Groups[0].Group != "google" || rep.Daily[0].Sent != 5 || rep.Providers[0].Hosts[0].Host != "gmail" || rep.Providers[0].Delivered != 4 ||
			rep.Mailboxes[0].DailyInboxRate[0] != nil || *rep.Mailboxes[0].DailyInboxRate[1] != 92.5 || rep.Mailboxes[0].Rate.Band != WarmupPlacementBandCollecting {
			t.Errorf("report wrong: %+v", rep)
		}
		if _, _, err := c.Analytics.WarmupPlacement(ctx, nil); err != nil {
			t.Fatal(err)
		}
		got.want(t, "GET", "/v1/analytics/warmup/placement", "", "")

		c, got = mbClient(t, 200, `{"period":"7d","volume":{"sent":10,"received":4,"threads_started":3,"replied":1,"reply_rate":33.3,"bounced":0,"median_reply_minutes":45},
			"tracking":{"mailboxes_opted_in":1,"mailboxes_total":3,"tracked_sent":5,"opened":2,"machine_opened":1,"clicked":0,"open_rate":40,"click_rate":0},
			"daily_trend":[{"date":"2026-01-01T00:00:00Z","sent":2,"received":1}],"mailboxes":[{"email_account_id":"e1","email":"a@b.com","track_direct_mail":true,"sent":2,"received":1}],
			"top_contacts":[{"email":"x@y.com","sent":2,"received":1,"last_at":"2026-01-01T00:00:00Z"}]}`)
		dm, _, err := c.Analytics.Direct(ctx, Period30Days)
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "GET", "/v1/analytics/direct", "period=30d", "")
		if dm.Volume.MedianReplyMinutes != 45 || dm.Tracking.MailboxesOptedIn != 1 || !dm.Mailboxes[0].TrackDirectMail || dm.TopContacts[0].Email != "x@y.com" {
			t.Errorf("direct wrong: %+v", dm)
		}

		c, got = mbClient(t, 200, `{"enabled":true,"data":[{"id":"r1","message_id":"m1","thread_id":"t1","kind":"reply","kind_confidence":0.9,"kind_source":"model","intent":"not_now","intent_confidence":0.8,
			"relevance":3,"priority":"normal","needs_review":true,"review_reason":"low","labels":["Later maybe"],"answers":{"q1":true},"model":"x","input_tokens":120,"actions":["hold"],"return_date":null,"created_at":"2026-01-01T00:00:00Z"}],
			"total":1,"summary":{"total":1,"needs_review":1,"from_offline":0,"acted":0},"pagination":{"next_cursor":null,"has_more":false}}`)
		tag, _, err := c.Analytics.InboxTagging(ctx, &InboxTagReviewParams{ListOptions: ListOptions{Limit: 50}, NeedsReviewOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		got.want(t, "GET", "/v1/analytics/inbox-tagging", "limit=50&needs_review=true", "")
		if !tag.Enabled || tag.Data[0].ReturnDate != nil || string(tag.Data[0].Answers) != `{"q1":true}` || tag.Summary.NeedsReview != 1 || tag.Pagination.NextCursor != nil {
			t.Errorf("tagging wrong: %+v", tag)
		}
	})

	t.Run("Deliverability new fields", func(t *testing.T) {
		c, _ := mbClient(t, 200, `{"from":"2026-01-01T00:00:00Z","to":"2026-01-08T00:00:00Z","intent_automated":4,"band":"healthy","score":90,
			"by_provider":[{"provider":"gmail","label":"Gmail","samples":10,"inbox":8,"promotions":1,"spam":0,"other":0,"missing":1,"inbox_rate":80,"spam_rate":0}],
			"warmup_placement":[{"provider":"gmail","label":"Gmail","delivered":20,"spam":1,"inbox_rate":95,"spam_rate":5}]}`)
		d, _, err := c.Analytics.Deliverability(ctx, day("2026-01-01"), day("2026-01-08"))
		if err != nil {
			t.Fatal(err)
		}
		if d.IntentAutomated != 4 || d.ByProvider[0].Missing != 1 || d.ByProvider[0].Label != "Gmail" || d.WarmupPlacement[0].Label != "Gmail" || d.WarmupPlacement[0].Domain != "" {
			t.Errorf("deliverability wrong: %+v", d)
		}
	})

	t.Run("Unibox list, move, seen, snooze, forward", func(t *testing.T) {
		c, got := mbClient(t, 200, `{"data":[],"pagination":{"has_more":false}}`)
		if _, err := c.Unibox.List(ctx, &UniboxListParams{Automated: Bool(false), IncludeArchived: true, Folder: ""}); err != nil {
			t.Fatal(err)
		}
		got.want(t, "GET", "/v1/unibox", "automated=false&include_archived=true", "")

		c, got = mbClient(t, 200, `{"folder":"archive","email_ids":[],"thread_ids":["t1"]}`)
		if _, err := c.Unibox.MoveToFolder(ctx, &MoveFolderParams{Folder: FolderArchive, ThreadIDs: []string{"t1"}}); err != nil {
			t.Fatal(err)
		}
		got.want(t, "PATCH", "/v1/unibox/folder", "", `{"folder":"archive","thread_ids":["t1"]}`)

		if _, err := c.Unibox.MarkThreadsSeen(ctx, []string{"t1"}, true); err != nil {
			t.Fatal(err)
		}
		got.want(t, "PATCH", "/v1/unibox/seen", "", `{"thread_ids":["t1"],"seen":true}`)

		if _, _, err := c.Unibox.Reply(ctx, &UniboxReplyParams{EmailAccountID: "e1", To: []string{"x@y.com"}, Subject: "Fwd", ForwardMessageID: "m1"}); err != nil {
			t.Fatal(err)
		}
		got.want(t, "POST", "/v1/unibox/reply", "", `"forward_message_id":"m1"`)

		// One thread: the server answers with the bare row.
		c, got = mbClient(t, 200, `{"id":"s1","user_id":"u1","thread_id":"t1","snoozed_until":"2026-02-01T00:00:00Z"}`)
		rows, _, err := c.Unibox.SnoozeThreads(ctx, []string{"t1"}, day("2026-02-01"))
		if err != nil || len(rows) != 1 || rows[0].ThreadID != "t1" {
			t.Fatalf("single snooze = %+v err=%v", rows, err)
		}
		got.want(t, "POST", "/v1/unibox/snooze", "", `"thread_ids":["t1"]`)
		// Several: {"data": [...]}.
		c, _ = mbClient(t, 200, `{"data":[{"id":"s1","thread_id":"t1"},{"id":"s2","thread_id":"t2"}]}`)
		rows, _, err = c.Unibox.SnoozeThreads(ctx, []string{"t1", "t2"}, day("2026-02-01"))
		if err != nil || len(rows) != 2 {
			t.Fatalf("multi snooze = %+v err=%v", rows, err)
		}

		c, got = mbClient(t, 204, ``)
		if _, err := c.Unibox.UnsnoozeThreads(ctx, []string{"t1", "t2"}); err != nil {
			t.Fatal(err)
		}
		got.want(t, "DELETE", "/v1/unibox/snooze", "thread_id=t1%2Ct2", "")

		c, _ = mbClient(t, 200, `{"total":1,"unread":1,"today":0,"week":0,"snoozed":0,"awaiting_reply":0,"automated":7,"automated_unread":2,"awaiting_agent_draft":0,"scheduled_pending":1,"scheduled_pending_max":100}`)
		ov, _, err := c.Unibox.Overview(ctx)
		if err != nil || ov.Automated != 7 || ov.AutomatedUnread != 2 {
			t.Fatalf("overview = %+v err=%v", ov, err)
		}

		c, _ = mbClient(t, 200, `{"data":[{"id":"m1","email_id":"e1","thread_id":"t1","subject":"Re","internal_date":"2026-01-01T00:00:00Z","seen":true,"answers_mailbox_id":"e2"}],"pagination":{"has_more":false}}`)
		th, err := c.Unibox.Thread(ctx, "t1", "", nil)
		if err != nil || th.Data[0].AnswersMailboxID == nil || *th.Data[0].AnswersMailboxID != "e2" {
			t.Fatalf("thread = %+v err=%v", th, err)
		}
	})

	t.Run("Outreach settings round-trip keeps new keys", func(t *testing.T) {
		c, _ := mbClient(t, 200, `{"reply_intent":{"enabled":true,"crm_task_intents":null,"hold_on_out_of_office":true,"out_of_office_hold_days":7},
			"inbox_tagging":{"hold_on_not_now":true,"not_now_hold_days":30,"stop_on_declined":true,"task_on_call_request":true,"suppress_on_removal_request":false,
				"questions":[{"id":"q1","type":"choice","question":"Q?","action":{"type":""},"choices":[{"label":"A b","description":"d","action":{"type":"hold","hold_days":10}},{"label":"C","description":"e","action":{"type":""}}]}],
				"languages":["de"],"action_required_in_inbox":true}}`)
		o, _, err := c.Outreach.Get(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if o.ReplyIntent.CRMTaskIntents != nil || !o.ReplyIntent.HoldOnOutOfOffice || o.ReplyIntent.OutOfOfficeHoldDays != 7 || !o.InboxTagging.HoldOnNotNow ||
			o.InboxTagging.Questions[0].Choices[0].Action.HoldDays != 10 || o.InboxTagging.Languages[0] != "de" {
			t.Fatalf("settings wrong: %+v", o)
		}
		c, got := mbClient(t, 204, ``)
		if _, err := c.Outreach.Update(ctx, o); err != nil {
			t.Fatal(err)
		}
		got.want(t, "PATCH", "/v1/outreach/settings", "", `"inbox_tagging":{"hold_on_not_now":true,"not_now_hold_days":30`)
		for _, frag := range []string{`"crm_task_intents":null`, `"hold_on_out_of_office":true`, `"hold_days":10`, `"languages":["de"]`} {
			if !strings.Contains(got.body, frag) {
				t.Errorf("body %q missing %s", got.body, frag)
			}
		}
	})
}
