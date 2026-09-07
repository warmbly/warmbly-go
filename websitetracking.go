package warmbly

import (
	"context"
	"time"
)

// WebsiteTrackingService configures website visitor tracking: the snippet a
// workspace installs on its own site so page views land on a contact's
// timeline. It governs consent mode, location precision, allowed hosts,
// retention and the site key.
//
// These routes are session-only and need the manage-settings organization
// permission ([OrgPermManageSettings]): they are privacy governance, so they
// stay off the API-key surface. The snippet itself is not served by the API;
// it is loaded from the deployment's tracking host
// ([WebsiteTrackingSettings.TrackingHost]) as
// https://<tracking_host>/tracking.js with the site key in data-site, and page
// views go to that host too.
type WebsiteTrackingService service

// Consent modes returned in [WebsiteTrackingSettings.ConsentMode].
const (
	// WebsiteConsentExplicit records nothing until the page calls
	// warmbly('consent', 'granted'). The default; enforced server-side as
	// well, so a stale snippet cannot downgrade it.
	WebsiteConsentExplicit = "explicit"
	// WebsiteConsentImplicit records on load. The workspace asserts its own
	// lawful basis by choosing it.
	WebsiteConsentImplicit = "implicit"
)

// How much IP-derived location is kept, in
// [WebsiteTrackingSettings.LocationPrecision].
const (
	WebsiteLocationNone    = "none"
	WebsiteLocationCountry = "country"
	WebsiteLocationCity    = "city"
)

// Bounds for [WebsiteTrackingSettings.RetentionDays].
const (
	WebsiteRetentionMinDays     = 7
	WebsiteRetentionMaxDays     = 365
	WebsiteRetentionDefaultDays = 90
)

// WebsiteTrackingSettings is a workspace's tracking configuration. It is
// created on first read with tracking disabled, so every workspace has a site
// key to show but nothing is accepted until Enabled is set.
type WebsiteTrackingSettings struct {
	OrganizationID string `json:"organization_id"`
	// Enabled gates ingestion entirely.
	Enabled bool `json:"enabled"`
	// SiteKey identifies the workspace in the snippet. It is public — it only
	// says which workspace a page view belongs to — but a leaked copy can be
	// retired with [WebsiteTrackingService.RotateKey].
	SiteKey string `json:"site_key"`
	// ConsentMode is [WebsiteConsentExplicit] or [WebsiteConsentImplicit].
	ConsentMode string `json:"consent_mode"`
	// LocationPrecision is one of the WebsiteLocation* constants.
	LocationPrecision string `json:"location_precision"`
	// AllowedHosts are the hosts the site runs on; a host covers its
	// subdomains. Page views from any other host are ignored, and campaign
	// links only identify a visitor when they lead to one of these. Empty
	// means views from anywhere are recorded but never tied to a contact.
	AllowedHosts []string `json:"allowed_hosts"`
	// RetentionDays is how long page views are kept, between
	// [WebsiteRetentionMinDays] and [WebsiteRetentionMaxDays].
	RetentionDays int       `json:"retention_days"`
	UpdatedAt     time.Time `json:"updated_at"`
	// TrackingHost is the deployment's tracking host, so a client can render
	// the exact snippet. Empty when the install has none, in which case
	// tracking cannot work.
	TrackingHost string `json:"tracking_host"`
}

// WebsiteTrackingUpdateParams changes the tracking configuration. Nil fields
// are left unchanged; an empty (non-nil) AllowedHosts clears the list.
type WebsiteTrackingUpdateParams struct {
	Enabled           *bool     `json:"enabled,omitempty"`
	ConsentMode       *string   `json:"consent_mode,omitempty"`
	LocationPrecision *string   `json:"location_precision,omitempty"`
	AllowedHosts      *[]string `json:"allowed_hosts,omitempty"`
	RetentionDays     *int      `json:"retention_days,omitempty"`
}

// Settings returns the workspace's tracking configuration, creating a disabled
// one with a fresh site key on first call.
func (s *WebsiteTrackingService) Settings(ctx context.Context, opts ...RequestOption) (*WebsiteTrackingSettings, *Response, error) {
	return fetch[WebsiteTrackingSettings](ctx, s.client, "website-tracking/settings", opts)
}

// UpdateSettings changes the tracking configuration and returns the result.
func (s *WebsiteTrackingService) UpdateSettings(ctx context.Context, params *WebsiteTrackingUpdateParams, opts ...RequestOption) (*WebsiteTrackingSettings, *Response, error) {
	return send[WebsiteTrackingSettings](ctx, s.client.patch, "website-tracking/settings", params, opts)
}

// RotateKey issues a new site key and returns the settings carrying it. The
// old key stops working at once, so every installed snippet has to be
// updated. It takes no body and is safe to repeat; each call issues another
// key.
func (s *WebsiteTrackingService) RotateKey(ctx context.Context, opts ...RequestOption) (*WebsiteTrackingSettings, *Response, error) {
	return send[WebsiteTrackingSettings](ctx, s.client.post, "website-tracking/settings/rotate-key", nil, opts)
}
