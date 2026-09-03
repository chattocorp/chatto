package config

import (
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestByteSizeLimitUnmarshalText(t *testing.T) {
	tests := []struct {
		input   string
		want    int64
		wantErr bool
	}{
		{input: "-1", want: -1},
		{input: "1", want: 1},
		{input: "256MiB", want: 256 << 20},
		{input: "256 MB", want: 256 << 20},
		{input: "1GiB", want: 1 << 30},
		{input: "0", want: 0},
		{input: "-2", wantErr: true},
		{input: "invalid", wantErr: true},
		{input: "16Mib", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			var limit ByteSizeLimit
			err := limit.UnmarshalText([]byte(test.input))
			if (err != nil) != test.wantErr {
				t.Fatalf("UnmarshalText(%q) error = %v, want error %v", test.input, err, test.wantErr)
			}
			if err == nil && limit.Bytes() != test.want {
				t.Fatalf("UnmarshalText(%q) = %d, want %d", test.input, limit.Bytes(), test.want)
			}
		})
	}
}

func TestByteSizeLimitTOMLDecoding(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{input: `limit = "256MiB"`, want: 256 << 20},
		{input: `limit = -1`, want: -1},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			var decoded struct {
				Limit *ByteSizeLimit `toml:"limit"`
			}
			if err := toml.Unmarshal([]byte(test.input), &decoded); err != nil {
				t.Fatalf("decode TOML: %v", err)
			}
			if decoded.Limit == nil || decoded.Limit.Bytes() != test.want {
				t.Fatalf("decoded limit = %v, want %d", decoded.Limit, test.want)
			}
		})
	}
}
