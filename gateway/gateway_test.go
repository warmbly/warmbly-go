package gateway

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
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

func TestTopicHelpers(t *testing.T) {
	cases := map[string]string{
		OrgTopic("org_1"):         "org:org_1",
		UserTopic("u_1"):          "user:u_1",
		CampaignTopic("c_1"):      "campaign:c_1",
		AccountTopic("a_1"):       "account:a_1",
		BulkTopic("op_1"):         "bulk:op_1",
		New("k", "org_1").topic(): "org:org_1",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("topic = %q, want %q", got, want)
		}
	}
}

// TestJoinErrorClassification pins the mapping the client branches on. Getting
// it wrong either retries a permission failure forever or gives up on a rate
// limit that would have cleared in seconds.
func TestJoinErrorClassification(t *testing.T) {
	cases := []struct {
		name      string
		payload   string
		wantCode  int
		unwrapsTo error
		permanent bool
	}{
		{"missing token", `{"code":4003,"reason":"Not authenticated"}`, JoinCodeUnauthenticated, ErrUnauthorized, true},
		{"auth failed", `{"code":4004,"reason":"Token expired"}`, JoinCodeAuthFailed, ErrUnauthorized, true},
		{"malformed topic", `{"code":4005,"reason":"invalid_campaign_id"}`, JoinCodeInvalidTopic, nil, true},
		{"rate limited", `{"code":4007,"reason":"rate_limited","category":"ws_join","retry_after_ms":21400}`, JoinCodeRateLimited, nil, false},
		{"connection cap", `{"code":4009,"reason":"Connection limit exceeded"}`, JoinCodeConnectionLimit, nil, false},
		{"not a member", `{"code":4010,"reason":"not_a_member"}`, JoinCodePermissionDenied, ErrForbidden, true},
		{"no such campaign", `{"code":4010,"reason":"campaign_not_found"}`, JoinCodePermissionDenied, ErrForbidden, true},
		{"unknown code", `{"code":5000,"reason":"server busy"}`, 5000, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := error(newJoinError("org:1", json.RawMessage(tc.payload)))

			var je *JoinError
			if !errors.As(err, &je) {
				t.Fatalf("errors.As did not yield a *JoinError from %v", err)
			}
			if je.Code != tc.wantCode {
				t.Errorf("Code = %d, want %d", je.Code, tc.wantCode)
			}
			if je.Topic != "org:1" {
				t.Errorf("Topic = %q, want org:1", je.Topic)
			}
			if je.Permanent() != tc.permanent {
				t.Errorf("Permanent() = %v, want %v", je.Permanent(), tc.permanent)
			}
			for _, sentinel := range []error{ErrUnauthorized, ErrForbidden} {
				want := errors.Is(tc.unwrapsTo, sentinel)
				if got := errors.Is(err, sentinel); got != want {
					t.Errorf("errors.Is(err, %v) = %v, want %v", sentinel, got, want)
				}
			}
			if !strings.Contains(je.Error(), je.Reason) {
				t.Errorf("Error() = %q, want it to name the reason", je.Error())
			}
		})
	}

	// The rate-limit hint is the wait the client honors, so it must survive
	// decoding intact.
	je := newJoinError("org:1", json.RawMessage(`{"code":4007,"reason":"rate_limited","category":"ws_join","retry_after_ms":21400}`))
	if je.RetryAfter != 21400*time.Millisecond {
		t.Errorf("RetryAfter = %v, want 21.4s", je.RetryAfter)
	}
	if je.Category != RateLimitCategoryJoin {
		t.Errorf("Category = %q, want %q", je.Category, RateLimitCategoryJoin)
	}
}

// TestJoinErrorFallsBackToAReason covers a refusal body this client does not
// recognize: a bare string, or an object with no reason at all.
func TestJoinErrorFallsBackToAReason(t *testing.T) {
	if got := newJoinError("org:1", json.RawMessage(`"unauthorized"`)); got.Reason != "unauthorized" {
		t.Errorf("Reason = %q, want unauthorized", got.Reason)
	}
	if got := newJoinError("org:1", json.RawMessage(`{}`)); got.Reason == "" {
		t.Error("Reason should never be empty")
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

// --- integration against a fake Phoenix server -----------------------------

// TestJoinRateLimitRetriesOnSameSocket is the behavior the gateway docs ask
// for: a 4007 leaves the socket open, so the client waits out retry_after_ms
// and re-sends the join instead of reconnecting. Reconnecting would spend a
// handshake from a different budget and change nothing.
func TestJoinRateLimitRetriesOnSameSocket(t *testing.T) {
	srv := newPhxServer(t)
	srv.onJoin = func(attempt int, topic string) (string, string) {
		if attempt == 1 {
			return "error", `{"code":4007,"reason":"rate_limited","category":"ws_join","retry_after_ms":40}`
		}
		return "ok", helloResponse
	}

	limits := make(chan RateLimited, 4)
	c := srv.client(t)
	On(c, EventRateLimited, func(_ context.Context, r *RateLimited) {
		select {
		case limits <- *r:
		default:
		}
	})

	if err := c.Open(testContext(t)); err != nil {
		t.Fatalf("Open: %v", err)
	}

	if got := srv.handshakes(); got != 1 {
		t.Errorf("handshakes = %d, want 1: the retry must reuse the socket", got)
	}
	if got := srv.joins("org:org_1"); got != 2 {
		t.Errorf("joins = %d, want 2 (refused, then accepted)", got)
	}
	if got := c.Ready().Seq; got != 4821 {
		t.Errorf("Ready().Seq = %d, want the HELLO seq", got)
	}

	select {
	case r := <-limits:
		if r.Category != RateLimitCategoryJoin || r.Topic != "org:org_1" || r.RetryAfterMS != 40 {
			t.Errorf("rate limit event = %+v", r)
		}
	case <-time.After(2 * time.Second):
		t.Error("no rate_limited event was raised for the refused join")
	}
}

// TestJoinRefusedPermanently checks the codes that must end the session on the
// first reply rather than being retried.
func TestJoinRefusedPermanently(t *testing.T) {
	cases := []struct {
		name     string
		response string
		wantCode int
		sentinel error
	}{
		{"malformed topic", `{"code":4005,"reason":"invalid_campaign_id"}`, JoinCodeInvalidTopic, nil},
		{"not a member", `{"code":4010,"reason":"not_a_member"}`, JoinCodePermissionDenied, ErrForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newPhxServer(t)
			srv.onJoin = func(int, string) (string, string) { return "error", tc.response }

			c := srv.client(t)
			err := c.Open(testContext(t))
			if err == nil {
				t.Fatal("Open should have failed")
			}

			var je *JoinError
			if !errors.As(err, &je) {
				t.Fatalf("Open error is not a *JoinError: %v", err)
			}
			if je.Code != tc.wantCode {
				t.Errorf("Code = %d, want %d", je.Code, tc.wantCode)
			}
			if tc.sentinel != nil && !errors.Is(err, tc.sentinel) {
				t.Errorf("errors.Is(err, %v) = false", tc.sentinel)
			}

			// The session is over, and the refusal is not retried.
			select {
			case <-c.Done():
			case <-time.After(2 * time.Second):
				t.Fatal("the session did not stop")
			}
			if !errors.Is(c.Err(), err) && c.Err() == nil {
				t.Errorf("Err() = %v, want the join refusal", c.Err())
			}
			if got := srv.joins("org:org_1"); got != 1 {
				t.Errorf("joins = %d, want 1: a permanent refusal must not be retried", got)
			}
			if got := srv.handshakes(); got != 1 {
				t.Errorf("handshakes = %d, want 1", got)
			}
		})
	}
}

// TestExtraTopicRefusalKeepsTheSession covers the point of [WithTopics]: one
// bad topic must not cost you the workspace stream.
func TestExtraTopicRefusalKeepsTheSession(t *testing.T) {
	srv := newPhxServer(t)
	srv.onJoin = func(_ int, topic string) (string, string) {
		if topic == "campaign:nope" {
			return "error", `{"code":4010,"reason":"campaign_not_found"}`
		}
		return "ok", helloResponse
	}

	failures := make(chan JoinFailed, 4)
	c := srv.client(t, WithTopics(CampaignTopic("nope")))
	On(c, EventJoinFailed, func(_ context.Context, f *JoinFailed) {
		select {
		case failures <- *f:
		default:
		}
	})

	if err := c.Open(testContext(t)); err != nil {
		t.Fatalf("Open: %v", err)
	}

	select {
	case f := <-failures:
		if f.Topic != "campaign:nope" || f.Code != JoinCodePermissionDenied || f.Reason != JoinReasonCampaignNotFound {
			t.Errorf("join failure = %+v", f)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no join_failed event was raised")
	}

	select {
	case <-c.Done():
		t.Fatalf("the session ended over an extra topic: %v", c.Err())
	default:
	}
}

// TestEventsCarryTopicAndAdvanceSeq checks the two things a resuming client
// depends on: the sequence position moves forward, and a handler can tell which
// channel an event came from.
func TestEventsCarryTopicAndAdvanceSeq(t *testing.T) {
	srv := newPhxServer(t)
	srv.onJoin = func(int, string) (string, string) { return "ok", helloResponse }

	events := make(chan *Event, 8)
	c := srv.client(t, WithTopics(UserTopic("u_1")))
	c.HandleAny(func(_ context.Context, e *Event) {
		if strings.HasPrefix(e.Type, "__") {
			return
		}
		select {
		case events <- e:
		default:
		}
	})

	if err := c.Open(testContext(t)); err != nil {
		t.Fatalf("Open: %v", err)
	}
	srv.push("org:org_1", EventCampaignIdle, `{"event_type":"CAMPAIGN_IDLE","campaign_id":"c_1","status":"active","seq":4900}`)
	srv.push("user:u_1", EventNotificationCreated, `{"event_type":"NOTIFICATION_CREATED","notification_id":"n_1"}`)

	seen := map[string]*Event{}
	for len(seen) < 2 {
		select {
		case e := <-events:
			seen[e.Type] = e
		case <-time.After(2 * time.Second):
			t.Fatalf("only saw %d events", len(seen))
		}
	}

	if got := seen[EventCampaignIdle]; got.Topic != "org:org_1" || got.Seq != 4900 {
		t.Errorf("workspace event = topic %q seq %d", got.Topic, got.Seq)
	}
	if got := seen[EventNotificationCreated]; got.Topic != "user:u_1" || got.Seq != 0 {
		t.Errorf("user event = topic %q seq %d, want an unsequenced user-topic event", got.Topic, got.Seq)
	}
	if got := c.LastSeq(); got != 4900 {
		t.Errorf("LastSeq() = %d, want 4900", got)
	}
}

// TestResumeFailedAdvancesPosition: a client that keeps asking for an evicted
// position would never resume again, so the failure marker must move it on.
func TestResumeFailedAdvancesPosition(t *testing.T) {
	srv := newPhxServer(t)
	srv.onJoin = func(int, string) (string, string) { return "ok", helloResponse }

	done := make(chan struct{})
	c := srv.client(t, WithResumeFrom(10))
	var once sync.Once
	On(c, EventResumeFailed, func(context.Context, *ResumeFailed) { once.Do(func() { close(done) }) })

	if err := c.Open(testContext(t)); err != nil {
		t.Fatalf("Open: %v", err)
	}
	// The resume position the client sent must be its own, not the HELLO's.
	if got := srv.lastJoinPayload("org:org_1"); !strings.Contains(got, `"last_seq":10`) {
		t.Errorf("join payload = %s, want a resume token for seq 10", got)
	}

	srv.push("org:org_1", EventResumeFailed, `{"reason":"buffer_evicted","current_seq":5300}`)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("resume_failed was not dispatched")
	}
	// Poll: the read loop advances the position before the handler runs, but
	// nothing orders the two.
	deadline := time.Now().Add(2 * time.Second)
	for c.LastSeq() != 5300 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := c.LastSeq(); got != 5300 {
		t.Errorf("LastSeq() = %d, want 5300 after a buffer eviction", got)
	}
}

// TestHandshakeRefusalIsTerminalOnTheFirstConnection: a 403 upgrade carries no
// machine-readable body, and on a credential that has never connected it almost
// always means the credential.
func TestHandshakeRefusalIsTerminalOnTheFirstConnection(t *testing.T) {
	srv := newPhxServer(t)
	srv.upgradeStatus = http.StatusForbidden

	c := srv.client(t)
	err := c.Open(testContext(t))
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("Open error = %v, want ErrForbidden", err)
	}
	select {
	case <-c.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("the session did not stop")
	}
	if got := srv.handshakes(); got != 1 {
		t.Errorf("handshakes = %d, want 1", got)
	}
}

// --- event payload decoding ------------------------------------------------

func TestDecodeEventPayloads(t *testing.T) {
	t.Run("campaign idle", func(t *testing.T) {
		var e CampaignEvent
		decodeInto(t, `{"event_type":"CAMPAIGN_IDLE","org_id":"o_1","campaign_id":"c_1","name":"Q3","status":"active","seq":12,"timestamp":"2026-06-14T15:00:00Z"}`, &e)
		if e.EventType != EventCampaignIdle || e.CampaignID != "c_1" {
			t.Fatalf("decoded = %+v", e)
		}
		// An idle campaign is waiting, not finished.
		if e.Status != "active" {
			t.Errorf("Status = %q, want active", e.Status)
		}
		if e.Seq != 12 || e.Timestamp.IsZero() {
			t.Errorf("envelope lost: seq %d ts %v", e.Seq, e.Timestamp)
		}
	})

	t.Run("account sync state", func(t *testing.T) {
		var held AccountEvent
		decodeInto(t, `{"event_type":"ACCOUNT_SYNC_STATE","email_account_id":"a_1","status":"running","reason":"org_daily"}`, &held)
		if held.Status != SyncBackfillRunning || held.Reason != SyncThrottleOrgDaily {
			t.Errorf("throttled sync = %+v", held)
		}

		var released AccountEvent
		decodeInto(t, `{"event_type":"ACCOUNT_SYNC_STATE","email_account_id":"a_1","status":"complete"}`, &released)
		if released.Status != SyncBackfillComplete || released.Reason != "" {
			t.Errorf("released sync = %+v: the reason clears when the throttle lifts", released)
		}
	})

	t.Run("page hit", func(t *testing.T) {
		var e PageHitEvent
		decodeInto(t, `{"event_type":"PAGE_HIT","org_id":"o_1","contact_id":"ct_1","url":"https://example.com/pricing","title":"Pricing"}`, &e)
		if e.ContactID != "ct_1" || e.URL != "https://example.com/pricing" || e.Title != "Pricing" {
			t.Errorf("decoded = %+v", e)
		}
		// Page hits belong to the workspace, not to a member.
		if e.UserID != "" {
			t.Errorf("UserID = %q, want empty", e.UserID)
		}
	})

	t.Run("form submission", func(t *testing.T) {
		var e FormSubmissionEvent
		decodeInto(t, `{"event_type":"FORM_SUBMISSION_CREATED","org_id":"o_1","form_id":"f_1","submission_id":"s_1","contact_id":"ct_1"}`, &e)
		if e.FormID != "f_1" || e.SubmissionID != "s_1" || e.ContactID != "ct_1" {
			t.Errorf("decoded = %+v", e)
		}

		// An anonymous submission matched no contact.
		var anon FormSubmissionEvent
		decodeInto(t, `{"event_type":"FORM_SUBMISSION_CREATED","form_id":"f_1","submission_id":"s_2"}`, &anon)
		if anon.ContactID != "" {
			t.Errorf("ContactID = %q, want empty", anon.ContactID)
		}
	})

	t.Run("enriched engagement", func(t *testing.T) {
		var e EngagementEvent
		decodeInto(t, `{"event_type":"EMAIL_CLICKED","campaign_id":"c_1","contact_id":"ct_1","contact_email":"a@b.com",
			"original_url":"https://example.com/x","link_label":"Book a call","machine":true,
			"occurred_at":"2026-06-14T14:59:00Z","timestamp":"2026-06-14T15:00:00Z",
			"client":"Gmail","device_type":"mobile","country_code":"GB","city":"London"}`, &e)
		if e.URL != "https://example.com/x" || e.LinkLabel != "Book a call" {
			t.Errorf("click detail = %+v", e)
		}
		if !e.Machine {
			t.Error("Machine should be true for a scanner's fetch")
		}
		// occurred_at is when it happened; timestamp is when we published it.
		if !e.OccurredAt.Before(e.Timestamp) {
			t.Errorf("OccurredAt %v should precede Timestamp %v", e.OccurredAt, e.Timestamp)
		}
		if e.Client != "Gmail" || e.DeviceType != "mobile" || e.CountryCode != "GB" || e.City != "London" {
			t.Errorf("geo/client detail = %+v", e)
		}
	})

	t.Run("rate limited", func(t *testing.T) {
		var e RateLimited
		decodeInto(t, `{"category":"ws_message","retry_after_ms":21400}`, &e)
		if e.Category != RateLimitCategoryMessage || e.RetryAfterMS != 21400 {
			t.Errorf("decoded = %+v", e)
		}
	})

	t.Run("presence", func(t *testing.T) {
		var state PresenceState
		decodeInto(t, `{"u_1":{"metas":[{"online_at":1750000000,"name":"Ada","action":"replying","resource":"thread:t_1"}]}}`, &state)
		metas := state["u_1"].Metas
		if len(metas) != 1 || metas[0].Name != "Ada" || metas[0].Action != "replying" {
			t.Errorf("decoded = %+v", state)
		}

		var diff PresenceDiff
		decodeInto(t, `{"joins":{"u_2":{"metas":[{"online_at":1}]}},"leaves":{}}`, &diff)
		if len(diff.Joins) != 1 || len(diff.Leaves) != 0 {
			t.Errorf("diff = %+v", diff)
		}
	})
}

func decodeInto(t *testing.T, raw string, v any) {
	t.Helper()
	e := &Event{Raw: json.RawMessage(raw)}
	if err := e.Into(v); err != nil {
		t.Fatalf("Into: %v", err)
	}
}

// --- fake Phoenix server ---------------------------------------------------

// helloResponse is the workspace channel's join reply, which doubles as a
// HELLO.
const helloResponse = `{"org_id":"org_1","role":"owner","heartbeat_interval_ms":25000,"server_timeout_ms":60000,"seq":4821,"resume_supported":true}`

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// phxServer speaks just enough of RFC 6455 and the Phoenix v2 serializer for
// the client under test: it accepts the upgrade, answers phx_join with whatever
// onJoin returns, and can push events on any topic.
type phxServer struct {
	ln  net.Listener
	url string

	// upgradeStatus, when non-zero, refuses the upgrade with that HTTP status
	// instead of completing the handshake.
	upgradeStatus int
	// onJoin answers a phx_join. attempt counts this topic's joins on this
	// server, starting at 1.
	onJoin func(attempt int, topic string) (status, response string)

	mu         sync.Mutex
	shakes     int
	joinCounts map[string]int
	joinBodies map[string]string
	live       []net.Conn
}

func newPhxServer(t *testing.T) *phxServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &phxServer{
		ln:         ln,
		url:        "ws://" + ln.Addr().String() + "/socket/websocket",
		joinCounts: map[string]int{},
		joinBodies: map[string]string{},
	}
	t.Cleanup(func() {
		_ = ln.Close()
		s.mu.Lock()
		for _, c := range s.live {
			_ = c.Close()
		}
		s.mu.Unlock()
	})
	go s.accept()
	return s
}

// client builds a client pointed at this server, closed when the test ends.
func (s *phxServer) client(t *testing.T, opts ...Option) *Client {
	t.Helper()
	opts = append([]Option{WithURL(s.url), WithLogger(t.Logf)}, opts...)
	c := New("wmbly_test", "org_1", opts...)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func (s *phxServer) accept() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.live = append(s.live, conn)
		s.mu.Unlock()
		go s.serve(conn)
	}
}

func (s *phxServer) serve(conn net.Conn) {
	defer conn.Close()
	br := bufio.NewReader(conn)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	s.mu.Lock()
	s.shakes++
	status := s.upgradeStatus
	s.mu.Unlock()

	if status != 0 {
		fmt.Fprintf(conn, "HTTP/1.1 %d %s\r\nContent-Length: 0\r\nConnection: close\r\n\r\n",
			status, http.StatusText(status))
		return
	}

	sum := sha1.Sum([]byte(req.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	fmt.Fprintf(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n",
		base64.StdEncoding.EncodeToString(sum[:]))

	for {
		op, payload, err := wsReadFrame(br)
		if err != nil {
			return
		}
		if op == 0x8 { // close
			return
		}
		if op != 0x1 {
			continue
		}
		f, err := decodeFrame(payload)
		if err != nil {
			continue
		}
		switch f.Event {
		case evHeartbeat:
			s.send(conn, fmt.Sprintf(`[null,%q,"phoenix","phx_reply",{"status":"ok","response":{}}]`, f.Ref))
		case evJoin:
			s.mu.Lock()
			s.joinCounts[f.Topic]++
			n := s.joinCounts[f.Topic]
			s.joinBodies[f.Topic] = string(f.Payload)
			s.mu.Unlock()

			status, response := "ok", `{}`
			if s.onJoin != nil {
				status, response = s.onJoin(n, f.Topic)
			}
			s.send(conn, fmt.Sprintf(`[%q,%q,%q,"phx_reply",{"status":%q,"response":%s}]`,
				f.JoinRef, f.Ref, f.Topic, status, response))
		}
	}
}

// push sends a server event on every live connection.
func (s *phxServer) push(topic, event, payload string) {
	s.mu.Lock()
	conns := append([]net.Conn(nil), s.live...)
	s.mu.Unlock()
	for _, c := range conns {
		s.send(c, fmt.Sprintf(`[null,null,%q,%q,%s]`, topic, event, payload))
	}
}

func (s *phxServer) send(conn net.Conn, frame string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = wsWriteFrame(conn, 0x1, []byte(frame))
}

func (s *phxServer) handshakes() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shakes
}

func (s *phxServer) joins(topic string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.joinCounts[topic]
}

func (s *phxServer) lastJoinPayload(topic string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.joinBodies[topic]
}

// wsReadFrame reads one client frame. Client frames are always masked.
func wsReadFrame(br *bufio.Reader) (opcode byte, payload []byte, err error) {
	var h [2]byte
	if _, err = io.ReadFull(br, h[:]); err != nil {
		return 0, nil, err
	}
	opcode = h[0] & 0x0f
	masked := h[1]&0x80 != 0
	n := int(h[1] & 0x7f)
	switch n {
	case 126:
		var ext [2]byte
		if _, err = io.ReadFull(br, ext[:]); err != nil {
			return 0, nil, err
		}
		n = int(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err = io.ReadFull(br, ext[:]); err != nil {
			return 0, nil, err
		}
		n = int(binary.BigEndian.Uint64(ext[:]))
	}
	var mask [4]byte
	if masked {
		if _, err = io.ReadFull(br, mask[:]); err != nil {
			return 0, nil, err
		}
	}
	payload = make([]byte, n)
	if _, err = io.ReadFull(br, payload); err != nil {
		return 0, nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return opcode, payload, nil
}

// wsWriteFrame writes one unmasked server frame.
func wsWriteFrame(w io.Writer, opcode byte, payload []byte) error {
	n := len(payload)
	var hdr []byte
	switch {
	case n < 126:
		hdr = []byte{0x80 | opcode, byte(n)}
	case n < 1<<16:
		hdr = []byte{0x80 | opcode, 126, byte(n >> 8), byte(n)}
	default:
		hdr = make([]byte, 10)
		hdr[0] = 0x80 | opcode
		hdr[1] = 127
		binary.BigEndian.PutUint64(hdr[2:], uint64(n))
	}
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}
