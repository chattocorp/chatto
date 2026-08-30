import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { Code, ConnectError } from '@connectrpc/connect';
import Harness from './UseUnreadMarkerHarness.svelte';

const NO_MARKER_RESULT = {
  previousLastReadAt: null,
  lastReadAt: null
};

type HarnessAPI = {
  readonly unreadMarkerEventId: string | null;
  readonly unreadMarkerWindow: {
    afterTime: string;
    beforeTime: string | number;
  } | null;
  markAsRead(targetId: string, upToEventId?: string): Promise<unknown>;
  setUnreadMarkerEventId(eventId: string | null): void;
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((nextResolve) => {
    resolve = nextResolve;
  });
  return { promise, resolve };
}

function getApi(api: HarnessAPI | undefined): HarnessAPI {
  if (!api) {
    throw new Error('Unread marker harness API was not initialized');
  }
  return api;
}

function setVisibility(value: DocumentVisibilityState): void {
  Object.defineProperty(document, 'visibilityState', {
    value,
    writable: true,
    configurable: true
  });
  document.dispatchEvent(new Event('visibilitychange'));
}

function setPresent(present: boolean): void {
  window.dispatchEvent(new Event(present ? 'focus' : 'blur'));
  setVisibility(present ? 'visible' : 'hidden');
  flushSync();
}

describe('useUnreadMarker', () => {
  beforeEach(() => {
    setPresent(true);
  });

  afterEach(() => {
    vi.useRealTimers();
    setPresent(true);
    vi.restoreAllMocks();
  });

  it('marks the same target as read again on refocus', async () => {
    const markAsRead = vi.fn().mockResolvedValue(NO_MARKER_RESULT);

    const rendered = render(Harness, {
      props: {
        targetId: 'room-1',
        markAsRead,
        onReady: () => {}
      }
    });
    flushSync();
    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledOnce());

    setPresent(false);
    setPresent(true);

    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledTimes(2));
    expect(markAsRead).toHaveBeenLastCalledWith('room-1', undefined, expect.any(AbortSignal));
    rendered.unmount();
  });

  it('marks a visible target on entry without requiring focus', async () => {
    setVisibility('visible');
    window.dispatchEvent(new Event('blur'));
    flushSync();
    const markAsRead = vi.fn().mockResolvedValue(NO_MARKER_RESULT);

    const rendered = render(Harness, {
      props: { targetId: 'room-1', markAsRead, onReady: () => {} }
    });
    flushSync();

    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledOnce());
    expect(markAsRead).toHaveBeenCalledWith('room-1', undefined, expect.any(AbortSignal));
    rendered.unmount();
  });

  it('marks a hidden target when it becomes visible without a focus event', async () => {
    setVisibility('hidden');
    window.dispatchEvent(new Event('blur'));
    flushSync();
    const markAsRead = vi.fn().mockResolvedValue(NO_MARKER_RESULT);

    const rendered = render(Harness, {
      props: { targetId: 'room-1', markAsRead, onReady: () => {} }
    });
    flushSync();
    await Promise.resolve();
    expect(markAsRead).not.toHaveBeenCalled();

    setVisibility('visible');
    window.dispatchEvent(new Event('blur'));
    flushSync();

    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledOnce());
    rendered.unmount();
  });

  it('marks the target after pageshow and ignores duplicate foreground signals', async () => {
    const markAsRead = vi.fn().mockResolvedValue(NO_MARKER_RESULT);
    const rendered = render(Harness, {
      props: { targetId: 'room-1', markAsRead, onReady: () => {} }
    });
    flushSync();
    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledOnce());

    window.dispatchEvent(new Event('pagehide'));
    window.dispatchEvent(new Event('pageshow'));
    flushSync();
    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledTimes(2));

    window.dispatchEvent(new Event('pageshow'));
    document.dispatchEvent(new Event('resume'));
    setVisibility('visible');
    flushSync();
    await Promise.resolve();
    expect(markAsRead).toHaveBeenCalledTimes(2);
    rendered.unmount();
  });

  it('recovers missing focus state from a visible interaction', async () => {
    const markAsRead = vi.fn().mockResolvedValue(NO_MARKER_RESULT);
    const rendered = render(Harness, {
      props: { targetId: 'room-1', markAsRead, onReady: () => {} }
    });
    flushSync();
    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledOnce());

    window.dispatchEvent(new Event('blur'));
    flushSync();
    await userEvent.click(rendered.container.querySelector('button')!);
    flushSync();

    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledTimes(2));
    rendered.unmount();
  });

  it('recovers foreground state from interaction after pagehide without pageshow', async () => {
    const markAsRead = vi.fn().mockResolvedValue(NO_MARKER_RESULT);
    const rendered = render(Harness, {
      props: { targetId: 'room-1', markAsRead, onReady: () => {} }
    });
    flushSync();
    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledOnce());

    window.dispatchEvent(new Event('pagehide'));
    flushSync();
    await userEvent.click(rendered.container.querySelector('button')!);
    flushSync();

    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledTimes(2));
    rendered.unmount();
  });

  it('retries transient failures with backoff', async () => {
    vi.useFakeTimers();
    const markAsRead = vi
      .fn()
      .mockRejectedValueOnce(new ConnectError('temporarily unavailable', Code.Unavailable))
      .mockResolvedValueOnce(NO_MARKER_RESULT);
    const rendered = render(Harness, {
      props: { targetId: 'room-1', markAsRead, onReady: () => {} }
    });
    flushSync();
    await Promise.resolve();
    expect(markAsRead).toHaveBeenCalledOnce();

    await vi.advanceTimersByTimeAsync(499);
    expect(markAsRead).toHaveBeenCalledOnce();
    await vi.advanceTimersByTimeAsync(1);
    expect(markAsRead).toHaveBeenCalledTimes(2);
    rendered.unmount();
  });

  it('does not retry permanent failures', async () => {
    vi.useFakeTimers();
    const markAsRead = vi
      .fn()
      .mockRejectedValue(new ConnectError('permission denied', Code.PermissionDenied));
    const rendered = render(Harness, {
      props: { targetId: 'room-1', markAsRead, onReady: () => {} }
    });
    flushSync();
    await Promise.resolve();

    await vi.advanceTimersByTimeAsync(60_000);
    expect(markAsRead).toHaveBeenCalledOnce();
    rendered.unmount();
  });

  it('cancels a scheduled retry when the target becomes hidden', async () => {
    vi.useFakeTimers();
    const markAsRead = vi
      .fn()
      .mockRejectedValueOnce(new ConnectError('temporarily unavailable', Code.Unavailable));
    const rendered = render(Harness, {
      props: { targetId: 'room-1', markAsRead, onReady: () => {} }
    });
    flushSync();
    await Promise.resolve();

    setVisibility('hidden');
    flushSync();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(markAsRead).toHaveBeenCalledOnce();
    rendered.unmount();
  });

  it('cancels a scheduled retry on pagehide without visibilitychange', async () => {
    vi.useFakeTimers();
    const markAsRead = vi
      .fn()
      .mockRejectedValueOnce(new ConnectError('temporarily unavailable', Code.Unavailable));
    const rendered = render(Harness, {
      props: { targetId: 'room-1', markAsRead, onReady: () => {} }
    });
    flushSync();
    await Promise.resolve();

    window.dispatchEvent(new Event('pagehide'));
    flushSync();
    await vi.advanceTimersByTimeAsync(60_000);

    expect(markAsRead).toHaveBeenCalledOnce();
    rendered.unmount();
  });

  it('aborts an in-flight request and ignores its response after hiding', async () => {
    const request = deferred<{
      previousLastReadAt: string;
      lastReadAt: string;
    }>();
    let requestSignal: AbortSignal | undefined;
    const markAsRead = vi.fn(
      (_targetId: string, _upToEventId: string | undefined, signal: AbortSignal) => {
        requestSignal = signal;
        return request.promise;
      }
    );
    let api: HarnessAPI | undefined;
    const rendered = render(Harness, {
      props: {
        targetId: 'room-1',
        markAsRead,
        onReady: (nextApi: HarnessAPI) => {
          api = nextApi;
        }
      }
    });
    flushSync();
    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledOnce());

    setVisibility('hidden');
    flushSync();
    expect(requestSignal?.aborted).toBe(true);

    request.resolve({
      previousLastReadAt: '2026-07-08T09:00:00.000Z',
      lastReadAt: '2026-07-08T10:00:00.000Z'
    });
    await request.promise;
    flushSync();

    expect(getApi(api).unreadMarkerWindow).toBeNull();
    rendered.unmount();
  });

  it('retries immediately after the app returns to the foreground', async () => {
    vi.useFakeTimers();
    const markAsRead = vi
      .fn()
      .mockRejectedValueOnce(new ConnectError('temporarily unavailable', Code.Unavailable))
      .mockResolvedValueOnce(NO_MARKER_RESULT);
    const rendered = render(Harness, {
      props: { targetId: 'room-1', markAsRead, onReady: () => {} }
    });
    flushSync();
    await Promise.resolve();
    expect(markAsRead).toHaveBeenCalledOnce();

    window.dispatchEvent(new Event('pagehide'));
    window.dispatchEvent(new Event('pageshow'));
    flushSync();
    await Promise.resolve();

    expect(markAsRead).toHaveBeenCalledTimes(2);
    rendered.unmount();
  });

  it('cancels a scheduled retry on unmount', async () => {
    vi.useFakeTimers();
    const markAsRead = vi
      .fn()
      .mockRejectedValueOnce(new ConnectError('temporarily unavailable', Code.Unavailable));
    const rendered = render(Harness, {
      props: { targetId: 'room-1', markAsRead, onReady: () => {} }
    });
    flushSync();
    await Promise.resolve();

    rendered.unmount();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(markAsRead).toHaveBeenCalledOnce();
  });

  it('cancels the old retry when the target changes', async () => {
    vi.useFakeTimers();
    const markAsRead = vi
      .fn()
      .mockRejectedValueOnce(new ConnectError('temporarily unavailable', Code.Unavailable))
      .mockResolvedValue(NO_MARKER_RESULT);
    const rendered = render(Harness, {
      props: { targetId: 'room-1', markAsRead, onReady: () => {} }
    });
    flushSync();
    await Promise.resolve();

    const rerendered = rendered.rerender({
      targetId: 'room-2',
      markAsRead,
      onReady: () => {}
    });
    await vi.advanceTimersByTimeAsync(0);
    await rerendered;
    flushSync();
    await Promise.resolve();
    await vi.advanceTimersByTimeAsync(60_000);

    expect(markAsRead.mock.calls.map(([targetId, upToEventId]) => [targetId, upToEventId])).toEqual(
      [
        ['room-1', undefined],
        ['room-2', undefined]
      ]
    );
    rendered.unmount();
  });

  it('cancels retries on permission loss and marks after permission returns', async () => {
    vi.useFakeTimers();
    const markAsRead = vi
      .fn()
      .mockRejectedValueOnce(new ConnectError('temporarily unavailable', Code.Unavailable))
      .mockResolvedValue(NO_MARKER_RESULT);
    const onReady = () => {};
    const rendered = render(Harness, {
      props: { targetId: 'room-1', markAsRead, canMarkAsRead: true, onReady }
    });
    flushSync();
    await Promise.resolve();

    const permissionRemoved = rendered.rerender({
      targetId: 'room-1',
      markAsRead,
      canMarkAsRead: false,
      onReady
    });
    await vi.advanceTimersByTimeAsync(0);
    await permissionRemoved;
    flushSync();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(markAsRead).toHaveBeenCalledOnce();

    const permissionRestored = rendered.rerender({
      targetId: 'room-1',
      markAsRead,
      canMarkAsRead: true,
      onReady
    });
    await vi.advanceTimersByTimeAsync(0);
    await permissionRestored;
    flushSync();
    await Promise.resolve();
    expect(markAsRead).toHaveBeenCalledTimes(2);
    rendered.unmount();
  });

  it('retries immediately when the browser comes online', async () => {
    vi.useFakeTimers();
    const markAsRead = vi
      .fn()
      .mockRejectedValueOnce(new ConnectError('temporarily unavailable', Code.Unavailable))
      .mockResolvedValueOnce(NO_MARKER_RESULT);
    const rendered = render(Harness, {
      props: { targetId: 'room-1', markAsRead, onReady: () => {} }
    });
    flushSync();
    await Promise.resolve();
    expect(markAsRead).toHaveBeenCalledOnce();

    window.dispatchEvent(new Event('online'));
    flushSync();
    await Promise.resolve();
    expect(markAsRead).toHaveBeenCalledTimes(2);
    rendered.unmount();
  });

  it('uses the read-state window returned on refocus', async () => {
    const markedAtMs = Date.UTC(2026, 6, 8, 10, 0, 30);
    vi.spyOn(Date, 'now').mockReturnValue(markedAtMs);
    const markAsRead = vi.fn().mockResolvedValueOnce(NO_MARKER_RESULT).mockResolvedValueOnce({
      previousLastReadAt: '2026-07-08T09:00:00.000Z',
      lastReadAt: '2026-07-08T10:00:00.000Z'
    });
    let api: HarnessAPI | undefined;

    const rendered = render(Harness, {
      props: {
        targetId: 'room-1',
        markAsRead,
        onReady: (nextApi: HarnessAPI) => {
          api = nextApi;
        }
      }
    });
    flushSync();
    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledOnce());

    setPresent(false);
    const currentApi = getApi(api);
    setPresent(true);

    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledTimes(2));
    await vi.waitFor(() =>
      expect(currentApi.unreadMarkerWindow).toEqual({
        afterTime: '2026-07-08T09:00:00.000Z',
        beforeTime: markedAtMs
      })
    );
    expect(currentApi.unreadMarkerEventId).toBeNull();
    rendered.unmount();
  });

  it('resolves a marker when eligible timeline events arrive', async () => {
    const markedAtMs = Date.parse('2026-07-08T10:00:30.000Z');
    vi.spyOn(Date, 'now').mockReturnValue(markedAtMs);
    const markAsRead = vi.fn().mockResolvedValue({
      previousLastReadAt: '2026-07-08T09:00:00.000Z',
      lastReadAt: '2026-07-08T10:00:00.000Z'
    });
    let api: HarnessAPI | undefined;
    const onReady = (nextApi: HarnessAPI) => {
      api = nextApi;
    };

    const rendered = render(Harness, {
      props: {
        targetId: 'room-1',
        markAsRead,
        events: [],
        skipActorId: 'current-user',
        onReady
      }
    });
    flushSync();
    const currentApi = getApi(api);
    await vi.waitFor(() => expect(currentApi.unreadMarkerWindow).not.toBeNull());

    await rendered.rerender({
      targetId: 'room-1',
      markAsRead,
      events: [
        {
          id: 'at-lower-bound',
          actorId: 'other-user',
          createdAt: '2026-07-08T09:00:00.000Z'
        },
        {
          id: 'current-users-event',
          actorId: 'current-user',
          createdAt: '2026-07-08T09:15:00.000Z'
        },
        {
          id: 'first-eligible-event',
          actorId: 'other-user',
          createdAt: '2026-07-08T09:30:00.000Z'
        },
        {
          id: 'after-upper-bound',
          actorId: 'other-user',
          createdAt: '2026-07-08T10:01:00.000Z'
        }
      ],
      skipActorId: 'current-user',
      onReady
    });
    flushSync();

    await vi.waitFor(() => expect(currentApi.unreadMarkerEventId).toBe('first-eligible-event'));
    expect(currentApi.unreadMarkerWindow).toBeNull();
    rendered.unmount();
  });

  it('clears the marker when refocus returns no previous read state', async () => {
    const markedAtMs = Date.UTC(2026, 6, 8, 10, 0, 30);
    vi.spyOn(Date, 'now').mockReturnValue(markedAtMs);
    const markAsRead = vi
      .fn()
      .mockResolvedValueOnce({
        previousLastReadAt: '2026-07-08T09:00:00.000Z',
        lastReadAt: '2026-07-08T10:00:00.000Z'
      })
      .mockResolvedValueOnce({
        previousLastReadAt: null,
        lastReadAt: '2026-07-08T10:05:00.000Z'
      });
    let api: HarnessAPI | undefined;

    const rendered = render(Harness, {
      props: {
        targetId: 'room-1',
        markAsRead,
        onReady: (nextApi: HarnessAPI) => {
          api = nextApi;
        }
      }
    });
    flushSync();
    const currentApi = getApi(api);
    await vi.waitFor(() =>
      expect(currentApi.unreadMarkerWindow).toEqual({
        afterTime: '2026-07-08T09:00:00.000Z',
        beforeTime: markedAtMs
      })
    );

    currentApi.setUnreadMarkerEventId('event-2');
    setPresent(false);
    setPresent(true);

    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(currentApi.unreadMarkerWindow).toBeNull());
    expect(currentApi.unreadMarkerEventId).toBeNull();
    rendered.unmount();
  });

  it('does not create a marker window when the read cursor did not advance', async () => {
    const markAsRead = vi.fn().mockResolvedValueOnce({
      previousLastReadAt: '2026-07-08T09:00:00.000Z',
      lastReadAt: '2026-07-08T09:00:00.000Z'
    });
    let api: HarnessAPI | undefined;

    const rendered = render(Harness, {
      props: {
        targetId: 'room-1',
        markAsRead,
        onReady: (nextApi: HarnessAPI) => {
          api = nextApi;
        }
      }
    });
    flushSync();
    const currentApi = getApi(api);

    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledOnce());
    expect(currentApi.unreadMarkerWindow).toBeNull();
    expect(currentApi.unreadMarkerEventId).toBeNull();
    rendered.unmount();
  });

  it('preserves a pending refocus marker after a newer explicit read', async () => {
    const markedAtMs = Date.UTC(2026, 6, 8, 10, 0, 30);
    vi.spyOn(Date, 'now').mockReturnValue(markedAtMs);
    let resolveRefocus!: (value: { previousLastReadAt: string; lastReadAt: string }) => void;
    const refocusRead = new Promise<{
      previousLastReadAt: string;
      lastReadAt: string;
    }>((resolve) => {
      resolveRefocus = resolve;
    });
    const markAsRead = vi
      .fn()
      .mockResolvedValueOnce(NO_MARKER_RESULT)
      .mockReturnValueOnce(refocusRead)
      .mockResolvedValueOnce(NO_MARKER_RESULT);
    let api: HarnessAPI | undefined;

    const rendered = render(Harness, {
      props: {
        targetId: 'room-1',
        markAsRead,
        onReady: (nextApi: HarnessAPI) => {
          api = nextApi;
        }
      }
    });
    flushSync();
    const currentApi = getApi(api);
    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledOnce());

    setPresent(false);
    setPresent(true);
    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledTimes(2));

    await currentApi.markAsRead('room-1', 'event-2');
    expect(markAsRead).toHaveBeenCalledTimes(3);

    resolveRefocus({
      previousLastReadAt: '2026-07-08T09:00:00.000Z',
      lastReadAt: '2026-07-08T10:00:00.000Z'
    });
    await Promise.resolve();
    flushSync();

    expect(currentApi.unreadMarkerWindow).toEqual({
      afterTime: '2026-07-08T09:00:00.000Z',
      beforeTime: markedAtMs
    });
    expect(currentApi.unreadMarkerEventId).toBeNull();
    rendered.unmount();
  });

  it('marks a new target as read when the target changes', async () => {
    const markAsRead = vi.fn().mockResolvedValue(NO_MARKER_RESULT);

    const rendered = render(Harness, {
      props: {
        targetId: 'room-1',
        markAsRead,
        onReady: () => {}
      }
    });
    flushSync();
    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledOnce());

    await rendered.rerender({
      targetId: 'room-2',
      markAsRead,
      onReady: () => {}
    });
    flushSync();

    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledTimes(2));
    expect(markAsRead).toHaveBeenLastCalledWith('room-2', undefined, expect.any(AbortSignal));
    rendered.unmount();
  });

  it('ignores a stale read-state window after the target changes', async () => {
    let resolveFirstRead!: (value: { previousLastReadAt: string; lastReadAt: string }) => void;
    const firstRead = new Promise<{
      previousLastReadAt: string;
      lastReadAt: string;
    }>((resolve) => {
      resolveFirstRead = resolve;
    });
    const markAsRead = vi
      .fn()
      .mockReturnValueOnce(firstRead)
      .mockResolvedValueOnce(NO_MARKER_RESULT);
    let api: HarnessAPI | undefined;
    const onReady = (nextApi: HarnessAPI) => {
      api = nextApi;
    };

    const rendered = render(Harness, {
      props: {
        targetId: 'room-1',
        markAsRead,
        onReady
      }
    });
    flushSync();
    const currentApi = getApi(api);
    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledOnce());

    await rendered.rerender({
      targetId: 'room-2',
      markAsRead,
      onReady
    });
    flushSync();
    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledTimes(2));

    resolveFirstRead({
      previousLastReadAt: '2026-07-08T09:00:00.000Z',
      lastReadAt: '2026-07-08T10:00:00.000Z'
    });
    await firstRead;
    flushSync();

    expect(currentApi.unreadMarkerWindow).toBeNull();
    expect(currentApi.unreadMarkerEventId).toBeNull();
    rendered.unmount();
  });
});
