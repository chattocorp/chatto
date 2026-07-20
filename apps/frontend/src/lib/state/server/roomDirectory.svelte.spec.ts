import { describe, it, expect, vi } from 'vitest';
import { flushSync } from 'svelte';
import {
  RoomDirectoryScope,
  RoomKind,
  type DirectoryRoomSummary,
  type RoomDirectoryAPI
} from '$lib/api-client/roomDirectory';
import { PresenceStatus } from '$lib/api-client/renderTypes';
import type { MemberDirectoryAPI } from '$lib/api-client/memberDirectory';
import { RoomDirectoryStore, type DirectoryRoom } from './roomDirectory.svelte';
import type { RoomCommandAPI } from '$lib/api-client/rooms';

function makeRoom(id: string, overrides: Partial<DirectoryRoomSummary> = {}): DirectoryRoomSummary {
  return {
    id,
    name: overrides.name ?? id,
    description: overrides.description ?? null,
    kind: overrides.kind ?? RoomKind.CHANNEL,
    archived: overrides.archived ?? false,
    isUniversal: overrides.isUniversal ?? false,
    isMember: overrides.isMember ?? false,
    hasUnread: overrides.hasUnread ?? false,
    canJoinRoom: overrides.canJoinRoom ?? true,
    canManageRoom: overrides.canManageRoom ?? false
  };
}

function makeRoomDirectoryAPI(
  rooms: DirectoryRoomSummary[] = []
): Pick<RoomDirectoryAPI, 'listRooms'> {
  return {
    listRooms: vi.fn().mockResolvedValue(rooms)
  };
}

function makeMemberDirectoryAPI(): Pick<MemberDirectoryAPI, 'listRoomMembers'> {
  return {
    listRoomMembers: vi.fn().mockResolvedValue({ members: [], totalCount: 0, hasMore: false })
  };
}

function roomAPI(
  overrides: Partial<Pick<RoomCommandAPI, 'joinRoom' | 'leaveRoom' | 'joinGroup'>> = {}
): Pick<RoomCommandAPI, 'joinRoom' | 'leaveRoom' | 'joinGroup'> {
  return {
    joinRoom: vi.fn().mockResolvedValue(null),
    leaveRoom: vi.fn().mockResolvedValue(true),
    joinGroup: vi.fn().mockResolvedValue([]),
    ...overrides
  };
}

function makeStore({
  roomDirectoryAPI = makeRoomDirectoryAPI(),
  memberDirectoryAPI = makeMemberDirectoryAPI(),
  commands = roomAPI()
}: {
  roomDirectoryAPI?: Pick<RoomDirectoryAPI, 'listRooms'>;
  memberDirectoryAPI?: Pick<MemberDirectoryAPI, 'listRoomMembers'>;
  commands?: Pick<RoomCommandAPI, 'joinRoom' | 'leaveRoom' | 'joinGroup'>;
} = {}) {
  return new RoomDirectoryStore(roomDirectoryAPI, memberDirectoryAPI, commands);
}

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

describe('RoomDirectoryStore - initial load', () => {
  it('populates allRooms and clears isLoading', async () => {
    const roomDirectoryAPI = makeRoomDirectoryAPI([
      makeRoom('r1', { description: 'Lobby' }),
      makeRoom('r2', { archived: true })
    ]);
    const store = makeStore({ roomDirectoryAPI });

    expect(store.isLoading).toBe(true);
    void store.refresh();
    await settle();

    expect(roomDirectoryAPI.listRooms).toHaveBeenCalledWith(RoomDirectoryScope.CHANNELS);
    // Both rooms (archived + non-archived) are stored. Filtering is a
    // presentation concern because Browse Rooms needs archived state.
    expect(store.allRooms).toMatchObject([
      { id: 'r1', description: 'Lobby', archived: false },
      { id: 'r2', archived: true }
    ]);
    expect(store.isLoading).toBe(false);
  });

  it('replaces allRooms with an empty list when Connect returns no rooms', async () => {
    const store = makeStore({ roomDirectoryAPI: makeRoomDirectoryAPI([]) });
    store.allRooms = [directoryRoom('stale')];

    await store.refresh();

    expect(store.allRooms).toEqual([]);
    expect(store.isLoading).toBe(false);
  });
});

describe('RoomDirectoryStore - join preview', () => {
  it('returns five sampled members and the exact total from ListMembers', async () => {
    const memberDirectoryAPI = makeMemberDirectoryAPI();
    vi.mocked(memberDirectoryAPI.listRoomMembers).mockResolvedValue({
      members: [
        {
          id: 'u1',
          login: 'alice',
          displayName: 'Alice',
          deleted: false,
          avatarUrl: null,
          presenceStatus: PresenceStatus.Offline,
          customStatus: null,
          roles: [],
          createdAt: null
        }
      ],
      totalCount: 12,
      hasMore: true
    });
    const store = makeStore({ memberDirectoryAPI });

    await expect(store.loadJoinPreview('r1')).resolves.toMatchObject({
      memberCount: 12,
      sampleMembers: [{ id: 'u1', displayName: 'Alice' }]
    });
    expect(memberDirectoryAPI.listRoomMembers).toHaveBeenCalledWith('r1', '', 5, 0);
  });

  it('returns no preview when the member listing fails', async () => {
    const memberDirectoryAPI = makeMemberDirectoryAPI();
    vi.mocked(memberDirectoryAPI.listRoomMembers).mockRejectedValue(new Error('offline'));
    const store = makeStore({ memberDirectoryAPI });

    await expect(store.loadJoinPreview('r1')).resolves.toBeNull();
  });
});

describe('RoomDirectoryStore - isJoined predicate', () => {
  it('returns true when the room is in the joined set', async () => {
    const store = makeStore();
    void store.refresh();
    await settle();

    expect(store.isJoined('r1', new Set(['r1']))).toBe(true);
    expect(store.isJoined('r2', new Set(['r1']))).toBe(false);
  });

  it('returns true for an optimistically-just-joined room even if not in the joined set yet', async () => {
    const store = makeStore();
    void store.refresh();
    await settle();

    store.justJoinedIds.add('r1');
    expect(store.isJoined('r1', new Set())).toBe(true);
  });

  it('returns false for an optimistically-just-left room even if still in the joined set', async () => {
    const store = makeStore();
    void store.refresh();
    await settle();

    store.justLeftIds.add('r1');
    expect(store.isJoined('r1', new Set(['r1']))).toBe(false);
  });

  it('justLeft takes precedence over justJoined when both are set', async () => {
    const store = makeStore();
    void store.refresh();
    await settle();

    store.justJoinedIds.add('r1');
    store.justLeftIds.add('r1');
    expect(store.isJoined('r1', new Set())).toBe(false);
  });
});

describe('RoomDirectoryStore - joinRoom', () => {
  it('marks joining during the request and just-joined on success', async () => {
    const store = makeStore({
      roomDirectoryAPI: makeRoomDirectoryAPI([makeRoom('r1', { name: 'general' })])
    });
    void store.refresh();
    await settle();

    const promise = store.joinRoom('r1');
    expect(store.joiningIds.has('r1')).toBe(true);

    const result = await promise;
    expect(result.ok).toBe(true);
    if (result.ok) expect(result.room?.name).toBe('general');
    expect(store.joiningIds.has('r1')).toBe(false);
    expect(store.justJoinedIds.has('r1')).toBe(true);
  });

  it('returns an error result and does not set just-joined when the mutation fails', async () => {
    const store = makeStore({
      roomDirectoryAPI: makeRoomDirectoryAPI([makeRoom('r1')]),
      commands: roomAPI({ joinRoom: vi.fn().mockRejectedValue(new Error('permission denied')) })
    });
    void store.refresh();
    await settle();

    const result = await store.joinRoom('r1');
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error.message).toBe('permission denied');
    expect(store.joiningIds.has('r1')).toBe(false);
    expect(store.justJoinedIds.has('r1')).toBe(false);
  });

  it('clears a stale justLeft when the user re-joins', async () => {
    const store = makeStore({ roomDirectoryAPI: makeRoomDirectoryAPI([makeRoom('r1')]) });
    void store.refresh();
    await settle();

    store.justLeftIds.add('r1');
    await store.joinRoom('r1');

    expect(store.justJoinedIds.has('r1')).toBe(true);
    expect(store.justLeftIds.has('r1')).toBe(false);
  });
});

describe('RoomDirectoryStore - leaveRoom', () => {
  it('marks leaving during the request and just-left on success, clearing justJoined', async () => {
    const store = makeStore({ roomDirectoryAPI: makeRoomDirectoryAPI([makeRoom('r1')]) });
    void store.refresh();
    await settle();

    store.justJoinedIds.add('r1');
    const promise = store.leaveRoom('r1');
    expect(store.leavingIds.has('r1')).toBe(true);

    const result = await promise;
    expect(result.ok).toBe(true);
    expect(store.leavingIds.has('r1')).toBe(false);
    expect(store.justLeftIds.has('r1')).toBe(true);
    expect(store.justJoinedIds.has('r1')).toBe(false);
  });

  it('returns an error result on failure', async () => {
    const store = makeStore({
      roomDirectoryAPI: makeRoomDirectoryAPI([makeRoom('r1')]),
      commands: roomAPI({ leaveRoom: vi.fn().mockRejectedValue(new Error('cannot leave')) })
    });
    void store.refresh();
    await settle();

    const result = await store.leaveRoom('r1');
    expect(result.ok).toBe(false);
    expect(store.leavingIds.has('r1')).toBe(false);
    expect(store.justLeftIds.has('r1')).toBe(false);
  });
});

describe('RoomDirectoryStore - refresh clears optimistic state', () => {
  it('refresh clears just-* sets so the authoritative joined membership wins', async () => {
    const store = makeStore({ roomDirectoryAPI: makeRoomDirectoryAPI([makeRoom('r1')]) });
    void store.refresh();
    await settle();

    store.justJoinedIds.add('r1');
    store.justLeftIds.add('r2');

    await store.refresh();
    await settle();

    expect(store.justJoinedIds.size).toBe(0);
    expect(store.justLeftIds.size).toBe(0);
  });
});

describe('RoomDirectoryStore - concurrent refresh guard', () => {
  it('discards out-of-order responses', async () => {
    let resolveFirst!: (value: DirectoryRoomSummary[]) => void;
    let resolveSecond!: (value: DirectoryRoomSummary[]) => void;

    const listRooms = vi
      .fn()
      .mockImplementationOnce(() => new Promise((resolve) => (resolveFirst = resolve)))
      .mockImplementationOnce(() => new Promise((resolve) => (resolveSecond = resolve)));
    const roomDirectoryAPI = { listRooms } as unknown as Pick<RoomDirectoryAPI, 'listRooms'>;

    const store = makeStore({ roomDirectoryAPI });
    void store.refresh();
    void store.refresh();

    resolveSecond([makeRoom('newer')]);
    await settle();

    expect(store.allRooms.map((r) => r.id)).toEqual(['newer']);

    resolveFirst([makeRoom('older')]);
    await settle();

    expect(store.allRooms.map((r) => r.id)).toEqual(['newer']);
  });
});

function directoryRoom(id: string): DirectoryRoom {
  return {
    id,
    name: id,
    description: null,
    archived: false,
    isUniversal: false,
    viewerCanJoinRoom: true
  };
}
