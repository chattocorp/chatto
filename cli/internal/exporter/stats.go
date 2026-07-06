package exporter

import (
	"sort"
	"sync"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type evtStats struct {
	mu sync.RWMutex

	users map[string]userState
	rooms map[string]corev1.RoomKind

	messages map[string]messageKind
	assets   map[string]assetState

	lastSeq        uint64
	replayComplete bool
}

type userState struct {
	verifiedEmail bool
}

type messageKind string

const (
	messageKindRoot   messageKind = "root"
	messageKindThread messageKind = "thread"
	messageKindEcho   messageKind = "echo"
)

type assetState struct {
	backend string
	kind    string
	deleted bool
}

type statsSnapshot struct {
	Users          map[string]int
	Presence       map[string]int
	Rooms          map[string]int
	Messages       map[string]int
	Assets         map[string]int
	LastSeq        uint64
	ReplayComplete bool
}

func newEVTStats() *evtStats {
	return &evtStats{
		users:    make(map[string]userState),
		rooms:    make(map[string]corev1.RoomKind),
		messages: make(map[string]messageKind),
		assets:   make(map[string]assetState),
	}
}

func (s *evtStats) apply(event *corev1.Event, seq uint64) {
	if event == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if seq > s.lastSeq {
		s.lastSeq = seq
	}

	switch e := event.GetEvent().(type) {
	case *corev1.Event_UserAccountCreated:
		if userID := e.UserAccountCreated.GetUserId(); userID != "" {
			state := s.users[userID]
			s.users[userID] = state
		}
	case *corev1.Event_UserVerifiedEmailAdded:
		if userID := e.UserVerifiedEmailAdded.GetUserId(); userID != "" {
			if state, ok := s.users[userID]; ok {
				state.verifiedEmail = true
				s.users[userID] = state
			}
		}
	case *corev1.Event_UserAccountDeleted:
		if userID := e.UserAccountDeleted.GetUserId(); userID != "" {
			delete(s.users, userID)
		}
	case *corev1.Event_RoomCreated:
		roomID := e.RoomCreated.GetRoomId()
		if roomID != "" {
			s.rooms[roomID] = e.RoomCreated.GetKind()
		}
	case *corev1.Event_RoomDeleted:
		roomID := e.RoomDeleted.GetRoomId()
		if roomID != "" {
			delete(s.rooms, roomID)
		}
	case *corev1.Event_MessagePosted:
		eventID := event.GetId()
		if eventID != "" {
			s.messages[eventID] = classifyMessage(e.MessagePosted)
		}
	case *corev1.Event_MessageRetracted:
		// Keep posted-message counters as lifetime totals. Retractions are
		// durable facts, but subtracting them would hide write volume.
	case *corev1.Event_AssetCreated:
		asset := e.AssetCreated.GetAsset()
		if asset == nil || asset.GetId() == "" {
			return
		}
		s.assets[asset.GetId()] = assetState{
			backend: assetBackend(asset),
			kind:    assetKind(e.AssetCreated),
		}
	case *corev1.Event_AssetDeleted:
		assetID := e.AssetDeleted.GetAssetId()
		if assetID == "" {
			return
		}
		state := s.assets[assetID]
		if state.backend == "" {
			state.backend = "unknown"
		}
		if state.kind == "" {
			state.kind = "unknown"
		}
		state.deleted = true
		s.assets[assetID] = state
	}
}

func (s *evtStats) markReplayComplete() {
	s.mu.Lock()
	s.replayComplete = true
	s.mu.Unlock()
}

func (s *evtStats) snapshot(presence map[string]int) statsSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := map[string]int{"verified": 0, "unverified": 0}
	for _, user := range s.users {
		if user.verifiedEmail {
			users["verified"]++
		} else {
			users["unverified"]++
		}
	}

	rooms := map[string]int{"channel": 0, "dm": 0}
	for _, kind := range s.rooms {
		rooms[roomKindLabel(kind)]++
	}

	messages := map[string]int{
		string(messageKindRoot):   0,
		string(messageKindThread): 0,
		string(messageKindEcho):   0,
	}
	for _, kind := range s.messages {
		messages[string(kind)]++
	}

	assets := map[string]int{}
	for _, state := range s.assets {
		backend := state.backend
		if backend == "" {
			backend = "unknown"
		}
		lifecycle := "active"
		if state.deleted {
			lifecycle = "deleted"
		}
		kind := state.kind
		if kind == "" {
			kind = "unknown"
		}
		assets[backend+"|"+lifecycle+"|"+kind]++
	}

	presenceCopy := make(map[string]int, len(presence))
	for status, count := range presence {
		presenceCopy[status] = count
	}

	return statsSnapshot{
		Users:          users,
		Presence:       presenceCopy,
		Rooms:          rooms,
		Messages:       messages,
		Assets:         assets,
		LastSeq:        s.lastSeq,
		ReplayComplete: s.replayComplete,
	}
}

func classifyMessage(event *corev1.MessagePostedEvent) messageKind {
	if event == nil {
		return messageKindRoot
	}
	if event.GetEchoOfEventId() != "" {
		return messageKindEcho
	}
	if event.GetInThread() != "" || event.GetInReplyTo() != "" {
		return messageKindThread
	}
	return messageKindRoot
}

func assetBackend(asset *corev1.AssetRecord) string {
	switch asset.GetStorage().(type) {
	case *corev1.AssetRecord_Nats:
		return "nats"
	case *corev1.AssetRecord_S3:
		return "s3"
	default:
		return "unknown"
	}
}

func assetKind(event *corev1.AssetCreatedEvent) string {
	if event == nil || event.GetParentAssetId() == "" {
		return "original"
	}
	switch event.GetDerivativeRole() {
	case corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_THUMBNAIL:
		return "thumbnail"
	case corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_VIDEO_VARIANT:
		return "video_variant"
	default:
		return "derivative"
	}
}

func roomKindLabel(kind corev1.RoomKind) string {
	switch kind {
	case corev1.RoomKind_ROOM_KIND_DM:
		return "dm"
	default:
		return "channel"
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
