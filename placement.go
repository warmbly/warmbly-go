package warmbly

import (
	"context"
	"net/url"
	"time"
)

// PlacementService runs inbox placement tests: a template or campaign step is
// sent from a real mailbox to a panel of seed inboxes, and each copy is
// classified by where it landed (inbox, Promotions, spam, ...). A test can be
// run from one mailbox ([PlacementService.Start]) or from many at once as a
// batch ([PlacementService.StartBatch]).
//
// Reads need the READ_ANALYTICS scope; starting or canceling a test or batch
// sends mail, so it needs SEND_CAMPAIGNS; changing which mailboxes are seeds
// needs WRITE_EMAILS. An organization must be selected for every call.
type PlacementService service

// Seed panels a test can send to, for [PlacementTestParams.Panel].
const (
	// PlacementPanelInstance is the panel the operator runs for every
	// workspace on the instance (on the hosted product, Warmbly's own).
	PlacementPanelInstance = "instance"
	// PlacementPanelWorkspace is the workspace's own seed inboxes.
	PlacementPanelWorkspace = "workspace"
	// PlacementPanelCloud is Warmbly Cloud's panel, for a linked self-hosted
	// instance.
	PlacementPanelCloud = "cloud"
)

// Statuses of a placement test, in [PlacementTest.Status].
const (
	PlacementStatusRunning   = "running"
	PlacementStatusCompleted = "completed"
	PlacementStatusCancelled = "cancelled" //nolint:misspell // wire value: the API sends "cancelled" here
	PlacementStatusFailed    = "failed"
)

// What started a test, in [PlacementTest.Origin]. Treat the set as open.
const (
	PlacementOriginManual  = "manual"
	PlacementOriginMonitor = "monitor"
	PlacementOriginAdmin   = "admin"
	PlacementOriginRemote  = "remote"
	PlacementOriginBatch   = "batch"
)

// Where one copy landed, in [PlacementResult.Folder]. Treat the set as open.
const (
	PlacementFolderPending    = "pending"
	PlacementFolderInbox      = "inbox"
	PlacementFolderPromotions = "promotions"
	// PlacementFolderOther is a Gmail tab other than Promotions.
	PlacementFolderOther = "other"
	PlacementFolderSpam  = "spam"
	// PlacementFolderMissing is a copy that left the sender and never showed
	// up in the seed within the classification window.
	PlacementFolderMissing = "missing"
	// PlacementFolderFailed is a copy that never left the sender.
	PlacementFolderFailed    = "failed"
	PlacementFolderCancelled = "cancelled" //nolint:misspell // wire value: the API sends "cancelled" here
	PlacementFolderUnknown   = "unknown"
	PlacementFolderArchive   = "archive"
	PlacementFolderCustom    = "custom"
)

// Tracking choices for [PlacementTestParams.Tracking].
const (
	// PlacementTrackingCampaign copies the campaign's own open and click
	// tracking; an ad-hoc test has none.
	PlacementTrackingCampaign = "campaign"
	PlacementTrackingOn       = "on"
	PlacementTrackingOff      = "off"
	// PlacementTrackingCompare runs the same copy twice, once tracked and once
	// not, interleaved per seed. It starts two tests.
	PlacementTrackingCompare = "compare"
)

// Pacing of a test's copies from one sender, for [PlacementTestParams.Pace].
const (
	// PlacementPaceSpaced sends about a minute apart. The default.
	PlacementPaceSpaced = "spaced"
	// PlacementPaceQuick sends a few seconds apart, for a result in minutes.
	PlacementPaceQuick = "quick"
)

// Statuses of a placement batch, in [PlacementBatch.Status]. Treat the set as
// open.
const (
	PlacementBatchQueued                = "queued"
	PlacementBatchRunning               = "running"
	PlacementBatchCompleted             = "completed"
	PlacementBatchCompletedWithWarnings = "completed_with_warnings"
	PlacementBatchCancelled             = "cancelled" //nolint:misspell // wire value: the API sends "cancelled" here
	PlacementBatchFailed                = "failed"
)

// Statuses of one sender in a batch, in [PlacementBatchSender.Status]. Treat
// the set as open.
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

// What a batch does with a sender that cannot run when its turn comes, for
// [PlacementBatchParams.OnUnavailable].
const (
	PlacementUnavailableSkip  = "skip"
	PlacementUnavailableDefer = "defer"
)

// Sender scope types for [PlacementSenderScope.Type].
const (
	// PlacementScopeCampaign selects a campaign's senders; set CampaignID.
	PlacementScopeCampaign = "campaign"
	// PlacementScopeWorkspace selects every mailbox in the workspace.
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

// ObservationMetric is a rate together with the population it was measured on
// and its uncertainty, so a figure is never read without its denominator.
type ObservationMetric struct {
	Version              string `json:"version"`
	Population           string `json:"population"`
	DenominatorKind      string `json:"denominator_kind"`
	Unit                 string `json:"unit"`
	Source               string `json:"source"`
	ClassificationPolicy string `json:"classification_policy"`
	Numerator            int    `json:"numerator"`
	Denominator          int    `json:"denominator"`
	Unresolved           int    `json:"unresolved"`
	WindowDays           int    `json:"window_days,omitempty"`
	WindowBasis          string `json:"window_basis"`
	MissingnessBasis     string `json:"missingness_basis"`
	// Value is Numerator over Denominator; nil when the denominator is zero.
	Value *float64 `json:"value"`
	// Interval is the 95% Wilson interval, assuming independent observations;
	// nil when there is no value.
	Interval *ObservationInterval `json:"wilson_95_independence_interval"`
}

// ObservationInterval bounds an [ObservationMetric].
type ObservationInterval struct {
	Lower float64 `json:"lower"`
	Upper float64 `json:"upper"`
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
	// Observed counts the copies seen in a seed (classified, unknown, archived
	// or custom); Classified those with a firm inbox, tab or spam verdict;
	// Unresolved those still pending or missing.
	Observed   int `json:"observed_receipts"`
	Classified int `json:"classified_receipts"`
	Unresolved int `json:"unresolved"`
	// PrimaryMetric is the inbox share of the classified copies, NonSpamMetric
	// the share that was not spam.
	PrimaryMetric *ObservationMetric `json:"primary_metric"`
	NonSpamMetric *ObservationMetric `json:"non_spam_metric"`
	// Delivered is every copy that left and got a verdict: inbox, promotions,
	// other, spam and missing.
	Delivered int `json:"delivered"`
	// InboxRate is the primary inbox share of Delivered, TabsRate the Gmail
	// tabs, SpamRate spam and MissingRate the copies never seen. All four are
	// nil until something is delivered.
	InboxRate   *float64 `json:"inbox_rate"`
	TabsRate    *float64 `json:"tabs_rate"`
	SpamRate    *float64 `json:"spam_rate"`
	MissingRate *float64 `json:"missing_rate"`
}

// PlacementFamilyCounts is one host family's share of a test.
type PlacementFamilyCounts struct {
	// Family is who hosts the seeds, for example "gmail", "microsoft365".
	Family string          `json:"family"`
	Label  string          `json:"label"`
	Counts PlacementCounts `json:"counts"`
}

// PlacementTest is one run: a template sent from one sender to a seed panel.
// In a list the copy (BodyHTML, BodyPlain) is left out.
type PlacementTest struct {
	ID              string  `json:"id"`
	SenderAccountID *string `json:"sender_account_id"`
	SenderEmail     string  `json:"sender_email"`
	CreatedBy       *string `json:"created_by"`
	CampaignID      *string `json:"campaign_id"`
	SequenceID      *string `json:"sequence_id"`
	ContactID       *string `json:"contact_id"`
	MonitorID       *string `json:"monitor_id"`
	// BatchID names the batch that started the test, if one did.
	BatchID      *string `json:"batch_id"`
	Subject      string  `json:"subject"`
	BodyHTML     string  `json:"body_html,omitempty"`
	BodyPlain    string  `json:"body_plain,omitempty"`
	OpenTracking bool    `json:"open_tracking"`
	LinkTracking bool    `json:"link_tracking"`
	// CompareGroupID ties the two halves of a [PlacementTrackingCompare] test.
	CompareGroupID *string `json:"compare_group_id"`
	// Origin is one of the PlacementOrigin* values and Panel one of the
	// PlacementPanel* values.
	Origin string `json:"origin"`
	Panel  string `json:"panel"`
	// Status is one of the PlacementStatus* values.
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	Pace   string `json:"pace"`
	// CreditsCharged is what the test cost past the monthly free allowance;
	// CreditsRefunded what came back because no copy was delivered, and
	// CreditsSettledAt when that was decided.
	CreditsCharged   int        `json:"credits_charged"`
	CreditsRefunded  int        `json:"credits_refunded"`
	CreditsSettledAt *time.Time `json:"credits_settled_at"`
	CreatedAt        time.Time  `json:"created_at"`
	FinishedAt       *time.Time `json:"finished_at"`

	// Summary is where the copies landed, overall; Families the same per host
	// family.
	Summary  PlacementCounts         `json:"summary"`
	Families []PlacementFamilyCounts `json:"families"`
}

// PlacementEvidence is what the seed's mail server recorded about a received
// copy. Every verdict is "pass", "fail" or "unknown"; treat the sets as open.
type PlacementEvidence struct {
	Version    string    `json:"version"`
	Source     string    `json:"source"`
	Trust      string    `json:"trust"`
	ObservedAt time.Time `json:"observed_at"`
	SPF        string    `json:"spf"`
	DKIM       string    `json:"dkim"`
	DMARC      string    `json:"dmarc"`
	Alignment  string    `json:"alignment"`
	TLS        string    `json:"tls"`
	FromDomain string    `json:"from_domain,omitempty"`
	// The *Claim fields are what the message itself asserts.
	EnvelopeDomainClaim          string `json:"envelope_domain_claim,omitempty"`
	SigningDomainClaim           string `json:"signing_domain_claim,omitempty"`
	SelectorClaim                string `json:"selector_claim,omitempty"`
	AuthenticationResultsPresent bool   `json:"authentication_results_present"`
	OneClickHeadersPresent       bool   `json:"one_click_headers_present"`
	OneClickSigningClaim         bool   `json:"one_click_signing_claim"`
	OneClickCompliance           string `json:"one_click_compliance"`
	// DKIMVerification is Warmbly's own DKIM check of the copy, when it ran.
	DKIMVerification *PlacementDKIMResult `json:"dkim_verification,omitempty"`
}

// PlacementDKIMResult is Warmbly's own verification of a copy's DKIM
// signature.
type PlacementDKIMResult struct {
	DKIM          string    `json:"dkim"`
	Alignment     string    `json:"alignment"`
	SigningDomain string    `json:"signing_domain,omitempty"`
	Verifier      string    `json:"verifier"`
	ObservedAt    time.Time `json:"observed_at"`
}

// PlacementResult is one copy sent to one seed.
type PlacementResult struct {
	// Seed is the seed's address, masked on the shared panels.
	Seed        string `json:"seed"`
	Family      string `json:"family"`
	FamilyLabel string `json:"family_label"`
	// Folder is one of the PlacementFolder* values.
	Folder      string     `json:"folder"`
	ScheduledAt *time.Time `json:"scheduled_at"`
	SentAt      *time.Time `json:"sent_at"`
	DetectedAt  *time.Time `json:"detected_at"`
	Error       string     `json:"error,omitempty"`
	// FirstFolder is where the copy sat when first seen, which differs from
	// Folder when the seed's mailbox moved it afterwards; LateObservation
	// reports that the first sighting came after the classification window.
	FirstFolder     *string            `json:"first_folder,omitempty"`
	ObservedAt      *time.Time         `json:"observed_at,omitempty"`
	LateObservation bool               `json:"late_observation,omitempty"`
	Evidence        *PlacementEvidence `json:"evidence"`
}

// ContentSpan is the exact fragment of a copy that triggered a content issue.
type ContentSpan struct {
	// Field is "subject" or "body".
	Field   string `json:"field"`
	Text    string `json:"text"`
	Line    int    `json:"line,omitempty"`
	Excerpt string `json:"excerpt,omitempty"`
}

// ContentIssue is one problem the content check found in a copy.
type ContentIssue struct {
	// Severity is "warn" or "high".
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	// Field is "subject" or "body" when the issue lives in exactly one of
	// them, and empty otherwise.
	Field      string        `json:"field,omitempty"`
	Spans      []ContentSpan `json:"spans,omitempty"`
	Suggestion string        `json:"suggestion,omitempty"`
}

// ContentCheck is the rules pass over a test's copy.
type ContentCheck struct {
	Score  int            `json:"score"`
	Issues []ContentIssue `json:"issues"`
}

// PlacementTestDetail is one test in full.
type PlacementTestDetail struct {
	PlacementTest
	Results []PlacementResult `json:"results"`
	Content ContentCheck      `json:"content"`
	// Compare is the other half of a [PlacementTrackingCompare] test.
	Compare *PlacementTest `json:"compare,omitempty"`
}

// PlacementPanelFamily counts the seeds one host family contributes.
type PlacementPanelFamily struct {
	Family string `json:"family"`
	Label  string `json:"label"`
	Seeds  int    `json:"seeds"`
}

// PlacementPanelInfo says whether the workspace can test on a panel.
type PlacementPanelInfo struct {
	// Panel is one of the PlacementPanel* values.
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
	// zero when tests cannot be paid for. CreditBalance is the spendable
	// balance, nil when credits are off.
	CreditsPerTest int       `json:"credits_per_test"`
	CreditBalance  *int      `json:"credit_balance"`
	PeriodStart    time.Time `json:"period_start"`
	PeriodEnd      time.Time `json:"period_end"`
}

// PlacementOverview lists the seed panels the workspace can test on and its
// monthly allowance.
type PlacementOverview struct {
	Panels         []PlacementPanelInfo `json:"panels"`
	Usage          PlacementUsage       `json:"usage"`
	WorkspaceSeeds int                  `json:"workspace_seeds"`
	SeedsPerTest   int                  `json:"seeds_per_test"`
	SpacingSeconds int                  `json:"spacing_seconds"`
}

// PlacementTestParams starts a placement test from one mailbox. Give either a
// campaign (CampaignID, optionally a step with SequenceID) to test its copy,
// or Subject and a body.
type PlacementTestParams struct {
	// SenderAccountID is the mailbox the copies are sent from. Required.
	SenderAccountID string `json:"sender_account_id"`
	CampaignID      string `json:"campaign_id,omitempty"`
	SequenceID      string `json:"sequence_id,omitempty"`
	// ContactID renders the copy's variables for this contact.
	ContactID string `json:"contact_id,omitempty"`
	Subject   string `json:"subject,omitempty"`
	BodyHTML  string `json:"body_html,omitempty"`
	BodyPlain string `json:"body_plain,omitempty"`
	// Tracking is one of the PlacementTracking* values.
	Tracking string `json:"tracking,omitempty"`
	// Panel is one of the PlacementPanel* values.
	Panel string `json:"panel,omitempty"`
	// SeedIDs narrows a workspace-panel test to these seed mailbox ids.
	SeedIDs []string `json:"seed_ids,omitempty"`
	// Families keeps only seeds hosted by these providers.
	Families []string `json:"families,omitempty"`
	// Pace is [PlacementPaceSpaced] or [PlacementPaceQuick].
	Pace string `json:"pace,omitempty"`
	// MaxCredits is the most the caller agrees to pay for a test past the
	// monthly free allowance; a higher price is refused, never charged.
	MaxCredits int `json:"max_credits,omitempty"`
}

// PlacementListParams filters and paginates placement tests. The page size
// defaults to 25 and is capped at 100.
type PlacementListParams struct {
	ListOptions
	// CampaignID keeps only tests of this campaign.
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

// placementData decodes the {"data": ...} wrapper most placement answers use.
type placementData[T any] struct {
	Data T `json:"data"`
}

func placementOne[T any](ctx context.Context, verb bodyVerb, path string, body any, opts []RequestOption) (*T, *Response, error) {
	var env placementData[*T]
	resp, err := verb(ctx, path, body, &env, opts...)
	if err != nil {
		return nil, resp, err
	}
	return env.Data, resp, nil
}

// Overview lists the seed panels the workspace can test on and its monthly
// allowance. Needs READ_ANALYTICS.
func (s *PlacementService) Overview(ctx context.Context, opts ...RequestOption) (*PlacementOverview, *Response, error) {
	var env placementData[*PlacementOverview]
	resp, err := s.client.get(ctx, "placement/overview", &env, opts...)
	if err != nil {
		return nil, resp, err
	}
	return env.Data, resp, nil
}

// Start starts a test from one mailbox. It answers with one test, or two for a
// [PlacementTrackingCompare] test. Sending real mail makes it the SEND_CAMPAIGNS
// scope; the API key must also be allowed to use the sender mailbox. A test
// past the monthly free allowance is charged in credits up to MaxCredits.
func (s *PlacementService) Start(ctx context.Context, params *PlacementTestParams, opts ...RequestOption) ([]PlacementTest, *Response, error) {
	return sendData[PlacementTest](ctx, s.client.post, "placement/tests", params, opts)
}

// List returns a page of the workspace's tests, newest first. Needs
// READ_ANALYTICS.
func (s *PlacementService) List(ctx context.Context, params *PlacementListParams, opts ...RequestOption) (*Page[PlacementTest], error) {
	return listJSON[PlacementTest](ctx, s.client, "placement/tests", params.values(), opts...)
}

// Get retrieves one test with every copy, the content check and, for a
// tracking comparison, the other half. Needs READ_ANALYTICS.
func (s *PlacementService) Get(ctx context.Context, id string, opts ...RequestOption) (*PlacementTestDetail, *Response, error) {
	var env placementData[*PlacementTestDetail]
	resp, err := s.client.get(ctx, "placement/tests/"+url.PathEscape(id), &env, opts...)
	if err != nil {
		return nil, resp, err
	}
	return env.Data, resp, nil
}

// Cancel stops the copies of a test that have not been sent yet. Needs
// SEND_CAMPAIGNS.
func (s *PlacementService) Cancel(ctx context.Context, id string, opts ...RequestOption) (*PlacementTest, *Response, error) {
	return placementOne[PlacementTest](ctx, s.client.post, "placement/tests/"+url.PathEscape(id)+"/cancel", nil, opts)
}

// --- batches ---

// PlacementSenderScope selects a batch's senders on the server, so a fleet of
// thousands needs no id list. The filters narrow whichever scope is chosen.
type PlacementSenderScope struct {
	// Type is [PlacementScopeCampaign] or [PlacementScopeWorkspace].
	Type       string `json:"type"`
	CampaignID string `json:"campaign_id,omitempty"`
	// Providers keeps mailboxes hosted by these families.
	Providers []string `json:"providers,omitempty"`
	Domains   []string `json:"domains,omitempty"`
	TagIDs    []string `json:"tag_ids,omitempty"`
	// IncludeInactive keeps disconnected mailboxes; they are skipped or
	// deferred when their turn comes.
	IncludeInactive bool `json:"include_inactive,omitempty"`
	// UntestedDays keeps mailboxes with no delivered test in that many days.
	UntestedDays int `json:"untested_days,omitempty"`
}

// PlacementSample picks part of the resolved senders.
type PlacementSample struct {
	// Mode is one of the PlacementSample* values.
	Mode    string `json:"mode"`
	Count   int    `json:"count,omitempty"`
	Percent int    `json:"percent,omitempty"`
	// Stratify spreads a random or percent sample across "provider" or
	// "domain" in proportion to each group's size.
	Stratify string `json:"stratify,omitempty"`
}

// PlacementBatchParams runs one placement test from many senders. Give either
// SenderAccountIDs or SenderScope. The same body drives
// [PlacementService.PreviewBatch] and [PlacementService.StartBatch].
type PlacementBatchParams struct {
	SenderAccountIDs []string              `json:"sender_account_ids,omitempty"`
	SenderScope      *PlacementSenderScope `json:"sender_scope,omitempty"`
	// Sample keeps part of the resolved senders; the zero value keeps all.
	Sample     PlacementSample `json:"sample"`
	CampaignID string          `json:"campaign_id,omitempty"`
	SequenceID string          `json:"sequence_id,omitempty"`
	ContactID  string          `json:"contact_id,omitempty"`
	Subject    string          `json:"subject,omitempty"`
	BodyHTML   string          `json:"body_html,omitempty"`
	BodyPlain  string          `json:"body_plain,omitempty"`
	Tracking   string          `json:"tracking,omitempty"`
	Panel      string          `json:"panel,omitempty"`
	Pace       string          `json:"pace,omitempty"`
	Families   []string        `json:"families,omitempty"`
	SeedIDs    []string        `json:"seed_ids,omitempty"`
	// OnUnavailable is [PlacementUnavailableSkip] or
	// [PlacementUnavailableDefer].
	OnUnavailable string `json:"on_unavailable,omitempty"`
	// MaxCredits is the most the caller agrees to pay across the whole batch
	// for tests past the monthly free allowance.
	MaxCredits int `json:"max_credits,omitempty"`
}

// PlacementBatchGroupCount is how many selected senders share a provider.
type PlacementBatchGroupCount struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Senders int    `json:"senders"`
}

// PlacementBatchPreview is what a batch would do, before it is started.
type PlacementBatchPreview struct {
	// Matched is how many senders the scope resolved to and Selected how many
	// the sample kept.
	Matched   int                        `json:"matched"`
	Selected  int                        `json:"selected"`
	Inactive  int                        `json:"inactive"`
	Domains   int                        `json:"domains"`
	Providers []PlacementBatchGroupCount `json:"providers"`
	// Variants is two for a tracking comparison, which doubles every count.
	Variants     int `json:"variants"`
	Tests        int `json:"tests"`
	SeedsPerTest int `json:"seeds_per_test"`
	// MaxSends is the most copies the batch sends; a sender's own daily limit
	// can only make it fewer.
	MaxSends int `json:"max_sends"`
	// FreeTests and PaidTests split Tests against the monthly allowance and
	// Credits is the most the paid ones cost. Unmetered panels leave all three
	// zero.
	Metered   bool           `json:"metered"`
	FreeTests int            `json:"free_tests"`
	PaidTests int            `json:"paid_tests"`
	Credits   int            `json:"credits"`
	Usage     PlacementUsage `json:"usage"`
	// SendersMax is the most senders one batch takes and Concurrency how many
	// send at once in this workspace.
	SendersMax  int `json:"senders_max"`
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
	Cancelled int `json:"cancelled"` //nolint:misspell // wire value: the API sends "cancelled" here
}

// PlacementBatch runs one placement test from many senders.
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
	// OnUnavailable is one of the PlacementUnavailable* values.
	OnUnavailable string                  `json:"on_unavailable"`
	Selection     PlacementBatchSelection `json:"selection"`
	SenderCount   int                     `json:"sender_count"`
	MaxCredits    int                     `json:"max_credits"`
	CreditsSpent  int                     `json:"credits_spent"`
	// Status is one of the PlacementBatch* values.
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	// RetryUntil is how long deferred senders keep being retried.
	RetryUntil time.Time  `json:"retry_until"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`

	// Progress counts the senders by status and Summary is where the batch's
	// copies landed (the tracked half of a tracking comparison).
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
	PlacementBatch
	// Untracked is the untracked half of a tracking comparison; Summary is
	// the tracked half, the copy the campaign really sends.
	Untracked *PlacementCounts `json:"untracked,omitempty"`
	// Domains and Providers group the placement by sending domain and sending
	// provider; Recipients groups it by recipient provider; Matrix crosses
	// domain with recipient provider.
	Domains    []PlacementBatchGroup     `json:"domains"`
	Providers  []PlacementBatchGroup     `json:"providers"`
	Recipients []PlacementFamilyCounts   `json:"recipients"`
	Matrix     []PlacementBatchMatrixRow `json:"matrix"`
	Content    ContentCheck              `json:"content"`
}

// PlacementBatchSender is one sender of a batch with where its copies landed.
type PlacementBatchSender struct {
	ID             string  `json:"id"`
	BatchID        string  `json:"batch_id"`
	EmailAccountID *string `json:"email_account_id"`
	SenderEmail    string  `json:"sender_email"`
	SenderDomain   string  `json:"sender_domain"`
	SenderFamily   string  `json:"sender_family"`
	// Status is one of the PlacementSender* values.
	Status        string     `json:"status"`
	Reason        string     `json:"reason,omitempty"`
	Detail        string     `json:"detail,omitempty"`
	Attempts      int        `json:"attempts"`
	NextAttemptAt time.Time  `json:"next_attempt_at"`
	StartedAt     *time.Time `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at"`

	SenderFamilyLabel string          `json:"sender_family_label"`
	Summary           PlacementCounts `json:"summary"`
	// TestIDs are the tests the sender ran.
	TestIDs []string `json:"test_ids"`
}

// PlacementBatchSendersParams filters and paginates a batch's senders. The
// page size defaults to 25 and is capped at 100.
type PlacementBatchSendersParams struct {
	ListOptions
	// Status keeps only senders in this PlacementSender* status.
	Status string
	// Query searches the sender address.
	Query string
	// Sort is "worst" (lowest inbox rate first, the default), "best", "email"
	// or "status".
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

// PlacementCoverage reports how much of the connected fleet delivered a
// placement test recently.
type PlacementCoverage struct {
	Mailboxes   int `json:"mailboxes"`
	Tested7d    int `json:"tested_7d"`
	Tested30d   int `json:"tested_30d"`
	NeverTested int `json:"never_tested"`
}

// PreviewBatch reports how many senders, tests, sends and credits a batch
// would come to. It starts nothing. Needs SEND_CAMPAIGNS.
func (s *PlacementService) PreviewBatch(ctx context.Context, params *PlacementBatchParams, opts ...RequestOption) (*PlacementBatchPreview, *Response, error) {
	return placementOne[PlacementBatchPreview](ctx, s.client.post, "placement/batches/preview", params, opts)
}

// StartBatch snapshots the senders and queues the batch; the backend then
// starts its senders a few at a time. Needs SEND_CAMPAIGNS. The API key must
// be allowed to use every sender mailbox: a restricted key only ever tests
// from its own.
func (s *PlacementService) StartBatch(ctx context.Context, params *PlacementBatchParams, opts ...RequestOption) (*PlacementBatch, *Response, error) {
	return placementOne[PlacementBatch](ctx, s.client.post, "placement/batches", params, opts)
}

// Batches returns a page of the workspace's batches, newest first. Needs
// READ_ANALYTICS.
func (s *PlacementService) Batches(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[PlacementBatch], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[PlacementBatch](ctx, s.client, "placement/batches", q, opts...)
}

// Batch retrieves one batch with its placement overall and grouped by sending
// domain, sending provider and recipient provider. Needs READ_ANALYTICS.
func (s *PlacementService) Batch(ctx context.Context, id string, opts ...RequestOption) (*PlacementBatchDetail, *Response, error) {
	var env placementData[*PlacementBatchDetail]
	resp, err := s.client.get(ctx, "placement/batches/"+url.PathEscape(id), &env, opts...)
	if err != nil {
		return nil, resp, err
	}
	return env.Data, resp, nil
}

// BatchSenders returns a page of a batch's senders with where each one's
// copies landed. Needs READ_ANALYTICS.
func (s *PlacementService) BatchSenders(ctx context.Context, id string, params *PlacementBatchSendersParams, opts ...RequestOption) (*Page[PlacementBatchSender], error) {
	return listJSON[PlacementBatchSender](ctx, s.client, "placement/batches/"+url.PathEscape(id)+"/senders", params.values(), opts...)
}

// CancelBatch stops a batch: no sender starts again and copies not sent yet
// are dropped, while copies already sent keep being classified. Needs
// SEND_CAMPAIGNS.
func (s *PlacementService) CancelBatch(ctx context.Context, id string, opts ...RequestOption) (*PlacementBatch, *Response, error) {
	return placementOne[PlacementBatch](ctx, s.client.post, "placement/batches/"+url.PathEscape(id)+"/cancel", nil, opts)
}

// Coverage reports how much of the workspace's connected fleet delivered a
// placement test in the last 7 and 30 days. Needs READ_ANALYTICS.
func (s *PlacementService) Coverage(ctx context.Context, opts ...RequestOption) (*PlacementCoverage, *Response, error) {
	var env placementData[*PlacementCoverage]
	resp, err := s.client.get(ctx, "placement/coverage", &env, opts...)
	if err != nil {
		return nil, resp, err
	}
	return env.Data, resp, nil
}

// --- the workspace's own seed inboxes ---

// PlacementWorkspaceSeed is one of the workspace's own mailboxes as a seed
// candidate.
type PlacementWorkspaceSeed struct {
	EmailAccountID string `json:"email_account_id"`
	Email          string `json:"email"`
	Family         string `json:"family"`
	Label          string `json:"label"`
	Status         string `json:"status"`
	// Seed reports whether the mailbox is currently a seed inbox.
	Seed bool `json:"seed"`
	// Blocker is why the mailbox cannot become a seed; empty when it can.
	Blocker string `json:"blocker,omitempty"`
}

// Seeds lists the workspace's mailboxes and which of them are its seed
// inboxes, limited to the mailboxes the API key may use. Needs READ_EMAILS.
func (s *PlacementService) Seeds(ctx context.Context, opts ...RequestOption) ([]PlacementWorkspaceSeed, *Response, error) {
	return fetchData[PlacementWorkspaceSeed](ctx, s.client, "placement/seeds", opts)
}

// SetSeed makes one of the workspace's mailboxes a seed inbox, or stops it
// being one. Idempotent. Needs WRITE_EMAILS.
func (s *PlacementService) SetSeed(ctx context.Context, emailID string, seed bool, opts ...RequestOption) (*PlacementWorkspaceSeed, *Response, error) {
	body := struct {
		Seed bool `json:"seed"`
	}{Seed: seed}
	return placementOne[PlacementWorkspaceSeed](ctx, s.client.put, "placement/seeds/"+url.PathEscape(emailID), body, opts)
}

// --- scheduled tests for a campaign ---

// PlacementMonitor re-tests a campaign's first step on a schedule.
type PlacementMonitor struct {
	ID           string     `json:"id"`
	CampaignID   string     `json:"campaign_id"`
	CreatedBy    *string    `json:"created_by"`
	Enabled      bool       `json:"enabled"`
	IntervalDays int        `json:"interval_days"`
	Panel        string     `json:"panel"`
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

// PlacementMonitorParams configures a campaign's scheduled placement test. A
// nil field keeps the stored value, or the default on a new monitor.
type PlacementMonitorParams struct {
	Enabled      *bool   `json:"enabled,omitempty"`
	IntervalDays *int    `json:"interval_days,omitempty"`
	Panel        *string `json:"panel,omitempty"`
	// AlertBelow is the inbox percentage under which an alert is raised.
	AlertBelow *int `json:"alert_below,omitempty"`
	// PauseOnAlert pauses the campaign when the alert fires.
	PauseOnAlert *bool `json:"pause_on_alert,omitempty"`
}

// Monitor returns a campaign's scheduled placement test, or nil when it has
// none. Needs READ_CAMPAIGNS.
func (s *PlacementService) Monitor(ctx context.Context, campaignID string, opts ...RequestOption) (*PlacementMonitor, *Response, error) {
	var env placementData[*PlacementMonitor]
	resp, err := s.client.get(ctx, "campaigns/"+url.PathEscape(campaignID)+"/placement-monitor", &env, opts...)
	if err != nil {
		return nil, resp, err
	}
	return env.Data, resp, nil
}

// SetMonitor creates or updates a campaign's scheduled placement test. A new
// or re-enabled monitor runs its first test within minutes. Repeating the
// same body lands on the same state. Needs SEND_CAMPAIGNS.
func (s *PlacementService) SetMonitor(ctx context.Context, campaignID string, params *PlacementMonitorParams, opts ...RequestOption) (*PlacementMonitor, *Response, error) {
	return placementOne[PlacementMonitor](ctx, s.client.put, "campaigns/"+url.PathEscape(campaignID)+"/placement-monitor", params, opts)
}

// DeleteMonitor removes a campaign's scheduled placement test. Needs
// SEND_CAMPAIGNS.
func (s *PlacementService) DeleteMonitor(ctx context.Context, campaignID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "campaigns/"+url.PathEscape(campaignID)+"/placement-monitor", opts...)
}
