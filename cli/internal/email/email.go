// Package email provides transactional email sending functionality.
package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"

	"github.com/wneessen/go-mail"

	"hmans.de/chatto/internal/config"
)

// ErrEmailDisabled is returned when no transactional email transport is enabled.
var ErrEmailDisabled = errors.New("email delivery is not enabled")

// ErrSMTPDisabled is retained for compatibility with SMTP sender callers.
var ErrSMTPDisabled = ErrEmailDisabled

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

// NewSender creates the configured transactional email transport. SMTP remains
// the default transport for configurations that predate EmailConfig.
func NewSender(emailConfig config.EmailConfig, smtpConfig config.SMTPConfig) Sender {
	if emailConfig.TransportOrDefault() == config.EmailTransportJMAP {
		return NewJMAPMailer(emailConfig.JMAP)
	}
	return NewMailer(smtpConfig)
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
	if !messageRequiresSMTPUTF8(message) {
		opts = append(opts, mail.WithoutSMTPUTF8())
	}

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

	if cfg.TLSSkipVerify || cfg.TLSServerName != "" {
		serverName := cfg.Host
		if cfg.TLSServerName != "" {
			serverName = cfg.TLSServerName
		}
		opts = append(opts, mail.WithTLSConfig(&tls.Config{
			ServerName:         serverName,
			InsecureSkipVerify: cfg.TLSSkipVerify,
			MinVersion:         tls.VersionTLS12,
		}))
	}

	return opts
}
