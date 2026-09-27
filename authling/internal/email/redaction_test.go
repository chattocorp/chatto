package email_test

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"

	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/email"
)

func TestMailerErrorsOmitAddresses(t *testing.T) {
	t.Run("invalid recipient", func(t *testing.T) {
		mailer := email.NewMailer(config.SMTPConfig{Enabled: true, Host: "127.0.0.1", Port: 25, From: "sender@example.com"})
		err := mailer.SendContext(t.Context(), email.Message{To: "not-an-address", Subject: "Code", Body: "Body"})
		if err == nil {
			t.Fatal("expected invalid recipient error")
		}
		if strings.Contains(err.Error(), "not-an-address") {
			t.Errorf("error repeats the submitted address: %q", err.Error())
		}
	})

	t.Run("rejected recipient", func(t *testing.T) {
		port := startRejectingRecipientServer(t)
		mailer := email.NewMailer(config.SMTPConfig{
			Enabled: true,
			Host:    "127.0.0.1",
			Port:    port,
			TLS:     config.SMTPTLSOpportunistic,
			From:    "sender@example.com",
		})
		err := mailer.SendContext(t.Context(), email.Message{To: "rejected@example.com", Subject: "Code", Body: "Body"})
		if err == nil {
			t.Fatal("expected rejected recipient error")
		}
		if strings.Contains(err.Error(), "rejected@example.com") {
			t.Errorf("error contains the recipient address: %q", err.Error())
		}
		if !strings.Contains(err.Error(), "RCPT TO") || !strings.Contains(err.Error(), "550") {
			t.Errorf("expected the failed SMTP command and code in the error, got %q", err.Error())
		}
	})
}

// startRejectingRecipientServer starts a plaintext SMTP server that rejects
// every recipient with a reply that repeats the address, as many servers do.
func startRejectingRecipientServer(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for SMTP: %v", err)
	}
	done := make(chan struct{})
	t.Cleanup(func() {
		_ = listener.Close()
		<-done
	})
	go func() {
		defer close(done)
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		reader := bufio.NewReader(connection)
		_, _ = fmt.Fprint(connection, "220 localhost ESMTP test server\r\n")
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			switch {
			case strings.HasPrefix(line, "EHLO "):
				_, err = fmt.Fprint(connection, "250 localhost\r\n")
			case strings.HasPrefix(line, "RCPT TO:"):
				address := strings.TrimPrefix(line, "RCPT TO:")
				_, err = fmt.Fprintf(connection, "550 5.1.1 %s: Recipient address rejected\r\n", address)
			case line == "QUIT":
				_, _ = fmt.Fprint(connection, "221 2.0.0 bye\r\n")
				return
			default:
				_, err = fmt.Fprint(connection, "250 2.0.0 ok\r\n")
			}
			if err != nil {
				return
			}
		}
	}()
	return listener.Addr().(*net.TCPAddr).Port
}
