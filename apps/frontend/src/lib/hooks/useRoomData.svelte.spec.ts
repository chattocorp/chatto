import { flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PresenceStatus } from '$lib/render/types';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { useRoomData } from './useRoomData.svelte';
import { createMemberDirectoryAPI } from '@chatto/api-client/memberDirectory';
import { createRoomDirectoryAPI } from '@chatto/api-client/roomDirectory';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    reconnect: { count: 0 },
    getRoom: vi.fn(),
    listRoomMembers: vi.fn()
  }
}));

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
    connectBaseUrl: '/api/connect',
    bearerToken: null,
    serverId: 'server-1'
  })
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    tryGetStore: () => ({
      currentUser: { user: { id: 'viewer' } },
      serverInfo: { name: 'Test Server' }
    })
  }
}));

vi.mock('@chatto/api-client/roomDirectory', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@chatto/api-client/roomDirectory')>();
  return {
    ...actual,
    createRoomDirectoryAPI: vi.fn(() => ({
      getRoom: mocks.getRoom
    }))
  };
});

vi.mock('@chatto/api-client/memberDirectory', () => ({
  createMemberDirectoryAPI: vi.fn(() => ({
    listRoomMembers: mocks.listRoomMembers
  }))
}));

vi.mock('$lib/hooks/useEvent.svelte', () => ({
  useActiveRoomLayoutUpdated: () => undefined
}));

vi.mock('$lib/hooks/useReconnectCallback.svelte', () => ({
  useReconnectTrigger: () => mocks.reconnect
}));

type Deferred<T> = {
  promise: Promise<T>;
  resolve: (value: T) => void;
  reject: (reason?: unknown) => void;
};

function deferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function roomDetails(roomId: string) {
  return {
    id: roomId,
    name: roomId,
    description: null,
    kind: RoomKind.DM,
    archived: false,
    isUniversal: false,
    isMember: true,
    hasUnread: false,
    canJoinRoom: false,
    canPostMessage: true,
    canPostInThread: true,
    canAttach: true,
    canReact: true,
    canEchoMessage: false,
    canManageOthersMessage: false,
    canManageRoom: false,
    canBanRoomMembers: false
  };
}

function dmMembersResult(userId: string, displayName: string) {
  return {
    members: [
      {
        id: userId,
        login: userId,
        displayName,
        deleted: false,
        avatarUrl: null,
        presenceStatus: PresenceStatus.Online,
        customStatus: null,
        roles: [],
        createdAt: null
      }
    ],
    totalCount: 1,
    hasMore: false
  };
}

type DmMembersResult = ReturnType<typeof dmMembersResult>;

describe('useRoomData', () => {
  let pendingDmQueries: Map<string, Deferred<DmMembersResult>>;

  beforeEach(() => {
    mocks.reconnect = { count: 0 };
    mocks.getRoom.mockReset();
    mocks.listRoomMembers.mockReset();
    pendingDmQueries = new Map();
    mocks.getRoom.mockImplementation((roomId: string) => Promise.resolve(roomDetails(roomId)));
    mocks.listRoomMembers.mockImplementation((roomId: string) => {
      const pending = deferred<DmMembersResult>();
      pendingDmQueries.set(roomId, pending);
      return pending.promise;
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('ignores stale DM member responses after switching rooms', async () => {
    let harness!: {
      room: ReturnType<typeof useRoomData>;
      switchRoom: (nextRoomId: string) => void;
    };
    const destroy = $effect.root(() => {
      let roomId = $state('dm-a');
      const room = useRoomData(() => ({ roomId }));

      flushSync();

      harness = {
        room,
        switchRoom(nextRoomId: string) {
          roomId = nextRoomId;
          flushSync();
        }
      };
    });

    try {
      await vi.waitFor(() => expect(pendingDmQueries.has('dm-a')).toBe(true));
      harness.switchRoom('dm-b');
      await vi.waitFor(() => expect(pendingDmQueries.has('dm-b')).toBe(true));

      expect(createRoomDirectoryAPI).toHaveBeenCalledWith({
        serverId: 'server-1',
        baseUrl: '/api/connect',
        bearerToken: null
      });
      expect(createMemberDirectoryAPI).toHaveBeenCalledWith({
        baseUrl: '/api/connect',
        bearerToken: null
      });

      pendingDmQueries.get('dm-b')?.resolve(dmMembersResult('user-b', 'User B'));
      await vi.waitFor(() => expect(harness.room.dmData?.participants[0]?.id).toBe('user-b'));

      pendingDmQueries.get('dm-a')?.resolve(dmMembersResult('user-a', 'User A'));
      await new Promise((resolve) => setTimeout(resolve, 0));

      expect(harness.room.dmData?.participants[0]?.id).toBe('user-b');
    } finally {
      destroy();
    }
  });
});
