package warmbly

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const (
	agID  = "6a1e0000-0000-4000-8000-0000000000a1"
	orgID = "9b1e0000-0000-4000-8000-000000000001"
)

func TestAgencyRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()
	base := "/v1/agency/" + agID

	cases := []struct {
		name       string
		call       func() error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string
	}{
		// Agencies.
		{"List", func() error { _, _, e := c.Agency.List(ctx); return e }, "GET", "/v1/agency", "", ""},
		{"Plans", func() error { _, _, e := c.Agency.Plans(ctx); return e }, "GET", "/v1/agency/plans", "", ""},
		{"Create", func() error {
			_, _, e := c.Agency.Create(ctx, &AgencyCreateParams{Name: "Northwind"}, WithIdempotencyKey("k1"))
			return e
		}, "POST", "/v1/agency", "", `{"name":"Northwind"}`},
		{"Get", func() error { _, _, e := c.Agency.Get(ctx, agID); return e }, "GET", base, "", ""},
		{"Update", func() error {
			_, _, e := c.Agency.Update(ctx, agID, &AgencyUpdateParams{Name: "Northwind Ltd"})
			return e
		}, "PATCH", base, "", `{"name":"Northwind Ltd"}`},

		// Staff.
		{"Members", func() error { _, _, e := c.Agency.Members(ctx, agID); return e }, "GET", base + "/members", "", ""},
		{"AddMember", func() error {
			_, _, e := c.Agency.AddMember(ctx, agID, &AgencyMemberAddParams{Email: "sam@example.com", Role: AgencyRoleAdmin})
			return e
		}, "POST", base + "/members", "", `{"email":"sam@example.com","role":"admin"}`},
		{"UpdateMember", func() error {
			_, _, e := c.Agency.UpdateMember(ctx, agID, "u_1", AgencyRoleMember)
			return e
		}, "PATCH", base + "/members/u_1", "", `{"role":"member"}`},
		{"RemoveMember", func() error { _, e := c.Agency.RemoveMember(ctx, agID, "u_1"); return e }, "DELETE", base + "/members/u_1", "", ""},
		{"CancelInvitation", func() error { _, e := c.Agency.CancelInvitation(ctx, agID, "inv_1"); return e }, "DELETE", base + "/invitations/inv_1", "", ""},

		// Clients.
		{"Clients", func() error { _, _, e := c.Agency.Clients(ctx, agID); return e }, "GET", base + "/clients", "", ""},
		{"CreateClient", func() error {
			_, _, e := c.Agency.CreateClient(ctx, agID, &AgencyClientCreateParams{Name: "Acme", Timezone: "Europe/Berlin"})
			return e
		}, "POST", base + "/clients", "", `{"name":"Acme","timezone":"Europe/Berlin"}`},
		{"AttachClient", func() error { _, _, e := c.Agency.AttachClient(ctx, agID, orgID); return e }, "POST", base + "/clients/attach", "", `{"organization_id":"` + orgID + `"}`},
		{"UpdateClient sets", func() error {
			_, _, e := c.Agency.UpdateClient(ctx, agID, orgID, &AgencyClientUpdateParams{MonthlySendLimit: Int(20000), MonthlyAILimitUSD: Float64(12.5)})
			return e
		}, "PATCH", base + "/clients/" + orgID, "", `{"monthly_ai_limit_usd":12.5,"monthly_send_limit":20000}`},
		{"UpdateClient clears", func() error {
			_, _, e := c.Agency.UpdateClient(ctx, agID, orgID, &AgencyClientUpdateParams{ClearMonthlySendLimit: true})
			return e
		}, "PATCH", base + "/clients/" + orgID, "", `{"monthly_send_limit":null}`},
		{"DetachClient", func() error { _, e := c.Agency.DetachClient(ctx, agID, orgID); return e }, "DELETE", base + "/clients/" + orgID, "", ""},

		// Usage.
		{"Usage", func() error { _, _, e := c.Agency.Usage(ctx, agID); return e }, "GET", base + "/usage", "", ""},
		{"AIBalance", func() error { _, _, e := c.Agency.AIBalance(ctx, agID); return e }, "GET", base + "/ai-balance", "", ""},
		{"TransferAIBalance", func() error {
			_, _, e := c.Agency.TransferAIBalance(ctx, agID, orgID, &AgencyAITransferParams{AmountUSD: 25})
			return e
		}, "POST", base + "/clients/" + orgID + "/ai-balance", "", `{"amount_usd":25}`},

		// Home.
		{"Overview", func() error { _, _, e := c.Agency.Overview(ctx, agID); return e }, "GET", base + "/overview", "", ""},
		{"Mailboxes", func() error {
			_, e := c.Agency.Mailboxes(ctx, agID, &AgencyMailboxListParams{
				ListOptions: ListOptions{Limit: 100},
				Clients:     []string{"o1", "o2"},
				Query:       "sales",
				Attention:   true,
			})
			return e
		}, "GET", base + "/mailboxes", "attention=true&client=o1%2Co2&limit=100&q=sales", ""},
		{"Mailboxes nil", func() error { _, e := c.Agency.Mailboxes(ctx, agID, nil); return e }, "GET", base + "/mailboxes", "", ""},
		{"Inbox", func() error {
			_, e := c.Agency.Inbox(ctx, agID, &AgencyInboxListParams{
				ListOptions: ListOptions{Cursor: "cur"},
				Clients:     []string{"o1"},
				Unread:      true,
				NeedsReply:  true,
				Positive:    true,
				Query:       "pricing",
			})
			return e
		}, "GET", base + "/inbox", "client=o1&cursor=cur&needs_reply=true&positive=true&q=pricing&unread=true", ""},
		{"InboxThread", func() error {
			_, e := c.Agency.InboxThread(ctx, agID, orgID, "thr-1", &ListOptions{Cursor: "c2", Limit: 9})
			return e
		}, "GET", base + "/inbox/thread", "cursor=c2&organization_id=" + orgID + "&thread_id=thr-1", ""},
		{"InboxMessage", func() error { _, _, e := c.Agency.InboxMessage(ctx, agID, orgID, "msg_1"); return e }, "GET", base + "/inbox/messages/msg_1", "organization_id=" + orgID, ""},

		// Migrations.
		{"MigrationProviders", func() error { _, _, e := c.Agency.MigrationProviders(ctx, agID); return e }, "GET", base + "/migrations/providers", "", ""},
		{"Migrations", func() error { _, e := c.Agency.Migrations(ctx, agID, &ListOptions{Limit: 10}); return e }, "GET", base + "/migrations", "limit=10", ""},
		{"CreateMigration", func() error {
			_, _, e := c.Agency.CreateMigration(ctx, agID, &AgencyMigrationCreateParams{
				Provider:    "smartlead",
				Credentials: []map[string]string{{"api_key": "sk"}},
			})
			return e
		}, "POST", base + "/migrations", "", `{"provider":"smartlead","credentials":[{"api_key":"sk"}]}`},
		{"GetMigration", func() error { _, _, e := c.Agency.GetMigration(ctx, agID, "mig_1"); return e }, "GET", base + "/migrations/mig_1", "", ""},
		{"StartMigration", func() error {
			_, _, e := c.Agency.StartMigration(ctx, agID, "mig_1", &AgencyMigrationStartParams{
				Options: AgencyMigrationOptions{Kinds: []string{AgencyMigrationKindCampaigns}, InFlight: AgencyMigrationRestart},
				Targets: []AgencyMigrationTargetChoice{{WorkspaceID: "w1", CreateName: "Acme"}, {WorkspaceID: "w2", Key: 1, OrganizationID: String(orgID)}},
			})
			return e
		}, "POST", base + "/migrations/mig_1/start", "", `{"options":{"kinds":["campaigns"],"in_flight":"restart","skip_finished":false,"warmup":false},"targets":[{"workspace_id":"w1","key":0,"create_name":"Acme"},{"workspace_id":"w2","key":1,"organization_id":"` + orgID + `"}]}`},
		{"CancelMigration", func() error { _, _, e := c.Agency.CancelMigration(ctx, agID, "mig_1"); return e }, "POST", base + "/migrations/mig_1/cancel", "", ""},
		{"LinkMigrationSenders", func() error { _, _, e := c.Agency.LinkMigrationSenders(ctx, agID, "mig_1"); return e }, "POST", base + "/migrations/mig_1/link-senders", "", ""},
		{"DismissMigration", func() error { _, e := c.Agency.DismissMigration(ctx, agID, "mig_1"); return e }, "DELETE", base + "/migrations/mig_1", "", ""},

		// Vendors.
		{"Vendors", func() error { _, _, e := c.Agency.Vendors(ctx, agID); return e }, "GET", base + "/vendors", "", ""},
		{"CreateVendor", func() error {
			_, _, e := c.Agency.CreateVendor(ctx, agID, &AgencyVendorParams{Vendor: "zapmail", Label: "Main", Fields: map[string]string{"api_key": "z"}})
			return e
		}, "POST", base + "/vendors", "", `{"vendor":"zapmail","label":"Main","fields":{"api_key":"z"}}`},
		{"UpdateVendor", func() error {
			_, _, e := c.Agency.UpdateVendor(ctx, agID, "v_1", &AgencyVendorParams{Label: "Renamed"})
			return e
		}, "PATCH", base + "/vendors/v_1", "", `{"label":"Renamed"}`},
		{"DeleteVendor", func() error { _, e := c.Agency.DeleteVendor(ctx, agID, "v_1"); return e }, "DELETE", base + "/vendors/v_1", "", ""},
		{"UseVendor", func() error { _, _, e := c.Agency.UseVendor(ctx, agID, "v_1", orgID); return e }, "PUT", base + "/vendors/v_1/clients/" + orgID, "", ""},

		// Billing.
		{"Checkout", func() error {
			_, _, e := c.Agency.Checkout(ctx, agID, &AgencyCheckoutParams{PlanID: "p_1", Interval: AgencyIntervalYear})
			return e
		}, "POST", base + "/billing/checkout", "", `{"plan_id":"p_1","interval":"year"}`},
		{"PreviewPlanChange", func() error { _, _, e := c.Agency.PreviewPlanChange(ctx, agID, "p_2", AgencyIntervalMonth); return e }, "GET", base + "/billing/change/preview", "interval=month&plan_id=p_2", ""},
		{"ChangePlan", func() error {
			_, _, e := c.Agency.ChangePlan(ctx, agID, &AgencyPlanChangeParams{PlanID: "p_2", ProrationDate: 1789000000, FromPriceID: "price_a"})
			return e
		}, "POST", base + "/billing/change", "", `{"plan_id":"p_2","proration_date":1789000000,"from_price_id":"price_a"}`},
		{"AutoTopup", func() error { _, _, e := c.Agency.AutoTopup(ctx, agID); return e }, "GET", base + "/billing/auto-topup", "", ""},
		{"SetAutoTopup", func() error {
			_, _, e := c.Agency.SetAutoTopup(ctx, agID, &AgencyAutoTopupParams{Enabled: false})
			return e
		}, "PUT", base + "/billing/auto-topup", "", `{"enabled":false,"threshold_sends":0,"max_packs":0}`},
		{"AIAutoTopup", func() error { _, _, e := c.Agency.AIAutoTopup(ctx, agID); return e }, "GET", base + "/billing/ai-auto-topup", "", ""},
		{"SetAIAutoTopup", func() error {
			_, _, e := c.Agency.SetAIAutoTopup(ctx, agID, &AgencyAIAutoTopupParams{Enabled: true, ThresholdUSD: 5, AmountUSD: 50, LimitUSD: 200})
			return e
		}, "PUT", base + "/billing/ai-auto-topup", "", `{"enabled":true,"threshold_usd":5,"amount_usd":50,"limit_usd":200}`},
		{"Topup", func() error { _, _, e := c.Agency.Topup(ctx, agID, &AgencyTopupParams{}); return e }, "POST", base + "/billing/topup", "", `{}`},
		{"AITopup", func() error {
			_, _, e := c.Agency.AITopup(ctx, agID, &AgencyAITopupParams{AmountUSD: 100, SuccessURL: "https://x.test/ok"})
			return e
		}, "POST", base + "/billing/ai-topup", "", `{"amount_usd":100,"success_url":"https://x.test/ok"}`},
		{"BillingPortal", func() error { _, _, e := c.Agency.BillingPortal(ctx, agID, ""); return e }, "POST", base + "/billing/portal", "", `{}`},

		// Brand.
		{"Brand", func() error { _, _, e := c.Agency.Brand(ctx, agID); return e }, "GET", base + "/brand", "", ""},
		{"UpdateBrand", func() error {
			signIn := AgencyPortalSignInWarmbly
			_, _, e := c.Agency.UpdateBrand(ctx, agID, &AgencyBrandParams{
				DisplayName:  "Northwind",
				AccentColor:  "#0284c7",
				PortalSignIn: &signIn,
				Theme:        &AgencyPortalTheme{Font: AgencyPortalFontSerif},
			})
			return e
		}, "PUT", base + "/brand", "", `{"display_name":"Northwind","accent_color":"#0284c7","website_url":"","hide_powered_by":false,"portal_sign_in":"warmbly","theme":{"font":"serif"}}`},
		{"DeleteBrandLogo", func() error { _, _, e := c.Agency.DeleteBrandLogo(ctx, agID); return e }, "DELETE", base + "/brand/logo", "", ""},
		{"DeleteBrandIcon", func() error { _, _, e := c.Agency.DeleteBrandIcon(ctx, agID); return e }, "DELETE", base + "/brand/icon", "", ""},
		{"DeleteBrandSignInImage", func() error { _, _, e := c.Agency.DeleteBrandSignInImage(ctx, agID); return e }, "DELETE", base + "/brand/sign-in-image", "", ""},
		{"SetPortalDomain", func() error { _, _, e := c.Agency.SetPortalDomain(ctx, agID, "portal.example.com"); return e }, "PUT", base + "/brand/domain", "", `{"domain":"portal.example.com"}`},
		{"SetPortalDomain clears", func() error { _, _, e := c.Agency.SetPortalDomain(ctx, agID, ""); return e }, "PUT", base + "/brand/domain", "", `{"domain":""}`},
		{"VerifyPortalDomain", func() error { _, _, e := c.Agency.VerifyPortalDomain(ctx, agID); return e }, "POST", base + "/brand/domain/verify", "", ""},

		// API keys.
		{"APIKeys", func() error { _, e := c.Agency.APIKeys(ctx, agID, &ListOptions{Limit: 25, Cursor: "k"}); return e }, "GET", base + "/api-keys", "cursor=k&limit=25", ""},
		{"APIKeyPermissions", func() error { _, _, e := c.Agency.APIKeyPermissions(ctx, agID); return e }, "GET", base + "/api-keys/permissions", "", ""},
		{"RevokeAPIKey", func() error { _, e := c.Agency.RevokeAPIKey(ctx, agID, "key_1"); return e }, "DELETE", base + "/api-keys/key_1", "", ""},
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
			if got.rawQuery != tc.wantQuery {
				t.Errorf("query = %q, want %q", got.rawQuery, tc.wantQuery)
			}
			if body := strings.TrimSpace(got.body); body != tc.wantBody {
				t.Errorf("body = %s, want %s", body, tc.wantBody)
			}
		})
	}
}

// The shared routing envelope carries "permissions": [], which a key's
// bitmask cannot decode, so the key writes answer with an empty object.
func TestAgencyAPIKeyWrites(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{}`)
	ctx := context.Background()
	base := "/v1/agency/" + agID

	if _, _, err := c.Agency.CreateAPIKey(ctx, agID, &AgencyAPIKeyCreateParams{Name: "CI", Permissions: PermAgencyRead | PermReadCampaigns, RateLimitPerMinute: Int(120)}); err != nil {
		t.Fatal(err)
	}
	if got.method != "POST" || got.path != base+"/api-keys" || strings.TrimSpace(got.body) != `{"name":"CI","permissions":67108866,"rate_limit_per_minute":120}` {
		t.Errorf("create: %s %s %s", got.method, got.path, got.body)
	}
	if _, _, err := c.Agency.UpdateAPIKey(ctx, agID, "key_1", &AgencyAPIKeyUpdateParams{AllowedIPs: &[]string{}}); err != nil {
		t.Fatal(err)
	}
	if got.method != "PATCH" || got.path != base+"/api-keys/key_1" || strings.TrimSpace(got.body) != `{"allowed_ips":[]}` {
		t.Errorf("update: %s %s %s", got.method, got.path, got.body)
	}
	if _, _, err := c.Agency.UpdateAPIKey(ctx, agID, "key_1", &AgencyAPIKeyUpdateParams{Name: String("Renamed")}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got.body) != `{"name":"Renamed"}` {
		t.Errorf("rename body = %s", got.body)
	}
}

func TestAgencyIdempotencyKeyForwarded(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	_, _, err := c.Agency.TransferAIBalance(context.Background(), agID, orgID, &AgencyAITransferParams{AmountUSD: 10}, WithIdempotencyKey("xfer-1"))
	if err != nil {
		t.Fatal(err)
	}
	if got.idempotencyKey != "xfer-1" {
		t.Errorf("Idempotency-Key = %q", got.idempotencyKey)
	}
}

func TestAgencyBrandUploads(t *testing.T) {
	var (
		mu    sync.Mutex
		paths []string
		field string
		ctype string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		ctype = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(1 << 20); err == nil {
			for k := range r.MultipartForm.File {
				field = k
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"brand":{"display_name":"Northwind","logo_url":"https://cdn.test/logo.png","accent_color":"","website_url":"","hide_powered_by":false,"updated_at":"2026-11-01T10:00:00Z"},"full_white_label":true,"can_manage":true,"portal_domain":{"forms_domain":"","forms_domain_verified":false,"forms_domain_verified_at":null,"cname_target":"portal.warmbly.test","status":"unset","message":"No portal domain is set.","forms_host_unresolvable":false},"portal_base_url":"https://portal.warmbly.test","portal_sign_in":"portal","icon_url":"","sign_in_image_url":"","theme":{}}`)
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	file := func() *FileUpload {
		return &FileUpload{Filename: "logo.png", Content: strings.NewReader("\x89PNG"), ContentType: "image/png"}
	}
	b, _, err := c.Agency.UploadBrandLogo(ctx, agID, file())
	if err != nil {
		t.Fatal(err)
	}
	if b.Brand.LogoURL != "https://cdn.test/logo.png" || !b.CanManage || b.PortalSignIn != AgencyPortalSignInPortal {
		t.Errorf("brand = %+v", b)
	}
	if !strings.HasPrefix(ctype, "multipart/form-data") || field != "file" {
		t.Errorf("content type %q, file field %q", ctype, field)
	}
	if _, _, err := c.Agency.UploadBrandIcon(ctx, agID, file()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Agency.UploadBrandSignInImage(ctx, agID, file()); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"POST /v1/agency/" + agID + "/brand/logo",
		"POST /v1/agency/" + agID + "/brand/icon",
		"POST /v1/agency/" + agID + "/brand/sign-in-image",
	}
	if strings.Join(paths, "|") != strings.Join(want, "|") {
		t.Errorf("paths = %v", paths)
	}
}

func TestAgencyWithOrganizationHeader(t *testing.T) {
	var header string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Get("Warmbly-Organization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmagency_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Agency.Clients(context.Background(), agID, WithAgencyOrganization(orgID)); err != nil {
		t.Fatal(err)
	}
	if header != orgID {
		t.Errorf("header = %q", header)
	}
}

func TestAgencyListDecode(t *testing.T) {
	const fixture = `{
	  "data": [{
	    "id": "` + agID + `",
	    "name": "Northwind",
	    "owner_user_id": "4e5f0000-0000-4000-8000-000000000002",
	    "plan_id": "00000000-0000-0000-0000-0000000000a2",
	    "status": "active",
	    "current_period_start": "2026-11-01T00:00:00Z",
	    "current_period_end": "2026-12-01T00:00:00Z",
	    "cancel_at_period_end": false,
	    "created_at": "2026-09-01T08:00:00Z",
	    "updated_at": "2026-11-01T08:00:00Z",
	    "plan": {
	      "id": "00000000-0000-0000-0000-0000000000a2",
	      "name": "Agency Pro",
	      "max_contacts": 0, "daily_emails": 0, "ai_generation": true, "account_limit": 0,
	      "price": 499, "discounted_price": 0, "duration": "month", "savings": 0, "public": true,
	      "dedicated_workers": 0, "monthly_credits": 0, "referral_reward_percent": 100,
	      "monthly_send_pool": 500000,
	      "agency_ai_allowance_usd": 50,
	      "created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z"
	    },
	    "role": "owner",
	    "client_count": 12,
	    "has_billing": true,
	    "granted": {"plan_id": "00000000-0000-0000-0000-0000000000a1"},
	    "billing_interval": "month"
	  }],
	  "billing_enabled": true
	}`
	var got recordedRequest
	c := recordingFixtureClient(t, &got, fixture)
	list, _, err := c.Agency.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !list.BillingEnabled || len(list.Data) != 1 {
		t.Fatalf("list = %+v", list)
	}
	a := list.Data[0]
	if a.Role != AgencyRoleOwner || a.ClientCount != 12 || !a.HasBilling || a.BillingInterval != AgencyIntervalMonth || a.Status != "active" {
		t.Errorf("agency = %+v", a)
	}
	if a.PlanID == nil || a.CurrentPeriodEnd == nil || a.CurrentPeriodEnd.Month() != 12 {
		t.Errorf("plan id / period = %v / %v", a.PlanID, a.CurrentPeriodEnd)
	}
	if a.Plan == nil || a.Plan.Name == nil || *a.Plan.Name != "Agency Pro" || a.Plan.Price != 499 {
		t.Fatalf("plan = %+v", a.Plan)
	}
	if a.Plan.MonthlySendPool == nil || *a.Plan.MonthlySendPool != 500000 || a.Plan.AgencyAIAllowanceUSD == nil || *a.Plan.AgencyAIAllowanceUSD != 50 {
		t.Errorf("agency plan extras = %+v", a.Plan)
	}
	if a.Granted == nil || a.Granted.Until != nil {
		t.Errorf("granted = %+v", a.Granted)
	}
}

func TestAgencyGetDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"agency":{"id":"`+agID+`","name":"Northwind","owner_user_id":"u","status":"","cancel_at_period_end":false,"created_at":"2026-09-01T08:00:00Z","updated_at":"2026-09-01T08:00:00Z","client_count":0,"has_billing":false},"billing_enabled":false}`)
	d, _, err := c.Agency.Get(context.Background(), agID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Agency == nil || d.Agency.Name != "Northwind" || d.Agency.Plan != nil || d.Agency.PlanID != nil || d.BillingEnabled {
		t.Errorf("detail = %+v", d)
	}
}

func TestAgencyMembersAndAddDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{
	  "data": [{"agency_id":"`+agID+`","user_id":"u1","role":"admin","created_at":"2026-09-02T08:00:00Z","email":"a@example.com","first_name":"Ada","last_name":"L"}],
	  "invitations": [{"id":"inv1","agency_id":"`+agID+`","email":"b@example.com","role":"member","invited_by":"u0","expires_at":"2026-11-20T08:00:00Z","created_at":"2026-11-06T08:00:00Z"}]
	}`)
	staff, _, err := c.Agency.Members(context.Background(), agID)
	if err != nil {
		t.Fatal(err)
	}
	if len(staff.Data) != 1 || staff.Data[0].Role != AgencyRoleAdmin || staff.Data[0].FirstName != "Ada" {
		t.Errorf("members = %+v", staff.Data)
	}
	if len(staff.Invitations) != 1 || staff.Invitations[0].InvitedBy == nil || staff.Invitations[0].Role != AgencyRoleMember {
		t.Errorf("invitations = %+v", staff.Invitations)
	}

	c = recordingFixtureClient(t, &got, `{"member":null,"invitation":{"id":"inv2","agency_id":"`+agID+`","email":"c@example.com","role":"member","expires_at":"2026-11-29T08:00:00Z","created_at":"2026-11-15T08:00:00Z"}}`)
	res, _, err := c.Agency.AddMember(context.Background(), agID, &AgencyMemberAddParams{Email: "c@example.com", Role: AgencyRoleMember})
	if err != nil {
		t.Fatal(err)
	}
	if res.Member != nil || res.Invitation == nil || res.Invitation.ID != "inv2" {
		t.Errorf("add result = %+v", res)
	}
}

func TestAgencyClientDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"organization_id":"`+orgID+`","agency_id":"`+agID+`","monthly_send_limit":20000,"added_at":"2026-10-01T08:00:00Z","added_by":"u1","name":"Acme","slug":"acme","owner_user_id":"u0"}`)
	cl, _, err := c.Agency.UpdateClient(context.Background(), agID, orgID, &AgencyClientUpdateParams{MonthlySendLimit: Int(20000), ClearMonthlyAILimit: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got.body) != `{"monthly_ai_limit_usd":null,"monthly_send_limit":20000}` {
		t.Errorf("body = %s", got.body)
	}
	if cl.MonthlySendLimit == nil || *cl.MonthlySendLimit != 20000 || cl.MonthlyAILimitUSD != nil || cl.Slug == nil || cl.AvatarURL != nil {
		t.Errorf("client = %+v", cl)
	}
}

func TestAgencyClientUpdateParamsEmpty(t *testing.T) {
	b, err := json.Marshal(AgencyClientUpdateParams{})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{}` {
		t.Errorf("empty params = %s", b)
	}
}

func TestAgencyUsageAndAIDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"unlimited":false,"period_start":"2026-11-01T00:00:00Z","period_end":"2026-12-01T00:00:00Z","days_left":16,"pool_size":500000,"topups":50000,"total":550000,"used":210000,"remaining":340000,"projected":420000,"clients":[{"organization_id":"`+orgID+`","name":"Acme","used":90000,"monthly_send_limit":100000}],"topup_sends":50000,"topup_price_cents":6000}`)
	u, _, err := c.Agency.Usage(context.Background(), agID)
	if err != nil {
		t.Fatal(err)
	}
	if u.Total != 550000 || u.DaysLeft != 16 || u.TopupPriceCents != 6000 || len(u.Clients) != 1 || u.Clients[0].MonthlySendLimit == nil {
		t.Errorf("usage = %+v", u)
	}

	c = recordingFixtureClient(t, &got, `{"unmetered":false,"period_start":"2026-11-01T00:00:00Z","period_end":"2026-12-01T00:00:00Z","allowance_usd":50,"allowance_left_usd":12.5,"purchased_usd":100,"bought_this_period_usd":100,"spent_usd":37.5,"moved_usd":20,"balance_usd":112.5,"clients":[{"organization_id":"`+orgID+`","name":"Acme","spent_usd":30,"moved_usd":20,"monthly_ai_limit_usd":40}],"top_up":{"min_usd":10,"max_usd":5000,"presets_usd":[25,50,100],"default_usd":50}}`)
	b, _, err := c.Agency.AIBalance(context.Background(), agID)
	if err != nil {
		t.Fatal(err)
	}
	if b.BalanceUSD != 112.5 || b.BoughtThisPeriodUSD != 100 || len(b.TopUp.PresetsUSD) != 3 || b.Clients[0].MonthlyAILimitUSD == nil {
		t.Errorf("ai balance = %+v", b)
	}

	c = recordingFixtureClient(t, &got, `{"amount_usd":25,"agency_balance_usd":87.5,"workspace_balance_usd":31.2}`)
	tr, _, err := c.Agency.TransferAIBalance(context.Background(), agID, orgID, &AgencyAITransferParams{AmountUSD: 25})
	if err != nil {
		t.Fatal(err)
	}
	if tr.AmountUSD != 25 || tr.AgencyBalanceUSD != 87.5 || tr.WorkspaceBalanceUSD != 31.2 {
		t.Errorf("transfer = %+v", tr)
	}
}

func TestAgencyOverviewDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{
	  "generated_at": "2026-11-15T09:00:00Z",
	  "usage": null,
	  "totals": {"clients":2,"sent_today":1200,"replies_waiting":7,"positive_replies_7d":3,"active_campaigns":5,"mailboxes":40,"mailbox_problems":2},
	  "attention": [
	    {"kind":"pool_low","severity":"critical","count":1200},
	    {"kind":"bounce_spike","severity":"warning","organization_id":"`+orgID+`","client_name":"Acme","count":44,"rate":0.071},
	    {"kind":"something_new","severity":"warning","count":1}
	  ],
	  "clients": [
	    {"organization_id":"`+orgID+`","name":"Acme","access":"full","manage_mailboxes":true,"pool_used":90000,
	     "campaigns":{"active":3,"paused_by_limits":1,"paused_by_billing":0,"sent_today":800,"sent_7d":5000,"bounced_7d":44,"bounce_rate_7d":0.0088,"positive_replies_7d":3},
	     "inbox":{"replies_waiting":7},
	     "mailboxes":{"total":30,"disconnected":1,"erroring":1,"watch":0,"throttled":0,"quarantined":0,"blocked":0},
	     "advisor":{"critical":0,"high":1,"medium":2,"low":0}},
	    {"organization_id":"o2","name":"Globex","access":"none","manage_mailboxes":false,"pool_used":0}
	  ]
	}`)
	o, _, err := c.Agency.Overview(context.Background(), agID)
	if err != nil {
		t.Fatal(err)
	}
	if o.Usage != nil || o.Totals.MailboxProblems != 2 || len(o.Attention) != 3 || len(o.Clients) != 2 {
		t.Fatalf("overview = %+v", o)
	}
	if o.Attention[0].Kind != AgencyAttentionPoolLow || o.Attention[0].OrganizationID != nil {
		t.Errorf("attention 0 = %+v", o.Attention[0])
	}
	if a := o.Attention[1]; a.Kind != AgencyAttentionBounceSpike || a.OrganizationID == nil || a.Rate != 0.071 {
		t.Errorf("attention 1 = %+v", a)
	}
	if o.Attention[2].Kind != "something_new" {
		t.Errorf("unknown kind not kept: %+v", o.Attention[2])
	}
	full, none := o.Clients[0], o.Clients[1]
	if full.Access != AgencyClientAccessFull || full.Campaigns == nil || full.Campaigns.Bounced7d != 44 || full.Advisor.High != 1 || full.Mailboxes.Total != 30 {
		t.Errorf("full client = %+v", full)
	}
	if none.Access != AgencyClientAccessNone || none.Campaigns != nil || none.Inbox != nil || none.Mailboxes != nil || none.Advisor != nil {
		t.Errorf("no-access client = %+v", none)
	}
}

func TestAgencyInboxPaginates(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("cursor") == "" {
			_, _ = io.WriteString(w, `{"data":[{"organization_id":"`+orgID+`","id":"m1","email_id":"e1","thread_id":"t1","from_addr":["lead@example.com"],"to_addr":["rep@acme.test"],"subject":"Re: pricing","snippet":"Sounds good","internal_date":"2026-11-14T10:00:00Z","seen":false,"message_count":4,"has_unread":true}],"pagination":{"next_cursor":"n1","has_more":true}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":[{"organization_id":"o2","id":"m2","email_id":"e2","thread_id":"t2","from_addr":[],"to_addr":[],"subject":"Hi","snippet":"","internal_date":"2026-11-13T10:00:00Z","seen":true,"message_count":1,"has_unread":false}],"pagination":{"next_cursor":null,"has_more":false}}`)
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmagency_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatal(err)
	}
	page, err := c.Agency.Inbox(context.Background(), agID, &AgencyInboxListParams{Clients: []string{orgID, "o2"}, Unread: true})
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, 2)
	for th, err := range page.All(context.Background()) {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, th.ID)
	}
	if strings.Join(ids, ",") != "m1,m2" {
		t.Errorf("ids = %v", ids)
	}
	if page.Data[0].MessageCount != 4 || !page.Data[0].HasUnread || page.Data[0].OrganizationID != orgID {
		t.Errorf("thread = %+v", page.Data[0])
	}
	want := []string{
		"client=" + orgID + "%2Co2&unread=true",
		"client=" + orgID + "%2Co2&cursor=n1&unread=true",
	}
	if strings.Join(queries, "|") != strings.Join(want, "|") {
		t.Errorf("queries = %v", queries)
	}
}

func TestAgencyInboxThreadAndMessageDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"data":[{"id":"m1","email_id":"e1","thread_id":"t1","from_addr":["lead@example.com"],"to_addr":["rep@acme.test"],"subject":"Re: pricing","snippet":"Sounds good","internal_date":"2026-11-14T10:00:00Z","sent_date":"2026-11-14T09:59:00Z","seen":true}],"pagination":{"next_cursor":null,"has_more":false}}`)
	page, err := c.Agency.InboxThread(context.Background(), agID, orgID, "t1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.rawQuery != "organization_id="+orgID+"&thread_id=t1" {
		t.Errorf("query = %q", got.rawQuery)
	}
	if len(page.Data) != 1 || page.Data[0].FromAddr[0] != "lead@example.com" || page.HasMore() {
		t.Errorf("thread = %+v", page)
	}

	c = recordingFixtureClient(t, &got, `{"id":"m1","gmail_id":"","uid":0,"email_id":"e1","folder":"inbox","parent_id":"","thread_id":"t1","flags":[],"bcc":[],"cc":[],"date":"2026-11-14T09:59:00Z","from":["lead@example.com"],"in_reply_to":[],"message_id":"<a@b>","ReplyTo":[],"to":["rep@acme.test"],"subject":"Re: pricing","size":2048,"body_plain":"Sounds good","seen":false}`)
	msg, _, err := c.Agency.InboxMessage(context.Background(), agID, orgID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	if msg.BodyPlain != "Sounds good" || len(msg.FromAddr) != 1 || msg.SentDate.IsZero() || msg.Seen {
		t.Errorf("message = %+v", msg)
	}
}

func TestAgencyMailboxesDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"data":[{"id":"mb1","organization_id":"`+orgID+`","client":"Acme","email":"rep@acme.test","name":"Rep","provider":"google","status":"connected","erroring":false,"warmup":true,"warmup_state":"watch","vendor":"zapmail","campaign_limit":30,"created_at":"2026-10-01T08:00:00Z"}],"pagination":{"next_cursor":"abc","has_more":true}}`)
	page, err := c.Agency.Mailboxes(context.Background(), agID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if page.NextCursor() != "abc" || len(page.Data) != 1 || page.Data[0].WarmupState != "watch" || page.Data[0].Client != "Acme" {
		t.Errorf("page = %+v", page)
	}
}

func TestAgencyMigrationDecode(t *testing.T) {
	const fixture = `{
	  "id": "mig_1",
	  "agency_id": "` + agID + `",
	  "created_by_user_id": "u1",
	  "provider": "smartlead",
	  "label": "Smartlead import",
	  "status": "running",
	  "accounts": [{"name": "Northwind", "email": "ops@northwind.test", "plan": "Pro"}],
	  "inventory": [{
	    "workspace": {"id": "w1", "name": "Acme", "key": 0},
	    "inventory": {"mailboxes": 4, "campaigns": 2, "leads": 900, "blocklist": 12,
	      "campaign_summaries": [{"id": "c1", "name": "Q4", "status": "active", "steps": 3, "leads": 600}]}
	  }],
	  "options": {"kinds": ["blocklist", "campaigns", "leads"], "in_flight": "restart", "skip_finished": true, "warmup": false},
	  "targets": [{
	    "id": "tg1", "migration_id": "mig_1",
	    "workspace": {"id": "w1", "name": "Acme"},
	    "organization_id": "` + orgID + `", "organization_name": "Acme",
	    "status": "running", "phase": "leads",
	    "counts": {"blocklist_imported": 12, "mailboxes_queued": 0, "mailboxes_manual": 0, "campaigns_created": 2, "campaigns_skipped": 0,
	      "steps_created": 6, "leads_read": 400, "contacts_imported": 380, "contacts_updated": 20, "leads_enrolled": 390,
	      "leads_not_enrolled": 5, "leads_suppressed": 3, "leads_failed": 2},
	    "followup": {"reconnect": [{"email": "rep@acme.test", "kind": "google"}], "notes": ["LinkedIn steps were skipped"],
	      "campaigns": [{"source_id": "c1", "campaign_id": "cmp1", "name": "Q4", "missing_senders": ["old@acme.test"]}]},
	    "category_id": "cat1",
	    "started_at": "2026-11-15T08:00:00Z",
	    "updated_at": "2026-11-15T08:10:00Z"
	  }],
	  "credentials_held": true,
	  "started_at": "2026-11-15T08:00:00Z",
	  "created_at": "2026-11-15T07:55:00Z",
	  "updated_at": "2026-11-15T08:10:00Z"
	}`
	var got recordedRequest
	c := recordingFixtureClient(t, &got, fixture)
	m, _, err := c.Agency.GetMigration(context.Background(), agID, "mig_1")
	if err != nil {
		t.Fatal(err)
	}
	if m.Status != AgencyMigrationRunning || m.AgencyID == nil || m.OrganizationID != nil || !m.CredentialsHeld || m.FinishedAt != nil {
		t.Errorf("migration = %+v", m)
	}
	if len(m.Inventory) != 1 || m.Inventory[0].Inventory.Leads != 900 || m.Inventory[0].Inventory.Summaries[0].Steps != 3 {
		t.Errorf("inventory = %+v", m.Inventory)
	}
	if m.Options == nil || !m.Options.SkipFinished || m.Options.InFlight != AgencyMigrationRestart || len(m.Options.Kinds) != 3 {
		t.Errorf("options = %+v", m.Options)
	}
	tg := m.Targets[0]
	if tg.OrganizationName != "Acme" || tg.Phase != AgencyMigrationKindLeads || tg.Counts.LeadsEnrolled != 390 || tg.MailboxImportID != nil || tg.CategoryID == nil {
		t.Errorf("target = %+v", tg)
	}
	if len(tg.Followup.Reconnect) != 1 || tg.Followup.Campaigns[0].MissingSenders[0] != "old@acme.test" {
		t.Errorf("followup = %+v", tg.Followup)
	}
}

func TestAgencyMigrationProvidersDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"data":[{"id":"smartlead","label":"Smartlead","fields":[{"key":"api_key","label":"API key","secret":true},{"key":"region","label":"Region","secret":false,"optional":true,"options":["us","eu"]}],"key_help":"Settings, API","kinds":["mailboxes","campaigns","leads","blocklist"],"workspaces":true,"multi_key":false,"mailbox_secrets":true}]}`)
	ps, _, err := c.Agency.MigrationProviders(context.Background(), agID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || !ps[0].Workspaces || !ps[0].MailboxSecrets || len(ps[0].Fields) != 2 || ps[0].Fields[1].Options[1] != "eu" {
		t.Errorf("providers = %+v", ps)
	}
}

func TestAgencyVendorsDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{
	  "data": [{"id":"v1","vendor":"zapmail","label":"Main","created_at":"2026-10-01T08:00:00Z","status":"active","mailboxes":20,
	    "clients":[{"organization_id":"`+orgID+`","name":"Acme","connection_id":"vc1","mailboxes":0,"mailboxes_hidden":true,"status":"active"}]}],
	  "catalog": [{"id":"zapmail","label":"Zapmail","website":"https://zapmail.test","key_help_url":"https://zapmail.test/keys","fields":[{"key":"api_key","label":"API key","secret":true,"required":true}],"signs_in":true}]
	}`)
	v, _, err := c.Agency.Vendors(context.Background(), agID)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Data) != 1 || v.Data[0].Mailboxes != 20 || !v.Data[0].Clients[0].MailboxesHidden || len(v.Catalog) != 1 || !v.Catalog[0].SignsIn || !v.Catalog[0].Fields[0].Required {
		t.Errorf("vendors = %+v", v)
	}

	c = recordingFixtureClient(t, &got, `{"id":"vc1","vendor":"zapmail","label":"Main","status":"active","created_at":"2026-10-01T08:00:00Z","mailboxes":0,"agency_connection_id":"v1"}`)
	conn, _, err := c.Agency.UseVendor(context.Background(), agID, "v1", orgID)
	if err != nil {
		t.Fatal(err)
	}
	if conn.AgencyConnectionID == nil || *conn.AgencyConnectionID != "v1" || conn.LastUsedAt != nil {
		t.Errorf("connection = %+v", conn)
	}
}

func TestAgencyBillingDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"checkout_url":"https://pay.test/cs_1"}`)
	co, _, err := c.Agency.Checkout(context.Background(), agID, &AgencyCheckoutParams{PlanID: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if co.CheckoutURL != "https://pay.test/cs_1" {
		t.Errorf("checkout = %+v", co)
	}

	c = recordingFixtureClient(t, &got, `{"url":"https://pay.test/portal"}`)
	u, _, err := c.Agency.BillingPortal(context.Background(), agID, "https://app.test/back")
	if err != nil {
		t.Fatal(err)
	}
	if u != "https://pay.test/portal" || strings.TrimSpace(got.body) != `{"return_url":"https://app.test/back"}` {
		t.Errorf("portal = %q, body %s", u, got.body)
	}

	c = recordingFixtureClient(t, &got, `{"plan_id":"p2","interval":"year","proration_cents":-1250,"amount_due_cents":478800,"currency":"usd","proration_date":1789000000,"from_price_id":"price_a"}`)
	pv, _, err := c.Agency.PreviewPlanChange(context.Background(), agID, "p2", AgencyIntervalYear)
	if err != nil {
		t.Fatal(err)
	}
	if pv.ProrationCents != -1250 || pv.ProrationDate != 1789000000 || pv.FromPriceID != "price_a" {
		t.Errorf("preview = %+v", pv)
	}

	c = recordingFixtureClient(t, &got, `{"enabled":false,"threshold_sends":5000,"max_packs":3,"bought_this_period":1,"failure":"card_declined","pack_sends":50000,"pack_price_cents":6000,"card":{"brand":"visa","last4":"4242"},"available":true}`)
	at, _, err := c.Agency.AutoTopup(context.Background(), agID)
	if err != nil {
		t.Fatal(err)
	}
	if at.Failure != AgencyAutoTopupCardDeclined || at.Card == nil || at.Card.Last4 != "4242" || at.CardMissing || at.PackPriceCents != 6000 {
		t.Errorf("auto topup = %+v", at)
	}

	c = recordingFixtureClient(t, &got, `{"enabled":true,"threshold_usd":5,"amount_usd":50,"limit_usd":200,"added_this_period_usd":50,"card_missing":true,"available":true,"min_usd":10,"max_usd":5000}`)
	ai, _, err := c.Agency.AIAutoTopup(context.Background(), agID)
	if err != nil {
		t.Fatal(err)
	}
	if !ai.CardMissing || ai.Card != nil || ai.AddedThisPeriodUSD != 50 || ai.MaxUSD != 5000 {
		t.Errorf("ai auto topup = %+v", ai)
	}
}

func TestAgencyBrandDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{
	  "brand": {"display_name":"Northwind","logo_url":"https://cdn.test/l.png","accent_color":"#0284c7","website_url":"https://northwind.test","hide_powered_by":true,"updated_at":"2026-11-01T10:00:00Z","updated_by":"u1"},
	  "full_white_label": true,
	  "can_manage": false,
	  "portal_domain": {"forms_domain":"portal.northwind.test","forms_domain_verified":true,"forms_domain_verified_at":"2026-11-02T10:00:00Z","cname_target":"portal.warmbly.test","txt_name":"_warmbly.portal.northwind.test","txt_value":"wm-123","status":"verified","message":"Verified.","forms_host_unresolvable":false,
	    "edge_records":[{"purpose":"ssl","type":"TXT","name":"_acme-challenge.portal.northwind.test","value":"x","ok":false}]},
	  "portal_base_url": "https://portal.northwind.test",
	  "powered_by": {"name":"Warmbly","url":"https://warmbly.test"},
	  "portal_sign_in": "warmbly",
	  "icon_url": "https://cdn.test/i.png",
	  "sign_in_image_url": "",
	  "theme": {"font":"geometric","corners":"round","sign_in_layout":"cover","sign_in_background":"image","support_email":"help@northwind.test"}
	}`)
	b, _, err := c.Agency.Brand(context.Background(), agID)
	if err != nil {
		t.Fatal(err)
	}
	if !b.Brand.HidePoweredBy || b.Brand.UpdatedBy == nil || b.CanManage || b.PortalSignIn != AgencyPortalSignInWarmbly {
		t.Errorf("brand = %+v", b)
	}
	d := b.PortalDomain
	if d.Domain != "portal.northwind.test" || !d.Verified || d.VerifiedAt == nil || d.TXTValue != "wm-123" || len(d.EdgeRecords) != 1 || d.EdgeRecords[0].Type != "TXT" {
		t.Errorf("portal domain = %+v", d)
	}
	if b.PoweredBy == nil || b.PoweredBy.Name != "Warmbly" {
		t.Errorf("powered by = %+v", b.PoweredBy)
	}
	if b.Theme.Font != AgencyPortalFontGeometric || b.Theme.SignInBackground != AgencyPortalSignInBackgroundImage || b.Theme.SupportEmail != "help@northwind.test" {
		t.Errorf("theme = %+v", b.Theme)
	}
}

func TestAgencyAPIKeysDecode(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{"data":[{"id":"k1","agency_id":"`+agID+`","created_by":"u1","name":"CI","key_prefix":"wmagency_ab","key_suffix":"wxyz","permissions":67108866,"allowed_ips":[],"rate_limit_per_minute":60,"status":"active","last_used_at":"2026-11-14T10:00:00Z","created_at":"2026-11-01T10:00:00Z","updated_at":"2026-11-01T10:00:00Z"}],"pagination":{"total":null,"next_cursor":"nk","has_more":true}}`)
	page, err := c.Agency.APIKeys(context.Background(), agID, nil)
	if err != nil {
		t.Fatal(err)
	}
	k := page.Data[0]
	if !k.Can(PermAgencyRead) || !k.Can(PermReadCampaigns) || k.Can(PermAgencyManageClients) || k.Status != APIKeyStatusActive || k.LastUsedAt == nil || k.ExpiresAt != nil {
		t.Errorf("key = %+v", k)
	}
	if page.NextCursor() != "nk" {
		t.Errorf("cursor = %q", page.NextCursor())
	}

	c = recordingFixtureClient(t, &got, `{"id":"k2","agency_id":"`+agID+`","created_by":"u1","name":"New","key_prefix":"wmagency_cd","key_suffix":"1234","permissions":67108864,"allowed_ips":["10.0.0.0/8"],"rate_limit_per_minute":60,"status":"active","created_at":"2026-11-15T10:00:00Z","updated_at":"2026-11-15T10:00:00Z","secret":"wmagency_cdsecret1234"}`)
	nk, _, err := c.Agency.CreateAPIKey(context.Background(), agID, &AgencyAPIKeyCreateParams{Name: "New", Permissions: PermAgencyRead, AllowedIPs: []string{"10.0.0.0/8"}})
	if err != nil {
		t.Fatal(err)
	}
	if nk.Secret != "wmagency_cdsecret1234" || nk.ID != "k2" || nk.AllowedIPs[0] != "10.0.0.0/8" {
		t.Errorf("new key = %+v", nk)
	}

	c = recordingFixtureClient(t, &got, `{"workspace":[{"name":"READ_EMAILS","value":1,"description":"Read email accounts","category":"read"}],"agency":[{"name":"AGENCY_READ","value":67108864,"description":"View the agency","category":"agency"},{"name":"AGENCY_MIGRATE","value":2147483648,"description":"Migrate","category":"agency"}],"presets":{"read_only":335544320,"full_access":4294967295},"grantable":4294967295}`)
	perms, _, err := c.Agency.APIKeyPermissions(context.Background(), agID)
	if err != nil {
		t.Fatal(err)
	}
	if len(perms.Agency) != 2 || perms.Agency[0].Value != PermAgencyRead || perms.Agency[1].Value != PermAgencyMigrate || perms.Presets.FullAccess != perms.Grantable {
		t.Errorf("permissions = %+v", perms)
	}
}
