package accounts

import (
	"errors"
	"testing"

	"hmans.de/chatto/pkg/datacrypto"
)

func TestPasswordChangedVerifierRejectsAADSubstitution(t *testing.T) {
	key, err := datacrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(key)
	aad := passwordChangedAAD("evt_change", "acc_example", "key_user", "key_credential")
	sealed, err := datacrypto.Seal(key, []byte("argon2 verifier"), aad)
	if err != nil {
		t.Fatal(err)
	}
	for name, substituted := range map[string][]byte{
		"event type": credentialAAD("evt_change", "acc_example", "key_user", "key_credential", "password-verifier"),
		"event ID":   passwordChangedAAD("evt_other", "acc_example", "key_user", "key_credential"),
		"account":    passwordChangedAAD("evt_change", "acc_other", "key_user", "key_credential"),
		"user key":   passwordChangedAAD("evt_change", "acc_example", "key_other", "key_credential"),
		"data key":   passwordChangedAAD("evt_change", "acc_example", "key_user", "key_other"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := datacrypto.Open(key, sealed.Ciphertext, sealed.Nonce, substituted); !errors.Is(err, datacrypto.ErrDecryptionFailed) {
				t.Fatalf("substitution error = %v, want ErrDecryptionFailed", err)
			}
		})
	}
}
