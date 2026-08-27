package datacrypto_test

import (
	"bytes"
	"errors"
	"strconv"
	"testing"

	"golang.org/x/crypto/chacha20poly1305"
	"hmans.de/chatto/pkg/datacrypto"
)

func TestSealAndOpen(t *testing.T) {
	key := mustGenerateKey(t)

	for _, plaintext := range [][]byte{nil, {}, []byte("protected data"), bytes.Repeat([]byte{0xa5}, 1<<20)} {
		plaintext := plaintext
		t.Run(testName(len(plaintext)), func(t *testing.T) {
			associatedData := []byte("application-owned context")
			sealed, err := datacrypto.Seal(key, plaintext, associatedData)
			if err != nil {
				t.Fatal(err)
			}
			if len(sealed.Nonce) != datacrypto.NonceSize {
				t.Fatalf("nonce length = %d, want %d", len(sealed.Nonce), datacrypto.NonceSize)
			}

			opened, err := datacrypto.Open(key, sealed.Ciphertext, sealed.Nonce, associatedData)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(opened, plaintext) {
				t.Fatalf("opened plaintext differs")
			}
		})
	}
}

func TestOpenRejectsSubstitutionAndTampering(t *testing.T) {
	key := mustGenerateKey(t)
	sealed, err := datacrypto.Seal(key, []byte("secret"), []byte("account:a"))
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]struct {
		key        []byte
		ciphertext []byte
		nonce      []byte
		aad        []byte
	}{
		"wrong key": {
			key: mustGenerateKey(t), ciphertext: sealed.Ciphertext, nonce: sealed.Nonce, aad: []byte("account:a"),
		},
		"wrong associated data": {
			key: key, ciphertext: sealed.Ciphertext, nonce: sealed.Nonce, aad: []byte("account:b"),
		},
		"tampered ciphertext": {
			key: key, ciphertext: flippedCopy(sealed.Ciphertext), nonce: sealed.Nonce, aad: []byte("account:a"),
		},
		"tampered nonce": {
			key: key, ciphertext: sealed.Ciphertext, nonce: flippedCopy(sealed.Nonce), aad: []byte("account:a"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := datacrypto.Open(test.key, test.ciphertext, test.nonce, test.aad)
			if !errors.Is(err, datacrypto.ErrDecryptionFailed) {
				t.Fatalf("Open error = %v, want ErrDecryptionFailed", err)
			}
		})
	}
}

func TestInvalidSizes(t *testing.T) {
	validKey := mustGenerateKey(t)

	for _, size := range []int{0, datacrypto.KeySize - 1, datacrypto.KeySize + 1} {
		_, err := datacrypto.Seal(make([]byte, size), nil, nil)
		if !errors.Is(err, datacrypto.ErrInvalidKeySize) {
			t.Fatalf("Seal key size %d error = %v, want ErrInvalidKeySize", size, err)
		}
		_, err = datacrypto.WrapKey(validKey, make([]byte, size), nil)
		if !errors.Is(err, datacrypto.ErrInvalidKeySize) {
			t.Fatalf("WrapKey key size %d error = %v, want ErrInvalidKeySize", size, err)
		}
	}

	_, err := datacrypto.Open(validKey, nil, make([]byte, datacrypto.NonceSize-1), nil)
	if !errors.Is(err, datacrypto.ErrInvalidNonceSize) {
		t.Fatalf("Open error = %v, want ErrInvalidNonceSize", err)
	}
}

func TestWrapAndUnwrapKey(t *testing.T) {
	wrappingKey := mustGenerateKey(t)
	key := mustGenerateKey(t)
	aad := []byte("authling:user-key:account-1:epoch-2")

	wrapped, err := datacrypto.WrapKey(wrappingKey, key, aad)
	if err != nil {
		t.Fatal(err)
	}
	unwrapped, err := datacrypto.UnwrapKey(wrappingKey, wrapped.Ciphertext, wrapped.Nonce, aad)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(unwrapped, key) {
		t.Fatal("unwrapped key differs")
	}
}

func TestUnwrapRejectsAuthenticatedNonKeyPayload(t *testing.T) {
	wrappingKey := mustGenerateKey(t)
	aead, err := chacha20poly1305.NewX(wrappingKey)
	if err != nil {
		t.Fatal(err)
	}
	nonce := bytes.Repeat([]byte{1}, datacrypto.NonceSize)
	ciphertext := aead.Seal(nil, nonce, []byte("not a 256-bit key"), nil)

	_, err = datacrypto.UnwrapKey(wrappingKey, ciphertext, nonce, nil)
	if !errors.Is(err, datacrypto.ErrInvalidKeySize) {
		t.Fatalf("UnwrapKey error = %v, want ErrInvalidKeySize", err)
	}
}

func TestRandomValuesAreUnique(t *testing.T) {
	keys := make(map[string]struct{}, 1000)
	nonces := make(map[string]struct{}, 1000)
	key := mustGenerateKey(t)
	for range 1000 {
		generated := mustGenerateKey(t)
		if _, exists := keys[string(generated)]; exists {
			t.Fatal("GenerateKey returned a duplicate")
		}
		keys[string(generated)] = struct{}{}

		sealed, err := datacrypto.Seal(key, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, exists := nonces[string(sealed.Nonce)]; exists {
			t.Fatal("Seal returned a duplicate nonce")
		}
		nonces[string(sealed.Nonce)] = struct{}{}
	}
}

func TestSealDoesNotMutateOrAliasInputs(t *testing.T) {
	key := bytes.Repeat([]byte{1}, datacrypto.KeySize)
	plaintext := []byte("plaintext")
	aad := []byte("associated data")
	wantKey := bytes.Clone(key)
	wantPlaintext := bytes.Clone(plaintext)
	wantAAD := bytes.Clone(aad)

	sealed, err := datacrypto.Seal(key, plaintext, aad)
	if err != nil {
		t.Fatal(err)
	}
	sealed.Ciphertext[0] ^= 1
	sealed.Nonce[0] ^= 1

	if !bytes.Equal(key, wantKey) || !bytes.Equal(plaintext, wantPlaintext) || !bytes.Equal(aad, wantAAD) {
		t.Fatal("Seal output aliases or mutated caller input")
	}
}

func mustGenerateKey(t *testing.T) []byte {
	t.Helper()
	key, err := datacrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != datacrypto.KeySize {
		t.Fatalf("key length = %d, want %d", len(key), datacrypto.KeySize)
	}
	return key
}

func flippedCopy(value []byte) []byte {
	copy := bytes.Clone(value)
	copy[0] ^= 1
	return copy
}

func testName(size int) string {
	switch size {
	case 0:
		return "empty"
	case 1 << 20:
		return "large"
	default:
		return "size_" + strconv.Itoa(size)
	}
}
