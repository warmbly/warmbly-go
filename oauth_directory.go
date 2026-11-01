package warmbly

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// OAuthDeveloperAccess is what the workspace may do with OAuth apps right now.
// An operator can block a workspace or a person from registering or publishing
// apps.
type OAuthDeveloperAccess struct {
	// Blocked is true when registering and publishing apps is refused.
	Blocked bool `json:"blocked"`
	// Reason is the operator's explanation, when there is one.
	Reason string `json:"reason,omitempty"`
}

// OAuthAppList is the answer of [OAuthAppService.ListWithAccess]: the
// workspace's applications and whether the caller may register or publish
// more.
type OAuthAppList struct {
	Applications    []OAuthApp           `json:"applications"`
	DeveloperAccess OAuthDeveloperAccess `json:"developer_access"`
}

// Directory listing states returned in [AppListing.Status]. The server may add
// states; treat an unknown one as not visible.
const (
	// AppListingPublished is reachable by its link, and listed in discovery
	// once enough workspaces use it.
	AppListingPublished = "published"
	// AppListingFeatured was picked by an operator and is always listed.
	AppListingFeatured = "featured"
	// AppListingHidden was taken down by an operator and is reachable
	// nowhere. It stays until the instance's administrators restore it.
	AppListingHidden = "hidden"
)

// AppListing is an OAuth app's page in the community directory.
type AppListing struct {
	ApplicationID  string `json:"application_id"`
	OrganizationID string `json:"organization_id"`
	// Slug is the app's link in the directory.
	Slug        string `json:"slug"`
	Tagline     string `json:"tagline"`
	Description string `json:"description"`
	// Category is one of the directory's categories, for example "crm",
	// "automation", "ai" or "other".
	Category   string `json:"category"`
	InstallURL string `json:"install_url"`
	SupportURL string `json:"support_url"`
	PrivacyURL string `json:"privacy_url"`
	// Status is one of the AppListing* constants.
	Status string `json:"status"`
	// StatusNote is the operator's note on a hidden or featured listing.
	StatusNote  string     `json:"status_note,omitempty"`
	StatusAt    *time.Time `json:"status_at,omitempty"`
	SubmittedAt time.Time  `json:"submitted_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// AppListingParams publishes or edits an app's directory listing. The server
// replaces the listing with what is sent.
type AppListingParams struct {
	Slug        string `json:"slug"`
	Tagline     string `json:"tagline"`
	Description string `json:"description"`
	Category    string `json:"category"`
	InstallURL  string `json:"install_url"`
	SupportURL  string `json:"support_url"`
	PrivacyURL  string `json:"privacy_url"`
}

// WorkspaceAuthorization is one member's live authorization of an OAuth app in
// the workspace, as the people who manage its credentials see it.
type WorkspaceAuthorization struct {
	ApplicationID string `json:"application_id"`
	Name          string `json:"name"`
	LogoURL       string `json:"logo_url"`
	WebsiteURL    string `json:"website_url"`
	UserID        string `json:"user_id"`
	UserEmail     string `json:"user_email"`
	UserName      string `json:"user_name"`
	// Scopes is the permission bitmask the member granted.
	Scopes       uint64     `json:"scopes"`
	AuthorizedAt time.Time  `json:"authorized_at"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
}

// ListWithAccess is [OAuthAppService.List] together with the caller's
// developer access, so a client can explain why registering is refused.
func (s *OAuthAppService) ListWithAccess(ctx context.Context, opts ...RequestOption) (*OAuthAppList, *Response, error) {
	out, resp, err := fetch[OAuthAppList](ctx, s.client, "oauth/applications", opts)
	if err != nil {
		return nil, resp, err
	}
	if out.Applications == nil {
		out.Applications = []OAuthApp{}
	}
	return out, resp, nil
}

// SetLogo uploads a logo for application id and attaches it in one step, then
// returns the updated application. The image must be a PNG or JPEG of at most
// 2 MB, between 32 and 1024 pixels on a side; the server re-encodes it and
// deletes the previous logo once nothing uses it. It fails when the app is
// suspended or the workspace is blocked from registering apps. Recorded in the
// audit log. Session-only: it needs the manage-API-keys organization
// permission and is not available to an OAuth app token.
func (s *OAuthAppService) SetLogo(ctx context.Context, id string, file *FileUpload, opts ...RequestOption) (*OAuthApp, *Response, error) {
	out := new(OAuthApp)
	resp, err := s.client.postMultipart(ctx, "oauth/applications/"+url.PathEscape(id)+"/logo", "file", file, nil, out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// RemoveLogo clears application id's logo and returns the updated
// application. Session-only, same permission as [OAuthAppService.SetLogo].
func (s *OAuthAppService) RemoveLogo(ctx context.Context, id string, opts ...RequestOption) (*OAuthApp, *Response, error) {
	out := new(OAuthApp)
	resp, err := s.client.call(ctx, http.MethodDelete, "oauth/applications/"+url.PathEscape(id)+"/logo", nil, out, opts)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// Listing returns application id's directory listing, or nil when the app is
// not published. Session-only; needs the manage-API-keys organization
// permission.
func (s *OAuthAppService) Listing(ctx context.Context, id string, opts ...RequestOption) (*AppListing, *Response, error) {
	var out struct {
		Listing *AppListing `json:"listing"`
	}
	resp, err := s.client.get(ctx, "oauth/applications/"+url.PathEscape(id)+"/listing", &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Listing, resp, nil
}

// PublishListing publishes application id in the community directory or edits
// its listing, and returns the stored listing. An edit that changes nothing
// keeps its status; a real change ends a feature, and a hidden listing stays
// hidden.
//
// It fails with code "invalid_listing" for a malformed listing,
// "listing_slug_taken" (409) when another app uses the slug, "app_not_listable"
// when the app is suspended or not active, and "developer_access_blocked" (403)
// when the workspace is blocked from publishing. Recorded in the audit log.
// Session-only; needs the manage-API-keys organization permission.
func (s *OAuthAppService) PublishListing(ctx context.Context, id string, params *AppListingParams, opts ...RequestOption) (*AppListing, *Response, error) {
	var out struct {
		Listing *AppListing `json:"listing"`
	}
	resp, err := s.client.put(ctx, "oauth/applications/"+url.PathEscape(id)+"/listing", params, &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out.Listing, resp, nil
}

// UnpublishListing removes application id from the community directory.
// Workspaces that already authorized the app keep their grant. A listing an
// operator hid cannot be removed (409, code "listing_hidden"). Recorded in the
// audit log. Session-only; needs the manage-API-keys organization permission.
func (s *OAuthAppService) UnpublishListing(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "oauth/applications/"+url.PathEscape(id)+"/listing", opts...)
}

// WorkspaceAuthorizations lists every member's live app authorizations in the
// workspace, for the people who manage its credentials; compare
// [OAuthAppService.AuthorizedApps], which lists only the caller's own.
// Session-only; needs the manage-API-keys or manage-settings organization
// permission.
func (s *OAuthAppService) WorkspaceAuthorizations(ctx context.Context, opts ...RequestOption) ([]WorkspaceAuthorization, *Response, error) {
	var out struct {
		Authorizations []WorkspaceAuthorization `json:"authorizations"`
	}
	resp, err := s.client.get(ctx, "oauth/workspace-authorizations", &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	if out.Authorizations == nil {
		out.Authorizations = []WorkspaceAuthorization{}
	}
	return out.Authorizations, resp, nil
}

// RevokeWorkspaceAuthorization ends one member's authorization of an app in
// the workspace and revokes the tokens issued under it. applicationID is
// [WorkspaceAuthorization.ApplicationID] and userID the member's
// [WorkspaceAuthorization.UserID]. Recorded in the audit log. Session-only;
// same permission as [OAuthAppService.WorkspaceAuthorizations].
func (s *OAuthAppService) RevokeWorkspaceAuthorization(ctx context.Context, applicationID, userID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "oauth/workspace-authorizations/"+url.PathEscape(applicationID)+"/members/"+url.PathEscape(userID), opts...)
}
