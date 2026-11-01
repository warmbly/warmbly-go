package warmbly

import (
	"context"
	"net/url"
	"time"
)

// --- inbox vendors ---
//
// An inbox vendor (InboxKit, Zapmail and similar) provisions mailboxes for
// you. Connecting a vendor account stores its API key, sealed, and lets the
// workspace import the mailboxes it holds without typing their passwords.
// Every route here is session-only and needs the manage-emails organization
// permission ([OrgPermManageEmails]). Instances without the feature answer
// 404.

// Error codes carried in [Error.Code] by vendor operations.
const (
	// ErrCodeMailboxVendorUnknown means the vendor id is not in the catalog.
	ErrCodeMailboxVendorUnknown = "mailbox_vendor_unknown"
	// ErrCodeMailboxVendorInvalidFields means a required credential field is
	// missing or malformed.
	ErrCodeMailboxVendorInvalidFields = "mailbox_vendor_invalid_fields"
	// ErrCodeMailboxVendorUnauthorized means the vendor refused the API key.
	ErrCodeMailboxVendorUnauthorized = "mailbox_vendor_unauthorized"
	// ErrCodeMailboxVendorRateLimited means the vendor is rate limiting the
	// account; retry later.
	ErrCodeMailboxVendorRateLimited = "mailbox_vendor_rate_limited"
	// ErrCodeMailboxVendorUnavailable means the vendor did not answer.
	ErrCodeMailboxVendorUnavailable = "mailbox_vendor_unavailable"
)

// VendorField is one credential a vendor asks for when connecting.
type VendorField struct {
	// Key is the name to send in [VendorCreateParams.Fields].
	Key      string `json:"key"`
	Label    string `json:"label"`
	Secret   bool   `json:"secret"`
	Required bool   `json:"required"`
	Help     string `json:"help,omitempty"`
}

// Vendor is one supported inbox vendor and what connecting it asks for.
type Vendor struct {
	ID         string        `json:"id"`
	Label      string        `json:"label"`
	Website    string        `json:"website"`
	KeyHelpURL string        `json:"key_help_url"`
	Fields     []VendorField `json:"fields"`
}

// VendorConnection is an inbox vendor account the workspace imports from. Its
// credentials are never returned.
type VendorConnection struct {
	ID     string `json:"id"`
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

// VendorCreateParams connects a vendor account, or with
// [EmailService.UpdateVendor] renames it and replaces its key.
type VendorCreateParams struct {
	// Vendor is a [Vendor.ID] from the catalog. It is ignored on an update.
	Vendor string `json:"vendor"`
	// Label names the account in the dashboard; empty uses the vendor's name.
	Label string `json:"label"`
	// Fields holds the credentials by [VendorField.Key].
	Fields map[string]string `json:"fields"`
}

// VendorImportParams picks the vendor mailboxes to import.
type VendorImportParams struct {
	// MailboxIDs are [VendorMailbox.ID] values.
	MailboxIDs []string `json:"mailbox_ids"`
	// All imports every mailbox not connected yet.
	All bool `json:"all"`
	// Options carries the import settings; the zero value uses the defaults.
	Options MailboxImportOptions `json:"options"`
}

// VendorCatalog lists the inbox vendors the instance supports and the
// credentials each asks for. It is session-only and needs
// [OrgPermManageEmails].
func (s *EmailService) VendorCatalog(ctx context.Context, opts ...RequestOption) ([]Vendor, *Response, error) {
	return fetchData[Vendor](ctx, s.client, "emails/vendors/catalog", opts)
}

// ListVendors returns the workspace's connected vendor accounts, never their
// keys. The list is not paginated. It is session-only and needs
// [OrgPermManageEmails].
func (s *EmailService) ListVendors(ctx context.Context, opts ...RequestOption) ([]VendorConnection, *Response, error) {
	return fetchData[VendorConnection](ctx, s.client, "emails/vendors", opts)
}

// CreateVendor verifies the key with the vendor and stores it sealed,
// answering 201. It is session-only and needs [OrgPermManageEmails].
func (s *EmailService) CreateVendor(ctx context.Context, params *VendorCreateParams, opts ...RequestOption) (*VendorConnection, *Response, error) {
	return send[VendorConnection](ctx, s.client.post, "emails/vendors", params, opts)
}

// UpdateVendor renames a vendor account or, when Fields is set, replaces its
// key after verifying it with the vendor. Vendor in the params is ignored. It
// is session-only and needs [OrgPermManageEmails].
func (s *EmailService) UpdateVendor(ctx context.Context, id string, params *VendorCreateParams, opts ...RequestOption) (*VendorConnection, *Response, error) {
	return send[VendorConnection](ctx, s.client.patch, "emails/vendors/"+url.PathEscape(id), params, opts)
}

// DeleteVendor removes a vendor account. Mailboxes it brought in stay
// connected. The server answers 204. It is session-only and needs
// [OrgPermManageEmails].
func (s *EmailService) DeleteVendor(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "emails/vendors/"+url.PathEscape(id), opts...)
}

// VendorMailboxes lists the mailboxes held in a vendor account, with whether
// each is already connected. The list is not paginated. It is session-only and
// needs [OrgPermManageEmails].
func (s *EmailService) VendorMailboxes(ctx context.Context, id string, opts ...RequestOption) ([]VendorMailbox, *Response, error) {
	return fetchData[VendorMailbox](ctx, s.client, "emails/vendors/"+url.PathEscape(id)+"/mailboxes", opts)
}

// ImportVendorMailboxes starts a background import of the picked vendor
// mailboxes and returns it, answering 201. The credentials are read from the
// vendor as each row is worked, never earlier. Follow it with
// [EmailService.GetImport]. Repeating the call updates what the first
// connected. It is session-only and needs [OrgPermManageEmails].
func (s *EmailService) ImportVendorMailboxes(ctx context.Context, id string, params *VendorImportParams, opts ...RequestOption) (*MailboxImport, *Response, error) {
	return send[MailboxImport](ctx, s.client.post, "emails/vendors/"+url.PathEscape(id)+"/import", params, opts)
}
