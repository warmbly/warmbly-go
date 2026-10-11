package warmbly

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// OAuthDeveloperAccess is whether the workspace, and the caller in it, may
// register and publish OAuth applications right now.
type OAuthDeveloperAccess struct {
	// Blocked is true when the instance's operators have blocked the
	// workspace or the person; Reason is what they gave.
	Blocked bool   `json:"blocked"`
	Reason  string `json:"reason,omitempty"`
}

// DeveloperAccess reports whether the workspace and the caller may register
// and publish OAuth applications. A blocked developer's writes fail with
// [ErrCodeDeveloperAccessBlocked]; reading it first lets a screen say so up
// front. It is served alongside the application list, so it costs the same
// call as [OAuthAppService.List].
//
// Requires the [PermAPIKeys] API permission; an OAuth access token is refused.
func (s *OAuthAppService) DeveloperAccess(ctx context.Context, opts ...RequestOption) (*OAuthDeveloperAccess, *Response, error) {
	var out struct {
		DeveloperAccess OAuthDeveloperAccess `json:"developer_access"`
	}
	resp, err := s.client.get(ctx, "oauth/applications", &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return &out.DeveloperAccess, resp, nil
}

// SetLogo uploads a new logo for an existing application (PNG or JPEG, up to
// 2 MB and at least 32 pixels on each side), stores it and returns the
// updated application. The image is re-encoded on the server and the previous
// logo is deleted. A new logo ends a directory feature, like any change to
// the application's public face. A suspended application fails with
// [ErrCodeAppSuspended].
//
// Requires the [PermAPIKeys] API permission; an OAuth access token is refused.
func (s *OAuthAppService) SetLogo(ctx context.Context, id string, file *FileUpload, opts ...RequestOption) (*OAuthApp, *Response, error) {
	out := new(OAuthApp)
	resp, err := s.client.postMultipart(ctx, "oauth/applications/"+url.PathEscape(id)+"/logo", "file", file, nil, out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// DeleteLogo removes the application's logo and returns the updated
// application. A suspended application fails with [ErrCodeAppSuspended].
//
// Requires the [PermAPIKeys] API permission; an OAuth access token is refused.
func (s *OAuthAppService) DeleteLogo(ctx context.Context, id string, opts ...RequestOption) (*OAuthApp, *Response, error) {
	out := new(OAuthApp)
	resp, err := s.client.call(ctx, http.MethodDelete, "oauth/applications/"+url.PathEscape(id)+"/logo", nil, out, opts)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// --- community directory listing ---

// Directory listing states returned in [AppListing.Status].
const (
	// AppListingPublished is reachable by its link, and shown in discovery
	// once enough workspaces use the application.
	AppListingPublished = "published"
	// AppListingFeatured was picked by the instance's operators and is always
	// shown in discovery.
	AppListingFeatured = "featured"
	// AppListingHidden was taken down by the operators and is reachable
	// nowhere; [AppListing.StatusNote] says why. Only they can restore it.
	AppListingHidden = "hidden"
)

// Directory categories accepted in [AppListingParams.Category].
const (
	AppCategoryCRM           = "crm"
	AppCategoryAutomation    = "automation"
	AppCategoryNotifications = "notifications"
	AppCategoryMeetings      = "meetings"
	AppCategoryData          = "data"
	AppCategoryVerification  = "verification"
	AppCategoryAI            = "ai"
	AppCategoryOther         = "other"
)

// Error codes carried in [Error.Code] by the directory listing calls.
const (
	// ErrCodeInvalidListing is the 400 for a listing field that breaks a
	// rule; the message names it.
	ErrCodeInvalidListing = "invalid_listing"
	// ErrCodeListingSlugTaken is the 409 for a slug another listing holds.
	ErrCodeListingSlugTaken = "listing_slug_taken"
	// ErrCodeAppNotListable is the 400 for an application that cannot be
	// listed: dynamically registered, disabled or suspended.
	ErrCodeAppNotListable = "app_not_listable"
	// ErrCodeListingHidden is the 409 for unpublishing a listing the
	// operators hid.
	ErrCodeListingHidden = "listing_hidden"
)

// AppListing is an OAuth application's page in the community app directory,
// where other workspaces find and install it.
type AppListing struct {
	ApplicationID  string `json:"application_id"`
	OrganizationID string `json:"organization_id"`
	// Slug is the listing's link.
	Slug        string `json:"slug"`
	Tagline     string `json:"tagline"`
	Description string `json:"description"`
	// Category is one of the AppCategory* constants.
	Category   string `json:"category"`
	InstallURL string `json:"install_url"`
	SupportURL string `json:"support_url"`
	PrivacyURL string `json:"privacy_url"`
	// Status is one of the AppListing* constants.
	Status string `json:"status"`
	// StatusNote is the operators' note on their last decision, shown to the
	// developer; StatusAt is when they made it.
	StatusNote  string     `json:"status_note,omitempty"`
	StatusAt    *time.Time `json:"status_at,omitempty"`
	SubmittedAt time.Time  `json:"submitted_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// AppListingParams publishes or edits a listing. The whole listing is
// replaced, so send every field.
type AppListingParams struct {
	// Slug is 3 to 48 lowercase letters, digits or single dashes, starting
	// and ending with a letter or digit. Built-in integration names are
	// reserved.
	Slug string `json:"slug"`
	// Tagline is required, one line of at most 120 characters.
	Tagline string `json:"tagline"`
	// Description is at most 2,000 characters of plain text.
	Description string `json:"description"`
	// Category is one of the AppCategory* constants.
	Category string `json:"category"`
	// InstallURL is required; SupportURL and PrivacyURL are optional. All
	// must be https.
	InstallURL string `json:"install_url"`
	SupportURL string `json:"support_url,omitempty"`
	PrivacyURL string `json:"privacy_url,omitempty"`
}

// Listing returns the application's directory listing, or nil when it is not
// published. A dynamically registered client fails with
// [ErrCodeAppNotListable].
//
// Requires the [PermAPIKeys] API permission; an OAuth access token is refused.
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

// PublishListing publishes the application in the community directory, or
// replaces its listing, and returns it. Saving the same content again changes
// nothing; any real change ends a feature. The application must be active and
// not suspended, or the call fails with [ErrCodeAppNotListable]; a slug in use
// fails with [ErrCodeListingSlugTaken] and a field that breaks a rule with
// [ErrCodeInvalidListing].
//
// Requires the [PermAPIKeys] API permission; an OAuth access token is refused.
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

// UnpublishListing takes the application out of the directory. Workspaces
// that already authorized it keep their grants. A listing the operators hid
// cannot be removed by its developer and fails with [ErrCodeListingHidden].
//
// Requires the [PermAPIKeys] API permission; an OAuth access token is refused.
func (s *OAuthAppService) UnpublishListing(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "oauth/applications/"+url.PathEscape(id)+"/listing", opts...)
}

// --- every member's authorizations (session-only) ---

// OAuthMemberAuthorization is one member's live authorization of an
// application in the workspace.
type OAuthMemberAuthorization struct {
	ApplicationID string `json:"application_id"`
	Name          string `json:"name"`
	LogoURL       string `json:"logo_url"`
	WebsiteURL    string `json:"website_url"`
	// UserID, UserEmail and UserName identify the member who authorized it.
	UserID    string `json:"user_id"`
	UserEmail string `json:"user_email"`
	UserName  string `json:"user_name"`
	// Scopes is the granted permission mask (the Perm* constants).
	Scopes       uint64     `json:"scopes"`
	AuthorizedAt time.Time  `json:"authorized_at"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
}

// WorkspaceAuthorizations lists every member's live application
// authorizations in the session's workspace, for the people who manage its
// credentials. [OAuthAppService.AuthorizedApps] is the caller's own subset.
//
// This route is session-only and needs the manage-API-keys or manage-settings
// organization permission.
func (s *OAuthAppService) WorkspaceAuthorizations(ctx context.Context, opts ...RequestOption) ([]OAuthMemberAuthorization, *Response, error) {
	var out struct {
		Authorizations []OAuthMemberAuthorization `json:"authorizations"`
	}
	resp, err := s.client.get(ctx, "oauth/workspace-authorizations", &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	if out.Authorizations == nil {
		out.Authorizations = []OAuthMemberAuthorization{}
	}
	return out.Authorizations, resp, nil
}

// RevokeWorkspaceAuthorization ends one member's authorization of an
// application and revokes the tokens it issued. Other members' grants for the
// same application are untouched. The call is recorded in the audit log.
//
// This route is session-only and needs the manage-API-keys or manage-settings
// organization permission.
func (s *OAuthAppService) RevokeWorkspaceAuthorization(ctx context.Context, applicationID, userID string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "oauth/workspace-authorizations/"+url.PathEscape(applicationID)+"/members/"+url.PathEscape(userID), opts...)
}
