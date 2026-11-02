package connectapi

import (
	"regexp"
	"testing"
)

func TestAvailableExternalIdentityLoginKeepsAvailableHint(t *testing.T) {
	env := newConnectAPITestEnv(t)
	if got := availableExternalIdentityLogin(env.core, "flow-token", "alice"); got != "alice" {
		t.Fatalf("available hint = %q, want alice", got)
	}
}

func TestAvailableExternalIdentityLoginSuffixesUnavailableHint(t *testing.T) {
	env := newConnectAPITestEnv(t)
	first := availableExternalIdentityLogin(env.core, "flow-token", "admin")
	second := availableExternalIdentityLogin(env.core, "flow-token", "admin")
	if first != second {
		t.Fatalf("suggestion changed across reads: %q then %q", first, second)
	}
	if !regexp.MustCompile(`^admin-[0-9]{4}$`).MatchString(first) {
		t.Fatalf("suffixed hint = %q", first)
	}
	if !env.core.IsLoginAvailable(first) {
		t.Fatalf("suffixed hint %q is unavailable", first)
	}
}
