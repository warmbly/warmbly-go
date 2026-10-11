package warmbly

// PermAppGrantable is every API scope a third-party OAuth app may be granted.
// Key management ([PermAPIKeys]) stays with people, so an app never holds it.
const PermAppGrantable = PermFullAccess &^ PermAPIKeys

// apiPermissionRoleNeeds is the organization permission each API scope acts
// under, mirroring the server. Zero means any member may delegate the scope.
var apiPermissionRoleNeeds = map[uint64]uint16{
	PermReadEmails:        OrgPermViewCampaigns,
	PermReadCampaigns:     OrgPermViewCampaigns,
	PermReadContacts:      OrgPermViewContacts,
	PermReadUnibox:        OrgPermAccessUnibox,
	PermReadAnalytics:     OrgPermViewAnalytics,
	PermWriteEmails:       OrgPermManageEmails,
	PermWriteCampaigns:    OrgPermManageCampaigns,
	PermWriteContacts:     OrgPermManageContacts,
	PermWriteUnibox:       OrgPermAccessUnibox,
	PermBulkContacts:      OrgPermManageContacts,
	PermBulkCampaigns:     OrgPermManageCampaigns,
	PermRealtimeSubscribe: 0,
	PermWebhooks:          OrgPermManageSettings,
	PermAPIKeys:           OrgPermManageAPIKeys,
	PermSendCampaigns:     OrgPermSendCampaigns,
	PermReadTemplates:     OrgPermViewCampaigns,
	PermWriteTemplates:    OrgPermManageCampaigns,
	PermReadCRM:           OrgPermViewContacts,
	PermWriteCRM:          OrgPermManageContacts,
	PermReadAuditLogs:     OrgPermViewAnalytics,
	PermIntegrations:      OrgPermUseIntegrations,
	PermWarmupRouting:     OrgPermManageSettings,
	PermAIAgent:           OrgPermUseAI,
	PermAIResearch:        OrgPermViewContacts | OrgPermUseAI,
}

// APIPermissionsForMember is the widest set of API scopes a member holding the
// given organization permissions (the OrgPerm* bits, as in
// [Member.Permissions]) may delegate to an API key or an OAuth app. The owner
// may delegate every scope.
//
// The server applies the same cap on every request, not only at creation: a
// key or OAuth grant is held to its creator's current role, so a scope the
// role no longer covers stops working, and creating a key with more answers
// [ErrCodeAPIKeyPermissionsExceedCaller]. OAuth consent narrows the requested
// scopes to this set and reports the ones withheld.
//
// The mapping is the one this SDK release knows; a scope the server adds
// later is not included.
func APIPermissionsForMember(orgPerms uint16, owner bool) uint64 {
	if owner {
		return PermFullAccess
	}
	var mask uint64
	for bit, need := range apiPermissionRoleNeeds {
		if orgPerms&need == need {
			mask |= bit
		}
	}
	return mask
}
