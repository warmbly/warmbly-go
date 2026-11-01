package warmbly

import (
	"context"
	"net/url"
	"time"
)

// --- per-mailbox settings: tracking, sync folders, certificates, identity ---

// SetDirectTracking switches open and click tracking on or off for the
// hand-written unibox sends of one mailbox, and returns the stored value.
// Campaign mail is tracked regardless. It is off by default: a pixel belongs
// in a cold sequence more comfortably than in a one-to-one reply. The body is
// the desired state, so repeating the call is safe. Requires the write_emails
// scope.
func (s *EmailService) SetDirectTracking(ctx context.Context, id string, enabled bool, opts ...RequestOption) (bool, *Response, error) {
	body := struct {
		Enabled bool `json:"enabled"`
	}{Enabled: enabled}
	var out struct {
		TrackDirectMail bool `json:"track_direct_mail"`
	}
	resp, err := s.client.patch(ctx, "emails/"+url.PathEscape(id)+"/direct-tracking", body, &out, opts...)
	if err != nil {
		return false, resp, err
	}
	return out.TrackDirectMail, resp, nil
}

// UpdateSyncFolders replaces the folders the mailbox's sync leaves alone and
// returns the stored list. Each name also covers its subfolders; use the
// names from [MailboxSync.Folders]. The special folders (inbox, sent and so
// on) cannot be skipped. The list is the desired state, so repeating the call
// is safe. Requires the write_emails scope.
func (s *EmailService) UpdateSyncFolders(ctx context.Context, id string, skipFolders []string, opts ...RequestOption) ([]string, *Response, error) {
	if skipFolders == nil {
		skipFolders = []string{}
	}
	body := struct {
		SkipFolders []string `json:"skip_folders"`
	}{SkipFolders: skipFolders}
	var out struct {
		SkipFolders []string `json:"skip_folders"`
	}
	resp, err := s.client.put(ctx, "emails/"+url.PathEscape(id)+"/sync", body, &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	if out.SkipFolders == nil {
		out.SkipFolders = []string{}
	}
	return out.SkipFolders, resp, nil
}

// MailCertificate describes the certificate a mail server presented.
type MailCertificate struct {
	// Fingerprint is the SHA-256 of the certificate's public key, as 64
	// lowercase hex characters.
	Fingerprint string `json:"fingerprint"`
	Subject     string `json:"subject,omitempty"`
	Issuer      string `json:"issuer,omitempty"`
	// NotAfter is the expiry as an RFC 3339 timestamp.
	NotAfter   string `json:"not_after,omitempty"`
	SelfSigned bool   `json:"self_signed,omitempty"`
	// Trusted is set when the chain also passes certificate authority and host
	// name verification.
	Trusted bool `json:"trusted,omitempty"`
}

// MailboxServerCertificate is one server's certificate policy on a mailbox.
type MailboxServerCertificate struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	// TLSUnverified reports that the owner trusts this server on its pinned
	// certificate instead of a certificate authority.
	TLSUnverified bool   `json:"tls_unverified"`
	TLSPin        string `json:"tls_pin,omitempty"`
	// Observed is what the server presented on a check; nil on a plain read.
	Observed *MailCertificate `json:"observed,omitempty"`
}

// MailboxCertificates is an SMTP/IMAP mailbox's certificate policy per server.
type MailboxCertificates struct {
	SMTP MailboxServerCertificate `json:"smtp"`
	IMAP MailboxServerCertificate `json:"imap"`
}

// ConfirmCertificatesParams names the fingerprints the owner confirmed, one
// per server that is trusted on a pin. A pin is 64 hex characters; a
// "sha256:" prefix and colon, space or dash separators are accepted.
type ConfirmCertificatesParams struct {
	SMTPPin string `json:"smtp_pin"`
	IMAPPin string `json:"imap_pin"`
}

// Certificates reads an SMTP/IMAP mailbox's stored certificate policy per
// server. It does not contact the mail servers. Requires the read_emails
// scope.
func (s *EmailService) Certificates(ctx context.Context, id string, opts ...RequestOption) (*MailboxCertificates, *Response, error) {
	return fetch[MailboxCertificates](ctx, s.client, "emails/"+url.PathEscape(id)+"/certificates", opts)
}

// CheckCertificates connects to the mailbox's SMTP and IMAP servers and
// reports the certificate each presents now, in [MailboxServerCertificate.Observed].
// It changes nothing, so repeating it is safe. Pair it with
// [EmailService.ConfirmCertificates] to trust a self-signed server. It is
// session-only and needs [OrgPermManageEmails]; an OAuth mailbox is refused.
func (s *EmailService) CheckCertificates(ctx context.Context, id string, opts ...RequestOption) (*MailboxCertificates, *Response, error) {
	return send[MailboxCertificates](ctx, s.client.post, "emails/onboarding/smtp-imap/"+url.PathEscape(id)+"/certificates/check", nil, opts)
}

// ConfirmCertificates pins the fingerprints the owner confirmed and returns
// the stored policy. The body is the desired pins, so repeating the call
// converges on the same value. It is session-only and needs
// [OrgPermManageEmails].
func (s *EmailService) ConfirmCertificates(ctx context.Context, id string, params *ConfirmCertificatesParams, opts ...RequestOption) (*MailboxCertificates, *Response, error) {
	return send[MailboxCertificates](ctx, s.client.post, "emails/onboarding/smtp-imap/"+url.PathEscape(id)+"/certificates/confirm", params, opts)
}

// SendAsIdentity is one address the provider has verified a mailbox to send
// as, as the provider last reported it.
type SendAsIdentity struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	// IsPrimary is the mailbox's own address; IsDefault is the one the provider
	// composes from by default, which Warmbly reports but does not follow.
	IsPrimary bool `json:"is_primary"`
	IsDefault bool `json:"is_default"`
	// Verified reports that the provider finished verifying the address.
	// Sending as an unverified alias is refused, so those are not selectable.
	Verified bool `json:"verified"`
}

// Signature provenance values in [SendIdentity.SignatureSource].
const (
	// SignatureSourceManual means the signature was written in Warmbly.
	SignatureSourceManual = "manual"
	// SignatureSourceProvider means it was imported from the mailbox provider.
	SignatureSourceProvider = "provider"
)

// SendIdentity is a mailbox's sending identity: which addresses the provider
// lets it send as, which one is in use, and where the stored signature came
// from.
type SendIdentity struct {
	// Supported reports whether the provider can be asked at all. Only Gmail
	// exposes send-as identities; other mailboxes answer false with an empty
	// list rather than an error.
	Supported bool   `json:"supported"`
	Provider  string `json:"provider"`
	// MailboxEmail is the address the mailbox authenticates as, which is
	// always a legal sender and is what an empty SendAsEmail means.
	MailboxEmail string           `json:"mailbox_email"`
	SendAsEmail  string           `json:"send_as_email"`
	Identities   []SendAsIdentity `json:"identities"`
	SyncedAt     *time.Time       `json:"synced_at,omitempty"`
	// SignatureSource is [SignatureSourceManual] or [SignatureSourceProvider].
	SignatureSource     string     `json:"signature_source"`
	SignatureImportedAt *time.Time `json:"signature_imported_at,omitempty"`
}

// SendIdentity reports which addresses a mailbox may send as, which one it
// uses, and where its signature came from. It reads stored state only and does
// not call the provider. Pick an alias with [EmailService.Update] and
// [EmailUpdateParams.SendAsEmail]. Requires the read_emails scope.
func (s *EmailService) SendIdentity(ctx context.Context, id string, opts ...RequestOption) (*SendIdentity, *Response, error) {
	return fetch[SendIdentity](ctx, s.client, "emails/"+url.PathEscape(id)+"/identity", opts)
}

// RefreshSendIdentity re-reads the mailbox's send-as addresses from the
// provider and stores them. With importSignature the provider's signature is
// imported too, overwriting the stored one. It converges on the provider's
// current state, so repeating it is safe. Requires the write_emails scope.
func (s *EmailService) RefreshSendIdentity(ctx context.Context, id string, importSignature bool, opts ...RequestOption) (*SendIdentity, *Response, error) {
	var body any
	if importSignature {
		body = struct {
			ImportSignature bool `json:"import_signature"`
		}{ImportSignature: true}
	}
	return send[SendIdentity](ctx, s.client.post, "emails/"+url.PathEscape(id)+"/identity/refresh", body, opts)
}

// SwitchToAppPassword moves a Gmail mailbox off per-mailbox Google sign-in
// onto IMAP and SMTP with a Google app password, checking the password live
// before storing it, and returns the mailbox. A repeat is refused once the
// mailbox has switched. It is session-only and needs [OrgPermManageEmails].
func (s *EmailService) SwitchToAppPassword(ctx context.Context, id, appPassword string, opts ...RequestOption) (*Email, *Response, error) {
	body := struct {
		AppPassword string `json:"app_password"`
	}{AppPassword: appPassword}
	return send[Email](ctx, s.client.post, "emails/onboarding/app-password/"+url.PathEscape(id), body, opts)
}

// MailboxOAuthStartParams are the options of [EmailService.StartOAuthWithParams].
type MailboxOAuthStartParams struct {
	// Provider is [ProviderGmail] or [ProviderOutlook].
	Provider string `json:"provider"`
	// LoginHint preselects the mailbox in the provider's account picker.
	LoginHint string `json:"login_hint,omitempty"`
	// Return is "web" when a browser app starts the flow, so a sign-in window
	// without an opener returns to it. Leave it empty otherwise.
	Return string `json:"return,omitempty"`
}

// StartOAuthWithParams is [EmailService.StartOAuth] with a login hint and the
// return mode. A Microsoft start also fills [OAuthStartResult.AdminConsentURL],
// which lets an administrator approve sign-in for the whole organization. It
// is session-only.
func (s *EmailService) StartOAuthWithParams(ctx context.Context, params *MailboxOAuthStartParams, opts ...RequestOption) (*OAuthStartResult, *Response, error) {
	return send[OAuthStartResult](ctx, s.client.post, "emails/onboarding/oauth/start", params, opts)
}
