package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSubagentConfiguration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gobotto.toml")
	base := "[conversation]\nmodel='test/model'\n[subagents.code]\ndescription='Investigate code'\ndir='../repo'\n"
	for _, tc := range []struct {
		suffix string
		valid  bool
	}{
		{"", true}, {"model='test/worker'\ntimeout='90s'\nmax_tool_calls=12", true},
		{"agents_md=false", true}, {"agents_md=true", true}, {"agents_md='false'", false},
		{"request_timeout='3m'\nrequest_retries=2\nmax_auto_resumes=3", true},
		{"request_timeout='0s'\nrequest_retries=0\nmax_auto_resumes=0", true},
		{"request_timeout='bad'", false}, {"request_timeout='-1s'", false}, {"request_retries=-1", false}, {"max_auto_resumes=-1", false},
		{"timeout='0s'", true}, {"timeout='30m'", true}, {"max_tool_calls=1000\nmax_model_steps=2000", true}, {"max_tool_calls=-1", false}, {"timeout='-1s'", false}, {"timeout='bad'", false}, {"max_model_steps=-1", false}, {"pwd='.'", false},
	} {
		if err := os.WriteFile(path, []byte(base+tc.suffix), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if (err == nil) != tc.valid {
			t.Fatal(tc.suffix, err)
		}
		if err == nil && tc.suffix == "" {
			s := cfg.Subagents["code"]
			if s.Dir != filepath.Join(filepath.Dir(dir), "repo") || s.Model != "test/model" || s.TimeoutDuration != 0 || s.MaxToolCalls != 0 || s.MaxModelSteps != 0 || s.AgentsMD != nil {
				t.Fatal("bad defaults", s)
			}
			if timeout, retries, resumes := s.RequestPolicy(); timeout != 90*time.Second || retries != 1 || resumes != 1 {
				t.Fatal("bad request defaults")
			}
		}
		if err == nil && tc.suffix == "agents_md=false" && (cfg.Subagents["code"].AgentsMD == nil || *cfg.Subagents["code"].AgentsMD) {
			t.Fatal("instruction opt-out lost")
		}
	}
	for _, field := range []string{"description='Investigate code'\n", "dir='../repo'\n"} {
		if err := os.WriteFile(path, []byte("[conversation]\nmodel='test/model'\n[subagents.code]\n"+field), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatal("missing required field accepted")
		}
	}
}

func TestExplicitRecoveryOptOut(t *testing.T) {
	zero := 0
	timeout, retries, resumes := (Subagent{RequestTimeout: "0s", RequestRetries: &zero, MaxAutoResumes: &zero}).RequestPolicy()
	if timeout != 0 || retries != 0 || resumes != 0 {
		t.Fatal(timeout, retries, resumes)
	}
}
