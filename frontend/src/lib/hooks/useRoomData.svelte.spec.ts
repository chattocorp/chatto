import { flushSync } from 'svelte';
import type { Client } from '@urql/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PresenceStatus, RoomType } from '$lib/gql/graphql';
import { useRoomData } from './useRoomData.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    client: undefined as Client | undefined,
    reconnect: { count: 0 }
  }
}));

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({ client: mocks.client })
}));

vi.mock('$lib/hooks/useEvent.svelte', () => ({
  useActiveRoomLayoutUpdated: () => undefined
}));

vi.mock('$lib/hooks/useReconnectCallback.svelte', () => ({
  useReconnectTrigger: () => mocks.reconnect
}));

type QueryResult = {
  data?: unknown;
  error?: unknown;
};

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

function operationName(document: unknown): string {
  const definition = (document as { definitions?: Array<{ name?: { value?: string } }> }).definitions?.find(
    (candidate) => candidate.name?.value
  );
  return definition?.name?.value ?? 'unknown';
}

function roomQueryResult(roomId: string): QueryResult {
  return {
    data: {
      room: {
        id: roomId,
        name: roomId,
        description: null,
        type: RoomType.Dm,
        isUniversal: false,
        viewerCanPostMessage: true,
        viewerCanPostInThread: true,
        viewerCanAttach: true,
        viewerCanReact: true,
        viewerCanManageOthersMessage: false,
        viewerCanEchoMessage: false,
        viewerCanManageRoom: false,
        viewerCanBanRoomMembers: false
      },
      server: {
        profile: { name: 'Test Server' },
        viewerCanManageRooms: false
      }
    }
  };
}

function dmMembersResult(roomId: string, userId: string, displayName: string): QueryResult {
  return {
    data: {
      room: {
        id: roomId,
        members: {
          users: [
            {
              id: userId,
              login: userId,
              displayName,
              avatarUrl: null,
              presenceStatus: PresenceStatus.Online
            }
          ],
          totalCount: 1,
          hasMore: false
        }
      },
      viewer: {
        user: { id: 'viewer' }
      }
    }
  };
}

describe('useRoomData', () => {
  let pendingDmQueries: Map<string, Deferred<QueryResult>>;

  beforeEach(() => {
    mocks.reconnect = { count: 0 };
    pendingDmQueries = new Map();
    mocks.client = {
      query: vi.fn().mockImplementation((document: unknown, variables: { roomId: string }) => {
        const name = operationName(document);

        if (name === 'GetRoom') {
          return { toPromise: () => Promise.resolve(roomQueryResult(variables.roomId)) };
        }

        if (name === 'GetDMRoomMembers') {
          const pending = deferred<QueryResult>();
          pendingDmQueries.set(variables.roomId, pending);
          return { toPromise: () => pending.promise };
        }

        throw new Error(`Unexpected query: ${name}`);
      }),
      mutation: vi.fn(),
      subscription: vi.fn()
    } as unknown as Client;
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

      pendingDmQueries.get('dm-b')?.resolve(dmMembersResult('dm-b', 'user-b', 'User B'));
      await vi.waitFor(() => expect(harness.room.dmData?.participants[0]?.id).toBe('user-b'));

      pendingDmQueries.get('dm-a')?.resolve(dmMembersResult('dm-a', 'user-a', 'User A'));
      await new Promise((resolve) => setTimeout(resolve, 0));

      expect(harness.room.dmData?.participants[0]?.id).toBe('user-b');
    } finally {
      destroy();
    }
  });
});
