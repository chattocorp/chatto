package config

import (
	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestLogOperatorPolicy(t *testing.T) {
	require.Equal(t, 7*24*time.Hour, (LogConfig{}).RetentionOrDefault())
	var cfg ChattoConfig
	require.NoError(t, toml.Unmarshal([]byte("[core.log]\nretention='2d'\n"), &cfg))
	require.Equal(t, 48*time.Hour, cfg.Core.Log.RetentionOrDefault())
	require.NoError(t, cfg.Core.Log.Validate())
	require.Error(t, (LogConfig{Retention: Duration(-time.Hour)}).Validate())
	require.Error(t, (LogConfig{Retention: Duration(time.Millisecond)}).Validate())
	t.Setenv("CHATTO_WEBSERVER_PORT", "4000")
	t.Setenv("CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("CHATTO_CORE_SECRET_KEY", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	t.Setenv("CHATTO_CORE_ASSETS_SIGNING_SECRET", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	t.Setenv("CHATTO_CORE_LOG_RETENTION", "3d")
	loaded, err := ReadConfig("")
	require.NoError(t, err)
	require.Equal(t, 72*time.Hour, loaded.Core.Log.RetentionOrDefault())
}
