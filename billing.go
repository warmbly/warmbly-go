package warmbly

import (
	"context"
	"net/url"
	"time"
)

// BillingService reads and changes the workspace's subscription, plan, AI
// credit balance and spend controls, and the referral program.
//
// These routes are session-only and gated on the manage-billing permission, so
// they need a session token rather than an API key. On a self-hosted deployment
// with billing disabled they may be absent entirely.
type BillingService service

// Subscription is the workspace's current plan subscription.
type Subscription struct {
	ID             string `json:"id"`
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id"`
	PlanID         string `json:"plan_id"`

	StripeCustomerID     string  `json:"stripe_customer_id,omitempty"`
	StripeSubscriptionID *string `json:"stripe_subscription_id,omitempty"`
	StripePriceID        *string `json:"stripe_price_id,omitempty"`

	// Status is the lifecycle state, for example "active", "trialing" or
	// "canceled".
	Status string `json:"status"`

	CurrentPeriodStart *time.Time `json:"current_period_start,omitempty"`
	CurrentPeriodEnd   *time.Time `json:"current_period_end,omitempty"`
	// CancelAtPeriodEnd is true when the subscription is set to lapse rather
	// than renew.
	CancelAtPeriodEnd bool       `json:"cancel_at_period_end"`
	CanceledAt        *time.Time `json:"canceled_at,omitempty"`

	TrialStart *time.Time `json:"trial_start,omitempty"`
	TrialEnd   *time.Time `json:"trial_end,omitempty"`

	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// TrialStatus is where the workspace stands in its free trial.
type TrialStatus struct {
	IsInTrial     bool       `json:"is_in_trial"`
	TrialEndsAt   *time.Time `json:"trial_ends_at,omitempty"`
	DaysRemaining int        `json:"days_remaining"`
	IsExpired     bool       `json:"is_expired"`
	IsSubscribed  bool       `json:"is_subscribed"`
}

// SubscriptionStatus is the plan-gate view: what the workspace is entitled to.
type SubscriptionStatus struct {
	HasSubscription    bool  `json:"has_subscription"`
	IsInFreeTrial      bool  `json:"is_in_free_trial"`
	IsFreeTrialExpired bool  `json:"is_free_trial_expired"`
	IsPaidSubscriber   bool  `json:"is_paid_subscriber"`
	DailyEmailLimit    int   `json:"daily_email_limit"`
	Plan               *Plan `json:"plan,omitempty"`
}

// FeatureStatus is the subscription status plus the specific capability gates a
// client should check before offering an action.
type FeatureStatus struct {
	Subscription     *SubscriptionStatus `json:"subscription"`
	CanSendCampaigns bool                `json:"can_send_campaigns"`
	CanUseWarmup     bool                `json:"can_use_warmup"`
	CanUseUnibox     bool                `json:"can_use_unibox"`
}

// CheckoutSession is a hosted checkout to send the user to.
type CheckoutSession struct {
	SessionID   string `json:"session_id"`
	CheckoutURL string `json:"checkout_url"`
}

// CheckoutParams starts a plan checkout.
type CheckoutParams struct {
	// PriceID is the payment-provider price to buy.
	PriceID string `json:"price_id"`
	// SuccessURL and CancelURL are where the provider returns the user.
	SuccessURL string `json:"success_url,omitempty"`
	CancelURL  string `json:"cancel_url,omitempty"`
	// DiscountCode applies a promotion at checkout.
	DiscountCode string `json:"discount_code,omitempty"`
}

// ChangePlanParams moves the workspace to a different plan.
type ChangePlanParams struct {
	PlanID string `json:"plan_id"`
	// ProrationBehavior controls how the provider settles the switch, for
	// example "create_prorations".
	ProrationBehavior string `json:"proration_behavior,omitempty"`
	DiscountCode      string `json:"discount_code,omitempty"`
	// Interval is [DurationMonth] or [DurationYear].
	Interval string `json:"interval,omitempty"`
}

// CreditBalance is the workspace's AI credit position across both pools: the
// monthly allowance that resets, and purchased credits that do not.
type CreditBalance struct {
	// Balance is the total spendable amount across both pools.
	Balance          int `json:"balance"`
	MonthlyBalance   int `json:"monthly_balance"`
	PurchasedBalance int `json:"purchased_balance"`
	// MonthlyAllowance is what the plan grants each month.
	MonthlyAllowance int `json:"monthly_allowance"`
	TotalPurchased   int `json:"total_purchased"`
	// MonthlyResetAt is when the monthly pool refills; NextResetAt is the end
	// of the billing period.
	MonthlyResetAt *time.Time `json:"monthly_reset_at,omitempty"`
	NextResetAt    *time.Time `json:"next_reset_at,omitempty"`
	// Packs are the top-up bundles available for purchase.
	Packs []CreditPack `json:"packs,omitempty"`
}

// CreditPack is a purchasable top-up bundle.
type CreditPack struct {
	Key     string `json:"key"`
	Credits int    `json:"credits"`
	// PriceCents is the pack's price in the smallest currency unit.
	PriceCents int    `json:"price_cents,omitempty"`
	Currency   string `json:"currency,omitempty"`
	Label      string `json:"label,omitempty"`
}

// CreditContext names what a charge was actually for.
type CreditContext struct {
	Detail     string `json:"detail,omitempty"`
	ThreadID   string `json:"thread_id,omitempty"`
	CampaignID string `json:"campaign_id,omitempty"`
	ContactID  string `json:"contact_id,omitempty"`
}

// CreditTransaction is one row of the append-only credit ledger. Amount is
// negative for spend and positive for grants and purchases.
type CreditTransaction struct {
	ID     string `json:"id"`
	OrgID  string `json:"org_id"`
	Amount int    `json:"amount"`
	// Reason names the feature that spent, for example "reply_draft".
	Reason     string `json:"reason"`
	ModelUsed  string `json:"model_used,omitempty"`
	TokensUsed int    `json:"tokens_used"`
	// BalanceAfter is the resulting total, captured atomically with the
	// charge.
	BalanceAfter int `json:"balance_after"`
	// PurchasedDelta and PurchasedBalanceAfter track the purchased pool
	// separately, so the log reconstructs both.
	PurchasedDelta        int     `json:"purchased_delta"`
	PurchasedBalanceAfter int     `json:"purchased_balance_after"`
	IdempotencyKey        *string `json:"idempotency_key,omitempty"`
	// ActorUserID is the member who triggered the charge; it is nil for
	// scheduled or system work.
	ActorUserID *string       `json:"actor_user_id,omitempty"`
	Context     CreditContext `json:"context"`
	CreatedAt   time.Time     `json:"created_at"`
}

// CreditUsage is AI spend over a window, with the daily series and breakdowns
// by feature and model.
type CreditUsage struct {
	SpentToday int `json:"spent_today"`
	SpentWeek  int `json:"spent_week"`
	SpentMonth int `json:"spent_month"`

	// LimitDaily, LimitWeekly and LimitMonthly are the configured spend caps.
	// A nil value means no cap for that window.
	LimitDaily   *int `json:"limit_daily"`
	LimitWeekly  *int `json:"limit_weekly"`
	LimitMonthly *int `json:"limit_monthly"`

	Series   []CreditUsagePoint  `json:"series"`
	ByReason []CreditUsageBucket `json:"by_reason"`
	ByModel  []CreditUsageBucket `json:"by_model"`
}

// CreditUsagePoint is one day of AI spend.
type CreditUsagePoint struct {
	// Date is a UTC calendar day formatted YYYY-MM-DD.
	Date    string `json:"date"`
	Credits int    `json:"credits"`
	Tokens  int    `json:"tokens"`
}

// CreditUsageBucket is one slice of a usage breakdown.
type CreditUsageBucket struct {
	Key     string `json:"key"`
	Credits int    `json:"credits"`
	Tokens  int    `json:"tokens"`
	// Count is how many charges landed in this bucket.
	Count int `json:"count"`
}

// CreditSettings are the workspace's AI spend controls.
type CreditSettings struct {
	OrgID string `json:"org_id"`

	// SpendLimit* cap workspace-wide spend per window. A nil value means no
	// limit.
	SpendLimitDaily   *int `json:"spend_limit_daily"`
	SpendLimitWeekly  *int `json:"spend_limit_weekly"`
	SpendLimitMonthly *int `json:"spend_limit_monthly"`

	// MemberLimit* cap what one member can spend per window. Scheduled and
	// system work is not counted against anyone.
	MemberLimitDaily   *int `json:"member_limit_daily"`
	MemberLimitWeekly  *int `json:"member_limit_weekly"`
	MemberLimitMonthly *int `json:"member_limit_monthly"`

	// LowBalanceThreshold is the balance at which an alert fires.
	LowBalanceThreshold  int        `json:"low_balance_threshold"`
	LowBalanceNotifiedAt *time.Time `json:"low_balance_notified_at,omitempty"`

	// AutoTopup buys AutoTopupPack whenever the balance falls below
	// AutoTopupThreshold, at most AutoTopupMaxPerMonth times a month.
	AutoTopupEnabled     bool   `json:"auto_topup_enabled"`
	AutoTopupPack        string `json:"auto_topup_pack"`
	AutoTopupThreshold   int    `json:"auto_topup_threshold"`
	AutoTopupMaxPerMonth int    `json:"auto_topup_max_per_month"`

	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// CreditSettingsParams updates the spend controls. An omitted limit disables
// that limit.
type CreditSettingsParams struct {
	SpendLimitDaily   *int `json:"spend_limit_daily,omitempty"`
	SpendLimitWeekly  *int `json:"spend_limit_weekly,omitempty"`
	SpendLimitMonthly *int `json:"spend_limit_monthly,omitempty"`

	MemberLimitDaily   *int `json:"member_limit_daily,omitempty"`
	MemberLimitWeekly  *int `json:"member_limit_weekly,omitempty"`
	MemberLimitMonthly *int `json:"member_limit_monthly,omitempty"`

	LowBalanceThreshold  int    `json:"low_balance_threshold,omitempty"`
	AutoTopupEnabled     bool   `json:"auto_topup_enabled,omitempty"`
	AutoTopupPack        string `json:"auto_topup_pack,omitempty"`
	AutoTopupThreshold   int    `json:"auto_topup_threshold,omitempty"`
	AutoTopupMaxPerMonth int    `json:"auto_topup_max_per_month,omitempty"`
}

// ReferralSummary is the workspace's referral position.
type ReferralSummary struct {
	Code     string `json:"code"`
	ShareURL string `json:"share_url"`
	Currency string `json:"currency"`
	// InviteePercentOff and InviteeMonths are what someone signing up through
	// the link receives.
	InviteePercentOff int `json:"invitee_percent_off"`
	InviteeMonths     int `json:"invitee_months"`

	// BalanceCents is the credit available now; LifetimeEarnedCents is the
	// running total.
	BalanceCents        int64 `json:"balance_cents"`
	LifetimeEarnedCents int64 `json:"lifetime_earned_cents"`

	TotalReferred int `json:"total_referred"`
	Pending       int `json:"pending"`
	Qualified     int `json:"qualified"`
	Rewarded      int `json:"rewarded"`
}

// ReferralAttribution is one referred workspace and where its reward stands.
type ReferralAttribution struct {
	ID             string  `json:"id"`
	ReferralCodeID *string `json:"referral_code_id,omitempty"`
	ReferrerUserID string  `json:"referrer_user_id"`
	ReferrerOrgID  string  `json:"referrer_org_id"`
	InviteeOrgID   string  `json:"invitee_org_id"`
	InviteeUserID  *string `json:"invitee_user_id,omitempty"`
	// Status is the reward's state, for example "pending", "qualified",
	// "rewarded" or "voided".
	Status         string `json:"status"`
	RewardCents    int64  `json:"reward_cents"`
	RewardCurrency string `json:"reward_currency"`

	QualifiedAt *time.Time `json:"qualified_at,omitempty"`
	RewardedAt  *time.Time `json:"rewarded_at,omitempty"`
	VoidedAt    *time.Time `json:"voided_at,omitempty"`
	VoidReason  *string    `json:"void_reason,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ReferralEarning is one movement in the referral credit ledger.
type ReferralEarning struct {
	ID            string `json:"id"`
	AttributionID string `json:"attribution_id,omitempty"`
	AmountCents   int64  `json:"amount_cents"`
	Currency      string `json:"currency"`
	// Kind is what moved the balance, for example "reward" or "clawback".
	Kind        string    `json:"kind,omitempty"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// EnterpriseInquiryParams asks the sales team to get in touch.
type EnterpriseInquiryParams struct {
	CompanyName  string `json:"company_name"`
	ContactName  string `json:"contact_name,omitempty"`
	ContactEmail string `json:"contact_email,omitempty"`
	// EstimatedVolume is expected monthly send volume.
	EstimatedVolume *int   `json:"estimated_volume,omitempty"`
	TeamSize        *int   `json:"team_size,omitempty"`
	Notes           string `json:"notes,omitempty"`
}

// Get returns the workspace's subscription.
func (s *BillingService) Get(ctx context.Context, opts ...RequestOption) (*Subscription, *Response, error) {
	return fetch[Subscription](ctx, s.client, "subscription", opts)
}

// Limits returns the plan-gate view of what the workspace is entitled to.
func (s *BillingService) Limits(ctx context.Context, opts ...RequestOption) (*SubscriptionStatus, *Response, error) {
	return fetch[SubscriptionStatus](ctx, s.client, "subscription/limits", opts)
}

// Trial returns where the workspace stands in its free trial.
func (s *BillingService) Trial(ctx context.Context, opts ...RequestOption) (*TrialStatus, *Response, error) {
	return fetch[TrialStatus](ctx, s.client, "subscription/trial", opts)
}

// Features returns the capability gates a client should check before offering
// an action.
func (s *BillingService) Features(ctx context.Context, opts ...RequestOption) (*FeatureStatus, *Response, error) {
	return fetch[FeatureStatus](ctx, s.client, "subscription/features", opts)
}

// Checkout starts a hosted checkout for a plan and returns the URL to send the
// user to.
func (s *BillingService) Checkout(ctx context.Context, params *CheckoutParams, opts ...RequestOption) (*CheckoutSession, *Response, error) {
	return send[CheckoutSession](ctx, s.client.post, "subscription/checkout", params, opts)
}

// Portal opens the payment provider's billing portal and returns its URL.
func (s *BillingService) Portal(ctx context.Context, returnURL string, opts ...RequestOption) (string, *Response, error) {
	body := struct {
		ReturnURL string `json:"return_url,omitempty"`
	}{ReturnURL: returnURL}
	var out struct {
		PortalURL string `json:"portal_url"`
	}
	resp, err := s.client.post(ctx, "subscription/portal", body, &out, opts...)
	if err != nil {
		return "", resp, err
	}
	return out.PortalURL, resp, nil
}

// Cancel ends the subscription at the end of the current period.
func (s *BillingService) Cancel(ctx context.Context, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "subscription/cancel", nil, nil, opts...)
}

// ChangePlan moves the workspace to a different plan.
func (s *BillingService) ChangePlan(ctx context.Context, params *ChangePlanParams, opts ...RequestOption) (*Subscription, *Response, error) {
	var out struct {
		Subscription *Subscription `json:"subscription"`
	}
	resp, err := s.client.post(ctx, "subscription/change-plan", params, &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Subscription, resp, nil
}

// PreviewPlanChange returns the proration a plan change would produce, without
// making it.
func (s *BillingService) PreviewPlanChange(ctx context.Context, newPlanID string, opts ...RequestOption) (map[string]any, *Response, error) {
	q := url.Values{"new_plan_id": {newPlanID}}
	var out map[string]any
	resp, err := s.client.get(ctx, withQuery("subscription/preview-change", q), &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// ValidateDiscount checks a promotion code and reports what it would do.
func (s *BillingService) ValidateDiscount(ctx context.Context, code string, opts ...RequestOption) (map[string]any, *Response, error) {
	body := struct {
		Code string `json:"code"`
	}{Code: code}
	var out map[string]any
	resp, err := s.client.post(ctx, "subscription/discount/validate", body, &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// AppliedDiscounts returns the promotions redeemed on the workspace.
func (s *BillingService) AppliedDiscounts(ctx context.Context, opts ...RequestOption) ([]map[string]any, *Response, error) {
	return fetchData[map[string]any](ctx, s.client, "subscription/discounts", opts)
}

// EnterpriseInquiry asks the sales team to get in touch about enterprise
// pricing.
func (s *BillingService) EnterpriseInquiry(ctx context.Context, params *EnterpriseInquiryParams, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "subscription/enterprise-inquiry", params, nil, opts...)
}

// --- AI credits ---

// Credits returns the workspace's AI credit position.
func (s *BillingService) Credits(ctx context.Context, opts ...RequestOption) (*CreditBalance, *Response, error) {
	return fetch[CreditBalance](ctx, s.client, "subscription/credits", opts)
}

// CreditTransactions returns a page of the credit ledger, newest first.
func (s *BillingService) CreditTransactions(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[CreditTransaction], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[CreditTransaction](ctx, s.client, "subscription/credits/transactions", q, opts...)
}

// BuyCredits starts a hosted checkout for a top-up pack. It requires an active
// paid plan.
func (s *BillingService) BuyCredits(ctx context.Context, pack, successURL, cancelURL string, opts ...RequestOption) (*CheckoutSession, *Response, error) {
	body := struct {
		Pack       string `json:"pack"`
		SuccessURL string `json:"success_url,omitempty"`
		CancelURL  string `json:"cancel_url,omitempty"`
	}{Pack: pack, SuccessURL: successURL, CancelURL: cancelURL}
	return send[CheckoutSession](ctx, s.client.post, "subscription/credits/checkout", body, opts)
}

// CreditUsage returns AI spend over the last days, from 1 to 90. Zero uses the
// server default.
func (s *BillingService) CreditUsage(ctx context.Context, days int, opts ...RequestOption) (*CreditUsage, *Response, error) {
	q := make(url.Values)
	setPositive(q, "days", days)
	return fetch[CreditUsage](ctx, s.client, withQuery("subscription/credits/usage", q), opts)
}

// CreditSettings returns the workspace's AI spend controls.
func (s *BillingService) CreditSettings(ctx context.Context, opts ...RequestOption) (*CreditSettings, *Response, error) {
	return fetch[CreditSettings](ctx, s.client, "subscription/credits/settings", opts)
}

// UpdateCreditSettings saves the AI spend controls.
func (s *BillingService) UpdateCreditSettings(ctx context.Context, params *CreditSettingsParams, opts ...RequestOption) (*CreditSettings, *Response, error) {
	return send[CreditSettings](ctx, s.client.patch, "subscription/credits/settings", params, opts)
}

// --- referrals ---

// Referral returns the workspace's referral summary.
func (s *BillingService) Referral(ctx context.Context, opts ...RequestOption) (*ReferralSummary, *Response, error) {
	return fetch[ReferralSummary](ctx, s.client, "subscription/referral", opts)
}

// EnsureReferralCode mints the workspace's referral code if it does not have
// one yet, and returns it either way.
func (s *BillingService) EnsureReferralCode(ctx context.Context, opts ...RequestOption) (*ReferralSummary, *Response, error) {
	return send[ReferralSummary](ctx, s.client.post, "subscription/referral", nil, opts)
}

// ReferralAttributions returns a page of the workspaces referred, with where
// each reward stands.
func (s *BillingService) ReferralAttributions(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[ReferralAttribution], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[ReferralAttribution](ctx, s.client, "subscription/referral/attributions", q, opts...)
}

// ReferralEarnings returns a page of the referral credit ledger.
func (s *BillingService) ReferralEarnings(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[ReferralEarning], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[ReferralEarning](ctx, s.client, "subscription/referral/earnings", q, opts...)
}
