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
	"github.com/blevesearch/bleve/v2/index/scorch"
	"github.com/charmbracelet/log"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/dekstore"
	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/kms"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	checkpointContractBaseID = "bleve-message-index-v6"
	checkpointInternalKey    = "chatto/search/checkpoint"
	dekInternalKey           = "chatto/search/deks"
	messageStatePrefix       = "chatto/search/message/"
	privacyCompactionKey     = "chatto/search/privacy-compaction-pending"
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
}

type persistedDEKs map[string]string

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
}

func NewProjection(directory string, languageCodes []string, keyWrapper kms.KeyWrapper, legacyKeys kms.LegacyKeyProvider, dekStore dekstore.Reader, logger *log.Logger) (*Projection, error) {
	directory = strings.TrimSpace(directory)
	cleanDirectory := filepath.Clean(directory)
	if directory == "" || cleanDirectory == "." || cleanDirectory == filepath.VolumeName(cleanDirectory)+string(filepath.Separator) {
		return nil, fmt.Errorf("search index requires a dedicated directory")
	}
	if logger == nil {
		logger = log.WithPrefix("search-provider")
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
		events.SubjectRoot + events.AggregateRoom + ".>",
		events.SubjectRoot + events.AggregateUser + ".>",
	}
}

func (p *Projection) CheckpointContractID() string { return p.contractID }

func (p *Projection) Apply(event *corev1.Event, seq uint64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.index == nil {
		return fmt.Errorf("search index is closed")
	}
	batch := p.index.NewBatch()
	dekChanged := false
	privacyCompaction := false

	switch payload := event.GetEvent().(type) {
	case *corev1.Event_UserDekGenerated:
		dek := payload.UserDekGenerated
		if dek != nil {
			p.deks[dekKey(dek.GetUserId(), dek.GetPurpose(), dek.GetEpoch())] = proto.Clone(dek).(*corev1.UserDEKGeneratedEvent)
			dekChanged = true
		}
	case *corev1.Event_MessageBody:
		bodyEvent := payload.MessageBody
		if bodyEvent != nil && bodyEvent.GetBody() != nil {
			if claimed := bodyEvent.GetBody().GetBodyEventId(); claimed != "" && claimed != event.GetId() {
				break
			}
			state, err := p.loadMessage(bodyEvent.GetEventId())
			if err != nil {
				return err
			}
			if seq > state.BodySequence {
				plaintext, err := p.decryptBody(context.Background(), bodyEvent.GetEventId(), bodyEvent.GetRoomId(), bodyEvent.GetBody())
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
				if err := p.storeMessage(batch, state); err != nil {
					return err
				}
			}
		}
	case *corev1.Event_MessagePosted:
		posted := payload.MessagePosted
		state, err := p.loadMessage(event.GetId())
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
			if err := p.storeMessage(batch, state); err != nil {
				return err
			}
		}
	case *corev1.Event_MessageRetracted:
		if err := p.deleteMessage(batch, payload.MessageRetracted.GetEventId()); err != nil {
			return err
		}
	case *corev1.Event_RoomDeleted:
		if err := p.deleteMatching(batch, "room_id", payload.RoomDeleted.GetRoomId()); err != nil {
			return err
		}
	case *corev1.Event_UserKeyShredded:
		userID := payload.UserKeyShredded.GetUserId()
		if err := p.deleteMatching(batch, "author_id", userID); err != nil {
			return err
		}
		for key, dek := range p.deks {
			if dek.GetUserId() == userID {
				delete(p.deks, key)
				dekChanged = true
			}
		}
		privacyCompaction = true
	}

	if dekChanged {
		data, err := encodeDEKs(p.deks)
		if err != nil {
			return err
		}
		batch.SetInternal([]byte(dekInternalKey), data)
	}
	if privacyCompaction {
		batch.SetInternal([]byte(privacyCompactionKey), []byte{1})
	}
	record := p.checkpoint
	record.CutoffSequence = seq
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	batch.SetInternal([]byte(checkpointInternalKey), data)
	if err := p.index.Batch(batch); err != nil {
		return fmt.Errorf("commit search index batch: %w", err)
	}
	p.checkpoint = record
	if privacyCompaction {
		if err := p.completePrivacyCompaction(); err != nil {
			return err
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
	compactionPending, err := p.index.GetInternal([]byte(privacyCompactionKey))
	if err != nil {
		return events.ProjectionCheckpoint{}, fmt.Errorf("read search privacy compaction state: %w", err)
	}
	if len(compactionPending) > 0 {
		if err := p.completePrivacyCompaction(); err != nil {
			return events.ProjectionCheckpoint{}, err
		}
	}
	return events.ProjectionCheckpoint{CutoffSequence: record.CutoffSequence}, nil
}

func (p *Projection) completePrivacyCompaction() error {
	advanced, err := p.index.Advanced()
	if err != nil {
		return fmt.Errorf("open Bleve index for privacy compaction: %w", err)
	}
	scorchIndex, ok := advanced.(*scorch.Scorch)
	if !ok {
		return fmt.Errorf("Bleve index does not support privacy compaction")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	for {
		err := scorchIndex.ForceMerge(ctx, nil)
		if err == nil {
			break
		}
		if !strings.Contains(err.Error(), "force merge already in progress") {
			return fmt.Errorf("compact search index after key shredding: %w", err)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("compact search index after key shredding: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	if err := p.index.DeleteInternal([]byte(privacyCompactionKey)); err != nil {
		return fmt.Errorf("clear search privacy compaction marker: %w", err)
	}
	return nil
}

func (p *Projection) ResetCheckpoint(_ context.Context, request events.ProjectionCheckpointRequest) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.index != nil {
		if err := p.index.Close(); err != nil {
			return fmt.Errorf("close search index before reset: %w", err)
		}
		p.index = nil
	}
	p.logger.Info("Resetting disposable search index", "stage", "checkpoint_reset")
	if err := os.RemoveAll(p.directory); err != nil {
		return fmt.Errorf("remove search index: %w", err)
	}
	p.deks = make(map[string]*corev1.UserDEKGeneratedEvent)
	p.checkpoint = checkpointFromRequest(request)
	return p.open()
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
	if errors.Is(err, blevesearch.ErrorIndexMetaMissing) || errors.Is(err, blevesearch.ErrorIndexMetaCorrupt) {
		// The index is a disposable projection. If an existing index cannot be
		// opened because its metadata is positively corrupt, discard it so the
		// projector can replay EVT. Operational I/O failures return untouched.
		p.logger.Warn("Discarding unreadable search index", "stage", "index_recovery", "error", err)
		if removeErr := os.RemoveAll(p.directory); removeErr != nil {
			return fmt.Errorf("remove unreadable search index after %v: %w", err, removeErr)
		}
	} else if !errors.Is(err, blevesearch.ErrorIndexPathDoesNotExist) {
		return fmt.Errorf("open search index: %w", err)
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

func messageStateKey(id string) []byte   { return []byte(messageStatePrefix + id) }
func messageDocumentID(id string) string { return "message:" + id }

func (p *Projection) loadMessage(id string) (messageDocument, error) {
	state := messageDocument{MessageID: id}
	data, err := p.index.GetInternal(messageStateKey(id))
	if err != nil {
		return state, fmt.Errorf("read search message state: %w", err)
	}
	if len(data) == 0 {
		return state, nil
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("decode search message state: %w", err)
	}
	return state, nil
}

func (p *Projection) storeMessage(batch *blevesearch.Batch, state messageDocument) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	batch.SetInternal(messageStateKey(state.MessageID), data)
	if err := batch.Index(messageDocumentID(state.MessageID), state); err != nil {
		return fmt.Errorf("index message: %w", err)
	}
	return nil
}

func (p *Projection) deleteMessage(batch *blevesearch.Batch, id string) error {
	if id == "" {
		return nil
	}
	batch.Delete(messageDocumentID(id))
	batch.DeleteInternal(messageStateKey(id))
	return nil
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
