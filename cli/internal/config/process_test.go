package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
)

func TestReadConfig_ShieldsEnabledFromEnv(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	t.Setenv("CHATTO_WEBSERVER_PORT", "4000")
	t.Setenv("CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("CHATTO_CORE_SECRET_KEY", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	t.Setenv("CHATTO_CORE_ASSETS_SIGNING_SECRET", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	t.Setenv("CHATTO_WEBSERVER_SHIELDS_ENABLED", "true")

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if !cfg.Webserver.Shields.Enabled {
		t.Fatal("expected shields enabled from env")
	}
}

func TestReadConfig_ShieldsEnabledFromNestedWebserverConfig(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	configContent := `
[webserver]
port = 4000
cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[webserver.shields]
enabled = true

[core]
secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "chatto.toml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if !cfg.Webserver.Shields.Enabled {
		t.Fatal("expected shields enabled from [webserver.shields]")
	}
}

func TestSearchProviderDirectoryDefault(t *testing.T) {
	var cfg SearchProviderConfig
	if got := cfg.DirectoryOrDefault(); got != "./data/search" {
		t.Fatalf("DirectoryOrDefault() = %q, want %q", got, "./data/search")
	}
}

func TestSearchProviderLanguagesDefaultAndExplicitEmpty(t *testing.T) {
	var defaults SearchProviderConfig
	if got := defaults.LanguagesOrDefault(); !slices.Equal(got, SupportedSearchProviderLanguages()) {
		t.Fatalf("default languages = %v", got)
	}

	explicitEmpty := SearchProviderConfig{Languages: []string{}}
	if got := explicitEmpty.LanguagesOrDefault(); got == nil || len(got) != 0 {
		t.Fatalf("explicit empty languages = %#v, want non-nil empty list", got)
	}

	environmentNone := SearchProviderConfig{Languages: []string{"none"}}
	if got := environmentNone.LanguagesOrDefault(); got == nil || len(got) != 0 {
		t.Fatalf("none languages = %#v, want non-nil empty list", got)
	}
}

func TestAssetProcessingEnabledIsIndependentFromVideoUploads(t *testing.T) {
	tests := []struct {
		name                string
		toml                string
		wantVideoUploads    bool
		wantAssetProcessing bool
	}{
		{
			name:                "uploads only",
			toml:                "[video]\nenabled = true\n",
			wantVideoUploads:    true,
			wantAssetProcessing: false,
		},
		{
			name:                "worker only",
			toml:                "[asset_processing]\nenabled = true\n",
			wantVideoUploads:    false,
			wantAssetProcessing: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var cfg ChattoConfig
			if err := toml.Unmarshal([]byte(test.toml), &cfg); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if cfg.Video.Enabled != test.wantVideoUploads {
				t.Fatalf("video.enabled = %v, want %v", cfg.Video.Enabled, test.wantVideoUploads)
			}
			if cfg.AssetProcessing.Enabled != test.wantAssetProcessing {
				t.Fatalf("asset_processing.enabled = %v, want %v", cfg.AssetProcessing.Enabled, test.wantAssetProcessing)
			}
		})
	}
}

func TestAssetProcessingSettingsAreIndependentFromVideoUploads(t *testing.T) {
	if got := (&AssetProcessingConfig{}).MaxConcurrentJobsOrDefault(); got != 2 {
		t.Fatalf("default asset_processing.max_concurrent_jobs = %d, want 2", got)
	}

	var cfg ChattoConfig
	configTOML := `[video]
enabled = true
max_upload_size = "250 MB"

[asset_processing]
enabled = true
ffmpeg_path = "/opt/bin/ffmpeg"
ffprobe_path = "/opt/bin/ffprobe"
max_concurrent_jobs = 4
temp_dir = "/var/tmp/chatto-assets"
`
	if err := toml.Unmarshal([]byte(configTOML), &cfg); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got := cfg.Video.MaxUploadSize.String(); got != "250MB" {
		t.Fatalf("video.max_upload_size = %q, want 250MB", got)
	}
	if got := cfg.AssetProcessing.FFmpegPath; got != "/opt/bin/ffmpeg" {
		t.Fatalf("asset_processing.ffmpeg_path = %q", got)
	}
	if got := cfg.AssetProcessing.FFprobePath; got != "/opt/bin/ffprobe" {
		t.Fatalf("asset_processing.ffprobe_path = %q", got)
	}
	if got := cfg.AssetProcessing.MaxConcurrentJobsOrDefault(); got != 4 {
		t.Fatalf("asset_processing.max_concurrent_jobs = %d, want 4", got)
	}
	if got := cfg.AssetProcessing.TempDir; got != "/var/tmp/chatto-assets" {
		t.Fatalf("asset_processing.temp_dir = %q", got)
	}
}

func TestReadConfig_AssetProcessingSettingsFromEnv(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDir) })

	t.Setenv("CHATTO_WEBSERVER_PORT", "4000")
	t.Setenv("CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("CHATTO_CORE_SECRET_KEY", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	t.Setenv("CHATTO_CORE_ASSETS_SIGNING_SECRET", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	t.Setenv("CHATTO_ASSET_PROCESSING_ENABLED", "true")
	t.Setenv("CHATTO_ASSET_PROCESSING_FFMPEG_PATH", "/opt/bin/ffmpeg")
	t.Setenv("CHATTO_ASSET_PROCESSING_FFPROBE_PATH", "/opt/bin/ffprobe")
	t.Setenv("CHATTO_ASSET_PROCESSING_MAX_CONCURRENT_JOBS", "6")
	t.Setenv("CHATTO_ASSET_PROCESSING_TEMP_DIR", "/var/tmp/chatto-assets")

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if !cfg.AssetProcessing.Enabled {
		t.Fatal("asset_processing.enabled = false, want true")
	}
	if got := cfg.AssetProcessing.FFmpegPath; got != "/opt/bin/ffmpeg" {
		t.Fatalf("asset_processing.ffmpeg_path = %q", got)
	}
	if got := cfg.AssetProcessing.FFprobePath; got != "/opt/bin/ffprobe" {
		t.Fatalf("asset_processing.ffprobe_path = %q", got)
	}
	if got := cfg.AssetProcessing.MaxConcurrentJobsOrDefault(); got != 6 {
		t.Fatalf("asset_processing.max_concurrent_jobs = %d, want 6", got)
	}
	if got := cfg.AssetProcessing.TempDir; got != "/var/tmp/chatto-assets" {
		t.Fatalf("asset_processing.temp_dir = %q", got)
	}
}

func TestSearchProviderExplicitEmptyLanguagesSurvivesTOMLParsing(t *testing.T) {
	var cfg ChattoConfig
	if err := toml.Unmarshal([]byte("[search_provider]\nlanguages = []\n"), &cfg); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got := cfg.SearchProvider.LanguagesOrDefault(); got == nil || len(got) != 0 {
		t.Fatalf("parsed empty languages = %#v, want non-nil empty list", got)
	}
}

func TestReadConfig_OperatorAPIFromEnv(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	t.Setenv("CHATTO_WEBSERVER_PORT", "4000")
	t.Setenv("CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("CHATTO_CORE_SECRET_KEY", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	t.Setenv("CHATTO_CORE_ASSETS_SIGNING_SECRET", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	t.Setenv("CHATTO_OPERATOR_API_ENABLED", "true")
	t.Setenv("CHATTO_OPERATOR_API_SOCKET_PATH", "/tmp/chatto-test/operator.sock")

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if !cfg.OperatorAPI.Enabled {
		t.Fatal("OperatorAPI.Enabled = false, want true")
	}
	if got := cfg.OperatorAPI.SocketPathOrDefault(); got != "/tmp/chatto-test/operator.sock" {
		t.Fatalf("OperatorAPI.SocketPathOrDefault() = %q", got)
	}
}

func TestReadConfig_InvalidCookieEncryptionSecretFromEnv(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	t.Setenv("CHATTO_WEBSERVER_PORT", "4000")
	t.Setenv("CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("CHATTO_WEBSERVER_COOKIE_ENCRYPTION_SECRET", "not-hex")
	t.Setenv("CHATTO_CORE_SECRET_KEY", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	t.Setenv("CHATTO_CORE_ASSETS_SIGNING_SECRET", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")

	_, err = ReadConfig("")
	if err == nil || !strings.Contains(err.Error(), "webserver.cookie_encryption_secret must be hex-encoded") {
		t.Fatalf("ReadConfig() error = %v, want cookie encryption validation error", err)
	}
}

func TestReadConfig_GeneralLogFormatFromTOMLAndEnv(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	configContent := `
[general]
log_format = "text"

[webserver]
port = 5000
cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[core]
secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "chatto.toml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if cfg.General.LogFormat != "text" {
		t.Fatalf("expected TOML log_format %q, got %q", "text", cfg.General.LogFormat)
	}

	t.Setenv("CHATTO_LOG_FORMAT", "json")
	cfg, err = ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() with env override failed: %v", err)
	}
	if cfg.General.LogFormat != "json" {
		t.Fatalf("expected env log_format %q, got %q", "json", cfg.General.LogFormat)
	}
}

func TestReadConfig_OAuthRedirectOriginsFromTOMLAndEnv(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	configContent := `
[webserver]
port = 5000
cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
oauth_redirect_origins = ["https://client.example"]
trusted_proxies = ["127.0.0.1", "10.0.0.0/8"]

[core]
secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "chatto.toml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if got, want := strings.Join(cfg.Webserver.OAuthRedirectOrigins, ","), "https://client.example"; got != want {
		t.Fatalf("expected TOML oauth_redirect_origins %q, got %q", want, got)
	}
	if got, want := strings.Join(cfg.Webserver.TrustedProxies, ","), "127.0.0.1,10.0.0.0/8"; got != want {
		t.Fatalf("expected TOML trusted_proxies %q, got %q", want, got)
	}

	t.Setenv("CHATTO_WEBSERVER_OAUTH_REDIRECT_ORIGINS", "*")
	t.Setenv("CHATTO_WEBSERVER_TRUSTED_PROXIES", "192.0.2.10,2001:db8::/32")
	cfg, err = ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() with env override failed: %v", err)
	}
	if got, want := strings.Join(cfg.Webserver.OAuthRedirectOrigins, ","), "*"; got != want {
		t.Fatalf("expected env oauth_redirect_origins %q, got %q", want, got)
	}
	if got, want := strings.Join(cfg.Webserver.TrustedProxies, ","), "192.0.2.10,2001:db8::/32"; got != want {
		t.Fatalf("expected env trusted_proxies %q, got %q", want, got)
	}
}

func TestTLSConfig_CacheDirOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		cacheDir string
		want     string
	}{
		{
			name:     "empty returns default",
			cacheDir: "",
			want:     ".chatto/certs",
		},
		{
			name:     "custom value returned",
			cacheDir: "/var/cache/certs",
			want:     "/var/cache/certs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &TLSConfig{CacheDir: tt.cacheDir}
			if got := c.CacheDirOrDefault(); got != tt.want {
				t.Errorf("CacheDirOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTLSConfig_HTTPPortOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		httpPort int
		want     int
	}{
		{
			name:     "zero returns default 80",
			httpPort: 0,
			want:     80,
		},
		{
			name:     "custom value returned",
			httpPort: 8080,
			want:     8080,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &TLSConfig{HTTPPort: tt.httpPort}
			if got := c.HTTPPortOrDefault(); got != tt.want {
				t.Errorf("HTTPPortOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWebserverConfig_EffectivePort(t *testing.T) {
	tests := []struct {
		name       string
		port       int
		tlsEnabled bool
		want       int
	}{
		{
			name:       "TLS enabled with port 0 returns 443",
			port:       0,
			tlsEnabled: true,
			want:       443,
		},
		{
			name:       "TLS enabled with custom port returns custom",
			port:       8443,
			tlsEnabled: true,
			want:       8443,
		},
		{
			name:       "TLS disabled returns configured port",
			port:       4000,
			tlsEnabled: false,
			want:       4000,
		},
		{
			name:       "TLS disabled with port 0 returns 0",
			port:       0,
			tlsEnabled: false,
			want:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &WebserverConfig{
				Port: tt.port,
				TLS:  TLSConfig{Enabled: tt.tlsEnabled},
			}
			if got := c.EffectivePort(); got != tt.want {
				t.Errorf("EffectivePort() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWebserverConfig_WebSocketCompressionEnabled(t *testing.T) {
	tests := []struct {
		name        string
		compression *bool
		want        bool
	}{
		{
			name:        "nil returns true (default)",
			compression: nil,
			want:        true,
		},
		{
			name:        "true returns true",
			compression: boolPtr(true),
			want:        true,
		},
		{
			name:        "false returns false",
			compression: boolPtr(false),
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &WebserverConfig{WebSocketCompression: tt.compression}
			if got := c.WebSocketCompressionEnabled(); got != tt.want {
				t.Errorf("WebSocketCompressionEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWebserverConfig_APICompression(t *testing.T) {
	tests := []struct {
		name        string
		compression *bool
		minBytes    *int
		wantEnabled bool
		wantMin     int
	}{
		{
			name:        "defaults to enabled above one KiB",
			wantEnabled: true,
			wantMin:     1024,
		},
		{
			name:        "explicitly disabled with custom threshold",
			compression: boolPtr(false),
			minBytes:    intPtr(8192),
			wantEnabled: false,
			wantMin:     8192,
		},
		{
			name:        "zero threshold compresses every non-empty response",
			compression: boolPtr(true),
			minBytes:    intPtr(0),
			wantEnabled: true,
			wantMin:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := WebserverConfig{
				APICompression:         tt.compression,
				APICompressionMinBytes: tt.minBytes,
			}
			if got := cfg.APICompressionEnabled(); got != tt.wantEnabled {
				t.Errorf("APICompressionEnabled() = %v, want %v", got, tt.wantEnabled)
			}
			if got := cfg.APICompressionMinBytesOrDefault(); got != tt.wantMin {
				t.Errorf("APICompressionMinBytesOrDefault() = %d, want %d", got, tt.wantMin)
			}
		})
	}
}

func TestChattoConfig_Validate_APICompressionMinBytes(t *testing.T) {
	cfg := validTestConfig()
	cfg.Webserver.APICompressionMinBytes = intPtr(-1)

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "webserver.api_compression_min_bytes must not be negative") {
		t.Fatalf("Validate() error = %v, want negative API compression threshold error", err)
	}
}

func TestMetricsConfig_Defaults(t *testing.T) {
	cfg := MetricsConfig{}

	if got := cfg.BindAddressOrDefault(); got != "127.0.0.1" {
		t.Errorf("BindAddressOrDefault() = %q, want 127.0.0.1", got)
	}
	if got := cfg.PortOrDefault(); got != 9090 {
		t.Errorf("PortOrDefault() = %d, want 9090", got)
	}
	if got := cfg.PathOrDefault(); got != "/metrics" {
		t.Errorf("PathOrDefault() = %q, want /metrics", got)
	}
}

func TestExporterConfig_Defaults(t *testing.T) {
	cfg := ExporterConfig{}

	if got := cfg.BindAddressOrDefault(); got != "127.0.0.1" {
		t.Errorf("BindAddressOrDefault() = %q, want 127.0.0.1", got)
	}
	if got := cfg.PortOrDefault(); got != 9100 {
		t.Errorf("PortOrDefault() = %d, want 9100", got)
	}
	if got := cfg.PathOrDefault(); got != "/metrics" {
		t.Errorf("PathOrDefault() = %q, want /metrics", got)
	}
	if got := cfg.S3RefreshIntervalOrDefault(); got != 15*time.Minute {
		t.Errorf("S3RefreshIntervalOrDefault() = %s, want 15m", got)
	}
	if got := cfg.S3TimeoutOrDefault(); got != 30*time.Second {
		t.Errorf("S3TimeoutOrDefault() = %s, want 30s", got)
	}
}

func TestReadConfig_MetricsFromTOMLAndEnv(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	configContent := `
[webserver]
port = 5000
cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[metrics]
enabled = true
bind_address = "0.0.0.0"
port = 9100
path = "/internal/metrics"
pprof = true

[core]
secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "chatto.toml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if !cfg.Metrics.Enabled {
		t.Fatal("Metrics.Enabled = false, want true")
	}
	if got := cfg.Metrics.BindAddressOrDefault(); got != "0.0.0.0" {
		t.Errorf("Metrics.BindAddress = %q, want 0.0.0.0", got)
	}
	if got := cfg.Metrics.PortOrDefault(); got != 9100 {
		t.Errorf("Metrics.Port = %d, want 9100", got)
	}
	if got := cfg.Metrics.PathOrDefault(); got != "/internal/metrics" {
		t.Errorf("Metrics.Path = %q, want /internal/metrics", got)
	}
	if !cfg.Metrics.Pprof {
		t.Fatal("Metrics.Pprof = false, want true")
	}

	t.Setenv("CHATTO_METRICS_ENABLED", "false")
	t.Setenv("CHATTO_METRICS_PORT", "9200")
	t.Setenv("CHATTO_METRICS_PATH", "/metrics")
	t.Setenv("CHATTO_METRICS_PPROF", "false")

	cfg, err = ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() with env override failed: %v", err)
	}
	if cfg.Metrics.Enabled {
		t.Fatal("Metrics.Enabled = true, want env override false")
	}
	if got := cfg.Metrics.PortOrDefault(); got != 9200 {
		t.Errorf("Metrics.Port env override = %d, want 9200", got)
	}
	if got := cfg.Metrics.PathOrDefault(); got != "/metrics" {
		t.Errorf("Metrics.Path env override = %q, want /metrics", got)
	}
	if cfg.Metrics.Pprof {
		t.Fatal("Metrics.Pprof = true, want env override false")
	}
}

func TestReadConfig_ExporterFromTOMLAndEnv(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	configContent := `
[webserver]
port = 5000
cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[exporter]
enabled = true
bind_address = "0.0.0.0"
port = 9200
path = "/internal/exporter"
s3_refresh_interval = "30m"
s3_timeout = "45s"

[core]
secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "chatto.toml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if !cfg.Exporter.Enabled {
		t.Fatal("Exporter.Enabled = false, want true")
	}
	if got := cfg.Exporter.BindAddressOrDefault(); got != "0.0.0.0" {
		t.Errorf("Exporter.BindAddress = %q, want 0.0.0.0", got)
	}
	if got := cfg.Exporter.PortOrDefault(); got != 9200 {
		t.Errorf("Exporter.Port = %d, want 9200", got)
	}
	if got := cfg.Exporter.PathOrDefault(); got != "/internal/exporter" {
		t.Errorf("Exporter.Path = %q, want /internal/exporter", got)
	}
	if got := cfg.Exporter.S3RefreshIntervalOrDefault(); got != 30*time.Minute {
		t.Errorf("Exporter.S3RefreshInterval = %s, want 30m", got)
	}
	if got := cfg.Exporter.S3TimeoutOrDefault(); got != 45*time.Second {
		t.Errorf("Exporter.S3Timeout = %s, want 45s", got)
	}

	t.Setenv("CHATTO_EXPORTER_ENABLED", "false")
	t.Setenv("CHATTO_EXPORTER_PORT", "9300")
	t.Setenv("CHATTO_EXPORTER_PATH", "/metrics")
	t.Setenv("CHATTO_EXPORTER_S3_REFRESH_INTERVAL", "5m")
	t.Setenv("CHATTO_EXPORTER_S3_TIMEOUT", "10s")

	cfg, err = ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() with env override failed: %v", err)
	}
	if cfg.Exporter.Enabled {
		t.Fatal("Exporter.Enabled = true, want env override false")
	}
	if got := cfg.Exporter.PortOrDefault(); got != 9300 {
		t.Errorf("Exporter.Port env override = %d, want 9300", got)
	}
	if got := cfg.Exporter.PathOrDefault(); got != "/metrics" {
		t.Errorf("Exporter.Path env override = %q, want /metrics", got)
	}
	if got := cfg.Exporter.S3RefreshIntervalOrDefault(); got != 5*time.Minute {
		t.Errorf("Exporter.S3RefreshInterval env override = %s, want 5m", got)
	}
	if got := cfg.Exporter.S3TimeoutOrDefault(); got != 10*time.Second {
		t.Errorf("Exporter.S3Timeout env override = %s, want 10s", got)
	}
}

func TestReadConfig_DiagnosticsFromTOMLAndEnv(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	configContent := `
[webserver]
port = 5000
cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[diagnostics]
startup_cpu_profile = "startup.pprof"

[core]
secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "chatto.toml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if got := cfg.Diagnostics.StartupCPUProfile; got != "startup.pprof" {
		t.Fatalf("Diagnostics.StartupCPUProfile = %q, want startup.pprof", got)
	}

	t.Setenv("CHATTO_DIAGNOSTICS_STARTUP_CPU_PROFILE", "env-startup.pprof")

	cfg, err = ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() with env override failed: %v", err)
	}
	if got := cfg.Diagnostics.StartupCPUProfile; got != "env-startup.pprof" {
		t.Fatalf("Diagnostics.StartupCPUProfile env override = %q, want env-startup.pprof", got)
	}
}

func TestChattoConfigValidateSearchProviderDirectory(t *testing.T) {
	for _, directory := range []string{".", "./data", "/"} {
		t.Run(directory, func(t *testing.T) {
			cfg := validTestConfig()
			cfg.SearchProvider = SearchProviderConfig{Enabled: true, Directory: directory}
			cfg.NATS.Embedded.DataDir = "./data"
			if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "search_provider.directory") {
				t.Fatalf("Validate() error = %v, want unsafe search provider directory error", err)
			}
		})
	}

	cfg := validTestConfig()
	cfg.SearchProvider = SearchProviderConfig{Enabled: true, Directory: "./data/search"}
	cfg.NATS.Embedded.DataDir = "./data"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestChattoConfigValidateSearchProviderLanguages(t *testing.T) {
	for _, test := range []struct {
		name      string
		languages []string
		wantError string
	}{
		{name: "supported", languages: []string{"cjk", "de", "en"}},
		{name: "explicit empty", languages: []string{}},
		{name: "environment none", languages: []string{"none"}},
		{name: "unsupported", languages: []string{"uk"}, wantError: `unsupported language "uk"`},
		{name: "duplicate", languages: []string{"en", "EN"}, wantError: `duplicate language "en"`},
		{name: "empty code", languages: []string{" "}, wantError: "must not contain empty language codes"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := validTestConfig()
			cfg.SearchProvider.Languages = test.languages
			err := cfg.Validate()
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("Validate() unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Validate() error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestChattoConfig_Validate_OperatorAPI(t *testing.T) {
	t.Run("uses socket defaults", func(t *testing.T) {
		operatorAPI := OperatorAPIConfig{}
		if got := operatorAPI.SocketPathOrDefault(); got != "/tmp/chatto/operator.sock" {
			t.Fatalf("SocketPathOrDefault() = %q", got)
		}
	})

	t.Run("enabled accepts defaults", func(t *testing.T) {
		cfg := validTestConfig()
		cfg.OperatorAPI.Enabled = true
		err := cfg.Validate()
		if err != nil {
			t.Fatalf("Validate() error = %v, want nil", err)
		}
	})

	t.Run("rejects configured socket mode", func(t *testing.T) {
		cfg := validTestConfig()
		cfg.OperatorAPI.Enabled = true
		cfg.OperatorAPI.SocketMode = "0600"
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "operator_api.socket_mode is no longer supported") {
			t.Fatalf("Validate() error = %v, want unsupported socket mode error", err)
		}
	})
}

func TestChattoConfig_Validate_CookieEncryptionSecret(t *testing.T) {
	base := validTestConfig()

	tests := []struct {
		name      string
		secret    string
		wantError string
	}{
		{
			name: "empty is allowed",
		},
		{
			name:   "16 byte key",
			secret: "000102030405060708090a0b0c0d0e0f",
		},
		{
			name:   "24 byte key",
			secret: "000102030405060708090a0b0c0d0e0f1011121314151617",
		},
		{
			name:   "32 byte key",
			secret: "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
		},
		{
			name:      "not hex",
			secret:    "not-hex",
			wantError: "webserver.cookie_encryption_secret must be hex-encoded",
		},
		{
			name:      "wrong decoded length",
			secret:    "000102030405060708090a0b0c0d0e",
			wantError: "webserver.cookie_encryption_secret must decode to 16, 24, or 32 bytes (got 15)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			cfg.Webserver.CookieEncryptionSecret = tt.secret
			err := cfg.Validate()
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("Validate() unexpected error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("Validate() error = %v, want to contain %q", err, tt.wantError)
			}
		})
	}
}

func TestChattoConfig_Validate_LogFormat(t *testing.T) {
	base := validTestConfig()

	for _, format := range []string{"", "auto", "text", "json", "logfmt", "JSON"} {
		t.Run("valid_"+format, func(t *testing.T) {
			cfg := base
			cfg.General.LogFormat = format
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate() unexpected error = %v", err)
			}
		})
	}

	cfg := base
	cfg.General.LogFormat = "pretty"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "general.log_format must be one of: auto, text, json, logfmt") {
		t.Fatalf("Validate() error = %v, want invalid log_format error", err)
	}
}

func TestChattoConfig_Validate_URLsAndOrigins(t *testing.T) {
	tests := []struct {
		name      string
		modify    func(*ChattoConfig)
		wantError string
	}{
		{
			name: "valid webserver URL and origins",
			modify: func(c *ChattoConfig) {
				c.Webserver.URL = "https://chat.example"
				c.Webserver.AllowedOrigins = []string{"https://client.example", "http://localhost:5173", ChattoDesktopOrigin, "*"}
				c.Webserver.OAuthRedirectOrigins = []string{"https://client.example", "http://localhost:5173", ChattoDesktopOrigin, "*"}
				c.Webserver.TrustedProxies = []string{"127.0.0.1", "10.0.0.0/8", "2001:db8::/32"}
			},
		},
		{
			name: "trusted proxy rejects hostnames",
			modify: func(c *ChattoConfig) {
				c.Webserver.TrustedProxies = []string{"proxy.internal"}
			},
			wantError: "webserver.trusted_proxies contains invalid IP address or CIDR",
		},
		{
			name: "webserver URL requires http or https",
			modify: func(c *ChattoConfig) {
				c.Webserver.URL = "chat.example"
			},
			wantError: "webserver.url must use http or https",
		},
		{
			name: "allowed origin rejects paths",
			modify: func(c *ChattoConfig) {
				c.Webserver.AllowedOrigins = []string{"https://client.example/path"}
			},
			wantError: "webserver.allowed_origins contains invalid origin",
		},
		{
			name: "OAuth origin requires https outside loopback",
			modify: func(c *ChattoConfig) {
				c.Webserver.OAuthRedirectOrigins = []string{"http://client.example"}
			},
			wantError: "non-loopback OAuth redirect origins must use https",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validTestConfig()
			tt.modify(&cfg)
			err := cfg.Validate()
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("Validate() unexpected error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("Validate() error = %v, want to contain %q", err, tt.wantError)
			}
		})
	}
}

func TestChattoConfig_Validate_Metrics(t *testing.T) {
	base := validTestConfig()

	tests := []struct {
		name     string
		modify   func(*ChattoConfig)
		errorMsg string
	}{
		{
			name: "accepts enabled metrics with defaults",
			modify: func(c *ChattoConfig) {
				c.Metrics.Enabled = true
			},
		},
		{
			name: "rejects invalid port",
			modify: func(c *ChattoConfig) {
				c.Metrics.Enabled = true
				c.Metrics.Port = 70000
			},
			errorMsg: "metrics.port must be between 0 and 65535",
		},
		{
			name: "rejects relative path",
			modify: func(c *ChattoConfig) {
				c.Metrics.Enabled = true
				c.Metrics.Path = "metrics"
			},
			errorMsg: "metrics.path must start with /",
		},
		{
			name: "rejects query string in path",
			modify: func(c *ChattoConfig) {
				c.Metrics.Enabled = true
				c.Metrics.Path = "/metrics?token=secret"
			},
			errorMsg: "metrics.path must not contain query strings or fragments",
		},
		{
			name: "accepts enabled exporter with defaults",
			modify: func(c *ChattoConfig) {
				c.Exporter.Enabled = true
			},
		},
		{
			name: "rejects exporter invalid port",
			modify: func(c *ChattoConfig) {
				c.Exporter.Enabled = true
				c.Exporter.Port = 70000
			},
			errorMsg: "exporter.port must be between 0 and 65535",
		},
		{
			name: "rejects exporter relative path",
			modify: func(c *ChattoConfig) {
				c.Exporter.Enabled = true
				c.Exporter.Path = "metrics"
			},
			errorMsg: "exporter.path must start with /",
		},
		{
			name: "rejects exporter query string in path",
			modify: func(c *ChattoConfig) {
				c.Exporter.Enabled = true
				c.Exporter.Path = "/metrics?token=secret"
			},
			errorMsg: "exporter.path must not contain query strings or fragments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			tt.modify(&cfg)
			err := cfg.Validate()
			if tt.errorMsg == "" {
				if err != nil {
					t.Fatalf("Validate() unexpected error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.errorMsg) {
				t.Fatalf("Validate() error = %v, want to contain %q", err, tt.errorMsg)
			}
		})
	}
}

func TestChattoConfig_Validate_TLS(t *testing.T) {
	baseConfig := func() ChattoConfig {
		return ChattoConfig{
			Webserver: WebserverConfig{
				Port:                4000,
				CookieSigningSecret: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
			Core: CoreConfig{
				SecretKey: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
				Assets: AssetsConfig{
					SigningSecret: "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
				},
			},
		}
	}

	tests := []struct {
		name      string
		modify    func(*ChattoConfig)
		wantError bool
		errorMsg  string
	}{
		{
			name:      "valid config without TLS",
			modify:    func(c *ChattoConfig) {},
			wantError: false,
		},
		{
			name: "valid config with TLS",
			modify: func(c *ChattoConfig) {
				c.Webserver.TLS.Enabled = true
				c.Webserver.TLS.Domain = "example.com"
				c.Webserver.TLS.Email = "admin@example.com"
			},
			wantError: false,
		},
		{
			name: "TLS enabled without domain fails",
			modify: func(c *ChattoConfig) {
				c.Webserver.TLS.Enabled = true
				c.Webserver.TLS.Email = "admin@example.com"
			},
			wantError: true,
			errorMsg:  "webserver.tls.domain is required when TLS is enabled",
		},
		{
			name: "TLS enabled without email fails",
			modify: func(c *ChattoConfig) {
				c.Webserver.TLS.Enabled = true
				c.Webserver.TLS.Domain = "example.com"
			},
			wantError: true,
			errorMsg:  "webserver.tls.email is required when TLS is enabled",
		},
		{
			name: "port 0 allowed when TLS enabled",
			modify: func(c *ChattoConfig) {
				c.Webserver.Port = 0
				c.Webserver.TLS.Enabled = true
				c.Webserver.TLS.Domain = "example.com"
				c.Webserver.TLS.Email = "admin@example.com"
			},
			wantError: false,
		},
		{
			name: "port 0 not allowed when TLS disabled",
			modify: func(c *ChattoConfig) {
				c.Webserver.Port = 0
			},
			wantError: true,
			errorMsg:  "webserver.port is required when TLS is disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			tt.modify(&cfg)

			err := cfg.Validate()
			if tt.wantError {
				if err == nil {
					t.Error("Validate() expected error, got nil")
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Validate() error = %v, want to contain %v", err, tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}
