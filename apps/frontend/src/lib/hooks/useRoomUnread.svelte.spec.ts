import { beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
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

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
    serverId: 'server-1',
    connectBaseUrl: '/api/connect',
    bearerToken: 'token'
  })
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'server-1'
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    getStore: () => ({ roomUnread: mocks.roomUnread })
  }
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
