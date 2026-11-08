package warmbly

import (
	"context"
	"net/url"
	"time"
)

// PlacementService runs inbox placement tests: a template or a campaign step
// is sent from one of your own sender mailboxes to a panel of seed mailboxes,
// and where each copy landed (inbox, a Gmail tab, spam, never arrived) is read
// back from the seed.
//
// A test sends one copy from one sender. A batch runs the same test from many
// senders, chosen by id list or by a server-side scope, so a fleet of
// thousands needs no id list. Batches report placement overall and grouped by
// sending domain, sending provider and recipient provider.
//
// Seed panels: "instance" is the panel the operator runs for every workspace
// (on the hosted product, Warmbly's own), "workspace" is your own seed
// inboxes (see [PlacementService.SetSeed]) and "cloud" is Warmbly Cloud's
// panel for a linked self-hosted instance. [PlacementService.Overview] says
// which are available.
//
// Reads (overview, tests, batches, coverage) need the read_analytics scope.
// Starting or canceling a test or batch and previewing a batch need the
// send_campaigns scope. Listing seeds needs read_emails, and changing a seed
// needs write_emails. A key limited to some mailboxes only ever tests from,
// and lists, those mailboxes.
type PlacementService service

// Seed panels in [PlacementTestParams.Panel], [PlacementTest.Panel] and
// [PlacementMonitor.Panel].
const (
	PlacementPanelInstance  = "instance"
	PlacementPanelWorkspace = "workspace"
	PlacementPanelCloud     = "cloud"
)

// Tracking choices when a test is created.
const (
	// PlacementTrackingCampaign copies the campaign's own open and link
	// tracking; an ad-hoc test has none. The default.
	PlacementTrackingCampaign = "campaign"
	PlacementTrackingOn       = "on"
	PlacementTrackingOff      = "off"
	// PlacementTrackingCompare runs the same copy twice, once tracked and once
	// not, so both see the same conditions.
	PlacementTrackingCompare = "compare"
)

// Pacing of a test's copies from one sender.
const (
	// PlacementPaceSpaced sends about a minute apart. The default.
	PlacementPaceSpaced = "spaced"
	// PlacementPaceQuick sends a few seconds apart, for a result in minutes.
	PlacementPaceQuick = "quick"
)

// Statuses of a [PlacementTest].
const (
	PlacementStatusRunning   = "running"
	PlacementStatusCompleted = "completed"
	PlacementStatusCancelled = "canceled"
	PlacementStatusFailed    = "failed"
)

// What started a [PlacementTest] ([PlacementTest.Origin]).
const (
	PlacementOriginManual  = "manual"
	PlacementOriginMonitor = "monitor"
	PlacementOriginAdmin   = "admin"
	PlacementOriginRemote  = "remote"
	PlacementOriginBatch   = "batch"
)

// Where one copy landed ([PlacementResult.Folder]). New values may appear;
// treat unknown ones as undecided.
const (
	PlacementFolderPending    = "pending"
	PlacementFolderInbox      = "inbox"
	PlacementFolderPromotions = "promotions"
	// PlacementFolderOther is a Gmail tab other than Promotions (Updates,
	// Social, Forums).
	PlacementFolderOther = "other"
	PlacementFolderSpam  = "spam"
	// PlacementFolderMissing is a copy that left the sender and never showed
	// up in the seed.
	PlacementFolderMissing = "missing"
	// PlacementFolderFailed is a copy that never left the sender.
	PlacementFolderFailed    = "failed"
	PlacementFolderCancelled = "canceled"
	PlacementFolderUnknown   = "unknown"
	PlacementFolderArchive   = "archive"
	PlacementFolderCustom    = "custom"
)

// Statuses of a [PlacementBatch].
const (
	PlacementBatchQueued                = "queued"
	PlacementBatchRunning               = "running"
	PlacementBatchCompleted             = "completed"
	PlacementBatchCompletedWithWarnings = "completed_with_warnings"
	PlacementBatchCancelled             = "canceled"
	PlacementBatchFailed                = "failed"
)

// Statuses of one sender in a batch ([PlacementBatchSender.Status] and the
// status filter of [PlacementBatchSendersParams]).
const (
	PlacementSenderQueued = "queued"
	// PlacementSenderDeferred could not run yet (no daily headroom, busy,
	// offline) and is retried later.
	PlacementSenderDeferred  = "deferred"
	PlacementSenderRunning   = "running"
	PlacementSenderCompleted = "completed"
	PlacementSenderSkipped   = "skipped"
	PlacementSenderFailed    = "failed"
	PlacementSenderCancelled = "cancelled" //nolint:misspell // wire spelling
)

// What a batch does with a sender that cannot run when its turn comes
// ([PlacementBatchParams.OnUnavailable]).
const (
	PlacementUnavailableSkip  = "skip"
	PlacementUnavailableDefer = "defer"
)

// Sender scopes a batch resolves on the server ([PlacementSenderScope.Type]).
const (
	PlacementScopeCampaign  = "campaign"
	PlacementScopeWorkspace = "workspace"
)

// Sampling modes ([PlacementSample.Mode]).
const (
	PlacementSampleAll         = "all"
	PlacementSampleRandom      = "random"
	PlacementSamplePercent     = "percent"
	PlacementSamplePerDomain   = "per_domain"
	PlacementSamplePerProvider = "per_provider"
)

// Sort orders of [PlacementBatchSendersParams.Sort].
const (
	// PlacementSortWorst lists the lowest inbox rate first. The default.
	PlacementSortWorst  = "worst"
	PlacementSortBest   = "best"
	PlacementSortEmail  = "email"
	PlacementSortStatus = "status"
)

// ObservationMetric is a rate together with the counts it was taken over, so
// a reader can see how much evidence stands behind a percentage.
type ObservationMetric struct {
	Version         string `json:"version"`
	Population      string `json:"population"`
	DenominatorKind string `json:"denominator_kind"`
	Unit            string `json:"unit"`
	Source          string `json:"source"`
	// PolicyVersion is the classification policy the counts were made under.
	PolicyVersion string `json:"classification_policy"`
	Numerator     int    `json:"numerator"`
	Denominator   int    `json:"denominator"`
	// Unresolved is the observed receipts that could not be classified.
	Unresolved       int    `json:"unresolved"`
	WindowDays       int    `json:"window_days,omitempty"`
	WindowBasis      string `json:"window_basis"`
	MissingnessBasis string `json:"missingness_basis"`
	// Value is nil until there is a denominator.
	Value *float64 `json:"value"`
	// Interval is the 95 percent Wilson interval, assuming independent
	// observations.
	Interval *ObservationInterval `json:"wilson_95_independence_interval"`
}

// ObservationInterval bounds an [ObservationMetric].
type ObservationInterval struct {
	Lower float64 `json:"lower"`
	Upper float64 `json:"upper"`
}

// ReceivedEvidence is what the receiving mailbox recorded about how a message
// arrived: the authentication verdicts on its headers.
type ReceivedEvidence struct {
	// DKIMVerification is the DKIM result Warmbly re-verified itself, when it
	// did.
	DKIMVerification *DiagnosticDKIMResult `json:"dkim_verification,omitempty"`
	Version          string                `json:"version"`
	Source           string                `json:"source"`
	Trust            string                `json:"trust"`
	ObservedAt       time.Time             `json:"observed_at"`
	SPF              string                `json:"spf"`
	DKIM             string                `json:"dkim"`
	DMARC            string                `json:"dmarc"`
	Alignment        string                `json:"alignment"`
	TLS              string                `json:"tls"`
	FromDomain       string                `json:"from_domain,omitempty"`
	// EnvelopeDomain, SigningDomain and Selector are claims read from the
	// headers, not verified facts.
	EnvelopeDomain               string `json:"envelope_domain_claim,omitempty"`
	SigningDomain                string `json:"signing_domain_claim,omitempty"`
	Selector                     string `json:"selector_claim,omitempty"`
	AuthenticationResultsPresent bool   `json:"authentication_results_present"`
	OneClickHeaders              bool   `json:"one_click_headers_present"`
	OneClickSigningClaim         bool   `json:"one_click_signing_claim"`
	OneClickCompliance           string `json:"one_click_compliance"`
}

// DiagnosticDKIMResult is a DKIM verification Warmbly ran itself.
type DiagnosticDKIMResult struct {
	DKIM          string    `json:"dkim"`
	Alignment     string    `json:"alignment"`
	SigningDomain string    `json:"signing_domain,omitempty"`
	Verifier      string    `json:"verifier"`
	ObservedAt    time.Time `json:"observed_at"`
}

// PlacementCounts is where a set of copies landed.
type PlacementCounts struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	Inbox      int `json:"inbox"`
	Promotions int `json:"promotions"`
	Other      int `json:"other"`
	Spam       int `json:"spam"`
	Missing    int `json:"missing"`
	Failed     int `json:"failed"`
	Canceled   int `json:"cancelled"` //nolint:misspell // wire spelling
	Unknown    int `json:"unknown"`
	Archived   int `json:"archive"`
	Custom     int `json:"custom"`
	// Observed and Classified are receipts the seed mailbox recorded and the
	// subset that could be classified.
	Observed   int `json:"observed_receipts"`
	Classified int `json:"classified_receipts"`
	Unresolved int `json:"unresolved"`
	// PrimaryMetric and NonSpamMetric carry the inbox-only and not-in-spam
	// rates with their evidence. Nil until something is delivered.
	PrimaryMetric *ObservationMetric `json:"primary_metric"`
	NonSpamMetric *ObservationMetric `json:"non_spam_metric"`
	// Delivered is every copy that left and got a verdict: inbox, promotions,
	// other, spam and missing.
	Delivered int `json:"delivered"`
	// InboxRate is the primary inbox share of Delivered, TabsRate the Gmail
	// tabs, SpamRate spam and MissingRate the copies never seen, all
	// percentages. Nil until something is delivered.
	InboxRate   *float64 `json:"inbox_rate"`
	TabsRate    *float64 `json:"tabs_rate"`
	SpamRate    *float64 `json:"spam_rate"`
	MissingRate *float64 `json:"missing_rate"`
}

// PlacementFamilyCounts is one host family's share of a test or batch.
type PlacementFamilyCounts struct {
	// Family is who hosts the seed: google_workspace, gmail, microsoft365,
	// outlook, yahoo and others.
	Family string          `json:"family"`
	Label  string          `json:"label"`
	Counts PlacementCounts `json:"counts"`
}

// PlacementTest is one run: a template sent from one sender to a seed panel.
// A list leaves the copy (BodyHTML, BodyPlain) out.
type PlacementTest struct {
	ID              string  `json:"id"`
	SenderAccountID *string `json:"sender_account_id"`
	SenderEmail     string  `json:"sender_email"`
	CreatedBy       *string `json:"created_by"`
	CampaignID      *string `json:"campaign_id"`
	SequenceID      *string `json:"sequence_id"`
	ContactID       *string `json:"contact_id"`
	// MonitorID names the [PlacementMonitor] that started the test.
	MonitorID *string `json:"monitor_id"`
	// BatchID names the batch that started the test.
	BatchID      *string `json:"batch_id"`
	Subject      string  `json:"subject"`
	BodyHTML     string  `json:"body_html,omitempty"`
	BodyPlain    string  `json:"body_plain,omitempty"`
	OpenTracking bool    `json:"open_tracking"`
	LinkTracking bool    `json:"link_tracking"`
	// CompareGroupID ties the two halves of a tracking comparison together.
	CompareGroupID *string `json:"compare_group_id"`
	// Origin is one of the PlacementOrigin* constants.
	Origin string `json:"origin"`
	// Panel is one of the PlacementPanel* constants.
	Panel string `json:"panel"`
	// Status is one of the PlacementStatus* constants.
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	// Pace is [PlacementPaceSpaced] or [PlacementPaceQuick].
	Pace string `json:"pace"`
	// CreditsCharged and CreditsRefunded are the legacy credit figures for
	// what the test cost past the monthly free allowance; ChargedUSD and
	// RefundedUSD are the same in dollars. A paid test is settled once it
	// finishes: the refund is what came back because no copy was delivered.
	CreditsCharged   int        `json:"credits_charged"`
	CreditsRefunded  int        `json:"credits_refunded"`
	CreditsSettledAt *time.Time `json:"credits_settled_at"`
	ChargedUSD       float64    `json:"charged_usd"`
	RefundedUSD      float64    `json:"refunded_usd"`
	CreatedAt        time.Time  `json:"created_at"`
	FinishedAt       *time.Time `json:"finished_at"`

	// Summary is where the test's copies landed, and Families the same per
	// host family.
	Summary  PlacementCounts         `json:"summary"`
	Families []PlacementFamilyCounts `json:"families"`
}

// PlacementResult is one copy to one seed, as [PlacementTestDetail] lists it.
type PlacementResult struct {
	// Seed is the seed's address, masked on the shared panels so the panel
	// cannot be listed and whitelisted.
	Seed        string `json:"seed"`
	Family      string `json:"family"`
	FamilyLabel string `json:"family_label"`
	// Folder is one of the PlacementFolder* constants.
	Folder      string     `json:"folder"`
	ScheduledAt *time.Time `json:"scheduled_at"`
	SentAt      *time.Time `json:"sent_at"`
	DetectedAt  *time.Time `json:"detected_at"`
	Error       string     `json:"error,omitempty"`
	// FirstFolder is where the copy first landed when it later moved.
	FirstFolder *string    `json:"first_folder,omitempty"`
	ObservedAt  *time.Time `json:"observed_at,omitempty"`
	// LateObservation is set when the copy was only seen after the classify
	// window.
	LateObservation bool              `json:"late_observation,omitempty"`
	Evidence        *ReceivedEvidence `json:"evidence"`
}

// ContentIssue is one finding of the content check over a test's copy.
type ContentIssue struct {
	// Severity is "warn" or "high".
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	// Field is "subject" or "body" when the issue lives in exactly one of
	// them, and empty otherwise.
	Field string `json:"field,omitempty"`
	// Spans are the exact fragments that triggered the issue, in reading
	// order.
	Spans []ContentSpan `json:"spans,omitempty"`
	// Suggestion is the concrete fix, in one line.
	Suggestion string `json:"suggestion,omitempty"`
}

// ContentSpan locates one fragment that triggered a [ContentIssue].
type ContentSpan struct {
	Field string `json:"field"`
	// Text is the fragment as written; Line the 1-based line it sits on and
	// Excerpt that whole line.
	Text    string `json:"text"`
	Line    int    `json:"line,omitempty"`
	Excerpt string `json:"excerpt,omitempty"`
}

// PlacementContentCheck is the rules pass over a test's copy.
type PlacementContentCheck struct {
	Score  int            `json:"score"`
	Issues []ContentIssue `json:"issues"`
}

// PlacementTestDetail is one test in full: every copy, the content check and,
// for a tracking comparison, the other half.
type PlacementTestDetail struct {
	PlacementTest
	Results []PlacementResult     `json:"results"`
	Content PlacementContentCheck `json:"content"`
	// Compare is the other half of a tracking comparison.
	Compare *PlacementTest `json:"compare,omitempty"`
}

// PlacementPanelFamily counts the seeds one host family contributes.
type PlacementPanelFamily struct {
	Family string `json:"family"`
	Label  string `json:"label"`
	Seeds  int    `json:"seeds"`
}

// PlacementPanel says whether the workspace can test on a seed panel.
type PlacementPanel struct {
	// Panel is one of the PlacementPanel* constants.
	Panel     string `json:"panel"`
	Available bool   `json:"available"`
	// Reason explains an unavailable panel in one sentence.
	Reason   string                 `json:"reason,omitempty"`
	Seeds    int                    `json:"seeds"`
	Families []PlacementPanelFamily `json:"families"`
	// Metered panels count against the monthly allowance.
	Metered bool `json:"metered"`
}

// PlacementUsage is a workspace's monthly allowance on the metered panels.
type PlacementUsage struct {
	Used int `json:"used"`
	// Limit is nil when the instance does not meter tests (self-hosted).
	Limit *int `json:"limit"`
	// PricePerTestUSD is what a test past the free allowance costs, zero when
	// tests cannot be paid for. BalanceUSD is the workspace's spendable
	// balance, nil when the balance is off. CreditsPerTest and CreditBalance
	// are the same in legacy credits.
	PricePerTestUSD float64   `json:"price_per_test_usd"`
	BalanceUSD      *float64  `json:"balance_usd"`
	CreditsPerTest  int       `json:"credits_per_test"`
	CreditBalance   *int      `json:"credit_balance"`
	PeriodStart     time.Time `json:"period_start"`
	PeriodEnd       time.Time `json:"period_end"`
}

// PlacementOverview is what a workspace can test on and how much of its
// allowance is left.
type PlacementOverview struct {
	Panels         []PlacementPanel `json:"panels"`
	Usage          PlacementUsage   `json:"usage"`
	WorkspaceSeeds int              `json:"workspace_seeds"`
	SeedsPerTest   int              `json:"seeds_per_test"`
	SpacingSeconds int              `json:"spacing_seconds"`
}

// PlacementSeed is one of the workspace's mailboxes as a seed candidate.
type PlacementSeed struct {
	EmailAccountID string `json:"email_account_id"`
	Email          string `json:"email"`
	Family         string `json:"family"`
	Label          string `json:"label"`
	Status         string `json:"status"`
	// Seed reports whether the mailbox is currently a workspace seed inbox.
	Seed bool `json:"seed"`
	// Blocker is why the mailbox cannot become a seed, empty when it can.
	Blocker string `json:"blocker,omitempty"`
}

// PlacementMonitor re-tests a campaign's first step on a schedule.
type PlacementMonitor struct {
	ID         string  `json:"id"`
	CampaignID string  `json:"campaign_id"`
	CreatedBy  *string `json:"created_by"`
	Enabled    bool    `json:"enabled"`
	// IntervalDays is how often the test repeats.
	IntervalDays int `json:"interval_days"`
	// Panel is one of the PlacementPanel* constants.
	Panel string `json:"panel"`
	// AlertBelow is the inbox percentage under which the monitor alerts.
	AlertBelow int `json:"alert_below"`
	// PauseOnAlert pauses the campaign when the monitor alerts.
	PauseOnAlert bool       `json:"pause_on_alert"`
	NextRunAt    time.Time  `json:"next_run_at"`
	LastRunAt    *time.Time `json:"last_run_at"`
	LastTestID   *string    `json:"last_test_id"`
	LastAlertAt  *time.Time `json:"last_alert_at"`
	LastError    string     `json:"last_error,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// PlacementSenderScope selects a batch's senders on the server. Filters
// narrow whichever scope is chosen.
type PlacementSenderScope struct {
	// Type is [PlacementScopeCampaign] or [PlacementScopeWorkspace].
	Type string `json:"type"`
	// CampaignID is the campaign whose sender pool is tested, for
	// [PlacementScopeCampaign].
	CampaignID *string `json:"campaign_id,omitempty"`
	// Providers keeps mailboxes hosted by these mailhost families, Domains
	// those sending from these domains and TagIDs those carrying these tags.
	Providers []string `json:"providers,omitempty"`
	Domains   []string `json:"domains,omitempty"`
	TagIDs    []string `json:"tag_ids,omitempty"`
	// IncludeInactive keeps disconnected mailboxes; they are skipped or
	// deferred when their turn comes.
	IncludeInactive bool `json:"include_inactive,omitempty"`
	// UntestedDays keeps mailboxes with no delivered placement test in the
	// last that many days.
	UntestedDays int `json:"untested_days,omitempty"`
}

// PlacementSample picks part of the resolved senders.
type PlacementSample struct {
	// Mode is one of the PlacementSample* constants. Empty means all.
	Mode    string `json:"mode"`
	Count   int    `json:"count,omitempty"`
	Percent int    `json:"percent,omitempty"`
	// Stratify spreads a random or percent sample across "provider" or
	// "domain" in proportion to each group's size.
	Stratify string `json:"stratify,omitempty"`
}

// PlacementBatchSelection is how a batch's senders were chosen.
type PlacementBatchSelection struct {
	// SenderAccountIDs is how many ids were listed explicitly.
	SenderAccountIDs int                   `json:"sender_account_ids,omitempty"`
	Scope            *PlacementSenderScope `json:"sender_scope,omitempty"`
	Sample           PlacementSample       `json:"sample"`
	// Matched is how many senders the scope resolved to before sampling.
	Matched int `json:"matched"`
}

// PlacementBatch runs one placement test from many senders.
type PlacementBatch struct {
	ID         string  `json:"id"`
	CreatedBy  *string `json:"created_by"`
	CampaignID *string `json:"campaign_id"`
	SequenceID *string `json:"sequence_id"`
	ContactID  *string `json:"contact_id"`
	Subject    string  `json:"subject"`
	BodyHTML   string  `json:"body_html,omitempty"`
	BodyPlain  string  `json:"body_plain,omitempty"`
	// Tracking is one of the PlacementTracking* constants.
	Tracking string `json:"tracking"`
	// Panel is one of the PlacementPanel* constants.
	Panel    string   `json:"panel"`
	Pace     string   `json:"pace"`
	Families []string `json:"families"`
	SeedIDs  []string `json:"seed_ids"`
	// OnUnavailable is [PlacementUnavailableSkip] or
	// [PlacementUnavailableDefer].
	OnUnavailable string                  `json:"on_unavailable"`
	Selection     PlacementBatchSelection `json:"selection"`
	SenderCount   int                     `json:"sender_count"`
	// MaxUSD is the most the batch may spend past the free allowance and
	// SpentUSD what it has spent; MaxCredits and CreditsSpent are the legacy
	// credit figures.
	MaxCredits   int     `json:"max_credits"`
	CreditsSpent int     `json:"credits_spent"`
	MaxUSD       float64 `json:"max_usd"`
	SpentUSD     float64 `json:"spent_usd"`
	// Status is one of the PlacementBatch* status constants.
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	// RetryUntil is how long a deferred sender keeps being retried.
	RetryUntil time.Time  `json:"retry_until"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

// PlacementBatchProgress counts a batch's senders by status.
type PlacementBatchProgress struct {
	Total     int `json:"total"`
	Queued    int `json:"queued"`
	Deferred  int `json:"deferred"`
	Running   int `json:"running"`
	Completed int `json:"completed"`
	Skipped   int `json:"skipped"`
	Failed    int `json:"failed"`
	Canceled  int `json:"cancelled"` //nolint:misspell // wire spelling
}

// PlacementBatchView is a batch with its progress and headline placement.
type PlacementBatchView struct {
	PlacementBatch
	Progress PlacementBatchProgress `json:"progress"`
	Summary  PlacementCounts        `json:"summary"`
}

// PlacementBatchGroup is a batch's placement for one sending domain or
// provider.
type PlacementBatchGroup struct {
	Key     string          `json:"key"`
	Label   string          `json:"label"`
	Senders int             `json:"senders"`
	Tested  int             `json:"tested"`
	Counts  PlacementCounts `json:"counts"`
}

// PlacementBatchMatrixRow is one sending domain's placement per recipient
// provider.
type PlacementBatchMatrixRow struct {
	Domain     string                  `json:"domain"`
	Recipients []PlacementFamilyCounts `json:"recipients"`
}

// PlacementBatchDetail is one batch in full.
type PlacementBatchDetail struct {
	PlacementBatchView
	// Untracked is the untracked half of a tracking comparison; Summary is
	// the tracked half, the copy the campaign really sends.
	Untracked  *PlacementCounts          `json:"untracked,omitempty"`
	Domains    []PlacementBatchGroup     `json:"domains"`
	Providers  []PlacementBatchGroup     `json:"providers"`
	Recipients []PlacementFamilyCounts   `json:"recipients"`
	Matrix     []PlacementBatchMatrixRow `json:"matrix"`
	Content    PlacementContentCheck     `json:"content"`
}

// PlacementBatchSender is one sender of a batch with where its copies landed.
type PlacementBatchSender struct {
	ID             string  `json:"id"`
	BatchID        string  `json:"batch_id"`
	EmailAccountID *string `json:"email_account_id"`
	SenderEmail    string  `json:"sender_email"`
	SenderDomain   string  `json:"sender_domain"`
	SenderFamily   string  `json:"sender_family"`
	// Status is one of the PlacementSender* constants.
	Status string `json:"status"`
	// Reason and Detail say why a sender was skipped, deferred or failed.
	Reason        string     `json:"reason,omitempty"`
	Detail        string     `json:"detail,omitempty"`
	Attempts      int        `json:"attempts"`
	NextAttemptAt time.Time  `json:"next_attempt_at"`
	StartedAt     *time.Time `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at"`

	SenderFamilyLabel string          `json:"sender_family_label"`
	Summary           PlacementCounts `json:"summary"`
	// TestIDs are the tests this sender ran.
	TestIDs []string `json:"test_ids"`
}

// PlacementBatchGroupCount is how many selected senders share a provider.
type PlacementBatchGroupCount struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Senders int    `json:"senders"`
}

// PlacementBatchPreview is what a batch would do, before it is started.
type PlacementBatchPreview struct {
	// Matched is how many senders the scope resolved to, Selected how many
	// the sample kept and Inactive how many of those are disconnected.
	Matched   int                        `json:"matched"`
	Selected  int                        `json:"selected"`
	Inactive  int                        `json:"inactive"`
	Domains   int                        `json:"domains"`
	Providers []PlacementBatchGroupCount `json:"providers"`
	// Variants is two for a tracking comparison, which doubles every count.
	Variants     int `json:"variants"`
	Tests        int `json:"tests"`
	SeedsPerTest int `json:"seeds_per_test"`
	// MaxSends is the most copies the batch sends; each sender's own daily
	// limit can only make it fewer.
	MaxSends int `json:"max_sends"`
	// Metered says whether the panel counts against the allowance.
	// FreeTests and PaidTests split Tests against it, and CostUSD is the most
	// the paid ones cost (Credits in legacy credits). Unmetered panels leave
	// them zero.
	Metered   bool           `json:"metered"`
	FreeTests int            `json:"free_tests"`
	PaidTests int            `json:"paid_tests"`
	CostUSD   float64        `json:"cost_usd"`
	Credits   int            `json:"credits"`
	Usage     PlacementUsage `json:"usage"`
	// SendersMax is the most senders one batch may hold, and Concurrency how
	// many send at once in this workspace.
	SendersMax  int `json:"senders_max"`
	Concurrency int `json:"concurrency"`
}

// PlacementCoverage reports how much of the workspace's connected fleet
// delivered a placement test recently.
type PlacementCoverage struct {
	Mailboxes   int `json:"mailboxes"`
	Tested7d    int `json:"tested_7d"`
	Tested30d   int `json:"tested_30d"`
	NeverTested int `json:"never_tested"`
}

// PlacementTestParams starts a placement test. SenderAccountID is required.
// Give either CampaignID and SequenceID (the campaign step's copy, rendered
// the way the campaign sends it, optionally for ContactID) or Subject with
// BodyHTML and BodyPlain.
//
// A tracking comparison ([PlacementTrackingCompare]) starts two tests, so the
// result holds two entries.
type PlacementTestParams struct {
	SenderAccountID string `json:"sender_account_id"`
	CampaignID      string `json:"campaign_id,omitempty"`
	SequenceID      string `json:"sequence_id,omitempty"`
	ContactID       string `json:"contact_id,omitempty"`
	Subject         string `json:"subject,omitempty"`
	BodyHTML        string `json:"body_html,omitempty"`
	BodyPlain       string `json:"body_plain,omitempty"`
	// Tracking is one of the PlacementTracking* constants. Default
	// [PlacementTrackingCampaign].
	Tracking string `json:"tracking,omitempty"`
	// Panel is one of the PlacementPanel* constants.
	Panel string `json:"panel,omitempty"`
	// SeedIDs and Families narrow the seeds the copies go to.
	SeedIDs  []string `json:"seed_ids,omitempty"`
	Families []string `json:"families,omitempty"`
	// Pace is [PlacementPaceSpaced] (default) or [PlacementPaceQuick].
	Pace string `json:"pace,omitempty"`
	// MaxUSD is the most you agree to pay for a test past the monthly free
	// allowance. Leave it nil to agree to nothing: a paid test is then
	// refused.
	MaxUSD *float64 `json:"max_usd,omitempty"`
}

// PlacementBatchParams starts or previews a batch. Choose the senders with
// exactly one of SenderAccountIDs and SenderScope, then optionally thin them
// out with Sample. The content fields work as in [PlacementTestParams].
type PlacementBatchParams struct {
	SenderAccountIDs []string              `json:"sender_account_ids,omitempty"`
	SenderScope      *PlacementSenderScope `json:"sender_scope,omitempty"`
	Sample           PlacementSample       `json:"sample"`
	CampaignID       string                `json:"campaign_id,omitempty"`
	SequenceID       string                `json:"sequence_id,omitempty"`
	ContactID        string                `json:"contact_id,omitempty"`
	Subject          string                `json:"subject,omitempty"`
	BodyHTML         string                `json:"body_html,omitempty"`
	BodyPlain        string                `json:"body_plain,omitempty"`
	Tracking         string                `json:"tracking,omitempty"`
	Panel            string                `json:"panel,omitempty"`
	Pace             string                `json:"pace,omitempty"`
	Families         []string              `json:"families,omitempty"`
	SeedIDs          []string              `json:"seed_ids,omitempty"`
	// OnUnavailable is [PlacementUnavailableSkip] or
	// [PlacementUnavailableDefer] (the default is chosen by the server).
	OnUnavailable string `json:"on_unavailable,omitempty"`
	// MaxUSD is the most you agree to pay across the whole batch for tests
	// past the monthly free allowance.
	MaxUSD *float64 `json:"max_usd,omitempty"`
}

// PlacementMonitorParams configures a campaign's scheduled placement test.
// Nil fields keep the stored value, or the default on a new monitor. A new or
// re-enabled monitor runs its first test within minutes.
type PlacementMonitorParams struct {
	Enabled      *bool `json:"enabled,omitempty"`
	IntervalDays *int  `json:"interval_days,omitempty"`
	// Panel is one of the PlacementPanel* constants.
	Panel *string `json:"panel,omitempty"`
	// AlertBelow is the inbox percentage under which the monitor alerts.
	AlertBelow   *int  `json:"alert_below,omitempty"`
	PauseOnAlert *bool `json:"pause_on_alert,omitempty"`
}

// PlacementListParams filters and paginates a list of tests.
type PlacementListParams struct {
	ListOptions
	// CampaignID restricts the list to tests of one campaign.
	CampaignID string
}

func (p *PlacementListParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	setNonEmpty(q, "campaign_id", p.CampaignID)
	return q
}

// PlacementBatchSendersParams filters and paginates a batch's senders.
type PlacementBatchSendersParams struct {
	ListOptions
	// Status restricts the list to one PlacementSender* status.
	Status string
	// Query searches the sender address.
	Query string
	// Sort is one of the PlacementSort* constants.
	Sort string
}

func (p *PlacementBatchSendersParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	setNonEmpty(q, "status", p.Status)
	setNonEmpty(q, "q", p.Query)
	setNonEmpty(q, "sort", p.Sort)
	return q
}

// placementEnvelope is the {"data": ...} wrapper the placement routes answer
// with.
type placementEnvelope[T any] struct {
	Data T `json:"data"`
}

func placementFetch[T any](ctx context.Context, c *Client, path string, opts []RequestOption) (T, *Response, error) {
	var env placementEnvelope[T]
	resp, err := c.get(ctx, path, &env, opts...)
	return env.Data, resp, err
}

func placementSend[T any](ctx context.Context, verb bodyVerb, path string, body any, opts []RequestOption) (T, *Response, error) {
	var env placementEnvelope[T]
	resp, err := verb(ctx, path, body, &env, opts...)
	return env.Data, resp, err
}

// Overview returns the seed panels the workspace can test on and how much of
// its monthly allowance is left. Needs the read_analytics scope.
func (s *PlacementService) Overview(ctx context.Context, opts ...RequestOption) (*PlacementOverview, *Response, error) {
	out, resp, err := placementFetch[PlacementOverview](ctx, s.client, "placement/overview", opts)
	if err != nil {
		return nil, resp, err
	}
	return &out, resp, nil
}

// Coverage reports how much of the workspace's connected fleet delivered a
// placement test in the last 7 and 30 days. Needs the read_analytics scope.
func (s *PlacementService) Coverage(ctx context.Context, opts ...RequestOption) (*PlacementCoverage, *Response, error) {
	out, resp, err := placementFetch[PlacementCoverage](ctx, s.client, "placement/coverage", opts)
	if err != nil {
		return nil, resp, err
	}
	return &out, resp, nil
}

// ListTests returns a page of the workspace's tests, newest first. The copy
// is left out of each. Needs the read_analytics scope. Limit is 1 to 100 (default 25).
func (s *PlacementService) ListTests(ctx context.Context, params *PlacementListParams, opts ...RequestOption) (*Page[PlacementTest], error) {
	return listJSON[PlacementTest](ctx, s.client, "placement/tests", params.values(), opts...)
}

// GetTest returns one test with every copy, the content check and, for a
// tracking comparison, the other half. Needs the read_analytics scope.
func (s *PlacementService) GetTest(ctx context.Context, id string, opts ...RequestOption) (*PlacementTestDetail, *Response, error) {
	out, resp, err := placementFetch[PlacementTestDetail](ctx, s.client, "placement/tests/"+url.PathEscape(id), opts)
	if err != nil {
		return nil, resp, err
	}
	return &out, resp, nil
}

// CreateTest starts a test from one sender and returns it: one test, or two
// for a tracking comparison. Needs the send_campaigns scope. A test past the monthly
// free allowance is paid for from the workspace balance up to
// [PlacementTestParams.MaxUSD]. It honors [WithIdempotencyKey].
func (s *PlacementService) CreateTest(ctx context.Context, params *PlacementTestParams, opts ...RequestOption) ([]PlacementTest, *Response, error) {
	out, resp, err := placementSend[[]PlacementTest](ctx, s.client.post, "placement/tests", params, opts)
	if err != nil {
		return nil, resp, err
	}
	if out == nil {
		out = []PlacementTest{}
	}
	return out, resp, nil
}

// CancelTest stops the copies of a test that have not been sent yet. Needs
// the send_campaigns scope.
func (s *PlacementService) CancelTest(ctx context.Context, id string, opts ...RequestOption) (*PlacementTest, *Response, error) {
	out, resp, err := placementSend[PlacementTest](ctx, s.client.post, "placement/tests/"+url.PathEscape(id)+"/cancel", nil, opts)
	if err != nil {
		return nil, resp, err
	}
	return &out, resp, nil
}

// ListBatches returns a page of the workspace's batches, newest first. Needs
// the read_analytics scope. Limit is 1 to 100 (default 25).
func (s *PlacementService) ListBatches(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[PlacementBatchView], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[PlacementBatchView](ctx, s.client, "placement/batches", q, opts...)
}

// GetBatch returns one batch with its placement overall and grouped by
// sending domain, sending provider and recipient provider. Needs
// the read_analytics scope.
func (s *PlacementService) GetBatch(ctx context.Context, id string, opts ...RequestOption) (*PlacementBatchDetail, *Response, error) {
	out, resp, err := placementFetch[PlacementBatchDetail](ctx, s.client, "placement/batches/"+url.PathEscape(id), opts)
	if err != nil {
		return nil, resp, err
	}
	return &out, resp, nil
}

// BatchSenders returns a page of a batch's senders with where each one's
// copies landed, worst inbox rate first unless sorted otherwise. Needs
// the read_analytics scope.
func (s *PlacementService) BatchSenders(ctx context.Context, id string, params *PlacementBatchSendersParams, opts ...RequestOption) (*Page[PlacementBatchSender], error) {
	return listJSON[PlacementBatchSender](ctx, s.client, "placement/batches/"+url.PathEscape(id)+"/senders", params.values(), opts...)
}

// PreviewBatch reports how many senders, tests, sends and credits a batch
// would come to. It starts nothing. Needs the send_campaigns scope.
func (s *PlacementService) PreviewBatch(ctx context.Context, params *PlacementBatchParams, opts ...RequestOption) (*PlacementBatchPreview, *Response, error) {
	out, resp, err := placementSend[PlacementBatchPreview](ctx, s.client.post, "placement/batches/preview", params, opts)
	if err != nil {
		return nil, resp, err
	}
	return &out, resp, nil
}

// CreateBatch snapshots the senders and queues the batch; the backend starts
// its senders a few at a time. Needs the send_campaigns scope. It honors
// [WithIdempotencyKey].
func (s *PlacementService) CreateBatch(ctx context.Context, params *PlacementBatchParams, opts ...RequestOption) (*PlacementBatchView, *Response, error) {
	out, resp, err := placementSend[PlacementBatchView](ctx, s.client.post, "placement/batches", params, opts)
	if err != nil {
		return nil, resp, err
	}
	return &out, resp, nil
}

// CancelBatch stops a batch. Copies already sent keep being classified.
// Needs the send_campaigns scope.
func (s *PlacementService) CancelBatch(ctx context.Context, id string, opts ...RequestOption) (*PlacementBatchView, *Response, error) {
	out, resp, err := placementSend[PlacementBatchView](ctx, s.client.post, "placement/batches/"+url.PathEscape(id)+"/cancel", nil, opts)
	if err != nil {
		return nil, resp, err
	}
	return &out, resp, nil
}

// Seeds lists the workspace's mailboxes and which of them are its seed
// inboxes. Needs the read_emails scope. The list is not paginated.
func (s *PlacementService) Seeds(ctx context.Context, opts ...RequestOption) ([]PlacementSeed, *Response, error) {
	return fetchData[PlacementSeed](ctx, s.client, "placement/seeds", opts)
}

// SetSeed makes a workspace mailbox a seed inbox, or not. A seed inbox stops
// warming up. Needs the write_emails scope.
func (s *PlacementService) SetSeed(ctx context.Context, emailAccountID string, seed bool, opts ...RequestOption) (*PlacementSeed, *Response, error) {
	body := struct {
		Seed bool `json:"seed"`
	}{Seed: seed}
	out, resp, err := placementSend[PlacementSeed](ctx, s.client.put, "placement/seeds/"+url.PathEscape(emailAccountID), body, opts)
	if err != nil {
		return nil, resp, err
	}
	return &out, resp, nil
}
