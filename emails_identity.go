package warmbly

import (
	"context"
	"net/url"
	"time"
)

// Signature provenance values returned in [SendIdentity.SignatureSource]. They
// say where the stored signature came from, which is unrelated to
// [Email.SignatureSync].
const (
	// SignatureSourceManual is a signature written in Warmbly.
	SignatureSourceManual = "manual"
	// SignatureSourceProvider is a signature imported from the mailbox
	// provider by [EmailService.RefreshSendIdentity].
	SignatureSourceProvider = "provider"
)

// Error codes carried in [Error.Code] by the sending-identity and folder
// settings of a mailbox.
const (
	// ErrCodeSendAsUnsupported is the 400 for a send-as change or refresh on
	// a provider that exposes no send-as addresses (anything but Gmail).
	ErrCodeSendAsUnsupported = "mailbox_send_as_unsupported"
	// ErrCodeSendAsUnknown is the 400 for a send-as address the provider has
	// not verified for the mailbox.
	ErrCodeSendAsUnknown = "mailbox_send_as_unknown"
	// ErrCodeInvalidSyncFolder is the 400 for a skip list on a mailbox that
	// is not IMAP, or one naming a folder the sync always follows.
	ErrCodeInvalidSyncFolder = "invalid_sync_folder"
)

// SendAsIdentity is one address the provider has verified the mailbox to send
// as, as the provider last reported it.
type SendAsIdentity struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	// IsPrimary marks the mailbox's own address.
	IsPrimary bool `json:"is_primary"`
	// IsDefault marks the address the provider composes from by default.
	// Warmbly reports it but does not follow it: the address Warmbly sends
	// from is [Email.SendAsEmail].
	IsDefault bool `json:"is_default"`
	// Verified reports that the provider finished verifying the address. An
	// unverified alias is listed but cannot be chosen.
	Verified bool `json:"verified"`
}

// SendIdentity is a mailbox's sending identity: which addresses the provider
// lets it send as, which one is in use, and where its stored signature came
// from.
type SendIdentity struct {
	// Supported reports whether the provider can be asked at all. Only Gmail
	// exposes send-as addresses and a stored signature; any other mailbox
	// answers with Supported false and an empty list rather than an error.
	Supported bool `json:"supported"`
	// Provider is the mailbox backend, as [Email.Provider].
	Provider string `json:"provider"`
	// MailboxEmail is the address the mailbox authenticates as, which is
	// always a legal sender and is what an empty SendAsEmail means.
	MailboxEmail string `json:"mailbox_email"`
	// SendAsEmail is the chosen alias, or "" for MailboxEmail.
	SendAsEmail string `json:"send_as_email"`
	// Identities is the cached list from the last refresh; never null.
	Identities []SendAsIdentity `json:"identities"`
	// SyncedAt is when the list was last read from the provider; nil until
	// the first refresh.
	SyncedAt *time.Time `json:"synced_at,omitempty"`

	// SignatureSource is [SignatureSourceManual] or
	// [SignatureSourceProvider].
	SignatureSource     string     `json:"signature_source"`
	SignatureImportedAt *time.Time `json:"signature_imported_at,omitempty"`
}

// SendIdentity reports which addresses the mailbox may send as, which one it
// uses, and where its signature came from. It reads stored state only and
// never calls the provider, so it is cheap to call on every render. Choose an
// alias with [EmailUpdateParams.SendAsEmail].
//
// Requires the [PermReadEmails] API permission.
func (s *EmailService) SendIdentity(ctx context.Context, id string, opts ...RequestOption) (*SendIdentity, *Response, error) {
	return fetch[SendIdentity](ctx, s.client, "emails/"+url.PathEscape(id)+"/identity", opts)
}

// RefreshSendIdentity re-reads the mailbox's send-as addresses from the
// provider, through the worker that holds the mailbox, stores them and
// returns the updated identity. A chosen alias the provider no longer lists is
// cleared in the same step. With importSignature the provider's signature for
// the chosen address, when it holds one, replaces the stored one and
// [SendIdentity.SignatureSource] becomes [SignatureSourceProvider]; without
// it the stored signature is left alone.
//
// A mailbox on a provider other than Gmail fails with
// [ErrCodeSendAsUnsupported]. The call reads the provider's current state and
// writes exactly that, so repeating it converges and no idempotency key is
// needed.
//
// Requires the [PermWriteEmails] API permission.
func (s *EmailService) RefreshSendIdentity(ctx context.Context, id string, importSignature bool, opts ...RequestOption) (*SendIdentity, *Response, error) {
	var body any
	if importSignature {
		body = struct {
			ImportSignature bool `json:"import_signature"`
		}{ImportSignature: true}
	}
	return send[SendIdentity](ctx, s.client.post, "emails/"+url.PathEscape(id)+"/identity/refresh", body, opts)
}

// SetDirectTracking switches open and click tracking on or off for the
// mailbox's hand-written unibox sends (replies and compose), and returns the
// stored value. It is off by default and set per mailbox, since a pixel suits
// a cold sequence better than a one-to-one reply. Campaign mail is tracked
// either way. The body is the desired state, so a retry converges.
//
// Requires the [PermWriteEmails] API permission.
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

// UpdateSyncFolders replaces the IMAP folders the mailbox's sync leaves alone
// and returns the stored, normalized list. Each name also covers its
// subfolders; pick names from [MailboxSync.Folders]. Mail already imported
// from a newly skipped folder is removed from the unibox. Pass an empty slice
// to follow every folder again.
//
// Only an SMTP/IMAP mailbox has folders to skip; any other mailbox, or a list
// naming one of the special folders the sync always follows, fails with
// [ErrCodeInvalidSyncFolder]. The body is the desired list,
// so a retry converges and no idempotency key is needed.
//
// Requires the [PermWriteEmails] API permission.
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
