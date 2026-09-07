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

// Values for [AuditLog.Action]. The trail is append-only and the vocabulary
// grows, so treat an action you do not recognize as informational rather than
// failing on it.
const (
	AuditActionCreate    = "create"
	AuditActionUpdate    = "update"
	AuditActionDelete    = "delete"
	AuditActionDuplicate = "duplicate"
	AuditActionSend      = "send"

	// Lifecycle actions on a campaign or a mailbox's warmup.
	AuditActionStart  = "start"
	AuditActionStop   = "stop"
	AuditActionPause  = "pause"
	AuditActionResume = "resume"

	// Credentials and connections.
	AuditActionRevoke     = "revoke"
	AuditActionRotate     = "rotate"
	AuditActionRotateKeys = "rotate_keys"
	AuditActionConnect    = "connect"
	AuditActionDisconnect = "disconnect"
	AuditActionTest       = "test"

	// Membership and ownership.
	AuditActionInvite   = "invite"
	AuditActionRemove   = "remove"
	AuditActionAssign   = "assign"
	AuditActionTransfer = "transfer"

	// Bulk movement of data in and out of the workspace.
	AuditActionExport = "export"
	AuditActionImport = "import"

	// AuditActionAPICall records a call made with an API key, when the key is
	// configured to log its requests.
	AuditActionAPICall = "api_call"
	// AuditActionApply records an advisor recommendation being applied.
	AuditActionApply = "apply"
)

// Values for [AuditLog.EntityType]. As with actions, the vocabulary grows;
// branch on the ones you care about and pass the rest through.
//
// The platform's own entities (workers, releases, instance settings) are
// audited too, but on the operator's trail rather than any organization's, so
// they never appear here.
const (
	AuditEntityCampaign     = "campaign"
	AuditEntityContact      = "contact"
	AuditEntityEmailAccount = "email_account"
	AuditEntityAPIKey       = "api_key"
	AuditEntityWebhook      = "webhook"
	AuditEntityTemplate     = "template"
	// AuditEntitySequence is a step within a campaign's sequence. The wire
	// value is "step".
	AuditEntitySequence     = "step"
	AuditEntityOrganization = "organization"
	AuditEntityUser         = "user"

	// Audiences and the do-not-contact list.
	AuditEntitySegment     = "segment"
	AuditEntityForm        = "form"
	AuditEntitySuppression = "suppression"

	// Labels.
	AuditEntityFolder   = "folder"
	AuditEntityTag      = "tag"
	AuditEntityCategory = "category"

	// Team and access.
	AuditEntityOrganizationMember = "organization_member"
	AuditEntityInvitation         = "invitation"
	AuditEntityRole               = "role"
	AuditEntityTeam               = "team"

	// CRM.
	AuditEntityCRMPipeline = "crm_pipeline"
	AuditEntityCRMStage    = "crm_stage"
	AuditEntityCRMDeal     = "crm_deal"
	AuditEntityCRMTask     = "crm_task"
	AuditEntityCRMNote     = "crm_note"

	// Connections and flows.
	AuditEntityIntegration    = "integration"
	AuditEntityAutomation     = "automation"
	AuditEntityLeadSyncSource = "lead_sync_source"
	AuditEntityMeeting        = "meeting"
	AuditEntityUnibox         = "unibox"

	// Warmup and sending posture.
	AuditEntityWarmupRoutingRule = "warmup_routing_rule"
	// AuditEntityOrgRisk is a change in the workspace's sending posture.
	AuditEntityOrgRisk = "org_risk"

	// AI.
	AuditEntityAISession      = "ai_session"
	AuditEntityAISkill        = "ai_skill"
	AuditEntityMCPServer      = "mcp_server"
	AuditEntityAdvisorFinding = "advisor_finding"

	// Billing.
	AuditEntitySubscription   = "subscription"
	AuditEntityReferral       = "referral"
	AuditEntityReferralCredit = "referral_credit"
	AuditEntityCreditPurchase = "credit_purchase"
	AuditEntityCreditGrant    = "credit_grant"

	// Workspace settings and the archives used to move a workspace between
	// instances.
	AuditEntitySettings   = "settings"
	AuditEntityOrgArchive = "org_archive"

	// The self-hosted warmup pool link, from the cloud side
	// ([AuditEntityPoolLink]) and the instance side ([AuditEntityCloudLink]).
	AuditEntityPoolLink  = "pool_link"
	AuditEntityCloudLink = "cloud_link"
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
