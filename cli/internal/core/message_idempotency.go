package core

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	runtimestatev1 "hmans.de/chatto/internal/pb/chatto/core/runtime_state/v1"
	"hmans.de/chatto/pkg/events"
	"hmans.de/chatto/pkg/jetstreamutil"
)

// MessageIdempotencyWindow is the fixed lifetime of an accepted send key.
// Callers must stop retrying within this period after their first attempt;
// an expired key can reserve another message identity.
const MessageIdempotencyWindow = 30 * time.Minute

const messagePostClaimPrefix = "message_post."

// MessagePostAppliedError reports that EVT contains the requested message but
// its result cannot be returned. Unwrap retains the current read-policy error.
// Transports must not describe this as an unapplied write.
type MessagePostAppliedError struct{ Cause error }

func (e *MessagePostAppliedError) Error() string { return "message was posted; result unavailable" }
func (e *MessagePostAppliedError) Unwrap() error { return e.Cause }

// messagePostClaim is request-local. Its immutable KV record reserves an ID,
// while committed is populated only from the authoritative EVT identity.
type messagePostClaim struct {
	messageID string
	expiresAt time.Time
	committed *evtstream.SubjectEvent
}

func messagePostClaimKey(actorID, key string) (string, error) {
	parsed, err := uuid.Parse(key)
	if err != nil || len(key) != 36 || parsed == uuid.Nil || parsed.Variant() != uuid.RFC4122 {
		return "", invalidArgument("idempotency_key must be a non-zero UUID in hyphenated form")
	}
	return messagePostClaimPrefix + actorID + "." + parsed.String(), nil
}

// claimMessagePost uses a leader read after each Create collision. A collision
// can describe an in-flight write which later fails; it does not prove a claim
// exists. A successful claim is never refreshed or marked complete.
func (s *MessageModel) claimMessagePost(ctx context.Context, input MessagePostInput) (*messagePostClaim, error) {
	if input.IdempotencyKey == "" {
		return nil, nil
	}
	if input.ActorID == "" {
		return nil, ErrNotAuthenticated
	}
	key, err := messagePostClaimKey(input.ActorID, input.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	fingerprint, err := s.postFingerprint(input)
	if err != nil {
		return nil, err
	}
	kv := s.core.storage.runtimeStateKV
	for range 5 {
		entry, err := kv.Get(ctx, key)
		if err == nil {
			return decodeMessagePostClaim(entry, fingerprint)
		}
		if !errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, fmt.Errorf("read message send claim: %w", err)
		}
		// Check current posting authority before allocating runtime storage.
		prepared, err := s.applyAutomaticThreadCreation(ctx, input)
		if err != nil {
			return nil, err
		}
		if _, err := s.AuthorizePost(ctx, authorizationInputForPost(prepared, prepared.ThreadRootEventID)); err != nil {
			return nil, err
		}
		record := &runtimestatev1.MessagePostClaim{MessageId: NewEventID(), RequestFingerprint: fingerprint}
		data, err := proto.Marshal(record)
		if err != nil {
			return nil, fmt.Errorf("encode message send claim: %w", err)
		}
		revision, err := kv.Create(ctx, key, data, jetstream.KeyTTL(MessageIdempotencyWindow))
		if err == nil {
			entry, err = kv.GetRevision(ctx, key, revision)
			if err != nil {
				return nil, fmt.Errorf("read accepted message send claim: %w", err)
			}
			return decodeMessagePostClaim(entry, fingerprint)
		}
		if !jetstreamutil.IsSequenceConflict(err) {
			return nil, fmt.Errorf("create message send claim: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("message send claim remained contested: %w", events.ErrConflict)
}

func decodeMessagePostClaim(entry jetstream.KeyValueEntry, fingerprint []byte) (*messagePostClaim, error) {
	var record runtimestatev1.MessagePostClaim
	if err := proto.Unmarshal(entry.Value(), &record); err != nil {
		return nil, fmt.Errorf("decode message send claim: %w", err)
	}
	if record.GetMessageId() == "" || len(record.GetRequestFingerprint()) != sha256.Size {
		return nil, errors.New("message send claim is incomplete")
	}
	if !hmac.Equal(record.GetRequestFingerprint(), fingerprint) {
		return nil, ErrMessageIdempotencyConflict
	}
	return &messagePostClaim{messageID: record.GetMessageId(), expiresAt: entry.Created().Add(MessageIdempotencyWindow)}, nil
}

// postFingerprint covers caller intent before room defaults, mention expansion,
// preview resolution, and asset attachment. JSON is only transient canonical
// hash input; the persisted claim uses protobuf and contains no request text.
func (s *MessageModel) postFingerprint(input MessagePostInput) ([]byte, error) {
	if len(input.Body) > MaxMessageBodyLength {
		return nil, ErrMessageTooLong
	}
	if err := validateMessageAttachmentAssetIDs(input.AttachmentAssetIDs); err != nil {
		return nil, err
	}
	descriptions, err := normalizeAttachmentDescriptionInputs(input.AttachmentAssetIDs, input.AttachmentDescriptions)
	if err != nil {
		return nil, err
	}
	for id, description := range descriptions {
		if description == "" {
			delete(descriptions, id)
		}
	}
	var assetIDs []string
	for _, id := range input.AttachmentAssetIDs {
		if !slices.Contains(assetIDs, id) {
			assetIDs = append(assetIDs, id)
		}
	}
	processingIDs := slices.Clone(input.VideoProcessingAssetIDs)
	slices.Sort(processingIDs)
	processingIDs = slices.Compact(processingIDs)
	if len(processingIDs) == 0 {
		processingIDs = nil
	}
	var preview []byte
	if input.LinkPreview != nil {
		preview, err = (proto.MarshalOptions{Deterministic: true}).Marshal(input.LinkPreview)
		if err != nil {
			return nil, fmt.Errorf("encode preview intent: %w", err)
		}
	}
	intent := struct {
		RoomID, Body, ThreadRootID, InReplyTo, PreviewToken string
		AssetIDs, ProcessingIDs                             []string
		Descriptions                                        map[string]string `json:",omitempty"`
		AlsoSendToChannel, CreateThread                     bool
		Preview                                             []byte `json:",omitempty"`
	}{input.RoomID, input.Body, input.ThreadRootEventID, input.InReplyTo, input.LinkPreviewToken,
		assetIDs, processingIDs, descriptions, input.AlsoSendToChannel, input.CreateThread, preview}
	data, err := json.Marshal(intent)
	if err != nil {
		return nil, fmt.Errorf("encode message send intent: %w", err)
	}
	return hex.DecodeString(s.core.runtimeTokenHash("message_post_request_v1", string(data)))
}

// claimedMessage decides absence only after the local projection has applied
// the captured room tail. Each new write later fences that same room history.
func (s *MessageModel) claimedMessage(ctx context.Context, input MessagePostInput, claim *messagePostClaim) (*evtv1.Event, error) {
	agg := evtstream.RoomAggregate(input.RoomID)
	position, err := s.core.EventPublisher.LastSubjectPosition(ctx, agg.AllEventsFilter())
	if err != nil {
		return nil, err
	}
	if err := s.core.roomModel.waitForDirectoryAndTimeline(ctx, position); err != nil {
		return nil, err
	}
	if err := s.core.lookupClaimedMessage(ctx, input.ActorID, input.RoomID, claim); err != nil {
		return nil, err
	}
	if claim.committed == nil {
		return nil, nil
	}
	return claim.committed.Event, nil
}

// lookupClaimedMessage runs after room catch-up, including inside every OCC
// attempt. A retracted or edited post still counts as committed. Only current
// read authority permits replaying the result; new-write gates are not rerun.
func (c *ChattoCore) lookupClaimedMessage(ctx context.Context, actorID, roomID string, claim *messagePostClaim) error {
	entry, exists := c.roomModel.timelineEntry(claim.messageID)
	if !exists {
		return nil
	}
	if !entry.IsMessagePost() || entry.RoomID != roomID || entry.ActorID != actorID {
		return errors.New("message send claim points to an invalid identity")
	}
	err := c.authorizeAtStableInputs(ctx, func() error {
		_, _, err := c.requireMessageReader(ctx, actorID, roomID, claim.messageID)
		return err
	})
	if err != nil {
		return &MessagePostAppliedError{Cause: err}
	}
	stored, err := c.eventReader.EventAt(ctx, entry.StreamSeq)
	if err != nil {
		return &MessagePostAppliedError{Cause: err}
	}
	claim.committed = stored
	return nil
}
