package oidcprovider

import (
	"bytes"
	"testing"
	"time"

	"github.com/chattocorp/chatto/authling/internal/config"
	"github.com/chattocorp/chatto/authling/internal/natsruntime"
	"github.com/chattocorp/chatto/authling/internal/storage"
)

func TestResourceAdmission(t *testing.T) {
	const resource = "https://api.example"
	s := &Storage{cfg: config.OIDCConfig{Resources: []config.OIDCResourceConfig{{URI: resource, AllowCIMD: true}, {URI: "https://private.example"}}}}
	for _, test := range []struct {
		name     string
		client   *Client
		resource string
		allowed  bool
	}{
		{"default public", &Client{Source: ClientSourceConfigured}, "", true},
		{"configured allowlist", &Client{Source: ClientSourceConfigured, Resources: []string{resource}}, resource, true},
		{"configured cannot use CIMD policy", &Client{Source: ClientSourceConfigured}, resource, false},
		{"CIMD opt in", &Client{Source: ClientSourceCIMD}, resource, true},
		{"CIMD private resource", &Client{Source: ClientSourceCIMD}, "https://private.example", false},
		{"unregistered resource", &Client{Source: ClientSourceCIMD}, "https://unknown.example", false},
		{"no prefix matching", &Client{Source: ClientSourceCIMD}, resource + "/other", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := s.checkResource(test.client, test.resource) == nil; got != test.allowed {
				t.Fatal("unexpected resource admission")
			}
		})
	}
}

func TestRefreshFamilyExpiryTamperingAndEncryptedStorage(t *testing.T) {
	connection, err := natsruntime.Open(t.Context(), config.NATSConfig{Embedded: config.EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := connection.Close(); err != nil {
			t.Error(err)
		}
	})
	js, _, err := storage.Open(t.Context(), connection.NATS, 1)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := storage.OpenStores(t.Context(), js, 1)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	s := &Storage{kv: stores.RuntimeState, key: bytes.Repeat([]byte{9}, 32), now: func() time.Time { return now }}
	request := &authRequestState{ClientID: "private-client", Subject: "acc_private", GrantID: "grant_private", Resource: "https://private.example", Scopes: []string{"openid", "offline_access"}}
	token, err := s.newRefresh(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	id, _, err := s.parseRefresh(token)
	if err != nil {
		t.Fatal(err)
	}
	key := s.derivedKey("refresh-family", id)
	entry, err := s.kv.Get(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{token, request.ClientID, request.Subject, request.GrantID, request.Resource} {
		if bytes.Contains(entry.Value(), []byte(secret)) || bytes.Contains([]byte(key), []byte(secret)) {
			t.Fatal("refresh state leaked protected value")
		}
	}
	if _, err := s.refreshRequest(t.Context(), token+"x", request.ClientID); err == nil {
		t.Fatal("tampered token accepted")
	}
	if _, err := s.refreshRequest(t.Context(), token, "another-client"); err == nil {
		t.Fatal("cross-client accepted")
	}
	if _, err := s.refreshRequest(t.Context(), token, request.ClientID); err != nil {
		t.Fatal(err)
	}
	now = now.Add(7 * 24 * time.Hour)
	if _, err := s.refreshRequest(t.Context(), token, request.ClientID); err == nil {
		t.Fatal("idle token accepted")
	}
	// Keep the idle deadline beyond the absolute deadline to prove the independent cap.
	var family refreshFamily
	if err := s.open(key, entry.Value(), &family); err != nil {
		t.Fatal(err)
	}
	family.IdleExpires = family.Expires.Add(time.Hour)
	data, err := s.seal(key, family)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.kv.UpdateWithTTL(t.Context(), key, data, entry.Revision(), time.Hour); err != nil {
		t.Fatal(err)
	}
	now = family.Expires
	if _, err := s.refreshRequest(t.Context(), token, request.ClientID); err == nil {
		t.Fatal("absolute expiry ignored")
	}
}
