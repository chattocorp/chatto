package core

import "context"

// ThreadFollows returns the operation-level model for user-facing thread
// follow state changes.
func (c *ChattoCore) ThreadFollows() *ThreadFollowModel {
	return c.threadFollows
}

// ThreadFollowModel owns public thread follow/unfollow mutations. It keeps
// membership and thread-root validation alongside the operation, while the
// lower-level KV helpers remain available for trusted/internal call sites.
type ThreadFollowModel struct {
	core *ChattoCore
}

func (s *ThreadFollowModel) ListFollowedThreads(ctx context.Context, actorID string, limit, offset int) (*FollowedThreadsPage, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	return s.core.ListFollowedThreadsPage(ctx, actorID, []string{LegacySpaceIDForRoomKind(KindChannel)}, limit, offset)
}

func (s *ThreadFollowModel) HasUnreadFollowedThreads(ctx context.Context, actorID string) (bool, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return false, err
	}
	return s.core.HasUnreadFollowedThreads(ctx, actorID, []string{LegacySpaceIDForRoomKind(KindChannel)})
}

func (s *ThreadFollowModel) FollowThread(ctx context.Context, actorID, roomID, threadRootEventID string) error {
	room, kind, err := s.core.requireRoomMember(ctx, actorID, roomID)
	if err != nil {
		return err
	}
	if _, err := s.core.requireThreadRoot(ctx, kind, room.Id, threadRootEventID); err != nil {
		return err
	}
	return s.core.FollowThread(ctx, kind, actorID, room.Id, threadRootEventID)
}

func (s *ThreadFollowModel) UnfollowThread(ctx context.Context, actorID, roomID, threadRootEventID string) error {
	room, kind, err := s.core.requireRoomMember(ctx, actorID, roomID)
	if err != nil {
		return err
	}
	if _, err := s.core.requireThreadRoot(ctx, kind, room.Id, threadRootEventID); err != nil {
		return err
	}
	return s.core.UnfollowThread(ctx, kind, actorID, room.Id, threadRootEventID)
}
