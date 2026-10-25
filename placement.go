package warmbly

import (
	"context"
	"net/url"
	"time"
)

// PlacementService runs inbox placement tests: a template or a campaign step
// is sent from one of the workspace's mailboxes to a panel of seed inboxes,
// and each seed reports whether the copy landed in the inbox, a Gmail tab,
// spam, or never arrived.
//
// There are two ways to run one:
//
//   - A test ([PlacementService.CreateTests]) is one sender. A tracking
//     comparison runs the same copy twice, so it starts two tests.
//   - A batch ([PlacementService.CreateBatch]) is one copy tested from many
//     senders. The server snapshots the senders and starts them a few at a
//     time, so a batch finishes over minutes or hours, not within the call.
//     [PlacementService.PreviewBatch] counts what a batch would do first.
//
// Reads ([PlacementService.Overview], the lists and gets, coverage) need
// [PermReadAnalytics]. Starting or canceling a test or batch sends real mail
// and needs [PermSendCampaigns]. Managing the workspace's own seed inboxes
// needs [PermReadEmails] to list and [PermWriteEmails] to change them. A
// campaign's scheduled test lives on [CampaignService]
// ([CampaignService.PlacementMonitor]).
//
// Credits are only charged for a test past the workspace's monthly free
// allowance, and a test that cannot afford its price is refused rather than
// charged: pass MaxCredits to state the most you agree to pay.
//
// Tests and batches are polled, not streamed: a running test's counts fill in
// as seeds report, so read it again until its Status is no longer
// [PlacementStatusRunning] (batches: [PlacementBatchStatus.Finished]).
type PlacementService service

// PlacementPanel names a seed panel a test sends to. Unknown values decode
// unchanged.
type PlacementPanel string

// Seed panels.
const (
	// PlacementPanelInstance is the panel the operator runs for every
	// workspace on the instance (on the hosted product, Warmbly's own). The
	// default.
	PlacementPanelInstance PlacementPanel = "instance"
	// PlacementPanelWorkspace is the workspace's own seed inboxes, chosen with
	// [PlacementService.SetSeed].
	PlacementPanelWorkspace PlacementPanel = "workspace"
	// PlacementPanelCloud is Warmbly Cloud's panel, for a linked self-hosted
	// instance.
	PlacementPanelCloud PlacementPanel = "cloud"
)

// PlacementStatus is the state of one test. Unknown values decode unchanged.
type PlacementStatus string

// Test statuses.
const (
	PlacementStatusRunning   PlacementStatus = "running"
	PlacementStatusCompleted PlacementStatus = "completed"
	PlacementStatusCancelled PlacementStatus = "cancelled" //nolint:misspell // wire value: the API sends "cancelled" here
	PlacementStatusFailed    PlacementStatus = "failed"
)

// PlacementOrigin is what started a test. Unknown values decode unchanged.
type PlacementOrigin string

// Test origins.
const (
	PlacementOriginManual  PlacementOrigin = "manual"
	PlacementOriginMonitor PlacementOrigin = "monitor"
	PlacementOriginAdmin   PlacementOrigin = "admin"
	// PlacementOriginRemote is a test the cloud runs for a linked instance.
	PlacementOriginRemote PlacementOrigin = "remote"
	// PlacementOriginBatch is a test a batch started for one of its senders.
	PlacementOriginBatch PlacementOrigin = "batch"
)

// PlacementFolder is where one probe landed. Unknown values decode unchanged.
type PlacementFolder string

// Probe folders.
const (
	PlacementFolderPending    PlacementFolder = "pending"
	PlacementFolderInbox      PlacementFolder = "inbox"
	PlacementFolderPromotions PlacementFolder = "promotions"
	// PlacementFolderOther is a Gmail tab other than Promotions (Updates,
	// Social, Forums).
	PlacementFolderOther PlacementFolder = "other"
	PlacementFolderSpam  PlacementFolder = "spam"
	// PlacementFolderMissing is a copy that left the sender and never showed
	// up in the seed within the classify window.
	PlacementFolderMissing PlacementFolder = "missing"
	// PlacementFolderFailed is a copy that never left the sender.
	PlacementFolderFailed    PlacementFolder = "failed"
	PlacementFolderCancelled PlacementFolder = "cancelled" //nolint:misspell // wire value: the API sends "cancelled" here
)

// PlacementTracking says whether a test's copy carries open and click
// tracking. Unknown values decode unchanged.
type PlacementTracking string

// Tracking choices.
const (
	// PlacementTrackingCampaign copies the campaign's own tracking; an
	// ad-hoc test has none. The default.
	PlacementTrackingCampaign PlacementTracking = "campaign"
	PlacementTrackingOn       PlacementTracking = "on"
	PlacementTrackingOff      PlacementTracking = "off"
	// PlacementTrackingCompare runs the same copy twice, once tracked and
	// once not, so the creation answers two tests (one batch variant pair).
	PlacementTrackingCompare PlacementTracking = "compare"
)

// PlacementPace is how a sender spaces a test's copies. Unknown values decode
// unchanged.
type PlacementPace string

// Paces.
const (
	// PlacementPaceSpaced sends about a minute apart. The default, and the
	// only pace a batch accepts.
	PlacementPaceSpaced PlacementPace = "spaced"
	// PlacementPaceQuick sends a few seconds apart, for a result in minutes.
	PlacementPaceQuick PlacementPace = "quick"
)

// PlacementBatchStatus is the state of a batch. Unknown values decode
// unchanged.
type PlacementBatchStatus string

// Batch statuses.
const (
	PlacementBatchQueued                PlacementBatchStatus = "queued"
	PlacementBatchRunning               PlacementBatchStatus = "running"
	PlacementBatchCompleted             PlacementBatchStatus = "completed"
	PlacementBatchCompletedWithWarnings PlacementBatchStatus = "completed_with_warnings"
	PlacementBatchCancelled             PlacementBatchStatus = "cancelled" //nolint:misspell // wire value: the API sends "cancelled" here
	PlacementBatchFailed                PlacementBatchStatus = "failed"
)

// Finished reports whether the status is final. An unknown status is treated
// as not finished.
func (s PlacementBatchStatus) Finished() bool {
	switch s {
	case PlacementBatchCompleted, PlacementBatchCompletedWithWarnings, PlacementBatchCancelled, PlacementBatchFailed: //nolint:misspell // wire value: the API sends "cancelled" here
		return true
	}
	return false
}

// PlacementSenderStatus is the state of one sender inside a batch. Unknown
// values decode unchanged.
type PlacementSenderStatus string

// Batch sender statuses.
const (
	PlacementSenderQueued PlacementSenderStatus = "queued"
	// PlacementSenderDeferred could not run yet (no daily headroom, busy,
	// offline) and is retried later.
	PlacementSenderDeferred  PlacementSenderStatus = "deferred"
	PlacementSenderRunning   PlacementSenderStatus = "running"
	PlacementSenderCompleted PlacementSenderStatus = "completed"
	PlacementSenderSkipped   PlacementSenderStatus = "skipped"
	PlacementSenderFailed    PlacementSenderStatus = "failed"
	PlacementSenderCancelled PlacementSenderStatus = "cancelled" //nolint:misspell // wire value: the API sends "cancelled" here
)

// PlacementUnavailable is what a batch does with a sender that cannot run
// when its turn comes. Unknown values decode unchanged.
type PlacementUnavailable string

// Handling of unavailable senders.
const (
	// PlacementUnavailableSkip drops the sender.
	PlacementUnavailableSkip PlacementUnavailable = "skip"
	// PlacementUnavailableDefer retries the sender for up to a week, then
	// skips it. The default.
	PlacementUnavailableDefer PlacementUnavailable = "defer"
)

// PlacementScopeType is how a [PlacementSenderScope] picks senders.
type PlacementScopeType string

// Sender scopes.
const (
	// PlacementScopeCampaign takes the mailboxes a campaign sends from;
	// [PlacementSenderScope.CampaignID] is then required.
	PlacementScopeCampaign PlacementScopeType = "campaign"
	// PlacementScopeWorkspace takes every sending mailbox in the workspace.
	PlacementScopeWorkspace PlacementScopeType = "workspace"
)

// PlacementSampleMode is how a [PlacementSample] keeps part of the senders.
type PlacementSampleMode string

// Sampling modes.
const (
	// PlacementSampleAll keeps every sender. The default.
	PlacementSampleAll PlacementSampleMode = "all"
	// PlacementSampleRandom keeps Count senders at random.
	PlacementSampleRandom PlacementSampleMode = "random"
	// PlacementSamplePercent keeps Percent of the senders.
	PlacementSamplePercent PlacementSampleMode = "percent"
	// PlacementSamplePerDomain keeps Count senders from each sending domain.
	PlacementSamplePerDomain PlacementSampleMode = "per_domain"
	// PlacementSamplePerProvider keeps Count senders from each sending
	// provider.
	PlacementSamplePerProvider PlacementSampleMode = "per_provider"
)

// Batch sender list sort orders for [PlacementBatchSendersParams.Sort].
const (
	// PlacementSortWorst lists the lowest inbox rate first. The default.
	PlacementSortWorst = "worst"
	PlacementSortBest  = "best"
	PlacementSortEmail = "email"
	// PlacementSortStatus orders by sender status.
	PlacementSortStatus = "status"
)

// Placement limits enforced by the server.
const (
	// PlacementMaxListLimit is the largest Limit a placement list accepts;
	// a larger one is a 400, not a clamp.
	PlacementMaxListLimit = 100
	// PlacementMonitorMinIntervalDays and PlacementMonitorMaxIntervalDays
	// bound [PlacementMonitorParams.IntervalDays].
	PlacementMonitorMinIntervalDays = 1
	PlacementMonitorMaxIntervalDays = 30
)

// PlacementCounts is where a set of probes landed. The rates are fractions
// from 0 to 1 of Delivered, and nil until something is delivered.
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
	// Delivered is every copy that left and got a verdict: Inbox, Promotions,
	// Other, Spam and Missing together.
	Delivered int `json:"delivered"`
	// InboxRate is the primary inbox share of Delivered, TabsRate the Gmail
	// tabs (Promotions and Other), SpamRate spam and MissingRate the copies
	// never seen.
	InboxRate   *float64 `json:"inbox_rate"`
	TabsRate    *float64 `json:"tabs_rate"`
	SpamRate    *float64 `json:"spam_rate"`
	MissingRate *float64 `json:"missing_rate"`
}

// PlacementFamilyCounts is one recipient host family's share of a test.
type PlacementFamilyCounts struct {
	// Family is who hosts the seed: a mailhost id such as google_workspace,
	// gmail, microsoft365, outlook or yahoo.
	Family string          `json:"family"`
	Label  string          `json:"label"`
	Counts PlacementCounts `json:"counts"`
}

// PlacementTest is one run: a copy sent from one sender to a seed panel. The
// copy itself (BodyHTML, BodyPlain) is left out of lists and present on
// [PlacementService.GetTest].
type PlacementTest struct {
	ID string `json:"id"`
	// SenderAccountID is nil once the sending mailbox has been deleted;
	// SenderEmail still names it.
	SenderAccountID *string `json:"sender_account_id"`
	SenderEmail     string  `json:"sender_email"`
	CreatedBy       *string `json:"created_by"`
	CampaignID      *string `json:"campaign_id"`
	SequenceID      *string `json:"sequence_id"`
	ContactID       *string `json:"contact_id"`
	// MonitorID is set on a test a campaign's monitor started.
	MonitorID *string `json:"monitor_id"`
	// BatchID is set on a test a batch started.
	BatchID   *string `json:"batch_id"`
	Subject   string  `json:"subject"`
	BodyHTML  string  `json:"body_html,omitempty"`
	BodyPlain string  `json:"body_plain,omitempty"`
	// OpenTracking and LinkTracking say whether the copy carried tracking.
	OpenTracking bool `json:"open_tracking"`
	LinkTracking bool `json:"link_tracking"`
	// CompareGroupID is shared by the two halves of a tracking comparison.
	CompareGroupID *string         `json:"compare_group_id"`
	Origin         PlacementOrigin `json:"origin"`
	Panel          PlacementPanel  `json:"panel"`
	Status         PlacementStatus `json:"status"`
	// Error says why a failed test failed.
	Error string        `json:"error,omitempty"`
	Pace  PlacementPace `json:"pace"`
	// CreditsCharged is what the test cost past the monthly free allowance.
	// A paid test is settled once it finishes: CreditsRefunded is what came
	// back because no copy was delivered, CreditsSettledAt when that was
	// decided (nil until then).
	CreditsCharged   int        `json:"credits_charged"`
	CreditsRefunded  int        `json:"credits_refunded"`
	CreditsSettledAt *time.Time `json:"credits_settled_at"`
	CreatedAt        time.Time  `json:"created_at"`
	FinishedAt       *time.Time `json:"finished_at"`

	// Summary counts where every probe landed; Families splits it by the
	// seed's host family.
	Summary  PlacementCounts         `json:"summary"`
	Families []PlacementFamilyCounts `json:"families"`
}

// PlacementResult is one probe: the copy as one seed received it.
type PlacementResult struct {
	// Seed is the seed's address, masked on the shared panels so the panel
	// cannot be listed and whitelisted.
	Seed        string          `json:"seed"`
	Family      string          `json:"family"`
	FamilyLabel string          `json:"family_label"`
	Folder      PlacementFolder `json:"folder"`
	ScheduledAt *time.Time      `json:"scheduled_at"`
	SentAt      *time.Time      `json:"sent_at"`
	DetectedAt  *time.Time      `json:"detected_at"`
	Error       string          `json:"error,omitempty"`
}

// PlacementContentSpan is one fragment of the copy that triggered an issue.
type PlacementContentSpan struct {
	// Field is "subject" or "body".
	Field string `json:"field"`
	Text  string `json:"text"`
	// Line is the 1-based line the fragment sits on within Field.
	Line    int    `json:"line,omitempty"`
	Excerpt string `json:"excerpt,omitempty"`
}

// PlacementContentIssue is one advisory problem the content check found.
type PlacementContentIssue struct {
	// Severity is "warn" or "high".
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	// Field is "subject" or "body" when the issue lives in exactly one of
	// them, and empty otherwise.
	Field      string                 `json:"field,omitempty"`
	Spans      []PlacementContentSpan `json:"spans,omitempty"`
	Suggestion string                 `json:"suggestion,omitempty"`
}

// PlacementContentCheck is the rules pass over a test's copy. It is advisory
// and never blocks a send.
type PlacementContentCheck struct {
	// Score runs 0 to 100, higher being safer.
	Score  int                     `json:"score"`
	Issues []PlacementContentIssue `json:"issues"`
}

// PlacementTestDetail is one test in full.
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

// PlacementPanelInfo says whether the workspace can test on one panel.
type PlacementPanelInfo struct {
	Panel     PlacementPanel `json:"panel"`
	Available bool           `json:"available"`
	// Reason explains an unavailable panel in one sentence.
	Reason   string                 `json:"reason,omitempty"`
	Seeds    int                    `json:"seeds"`
	Families []PlacementPanelFamily `json:"families"`
	// Metered panels count against the monthly allowance.
	Metered bool `json:"metered"`
}

// PlacementUsage is the workspace's monthly allowance on the metered panels.
type PlacementUsage struct {
	Used int `json:"used"`
	// Limit is nil when the instance does not meter tests (self-hosted).
	Limit *int `json:"limit"`
	// CreditsPerTest is what a test past the free allowance costs in credits,
	// zero when tests cannot be paid for.
	CreditsPerTest int `json:"credits_per_test"`
	// CreditBalance is the workspace's spendable balance, nil when credits
	// are off.
	CreditBalance *int      `json:"credit_balance"`
	PeriodStart   time.Time `json:"period_start"`
	PeriodEnd     time.Time `json:"period_end"`
}

// Remaining is how many metered tests are left this period, or -1 when tests
// are not metered.
func (u PlacementUsage) Remaining() int {
	if u.Limit == nil {
		return -1
	}
	return max(0, *u.Limit-u.Used)
}

// PlacementOverview is what the workspace can test on and how much of its
// allowance is left.
type PlacementOverview struct {
	Panels []PlacementPanelInfo `json:"panels"`
	Usage  PlacementUsage       `json:"usage"`
	// WorkspaceSeeds is how many of the workspace's own mailboxes are seeds.
	WorkspaceSeeds int `json:"workspace_seeds"`
	// SeedsPerTest is how many seeds one test sends to.
	SeedsPerTest int `json:"seeds_per_test"`
	// SpacingSeconds is the gap between a sender's copies at the spaced pace.
	SpacingSeconds int `json:"spacing_seconds"`
}

// PlacementSeed is one of the workspace's mailboxes as a seed candidate.
type PlacementSeed struct {
	EmailAccountID string `json:"email_account_id"`
	Email          string `json:"email"`
	// Family is the mailbox's host family; Label its display name.
	Family string `json:"family"`
	Label  string `json:"label"`
	Status string `json:"status"`
	// Seed reports whether the mailbox is currently a seed inbox.
	Seed bool `json:"seed"`
	// Blocker is why the mailbox cannot become a seed, empty when it can.
	Blocker string `json:"blocker,omitempty"`
}

// PlacementCoverage is how much of the workspace's connected fleet delivered
// a placement test recently.
type PlacementCoverage struct {
	Mailboxes   int `json:"mailboxes"`
	Tested7d    int `json:"tested_7d"`
	Tested30d   int `json:"tested_30d"`
	NeverTested int `json:"never_tested"`
}

// PlacementTestCreateParams starts a placement test from one mailbox.
// SenderAccountID and a copy are required: either name a campaign step
// (CampaignID, optionally SequenceID and ContactID to render for) or give
// Subject and a body.
type PlacementTestCreateParams struct {
	// SenderAccountID is the mailbox the copy is sent from.
	SenderAccountID string `json:"sender_account_id"`
	// CampaignID takes the copy from a campaign's step. SequenceID picks the
	// step (needs CampaignID) and ContactID the contact it is rendered for.
	CampaignID string `json:"campaign_id,omitempty"`
	SequenceID string `json:"sequence_id,omitempty"`
	ContactID  string `json:"contact_id,omitempty"`
	// Subject, BodyHTML and BodyPlain are an ad-hoc copy. A subject and one
	// body are required when no campaign is named.
	Subject   string `json:"subject,omitempty"`
	BodyHTML  string `json:"body_html,omitempty"`
	BodyPlain string `json:"body_plain,omitempty"`
	// Tracking defaults to [PlacementTrackingCampaign].
	Tracking PlacementTracking `json:"tracking,omitempty"`
	// Panel defaults to [PlacementPanelInstance].
	Panel PlacementPanel `json:"panel,omitempty"`
	// SeedIDs narrows a [PlacementPanelWorkspace] test to these seed inboxes.
	SeedIDs []string `json:"seed_ids,omitempty"`
	// Families keeps only seeds hosted by these providers.
	Families []string `json:"families,omitempty"`
	// Pace defaults to [PlacementPaceSpaced].
	Pace PlacementPace `json:"pace,omitempty"`
	// MaxCredits is the most you agree to pay for a test past the free
	// allowance; a higher price is refused, never charged.
	MaxCredits int `json:"max_credits,omitempty"`
}

// PlacementTestListParams filters and paginates [PlacementService.ListTests].
type PlacementTestListParams struct {
	// Limit is capped at [PlacementMaxListLimit]; more is a 400. Cursor is
	// the opaque token from a previous page.
	ListOptions
	// CampaignID keeps only tests made for that campaign.
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

// PlacementSenderScope picks a batch's senders on the server, so a fleet of
// thousands needs no id list. The filters narrow whichever scope is chosen.
type PlacementSenderScope struct {
	Type PlacementScopeType `json:"type"`
	// CampaignID is required for [PlacementScopeCampaign].
	CampaignID *string `json:"campaign_id,omitempty"`
	// Providers keeps mailboxes hosted by these families.
	Providers []string `json:"providers,omitempty"`
	Domains   []string `json:"domains,omitempty"`
	TagIDs    []string `json:"tag_ids,omitempty"`
	// IncludeInactive keeps disconnected mailboxes; they are skipped or
	// deferred when their turn comes.
	IncludeInactive bool `json:"include_inactive,omitempty"`
	// UntestedDays keeps mailboxes with no delivered placement test in the
	// last that many days (0 to 365).
	UntestedDays int `json:"untested_days,omitempty"`
}

// PlacementSample keeps part of the resolved senders.
type PlacementSample struct {
	// Mode defaults to [PlacementSampleAll] when empty.
	Mode PlacementSampleMode `json:"mode"`
	// Count is required for the random, per_domain and per_provider modes.
	Count int `json:"count,omitempty"`
	// Percent (1 to 100) is required for [PlacementSamplePercent].
	Percent int `json:"percent,omitempty"`
	// Stratify spreads a random or percent sample across "provider" or
	// "domain" in proportion to each group's size.
	Stratify string `json:"stratify,omitempty"`
}

// PlacementBatchParams describes a batch, for both
// [PlacementService.PreviewBatch] and [PlacementService.CreateBatch]. Choose
// the senders with exactly one of SenderAccountIDs and SenderScope. The copy
// is given as in [PlacementTestCreateParams]; a preview may leave it out to
// count senders alone.
type PlacementBatchParams struct {
	SenderAccountIDs []string              `json:"sender_account_ids,omitempty"`
	SenderScope      *PlacementSenderScope `json:"sender_scope,omitempty"`
	// Sample defaults to every sender when nil.
	Sample *PlacementSample `json:"sample,omitempty"`

	CampaignID string `json:"campaign_id,omitempty"`
	SequenceID string `json:"sequence_id,omitempty"`
	ContactID  string `json:"contact_id,omitempty"`
	Subject    string `json:"subject,omitempty"`
	BodyHTML   string `json:"body_html,omitempty"`
	BodyPlain  string `json:"body_plain,omitempty"`

	// Tracking defaults to [PlacementTrackingCampaign].
	Tracking PlacementTracking `json:"tracking,omitempty"`
	// Panel defaults to [PlacementPanelInstance].
	Panel PlacementPanel `json:"panel,omitempty"`
	// Pace may only be [PlacementPaceSpaced] (the default) for a batch.
	Pace     PlacementPace `json:"pace,omitempty"`
	Families []string      `json:"families,omitempty"`
	SeedIDs  []string      `json:"seed_ids,omitempty"`
	// OnUnavailable defaults to [PlacementUnavailableDefer].
	OnUnavailable PlacementUnavailable `json:"on_unavailable,omitempty"`
	// MaxCredits is the most you agree to pay across the whole batch for
	// tests past the free allowance.
	MaxCredits int `json:"max_credits,omitempty"`
}

// PlacementGroupCount is how many selected senders share a provider.
type PlacementGroupCount struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Senders int    `json:"senders"`
}

// PlacementBatchPreview is what a batch would do, before it starts.
type PlacementBatchPreview struct {
	// Matched is how many senders the scope resolved to, Selected how many
	// the sample kept and Inactive how many of those are disconnected.
	Matched   int                   `json:"matched"`
	Selected  int                   `json:"selected"`
	Inactive  int                   `json:"inactive"`
	Domains   int                   `json:"domains"`
	Providers []PlacementGroupCount `json:"providers"`
	// Variants is two for a tracking comparison, which doubles every count.
	Variants     int `json:"variants"`
	Tests        int `json:"tests"`
	SeedsPerTest int `json:"seeds_per_test"`
	// MaxSends is the most probes the batch sends; each sender's own daily
	// limit can only make it fewer.
	MaxSends int `json:"max_sends"`
	// FreeTests and PaidTests split Tests against the monthly allowance and
	// Credits is the most the paid ones cost. All three are zero on an
	// unmetered panel.
	Metered   bool           `json:"metered"`
	FreeTests int            `json:"free_tests"`
	PaidTests int            `json:"paid_tests"`
	Credits   int            `json:"credits"`
	Usage     PlacementUsage `json:"usage"`
	// SendersMax is the most senders one batch may hold.
	SendersMax int `json:"senders_max"`
	// Concurrency is how many senders send at once in this workspace.
	Concurrency int `json:"concurrency"`
}

// PlacementBatchSelection is how a batch's senders were chosen.
type PlacementBatchSelection struct {
	// SenderAccountIDs is how many ids were named, when they were.
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
	Cancelled int `json:"cancelled"` //nolint:misspell // wire value: the API sends "cancelled" here
}

// Open is how many senders still have something to do.
func (p PlacementBatchProgress) Open() int { return p.Queued + p.Deferred + p.Running }

// PlacementBatch is one copy tested from many senders, with its progress and
// headline placement.
type PlacementBatch struct {
	ID         string  `json:"id"`
	CreatedBy  *string `json:"created_by"`
	CampaignID *string `json:"campaign_id"`
	SequenceID *string `json:"sequence_id"`
	ContactID  *string `json:"contact_id"`
	Subject    string  `json:"subject"`
	// BodyHTML and BodyPlain are present on a single batch and left out of
	// lists.
	BodyHTML      string                  `json:"body_html,omitempty"`
	BodyPlain     string                  `json:"body_plain,omitempty"`
	Tracking      PlacementTracking       `json:"tracking"`
	Panel         PlacementPanel          `json:"panel"`
	Pace          PlacementPace           `json:"pace"`
	Families      []string                `json:"families"`
	SeedIDs       []string                `json:"seed_ids"`
	OnUnavailable PlacementUnavailable    `json:"on_unavailable"`
	Selection     PlacementBatchSelection `json:"selection"`
	// SenderCount is how many senders the batch snapshotted.
	SenderCount int `json:"sender_count"`
	MaxCredits  int `json:"max_credits"`
	// CreditsSpent is what the batch has charged so far.
	CreditsSpent int                  `json:"credits_spent"`
	Status       PlacementBatchStatus `json:"status"`
	Error        string               `json:"error,omitempty"`
	// RetryUntil is when a deferred sender is given up on.
	RetryUntil time.Time  `json:"retry_until"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`

	Progress PlacementBatchProgress `json:"progress"`
	// Summary is where the batch's copies landed so far; for a tracking
	// comparison it is the tracked half, the copy a campaign really sends.
	Summary PlacementCounts `json:"summary"`
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

// PlacementBatchDetail is one batch in full: its placement overall and
// grouped by sending domain, sending provider and recipient provider.
type PlacementBatchDetail struct {
	PlacementBatch
	// Untracked is the untracked half of a tracking comparison.
	Untracked  *PlacementCounts          `json:"untracked,omitempty"`
	Domains    []PlacementBatchGroup     `json:"domains"`
	Providers  []PlacementBatchGroup     `json:"providers"`
	Recipients []PlacementFamilyCounts   `json:"recipients"`
	Matrix     []PlacementBatchMatrixRow `json:"matrix"`
	Content    PlacementContentCheck     `json:"content"`
}

// PlacementBatchSender is one sender of a batch with where its copies landed.
type PlacementBatchSender struct {
	ID      string `json:"id"`
	BatchID string `json:"batch_id"`
	// EmailAccountID is nil once the mailbox has been deleted.
	EmailAccountID *string               `json:"email_account_id"`
	SenderEmail    string                `json:"sender_email"`
	SenderDomain   string                `json:"sender_domain"`
	SenderFamily   string                `json:"sender_family"`
	Status         PlacementSenderStatus `json:"status"`
	// Reason and Detail say why a sender was skipped, deferred or failed.
	Reason        string     `json:"reason,omitempty"`
	Detail        string     `json:"detail,omitempty"`
	Attempts      int        `json:"attempts"`
	NextAttemptAt time.Time  `json:"next_attempt_at"`
	StartedAt     *time.Time `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at"`

	SenderFamilyLabel string          `json:"sender_family_label"`
	Summary           PlacementCounts `json:"summary"`
	// TestIDs are the tests the sender ran, two for a tracking comparison.
	TestIDs []string `json:"test_ids"`
}

// PlacementBatchSendersParams filters and orders
// [PlacementService.BatchSenders].
type PlacementBatchSendersParams struct {
	// Limit is capped at [PlacementMaxListLimit]; more is a 400.
	ListOptions
	// Status keeps senders in one [PlacementSenderStatus].
	Status PlacementSenderStatus
	// Query is a free-text filter on the sender address (at most 200
	// characters).
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
	setNonEmpty(q, "status", string(p.Status))
	setNonEmpty(q, "q", p.Query)
	setNonEmpty(q, "sort", p.Sort)
	return q
}

// PlacementMonitor re-tests a campaign's first email step on a schedule.
type PlacementMonitor struct {
	ID         string  `json:"id"`
	CampaignID string  `json:"campaign_id"`
	CreatedBy  *string `json:"created_by"`
	Enabled    bool    `json:"enabled"`
	// IntervalDays is the gap between runs, 1 to 30.
	IntervalDays int            `json:"interval_days"`
	Panel        PlacementPanel `json:"panel"`
	// AlertBelow is the inbox rate in percent (0 to 100) under which a run
	// raises an alert.
	AlertBelow int `json:"alert_below"`
	// PauseOnAlert stops the campaign when a run alerts.
	PauseOnAlert bool       `json:"pause_on_alert"`
	NextRunAt    time.Time  `json:"next_run_at"`
	LastRunAt    *time.Time `json:"last_run_at"`
	LastTestID   *string    `json:"last_test_id"`
	LastAlertAt  *time.Time `json:"last_alert_at"`
	// LastError is why the last run could not start.
	LastError string    `json:"last_error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PlacementMonitorParams creates or changes a campaign's monitor. Nil fields
// keep the stored value, or the default on a new monitor (enabled, every 7
// days, the instance panel, alert below 70, no pause).
type PlacementMonitorParams struct {
	Enabled *bool `json:"enabled"`
	// IntervalDays is 1 to 30.
	IntervalDays *int            `json:"interval_days"`
	Panel        *PlacementPanel `json:"panel"`
	// AlertBelow is an inbox rate in percent, 0 to 100.
	AlertBelow   *int  `json:"alert_below"`
	PauseOnAlert *bool `json:"pause_on_alert"`
}

// --- wrapped-object helpers --------------------------------------------

// placementWrapped is the {"data": {...}} envelope placement answers use for a
// single object.
type placementWrapped[T any] struct {
	Data T `json:"data"`
}

// placementGet and placementSend always return a non-nil result on success,
// as [send] does, even if the server's data field is missing.
func placementGet[T any](ctx context.Context, c *Client, path string, opts []RequestOption) (*T, *Response, error) {
	var env placementWrapped[T]
	resp, err := c.get(ctx, path, &env, opts...)
	if err != nil {
		return nil, resp, err
	}
	return &env.Data, resp, nil
}

func placementSend[T any](ctx context.Context, verb bodyVerb, path string, body any, opts []RequestOption) (*T, *Response, error) {
	var env placementWrapped[T]
	resp, err := verb(ctx, path, body, &env, opts...)
	if err != nil {
		return nil, resp, err
	}
	return &env.Data, resp, nil
}

// placementGetOptional is placementGet for a resource that may not exist, where
// "data": null means absent and yields a nil result.
func placementGetOptional[T any](ctx context.Context, c *Client, path string, opts []RequestOption) (*T, *Response, error) {
	var env placementWrapped[*T]
	resp, err := c.get(ctx, path, &env, opts...)
	if err != nil {
		return nil, resp, err
	}
	return env.Data, resp, nil
}

// --- workspace ---------------------------------------------------------

// Overview returns the seed panels the workspace can test on and how much of
// its monthly allowance is left.
//
// Requires [PermReadAnalytics].
func (s *PlacementService) Overview(ctx context.Context, opts ...RequestOption) (*PlacementOverview, *Response, error) {
	return placementGet[PlacementOverview](ctx, s.client, "placement/overview", opts)
}

// Coverage reports how much of the workspace's connected fleet delivered a
// placement test in the last 7 and 30 days.
//
// Requires [PermReadAnalytics]. A server without batch support answers 501.
func (s *PlacementService) Coverage(ctx context.Context, opts ...RequestOption) (*PlacementCoverage, *Response, error) {
	return placementGet[PlacementCoverage](ctx, s.client, "placement/coverage", opts)
}

// --- tests -------------------------------------------------------------

// CreateTests starts a placement test from one mailbox and returns it, or the
// two halves of a tracking comparison ([PlacementTrackingCompare]). The tests
// start running immediately; read them with [PlacementService.GetTest].
//
// It can be refused with a 402 (no entitlement, quota or credits), 409 (the
// sender is not connected, a test is still sending from it, or the panel has
// no seeds) or 429 (too many tests running); the [Error.Code] names which.
//
// Requires [PermSendCampaigns]. The call sends real mail and is not
// idempotent: a retry can start a second test, so pass [WithIdempotencyKey].
func (s *PlacementService) CreateTests(ctx context.Context, params *PlacementTestCreateParams, opts ...RequestOption) ([]PlacementTest, *Response, error) {
	return sendData[PlacementTest](ctx, s.client.post, "placement/tests", params, opts)
}

// ListTests returns a page of the workspace's tests, newest first. Tests a
// batch started are left to [PlacementService.GetBatch].
//
// Requires [PermReadAnalytics].
func (s *PlacementService) ListTests(ctx context.Context, params *PlacementTestListParams, opts ...RequestOption) (*Page[PlacementTest], error) {
	return listJSON[PlacementTest](ctx, s.client, "placement/tests", params.values(), opts...)
}

// GetTest returns one test with every probe, the content check and, for a
// tracking comparison, the other half.
//
// Requires [PermReadAnalytics].
func (s *PlacementService) GetTest(ctx context.Context, id string, opts ...RequestOption) (*PlacementTestDetail, *Response, error) {
	return placementGet[PlacementTestDetail](ctx, s.client, "placement/tests/"+url.PathEscape(id), opts)
}

// CancelTest stops the probes of a running test that have not been sent yet.
// Probes already sent keep being classified. A test that is no longer running
// answers a 409.
//
// Requires [PermSendCampaigns].
func (s *PlacementService) CancelTest(ctx context.Context, id string, opts ...RequestOption) (*PlacementTest, *Response, error) {
	return placementSend[PlacementTest](ctx, s.client.post, "placement/tests/"+url.PathEscape(id)+"/cancel", nil, opts)
}

// --- batches -----------------------------------------------------------

// PreviewBatch reports how many senders, tests, sends and credits a batch
// would come to. It starts nothing.
//
// Requires [PermSendCampaigns].
func (s *PlacementService) PreviewBatch(ctx context.Context, params *PlacementBatchParams, opts ...RequestOption) (*PlacementBatchPreview, *Response, error) {
	return placementSend[PlacementBatchPreview](ctx, s.client.post, "placement/batches/preview", params, opts)
}

// CreateBatch snapshots the senders and queues the batch. The backend then
// starts its senders a few at a time; follow progress with
// [PlacementService.GetBatch]. A workspace may have only a handful of batches
// open at once (a 429 beyond that).
//
// Requires [PermSendCampaigns]. The call sends real mail and is not
// idempotent: a retry can start a second batch, so pass [WithIdempotencyKey].
func (s *PlacementService) CreateBatch(ctx context.Context, params *PlacementBatchParams, opts ...RequestOption) (*PlacementBatch, *Response, error) {
	return placementSend[PlacementBatch](ctx, s.client.post, "placement/batches", params, opts)
}

// ListBatches returns a page of the workspace's batches, newest first.
//
// Requires [PermReadAnalytics].
func (s *PlacementService) ListBatches(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[PlacementBatch], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[PlacementBatch](ctx, s.client, "placement/batches", q, opts...)
}

// GetBatch returns one batch with its placement overall and grouped by
// sending domain, sending provider and recipient provider.
//
// Requires [PermReadAnalytics].
func (s *PlacementService) GetBatch(ctx context.Context, id string, opts ...RequestOption) (*PlacementBatchDetail, *Response, error) {
	return placementGet[PlacementBatchDetail](ctx, s.client, "placement/batches/"+url.PathEscape(id), opts)
}

// BatchSenders returns a page of a batch's senders with where each one's
// copies landed, worst inbox rate first unless sorted otherwise.
//
// Requires [PermReadAnalytics].
func (s *PlacementService) BatchSenders(ctx context.Context, id string, params *PlacementBatchSendersParams, opts ...RequestOption) (*Page[PlacementBatchSender], error) {
	return listJSON[PlacementBatchSender](ctx, s.client, "placement/batches/"+url.PathEscape(id)+"/senders", params.values(), opts...)
}

// CancelBatch stops a batch: no sender starts again and copies not yet sent
// are canceled. Copies already sent keep being classified. A batch that is
// no longer running answers a 409.
//
// Requires [PermSendCampaigns].
func (s *PlacementService) CancelBatch(ctx context.Context, id string, opts ...RequestOption) (*PlacementBatch, *Response, error) {
	return placementSend[PlacementBatch](ctx, s.client.post, "placement/batches/"+url.PathEscape(id)+"/cancel", nil, opts)
}

// --- seeds -------------------------------------------------------------

// Seeds lists the workspace's mailboxes and which of them are its seed
// inboxes. An API key restricted to some mailboxes only sees those.
//
// Requires [PermReadEmails].
func (s *PlacementService) Seeds(ctx context.Context, opts ...RequestOption) ([]PlacementSeed, *Response, error) {
	return fetchData[PlacementSeed](ctx, s.client, "placement/seeds", opts)
}

// SetSeed makes a workspace mailbox a seed inbox, or stops it being one. A
// mailbox that cannot be a seed (see [PlacementSeed.Blocker]) is refused.
//
// Requires [PermWriteEmails] and, for a restricted API key, access to that
// mailbox.
func (s *PlacementService) SetSeed(ctx context.Context, emailAccountID string, seed bool, opts ...RequestOption) (*PlacementSeed, *Response, error) {
	body := struct {
		Seed bool `json:"seed"`
	}{Seed: seed}
	return placementSend[PlacementSeed](ctx, s.client.put, "placement/seeds/"+url.PathEscape(emailAccountID), body, opts)
}

// --- campaign monitor --------------------------------------------------

// PlacementMonitor returns the campaign's scheduled placement test. It
// returns a nil monitor, with no error, when the campaign has none.
//
// Requires [PermReadCampaigns].
func (s *CampaignService) PlacementMonitor(ctx context.Context, id string, opts ...RequestOption) (*PlacementMonitor, *Response, error) {
	return placementGetOptional[PlacementMonitor](ctx, s.client, "campaigns/"+url.PathEscape(id)+"/placement-monitor", opts)
}

// SetPlacementMonitor creates the campaign's scheduled placement test or
// changes it. A new or re-enabled monitor runs its first test within minutes.
// Nil fields keep the stored value (the server merges rather than replaces),
// so repeating a call lands on the same state.
//
// Requires [PermSendCampaigns], since each run sends real mail.
func (s *CampaignService) SetPlacementMonitor(ctx context.Context, id string, params *PlacementMonitorParams, opts ...RequestOption) (*PlacementMonitor, *Response, error) {
	if params == nil {
		params = &PlacementMonitorParams{}
	}
	return placementSend[PlacementMonitor](ctx, s.client.put, "campaigns/"+url.PathEscape(id)+"/placement-monitor", params, opts)
}

// DeletePlacementMonitor removes the campaign's scheduled placement test. A
// campaign with no monitor answers a 404.
//
// Requires [PermSendCampaigns].
func (s *CampaignService) DeletePlacementMonitor(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "campaigns/"+url.PathEscape(id)+"/placement-monitor", opts...)
}
