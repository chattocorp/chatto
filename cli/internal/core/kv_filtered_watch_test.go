package core

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/testutil"
)

func newFilteredWatchTestKV(t *testing.T) jetstream.KeyValue {
	t.Helper()
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	kv, err := js.CreateKeyValue(testContext(t), jetstream.KeyValueConfig{Bucket: "FILTERED_WATCH"})
	if err != nil {
		t.Fatal(err)
	}
	return kv
}

// recordingWatchKV records the key filters of every WatchFiltered call and can
// replace one filter's watcher.
type recordingWatchKV struct {
	jetstream.KeyValue
	mu       sync.Mutex
	calls    [][]string
	override map[string]jetstream.KeyWatcher
}

func (kv *recordingWatchKV) WatchFiltered(ctx context.Context, keys []string, opts ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	kv.mu.Lock()
	kv.calls = append(kv.calls, slices.Clone(keys))
	watcher := kv.override[keys[0]]
	kv.mu.Unlock()
	if watcher != nil {
		return watcher, nil
	}
	return kv.KeyValue.WatchFiltered(ctx, keys, opts...)
}

type channelKeyWatcher struct {
	updates chan jetstream.KeyValueEntry
}

func (w *channelKeyWatcher) Updates() <-chan jetstream.KeyValueEntry { return w.updates }
func (w *channelKeyWatcher) Stop() error                             { return nil }

func receiveKeyWatcherEntry(t *testing.T, watcher jetstream.KeyWatcher) (jetstream.KeyValueEntry, bool) {
	t.Helper()
	select {
	case entry, ok := <-watcher.Updates():
		return entry, ok
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a watcher entry")
		return nil, false
	}
}

func TestWatchKeyFiltersUsesOneSingleFilterWatcherPerFilter(t *testing.T) {
	ctx := testContext(t)
	kv := &recordingWatchKV{KeyValue: newFilteredWatchTestKV(t)}
	for _, key := range []string{"alpha.one", "alpha.two", "beta.one", "gamma.one"} {
		if _, err := kv.Put(ctx, key, []byte(key)); err != nil {
			t.Fatal(err)
		}
	}

	watcher, err := watchKeyFilters(ctx, kv, []string{"alpha.>", "beta.>"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = watcher.Stop() }()

	kv.mu.Lock()
	calls := slices.Clone(kv.calls)
	kv.mu.Unlock()
	if len(calls) != 2 || len(calls[0]) != 1 || len(calls[1]) != 1 {
		t.Fatalf("WatchFiltered calls = %v, want one single-filter watcher per filter", calls)
	}

	var initial []string
	for {
		entry, ok := receiveKeyWatcherEntry(t, watcher)
		if !ok {
			t.Fatal("watcher closed during the initial snapshot")
		}
		if entry == nil {
			break
		}
		initial = append(initial, entry.Key())
	}
	slices.Sort(initial)
	if want := []string{"alpha.one", "alpha.two", "beta.one"}; !slices.Equal(initial, want) {
		t.Fatalf("initial snapshot = %v, want %v", initial, want)
	}

	if _, err := kv.Put(ctx, "beta.two", []byte("new")); err != nil {
		t.Fatal(err)
	}
	if _, err := kv.Put(ctx, "gamma.two", []byte("ignored")); err != nil {
		t.Fatal(err)
	}
	entry, ok := receiveKeyWatcherEntry(t, watcher)
	if !ok || entry == nil || entry.Key() != "beta.two" {
		t.Fatalf("live update = %v, %v; want beta.two", entry, ok)
	}
}

func TestWatchKeyFiltersSendsInitialMarkerOnlyAfterEverySource(t *testing.T) {
	ctx := testContext(t)
	slow := &channelKeyWatcher{updates: make(chan jetstream.KeyValueEntry, 1)}
	kv := &recordingWatchKV{KeyValue: newFilteredWatchTestKV(t), override: map[string]jetstream.KeyWatcher{"slow.>": slow}}
	if _, err := kv.Put(ctx, "fast.one", []byte("fast")); err != nil {
		t.Fatal(err)
	}

	watcher, err := watchKeyFilters(ctx, kv, []string{"fast.>", "slow.>"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = watcher.Stop() }()

	entry, ok := receiveKeyWatcherEntry(t, watcher)
	if !ok || entry == nil || entry.Key() != "fast.one" {
		t.Fatalf("first entry = %v, %v; want fast.one", entry, ok)
	}
	select {
	case entry := <-watcher.Updates():
		t.Fatalf("received %v before the slow source finished its initial snapshot", entry)
	case <-time.After(100 * time.Millisecond):
	}

	slow.updates <- nil
	entry, ok = receiveKeyWatcherEntry(t, watcher)
	if !ok || entry != nil {
		t.Fatalf("entry after every initial snapshot = %v, %v; want the nil marker", entry, ok)
	}
}

func TestWatchKeyFiltersClosesWhenAnySourceCloses(t *testing.T) {
	ctx := testContext(t)
	closing := &channelKeyWatcher{updates: make(chan jetstream.KeyValueEntry)}
	kv := &recordingWatchKV{KeyValue: newFilteredWatchTestKV(t), override: map[string]jetstream.KeyWatcher{"closing.>": closing}}

	watcher, err := watchKeyFilters(ctx, kv, []string{"open.>", "closing.>"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = watcher.Stop() }()

	close(closing.updates)
	for {
		entry, ok := receiveKeyWatcherEntry(t, watcher)
		if !ok {
			return
		}
		if entry != nil {
			t.Fatalf("unexpected entry %v", entry.Key())
		}
	}
}

func TestWatchKeyFiltersStopClosesUpdates(t *testing.T) {
	ctx := testContext(t)
	kv := newFilteredWatchTestKV(t)
	watcher, err := watchKeyFilters(ctx, kv, []string{"a.>", "b.>"})
	if err != nil {
		t.Fatal(err)
	}
	if err := watcher.Stop(); err != nil {
		t.Fatal(err)
	}
	for {
		if _, ok := receiveKeyWatcherEntry(t, watcher); !ok {
			return
		}
	}
}
