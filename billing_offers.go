package warmbly

import (
	"context"
	"net/url"
	"time"
)

// PlanOffer is a private plan an operator offered to this workspace, as the
// billing page shows it. Private plans are not in the public catalog
// ([MetaService.Plans]); an offer is the only way to see or buy one.
type PlanOffer struct {
	PlanID   string `json:"plan_id"`
	Name     string `json:"name"`
	Currency string `json:"currency"`
	// MonthlyUSD and YearlyUSD are list prices; YearlyUSD is zero when the
	// plan has no yearly price.
	MonthlyUSD float32 `json:"monthly_usd"`
	YearlyUSD  float32 `json:"yearly_usd,omitempty"`
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
type PlanOfferCheckout struct {
	// CheckoutURL is the hosted checkout to send the person to. Empty when
	// Changed is true.
	CheckoutURL string `json:"checkout_url,omitempty"`
	// Changed is true when the workspace already paid through the provider,
	// so its subscription was switched to the offered plan in place and there
	// is no checkout to open.
	Changed bool `json:"changed,omitempty"`
}

// PlanOffers lists the private plans the workspace currently has an open offer
// for (the "offers" array of the response). It is empty on a deployment
// without offers. Session-only; the session needs a current workspace.
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
// default when empty) or [DurationYear]. The price is resolved on the server
// from the offer, never from the caller. A workspace already paying through
// the provider is switched in place ([PlanOfferCheckout.Changed]); otherwise
// the result carries the checkout URL, which expires with the offer.
//
// It fails with code "plan_offer_required" (403) when the workspace has no open offer for
// planID and with 503 when the plan has no price for that period yet.
// Session-only; needs the manage-billing organization permission and a recent
// confirmation ([AuthService.Reauth]). Recorded in the audit log.
func (s *BillingService) CheckoutPlanOffer(ctx context.Context, planID, interval string, opts ...RequestOption) (*PlanOfferCheckout, *Response, error) {
	body := struct {
		Interval string `json:"interval,omitempty"`
	}{Interval: interval}
	return send[PlanOfferCheckout](ctx, s.client.post, "subscription/offers/"+url.PathEscape(planID)+"/checkout", body, opts)
}
