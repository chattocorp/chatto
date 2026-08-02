package tinybasesync

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

type memoryProvider struct {
	mu     sync.Mutex
	stores map[string]*memoryStore
}

func (provider *memoryProvider) Store(accountID string) (Store, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.stores == nil {
		provider.stores = map[string]*memoryStore{}
	}
	if provider.stores[accountID] == nil {
		provider.stores[accountID] = &memoryStore{}
	}
	return provider.stores[accountID], nil
}

func TestHubIsolatesAccountsAndFansOutChanges(t *testing.T) {
	hub := NewHub(&memoryProvider{})
	first, err := hub.Connect(t.Context(), "account-a")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := hub.Connect(t.Context(), "account-a")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	other, err := hub.Connect(t.Context(), "account-b")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()

	for _, connection := range []*Connection{first, second, other} {
		requestID := "initial"
		if err := connection.Handle(t.Context(), Envelope{RequestID: &requestID, Message: MessageGetContentHashes, Body: json.RawMessage(`""`)}); err != nil {
			t.Fatal(err)
		}
		if _, err := connection.Next(t.Context()); err != nil {
			t.Fatal(err)
		}
	}

	change := json.RawMessage(`[[{"servers":[{"one":[{"name":["One","0000000000000001"]}]}]}],[{}],1]`)
	if err := first.Handle(t.Context(), Envelope{Message: MessageContentDiff, Body: change}); err != nil {
		t.Fatal(err)
	}
	message, err := second.Next(t.Context())
	if err != nil || message.Message != MessageContentDiff {
		t.Fatalf("same-account fanout message/error = %+v/%v", message, err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	if _, err := other.Next(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("other account received a message: %v", err)
	}
}

func TestHubLimitsConnectionsPerAccount(t *testing.T) {
	hub := NewHub(&memoryProvider{})
	connections := make([]*Connection, 0, MaxConnectionsPerAccount)
	for range MaxConnectionsPerAccount {
		connection, err := hub.Connect(t.Context(), "account")
		if err != nil {
			t.Fatal(err)
		}
		connections = append(connections, connection)
	}
	if _, err := hub.Connect(t.Context(), "account"); !errors.Is(err, ErrConnectionLimit) {
		t.Fatalf("extra connection error = %v, want connection limit", err)
	}
	for _, connection := range connections {
		connection.Close()
	}
	connection, err := hub.Connect(t.Context(), "account")
	if err != nil {
		t.Fatalf("connect after all clients left: %v", err)
	}
	connection.Close()
	hub.Close()
	if _, err := hub.Connect(t.Context(), "account"); err == nil {
		t.Fatal("closed hub accepted a connection")
	}
}

func TestHubRateLimitIsSharedByAccount(t *testing.T) {
	hub := NewHub(&memoryProvider{})
	first, err := hub.Connect(t.Context(), "account")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := hub.Connect(t.Context(), "account")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	connections := []*Connection{first, second}
	for message := 0; message < accountMessageBurst; message++ {
		connection := connections[message%len(connections)]
		if err := connection.Handle(t.Context(), Envelope{Message: MessageContentHashes, Body: json.RawMessage(`[0,0]`)}); err != nil {
			t.Fatalf("burst message %d: %v", message, err)
		}
	}
	if err := first.Handle(t.Context(), Envelope{Message: MessageContentHashes, Body: json.RawMessage(`[0,0]`)}); !errors.Is(err, ErrRateLimit) {
		t.Fatalf("message above shared burst error = %v, want rate limit", err)
	}
	first.Close()
	second.Close()
	retainedSpace := first.space
	reconnected, err := hub.Connect(t.Context(), "account")
	if err != nil {
		t.Fatal(err)
	}
	defer reconnected.Close()
	if reconnected.space != retainedSpace {
		t.Fatal("reconnect replaced the retained account peer")
	}
	if err := reconnected.Handle(t.Context(), Envelope{Message: MessageContentHashes, Body: json.RawMessage(`[0,0]`)}); !errors.Is(err, ErrRateLimit) {
		t.Fatalf("reconnect reset shared rate limit: %v", err)
	}
	reconnected.space.rate.mu.Lock()
	reconnected.space.rate.updated = reconnected.space.rate.updated.Add(-time.Second)
	reconnected.space.rate.mu.Unlock()
	if err := reconnected.Handle(t.Context(), Envelope{Message: MessageContentHashes, Body: json.RawMessage(`[0,0]`)}); err != nil {
		t.Fatalf("message after refill: %v", err)
	}
}

func TestHubLimitsSynchronizationBoundariesPerAccount(t *testing.T) {
	hub := NewHub(&memoryProvider{})
	connection, err := hub.Connect(t.Context(), "account")
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	for sync := 0; sync < accountSyncBurst; sync++ {
		if err := connection.Handle(t.Context(), Envelope{Message: MessageGetContentHashes, Body: json.RawMessage(`""`)}); err != nil {
			t.Fatalf("sync boundary %d: %v", sync, err)
		}
	}
	if err := connection.Handle(t.Context(), Envelope{Message: MessageGetContentHashes, Body: json.RawMessage(`""`)}); !errors.Is(err, ErrRateLimit) {
		t.Fatalf("excess sync boundary error = %v, want rate limit", err)
	}
	connection.space.rate.mu.Lock()
	connection.space.rate.syncUpdated = connection.space.rate.syncUpdated.Add(-time.Second)
	connection.space.rate.mu.Unlock()
	if err := connection.Handle(t.Context(), Envelope{Message: MessageGetContentHashes, Body: json.RawMessage(`""`)}); err != nil {
		t.Fatalf("sync boundary after refill: %v", err)
	}
}
