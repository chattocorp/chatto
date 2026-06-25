import { describe, it, expect, vi } from 'vitest';
import { flushSync } from 'svelte';
import type { Client } from '@urql/svelte';
import { RoomGroupItemType } from '$lib/gql/graphql';
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
  const roomsById = new Map<string, AdminRoomInfo>();
  for (const g of groups) {
    for (const r of g.rooms) roomsById.set(r.id, r);
  }
  return {
    server: {
      rooms: [...roomsById.values()],
      roomGroups: groups.map((g) => ({
        id: g.id,
        name: g.name,
        rooms: g.rooms.map((r) => ({ id: r.id })),
        items: (g.items ?? []).map((item) =>
          item.kind === 'room'
            ? {
                type: RoomGroupItemType.Room,
                id: item.room.id,
                room: { id: item.room.id },
                link: null
              }
            : {
                type: RoomGroupItemType.SidebarLink,
                id: item.link.id,
                room: null,
                link: item.link
              }
        )
      }))
    }
  };
}

type OperationResult = {
  data?: unknown;
  error?: unknown;
  reject?: Error;
};

function operation(result: OperationResult) {
  return {
    toPromise: vi.fn().mockImplementation(() => {
      if (result.reject) return Promise.reject(result.reject);
      return Promise.resolve({
        data: result.data ?? null,
        error: result.error ?? null
      });
    })
  };
}

function makeClient(
  opts: {
    queries?: OperationResult[];
    mutations?: OperationResult[];
  } = {}
) {
  const queries = [...(opts.queries ?? [])];
  const mutations = [...(opts.mutations ?? [])];
  const query = vi.fn(() => operation(queries.shift() ?? {}));
  const mutation = vi.fn(() => operation(mutations.shift() ?? {}));
  const client = { query, mutation, subscription: vi.fn() } as unknown as Client & {
    query: ReturnType<typeof vi.fn>;
    mutation: ReturnType<typeof vi.fn>;
  };
  return { client, query, mutation };
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
            { type: RoomGroupItemType.Room, id: 'b' },
            { type: RoomGroupItemType.Room, id: 'a' }
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
        { groupId: 'g1', items: [{ type: RoomGroupItemType.Room, id: 'a' }] },
        {
          groupId: 'g2',
          items: [
            { type: RoomGroupItemType.Room, id: 'c' },
            { type: RoomGroupItemType.Room, id: 'b' },
            { type: RoomGroupItemType.Room, id: 'd' }
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
    const store = new AdminRoomLayoutStore(client);

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

  it('treats partial roomGroups without rooms as empty instead of surfacing a page error', async () => {
    const { client } = makeClient({
      queries: [
        {
          data: {
            server: {
              rooms: [room('r1')],
              roomGroups: [{ id: 'g1', name: 'Lobby' }]
            }
          }
        }
      ]
    });
    const store = new AdminRoomLayoutStore(client);

    await store.refresh();

    expect(store.error).toBeNull();
    expect(store.groups).toEqual([{ id: 'g1', name: 'Lobby', rooms: [], items: [] }]);
  });

  it('keeps known good layout when the server is null or the query errors', async () => {
    const { client } = makeClient({
      queries: [
        { data: queryData([group('g1', [room('r1')], 'Lobby')]) },
        { data: { server: null } },
        { error: { message: 'offline' } }
      ]
    });
    const store = new AdminRoomLayoutStore(client);

    await store.refresh();
    expect(store.groups.map((g) => g.name)).toEqual(['Lobby']);

    await store.refresh();
    expect(store.error).toBe('Server not found');
    expect(store.groups.map((g) => g.name)).toEqual(['Lobby']);

    await store.refresh();
    expect(store.error).toBe('offline');
    expect(store.groups.map((g) => g.name)).toEqual(['Lobby']);
  });

  it('discards stale out-of-order refresh responses', async () => {
    let resolveFirst!: (value: { data: unknown; error: null }) => void;
    let resolveSecond!: (value: { data: unknown; error: null }) => void;
    const query = vi
      .fn()
      .mockImplementationOnce(() => ({
        toPromise: () => new Promise((resolve) => (resolveFirst = resolve))
      }))
      .mockImplementationOnce(() => ({
        toPromise: () => new Promise((resolve) => (resolveSecond = resolve))
      }));
    const client = { query, mutation: vi.fn(), subscription: vi.fn() } as unknown as Client;
    const store = new AdminRoomLayoutStore(client);

    void store.refresh();
    void store.refresh();

    resolveSecond({ data: queryData([group('new', [room('new-room')])]), error: null });
    await settle();
    expect(store.groups.map((g) => g.id)).toEqual(['new']);

    resolveFirst({ data: queryData([group('old', [room('old-room')])]), error: null });
    await settle();
    expect(store.groups.map((g) => g.id)).toEqual(['new']);
  });
});

describe('AdminRoomLayoutStore — mutations', () => {
  it('creates, renames, and deletes groups optimistically on success', async () => {
    const { client, mutation } = makeClient({
      mutations: [
        { data: { createRoomGroup: { id: 'g2', name: 'Projects' } } },
        { data: { updateRoomGroup: { id: 'g2', name: 'Renamed' } } },
        { data: { deleteRoomGroup: true } }
      ]
    });
    const store = new AdminRoomLayoutStore(client);

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
      { input: { name: 'Projects' } },
      { input: { id: 'g2', name: 'Renamed' } },
      { input: { id: 'g2' } }
    ]);
  });

  it('does not optimistically update a group when rename fails', async () => {
    const { client } = makeClient({ mutations: [{ error: { message: 'nope' } }] });
    const store = new AdminRoomLayoutStore(client);
    store.groups = [group('g1', [], 'Original')];

    await expect(store.renameGroup('g1', 'Changed')).resolves.toEqual({
      ok: false,
      error: 'nope'
    });
    expect(store.groups.map((g) => g.name)).toEqual(['Original']);
  });

  it('updates a room and refreshes for reconciliation', async () => {
    const { client, mutation, query } = makeClient({
      mutations: [{ data: { updateRoom: { id: 'r1', name: 'new-name', description: 'desc' } } }],
      queries: [{ data: queryData([group('g1', [room('r1', { name: 'new-name' })])]) }]
    });
    const store = new AdminRoomLayoutStore(client);

    await expect(store.updateRoom('r1', 'new-name', 'desc')).resolves.toEqual({ ok: true });

    expect((mutation.mock.calls[0] as unknown[])[1]).toEqual({
      input: { roomId: 'r1', name: 'new-name', description: 'desc' }
    });
    expect(query).toHaveBeenCalledTimes(1);
    expect(store.updatingRoom).toBe(false);
  });

  it('archives and unarchives rooms through matching mutations and refreshes', async () => {
    const { client, mutation, query } = makeClient({
      mutations: [
        { data: { archiveRoom: { id: 'r1', archived: true } } },
        { data: { unarchiveRoom: { id: 'r1', archived: false } } }
      ],
      queries: [
        { data: queryData([group('g1', [room('r1', { archived: true })])]) },
        { data: queryData([group('g1', [room('r1', { archived: false })])]) }
      ]
    });
    const store = new AdminRoomLayoutStore(client);

    await expect(store.archiveRoom('r1')).resolves.toEqual({ ok: true });
    await expect(store.unarchiveRoom('r1')).resolves.toEqual({ ok: true });

    expect(mutation.mock.calls.map((call: unknown[]) => call[1])).toEqual([
      { input: { roomId: 'r1' } },
      { input: { roomId: 'r1' } }
    ]);
    expect(query).toHaveBeenCalledTimes(2);
    expect(store.archivingRoomId).toBeNull();
  });

  it('sets room universal state and refreshes for reconciliation', async () => {
    const { client, mutation, query } = makeClient({
      mutations: [{ data: { setRoomUniversal: { id: 'r1', isUniversal: true } } }],
      queries: [{ data: queryData([group('g1', [room('r1', { isUniversal: true })])]) }]
    });
    const store = new AdminRoomLayoutStore(client);

    await expect(store.setRoomUniversal('r1', true)).resolves.toEqual({ ok: true });

    expect((mutation.mock.calls[0] as unknown[])[1]).toEqual({
      input: { roomId: 'r1', isUniversal: true }
    });
    expect(query).toHaveBeenCalledTimes(1);
    expect(store.universalRoomId).toBeNull();
  });
});

describe('AdminRoomLayoutStore — drag sequencing', () => {
  it('flushes room move mutations before room reorder mutations', async () => {
    const { client, mutation } = makeClient({
      mutations: [
        { data: { moveRoomToGroup: { id: 'g2' } } },
        { data: { reorderRoomsInGroup: { id: 'g1' } } },
        { data: { reorderRoomsInGroup: { id: 'g2' } } }
      ]
    });
    const store = new AdminRoomLayoutStore(client);
    const a = room('a');
    const b = room('b');
    const c = room('c');
    const d = room('d');
    store.groups = [group('g1', [a, b]), group('g2', [c, d])];

    store.handleRoomDragConsider('g1', [a]);
    const result = await store.handleRoomDragFinalize('g2', [c, b, d]);

    expect(result).toEqual({ ok: true, movedCount: 1, reorderedCount: 2 });
    expect(mutation.mock.calls.map((call: unknown[]) => call[1])).toEqual([
      { input: { roomId: 'b', groupId: 'g2' } },
      { input: { groupId: 'g1', items: [{ type: RoomGroupItemType.Room, id: 'a' }] } },
      {
        input: {
          groupId: 'g2',
          items: [
            { type: RoomGroupItemType.Room, id: 'c' },
            { type: RoomGroupItemType.Room, id: 'b' },
            { type: RoomGroupItemType.Room, id: 'd' }
          ]
        }
      }
    ]);
  });

  it('requests a refresh when a room move or reorder fails', async () => {
    const { client, query } = makeClient({
      mutations: [
        { error: { message: 'move denied' } },
        { data: { reorderRoomsInGroup: { id: 'g1' } } },
        { data: { reorderRoomsInGroup: { id: 'g2' } } }
      ],
      queries: [{ data: queryData([group('g1', [room('a')])]) }]
    });
    const store = new AdminRoomLayoutStore(client);
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
    const store = new AdminRoomLayoutStore(client);
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
      mutations: [{ data: { reorderRoomGroups: [{ id: 'g2' }, { id: 'g1' }] } }]
    });
    const store = new AdminRoomLayoutStore(client);
    store.groups = [group('g1', []), group('g2', [])];

    store.handleGroupsConsider([group('g2', []), group('g1', [])], 'g2');
    await expect(store.handleGroupsFinalize([group('g2', []), group('g1', [])])).resolves.toEqual({
      ok: true,
      changed: true
    });
    expect((mutation.mock.calls[0] as unknown[])[1]).toEqual({
      input: { orderedIds: ['g2', 'g1'] }
    });
  });
});

describe('AdminRoomLayoutStore — live events', () => {
  it('suppresses own room-layout echo events but refreshes later events', async () => {
    let now = 1000;
    const { client, query } = makeClient({
      mutations: [{ data: { createRoomGroup: { id: 'g1', name: 'Lobby' } } }],
      queries: [{ data: queryData([group('g1', [])]) }]
    });
    const store = new AdminRoomLayoutStore(client, () => now);

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
    const store = new AdminRoomLayoutStore(client);

    expect(store.ingestServerEvent({ event: { __typename: 'RoomUpdatedEvent' } })).toBe(true);
    await settle();
    expect(store.ingestServerEvent({ event: { __typename: 'RoomArchivedEvent' } })).toBe(true);
    await settle();
    expect(store.ingestServerEvent({ event: { __typename: 'RoomUnarchivedEvent' } })).toBe(true);
    await settle();
    expect(store.ingestServerEvent({ event: { __typename: 'RoomUniversalChangedEvent' } })).toBe(
      true
    );
    await settle();

    expect(query).toHaveBeenCalledTimes(4);
  });
});
