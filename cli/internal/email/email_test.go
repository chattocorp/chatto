package email

import (
	"errors"
	"reflect"
	"testing"

	"github.com/wneessen/go-mail"

	"hmans.de/chatto/internal/config"
)

func TestMailer_Send_Disabled(t *testing.T) {
	cfg := config.SMTPConfig{
		Enabled: false,
	}
	mailer := NewMailer(cfg)

	err := mailer.Send(Message{
		To:      "test@example.com",
		Subject: "Test",
		Body:    "Test body",
	})

	if !errors.Is(err, ErrSMTPDisabled) {
		t.Errorf("expected ErrSMTPDisabled, got %v", err)
	}
}

func TestMailer_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		expected bool
	}{
		{"enabled", true, true},
		{"disabled", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.SMTPConfig{Enabled: tt.enabled}
			mailer := NewMailer(cfg)
			if got := mailer.IsEnabled(); got != tt.expected {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMailOptionsTLS(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.SMTPConfig
		wantSSL bool
	}{
		{
			name:    "mandatory STARTTLS does not use implicit TLS",
			cfg:     config.SMTPConfig{Port: 587, TLS: config.SMTPTLSMandatory},
			wantSSL: false,
		},
		{
			name:    "opportunistic STARTTLS does not use implicit TLS",
			cfg:     config.SMTPConfig{Port: 587, TLS: config.SMTPTLSOpportunistic},
			wantSSL: false,
		},
		{
			name:    "explicit implicit TLS uses SSL mode",
			cfg:     config.SMTPConfig{Port: 465, TLS: config.SMTPTLSImplicit},
			wantSSL: true,
		},
		{
			name:    "port 465 with default policy uses SSL mode",
			cfg:     config.SMTPConfig{Port: 465},
			wantSSL: true,
		},
		{
			name:    "port 465 with mandatory policy uses SSL mode",
			cfg:     config.SMTPConfig{Port: 465, TLS: config.SMTPTLSMandatory},
			wantSSL: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := mail.NewClient("smtp.example.com", mailOptions(tt.cfg)...)
			if err != nil {
				t.Fatalf("NewClient() failed: %v", err)
			}
			if got := clientUsesSSL(client); got != tt.wantSSL {
				t.Errorf("implicit TLS = %v, want %v", got, tt.wantSSL)
			}
		})
	}
}

func clientUsesSSL(client *mail.Client) bool {
	return reflect.ValueOf(client).Elem().FieldByName("useSSL").Bool()
}
