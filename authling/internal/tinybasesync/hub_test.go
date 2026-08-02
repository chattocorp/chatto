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
}
