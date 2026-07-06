package email

import (
	"crypto/tls"
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
		name           string
		cfg            config.SMTPConfig
		wantSSL        bool
		wantSkipVerify bool
		wantServerName string
	}{
		{
			name:           "mandatory STARTTLS does not use implicit TLS",
			cfg:            config.SMTPConfig{Port: 587, TLS: config.SMTPTLSMandatory},
			wantSSL:        false,
			wantServerName: "smtp.example.com",
		},
		{
			name:           "opportunistic STARTTLS does not use implicit TLS",
			cfg:            config.SMTPConfig{Port: 587, TLS: config.SMTPTLSOpportunistic},
			wantSSL:        false,
			wantServerName: "smtp.example.com",
		},
		{
			name:           "explicit implicit TLS uses SSL mode",
			cfg:            config.SMTPConfig{Port: 465, TLS: config.SMTPTLSImplicit},
			wantSSL:        true,
			wantServerName: "smtp.example.com",
		},
		{
			name:           "port 465 with default policy uses SSL mode",
			cfg:            config.SMTPConfig{Port: 465},
			wantSSL:        true,
			wantServerName: "smtp.example.com",
		},
		{
			name:           "port 465 with mandatory policy uses SSL mode",
			cfg:            config.SMTPConfig{Port: 465, TLS: config.SMTPTLSMandatory},
			wantSSL:        true,
			wantServerName: "smtp.example.com",
		},
		{
			name:           "skip verify configures insecure TLS verification",
			cfg:            config.SMTPConfig{Host: "smtp.example.com", Port: 587, TLS: config.SMTPTLSMandatory, TLSSkipVerify: true},
			wantSSL:        false,
			wantSkipVerify: true,
			wantServerName: "smtp.example.com",
		},
		{
			name:           "server name override configures SNI without skip verify",
			cfg:            config.SMTPConfig{Host: "192.0.2.10", Port: 587, TLS: config.SMTPTLSMandatory, TLSServerName: "mail.example.com"},
			wantSSL:        false,
			wantServerName: "mail.example.com",
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
			if got := clientTLSSkipVerify(client); got != tt.wantSkipVerify {
				t.Errorf("TLS skip verify = %v, want %v", got, tt.wantSkipVerify)
			}
			if got := clientTLSServerName(client); got != tt.wantServerName {
				t.Errorf("TLS server name = %q, want %q", got, tt.wantServerName)
			}
			if got := clientTLSMinVersion(client); got != tls.VersionTLS12 {
				t.Errorf("TLS min version = %d, want %d", got, tls.VersionTLS12)
			}
		})
	}
}

func clientUsesSSL(client *mail.Client) bool {
	return reflect.ValueOf(client).Elem().FieldByName("useSSL").Bool()
}

func clientTLSSkipVerify(client *mail.Client) bool {
	return reflect.ValueOf(client).Elem().FieldByName("tlsconfig").Elem().FieldByName("InsecureSkipVerify").Bool()
}

func clientTLSServerName(client *mail.Client) string {
	return reflect.ValueOf(client).Elem().FieldByName("tlsconfig").Elem().FieldByName("ServerName").String()
}

func clientTLSMinVersion(client *mail.Client) uint64 {
	return reflect.ValueOf(client).Elem().FieldByName("tlsconfig").Elem().FieldByName("MinVersion").Uint()
}
