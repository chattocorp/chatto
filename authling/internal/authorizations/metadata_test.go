package authorizations

import (
	"bytes"
	"testing"

	"google.golang.org/protobuf/proto"
	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/keyvault"
	"hmans.de/authling/internal/natsruntime"
	corev1 "hmans.de/authling/internal/pb/authling/core/v1"
	"hmans.de/authling/internal/storage"
)

func TestProtectedMetadataBindsAuthorizationContext(t *testing.T) {
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
	vault := keyvault.New(stores.Keys)
	op, userRef, dataRef, key, err := vault.ProvisionCredentialKeys(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	clear(key)
	if err := vault.CompleteProvisioning(t.Context(), op); err != nil {
		t.Fatal(err)
	}
	service := &Service{vault: vault}
	event := grantAuthorized("evt_protected", "grant_one", "")
	client := Client{ID: "private-client", Name: "Private Person's App", Host: "private-person.example"}
	if err := service.sealMetadata(t.Context(), event, accountKeys{userRef, dataRef}, client); err != nil {
		t.Fatal(err)
	}
	raw, err := proto.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{client.ID, client.Name, client.Host} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatal("event leaked client metadata")
		}
	}
	projection := NewProjection()
	created := accountCreated("evt_account")
	created.GetAccountCreated().UserKeyRef = userRef
	created.GetAccountCreated().CredentialKeyRef = dataRef
	if err := projection.Apply(created, 1); err != nil {
		t.Fatal(err)
	}
	if err := projection.Apply(event, 2); err != nil {
		t.Fatal(err)
	}
	projected := projection.list("acc_one")[0]
	if projected.ClientName != "" || projected.ClientHost != "" {
		t.Fatal("projection retained plaintext metadata")
	}
	opened, err := service.openMetadata(t.Context(), projected)
	if err != nil || opened.ClientName != client.Name || opened.ClientHost != client.Host {
		t.Fatalf("metadata round trip failed: %v", err)
	}
	if projection.list("acc_one")[0].ClientName != "" {
		t.Fatal("service read mutated ciphertext projection")
	}
	for name, mutate := range map[string]func(*Grant){
		"event":               func(g *Grant) { g.AuthorizationEventID = "evt_other" },
		"account":             func(g *Grant) { g.metadata.AccountId = "acc_other" },
		"grant":               func(g *Grant) { g.metadata.GrantId = "grant_other" },
		"client":              func(g *Grant) { g.metadata.ClientIdDigest[0] ^= 1 },
		"scope":               func(g *Grant) { g.metadata.Scopes = []string{"profile"} },
		"prior authorization": func(g *Grant) { g.metadata.PriorAuthorizationEventId = "evt_prior" },
		"disclosure":          func(g *Grant) { g.metadata.ConsentVersion++ },
		"user key":            func(g *Grant) { g.metadata.UserKeyRef = "uk_other" },
		"data key":            func(g *Grant) { g.metadata.CredentialKeyRef = "dk_other" },
		"nonce":               func(g *Grant) { g.metadata.MetadataNonce[0] ^= 1 },
		"ciphertext":          func(g *Grant) { g.metadata.MetadataCiphertext[0] ^= 1 },
	} {
		t.Run(name, func(t *testing.T) {
			changed := projected
			changed.metadata = proto.Clone(projected.metadata).(*corev1.OIDCGrantAuthorizedEvent)
			mutate(&changed)
			if _, err := service.openMetadata(t.Context(), changed); err == nil {
				t.Fatal("accepted substituted metadata")
			}
		})
	}
	wrongKey := proto.Clone(event).(*corev1.Event)
	wrongKey.Id = "evt_wrongkey"
	wrongKey.GetOidcGrantAuthorized().PriorAuthorizationEventId = event.GetId()
	wrongKey.GetOidcGrantAuthorized().UserKeyRef = "uk_other"
	if err := projection.Apply(wrongKey, 3); err == nil {
		t.Fatal("projection accepted another account key hierarchy")
	}
	if err := stores.Keys.Purge(t.Context(), userRef); err != nil {
		t.Fatal(err)
	}
	if _, err := service.openMetadata(t.Context(), projected); err == nil {
		t.Fatal("opened metadata after key loss")
	}
	if err := service.sealMetadata(t.Context(), event, accountKeys{userRef, dataRef}, client); err == nil {
		t.Fatal("sealed metadata after key loss")
	}
	// Revocation remains replayable without decrypting lost or damaged metadata.
	if err := projection.Apply(grantRevoked("evt_revoke", "grant_one", "evt_protected"), 3); err != nil {
		t.Fatal(err)
	}
}
