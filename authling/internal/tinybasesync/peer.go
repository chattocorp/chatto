// Package tinybasesync contains an experimental TinyBase synchronization peer.
//
// Authling uses it as the durable, always-online peer for browser TinyBase
// MergeableStore clients.
package tinybasesync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

// Message numbers are part of TinyBase's experimental synchronizer protocol.
const (
	// StateFormatVersion identifies Authling's durable peer-state schema.
	StateFormatVersion = 1
	// TinyBaseVersion is the exact protocol version implemented by this peer.
	TinyBaseVersion = "9.3.0"
	// UndefinedString is TinyBase's reserved JSON transport value for undefined.
	UndefinedString = "\uFFFC"

	MessageResponse         = 0
	MessageGetContentHashes = 1
	MessageContentHashes    = 2
	MessageContentDiff      = 3
	MessageGetTableDiff     = 4
	MessageGetRowDiff       = 5
	MessageGetCellDiff      = 6
	MessageGetValueDiff     = 7
)

var hlcPattern = regexp.MustCompile(`^[-0-9A-Z_a-z]{16}$`)

const durableRefreshInterval = time.Second

// ErrConflict means another Authling replica changed the durable data space.
var ErrConflict = errors.New("TinyBase state conflict")

// UndefinedJSON is TinyBase's reserved JSON transport representation of
// JavaScript's undefined deletion tombstone.
var UndefinedJSON = json.RawMessage(`"\ufffc"`)

// Envelope is one message from a TinyBase client to the peer.
type Envelope struct {
	ClientID  string          `json:"clientId"`
	RequestID *string         `json:"requestId"`
	Message   int             `json:"message"`
	Body      json.RawMessage `json:"body"`
}

// Outbound is one message from the peer to a TinyBase client.
type Outbound struct {
	ClientID  string          `json:"clientId"`
	RequestID *string         `json:"requestId"`
	Message   int             `json:"message"`
	Body      json.RawMessage `json:"body"`
}

// Store saves the complete state of one logical TinyBase data space.
// A later Authling slice can implement this interface with JetStream.
type Store interface {
	Load(context.Context) ([]byte, uint64, error)
	Save(context.Context, []byte, uint64) (uint64, error)
}

// FileStore is the small durable store used by the protocol proof.
type FileStore struct {
	Path string
	mu   sync.Mutex
}

type fileState struct {
	Revision uint64          `json:"revision"`
	Content  json.RawMessage `json:"content"`
}

func (store *FileStore) Load(_ context.Context) ([]byte, uint64, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.load()
}

func (store *FileStore) load() ([]byte, uint64, error) {
	content, err := os.ReadFile(store.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	var stored fileState
	if err := json.Unmarshal(content, &stored); err != nil || stored.Revision == 0 || len(stored.Content) == 0 {
		return nil, 0, errors.New("decode TinyBase file state")
	}
	return stored.Content, stored.Revision, nil
}

func (store *FileStore) Save(_ context.Context, content []byte, expectedRevision uint64) (uint64, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	_, currentRevision, err := store.load()
	if err != nil {
		return 0, err
	}
	if currentRevision != expectedRevision {
		return 0, ErrConflict
	}
	newRevision := currentRevision + 1
	encoded, err := json.Marshal(fileState{Revision: newRevision, Content: content})
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(store.Path), 0o700); err != nil {
		return 0, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(store.Path), ".tinybase-state-*")
	if err != nil {
		return 0, err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return 0, err
	}
	if _, err := temporary.Write(encoded); err != nil {
		temporary.Close()
		return 0, err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return 0, err
	}
	if err := temporary.Close(); err != nil {
		return 0, err
	}
	if err := os.Rename(temporaryName, store.Path); err != nil {
		return 0, err
	}
	return newRevision, nil
}

type leaf struct {
	Value json.RawMessage `json:"value"`
	HLC   string          `json:"hlc"`
}

type row struct {
	Cells map[string]leaf `json:"cells"`
	HLC   string          `json:"hlc"`
}

type table struct {
	Rows map[string]row `json:"rows"`
	HLC  string         `json:"hlc"`
}

type state struct {
	FormatVersion   int              `json:"formatVersion"`
	TinyBaseVersion string           `json:"tinyBaseVersion"`
	Tables          map[string]table `json:"tables"`
	TablesHLC       string           `json:"tablesHlc"`
	Values          map[string]leaf  `json:"values"`
	ValuesHLC       string           `json:"valuesHlc"`
}

func newState() state {
	return state{
		FormatVersion: StateFormatVersion, TinyBaseVersion: TinyBaseVersion,
		Tables: map[string]table{}, Values: map[string]leaf{},
	}
}

// Peer implements the responder side of TinyBase 9.3's custom synchronizer
// protocol. It deliberately returns complete stamped tables and values instead
// of using TinyBase's optional row and cell hash-tree optimisation.
type Peer struct {
	mu          sync.Mutex
	store       Store
	state       state
	revision    uint64
	clients     map[string]struct{}
	pending     map[string]pendingRequest
	nextID      uint64
	lastRefresh time.Time
}

type pendingRequest struct {
	clientID string
	kind     int
}

// NewPeer loads one peer from durable storage.
func NewPeer(ctx context.Context, store Store) (*Peer, error) {
	peer := &Peer{
		store:   store,
		state:   newState(),
		clients: map[string]struct{}{},
		pending: map[string]pendingRequest{},
	}
	content, revision, err := store.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("load TinyBase peer: %w", err)
	}
	if len(content) != 0 {
		if err := decodeState(content, &peer.state); err != nil {
			return nil, fmt.Errorf("decode TinyBase peer: %w", err)
		}
	}
	peer.revision = revision
	peer.lastRefresh = time.Now()
	return peer, nil
}

// Handle applies one message and returns all messages that must be delivered.
func (peer *Peer) Handle(ctx context.Context, message Envelope) ([]Outbound, error) {
	peer.mu.Lock()
	defer peer.mu.Unlock()
	if message.ClientID == "" {
		return nil, errors.New("client ID is required")
	}
	if message.Message == MessageGetContentHashes || time.Since(peer.lastRefresh) >= durableRefreshInterval {
		if err := peer.refresh(ctx); err != nil {
			return nil, err
		}
	}
	peer.clients[message.ClientID] = struct{}{}

	switch message.Message {
	case MessageResponse:
		return peer.handleResponse(ctx, message)
	case MessageGetContentHashes:
		var body string
		if err := json.Unmarshal(message.Body, &body); err != nil || body != "" {
			return nil, errors.New("invalid TinyBase content-hash request")
		}
		return []Outbound{peer.response(message, peer.contentHashes())}, nil
	case MessageContentHashes:
		return peer.pullDifferentContent(message)
	case MessageContentDiff:
		return peer.applyAndBroadcast(ctx, message.ClientID, message.RequestID, message.Body, true)
	case MessageGetTableDiff:
		if !validHashTree(message.Body, 0) {
			return nil, errors.New("invalid TinyBase table hashes")
		}
		body, err := peer.tableDiff()
		if err != nil {
			return nil, err
		}
		return []Outbound{peer.response(message, body)}, nil
	case MessageGetValueDiff:
		if !validHashTree(message.Body, 0) {
			return nil, errors.New("invalid TinyBase value hashes")
		}
		body, err := peer.valueDiff()
		if err != nil {
			return nil, err
		}
		return []Outbound{peer.response(message, body)}, nil
	case MessageGetRowDiff:
		if !validHashTree(message.Body, 1) {
			return nil, errors.New("invalid TinyBase row hashes")
		}
		return nil, errors.New("the complete-state peer does not use row diffs")
	case MessageGetCellDiff:
		if !validHashTree(message.Body, 2) {
			return nil, errors.New("invalid TinyBase cell hashes")
		}
		return nil, errors.New("the complete-state peer does not use row or cell diffs")
	default:
		return nil, fmt.Errorf("unsupported TinyBase message %d", message.Message)
	}
}

// RemoveClient removes a disconnected transport peer from local fanout.
func (peer *Peer) RemoveClient(clientID string) {
	peer.mu.Lock()
	defer peer.mu.Unlock()
	delete(peer.clients, clientID)
	for requestID, pending := range peer.pending {
		if pending.clientID == clientID {
			delete(peer.pending, requestID)
		}
	}
}

func (peer *Peer) pullDifferentContent(message Envelope) ([]Outbound, error) {
	var hashes []uint32
	if err := json.Unmarshal(message.Body, &hashes); err != nil || len(hashes) != 2 {
		return nil, errors.New("invalid TinyBase content hashes")
	}
	local := []uint32{tablesHash(peer.state), valuesHash(peer.state.Values, peer.state.ValuesHLC)}
	out := make([]Outbound, 0, 2)
	if len(peer.pending) > 62 {
		return nil, errors.New("too many pending TinyBase requests")
	}
	for kind := 0; kind < 2; kind++ {
		if hashes[kind] == local[kind] {
			continue
		}
		peer.nextID++
		requestID := fmt.Sprintf("authling.%d.%d", peer.nextID, kind)
		peer.pending[requestID] = pendingRequest{clientID: message.ClientID, kind: kind}
		requestMessage := MessageGetTableDiff
		if kind == 1 {
			requestMessage = MessageGetValueDiff
		}
		out = append(out, peer.send(message.ClientID, &requestID, requestMessage, json.RawMessage(`{}`)))
	}
	return out, nil
}

func (peer *Peer) handleResponse(ctx context.Context, message Envelope) ([]Outbound, error) {
	if message.RequestID == nil {
		return nil, errors.New("TinyBase response has no request ID")
	}
	pending, exists := peer.pending[*message.RequestID]
	if !exists || pending.clientID != message.ClientID {
		return nil, errors.New("TinyBase response does not match a request")
	}
	delete(peer.pending, *message.RequestID)

	var content json.RawMessage
	if pending.kind == 0 {
		var response []json.RawMessage
		if err := json.Unmarshal(message.Body, &response); err != nil || len(response) != 2 {
			return nil, errors.New("invalid TinyBase table diff response")
		}
		content, _ = marshalBody([]json.RawMessage{response[0], json.RawMessage(`[{}]`), json.RawMessage(`1`)})
	} else {
		content, _ = marshalBody([]json.RawMessage{json.RawMessage(`[{}]`), message.Body, json.RawMessage(`1`)})
	}
	return peer.applyAndBroadcast(ctx, message.ClientID, message.RequestID, content, true)
}

func (peer *Peer) applyAndBroadcast(ctx context.Context, clientID string, requestID *string, body json.RawMessage, notifySource bool) ([]Outbound, error) {
	for range 5 {
		candidate := cloneState(peer.state)
		changed, err := applyContent(&candidate, body)
		if err != nil {
			return nil, err
		}
		if !changed {
			return peer.hashNotifications(requestID, notifySource), nil
		}
		encoded, err := json.Marshal(candidate)
		if err != nil {
			return nil, fmt.Errorf("encode TinyBase peer: %w", err)
		}
		newRevision, err := peer.store.Save(ctx, encoded, peer.revision)
		if errors.Is(err, ErrConflict) {
			if err := peer.refresh(ctx); err != nil {
				return nil, err
			}
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("save TinyBase peer: %w", err)
		}
		peer.state = candidate
		peer.revision = newRevision
		out := peer.broadcast(clientID, requestID, body)
		return append(out, peer.hashNotifications(requestID, notifySource)...), nil
	}
	return nil, errors.New("save TinyBase peer after repeated conflicts")
}

func (peer *Peer) hashNotifications(requestID *string, notify bool) []Outbound {
	if !notify {
		return nil
	}
	out := make([]Outbound, 0, len(peer.clients))
	for clientID := range peer.clients {
		out = append(out, peer.send(clientID, requestID, MessageContentHashes, peer.contentHashes()))
	}
	return out
}

func (peer *Peer) broadcast(clientID string, requestID *string, body json.RawMessage) []Outbound {
	out := make([]Outbound, 0, len(peer.clients)-1)
	for otherClientID := range peer.clients {
		if otherClientID != clientID {
			out = append(out, peer.send(otherClientID, requestID, MessageContentDiff, body))
		}
	}
	return out
}

func (peer *Peer) refresh(ctx context.Context) error {
	content, revision, err := peer.store.Load(ctx)
	if err != nil {
		return fmt.Errorf("refresh TinyBase peer: %w", err)
	}
	peer.lastRefresh = time.Now()
	if revision == peer.revision {
		return nil
	}
	refreshed := newState()
	if len(content) != 0 {
		if err := decodeState(content, &refreshed); err != nil {
			return fmt.Errorf("refresh TinyBase peer: %w", err)
		}
	}
	peer.state = refreshed
	peer.revision = revision
	return nil
}

func (peer *Peer) contentHashes() json.RawMessage {
	return json.RawMessage(fmt.Sprintf("[%d,%d]", tablesHash(peer.state), valuesHash(peer.state.Values, peer.state.ValuesHLC)))
}

func (peer *Peer) response(in Envelope, body json.RawMessage) Outbound {
	return peer.send(in.ClientID, in.RequestID, MessageResponse, body)
}

func (peer *Peer) send(clientID string, requestID *string, message int, body json.RawMessage) Outbound {
	return Outbound{ClientID: clientID, RequestID: requestID, Message: message, Body: body}
}

func cloneState(current state) state {
	copy := newState()
	encoded, _ := json.Marshal(current)
	_ = json.Unmarshal(encoded, &copy)
	return copy
}

func (peer *Peer) tableDiff() (json.RawMessage, error) {
	tables := make(map[string]any, len(peer.state.Tables))
	for tableID, storedTable := range peer.state.Tables {
		rows := make(map[string]any, len(storedTable.Rows))
		for rowID, storedRow := range storedTable.Rows {
			cells := make(map[string]any, len(storedRow.Cells))
			for cellID, storedCell := range storedRow.Cells {
				cells[cellID] = stamp(storedCell.Value, storedCell.HLC)
			}
			rows[rowID] = stamp(cells, storedRow.HLC)
		}
		tables[tableID] = stamp(rows, storedTable.HLC)
	}
	return marshalBody([]any{stamp(tables, peer.state.TablesHLC), map[string]uint32{}})
}

func (peer *Peer) valueDiff() (json.RawMessage, error) {
	values := make(map[string]any, len(peer.state.Values))
	for valueID, storedValue := range peer.state.Values {
		values[valueID] = stamp(storedValue.Value, storedValue.HLC)
	}
	return marshalBody(stamp(values, peer.state.ValuesHLC))
}

func stamp(value any, hlc string) []any {
	if hlc == "" {
		return []any{value}
	}
	return []any{value, hlc}
}

func marshalBody(value any) (json.RawMessage, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode TinyBase response: %w", err)
	}
	return body, nil
}

func applyContent(target *state, body json.RawMessage) (bool, error) {
	var content []json.RawMessage
	if err := json.Unmarshal(body, &content); err != nil || (len(content) != 2 && len(content) != 3) {
		return false, errors.New("invalid TinyBase content diff")
	}
	if len(content) == 3 && !bytes.Equal(bytes.TrimSpace(content[2]), []byte("1")) {
		return false, errors.New("invalid TinyBase changes marker")
	}
	tablesObject, tablesHLC, err := parseContainer(content[0])
	if err != nil {
		return false, fmt.Errorf("invalid TinyBase tables stamp: %w", err)
	}
	valuesObject, valuesHLC, err := parseContainer(content[1])
	if err != nil {
		return false, fmt.Errorf("invalid TinyBase values stamp: %w", err)
	}

	changed := false
	for tableID, tableStamp := range tablesObject {
		rowsObject, tableHLC, err := parseContainer(tableStamp)
		if err != nil {
			return false, fmt.Errorf("invalid table %q: %w", tableID, err)
		}
		storedTable := target.Tables[tableID]
		if storedTable.Rows == nil {
			storedTable.Rows = map[string]row{}
		}
		for rowID, rowStamp := range rowsObject {
			cellsObject, rowHLC, err := parseContainer(rowStamp)
			if err != nil {
				return false, fmt.Errorf("invalid row %q: %w", rowID, err)
			}
			storedRow := storedTable.Rows[rowID]
			if storedRow.Cells == nil {
				storedRow.Cells = map[string]leaf{}
			}
			for cellID, cellStamp := range cellsObject {
				incoming, err := parseLeaf(cellStamp)
				if err != nil {
					return false, fmt.Errorf("invalid cell %q: %w", cellID, err)
				}
				current, exists := storedRow.Cells[cellID]
				if !exists || incoming.HLC > current.HLC {
					storedRow.Cells[cellID] = incoming
					changed = true
				}
			}
			if next := latest(storedRow.HLC, rowHLC); next != storedRow.HLC {
				storedRow.HLC = next
				changed = true
			}
			storedTable.Rows[rowID] = storedRow
		}
		if next := latest(storedTable.HLC, tableHLC); next != storedTable.HLC {
			storedTable.HLC = next
			changed = true
		}
		target.Tables[tableID] = storedTable
	}
	if next := latest(target.TablesHLC, tablesHLC); next != target.TablesHLC {
		target.TablesHLC = next
		changed = true
	}

	for valueID, valueStamp := range valuesObject {
		incoming, err := parseLeaf(valueStamp)
		if err != nil {
			return false, fmt.Errorf("invalid value %q: %w", valueID, err)
		}
		current, exists := target.Values[valueID]
		if !exists || incoming.HLC > current.HLC {
			target.Values[valueID] = incoming
			changed = true
		}
	}
	if next := latest(target.ValuesHLC, valuesHLC); next != target.ValuesHLC {
		target.ValuesHLC = next
		changed = true
	}
	return changed, nil
}

func parseContainer(raw json.RawMessage) (map[string]json.RawMessage, string, error) {
	items, hlc, err := parseStamp(raw)
	if err != nil {
		return nil, "", err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(items, &object); err != nil || object == nil {
		return nil, "", errors.New("stamp value is not an object")
	}
	return object, hlc, nil
}

func parseLeaf(raw json.RawMessage) (leaf, error) {
	value, hlc, err := parseStamp(raw)
	if err != nil {
		return leaf{}, err
	}
	if len(value) == 0 {
		return leaf{}, errors.New("missing leaf value")
	}
	if err := validateLeafValue(value); err != nil {
		return leaf{}, err
	}
	return leaf{Value: append(json.RawMessage(nil), value...), HLC: hlc}, nil
}

func decodeState(encoded []byte, target *state) error {
	if err := json.Unmarshal(encoded, target); err != nil {
		return err
	}
	return validateState(*target)
}

func validateState(content state) error {
	if content.FormatVersion != StateFormatVersion || content.TinyBaseVersion != TinyBaseVersion {
		return errors.New("unsupported durable state version")
	}
	if content.Tables == nil || content.Values == nil || !validHLC(content.TablesHLC) || !validHLC(content.ValuesHLC) {
		return errors.New("invalid durable state root")
	}
	for _, storedTable := range content.Tables {
		if storedTable.Rows == nil || !validHLC(storedTable.HLC) {
			return errors.New("invalid durable table")
		}
		for _, storedRow := range storedTable.Rows {
			if storedRow.Cells == nil || !validHLC(storedRow.HLC) {
				return errors.New("invalid durable row")
			}
			for _, storedCell := range storedRow.Cells {
				if !validHLC(storedCell.HLC) || validateLeafValue(storedCell.Value) != nil {
					return errors.New("invalid durable cell")
				}
			}
		}
	}
	for _, storedValue := range content.Values {
		if !validHLC(storedValue.HLC) || validateLeafValue(storedValue.Value) != nil {
			return errors.New("invalid durable value")
		}
	}
	return nil
}

func validateLeafValue(encoded json.RawMessage) error {
	if !json.Valid(encoded) {
		return errors.New("leaf value is not JSON")
	}
	var text string
	if json.Unmarshal(encoded, &text) == nil && len(text) > 0 {
		if text == UndefinedString {
			return nil
		}
		// TinyBase reserves U+FFFD-prefixed strings for encoded object and
		// array values inside its mergeable protocol.
		if []rune(text)[0] == '\uFFFD' {
			var decoded any
			if len(text) == len("\uFFFD") || json.Unmarshal([]byte(text[len("\uFFFD"):]), &decoded) != nil {
				return errors.New("leaf value has invalid TinyBase-encoded JSON")
			}
			switch decoded.(type) {
			case map[string]any, []any:
				return nil
			default:
				return errors.New("leaf value has invalid TinyBase-encoded JSON type")
			}
		}
	}
	return nil
}

func parseStamp(raw json.RawMessage) (json.RawMessage, string, error) {
	var stamp []json.RawMessage
	if err := json.Unmarshal(raw, &stamp); err != nil || len(stamp) < 1 || len(stamp) > 2 {
		return nil, "", errors.New("stamp must contain a value and optional HLC")
	}
	hlc := ""
	if len(stamp) == 2 {
		if err := json.Unmarshal(stamp[1], &hlc); err != nil || !validHLC(hlc) {
			return nil, "", errors.New("stamp has an invalid HLC")
		}
	}
	return stamp[0], hlc, nil
}

func validHLC(hlc string) bool {
	if hlc == "" {
		return true
	}
	if !hlcPattern.MatchString(hlc) {
		return false
	}
	var logicalTime int64
	const alphabet = "-0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz"
	for index := 0; index < 7; index++ {
		value := int64(bytes.IndexByte([]byte(alphabet), hlc[index]))
		if value < 0 {
			return false
		}
		logicalTime = logicalTime*64 + value
	}
	return logicalTime <= time.Now().Add(5*time.Minute).UnixMilli()
}

func validHashTree(raw json.RawMessage, depth int) bool {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return false
	}
	return validateHashTreeValue(value, depth)
}

func validateHashTreeValue(value any, depth int) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for _, child := range object {
		if depth > 0 {
			if !validateHashTreeValue(child, depth-1) {
				return false
			}
			continue
		}
		number, ok := child.(json.Number)
		if !ok {
			return false
		}
		integer, err := number.Int64()
		if err != nil || integer < 0 || integer > int64(^uint32(0)) {
			return false
		}
	}
	return true
}

func latest(first, second string) string {
	if second > first {
		return second
	}
	return first
}

func tablesHash(content state) uint32 {
	var hash uint32
	for tableID, storedTable := range content.Tables {
		var tableHash uint32
		for rowID, storedRow := range storedTable.Rows {
			tableHash ^= hashEntry(rowID, valuesHash(storedRow.Cells, storedRow.HLC))
		}
		if storedTable.HLC != "" {
			tableHash ^= fnv1a(storedTable.HLC)
		}
		hash ^= hashEntry(tableID, tableHash)
	}
	if content.TablesHLC != "" {
		hash ^= fnv1a(content.TablesHLC)
	}
	return hash
}

func valuesHash[Value ~map[string]leaf](values Value, hlc string) uint32 {
	var hash uint32
	for valueID, value := range values {
		valueHash := leafHash(value)
		// TinyBase 9.3 retains this legacy value and cell contribution.
		hash ^= hashEntry(valueID, valueHash) ^ hashEntry(valueID, 0)
	}
	if hlc != "" {
		hash ^= fnv1a(hlc)
	}
	return hash
}

func leafHash(value leaf) uint32 {
	encoded := value.Value
	if isUndefined(encoded) {
		encoded = json.RawMessage("null")
	} else if decoded, ok := decodedTinyBaseJSON(encoded); ok {
		encoded = decoded
	}
	return fnv1a(string(encoded) + ":" + value.HLC)
}

func decodedTinyBaseJSON(encoded json.RawMessage) (json.RawMessage, bool) {
	var value string
	if json.Unmarshal(encoded, &value) != nil || len(value) == 0 || []rune(value)[0] != '\uFFFD' {
		return nil, false
	}
	return json.RawMessage(value[len("\uFFFD"):]), true
}

func isUndefined(encoded json.RawMessage) bool {
	var value string
	return json.Unmarshal(encoded, &value) == nil && value == UndefinedString
}

func hashEntry(id string, hash uint32) uint32 {
	return fnv1a(fmt.Sprintf("%s:%d", id, hash))
}

func fnv1a(value string) uint32 {
	hash := uint32(0x811c9dc5)
	for _, character := range []byte(value) {
		hash ^= uint32(character)
		hash += (hash << 1) + (hash << 4) + (hash << 7) + (hash << 8) + (hash << 24)
	}
	return hash
}
