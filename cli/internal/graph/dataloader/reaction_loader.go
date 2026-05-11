package dataloader

import (
	"context"

	"github.com/vikstrous/dataloadgen"
	"hmans.de/chatto/internal/core"
)

// ReactionKey identifies a specific message's reactions.
// Post-ADR-030 the storage is server-wide; the loader keys solely on event id.
type ReactionKey struct {
	EventID string
}

// newReactionLoader creates a dataloader that batches reaction lookups.
// Multiple messages' reactions are fetched in a single ListKeysFiltered call.
func newReactionLoader(c *core.ChattoCore) *dataloadgen.Loader[ReactionKey, []core.ReactionSummary] {
	return dataloadgen.NewLoader(
		func(ctx context.Context, keys []ReactionKey) ([][]core.ReactionSummary, []error) {
			return batchGetReactions(ctx, c, keys)
		},
		dataloadgen.WithWait(defaultWait),
	)
}

// batchGetReactions fetches reactions for multiple messages in a single ListKeysFiltered call.
// Returns results and errors in the same order as keys.
func batchGetReactions(ctx context.Context, c *core.ChattoCore, keys []ReactionKey) ([][]core.ReactionSummary, []error) {
	results := make([][]core.ReactionSummary, len(keys))
	errs := make([]error, len(keys))

	eventIDs := make([]string, len(keys))
	for i, k := range keys {
		eventIDs[i] = k.EventID
	}

	batch, err := c.GetReactionsBatch(ctx, eventIDs)
	if err != nil {
		for i := range keys {
			errs[i] = err
		}
		return results, errs
	}

	for i, k := range keys {
		summaries := batch[k.EventID]
		if summaries == nil {
			summaries = []core.ReactionSummary{}
		}
		results[i] = summaries
	}

	return results, errs
}
