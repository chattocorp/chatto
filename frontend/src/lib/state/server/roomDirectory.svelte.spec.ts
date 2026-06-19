import { beforeEach, describe, it, expect, vi } from 'vitest';
import { flushSync } from 'svelte';
import type { RoomEventViewFragment } from '$lib/chatTypes';
import {
  RoomDirectoryStore,
  type DirectoryRoom,
  type RoomDirectoryWireQueries
} from './roomDirectory.svelte';

const SPACE_ID = 's_main';

const wireMocks = vi.hoisted(() => ({
  joinRoom: vi.fn(),
  leaveRoom: vi.fn(),
  joinGroup: vi.fn()
}));

vi.mock('$lib/wire', () => ({
  tryWireJoinRoom: wireMocks.joinRoom,
  tryWireLeaveRoom: wireMocks.leaveRoom,
  tryWireJoinGroup: wireMocks.joinGroup
}));

function makeRoom(id: string, overrides: Partial<DirectoryRoom> = {}): DirectoryRoom {
  return {
    id,
    name: overrides.name ?? id,
    description: overrides.description ?? null,
    archived: overrides.archived ?? false,
    viewerCanJoinRoom: overrides.viewerCanJoinRoom ?? true
  };
}

function makeWireQueries(opts: { rooms?: DirectoryRoom[] | null }) {
  const listRoomsMock = vi.fn(() => Promise.resolve(opts.rooms ?? null));
  const wireQueries: RoomDirectoryWireQueries = { listRooms: listRoomsMock };
  return { wireQueries, listRoomsMock };
}

function makeStore(wireQueries: RoomDirectoryWireQueries): RoomDirectoryStore {
  return new RoomDirectoryStore(SPACE_ID, undefined, wireQueries);
}

beforeEach(() => {
  wireMocks.joinRoom.mockReset().mockResolvedValue(true);
  wireMocks.leaveRoom.mockReset().mockResolvedValue(true);
  wireMocks.joinGroup.mockReset().mockResolvedValue(['r1', 'r2']);
});

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

describe('RoomDirectoryStore — initial load', () => {
  it('populates allRooms and clears isLoading', async () => {
    const { wireQueries } = makeWireQueries({
      rooms: [makeRoom('r1'), makeRoom('r2', { archived: true })]
    });
    const store = makeStore(wireQueries);

    expect(store.isLoading).toBe(true);
    void store.refresh();
    await settle();

    // Both rooms (archived + non-archived) are stored — the directory
    // surfaces archived state to UI but the store keeps them. Filtering is
    // a presentation concern.
    expect(store.allRooms.map((r) => r.id)).toEqual(['r1', 'r2']);
    expect(store.isLoading).toBe(false);
  });

  it('keeps allRooms unchanged when the query returns no data', async () => {
    const { wireQueries } = makeWireQueries({ rooms: null });
    const store = makeStore(wireQueries);
    void store.refresh();
    await settle();

    expect(store.allRooms).toEqual([]);
    expect(store.isLoading).toBe(false);
  });
});

describe('RoomDirectoryStore — isJoined predicate', () => {
  it('returns true when the room is in the joined set', async () => {
    const { wireQueries } = makeWireQueries({ rooms: null });
    const store = makeStore(wireQueries);
    void store.refresh();
    await settle();

    expect(store.isJoined('r1', new Set(['r1']))).toBe(true);
    expect(store.isJoined('r2', new Set(['r1']))).toBe(false);
  });

  it('returns true for an optimistically-just-joined room even if not in the joined set yet', async () => {
    const { wireQueries } = makeWireQueries({ rooms: null });
    const store = makeStore(wireQueries);
    void store.refresh();
    await settle();

    store.justJoinedIds.add('r1');
    expect(store.isJoined('r1', new Set())).toBe(true);
  });

  it('returns false for an optimistically-just-left room even if still in the joined set', async () => {
    const { wireQueries } = makeWireQueries({ rooms: null });
    const store = makeStore(wireQueries);
    void store.refresh();
    await settle();

    store.justLeftIds.add('r1');
    expect(store.isJoined('r1', new Set(['r1']))).toBe(false);
  });

  it('justLeft takes precedence over justJoined when both are set', async () => {
    const { wireQueries } = makeWireQueries({ rooms: null });
    const store = makeStore(wireQueries);
    void store.refresh();
    await settle();

    store.justJoinedIds.add('r1');
    store.justLeftIds.add('r1');
    expect(store.isJoined('r1', new Set())).toBe(false);
  });
});

describe('RoomDirectoryStore — joinRoom', () => {
  it('marks joining during the request and just-joined on success', async () => {
    const { wireQueries } = makeWireQueries({
      rooms: [makeRoom('r1', { name: 'general' })]
    });
    const store = makeStore(wireQueries);
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
    const { wireQueries } = makeWireQueries({ rooms: [makeRoom('r1')] });
    wireMocks.joinRoom.mockRejectedValueOnce(new Error('permission denied'));
    const store = makeStore(wireQueries);
    void store.refresh();
    await settle();

    const result = await store.joinRoom('r1');
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error.message).toBe('permission denied');
    expect(store.joiningIds.has('r1')).toBe(false);
    expect(store.justJoinedIds.has('r1')).toBe(false);
  });

  it('clears a stale justLeft when the user re-joins', async () => {
    const { wireQueries } = makeWireQueries({ rooms: [makeRoom('r1')] });
    const store = makeStore(wireQueries);
    void store.refresh();
    await settle();

    store.justLeftIds.add('r1');
    await store.joinRoom('r1');

    expect(store.justJoinedIds.has('r1')).toBe(true);
    expect(store.justLeftIds.has('r1')).toBe(false);
  });
});

describe('RoomDirectoryStore — joinGroup', () => {
  it('marks group joining during the request and tracks joined room ids on success', async () => {
    const { wireQueries } = makeWireQueries({
      rooms: [makeRoom('r1'), makeRoom('r2'), makeRoom('r3')]
    });
    wireMocks.joinGroup.mockResolvedValueOnce(['r1', 'r3']);
    const store = makeStore(wireQueries);
    void store.refresh();
    await settle();

    const promise = store.joinGroup('g1');
    expect(store.joiningGroupIds.has('g1')).toBe(true);

    const result = await promise;
    expect(result).toEqual({ ok: true, joinedRoomIds: ['r1', 'r3'] });
    expect(wireMocks.joinGroup).toHaveBeenCalledWith({ groupId: 'g1' });
    expect(store.joiningGroupIds.has('g1')).toBe(false);
    expect(store.justJoinedIds.has('r1')).toBe(true);
    expect(store.justJoinedIds.has('r3')).toBe(true);
  });

  it('returns an error result and does not set just-joined when the join-all request fails', async () => {
    const { wireQueries } = makeWireQueries({
      rooms: [makeRoom('r1'), makeRoom('r2')]
    });
    wireMocks.joinGroup.mockRejectedValueOnce(new Error('cannot join group'));
    const store = makeStore(wireQueries);
    void store.refresh();
    await settle();

    const result = await store.joinGroup('g1');
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error.message).toBe('cannot join group');
    expect(store.joiningGroupIds.has('g1')).toBe(false);
    expect(store.justJoinedIds.size).toBe(0);
  });
});

describe('RoomDirectoryStore — leaveRoom', () => {
  it('marks leaving during the request and just-left on success, clearing justJoined', async () => {
    const { wireQueries } = makeWireQueries({ rooms: [makeRoom('r1')] });
    const store = makeStore(wireQueries);
    void store.refresh();
    await settle();

    store.justJoinedIds.add('r1'); // simulate prior optimistic join
    const promise = store.leaveRoom('r1');
    expect(store.leavingIds.has('r1')).toBe(true);

    const result = await promise;
    expect(result.ok).toBe(true);
    expect(store.leavingIds.has('r1')).toBe(false);
    expect(store.justLeftIds.has('r1')).toBe(true);
    expect(store.justJoinedIds.has('r1')).toBe(false);
  });

  it('returns an error result on failure', async () => {
    const { wireQueries } = makeWireQueries({ rooms: [makeRoom('r1')] });
    wireMocks.leaveRoom.mockRejectedValueOnce(new Error('cannot leave'));
    const store = makeStore(wireQueries);
    void store.refresh();
    await settle();

    const result = await store.leaveRoom('r1');
    expect(result.ok).toBe(false);
    expect(store.leavingIds.has('r1')).toBe(false);
    expect(store.justLeftIds.has('r1')).toBe(false);
  });
});

describe('RoomDirectoryStore — refresh clears optimistic state', () => {
  it('refresh clears just-* sets so the authoritative joined membership wins', async () => {
    const { wireQueries } = makeWireQueries({ rooms: [makeRoom('r1')] });
    const store = makeStore(wireQueries);
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

describe('RoomDirectoryStore — ingestServerEvent', () => {
  function makeEvent(typename: string): RoomEventViewFragment {
    return { event: { __typename: typename } } as unknown as RoomEventViewFragment;
  }

  it('refreshes on UserJoinedRoomEvent', async () => {
    const { wireQueries, listRoomsMock } = makeWireQueries({ rooms: [] });
    const store = makeStore(wireQueries);
    void store.refresh();
    await settle();
    expect(listRoomsMock).toHaveBeenCalledTimes(1);

    store.ingestServerEvent(makeEvent('UserJoinedRoomEvent'));
    await settle();
    expect(listRoomsMock).toHaveBeenCalledTimes(2);
  });

  it('refreshes on UserLeftRoomEvent', async () => {
    const { wireQueries, listRoomsMock } = makeWireQueries({ rooms: [] });
    const store = makeStore(wireQueries);
    void store.refresh();
    await settle();

    store.ingestServerEvent(makeEvent('UserLeftRoomEvent'));
    await settle();
    expect(listRoomsMock).toHaveBeenCalledTimes(2);
  });

  it('refreshes on room catalog and layout changes', async () => {
    const { wireQueries, listRoomsMock } = makeWireQueries({ rooms: [] });
    const store = makeStore(wireQueries);
    void store.refresh();
    await settle();

    store.ingestServerEvent(makeEvent('RoomCreatedEvent'));
    await settle();
    store.ingestServerEvent(makeEvent('RoomUpdatedEvent'));
    await settle();
    store.ingestServerEvent(makeEvent('RoomArchivedEvent'));
    await settle();
    store.ingestServerEvent(makeEvent('RoomUnarchivedEvent'));
    await settle();
    store.ingestServerEvent(makeEvent('RoomDeletedEvent'));
    await settle();
    store.ingestServerEvent(makeEvent('RoomGroupsUpdatedEvent'));
    await settle();

    expect(listRoomsMock).toHaveBeenCalledTimes(7);
  });

  it('does NOT refresh on irrelevant event types', async () => {
    const { wireQueries, listRoomsMock } = makeWireQueries({ rooms: [] });
    const store = makeStore(wireQueries);
    void store.refresh();
    await settle();

    store.ingestServerEvent(makeEvent('MessagePostedEvent'));
    store.ingestServerEvent(makeEvent('ReactionAddedEvent'));
    await settle();

    expect(listRoomsMock).toHaveBeenCalledTimes(1);
  });

  it('ingestRoomLayoutUpdated triggers a refresh', async () => {
    const { wireQueries, listRoomsMock } = makeWireQueries({ rooms: [] });
    const store = makeStore(wireQueries);
    void store.refresh();
    await settle();

    store.ingestRoomLayoutUpdated();
    await settle();
    expect(listRoomsMock).toHaveBeenCalledTimes(2);
  });
});

describe('RoomDirectoryStore — concurrent refresh guard', () => {
  it('discards out-of-order responses', async () => {
    let resolveFirst!: (value: DirectoryRoom[] | null) => void;
    let resolveSecond!: (value: DirectoryRoom[] | null) => void;

    const listRoomsMock = vi
      .fn()
      .mockImplementationOnce(() => new Promise((r) => (resolveFirst = r)))
      .mockImplementationOnce(() => new Promise((r) => (resolveSecond = r)));
    const wireQueries: RoomDirectoryWireQueries = { listRooms: listRoomsMock };

    const store = makeStore(wireQueries);
    void store.refresh(); // first load
    void store.refresh(); // second concurrent load

    // Resolve the SECOND load first (out-of-order)
    resolveSecond([makeRoom('newer')]);
    await settle();

    expect(store.allRooms.map((r) => r.id)).toEqual(['newer']);

    // The earlier load now resolves — should be ignored
    resolveFirst([makeRoom('older')]);
    await settle();

    expect(store.allRooms.map((r) => r.id)).toEqual(['newer']);
  });
});
