package core

import (
	"context"
	"fmt"
	"time"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/pkg/events"
)

const maxStableAuthorizationAttempts = 5

type authorizationInputPositions struct {
	rbac  events.StreamPosition
	group events.StreamPosition
	user  events.StreamPosition
	room  events.StreamPosition
}

func (p authorizationInputPositions) equal(other authorizationInputPositions) bool {
	return p.rbac.Seq == other.rbac.Seq &&
		p.group.Seq == other.group.Seq &&
		p.user.Seq == other.user.Seq &&
		p.room.Seq == other.room.Seq
}

// authorizeAtStableInputs evaluates check against a stable request-time view
// of the cross-aggregate inputs used by permission resolution. It repeats the
// read when an input changes while check is running. A change after the final
// validation is concurrent with the authorized command and does not cancel it.
// Target aggregate invariants remain protected separately by the command's OCC
// guard.
func (a *AuthorizationModel) authorizeAtStableInputs(ctx context.Context, check func() error) error {
	return a.authorizeAtStableInputsWithRoomCatalog(ctx, false, check)
}

// authorizeAtStableRoomInputs also stabilizes the complete room catalog. It is
// reserved for low-frequency decisions, such as delegated role assignment,
// that can inspect permission scopes in more than one room. Ordinary room
// commands use their exact room aggregate instead.
func (a *AuthorizationModel) authorizeAtStableRoomInputs(ctx context.Context, check func() error) error {
	return a.authorizeAtStableInputsWithRoomCatalog(ctx, true, check)
}

func (a *AuthorizationModel) authorizeAtStableInputsWithRoomCatalog(ctx context.Context, includeRooms bool, check func() error) error {
	if check == nil {
		return nil
	}

	for attempt := range maxStableAuthorizationAttempts {
		before, err := a.authorizationInputPositions(ctx, includeRooms)
		if err != nil {
			return err
		}
		if err := a.waitForAuthorizationInputs(ctx, before); err != nil {
			return err
		}

		decisionErr := check()
		after, err := a.authorizationInputPositions(ctx, includeRooms)
		if err != nil {
			return err
		}
		if before.equal(after) {
			return decisionErr
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}

	return fmt.Errorf("stable authorization read exceeded %d attempts: %w", maxStableAuthorizationAttempts, events.ErrConflict)
}

func (a *AuthorizationModel) authorizationInputPositions(ctx context.Context, includeRooms bool) (authorizationInputPositions, error) {
	rbac, err := a.publisher.LastSubjectPosition(ctx, evtstream.RBACSubjectFilter())
	if err != nil {
		return authorizationInputPositions{}, fmt.Errorf("read RBAC authorization input: %w", err)
	}
	group, err := a.publisher.LastSubjectPosition(ctx, evtstream.GroupSubjectFilter())
	if err != nil {
		return authorizationInputPositions{}, fmt.Errorf("read room-group authorization input: %w", err)
	}
	user, err := a.publisher.LastSubjectPosition(ctx, evtstream.UserSubjectFilter())
	if err != nil {
		return authorizationInputPositions{}, fmt.Errorf("read user authorization input: %w", err)
	}
	positions := authorizationInputPositions{rbac: rbac, group: group, user: user}
	if includeRooms {
		room, err := a.publisher.LastSubjectPosition(ctx, evtstream.RoomSubjectFilter())
		if err != nil {
			return authorizationInputPositions{}, fmt.Errorf("read room authorization input: %w", err)
		}
		positions.room = room
	}
	return positions, nil
}

func (a *AuthorizationModel) waitForAuthorizationInputs(ctx context.Context, positions authorizationInputPositions) error {
	if !positions.rbac.IsZero() {
		if err := a.rbac.waitFor(ctx, positions.rbac); err != nil {
			return fmt.Errorf("wait for RBAC authorization input: %w", err)
		}
	}
	if !positions.group.IsZero() {
		if err := a.rooms.waitForGroupLayout(ctx, positions.group); err != nil {
			return fmt.Errorf("wait for room-group authorization input: %w", err)
		}
	}
	if !positions.user.IsZero() {
		if err := a.users.waitForUsers(ctx, positions.user); err != nil {
			return fmt.Errorf("wait for user authorization input: %w", err)
		}
	}
	if !positions.room.IsZero() {
		if err := a.rooms.waitForDirectory(ctx, positions.room); err != nil {
			return fmt.Errorf("wait for room authorization input: %w", err)
		}
	}
	return nil
}
