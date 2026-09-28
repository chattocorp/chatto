import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { Code, ConnectError } from '@connectrpc/connect';
import { RoomUnreadStore } from '$lib/state/server/roomUnread.svelte';
import { createTestServerScope, type TestServerScope } from '$lib/test-utils/serverScope.svelte';
import Harness from './UseRoomUnreadHarness.svelte';

const mocks = vi.hoisted(() => ({ markRoomAsRead: vi.fn() }));

vi.mock('$lib/api-client/readState', () => ({
  createReadStateAPI: () => ({ markRoomAsRead: mocks.markRoomAsRead })
}));

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

let server: TestServerScope;
let roomUnread: RoomUnreadStore;

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function setPresent(): void {
  window.dispatchEvent(new Event('focus'));
  Object.defineProperty(document, 'visibilityState', {
    value: 'visible',
    writable: true,
    configurable: true
  });
  document.dispatchEvent(new Event('visibilitychange'));
  flushSync();
}

describe('useRoomUnread', () => {
  beforeEach(() => {
    roomUnread = new RoomUnreadStore();
    server = createTestServerScope({ store: { roomUnread } });
    mocks.markRoomAsRead.mockReset();
    setPresent();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('rolls back the optimistic read when the RPC fails', async () => {
    const request = deferred<never>();
    mocks.markRoomAsRead.mockReturnValue(request.promise);
    roomUnread.setRoomUnread('room-1', true);
    vi.spyOn(console, 'error').mockImplementation(() => {});

    const rendered = render(Harness, {
      props: { roomId: 'room-1', onReady: () => {} }
    });
    flushSync();

    await vi.waitFor(() => expect(mocks.markRoomAsRead).toHaveBeenCalledOnce());
    expect(roomUnread.roomIsUnread('room-1')).toBe(false);

    request.reject(new Error('network down'));
    await vi.waitFor(() => expect(roomUnread.roomIsUnread('room-1')).toBe(true));
    rendered.unmount();
  });

  it('aborts an in-flight room read and rolls back on unmount', async () => {
    let requestSignal: AbortSignal | undefined;
    mocks.markRoomAsRead.mockImplementation(
      (_input: unknown, options: { signal?: AbortSignal } = {}) =>
        new Promise((_resolve, reject) => {
          requestSignal = options.signal;
          options.signal?.addEventListener('abort', () => {
            reject(new DOMException('Request canceled', 'AbortError'));
          });
        })
    );
    roomUnread.setRoomUnread('room-1', true);

    const rendered = render(Harness, {
      props: { roomId: 'room-1', onReady: () => {} }
    });
    flushSync();

    await vi.waitFor(() => expect(mocks.markRoomAsRead).toHaveBeenCalledOnce());
    expect(requestSignal?.aborted).toBe(false);
    expect(roomUnread.roomIsUnread('room-1')).toBe(false);

    rendered.unmount();

    await vi.waitFor(() => expect(requestSignal?.aborted).toBe(true));
    await vi.waitFor(() => expect(roomUnread.roomIsUnread('room-1')).toBe(true));
  });

  it('retries a failed room read and clears the unread overlay after success', async () => {
    vi.useFakeTimers();
    mocks.markRoomAsRead
      .mockRejectedValueOnce(new ConnectError('network down', Code.Unavailable))
      .mockResolvedValueOnce({
        lastReadAt: '2026-07-10T20:00:00.000Z',
        previousLastReadAt: null
      });
    roomUnread.setRoomUnread('room-1', true);
    vi.spyOn(console, 'error').mockImplementation(() => {});

    const rendered = render(Harness, {
      props: { roomId: 'room-1', onReady: () => {} }
    });
    flushSync();
    await Promise.resolve();
    await Promise.resolve();

    expect(roomUnread.roomIsUnread('room-1')).toBe(true);
    await vi.advanceTimersByTimeAsync(500);
    flushSync();

    expect(mocks.markRoomAsRead).toHaveBeenCalledTimes(2);
    expect(roomUnread.roomIsUnread('room-1')).toBe(false);
    rendered.unmount();
  });

  it('does not update read state without permission to read messages', async () => {
    roomUnread.setRoomUnread('room-1', true);

    const rendered = render(Harness, {
      props: { roomId: 'room-1', canReadMessages: false, onReady: () => {} }
    });
    flushSync();
    await Promise.resolve();

    expect(mocks.markRoomAsRead).not.toHaveBeenCalled();
    expect(roomUnread.roomIsUnread('room-1')).toBe(true);
    rendered.unmount();
  });

  it('does not mark a saved room as read before viewer verification', async () => {
    server.currentUser.invalidateVerification();
    roomUnread.setRoomUnread('room-1', true);

    const rendered = render(Harness, {
      props: { roomId: 'room-1', onReady: () => {} }
    });
    flushSync();
    await Promise.resolve();

    expect(mocks.markRoomAsRead).not.toHaveBeenCalled();
    expect(roomUnread.roomIsUnread('room-1')).toBe(true);
    rendered.unmount();
  });

  it('places the separator at the first event from another user', async () => {
    vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2026-07-10T20:05:00.000Z'));
    mocks.markRoomAsRead.mockResolvedValue({
      previousLastReadAt: '2026-07-10T20:00:00.000Z',
      lastReadAt: '2026-07-10T20:03:00.000Z'
    });
    let api: ReturnType<typeof import('./useRoomUnread.svelte').useRoomUnread> | undefined;

    const rendered = render(Harness, {
      props: {
        roomId: 'room-1',
        events: [
          { id: 'own-event', actorId: 'viewer-1', createdAt: '2026-07-10T20:01:00.000Z' },
          { id: 'other-event', actorId: 'user-2', createdAt: '2026-07-10T20:02:00.000Z' }
        ],
        onReady: (nextApi) => {
          api = nextApi;
        }
      }
    });
    flushSync();

    await vi.waitFor(() => expect(api?.unreadMarkerEventId).toBe('other-event'));
    rendered.unmount();
  });

  it('preserves a newer unread message when the earlier read succeeds', async () => {
    const request = deferred<{ lastReadAt: string; previousLastReadAt: null }>();
    mocks.markRoomAsRead.mockReturnValue(request.promise);
    roomUnread.setRoomUnread('room-1', true);

    const rendered = render(Harness, {
      props: { roomId: 'room-1', onReady: () => {} }
    });
    flushSync();

    await vi.waitFor(() => expect(mocks.markRoomAsRead).toHaveBeenCalledOnce());
    roomUnread.setRoomUnread('room-1', true);
    request.resolve({ lastReadAt: '2026-07-10T20:00:00.000Z', previousLastReadAt: null });
    await request.promise;
    await Promise.resolve();
    flushSync();

    expect(roomUnread.roomIsUnread('room-1')).toBe(true);
    rendered.unmount();
  });
});
