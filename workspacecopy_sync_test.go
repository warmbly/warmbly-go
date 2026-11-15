package warmbly

import (
	"context"
	"testing"
)

const workspaceCopyFixture = `{"data": [
	{"organization_id": "org_2", "organization_name": "Client B", "status": "partial",
		"campaign": {"id": "camp_9", "name": "Q4 outbound"},
		"templates": [{"id": "tpl_7", "name": "Follow-up"}],
		"review": [{"step_id": "st_1", "step_name": "Step 2", "fields": ["tag", "form"]}],
		"warnings": ["The template \"Old\" could not be copied."]},
	{"organization_id": "org_3", "organization_name": "Client C", "status": "failed",
		"templates": [], "review": [], "warnings": [],
		"error": {"code": "copy_destination_forbidden", "message": "You cannot manage campaigns in Client C."}}
]}`

func TestCampaignCopyToWorkspaces(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, workspaceCopyFixture)
	res, _, err := c.Campaigns.CopyToWorkspaces(context.Background(), "camp_1", &CopyCampaignParams{
		OrganizationIDs: []string{"org_2", "org_3"},
		TemplateIDs:     []string{"tpl_1"},
	})
	if err != nil {
		t.Fatalf("CopyToWorkspaces: %v", err)
	}
	checkRequest(t, got, "POST", "/v1/campaigns/camp_1/copy-to", "", `{"organization_ids":["org_2","org_3"],"template_ids":["tpl_1"]}`)
	if len(res) != 2 {
		t.Fatalf("results = %+v", res)
	}
	if res[0].Status != WorkspaceCopyPartial || res[0].Campaign == nil || res[0].Campaign.ID != "camp_9" || res[0].Error != nil {
		t.Errorf("first = %+v", res[0])
	}
	if len(res[0].Review) != 1 || res[0].Review[0].Fields[1] != "form" || len(res[0].Warnings) != 1 {
		t.Errorf("review/warnings = %+v", res[0])
	}
	if res[1].Status != WorkspaceCopyFailed || res[1].Campaign != nil || res[1].Error == nil || res[1].Error.Code != "copy_destination_forbidden" {
		t.Errorf("second = %+v", res[1])
	}

	got = recordedRequest{}
	if _, _, err := c.Campaigns.CopyToWorkspaces(context.Background(), "camp_1", &CopyCampaignParams{OrganizationIDs: []string{"org_2"}}); err != nil {
		t.Fatalf("CopyToWorkspaces: %v", err)
	}
	checkRequest(t, got, "POST", "/v1/campaigns/camp_1/copy-to", "", `{"organization_ids":["org_2"]}`)
}

func TestTemplateCopyToWorkspaces(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, workspaceCopyFixture)
	res, _, err := c.Templates.CopyToWorkspaces(context.Background(), &CopyTemplatesParams{
		TemplateIDs:     []string{"tpl_1", "tpl_2"},
		OrganizationIDs: []string{"org_2"},
	})
	if err != nil {
		t.Fatalf("CopyToWorkspaces: %v", err)
	}
	checkRequest(t, got, "POST", "/v1/templates/copy-to", "", `{"template_ids":["tpl_1","tpl_2"],"organization_ids":["org_2"]}`)
	if len(res) != 2 || res[0].Templates[0].Name != "Follow-up" {
		t.Errorf("results = %+v", res)
	}
}

func TestCampaignLeadTimezones(t *testing.T) {
	var got recordedRequest
	c := recordingFixtureClient(t, &got, `{
		"campaign_id": "camp_1", "timezone_mode": "recipient", "timezone": "America/New_York",
		"total": 120, "unknown": 14, "resolving": 3,
		"zones": [
			{"timezone": "Europe/Berlin", "country": "DE", "leads": 60},
			{"timezone": "Etc/GMT-2", "country": "", "leads": 4}
		]
	}`)
	out, _, err := c.Campaigns.LeadTimezones(context.Background(), "camp_1")
	if err != nil {
		t.Fatalf("LeadTimezones: %v", err)
	}
	checkRequest(t, got, "GET", "/v1/campaigns/camp_1/lead-timezones", "", "")
	if out.TimezoneMode != CampaignTimezoneModeRecipient || out.Total != 120 || out.Unknown != 14 || out.Resolving != 3 {
		t.Errorf("out = %+v", out)
	}
	if len(out.Zones) != 2 || out.Zones[0].Country != "DE" || out.Zones[1].Country != "" || out.Zones[1].Leads != 4 {
		t.Errorf("zones = %+v", out.Zones)
	}
}
