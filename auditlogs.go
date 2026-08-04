package warmbly

import (
	"context"
	"net/url"
	"time"
)

// AuditLogService reads the organization's audit trail: who changed what, when
// and from where. The trail is org-scoped server-side, so one organization can
// never read another's.
type AuditLogService service

// Common values for [AuditLog.Action].
const (
	AuditActionCreate = "create"
	AuditActionUpdate = "update"
	AuditActionDelete = "delete"
	AuditActionSend   = "send"
	AuditActionRevoke = "revoke"
)

// Common values for [AuditLog.EntityType].
const (
	AuditEntityCampaign     = "campaign"
	AuditEntityContact      = "contact"
	AuditEntityEmailAccount = "email_account"
	AuditEntityAPIKey       = "api_key"
	AuditEntityWebhook      = "webhook"
	AuditEntityTemplate     = "template"
	AuditEntitySequence     = "sequence"
	AuditEntityOrganization = "organization"
)

// AuditActor is the member who performed an audited action.
type AuditActor struct {
	ID        string `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
}

// AuditLog is one entry in the organization's audit trail.
type AuditLog struct {
	ID    string `json:"id"`
	OrgID string `json:"org_id"`
	// UserID is the acting member's id; Actor carries their hydrated profile
	// when the record could be joined.
	UserID string      `json:"user_id"`
	Actor  *AuditActor `json:"actor,omitempty"`

	ActionDate time.Time `json:"action_date"`
	// Action is what happened, for example [AuditActionUpdate].
	Action string `json:"action"`
	// EntityType is what it happened to, for example [AuditEntityCampaign].
	EntityType string  `json:"entity_type"`
	EntityID   *string `json:"entity_id,omitempty"`

	IPAddress string `json:"ip_address"`
	UserAgent string `json:"user_agent"`
	// Changes records the fields that moved; Metadata carries extra context.
	Changes   map[string]string `json:"changes,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// AuditLogListParams filters and paginates the audit trail.
type AuditLogListParams struct {
	ListOptions
	// ActorID limits the trail to one acting member.
	ActorID string
	// EntityID limits it to one entity, and EntityType to one kind of entity.
	EntityID   string
	EntityType string
	// Action limits it to one action.
	Action string
	// Date selects a single UTC day. StartDate and EndDate override it with an
	// explicit range.
	Date      time.Time
	StartDate time.Time
	EndDate   time.Time
}

func (p *AuditLogListParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	setNonEmpty(q, "actor_id", p.ActorID)
	setNonEmpty(q, "entity_id", p.EntityID)
	setNonEmpty(q, "entity_type", p.EntityType)
	setNonEmpty(q, "action", p.Action)
	setNonEmpty(q, "date", formatDay(p.Date))
	setTime(q, "start_date", &p.StartDate)
	setTime(q, "end_date", &p.EndDate)
	return q
}

// List returns a page of audit-trail entries, newest first. The page size must
// be between 10 and 200; zero uses the server default of 50.
func (s *AuditLogService) List(ctx context.Context, params *AuditLogListParams, opts ...RequestOption) (*Page[AuditLog], error) {
	return listJSON[AuditLog](ctx, s.client, "audit-logs", params.values(), opts...)
}
