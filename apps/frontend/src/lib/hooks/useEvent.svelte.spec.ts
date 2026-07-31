import { flushSync } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ProjectionHandler } from '$lib/eventBus.svelte';

const serverScope = $state({ serverId: 'origin' });

const { mocks } = vi.hoisted(() => ({
  mocks: {
    onProjectionEvent: vi.fn(),
    useServerScope: vi.fn()
  }
}));

vi.mock('$lib/eventBus.svelte', () => ({
  onProjectionEvent: mocks.onProjectionEvent,
  onPresenceChange: vi.fn(),
  onSessionTerminated: vi.fn(),
  onTypingEvent: vi.fn()
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: mocks.useServerScope
}));

import { useProjectionEvent } from './useEvent.svelte';

describe('useProjectionEvent', () => {
  beforeEach(() => {
    serverScope.serverId = 'origin';
    mocks.useServerScope.mockReset();
    mocks.useServerScope.mockImplementation(() => serverScope);
    mocks.onProjectionEvent.mockReset();
  });

  it('moves the subscription when the route server scope changes', () => {
    const cleanups: Array<ReturnType<typeof vi.fn>> = [];
    mocks.onProjectionEvent.mockImplementation(() => {
      const cleanup = vi.fn();
      cleanups.push(cleanup);
      return cleanup;
    });

    const dispose = $effect.root(() => {
      useProjectionEvent(vi.fn() as ProjectionHandler);
    });
    flushSync();

    expect(mocks.onProjectionEvent).toHaveBeenCalledOnce();
    expect(mocks.onProjectionEvent.mock.calls[0]?.[0]).toBe('origin');

    serverScope.serverId = 'remote';
    flushSync();

    expect(cleanups[0]).toHaveBeenCalledOnce();
    expect(mocks.onProjectionEvent).toHaveBeenCalledTimes(2);
    expect(mocks.onProjectionEvent.mock.calls[1]?.[0]).toBe('remote');

    dispose();
    expect(cleanups[1]).toHaveBeenCalledOnce();
  });

  it('uses an explicit origin selector without reading route context', () => {
    mocks.onProjectionEvent.mockImplementation(() => vi.fn());

    const dispose = $effect.root(() => {
      useProjectionEvent(vi.fn() as ProjectionHandler, () => 'origin');
    });
    flushSync();

    expect(mocks.useServerScope).not.toHaveBeenCalled();
    expect(mocks.onProjectionEvent.mock.calls[0]?.[0]).toBe('origin');
    dispose();
  });
});
