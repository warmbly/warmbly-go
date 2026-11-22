package warmbly

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// StoreService is the domain and mailbox store: domains bought through the
// instance's GraphKit account, Google Workspace and Microsoft 365 mailboxes
// on them, domains registered elsewhere and connected here, and the DNS
// records of every store domain.
//
// Every route accepts a session, an API key, an OAuth token, or an agency key
// acting in a client workspace (see [WithAgencyOrganization]). Each needs a
// current workspace, and the member behind the call needs the manage-emails
// permission ([OrgPermManageEmails]). A key or token needs PermReadStore to
// read and PermManageStore to change anything. Anything that charges, or
// changes what is charged, also needs the member to hold manage-billing
// ([OrgPermManageBilling]), checked on every request; a session additionally
// needs a recent sign-in (see [AuthService.Reauth]) for those, which API keys
// and OAuth tokens skip.
//
// Money is integer micro-USD (1000000 is $1.00) in every field ending in
// Micros. Every write accepts [WithIdempotencyKey]; always send one with an
// order, a renewal, a cancellation or a resume, so a retry never charges or
// applies twice. [StoreService.TransferOut] and [StoreService.SignIn] are the
// exceptions: they answer with a secret shown once, so the server keeps no
// copy and ignores a key.
//
// When the instance has no GraphKit API key, [StoreService.Status] answers
// Enabled false with [StoreReasonNotConfigured], the list reads still answer
// from what was bought here, and every other call fails with 404
// "store_disabled". In a workspace a partner manages, every write fails with
// 403 "store_managed_by_partner".
type StoreService service

// StoreDisabledReason says why [StoreStatus.Enabled] is false. The set is
// open.
type StoreDisabledReason string

const (
	// StoreReasonNotConfigured means the instance has no GraphKit API key.
	StoreReasonNotConfigured StoreDisabledReason = "not_configured"
	// StoreReasonPartnerManaged means a partner buys for this workspace.
	StoreReasonPartnerManaged StoreDisabledReason = "partner_managed"
)

// StoreDomainMarkup says whether the instance adds to domain prices. The set
// is open.
type StoreDomainMarkup string

const (
	// StoreDomainAtCost means domains are sold at the wholesale price.
	StoreDomainAtCost StoreDomainMarkup = "at_cost"
	// StoreDomainMarkedUp means the instance adds a markup.
	StoreDomainMarkedUp StoreDomainMarkup = "markup"
)

// StoreProvider is a mailbox provider. The set is open.
type StoreProvider string

const (
	StoreProviderGoogle    StoreProvider = "google"
	StoreProviderMicrosoft StoreProvider = "microsoft"
)

// StoreItemType is the kind of a quote item or line. The set is open.
type StoreItemType string

const (
	// StoreItemDomain registers a domain.
	StoreItemDomain StoreItemType = "domain"
	// StoreItemMailboxes adds mailboxes on a domain.
	StoreItemMailboxes StoreItemType = "mailboxes"
)

// StoreOfferReason says why a search result cannot be bought. The set is
// open.
type StoreOfferReason string

const (
	StoreOfferRegistered     StoreOfferReason = "registered"
	StoreOfferPremium        StoreOfferReason = "premium"
	StoreOfferReserved       StoreOfferReason = "reserved"
	StoreOfferTLDUnsupported StoreOfferReason = "tld_unsupported"
)

// StoreOrderStatus is a [StoreOrder]'s status. The set is open.
type StoreOrderStatus string

const (
	// StoreOrderCharging is before fulfillment accepted the order. It can
	// last about two minutes; the order then moves on or is refunded.
	StoreOrderCharging        StoreOrderStatus = "charging"
	StoreOrderProcessing      StoreOrderStatus = "processing"
	StoreOrderCompleted       StoreOrderStatus = "completed"
	StoreOrderPartiallyFailed StoreOrderStatus = "partially_failed"
	StoreOrderFailed          StoreOrderStatus = "failed"
)

// StoreStepKey names one stage of an order's progress, in this order. The
// set is open.
type StoreStepKey string

const (
	// StoreStepRegister is skipped on a domain the workspace connected.
	StoreStepRegister StoreStepKey = "register"
	StoreStepDNS      StoreStepKey = "dns"
	StoreStepMailAuth StoreStepKey = "mail_auth"
	// StoreStepMailboxes can stay running from a few hours to a couple of
	// days while Google or Microsoft creates the mailboxes.
	StoreStepMailboxes  StoreStepKey = "mailboxes"
	StoreStepDelegation StoreStepKey = "delegation"
	StoreStepReady      StoreStepKey = "ready"
)

// StoreStepStatus is one order step's state. The set is open.
type StoreStepStatus string

const (
	StoreStepPending StoreStepStatus = "pending"
	StoreStepRunning StoreStepStatus = "running"
	StoreStepDone    StoreStepStatus = "done"
	StoreStepFailed  StoreStepStatus = "failed"
	StoreStepSkipped StoreStepStatus = "skipped"
)

// StoreDomainStatus is a [StoreDomain]'s status. The set is open; treat an
// unknown value as not ready.
type StoreDomainStatus string

const (
	StoreDomainRegistering     StoreDomainStatus = "registering"
	StoreDomainActive          StoreDomainStatus = "active"
	StoreDomainExpired         StoreDomainStatus = "expired"
	StoreDomainRedemption      StoreDomainStatus = "redemption"
	StoreDomainTransferringOut StoreDomainStatus = "transferring_out"
	StoreDomainTransferredOut  StoreDomainStatus = "transferred_out"
	StoreDomainFailed          StoreDomainStatus = "failed"
	// StoreDomainConnecting is a connected domain not connected yet.
	StoreDomainConnecting StoreDomainStatus = "connecting"
	StoreDomainRemoved    StoreDomainStatus = "removed"
)

// StoreDomainSource says where a store domain came from. The set is open.
type StoreDomainSource string

const (
	// StoreSourcePurchased is a domain bought in the store.
	StoreSourcePurchased StoreDomainSource = "purchased"
	// StoreSourceConnected is a domain registered elsewhere and connected.
	StoreSourceConnected StoreDomainSource = "connected"
)

// StoreConnectionMethod is how a domain registered elsewhere is connected.
// The set is open.
type StoreConnectionMethod string

const (
	// StoreConnectNameservers moves the domain's DNS here (recommended); the
	// records it serves today are copied over.
	StoreConnectNameservers StoreConnectionMethod = "nameservers"
	// StoreConnectManual leaves DNS at the current provider, where the
	// customer adds the records in [StoreDomain.DNS].
	StoreConnectManual StoreConnectionMethod = "manual"
)

// StoreConnectionStatus is how far a connected domain is connected. The set
// is open.
type StoreConnectionStatus string

const (
	// StoreConnectionPending expires 48 hours after the connection was made.
	StoreConnectionPending StoreConnectionStatus = "pending"
	StoreConnectionActive  StoreConnectionStatus = "active"
	StoreConnectionMoved   StoreConnectionStatus = "moved"
	// StoreConnectionExpired can be started again with
	// [StoreService.RegenerateConnection].
	StoreConnectionExpired StoreConnectionStatus = "expired"
)

// StoreForwardingStatus is the state of a domain's website forwarding. The
// set is open.
type StoreForwardingStatus string

const (
	StoreForwardingNone    StoreForwardingStatus = "none"
	StoreForwardingPending StoreForwardingStatus = "pending"
	StoreForwardingActive  StoreForwardingStatus = "active"
)

// StoreDNSPurpose says why a DNS record exists. The set is open; treat an
// unknown value as [StoreDNSCustom].
type StoreDNSPurpose string

const (
	StoreDNSMX           StoreDNSPurpose = "mx"
	StoreDNSSPF          StoreDNSPurpose = "spf"
	StoreDNSDKIM         StoreDNSPurpose = "dkim"
	StoreDNSDMARC        StoreDNSPurpose = "dmarc"
	StoreDNSVerification StoreDNSPurpose = "verification"
	StoreDNSForwarding   StoreDNSPurpose = "forwarding"
	// StoreDNSOwnership proves a manually connected domain; add it first.
	StoreDNSOwnership StoreDNSPurpose = "ownership"
	// StoreDNSImported is a record copied over when the domain was connected.
	StoreDNSImported StoreDNSPurpose = "imported"
	// StoreDNSCustom is a record the workspace added.
	StoreDNSCustom StoreDNSPurpose = "custom"
)

// StoreDNSCheckStatus is a DNS check's verdict on one record. The set is
// open.
type StoreDNSCheckStatus string

const (
	StoreDNSCheckOK      StoreDNSCheckStatus = "ok"
	StoreDNSCheckMissing StoreDNSCheckStatus = "missing"
)

// StoreMailboxStatus is a purchased mailbox's status at the provider. The set
// is open.
type StoreMailboxStatus string

const (
	StoreMailboxProvisioning StoreMailboxStatus = "provisioning"
	StoreMailboxActive       StoreMailboxStatus = "active"
	StoreMailboxSuspended    StoreMailboxStatus = "suspended"
	StoreMailboxCancelling   StoreMailboxStatus = "cancelling" //nolint:misspell // wire value
	StoreMailboxCancelled    StoreMailboxStatus = "cancelled"  //nolint:misspell // wire value
	StoreMailboxFailed       StoreMailboxStatus = "failed"
)

// StoreConnectState is how far a purchased mailbox is connected to Warmbly.
// The set is open.
type StoreConnectState string

const (
	StoreConnectWaiting    StoreConnectState = "waiting"
	StoreConnectConnecting StoreConnectState = "connecting"
	StoreConnectConnected  StoreConnectState = "connected"
	StoreConnectFailed     StoreConnectState = "failed"
)

// StorePhotoState is a purchased mailbox's profile photo at the provider. The
// set is open.
type StorePhotoState string

const (
	StorePhotoNone StorePhotoState = "none"
	// StorePhotoPending is chosen and sent, not taken by the provider yet.
	StorePhotoPending StorePhotoState = "pending"
	StorePhotoSet     StorePhotoState = "set"
)

// StoreActionRequired is the step only a person can take before a purchased
// mailbox connects. The set is open; treat an unknown value as a step to show.
type StoreActionRequired string

// StoreActionMicrosoftConsent means the domain's Microsoft 365 administrator
// has to approve Warmbly; see [StoreService.MicrosoftConsent].
const StoreActionMicrosoftConsent StoreActionRequired = "microsoft_consent"

// StoreActionReason says why the background approval left a step to a
// person. The set is open; treat an unknown value as [StoreActionFailed].
type StoreActionReason string

const (
	StoreActionMFARequired   StoreActionReason = "mfa_required"
	StoreActionWrongPassword StoreActionReason = "wrong_password"
	StoreActionNotAdmin      StoreActionReason = "not_admin"
	StoreActionWrongTenant   StoreActionReason = "wrong_tenant"
	StoreActionSignInBlocked StoreActionReason = "sign_in_blocked"
	StoreActionNoAdminSignIn StoreActionReason = "no_admin_sign_in"
	StoreActionFailed        StoreActionReason = "failed"
)

// StoreCard is the saved card a checkout tops the balance up from.
type StoreCard struct {
	Brand string `json:"brand"`
	Last4 string `json:"last4"`
}

// StoreBalance is the workspace balance shown at checkout.
type StoreBalance struct {
	BalanceMicros int64 `json:"balance_micros"`
	// Card is nil when no card is saved.
	Card *StoreCard `json:"card"`
}

// StoreLimits are the per-domain mailbox caps.
type StoreLimits struct {
	MailboxesPerDomainMax         int `json:"mailboxes_per_domain_max"`
	MailboxesPerDomainRecommended int `json:"mailboxes_per_domain_recommended"`
}

// StoreDisconnected counts a workspace's live purchases while the instance
// has no GraphKit API key; their renewals, billing and Google connects are
// paused.
type StoreDisconnected struct {
	Mailboxes int `json:"mailboxes"`
	Domains   int `json:"domains"`
}

// StoreNextBundle is the next monthly mailbox charge.
type StoreNextBundle struct {
	ChargeOn     time.Time `json:"charge_on"`
	Mailboxes    int       `json:"mailboxes"`
	AmountMicros int64     `json:"amount_micros"`
}

// StoreRegistrant is the ICANN registrant a workspace's domains are
// registered under. Phone is "+CC.NUMBER" and Country an ISO 3166-1 alpha-2
// code. A refused field fails with 400 "contact_invalid".
type StoreRegistrant struct {
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	Organization string `json:"organization"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	Address1     string `json:"address1"`
	Address2     string `json:"address2"`
	City         string `json:"city"`
	State        string `json:"state"`
	PostalCode   string `json:"postal_code"`
	Country      string `json:"country"`
}

// StoreStatus is whether the store is open to the workspace and what buying
// needs. It never carries domain prices; those come from
// [StoreService.SearchDomains] and quotes.
type StoreStatus struct {
	Enabled bool `json:"enabled"`
	// Configured is whether the instance has a GraphKit API key at all.
	Configured bool `json:"configured"`
	// Reason says why Enabled is false; empty when it is true.
	Reason StoreDisabledReason `json:"reason,omitempty"`
	// Locked is set with [StoreReasonNotConfigured] on a self-hosted
	// instance, which shows buying as a feature to set up.
	Locked bool `json:"locked"`
	// CanConfigure is set with Locked for a session whose user is an
	// instance admin who may add the GraphKit API key; never for a key.
	CanConfigure bool `json:"can_configure"`
	// ConfigureURL is the admin panel page that takes the key, when known.
	ConfigureURL string `json:"configure_url,omitempty"`
	// Disconnected is set with [StoreReasonNotConfigured] while the workspace
	// still has live purchases.
	Disconnected *StoreDisconnected `json:"disconnected,omitempty"`
	// Metered is false on an instance that does not bill its users.
	Metered              bool              `json:"metered"`
	Currency             string            `json:"currency"`
	MailboxMonthlyMicros int64             `json:"mailbox_monthly_micros"`
	DomainMarkup         StoreDomainMarkup `json:"domain_markup"`
	Limits               StoreLimits       `json:"limits"`
	Providers            []StoreProvider   `json:"providers"`
	// Balance is nil when it could not be read.
	Balance *StoreBalance `json:"balance"`
	// PlanActive is false when the workspace has no paid plan, which buying
	// needs. Always true on an instance that does not bill.
	PlanActive bool `json:"plan_active"`
	// Registrant is the saved registrant, nil when none is saved.
	Registrant *StoreRegistrant `json:"registrant"`
	// RegistrantPrefill is a draft from the billing details while Registrant
	// is nil.
	RegistrantPrefill *StoreRegistrant `json:"registrant_prefill"`
	// StripePublishableKey lets a metered checkout save a card in the page.
	StripePublishableKey string `json:"stripe_publishable_key,omitempty"`
	// MinCardChargeMicros is the least a store charge puts on the card.
	MinCardChargeMicros int64 `json:"min_card_charge_micros"`
	// BillingDay is the day of the month (1 to 28) every mailbox is billed;
	// nil before the first mailbox.
	BillingDay *int `json:"billing_day"`
	// NextBundle is the next monthly mailbox charge; nil when nothing is due.
	NextBundle *StoreNextBundle `json:"next_bundle"`
	// DefaultForwardingURL is where new domains forward unless the buyer
	// changes it; empty for none.
	DefaultForwardingURL string `json:"default_forwarding_url"`
	// Photos is whether mailbox photos can be uploaded on this instance.
	Photos bool `json:"photos"`
}

// StoreOffer is one domain search result at retail.
type StoreOffer struct {
	Domain      string `json:"domain"`
	DisplayName string `json:"display_name"`
	TLD         string `json:"tld"`
	Available   bool   `json:"available"`
	Exact       bool   `json:"exact"`
	// Years is the extension's minimum registration term, which PriceMicros
	// covers; pass it as [StoreQuoteItem.Years].
	Years       int   `json:"years"`
	PriceMicros int64 `json:"price_micros"`
	// RenewMicros is per year.
	RenewMicros int64 `json:"renew_micros"`
	// Reason says why an unavailable domain cannot be bought; nil otherwise.
	Reason *StoreOfferReason `json:"reason"`
}

// StoreSearchResult is a domain search.
type StoreSearchResult struct {
	Query string       `json:"query"`
	Data  []StoreOffer `json:"data"`
}

// StoreSearchParams is a domain search.
type StoreSearchParams struct {
	// Query is a word or a domain name; anything else fails with 400
	// "invalid_query".
	Query string
	// TLDs narrows the search to these extensions (at most 10), such as
	// "com" or "io".
	TLDs []string
	// ExactOnly returns only the exact matches (suggestions=false), which
	// answers much sooner.
	ExactOnly bool
}

// StoreMailboxSignature is a mailbox's finished signature, set when it
// connects. Nothing is filled in on the server; both empty buys the mailbox
// without a signature.
type StoreMailboxSignature struct {
	HTML  string `json:"html"`
	Plain string `json:"plain"`
}

// StoreNewMailbox is one mailbox on a [StoreItemMailboxes] quote item.
type StoreNewMailbox struct {
	LocalPart string `json:"local_part"`
	// FirstName and LastName are required, 1 to 64 characters each.
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	// Admin makes this mailbox the domain's Google Workspace or Microsoft 365
	// admin; at most one per item. Without one, the first mailbox is. Leave
	// it false for a domain that already has an admin.
	Admin bool `json:"admin,omitempty"`
	// PhotoID is an uploaded photo ([StoreService.UploadPhoto]) for this
	// mailbox, winning over the item's.
	PhotoID string `json:"photo_id,omitempty"`
	// Signature is set on the mailbox when it connects.
	Signature *StoreMailboxSignature `json:"signature,omitempty"`
}

// StoreQuoteItem is one requested line: a domain, or mailboxes on a domain.
type StoreQuoteItem struct {
	Type   StoreItemType `json:"type"`
	Domain string        `json:"domain"`
	// Years is the search result's Years, for a [StoreItemDomain] item.
	Years int `json:"years,omitempty"`
	// Provider is the mail provider, for a [StoreItemMailboxes] item.
	Provider  StoreProvider     `json:"provider,omitempty"`
	Mailboxes []StoreNewMailbox `json:"mailboxes,omitempty"`
	// PhotoID is an uploaded photo for every mailbox on the item's domain.
	PhotoID string `json:"photo_id,omitempty"`
}

// StoreQuoteParams prices a checkout. A mailboxes item with no domain item
// adds mailboxes to a domain the workspace already has in the store.
type StoreQuoteParams struct {
	Items []StoreQuoteItem `json:"items"`
}

// StoreQuoteLine is one priced line of a quote.
type StoreQuoteLine struct {
	Type          StoreItemType `json:"type"`
	Domain        string        `json:"domain"`
	Years         int           `json:"years,omitempty"`
	Provider      StoreProvider `json:"provider,omitempty"`
	Count         int           `json:"count,omitempty"`
	AmountMicros  int64         `json:"amount_micros"`
	RenewMicros   int64         `json:"renew_micros,omitempty"`
	MonthlyMicros int64         `json:"monthly_micros,omitempty"`
	// AdminEmail is the mailbox that becomes the domain's admin; nil when the
	// domain already has one.
	AdminEmail *string `json:"admin_email,omitempty"`
	// NameserversChange means a connected domain's nameservers have to change
	// after the order, to the set shown on the domain.
	NameserversChange bool `json:"nameservers_change,omitempty"`
}

// StoreAutoReload is the workspace's automatic balance reload as it will be
// after the order.
type StoreAutoReload struct {
	Enabled         bool  `json:"enabled"`
	ThresholdMicros int64 `json:"threshold_micros"`
	AmountMicros    int64 `json:"amount_micros"`
	// StoreManaged is a reload the store turned on and keeps sized to the
	// mailboxes until it is changed by hand.
	StoreManaged bool `json:"store_managed"`
}

// StorePayment is how a checkout will be paid: from the balance, then the
// card, which tops the balance up by at least the store's minimum.
type StorePayment struct {
	BalanceMicros     int64 `json:"balance_micros"`
	FromBalanceMicros int64 `json:"from_balance_micros"`
	// CardChargeMicros is 0 when the balance covers the order.
	CardChargeMicros int64 `json:"card_charge_micros"`
	// BalanceAfterMicros is the balance once the card was charged and the
	// order paid.
	BalanceAfterMicros int64            `json:"balance_after_micros"`
	Card               *StoreCard       `json:"card"`
	NeedsPaymentMethod bool             `json:"needs_payment_method"`
	AutoReload         *StoreAutoReload `json:"auto_reload"`
}

// StoreQuote is a priced checkout, held for 15 minutes (ExpiresAt).
type StoreQuote struct {
	ID                        string           `json:"id"`
	ExpiresAt                 time.Time        `json:"expires_at"`
	Lines                     []StoreQuoteLine `json:"lines"`
	DueNowMicros              int64            `json:"due_now_micros"`
	RecurringMonthlyMicros    int64            `json:"recurring_monthly_micros"`
	YearlyDomainRenewalMicros int64            `json:"yearly_domain_renewal_micros"`
	Payment                   StorePayment     `json:"payment"`
}

// StoreOrderParams pays a quote.
type StoreOrderParams struct {
	QuoteID string `json:"quote_id"`
	// ExpectedDueNowMicros is the total the buyer was shown. When an expired
	// quote prices differently, the order fails with 409 "price_changed" and
	// nothing is charged.
	ExpectedDueNowMicros *int64 `json:"expected_due_now_micros,omitempty"`
	// ForwardingURL is where new domains forward (an https URL); empty for
	// none.
	ForwardingURL string `json:"forwarding_url,omitempty"`
	// EnableAutoReload turns the balance's automatic reload on with the first
	// purchase, sized to the mailboxes. Nil means yes unless the workspace
	// set automatic reload up itself.
	EnableAutoReload *bool `json:"enable_auto_reload,omitempty"`
}

// StoreStep is one stage of an order's progress track.
type StoreStep struct {
	Key    StoreStepKey    `json:"key"`
	Status StoreStepStatus `json:"status"`
	At     *time.Time      `json:"at"`
}

// StoreMailAuth is the state of a domain's mail records.
type StoreMailAuth struct {
	MX    string `json:"mx"`
	SPF   string `json:"spf"`
	DKIM  string `json:"dkim"`
	DMARC string `json:"dmarc"`
}

// StoreDNSRecord is one record in a store domain's zone.
type StoreDNSRecord struct {
	// ID is stable per record.
	ID string `json:"id"`
	// Type is "A", "AAAA", "CNAME", "MX", "TXT", "SRV", "CAA" or another.
	Type string `json:"type"`
	// Name is relative; "@" is the apex.
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	// Priority is set on MX and SRV records, otherwise nil.
	Priority *int `json:"priority"`
	// Managed records are kept by Warmbly for mail and forwarding, so they
	// cannot be changed.
	Managed bool            `json:"managed"`
	Purpose StoreDNSPurpose `json:"purpose"`
	// Status is set only by [StoreService.CheckDNS].
	Status StoreDNSCheckStatus `json:"status,omitempty"`
}

// StoreDNSCheck is every record of a domain looked up in public DNS now.
type StoreDNSCheck struct {
	// Nameservers are the nameservers that answered.
	Nameservers []string         `json:"nameservers"`
	Records     []StoreDNSRecord `json:"records"`
}

// StoreDomainConnection is how far a domain registered elsewhere is
// connected.
type StoreDomainConnection struct {
	Method    StoreConnectionMethod `json:"method"`
	Status    StoreConnectionStatus `json:"status"`
	ExpiresAt *time.Time            `json:"expires_at"`
	CheckedAt *time.Time            `json:"checked_at"`
	// CurrentNameservers is what public DNS showed at the last check.
	CurrentNameservers []string `json:"current_nameservers"`
}

// StoreDNSConflict is a record that blocks mail. Mailboxes cannot be bought
// on the domain while any remain.
type StoreDNSConflict struct {
	// RecordID is set when the record can be deleted with
	// [StoreService.DeleteDNSRecord]; nil when it has to be removed at the
	// domain's DNS provider.
	RecordID *string `json:"record_id"`
	Type     string  `json:"type"`
	Name     string  `json:"name"`
	Content  string  `json:"content"`
}

// StoreOwnerVerification is the registrant's email check. While Status is
// "pending", the registrant has to confirm Email before Deadline.
type StoreOwnerVerification struct {
	Status   string     `json:"status"`
	Email    string     `json:"email"`
	Deadline *time.Time `json:"deadline"`
}

// StoreTransferRequest is another registrar asking for the domain.
type StoreTransferRequest struct {
	// Status is "requested".
	Status        string     `json:"status"`
	RequestedAt   *time.Time `json:"requested_at"`
	AutoApproveAt *time.Time `json:"auto_approve_at"`
}

// StoreDomain is a domain bought in the store, or one registered elsewhere
// and connected.
type StoreDomain struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Status StoreDomainStatus `json:"status"`
	Source StoreDomainSource `json:"source"`
	// Provider is the domain's mail provider; empty before its first mailbox.
	Provider StoreProvider `json:"provider"`
	// Renewable is a purchased domain that is active or expired: renewal,
	// auto-renew and transfer-out apply to it.
	Renewable        bool                  `json:"renewable"`
	ExpiresAt        *time.Time            `json:"expires_at"`
	AutoRenew        bool                  `json:"auto_renew"`
	RenewMicros      int64                 `json:"renew_micros"`
	ForwardingURL    string                `json:"forwarding_url"`
	ForwardingStatus StoreForwardingStatus `json:"forwarding_status"`
	MailAuth         StoreMailAuth         `json:"mail_auth"`
	MailboxCount     int                   `json:"mailbox_count"`
	// Nameservers is the pair the domain should be delegated to.
	Nameservers []string `json:"nameservers"`
	// Connection is nil on a purchased domain.
	Connection        *StoreDomainConnection  `json:"connection"`
	DNSConflicts      []StoreDNSConflict      `json:"dns_conflicts"`
	OwnerVerification *StoreOwnerVerification `json:"owner_verification"`
	Transfer          *StoreTransferRequest   `json:"transfer"`
	// TrackingHost is the open and click tracking host set up once the
	// domain's first mailbox connects; empty until then.
	TrackingHost string    `json:"tracking_host"`
	CreatedAt    time.Time `json:"created_at"`
	// DNS is the domain's records, carried only by single-domain reads and
	// writes such as [StoreService.GetDomain].
	DNS []StoreDNSRecord `json:"dns,omitempty"`
}

// StoreMailbox is a purchased mailbox.
type StoreMailbox struct {
	ID        string             `json:"id"`
	Email     string             `json:"email"`
	DomainID  string             `json:"domain_id"`
	Provider  StoreProvider      `json:"provider"`
	Status    StoreMailboxStatus `json:"status"`
	FirstName string             `json:"first_name"`
	LastName  string             `json:"last_name"`
	// IsAdmin is the domain's Google Workspace or Microsoft 365 admin.
	IsAdmin bool `json:"is_admin"`
	// Paused is campaign sending and warmup stopped because the workspace has
	// no active plan; the mailbox still syncs and bills.
	Paused        bool      `json:"paused"`
	MonthlyMicros int64     `json:"monthly_micros"`
	PaidThrough   time.Time `json:"paid_through"`
	// CancelsAt is set once the mailbox is canceled at the end of its paid
	// month.
	CancelsAt *time.Time `json:"cancels_at"`
	// CancelledAt is when an ended mailbox was switched off in Warmbly.
	CancelledAt *time.Time `json:"cancelled_at"` //nolint:misspell // wire value
	// EmailAccountID is the connected mailbox, once there is one.
	EmailAccountID *string           `json:"email_account_id"`
	ConnectState   StoreConnectState `json:"connect_state"`
	// ActionRequired is the step only a person can take; nil when none.
	ActionRequired *StoreActionRequired `json:"action_required"`
	// ActionReason is set only with ActionRequired.
	ActionReason *StoreActionReason `json:"action_reason"`
	Photo        StorePhotoState    `json:"photo"`
	CreatedAt    time.Time          `json:"created_at"`
}

// StoreOrder is one checkout and its live progress.
type StoreOrder struct {
	ID             string           `json:"id"`
	Status         StoreOrderStatus `json:"status"`
	CreatedAt      time.Time        `json:"created_at"`
	CompletedAt    *time.Time       `json:"completed_at"`
	DueNowMicros   int64            `json:"due_now_micros"`
	ChargedMicros  int64            `json:"charged_micros"`
	RefundedMicros int64            `json:"refunded_micros"`
	Steps          []StoreStep      `json:"steps"`
	Domains        []StoreDomain    `json:"domains"`
	Mailboxes      []StoreMailbox   `json:"mailboxes"`
}

// StoreDomainUpdateParams changes a domain. Nil fields are left alone.
type StoreDomainUpdateParams struct {
	// AutoRenew changes what is charged, so it also needs the manage-billing
	// permission and, for a session, a recent sign-in.
	AutoRenew *bool `json:"auto_renew,omitempty"`
	// ForwardingURL is an https URL, or empty to stop forwarding.
	ForwardingURL *string `json:"forwarding_url,omitempty"`
}

// StoreConnectDomainParams adds a domain registered elsewhere.
type StoreConnectDomainParams struct {
	Domain string `json:"domain"`
	// Method defaults to [StoreConnectNameservers] when empty.
	Method StoreConnectionMethod `json:"method,omitempty"`
}

// StoreDNSRecordParams adds a DNS record.
type StoreDNSRecordParams struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	// TTL is optional.
	TTL int `json:"ttl,omitempty"`
	// Priority is for MX and SRV records.
	Priority *int `json:"priority,omitempty"`
}

// StoreDNSRecordUpdateParams changes a DNS record; its type is fixed. Nil
// fields are left alone.
type StoreDNSRecordUpdateParams struct {
	Name     *string `json:"name,omitempty"`
	Content  *string `json:"content,omitempty"`
	TTL      *int    `json:"ttl,omitempty"`
	Priority *int    `json:"priority,omitempty"`
}

// StoreMailboxUpdateParams renames a mailbox's sender. Nil fields are left
// alone; a refused name fails with 400 "invalid_name".
type StoreMailboxUpdateParams struct {
	FirstName *string `json:"first_name,omitempty"`
	LastName  *string `json:"last_name,omitempty"`
}

// StorePhoto is an uploaded mailbox profile photo, named by
// [StoreNewMailbox.PhotoID], [StoreQuoteItem.PhotoID] or
// [StoreService.SetMailboxPhoto]. One nothing uses is deleted after 24 hours.
type StorePhoto struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

// StoreConsentParams starts a Microsoft 365 approval.
type StoreConsentParams struct {
	// OrderID is the order the administrator lands back on; optional.
	OrderID string `json:"order_id,omitempty"`
}

// StoreConsentLink is where a purchased domain's Microsoft 365 administrator
// approves Warmbly.
type StoreConsentLink struct {
	// URL is a single-use page, valid for an hour; nil when Consented.
	URL       *string    `json:"url"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	// Consented is true when the approval already works; the waiting
	// mailboxes connect within a minute.
	Consented bool `json:"consented"`
}

// StoreTransferCode is a domain's transfer authorization code, shown once.
type StoreTransferCode struct {
	AuthCode string `json:"auth_code"`
}

// StoreSignIn is a purchased mailbox's sign-in, fresh on each call. Show it
// once and do not keep it.
type StoreSignIn struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
	LoginURL string `json:"login_url"`
	// AdminConsoleURL is set for the domain's admin mailbox, whose sign-in
	// opens the Google Workspace or Microsoft 365 admin console.
	AdminConsoleURL *string `json:"admin_console_url"`
}

func storePath(parts ...string) string {
	var b strings.Builder
	b.WriteString("store")
	for _, p := range parts {
		b.WriteByte('/')
		b.WriteString(p)
	}
	return b.String()
}

func storeDomainPath(id string, rest ...string) string {
	return storePath(append([]string{"domains", url.PathEscape(id)}, rest...)...)
}

func storeMailboxPath(id string, rest ...string) string {
	return storePath(append([]string{"mailboxes", url.PathEscape(id)}, rest...)...)
}

// Status reports whether the store is open to the workspace, the balance,
// the saved registrant and what buying needs.
//
// Scope: PermReadStore; the member needs the manage-emails permission.
func (s *StoreService) Status(ctx context.Context, opts ...RequestOption) (*StoreStatus, *Response, error) {
	return fetch[StoreStatus](ctx, s.client, storePath("status"), opts)
}

// SearchDomains searches for domains to buy, priced at retail.
//
// Scope: PermReadStore; the member needs the manage-emails permission.
func (s *StoreService) SearchDomains(ctx context.Context, params *StoreSearchParams, opts ...RequestOption) (*StoreSearchResult, *Response, error) {
	q := make(url.Values)
	if params != nil {
		setNonEmpty(q, "q", params.Query)
		if len(params.TLDs) > 0 {
			q.Set("tlds", strings.Join(params.TLDs, ","))
		}
		if params.ExactOnly {
			q.Set("suggestions", "false")
		}
	}
	return fetch[StoreSearchResult](ctx, s.client, withQuery(storePath("domains", "search"), q), opts)
}

// PutRegistrant saves the registrant new domains are registered to and
// returns it. A quote with a domain in it needs one first.
//
// Scope: PermManageStore; the member needs the manage-emails and
// manage-billing permissions.
func (s *StoreService) PutRegistrant(ctx context.Context, registrant *StoreRegistrant, opts ...RequestOption) (*StoreRegistrant, *Response, error) {
	return send[StoreRegistrant](ctx, s.client.put, storePath("registrant"), registrant, opts)
}

// CreateQuote prices a checkout and answers 201. Nothing is charged; pay it
// with [StoreService.CreateOrder] within 15 minutes, or it is priced again.
// It fails with 402 "plan_required" without a paid plan and 422
// "registrant_required" for a domain without a saved registrant.
//
// Scope: PermManageStore; the member needs the manage-emails permission.
func (s *StoreService) CreateQuote(ctx context.Context, params *StoreQuoteParams, opts ...RequestOption) (*StoreQuote, *Response, error) {
	return send[StoreQuote](ctx, s.client.post, storePath("quotes"), params, opts)
}

// CreateOrder pays a quote from the balance, charging the saved card for a
// shortfall, and answers 201 with the order. An item the store cannot deliver
// is refunded to the balance. It fails with 409 "price_changed" when an
// expired quote priced differently from ExpectedDueNowMicros, and with 402
// "payment_method_required" when no card covers a shortfall; nothing is
// charged in either case. Send [WithIdempotencyKey] so a retry never charges
// twice.
//
// Scope: PermManageStore; the member needs the manage-emails and
// manage-billing permissions, and a session needs a recent sign-in.
func (s *StoreService) CreateOrder(ctx context.Context, params *StoreOrderParams, opts ...RequestOption) (*StoreOrder, *Response, error) {
	return send[StoreOrder](ctx, s.client.post, storePath("orders"), params, opts)
}

// ListOrders returns a page of the workspace's orders, newest first. Limit is
// 1 to 100.
//
// Scope: PermReadStore; the member needs the manage-emails permission.
func (s *StoreService) ListOrders(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[StoreOrder], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[StoreOrder](ctx, s.client, storePath("orders"), q, opts...)
}

// GetOrder retrieves one order with its progress, domains and mailboxes.
//
// Scope: PermReadStore; the member needs the manage-emails permission.
func (s *StoreService) GetOrder(ctx context.Context, id string, opts ...RequestOption) (*StoreOrder, *Response, error) {
	return fetch[StoreOrder](ctx, s.client, storePath("orders", url.PathEscape(id)), opts)
}

// ListDomains returns a page of the workspace's store domains, purchased and
// connected. Limit is 1 to 100.
//
// Scope: PermReadStore; the member needs the manage-emails permission.
func (s *StoreService) ListDomains(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[StoreDomain], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[StoreDomain](ctx, s.client, storePath("domains"), q, opts...)
}

// GetDomain retrieves one store domain with its DNS records.
//
// Scope: PermReadStore; the member needs the manage-emails permission.
func (s *StoreService) GetDomain(ctx context.Context, id string, opts ...RequestOption) (*StoreDomain, *Response, error) {
	return fetch[StoreDomain](ctx, s.client, storeDomainPath(id), opts)
}

// UpdateDomain changes a domain's auto-renew or website forwarding and
// returns it. Auto-renew applies only when [StoreDomain.Renewable].
//
// Scope: PermManageStore; the member needs the manage-emails permission.
// Changing AutoRenew also needs the manage-billing permission and, for a
// session, a recent sign-in (403 "reauth_required" otherwise).
func (s *StoreService) UpdateDomain(ctx context.Context, id string, params *StoreDomainUpdateParams, opts ...RequestOption) (*StoreDomain, *Response, error) {
	return send[StoreDomain](ctx, s.client.patch, storeDomainPath(id), params, opts)
}

// RenewDomain charges and renews a purchased domain for a year now, and
// returns it. A connected domain fails with 409
// "domain_registered_elsewhere". Send [WithIdempotencyKey] so a retry never
// charges twice.
//
// Scope: PermManageStore; the member needs the manage-emails and
// manage-billing permissions, and a session needs a recent sign-in.
func (s *StoreService) RenewDomain(ctx context.Context, id string, opts ...RequestOption) (*StoreDomain, *Response, error) {
	return send[StoreDomain](ctx, s.client.post, storeDomainPath(id, "renew"), nil, opts)
}

// TransferOut unlocks a purchased domain and returns its transfer
// authorization code, shown once and sent with Cache-Control: no-store.
//
// It takes no Idempotency-Key: the server keeps no copy of the answer and
// ignores a key. Repeating the call charges nothing and returns the code
// again.
//
// Scope: PermManageStore; the member needs the manage-emails and
// manage-billing permissions, and a session needs a recent sign-in.
func (s *StoreService) TransferOut(ctx context.Context, id string, opts ...RequestOption) (*StoreTransferCode, *Response, error) {
	return send[StoreTransferCode](ctx, s.client.post, storeDomainPath(id, "transfer-out"), nil, opts)
}

// ConnectDomain adds a domain registered elsewhere and answers 201 with it,
// DNS records included. Nothing is charged. A domain that cannot be added
// fails with 409 "domain_unavailable" and a [Error.Reason].
//
// Scope: PermManageStore; the member needs the manage-emails permission.
func (s *StoreService) ConnectDomain(ctx context.Context, params *StoreConnectDomainParams, opts ...RequestOption) (*StoreDomain, *Response, error) {
	return send[StoreDomain](ctx, s.client.post, storePath("domains", "connect"), params, opts)
}

// CheckConnection looks for a connected domain's nameservers or records now
// and returns the domain. Warmbly also checks on its own.
//
// Scope: PermManageStore; the member needs the manage-emails permission.
func (s *StoreService) CheckConnection(ctx context.Context, id string, opts ...RequestOption) (*StoreDomain, *Response, error) {
	return send[StoreDomain](ctx, s.client.post, storeDomainPath(id, "connection", "check"), nil, opts)
}

// RegenerateConnection issues a new pair of nameservers or a new ownership
// record after a connection reached [StoreConnectionExpired], and returns the
// domain with its DNS records.
//
// Scope: PermManageStore; the member needs the manage-emails permission.
func (s *StoreService) RegenerateConnection(ctx context.Context, id string, opts ...RequestOption) (*StoreDomain, *Response, error) {
	return send[StoreDomain](ctx, s.client.post, storeDomainPath(id, "connection", "regenerate"), nil, opts)
}

// RemoveDomain removes a connected domain. The server answers 204. A domain
// with mailboxes fails with 409 "domain_in_use", and a purchased one with 409
// "domain_registered_here" (it leaves by [StoreService.TransferOut]).
//
// Scope: PermManageStore; the member needs the manage-emails permission.
func (s *StoreService) RemoveDomain(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, storeDomainPath(id), opts...)
}

// AddDNSRecord adds a record to a store domain and answers 201 with it.
//
// Scope: PermManageStore; the member needs the manage-emails permission.
func (s *StoreService) AddDNSRecord(ctx context.Context, domainID string, params *StoreDNSRecordParams, opts ...RequestOption) (*StoreDNSRecord, *Response, error) {
	return send[StoreDNSRecord](ctx, s.client.post, storeDomainPath(domainID, "dns"), params, opts)
}

// UpdateDNSRecord changes a record and returns it. A managed record fails
// with 409 "dns_record_managed".
//
// Scope: PermManageStore; the member needs the manage-emails permission.
func (s *StoreService) UpdateDNSRecord(ctx context.Context, domainID, recordID string, params *StoreDNSRecordUpdateParams, opts ...RequestOption) (*StoreDNSRecord, *Response, error) {
	return send[StoreDNSRecord](ctx, s.client.patch, storeDomainPath(domainID, "dns", url.PathEscape(recordID)), params, opts)
}

// DeleteDNSRecord deletes a record. The server answers 204.
//
// Scope: PermManageStore; the member needs the manage-emails permission.
func (s *StoreService) DeleteDNSRecord(ctx context.Context, domainID, recordID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, storeDomainPath(domainID, "dns", url.PathEscape(recordID)), opts...)
}

// CheckDNS looks every record of a domain up in public DNS now, setting each
// record's Status. It changes nothing.
//
// Scope: PermReadStore; the member needs the manage-emails permission.
func (s *StoreService) CheckDNS(ctx context.Context, domainID string, opts ...RequestOption) (*StoreDNSCheck, *Response, error) {
	return send[StoreDNSCheck](ctx, s.client.post, storeDomainPath(domainID, "dns", "check"), nil, opts)
}

// MicrosoftConsent returns the page where a purchased domain's Microsoft 365
// administrator approves Warmbly for the mailboxes waiting with
// [StoreActionMicrosoftConsent]. It changes nothing until they approve.
// params may be nil.
//
// Scope: PermManageStore; the member needs the manage-emails permission.
func (s *StoreService) MicrosoftConsent(ctx context.Context, domainID string, params *StoreConsentParams, opts ...RequestOption) (*StoreConsentLink, *Response, error) {
	return send[StoreConsentLink](ctx, s.client.post, storeDomainPath(domainID, "microsoft-consent"), params, opts)
}

// ListMailboxes returns a page of the workspace's purchased mailboxes. Limit
// is 1 to 100.
//
// Scope: PermReadStore; the member needs the manage-emails permission.
func (s *StoreService) ListMailboxes(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[StoreMailbox], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[StoreMailbox](ctx, s.client, storePath("mailboxes"), q, opts...)
}

// UpdateMailbox changes a purchased mailbox's sender name and returns it.
//
// Scope: PermManageStore; the member needs the manage-emails permission.
func (s *StoreService) UpdateMailbox(ctx context.Context, id string, params *StoreMailboxUpdateParams, opts ...RequestOption) (*StoreMailbox, *Response, error) {
	return send[StoreMailbox](ctx, s.client.patch, storeMailboxPath(id), params, opts)
}

// UploadPhoto uploads a profile photo for purchased mailboxes, sent as the
// multipart field "file" (PNG or JPG, up to 2 MB and 2048 px on each side),
// and answers 201. Name the photo's ID on a quote or with
// [StoreService.SetMailboxPhoto].
//
// Scope: PermManageStore; the member needs the manage-emails permission.
func (s *StoreService) UploadPhoto(ctx context.Context, file *FileUpload, opts ...RequestOption) (*StorePhoto, *Response, error) {
	out := new(StorePhoto)
	resp, err := s.client.postMultipart(ctx, storePath("photos"), "file", file, nil, out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// CopyProfilePhoto copies the caller's own profile photo as a mailbox photo
// and answers 201. It fails with 404 "profile_photo_missing" when the caller
// has none.
//
// Scope: PermManageStore; the member needs the manage-emails permission.
func (s *StoreService) CopyProfilePhoto(ctx context.Context, opts ...RequestOption) (*StorePhoto, *Response, error) {
	body := struct {
		From string `json:"from"`
	}{From: "profile"}
	return send[StorePhoto](ctx, s.client.post, storePath("photos"), body, opts)
}

// GetPhoto streams one of the workspace's uploaded photos to w as a JPEG.
//
// Scope: PermReadStore; the member needs the manage-emails permission.
func (s *StoreService) GetPhoto(ctx context.Context, id string, w io.Writer, opts ...RequestOption) (*Response, error) {
	return s.client.get(ctx, storePath("photos", url.PathEscape(id)), w, opts...)
}

// SetMailboxPhoto makes an uploaded photo the mailbox's Google or Microsoft
// profile picture and returns the mailbox. A photo can be replaced, not
// removed.
//
// Scope: PermManageStore; the member needs the manage-emails permission.
func (s *StoreService) SetMailboxPhoto(ctx context.Context, mailboxID, photoID string, opts ...RequestOption) (*StoreMailbox, *Response, error) {
	body := struct {
		PhotoID string `json:"photo_id"`
	}{PhotoID: photoID}
	return send[StoreMailbox](ctx, s.client.put, storeMailboxPath(mailboxID, "photo"), body, opts)
}

// CancelMailbox cancels a purchased mailbox at the end of its paid month and
// returns it with CancelsAt set. Send [WithIdempotencyKey].
//
// Scope: PermManageStore; the member needs the manage-emails and
// manage-billing permissions, and a session needs a recent sign-in.
func (s *StoreService) CancelMailbox(ctx context.Context, id string, opts ...RequestOption) (*StoreMailbox, *Response, error) {
	out := new(StoreMailbox)
	resp, err := s.client.call(ctx, http.MethodDelete, storeMailboxPath(id), nil, out, opts)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// ResumeMailbox clears a pending cancellation and returns the mailbox. Send
// [WithIdempotencyKey].
//
// Scope: PermManageStore; the member needs the manage-emails and
// manage-billing permissions, and a session needs a recent sign-in.
func (s *StoreService) ResumeMailbox(ctx context.Context, id string, opts ...RequestOption) (*StoreMailbox, *Response, error) {
	return send[StoreMailbox](ctx, s.client.post, storeMailboxPath(id, "resume"), nil, opts)
}

// SignIn returns a fresh sign-in for a purchased mailbox, shown once and sent
// with Cache-Control: no-store; for the admin mailbox it opens the admin
// console. Each call is audited. It fails with 409 "sign_in_unavailable" when
// there is nothing to show.
//
// It takes no Idempotency-Key: the server keeps no copy of the answer and
// ignores a key. Repeating the call returns a sign-in again.
//
// Scope: PermManageStore; the member needs the manage-emails permission, and
// a session needs a recent sign-in.
func (s *StoreService) SignIn(ctx context.Context, mailboxID string, opts ...RequestOption) (*StoreSignIn, *Response, error) {
	return send[StoreSignIn](ctx, s.client.post, storeMailboxPath(mailboxID, "sign-in"), nil, opts)
}
