package warmbly

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestScopesMatchPermissionBits(t *testing.T) {
	if len(scopeBits) != 24 {
		t.Fatalf("scopeBits has %d entries, want one per API permission (24)", len(scopeBits))
	}
	var all uint64
	for s, bit := range scopeBits {
		if all&bit != 0 {
			t.Errorf("scope %q reuses bit %d", s, bit)
		}
		all |= bit
	}
	if all != PermFullAccess {
		t.Errorf("scopes cover %b, want PermFullAccess %b", all, PermFullAccess)
	}

	got := ScopesForPermissions(PermReadCampaigns | PermRealtimeSubscribe | PermAIResearch)
	want := []string{ScopeAIResearch, ScopeReadCampaigns, ScopeRealtimeSubscribe}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ScopesForPermissions = %v, want %v", got, want)
	}
	if got := ScopesForPermissions(0); len(got) != 0 {
		t.Errorf("ScopesForPermissions(0) = %v, want none", got)
	}

	mask, unknown := PermissionsForScopes("read_campaigns REALTIME_SUBSCRIBE", "ai_research", "campaigns:read")
	if mask != PermReadCampaigns|PermRealtimeSubscribe|PermAIResearch {
		t.Errorf("PermissionsForScopes mask = %b", mask)
	}
	if !reflect.DeepEqual(unknown, []string{"campaigns:read"}) {
		t.Errorf("PermissionsForScopes unknown = %v", unknown)
	}
}

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

func TestNewErrorCodesMatch(t *testing.T) {
	body := `{"error":"Unauthorized","message":"The member who created this API key is no longer in this workspace.","code":"api_key_holder_left","request_id":"req_1"}`
	var apiErr Error
	if err := json.Unmarshal([]byte(body), &apiErr); err != nil {
		t.Fatal(err)
	}
	apiErr.StatusCode = 401
	if !apiErr.HasCode(ErrCodeAPIKeyHolderLeft) {
		t.Errorf("HasCode(%q) = false", ErrCodeAPIKeyHolderLeft)
	}
	if !errors.Is(&apiErr, ErrUnauthorized) {
		t.Error("api_key_holder_left should match ErrUnauthorized")
	}
	for _, code := range []string{
		ErrCodeReauthRequired, ErrCodeReauthLimited, ErrCodeReauthNoFactor,
		ErrCodeAPIKeyMailboxLimited, ErrCodeAPIKeyPermissionsExceedCaller,
		ErrCodeOAuthTokenNotAllowed, ErrCodeMemberAccessRestricted,
		ErrCodeSelectionTooLarge, ErrCodeInvalidRecipient, ErrCodeInvalidMessageID,
		ErrCodeInvalidName, ErrCodePasswordBreached, ErrCodeSSOLinkExpired,
		ErrCodePasswordChangedSignInAgain, ErrCodePasskeyUserVerificationRequired,
		ErrCodeTwoFAInvalidCode,
	} {
		if code == "" {
			t.Error("empty error code constant")
		}
	}
}

func TestOAuthAppDecodesSuspension(t *testing.T) {
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	want := OAuthApp{
		ID: "app_1", OrganizationID: "org_1", CreatedBy: "u_1", Name: "Acme Sync",
		ClientID: "wmbly_client_1", RedirectURIs: []string{"https://acme.example/cb"},
		Scopes: PermReadCampaigns, Status: OAuthAppDisabled,
		SuspendedAt: &at, SuspendedReason: "Abuse report",
		CreatedAt: at, UpdatedAt: at,
	}
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got OAuthApp
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
	var wire map[string]any
	_ = json.Unmarshal(raw, &wire)
	if wire["suspended_at"] == nil || wire["suspended_reason"] != "Abuse report" || wire["status"] != "disabled" {
		t.Errorf("wire = %v", wire)
	}
}

func TestWarmupWebhookPayloadsDecode(t *testing.T) {
	ev := &WebhookEvent{
		EventType: EventWarmupPlacementInSpam,
		Data:      json.RawMessage(`{"email_account_id":"a_1","email":"s@acme.com","content_source":"library","recipient_provider":"gmail","recipient_host":"google_workspace"}`),
	}
	var spam WarmupPlacementInSpamPayload
	if err := ev.Into(&spam); err != nil {
		t.Fatal(err)
	}
	if spam.RecipientHost != "google_workspace" || spam.EmailAccountID != "a_1" || spam.RecipientProvider != "gmail" {
		t.Errorf("WarmupPlacementInSpamPayload = %+v", spam)
	}

	ev = &WebhookEvent{
		EventType: EventWarmupBlocked,
		Data:      json.RawMessage(`{"email_account_id":"a_1","email":"s@acme.com","previous_state":"quarantined","new_state":"blocked","reason":"Repeated spam reports"}`),
	}
	var health WarmupHealthPayload
	if err := ev.Into(&health); err != nil {
		t.Fatal(err)
	}
	if health.PreviousState != "quarantined" || health.NewState != "blocked" || health.Reason == "" {
		t.Errorf("WarmupHealthPayload = %+v", health)
	}
}

func TestNewAuditEntitiesAreDistinct(t *testing.T) {
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
