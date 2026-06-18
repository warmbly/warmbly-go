package warmbly

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"testing"
)

func TestContactSearch(t *testing.T) {
	t.Run("with params", func(t *testing.T) {
		var gotMethod, gotPath string
		var body ContactSearchParams
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Request-ID", "req_search")
			_, _ = w.Write([]byte(`{"data":[{"id":"ct_1","email":"a@example.com"},{"id":"ct_2","email":"b@example.com"}],"pagination":{"has_more":true,"next_cursor":"CURSOR2"}}`))
		})

		subscribed := true
		params := &ContactSearchParams{
			Limit:      25,
			Cursor:     "CURSOR1",
			Query:      "acme",
			CampaignID: "camp_1",
			Status:     "active",
			Subscribed: &subscribed,
			Tags:       []string{"vip", "lead"},
		}
		page, err := c.Contacts.Search(context.Background(), params)
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if gotMethod != http.MethodPost {
			t.Errorf("method = %s, want POST", gotMethod)
		}
		if gotPath != "/v1/contacts/search" {
			t.Errorf("path = %s, want /v1/contacts/search", gotPath)
		}
		if body.Limit != 25 || body.Cursor != "CURSOR1" || body.Query != "acme" {
			t.Errorf("unexpected body pagination/query: %+v", body)
		}
		if body.CampaignID != "camp_1" || body.Status != "active" {
			t.Errorf("unexpected body filters: %+v", body)
		}
		if body.Subscribed == nil || !*body.Subscribed {
			t.Errorf("body.Subscribed = %v, want pointer to true", body.Subscribed)
		}
		if !reflect.DeepEqual(body.Tags, []string{"vip", "lead"}) {
			t.Errorf("body.Tags = %v", body.Tags)
		}

		if len(page.Data) != 2 || page.Data[0].ID != "ct_1" || page.Data[1].Email != "b@example.com" {
			t.Errorf("unexpected page data: %+v", page.Data)
		}
		if page.Response() == nil || page.Response().RequestID != "req_search" {
			t.Errorf("unexpected response: %+v", page.Response())
		}

		// Search pages are not auto-wired: Next must report ErrNoMorePages even
		// though the server reported has_more with a cursor.
		if !page.HasMore() {
			t.Fatal("HasMore() = false, want true for this fixture")
		}
		if _, err := page.Next(context.Background()); !errors.Is(err, ErrNoMorePages) {
			t.Errorf("Next err = %v, want ErrNoMorePages", err)
		}
	})

	t.Run("nil params defaults to empty struct", func(t *testing.T) {
		var raw map[string]any
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			data, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[],"pagination":{"has_more":false}}`))
		})

		page, err := c.Contacts.Search(context.Background(), nil)
		if err != nil {
			t.Fatalf("Search(nil): %v", err)
		}
		// All omitempty fields drop out, so the encoded body is "{}".
		if len(raw) != 0 {
			t.Errorf("body = %v, want empty object", raw)
		}
		if len(page.Data) != 0 {
			t.Errorf("page.Data = %v, want empty", page.Data)
		}
		if page.HasMore() {
			t.Error("HasMore() = true, want false")
		}
		if _, err := page.Next(context.Background()); !errors.Is(err, ErrNoMorePages) {
			t.Errorf("Next err = %v, want ErrNoMorePages", err)
		}
	})

	t.Run("error", func(t *testing.T) {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"bad","message":"nope","code":"invalid_request"}`))
		})
		page, err := c.Contacts.Search(context.Background(), &ContactSearchParams{Query: "x"})
		if err == nil {
			t.Fatal("expected error")
		}
		if page != nil {
			t.Errorf("page = %+v, want nil on error", page)
		}
	})
}

func TestContactCreate(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		var gotMethod, gotPath string
		var body struct {
			Contacts []ContactInput `json:"contacts"`
		}
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"created":2,"updated":1,"skipped":3}`))
		})

		inputs := []ContactInput{
			{Email: "a@example.com", FirstName: "A", Tags: []string{"new"}},
			{Email: "b@example.com", Company: "Acme"},
		}
		result, resp, err := c.Contacts.Create(context.Background(), inputs)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if gotMethod != http.MethodPost {
			t.Errorf("method = %s, want POST", gotMethod)
		}
		if gotPath != "/v1/contacts" {
			t.Errorf("path = %s, want /v1/contacts", gotPath)
		}
		if len(body.Contacts) != 2 || body.Contacts[0].Email != "a@example.com" || body.Contacts[1].Company != "Acme" {
			t.Errorf("unexpected request contacts: %+v", body.Contacts)
		}
		if result.Created != 2 || result.Updated != 1 || result.Skipped != 3 {
			t.Errorf("unexpected result: %+v", result)
		}
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("status = %d, want 201", resp.StatusCode)
		}
	})

	t.Run("error", func(t *testing.T) {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"error":"invalid","message":"email required","code":"validation_error"}`))
		})
		result, resp, err := c.Contacts.Create(context.Background(), []ContactInput{{}})
		if err == nil {
			t.Fatal("expected error")
		}
		if result != nil {
			t.Errorf("result = %+v, want nil", result)
		}
		if resp == nil || resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("resp = %+v, want non-nil with 422", resp)
		}
	})
}

func TestContactBulkUpdate(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		var gotMethod, gotPath string
		var body ContactBulkUpdateParams
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			w.WriteHeader(http.StatusNoContent)
		})

		subscribed := false
		params := &ContactBulkUpdateParams{
			IDs:        []string{"ct_1", "ct_2"},
			Subscribed: &subscribed,
			AddTags:    []string{"hot"},
			RemoveTags: []string{"cold"},
		}
		resp, err := c.Contacts.BulkUpdate(context.Background(), params)
		if err != nil {
			t.Fatalf("BulkUpdate: %v", err)
		}
		if gotMethod != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", gotMethod)
		}
		if gotPath != "/v1/contacts" {
			t.Errorf("path = %s, want /v1/contacts", gotPath)
		}
		if !reflect.DeepEqual(body.IDs, []string{"ct_1", "ct_2"}) {
			t.Errorf("body.IDs = %v", body.IDs)
		}
		if body.Subscribed == nil || *body.Subscribed {
			t.Errorf("body.Subscribed = %v, want pointer to false", body.Subscribed)
		}
		if !reflect.DeepEqual(body.AddTags, []string{"hot"}) || !reflect.DeepEqual(body.RemoveTags, []string{"cold"}) {
			t.Errorf("unexpected tags: add=%v remove=%v", body.AddTags, body.RemoveTags)
		}
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("status = %d, want 204", resp.StatusCode)
		}
	})

	t.Run("error", func(t *testing.T) {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"forbidden","message":"no","code":"forbidden"}`))
		})
		resp, err := c.Contacts.BulkUpdate(context.Background(), &ContactBulkUpdateParams{IDs: []string{"ct_1"}})
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("errors.Is(err, ErrForbidden) = false; err = %v", err)
		}
		if resp == nil || resp.StatusCode != http.StatusForbidden {
			t.Errorf("resp = %+v, want non-nil with 403", resp)
		}
	})
}

func TestContactBulkDelete(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		var gotMethod, gotPath string
		var body struct {
			IDs []string `json:"ids"`
		}
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			w.WriteHeader(http.StatusNoContent)
		})

		resp, err := c.Contacts.BulkDelete(context.Background(), []string{"ct_1", "ct_2", "ct_3"})
		if err != nil {
			t.Fatalf("BulkDelete: %v", err)
		}
		if gotMethod != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", gotMethod)
		}
		if gotPath != "/v1/contacts" {
			t.Errorf("path = %s, want /v1/contacts", gotPath)
		}
		if !reflect.DeepEqual(body.IDs, []string{"ct_1", "ct_2", "ct_3"}) {
			t.Errorf("body.IDs = %v", body.IDs)
		}
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("status = %d, want 204", resp.StatusCode)
		}
	})

	t.Run("error", func(t *testing.T) {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom","message":"server","code":"internal"}`))
		})
		resp, err := c.Contacts.BulkDelete(context.Background(), []string{"ct_1"})
		if err == nil {
			t.Fatal("expected error")
		}
		if resp == nil {
			t.Error("resp = nil, want non-nil")
		}
	})
}

func TestContactGet(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		var gotPath string
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.EscapedPath()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"ct 1","email":"a@example.com","subscribed":true}`))
		})

		// A space in the id must be path-escaped to %20.
		contact, resp, err := c.Contacts.Get(context.Background(), "ct 1")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if gotPath != "/v1/contacts/ct%201" {
			t.Errorf("path = %s, want /v1/contacts/ct%%201", gotPath)
		}
		if contact.ID != "ct 1" || contact.Email != "a@example.com" || !contact.Subscribed {
			t.Errorf("unexpected contact: %+v", contact)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d", resp.StatusCode)
		}
	})

	t.Run("error", func(t *testing.T) {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"nf","message":"gone","code":"resource_not_found"}`))
		})
		contact, resp, err := c.Contacts.Get(context.Background(), "missing")
		if err == nil {
			t.Fatal("expected error")
		}
		if contact != nil {
			t.Errorf("contact = %+v, want nil", contact)
		}
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("errors.Is(err, ErrNotFound) = false; err = %v", err)
		}
		if resp == nil || resp.StatusCode != http.StatusNotFound {
			t.Errorf("resp = %+v, want non-nil with 404", resp)
		}
	})
}

func TestContactUpdate(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		var gotMethod, gotPath string
		var body ContactUpdateParams
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"ct_1","first_name":"New","subscribed":false}`))
		})

		first := "New"
		subscribed := false
		fields := map[string]string{"plan": "pro"}
		tags := []string{"a", "b"}
		params := &ContactUpdateParams{
			FirstName:    &first,
			Subscribed:   &subscribed,
			CustomFields: &fields,
			Tags:         &tags,
		}
		contact, resp, err := c.Contacts.Update(context.Background(), "ct_1", params)
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if gotMethod != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", gotMethod)
		}
		if gotPath != "/v1/contacts/ct_1" {
			t.Errorf("path = %s", gotPath)
		}
		if body.FirstName == nil || *body.FirstName != "New" {
			t.Errorf("body.FirstName = %v", body.FirstName)
		}
		if body.Subscribed == nil || *body.Subscribed {
			t.Errorf("body.Subscribed = %v, want pointer to false", body.Subscribed)
		}
		if body.CustomFields == nil || (*body.CustomFields)["plan"] != "pro" {
			t.Errorf("body.CustomFields = %v", body.CustomFields)
		}
		if body.Tags == nil || !reflect.DeepEqual(*body.Tags, []string{"a", "b"}) {
			t.Errorf("body.Tags = %v", body.Tags)
		}
		if contact.FirstName != "New" || contact.Subscribed {
			t.Errorf("unexpected contact: %+v", contact)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d", resp.StatusCode)
		}
	})

	t.Run("error", func(t *testing.T) {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"conflict","message":"dup","code":"conflict"}`))
		})
		contact, resp, err := c.Contacts.Update(context.Background(), "ct_1", &ContactUpdateParams{})
		if err == nil {
			t.Fatal("expected error")
		}
		if contact != nil {
			t.Errorf("contact = %+v, want nil", contact)
		}
		if resp == nil || resp.StatusCode != http.StatusConflict {
			t.Errorf("resp = %+v, want non-nil with 409", resp)
		}
	})
}

func TestContactDelete(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		var gotMethod, gotPath string
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.EscapedPath()
			w.WriteHeader(http.StatusNoContent)
		})

		resp, err := c.Contacts.Delete(context.Background(), "ct/1")
		if err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if gotMethod != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", gotMethod)
		}
		if gotPath != "/v1/contacts/ct%2F1" {
			t.Errorf("path = %s, want escaped slash", gotPath)
		}
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("status = %d, want 204", resp.StatusCode)
		}
	})

	t.Run("error", func(t *testing.T) {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"nf","message":"gone","code":"resource_not_found"}`))
		})
		resp, err := c.Contacts.Delete(context.Background(), "missing")
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("errors.Is(err, ErrNotFound) = false; err = %v", err)
		}
		if resp == nil {
			t.Error("resp = nil, want non-nil")
		}
	})
}

func TestContactTimeline(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		var gotMethod, gotPath string
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"type":"open","description":"Opened email","occurred_at":"2026-01-02T03:04:05Z","metadata":{"campaign":"camp_1"}},{"type":"click","description":"Clicked link"}]}`))
		})

		events, resp, err := c.Contacts.Timeline(context.Background(), "ct_1")
		if err != nil {
			t.Fatalf("Timeline: %v", err)
		}
		if gotMethod != http.MethodGet {
			t.Errorf("method = %s, want GET", gotMethod)
		}
		if gotPath != "/v1/contacts/ct_1/timeline" {
			t.Errorf("path = %s", gotPath)
		}
		if len(events) != 2 || events[0].Type != "open" || events[1].Type != "click" {
			t.Errorf("unexpected events: %+v", events)
		}
		if events[0].Metadata["campaign"] != "camp_1" {
			t.Errorf("metadata = %v", events[0].Metadata)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d", resp.StatusCode)
		}
	})

	t.Run("error", func(t *testing.T) {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"nf","message":"gone","code":"resource_not_found"}`))
		})
		events, resp, err := c.Contacts.Timeline(context.Background(), "missing")
		if err == nil {
			t.Fatal("expected error")
		}
		if events != nil {
			t.Errorf("events = %+v, want nil", events)
		}
		if resp == nil {
			t.Error("resp = nil, want non-nil")
		}
	})
}

func TestContactNotes(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		var gotMethod, gotPath string
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"note_1","body":"Called","author_id":"usr_1","created_at":"2026-01-02T03:04:05Z"}]}`))
		})

		notes, resp, err := c.Contacts.Notes(context.Background(), "ct_1")
		if err != nil {
			t.Fatalf("Notes: %v", err)
		}
		if gotMethod != http.MethodGet {
			t.Errorf("method = %s, want GET", gotMethod)
		}
		if gotPath != "/v1/contacts/ct_1/notes" {
			t.Errorf("path = %s", gotPath)
		}
		if len(notes) != 1 || notes[0].ID != "note_1" || notes[0].Body != "Called" || notes[0].AuthorID != "usr_1" {
			t.Errorf("unexpected notes: %+v", notes)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d", resp.StatusCode)
		}
	})

	t.Run("error", func(t *testing.T) {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauth","message":"nope","code":"unauthorized"}`))
		})
		notes, resp, err := c.Contacts.Notes(context.Background(), "ct_1")
		if err == nil {
			t.Fatal("expected error")
		}
		if notes != nil {
			t.Errorf("notes = %+v, want nil", notes)
		}
		if resp == nil {
			t.Error("resp = nil, want non-nil")
		}
	})
}

func TestContactAddNote(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		var gotMethod, gotPath string
		var body struct {
			Body string `json:"body"`
		}
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"note_1","body":"Followed up","author_id":"usr_1"}`))
		})

		note, resp, err := c.Contacts.AddNote(context.Background(), "ct_1", "Followed up")
		if err != nil {
			t.Fatalf("AddNote: %v", err)
		}
		if gotMethod != http.MethodPost {
			t.Errorf("method = %s, want POST", gotMethod)
		}
		if gotPath != "/v1/contacts/ct_1/notes" {
			t.Errorf("path = %s", gotPath)
		}
		if body.Body != "Followed up" {
			t.Errorf("body.Body = %q", body.Body)
		}
		if note.ID != "note_1" || note.Body != "Followed up" {
			t.Errorf("unexpected note: %+v", note)
		}
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("status = %d, want 201", resp.StatusCode)
		}
	})

	t.Run("error", func(t *testing.T) {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"error":"invalid","message":"empty","code":"validation_error"}`))
		})
		note, resp, err := c.Contacts.AddNote(context.Background(), "ct_1", "")
		if err == nil {
			t.Fatal("expected error")
		}
		if note != nil {
			t.Errorf("note = %+v, want nil", note)
		}
		if resp == nil || resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("resp = %+v, want non-nil with 422", resp)
		}
	})
}
