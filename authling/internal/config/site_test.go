package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSiteConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "site.toml")
	if err := os.WriteFile(path, []byte(`[site]
name = "Example accounts"
description = "One account for our apps."
[http]
public_url = "https://accounts.example.com"
[nats.embedded]
enabled = true
`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Site.Name != "Example accounts" || cfg.Site.Description != "One account for our apps." {
		t.Fatal("TOML site settings were not loaded")
	}
	t.Setenv("AUTHLING_SITE_NAME", " chatto.id ")
	t.Setenv("AUTHLING_SITE_DESCRIPTION", " Your account for Chatto. ")
	cfg, err = Read(path)
	if err != nil {
		t.Fatal(err)
	}
	site := cfg.Site.Resolve(cfg.HTTP.PublicURLOrDefault())
	if site.Name != "chatto.id" || site.Description != "Your account for Chatto." {
		t.Fatalf("resolved site = %+v", site)
	}
	if cfg.HTTP.PublicURLOrDefault() != "https://accounts.example.com" {
		t.Fatal("branding changed the public URL")
	}
}

func TestSiteNameDefaultsToConfiguredHostname(t *testing.T) {
	for _, publicURL := range []string{"https://chatto.id:8443", "http://chatto.id"} {
		site := (SiteConfig{Name: "  "}).Resolve(publicURL)
		if site.Name != "chatto.id" || site.Description != "" {
			t.Fatalf("site = %+v", site)
		}
	}
}

func TestSiteRejectsHeaderInjectionAndExcessiveText(t *testing.T) {
	for _, site := range []SiteConfig{
		{Name: "Accounts\r\nBcc: someone@example.com"},
		{Description: "line one\nline two"},
		{Name: strings.Repeat("x", 121)},
		{Description: strings.Repeat("x", 501)},
		{Name: string([]byte{0xff})},
	} {
		cfg := Config{Site: site, NATS: NATSConfig{Embedded: EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}}}
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "site.") {
			t.Fatalf("validation error = %v", err)
		}
	}
}
