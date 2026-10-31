package bot

import chatto "github.com/chattocorp/chatto/experimental/chatto-go"

// ownerOnlyBatch rejects mixed authors, including messages already deduplicated
// into initial context. Ownership never carries over from prior turns.
func (c *conversation) ownerOnlyBatch(batch []chatto.MessageEvent) bool {
	if len(batch) == 0 {
		return false
	}
	for _, e := range batch {
		if !c.isOwner(e.Message.ActorID) {
			return false
		}
	}
	return true
}

// triggerAuthor binds recognition to a server-supplied author of one message.
// This metadata is model context, not authorization for privileged tool calls.
type triggerAuthor struct {
	MessageID string `json:"messageId"`
	UserID    string `json:"userId"`
	IsOwner   bool   `json:"isOwner"`
}

func (c *conversation) isOwner(userID string) bool {
	if userID == "" {
		return false
	}
	for _, owner := range c.ownerUserIDs {
		if userID == owner {
			return true
		}
	}
	return false
}
