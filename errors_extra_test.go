package warmbly

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

// TestErrorEnvelopeErrorFallback exercises (*Error).Error across the optional
// Code and RequestID branches, and the empty-Message fallback to
// http.StatusText.
func TestErrorEnvelopeErrorFallback(t *testing.T) {
	tests := []struct {
		name    string
		err     *Error
		want    []string
		notWant []string
	}{
		{
			name:    "empty message falls back to status text",
			err:     &Error{StatusCode: http.StatusNotFound},
			want:    []string{"warmbly: 404", http.StatusText(http.StatusNotFound)},
			notWant: []string{"(", "request_id="},
		},
		{
			name:    "code present, no request id",
			err:     &Error{StatusCode: http.StatusBadRequest, Code: "bad_input", Message: "nope"},
			want:    []string{"warmbly: 400", "(bad_input)", ": nope"},
			notWant: []string{"request_id="},
		},
		{
			name:    "request id present, no code",
			err:     &Error{StatusCode: http.StatusConflict, Message: "exists", RequestID: "req_xyz"},
			want:    []string{"warmbly: 409", ": exists", "[request_id=req_xyz]"},
			notWant: []string{"("},
		},
		{
			name:    "unknown status code yields empty status text",
			err:     &Error{StatusCode: 799},
			want:    []string{"warmbly: 799", ": "},
			notWant: []string{"request_id="},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("Error() = %q, missing %q", got, w)
				}
			}
			for _, nw := range tt.notWant {
				if strings.Contains(got, nw) {
					t.Errorf("Error() = %q, should not contain %q", got, nw)
				}
			}
		})
	}
}

// TestErrorEnvelopeIsNonErrorTarget checks that (*Error).Is returns false when
// the target is not an *Error, both directly and via errors.Is.
func TestErrorEnvelopeIsNonErrorTarget(t *testing.T) {
	e := &Error{StatusCode: http.StatusNotFound}

	if e.Is(errors.New("plain")) {
		t.Error("(*Error).Is(plain error) = true, want false")
	}
	if errors.Is(e, errors.New("x")) {
		t.Error("errors.Is(*Error, plain error) = true, want false")
	}

	// A non-pointer concrete type must also not match.
	if errors.Is(e, http.ErrNotSupported) {
		t.Error("errors.Is(*Error, http.ErrNotSupported) = true, want false")
	}
}

// TestErrorEnvelopeIsErrorTarget covers the *Error target branches of
// (*Error).Is that are not already exercised elsewhere: the ErrServer 5xx
// catch-all returning false for a non-5xx, and an exact same-status match
// between two distinct *Error values.
func TestErrorEnvelopeIsErrorTarget(t *testing.T) {
	// ErrServer matches any 5xx, but not a 4xx.
	if (&Error{StatusCode: http.StatusBadRequest}).Is(ErrServer) {
		t.Error("400.Is(ErrServer) = true, want false")
	}
	if !(&Error{StatusCode: http.StatusBadGateway}).Is(ErrServer) {
		t.Error("502.Is(ErrServer) = false, want true")
	}

	// Exact status match against a non-sentinel *Error target.
	target := &Error{StatusCode: http.StatusTeapot}
	if !(&Error{StatusCode: http.StatusTeapot, Message: "brew"}).Is(target) {
		t.Error("418.Is(418) = false, want true")
	}
	if (&Error{StatusCode: http.StatusOK}).Is(target) {
		t.Error("200.Is(418) = true, want false")
	}
}
