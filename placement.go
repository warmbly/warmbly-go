package warmbly

import (
	"context"
	"net/url"
	"time"
)

// PlacementService runs inbox placement tests: a template or campaign step
// sent from one of the workspace's mailboxes to a panel of seed inboxes, with
// each copy then classified by where it landed (primary inbox, a Gmail tab,
// spam, or never seen). A placement batch runs the same test from many
// senders at once and aggregates the result by sending domain, sending
// provider and recipient provider.
//
// Tests on the metered panels count against a monthly allowance; past it a
// test costs credits, and the caller must agree to the price up front with
// MaxCredits on the create parameters. A test that delivers nothing is
// refunded automatically.
//
// A campaign's scheduled placement test (its monitor) is managed through
// [CampaignService.PlacementMonitor] and its siblings.
type PlacementService service

// Seed panels a placement test can send to.
const (
	// PlacementPanelInstance is the seed panel the operator runs for every
	// workspace on the instance (on the hosted product, Warmbly's own).
	PlacementPanelInstance = "instance"
	// PlacementPanelWorkspace is the workspace's own seed inboxes; see
	// [PlacementService.SetSeed].
	PlacementPanelWorkspace = "workspace"
	// PlacementPanelCloud is Warmbly Cloud's panel, for a self-hosted
	// instance linked to it.
	PlacementPanelCloud = "cloud"
)

// Placement test statuses returned in [PlacementTest.Status].
const (
	PlacementStatusRunning   = "running"
	PlacementStatusCompleted = "completed"
	PlacementStatusCancelled = "cancelled" //nolint:misspell // wire value: the API sends "cancelled" here
	PlacementStatusFailed    = "failed"
)

// What started a placement test, returned in [PlacementTest.Origin].
const (
	PlacementOriginManual  = "manual"
	PlacementOriginMonitor = "monitor"
	PlacementOriginAdmin   = "admin"
	// PlacementOriginRemote is a test Warmbly Cloud runs for a linked
	// instance.
	PlacementOriginRemote = "remote"
	// PlacementOriginBatch is a test a placement batch started for one of its
	// senders.
	PlacementOriginBatch = "batch"
)

// Where one copy landed, returned in [PlacementResult.Folder]. New values
// may appear; treat anything unrecognized as unresolved.
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
	PlacementFolderCancelled = "cancelled" //nolint:misspell // wire value: the API sends "cancelled" here
	PlacementFolderUnknown   = "unknown"
	PlacementFolderArchive   = "archive"
	PlacementFolderCustom    = "custom"
)

// Tracking choices for a placement test or batch.
const (
	// PlacementTrackingCampaign copies the campaign's own open and click
	// tracking; an ad-hoc test has none. It is the default.
	PlacementTrackingCampaign = "campaign"
	PlacementTrackingOn       = "on"
	PlacementTrackingOff      = "off"
	// PlacementTrackingCompare runs the same copy twice, once tracked and
	// once not, interleaved per seed so both see the same conditions. It
	// creates two tests (or doubles a batch) linked by CompareGroupID.
	PlacementTrackingCompare = "compare"
)

// Pacing of a test's copies from one sender.
const (
	// PlacementPaceSpaced sends at the instance's spacing, about a minute
	// apart. It is the default, and the only pace a batch accepts.
	PlacementPaceSpaced = "spaced"
	// PlacementPaceQuick sends a few seconds apart, for a result in minutes.
	PlacementPaceQuick = "quick"
)

// Placement batch statuses returned in [PlacementBatch.Status].
const (
	PlacementBatchQueued                = "queued"
	PlacementBatchRunning               = "running"
	PlacementBatchCompleted             = "completed"
	PlacementBatchCompletedWithWarnings = "completed_with_warnings"
	PlacementBatchCancelled             = "cancelled" //nolint:misspell // wire value: the API sends "cancelled" here
	PlacementBatchFailed                = "failed"
)

// Statuses of one sender in a batch, returned in [PlacementBatchSender.Status]
// and accepted by [PlacementBatchSenderListParams.Status].
const (
	PlacementSenderQueued = "queued"
	// PlacementSenderDeferred could not run yet (no daily headroom, busy,
	// offline) and is retried later.
	PlacementSenderDeferred  = "deferred"
	PlacementSenderRunning   = "running"
	PlacementSenderCompleted = "completed"
	PlacementSenderSkipped   = "skipped"
	PlacementSenderFailed    = "failed"
	PlacementSenderCancelled = "cancelled" //nolint:misspell // wire value: the API sends "cancelled" here
)

// What a batch does with a sender that cannot run when its turn comes.
const (
	PlacementUnavailableSkip = "skip"
	// PlacementUnavailableDefer retries the sender later. It is the default.
	PlacementUnavailableDefer = "defer"
)

// Sender scopes a batch resolves on the server, for [PlacementSenderScope.Type].
const (
	PlacementScopeCampaign  = "campaign"
	PlacementScopeWorkspace = "workspace"
)

// Sampling modes for [PlacementSample.Mode].
const (
	PlacementSampleAll         = "all"
	PlacementSampleRandom      = "random"
	PlacementSamplePercent     = "percent"
	PlacementSamplePerDomain   = "per_domain"
	PlacementSamplePerProvider = "per_provider"
)

// Sort orders for [PlacementBatchSenderListParams.Sort].
const (
	// PlacementSenderSortWorst lists the lowest inbox rate first. It is the
	// default.
	PlacementSenderSortWorst  = "worst"
	PlacementSenderSortBest   = "best"
	PlacementSenderSortEmail  = "email"
	PlacementSenderSortStatus = "status"
)

// PlacementTest is one run: a template sent from one sender to a seed panel,
// with where its copies landed overall and per host family. Lists leave the
// copy itself (BodyHTML, BodyPlain) out.
type PlacementTest struct {
	ID              string  `json:"id"`
	SenderAccountID *string `json:"sender_account_id"`
	SenderEmail     string  `json:"sender_email"`
	// CreatedBy is nil for a test started by an API key or a monitor.
	CreatedBy  *string `json:"created_by"`
	CampaignID *string `json:"campaign_id"`
	SequenceID *string `json:"sequence_id"`
	ContactID  *string `json:"contact_id"`
	MonitorID  *string `json:"monitor_id"`
	// BatchID names the placement batch that started the test.
	BatchID      *string `json:"batch_id"`
	Subject      string  `json:"subject"`
	BodyHTML     string  `json:"body_html,omitempty"`
	BodyPlain    string  `json:"body_plain,omitempty"`
	OpenTracking bool    `json:"open_tracking"`
	LinkTracking bool    `json:"link_tracking"`
	// CompareGroupID links the two halves of a tracking comparison.
	CompareGroupID *string `json:"compare_group_id"`
	// Origin is one of the PlacementOrigin* constants.
	Origin string `json:"origin"`
	// Panel is one of the PlacementPanel* constants.
	Panel string `json:"panel"`
	// Status is one of the PlacementStatus* constants.
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	// Pace is one of the PlacementPace* constants.
	Pace string `json:"pace"`
	// CreditsCharged is what the test cost past the monthly free allowance.
	// A paid test is settled once it finishes: CreditsRefunded is what came
	// back because no copy was delivered, CreditsSettledAt when that was
	// decided.
	CreditsCharged   int        `json:"credits_charged"`
	CreditsRefunded  int        `json:"credits_refunded"`
	CreditsSettledAt *time.Time `json:"credits_settled_at"`
	CreatedAt        time.Time  `json:"created_at"`
	FinishedAt       *time.Time `json:"finished_at"`

	Summary  PlacementCounts         `json:"summary"`
	Families []PlacementFamilyCounts `json:"families"`
}

// PlacementTestDetail is one test in full: every copy, the content check and,
// for a tracking comparison, the other half.
type PlacementTestDetail struct {
	PlacementTest
	Results []PlacementResult     `json:"results"`
	Content PlacementContentCheck `json:"content"`
	// Compare is the other half of a tracking comparison, nil otherwise.
	Compare *PlacementTest `json:"compare,omitempty"`
}

// PlacementResult is one copy sent to one seed.
type PlacementResult struct {
	// Seed is the seed's address, masked on the shared panels so the panel
	// cannot be listed and allowlisted.
	Seed string `json:"seed"`
	// Family is who hosts the seed, a mail host value such as
	// google_workspace, gmail, microsoft365, outlook or yahoo.
	Family      string `json:"family"`
	FamilyLabel string `json:"family_label"`
	// Folder is one of the PlacementFolder* constants.
	Folder      string     `json:"folder"`
	ScheduledAt *time.Time `json:"scheduled_at"`
	SentAt      *time.Time `json:"sent_at"`
	DetectedAt  *time.Time `json:"detected_at"`
	Error       string     `json:"error,omitempty"`
	// FirstFolder is the folder the copy was first observed in, and
	// ObservedAt when.
	FirstFolder *string    `json:"first_folder,omitempty"`
	ObservedAt  *time.Time `json:"observed_at,omitempty"`
	// LateObservation is true when the copy was seen after the classify
	// window had already closed.
	LateObservation bool `json:"late_observation,omitempty"`
	// Evidence is the authentication read from the received copy, nil when
	// the copy was not observed.
	Evidence *ReceivedEvidence `json:"evidence"`
}

// ReceivedEvidence is what the receiving mailbox recorded about a message's
// authentication (SPF, DKIM, DMARC, alignment, TLS and one-click
// unsubscribe). Values are verdict words such as "pass", "fail" or
// "unknown".
type ReceivedEvidence struct {
	DKIMVerification *ReceivedDKIMVerification `json:"dkim_verification,omitempty"`
	Version          string                    `json:"version"`
	Source           string                    `json:"source"`
	Trust            string                    `json:"trust"`
	ObservedAt       time.Time                 `json:"observed_at"`
	SPF              string                    `json:"spf"`
	DKIM             string                    `json:"dkim"`
	DMARC            string                    `json:"dmarc"`
	Alignment        string                    `json:"alignment"`
	TLS              string                    `json:"tls"`
	FromDomain       string                    `json:"from_domain,omitempty"`
	// EnvelopeDomain, SigningDomain and Selector are what the headers claim,
	// not independently verified.
	EnvelopeDomain               string `json:"envelope_domain_claim,omitempty"`
	SigningDomain                string `json:"signing_domain_claim,omitempty"`
	Selector                     string `json:"selector_claim,omitempty"`
	AuthenticationResultsPresent bool   `json:"authentication_results_present"`
	OneClickHeaders              bool   `json:"one_click_headers_present"`
	OneClickSigningClaim         bool   `json:"one_click_signing_claim"`
	OneClickCompliance           string `json:"one_click_compliance"`
}

// ReceivedDKIMVerification is Warmbly's own DKIM check of a received message.
type ReceivedDKIMVerification struct {
	DKIM          string    `json:"dkim"`
	Alignment     string    `json:"alignment"`
	SigningDomain string    `json:"signing_domain,omitempty"`
	Verifier      string    `json:"verifier"`
	ObservedAt    time.Time `json:"observed_at"`
}

// PlacementContentCheck is the spam-risk rules pass over a test's copy, the
// same one [TemplateService.Score] runs.
type PlacementContentCheck struct {
	Score  int             `json:"score"`
	Issues []TemplateIssue `json:"issues"`
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
	Cancelled  int `json:"cancelled"` //nolint:misspell // wire value: the API sends "cancelled" here
	Unknown    int `json:"unknown"`
	Archived   int `json:"archive"`
	Custom     int `json:"custom"`
	// Observed is every copy seen in a seed; Classified the ones whose folder
	// settled into inbox, a tab or spam. Unresolved is missing plus pending.
	Observed   int `json:"observed_receipts"`
	Classified int `json:"classified_receipts"`
	Unresolved int `json:"unresolved"`
	// PrimaryMetric is the primary inbox share of classified copies and
	// NonSpamMetric the share that stayed out of spam, each with its
	// confidence interval.
	PrimaryMetric *ObservationMetric `json:"primary_metric"`
	NonSpamMetric *ObservationMetric `json:"non_spam_metric"`
	// Delivered is every copy that left and got a verdict:
	// inbox + promotions + other + spam + missing.
	Delivered int `json:"delivered"`
	// InboxRate is the primary inbox share of Delivered, TabsRate the Gmail
	// tabs, SpamRate spam and MissingRate the copies never seen, each as a
	// fraction from 0 to 1. All four are nil until something is delivered.
	InboxRate   *float64 `json:"inbox_rate"`
	TabsRate    *float64 `json:"tabs_rate"`
	SpamRate    *float64 `json:"spam_rate"`
	MissingRate *float64 `json:"missing_rate"`
}

// ObservationMetric is a rate measured over observed mailbox receipts, with
// the population and denominator it was taken over and a Wilson 95% interval.
// Value is nil when the denominator is zero.
type ObservationMetric struct {
	Version          string `json:"version"`
	Population       string `json:"population"`
	DenominatorKind  string `json:"denominator_kind"`
	Unit             string `json:"unit"`
	Source           string `json:"source"`
	PolicyVersion    string `json:"classification_policy"`
	Numerator        int    `json:"numerator"`
	Denominator      int    `json:"denominator"`
	Unresolved       int    `json:"unresolved"`
	WindowDays       int    `json:"window_days,omitempty"`
	WindowBasis      string `json:"window_basis"`
	MissingnessBasis string `json:"missingness_basis"`
	// Unit says whether Value is a "fraction" (0 to 1) or a "percent".
	Value    *float64             `json:"value"`
	Interval *ObservationInterval `json:"wilson_95_independence_interval"`
}

// ObservationInterval is the lower and upper bound of a confidence interval,
// in the metric's own unit.
type ObservationInterval struct {
	Lower float64 `json:"lower"`
	Upper float64 `json:"upper"`
}

// PlacementFamilyCounts is one host family's share of a test or batch.
type PlacementFamilyCounts struct {
	Family string          `json:"family"`
	Label  string          `json:"label"`
	Counts PlacementCounts `json:"counts"`
}

// PlacementOverview is what a workspace can test on and how much of its
// allowance is left.
type PlacementOverview struct {
	Panels []PlacementPanel `json:"panels"`
	Usage  PlacementUsage   `json:"usage"`
	// WorkspaceSeeds is how many of the workspace's own mailboxes are seeds.
	WorkspaceSeeds int `json:"workspace_seeds"`
	SeedsPerTest   int `json:"seeds_per_test"`
	// SpacingSeconds is the gap between two copies of a spaced test.
	SpacingSeconds int `json:"spacing_seconds"`
}

// PlacementPanel says whether a workspace can test on a seed panel.
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

// PlacementPanelFamily counts the seeds one host family contributes to a
// panel.
type PlacementPanelFamily struct {
	Family string `json:"family"`
	Label  string `json:"label"`
	Seeds  int    `json:"seeds"`
}

// PlacementUsage is a workspace's monthly allowance on the metered panels.
type PlacementUsage struct {
	Used int `json:"used"`
	// Limit is nil when the instance does not meter tests (self-hosted).
	Limit *int `json:"limit"`
	// CreditsPerTest is what a test past the free allowance costs in
	// credits, zero when tests cannot be paid for. CreditBalance is the
	// workspace's spendable balance, nil when credits are off.
	CreditsPerTest int       `json:"credits_per_test"`
	CreditBalance  *int      `json:"credit_balance"`
	PeriodStart    time.Time `json:"period_start"`
	PeriodEnd      time.Time `json:"period_end"`
}

// PlacementSeed is one of the workspace's own mailboxes as a seed inbox
// candidate.
type PlacementSeed struct {
	EmailAccountID string `json:"email_account_id"`
	Email          string `json:"email"`
	Family         string `json:"family"`
	Label          string `json:"label"`
	Status         string `json:"status"`
	// Seed reports whether the mailbox is a seed inbox now.
	Seed bool `json:"seed"`
	// Blocker is why the mailbox cannot become a seed, empty when it can.
	Blocker string `json:"blocker,omitempty"`
}

// PlacementTestCreateParams starts a placement test. Name a campaign and
// sequence step to test exactly what the campaign sends (rendered for
// ContactID when given), or pass Subject and a body for ad-hoc copy.
type PlacementTestCreateParams struct {
	// SenderAccountID is the mailbox the copies are sent from. Required.
	SenderAccountID string `json:"sender_account_id"`
	CampaignID      string `json:"campaign_id,omitempty"`
	SequenceID      string `json:"sequence_id,omitempty"`
	ContactID       string `json:"contact_id,omitempty"`
	Subject         string `json:"subject,omitempty"`
	BodyHTML        string `json:"body_html,omitempty"`
	BodyPlain       string `json:"body_plain,omitempty"`
	// Tracking is one of the PlacementTracking* constants; empty is
	// [PlacementTrackingCampaign].
	Tracking string `json:"tracking,omitempty"`
	// Panel is one of the PlacementPanel* constants; empty is
	// [PlacementPanelInstance].
	Panel string `json:"panel,omitempty"`
	// SeedIDs narrows a [PlacementPanelWorkspace] test to these seed
	// mailboxes (their email account IDs).
	SeedIDs []string `json:"seed_ids,omitempty"`
	// Families keeps only seeds hosted by these mail host families.
	Families []string `json:"families,omitempty"`
	// Pace is one of the PlacementPace* constants; empty is
	// [PlacementPaceSpaced].
	Pace string `json:"pace,omitempty"`
	// MaxCredits is the most the caller agrees to pay for a test past the
	// monthly free allowance. A higher price is refused, never charged.
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

// PlacementBatch is a placement test run from many senders, with its
// progress and headline placement. Summary covers the copy the campaign
// really sends: the tracked half of a tracking comparison.
type PlacementBatch struct {
	ID         string  `json:"id"`
	CreatedBy  *string `json:"created_by"`
	CampaignID *string `json:"campaign_id"`
	SequenceID *string `json:"sequence_id"`
	ContactID  *string `json:"contact_id"`
	Subject    string  `json:"subject"`
	BodyHTML   string  `json:"body_html,omitempty"`
	BodyPlain  string  `json:"body_plain,omitempty"`
	// Tracking, Panel and Pace use the PlacementTracking*, PlacementPanel*
	// and PlacementPace* constants.
	Tracking string   `json:"tracking"`
	Panel    string   `json:"panel"`
	Pace     string   `json:"pace"`
	Families []string `json:"families"`
	SeedIDs  []string `json:"seed_ids"`
	// OnUnavailable is one of the PlacementUnavailable* constants.
	OnUnavailable string                  `json:"on_unavailable"`
	Selection     PlacementBatchSelection `json:"selection"`
	SenderCount   int                     `json:"sender_count"`
	MaxCredits    int                     `json:"max_credits"`
	CreditsSpent  int                     `json:"credits_spent"`
	// Status is one of the PlacementBatch* constants.
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	// RetryUntil is how long deferred senders keep being retried.
	RetryUntil time.Time  `json:"retry_until"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`

	Progress PlacementBatchProgress `json:"progress"`
	Summary  PlacementCounts        `json:"summary"`
}

// PlacementBatchDetail is one batch in full, grouped by sending domain,
// sending provider and recipient provider.
type PlacementBatchDetail struct {
	PlacementBatch
	// Untracked is the untracked half of a tracking comparison, nil
	// otherwise.
	Untracked  *PlacementCounts          `json:"untracked,omitempty"`
	Domains    []PlacementBatchGroup     `json:"domains"`
	Providers  []PlacementBatchGroup     `json:"providers"`
	Recipients []PlacementFamilyCounts   `json:"recipients"`
	Matrix     []PlacementBatchMatrixRow `json:"matrix"`
	Content    PlacementContentCheck     `json:"content"`
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

// PlacementBatchMatrixRow is one sending domain's placement per recipient
// provider.
type PlacementBatchMatrixRow struct {
	Domain     string                  `json:"domain"`
	Recipients []PlacementFamilyCounts `json:"recipients"`
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
	Cancelled int `json:"cancelled"` //nolint:misspell // wire value: the API sends "cancelled" here
}

// PlacementBatchSelection is how a batch's senders were chosen.
type PlacementBatchSelection struct {
	// SenderAccountIDs is how many senders were named explicitly, zero when a
	// scope chose them.
	SenderAccountIDs int                   `json:"sender_account_ids,omitempty"`
	Scope            *PlacementSenderScope `json:"sender_scope,omitempty"`
	Sample           PlacementSample       `json:"sample"`
	// Matched is how many senders the scope resolved to before sampling.
	Matched int `json:"matched"`
}

// PlacementSenderScope selects a batch's senders on the server, so a fleet of
// thousands needs no ID list. The filters narrow whichever scope is chosen.
type PlacementSenderScope struct {
	// Type is [PlacementScopeCampaign] or [PlacementScopeWorkspace].
	Type string `json:"type"`
	// CampaignID names the campaign of a [PlacementScopeCampaign] scope.
	CampaignID string `json:"campaign_id,omitempty"`
	// Providers keeps mailboxes hosted by these mail host families.
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

// PlacementSample picks part of a batch's resolved senders.
type PlacementSample struct {
	// Mode is one of the PlacementSample* constants; empty is
	// [PlacementSampleAll].
	Mode string `json:"mode"`
	// Count is the sample size for random, per_domain and per_provider.
	Count int `json:"count,omitempty"`
	// Percent is the sample size for percent, 1 to 100.
	Percent int `json:"percent,omitempty"`
	// Stratify spreads a random or percent sample across "provider" or
	// "domain" in proportion to each group's size.
	Stratify string `json:"stratify,omitempty"`
}

// PlacementBatchParams describes a placement batch for
// [PlacementService.PreviewBatch] and [PlacementService.CreateBatch]. Set
// exactly one of SenderAccountIDs and SenderScope.
type PlacementBatchParams struct {
	SenderAccountIDs []string              `json:"sender_account_ids,omitempty"`
	SenderScope      *PlacementSenderScope `json:"sender_scope,omitempty"`
	// Sample keeps part of the resolved senders; nil keeps all of them.
	Sample     *PlacementSample `json:"sample,omitempty"`
	CampaignID string           `json:"campaign_id,omitempty"`
	SequenceID string           `json:"sequence_id,omitempty"`
	ContactID  string           `json:"contact_id,omitempty"`
	Subject    string           `json:"subject,omitempty"`
	BodyHTML   string           `json:"body_html,omitempty"`
	BodyPlain  string           `json:"body_plain,omitempty"`
	// Tracking is one of the PlacementTracking* constants; empty is
	// [PlacementTrackingCampaign].
	Tracking string `json:"tracking,omitempty"`
	// Panel is one of the PlacementPanel* constants; empty is
	// [PlacementPanelInstance].
	Panel string `json:"panel,omitempty"`
	// Pace may only be [PlacementPaceSpaced] (or empty) on a batch.
	Pace     string   `json:"pace,omitempty"`
	Families []string `json:"families,omitempty"`
	// SeedIDs narrows a [PlacementPanelWorkspace] batch to these seeds.
	SeedIDs []string `json:"seed_ids,omitempty"`
	// OnUnavailable is one of the PlacementUnavailable* constants; empty is
	// [PlacementUnavailableDefer].
	OnUnavailable string `json:"on_unavailable,omitempty"`
	// MaxCredits is the most the caller agrees to pay across the whole batch
	// for tests past the monthly free allowance.
	MaxCredits int `json:"max_credits,omitempty"`
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
	// FreeTests and PaidTests split Tests against the monthly allowance, and
	// Credits is the most the paid ones cost. Unmetered panels leave all
	// three zero.
	Metered    bool           `json:"metered"`
	FreeTests  int            `json:"free_tests"`
	PaidTests  int            `json:"paid_tests"`
	Credits    int            `json:"credits"`
	Usage      PlacementUsage `json:"usage"`
	SendersMax int            `json:"senders_max"`
	// Concurrency is how many senders send at once in this workspace.
	Concurrency int `json:"concurrency"`
}

// PlacementBatchGroupCount is how many selected senders share a provider.
type PlacementBatchGroupCount struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Senders int    `json:"senders"`
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
	// Status is one of the PlacementSender* constants.
	Status string `json:"status"`
	// Reason and Detail explain a deferred, skipped or failed sender.
	Reason        string          `json:"reason,omitempty"`
	Detail        string          `json:"detail,omitempty"`
	Attempts      int             `json:"attempts"`
	NextAttemptAt time.Time       `json:"next_attempt_at"`
	StartedAt     *time.Time      `json:"started_at"`
	FinishedAt    *time.Time      `json:"finished_at"`
	Summary       PlacementCounts `json:"summary"`
	// TestIDs are the placement tests this sender ran (two for a tracking
	// comparison).
	TestIDs []string `json:"test_ids"`
}

// PlacementBatchSenderListParams filters [PlacementService.ListBatchSenders].
type PlacementBatchSenderListParams struct {
	ListOptions
	// Status is one of the PlacementSender* constants.
	Status string
	// Query is a substring search over the sender address.
	Query string
	// Sort is one of the PlacementSenderSort* constants.
	Sort string
}

func (p *PlacementBatchSenderListParams) values() url.Values {
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

// PlacementCoverage is how much of the workspace's connected fleet delivered
// a placement test recently.
type PlacementCoverage struct {
	Mailboxes   int `json:"mailboxes"`
	Tested7d    int `json:"tested_7d"`
	Tested30d   int `json:"tested_30d"`
	NeverTested int `json:"never_tested"`
}

// PlacementMonitor re-tests a campaign's first email step on a schedule,
// from the campaign's mailboxes in turn, and alerts (or pauses the campaign)
// when the inbox rate drops below AlertBelow.
type PlacementMonitor struct {
	ID         string  `json:"id"`
	CampaignID string  `json:"campaign_id"`
	CreatedBy  *string `json:"created_by"`
	Enabled    bool    `json:"enabled"`
	// IntervalDays is how often the test runs, 1 to 30.
	IntervalDays int `json:"interval_days"`
	// Panel is one of the PlacementPanel* constants.
	Panel string `json:"panel"`
	// AlertBelow is the inbox rate, in percent, under which a run alerts.
	AlertBelow   int        `json:"alert_below"`
	PauseOnAlert bool       `json:"pause_on_alert"`
	NextRunAt    time.Time  `json:"next_run_at"`
	LastRunAt    *time.Time `json:"last_run_at"`
	LastTestID   *string    `json:"last_test_id"`
	LastAlertAt  *time.Time `json:"last_alert_at"`
	LastError    string     `json:"last_error,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// PlacementMonitorParams configures a campaign's placement monitor. Nil
// fields keep the stored value, or the default on a new monitor (enabled,
// every 7 days, the instance panel, alert below 70%, no auto-pause).
type PlacementMonitorParams struct {
	Enabled *bool `json:"enabled,omitempty"`
	// IntervalDays must be 1 to 30.
	IntervalDays *int `json:"interval_days,omitempty"`
	// Panel is one of the PlacementPanel* constants.
	Panel *string `json:"panel,omitempty"`
	// AlertBelow is an inbox rate in percent, 0 to 100.
	AlertBelow   *int  `json:"alert_below,omitempty"`
	PauseOnAlert *bool `json:"pause_on_alert,omitempty"`
}

// placementEnvelope is the {"data": ...} wrapper every placement endpoint
// answers with.
type placementEnvelope[T any] struct {
	Data *T `json:"data"`
}

// getPlacementData GETs path and unwraps its {"data": {...}} envelope. A
// null data decodes as a nil T.
func getPlacementData[T any](ctx context.Context, c *Client, path string, opts []RequestOption) (*T, *Response, error) {
	var env placementEnvelope[T]
	resp, err := c.get(ctx, path, &env, opts...)
	if err != nil {
		return nil, resp, err
	}
	return env.Data, resp, nil
}

// sendPlacementData is [getPlacementData] for a request that carries a body.
func sendPlacementData[T any](ctx context.Context, verb bodyVerb, path string, body any, opts []RequestOption) (*T, *Response, error) {
	var env placementEnvelope[T]
	resp, err := verb(ctx, path, body, &env, opts...)
	if err != nil {
		return nil, resp, err
	}
	if env.Data == nil {
		env.Data = new(T)
	}
	return env.Data, resp, nil
}

// Overview returns the seed panels the workspace can test on and its monthly
// allowance. Requires [PermReadAnalytics].
func (s *PlacementService) Overview(ctx context.Context, opts ...RequestOption) (*PlacementOverview, *Response, error) {
	out, resp, err := getPlacementData[PlacementOverview](ctx, s.client, "placement/overview", opts)
	if err == nil && out == nil {
		out = &PlacementOverview{}
	}
	return out, resp, err
}

// ListTests returns a page of the workspace's placement tests, newest first.
// Limit is 1 to 100 (default 25). Requires [PermReadAnalytics].
func (s *PlacementService) ListTests(ctx context.Context, params *PlacementTestListParams, opts ...RequestOption) (*Page[PlacementTest], error) {
	return listJSON[PlacementTest](ctx, s.client, "placement/tests", params.values(), opts...)
}

// GetTest returns one placement test with every copy, the content check and,
// for a tracking comparison, the other half. Requires [PermReadAnalytics].
func (s *PlacementService) GetTest(ctx context.Context, id string, opts ...RequestOption) (*PlacementTestDetail, *Response, error) {
	out, resp, err := getPlacementData[PlacementTestDetail](ctx, s.client, "placement/tests/"+url.PathEscape(id), opts)
	if err == nil && out == nil {
		out = &PlacementTestDetail{}
	}
	return out, resp, err
}

// CreateTest starts a placement test and answers 201. It returns one test,
// or two linked by CompareGroupID for [PlacementTrackingCompare]. A test past
// the monthly allowance fails with 402 unless MaxCredits covers its price.
// Requires [PermSendCampaigns].
func (s *PlacementService) CreateTest(ctx context.Context, params *PlacementTestCreateParams, opts ...RequestOption) ([]PlacementTest, *Response, error) {
	return sendData[PlacementTest](ctx, s.client.post, "placement/tests", params, opts)
}

// CancelTest stops the copies of a test that have not been sent yet and
// returns the test. Copies already sent keep being classified. Requires
// [PermSendCampaigns].
func (s *PlacementService) CancelTest(ctx context.Context, id string, opts ...RequestOption) (*PlacementTest, *Response, error) {
	return sendPlacementData[PlacementTest](ctx, s.client.post, "placement/tests/"+url.PathEscape(id)+"/cancel", nil, opts)
}

// ListBatches returns a page of the workspace's placement batches, newest
// first. Limit is 1 to 100 (default 25). Requires [PermReadAnalytics].
func (s *PlacementService) ListBatches(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[PlacementBatch], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[PlacementBatch](ctx, s.client, "placement/batches", q, opts...)
}

// GetBatch returns one placement batch with its placement overall and
// grouped by sending domain, sending provider and recipient provider.
// Requires [PermReadAnalytics].
func (s *PlacementService) GetBatch(ctx context.Context, id string, opts ...RequestOption) (*PlacementBatchDetail, *Response, error) {
	out, resp, err := getPlacementData[PlacementBatchDetail](ctx, s.client, "placement/batches/"+url.PathEscape(id), opts)
	if err == nil && out == nil {
		out = &PlacementBatchDetail{}
	}
	return out, resp, err
}

// ListBatchSenders returns a page of a batch's senders with where each one's
// copies landed, worst inbox rate first unless sorted otherwise. Limit is 1
// to 100 (default 25). Requires [PermReadAnalytics].
func (s *PlacementService) ListBatchSenders(ctx context.Context, id string, params *PlacementBatchSenderListParams, opts ...RequestOption) (*Page[PlacementBatchSender], error) {
	return listJSON[PlacementBatchSender](ctx, s.client, "placement/batches/"+url.PathEscape(id)+"/senders", params.values(), opts...)
}

// PreviewBatch reports how many senders, tests, sends and credits a batch
// would come to. It starts nothing. Requires [PermSendCampaigns].
func (s *PlacementService) PreviewBatch(ctx context.Context, params *PlacementBatchParams, opts ...RequestOption) (*PlacementBatchPreview, *Response, error) {
	return sendPlacementData[PlacementBatchPreview](ctx, s.client.post, "placement/batches/preview", params, opts)
}

// CreateBatch snapshots the batch's senders and queues it, answering 201; the
// server then starts its senders a few at a time. Run [PlacementService.PreviewBatch]
// first to learn the credit cost to pass as MaxCredits. Requires
// [PermSendCampaigns].
func (s *PlacementService) CreateBatch(ctx context.Context, params *PlacementBatchParams, opts ...RequestOption) (*PlacementBatch, *Response, error) {
	return sendPlacementData[PlacementBatch](ctx, s.client.post, "placement/batches", params, opts)
}

// CancelBatch stops a batch and returns it. Copies already sent keep being
// classified. Requires [PermSendCampaigns].
func (s *PlacementService) CancelBatch(ctx context.Context, id string, opts ...RequestOption) (*PlacementBatch, *Response, error) {
	return sendPlacementData[PlacementBatch](ctx, s.client.post, "placement/batches/"+url.PathEscape(id)+"/cancel", nil, opts)
}

// Coverage reports how much of the workspace's connected fleet delivered a
// placement test in the last 7 and 30 days. Requires [PermReadAnalytics].
func (s *PlacementService) Coverage(ctx context.Context, opts ...RequestOption) (*PlacementCoverage, *Response, error) {
	out, resp, err := getPlacementData[PlacementCoverage](ctx, s.client, "placement/coverage", opts)
	if err == nil && out == nil {
		out = &PlacementCoverage{}
	}
	return out, resp, err
}

// ListSeeds returns the workspace's mailboxes and which of them are its seed
// inboxes. A key limited to some mailboxes sees only those. Requires
// [PermReadEmails].
func (s *PlacementService) ListSeeds(ctx context.Context, opts ...RequestOption) ([]PlacementSeed, *Response, error) {
	return fetchData[PlacementSeed](ctx, s.client, "placement/seeds", opts)
}

// SetSeed makes one of the workspace's mailboxes a seed inbox for
// [PlacementPanelWorkspace] tests, or stops it being one. Requires
// [PermWriteEmails].
func (s *PlacementService) SetSeed(ctx context.Context, emailAccountID string, seed bool, opts ...RequestOption) (*PlacementSeed, *Response, error) {
	body := struct {
		Seed bool `json:"seed"`
	}{Seed: seed}
	return sendPlacementData[PlacementSeed](ctx, s.client.put, "placement/seeds/"+url.PathEscape(emailAccountID), body, opts)
}

// PlacementMonitor returns a campaign's scheduled placement test, or nil when
// it has none. Requires [PermReadCampaigns].
func (s *CampaignService) PlacementMonitor(ctx context.Context, id string, opts ...RequestOption) (*PlacementMonitor, *Response, error) {
	return getPlacementData[PlacementMonitor](ctx, s.client, "campaigns/"+url.PathEscape(id)+"/placement-monitor", opts)
}

// SetPlacementMonitor creates or updates a campaign's scheduled placement
// test. The write is a full-state PUT, so a retry lands on the same monitor
// and needs no idempotency key. A new or re-enabled monitor runs its first
// test within minutes. Requires [PermSendCampaigns].
func (s *CampaignService) SetPlacementMonitor(ctx context.Context, id string, params *PlacementMonitorParams, opts ...RequestOption) (*PlacementMonitor, *Response, error) {
	if params == nil {
		params = &PlacementMonitorParams{}
	}
	return sendPlacementData[PlacementMonitor](ctx, s.client.put, "campaigns/"+url.PathEscape(id)+"/placement-monitor", params, opts)
}

// DeletePlacementMonitor removes a campaign's scheduled placement test. The
// API answers 204, or 404 when the campaign has none. Requires
// [PermSendCampaigns].
func (s *CampaignService) DeletePlacementMonitor(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "campaigns/"+url.PathEscape(id)+"/placement-monitor", opts...)
}
