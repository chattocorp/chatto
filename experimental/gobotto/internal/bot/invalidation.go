package bot

import (
	"context"
	"errors"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

// invalidatedTurn is a host-authored cancellation cause, safe for logs.
type invalidatedTurn string

func (e invalidatedTurn) Error() string { return string(e) }

func invalidationCause(e chatto.MessageEvent) error {
	switch e.InvalidationReason {
	case chatto.MessageEdited:
		return invalidatedTurn("Cancelled: a message in this conversation was edited")
	case chatto.MessageRetracted:
		return invalidatedTurn("Cancelled: a message in this conversation was retracted")
	case chatto.RoomDeleted:
		return invalidatedTurn("Cancelled: room deleted")
	case chatto.ViewerLeftRoom:
		return invalidatedTurn("Cancelled: bot left the room")
	case chatto.ViewerPermissionsChanged:
		return invalidatedTurn("Cancelled: viewer permissions changed")
	case chatto.AccountDeleted:
		return invalidatedTurn("Cancelled: account deleted")
	default:
		return invalidatedTurn("Cancelled: conversation access or history invalidated")
	}
}

// affectedBy runs on the intake goroutine. The worker publishes context IDs via
// sync.Map so this check never reads its mutable transcript or fingerprint maps.
func (c *conversation) affectedBy(e chatto.MessageEvent, active bool, batch []chatto.MessageEvent) bool {
	if e.InvalidateAll {
		return true
	}
	if c.key.room != e.InvalidatedRoom {
		return false
	}
	if e.InvalidatedMessage == "" {
		return true
	}
	if e.InvalidationReason != chatto.MessageEdited && e.InvalidationReason != chatto.MessageRetracted {
		return true
	}
	if e.InvalidatedMessage == c.key.root {
		return true
	}
	if _, ok := c.contextIDs.Load(e.InvalidatedMessage); ok {
		return true
	}
	for _, pending := range c.pending {
		if pending.Message.ID == e.InvalidatedMessage {
			return true
		}
	}
	if active {
		for _, pending := range batch {
			if pending.Message.ID == e.InvalidatedMessage {
				return true
			}
		}
		// A read in progress may still return the changed message. Until its
		// membership is known, do not let a potentially stale snapshot proceed.
		return !c.contextReady.Load()
	}
	return false
}

func modelFailureDetail(ctx context.Context, err error) string {
	var cause invalidatedTurn
	if ctx.Err() != nil && errors.As(context.Cause(ctx), &cause) {
		return cause.Error()
	}
	return agent.ModelFailureDetail(err)
}
