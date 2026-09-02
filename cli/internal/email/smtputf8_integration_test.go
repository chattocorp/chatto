package email_test

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/email"
)

func TestMailer_SMTPUTF8Usage(t *testing.T) {
	tests := []struct {
		name         string
		from         string
		message      email.Message
		wantSMTPUTF8 bool
	}{
		{
			name: "ASCII envelope and headers omit SMTPUTF8",
			from: "sender@example.com",
			message: email.Message{
				To:      "recipient@example.com",
				Subject: "Verification code",
				Body:    "Unicode body content is MIME-safe: Grüß dich.",
			},
		},
		{
			name: "UTF-8 subject is MIME-encoded and omits SMTPUTF8",
			from: "sender@example.com",
			message: email.Message{
				To:      "recipient@example.com",
				Subject: "Bestätigungscode",
				Body:    "Test body",
			},
		},
		{
			name: "internationalized sender requires SMTPUTF8",
			from: "absenderü@example.com",
			message: email.Message{
				To:      "recipient@example.com",
				Subject: "Verification code",
				Body:    "Test body",
			},
			wantSMTPUTF8: true,
		},
		{
			name: "internationalized recipient requires SMTPUTF8",
			from: "sender@example.com",
			message: email.Message{
				To:      "empfänger@example.com",
				Subject: "Verification code",
				Body:    "Test body",
			},
			wantSMTPUTF8: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port, mailFrom := startSMTPUTF8Server(t)
			mailer := email.NewMailer(config.SMTPConfig{
				Enabled: true,
				Host:    "127.0.0.1",
				Port:    port,
				TLS:     config.SMTPTLSOpportunistic,
				From:    tt.from,
			})

			if err := mailer.Send(tt.message); err != nil {
				t.Fatalf("Send() failed: %v", err)
			}
			command := receiveMailFrom(t, mailFrom)
			if got := strings.Contains(command, " SMTPUTF8"); got != tt.wantSMTPUTF8 {
				t.Errorf("MAIL FROM SMTPUTF8 presence = %v, want %v; command: %q", got, tt.wantSMTPUTF8, command)
			}
		})
	}
}

func startSMTPUTF8Server(t *testing.T) (int, <-chan string) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for SMTP: %v", err)
	}
	mailFrom := make(chan string, 1)
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- serveSMTPUTF8(listener, mailFrom) }()
	t.Cleanup(func() {
		_ = listener.Close()
		select {
		case err := <-serverErrors:
			if err != nil {
				t.Errorf("SMTP server failed: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("SMTP server did not stop")
		}
	})
	return listener.Addr().(*net.TCPAddr).Port, mailFrom
}

func serveSMTPUTF8(listener net.Listener, mailFrom chan<- string) error {
	connection, err := listener.Accept()
	if err != nil {
		return err
	}
	defer connection.Close()
	reader := bufio.NewReader(connection)
	if _, err := fmt.Fprint(connection, "220 localhost ESMTP test server\r\n"); err != nil {
		return err
	}
	inData := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if inData {
			if line == "." {
				inData = false
				if _, err := fmt.Fprint(connection, "250 2.0.0 queued\r\n"); err != nil {
					return err
				}
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "EHLO "):
			_, err = fmt.Fprint(connection, "250-localhost\r\n250-8BITMIME\r\n250 SMTPUTF8\r\n")
		case strings.HasPrefix(line, "MAIL FROM:"):
			mailFrom <- line
			_, err = fmt.Fprint(connection, "250 2.1.0 sender accepted\r\n")
		case strings.HasPrefix(line, "RCPT TO:"):
			_, err = fmt.Fprint(connection, "250 2.1.5 recipient accepted\r\n")
		case line == "DATA":
			inData = true
			_, err = fmt.Fprint(connection, "354 End data with <CR><LF>.<CR><LF>\r\n")
		case line == "RSET":
			_, err = fmt.Fprint(connection, "250 2.0.0 reset\r\n")
		case line == "NOOP":
			_, err = fmt.Fprint(connection, "250 2.0.0 ok\r\n")
		case line == "QUIT":
			_, err = fmt.Fprint(connection, "221 2.0.0 bye\r\n")
			return err
		default:
			_, err = fmt.Fprint(connection, "500 5.5.1 unsupported command\r\n")
		}
		if err != nil {
			return err
		}
	}
}

func receiveMailFrom(t *testing.T, mailFrom <-chan string) string {
	t.Helper()
	select {
	case command := <-mailFrom:
		return command
	case <-time.After(time.Second):
		t.Fatal("SMTP server did not receive MAIL FROM")
		return ""
	}
}
