package warmbly

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestSelfhostWorkspaceLinkSync(t *testing.T) {
	ctx := context.Background()
	// A cloud-link mailbox row (models.CloudLinkMailboxRow) whose cloud state
	// carries the participation and standing fields added since the last sync.
	const row = `{
		"legacy": false,
		"id": "0c0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b",
		"email": "ops@acme.com",
		"name": "Ops",
		"provider": "smtp_imap",
		"status": "active",
		"enrolled": true,
		"enrolled_at": "2026-09-01T00:00:00Z",
		"managed": false,
		"enrollment_state": "active",
		"standing_observed_at": "2026-10-11T09:55:00Z",
		"cloud": {
			"participation": {"mode": "diagnostic", "send": true, "receive": false, "rolling_recipient_limit": 60},
			"consent_completed_at": "2026-09-01T00:00:00Z",
			"remote_id": "0c0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b",
			"email_account_id": "9a9b9c9d-0000-4111-8222-933344455566",
			"email": "ops@acme.com", "name": "Ops", "provider": "smtp_imap", "status": "active",
			"enrolled_at": "2026-09-01T00:00:00Z", "managed": false,
			"health": {"pool_type": "premium", "source": "cloud", "state": "watch", "score": 71.5, "spam_score": 0},
			"sent_today": 4, "sent_7d": 30, "replied_7d": 9, "spam_placed_7d": 1,
			"auth_state": "passing",
			"settings": {"base": 2, "max": 40, "increase": 2, "reply_rate": 30, "start_time": "09:00", "end_time": "17:00", "days": 31, "timezone": "UTC"}
		}
	}`
	const status = `{
		"legacy_connected": true,
		"connected": true,
		"link": {"organization_id": "1d2e3f40-1111-4222-8333-944455566677", "cloud_url": "https://api.warmbly.com", "instance_id": "8f8e8d8c-0000-4111-8222-933344455566",
			"organization_name": "Acme Cloud", "connected_at": "2026-09-01T00:00:00Z", "disconnect_pending": true},
		"info": {
			"managed_consent_protocol": 1,
			"instance": {"remote_organization_id": "1d2e3f40-1111-4222-8333-944455566677", "id": "8f8e8d8c-0000-4111-8222-933344455566",
				"organization_id": "aaaa0000-1111-4222-8333-944455566677", "name": "box", "url": "https://box.example", "version": "1.4.0",
				"created_at": "2026-09-01T00:00:00Z", "mailbox_count": 3},
			"organization": {"id": "aaaa0000-1111-4222-8333-944455566677", "name": "Acme Cloud"},
			"plan": {"tier": "paid", "mailbox_limit": null, "enrolled": 3, "warming": 2, "price_usd": 29, "manage_url": "https://app.warmbly.com/app/settings/billing", "warmup_entitled": true},
			"redirects": {"available": true, "host": "r.warmbly.com", "limit": 5, "used": 1}
		},
		"reachable": true,
		"default_cloud_url": "https://api.warmbly.com"
	}`

	runWireCases(t, []wireCase{
		{
			name: "cloudLink.Status", response: status,
			call: func(c *Client) error {
				st, _, err := c.CloudLink.Status(ctx)
				if err != nil {
					return err
				}
				if !st.LegacyConnected || st.Link == nil || st.Link.OrganizationID == nil || !st.Link.DisconnectPending {
					return errors.New("link not decoded")
				}
				i := st.Info
				if i == nil || i.ManagedConsentProtocol != 1 || i.Redirects == nil || i.Redirects.Limit != 5 ||
					i.Instance.RemoteOrganizationID == nil || i.Plan.Warming != 2 || i.Plan.ManageURL == "" {
					return errors.New("info not decoded")
				}
				return nil
			},
			method: "GET", path: "/v1/cloud-link",
		},
		{
			name: "cloudLink.SetParticipation", response: row,
			call: func(c *Client) error {
				m, _, err := c.CloudLink.SetParticipation(ctx, "em_1", &DiagnosticParticipation{
					Mode: TestParticipationDiagnostic, Send: true, RollingRecipientLimit: Int(60),
				})
				if err != nil {
					return err
				}
				if m.EnrollmentState != CloudEnrollmentActive || m.StandingObservedAt == nil || m.Cloud == nil ||
					m.Cloud.Participation == nil || !m.Cloud.Participation.Send || m.Cloud.ConsentCompletedAt == nil ||
					m.Cloud.Health == nil || m.Cloud.Health.Source != "cloud" || m.Cloud.Health.PoolType != "premium" {
					return errors.New("row not decoded")
				}
				return nil
			},
			method: "PATCH", path: "/v1/cloud-link/mailboxes/em_1/participation",
			body: `{"mode":"diagnostic","send":true,"receive":false,"rolling_recipient_limit":60}`,
		},
		{
			name: "cloudLink.Disconnect", status: http.StatusNoContent,
			call: func(c *Client) error {
				_, err := c.CloudLink.Disconnect(ctx)
				return err
			},
			method: "DELETE", path: "/v1/cloud-link",
		},
		{
			name: "cloudLink.DisconnectLegacy", status: http.StatusNoContent,
			call: func(c *Client) error {
				_, err := c.CloudLink.DisconnectLegacy(ctx)
				return err
			},
			method: "DELETE", path: "/v1/cloud-link", rawQuery: "legacy=true",
		},
		{
			name: "poolLink.StartCode", response: `{"workspace_scoped": true, "device_code": "dev_1", "user_code": "ABCD-EFGH", "verification_url": "https://app.warmbly.com/link?code=ABCD-EFGH", "expires_in": 900, "interval": 5}`,
			call: func(c *Client) error {
				g, _, err := c.PoolLink.StartCode(ctx, &PoolLinkStartParams{RemoteOrganizationID: "org_r", InstanceName: "box"})
				if err == nil && !g.WorkspaceScoped {
					err = errors.New("workspace_scoped not decoded")
				}
				return err
			},
			method: "POST", path: "/v1/pool-link/codes",
			body: `{"remote_organization_id":"org_r","instance_name":"box","instance_url":"","instance_version":""}`,
		},
		{
			name: "poolLink.DescribeCode", response: `{"remote_organization_id": "1d2e3f40-1111-4222-8333-944455566677", "id": "c1", "user_code": "ABCD-EFGH",
				"instance_name": "box", "instance_url": "", "instance_version": "1.4.0", "status": "pending",
				"expires_at": "2026-10-11T10:15:00Z", "created_at": "2026-10-11T10:00:00Z"}`,
			call: func(c *Client) error {
				code, _, err := c.PoolLink.DescribeCode(ctx, "ABCD-EFGH")
				if err == nil && code.RemoteOrganizationID == nil {
					err = errors.New("remote_organization_id not decoded")
				}
				return err
			},
			method: "GET", path: "/v1/pool-link/codes/ABCD-EFGH",
		},
		{
			name: "poolLink.Offer", response: `{"available": true, "monthly_available": true, "yearly_available": false, "monthly_usd": 29.5, "yearly_usd": 0, "currency": "usd"}`,
			call: func(c *Client) error {
				o, _, err := c.PoolLink.Offer(ctx)
				if err == nil && (!o.Available || !o.MonthlyAvailable || o.YearlyAvailable || o.MonthlyUSD != 29.5 || o.Currency != "usd") {
					err = errors.New("offer not decoded")
				}
				return err
			},
			method: "GET", path: "/v1/pool-link/offer",
		},
		{
			name: "poolLink.Checkout yearly", response: `{"checkout_url": "https://checkout.stripe.com/c/pay_1"}`,
			call: func(c *Client) error {
				u, _, err := c.PoolLink.Checkout(ctx, PoolLinkIntervalYear)
				if err == nil && u != "https://checkout.stripe.com/c/pay_1" {
					err = errors.New("checkout_url not decoded")
				}
				return err
			},
			method: "POST", path: "/v1/pool-link/checkout", body: `{"interval":"year"}`,
		},
		{
			name: "poolLink.Checkout default period", response: `{"checkout_url": "https://checkout.stripe.com/c/pay_2"}`,
			call: func(c *Client) error {
				_, _, err := c.PoolLink.Checkout(ctx, "")
				return err
			},
			method: "POST", path: "/v1/pool-link/checkout", body: `{}`,
		},
	})
}

func TestSelfhostLegacyLinkRefusal(t *testing.T) {
	c := emailFixtureClient(t, http.StatusConflict, `{"code":"cloud_link_workspace_required","message":"This connection is shared across this server's workspaces."}`, nil)
	_, err := c.CloudLink.Disconnect(context.Background())
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "cloud_link_workspace_required" {
		t.Fatalf("err = %v", err)
	}
}
