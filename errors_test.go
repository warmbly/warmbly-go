package warmbly

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestErrorIsSentinels(t *testing.T) {
	tests := []struct {
		status   int
		sentinel error
		want     bool
	}{
		{http.StatusNotFound, ErrNotFound, true},
		{http.StatusNotFound, ErrUnauthorized, false},
		{http.StatusUnauthorized, ErrUnauthorized, true},
		{http.StatusForbidden, ErrForbidden, true},
		{http.StatusConflict, ErrConflict, true},
		{http.StatusTooManyRequests, ErrRateLimited, true},
		{http.StatusUnprocessableEntity, ErrUnprocessable, true},
		{http.StatusInternalServerError, ErrServer, true},
		{http.StatusServiceUnavailable, ErrServer, true}, // any 5xx matches ErrServer
		{http.StatusNotFound, ErrServer, false},
	}
	for _, tt := range tests {
		err := error(&Error{StatusCode: tt.status})
		if got := errors.Is(err, tt.sentinel); got != tt.want {
			t.Errorf("errors.Is(Error{%d}, sentinel) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestErrorTemporary(t *testing.T) {
	cases := map[int]bool{
		http.StatusTooManyRequests:     true,
		http.StatusInternalServerError: true,
		http.StatusBadGateway:          true,
		http.StatusNotFound:            false,
		http.StatusBadRequest:          false,
	}
	for status, want := range cases {
		e := &Error{StatusCode: status}
		if got := e.Temporary(); got != want {
			t.Errorf("Error{%d}.Temporary() = %v, want %v", status, got, want)
		}
	}
}

func TestErrorString(t *testing.T) {
	e := &Error{StatusCode: 404, Code: "resource_not_found", Message: "Resource not found.", RequestID: "req_123"}
	got := e.Error()
	for _, want := range []string{"404", "resource_not_found", "Resource not found.", "req_123"} {
		if !strings.Contains(got, want) {
			t.Errorf("Error() = %q, missing %q", got, want)
		}
	}
}
