package warmbly

import (
	"testing"
	"time"
)

func TestTokenValid(t *testing.T) {
	fixed := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	old := timeNow
	timeNow = func() time.Time { return fixed }
	defer func() { timeNow = old }()

	tests := []struct {
		name string
		tok  *Token
		want bool
	}{
		{"nil", nil, false},
		{"empty access token", &Token{}, false},
		{"no expiry", &Token{AccessToken: "a"}, true},
		{"future expiry", &Token{AccessToken: "a", Expiry: fixed.Add(time.Hour)}, true},
		{"just expired", &Token{AccessToken: "a", Expiry: fixed.Add(-time.Second)}, false},
		{"within expiry delta", &Token{AccessToken: "a", Expiry: fixed.Add(expiryDelta / 2)}, false},
	}
	for _, tt := range tests {
		if got := tt.tok.Valid(); got != tt.want {
			t.Errorf("%s: Valid() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestTokenType(t *testing.T) {
	cases := map[string]string{
		"":       "Bearer",
		"bearer": "Bearer",
		"Bearer": "Bearer",
		"BEARER": "Bearer",
		"MAC":    "MAC",
	}
	for in, want := range cases {
		tok := &Token{TokenType: in}
		if got := tok.Type(); got != want {
			t.Errorf("Token{%q}.Type() = %q, want %q", in, got, want)
		}
	}
}

func TestStaticTokenSource(t *testing.T) {
	want := &Token{AccessToken: "abc"}
	got, err := StaticTokenSource(want).Token()
	if err != nil {
		t.Fatalf("Token() error: %v", err)
	}
	if got != want {
		t.Errorf("StaticTokenSource returned %v, want %v", got, want)
	}
}
