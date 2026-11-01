package warmbly

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestErrorEnvelopeCarriesReason: a few codes add an optional "reason" beside
// the code, and the typed error must keep it alongside the new code constants.
func TestErrorEnvelopeCarriesReason(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "req_1")
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"error":"Payment Required","message":"No free tests left this month.","code":"placement_quota_exceeded","reason":"free_tests_used","request_id":"req_1"}`))
	})

	_, _, err := c.Webhooks.EventTypes(context.Background())
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want *Error", err)
	}
	if !apiErr.HasCode(ErrCodePlacementQuotaExceeded) {
		t.Errorf("Code = %q, want %q", apiErr.Code, ErrCodePlacementQuotaExceeded)
	}
	if apiErr.Reason != "free_tests_used" || apiErr.StatusCode != http.StatusPaymentRequired {
		t.Errorf("Reason = %q, status %d", apiErr.Reason, apiErr.StatusCode)
	}
	if apiErr.Temporary() {
		t.Error("a 402 is not transient")
	}
}

// TestErrorCodeValues pins the wire value of codes whose constant names are
// easy to get wrong, across each area of the catalog.
func TestErrorCodeValues(t *testing.T) {
	cases := map[string]string{
		ErrCodeDuplicateQueryParameter:       "duplicate_query_parameter",
		ErrCodeDuplicateHeader:               "duplicate_header",
		ErrCodeAPIKeyPermissionsExceedCaller: "api_key_permissions_exceed_caller",
		ErrCodeAPIKeyHolderLeft:              "api_key_holder_left",
		ErrCodeAPIKeyMailboxLimited:          "api_key_mailbox_limited",
		ErrCodeOAuthTokenNotAllowed:          "oauth_token_not_allowed",
		ErrCodeMemberAccessRestricted:        "member_access_restricted",
		ErrCodeCRMReauthRequired:             "crm_reauth_required",
		ErrCodeMailboxImportTooLarge:         "mailbox_import_too_large",
		ErrCodePlacementTooManyRunning:       "placement_too_many_running",
		ErrCodeRecipientSuppressed:           "recipient_suppressed",
		ErrCodeSelectionTooLarge:             "selection_too_large",
		ErrCodeSessionEnded:                  "session_ended",
		ErrCodeTwoFAInvalidCode:              "two_fa_invalid_code",
		ErrCodeReauthRequired:                "reauth_required",
		ErrCodePoolLinkDenied:                "pool_link_denied",
		ErrCodeCloudLinkNotConnected:         "cloud_link_not_connected",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("code constant = %q, want %q", got, want)
		}
	}
}

// TestRateLimitReset parses the category limiter's X-RateLimit-Reset header.
func TestRateLimitReset(t *testing.T) {
	h := http.Header{}
	h.Set("X-RateLimit-Limit", "100")
	h.Set("X-RateLimit-Remaining", "0")
	h.Set("X-RateLimit-Reset", "1790000000")
	h.Set("Retry-After", "12")
	rl := parseRateLimit(h)
	if rl.Limit != 100 || rl.Remaining != 0 || rl.RetryAfter != 12*time.Second {
		t.Errorf("rate limit = %+v", rl)
	}
	if !rl.Reset.Equal(time.Unix(1790000000, 0)) {
		t.Errorf("Reset = %v", rl.Reset)
	}
	if got := parseRateLimit(http.Header{}).Reset; !got.IsZero() {
		t.Errorf("Reset = %v, want zero without the header", got)
	}
}

// TestListParamsNeverRepeatAKey: the server refuses a repeated query parameter
// except on the routes that read a list that way, so a list helper must send
// one comma-separated value.
func TestListParamsNeverRepeatAKey(t *testing.T) {
	q := url.Values{}
	setCSV(q, "ids", []string{"a", "", "b", "c"})
	if got := q["ids"]; len(got) != 1 || got[0] != "a,b,c" {
		t.Errorf("ids = %v, want one comma-separated value", got)
	}
	setCSV(q, "none", nil)
	if _, ok := q["none"]; ok {
		t.Error("an empty list should send nothing")
	}

	// The advisor's status filter is one of the routes that reads repeats.
	var raw string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"data":[],"pagination":{"has_more":false}}`))
	})
	if _, _, err := c.Advisor.Recommendations(context.Background(), &AdvisorListParams{Statuses: []string{"open", "snoozed"}}); err != nil {
		t.Fatalf("Recommendations: %v", err)
	}
	if strings.Count(raw, "status=") != 2 {
		t.Errorf("query = %q, want status repeated once per value on this route", raw)
	}
}

// TestSDKNeverSendsDuplicateHeaders: the server refuses a second
// Idempotency-Key or Authorization line, so overriding a default header must
// replace it.
func TestSDKNeverSendsDuplicateHeaders(t *testing.T) {
	var key []string
	var auth []string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		key = r.Header.Values("Idempotency-Key")
		auth = r.Header.Values("Authorization")
		_, _ = w.Write([]byte(`{"event_types":[]}`))
	})
	c.defaultHeaders.Set("Idempotency-Key", "default")
	if _, _, err := c.Webhooks.EventTypes(context.Background(), WithIdempotencyKey("mine")); err != nil {
		t.Fatalf("EventTypes: %v", err)
	}
	if len(key) != 1 || key[0] != "mine" {
		t.Errorf("Idempotency-Key lines = %v, want exactly the per-request one", key)
	}
	if len(auth) != 1 {
		t.Errorf("Authorization lines = %v, want exactly one", auth)
	}
}

// TestPermAppGrantable: an app may be granted everything except API-key
// management.
func TestPermAppGrantable(t *testing.T) {
	if PermAppGrantable&PermAPIKeys != 0 {
		t.Error("PermAppGrantable must not include PermAPIKeys")
	}
	if PermAppGrantable|PermAPIKeys != PermFullAccess {
		t.Errorf("PermAppGrantable|PermAPIKeys = %b, want PermFullAccess %b", PermAppGrantable|PermAPIKeys, PermFullAccess)
	}
}

// TestAuditCatalogue pins the wire values of the entity types added since the
// last sync.
func TestAuditCatalogue(t *testing.T) {
	cases := map[string]string{
		AuditEntityCampaignLead:       "campaign_lead",
		AuditEntityMailboxImport:      "mailbox_import",
		AuditEntityMailboxGrant:       "mailbox_grant",
		AuditEntityMailboxVendor:      "mailbox_vendor",
		AuditEntityDomainRedirect:     "domain_redirect",
		AuditEntityEmailImage:         "email_image",
		AuditEntityAIToolPolicy:       "ai_tool_policy",
		AuditEntityAppListing:         "app_listing",
		AuditEntityOAuthApplication:   "oauth_application",
		AuditEntityOAuthAuthorization: "oauth_authorization",
		AuditEntityPlacementTest:      "placement_test",
		AuditEntityPlacementBatch:     "placement_batch",
		AuditEntityPlacementMonitor:   "placement_monitor",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("entity = %q, want %q", got, want)
		}
	}
}

// TestWebhookPayloadShapes decodes the delivery bodies whose shape changed.
func TestWebhookPayloadShapes(t *testing.T) {
	t.Run("reply with a tagging verdict", func(t *testing.T) {
		var p ReplyReceivedPayload
		body := `{"contact_email":"jo@example.com","intent":"positive","confidence":0.8,"subject":"Re: hi","snippet":"yes","action_taken":"none",
			"thread_id":"th_1","email_account_id":"a_1","campaign_id":"c_1",
			"ai_labels":["Meeting","Interested"],"ai_intent":"scheduling","ai_intent_confidence":0.91,"ai_kind":"human_reply","ai_relevance":82,"ai_needs_review":false}`
		if err := json.Unmarshal([]byte(body), &p); err != nil {
			t.Fatal(err)
		}
		if !p.Tagged() || p.AIIntent != "scheduling" || len(p.AILabels) != 2 || *p.AIRelevance != 82 || *p.AINeedsReview {
			t.Errorf("decoded = %+v", p)
		}
		if p.ContactID != "" || p.CampaignID != "c_1" {
			t.Errorf("optional ids = %q %q", p.ContactID, p.CampaignID)
		}
	})

	t.Run("reply never judged by tagging", func(t *testing.T) {
		var p ReplyReceivedPayload
		if err := json.Unmarshal([]byte(`{"contact_email":"jo@example.com","thread_id":"th_1"}`), &p); err != nil {
			t.Fatal(err)
		}
		if p.Tagged() {
			t.Error("Tagged() should be false when the ai_* keys are absent")
		}
	})

	t.Run("click with origin", func(t *testing.T) {
		var p CampaignEngagementPayload
		body := `{"campaign_id":"c_1","contact_id":"ct_1","contact_email":"jo@example.com","sequence_id":"s_1","url":"https://example.com","link_label":"Book",
			"origin":{"client":"Gmail","client_type":"webmail","device_hidden":true,"os":"macOS","browser":"Safari","country_code":"GB"}}`
		if err := json.Unmarshal([]byte(body), &p); err != nil {
			t.Fatal(err)
		}
		if p.Origin == nil || p.Origin.ClientType != "webmail" || !p.Origin.DeviceHidden || p.Origin.OS != "macOS" {
			t.Errorf("origin = %+v", p.Origin)
		}
		var open CampaignEngagementPayload
		if err := json.Unmarshal([]byte(`{"campaign_id":"c_1","contact_id":"ct_1","contact_email":"jo@example.com","sequence_id":"s_1"}`), &open); err != nil {
			t.Fatal(err)
		}
		if open.Origin != nil {
			t.Error("origin should be absent when the open said nothing about itself")
		}
	})

	t.Run("placement in spam", func(t *testing.T) {
		var p WarmupPlacementInSpamPayload
		if err := json.Unmarshal([]byte(`{"email_account_id":"a_1","email":"ana@example.com","content_source":"library","recipient_provider":"smtp_imap","recipient_host":"zoho"}`), &p); err != nil {
			t.Fatal(err)
		}
		if p.RecipientHost != "zoho" || p.RecipientProvider != "smtp_imap" || p.Email != "ana@example.com" {
			t.Errorf("decoded = %+v", p)
		}
	})
}

// TestWebhookSignatureStillVerifies pins the scheme the server uses: the
// header is "t=<unix>,v1=<hex>" over "<unix>." plus the raw body.
func TestWebhookSignatureStillVerifies(t *testing.T) {
	body := []byte(`{"id":"e_1","event_type":"campaign.reply_received","organization_id":"o_1","created_at":"2026-10-01T09:00:00Z","data":{}}`)
	ts := time.Unix(1790000000, 0)
	header := ComputeWebhookSignature(body, "whsec_test", ts)
	if !strings.HasPrefix(header, "t=1790000000,v1=") {
		t.Fatalf("header = %q", header)
	}
	if !VerifyWebhookSignature(body, header, "whsec_test") {
		t.Error("a valid signature was rejected")
	}
	if VerifyWebhookSignature(append(body, ' '), header, "whsec_test") {
		t.Error("a modified body was accepted")
	}
	// A second scheme alongside v1 is ignored, not trusted.
	if !VerifyWebhookSignature(body, header+",v2=deadbeef", "whsec_test") {
		t.Error("an unknown extra scheme should be ignored")
	}
}
