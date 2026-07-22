package core

import (
	"context"
	"errors"
	"slices"
	"strings"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// MessageSearchReads returns the operation-level model that resolves a
// viewer's provider query scope and re-authorizes provider hits against current
// room membership and message state.
func (c *ChattoCore) MessageSearchReads() *MessageSearchReadModel {
	return c.messageSearchReads
}

// MessageSearchReadModel keeps the full-text provider outside Chatto's
// authorization boundary. Provider hits are candidates, never readable facts.
type MessageSearchReadModel struct {
	core *ChattoCore
}

// MessageSearchScopeInput combines explicit public request filters with
// selectors parsed from the canonical query language.
type MessageSearchScopeInput struct {
	ActorID         string
	RoomID          string
	RoomSelectors   []string
	AuthorID        string
	AuthorSelectors []string
}

// MessageSearchScope contains stable provider filters and the current room
// objects used to reject provider hits outside the authorized candidate set.
type MessageSearchScope struct {
	RoomIDs   []string
	AuthorIDs []string
	NoMatches bool
	rooms     map[string]*corev1.Room
}

// MessageSearchHit is one untrusted provider candidate.
type MessageSearchHit struct {
	MessageID   string
	RoomID      string
	BodyEventID string
}

// MessageSearchResult is one current message that survived authorization and
// current-state hydration.
type MessageSearchResult struct {
	Kind  RoomKind
	Event *corev1.Event
}

// ResolveScope returns only rooms the actor is currently an effective member
// of. Archived rooms remain eligible because membership still grants history
// reads, while DM visibility is always membership-only.
func (s *MessageSearchReadModel) ResolveScope(ctx context.Context, input MessageSearchScopeInput) (*MessageSearchScope, error) {
	if err := requireAuthenticatedActor(input.ActorID); err != nil {
		return nil, err
	}
	rooms := make(map[string]*corev1.Room)
	for _, kind := range []RoomKind{KindChannel, KindDM} {
		memberRooms, err := s.core.ListMemberRooms(ctx, kind, input.ActorID, MemberRoomListOptions{})
		if err != nil {
			return nil, err
		}
		for _, room := range memberRooms {
			if room != nil {
				rooms[room.GetId()] = room
			}
		}
	}

	rooms = filterSearchRoomByID(rooms, input.RoomID)
	rooms = filterSearchRoomsBySelectors(rooms, input.RoomSelectors)
	roomIDs := make([]string, 0, len(rooms))
	for roomID := range rooms {
		roomIDs = append(roomIDs, roomID)
	}
	slices.Sort(roomIDs)

	var authorIDs []string
	if input.AuthorID != "" {
		authorIDs = []string{input.AuthorID}
	}
	noMatches := false
	if len(input.AuthorSelectors) > 0 {
		selectorIDs, err := s.resolveAuthorSelectors(ctx, input.AuthorSelectors)
		if err != nil {
			return nil, err
		}
		if len(authorIDs) == 0 {
			authorIDs = selectorIDs
		} else {
			authorIDs = intersectSortedStrings(authorIDs, selectorIDs)
		}
		noMatches = len(authorIDs) == 0
	}
	return &MessageSearchScope{RoomIDs: roomIDs, AuthorIDs: authorIDs, NoMatches: noMatches, rooms: rooms}, nil
}

// HydrateHits preserves provider order while omitting duplicate, inaccessible,
// stale, retracted, crypto-shredded, wrong-room, and non-message candidates.
func (s *MessageSearchReadModel) HydrateHits(ctx context.Context, actorID string, scope *MessageSearchScope, hits []MessageSearchHit) ([]MessageSearchResult, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	if scope == nil || scope.NoMatches || len(scope.RoomIDs) == 0 || len(hits) == 0 {
		return nil, nil
	}

	byRoom := make(map[string][]string)
	for _, hit := range hits {
		if hit.MessageID == "" || scope.rooms[hit.RoomID] == nil {
			continue
		}
		byRoom[hit.RoomID] = append(byRoom[hit.RoomID], hit.MessageID)
	}

	type hydrated struct {
		kind  RoomKind
		event *corev1.Event
	}
	current := make(map[string]hydrated)
	for roomID, messageIDs := range byRoom {
		read, err := s.core.RoomTimelineReads().BatchGetMessages(ctx, actorID, roomID, messageIDs)
		if err != nil {
			if errors.Is(err, ErrNotRoomMember) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrPermissionDenied) {
				continue
			}
			return nil, err
		}
		for _, event := range read.Events {
			if event != nil {
				current[searchHitKey(roomID, event.GetId())] = hydrated{kind: read.Kind, event: event}
			}
		}
	}

	seen := make(map[string]struct{}, len(hits))
	results := make([]MessageSearchResult, 0, len(hits))
	for _, hit := range hits {
		key := searchHitKey(hit.RoomID, hit.MessageID)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		candidate, ok := current[key]
		if !ok {
			continue
		}
		body, retracted, bodyKnown := s.core.roomModel.latestBody(hit.MessageID)
		if !bodyKnown || retracted || body == nil || body.GetBodyEventId() != hit.BodyEventID {
			continue
		}
		seen[key] = struct{}{}
		results = append(results, MessageSearchResult{Kind: candidate.kind, Event: candidate.event})
	}
	return results, nil
}

func (s *MessageSearchReadModel) resolveAuthorSelectors(ctx context.Context, selectors []string) ([]string, error) {
	ids := make([]string, 0, len(selectors))
	for _, selector := range selectors {
		user, err := s.core.GetUser(ctx, selector)
		if err != nil && errors.Is(err, ErrNotFound) {
			user, err = s.core.GetUserByLogin(ctx, selector)
		}
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		if user != nil && !user.GetDeleted() {
			ids = append(ids, user.GetId())
		}
	}
	return uniqueSortedStrings(ids), nil
}

func filterSearchRoomByID(rooms map[string]*corev1.Room, requested string) map[string]*corev1.Room {
	if requested == "" {
		return rooms
	}
	result := make(map[string]*corev1.Room)
	if room := rooms[requested]; room != nil {
		result[requested] = room
	}
	return result
}

func filterSearchRoomsBySelectors(rooms map[string]*corev1.Room, selectors []string) map[string]*corev1.Room {
	if len(selectors) == 0 {
		return rooms
	}
	result := make(map[string]*corev1.Room)
	for _, selector := range selectors {
		for roomID, room := range rooms {
			if roomID == selector || (room.GetName() != "" && strings.EqualFold(room.GetName(), selector)) {
				result[roomID] = room
			}
		}
	}
	return result
}

func uniqueSortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	slices.Sort(result)
	return slices.Compact(result)
}

func intersectSortedStrings(left, right []string) []string {
	rightSet := make(map[string]struct{}, len(right))
	for _, value := range right {
		rightSet[value] = struct{}{}
	}
	result := make([]string, 0, len(left))
	for _, value := range left {
		if _, ok := rightSet[value]; ok {
			result = append(result, value)
		}
	}
	return result
}

func searchHitKey(roomID, messageID string) string {
	return roomID + "\x00" + messageID
}
