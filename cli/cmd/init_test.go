package cmd

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/pkg/natsauth"
)

func TestRunInitCommandCreatesEmbeddedConfiguration(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "chatto.toml")
	var output bytes.Buffer
	var gotWizardOptions initWizardOptions

	err := runInitCommand(initCommandOptions{configPath: configPath}, initCommandDependencies{
		in:      strings.NewReader(""),
		out:     &output,
		entropy: bytes.NewReader(bytes.Repeat([]byte{0x42}, 32*5)),
		getenv:  func(string) string { return "" },
		wizard: func(answers *initAnswers, opts initWizardOptions) error {
			gotWizardOptions = opts
			answers.PublicURL = "https://chat.example.com"
			answers.ListenPort = "4444"
			answers.EmbeddedDataDir = "/var/lib/chatto"
			answers.NATSReplicas = 5 // Stale external-mode answer must not leak through.
			answers.Confirmed = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("runInitCommand() error = %v", err)
	}
	if gotWizardOptions.configPath != configPath {
		t.Fatalf("wizard config path = %q, want %q", gotWizardOptions.configPath, configPath)
	}

	cfg, err := config.ReadConfig(configPath)
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	if cfg.Webserver.URL != "https://chat.example.com" || cfg.Webserver.Port != 4444 {
		t.Fatalf("generated webserver = %q:%d", cfg.Webserver.URL, cfg.Webserver.Port)
	}
	assertHexSecret(t, "core secret", cfg.Core.SecretKey)
	assertHexSecret(t, "cookie signing secret", cfg.Webserver.CookieSigningSecret)
	assertHexSecret(t, "cookie encryption secret", cfg.Webserver.CookieEncryptionSecret)
	assertHexSecret(t, "asset signing secret", cfg.Core.Assets.SigningSecret)
	assertHexSecret(t, "embedded NATS token", cfg.NATS.Embedded.AuthToken)
	if !cfg.NATS.Embedded.Enabled || cfg.NATS.Embedded.DataDir != "/var/lib/chatto" {
		t.Fatalf("generated embedded NATS = %+v", cfg.NATS.Embedded)
	}
	if cfg.NATS.Embedded.Port != 0 {
		t.Fatalf("generated embedded NATS port = %d, want 0 when commented", cfg.NATS.Embedded.Port)
	}
	if cfg.NATS.Client.URL != "" {
		t.Fatalf("generated external NATS URL = %q, want empty", cfg.NATS.Client.URL)
	}
	if cfg.NATS.Replicas != 1 {
		t.Fatalf("generated NATS replicas = %d, want 1", cfg.NATS.Replicas)
	}
	if cfg.Core.Assets.StorageBackend != config.StorageBackendNATS {
		t.Fatalf("generated asset storage = %q", cfg.Core.Assets.StorageBackend)
	}
	if !cfg.Auth.EmailOTP.ThrottlingEnabledOrDefault() {
		t.Fatal("generated config should enable email OTP throttling")
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat generated config: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("generated config mode = %o, want 600", got)
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	text := string(raw)
	for _, want := range []string{
		"allowed_origins = ['*']",
		"storage_backend = 'nats'",
		"[auth.email_otp]",
		"throttling_enabled = true",
		"# [[auth.providers]]",
		"# [nats.client]",
		"[nats.embedded]",
		"enabled = true",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("generated config missing %q", want)
		}
	}
	if !strings.Contains(output.String(), "The lights are on") || !strings.Contains(output.String(), "chatto run --config "+configPath) {
		t.Fatalf("completion output = %q", output.String())
	}
}

func TestRunInitCommandCreatesExternalNATSConfiguration(t *testing.T) {
	validNKeySeed, _, err := natsauth.GenerateUserNKey()
	if err != nil {
		t.Fatalf("generate test NKey: %v", err)
	}
	tests := []struct {
		name       string
		configure  func(*initAnswers)
		assertNATS func(*testing.T, config.NATSClientConfig)
	}{
		{
			name: "credentials file",
			configure: func(a *initAnswers) {
				a.NATSAuthMethod = config.NATSAuthCredentials
				a.NATSCredentialsFile = "/run/secrets/chatto.creds"
			},
			assertNATS: func(t *testing.T, c config.NATSClientConfig) {
				if c.CredentialsFile != "/run/secrets/chatto.creds" {
					t.Fatalf("credentials file = %q", c.CredentialsFile)
				}
			},
		},
		{
			name: "token",
			configure: func(a *initAnswers) {
				a.NATSAuthMethod = config.NATSAuthToken
				a.NATSToken = "top-secret-token"
			},
			assertNATS: func(t *testing.T, c config.NATSClientConfig) {
				if c.Token != "top-secret-token" {
					t.Fatalf("token = %q", c.Token)
				}
			},
		},
		{
			name: "userpass",
			configure: func(a *initAnswers) {
				a.NATSAuthMethod = config.NATSAuthUserPass
				a.NATSUsername = "chatto"
				a.NATSPassword = "password"
			},
			assertNATS: func(t *testing.T, c config.NATSClientConfig) {
				if c.Username != "chatto" || c.Password != "password" {
					t.Fatalf("userpass = %q/%q", c.Username, c.Password)
				}
			},
		},
		{
			name: "nkey",
			configure: func(a *initAnswers) {
				a.NATSAuthMethod = config.NATSAuthNKey
				a.NATSNKeySeed = validNKeySeed
			},
			assertNATS: func(t *testing.T, c config.NATSClientConfig) {
				if c.NKeySeed != validNKeySeed {
					t.Fatalf("NKey seed = %q", c.NKeySeed)
				}
			},
		},
		{
			name: "none",
			configure: func(a *initAnswers) {
				a.NATSAuthMethod = config.NATSAuthNone
			},
			assertNATS: func(t *testing.T, c config.NATSClientConfig) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "chatto.toml")
			err := runInitCommand(initCommandOptions{configPath: configPath}, initCommandDependencies{
				in:      strings.NewReader(""),
				out:     ioDiscard{},
				entropy: bytes.NewReader(bytes.Repeat([]byte{0x24}, 32*5)),
				getenv:  func(string) string { return "" },
				wizard: func(answers *initAnswers, _ initWizardOptions) error {
					answers.NATSMode = initNATSExternal
					answers.ExternalNATSURL = "nats://nats-1:4222,nats://nats-2:4222"
					answers.NATSReplicas = 3
					answers.NATSToken = "abandoned-token"
					answers.NATSUsername = "abandoned-user"
					answers.NATSPassword = "abandoned-password"
					answers.NATSCredentialsFile = "/abandoned.creds"
					answers.NATSNKeySeed = validNKeySeed
					tt.configure(answers)
					answers.Confirmed = true
					return nil
				},
			})
			if err != nil {
				t.Fatalf("runInitCommand() error = %v", err)
			}
			cfg, err := config.ReadConfig(configPath)
			if err != nil {
				t.Fatalf("read generated config: %v", err)
			}
			if cfg.NATS.Embedded.Enabled {
				t.Fatal("embedded NATS should be disabled")
			}
			if cfg.NATS.Client.URL != "nats://nats-1:4222,nats://nats-2:4222" || cfg.NATS.Replicas != 3 {
				t.Fatalf("external NATS = %+v", cfg.NATS)
			}
			if cfg.NATS.Client.AuthMethod != ttAuthMethod(tt.name) {
				t.Fatalf("auth method = %q", cfg.NATS.Client.AuthMethod)
			}
			assertOnlySelectedNATSCredentials(t, cfg.NATS.Client)
			tt.assertNATS(t, cfg.NATS.Client)

			raw, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("read generated config: %v", err)
			}
			if !strings.Contains(string(raw), "\n[nats.client]\n") || strings.Contains(string(raw), "\n# [nats.client]\n") {
				t.Fatalf("external NATS client table was not activated:\n%s", raw)
			}
		})
	}
}

func assertOnlySelectedNATSCredentials(t *testing.T, client config.NATSClientConfig) {
	t.Helper()
	if client.AuthMethod != config.NATSAuthToken && client.Token != "" {
		t.Errorf("unselected token was retained")
	}
	if client.AuthMethod != config.NATSAuthUserPass && (client.Username != "" || client.Password != "") {
		t.Errorf("unselected username/password was retained")
	}
	if client.AuthMethod != config.NATSAuthCredentials && client.CredentialsFile != "" {
		t.Errorf("unselected credentials file was retained")
	}
	if client.AuthMethod != config.NATSAuthNKey && client.NKeySeed != "" {
		t.Errorf("unselected NKey seed was retained")
	}
}

func ttAuthMethod(name string) config.NATSAuthMethod {
	switch name {
	case "credentials file":
		return config.NATSAuthCredentials
	case "token":
		return config.NATSAuthToken
	case "userpass":
		return config.NATSAuthUserPass
	case "nkey":
		return config.NATSAuthNKey
	default:
		return config.NATSAuthNone
	}
}

func TestRunInitCommandCancellationWritesNothing(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "chatto.toml")
	for _, wizard := range []func(*initAnswers, initWizardOptions) error{
		func(*initAnswers, initWizardOptions) error { return huh.ErrUserAborted },
		func(answers *initAnswers, _ initWizardOptions) error {
			answers.Confirmed = false
			return nil
		},
	} {
		err := runInitCommand(initCommandOptions{configPath: configPath}, initCommandDependencies{
			in:      strings.NewReader(""),
			out:     ioDiscard{},
			entropy: bytes.NewReader(bytes.Repeat([]byte{0x42}, 32*5)),
			getenv:  func(string) string { return "" },
			wizard:  wizard,
		})
		if err == nil || !strings.Contains(err.Error(), "nothing was written") {
			t.Fatalf("runInitCommand() error = %v", err)
		}
		if _, err := os.Stat(configPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("config exists after cancellation: %v", err)
		}
	}
}

func TestRunInitCommandRejectsInvalidAnswersBeforeWriting(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*initAnswers)
	}{
		{name: "public URL", configure: func(a *initAnswers) { a.PublicURL = "/chat" }},
		{name: "listen port", configure: func(a *initAnswers) { a.ListenPort = "70000" }},
		{name: "NATS mode", configure: func(a *initAnswers) { a.NATSMode = "mystery" }},
		{name: "embedded data directory", configure: func(a *initAnswers) { a.EmbeddedDataDir = " " }},
		{name: "external URL", configure: func(a *initAnswers) {
			a.NATSMode = initNATSExternal
			a.ExternalNATSURL = "https://nats.example.com"
		}},
		{name: "external replicas", configure: func(a *initAnswers) {
			a.NATSMode = initNATSExternal
			a.NATSReplicas = 2
		}},
		{name: "missing credentials file", configure: func(a *initAnswers) {
			a.NATSMode = initNATSExternal
			a.NATSCredentialsFile = ""
		}},
		{name: "missing token", configure: func(a *initAnswers) {
			a.NATSMode = initNATSExternal
			a.NATSAuthMethod = config.NATSAuthToken
		}},
		{name: "missing username", configure: func(a *initAnswers) {
			a.NATSMode = initNATSExternal
			a.NATSAuthMethod = config.NATSAuthUserPass
			a.NATSPassword = "secret"
		}},
		{name: "missing password", configure: func(a *initAnswers) {
			a.NATSMode = initNATSExternal
			a.NATSAuthMethod = config.NATSAuthUserPass
			a.NATSUsername = "chatto"
		}},
		{name: "invalid NKey seed", configure: func(a *initAnswers) {
			a.NATSMode = initNATSExternal
			a.NATSAuthMethod = config.NATSAuthNKey
			a.NATSNKeySeed = "not-a-seed"
		}},
		{name: "unknown auth method", configure: func(a *initAnswers) {
			a.NATSMode = initNATSExternal
			a.NATSAuthMethod = "mystery"
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "chatto.toml")
			err := runInitCommand(initCommandOptions{configPath: configPath}, initCommandDependencies{
				in:      strings.NewReader(""),
				out:     ioDiscard{},
				entropy: panicReader{},
				getenv:  func(string) string { return "" },
				wizard: func(answers *initAnswers, _ initWizardOptions) error {
					tt.configure(answers)
					answers.Confirmed = true
					return nil
				},
			})
			if err == nil {
				t.Fatal("runInitCommand() succeeded")
			}
			if _, statErr := os.Stat(configPath); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("config exists after validation error: %v", statErr)
			}
		})
	}
}

func TestRunInitCommandRefusesOverwriteBeforeWizard(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "chatto.toml")
	if err := os.WriteFile(configPath, []byte("sentinel"), 0o600); err != nil {
		t.Fatalf("write existing config: %v", err)
	}
	wizardCalled := false
	err := runInitCommand(initCommandOptions{configPath: configPath}, initCommandDependencies{
		wizard: func(*initAnswers, initWizardOptions) error {
			wizardCalled = true
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("runInitCommand() error = %v", err)
	}
	if wizardCalled {
		t.Fatal("wizard ran before overwrite protection")
	}
	got, err := os.ReadFile(configPath)
	if err != nil || string(got) != "sentinel" {
		t.Fatalf("existing config changed: %q, %v", got, err)
	}
}

func TestRunInitCommandAccessibleMode(t *testing.T) {
	tests := []struct {
		name          string
		flag          bool
		accessibleEnv string
		term          string
		wantMode      bool
	}{
		{name: "default"},
		{name: "flag", flag: true, wantMode: true},
		{name: "environment", accessibleEnv: "1", wantMode: true},
		{name: "dumb terminal", term: "dumb", wantMode: true},
		{name: "case-insensitive dumb terminal", term: " DUMB ", wantMode: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "chatto.toml")
			gotAccessible := false
			err := runInitCommand(initCommandOptions{configPath: configPath, accessible: tt.flag}, initCommandDependencies{
				in:      strings.NewReader(""),
				out:     ioDiscard{},
				entropy: bytes.NewReader(bytes.Repeat([]byte{0x42}, 32*5)),
				getenv: func(name string) string {
					switch name {
					case "CHATTO_ACCESSIBLE":
						return tt.accessibleEnv
					case "TERM":
						return tt.term
					default:
						return ""
					}
				},
				wizard: func(answers *initAnswers, opts initWizardOptions) error {
					gotAccessible = opts.accessible
					answers.Confirmed = true
					return nil
				},
			})
			if err != nil {
				t.Fatalf("runInitCommand() error = %v", err)
			}
			if gotAccessible != tt.wantMode {
				t.Fatalf("accessible = %v, want %v", gotAccessible, tt.wantMode)
			}
		})
	}
}

func TestRunInitCommandDumbTerminalEOFWritesNothing(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "chatto.toml")
	err := runInitCommand(initCommandOptions{configPath: configPath}, initCommandDependencies{
		in:      strings.NewReader(""),
		out:     ioDiscard{},
		entropy: panicReader{},
		getenv: func(name string) string {
			if name == "TERM" {
				return "dumb"
			}
			return ""
		},
		wizard: runInitWizard,
	})
	if err == nil || !strings.Contains(err.Error(), "nothing was written") {
		t.Fatalf("runInitCommand() error = %v", err)
	}
	if _, statErr := os.Stat(configPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("config exists after EOF: %v", statErr)
	}
}

func TestInitWizardAccessibleReviewIncludesSummary(t *testing.T) {
	answers := defaultInitAnswers()
	answers.PublicURL = "https://chat.example.com"
	var output bytes.Buffer
	opts := initWizardOptions{
		input:      strings.NewReader("y\n"),
		output:     &output,
		accessible: true,
		configPath: "/etc/chatto/chatto.toml",
	}
	err := newInitForm(opts, initReviewGroup(&answers, opts.configPath, false)).Run()
	if err != nil {
		t.Fatalf("review form error = %v", err)
	}
	text := output.String()
	for _, want := range []string{"Launch card", "https://chat.example.com", "/etc/chatto/chatto.toml"} {
		if !strings.Contains(text, want) {
			t.Errorf("accessible output missing %q:\n%s", want, text)
		}
	}
}

func TestRunInitWizardAccessibleAcceptsDisplayedDefaults(t *testing.T) {
	reader, writer := io.Pipe()
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})
	output := newAccessibleScriptWriter(writer, []accessiblePromptResponse{
		{prompt: "Where will people open Chatto?", response: "\n"},
		{prompt: "Which local port should Chatto listen on?", response: "\n"},
		{prompt: "Where should Chatto remember everything?", response: "1\n"},
		{prompt: "Where should embedded NATS keep its data?", response: "\n"},
		{prompt: "Create this configuration?", response: "y\n"},
	})
	answers := defaultInitAnswers()
	err := runInitWizard(&answers, initWizardOptions{
		input:      reader,
		output:     output,
		accessible: true,
		configPath: "/etc/chatto/chatto.toml",
	})
	if err != nil {
		t.Fatalf("runInitWizard() error = %v", err)
	}
	if answers.PublicURL != "http://localhost:4000" || answers.ListenPort != "4000" ||
		answers.NATSMode != initNATSEmbedded || answers.EmbeddedDataDir != "./data" || !answers.Confirmed {
		t.Fatalf("answers = %+v", answers)
	}
	text := output.String()
	for _, want := range []string{
		"[default: http://localhost:4000]",
		"[default: 4000]",
		"[default: ./data]",
		"Launch card",
		"/etc/chatto/chatto.toml",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("accessible output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "How can Chatto reach NATS?") {
		t.Fatalf("embedded flow asked external NATS questions:\n%s", text)
	}
}

func TestInitWizardSurvivesInvalidTerminalWidths(t *testing.T) {
	answers := defaultInitAnswers()
	form := newInitForm(initWizardOptions{}, initFrontDoorGroup(&answers, false))
	for _, width := range []int{0, -1, -80} {
		model, _ := form.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		form = model.(*huh.Form)
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("form panicked at terminal width %d: %v", width, recovered)
				}
			}()
			_ = form.View()
		}()
	}
}

func TestInitWizardModelOwnsIntroAndForm(t *testing.T) {
	answers := defaultInitAnswers()
	form := newInitForm(initWizardOptions{}, initFrontDoorGroup(&answers, false))
	model := newInitWizardModel(form)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	model = updated.(*initWizardModel)

	intro := model.View()
	if !intro.AltScreen {
		t.Fatal("wizard does not use the alternate screen")
	}
	if !strings.Contains(intro.Content, "┌─┐┬ ┬") {
		t.Fatalf("intro does not contain the Chatto wordmark:\n%s", intro.Content)
	}

	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = updated.(*initWizardModel)
	formView := model.View()
	if model.stage != initWizardForm {
		t.Fatalf("stage = %v, want form", model.stage)
	}
	if strings.Contains(formView.Content, "┌─┐┬ ┬") {
		t.Fatalf("wordmark remained after entering the form:\n%s", formView.Content)
	}
	if !strings.Contains(formView.Content, "The front door") {
		t.Fatalf("form view does not contain first group:\n%s", formView.Content)
	}
}

func TestInitWizardModelConstrainsFormToTerminal(t *testing.T) {
	model := newInitWizardModel(newInitForm(initWizardOptions{}, initWelcomeGroup()))
	tests := []struct {
		width int
		want  int
	}{
		{width: 10, want: initFormMinWidth},
		{width: 80, want: 72},
		{width: 200, want: initWizardMaxFormWidth},
	}
	for _, tt := range tests {
		model.width = tt.width
		model.height = 30
		if got := model.formWindowSize().Width; got != tt.want {
			t.Errorf("terminal width %d: form width = %d, want %d", tt.width, got, tt.want)
		}
	}
}

func TestInitWizardIntroFramesHaveStableGeometry(t *testing.T) {
	for _, width := range []int{50, 100} {
		first := initWizardIntroView(width, 1, true)
		wantWidth := lipgloss.Width(first)
		wantHeight := lipgloss.Height(first)
		for frame := 2; frame <= initWizardIntroFrames(); frame++ {
			view := initWizardIntroView(width, frame, true)
			if got := lipgloss.Width(view); got != wantWidth {
				t.Errorf("width %d frame %d: rendered width = %d, want %d", width, frame, got, wantWidth)
			}
			if got := lipgloss.Height(view); got != wantHeight {
				t.Errorf("width %d frame %d: rendered height = %d, want %d", width, frame, got, wantHeight)
			}
		}
	}
}

func TestRunInteractiveInitWizardUsesOneTerminalRenderer(t *testing.T) {
	answers := defaultInitAnswers()
	form := newInitForm(initWizardOptions{}, initFrontDoorGroup(&answers, false))
	input, inputWriter := io.Pipe()
	output := newTerminalCaptureWriter()
	go func() {
		<-output.enteredAltScreen
		_, _ = inputWriter.Write([]byte("\r\x03"))
		_ = inputWriter.Close()
	}()
	err := runInteractiveInitWizard(form, initWizardOptions{
		input:  input,
		output: output,
	})
	if !errors.Is(err, huh.ErrUserAborted) {
		t.Fatalf("runInteractiveInitWizard() error = %v, want user aborted", err)
	}
	const (
		enterAltScreen = "\x1b[?1049h"
		exitAltScreen  = "\x1b[?1049l"
	)
	if got := strings.Count(output.String(), enterAltScreen); got != 1 {
		t.Fatalf("alternate screen entered %d times, want exactly once", got)
	}
	if got := strings.Count(output.String(), exitAltScreen); got != 1 {
		t.Fatalf("alternate screen exited %d times, want exactly once", got)
	}
}

type terminalCaptureWriter struct {
	mu               sync.Mutex
	buffer           bytes.Buffer
	enteredAltScreen chan struct{}
	enteredOnce      sync.Once
}

func newTerminalCaptureWriter() *terminalCaptureWriter {
	return &terminalCaptureWriter{enteredAltScreen: make(chan struct{})}
}

func (w *terminalCaptureWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.buffer.Write(p)
	if strings.Contains(w.buffer.String(), "\x1b[?1049h") {
		w.enteredOnce.Do(func() { close(w.enteredAltScreen) })
	}
	return n, err
}

func (w *terminalCaptureWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.String()
}

func TestInitWizardValidators(t *testing.T) {
	valid := []struct {
		name string
		fn   func(string) error
		text string
	}{
		{name: "https URL", fn: validatePublicURL, text: "https://chat.example.com"},
		{name: "loopback URL", fn: validatePublicURL, text: "http://localhost:4000"},
		{name: "port", fn: validatePort, text: "4000"},
		{name: "NATS URL", fn: validateNATSURLs, text: "nats://one:4222,tls://two:4222"},
		{name: "not blank", fn: validateNotBlank("value"), text: "hello"},
	}
	for _, tt := range valid {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(tt.text); err != nil {
				t.Fatalf("validate(%q) = %v", tt.text, err)
			}
		})
	}

	invalid := []struct {
		name string
		fn   func(string) error
		text string
	}{
		{name: "relative URL", fn: validatePublicURL, text: "/chat"},
		{name: "URL path", fn: validatePublicURL, text: "https://example.com/chat"},
		{name: "bad port", fn: validatePort, text: "70000"},
		{name: "bad NATS scheme", fn: validateNATSURLs, text: "https://nats.example.com"},
		{name: "blank", fn: validateNotBlank("value"), text: "  "},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(tt.text); err == nil {
				t.Fatalf("validate(%q) succeeded", tt.text)
			}
		})
	}
}

func assertHexSecret(t *testing.T, name, value string) {
	t.Helper()
	if len(value) != 64 {
		t.Fatalf("%s length = %d, want 64", name, len(value))
	}
	if _, err := hex.DecodeString(value); err != nil {
		t.Fatalf("%s is not hex: %v", name, err)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("entropy read before answer validation") }

type accessiblePromptResponse struct {
	prompt   string
	response string
}

type accessibleScriptWriter struct {
	mu     sync.Mutex
	output bytes.Buffer
	input  *io.PipeWriter
	steps  []accessiblePromptResponse
	next   int
}

func newAccessibleScriptWriter(input *io.PipeWriter, steps []accessiblePromptResponse) *accessibleScriptWriter {
	return &accessibleScriptWriter{input: input, steps: steps}
}

func (w *accessibleScriptWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.output.Write(p)
	if err != nil || w.next >= len(w.steps) {
		return n, err
	}
	step := w.steps[w.next]
	if strings.Contains(w.output.String(), step.prompt) {
		w.next++
		go func() {
			_, _ = io.WriteString(w.input, step.response)
		}()
	}
	return n, nil
}

func (w *accessibleScriptWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.output.String()
}
