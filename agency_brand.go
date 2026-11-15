package warmbly

import (
	"context"
	"time"
)

// AgencyPortalSignIn is where an agency's clients sign in to its portal
// domain.
type AgencyPortalSignIn string

const (
	// AgencyPortalSignInPortal signs clients in on the agency's own domain.
	// The default.
	AgencyPortalSignInPortal AgencyPortalSignIn = "portal"
	// AgencyPortalSignInWarmbly signs them in on the main dashboard address,
	// which hands the portal a session.
	AgencyPortalSignInWarmbly AgencyPortalSignIn = "warmbly"
)

// Values for the [PortalTheme] fields. An empty field means the
// default.
const (
	// AgencyPortalFontSystem sets the portal in the system typeface.
	AgencyPortalFontSystem = "system"
	// AgencyPortalFontGeometric sets the portal in a geometric sans.
	AgencyPortalFontGeometric = "geometric"
	// AgencyPortalFontSerif sets the portal in a serif.
	AgencyPortalFontSerif = "serif"

	// AgencyPortalCornersSharp squares buttons, fields and cards.
	AgencyPortalCornersSharp = "sharp"
	// AgencyPortalCornersRound rounds them.
	AgencyPortalCornersRound = "round"

	// AgencyPortalAppearanceLight starts clients in the light scheme.
	AgencyPortalAppearanceLight = "light"
	// AgencyPortalAppearanceDark starts clients in the dark scheme.
	AgencyPortalAppearanceDark = "dark"

	// AgencyPortalSignInLayoutSplit puts the sign-in form beside a panel.
	AgencyPortalSignInLayoutSplit = "split"
	// AgencyPortalSignInLayoutCover puts it over a full-page background.
	AgencyPortalSignInLayoutCover = "cover"

	// AgencyPortalSignInBackgroundAccent fills the sign-in page with the
	// accent color.
	AgencyPortalSignInBackgroundAccent = "accent"
	// AgencyPortalSignInBackgroundDark fills it with a dark color.
	AgencyPortalSignInBackgroundDark = "dark"
	// AgencyPortalSignInBackgroundImage uses the uploaded sign-in image.
	AgencyPortalSignInBackgroundImage = "image"
)

// AgencyPortalDomainStatus is the agency's portal domain and its DNS
// verification. The wire names reuse the forms custom-domain shape, so the
// domain travels as "forms_domain".
type AgencyPortalDomainStatus struct {
	// Domain is the portal host, for example "portal.example.com"; empty
	// when none is set.
	Domain     string     `json:"forms_domain"`
	Verified   bool       `json:"forms_domain_verified"`
	VerifiedAt *time.Time `json:"forms_domain_verified_at"`
	// CNAMETarget is the value to point the domain's CNAME at; empty means
	// the deployment has no portal address configured.
	CNAMETarget string `json:"cname_target"`
	// TXTName and TXTValue are the ownership record a new verification
	// needs beside the CNAME.
	TXTName  string `json:"txt_name,omitempty"`
	TXTValue string `json:"txt_value,omitempty"`
	// Status is "verified", "unset", "no_target", "pending", "not_found",
	// "wrong_target", "proof_missing" or "lookup_error".
	Status  string `json:"status"`
	Message string `json:"message"`
	// Observed is what DNS actually returned.
	Observed string `json:"observed,omitempty"`
	// FormsHostUnresolvable reports that the deployment's own host does not
	// resolve, an operator problem rather than the customer's record.
	FormsHostUnresolvable bool `json:"forms_host_unresolvable"`
	// EdgeRecords are the records the edge provider still needs before it
	// serves the domain over HTTPS.
	EdgeRecords []DNSRecord `json:"edge_records,omitempty"`
}

// AgencyBrand is the agency's white-label brand, inherited by every client
// workspace without a brand of its own, plus its portal domain.
type AgencyBrand struct {
	Brand BrandSettings `json:"brand"`
	// FullWhiteLabel reports a plan that may hide the attribution.
	FullWhiteLabel bool `json:"full_white_label"`
	// CanManage reports whether the caller may change the brand (owner or
	// admin).
	CanManage    bool                     `json:"can_manage"`
	PortalDomain AgencyPortalDomainStatus `json:"portal_domain"`
	// PortalBaseURL is the verified portal domain as an origin, else the
	// shared portal address; empty when the deployment has none.
	PortalBaseURL string `json:"portal_base_url"`
	// PoweredBy is the deployment's attribution; nil when it has none.
	PoweredBy    *PoweredBy         `json:"powered_by,omitempty"`
	PortalSignIn AgencyPortalSignIn `json:"portal_sign_in"`
	// IconURL and SignInImageURL are the uploaded tab icon and sign-in page
	// image; empty when none.
	IconURL        string      `json:"icon_url"`
	SignInImageURL string      `json:"sign_in_image_url"`
	Theme          PortalTheme `json:"theme"`
}

// AgencyBrandParams replaces the agency's brand. Text fields are written as
// sent, so an empty one clears it.
type AgencyBrandParams struct {
	DisplayName string `json:"display_name"`
	// AccentColor is a hex color such as "#0284c7"; empty is the default.
	AccentColor string `json:"accent_color"`
	WebsiteURL  string `json:"website_url"`
	// HidePoweredBy needs [AgencyBrand.FullWhiteLabel]
	// ("white_label_not_entitled").
	HidePoweredBy bool `json:"hide_powered_by"`
	// PortalSignIn is kept when nil.
	PortalSignIn *AgencyPortalSignIn `json:"portal_sign_in,omitempty"`
	// Theme is kept when nil; a value replaces the whole theme.
	Theme *PortalTheme `json:"theme,omitempty"`
}

// Brand returns the agency's brand and portal domain.
//
// Scope: [PermAgencyRead].
func (s *AgencyService) Brand(ctx context.Context, agencyID string, opts ...RequestOption) (*AgencyBrand, *Response, error) {
	return fetch[AgencyBrand](ctx, s.client, agencyPath(agencyID, "brand"), opts)
}

// UpdateBrand replaces the agency's brand. Owner or admin. A replace, so a
// retry is safe.
//
// Session only.
func (s *AgencyService) UpdateBrand(ctx context.Context, agencyID string, params *AgencyBrandParams, opts ...RequestOption) (*AgencyBrand, *Response, error) {
	return send[AgencyBrand](ctx, s.client.put, agencyPath(agencyID, "brand"), params, opts)
}

func (s *AgencyService) uploadBrandImage(ctx context.Context, agencyID, image string, file *FileUpload, opts []RequestOption) (*AgencyBrand, *Response, error) {
	out := new(AgencyBrand)
	resp, err := s.client.postMultipart(ctx, agencyPath(agencyID, "brand", image), "file", file, nil, out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

func (s *AgencyService) deleteBrandImage(ctx context.Context, agencyID, image string, opts []RequestOption) (*AgencyBrand, *Response, error) {
	out := new(AgencyBrand)
	resp, err := s.deleteJSON(ctx, agencyPath(agencyID, "brand", image), out, opts)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// UploadBrandLogo sets the agency's logo and returns the brand. The image
// must be a PNG or JPEG, at most 2 MB and 1024 pixels on a side. Owner or
// admin.
//
// Session only.
func (s *AgencyService) UploadBrandLogo(ctx context.Context, agencyID string, file *FileUpload, opts ...RequestOption) (*AgencyBrand, *Response, error) {
	return s.uploadBrandImage(ctx, agencyID, "logo", file, opts)
}

// DeleteBrandLogo removes the agency's logo and returns the brand.
//
// Session only.
func (s *AgencyService) DeleteBrandLogo(ctx context.Context, agencyID string, opts ...RequestOption) (*AgencyBrand, *Response, error) {
	return s.deleteBrandImage(ctx, agencyID, "logo", opts)
}

// UploadBrandIcon sets the portal's browser tab icon and returns the brand.
// The image must be a PNG, at most 1 MB and 512 pixels on a side. Owner or
// admin.
//
// Session only.
func (s *AgencyService) UploadBrandIcon(ctx context.Context, agencyID string, file *FileUpload, opts ...RequestOption) (*AgencyBrand, *Response, error) {
	return s.uploadBrandImage(ctx, agencyID, "icon", file, opts)
}

// DeleteBrandIcon removes the portal's tab icon and returns the brand.
//
// Session only.
func (s *AgencyService) DeleteBrandIcon(ctx context.Context, agencyID string, opts ...RequestOption) (*AgencyBrand, *Response, error) {
	return s.deleteBrandImage(ctx, agencyID, "icon", opts)
}

// UploadBrandSignInImage sets the image on the portal's sign-in page and
// returns the brand. The image must be a PNG or JPEG, at most 4 MB and 2560
// pixels on a side. Owner or admin.
//
// Session only.
func (s *AgencyService) UploadBrandSignInImage(ctx context.Context, agencyID string, file *FileUpload, opts ...RequestOption) (*AgencyBrand, *Response, error) {
	return s.uploadBrandImage(ctx, agencyID, "sign-in-image", file, opts)
}

// DeleteBrandSignInImage removes the sign-in page image and returns the
// brand.
//
// Session only.
func (s *AgencyService) DeleteBrandSignInImage(ctx context.Context, agencyID string, opts ...RequestOption) (*AgencyBrand, *Response, error) {
	return s.deleteBrandImage(ctx, agencyID, "sign-in-image", opts)
}

// SetPortalDomain stores the agency's portal host (for example
// "portal.example.com") and checks its DNS at once; an empty domain clears
// it. A host another agency holds is refused ("portal_domain_taken"). Owner
// or admin.
//
// Session only.
func (s *AgencyService) SetPortalDomain(ctx context.Context, agencyID, domain string, opts ...RequestOption) (*AgencyBrand, *Response, error) {
	body := map[string]string{"domain": domain}
	return send[AgencyBrand](ctx, s.client.put, agencyPath(agencyID, "brand", "domain"), body, opts)
}

// VerifyPortalDomain re-checks the portal domain's DNS (a CNAME to the
// deployment's portal address plus the TXT proof). Safe to retry. Owner or
// admin.
//
// Session only.
func (s *AgencyService) VerifyPortalDomain(ctx context.Context, agencyID string, opts ...RequestOption) (*AgencyBrand, *Response, error) {
	return send[AgencyBrand](ctx, s.client.post, agencyPath(agencyID, "brand", "domain", "verify"), nil, opts)
}
