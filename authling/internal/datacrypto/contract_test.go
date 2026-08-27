// Package datacrypto_test exercises Authling's planned cryptographic hierarchy
// against the shared primitive without claiming a production implementation.
package datacrypto_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"hmans.de/chatto/pkg/datacrypto"
)

func TestHierarchicalKeyConsumerContract(t *testing.T) {
	userKey := generateKey(t)
	dataKey := generateKey(t)
	accountID := "acct_example"

	keyAAD := authlingAAD("authling:user-data-key:v1", accountID, "profile", "epoch-1")
	wrappedDataKey, err := datacrypto.WrapKey(userKey, dataKey, keyAAD)
	if err != nil {
		t.Fatal(err)
	}
	unwrappedDataKey, err := datacrypto.UnwrapKey(
		userKey,
		wrappedDataKey.Ciphertext,
		wrappedDataKey.Nonce,
		keyAAD,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(unwrappedDataKey, dataKey) {
		t.Fatal("unwrapped data key differs")
	}

	fieldAAD := authlingAAD(
		"authling:protected-field:v1",
		"event-type:account-profile-updated",
		accountID,
		"purpose:core-profile",
		"epoch:1",
		"evt_example",
		"email",
	)
	sealed, err := datacrypto.Seal(dataKey, []byte("person@example.invalid"), fieldAAD)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := datacrypto.Open(dataKey, sealed.Ciphertext, sealed.Nonce, fieldAAD); err != nil {
		t.Fatal(err)
	}

	for name, substitutedAAD := range map[string][]byte{
		"event type": authlingAAD("authling:protected-field:v1", "event-type:credential-linked", accountID, "purpose:core-profile", "epoch:1", "evt_example", "email"),
		"account":    authlingAAD("authling:protected-field:v1", "event-type:account-profile-updated", "acct_other", "purpose:core-profile", "epoch:1", "evt_example", "email"),
		"purpose":    authlingAAD("authling:protected-field:v1", "event-type:account-profile-updated", accountID, "purpose:credentials", "epoch:1", "evt_example", "email"),
		"epoch":      authlingAAD("authling:protected-field:v1", "event-type:account-profile-updated", accountID, "purpose:core-profile", "epoch:2", "evt_example", "email"),
		"event ID":   authlingAAD("authling:protected-field:v1", "event-type:account-profile-updated", accountID, "purpose:core-profile", "epoch:1", "evt_other", "email"),
		"field":      authlingAAD("authling:protected-field:v1", "event-type:account-profile-updated", accountID, "purpose:core-profile", "epoch:1", "evt_example", "display-name"),
	} {
		t.Run("reject "+name+" substitution", func(t *testing.T) {
			_, err := datacrypto.Open(dataKey, sealed.Ciphertext, sealed.Nonce, substitutedAAD)
			if !errors.Is(err, datacrypto.ErrDecryptionFailed) {
				t.Fatalf("substitution error = %v, want ErrDecryptionFailed", err)
			}
		})
	}

	wrongKeyAAD := authlingAAD("authling:user-data-key:v1", "acct_other", "profile", "epoch-1")
	_, err = datacrypto.UnwrapKey(userKey, wrappedDataKey.Ciphertext, wrappedDataKey.Nonce, wrongKeyAAD)
	if !errors.Is(err, datacrypto.ErrDecryptionFailed) {
		t.Fatalf("key substitution error = %v, want ErrDecryptionFailed", err)
	}
}

func authlingAAD(domain string, fields ...string) []byte {
	result := appendLengthPrefixed(nil, domain)
	for _, field := range fields {
		result = appendLengthPrefixed(result, field)
	}
	return result
}

func appendLengthPrefixed(destination []byte, value string) []byte {
	destination = binary.BigEndian.AppendUint32(destination, uint32(len(value)))
	return append(destination, value...)
}

func generateKey(t *testing.T) []byte {
	t.Helper()
	key, err := datacrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return key
}
