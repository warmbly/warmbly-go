package warmbly

import (
	"context"
	"errors"
	"testing"
	"time"
)

func sampleSendIdentity() SendIdentity {
	synced := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	imported := time.Date(2026, 9, 1, 8, 5, 0, 0, time.UTC)
	return SendIdentity{
		Supported:    true,
		Provider:     ProviderGmail,
		MailboxEmail: "sam@acme.com",
		SendAsEmail:  "sales@acme.com",
		Identities: []SendAsIdentity{
			{Email: "sam@acme.com", Name: "Sam", IsPrimary: true, IsDefault: true, Verified: true},
			{Email: "sales@acme.com", Name: "Acme Sales", Verified: true},
			{Email: "old@acme.com", Name: "Old", Verified: false},
		},
		SyncedAt:            &synced,
		SignatureSource:     SignatureSourceProvider,
		SignatureImportedAt: &imported,
	}
}

func TestEmailIdentity(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, jsonOf(t, sampleSendIdentity()))
	out, _, err := c.Emails.Identity(context.Background(), "acc_1")
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "GET", "/v1/emails/acc_1/identity", "")
	got.wantBody(t, "")
	if !out.Supported || out.SendAsEmail != "sales@acme.com" || len(out.Identities) != 3 ||
		!out.Identities[0].IsPrimary || out.Identities[2].Verified || out.SyncedAt == nil ||
		out.SignatureSource != SignatureSourceProvider || out.SignatureImportedAt == nil {
		t.Errorf("identity = %+v", out)
	}
}

func TestEmailIdentityUnsupportedProviderDecodes(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, `{"supported":false,"provider":"smtp_imap","mailbox_email":"a@b.com","send_as_email":"","identities":[],"signature_source":"manual"}`)
	out, _, err := c.Emails.Identity(context.Background(), "acc_2")
	if err != nil {
		t.Fatal(err)
	}
	if out.Supported || out.SyncedAt != nil || out.SignatureImportedAt != nil || len(out.Identities) != 0 || out.SignatureSource != SignatureSourceManual {
		t.Errorf("identity = %+v", out)
	}
}

func TestEmailRefreshIdentity(t *testing.T) {
	t.Run("addresses only sends no body", func(t *testing.T) {
		var got syncCapture
		c := respondingClient(t, &got, 200, jsonOf(t, sampleSendIdentity()))
		out, _, err := c.Emails.RefreshIdentity(context.Background(), "acc_1", nil)
		if err != nil {
			t.Fatal(err)
		}
		got.wantRequest(t, "POST", "/v1/emails/acc_1/identity/refresh", "")
		got.wantBody(t, "")
		if len(out.Identities) != 3 {
			t.Errorf("identity = %+v", out)
		}
	})
	t.Run("signature import is opt-in", func(t *testing.T) {
		var got syncCapture
		c := respondingClient(t, &got, 200, jsonOf(t, sampleSendIdentity()))
		if _, _, err := c.Emails.RefreshIdentity(context.Background(), "acc_1", &SendIdentityRefreshParams{ImportSignature: true}); err != nil {
			t.Fatal(err)
		}
		got.wantBody(t, `{"import_signature":true}`)
	})
	t.Run("unsupported provider is a typed error", func(t *testing.T) {
		var got syncCapture
		c := respondingClient(t, &got, 400, `{"error":"Bad Request","message":"This mailbox's provider does not expose send-as addresses.","code":"mailbox_send_as_unsupported"}`)
		_, _, err := c.Emails.RefreshIdentity(context.Background(), "acc_9", nil)
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.StatusCode != 400 || !apiErr.HasCode("mailbox_send_as_unsupported") {
			t.Fatalf("err = %v, want a 400 carrying mailbox_send_as_unsupported", err)
		}
	})
}

func TestEmailSetDirectTracking(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		var got syncCapture
		c := respondingClient(t, &got, 200, jsonOf(t, map[string]bool{"track_direct_mail": enabled}))
		stored, _, err := c.Emails.SetDirectTracking(context.Background(), "acc_1", enabled)
		if err != nil {
			t.Fatal(err)
		}
		got.wantRequest(t, "PATCH", "/v1/emails/acc_1/direct-tracking", "")
		if enabled {
			got.wantBody(t, `{"enabled":true}`)
		} else {
			got.wantBody(t, `{"enabled":false}`)
		}
		if stored != enabled {
			t.Errorf("stored = %v, want %v", stored, enabled)
		}
	}
}

func TestEmailUpdateCarriesNewSettings(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, jsonOf(t, Email{
		ID: "acc_1", SendAsEmail: "sales@acme.com", MailHost: MailHostGoogleWorkspace, AuthMethod: MailAuthDelegated,
		TrackDirectMail: true, RelayFolderMoves: true, WarmupPlacement: WarmupPlacementArchive, WarmupFolder: "", WarmupRetentionDays: 14,
		AvatarURL: "https://cdn.example.com/a.png", Vendor: "inboxkit",
	}))
	out, _, err := c.Emails.Update(context.Background(), "acc_1", &EmailUpdateParams{
		SendAsEmail:         String("sales@acme.com"),
		WarmupPlacement:     String(WarmupPlacementArchive),
		WarmupFolder:        String(""),
		WarmupRetentionDays: Int(14),
		RelayFolderMoves:    Bool(false),
	})
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "PATCH", "/v1/emails/acc_1", "")
	got.wantBody(t, `{"send_as_email":"sales@acme.com","warmup_placement":"archive","warmup_folder":"","warmup_retention_days":14,"relay_folder_moves":false}`)
	if out.SendAsEmail != "sales@acme.com" || out.MailHost != MailHostGoogleWorkspace || out.AuthMethod != MailAuthDelegated ||
		!out.TrackDirectMail || !out.RelayFolderMoves || out.WarmupPlacement != WarmupPlacementArchive ||
		out.WarmupRetentionDays != 14 || out.AvatarURL == "" || out.Vendor != "inboxkit" {
		t.Errorf("email = %+v", out)
	}
}

func TestEmailUpdateClearsSendAs(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, `{"id":"acc_1"}`)
	if _, _, err := c.Emails.Update(context.Background(), "acc_1", &EmailUpdateParams{SendAsEmail: String("")}); err != nil {
		t.Fatal(err)
	}
	got.wantBody(t, `{"send_as_email":""}`)
}

func TestEmailDecodesUnknownMailHostAndAuthMethod(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, `{"id":"acc_1","mail_host":"brand_new_host","auth_method":"passkey","domain_grant_id":"g_1","vendor_connection_id":"v_1"}`)
	out, _, err := c.Emails.Get(context.Background(), "acc_1")
	if err != nil {
		t.Fatal(err)
	}
	if out.MailHost != "brand_new_host" || out.AuthMethod != "passkey" || out.DomainGrantID == nil || *out.DomainGrantID != "g_1" ||
		out.VendorConnectionID == nil {
		t.Errorf("email = %+v", out)
	}
}
