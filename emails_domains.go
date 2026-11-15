package warmbly

import (
	"context"
	"net/url"
	"time"
)

// --- sending domains ---
//
// A sending domain is the part after the @ of one or more connected
// mailboxes. These routes set the tracking host every mailbox on the domain
// uses and the redirect that sends the bare domain to the workspace website,
// directly or through the inbox vendor holding the domain.
//
// Every route here is session only (an API key is refused) and needs the
// manage-emails permission ([OrgPermManageEmails]). A domain with no mailbox
// in the workspace answers 404 with [ErrCodeSendingDomainNotInWorkspace].

// Error codes carried in [Error.Code] by sending domain calls.
const (
	// ErrCodeSendingDomainNotInWorkspace is the 404 for a domain no mailbox in
	// the workspace sends from.
	ErrCodeSendingDomainNotInWorkspace = "sending_domain_not_in_workspace"
	// ErrCodeSendingDomainSharedProvider refuses a redirect on a shared
	// mailbox provider's domain, such as gmail.com.
	ErrCodeSendingDomainSharedProvider = "sending_domain_shared_provider"
	// ErrCodeSendingDomainBulkInvalid is the 400 for a bulk setup with no
	// domain, more than [MaxBulkDomains], nothing to set, a tracking label
	// that is not one DNS label, or an unknown served_by.
	ErrCodeSendingDomainBulkInvalid = "sending_domain_bulk_invalid"
	// ErrCodeTrackingHostNotConfigured is the 400 when this instance has no
	// open and click tracking host to point a domain at.
	ErrCodeTrackingHostNotConfigured = "tracking_host_not_configured"
	// ErrCodeDomainRedirectInvalidTarget refuses a redirect destination that
	// is not a usable website URL.
	ErrCodeDomainRedirectInvalidTarget = "domain_redirect_invalid_target"
	// ErrCodeDomainRedirectTaken refuses a domain whose redirect another
	// workspace holds.
	ErrCodeDomainRedirectTaken = "domain_redirect_taken"
	// ErrCodeRedirectTargetNotOwnDomain is the 400 for a redirect destination
	// outside the workspace's own domains and proved websites.
	ErrCodeRedirectTargetNotOwnDomain = "redirect_target_not_own_domain"
	// ErrCodeDomainRedirectLinked is the 409 for a redirect a linked
	// self-hosted instance manages; change it there.
	ErrCodeDomainRedirectLinked = "domain_redirect_linked"
	// ErrCodeDomainRedirectCloudUnavailable refuses [RedirectServedByCloud]
	// when the instance is not linked to Warmbly Cloud or the cloud does not
	// offer redirects.
	ErrCodeDomainRedirectCloudUnavailable = "domain_redirect_cloud_unavailable"
	// ErrCodeDomainRedirectCloudUnreachable refuses a move or removal Warmbly
	// Cloud could not be told about; try again shortly.
	ErrCodeDomainRedirectCloudUnreachable = "domain_redirect_cloud_unreachable"
	// ErrCodeDomainRedirectLimit refuses a cloud-served redirect past the
	// linked instance's allowance.
	ErrCodeDomainRedirectLimit = "domain_redirect_limit"
	// ErrCodeMailboxVendorDomainUnsupported refuses a vendor forwarding or
	// tracking call the vendor's API cannot do for the domain.
	ErrCodeMailboxVendorDomainUnsupported = "mailbox_vendor_domain_unsupported"
)

// Who answers visitors of a domain redirect, in [DomainRedirect.ServedBy],
// [DomainRedirectParams.ServedBy] and [BulkDomainSetupParams.ServedBy].
const (
	// RedirectServedByInstance means this instance serves the redirect.
	RedirectServedByInstance = "instance"
	// RedirectServedByCloud means Warmbly Cloud serves it for a linked
	// self-hosted instance.
	RedirectServedByCloud = "cloud"
)

// Redirect reachability states in [RedirectReach.Status]. The set is open.
const (
	RedirectReachOK = "ok"
	// RedirectReachNotReaching: something answered, but not with the
	// redirect.
	RedirectReachNotReaching = "not_reaching"
	// RedirectReachHTTPSError: the redirect answers over http, but https has
	// no valid certificate.
	RedirectReachHTTPSError = "https_error"
	// RedirectReachUnreachable: nothing answered, which may be the server
	// failing to reach its own address.
	RedirectReachUnreachable = "unreachable"
)

// RedirectReach is one HTTP check of a verified redirect's domain.
type RedirectReach struct {
	// Status is one of the RedirectReach* constants.
	Status string `json:"status"`
	// Hint names the likely cause of a miss: "not_routed", "host_header",
	// "wrong_target", "certificate", "no_listener", "settling", "edge_record"
	// or "edge_inactive". The set is open.
	Hint string `json:"hint,omitempty"`
	// Detail is one sentence describing what the check saw.
	Detail string `json:"detail,omitempty"`
	// Proxy is the web server that answered in Warmbly's place, when it could
	// be told, for example "traefik", "nginx" or "caddy".
	Proxy     string     `json:"proxy,omitempty"`
	CheckedAt *time.Time `json:"checked_at,omitempty"`
}

// RedirectWebsite is a destination website and the TXT record that proves it
// belongs to the workspace.
type RedirectWebsite struct {
	Domain   string `json:"domain"`
	Verified bool   `json:"verified"`
	// Record is the TXT record to publish at the website's DNS; nil when the
	// website needs none.
	Record *DNSRecord `json:"record,omitempty"`
}

// DomainRedirect sends a sending domain's root to the workspace's website.
type DomainRedirect struct {
	ID            string     `json:"id"`
	Domain        string     `json:"domain"`
	TargetURL     string     `json:"target_url"`
	IncludeWWW    bool       `json:"include_www"`
	Verified      bool       `json:"verified"`
	VerifiedAt    *time.Time `json:"verified_at,omitempty"`
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	// Records are the DNS records to add, each with whether it is in place.
	Records []DNSRecord `json:"records"`
	// ServedBy is [RedirectServedByInstance] or [RedirectServedByCloud].
	ServedBy string `json:"served_by"`
	// ServeHost is the tracking host that answers, which a proxy routes the
	// domain to.
	ServeHost string `json:"serve_host,omitempty"`
	// Reach is the last HTTP check of the domain; nil until DNS verifies.
	Reach *RedirectReach `json:"reach,omitempty"`
	// TargetActive is false when the destination is not on one of the
	// workspace's own domains, so the redirect is not served.
	TargetActive *bool `json:"target_active,omitempty"`
	// TargetInactiveReason says why, when TargetActive is false.
	TargetInactiveReason string `json:"target_inactive_reason,omitempty"`
	// Website is set when the destination counts as the workspace's only once
	// its TXT proof is published; see [EmailService.VerifyRedirectWebsite].
	Website *RedirectWebsite `json:"website,omitempty"`
}

// VendorDomainLink is a sending domain held by an inbox vendor account, with
// what the vendor's API can do for it.
type VendorDomainLink struct {
	// Vendor is a [MailboxVendor.ID].
	Vendor string `json:"vendor"`
	// ConnectionID is the [VendorConnection] holding the domain.
	ConnectionID string `json:"connection_id"`
	// Forwarding is the vendor's current root forwarding target.
	Forwarding string `json:"forwarding,omitempty"`
	CanForward bool   `json:"can_forward"`
	// CanUnforward reports whether an empty target removes the vendor's
	// forwarding.
	CanUnforward bool `json:"can_unforward"`
	// ForwardingReviewed means the vendor's staff apply a change later.
	ForwardingReviewed bool `json:"forwarding_reviewed"`
	CanDNS             bool `json:"can_dns"`
	// DNSTypes lists the record types the vendor's API can write.
	DNSTypes []string `json:"dns_types"`
}

// Tracking suggestion states in [TrackingSuggestion.Status]. The set is open.
const (
	// TrackingSuggestionActive means a mailbox on the domain already uses the
	// host and it is verified.
	TrackingSuggestionActive = "active"
	// TrackingSuggestionFound means DNS already points the host at this
	// instance.
	TrackingSuggestionFound = "found"
	// TrackingSuggestionSuggested means the host is a proposal to add.
	TrackingSuggestionSuggested = "suggested"
)

// TrackingSuggestion is the tracking host a domain's mailboxes can use.
type TrackingSuggestion struct {
	Host string `json:"host"`
	// Status is one of the TrackingSuggestion* constants.
	Status string `json:"status"`
	// CNAMETarget is the value the host's CNAME record must point at.
	CNAMETarget string `json:"cname_target"`
	// VendorDomain is set when a connected vendor account holds the domain, so
	// its API may write the CNAME ([EmailService.SetDomainVendorTracking]).
	VendorDomain *VendorDomainLink `json:"vendor_domain,omitempty"`
}

// TrackingDomainUse is one custom tracking host a domain's mailboxes use.
type TrackingDomainUse struct {
	Host      string `json:"host"`
	Verified  bool   `json:"verified"`
	Mailboxes int    `json:"mailboxes"`
}

// SendingDomain is one domain the workspace sends from.
type SendingDomain struct {
	Domain    string   `json:"domain"`
	Mailboxes int      `json:"mailboxes"`
	MailHosts []string `json:"mail_hosts"`
	// AuthState is [AuthStatePassing], [AuthStateFailing] or
	// [AuthStateUnknown].
	AuthState       string              `json:"auth_state"`
	AuthSPF         bool                `json:"auth_spf"`
	AuthDKIM        bool                `json:"auth_dkim"`
	AuthDMARC       bool                `json:"auth_dmarc"`
	TrackingDomains []TrackingDomainUse `json:"tracking_domains"`
	Redirect        *DomainRedirect     `json:"redirect,omitempty"`
	// Vendors are the inbox vendors this domain's mailboxes were imported
	// from, and VendorConnectionIDs the connections.
	Vendors             []string `json:"vendors"`
	VendorConnectionIDs []string `json:"vendor_connection_ids"`
	// VendorDomain is set when a connected vendor account holds this domain.
	VendorDomain *VendorDomainLink `json:"vendor_domain,omitempty"`
}

// DomainTrackingResult is the outcome of setting a domain's tracking host.
type DomainTrackingResult struct {
	// Tracking is the verification verdict for the host.
	Tracking TrackingDomainStatus `json:"tracking"`
	// Mailboxes is how many mailboxes on the domain now use the host.
	Mailboxes int `json:"mailboxes"`
}

// DomainRedirectParams sets a domain's redirect.
type DomainRedirectParams struct {
	// TargetURL is where the bare domain redirects. It must be on one of the
	// workspace's own domains (a mailbox domain, its verified tracking or
	// forms domain, or a subdomain of one) or on a website first proved with
	// [EmailService.VerifyRedirectWebsite]; anything else is refused with
	// [ErrCodeRedirectTargetNotOwnDomain].
	TargetURL string `json:"target_url"`
	// IncludeWWW also redirects www.<domain>. Nil means true.
	IncludeWWW *bool `json:"include_www,omitempty"`
	// ServedBy is [RedirectServedByInstance] or [RedirectServedByCloud].
	// Empty keeps the current choice.
	ServedBy string `json:"served_by,omitempty"`
}

// MaxBulkDomains is the most domains one [EmailService.BulkSetupDomains] call
// accepts.
const MaxBulkDomains = 100

// BulkDomainSetupParams sets up tracking and a root redirect for up to
// [MaxBulkDomains] domains at once. Give a tracking label or host, a redirect
// URL, or both.
type BulkDomainSetupParams struct {
	Domains []string `json:"domains"`
	// TrackingLabel sets <label>.<domain> as every listed domain's tracking
	// host. It is one DNS label, for example "track".
	TrackingLabel string `json:"tracking_label,omitempty"`
	// RedirectURL sends every listed domain's root to one website.
	RedirectURL string `json:"redirect_url,omitempty"`
	// TrackingHosts and RedirectURLs override the shared value for the
	// domains they name.
	TrackingHosts map[string]string `json:"tracking_hosts,omitempty"`
	RedirectURLs  map[string]string `json:"redirect_urls,omitempty"`
	// ServedBy picks where a redirect not forwarded by a vendor is served
	// from; empty keeps each domain's current choice.
	ServedBy string `json:"served_by,omitempty"`
}

// Routes by which a bulk setup applied a setting, in [BulkDomainTracking.Via]
// and [BulkDomainRedirect.Via].
const (
	// BulkViaVendor means the domain's inbox vendor applied it.
	BulkViaVendor = "vendor"
	// BulkViaDNS means the customer adds the DNS records (and, for a
	// redirect, this instance serves it).
	BulkViaDNS = "dns"
	// BulkViaCloud means Warmbly Cloud serves the redirect.
	BulkViaCloud = "cloud"
)

// BulkDomainTracking is one domain's tracking host after a bulk setup.
type BulkDomainTracking struct {
	Host string `json:"host"`
	// Via is [BulkViaVendor] or [BulkViaDNS].
	Via         string `json:"via"`
	Verified    bool   `json:"verified"`
	Mailboxes   int    `json:"mailboxes"`
	CNAMETarget string `json:"cname_target,omitempty"`
	// Note says why the vendor did not write the record, when it was asked
	// and refused.
	Note string `json:"note,omitempty"`
	// Error and Code describe a refusal for this domain.
	Error string `json:"error,omitempty"`
	Code  string `json:"code,omitempty"`
}

// BulkDomainRedirect is one domain's root redirect after a bulk setup.
type BulkDomainRedirect struct {
	TargetURL string `json:"target_url,omitempty"`
	// Via is [BulkViaVendor], [BulkViaDNS] or [BulkViaCloud].
	Via      string `json:"via"`
	Verified bool   `json:"verified"`
	// Reviewed means the vendor's staff apply the forwarding later.
	Reviewed bool `json:"reviewed,omitempty"`
	// Error and Code describe a refusal for this domain.
	Error string `json:"error,omitempty"`
	Code  string `json:"code,omitempty"`
}

// BulkDomainResult is what one domain got from a bulk setup. A refusal on one
// domain is reported on its own row and never stops the others.
type BulkDomainResult struct {
	Domain   string              `json:"domain"`
	Tracking *BulkDomainTracking `json:"tracking,omitempty"`
	Redirect *BulkDomainRedirect `json:"redirect,omitempty"`
}

func sendingDomainPath(domain string, rest ...string) string {
	p := "emails/domains/" + url.PathEscape(domain)
	for _, r := range rest {
		p += "/" + r
	}
	return p
}

// ListDomains returns every domain the workspace sends from, with its
// authentication, tracking hosts, redirect and vendor. The list is not
// paginated.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) ListDomains(ctx context.Context, opts ...RequestOption) ([]SendingDomain, *Response, error) {
	return fetchData[SendingDomain](ctx, s.client, "emails/domains", opts)
}

// BulkSetupDomains sets one tracking subdomain and one root redirect for up to
// [MaxBulkDomains] domains, each through the vendor holding the domain when
// its API can and with DNS the customer adds otherwise. The call succeeds with
// one result per domain even when some were refused, so read each row. Each
// setting is set rather than added, so repeating the call is safe.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) BulkSetupDomains(ctx context.Context, params *BulkDomainSetupParams, opts ...RequestOption) ([]BulkDomainResult, *Response, error) {
	return sendData[BulkDomainResult](ctx, s.client.post, "emails/domains/bulk", params, opts)
}

// TrackingSuggestion returns the tracking host the domain's mailboxes can use
// and what is left to do for it. It answers 400 with
// [ErrCodeTrackingHostNotConfigured] when this instance has no tracking host.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) TrackingSuggestion(ctx context.Context, domain string, opts ...RequestOption) (*TrackingSuggestion, *Response, error) {
	return fetch[TrackingSuggestion](ctx, s.client, sendingDomainPath(domain, "tracking-suggestion"), opts)
}

// SetDomainTracking points every mailbox on the domain at one tracking host
// and returns the verification verdict. An empty host clears it. Repeating
// the call is safe.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) SetDomainTracking(ctx context.Context, domain, host string, opts ...RequestOption) (*DomainTrackingResult, *Response, error) {
	body := struct {
		Host string `json:"host"`
	}{Host: host}
	return send[DomainTrackingResult](ctx, s.client.put, sendingDomainPath(domain, "tracking"), body, opts)
}

// SetDomainRedirect records (or updates) the redirect from the bare domain to
// a website and checks it once. The domain keeps its verification token, so a
// TXT record already published stays valid and repeating the call is safe.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) SetDomainRedirect(ctx context.Context, domain string, params *DomainRedirectParams, opts ...RequestOption) (*DomainRedirect, *Response, error) {
	return send[DomainRedirect](ctx, s.client.put, sendingDomainPath(domain, "redirect"), params, opts)
}

// VerifyDomainRedirect checks the redirect's DNS now and returns it.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) VerifyDomainRedirect(ctx context.Context, domain string, opts ...RequestOption) (*DomainRedirect, *Response, error) {
	return send[DomainRedirect](ctx, s.client.post, sendingDomainPath(domain, "redirect", "verify"), nil, opts)
}

// VerifyRedirectWebsite returns the TXT record that proves targetURL's
// website belongs to the workspace, checked now. Repeating it is safe.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) VerifyRedirectWebsite(ctx context.Context, domain, targetURL string, opts ...RequestOption) (*RedirectWebsite, *Response, error) {
	body := struct {
		TargetURL string `json:"target_url"`
	}{TargetURL: targetURL}
	return send[RedirectWebsite](ctx, s.client.post, sendingDomainPath(domain, "redirect", "website"), body, opts)
}

// DeleteDomainRedirect removes the domain's redirect. The server answers 204.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) DeleteDomainRedirect(ctx context.Context, domain string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, sendingDomainPath(domain, "redirect"), opts...)
}

// SetDomainVendorForwarding has the inbox vendor holding the domain forward
// its root to targetURL and returns the vendor's view of the domain. An empty
// URL removes the forwarding where the vendor allows it
// ([VendorDomainLink.CanUnforward]). Repeating the call is safe.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) SetDomainVendorForwarding(ctx context.Context, domain, targetURL string, opts ...RequestOption) (*VendorDomainLink, *Response, error) {
	body := struct {
		URL string `json:"url"`
	}{URL: targetURL}
	return send[VendorDomainLink](ctx, s.client.put, sendingDomainPath(domain, "vendor-forwarding"), body, opts)
}

// SetDomainVendorTracking has the inbox vendor holding the domain write the
// tracking CNAME for host, then points every mailbox on the domain at it. The
// host is required. Repeating the call is safe: the record is upserted and
// the host is set rather than added.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) SetDomainVendorTracking(ctx context.Context, domain, host string, opts ...RequestOption) (*DomainTrackingResult, *Response, error) {
	body := struct {
		Host string `json:"host"`
	}{Host: host}
	return send[DomainTrackingResult](ctx, s.client.post, sendingDomainPath(domain, "vendor-tracking"), body, opts)
}
