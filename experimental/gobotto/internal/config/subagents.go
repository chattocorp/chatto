package config

import (
	"errors"
	"path/filepath"
	"strings"
	"time"
)

// Subagent configures an isolated read-only investigator. Dir is the filesystem
// boundary, resolved relative to the TOML file, never supplied by the model.
type Subagent struct {
	Description  string `toml:"description"`
	Dir          string `toml:"dir"`
	Model        string `toml:"model"`
	SystemPrompt string `toml:"system_prompt"`
	// AgentsMD defaults to enabled. False disables automatic repository instructions.
	AgentsMD        *bool         `toml:"agents_md"`
	Timeout         string        `toml:"timeout"`
	MaxToolCalls    int           `toml:"max_tool_calls"`
	MaxModelSteps   int           `toml:"max_model_steps"`
	TimeoutDuration time.Duration `toml:"-"`
	// RequestTimeout defaults to 90s; explicit 0s disables the HTTP deadline.
	RequestTimeout string `toml:"request_timeout"`
	RequestRetries *int   `toml:"request_retries"`
	MaxAutoResumes *int   `toml:"max_auto_resumes"`
}

func (cfg *Config) validateSubagents(path string) error {
	bad := errors.New("subagents require valid names, description, dir, and nonnegative execution limits")
	if len(cfg.Subagents) > 16 {
		return bad
	}
	for name, s := range cfg.Subagents {
		if !collectionName.MatchString(name) || len(name) > 32 || strings.TrimSpace(s.Description) == "" || strings.TrimSpace(s.Dir) == "" {
			return bad
		}
		if s.Model = strings.TrimSpace(s.Model); s.Model == "" {
			s.Model = cfg.Conversation.Model
		}
		var err error
		s.TimeoutDuration, err = executionTimeout(s.Timeout)
		if err != nil || s.MaxToolCalls < 0 || s.MaxModelSteps < 0 {
			return bad
		}
		if _, err := executionTimeout(s.RequestTimeout); err != nil {
			return bad
		}
		if (s.RequestRetries != nil && *s.RequestRetries < 0) || (s.MaxAutoResumes != nil && *s.MaxAutoResumes < 0) {
			return bad
		}
		if !filepath.IsAbs(s.Dir) {
			s.Dir = filepath.Join(filepath.Dir(path), s.Dir)
		}
		s.Dir, err = filepath.Abs(s.Dir)
		if err != nil {
			return bad
		}
		cfg.Subagents[name] = s
	}
	return nil
}

// RequestPolicy supplies bounded automatic recovery, separately from task limits.
func (s Subagent) RequestPolicy() (time.Duration, int, int) {
	timeout := 90 * time.Second
	if s.RequestTimeout != "" {
		timeout, _ = time.ParseDuration(s.RequestTimeout)
	}
	retries, resumes := 1, 1
	if s.RequestRetries != nil {
		retries = *s.RequestRetries
	}
	if s.MaxAutoResumes != nil {
		resumes = *s.MaxAutoResumes
	}
	return timeout, retries, resumes
}

func executionTimeout(value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil || d < 0 {
		return 0, errors.New("invalid execution timeout")
	}
	return d, nil
}
