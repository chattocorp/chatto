package authentication

import "testing"

// TestRuntimeKeysArePinned fails when a runtime-state key changes. Stored keys
// are persisted contracts (ADR-114).
func TestRuntimeKeysArePinned(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	s := &Service{key: key}
	if got, want := s.attemptKey("login-attempt", "person@example.com"), "login-limit.bPxbVP2sYIhK-KaAE3TeYd2BHxg1lR-kFipUVh3Rb9Y"; got != want {
		t.Errorf("attemptKey = %q, want %q", got, want)
	}
}
