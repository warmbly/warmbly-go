package warmbly

import (
	"context"
	"net/url"
	"time"
)

// EmailService manages connected email accounts, their warmup configuration and
// the messages sent through them.
type EmailService service

// Email is a connected email account (mailbox) as returned by the API.
type Email struct {
	ID             string  `json:"id"`
	OrganizationID *string `json:"organization_id,omitempty"`
	Email          string  `json:"email"`
	Name           string  `json:"name"`
	// Provider is the mailbox backend: "gmail", "outlook" or "smtp_imap".
	Provider string `json:"provider"`
	// Status is the connection state: "active", "inactive" or "revoked".
	Status         string `json:"status"`
	SignaturePlain string `json:"signature_plain"`
	SignatureHTML  string `json:"signature_html"`
	// WorkerID identifies the worker currently handling this mailbox, when assigned.
	WorkerID *string `json:"worker_id,omitempty"`
	// Warmup is the warmup anchor timestamp; a nil value means warmup is disabled.
	Warmup *time.Time `json:"warmup,omitempty"`
	// WarmupPausedAt is when warmup was paused, when it is currently paused.
	WarmupPausedAt         *time.Time `json:"warmup_paused_at,omitempty"`
	WarmupBase             int        `json:"warmup_base"`
	WarmupMax              int        `json:"warmup_max"`
	WarmupIncrease         int        `json:"warmup_increase"`
	WarmupReplyRate        int        `json:"warmup_reply_rate"`
	WarmupPoolType         string     `json:"warmup_pool_type"`
	TrackingDomain         string     `json:"tracking_domain"`
	TrackingDomainVerified bool       `json:"tracking_domain_verified"`
	CampaignLimit          int        `json:"campaign_limit"`
	Tags                   []string   `json:"tags"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

// WarmupEnabled reports whether warmup is enabled for this mailbox.
func (e *Email) WarmupEnabled() bool { return e.Warmup != nil }

// WarmupBanStatus describes whether a mailbox has been banned from the warmup
// pool and whether that ban can be appealed.
type WarmupBanStatus struct {
	Banned bool   `json:"banned"`
	Reason string `json:"reason,omitempty"`
	// Since is when the ban took effect, when banned.
	Since      *time.Time `json:"since,omitempty"`
	Appealable bool       `json:"appealable"`
}

// SendEmailParams are the parameters for sending a one-off message from a mailbox.
type SendEmailParams struct {
	To        []string `json:"to"`
	CC        []string `json:"cc,omitempty"`
	BCC       []string `json:"bcc,omitempty"`
	Subject   string   `json:"subject"`
	BodyPlain string   `json:"body_plain,omitempty"`
	BodyHTML  string   `json:"body_html,omitempty"`
	ReplyTo   string   `json:"reply_to,omitempty"`
}

// SendResult is returned when a message has been accepted for delivery.
type SendResult struct {
	MessageID string    `json:"message_id"`
	QueuedAt  time.Time `json:"queued_at"`
}

// WarmupAppealParams are the parameters for appealing a warmup ban.
type WarmupAppealParams struct {
	Message string `json:"message"`
}

// EmailUpdateParams are the parameters for updating a mailbox. Unset (nil)
// fields are left unchanged.
type EmailUpdateParams struct {
	Name            *string   `json:"name,omitempty"`
	SignaturePlain  *string   `json:"signature_plain,omitempty"`
	SignatureHTML   *string   `json:"signature_html,omitempty"`
	TrackingDomain  *string   `json:"tracking_domain,omitempty"`
	CampaignLimit   *int      `json:"campaign_limit,omitempty"`
	Tags            *[]string `json:"tags,omitempty"`
	WarmupBase      *int      `json:"warmup_base,omitempty"`
	WarmupMax       *int      `json:"warmup_max,omitempty"`
	WarmupIncrease  *int      `json:"warmup_increase,omitempty"`
	WarmupReplyRate *int      `json:"warmup_reply_rate,omitempty"`
}

// EmailListParams filters and paginates a list of connected mailboxes.
type EmailListParams struct {
	ListOptions
	Search   string
	Provider string
	Status   string
	Tag      string
}

func (p *EmailListParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	if p.Search != "" {
		q.Set("search", p.Search)
	}
	if p.Provider != "" {
		q.Set("provider", p.Provider)
	}
	if p.Status != "" {
		q.Set("status", p.Status)
	}
	if p.Tag != "" {
		q.Set("tag", p.Tag)
	}
	return q
}

// List returns a page of connected mailboxes.
func (s *EmailService) List(ctx context.Context, params *EmailListParams) (*Page[Email], error) {
	return listJSON[Email](ctx, s.client, "emails", params.values())
}

// Get retrieves a single mailbox by ID.
func (s *EmailService) Get(ctx context.Context, id string) (*Email, *Response, error) {
	email := new(Email)
	resp, err := s.client.get(ctx, "emails/"+url.PathEscape(id), email)
	if err != nil {
		return nil, resp, err
	}
	return email, resp, nil
}

// Update modifies a mailbox's settings.
func (s *EmailService) Update(ctx context.Context, id string, params *EmailUpdateParams) (*Email, *Response, error) {
	email := new(Email)
	resp, err := s.client.patch(ctx, "emails/"+url.PathEscape(id), params, email)
	if err != nil {
		return nil, resp, err
	}
	return email, resp, nil
}

// Delete disconnects and removes a mailbox.
func (s *EmailService) Delete(ctx context.Context, id string) (*Response, error) {
	return s.client.delete(ctx, "emails/"+url.PathEscape(id))
}

// Send sends a one-off message from the given mailbox.
func (s *EmailService) Send(ctx context.Context, id string, params *SendEmailParams) (*SendResult, *Response, error) {
	result := new(SendResult)
	resp, err := s.client.post(ctx, "emails/"+url.PathEscape(id)+"/send", params, result)
	if err != nil {
		return nil, resp, err
	}
	return result, resp, nil
}

// StartWarmup enables and starts warmup for a mailbox.
func (s *EmailService) StartWarmup(ctx context.Context, id string) (*Email, *Response, error) {
	email := new(Email)
	resp, err := s.client.post(ctx, "emails/"+url.PathEscape(id)+"/warmup/start", nil, email)
	if err != nil {
		return nil, resp, err
	}
	return email, resp, nil
}

// PauseWarmup pauses warmup for a mailbox without disabling it.
func (s *EmailService) PauseWarmup(ctx context.Context, id string) (*Email, *Response, error) {
	email := new(Email)
	resp, err := s.client.post(ctx, "emails/"+url.PathEscape(id)+"/warmup/pause", nil, email)
	if err != nil {
		return nil, resp, err
	}
	return email, resp, nil
}

// ResumeWarmup resumes a previously paused warmup for a mailbox.
func (s *EmailService) ResumeWarmup(ctx context.Context, id string) (*Email, *Response, error) {
	email := new(Email)
	resp, err := s.client.post(ctx, "emails/"+url.PathEscape(id)+"/warmup/resume", nil, email)
	if err != nil {
		return nil, resp, err
	}
	return email, resp, nil
}

// StopWarmup stops and disables warmup for a mailbox.
func (s *EmailService) StopWarmup(ctx context.Context, id string) (*Email, *Response, error) {
	email := new(Email)
	resp, err := s.client.post(ctx, "emails/"+url.PathEscape(id)+"/warmup/stop", nil, email)
	if err != nil {
		return nil, resp, err
	}
	return email, resp, nil
}

// WarmupBanStatus reports whether a mailbox has been banned from the warmup pool.
func (s *EmailService) WarmupBanStatus(ctx context.Context, id string) (*WarmupBanStatus, *Response, error) {
	status := new(WarmupBanStatus)
	resp, err := s.client.get(ctx, "emails/"+url.PathEscape(id)+"/warmup/ban-status", status)
	if err != nil {
		return nil, resp, err
	}
	return status, resp, nil
}

// AppealWarmupBan submits an appeal against a mailbox's warmup ban.
func (s *EmailService) AppealWarmupBan(ctx context.Context, id string, params *WarmupAppealParams) (*Response, error) {
	return s.client.post(ctx, "emails/"+url.PathEscape(id)+"/warmup/appeal", params, nil)
}
