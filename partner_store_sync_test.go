package warmbly

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Server 83c7df4..db3ae43: partner-managed workspaces, balance card setup and
// fee, AI overage settings, store ledger category, store scopes and codes.

func TestSyncPartnerMailboxes(t *testing.T) {
	var got recordedRequest
	c := agentToolServer(t, http.StatusOK, `{"data":[
		{"email_account_id":"e1","enablement":"sending_warmup","managed":true},
		{"email_account_id":"e2","enablement":"off","managed":false}]}`, &got)
	out, _, err := c.Emails.PartnerMailboxes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	checkRequest(t, got, "GET", "/v1/emails/partner", "", "")
	if len(out) != 2 || out[0].EmailAccountID != "e1" || out[0].Enablement != PartnerEnablementSendingWarmup || !out[0].Managed || out[1].Managed {
		t.Fatalf("%+v", out)
	}
	c = agentToolServer(t, http.StatusOK, `{"data":null}`, nil)
	if out, _, err := c.Emails.PartnerMailboxes(context.Background()); err != nil || out == nil || len(out) != 0 {
		t.Fatalf("%#v %v", out, err)
	}
}

func TestSyncBalanceCardSetup(t *testing.T) {
	ctx := context.Background()
	var got recordedRequest
	c := agentToolServer(t, http.StatusOK, `{"client_secret":"seti_1_secret_x","publishable_key":"pk_test"}`, &got)
	setup, _, err := c.Billing.CreateCardSetupIntent(ctx)
	if err != nil {
		t.Fatal(err)
	}
	checkRequest(t, got, "POST", "/v1/subscription/balance/setup-intent", "", "")
	if setup.ClientSecret != "seti_1_secret_x" || setup.PublishableKey != "pk_test" {
		t.Fatalf("%+v", setup)
	}

	c = agentToolServer(t, http.StatusNoContent, ``, &got)
	if _, err := c.Billing.AdoptCardSetup(ctx, "seti_1"); err != nil {
		t.Fatal(err)
	}
	checkRequest(t, got, "POST", "/v1/subscription/balance/setup-intent/default", "", `{"setup_intent_id":"seti_1"}`)
}

func TestSyncBalanceFeeAndBurn(t *testing.T) {
	ctx := context.Background()
	var got recordedRequest
	c := agentToolServer(t, http.StatusOK, `{"unlimited":false,"can_add_money":true,"balance_usd":46.4375,
		"top_up":{"min_usd":5,"max_usd":5000,"presets_usd":[10,25,50,100,250],"default_usd":25,"fee_bps":300},
		"stripe_publishable_key":"pk_live",
		"burn":{"per_day_usd":1.12,"mailboxes_per_day_usd":0.3,"renewals_per_day_usd":0.62,"ai_per_day_usd":0.2,
			"mailboxes_monthly_usd":9,"renewals_due_usd":11.17,"billing_day":17}}`, &got)
	b, _, err := c.Billing.BalanceWithBurn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	checkRequest(t, got, "GET", "/v1/subscription/balance", "include=burn", "")
	if b.TopUp == nil || b.TopUp.FeeBps != 300 || b.StripePublishableKey != "pk_live" || b.Burn == nil ||
		b.Burn.PerDayUSD != 1.12 || b.Burn.BillingDay == nil || *b.Burn.BillingDay != 17 || b.Burn.RenewalsDueUSD != 11.17 {
		t.Fatalf("%+v %+v", b, b.Burn)
	}

	c = agentToolServer(t, http.StatusOK, `{"unlimited":false,"burn":{"per_day_usd":0,"mailboxes_monthly_usd":0,"billing_day":null}}`, &got)
	b, _, err = c.Billing.Balance(ctx)
	if err != nil || b.Burn == nil || b.Burn.BillingDay != nil {
		t.Fatalf("%+v %v", b, err)
	}
	checkRequest(t, got, "GET", "/v1/subscription/balance", "", "")

	c = agentToolServer(t, http.StatusOK, `{"status":"paid","amount_usd":25,"charged_usd":25.75,"balance_usd":46.4375,"charge_in_progress":false}`, &got)
	top, _, err := c.Billing.TopUpBalance(ctx, 25)
	if err != nil || top.AmountUSD != 25 || top.ChargedUSD != 25.75 {
		t.Fatalf("%+v %v", top, err)
	}
}

func TestSyncCreditSettingsOverage(t *testing.T) {
	ctx := context.Background()
	var got recordedRequest
	resp := `{"spend_limit_daily_usd":null,"spend_limit_weekly_usd":5,"spend_limit_monthly_usd":null,
		"member_limit_daily_usd":null,"member_limit_weekly_usd":null,"member_limit_monthly_usd":2.5,
		"low_balance_usd":1,"auto_reload_enabled":true,
		"ai_overage_enabled":true,"ai_overage_monthly_limit_usd":20,"ai_overage_monthly_limit_micros":20000000}`
	c := agentToolServer(t, http.StatusOK, resp, &got)
	s, _, err := c.Billing.CreditSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !s.AIOverageEnabled || s.AIOverageMonthlyLimitUSD == nil || *s.AIOverageMonthlyLimitUSD != 20 ||
		s.AIOverageMonthlyLimitMicros == nil || *s.AIOverageMonthlyLimitMicros != 20000000 ||
		s.SpendLimitWeeklyUSD == nil || *s.SpendLimitWeeklyUSD != 5 || s.SpendLimitDailyUSD != nil ||
		s.MemberLimitMonthlyUSD == nil || s.LowBalanceUSD != 1 || !s.AutoReloadEnabled {
		t.Fatalf("%+v", s)
	}

	on, limit := false, 12.5
	if _, _, err := c.Billing.UpdateCreditSettings(ctx, &CreditSettingsParams{AIOverageEnabled: &on, AIOverageMonthlyLimitUSD: &limit}); err != nil {
		t.Fatal(err)
	}
	checkRequest(t, got, "PATCH", "/v1/subscription/credits/settings", "", `{"ai_overage_enabled":false,"ai_overage_monthly_limit_usd":12.5}`)

	if _, _, err := c.Billing.UpdateCreditSettings(ctx, &CreditSettingsParams{AIOverageMonthlyLimitUSD: &limit, ClearAIOverageLimit: true}); err != nil {
		t.Fatal(err)
	}
	checkRequest(t, got, "PATCH", "/v1/subscription/credits/settings", "", `{"ai_overage_monthly_limit_usd":null}`)

	if _, _, err := c.Billing.UpdateCreditSettings(ctx, &CreditSettingsParams{}); err != nil {
		t.Fatal(err)
	}
	checkRequest(t, got, "PATCH", "/v1/subscription/credits/settings", "", `{}`)
}

func TestSyncCreditUsageAndStoreLedger(t *testing.T) {
	ctx := context.Background()
	var got recordedRequest
	c := agentToolServer(t, http.StatusOK, `{"spent_today_usd":0.5,"spent_week_usd":1,"spent_month_usd":3,
		"limit_daily_usd":null,"limit_weekly_usd":10,"limit_monthly_usd":null,"spent_today":50,"spent_week":100,"spent_month":300,
		"ai_from_balance_month_usd":1.25,
		"series":[{"date":"2026-11-20","amount_usd":0.5,"tokens":900,"credits":50}],
		"by_reason":[{"key":"reply_draft","amount_usd":0.5,"tokens":900,"count":3,"credits":50}],"by_model":[]}`, &got)
	u, _, err := c.Billing.CreditUsage(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	checkRequest(t, got, "GET", "/v1/subscription/credits/usage", "days=7", "")
	if u.AIFromBalanceMonthUSD != 1.25 || u.SpentMonthUSD != 3 || u.LimitWeeklyUSD == nil || u.LimitDailyUSD != nil ||
		len(u.Series) != 1 || u.Series[0].AmountUSD != 0.5 || u.ByReason[0].AmountUSD != 0.5 || u.ByReason[0].Count != 3 {
		t.Fatalf("%+v", u)
	}

	c = agentToolServer(t, http.StatusOK, `{"data":[{"id":"t1","org_id":"o","reason":"store_order","model_used":"",
		"amount_usd":-12.5,"cost_usd":0,"input_tokens":0,"output_tokens":0,"quantity":1,"balance_after_usd":30,
		"monthly_after_usd":5,"purchased_after_usd":25,"purchased_delta_usd":-12.5,"agency_delta_usd":0,
		"actor_user_id":null,"context":{"detail":"acme.com"},"created_at":"2026-11-20T10:00:00Z",
		"amount":-1250,"tokens_used":0,"balance_after":500,"purchased_delta":-1250,"purchased_balance_after":2500}],
		"pagination":{"next_cursor":"cur2","has_more":true}}`, &got)
	page, err := c.Billing.CreditTransactionsInCategory(ctx, CreditCategoryStore, &ListOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got.method != "GET" || got.path != "/v1/subscription/credits/transactions" || got.rawQuery != "category=store&limit=10" {
		t.Fatalf("%+v", got)
	}
	if len(page.Data) != 1 || page.Data[0].AmountUSD != -12.5 || page.Data[0].PurchasedAfterUSD != 25 || page.Data[0].Quantity != 1 || page.Data[0].Context.Detail != "acme.com" {
		t.Fatalf("%+v", page.Data)
	}
	if _, err := page.Next(ctx); err != nil {
		t.Fatal(err)
	}
	if got.rawQuery != "category=store&cursor=cur2&limit=10" {
		t.Fatalf("next query = %q", got.rawQuery)
	}

	c = agentToolServer(t, http.StatusBadRequest, `{"error":"Bad Request","message":"category must be store or left out.","code":"invalid_category","request_id":"r"}`, nil)
	_, err = c.Billing.CreditTransactionsInCategory(ctx, "ai", nil)
	var apiErr *Error
	if !errors.As(err, &apiErr) || !apiErr.HasCode(ErrCodeInvalidCategory) {
		t.Fatalf("%v", err)
	}
}

func TestSyncPartnerManagedShapes(t *testing.T) {
	var org OrganizationWithLimits
	in := `{"id":"o","name":"Acme","owner_user_id":"u","created_at":"2026-11-01T00:00:00Z","updated_at":"2026-11-01T00:00:00Z",
		"presence_show_online":true,"presence_show_activity":true,"product_description":"","icp_notes":"","voice_profile":"",
		"inbox_agent_enabled":false,"assistant_shared_history":false,
		"partner":{"display_name":"Resell Co","state":"suspended","suspended_reason":"billing","deletes_at":"2026-12-20T00:00:00Z"}}`
	if err := json.Unmarshal([]byte(in), &org); err != nil {
		t.Fatal(err)
	}
	if p := org.Partner; p == nil || p.DisplayName != "Resell Co" || p.State != PartnerOrgSuspended || p.SuspendedReason == nil || *p.SuspendedReason != "billing" || p.DeletesAt == nil {
		t.Fatalf("%+v", org.Partner)
	}
	var plain OrganizationWithLimits
	if err := json.Unmarshal([]byte(`{"id":"o"}`), &plain); err != nil || plain.Partner != nil {
		t.Fatalf("%+v %v", plain, err)
	}

	var u User
	if err := json.Unmarshal([]byte(`{"id":"u","first_name":"A","last_name":"B","email":"a@client.test","roles":[],"partner_account":true}`), &u); err != nil || !u.PartnerAccount || u.ContactEmail != "" {
		t.Fatalf("%+v %v", u, err)
	}
	if err := json.Unmarshal([]byte(`{"id":"u","email":"svc@p.invalid","contact_email":"a@client.test"}`), &u); err != nil || u.ContactEmail != "a@client.test" {
		t.Fatalf("%+v %v", u, err)
	}

	var e Email
	if err := json.Unmarshal([]byte(`{"id":"e","email":"x@acme.com","purchased":true,"purchase_cancelled_at":"2026-12-01T00:00:00Z","avatar_url":""}`), &e); err != nil || !e.Purchased || e.PurchaseCancelledAt == nil { //nolint:misspell // wire name
		t.Fatalf("%+v %v", e, err)
	}
	var g DomainGrant
	if err := json.Unmarshal([]byte(`{"id":"g","provider":"google","tenant":"acme.com","domains":["acme.com"],"status":"active","created_at":"2026-11-01T00:00:00Z","mailboxes":2,"purchased":true}`), &g); err != nil || !g.Purchased {
		t.Fatalf("%+v %v", g, err)
	}
	var p MailboxSyncPolicy
	if err := json.Unmarshal([]byte(`{"backfill_days":90,"backfill_messages":5000,"daily_messages":2000,"org_daily_messages":20000,"full_check_seconds":300}`), &p); err != nil || p.FullCheckSeconds != 300 {
		t.Fatalf("%+v %v", p, err)
	}
	var top AgencyAITopUpLimits
	if err := json.Unmarshal([]byte(`{"min_usd":10,"max_usd":5000,"presets_usd":[25,50],"default_usd":50,"fee_bps":300}`), &top); err != nil || top.FeeBps != 300 {
		t.Fatalf("%+v %v", top, err)
	}
}

func TestSyncSegmentIDsOnSources(t *testing.T) {
	var f Form
	if err := json.Unmarshal([]byte(`{"id":"f","category_ids":[],"segment_ids":["s1","s2"],"allowed_domains":[]}`), &f); err != nil || len(f.SegmentIDs) != 2 {
		t.Fatalf("%+v %v", f, err)
	}
	segs := []string{"s1"}
	b, _ := json.Marshal(FormUpdateParams{SegmentIDs: &segs})
	if string(b) != `{"segment_ids":["s1"]}` {
		t.Fatalf("form params = %s", b)
	}
	empty := []string{}
	b, _ = json.Marshal(ApolloImportSourceParams{SegmentIDs: &empty})
	if string(b) != `{"segment_ids":[]}` {
		t.Fatalf("apollo params = %s", b)
	}
	b, _ = json.Marshal(ApolloImportSourceParams{})
	if string(b) != `{}` {
		t.Fatalf("apollo params = %s", b)
	}
	b, _ = json.Marshal(SalesforceImportSourceParams{SegmentIDs: &segs, Recurring: new(bool)})
	if string(b) != `{"segment_ids":["s1"],"recurring":false}` {
		t.Fatalf("salesforce params = %s", b)
	}
	var a ApolloImportSource
	if err := json.Unmarshal([]byte(`{"id":"a","category_ids":[],"segment_ids":["s1"]}`), &a); err != nil || len(a.SegmentIDs) != 1 {
		t.Fatalf("%+v %v", a, err)
	}
	var s SalesforceImportSource
	if err := json.Unmarshal([]byte(`{"id":"a","category_ids":[],"segment_ids":["s1"]}`), &s); err != nil || len(s.SegmentIDs) != 1 {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestSyncStorePermissionsAndCatalogue(t *testing.T) {
	// Server: APIPermReadStore and APIPermManageStore follow APIPermAgencyMigrate (bit 31).
	if PermReadStore != 1<<32 || PermManageStore != 1<<33 {
		t.Fatalf("store bits = %d, %d", PermReadStore, PermManageStore)
	}
	// Server presets: read only 4295655455; full access holds MANAGE_STORE too.
	if PermReadOnly != 4295655455 {
		t.Errorf("PermReadOnly = %d", PermReadOnly)
	}
	if PermFullAccess&(PermReadStore|PermManageStore) != PermReadStore|PermManageStore || PermFullAccess&PermAgencyAll != 0 {
		t.Errorf("PermFullAccess = %d", PermFullAccess)
	}
	if PermAppGrantable&(PermReadStore|PermManageStore) != PermReadStore|PermManageStore {
		t.Errorf("PermAppGrantable = %d", PermAppGrantable)
	}
	if AuditActionApprove != "approve" || AuditEntityStoreOrder != "store_order" || AuditEntityStoreDomain != "store_domain" || AuditEntityStoreMailbox != "store_mailbox" {
		t.Error("audit catalog drifted")
	}
}

func TestSyncNewErrorCodes(t *testing.T) {
	codes := map[string]string{
		ErrCodeStoreDisabled: "store_disabled", ErrCodePurchasedMailbox: "purchased_mailbox", ErrCodePriceChanged: "price_changed",
		ErrCodeInsufficientBalance: "insufficient_balance", ErrCodePlanRequired: "plan_required", ErrCodeStoreUnavailable: "store_unavailable",
		ErrCodeAIAllowanceUsed: "ai_allowance_used", ErrCodeAIOverageLimitReached: "ai_overage_limit_reached",
		ErrCodeOrganizationSuspended: "organization_suspended", ErrCodeOrganizationCancelled: "organization_cancelled", //nolint:misspell // wire value
		ErrCodePartnerManaged: "partner_managed", ErrCodePartnerAccount: "partner_account",
		ErrCodePartnerManagedMailbox: "partner_managed_mailbox", ErrCodeLeadStepSent: "lead_step_sent",
		ErrCodeInvalidCategory: "invalid_category", ErrCodeTLDUnsupported: "tld_unsupported", ErrCodeDNSNotHosted: "dns_not_hosted",
	}
	for got, want := range codes {
		if got != want {
			t.Errorf("code %q, want %q", got, want)
		}
	}
	c := agentToolServer(t, http.StatusPaymentRequired, `{"error":"Payment Required","message":"m","code":"ai_allowance_used","request_id":"r"}`, nil)
	_, _, err := c.Billing.Credits(context.Background())
	var apiErr *Error
	if !errors.As(err, &apiErr) || !apiErr.HasCode(ErrCodeAIAllowanceUsed) || apiErr.StatusCode != 402 {
		t.Fatalf("%v", err)
	}
}

func TestSyncClientClosedEmptyBody(t *testing.T) {
	// The server records 499 with no body when the client gave up first.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "req_499")
		w.WriteHeader(499)
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = c.Billing.Credits(context.Background())
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 499 || apiErr.Code != "" || apiErr.RequestID != "req_499" || apiErr.Temporary() {
		t.Fatalf("%#v", err)
	}
	if apiErr.Error() != "warmbly: 499: Client Closed Request [request_id=req_499]" {
		t.Errorf("Error() = %q", apiErr.Error())
	}
	if (&Error{StatusCode: 599}).Error() != "warmbly: 599: unexpected status" {
		t.Errorf("Error() = %q", (&Error{StatusCode: 599}).Error())
	}
}
