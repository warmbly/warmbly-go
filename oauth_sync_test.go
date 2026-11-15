package warmbly

import (
	"context"
	"net/http"
	"testing"
)

func TestOAuthWorkspaceAuthorizations(t *testing.T) {
	var got recordedRequest
	c := agentToolServer(t, http.StatusOK, `{"authorizations": [
		{"application_id": "app_1", "name": "Pipeline Sync", "logo_url": "https://cdn.example/l.png",
		 "website_url": "https://pipeline.example", "user_id": "usr_1", "user_email": "sam@acme.test",
		 "user_name": "Sam", "scopes": 6, "authorized_at": "2026-10-01T10:00:00Z", "last_used_at": "2026-11-14T08:00:00Z"},
		{"application_id": "app_2", "name": "Notes", "logo_url": "", "website_url": "", "user_id": "usr_2",
		 "user_email": "ana@acme.test", "user_name": "", "scopes": 1, "authorized_at": "2026-11-01T10:00:00Z"}
	]}`, &got)

	auths, _, err := c.OAuthApps.WorkspaceAuthorizations(context.Background())
	if err != nil {
		t.Fatalf("WorkspaceAuthorizations: %v", err)
	}
	checkRequest(t, got, "GET", "/v1/oauth/workspace-authorizations", "", "")
	if len(auths) != 2 {
		t.Fatalf("authorizations = %d", len(auths))
	}
	if a := auths[0]; a.ApplicationID != "app_1" || a.UserID != "usr_1" || a.UserEmail != "sam@acme.test" || a.Scopes != 6 || a.LastUsedAt == nil {
		t.Errorf("auth 0 = %+v", a)
	}
	if a := auths[1]; a.LastUsedAt != nil || a.AuthorizedAt.IsZero() {
		t.Errorf("auth 1 = %+v", a)
	}

	c = agentToolServer(t, http.StatusOK, `{"authorizations": null}`, nil)
	auths, _, err = c.OAuthApps.WorkspaceAuthorizations(context.Background())
	if err != nil || auths == nil || len(auths) != 0 {
		t.Errorf("empty authorizations = %#v, %v", auths, err)
	}
}

func TestOAuthRevokeWorkspaceAuthorization(t *testing.T) {
	var got recordedRequest
	c := agentToolServer(t, http.StatusOK, `{"revoked": true}`, &got)

	if _, err := c.OAuthApps.RevokeWorkspaceAuthorization(context.Background(), "app_1", "usr_1"); err != nil {
		t.Fatalf("RevokeWorkspaceAuthorization: %v", err)
	}
	checkRequest(t, got, "DELETE", "/v1/oauth/workspace-authorizations/app_1/members/usr_1", "", "")
}
