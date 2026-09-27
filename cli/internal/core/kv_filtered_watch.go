package core

import (
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/nats-io/nats.go/jetstream"
)

// watchKeyFilters watches several KV key filters through one single-filter
// watcher per filter and merges their updates.
//
// JetStream serves a single-filter watcher from its per-subject index. One
// multi-filter watcher instead intersects the filters with every message block
// of the bucket. On a high-churn bucket such as RUNTIME_STATE, which holds
// thousands of sparse blocks, that scan made each initial snapshot take
// seconds and delayed server startup.
//
// The merged watcher keeps the KeyWatcher contract that callers rely on:
//   - It sends one nil entry after every source watcher has delivered its
//     initial values, so the nil still marks a complete initial snapshot.
//   - Entries from one source keep their order. Entries from different
//     sources can interleave, which is safe because the filters select
//     disjoint keys.
//   - Updates closes when any source closes, so callers detect a stopped
//     watcher and restart the complete set.
//
// With the UpdatesOnly option, JetStream sends no initial-snapshot marker, so
// the merged watcher sends none either. At least one filter is required;
// JetStream would treat an empty filter list as "all keys".
func watchKeyFilters(ctx context.Context, kv jetstream.KeyValue, filters []string, opts ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	if len(filters) == 0 {
		return nil, errors.New("watch key filters: at least one filter is required")
	}
	if len(filters) == 1 {
		// WatchFiltered rewrites its key slice in place; keep the caller's
		// slice unchanged.
		return kv.WatchFiltered(ctx, slices.Clone(filters), opts...)
	}
	merged := &mergedKeyWatcher{
		updates: make(chan jetstream.KeyValueEntry, 256),
		stop:    make(chan struct{}),
	}
	for _, filter := range filters {
		watcher, err := kv.WatchFiltered(ctx, []string{filter}, opts...)
		if err != nil {
			_ = merged.Stop()
			return nil, err
		}
		merged.sources = append(merged.sources, watcher)
	}
	merged.start()
	return merged, nil
}

// mergedKeyWatcher fans several KV watchers into one KeyWatcher.
type mergedKeyWatcher struct {
	sources  []jetstream.KeyWatcher
	updates  chan jetstream.KeyValueEntry
	stop     chan struct{}
	stopOnce sync.Once
	stopErr  error
}

func (w *mergedKeyWatcher) start() {
	var (
		forwarders   sync.WaitGroup
		initialMu    sync.Mutex
		initialsLeft = len(w.sources)
	)
	for _, source := range w.sources {
		forwarders.Add(1)
		go func(source jetstream.KeyWatcher) {
			defer forwarders.Done()
			// A closed source ends the merged watcher, so the caller restarts
			// every source together.
			defer func() { _ = w.Stop() }()
			initialDone := false
			for {
				var entry jetstream.KeyValueEntry
				select {
				case <-w.stop:
					return
				case next, ok := <-source.Updates():
					if !ok {
						return
					}
					entry = next
				}
				if entry == nil {
					if initialDone {
						continue
					}
					initialDone = true
					initialMu.Lock()
					initialsLeft--
					last := initialsLeft == 0
					initialMu.Unlock()
					if !last {
						continue
					}
				}
				select {
				case <-w.stop:
					return
				case w.updates <- entry:
				}
			}
		}(source)
	}
	go func() {
		forwarders.Wait()
		close(w.updates)
	}()
}

// Updates returns the merged entries. See watchKeyFilters for its contract.
func (w *mergedKeyWatcher) Updates() <-chan jetstream.KeyValueEntry {
	return w.updates
}

// Stop stops every source watcher. It is safe to call more than once.
func (w *mergedKeyWatcher) Stop() error {
	w.stopOnce.Do(func() {
		close(w.stop)
		var errs []error
		for _, source := range w.sources {
			if err := source.Stop(); err != nil {
				errs = append(errs, err)
			}
		}
		w.stopErr = errors.Join(errs...)
	})
	return w.stopErr
}
