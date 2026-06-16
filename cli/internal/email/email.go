// Package email provides transactional email sending functionality.
package email

import (
	"context"
	"errors"
	"fmt"

	"github.com/wneessen/go-mail"

	"hmans.de/chatto/internal/config"
)

// ErrSMTPDisabled is returned when attempting to send email with SMTP disabled.
var ErrSMTPDisabled = errors.New("SMTP is not enabled")

// Message represents an email message to be sent.
type Message struct {
	To      string
	Subject string
	Body    string
}

// Sender is the interface for sending emails. Implemented by Mailer.
// Use this interface in components that need to send emails to enable testing.
type Sender interface {
	Send(msg Message) error
	SendContext(ctx context.Context, msg Message) error
	IsEnabled() bool
}

// Mailer handles sending transactional emails via SMTP.
type Mailer struct {
	config config.SMTPConfig
}

// Verify Mailer implements Sender at compile time.
var _ Sender = (*Mailer)(nil)

// NewMailer creates a new Mailer with the given SMTP configuration.
func NewMailer(cfg config.SMTPConfig) *Mailer {
	return &Mailer{config: cfg}
}

// Send sends an email message. Returns ErrSMTPDisabled if SMTP is not enabled.
func (m *Mailer) Send(msg Message) error {
	return m.SendContext(context.Background(), msg)
}

// SendContext sends an email message with context support.
func (m *Mailer) SendContext(ctx context.Context, msg Message) error {
	if !m.config.Enabled {
		return ErrSMTPDisabled
	}

	// Create the message
	message := mail.NewMsg()
	if err := message.From(m.config.From); err != nil {
		return fmt.Errorf("invalid from address: %w", err)
	}
	if err := message.To(msg.To); err != nil {
		return fmt.Errorf("invalid to address: %w", err)
	}
	message.Subject(msg.Subject)
	message.SetBodyString(mail.TypeTextPlain, msg.Body)

	// Build client options
	opts := mailOptions(m.config)

	// Add authentication if credentials provided
	if m.config.Username != "" && m.config.Password != "" {
		opts = append(opts,
			mail.WithSMTPAuth(mail.SMTPAuthPlain),
			mail.WithUsername(m.config.Username),
			mail.WithPassword(m.config.Password),
		)
	}

	// Create client and send
	client, err := mail.NewClient(m.config.Host, opts...)
	if err != nil {
		return fmt.Errorf("failed to create mail client: %w", err)
	}

	if err := client.DialAndSendWithContext(ctx, message); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}

// IsEnabled returns whether SMTP is configured and enabled.
func (m *Mailer) IsEnabled() bool {
	return m.config.Enabled
}

func mailOptions(cfg config.SMTPConfig) []mail.Option {
	opts := []mail.Option{
		mail.WithPort(cfg.Port),
		mail.WithHELO("localhost"), // Use consistent HELO domain across all environments
	}

	switch cfg.TLSPolicyOrDefault() {
	case config.SMTPTLSImplicit:
		opts = append(opts, mail.WithSSL())
	case config.SMTPTLSOpportunistic:
		opts = append(opts, mail.WithTLSPortPolicy(mail.TLSOpportunistic))
	default:
		opts = append(opts, mail.WithTLSPortPolicy(mail.TLSMandatory))
	}

	return opts
}
