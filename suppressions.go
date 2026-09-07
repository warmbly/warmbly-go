package warmbly

import (
	"context"
	"net/url"
	"time"
)

// SuppressionService manages the workspace suppression list: every address
// and domain that no campaign will email, whatever put it there. The list is
// checked before every campaign send.
//
// Entries arrive two ways. Warmbly adds them automatically on a hard bounce,
// a spam complaint or an unsubscribe (sources bounce, complaint,
// unsubscribe); you add them by hand with [SuppressionService.Add] (sources
// manual and import). Lifting an entry with [SuppressionService.Delete] lets
// campaigns email the address, or every address at the domain, again.
//
// Adding an address also switches off the matching contact's subscribed
// flag, and removing it switches the flag back on, so the list and the
// contact never disagree. Reads take the contact read scope, writes the
// contact write scope.
type SuppressionService service

// SuppressionKind says what a suppression entry matches.
type SuppressionKind string

const (
	// SuppressionKindEmail matches one address.
	SuppressionKindEmail SuppressionKind = "email"
	// SuppressionKindDomain matches every address at a domain; the entry's
	// Email holds the bare host.
	SuppressionKindDomain SuppressionKind = "domain"
)

// SuppressionSource records what put an entry on the list.
type SuppressionSource string

const (
	// SuppressionSourceBounce: a permanent (5.x.x) delivery failure.
	SuppressionSourceBounce SuppressionSource = "bounce"
	// SuppressionSourceComplaint: the recipient marked a send as spam.
	SuppressionSourceComplaint SuppressionSource = "complaint"
	// SuppressionSourceUnsubscribe: the recipient opted out, by link, reply
	// keyword or List-Unsubscribe header.
	SuppressionSourceUnsubscribe SuppressionSource = "unsubscribe"
	// SuppressionSourceManual: added by hand, one entry at a time.
	SuppressionSourceManual SuppressionSource = "manual"
	// SuppressionSourceImport: added by hand as a batch of two or more.
	SuppressionSourceImport SuppressionSource = "import"
)

// SuppressionMaxEntries is the most entries one Add call accepts.
const SuppressionMaxEntries = 5000

// SuppressedRecipient is one entry on the suppression list.
type SuppressedRecipient struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	// Email is the suppressed address, or the bare domain when Kind is
	// [SuppressionKindDomain]. Always lower-case.
	Email  string            `json:"email"`
	Kind   SuppressionKind   `json:"kind"`
	Reason string            `json:"reason"`
	Source SuppressionSource `json:"source"`
	// CampaignID is the campaign whose send produced an automatic entry;
	// nil for manual and import entries.
	CampaignID *string `json:"campaign_id,omitempty"`
	// ExpiresAt, when set, is when the entry stops applying. Expired entries
	// are never listed. Entries added through the API do not expire.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	// Metadata carries provenance, such as "added_by" (the user who added a
	// manual entry) or "via" for an unsubscribe.
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// SuppressionListParams filters and pages the suppression list. Pages are
// newest first; Limit is capped at 200 and defaults to 50.
type SuppressionListParams struct {
	ListOptions
	// Query is a case-insensitive substring filter on the address or
	// domain.
	Query string `json:"-"`
}

func (p *SuppressionListParams) query() url.Values {
	q := url.Values{}
	if p == nil {
		return q
	}
	p.apply(q)
	setNonEmpty(q, "q", p.Query)
	return q
}

// SuppressionEntry is one value to suppress.
type SuppressionEntry struct {
	// Value is an address ("dana@acme.com") or a domain ("acme.com" or
	// "@acme.com"). Anything with an "@" after a leading one is stripped is
	// treated as an address; a bare host is a domain. The server decides
	// the kind: there is no way to force a value to one or the other.
	Value string `json:"value"`
	// Reason overrides [SuppressionAddParams.Reason] for this entry.
	Reason string `json:"reason,omitempty"`
}

// SuppressionAddParams adds addresses and domains to the list.
type SuppressionAddParams struct {
	// Entries holds up to [SuppressionMaxEntries] values. Duplicates within
	// the batch are collapsed.
	Entries []SuppressionEntry `json:"entries"`
	// Reason applies to every entry that does not carry its own. When both
	// are empty the server records a generic one.
	Reason string `json:"reason,omitempty"`
}

// SuppressionAddResult reports what an Add did.
type SuppressionAddResult struct {
	// Added is how many entries were written, counting values already on
	// the list that were updated in place.
	Added int `json:"added"`
	// Skipped lists the values, as sent, that were neither a valid address
	// nor a valid domain. They do not fail the request.
	Skipped []string `json:"skipped"`
}

// List pages the suppression list, newest first. Expired entries are
// omitted. Pass nil for the first page with defaults.
func (s *SuppressionService) List(ctx context.Context, params *SuppressionListParams, opts ...RequestOption) (*Page[SuppressedRecipient], error) {
	return listJSON[SuppressedRecipient](ctx, s.client, "suppressions", params.query(), opts...)
}

// Add puts addresses and domains on the list. A single entry is recorded
// with source manual, two or more with source import. Values already on the
// list are updated in place rather than rejected, so the call is safe to
// repeat; values that are neither an address nor a domain come back in
// [SuppressionAddResult.Skipped]. The batch is written in one transaction.
// Adding an address also unsubscribes the matching contact.
func (s *SuppressionService) Add(ctx context.Context, params *SuppressionAddParams, opts ...RequestOption) (*SuppressionAddResult, *Response, error) {
	out, resp, err := send[SuppressionAddResult](ctx, s.client.post, "suppressions", params, opts)
	if err != nil {
		return nil, resp, err
	}
	if out.Skipped == nil {
		out.Skipped = []string{}
	}
	return out, resp, nil
}

// Delete lifts one entry so campaigns can email the address (or every
// address at the domain) again. Removing an address entry also restores the
// matching contact's subscribed flag. The removal is audited with the
// entry's value, kind and source, since lifting an opt-out the recipient
// made themselves is what a compliance review looks for. Returns a 404 when
// the entry is not in this organization.
func (s *SuppressionService) Delete(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "suppressions/"+url.PathEscape(id), opts...)
}
