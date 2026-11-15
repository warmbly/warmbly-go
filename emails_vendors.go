package warmbly

import (
	"context"
	"net/url"
	"time"
)

// --- inbox vendors ---
//
// An inbox vendor provisions mailboxes for you. Connecting a vendor account
// stores its API key, sealed, and lets the workspace import the mailboxes it
// holds without typing their passwords; a vendor whose
// [MailboxVendor.SignsIn] is set can also sign in again as its Google
// mailboxes so they reconnect in place.
//
// Every route here is session only (an API key is refused) and needs the
// manage-emails permission ([OrgPermManageEmails]). Instances without the
// feature answer 404. A connection made from an agency's vendor account
// answers only that agency's staff ([ErrCodeAgencyVendorStaffOnly]) and has
// its key changed at the agency ([ErrCodeAgencyVendorManaged]).

// Error codes carried in [Error.Code] by inbox vendor calls.
const (
	// ErrCodeMailboxVendorUnknown refuses a vendor that is not in the
	// catalog.
	ErrCodeMailboxVendorUnknown = "mailbox_vendor_unknown"
	// ErrCodeMailboxVendorInvalidFields refuses a connection missing a
	// required [MailboxVendorField].
	ErrCodeMailboxVendorInvalidFields = "mailbox_vendor_invalid_fields"
	// ErrCodeMailboxVendorUnauthorized: the vendor refused the API key.
	ErrCodeMailboxVendorUnauthorized = "mailbox_vendor_unauthorized"
	// ErrCodeMailboxVendorNoWorkspace: the vendor shows no workspace for the
	// key.
	ErrCodeMailboxVendorNoWorkspace = "mailbox_vendor_no_workspace"
	// ErrCodeMailboxVendorRateLimited is the 429 passed on when the vendor
	// rate limits requests.
	ErrCodeMailboxVendorRateLimited = "mailbox_vendor_rate_limited"
	// ErrCodeMailboxVendorUnavailable: the vendor did not answer as expected,
	// or no longer has the mailbox.
	ErrCodeMailboxVendorUnavailable = "mailbox_vendor_unavailable"
	// ErrCodeMailboxVendorReconnectUnavailable: this instance has no Google
	// sign-in, so a vendor cannot sign in to mailboxes for it.
	ErrCodeMailboxVendorReconnectUnavailable = "mailbox_vendor_reconnect_unavailable"
	// ErrCodeMailboxVendorReconnectUnsupported: the vendor cannot sign in to
	// mailboxes ([MailboxVendor.SignsIn] is false).
	ErrCodeMailboxVendorReconnectUnsupported = "mailbox_vendor_reconnect_unsupported"
	// ErrCodeMailboxVendorReconnectNone: no Google mailbox from the
	// connection matches (404), or more than [MaxVendorReconnectMailboxes]
	// were named (400).
	ErrCodeMailboxVendorReconnectNone = "mailbox_vendor_reconnect_none"
)

// MaxVendorReconnectMailboxes is the most mailboxes one
// [EmailService.ReconnectVendorMailboxes] call reconnects.
const MaxVendorReconnectMailboxes = 1000

// MailboxVendorField is one credential a vendor asks for when connecting.
type MailboxVendorField struct {
	// Key is the name to send in [VendorConnectionParams.Fields].
	Key      string `json:"key"`
	Label    string `json:"label"`
	Secret   bool   `json:"secret"`
	Required bool   `json:"required"`
	Help     string `json:"help,omitempty"`
}

// MailboxVendor is one supported inbox vendor and what connecting it asks
// for.
type MailboxVendor struct {
	ID         string               `json:"id"`
	Label      string               `json:"label"`
	Website    string               `json:"website"`
	KeyHelpURL string               `json:"key_help_url"`
	Fields     []MailboxVendorField `json:"fields"`
	// SignsIn is a vendor that can sign in again as its Google mailboxes, so
	// they reconnect in place ([EmailService.ReconnectVendorMailboxes]).
	SignsIn bool `json:"signs_in"`
}

// VendorConnection is an inbox vendor account the workspace imports from. Its
// credentials are never returned.
type VendorConnection struct {
	ID string `json:"id"`
	// Vendor is a [MailboxVendor.ID].
	Vendor string `json:"vendor"`
	Label  string `json:"label"`
	// Status is "active" while the key works and "invalid" once the vendor
	// refused it; LastError says why. The set is open.
	Status     string     `json:"status"`
	LastError  string     `json:"last_error,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	// Mailboxes counts the mailboxes imported through the account.
	Mailboxes int `json:"mailboxes"`
	// AgencyConnectionID is the agency vendor account the connection was
	// made from, when it was.
	AgencyConnectionID *string `json:"agency_connection_id,omitempty"`
}

// VendorMailbox is one mailbox in a vendor account, as the picker shows it.
type VendorMailbox struct {
	// ID is the vendor's own id. Pass it in [VendorImportParams.MailboxIDs].
	ID       string `json:"id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Domain   string `json:"domain"`
	Provider string `json:"provider"`
	Status   string `json:"status"`
	// Workspace names the vendor workspace holding the mailbox, when it has
	// several.
	Workspace string `json:"workspace,omitempty"`
	Connected bool   `json:"connected"`
	// EmailAccountID is the connected mailbox, when Connected.
	EmailAccountID *string `json:"email_account_id,omitempty"`
}

// VendorConnectionParams connects a vendor account, or with
// [EmailService.UpdateVendor] renames it or replaces its key.
type VendorConnectionParams struct {
	// Vendor is a [MailboxVendor.ID]; required on create, ignored on update.
	Vendor string `json:"vendor,omitempty"`
	// Label names the account; empty takes the vendor's name on create and
	// keeps the current one on update.
	Label string `json:"label,omitempty"`
	// Fields holds the credentials by [MailboxVendorField.Key]. They are
	// verified with the vendor before being stored. On update, empty keeps
	// the current key.
	Fields map[string]string `json:"fields,omitempty"`
}

// VendorImportParams picks the vendor mailboxes to import.
type VendorImportParams struct {
	// MailboxIDs are [VendorMailbox.ID] values.
	MailboxIDs []string `json:"mailbox_ids,omitempty"`
	// All imports every mailbox not connected yet.
	All bool `json:"all,omitempty"`
	// Options carries the import settings; the zero value uses the defaults.
	Options MailboxImportOptions `json:"options"`
}

// VendorReconnectParams names the mailboxes a vendor reconnect covers. Set
// EmailAccountID or EmailAccountIDs, not both; with neither, every Google
// mailbox imported from the connection is reconnected.
type VendorReconnectParams struct {
	EmailAccountID  *string  `json:"email_account_id,omitempty"`
	EmailAccountIDs []string `json:"email_account_ids,omitempty"`
}

// Per-mailbox states in [VendorReconnectMailbox.Status]. The set is open.
const (
	VendorReconnectQueued    = "queued"
	VendorReconnectTrusting  = "trusting"
	VendorReconnectSigningIn = "signing_in"
	VendorReconnectConnected = "connected"
	VendorReconnectFailed    = "failed"
)

// VendorReconnectMailbox is where one mailbox's vendor sign-in stands.
type VendorReconnectMailbox struct {
	EmailAccountID string `json:"email_account_id"`
	Email          string `json:"email"`
	// Status is one of the VendorReconnect* constants; connected and failed
	// are final.
	Status string `json:"status"`
	// Message says why a mailbox failed.
	Message string `json:"message,omitempty"`
}

func vendorPath(id string, rest ...string) string {
	p := "emails/vendors/" + url.PathEscape(id)
	for _, r := range rest {
		p += "/" + r
	}
	return p
}

// VendorCatalog lists the inbox vendors the instance supports and the
// credentials each asks for.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) VendorCatalog(ctx context.Context, opts ...RequestOption) ([]MailboxVendor, *Response, error) {
	return fetchData[MailboxVendor](ctx, s.client, "emails/vendors/catalog", opts)
}

// ListVendors returns the workspace's connected vendor accounts, never their
// keys. The list is not paginated.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) ListVendors(ctx context.Context, opts ...RequestOption) ([]VendorConnection, *Response, error) {
	return fetchData[VendorConnection](ctx, s.client, "emails/vendors", opts)
}

// CreateVendor verifies the key with the vendor and stores it sealed,
// answering 201.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) CreateVendor(ctx context.Context, params *VendorConnectionParams, opts ...RequestOption) (*VendorConnection, *Response, error) {
	return send[VendorConnection](ctx, s.client.post, "emails/vendors", params, opts)
}

// UpdateVendor renames a vendor account or, when params.Fields is set,
// replaces its key after verifying it with the vendor.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) UpdateVendor(ctx context.Context, id string, params *VendorConnectionParams, opts ...RequestOption) (*VendorConnection, *Response, error) {
	if params == nil {
		params = &VendorConnectionParams{}
	}
	return send[VendorConnection](ctx, s.client.patch, vendorPath(id), params, opts)
}

// DeleteVendor removes a vendor account. Mailboxes it brought in stay
// connected. The server answers 204.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) DeleteVendor(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, vendorPath(id), opts...)
}

// VendorMailboxes lists the mailboxes held in a vendor account, with whether
// each is already connected. The list is not paginated.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) VendorMailboxes(ctx context.Context, id string, opts ...RequestOption) ([]VendorMailbox, *Response, error) {
	return fetchData[VendorMailbox](ctx, s.client, vendorPath(id, "mailboxes"), opts)
}

// ImportVendorMailboxes starts a background import of the picked vendor
// mailboxes and returns it, answering 201. The credentials are read from the
// vendor as each row is worked. Follow it with [EmailService.GetImport].
// Repeating the call updates what the first connected.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) ImportVendorMailboxes(ctx context.Context, id string, params *VendorImportParams, opts ...RequestOption) (*MailboxImport, *Response, error) {
	if params == nil {
		params = &VendorImportParams{}
	}
	return send[MailboxImport](ctx, s.client.post, vendorPath(id, "import"), params, opts)
}

// ReconnectVendorMailboxes has the vendor sign in again as the connection's
// Google mailboxes, renewing each in place. The server answers 202 at once
// with each mailbox's starting state; follow it with
// [EmailService.VendorReconnectStatus]. params may be nil to reconnect every
// Google mailbox from the connection.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) ReconnectVendorMailboxes(ctx context.Context, id string, params *VendorReconnectParams, opts ...RequestOption) ([]VendorReconnectMailbox, *Response, error) {
	if params == nil {
		params = &VendorReconnectParams{}
	}
	return sendData[VendorReconnectMailbox](ctx, s.client.post, vendorPath(id, "reconnect"), params, opts)
}

// VendorReconnectStatus reports where the connection's last reconnect
// stands, mailbox by mailbox. It is empty when none ran recently.
//
// Session only; needs the manage-emails permission.
func (s *EmailService) VendorReconnectStatus(ctx context.Context, id string, opts ...RequestOption) ([]VendorReconnectMailbox, *Response, error) {
	return fetchData[VendorReconnectMailbox](ctx, s.client, vendorPath(id, "reconnect"), opts)
}
