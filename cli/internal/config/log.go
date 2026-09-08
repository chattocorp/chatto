package config

import (
	"fmt"
	"time"
)

// LogConfig sets the retention policy for operational records in LOG.
type LogConfig struct {
	Retention Duration `toml:"retention,commented" env:"CHATTO_CORE_LOG_RETENTION" comment:"Operational log retention. Default: 7d. Expired records cannot be recovered."`
}

// RetentionOrDefault returns the storage lifetime of operational records.
func (c LogConfig) RetentionOrDefault() time.Duration {
	if c.Retention == 0 {
		return 7 * 24 * time.Hour
	}
	return time.Duration(c.Retention)
}

// Validate rejects lifetimes below one second; zero selects the default.
func (c LogConfig) Validate() error {
	if c.RetentionOrDefault() < time.Second {
		return fmt.Errorf("core.log.retention must be at least 1s")
	}
	return nil
}
