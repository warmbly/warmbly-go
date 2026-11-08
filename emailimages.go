package warmbly

import (
	"context"
	"net/url"
	"time"
)

// EmailImageService manages the workspace's image library for email bodies.
// An image placed in a message is fetched by the recipient's mail client with
// no session, so uploads become public objects and each [EmailImage] carries
// the URL to insert. Library images count against the workspace storage quota.
//
// Listing needs the campaign read scope; uploading and deleting need the
// campaign write scope.
type EmailImageService service

// EmailImage is one image in the workspace's email image library.
type EmailImage struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	// UserID is who uploaded the image, when known.
	UserID *string `json:"user_id,omitempty"`
	// Filename is the display name, with its extension forced to match the
	// detected image type.
	Filename string `json:"filename"`
	// MimeType is detected from the bytes, never from the upload's header.
	MimeType string `json:"mime_type"`
	// Size is the file size in bytes.
	Size   int64 `json:"size"`
	Width  int   `json:"width"`
	Height int   `json:"height"`
	// URL is the public address to place in an email body.
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

// EmailImageListParams paginates the library, newest first.
type EmailImageListParams struct {
	// Limit is 1 to 100; zero uses the server default of 40.
	ListOptions
}

func (p *EmailImageListParams) values() url.Values {
	q := make(url.Values)
	if p == nil {
		return q
	}
	p.apply(q)
	return q
}

// List returns one page of the library, newest first.
func (s *EmailImageService) List(ctx context.Context, params *EmailImageListParams, opts ...RequestOption) (*Page[EmailImage], error) {
	return listJSON[EmailImage](ctx, s.client, "email-images", params.values(), opts...)
}

// Upload adds an image to the library. PNG, JPEG, GIF and WebP are accepted
// (SVG is not, since no mail client renders it); the type is decided from the
// bytes. The file is capped at 5 MB and 4000 pixels on a side, and a workspace
// over its storage quota is refused with a 403. Each upload creates a new
// image, so pass [WithIdempotencyKey] when retrying.
func (s *EmailImageService) Upload(ctx context.Context, file *FileUpload, opts ...RequestOption) (*EmailImage, *Response, error) {
	out := new(EmailImage)
	resp, err := s.client.postMultipart(ctx, "email-images", "file", file, nil, out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// Delete removes an image and its public file, so any message already sent
// with it loses the image. Deleting is idempotent as far as the stored file
// goes, but a second call for a removed image answers 404.
func (s *EmailImageService) Delete(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "email-images/"+url.PathEscape(id), opts...)
}
