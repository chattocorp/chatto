package email_test

import (
	"strings"
	"testing"
	"time"

	smtpmock "github.com/mocktools/go-smtp-mock/v2"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/email"
)

func startMockServer(t *testing.T, cfg smtpmock.ConfigurationAttr) *smtpmock.Server {
	t.Helper()
	// Always enable MultipleMessageReceiving to preserve messages across RSET
	// (go-mail sends RSET after each message)
	cfg.MultipleMessageReceiving = true
	server := smtpmock.New(cfg)
	if err := server.Start(); err != nil {
		t.Fatalf("failed to start mock SMTP server: %v", err)
	}
	t.Cleanup(func() {
		if err := server.Stop(); err != nil {
			t.Errorf("failed to stop mock SMTP server: %v", err)
		}
	})
	return server
}

func TestMailer_Integration_SendSuccess(t *testing.T) {
	server := startMockServer(t, smtpmock.ConfigurationAttr{})

	mailer := email.NewMailer(config.SMTPConfig{
		Enabled: true,
		Host:    "127.0.0.1",
		Port:    server.PortNumber(),
		TLS:     config.SMTPTLSOpportunistic,
		From:    "sender@example.com",
	})

	err := mailer.Send(email.Message{
		To:      "recipient@example.com",
		Subject: "Test Subject",
		Body:    "Test body content",
	})
	if err != nil {
		t.Fatalf("Send() failed: %v", err)
	}

	// Use WaitForMessages to handle async connection processing
	// server.Messages() can return incomplete results due to async connection close
	messages, err := server.WaitForMessages(1, time.Second)
	if err != nil {
		t.Fatalf("WaitForMessages failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	msg := messages[0]

	// Verify message is consistent (MAIL FROM, RCPT TO, DATA all succeeded)
	if !msg.IsConsistent() {
		t.Error("expected message to be consistent")
	}

	// Verify MAIL FROM
	if !strings.Contains(msg.MailfromRequest(), "sender@example.com") {
		t.Errorf("expected MAIL FROM to contain sender@example.com, got %q", msg.MailfromRequest())
	}

	// Verify RCPT TO
	rcpttoData := msg.RcpttoRequestResponse()
	if len(rcpttoData) == 0 || !strings.Contains(rcpttoData[0][0], "recipient@example.com") {
		t.Errorf("expected RCPT TO to contain recipient@example.com, got %v", rcpttoData)
	}

	// Verify message data contains subject and body
	data := msg.MsgRequest()
	if !strings.Contains(data, "Test Subject") {
		t.Errorf("expected message to contain subject, got %q", data)
	}
	if !strings.Contains(data, "Test body content") {
		t.Errorf("expected message to contain body, got %q", data)
	}
}

func TestMailer_Integration_DefaultTLSRejectsPlaintextServer(t *testing.T) {
	server := startMockServer(t, smtpmock.ConfigurationAttr{})

	mailer := email.NewMailer(config.SMTPConfig{
		Enabled: true,
		Host:    "127.0.0.1",
		Port:    server.PortNumber(),
		From:    "sender@example.com",
	})

	err := mailer.Send(email.Message{
		To:      "recipient@example.com",
		Subject: "Secure Test",
		Body:    "Testing default TLS policy",
	})
	if err == nil {
		t.Fatal("expected error when plaintext server does not support STARTTLS, got nil")
	}

	if !strings.Contains(err.Error(), "STARTTLS") {
		t.Errorf("expected STARTTLS-related error, got %q", err.Error())
	}
}

func TestMailer_Integration_AuthNotSupported(t *testing.T) {
	// go-smtp-mock does not support SMTP AUTH
	// This test verifies that we get an appropriate error when auth is configured
	// but the server doesn't support it
	server := startMockServer(t, smtpmock.ConfigurationAttr{})

	mailer := email.NewMailer(config.SMTPConfig{
		Enabled:  true,
		Host:     "127.0.0.1",
		Port:     server.PortNumber(),
		TLS:      config.SMTPTLSOpportunistic,
		From:     "sender@example.com",
		Username: "testuser",
		Password: "testpass",
	})

	err := mailer.Send(email.Message{
		To:      "recipient@example.com",
		Subject: "Auth Test",
		Body:    "Testing with authentication",
	})

	// Should fail because mock server doesn't support AUTH
	if err == nil {
		t.Fatal("expected error when server doesn't support AUTH, got nil")
	}

	if !strings.Contains(err.Error(), "AUTH") {
		t.Errorf("expected AUTH-related error, got %q", err.Error())
	}
}

func TestMailer_Integration_ConnectionError(t *testing.T) {
	// Use a port that nothing is listening on
	mailer := email.NewMailer(config.SMTPConfig{
		Enabled: true,
		Host:    "127.0.0.1",
		Port:    59999, // Unlikely to be in use
		From:    "sender@example.com",
	})

	err := mailer.Send(email.Message{
		To:      "recipient@example.com",
		Subject: "Test",
		Body:    "Test",
	})
	if err == nil {
		t.Fatal("expected error for connection failure, got nil")
	}

	// Should contain some indication of connection failure
	if !strings.Contains(err.Error(), "failed to send email") {
		t.Errorf("expected error to indicate send failure, got %q", err.Error())
	}
}

func TestMailer_Integration_InvalidFromAddress(t *testing.T) {
	server := startMockServer(t, smtpmock.ConfigurationAttr{})

	mailer := email.NewMailer(config.SMTPConfig{
		Enabled: true,
		Host:    "127.0.0.1",
		Port:    server.PortNumber(),
		From:    "not-an-email",
	})

	err := mailer.Send(email.Message{
		To:      "recipient@example.com",
		Subject: "Test",
		Body:    "Test",
	})
	if err == nil {
		t.Fatal("expected error for invalid from address, got nil")
	}

	if !strings.Contains(err.Error(), "invalid from address") {
		t.Errorf("expected 'invalid from address' error, got %q", err.Error())
	}
}

func TestMailer_Integration_InvalidToAddress(t *testing.T) {
	server := startMockServer(t, smtpmock.ConfigurationAttr{})

	mailer := email.NewMailer(config.SMTPConfig{
		Enabled: true,
		Host:    "127.0.0.1",
		Port:    server.PortNumber(),
		TLS:     config.SMTPTLSOpportunistic,
		From:    "sender@example.com",
	})

	err := mailer.Send(email.Message{
		To:      "not-an-email",
		Subject: "Test",
		Body:    "Test",
	})
	if err == nil {
		t.Fatal("expected error for invalid to address, got nil")
	}

	if !strings.Contains(err.Error(), "invalid to address") {
		t.Errorf("expected 'invalid to address' error, got %q", err.Error())
	}
}

func TestMailer_Integration_MultipleMessages(t *testing.T) {
	server := startMockServer(t, smtpmock.ConfigurationAttr{})

	mailer := email.NewMailer(config.SMTPConfig{
		Enabled: true,
		Host:    "127.0.0.1",
		Port:    server.PortNumber(),
		TLS:     config.SMTPTLSOpportunistic,
		From:    "sender@example.com",
	})

	// Send multiple messages
	for i := range 3 {
		err := mailer.Send(email.Message{
			To:      "recipient@example.com",
			Subject: "Test",
			Body:    "Test",
		})
		if err != nil {
			t.Fatalf("Send() %d failed: %v", i, err)
		}
	}

	// Use WaitForMessages to handle async connection processing
	// This is the recommended approach per go-smtp-mock docs
	messages, err := server.WaitForMessages(3, time.Second)
	if err != nil {
		t.Fatalf("WaitForMessages failed: %v", err)
	}
	if len(messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(messages))
	}
}

func TestMailer_Integration_BlacklistedRecipient(t *testing.T) {
	server := startMockServer(t, smtpmock.ConfigurationAttr{
		BlacklistedRcpttoEmails: []string{"blocked@example.com"},
	})

	mailer := email.NewMailer(config.SMTPConfig{
		Enabled: true,
		Host:    "127.0.0.1",
		Port:    server.PortNumber(),
		TLS:     config.SMTPTLSOpportunistic,
		From:    "sender@example.com",
	})

	err := mailer.Send(email.Message{
		To:      "blocked@example.com",
		Subject: "Test",
		Body:    "Test",
	})

	if err == nil {
		t.Fatal("expected error for blacklisted recipient, got nil")
	}
}
