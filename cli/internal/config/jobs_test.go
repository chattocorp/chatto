package config

import (
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"
)

func TestJobsRetentionPolicy(t *testing.T) {
	require.Equal(t, 7*24*time.Hour, (JobsConfig{}).MaxAgeOrDefault())
	var cfg ChattoConfig
	require.NoError(t, toml.Unmarshal([]byte("[jobs]\nmax_age='14d'\n"), &cfg))
	require.Equal(t, 14*24*time.Hour, cfg.Jobs.MaxAgeOrDefault())
	require.NoError(t, cfg.Jobs.Validate())
	require.Error(t, (JobsConfig{MaxAge: Duration(-time.Second)}).Validate())
}
