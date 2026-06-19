package warmbly

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
)

func TestOrgCreate(t *testing.T) {
	var gotMethod, gotPath string
	var body map[string]any
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("X-Request-ID", "req_create")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"org_1","name":"Acme","slug":"acme","owner_user_id":"usr_1","created_at":"2026-01-02T03:04:05Z"}`))
	})

	org, resp, err := c.Organization.Create(context.Background(), &OrganizationCreateParams{Name: "Acme", Slug: "acme"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/v1/organization" {
		t.Errorf("path = %q, want /v1/organization", gotPath)
	}
	if body["name"] != "Acme" || body["slug"] != "acme" {
		t.Errorf("body = %v", body)
	}
	if org.ID != "org_1" || org.Name != "Acme" {
		t.Errorf("org = %+v", org)
	}
	if org.Slug == nil || *org.Slug != "acme" {
		t.Errorf("slug = %v", org.Slug)
	}
	if org.OwnerUserID != "usr_1" {
		t.Errorf("owner = %q", org.OwnerUserID)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d", resp.StatusCode)
	}
	if resp.RequestID != "req_create" {
		t.Errorf("request id = %q", resp.RequestID)
	}
}

func TestOrgCreateError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid","message":"bad request","code":"bad_request"}`))
	})

	org, resp, err := c.Organization.Create(context.Background(), &OrganizationCreateParams{Name: "Acme"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if org != nil {
		t.Errorf("org = %+v, want nil", org)
	}
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As(*Error) failed; err = %v", err)
	}
	if resp == nil || resp.StatusCode != http.StatusBadRequest {
		t.Errorf("resp = %+v", resp)
	}
}

func TestOrgList(t *testing.T) {
	var gotMethod, gotPath string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("X-RateLimit-Limit", "100")
		w.Header().Set("X-RateLimit-Remaining", "97")
		_, _ = w.Write([]byte(`{"data":[{"id":"org_1","name":"One","owner_user_id":"usr_1","created_at":"2026-01-02T03:04:05Z"},{"id":"org_2","name":"Two","owner_user_id":"usr_2","created_at":"2026-01-02T03:04:05Z"}]}`))
	})

	orgs, resp, err := c.Organization.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %s, want GET", gotMethod)
	}
	if gotPath != "/v1/organization" {
		t.Errorf("path = %q, want /v1/organization", gotPath)
	}
	if len(orgs) != 2 || orgs[0].ID != "org_1" || orgs[1].Name != "Two" {
		t.Errorf("orgs = %+v", orgs)
	}
	if resp.RateLimit.Limit != 100 || resp.RateLimit.Remaining != 97 {
		t.Errorf("rate limit = %+v", resp.RateLimit)
	}
}

func TestOrgListError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized","message":"nope","code":"unauthorized"}`))
	})

	orgs, _, err := c.Organization.List(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if orgs != nil {
		t.Errorf("orgs = %+v, want nil", orgs)
	}
}

func TestOrgCurrent(t *testing.T) {
	var gotMethod, gotPath string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_, _ = w.Write([]byte(`{"id":"org_cur","name":"Current","owner_user_id":"usr_1","created_at":"2026-01-02T03:04:05Z"}`))
	})

	org, _, err := c.Organization.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %s, want GET", gotMethod)
	}
	if gotPath != "/v1/organization/current" {
		t.Errorf("path = %q, want /v1/organization/current", gotPath)
	}
	if org.ID != "org_cur" || org.Name != "Current" {
		t.Errorf("org = %+v", org)
	}
	if org.Slug != nil {
		t.Errorf("slug = %v, want nil", org.Slug)
	}
}

func TestOrgCurrentError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not_found","message":"gone","code":"resource_not_found"}`))
	})

	org, _, err := c.Organization.Current(context.Background())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("errors.Is(err, ErrNotFound) = false; err = %v", err)
	}
	if org != nil {
		t.Errorf("org = %+v, want nil", org)
	}
}

func TestOrgUpdate(t *testing.T) {
	var gotMethod, gotPath string
	var body map[string]any
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"id":"org_cur","name":"Renamed","slug":"renamed","owner_user_id":"usr_1","created_at":"2026-01-02T03:04:05Z"}`))
	})

	name := "Renamed"
	slug := "renamed"
	org, _, err := c.Organization.Update(context.Background(), &OrganizationUpdateParams{Name: &name, Slug: &slug})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", gotMethod)
	}
	if gotPath != "/v1/organization/current" {
		t.Errorf("path = %q, want /v1/organization/current", gotPath)
	}
	if body["name"] != "Renamed" || body["slug"] != "renamed" {
		t.Errorf("body = %v", body)
	}
	if org.Name != "Renamed" {
		t.Errorf("org name = %q", org.Name)
	}
}

func TestOrgUpdateError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden","message":"no","code":"forbidden"}`))
	})

	name := "Renamed"
	org, _, err := c.Organization.Update(context.Background(), &OrganizationUpdateParams{Name: &name})
	if err == nil {
		t.Fatal("expected an error")
	}
	if org != nil {
		t.Errorf("org = %+v, want nil", org)
	}
}

func TestOrgMembers(t *testing.T) {
	var gotMethod, gotPath string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_, _ = w.Write([]byte(`{"data":[{"id":"mem_1","user_id":"usr_1","email":"a@example.com","first_name":"A","last_name":"Lpha","role":"admin","status":"active","created_at":"2026-01-02T03:04:05Z"},{"id":"mem_2","user_id":"usr_2","email":"b@example.com","first_name":"B","last_name":"Eta","role":"member","status":"invited","created_at":"2026-01-02T03:04:05Z"}]}`))
	})

	members, _, err := c.Organization.Members(context.Background())
	if err != nil {
		t.Fatalf("Members: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %s, want GET", gotMethod)
	}
	if gotPath != "/v1/organization/members" {
		t.Errorf("path = %q, want /v1/organization/members", gotPath)
	}
	if len(members) != 2 || members[0].Email != "a@example.com" || members[1].Role != "member" {
		t.Errorf("members = %+v", members)
	}
}

func TestOrgMembersError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom","message":"server","code":"server_error"}`))
	})

	members, _, err := c.Organization.Members(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if members != nil {
		t.Errorf("members = %+v, want nil", members)
	}
}

func TestOrgInvite(t *testing.T) {
	var gotMethod, gotPath string
	var body map[string]any
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"mem_new","user_id":"usr_9","email":"new@example.com","first_name":"","last_name":"","role":"member","status":"invited","created_at":"2026-01-02T03:04:05Z"}`))
	})

	member, resp, err := c.Organization.Invite(context.Background(), &InviteMemberParams{Email: "new@example.com", Role: "member"})
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/v1/organization/members/invite" {
		t.Errorf("path = %q, want /v1/organization/members/invite", gotPath)
	}
	if body["email"] != "new@example.com" || body["role"] != "member" {
		t.Errorf("body = %v", body)
	}
	if member.ID != "mem_new" || member.Status != "invited" {
		t.Errorf("member = %+v", member)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestOrgInviteError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"conflict","message":"already a member","code":"conflict"}`))
	})

	member, _, err := c.Organization.Invite(context.Background(), &InviteMemberParams{Email: "dup@example.com", Role: "member"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if member != nil {
		t.Errorf("member = %+v, want nil", member)
	}
}

func TestOrgUpdateMember(t *testing.T) {
	var gotMethod, gotPath string
	var body map[string]any
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.EscapedPath()
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"id":"mem 1/ä","user_id":"usr_1","email":"a@example.com","first_name":"A","last_name":"Lpha","role":"admin","status":"active","created_at":"2026-01-02T03:04:05Z"}`))
	})

	role := "admin"
	member, _, err := c.Organization.UpdateMember(context.Background(), "mem 1/ä", &UpdateMemberParams{Role: &role})
	if err != nil {
		t.Fatalf("UpdateMember: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", gotMethod)
	}
	// The id is path-escaped by url.PathEscape, so the server sees the escaped form.
	if gotPath != "/v1/organization/members/mem%201%2F%C3%A4" {
		t.Errorf("path = %q, want escaped id", gotPath)
	}
	if body["role"] != "admin" {
		t.Errorf("body = %v", body)
	}
	if member.Role != "admin" {
		t.Errorf("member role = %q", member.Role)
	}
}

func TestOrgUpdateMemberError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not_found","message":"no member","code":"resource_not_found"}`))
	})

	role := "admin"
	member, _, err := c.Organization.UpdateMember(context.Background(), "mem_x", &UpdateMemberParams{Role: &role})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("errors.Is(err, ErrNotFound) = false; err = %v", err)
	}
	if member != nil {
		t.Errorf("member = %+v, want nil", member)
	}
}

func TestOrgRemoveMember(t *testing.T) {
	var gotMethod, gotPath string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("X-Request-ID", "req_del")
		w.WriteHeader(http.StatusNoContent)
	})

	resp, err := c.Organization.RemoveMember(context.Background(), "mem_1")
	if err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", gotMethod)
	}
	if gotPath != "/v1/organization/members/mem_1" {
		t.Errorf("path = %q, want /v1/organization/members/mem_1", gotPath)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d", resp.StatusCode)
	}
	if resp.RequestID != "req_del" {
		t.Errorf("request id = %q", resp.RequestID)
	}
	// Drain to keep the connection reusable; the body should be empty.
	if resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}

func TestOrgRemoveMemberError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden","message":"cannot remove owner","code":"forbidden"}`))
	})

	resp, err := c.Organization.RemoveMember(context.Background(), "mem_owner")
	if err == nil {
		t.Fatal("expected an error")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Errorf("resp = %+v", resp)
	}
}
