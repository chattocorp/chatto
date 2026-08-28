package config

import (
	"testing"
	"time"
)

func TestDuration_UnmarshalText(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		// Extended format with days
		{"7d", 7 * 24 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"30d", 30 * 24 * time.Hour, false},
		{"0d", 0, false},

		// Extended format with weeks
		{"1w", 7 * 24 * time.Hour, false},
		{"2w", 14 * 24 * time.Hour, false},

		// Standard Go duration format
		{"168h", 168 * time.Hour, false},
		{"24h30m", 24*time.Hour + 30*time.Minute, false},
		{"1h", time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"1h30m45s", time.Hour + 30*time.Minute + 45*time.Second, false},

		// Combined formats (go-str2duration supports these)
		{"1d2h", 26 * time.Hour, false},
		{"1w1d", 8 * 24 * time.Hour, false},

		// Invalid formats
		{"invalid", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var d Duration
			err := d.UnmarshalText([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalText(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && d.Duration() != tt.want {
				t.Errorf("UnmarshalText(%q) = %v, want %v", tt.input, d.Duration(), tt.want)
			}
		})
	}
}
