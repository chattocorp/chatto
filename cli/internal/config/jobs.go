package config

import (
	"fmt"
	"time"
)

// JobsConfig sets retention for the shared pending-job stream. Individual
// features can stop retrying earlier through their own job expiry policy.
type JobsConfig struct {
	MaxAge Duration `toml:"max_age,commented" env:"CHATTO_JOBS_MAX_AGE" comment:"Hard retention limit for outstanding jobs, including jobs never picked up by a worker. Default: 7d."`
}

// MaxAgeOrDefault returns the stream's hard retention limit.
func (c JobsConfig) MaxAgeOrDefault() time.Duration {
	if c.MaxAge == 0 {
		return 7 * 24 * time.Hour
	}
	return time.Duration(c.MaxAge)
}

// Validate rejects negative retention. Zero selects the seven-day default.
func (c JobsConfig) Validate() error {
	if c.MaxAgeOrDefault() <= 0 {
		return fmt.Errorf("jobs.max_age must be positive")
	}
	return nil
}
