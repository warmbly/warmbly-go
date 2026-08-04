package warmbly

import (
	"context"
	"net/url"
	"time"
)

// OrganizationService manages the organization (workspace): its settings and
// AI voice profile, its members, custom roles and invitations, plan limits, and
// the danger zone.
//
// These routes are session-only. They accept a JWT obtained from
// [AuthService.Login] (pass it with [WithAccessToken] or [WithTokenSource]) and
// reject a long-lived API key, because organization governance should never sit
// behind a static credential.
type OrganizationService service

// Organization permission bits. A member's effective grant is the bitwise OR
// across their assigned roles, which is what travels in
// [Member.Permissions] and [Role.Permissions].
//
// These gate a human session and are distinct from the API key scopes (the
// Perm* constants in apikeys.go).
const (
	// OrgPermManageTeam allows inviting and removing members.
	OrgPermManageTeam uint16 = 1 << iota
	// OrgPermManageBilling allows viewing invoices and changing plans.
	OrgPermManageBilling
	// OrgPermManageCampaigns allows creating and editing campaigns.
	OrgPermManageCampaigns
	// OrgPermManageContacts allows creating and editing contacts.
	OrgPermManageContacts
	// OrgPermManageEmails allows connecting and editing mailboxes.
	OrgPermManageEmails
	// OrgPermViewAnalytics allows viewing reports.
	OrgPermViewAnalytics
	// OrgPermSendCampaigns allows starting campaigns, which sends real mail.
	OrgPermSendCampaigns
	// OrgPermAccessUnibox allows using the unified inbox.
	OrgPermAccessUnibox
	// OrgPermManageSequences allows creating and editing sequence steps.
	OrgPermManageSequences
	// OrgPermManageSettings allows changing organization settings.
	OrgPermManageSettings
	// OrgPermViewCampaigns grants read-only campaign access.
	OrgPermViewCampaigns
	// OrgPermViewContacts grants read-only contact access.
	OrgPermViewContacts
	// OrgPermTransferOwnership allows handing the workspace to another member.
	OrgPermTransferOwnership
	// OrgPermManageAPIKeys allows managing API keys and OAuth applications.
	OrgPermManageAPIKeys
	// OrgPermUseIntegrations allows operating connected integrations — pushing
	// records, running automations — without granting full settings access.
	OrgPermUseIntegrations
	// OrgPermUseAI allows using the AI features, which spend the shared credit
	// balance.
	OrgPermUseAI
)

// OrgPermAll is every organization permission, which is what the owner holds.
const OrgPermAll uint16 = 0xFFFF

// Built-in role names returned in [Member.Role].
const (
	RoleOwner   = "owner"
	RoleAdmin   = "admin"
	RoleManager = "manager"
	RoleViewer  = "viewer"
)

// Group is a user-scoped label: a campaign folder, a mailbox tag or a contact
// category. See [GroupService].
type Group struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Color string `json:"color"`
	// Position is the group's index in its ordered set.
	Position  int32     `json:"position"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// User is a Warmbly user account.
type User struct {
	ID        string   `json:"id"`
	FirstName string   `json:"first_name"`
	LastName  string   `json:"last_name"`
	Email     string   `json:"email"`
	AvatarURL *string  `json:"avatar_url,omitempty"`
	Roles     []string `json:"roles,omitempty"`

	ReferralSource        *string    `json:"referral_source,omitempty"`
	OnboardingCompletedAt *time.Time `json:"onboarding_completed_at,omitempty"`

	MaxOrganizations int  `json:"max_organizations,omitempty"`
	FreeTrialUsed    bool `json:"free_trial_used,omitempty"`

	// UndoSendSeconds is how long an instant send is held before it leaves, so
	// the user can still cancel it.
	UndoSendSeconds int `json:"undo_send_seconds,omitempty"`

	// IsAdmin reports platform-admin access; AdminPermissions is the raw
	// bitmask behind it. Both are only populated on the caller's own profile.
	AdminPermissions uint64 `json:"admin_permissions,omitempty"`
	IsAdmin          bool   `json:"is_admin,omitempty"`

	// DeletionScheduledFor is set while the account is pending a hard delete.
	DeletionScheduledAt  *time.Time `json:"deletion_scheduled_at,omitempty"`
	DeletionScheduledFor *time.Time `json:"deletion_scheduled_for,omitempty"`

	// Folders, Tags and Categories are the user's label groups, returned on
	// their own profile.
	Folders    []Group `json:"folders,omitempty"`
	Tags       []Group `json:"tags,omitempty"`
	Categories []Group `json:"categories,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PendingDeletion reports whether the account is scheduled for a hard delete.
func (u *User) PendingDeletion() bool { return u.DeletionScheduledFor != nil }

// Organization is a workspace.
type Organization struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Slug        *string   `json:"slug,omitempty"`
	AvatarURL   *string   `json:"avatar_url,omitempty"`
	OwnerUserID string    `json:"owner_user_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// DeletionScheduledFor is set while the workspace is pending a hard delete.
	DeletionScheduledAt  *time.Time `json:"deletion_scheduled_at,omitempty"`
	DeletionScheduledFor *time.Time `json:"deletion_scheduled_for,omitempty"`

	// PresenceShowOnline and PresenceShowActivity control team-presence
	// privacy. With PresenceShowOnline false nobody is tracked at all; with
	// PresenceShowActivity false, presence is shown without the
	// viewing-or-editing detail.
	PresenceShowOnline   bool `json:"presence_show_online"`
	PresenceShowActivity bool `json:"presence_show_activity"`

	// ProductDescription, ICPNotes and VoiceProfile are the AI voice profile:
	// organization grounding folded into every AI writing surface.
	ProductDescription string `json:"product_description"`
	ICPNotes           string `json:"icp_notes"`
	VoiceProfile       string `json:"voice_profile"`

	// InboxAgentEnabled opts the workspace into the inbox agent, which drafts
	// a suggested reply on an inbound human reply. It never sends on its own.
	InboxAgentEnabled bool `json:"inbox_agent_enabled"`
	// AssistantSharedHistory makes AI assistant conversations visible to every
	// member with the use-AI permission rather than only their author.
	AssistantSharedHistory bool `json:"assistant_shared_history"`

	Owner *User `json:"owner,omitempty"`
}

// PendingDeletion reports whether the workspace is scheduled for a hard delete.
func (o *Organization) PendingDeletion() bool { return o.DeletionScheduledFor != nil }

// OrganizationWithLimits is an organization together with its plan limits and
// current usage.
type OrganizationWithLimits struct {
	Organization
	Limits *OrganizationLimits `json:"limits,omitempty"`
	Counts *OrganizationCounts `json:"counts,omitempty"`
}

// OrganizationLimits are the effective plan ceilings. A nil field is
// unlimited.
type OrganizationLimits struct {
	MaxCampaigns       *int `json:"max_campaigns,omitempty"`
	MaxActiveCampaigns *int `json:"max_active_campaigns,omitempty"`
	MaxTeamMembers     *int `json:"max_team_members,omitempty"`
	MaxEmailAccounts   *int `json:"max_email_accounts,omitempty"`
	MaxContacts        *int `json:"max_contacts,omitempty"`
	DailyCampaignLimit *int `json:"daily_campaign_limit,omitempty"`
}

// OrganizationCounts is current usage against [OrganizationLimits].
type OrganizationCounts struct {
	TotalCampaigns  int `json:"total_campaigns"`
	ActiveCampaigns int `json:"active_campaigns"`
	TotalContacts   int `json:"total_contacts"`
	TotalMembers    int `json:"total_members"`
	EmailAccounts   int `json:"email_accounts"`
	EmailsSentToday int `json:"emails_sent_today"`
}

// LimitsAndCounts pairs the plan ceilings with current usage.
type LimitsAndCounts struct {
	Limits *OrganizationLimits `json:"limits"`
	Counts *OrganizationCounts `json:"counts"`
}

// MemberRole is a lightweight role reference for rendering the roles a member
// holds, without the permission payload.
type MemberRole struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Member is a user's membership in an organization.
type Member struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	UserID         string `json:"user_id"`
	// Role is the built-in role name, one of the Role* constants. Owner is a
	// membership flag rather than a role row.
	Role string `json:"role"`
	// RoleID is the member's first assigned role; Roles is the full set.
	RoleID *string      `json:"role_id,omitempty"`
	Roles  []MemberRole `json:"roles,omitempty"`
	// Permissions is the effective grant across every assigned role. Test it
	// with [Member.Can].
	Permissions uint16 `json:"permissions"`

	InvitedBy  *string    `json:"invited_by,omitempty"`
	InvitedAt  time.Time  `json:"invited_at"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`

	User         *User         `json:"user,omitempty"`
	Organization *Organization `json:"organization,omitempty"`

	// Email and Name are flattened from the joined user for convenience.
	Email string `json:"email"`
	Name  string `json:"name"`
}

// Can reports whether the member holds every bit in perms. The owner always
// holds every permission.
func (m *Member) Can(perms uint16) bool {
	if m.Role == RoleOwner {
		return true
	}
	return m.Permissions&perms == perms
}

// IsOwner reports whether the member owns the organization.
func (m *Member) IsOwner() bool { return m.Role == RoleOwner }

// Role is an organization-scoped custom role: a named permission set members
// are assigned to. Editing a role writes through to every member holding it.
type Role struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	Color          string `json:"color"`
	// Permissions is the role's grant, an OR of the OrgPerm* bits.
	Permissions uint16    `json:"permissions"`
	MemberCount int       `json:"member_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// RoleCreateParams creates a custom role. The name "owner" is reserved.
type RoleCreateParams struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"`
	Permissions uint16 `json:"permissions"`
}

// RoleUpdateParams edits a custom role. Nil fields are left unchanged; edits
// propagate to every member holding the role.
type RoleUpdateParams struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Color       *string `json:"color,omitempty"`
	Permissions *uint16 `json:"permissions,omitempty"`
}

// Invitation is a pending invitation to join an organization. The invite token
// is never returned in a listing; fetch it with
// [OrganizationService.InvitationLink].
type Invitation struct {
	ID             string       `json:"id"`
	OrganizationID string       `json:"organization_id"`
	Email          string       `json:"email"`
	Role           string       `json:"role"`
	RoleID         *string      `json:"role_id,omitempty"`
	Roles          []MemberRole `json:"roles,omitempty"`
	Permissions    uint16       `json:"permissions"`
	InvitedBy      string       `json:"invited_by"`
	ExpiresAt      time.Time    `json:"expires_at"`
	CreatedAt      time.Time    `json:"created_at"`

	Organization  *Organization `json:"organization,omitempty"`
	InvitedByUser *User         `json:"invited_by_user,omitempty"`
}

// Expired reports whether the invitation has lapsed.
func (i *Invitation) Expired() bool { return timeNow().After(i.ExpiresAt) }

// OrganizationCreateParams creates a workspace.
type OrganizationCreateParams struct {
	Name string `json:"name"`
}

// OrganizationUpdateParams updates the current workspace. Nil fields are left
// unchanged; an empty string clears a text field.
type OrganizationUpdateParams struct {
	Name *string `json:"name,omitempty"`
	Slug *string `json:"slug,omitempty"`

	PresenceShowOnline   *bool `json:"presence_show_online,omitempty"`
	PresenceShowActivity *bool `json:"presence_show_activity,omitempty"`

	ProductDescription *string `json:"product_description,omitempty"`
	ICPNotes           *string `json:"icp_notes,omitempty"`
	VoiceProfile       *string `json:"voice_profile,omitempty"`

	InboxAgentEnabled      *bool `json:"inbox_agent_enabled,omitempty"`
	AssistantSharedHistory *bool `json:"assistant_shared_history,omitempty"`
}

// InviteMemberParams invites someone by email into one or more roles.
type InviteMemberParams struct {
	Email string `json:"email"`
	// RoleIDs are the roles the invitee lands in; at least one is required.
	RoleIDs []string `json:"role_ids,omitempty"`
	// RoleID is a single-role shorthand for RoleIDs.
	RoleID *string `json:"role_id,omitempty"`
}

// UpdateMemberParams replaces a member's assigned roles.
type UpdateMemberParams struct {
	RoleIDs []string `json:"role_ids,omitempty"`
	RoleID  *string  `json:"role_id,omitempty"`
}

// DangerZoneStatus describes a workspace's or account's deletion state, plus
// the confirmation phrase a client should require before scheduling one.
type DangerZoneStatus struct {
	// ResourceType is "organization" or "user".
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	ResourceName string `json:"resource_name"`
	// ConfirmationHint is what the user must type to confirm: the workspace
	// name, or their own email address.
	ConfirmationHint string `json:"confirmation_hint"`
	// GraceDays is how long the delete is delayed and remains cancelable.
	GraceDays int `json:"grace_days"`
	// PendingDeletion is set only while a delete is scheduled.
	PendingDeletion *ScheduledDeletion `json:"pending_deletion,omitempty"`
}

// Scheduled-deletion states returned in [ScheduledDeletion.Status].
const (
	DeletionStatusPending   = "pending"
	DeletionStatusExecuting = "executing"
	DeletionStatusCompleted = "completed"
	DeletionStatusCancelled = "cancelled" //nolint:misspell // wire value: the API sends "cancelled" here
	DeletionStatusFailed    = "failed"
)

// ScheduledDeletion is a pending hard delete, cancelable until ExecuteAfter.
type ScheduledDeletion struct {
	ID             string  `json:"id"`
	ResourceType   string  `json:"resource_type"`
	ResourceID     string  `json:"resource_id"`
	OrganizationID *string `json:"organization_id,omitempty"`

	RequestedByUserID string  `json:"requested_by_user_id"`
	Reason            *string `json:"reason,omitempty"`

	ScheduledAt  time.Time `json:"scheduled_at"`
	ExecuteAfter time.Time `json:"execute_after"`
	GraceDays    int       `json:"grace_days"`
	// Status is the deletion's lifecycle state: one of the DeletionStatus*
	// constants.
	Status string `json:"status"`

	CancelledAt       *time.Time `json:"cancelled_at,omitempty"`         //nolint:misspell // wire value: the API sends "cancelled" here
	CancelledByUserID *string    `json:"cancelled_by_user_id,omitempty"` //nolint:misspell // wire value: the API sends "cancelled" here
	CancelledReason   *string    `json:"cancelled_reason,omitempty"`     //nolint:misspell // wire value: the API sends "cancelled" here

	ExecutedAt     *time.Time `json:"executed_at,omitempty"`
	ExecutionError *string    `json:"execution_error,omitempty"`
	LastReminderAt *time.Time `json:"last_reminder_at,omitempty"`
}

// ScheduleDeletionParams schedules a delayed hard delete. Confirmation must
// match the [DangerZoneStatus.ConfirmationHint] exactly.
type ScheduleDeletionParams struct {
	Confirmation string `json:"confirmation"`
	Reason       string `json:"reason,omitempty"`
}

// Create provisions a new workspace and returns it.
func (s *OrganizationService) Create(ctx context.Context, params *OrganizationCreateParams, opts ...RequestOption) (*Organization, *Response, error) {
	return send[Organization](ctx, s.client.post, "organization", params, opts)
}

// List returns the memberships the caller holds across every workspace.
func (s *OrganizationService) List(ctx context.Context, opts ...RequestOption) ([]Member, *Response, error) {
	return fetchData[Member](ctx, s.client, "organization", opts)
}

// Switch points the caller's session at another workspace they belong to.
// Subsequent requests on that session act on the new workspace.
func (s *OrganizationService) Switch(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "organization/switch/"+url.PathEscape(id), nil, nil, opts...)
}

// Current retrieves the workspace the session acts on, with its limits and
// current usage.
func (s *OrganizationService) Current(ctx context.Context, opts ...RequestOption) (*OrganizationWithLimits, *Response, error) {
	return fetch[OrganizationWithLimits](ctx, s.client, "organization/current", opts)
}

// Update modifies the current workspace.
func (s *OrganizationService) Update(ctx context.Context, params *OrganizationUpdateParams, opts ...RequestOption) (*Organization, *Response, error) {
	return send[Organization](ctx, s.client.patch, "organization/current", params, opts)
}

// Limits returns the workspace's plan ceilings alongside current usage.
func (s *OrganizationService) Limits(ctx context.Context, opts ...RequestOption) (*LimitsAndCounts, *Response, error) {
	return fetch[LimitsAndCounts](ctx, s.client, "organization/current/limits", opts)
}

// UploadAvatar sets the workspace logo.
func (s *OrganizationService) UploadAvatar(ctx context.Context, file *FileUpload, opts ...RequestOption) (*Organization, *Response, error) {
	out := new(Organization)
	resp, err := s.client.postMultipart(ctx, "organization/avatar", "avatar", file, nil, out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// DeleteAvatar removes the workspace logo.
func (s *OrganizationService) DeleteAvatar(ctx context.Context, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "organization/avatar", opts...)
}

// --- members ---

// Members returns the workspace's members.
func (s *OrganizationService) Members(ctx context.Context, opts ...RequestOption) ([]Member, *Response, error) {
	return fetchData[Member](ctx, s.client, "organization/members", opts)
}

// Invite invites someone to the workspace by email and returns the pending
// invitation.
func (s *OrganizationService) Invite(ctx context.Context, params *InviteMemberParams, opts ...RequestOption) (*Invitation, *Response, error) {
	var out struct {
		Invitation *Invitation `json:"invitation"`
	}
	resp, err := s.client.post(ctx, "organization/members/invite", params, &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Invitation, resp, nil
}

// UpdateMember replaces a member's assigned roles.
func (s *OrganizationService) UpdateMember(ctx context.Context, id string, params *UpdateMemberParams, opts ...RequestOption) (*Member, *Response, error) {
	return send[Member](ctx, s.client.patch, "organization/members/"+url.PathEscape(id), params, opts)
}

// RemoveMember removes a member from the workspace.
func (s *OrganizationService) RemoveMember(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "organization/members/"+url.PathEscape(id), opts...)
}

// TransferOwnership hands the workspace to another member. Only the current
// owner may call it.
func (s *OrganizationService) TransferOwnership(ctx context.Context, newOwnerUserID string, opts ...RequestOption) (*Response, error) {
	body := struct {
		NewOwnerUserID string `json:"new_owner_user_id"`
	}{NewOwnerUserID: newOwnerUserID}
	return s.client.post(ctx, "organization/transfer-ownership", body, nil, opts...)
}

// --- roles ---

// Roles returns the workspace's custom roles.
func (s *OrganizationService) Roles(ctx context.Context, opts ...RequestOption) ([]Role, *Response, error) {
	return fetchData[Role](ctx, s.client, "organization/roles", opts)
}

// CreateRole adds a custom role.
func (s *OrganizationService) CreateRole(ctx context.Context, params *RoleCreateParams, opts ...RequestOption) (*Role, *Response, error) {
	return send[Role](ctx, s.client.post, "organization/roles", params, opts)
}

// UpdateRole edits a custom role. Changes propagate to every member holding it.
func (s *OrganizationService) UpdateRole(ctx context.Context, id string, params *RoleUpdateParams, opts ...RequestOption) (*Role, *Response, error) {
	return send[Role](ctx, s.client.patch, "organization/roles/"+url.PathEscape(id), params, opts)
}

// DeleteRole removes a custom role.
func (s *OrganizationService) DeleteRole(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "organization/roles/"+url.PathEscape(id), opts...)
}

// --- invitations ---

// Invitations returns the workspace's pending invitations.
func (s *OrganizationService) Invitations(ctx context.Context, opts ...RequestOption) ([]Invitation, *Response, error) {
	return fetchData[Invitation](ctx, s.client, "organization/invitations", opts)
}

// CancelInvitation withdraws a pending invitation.
func (s *OrganizationService) CancelInvitation(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "organization/invitations/"+url.PathEscape(id), opts...)
}

// InvitationLink returns the invite token for a pending invitation, so it can
// be shared out of band. Treat it as a credential: anyone holding it can join.
func (s *OrganizationService) InvitationLink(ctx context.Context, id string, opts ...RequestOption) (string, *Response, error) {
	var out struct {
		Token string `json:"token"`
	}
	resp, err := s.client.get(ctx, "organization/invitations/"+url.PathEscape(id)+"/link", &out, opts...)
	if err != nil {
		return "", resp, err
	}
	return out.Token, resp, nil
}

// MyInvitations returns the invitations awaiting the caller across every
// workspace.
func (s *OrganizationService) MyInvitations(ctx context.Context, opts ...RequestOption) ([]Invitation, *Response, error) {
	return fetchData[Invitation](ctx, s.client, "invitations", opts)
}

// PreviewInvitation resolves an invite token to the workspace it points at,
// before the caller commits to joining. It needs no credentials: the token is
// the capability.
func (s *OrganizationService) PreviewInvitation(ctx context.Context, token string, opts ...RequestOption) (*Invitation, *Response, error) {
	q := url.Values{"token": {token}}
	return fetch[Invitation](ctx, s.client, withQuery("invitations/lookup", q), opts)
}

// AcceptInvitation joins the workspace an invite token points at.
func (s *OrganizationService) AcceptInvitation(ctx context.Context, token string, opts ...RequestOption) (*Member, *Response, error) {
	body := struct {
		Token string `json:"token"`
	}{Token: token}
	var out struct {
		Member *Member `json:"member"`
	}
	resp, err := s.client.post(ctx, "invitations/accept", body, &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Member, resp, nil
}

// --- plan limit increases ---

// Limit-request states returned in [LimitRequest.Status].
const (
	LimitRequestPending   = "pending"
	LimitRequestApproved  = "approved"
	LimitRequestRejected  = "rejected"
	LimitRequestCancelled = "cancelled" //nolint:misspell // wire value: the API sends "cancelled" here
)

// LimitRequest is an ask for a higher plan ceiling, pending review.
type LimitRequest struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	// Field names the ceiling being raised, matching a key of
	// [OrganizationLimits] such as "max_email_accounts".
	Field string `json:"field"`
	// CurrentEffective is what the limit was when the request was filed, so a
	// reviewer sees what the asker was looking at.
	CurrentEffective int    `json:"current_effective"`
	Requested        int    `json:"requested"`
	Reason           string `json:"reason"`
	// Status is one of the LimitRequest* constants.
	Status      string    `json:"status"`
	SubmittedBy string    `json:"submitted_by"`
	SubmittedAt time.Time `json:"submitted_at"`

	ReviewedBy  *string    `json:"reviewed_by,omitempty"`
	ReviewedAt  *time.Time `json:"reviewed_at,omitempty"`
	ReviewNotes string     `json:"review_notes,omitempty"`
}

// LimitRequestParams asks for a higher ceiling. Requested must exceed the
// current effective limit, and Field must name a real one.
type LimitRequestParams struct {
	Field     string `json:"field"`
	Requested int    `json:"requested"`
	Reason    string `json:"reason"`
}

// LimitRequests returns the workspace's limit-increase requests.
func (s *OrganizationService) LimitRequests(ctx context.Context, orgID string, opts ...RequestOption) ([]LimitRequest, *Response, error) {
	return fetchData[LimitRequest](ctx, s.client, "organization/"+url.PathEscape(orgID)+"/limit-requests", opts)
}

// RequestLimitIncrease files a request to raise one plan ceiling.
func (s *OrganizationService) RequestLimitIncrease(ctx context.Context, orgID string, params *LimitRequestParams, opts ...RequestOption) (*LimitRequest, *Response, error) {
	return send[LimitRequest](ctx, s.client.post, "organization/"+url.PathEscape(orgID)+"/limit-requests", params, opts)
}

// CancelLimitRequest withdraws a pending request. Only its submitter may.
func (s *OrganizationService) CancelLimitRequest(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "limit-requests/"+url.PathEscape(id), opts...)
}

// --- danger zone ---

// DangerZone returns the workspace's deletion state and the confirmation
// phrase required to schedule one.
func (s *OrganizationService) DangerZone(ctx context.Context, opts ...RequestOption) (*DangerZoneStatus, *Response, error) {
	return fetch[DangerZoneStatus](ctx, s.client, "organization/current/danger-zone", opts)
}

// ScheduleDeletion schedules the workspace for a delayed hard delete. It is
// owner-only, and Confirmation must match
// [DangerZoneStatus.ConfirmationHint] — the workspace name.
func (s *OrganizationService) ScheduleDeletion(ctx context.Context, params *ScheduleDeletionParams, opts ...RequestOption) (*ScheduledDeletion, *Response, error) {
	return send[ScheduledDeletion](ctx, s.client.post, "organization/current/danger-zone/delete", params, opts)
}

// CancelDeletion cancels a pending workspace deletion.
func (s *OrganizationService) CancelDeletion(ctx context.Context, reason string, opts ...RequestOption) (*Response, error) {
	body := struct {
		Reason string `json:"reason,omitempty"`
	}{Reason: reason}
	return s.client.deleteBody(ctx, "organization/current/danger-zone/delete", body, opts...)
}
