package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"hmans.de/chatto/internal/config"
)

func TestConfigCheckGeneratedDefaults(t *testing.T) {
	// Serial: loading observes the process environment.
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "CHATTO_") {
			t.Setenv(name, "")
			if err := os.Unsetenv(name); err != nil {
				t.Fatal(err)
			}
		}
	}
	opts := defaultInitOptions()
	opts.ConfigPath = filepath.Join(t.TempDir(), "chatto.toml")
	if err := writeInitialConfig(initCmd, opts); err != nil {
		t.Fatal(err)
	}
	command := newConfigCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"check", "-c", opts.ConfigPath})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Configuration check passed") {
		t.Fatal(output.String())
	}
	generated, err := config.ReadConfig(opts.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	// Match deployment choices and credentials, while omitting optional defaults.
	t.Setenv("CHATTO_WEBSERVER_PORT", "4000")
	t.Setenv("CHATTO_WEBSERVER_URL", opts.PublicURL)
	t.Setenv("CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET", generated.Webserver.CookieSigningSecret)
	t.Setenv("CHATTO_WEBSERVER_COOKIE_ENCRYPTION_SECRET", generated.Webserver.CookieEncryptionSecret)
	t.Setenv("CHATTO_CORE_SECRET_KEY", generated.Core.SecretKey)
	t.Setenv("CHATTO_CORE_ASSETS_SIGNING_SECRET", generated.Core.Assets.SigningSecret)
	t.Setenv("CHATTO_NATS_EMBEDDED_ENABLED", "true")
	t.Setenv("CHATTO_NATS_EMBEDDED_DATA_DIR", "./data")
	t.Setenv("CHATTO_NATS_EMBEDDED_AUTH_TOKEN", generated.NATS.Embedded.AuthToken)
	t.Setenv("CHATTO_ASSET_PROCESSING_ENABLED", "true")
	t.Setenv("CHATTO_SMTP_PORT", "587")
	envOnly, err := config.ReadConfig(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatal(err)
	}
	compareDefaultMethods(t, reflect.ValueOf(&generated), reflect.ValueOf(&envOnly), "config")
}

// Compare every no-argument OrDefault method, including nested optional
// settings, so new defaults join the generated-versus-ENV regression check.
func compareDefaultMethods(t *testing.T, a, b reflect.Value, path string) {
	t.Helper()
	for i := 0; i < a.NumMethod(); i++ {
		method := a.Type().Method(i)
		if !strings.HasSuffix(method.Name, "OrDefault") || method.Type.NumIn() != 1 {
			continue
		}
		av, bv := a.Method(i).Call(nil), b.Method(i).Call(nil)
		for j := range av {
			if !reflect.DeepEqual(av[j].Interface(), bv[j].Interface()) {
				t.Errorf("%s.%s differs between generated and ENV-only config", path, method.Name)
			}
		}
	}
	a, b = a.Elem(), b.Elem()
	for i := 0; i < a.NumField(); i++ {
		if a.Field(i).Kind() == reflect.Struct {
			compareDefaultMethods(t, a.Field(i).Addr(), b.Field(i).Addr(), path+"."+a.Type().Field(i).Name)
		}
	}
}

func TestConfigCheckCommandFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chatto.toml")
	if err := os.WriteFile(path, []byte("[auth]\nmisspelled='private-sentinel'"), 0600); err != nil {
		t.Fatal(err)
	}
	command := newConfigCommand()
	command.SilenceUsage, command.SilenceErrors = true, true
	command.SetArgs([]string{"check", "-c", path})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "auth.misspelled") || strings.Contains(err.Error(), "private-sentinel") {
		t.Fatalf("unexpected diagnostic: %v", err)
	}
}
