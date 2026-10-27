package core

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

var errAccountDeactivationUnchanged = errors.New("account deactivation is unchanged")

// CanChangeUserDeactivation supplies the same policy for commands and admin UI.
// Human accounts can be managed, but self-deactivation and owner lockout are
// prohibited. Bot lifecycle remains owned by bot management.
func (c *ChattoCore) CanChangeUserDeactivation(ctx context.Context, actorID, userID string) (bool, error) {
	if actorID == "" || actorID == userID {
		return false, nil
	}
	allowed, err := c.CanManageUserAccounts(ctx, actorID)
	if err != nil || !allowed {
		return false, err
	}
	user, err := c.GetUser(ctx, userID)
	if err != nil {
		return false, err
	}
	if user.GetIsBot() || user.GetDeleted() {
		return false, nil
	}
	owner, err := c.IsServerOwner(ctx, userID)
	return !owner && err == nil, err
}

// SetUserDeactivated changes access without erasing account data. The durable
// auth projection rejects credentials on every replica, including credentials
// written by a login concurrent with this command. Reactivation never restores
// an old session. Repeated commands do not append another fact.
func (c *ChattoCore) SetUserDeactivated(ctx context.Context, actorID, userID string, deactivated bool) error {
	if actorID == "" {
		return ErrNotAuthenticated
	}
	if userID == "" {
		return fmt.Errorf("%w: target user ID is required", ErrInvalidArgument)
	}
	event := newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_UserAccountDeactivationChanged{
		UserAccountDeactivationChanged: &evtv1.UserAccountDeactivationChangedEvent{UserId: userID, Deactivated: deactivated},
	}})
	// Include role changes in OCC so a concurrent owner assignment or permission
	// revocation cannot invalidate the authorization decision before commit.
	_, err := c.appendUserEvent(ctx, userID, event, evtstream.EventSubjectFilter(), func() error {
		allowed, err := c.CanChangeUserDeactivation(ctx, actorID, userID)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrPermissionDenied
		}
		user, err := c.GetUser(ctx, userID)
		if err != nil {
			return err
		}
		if user.GetDeactivated() == deactivated {
			return errAccountDeactivationUnchanged
		}
		return nil
	})
	if errors.Is(err, errAccountDeactivationUnchanged) {
		return nil
	}
	return err
}

// WatchAccountAccessInvalidated closes when a durable deactivation transition
// invalidates this credential. Registration checks state under the projection
// lock, so a transition before registration cannot leave a socket unwatched.
// The caller must cancel the watch when its connection closes.
func (c *ChattoCore) WatchAccountAccessInvalidated(userID string) (<-chan struct{}, func()) {
	p := c.userModel.auth.Projection()
	invalidated := make(chan struct{})
	p.Lock()
	u := p.users[userID]
	if u == nil || u.deleted || u.deactivated {
		close(invalidated)
		p.Unlock()
		return invalidated, func() {}
	}
	if p.accountAccessWatchers == nil {
		p.accountAccessWatchers = make(map[string]map[chan struct{}]struct{})
	}
	if p.accountAccessWatchers[userID] == nil {
		p.accountAccessWatchers[userID] = make(map[chan struct{}]struct{})
	}
	p.accountAccessWatchers[userID][invalidated] = struct{}{}
	p.Unlock()
	var once sync.Once
	return invalidated, func() {
		once.Do(func() {
			p.Lock()
			defer p.Unlock()
			delete(p.accountAccessWatchers[userID], invalidated)
			if len(p.accountAccessWatchers[userID]) == 0 {
				delete(p.accountAccessWatchers, userID)
			}
		})
	}
}
