package core

import (
	"context"
	"errors"
	"sort"
	"time"
)

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

// ActiveThreadInteraction is one current relationship plus its latest visible
// thread activity time.
type ActiveThreadInteraction struct {
	Interaction    *ThreadInteraction
	LastActivityAt time.Time
}

// ActiveThreadInteractionsPage is one offset page of current relationships.
type ActiveThreadInteractionsPage struct {
	Interactions []*ActiveThreadInteraction
	TotalCount   int
	HasMore      bool
}

// ListInteractions returns the account's current interaction relationships.
// Membership and message.read-interactions are revalidated for every row.
func (s *ThreadFollowModel) ListInteractions(ctx context.Context, actorID string, limit, offset int) (*ActiveThreadInteractionsPage, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	refs := s.core.roomModel.threadInteractionsForUser(actorID)
	interactions := make([]*ActiveThreadInteraction, 0, len(refs))
	for _, ref := range refs {
		active, err := s.activeInteraction(ctx, actorID, ref)
		if err != nil {
			if errors.Is(err, ErrNotFound) || errors.Is(err, ErrNotRoomMember) || errors.Is(err, ErrPermissionDenied) {
				continue
			}
			return nil, err
		}
		interactions = append(interactions, active)
	}
	sort.Slice(interactions, func(i, j int) bool {
		if !interactions[i].LastActivityAt.Equal(interactions[j].LastActivityAt) {
			return interactions[i].LastActivityAt.After(interactions[j].LastActivityAt)
		}
		left := interactions[i].Interaction.RoomID + "\x00" + interactions[i].Interaction.ThreadRootEventID
		right := interactions[j].Interaction.RoomID + "\x00" + interactions[j].Interaction.ThreadRootEventID
		return left < right
	})
	total := len(interactions)
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := total
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return &ActiveThreadInteractionsPage{
		Interactions: interactions[offset:end], TotalCount: total, HasMore: end < total,
	}, nil
}

// GetInteraction returns one current relationship for the account.
func (s *ThreadFollowModel) GetInteraction(ctx context.Context, actorID, roomID, threadRootEventID string) (*ActiveThreadInteraction, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	if err := s.requireInteractionScope(ctx, actorID, roomID); err != nil {
		return nil, err
	}
	interaction, ok := s.core.roomModel.threadInteraction(actorID, roomID, threadRootEventID)
	if !ok {
		return nil, ErrNotFound
	}
	return s.interactionActivity(ctx, interaction)
}

func (s *ThreadFollowModel) activeInteraction(ctx context.Context, actorID string, interaction *ThreadInteraction) (*ActiveThreadInteraction, error) {
	if interaction == nil || interaction.RoomID == "" || interaction.ThreadRootEventID == "" {
		return nil, ErrNotFound
	}
	if err := s.requireInteractionScope(ctx, actorID, interaction.RoomID); err != nil {
		return nil, err
	}
	return s.interactionActivity(ctx, interaction)
}

func (s *ThreadFollowModel) requireInteractionScope(ctx context.Context, actorID, roomID string) error {
	room, err := s.core.FindRoomByID(ctx, roomID)
	if err != nil {
		return err
	}
	if KindOfRoom(room) != KindChannel {
		return ErrNotFound
	}
	member, err := s.core.RoomMembershipExists(ctx, KindChannel, actorID, roomID)
	if err != nil {
		return err
	}
	if !member {
		return ErrNotRoomMember
	}
	allowed, err := s.core.CanReadMessageInteractions(ctx, actorID, KindChannel, roomID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrPermissionDenied
	}
	return nil
}

func (s *ThreadFollowModel) interactionActivity(ctx context.Context, interaction *ThreadInteraction) (*ActiveThreadInteraction, error) {
	latest := time.Time{}
	for _, cause := range interaction.Causes {
		if cause.CreatedAt.After(latest) {
			latest = cause.CreatedAt
		}
	}
	metadata, err := s.core.GetThreadMetadata(ctx, KindChannel, interaction.RoomID, interaction.ThreadRootEventID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if metadata != nil && metadata.LastReplyAt != nil && metadata.LastReplyAt.After(latest) {
		latest = *metadata.LastReplyAt
	}
	return &ActiveThreadInteraction{Interaction: interaction, LastActivityAt: latest}, nil
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

// ListFollowedThreadViewerStates returns an exhaustive, authoritative set for
// realtime replacement semantics. Unlike the user-facing directory list, it
// fails on uncertain rows instead of silently omitting them.
func (s *ThreadFollowModel) ListFollowedThreadViewerStates(ctx context.Context, actorID string) ([]*FollowedThread, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	return s.core.listFollowedThreadViewerStates(ctx, actorID)
}

func (s *ThreadFollowModel) FollowThread(ctx context.Context, actorID, roomID, threadRootEventID string) error {
	room, kind, err := s.core.requireThreadMessageReader(ctx, actorID, roomID, threadRootEventID)
	if err != nil {
		return err
	}
	if _, err := s.core.requireThreadRoot(ctx, kind, room.Id, threadRootEventID); err != nil {
		return err
	}
	return s.core.FollowThread(ctx, kind, actorID, room.Id, threadRootEventID)
}

func (s *ThreadFollowModel) UnfollowThread(ctx context.Context, actorID, roomID, threadRootEventID string) error {
	room, kind, err := s.core.requireThreadMessageReader(ctx, actorID, roomID, threadRootEventID)
	if err != nil {
		return err
	}
	if _, err := s.core.requireThreadRoot(ctx, kind, room.Id, threadRootEventID); err != nil {
		return err
	}
	return s.core.UnfollowThread(ctx, kind, actorID, room.Id, threadRootEventID)
}
