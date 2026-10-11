package warmbly

import (
	"context"
	"net/url"
	"time"
)

// EmailImageService manages the workspace image library for email bodies.
//
// Unlike an attachment, an image placed in a body is fetched by the
// recipient's mail client with no Warmbly session, so it is stored as a public
// object and [EmailImage.URL] is what goes into the HTML. Images count against
// the same per-plan storage quota as attachments.
//
// Reading the library needs [PermReadCampaigns]; uploading and deleting need
// [PermWriteCampaigns] (for a session: view-campaigns and manage-campaigns).
type EmailImageService service

// EmailImage is one image in the workspace library.
type EmailImage struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	// UserID is the member who uploaded it, when known.
	UserID *string `json:"user_id,omitempty"`
	// Filename is the uploader's name for the file, kept on the record (and as
	// the default alt text) but never part of the public URL.
	Filename string `json:"filename"`
	// MimeType is what the server detected from the bytes: image/png,
	// image/jpeg, image/gif or image/webp.
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	// URL is the public address to embed in an email body.
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

// Bounds the server enforces on [EmailImageService.Upload].
const (
	// EmailImageMaxBytes is the largest image accepted, 5 MB.
	EmailImageMaxBytes = 5 * 1024 * 1024
	// EmailImageMaxDimension is the most pixels allowed on either side.
	EmailImageMaxDimension = 4000
)

// List returns a page of the library, newest first. The server defaults
// [ListOptions.Limit] to 40 and caps it at 100. It needs [PermReadCampaigns].
func (s *EmailImageService) List(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[EmailImage], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[EmailImage](ctx, s.client, "email-images", q, opts...)
}

// Upload adds an image to the library and returns it (201). The format is
// detected from the bytes, not the name or declared type: PNG, JPEG, GIF and
// WebP are accepted, SVG never is. An image over [EmailImageMaxBytes] or
// [EmailImageMaxDimension], or one that would exceed the storage quota, is
// refused. It needs [PermWriteCampaigns].
func (s *EmailImageService) Upload(ctx context.Context, file *FileUpload, opts ...RequestOption) (*EmailImage, *Response, error) {
	out := new(EmailImage)
	resp, err := s.client.postMultipart(ctx, "email-images", "file", file, nil, out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// Delete removes an image and its stored bytes (204). Any email already sent
// with it loses the image. Deleting is safe to retry. It needs
// [PermWriteCampaigns].
func (s *EmailImageService) Delete(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "email-images/"+url.PathEscape(id), opts...)
}
