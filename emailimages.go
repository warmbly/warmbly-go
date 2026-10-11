package warmbly

import (
	"context"
	"net/url"
	"time"
)

// EmailImageService manages the workspace's image library for email bodies.
// Unlike an attachment, an image placed in a body is fetched by the
// recipient's mail client with no session, so each image is stored as a
// public object and [EmailImage.URL] is what goes into the HTML. The service
// only manages the library; reading an image needs no credentials.
//
// Images count against the same per-plan storage quota as campaign
// attachments.
type EmailImageService service

// EmailImage is one image in the library.
type EmailImage struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organization_id"`
	UserID         *string `json:"user_id,omitempty"`
	// Filename is the display name, with the extension forced to match the
	// detected format. It never appears in URL.
	Filename string `json:"filename"`
	// MimeType is image/png, image/jpeg, image/gif or image/webp, detected
	// from the bytes rather than taken from the upload.
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
	// Width and Height are in pixels. They are 0 for a WebP image, whose
	// dimensions the server does not read.
	Width  int `json:"width"`
	Height int `json:"height"`
	// URL is the public address a mail client loads the image from.
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

// List returns a page of the library, newest first. [ListOptions.Limit] is 1
// to 100 (default 40); anything else is a 400.
//
// Requires [PermReadCampaigns].
func (s *EmailImageService) List(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[EmailImage], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[EmailImage](ctx, s.client, "email-images", q, opts...)
}

// Upload adds a PNG, JPEG, GIF or WebP image of at most 5 MB and 4000 pixels
// on each side. The format is detected from the bytes, so
// [FileUpload.ContentType] is ignored; SVG is refused. An upload that would
// exceed the plan's storage quota is refused with
// [ErrCodeStorageLimitReached].
//
// Requires [PermWriteCampaigns].
func (s *EmailImageService) Upload(ctx context.Context, file *FileUpload, opts ...RequestOption) (*EmailImage, *Response, error) {
	out := new(EmailImage)
	resp, err := s.client.postMultipart(ctx, "email-images", "file", file, nil, out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// Delete removes an image and its stored bytes, so any email already sent
// with it shows a broken image from then on. The API answers 204.
//
// Requires [PermWriteCampaigns].
func (s *EmailImageService) Delete(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "email-images/"+url.PathEscape(id), opts...)
}
