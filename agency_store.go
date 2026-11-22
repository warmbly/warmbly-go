package warmbly

import (
	"context"
	"time"
)

// AgencyStoreAttentionKind names a problem in a client's purchases. The set
// is open; treat an unknown kind as needing attention.
type AgencyStoreAttentionKind string

const (
	// AgencyStoreConnectFailed is a purchased mailbox that could not be
	// connected.
	AgencyStoreConnectFailed AgencyStoreAttentionKind = "connect_failed"
	// AgencyStoreBillingFailed is a mailbox month not paid.
	AgencyStoreBillingFailed AgencyStoreAttentionKind = "billing_failed"
	// AgencyStoreRenewalFailed is a domain renewal not paid.
	AgencyStoreRenewalFailed AgencyStoreAttentionKind = "renewal_failed"
	// AgencyStoreAutoRenewOff is a domain with auto-renew turned off.
	AgencyStoreAutoRenewOff AgencyStoreAttentionKind = "auto_renew_off"
	// AgencyStoreOrderFailed is a failed order.
	AgencyStoreOrderFailed AgencyStoreAttentionKind = "order_failed"
)

// AgencyStoreAttention is one kind of problem in a client's purchases and how
// many.
type AgencyStoreAttention struct {
	Kind  AgencyStoreAttentionKind `json:"kind"`
	Count int                      `json:"count"`
}

// AgencyStoreNextRenewal is a client's soonest domain renewal.
type AgencyStoreNextRenewal struct {
	DomainID  string     `json:"domain_id"`
	Domain    string     `json:"domain"`
	ExpiresAt *time.Time `json:"expires_at"`
	AutoRenew bool       `json:"auto_renew"`
}

// AgencyStoreClient is one client's purchased infrastructure.
type AgencyStoreClient struct {
	OrganizationID     string  `json:"organization_id"`
	Name               string  `json:"name"`
	AvatarURL          *string `json:"avatar_url"`
	Domains            int     `json:"domains"`
	MailboxesLive      int     `json:"mailboxes_live"`
	MailboxesConnected int     `json:"mailboxes_connected"`
	// MonthlySpendMicros is what the client's live mailboxes cost a month.
	MonthlySpendMicros int64 `json:"monthly_spend_micros"`
	Renewals30d        int   `json:"renewals_30d"`
	// NextRenewal is nil when the client has no domain to renew.
	NextRenewal *AgencyStoreNextRenewal `json:"next_renewal"`
	OrdersOpen  int                     `json:"orders_open"`
	Attention   []AgencyStoreAttention  `json:"attention"`
}

// AgencyStoreRenewal is one upcoming domain renewal across the clients.
type AgencyStoreRenewal struct {
	OrganizationID   string     `json:"organization_id"`
	OrganizationName string     `json:"organization_name"`
	DomainID         string     `json:"domain_id"`
	Domain           string     `json:"domain"`
	ExpiresAt        *time.Time `json:"expires_at"`
	AutoRenew        bool       `json:"auto_renew"`
	RenewMicros      int64      `json:"renew_micros"`
}

// AgencyStoreTotals adds the clients up.
type AgencyStoreTotals struct {
	Domains            int   `json:"domains"`
	MailboxesLive      int   `json:"mailboxes_live"`
	MonthlySpendMicros int64 `json:"monthly_spend_micros"`
	Renewals30d        int   `json:"renewals_30d"`
	Attention          int   `json:"attention"`
}

// AgencyStore is the purchased domains and mailboxes across an agency's
// clients.
type AgencyStore struct {
	// Enabled is false when the instance has no store; the lists are then
	// empty.
	Enabled bool                `json:"enabled"`
	Clients []AgencyStoreClient `json:"clients"`
	// Renewals lists domain renewals in the next 30 days, soonest first.
	Renewals []AgencyStoreRenewal `json:"renewals"`
	Totals   AgencyStoreTotals    `json:"totals"`
}

// Store sums up the store's domains and mailboxes in each client workspace
// where the caller's own membership holds the manage-emails permission,
// problems first. To act on one client's store, use [StoreService] with
// [WithAgencyOrganization].
//
// Scope: [PermAgencyRead] and PermReadStore.
func (s *AgencyService) Store(ctx context.Context, agencyID string, opts ...RequestOption) (*AgencyStore, *Response, error) {
	return fetch[AgencyStore](ctx, s.client, agencyPath(agencyID, "store"), opts)
}
