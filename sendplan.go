package warmbly

import (
	"context"
	"net/url"
	"time"
)

// Limit kinds returned in [CampaignSendLimit.Kind] and
// [CampaignSendPlan.Bottleneck], in the order the scheduler applies them. New
// kinds may appear.
const (
	SendLimitCampaignDailyLimit = "campaign_daily_limit"
	SendLimitCampaignRamp       = "campaign_ramp"
	SendLimitWarmupGraduation   = "warmup_graduation"
	SendLimitWorkspaceRisk      = "workspace_risk"
	SendLimitDomainAuth         = "domain_auth"
	SendLimitSendingBehavior    = "sending_behavior"
	SendLimitResting            = "resting"
	SendLimitHealthHold         = "warmup_health_hold"
	SendLimitRecovery           = "send_recovery"
	SendLimitCooldown           = "send_cooldown"
	SendLimitOtherCampaigns     = "other_campaigns"
	SendLimitHealthPace         = "warmup_health_pace"
	SendLimitMailboxHours       = "mailbox_hours"
	SendLimitSpacing            = "spacing"
	SendLimitSendingWindow      = "sending_window"
	SendLimitNotRunning         = "not_running"
	SendLimitOrgDailyLimit      = "org_daily_limit"
	SendLimitNewLeadCap         = "new_lead_cap"
	SendLimitLeads              = "leads"
)

// SendBottleneckBudgetSpent is the [CampaignSendPlan.Bottleneck] of a
// campaign that has sent everything its mailboxes had today: no limit binds,
// the day is simply used.
const SendBottleneckBudgetSpent = "budget_spent"

// Mailbox states returned in [CampaignMailboxPlan.State]. New states may
// appear.
const (
	MailboxPlanSending      = "sending"
	MailboxPlanBudgetSpent  = "budget_spent"
	MailboxPlanHoursClosed  = "hours_closed"
	MailboxPlanNoWorkingDay = "no_working_day"
	MailboxPlanDomainAuth   = "domain_auth"
	MailboxPlanResting      = "resting"
	MailboxPlanHealthHold   = "health_hold"
	MailboxPlanRecovery     = "send_recovery"
	MailboxPlanCooldown     = "send_cooldown"
	MailboxPlanAdmission    = "send_authority"
	MailboxPlanWindowClosed = "window_closed"
	// MailboxPlanNoWorker is a mailbox no running worker holds right now.
	MailboxPlanNoWorker = "no_worker"
)

// CampaignSendPlan is what one campaign will send today and why that number
// is what it is. It is derived on every read through the scheduler's own
// gates and never stored, so it cannot promise a volume the send path would
// refuse.
//
// The arithmetic is a waterfall that adds up: ConfiguredCeiling minus every
// Limits entry minus SentToday equals ExpectedRemaining.
type CampaignSendPlan struct {
	CampaignID string `json:"campaign_id"`
	Status     string `json:"status"`
	// Day is the UTC date the plan counts: every daily counter resets at
	// midnight UTC, whatever the campaign's timezone. The window's times are
	// in the campaign's own Timezone.
	Day      string `json:"day"`
	Timezone string `json:"timezone"`
	// ComputedAt is when the figures were computed. Stale is true when they
	// come from a snapshot the campaign has since outrun (it was edited,
	// started or stopped, or a new day began); a refreshed plan follows
	// within moments.
	ComputedAt time.Time `json:"computed_at"`
	Stale      bool      `json:"stale"`

	// ConfiguredCeiling is the sum of the attached mailboxes' own daily
	// caps, before anything else is applied.
	ConfiguredCeiling int `json:"configured_ceiling"`
	// ProjectedToday is what has gone out plus what is still expected to.
	ProjectedToday int `json:"projected_today"`
	SentToday      int `json:"sent_today"`
	// ExpectedRemaining is what the pool can still send today for this
	// campaign after every limit, and after the leads actually due.
	ExpectedRemaining int `json:"expected_remaining"`
	// Bottleneck names the limit that decides ProjectedToday: one of the
	// SendLimit* constants, [SendBottleneckBudgetSpent], or empty when
	// nothing binds below the ceiling.
	Bottleneck string `json:"bottleneck"`

	Limits    []CampaignSendLimit   `json:"limits"`
	Window    CampaignSendWindow    `json:"window"`
	Leads     CampaignLeadSupply    `json:"leads"`
	Mailboxes []CampaignMailboxPlan `json:"mailboxes"`
	// Organization is the workspace's plan-level daily allowance, nil when
	// the plan is unlimited.
	Organization *CampaignOrgAllowance `json:"organization,omitempty"`
	// NextWakeAt is when the campaign's scheduler next runs, nil when it has
	// no pending wakeup.
	NextWakeAt *time.Time `json:"next_wake_at,omitempty"`
}

// CampaignSendLimit is one clamp in the waterfall and how many of today's
// emails it took off the ceiling. Only clamps that removed something are
// listed.
type CampaignSendLimit struct {
	// Kind is one of the SendLimit* constants.
	Kind   string `json:"kind"`
	Emails int    `json:"emails"`
	// Mailboxes is how many of the pool's mailboxes it touched, zero for a
	// campaign-level clamp.
	Mailboxes int `json:"mailboxes,omitempty"`
}

// CampaignSendWindow is the campaign's calendar for today.
type CampaignSendWindow struct {
	// SendingDay is false when the schedule has no window today.
	SendingDay bool `json:"sending_day"`
	OpenNow    bool `json:"open_now"`
	// OpensAt is the next opening when the window is closed now; it may be
	// on a later day.
	OpensAt *time.Time `json:"opens_at,omitempty"`
	// ClosesAt is the end of the last window today, when there is one.
	ClosesAt *time.Time `json:"closes_at,omitempty"`
	// MinutesLeft is the sending time still ahead today.
	MinutesLeft int `json:"minutes_left"`
	// StartsAt is the campaign start date when it is still ahead.
	StartsAt *time.Time `json:"starts_at,omitempty"`
	// EndsAt is the campaign end date, when one is set.
	EndsAt *time.Time `json:"ends_at,omitempty"`
}

// CampaignLeadSupply is the other half of the number: mailboxes can only
// send to leads whose step is due.
type CampaignLeadSupply struct {
	// DueNow is the email steps that could go this minute; DueLaterToday the
	// ones whose wait elapses before the day ends.
	DueNow        int `json:"due_now"`
	DueLaterToday int `json:"due_later_today"`
	// NewLeadsDueToday is how many of those are first emails, which the
	// new-lead cap governs.
	NewLeadsDueToday int `json:"new_leads_due_today"`
	// WaitingOnStep is the leads whose next step is due after today.
	WaitingOnStep int `json:"waiting_on_step"`
	// WaitingOnCondition is the leads inside an undecided branch window.
	WaitingOnCondition int `json:"waiting_on_condition"`
	// Held is the leads paused (out of office, or by hand).
	Held int `json:"held"`
	// WaitingOnSender is the due steps whose own mailbox has nothing left
	// today. Each contact keeps the address they first heard from.
	WaitingOnSender int `json:"waiting_on_sender"`
	// NewLeadsStartedToday and MaxNewLeadsPerDay are the new-lead throttle;
	// the cap is 0 when unlimited.
	NewLeadsStartedToday int `json:"new_leads_started_today"`
	MaxNewLeadsPerDay    int `json:"max_new_leads_per_day"`
	// NextDueAt is the soonest moment a waiting lead becomes due, when
	// nothing is due right now.
	NextDueAt *time.Time `json:"next_due_at,omitempty"`
}

// CampaignMailboxPlan is one mailbox's day on a campaign.
type CampaignMailboxPlan struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Provider string `json:"provider"`
	// ConfiguredCap is the mailbox's own daily cold cap; CapToday the cap
	// this campaign gives it today after the cap clamps.
	ConfiguredCap int `json:"configured_cap"`
	CapToday      int `json:"cap_today"`
	// LimitedBy names the clamp that set CapToday: mailbox_daily_cap or one
	// of the SendLimit* cap clamps.
	LimitedBy string `json:"limited_by"`
	// SentToday is this campaign's sends from the mailbox today;
	// SentByOtherCampaigns is what other campaigns took from the same cap.
	SentToday            int `json:"sent_today"`
	SentByOtherCampaigns int `json:"sent_by_other_campaigns"`
	ExpectedRemaining    int `json:"expected_remaining"`
	// State is one of the MailboxPlan* constants.
	State string `json:"state"`
	// ReopensAt is when a mailbox outside its own hours next opens.
	ReopensAt *time.Time `json:"reopens_at,omitempty"`
	// Health is the warmup health band when it is not healthy.
	Health string `json:"health,omitempty"`
	// MinGapSeconds is the spacing between two of its sends, which warmup
	// mail shares.
	MinGapSeconds int `json:"min_gap_seconds"`
	// Graduation is set while the warmup graduation ceiling holds the
	// mailbox below its own cap.
	Graduation *ColdRamp `json:"graduation,omitempty"`
}

// CampaignOrgAllowance is the workspace's plan-level daily campaign limit.
type CampaignOrgAllowance struct {
	DailyLimit int `json:"daily_limit"`
	SentToday  int `json:"sent_today"`
	Remaining  int `json:"remaining"`
}

// SendPlan returns what a campaign will send today and which limit decides
// that number, worked out through the scheduler's own gates on every read.
// Requires [PermReadCampaigns].
func (s *CampaignService) SendPlan(ctx context.Context, id string, opts ...RequestOption) (*CampaignSendPlan, *Response, error) {
	return fetch[CampaignSendPlan](ctx, s.client, "campaigns/"+url.PathEscape(id)+"/send-plan", opts)
}
