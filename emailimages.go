package warmbly

import (
	"context"
	"net/url"
	"time"
)

// EmailImageService manages the workspace's image library for email bodies:
// pictures uploaded once and placed in a campaign step, a template or a reply by
// URL. The bytes are public objects, because a recipient's mail client fetches
// them with no session; these routes only manage the library, never serve
// reads.
type EmailImageService service

// EmailImage is one picture in the workspace's image library.
type EmailImage struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	// UserID is the member who uploaded it; nil when an API key did.
	UserID *string `json:"user_id,omitempty"`
	// Filename is the display name kept on the row: what the library lists and
	// what alt text defaults to. Its extension always matches the sniffed type.
	Filename string `json:"filename"`
	// MimeType is detected from the bytes, never taken from the upload: one of
	// image/png, image/jpeg, image/gif or image/webp.
	MimeType string `json:"mime_type"`
	// Size is the file size in bytes.
	Size int64 `json:"size"`
	// Width and Height are in pixels. They are 0 for a WebP, whose dimensions
	// the server does not read.
	Width  int `json:"width"`
	Height int `json:"height"`
	// URL is the public address to put in an email's <img src>. Deleting the
	// image breaks it in every email already sent.
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

// List returns a page of the image library, newest first. The page size
// defaults to 40 and may be 1 to 100. Pages are cursor-based and carry no
// total.
//
// Requires the view-campaigns permission ([PermReadCampaigns] for an API key).
func (s *EmailImageService) List(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[EmailImage], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[EmailImage](ctx, s.client, "email-images", q, opts...)
}

// Upload adds a picture to the library (201). The multipart field is "file".
// The server judges the bytes, not the declared type: a PNG, JPEG, GIF or WebP
// of 1 byte to 5 MB and at most 4,000 pixels on each side is accepted (SVG is
// not). The upload counts against the workspace's storage quota, and one over
// it is refused with a 400 carrying [ErrCodeStorageLimitReached].
//
// Requires the manage-campaigns permission ([PermWriteCampaigns] for an API key).
func (s *EmailImageService) Upload(ctx context.Context, file *FileUpload, opts ...RequestOption) (*EmailImage, *Response, error) {
	out := new(EmailImage)
	resp, err := s.client.postMultipart(ctx, "email-images", "file", file, nil, out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// Delete removes a picture and its stored bytes (204). Every email already sent
// with the picture loses it. Deleting is idempotent, so a retry after a
// storage failure is safe.
//
// Requires the manage-campaigns permission ([PermWriteCampaigns] for an API key).
func (s *EmailImageService) Delete(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "email-images/"+url.PathEscape(id), opts...)
}
