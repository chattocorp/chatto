package bot

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
