package warmbly

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

// FormService manages hosted lead-capture forms: their definition and theme,
// their submissions and funnel analytics, per-contact personalized links,
// uploaded brand assets and the workspace-wide custom forms domain.
//
// A form is built here but served elsewhere: the public page (/f/:public_id),
// its submit endpoint and the embed loader (/forms.js) live on the standalone
// forms host (FORMS_DOMAIN), take no authentication and are not part of this
// SDK. [Form.ShareURL] points at that host.
//
// Form routes take the contact scopes (read_contacts to view, write_contacts to
// change), since a form exists only to create and update contacts. The
// /forms/domain routes additionally need the manage_settings organization
// permission for session (JWT) callers.
type FormService service

// Form lifecycle values returned in [Form.Status]. Only a published form
// renders and accepts submissions; a draft or archived form's page answers
// not found, while its data is kept.
const (
	FormStatusDraft     = "draft"
	FormStatusPublished = "published"
	FormStatusArchived  = "archived"
)

// Block types for [FormField.Type]. Input types collect a value on submit;
// heading, paragraph, divider and page_break only render.
const (
	FormFieldTypeText       = "text"
	FormFieldTypeEmail      = "email"
	FormFieldTypePhone      = "phone"
	FormFieldTypeTextarea   = "textarea"
	FormFieldTypeNumber     = "number"
	FormFieldTypeSelect     = "select"
	FormFieldTypeRadio      = "radio"
	FormFieldTypeCheckboxes = "checkboxes"
	FormFieldTypeCheckbox   = "checkbox"
	FormFieldTypeDate       = "date"
	// FormFieldTypeHidden submits the constant in [FormField.Value] without
	// rendering anything; use it to tag which page or campaign a form sits on.
	FormFieldTypeHidden    = "hidden"
	FormFieldTypeHeading   = "heading"
	FormFieldTypeParagraph = "paragraph"
	FormFieldTypeDivider   = "divider"
	// FormFieldTypePageBreak starts a new page; its label is the page title
	// shown on the form and in the analytics funnel.
	FormFieldTypePageBreak = "page_break"
)

// Contact columns a field may fill through [FormField.MapTo]. At most one
// field may map to each column, and an email-type field always maps to
// [FormMapToEmail].
const (
	FormMapToFirstName = "first_name"
	FormMapToLastName  = "last_name"
	FormMapToEmail     = "email"
	FormMapToCompany   = "company"
	FormMapToPhone     = "phone"
)

// Column widths for [FormField.Width]. Two half-width fields share a row.
const (
	FormFieldWidthFull = "full"
	FormFieldWidthHalf = "half"
)

// Layouts for [FormDesign.Layout]: a centered card, fields directly on the
// page background, or a cover panel beside the form.
const (
	FormLayoutCard  = "card"
	FormLayoutWide  = "wide"
	FormLayoutSplit = "split"
)

// Modes for [FormDesign.Mode]: every field of a page at once, or one question
// per screen.
const (
	FormModeClassic = "classic"
	FormModeFocus   = "focus"
)

// Font families accepted in [FormDesign.FontFamily].
const (
	FormFontSystem       = "system"
	FormFontInter        = "inter"
	FormFontSerif        = "serif"
	FormFontMono         = "mono"
	FormFontManrope      = "manrope"
	FormFontSora         = "sora"
	FormFontFraunces     = "fraunces"
	FormFontSpaceGrotesk = "space-grotesk"
)

// Sizes shared by [FormDesign.ButtonSize] and [FormDesign.LogoSize].
const (
	FormSizeSmall  = "sm"
	FormSizeMedium = "md"
	FormSizeLarge  = "lg"
)

// Vertical rhythm values for [FormDesign.Spacing].
const (
	FormSpacingCompact = "compact"
	FormSpacingNormal  = "normal"
	FormSpacingRelaxed = "relaxed"
)

// Alignment values. [FormDesign.Align] takes left or center;
// [FormDesign.HeaderAlign] also takes between (logo one side, title the
// other).
const (
	FormAlignLeft    = "left"
	FormAlignCenter  = "center"
	FormAlignBetween = "between"
)

// Placements for [FormDesign.HeaderPlacement]: edge to edge above everything,
// or on the form surface above the fields.
const (
	FormHeaderPlacementPage   = "page"
	FormHeaderPlacementInline = "inline"
)

// Positions for [FormDesign.LogoPosition] on the card layout: on the card, or
// above it on the page background.
const (
	FormLogoPositionCard = "card"
	FormLogoPositionPage = "page"
)

// Fits for [FormDesign.BackgroundSize], how the uploaded background image
// covers the page.
const (
	FormBackgroundSizeCover   = "cover"
	FormBackgroundSizeContain = "contain"
	FormBackgroundSizeTile    = "tile"
)

// Asset kinds for [FormService.UploadAsset] and [FormService.DeleteAsset].
// Every asset must be a PNG or JPG. A logo is capped at 1 MB and 1024px on
// its longest side; a cover (the split layout's side panel) and a background
// (behind the whole page) at 4 MB and 2560px.
const (
	FormAssetLogo       = "logo"
	FormAssetCover      = "cover"
	FormAssetBackground = "background"
)

// Windows accepted by [FormService.Stats]. Funnel events are kept for 180
// days, so 90 days is the widest window the server offers.
const (
	FormStatsRange7Days  = "7d"
	FormStatsRange30Days = "30d"
	FormStatsRange90Days = "90d"
)

// Verification outcomes reported in [FormsDomainStatus.Status]. Only
// [FormsDomainStatusVerified] puts form links on the custom domain; every
// other state leaves them on the shared host, so a half-configured domain
// never breaks a form.
const (
	// FormsDomainStatusVerified means the CNAME resolves to the forms host.
	FormsDomainStatusVerified = "verified"
	// FormsDomainStatusPending is reported by [FormService.Domain] for a
	// stored domain that has not verified yet; it is a read of stored state,
	// not a DNS verdict.
	FormsDomainStatusPending = "pending"
	// FormsDomainStatusUnset means no custom domain is configured.
	FormsDomainStatusUnset = "unset"
	// FormsDomainStatusNoTarget means this install has no forms host, so
	// there is nothing to point a record at (an operator problem).
	FormsDomainStatusNoTarget = "no_target"
	// FormsDomainStatusNotFound means DNS returned no record for the domain.
	FormsDomainStatusNotFound = "not_found"
	// FormsDomainStatusWrongTarget means the record exists but points
	// somewhere other than [FormsDomainStatus.CNAMETarget]; compare
	// [FormsDomainStatus.Observed] to spot the typo.
	FormsDomainStatusWrongTarget = "wrong_target"
	// FormsDomainStatusLookupError means the lookup itself failed
	// (timeout, SERVFAIL); retry rather than treating it as misconfigured.
	FormsDomainStatusLookupError = "lookup_error"
)

// Form is a hosted lead-capture form: an ordered block list plus a theme,
// published at a public URL and embeddable on any site. A submission that
// carries an email creates or updates a contact, files it under CategoryIDs
// and, when CampaignID is set, enrolls it as a lead there.
type Form struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organization_id"`
	CreatedBy      *string `json:"created_by,omitempty"`
	// PublicID is the unguessable token in the hosted URL and the embed
	// codes. It survives a workspace export/import so installed embeds keep
	// working.
	PublicID string `json:"public_id"`
	// Name is the internal label shown in the dashboard, never to visitors.
	Name string `json:"name"`
	// Status is one of the FormStatus* constants.
	Status string `json:"status"`
	// Fields are the blocks in render order. A new form is seeded with first
	// name, last name and a required email.
	Fields []FormField `json:"fields"`
	Design FormDesign  `json:"design"`
	// SuccessMessage is shown after a submit unless RedirectURL is set.
	SuccessMessage string `json:"success_message"`
	// RedirectURL, when set, sends the visitor to your own thank-you page
	// instead of showing SuccessMessage.
	RedirectURL string `json:"redirect_url"`
	// CampaignID is the campaign new contacts are enrolled in on submit.
	// Sending still follows that campaign's own schedule and limits; a form
	// never causes immediate mail.
	CampaignID *string `json:"campaign_id,omitempty"`
	// CategoryIDs are the contact categories every submitted contact is
	// filed under.
	CategoryIDs []string `json:"category_ids"`
	// AllowedDomains restricts which sites may embed the form; a domain
	// covers its subdomains. Empty allows any site. The hosted link works
	// either way.
	AllowedDomains []string `json:"allowed_domains"`
	// CaptchaEnabled adds a Cloudflare Turnstile challenge. It only takes
	// effect when [FormsConfig.CaptchaAvailable] is true for the instance.
	CaptchaEnabled bool `json:"captcha_enabled"`

	// LogoURL, CoverURL and BackgroundURL are the public object URLs of the
	// uploaded brand assets; empty when none is set. Change them with
	// [FormService.UploadAsset] and [FormService.DeleteAsset], not Update.
	LogoURL       string `json:"logo_url"`
	CoverURL      string `json:"cover_url"`
	BackgroundURL string `json:"background_url"`

	// ViewsCount and SubmissionsCount are lifetime counters, kept forever.
	ViewsCount       int64      `json:"views_count"`
	SubmissionsCount int64      `json:"submissions_count"`
	LastSubmissionAt *time.Time `json:"last_submission_at,omitempty"`
	// PublishedAt is when the form last went from unpublished to published.
	PublishedAt *time.Time `json:"published_at,omitempty"`

	// StartsCount, IdentifiedCount and Trend are rollups over the last 14
	// days of funnel events. They are populated by [FormService.List] only;
	// other reads leave them zero and Trend nil. Trend has one entry per
	// day, oldest first.
	StartsCount     int64   `json:"starts_count"`
	IdentifiedCount int64   `json:"identified_count"`
	Trend           []int64 `json:"trend,omitempty"`

	// ShareURL is the hosted page URL, built on the workspace's verified
	// custom forms domain when it has one and the shared forms host
	// otherwise. Empty when the install has no forms host configured.
	ShareURL string `json:"share_url,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FormField is one block on a form.
type FormField struct {
	// ID is a builder-chosen slug (lowercase letters, digits, "_" and "-",
	// up to 40 characters), unique within the form and stable across edits.
	// Submissions key their answers by it.
	ID string `json:"id"`
	// Type is one of the FormFieldType* constants.
	Type string `json:"type"`
	// Label is required for every input type except hidden. For a page
	// break it is the page title.
	Label       string `json:"label"`
	Placeholder string `json:"placeholder,omitempty"`
	HelpText    string `json:"help_text,omitempty"`
	Required    bool   `json:"required"`
	// Options are the choices of a select, radio or checkboxes block (1 to
	// 50, each unique). Other types ignore them.
	Options []string `json:"options,omitempty"`
	// MapTo names the contact column the answer fills (a FormMapTo*
	// constant). Empty means the answer lands in the contact's custom fields
	// under the field's label.
	MapTo string `json:"map_to,omitempty"`
	// Value is the constant a hidden field submits, or the body text of a
	// paragraph block.
	Value string `json:"value,omitempty"`
	// Width is [FormFieldWidthFull] (the default) or [FormFieldWidthHalf].
	Width string `json:"width,omitempty"`
	// Rows is the visible height of a textarea (0 to 20; 0 uses the default).
	Rows int `json:"rows,omitempty"`
}

// FormDesign is the theme rendered around the fields. Every value is optional
// and an unset one falls back to the renderer's default, so a sparse design
// is normal. Colors are "#rrggbb" or "transparent".
type FormDesign struct {
	// FontFamily is one of the FormFont* constants.
	FontFamily       string `json:"font_family,omitempty"`
	PageBackground   string `json:"page_background,omitempty"`
	FormBackground   string `json:"form_background,omitempty"`
	TextColor        string `json:"text_color,omitempty"`
	LabelColor       string `json:"label_color,omitempty"`
	InputBackground  string `json:"input_background,omitempty"`
	InputBorderColor string `json:"input_border_color,omitempty"`
	InputTextColor   string `json:"input_text_color,omitempty"`
	PlaceholderColor string `json:"placeholder_color,omitempty"`
	AccentColor      string `json:"accent_color,omitempty"`
	ButtonBackground string `json:"button_background,omitempty"`
	ButtonTextColor  string `json:"button_text_color,omitempty"`
	// ButtonText is the submit button's caption.
	ButtonText string `json:"button_text,omitempty"`
	// ButtonSize is one of the FormSize* constants.
	ButtonSize      string `json:"button_size,omitempty"`
	ButtonFullWidth bool   `json:"button_full_width,omitempty"`
	// BorderRadius is clamped by the server to 0-24 pixels.
	BorderRadius *int `json:"border_radius,omitempty"`
	// MaxWidth is clamped by the server to 320-960 pixels (default 560).
	MaxWidth *int `json:"max_width,omitempty"`
	// Spacing is one of the FormSpacing* constants.
	Spacing string `json:"spacing,omitempty"`
	Shadow  *bool  `json:"shadow,omitempty"`
	// Theme records which preset seeded the current colors. Renderers never
	// read it, so any short lowercase slug is accepted.
	Theme string `json:"theme,omitempty"`
	// Layout is one of the FormLayout* constants.
	Layout string `json:"layout,omitempty"`
	// Mode is one of the FormMode* constants.
	Mode string `json:"mode,omitempty"`
	// PageBackgroundEnd turns the page background into a vertical gradient
	// from PageBackground to this color.
	PageBackgroundEnd string `json:"page_background_end,omitempty"`
	// Align is [FormAlignLeft] or [FormAlignCenter].
	Align string `json:"align,omitempty"`
	// ShowProgress shows a progress bar on multi-page forms.
	ShowProgress *bool `json:"show_progress,omitempty"`

	// BackgroundSize (a FormBackgroundSize* constant) and BackgroundOverlay
	// (0-100) style the uploaded background image: the overlay veils it with
	// the page color so text on top stays legible.
	BackgroundSize    string `json:"background_size,omitempty"`
	BackgroundOverlay *int   `json:"background_overlay,omitempty"`

	// The header is an optional bar carrying the logo and a title. A header
	// with neither is skipped on the live page. HeaderBackground and
	// HeaderSticky apply only to the page placement.
	HeaderEnabled    *bool  `json:"header_enabled,omitempty"`
	HeaderTitle      string `json:"header_title,omitempty"`
	HeaderBackground string `json:"header_background,omitempty"`
	// HeaderPlacement is one of the FormHeaderPlacement* constants.
	HeaderPlacement string `json:"header_placement,omitempty"`
	// HeaderAlign is [FormAlignLeft], [FormAlignCenter] or [FormAlignBetween].
	HeaderAlign    string `json:"header_align,omitempty"`
	HeaderSticky   *bool  `json:"header_sticky,omitempty"`
	HeaderShowLogo *bool  `json:"header_show_logo,omitempty"`

	// CoverTitle and CoverSubtitle are drawn over the split layout's cover
	// panel.
	CoverTitle    string `json:"cover_title,omitempty"`
	CoverSubtitle string `json:"cover_subtitle,omitempty"`

	// LogoSize is one of the FormSize* constants; LogoPosition one of the
	// FormLogoPosition* constants.
	LogoSize     string `json:"logo_size,omitempty"`
	LogoPosition string `json:"logo_position,omitempty"`
}

// FormCreateParams names a new form. It is created as a draft seeded with
// first name, last name and email fields and the default success message;
// everything else is set with [FormService.Update].
type FormCreateParams struct {
	// Name is required, at most 120 characters.
	Name string `json:"name"`
}

// FormUpdateParams is the PATCH payload for [FormService.Update]. Nil fields
// are left untouched. Fields, Design, CategoryIDs and AllowedDomains replace
// their whole value when set (send the complete list or theme, not a diff),
// so an empty slice clears the list.
type FormUpdateParams struct {
	Name *string `json:"name,omitempty"`
	// Status moves the form through its lifecycle (FormStatus* constants).
	// Publishing requires at least one non-hidden input field.
	Status *string `json:"status,omitempty"`
	// Fields replaces the block list. Field IDs must stay stable across
	// edits or existing submissions lose their labels.
	Fields *[]FormField `json:"fields,omitempty"`
	// Design replaces the whole theme; it is not merged with the stored one.
	Design *FormDesign `json:"design,omitempty"`
	// SuccessMessage is capped at 2000 characters; an empty string resets it
	// to the server default.
	SuccessMessage *string `json:"success_message,omitempty"`
	// RedirectURL must be an absolute http(s) URL; an empty string clears it.
	RedirectURL *string `json:"redirect_url,omitempty"`
	// CampaignID enrolls new contacts in a campaign on submit. To detach the
	// form from its campaign set ClearCampaign instead: a nil pointer means
	// "leave as is".
	CampaignID *string `json:"campaign_id,omitempty"`
	// ClearCampaign sends campaign_id as JSON null, detaching the campaign.
	// It wins over CampaignID when both are set.
	ClearCampaign  bool      `json:"-"`
	CategoryIDs    *[]string `json:"category_ids,omitempty"`
	AllowedDomains *[]string `json:"allowed_domains,omitempty"`
	CaptchaEnabled *bool     `json:"captcha_enabled,omitempty"`
}

// MarshalJSON emits campaign_id as an explicit null when ClearCampaign is set,
// which is how the server distinguishes "detach" from "unchanged".
func (p FormUpdateParams) MarshalJSON() ([]byte, error) {
	type plain FormUpdateParams
	if !p.ClearCampaign {
		return json.Marshal(plain(p))
	}
	p.CampaignID = nil
	// The outer field shadows the embedded omitempty one, so a nil pointer
	// here serializes as null rather than being dropped.
	return json.Marshal(struct {
		*plain
		CampaignID *string `json:"campaign_id"`
	}{plain: (*plain)(&p)})
}

// FormsConfig describes what this instance can do with forms, so a builder
// never offers a switch that cannot work.
type FormsConfig struct {
	// BaseURL is the origin of the standalone forms host, for example
	// "https://forms.example.com"; empty when FORMS_DOMAIN is not configured,
	// in which case no form can be served.
	BaseURL string `json:"base_url"`
	// CaptchaAvailable reports whether the operator configured a captcha
	// provider, which [Form.CaptchaEnabled] needs to take effect.
	CaptchaAvailable bool `json:"captcha_available"`
}

// FormSubmission is one public submit, kept verbatim.
type FormSubmission struct {
	ID             string `json:"id"`
	FormID         string `json:"form_id"`
	OrganizationID string `json:"organization_id"`
	// ContactID is the contact the submission created or updated, or nil
	// when the form has no email field.
	ContactID *string `json:"contact_id,omitempty"`
	// CampaignID is the campaign whose email carried the personalized link
	// the visitor arrived through, when there was one.
	CampaignID *string `json:"campaign_id,omitempty"`
	// Data holds every answer keyed by [FormField.ID]. A checkboxes block
	// decodes as a []any of strings; every other input as a string.
	Data map[string]any `json:"data"`
	// SourceURL is the page the form was submitted from (the host page for
	// an embed, the hosted page otherwise).
	SourceURL string    `json:"source_url"`
	CreatedAt time.Time `json:"created_at"`

	// ContactEmail, ContactName and CampaignName are display summaries of
	// the linked records, resolved at read time; empty when unlinked.
	ContactEmail string `json:"contact_email,omitempty"`
	ContactName  string `json:"contact_name,omitempty"`
	CampaignName string `json:"campaign_name,omitempty"`
}

// FormSubmissionListParams control [FormService.ListSubmissions]. Submissions
// are keyset-paginated newest first on created_at rather than by opaque
// cursor.
type FormSubmissionListParams struct {
	// Limit is the page size, 1 to 100. Zero uses the server default of 50.
	Limit int
	// Before returns only submissions created strictly before this instant.
	// Pass the previous page's last CreatedAt to walk backwards in time;
	// [Page.Next] and [Page.All] do this for you.
	Before *time.Time
}

func (p *FormSubmissionListParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	setPositive(q, "limit", p.Limit)
	if p.Before != nil && !p.Before.IsZero() {
		q.Set("before", formCursor(*p.Before))
	}
	return q
}

// formCursor formats a submission timestamp with full precision, so a page
// boundary inside a second neither skips nor repeats rows.
func formCursor(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// FormStats is the analytics payload for one form over a window: funnel
// totals, a daily series, per-page drop-off, traffic breakdowns and the
// contacts identified through personalized links. Location is resolved to
// country only; visitor IPs are never stored.
type FormStats struct {
	Totals FormStatsTotals `json:"totals"`
	// Daily has one entry per day of the window, oldest first.
	Daily []FormStatsDay `json:"daily"`
	// Pages is the page funnel; a single-page form has one row.
	Pages []FormFunnelPage `json:"pages"`
	// Sources, Countries, Devices and Campaigns are the top buckets (at most
	// eight each) keyed by referrer domain, ISO country code, device class
	// (desktop, mobile, tablet or unknown) and campaign name.
	Sources   []FormStatsBucket `json:"sources"`
	Countries []FormStatsBucket `json:"countries"`
	Devices   []FormStatsBucket `json:"devices"`
	Campaigns []FormStatsBucket `json:"campaigns"`
	// Identified lists the most recent contacts (up to 25) who opened the
	// form through a personalized link.
	Identified []FormIdentifiedVisitor `json:"identified"`
}

// FormStatsTotals are the funnel counts for the whole window.
type FormStatsTotals struct {
	Views int64 `json:"views"`
	// Starts counts visitors who interacted with a field.
	Starts      int64 `json:"starts"`
	Submissions int64 `json:"submissions"`
	// CompletionRate is submissions over views, 0 to 1.
	CompletionRate float64 `json:"completion_rate"`
	// IdentifiedVisitors counts distinct contacts seen through a
	// personalized link.
	IdentifiedVisitors int64 `json:"identified_visitors"`
}

// FormStatsDay is one day of the funnel series.
type FormStatsDay struct {
	// Date is the calendar day as YYYY-MM-DD.
	Date        string `json:"date"`
	Views       int64  `json:"views"`
	Starts      int64  `json:"starts"`
	Submissions int64  `json:"submissions"`
}

// FormFunnelPage is one row of the page funnel: how many visitors reached the
// page, and how many of those went on to submit.
type FormFunnelPage struct {
	// PageIndex is zero-based.
	PageIndex int `json:"page_index"`
	// Title is the page-break label, or a generated one when unset.
	Title         string `json:"title"`
	Reached       int64  `json:"reached"`
	CompletedFrom int64  `json:"completed_from"`
}

// FormStatsBucket is one row of a breakdown.
type FormStatsBucket struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

// FormIdentifiedVisitor is a contact who opened the form through a
// personalized link, and how far they got.
type FormIdentifiedVisitor struct {
	ContactID string    `json:"contact_id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	LastSeen  time.Time `json:"last_seen"`
	// FurthestPage is the zero-based index of the last page reached.
	FurthestPage int  `json:"furthest_page"`
	Completed    bool `json:"completed"`
	// Campaign names the campaign whose email brought this contact here,
	// when there was one.
	Campaign string `json:"campaign,omitempty"`
}

// FormLink is a personalized form URL for one contact: the hosted page URL
// with a per-contact ticket (?t=...). Opening it pre-fills the contact's
// mapped fields and attributes the visit and any submission to that contact
// even if they type a different email. Holding the link is the identity, so
// treat it like an unsubscribe link.
type FormLink struct {
	URL string `json:"url"`
}

// FormsDomainStatus is the state of the workspace's custom forms domain,
// shaped like [TrackingDomainStatus]. Only a verified domain is used to build
// form URLs; until then every link stays on the shared forms host.
type FormsDomainStatus struct {
	// FormsDomain is the stored custom host, for example "forms.acme.com";
	// empty when none is set.
	FormsDomain           string     `json:"forms_domain"`
	FormsDomainVerified   bool       `json:"forms_domain_verified"`
	FormsDomainVerifiedAt *time.Time `json:"forms_domain_verified_at"`
	// CNAMETarget is the value to put in the CNAME record: this install's
	// shared forms host. Empty means the install has none, so nothing can
	// verify.
	CNAMETarget string `json:"cname_target"`
	// Status is one of the FormsDomainStatus* constants.
	Status string `json:"status"`
	// Message is a human-readable explanation of Status.
	Message string `json:"message"`
	// Observed is what DNS actually returned for the domain, so a typo can
	// be spotted by comparing it with CNAMETarget.
	Observed string `json:"observed,omitempty"`
	// FormsHostUnresolvable reports that the install's own forms host does
	// not resolve, an operator problem rather than the customer's record.
	FormsHostUnresolvable bool `json:"forms_host_unresolvable"`
}

// List returns every form in the workspace. There is no pagination or
// filtering; a workspace holds at most 100 forms. This is the only read that
// fills the 14-day rollups ([Form.StartsCount], [Form.IdentifiedCount] and
// [Form.Trend]).
func (s *FormService) List(ctx context.Context, opts ...RequestOption) ([]Form, *Response, error) {
	return fetchData[Form](ctx, s.client, "forms", opts)
}

// Config reports the instance's forms deployment: the public forms origin and
// whether a captcha provider is available.
func (s *FormService) Config(ctx context.Context, opts ...RequestOption) (*FormsConfig, *Response, error) {
	return fetch[FormsConfig](ctx, s.client, "forms/config", opts)
}

// Create makes a new draft form seeded with first name, last name and email
// fields. Configure and publish it with [FormService.Update].
func (s *FormService) Create(ctx context.Context, params *FormCreateParams, opts ...RequestOption) (*Form, *Response, error) {
	return send[Form](ctx, s.client.post, "forms", params, opts)
}

// Get retrieves a single form by ID.
func (s *FormService) Get(ctx context.Context, id string, opts ...RequestOption) (*Form, *Response, error) {
	return fetch[Form](ctx, s.client, "forms/"+url.PathEscape(id), opts)
}

// Update modifies a form. Edits to a published form go live immediately.
// Moving Status to published stamps [Form.PublishedAt] and requires at least
// one non-hidden input field.
func (s *FormService) Update(ctx context.Context, id string, params *FormUpdateParams, opts ...RequestOption) (*Form, *Response, error) {
	return send[Form](ctx, s.client.patch, "forms/"+url.PathEscape(id), params, opts)
}

// Delete permanently removes a form together with its submissions, link
// tickets and funnel events. Contacts it created are kept. To take a form
// offline without losing anything, set its status to [FormStatusArchived]
// instead.
func (s *FormService) Delete(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "forms/"+url.PathEscape(id), opts...)
}

// ListSubmissions returns a page of the form's submissions, newest first.
// Paging is by timestamp rather than opaque cursor: the returned page's
// NextCursor is the RFC 3339 created_at of its last row, and [Page.Next] and
// [Page.All] pass it back as the before parameter. [Pagination.Total] is
// never set; use [Form.SubmissionsCount] for the lifetime count.
func (s *FormService) ListSubmissions(ctx context.Context, id string, params *FormSubmissionListParams, opts ...RequestOption) (*Page[FormSubmission], error) {
	return s.submissionsPage(ctx, "forms/"+url.PathEscape(id)+"/submissions", params.values(), opts)
}

// submissionsPage adapts the endpoint's {"data": [...], "has_more": bool}
// shape onto [Page] so the usual Next/All helpers work, deriving the cursor
// from the last row's timestamp.
func (s *FormService) submissionsPage(ctx context.Context, path string, query url.Values, opts []RequestOption) (*Page[FormSubmission], error) {
	var wire struct {
		Data    []FormSubmission `json:"data"`
		HasMore bool             `json:"has_more"`
	}
	resp, err := s.client.get(ctx, withQuery(path, query), &wire, opts...)
	if err != nil {
		return nil, err
	}
	page := &Page[FormSubmission]{Data: wire.Data, resp: resp}
	if page.Data == nil {
		page.Data = []FormSubmission{}
	}
	page.Pagination.HasMore = wire.HasMore
	if wire.HasMore && len(wire.Data) > 0 {
		cursor := formCursor(wire.Data[len(wire.Data)-1].CreatedAt)
		page.Pagination.NextCursor = &cursor
	}
	page.fetch = func(ctx context.Context, cursor string) (*Page[FormSubmission], error) {
		q := cloneValues(query)
		q.Set("before", cursor)
		return s.submissionsPage(ctx, path, q, opts)
	}
	return page, nil
}

// DeleteSubmission removes one submission. The contact it created or updated
// is kept.
func (s *FormService) DeleteSubmission(ctx context.Context, id, submissionID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "forms/"+url.PathEscape(id)+"/submissions/"+url.PathEscape(submissionID), opts...)
}

// Stats returns the form's funnel analytics over a trailing window. window is
// one of the FormStatsRange* constants; empty means [FormStatsRange30Days].
func (s *FormService) Stats(ctx context.Context, id, window string, opts ...RequestOption) (*FormStats, *Response, error) {
	q := make(url.Values)
	setNonEmpty(q, "range", window)
	return fetch[FormStats](ctx, s.client, withQuery("forms/"+url.PathEscape(id)+"/stats", q), opts)
}

// MintLink returns the personalized URL of the form for one contact. It is a
// GET on top of an upsert: the first call creates the contact's ticket and
// every later call returns the same one, so retries are safe. It needs the
// contact write scope, since minting writes a link row. It fails when the
// install has no forms host configured.
func (s *FormService) MintLink(ctx context.Context, id, contactID string, opts ...RequestOption) (*FormLink, *Response, error) {
	return fetch[FormLink](ctx, s.client, "forms/"+url.PathEscape(id)+"/links/"+url.PathEscape(contactID), opts)
}

// UploadAsset uploads a brand image and returns the updated form. kind is one
// of the FormAsset* constants; see them for the size caps. The file must be a
// PNG or JPG, sent as the multipart field "file". Uploading over an existing
// asset replaces it.
func (s *FormService) UploadAsset(ctx context.Context, id, kind string, file *FileUpload, opts ...RequestOption) (*Form, *Response, error) {
	out := new(Form)
	resp, err := s.client.postMultipart(ctx, "forms/"+url.PathEscape(id)+"/assets/"+url.PathEscape(kind), "file", file, nil, out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// DeleteAsset removes a brand image and returns the updated form. kind is one
// of the FormAsset* constants. Removing an asset that is not set succeeds.
func (s *FormService) DeleteAsset(ctx context.Context, id, kind string, opts ...RequestOption) (*Form, *Response, error) {
	out := new(Form)
	resp, err := s.client.call(ctx, http.MethodDelete, "forms/"+url.PathEscape(id)+"/assets/"+url.PathEscape(kind), nil, out, opts)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// Domain reads the stored state of the workspace's custom forms domain
// without doing any DNS work. A stored but unverified domain reports
// [FormsDomainStatusPending]; use [FormService.VerifyDomain] for a live
// verdict.
func (s *FormService) Domain(ctx context.Context, opts ...RequestOption) (*FormsDomainStatus, *Response, error) {
	return fetch[FormsDomainStatus](ctx, s.client, "forms/domain", opts)
}

// SetDomain stores a custom forms domain (a subdomain of the domain you send
// from, for example "forms.acme.com") and immediately resolves it, so saving
// and verifying are one step. An empty domain clears it back to the shared
// host. An unresolved record is not an error: the domain stays stored and
// unverified with a Status explaining why, and links keep using the shared
// host until it verifies. The server re-checks hourly on its own.
//
// This is a workspace-wide setting; session callers need manage_settings.
func (s *FormService) SetDomain(ctx context.Context, domain string, opts ...RequestOption) (*FormsDomainStatus, *Response, error) {
	body := struct {
		FormsDomain string `json:"forms_domain"`
	}{FormsDomain: domain}
	return send[FormsDomainStatus](ctx, s.client.put, "forms/domain", body, opts)
}

// VerifyDomain re-resolves the stored custom forms domain and records the
// verdict. Call it after a DNS change instead of waiting for the hourly
// re-check. Session callers need manage_settings.
func (s *FormService) VerifyDomain(ctx context.Context, opts ...RequestOption) (*FormsDomainStatus, *Response, error) {
	return send[FormsDomainStatus](ctx, s.client.post, "forms/domain/verify", nil, opts)
}
