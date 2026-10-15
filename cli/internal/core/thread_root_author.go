package core

import (
	"context"
	"fmt"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
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

// matchesRootAuthor reads the root's recorded message author, including imported
// authors, from the timeline projection without loading message bodies.
// Missing roots and hidden echoes cannot be classified.
func (s *ThreadFollowModel) matchesRootAuthor(ctx context.Context, actorID string, roomID, rootID string, filter ThreadRootAuthorFilter) (bool, error) {
	if filter == ThreadRootAuthorAll {
		return true, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	root, ok := s.rooms.timelineEntry(rootID)
	if !ok || root.RoomID != roomID || root.EventType != evtstream.EventMessagePosted || root.InThreadEventID != "" || s.rooms.isHiddenEcho(rootID) {
		return false, nil
	}
	authorID := timelineEntryMessageAuthorID(root)
	if authorID == "" {
		return false, nil
	}
	return (authorID == actorID) == (filter == ThreadRootAuthorViewer), nil
}
