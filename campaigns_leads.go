package warmbly

import (
	"context"
	"net/url"
	"time"
)

// Limit kinds in a [CampaignSendPlan], in waterfall order
// ([CampaignSendLimit].Kind and [CampaignSendPlan].Bottleneck). New kinds may
// appear.
const (
	SendLimitCampaignDailyLimit = "campaign_daily_limit"
	SendLimitCampaignRamp       = "campaign_ramp"
	SendLimitWarmupGraduation   = "warmup_graduation"
	SendLimitWorkspaceRisk      = "workspace_risk"
	SendLimitDomainAuth         = "domain_auth"
	SendLimitResting            = "resting"
	SendLimitHealthHold         = "warmup_health_hold"
	SendLimitCloudStatusHold    = "cloud_status_hold"
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
	SendLimitSendingBehavior    = "sending_behavior"
	// SendBottleneckBudgetSpent is the Bottleneck of a campaign that has sent
	// everything its mailboxes had today: no limit binds, the day is used.
	SendBottleneckBudgetSpent = "budget_spent"
)

// Mailbox states in a [CampaignMailboxPlan]. New states may appear.
const (
	MailboxPlanSending      = "sending"
	MailboxPlanBudgetSpent  = "budget_spent"
	MailboxPlanHoursClosed  = "hours_closed"
	MailboxPlanNoWorkingDay = "no_working_day"
	MailboxPlanDomainAuth   = "domain_auth"
	MailboxPlanResting      = "resting"
	MailboxPlanHealthHold   = "health_hold"
	MailboxPlanCloudStatus  = "cloud_status_hold"
	MailboxPlanRecovery     = "send_recovery"
	MailboxPlanCooldown     = "send_cooldown"
	MailboxPlanAdmission    = "send_authority"
	MailboxPlanWindowClosed = "window_closed"
	// MailboxPlanNoWorker is a mailbox no worker holds right now.
	MailboxPlanNoWorker = "no_worker"
)

// Sources of a [LeadHold].
const (
	// LeadHoldOutOfOffice is a hold an auto-reply parked.
	LeadHoldOutOfOffice = "out_of_office"
	// LeadHoldManual is a hold a member or API caller set.
	LeadHoldManual = "manual"
	// LeadHoldCC holds a contact's own lead while they are copied on another
	// lead's thread in the same campaign, so they never get two sequences.
	LeadHoldCC = "cc"
	// LeadHoldInboxTagging is a hold inbox tagging set.
	LeadHoldInboxTagging = "inbox_tagging"
)

// Statuses of a [CampaignLeadCC]. Anything but LeadCCActive is left off the
// next email.
const (
	LeadCCActive = "active"
	// LeadCCUnsubscribed is an opted-out or suppressed address.
	LeadCCUnsubscribed = "unsubscribed"
	// LeadCCBounced is an address that bounced on this thread or on any
	// campaign email of its own.
	LeadCCBounced = "bounced"
	// LeadCCUndeliverable is an address verification refused.
	LeadCCUndeliverable = "undeliverable"
)

// CampaignSendPlan is what one campaign will send today and why that number is
// what it is. For active campaigns it is computed in the background and served
// from a stored snapshot; a campaign without a snapshot is computed on request.
//
// The arithmetic is a waterfall that adds up: ConfiguredCeiling minus every
// Limits entry minus SentToday equals ExpectedRemaining.
type CampaignSendPlan struct {
	CampaignID string `json:"campaign_id"`
	Status     string `json:"status"`
	// Day is the budget day the plan counts: every daily counter resets at
	// midnight UTC, whatever the campaign's timezone, so this is the UTC date.
	// The window's times are in the campaign's own timezone.
	Day      string `json:"day"`
	Timezone string `json:"timezone"`
	// ComputedAt is when this plan was computed.
	ComputedAt time.Time `json:"computed_at"`
	// Stale is true when the plan was served from a snapshot the campaign has
	// since outrun; a refreshed plan follows within moments.
	Stale bool `json:"stale"`
	// ConfiguredCeiling is the sum of the attached mailboxes' own daily caps.
	ConfiguredCeiling int `json:"configured_ceiling"`
	// Projected is today's total: what has gone out plus what is still
	// expected to.
	Projected int `json:"projected_today"`
	// SentToday is this campaign's sends so far today.
	SentToday int `json:"sent_today"`
	// ExpectedRemaining is what the pool can still send today for this
	// campaign after every limit and after the leads that are actually due.
	ExpectedRemaining int `json:"expected_remaining"`
	// Bottleneck names the limit that decides Projected: one of the SendLimit
	// constants, or empty when nothing binds below the ceiling.
	Bottleneck string                `json:"bottleneck"`
	Limits     []CampaignSendLimit   `json:"limits"`
	Window     CampaignSendWindow    `json:"window"`
	Leads      CampaignLeadSupply    `json:"leads"`
	Mailboxes  []CampaignMailboxPlan `json:"mailboxes"`
	// Organization is the workspace's plan-level daily allowance. Nil when
	// the plan is unlimited.
	Organization *CampaignOrgAllowance `json:"organization,omitempty"`
	// NextWakeAt is when the campaign's chain next runs. Nil when it has no
	// pending wakeup.
	NextWakeAt *time.Time `json:"next_wake_at,omitempty"`
}

// CampaignSendLimit is one clamp in the send-plan waterfall and how many of
// today's emails it took off the ceiling.
type CampaignSendLimit struct {
	// Kind is one of the SendLimit constants.
	Kind   string `json:"kind"`
	Emails int    `json:"emails"`
	// Mailboxes is how many of the pool's mailboxes it touched. Zero for a
	// campaign-level clamp.
	Mailboxes int `json:"mailboxes,omitempty"`
}

// CampaignSendWindow is the campaign's calendar for today.
type CampaignSendWindow struct {
	// SendingDay is false when the schedule has no window today.
	SendingDay bool `json:"sending_day"`
	OpenNow    bool `json:"open_now"`
	// OpensAt is the next opening when the window is closed now.
	OpensAt *time.Time `json:"opens_at,omitempty"`
	// ClosesAt is the end of the last window today.
	ClosesAt *time.Time `json:"closes_at,omitempty"`
	// MinutesLeft is the sending time still ahead today.
	MinutesLeft int `json:"minutes_left"`
	// StartsAt is the campaign start date when it is still ahead.
	StartsAt *time.Time `json:"starts_at,omitempty"`
	// EndsAt is the campaign end date, when one is set.
	EndsAt *time.Time `json:"ends_at,omitempty"`
}

// CampaignLeadSupply is the lead side of a send plan: mailboxes can only send
// to leads whose step is due.
type CampaignLeadSupply struct {
	// DueNow is the email steps that could go this minute.
	DueNow int `json:"due_now"`
	// DueLaterToday is the email steps whose wait elapses before the day ends.
	DueLaterToday int `json:"due_later_today"`
	// NewLeadsDueToday is how many of DueNow plus DueLaterToday are first
	// emails, which the new-lead cap governs.
	NewLeadsDueToday int `json:"new_leads_due_today"`
	// WaitingOnStep is the leads whose next step is due after today.
	WaitingOnStep int `json:"waiting_on_step"`
	// WaitingOnCondition is the leads inside an undecided branch window.
	WaitingOnCondition int `json:"waiting_on_condition"`
	// Held is the leads paused (out of office, or by hand).
	Held int `json:"held"`
	// WaitingOnSender is the due steps whose own mailbox has nothing left
	// today.
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
	// ConfiguredCap is the mailbox's own daily cold cap.
	ConfiguredCap int `json:"configured_cap"`
	// CapToday is the cap this campaign gives it today, after the cap clamps.
	CapToday int `json:"cap_today"`
	// LimitedBy names the clamp that set CapToday: mailbox_daily_cap,
	// campaign_daily_limit, campaign_ramp, warmup_graduation or
	// workspace_risk.
	LimitedBy string `json:"limited_by"`
	// SentToday is this campaign's sends from the mailbox today;
	// SentByOtherCampaigns is what other campaigns took from the same cap.
	SentToday            int `json:"sent_today"`
	SentByOtherCampaigns int `json:"sent_by_other_campaigns"`
	// ExpectedRemaining is what this mailbox is expected to still send today
	// for this campaign.
	ExpectedRemaining int `json:"expected_remaining"`
	// State is why the mailbox is or is not sending right now, one of the
	// MailboxPlan constants.
	State string `json:"state"`
	// ReopensAt is when a mailbox outside its own hours is next open.
	ReopensAt *time.Time `json:"reopens_at,omitempty"`
	// Health is the warmup health band when it is not healthy.
	Health string `json:"health,omitempty"`
	// MinGapSeconds is the spacing between two of its sends, which warmup
	// mail shares.
	MinGapSeconds int `json:"min_gap_seconds"`
	// Graduation is set while the warmup graduation ceiling holds the mailbox
	// below its own cap.
	Graduation *ColdRamp `json:"graduation,omitempty"`
}

// CampaignOrgAllowance is the workspace's plan-level daily campaign limit.
type CampaignOrgAllowance struct {
	DailyLimit int `json:"daily_limit"`
	SentToday  int `json:"sent_today"`
	Remaining  int `json:"remaining"`
}

// LeadHold is one contact's flow parked inside one campaign.
type LeadHold struct {
	Since time.Time `json:"since"`
	// Until is nil for a hold with no end, which only a resume lifts.
	Until  *time.Time `json:"until,omitempty"`
	Reason string     `json:"reason,omitempty"`
	// Source is one of the LeadHold constants.
	Source string `json:"source"`
}

// CampaignLeadHold says whether one lead's flow in a campaign is held.
type CampaignLeadHold struct {
	CampaignID string `json:"campaign_id"`
	ContactID  string `json:"contact_id"`
	// Hold is nil when the lead is not held.
	Hold *LeadHold `json:"hold,omitempty"`
}

// CampaignLeadPauseParams holds one lead's flow. Both fields are optional.
type CampaignLeadPauseParams struct {
	// Until lifts the hold at that moment. Nil holds with no end, which only
	// a resume lifts.
	Until *time.Time `json:"until,omitempty"`
	// Reason is a note stored with the hold.
	Reason string `json:"reason,omitempty"`
}

// CampaignLeadCC is a contact copied on every email a campaign sends one lead,
// so several people at one company share a single thread.
type CampaignLeadCC struct {
	ContactID string `json:"contact_id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Company   string `json:"company,omitempty"`
	// Status says whether the next email copies them, one of the LeadCC
	// constants. Anything but LeadCCActive is left off.
	Status    string     `json:"status"`
	BouncedAt *time.Time `json:"bounced_at,omitempty"`
}

// CampaignLeadCCList is the contacts copied on one lead.
type CampaignLeadCCList struct {
	CampaignID string           `json:"campaign_id"`
	ContactID  string           `json:"contact_id"`
	CC         []CampaignLeadCC `json:"cc"`
}

// CampaignLeadCCSuggestion is a contact who looks like a colleague of the
// lead, offered first when picking who to copy.
type CampaignLeadCCSuggestion struct {
	ContactID string `json:"contact_id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Company   string `json:"company,omitempty"`
	// Reason is "company" when the company names match and "domain" when only
	// the email domain does.
	Reason string `json:"reason"`
}

// SendPlan returns today's sending plan for a campaign: what will go out today
// and every limit that decided it, read through the scheduler's own gates. The
// figures may be served from a recent snapshot (see [CampaignSendPlan].Stale).
// Needs [PermReadCampaigns].
func (s *CampaignService) SendPlan(ctx context.Context, id string, opts ...RequestOption) (*CampaignSendPlan, *Response, error) {
	return fetch[CampaignSendPlan](ctx, s.client, "campaigns/"+url.PathEscape(id)+"/send-plan", opts)
}

// LeadHold reports whether one contact's flow in a campaign is held. Needs
// [PermReadCampaigns].
func (s *CampaignService) LeadHold(ctx context.Context, id, contactID string, opts ...RequestOption) (*CampaignLeadHold, *Response, error) {
	return fetch[CampaignLeadHold](ctx, s.client, leadPath(id, contactID, "hold"), opts)
}

// PauseLead parks one contact's flow inside one campaign until a date, or with
// no end. The contact stays subscribed and stays a lead of the campaign: this
// is not an unsubscribe and not a suppression. Pausing a held lead replaces
// the hold. params may be nil to hold with no end. Needs [PermWriteCampaigns].
func (s *CampaignService) PauseLead(ctx context.Context, id, contactID string, params *CampaignLeadPauseParams, opts ...RequestOption) (*CampaignLeadHold, *Response, error) {
	return send[CampaignLeadHold](ctx, s.client.post, leadPath(id, contactID, "pause"), params, opts)
}

// ResumeLead lifts a lead's hold now. Resuming a lead that is not held
// succeeds; a contact that is not a lead of the campaign is a 404. Needs
// [PermWriteCampaigns].
func (s *CampaignService) ResumeLead(ctx context.Context, id, contactID string, opts ...RequestOption) (*CampaignLeadHold, *Response, error) {
	return send[CampaignLeadHold](ctx, s.client.post, leadPath(id, contactID, "resume"), nil, opts)
}

// LeadCC lists the contacts copied on every email to one lead. Needs
// [PermReadCampaigns] and [PermReadContacts].
func (s *CampaignService) LeadCC(ctx context.Context, id, contactID string, opts ...RequestOption) (*CampaignLeadCCList, *Response, error) {
	return fetch[CampaignLeadCCList](ctx, s.client, leadPath(id, contactID, "cc"), opts)
}

// SetLeadCC replaces the contacts copied on one lead with contactIDs; an empty
// list removes them all. Needs [PermWriteCampaigns] and [PermReadContacts].
func (s *CampaignService) SetLeadCC(ctx context.Context, id, contactID string, contactIDs []string, opts ...RequestOption) (*CampaignLeadCCList, *Response, error) {
	if contactIDs == nil {
		contactIDs = []string{}
	}
	body := struct {
		ContactIDs []string `json:"contact_ids"`
	}{contactIDs}
	return send[CampaignLeadCCList](ctx, s.client.put, leadPath(id, contactID, "cc"), body, opts)
}

// SuggestLeadCC offers the lead's likely colleagues to copy. Needs
// [PermReadCampaigns] and [PermReadContacts].
func (s *CampaignService) SuggestLeadCC(ctx context.Context, id, contactID string, opts ...RequestOption) ([]CampaignLeadCCSuggestion, *Response, error) {
	return fetchData[CampaignLeadCCSuggestion](ctx, s.client, leadPath(id, contactID, "cc/suggestions"), opts)
}

func leadPath(id, contactID, tail string) string {
	return "campaigns/" + url.PathEscape(id) + "/leads/" + url.PathEscape(contactID) + "/" + tail
}
