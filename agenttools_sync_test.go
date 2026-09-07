package warmbly

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// agentToolServer answers every request with status and body, recording the
// last request so a test can assert on method, path, query and body.
func agentToolServer(t *testing.T, status int, body string, got *recordedRequest) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got != nil {
			got.method = r.Method
			got.path = r.URL.Path
			got.rawQuery = r.URL.RawQuery
			raw, _ := io.ReadAll(r.Body)
			got.body = string(raw)
			got.idempotencyKey = r.Header.Get("Idempotency-Key")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"), WithMaxRetries(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestAgentToolRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()

	if _, _, err := c.AgentTools.List(ctx); err != nil {
		t.Fatalf("List: %v", err)
	}
	if got.method != http.MethodGet || got.path != "/v1/ai/tools" || got.rawQuery != "" {
		t.Errorf("List = %s %s?%s, want GET /v1/ai/tools", got.method, got.path, got.rawQuery)
	}

	if _, _, err := c.AgentTools.ListFunctions(ctx); err != nil {
		t.Fatalf("ListFunctions: %v", err)
	}
	if got.method != http.MethodGet || got.path != "/v1/ai/tools" || got.rawQuery != "format=openai" {
		t.Errorf("ListFunctions = %s %s?%s, want GET /v1/ai/tools?format=openai", got.method, got.path, got.rawQuery)
	}
}

func TestAgentToolCallRequest(t *testing.T) {
	var got recordedRequest
	c := agentToolServer(t, http.StatusOK, `{"data":{"name":"list_threads","result":{"threads":[],"count":0}}}`, &got)
	ctx := context.Background()

	// A map argument object is sent as the body verbatim, and the name is
	// path-escaped.
	res, _, err := c.AgentTools.Call(ctx, "list_threads", map[string]any{"folder": "inbox", "limit": 10}, WithIdempotencyKey("idem-1"))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got.method != http.MethodPost || got.path != "/v1/ai/tools/list_threads/call" {
		t.Errorf("Call = %s %s, want POST /v1/ai/tools/list_threads/call", got.method, got.path)
	}
	if got.idempotencyKey != "idem-1" {
		t.Errorf("Idempotency-Key = %q, want idem-1", got.idempotencyKey)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(got.body), &sent); err != nil {
		t.Fatalf("body %q is not a JSON object: %v", got.body, err)
	}
	if sent["folder"] != "inbox" || sent["limit"] != float64(10) {
		t.Errorf("body = %v, want folder=inbox limit=10", sent)
	}
	if res.Name != "list_threads" {
		t.Errorf("Name = %q", res.Name)
	}

	// Raw model output passes through untouched.
	raw := json.RawMessage(`{"query":"acme","limit":5}`)
	if _, _, err := c.AgentTools.Call(ctx, "search_contacts", raw); err != nil {
		t.Fatalf("Call raw: %v", err)
	}
	if bytes.TrimSpace([]byte(got.body)) == nil || !bytes.Equal(bytes.TrimSpace([]byte(got.body)), raw) {
		t.Errorf("raw body = %q, want %s", got.body, raw)
	}

	// No arguments means no body at all, not "null".
	if _, _, err := c.AgentTools.Call(ctx, "list_campaigns", nil); err != nil {
		t.Fatalf("Call nil: %v", err)
	}
	if got.body != "" {
		t.Errorf("nil args sent body %q, want empty", got.body)
	}
	if got.path != "/v1/ai/tools/list_campaigns/call" {
		t.Errorf("path = %s", got.path)
	}
}

func TestAgentToolListDecode(t *testing.T) {
	const fixture = `{
	  "data": [
	    {
	      "name": "list_threads",
	      "description": "List unified-inbox conversation threads.",
	      "input_schema": {
	        "type": "object",
	        "properties": {
	          "subject": {"type": "string"},
	          "unseen_only": {"type": "boolean"},
	          "limit": {"type": "integer"}
	        }
	      }
	    },
	    {
	      "name": "list_campaigns",
	      "description": "List campaigns.",
	      "input_schema": {"type": "object", "properties": {}}
	    }
	  ]
	}`
	c := agentToolServer(t, http.StatusOK, fixture, nil)

	tools, _, err := c.AgentTools.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("got %d tools, want 2", len(tools))
	}
	if tools[0].Name != "list_threads" || tools[0].Description != "List unified-inbox conversation threads." {
		t.Errorf("tool[0] = %+v", tools[0])
	}

	// The schema round-trips byte-for-byte (modulo whitespace) so it can go
	// into a function-calling client unchanged.
	var want, gotSchema bytes.Buffer
	_ = json.Compact(&want, []byte(`{"type":"object","properties":{"subject":{"type":"string"},"unseen_only":{"type":"boolean"},"limit":{"type":"integer"}}}`))
	_ = json.Compact(&gotSchema, tools[0].InputSchema)
	if want.String() != gotSchema.String() {
		t.Errorf("InputSchema = %s, want %s", gotSchema.String(), want.String())
	}

	// Local conversion to the OpenAI shape.
	fn := tools[0].Function()
	if fn.Type != "function" || fn.Function.Name != "list_threads" || !bytes.Equal(fn.Function.Parameters, tools[0].InputSchema) {
		t.Errorf("Function() = %+v", fn)
	}
	fns := AgentToolFunctions(tools)
	if len(fns) != 2 || fns[1].Function.Name != "list_campaigns" {
		t.Errorf("AgentToolFunctions = %+v", fns)
	}
	out, err := json.Marshal(fns[1])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const wantJSON = `{"type":"function","function":{"name":"list_campaigns","description":"List campaigns.","parameters":{"type":"object","properties":{}}}}`
	if string(out) != wantJSON {
		t.Errorf("marshaled function = %s, want %s", out, wantJSON)
	}
}

func TestAgentToolListFunctionsDecode(t *testing.T) {
	const fixture = `{"data":[{"type":"function","function":{"name":"get_contact","description":"Read one contact.","parameters":{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}}}]}`
	c := agentToolServer(t, http.StatusOK, fixture, nil)

	fns, _, err := c.AgentTools.ListFunctions(context.Background())
	if err != nil {
		t.Fatalf("ListFunctions: %v", err)
	}
	if len(fns) != 1 || fns[0].Type != "function" || fns[0].Function.Name != "get_contact" {
		t.Fatalf("ListFunctions = %+v", fns)
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(fns[0].Function.Parameters, &schema); err != nil || len(schema.Required) != 1 || schema.Required[0] != "id" {
		t.Errorf("Parameters = %s (err %v)", fns[0].Function.Parameters, err)
	}
}

func TestAgentToolCallDecode(t *testing.T) {
	t.Run("json result", func(t *testing.T) {
		c := agentToolServer(t, http.StatusOK, `{"data":{"name":"list_threads","result":{"threads":[{"id":"th_1","subject":"Re: pricing"}],"count":1}}}`, nil)
		res, _, err := c.AgentTools.Call(context.Background(), "list_threads", map[string]any{})
		if err != nil {
			t.Fatalf("Call: %v", err)
		}
		if res.Name != "list_threads" {
			t.Errorf("Name = %q", res.Name)
		}
		var out struct {
			Threads []struct {
				ID      string `json:"id"`
				Subject string `json:"subject"`
			} `json:"threads"`
			Count int `json:"count"`
		}
		if err := res.Decode(&out); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if out.Count != 1 || len(out.Threads) != 1 || out.Threads[0].ID != "th_1" {
			t.Errorf("decoded = %+v", out)
		}
	})

	t.Run("string result", func(t *testing.T) {
		c := agentToolServer(t, http.StatusOK, `{"data":{"name":"fetch_url","result":"plain text body"}}`, nil)
		res, _, err := c.AgentTools.Call(context.Background(), "fetch_url", nil)
		if err != nil {
			t.Fatalf("Call: %v", err)
		}
		var s string
		if err := res.Decode(&s); err != nil || s != "plain text body" {
			t.Errorf("Decode string = %q (err %v)", s, err)
		}
	})
}

func TestAgentToolCallErrors(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		sentinel *Error
		code     string
	}{
		{"unknown or send-class tool", http.StatusNotFound, `{"error":"Not Found","message":"tool not found","code":"not_found"}`, ErrNotFound, "not_found"},
		{"permission denied", http.StatusForbidden, `{"error":"Forbidden","message":"your credentials lack the permission for this tool","code":"forbidden"}`, ErrForbidden, "forbidden"},
		{"malformed body", http.StatusBadRequest, `{"error":"Bad Request","message":"the request body must be the tool's JSON argument object","code":"bad_request"}`, ErrBadRequest, "bad_request"},
		{"invalid arguments", http.StatusUnprocessableEntity, `{"error":"Unprocessable","message":"invalid tool arguments","code":"unprocessable"}`, ErrUnprocessable, "unprocessable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := agentToolServer(t, tc.status, tc.body, nil)
			res, _, err := c.AgentTools.Call(context.Background(), "x", nil)
			if res != nil {
				t.Errorf("result = %+v, want nil", res)
			}
			if !errors.Is(err, tc.sentinel) {
				t.Fatalf("err = %v, want %v", err, tc.sentinel)
			}
			var apiErr *Error
			if !errors.As(err, &apiErr) {
				t.Fatalf("err %T is not *Error", err)
			}
			if apiErr.Code != tc.code {
				t.Errorf("Code = %q, want %q", apiErr.Code, tc.code)
			}
			if apiErr.Message == "" {
				t.Error("Message is empty; the tool's message is meant for the model")
			}
		})
	}
}
