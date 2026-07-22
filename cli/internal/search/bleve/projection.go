// Package bleve implements Chatto's bundled EVT-backed message search provider.
package bleve

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	blevesearch "github.com/blevesearch/bleve/v2"
	bleveindex "github.com/blevesearch/bleve_index_api"
	"github.com/charmbracelet/log"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/dekstore"
	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/kms"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	checkpointContractBaseID = "bleve-message-index-v8"
	checkpointInternalKey    = "chatto/search/checkpoint"
	dekInternalKey           = "chatto/search/deks"
	startupReplayBatchSize   = 256
	slowIndexOperation       = 10 * time.Second
)

type checkpointRecord struct {
	ProjectionKey  string `json:"projection_key"`
	ContractID     string `json:"contract_id"`
	StreamName     string `json:"stream_name"`
	StreamIdentity string `json:"stream_identity"`
	CutoffSequence uint64 `json:"cutoff_sequence"`
}

type messageDocument struct {
	MessageID      string    `json:"message_id"`
	RoomID         string    `json:"room_id"`
	AuthorID       string    `json:"author_id"`
	Body           string    `json:"body"`
	BodyEventID    string    `json:"body_event_id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	HasAttachments bool      `json:"has_attachments"`
	Visible        bool      `json:"visible"`
	BodySequence   uint64    `json:"body_sequence"`
	PostedSequence uint64    `json:"posted_sequence"`
	// ProjectionState is stored but not indexed. It lets a later EVT event
	// reconstruct this document without maintaining a second per-message copy
	// in Bleve's high-churn internal Bolt keyspace.
	ProjectionState string `json:"projection_state,omitempty"`
}

type persistedDEKs map[string]string

type projectionBatch struct {
	projection      *Projection
	index           *blevesearch.Batch
	messages        map[string]messageDocument
	deletedMessages map[string]struct{}
	deks            map[string]*corev1.UserDEKGeneratedEvent
	deksMutable     bool
	dekChanged      bool
}

// makeDEKsMutable gives a batch its own map before changing retained DEK
// metadata. Existing protobuf values are immutable, so a shallow map copy is
// sufficient and ordinary message events do not copy the server-wide DEK set.
func (b *projectionBatch) makeDEKsMutable() {
	if b.deksMutable {
		return
	}
	deks := make(map[string]*corev1.UserDEKGeneratedEvent, len(b.deks))
	for key, dek := range b.deks {
		deks[key] = dek
	}
	b.deks = deks
	b.deksMutable = true
}

// Projection materializes searchable plaintext into a disposable local Bleve
// index. Bleve batch commits bind every mutation to its EVT cutoff.
type Projection struct {
	mu         sync.RWMutex
	directory  string
	index      blevesearch.Index
	logger     *log.Logger
	keyWrapper kms.KeyWrapper
	legacyKeys kms.LegacyKeyProvider
	dekStore   dekstore.Reader
	deks       map[string]*corev1.UserDEKGeneratedEvent
	checkpoint checkpointRecord
	languages  []languageAnalyzer
	contractID string
	// commitBatch is an optional test seam; production commits through index.Batch.
	commitBatch func(*blevesearch.Batch) error
}

func NewProjection(directory string, languageCodes []string, keyWrapper kms.KeyWrapper, legacyKeys kms.LegacyKeyProvider, dekStore dekstore.Reader, logger *log.Logger) (*Projection, error) {
	directory = strings.TrimSpace(directory)
	cleanDirectory := filepath.Clean(directory)
	if directory == "" || cleanDirectory == "." || cleanDirectory == filepath.VolumeName(cleanDirectory)+string(filepath.Separator) {
		return nil, fmt.Errorf("search index requires a dedicated directory")
	}
	if logger == nil {
		logger = log.WithPrefix(runtimeUnitName)
	}
	languages, err := resolveLanguageAnalyzers(languageCodes)
	if err != nil {
		return nil, err
	}
	p := &Projection{
		directory:  cleanDirectory,
		logger:     logger,
		keyWrapper: keyWrapper,
		legacyKeys: legacyKeys,
		dekStore:   dekStore,
		deks:       make(map[string]*corev1.UserDEKGeneratedEvent),
		languages:  languages,
		contractID: languageCheckpointContractID(languages),
	}
	if err := p.open(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Projection) Subjects() []string {
	return []string{
		events.RoomEventTypeFilter(events.EventMessageBody),
		events.RoomEventTypeFilter(events.EventMessagePosted),
		events.RoomEventTypeFilter(events.EventMessageRetracted),
		events.RoomEventTypeFilter(events.EventRoomDeleted),
		events.UserEventTypeFilter(events.EventUserDEKGenerated),
		events.UserEventTypeFilter(events.EventUserKeyShredded),
	}
}

func (p *Projection) CheckpointContractID() string { return p.contractID }

// StartupBatchSize selects the number of ordered replay events committed with
// one Bleve transaction. Live events still commit individually through Apply.
func (*Projection) StartupBatchSize() int { return startupReplayBatchSize }

func (p *Projection) Apply(event *corev1.Event, seq uint64) error {
	return p.applyBatch([]events.SequencedEvent{{Event: event, Sequence: seq}})
}

// ApplyStartupBatch applies ordered startup events with one atomic Bleve
// mutation and checkpoint commit.
func (p *Projection) ApplyStartupBatch(items []events.SequencedEvent) error {
	return p.applyBatch(items)
}

func (p *Projection) applyBatch(items []events.SequencedEvent) error {
	if len(items) == 0 {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	firstSequence := items[0].Sequence
	lastSequence := items[len(items)-1].Sequence
	applyTimer := time.AfterFunc(slowIndexOperation, func() {
		p.logger.Warn("Search index operation is slow",
			"stage", "event_preparation_or_commit",
			"batch_events", len(items),
			"first_seq", firstSequence,
			"last_seq", lastSequence,
			"threshold", slowIndexOperation)
	})
	defer applyTimer.Stop()
	if p.index == nil {
		return fmt.Errorf("search index is closed")
	}
	batch := &projectionBatch{
		projection:      p,
		index:           p.index.NewBatch(),
		messages:        make(map[string]messageDocument),
		deletedMessages: make(map[string]struct{}),
		deks:            p.deks,
	}
	var previousSequence uint64
	for i, item := range items {
		if item.Sequence == 0 {
			return fmt.Errorf("search index batch contains a zero EVT sequence")
		}
		if i > 0 && item.Sequence <= previousSequence {
			return fmt.Errorf("search index batch sequences must be strictly increasing")
		}
		if err := p.applyEvent(batch, item.Event, item.Sequence); err != nil {
			return err
		}
		previousSequence = item.Sequence
	}

	if batch.dekChanged {
		data, err := encodeDEKs(batch.deks)
		if err != nil {
			return err
		}
		batch.index.SetInternal([]byte(dekInternalKey), data)
	}
	record := p.checkpoint
	record.CutoffSequence = lastSequence
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	batch.index.SetInternal([]byte(checkpointInternalKey), data)
	commitBatch := p.index.Batch
	if p.commitBatch != nil {
		commitBatch = p.commitBatch
	}
	commitTimer := time.AfterFunc(slowIndexOperation, func() {
		p.logger.Warn("Search index operation is slow",
			"stage", "bleve_batch_commit",
			"batch_events", len(items),
			"first_seq", firstSequence,
			"last_seq", lastSequence,
			"threshold", slowIndexOperation)
	})
	if err := commitBatch(batch.index); err != nil {
		commitTimer.Stop()
		return fmt.Errorf("commit search index batch: %w", err)
	}
	commitTimer.Stop()
	if batch.dekChanged {
		p.deks = batch.deks
	}
	p.checkpoint = record
	return nil
}

func (p *Projection) applyEvent(batch *projectionBatch, event *corev1.Event, seq uint64) error {
	switch payload := event.GetEvent().(type) {
	case *corev1.Event_UserDekGenerated:
		dek := payload.UserDekGenerated
		if dek != nil {
			batch.makeDEKsMutable()
			batch.deks[dekKey(dek.GetUserId(), dek.GetPurpose(), dek.GetEpoch())] = proto.Clone(dek).(*corev1.UserDEKGeneratedEvent)
			batch.dekChanged = true
		}
	case *corev1.Event_MessageBody:
		bodyEvent := payload.MessageBody
		if bodyEvent != nil && bodyEvent.GetBody() != nil {
			if claimed := bodyEvent.GetBody().GetBodyEventId(); claimed != "" && claimed != event.GetId() {
				break
			}
			state, err := batch.loadMessage(bodyEvent.GetEventId())
			if err != nil {
				return err
			}
			if seq > state.BodySequence {
				plaintext, err := p.decryptBodyWithDEKs(context.Background(), bodyEvent.GetEventId(), bodyEvent.GetRoomId(), bodyEvent.GetBody(), batch.deks)
				if err != nil && !errors.Is(err, encryption.ErrKeyNotFound) {
					return err
				}
				body := bodyEvent.GetBody()
				state.MessageID = bodyEvent.GetEventId()
				state.RoomID = bodyEvent.GetRoomId()
				state.AuthorID = body.GetAuthorId()
				state.BodyEventID = event.GetId()
				state.Body = string(plaintext)
				state.HasAttachments = len(body.GetAttachments()) > 0 || len(body.GetAssetIds()) > 0
				if body.GetCreatedAt() != nil {
					state.CreatedAt = body.GetCreatedAt().AsTime()
				}
				if body.GetUpdatedAt() != nil {
					state.UpdatedAt = body.GetUpdatedAt().AsTime()
				}
				state.BodySequence = seq
				if err := batch.storeMessage(state); err != nil {
					return err
				}
			}
		}
	case *corev1.Event_MessagePosted:
		posted := payload.MessagePosted
		state, err := batch.loadMessage(event.GetId())
		if err != nil {
			return err
		}
		if seq > state.PostedSequence {
			state.MessageID = event.GetId()
			state.RoomID = posted.GetRoomId()
			state.AuthorID = event.GetActorId()
			state.Visible = true
			if event.GetCreatedAt() != nil {
				state.CreatedAt = event.GetCreatedAt().AsTime()
			}
			state.PostedSequence = seq
			if err := batch.storeMessage(state); err != nil {
				return err
			}
		}
	case *corev1.Event_MessageRetracted:
		batch.deleteMessage(payload.MessageRetracted.GetEventId())
	case *corev1.Event_RoomDeleted:
		if err := batch.deleteMatching("room_id", payload.RoomDeleted.GetRoomId()); err != nil {
			return err
		}
	case *corev1.Event_UserKeyShredded:
		userID := payload.UserKeyShredded.GetUserId()
		if err := batch.deleteMatching("author_id", userID); err != nil {
			return err
		}
		var keys []string
		for key, dek := range batch.deks {
			if dek.GetUserId() == userID {
				keys = append(keys, key)
			}
		}
		if len(keys) > 0 {
			batch.makeDEKsMutable()
			for _, key := range keys {
				delete(batch.deks, key)
			}
			batch.dekChanged = true
		}
	}
	return nil
}

func (p *Projection) RestoreCheckpoint(_ context.Context, request events.ProjectionCheckpointRequest) (events.ProjectionCheckpoint, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	data, err := p.index.GetInternal([]byte(checkpointInternalKey))
	if err != nil {
		return events.ProjectionCheckpoint{}, fmt.Errorf("read search checkpoint: %w", err)
	}
	if len(data) == 0 {
		p.checkpoint = checkpointFromRequest(request)
		return events.ProjectionCheckpoint{}, nil
	}
	var record checkpointRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return events.ProjectionCheckpoint{}, fmt.Errorf("%w: decode search checkpoint: %v", events.ErrProjectionCheckpointInvalid, err)
	}
	if record.ProjectionKey != request.ProjectionKey || record.ContractID != request.ContractID || record.StreamName != request.StreamName || record.StreamIdentity != request.StreamIdentity {
		return events.ProjectionCheckpoint{}, fmt.Errorf("%w: search checkpoint contract or EVT stream changed", events.ErrProjectionCheckpointInvalid)
	}
	dekData, err := p.index.GetInternal([]byte(dekInternalKey))
	if err != nil {
		return events.ProjectionCheckpoint{}, fmt.Errorf("read search DEK metadata: %w", err)
	}
	deks, err := decodeDEKs(dekData)
	if err != nil {
		return events.ProjectionCheckpoint{}, fmt.Errorf("%w: decode search DEK metadata: %v", events.ErrProjectionCheckpointInvalid, err)
	}
	p.deks = deks
	p.checkpoint = record
	return events.ProjectionCheckpoint{CutoffSequence: record.CutoffSequence}, nil
}

func (p *Projection) ResetCheckpoint(_ context.Context, request events.ProjectionCheckpointRequest) error {
	return fmt.Errorf(
		"search index %q is incompatible with projection %q; stop the provider, move or delete that directory, and restart to rebuild it",
		p.directory,
		request.ProjectionKey,
	)
}

func (p *Projection) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.index == nil {
		return nil
	}
	err := p.index.Close()
	p.index = nil
	return err
}

func checkpointFromRequest(r events.ProjectionCheckpointRequest) checkpointRecord {
	return checkpointRecord{ProjectionKey: r.ProjectionKey, ContractID: r.ContractID, StreamName: r.StreamName, StreamIdentity: r.StreamIdentity}
}

func (p *Projection) open() error {
	if err := os.MkdirAll(filepath.Dir(p.directory), 0o755); err != nil {
		return fmt.Errorf("create search index parent: %w", err)
	}
	index, err := blevesearch.Open(p.directory)
	if err == nil {
		p.index = index
		return nil
	}
	create := errors.Is(err, blevesearch.ErrorIndexPathDoesNotExist)
	if errors.Is(err, blevesearch.ErrorIndexMetaMissing) {
		entries, readErr := os.ReadDir(p.directory)
		if readErr != nil {
			return fmt.Errorf("inspect search index directory %q: %w", p.directory, readErr)
		}
		create = len(entries) == 0
	}
	if !create {
		return fmt.Errorf(
			"open search index %q: %w; Chatto will not modify an unreadable index, so move or delete that directory explicitly before restarting the provider",
			p.directory,
			err,
		)
	}
	index, err = blevesearch.New(p.directory, newIndexMapping(p.languages))
	if err != nil {
		return fmt.Errorf("create search index: %w", err)
	}
	p.index = index
	return nil
}

func languageCheckpointContractID(languages []languageAnalyzer) string {
	codes := make([]string, len(languages))
	for i, language := range languages {
		codes[i] = language.code
	}
	sum := sha256.Sum256([]byte(strings.Join(codes, ",")))
	return fmt.Sprintf("%s-%x", checkpointContractBaseID, sum[:8])
}

func messageDocumentID(id string) string { return "message:" + id }

func (p *Projection) loadMessage(id string) (messageDocument, error) {
	state := messageDocument{MessageID: id}
	document, err := p.index.Document(messageDocumentID(id))
	if err != nil {
		return state, fmt.Errorf("read search message document: %w", err)
	}
	if document == nil {
		return state, nil
	}
	found := false
	var decodeErr error
	document.VisitFields(func(field bleveindex.Field) {
		if field.Name() != projectionStateField {
			return
		}
		found = true
		decodeErr = json.Unmarshal(field.Value(), &state)
	})
	if decodeErr != nil {
		return state, fmt.Errorf("decode search message state: %w", decodeErr)
	}
	if found {
		return state, nil
	}
	return state, fmt.Errorf("search message document %q has no projection state", id)
}

func (b *projectionBatch) loadMessage(id string) (messageDocument, error) {
	if state, ok := b.messages[id]; ok {
		return state, nil
	}
	if _, deleted := b.deletedMessages[id]; deleted {
		return messageDocument{MessageID: id}, nil
	}
	return b.projection.loadMessage(id)
}

func (b *projectionBatch) storeMessage(state messageDocument) error {
	state.ProjectionState = ""
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	state.ProjectionState = string(data)
	if err := b.index.Index(messageDocumentID(state.MessageID), state); err != nil {
		return fmt.Errorf("index message: %w", err)
	}
	state.ProjectionState = ""
	b.messages[state.MessageID] = state
	delete(b.deletedMessages, state.MessageID)
	return nil
}

func (b *projectionBatch) deleteMessage(id string) {
	if id == "" {
		return
	}
	b.index.Delete(messageDocumentID(id))
	delete(b.messages, id)
	b.deletedMessages[id] = struct{}{}
}

func encodeDEKs(deks map[string]*corev1.UserDEKGeneratedEvent) ([]byte, error) {
	persisted := make(persistedDEKs, len(deks))
	for key, event := range deks {
		data, err := proto.Marshal(event)
		if err != nil {
			return nil, err
		}
		persisted[key] = base64.RawStdEncoding.EncodeToString(data)
	}
	return json.Marshal(persisted)
}

func decodeDEKs(data []byte) (map[string]*corev1.UserDEKGeneratedEvent, error) {
	result := make(map[string]*corev1.UserDEKGeneratedEvent)
	if len(data) == 0 {
		return result, nil
	}
	var persisted persistedDEKs
	if err := json.Unmarshal(data, &persisted); err != nil {
		return nil, err
	}
	for key, encoded := range persisted {
		data, err := base64.RawStdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, err
		}
		var event corev1.UserDEKGeneratedEvent
		if err := proto.Unmarshal(data, &event); err != nil {
			return nil, err
		}
		result[key] = &event
	}
	return result, nil
}

func dekKey(userID string, purpose corev1.UserDEKPurpose, epoch int32) string {
	return fmt.Sprintf("%s/%d/%d", userID, purpose, epoch)
}
