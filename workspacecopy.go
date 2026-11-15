package warmbly

// Copying between workspaces: [CampaignService.CopyToWorkspaces] and
// [TemplateService.CopyToWorkspaces] copy configuration from the workspace the
// caller acts on into other workspaces the caller belongs to. Each destination
// is copied on its own, so one refused or failed destination does not undo the
// others; the answer reports every destination in the order it was asked for.

// Bounds on one copy request.
const (
	// WorkspaceCopyMaxDestinations is the most workspaces one request copies
	// into.
	WorkspaceCopyMaxDestinations = 10
	// WorkspaceCopyMaxTemplates is the most templates one request copies.
	WorkspaceCopyMaxTemplates = 50
)

// Outcomes of one destination, for [WorkspaceCopyResult.Status]. Other values
// may appear in later API versions.
const (
	// WorkspaceCopyCopied: everything asked for landed.
	WorkspaceCopyCopied = "copied"
	// WorkspaceCopyPartial: the copy landed but something was left out; see
	// [WorkspaceCopyResult.Warnings].
	WorkspaceCopyPartial = "partial"
	// WorkspaceCopyFailed: nothing landed in this destination; see
	// [WorkspaceCopyResult.Error].
	WorkspaceCopyFailed = "failed"
)

// CopyCampaignParams copies a campaign into other workspaces as a draft.
type CopyCampaignParams struct {
	// OrganizationIDs are the destination workspaces, 1 to
	// [WorkspaceCopyMaxDestinations]. The caller must be able to write
	// campaigns in each.
	OrganizationIDs []string `json:"organization_ids"`
	// TemplateIDs are reply templates of the source workspace that travel
	// along with the campaign. Sending any also needs the template read and
	// write scopes on an API key.
	TemplateIDs []string `json:"template_ids,omitempty"`
}

// CopyTemplatesParams copies reply templates into other workspaces.
type CopyTemplatesParams struct {
	// TemplateIDs are the source workspace's templates to copy, 1 to
	// [WorkspaceCopyMaxTemplates].
	TemplateIDs []string `json:"template_ids"`
	// OrganizationIDs are the destination workspaces, 1 to
	// [WorkspaceCopyMaxDestinations].
	OrganizationIDs []string `json:"organization_ids"`
}

// CopiedRef names one row a copy created in a destination workspace.
type CopiedRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CopyReview is a copied step whose settings pointed at something only the
// source workspace has (a tag, segment, pipeline, automation, teammate or
// form). Fields names those settings. The copy clears every one of them except
// a form link, which resolves to nothing in the destination until it is
// replaced.
type CopyReview struct {
	StepID   string   `json:"step_id"`
	StepName string   `json:"step_name"`
	Fields   []string `json:"fields"`
}

// CopyFailure is why a destination failed: a stable error code (for example
// "copy_destination_forbidden" or [ErrCodeStorageLimitReached]) and a
// message.
type CopyFailure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WorkspaceCopyResult is what one destination workspace received.
type WorkspaceCopyResult struct {
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name"`
	// Status is one of the WorkspaceCopy* constants.
	Status string `json:"status"`
	// Campaign is the draft created by a campaign copy; nil for a template
	// copy or a failed destination.
	Campaign *CopiedRef `json:"campaign,omitempty"`
	// Templates are the templates created in the destination.
	Templates []CopiedRef `json:"templates"`
	// Review lists copied steps whose settings need a look before launch.
	Review []CopyReview `json:"review"`
	// Warnings are human-readable notes on what was left out.
	Warnings []string `json:"warnings"`
	// Error is set when Status is [WorkspaceCopyFailed].
	Error *CopyFailure `json:"error,omitempty"`
}
