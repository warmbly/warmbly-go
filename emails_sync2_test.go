package warmbly

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const certsFixture = `{
  "smtp":{"host":"mail.example.com","port":465,"tls_unverified":true,"tls_pin":"` + pinA + `",
          "observed":{"fingerprint":"` + pinA + `","subject":"mail.example.com","issuer":"mail.example.com","not_after":"2027-01-02T03:04:05Z","self_signed":true}},
  "imap":{"host":"mail.example.com","port":993,"tls_unverified":false}
}`

const (
	pinA = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	pinB = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
)

func TestEmailCertificatesRoutes(t *testing.T) {
	ctx := context.Background()
	var got recordedRequest
	c := recordingFixtureClient(t, &got, certsFixture)

	certs, _, err := c.Emails.Certificates(ctx, "em 1")
	if err != nil {
		t.Fatal(err)
	}
	if got.method != "GET" || got.path != "/v1/emails/em 1/certificates" || got.rawQuery != "" || got.body != "" {
		t.Errorf("Certificates sent %s %s ?%s %q", got.method, got.path, got.rawQuery, got.body)
	}
	if !certs.SMTP.TLSUnverified || certs.SMTP.TLSPin != pinA || certs.SMTP.Port != 465 || certs.IMAP.TLSUnverified || certs.IMAP.TLSPin != "" {
		t.Errorf("decoded %+v", certs)
	}
	obs := certs.SMTP.Observed
	if obs == nil || obs.Fingerprint != pinA || !obs.SelfSigned || obs.Trusted || obs.NotAfter != "2027-01-02T03:04:05Z" {
		t.Errorf("observed = %+v", obs)
	}
	if certs.IMAP.Observed != nil {
		t.Errorf("IMAP.Observed = %+v, want nil", certs.IMAP.Observed)
	}

	got = recordedRequest{}
	if _, _, err := c.Emails.CheckCertificates(ctx, "em_1"); err != nil {
		t.Fatal(err)
	}
	if got.method != "POST" || got.path != "/v1/emails/onboarding/smtp-imap/em_1/certificates/check" || got.rawQuery != "" || strings.TrimSpace(got.body) != "" {
		t.Errorf("CheckCertificates sent %s %s ?%s %q", got.method, got.path, got.rawQuery, got.body)
	}

	got = recordedRequest{}
	out, _, err := c.Emails.ConfirmCertificates(ctx, "em_1", &ConfirmCertificatesParams{SMTPPin: pinA, IMAPPin: pinB})
	if err != nil {
		t.Fatal(err)
	}
	if got.method != "POST" || got.path != "/v1/emails/onboarding/smtp-imap/em_1/certificates/confirm" || got.rawQuery != "" {
		t.Errorf("ConfirmCertificates sent %s %s ?%s", got.method, got.path, got.rawQuery)
	}
	if want := `{"smtp_pin":"` + pinA + `","imap_pin":"` + pinB + `"}`; strings.TrimSpace(got.body) != want {
		t.Errorf("body = %s, want %s", got.body, want)
	}
	if out.SMTP.TLSPin != pinA {
		t.Errorf("decoded %+v", out)
	}
}

func TestMailboxCredentialsCarryTheCertificatePin(t *testing.T) {
	b, err := json.Marshal(&SMTPIMAPCredentialsParams{
		SMTP: &MailboxCredentials{Host: "m", Port: 465, Security: MailSecurityTLS, TLSUnverified: true, TLSPin: pinA},
		IMAP: &MailboxCredentials{Host: "m", Port: 993},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"tls_unverified":true,"tls_pin":"`+pinA+`"`) {
		t.Errorf("smtp leg lost the pin: %s", s)
	}
	if strings.Count(s, "tls_unverified") != 1 || strings.Count(s, "tls_pin") != 1 {
		t.Errorf("an unset pin must be omitted: %s", s)
	}
}

func TestErrorCarriesReasonAndRetryAfterHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/check") {
			w.Header().Set("Retry-After", "5")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"Service Unavailable","message":"We could not check this attempt right now.","code":"attempt_check_unavailable","request_id":"r2"}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Bad Request","message":"IMAP (mail.example.com:993) did not complete a secure connection.","code":"mailbox_tls_failed","reason":"certificate_untrusted","request_id":"r1"}`))
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithAPIKey("wmbly_test"), WithBaseURL(srv.URL+"/v1/"), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = c.Emails.ConfirmCertificates(context.Background(), "em_1", &ConfirmCertificatesParams{})
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v", err)
	}
	if !apiErr.HasCode(ErrCodeMailboxTLSFailed) || apiErr.Reason != ErrReasonCertificateUntrusted {
		t.Errorf("code %q reason %q", apiErr.Code, apiErr.Reason)
	}

	_, _, err = c.Emails.CheckCertificates(context.Background(), "em_1")
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v", err)
	}
	if !apiErr.HasCode(ErrCodeAttemptCheckUnavailable) || apiErr.RetryAfter != 5 || !apiErr.Temporary() || apiErr.Reason != "" {
		t.Errorf("got %+v", apiErr)
	}
}
