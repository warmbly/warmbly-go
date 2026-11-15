package warmbly

import (
	"context"
	"net/url"
	"time"
)

// --- private plan offers ---

// PlanOffer is a private plan an operator offered to this workspace, as the
// billing page shows it. Private plans are not in the public catalog
// ([MetaService.Plans]); an offer is the only way to see or buy one.
type PlanOffer struct {
	PlanID   string `json:"plan_id"`
	Name     string `json:"name"`
	Currency string `json:"currency"`
	// MonthlyUSD and YearlyUSD are list prices. YearlyUSD is zero when the
	// plan has no yearly price or billing is off.
	MonthlyUSD float64 `json:"monthly_usd"`
	YearlyUSD  float64 `json:"yearly_usd,omitempty"`
	// MonthlyAvailable and YearlyAvailable report whether a checkout can be
	// opened for that period; Available is true when either can. All are
	// false when billing is off or the plan has no price attached.
	MonthlyAvailable bool `json:"monthly_available"`
	YearlyAvailable  bool `json:"yearly_available"`
	Available        bool `json:"available"`
	// WarmupOnly marks a plan that buys mailbox warmup and nothing else.
	WarmupOnly bool `json:"warmup_only"`
	// ExpiresAt is when the offer lapses; nil means it stays open.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// PlanOfferCheckout is the outcome of [BillingService.CheckoutPlanOffer].
// Exactly one of CheckoutURL and Changed is set.
type PlanOfferCheckout struct {
	// CheckoutURL is the hosted checkout to send the person to.
	CheckoutURL string `json:"checkout_url,omitempty"`
	// Changed is true when the workspace already paid through the provider,
	// so its subscription was switched to the offered plan in place and there
	// is no checkout to open.
	Changed bool `json:"changed,omitempty"`
}

// PlanOffers lists the private plans the workspace has an open offer for. It
// is empty on a deployment without offers. Session-only; any member of the
// current workspace may read it.
func (s *BillingService) PlanOffers(ctx context.Context, opts ...RequestOption) ([]PlanOffer, *Response, error) {
	var out struct {
		Offers []PlanOffer `json:"offers"`
	}
	resp, err := s.client.get(ctx, "subscription/offers", &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	if out.Offers == nil {
		out.Offers = []PlanOffer{}
	}
	return out.Offers, resp, nil
}

// CheckoutPlanOffer buys an offered plan. interval is [DurationMonth] (the
// default when empty) or [DurationYear]. The price comes from the offer on
// the server, never from the caller. A workspace already paying through the
// provider is switched in place ([PlanOfferCheckout.Changed]); otherwise the
// result carries a checkout URL that expires no later than the offer.
//
// It fails with 403 code "plan_offer_required" when the workspace has no open
// offer for planID, and with 503 when the plan has no price for that period.
// Session-only; needs the manage-billing organization permission and a
// recently confirmed session (see [AuthService.Reauth]).
func (s *BillingService) CheckoutPlanOffer(ctx context.Context, planID, interval string, opts ...RequestOption) (*PlanOfferCheckout, *Response, error) {
	body := struct {
		Interval string `json:"interval,omitempty"`
	}{Interval: interval}
	return send[PlanOfferCheckout](ctx, s.client.post, "subscription/offers/"+url.PathEscape(planID)+"/checkout", body, opts)
}

// --- balance ---

// BalanceState is everything the balance page shows: what is left, the plan
// allowance, the saved card and automatic reload. Amounts are US dollars.
//
// Check Unlimited first: a deployment that meters nothing answers with only
// that field set.
type BalanceState struct {
	Unlimited bool `json:"unlimited"`
	// CanAddMoney is false until the workspace is on a paid plan; topping up
	// and turning on automatic reload are refused until then.
	CanAddMoney bool `json:"can_add_money"`

	// BalanceUSD is the total left: MonthlyUSD (this period's allowance
	// still unspent) plus PurchasedUSD (money added, which does not reset).
	BalanceUSD          float64 `json:"balance_usd"`
	MonthlyUSD          float64 `json:"monthly_usd"`
	PurchasedUSD        float64 `json:"purchased_usd"`
	MonthlyAllowanceUSD float64 `json:"monthly_allowance_usd"`
	MonthlyUsedUSD      float64 `json:"monthly_used_usd"`
	// NextResetAt is when the monthly allowance refills; nil without a
	// billing period.
	NextResetAt *time.Time `json:"next_reset_at,omitempty"`
	// AgencyUSD is what the workspace's agency balance would add on top;
	// zero when there is none.
	AgencyUSD float64 `json:"agency_usd,omitempty"`

	// TopUp holds the bounds for [BillingService.TopUpBalance] and
	// [BillingService.BalanceCheckout].
	TopUp *BalanceTopUpBounds `json:"top_up,omitempty"`

	// AutoReload, LowBalanceUSD, PaymentMethod and ChargeInProgress are
	// absent on a deployment without card payments. PaymentMethod is nil
	// when no card is saved.
	AutoReload    *AutoReloadState      `json:"auto_reload,omitempty"`
	LowBalanceUSD float64               `json:"low_balance_usd,omitempty"`
	PaymentMethod *PaymentMethodSummary `json:"payment_method,omitempty"`
	// ChargeInProgress is true while a card payment is still going through;
	// a second top-up is refused with 409 "charge_in_progress" until it
	// settles.
	ChargeInProgress bool `json:"charge_in_progress,omitempty"`
}

// BalanceTopUpBounds are the amounts a top-up may take, in US dollars.
type BalanceTopUpBounds struct {
	MinUSD float64 `json:"min_usd"`
	MaxUSD float64 `json:"max_usd"`
	// PresetsUSD are the amounts the dashboard offers as one-click choices,
	// and DefaultUSD the one it selects.
	PresetsUSD []float64 `json:"presets_usd"`
	DefaultUSD float64   `json:"default_usd"`
}

// AutoReloadState is the automatic reload configuration and how it has been
// going.
type AutoReloadState struct {
	Enabled bool `json:"enabled"`
	// ThresholdUSD is the balance that triggers a reload of AmountUSD;
	// MonthlyLimitUSD caps the reloads in one calendar month.
	ThresholdUSD    float64 `json:"threshold_usd"`
	AmountUSD       float64 `json:"amount_usd"`
	MonthlyLimitUSD float64 `json:"monthly_limit_usd"`
	// ReloadedThisMonthUSD is what reloads have charged this month.
	ReloadedThisMonthUSD float64 `json:"reloaded_this_month_usd"`
	// FailureCode is set after a reload failed, one of the ReloadFailure*
	// constants, with FailedAt when it happened.
	FailureCode *string    `json:"failure_code,omitempty"`
	FailedAt    *time.Time `json:"failed_at,omitempty"`
	// LastReload is the most recent successful reload; nil when none.
	LastReload *AutoReloadCharge `json:"last_reload,omitempty"`
	// RetryAt is when a softly declined reload is tried again, and
	// RetryReason the failure code that deferred it.
	RetryAt     *time.Time `json:"retry_at,omitempty"`
	RetryReason *string    `json:"retry_reason,omitempty"`
}

// AutoReloadCharge is one completed automatic reload.
type AutoReloadCharge struct {
	AmountUSD float64    `json:"amount_usd"`
	At        *time.Time `json:"at,omitempty"`
}

// Failure codes in [AutoReloadState.FailureCode] and
// [AutoReloadState.RetryReason]. "card_declined" and
// "authentication_required" are also the codes of a 402 from
// [BillingService.TopUpBalance]. Other values may appear.
const (
	ReloadFailureCardDeclined           = "card_declined"
	ReloadFailureAuthenticationRequired = "authentication_required"
	ReloadFailureNoPaymentMethod        = "no_payment_method"
	ReloadFailurePaymentFailed          = "payment_failed"
	ReloadFailureUnavailable            = "unavailable"
)

// PaymentMethodSummary is the saved card, as safe to display.
type PaymentMethodSummary struct {
	Brand    string `json:"brand"`
	Last4    string `json:"last4"`
	ExpMonth int    `json:"exp_month"`
	ExpYear  int    `json:"exp_year"`
	// Wallet names a wallet the card is used through, such as "apple_pay";
	// empty for a plain card.
	Wallet string `json:"wallet,omitempty"`
}

// Outcomes of a top-up, in [BalanceTopUp.Status]. Other values may appear.
const (
	// BalanceTopUpPaid means the money is in the balance (HTTP 200).
	BalanceTopUpPaid = "paid"
	// BalanceTopUpProcessing means the payment is still going through (HTTP
	// 202); the balance updates when it settles.
	BalanceTopUpProcessing = "processing"
)

// BalanceTopUp is the result of [BillingService.TopUpBalance].
type BalanceTopUp struct {
	// Status is one of the BalanceTopUp* constants.
	Status    string  `json:"status"`
	AmountUSD float64 `json:"amount_usd"`
	// BalanceUSD is the balance after the charge, or before it while it is
	// still processing.
	BalanceUSD       float64 `json:"balance_usd"`
	ChargeInProgress bool    `json:"charge_in_progress"`
}

// AutoReloadParams replaces the automatic reload configuration whole: every
// field is sent. AmountUSD must be above ThresholdUSD (422
// "recharge_too_small" otherwise) and MonthlyLimitUSD at least AmountUSD (422
// "monthly_limit_too_small" otherwise). The limits are checked even when
// Enabled is false.
type AutoReloadParams struct {
	Enabled bool `json:"enabled"`
	// ThresholdUSD is from 0.01 to 1,000.
	ThresholdUSD float64 `json:"threshold_usd"`
	// AmountUSD is from 5 to 5,000.
	AmountUSD float64 `json:"amount_usd"`
	// MonthlyLimitUSD is at most 50,000.
	MonthlyLimitUSD float64 `json:"monthly_limit_usd"`
}

// Kinds of invoice in [BalanceInvoice.Kind]. Other values may appear.
const (
	InvoiceKindSubscription = "subscription"
	InvoiceKindTopUp        = "top_up"
	InvoiceKindAutoReload   = "auto_reload"
)

// BalanceInvoice is one invoice from the payment provider, for a
// subscription period or money added to the balance.
type BalanceInvoice struct {
	ID     string `json:"id"`
	Number string `json:"number"`
	// Kind is one of the InvoiceKind* constants.
	Kind string `json:"kind"`
	// Status is the provider's invoice status, for example "paid" or "open".
	Status string `json:"status"`
	// AmountCents is the total in the smallest unit of Currency.
	AmountCents int64     `json:"amount_cents"`
	Currency    string    `json:"currency"`
	CreatedAt   time.Time `json:"created_at"`
	// HostedURL and PDFURL are the provider's invoice page and PDF, when it
	// has them.
	HostedURL string `json:"hosted_url,omitempty"`
	PDFURL    string `json:"pdf_url,omitempty"`
}

// Balance returns the workspace's balance, saved card and automatic reload
// state. Session-only; needs the manage-billing organization permission.
func (s *BillingService) Balance(ctx context.Context, opts ...RequestOption) (*BalanceState, *Response, error) {
	return fetch[BalanceState](ctx, s.client, "subscription/balance", opts)
}

// TopUpBalance charges the saved card amountUSD (rounded to whole cents,
// from 5 to 5,000) and adds it to the balance. A charge still going through
// comes back with [BalanceTopUpProcessing] and HTTP 202 rather than an
// error.
//
// It fails with 402 and code "card_declined" or "authentication_required"
// when the card was not charged (offer [BillingService.BalanceCheckout]
// instead), 409 "payment_method_required" with no saved card, 409
// "charge_in_progress" while another payment settles, and 403 without a paid
// plan. Pass [WithIdempotencyKey] to make a retry safe. Session-only; needs
// the manage-billing organization permission and a recently confirmed
// session (see [AuthService.Reauth]).
func (s *BillingService) TopUpBalance(ctx context.Context, amountUSD float64, opts ...RequestOption) (*BalanceTopUp, *Response, error) {
	body := struct {
		AmountUSD float64 `json:"amount_usd"`
	}{AmountUSD: amountUSD}
	return send[BalanceTopUp](ctx, s.client.post, "subscription/balance/top-up", body, opts)
}

// BalanceCheckout opens a hosted checkout that adds amountUSD (from 5 to
// 5,000) to the balance and saves the card for later charges. The money lands
// once the provider confirms the payment. It needs a paid plan (403
// otherwise). Session-only; needs the manage-billing organization permission
// and a recently confirmed session (see [AuthService.Reauth]).
func (s *BillingService) BalanceCheckout(ctx context.Context, amountUSD float64, opts ...RequestOption) (*CheckoutSession, *Response, error) {
	body := struct {
		AmountUSD float64 `json:"amount_usd"`
	}{AmountUSD: amountUSD}
	return send[CheckoutSession](ctx, s.client.post, "subscription/balance/checkout", body, opts)
}

// SetupPaymentMethod opens a hosted checkout in setup mode, which saves or
// replaces the card used for top-ups and automatic reload without charging
// it. Session-only; needs the manage-billing organization permission and a
// recently confirmed session (see [AuthService.Reauth]).
func (s *BillingService) SetupPaymentMethod(ctx context.Context, opts ...RequestOption) (*CheckoutSession, *Response, error) {
	return send[CheckoutSession](ctx, s.client.post, "subscription/balance/payment-method", nil, opts)
}

// UpdateAutoReload replaces the automatic reload configuration and returns
// the balance state that results. Turning it on needs a paid plan (403) and a
// saved card (409 "payment_method_required"). Session-only; needs the
// manage-billing organization permission and a recently confirmed session
// (see [AuthService.Reauth]).
func (s *BillingService) UpdateAutoReload(ctx context.Context, params *AutoReloadParams, opts ...RequestOption) (*BalanceState, *Response, error) {
	return send[BalanceState](ctx, s.client.patch, "subscription/balance/auto-reload", params, opts)
}

// BalanceInvoices returns the workspace's most recent invoices (up to 24),
// subscription and balance alike, newest first. Drafts and voided invoices
// are left out. It is not paginated. Session-only; needs the manage-billing
// organization permission.
func (s *BillingService) BalanceInvoices(ctx context.Context, opts ...RequestOption) ([]BalanceInvoice, *Response, error) {
	return fetchData[BalanceInvoice](ctx, s.client, "subscription/balance/invoices", opts)
}
