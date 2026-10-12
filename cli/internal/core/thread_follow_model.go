package core

import (
	"context"

	"github.com/charmbracelet/log"

	"hmans.de/chatto/internal/evtstream"
)

// ThreadFollows returns the operation-level model for user-facing thread
// follow state changes.
func (c *ChattoCore) ThreadFollows() *ThreadFollowModel {
	return c.threadFollows
}

// ThreadFollowModel owns public thread follow/unfollow mutations. It keeps
// membership and thread-root validation alongside durable EVT writes and
// projection waits. Trusted internal callers can also request automatic follows.
type ThreadFollowModel struct {
	rooms        *RoomModel
	memberships  *RoomMembershipModel
	readState    *ReadStateModel
	publisher    *evtstream.Publisher
	policy       *AuthorizationModel
	timeline     threadFollowModelTimeline
	effects      threadFollowModelEffects
	materializer *NotificationMaterializer
	logger       *log.Logger
}

// ListFollowedThreads returns one page of the actor's followed threads. When
// unreadOnly is set, the page and its total include only threads with unread
// replies.
func (s *ThreadFollowModel) ListFollowedThreads(ctx context.Context, actorID string, includeDM, unreadOnly bool, limit, offset int) (*FollowedThreadsPage, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	spaceIDs := []string{LegacySpaceIDForRoomKind(KindChannel)}
	if includeDM {
		spaceIDs = append(spaceIDs, LegacySpaceIDForRoomKind(KindDM))
	}
	return s.ListFollowedThreadsPage(ctx, actorID, spaceIDs, unreadOnly, limit, offset)
}

func (s *ThreadFollowModel) HasUnreadFollowedThreads(ctx context.Context, actorID string) (bool, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return false, err
	}
	return s.hasUnreadFollowedThreads(ctx, actorID, []string{
		LegacySpaceIDForRoomKind(KindChannel),
		LegacySpaceIDForRoomKind(KindDM),
	})
}

func (s *ThreadFollowModel) FollowThread(ctx context.Context, actorID, roomID, threadRootEventID string) error {
	room, kind, err := s.policy.requireThreadMessageReader(ctx, actorID, roomID, threadRootEventID)
	if err != nil {
		return err
	}
	if _, err := s.policy.requireThreadRoot(ctx, kind, room.Id, threadRootEventID); err != nil {
		return err
	}
	return s.followThread(ctx, kind, actorID, room.Id, threadRootEventID)
}

func (s *ThreadFollowModel) UnfollowThread(ctx context.Context, actorID, roomID, threadRootEventID string) error {
	room, kind, err := s.policy.requireThreadMessageReader(ctx, actorID, roomID, threadRootEventID)
	if err != nil {
		return err
	}
	if _, err := s.policy.requireThreadRoot(ctx, kind, room.Id, threadRootEventID); err != nil {
		return err
	}
	return s.unfollowThread(ctx, kind, actorID, room.Id, threadRootEventID)
}
