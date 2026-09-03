package core

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

const (
	realtimeCursorVersion         = 4
	realtimeCursorPurpose         = "chatto-realtime-resume-v4"
	realtimeCursorAudience        = "chatto-realtime"
	realtimeCursorScope           = "all-events"
	realtimeCursorLifetime        = 15 * time.Minute
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
)

type realtimeCursorClaims struct {
	Position string `json:"p"`
	Version  int    `json:"v"`
	jwt.RegisteredClaims
}

// RealtimeReplayPlan is a bounded, authorized durable replay ending at one
// stable EVT boundary. The caller starts live delivery before requesting the
// plan, buffers that stream while replay is sent, and discards buffered EVT
// events through BoundarySequence before continuing live.
type RealtimeReplayPlan struct {
	// Reset requires the transport to use the requested current-state fallback.
	Reset bool
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
	identity, err := evtstream.Identity(c.storage.serverEvtStream)
	if err != nil {
		return "", fmt.Errorf("read EVT stream identity: %w", err)
	}
	return c.encodeRealtimeCursor(userID, identity, sequence)
}

// RealtimeCursorAtCurrentBoundary reports whether cursor already names the
// current EVT boundary for userID. It lets transport admission distinguish a
// cheap, no-gap reconnect from a replay attempt without exposing the internal
// stream sequence represented by the opaque position claim.
func (c *ChattoCore) RealtimeCursorAtCurrentBoundary(ctx context.Context, userID, cursor string) (bool, error) {
	if strings.TrimSpace(cursor) == "" {
		return false, nil
	}
	identity, err := evtstream.Identity(c.storage.serverEvtStream)
	if err != nil {
		return false, fmt.Errorf("read EVT stream identity: %w", err)
	}
	info, err := c.storage.serverEvtStream.Info(ctx)
	if err != nil {
		return false, fmt.Errorf("read EVT stream info: %w", err)
	}
	claims, err := c.parseRealtimeCursorAt(userID, cursor, time.Now())
	if err != nil {
		// Invalid, expired, and cross-user cursors take the normal metered
		// recovery path. PlanRealtimeReplay turns them into a safe fallback.
		return false, nil
	}
	return hmac.Equal(
		[]byte(claims.Position),
		[]byte(c.realtimeCursorPosition(identity, userID, info.State.LastSeq)),
	), nil
}

// WaitForRealtimeCursor validates one viewer-bound public cursor and waits
// until this replica's projections include at least that EVT boundary.
//
// The projection wait intentionally captures each projection's current
// relevant stream target. That target is at or after the validated cursor and
// avoids treating an unrelated global EVT message as input to every projector.
func (c *ChattoCore) WaitForRealtimeCursor(ctx context.Context, userID, cursor string) error {
	identity, err := evtstream.Identity(c.storage.serverEvtStream)
	if err != nil {
		return fmt.Errorf("read EVT stream identity: %w", err)
	}
	info, err := c.storage.serverEvtStream.Info(ctx)
	if err != nil {
		return fmt.Errorf("read EVT stream info: %w", err)
	}
	if _, err := c.resolveRealtimeCursorAt(userID, strings.TrimSpace(cursor), identity, info.State.FirstSeq, info.State.LastSeq, time.Now()); err != nil {
		return err
	}
	if err := c.WaitForProjectionsCurrent(ctx); err != nil {
		return fmt.Errorf("wait for realtime resource boundary: %w", err)
	}
	return nil
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
	identity, err := evtstream.Identity(stream)
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
	// authorization or a snapshot fallback includes all durable facts through
	// this boundary. Waiting here, before any reset early-return or membership
	// capture, prevents a lagging replica from publishing stale plaintext or
	// permissions and then discarding the durable facts that would correct it.
	if err := c.WaitForProjectionsCurrent(ctx); err != nil {
		return RealtimeReplayPlan{}, fmt.Errorf("wait for realtime projection boundary: %w", err)
	}

	plan := RealtimeReplayPlan{
		Reset:            strings.TrimSpace(resumeCursor) == "",
		BoundaryCursor:   boundaryCursor,
		BoundarySequence: boundarySeq,
	}
	if strings.TrimSpace(resumeCursor) == "" {
		return plan, nil
	}

	cursorSequence, err := c.resolveRealtimeCursorAt(userID, resumeCursor, identity, info.State.FirstSeq, boundarySeq, time.Now())
	if err != nil {
		plan.Reset = true
		plan.HadSequenceGap = true
		return plan, nil
	}
	plan.HadSequenceGap = cursorSequence < boundarySeq

	memberRooms := make(map[string]struct{})
	if err := c.myEventsModel.populateMemberRoomsCache(ctx, userID, memberRooms); err != nil {
		return RealtimeReplayPlan{}, fmt.Errorf("load replay room visibility: %w", err)
	}

	for seq := cursorSequence + 1; seq <= boundarySeq; seq++ {
		msg, err := stream.GetMsg(ctx, seq)
		if err != nil {
			if errors.Is(err, jetstream.ErrMsgNotFound) {
				continue
			}
			return RealtimeReplayPlan{}, fmt.Errorf("read EVT sequence %d: %w", seq, err)
		}

		if strings.HasPrefix(msg.Subject, strings.TrimSuffix(evtstream.RBACSubjectFilter(), ">")) {
			// RBAC changes can revoke visibility without producing a room event.
			// Rebuild from current authorized state rather than risk retaining a
			// resource that the viewer may no longer read.
			plan.Reset = true
			plan.Events = nil
			return plan, nil
		}
		if realtimeReplayRequiresReset(msg.Subject) {
			plan.Reset = true
			plan.Events = nil
			return plan, nil
		}

		var event evtv1.Event
		if err := proto.Unmarshal(msg.Data, &event); err != nil {
			return RealtimeReplayPlan{}, fmt.Errorf("decode EVT sequence %d: %w", seq, err)
		}
		if evtstream.EventTypeOf(&event) != liveEventType(msg.Subject) {
			// Protobuf preserves an unknown future oneof field as unknown bytes,
			// which leaves Event unset on this older server. The subject can also
			// disagree with a known payload after damaged or invalid publication.
			// In both cases only a current exact snapshot is safe.
			plan.Reset = true
			plan.Events = nil
			return plan, nil
		}
		if event.GetUserKeyShreddingRequested() != nil || event.GetUserKeyShredded() != nil {
			// Key shredding can tombstone messages across many retained rooms.
			// A reset purges every cached plaintext row in one ordered operation.
			plan.Reset = true
			plan.Events = nil
			return plan, nil
		}
		roomID, roomSubject := realtimeReplayRoomSubject(msg.Subject)
		assetID, assetSubject := evtstream.ParseAssetSubject(msg.Subject)
		userIDFromSubject, userSubject := evtstream.ParseUserSubject(msg.Subject)
		configSubjectID, configSubject := liveEVTConfigSubjectID(msg.Subject)
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
				// A visibility-closing fact for this viewer is safe and necessary:
				// it removes state that the client could have retained before the
				// gap. Other visibility changes need a new snapshot because current
				// state cannot prove that this viewer saw the earlier room.
				if eventClosesUserRoomVisibility(&event, userID) {
					// Continue and deliver the closing fact.
				} else if eventChangesRoomVisibility(&event) || isRoomDirectoryProjectionEvent(&event) {
					plan.Reset = true
					plan.Events = nil
					return plan, nil
				} else {
					continue
				}
			}
			waitCtx, cancel := context.WithTimeout(ctx, liveEVTProjectionWaitTimeout)
			err = c.myEventsModel.waitForLiveEVTRoomEvent(waitCtx, msg.Subject, &event, seq)
			cancel()
			if err != nil {
				return RealtimeReplayPlan{}, fmt.Errorf("wait for replay sequence %d: %w", seq, err)
			}
			if legacyAssetEvent {
				assetRoomID, ok := c.assetModel.AssetRoomID(assetIDOfLifecycleEvent(&event))
				if !ok || assetRoomID != roomID {
					continue
				}
			}
		case assetSubject:
			if assetIDOfLifecycleEvent(&event) != assetID || !isDeliverableLiveEVTAssetEvent(&event) {
				continue
			}
			waitCtx, cancel := context.WithTimeout(ctx, liveEVTProjectionWaitTimeout)
			err = c.myEventsModel.waitForLiveEVTAssetEvent(waitCtx, msg.Subject, &event, seq)
			cancel()
			if err != nil {
				return RealtimeReplayPlan{}, fmt.Errorf("wait for replay sequence %d: %w", seq, err)
			}
			assetRoomID, ok := c.assetModel.AssetRoomID(assetID)
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
			if userIDOfUserEvent(&event) != userIDFromSubject {
				plan.Reset = true
				plan.Events = nil
				return plan, nil
			}
			waitCtx, cancel := context.WithTimeout(ctx, liveEVTProjectionWaitTimeout)
			err = c.myEventsModel.waitForLiveEVTUserEvent(waitCtx, msg.Subject, seq)
			cancel()
			if err != nil {
				return RealtimeReplayPlan{}, fmt.Errorf("wait for replay sequence %d: %w", seq, err)
			}
			if event.GetUserServerPreferencesChanged() != nil {
				waitCtx, cancel := context.WithTimeout(ctx, liveEVTProjectionWaitTimeout)
				err = c.configModel.waitFor(waitCtx, events.SubjectPosition(msg.Subject, seq))
				cancel()
				if err != nil {
					return RealtimeReplayPlan{}, fmt.Errorf("wait for replay legacy preference sequence %d: %w", seq, err)
				}
			}
		case configSubject:
			if configSubjectID == evtstream.ConfigSingletonID {
				if !isDeliverableLiveEVTServerConfigEvent(&event) {
					if isKnownNonSnapshotServerConfigEventType(evtstream.EventTypeOf(&event)) {
						continue
					}
					plan.Reset = true
					plan.Events = nil
					return plan, nil
				}
			} else {
				if !isDeliverableLiveEVTUserConfigEvent(&event) {
					continue
				}
				if userIDOfUserConfigEvent(&event) != configSubjectID {
					plan.Reset = true
					plan.Events = nil
					return plan, nil
				}
			}
			waitCtx, cancel := context.WithTimeout(ctx, liveEVTProjectionWaitTimeout)
			err = c.configModel.waitFor(waitCtx, events.SubjectPosition(msg.Subject, seq))
			cancel()
			if err != nil {
				return RealtimeReplayPlan{}, fmt.Errorf("wait for replay config sequence %d: %w", seq, err)
			}
		default:
			if !isKnownNonRealtimeEVTSubject(msg.Subject) {
				// A newer server can introduce an aggregate that contributes to
				// the exact snapshot. Match live delivery and fail closed because
				// this replica cannot classify the fact's state impact.
				plan.Reset = true
				plan.Events = nil
				return plan, nil
			}
			continue
		}
		if protectedRoomID, protected := c.MessageReadProtectedEventRoomID(&event); protected {
			kind, err := c.FindRoomKind(ctx, protectedRoomID)
			if errors.Is(err, ErrNotFound) {
				continue
			}
			if err != nil {
				return RealtimeReplayPlan{}, fmt.Errorf("resolve replay message room %s: %w", protectedRoomID, err)
			}
			canRead, err := c.CanReadMessageEvent(ctx, userID, kind, protectedRoomID, &event)
			if err != nil {
				return RealtimeReplayPlan{}, fmt.Errorf("authorize replay message room %s: %w", protectedRoomID, err)
			}
			if !canRead {
				continue
			}
		}
		plan.Events = append(plan.Events, NewEVTEventEnvelopeWithDeliverySeq(&event, seq))
		if len(plan.Events) > realtimeReplayMaxEvents {
			plan.Reset = true
			plan.Events = nil
			return plan, nil
		}
	}

	return plan, nil
}

// eventClosesUserRoomVisibility reports facts that remove the viewer's own
// room access. These facts are safe to replay after current authorization has
// removed the room because they only delete state the client might retain.
// Opening and room-wide visibility facts need snapshot fallback when current
// state cannot prove that the viewer may see the room.
func eventClosesUserRoomVisibility(event *evtv1.Event, userID string) bool {
	if event == nil || userID == "" {
		return false
	}
	switch payload := event.Event.(type) {
	case *evtv1.Event_UserLeftRoom:
		return event.GetActorId() == userID
	case *evtv1.Event_RoomMemberRemoved:
		return payload.RoomMemberRemoved.GetUserId() == userID
	case *evtv1.Event_RoomMemberBanned:
		return payload.RoomMemberBanned.GetUserId() == userID
	default:
		return false
	}
}

func realtimeReplayRequiresReset(subject string) bool {
	parts := strings.Split(subject, ".")
	if len(parts) < 2 || parts[0] != strings.TrimSuffix(evtstream.SubjectRoot, ".") {
		return false
	}
	switch parts[1] {
	case evtstream.AggregateGroup, evtstream.AggregateLayout:
		return true
	default:
		return false
	}
}

func realtimeReplayRoomSubject(subject string) (string, bool) {
	parts := strings.Split(subject, ".")
	if len(parts) != 4 || parts[0] != "evt" || parts[1] != evtstream.AggregateRoom || parts[2] == "" || parts[3] == "" {
		return "", false
	}
	return parts[2], true
}

func (c *ChattoCore) encodeRealtimeCursor(userID, streamIdentity string, sequence uint64) (string, error) {
	return c.encodeRealtimeCursorAt(userID, streamIdentity, sequence, time.Now())
}

func (c *ChattoCore) encodeRealtimeCursorAt(userID, streamIdentity string, sequence uint64, now time.Time) (string, error) {
	if userID == "" || !evtstream.ValidIdentity(streamIdentity) {
		return "", ErrRealtimeCursorInvalid
	}
	claims := realtimeCursorClaims{
		Position: c.realtimeCursorPosition(streamIdentity, userID, sequence),
		Version:  realtimeCursorVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Audience:  jwt.ClaimStrings{realtimeCursorAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(realtimeCursorLifetime)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(c.realtimeCursorSigningKey())
	if err != nil {
		return "", fmt.Errorf("sign realtime cursor: %w", err)
	}
	return token, nil
}

func (c *ChattoCore) realtimeCursorSigningKey() []byte {
	mac := hmac.New(sha256.New, []byte(c.config.SecretKey))
	_, _ = mac.Write([]byte(realtimeCursorPurpose + "\x00jwt"))
	return mac.Sum(nil)
}

func (c *ChattoCore) realtimeCursorPosition(streamIdentity, userID string, sequence uint64) string {
	mac := hmac.New(sha256.New, []byte(c.config.SecretKey))
	_, _ = mac.Write([]byte(realtimeCursorPurpose))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(realtimeCursorScope))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(streamIdentity))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(userID))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(strconv.FormatUint(sequence, 10)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (c *ChattoCore) parseRealtimeCursorAt(userID, cursor string, now time.Time) (realtimeCursorClaims, error) {
	claims := realtimeCursorClaims{}
	token, err := jwt.ParseWithClaims(
		cursor,
		&claims,
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, ErrRealtimeCursorInvalid
			}
			return c.realtimeCursorSigningKey(), nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithAudience(realtimeCursorAudience),
		jwt.WithSubject(userID),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(func() time.Time { return now }),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return realtimeCursorClaims{}, ErrRealtimeCursorExpired
		}
		return realtimeCursorClaims{}, ErrRealtimeCursorInvalid
	}
	if !token.Valid || claims.Version != realtimeCursorVersion || claims.Position == "" || claims.IssuedAt == nil || claims.ExpiresAt == nil {
		return realtimeCursorClaims{}, ErrRealtimeCursorInvalid
	}
	position, err := base64.RawURLEncoding.DecodeString(claims.Position)
	if err != nil || len(position) != sha256.Size {
		return realtimeCursorClaims{}, ErrRealtimeCursorInvalid
	}
	if claims.IssuedAt.Time.After(now.Add(realtimeCursorFutureSkew)) || claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time) != realtimeCursorLifetime {
		return realtimeCursorClaims{}, ErrRealtimeCursorInvalid
	}
	return claims, nil
}

// resolveRealtimeCursorAt recovers the hidden EVT position by comparing the
// token's HMAC against the bounded sequence window. No stream coordinate is
// present in the JWT payload.
func (c *ChattoCore) resolveRealtimeCursorAt(
	userID, cursor, streamIdentity string,
	firstSequence, lastSequence uint64,
	now time.Time,
) (uint64, error) {
	claims, err := c.parseRealtimeCursorAt(userID, cursor, now)
	if err != nil {
		return 0, err
	}
	lower := uint64(0)
	if firstSequence > 0 {
		lower = firstSequence - 1
	}
	if lastSequence > realtimeReplayMaxSequenceSpan && lower < lastSequence-realtimeReplayMaxSequenceSpan {
		lower = lastSequence - realtimeReplayMaxSequenceSpan
	}
	for sequence := lastSequence; ; sequence-- {
		if hmac.Equal([]byte(claims.Position), []byte(c.realtimeCursorPosition(streamIdentity, userID, sequence))) {
			return sequence, nil
		}
		if sequence == lower {
			break
		}
	}
	return 0, ErrRealtimeCursorExpired
}
