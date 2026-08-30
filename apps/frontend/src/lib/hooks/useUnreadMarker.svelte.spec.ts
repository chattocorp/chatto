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

function getApi(api: HarnessAPI | undefined): HarnessAPI {
  if (!api) {
    throw new Error('Unread marker harness API was not initialized');
  }
  return api;
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((nextResolve) => {
    resolve = nextResolve;
  });
  return { promise, resolve };
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

  it('recovers foreground state from trusted interaction after pagehide', async () => {
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

  it('retries transient failures with exponential backoff', async () => {
    vi.useFakeTimers();
    const markAsRead = vi
      .fn()
      .mockRejectedValueOnce(new ConnectError('temporarily unavailable', Code.Unavailable))
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
    await vi.advanceTimersByTimeAsync(999);
    expect(markAsRead).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(1);
    expect(markAsRead).toHaveBeenCalledTimes(3);
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

  it('cancels the old retry when the target changes', async () => {
    vi.useFakeTimers();
    const markAsRead = vi
      .fn()
      .mockRejectedValueOnce(new ConnectError('temporarily unavailable', Code.Unavailable))
      .mockResolvedValue(NO_MARKER_RESULT);
    const onReady = () => {};
    const rendered = render(Harness, {
      props: { targetId: 'room-1', markAsRead, onReady }
    });
    flushSync();
    await Promise.resolve();

    const rerendered = rendered.rerender({ targetId: 'room-2', markAsRead, onReady });
    await vi.advanceTimersByTimeAsync(0);
    await rerendered;
    flushSync();
    await Promise.resolve();
    await vi.advanceTimersByTimeAsync(60_000);

    expect(markAsRead.mock.calls.map(([targetId]) => targetId)).toEqual(['room-1', 'room-2']);
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

  it('ignores a stale read-state response after the target changes', async () => {
    const firstRead = deferred<{
      previousLastReadAt: string;
      lastReadAt: string;
    }>();
    let firstSignal: AbortSignal | undefined;
    const markAsRead = vi
      .fn()
      .mockImplementationOnce(
        (_targetId: string, _upToEventId: string | undefined, signal: AbortSignal) => {
          firstSignal = signal;
          return firstRead.promise;
        }
      )
      .mockResolvedValueOnce(NO_MARKER_RESULT);
    let api: HarnessAPI | undefined;
    const onReady = (nextApi: HarnessAPI) => {
      api = nextApi;
    };

    const rendered = render(Harness, {
      props: { targetId: 'room-1', markAsRead, onReady }
    });
    flushSync();
    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledOnce());

    await rendered.rerender({ targetId: 'room-2', markAsRead, onReady });
    flushSync();
    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledTimes(2));
    expect(firstSignal?.aborted).toBe(true);

    firstRead.resolve({
      previousLastReadAt: '2026-07-08T09:00:00.000Z',
      lastReadAt: '2026-07-08T10:00:00.000Z'
    });
    await firstRead.promise;
    flushSync();

    expect(getApi(api).unreadMarkerWindow).toBeNull();
    rendered.unmount();
  });
});
