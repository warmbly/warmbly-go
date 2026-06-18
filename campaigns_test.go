package warmbly

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

func TestCampaignListParamsValues(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var p *CampaignListParams
		q := p.values()
		if len(q) != 0 {
			t.Errorf("nil params produced %d query keys, want 0: %v", len(q), q)
		}
	})

	t.Run("all set", func(t *testing.T) {
		p := &CampaignListParams{
			ListOptions: ListOptions{Limit: 25, Cursor: "cur_1"},
			Search:      "launch",
			Status:      "running",
		}
		q := p.values()
		if got := q.Get("limit"); got != "25" {
			t.Errorf("limit = %q, want 25", got)
		}
		if got := q.Get("cursor"); got != "cur_1" {
			t.Errorf("cursor = %q, want cur_1", got)
		}
		if got := q.Get("search"); got != "launch" {
			t.Errorf("search = %q, want launch", got)
		}
		if got := q.Get("status"); got != "running" {
			t.Errorf("status = %q, want running", got)
		}
	})

	t.Run("empty filters omitted", func(t *testing.T) {
		p := &CampaignListParams{}
		q := p.values()
		if _, ok := q["search"]; ok {
			t.Errorf("empty Search should not set search key: %v", q)
		}
		if _, ok := q["status"]; ok {
			t.Errorf("empty Status should not set status key: %v", q)
		}
	})
}

func TestCampaignUpdate(t *testing.T) {
	var gotMethod, gotPath string
	var body map[string]any
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("X-Request-ID", "req_upd")
		_, _ = w.Write([]byte(`{"id":"camp_1","name":"Renamed","status":"draft"}`))
	})

	name := "Renamed"
	camp, resp, err := c.Campaigns.Update(context.Background(), "camp_1", &CampaignUpdateParams{Name: &name})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", gotMethod)
	}
	if gotPath != "/v1/campaigns/camp_1" {
		t.Errorf("path = %q", gotPath)
	}
	if body["name"] != "Renamed" {
		t.Errorf("body name = %v", body["name"])
	}
	if camp.Name != "Renamed" {
		t.Errorf("returned name = %q", camp.Name)
	}
	if resp.RequestID != "req_upd" {
		t.Errorf("RequestID = %q", resp.RequestID)
	}
}

func TestCampaignDelete(t *testing.T) {
	var gotMethod, gotPath string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	})

	resp, err := c.Campaigns.Delete(context.Background(), "camp_1")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", gotMethod)
	}
	if gotPath != "/v1/campaigns/camp_1" {
		t.Errorf("path = %q", gotPath)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestCampaignStart(t *testing.T) {
	var gotMethod, gotPath string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"id":"camp_1","status":"running"}`))
	})

	camp, resp, err := c.Campaigns.Start(context.Background(), "camp_1")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/v1/campaigns/camp_1/start" {
		t.Errorf("path = %q", gotPath)
	}
	if camp.Status != "running" {
		t.Errorf("status = %q", camp.Status)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status code = %d", resp.StatusCode)
	}
}

func TestCampaignStop(t *testing.T) {
	var gotMethod, gotPath string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"id":"camp_1","status":"stopped"}`))
	})

	camp, _, err := c.Campaigns.Stop(context.Background(), "camp_1")
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/v1/campaigns/camp_1/stop" {
		t.Errorf("path = %q", gotPath)
	}
	if camp.Status != "stopped" {
		t.Errorf("status = %q", camp.Status)
	}
}

func TestCampaignLogs(t *testing.T) {
	var gotMethod, gotPath string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"log_1","level":"info","message":"sent"},{"id":"log_2","level":"error","message":"bounce"}],"pagination":{"has_more":false,"next_cursor":null}}`))
	})

	page, err := c.Campaigns.Logs(context.Background(), "camp_1", nil)
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %s, want GET", gotMethod)
	}
	if gotPath != "/v1/campaigns/camp_1/logs" {
		t.Errorf("path = %q", gotPath)
	}
	if len(page.Data) != 2 {
		t.Fatalf("got %d entries, want 2", len(page.Data))
	}
	if page.Data[0].ID != "log_1" || page.Data[0].Level != "info" || page.Data[0].Message != "sent" {
		t.Errorf("entry 0 = %+v", page.Data[0])
	}
	if page.Data[1].Level != "error" {
		t.Errorf("entry 1 level = %q", page.Data[1].Level)
	}
	if page.HasMore() {
		t.Errorf("HasMore should be false")
	}
}

func TestCampaignTestEmail(t *testing.T) {
	var gotMethod, gotPath string
	var body map[string]any
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusAccepted)
	})

	resp, err := c.Campaigns.TestEmail(context.Background(), "camp_1", &CampaignTestEmailParams{To: "qa@example.com", StepID: "step_2"})
	if err != nil {
		t.Fatalf("TestEmail: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/v1/campaigns/camp_1/test-email" {
		t.Errorf("path = %q", gotPath)
	}
	if body["to"] != "qa@example.com" {
		t.Errorf("body to = %v", body["to"])
	}
	if body["step_id"] != "step_2" {
		t.Errorf("body step_id = %v", body["step_id"])
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestCampaignSenders(t *testing.T) {
	var gotMethod, gotPath string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"data":["acct_1","acct_2"]}`))
	})

	senders, resp, err := c.Campaigns.Senders(context.Background(), "camp_1")
	if err != nil {
		t.Fatalf("Senders: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %s, want GET", gotMethod)
	}
	if gotPath != "/v1/campaigns/camp_1/senders" {
		t.Errorf("path = %q", gotPath)
	}
	if len(senders) != 2 || senders[0] != "acct_1" || senders[1] != "acct_2" {
		t.Errorf("senders = %v", senders)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestCampaignSetSenders(t *testing.T) {
	var gotMethod, gotPath string
	var body struct {
		Senders []string `json:"senders"`
	}
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"data":["acct_9"]}`))
	})

	senders, _, err := c.Campaigns.SetSenders(context.Background(), "camp_1", []string{"acct_9"})
	if err != nil {
		t.Fatalf("SetSenders: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", gotMethod)
	}
	if gotPath != "/v1/campaigns/camp_1/senders" {
		t.Errorf("path = %q", gotPath)
	}
	if len(body.Senders) != 1 || body.Senders[0] != "acct_9" {
		t.Errorf("request body senders = %v", body.Senders)
	}
	if len(senders) != 1 || senders[0] != "acct_9" {
		t.Errorf("returned senders = %v", senders)
	}
}

func TestCampaignCreateStep(t *testing.T) {
	var gotMethod, gotPath string
	var body map[string]any
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"step_1","name":"Intro","subject":"Hi","kind":"email","position":0}`))
	})

	wait := 1440
	step, resp, err := c.Campaigns.CreateStep(context.Background(), "camp_1", &StepParams{Name: "Intro", Subject: "Hi", Kind: "email", WaitAfter: &wait})
	if err != nil {
		t.Fatalf("CreateStep: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/v1/campaigns/camp_1/steps" {
		t.Errorf("path = %q", gotPath)
	}
	if body["name"] != "Intro" {
		t.Errorf("body name = %v", body["name"])
	}
	if step.ID != "step_1" || step.Kind != "email" {
		t.Errorf("step = %+v", step)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestCampaignUpdateStep(t *testing.T) {
	var gotMethod, gotPath string
	var body map[string]any
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"id":"step_2","name":"Follow up","kind":"email"}`))
	})

	pos := 2
	step, _, err := c.Campaigns.UpdateStep(context.Background(), "camp_1", "step_2", &StepParams{Name: "Follow up", Position: &pos})
	if err != nil {
		t.Fatalf("UpdateStep: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", gotMethod)
	}
	if gotPath != "/v1/campaigns/camp_1/steps/step_2" {
		t.Errorf("path = %q", gotPath)
	}
	if body["name"] != "Follow up" {
		t.Errorf("body name = %v", body["name"])
	}
	if step.ID != "step_2" {
		t.Errorf("step id = %q", step.ID)
	}
}

func TestCampaignDeleteStep(t *testing.T) {
	var gotMethod, gotPath string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	})

	resp, err := c.Campaigns.DeleteStep(context.Background(), "camp_1", "step_2")
	if err != nil {
		t.Fatalf("DeleteStep: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", gotMethod)
	}
	if gotPath != "/v1/campaigns/camp_1/steps/step_2" {
		t.Errorf("path = %q", gotPath)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestCampaignErrorPaths(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "req_err")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad_request","message":"nope","code":"invalid"}`))
	})

	ctx := context.Background()

	t.Run("Create", func(t *testing.T) {
		camp, _, err := c.Campaigns.Create(ctx, &CampaignCreateParams{Name: "x"})
		if err == nil || camp != nil {
			t.Fatalf("want error and nil campaign, got camp=%+v err=%v", camp, err)
		}
	})

	t.Run("ListSteps", func(t *testing.T) {
		steps, _, err := c.Campaigns.ListSteps(ctx, "camp_1")
		if err == nil || steps != nil {
			t.Fatalf("want error and nil steps, got %v err=%v", steps, err)
		}
	})

	t.Run("Update", func(t *testing.T) {
		camp, resp, err := c.Campaigns.Update(ctx, "camp_1", &CampaignUpdateParams{})
		if err == nil {
			t.Fatal("expected error")
		}
		if camp != nil {
			t.Errorf("campaign = %+v, want nil", camp)
		}
		var apiErr *Error
		if !errors.As(err, &apiErr) {
			t.Fatalf("errors.As(*Error) failed: %v", err)
		}
		if apiErr.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d", apiErr.StatusCode)
		}
		if resp == nil {
			t.Error("resp should be non-nil on API error")
		}
	})

	t.Run("Start", func(t *testing.T) {
		camp, _, err := c.Campaigns.Start(ctx, "camp_1")
		if err == nil || camp != nil {
			t.Fatalf("want error and nil campaign, got camp=%+v err=%v", camp, err)
		}
	})

	t.Run("Stop", func(t *testing.T) {
		camp, _, err := c.Campaigns.Stop(ctx, "camp_1")
		if err == nil || camp != nil {
			t.Fatalf("want error and nil campaign, got camp=%+v err=%v", camp, err)
		}
	})

	t.Run("Senders", func(t *testing.T) {
		senders, _, err := c.Campaigns.Senders(ctx, "camp_1")
		if err == nil || senders != nil {
			t.Fatalf("want error and nil senders, got %v err=%v", senders, err)
		}
	})

	t.Run("SetSenders", func(t *testing.T) {
		senders, _, err := c.Campaigns.SetSenders(ctx, "camp_1", []string{"x"})
		if err == nil || senders != nil {
			t.Fatalf("want error and nil senders, got %v err=%v", senders, err)
		}
	})

	t.Run("CreateStep", func(t *testing.T) {
		step, _, err := c.Campaigns.CreateStep(ctx, "camp_1", &StepParams{})
		if err == nil || step != nil {
			t.Fatalf("want error and nil step, got %+v err=%v", step, err)
		}
	})

	t.Run("UpdateStep", func(t *testing.T) {
		step, _, err := c.Campaigns.UpdateStep(ctx, "camp_1", "step_2", &StepParams{})
		if err == nil || step != nil {
			t.Fatalf("want error and nil step, got %+v err=%v", step, err)
		}
	})
}
