package core

import (
	"context"
	"sort"
)

// InteractionUserPage is a bounded set of user references. TotalCount counts
// relationships before pagination; clients can hydrate profiles separately.
type InteractionUserPage struct {
	UserIDs    []string
	TotalCount int
	HasMore    bool
}

func interactionUserPage(ids []string, limit, offset int) *InteractionUserPage {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	sort.Strings(ids)
	page, total, more := paginateCoreSlice(ids, limit, offset)
	return &InteractionUserPage{UserIDs: page, TotalCount: total, HasMore: more}
}

// ListReactionUsers returns complete reaction membership under the same read
// authorization as the message. A channel echo is an alias for its reply.
func (s *RoomTimelineReadModel) ListReactionUsers(ctx context.Context, actorID, roomID, eventID, emoji string, limit, offset int) (*InteractionUserPage, error) {
	if _, _, err := s.core.requireMessageReader(ctx, actorID, roomID, eventID); err != nil {
		return nil, err
	}
	canonical, err := s.core.canonicalReactionMessageEventID(roomID, eventID)
	if err != nil {
		return nil, err
	}
	if _, err := s.timelineMessageEntry(roomID, canonical, false); err != nil {
		return nil, err
	}
	name, err := resolveEmojiInput(emoji)
	if err != nil {
		return nil, err
	}
	if err := s.core.roomModel.waitForReactionsCurrent(ctx, s.core.EventPublisher, roomID); err != nil {
		return nil, err
	}
	for _, reaction := range s.core.roomModel.reactionsForMessage(canonical) {
		if reaction.Emoji == name {
			return interactionUserPage(append([]string(nil), reaction.UserIDs...), limit, offset), nil
		}
	}
	return interactionUserPage(nil, limit, offset), nil
}

// ListThreadParticipants returns distinct authors of current visible replies.
// It excludes the root author unless they replied, matching thread summaries.
func (s *RoomTimelineReadModel) ListThreadParticipants(ctx context.Context, actorID, roomID, rootID string, limit, offset int) (*InteractionUserPage, error) {
	_, kind, err := s.core.requireThreadMessageReader(ctx, actorID, roomID, rootID)
	if err != nil {
		return nil, err
	}
	if _, err := s.core.requireThreadRoot(ctx, kind, roomID, rootID); err != nil {
		return nil, err
	}
	if _, err := s.timelineMessageEntry(roomID, rootID, false); err != nil {
		return nil, err
	}
	ids, err := s.core.roomModel.threadParticipantIDs(ctx, rootID)
	if err != nil {
		return nil, err
	}
	return interactionUserPage(ids, limit, offset), nil
}
