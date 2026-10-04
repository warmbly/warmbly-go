package warmbly

import (
	"context"
	"net/url"
	"time"
)

// This file holds the per-lead controls of a campaign (copying colleagues on a
// lead's emails, holding one lead's flow) and the campaign's daily send plan.
// All of them hang off [CampaignService].

// LeadHoldSource says what parked a lead. Unknown values decode unchanged.
type LeadHoldSource string

// Hold sources.
const (
	// LeadHoldManual is a hold set with [CampaignService.PauseLead].
	LeadHoldManual LeadHoldSource = "manual"
	// LeadHoldOutOfOffice is an auto-reply that parked the lead until the
	// return date it named.
	LeadHoldOutOfOffice LeadHoldSource = "out_of_office"
	// LeadHoldCC holds a contact's own lead while they are copied on another
	// lead's thread in the same campaign, so they never get two sequences.
	LeadHoldCC LeadHoldSource = "cc"
	// LeadHoldInboxTagging is a hold a classified reply wrote: "not now" for
	// a while, or a decline with no end.
	LeadHoldInboxTagging LeadHoldSource = "inbox_tagging"
)

// LeadCCStatus says whether the next email to a lead copies a contact.
// Anything but [LeadCCActive] is left off the email. Unknown values decode
// unchanged.
type LeadCCStatus string

// Copy statuses, in the order the server decides them.
const (
	// LeadCCActive means the next email copies the contact.
	LeadCCActive LeadCCStatus = "active"
	// LeadCCUnsubscribed is an opted-out or suppressed address.
	LeadCCUnsubscribed LeadCCStatus = "unsubscribed"
	// LeadCCBounced is an address that bounced on this thread or on any
	// campaign email of its own.
	LeadCCBounced LeadCCStatus = "bounced"
	// LeadCCUndeliverable is an address verification refused, under the same
	// rule the campaign applies to its leads.
	LeadCCUndeliverable LeadCCStatus = "undeliverable"
)

// CampaignLeadMaxCC is the most contacts that can be copied on one lead's
// emails. More is a 400 with the code "lead_cc_limit".
const CampaignLeadMaxCC = 2

// Reasons a contact is suggested as a copy ([LeadCCSuggestion.Reason]).
const (
	// LeadCCReasonCompany means the company names match.
	LeadCCReasonCompany = "company"
	// LeadCCReasonDomain means only the email domain matches.
	LeadCCReasonDomain = "domain"
)

// LeadHold is one contact's flow parked inside one campaign. The contact
// stays subscribed and stays a lead of the campaign: a hold is not an
// unsubscribe and not a suppression.
type LeadHold struct {
	// Since is when the hold began. Replacing a live hold keeps it.
	Since time.Time `json:"since"`
	// Until is when the hold lifts, nil for a hold with no end, which only
	// [CampaignService.ResumeLead] lifts.
	Until  *time.Time     `json:"until,omitempty"`
	Reason string         `json:"reason,omitempty"`
	Source LeadHoldSource `json:"source"`
}

// LeadPauseParams parks one lead. Both fields are optional: an empty body
// holds the lead with no end.
type LeadPauseParams struct {
	// Until is when the hold lifts. It must be in the future and within a
	// year of now; nil holds the lead until [CampaignService.ResumeLead].
	Until *time.Time `json:"until,omitempty"`
	// Reason is a note shown with the hold; the server trims it to one short
	// line.
	Reason string `json:"reason,omitempty"`
}

// CampaignLeadCC is a contact copied on every email one campaign sends one
// lead, so several people at one company share a single thread.
type CampaignLeadCC struct {
	ContactID string `json:"contact_id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Company   string `json:"company,omitempty"`
	// Status says whether the next email copies them.
	Status LeadCCStatus `json:"status"`
	// BouncedAt is when the address bounced, for [LeadCCBounced].
	BouncedAt *time.Time `json:"bounced_at,omitempty"`
}

// Copied reports whether the next email to the lead carries this address.
func (c CampaignLeadCC) Copied() bool { return c.Status == LeadCCActive }

// LeadCCSuggestion is a contact who looks like a colleague of the lead,
// offered first when choosing who to copy.
type LeadCCSuggestion struct {
	ContactID string `json:"contact_id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Company   string `json:"company,omitempty"`
	// Reason is [LeadCCReasonCompany] or [LeadCCReasonDomain].
	Reason string `json:"reason"`
}

// --- send plan ---------------------------------------------------------

// Send plan limit kinds ([CampaignSendLimit.Kind]) and bottlenecks
// ([CampaignSendPlan.Bottleneck]), in waterfall order. Unknown kinds decode
// unchanged.
const (
	SendLimitCampaignDailyLimit = "campaign_daily_limit"
	SendLimitCampaignRamp       = "campaign_ramp"
	SendLimitWarmupGraduation   = "warmup_graduation"
	SendLimitWorkspaceRisk      = "workspace_risk"
	SendLimitDomainAuth         = "domain_auth"
	SendLimitResting            = "resting"
	SendLimitHealthHold         = "warmup_health_hold"
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
	// SendBottleneckBudgetSpent is the bottleneck of a campaign that has sent
	// everything its mailboxes had today: no limit binds, the day is simply
	// used.
	SendBottleneckBudgetSpent = "budget_spent"
)

// Mailbox states in a send plan ([CampaignMailboxPlan.State]). Unknown states
// decode unchanged.
const (
	MailboxPlanSending      = "sending"
	MailboxPlanBudgetSpent  = "budget_spent"
	MailboxPlanHoursClosed  = "hours_closed"
	MailboxPlanNoWorkingDay = "no_working_day"
	MailboxPlanDomainAuth   = "domain_auth"
	MailboxPlanResting      = "resting"
	MailboxPlanHealthHold   = "health_hold"
	MailboxPlanWindowClosed = "window_closed"
	// MailboxPlanNoWorker is a mailbox no heartbeating worker holds right now.
	MailboxPlanNoWorker = "no_worker"
)

// CampaignSendPlan is what one campaign will send today and why that number
// is what it is. It is derived through the scheduler's own gates, so it cannot
// promise a volume the send path would refuse.
//
// The arithmetic is a waterfall that adds up: ConfiguredCeiling minus every
// [CampaignSendLimit.Emails] minus SentToday equals ExpectedRemaining.
type CampaignSendPlan struct {
	CampaignID string `json:"campaign_id"`
	// Status is the campaign's status, one of the CampaignStatus* constants.
	Status string `json:"status"`
	// Day is the budget day the plan counts, a UTC date (YYYY-MM-DD): every
	// daily counter resets at midnight UTC whatever the campaign's timezone.
	// The times in Window are in the campaign's own Timezone.
	Day      string `json:"day"`
	Timezone string `json:"timezone"`
	// ComputedAt is when the plan was computed. A plan served from the
	// background snapshot carries the moment the snapshotter last walked the
	// campaign, which tells how fresh the figures are.
	ComputedAt time.Time `json:"computed_at"`
	// Stale is true when the plan came from a snapshot the campaign has since
	// outrun (it was edited, started or stopped, or a new budget day began)
	// and a fresh one is being computed in the background. The figures are
	// the last good ones; read again in a moment.
	Stale bool `json:"stale"`

	// ConfiguredCeiling is the sum of the attached mailboxes' own daily caps.
	ConfiguredCeiling int `json:"configured_ceiling"`
	// ProjectedToday is today's total: what has gone out plus what is still
	// expected to.
	ProjectedToday int `json:"projected_today"`
	// SentToday is this campaign's sends so far today.
	SentToday int `json:"sent_today"`
	// ExpectedRemaining is what the pool can still send today for this
	// campaign after every limit and after the leads that are actually due.
	ExpectedRemaining int `json:"expected_remaining"`
	// Bottleneck names the limit that decides ProjectedToday: a SendLimit*
	// constant or [SendBottleneckBudgetSpent], empty when nothing binds below
	// the ceiling.
	Bottleneck string `json:"bottleneck"`

	// Limits lists the clamps that removed something, in the order the
	// scheduler applies them.
	Limits    []CampaignSendLimit   `json:"limits"`
	Window    CampaignSendWindow    `json:"window"`
	Leads     CampaignLeadSupply    `json:"leads"`
	Mailboxes []CampaignMailboxPlan `json:"mailboxes"`
	// Organization is the workspace's plan-level daily allowance, nil when it
	// is unlimited.
	Organization *CampaignOrgAllowance `json:"organization,omitempty"`
	// NextWakeAt is when the campaign's chain next runs, nil when it has no
	// pending wakeup (not running, or being re-seeded).
	NextWakeAt *time.Time `json:"next_wake_at,omitempty"`
}

// CampaignSendLimit is one clamp in the waterfall and how many of today's
// emails it took off the ceiling.
type CampaignSendLimit struct {
	// Kind is one of the SendLimit* constants.
	Kind string `json:"kind"`
	// Emails is how many sends this clamp removed from today's total.
	Emails int `json:"emails"`
	// Mailboxes is how many of the pool's mailboxes it touched, absent for a
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
	// StartsAt is the campaign start date when it is still ahead; EndsAt the
	// end date, when one is set.
	StartsAt *time.Time `json:"starts_at,omitempty"`
	EndsAt   *time.Time `json:"ends_at,omitempty"`
}

// CampaignLeadSupply is the other half of the number: mailboxes can only send
// to leads whose step is due.
type CampaignLeadSupply struct {
	// DueNow is the email steps that could go this minute; DueLaterToday
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
	// today. Each contact keeps the address they first heard from, so these
	// wait for it rather than going out from another mailbox.
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
	// LimitedBy names the clamp that set CapToday: mailbox_daily_cap, or a
	// SendLimit* kind such as campaign_daily_limit, campaign_ramp,
	// warmup_graduation or workspace_risk.
	LimitedBy string `json:"limited_by"`
	// SentToday is this campaign's sends from the mailbox today;
	// SentByOtherCampaigns is what other campaigns took from the same cap.
	SentToday            int `json:"sent_today"`
	SentByOtherCampaigns int `json:"sent_by_other_campaigns"`
	// ExpectedRemaining is what the mailbox is expected to still send today
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

// SendPlan returns what the campaign will send today and why: a waterfall
// from the mailboxes' configured ceiling down to the expected remaining sends,
// the sending window, the supply of due leads and each mailbox's day.
//
// The server answers from a snapshot it refreshes in the background, so the
// read is cheap whatever the campaign's size; check [CampaignSendPlan.Stale]
// and [CampaignSendPlan.ComputedAt] for freshness.
//
// Requires [PermReadCampaigns].
func (s *CampaignService) SendPlan(ctx context.Context, id string, opts ...RequestOption) (*CampaignSendPlan, *Response, error) {
	return fetch[CampaignSendPlan](ctx, s.client, "campaigns/"+url.PathEscape(id)+"/send-plan", opts)
}

// --- lead hold ---------------------------------------------------------

func leadPath(campaignID, contactID, tail string) string {
	return "campaigns/" + url.PathEscape(campaignID) + "/leads/" + url.PathEscape(contactID) + "/" + tail
}

// leadHoldResult is the answer all three hold endpoints share. Hold is absent
// when the lead is not held.
type leadHoldResult struct {
	Hold *LeadHold `json:"hold"`
}

// LeadHold reports whether one lead's flow is currently held. It returns a nil
// hold, with no error, when the lead is not held. A contact that is not a
// lead of the campaign answers a 404.
//
// Requires [PermReadCampaigns].
func (s *CampaignService) LeadHold(ctx context.Context, campaignID, contactID string, opts ...RequestOption) (*LeadHold, *Response, error) {
	out, resp, err := fetch[leadHoldResult](ctx, s.client, leadPath(campaignID, contactID, "hold"), opts)
	if err != nil {
		return nil, resp, err
	}
	return out.Hold, resp, nil
}

// PauseLead parks one contact's flow inside one campaign until a date, or
// with no end when params is nil or has no Until. The contact stays
// subscribed and stays a lead: this is not an unsubscribe. Pausing an
// already held lead replaces the hold and keeps its start, so a retry is
// safe.
//
// Requires [PermWriteCampaigns].
func (s *CampaignService) PauseLead(ctx context.Context, campaignID, contactID string, params *LeadPauseParams, opts ...RequestOption) (*LeadHold, *Response, error) {
	out, resp, err := send[leadHoldResult](ctx, s.client.post, leadPath(campaignID, contactID, "pause"), params, opts)
	if err != nil {
		return nil, resp, err
	}
	return out.Hold, resp, nil
}

// ResumeLead lifts a lead's hold now. Resuming a lead that is not held
// succeeds and changes nothing. A contact that is not a lead of the campaign
// answers a 404.
//
// Requires [PermWriteCampaigns].
func (s *CampaignService) ResumeLead(ctx context.Context, campaignID, contactID string, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, leadPath(campaignID, contactID, "resume"), nil, nil, opts...)
}

// --- lead cc -----------------------------------------------------------

// leadCCResult is the answer the CC read and write share.
type leadCCResult struct {
	CC []CampaignLeadCC `json:"cc"`
}

// LeadCC lists the contacts copied on every email the campaign sends one
// lead. A contact that is not a lead of the campaign answers a 404.
//
// Requires [PermReadCampaigns] and [PermReadContacts].
func (s *CampaignService) LeadCC(ctx context.Context, campaignID, contactID string, opts ...RequestOption) ([]CampaignLeadCC, *Response, error) {
	out, resp, err := fetch[leadCCResult](ctx, s.client, leadPath(campaignID, contactID, "cc"), opts)
	if err != nil {
		return nil, resp, err
	}
	return nonNilLeadCC(out.CC), resp, nil
}

// SetLeadCC replaces the contacts copied on one lead's emails with
// contactIDs, up to [CampaignLeadMaxCC]; an empty list removes them all. It
// returns the resulting list with each copy's status. A retry lands on the
// same state.
//
// The server refuses a list that names the lead itself (400
// "lead_cc_self"), too many contacts ("lead_cc_limit"), a contact outside
// the workspace (404 "lead_cc_contact_not_found"), a lead that is itself
// copied on another lead's emails in this campaign ("lead_cc_lead_is_copied")
// and a contact that has copies of its own ("lead_cc_has_copies"), the last
// two as 409s.
//
// Requires [PermWriteCampaigns] and [PermReadContacts].
func (s *CampaignService) SetLeadCC(ctx context.Context, campaignID, contactID string, contactIDs []string, opts ...RequestOption) ([]CampaignLeadCC, *Response, error) {
	if contactIDs == nil {
		contactIDs = []string{}
	}
	body := struct {
		ContactIDs []string `json:"contact_ids"`
	}{ContactIDs: contactIDs}
	out, resp, err := send[leadCCResult](ctx, s.client.put, leadPath(campaignID, contactID, "cc"), body, opts)
	if err != nil {
		return nil, resp, err
	}
	return nonNilLeadCC(out.CC), resp, nil
}

// LeadCCSuggestions offers up to eight contacts who look like colleagues of
// the lead, same-company contacts first, for choosing who to copy.
//
// Requires [PermReadCampaigns] and [PermReadContacts].
func (s *CampaignService) LeadCCSuggestions(ctx context.Context, campaignID, contactID string, opts ...RequestOption) ([]LeadCCSuggestion, *Response, error) {
	return fetchData[LeadCCSuggestion](ctx, s.client, leadPath(campaignID, contactID, "cc/suggestions"), opts)
}

func nonNilLeadCC(cc []CampaignLeadCC) []CampaignLeadCC {
	if cc == nil {
		return []CampaignLeadCC{}
	}
	return cc
}
