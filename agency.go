package warmbly

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// AgencyService manages agencies: a team that runs many client workspaces
// from one place, with shared staff, a shared monthly send pool and AI
// balance, one bill, a white-label brand and agency API keys.
//
// Agency endpoints are not tied to one workspace, so they check agency staff
// rather than a workspace permission. Two credentials reach them:
//
//   - A signed-in session (a token from [AuthService.Login]). Every method
//     accepts one; the service then checks the caller's agency role (owner,
//     admin or member) per call.
//   - An agency API key (prefixed "wmagency_", created with
//     [AgencyService.CreateAPIKey] and passed to [WithAPIKey]). A key reaches
//     only its own agency, and only the methods whose documentation names a
//     PermAgency* scope the key holds. Methods documented as session only
//     refuse a key with [ErrCodeForbidden] (code "agency_key_not_allowed").
//
// A workspace API key or an OAuth access token is refused on every agency
// endpoint. Methods documented as needing a recent sign-in also want a
// recently confirmed session (see [AuthService.Reauth]) and otherwise fail
// with [ErrCodeReauthRequired].
//
// To act inside one client workspace with an agency key (any non-agency
// endpoint, such as campaigns or contacts), name the workspace with
// [WithAgencyOrganization] on each request.
type AgencyService service

// The agency API key scopes are [PermAgencyRead], [PermAgencyManageClients],
// [PermAgencyReadUsage], [PermAgencyTransferAI] and [PermAgencyMigrate]
// (together [PermAgencyAll]). Only an agency API key holds them; OR them into
// [AgencyAPIKeyCreateParams.Permissions].

// AgencyOrganizationHeader is the header an agency API key names the client
// workspace it acts on with. [WithAgencyOrganization] sets it.
const AgencyOrganizationHeader = "Warmbly-Organization"

// WithAgencyOrganization names the client workspace an agency API key acts
// on, for any workspace endpoint outside [AgencyService]. The workspace must be
// a current client of the key's agency and the key's creator a member of it;
// without the header the server answers 400 with code
// "agency_key_organization_required".
func WithAgencyOrganization(organizationID string) RequestOption {
	return WithRequestHeader(AgencyOrganizationHeader, organizationID)
}

// AgencyRole is a staff member's role in an agency. Owners and admins manage
// clients, staff, billing, the brand and API keys; members can read and work
// in client workspaces. Staff hold the workspace role "Admin" (owner, admin)
// or "Manager" (member) in every client workspace.
type AgencyRole string

const (
	// AgencyRoleOwner created the agency. There is exactly one, and the role
	// cannot be given or taken through the API.
	AgencyRoleOwner AgencyRole = "owner"
	// AgencyRoleAdmin manages the agency alongside the owner.
	AgencyRoleAdmin AgencyRole = "admin"
	// AgencyRoleMember is regular staff.
	AgencyRoleMember AgencyRole = "member"
)

// Billing periods an agency subscription can take.
const (
	// AgencyIntervalMonth bills monthly. The default.
	AgencyIntervalMonth = "month"
	// AgencyIntervalYear bills yearly. The send pool still renews monthly.
	AgencyIntervalYear = "year"
)

// Agency is one agency as the API returns it.
type Agency struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	OwnerUserID string `json:"owner_user_id"`
	// PlanID is the subscribed agency plan; nil before checkout.
	PlanID *string `json:"plan_id,omitempty"`
	// Status is the subscription lifecycle state: "active", "trialing",
	// "past_due", "canceled", "unpaid", "incomplete", "incomplete_expired" or
	// "paused".
	Status             string     `json:"status"`
	CurrentPeriodStart *time.Time `json:"current_period_start,omitempty"`
	CurrentPeriodEnd   *time.Time `json:"current_period_end,omitempty"`
	CancelAtPeriodEnd  bool       `json:"cancel_at_period_end"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`

	// Plan is the subscribed plan, when there is one.
	Plan *AgencyPlan `json:"plan,omitempty"`
	// Role is the caller's role in the agency.
	Role        AgencyRole `json:"role,omitempty"`
	ClientCount int        `json:"client_count"`
	// HasBilling reports a payment-provider customer, so the billing portal
	// ([AgencyService.BillingPortal]) has something to open.
	HasBilling bool `json:"has_billing"`
	// Granted is set while an operator-granted plan is in force in place of
	// a paid subscription.
	Granted *AgencyGrant `json:"granted,omitempty"`
	// BillingInterval is [AgencyIntervalMonth] or [AgencyIntervalYear] while
	// a subscription is in force; empty otherwise.
	BillingInterval string `json:"billing_interval,omitempty"`
}

// AgencyGrant is a plan an operator gave the agency without a subscription.
type AgencyGrant struct {
	PlanID string `json:"plan_id"`
	// Until is when the grant lapses; nil never lapses.
	Until *time.Time `json:"until,omitempty"`
}

// AgencyPlan is a plan sold to agencies. It is a [Plan] whose
// MonthlySendPool and AgencyAIAllowanceUSD are set.
type AgencyPlan = Plan

// AgencyList is the agencies the caller belongs to.
type AgencyList struct {
	Data []Agency `json:"data"`
	// BillingEnabled is false on a deployment without a billing provider,
	// where checkout and the billing methods are unavailable.
	BillingEnabled bool `json:"billing_enabled"`
}

// AgencyPlanList is the agency plan catalog.
type AgencyPlanList struct {
	Data           []AgencyPlan `json:"data"`
	BillingEnabled bool         `json:"billing_enabled"`
}

// AgencyDetail is one agency and whether billing is available.
type AgencyDetail struct {
	Agency         *Agency `json:"agency"`
	BillingEnabled bool    `json:"billing_enabled"`
}

// AgencyCreateParams creates an agency.
type AgencyCreateParams struct {
	// Name is required.
	Name string `json:"name"`
}

// AgencyUpdateParams renames an agency.
type AgencyUpdateParams struct {
	// Name is required.
	Name string `json:"name"`
}

// AgencyMember is one staff member of an agency.
type AgencyMember struct {
	AgencyID  string     `json:"agency_id"`
	UserID    string     `json:"user_id"`
	Role      AgencyRole `json:"role"`
	CreatedAt time.Time  `json:"created_at"`
	Email     string     `json:"email"`
	FirstName string     `json:"first_name"`
	LastName  string     `json:"last_name"`
}

// AgencyInvitation is staff invited by an address no account uses yet. It
// turns into membership when the account is created.
type AgencyInvitation struct {
	ID        string     `json:"id"`
	AgencyID  string     `json:"agency_id"`
	Email     string     `json:"email"`
	Role      AgencyRole `json:"role"`
	InvitedBy *string    `json:"invited_by,omitempty"`
	ExpiresAt time.Time  `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// AgencyStaff is an agency's staff and its open invitations.
type AgencyStaff struct {
	Data        []AgencyMember     `json:"data"`
	Invitations []AgencyInvitation `json:"invitations"`
}

// AgencyMemberAddParams adds staff by email.
type AgencyMemberAddParams struct {
	Email string `json:"email"`
	// Role is [AgencyRoleAdmin] or [AgencyRoleMember]. Only the owner can
	// add an admin.
	Role AgencyRole `json:"role"`
}

// AgencyMemberAddResult is the outcome of adding staff: exactly one of
// Member (the address has an account, which joined at once) and Invitation
// (it has none yet) is set.
type AgencyMemberAddResult struct {
	Member     *AgencyMember     `json:"member"`
	Invitation *AgencyInvitation `json:"invitation"`
}

// AgencyClient is one client workspace of an agency.
type AgencyClient struct {
	OrganizationID string `json:"organization_id"`
	AgencyID       string `json:"agency_id"`
	// MonthlySendLimit caps the client's share of the send pool per period;
	// nil is uncapped.
	MonthlySendLimit *int `json:"monthly_send_limit,omitempty"`
	// MonthlyAILimitUSD caps the client's spend from the agency AI balance
	// per period; nil is uncapped.
	MonthlyAILimitUSD *float64  `json:"monthly_ai_limit_usd,omitempty"`
	AddedAt           time.Time `json:"added_at"`
	AddedBy           *string   `json:"added_by,omitempty"`
	Name              string    `json:"name"`
	Slug              *string   `json:"slug,omitempty"`
	AvatarURL         *string   `json:"avatar_url,omitempty"`
	OwnerUserID       string    `json:"owner_user_id"`
}

// AgencyClientCreateParams creates a new client workspace, owned by the
// agency's owner.
type AgencyClientCreateParams struct {
	Name string `json:"name"`
	// Timezone is an IANA name; empty takes the server default.
	Timezone string `json:"timezone,omitempty"`
}

// AgencyClientUpdateParams sets a client's caps. At least one of the two caps
// must be set or cleared; a field left nil and not cleared is unchanged.
type AgencyClientUpdateParams struct {
	// MonthlySendLimit caps the client's sends per period.
	MonthlySendLimit *int `json:"-"`
	// ClearMonthlySendLimit removes the send cap.
	ClearMonthlySendLimit bool `json:"-"`
	// MonthlyAILimitUSD caps the client's AI spend from the agency balance
	// per period, in dollars (rounded to cents).
	MonthlyAILimitUSD *float64 `json:"-"`
	// ClearMonthlyAILimit removes the AI cap.
	ClearMonthlyAILimit bool `json:"-"`
}

// MarshalJSON sends a cleared cap as an explicit null, which the server reads
// as "remove the cap", and leaves an untouched cap out.
func (p AgencyClientUpdateParams) MarshalJSON() ([]byte, error) {
	out := make(map[string]any, 2)
	switch {
	case p.ClearMonthlySendLimit:
		out["monthly_send_limit"] = nil
	case p.MonthlySendLimit != nil:
		out["monthly_send_limit"] = *p.MonthlySendLimit
	}
	switch {
	case p.ClearMonthlyAILimit:
		out["monthly_ai_limit_usd"] = nil
	case p.MonthlyAILimitUSD != nil:
		out["monthly_ai_limit_usd"] = *p.MonthlyAILimitUSD
	}
	return json.Marshal(out)
}

// AgencyUsage is the agency's shared send pool in the current period.
type AgencyUsage struct {
	// Unlimited is a plan with no pool cap; the totals then only count.
	Unlimited   bool      `json:"unlimited"`
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
	DaysLeft    int       `json:"days_left"`
	PoolSize    int64     `json:"pool_size"`
	// Topups is the sends bought on top of the pool this period.
	Topups    int64 `json:"topups"`
	Total     int64 `json:"total"`
	Used      int64 `json:"used"`
	Remaining int64 `json:"remaining"`
	// Projected extrapolates this period's use to its end at the pace so far.
	Projected int64               `json:"projected"`
	Clients   []AgencyClientUsage `json:"clients"`
	// TopupSends and TopupPriceCents are one top-up pack.
	TopupSends      int   `json:"topup_sends"`
	TopupPriceCents int64 `json:"topup_price_cents"`
}

// AgencyClientUsage is one client's use of the send pool this period.
type AgencyClientUsage struct {
	OrganizationID   string `json:"organization_id"`
	Name             string `json:"name"`
	Used             int64  `json:"used"`
	MonthlySendLimit *int   `json:"monthly_send_limit,omitempty"`
}

// AgencyAIBalance is the agency's shared AI balance: the plan's allowance for
// the period plus purchased money, which never expires.
type AgencyAIBalance struct {
	// Unmetered is a self-hosted deployment, where AI is not charged and the
	// balance does not apply.
	Unmetered        bool      `json:"unmetered"`
	PeriodStart      time.Time `json:"period_start"`
	PeriodEnd        time.Time `json:"period_end"`
	AllowanceUSD     float64   `json:"allowance_usd"`
	AllowanceLeftUSD float64   `json:"allowance_left_usd"`
	PurchasedUSD     float64   `json:"purchased_usd"`
	// BoughtThisPeriodUSD is what was purchased in the current period.
	BoughtThisPeriodUSD float64 `json:"bought_this_period_usd"`
	SpentUSD            float64 `json:"spent_usd"`
	// MovedUSD is what was transferred into client workspaces this period.
	MovedUSD   float64             `json:"moved_usd"`
	BalanceUSD float64             `json:"balance_usd"`
	Clients    []AgencyAIClient    `json:"clients"`
	TopUp      AgencyAITopUpLimits `json:"top_up"`
}

// AgencyAIClient is one client's share of the agency AI balance this period.
type AgencyAIClient struct {
	OrganizationID    string   `json:"organization_id"`
	Name              string   `json:"name"`
	SpentUSD          float64  `json:"spent_usd"`
	MovedUSD          float64  `json:"moved_usd"`
	MonthlyAILimitUSD *float64 `json:"monthly_ai_limit_usd,omitempty"`
}

// AgencyAITopUpLimits is the range a purchase of agency AI balance may take.
type AgencyAITopUpLimits struct {
	MinUSD     float64 `json:"min_usd"`
	MaxUSD     float64 `json:"max_usd"`
	PresetsUSD []int   `json:"presets_usd"`
	DefaultUSD int     `json:"default_usd"`
	// FeeBps is the card processing fee charged on top of the amount, in
	// basis points (300 is 3%).
	FeeBps int `json:"fee_bps"`
}

// AgencyAITransferParams moves agency AI balance into a client workspace.
type AgencyAITransferParams struct {
	// AmountUSD is the amount in dollars, rounded to cents.
	AmountUSD float64 `json:"amount_usd"`
}

// AgencyAITransfer is the outcome of a transfer, with both balances after it.
type AgencyAITransfer struct {
	AmountUSD           float64 `json:"amount_usd"`
	AgencyBalanceUSD    float64 `json:"agency_balance_usd"`
	WorkspaceBalanceUSD float64 `json:"workspace_balance_usd"`
}

// agencyPath builds "agency/<id>/<parts...>" with every segment escaped.
func agencyPath(agencyID string, parts ...string) string {
	var b strings.Builder
	b.WriteString("agency/")
	b.WriteString(url.PathEscape(agencyID))
	for _, p := range parts {
		b.WriteByte('/')
		b.WriteString(p)
	}
	return b.String()
}

// --- agencies ---

// List returns the agencies the caller belongs to, each with the caller's
// role. An agency API key sees only its own agency.
//
// Scope: [PermAgencyRead].
func (s *AgencyService) List(ctx context.Context, opts ...RequestOption) (*AgencyList, *Response, error) {
	out, resp, err := fetch[AgencyList](ctx, s.client, "agency", opts)
	if err == nil && out.Data == nil {
		out.Data = []Agency{}
	}
	return out, resp, err
}

// Plans returns the agency plan catalog.
//
// Scope: [PermAgencyRead].
func (s *AgencyService) Plans(ctx context.Context, opts ...RequestOption) (*AgencyPlanList, *Response, error) {
	out, resp, err := fetch[AgencyPlanList](ctx, s.client, "agency/plans", opts)
	if err == nil && out.Data == nil {
		out.Data = []AgencyPlan{}
	}
	return out, resp, err
}

// Create creates an agency owned by the caller. A person owns a limited
// number of agencies ("agency_limit_reached"). Pass [WithIdempotencyKey] to
// make a retry safe.
//
// Session only.
func (s *AgencyService) Create(ctx context.Context, params *AgencyCreateParams, opts ...RequestOption) (*Agency, *Response, error) {
	return send[Agency](ctx, s.client.post, "agency", params, opts)
}

// Get retrieves one agency the caller is staff of.
//
// Scope: [PermAgencyRead].
func (s *AgencyService) Get(ctx context.Context, agencyID string, opts ...RequestOption) (*AgencyDetail, *Response, error) {
	return fetch[AgencyDetail](ctx, s.client, agencyPath(agencyID), opts)
}

// Update renames an agency. Owner or admin.
//
// Session only.
func (s *AgencyService) Update(ctx context.Context, agencyID string, params *AgencyUpdateParams, opts ...RequestOption) (*Agency, *Response, error) {
	return send[Agency](ctx, s.client.patch, agencyPath(agencyID), params, opts)
}

// --- staff ---

// Members returns the agency's staff and its open invitations.
//
// Scope: [PermAgencyRead].
func (s *AgencyService) Members(ctx context.Context, agencyID string, opts ...RequestOption) (*AgencyStaff, *Response, error) {
	out, resp, err := fetch[AgencyStaff](ctx, s.client, agencyPath(agencyID, "members"), opts)
	if err == nil {
		if out.Data == nil {
			out.Data = []AgencyMember{}
		}
		if out.Invitations == nil {
			out.Invitations = []AgencyInvitation{}
		}
	}
	return out, resp, err
}

// AddMember adds staff by email: someone with an account joins at once,
// anyone else is invited. Owner or admin; only the owner adds an admin.
// Pass [WithIdempotencyKey] to make a retry safe.
//
// Session only; needs a recent sign-in.
func (s *AgencyService) AddMember(ctx context.Context, agencyID string, params *AgencyMemberAddParams, opts ...RequestOption) (*AgencyMemberAddResult, *Response, error) {
	return send[AgencyMemberAddResult](ctx, s.client.post, agencyPath(agencyID, "members"), params, opts)
}

// UpdateMember changes a staff member's role to [AgencyRoleAdmin] or
// [AgencyRoleMember]. Only the owner makes someone an admin.
//
// Session only; needs a recent sign-in.
func (s *AgencyService) UpdateMember(ctx context.Context, agencyID, userID string, role AgencyRole, opts ...RequestOption) (*AgencyMember, *Response, error) {
	body := map[string]AgencyRole{"role": role}
	return send[AgencyMember](ctx, s.client.patch, agencyPath(agencyID, "members", url.PathEscape(userID)), body, opts)
}

// RemoveMember removes a staff member from the agency.
//
// Session only; needs a recent sign-in.
func (s *AgencyService) RemoveMember(ctx context.Context, agencyID, userID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, agencyPath(agencyID, "members", url.PathEscape(userID)), opts...)
}

// CancelInvitation withdraws an open staff invitation.
//
// Session only.
func (s *AgencyService) CancelInvitation(ctx context.Context, agencyID, invitationID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, agencyPath(agencyID, "invitations", url.PathEscape(invitationID)), opts...)
}

// --- clients ---

// Clients returns the agency's client workspaces.
//
// Scope: [PermAgencyRead].
func (s *AgencyService) Clients(ctx context.Context, agencyID string, opts ...RequestOption) ([]AgencyClient, *Response, error) {
	return fetchData[AgencyClient](ctx, s.client, agencyPath(agencyID, "clients"), opts)
}

// CreateClient creates a new client workspace under the agency. Pass
// [WithIdempotencyKey] to make a retry safe.
//
// Scope: [PermAgencyManageClients].
func (s *AgencyService) CreateClient(ctx context.Context, agencyID string, params *AgencyClientCreateParams, opts ...RequestOption) (*AgencyClient, *Response, error) {
	return send[AgencyClient](ctx, s.client.post, agencyPath(agencyID, "clients"), params, opts)
}

// AttachClient brings an existing workspace the caller owns into the agency.
// A workspace with its own paid subscription, or one already in an agency,
// is refused. Pass [WithIdempotencyKey] to make a retry safe.
//
// Scope: [PermAgencyManageClients]; a session needs a recent sign-in.
func (s *AgencyService) AttachClient(ctx context.Context, agencyID, organizationID string, opts ...RequestOption) (*AgencyClient, *Response, error) {
	body := map[string]string{"organization_id": organizationID}
	return send[AgencyClient](ctx, s.client.post, agencyPath(agencyID, "clients", "attach"), body, opts)
}

// UpdateClient sets or clears a client's monthly send cap and AI cap.
//
// Scope: [PermAgencyManageClients].
func (s *AgencyService) UpdateClient(ctx context.Context, agencyID, organizationID string, params *AgencyClientUpdateParams, opts ...RequestOption) (*AgencyClient, *Response, error) {
	return send[AgencyClient](ctx, s.client.patch, agencyPath(agencyID, "clients", url.PathEscape(organizationID)), params, opts)
}

// DetachClient takes a client workspace out of the agency. The workspace
// keeps existing on its own.
//
// Scope: [PermAgencyManageClients]; a session needs a recent sign-in.
func (s *AgencyService) DetachClient(ctx context.Context, agencyID, organizationID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, agencyPath(agencyID, "clients", url.PathEscape(organizationID)), opts...)
}

// --- usage ---

// Usage returns the shared send pool for the current period, per client.
//
// Scope: [PermAgencyReadUsage].
func (s *AgencyService) Usage(ctx context.Context, agencyID string, opts ...RequestOption) (*AgencyUsage, *Response, error) {
	return fetch[AgencyUsage](ctx, s.client, agencyPath(agencyID, "usage"), opts)
}

// AIBalance returns the shared AI balance for the current period, per client.
//
// Scope: [PermAgencyReadUsage].
func (s *AgencyService) AIBalance(ctx context.Context, agencyID string, opts ...RequestOption) (*AgencyAIBalance, *Response, error) {
	return fetch[AgencyAIBalance](ctx, s.client, agencyPath(agencyID, "ai-balance"), opts)
}

// TransferAIBalance moves money from the agency AI balance into a client
// workspace's own balance. Pass [WithIdempotencyKey]: the server keys the
// transfer on it, so a retry with the same key moves the money once.
//
// Scope: [PermAgencyTransferAI]; a session needs a recent sign-in.
func (s *AgencyService) TransferAIBalance(ctx context.Context, agencyID, organizationID string, params *AgencyAITransferParams, opts ...RequestOption) (*AgencyAITransfer, *Response, error) {
	return send[AgencyAITransfer](ctx, s.client.post, agencyPath(agencyID, "clients", url.PathEscape(organizationID), "ai-balance"), params, opts)
}

// deleteJSON issues a DELETE whose response carries a body to decode.
func (s *AgencyService) deleteJSON(ctx context.Context, path string, out any, opts []RequestOption) (*Response, error) {
	return s.client.call(ctx, http.MethodDelete, path, nil, out, opts)
}
