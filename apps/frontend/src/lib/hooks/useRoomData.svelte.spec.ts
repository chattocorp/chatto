import { flushSync } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PresenceStatus } from '$lib/render/types';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { useRoomData } from './useRoomData.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    store: undefined as unknown as {
      realtimeSync: {
        phase: 'empty' | 'hydrating' | 'ready' | 'stale';
        hasUsableProjection: boolean;
      };
      projection: { rooms: SvelteMap<string, unknown> };
      projectedMembersForRoom: ReturnType<typeof vi.fn>;
      currentUser: { user: { id: string } | undefined };
      serverInfo: { name: string };
    }
  }
}));

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({ serverId: 'server-1' })
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    tryGetStore: () => mocks.store
  }
}));

function projectedRoom(roomId: string, kind = RoomKind.DM) {
  return {
    room: {
      room: {
        id: roomId,
        name: roomId,
        description: '',
        kind,
        archived: false,
        universal: false
      },
      viewerState: {
        isMember: true,
        hasUnread: false,
        permissions: [
          { permission: 'message.post', granted: true },
          { permission: 'message.post-in-thread', granted: true },
          { permission: 'message.attach', granted: true },
          { permission: 'message.react', granted: true }
        ]
      }
    }
  };
}

function member(id: string) {
  return {
    id,
    login: id,
    displayName: `User ${id}`,
    deleted: false,
    avatarUrl: null,
    presenceStatus: PresenceStatus.Online
  };
}

describe('useRoomData projection selector', () => {
  beforeEach(() => {
    const realtimeSync = $state({
      phase: 'empty' as 'empty' | 'hydrating' | 'ready' | 'stale',
      get hasUsableProjection() {
        return this.phase === 'ready' || this.phase === 'stale';
      }
    });
    mocks.store = {
      realtimeSync,
      projection: { rooms: new SvelteMap() },
      projectedMembersForRoom: vi.fn((roomId: string) => [member(roomId)]),
      currentUser: { user: { id: 'viewer' } },
      serverInfo: { name: 'Test Server' }
    };
  });

  afterEach(() => vi.restoreAllMocks());

  it('keeps the honest loading state until the server projection is usable', () => {
    let room!: ReturnType<typeof useRoomData>;
    const destroy = $effect.root(() => {
      room = useRoomData(() => ({ roomId: 'dm-a' }));
      flushSync();
    });

    try {
      expect(room.roomData).toBeUndefined();
      expect(room.isRoomLoading).toBe(true);
    } finally {
      destroy();
    }
  });

  it('switches rooms and DM participants synchronously from retained projection state', () => {
    mocks.store.projection.rooms.set('dm-a', projectedRoom('dm-a'));
    mocks.store.projection.rooms.set('dm-b', projectedRoom('dm-b'));
    mocks.store.realtimeSync.phase = 'ready';

    let room!: ReturnType<typeof useRoomData>;
    let switchRoom!: (roomId: string) => void;
    const destroy = $effect.root(() => {
      let roomId = $state('dm-a');
      room = useRoomData(() => ({ roomId }));
      switchRoom = (nextRoomId) => {
        roomId = nextRoomId;
        flushSync();
      };
      flushSync();
    });

    try {
      expect(room.roomData?.room.id).toBe('dm-a');
      expect(room.roomData?.canPostMessage).toBe(true);
      expect(room.roomData?.canPostInThread).toBe(true);
      expect(room.roomData?.canAttach).toBe(true);
      expect(room.roomData?.canReact).toBe(true);
      expect(room.dmData?.participants[0]?.id).toBe('dm-a');
      expect(room.isRoomLoading).toBe(false);

      switchRoom('dm-b');

      expect(room.roomData?.room.id).toBe('dm-b');
      expect(room.dmData?.participants[0]?.id).toBe('dm-b');
      expect(room.isRoomLoading).toBe(false);
    } finally {
      destroy();
    }
  });

  it('renders known stale rooms but waits for caught_up before declaring absence', () => {
    mocks.store.projection.rooms.set('known', projectedRoom('known', RoomKind.CHANNEL));
    mocks.store.realtimeSync.phase = 'stale';
    let known!: ReturnType<typeof useRoomData>;
    let missing!: ReturnType<typeof useRoomData>;
    const destroy = $effect.root(() => {
      known = useRoomData(() => ({ roomId: 'known' }));
      missing = useRoomData(() => ({ roomId: 'missing' }));
      flushSync();
    });

    try {
      expect(known.roomData?.room.id).toBe('known');
      expect(missing.roomData).toBeUndefined();
    } finally {
      destroy();
    }
  });

  it('reports a missing room only after hydration has completed', () => {
    mocks.store.realtimeSync.phase = 'ready';
    let room!: ReturnType<typeof useRoomData>;
    const destroy = $effect.root(() => {
      room = useRoomData(() => ({ roomId: 'missing' }));
      flushSync();
    });

    try {
      expect(room.roomData).toBeNull();
      expect(room.isRoomLoading).toBe(false);
    } finally {
      destroy();
    }
  });
});
