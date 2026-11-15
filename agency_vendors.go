package warmbly

import (
	"context"
	"net/url"
	"time"
)

// AgencyVendorAccount is an inbox vendor account the agency holds once and
// uses in any of its clients. Its API key is never returned.
type AgencyVendorAccount struct {
	ID string `json:"id"`
	// Vendor is an [AgencyVendor.ID].
	Vendor    string    `json:"vendor"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"created_at"`
	// Status is "active", or "invalid" when the vendor refused the key in
	// any client that uses it.
	Status    string `json:"status"`
	LastError string `json:"last_error,omitempty"`
	// Mailboxes counts the mailboxes connected from it across every client.
	Mailboxes int                         `json:"mailboxes"`
	Clients   []AgencyVendorAccountClient `json:"clients"`
}

// AgencyVendorAccountClient is one client workspace an agency vendor account
// is used in.
type AgencyVendorAccountClient struct {
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	// ConnectionID is the client workspace's own connection to the account.
	ConnectionID string `json:"connection_id"`
	Mailboxes    int    `json:"mailboxes"`
	// MailboxesHidden is a client whose mailbox list the caller cannot read,
	// so Mailboxes is left out.
	MailboxesHidden bool   `json:"mailboxes_hidden,omitempty"`
	Status          string `json:"status"`
	LastError       string `json:"last_error,omitempty"`
}

// AgencyVendor is one supported inbox vendor and what connecting it asks
// for.
type AgencyVendor struct {
	ID         string              `json:"id"`
	Label      string              `json:"label"`
	Website    string              `json:"website"`
	KeyHelpURL string              `json:"key_help_url"`
	Fields     []AgencyVendorField `json:"fields"`
	// SignsIn is a vendor that can sign in again as its Google mailboxes, so
	// they reconnect in place.
	SignsIn bool `json:"signs_in"`
}

// AgencyVendorField is one credential input a vendor needs.
type AgencyVendorField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Secret   bool   `json:"secret"`
	Required bool   `json:"required"`
	Help     string `json:"help,omitempty"`
}

// AgencyVendorList is the agency's vendor accounts and the vendor catalog.
type AgencyVendorList struct {
	Data    []AgencyVendorAccount `json:"data"`
	Catalog []AgencyVendor        `json:"catalog"`
}

// AgencyVendorParams creates or updates an agency vendor account.
type AgencyVendorParams struct {
	// Vendor is an [AgencyVendor.ID]; required on create, ignored on update.
	Vendor string `json:"vendor,omitempty"`
	// Label names the account; empty takes the vendor's name on create and
	// keeps the current one on update.
	Label string `json:"label,omitempty"`
	// Fields are the credentials, keyed by [AgencyVendorField.Key]. They
	// are verified with the vendor before being stored. On update, empty
	// keeps the current key.
	Fields map[string]string `json:"fields,omitempty"`
}

// AgencyVendorConnection is a client workspace's connection to an agency
// vendor account.
type AgencyVendorConnection struct {
	ID         string     `json:"id"`
	Vendor     string     `json:"vendor"`
	Label      string     `json:"label"`
	Status     string     `json:"status"`
	LastError  string     `json:"last_error,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	Mailboxes  int        `json:"mailboxes"`
	// AgencyConnectionID is the agency vendor account the connection was
	// made from.
	AgencyConnectionID *string `json:"agency_connection_id,omitempty"`
}

// Vendors returns the agency's inbox vendor accounts, with the clients each
// is used in, and the catalog of supported vendors.
//
// Session only.
func (s *AgencyService) Vendors(ctx context.Context, agencyID string, opts ...RequestOption) (*AgencyVendorList, *Response, error) {
	out, resp, err := fetch[AgencyVendorList](ctx, s.client, agencyPath(agencyID, "vendors"), opts)
	if err == nil {
		if out.Data == nil {
			out.Data = []AgencyVendorAccount{}
		}
		if out.Catalog == nil {
			out.Catalog = []AgencyVendor{}
		}
	}
	return out, resp, err
}

// CreateVendor verifies an API key with the vendor and holds it for every
// client. Pass [WithIdempotencyKey] to make a retry safe.
//
// Session only; needs a recent sign-in.
func (s *AgencyService) CreateVendor(ctx context.Context, agencyID string, params *AgencyVendorParams, opts ...RequestOption) (*AgencyVendorAccount, *Response, error) {
	return send[AgencyVendorAccount](ctx, s.client.post, agencyPath(agencyID, "vendors"), params, opts)
}

// UpdateVendor renames an account or, when params.Fields is set, replaces
// its key in every client that uses it.
//
// Session only; needs a recent sign-in.
func (s *AgencyService) UpdateVendor(ctx context.Context, agencyID, id string, params *AgencyVendorParams, opts ...RequestOption) (*AgencyVendorAccount, *Response, error) {
	return send[AgencyVendorAccount](ctx, s.client.patch, agencyPath(agencyID, "vendors", url.PathEscape(id)), params, opts)
}

// DeleteVendor forgets the account's key in the agency and in every client.
// Mailboxes it brought in stay, without automatic reconnects.
//
// Session only; needs a recent sign-in.
func (s *AgencyService) DeleteVendor(ctx context.Context, agencyID, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, agencyPath(agencyID, "vendors", url.PathEscape(id)), opts...)
}

// UseVendor gives a client workspace its connection to an agency vendor
// account, making it the first time and returning the existing one after.
// The caller must manage mailboxes in that workspace.
//
// Session only.
func (s *AgencyService) UseVendor(ctx context.Context, agencyID, id, organizationID string, opts ...RequestOption) (*AgencyVendorConnection, *Response, error) {
	return send[AgencyVendorConnection](ctx, s.client.put, agencyPath(agencyID, "vendors", url.PathEscape(id), "clients", url.PathEscape(organizationID)), nil, opts)
}
