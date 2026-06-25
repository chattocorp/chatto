package core

import "context"

// ThreadFollows returns the operation-level service for user-facing thread
// follow state changes.
func (c *ChattoCore) ThreadFollows() *ThreadFollowService {
	return c.threadFollows
}

// ThreadFollowService owns public thread follow/unfollow mutations. It keeps
// membership and thread-root validation alongside the operation, while the
// lower-level KV helpers remain available for trusted/internal call sites.
type ThreadFollowService struct {
	core *ChattoCore
}

func (s *ThreadFollowService) FollowThread(ctx context.Context, actorID, roomID, threadRootEventID string) error {
	room, kind, err := s.core.requireRoomMember(ctx, actorID, roomID)
	if err != nil {
		return err
	}
	if _, err := s.core.requireThreadRoot(ctx, kind, room.Id, threadRootEventID); err != nil {
		return err
	}
	return s.core.FollowThread(ctx, kind, actorID, room.Id, threadRootEventID)
}

func (s *ThreadFollowService) UnfollowThread(ctx context.Context, actorID, roomID, threadRootEventID string) error {
	room, kind, err := s.core.requireRoomMember(ctx, actorID, roomID)
	if err != nil {
		return err
	}
	if _, err := s.core.requireThreadRoot(ctx, kind, room.Id, threadRootEventID); err != nil {
		return err
	}
	return s.core.UnfollowThread(ctx, kind, actorID, room.Id, threadRootEventID)
}
