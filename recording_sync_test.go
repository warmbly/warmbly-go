package warmbly

import (
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// syncCapture is the last request a [respondingClient] saw.
type syncCapture struct {
	method      string
	path        string
	rawQuery    string
	body        string
	contentType string
}

// respondingClient answers every request with one canned status and body, and
// records what it was asked. The canned body is built from the SDK's own Go
// structs by the callers, so a field whose JSON tag drifts from what the test
// asserts fails the round trip.
func respondingClient(t *testing.T, got *syncCapture, status int, body string) *Client {
	t.Helper()
	return respondingClientType(t, got, status, "application/json", body)
}

func respondingClientType(t *testing.T, got *syncCapture, status int, contentType, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.path = r.URL.Path
		got.rawQuery = r.URL.RawQuery
		got.contentType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		got.body = string(raw)
		if status == http.StatusNoContent {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// jsonOf marshals v for use as a canned response body.
func jsonOf(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return string(b)
}

// wantRequest asserts the method, path and raw query of a capture.
func (got *syncCapture) wantRequest(t *testing.T, method, path, rawQuery string) {
	t.Helper()
	if got.method != method || got.path != path {
		t.Fatalf("request = %s %s, want %s %s", got.method, got.path, method, path)
	}
	if got.rawQuery != rawQuery {
		t.Fatalf("query = %q, want %q", got.rawQuery, rawQuery)
	}
}

// wantBody asserts the JSON body of a capture, ignoring key order and spacing.
func (got *syncCapture) wantBody(t *testing.T, want string) {
	t.Helper()
	if want == "" {
		if got.body != "" {
			t.Fatalf("body = %s, want none", got.body)
		}
		return
	}
	var a, b any
	if err := json.Unmarshal([]byte(got.body), &a); err != nil {
		t.Fatalf("body %q is not JSON: %v", got.body, err)
	}
	if err := json.Unmarshal([]byte(want), &b); err != nil {
		t.Fatalf("bad want: %v", err)
	}
	ga, _ := json.Marshal(a)
	gb, _ := json.Marshal(b)
	if string(ga) != string(gb) {
		t.Fatalf("body = %s, want %s", got.body, want)
	}
}

// multipartParts reads a captured multipart body into its plain fields and its
// file parts (name -> "filename|content").
func (got *syncCapture) multipartParts(t *testing.T) (fields, files map[string]string) {
	t.Helper()
	mt, params, err := mime.ParseMediaType(got.contentType)
	if err != nil || !strings.HasPrefix(mt, "multipart/") {
		t.Fatalf("Content-Type = %q, want multipart/form-data", got.contentType)
	}
	fields, files = map[string]string{}, map[string]string{}
	mr := multipart.NewReader(strings.NewReader(got.body), params["boundary"])
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read multipart: %v", err)
		}
		data, _ := io.ReadAll(p)
		if p.FileName() != "" {
			files[p.FormName()] = p.FileName() + "|" + string(data)
		} else {
			fields[p.FormName()] = string(data)
		}
	}
	return fields, files
}
