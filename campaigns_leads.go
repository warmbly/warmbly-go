package warmbly

import (
	"context"
	"net/url"
	"time"
)

// Sources of a [LeadHold] ([LeadHold.Source]). New sources may appear.
const (
	// LeadHoldManual is a hold a member or an API call placed.
	LeadHoldManual = "manual"
	// LeadHoldOutOfOffice is a hold an out-of-office auto-reply wrote.
	LeadHoldOutOfOffice = "out_of_office"
	// LeadHoldInboxTagging is a hold a classified reply wrote: "not now" for a
	// while, or a decline with no end.
	LeadHoldInboxTagging = "inbox_tagging"
	// LeadHoldCRM is a hold a CRM rule wrote: a lead status, a conversion or
	// an open opportunity in Salesforce.
	LeadHoldCRM = "crm"
	// LeadHoldCC holds a contact's own lead while they are copied on another
	// lead's thread in the same campaign, so they never get two sequences.
	LeadHoldCC = "cc"
)

// Why a copied contact is or is not on the next email
// ([CampaignLeadCC.Status]), in the order they are decided.
const (
	// LeadCCActive is copied on the next email.
	LeadCCActive = "active"
	// LeadCCUnsubscribed is an opted-out or suppressed address.
	LeadCCUnsubscribed = "unsubscribed"
	// LeadCCBounced is an address that bounced on this thread or on any
	// campaign email of its own.
	LeadCCBounced = "bounced"
	// LeadCCUndeliverable is an address verification refused.
	LeadCCUndeliverable = "undeliverable"
)

// LeadHold parks one contact's flow inside one campaign. The contact stays
// subscribed and stays a lead of the campaign: a hold is not an unsubscribe
// and not a suppression.
type LeadHold struct {
	Since time.Time `json:"since"`
	// Until is when the hold lifts by itself. Nil holds with no end, which
	// only a resume lifts.
	Until  *time.Time `json:"until,omitempty"`
	Reason string     `json:"reason,omitempty"`
	// Source is one of the LeadHold* source constants.
	Source string `json:"source"`
}

// CampaignLeadHold is a lead's hold state in one campaign.
type CampaignLeadHold struct {
	CampaignID string `json:"campaign_id"`
	ContactID  string `json:"contact_id"`
	// Hold is nil when the lead is not held.
	Hold *LeadHold `json:"hold,omitempty"`
}

// CampaignLeadPauseParams is the optional body of [CampaignService.PauseLead].
type CampaignLeadPauseParams struct {
	// Until is when the hold lifts. Nil holds with no end.
	Until  *time.Time `json:"until"`
	Reason string     `json:"reason,omitempty"`
}

// CampaignLeadCC is a contact copied on every email to one lead.
type CampaignLeadCC struct {
	ContactID string `json:"contact_id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Company   string `json:"company,omitempty"`
	// Status says whether the next email copies them: one of the LeadCC*
	// constants. Anything but [LeadCCActive] is left off.
	Status    string     `json:"status"`
	BouncedAt *time.Time `json:"bounced_at,omitempty"`
}

// Copied reports whether the next email to the lead carries this address.
func (c CampaignLeadCC) Copied() bool { return c.Status == LeadCCActive }

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

func leadPath(campaignID, contactID, suffix string) string {
	return "campaigns/" + url.PathEscape(campaignID) + "/leads/" + url.PathEscape(contactID) + suffix
}

// LeadHold reads whether one lead's flow is currently held. Needs the
// read_campaigns scope.
func (s *CampaignService) LeadHold(ctx context.Context, id, contactID string, opts ...RequestOption) (*CampaignLeadHold, *Response, error) {
	return fetch[CampaignLeadHold](ctx, s.client, leadPath(id, contactID, "/hold"), opts)
}

// PauseLead parks one contact's flow inside one campaign until a date, or
// with no end when params or [CampaignLeadPauseParams.Until] is nil. The
// contact stays subscribed and stays a lead of the campaign. Replacing a live
// hold keeps its start, so a retry lands on the same state. A contact that is
// not a lead of the campaign is a 404. Needs the write_campaigns scope.
func (s *CampaignService) PauseLead(ctx context.Context, id, contactID string, params *CampaignLeadPauseParams, opts ...RequestOption) (*CampaignLeadHold, *Response, error) {
	var body any
	if params != nil {
		body = params
	}
	return send[CampaignLeadHold](ctx, s.client.post, leadPath(id, contactID, "/pause"), body, opts)
}

// ResumeLead lifts a lead's hold now. Resuming a lead that is not held is a
// success; the answer has no hold. Needs the write_campaigns scope.
func (s *CampaignService) ResumeLead(ctx context.Context, id, contactID string, opts ...RequestOption) (*CampaignLeadHold, *Response, error) {
	return send[CampaignLeadHold](ctx, s.client.post, leadPath(id, contactID, "/resume"), nil, opts)
}

// LeadCC lists the contacts copied on every email to one lead. Needs the
// read_campaigns and read_contacts scopes.
func (s *CampaignService) LeadCC(ctx context.Context, id, contactID string, opts ...RequestOption) (*CampaignLeadCCList, *Response, error) {
	return fetch[CampaignLeadCCList](ctx, s.client, leadPath(id, contactID, "/cc"), opts)
}

// SetLeadCC replaces the contacts copied on one lead, so colleagues share one
// thread. An empty list removes them all. The list is the whole state, so a
// retry is safe. Needs the write_campaigns and read_contacts scopes.
func (s *CampaignService) SetLeadCC(ctx context.Context, id, contactID string, ccContactIDs []string, opts ...RequestOption) (*CampaignLeadCCList, *Response, error) {
	if ccContactIDs == nil {
		ccContactIDs = []string{}
	}
	body := struct {
		ContactIDs []string `json:"contact_ids"`
	}{ContactIDs: ccContactIDs}
	return send[CampaignLeadCCList](ctx, s.client.put, leadPath(id, contactID, "/cc"), body, opts)
}

// SuggestLeadCC offers the lead's likely colleagues to copy. Needs the
// read_campaigns and read_contacts scopes.
func (s *CampaignService) SuggestLeadCC(ctx context.Context, id, contactID string, opts ...RequestOption) ([]CampaignLeadCCSuggestion, *Response, error) {
	return fetchData[CampaignLeadCCSuggestion](ctx, s.client, leadPath(id, contactID, "/cc/suggestions"), opts)
}

// Limit kinds in [CampaignSendLimit.Kind], in the order the scheduler applies
// them. New kinds may appear.
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
)

// Mailbox states in [CampaignMailboxPlan.State]. New states may appear.
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
	// MailboxPlanNoWorker is a mailbox no heartbeating worker holds right now.
	MailboxPlanNoWorker = "no_worker"
)

// SendBottleneckBudgetSpent is the [CampaignSendPlan.Bottleneck] of a
// campaign that has sent everything its mailboxes had today: no limit binds,
// the day is simply used.
const SendBottleneckBudgetSpent = "budget_spent"

// CampaignSendPlan is today's sending plan for a campaign: what will go out
// today and every limit that decided it, derived through the scheduler's own
// gates on every read and never stored.
//
// The arithmetic is a waterfall that adds up: ConfiguredCeiling minus every
// Limits entry minus SentToday equals ExpectedRemaining.
type CampaignSendPlan struct {
	CampaignID string `json:"campaign_id"`
	Status     string `json:"status"`
	// Day is the budget day the plan counts, as a UTC date (YYYY-MM-DD): every
	// daily counter resets at midnight UTC whatever the campaign's timezone.
	// The window's times are in the campaign's own timezone.
	Day      string `json:"day"`
	Timezone string `json:"timezone"`
	// ComputedAt is when this plan was computed; a plan served from the
	// background snapshot carries the moment it was last walked.
	ComputedAt time.Time `json:"computed_at"`
	// Stale is true when the plan was served from a snapshot the campaign has
	// since outrun (edited, started, stopped or a new budget day); a fresh
	// walk is already running and a refreshed plan follows within moments.
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
	// Bottleneck names the limit that decides Projected: a SendLimit*
	// constant, [SendBottleneckBudgetSpent], or "" when nothing binds below
	// the ceiling.
	Bottleneck string `json:"bottleneck"`

	Limits    []CampaignSendLimit   `json:"limits"`
	Window    CampaignSendWindow    `json:"window"`
	Leads     CampaignLeadSupply    `json:"leads"`
	Mailboxes []CampaignMailboxPlan `json:"mailboxes"`
	// Organization is the workspace's plan-level daily allowance. Nil when the
	// plan is unlimited.
	Organization *CampaignOrgAllowance `json:"organization,omitempty"`
	// NextWakeAt is when the campaign's chain next runs. Nil when it has no
	// pending wakeup.
	NextWakeAt *time.Time `json:"next_wake_at,omitempty"`
}

// CampaignSendLimit is one clamp in the waterfall and how many of today's
// emails it took off the ceiling. Only clamps that removed something are
// listed.
type CampaignSendLimit struct {
	// Kind is one of the SendLimit* constants.
	Kind string `json:"kind"`
	// Emails is how many sends this clamp removed from today's total.
	Emails int `json:"emails"`
	// Mailboxes is how many of the pool's mailboxes it touched; zero for a
	// campaign-level clamp.
	Mailboxes int `json:"mailboxes,omitempty"`
}

// CampaignSendWindow is the campaign's calendar for today, in the campaign's
// timezone.
type CampaignSendWindow struct {
	// SendingDay is false when the schedule has no window today.
	SendingDay bool `json:"sending_day"`
	OpenNow    bool `json:"open_now"`
	// OpensAt is the next opening when the window is closed now; it may be on
	// a later day.
	OpensAt *time.Time `json:"opens_at,omitempty"`
	// ClosesAt is the end of the last window today, when there is one.
	ClosesAt *time.Time `json:"closes_at,omitempty"`
	// MinutesLeft is the sending time still ahead today.
	MinutesLeft int `json:"minutes_left"`
	// StartsAt is the campaign start date when it is still ahead, and EndsAt
	// its end date when one is set.
	StartsAt *time.Time `json:"starts_at,omitempty"`
	EndsAt   *time.Time `json:"ends_at,omitempty"`
}

// CampaignLeadSupply is the other half of the number: mailboxes can only send
// to leads whose step is due.
type CampaignLeadSupply struct {
	// DueNow is the email steps that could go this minute and DueLaterToday
	// those whose wait elapses before the day ends.
	DueNow        int `json:"due_now"`
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
	// ConfiguredCap is the mailbox's own daily cold cap and CapToday the cap
	// this campaign gives it today, after the cap clamps. LimitedBy names the
	// clamp that set CapToday.
	ConfiguredCap int    `json:"configured_cap"`
	CapToday      int    `json:"cap_today"`
	LimitedBy     string `json:"limited_by"`
	// SentToday is this campaign's sends from the mailbox today;
	// SentByOtherCampaigns is what other campaigns took from the same cap.
	SentToday            int `json:"sent_today"`
	SentByOtherCampaigns int `json:"sent_by_other_campaigns"`
	// ExpectedRemaining is what this mailbox is expected to still send today
	// for this campaign.
	ExpectedRemaining int `json:"expected_remaining"`
	// State is why the mailbox is or is not sending right now: one of the
	// MailboxPlan* constants.
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

// SendPlan returns today's sending plan for a campaign: what will go out
// today and every limit that decided it. Needs the read_campaigns scope.
func (s *CampaignService) SendPlan(ctx context.Context, id string, opts ...RequestOption) (*CampaignSendPlan, *Response, error) {
	return fetch[CampaignSendPlan](ctx, s.client, "campaigns/"+url.PathEscape(id)+"/send-plan", opts)
}

// WarmupCopyPlacement counts where warmup threads opened with a piece of copy
// landed, as the recipient mailboxes reported them.
type WarmupCopyPlacement struct {
	// Sent counts every dispatched opening; the landing counters cover the
	// receipts that came back.
	Sent int `json:"sent"`
	// Reported counts the sends whose recipient filed a receipt; landing
	// rates are read over it.
	Reported int `json:"reported"`
	Inbox    int `json:"inbox"`
	Tabs     int `json:"tabs"`
	Spam     int `json:"spam"`
	Rescued  int `json:"rescued"`
}

// WarmupCopyGroupPlacement is [WarmupCopyPlacement] for one recipient
// provider group.
type WarmupCopyGroupPlacement struct {
	// Group is "google" (Google Workspace) or "microsoft" (Microsoft 365).
	Group string `json:"group"`
	WarmupCopyPlacement
}

// CampaignStepWarmupPlacement is where one arm of a campaign step landed when
// the senders' warmup carried its copy.
type CampaignStepWarmupPlacement struct {
	SequenceID string `json:"sequence_id"`
	// VariantID is nil for the step's own copy and set for an A/B variant.
	VariantID *string `json:"variant_id"`
	// Placement counts business inboxes on Google Workspace and Microsoft 365,
	// the only ones campaign copy is sent to.
	Placement WarmupCopyPlacement        `json:"placement"`
	ByGroup   []WarmupCopyGroupPlacement `json:"by_group"`
}

// CampaignWarmupPlacement is a campaign's warmup placement report.
type CampaignWarmupPlacement struct {
	// Data lists only the arms warmup carried.
	Data []CampaignStepWarmupPlacement `json:"data"`
	// Days is how far back the report reads (30).
	Days int `json:"days"`
}

// WarmupPlacement reports where each of the campaign's opening steps landed
// when the senders' warmup carried its copy, over the last 30 days. Needs the
// read_campaigns scope.
func (s *CampaignService) WarmupPlacement(ctx context.Context, id string, opts ...RequestOption) (*CampaignWarmupPlacement, *Response, error) {
	out, resp, err := fetch[CampaignWarmupPlacement](ctx, s.client, "campaigns/"+url.PathEscape(id)+"/warmup-placement", opts)
	if err != nil {
		return nil, resp, err
	}
	if out.Data == nil {
		out.Data = []CampaignStepWarmupPlacement{}
	}
	return out, resp, nil
}

// PlacementMonitor returns the campaign's scheduled placement test, or nil
// when it has none. Needs the read_campaigns scope.
func (s *CampaignService) PlacementMonitor(ctx context.Context, id string, opts ...RequestOption) (*PlacementMonitor, *Response, error) {
	return placementFetch[*PlacementMonitor](ctx, s.client, "campaigns/"+url.PathEscape(id)+"/placement-monitor", opts)
}

// SetPlacementMonitor creates or updates the campaign's scheduled placement
// test. It is a full-state write, so a retry lands on the same monitor. A new
// or re-enabled monitor runs its first test within minutes. Needs the
// send_campaigns scope.
func (s *CampaignService) SetPlacementMonitor(ctx context.Context, id string, params *PlacementMonitorParams, opts ...RequestOption) (*PlacementMonitor, *Response, error) {
	if params == nil {
		params = &PlacementMonitorParams{}
	}
	out, resp, err := placementSend[PlacementMonitor](ctx, s.client.put, "campaigns/"+url.PathEscape(id)+"/placement-monitor", params, opts)
	if err != nil {
		return nil, resp, err
	}
	return &out, resp, nil
}

// DeletePlacementMonitor removes the campaign's scheduled placement test.
// Needs the send_campaigns scope.
func (s *CampaignService) DeletePlacementMonitor(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "campaigns/"+url.PathEscape(id)+"/placement-monitor", opts...)
}
