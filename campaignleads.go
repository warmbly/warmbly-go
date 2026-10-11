package warmbly

import (
	"context"
	"net/url"
	"time"
)

// Per-lead controls inside one campaign: a hold that parks one contact's flow
// without unsubscribing them or removing them from the campaign, and the
// contacts copied on every email to one lead.

// Who placed a hold, returned in [LeadHold.Source].
const (
	// LeadHoldSourceManual is a member's pause ([CampaignService.PauseLead]).
	LeadHoldSourceManual = "manual"
	// LeadHoldSourceOutOfOffice is an auto-reply that said the contact is away.
	LeadHoldSourceOutOfOffice = "out_of_office"
	// LeadHoldSourceInboxTagging is a classified reply: "not now" for a
	// while, or a decline with no end.
	LeadHoldSourceInboxTagging = "inbox_tagging"
	// LeadHoldSourceCRM is a CRM rule: a lead status, a conversion or an open
	// opportunity in the connected CRM.
	LeadHoldSourceCRM = "crm"
	// LeadHoldSourceCC holds a contact's own lead while they are copied on
	// another lead's emails in the same campaign, so they never get two
	// sequences.
	LeadHoldSourceCC = "cc"
)

// LeadHold is one contact's flow parked inside one campaign. The lead keeps
// its place in the sequence and resumes where it stopped.
type LeadHold struct {
	Since time.Time `json:"since"`
	// Until is when the hold lifts by itself; nil holds with no end, which
	// only a resume lifts.
	Until  *time.Time `json:"until,omitempty"`
	Reason string     `json:"reason,omitempty"`
	// Source is one of the LeadHoldSource* constants.
	Source string `json:"source"`
}

// CampaignLeadHold is the hold state of one lead, returned by every hold
// method. Hold is nil when the lead is not held.
type CampaignLeadHold struct {
	CampaignID string    `json:"campaign_id"`
	ContactID  string    `json:"contact_id"`
	Hold       *LeadHold `json:"hold,omitempty"`
}

// LeadPauseParams is the optional body of [CampaignService.PauseLead].
type LeadPauseParams struct {
	// Until is when the hold lifts, in the future and within a year. Nil
	// holds with no end.
	Until *time.Time `json:"until,omitempty"`
	// Reason is a note kept on the hold, cut to 200 characters.
	Reason string `json:"reason,omitempty"`
}

// Whether the next email to a lead copies a contact, returned in
// [CampaignLeadCC.Status]. Anything but [LeadCCStatusActive] is left off.
const (
	LeadCCStatusActive = "active"
	// LeadCCStatusUnsubscribed is an opted-out or suppressed address.
	LeadCCStatusUnsubscribed = "unsubscribed"
	// LeadCCStatusBounced is an address that bounced on this thread or on any
	// campaign email of its own.
	LeadCCStatusBounced = "bounced"
	// LeadCCStatusUndeliverable is an address verification refused, under the
	// same rule the campaign applies to its leads.
	LeadCCStatusUndeliverable = "undeliverable"
)

// Why a contact is suggested as a copy, returned in
// [CampaignLeadCCSuggestion.Reason].
const (
	// LeadCCReasonCompany means the company names match.
	LeadCCReasonCompany = "company"
	// LeadCCReasonDomain means only the email domains match.
	LeadCCReasonDomain = "domain"
)

// Error codes ([Error.Code]) [CampaignService.SetLeadCC] can refuse with.
const (
	// ErrCodeLeadCCLimit (400): a lead can have at most 2 contacts copied.
	ErrCodeLeadCCLimit = "lead_cc_limit"
	// ErrCodeLeadCCSelf (400): a lead cannot be copied on their own emails.
	ErrCodeLeadCCSelf = "lead_cc_self"
	// ErrCodeLeadCCContactNotFound (404): a contact to copy is not in the
	// workspace.
	ErrCodeLeadCCContactNotFound = "lead_cc_contact_not_found"
	// ErrCodeLeadCCLeadIsCopied (409): the lead is itself copied on another
	// lead's emails in the campaign, so it sends none of its own.
	ErrCodeLeadCCLeadIsCopied = "lead_cc_lead_is_copied"
	// ErrCodeLeadCCHasCopies (409): a contact to copy has copies of its own
	// in the campaign; remove those first.
	ErrCodeLeadCCHasCopies = "lead_cc_has_copies"
)

// CampaignLeadCC is one contact copied on every email to a lead.
type CampaignLeadCC struct {
	ContactID string `json:"contact_id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Company   string `json:"company,omitempty"`
	// Status is one of the LeadCCStatus* constants.
	Status    string     `json:"status"`
	BouncedAt *time.Time `json:"bounced_at,omitempty"`
}

// CampaignLeadCCList is the contacts copied on one lead of a campaign.
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
	// Reason is [LeadCCReasonCompany] or [LeadCCReasonDomain].
	Reason string `json:"reason"`
}

// LeadHold reads whether one lead's flow in the campaign is currently held. A
// contact that is not a lead of the campaign is a 404.
//
// Requires [PermReadCampaigns].
func (s *CampaignService) LeadHold(ctx context.Context, campaignID, contactID string, opts ...RequestOption) (*CampaignLeadHold, *Response, error) {
	return fetch[CampaignLeadHold](ctx, s.client, campaignLeadPath(campaignID, contactID)+"/hold", opts)
}

// PauseLead parks one contact's flow in the campaign until params.Until, or
// with no end when params is nil or Until is nil. The contact stays subscribed
// and stays a lead: this is not an unsubscribe or a suppression. The request
// states an absolute hold and replacing a live one keeps its start, so a retry
// lands on the same state.
//
// Requires [PermWriteCampaigns].
func (s *CampaignService) PauseLead(ctx context.Context, campaignID, contactID string, params *LeadPauseParams, opts ...RequestOption) (*CampaignLeadHold, *Response, error) {
	return send[CampaignLeadHold](ctx, s.client.post, campaignLeadPath(campaignID, contactID)+"/pause", params, opts)
}

// ResumeLead lifts the lead's hold now. Resuming a lead that is not held
// succeeds and changes nothing; a contact that is not a lead of the campaign
// is a 404.
//
// Requires [PermWriteCampaigns].
func (s *CampaignService) ResumeLead(ctx context.Context, campaignID, contactID string, opts ...RequestOption) (*CampaignLeadHold, *Response, error) {
	return send[CampaignLeadHold](ctx, s.client.post, campaignLeadPath(campaignID, contactID)+"/resume", nil, opts)
}

// LeadCC lists the contacts copied on every email to one lead of the
// campaign. A contact that is not a lead of the campaign is a 404.
//
// Requires [PermReadCampaigns] and [PermReadContacts].
func (s *CampaignService) LeadCC(ctx context.Context, campaignID, contactID string, opts ...RequestOption) (*CampaignLeadCCList, *Response, error) {
	return fetch[CampaignLeadCCList](ctx, s.client, campaignLeadPath(campaignID, contactID)+"/cc", opts)
}

// SetLeadCC replaces the contacts copied on one lead, at most 2. An empty
// list removes them all. A copied contact's own lead in the campaign is held
// ([LeadHoldSourceCC]) while they are copied, and released when removed. The
// body is the whole list, so a retry is safe. Refusals carry one of the
// ErrCodeLeadCC* codes.
//
// Requires [PermWriteCampaigns] and [PermReadContacts].
func (s *CampaignService) SetLeadCC(ctx context.Context, campaignID, contactID string, contactIDs []string, opts ...RequestOption) (*CampaignLeadCCList, *Response, error) {
	if contactIDs == nil {
		contactIDs = []string{}
	}
	body := struct {
		ContactIDs []string `json:"contact_ids"`
	}{ContactIDs: contactIDs}
	return send[CampaignLeadCCList](ctx, s.client.put, campaignLeadPath(campaignID, contactID)+"/cc", body, opts)
}

// LeadCCSuggestions offers up to 8 of the lead's likely colleagues to copy,
// matched by company name or email domain.
//
// Requires [PermReadCampaigns] and [PermReadContacts].
func (s *CampaignService) LeadCCSuggestions(ctx context.Context, campaignID, contactID string, opts ...RequestOption) ([]CampaignLeadCCSuggestion, *Response, error) {
	return fetchData[CampaignLeadCCSuggestion](ctx, s.client, campaignLeadPath(campaignID, contactID)+"/cc/suggestions", opts)
}

func campaignLeadPath(campaignID, contactID string) string {
	return "campaigns/" + url.PathEscape(campaignID) + "/leads/" + url.PathEscape(contactID)
}
