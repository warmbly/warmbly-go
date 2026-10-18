package warmbly

import "testing"

func TestAPIPermissionsForMember(t *testing.T) {
	if got := APIPermissionsForMember(0, true); got != PermFullAccess {
		t.Errorf("owner = %b, want PermFullAccess", got)
	}
	if got := APIPermissionsForMember(0, false); got != PermRealtimeSubscribe {
		t.Errorf("no permissions = %b, want only PermRealtimeSubscribe", got)
	}

	viewer := OrgPermViewCampaigns | OrgPermViewContacts | OrgPermViewAnalytics
	want := PermReadEmails | PermReadCampaigns | PermReadContacts | PermReadAnalytics |
		PermReadTemplates | PermReadCRM | PermReadAuditLogs | PermRealtimeSubscribe
	if got := APIPermissionsForMember(viewer, false); got != want {
		t.Errorf("viewer = %b, want %b", got, want)
	}

	// AI research needs both contacts and AI.
	if got := APIPermissionsForMember(OrgPermUseAI, false); got&PermAIResearch != 0 || got&PermAIAgent == 0 {
		t.Errorf("use_ai alone = %b, want AI agent without AI research", got)
	}
	if got := APIPermissionsForMember(OrgPermUseAI|OrgPermViewContacts, false); got&PermAIResearch == 0 {
		t.Errorf("use_ai + view_contacts = %b, want AI research", got)
	}

	if got := APIPermissionsForMember(OrgPermAll, false); got != PermFullAccess {
		t.Errorf("every org permission = %b, want PermFullAccess", got)
	}
	if PermAppGrantable&PermAPIKeys != 0 || PermAppGrantable|PermAPIKeys != PermFullAccess {
		t.Errorf("PermAppGrantable = %b", PermAppGrantable)
	}
}

// TestAPIPermissionRoleNeedsCoverEveryScope keeps the cap table in step with
// the scope bits: a scope missing from it could never be delegated.
func TestAPIPermissionRoleNeedsCoverEveryScope(t *testing.T) {
	var covered uint64
	for bit := range apiPermissionRoleNeeds {
		covered |= bit
	}
	if covered != PermFullAccess {
		t.Errorf("apiPermissionRoleNeeds covers %b, want PermFullAccess %b", covered, PermFullAccess)
	}
}

func TestAuditEntitiesAreDistinct(t *testing.T) {
	seen := map[string]bool{}
	for _, v := range []string{
		AuditEntityCampaignLead, AuditEntityMailboxImport, AuditEntityMailboxGrant,
		AuditEntityMailboxVendor, AuditEntityDomainRedirect, AuditEntityEmailImage,
		AuditEntityAIToolPolicy, AuditEntityAppListing, AuditEntityOAuthAuthorization,
		AuditEntityPlacementTest, AuditEntityPlacementBatch, AuditEntityPlacementMonitor,
	} {
		if v == "" || seen[v] {
			t.Errorf("audit entity %q is empty or repeated", v)
		}
		seen[v] = true
	}
}
