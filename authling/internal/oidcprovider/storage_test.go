package oidcprovider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/natsruntime"
	appstorage "hmans.de/authling/internal/storage"
)

func TestAccountDataGrantRejectsExpiredAuthority(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	connection, err := natsruntime.Open(ctx, config.NATSConfig{
		Embedded: config.EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	js, err := jetstream.New(connection.NATS)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := appstorage.OpenStores(ctx, js, 1)
	if err != nil {
		t.Fatal(err)
	}
	fixedNow := time.Date(2026, time.August, 2, 12, 0, 0, 0, time.UTC)
	storage := NewStorage(stores.RuntimeState, js, make([]byte, 32), nil, nil)
	storage.now = func() time.Time { return fixedNow }
	state := tokenState{
		ClientID: "client", Subject: "account", Scopes: []string{"openid", ScopeAccountData},
		Origin: "https://client.example", Expires: fixedNow.Add(time.Minute),
	}
	if err := storage.create(ctx, storage.tokenKey("token"), state, time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.AccountDataGrant(ctx, "token", "account", "https://client.example"); err != nil {
		t.Fatalf("valid authority: %v", err)
	}
	storage.now = func() time.Time { return fixedNow.Add(time.Minute) }
	if _, err := storage.AccountDataGrant(ctx, "token", "account", "https://client.example"); !errors.Is(err, errOIDCStateNotFound) {
		t.Fatalf("expired authority error = %v, want not found", err)
	}
}
