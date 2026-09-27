// Package email sends Authling transactional email without exposing recipient
// values to logs or callers beyond the delivery boundary.
package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"

	mail "github.com/wneessen/go-mail"
	"hmans.de/authling/internal/config"
)

// Message is one plain-text transactional email.
type Message struct{ To, Subject, Body string }

// Sender is the delivery seam used by account registration.
type Sender interface {
	SendContext(context.Context, Message) error
}

// Mailer delivers through configured SMTP.
type Mailer struct{ config config.SMTPConfig }

func NewMailer(cfg config.SMTPConfig) *Mailer { return &Mailer{config: cfg} }

func (m *Mailer) SendContext(ctx context.Context, msg Message) error {
	if !m.config.Enabled {
		return fmt.Errorf("SMTP is not enabled")
	}
	message := mail.NewMsg()
	// Address parse errors repeat the address, so do not wrap them.
	if err := message.From(m.config.From); err != nil {
		return errors.New("invalid SMTP from address")
	}
	if err := message.To(msg.To); err != nil {
		return errors.New("invalid SMTP recipient")
	}
	message.Subject(msg.Subject)
	message.SetBodyString(mail.TypeTextPlain, msg.Body)
	opts := []mail.Option{mail.WithPort(m.config.Port), mail.WithHELO("localhost")}
	if !messageRequiresSMTPUTF8(message) {
		opts = append(opts, mail.WithoutSMTPUTF8())
	}
	switch m.config.TLSPolicyOrDefault() {
	case config.SMTPTLSImplicit:
		opts = append(opts, mail.WithSSL())
	case config.SMTPTLSOpportunistic:
		opts = append(opts, mail.WithTLSPortPolicy(mail.TLSOpportunistic))
	default:
		opts = append(opts, mail.WithTLSPortPolicy(mail.TLSMandatory))
	}
	if m.config.TLSSkipVerify || m.config.TLSServerName != "" {
		serverName := m.config.Host
		if m.config.TLSServerName != "" {
			serverName = m.config.TLSServerName
		}
		opts = append(opts, mail.WithTLSConfig(&tls.Config{ServerName: serverName, InsecureSkipVerify: m.config.TLSSkipVerify, MinVersion: tls.VersionTLS12}))
	}
	if m.config.Username != "" || m.config.Password != "" {
		opts = append(opts, mail.WithSMTPAuth(mail.SMTPAuthPlain), mail.WithUsername(m.config.Username), mail.WithPassword(m.config.Password))
	}
	client, err := mail.NewClient(m.config.Host, opts...)
	if err != nil {
		return fmt.Errorf("create SMTP client: %w", err)
	}
	if err := client.DialAndSendWithContext(ctx, message); err != nil {
		return fmt.Errorf("send SMTP message: %w", redactSendError(err))
	}
	return nil
}

// redactSendError removes the recipient addresses, message ID, and SMTP server
// replies that go-mail includes in a SendError. Server replies often repeat the
// rejected address. Dial, TLS, and authentication errors do not contain message
// data and are returned unchanged.
func redactSendError(err error) error {
	var sendErr *mail.SendError
	if !errors.As(err, &sendErr) {
		return err
	}
	return fmt.Errorf("%s failed (code %d, enhanced status %q, temporary %t)",
		sendErr.Reason, sendErr.ErrorCode(), sendErr.EnhancedStatusCode(), sendErr.IsTemp())
}

// messageRequiresSMTPUTF8 reports whether the SMTP envelope or a stored message
// header contains non-ASCII data. UTF-8 MIME body content does not require the
// SMTPUTF8 extension.
func messageRequiresSMTPUTF8(message *mail.Msg) bool {
	sender, err := message.GetSender(false)
	if err != nil || !isASCII(sender) {
		return true
	}
	recipients, err := message.GetRecipients()
	if err != nil {
		return true
	}
	for _, recipient := range recipients {
		if !isASCII(recipient) {
			return true
		}
	}
	for _, subject := range message.GetGenHeader(mail.HeaderSubject) {
		if !isASCII(subject) {
			return true
		}
	}
	return false
}

func isASCII(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] >= 0x80 {
			return false
		}
	}
	return true
}
