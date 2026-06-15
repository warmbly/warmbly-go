package warmbly

import (
	"context"
	"net/url"
	"time"
)

// CampaignService manages outreach campaigns and their sequence steps.
//
// A campaign is an ordered sequence of steps (emails, waits and actions)
// delivered to enrolled contacts from one or more connected email accounts.
// Steps are addressed as a sub-resource under their campaign.
type CampaignService service

// Campaign is an outreach campaign as returned by the API.
type Campaign struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	// Status is the lifecycle state, for example "draft", "running" or
	// "stopped".
	Status string `json:"status"`
	// StopOnReply halts a contact's sequence as soon as they reply.
	StopOnReply bool `json:"stop_on_reply"`
	// OpenTracking enables open tracking via a tracking pixel.
	OpenTracking bool `json:"open_tracking"`
	// LinkTracking enables click tracking by rewriting links.
	LinkTracking bool `json:"link_tracking"`
	// DailyLimit is the maximum number of new sends per day for the campaign.
	DailyLimit int `json:"daily_limit"`
	// UnsubscribeHeader controls whether a List-Unsubscribe header is added.
	UnsubscribeHeader bool `json:"unsubscribe_header"`
	// CC and BCC are addresses copied on every send.
	CC  []string `json:"cc"`
	BCC []string `json:"bcc"`
	// StartDate and EndDate bound the active sending window, when set.
	StartDate *time.Time `json:"start_date,omitempty"`
	EndDate   *time.Time `json:"end_date,omitempty"`
	// Timezone is the IANA timezone used to interpret the sending schedule.
	Timezone string `json:"timezone"`
	// SenderStrategy selects how sending accounts are chosen: "tags" picks from
	// accounts matching configured tags, "explicit" uses Senders verbatim.
	SenderStrategy string `json:"sender_strategy"`
	// RotationMode controls how sends are distributed across sender accounts.
	RotationMode string `json:"rotation_mode"`
	// Senders are the email-account IDs used to send, when SenderStrategy is
	// "explicit".
	Senders []string `json:"senders,omitempty"`
	// RampEnabled gradually increases daily volume for new sender accounts.
	RampEnabled bool `json:"ramp_enabled"`
	// RampStart is the initial daily volume per sender when ramping.
	RampStart int `json:"ramp_start"`
	// RampIncrement is the daily increase per sender while ramping.
	RampIncrement int `json:"ramp_increment"`
	// RampCeiling is the maximum daily volume per sender once ramped.
	RampCeiling int `json:"ramp_ceiling"`
	// TrackingDomain is the custom domain used for tracking links, when set.
	TrackingDomain string    `json:"tracking_domain,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Step is a single step in a campaign's sequence.
type Step struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Subject string `json:"subject"`
	// BodyPlain is the plain-text body of the message.
	BodyPlain string `json:"body_plain"`
	// BodyHTML is the HTML body of the message.
	BodyHTML string `json:"body_html"`
	// WaitAfter is the number of minutes to wait after this step before the
	// next one runs.
	WaitAfter int `json:"wait_after"`
	// Position is the step's zero-based index in the sequence.
	Position int `json:"position"`
	// Kind is the step type: "email", "action" or "wait".
	Kind string `json:"kind"`
}

// CampaignLogEntry is a single entry from a campaign's activity log.
type CampaignLogEntry struct {
	ID string `json:"id"`
	// Level is the severity, for example "info", "warning" or "error".
	Level   string `json:"level"`
	Message string `json:"message"`
	// ContactID identifies the related contact, when the entry concerns one.
	ContactID string    `json:"contact_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// CampaignCreateParams are the parameters for creating a campaign. Only Name is
// required; the remaining knobs fall back to server defaults when omitted.
type CampaignCreateParams struct {
	Name           string   `json:"name"`
	Description    string   `json:"description,omitempty"`
	DailyLimit     int      `json:"daily_limit,omitempty"`
	StopOnReply    bool     `json:"stop_on_reply,omitempty"`
	OpenTracking   bool     `json:"open_tracking,omitempty"`
	LinkTracking   bool     `json:"link_tracking,omitempty"`
	Timezone       string   `json:"timezone,omitempty"`
	SenderStrategy string   `json:"sender_strategy,omitempty"`
	Senders        []string `json:"senders,omitempty"`
}

// CampaignUpdateParams are the parameters for updating a campaign. Only non-nil
// fields are sent, so zero values are not mistaken for "clear this field".
type CampaignUpdateParams struct {
	Name           *string    `json:"name,omitempty"`
	Description    *string    `json:"description,omitempty"`
	Status         *string    `json:"status,omitempty"`
	StopOnReply    *bool      `json:"stop_on_reply,omitempty"`
	OpenTracking   *bool      `json:"open_tracking,omitempty"`
	LinkTracking   *bool      `json:"link_tracking,omitempty"`
	DailyLimit     *int       `json:"daily_limit,omitempty"`
	Timezone       *string    `json:"timezone,omitempty"`
	SenderStrategy *string    `json:"sender_strategy,omitempty"`
	Senders        *[]string  `json:"senders,omitempty"`
	StartDate      *time.Time `json:"start_date,omitempty"`
	EndDate        *time.Time `json:"end_date,omitempty"`
}

// StepParams are the parameters for creating or updating a campaign step. For
// updates, omitted fields leave the existing value unchanged.
type StepParams struct {
	Name      string `json:"name,omitempty"`
	Subject   string `json:"subject,omitempty"`
	BodyPlain string `json:"body_plain,omitempty"`
	BodyHTML  string `json:"body_html,omitempty"`
	WaitAfter *int   `json:"wait_after,omitempty"`
	Position  *int   `json:"position,omitempty"`
	Kind      string `json:"kind,omitempty"`
}

// CampaignTestEmailParams are the parameters for sending a test email.
type CampaignTestEmailParams struct {
	// To is the recipient address for the test send.
	To string `json:"to"`
	// StepID optionally selects which step to render; the first step is used
	// when empty.
	StepID string `json:"step_id,omitempty"`
}

// CampaignListParams filters and paginates a list of campaigns.
type CampaignListParams struct {
	ListOptions
	// Search filters by name substring.
	Search string
	// Status filters by lifecycle status, for example "running".
	Status string
}

func (p *CampaignListParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	if p.Search != "" {
		q.Set("search", p.Search)
	}
	if p.Status != "" {
		q.Set("status", p.Status)
	}
	return q
}

// List returns a page of campaigns.
func (s *CampaignService) List(ctx context.Context, params *CampaignListParams) (*Page[Campaign], error) {
	return listJSON[Campaign](ctx, s.client, "campaigns", params.values())
}

// Create creates a new campaign.
func (s *CampaignService) Create(ctx context.Context, params *CampaignCreateParams) (*Campaign, *Response, error) {
	campaign := new(Campaign)
	resp, err := s.client.post(ctx, "campaigns", params, campaign)
	if err != nil {
		return nil, resp, err
	}
	return campaign, resp, nil
}

// Get retrieves a single campaign by ID.
func (s *CampaignService) Get(ctx context.Context, id string) (*Campaign, *Response, error) {
	campaign := new(Campaign)
	resp, err := s.client.get(ctx, "campaigns/"+url.PathEscape(id), campaign)
	if err != nil {
		return nil, resp, err
	}
	return campaign, resp, nil
}

// Update modifies an existing campaign.
func (s *CampaignService) Update(ctx context.Context, id string, params *CampaignUpdateParams) (*Campaign, *Response, error) {
	campaign := new(Campaign)
	resp, err := s.client.patch(ctx, "campaigns/"+url.PathEscape(id), params, campaign)
	if err != nil {
		return nil, resp, err
	}
	return campaign, resp, nil
}

// Delete permanently deletes a campaign.
func (s *CampaignService) Delete(ctx context.Context, id string) (*Response, error) {
	return s.client.delete(ctx, "campaigns/"+url.PathEscape(id))
}

// Start begins (or resumes) sending for a campaign.
func (s *CampaignService) Start(ctx context.Context, id string) (*Campaign, *Response, error) {
	campaign := new(Campaign)
	resp, err := s.client.post(ctx, "campaigns/"+url.PathEscape(id)+"/start", nil, campaign)
	if err != nil {
		return nil, resp, err
	}
	return campaign, resp, nil
}

// Stop pauses sending for a campaign.
func (s *CampaignService) Stop(ctx context.Context, id string) (*Campaign, *Response, error) {
	campaign := new(Campaign)
	resp, err := s.client.post(ctx, "campaigns/"+url.PathEscape(id)+"/stop", nil, campaign)
	if err != nil {
		return nil, resp, err
	}
	return campaign, resp, nil
}

// Logs returns a page of activity-log entries for a campaign.
func (s *CampaignService) Logs(ctx context.Context, id string, params *ListOptions) (*Page[CampaignLogEntry], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[CampaignLogEntry](ctx, s.client, "campaigns/"+url.PathEscape(id)+"/logs", q)
}

// TestEmail sends a one-off test email rendering a campaign step.
func (s *CampaignService) TestEmail(ctx context.Context, id string, params *CampaignTestEmailParams) (*Response, error) {
	return s.client.post(ctx, "campaigns/"+url.PathEscape(id)+"/test-email", params, nil)
}

// Senders returns the email-account IDs configured to send for a campaign.
func (s *CampaignService) Senders(ctx context.Context, id string) ([]string, *Response, error) {
	var out struct {
		Data []string `json:"data"`
	}
	resp, err := s.client.get(ctx, "campaigns/"+url.PathEscape(id)+"/senders", &out)
	if err != nil {
		return nil, resp, err
	}
	return out.Data, resp, nil
}

// SetSenders replaces the email-account IDs configured to send for a campaign
// and returns the resulting set.
func (s *CampaignService) SetSenders(ctx context.Context, id string, senders []string) ([]string, *Response, error) {
	body := struct {
		Senders []string `json:"senders"`
	}{Senders: senders}
	var out struct {
		Data []string `json:"data"`
	}
	resp, err := s.client.put(ctx, "campaigns/"+url.PathEscape(id)+"/senders", body, &out)
	if err != nil {
		return nil, resp, err
	}
	return out.Data, resp, nil
}

// ListSteps returns the ordered steps of a campaign.
func (s *CampaignService) ListSteps(ctx context.Context, id string) ([]Step, *Response, error) {
	var out struct {
		Data []Step `json:"data"`
	}
	resp, err := s.client.get(ctx, "campaigns/"+url.PathEscape(id)+"/steps", &out)
	if err != nil {
		return nil, resp, err
	}
	return out.Data, resp, nil
}

// CreateStep appends a new step to a campaign's sequence.
func (s *CampaignService) CreateStep(ctx context.Context, id string, params *StepParams) (*Step, *Response, error) {
	step := new(Step)
	resp, err := s.client.post(ctx, "campaigns/"+url.PathEscape(id)+"/steps", params, step)
	if err != nil {
		return nil, resp, err
	}
	return step, resp, nil
}

// UpdateStep modifies an existing step within a campaign.
func (s *CampaignService) UpdateStep(ctx context.Context, id, stepID string, params *StepParams) (*Step, *Response, error) {
	step := new(Step)
	resp, err := s.client.patch(ctx, "campaigns/"+url.PathEscape(id)+"/steps/"+url.PathEscape(stepID), params, step)
	if err != nil {
		return nil, resp, err
	}
	return step, resp, nil
}

// DeleteStep removes a step from a campaign's sequence.
func (s *CampaignService) DeleteStep(ctx context.Context, id, stepID string) (*Response, error) {
	return s.client.delete(ctx, "campaigns/"+url.PathEscape(id)+"/steps/"+url.PathEscape(stepID))
}
