package warmbly

import (
	"context"
	"net/http"
	"testing"
)

func TestCloudLinkParticipation(t *testing.T) {
	var got recordedRequest
	c := emailFixtureClient(t, http.StatusOK, `{
		"legacy": false, "id": "em_1", "email": "ana@acme.io", "name": "Ana", "provider": "smtp_imap", "status": "active",
		"enrolled": true, "enrolled_at": "2026-11-01T08:00:00Z", "managed": false
	}`, &got)
	ctx := context.Background()

	box, _, err := c.CloudLink.SetMailboxParticipation(ctx, "em_1", &MailboxParticipation{
		Mode: TestModeDiagnostic, Send: true, Receive: false, SharedDailyLimit: Int(40),
	})
	if err != nil {
		t.Fatal(err)
	}
	checkRequest(t, got, "PATCH", "/v1/cloud-link/mailboxes/em_1/participation", "",
		`{"mode":"diagnostic","send":true,"receive":false,"shared_daily_limit":40}`)
	if box.ID != "em_1" || !box.Enrolled || box.EnrolledAt == nil {
		t.Errorf("mailbox = %+v", box)
	}

	if _, _, err := c.CloudLink.SetMailboxParticipation(ctx, "em_1", &MailboxParticipation{Mode: TestModeOff}); err != nil {
		t.Fatal(err)
	}
	checkRequest(t, got, "PATCH", "/v1/cloud-link/mailboxes/em_1/participation", "", `{"mode":"off","send":false,"receive":false}`)
}
