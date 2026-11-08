package warmbly

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// LeadService is the lead finder: businesses found on a map become leads, are
// enriched from their websites, and are then turned into contacts.
//
// Do not confuse it with [LeadSyncService], which syncs leads from Google
// Sheets, nor with the per-campaign "leads" (a contact enrolled in a campaign)
// in [CampaignService].
//
// Leads are prospect contacts, so the contact scopes gate them: reads (query,
// points, facets, export, config, geocode, enrichment progress and saved
// searches) take the contact read scope, searching and single-lead edits take
// the contact write scope, and the bulk operations (bulk update, bulk delete,
// enrich, add to contacts) take the bulk contact scope. A lead search or an
// enrichment run can spend workspace credits on instances that meter them; the
// results say how much was charged.
type LeadService service

// Lead providers for [LeadSearchParams.Provider] and [Lead.Provider]. Which
// ones can run depends on the workspace; see [LeadService.Config].
const (
	// LeadProviderOSM is OpenStreetMap through an Overpass endpoint. Free.
	LeadProviderOSM = "osm"
	// LeadProviderGooglePlaces is the Google Places API.
	LeadProviderGooglePlaces = "google_places"
	// LeadProviderFoursquare is the Foursquare Places API.
	LeadProviderFoursquare = "foursquare"
	// LeadProviderOutscraper is Outscraper's Google Maps search.
	LeadProviderOutscraper = "outscraper"
)

// Lead statuses for [Lead.Status] and [LeadPatch.Status]: where the lead
// stands in someone's prospecting.
const (
	LeadFinderStatusNew       = "new"
	LeadFinderStatusSaved     = "saved"
	LeadFinderStatusContacted = "contacted"
	LeadFinderStatusDismissed = "dismissed"
)

// Enrichment states for [Lead.EnrichmentStatus]: how far reading the lead's
// website got.
const (
	LeadEnrichmentNone      = "none"
	LeadEnrichmentQueued    = "queued"
	LeadEnrichmentDone      = "done"
	LeadEnrichmentNoWebsite = "no_website"
	LeadEnrichmentFailed    = "failed"
)

// Enrichment run states for [LeadEnrichmentRun.Status].
const (
	LeadRunQueued  = "queued"
	LeadRunRunning = "running"
	LeadRunDone    = "done"
	LeadRunError   = "error"
)

// Sort orders for [LeadQueryParams.Sort].
const (
	// LeadSortRecent is most recently seen first. The default.
	LeadSortRecent  = "recent"
	LeadSortScore   = "score"
	LeadSortRating  = "rating"
	LeadSortReviews = "reviews"
	LeadSortName    = "name"
)

// Search area shapes for [LeadArea.Kind].
const (
	LeadAreaBBox    = "bbox"
	LeadAreaCircle  = "circle"
	LeadAreaPolygon = "polygon"
)

// How a person's seniority is placed, for [LeadPerson.Seniority].
const (
	LeadSeniorityOwner     = "owner"
	LeadSeniorityExecutive = "executive"
	LeadSeniorityManager   = "manager"
	LeadSeniorityStaff     = "staff"
)

// Where a person was found, for [LeadPerson.Source].
const (
	// LeadPersonSchema is structured data on the website.
	LeadPersonSchema = "schema"
	// LeadPersonPage is a name and role read off a team or about page.
	LeadPersonPage = "page"
	// LeadPersonImprint is a legal notice naming the owner.
	LeadPersonImprint = "imprint"
	// LeadPersonEmail is a personal address on the site whose owner was named.
	LeadPersonEmail = "email"
	// LeadPersonLinkedIn is a LinkedIn profile linked from the website.
	LeadPersonLinkedIn = "linkedin"
	// LeadPersonManual is someone a teammate typed in.
	LeadPersonManual = "manual"
)

// How a person's address was arrived at, for [LeadPerson.EmailSource]. The
// empty string means no address.
const (
	// LeadEmailWebsite was published on the business's own site.
	LeadEmailWebsite = "website"
	// LeadEmailPattern was built from the name and the domain's address pattern.
	LeadEmailPattern = "pattern"
	// LeadEmailManual was typed in by a teammate.
	LeadEmailManual = "manual"
)

// LeadPersonStatusGuessed is the [LeadPerson.EmailStatus] of a pattern-built
// address that nothing could confirm.
const LeadPersonStatusGuessed = "guessed"

// Who to turn into contacts, for [LeadAddToContactsParams.Who].
const (
	// LeadWhoBusiness uses each lead's own address. The default.
	LeadWhoBusiness = "business"
	// LeadWhoPeople uses the people found at each lead.
	LeadWhoPeople = "people"
	// LeadWhoBoth uses both.
	LeadWhoBoth = "both"
)

// Export row layouts for [LeadExportParams.Rows].
const (
	// LeadRowsLeads is one row per business. The default.
	LeadRowsLeads = "leads"
	// LeadRowsPeople is one row per person, with the business columns repeated.
	LeadRowsPeople = "people"
)

// Error identifiers carried in [Error.Code] by lead finder calls.
const (
	// ErrCodeLeadSearchExhausted is the 400 returned when a search is asked
	// for another page after its last.
	ErrCodeLeadSearchExhausted = "lead_search_exhausted"
	// ErrCodeLeadSelectionTooLarge is the 400 returned when more than 5,000
	// lead ids are named at once.
	ErrCodeLeadSelectionTooLarge = "lead_selection_too_large"
)

// LatLng is one point on the map.
type LatLng struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// LeadBounds is a south-west to north-east rectangle.
type LeadBounds struct {
	South float64 `json:"south"`
	West  float64 `json:"west"`
	North float64 `json:"north"`
	East  float64 `json:"east"`
}

// LeadArea is the part of the map a search covers. Bounds is always required:
// for a circle or polygon it is the shape's bounding box.
type LeadArea struct {
	// Kind is [LeadAreaBBox], [LeadAreaCircle] or [LeadAreaPolygon].
	Kind   string     `json:"kind"`
	Bounds LeadBounds `json:"bounds"`
	// Center and RadiusM describe a circle.
	Center  *LatLng `json:"center,omitempty"`
	RadiusM float64 `json:"radius_m,omitempty"`
	// Polygon is the vertices of a polygon.
	Polygon []LatLng `json:"polygon,omitempty"`
}

// LeadEmail is one address found for a lead and what checking it said.
type LeadEmail struct {
	Email string `json:"email"`
	// Source is "website" when read off the business's site, else the provider.
	Source      string `json:"source"`
	URL         string `json:"url,omitempty"`
	DomainMatch bool   `json:"domain_match"`
	// Status is the verification verdict, when the address was checked.
	Status    string `json:"status,omitempty"`
	SubStatus string `json:"sub_status,omitempty"`
}

// LeadSite is what reading a lead's website found besides addresses.
type LeadSite struct {
	Reachable   bool     `json:"reachable"`
	FinalURL    string   `json:"final_url,omitempty"`
	HTTPS       bool     `json:"https"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Platform    string   `json:"platform,omitempty"`
	Signals     []string `json:"signals,omitempty"`
	Phones      []string `json:"phones,omitempty"`
	Pages       int      `json:"pages,omitempty"`
	// EmailPattern is how the domain builds a person's address, for example
	// "first.last".
	EmailPattern string `json:"email_pattern,omitempty"`
	// AcceptsAll is set when the domain's mail server accepts any address, so
	// a guessed address cannot be confirmed.
	AcceptsAll bool `json:"accepts_all,omitempty"`
	// FoundWebsite is set when the listing had no website and one was worked
	// out from the business's name.
	FoundWebsite bool       `json:"found_website,omitempty"`
	Error        string     `json:"error,omitempty"`
	CheckedAt    *time.Time `json:"checked_at,omitempty"`
}

// Lead is one business in the workspace's lead finder.
type Lead struct {
	ID string `json:"id"`
	// Provider is where the record came from; see the LeadProvider* constants.
	Provider   string   `json:"provider"`
	ExternalID string   `json:"external_id"`
	Name       string   `json:"name"`
	Category   string   `json:"category"`
	Categories []string `json:"categories"`
	Address    string   `json:"address"`
	Street     string   `json:"street"`
	City       string   `json:"city"`
	Region     string   `json:"region"`
	Postcode   string   `json:"postcode"`
	Country    string   `json:"country"`
	Latitude   float64  `json:"latitude"`
	Longitude  float64  `json:"longitude"`
	Phone      string   `json:"phone"`
	Website    string   `json:"website"`
	Domain     string   `json:"domain"`
	// Email is the best address found; Emails lists every one.
	Email  string      `json:"email"`
	Emails []LeadEmail `json:"emails"`
	// Socials maps a network ("facebook", "instagram", "linkedin", "twitter",
	// "tiktok", "youtube") to its URL.
	Socials        map[string]string `json:"socials"`
	Rating         *float64          `json:"rating"`
	ReviewCount    *int              `json:"review_count"`
	PriceLevel     *int              `json:"price_level"`
	Hours          []string          `json:"hours"`
	BusinessStatus string            `json:"business_status"`
	MapsURL        string            `json:"maps_url"`
	Description    string            `json:"description"`
	// Chain names the chain the business belongs to, or "".
	Chain string   `json:"chain"`
	Site  LeadSite `json:"site"`
	// Score ranks the lead as a prospect.
	Score int `json:"score"`
	// Status is one of the LeadFinderStatus* constants.
	Status string `json:"status"`
	// EnrichmentStatus is one of the LeadEnrichment* constants.
	EnrichmentStatus string     `json:"enrichment_status"`
	EnrichedAt       *time.Time `json:"enriched_at"`
	// ContactID is the contact this lead was added as, once it has been.
	ContactID   *string  `json:"contact_id"`
	Tags        []string `json:"tags"`
	Notes       string   `json:"notes"`
	PeopleCount int      `json:"people_count"`
	// People is filled only where one lead is read in full ([LeadService.Get]).
	People      []LeadPerson `json:"people,omitempty"`
	FirstSeenAt time.Time    `json:"first_seen_at"`
	LastSeenAt  time.Time    `json:"last_seen_at"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// LeadPerson is one person who works at a lead's business.
type LeadPerson struct {
	ID        string `json:"id"`
	LeadID    string `json:"lead_id"`
	Name      string `json:"name"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Title     string `json:"title"`
	// Seniority is one of the LeadSeniority* constants.
	Seniority string `json:"seniority"`
	Email     string `json:"email"`
	// EmailSource is one of the LeadEmail* source constants, or "" without
	// an address.
	EmailSource string `json:"email_source"`
	// EmailStatus is the verification verdict, or [LeadPersonStatusGuessed].
	EmailStatus string `json:"email_status"`
	// EmailConfidence is how sure the finder is of the address, 0 to 100.
	EmailConfidence int    `json:"email_confidence"`
	LinkedInURL     string `json:"linkedin_url"`
	Phone           string `json:"phone"`
	// Source is one of the LeadPerson* source constants.
	Source    string    `json:"source"`
	SourceURL string    `json:"source_url"`
	ContactID *string   `json:"contact_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LeadSearch is one search run on the lead finder map.
type LeadSearch struct {
	ID string `json:"id"`
	// CreatedByUserID is who ran the search, when known.
	CreatedByUserID *string `json:"created_by_user_id"`
	Name            string  `json:"name"`
	// Provider is one of the LeadProvider* constants.
	Provider      string   `json:"provider"`
	Query         string   `json:"query"`
	Category      string   `json:"category"`
	LocationLabel string   `json:"location_label"`
	Area          LeadArea `json:"area"`
	Saved         bool     `json:"saved"`
	ResultCount   int      `json:"result_count"`
	PagesFetched  int      `json:"pages_fetched"`
	// HasMore reports whether another page can be fetched by repeating the
	// search with this search's ID.
	HasMore   bool      `json:"has_more"`
	LastRunAt time.Time `json:"last_run_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LeadEnrichmentRun is one background pass reading leads' websites.
type LeadEnrichmentRun struct {
	ID string `json:"id"`
	// RequestedByUserID is who started the run, when known.
	RequestedByUserID *string `json:"requested_by_user_id"`
	// Verify and People echo what the run was asked to do.
	Verify bool `json:"verify"`
	People bool `json:"people"`
	// Status is one of the LeadRun* constants.
	Status      string `json:"status"`
	Total       int    `json:"total"`
	Processed   int    `json:"processed"`
	EmailsFound int    `json:"emails_found"`
	PeopleFound int    `json:"people_found"`
	NoWebsite   int    `json:"no_website"`
	Failed      int    `json:"failed"`
	// ChargedUSD is what the run cost the workspace.
	ChargedUSD float64    `json:"charged_usd"`
	LastError  string     `json:"last_error"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// Done reports whether the run has finished, successfully or not.
func (r *LeadEnrichmentRun) Done() bool {
	return r.Status == LeadRunDone || r.Status == LeadRunError
}

// LeadFilter narrows a workspace's leads. Every field is optional.
type LeadFilter struct {
	// SearchID limits to the leads one search found.
	SearchID string `json:"search_id,omitempty"`
	// Q is a free-text search.
	Q string `json:"q,omitempty"`
	// Statuses are LeadFinderStatus* values.
	Statuses []string `json:"statuses,omitempty"`
	// Providers are LeadProvider* values.
	Providers []string `json:"providers,omitempty"`
	Category  string   `json:"category,omitempty"`
	Tag       string   `json:"tag,omitempty"`
	// The Has* and InContacts filters match on presence when true and on
	// absence when false; nil does not filter.
	HasEmail   *bool `json:"has_email,omitempty"`
	HasWebsite *bool `json:"has_website,omitempty"`
	HasPhone   *bool `json:"has_phone,omitempty"`
	HasSocials *bool `json:"has_socials,omitempty"`
	HasPeople  *bool `json:"has_people,omitempty"`
	InContacts *bool `json:"in_contacts,omitempty"`
	// Enrichment are LeadEnrichment* values.
	Enrichment []string `json:"enrichment,omitempty"`
	MinRating  *float64 `json:"min_rating,omitempty"`
	MinReviews *int     `json:"min_reviews,omitempty"`
	MaxReviews *int     `json:"max_reviews,omitempty"`
	MinScore   *int     `json:"min_score,omitempty"`
	// Bounds limits to leads inside a map rectangle.
	Bounds *LeadBounds `json:"bounds,omitempty"`
	// ExcludeChains drops leads that belong to a chain.
	ExcludeChains bool `json:"exclude_chains,omitempty"`
}

// LeadSelection names a set of leads for a bulk call: explicit ids (at most
// 5,000), or every lead matching Filter when All is set.
type LeadSelection struct {
	LeadIDs []string `json:"lead_ids,omitempty"`
	// All selects every lead matching Filter (or every lead when Filter is
	// nil), capped at 5,000.
	All    bool        `json:"all,omitempty"`
	Filter *LeadFilter `json:"filter,omitempty"`
}

// LeadPatch is the editable part of a lead. Nil and empty fields are left
// alone.
type LeadPatch struct {
	// Status is one of the LeadFinderStatus* constants.
	Status *string `json:"status,omitempty"`
	// Notes is limited to 5,000 characters.
	Notes *string `json:"notes,omitempty"`
	// Tags replaces the lead's tags; a pointer to an empty slice clears them.
	Tags *[]string `json:"tags,omitempty"`
	// AddTags and RemoveTags change the tag set without replacing it.
	AddTags    []string `json:"add_tags,omitempty"`
	RemoveTags []string `json:"remove_tags,omitempty"`
}

// LeadProviderInfo describes a lead provider to the caller.
type LeadProviderInfo struct {
	// ID is one of the LeadProvider* constants.
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Available is true when a search can run: the provider is free or a key
	// is configured.
	Available bool `json:"available"`
	// KeySource is "none", "workspace" or "instance".
	KeySource string `json:"key_source"`
	NeedsKey  bool   `json:"needs_key"`
	PageSize  int    `json:"page_size"`
	MaxPages  int    `json:"max_pages"`
	// CostUSD is the price of one page, on metered instances.
	CostUSD     float64  `json:"cost_usd"`
	Fields      []string `json:"fields"`
	Integration string   `json:"integration,omitempty"`
}

// LeadCategory is a preset kind of business, usable as
// [LeadSearchParams.Category].
type LeadCategory struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Group string `json:"group"`
}

// LeadExportColumn is one column a lead export can carry; Key goes in
// [LeadExportParams.Columns].
type LeadExportColumn struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// LeadMapStyle is the vector map style a map client should draw.
type LeadMapStyle struct {
	StyleLight string `json:"style_light"`
	StyleDark  string `json:"style_dark"`
}

// LeadConfig is what a client needs before its first search.
type LeadConfig struct {
	Providers       []LeadProviderInfo `json:"providers"`
	Categories      []LeadCategory     `json:"categories"`
	DefaultProvider string             `json:"default_provider"`
	// EnrichCostUSD is the price of reading one lead's website, and
	// PersonEmailCostUSD of working out one person's address; both are zero
	// on instances that do not meter.
	EnrichCostUSD      float64 `json:"enrich_cost_usd"`
	PersonEmailCostUSD float64 `json:"person_email_cost_usd"`
	// ExportColumns are the business columns an export can carry and
	// PersonColumns the extra ones of a one-row-per-person export.
	ExportColumns        []LeadExportColumn `json:"export_columns"`
	PersonColumns        []LeadExportColumn `json:"person_columns"`
	DefaultExportColumns []string           `json:"default_export_columns"`
	Map                  LeadMapStyle       `json:"map"`
}

// LeadGeocodeResult is one place a location search matched.
type LeadGeocodeResult struct {
	Label  string `json:"label"`
	Detail string `json:"detail"`
	// Kind is the kind of place, for example "city".
	Kind   string      `json:"kind"`
	Center LatLng      `json:"center"`
	Bounds *LeadBounds `json:"bounds,omitempty"`
}

// LeadGeocodeParams looks up a typed location.
type LeadGeocodeParams struct {
	// Query is the place to look up, up to 200 characters.
	Query string
	// Near biases results toward a point; both coordinates are sent together.
	Near *LatLng
	// Language is a language tag for the labels, for example "de".
	Language string
}

func (p *LeadGeocodeParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	setNonEmpty(q, "q", p.Query)
	if p.Near != nil {
		q.Set("lat", strconv.FormatFloat(p.Near.Lat, 'f', -1, 64))
		q.Set("lng", strconv.FormatFloat(p.Near.Lng, 'f', -1, 64))
	}
	setNonEmpty(q, "lang", p.Language)
	return q
}

// LeadSearchParams starts a provider search or fetches the next page of one.
type LeadSearchParams struct {
	// SearchID continues an earlier search with its next page. Leave the
	// other fields empty when set.
	SearchID string `json:"search_id,omitempty"`
	// Provider is one of the LeadProvider* constants.
	Provider string `json:"provider,omitempty"`
	// Query is a free-text search phrase; Category is a preset from
	// [LeadService.Config].
	Query    string `json:"query,omitempty"`
	Category string `json:"category,omitempty"`
	// LocationLabel is the human label of the area, kept with the search.
	LocationLabel string   `json:"location_label"`
	Area          LeadArea `json:"area"`
	// Save keeps the search in the saved list; Name names it.
	Save bool   `json:"save,omitempty"`
	Name string `json:"name,omitempty"`
	// Language is a language tag for provider results.
	Language string `json:"language,omitempty"`
}

// LeadSearchResult is one page of a search.
type LeadSearchResult struct {
	Search *LeadSearch `json:"search"`
	Leads  []Lead      `json:"leads"`
	// Found counts the leads this page returned; OutsideArea those the
	// provider returned beyond the searched area, which are dropped.
	Found       int `json:"found"`
	OutsideArea int `json:"outside_area"`
	// ChargedUSD is what this page cost on metered instances.
	ChargedUSD float64 `json:"charged_usd"`
}

// LeadQueryParams lists leads with a filter, a page at a time. The cursor is
// opaque: pass NextCursor from the previous page, or use [Page.Next] or
// [Page.All].
type LeadQueryParams struct {
	Filter *LeadFilter `json:"filter,omitempty"`
	// Sort is one of the LeadSort* constants; empty means [LeadSortRecent].
	Sort string `json:"sort,omitempty"`
	// Limit is 1 to 200; zero uses the server default of 50.
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

// LeadPoint is the slice of a lead a map draws as a pin.
type LeadPoint struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Category  string   `json:"category"`
	Latitude  float64  `json:"latitude"`
	Longitude float64  `json:"longitude"`
	Status    string   `json:"status"`
	Score     int      `json:"score"`
	HasEmail  bool     `json:"has_email"`
	HasSite   bool     `json:"has_website"`
	HasPhone  bool     `json:"has_phone"`
	Rating    *float64 `json:"rating"`
	Reviews   *int     `json:"review_count"`
	InContact bool     `json:"in_contacts"`
}

// LeadPoints is the map pins for a filter.
type LeadPoints struct {
	Data []LeadPoint `json:"data"`
	// Truncated is true when the pin cap (5,000) was reached, so more leads
	// match than are shown. Narrow the filter or zoom in.
	Truncated bool `json:"truncated"`
}

// LeadFacet is one value of a filterable field and how many leads carry it.
type LeadFacet struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// LeadFacets are the values a filter panel can offer for the current scope.
type LeadFacets struct {
	Categories []LeadFacet `json:"categories"`
	Tags       []LeadFacet `json:"tags"`
	Cities     []LeadFacet `json:"cities"`
}

// LeadBulkUpdateParams edits many leads at once: Selection picks them and
// the embedded patch says what to change.
type LeadBulkUpdateParams struct {
	LeadSelection
	LeadPatch
}

// LeadEnrichParams starts a background run that reads the selected leads'
// websites.
type LeadEnrichParams struct {
	LeadSelection
	// Verify checks the addresses found. The server default is true.
	Verify *bool `json:"verify,omitempty"`
	// People also finds the people each site names and works out their
	// addresses. The server default is true.
	People *bool `json:"people,omitempty"`
}

// Export formats for [LeadExportParams.Format].
const (
	LeadExportCSV  = "csv"
	LeadExportXLSX = "xlsx"
	LeadExportJSON = "json"
)

// LeadExportParams asks for a file of leads.
type LeadExportParams struct {
	LeadSelection
	// Format is [LeadExportCSV] (the default), [LeadExportXLSX] or
	// [LeadExportJSON].
	Format string `json:"format,omitempty"`
	// Columns are keys from [LeadConfig.ExportColumns]; empty uses the
	// default set.
	Columns []string `json:"columns,omitempty"`
	// Rows is [LeadRowsLeads] (the default) or [LeadRowsPeople].
	Rows string `json:"rows,omitempty"`
}

// LeadAddToContactsParams turns selected leads, or the people at them, into
// contacts.
type LeadAddToContactsParams struct {
	LeadSelection
	// CampaignIDs, CategoryIDs and SegmentIDs enroll or file the contacts;
	// all must be UUIDs.
	CampaignIDs []string `json:"campaign_ids,omitempty"`
	CategoryIDs []string `json:"category_ids,omitempty"`
	SegmentIDs  []string `json:"segment_ids,omitempty"`
	// VerifiedOnly keeps out every address that did not check as valid.
	VerifiedOnly bool `json:"verified_only,omitempty"`
	// Who is [LeadWhoBusiness] (the default), [LeadWhoPeople] or
	// [LeadWhoBoth].
	Who string `json:"who,omitempty"`
	// IncludeGuessed adds people whose address was worked out but not
	// confirmed.
	IncludeGuessed bool `json:"include_guessed,omitempty"`
	// KeepExisting leaves a contact already in the workspace as it is; it
	// still gets the segments, categories and campaigns.
	KeepExisting bool `json:"keep_existing,omitempty"`
	// SkipActiveInOtherCampaigns enrolls nobody another campaign is emailing.
	SkipActiveInOtherCampaigns bool `json:"skip_active_in_other_campaigns,omitempty"`
}

// LeadAddToContactsResult says what became of the selection.
type LeadAddToContactsResult struct {
	Selected   int `json:"selected"`
	Imported   int `json:"imported"`
	Updated    int `json:"updated"`
	Skipped    int `json:"skipped"`
	Failed     int `json:"failed"`
	NoEmail    int `json:"no_email"`
	Unverified int `json:"unverified"`
	Suppressed int `json:"suppressed"`
	Guessed    int `json:"guessed"`
	People     int `json:"people"`
	// Errors lists rows that could not be imported (capped server-side).
	Errors []ImportRowError `json:"errors,omitempty"`
}

// LeadSearchUpdateParams renames or saves a search. Nil fields are left alone.
type LeadSearchUpdateParams struct {
	// Name is 1 to 120 characters.
	Name  *string `json:"name,omitempty"`
	Saved *bool   `json:"saved,omitempty"`
}

// LeadPersonInput is a person typed in on a lead. Name or FirstName is
// required.
type LeadPersonInput struct {
	Name        string `json:"name,omitempty"`
	FirstName   string `json:"first_name,omitempty"`
	LastName    string `json:"last_name,omitempty"`
	Title       string `json:"title,omitempty"`
	Email       string `json:"email,omitempty"`
	LinkedInURL string `json:"linkedin_url,omitempty"`
	Phone       string `json:"phone,omitempty"`
}

// LeadPersonUpdateParams edits a person. Nil fields are left alone; an empty
// Email clears the address.
type LeadPersonUpdateParams struct {
	Name        *string `json:"name,omitempty"`
	FirstName   *string `json:"first_name,omitempty"`
	LastName    *string `json:"last_name,omitempty"`
	Title       *string `json:"title,omitempty"`
	Email       *string `json:"email,omitempty"`
	LinkedInURL *string `json:"linkedin_url,omitempty"`
	Phone       *string `json:"phone,omitempty"`
}

// Config lists the providers this workspace can search, the preset
// categories, the export columns and the map style.
func (s *LeadService) Config(ctx context.Context, opts ...RequestOption) (*LeadConfig, *Response, error) {
	return fetch[LeadConfig](ctx, s.client, "leads/config", opts)
}

// Geocode looks up a typed location and returns up to six matching places to
// center a search on. A lookup the geocoder cannot serve answers 503.
func (s *LeadService) Geocode(ctx context.Context, params *LeadGeocodeParams, opts ...RequestOption) ([]LeadGeocodeResult, *Response, error) {
	return fetchData[LeadGeocodeResult](ctx, s.client, withQuery("leads/geocode", params.values()), opts)
}

// Search runs one page of a provider search over an area and stores what it
// finds. To fetch the next page, call again with the returned
// [LeadSearch.ID] as [LeadSearchParams.SearchID] while
// [LeadSearch.HasMore] holds. Pass [WithIdempotencyKey] so a retry replays the
// stored answer instead of searching (and charging) again.
func (s *LeadService) Search(ctx context.Context, params *LeadSearchParams, opts ...RequestOption) (*LeadSearchResult, *Response, error) {
	return send[LeadSearchResult](ctx, s.client.post, "leads/search", params, opts)
}

// Query lists leads with a filter, a page at a time (a read that carries its
// filter in the body). The page reports the total number of matching leads.
func (s *LeadService) Query(ctx context.Context, params *LeadQueryParams, opts ...RequestOption) (*Page[Lead], error) {
	var p LeadQueryParams
	if params != nil {
		p = *params
	}
	page := &Page[Lead]{}
	resp, err := s.client.post(ctx, "leads/query", p, page, opts...)
	if err != nil {
		return nil, err
	}
	page.resp = resp
	page.fetch = func(ctx context.Context, cursor string) (*Page[Lead], error) {
		next := p
		next.Cursor = cursor
		return s.Query(ctx, &next, opts...)
	}
	return page, nil
}

// Points returns the map pins for a filter, up to 5,000. A read with a body.
func (s *LeadService) Points(ctx context.Context, filter *LeadFilter, opts ...RequestOption) (*LeadPoints, *Response, error) {
	return send[LeadPoints](ctx, s.client.post, "leads/points", leadFilterBody{Filter: filter}, opts)
}

// Facets returns the category, tag and city values the filter offers for the
// current scope, with counts. A read with a body.
func (s *LeadService) Facets(ctx context.Context, filter *LeadFilter, opts ...RequestOption) (*LeadFacets, *Response, error) {
	return send[LeadFacets](ctx, s.client.post, "leads/facets", leadFilterBody{Filter: filter}, opts)
}

type leadFilterBody struct {
	Filter *LeadFilter `json:"filter,omitempty"`
}

// Get returns one lead with its people.
func (s *LeadService) Get(ctx context.Context, id string, opts ...RequestOption) (*Lead, *Response, error) {
	return fetch[Lead](ctx, s.client, "leads/"+url.PathEscape(id), opts)
}

// Update edits one lead's status, notes or tags and returns the lead. It sets
// values, so a retry lands on the same state.
func (s *LeadService) Update(ctx context.Context, id string, patch *LeadPatch, opts ...RequestOption) (*Lead, *Response, error) {
	return send[Lead](ctx, s.client.patch, "leads/"+url.PathEscape(id), patch, opts)
}

// BulkUpdate edits many leads at once and returns how many changed. Setting
// values and adding or removing tags converge, so a retry is safe. It takes
// the bulk contact scope.
func (s *LeadService) BulkUpdate(ctx context.Context, params *LeadBulkUpdateParams, opts ...RequestOption) (int64, *Response, error) {
	out, resp, err := send[struct {
		Updated int64 `json:"updated"`
	}](ctx, s.client.patch, "leads", params, opts)
	if err != nil {
		return 0, resp, err
	}
	return out.Updated, resp, nil
}

// BulkDelete removes the selected leads and returns how many were deleted.
// Deleting what is already gone is a no-op, so a retry is safe. It takes the
// bulk contact scope.
func (s *LeadService) BulkDelete(ctx context.Context, selection *LeadSelection, opts ...RequestOption) (int64, *Response, error) {
	out := new(struct {
		Deleted int64 `json:"deleted"`
	})
	resp, err := s.client.call(ctx, http.MethodDelete, "leads", selection, out, opts)
	if err != nil {
		return 0, resp, err
	}
	return out.Deleted, resp, nil
}

// Enrich starts a background run reading the selected leads' websites for
// addresses, and optionally verifying them and finding the people named. It
// answers 202 with the run; follow it with [LeadService.EnrichmentRun]. It
// spends credits on metered instances. Pass [WithIdempotencyKey] to make a
// retry return the same run. It takes the bulk contact scope.
func (s *LeadService) Enrich(ctx context.Context, params *LeadEnrichParams, opts ...RequestOption) (*LeadEnrichmentRun, *Response, error) {
	return send[LeadEnrichmentRun](ctx, s.client.post, "leads/enrich", params, opts)
}

// EnrichmentRuns lists the runs still going (and those that finished within
// the last minute).
func (s *LeadService) EnrichmentRuns(ctx context.Context, opts ...RequestOption) ([]LeadEnrichmentRun, *Response, error) {
	return fetchData[LeadEnrichmentRun](ctx, s.client, "leads/enrichments", opts)
}

// EnrichmentRun returns one run's progress.
func (s *LeadService) EnrichmentRun(ctx context.Context, id string, opts ...RequestOption) (*LeadEnrichmentRun, *Response, error) {
	return fetch[LeadEnrichmentRun](ctx, s.client, "leads/enrichments/"+url.PathEscape(id), opts)
}

// Export downloads the selected leads as CSV, XLSX or JSON, streaming the file
// into w. It is a read, so it takes the contact read scope.
func (s *LeadService) Export(ctx context.Context, params *LeadExportParams, w io.Writer, opts ...RequestOption) (*Response, error) {
	return s.client.post(ctx, "leads/export", params, w, opts...)
}

// AddToContacts imports the selected leads, or the people at them, as
// contacts. The import updates a contact that already exists, so a retry adds
// nobody twice. It takes the bulk contact scope.
func (s *LeadService) AddToContacts(ctx context.Context, params *LeadAddToContactsParams, opts ...RequestOption) (*LeadAddToContactsResult, *Response, error) {
	return send[LeadAddToContactsResult](ctx, s.client.post, "leads/contacts", params, opts)
}

// Searches lists recent searches, or only the saved ones when savedOnly is
// set (the server returns up to 50).
func (s *LeadService) Searches(ctx context.Context, savedOnly bool, opts ...RequestOption) ([]LeadSearch, *Response, error) {
	q := make(url.Values)
	if savedOnly {
		q.Set("saved", "true")
	}
	return fetchData[LeadSearch](ctx, s.client, withQuery("leads/searches", q), opts)
}

// UpdateSearch renames or saves a search. It sets values, so a retry is safe.
func (s *LeadService) UpdateSearch(ctx context.Context, id string, params *LeadSearchUpdateParams, opts ...RequestOption) (*LeadSearch, *Response, error) {
	return send[LeadSearch](ctx, s.client.patch, "leads/searches/"+url.PathEscape(id), params, opts)
}

// DeleteSearch removes a search; the leads it found stay.
func (s *LeadService) DeleteSearch(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "leads/searches/"+url.PathEscape(id), opts...)
}

// AddPerson adds someone to a lead and works out their address when none is
// given. People on a lead are matched by name, so a retry lands on the same
// person.
func (s *LeadService) AddPerson(ctx context.Context, leadID string, in *LeadPersonInput, opts ...RequestOption) (*LeadPerson, *Response, error) {
	return send[LeadPerson](ctx, s.client.post, "leads/"+url.PathEscape(leadID)+"/people", in, opts)
}

// UpdatePerson edits someone on a lead. It sets values, so a retry is safe.
func (s *LeadService) UpdatePerson(ctx context.Context, leadID, personID string, params *LeadPersonUpdateParams, opts ...RequestOption) (*LeadPerson, *Response, error) {
	return send[LeadPerson](ctx, s.client.patch, "leads/"+url.PathEscape(leadID)+"/people/"+url.PathEscape(personID), params, opts)
}

// DeletePerson removes someone from a lead.
func (s *LeadService) DeletePerson(ctx context.Context, leadID, personID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "leads/"+url.PathEscape(leadID)+"/people/"+url.PathEscape(personID), opts...)
}

// FindPersonEmail works out a person's address at the lead's domain again.
// Searching again converges on the same address, so a retry is safe.
func (s *LeadService) FindPersonEmail(ctx context.Context, leadID, personID string, opts ...RequestOption) (*LeadPerson, *Response, error) {
	return send[LeadPerson](ctx, s.client.post, "leads/"+url.PathEscape(leadID)+"/people/"+url.PathEscape(personID)+"/find-email", nil, opts)
}
