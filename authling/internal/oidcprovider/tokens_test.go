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

func TestRefreshReuseGrace(t *testing.T) {
	const grace = 10 * time.Second
	now := time.Now().UTC()
	rotated := refreshFamily{Generation: 2, RotatedAt: now, Expires: now.Add(time.Hour), IdleExpires: now.Add(time.Hour)}
	for _, test := range []struct {
		name       string
		family     refreshFamily
		generation uint64
		at         time.Time
		want       refreshUse
	}{
		{"current", rotated, 2, now, refreshCurrent},
		{"previous inside grace", rotated, 1, now.Add(grace - time.Nanosecond), refreshRepeat},
		{"previous at grace end", rotated, 1, now.Add(grace), refreshReuse},
		{"two generations old inside grace", rotated, 0, now, refreshReuse},
		{"future generation", rotated, 3, now, refreshInvalid},
		{"second repeat inside grace", refreshFamily{Generation: 2, RotatedAt: now, Repeated: true, Expires: now.Add(time.Hour), IdleExpires: now.Add(time.Hour)}, 1, now, refreshReuse},
		{"current after repeat", refreshFamily{Generation: 2, RotatedAt: now, Repeated: true, Expires: now.Add(time.Hour), IdleExpires: now.Add(time.Hour)}, 2, now, refreshCurrent},
		{"revoked previous inside grace", refreshFamily{Generation: 2, RotatedAt: now, Expires: now.Add(time.Hour), IdleExpires: now.Add(time.Hour), Revoked: true}, 1, now, refreshInvalid},
		{"expired current", rotated, 2, now.Add(time.Hour), refreshInvalid},
		{"generation zero has no previous", refreshFamily{Expires: now.Add(time.Hour), IdleExpires: now.Add(time.Hour)}, 0, now, refreshCurrent},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.family.use(test.generation, test.at, grace); got != test.want {
				t.Fatalf("use = %d, want %d", got, test.want)
			}
		})
	}
}

func TestRefreshReuseGraceDisabled(t *testing.T) {
	now := time.Now().UTC()
	family := refreshFamily{Generation: 1, RotatedAt: now, Expires: now.Add(time.Hour), IdleExpires: now.Add(time.Hour)}
	if family.use(0, now, 0) != refreshReuse {
		t.Fatal("previous credential repeated with grace disabled")
	}
}

func TestRefreshRequestRevokesPreviousGenerationAfterGrace(t *testing.T) {
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
	s := &Storage{kv: stores.RuntimeState, key: bytes.Repeat([]byte{7}, 32), now: func() time.Time { return now }}
	request := &authRequestState{ClientID: "client", Subject: "acc_grace", GrantID: "grant_grace", Scopes: []string{"openid", "offline_access"}}
	previous, err := s.newRefresh(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	id, _, err := s.parseRefresh(previous)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a completed rotation without a grant store.
	key := s.derivedKey("refresh-family", id)
	entry, err := s.kv.Get(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	var family refreshFamily
	if err := s.open(key, entry.Value(), &family); err != nil {
		t.Fatal(err)
	}
	family.Generation, family.RotatedAt = 1, now
	data, err := s.seal(key, family)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.kv.UpdateWithTTL(t.Context(), key, data, entry.Revision(), time.Hour); err != nil {
		t.Fatal(err)
	}
	current := s.refreshToken(id, 1)

	if _, err := s.refreshRequest(t.Context(), previous, request.ClientID); err != nil {
		t.Fatal("previous credential rejected inside grace")
	}
	if _, err := s.refreshRequest(t.Context(), current, request.ClientID); err != nil {
		t.Fatal("grace repeat revoked family")
	}
	now = now.Add(s.cfg.RefreshReuseGrace())
	if _, err := s.refreshRequest(t.Context(), previous, request.ClientID); err == nil {
		t.Fatal("previous credential accepted after grace")
	}
	if _, err := s.refreshRequest(t.Context(), current, request.ClientID); err == nil {
		t.Fatal("reuse after grace did not revoke family")
	}
}
