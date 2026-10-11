package warmbly

import (
	"sort"
	"strings"
)

// OAuth scope strings. Each is the lowercased name of an API permission, so a
// token's granted scopes are an API permission mask and pass through the same
// route gates as an API key. Request them in [OAuth2Config.Scopes] or
// [ClientCredentialsConfig.Scopes].
//
// An app is never granted [ScopeAPIKeys] (see [PermAppGrantable]), and consent
// narrows every request to what the approving member's role may delegate (see
// [APIPermissionsForMember]).
const (
	ScopeReadEmails        = "read_emails"
	ScopeReadCampaigns     = "read_campaigns"
	ScopeReadContacts      = "read_contacts"
	ScopeReadUnibox        = "read_unibox"
	ScopeReadAnalytics     = "read_analytics"
	ScopeReadTemplates     = "read_templates"
	ScopeReadCRM           = "read_crm"
	ScopeReadAuditLogs     = "read_audit_logs"
	ScopeWriteEmails       = "write_emails"
	ScopeWriteCampaigns    = "write_campaigns"
	ScopeWriteContacts     = "write_contacts"
	ScopeWriteUnibox       = "write_unibox"
	ScopeWriteTemplates    = "write_templates"
	ScopeWriteCRM          = "write_crm"
	ScopeSendCampaigns     = "send_campaigns"
	ScopeBulkContacts      = "bulk_contacts"
	ScopeBulkCampaigns     = "bulk_campaigns"
	ScopeRealtimeSubscribe = "realtime_subscribe"
	ScopeWebhooks          = "webhooks"
	ScopeAPIKeys           = "api_keys"
	ScopeIntegrations      = "integrations"
	ScopeWarmupRouting     = "warmup_routing"
	ScopeAIAgent           = "ai_agent"
	ScopeAIResearch        = "ai_research"
)

// scopeBits maps each scope string to its API permission bit.
var scopeBits = map[string]uint64{
	ScopeReadEmails:        PermReadEmails,
	ScopeReadCampaigns:     PermReadCampaigns,
	ScopeReadContacts:      PermReadContacts,
	ScopeReadUnibox:        PermReadUnibox,
	ScopeReadAnalytics:     PermReadAnalytics,
	ScopeReadTemplates:     PermReadTemplates,
	ScopeReadCRM:           PermReadCRM,
	ScopeReadAuditLogs:     PermReadAuditLogs,
	ScopeWriteEmails:       PermWriteEmails,
	ScopeWriteCampaigns:    PermWriteCampaigns,
	ScopeWriteContacts:     PermWriteContacts,
	ScopeWriteUnibox:       PermWriteUnibox,
	ScopeWriteTemplates:    PermWriteTemplates,
	ScopeWriteCRM:          PermWriteCRM,
	ScopeSendCampaigns:     PermSendCampaigns,
	ScopeBulkContacts:      PermBulkContacts,
	ScopeBulkCampaigns:     PermBulkCampaigns,
	ScopeRealtimeSubscribe: PermRealtimeSubscribe,
	ScopeWebhooks:          PermWebhooks,
	ScopeAPIKeys:           PermAPIKeys,
	ScopeIntegrations:      PermIntegrations,
	ScopeWarmupRouting:     PermWarmupRouting,
	ScopeAIAgent:           PermAIAgent,
	ScopeAIResearch:        PermAIResearch,
}

// ScopesForPermissions expands an API permission mask into its scope strings,
// sorted, in the form a token response and the consent screen use. Bits this
// SDK release does not know are skipped.
func ScopesForPermissions(mask uint64) []string {
	out := make([]string, 0, len(scopeBits))
	for s, bit := range scopeBits {
		if mask&bit == bit {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// PermissionsForScopes folds scope strings into an API permission mask. Each
// entry may itself hold several space-separated scopes, as a token response's
// "scope" field does, and matching ignores case like the server. Scopes it
// does not recognize are returned in unknown rather than dropped silently.
func PermissionsForScopes(scopes ...string) (mask uint64, unknown []string) {
	for _, raw := range scopes {
		for _, s := range strings.Fields(raw) {
			if bit, ok := scopeBits[strings.ToLower(s)]; ok {
				mask |= bit
			} else {
				unknown = append(unknown, s)
			}
		}
	}
	return mask, unknown
}
