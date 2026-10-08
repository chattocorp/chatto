package oidcprovider

import "testing"

// TestRuntimeKeysArePinned fails when a runtime-state key changes. Stored keys
// are persisted contracts (ADR-114).
func TestRuntimeKeysArePinned(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	s := &Storage{key: key}
	if got, want := s.derivedKey("request", "abc"), "oidc.request.ad9Zk1CZNYuX8_qWqfBbPwsQegq9yPyzpKNVDMHV5NA"; got != want {
		t.Errorf("derivedKey = %q, want %q", got, want)
	}
}
