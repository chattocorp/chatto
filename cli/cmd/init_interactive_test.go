package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"hmans.de/chatto/internal/config"
)

func TestInteractiveOptionsGenerateMatchingConfigs(t *testing.T) {
	t.Parallel()
	opts := defaultInitOptions()
	opts.ConfigPath = filepath.Join(t.TempDir(), "community.toml")
	opts.Port = 4510
	opts.PublicURL = "https://chat.example.com"
	opts.DirectRegistration = false
	opts.WithSearch, opts.WithLiveKit = true, true
	opts.SMTP = config.SMTPConfig{
		Enabled: true, Host: "smtp.example.com", Port: 465, TLS: config.SMTPTLSImplicit,
		From: "noreply@example.com", Username: "smtp-user", Password: "private-test-password'\"",
	}
	var output bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&output)
	if err := writeInitialConfig(cmd, opts); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(opts.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	// Decode without environment overrides to verify the exact written choices.
	var cfg config.ChattoConfig
	if err := toml.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("invalid generated configuration: %v", err)
	}
	if cfg.Webserver.Port != opts.Port || cfg.Webserver.URL != opts.PublicURL {
		t.Fatal("server address choices did not survive serialization")
	}
	if cfg.Auth.DirectRegistration == nil || *cfg.Auth.DirectRegistration || !cfg.Auth.EmailOTP.ThrottlingEnabledOrDefault() {
		t.Fatal("closed registration must preserve email throttling")
	}
	if cfg.SMTP != opts.SMTP {
		t.Fatal("SMTP choices did not survive serialization")
	}
	var liveKit struct {
		Webhook struct {
			URLs []string `yaml:"urls"`
		} `yaml:"webhook"`
	}
	yamlBytes, err := os.ReadFile(filepath.Join(filepath.Dir(opts.ConfigPath), "livekit.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(yamlBytes, &liveKit); err != nil {
		t.Fatal(err)
	}
	if len(liveKit.Webhook.URLs) != 1 || liveKit.Webhook.URLs[0] != opts.PublicURL+"/webhooks/livekit" {
		t.Fatal("LiveKit webhook must use the selected Chatto URL")
	}
	printInitNextSteps(cmd, opts)
	review := initReview(opts, "4510") + output.String()
	for _, secret := range []string{opts.SMTP.Password, opts.SMTP.Username, opts.SMTP.From, cfg.LiveKit.APISecret, cfg.Core.SecretKey} {
		if strings.Contains(review, secret) {
			t.Fatal("review or output exposed a credential or submitted email identifier")
		}
	}
}

func TestInteractiveInitRequiresTerminalWithoutWriting(t *testing.T) {
	t.Parallel()
	opts := defaultInitOptions()
	opts.ConfigPath = filepath.Join(t.TempDir(), "chatto.toml")
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(&bytes.Buffer{})
	confirmed, err := promptInitOptions(cmd, &opts)
	if confirmed || err == nil || !strings.Contains(err.Error(), "requires a terminal") {
		t.Fatalf("confirmed = %v, error = %v", confirmed, err)
	}
	if _, err := os.Stat(opts.ConfigPath); !os.IsNotExist(err) {
		t.Fatal("non-terminal setup must not create a config")
	}
}

func TestInitInputValidation(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"", "0", "65536", "-1", "hello"} {
		if validateInitPort(value) == nil {
			t.Errorf("accepted invalid port %q", value)
		}
	}
	for _, value := range []string{"1", "4000", "65535"} {
		if err := validateInitPort(value); err != nil {
			t.Errorf("rejected valid port %q: %v", value, err)
		}
	}
	for _, value := range []string{"localhost", "ftp://example.com", "https://example.com/path", "https://user:password@example.com", "https://example.com?token=x", "https://example.com#fragment", "http://localhost:65536"} {
		if validateInitURL(value) == nil {
			t.Errorf("accepted invalid URL %q", value)
		}
	}
	for _, value := range []string{"", "http://localhost:4510", "https://chat.example.com/", "http://[::1]:4000"} {
		if err := validateInitURL(value); err != nil {
			t.Errorf("rejected valid URL %q: %v", value, err)
		}
	}
	if got := initPublicURL("", "4510"); got != "http://localhost:4510" {
		t.Fatalf("local URL = %q", got)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "chatto.toml")
	if err := validateInitPath(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"", path, dir, filepath.Join(dir, "missing", "chatto.toml")} {
		if validateInitPath(invalid) == nil {
			t.Errorf("accepted invalid config path %q", invalid)
		}
	}
}

func TestInitReviewValidatesSkippedAnswers(t *testing.T) {
	t.Parallel()
	opts := defaultInitOptions()
	if err := validateInitAnswers(opts, "4000", ""); err != nil {
		t.Fatalf("disabled email must not require SMTP answers: %v", err)
	}
	opts.SMTP.Enabled = true
	if err := validateInitAnswers(opts, "4000", "587"); err == nil {
		t.Fatal("review must reject an unfinished SMTP server")
	}
	opts.SMTP.Host = "smtp.example.com"
	if err := validateInitAnswers(opts, "4000", "587"); err == nil {
		t.Fatal("review must reject an unfinished sender address")
	}
	opts.SMTP.From = "noreply@example.com"
	if err := validateInitAnswers(opts, "4000", "587"); err != nil {
		t.Fatalf("complete draft should pass: %v", err)
	}
	if err := validateInitAnswers(opts, "0", "587"); err == nil {
		t.Fatal("review must reject an invalid listen port")
	}
}

func TestInteractiveInitRejectsExistingConfigBeforeTerminalCheck(t *testing.T) {
	t.Parallel()
	opts := defaultInitOptions()
	opts.ConfigPath = filepath.Join(t.TempDir(), "chatto.toml")
	original := []byte("# preserve this file\n")
	if err := os.WriteFile(opts.ConfigPath, original, 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(&output)
	confirmed, err := promptInitOptions(cmd, &opts)
	if confirmed || err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("confirmed = %v, error = %v", confirmed, err)
	}
	if output.Len() != 0 {
		t.Fatal("existing config must be rejected before the UI opens")
	}
	data, err := os.ReadFile(opts.ConfigPath)
	if err != nil || !bytes.Equal(data, original) {
		t.Fatal("existing config was changed")
	}
}
