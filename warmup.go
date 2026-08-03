package warmbly

import (
	"context"
	"encoding/json"
	"net/url"
	"time"
)

// WarmupRoutingService manages warmup routing rules: preferences for how the
// premium partner pool pairs senders with recipients, for example "send to
// Gmail recipients only from Google-hosted mailboxes".
//
// Rules are evaluated in Priority order and bias partner selection by Weight.
// They are preferences, not hard constraints: the pool falls back to an
// unmatched partner rather than sending nothing.
type WarmupRoutingService service

// Match modes for the sender and recipient sides of a routing rule.
const (
	// MatchAny matches every address on that side.
	MatchAny = "any"
	// MatchDomain matches an exact domain.
	MatchDomain = "domain"
	// MatchTLD matches a top-level domain.
	MatchTLD = "tld"
	// MatchProvider matches a mail provider classification, for example
	// "gmail" or "outlook".
	MatchProvider = "provider"
)

// WarmupRoutingRule biases partner selection in the warmup pool.
type WarmupRoutingRule struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	// Priority orders evaluation; lower runs first.
	Priority int `json:"priority"`
	// SenderMatchType is one of the Match* constants; SenderMatchValue is
	// ignored for [MatchAny].
	SenderMatchType  string `json:"sender_match_type"`
	SenderMatchValue string `json:"sender_match_value,omitempty"`
	// RecipientMatchType and RecipientMatchValue mirror the sender side.
	RecipientMatchType  string `json:"recipient_match_type"`
	RecipientMatchValue string `json:"recipient_match_value,omitempty"`
	// Weight scales how strongly a match is preferred.
	Weight  float64 `json:"weight"`
	Enabled bool    `json:"enabled"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WarmupRoutingRuleParams creates or replaces a routing rule. The API replaces
// the rule wholesale on update, so send the complete desired state.
type WarmupRoutingRuleParams struct {
	Name     string `json:"name"`
	Priority int    `json:"priority,omitempty"`
	// SenderMatchType and RecipientMatchType are Match* constants.
	SenderMatchType     string  `json:"sender_match_type"`
	SenderMatchValue    string  `json:"sender_match_value,omitempty"`
	RecipientMatchType  string  `json:"recipient_match_type"`
	RecipientMatchValue string  `json:"recipient_match_value,omitempty"`
	Weight              float64 `json:"weight,omitempty"`
	Enabled             *bool   `json:"enabled,omitempty"`
}

// List returns the workspace's routing rules in priority order.
func (s *WarmupRoutingService) List(ctx context.Context, opts ...RequestOption) ([]WarmupRoutingRule, *Response, error) {
	var out struct {
		Rules []WarmupRoutingRule `json:"rules"`
	}
	resp, err := s.client.get(ctx, "warmup/routing", &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Rules, resp, nil
}

// Create adds a routing rule.
func (s *WarmupRoutingService) Create(ctx context.Context, params *WarmupRoutingRuleParams, opts ...RequestOption) (*WarmupRoutingRule, *Response, error) {
	return send[WarmupRoutingRule](ctx, s.client, s.client.post, "warmup/routing", params, opts)
}

// Update replaces a routing rule.
func (s *WarmupRoutingService) Update(ctx context.Context, id string, params *WarmupRoutingRuleParams, opts ...RequestOption) (*WarmupRoutingRule, *Response, error) {
	return send[WarmupRoutingRule](ctx, s.client, s.client.patch, "warmup/routing/"+url.PathEscape(id), params, opts)
}

// Delete removes a routing rule.
func (s *WarmupRoutingService) Delete(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "warmup/routing/"+url.PathEscape(id), opts...)
}

// TaskService inspects and replays the send-task dead-letter queue: work that
// exhausted its retries and parked instead of being dropped.
//
// A replay re-dispatches real mail, so it needs the send permission.
type TaskService service

// DeadLetter is one parked task.
type DeadLetter struct {
	ID     string `json:"id"`
	TaskID string `json:"task_id"`
	// TaskType is what the task was doing, for example "campaign_email".
	TaskType string `json:"task_type"`
	// Payload is the original task body, kept so a replay is faithful.
	Payload     json.RawMessage `json:"payload,omitempty"`
	LastError   string          `json:"last_error,omitempty"`
	Attempts    int             `json:"attempts"`
	MaxAttempts int             `json:"max_attempts"`
	// Status is the queue state, for example "pending" or "replayed".
	Status      string     `json:"status"`
	NextRetryAt *time.Time `json:"next_retry_at,omitempty"`
	ReplayedAt  *time.Time `json:"replayed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// DeadLetterListParams filters the dead-letter queue.
type DeadLetterListParams struct {
	// Status filters by queue state, for example "pending".
	Status string
	// Limit caps the rows returned, from 1 to 200. Zero uses the server
	// default of 100.
	Limit int
}

func (p *DeadLetterListParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	setNonEmpty(q, "status", p.Status)
	setPositive(q, "limit", p.Limit)
	return q
}

// DeadLetters returns parked tasks.
func (s *TaskService) DeadLetters(ctx context.Context, params *DeadLetterListParams, opts ...RequestOption) ([]DeadLetter, *Response, error) {
	return fetchData[DeadLetter](ctx, s.client, withQuery("tasks/dlq", params.values()), opts)
}

// Replay re-dispatches a parked task. This sends real mail.
func (s *TaskService) Replay(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "tasks/dlq/"+url.PathEscape(id)+"/replay", nil, nil, opts...)
}
