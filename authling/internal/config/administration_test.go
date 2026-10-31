package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAdministrationConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authling.toml")
	if err := os.WriteFile(path, []byte("[administration]\nadmin_emails = ['file@example.invalid']\n[nats.embedded]\nenabled = true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Read(path)
	if err != nil || !slices.Equal(cfg.Administration.AdminEmails, []string{"file@example.invalid"}) {
		t.Fatalf("file configuration: %v", err)
	}
	t.Setenv("AUTHLING_ADMIN_EMAILS", "one@example.invalid,two@example.invalid")
	cfg, err = Read(path)
	if err != nil || !slices.Equal(cfg.Administration.AdminEmails, []string{"one@example.invalid", "two@example.invalid"}) {
		t.Fatalf("environment override: %v", err)
	}
	for _, invalid := range []string{"not-email", "", "Name <person@example.invalid>", "person@example.invalid\r\nBcc: other@example.invalid"} {
		cfg.Administration.AdminEmails = []string{invalid}
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "administration.admin_emails") || strings.Contains(err.Error(), "@") {
			t.Fatal("expected a redacted configuration error")
		}
	}
}
