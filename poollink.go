package warmbly

import (
	"context"
	"net/url"
	"time"
)

// PoolLinkService is the hosted (cloud) side of the self-hosted warmup pool
// link: a self-hosted Warmbly instance asks to warm its mailboxes in this
// workspace's pool, a member approves it, and the workspace then lists and
// unlinks the instances it has admitted.
//
// Linking is a device-code handshake. The instance calls
// [PoolLinkService.StartCode] before it holds any credential and shows the
// returned user code to its operator; that operator signs in to Warmbly Cloud,
// reviews the code with [PoolLinkService.DescribeCode] and settles it with
// [PoolLinkService.ApproveCode] or [PoolLinkService.DenyCode]; meanwhile the
// instance loops on [PoolLinkService.Poll] (or [PoolLinkService.WaitForApproval])
// until the code is approved and its instance token is delivered, once.
//
// Only StartCode and Poll are public: they need no credential at all and share
// the per-IP sign-in rate budget, so a runaway poll loop can lock the address
// out of the browser login for the rest of the window. Every other route is
// session-only: it needs a token from [AuthService.Login], never an API key,
// because approving a code mints a credential for a third party. Listing and
// revoking instances additionally need the manage-settings organization
// permission.
//
// The instance's own machine-to-machine surface, /pool-link/instance/*
// (status and standing, warmup analytics, enrollment, SMTP association,
// participation, brokered access tokens, the warmup delivery, arrival and
// observation reports, the command queue, the placement seed panel and the
// root redirects), accepts only the instance token minted by the handshake
// and is deliberately not modeled here. It is the protocol between two
// Warmbly servers, versioned by capability flags in [PoolLinkInstanceInfo]
// rather than by the API version, and a self-hosted Warmbly speaks it for
// you; the self-hosted dashboard's view of it is [CloudLinkService].
//
// On a deployment that has the pool link switched off every route answers
// 501 Not Implemented.
type PoolLinkService service

// Lifecycle of one device-code handshake, as reported in
// [PoolLinkCode.Status] and [PoolLinkPollResult.Status].
const (
	// PoolLinkCodePending is a code nobody has settled yet.
	PoolLinkCodePending = "pending"
	// PoolLinkCodeApproved is a code a member approved whose instance has not
	// yet collected its token. Poll delivers the token exactly once and moves
	// the code to [PoolLinkCodeClaimed].
	PoolLinkCodeApproved = "approved"
	// PoolLinkCodeClaimed is a spent code: the instance fetched its token.
	// Polling it again fails with code "pool_link_code_used".
	PoolLinkCodeClaimed = "claimed"
	// PoolLinkCodeDenied is a code a member declined. Polling it fails with
	// code "pool_link_denied".
	PoolLinkCodeDenied = "denied"
)

// PoolLinkStartParams identifies the instance that wants to link, as shown to
// the approving member.
type PoolLinkStartParams struct {
	// RemoteOrganizationID is the ID of the workspace on the self-hosted
	// instance that is linking. The link belongs to that workspace, not to the
	// whole instance, and the API refuses a start without it (409, code
	// "cloud_link_workspace_required").
	RemoteOrganizationID string `json:"remote_organization_id"`
	// InstanceName is the label the approval page shows, typically the host
	// name or the operator's INSTANCE_NAME.
	InstanceName string `json:"instance_name"`
	// InstanceURL is the instance's public base URL.
	InstanceURL string `json:"instance_url"`
	// InstanceVersion is the instance's build, for support and compatibility.
	InstanceVersion string `json:"instance_version"`
}

// PoolLinkCodeGrant is the device-code grant that opens a handshake.
type PoolLinkCodeGrant struct {
	// WorkspaceScoped is true when the link will belong to one instance
	// workspace ([PoolLinkStartParams.RemoteOrganizationID]) rather than the
	// whole instance. Current servers always answer true.
	WorkspaceScoped bool `json:"workspace_scoped"`
	// DeviceCode is the secret half: keep it on the instance and send it only
	// to [PoolLinkService.Poll].
	DeviceCode string `json:"device_code"`
	// UserCode is the short code the operator types in at VerificationURL.
	UserCode string `json:"user_code"`
	// VerificationURL is the page where a signed-in member approves the code.
	// It already carries the user code as a query parameter.
	VerificationURL string `json:"verification_url"`
	// ExpiresIn is how long the code stays approvable, in seconds (15 minutes
	// at the time of writing).
	ExpiresIn int `json:"expires_in"`
	// Interval is the minimum number of seconds between polls.
	Interval int `json:"interval"`
}

// PoolLinkPollResult is one answer to [PoolLinkService.Poll].
type PoolLinkPollResult struct {
	// Status is [PoolLinkCodePending] or [PoolLinkCodeApproved]; a denied or
	// spent code is reported as an error, not a status.
	Status string `json:"status"`
	// InstanceID is set once approved.
	InstanceID *string `json:"instance_id,omitempty"`
	// InstanceToken is the instance's long-lived credential for
	// /pool-link/instance/*. It is delivered exactly once: store it
	// immediately, because the next poll fails with "pool_link_code_used".
	InstanceToken string `json:"instance_token,omitempty"`
	// Organization is the workspace the instance was linked to, once approved.
	Organization *PoolLinkOrganization `json:"organization,omitempty"`
}

// PoolLinkOrganization is the cloud workspace a link belongs to.
type PoolLinkOrganization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PoolLinkCode describes a pending handshake to the member about to settle it.
type PoolLinkCode struct {
	// RemoteOrganizationID is the instance workspace that asked to link.
	RemoteOrganizationID *string `json:"remote_organization_id,omitempty"`
	ID                   string  `json:"id"`
	// UserCode is the code the operator typed in.
	UserCode string `json:"user_code"`
	// InstanceName, InstanceURL and InstanceVersion are what the instance
	// claimed about itself in [PoolLinkStartParams]; nothing here is verified.
	InstanceName    string `json:"instance_name"`
	InstanceURL     string `json:"instance_url"`
	InstanceVersion string `json:"instance_version"`
	// Status is one of the PoolLinkCode* constants.
	Status string `json:"status"`
	// OrganizationID and InstanceID are set once the code has been approved.
	OrganizationID *string `json:"organization_id,omitempty"`
	InstanceID     *string `json:"instance_id,omitempty"`
	// ExpiresAt is when an unsettled code stops being approvable.
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// PoolLinkInstance is a linked self-hosted instance as the workspace sees it.
type PoolLinkInstance struct {
	// RemoteOrganizationID is the instance workspace the link belongs to.
	RemoteOrganizationID *string `json:"remote_organization_id,omitempty"`
	ID                   string  `json:"id"`
	OrganizationID       string  `json:"organization_id"`
	Name                 string  `json:"name"`
	URL                  string  `json:"url"`
	// Version is the build the instance most recently reported.
	Version string `json:"version"`
	// CreatedBy is the member who approved the link, when still known.
	CreatedBy *string   `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	// LastSeenAt is the instance's most recent authenticated call.
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	// RevokedAt is set once the link has been ended from either side.
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	// MailboxCount is how many of the instance's mailboxes are enrolled in the
	// pool. It is filled by [PoolLinkService.ListInstances] only and is zero
	// on the instance returned from [PoolLinkService.ApproveCode].
	MailboxCount int `json:"mailbox_count"`
}

// Tiers reported in [PoolLinkPlan.Tier].
const (
	// PoolLinkTierFree warms a capped number of linked mailboxes at no cost.
	PoolLinkTierFree = "free"
	// PoolLinkTierPaid lifts the mailbox cap; the workspace's regular hosted
	// plan also counts as paid.
	PoolLinkTierPaid = "paid"
)

// PoolLinkPlan is the pool allowance the cloud computes for a workspace's
// linked instances.
type PoolLinkPlan struct {
	// Tier is [PoolLinkTierFree] or [PoolLinkTierPaid].
	Tier string `json:"tier"`
	// MailboxLimit is the cap on enrolled mailboxes across every linked
	// instance, or nil when unlimited.
	MailboxLimit *int `json:"mailbox_limit"`
	// Enrolled is how many linked mailboxes currently count against the cap.
	Enrolled int `json:"enrolled"`
	// Warming is how many of them have warmup on and not paused; connected is
	// not warming.
	Warming int `json:"warming"`
	// PriceUSD is the monthly price of the paid tier, for an upgrade prompt.
	PriceUSD int `json:"price_usd"`
	// UpgradeURL is where to send the operator to upgrade; empty when billing
	// is off or the paid tier has no price.
	UpgradeURL string `json:"upgrade_url,omitempty"`
	// ManageURL is the cloud billing page; empty when billing is off.
	ManageURL string `json:"manage_url,omitempty"`
	// WarmupEntitled is false when the cloud workspace itself is not allowed
	// to warm, in which case linked mailboxes will not warm either.
	WarmupEntitled bool `json:"warmup_entitled"`
}

// PoolLinkInstanceList is the answer to [PoolLinkService.ListInstances]: the
// linked instances plus the allowance they share.
type PoolLinkInstanceList struct {
	Data []PoolLinkInstance `json:"data"`
	Plan PoolLinkPlan       `json:"plan"`
}

// PoolLinkInstanceInfo is the cloud's status document for one linked
// instance, as the instance itself sees it. The self-hosted dashboard
// surfaces it through [CloudLinkStatus.Info].
type PoolLinkInstanceInfo struct {
	// SMTPAssociationProtocol and ManagedConsentProtocol advertise which
	// versions of those handshakes this cloud speaks; zero means unsupported.
	SMTPAssociationProtocol int                  `json:"smtp_association_protocol,omitempty"`
	ManagedConsentProtocol  int                  `json:"managed_consent_protocol,omitempty"`
	Instance                PoolLinkInstance     `json:"instance"`
	Organization            PoolLinkOrganization `json:"organization"`
	Plan                    PoolLinkPlan         `json:"plan"`
	// Redirects describes the root redirects the cloud serves for the
	// instance's domains; nil from a cloud that serves none.
	Redirects *PoolLinkRedirectOffer `json:"redirects,omitempty"`
	// SelfExecution is the grant to run warmup on the instance without
	// handing the cloud the mailbox credentials; nil from a cloud that
	// cannot.
	SelfExecution *PoolLinkSelfExecution `json:"self_execution,omitempty"`
}

// PoolLinkRedirectOffer is the root-redirect allowance a linked instance has.
type PoolLinkRedirectOffer struct {
	Available bool `json:"available"`
	// Host is the cloud host the redirect domains point at.
	Host  string `json:"host,omitempty"`
	Limit int    `json:"limit"`
	Used  int    `json:"used"`
}

// PoolLinkSelfExecution is the instance-facing view of the grant to execute
// warmup for enrolled mailboxes on the instance itself.
type PoolLinkSelfExecution struct {
	Protocol  int        `json:"protocol"`
	Granted   bool       `json:"granted"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// Who runs an enrolled mailbox's warmup, in [CloudLinkMailbox.Executor] and
// [PoolLinkMailboxState.Executor]. Empty reads as [PoolLinkExecutorCloud]. The
// server may add executors; treat an unknown one as cloud.
const (
	// PoolLinkExecutorCloud means Warmbly Cloud holds the credential and sends.
	PoolLinkExecutorCloud = "cloud"
	// PoolLinkExecutorInstance means the instance keeps the credential and
	// runs the warmup itself on commands the cloud hands it.
	PoolLinkExecutorInstance = "instance"
)

// DiagnosticParticipation is a mailbox's setting for the diagnostic sends
// (such as inbox placement tests) that use a shared pool of seed mailboxes.
type DiagnosticParticipation struct {
	// Mode is [TestParticipationLegacy], [TestParticipationDiagnostic] or
	// [TestParticipationOff].
	Mode string `json:"mode"`
	// Send and Receive say which directions the mailbox takes part in; both
	// must be false when Mode is off.
	Send    bool `json:"send"`
	Receive bool `json:"receive"`
	// SharedDailyLimit and RollingRecipientLimit cap the shared use; nil
	// leaves the server default.
	SharedDailyLimit      *int `json:"shared_daily_limit,omitempty"`
	RollingRecipientLimit *int `json:"rolling_recipient_limit,omitempty"`
}

// Diagnostic participation modes in [DiagnosticParticipation.Mode].
const (
	TestParticipationLegacy     = "legacy"
	TestParticipationDiagnostic = "diagnostic"
	TestParticipationOff        = "off"
)

// PoolLinkOffer is the dedicated warmup plan as an upgrade prompt shows it:
// the plan is not in the public catalog, so this is how its price is learned.
type PoolLinkOffer struct {
	// Available is false when billing is off or no price is attached, in which
	// case a checkout cannot work.
	Available        bool `json:"available"`
	MonthlyAvailable bool `json:"monthly_available"`
	YearlyAvailable  bool `json:"yearly_available"`
	// MonthlyUSD and YearlyUSD are list prices; YearlyUSD is zero when only
	// monthly is sold.
	MonthlyUSD float32 `json:"monthly_usd"`
	YearlyUSD  float32 `json:"yearly_usd"`
	Currency   string  `json:"currency"`
}

// PoolLinkWarmupSettings is the ramp an enrolled mailbox warms on. It is
// copied from the mailbox's own settings on the instance at enrollment time;
// zero values mean the cloud's defaults.
type PoolLinkWarmupSettings struct {
	// Base is the starting daily volume, Max the ceiling and Increase the
	// daily step between them.
	Base     int `json:"base"`
	Max      int `json:"max"`
	Increase int `json:"increase"`
	// ReplyRate is the percentage of warmup mail that gets a reply.
	ReplyRate int `json:"reply_rate"`
	// StartTime and EndTime bound the sending window, as "HH:MM".
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	// Days is a bitmask of weekdays the mailbox warms on.
	Days     int    `json:"days"`
	Timezone string `json:"timezone"`
}

// PoolLinkWarmupStatus is the live ramp state of an enrolled mailbox.
type PoolLinkWarmupStatus struct {
	Enabled  bool       `json:"enabled"`
	Paused   bool       `json:"paused"`
	PausedAt *time.Time `json:"paused_at,omitempty"`
	// StartedAt is when the ramp began.
	StartedAt time.Time `json:"started_at"`
	// CurrentVolume is today's sends so far against TargetVolume, today's
	// ramp target, which climbs toward MaxVolume.
	CurrentVolume int `json:"current_volume"`
	TargetVolume  int `json:"target_volume"`
	MaxVolume     int `json:"max_volume"`
	ReplyRate     int `json:"reply_rate"`
	DaysActive    int `json:"days_active"`
	// RampHold explains a ramp that is not climbing, so a target below the
	// plain ramp is never an unexplained drop.
	RampHold *PoolLinkWarmupRampHold `json:"ramp_hold,omitempty"`
	// PartnerLimit is present while today's target is capped by how many
	// partners the mailbox can still reach.
	PartnerLimit *PoolLinkWarmupPartnerLimit `json:"partner_limit,omitempty"`
	// SendFailure is present while the newest warmup send failed and no later
	// one was confirmed delivered.
	SendFailure *PoolLinkWarmupSendFailure `json:"send_failure,omitempty"`
	// NextSendAt is when the parked warmup wakeup fires; nil while nothing is
	// queued.
	NextSendAt *time.Time `json:"next_send_at,omitempty"`
}

// PoolLinkWarmupPartnerLimit explains a target held below the ramp because a
// mailbox never writes to the same partner twice in a day.
type PoolLinkWarmupPartnerLimit struct {
	// Reachable is how many partners are available to it today, including any
	// it already wrote to.
	Reachable int `json:"reachable"`
	// RampTarget is what the ramp alone would send today.
	RampTarget int `json:"ramp_target"`
}

// PoolLinkWarmupSendFailure is why the newest warmup send failed.
type PoolLinkWarmupSendFailure struct {
	Message string    `json:"message"`
	At      time.Time `json:"at"`
	// Kind classifies the failure when the server could.
	Kind           string     `json:"kind,omitempty"`
	FirstFailureAt *time.Time `json:"first_failure_at,omitempty"`
}

// PoolLinkWarmupRampHold explains a frozen ramp. It is present for the whole
// freeze; VolumeCut says whether today's volume is also reduced, which lasts
// a shorter window.
type PoolLinkWarmupRampHold struct {
	// Placements and Sends cover the last 48 hours.
	Placements int  `json:"placements"`
	Sends      int  `json:"sends"`
	VolumeCut  bool `json:"volume_cut"`
	// ResumesAt is when the ramp climbs again if nothing else lands.
	ResumesAt time.Time `json:"resumes_at"`
}

// PoolLinkWarmupHealth is the pool's reputation verdict on an enrolled
// mailbox.
type PoolLinkWarmupHealth struct {
	// State is one of the Band* constants: [BandHealthy], [BandWatch],
	// [BandThrottled], [BandQuarantined] or [BandBlocked].
	State string  `json:"state"`
	Score float64 `json:"score"`
	// Reason explains a state other than healthy.
	Reason string `json:"reason,omitempty"`
	// SpamScore is always zero; the field is kept so older clients keep
	// working. Read Score and Reason instead.
	SpamScore int `json:"spam_score"`
	// BlockedUntil is when a quarantine or block lifts on its own.
	BlockedUntil *time.Time `json:"blocked_until,omitempty"`
	EvaluatedAt  *time.Time `json:"evaluated_at,omitempty"`
	// PoolType is the pool the mailbox warms in, "premium" or "free". Source
	// is "cloud" when Warmbly Cloud warms the mailbox and reported this
	// standing, and empty for the instance's own pool.
	PoolType string `json:"pool_type,omitempty"`
	Source   string `json:"source,omitempty"`
	// Partner diversity counts confirmed warmup deliveries over seven days.
	PartnerMailboxes7d     int `json:"partner_mailboxes_7d"`
	PartnerDomains7d       int `json:"partner_domains_7d"`
	PartnerOrganizations7d int `json:"partner_organizations_7d"`
	// Received7d and Senders7d are the receiving side over the same window:
	// verified warmup arrivals and the distinct partners they came from.
	Received7d int `json:"received_7d"`
	Senders7d  int `json:"senders_7d"`
}

// PoolLinkMailboxState is the cloud's per-mailbox view of an enrolled
// mailbox, shown in both dashboards.
type PoolLinkMailboxState struct {
	// RemoteID is the mailbox's ID on the self-hosted instance;
	// EmailAccountID is its ID in the cloud workspace.
	RemoteID       string `json:"remote_id"`
	EmailAccountID string `json:"email_account_id"`
	Email          string `json:"email"`
	Name           string `json:"name"`
	// Provider is [ProviderGmail], [ProviderOutlook] or [ProviderSMTPIMAP].
	Provider string `json:"provider"`
	// Status is the cloud mailbox's connection state, for example
	// [MailboxStatusActive].
	Status     string    `json:"status"`
	EnrolledAt time.Time `json:"enrolled_at"`
	// Managed is true when the cloud holds the only credential (a Google or
	// Microsoft sign-in on Warmbly's own OAuth apps) and the instance sends
	// with brokered short-lived tokens.
	Managed bool                  `json:"managed"`
	Warmup  *PoolLinkWarmupStatus `json:"warmup,omitempty"`
	Health  *PoolLinkWarmupHealth `json:"health,omitempty"`
	// SentToday, Sent7d, Replied7d and SpamPlaced7d are warmup counters.
	SentToday    int `json:"sent_today"`
	Sent7d       int `json:"sent_7d"`
	Replied7d    int `json:"replied_7d"`
	SpamPlaced7d int `json:"spam_placed_7d"`
	// Errors are the cloud mailbox's open account errors.
	Errors []AccountError `json:"errors,omitempty"`
	// AuthState is the cloud's view of the mailbox credential.
	AuthState string                 `json:"auth_state"`
	Settings  PoolLinkWarmupSettings `json:"settings"`
	// Participation is the mailbox's diagnostic-test setting; nil when the
	// cloud reports none.
	Participation *DiagnosticParticipation `json:"participation,omitempty"`
	// ConsentCompletedAt is when the managed sign-in consent finished.
	ConsentCompletedAt *time.Time `json:"consent_completed_at,omitempty"`
	// Executor is who runs the mailbox's warmup ([PoolLinkExecutorCloud] or
	// [PoolLinkExecutorInstance]); empty reads as cloud.
	Executor string `json:"executor,omitempty"`
}

// PoolLinkWorkspaceMailbox is a Google or Microsoft mailbox connected directly
// on the cloud workspace that a linked instance may adopt.
type PoolLinkWorkspaceMailbox struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	// Provider is [ProviderGmail] or [ProviderOutlook].
	Provider string `json:"provider"`
	Status   string `json:"status"`
	// MailHost is the mailbox's mail host when known, for example
	// "google.com".
	MailHost string `json:"mail_host,omitempty"`
}

// poolLinkPollFloor is the shortest gap the poll helpers will ever leave
// between two polls, whatever interval the server reported: the handshake
// routes share the per-IP sign-in budget and must not be hammered.
var poolLinkPollFloor = time.Second

// poolLinkWait sleeps for the longer of interval and poolLinkPollFloor, or
// until ctx is done, in which case it reports ctx.Err().
func poolLinkWait(ctx context.Context, interval time.Duration) error {
	if interval < poolLinkPollFloor {
		interval = poolLinkPollFloor
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// --- public handshake (no credential, per-IP rate limited) ---

// StartCode opens a device-code handshake on behalf of a self-hosted instance
// that holds no credential yet. Show [PoolLinkCodeGrant.UserCode] and
// [PoolLinkCodeGrant.VerificationURL] to the operator, keep
// [PoolLinkCodeGrant.DeviceCode] private, and follow up with
// [PoolLinkService.WaitForApproval].
//
// Public and per-IP rate limited; the client's credential, if any, is sent
// but ignored.
func (s *PoolLinkService) StartCode(ctx context.Context, params *PoolLinkStartParams, opts ...RequestOption) (*PoolLinkCodeGrant, *Response, error) {
	return send[PoolLinkCodeGrant](ctx, s.client.post, "pool-link/codes", params, opts)
}

// Poll asks once whether the code behind deviceCode has been approved. While
// nobody has settled it the result is [PoolLinkCodePending]; once a member
// approves it the result is [PoolLinkCodeApproved] with the instance token,
// which is delivered on this one call only.
//
// A denied code fails with an [*Error] whose Code is "pool_link_denied"; an
// already collected one with "pool_link_code_used"; an unknown or expired one
// with "pool_link_code_not_found". Wait at least
// [PoolLinkCodeGrant.Interval] seconds between calls: the route is public and
// draws on the per-IP sign-in budget.
func (s *PoolLinkService) Poll(ctx context.Context, deviceCode string, opts ...RequestOption) (*PoolLinkPollResult, *Response, error) {
	body := struct {
		DeviceCode string `json:"device_code"`
	}{DeviceCode: deviceCode}
	return send[PoolLinkPollResult](ctx, s.client.post, "pool-link/poll", body, opts)
}

// WaitForApproval polls grant every [PoolLinkCodeGrant.Interval] seconds
// (never faster than once a second) until the code is approved, the server
// reports a terminal error, or ctx is done. On success the result carries the
// one-time instance token.
//
// The server retires an unapproved code after [PoolLinkCodeGrant.ExpiresIn]
// seconds, after which the loop ends with a "pool_link_code_not_found" error;
// bound ctx yourself to give up sooner.
func (s *PoolLinkService) WaitForApproval(ctx context.Context, grant *PoolLinkCodeGrant, opts ...RequestOption) (*PoolLinkPollResult, error) {
	interval := time.Duration(grant.Interval) * time.Second
	for {
		res, _, err := s.Poll(ctx, grant.DeviceCode, opts...)
		if err != nil {
			return nil, err
		}
		if res.Status == PoolLinkCodeApproved {
			return res, nil
		}
		if err := poolLinkWait(ctx, interval); err != nil {
			return nil, err
		}
	}
}

// --- member side (session-only) ---

// DescribeCode shows the signed-in member what they are about to link: the
// instance's self-reported name, URL and version, and whether the code is
// still pending. Session-only.
//
// An unknown or expired code fails with "pool_link_code_not_found".
func (s *PoolLinkService) DescribeCode(ctx context.Context, userCode string, opts ...RequestOption) (*PoolLinkCode, *Response, error) {
	return fetch[PoolLinkCode](ctx, s.client, "pool-link/codes/"+url.PathEscape(userCode), opts)
}

// ApproveCode admits the instance behind userCode into the workspace
// organizationID and returns the new linked instance. The instance collects
// its token on its next poll.
//
// organizationID is the workspace to link to, which need not be the session's
// active one; pass "" to fall back to the session's workspace. The caller
// needs the manage-settings permission in that workspace. Session-only; the
// call is recorded in the workspace audit log.
//
// A code that is no longer pending fails with "pool_link_code_used".
func (s *PoolLinkService) ApproveCode(ctx context.Context, userCode, organizationID string, opts ...RequestOption) (*PoolLinkInstance, *Response, error) {
	body := struct {
		OrganizationID string `json:"organization_id,omitempty"`
	}{OrganizationID: organizationID}
	return send[PoolLinkInstance](ctx, s.client.post, "pool-link/codes/"+url.PathEscape(userCode)+"/approve", body, opts)
}

// DenyCode declines the handshake behind userCode. The instance's next poll
// fails with "pool_link_denied" and the code cannot be approved afterwards.
// Session-only.
func (s *PoolLinkService) DenyCode(ctx context.Context, userCode string, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "pool-link/codes/"+url.PathEscape(userCode)+"/deny", nil, nil, opts...)
}

// ListInstances lists the instances linked to the session's workspace,
// together with the pool allowance they share. Session-only; needs the
// manage-settings permission.
func (s *PoolLinkService) ListInstances(ctx context.Context, opts ...RequestOption) (*PoolLinkInstanceList, *Response, error) {
	out, resp, err := fetch[PoolLinkInstanceList](ctx, s.client, "pool-link/instances", opts)
	if err != nil {
		return nil, resp, err
	}
	if out.Data == nil {
		out.Data = []PoolLinkInstance{}
	}
	return out, resp, nil
}

// RevokeInstance unlinks an instance: its token stops working, every mailbox
// it enrolled leaves the pool and their credentials are deleted on the cloud.
// The instance's next call fails with "pool_link_revoked" and its local
// warmup takes over. Session-only; needs the manage-settings permission and
// is recorded in the workspace audit log.
func (s *PoolLinkService) RevokeInstance(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "pool-link/instances/"+url.PathEscape(id), opts...)
}

// Offer returns the dedicated warmup plan's price and whether it can be bought
// here, for an upgrade prompt. The plan is not in the public catalog
// ([MetaService.Plans]), so this is the only place its price is published.
// Session-only; any signed-in member may read it. On a deployment with billing
// off the offer comes back with Available false.
func (s *PoolLinkService) Offer(ctx context.Context, opts ...RequestOption) (*PoolLinkOffer, *Response, error) {
	return fetch[PoolLinkOffer](ctx, s.client, "pool-link/offer", opts)
}

// Checkout opens a hosted checkout for the warmup plan and returns its URL.
// interval is [DurationMonth] (the default when empty) or [DurationYear]; the
// price is resolved on the server. It fails with 503 when the plan has no
// price for that period and 404 when the plan is not configured on the
// deployment. Session-only; needs the manage-billing organization permission
// and a recent confirmation ([AuthService.Reauth]). Recorded in the audit log.
func (s *PoolLinkService) Checkout(ctx context.Context, interval string, opts ...RequestOption) (string, *Response, error) {
	body := struct {
		Interval string `json:"interval,omitempty"`
	}{Interval: interval}
	var out struct {
		CheckoutURL string `json:"checkout_url"`
	}
	resp, err := s.client.post(ctx, "pool-link/checkout", body, &out, opts...)
	if err != nil {
		return "", resp, err
	}
	return out.CheckoutURL, resp, nil
}
