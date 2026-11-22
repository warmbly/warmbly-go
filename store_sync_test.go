package warmbly

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStoreRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()

	runExactCases(t, &got, []exactCase{
		{"Status", func() error { _, _, e := c.Store.Status(ctx); return e }, "GET", "/v1/store/status", "", ""},
		{"SearchDomains", func() error {
			_, _, e := c.Store.SearchDomains(ctx, &StoreSearchParams{Query: "acme", TLDs: []string{"com", "io"}, ExactOnly: true})
			return e
		}, "GET", "/v1/store/domains/search", "q=acme&suggestions=false&tlds=com%2Cio", ""},
		{"SearchDomains nil", func() error { _, _, e := c.Store.SearchDomains(ctx, nil); return e }, "GET", "/v1/store/domains/search", "", ""},
		{"PutRegistrant", func() error {
			_, _, e := c.Store.PutRegistrant(ctx, &StoreRegistrant{FirstName: "Ada", LastName: "Lovelace", Email: "ada@acme.test",
				Phone: "+44.2071234567", Address1: "1 Main St", City: "London", PostalCode: "N1", Country: "GB"})
			return e
		}, "PUT", "/v1/store/registrant", "",
			`{"first_name":"Ada","last_name":"Lovelace","organization":"","email":"ada@acme.test","phone":"+44.2071234567","address1":"1 Main St","address2":"","city":"London","state":"","postal_code":"N1","country":"GB"}`},
		{"CreateQuote", func() error {
			_, _, e := c.Store.CreateQuote(ctx, &StoreQuoteParams{Items: []StoreQuoteItem{
				{Type: StoreItemDomain, Domain: "getacme.com", Years: 1},
				{Type: StoreItemMailboxes, Domain: "getacme.com", Provider: StoreProviderGoogle, PhotoID: "ph_1", Mailboxes: []StoreNewMailbox{
					{LocalPart: "ada", FirstName: "Ada", LastName: "Lovelace", Admin: true, PhotoID: "ph_2",
						Signature: &StoreMailboxSignature{HTML: "<p>Ada</p>", Plain: "Ada"}},
					{LocalPart: "bob", FirstName: "Bob", LastName: "Smith"},
				}},
			}})
			return e
		}, "POST", "/v1/store/quotes", "",
			`{"items":[{"type":"domain","domain":"getacme.com","years":1},{"type":"mailboxes","domain":"getacme.com","provider":"google","mailboxes":[{"local_part":"ada","first_name":"Ada","last_name":"Lovelace","admin":true,"photo_id":"ph_2","signature":{"html":"<p>Ada</p>","plain":"Ada"}},{"local_part":"bob","first_name":"Bob","last_name":"Smith"}],"photo_id":"ph_1"}]}`},
		{"CreateOrder", func() error {
			_, _, e := c.Store.CreateOrder(ctx, &StoreOrderParams{QuoteID: "q_1", ExpectedDueNowMicros: Int64(23170000),
				ForwardingURL: "https://acme.test", EnableAutoReload: Bool(false)})
			return e
		}, "POST", "/v1/store/orders", "",
			`{"quote_id":"q_1","expected_due_now_micros":23170000,"forwarding_url":"https://acme.test","enable_auto_reload":false}`},
		{"ListOrders", func() error {
			_, e := c.Store.ListOrders(ctx, &ListOptions{Limit: 20, Cursor: "cur_1"})
			return e
		}, "GET", "/v1/store/orders", "cursor=cur_1&limit=20", ""},
		{"ListOrders nil", func() error { _, e := c.Store.ListOrders(ctx, nil); return e }, "GET", "/v1/store/orders", "", ""},
		{"GetOrder", func() error { _, _, e := c.Store.GetOrder(ctx, "o_1"); return e }, "GET", "/v1/store/orders/o_1", "", ""},
		{"ListDomains", func() error {
			_, e := c.Store.ListDomains(ctx, &ListOptions{Limit: 100})
			return e
		}, "GET", "/v1/store/domains", "limit=100", ""},
		{"GetDomain", func() error { _, _, e := c.Store.GetDomain(ctx, "d_1"); return e }, "GET", "/v1/store/domains/d_1", "", ""},
		{"UpdateDomain", func() error {
			_, _, e := c.Store.UpdateDomain(ctx, "d_1", &StoreDomainUpdateParams{AutoRenew: Bool(false), ForwardingURL: String("")})
			return e
		}, "PATCH", "/v1/store/domains/d_1", "", `{"auto_renew":false,"forwarding_url":""}`},
		{"RenewDomain", func() error { _, _, e := c.Store.RenewDomain(ctx, "d_1"); return e }, "POST", "/v1/store/domains/d_1/renew", "", ""},
		{"TransferOut", func() error { _, _, e := c.Store.TransferOut(ctx, "d_1"); return e }, "POST", "/v1/store/domains/d_1/transfer-out", "", ""},
		{"ConnectDomain", func() error {
			_, _, e := c.Store.ConnectDomain(ctx, &StoreConnectDomainParams{Domain: "acme.com", Method: StoreConnectManual})
			return e
		}, "POST", "/v1/store/domains/connect", "", `{"domain":"acme.com","method":"manual"}`},
		{"CheckConnection", func() error { _, _, e := c.Store.CheckConnection(ctx, "d_1"); return e }, "POST", "/v1/store/domains/d_1/connection/check", "", ""},
		{"RegenerateConnection", func() error { _, _, e := c.Store.RegenerateConnection(ctx, "d_1"); return e }, "POST", "/v1/store/domains/d_1/connection/regenerate", "", ""},
		{"AddDNSRecord", func() error {
			_, _, e := c.Store.AddDNSRecord(ctx, "d_1", &StoreDNSRecordParams{Type: "MX", Name: "@", Content: "mx.acme.test", TTL: 300, Priority: Int(10)})
			return e
		}, "POST", "/v1/store/domains/d_1/dns", "", `{"type":"MX","name":"@","content":"mx.acme.test","ttl":300,"priority":10}`},
		{"AddDNSRecord minimal", func() error {
			_, _, e := c.Store.AddDNSRecord(ctx, "d_1", &StoreDNSRecordParams{Type: "TXT", Name: "@", Content: "hello"})
			return e
		}, "POST", "/v1/store/domains/d_1/dns", "", `{"type":"TXT","name":"@","content":"hello"}`},
		{"UpdateDNSRecord", func() error {
			_, _, e := c.Store.UpdateDNSRecord(ctx, "d_1", "r_1", &StoreDNSRecordUpdateParams{Content: String("v=spf1 -all"), TTL: Int(600)})
			return e
		}, "PATCH", "/v1/store/domains/d_1/dns/r_1", "", `{"content":"v=spf1 -all","ttl":600}`},
		{"CheckDNS", func() error { _, _, e := c.Store.CheckDNS(ctx, "d_1"); return e }, "POST", "/v1/store/domains/d_1/dns/check", "", ""},
		{"MicrosoftConsent", func() error {
			_, _, e := c.Store.MicrosoftConsent(ctx, "d_1", &StoreConsentParams{OrderID: "o_1"})
			return e
		}, "POST", "/v1/store/domains/d_1/microsoft-consent", "", `{"order_id":"o_1"}`},
		{"MicrosoftConsent nil", func() error { _, _, e := c.Store.MicrosoftConsent(ctx, "d_1", nil); return e }, "POST", "/v1/store/domains/d_1/microsoft-consent", "", ""},
		{"ListMailboxes", func() error {
			_, e := c.Store.ListMailboxes(ctx, &ListOptions{Cursor: "cur_2"})
			return e
		}, "GET", "/v1/store/mailboxes", "cursor=cur_2", ""},
		{"UpdateMailbox", func() error {
			_, _, e := c.Store.UpdateMailbox(ctx, "m_1", &StoreMailboxUpdateParams{FirstName: String("Ada")})
			return e
		}, "PATCH", "/v1/store/mailboxes/m_1", "", `{"first_name":"Ada"}`},
		{"CopyProfilePhoto", func() error { _, _, e := c.Store.CopyProfilePhoto(ctx); return e }, "POST", "/v1/store/photos", "", `{"from":"profile"}`},
		{"SetMailboxPhoto", func() error { _, _, e := c.Store.SetMailboxPhoto(ctx, "m_1", "ph_1"); return e }, "PUT", "/v1/store/mailboxes/m_1/photo", "", `{"photo_id":"ph_1"}`},
		{"CancelMailbox", func() error { _, _, e := c.Store.CancelMailbox(ctx, "m_1"); return e }, "DELETE", "/v1/store/mailboxes/m_1", "", ""},
		{"ResumeMailbox", func() error { _, _, e := c.Store.ResumeMailbox(ctx, "m_1"); return e }, "POST", "/v1/store/mailboxes/m_1/resume", "", ""},
		{"SignIn", func() error { _, _, e := c.Store.SignIn(ctx, "m_1"); return e }, "POST", "/v1/store/mailboxes/m_1/sign-in", "", ""},
		{"Agency.Store", func() error { _, _, e := c.Agency.Store(ctx, "ag_1"); return e }, "GET", "/v1/agency/ag_1/store", "", ""},
	})
}

func TestStoreNoContent(t *testing.T) {
	var got recordedRequest
	c := emailFixtureClient(t, http.StatusNoContent, ``, &got)
	ctx := context.Background()

	resp, err := c.Store.RemoveDomain(ctx, "d_1")
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("RemoveDomain: %v %v", resp, err)
	}
	checkRequest(t, got, "DELETE", "/v1/store/domains/d_1", "", "")

	resp, err = c.Store.DeleteDNSRecord(ctx, "d_1", "r_1")
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DeleteDNSRecord: %v %v", resp, err)
	}
	checkRequest(t, got, "DELETE", "/v1/store/domains/d_1/dns/r_1", "", "")
}

func TestStorePhotoUpload(t *testing.T) {
	var field, filename, ctype string
	var content []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/store/photos" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		ctype = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
		}
		for name, files := range r.MultipartForm.File {
			field, filename = name, files[0].Filename
			f, _ := files[0].Open()
			content, _ = io.ReadAll(f)
			_ = f.Close()
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"6b0e2f0a-1c1f-4b55-9d2a-3f9b7a1e0c11","created_at":"2026-11-20T10:00:00Z"}`))
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatal(err)
	}
	p, resp, err := c.Store.UploadPhoto(context.Background(), &FileUpload{Filename: "ada.png", Content: strings.NewReader("PNGDATA"), ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ctype, "multipart/form-data") || field != "file" || filename != "ada.png" || string(content) != "PNGDATA" {
		t.Errorf("upload ctype=%q field=%q filename=%q content=%q", ctype, field, filename, content)
	}
	if resp.StatusCode != http.StatusCreated || p.ID != "6b0e2f0a-1c1f-4b55-9d2a-3f9b7a1e0c11" || p.CreatedAt.IsZero() {
		t.Errorf("photo = %+v", p)
	}
}

func TestStoreGetPhoto(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/v1/store/photos/ph_1" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte{0xff, 0xd8, 0xff, 0xe0})
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := c.Store.GetPhoto(context.Background(), "ph_1", &buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf.Bytes(), []byte{0xff, 0xd8, 0xff, 0xe0}) {
		t.Errorf("photo bytes = %x", buf.Bytes())
	}
}

func TestStoreStatusDecode(t *testing.T) {
	c := emailFixtureClient(t, http.StatusOK, `{
		"enabled": false, "configured": false, "reason": "not_configured", "locked": true, "can_configure": true,
		"configure_url": "https://admin.acme.test/health?tab=integrations",
		"disconnected": {"mailboxes": 3, "domains": 1},
		"metered": true, "currency": "USD", "mailbox_monthly_micros": 3000000, "domain_markup": "at_cost",
		"limits": {"mailboxes_per_domain_max": 5, "mailboxes_per_domain_recommended": 3},
		"providers": ["google", "microsoft"],
		"balance": {"balance_micros": 12500000, "card": {"brand": "visa", "last4": "4242"}},
		"plan_active": true, "registrant": null,
		"registrant_prefill": {"first_name": "Ada", "last_name": "Lovelace", "organization": "Acme", "email": "ada@acme.test",
			"phone": "", "address1": "", "address2": "", "city": "", "state": "", "postal_code": "", "country": "GB"},
		"stripe_publishable_key": "pk_test_1", "min_card_charge_micros": 10000000, "billing_day": 14,
		"next_bundle": {"charge_on": "2026-12-14T00:00:00Z", "mailboxes": 3, "amount_micros": 9000000},
		"default_forwarding_url": "https://acme.test", "photos": true, "some_new_field": 1
	}`, nil)
	st, _, err := c.Store.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Enabled || st.Reason != StoreReasonNotConfigured || !st.Locked || st.Disconnected == nil || st.Disconnected.Mailboxes != 3 {
		t.Errorf("status = %+v", st)
	}
	if st.DomainMarkup != StoreDomainAtCost || st.Limits.MailboxesPerDomainMax != 5 || len(st.Providers) != 2 || st.Providers[1] != StoreProviderMicrosoft {
		t.Errorf("catalog = %+v", st)
	}
	if st.Balance == nil || st.Balance.Card == nil || st.Balance.Card.Last4 != "4242" || st.Registrant != nil || st.RegistrantPrefill == nil || st.RegistrantPrefill.Country != "GB" {
		t.Errorf("balance/registrant = %+v %+v %+v", st.Balance, st.Registrant, st.RegistrantPrefill)
	}
	if st.BillingDay == nil || *st.BillingDay != 14 || st.NextBundle == nil || st.NextBundle.AmountMicros != 9000000 || !st.Photos {
		t.Errorf("billing = %+v %+v", st.BillingDay, st.NextBundle)
	}
}

func TestStoreSearchDecode(t *testing.T) {
	c := emailFixtureClient(t, http.StatusOK, `{"query": "acme", "data": [
		{"domain": "getacme.ai", "display_name": "getacme.ai", "tld": "ai", "available": true, "exact": true, "years": 2,
			"price_micros": 160000000, "renew_micros": 80000000, "reason": null},
		{"domain": "acme.com", "display_name": "acme.com", "tld": "com", "available": false, "exact": true, "years": 1,
			"price_micros": 0, "renew_micros": 0, "reason": "registered"}
	]}`, nil)
	res, _, err := c.Store.SearchDomains(context.Background(), &StoreSearchParams{Query: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Query != "acme" || len(res.Data) != 2 || res.Data[0].Years != 2 || res.Data[0].Reason != nil ||
		res.Data[1].Reason == nil || *res.Data[1].Reason != StoreOfferRegistered {
		t.Errorf("search = %+v", res)
	}
}

func TestStoreQuoteDecode(t *testing.T) {
	c := emailFixtureClient(t, http.StatusCreated, `{
		"id": "2c0c7a0e-4c59-4a8e-9a3e-0b8f2e4b1d77", "expires_at": "2026-11-20T10:15:00Z",
		"lines": [
			{"type": "domain", "domain": "getacme.com", "years": 1, "amount_micros": 11170000, "renew_micros": 11170000},
			{"type": "mailboxes", "domain": "getacme.com", "provider": "google", "count": 2, "amount_micros": 6000000,
				"monthly_micros": 3000000, "admin_email": "ada@getacme.com"},
			{"type": "mailboxes", "domain": "acme.io", "provider": "microsoft", "count": 1, "amount_micros": 3000000,
				"monthly_micros": 3000000, "nameservers_change": true}
		],
		"due_now_micros": 20170000, "recurring_monthly_micros": 9000000, "yearly_domain_renewal_micros": 11170000,
		"payment": {"balance_micros": 5000000, "from_balance_micros": 5000000, "card_charge_micros": 25000000,
			"balance_after_micros": 9830000, "card": null, "needs_payment_method": true,
			"auto_reload": {"enabled": true, "threshold_micros": 9000000, "amount_micros": 25000000, "store_managed": true}}
	}`, nil)
	q, resp, err := c.Store.CreateQuote(context.Background(), &StoreQuoteParams{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated || len(q.Lines) != 3 || q.Lines[0].Type != StoreItemDomain || q.Lines[0].AdminEmail != nil {
		t.Fatalf("quote = %+v", q)
	}
	if q.Lines[1].AdminEmail == nil || *q.Lines[1].AdminEmail != "ada@getacme.com" || !q.Lines[2].NameserversChange || q.Lines[2].Provider != StoreProviderMicrosoft {
		t.Errorf("lines = %+v", q.Lines)
	}
	p := q.Payment
	if p.Card != nil || !p.NeedsPaymentMethod || p.AutoReload == nil || !p.AutoReload.StoreManaged || p.BalanceAfterMicros != 9830000 {
		t.Errorf("payment = %+v", p)
	}
}

const storeDomainFixture = `{
	"id": "8c1d2e3f-0000-4000-8000-000000000001", "name": "acme.io", "status": "connecting", "source": "connected",
	"provider": "", "renewable": false, "expires_at": null, "auto_renew": false, "renew_micros": 0,
	"forwarding_url": "", "forwarding_status": "none",
	"mail_auth": {"mx": "pending", "spf": "pending", "dkim": "pending", "dmarc": "pending"},
	"mailbox_count": 0, "nameservers": ["ns1.graphkit.test", "ns2.graphkit.test"],
	"connection": {"method": "nameservers", "status": "pending", "expires_at": "2026-11-22T10:00:00Z", "checked_at": null,
		"current_nameservers": ["ns1.oldhost.test"]},
	"dns_conflicts": [
		{"record_id": "8c1d2e3f-0000-4000-8000-0000000000aa", "type": "MX", "name": "@", "content": "mx.oldhost.test"},
		{"record_id": null, "type": "TXT", "name": "@", "content": "v=spf1 include:old -all"}
	],
	"owner_verification": {"status": "pending", "email": "ada@acme.test", "deadline": "2026-12-05T00:00:00Z"},
	"transfer": null, "tracking_host": "", "created_at": "2026-11-20T10:00:00Z",
	"dns": [
		{"id": "8c1d2e3f-0000-4000-8000-0000000000bb", "type": "MX", "name": "@", "content": "smtp.google.com", "ttl": 3600,
			"priority": 1, "managed": true, "purpose": "mx"},
		{"id": "8c1d2e3f-0000-4000-8000-0000000000cc", "type": "TXT", "name": "_gk", "content": "gk=abc", "ttl": 300,
			"priority": null, "managed": true, "purpose": "ownership"}
	]
}`

func TestStoreDomainDecode(t *testing.T) {
	c := emailFixtureClient(t, http.StatusCreated, storeDomainFixture, nil)
	d, _, err := c.Store.ConnectDomain(context.Background(), &StoreConnectDomainParams{Domain: "acme.io"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != StoreDomainConnecting || d.Source != StoreSourceConnected || d.Renewable || d.ExpiresAt != nil || d.ForwardingStatus != StoreForwardingNone {
		t.Errorf("domain = %+v", d)
	}
	if d.Connection == nil || d.Connection.Method != StoreConnectNameservers || d.Connection.Status != StoreConnectionPending ||
		d.Connection.CheckedAt != nil || len(d.Connection.CurrentNameservers) != 1 {
		t.Errorf("connection = %+v", d.Connection)
	}
	if len(d.DNSConflicts) != 2 || d.DNSConflicts[0].RecordID == nil || d.DNSConflicts[1].RecordID != nil {
		t.Errorf("conflicts = %+v", d.DNSConflicts)
	}
	if d.OwnerVerification == nil || d.OwnerVerification.Deadline == nil || d.Transfer != nil || d.MailAuth.DKIM != "pending" {
		t.Errorf("verification = %+v transfer = %+v", d.OwnerVerification, d.Transfer)
	}
	if len(d.DNS) != 2 || d.DNS[0].Priority == nil || *d.DNS[0].Priority != 1 || d.DNS[1].Priority != nil || d.DNS[1].Purpose != StoreDNSOwnership || d.DNS[0].Status != "" {
		t.Errorf("dns = %+v", d.DNS)
	}
}

//nolint:misspell // wire value: the API sends "cancelled_at"
const storeMailboxFixture = `{
	"id": "5d6e7f80-0000-4000-8000-000000000002", "email": "ada@getacme.com", "domain_id": "8c1d2e3f-0000-4000-8000-000000000001",
	"provider": "microsoft", "status": "active", "first_name": "Ada", "last_name": "Lovelace", "is_admin": true,
	"paused": false, "monthly_micros": 3000000, "paid_through": "2026-12-14T00:00:00Z",
	"cancels_at": "2026-12-14T00:00:00Z", "cancelled_at": null, "email_account_id": null, "connect_state": "waiting",
	"action_required": "microsoft_consent", "action_reason": "mfa_required", "photo": "pending",
	"created_at": "2026-11-14T10:00:00Z"
}`

func TestStoreOrderAndMailboxDecode(t *testing.T) {
	c := emailFixtureClient(t, http.StatusOK, `{
		"id": "1a2b3c4d-0000-4000-8000-000000000003", "status": "processing", "created_at": "2026-11-20T10:00:00Z",
		"completed_at": null, "due_now_micros": 20170000, "charged_micros": 25000000, "refunded_micros": 0,
		"steps": [
			{"key": "register", "status": "done", "at": "2026-11-20T10:01:00Z"},
			{"key": "dns", "status": "running", "at": null},
			{"key": "mail_auth", "status": "pending", "at": null},
			{"key": "mailboxes", "status": "pending", "at": null},
			{"key": "delegation", "status": "pending", "at": null},
			{"key": "ready", "status": "pending", "at": null}
		],
		"domains": [`+storeDomainFixture+`],
		"mailboxes": [`+storeMailboxFixture+`]
	}`, nil)
	o, _, err := c.Store.GetOrder(context.Background(), "o_1")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != StoreOrderProcessing || o.CompletedAt != nil || len(o.Steps) != 6 || o.Steps[0].Key != StoreStepRegister ||
		o.Steps[0].At == nil || o.Steps[1].Status != StoreStepRunning || o.Steps[5].Key != StoreStepReady {
		t.Errorf("order = %+v", o)
	}
	if len(o.Domains) != 1 || len(o.Mailboxes) != 1 {
		t.Fatalf("order contents = %+v", o)
	}
	m := o.Mailboxes[0]
	if m.Provider != StoreProviderMicrosoft || m.Status != StoreMailboxActive || !m.IsAdmin || m.CancelsAt == nil || m.CancelledAt != nil ||
		m.EmailAccountID != nil || m.ConnectState != StoreConnectWaiting || m.Photo != StorePhotoPending {
		t.Errorf("mailbox = %+v", m)
	}
	if m.ActionRequired == nil || *m.ActionRequired != StoreActionMicrosoftConsent || m.ActionReason == nil || *m.ActionReason != StoreActionMFARequired {
		t.Errorf("action = %v %v", m.ActionRequired, m.ActionReason)
	}
}

func TestStoreListDecode(t *testing.T) {
	var got recordedRequest
	c := emailFixtureClient(t, http.StatusOK, `{"data": [`+storeMailboxFixture+`], "pagination": {"next_cursor": "nx", "has_more": true}}`, &got)
	ctx := context.Background()
	page, err := c.Store.ListMailboxes(ctx, &ListOptions{Limit: 25})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 1 || page.Data[0].Email != "ada@getacme.com" || !page.HasMore() {
		t.Errorf("page = %+v", page)
	}
	if _, err := page.Next(ctx); err != nil || got.rawQuery != "cursor=nx&limit=25" {
		t.Errorf("Next: %v ?%s", err, got.rawQuery)
	}
}

func TestStoreDNSCheckDecode(t *testing.T) {
	c := emailFixtureClient(t, http.StatusOK, `{"nameservers": ["ns1.graphkit.test"], "records": [
		{"id": "r1", "type": "MX", "name": "@", "content": "smtp.google.com", "ttl": 3600, "priority": 1, "managed": true, "purpose": "mx", "status": "ok"},
		{"id": "r2", "type": "TXT", "name": "@", "content": "v=spf1", "ttl": 3600, "priority": null, "managed": false, "purpose": "brand_new", "status": "missing"}
	]}`, nil)
	res, _, err := c.Store.CheckDNS(context.Background(), "d_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Nameservers) != 1 || len(res.Records) != 2 || res.Records[0].Status != StoreDNSCheckOK ||
		res.Records[1].Status != StoreDNSCheckMissing || res.Records[1].Purpose != "brand_new" {
		t.Errorf("check = %+v", res)
	}
}

func TestStoreSecretsDecode(t *testing.T) {
	var got recordedRequest
	c := emailFixtureClient(t, http.StatusOK, `{"auth_code": "Xy7#aB9!"}`, &got)
	code, _, err := c.Store.TransferOut(context.Background(), "d_1")
	if err != nil || code.AuthCode != "Xy7#aB9!" {
		t.Fatalf("TransferOut: %+v %v", code, err)
	}

	c = emailFixtureClient(t, http.StatusOK, `{"email": "ada@getacme.com", "username": "ada@getacme.com", "password": "s3cret",
		"login_url": "https://accounts.google.com", "admin_console_url": "https://admin.google.com"}`, nil)
	in, _, err := c.Store.SignIn(context.Background(), "m_1")
	if err != nil || in.Password != "s3cret" || in.AdminConsoleURL == nil || *in.AdminConsoleURL != "https://admin.google.com" {
		t.Fatalf("SignIn: %+v %v", in, err)
	}

	c = emailFixtureClient(t, http.StatusOK, `{"email": "bob@getacme.com", "username": "bob", "password": "p", "login_url": "https://login.microsoftonline.com", "admin_console_url": null}`, nil)
	in, _, err = c.Store.SignIn(context.Background(), "m_2")
	if err != nil || in.AdminConsoleURL != nil {
		t.Fatalf("SignIn non-admin: %+v %v", in, err)
	}
}

func TestStoreConsentDecode(t *testing.T) {
	c := emailFixtureClient(t, http.StatusOK, `{"url": "https://app.acme.test/consent/abc", "expires_at": "2026-11-20T11:00:00Z", "consented": false}`, nil)
	l, _, err := c.Store.MicrosoftConsent(context.Background(), "d_1", nil)
	if err != nil || l.URL == nil || l.ExpiresAt == nil || l.Consented {
		t.Fatalf("consent = %+v %v", l, err)
	}
	c = emailFixtureClient(t, http.StatusOK, `{"url": null, "consented": true}`, nil)
	l, _, err = c.Store.MicrosoftConsent(context.Background(), "d_1", nil)
	if err != nil || l.URL != nil || l.ExpiresAt != nil || !l.Consented {
		t.Fatalf("consented = %+v %v", l, err)
	}
}

func TestAgencyStoreDecode(t *testing.T) {
	var got recordedRequest
	c := emailFixtureClient(t, http.StatusOK, `{
		"enabled": true,
		"clients": [{
			"organization_id": "2f6b0000-0000-4000-8000-000000000004", "name": "Acme", "avatar_url": null,
			"domains": 3, "mailboxes_live": 9, "mailboxes_connected": 8, "monthly_spend_micros": 27000000,
			"renewals_30d": 1,
			"next_renewal": {"domain_id": "8c1d0000-0000-4000-8000-000000000005", "domain": "getacme.com", "expires_at": "2026-12-04T00:00:00Z", "auto_renew": true},
			"orders_open": 0, "attention": [{"kind": "connect_failed", "count": 1}, {"kind": "something_new", "count": 2}]
		}],
		"renewals": [{"organization_id": "2f6b0000-0000-4000-8000-000000000004", "organization_name": "Acme",
			"domain_id": "8c1d0000-0000-4000-8000-000000000005", "domain": "getacme.com", "expires_at": "2026-12-04T00:00:00Z",
			"auto_renew": true, "renew_micros": 11170000}],
		"totals": {"domains": 3, "mailboxes_live": 9, "monthly_spend_micros": 27000000, "renewals_30d": 1, "attention": 1}
	}`, &got)
	out, _, err := c.Agency.Store(context.Background(), "ag_1")
	if err != nil {
		t.Fatal(err)
	}
	checkRequest(t, got, "GET", "/v1/agency/ag_1/store", "", "")
	if !out.Enabled || len(out.Clients) != 1 || len(out.Renewals) != 1 || out.Totals.MonthlySpendMicros != 27000000 {
		t.Fatalf("store = %+v", out)
	}
	cl := out.Clients[0]
	if cl.AvatarURL != nil || cl.NextRenewal == nil || !cl.NextRenewal.AutoRenew || len(cl.Attention) != 2 ||
		cl.Attention[0].Kind != AgencyStoreConnectFailed || cl.Attention[1].Kind != "something_new" {
		t.Errorf("client = %+v", cl)
	}
	if out.Renewals[0].RenewMicros != 11170000 || out.Renewals[0].ExpiresAt == nil {
		t.Errorf("renewal = %+v", out.Renewals[0])
	}
}
