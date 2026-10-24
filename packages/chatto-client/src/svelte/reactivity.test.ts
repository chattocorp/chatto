import { flushSync } from 'svelte';
import { describe, expect, it } from 'vitest';
import { computed, ReactiveMap, signal } from '../reactivity/index.js';
import { MessagesStore } from '../room/messages/MessagesStore.js';
import type { RoomTimelineAPI } from '../api/roomTimeline.js';
import type { ServerConnection } from '../server/serverConnection.js';
import './index.js';
import { observe } from './test-harness.svelte.js';

describe('Svelte reactivity bridge', () => {
  it('reruns Svelte effects after client signals change', () => {
    const count = signal(1);
    const observed = observe(() => count.get());
    count.set(2);
    flushSync();
    expect(observed.values).toEqual([1, 2]);
    observed.stop();
  });

  it('tracks computed values and per-key map reads', () => {
    const map = new ReactiveMap([['a', 1]]);
    const doubled = computed(() => (map.get('a') ?? 0) * 2);
    const observed = observe(() => doubled.get());
    map.set('b', 5);
    flushSync();
    map.set('a', 2);
    flushSync();
    expect(observed.values).toEqual([2, 4]);
    observed.stop();
  });

  it('works inside $derived', () => {
    const count = signal(1);
    const observed = observe(() => count.get(), { derived: true });
    count.set(3);
    flushSync();
    expect(observed.values).toEqual([1, 3]);
    observed.stop();
  });

  it('creates a timeline store inside $derived without unsafe state mutations', () => {
    const errors: unknown[] = [];
    const original = console.error;
    console.error = (...args: unknown[]) => errors.push(args);
    const timeline = {
      getThreadEvents: () => new Promise(() => {})
    } as unknown as RoomTimelineAPI;
    try {
      const observed = observe(
        () =>
          new MessagesStore(
            {} as ServerConnection,
            () => null,
            { roomId: 'room', threadRootEventId: 'root' },
            timeline
          ).isInitialLoading,
        { derived: true }
      );
      expect(observed.values).toEqual([true]);
      observed.stop();
    } finally {
      console.error = original;
    }
    expect(errors).toEqual([]);
  });

  it('stops client subscriptions after Svelte effects are destroyed', async () => {
    const count = signal(1);
    const observed = observe(() => count.get());
    observed.stop();
    await Promise.resolve();
    expect(count.observers.size).toBe(0);
  });
});
