package tinybasesync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

type memoryProvider struct {
	mu     sync.Mutex
	stores map[string]*memoryStore
}

type blockingProvider struct {
	memoryProvider
	started chan struct{}
	release chan struct{}
}

func (provider *blockingProvider) Store(ctx context.Context, accountID string) (Store, error) {
	if accountID == "blocked" {
		select {
		case <-provider.started:
		default:
			close(provider.started)
		}
		select {
		case <-provider.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return provider.memoryProvider.Store(ctx, accountID)
}

func (provider *memoryProvider) Store(_ context.Context, accountID string) (Store, error) {
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

func TestHubLimitsConnectionsAcrossAccounts(t *testing.T) {
	hub := NewHub(&memoryProvider{})
	defer hub.Close()
	connections := make([]*Connection, 0, MaxConnectionsPerProcess)
	for index := range MaxConnectionsPerProcess {
		accountID := fmt.Sprintf("account-%d", index/MaxConnectionsPerAccount)
		connection, err := hub.Connect(t.Context(), accountID)
		if err != nil {
			t.Fatalf("connection %d: %v", index, err)
		}
		connections = append(connections, connection)
	}
	if _, err := hub.Connect(t.Context(), "another-account"); !errors.Is(err, ErrCapacityLimit) {
		t.Fatalf("extra process connection error = %v, want capacity limit", err)
	}
	connections[0].Close()
	connection, err := hub.Connect(t.Context(), "another-account")
	if err != nil {
		t.Fatalf("connect after capacity returned: %v", err)
	}
	connection.Close()
}

func TestHubBoundsAndEvictsRetainedAccountSpaces(t *testing.T) {
	hub := NewHub(&memoryProvider{})
	defer hub.Close()
	for index := range MaxAccountSpacesPerProcess {
		connection, err := hub.Connect(t.Context(), fmt.Sprintf("account-%d", index))
		if err != nil {
			t.Fatalf("space %d: %v", index, err)
		}
		connection.Close()
	}
	connection, err := hub.Connect(t.Context(), "replacement-account")
	if err != nil {
		t.Fatalf("connect with idle spaces at capacity: %v", err)
	}
	defer connection.Close()
	hub.mu.Lock()
	spaceCount := len(hub.spaces)
	hub.mu.Unlock()
	if spaceCount != MaxAccountSpacesPerProcess {
		t.Fatalf("retained spaces = %d, want %d", spaceCount, MaxAccountSpacesPerProcess)
	}
}

func TestHubRejectsNewSpaceWhenAllSpacesAreActive(t *testing.T) {
	hub := NewHub(&memoryProvider{})
	defer hub.Close()
	for index := range MaxAccountSpacesPerProcess {
		if _, err := hub.Connect(t.Context(), fmt.Sprintf("account-%d", index)); err != nil {
			t.Fatalf("space %d: %v", index, err)
		}
	}
	if _, err := hub.Connect(t.Context(), "another-account"); !errors.Is(err, ErrCapacityLimit) {
		t.Fatalf("new active space error = %v, want capacity limit", err)
	}
}

func TestHubLoadsDifferentAccountsConcurrently(t *testing.T) {
	provider := &blockingProvider{started: make(chan struct{}), release: make(chan struct{})}
	hub := NewHub(provider)
	defer hub.Close()
	blockedResult := make(chan error, 1)
	go func() {
		connection, err := hub.Connect(t.Context(), "blocked")
		if connection != nil {
			connection.Close()
		}
		blockedResult <- err
	}()
	<-provider.started
	fast, err := hub.Connect(t.Context(), "fast")
	if err != nil {
		t.Fatalf("other account was blocked by cold load: %v", err)
	}
	fast.Close()
	close(provider.release)
	if err := <-blockedResult; err != nil {
		t.Fatalf("blocked account load after release: %v", err)
	}
}

func TestHubDoesNotRetainColdSpaceWhenConnectionCapacityFillsDuringLoad(t *testing.T) {
	provider := &blockingProvider{started: make(chan struct{}), release: make(chan struct{})}
	hub := NewHub(provider)
	defer hub.Close()
	connections := make([]*Connection, 0, MaxConnectionsPerProcess)
	for index := 0; index < MaxConnectionsPerProcess-1; index++ {
		accountID := fmt.Sprintf("account-%d", index/MaxConnectionsPerAccount)
		connection, err := hub.Connect(t.Context(), accountID)
		if err != nil {
			t.Fatalf("connection %d: %v", index, err)
		}
		connections = append(connections, connection)
	}
	blockedResult := make(chan error, 1)
	go func() {
		_, err := hub.Connect(t.Context(), "blocked")
		blockedResult <- err
	}()
	<-provider.started
	last, err := hub.Connect(t.Context(), "account-31")
	if err != nil {
		t.Fatalf("fill final connection slot: %v", err)
	}
	connections = append(connections, last)
	close(provider.release)
	if err := <-blockedResult; !errors.Is(err, ErrCapacityLimit) {
		t.Fatalf("cold load attachment error = %v, want capacity limit", err)
	}
	hub.mu.Lock()
	retained := hub.spaces["blocked"] != nil || hub.rates["blocked"] != nil
	hub.mu.Unlock()
	if retained {
		t.Fatal("never-attached cold space remained in memory")
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
