package passwordreset

import "testing"

// TestRuntimeKeysArePinned fails when a runtime-state key changes. Stored keys
// are persisted contracts (ADR-114).
func TestRuntimeKeysArePinned(t *testing.T) {
	s := &Service{key: pinnedTestKey()}
	if got, want := s.flowKey("tok"), "password-reset._-lZwUofORyymoWahdIWQxFCvitkYaooIZap9_RnffE"; got != want {
		t.Errorf("flowKey = %q, want %q", got, want)
	}
	if got, want := s.deliveryKey("person@example.com"), "password-reset-limit.L90KokmxnCim3bQhsTH2qCemEyiyTKWlND3_Qatt14g"; got != want {
		t.Errorf("deliveryKey = %q, want %q", got, want)
	}
	if got, want := s.admissionKey("person@example.com"), "password-reset-admission.AO7chzv8wRoxGHpwFjHMAgAmHZ_w_0cOUYJkf9PFEhE"; got != want {
		t.Errorf("admissionKey = %q, want %q", got, want)
	}
}

func pinnedTestKey() []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	return key
}
