package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

const maxBotThreadAccessMutationAttempts = 5

// BotContentReadBoundary is an opaque durable authorization snapshot. Callers
// that assemble protected bot-visible content after authorization must verify
// it before returning that content.
type BotContentReadBoundary struct {
	authorizationSeq uint64
	roomFilter       string
	roomSeq          uint64
}

// BotContentListBoundary retains the global authorization fence and every
// room boundary contributing metadata to one bot list response.
type BotContentListBoundary struct {
	authorizationSeq uint64
	rooms            []BotContentReadBoundary
}

func (c *ChattoCore) waitForBotThreadContextsCurrent(ctx context.Context, roomID string) error {
	if roomID == "" {
		return waitForProjectionSubjectsCurrent(ctx, c.EventPublisher, "bot thread contexts", c.roomModel.threads.Projector(),
			c.roomModel.threads.Projection().Subjects()...)
	}
	aggregate := evtstream.RoomAggregate(roomID)
	return waitForProjectionSubjectsCurrent(ctx, c.EventPublisher, "bot thread contexts", c.roomModel.threads.Projector(),
		aggregate.Subject(evtstream.EventMessagePosted),
		aggregate.Subject(evtstream.EventUserLeftRoom),
		aggregate.Subject(evtstream.EventBotThreadAccessRemoved),
	)
}

// prepareBotThreadReadBoundary brings every projection that contributes to a
// bot thread authorization decision to a captured durable boundary. Callers
// must verify the boundary again after reading projection state: room facts use
// their aggregate lane, while roles, permissions, group layout, and account
// state share the global authorization fence.
func (c *ChattoCore) prepareBotThreadReadBoundary(ctx context.Context, roomID string) (BotContentReadBoundary, error) {
	boundary := BotContentReadBoundary{roomFilter: evtstream.RoomAggregate(roomID).AllEventsFilter()}
	var err error
	boundary.authorizationSeq, err = c.authorizationFenceSeq(ctx)
	if err != nil {
		return BotContentReadBoundary{}, fmt.Errorf("read bot thread authorization fence: %w", err)
	}
	roomPosition, err := c.EventPublisher.LastSubjectPosition(ctx, boundary.roomFilter)
	if err != nil {
		return BotContentReadBoundary{}, fmt.Errorf("read bot thread room boundary: %w", err)
	}
	boundary.roomSeq = roomPosition.Seq
	groupPosition, err := c.EventPublisher.LastSubjectPosition(ctx, evtstream.GroupSubjectFilter())
	if err != nil {
		return BotContentReadBoundary{}, fmt.Errorf("read bot thread group boundary: %w", err)
	}
	rbacPosition, err := c.EventPublisher.LastSubjectPosition(ctx, evtstream.RBACSubjectFilter())
	if err != nil {
		return BotContentReadBoundary{}, fmt.Errorf("read bot thread RBAC boundary: %w", err)
	}

	if !roomPosition.IsZero() {
		if err := c.roomModel.waitForDirectory(ctx, roomPosition); err != nil {
			return BotContentReadBoundary{}, err
		}
	}
	if !groupPosition.IsZero() {
		if err := c.roomModel.waitForGroupLayout(ctx, groupPosition); err != nil {
			return BotContentReadBoundary{}, err
		}
	}
	if !rbacPosition.IsZero() {
		if err := c.rbacModel.waitFor(ctx, rbacPosition); err != nil {
			return BotContentReadBoundary{}, err
		}
	}
	if err := c.waitForBotThreadContextsCurrent(ctx, roomID); err != nil {
		return BotContentReadBoundary{}, err
	}
	return boundary, nil
}

// BotContentReadBoundaryUnchanged reports whether the room and authorization
// inputs captured by a prior bot-content authorization stayed stable while the
// caller assembled protected content.
func (c *ChattoCore) BotContentReadBoundaryUnchanged(ctx context.Context, boundary BotContentReadBoundary) (bool, error) {
	authorizationSeq, err := c.authorizationFenceSeq(ctx)
	if err != nil {
		return false, fmt.Errorf("re-read bot thread authorization fence: %w", err)
	}
	roomSeq, err := c.EventPublisher.LastSubjectSeq(ctx, boundary.roomFilter)
	if err != nil {
		return false, fmt.Errorf("re-read bot thread room boundary: %w", err)
	}
	return authorizationSeq == boundary.authorizationSeq && roomSeq == boundary.roomSeq, nil
}

// BotContentListBoundaryUnchanged verifies a retained bot-list snapshot before
// the transport releases room or thread metadata.
func (c *ChattoCore) BotContentListBoundaryUnchanged(ctx context.Context, boundary BotContentListBoundary) (bool, error) {
	authorizationSeq, err := c.authorizationFenceSeq(ctx)
	if err != nil {
		return false, fmt.Errorf("re-read bot list authorization fence: %w", err)
	}
	if authorizationSeq != boundary.authorizationSeq {
		return false, nil
	}
	for _, room := range boundary.rooms {
		roomSeq, err := c.EventPublisher.LastSubjectSeq(ctx, room.roomFilter)
		if err != nil {
			return false, fmt.Errorf("re-read bot list room boundary: %w", err)
		}
		if roomSeq != room.roomSeq {
			return false, nil
		}
	}
	return true, nil
}

func (c *ChattoCore) prepareBotDirectMessageReadBoundary(ctx context.Context, roomID string) (BotContentReadBoundary, error) {
	boundary := BotContentReadBoundary{roomFilter: evtstream.RoomAggregate(roomID).AllEventsFilter()}
	var err error
	boundary.authorizationSeq, err = c.authorizationFenceSeq(ctx)
	if err != nil {
		return BotContentReadBoundary{}, fmt.Errorf("read bot DM authorization fence: %w", err)
	}
	roomPosition, err := c.EventPublisher.LastSubjectPosition(ctx, boundary.roomFilter)
	if err != nil {
		return BotContentReadBoundary{}, fmt.Errorf("read bot DM room boundary: %w", err)
	}
	boundary.roomSeq = roomPosition.Seq
	if !roomPosition.IsZero() {
		if err := c.roomModel.waitForDirectoryAndTimeline(ctx, roomPosition); err != nil {
			return BotContentReadBoundary{}, err
		}
	}
	return boundary, nil
}

// AuthorizeBotDirectMessageReadAtBoundary verifies the bot capability and
// explicit DM context at a durable boundary that response assemblers retain
// until immediately before releasing message content.
func (c *ChattoCore) AuthorizeBotDirectMessageReadAtBoundary(ctx context.Context, botID, roomID string) (BotContentReadBoundary, error) {
	for attempt := 1; attempt <= maxBotThreadAccessMutationAttempts; attempt++ {
		boundary, err := c.prepareBotDirectMessageReadBoundary(ctx, roomID)
		if err != nil {
			return BotContentReadBoundary{}, err
		}
		if _, _, err := c.AuthorizeBotCapability(ctx, botID, ApplicationCapabilityDMMessageRead); err != nil {
			return BotContentReadBoundary{}, err
		}
		room, err := c.FindRoomByID(ctx, roomID)
		if err != nil || KindOfRoom(room) != KindDM || !c.roomModel.hasExplicitRoomMembership(roomID, botID) {
			return BotContentReadBoundary{}, ErrPermissionDenied
		}
		unchanged, err := c.BotContentReadBoundaryUnchanged(ctx, boundary)
		if err != nil {
			return BotContentReadBoundary{}, err
		}
		if unchanged {
			return boundary, nil
		}
	}
	return BotContentReadBoundary{}, fmt.Errorf("authorize bot DM read retry exhausted: %w", events.ErrConflict)
}

// ListBotDirectMessageRoomsAtBoundary returns explicit bot DMs together with
// the authorization and room boundaries the caller must retain through list
// response assembly.
func (c *ChattoCore) ListBotDirectMessageRoomsAtBoundary(ctx context.Context, botID string) ([]*corev1.Room, BotContentListBoundary, error) {
	for attempt := 1; attempt <= maxBotThreadAccessMutationAttempts; attempt++ {
		authorizationSeq, err := c.authorizationFenceSeq(ctx)
		if err != nil {
			return nil, BotContentListBoundary{}, err
		}
		if _, _, err := c.AuthorizeBotCapability(ctx, botID, ApplicationCapabilityDMMessageRead); err != nil {
			return nil, BotContentListBoundary{}, err
		}
		if err := c.roomModel.waitForDirectoryCurrent(ctx, c.EventPublisher); err != nil {
			return nil, BotContentListBoundary{}, err
		}
		rooms, err := c.ListMemberRooms(ctx, KindDM, botID, MemberRoomListOptions{})
		if err != nil {
			return nil, BotContentListBoundary{}, err
		}
		boundary := BotContentListBoundary{authorizationSeq: authorizationSeq, rooms: make([]BotContentReadBoundary, 0, len(rooms))}
		for _, room := range rooms {
			roomFilter := evtstream.RoomAggregate(room.GetId()).AllEventsFilter()
			roomPosition, err := c.EventPublisher.LastSubjectPosition(ctx, roomFilter)
			if err != nil {
				return nil, BotContentListBoundary{}, err
			}
			if !roomPosition.IsZero() {
				if err := c.roomModel.waitForDirectory(ctx, roomPosition); err != nil {
					return nil, BotContentListBoundary{}, err
				}
			}
			boundary.rooms = append(boundary.rooms, BotContentReadBoundary{
				authorizationSeq: authorizationSeq, roomFilter: roomFilter, roomSeq: roomPosition.Seq,
			})
		}
		unchanged, err := c.BotContentListBoundaryUnchanged(ctx, boundary)
		if err != nil {
			return nil, BotContentListBoundary{}, err
		}
		if unchanged {
			return rooms, boundary, nil
		}
	}
	return nil, BotContentListBoundary{}, fmt.Errorf("list bot DMs retry exhausted: %w", events.ErrConflict)
}

// ListBotThreadContexts returns the active mention-derived thread contexts for
// a bot after the global room-event projection is current. Capability and
// owner checks are included because this method crosses a privacy boundary.
func (c *ChattoCore) ListBotThreadContexts(ctx context.Context, botID string) ([]BotThreadContext, error) {
	contexts, _, err := c.ListBotThreadContextsAtBoundary(ctx, botID)
	return contexts, err
}

// ListBotThreadContextsAtBoundary returns the authorized thread-context
// snapshot and every boundary needed to keep the assembled list coherent.
func (c *ChattoCore) ListBotThreadContextsAtBoundary(ctx context.Context, botID string) ([]BotThreadContext, BotContentListBoundary, error) {
	for attempt := 1; attempt <= maxBotThreadAccessMutationAttempts; attempt++ {
		authorizationSeq, err := c.authorizationFenceSeq(ctx)
		if err != nil {
			return nil, BotContentListBoundary{}, err
		}
		if _, _, err := c.AuthorizeBotCapability(ctx, botID, ApplicationCapabilityThreadRead); err != nil {
			return nil, BotContentListBoundary{}, err
		}
		if err := c.waitForBotThreadContextsCurrent(ctx, ""); err != nil {
			return nil, BotContentListBoundary{}, err
		}
		candidates := c.roomModel.threads.Projection().BotThreadContexts(botID)
		out := make([]BotThreadContext, 0, len(candidates))
		boundary := BotContentListBoundary{authorizationSeq: authorizationSeq, rooms: make([]BotContentReadBoundary, 0, len(candidates))}
		for _, candidate := range candidates {
			_, _, context, roomBoundary, err := c.AuthorizeBotThreadContextAtBoundary(ctx, botID, candidate.RoomID, candidate.ThreadRootEventID, ApplicationCapabilityThreadRead)
			if errors.Is(err, ErrPermissionDenied) || errors.Is(err, ErrNotFound) {
				continue
			}
			if err != nil {
				return nil, BotContentListBoundary{}, err
			}
			out = append(out, context)
			boundary.rooms = append(boundary.rooms, roomBoundary)
		}
		unchanged, err := c.BotContentListBoundaryUnchanged(ctx, boundary)
		if err != nil {
			return nil, BotContentListBoundary{}, err
		}
		if unchanged {
			return out, boundary, nil
		}
	}
	return nil, BotContentListBoundary{}, fmt.Errorf("list bot thread contexts retry exhausted: %w", events.ErrConflict)
}

// AuthorizeBotThreadContext applies capability, owner, room-installation, and
// explicit thread-invitation gates against one coherent durable boundary.
func (c *ChattoCore) AuthorizeBotThreadContext(ctx context.Context, botID, roomID, threadRootEventID string, capability ApplicationCapability) (*corev1.User, *corev1.User, BotThreadContext, error) {
	bot, owner, threadContext, _, err := c.AuthorizeBotThreadContextAtBoundary(ctx, botID, roomID, threadRootEventID, capability)
	return bot, owner, threadContext, err
}

// AuthorizeBotThreadContextAtBoundary returns the durable boundary that made
// the decision valid so a privacy-bearing caller can retain authorization
// through response assembly.
func (c *ChattoCore) AuthorizeBotThreadContextAtBoundary(ctx context.Context, botID, roomID, threadRootEventID string, capability ApplicationCapability) (*corev1.User, *corev1.User, BotThreadContext, BotContentReadBoundary, error) {
	for attempt := 1; attempt <= maxBotThreadAccessMutationAttempts; attempt++ {
		boundary, err := c.prepareBotThreadReadBoundary(ctx, roomID)
		if err != nil {
			return nil, nil, BotThreadContext{}, BotContentReadBoundary{}, err
		}
		bot, owner, err := c.AuthorizeBotCapability(ctx, botID, capability)
		if err != nil {
			return nil, nil, BotThreadContext{}, BotContentReadBoundary{}, err
		}
		room, err := c.FindRoomByID(ctx, roomID)
		if err != nil || KindOfRoom(room) != KindChannel || room.GetArchived() {
			return nil, nil, BotThreadContext{}, BotContentReadBoundary{}, ErrPermissionDenied
		}
		if !c.roomModel.hasExplicitRoomMembership(roomID, botID) {
			return nil, nil, BotThreadContext{}, BotContentReadBoundary{}, ErrPermissionDenied
		}
		ownerMember, err := c.RoomMembershipExists(ctx, KindChannel, owner.GetId(), roomID)
		if err != nil {
			return nil, nil, BotThreadContext{}, BotContentReadBoundary{}, err
		}
		if !ownerMember {
			return nil, nil, BotThreadContext{}, BotContentReadBoundary{}, ErrPermissionDenied
		}
		context, ok := c.roomModel.threads.Projection().BotThreadContext(botID, roomID, threadRootEventID)
		if !ok {
			return nil, nil, BotThreadContext{}, BotContentReadBoundary{}, ErrPermissionDenied
		}
		unchanged, err := c.BotContentReadBoundaryUnchanged(ctx, boundary)
		if err != nil {
			return nil, nil, BotThreadContext{}, BotContentReadBoundary{}, err
		}
		if unchanged {
			return bot, owner, context, boundary, nil
		}
	}
	return nil, nil, BotThreadContext{}, BotContentReadBoundary{}, fmt.Errorf("authorize bot thread context retry exhausted: %w", events.ErrConflict)
}

// RemoveBotThreadAccess revokes one mention-derived grant. The original
// inviter, the accountable bot owner, or a current room manager may remove it.
func (c *ChattoCore) RemoveBotThreadAccess(ctx context.Context, actorID, botID, roomID, threadRootEventID string) (bool, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return false, err
	}
	aggregate := evtstream.RoomAggregate(roomID)
	filter := aggregate.AllEventsFilter()

	for attempt := 1; attempt <= maxBotThreadAccessMutationAttempts; attempt++ {
		var exists bool
		prepared, err := c.prepareMessageAppendAttempt(ctx, aggregate, actorID, func(attemptCtx context.Context) error {
			if err := c.userModel.waitForUsersCurrent(attemptCtx, "bot thread access removal", evtstream.UserAggregate(botID).AllEventsFilter()); err != nil {
				return err
			}
			bot, err := c.GetUser(attemptCtx, botID)
			if err != nil || !isBotAccount(bot) {
				return ErrNotFound
			}
			if err := c.waitForBotThreadContextsCurrent(attemptCtx, roomID); err != nil {
				return err
			}
			context, contextExists := c.roomModel.threads.Projection().BotThreadContext(botID, roomID, threadRootEventID)
			exists = contextExists
			if !exists {
				return nil
			}
			if slices.Contains(context.InviterIDs, actorID) || bot.GetBot().GetOwnerId() == actorID {
				return nil
			}
			allowed, err := c.PermResolver().HasRoomPermission(attemptCtx, actorID, KindChannel, roomID, PermRoomManage)
			if err != nil {
				return err
			}
			if !allowed {
				return ErrPermissionDenied
			}
			return nil
		})
		if err != nil {
			return false, err
		}
		if !exists {
			return false, nil
		}

		event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_BotThreadAccessRemoved{
			BotThreadAccessRemoved: &corev1.BotThreadAccessRemovedEvent{
				RoomId: roomID, ThreadRootEventId: threadRootEventID, BotId: botID,
			},
		}})
		sequences, err := c.appendAuthorizationFencedBatch(ctx, actorID, []evtstream.BatchEntry{{
			Subject: aggregate.SubjectFor(event), Event: event, HasOCC: true,
			ExpectedSeq: prepared.roomSeq, FilterSubject: filter,
		}}, prepared.authorizationSeq)
		if errors.Is(err, events.ErrConflict) {
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
			}
			continue
		}
		if err != nil {
			return false, fmt.Errorf("remove bot thread access: %w", err)
		}
		if err := c.roomModel.waitForThreads(ctx, events.SubjectPosition(aggregate.SubjectFor(event), sequences[0])); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, fmt.Errorf("remove bot thread access retry exhausted: %w", events.ErrConflict)
}
