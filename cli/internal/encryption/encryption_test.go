package encryption

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/chacha20poly1305"
)

func TestEncryptDecrypt(t *testing.T) {
	tests := []struct {
		name      string
		plaintext string
	}{
		{"empty string", ""},
		{"short message", "Hello, World!"},
		{"unicode", "Hello, 世界! 🌍"},
		{"long message", strings.Repeat("a", 10000)},
		{"with newlines", "Line 1\nLine 2\nLine 3"},
		{"binary-like", "\x00\x01\x02\xff\xfe\xfd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := GenerateKey()
			require.NoError(t, err)

			encrypted, err := Encrypt(key, []byte(tt.plaintext))
			require.NoError(t, err)
			require.NotEqual(t, tt.plaintext, string(encrypted.Ciphertext))
			require.Len(t, encrypted.Nonce, NonceSize)

			decrypted, err := Decrypt(key, encrypted.Ciphertext, encrypted.Nonce)
			require.NoError(t, err)
			require.Equal(t, tt.plaintext, string(decrypted))
		})
	}
}

func TestDecryptWithWrongKey(t *testing.T) {
	key1, err := GenerateKey()
	require.NoError(t, err)
	key2, err := GenerateKey()
	require.NoError(t, err)

	encrypted, err := Encrypt(key1, []byte("secret message"))
	require.NoError(t, err)

	_, err = Decrypt(key2, encrypted.Ciphertext, encrypted.Nonce)
	require.ErrorIs(t, err, ErrDecryptionFailed)
}

func TestDecryptWithTamperedCiphertext(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	encrypted, err := Encrypt(key, []byte("secret message"))
	require.NoError(t, err)

	// Tamper with ciphertext
	encrypted.Ciphertext[0] ^= 0xFF

	_, err = Decrypt(key, encrypted.Ciphertext, encrypted.Nonce)
	require.ErrorIs(t, err, ErrDecryptionFailed)
}

func TestDecryptWithTamperedNonce(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	encrypted, err := Encrypt(key, []byte("secret message"))
	require.NoError(t, err)

	// Tamper with nonce
	encrypted.Nonce[0] ^= 0xFF

	_, err = Decrypt(key, encrypted.Ciphertext, encrypted.Nonce)
	require.ErrorIs(t, err, ErrDecryptionFailed)
}

func TestContentKeyBodyEncryptDecrypt(t *testing.T) {
	kek, err := GenerateKey()
	require.NoError(t, err)
	contentKey, err := GenerateKey()
	require.NoError(t, err)
	aad := []byte("event=E123\x00room=R123\x00author=U123")

	wrapped, err := WrapContentKey(kek, contentKey, []byte("user=U123\x00epoch=1"))
	require.NoError(t, err)
	require.NotEmpty(t, wrapped.EncryptedContentKey)
	require.Len(t, wrapped.Nonce, XNonceSize)

	unwrapped, err := UnwrapContentKey(kek, wrapped.EncryptedContentKey, wrapped.Nonce, []byte("user=U123\x00epoch=1"))
	require.NoError(t, err)
	require.Equal(t, contentKey, unwrapped)

	encrypted, err := EncryptWithContentKey(unwrapped, []byte("secret message"), aad)
	require.NoError(t, err)
	require.NotEqual(t, "secret message", string(encrypted.Ciphertext))
	require.Len(t, encrypted.Nonce, XNonceSize)

	decrypted, err := DecryptWithContentKey(contentKey, encrypted.Ciphertext, encrypted.Nonce, aad)
	require.NoError(t, err)
	require.Equal(t, "secret message", string(decrypted))
}

func TestDecryptWithContentKeyRejectsTamperedAAD(t *testing.T) {
	contentKey, err := GenerateKey()
	require.NoError(t, err)
	encrypted, err := EncryptWithContentKey(contentKey, []byte("secret message"), []byte("event=E123"))
	require.NoError(t, err)

	_, err = DecryptWithContentKey(contentKey, encrypted.Ciphertext, encrypted.Nonce, []byte("event=E456"))
	require.ErrorIs(t, err, ErrDecryptionFailed)
}

func TestUnwrapContentKeyRejectsTamperedWrappedKey(t *testing.T) {
	kek, err := GenerateKey()
	require.NoError(t, err)
	contentKey, err := GenerateKey()
	require.NoError(t, err)
	aad := []byte("event=E123")
	wrapped, err := WrapContentKey(kek, contentKey, aad)
	require.NoError(t, err)
	wrapped.EncryptedContentKey[0] ^= 0xFF

	_, err = UnwrapContentKey(kek, wrapped.EncryptedContentKey, wrapped.Nonce, aad)
	require.ErrorIs(t, err, ErrDecryptionFailed)
}

func TestUnwrapContentKeyRejectsInvalidPlaintextKeySize(t *testing.T) {
	kek, err := GenerateKey()
	require.NoError(t, err)
	aad := []byte("event=E123")
	wrapAEAD, err := chacha20poly1305.NewX(kek)
	require.NoError(t, err)
	nonce, err := randomBytes(XNonceSize)
	require.NoError(t, err)
	ciphertext := wrapAEAD.Seal(nil, nonce, []byte("too-short"), aadForContentKey(aad))

	_, err = UnwrapContentKey(kek, ciphertext, nonce, aad)
	require.ErrorIs(t, err, ErrInvalidKeySize)
}

func TestNonceUniqueness(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	nonces := make(map[string]bool)

	for i := 0; i < 1000; i++ {
		encrypted, err := Encrypt(key, []byte("test"))
		require.NoError(t, err)

		nonceStr := string(encrypted.Nonce)
		require.False(t, nonces[nonceStr], "duplicate nonce generated")
		nonces[nonceStr] = true
	}
}

func TestInvalidKeySize(t *testing.T) {
	shortKey := make([]byte, 16)
	longKey := make([]byte, 64)

	_, err := Encrypt(shortKey, []byte("test"))
	require.ErrorIs(t, err, ErrInvalidKeySize)

	_, err = Encrypt(longKey, []byte("test"))
	require.ErrorIs(t, err, ErrInvalidKeySize)

	validKey, _ := GenerateKey()
	encrypted, _ := Encrypt(validKey, []byte("test"))

	_, err = Decrypt(shortKey, encrypted.Ciphertext, encrypted.Nonce)
	require.ErrorIs(t, err, ErrInvalidKeySize)
}

func TestInvalidNonceSize(t *testing.T) {
	key, _ := GenerateKey()
	encrypted, _ := Encrypt(key, []byte("test"))

	shortNonce := make([]byte, 8)
	longNonce := make([]byte, 16)

	_, err := Decrypt(key, encrypted.Ciphertext, shortNonce)
	require.ErrorIs(t, err, ErrInvalidNonceSize)

	_, err = Decrypt(key, encrypted.Ciphertext, longNonce)
	require.ErrorIs(t, err, ErrInvalidNonceSize)
}

func TestGenerateKey(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)
	require.Len(t, key, KeySize)

	// Keys should be unique
	key2, err := GenerateKey()
	require.NoError(t, err)
	require.NotEqual(t, key, key2)
}
