// Package appconfig loads application-owned configuration structs from TOML
// files with environment-variable overrides.
package appconfig

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"github.com/caarlos0/env/v11"
	"github.com/pelletier/go-toml/v2"
)

// Options controls file selection and TOML compatibility policy. Configuration
// schemas, environment names, defaults, normalization, and validation remain
// the caller's responsibility.
type Options struct {
	// Path is an explicitly selected configuration file. When empty,
	// DefaultPath is used instead.
	Path string

	// DefaultPath is the application-owned conventional configuration path.
	// When both paths are empty, loading proceeds from the environment only.
	DefaultPath string

	// RequireExplicitFile makes a missing Path an error. A missing DefaultPath
	// remains allowed for environment-only deployments.
	RequireExplicitFile bool

	// DisallowUnknownFields rejects TOML keys that do not map to the target
	// configuration type.
	DisallowUnknownFields bool
}

// Load decodes TOML into T and then applies environment-variable overrides
// described by T's env tags. It returns a zero T on every error so callers
// cannot accidentally use partially decoded configuration.
func Load[T any](options Options) (T, error) {
	var config T
	explicitPath := options.Path != ""
	path := options.Path
	if !explicitPath {
		path = options.DefaultPath
	}

	if path != "" {
		data, err := os.ReadFile(path)
		switch {
		case err == nil:
			if err := decodeTOML(data, &config, options.DisallowUnknownFields); err != nil {
				var zero T
				return zero, fmt.Errorf("decode %s: %w", path, err)
			}
		case errors.Is(err, os.ErrNotExist) && (!explicitPath || !options.RequireExplicitFile):
		case err != nil:
			var zero T
			return zero, fmt.Errorf("read %s: %w", path, err)
		}
	}

	if err := env.Parse(&config); err != nil {
		var zero T
		return zero, fmt.Errorf("parse environment: %w", err)
	}
	return config, nil
}

func decodeTOML(data []byte, target any, disallowUnknownFields bool) error {
	if !disallowUnknownFields {
		return toml.Unmarshal(data, target)
	}
	decoder := toml.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}
