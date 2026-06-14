package core

import "context"

func (c *ChattoCore) secureDeleteMessageBodyEvents(ctx context.Context, seqs []uint64) {
	if c == nil || c.storage == nil || c.storage.serverEvtStream == nil {
		return
	}
	seen := make(map[uint64]struct{}, len(seqs))
	for _, seq := range seqs {
		if seq == 0 {
			continue
		}
		if _, ok := seen[seq]; ok {
			continue
		}
		seen[seq] = struct{}{}
		if err := c.storage.serverEvtStream.SecureDeleteMsg(ctx, seq); err != nil {
			c.logger.Warn("Failed to secure-delete message body event", "seq", seq, "error", err)
		}
	}
}

func (c *ChattoCore) secureDeleteObsoleteMessageBodyEvents(ctx context.Context, eventID string) {
	c.secureDeleteMessageBodyEvents(ctx, c.rooms().obsoleteBodyEventSeqs(eventID))
}

func (c *ChattoCore) secureDeleteAllMessageBodyEvents(ctx context.Context, eventID string) {
	seqs, _, ok := c.rooms().bodyEventSeqs(eventID)
	if !ok {
		return
	}
	c.secureDeleteMessageBodyEvents(ctx, seqs)
}

func (c *ChattoCore) secureDeleteObsoleteProjectedMessageBodyEvents(ctx context.Context) {
	c.secureDeleteMessageBodyEvents(ctx, c.rooms().allObsoleteBodyEventSeqs())
}
