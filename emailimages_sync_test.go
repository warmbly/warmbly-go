package warmbly

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func sampleEmailImage() EmailImage {
	uploader := "usr_1"
	return EmailImage{
		ID:             "img_1",
		OrganizationID: "org_1",
		UserID:         &uploader,
		Filename:       "logo.png",
		MimeType:       "image/png",
		Size:           20480,
		Width:          320,
		Height:         96,
		URL:            "https://api.example.com/public/email-images/org_1/abc.png",
		CreatedAt:      time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC),
	}
}

func TestEmailImageUploadUsesFileField(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, http.StatusCreated, jsonOf(t, sampleEmailImage()))
	img, resp, err := c.EmailImages.Upload(context.Background(), &FileUpload{
		Filename:    "logo.png",
		Content:     strings.NewReader("\x89PNG fake"),
		ContentType: "image/png",
	})
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "POST", "/v1/email-images", "")
	fields, files := got.multipartParts(t)
	if files["file"] != "logo.png|\x89PNG fake" || len(fields) != 0 {
		t.Errorf("multipart = fields %v files %v; the server reads the part named %q", fields, files, "file")
	}
	if resp.StatusCode != http.StatusCreated || img.ID != "img_1" || img.MimeType != "image/png" ||
		img.Width != 320 || img.Size != 20480 || img.UserID == nil || img.URL == "" {
		t.Errorf("image = %+v", img)
	}
}

func TestEmailImageUploadRejectsMissingFile(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, http.StatusCreated, `{}`)
	if _, _, err := c.EmailImages.Upload(context.Background(), nil); err == nil {
		t.Fatal("a nil upload must fail")
	}
	if got.method != "" {
		t.Errorf("no request should be sent, got %s %s", got.method, got.path)
	}
}

func TestEmailImageListAndDelete(t *testing.T) {
	var got syncCapture
	next := "cur_2"
	body := jsonOf(t, map[string]any{
		"data":       []EmailImage{sampleEmailImage()},
		"pagination": Pagination{HasMore: true, NextCursor: &next},
	})
	c := respondingClient(t, &got, 200, body)
	page, err := c.EmailImages.List(context.Background(), &ListOptions{Limit: 40, Cursor: "cur_1"})
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "GET", "/v1/email-images", "cursor=cur_1&limit=40")
	if len(page.Data) != 1 || page.Data[0].Filename != "logo.png" || page.NextCursor() != "cur_2" {
		t.Errorf("page = %+v", page)
	}

	c = respondingClient(t, &got, http.StatusNoContent, "")
	resp, err := c.EmailImages.Delete(context.Background(), "img_1")
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "DELETE", "/v1/email-images/img_1", "")
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestEmailImageWithoutUserDecodes(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, `{"data":[{"id":"img_2","filename":"a.webp","mime_type":"image/webp","size":10,"width":0,"height":0,"url":"u","created_at":"2026-09-03T12:00:00Z"}],"pagination":{"next_cursor":null,"has_more":false}}`)
	page, err := c.EmailImages.List(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if page.Data[0].UserID != nil || page.Data[0].Width != 0 || page.HasMore() {
		t.Errorf("image = %+v", page.Data[0])
	}
}
