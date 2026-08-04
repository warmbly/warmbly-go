package warmbly

import (
	"context"
	"encoding/json"
	"net/url"
	"time"
)

// AdvisorService reads and acts on the Advisor: continuous checks on the
// workspace's sending posture, surfaced as findings on the thing they are
// about.
//
// A finding either carries a one-click [AdvisorAction], can be handed to an
// agent with [AdvisorService.AgentFix], or comes with [AdvisorFinding.Steps]
// and [AdvisorFinding.Snippets] for what the platform cannot reach itself, such
// as a DNS record.
//
// Applying a fix runs through the same permission checks as doing it by hand,
// so a viewer sees the advice and gets a clean 403 if they try to apply it.
type AdvisorService service

// Finding severities returned in [AdvisorFinding.Severity].
const (
	AdvisorCritical = "critical"
	AdvisorHigh     = "high"
	AdvisorMedium   = "medium"
	AdvisorLow      = "low"
)

// Finding categories returned in [AdvisorFinding.Category].
const (
	AdvisorCategoryDeliverability = "deliverability"
	AdvisorCategoryMailbox        = "mailbox"
	AdvisorCategoryWarmup         = "warmup"
	AdvisorCategoryCampaign       = "campaign"
	AdvisorCategoryCopy           = "copy"
	AdvisorCategoryList           = "list"
)

// Surfaces a finding is shown on, returned in [AdvisorFinding.Surface].
const (
	AdvisorSurfaceCampaigns      = "campaigns"
	AdvisorSurfaceMailboxes      = "emails"
	AdvisorSurfaceDeliverability = "deliverability"
	AdvisorSurfaceContacts       = "contacts"
	AdvisorSurfaceAnalytics      = "analytics"
	AdvisorSurfaceSettings       = "settings"
)

// Finding states returned in [AdvisorFinding.Status].
const (
	AdvisorStatusOpen      = "open"
	AdvisorStatusSnoozed   = "snoozed"
	AdvisorStatusDismissed = "dismissed"
	AdvisorStatusApplied   = "applied"
	AdvisorStatusResolved  = "resolved"
)

// AdvisorFinding is one piece of advice about one entity.
type AdvisorFinding struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`

	// DetectorKey identifies the check that produced the finding.
	DetectorKey string `json:"detector_key"`
	// Category is one of the AdvisorCategory* constants, Severity one of the
	// Advisor{Critical,High,Medium,Low} constants, and Surface one of the
	// AdvisorSurface* constants.
	Category string `json:"category"`
	Severity string `json:"severity"`
	Surface  string `json:"surface"`

	// EntityType, EntityID and EntityLabel name what the finding is about.
	EntityType  string  `json:"entity_type,omitempty"`
	EntityID    *string `json:"entity_id,omitempty"`
	EntityLabel string  `json:"entity_label,omitempty"`

	// ParentType and ParentID name where the finding belongs when that differs
	// from its subject: a step's copy problem belongs to its campaign.
	ParentType string  `json:"parent_type,omitempty"`
	ParentID   *string `json:"parent_id,omitempty"`

	// Status is one of the AdvisorStatus* constants.
	Status string `json:"status"`
	// Impact ranks the finding against the others.
	Impact int `json:"impact"`

	Title string `json:"title"`
	// GroupTitle names the finding when several of its kind are listed
	// together, with a {count} placeholder. Empty means it always stands alone.
	GroupTitle string `json:"group_title,omitempty"`
	Detail     string `json:"detail"`
	Remedy     string `json:"remedy"`
	// Steps is the manual how-to, set only on findings with no one-click fix.
	Steps []string `json:"steps,omitempty"`
	// AgentFixable reports whether [AdvisorService.AgentFix] could resolve
	// this. It is false for anything outside the platform, such as a DNS
	// record, so a client shows the steps rather than a button that cannot
	// succeed.
	AgentFixable bool `json:"agent_fixable"`
	// Snippets are exact values to paste somewhere the platform cannot reach.
	Snippets []AdvisorSnippet `json:"snippets,omitempty"`
	// Narrated is false while the finding still shows built-in fallback copy
	// rather than AI narration. It is fully usable either way.
	Narrated bool `json:"narrated"`

	// Evidence is the detector's raw supporting data.
	Evidence json.RawMessage `json:"evidence,omitempty"`
	// Action is the one-click remedy, when there is one.
	Action *AdvisorAction `json:"action,omitempty"`

	FirstSeenAt time.Time  `json:"first_seen_at"`
	LastSeenAt  time.Time  `json:"last_seen_at"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`

	SnoozedUntil  *time.Time `json:"snoozed_until,omitempty"`
	DismissedAt   *time.Time `json:"dismissed_at,omitempty"`
	DismissReason string     `json:"dismiss_reason,omitempty"`

	AppliedAt     *time.Time `json:"applied_at,omitempty"`
	AppliedBy     *string    `json:"applied_by,omitempty"`
	AppliedResult string     `json:"applied_result,omitempty"`
}

// AdvisorAction is a finding's one-click remedy. It runs as the calling member
// with their permissions enforced.
type AdvisorAction struct {
	// Tool is the registry name of what would run, and Args its payload.
	Tool string          `json:"tool"`
	Args json.RawMessage `json:"args"`
	// Label is the button text.
	Label string `json:"label"`
	// Auto marks a fix autopilot may apply unattended. It is true only for a
	// bounded, reversible settings change in the safe direction.
	Auto bool `json:"auto,omitempty"`
	// Preview is the exact before and after.
	Preview []AdvisorPreviewChange `json:"preview,omitempty"`
	// Undo, when set, reverts the action.
	Undo *AdvisorUndo `json:"undo,omitempty"`
}

// AdvisorPreviewChange is one field an action would change.
type AdvisorPreviewChange struct {
	Field string `json:"field"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// AdvisorUndo reverts an applied action.
type AdvisorUndo struct {
	Tool string          `json:"tool"`
	Args json.RawMessage `json:"args"`
}

// AdvisorSnippet is an exact value to paste into something outside Warmbly,
// such as a DNS record.
type AdvisorSnippet struct {
	// Label names the field in the target system's own words.
	Label string `json:"label"`
	Value string `json:"value"`
	// Note is the caveat that trips people up on this specific field.
	Note string `json:"note,omitempty"`
}

// AdvisorSummary is the workspace rollup behind the health score and the
// per-tab badges.
type AdvisorSummary struct {
	// Score is 0-100, falling as open findings accumulate and weighted so one
	// critical finding outweighs a pile of low-severity nits.
	Score    int                   `json:"score"`
	Total    int                   `json:"total"`
	Critical int                   `json:"critical"`
	High     int                   `json:"high"`
	Medium   int                   `json:"medium"`
	Low      int                   `json:"low"`
	Surfaces []AdvisorSurfaceCount `json:"surfaces"`
	// LastRunAt is nil before the first evaluation.
	LastRunAt *time.Time `json:"last_run_at,omitempty"`
}

// AdvisorSurfaceCount is one surface's badge payload.
type AdvisorSurfaceCount struct {
	// Surface is one of the AdvisorSurface* constants.
	Surface string `json:"surface"`
	// Total is every open finding on the surface; Critical and High set the
	// badge's tone.
	Total    int `json:"total"`
	Critical int `json:"critical"`
	High     int `json:"high"`
}

// AdvisorSettings controls which checks run for the workspace. Reading it is an
// analytics read; changing it is workspace governance and is session-only.
type AdvisorSettings struct {
	OrganizationID string `json:"organization_id"`
	Enabled        bool   `json:"enabled"`
	// MutedCategories holds AdvisorCategory* values, MutedDetectors holds
	// detector keys.
	MutedCategories []string `json:"muted_categories"`
	MutedDetectors  []string `json:"muted_detectors"`
	// MinSeverity hides anything below it.
	MinSeverity string `json:"min_severity"`
	// Autopilot applies auto-safe fixes on its own. It is off by default.
	Autopilot bool `json:"autopilot"`
	// AutopilotActorID is the member autopilot acts as. It is set to whoever
	// switched it on, and autopilot stops if they leave.
	AutopilotActorID *string   `json:"autopilot_actor_id,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// AdvisorAgentResult is what an agent fix reports back.
type AdvisorAgentResult struct {
	FindingID string `json:"finding_id"`
	// Applied is true only when the agent actually changed something. A run
	// that looked and decided nothing was wrong reports false and the finding
	// stays open.
	Applied bool `json:"applied"`
	// Summary is the agent's own account of what it did.
	Summary string `json:"summary"`
	// Steps are the calls it made, in order, so they can be checked against
	// the audit log.
	Steps []string `json:"steps,omitempty"`
}

// AdvisorListParams filters the findings list.
type AdvisorListParams struct {
	// Surface is one of the AdvisorSurface* constants.
	Surface string
	// Category is one of the AdvisorCategory* constants.
	Category string
	// EntityType and EntityID narrow to one entity.
	EntityType string
	EntityID   string
	// Statuses matches any of the AdvisorStatus* constants. The default is
	// open findings.
	Statuses []string
	// Limit caps the rows returned, from 1 to 200.
	Limit int
}

func (p *AdvisorListParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	setNonEmpty(q, "surface", p.Surface)
	setNonEmpty(q, "category", p.Category)
	setNonEmpty(q, "entity_type", p.EntityType)
	setNonEmpty(q, "entity_id", p.EntityID)
	setCSV(q, "status", p.Statuses)
	setPositive(q, "limit", p.Limit)
	return q
}

// Recommendations returns the workspace's findings.
func (s *AdvisorService) Recommendations(ctx context.Context, params *AdvisorListParams, opts ...RequestOption) ([]AdvisorFinding, *Response, error) {
	return fetchData[AdvisorFinding](ctx, s.client, withQuery("advisor/recommendations", params.values()), opts)
}

// Summary returns the workspace health score and the per-surface badge counts.
func (s *AdvisorService) Summary(ctx context.Context, opts ...RequestOption) (*AdvisorSummary, *Response, error) {
	return fetch[AdvisorSummary](ctx, s.client, "advisor/summary", opts)
}

// Refresh triggers a re-evaluation and returns the summary. The re-run happens
// in the background, so the summary may still reflect the previous pass.
func (s *AdvisorService) Refresh(ctx context.Context, opts ...RequestOption) (*AdvisorSummary, *Response, error) {
	return send[AdvisorSummary](ctx, s.client.post, "advisor/refresh", nil, opts)
}

// Apply performs a finding's one-click remedy and returns the updated finding.
// It runs as the calling member, so it fails with a 403 if they could not make
// the same change by hand.
func (s *AdvisorService) Apply(ctx context.Context, id string, opts ...RequestOption) (*AdvisorFinding, *Response, error) {
	return send[AdvisorFinding](ctx, s.client.post, "advisor/recommendations/"+url.PathEscape(id)+"/apply", nil, opts)
}

// Undo reverts an applied remedy.
func (s *AdvisorService) Undo(ctx context.Context, id string, opts ...RequestOption) (*AdvisorFinding, *Response, error) {
	return send[AdvisorFinding](ctx, s.client.post, "advisor/recommendations/"+url.PathEscape(id)+"/undo", nil, opts)
}

// AgentFix hands a finding to a bounded agent for the cases a settings change
// cannot resolve. It spends AI credits, acts as the calling member inside their
// permissions, and reports the calls it actually made.
//
// This route is session-only: it is not reachable with an API key.
func (s *AdvisorService) AgentFix(ctx context.Context, id string, opts ...RequestOption) (*AdvisorAgentResult, *Response, error) {
	return send[AdvisorAgentResult](ctx, s.client.post, "advisor/recommendations/"+url.PathEscape(id)+"/agent-fix", nil, opts)
}

// Snooze hides a finding for a bounded number of days, from 1 to 90. An
// unbounded snooze would be a dismissal in disguise, so use
// [AdvisorService.Dismiss] for that.
func (s *AdvisorService) Snooze(ctx context.Context, id string, days int, opts ...RequestOption) (*Response, error) {
	body := struct {
		Days int `json:"days"`
	}{Days: days}
	return s.client.post(ctx, "advisor/recommendations/"+url.PathEscape(id)+"/snooze", body, nil, opts...)
}

// Dismiss rejects a finding. The dismissal sticks until the underlying
// condition clears and later recurs.
func (s *AdvisorService) Dismiss(ctx context.Context, id, reason string, opts ...RequestOption) (*Response, error) {
	body := struct {
		Reason string `json:"reason,omitempty"`
	}{Reason: reason}
	return s.client.post(ctx, "advisor/recommendations/"+url.PathEscape(id)+"/dismiss", body, nil, opts...)
}

// Feedback records whether a finding was helpful, which feeds detector tuning.
func (s *AdvisorService) Feedback(ctx context.Context, id string, helpful bool, reason string, opts ...RequestOption) (*Response, error) {
	body := struct {
		Helpful bool   `json:"helpful"`
		Reason  string `json:"reason,omitempty"`
	}{Helpful: helpful, Reason: reason}
	return s.client.post(ctx, "advisor/recommendations/"+url.PathEscape(id)+"/feedback", body, nil, opts...)
}

// Settings returns which Advisor checks are enabled for the workspace.
func (s *AdvisorService) Settings(ctx context.Context, opts ...RequestOption) (*AdvisorSettings, *Response, error) {
	return fetch[AdvisorSettings](ctx, s.client, "advisor/settings", opts)
}

// UpdateSettings replaces the Advisor configuration. Silencing checks for a
// whole workspace is governance, so this route is session-only and needs the
// manage-settings permission.
func (s *AdvisorService) UpdateSettings(ctx context.Context, settings *AdvisorSettings, opts ...RequestOption) (*AdvisorSettings, *Response, error) {
	return send[AdvisorSettings](ctx, s.client.patch, "advisor/settings", settings, opts)
}
