package sessions

import "testing"

// TestRuntimeKeysArePinned fails when a runtime-state key changes. Stored keys
// are persisted contracts (ADR-114).
func TestRuntimeKeysArePinned(t *testing.T) {
	s := &Service{key: pinnedTestKey()}
	if got, want := s.sessionKey("tok"), "session.IPjyNeYa2NSsd74PJ20hJztixpL6tGoTtSv4vVph914"; got != want {
		t.Errorf("sessionKey = %q, want %q", got, want)
	}
}

func pinnedTestKey() []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	return key
}
