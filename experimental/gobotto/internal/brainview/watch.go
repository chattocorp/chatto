package brainview

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/hmans/means/collection"
)

// Changes optionally supplies coalesced filesystem notifications. A nil error
// means refresh; non-nil errors are safe for display. Cancellation closes the
// channel after the watcher releases all directory handles.
type Changes interface {
	Watch(context.Context) (<-chan error, error)
}

var errWatch = errors.New("Automatic refresh unavailable; use r to refresh")

// Watch combines means collection notifications with Gobotto's brain discovery.
// The root subscription observes new collection boundaries; separate subscriptions
// observe each brain's contents. No collection settings or documents are loaded.
func (b *Browser) Watch(ctx context.Context) (<-chan error, error) {
	ctx, cancel := context.WithCancel(ctx)
	events := make(chan error)
	out := make(chan error)
	subscriptions := map[string]context.CancelFunc{}
	var workers sync.WaitGroup
	stop := func() {
		cancel()
		workers.Wait()
	}
	subscribe := func(path string) error {
		child, stopChild := context.WithCancel(ctx)
		changes, err := collection.Watch(child, path)
		if err != nil {
			stopChild()
			return err
		}
		subscriptions[path] = stopChild
		workers.Add(1)
		go func() {
			defer workers.Done()
			// Drain after cancellation so means has released its handles before
			// this worker completes.
			for event := range changes {
				var safe error
				if event.Err != nil {
					safe = errWatch
				}
				select {
				case events <- safe:
				case <-child.Done():
				}
			}
		}()
		return nil
	}
	reconcile := func() error {
		paths, err := b.bindings(ctx)
		if err != nil {
			return err
		}
		if b.root != "" {
			paths["root"] = b.root
		}
		wanted := map[string]bool{}
		for key, path := range paths {
			absolute, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			// Built-ins are discovered through the root. For a missing root or
			// fixed binding, watch its existing parent until it appears.
			if _, active := subscriptions[absolute]; !active {
				if _, err := os.Stat(absolute); os.IsNotExist(err) {
					if key == "memory" || key == "conversations" || key == "schedules" {
						continue
					}
					absolute = filepath.Dir(absolute)
				}
				if _, active := subscriptions[absolute]; !active {
					if err := subscribe(absolute); err != nil {
						return err
					}
				}
			}
			wanted[absolute] = true
		}
		for path, stopChild := range subscriptions {
			if !wanted[path] {
				stopChild()
				delete(subscriptions, path)
			}
		}
		return nil
	}
	if err := reconcile(); err != nil {
		stop()
		return nil, errWatch
	}
	go func() {
		defer close(out)
		defer stop()
		var pending error
		ready := false
		for {
			var deliver chan error
			if ready {
				deliver = out
			}
			select {
			case <-ctx.Done():
				return
			case err := <-events:
				if reconcile() != nil || err != nil {
					pending = errWatch
				}
				ready = true
			case deliver <- pending:
				pending = nil
				ready = false
			}
		}
	}()
	return out, nil
}
