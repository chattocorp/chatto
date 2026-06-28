import { describe, it, expect, vi } from 'vitest';
import { flushSync } from 'svelte';
import type { AdminRoomLayoutAPI } from '$lib/api/adminRoomLayout';
import type { RoomCommandAPI } from '$lib/api/rooms';
import { RoomEventKind } from '$lib/render/eventKinds';
import {
  AdminRoomLayoutStore,
  buildGroupRoomOrder,
  planGroupReorder,
  planRoomMoveMutations,
  type AdminRoomGroup,
  type AdminRoomInfo
} from './adminRoomLayout.svelte';

function room(id: string, overrides: Partial<AdminRoomInfo> = {}): AdminRoomInfo {
  return {
    id,
    name: overrides.name ?? id,
    description: overrides.description ?? null,
    archived: overrides.archived ?? false,
    isUniversal: overrides.isUniversal ?? false
  };
}

function group(id: string, rooms: AdminRoomInfo[], name = id): AdminRoomGroup {
  return {
    id,
    name,
    rooms,
    items: rooms.map((room) => ({ id: `room:${room.id}`, kind: 'room', room }))
  };
}

function queryData(groups: AdminRoomGroup[]) {
  return groups;
}

type QueuedResult = {
  data?: unknown;
  error?: unknown;
  reject?: Error;
};

function serverEvent(kind: RoomEventKind) {
  return { event: { kind } } as never;
}

function makeClient(
  opts: {
    queries?: QueuedResult[];
    mutations?: QueuedResult[];
  } = {}
) {
  const queries = [...(opts.queries ?? [])];
  const mutations = [...(opts.mutations ?? [])];
  const nextResult = async (result: QueuedResult) => {
    if (result.reject) throw result.reject;
    if (result.error) throw result.error;
    return result.data ?? null;
  };
  const query = vi.fn(() => nextResult(queries.shift() ?? {}));
  const mutation = vi.fn((_method: string, _input?: unknown) =>
    nextResult(mutations.shift() ?? {})
  );
  const client = {
    listAdminRoomLayout: query,
    createRoomGroup: vi.fn((input) => mutation('createRoomGroup', input)),
    updateRoomGroup: vi.fn((input) => mutation('updateRoomGroup', input)),
    deleteRoomGroup: vi.fn((groupId) => mutation('deleteRoomGroup', groupId)),
    reorderRoomGroups: vi.fn((orderedGroupIds) => mutation('reorderRoomGroups', orderedGroupIds)),
    moveRoomToGroup: vi.fn((input) => mutation('moveRoomToGroup', input)),
    reorderSidebarItemsInGroup: vi.fn((input) => mutation('reorderSidebarItemsInGroup', input)),
    createSidebarLink: vi.fn((input) => mutation('createSidebarLink', input)),
    updateSidebarLink: vi.fn((input) => mutation('updateSidebarLink', input)),
    deleteSidebarLink: vi.fn((linkId) => mutation('deleteSidebarLink', linkId)),
    moveSidebarLinkToGroup: vi.fn((input) => mutation('moveSidebarLinkToGroup', input))
  } as unknown as AdminRoomLayoutAPI;
  return { client, query, mutation };
}

function roomAPI(
  overrides: Partial<
    Pick<RoomCommandAPI, 'updateRoom' | 'archiveRoom' | 'unarchiveRoom' | 'setRoomUniversal'>
  > = {}
): Pick<RoomCommandAPI, 'updateRoom' | 'archiveRoom' | 'unarchiveRoom' | 'setRoomUniversal'> {
  return {
    updateRoom: vi.fn().mockResolvedValue(null),
    archiveRoom: vi.fn().mockResolvedValue(null),
    unarchiveRoom: vi.fn().mockResolvedValue(null),
    setRoomUniversal: vi.fn().mockResolvedValue(null),
    ...overrides
  };
}

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

describe('admin room layout diff helpers', () => {
  it('emits no mutations for a no-op room drag', () => {
    const before = buildGroupRoomOrder([group('g1', [room('a'), room('b')])]);
    const after = buildGroupRoomOrder([group('g1', [room('a'), room('b')])]);

    expect(planRoomMoveMutations(before, after)).toEqual({
      moves: [],
      linkMoves: [],
      reorders: []
    });
  });

  it('emits only reorderRoomsInGroup for an intra-group reorder', () => {
    const before = buildGroupRoomOrder([group('g1', [room('a'), room('b')])]);
    const after = buildGroupRoomOrder([group('g1', [room('b'), room('a')])]);

    expect(planRoomMoveMutations(before, after)).toEqual({
      moves: [],
      linkMoves: [],
      reorders: [
        {
          groupId: 'g1',
          items: [
            { kind: 'room', id: 'b' },
            { kind: 'room', id: 'a' }
          ]
        }
      ]
    });
  });

  it('emits cross-group move before source/target reorders', () => {
    const before = buildGroupRoomOrder([
      group('g1', [room('a'), room('b')]),
      group('g2', [room('c'), room('d')])
    ]);
    const after = buildGroupRoomOrder([
      group('g1', [room('a')]),
      group('g2', [room('c'), room('b'), room('d')])
    ]);

    expect(planRoomMoveMutations(before, after)).toEqual({
      moves: [{ roomId: 'b', groupId: 'g2' }],
      linkMoves: [],
      reorders: [
        { groupId: 'g1', items: [{ kind: 'room', id: 'a' }] },
        {
          groupId: 'g2',
          items: [
            { kind: 'room', id: 'c' },
            { kind: 'room', id: 'b' },
            { kind: 'room', id: 'd' }
          ]
        }
      ]
    });
  });

  it('returns null for unchanged group order', () => {
    expect(planGroupReorder(['g1', 'g2'], ['g1', 'g2'])).toBeNull();
  });

  it('returns ordered IDs for changed group order', () => {
    expect(planGroupReorder(['g1', 'g2'], ['g2', 'g1'])).toEqual(['g2', 'g1']);
  });
});

describe('AdminRoomLayoutStore — loading', () => {
  it('maps server rooms plus roomGroups and preserves archived rooms', async () => {
    const archived = room('r2', { archived: true, description: 'hidden' });
    const { client } = makeClient({
      queries: [{ data: queryData([group('g1', [room('r1'), archived], 'Lobby')]) }]
    });
    const store = new AdminRoomLayoutStore(client, roomAPI());

    expect(store.loading).toBe(false);
    void store.refresh();
    expect(store.loading).toBe(true);
    await settle();

    expect(store.error).toBeNull();
    expect(store.groups).toEqual([
      {
        id: 'g1',
        name: 'Lobby',
        rooms: [room('r1'), archived],
        items: [
          { id: 'room:r1', kind: 'room', room: room('r1') },
          { id: 'room:r2', kind: 'room', room: archived }
        ]
      }
    ]);
    expect(store.initialized).toBe(true);
    expect(store.loading).toBe(false);
  });

  it('fills item order from rooms when the API does not provide mixed sidebar items', async () => {
    const { client } = makeClient({
      queries: [{ data: [{ id: 'g1', name: 'Lobby', rooms: [room('r1')] }] }]
    });
    const store = new AdminRoomLayoutStore(client, roomAPI());

    await store.refresh();

    expect(store.error).toBeNull();
    expect(store.groups).toEqual([
      {
        id: 'g1',
        name: 'Lobby',
        rooms: [room('r1')],
        items: [{ id: 'room:r1', kind: 'room', room: room('r1') }]
      }
    ]);
  });

  it('keeps known good layout when refresh fails', async () => {
    const { client } = makeClient({
      queries: [
        { data: queryData([group('g1', [room('r1')], 'Lobby')]) },
        { error: { message: 'offline' } }
      ]
    });
    const store = new AdminRoomLayoutStore(client, roomAPI());

    await store.refresh();
    expect(store.groups.map((g) => g.name)).toEqual(['Lobby']);

    await store.refresh();
    expect(store.error).toBe('offline');
    expect(store.groups.map((g) => g.name)).toEqual(['Lobby']);
  });

  it('discards stale out-of-order refresh responses', async () => {
    let resolveFirst!: (value: AdminRoomGroup[]) => void;
    let resolveSecond!: (value: AdminRoomGroup[]) => void;
    const listAdminRoomLayout = vi
      .fn()
      .mockImplementationOnce(() => new Promise((resolve) => (resolveFirst = resolve)))
      .mockImplementationOnce(() => new Promise((resolve) => (resolveSecond = resolve)));
    const client = {
      ...makeClient().client,
      listAdminRoomLayout
    };
    const store = new AdminRoomLayoutStore(client, roomAPI());

    void store.refresh();
    void store.refresh();

    resolveSecond(queryData([group('new', [room('new-room')])]));
    await settle();
    expect(store.groups.map((g) => g.id)).toEqual(['new']);

    resolveFirst(queryData([group('old', [room('old-room')])]));
    await settle();
    expect(store.groups.map((g) => g.id)).toEqual(['new']);
  });
});

describe('AdminRoomLayoutStore — mutations', () => {
  it('creates, renames, and deletes groups optimistically on success', async () => {
    const { client, mutation } = makeClient({
      mutations: [
        { data: { id: 'g2', name: 'Projects', rooms: [], items: [] } },
        { data: { id: 'g2', name: 'Renamed', rooms: [], items: [] } },
        { data: true }
      ]
    });
    const store = new AdminRoomLayoutStore(client, roomAPI());

    const createResult = await store.createGroup('Projects');
    expect(createResult).toEqual({
      ok: true,
      group: { id: 'g2', name: 'Projects', rooms: [], items: [] }
    });
    expect(store.groups.map((g) => g.name)).toEqual(['Projects']);

    await expect(store.renameGroup('g2', 'Renamed')).resolves.toEqual({ ok: true });
    expect(store.groups.map((g) => g.name)).toEqual(['Renamed']);

    await expect(store.deleteGroup('g2')).resolves.toEqual({ ok: true });
    expect(store.groups).toEqual([]);
    expect(mutation.mock.calls.map((call: unknown[]) => call[1])).toEqual([
      { name: 'Projects' },
      { groupId: 'g2', name: 'Renamed' },
      'g2'
    ]);
  });

  it('does not optimistically update a group when rename fails', async () => {
    const { client } = makeClient({ mutations: [{ error: { message: 'nope' } }] });
    const store = new AdminRoomLayoutStore(client, roomAPI());
    store.groups = [group('g1', [], 'Original')];

    await expect(store.renameGroup('g1', 'Changed')).resolves.toEqual({
      ok: false,
      error: 'nope'
    });
    expect(store.groups.map((g) => g.name)).toEqual(['Original']);
  });

  it('updates a room and refreshes for reconciliation', async () => {
    const { client, query } = makeClient({
      queries: [{ data: queryData([group('g1', [room('r1', { name: 'new-name' })])]) }]
    });
    const api = roomAPI();
    const store = new AdminRoomLayoutStore(client, api);

    await expect(store.updateRoom('r1', 'new-name', 'desc')).resolves.toEqual({ ok: true });

    expect(api.updateRoom).toHaveBeenCalledWith({
      roomId: 'r1',
      name: 'new-name',
      description: 'desc'
    });
    expect(query).toHaveBeenCalledTimes(1);
    expect(store.updatingRoom).toBe(false);
  });

  it('archives and unarchives rooms through Connect and refreshes', async () => {
    const { client, query } = makeClient({
      queries: [
        { data: queryData([group('g1', [room('r1', { archived: true })])]) },
        { data: queryData([group('g1', [room('r1', { archived: false })])]) }
      ]
    });
    const api = roomAPI();
    const store = new AdminRoomLayoutStore(client, api);

    await expect(store.archiveRoom('r1')).resolves.toEqual({ ok: true });
    await expect(store.unarchiveRoom('r1')).resolves.toEqual({ ok: true });

    expect(api.archiveRoom).toHaveBeenCalledWith('r1');
    expect(api.unarchiveRoom).toHaveBeenCalledWith('r1');
    expect(query).toHaveBeenCalledTimes(2);
    expect(store.archivingRoomId).toBeNull();
  });

  it('sets room universal state and refreshes for reconciliation', async () => {
    const { client, query } = makeClient({
      queries: [{ data: queryData([group('g1', [room('r1', { isUniversal: true })])]) }]
    });
    const api = roomAPI();
    const store = new AdminRoomLayoutStore(client, api);

    await expect(store.setRoomUniversal('r1', true)).resolves.toEqual({ ok: true });

    expect(api.setRoomUniversal).toHaveBeenCalledWith('r1', true);
    expect(query).toHaveBeenCalledTimes(1);
    expect(store.universalRoomId).toBeNull();
  });
});

describe('AdminRoomLayoutStore — drag sequencing', () => {
  it('flushes room move mutations before room reorder mutations', async () => {
    const { client, mutation } = makeClient({
      mutations: [{ data: null }, { data: null }, { data: null }]
    });
    const store = new AdminRoomLayoutStore(client, roomAPI());
    const a = room('a');
    const b = room('b');
    const c = room('c');
    const d = room('d');
    store.groups = [group('g1', [a, b]), group('g2', [c, d])];

    store.handleRoomDragConsider('g1', [a]);
    const result = await store.handleRoomDragFinalize('g2', [c, b, d]);

    expect(result).toEqual({ ok: true, movedCount: 1, reorderedCount: 2 });
    expect(mutation.mock.calls.map((call: unknown[]) => call[1])).toEqual([
      { roomId: 'b', groupId: 'g2' },
      { groupId: 'g1', items: [{ kind: 'room', id: 'a' }] },
      {
        groupId: 'g2',
        items: [
          { kind: 'room', id: 'c' },
          { kind: 'room', id: 'b' },
          { kind: 'room', id: 'd' }
        ]
      }
    ]);
  });

  it('requests a refresh when a room move or reorder fails', async () => {
    const { client, query } = makeClient({
      mutations: [{ error: { message: 'move denied' } }, { data: null }, { data: null }],
      queries: [{ data: queryData([group('g1', [room('a')])]) }]
    });
    const store = new AdminRoomLayoutStore(client, roomAPI());
    const a = room('a');
    const b = room('b');
    const c = room('c');
    store.groups = [group('g1', [a, b]), group('g2', [c])];

    store.handleRoomDragConsider('g1', [a]);
    const result = await store.handleRoomDragFinalize('g2', [c, b]);
    await settle();

    expect(result).toEqual({
      ok: false,
      movedCount: 1,
      reorderedCount: 2,
      errors: ['Failed to move room: move denied'],
      refreshRequested: true
    });
    expect(query).toHaveBeenCalledTimes(1);
  });

  it('does not call reorderRoomGroups when group order is unchanged', async () => {
    const { client, mutation } = makeClient();
    const store = new AdminRoomLayoutStore(client, roomAPI());
    store.groups = [group('g1', []), group('g2', [])];

    store.handleGroupsConsider([group('g1', []), group('g2', [])], 'g1');
    await expect(store.handleGroupsFinalize([group('g1', []), group('g2', [])])).resolves.toEqual({
      ok: true,
      changed: false
    });
    expect(mutation).not.toHaveBeenCalled();
  });

  it('calls reorderRoomGroups when group order changes', async () => {
    const { client, mutation } = makeClient({
      mutations: [{ data: [group('g2', []), group('g1', [])] }]
    });
    const store = new AdminRoomLayoutStore(client, roomAPI());
    store.groups = [group('g1', []), group('g2', [])];

    store.handleGroupsConsider([group('g2', []), group('g1', [])], 'g2');
    await expect(store.handleGroupsFinalize([group('g2', []), group('g1', [])])).resolves.toEqual({
      ok: true,
      changed: true
    });
    expect((mutation.mock.calls[0] as unknown[])[1]).toEqual(['g2', 'g1']);
  });
});

describe('AdminRoomLayoutStore — live events', () => {
  it('suppresses own room-layout echo events but refreshes later events', async () => {
    let now = 1000;
    const { client, query } = makeClient({
      mutations: [{ data: group('g1', [], 'Lobby') }],
      queries: [{ data: queryData([group('g1', [])]) }]
    });
    const store = new AdminRoomLayoutStore(client, roomAPI(), () => now);

    await store.createGroup('Lobby');
    now = 1500;
    expect(store.ingestRoomLayoutUpdated()).toBe(false);
    expect(query).not.toHaveBeenCalled();

    now = 3100;
    expect(store.ingestRoomLayoutUpdated()).toBe(true);
    await settle();
    expect(query).toHaveBeenCalledTimes(1);
  });

  it('refreshes on external room metadata/archive events', async () => {
    const { client, query } = makeClient({
      queries: [
        { data: queryData([group('g1', [room('r1', { name: 'fresh' })])]) },
        { data: queryData([group('g1', [room('r1', { archived: true })])]) },
        { data: queryData([group('g1', [room('r1', { archived: false })])]) },
        { data: queryData([group('g1', [room('r1', { isUniversal: true })])]) }
      ]
    });
    const store = new AdminRoomLayoutStore(client, roomAPI());

    expect(store.ingestServerEvent(serverEvent(RoomEventKind.RoomUpdated))).toBe(true);
    await settle();
    expect(store.ingestServerEvent(serverEvent(RoomEventKind.RoomArchived))).toBe(true);
    await settle();
    expect(store.ingestServerEvent(serverEvent(RoomEventKind.RoomUnarchived))).toBe(true);
    await settle();
    expect(store.ingestServerEvent(serverEvent(RoomEventKind.RoomUniversalChanged))).toBe(true);
    await settle();

    expect(query).toHaveBeenCalledTimes(4);
  });
});
