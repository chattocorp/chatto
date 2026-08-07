package accountdata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/keyvault"
	"hmans.de/authling/internal/natsruntime"
	"hmans.de/authling/internal/storage"
	"hmans.de/authling/internal/tinybasesync"
)

type accountKeys map[string]string

func (keys accountKeys) UserKeyRef(accountID string) (string, bool) {
	ref, ok := keys[accountID]
	return ref, ok
}

func TestStoreEncryptsAccountDataAndEnforcesOCC(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	connection, err := natsruntime.Open(ctx, config.NATSConfig{Embedded: config.EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	js, err := jetstream.New(connection.NATS)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := storage.OpenStores(ctx, js, 1)
	if err != nil {
		t.Fatal(err)
	}
	vault := keyvault.New(stores.Keys)
	workflowKey, err := vault.WorkflowKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(workflowKey)
	_, userRef, _, credentialKey, err := vault.ProvisionCredentialKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	clear(credentialKey)

	service := New(stores.UserData, vault, accountKeys{"account-a": userRef}, workflowKey)
	first, err := service.Store(ctx, "account-a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Store(ctx, "account-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, revision, err := first.Load(ctx); err != nil || revision != 0 {
		t.Fatalf("initial load revision/error = %d/%v", revision, err)
	}
	secret := []byte(`{"registered-server":"https://private.example"}`)
	revision, err := first.Save(ctx, secret, 0)
	if err != nil || revision == 0 {
		t.Fatalf("first save revision/error = %d/%v", revision, err)
	}
	cached, cachedRevision, err := first.Load(ctx)
	if err != nil || cachedRevision != revision || !bytes.Equal(cached, secret) {
		t.Fatalf("cached load state/revision/error = %s/%d/%v", cached, cachedRevision, err)
	}
	cached[0] ^= 0xff
	cachedAgain, _, err := first.Load(ctx)
	if err != nil || !bytes.Equal(cachedAgain, secret) {
		t.Fatalf("cached state was exposed to mutation: %s/%v", cachedAgain, err)
	}
	internal := first.(*store)
	if wrongPurposeKey, err := vault.ResolveDataKey(ctx, internal.dataKeyRef, userRef); err == nil {
		clear(wrongPurposeKey)
		t.Fatal("account-data key resolved as a credential key")
	}
	entry, err := stores.UserData.Get(ctx, internal.stateKey)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(entry.Value(), secret) || bytes.Contains(entry.Value(), []byte("private.example")) {
		t.Fatalf("durable account data contains plaintext: %s", entry.Value())
	}
	loaded, loadedRevision, err := second.Load(ctx)
	if err != nil || loadedRevision != revision || !bytes.Equal(loaded, secret) {
		t.Fatalf("loaded state/revision/error = %s/%d/%v", loaded, loadedRevision, err)
	}
	latestRevision, err := first.Save(ctx, []byte(`{"winner":true}`), revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.Save(ctx, []byte(`{"stale":true}`), revision); !errors.Is(err, tinybasesync.ErrConflict) {
		t.Fatalf("stale save error = %v, want conflict", err)
	}

	_, secondUserRef, _, secondCredentialKey, err := vault.ProvisionCredentialKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	clear(secondCredentialKey)
	secondService := New(stores.UserData, vault, accountKeys{"account-b": secondUserRef}, workflowKey)
	secondAccountStoreValue, err := secondService.Store(ctx, "account-b")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := secondAccountStoreValue.Save(ctx, []byte(`{"second":true}`), 0); err != nil {
		t.Fatal(err)
	}
	secondInternal := secondAccountStoreValue.(*store)
	secondEntry, err := stores.UserData.Get(ctx, secondInternal.stateKey)
	if err != nil {
		t.Fatal(err)
	}
	var substituted sealedState
	if err := json.Unmarshal(entry.Value(), &substituted); err != nil {
		t.Fatal(err)
	}
	substituted.DataKeyRef = secondInternal.dataKeyRef
	substitutedEnvelope, _ := json.Marshal(substituted)
	if _, err := stores.UserData.Update(ctx, secondInternal.stateKey, substitutedEnvelope, secondEntry.Revision()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := secondAccountStoreValue.Load(ctx); err == nil {
		t.Fatal("ciphertext substituted between accounts was accepted")
	}

	maximum := bytes.Repeat([]byte("x"), MaxPlaintextSize)
	maximumRevision, err := first.Save(ctx, maximum, latestRevision)
	if err != nil {
		t.Fatalf("save maximum plaintext size: %v", err)
	}
	loadedMaximum, loadedMaximumRevision, err := first.Load(ctx)
	if err != nil || loadedMaximumRevision != maximumRevision || !bytes.Equal(loadedMaximum, maximum) {
		t.Fatalf("maximum-size load revision/error = %d/%v", loadedMaximumRevision, err)
	}
}

func TestStoreRejectsUnknownAccountsAndOversizeState(t *testing.T) {
	service := New(nil, nil, accountKeys{}, []byte("index"))
	if _, err := service.Store(t.Context(), "unknown"); err == nil {
		t.Fatal("unknown account received a data store")
	}

	oversize := &store{}
	if _, err := oversize.Save(t.Context(), make([]byte, MaxPlaintextSize+1), 0); err == nil {
		t.Fatal("oversize state was accepted")
	}
}

type barrierStore struct {
	tinybasesync.Store
	ready   *sync.WaitGroup
	release <-chan struct{}
}

func (store barrierStore) Save(ctx context.Context, content []byte, expected uint64) (uint64, error) {
	if expected == 0 {
		store.ready.Done()
		<-store.release
	}
	return store.Store.Save(ctx, content, expected)
}

func TestConcurrentPeersMergeThroughJetStreamOCC(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	connection, err := natsruntime.Open(ctx, config.NATSConfig{Embedded: config.EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	js, err := jetstream.New(connection.NATS)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := storage.OpenStores(ctx, js, 1)
	if err != nil {
		t.Fatal(err)
	}
	vault := keyvault.New(stores.Keys)
	workflowKey, err := vault.WorkflowKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(workflowKey)
	_, userRef, _, credentialKey, err := vault.ProvisionCredentialKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	clear(credentialKey)
	service := New(stores.UserData, vault, accountKeys{"account": userRef}, workflowKey)
	firstStore, _ := service.Store(ctx, "account")
	secondStore, _ := service.Store(ctx, "account")
	var ready sync.WaitGroup
	ready.Add(2)
	release := make(chan struct{})
	first, err := tinybasesync.NewPeer(ctx, barrierStore{Store: firstStore, ready: &ready, release: release})
	if err != nil {
		t.Fatal(err)
	}
	second, err := tinybasesync.NewPeer(ctx, barrierStore{Store: secondStore, ready: &ready, release: release})
	if err != nil {
		t.Fatal(err)
	}
	errorsChannel := make(chan error, 2)
	for index, peer := range []*tinybasesync.Peer{first, second} {
		index, peer := index, peer
		go func() {
			cell := "left"
			hlc := "0000000000000001"
			if index == 1 {
				cell, hlc = "right", "0000000000000002"
			}
			body := json.RawMessage(`[[{"table":[{"row":[{"` + cell + `":[true,"` + hlc + `"]}]}]}],[{}],1]`)
			_, handleErr := peer.Handle(ctx, tinybasesync.Envelope{ClientID: cell, Message: tinybasesync.MessageContentDiff, Body: body})
			errorsChannel <- handleErr
		}()
	}
	ready.Wait()
	close(release)
	for range 2 {
		if err := <-errorsChannel; err != nil {
			t.Fatal(err)
		}
	}
	content, _, err := firstStore.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(content, []byte(`"left"`)) || !bytes.Contains(content, []byte(`"right"`)) {
		t.Fatalf("concurrent state did not merge both cells: %s", content)
	}
}
