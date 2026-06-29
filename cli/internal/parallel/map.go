package parallel

import (
	"context"

	"golang.org/x/sync/errgroup"
)

// Map runs fn for each item with bounded concurrency and returns results in
// input order. The first error cancels the remaining work.
func Map[T any, R any](ctx context.Context, limit int, items []T, fn func(context.Context, int, T) (R, error)) ([]R, error) {
	out := make([]R, len(items))
	if len(items) == 0 {
		return out, nil
	}
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(limit)

	for i, item := range items {
		i, item := i, item
		g.Go(func() error {
			result, err := fn(ctx, i, item)
			if err != nil {
				return err
			}
			out[i] = result
			return nil
		})
	}

	return out, g.Wait()
}
