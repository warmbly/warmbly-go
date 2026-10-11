package warmbly

import (
	"context"
	"net/http"
	"testing"
)

func TestSegmentSetMembersSelection(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusOK, "application/json", `{"updated": 1200}`, &got)
	n, _, err := c.Segments.SetMembersSelection(context.Background(), "seg_1", &ContactSelection{
		All:     true,
		Filters: &ContactSearchParams{CategoryIDs: []string{"cat_1"}},
		Exclude: []string{"ct_9"},
	}, SegmentMemberInclude)
	if err != nil {
		t.Fatalf("SetMembersSelection: %v", err)
	}
	assertRequest(t, got, "POST", "/v1/segments/seg_1/members", "",
		`"all":true`, `"filters":{"category_ids":["cat_1"]}`, `"exclude":["ct_9"]`, `"mode":"include"`)
	if n != 1200 {
		t.Errorf("updated = %d", n)
	}
}

func TestSegmentFieldOptionLabels(t *testing.T) {
	var got capturedRequest
	c := syncServer(t, http.StatusOK, "application/json", `{"data": [
  {"field": "mail_host", "label": "Email provider", "group": "Contact", "kind": "enum",
   "options": ["google_workspace", "other"], "option_labels": {"google_workspace": "Google Workspace", "other": "Other"}}
]}`, &got)
	fields, _, err := c.Segments.Fields(context.Background())
	if err != nil {
		t.Fatalf("Fields: %v", err)
	}
	if fields[0].OptionLabels[MailHostGoogleWorkspace] != "Google Workspace" {
		t.Errorf("labels = %+v", fields[0].OptionLabels)
	}
}

func TestFormTriageFields(t *testing.T) {
	var got capturedRequest
	ctx := context.Background()

	c := syncServer(t, http.StatusOK, "application/json", `{"base_url": "https://forms.example.com", "captcha_available": false, "triage_available": true}`, &got)
	cfg, _, err := c.Forms.Config(ctx)
	if err != nil {
		t.Fatalf("Config: %v", err)
	}
	if !cfg.TriageAvailable {
		t.Errorf("config = %+v", cfg)
	}

	c = syncServer(t, http.StatusOK, "application/json", `{"id": "frm_1", "triage_enabled": true, "created_at": "2026-10-01T09:00:00Z", "updated_at": "2026-10-01T09:00:00Z"}`, &got)
	f, _, err := c.Forms.Update(ctx, "frm_1", &FormUpdateParams{TriageEnabled: Bool(true)})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	assertRequest(t, got, "PATCH", "/v1/forms/frm_1", "", `"triage_enabled":true`)
	if !f.TriageEnabled {
		t.Errorf("form = %+v", f)
	}

	c = syncServer(t, http.StatusOK, "application/json", `{"data": [
  {"id": "sub_1", "form_id": "frm_1", "organization_id": "org_1", "data": {}, "source_url": "", "triage": "junk", "triage_confidence": 0.93, "created_at": "2026-10-01T09:00:00Z"}
], "pagination": {"has_more": false}}`, &got)
	page, err := c.Forms.ListSubmissions(ctx, "frm_1", nil)
	if err != nil {
		t.Fatalf("ListSubmissions: %v", err)
	}
	if s := page.Data[0]; s.Triage != FormTriageJunk || s.TriageConfidence != 0.93 {
		t.Errorf("submission = %+v", s)
	}
}
