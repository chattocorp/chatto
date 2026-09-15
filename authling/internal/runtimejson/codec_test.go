package runtimejson

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"golang.org/x/crypto/chacha20poly1305"
	"hmans.de/chatto/pkg/datacrypto"
)

// legacyRecord reproduces the pre-codec wire format using a fixed test nonce
// and the underlying AEAD, independently of Seal and the production envelope.
func legacyRecord(t *testing.T, key, aad, plain []byte, lowercase bool) []byte {
	t.Helper()
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		t.Fatal(err)
	}
	nonce := bytes.Repeat([]byte{2}, chacha20poly1305.NonceSizeX)
	ciphertext := aead.Seal(nil, nonce, plain, aad)
	fields := map[string]any{"version": 1}
	if lowercase {
		fields["nonce"], fields["ciphertext"] = nonce, ciphertext
	} else {
		fields["Nonce"], fields["Ciphertext"] = nonce, ciphertext
	}
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestLegacyEnvelopeCompatibility(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	plain := []byte(`{"id":"opaque-record","attempts":2}`)
	for _, tc := range []struct {
		name, domain string
		fields       FieldNames
	}{
		{"signup", "authling:runtime:v1", CapitalizedFields},
		{"reset", "authling:password-reset-runtime:v1", CapitalizedFields},
		{"email", "authling:email-change-runtime:v1", CapitalizedFields},
		{"session", "authling:runtime-session:v1", LowercaseFields},
		{"oidc", "authling:oidc-runtime:v1", LowercaseFields},
	} {
		t.Run(tc.name, func(t *testing.T) {
			aad := []byte(tc.domain + "\x00opaque-storage-key")
			old := legacyRecord(t, key, aad, plain, tc.fields == LowercaseFields)
			var got struct {
				ID       string `json:"id"`
				Attempts int    `json:"attempts"`
			}
			if err := Open(key, aad, old, &got); err != nil || got.ID != "opaque-record" || got.Attempts != 2 {
				t.Fatalf("read legacy record: %v", err)
			}
			data, err := Seal(key, aad, json.RawMessage(plain), tc.fields)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatal(err)
			}
			nonceName, ciphertextName := "Nonce", "Ciphertext"
			if tc.fields == LowercaseFields {
				nonceName, ciphertextName = "nonce", "ciphertext"
			}
			if len(fields) != 3 || string(fields["version"]) != "1" || fields[nonceName] == nil || fields[ciphertextName] == nil {
				t.Fatal("new record changed legacy field names or version")
			}
			var nonce, ciphertext []byte
			if err := json.Unmarshal(fields[nonceName], &nonce); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(fields[ciphertextName], &ciphertext); err != nil {
				t.Fatal(err)
			}
			aead, err := chacha20poly1305.NewX(key)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := aead.Open(nil, nonce, ciphertext, aad)
			if err != nil || !bytes.Equal(decoded, plain) {
				t.Fatalf("legacy reader: %v", err)
			}
			if bytes.Contains(data, []byte("opaque-record")) {
				t.Fatal("plaintext exposed in envelope")
			}
		})
	}
}

func TestOpenRejectsMalformedAndSubstitutedRecords(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	aad := []byte("purpose\x00record")
	valid := legacyRecord(t, key, aad, []byte(`{"value":"secret"}`), true)
	mutate := func(change func(*envelope)) []byte {
		var record envelope
		if err := json.Unmarshal(valid, &record); err != nil {
			t.Fatal(err)
		}
		change(&record)
		data, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	for name, data := range map[string][]byte{
		"broken JSON":          []byte(`{`),
		"null":                 []byte(`null`),
		"missing version":      []byte(`{}`),
		"future version":       mutate(func(e *envelope) { e.Version = 2 }),
		"missing nonce":        mutate(func(e *envelope) { e.Nonce = nil }),
		"short nonce":          mutate(func(e *envelope) { e.Nonce = e.Nonce[:1] }),
		"missing ciphertext":   mutate(func(e *envelope) { e.Ciphertext = nil }),
		"altered ciphertext":   mutate(func(e *envelope) { e.Ciphertext[0] ^= 1 }),
		"bad base64":           []byte(`{"version":1,"nonce":"!","ciphertext":"!"}`),
		"invalid plaintext":    legacyRecord(t, key, aad, []byte(`{`), true),
		"wrong plaintext type": legacyRecord(t, key, aad, []byte(`[]`), true),
	} {
		t.Run(name, func(t *testing.T) {
			var state struct{ Value string }
			if err := Open(key, aad, data, &state); err == nil {
				t.Fatal("invalid record accepted")
			}
		})
	}
	for name, changed := range map[string][]byte{"purpose": []byte("other\x00record"), "record key": []byte("purpose\x00other")} {
		t.Run(name, func(t *testing.T) {
			var state any
			if err := Open(key, changed, valid, &state); !errors.Is(err, datacrypto.ErrDecryptionFailed) {
				t.Fatalf("substitution: %v", err)
			}
		})
	}
	var state any
	if err := Open(bytes.Repeat([]byte{3}, 32), aad, valid, &state); !errors.Is(err, datacrypto.ErrDecryptionFailed) {
		t.Fatalf("wrong key: %v", err)
	}
	if _, err := Seal(key[:1], aad, state, LowercaseFields); err == nil {
		t.Fatal("invalid key accepted")
	}
	if _, err := Seal(key, aad, make(chan int), LowercaseFields); err == nil {
		t.Fatal("unencodable state accepted")
	}
}

// A custom unmarshaler can observe the temporary input buffer to verify cleanup.
// Normal callers must copy bytes they retain, as encoding/json requires.
type retainedInput struct {
	input []byte
	err   error
}

func (r *retainedInput) UnmarshalJSON(data []byte) error { r.input = data; return r.err }

func TestOpenClearsPlaintextOnSuccessAndDecodeFailure(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	aad := []byte("purpose\x00record")
	data := legacyRecord(t, key, aad, []byte(`{"value":"secret"}`), true)
	for _, decodeError := range []error{nil, errors.New("injected decode failure")} {
		state := retainedInput{err: decodeError}
		err := Open(key, aad, data, &state)
		if (err != nil) != (decodeError != nil) {
			t.Fatalf("decode error: %v", err)
		}
		if len(state.input) == 0 || !bytes.Equal(state.input, make([]byte, len(state.input))) {
			t.Fatal("temporary plaintext was not cleared")
		}
	}
}
