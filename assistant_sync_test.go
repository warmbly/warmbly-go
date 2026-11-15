package warmbly

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestAssistantToolPolicies(t *testing.T) {
	var got recordedRequest
	c := agentToolServer(t, http.StatusOK, `{"data": [
		{"org_id": "org_1", "tool_name": "add_tag", "decision": "always_allow", "created_by": "usr_1",
		 "created_by_name": "Sam", "created_at": "2026-11-01T10:00:00Z"},
		{"org_id": "org_1", "tool_name": "update_contact_fields", "decision": "always_allow",
		 "created_at": "2026-11-02T10:00:00Z"}
	]}`, &got)

	policies, _, err := c.Assistant.ToolPolicies(context.Background())
	if err != nil {
		t.Fatalf("ToolPolicies: %v", err)
	}
	checkRequest(t, got, "GET", "/v1/ai/tool-policies", "", "")
	if len(policies) != 2 {
		t.Fatalf("policies = %d", len(policies))
	}
	if p := policies[0]; p.ToolName != "add_tag" || p.Decision != ApprovalAlwaysAllow || p.CreatedBy == nil || *p.CreatedBy != "usr_1" || p.CreatedByName != "Sam" {
		t.Errorf("policy 0 = %+v", p)
	}
	if p := policies[1]; p.CreatedBy != nil || p.CreatedByName != "" || p.CreatedAt.IsZero() {
		t.Errorf("policy 1 = %+v", p)
	}
}

func TestAssistantRevokeToolPolicy(t *testing.T) {
	var got recordedRequest
	c := agentToolServer(t, http.StatusOK, `{"message": "policy revoked"}`, &got)

	if _, err := c.Assistant.RevokeToolPolicy(context.Background(), "add_tag"); err != nil {
		t.Fatalf("RevokeToolPolicy: %v", err)
	}
	checkRequest(t, got, "DELETE", "/v1/ai/tool-policies/add_tag", "", "")

	c = agentToolServer(t, http.StatusNotFound, `{"error": "not_found", "message": "not found"}`, nil)
	_, err := c.Assistant.RevokeToolPolicy(context.Background(), "send_email")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("err = %v", err)
	}
}
