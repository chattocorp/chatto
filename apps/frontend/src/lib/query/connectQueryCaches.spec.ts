import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { ServerStateStore } from '@chatto/client/server/store';
import type { ProjectionReset, RoomAccessLoss } from '@chatto/client/server/storeEvents';
import { connectQueryCaches, queryCaches } from './cacheRegistry';

type Listener = (...args: never[]) => unknown;

/** A server store that records the listeners of its boundary events. */
function fakeStore() {
  const listeners = new Map<string, Listener[]>();
  const on = (name: string) => (listener: Listener) => {
    listeners.set(name, [...(listeners.get(name) ?? []), listener]);
    return () =>
      listeners.set(
        name,
        (listeners.get(name) ?? []).filter((l) => l !== listener)
      );
  };
  const store = Object.fromEntries(
    [
      'onReset',
      'onSessionEnded',
      'onRoomAccessLost',
      'onUserDeleted',
      'onAuthorityChanged',
      'onPermissionsChanged',
      'onUpdate',
      'onDispose'
    ].map((name) => [name, on(name)])
  );
  const emit = (name: string, ...args: unknown[]) =>
    (listeners.get(name) ?? []).map((listener) =>
      (listener as (...a: unknown[]) => unknown)(...args)
    );
  return {
    store: { serverId: 'S1', ...store } as unknown as ServerStateStore,
    emit,
    count: (name: string) => listeners.get(name)?.length ?? 0
  };
}

const caches = {
  server: {
    remove: vi.fn(),
    refresh: vi.fn(async () => {}),
    removeAdmin: vi.fn(),
    refreshAdmin: vi.fn(),
    refreshRoles: vi.fn(),
    removeAdminUser: vi.fn(),
    reconcileAdminRoomGroups: vi.fn()
  },
  followedThreads: {
    reset: vi.fn(),
    refresh: vi.fn(),
    retractMessage: vi.fn(),
    scrubRoom: vi.fn()
  },
  roomMembers: { purgeRoom: vi.fn(), scrubUser: vi.fn() }
};

beforeEach(() => {
  queryCaches.server = caches.server;
  queryCaches.followedThreads = caches.followedThreads;
  queryCaches.roomMembers = caches.roomMembers;
});

afterEach(() => {
  vi.clearAllMocks();
  delete queryCaches.server;
  delete queryCaches.followedThreads;
  delete queryCaches.roomMembers;
});

describe('connectQueryCaches', () => {
  it('removes private reads at a privacy reset and when the session ends', () => {
    const { store, emit } = fakeStore();
    connectQueryCaches(store);

    emit('onReset', { privacy: false, retainView: true } satisfies ProjectionReset);
    expect(caches.server.remove).not.toHaveBeenCalled();
    expect(caches.followedThreads.reset).not.toHaveBeenCalled();

    emit('onReset', { privacy: true, retainView: false } satisfies ProjectionReset);
    expect(caches.server.remove).toHaveBeenCalledWith('S1');
    expect(caches.followedThreads.reset).toHaveBeenCalledWith('S1');
    expect(caches.server.refreshAdmin).toHaveBeenCalledWith('S1');

    caches.server.remove.mockClear();
    emit('onSessionEnded');
    expect(caches.server.remove).toHaveBeenCalledWith('S1');
  });

  it('fails the reset when the cache cannot remove private reads', () => {
    const { store, emit } = fakeStore();
    connectQueryCaches(store);
    vi.spyOn(console, 'error').mockImplementation(() => {});
    caches.server.remove.mockImplementationOnce(() => {
      throw new Error('cache busy');
    });
    expect(() => emit('onReset', { privacy: true, retainView: false })).toThrow(
      'Query cleanup incomplete'
    );
  });

  it('scrubs reads of a lost room and of a deleted user', () => {
    const { store, emit } = fakeStore();
    connectQueryCaches(store);

    emit('onRoomAccessLost', {
      roomId: 'R1',
      messagesOnly: true,
      removed: false
    } satisfies RoomAccessLoss);
    expect(caches.followedThreads.scrubRoom).toHaveBeenCalledWith('S1', 'R1');
    expect(caches.roomMembers.purgeRoom).not.toHaveBeenCalled();
    emit('onRoomAccessLost', { roomId: 'R1', messagesOnly: false, removed: true });
    expect(caches.roomMembers.purgeRoom).toHaveBeenCalledWith('S1', 'R1');

    emit('onUserDeleted', 'U2');
    expect(caches.roomMembers.scrubUser).toHaveBeenCalledWith('S1', 'U2');
    expect(caches.server.removeAdminUser).toHaveBeenCalledWith('S1', 'U2');
    expect(caches.followedThreads.reset).toHaveBeenCalledWith('S1');
  });

  it('removes admin reads on lost authority, rereads them otherwise, and rechecks permissions', async () => {
    const { store, emit } = fakeStore();
    connectQueryCaches(store);

    emit('onAuthorityChanged', { lost: true });
    expect(caches.server.removeAdmin).toHaveBeenCalledWith('S1');
    emit('onAuthorityChanged', { lost: false });
    expect(caches.server.refreshAdmin).toHaveBeenCalledWith('S1');

    const [refreshed] = emit('onPermissionsChanged');
    await refreshed;
    expect(caches.server.refresh).toHaveBeenCalledWith('S1');
  });

  it('refreshes reads that realtime changes make stale', () => {
    const { store, emit } = fakeStore();
    connectQueryCaches(store);
    const event = (value: object) => ({ event: { event: value } });

    emit('onUpdate', event({ case: 'roleCreated', value: {} }));
    expect(caches.server.refreshRoles).toHaveBeenCalledWith('S1');
    emit('onUpdate', event({ case: 'userProfileChanged', value: { userId: 'U2' } }));
    expect(caches.server.refreshAdmin).toHaveBeenCalledWith('S1');

    emit('onUpdate', event({ case: 'messagePosted', value: { roomId: 'R1' } }));
    expect(caches.followedThreads.refresh).not.toHaveBeenCalled();
    emit('onUpdate', event({ case: 'messagePosted', value: { threadRootEventId: 'T1' } }));
    emit('onUpdate', event({ case: 'threadViewerStateChanged', value: {} }));
    expect(caches.followedThreads.refresh).toHaveBeenCalledTimes(2);

    emit(
      'onUpdate',
      event({ case: 'messageRetracted', value: { roomId: 'R1', messageEventId: 'E-REPLY' } })
    );
    expect(caches.followedThreads.retractMessage).toHaveBeenCalledExactlyOnceWith(
      'S1',
      'R1',
      'E-REPLY'
    );
    expect(caches.followedThreads.reset).not.toHaveBeenCalled();

    emit('onUpdate', {
      event: null,
      resource: { case: 'roomGroups', value: { groups: [{ id: 'G1' }] } }
    });
    expect(caches.server.reconcileAdminRoomGroups).toHaveBeenCalledWith('S1', ['G1']);
  });

  it('removes the reads of a disposed store and stops on request', () => {
    const { store, emit, count } = fakeStore();
    const stop = connectQueryCaches(store);
    emit('onDispose');
    expect(caches.server.remove).toHaveBeenCalledWith('S1');
    stop();
    expect(count('onUpdate')).toBe(0);
  });
});
