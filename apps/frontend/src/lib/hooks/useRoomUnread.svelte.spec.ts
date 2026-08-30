import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { Code, ConnectError } from '@connectrpc/connect';
import { RoomUnreadStore } from '$lib/state/server/roomUnread.svelte';
import Harness from './UseRoomUnreadHarness.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    markRoomAsRead: vi.fn(),
    roomUnread: null as RoomUnreadStore | null
  }
}));

vi.mock('$lib/api-client/readState', () => ({
  createReadStateAPI: () => ({ markRoomAsRead: mocks.markRoomAsRead })
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    store: {
      get roomUnread() {
        return mocks.roomUnread;
      }
    },
    connection: {
      serverId: 'server-1',
      connectBaseUrl: '/api/connect',
      bearerToken: 'token',
      getAPI: (factory: (config: never) => unknown) => factory({} as never)
    }
  })
}));

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
    mocks.roomUnread = new RoomUnreadStore();
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
    mocks.roomUnread!.setRoomUnread('room-1', true);
    vi.spyOn(console, 'error').mockImplementation(() => {});

    const rendered = render(Harness, {
      props: { roomId: 'room-1', onReady: () => {} }
    });
    flushSync();

    await vi.waitFor(() => expect(mocks.markRoomAsRead).toHaveBeenCalledOnce());
    expect(mocks.roomUnread!.roomIsUnread('room-1')).toBe(false);

    request.reject(new Error('network down'));
    await vi.waitFor(() => expect(mocks.roomUnread!.roomIsUnread('room-1')).toBe(true));
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
    mocks.roomUnread!.setRoomUnread('room-1', true);

    const rendered = render(Harness, {
      props: { roomId: 'room-1', onReady: () => {} }
    });
    flushSync();

    await vi.waitFor(() => expect(mocks.markRoomAsRead).toHaveBeenCalledOnce());
    expect(requestSignal?.aborted).toBe(false);
    expect(mocks.roomUnread!.roomIsUnread('room-1')).toBe(false);

    rendered.unmount();

    await vi.waitFor(() => expect(requestSignal?.aborted).toBe(true));
    await vi.waitFor(() => expect(mocks.roomUnread!.roomIsUnread('room-1')).toBe(true));
  });

  it('retries a failed room read and clears the unread overlay after success', async () => {
    vi.useFakeTimers();
    mocks.markRoomAsRead
      .mockRejectedValueOnce(new ConnectError('network down', Code.Unavailable))
      .mockResolvedValueOnce({
        lastReadAt: '2026-07-10T20:00:00.000Z',
        previousLastReadAt: null
      });
    mocks.roomUnread!.setRoomUnread('room-1', true);
    vi.spyOn(console, 'error').mockImplementation(() => {});

    const rendered = render(Harness, {
      props: { roomId: 'room-1', onReady: () => {} }
    });
    flushSync();
    await Promise.resolve();
    await Promise.resolve();

    expect(mocks.roomUnread!.roomIsUnread('room-1')).toBe(true);
    await vi.advanceTimersByTimeAsync(500);
    flushSync();

    expect(mocks.markRoomAsRead).toHaveBeenCalledTimes(2);
    expect(mocks.roomUnread!.roomIsUnread('room-1')).toBe(false);
    rendered.unmount();
  });

  it('does not update read state without permission to read messages', async () => {
    mocks.roomUnread!.setRoomUnread('room-1', true);

    const rendered = render(Harness, {
      props: { roomId: 'room-1', canReadMessages: false, onReady: () => {} }
    });
    flushSync();
    await Promise.resolve();

    expect(mocks.markRoomAsRead).not.toHaveBeenCalled();
    expect(mocks.roomUnread!.roomIsUnread('room-1')).toBe(true);
    rendered.unmount();
  });

  it('preserves a newer unread message when the earlier read succeeds', async () => {
    const request = deferred<{ lastReadAt: string; previousLastReadAt: null }>();
    mocks.markRoomAsRead.mockReturnValue(request.promise);
    mocks.roomUnread!.setRoomUnread('room-1', true);

    const rendered = render(Harness, {
      props: { roomId: 'room-1', onReady: () => {} }
    });
    flushSync();

    await vi.waitFor(() => expect(mocks.markRoomAsRead).toHaveBeenCalledOnce());
    mocks.roomUnread!.setRoomUnread('room-1', true);
    request.resolve({ lastReadAt: '2026-07-10T20:00:00.000Z', previousLastReadAt: null });
    await request.promise;
    await Promise.resolve();
    flushSync();

    expect(mocks.roomUnread!.roomIsUnread('room-1')).toBe(true);
    rendered.unmount();
  });
});
