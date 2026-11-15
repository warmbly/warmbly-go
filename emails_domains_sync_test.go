package warmbly

import (
	"context"
	"net/http"
	"testing"
)

func TestSendingDomainRouting(t *testing.T) {
	var got recordedRequest
	c := routingClient(t, &got)
	ctx := context.Background()

	runExactCases(t, &got, []exactCase{
		{"ListDomains", func() error { _, _, e := c.Emails.ListDomains(ctx); return e }, "GET", "/v1/emails/domains", "", ""},
		{"BulkSetupDomains", func() error {
			_, _, e := c.Emails.BulkSetupDomains(ctx, &BulkDomainSetupParams{
				Domains:       []string{"acme.io", "acme.co"},
				TrackingLabel: "track",
				RedirectURLs:  map[string]string{"acme.co": "https://acme.com/co"},
				ServedBy:      RedirectServedByCloud,
			})
			return e
		}, "POST", "/v1/emails/domains/bulk", "",
			`{"domains":["acme.io","acme.co"],"tracking_label":"track","redirect_urls":{"acme.co":"https://acme.com/co"},"served_by":"cloud"}`},
		{"TrackingSuggestion", func() error { _, _, e := c.Emails.TrackingSuggestion(ctx, "acme.io"); return e }, "GET", "/v1/emails/domains/acme.io/tracking-suggestion", "", ""},
		{"SetDomainTracking", func() error { _, _, e := c.Emails.SetDomainTracking(ctx, "acme.io", "link.acme.io"); return e }, "PUT", "/v1/emails/domains/acme.io/tracking", "", `{"host":"link.acme.io"}`},
		{"SetDomainTracking clear", func() error { _, _, e := c.Emails.SetDomainTracking(ctx, "acme.io", ""); return e }, "PUT", "/v1/emails/domains/acme.io/tracking", "", `{"host":""}`},
		{"SetDomainRedirect", func() error {
			_, _, e := c.Emails.SetDomainRedirect(ctx, "acme.io", &DomainRedirectParams{TargetURL: "https://acme.com", IncludeWWW: Bool(false)})
			return e
		}, "PUT", "/v1/emails/domains/acme.io/redirect", "", `{"target_url":"https://acme.com","include_www":false}`},
		{"VerifyDomainRedirect", func() error { _, _, e := c.Emails.VerifyDomainRedirect(ctx, "acme.io"); return e }, "POST", "/v1/emails/domains/acme.io/redirect/verify", "", ""},
		{"VerifyRedirectWebsite", func() error {
			_, _, e := c.Emails.VerifyRedirectWebsite(ctx, "acme.io", "https://acme.com")
			return e
		}, "POST", "/v1/emails/domains/acme.io/redirect/website", "", `{"target_url":"https://acme.com"}`},
		{"DeleteDomainRedirect", func() error { _, e := c.Emails.DeleteDomainRedirect(ctx, "acme.io"); return e }, "DELETE", "/v1/emails/domains/acme.io/redirect", "", ""},
		{"SetDomainVendorForwarding", func() error {
			_, _, e := c.Emails.SetDomainVendorForwarding(ctx, "acme.io", "https://acme.com")
			return e
		}, "PUT", "/v1/emails/domains/acme.io/vendor-forwarding", "", `{"url":"https://acme.com"}`},
		{"SetDomainVendorTracking", func() error {
			_, _, e := c.Emails.SetDomainVendorTracking(ctx, "acme.io", "link.acme.io")
			return e
		}, "POST", "/v1/emails/domains/acme.io/vendor-tracking", "", `{"host":"link.acme.io"}`},
	})
}

func TestSendingDomainListDecode(t *testing.T) {
	c := emailFixtureClient(t, http.StatusOK, `{"data": [{
		"domain": "acme.io", "mailboxes": 4, "mail_hosts": ["google"],
		"auth_state": "failing", "auth_spf": true, "auth_dkim": false, "auth_dmarc": true,
		"tracking_domains": [{"host": "link.acme.io", "verified": true, "mailboxes": 4}],
		"redirect": {
			"id": "rd_1", "domain": "acme.io", "target_url": "https://acme.com", "include_www": true, "verified": true,
			"verified_at": "2026-11-10T08:00:00Z", "created_at": "2026-11-09T08:00:00Z",
			"records": [{"purpose": "redirect", "type": "A", "name": "acme.io", "value": "203.0.113.7", "ok": true}],
			"served_by": "instance", "serve_host": "track.warmbly.test",
			"reach": {"status": "not_reaching", "hint": "edge_record", "detail": "Cloudflare refused the name.", "proxy": "cloudflare"},
			"target_active": false, "target_inactive_reason": "Verify the website first.",
			"website": {"domain": "acme.com", "verified": false, "record": {"purpose": "website", "type": "TXT", "name": "_warmbly.acme.com", "value": "wm=abc", "ok": false}}
		},
		"vendors": ["inboxkit"], "vendor_connection_ids": ["vc_1"],
		"vendor_domain": {"vendor": "inboxkit", "connection_id": "vc_1", "forwarding": "https://acme.com", "can_forward": true,
			"can_unforward": true, "forwarding_reviewed": true, "can_dns": true, "dns_types": ["CNAME", "TXT"]}
	}]}`, nil)
	list, _, err := c.Emails.ListDomains(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("list = %+v", list)
	}
	d := list[0]
	if d.AuthState != AuthStateFailing || d.AuthDKIM || len(d.TrackingDomains) != 1 || !d.TrackingDomains[0].Verified {
		t.Errorf("domain = %+v", d)
	}
	r := d.Redirect
	if r == nil || r.ServedBy != RedirectServedByInstance || r.VerifiedAt == nil || r.LastCheckedAt != nil || len(r.Records) != 1 || !r.Records[0].OK {
		t.Fatalf("redirect = %+v", r)
	}
	if r.Reach == nil || r.Reach.Status != RedirectReachNotReaching || r.Reach.Hint != "edge_record" || r.Reach.CheckedAt != nil {
		t.Errorf("reach = %+v", r.Reach)
	}
	if r.TargetActive == nil || *r.TargetActive || r.Website == nil || r.Website.Record == nil || r.Website.Record.Type != "TXT" {
		t.Errorf("target = %v website = %+v", r.TargetActive, r.Website)
	}
	if v := d.VendorDomain; v == nil || !v.ForwardingReviewed || len(v.DNSTypes) != 2 || d.VendorConnectionIDs[0] != "vc_1" {
		t.Errorf("vendor domain = %+v", v)
	}
}

func TestSendingDomainTrackingDecode(t *testing.T) {
	c := emailFixtureClient(t, http.StatusOK, `{"tracking": {
		"tracking_domain": "link.acme.io", "tracking_domain_verified": false, "tracking_domain_verified_at": null,
		"cname_target": "track.warmbly.test", "txt_name": "_warmbly.link.acme.io", "txt_value": "wm=xyz",
		"status": "proof_missing", "message": "Add the TXT record.", "tracking_host_unresolvable": false
	}, "mailboxes": 4}`, nil)
	res, _, err := c.Emails.SetDomainVendorTracking(context.Background(), "acme.io", "link.acme.io")
	if err != nil {
		t.Fatal(err)
	}
	if res.Mailboxes != 4 || res.Tracking.Status != TrackingStatusProofMissing || res.Tracking.TXTName != "_warmbly.link.acme.io" || res.Tracking.TrackingDomainVerifiedAt != nil {
		t.Errorf("result = %+v", res)
	}
}

func TestSendingDomainBulkDecode(t *testing.T) {
	c := emailFixtureClient(t, http.StatusOK, `{"data": [
		{"domain": "acme.io", "tracking": {"host": "track.acme.io", "via": "vendor", "verified": true, "mailboxes": 2},
			"redirect": {"target_url": "https://acme.com", "via": "vendor", "verified": false, "reviewed": true}},
		{"domain": "acme.co", "tracking": {"host": "track.acme.co", "via": "dns", "verified": false, "mailboxes": 1,
			"cname_target": "track.warmbly.test", "note": "The vendor cannot write CNAME records."},
			"redirect": {"via": "dns", "verified": false, "error": "Another workspace already redirects this domain.", "code": "domain_redirect_taken"}}
	]}`, nil)
	rows, _, err := c.Emails.BulkSetupDomains(context.Background(), &BulkDomainSetupParams{Domains: []string{"acme.io", "acme.co"}, TrackingLabel: "track"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Tracking.Via != BulkViaVendor || !rows[0].Redirect.Reviewed {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[1].Tracking.CNAMETarget == "" || rows[1].Redirect.Code != ErrCodeDomainRedirectTaken {
		t.Errorf("row 1 = %+v %+v", rows[1].Tracking, rows[1].Redirect)
	}
}

func TestSendingDomainSuggestionDecode(t *testing.T) {
	c := emailFixtureClient(t, http.StatusOK, `{"host": "link.acme.io", "status": "found", "cname_target": "track.warmbly.test",
		"vendor_domain": {"vendor": "zapmail", "connection_id": "vc_2", "can_forward": false, "can_unforward": false, "forwarding_reviewed": false, "can_dns": true, "dns_types": ["CNAME"]}}`, nil)
	s, _, err := c.Emails.TrackingSuggestion(context.Background(), "acme.io")
	if err != nil {
		t.Fatal(err)
	}
	if s.Status != TrackingSuggestionFound || s.VendorDomain == nil || !s.VendorDomain.CanDNS {
		t.Errorf("suggestion = %+v", s)
	}
}
