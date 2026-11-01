package warmbly

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
)

// The fixtures below mirror the JSON the server's models produce, field for
// field, so a renamed or retyped field fails here rather than in production.

func TestMailboxImportPreviewDecode(t *testing.T) {
	const body = `{
		"format": "csv",
		"has_header": true,
		"vendor": {"id": "inboxkit", "label": "InboxKit"},
		"columns": [
			{"index": 0, "header": "Email", "samples": ["a@acme.io"], "secret": false, "field": "email", "source": "alias", "confidence": 1},
			{"index": 1, "header": "Pass", "samples": ["****"], "secret": true, "field": "password", "source": "shape", "confidence": 0.8}
		],
		"mapping": {"0": "email", "1": "password"},
		"saved_mapping": false,
		"summary": {"total": 3, "ready": 1, "needs_signin": 1, "invalid": 0, "existing": 1, "duplicate": 0},
		"rows": [
			{"line": 2, "email": "a@acme.io", "name": "A", "mail_host": "google_workspace", "auth_method": "app_password", "status": "needs_signin", "cause": "google_signin", "problem": "Google needs an app password.",
			 "smtp": {"host": "smtp.gmail.com", "port": 465, "security": "tls", "username": "a@acme.io"},
			 "imap": {"host": "imap.gmail.com", "port": 993, "security": "tls"}},
			{"line": 3, "email": "b@acme.io", "name": "", "mail_host": "", "auth_method": "password", "status": "a_state_from_the_future"}
		],
		"domains": [{
			"domain": "acme.io", "mail_host": "google_workspace", "label": "Google Workspace", "source": "mx", "rows": 3,
			"password_auth": "app_password", "app_password_url": "https://myaccount.google.com/apppasswords",
			"auth": {"state": "passing", "spf": true, "dkim": true, "dmarc": true},
			"tracking": {"host": "link.acme.io", "status": "suggested", "cname_target": "t.warmbly.com"},
			"redirect": {"id": "r1", "domain": "acme.io", "target_url": "https://acme.com", "include_www": true, "verified": false, "created_at": "2026-09-10T08:00:00Z", "records": [{"purpose": "ownership", "type": "TXT", "name": "_warmbly.acme.io", "value": "abc", "ok": false}], "served_by": "instance"},
			"vendor_domain": {"vendor": "inboxkit", "connection_id": "c1", "can_forward": true, "can_unforward": false, "forwarding_reviewed": false, "can_dns": true, "dns_types": ["CNAME"]}
		}],
		"issues": [{"cause": "google_signin", "title": "Connect with Google sign-in", "fix": "Use an app password.", "count": 1, "lines": [2]}],
		"allowance": {"used": 10, "allowance": 50, "remaining": 40, "basis": "plan", "sends_per_mailbox": 1, "paid": true},
		"max_rows": 5000
	}`
	var got recordedRequest
	c := emailFixtureClient(t, http.StatusOK, body, &got)
	p, _, err := c.Emails.PreviewImport(context.Background(), &MailboxImportParams{Text: "a@acme.io"})
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	if p.Vendor == nil || p.Vendor.ID != "inboxkit" || p.Mapping["1"] != ImportFieldPassword || p.MaxRows != 5000 {
		t.Errorf("preview = %+v", p)
	}
	if !p.Columns[1].Secret || p.Columns[1].Confidence != 0.8 || p.Summary.NeedsSignin != 1 {
		t.Errorf("columns/summary = %+v %+v", p.Columns, p.Summary)
	}
	if p.Rows[0].Status != MailboxImportPreviewNeedsSignin || p.Rows[0].SMTP == nil || p.Rows[0].SMTP.Port != 465 || p.Rows[0].IMAP.Username != "" {
		t.Errorf("row 0 = %+v", p.Rows[0])
	}
	if p.Rows[1].Status != "a_state_from_the_future" {
		t.Errorf("unknown status did not survive: %q", p.Rows[1].Status)
	}
	d := p.Domains[0]
	if d.Auth == nil || d.Auth.State != AuthStatePassing || d.Tracking == nil || d.Tracking.Status != TrackingSuggestionSuggested {
		t.Errorf("domain = %+v", d)
	}
	if d.Redirect == nil || d.Redirect.ServedBy != RedirectServedByInstance || len(d.Redirect.Records) != 1 || d.VendorDomain == nil || !d.VendorDomain.CanDNS {
		t.Errorf("domain redirect/vendor = %+v %+v", d.Redirect, d.VendorDomain)
	}
	if p.Allowance == nil || p.Allowance.Remaining == nil || *p.Allowance.Remaining != 40 {
		t.Errorf("allowance = %+v", p.Allowance)
	}
}

func TestMailboxImportDecode(t *testing.T) {
	const imp = `{
		"id": "0c0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b",
		"created_by": "a1b2c3d4-0000-0000-0000-000000000000",
		"status": "running",
		"source": "vendor",
		"filename": "",
		"vendor": "zapmail",
		"on_existing": "update",
		"created_at": "2026-09-10T08:00:00Z",
		"updated_at": "2026-09-10T08:01:00Z",
		"credentials_expire_at": "2026-09-17T08:00:00Z",
		"total": 10,
		"counts": {"queued": 2, "running": 1, "connected": 5, "updated": 0, "skipped": 0, "failed": 1, "needs_signin": 1},
		"causes": [{"cause": "no_worker", "title": "No worker available", "fix": "Retry.", "count": 1, "retryable": true}],
		"authorizing": {"google": 3}
	}`
	var got recordedRequest
	c := emailFixtureClient(t, http.StatusCreated, imp, &got)
	i, resp, err := c.Emails.ImportVendorMailboxes(context.Background(), "v1", &VendorImportParams{All: true})
	if err != nil {
		t.Fatalf("ImportVendorMailboxes: %v", err)
	}
	if resp.StatusCode != http.StatusCreated || i.Status != MailboxImportRunning || i.Counts.Connected != 5 || i.Counts.NeedsSignin != 1 {
		t.Errorf("import = %+v", i)
	}
	if i.Finished() || i.CredentialsExpireAt == nil || i.Authorizing["google"] != 3 || !i.Causes[0].Retryable || i.CreatedBy == nil {
		t.Errorf("import detail = %+v", i)
	}

	// A finished import carries finished_at.
	c2 := emailFixtureClient(t, http.StatusOK, `{"id":"x","status":"completed","finished_at":"2026-09-10T09:00:00Z","counts":{},"causes":[],"authorizing":{}}`, nil)
	done, _, err := c2.Emails.GetImport(context.Background(), "x")
	if err != nil {
		t.Fatalf("GetImport: %v", err)
	}
	if !done.Finished() || done.Status != MailboxImportCompleted {
		t.Errorf("done = %+v", done)
	}
}

func TestMailboxImportRowsDecode(t *testing.T) {
	const body = `{
		"data": [
			{"line": 2, "email": "a@acme.io", "name": "A", "mail_host": "other", "status": "failed", "code": "mailbox_unreachable", "cause": "no_worker", "message": "No worker answered.",
			 "retryable": true, "fields": {"email": "a@acme.io", "smtp_host": "smtp.acme.io"}, "updated_at": "2026-09-10T08:00:00Z"},
			{"line": 3, "email": "b@acme.io", "name": "", "mail_host": "", "status": "connected", "code": "", "cause": "", "message": "",
			 "email_account_id": "0c0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b", "retryable": false, "fields": {}, "updated_at": "2026-09-10T08:00:00Z"}
		],
		"pagination": {"next_cursor": "abc", "has_more": true}
	}`
	var got recordedRequest
	c := emailFixtureClient(t, http.StatusOK, body, &got)
	page, err := c.Emails.ImportRows(context.Background(), "imp_1", nil)
	if err != nil {
		t.Fatalf("ImportRows: %v", err)
	}
	if len(page.Data) != 2 || !page.HasMore() || page.NextCursor() != "abc" {
		t.Fatalf("page = %+v", page.Pagination)
	}
	if r := page.Data[0]; r.Status != MailboxImportRowFailed || !r.Retryable || r.Fields["smtp_host"] != "smtp.acme.io" || r.EmailAccountID != nil {
		t.Errorf("row 0 = %+v", r)
	}
	if r := page.Data[1]; r.EmailAccountID == nil || r.Status != MailboxImportRowConnected {
		t.Errorf("row 1 = %+v", r)
	}

	// The last page carries an empty cursor, not a null one.
	c2 := emailFixtureClient(t, http.StatusOK, `{"data":[],"pagination":{"next_cursor":"","has_more":false}}`, nil)
	last, err := c2.Emails.ListImports(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListImports: %v", err)
	}
	if last.HasMore() {
		t.Error("an empty cursor must not report more pages")
	}
}

func TestMailboxNoContentRoutes(t *testing.T) {
	var got recordedRequest
	c := emailFixtureClient(t, http.StatusNoContent, "", &got)
	ctx := context.Background()
	for name, call := range map[string]func() (*Response, error){
		"DismissImport":        func() (*Response, error) { return c.Emails.DismissImport(ctx, "imp_1") },
		"DeleteGrant":          func() (*Response, error) { return c.Emails.DeleteGrant(ctx, "g_1") },
		"DeleteVendor":         func() (*Response, error) { return c.Emails.DeleteVendor(ctx, "v_1") },
		"DeleteDomainRedirect": func() (*Response, error) { return c.Emails.DeleteDomainRedirect(ctx, "a.io") },
		"DeleteImage":          func() (*Response, error) { return c.Emails.DeleteImage(ctx, "i_1") },
	} {
		resp, err := call()
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("%s status = %d", name, resp.StatusCode)
		}
	}
}

func TestImportFailuresDownload(t *testing.T) {
	const csv = "line,email,error\n2,a@acme.io,No worker\n"
	var got recordedRequest
	c := emailFixtureClient(t, http.StatusOK, csv, &got)
	var buf bytes.Buffer
	if _, err := c.Emails.DownloadImportFailures(context.Background(), "imp_1", &buf); err != nil {
		t.Fatalf("DownloadImportFailures: %v", err)
	}
	if buf.String() != csv {
		t.Errorf("csv = %q", buf.String())
	}
}

func TestGrantsDecode(t *testing.T) {
	var got recordedRequest

	c := emailFixtureClient(t, http.StatusOK, `{
		"google_enabled": true, "google_client_id": "123.apps.googleusercontent.com",
		"google_scopes": ["https://mail.google.com/"], "microsoft_enabled": false,
		"google_missing": [], "microsoft_missing": ["BOX_OUTLOOK_CLIENT_ID"]
	}`, &got)
	cfg, _, err := c.Emails.GrantConfig(context.Background())
	if err != nil {
		t.Fatalf("GrantConfig: %v", err)
	}
	if !cfg.GoogleEnabled || cfg.MicrosoftEnabled || len(cfg.GoogleScopes) != 1 || cfg.MicrosoftMissing[0] != "BOX_OUTLOOK_CLIENT_ID" {
		t.Errorf("config = %+v", cfg)
	}

	c = emailFixtureClient(t, http.StatusOK, `{"data": [
		{"id": "0c0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b", "provider": "google", "tenant": "acme.io", "admin_email": "admin@acme.io",
		 "domains": ["acme.io"], "status": "invalid", "last_error": "Delegation missing.", "verified_at": "2026-09-01T08:00:00Z",
		 "created_at": "2026-08-01T08:00:00Z", "mailboxes": 12}
	]}`, &got)
	grants, _, err := c.Emails.ListGrants(context.Background())
	if err != nil {
		t.Fatalf("ListGrants: %v", err)
	}
	g := grants[0]
	if g.Provider != GrantProviderGoogle || g.Status != "invalid" || g.Mailboxes != 12 || g.VerifiedAt == nil || g.Domains[0] != "acme.io" {
		t.Errorf("grant = %+v", g)
	}

	c = emailFixtureClient(t, http.StatusOK, `{"method":"dns","txt_name":"_warmbly.acme.io","txt_value":"proof"}`, &got)
	st, _, err := c.Emails.StartGoogleGrant(context.Background(), "acme.io", "admin@acme.io")
	if err != nil {
		t.Fatalf("StartGoogleGrant: %v", err)
	}
	if st.Method != GrantMethodDNS || st.URL != "" || st.TXTValue != "proof" {
		t.Errorf("start = %+v", st)
	}

	c = emailFixtureClient(t, http.StatusCreated, `{"id":"g1","provider":"microsoft","tenant":"t","domains":[],"status":"active","created_at":"2026-08-01T08:00:00Z","mailboxes":0}`, &got)
	fin, resp, err := c.Emails.FinishMicrosoftGrant(context.Background(), "s", "c")
	if err != nil || resp.StatusCode != http.StatusCreated || fin.Provider != GrantProviderMicrosoft {
		t.Errorf("FinishMicrosoftGrant = %+v %v %v", fin, resp, err)
	}

	c = emailFixtureClient(t, http.StatusOK, `{"data": [
		{"id": "a@acme.io", "email": "a@acme.io", "name": "A", "enabled": true, "connected": true, "email_account_id": "0c0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b", "upgrade": true},
		{"id": "b@acme.io", "email": "b@acme.io", "name": "B", "enabled": false, "connected": false, "upgrade": false}
	]}`, &got)
	users, _, err := c.Emails.GrantUsers(context.Background(), "g1")
	if err != nil {
		t.Fatalf("GrantUsers: %v", err)
	}
	if len(users) != 2 || users[0].EmailAccountID == nil || !users[0].Upgrade || users[1].EmailAccountID != nil {
		t.Errorf("users = %+v", users)
	}

	c = emailFixtureClient(t, http.StatusOK, `{"data": [
		{"domain": "acme.io", "kind": "workspace", "grant_id": "0c0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b",
		 "mailboxes": [{"id": "m1", "email": "a@acme.io", "name": "A", "status": "active"}]},
		{"domain": "gmail.com", "kind": "personal", "mailboxes": []}
	], "total": 1}`, &got)
	mig, _, err := c.Emails.SigninMigration(context.Background())
	if err != nil {
		t.Fatalf("SigninMigration: %v", err)
	}
	if mig.Total != 1 || mig.Data[0].Kind != SigninMigrationWorkspace || mig.Data[0].GrantID == nil || mig.Data[1].GrantID != nil || mig.Data[0].Mailboxes[0].Email != "a@acme.io" {
		t.Errorf("migration = %+v", mig)
	}
}

func TestSendingDomainsDecode(t *testing.T) {
	var got recordedRequest

	c := emailFixtureClient(t, http.StatusOK, `{"data": [{
		"domain": "acme.io", "mailboxes": 4, "mail_hosts": ["google_workspace"], "auth_state": "failing",
		"auth_spf": true, "auth_dkim": false, "auth_dmarc": false,
		"tracking_domains": [{"host": "link.acme.io", "verified": true, "mailboxes": 4}],
		"redirect": {
			"id": "0c0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b", "domain": "acme.io", "target_url": "https://acme.com", "include_www": true,
			"verified": true, "verified_at": "2026-09-01T08:00:00Z", "last_checked_at": "2026-09-02T08:00:00Z", "created_at": "2026-08-01T08:00:00Z",
			"records": [{"purpose": "route", "type": "A", "name": "acme.io", "value": "203.0.113.5", "ok": true, "optional": true}],
			"served_by": "cloud", "serve_host": "r.warmbly.com",
			"reach": {"status": "not_reaching", "hint": "not_routed", "detail": "A proxy answered.", "proxy": "traefik", "checked_at": "2026-09-02T08:00:00Z"},
			"target_active": false, "target_inactive_reason": "Not one of your domains.",
			"website": {"domain": "acme.com", "verified": false, "record": {"purpose": "website", "type": "TXT", "name": "_warmbly.acme.com", "value": "abc", "ok": false}}
		},
		"vendors": ["inboxkit"], "vendor_connection_ids": ["0c0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b"]
	}]}`, &got)
	doms, _, err := c.Emails.ListDomains(context.Background())
	if err != nil {
		t.Fatalf("ListDomains: %v", err)
	}
	d := doms[0]
	if d.AuthState != AuthStateFailing || d.TrackingDomains[0].Host != "link.acme.io" || len(d.VendorConnectionIDs) != 1 || d.VendorDomain != nil {
		t.Errorf("domain = %+v", d)
	}
	r := d.Redirect
	if r == nil || r.ServedBy != RedirectServedByCloud || r.Reach == nil || r.Reach.Status != RedirectReachNotReaching || r.Reach.Hint != "not_routed" || r.Reach.Proxy != "traefik" {
		t.Fatalf("redirect = %+v", r)
	}
	if r.TargetActive == nil || *r.TargetActive || r.Website == nil || r.Website.Record == nil || r.Website.Record.Type != "TXT" || !r.Records[0].Optional {
		t.Errorf("redirect detail = %+v", r)
	}

	c = emailFixtureClient(t, http.StatusOK, `{"tracking": {"tracking_domain": "link.acme.io", "tracking_domain_verified": false, "tracking_domain_verified_at": null,
		"cname_target": "t.warmbly.com", "txt_name": "_warmbly-track.link.acme.io", "txt_value": "tok", "status": "proof_missing", "message": "Publish the TXT record."}, "mailboxes": 4}`, &got)
	res, _, err := c.Emails.SetDomainTracking(context.Background(), "acme.io", "link.acme.io")
	if err != nil {
		t.Fatalf("SetDomainTracking: %v", err)
	}
	if res.Mailboxes != 4 || res.Tracking.Status != TrackingStatusProofMissing || res.Tracking.TXTName == "" || res.Tracking.TXTValue != "tok" {
		t.Errorf("tracking = %+v", res)
	}

	c = emailFixtureClient(t, http.StatusOK, `{"data": [
		{"domain": "a.io", "tracking": {"host": "link.a.io", "via": "vendor", "verified": true, "mailboxes": 2},
		 "redirect": {"target_url": "https://acme.com", "via": "vendor", "verified": false, "reviewed": true}},
		{"domain": "b.io", "tracking": {"host": "link.b.io", "via": "dns", "verified": false, "mailboxes": 1, "cname_target": "t.warmbly.com", "note": "Vendor cannot write CNAMEs.", "error": "x", "code": "mailbox_vendor_domain_unsupported"}}
	]}`, &got)
	rows, _, err := c.Emails.BulkSetupDomains(context.Background(), &BulkDomainSetupParams{Domains: []string{"a.io", "b.io"}})
	if err != nil {
		t.Fatalf("BulkSetupDomains: %v", err)
	}
	if len(rows) != 2 || rows[0].Tracking.Via != BulkViaVendor || !rows[0].Redirect.Reviewed || rows[1].Redirect != nil || rows[1].Tracking.Code == "" {
		t.Errorf("bulk = %+v", rows)
	}

	c = emailFixtureClient(t, http.StatusOK, `{"host": "link.acme.io", "status": "found", "cname_target": "t.warmbly.com"}`, &got)
	sug, _, err := c.Emails.TrackingSuggestion(context.Background(), "acme.io")
	if err != nil || sug.Status != TrackingSuggestionFound || sug.VendorDomain != nil {
		t.Errorf("suggestion = %+v %v", sug, err)
	}
}

func TestVendorsDecode(t *testing.T) {
	var got recordedRequest

	c := emailFixtureClient(t, http.StatusOK, `{"data": [{
		"id": "inboxkit", "label": "InboxKit", "website": "https://inboxkit.com", "key_help_url": "https://inboxkit.com/keys",
		"fields": [{"key": "api_key", "label": "API key", "secret": true, "required": true, "help": "Find it in Settings."}]
	}]}`, &got)
	cat, _, err := c.Emails.VendorCatalog(context.Background())
	if err != nil {
		t.Fatalf("VendorCatalog: %v", err)
	}
	if len(cat) != 1 || !cat[0].Fields[0].Secret || cat[0].Fields[0].Key != "api_key" {
		t.Errorf("catalog = %+v", cat)
	}

	c = emailFixtureClient(t, http.StatusCreated, `{"id":"0c0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b","vendor":"inboxkit","label":"Main","status":"active","last_used_at":"2026-09-01T08:00:00Z","created_at":"2026-08-01T08:00:00Z","mailboxes":3}`, &got)
	v, resp, err := c.Emails.CreateVendor(context.Background(), &VendorCreateParams{Vendor: "inboxkit", Fields: map[string]string{"api_key": "k"}})
	if err != nil || resp.StatusCode != http.StatusCreated || v.Mailboxes != 3 || v.LastUsedAt == nil || v.LastError != "" {
		t.Errorf("CreateVendor = %+v %v", v, err)
	}

	c = emailFixtureClient(t, http.StatusOK, `{"data": [
		{"id": "mb1", "email": "a@acme.io", "name": "A", "domain": "acme.io", "provider": "google", "status": "active", "workspace": "Team", "connected": true, "email_account_id": "0c0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b"},
		{"id": "mb2", "email": "b@acme.io", "name": "B", "domain": "acme.io", "provider": "microsoft", "status": "pending", "connected": false}
	]}`, &got)
	boxes, _, err := c.Emails.VendorMailboxes(context.Background(), "v1")
	if err != nil {
		t.Fatalf("VendorMailboxes: %v", err)
	}
	if len(boxes) != 2 || boxes[0].EmailAccountID == nil || boxes[0].Workspace != "Team" || boxes[1].Connected {
		t.Errorf("mailboxes = %+v", boxes)
	}
}

func TestMailboxSettingsDecode(t *testing.T) {
	var got recordedRequest

	// GET /emails/:id/sync now carries the skip list and the folder list.
	c := emailFixtureClient(t, http.StatusOK, `{
		"state": {"transport_generation": "0c0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b", "backfill_status": "running",
			"backfill_cursor": {"page_token": "p", "folders": {"INBOX": {"uid": 40}}, "google_recovery": {"history_id": 99, "since": "2026-09-01T08:00:00Z", "messages_done": true}},
			"backfill_synced": 120, "deferred": 0, "folders_skipped_cap": 3, "folders_skipped_conflict": 1},
		"policy": {"backfill_days": 90, "backfill_messages": 5000, "daily_messages": 500, "org_daily_messages": 5000, "skip_folders": ["Old"]},
		"skip_folders": ["Old"],
		"folders": [{"name": "INBOX", "folder": "inbox"}, {"name": "Old", "folder": "archive"}]
	}`, &got)
	sync, _, err := c.Emails.SyncStatus(context.Background(), "em_1")
	if err != nil {
		t.Fatalf("SyncStatus: %v", err)
	}
	if sync.State == nil || sync.State.FoldersSkippedCap != 3 || sync.State.FoldersSkippedConflict != 1 || sync.State.BackfillCursor.GoogleRecovery == nil || sync.State.BackfillCursor.GoogleRecovery.HistoryID != 99 {
		t.Errorf("state = %+v", sync.State)
	}
	if len(sync.SkipFolders) != 1 || sync.Policy.SkipFolders[0] != "Old" || len(sync.Folders) != 2 || sync.Folders[1].Folder != FolderArchive {
		t.Errorf("sync = %+v", sync)
	}

	c = emailFixtureClient(t, http.StatusOK, `{"skip_folders": ["Old"]}`, &got)
	skip, _, err := c.Emails.UpdateSyncFolders(context.Background(), "em_1", []string{"Old"})
	if err != nil || len(skip) != 1 || skip[0] != "Old" {
		t.Errorf("UpdateSyncFolders = %v %v", skip, err)
	}

	c = emailFixtureClient(t, http.StatusOK, `{"track_direct_mail": true}`, &got)
	on, _, err := c.Emails.SetDirectTracking(context.Background(), "em_1", true)
	if err != nil || !on {
		t.Errorf("SetDirectTracking = %v %v", on, err)
	}

	c = emailFixtureClient(t, http.StatusOK, `{
		"smtp": {"host": "smtp.acme.io", "port": 465, "tls_unverified": true, "tls_pin": "aa",
			"observed": {"fingerprint": "bb", "subject": "CN=smtp.acme.io", "issuer": "CN=smtp.acme.io", "not_after": "2027-01-01T00:00:00Z", "self_signed": true}},
		"imap": {"host": "imap.acme.io", "port": 993, "tls_unverified": false}
	}`, &got)
	certs, _, err := c.Emails.CheckCertificates(context.Background(), "em_1")
	if err != nil {
		t.Fatalf("CheckCertificates: %v", err)
	}
	if !certs.SMTP.TLSUnverified || certs.SMTP.Observed == nil || !certs.SMTP.Observed.SelfSigned || certs.SMTP.Observed.Fingerprint != "bb" || certs.IMAP.Observed != nil || certs.IMAP.TLSPin != "" {
		t.Errorf("certificates = %+v", certs)
	}

	c = emailFixtureClient(t, http.StatusOK, `{
		"supported": true, "provider": "gmail", "mailbox_email": "a@acme.io", "send_as_email": "sales@acme.io",
		"identities": [{"email": "a@acme.io", "name": "A", "is_primary": true, "is_default": true, "verified": true}, {"email": "sales@acme.io", "name": "Sales", "is_primary": false, "is_default": false, "verified": true}],
		"synced_at": "2026-09-01T08:00:00Z", "signature_source": "provider", "signature_imported_at": "2026-09-01T08:00:00Z"
	}`, &got)
	id, _, err := c.Emails.SendIdentity(context.Background(), "em_1")
	if err != nil {
		t.Fatalf("SendIdentity: %v", err)
	}
	if !id.Supported || len(id.Identities) != 2 || !id.Identities[0].IsPrimary || id.SendAsEmail != "sales@acme.io" || id.SignatureSource != SignatureSourceProvider || id.SyncedAt == nil {
		t.Errorf("identity = %+v", id)
	}

	// An unsupported provider answers an empty list, not an error.
	c = emailFixtureClient(t, http.StatusOK, `{"supported": false, "provider": "outlook", "mailbox_email": "a@acme.io", "send_as_email": "", "identities": [], "signature_source": "manual"}`, &got)
	id, _, err = c.Emails.SendIdentity(context.Background(), "em_1")
	if err != nil || id.Supported || id.SyncedAt != nil {
		t.Errorf("unsupported identity = %+v %v", id, err)
	}

	c = emailFixtureClient(t, http.StatusOK, `{"url": "https://login.microsoftonline.com/x", "state": "s", "admin_consent_url": "https://login.microsoftonline.com/admin"}`, &got)
	start, _, err := c.Emails.StartOAuthWithParams(context.Background(), &MailboxOAuthStartParams{Provider: ProviderOutlook})
	if err != nil || start.AdminConsentURL == "" {
		t.Errorf("StartOAuthWithParams = %+v %v", start, err)
	}
}

func TestEmailDecodeSecondSyncFields(t *testing.T) {
	const body = `{
		"id": "0c0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b",
		"user_id": "a1b2c3d4-0000-0000-0000-000000000000",
		"email": "sales@acme.io",
		"send_as_email": "team@acme.io",
		"provider": "gmail",
		"status": "active",
		"mail_host": "google_workspace",
		"auth_method": "delegated",
		"domain_grant_id": "5e6f7a8b-9c0d-1e2f-3a4b-5c6d7e8f9a0b",
		"vendor_connection_id": "6f7a8b9c-0d1e-2f3a-4b5c-6d7e8f9a0b1c",
		"vendor": "inboxkit",
		"avatar_url": "",
		"last_synced_at": null,
		"test_mode": "diagnostic",
		"test_send_enabled": true,
		"test_receive_enabled": false,
		"shared_daily_limit": 100,
		"rolling_recipient_limit": null,
		"track_direct_mail": true,
		"warmup": null,
		"warmup_placement": "archive",
		"warmup_folder": "",
		"warmup_retention_days": 14,
		"relay_folder_moves": true,
		"tags": []
	}`
	var got recordedRequest
	c := emailFixtureClient(t, http.StatusOK, body, &got)
	em, _, err := c.Emails.Get(context.Background(), "em_1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if em.SendAsEmail != "team@acme.io" || em.MailHost != "google_workspace" || em.AuthMethod != "delegated" || em.DomainGrantID == nil || em.VendorConnectionID == nil || em.Vendor != "inboxkit" {
		t.Errorf("origin = %+v", em)
	}
	if em.TestMode == nil || *em.TestMode != "diagnostic" || !em.TestSendEnabled || em.TestReceiveEnabled || em.SharedDailyLimit == nil || *em.SharedDailyLimit != 100 || em.RollingRecipientLimit != nil {
		t.Errorf("test participation = %+v", em)
	}
	if !em.TrackDirectMail || em.WarmupPlacement != "archive" || em.WarmupRetentionDays != 14 || !em.RelayFolderMoves {
		t.Errorf("settings = %+v", em)
	}

	// The update body carries the new settings.
	_, _, err = c.Emails.Update(context.Background(), "em_1", &EmailUpdateParams{
		SendAsEmail: String(""), WarmupPlacement: String("folder"), WarmupFolder: String("Warmup"), RelayFolderMoves: Bool(false),
		TestMode: String("off"), SharedDailyLimit: Int(50),
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	for _, want := range []string{`"send_as_email":""`, `"warmup_placement":"folder"`, `"warmup_folder":"Warmup"`, `"relay_folder_moves":false`, `"test_mode":"off"`, `"shared_daily_limit":50`} {
		if !strings.Contains(got.body, want) {
			t.Errorf("update body %q is missing %s", got.body, want)
		}
	}
}

func TestUniboxMoveToFolderDecode(t *testing.T) {
	var got recordedRequest
	c := emailFixtureClient(t, http.StatusOK, `{"email_ids": ["a","b"], "thread_ids": ["t1"], "folder": "trash"}`, &got)
	res, _, err := c.Unibox.MoveToFolder(context.Background(), &UniboxMoveParams{EmailIDs: []string{"a", "b"}, ThreadIDs: []string{"t1"}, Folder: FolderTrash})
	if err != nil {
		t.Fatalf("MoveToFolder: %v", err)
	}
	if res.Folder != FolderTrash || len(res.EmailIDs) != 2 || res.ThreadIDs[0] != "t1" {
		t.Errorf("result = %+v", res)
	}

	c = emailFixtureClient(t, http.StatusOK, `{
		"total": 10, "unread": 4, "today": 1, "week": 3, "snoozed": 0, "awaiting_reply": 2,
		"automated": 7, "automated_unread": 5, "awaiting_agent_draft": 1, "drafts": 6,
		"scheduled_pending": 0, "scheduled_pending_max": 200,
		"folders": [], "mailboxes": [], "tags": [], "categories": [],
		"generated_at": "2026-09-10T08:00:00Z", "window_today_start": "2026-09-10T00:00:00Z", "window_week_start": "2026-09-07T00:00:00Z"
	}`, &got)
	ov, _, err := c.Unibox.Overview(context.Background())
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if ov.Automated != 7 || ov.AutomatedUnread != 5 || ov.Drafts != 6 {
		t.Errorf("overview = %+v", ov)
	}
}
