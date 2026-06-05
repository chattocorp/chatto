package graph

import (
	"context"
	"errors"
	"fmt"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// unwrapEvent extracts the concrete GraphQL payload from the delivered event
// envelope. Returns nil for an empty envelope or an unknown variant.
func unwrapEvent(event core.EventEnvelope) any {
	if event == nil {
		return nil
	}
	if evt := event.EVTEvent(); evt != nil {
		return unwrapEVTEvent(evt)
	}
	if live := event.LiveEvent(); live != nil {
		return unwrapLiveEvent(live)
	}
	if heartbeat := event.HeartbeatEvent(); heartbeat != nil {
		return heartbeat
	}
	return nil
}

func unwrapEVTEvent(event *corev1.Event) any {
	if event == nil || event.Event == nil {
		return nil
	}

	switch e := event.Event.(type) {
	// ---- Room lifecycle ----
	case *corev1.Event_RoomCreated:
		return e.RoomCreated
	case *corev1.Event_RoomUpdated:
		return e.RoomUpdated
	case *corev1.Event_RoomDeleted:
		return e.RoomDeleted
	case *corev1.Event_RoomArchived:
		return e.RoomArchived
	case *corev1.Event_RoomUnarchived:
		return e.RoomUnarchived

	// ---- Room membership ----
	case *corev1.Event_UserJoinedRoom:
		return e.UserJoinedRoom
	case *corev1.Event_UserLeftRoom:
		return e.UserLeftRoom
	case *corev1.Event_SpaceMemberDeleted:
		return e.SpaceMemberDeleted

	// ---- Messages ----
	case *corev1.Event_MessagePosted:
		return &model.MessagePostedEvent{Envelope: event, Payload: e.MessagePosted, RoomID: e.MessagePosted.GetRoomId()}
	case *corev1.Event_MessageEdited:
		return e.MessageEdited
	case *corev1.Event_MessageRetracted:
		return e.MessageRetracted

	// ---- Threads ----
	case *corev1.Event_ThreadCreated:
		return e.ThreadCreated

	// ---- Assets ----
	case *corev1.Event_AssetCreated:
		return e.AssetCreated
	case *corev1.Event_AssetDeleted:
		return e.AssetDeleted

	// ---- Reactions ----
	case *corev1.Event_ReactionAdded:
		return e.ReactionAdded
	case *corev1.Event_ReactionRemoved:
		return e.ReactionRemoved

	case *corev1.Event_AssetProcessingStarted:
		return e.AssetProcessingStarted
	case *corev1.Event_AssetProcessingSucceeded:
		return e.AssetProcessingSucceeded
	case *corev1.Event_AssetProcessingFailed:
		return e.AssetProcessingFailed

	default:
		return nil
	}
}

func unwrapLiveEvent(event *corev1.LiveEvent) any {
	if event == nil || event.Event == nil {
		return nil
	}

	switch e := event.Event.(type) {
	case *corev1.LiveEvent_ConfigUpdated:
		return e.ConfigUpdated
	case *corev1.LiveEvent_UserCreated:
		return e.UserCreated
	case *corev1.LiveEvent_UserDeleted:
		return e.UserDeleted
	case *corev1.LiveEvent_UserProfileUpdated:
		return e.UserProfileUpdated
	case *corev1.LiveEvent_ServerUserPreferencesUpdated:
		return e.ServerUserPreferencesUpdated
	case *corev1.LiveEvent_NotificationLevelChanged:
		return e.NotificationLevelChanged
	case *corev1.LiveEvent_ThreadFollowChanged:
		return e.ThreadFollowChanged
	case *corev1.LiveEvent_SpaceMemberDeleted:
		return e.SpaceMemberDeleted
	case *corev1.LiveEvent_ServerUpdated:
		return e.ServerUpdated
	case *corev1.LiveEvent_UserTyping:
		return e.UserTyping
	case *corev1.LiveEvent_VideoProcessingCompleted:
		return e.VideoProcessingCompleted
	case *corev1.LiveEvent_PresenceChanged:
		return e.PresenceChanged
	case *corev1.LiveEvent_MentionNotification:
		return e.MentionNotification
	case *corev1.LiveEvent_NewDirectMessageNotification:
		return e.NewDirectMessageNotification
	case *corev1.LiveEvent_CallParticipantJoined:
		return e.CallParticipantJoined
	case *corev1.LiveEvent_CallParticipantLeft:
		return e.CallParticipantLeft
	case *corev1.LiveEvent_NotificationCreated:
		return e.NotificationCreated
	case *corev1.LiveEvent_NotificationDismissed:
		return e.NotificationDismissed
	case *corev1.LiveEvent_RoomMarkedAsRead:
		return e.RoomMarkedAsRead
	case *corev1.LiveEvent_MentionStatusCleared:
		return e.MentionStatusCleared
	case *corev1.LiveEvent_RoomGroupsUpdated:
		return e.RoomGroupsUpdated
	case *corev1.LiveEvent_SessionTerminated:
		return e.SessionTerminated
	default:
		return nil
	}
}

// resolveEventActor loads the actor User for an event envelope.
// Returns nil (without error) for system-authored events (empty ActorId)
// and for actors whose accounts have been deleted.
func (r *Resolver) resolveEventActor(ctx context.Context, event core.EventEnvelope) (*corev1.User, error) {
	if event == nil {
		return nil, nil
	}
	actorID := event.ActorID()
	if actorID == "" {
		return nil, nil
	}
	user, err := r.getUser(ctx, actorID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

// assetCreationForProcessing looks up the AssetCreatedEvent referenced by an
// asset-processing event. Returns nil when the asset is unknown to the
// projection (e.g. after AssetDeletedEvent has dropped it). Callers must
// tolerate the nil and return empty fields rather than errors — these events
// surface through non-null GraphQL fields, so a returned error would
// propagate up and blank the entire room.events list.
func (r *Resolver) assetCreationForProcessing(assetID string) *corev1.AssetCreatedEvent {
	if assetID == "" {
		return nil
	}
	declared, ok := r.core.RoomTimeline.AssetCreation(assetID)
	if !ok {
		return nil
	}
	return declared
}

func (r *attachmentResolver) assetSourceAvailable(assetID string, fallback bool) bool {
	created, ok := r.core.RoomTimeline.AssetCreation(assetID)
	if !ok || created == nil {
		return fallback
	}
	return created.GetOriginalBinaryAvailable()
}

func assetCreatedRoomID(event *corev1.AssetCreatedEvent) string {
	if event == nil {
		return ""
	}
	return event.GetRoomId()
}

func assetDimensions(asset *corev1.AssetRecord) (int32, int32) {
	if asset == nil {
		return 0, 0
	}
	return asset.GetWidth(), asset.GetHeight()
}

func assetProcessingFailureReasonCode(code corev1.AssetProcessingFailureCode) string {
	switch code {
	case corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_SOURCE_MISSING:
		return "original_missing"
	case corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_PROCESSING_FAILED:
		return "processing_failed"
	default:
		return "processing_failed"
	}
}

// unwrapEventAs unwraps a delivered event and asserts the payload to the
// requested GraphQL union interface (model.EventType). Returns a typed error
// for nil or unknown payloads.
func unwrapEventAs[T any](event core.EventEnvelope, unionName string) (T, error) {
	var zero T
	unwrapped := unwrapEvent(event)
	if unwrapped == nil {
		return zero, fmt.Errorf("unknown event variant")
	}
	typed, ok := unwrapped.(T)
	if !ok {
		return zero, fmt.Errorf("event does not implement %s: %T", unionName, unwrapped)
	}
	return typed, nil
}
