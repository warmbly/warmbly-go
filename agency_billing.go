package warmbly

import (
	"context"
	"net/url"
)

// Why an agency's automatic top-up turned itself off, in
// [AgencyAutoTopup.Failure] and [AgencyAIAutoTopup.Failure].
const (
	// AgencyAutoTopupNoPaymentMethod: there was no saved card to charge.
	AgencyAutoTopupNoPaymentMethod = "no_payment_method"
	// AgencyAutoTopupCardDeclined: the saved card was declined.
	AgencyAutoTopupCardDeclined = "card_declined"
)

// AgencyCheckoutParams starts the agency's subscription checkout.
type AgencyCheckoutParams struct {
	// PlanID is an agency plan from [AgencyService.Plans]. Required.
	PlanID string `json:"plan_id"`
	// Interval is [AgencyIntervalMonth] (the default) or [AgencyIntervalYear].
	Interval string `json:"interval,omitempty"`
	// SuccessURL and CancelURL are where checkout returns to; empty returns
	// to the dashboard's agency billing page.
	SuccessURL string `json:"success_url,omitempty"`
	CancelURL  string `json:"cancel_url,omitempty"`
}

// AgencyCheckout is a hosted payment page to send the person to.
type AgencyCheckout struct {
	CheckoutURL string `json:"checkout_url"`
}

// AgencyPlanChangePreview is what switching plan or billing period would
// bill. Send ProrationDate and FromPriceID back in [AgencyPlanChangeParams]
// so the switch bills exactly what was shown.
type AgencyPlanChangePreview struct {
	PlanID   string `json:"plan_id"`
	Interval string `json:"interval"`
	// ProrationCents is the prorated difference for the rest of the period;
	// negative is a credit.
	ProrationCents int64 `json:"proration_cents"`
	// AmountDueCents is the next invoice's total with the switch applied.
	AmountDueCents int64  `json:"amount_due_cents"`
	Currency       string `json:"currency"`
	// ProrationDate is the Unix time the amounts were calculated at.
	ProrationDate int64 `json:"proration_date"`
	// FromPriceID is the price the subscription was on.
	FromPriceID string `json:"from_price_id"`
}

// AgencyPlanChangeParams moves the agency's subscription to another plan or
// billing period.
type AgencyPlanChangeParams struct {
	// PlanID is required.
	PlanID string `json:"plan_id"`
	// Interval is [AgencyIntervalMonth] (the default) or [AgencyIntervalYear].
	Interval string `json:"interval,omitempty"`
	// ProrationDate and FromPriceID are the preview's. With them the switch
	// bills what the preview showed; a preview older than 30 minutes is
	// refused ("agency_plan_preview_expired").
	ProrationDate int64  `json:"proration_date,omitempty"`
	FromPriceID   string `json:"from_price_id,omitempty"`
}

// AgencySavedCard is the card automatic top-ups charge.
type AgencySavedCard struct {
	Brand string `json:"brand"`
	Last4 string `json:"last4"`
}

// AgencyAutoTopup is the send pool's automatic top-up: a pack is bought on
// the saved card once ThresholdSends or fewer sends are left, up to MaxPacks
// a period.
type AgencyAutoTopup struct {
	Enabled        bool `json:"enabled"`
	ThresholdSends int  `json:"threshold_sends"`
	MaxPacks       int  `json:"max_packs"`
	// BoughtThisPeriod is how many packs it bought in the current period.
	BoughtThisPeriod int `json:"bought_this_period"`
	// Failure is why it turned itself off: [AgencyAutoTopupNoPaymentMethod]
	// or [AgencyAutoTopupCardDeclined]. Empty while healthy.
	Failure        string `json:"failure,omitempty"`
	PackSends      int    `json:"pack_sends"`
	PackPriceCents int64  `json:"pack_price_cents"`
	// Card is the card a pack is charged to; nil when none is saved or the
	// lookup failed.
	Card *AgencySavedCard `json:"card,omitempty"`
	// CardMissing is true when the lookup succeeded and found no card.
	CardMissing bool `json:"card_missing,omitempty"`
	// Available is false when there is no pool to top up: no plan in force,
	// or a plan with no cap.
	Available bool `json:"available"`
}

// AgencyAutoTopupParams replaces the send pool's automatic top-up settings.
type AgencyAutoTopupParams struct {
	Enabled bool `json:"enabled"`
	// ThresholdSends is the remaining sends that trigger a pack.
	ThresholdSends int `json:"threshold_sends"`
	// MaxPacks bounds the packs bought in one period (at most 20).
	MaxPacks int `json:"max_packs"`
}

// AgencyAIAutoTopup is the shared AI balance's automatic top-up: under
// ThresholdUSD left, AmountUSD is added, never more than LimitUSD a period.
type AgencyAIAutoTopup struct {
	Enabled      bool    `json:"enabled"`
	ThresholdUSD float64 `json:"threshold_usd"`
	AmountUSD    float64 `json:"amount_usd"`
	LimitUSD     float64 `json:"limit_usd"`
	// AddedThisPeriodUSD is what it added in the current period.
	AddedThisPeriodUSD float64 `json:"added_this_period_usd"`
	// Failure is why it turned itself off: [AgencyAutoTopupNoPaymentMethod]
	// or [AgencyAutoTopupCardDeclined].
	Failure     string           `json:"failure,omitempty"`
	Card        *AgencySavedCard `json:"card,omitempty"`
	CardMissing bool             `json:"card_missing,omitempty"`
	// Available is false where there is no shared AI balance to add to: no
	// plan in force, or AI is not charged.
	Available bool `json:"available"`
	// MinUSD and MaxUSD bound AmountUSD.
	MinUSD float64 `json:"min_usd"`
	MaxUSD float64 `json:"max_usd"`
}

// AgencyAIAutoTopupParams replaces the AI balance's automatic top-up
// settings. Amounts are dollars, rounded to cents.
type AgencyAIAutoTopupParams struct {
	Enabled      bool    `json:"enabled"`
	ThresholdUSD float64 `json:"threshold_usd"`
	AmountUSD    float64 `json:"amount_usd"`
	LimitUSD     float64 `json:"limit_usd"`
}

// AgencyTopupParams opens checkout for one send-pool pack.
type AgencyTopupParams struct {
	// SuccessURL and CancelURL are where checkout returns to; empty returns
	// to the dashboard's agency billing page.
	SuccessURL string `json:"success_url,omitempty"`
	CancelURL  string `json:"cancel_url,omitempty"`
}

// AgencyAITopupParams opens checkout for money added to the AI balance.
type AgencyAITopupParams struct {
	// AmountUSD is the amount in dollars, within [AgencyAITopUpLimits].
	AmountUSD  float64 `json:"amount_usd"`
	SuccessURL string  `json:"success_url,omitempty"`
	CancelURL  string  `json:"cancel_url,omitempty"`
}

// Checkout opens a hosted checkout for the agency's subscription. Owner or
// admin. Pass [WithIdempotencyKey] to make a retry safe.
//
// Session only; needs a recent sign-in.
func (s *AgencyService) Checkout(ctx context.Context, agencyID string, params *AgencyCheckoutParams, opts ...RequestOption) (*AgencyCheckout, *Response, error) {
	return send[AgencyCheckout](ctx, s.client.post, agencyPath(agencyID, "billing", "checkout"), params, opts)
}

// PreviewPlanChange prices a switch to planID on interval
// ([AgencyIntervalMonth] or [AgencyIntervalYear]; empty is monthly) without
// making it.
//
// Session only.
func (s *AgencyService) PreviewPlanChange(ctx context.Context, agencyID, planID, interval string, opts ...RequestOption) (*AgencyPlanChangePreview, *Response, error) {
	q := url.Values{"plan_id": {planID}}
	setNonEmpty(q, "interval", interval)
	return fetch[AgencyPlanChangePreview](ctx, s.client, withQuery(agencyPath(agencyID, "billing", "change", "preview"), q), opts)
}

// ChangePlan moves the agency's subscription to another plan or billing
// period and returns the agency. Pass [WithIdempotencyKey] to make a retry
// safe.
//
// Session only; needs a recent sign-in.
func (s *AgencyService) ChangePlan(ctx context.Context, agencyID string, params *AgencyPlanChangeParams, opts ...RequestOption) (*Agency, *Response, error) {
	return send[Agency](ctx, s.client.post, agencyPath(agencyID, "billing", "change"), params, opts)
}

// AutoTopup returns the send pool's automatic top-up settings and the card a
// pack is charged to.
//
// Session only.
func (s *AgencyService) AutoTopup(ctx context.Context, agencyID string, opts ...RequestOption) (*AgencyAutoTopup, *Response, error) {
	return fetch[AgencyAutoTopup](ctx, s.client, agencyPath(agencyID, "billing", "auto-topup"), opts)
}

// SetAutoTopup turns the send pool's automatic top-up on or off and sets its
// limits. Turning it on lets the saved card be charged with nobody present.
//
// Session only; needs a recent sign-in.
func (s *AgencyService) SetAutoTopup(ctx context.Context, agencyID string, params *AgencyAutoTopupParams, opts ...RequestOption) (*AgencyAutoTopup, *Response, error) {
	return send[AgencyAutoTopup](ctx, s.client.put, agencyPath(agencyID, "billing", "auto-topup"), params, opts)
}

// AIAutoTopup returns the shared AI balance's automatic top-up settings.
//
// Session only.
func (s *AgencyService) AIAutoTopup(ctx context.Context, agencyID string, opts ...RequestOption) (*AgencyAIAutoTopup, *Response, error) {
	return fetch[AgencyAIAutoTopup](ctx, s.client, agencyPath(agencyID, "billing", "ai-auto-topup"), opts)
}

// SetAIAutoTopup turns the AI balance's automatic top-up on or off and sets
// its amounts.
//
// Session only; needs a recent sign-in.
func (s *AgencyService) SetAIAutoTopup(ctx context.Context, agencyID string, params *AgencyAIAutoTopupParams, opts ...RequestOption) (*AgencyAIAutoTopup, *Response, error) {
	return send[AgencyAIAutoTopup](ctx, s.client.put, agencyPath(agencyID, "billing", "ai-auto-topup"), params, opts)
}

// Topup opens a hosted checkout for one send-pool pack. Pass
// [WithIdempotencyKey] to make a retry safe.
//
// Session only; needs a recent sign-in.
func (s *AgencyService) Topup(ctx context.Context, agencyID string, params *AgencyTopupParams, opts ...RequestOption) (*AgencyCheckout, *Response, error) {
	return send[AgencyCheckout](ctx, s.client.post, agencyPath(agencyID, "billing", "topup"), params, opts)
}

// AITopup opens a hosted checkout for money added to the shared AI balance.
// Pass [WithIdempotencyKey] to make a retry safe.
//
// Session only; needs a recent sign-in.
func (s *AgencyService) AITopup(ctx context.Context, agencyID string, params *AgencyAITopupParams, opts ...RequestOption) (*AgencyCheckout, *Response, error) {
	return send[AgencyCheckout](ctx, s.client.post, agencyPath(agencyID, "billing", "ai-topup"), params, opts)
}

// BillingPortal opens the payment provider's billing portal for the agency
// and returns its URL. An empty returnURL returns to the dashboard's agency
// billing page.
//
// Session only; needs a recent sign-in.
func (s *AgencyService) BillingPortal(ctx context.Context, agencyID, returnURL string, opts ...RequestOption) (string, *Response, error) {
	var body struct {
		ReturnURL string `json:"return_url,omitempty"`
	}
	body.ReturnURL = returnURL
	var out struct {
		URL string `json:"url"`
	}
	resp, err := s.client.post(ctx, agencyPath(agencyID, "billing", "portal"), &body, &out, opts...)
	if err != nil {
		return "", resp, err
	}
	return out.URL, resp, nil
}
