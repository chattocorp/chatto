package core

import (
	"context"
)

// authorizeAtStableInputs delegates to shared authorization policy.
func (c *ChattoCore) authorizeAtStableInputs(ctx context.Context, check func() error) error {
	return c.authorization.authorizeAtStableInputs(ctx, check)
}

// authorizeAtStableRoomInputs delegates to shared authorization policy.
func (c *ChattoCore) authorizeAtStableRoomInputs(ctx context.Context, check func() error) error {
	return c.authorization.authorizeAtStableRoomInputs(ctx, check)
}
