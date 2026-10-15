package core

import (
	"context"
	"fmt"
)

// ThreadRootAuthorFilter selects followed threads by their root author.
type ThreadRootAuthorFilter int

const (
	// ThreadRootAuthorAll preserves the complete followed-thread feed.
	ThreadRootAuthorAll ThreadRootAuthorFilter = iota
	// ThreadRootAuthorViewer selects roots created by the viewer.
	ThreadRootAuthorViewer
	// ThreadRootAuthorOthers selects roots created by other users.
	ThreadRootAuthorOthers
)

// Validate rejects unknown filters rather than silently returning another feed.
func (f ThreadRootAuthorFilter) Validate() error {
	if f < ThreadRootAuthorAll || f > ThreadRootAuthorOthers {
		return fmt.Errorf("%w: unknown thread root author filter", ErrInvalidArgument)
	}
	return nil
}

// matchesRootAuthor reads the root's authoritative actor, independently of
// matching replies and participant previews. Missing roots cannot be classified.
func (s *ThreadFollowModel) matchesRootAuthor(ctx context.Context, actorID string, kind RoomKind, roomID, rootID string, filter ThreadRootAuthorFilter) (bool, error) {
	if filter == ThreadRootAuthorAll {
		return true, nil
	}
	root, err := s.integration.GetRoomEventByEventID(ctx, kind, roomID, rootID)
	if err != nil {
		return false, err
	}
	if root == nil || root.GetMessagePosted() == nil || root.GetActorId() == "" {
		return false, nil
	}
	return (root.GetActorId() == actorID) == (filter == ThreadRootAuthorViewer), nil
}
