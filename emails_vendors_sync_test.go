package warmbly

import (
	"context"
	"net/http"
	"testing"
)

func TestMailboxVendorRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()

	runExactCases(t, &got, []exactCase{
		{"VendorCatalog", func() error { _, _, e := c.Emails.VendorCatalog(ctx); return e }, "GET", "/v1/emails/vendors/catalog", "", ""},
		{"ListVendors", func() error { _, _, e := c.Emails.ListVendors(ctx); return e }, "GET", "/v1/emails/vendors", "", ""},
		{"CreateVendor", func() error {
			_, _, e := c.Emails.CreateVendor(ctx, &VendorConnectionParams{Vendor: "inboxkit", Label: "Main", Fields: map[string]string{"api_key": "ik_1"}})
			return e
		}, "POST", "/v1/emails/vendors", "", `{"vendor":"inboxkit","label":"Main","fields":{"api_key":"ik_1"}}`},
		{"UpdateVendor rename", func() error {
			_, _, e := c.Emails.UpdateVendor(ctx, "vc_1", &VendorConnectionParams{Label: "Renamed"})
			return e
		}, "PATCH", "/v1/emails/vendors/vc_1", "", `{"label":"Renamed"}`},
		{"UpdateVendor nil", func() error { _, _, e := c.Emails.UpdateVendor(ctx, "vc_1", nil); return e }, "PATCH", "/v1/emails/vendors/vc_1", "", `{}`},
		{"DeleteVendor", func() error { _, e := c.Emails.DeleteVendor(ctx, "vc_1"); return e }, "DELETE", "/v1/emails/vendors/vc_1", "", ""},
		{"VendorMailboxes", func() error { _, _, e := c.Emails.VendorMailboxes(ctx, "vc_1"); return e }, "GET", "/v1/emails/vendors/vc_1/mailboxes", "", ""},
		{"ImportVendorMailboxes", func() error {
			_, _, e := c.Emails.ImportVendorMailboxes(ctx, "vc_1", &VendorImportParams{
				MailboxIDs: []string{"mb_1", "mb_2"},
				Options:    MailboxImportOptions{OnExisting: MailboxImportOnExistingSkip},
			})
			return e
		}, "POST", "/v1/emails/vendors/vc_1/import", "", `{"mailbox_ids":["mb_1","mb_2"],"options":{"on_existing":"skip","settings":{}}}`},
		{"ReconnectVendorMailboxes all", func() error {
			_, _, e := c.Emails.ReconnectVendorMailboxes(ctx, "vc_1", nil)
			return e
		}, "POST", "/v1/emails/vendors/vc_1/reconnect", "", `{}`},
		{"ReconnectVendorMailboxes some", func() error {
			_, _, e := c.Emails.ReconnectVendorMailboxes(ctx, "vc_1", &VendorReconnectParams{EmailAccountIDs: []string{"acc_1", "acc_2"}})
			return e
		}, "POST", "/v1/emails/vendors/vc_1/reconnect", "", `{"email_account_ids":["acc_1","acc_2"]}`},
		{"ReconnectVendorMailboxes one", func() error {
			_, _, e := c.Emails.ReconnectVendorMailboxes(ctx, "vc_1", &VendorReconnectParams{EmailAccountID: String("acc_1")})
			return e
		}, "POST", "/v1/emails/vendors/vc_1/reconnect", "", `{"email_account_id":"acc_1"}`},
		{"VendorReconnectStatus", func() error { _, _, e := c.Emails.VendorReconnectStatus(ctx, "vc_1"); return e }, "GET", "/v1/emails/vendors/vc_1/reconnect", "", ""},
	})
}

func TestMailboxVendorCatalogDecode(t *testing.T) {
	c := emailFixtureClient(t, http.StatusOK, `{"data": [{"id": "inboxkit", "label": "InboxKit", "website": "https://inboxkit.test",
		"key_help_url": "https://inboxkit.test/keys", "signs_in": true,
		"fields": [{"key": "api_key", "label": "API key", "secret": true, "required": true, "help": "Settings > API"}]}]}`, nil)
	cat, _, err := c.Emails.VendorCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cat) != 1 || !cat[0].SignsIn || len(cat[0].Fields) != 1 || !cat[0].Fields[0].Secret || cat[0].Fields[0].Help == "" {
		t.Errorf("catalog = %+v", cat)
	}
	// The agency catalog is the same type.
	a := AgencyVendorList{Catalog: cat}.Catalog[0]
	if a.ID != "inboxkit" {
		t.Errorf("agency vendor = %+v", a)
	}
}

func TestMailboxVendorListDecode(t *testing.T) {
	c := emailFixtureClient(t, http.StatusOK, `{"data": [
		{"id": "vc_1", "vendor": "inboxkit", "label": "Main", "status": "invalid", "last_error": "The key was revoked.",
			"last_used_at": "2026-11-13T12:00:00Z", "created_at": "2026-10-01T12:00:00Z", "mailboxes": 40},
		{"id": "vc_2", "vendor": "zapmail", "label": "Agency", "status": "active", "created_at": "2026-10-02T12:00:00Z",
			"mailboxes": 0, "agency_connection_id": "avc_1"}
	]}`, nil)
	list, _, err := c.Emails.ListVendors(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].LastUsedAt == nil || list[0].LastError == "" || list[0].AgencyConnectionID != nil {
		t.Fatalf("list = %+v", list)
	}
	if list[1].LastUsedAt != nil || list[1].AgencyConnectionID == nil || *list[1].AgencyConnectionID != "avc_1" {
		t.Errorf("vendor 1 = %+v", list[1])
	}
}

func TestMailboxVendorMailboxesDecode(t *testing.T) {
	c := emailFixtureClient(t, http.StatusOK, `{"data": [
		{"id": "mb_1", "email": "ana@acme.io", "name": "Ana", "domain": "acme.io", "provider": "google", "status": "active",
			"workspace": "Acme", "connected": true, "email_account_id": "acc_1"},
		{"id": "mb_2", "email": "bo@acme.io", "name": "Bo", "domain": "acme.io", "provider": "microsoft", "status": "active", "connected": false}
	]}`, nil)
	boxes, _, err := c.Emails.VendorMailboxes(context.Background(), "vc_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(boxes) != 2 || boxes[0].EmailAccountID == nil || boxes[0].Workspace != "Acme" || boxes[1].Connected || boxes[1].EmailAccountID != nil {
		t.Errorf("mailboxes = %+v", boxes)
	}
}

func TestMailboxVendorReconnectDecode(t *testing.T) {
	var got recordedRequest
	c := emailFixtureClient(t, http.StatusAccepted, `{"data": [
		{"email_account_id": "acc_1", "email": "ana@acme.io", "status": "queued"},
		{"email_account_id": "acc_2", "email": "bo@acme.io", "status": "failed", "message": "The vendor no longer has this mailbox."}
	]}`, &got)
	rows, resp, err := c.Emails.ReconnectVendorMailboxes(context.Background(), "vc_1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusAccepted || len(rows) != 2 || rows[0].Status != VendorReconnectQueued || rows[1].Message == "" {
		t.Errorf("rows = %+v", rows)
	}
}

func TestMailboxVendorImportDecode(t *testing.T) {
	c := emailFixtureClient(t, http.StatusCreated, `{"id": "imp_2", "status": "running", "source": "vendor", "filename": "Main",
		"vendor": "inboxkit", "on_existing": "update", "created_at": "2026-11-14T09:00:00Z", "updated_at": "2026-11-14T09:00:00Z",
		"total": 2, "counts": {"queued": 2}, "causes": [], "authorizing": {}}`, nil)
	imp, resp, err := c.Emails.ImportVendorMailboxes(context.Background(), "vc_1", &VendorImportParams{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated || imp.Source != "vendor" || imp.Counts.Queued != 2 {
		t.Errorf("import = %+v", imp)
	}
}
