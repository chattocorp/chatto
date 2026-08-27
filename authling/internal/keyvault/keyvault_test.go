package keyvault

import (
	"bytes"
	"testing"

	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/natsruntime"
	"hmans.de/authling/internal/storage"
)

func TestOIDCTokenAndSigningKeysHaveIndependentDurableLifecycles(t *testing.T) {
	connection, err := natsruntime.Open(t.Context(), config.NATSConfig{Embedded: config.EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := connection.Close(); err != nil {
			t.Errorf("close NATS: %v", err)
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
	vault := New(stores.Keys)

	initialTokenKey := bytes.Repeat([]byte{7}, 32)
	tokenKey, err := vault.OIDCTokenKey(t.Context(), initialTokenKey)
	if err != nil {
		t.Fatal(err)
	}
	reopenedTokenKey, err := vault.OIDCTokenKey(t.Context(), bytes.Repeat([]byte{8}, 32))
	if err != nil || !bytes.Equal(tokenKey, reopenedTokenKey) {
		t.Fatalf("reopened OIDC token key differs: %v", err)
	}

	const ref = "system.oidc-signing.key_test"
	key, err := vault.EnsureOIDCSigningKey(t.Context(), ref)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := vault.EnsureOIDCSigningKey(t.Context(), ref)
	if err != nil || reopened.ID != key.ID || reopened.Ref != ref {
		t.Fatalf("reopened signing key = %+v, %v; want kid %q", reopened, err, key.ID)
	}
	if err := vault.DestroyOIDCSigningKey(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	if err := vault.DestroyOIDCSigningKey(t.Context(), ref); err != nil {
		t.Fatalf("repeat signing-key destruction: %v", err)
	}
	if _, err := vault.ResolveOIDCSigningKey(t.Context(), ref); err == nil {
		t.Fatal("destroyed signing key remained resolvable")
	}
	if got, err := vault.OIDCTokenKey(t.Context(), bytes.Repeat([]byte{9}, 32)); err != nil || !bytes.Equal(got, tokenKey) {
		t.Fatalf("signing-key destruction changed token key: %v", err)
	}
}

func TestOIDCSigningKeyReferencesRejectStorageCoordinates(t *testing.T) {
	for _, ref := range []string{"", "system.oidc-signing.", "system.oidc-signing.bad.ref", "other.key"} {
		if validOIDCSigningKeyRef(ref) {
			t.Fatalf("accepted invalid OIDC signing-key reference %q", ref)
		}
	}
	if !validOIDCSigningKeyRef(systemOIDCSigningKey) || !validOIDCSigningKeyRef("system.oidc-signing.key_safe") {
		t.Fatal("rejected valid OIDC signing-key reference")
	}
}
