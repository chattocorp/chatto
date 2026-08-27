package config

import (
	"fmt"
	"time"

	str2duration "github.com/xhit/go-str2duration/v2"
)

// Duration is a time.Duration that supports extended parsing including days (d), weeks (w),
// months (mo), and years (y). Examples: "7d", "1w", "168h", "24h30m"
type Duration time.Duration

// UnmarshalText implements encoding.TextUnmarshaler for TOML/env parsing.
func (d *Duration) UnmarshalText(text []byte) error {
	parsed, err := str2duration.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", string(text), err)
	}
	*d = Duration(parsed)
	return nil
}

// Duration returns the underlying time.Duration.
func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}
