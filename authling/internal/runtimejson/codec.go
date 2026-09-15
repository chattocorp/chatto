// Package runtimejson encodes Authling's version-1 encrypted runtime envelopes.
// Callers own encryption keys, associated data, storage, expiry, and validation.
package runtimejson

import (
	"encoding/json"
	"errors"
	"fmt"

	"hmans.de/chatto/pkg/datacrypto"
)

// FieldNames selects an existing envelope spelling. This affects writes only;
// reads accept both spellings, as the original encoding/json readers did.
type FieldNames uint8

const (
	// CapitalizedFields preserves the email workflows' Nonce and Ciphertext fields.
	CapitalizedFields FieldNames = iota
	// LowercaseFields preserves the session and OIDC nonce and ciphertext fields.
	LowercaseFields
)

// ErrInvalidEnvelope indicates malformed JSON or an unsupported envelope version.
var ErrInvalidEnvelope = errors.New("invalid runtime JSON envelope")

// ErrInvalidState indicates that decrypted JSON cannot decode into the destination.
var ErrInvalidState = errors.New("invalid runtime JSON state")

type envelope struct {
	Version    int    `json:"version"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

// Seal encodes value as JSON and encrypts it with caller-supplied associated
// data. Associated data must bind the record to its purpose and storage key.
// The temporary plaintext buffer is cleared before return. No key is retained.
func Seal(key, aad []byte, value any, fields FieldNames) ([]byte, error) {
	if fields != CapitalizedFields && fields != LowercaseFields {
		return nil, fmt.Errorf("invalid runtime JSON field names")
	}
	plain, err := json.Marshal(value)
	defer clear(plain)
	if err != nil {
		return nil, err
	}
	sealed, err := datacrypto.Seal(key, plain, aad)
	if err != nil {
		return nil, err
	}
	if fields == CapitalizedFields {
		return json.Marshal(struct {
			Version           int `json:"version"`
			Nonce, Ciphertext []byte
		}{Version: 1, Nonce: sealed.Nonce, Ciphertext: sealed.Ciphertext})
	}
	return json.Marshal(envelope{Version: 1, Nonce: sealed.Nonce, Ciphertext: sealed.Ciphertext})
}

// Open authenticates an envelope before decoding its JSON into destination.
// It clears the temporary plaintext buffer before return. Callers must discard
// destination on error and validate decoded domain fields before using them.
// Custom JSON unmarshallers must copy any input bytes they retain.
func Open(key, aad, data []byte, destination any) error {
	var sealed envelope
	if err := json.Unmarshal(data, &sealed); err != nil || sealed.Version != 1 {
		return ErrInvalidEnvelope
	}
	plain, err := datacrypto.Open(key, sealed.Ciphertext, sealed.Nonce, aad)
	defer clear(plain)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(plain, destination); err != nil {
		return ErrInvalidState
	}
	return nil
}
