package warmbly

import (
	"context"
	"encoding/json"
	"net/url"
	"time"
)

// CampaignReviewStatus is a lead's review state, derived from its hold and
// whether it was emailed. New states may appear, so compare against the
// constants and tolerate other values.
type CampaignReviewStatus string

const (
	// CampaignReviewWaiting is held for review: nothing goes out until it is
	// approved.
	CampaignReviewWaiting CampaignReviewStatus = "waiting"
	// CampaignReviewReady has not been emailed yet and nothing keeps it from
	// starting.
	CampaignReviewReady CampaignReviewStatus = "ready"
	// CampaignReviewHeld has not been emailed yet and is paused for another
	// reason.
	CampaignReviewHeld CampaignReviewStatus = "held"
	// CampaignReviewStarted has been emailed, or has a send in flight.
	CampaignReviewStarted CampaignReviewStatus = "started"
	// CampaignReviewAll is the [CampaignReviewLeadsParams.Status] filter that
	// keeps every lead, the same as leaving it empty. It never appears on a
	// lead.
	CampaignReviewAll CampaignReviewStatus = "all"
)

// CampaignReviewField is a contact field the review readiness check counts
// blanks of. The values are the merge field names as templates print them.
type CampaignReviewField string

const (
	CampaignReviewFieldFirstName CampaignReviewField = "FirstName"
	CampaignReviewFieldLastName  CampaignReviewField = "LastName"
	CampaignReviewFieldCompany   CampaignReviewField = "Company"
	CampaignReviewFieldPhone     CampaignReviewField = "Phone"
)

// CampaignReviewWriteStop names why a [CampaignService.WriteReviewAI] pass
// ended early ([CampaignReviewWriteResult.Stopped]). New reasons may appear.
type CampaignReviewWriteStop string

const (
	// CampaignReviewStopInsufficientCredits is a balance too low for the next
	// block.
	CampaignReviewStopInsufficientCredits CampaignReviewWriteStop = "insufficient_credits"
	// CampaignReviewStopUsageCap is the workspace reaching its AI spend limit.
	CampaignReviewStopUsageCap CampaignReviewWriteStop = "usage_cap_exceeded"
)

// CampaignReview is a campaign's review readiness report: how its leads split
// by review state, the merge fields some unstarted leads would leave blank,
// what sending skips, steps whose copy does not render, and how much of the
// first email's AI copy is written ahead.
type CampaignReview struct {
	CampaignID  string               `json:"campaign_id"`
	ReviewLeads bool                 `json:"review_leads"`
	Counts      CampaignReviewCounts `json:"counts"`
	// Missing counts unstarted, subscribed leads with a field empty that an
	// email step prints with nothing to fall back on, largest first.
	Missing []CampaignReviewMissingField `json:"missing"`
	// Undeliverable counts leads whose address verification rejected.
	Undeliverable int `json:"undeliverable"`
	// Unsubscribed counts leads who opted out; sending skips them.
	Unsubscribed int              `json:"unsubscribed"`
	AI           CampaignReviewAI `json:"ai"`
	// TemplateErrors names email steps whose copy does not render as written.
	TemplateErrors []CampaignStepIssue `json:"template_errors,omitempty"`
}

// CampaignReviewCounts is how a campaign's leads split by review state.
type CampaignReviewCounts struct {
	Total   int `json:"total"`
	Waiting int `json:"waiting"`
	Ready   int `json:"ready"`
	Held    int `json:"held"`
	Started int `json:"started"`
	// Personalized counts leads with their own copy of at least one step.
	Personalized int `json:"personalized"`
}

// CampaignReviewMissingField is one field the emails print that some
// unstarted leads have empty.
type CampaignReviewMissingField struct {
	Field CampaignReviewField `json:"field"`
	Leads int                 `json:"leads"`
}

// CampaignReviewAI is how much of the first email's AI copy is written ahead.
type CampaignReviewAI struct {
	// StepID is the first email step, nil when the campaign has none.
	StepID *string `json:"step_id,omitempty"`
	// Blocks is how many AI blocks the first email has.
	Blocks int `json:"blocks"`
	// Written counts unstarted leads whose first email has every block
	// written.
	Written int `json:"written"`
	// Pending counts unstarted leads with at least one block still to write.
	Pending int `json:"pending"`
	// EstimatedCostUSD is about what writing the pending blocks costs, at the
	// per-block estimate the balance check reserves.
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
	// Auto is true while the instance writes the pending leads on its own (an
	// active campaign with AI available), with no call needed.
	Auto bool `json:"auto"`
}

// CampaignStepIssue names a step and what is wrong with it.
type CampaignStepIssue struct {
	StepID   string `json:"step_id"`
	Position int    `json:"position"`
	Message  string `json:"message"`
}

// CampaignReviewChange is the answer to a review write: how many leads moved.
type CampaignReviewChange struct {
	CampaignID  string `json:"campaign_id"`
	ReviewLeads bool   `json:"review_leads"`
	// Held counts leads put into review; Released counts leads approved.
	Held     int `json:"held"`
	Released int `json:"released"`
}

// CampaignReviewLead is one lead of a campaign with its review state.
type CampaignReviewLead struct {
	ContactID string               `json:"contact_id"`
	Email     string               `json:"email"`
	FirstName string               `json:"first_name"`
	LastName  string               `json:"last_name"`
	Company   string               `json:"company"`
	Status    CampaignReviewStatus `json:"status"`
	// Hold is nil when the lead is not held. A lead waiting for review shows
	// the [LeadHoldReview] hold ahead of any other it has.
	Hold *LeadHold `json:"hold,omitempty"`
	// Personalized counts the steps with this lead's own copy.
	Personalized int `json:"personalized"`
	// Missing lists the fields the campaign's emails print that this lead
	// has empty.
	Missing []CampaignReviewField `json:"missing,omitempty"`
	AddedAt *time.Time            `json:"added_at,omitempty"`
}

// CampaignReviewLeadsParams filters [CampaignService.ReviewLeads]. Leads are
// ordered by email. Limit is 1 to 200 (default 50).
type CampaignReviewLeadsParams struct {
	ListOptions
	// Status keeps leads in one review state; empty or [CampaignReviewAll]
	// keeps every lead.
	Status CampaignReviewStatus
	// Missing keeps leads with this field empty.
	Missing CampaignReviewField
	// Query matches email, name or company.
	Query string
}

func (p *CampaignReviewLeadsParams) values() url.Values {
	q := url.Values{}
	if p == nil {
		return q
	}
	p.apply(q)
	setNonEmpty(q, "status", string(p.Status))
	setNonEmpty(q, "missing", string(p.Missing))
	setNonEmpty(q, "q", p.Query)
	return q
}

// CampaignReviewApproveParams picks the leads [CampaignService.ApproveReviewLeads]
// releases. Send ContactIDs (at most 500) or All, not both.
type CampaignReviewApproveParams struct {
	ContactIDs []string `json:"contact_ids,omitempty"`
	// All approves whoever is waiting when it runs, so repeating it also
	// approves leads that started waiting since the first call. Retry with
	// the ContactIDs you meant to approve instead.
	All bool `json:"all,omitempty"`
}

// CampaignReviewWriteParams scopes one [CampaignService.WriteReviewAI] pass.
// The zero value writes the first email step for every lead not yet emailed.
type CampaignReviewWriteParams struct {
	// StepID is the email step to write; empty is the first one.
	StepID string
	// ContactIDs limits the pass to these leads (1 to 500). Nil writes every
	// lead in scope. A non-nil empty slice is sent as an empty list, which the
	// server refuses with 400 so a bug cannot widen a paid pass to every lead.
	ContactIDs []string
	// WaitingOnly, without ContactIDs, limits the pass to leads waiting for
	// review.
	WaitingOnly bool
	// After is the previous pass's [CampaignReviewWriteResult.Next].
	After string
}

// MarshalJSON keeps a nil ContactIDs off the wire and sends a non-nil empty
// one as [], so the server can tell "every lead" from "no leads".
func (p CampaignReviewWriteParams) MarshalJSON() ([]byte, error) {
	var ids *[]string
	if p.ContactIDs != nil {
		ids = &p.ContactIDs
	}
	return json.Marshal(struct {
		StepID      string    `json:"step_id,omitempty"`
		ContactIDs  *[]string `json:"contact_ids,omitempty"`
		WaitingOnly bool      `json:"waiting_only,omitempty"`
		After       string    `json:"after,omitempty"`
	}{p.StepID, ids, p.WaitingOnly, p.After})
}

// CampaignReviewWriteResult is one pass of writing AI copy ahead.
type CampaignReviewWriteResult struct {
	Written int `json:"written"`
	// Failed counts leads whose copy could not be written; they are written
	// at send time instead.
	Failed int `json:"failed"`
	// Remaining counts leads in the request's scope still to write after
	// this pass.
	Remaining int `json:"remaining"`
	// Stopped is set when the pass ended early. The pass keeps what it wrote
	// and Next resumes where it began.
	Stopped CampaignReviewWriteStop `json:"stopped,omitempty"`
	// Next is the cursor the following pass sends as
	// [CampaignReviewWriteParams.After]. It moves past every lead a pass
	// tried, so a lead that keeps failing cannot stall the run.
	Next string `json:"next,omitempty"`
}

// LeadEmailCopy is the subject and bodies of one email: the template a lead's
// copy renders from ([CampaignLeadEmailStep.Source]), and the body of
// [CampaignService.SetLeadEmail].
type LeadEmailCopy struct {
	// Subject is up to 998 characters. It is ignored on a follow-up that
	// replies in the lead's thread.
	Subject string `json:"subject"`
	// BodyHTML and BodyPlain: send one or both.
	BodyHTML  string `json:"body_html"`
	BodyPlain string `json:"body_plain"`
}

// LeadStepOverride is one lead's own copy of one email step.
type LeadStepOverride struct {
	CampaignID string `json:"campaign_id"`
	ContactID  string `json:"contact_id"`
	StepID     string `json:"step_id"`
	Subject    string `json:"subject"`
	BodyHTML   string `json:"body_html"`
	BodyPlain  string `json:"body_plain"`
	// EditedBy is the member who wrote the copy, nil for an API key.
	EditedBy  *string   `json:"edited_by,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LeadEmailAIBlock is one AI block in a step, and whether its copy is
// written yet.
type LeadEmailAIBlock struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Written bool   `json:"written"`
}

// LeadEmailFrom is the mailbox a lead's emails go out from.
type LeadEmailFrom struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// CampaignLeadEmailStep is one email step rendered for one lead as it will
// be sent. In BodyHTML, written AI copy is wrapped in
// <span data-ai-block="ID"> and an unwritten block is an empty
// <span data-ai-pending="ID" data-ai-name="...">; BodyPlain shows
// [AI: name] for an unwritten block. These markers are never sent.
type CampaignLeadEmailStep struct {
	StepID    string `json:"step_id"`
	Position  int    `json:"position"`
	Name      string `json:"name"`
	Subject   string `json:"subject"`
	BodyHTML  string `json:"body_html"`
	BodyPlain string `json:"body_plain"`
	// Source is the template this lead's copy renders from: their own copy,
	// the A/B arm drawn for them, or the step's. A personalization starts
	// from it.
	Source LeadEmailCopy `json:"source"`
	// Threaded is a follow-up that replies in the lead's thread, so its
	// subject is the conversation's.
	Threaded bool `json:"threaded"`
	// Personalized is this lead's own copy, which replaces the step's.
	Personalized bool       `json:"personalized"`
	EditedAt     *time.Time `json:"edited_at,omitempty"`
	// StepChanged is a personalized step whose shared copy was edited after
	// it.
	StepChanged bool `json:"step_changed"`
	// VariantID is the A/B variant this lead gets, nil for the step's own
	// copy.
	VariantID *string            `json:"variant_id,omitempty"`
	AIBlocks  []LeadEmailAIBlock `json:"ai_blocks,omitempty"`
	// Missing lists fields the copy prints that this lead has empty.
	Missing []CampaignReviewField `json:"missing,omitempty"`
	Errors  []string              `json:"errors,omitempty"`
	Sent    bool                  `json:"sent"`
	SentAt  *time.Time            `json:"sent_at,omitempty"`
}

// CampaignLeadEmails is every email step of a campaign rendered for one lead.
type CampaignLeadEmails struct {
	CampaignID string             `json:"campaign_id"`
	Lead       CampaignReviewLead `json:"lead"`
	// From is set once a mailbox has written to the lead.
	From  *LeadEmailFrom          `json:"from,omitempty"`
	Steps []CampaignLeadEmailStep `json:"steps"`
}

// LeadEmailWriteParams is the optional body of
// [CampaignService.WriteLeadEmailAI].
type LeadEmailWriteParams struct {
	// Rewrite replaces copy already written with a new generation, which is
	// charged again.
	Rewrite bool `json:"rewrite,omitempty"`
}

func reviewPath(campaignID, suffix string) string {
	return "campaigns/" + url.PathEscape(campaignID) + "/review" + suffix
}

func leadEmailPath(campaignID, contactID, stepID, suffix string) string {
	return leadPath(campaignID, contactID, "/emails/"+url.PathEscape(stepID)+suffix)
}

// Review returns a campaign's review readiness report. Needs the
// read_campaigns scope.
func (s *CampaignService) Review(ctx context.Context, id string, opts ...RequestOption) (*CampaignReview, *Response, error) {
	return fetch[CampaignReview](ctx, s.client, reviewPath(id, ""), opts)
}

// SetReview turns review mode on or off. On, every lead not yet emailed, and
// every lead enrolled later, waits until a member approves its first email;
// follow-ups after it go out on their own. Off approves every waiting lead.
// Only a change moves leads: sending the mode the campaign already has
// changes nothing, so a retry is safe. Needs the write_campaigns scope, and
// send_campaigns too when enabled is false.
func (s *CampaignService) SetReview(ctx context.Context, id string, enabled bool, opts ...RequestOption) (*CampaignReviewChange, *Response, error) {
	body := struct {
		Enabled bool `json:"enabled"`
	}{enabled}
	return send[CampaignReviewChange](ctx, s.client.put, reviewPath(id, ""), body, opts)
}

// ReviewLeads pages a campaign's leads with their review state, ordered by
// email. Needs the read_campaigns and read_contacts scopes.
func (s *CampaignService) ReviewLeads(ctx context.Context, id string, params *CampaignReviewLeadsParams, opts ...RequestOption) (*Page[CampaignReviewLead], error) {
	return listJSON[CampaignReviewLead](ctx, s.client, reviewPath(id, "/leads"), params.values(), opts...)
}

// ApproveReviewLeads releases leads waiting for review. Their first email
// goes out under the campaign's schedule and every other sending rule, and
// a lead also paused for another reason keeps that pause. Approving a lead
// that is not waiting changes nothing, so retrying the same ContactIDs is
// safe; see [CampaignReviewApproveParams.All] for retrying All. Needs the
// send_campaigns scope.
func (s *CampaignService) ApproveReviewLeads(ctx context.Context, id string, params *CampaignReviewApproveParams, opts ...RequestOption) (*CampaignReviewChange, *Response, error) {
	if params == nil {
		params = &CampaignReviewApproveParams{}
	}
	return send[CampaignReviewChange](ctx, s.client.post, reviewPath(id, "/approve"), params, opts)
}

// ReopenReviewLeads puts the listed leads (1 to 500) back into review. The
// campaign must have review mode on, and a lead already emailed is left as
// it is. A lead already waiting stays waiting, so a retry is safe. Needs the
// write_campaigns scope.
func (s *CampaignService) ReopenReviewLeads(ctx context.Context, id string, contactIDs []string, opts ...RequestOption) (*CampaignReviewChange, *Response, error) {
	body := struct {
		ContactIDs []string `json:"contact_ids"`
	}{contactIDs}
	return send[CampaignReviewChange](ctx, s.client.post, reviewPath(id, "/reopen"), body, opts)
}

// WriteReviewAI writes the AI blocks of the first email step, or
// [CampaignReviewWriteParams.StepID], ahead for up to ten leads not yet
// emailed that still miss one. Each block is charged to the balance as it
// would be at send time and cached where the send reads it, so the send
// reuses it. Each pass skips what is already written, so a retry charges
// nothing twice. Call it again with Next as After while Remaining is above
// zero. A lead with its own copy of the step is skipped. Needs the
// write_campaigns scope (and the use_ai org permission for a member).
func (s *CampaignService) WriteReviewAI(ctx context.Context, id string, params *CampaignReviewWriteParams, opts ...RequestOption) (*CampaignReviewWriteResult, *Response, error) {
	var body any
	if params != nil {
		body = params
	}
	return send[CampaignReviewWriteResult](ctx, s.client.post, reviewPath(id, "/write"), body, opts)
}

// LeadEmails renders every email step for one lead the way the send will:
// their own copy or the A/B variant drawn for them, merge fields, spintax,
// the AI copy written so far, the signature and the opt-out footer. Nothing
// is written or charged. A contact that is not a lead of the campaign is a
// 404. Needs the read_campaigns and read_contacts scopes.
func (s *CampaignService) LeadEmails(ctx context.Context, id, contactID string, opts ...RequestOption) (*CampaignLeadEmails, *Response, error) {
	return fetch[CampaignLeadEmails](ctx, s.client, leadPath(id, contactID, "/emails"), opts)
}

// SetLeadEmail stores one lead's own copy of one email step. It replaces the
// step's copy, and any A/B variant, for this lead only; merge fields, spintax
// and AI blocks in it still resolve when it sends, and later edits to the
// step do not change it. The body replaces the whole copy, so a retry lands
// on the same row. Once the lead received the email, or while it is being
// sent, the server answers 409 lead_step_sent. Needs the write_campaigns and
// read_contacts scopes.
func (s *CampaignService) SetLeadEmail(ctx context.Context, id, contactID, stepID string, params *LeadEmailCopy, opts ...RequestOption) (*LeadStepOverride, *Response, error) {
	if params == nil {
		params = &LeadEmailCopy{}
	}
	return send[LeadStepOverride](ctx, s.client.put, leadEmailPath(id, contactID, stepID, ""), params, opts)
}

// ResetLeadEmail drops one lead's own copy of a step, so the step's copy goes
// out again. Deleting a copy that is not there is a success. Once the lead
// received the email, or while it is being sent, the server answers 409
// lead_step_sent. Needs the write_campaigns and read_contacts scopes.
func (s *CampaignService) ResetLeadEmail(ctx context.Context, id, contactID, stepID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, leadEmailPath(id, contactID, stepID, ""), opts...)
}

// WriteLeadEmailAI writes one step's AI blocks for one lead now instead of at
// send time, and answers the lead's emails as [CampaignService.LeadEmails]
// does. The copy is charged and cached where the send reads it, so a retry
// writes and charges nothing more. Blocks are saved one at a time, so a call
// that fails partway keeps and charges the blocks it finished.
//
// With params.Rewrite the copy already written is replaced by a new
// generation and charged again. A repeated rewrite is another charge, so
// do not retry one blindly: pass [WithIdempotencyKey], or build the client
// with [WithMaxRetries](0) for it.
//
// Needs the write_campaigns and read_contacts scopes (and the use_ai org
// permission for a member).
func (s *CampaignService) WriteLeadEmailAI(ctx context.Context, id, contactID, stepID string, params *LeadEmailWriteParams, opts ...RequestOption) (*CampaignLeadEmails, *Response, error) {
	var body any
	if params != nil {
		body = params
	}
	return send[CampaignLeadEmails](ctx, s.client.post, leadEmailPath(id, contactID, stepID, "/write"), body, opts)
}
