package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/publiccursor"
)

const (
	realtimeCursorVersion         = 2
	realtimeCursorPurpose         = "realtime-resume-v2"
	realtimeCursorLifetime        = 24 * time.Hour
	realtimeCursorFutureSkew      = 5 * time.Minute
	realtimeReplayMaxSequenceSpan = uint64(10_000)
	realtimeReplayMaxEvents       = 2_000
)

var (
	// ErrRealtimeCursorInvalid means the cursor is malformed, references a
	// different EVT incarnation, or points beyond the current stream.
	ErrRealtimeCursorInvalid = errors.New("invalid realtime cursor")
	// ErrRealtimeCursorExpired means the cursor is older than its public
	// lifetime or precedes retained EVT history.
	ErrRealtimeCursorExpired = errors.New("realtime cursor expired")
	// ErrRealtimeReplayLimitExceeded means the requested gap exceeds the
	// bounded reconnect replay budget.
	ErrRealtimeReplayLimitExceeded = errors.New("realtime replay limit exceeded")
)

type realtimeCursorPayload struct {
	Version        int    `json:"v"`
	StreamIdentity string `json:"i"`
	Sequence       uint64 `json:"s"`
	UserID         string `json:"u"`
	IssuedAtUnix   int64  `json:"t"`
}

// RealtimeReplayPlan is a bounded, authorized durable replay ending at one
// stable EVT boundary. The caller starts live delivery before requesting the
// plan, buffers that stream while replay is sent, and discards buffered EVT
// events through BoundarySequence before continuing live.
type RealtimeReplayPlan struct {
	// Reset requires a compacted current-state prefix before replay/live events.
	Reset bool
	// StartCursor is the validated request cursor, or BoundaryCursor for a
	// subscription that did not request history.
	StartCursor string
	// BoundaryCursor is safe to persist after all Events have been applied.
	BoundaryCursor string
	// BoundarySequence is the EVT cutoff used to suppress buffered duplicates.
	BoundarySequence uint64
	// Events contains authorized deliverable durable events in global EVT order.
	Events []EventEnvelope
	// HadSequenceGap records that the validated request cursor preceded the
	// captured boundary, including gaps that ultimately require a reset.
	HadSequenceGap bool
}

// RealtimeCursorForSequence returns the opaque public cursor for one durable
// EVT delivery sequence.
func (c *ChattoCore) RealtimeCursorForSequence(userID string, sequence uint64) (string, error) {
	identity, err := events.StreamIdentity(c.storage.serverEvtStream)
	if err != nil {
		return "", fmt.Errorf("read EVT stream identity: %w", err)
	}
	return c.encodeRealtimeCursor(userID, identity, sequence)
}

// RealtimeCursorAtCurrentBoundary reports whether cursor already names the
// current EVT boundary for userID. It lets transport admission distinguish a
// cheap, no-gap reconnect from a replay attempt without exposing the internal
// stream sequence carried by the opaque cursor.
func (c *ChattoCore) RealtimeCursorAtCurrentBoundary(ctx context.Context, userID, cursor string) (bool, error) {
	if strings.TrimSpace(cursor) == "" {
		return false, nil
	}
	decoded, err := c.decodeRealtimeCursor(userID, cursor)
	if err != nil {
		// Invalid, expired, cross-user, and old-incarnation cursors all take the
		// normal metered path. PlanRealtimeReplay will later turn them into a
		// safe compacted reset.
		return false, nil
	}
	identity, err := events.StreamIdentity(c.storage.serverEvtStream)
	if err != nil {
		return false, fmt.Errorf("read EVT stream identity: %w", err)
	}
	if decoded.StreamIdentity != identity {
		return false, nil
	}
	info, err := c.storage.serverEvtStream.Info(ctx)
	if err != nil {
		return false, fmt.Errorf("read EVT stream info: %w", err)
	}
	return decoded.Sequence == info.State.LastSeq, nil
}

// PlanRealtimeReplay builds a caller-wide replay of public durable events after
// resumeCursor. An empty cursor starts at the current EVT boundary and returns
// no history. Authorization uses the caller's current room visibility;
// transient live.sync events are intentionally not replayed.
//
// This initial implementation scans a bounded global sequence range directly.
// It is suitable for reconnect gaps, not bulk event-log export.
func (c *ChattoCore) PlanRealtimeReplay(ctx context.Context, userID, resumeCursor string) (RealtimeReplayPlan, error) {
	stream := c.storage.serverEvtStream
	identity, err := events.StreamIdentity(stream)
	if err != nil {
		return RealtimeReplayPlan{}, fmt.Errorf("read EVT stream identity: %w", err)
	}
	info, err := stream.Info(ctx)
	if err != nil {
		return RealtimeReplayPlan{}, fmt.Errorf("read EVT stream info: %w", err)
	}
	boundarySeq := info.State.LastSeq
	boundaryCursor, err := c.encodeRealtimeCursor(userID, identity, boundarySeq)
	if err != nil {
		return RealtimeReplayPlan{}, err
	}
	// The public cursor promises that every current-state read used to shape
	// authorization or a compacted reset includes all durable facts through
	// this boundary. Waiting here, before any reset early-return or membership
	// capture, prevents a lagging replica from publishing stale plaintext or
	// permissions and then discarding the durable facts that would correct it.
	if err := c.WaitForProjectionsCurrent(ctx); err != nil {
		return RealtimeReplayPlan{}, fmt.Errorf("wait for realtime projection boundary: %w", err)
	}

	plan := RealtimeReplayPlan{
		Reset:            strings.TrimSpace(resumeCursor) == "",
		StartCursor:      boundaryCursor,
		BoundaryCursor:   boundaryCursor,
		BoundarySequence: boundarySeq,
	}
	if strings.TrimSpace(resumeCursor) == "" {
		return plan, nil
	}

	cursor, err := c.decodeRealtimeCursor(userID, resumeCursor)
	if err != nil {
		plan.Reset = true
		return plan, nil
	}
	if cursor.StreamIdentity != identity || cursor.Sequence > boundarySeq {
		plan.Reset = true
		return plan, nil
	}
	plan.HadSequenceGap = cursor.Sequence < boundarySeq
	if info.State.FirstSeq > 0 && cursor.Sequence < info.State.FirstSeq-1 {
		plan.Reset = true
		return plan, nil
	}
	if boundarySeq-cursor.Sequence > realtimeReplayMaxSequenceSpan {
		plan.Reset = true
		return plan, nil
	}
	plan.StartCursor = resumeCursor

	memberRooms := make(map[string]struct{})
	if err := c.myEvents().populateMemberRoomsCache(ctx, userID, memberRooms); err != nil {
		return RealtimeReplayPlan{}, fmt.Errorf("load replay room visibility: %w", err)
	}

	for seq := cursor.Sequence + 1; seq <= boundarySeq; seq++ {
		msg, err := stream.GetMsg(ctx, seq)
		if err != nil {
			if errors.Is(err, jetstream.ErrMsgNotFound) {
				continue
			}
			return RealtimeReplayPlan{}, fmt.Errorf("read EVT sequence %d: %w", seq, err)
		}

		if strings.HasPrefix(msg.Subject, strings.TrimSuffix(events.RBACSubjectFilter(), ">")) {
			// RBAC changes can revoke visibility without producing a room event.
			// Rebuild from current authorized state rather than risk retaining a
			// resource that the viewer may no longer read.
			plan.Reset = true
			plan.Events = nil
			plan.StartCursor = boundaryCursor
			return plan, nil
		}
		if realtimeReplayRequiresReset(msg.Subject) {
			plan.Reset = true
			plan.Events = nil
			plan.StartCursor = boundaryCursor
			return plan, nil
		}

		var event corev1.Event
		if err := proto.Unmarshal(msg.Data, &event); err != nil {
			return RealtimeReplayPlan{}, fmt.Errorf("decode EVT sequence %d: %w", seq, err)
		}
		if event.GetUserKeyShredded() != nil {
			// Key shredding can tombstone messages across many retained rooms.
			// A reset purges every cached plaintext row in one ordered operation.
			plan.Reset = true
			plan.Events = nil
			plan.StartCursor = boundaryCursor
			return plan, nil
		}
		roomID, roomSubject := realtimeReplayRoomSubject(msg.Subject)
		assetID, assetSubject := events.ParseAssetSubject(msg.Subject)
		_, userSubject := events.ParseUserSubject(msg.Subject)
		switch {
		case roomSubject:
			if !isDeliverableLiveEVTRoomEvent(&event) {
				continue
			}
			legacyAssetEvent := isAssetLifecycleEvent(&event)
			if !legacyAssetEvent && roomIDOfEvent(&event) != roomID {
				continue
			}
			if _, authorized := memberRooms[roomID]; !authorized {
				// A caller that lost access during the gap must discard its old
				// room state. A compacted replay is the only safe way to do that
				// without disclosing rooms it never held.
				if eventChangesRoomVisibility(&event) || isRoomDirectoryProjectionEvent(&event) {
					plan.Reset = true
					plan.Events = nil
					plan.StartCursor = boundaryCursor
					return plan, nil
				}
				continue
			}
			waitCtx, cancel := context.WithTimeout(ctx, liveEVTProjectionWaitTimeout)
			err = c.myEvents().waitForLiveEVTRoomEvent(waitCtx, msg.Subject, &event, seq)
			cancel()
			if err != nil {
				return RealtimeReplayPlan{}, fmt.Errorf("wait for replay sequence %d: %w", seq, err)
			}
			if legacyAssetEvent {
				assetRoomID, ok := c.assetLifecycle().AssetRoomID(assetIDOfLifecycleEvent(&event))
				if !ok || assetRoomID != roomID {
					continue
				}
			}
		case assetSubject:
			if assetIDOfLifecycleEvent(&event) != assetID || !isDeliverableLiveEVTAssetEvent(&event) {
				continue
			}
			waitCtx, cancel := context.WithTimeout(ctx, liveEVTProjectionWaitTimeout)
			err = c.myEvents().waitForLiveEVTAssetEvent(waitCtx, msg.Subject, seq)
			cancel()
			if err != nil {
				return RealtimeReplayPlan{}, fmt.Errorf("wait for replay sequence %d: %w", seq, err)
			}
			assetRoomID, ok := c.assetLifecycle().AssetRoomID(assetID)
			if !ok {
				continue
			}
			if _, authorized := memberRooms[assetRoomID]; !authorized {
				continue
			}
		case userSubject:
			if !isDeliverableLiveEVTUserEvent(&event) {
				continue
			}
			waitCtx, cancel := context.WithTimeout(ctx, liveEVTProjectionWaitTimeout)
			err = c.myEvents().waitForLiveEVTUserEvent(waitCtx, msg.Subject, seq)
			cancel()
			if err != nil {
				return RealtimeReplayPlan{}, fmt.Errorf("wait for replay sequence %d: %w", seq, err)
			}
		default:
			continue
		}
		plan.Events = append(plan.Events, NewEVTEventEnvelopeWithDeliverySeq(&event, seq))
		if len(plan.Events) > realtimeReplayMaxEvents {
			plan.Reset = true
			plan.Events = nil
			plan.StartCursor = boundaryCursor
			return plan, nil
		}
	}

	return plan, nil
}

func realtimeReplayRequiresReset(subject string) bool {
	parts := strings.Split(subject, ".")
	if len(parts) < 2 || parts[0] != strings.TrimSuffix(events.SubjectRoot, ".") {
		return false
	}
	switch parts[1] {
	case events.AggregateConfig, events.AggregateGroup, events.AggregateLayout:
		return true
	default:
		return false
	}
}

func realtimeReplayRoomSubject(subject string) (string, bool) {
	parts := strings.Split(subject, ".")
	if len(parts) != 4 || parts[0] != "evt" || parts[1] != events.AggregateRoom || parts[2] == "" || parts[3] == "" {
		return "", false
	}
	return parts[2], true
}

func (c *ChattoCore) encodeRealtimeCursor(userID, streamIdentity string, sequence uint64) (string, error) {
	return c.encodeRealtimeCursorAt(userID, streamIdentity, sequence, time.Now())
}

func (c *ChattoCore) encodeRealtimeCursorAt(userID, streamIdentity string, sequence uint64, now time.Time) (string, error) {
	if userID == "" || !events.ValidStreamIdentity(streamIdentity) {
		return "", ErrRealtimeCursorInvalid
	}
	payload, err := json.Marshal(realtimeCursorPayload{
		Version:        realtimeCursorVersion,
		StreamIdentity: streamIdentity,
		Sequence:       sequence,
		UserID:         userID,
		IssuedAtUnix:   now.Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("encode realtime cursor: %w", err)
	}
	token, err := publiccursor.Seal(c.config.SecretKey, realtimeCursorPurpose, userID, payload)
	if err != nil {
		return "", fmt.Errorf("seal realtime cursor: %w", err)
	}
	return token, nil
}

func (c *ChattoCore) decodeRealtimeCursor(userID, cursor string) (realtimeCursorPayload, error) {
	return c.decodeRealtimeCursorAt(userID, cursor, time.Now())
}

func (c *ChattoCore) decodeRealtimeCursorAt(userID, cursor string, now time.Time) (realtimeCursorPayload, error) {
	payload, err := publiccursor.Open(c.config.SecretKey, realtimeCursorPurpose, userID, cursor)
	if err != nil {
		return realtimeCursorPayload{}, ErrRealtimeCursorInvalid
	}
	var decoded realtimeCursorPayload
	if err := json.Unmarshal(payload, &decoded); err != nil || decoded.Version != realtimeCursorVersion || decoded.UserID != userID || decoded.IssuedAtUnix <= 0 || !events.ValidStreamIdentity(decoded.StreamIdentity) {
		return realtimeCursorPayload{}, ErrRealtimeCursorInvalid
	}
	issuedAt := time.Unix(decoded.IssuedAtUnix, 0)
	if issuedAt.After(now.Add(realtimeCursorFutureSkew)) {
		return realtimeCursorPayload{}, ErrRealtimeCursorInvalid
	}
	if now.Sub(issuedAt) > realtimeCursorLifetime {
		return realtimeCursorPayload{}, ErrRealtimeCursorExpired
	}
	return decoded, nil
}
