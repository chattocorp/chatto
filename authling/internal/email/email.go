// Package email sends Authling transactional email without exposing recipient
// values to logs or callers beyond the delivery boundary.
package email

import (
	"context"
	"crypto/tls"
	"fmt"

	mail "github.com/wneessen/go-mail"
	"hmans.de/authling/internal/config"
)

// Message is one plain-text transactional email.
type Message struct { To, Subject, Body string }

// Sender is the delivery seam used by account registration.
type Sender interface { SendContext(context.Context, Message) error }

// Mailer delivers through configured SMTP.
type Mailer struct{ config config.SMTPConfig }

func NewMailer(cfg config.SMTPConfig) *Mailer { return &Mailer{config: cfg} }

func (m *Mailer) SendContext(ctx context.Context, msg Message) error {
	if !m.config.Enabled { return fmt.Errorf("SMTP is not enabled") }
	message := mail.NewMsg()
	if err := message.From(m.config.From); err != nil { return fmt.Errorf("invalid SMTP from address: %w", err) }
	if err := message.To(msg.To); err != nil { return fmt.Errorf("invalid SMTP recipient: %w", err) }
	message.Subject(msg.Subject)
	message.SetBodyString(mail.TypeTextPlain, msg.Body)
	opts := []mail.Option{mail.WithPort(m.config.Port), mail.WithHELO("localhost")}
	switch m.config.TLSPolicyOrDefault() {
	case config.SMTPTLSImplicit: opts = append(opts, mail.WithSSL())
	case config.SMTPTLSOpportunistic: opts = append(opts, mail.WithTLSPortPolicy(mail.TLSOpportunistic))
	default: opts = append(opts, mail.WithTLSPortPolicy(mail.TLSMandatory))
	}
	if m.config.TLSSkipVerify || m.config.TLSServerName != "" {
		serverName := m.config.Host
		if m.config.TLSServerName != "" { serverName = m.config.TLSServerName }
		opts = append(opts, mail.WithTLSConfig(&tls.Config{ServerName: serverName, InsecureSkipVerify: m.config.TLSSkipVerify, MinVersion: tls.VersionTLS12}))
	}
	if m.config.Username != "" || m.config.Password != "" {
		opts = append(opts, mail.WithSMTPAuth(mail.SMTPAuthPlain), mail.WithUsername(m.config.Username), mail.WithPassword(m.config.Password))
	}
	client, err := mail.NewClient(m.config.Host, opts...)
	if err != nil { return fmt.Errorf("create SMTP client: %w", err) }
	if err := client.DialAndSendWithContext(ctx, message); err != nil { return fmt.Errorf("send SMTP message: %w", err) }
	return nil
}
