// SPDX-License-Identifier: AGPL-3.0-or-later

package publiccursor

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"
)

func TestSealOpenRoundTripAndConfidentiality(t *testing.T) {
	payload := []byte("EVT:incarnation:sequence:18446744073709551615")
	first, err := Seal("server-secret", "realtime", "viewer-1", payload)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	second, err := Seal("server-secret", "realtime", "viewer-1", payload)
	if err != nil {
		t.Fatalf("Seal second: %v", err)
	}
	if first == second {
		t.Fatal("Seal returned deterministic tokens; want randomized nonces")
	}
	envelope, err := base64.RawURLEncoding.DecodeString(first)
	if err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if bytes.Contains(envelope, payload) || bytes.Contains(envelope, []byte("EVT")) {
		t.Fatalf("sealed envelope reveals plaintext storage coordinate: %q", envelope)
	}
	got, err := Open("server-secret", "realtime", "viewer-1", first)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("Open payload = %q, want %q", got, payload)
	}
}

func TestOpenRejectsWrongBindingAndTampering(t *testing.T) {
	token, err := Seal("server-secret", "realtime", "viewer-1", []byte("coordinate"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	for name, binding := range map[string][3]string{
		"secret":  {"other-secret", "realtime", "viewer-1"},
		"purpose": {"server-secret", "timeline", "viewer-1"},
		"scope":   {"server-secret", "realtime", "viewer-2"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Open(binding[0], binding[1], binding[2], token); !errors.Is(err, ErrInvalid) {
				t.Fatalf("Open error = %v, want ErrInvalid", err)
			}
		})
	}

	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("decode token: %v", err)
	}
	raw[len(raw)-1] ^= 1
	tampered := base64.RawURLEncoding.EncodeToString(raw)
	if _, err := Open("server-secret", "realtime", "viewer-1", tampered); !errors.Is(err, ErrInvalid) {
		t.Fatalf("tampered Open error = %v, want ErrInvalid", err)
	}
}
