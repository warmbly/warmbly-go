package warmbly

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestCampaignReviewRouting(t *testing.T) {
	var got recordedRequest
	ctx := context.Background()
	change := `{"campaign_id":"c1","review_leads":true,"held":0,"released":2}`
	emails := `{"campaign_id":"c1","lead":{"contact_id":"k1","email":"a@x.example","status":"waiting","personalized":0},"steps":[]}`

	cases := []struct {
		name       string
		status     int
		resp       string
		call       func(c *Client) error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string
	}{
		{"Review", 200, `{"campaign_id":"c1","review_leads":false,"counts":{},"missing":[],"ai":{}}`, func(c *Client) error { _, _, e := c.Campaigns.Review(ctx, "c1"); return e }, "GET", "/v1/campaigns/c1/review", "", ""},
		{"SetReviewOn", 200, change, func(c *Client) error { _, _, e := c.Campaigns.SetReview(ctx, "c1", true); return e }, "PUT", "/v1/campaigns/c1/review", "", `{"enabled":true}`},
		{"SetReviewOff", 200, change, func(c *Client) error { _, _, e := c.Campaigns.SetReview(ctx, "c1", false); return e }, "PUT", "/v1/campaigns/c1/review", "", `{"enabled":false}`},
		{"ReviewLeadsNil", 200, `{"data":[],"pagination":{"next_cursor":null,"has_more":false}}`, func(c *Client) error { _, e := c.Campaigns.ReviewLeads(ctx, "c1", nil); return e }, "GET", "/v1/campaigns/c1/review/leads", "", ""},
		{"ReviewLeads", 200, `{"data":[],"pagination":{"next_cursor":null,"has_more":false}}`, func(c *Client) error {
			_, e := c.Campaigns.ReviewLeads(ctx, "c1", &CampaignReviewLeadsParams{
				ListOptions: ListOptions{Limit: 20, Cursor: "cur"},
				Status:      CampaignReviewWaiting, Missing: CampaignReviewFieldFirstName, Query: "acme",
			})
			return e
		}, "GET", "/v1/campaigns/c1/review/leads", "cursor=cur&limit=20&missing=FirstName&q=acme&status=waiting", ""},
		{"ApproveIDs", 200, change, func(c *Client) error {
			_, _, e := c.Campaigns.ApproveReviewLeads(ctx, "c1", &CampaignReviewApproveParams{ContactIDs: []string{"k1", "k2"}})
			return e
		}, "POST", "/v1/campaigns/c1/review/approve", "", `{"contact_ids":["k1","k2"]}`},
		{"ApproveAll", 200, change, func(c *Client) error {
			_, _, e := c.Campaigns.ApproveReviewLeads(ctx, "c1", &CampaignReviewApproveParams{All: true})
			return e
		}, "POST", "/v1/campaigns/c1/review/approve", "", `{"all":true}`},
		{"Reopen", 200, change, func(c *Client) error {
			_, _, e := c.Campaigns.ReopenReviewLeads(ctx, "c1", []string{"k1"})
			return e
		}, "POST", "/v1/campaigns/c1/review/reopen", "", `{"contact_ids":["k1"]}`},
		{"WriteNil", 200, `{"written":0,"failed":0,"remaining":0}`, func(c *Client) error { _, _, e := c.Campaigns.WriteReviewAI(ctx, "c1", nil); return e }, "POST", "/v1/campaigns/c1/review/write", "", ""},
		{"WriteZero", 200, `{"written":0,"failed":0,"remaining":0}`, func(c *Client) error {
			_, _, e := c.Campaigns.WriteReviewAI(ctx, "c1", &CampaignReviewWriteParams{})
			return e
		}, "POST", "/v1/campaigns/c1/review/write", "", `{}`},
		{"WriteFull", 200, `{"written":10,"failed":0,"remaining":5,"next":"n2"}`, func(c *Client) error {
			_, _, e := c.Campaigns.WriteReviewAI(ctx, "c1", &CampaignReviewWriteParams{StepID: "s1", ContactIDs: []string{"k1"}, WaitingOnly: true, After: "n1"})
			return e
		}, "POST", "/v1/campaigns/c1/review/write", "", `{"step_id":"s1","contact_ids":["k1"],"waiting_only":true,"after":"n1"}`},
		{"WriteEmptyIDs", 200, `{"written":0,"failed":0,"remaining":0}`, func(c *Client) error {
			// A non-nil empty list reaches the server as [], which it refuses,
			// instead of being dropped and widening the pass to every lead.
			_, _, e := c.Campaigns.WriteReviewAI(ctx, "c1", &CampaignReviewWriteParams{ContactIDs: []string{}})
			return e
		}, "POST", "/v1/campaigns/c1/review/write", "", `{"contact_ids":[]}`},
		{"LeadEmails", 200, emails, func(c *Client) error { _, _, e := c.Campaigns.LeadEmails(ctx, "c1", "k1"); return e }, "GET", "/v1/campaigns/c1/leads/k1/emails", "", ""},
		{"SetLeadEmail", 200, `{"campaign_id":"c1","contact_id":"k1","step_id":"s1","subject":"Hi","body_html":"<p>x</p>","body_plain":"","updated_at":"2026-11-14T10:02:00Z"}`, func(c *Client) error {
			_, _, e := c.Campaigns.SetLeadEmail(ctx, "c1", "k1", "s1", &LeadEmailCopy{Subject: "Hi", BodyHTML: "<p>x</p>"})
			return e
		}, "PUT", "/v1/campaigns/c1/leads/k1/emails/s1", "", `{"subject":"Hi","body_html":"<p>x</p>","body_plain":""}`},
		{"ResetLeadEmail", 204, ``, func(c *Client) error { _, e := c.Campaigns.ResetLeadEmail(ctx, "c1", "k1", "s1"); return e }, "DELETE", "/v1/campaigns/c1/leads/k1/emails/s1", "", ""},
		{"WriteLeadEmail", 200, emails, func(c *Client) error { _, _, e := c.Campaigns.WriteLeadEmailAI(ctx, "c1", "k1", "s1", nil); return e }, "POST", "/v1/campaigns/c1/leads/k1/emails/s1/write", "", ""},
		{"RewriteLeadEmail", 200, emails, func(c *Client) error {
			_, _, e := c.Campaigns.WriteLeadEmailAI(ctx, "c1", "k1", "s1", &LeadEmailWriteParams{Rewrite: true})
			return e
		}, "POST", "/v1/campaigns/c1/leads/k1/emails/s1/write", "", `{"rewrite":true}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got = recordedRequest{}
			c := emailFixtureClient(t, tc.status, tc.resp, &got)
			if err := tc.call(c); err != nil {
				t.Fatalf("call: %v", err)
			}
			if got.method != tc.wantMethod || got.path != tc.wantPath {
				t.Errorf("got %s %s, want %s %s", got.method, got.path, tc.wantMethod, tc.wantPath)
			}
			if got.rawQuery != tc.wantQuery {
				t.Errorf("query = %q, want %q", got.rawQuery, tc.wantQuery)
			}
			if strings.TrimSpace(got.body) != tc.wantBody {
				t.Errorf("body = %q, want %q", got.body, tc.wantBody)
			}
		})
	}
}

func TestCampaignReviewDecode(t *testing.T) {
	ctx := context.Background()

	c := emailFixtureClient(t, 200, `{
		"campaign_id":"8f1d6b2e-2b7a-4c9e-9a1f-0e6d4c3b2a10","review_leads":true,
		"counts":{"total":240,"waiting":180,"ready":12,"held":3,"started":45,"personalized":7},
		"missing":[{"field":"FirstName","leads":14}],
		"undeliverable":2,"unsubscribed":1,
		"ai":{"step_id":"5c2e7a10-9b1d-4f3e-8a6c-2d4b0e1f7a93","blocks":1,"written":40,"pending":155,"estimated_cost_usd":1.55,"auto":true},
		"template_errors":[{"step_id":"s2","position":2,"message":"unclosed action"}]
	}`, nil)
	r, _, err := c.Campaigns.Review(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if !r.ReviewLeads || r.Counts.Waiting != 180 || r.Counts.Personalized != 7 || r.Missing[0].Field != CampaignReviewFieldFirstName || r.Missing[0].Leads != 14 ||
		r.Undeliverable != 2 || r.Unsubscribed != 1 || r.AI.StepID == nil || r.AI.EstimatedCostUSD != 1.55 || !r.AI.Auto ||
		len(r.TemplateErrors) != 1 || r.TemplateErrors[0].Position != 2 {
		t.Fatalf("Review = %+v", r)
	}

	// A campaign with no email step: no ai.step_id.
	c = emailFixtureClient(t, 200, `{"campaign_id":"c1","review_leads":false,"counts":{"total":0,"waiting":0,"ready":0,"held":0,"started":0,"personalized":0},"missing":[],"undeliverable":0,"unsubscribed":0,"ai":{"blocks":0,"written":0,"pending":0,"estimated_cost_usd":0,"auto":false}}`, nil)
	if r, _, err = c.Campaigns.Review(ctx, "c1"); err != nil || r.AI.StepID != nil || r.TemplateErrors != nil {
		t.Fatalf("Review empty = %+v, %v", r, err)
	}

	c = emailFixtureClient(t, 200, `{"campaign_id":"c1","review_leads":false,"held":0,"released":192}`, nil)
	ch, _, err := c.Campaigns.SetReview(ctx, "c1", false)
	if err != nil || ch.ReviewLeads || ch.Released != 192 || ch.CampaignID != "c1" {
		t.Fatalf("SetReview = %+v, %v", ch, err)
	}

	page := `{"data":[{"contact_id":"3a5e9c71-4f2b-4d88-9a0c-1b7e5d2f6c34","email":"ana@acme.example","first_name":"","last_name":"Ruiz","company":"Acme","status":"waiting","hold":{"since":"2026-11-14T10:02:00Z","source":"review"},"personalized":1,"missing":["FirstName"],"added_at":"2026-11-14T10:02:00Z"},
		{"contact_id":"k2","email":"bo@acme.example","first_name":"Bo","last_name":"","company":"","status":"brand_new","personalized":0}],
		"pagination":{"next_cursor":"YW5h","has_more":true}}`
	var got recordedRequest
	c = emailFixtureClient(t, 200, page, &got)
	p, err := c.Campaigns.ReviewLeads(ctx, "c1", &CampaignReviewLeadsParams{Status: CampaignReviewAll})
	if err != nil {
		t.Fatal(err)
	}
	lead := p.Data[0]
	if lead.Status != CampaignReviewWaiting || lead.Hold == nil || lead.Hold.Source != LeadHoldReview || lead.Hold.Until != nil ||
		lead.Missing[0] != CampaignReviewFieldFirstName || lead.AddedAt == nil || lead.Personalized != 1 {
		t.Fatalf("lead = %+v", lead)
	}
	// An unknown state still decodes, and absent hold/added_at stay nil.
	if p.Data[1].Status != "brand_new" || p.Data[1].Hold != nil || p.Data[1].AddedAt != nil {
		t.Fatalf("lead 2 = %+v", p.Data[1])
	}
	if p.Pagination.Total != nil || !p.HasMore() || p.NextCursor() != "YW5h" {
		t.Fatalf("pagination = %+v", p.Pagination)
	}
	if _, err := p.Next(ctx); err != nil {
		t.Fatal(err)
	}
	if got.rawQuery != "cursor=YW5h&status=all" {
		t.Errorf("next query = %q", got.rawQuery)
	}

	c = emailFixtureClient(t, 200, `{"written":3,"failed":1,"remaining":145,"stopped":"insufficient_credits","next":"YW5hQGFj"}`, nil)
	w, _, err := c.Campaigns.WriteReviewAI(ctx, "c1", nil)
	if err != nil || w.Written != 3 || w.Failed != 1 || w.Remaining != 145 || w.Stopped != CampaignReviewStopInsufficientCredits || w.Next != "YW5hQGFj" {
		t.Fatalf("WriteReviewAI = %+v, %v", w, err)
	}

	c = emailFixtureClient(t, 200, `{"campaign_id":"c1","contact_id":"k1","step_id":"s1","subject":"Hi","body_html":"<p>Hi</p>","body_plain":"Hi","edited_by":"u1","updated_at":"2026-11-14T10:02:00Z"}`, nil)
	o, _, err := c.Campaigns.SetLeadEmail(ctx, "c1", "k1", "s1", &LeadEmailCopy{BodyPlain: "Hi"})
	if err != nil || o.EditedBy == nil || *o.EditedBy != "u1" || o.UpdatedAt.IsZero() || o.BodyPlain != "Hi" {
		t.Fatalf("SetLeadEmail = %+v, %v", o, err)
	}
	c = emailFixtureClient(t, 200, `{"campaign_id":"c1","contact_id":"k1","step_id":"s1","subject":"","body_html":"","body_plain":"Hi","updated_at":"2026-11-14T10:02:00Z"}`, nil)
	if o, _, err = c.Campaigns.SetLeadEmail(ctx, "c1", "k1", "s1", nil); err != nil || o.EditedBy != nil {
		t.Fatalf("SetLeadEmail api key = %+v, %v", o, err)
	}

	c = emailFixtureClient(t, http.StatusNoContent, ``, nil)
	resp, err := c.Campaigns.ResetLeadEmail(ctx, "c1", "k1", "s1")
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("ResetLeadEmail = %v, %v", resp, err)
	}
}

func TestCampaignLeadEmailsDecode(t *testing.T) {
	want := CampaignLeadEmails{
		CampaignID: "8f1d6b2e-2b7a-4c9e-9a1f-0e6d4c3b2a10",
		Lead:       CampaignReviewLead{ContactID: "k1", Email: "ana@acme.example", Status: CampaignReviewStarted, Missing: []CampaignReviewField{CampaignReviewFieldFirstName}},
		From:       &LeadEmailFrom{Name: "Sam", Email: "sam@out.example"},
		Steps: []CampaignLeadEmailStep{{
			StepID: "s1", Position: 1, Name: "Step 1", Subject: "Quick question about Acme",
			BodyHTML: `<p>Hi there, <span data-ai-pending="opener" data-ai-name="Opener"></span></p>`, BodyPlain: "Hi there, [AI: Opener]",
			Source:       LeadEmailCopy{Subject: "Quick question about {{.Company}}", BodyHTML: `<p>Hi {{.FirstName | default "there"}}</p>`},
			Personalized: true, StepChanged: true, VariantID: String("v1"),
			AIBlocks: []LeadEmailAIBlock{{ID: "opener", Name: "Opener", Written: false}},
			Missing:  []CampaignReviewField{CampaignReviewFieldCompany}, Errors: []string{"bad"}, Sent: true,
		}},
	}
	body, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	c := emailFixtureClient(t, 200, string(body), nil)
	got, _, err := c.Campaigns.WriteLeadEmailAI(context.Background(), "c1", "k1", "s1", &LeadEmailWriteParams{Rewrite: true})
	if err != nil {
		t.Fatal(err)
	}
	st := got.Steps[0]
	if got.From == nil || got.From.Email != "sam@out.example" || got.Lead.Status != CampaignReviewStarted ||
		st.Source.Subject != want.Steps[0].Source.Subject || !st.Personalized || !st.StepChanged || st.VariantID == nil || *st.VariantID != "v1" ||
		len(st.AIBlocks) != 1 || st.AIBlocks[0].Written || st.Missing[0] != CampaignReviewFieldCompany || !st.Sent || st.SentAt != nil || st.EditedAt != nil {
		t.Fatalf("LeadEmails = %+v", got)
	}

	// The server omits from, variant_id, ai_blocks and the timestamps when empty.
	c = emailFixtureClient(t, 200, `{"campaign_id":"c1","lead":{"contact_id":"k1","email":"a@x.example","first_name":"","last_name":"","company":"","status":"ready","personalized":0},"steps":[{"step_id":"s1","position":1,"name":"","subject":"","body_html":"","body_plain":"x","source":{"subject":"","body_html":"","body_plain":"x"},"threaded":true,"personalized":false,"step_changed":false,"sent":false}]}`, nil)
	got, _, err = c.Campaigns.LeadEmails(context.Background(), "c1", "k1")
	if err != nil || got.From != nil || got.Steps[0].VariantID != nil || !got.Steps[0].Threaded || got.Steps[0].AIBlocks != nil {
		t.Fatalf("LeadEmails empty = %+v, %v", got, err)
	}
}

func TestCampaignDecodeReviewLeads(t *testing.T) {
	c := emailFixtureClient(t, 200, `{"id":"c1","name":"Q4","status":"draft","review_leads":true}`, nil)
	camp, _, err := c.Campaigns.Get(context.Background(), "c1")
	if err != nil || !camp.ReviewLeads {
		t.Fatalf("Get = %+v, %v", camp, err)
	}
}
