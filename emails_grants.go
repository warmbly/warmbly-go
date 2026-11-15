package warmbly

import (
	"context"
	"net/url"
	"time"
)

// --- administrator grants ---
//
// A domain-wide grant lets one administrator connect every mailbox of a
// Google Workspace domain or a Microsoft 365 tenant without a password or a
// sign-in per mailbox.
//
// Every route here is session only (an API key is refused) and needs the
// manage-emails permission ([OrgPermManageEmails]). Recording, removing and
// using a grant (the two finish calls, the delete and the connect) also need
// a recent sign-in and are refused with [ErrCodeReauthRequired] otherwise.
// Starting a grant and connecting through one are refused on a Partnership
// workspace that has no OAuth app of its own, with
// [ErrCodePartnershipOAuthAppRequired]. Instances without the feature answer
// 404, except [EmailService.SigninMigration].

// Error codes carried in [Error.Code] by grant calls.
const (
	// ErrCodeMailboxGrantNotConfigured: this instance has no credentials for
	// the provider; [GrantConfig] names the missing settings.
	ErrCodeMailboxGrantNotConfigured = "mailbox_grant_not_configured"
	// ErrCodeGoogleDelegationUnauthorized: the Google administrator has not
	// authorized [GrantConfig.GoogleClientID] for [GrantConfig.GoogleScopes].
	ErrCodeGoogleDelegationUnauthorized = "google_delegation_unauthorized"
	// ErrCodeGoogleDelegationRetired refuses a new Google grant for a domain
	// the workspace holds none for.
	ErrCodeGoogleDelegationRetired = "google_delegation_retired"
	// ErrCodeMicrosoftConsentMissing: the Microsoft administrator has not
	// consented.
	ErrCodeMicrosoftConsentMissing = "microsoft_consent_missing"
	// ErrCodeMailboxGrantStateInvalid: the sign-in state is unknown, used or
	// expired; start again.
	ErrCodeMailboxGrantStateInvalid = "mailbox_grant_state_invalid"
	// ErrCodeMailboxGrantDomainMismatch: the domain is empty or the
	// administrator address is not on it.
	ErrCodeMailboxGrantDomainMismatch = "mailbox_grant_domain_mismatch"
	// ErrCodeMailboxGrantProofMissing: control of the domain is not proved:
	// the DNS TXT record from [GoogleGrantStart] is not published, or the
	// account that signed in is not the administrator entered (or not a super
	// administrator).
	ErrCodeMailboxGrantProofMissing = "mailbox_grant_proof_missing"
	// ErrCodeMailboxGrantInactive: the grant failed its last check; run
	// [EmailService.CheckGrant] after fixing it.
	ErrCodeMailboxGrantInactive = "mailbox_grant_inactive"
	// ErrCodeMailboxGrantMailboxUnreachable: the grant cannot reach this
	// mailbox.
	ErrCodeMailboxGrantMailboxUnreachable = "mailbox_grant_mailbox_unreachable"
	// ErrCodeMailboxGrantUnavailable: the provider did not answer as
	// expected; try again shortly.
	ErrCodeMailboxGrantUnavailable = "mailbox_grant_unavailable"
)

// Grant providers in [DomainGrant.Provider].
const (
	GrantProviderGoogle    = "google"
	GrantProviderMicrosoft = "microsoft"
)

// Methods a Google grant start offers in [GoogleGrantStart.Method].
const (
	// GrantMethodSignin means the administrator signs in with Google at
	// [GoogleGrantStart.URL].
	GrantMethodSignin = "signin"
	// GrantMethodDNS means the administrator publishes a TXT record.
	GrantMethodDNS = "dns"
)

// GrantConfig says which administrator connections this instance can take
// and what a Google administrator has to authorize.
type GrantConfig struct {
	GoogleEnabled bool `json:"google_enabled"`
	// GoogleClientID and GoogleScopes are what the administrator authorizes
	// under domain-wide delegation in the Google Admin console.
	GoogleClientID   string   `json:"google_client_id,omitempty"`
	GoogleScopes     []string `json:"google_scopes"`
	MicrosoftEnabled bool     `json:"microsoft_enabled"`
	// GoogleMissing and MicrosoftMissing name the settings an operator still
	// has to set; empty when the provider is enabled.
	GoogleMissing    []string `json:"google_missing"`
	MicrosoftMissing []string `json:"microsoft_missing"`
	// GoogleMailboxClientID is the per-mailbox Google sign-in client a
	// Workspace administrator marks trusted; empty when none is configured.
	GoogleMailboxClientID string `json:"google_mailbox_client_id,omitempty"`
}

// DomainGrant is an administrator's grant over a Google Workspace domain or a
// Microsoft 365 tenant.
type DomainGrant struct {
	ID string `json:"id"`
	// Provider is [GrantProviderGoogle] or [GrantProviderMicrosoft].
	Provider string `json:"provider"`
	// Tenant is the Google domain or the Microsoft tenant id.
	Tenant     string   `json:"tenant"`
	AdminEmail string   `json:"admin_email,omitempty"`
	Domains    []string `json:"domains"`
	// Status is "active" while the grant works; LastError says why it does
	// not. The set is open.
	Status     string     `json:"status"`
	LastError  string     `json:"last_error,omitempty"`
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	// Mailboxes counts the mailboxes connected through the grant.
	Mailboxes int `json:"mailboxes"`
}

// GoogleGrantStart says how the workspace proves it controls the domain.
type GoogleGrantStart struct {
	// Method is [GrantMethodSignin] when the administrator can sign in with
	// Google, else [GrantMethodDNS].
	Method string `json:"method"`
	// URL and State are set for [GrantMethodSignin]: send the administrator to
	// URL, then finish with the state and the code Google returns.
	URL   string `json:"url,omitempty"`
	State string `json:"state,omitempty"`
	// TXTName and TXTValue are the DNS proof, which always works.
	TXTName  string `json:"txt_name"`
	TXTValue string `json:"txt_value"`
}

// GoogleGrantFinishParams completes a Google grant: a sign-in code with its
// state, or the domain and administrator whose DNS proof was published.
type GoogleGrantFinishParams struct {
	State      string `json:"state,omitempty"`
	Code       string `json:"code,omitempty"`
	Domain     string `json:"domain,omitempty"`
	AdminEmail string `json:"admin_email,omitempty"`
}

// MicrosoftGrantStart is the administrator consent URL and its state.
type MicrosoftGrantStart struct {
	URL   string `json:"url"`
	State string `json:"state"`
}

// DirectoryUser is one account in a granted domain or tenant.
type DirectoryUser struct {
	// ID is the subject tokens are minted for: the address (Google) or the
	// Graph user id (Microsoft). Pass it in [GrantConnectParams.UserIDs].
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	Connected bool   `json:"connected"`
	// EmailAccountID is the connected mailbox, when Connected.
	EmailAccountID *string `json:"email_account_id,omitempty"`
	// Upgrade marks a connected mailbox that signs in on its own; connecting
	// it moves it onto the grant and keeps its history.
	Upgrade bool `json:"upgrade"`
}

// GrantConnectParams picks the directory users to connect through a grant.
type GrantConnectParams struct {
	// UserIDs are [DirectoryUser.ID] values.
	UserIDs []string `json:"user_ids,omitempty"`
	// All also connects every enabled user not connected yet, and every one
	// marked [DirectoryUser.Upgrade].
	All bool `json:"all,omitempty"`
	// Options carries the import settings; the zero value uses the defaults.
	Options MailboxImportOptions `json:"options"`
}

// Kinds in [SigninMigrationGroup.Kind].
const (
	// SigninMigrationWorkspace moves to an administrator's grant.
	SigninMigrationWorkspace = "workspace"
	// SigninMigrationPersonal is a shared address such as gmail.com, which
	// moves to an app password ([EmailService.SwitchToAppPassword]).
	SigninMigrationPersonal = "personal"
)

// MigrationMailbox is one mailbox still on per-mailbox Google sign-in.
type MigrationMailbox struct {
	ID     string `json:"id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	Status string `json:"status"`
	// VendorConnectionID is the healthy [VendorConnection] that can sign in
	// again as the mailbox, when one can; Vendor is its vendor.
	VendorConnectionID *string `json:"vendor_connection_id,omitempty"`
	Vendor             string  `json:"vendor,omitempty"`
}

// SigninMigrationGroup groups the workspace's mailboxes on the retiring
// per-mailbox Google sign-in by domain.
type SigninMigrationGroup struct {
	Domain string `json:"domain"`
	// Kind is [SigninMigrationWorkspace] or [SigninMigrationPersonal].
	Kind string `json:"kind"`
	// GrantID is the workspace's active grant covering the domain, when there
	// is one.
	GrantID   *string            `json:"grant_id,omitempty"`
	Mailboxes []MigrationMailbox `json:"mailboxes"`
}

// SigninMigration lists the mailboxes still on per-mailbox Google sign-in.
type SigninMigration struct {
	Data []SigninMigrationGroup `json:"data"`
	// Total counts the mailboxes across all groups.
	Total int `json:"total"`
}

// GrantConfig reports which administrator connections this instance can
// take and what a Google administrator has to authorize.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) GrantConfig(ctx context.Context, opts ...RequestOption) (*GrantConfig, *Response, error) {
	return fetch[GrantConfig](ctx, s.client, "emails/grants/config", opts)
}

// ListGrants returns the workspace's administrator grants. The list is not
// paginated.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) ListGrants(ctx context.Context, opts ...RequestOption) ([]DomainGrant, *Response, error) {
	return fetchData[DomainGrant](ctx, s.client, "emails/grants", opts)
}

// SigninMigration lists the workspace's mailboxes still on per-mailbox
// Google sign-in, by domain, with where each moves. It answers an empty list,
// rather than 404, on an instance without grants.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) SigninMigration(ctx context.Context, opts ...RequestOption) (*SigninMigration, *Response, error) {
	out, resp, err := fetch[SigninMigration](ctx, s.client, "emails/grants/migration", opts)
	if err != nil {
		return nil, resp, err
	}
	if out.Data == nil {
		out.Data = []SigninMigrationGroup{}
	}
	return out, resp, nil
}

// StartGoogleGrant begins a grant over a Google Workspace domain and says how
// to prove control of it. adminEmail must be on domain.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) StartGoogleGrant(ctx context.Context, domain, adminEmail string, opts ...RequestOption) (*GoogleGrantStart, *Response, error) {
	body := struct {
		Domain     string `json:"domain"`
		AdminEmail string `json:"admin_email"`
	}{Domain: domain, AdminEmail: adminEmail}
	return send[GoogleGrantStart](ctx, s.client.post, "emails/grants/google/start", body, opts)
}

// FinishGoogleGrant records the grant once the workspace has proved the
// domain and the administrator is one in the directory, answering 201. A
// sign-in state is single-use; repeating a DNS finish re-verifies the same
// grant.
//
// Session only; needs the manage-emails permission and a recent sign-in.
func (s *EmailService) FinishGoogleGrant(ctx context.Context, params *GoogleGrantFinishParams, opts ...RequestOption) (*DomainGrant, *Response, error) {
	return send[DomainGrant](ctx, s.client.post, "emails/grants/google/finish", params, opts)
}

// StartMicrosoftGrant returns the administrator consent URL and state for a
// grant over a Microsoft 365 tenant.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) StartMicrosoftGrant(ctx context.Context, opts ...RequestOption) (*MicrosoftGrantStart, *Response, error) {
	return send[MicrosoftGrantStart](ctx, s.client.post, "emails/grants/microsoft/start", nil, opts)
}

// FinishMicrosoftGrant records the grant after the administrator consented,
// answering 201. The state is single-use, so a repeat is refused rather than
// recorded twice.
//
// Session only; needs the manage-emails permission and a recent sign-in.
func (s *EmailService) FinishMicrosoftGrant(ctx context.Context, state, code string, opts ...RequestOption) (*DomainGrant, *Response, error) {
	body := struct {
		State string `json:"state"`
		Code  string `json:"code"`
	}{State: state, Code: code}
	return send[DomainGrant](ctx, s.client.post, "emails/grants/microsoft/finish", body, opts)
}

// GetGrant retrieves one administrator grant.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) GetGrant(ctx context.Context, id string, opts ...RequestOption) (*DomainGrant, *Response, error) {
	return fetch[DomainGrant](ctx, s.client, "emails/grants/"+url.PathEscape(id), opts)
}

// CheckGrant verifies a grant again and puts its mailboxes back to work if it
// recovered, returning the refreshed grant.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) CheckGrant(ctx context.Context, id string, opts ...RequestOption) (*DomainGrant, *Response, error) {
	return send[DomainGrant](ctx, s.client.post, "emails/grants/"+url.PathEscape(id)+"/check", nil, opts)
}

// DeleteGrant removes a grant. The mailboxes connected through it stop. The
// server answers 204.
//
// Session only; needs the manage-emails permission and a recent sign-in.
func (s *EmailService) DeleteGrant(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "emails/grants/"+url.PathEscape(id), opts...)
}

// GrantUsers lists the granted directory: every account in the domain or
// tenant and whether it is connected. The list is not paginated.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) GrantUsers(ctx context.Context, id string, opts ...RequestOption) ([]DirectoryUser, *Response, error) {
	return fetchData[DirectoryUser](ctx, s.client, "emails/grants/"+url.PathEscape(id)+"/users", opts)
}

// ConnectGrantUsers imports the picked directory users (or, with All, every
// enabled one not yet connected) as a background import and returns it,
// answering 201. Follow it with [EmailService.GetImport]. Repeating the call
// updates what the first connected.
//
// Session only; needs the manage-emails permission and a recent sign-in.
func (s *EmailService) ConnectGrantUsers(ctx context.Context, id string, params *GrantConnectParams, opts ...RequestOption) (*MailboxImport, *Response, error) {
	if params == nil {
		params = &GrantConnectParams{}
	}
	return send[MailboxImport](ctx, s.client.post, "emails/grants/"+url.PathEscape(id)+"/connect", params, opts)
}

// SwitchToAppPassword moves mailbox id off per-mailbox Google sign-in onto
// Gmail's IMAP and SMTP with a 16-letter app password, checked live before it
// is stored, and returns the mailbox. Spaces in appPassword are ignored. A
// malformed password is refused with [ErrCodeAppPasswordInvalid], and a
// mailbox that is not on Google sign-in (including one already switched, so a
// retry changes nothing) with a 409 [ErrCodeMailboxNotGoogleSignin].
//
// Session only; needs the manage-emails permission.
func (s *EmailService) SwitchToAppPassword(ctx context.Context, id, appPassword string, opts ...RequestOption) (*Email, *Response, error) {
	body := struct {
		AppPassword string `json:"app_password"`
	}{AppPassword: appPassword}
	return send[Email](ctx, s.client.post, "emails/onboarding/app-password/"+url.PathEscape(id), body, opts)
}

// Error codes carried in [Error.Code] by [EmailService.SwitchToAppPassword].
const (
	// ErrCodeAppPasswordInvalid is the 400 for an app password that is not
	// 16 letters.
	ErrCodeAppPasswordInvalid = "app_password_invalid"
	// ErrCodeMailboxNotGoogleSignin is the 409 for a mailbox that does not
	// sign in with Google on its own or through a grant.
	ErrCodeMailboxNotGoogleSignin = "mailbox_not_google_signin"
)
