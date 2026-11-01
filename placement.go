package warmbly

import (
	"context"
	"net/url"
	"time"
)

// PlacementService runs inbox placement tests: a message is sent from one of
// your mailboxes to a panel of seed inboxes and the service reports where each
// copy landed (inbox, a Gmail tab, spam, or nowhere).
//
// There are three seed panels, named by the Placement panel constants:
// the instance panel the operator runs for every workspace, the workspace's
// own seed inboxes (mailboxes you mark with [PlacementService.SetSeed]), and
// Warmbly Cloud's panel for a linked self-hosted instance. The instance and
// cloud panels are metered against a monthly allowance, see
// [PlacementService.Overview].
//
// Three shapes of run exist:
//
//   - A test ([PlacementService.CreateTest]) sends one message from one
//     mailbox. A tracking comparison yields two tests.
//   - A batch ([PlacementService.CreateBatch]) runs one test per sender across
//     many mailboxes, selected by id or by a server-side scope, and reports
//     the placement by sending domain, sending provider and recipient
//     provider.
//   - A monitor ([CampaignService.PlacementMonitor]) re-tests a campaign's
//     first step on a schedule.
//
// Reads need [PermReadAnalytics]. Starting or canceling tests and batches
// needs [PermSendCampaigns]. Listing seed inboxes needs [PermReadEmails] and
// changing one needs [PermWriteEmails].
type PlacementService service

// Seed panels a placement test can send to.
const (
	// PlacementPanelInstance is the seed panel the operator runs for every
	// workspace on the instance. The default.
	PlacementPanelInstance = "instance"
	// PlacementPanelWorkspace is the workspace's own seed inboxes.
	PlacementPanelWorkspace = "workspace"
	// PlacementPanelCloud is Warmbly Cloud's panel, for a linked self-hosted
	// instance.
	PlacementPanelCloud = "cloud"
)

// Statuses of a [PlacementTest].
const (
	PlacementStatusRunning   = "running"
	PlacementStatusCompleted = "completed"
	PlacementStatusCancelled = "cancelled" //nolint:misspell // wire value: the API sends "cancelled"
	PlacementStatusFailed    = "failed"
)

// What started a [PlacementTest] ([PlacementTest.Origin]).
const (
	PlacementOriginManual  = "manual"
	PlacementOriginMonitor = "monitor"
	PlacementOriginAdmin   = "admin"
	// PlacementOriginRemote is a test the cloud runs for a linked instance.
	PlacementOriginRemote = "remote"
	// PlacementOriginBatch is a test a placement batch started for one sender.
	PlacementOriginBatch = "batch"
)

// Where one probe landed ([PlacementResult.Folder]). New values may appear;
// treat an unknown one as unresolved.
const (
	PlacementFolderPending    = "pending"
	PlacementFolderInbox      = "inbox"
	PlacementFolderPromotions = "promotions"
	// PlacementFolderOther is a Gmail tab other than Promotions (Updates,
	// Social, Forums).
	PlacementFolderOther = "other"
	PlacementFolderSpam  = "spam"
	// PlacementFolderMissing is a copy that left the sender and never showed
	// up in the seed within the classify window.
	PlacementFolderMissing = "missing"
	// PlacementFolderFailed is a copy that never left the sender.
	PlacementFolderFailed    = "failed"
	PlacementFolderCancelled = "cancelled" //nolint:misspell // wire value: the API sends "cancelled"
	PlacementFolderUnknown   = "unknown"
	PlacementFolderArchive   = "archive"
	PlacementFolderCustom    = "custom"
)

// Tracking choices when a test or batch is created.
const (
	// PlacementTrackingCampaign copies the campaign's own open and click
	// tracking; an ad-hoc test has none. The default.
	PlacementTrackingCampaign = "campaign"
	PlacementTrackingOn       = "on"
	PlacementTrackingOff      = "off"
	// PlacementTrackingCompare runs the same copy twice, once tracked and once
	// not, interleaved per seed so both see the same conditions.
	PlacementTrackingCompare = "compare"
)

// Pacing of a test's copies from one sender.
const (
	// PlacementPaceSpaced sends at the instance's spacing, about a minute
	// apart. The default, and the only pace a batch accepts.
	PlacementPaceSpaced = "spaced"
	// PlacementPaceQuick sends a few seconds apart, for a result in minutes.
	PlacementPaceQuick = "quick"
)

// Statuses of a [PlacementBatch].
const (
	PlacementBatchQueued                = "queued"
	PlacementBatchRunning               = "running"
	PlacementBatchCompleted             = "completed"
	PlacementBatchCompletedWithWarnings = "completed_with_warnings"
	PlacementBatchCancelled             = "cancelled" //nolint:misspell // wire value: the API sends "cancelled"
	PlacementBatchFailed                = "failed"
)

// Statuses of one sender in a batch ([PlacementBatchSender.Status]).
const (
	PlacementSenderQueued = "queued"
	// PlacementSenderDeferred could not run yet (no daily headroom, busy,
	// offline) and is retried later.
	PlacementSenderDeferred  = "deferred"
	PlacementSenderRunning   = "running"
	PlacementSenderCompleted = "completed"
	PlacementSenderSkipped   = "skipped"
	PlacementSenderFailed    = "failed"
	PlacementSenderCancelled = "cancelled" //nolint:misspell // wire value: the API sends "cancelled"
)

// What a batch does with a sender that cannot run when its turn comes
// ([PlacementBatchCreateParams.OnUnavailable]).
const (
	PlacementUnavailableSkip = "skip"
	// PlacementUnavailableDefer retries the sender later. The default.
	PlacementUnavailableDefer = "defer"
)

// Sender scopes a batch can resolve on the server
// ([PlacementSenderScope.Type]).
const (
	// PlacementScopeCampaign selects the mailboxes of
	// [PlacementSenderScope.CampaignID].
	PlacementScopeCampaign = "campaign"
	// PlacementScopeWorkspace selects every sending mailbox of the workspace.
	PlacementScopeWorkspace = "workspace"
)

// Sampling modes ([PlacementSample.Mode]).
const (
	// PlacementSampleAll keeps every resolved sender. The default.
	PlacementSampleAll = "all"
	// PlacementSampleRandom keeps Count senders at random.
	PlacementSampleRandom = "random"
	// PlacementSamplePercent keeps Percent (1 to 100) of the senders.
	PlacementSamplePercent = "percent"
	// PlacementSamplePerDomain keeps Count senders per sending domain.
	PlacementSamplePerDomain = "per_domain"
	// PlacementSamplePerProvider keeps Count senders per sending provider.
	PlacementSamplePerProvider = "per_provider"
)

// Sort orders for [PlacementBatchSenderListParams.Sort].
const (
	// PlacementSortWorst lists the lowest inbox rate first. The default.
	PlacementSortWorst = "worst"
	PlacementSortBest  = "best"
	PlacementSortEmail = "email"
	// PlacementSortStatus orders by sender status.
	PlacementSortStatus = "status"
)

// ObservationInterval is a 95 percent Wilson confidence interval.
type ObservationInterval struct {
	Lower float64 `json:"lower"`
	Upper float64 `json:"upper"`
}

// ObservationMetric is an instrument reading with its denominator and
// uncertainty. It describes what a seed panel or the warmup pool observed, not
// the probability that a real recipient's mail reaches the inbox.
type ObservationMetric struct {
	Version         string `json:"version"`
	Population      string `json:"population"`
	DenominatorKind string `json:"denominator_kind"`
	// Unit is "fraction" or "percent" and says how Value and Interval scale.
	Unit                 string `json:"unit"`
	Source               string `json:"source"`
	ClassificationPolicy string `json:"classification_policy"`
	Numerator            int    `json:"numerator"`
	Denominator          int    `json:"denominator"`
	// Unresolved counts the observations that could not be classified.
	Unresolved       int    `json:"unresolved"`
	WindowDays       int    `json:"window_days,omitempty"`
	WindowBasis      string `json:"window_basis"`
	MissingnessBasis string `json:"missingness_basis"`
	// Value is nil when the denominator is zero.
	Value    *float64             `json:"value"`
	Interval *ObservationInterval `json:"wilson_95_independence_interval"`
}

// DKIMVerification is the platform's own DKIM check of a received copy.
type DKIMVerification struct {
	DKIM          string    `json:"dkim"`
	Alignment     string    `json:"alignment"`
	SigningDomain string    `json:"signing_domain,omitempty"`
	Verifier      string    `json:"verifier"`
	ObservedAt    time.Time `json:"observed_at"`
}

// ReceivedEvidence separates what the headers of a received copy claim from
// verified receiver verdicts. Authentication results are strings such as
// "pass", "fail" or "unknown"; "unknown" is common.
type ReceivedEvidence struct {
	DKIMVerification *DKIMVerification `json:"dkim_verification,omitempty"`
	Version          string            `json:"version"`
	Source           string            `json:"source"`
	// Trust says how far to believe the verdict fields, for example
	// "unverified_headers" or "unknown".
	Trust                        string    `json:"trust"`
	ObservedAt                   time.Time `json:"observed_at"`
	SPF                          string    `json:"spf"`
	DKIM                         string    `json:"dkim"`
	DMARC                        string    `json:"dmarc"`
	Alignment                    string    `json:"alignment"`
	TLS                          string    `json:"tls"`
	FromDomain                   string    `json:"from_domain,omitempty"`
	EnvelopeDomainClaim          string    `json:"envelope_domain_claim,omitempty"`
	SigningDomainClaim           string    `json:"signing_domain_claim,omitempty"`
	SelectorClaim                string    `json:"selector_claim,omitempty"`
	AuthenticationResultsPresent bool      `json:"authentication_results_present"`
	OneClickHeadersPresent       bool      `json:"one_click_headers_present"`
	OneClickSigningClaim         bool      `json:"one_click_signing_claim"`
	OneClickCompliance           string    `json:"one_click_compliance"`
}

// PlacementCounts is where a set of probes landed. The rate fields are nil
// until at least one copy has been delivered.
type PlacementCounts struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	Inbox      int `json:"inbox"`
	Promotions int `json:"promotions"`
	Other      int `json:"other"`
	Spam       int `json:"spam"`
	Missing    int `json:"missing"`
	Failed     int `json:"failed"`
	Cancelled  int `json:"cancelled"` //nolint:misspell // wire value: the API sends "cancelled"
	Unknown    int `json:"unknown"`
	// Archived counts copies found in an archive folder (wire name "archive").
	Archived int `json:"archive"`
	Custom   int `json:"custom"`
	// ObservedReceipts and ClassifiedReceipts count the copies seen, and the
	// subset placed in inbox, tab or spam.
	ObservedReceipts   int `json:"observed_receipts"`
	ClassifiedReceipts int `json:"classified_receipts"`
	Unresolved         int `json:"unresolved"`
	// PrimaryMetric is the inbox share and NonSpamMetric the inbox plus tabs
	// share, each over the classified copies with a confidence interval.
	PrimaryMetric *ObservationMetric `json:"primary_metric"`
	NonSpamMetric *ObservationMetric `json:"non_spam_metric"`
	// Delivered is every copy that left and got a verdict: inbox, promotions,
	// other, spam and missing.
	Delivered int `json:"delivered"`
	// InboxRate is the primary inbox share of Delivered, TabsRate the Gmail
	// tabs, SpamRate spam and MissingRate the copies never seen.
	InboxRate   *float64 `json:"inbox_rate"`
	TabsRate    *float64 `json:"tabs_rate"`
	SpamRate    *float64 `json:"spam_rate"`
	MissingRate *float64 `json:"missing_rate"`
}

// PlacementFamilyCounts is one provider family's share of a test.
type PlacementFamilyCounts struct {
	// Family is a mail host such as google_workspace, gmail, microsoft365,
	// outlook or yahoo. New families may appear.
	Family string          `json:"family"`
	Label  string          `json:"label"`
	Counts PlacementCounts `json:"counts"`
}

// PlacementTest is one run: a message sent from one mailbox to a seed panel,
// with where its copies landed overall and per provider family. Lists leave
// the copy (BodyHTML, BodyPlain) out.
type PlacementTest struct {
	ID string `json:"id"`
	// SenderAccountID is nil once the sending mailbox has been deleted.
	SenderAccountID *string `json:"sender_account_id"`
	SenderEmail     string  `json:"sender_email"`
	CreatedBy       *string `json:"created_by"`
	CampaignID      *string `json:"campaign_id"`
	SequenceID      *string `json:"sequence_id"`
	ContactID       *string `json:"contact_id"`
	// MonitorID is set on a test a campaign's placement monitor started.
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
	Origin         string  `json:"origin"`
	Panel          string  `json:"panel"`
	Status         string  `json:"status"`
	Error          string  `json:"error,omitempty"`
	Pace           string  `json:"pace"`
	// CreditsCharged is what the test cost past the monthly free allowance.
	// A paid test is settled once it finishes: CreditsRefunded is what came
	// back because no copy was delivered, CreditsSettledAt when that was
	// decided.
	CreditsCharged   int        `json:"credits_charged"`
	CreditsRefunded  int        `json:"credits_refunded"`
	CreditsSettledAt *time.Time `json:"credits_settled_at"`
	CreatedAt        time.Time  `json:"created_at"`
	FinishedAt       *time.Time `json:"finished_at"`
	// Summary counts where the copies landed.
	Summary PlacementCounts `json:"summary"`
	// Families breaks Summary down by recipient provider family.
	Families []PlacementFamilyCounts `json:"families"`
}

// PlacementResult is one probe: one copy sent to one seed.
type PlacementResult struct {
	// Seed is the seed's address, masked on the shared panels so the panel
	// cannot be listed and whitelisted.
	Seed        string `json:"seed"`
	Family      string `json:"family"`
	FamilyLabel string `json:"family_label"`
	// Folder says where the copy landed, see the PlacementFolder constants.
	Folder      string     `json:"folder"`
	ScheduledAt *time.Time `json:"scheduled_at"`
	SentAt      *time.Time `json:"sent_at"`
	DetectedAt  *time.Time `json:"detected_at"`
	Error       string     `json:"error,omitempty"`
	// FirstFolder is where the copy first appeared, when it later moved.
	FirstFolder *string    `json:"first_folder,omitempty"`
	ObservedAt  *time.Time `json:"observed_at,omitempty"`
	// LateObservation marks a copy counted missing that turned up after the
	// classify window.
	LateObservation bool              `json:"late_observation,omitempty"`
	Evidence        *ReceivedEvidence `json:"evidence"`
}

// SpamSpan is the exact fragment of a copy that triggered a content issue.
type SpamSpan struct {
	// Field is "subject" or "body".
	Field string `json:"field"`
	Text  string `json:"text"`
	// Line is the 1-based line within the field.
	Line    int    `json:"line,omitempty"`
	Excerpt string `json:"excerpt,omitempty"`
}

// ContentIssue is one advisory content problem found in a copy.
type ContentIssue struct {
	// Severity is "warn" or "high".
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	// Field is "subject" or "body" when the issue lives in exactly one of
	// them, empty otherwise.
	Field      string     `json:"field,omitempty"`
	Spans      []SpamSpan `json:"spans,omitempty"`
	Suggestion string     `json:"suggestion,omitempty"`
}

// PlacementContentCheck is the advisory rules pass over a test's copy.
type PlacementContentCheck struct {
	// Score is 0 to 100; higher is safer.
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

// PlacementPanelFamily counts the seeds one provider family contributes.
type PlacementPanelFamily struct {
	Family string `json:"family"`
	Label  string `json:"label"`
	Seeds  int    `json:"seeds"`
}

// PlacementPanel says whether the workspace can test on a seed panel.
type PlacementPanel struct {
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
	// CreditsPerTest is what a test past the free allowance costs in credits,
	// zero when tests cannot be paid for.
	CreditsPerTest int `json:"credits_per_test"`
	// CreditBalance is the workspace's spendable balance, nil when credits are
	// off.
	CreditBalance *int      `json:"credit_balance"`
	PeriodStart   time.Time `json:"period_start"`
	PeriodEnd     time.Time `json:"period_end"`
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

// PlacementSeed is one of a workspace's mailboxes as a seed candidate.
type PlacementSeed struct {
	EmailAccountID string `json:"email_account_id"`
	Email          string `json:"email"`
	Family         string `json:"family"`
	Label          string `json:"label"`
	Status         string `json:"status"`
	// Seed reports whether the mailbox is currently a seed inbox.
	Seed bool `json:"seed"`
	// Blocker is why the mailbox cannot become a seed, empty when it can.
	Blocker string `json:"blocker,omitempty"`
}

// PlacementMonitor re-tests a campaign's first email step on a schedule.
type PlacementMonitor struct {
	ID         string  `json:"id"`
	CampaignID string  `json:"campaign_id"`
	CreatedBy  *string `json:"created_by"`
	Enabled    bool    `json:"enabled"`
	// IntervalDays is the days between tests.
	IntervalDays int    `json:"interval_days"`
	Panel        string `json:"panel"`
	// AlertBelow is the inbox rate, in percent, under which the monitor
	// alerts.
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

// PlacementMonitorParams configures a campaign's placement monitor. It is a
// partial write: a nil field keeps the stored value, or the default on a new
// monitor.
type PlacementMonitorParams struct {
	// Enabled switches the monitor on or off. A new or re-enabled monitor runs
	// its first test within minutes.
	Enabled *bool `json:"enabled,omitempty"`
	// IntervalDays is the days between tests, within the range the server
	// allows.
	IntervalDays *int `json:"interval_days,omitempty"`
	// Panel is one of the PlacementPanel constants.
	Panel *string `json:"panel,omitempty"`
	// AlertBelow is the inbox rate, 0 to 100, under which to alert.
	AlertBelow   *int  `json:"alert_below,omitempty"`
	PauseOnAlert *bool `json:"pause_on_alert,omitempty"`
}

// PlacementTestCreateParams starts a placement test. Give either a campaign
// (and optionally a sequence step) whose copy is tested, or the copy itself.
type PlacementTestCreateParams struct {
	// SenderAccountID is the mailbox the test is sent from. Required. It must
	// be active and must not itself be a seed inbox.
	SenderAccountID string `json:"sender_account_id"`
	// CampaignID tests a campaign's copy.
	CampaignID string `json:"campaign_id,omitempty"`
	// SequenceID names the campaign step to test.
	SequenceID string `json:"sequence_id,omitempty"`
	// ContactID renders the copy for a real contact.
	ContactID string `json:"contact_id,omitempty"`
	Subject   string `json:"subject,omitempty"`
	BodyHTML  string `json:"body_html,omitempty"`
	BodyPlain string `json:"body_plain,omitempty"`
	// Tracking is one of the PlacementTracking constants.
	Tracking string `json:"tracking,omitempty"`
	// Panel is one of the PlacementPanel constants; the instance panel when
	// empty.
	Panel string `json:"panel,omitempty"`
	// SeedIDs restricts a workspace-panel test to some of its seed inboxes.
	SeedIDs []string `json:"seed_ids,omitempty"`
	// Families keeps only seeds hosted by these provider families.
	Families []string `json:"families,omitempty"`
	// Pace is PlacementPaceSpaced (default) or PlacementPaceQuick.
	Pace string `json:"pace,omitempty"`
	// MaxCredits is the most you agree to pay, in credits, for a test past
	// the monthly free allowance.
	MaxCredits int `json:"max_credits,omitempty"`
}

// PlacementTestListParams filters [PlacementService.ListTests].
type PlacementTestListParams struct {
	ListOptions
	// CampaignID keeps only the tests of one campaign.
	CampaignID string
}

func (p *PlacementTestListParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	setNonEmpty(q, "campaign_id", p.CampaignID)
	return q
}

// PlacementSenderScope selects a batch's senders on the server, so a fleet of
// thousands needs no id list. Filters narrow whichever scope is chosen.
type PlacementSenderScope struct {
	// Type is PlacementScopeCampaign or PlacementScopeWorkspace.
	Type string `json:"type"`
	// CampaignID is required for the campaign scope.
	CampaignID string `json:"campaign_id,omitempty"`
	// Providers keeps mailboxes hosted by these provider families.
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
	// Mode is one of the PlacementSample constants.
	Mode    string `json:"mode"`
	Count   int    `json:"count,omitempty"`
	Percent int    `json:"percent,omitempty"`
	// Stratify spreads a random or percent sample across "provider" or
	// "domain" in proportion to each group's size.
	Stratify string `json:"stratify,omitempty"`
}

// PlacementBatchCreateParams starts a placement batch, or previews one with
// [PlacementService.PreviewBatch]. Choose senders with exactly one of
// SenderAccountIDs and SenderScope.
type PlacementBatchCreateParams struct {
	SenderAccountIDs []string              `json:"sender_account_ids,omitempty"`
	SenderScope      *PlacementSenderScope `json:"sender_scope,omitempty"`
	// Sample keeps part of the resolved senders; all of them when nil.
	Sample     *PlacementSample `json:"sample,omitempty"`
	CampaignID string           `json:"campaign_id,omitempty"`
	SequenceID string           `json:"sequence_id,omitempty"`
	ContactID  string           `json:"contact_id,omitempty"`
	Subject    string           `json:"subject,omitempty"`
	BodyHTML   string           `json:"body_html,omitempty"`
	BodyPlain  string           `json:"body_plain,omitempty"`
	// Tracking is one of the PlacementTracking constants.
	Tracking string `json:"tracking,omitempty"`
	// Panel is one of the PlacementPanel constants.
	Panel string `json:"panel,omitempty"`
	// Pace must be empty or PlacementPaceSpaced: a batch always sends spaced.
	Pace     string   `json:"pace,omitempty"`
	Families []string `json:"families,omitempty"`
	SeedIDs  []string `json:"seed_ids,omitempty"`
	// OnUnavailable is PlacementUnavailableDefer (default) or
	// PlacementUnavailableSkip.
	OnUnavailable string `json:"on_unavailable,omitempty"`
	// MaxCredits is the most you agree to pay across the whole batch for
	// tests past the monthly free allowance.
	MaxCredits int `json:"max_credits,omitempty"`
}

// PlacementProviderCount is how many selected senders share a provider.
type PlacementProviderCount struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Senders int    `json:"senders"`
}

// PlacementBatchPreview is what a batch would do, before it is started.
type PlacementBatchPreview struct {
	// Matched is how many senders the scope resolved to, Selected how many
	// the sample kept.
	Matched   int                      `json:"matched"`
	Selected  int                      `json:"selected"`
	Inactive  int                      `json:"inactive"`
	Domains   int                      `json:"domains"`
	Providers []PlacementProviderCount `json:"providers"`
	// Variants is two for a tracking comparison, which doubles every count.
	Variants     int `json:"variants"`
	Tests        int `json:"tests"`
	SeedsPerTest int `json:"seeds_per_test"`
	// MaxSends is the most probes the batch sends; each sender's own daily
	// limit can only make it fewer.
	MaxSends int `json:"max_sends"`
	// FreeTests and PaidTests split Tests against the monthly allowance, and
	// Credits is the most the paid ones cost. Unmetered panels leave all three
	// zero.
	Metered    bool           `json:"metered"`
	FreeTests  int            `json:"free_tests"`
	PaidTests  int            `json:"paid_tests"`
	Credits    int            `json:"credits"`
	Usage      PlacementUsage `json:"usage"`
	SendersMax int            `json:"senders_max"`
	// Concurrency is how many senders send at once in this workspace.
	Concurrency int `json:"concurrency"`
}

// PlacementBatchSelection is how a batch's senders were chosen.
type PlacementBatchSelection struct {
	SenderAccountIDs int                   `json:"sender_account_ids,omitempty"`
	Scope            *PlacementSenderScope `json:"sender_scope,omitempty"`
	Sample           PlacementSample       `json:"sample"`
	// Matched is how many senders the scope resolved to before sampling.
	Matched int `json:"matched"`
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
	Cancelled int `json:"cancelled"` //nolint:misspell // wire value: the API sends "cancelled"
}

// PlacementBatch runs one placement test from many senders, with its progress
// and headline placement.
type PlacementBatch struct {
	ID         string   `json:"id"`
	CreatedBy  *string  `json:"created_by"`
	CampaignID *string  `json:"campaign_id"`
	SequenceID *string  `json:"sequence_id"`
	ContactID  *string  `json:"contact_id"`
	Subject    string   `json:"subject"`
	BodyHTML   string   `json:"body_html,omitempty"`
	BodyPlain  string   `json:"body_plain,omitempty"`
	Tracking   string   `json:"tracking"`
	Panel      string   `json:"panel"`
	Pace       string   `json:"pace"`
	Families   []string `json:"families"`
	SeedIDs    []string `json:"seed_ids"`
	// OnUnavailable is PlacementUnavailableSkip or PlacementUnavailableDefer.
	OnUnavailable string                  `json:"on_unavailable"`
	Selection     PlacementBatchSelection `json:"selection"`
	SenderCount   int                     `json:"sender_count"`
	MaxCredits    int                     `json:"max_credits"`
	CreditsSpent  int                     `json:"credits_spent"`
	Status        string                  `json:"status"`
	Error         string                  `json:"error,omitempty"`
	// RetryUntil is when deferred senders stop being retried.
	RetryUntil time.Time              `json:"retry_until"`
	CreatedAt  time.Time              `json:"created_at"`
	StartedAt  *time.Time             `json:"started_at"`
	FinishedAt *time.Time             `json:"finished_at"`
	Progress   PlacementBatchProgress `json:"progress"`
	// Summary counts where the batch's copies landed (the tracked half of a
	// tracking comparison).
	Summary PlacementCounts `json:"summary"`
}

// PlacementBatchGroup is a batch's placement for one sending domain or
// sending provider.
type PlacementBatchGroup struct {
	Key     string          `json:"key"`
	Label   string          `json:"label"`
	Senders int             `json:"senders"`
	Tested  int             `json:"tested"`
	Counts  PlacementCounts `json:"counts"`
}

// PlacementMatrixRow is one sending domain's placement per recipient
// provider.
type PlacementMatrixRow struct {
	Domain     string                  `json:"domain"`
	Recipients []PlacementFamilyCounts `json:"recipients"`
}

// PlacementBatchDetail is one batch in full: its placement overall and grouped
// by sending domain, sending provider and recipient provider.
type PlacementBatchDetail struct {
	PlacementBatch
	// Untracked is the untracked half of a tracking comparison.
	Untracked  *PlacementCounts        `json:"untracked,omitempty"`
	Domains    []PlacementBatchGroup   `json:"domains"`
	Providers  []PlacementBatchGroup   `json:"providers"`
	Recipients []PlacementFamilyCounts `json:"recipients"`
	Matrix     []PlacementMatrixRow    `json:"matrix"`
	Content    PlacementContentCheck   `json:"content"`
}

// PlacementBatchSender is one sender of a batch with where its copies landed.
type PlacementBatchSender struct {
	ID      string `json:"id"`
	BatchID string `json:"batch_id"`
	// EmailAccountID is nil once the mailbox has been deleted.
	EmailAccountID    *string `json:"email_account_id"`
	SenderEmail       string  `json:"sender_email"`
	SenderDomain      string  `json:"sender_domain"`
	SenderFamily      string  `json:"sender_family"`
	SenderFamilyLabel string  `json:"sender_family_label"`
	// Status is one of the PlacementSender constants.
	Status string `json:"status"`
	// Reason and Detail say why a sender was skipped, deferred or failed.
	Reason        string          `json:"reason,omitempty"`
	Detail        string          `json:"detail,omitempty"`
	Attempts      int             `json:"attempts"`
	NextAttemptAt time.Time       `json:"next_attempt_at"`
	StartedAt     *time.Time      `json:"started_at"`
	FinishedAt    *time.Time      `json:"finished_at"`
	Summary       PlacementCounts `json:"summary"`
	// TestIDs are the tests this sender ran (two for a tracking comparison).
	TestIDs []string `json:"test_ids"`
}

// PlacementBatchSenderListParams filters [PlacementService.ListBatchSenders].
type PlacementBatchSenderListParams struct {
	ListOptions
	// Status keeps senders in one PlacementSender status.
	Status string
	// Search matches the sender address.
	Search string
	// Sort is one of the PlacementSort constants.
	Sort string
}

func (p *PlacementBatchSenderListParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	setNonEmpty(q, "status", p.Status)
	setNonEmpty(q, "q", p.Search)
	setNonEmpty(q, "sort", p.Sort)
	return q
}

// PlacementCoverage is how much of a workspace's connected fleet delivered a
// placement test recently.
type PlacementCoverage struct {
	Mailboxes   int `json:"mailboxes"`
	Tested7d    int `json:"tested_7d"`
	Tested30d   int `json:"tested_30d"`
	NeverTested int `json:"never_tested"`
}

// placementData is the {"data": ...} wrapper the placement endpoints use for
// a single object.
type placementData[T any] struct {
	Data T `json:"data"`
}

func placementFetch[T any](ctx context.Context, c *Client, path string, opts []RequestOption) (*T, *Response, error) {
	var env placementData[*T]
	resp, err := c.get(ctx, path, &env, opts...)
	if err != nil {
		return nil, resp, err
	}
	return env.Data, resp, nil
}

func placementSend[T any](ctx context.Context, verb bodyVerb, path string, body any, opts []RequestOption) (*T, *Response, error) {
	var env placementData[*T]
	resp, err := verb(ctx, path, body, &env, opts...)
	if err != nil {
		return nil, resp, err
	}
	return env.Data, resp, nil
}

// Overview lists the seed panels the workspace can test on, with their
// provider mix, and the monthly allowance. Needs [PermReadAnalytics].
func (s *PlacementService) Overview(ctx context.Context, opts ...RequestOption) (*PlacementOverview, *Response, error) {
	return placementFetch[PlacementOverview](ctx, s.client, "placement/overview", opts)
}

// ListTests returns a page of the workspace's placement tests, newest first.
// Limit is capped at 100. Needs [PermReadAnalytics].
func (s *PlacementService) ListTests(ctx context.Context, params *PlacementTestListParams, opts ...RequestOption) (*Page[PlacementTest], error) {
	return listJSON[PlacementTest](ctx, s.client, "placement/tests", params.values(), opts...)
}

// GetTest returns one test with every copy, the content check and, for a
// tracking comparison, the other half. Needs [PermReadAnalytics].
func (s *PlacementService) GetTest(ctx context.Context, id string, opts ...RequestOption) (*PlacementTestDetail, *Response, error) {
	return placementFetch[PlacementTestDetail](ctx, s.client, "placement/tests/"+url.PathEscape(id), opts)
}

// CreateTest starts a placement test from one mailbox. It returns one test, or
// two for a tracking comparison ([PlacementTrackingCompare]). Past the monthly
// free allowance a test costs credits, up to PlacementTestCreateParams.MaxCredits.
// Needs [PermSendCampaigns].
func (s *PlacementService) CreateTest(ctx context.Context, params *PlacementTestCreateParams, opts ...RequestOption) ([]PlacementTest, *Response, error) {
	return sendData[PlacementTest](ctx, s.client.post, "placement/tests", params, opts)
}

// CancelTest stops the copies of a test that have not been sent yet. Needs
// [PermSendCampaigns].
func (s *PlacementService) CancelTest(ctx context.Context, id string, opts ...RequestOption) (*PlacementTest, *Response, error) {
	return placementSend[PlacementTest](ctx, s.client.post, "placement/tests/"+url.PathEscape(id)+"/cancel", nil, opts)
}

// ListBatches returns a page of the workspace's placement batches, newest
// first. Limit is capped at 100. Needs [PermReadAnalytics].
func (s *PlacementService) ListBatches(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[PlacementBatch], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[PlacementBatch](ctx, s.client, "placement/batches", q, opts...)
}

// GetBatch returns one batch with its placement overall and grouped by
// sending domain, sending provider and recipient provider. Needs
// [PermReadAnalytics].
func (s *PlacementService) GetBatch(ctx context.Context, id string, opts ...RequestOption) (*PlacementBatchDetail, *Response, error) {
	return placementFetch[PlacementBatchDetail](ctx, s.client, "placement/batches/"+url.PathEscape(id), opts)
}

// ListBatchSenders returns a page of a batch's senders with where each one's
// copies landed, worst inbox rate first unless sorted otherwise. Limit is
// capped at 100. Needs [PermReadAnalytics].
func (s *PlacementService) ListBatchSenders(ctx context.Context, id string, params *PlacementBatchSenderListParams, opts ...RequestOption) (*Page[PlacementBatchSender], error) {
	return listJSON[PlacementBatchSender](ctx, s.client, "placement/batches/"+url.PathEscape(id)+"/senders", params.values(), opts...)
}

// PreviewBatch reports how many senders, tests, sends and credits a batch
// would come to. It starts nothing. Needs [PermSendCampaigns].
func (s *PlacementService) PreviewBatch(ctx context.Context, params *PlacementBatchCreateParams, opts ...RequestOption) (*PlacementBatchPreview, *Response, error) {
	return placementSend[PlacementBatchPreview](ctx, s.client.post, "placement/batches/preview", params, opts)
}

// CreateBatch snapshots the senders and queues the batch; the service starts
// its senders a few at a time. Needs [PermSendCampaigns].
func (s *PlacementService) CreateBatch(ctx context.Context, params *PlacementBatchCreateParams, opts ...RequestOption) (*PlacementBatch, *Response, error) {
	return placementSend[PlacementBatch](ctx, s.client.post, "placement/batches", params, opts)
}

// CancelBatch stops a batch. Copies already sent keep being classified. Needs
// [PermSendCampaigns].
func (s *PlacementService) CancelBatch(ctx context.Context, id string, opts ...RequestOption) (*PlacementBatch, *Response, error) {
	return placementSend[PlacementBatch](ctx, s.client.post, "placement/batches/"+url.PathEscape(id)+"/cancel", nil, opts)
}

// Coverage reports how much of the workspace's connected fleet delivered a
// placement test in the last 7 and 30 days. Needs [PermReadAnalytics].
func (s *PlacementService) Coverage(ctx context.Context, opts ...RequestOption) (*PlacementCoverage, *Response, error) {
	return placementFetch[PlacementCoverage](ctx, s.client, "placement/coverage", opts)
}

// ListSeeds lists the workspace's mailboxes and which of them are its seed
// inboxes. Needs [PermReadEmails].
func (s *PlacementService) ListSeeds(ctx context.Context, opts ...RequestOption) ([]PlacementSeed, *Response, error) {
	return fetchData[PlacementSeed](ctx, s.client, "placement/seeds", opts)
}

// SetSeed makes a workspace mailbox a seed inbox, or an ordinary mailbox
// again. A seed stops warming and stops sending campaigns. A mailbox on the
// instance panel cannot be changed (409). Needs [PermWriteEmails].
func (s *PlacementService) SetSeed(ctx context.Context, emailAccountID string, seed bool, opts ...RequestOption) (*PlacementSeed, *Response, error) {
	body := struct {
		Seed bool `json:"seed"`
	}{seed}
	return placementSend[PlacementSeed](ctx, s.client.put, "placement/seeds/"+url.PathEscape(emailAccountID), body, opts)
}

// PlacementMonitor returns a campaign's scheduled placement test, or nil when
// it has none. Needs [PermReadCampaigns].
func (s *CampaignService) PlacementMonitor(ctx context.Context, id string, opts ...RequestOption) (*PlacementMonitor, *Response, error) {
	return placementFetch[PlacementMonitor](ctx, s.client, "campaigns/"+url.PathEscape(id)+"/placement-monitor", opts)
}

// SetPlacementMonitor creates or updates a campaign's scheduled placement
// test. Fields left nil keep their stored value (or the default on a new
// monitor), and repeating the same call lands on the same state. Needs
// [PermSendCampaigns].
func (s *CampaignService) SetPlacementMonitor(ctx context.Context, id string, params *PlacementMonitorParams, opts ...RequestOption) (*PlacementMonitor, *Response, error) {
	if params == nil {
		params = &PlacementMonitorParams{}
	}
	return placementSend[PlacementMonitor](ctx, s.client.put, "campaigns/"+url.PathEscape(id)+"/placement-monitor", params, opts)
}

// DeletePlacementMonitor removes a campaign's scheduled placement test. It
// fails with a 404 when the campaign has none. Needs [PermSendCampaigns].
func (s *CampaignService) DeletePlacementMonitor(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "campaigns/"+url.PathEscape(id)+"/placement-monitor", opts...)
}
