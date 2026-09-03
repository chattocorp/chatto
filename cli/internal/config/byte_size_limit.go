package config

import (
	"fmt"
	"math"
	"strings"

	"github.com/c2h5oh/datasize"
)

// ByteSizeLimit is a positive byte count or -1 for no limit. It accepts the
// human-readable byte formats used by Chatto configuration, including IEC unit
// names such as MiB.
type ByteSizeLimit int64

// UnmarshalText implements encoding.TextUnmarshaler for TOML and environment
// configuration.
func (l *ByteSizeLimit) UnmarshalText(text []byte) error {
	value := strings.TrimSpace(string(text))
	if value == "-1" {
		*l = -1
		return nil
	}
	if strings.HasPrefix(value, "-") {
		return fmt.Errorf("byte-size limit must be -1 or a non-negative size")
	}

	size, err := datasize.ParseString(normalizeIECByteUnit(value))
	if err != nil {
		return fmt.Errorf("invalid byte-size limit %q: %w", value, err)
	}
	if size.Bytes() > math.MaxInt64 {
		return fmt.Errorf("byte-size limit %q exceeds the supported maximum", value)
	}
	*l = ByteSizeLimit(size.Bytes())
	return nil
}

// Bytes returns the signed byte count. A result of -1 means no limit.
func (l ByteSizeLimit) Bytes() int64 {
	return int64(l)
}

func normalizeIECByteUnit(value string) string {
	for _, unit := range []struct {
		iec    string
		legacy string
	}{
		{iec: "KiB", legacy: "KB"},
		{iec: "MiB", legacy: "MB"},
		{iec: "GiB", legacy: "GB"},
		{iec: "TiB", legacy: "TB"},
		{iec: "PiB", legacy: "PB"},
		{iec: "EiB", legacy: "EB"},
	} {
		if strings.HasSuffix(value, unit.iec) {
			return value[:len(value)-len(unit.iec)] + unit.legacy
		}
	}
	return value
}
