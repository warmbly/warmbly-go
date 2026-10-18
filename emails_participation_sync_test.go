package warmbly

import (
	"context"
	"errors"
	"testing"
)

func TestEmailTestParticipationSync(t *testing.T) {
	ctx := context.Background()
	const mailbox = `{
		"id": "0c0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b",
		"email": "sales@acme.com",
		"provider": "gmail",
		"status": "active",
		"campaign_limit": 30,
		"test_mode": "diagnostic",
		"test_send_enabled": true,
		"test_receive_enabled": false,
		"shared_daily_limit": 80,
		"rolling_recipient_limit": null
	}`
	runAuthWireCases(t, []authWireCase{
		{
			name: "Get decodes participation and caps", response: mailbox,
			call: func(c *Client) error {
				e, _, err := c.Emails.Get(ctx, "em_1")
				if err == nil && (e.TestMode == nil || *e.TestMode != TestParticipationDiagnostic || !e.TestSendEnabled ||
					e.TestReceiveEnabled || e.SharedDailyLimit == nil || *e.SharedDailyLimit != 80 || e.RollingRecipientLimit != nil) {
					err = errors.New("participation not decoded")
				}
				return err
			},
			method: "GET", path: "/v1/emails/em_1",
		},
		{
			name: "Update sends participation, caps and a recovery resolution", response: mailbox,
			call: func(c *Client) error {
				_, _, err := c.Emails.Update(ctx, "em_1", &EmailUpdateParams{
					TestMode:              String(TestParticipationOff),
					TestSendEnabled:       Bool(false),
					SharedDailyLimit:      Int(0),
					RollingRecipientLimit: Int(120),
					SendRecoveryResolution: &SendRecoveryResolution{
						HeldTaskID:            "task_1",
						HeldReason:            SendHeldUnknown,
						EvidenceType:          SendEvidenceConfirmedSent,
						ConfirmationReference: "ticket 42",
						MessageID:             "<m1@acme.com>",
					},
				})
				return err
			},
			method: "PATCH", path: "/v1/emails/em_1", jsonEqual: true,
			body: `{"test_mode":"off","test_send_enabled":false,"shared_daily_limit":0,"rolling_recipient_limit":120,
				"send_recovery_resolution":{"held_task_id":"task_1","held_reason":"unknown","evidence_type":"operator_confirmed_sent",
				"confirmation_reference":"ticket 42","message_id":"<m1@acme.com>"}}`,
		},
		{
			name: "SyncStatus decodes a Gmail recovery cursor",
			response: `{"state": {"backfill_cursor": {"google_recovery": {"history_id": 991, "since": "2026-10-10T00:00:00Z", "messages_done": true}}},
				"policy": {}, "skip_folders": [], "folders": []}`,
			call: func(c *Client) error {
				s, _, err := c.Emails.SyncStatus(ctx, "em_1")
				if err == nil && (s.State == nil || s.State.BackfillCursor.GoogleRecovery == nil ||
					s.State.BackfillCursor.GoogleRecovery.HistoryID != 991 || !s.State.BackfillCursor.GoogleRecovery.MessagesDone) {
					err = errors.New("google recovery not decoded")
				}
				return err
			},
			method: "GET", path: "/v1/emails/em_1/sync",
		},
	})
}
