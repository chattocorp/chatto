package core

import (
	"context"

	"github.com/chattocorp/chatto/cli/internal/readmodel"
)

// GetUser retrieves a user from the user projection.
func (m *UserModel) GetUser(ctx context.Context, userID string) (*readmodel.User, error) {
	user, ok, err := m.user(ctx, userID)
	if err != nil {
		return nil, err
	}
	if ok {
		return user, nil
	}
	return nil, ErrNotFound
}

// GetUserByLogin retrieves a user by their login name using the login index.
func (m *UserModel) GetUserByLogin(ctx context.Context, login string) (*readmodel.User, error) {
	user, ok, err := m.userByLogin(ctx, login)
	if err != nil {
		return nil, err
	}
	if ok {
		return user, nil
	}
	return nil, ErrNotFound
}

// activeIDs filters candidates against the current account projection.
func (m *UserModel) activeIDs(ids []string) []string {
	return m.users.Projection().ActiveIDs(ids)
}

// allActiveIDs returns the active accounts used by universal-room membership.
func (m *UserModel) allActiveIDs() []string {
	return m.users.Projection().AllActiveIDs()
}
