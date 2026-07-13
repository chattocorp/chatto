// Package projectionsnapshot persists encrypted, disposable projection state.
// EVT remains authoritative; callers must treat every error as a cold-replay
// signal rather than a server-startup failure.
package projectionsnapshot

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

const (
	envelopeVersion    byte = 1
	keySchemeV1        byte = 1
	saltSize                = 16
	generationIDSize        = 16
	envelopeHeaderSize      = 8 + 1 + 1 + saltSize + chacha20poly1305.NonceSizeX + generationIDSize
)

var (
	envelopeMagic = [8]byte{'C', 'H', 'T', 'S', 'N', 'A', 'P', 0}
	keyInfo       = []byte("chatto/projection-snapshot/core-secret-hkdf-v1")

	ErrInvalidEnvelope = errors.New("invalid projection snapshot envelope")
)

type envelopeCodec struct {
	secret []byte
	rand   io.Reader
}

func newEnvelopeCodec(secret []byte, random io.Reader) (*envelopeCodec, error) {
	if len(secret) != 32 {
		return nil, fmt.Errorf("snapshot secret must be 32 bytes")
	}
	if random == nil {
		random = rand.Reader
	}
	return &envelopeCodec{secret: append([]byte(nil), secret...), rand: random}, nil
}

func (c *envelopeCodec) seal(generationID [generationIDSize]byte, plaintext []byte) ([]byte, error) {
	header := make([]byte, envelopeHeaderSize)
	copy(header[:8], envelopeMagic[:])
	header[8] = envelopeVersion
	header[9] = keySchemeV1
	salt := header[10 : 10+saltSize]
	nonce := header[10+saltSize : 10+saltSize+chacha20poly1305.NonceSizeX]
	copy(header[envelopeHeaderSize-generationIDSize:], generationID[:])
	if _, err := io.ReadFull(c.rand, salt); err != nil {
		return nil, fmt.Errorf("generate snapshot salt: %w", err)
	}
	if _, err := io.ReadFull(c.rand, nonce); err != nil {
		return nil, fmt.Errorf("generate snapshot nonce: %w", err)
	}
	aead, err := c.aead(salt)
	if err != nil {
		return nil, err
	}
	return aead.Seal(header, nonce, plaintext, header), nil
}

func (c *envelopeCodec) open(data []byte) ([generationIDSize]byte, []byte, error) {
	var generationID [generationIDSize]byte
	if len(data) < envelopeHeaderSize+chacha20poly1305.Overhead {
		return generationID, nil, ErrInvalidEnvelope
	}
	header := data[:envelopeHeaderSize]
	if string(header[:8]) != string(envelopeMagic[:]) || header[8] != envelopeVersion || header[9] != keySchemeV1 {
		return generationID, nil, ErrInvalidEnvelope
	}
	salt := header[10 : 10+saltSize]
	nonce := header[10+saltSize : 10+saltSize+chacha20poly1305.NonceSizeX]
	copy(generationID[:], header[envelopeHeaderSize-generationIDSize:])
	aead, err := c.aead(salt)
	if err != nil {
		return generationID, nil, err
	}
	plaintext, err := aead.Open(nil, nonce, data[envelopeHeaderSize:], header)
	if err != nil {
		return generationID, nil, fmt.Errorf("%w: authentication failed", ErrInvalidEnvelope)
	}
	return generationID, plaintext, nil
}

func (c *envelopeCodec) aead(salt []byte) (cipherAEAD, error) {
	reader := hkdf.New(sha256.New, c.secret, salt, keyInfo)
	key := make([]byte, chacha20poly1305.KeySize)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, fmt.Errorf("derive snapshot key: %w", err)
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("create snapshot cipher: %w", err)
	}
	return aead, nil
}

// cipherAEAD is the subset returned by cipher.AEAD. Keeping the interface local
// makes the key derivation helper easy to read without exporting crypto details.
type cipherAEAD interface {
	Seal(dst, nonce, plaintext, additionalData []byte) []byte
	Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
}

func generationIDString(id [generationIDSize]byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, generationIDSize*2)
	for i, b := range id {
		out[i*2] = hex[b>>4]
		out[i*2+1] = hex[b&0x0f]
	}
	return string(out)
}

func parseGenerationID(value string) ([generationIDSize]byte, error) {
	var id [generationIDSize]byte
	if len(value) != generationIDSize*2 {
		return id, fmt.Errorf("invalid generation id length")
	}
	for i := range id {
		hi, ok := hexNibble(value[i*2])
		if !ok {
			return id, fmt.Errorf("invalid generation id")
		}
		lo, ok := hexNibble(value[i*2+1])
		if !ok {
			return id, fmt.Errorf("invalid generation id")
		}
		id[i] = hi<<4 | lo
	}
	return id, nil
}

func hexNibble(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, true
	default:
		return 0, false
	}
}

func opaqueLocator(secret []byte, projectionKey string) string {
	h := hmac.New(sha256.New, secret)
	_, _ = h.Write([]byte("chatto/projection-snapshot/pointer-v1\x00"))
	_, _ = h.Write([]byte(projectionKey))
	sum := h.Sum(nil)
	return fmt.Sprintf("%x", sum[:8])
}
