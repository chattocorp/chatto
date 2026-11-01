// Package config loads operator-owned Gobotto configuration. Credentials remain
// in the environment and are never part of the TOML file.
package config

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
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
	Titles    *Titles             `toml:"titles"`
	Memory    Memory              `toml:"memory"`
	MCP       MCP                 `toml:"mcp"`
	Subagents map[string]Subagent `toml:"subagents"`
}

// Memory binds fixed collections and an optional directory for managed brains.
type Memory struct {
	Directory   string       `toml:"directory"`
	Collections []Collection `toml:"collections"`
}

// Collection binds a collection with an optional hard read-only limit.
// Relative paths resolve from the configuration file, not the process directory.
type Collection struct {
	Name     string `toml:"name"`
	Path     string `toml:"path"`
	Writable bool   `toml:"writable"`
}

// Titles selects a separate OpenRouter model for one short title per session.
type Titles struct {
	Model        string `toml:"model"`
	SystemPrompt string `toml:"system_prompt"`
}

// Conversation selects the OpenRouter model and operator instructions. The
// system prompt supplements Gobotto's internal protocol and behavior rules.
type Conversation struct {
	// Omitted or zero execution limits are unlimited. Timeout is a Go duration.
	Timeout         string        `toml:"timeout"`
	TimeoutDuration time.Duration `toml:"-"`
	MaxToolCalls    int           `toml:"max_tool_calls"`
	MaxModelSteps   int           `toml:"max_model_steps"`
	MaxDelegations  int           `toml:"max_delegations"`
	// Debounce is a positive Go duration, such as "4s" or "750ms".
	Debounce string `toml:"debounce"`
	// DebounceDuration is the validated startup value derived from Debounce.
	DebounceDuration time.Duration `toml:"-"`
	Model            string        `toml:"model"`
	SystemPrompt     string        `toml:"system_prompt"`
	// MaxOutputTokens optionally caps completions, including provider reasoning.
	// Nil leaves the provider's default in effect.
	MaxOutputTokens *int       `toml:"max_output_tokens"`
	Reasoning       *Reasoning `toml:"reasoning"`
}

// Reasoning contains optional OpenRouter controls. Nil values preserve provider
// defaults; effort and explicit token budgets are alternative controls.
type Reasoning struct {
	Enabled   *bool  `toml:"enabled"`
	Effort    string `toml:"effort"`
	MaxTokens *int   `toml:"max_tokens"`
	Exclude   *bool  `toml:"exclude"`
}

var collectionName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// Load rejects missing files, malformed TOML, unknown settings, and empty model
// IDs. Errors omit parser details because they can contain private prompt text.
func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, errors.New("cannot open Gobotto config; check gobotto.toml or GOBOTTO_CONFIG")
	}
	defer f.Close()
	cfg := Config{Conversation: Conversation{Debounce: "4s"}}
	if toml.NewDecoder(f).DisallowUnknownFields().Decode(&cfg) != nil {
		return Config{}, errors.New("invalid Gobotto config: check TOML syntax and supported settings")
	}
	if err := cfg.MCP.Validate(); err != nil {
		return Config{}, err
	}
	for _, id := range cfg.OwnerUserIDs {
		if id == "" || strings.ContainsFunc(id, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
			return Config{}, errors.New("owner_user_ids must contain nonempty user IDs without whitespace or control characters")
		}
	}
	slices.Sort(cfg.OwnerUserIDs)
	cfg.OwnerUserIDs = slices.Compact(cfg.OwnerUserIDs)
	names := map[string]bool{}
	if cfg.Memory.Directory != "" {
		if !filepath.IsAbs(cfg.Memory.Directory) {
			cfg.Memory.Directory = filepath.Join(filepath.Dir(path), cfg.Memory.Directory)
		}
		cfg.Memory.Directory, err = filepath.Abs(cfg.Memory.Directory)
		if err != nil {
			return Config{}, errors.New("invalid memory directory")
		}
	}
	for i := range cfg.Memory.Collections {
		c := &cfg.Memory.Collections[i]
		if !collectionName.MatchString(c.Name) || names[c.Name] || strings.TrimSpace(c.Path) == "" {
			return Config{}, errors.New("memory collections require unique lowercase names and nonempty paths")
		}
		names[c.Name] = true
		if !filepath.IsAbs(c.Path) {
			c.Path = filepath.Join(filepath.Dir(path), c.Path)
		}
		absolute, err := filepath.Abs(c.Path)
		if err != nil {
			return Config{}, errors.New("invalid memory collection path")
		}
		c.Path = absolute
	}
	cfg.Conversation.Model = strings.TrimSpace(cfg.Conversation.Model)
	cfg.Conversation.DebounceDuration, err = time.ParseDuration(cfg.Conversation.Debounce)
	if err != nil || cfg.Conversation.DebounceDuration <= 0 {
		return Config{}, errors.New("conversation.debounce must be a positive duration, such as 4s or 750ms")
	}
	if cfg.Conversation.MaxOutputTokens != nil && *cfg.Conversation.MaxOutputTokens <= 0 {
		return Config{}, errors.New("conversation.max_output_tokens must be positive")
	}
	if r := cfg.Conversation.Reasoning; r != nil {
		if r.Effort != "" && !slices.Contains([]string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}, r.Effort) {
			return Config{}, errors.New("invalid conversation.reasoning.effort")
		}
		if r.MaxTokens != nil && (r.Effort != "" || *r.MaxTokens <= 0 || (cfg.Conversation.MaxOutputTokens != nil && *r.MaxTokens >= *cfg.Conversation.MaxOutputTokens)) {
			return Config{}, errors.New("reasoning.max_tokens must be positive, below max_output_tokens, and used without effort")
		}
		if r.Enabled != nil && ((!*r.Enabled && (r.MaxTokens != nil || (r.Effort != "" && r.Effort != "none"))) || (*r.Enabled && r.Effort == "none")) {
			return Config{}, errors.New("reasoning.enabled conflicts with the configured effort or token budget")
		}
	}
	if cfg.Conversation.Model == "" {
		return Config{}, errors.New("set conversation.model in Gobotto config")
	}
	cfg.Conversation.TimeoutDuration, err = executionTimeout(cfg.Conversation.Timeout)
	if err != nil || cfg.Conversation.MaxToolCalls < 0 || cfg.Conversation.MaxModelSteps < 0 || cfg.Conversation.MaxDelegations < 0 {
		return Config{}, errors.New("conversation execution limits must be nonnegative; timeout must be a duration")
	}
	if cfg.Titles != nil {
		cfg.Titles.Model = strings.TrimSpace(cfg.Titles.Model)
		if cfg.Titles.Model == "" {
			return Config{}, errors.New("set titles.model or remove the titles section")
		}
	}
	if err := cfg.validateSubagents(path); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
