package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestFrameRoundTrip(t *testing.T) {
	f := frame{
		JoinRef: "1",
		Ref:     "2",
		Topic:   "org:abc",
		Event:   evJoin,
		Payload: json.RawMessage(`{"intents":["CAMPAIGN"]}`),
	}
	encoded, err := f.encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	// The wire format is a five-element array, not an object.
	var arr []json.RawMessage
	if err := json.Unmarshal(encoded, &arr); err != nil {
		t.Fatalf("encoded frame is not an array: %v", err)
	}
	if len(arr) != 5 {
		t.Fatalf("got %d elements, want 5", len(arr))
	}

	got, err := decodeFrame(encoded)
	if err != nil {
		t.Fatalf("decodeFrame: %v", err)
	}
	if got.JoinRef != f.JoinRef || got.Ref != f.Ref || got.Topic != f.Topic || got.Event != f.Event {
		t.Errorf("round trip lost fields: %+v", got)
	}
	if string(got.Payload) != string(f.Payload) {
		t.Errorf("payload = %s, want %s", got.Payload, f.Payload)
	}
}

// TestFrameEncodesNullRefs covers a server-push-shaped frame: the correlation
// refs must serialize as null rather than empty strings, which the server
// rejects.
func TestFrameEncodesNullRefs(t *testing.T) {
	encoded, err := frame{Topic: "phoenix", Event: evHeartbeat}.encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	want := `[null,null,"phoenix","heartbeat",{}]`
	if string(encoded) != want {
		t.Errorf("encoded = %s, want %s", encoded, want)
	}
}

func TestDecodeFrameRejectsMalformed(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"not an array", `{"topic":"org:1"}`},
		{"too few elements", `[null,null,"org:1"]`},
		{"too many elements", `[null,null,"org:1","ev",{},"extra"]`},
		{"non-string topic", `[null,null,42,"ev",{}]`},
		{"non-string event", `[null,null,"org:1",42,{}]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := decodeFrame([]byte(tc.in)); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

// TestDecodeServerPush covers the shape the server actually sends: an event
// push with null refs.
func TestDecodeServerPush(t *testing.T) {
	got, err := decodeFrame([]byte(`[null,null,"org:abc","CAMPAIGN_STARTED",{"seq":42,"campaign_id":"c_1"}]`))
	if err != nil {
		t.Fatalf("decodeFrame: %v", err)
	}
	if got.JoinRef != "" || got.Ref != "" {
		t.Errorf("refs should be empty on a push, got %q/%q", got.JoinRef, got.Ref)
	}
	if got.Event != EventCampaignStarted {
		t.Errorf("event = %q", got.Event)
	}
	if seq := extractSeq(got.Payload); seq != 42 {
		t.Errorf("seq = %d, want 42", seq)
	}
}

func TestExtractSeq(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"present", `{"seq":7}`, 7},
		{"absent", `{"campaign_id":"c_1"}`, 0},
		{"not an object", `[]`, 0},
		{"empty", ``, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractSeq(json.RawMessage(tc.in)); got != tc.want {
				t.Errorf("extractSeq(%s) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestSocketURLCarriesCredentialAndVersion(t *testing.T) {
	c := New("wmbly_key", "org_1", WithURL("wss://realtime.example.com/socket/websocket"))
	got := c.socketURL()
	for _, want := range []string{"token=wmbly_key", "vsn=" + protocolVersion} {
		if !strings.Contains(got, want) {
			t.Errorf("socketURL() = %q, want it to contain %q", got, want)
		}
	}
}

func TestTopicIsScopedToOrganization(t *testing.T) {
	if got := New("k", "org_1").topic(); got != "org:org_1" {
		t.Errorf("topic() = %q, want org:org_1", got)
	}
}

// TestJoinErrorMapsCodes checks that a rejected join is classified, since the
// client must not retry a credential problem forever.
func TestJoinErrorMapsCodes(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    error
	}{
		{"unauthorized", `{"code":4001,"reason":"bad token"}`, ErrUnauthorized},
		{"forbidden", `{"code":4003,"reason":"not a member"}`, ErrForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := joinError(json.RawMessage(tc.payload))
			if !errors.Is(err, tc.want) {
				t.Errorf("joinError = %v, want it to wrap %v", err, tc.want)
			}
		})
	}

	// An unclassified code is still an error, just a retryable one.
	err := joinError(json.RawMessage(`{"code":5000,"reason":"server busy"}`))
	if errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrForbidden) {
		t.Errorf("unexpected classification for %v", err)
	}
}

func TestHeartbeatIntervalFallsBack(t *testing.T) {
	if got := heartbeatInterval(Ready{HeartbeatIntervalMS: 5000}); got != 5*time.Second {
		t.Errorf("interval = %v, want 5s", got)
	}
	if got := heartbeatInterval(Ready{}); got != 25*time.Second {
		t.Errorf("fallback interval = %v, want 25s", got)
	}
}

func TestOpenRequiresCredentials(t *testing.T) {
	if err := New("", "org_1").Open(context.Background()); err == nil {
		t.Error("expected an error without a token")
	}
	if err := New("k", "").Open(context.Background()); err == nil {
		t.Error("expected an error without an organization id")
	}
}
