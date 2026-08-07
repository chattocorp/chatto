package tinybasesync

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// MaxConnectionsPerAccount bounds process-local live fanout.
	MaxConnectionsPerAccount = 8
	// MaxConnectionsPerProcess bounds all process-local live fanout.
	MaxConnectionsPerProcess = 256
	// MaxAccountSpacesPerProcess bounds retained decrypted account state.
	MaxAccountSpacesPerProcess = 128
	connectionQueueSize        = 32
	accountMessagesPerSecond   = 8
	accountMessageBurst        = 32
	accountSyncsPerSecond      = 1
	accountSyncBurst           = 4
	accountRateRetention       = 5 * time.Minute
	accountSpaceLoadTimeout    = 5 * time.Second
)

// ErrConnectionLimit means one account already has the maximum live devices
// attached to this Authling process.
var ErrConnectionLimit = errors.New("account data connection limit reached")

// ErrCapacityLimit means this process cannot retain another account data
// space or accept another live connection until capacity becomes available.
var ErrCapacityLimit = errors.New("account data process capacity reached")

// ErrRateLimit means the account data space exceeded its shared message rate.
var ErrRateLimit = errors.New("account data message rate exceeded")

// StoreProvider selects durable state only after an account is authenticated.
type StoreProvider interface {
	Store(context.Context, string) (Store, error)
}

// Hub owns process-local live connections for account data spaces.
type Hub struct {
	mu          sync.Mutex
	provider    StoreProvider
	spaces      map[string]*space
	loads       map[string]*spaceLoad
	rates       map[string]*accountRate
	connections atomic.Int64
	closed      bool
}

type spaceLoad struct {
	done chan struct{}
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
	mu          sync.Mutex
	tokens      float64
	updated     time.Time
	lastUsed    time.Time
	timer       *time.Timer
	syncTokens  float64
	syncUpdated time.Time
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
	return &Hub{
		provider: provider, spaces: map[string]*space{}, loads: map[string]*spaceLoad{},
		rates: map[string]*accountRate{},
	}
}

// Connect attaches one device to the authenticated account's data space.
func (hub *Hub) Connect(ctx context.Context, accountID string) (*Connection, error) {
	if hub == nil || hub.provider == nil || accountID == "" {
		return nil, errors.New("account sync unavailable")
	}
	for {
		hub.mu.Lock()
		if hub.closed {
			hub.mu.Unlock()
			return nil, errors.New("account sync hub is closed")
		}
		if current := hub.spaces[accountID]; current != nil {
			connection, err := hub.attachLocked(current)
			hub.mu.Unlock()
			return connection, err
		}
		if loading := hub.loads[accountID]; loading != nil {
			hub.mu.Unlock()
			select {
			case <-loading.done:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if !hub.reserveSpaceLocked() {
			hub.mu.Unlock()
			return nil, ErrCapacityLimit
		}
		loading := &spaceLoad{done: make(chan struct{})}
		hub.loads[accountID] = loading
		hub.mu.Unlock()

		loadContext, cancel := context.WithTimeout(ctx, accountSpaceLoadTimeout)
		store, err := hub.provider.Store(loadContext, accountID)
		var peer *Peer
		if err == nil {
			peer, err = NewPeer(loadContext, store)
		}
		cancel()

		hub.mu.Lock()
		delete(hub.loads, accountID)
		close(loading.done)
		if err != nil {
			hub.mu.Unlock()
			return nil, err
		}
		if hub.closed {
			hub.mu.Unlock()
			return nil, errors.New("account sync hub is closed")
		}
		now := time.Now()
		rate := hub.rates[accountID]
		if rate == nil {
			rate = &accountRate{
				tokens: accountMessageBurst, updated: now, lastUsed: now,
				syncTokens: accountSyncBurst, syncUpdated: now,
			}
			hub.rates[accountID] = rate
		}
		current := &space{
			accountID: accountID, peer: peer, connections: map[string]*Connection{}, hub: hub,
			rate: rate,
		}
		hub.spaces[accountID] = current
		connection, err := hub.attachLocked(current)
		if err != nil {
			delete(hub.spaces, accountID)
			delete(hub.rates, accountID)
		}
		hub.mu.Unlock()
		return connection, err
	}
}

func (hub *Hub) attachLocked(current *space) (*Connection, error) {
	current.mu.Lock()
	defer current.mu.Unlock()
	if len(current.connections) >= MaxConnectionsPerAccount {
		return nil, ErrConnectionLimit
	}
	if hub.connections.Load() >= MaxConnectionsPerProcess {
		return nil, ErrCapacityLimit
	}
	if current.rate.timer != nil {
		current.rate.timer.Stop()
		current.rate.timer = nil
	}
	id, err := connectionID()
	if err != nil {
		return nil, err
	}
	connection := &Connection{id: id, space: current, messages: make(chan Outbound, connectionQueueSize), done: make(chan struct{})}
	current.connections[id] = connection
	hub.connections.Add(1)
	return connection, nil
}

func (hub *Hub) reserveSpaceLocked() bool {
	if len(hub.spaces)+len(hub.loads) < MaxAccountSpacesPerProcess {
		return true
	}
	var candidate *space
	var candidateUsed time.Time
	for _, current := range hub.spaces {
		current.mu.Lock()
		empty := len(current.connections) == 0
		current.mu.Unlock()
		if !empty {
			continue
		}
		current.rate.mu.Lock()
		lastUsed := current.rate.lastUsed
		current.rate.mu.Unlock()
		if candidate == nil || lastUsed.Before(candidateUsed) {
			candidate, candidateUsed = current, lastUsed
		}
	}
	if candidate == nil {
		return false
	}
	if candidate.rate.timer != nil {
		candidate.rate.timer.Stop()
	}
	delete(hub.spaces, candidate.accountID)
	delete(hub.rates, candidate.accountID)
	return true
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
	hub.loads = map[string]*spaceLoad{}
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
	if !connection.space.rate.allow(time.Now(), message.Message) {
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

func (rate *accountRate) allow(now time.Time, message int) bool {
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
	if message == MessageGetContentHashes {
		syncElapsed := now.Sub(rate.syncUpdated).Seconds()
		if syncElapsed > 0 {
			rate.syncTokens = min(accountSyncBurst, rate.syncTokens+syncElapsed*accountSyncsPerSecond)
			rate.syncUpdated = now
		}
		if rate.syncTokens < 1 {
			return false
		}
		rate.syncTokens--
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
	current.hub.connections.Add(-1)
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
