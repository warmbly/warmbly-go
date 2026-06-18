package warmbly

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestEmailsWarmupEnabled(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name  string
		email Email
		want  bool
	}{
		{"set", Email{Warmup: &now}, true},
		{"nil", Email{Warmup: nil}, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.email.WarmupEnabled(); got != tt.want {
				t.Errorf("WarmupEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEmailsListParamsValues(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var p *EmailListParams
		q := p.values()
		if len(q) != 0 {
			t.Errorf("nil params produced %v, want empty", q)
		}
	})

	t.Run("all fields", func(t *testing.T) {
		p := &EmailListParams{
			ListOptions: ListOptions{Limit: 25, Cursor: "CUR"},
			Search:      "alice",
			Provider:    "gmail",
			Status:      "active",
			Tag:         "vip",
		}
		q := p.values()
		want := map[string]string{
			"limit":    "25",
			"cursor":   "CUR",
			"search":   "alice",
			"provider": "gmail",
			"status":   "active",
			"tag":      "vip",
		}
		for k, v := range want {
			if got := q.Get(k); got != v {
				t.Errorf("values()[%q] = %q, want %q", k, got, v)
			}
		}
		if len(q) != len(want) {
			t.Errorf("values() has %d keys, want %d: %v", len(q), len(want), q)
		}
	})

	t.Run("empty fields omitted", func(t *testing.T) {
		p := &EmailListParams{}
		q := p.values()
		if len(q) != 0 {
			t.Errorf("empty params produced %v, want empty", q)
		}
	})
}

func TestEmailsList(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/v1/emails" {
			t.Errorf("path = %q, want /v1/emails", r.URL.Path)
		}
		if got := r.URL.Query().Get("provider"); got != "gmail" {
			t.Errorf("provider query = %q, want gmail", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "req_list")
		_, _ = w.Write([]byte(`{"data":[{"id":"email_1","email":"a@x.com"},{"id":"email_2","email":"b@x.com"}],"pagination":{"has_more":false,"next_cursor":null}}`))
	})

	page, err := c.Emails.List(context.Background(), &EmailListParams{Provider: "gmail"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(page.Data) != 2 {
		t.Fatalf("len(Data) = %d, want 2", len(page.Data))
	}
	if page.Data[0].ID != "email_1" || page.Data[1].Email != "b@x.com" {
		t.Errorf("unexpected data: %+v", page.Data)
	}
	if resp := page.Response(); resp == nil || resp.RequestID != "req_list" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestEmailsListNilParams(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("expected no query, got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"pagination":{"has_more":false,"next_cursor":null}}`))
	})

	page, err := c.Emails.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(page.Data) != 0 {
		t.Errorf("len(Data) = %d, want 0", len(page.Data))
	}
}

func TestEmailsGet(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("method = %s, want GET", r.Method)
			}
			if r.URL.Path != "/v1/emails/email 1" {
				t.Errorf("decoded path = %q", r.URL.Path)
			}
			if r.URL.EscapedPath() != "/v1/emails/email%201" {
				t.Errorf("escaped path = %q, want id percent-encoded", r.URL.EscapedPath())
			}
			w.Header().Set("X-Request-ID", "req_get")
			_, _ = w.Write([]byte(`{"id":"email 1","email":"a@x.com","name":"Alice"}`))
		})

		email, resp, err := c.Emails.Get(context.Background(), "email 1")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if email.ID != "email 1" || email.Name != "Alice" {
			t.Errorf("unexpected email: %+v", email)
		}
		if resp.StatusCode != http.StatusOK || resp.RequestID != "req_get" {
			t.Errorf("unexpected response: %+v", resp)
		}
	})

	t.Run("error", func(t *testing.T) {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not_found","message":"missing","code":"resource_not_found"}`))
		})

		email, resp, err := c.Emails.Get(context.Background(), "nope")
		if err == nil {
			t.Fatal("expected error")
		}
		if email != nil {
			t.Errorf("email = %+v, want nil", email)
		}
		if resp == nil || resp.StatusCode != http.StatusNotFound {
			t.Errorf("unexpected response: %+v", resp)
		}
	})
}

func TestEmailsUpdate(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		var body map[string]any
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPatch {
				t.Errorf("method = %s, want PATCH", r.Method)
			}
			if r.URL.Path != "/v1/emails/email_1" {
				t.Errorf("path = %q", r.URL.Path)
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"email_1","name":"Renamed"}`))
		})

		name := "Renamed"
		email, resp, err := c.Emails.Update(context.Background(), "email_1", &EmailUpdateParams{Name: &name})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if email.Name != "Renamed" {
			t.Errorf("name = %q, want Renamed", email.Name)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d", resp.StatusCode)
		}
		if body["name"] != "Renamed" {
			t.Errorf("request body name = %v, want Renamed", body["name"])
		}
	})

	t.Run("error", func(t *testing.T) {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"bad","message":"nope","code":"validation_error"}`))
		})

		email, resp, err := c.Emails.Update(context.Background(), "email_1", &EmailUpdateParams{})
		if err == nil {
			t.Fatal("expected error")
		}
		if email != nil {
			t.Errorf("email = %+v, want nil", email)
		}
		if resp == nil || resp.StatusCode != http.StatusBadRequest {
			t.Errorf("unexpected response: %+v", resp)
		}
	})
}

func TestEmailsDelete(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		if r.URL.Path != "/v1/emails/email_1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	resp, err := c.Emails.Delete(context.Background(), "email_1")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want 204", resp.StatusCode)
	}
}

func TestEmailsSend(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		var body SendEmailParams
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", r.Method)
			}
			if r.URL.Path != "/v1/emails/email_1/send" {
				t.Errorf("path = %q, want .../send", r.URL.Path)
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"message_id":"msg_1","queued_at":"2026-06-18T10:00:00Z"}`))
		})

		result, resp, err := c.Emails.Send(context.Background(), "email_1", &SendEmailParams{
			To:      []string{"to@x.com"},
			Subject: "Hi",
		})
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		if result.MessageID != "msg_1" {
			t.Errorf("message_id = %q, want msg_1", result.MessageID)
		}
		if result.QueuedAt.IsZero() {
			t.Error("queued_at not decoded")
		}
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("status = %d", resp.StatusCode)
		}
		if len(body.To) != 1 || body.To[0] != "to@x.com" || body.Subject != "Hi" {
			t.Errorf("unexpected request body: %+v", body)
		}
	})

	t.Run("error", func(t *testing.T) {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"error":"bad","message":"no recipients","code":"validation_error"}`))
		})

		result, resp, err := c.Emails.Send(context.Background(), "email_1", &SendEmailParams{})
		if err == nil {
			t.Fatal("expected error")
		}
		if result != nil {
			t.Errorf("result = %+v, want nil", result)
		}
		if resp == nil || resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("unexpected response: %+v", resp)
		}
	})
}

func TestEmailsWarmupTransitions(t *testing.T) {
	warmupAt := "2026-06-18T09:00:00Z"
	cases := []struct {
		name     string
		wantPath string
		call     func(c *Client) (*Email, *Response, error)
	}{
		{
			name:     "start",
			wantPath: "/v1/emails/email_1/warmup/start",
			call: func(c *Client) (*Email, *Response, error) {
				return c.Emails.StartWarmup(context.Background(), "email_1")
			},
		},
		{
			name:     "pause",
			wantPath: "/v1/emails/email_1/warmup/pause",
			call: func(c *Client) (*Email, *Response, error) {
				return c.Emails.PauseWarmup(context.Background(), "email_1")
			},
		},
		{
			name:     "resume",
			wantPath: "/v1/emails/email_1/warmup/resume",
			call: func(c *Client) (*Email, *Response, error) {
				return c.Emails.ResumeWarmup(context.Background(), "email_1")
			},
		},
		{
			name:     "stop",
			wantPath: "/v1/emails/email_1/warmup/stop",
			call: func(c *Client) (*Email, *Response, error) {
				return c.Emails.StopWarmup(context.Background(), "email_1")
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("method = %s, want POST", r.Method)
				}
				if r.URL.Path != tt.wantPath {
					t.Errorf("path = %q, want %q", r.URL.Path, tt.wantPath)
				}
				_, _ = w.Write([]byte(`{"id":"email_1","warmup":"` + warmupAt + `"}`))
			})

			email, resp, err := tt.call(c)
			if err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
			if email.ID != "email_1" {
				t.Errorf("id = %q, want email_1", email.ID)
			}
			if !email.WarmupEnabled() {
				t.Error("WarmupEnabled() = false, want true")
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("status = %d", resp.StatusCode)
			}
		})
	}
}

func TestEmailsWarmupTransitionsError(t *testing.T) {
	cases := []struct {
		name string
		call func(c *Client) (*Email, *Response, error)
	}{
		{"start", func(c *Client) (*Email, *Response, error) {
			return c.Emails.StartWarmup(context.Background(), "email_1")
		}},
		{"pause", func(c *Client) (*Email, *Response, error) {
			return c.Emails.PauseWarmup(context.Background(), "email_1")
		}},
		{"resume", func(c *Client) (*Email, *Response, error) {
			return c.Emails.ResumeWarmup(context.Background(), "email_1")
		}},
		{"stop", func(c *Client) (*Email, *Response, error) {
			return c.Emails.StopWarmup(context.Background(), "email_1")
		}},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"error":"conflict","message":"bad state","code":"conflict"}`))
			})

			email, resp, err := tt.call(c)
			if err == nil {
				t.Fatal("expected error")
			}
			if email != nil {
				t.Errorf("email = %+v, want nil", email)
			}
			if resp == nil || resp.StatusCode != http.StatusConflict {
				t.Errorf("unexpected response: %+v", resp)
			}
		})
	}
}

func TestEmailsWarmupBanStatus(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("method = %s, want GET", r.Method)
			}
			if r.URL.Path != "/v1/emails/email_1/warmup/ban-status" {
				t.Errorf("path = %q", r.URL.Path)
			}
			_, _ = w.Write([]byte(`{"banned":true,"reason":"spam","since":"2026-06-01T00:00:00Z","appealable":true}`))
		})

		status, resp, err := c.Emails.WarmupBanStatus(context.Background(), "email_1")
		if err != nil {
			t.Fatalf("WarmupBanStatus: %v", err)
		}
		if !status.Banned || status.Reason != "spam" || !status.Appealable {
			t.Errorf("unexpected status: %+v", status)
		}
		if status.Since == nil {
			t.Error("since not decoded")
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d", resp.StatusCode)
		}
	})

	t.Run("error", func(t *testing.T) {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"forbidden","message":"no","code":"forbidden"}`))
		})

		status, resp, err := c.Emails.WarmupBanStatus(context.Background(), "email_1")
		if err == nil {
			t.Fatal("expected error")
		}
		if status != nil {
			t.Errorf("status = %+v, want nil", status)
		}
		if resp == nil || resp.StatusCode != http.StatusForbidden {
			t.Errorf("unexpected response: %+v", resp)
		}
	})
}

func TestEmailsAppealWarmupBan(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		var body map[string]any
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", r.Method)
			}
			if r.URL.Path != "/v1/emails/email_1/warmup/appeal" {
				t.Errorf("path = %q", r.URL.Path)
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			w.WriteHeader(http.StatusNoContent)
		})

		resp, err := c.Emails.AppealWarmupBan(context.Background(), "email_1", &WarmupAppealParams{Message: "please"})
		if err != nil {
			t.Fatalf("AppealWarmupBan: %v", err)
		}
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("status = %d, want 204", resp.StatusCode)
		}
		if body["message"] != "please" {
			t.Errorf("request body message = %v, want please", body["message"])
		}
	})

	t.Run("error", func(t *testing.T) {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"bad","message":"not appealable","code":"validation_error"}`))
		})

		resp, err := c.Emails.AppealWarmupBan(context.Background(), "email_1", &WarmupAppealParams{Message: "x"})
		if err == nil {
			t.Fatal("expected error")
		}
		if resp == nil || resp.StatusCode != http.StatusBadRequest {
			t.Errorf("unexpected response: %+v", resp)
		}
	})
}
