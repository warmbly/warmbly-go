package warmbly

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestBillingPlanOffers(t *testing.T) {
	ctx := context.Background()
	var got recordedRequest
	c := agentToolServer(t, http.StatusOK, `{"offers": [
		{"plan_id": "pl_1", "name": "Partner", "currency": "usd", "monthly_usd": 79.5, "yearly_usd": 795,
		 "monthly_available": true, "yearly_available": true, "available": true, "warmup_only": false,
		 "expires_at": "2026-12-31T00:00:00Z"},
		{"plan_id": "pl_2", "name": "Warm", "currency": "usd", "monthly_usd": 19,
		 "monthly_available": false, "yearly_available": false, "available": false, "warmup_only": true}
	]}`, &got)

	offers, _, err := c.Billing.PlanOffers(ctx)
	if err != nil {
		t.Fatalf("PlanOffers: %v", err)
	}
	checkRequest(t, got, "GET", "/v1/subscription/offers", "", "")
	if len(offers) != 2 {
		t.Fatalf("offers = %d, want 2", len(offers))
	}
	if o := offers[0]; o.PlanID != "pl_1" || o.MonthlyUSD != 79.5 || o.YearlyUSD != 795 || !o.Available || o.ExpiresAt == nil {
		t.Errorf("offer 0 = %+v", o)
	}
	if o := offers[1]; o.ExpiresAt != nil || o.YearlyUSD != 0 || !o.WarmupOnly || o.Available {
		t.Errorf("offer 1 = %+v", o)
	}
}

func TestBillingPlanOffersEmpty(t *testing.T) {
	c := agentToolServer(t, http.StatusOK, `{"offers": null}`, nil)
	offers, _, err := c.Billing.PlanOffers(context.Background())
	if err != nil {
		t.Fatalf("PlanOffers: %v", err)
	}
	if offers == nil || len(offers) != 0 {
		t.Errorf("offers = %#v, want empty non-nil", offers)
	}
}

func TestBillingCheckoutPlanOffer(t *testing.T) {
	ctx := context.Background()
	var got recordedRequest
	c := agentToolServer(t, http.StatusOK, `{"checkout_url": "https://checkout.example/s/1"}`, &got)

	out, _, err := c.Billing.CheckoutPlanOffer(ctx, "pl_1", DurationYear)
	if err != nil {
		t.Fatalf("CheckoutPlanOffer: %v", err)
	}
	checkRequest(t, got, "POST", "/v1/subscription/offers/pl_1/checkout", "", `{"interval":"year"}`)
	if out.CheckoutURL != "https://checkout.example/s/1" || out.Changed {
		t.Errorf("out = %+v", out)
	}

	c = agentToolServer(t, http.StatusOK, `{"changed": true}`, &got)
	out, _, err = c.Billing.CheckoutPlanOffer(ctx, "pl_1", "")
	if err != nil {
		t.Fatalf("CheckoutPlanOffer: %v", err)
	}
	checkRequest(t, got, "POST", "/v1/subscription/offers/pl_1/checkout", "", `{}`)
	if !out.Changed || out.CheckoutURL != "" {
		t.Errorf("out = %+v", out)
	}
}

func TestBillingCheckoutPlanOfferRefused(t *testing.T) {
	c := agentToolServer(t, http.StatusForbidden,
		`{"error": "forbidden", "code": "plan_offer_required", "message": "This plan is not offered to this workspace."}`, nil)
	_, _, err := c.Billing.CheckoutPlanOffer(context.Background(), "pl_9", "")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden || apiErr.Code != "plan_offer_required" {
		t.Fatalf("err = %v", err)
	}
}

const balanceFixture = `{
	"unlimited": false,
	"can_add_money": true,
	"balance_usd": 42.125,
	"monthly_usd": 12.125,
	"purchased_usd": 30,
	"monthly_allowance_usd": 20,
	"monthly_used_usd": 7.875,
	"next_reset_at": "2026-12-01T00:00:00Z",
	"top_up": {"min_usd": 5, "max_usd": 5000, "presets_usd": [10, 25, 50, 100, 250], "default_usd": 25},
	"agency_usd": 100,
	"auto_reload": {
		"enabled": true, "threshold_usd": 5, "amount_usd": 25, "monthly_limit_usd": 100,
		"reloaded_this_month_usd": 50, "failure_code": "card_declined", "failed_at": "2026-11-10T08:00:00Z",
		"last_reload": {"amount_usd": 25, "at": "2026-11-05T09:30:00Z"},
		"retry_at": "2026-11-10T09:00:00Z", "retry_reason": "card_declined"
	},
	"low_balance_usd": 2,
	"payment_method": {"brand": "visa", "last4": "4242", "exp_month": 4, "exp_year": 2029},
	"charge_in_progress": false
}`

func TestBillingBalance(t *testing.T) {
	var got recordedRequest
	c := agentToolServer(t, http.StatusOK, balanceFixture, &got)

	b, _, err := c.Billing.Balance(context.Background())
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	checkRequest(t, got, "GET", "/v1/subscription/balance", "", "")
	if b.Unlimited || !b.CanAddMoney || b.BalanceUSD != 42.125 || b.PurchasedUSD != 30 || b.MonthlyUsedUSD != 7.875 || b.AgencyUSD != 100 {
		t.Errorf("balance = %+v", b)
	}
	if b.NextResetAt == nil || b.NextResetAt.Day() != 1 {
		t.Errorf("next_reset_at = %v", b.NextResetAt)
	}
	if b.TopUp == nil || b.TopUp.MinUSD != 5 || b.TopUp.MaxUSD != 5000 || len(b.TopUp.PresetsUSD) != 5 || b.TopUp.DefaultUSD != 25 {
		t.Errorf("top_up = %+v", b.TopUp)
	}
	r := b.AutoReload
	if r == nil || !r.Enabled || r.AmountUSD != 25 || r.ReloadedThisMonthUSD != 50 {
		t.Fatalf("auto_reload = %+v", r)
	}
	if r.FailureCode == nil || *r.FailureCode != ReloadFailureCardDeclined || r.FailedAt == nil {
		t.Errorf("failure = %v %v", r.FailureCode, r.FailedAt)
	}
	if r.LastReload == nil || r.LastReload.AmountUSD != 25 || r.LastReload.At == nil {
		t.Errorf("last_reload = %+v", r.LastReload)
	}
	if r.RetryAt == nil || r.RetryReason == nil || *r.RetryReason != "card_declined" {
		t.Errorf("retry = %v %v", r.RetryAt, r.RetryReason)
	}
	if pm := b.PaymentMethod; pm == nil || pm.Brand != "visa" || pm.Last4 != "4242" || pm.ExpYear != 2029 || pm.Wallet != "" {
		t.Errorf("payment_method = %+v", pm)
	}
}

// A fresh workspace has nulls where the fixture above has values, and an
// unmetered deployment answers with only "unlimited".
func TestBillingBalanceNulls(t *testing.T) {
	c := agentToolServer(t, http.StatusOK, `{
		"unlimited": false, "can_add_money": false, "balance_usd": 0, "monthly_usd": 0, "purchased_usd": 0,
		"monthly_allowance_usd": 0, "monthly_used_usd": 0, "next_reset_at": null,
		"top_up": {"min_usd": 5, "max_usd": 5000, "presets_usd": [10], "default_usd": 25},
		"auto_reload": {"enabled": false, "threshold_usd": 0, "amount_usd": 0, "monthly_limit_usd": 0,
			"reloaded_this_month_usd": 0, "failure_code": null, "failed_at": null, "last_reload": null,
			"retry_at": null, "retry_reason": null},
		"low_balance_usd": 0, "payment_method": null, "charge_in_progress": false
	}`, nil)
	b, _, err := c.Billing.Balance(context.Background())
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if b.NextResetAt != nil || b.PaymentMethod != nil || b.AutoReload == nil {
		t.Errorf("balance = %+v", b)
	}
	if r := b.AutoReload; r.FailureCode != nil || r.FailedAt != nil || r.LastReload != nil || r.RetryAt != nil || r.RetryReason != nil {
		t.Errorf("auto_reload = %+v", r)
	}

	c = agentToolServer(t, http.StatusOK, `{"unlimited": true}`, nil)
	b, _, err = c.Billing.Balance(context.Background())
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if !b.Unlimited || b.TopUp != nil || b.AutoReload != nil {
		t.Errorf("unlimited balance = %+v", b)
	}
}

func TestBillingTopUpBalance(t *testing.T) {
	ctx := context.Background()
	var got recordedRequest
	c := agentToolServer(t, http.StatusOK, `{"status": "paid", "amount_usd": 25, "balance_usd": 67.125, "charge_in_progress": false}`, &got)

	out, resp, err := c.Billing.TopUpBalance(ctx, 25, WithIdempotencyKey("top-up-1"))
	if err != nil {
		t.Fatalf("TopUpBalance: %v", err)
	}
	checkRequest(t, got, "POST", "/v1/subscription/balance/top-up", "", `{"amount_usd":25}`)
	if got.idempotencyKey != "top-up-1" {
		t.Errorf("Idempotency-Key = %q", got.idempotencyKey)
	}
	if out.Status != BalanceTopUpPaid || out.AmountUSD != 25 || out.BalanceUSD != 67.125 || out.ChargeInProgress || resp.StatusCode != http.StatusOK {
		t.Errorf("out = %+v", out)
	}

	c = agentToolServer(t, http.StatusAccepted, `{"status": "processing", "amount_usd": 12.5, "balance_usd": 42.125, "charge_in_progress": true}`, &got)
	out, resp, err = c.Billing.TopUpBalance(ctx, 12.5)
	if err != nil {
		t.Fatalf("TopUpBalance: %v", err)
	}
	checkRequest(t, got, "POST", "/v1/subscription/balance/top-up", "", `{"amount_usd":12.5}`)
	if out.Status != BalanceTopUpProcessing || !out.ChargeInProgress || resp.StatusCode != http.StatusAccepted {
		t.Errorf("out = %+v status %d", out, resp.StatusCode)
	}
}

func TestBillingTopUpBalanceDeclined(t *testing.T) {
	c := agentToolServer(t, http.StatusPaymentRequired,
		`{"code": "card_declined", "request_id": "req_1", "message": "The card was not charged. Try another way to pay."}`, nil)
	_, _, err := c.Billing.TopUpBalance(context.Background(), 25)
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusPaymentRequired || apiErr.Code != ReloadFailureCardDeclined || apiErr.RequestID != "req_1" {
		t.Fatalf("err = %v", err)
	}
}

func TestBillingBalanceCheckouts(t *testing.T) {
	ctx := context.Background()
	var got recordedRequest
	c := agentToolServer(t, http.StatusOK, `{"session_id": "cs_1", "checkout_url": "https://checkout.example/cs_1"}`, &got)

	sess, _, err := c.Billing.BalanceCheckout(ctx, 100)
	if err != nil {
		t.Fatalf("BalanceCheckout: %v", err)
	}
	checkRequest(t, got, "POST", "/v1/subscription/balance/checkout", "", `{"amount_usd":100}`)
	if sess.SessionID != "cs_1" || sess.CheckoutURL != "https://checkout.example/cs_1" {
		t.Errorf("session = %+v", sess)
	}

	sess, _, err = c.Billing.SetupPaymentMethod(ctx)
	if err != nil {
		t.Fatalf("SetupPaymentMethod: %v", err)
	}
	checkRequest(t, got, "POST", "/v1/subscription/balance/payment-method", "", "")
	if sess.SessionID != "cs_1" {
		t.Errorf("session = %+v", sess)
	}
}

func TestBillingUpdateAutoReload(t *testing.T) {
	var got recordedRequest
	c := agentToolServer(t, http.StatusOK, balanceFixture, &got)

	b, _, err := c.Billing.UpdateAutoReload(context.Background(), &AutoReloadParams{
		Enabled: false, ThresholdUSD: 5, AmountUSD: 25, MonthlyLimitUSD: 100,
	})
	if err != nil {
		t.Fatalf("UpdateAutoReload: %v", err)
	}
	checkRequest(t, got, "PATCH", "/v1/subscription/balance/auto-reload", "",
		`{"enabled":false,"threshold_usd":5,"amount_usd":25,"monthly_limit_usd":100}`)
	if b.AutoReload == nil || b.AutoReload.MonthlyLimitUSD != 100 {
		t.Errorf("balance = %+v", b)
	}
}

func TestBillingUpdateAutoReloadRefused(t *testing.T) {
	c := agentToolServer(t, http.StatusUnprocessableEntity,
		`{"code": "monthly_limit_too_small", "request_id": "req_2", "message": "The monthly limit has to be at least one reload and at most $50,000."}`, nil)
	_, _, err := c.Billing.UpdateAutoReload(context.Background(), &AutoReloadParams{Enabled: true, ThresholdUSD: 5, AmountUSD: 25, MonthlyLimitUSD: 10})
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnprocessableEntity || apiErr.Code != "monthly_limit_too_small" {
		t.Fatalf("err = %v", err)
	}
}

func TestBillingBalanceInvoices(t *testing.T) {
	var got recordedRequest
	c := agentToolServer(t, http.StatusOK, `{"data": [
		{"id": "in_2", "number": "WB-0002", "kind": "auto_reload", "status": "paid", "amount_cents": 2500,
		 "currency": "usd", "created_at": "2026-11-05T09:30:00Z", "hosted_url": "https://pay.example/in_2", "pdf_url": "https://pay.example/in_2.pdf"},
		{"id": "in_1", "number": "WB-0001", "kind": "subscription", "status": "open", "amount_cents": 4900,
		 "currency": "usd", "created_at": "2026-11-01T00:00:00Z"}
	]}`, &got)

	invoices, _, err := c.Billing.BalanceInvoices(context.Background())
	if err != nil {
		t.Fatalf("BalanceInvoices: %v", err)
	}
	checkRequest(t, got, "GET", "/v1/subscription/balance/invoices", "", "")
	if len(invoices) != 2 {
		t.Fatalf("invoices = %d", len(invoices))
	}
	if in := invoices[0]; in.Kind != InvoiceKindAutoReload || in.AmountCents != 2500 || in.PDFURL == "" || in.CreatedAt.IsZero() {
		t.Errorf("invoice 0 = %+v", in)
	}
	if in := invoices[1]; in.Kind != InvoiceKindSubscription || in.HostedURL != "" || in.Status != "open" {
		t.Errorf("invoice 1 = %+v", in)
	}

	c = agentToolServer(t, http.StatusOK, `{"data": []}`, nil)
	invoices, _, err = c.Billing.BalanceInvoices(context.Background())
	if err != nil || invoices == nil || len(invoices) != 0 {
		t.Errorf("empty invoices = %#v, %v", invoices, err)
	}
}
