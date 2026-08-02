package tinybasesync

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	// MaxConnectionsPerAccount bounds process-local live fanout.
	MaxConnectionsPerAccount = 8
	connectionQueueSize      = 32
	accountMessagesPerSecond = 8
	accountMessageBurst      = 32
	accountRateRetention     = 5 * time.Minute
)

// ErrConnectionLimit means one account already has the maximum live devices
// attached to this Authling process.
var ErrConnectionLimit = errors.New("account data connection limit reached")

// ErrRateLimit means the account data space exceeded its shared message rate.
var ErrRateLimit = errors.New("account data message rate exceeded")

// StoreProvider selects durable state only after an account is authenticated.
type StoreProvider interface {
	Store(accountID string) (Store, error)
}

// Hub owns process-local live connections for account data spaces.
type Hub struct {
	mu       sync.Mutex
	provider StoreProvider
	spaces   map[string]*space
	rates    map[string]*accountRate
	closed   bool
}

type space struct {
	accountID   string
	peer        *Peer
	connections map[string]*Connection
	hub         *Hub
	mu          sync.Mutex
	rate        *accountRate
}

type accountRate struct {
	mu       sync.Mutex
	tokens   float64
	updated  time.Time
	lastUsed time.Time
	timer    *time.Timer
}

// Connection is one authenticated device attached to an account data space.
type Connection struct {
	id        string
	space     *space
	messages  chan Outbound
	done      chan struct{}
	closeOnce sync.Once
}

// NewHub constructs the process-local account sync hub.
func NewHub(provider StoreProvider) *Hub {
	return &Hub{provider: provider, spaces: map[string]*space{}, rates: map[string]*accountRate{}}
}

// Connect attaches one device to the authenticated account's data space.
func (hub *Hub) Connect(ctx context.Context, accountID string) (*Connection, error) {
	if hub == nil || hub.provider == nil || accountID == "" {
		return nil, errors.New("account sync unavailable")
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if hub.closed {
		return nil, errors.New("account sync hub is closed")
	}
	current := hub.spaces[accountID]
	if current != nil && current.rate.timer != nil {
		current.rate.timer.Stop()
		current.rate.timer = nil
	}
	if current == nil {
		now := time.Now()
		hub.pruneRates(now)
		rate := hub.rates[accountID]
		if rate == nil {
			rate = &accountRate{tokens: accountMessageBurst, updated: now, lastUsed: now}
			hub.rates[accountID] = rate
		}
		if rate.timer != nil {
			rate.timer.Stop()
			rate.timer = nil
		}
		store, err := hub.provider.Store(accountID)
		if err != nil {
			return nil, err
		}
		peer, err := NewPeer(ctx, store)
		if err != nil {
			return nil, err
		}
		current = &space{
			accountID: accountID, peer: peer, connections: map[string]*Connection{}, hub: hub,
			rate: rate,
		}
		hub.spaces[accountID] = current
	}
	current.mu.Lock()
	defer current.mu.Unlock()
	if len(current.connections) >= MaxConnectionsPerAccount {
		return nil, ErrConnectionLimit
	}
	id, err := connectionID()
	if err != nil {
		return nil, err
	}
	connection := &Connection{id: id, space: current, messages: make(chan Outbound, connectionQueueSize), done: make(chan struct{})}
	current.connections[id] = connection
	return connection, nil
}

// Close disconnects every live device and rejects new connections.
func (hub *Hub) Close() {
	if hub == nil {
		return
	}
	hub.mu.Lock()
	if hub.closed {
		hub.mu.Unlock()
		return
	}
	hub.closed = true
	spaces := make([]*space, 0, len(hub.spaces))
	for _, current := range hub.spaces {
		spaces = append(spaces, current)
	}
	hub.spaces = map[string]*space{}
	for _, rate := range hub.rates {
		if rate.timer != nil {
			rate.timer.Stop()
		}
	}
	hub.rates = map[string]*accountRate{}
	hub.mu.Unlock()
	for _, current := range spaces {
		current.mu.Lock()
		for id := range current.connections {
			current.removeLocked(id)
		}
		current.mu.Unlock()
	}
}

// Handle applies one TinyBase message and routes protocol output to the local
// account connections named by the peer.
func (connection *Connection) Handle(ctx context.Context, message Envelope) error {
	select {
	case <-connection.done:
		return errors.New("account sync connection is closed")
	default:
	}
	if !connection.space.rate.allow(time.Now()) {
		return ErrRateLimit
	}
	message.ClientID = connection.id
	outbound, err := connection.space.peer.Handle(ctx, message)
	if err != nil {
		return err
	}
	select {
	case <-connection.done:
		connection.space.peer.RemoveClient(connection.id)
		return errors.New("account sync connection is closed")
	default:
	}
	connection.space.deliver(outbound)
	return nil
}

func (rate *accountRate) allow(now time.Time) bool {
	rate.mu.Lock()
	defer rate.mu.Unlock()
	elapsed := now.Sub(rate.updated).Seconds()
	if elapsed > 0 {
		rate.tokens = min(accountMessageBurst, rate.tokens+elapsed*accountMessagesPerSecond)
		rate.updated = now
	}
	rate.lastUsed = now
	if rate.tokens < 1 {
		return false
	}
	rate.tokens--
	return true
}

func (hub *Hub) pruneRates(now time.Time) {
	for accountID, rate := range hub.rates {
		if hub.spaces[accountID] != nil {
			continue
		}
		rate.mu.Lock()
		expired := now.Sub(rate.lastUsed) >= accountRateRetention
		rate.mu.Unlock()
		if expired {
			if rate.timer != nil {
				rate.timer.Stop()
			}
			delete(hub.rates, accountID)
		}
	}
}

// Next waits for one message that the transport must send to this device.
func (connection *Connection) Next(ctx context.Context) (Outbound, error) {
	select {
	case message := <-connection.messages:
		return message, nil
	case <-connection.done:
		return Outbound{}, errors.New("account sync connection is closed")
	case <-ctx.Done():
		return Outbound{}, ctx.Err()
	}
}

// Close detaches the device. It is safe to call more than once.
func (connection *Connection) Close() {
	connection.closeOnce.Do(func() { connection.space.remove(connection.id) })
}

func (current *space) deliver(messages []Outbound) {
	current.mu.Lock()
	var slow []string
	for _, message := range messages {
		connection := current.connections[message.ClientID]
		if connection == nil {
			continue
		}
		select {
		case connection.messages <- message:
		default:
			slow = append(slow, connection.id)
		}
	}
	for _, id := range slow {
		current.removeLocked(id)
	}
	empty := len(current.connections) == 0
	current.mu.Unlock()
	if empty {
		current.hub.removeSpace(current)
	}
}

func (current *space) remove(id string) {
	current.mu.Lock()
	current.removeLocked(id)
	empty := len(current.connections) == 0
	current.mu.Unlock()
	if empty {
		current.hub.removeSpace(current)
	}
}

func (current *space) removeLocked(id string) {
	connection := current.connections[id]
	if connection == nil {
		return
	}
	delete(current.connections, id)
	close(connection.done)
	current.peer.RemoveClient(id)
}

func (hub *Hub) removeSpace(current *space) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if hub.spaces[current.accountID] == current {
		current.mu.Lock()
		if len(current.connections) == 0 {
			hub.scheduleRateExpiry(current.accountID)
		}
		current.mu.Unlock()
	}
}

func (hub *Hub) scheduleRateExpiry(accountID string) {
	rate := hub.rates[accountID]
	if rate == nil {
		return
	}
	if rate.timer != nil {
		rate.timer.Stop()
	}
	rate.timer = time.AfterFunc(accountRateRetention, func() {
		hub.mu.Lock()
		defer hub.mu.Unlock()
		current := hub.spaces[accountID]
		if current == nil || hub.rates[accountID] != rate {
			return
		}
		current.mu.Lock()
		defer current.mu.Unlock()
		if len(current.connections) == 0 {
			delete(hub.spaces, accountID)
			delete(hub.rates, accountID)
		}
	})
}

func connectionID() (string, error) {
	value := make([]byte, 18)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate sync connection ID: %w", err)
	}
	return "sync_" + base64.RawURLEncoding.EncodeToString(value), nil
}
