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

	// FreeTrialStartedAt and FreeTrialEndsAt are Warmbly's own free trial,
	// distinct from a provider-side trial.
	FreeTrialStartedAt *time.Time `json:"free_trial_started_at,omitempty"`
	FreeTrialEndsAt    *time.Time `json:"free_trial_ends_at,omitempty"`

	// IsEnterprise marks a subscription negotiated outside the plan catalog.
	IsEnterprise bool `json:"is_enterprise"`
	// Plan is the plan this subscription is on, joined in by the API so a
	// caller does not have to look it up in [MetaService.Plans].
	Plan *Plan `json:"plan,omitempty"`

	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// SubscriptionWithLimits is the subscription together with the realtime
// gateway ceilings its plan carries.
type SubscriptionWithLimits struct {
	Subscription
	RateLimits *RealtimeRateLimits `json:"rate_limits,omitempty"`
}

// RealtimeRateLimits are the plan's websocket ceilings: messages, channel
// joins and events per minute, and concurrent connections.
type RealtimeRateLimits struct {
	LimitWSMessagePM int `json:"limit_ws_message_pm"`
	LimitWSJoinPM    int `json:"limit_ws_join_pm"`
	LimitWSEventPM   int `json:"limit_ws_event_pm"`
	MaxConnections   int `json:"max_connections"`
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
	HasSubscription    bool `json:"has_subscription"`
	IsInFreeTrial      bool `json:"is_in_free_trial"`
	IsFreeTrialExpired bool `json:"is_free_trial_expired"`
	IsPaidSubscriber   bool `json:"is_paid_subscriber"`
	// DailyEmailLimit is the send cap the server enforces: an approved limit
	// increase, else the plan's, else the trial's.
	DailyEmailLimit int   `json:"daily_email_limit"`
	Plan            *Plan `json:"plan,omitempty"`
}

// FeatureStatus is the subscription status plus the specific capability gates a
// client should check before offering an action. Every workspace may warm its
// mailboxes, so CanUseWarmup is only false where sending is blocked outright.
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

// CheckoutParams starts a plan checkout. PriceID, SuccessURL and CancelURL
// are required.
type CheckoutParams struct {
	// PriceID is the payment-provider price to buy.
	PriceID string `json:"price_id"`
	// SuccessURL and CancelURL are where the provider returns the user.
	SuccessURL string `json:"success_url"`
	CancelURL  string `json:"cancel_url"`
	// DiscountCode applies a promotion at checkout.
	DiscountCode string `json:"discount_code,omitempty"`
}

// Proration behaviors accepted by [ChangePlanParams.ProrationBehavior].
const (
	ProrationCreate        = "create_prorations"
	ProrationAlwaysInvoice = "always_invoice"
	ProrationNone          = "none"
)

// ChangePlanParams moves the workspace to a different plan.
type ChangePlanParams struct {
	PlanID string `json:"plan_id"`
	// ProrationBehavior controls how the provider settles the switch: one of
	// the Proration* constants.
	ProrationBehavior string `json:"proration_behavior,omitempty"`
	DiscountCode      string `json:"discount_code,omitempty"`
	// Interval is [DurationMonth] or [DurationYear].
	Interval string `json:"interval,omitempty"`
}

// CreditBalance is the workspace's AI credit position across both pools: the
// monthly allowance that resets, and purchased credits that do not.
type CreditBalance struct {
	// Unlimited is true on a deployment with no billing provider: AI is not
	// metered there, every numeric field is zero and Packs is empty. Check it
	// before rendering a balance as "0 left".
	Unlimited bool `json:"unlimited"`
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

// ReferralCode is the workspace's own referral code, as returned by
// [BillingService.EnsureReferralCode].
type ReferralCode struct {
	ID          string `json:"id"`
	OwnerUserID string `json:"owner_user_id"`
	OwnerOrgID  string `json:"owner_org_id"`
	// Code is the token that goes in a share link's ?ref= parameter and in
	// [LoginParams.ReferralCode].
	Code string `json:"code"`
	// DiscountCodeID is the promotion an invitee redeems by signing up with
	// the code, when the deployment attaches one.
	DiscountCodeID *string   `json:"discount_code_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
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

// Reward states returned in [ReferralAttribution.Status].
const (
	// ReferralStatusPending means the invitee signed up but has not converted
	// yet.
	ReferralStatusPending = "pending"
	// ReferralStatusQualified means the conversion counts; the reward is owed.
	ReferralStatusQualified = "qualified"
	// ReferralStatusRewarded means the credit has been applied.
	ReferralStatusRewarded = "rewarded"
	// ReferralStatusVoid means the reward was withdrawn, for example after a
	// refund. See [ReferralAttribution.VoidReason].
	ReferralStatusVoid = "void"
)

// ReferralAttribution is one referred workspace and where its reward stands.
type ReferralAttribution struct {
	ID             string  `json:"id"`
	ReferralCodeID *string `json:"referral_code_id,omitempty"`
	ReferrerUserID string  `json:"referrer_user_id"`
	ReferrerOrgID  string  `json:"referrer_org_id"`
	InviteeOrgID   string  `json:"invitee_org_id"`
	InviteeUserID  *string `json:"invitee_user_id,omitempty"`
	// Status is one of the ReferralStatus* constants.
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
	ID    string `json:"id"`
	OrgID string `json:"org_id"`
	// AttributionID is the referral the movement settles, when there is one.
	AttributionID *string `json:"attribution_id,omitempty"`
	// AmountCents is positive for a reward and negative for a clawback.
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
	// Reason names what moved the balance.
	Reason string `json:"reason"`
	// BalanceAfterCents is the running balance once this movement applied.
	BalanceAfterCents int64 `json:"balance_after_cents"`
	// StripeCustomerBalanceTxnID links the movement to the provider-side
	// customer balance transaction it produced.
	StripeCustomerBalanceTxnID *string   `json:"stripe_customer_balance_txn_id,omitempty"`
	CreatedAt                  time.Time `json:"created_at"`
}

// PlanChangePreview is what a plan change would cost, computed by the payment
// provider without making the change.
type PlanChangePreview struct {
	CurrentPlan *Plan `json:"current_plan"`
	NewPlan     *Plan `json:"new_plan"`
	// ProrationAmount is the credit or charge for the unused part of the
	// current period, and AmountDue what the next invoice comes to. Both are
	// in the smallest unit of Currency, and either can be negative.
	ProrationAmount int64 `json:"proration_amount"`
	AmountDue       int64 `json:"amount_due"`
	// NextBillingDate is when that invoice falls due.
	NextBillingDate time.Time `json:"next_billing_date"`
	Currency        string    `json:"currency"`
}

// What a discount code grants, in [DiscountPreview.Type] and
// [DiscountRedemption.Type].
const (
	// DiscountTypePercent takes a percentage off.
	DiscountTypePercent = "percent"
	// DiscountTypeFixed takes a fixed amount off.
	DiscountTypeFixed = "fixed"
	// DiscountTypeTrialExtension adds trial days instead of reducing a
	// charge.
	DiscountTypeTrialExtension = "trial_extension"
)

// How long a money discount keeps applying, in [DiscountPreview.Duration].
const (
	DiscountDurationOnce      = "once"
	DiscountDurationRepeating = "repeating"
	DiscountDurationForever   = "forever"
)

// Redemption states returned in [DiscountRedemption.Status].
const (
	DiscountRedemptionPending  = "pending"
	DiscountRedemptionApplied  = "applied"
	DiscountRedemptionCanceled = "canceled"
)

// DiscountPreview is what a promotion code would do. Check Valid first: an
// unusable code answers 200 with Valid false and a Reason, rather than an
// error.
type DiscountPreview struct {
	Valid  bool   `json:"valid"`
	Reason string `json:"reason,omitempty"`

	Code string `json:"code,omitempty"`
	// Type is one of the DiscountType* constants; exactly one of PercentOff,
	// AmountOff and TrialExtensionDays is set to match it.
	Type               string   `json:"type,omitempty"`
	PercentOff         *int     `json:"percent_off,omitempty"`
	AmountOff          *float64 `json:"amount_off,omitempty"`
	Currency           *string  `json:"currency,omitempty"`
	TrialExtensionDays *int     `json:"trial_extension_days,omitempty"`
	// Duration is one of the DiscountDuration* constants, with
	// DurationInMonths set when it repeats.
	Duration         string `json:"duration,omitempty"`
	DurationInMonths *int   `json:"duration_in_months,omitempty"`

	// OriginalAmount, DiscountedAmount and SavingsAmount are only computed
	// when a plan was named and the code is a money discount.
	OriginalAmount   *float64 `json:"original_amount,omitempty"`
	DiscountedAmount *float64 `json:"discounted_amount,omitempty"`
	SavingsAmount    *float64 `json:"savings_amount,omitempty"`
}

// DiscountRedemption is one promotion code the workspace has redeemed.
type DiscountRedemption struct {
	ID             string  `json:"id"`
	DiscountCodeID string  `json:"discount_code_id"`
	OrganizationID string  `json:"organization_id"`
	RedeemedBy     *string `json:"redeemed_by,omitempty"`
	SubscriptionID *string `json:"subscription_id,omitempty"`
	PlanID         *string `json:"plan_id,omitempty"`

	StripeCouponID          *string `json:"stripe_coupon_id,omitempty"`
	StripeCheckoutSessionID *string `json:"stripe_checkout_session_id,omitempty"`

	// Type is one of the DiscountType* constants.
	Type               string   `json:"type"`
	PercentOff         *int     `json:"percent_off,omitempty"`
	AmountOff          *float64 `json:"amount_off,omitempty"`
	Currency           *string  `json:"currency,omitempty"`
	TrialExtensionDays *int     `json:"trial_extension_days,omitempty"`

	// Status is one of the DiscountRedemption* constants.
	Status     string     `json:"status"`
	RedeemedAt time.Time  `json:"redeemed_at"`
	AppliedAt  *time.Time `json:"applied_at,omitempty"`

	// Code is the promotion's code, when the API joins it in.
	Code string `json:"code,omitempty"`
}

// EnterpriseInquiryParams asks the sales team to get in touch. CompanyName,
// ContactName and ContactEmail are required.
type EnterpriseInquiryParams struct {
	CompanyName  string `json:"company_name"`
	ContactName  string `json:"contact_name"`
	ContactEmail string `json:"contact_email"`
	// EstimatedVolume is expected monthly send volume.
	EstimatedVolume *int   `json:"estimated_volume,omitempty"`
	TeamSize        *int   `json:"team_size,omitempty"`
	Notes           string `json:"notes,omitempty"`
}

// Get returns the workspace's subscription.
func (s *BillingService) Get(ctx context.Context, opts ...RequestOption) (*Subscription, *Response, error) {
	return fetch[Subscription](ctx, s.client, "subscription", opts)
}

// Limits returns the subscription together with the realtime rate limits its
// plan carries. For the plan-gate view (trial state, daily send cap) see
// [BillingService.Features].
func (s *BillingService) Limits(ctx context.Context, opts ...RequestOption) (*SubscriptionWithLimits, *Response, error) {
	return fetch[SubscriptionWithLimits](ctx, s.client, "subscription/limits", opts)
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
// returnURL, where the portal sends the user back, is required.
func (s *BillingService) Portal(ctx context.Context, returnURL string, opts ...RequestOption) (string, *Response, error) {
	body := struct {
		ReturnURL string `json:"return_url"`
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

// Cancel schedules the subscription to lapse at the end of the current period
// (atPeriodEnd true), or clears a cancellation that was already scheduled so
// it renews again (atPeriodEnd false). Either way the workspace keeps its plan
// until the period actually ends. It needs an active provider subscription; a
// workspace that never had one is refused.
func (s *BillingService) Cancel(ctx context.Context, atPeriodEnd bool, opts ...RequestOption) (*Response, error) {
	body := struct {
		CancelAtPeriodEnd bool `json:"cancel_at_period_end"`
	}{CancelAtPeriodEnd: atPeriodEnd}
	return s.client.post(ctx, "subscription/cancel", body, nil, opts...)
}

// ChangePlan moves the workspace to a different plan and returns the updated
// subscription. It needs the manage-billing organization permission.
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

// PreviewPlanChange returns the proration moving to newPlanID would produce,
// without making the change. It needs the manage-billing organization
// permission.
func (s *BillingService) PreviewPlanChange(ctx context.Context, newPlanID string, opts ...RequestOption) (*PlanChangePreview, *Response, error) {
	q := url.Values{"new_plan_id": {newPlanID}}
	return fetch[PlanChangePreview](ctx, s.client, withQuery("subscription/preview-change", q), opts)
}

// ValidateDiscount checks a promotion code and reports what it would do. Name
// a planID to get the amounts it works out to on that plan; pass "" to just
// check the code. A code that cannot be used is not an error: the preview
// comes back with [DiscountPreview.Valid] false and a reason.
func (s *BillingService) ValidateDiscount(ctx context.Context, code, planID string, opts ...RequestOption) (*DiscountPreview, *Response, error) {
	body := struct {
		Code   string  `json:"code"`
		PlanID *string `json:"plan_id,omitempty"`
	}{Code: code}
	if planID != "" {
		body.PlanID = &planID
	}
	return send[DiscountPreview](ctx, s.client.post, "subscription/discount/validate", body, opts)
}

// AppliedDiscounts returns the promotions redeemed on the workspace, most
// recent first. It is not paginated: the server answers with its most recent
// 50.
func (s *BillingService) AppliedDiscounts(ctx context.Context, opts ...RequestOption) ([]DiscountRedemption, *Response, error) {
	return fetchData[DiscountRedemption](ctx, s.client, "subscription/discounts", opts)
}

// EnterpriseInquiry asks the sales team to get in touch about enterprise
// pricing and returns the inquiry's id.
func (s *BillingService) EnterpriseInquiry(ctx context.Context, params *EnterpriseInquiryParams, opts ...RequestOption) (string, *Response, error) {
	var out struct {
		InquiryID string `json:"inquiry_id"`
	}
	resp, err := s.client.post(ctx, "subscription/enterprise-inquiry", params, &out, opts...)
	if err != nil {
		return "", resp, err
	}
	return out.InquiryID, resp, nil
}

// --- AI credits ---

// Credits returns the workspace's AI credit position. Check
// [CreditBalance.Unlimited] first: a deployment without billing meters
// nothing.
func (s *BillingService) Credits(ctx context.Context, opts ...RequestOption) (*CreditBalance, *Response, error) {
	return fetch[CreditBalance](ctx, s.client, "subscription/credits", opts)
}

// CreditTransactions returns a page of the credit ledger, newest first.
func (s *BillingService) CreditTransactions(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[CreditTransaction], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[CreditTransaction](ctx, s.client, "subscription/credits/transactions", q, opts...)
}

// BuyCredits starts a hosted checkout for a top-up pack (a
// [CreditPack.Key] from [CreditBalance.Packs]). It requires an active paid
// plan; all three arguments are required.
func (s *BillingService) BuyCredits(ctx context.Context, pack, successURL, cancelURL string, opts ...RequestOption) (*CheckoutSession, *Response, error) {
	body := struct {
		Pack       string `json:"pack"`
		SuccessURL string `json:"success_url"`
		CancelURL  string `json:"cancel_url"`
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
// one yet, and returns it either way. It is idempotent: a workspace has one
// code, and calling this again returns the same one. For the shareable link
// and the running totals, read [BillingService.Referral].
func (s *BillingService) EnsureReferralCode(ctx context.Context, opts ...RequestOption) (*ReferralCode, *Response, error) {
	return send[ReferralCode](ctx, s.client.post, "subscription/referral", nil, opts)
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
