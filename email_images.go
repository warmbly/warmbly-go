package warmbly

import (
	"context"
	"net/url"
	"time"
)

// EmailImage is one image in a workspace's library for email bodies. The bytes
// live at a public URL, because the recipient's mail client fetches them with
// no session.
type EmailImage struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organization_id"`
	UserID         *string `json:"user_id,omitempty"`
	// Filename is the display name; the public URL does not contain it.
	Filename string `json:"filename"`
	MimeType string `json:"mime_type"`
	// Size is in bytes.
	Size   int64 `json:"size"`
	Width  int   `json:"width"`
	Height int   `json:"height"`
	// URL is the public address to put in an <img> tag.
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

// ListImages returns a page of the workspace's email image library, newest
// first. The page size defaults to 40 and is capped at 100. Requires an
// organization and the read_campaigns scope.
func (s *EmailService) ListImages(ctx context.Context, params *ListOptions, opts ...RequestOption) (*Page[EmailImage], error) {
	q := make(url.Values)
	params.apply(q)
	return listJSON[EmailImage](ctx, s.client, "email-images", q, opts...)
}

// UploadImage adds an image to the library and returns it, answering 201. The
// server decides the type from the bytes, never from ContentType: PNG, JPEG,
// GIF and WebP are accepted up to 5 MB and 4000 pixels on a side. SVG is
// refused because no mail client renders it. The upload counts against the
// workspace's storage quota and fails with the storage-limit error once it is
// full ([ErrCodeStorageLimitReached]). Requires an organization and the
// write_campaigns scope.
func (s *EmailService) UploadImage(ctx context.Context, file *FileUpload, opts ...RequestOption) (*EmailImage, *Response, error) {
	out := new(EmailImage)
	resp, err := s.client.postMultipart(ctx, "email-images", "file", file, nil, out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// DeleteImage removes an image and its public file. An email already sent with
// the image loses it, so check where it is used first. The server answers 204.
// Requires an organization and the write_campaigns scope.
func (s *EmailService) DeleteImage(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "email-images/"+url.PathEscape(id), opts...)
}
