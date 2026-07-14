package projectionsnapshot

import (
	"bytes"
	"errors"
	"testing"
)

func TestEnvelopeRoundTripAndAuthentication(t *testing.T) {
	secret := bytes.Repeat([]byte{0x42}, 32)
	codec, err := newEnvelopeCodec(secret, bytes.NewReader(bytes.Repeat([]byte{0x11}, 80)))
	if err != nil {
		t.Fatal(err)
	}
	var id [generationIDSize]byte
	copy(id[:], []byte("generation-id-01"))
	sealed, err := codec.seal(id, []byte("private projection metadata"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("private projection metadata")) {
		t.Fatal("ciphertext contains plaintext")
	}
	openedID, plaintext, err := codec.open(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if openedID != id || string(plaintext) != "private projection metadata" {
		t.Fatalf("round trip = %x/%q", openedID, plaintext)
	}

	sealed[len(sealed)-1] ^= 0x80
	if _, _, err := codec.open(sealed); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("tampered envelope error = %v, want ErrInvalidEnvelope", err)
	}
}

func TestEnvelopeRejectsWrongKeyAndMalformedInput(t *testing.T) {
	codecA, _ := newEnvelopeCodec(bytes.Repeat([]byte{1}, 32), bytes.NewReader(bytes.Repeat([]byte{2}, 80)))
	codecB, _ := newEnvelopeCodec(bytes.Repeat([]byte{3}, 32), nil)
	sealed, err := codecA.seal([generationIDSize]byte{1}, []byte("state"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := codecB.open(sealed); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("wrong-key error = %v, want ErrInvalidEnvelope", err)
	}
	for _, malformed := range [][]byte{nil, []byte("short"), append([]byte(nil), sealed[:envelopeHeaderSize]...)} {
		if _, _, err := codecA.open(malformed); !errors.Is(err, ErrInvalidEnvelope) {
			t.Fatalf("malformed envelope error = %v, want ErrInvalidEnvelope", err)
		}
	}
}
