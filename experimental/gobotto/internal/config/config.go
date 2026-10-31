// Package config loads operator-owned Gobotto configuration. Credentials remain
// in the environment and are never part of the TOML file.
package config

import (
	"errors"
	"os"
	"slices"
	"strings"
	"unicode"

	"github.com/pelletier/go-toml/v2"
)

// Config contains settings applied at startup, including to restored sessions.
type Config struct {
	// OwnerUserIDs identifies owners by exact Chatto user ID, never by display name.
	// Omission or an empty list means that no participant is an owner.
	OwnerUserIDs []string     `toml:"owner_user_ids"`
	Conversation Conversation `toml:"conversation"`
	// Titles is optional; omission disables automatic session titles.
	Titles *Titles `toml:"titles"`
}

// Titles selects a separate OpenRouter model for one short title per session.
type Titles struct {
	Model        string `toml:"model"`
	SystemPrompt string `toml:"system_prompt"`
}

// Conversation selects the OpenRouter model and operator instructions. The
// system prompt supplements Gobotto's internal protocol and behavior rules.
type Conversation struct {
	Model        string `toml:"model"`
	SystemPrompt string `toml:"system_prompt"`
}

// Load rejects missing files, malformed TOML, unknown settings, and empty model
// IDs. Errors omit parser details because they can contain private prompt text.
func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, errors.New("cannot open Gobotto config; check gobotto.toml or GOBOTTO_CONFIG")
	}
	defer f.Close()
	var cfg Config
	if toml.NewDecoder(f).DisallowUnknownFields().Decode(&cfg) != nil {
		return Config{}, errors.New("invalid Gobotto config: check TOML syntax and supported settings")
	}
	for _, id := range cfg.OwnerUserIDs {
		if id == "" || strings.ContainsFunc(id, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
			return Config{}, errors.New("owner_user_ids must contain nonempty user IDs without whitespace or control characters")
		}
	}
	slices.Sort(cfg.OwnerUserIDs)
	cfg.OwnerUserIDs = slices.Compact(cfg.OwnerUserIDs)
	cfg.Conversation.Model = strings.TrimSpace(cfg.Conversation.Model)
	if cfg.Conversation.Model == "" {
		return Config{}, errors.New("set conversation.model in Gobotto config")
	}
	if cfg.Titles != nil {
		cfg.Titles.Model = strings.TrimSpace(cfg.Titles.Model)
		if cfg.Titles.Model == "" {
			return Config{}, errors.New("set titles.model or remove the titles section")
		}
	}
	return cfg, nil
}
