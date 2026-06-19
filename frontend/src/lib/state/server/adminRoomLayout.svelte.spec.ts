import { describe, it, expect, vi } from 'vitest';
import { flushSync } from 'svelte';
import {
  AdminRoomLayoutStore,
  buildGroupRoomOrder,
  planGroupReorder,
  planRoomMoveMutations,
  type AdminRoomGroup,
  type AdminRoomLayoutWireClient,
  type AdminRoomInfo
} from './adminRoomLayout.svelte';

function room(id: string, overrides: Partial<AdminRoomInfo> = {}): AdminRoomInfo {
  return {
    id,
    name: overrides.name ?? id,
    description: overrides.description ?? null,
    archived: overrides.archived ?? false
  };
}

function group(id: string, rooms: AdminRoomInfo[], name = id): AdminRoomGroup {
  return { id, name, rooms };
}

function wireGroup(group: AdminRoomGroup) {
  return {
    id: group.id,
    name: group.name,
    rooms: group.rooms.map((r) => ({
      id: r.id,
      name: r.name,
      description: r.description ?? '',
      archived: r.archived
    }))
  };
}

function wireLayout(groups: AdminRoomGroup[]) {
  return {
    groups: groups.map(wireGroup)
  };
}

type WireOperationResult = {
  value?: unknown;
  reject?: Error;
};

type WireMethodName = keyof AdminRoomLayoutWireClient;

function makeClient(results: Partial<Record<WireMethodName, WireOperationResult[]>> = {}) {
  const queues = new Map<WireMethodName, WireOperationResult[]>(
    Object.entries(results).map(([key, value]) => [key as WireMethodName, [...(value ?? [])]])
  );
  const method = (name: WireMethodName) =>
    vi.fn((..._args: unknown[]) => {
      const next = queues.get(name)?.shift() ?? {};
      if (next.reject) return Promise.reject(next.reject);
      return Promise.resolve(next.value ?? {});
    });

  const client = {
    getAdminRoomLayout: method('getAdminRoomLayout'),
    createAdminRoomGroup: method('createAdminRoomGroup'),
    updateAdminRoomGroup: method('updateAdminRoomGroup'),
    deleteAdminRoomGroup: method('deleteAdminRoomGroup'),
    reorderAdminRoomGroups: method('reorderAdminRoomGroups'),
    moveAdminRoomToGroup: method('moveAdminRoomToGroup'),
    reorderAdminRoomsInGroup: method('reorderAdminRoomsInGroup'),
    updateAdminRoom: method('updateAdminRoom'),
    archiveAdminRoom: method('archiveAdminRoom'),
    unarchiveAdminRoom: method('unarchiveAdminRoom')
  };
  return { client: client as unknown as AdminRoomLayoutWireClient, ...client };
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

    expect(planRoomMoveMutations(before, after)).toEqual({ moves: [], reorders: [] });
  });

  it('emits only reorderRoomsInGroup for an intra-group reorder', () => {
    const before = buildGroupRoomOrder([group('g1', [room('a'), room('b')])]);
    const after = buildGroupRoomOrder([group('g1', [room('b'), room('a')])]);

    expect(planRoomMoveMutations(before, after)).toEqual({
      moves: [],
      reorders: [{ groupId: 'g1', orderedRoomIds: ['b', 'a'] }]
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
      reorders: [
        { groupId: 'g1', orderedRoomIds: ['a'] },
        { groupId: 'g2', orderedRoomIds: ['c', 'b', 'd'] }
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
      getAdminRoomLayout: [{ value: wireLayout([group('g1', [room('r1'), archived], 'Lobby')]) }]
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
        rooms: [room('r1'), archived]
      }
    ]);
    expect(store.initialized).toBe(true);
    expect(store.loading).toBe(false);
  });

  it('treats partial roomGroups without rooms as empty instead of surfacing a page error', async () => {
    const { client } = makeClient({
      getAdminRoomLayout: [
        {
          value: {
            groups: [{ id: 'g1', name: 'Lobby' }]
          }
        }
      ]
    });
    const store = new AdminRoomLayoutStore(client);

    await store.refresh();

    expect(store.error).toBeNull();
    expect(store.groups).toEqual([{ id: 'g1', name: 'Lobby', rooms: [] }]);
  });

  it('keeps known good layout when the wire request errors', async () => {
    const { client } = makeClient({
      getAdminRoomLayout: [
        { value: wireLayout([group('g1', [room('r1')], 'Lobby')]) },
        { reject: new Error('offline') }
      ]
    });
    const store = new AdminRoomLayoutStore(client);

    await store.refresh();
    expect(store.groups.map((g) => g.name)).toEqual(['Lobby']);

    await store.refresh();
    expect(store.error).toBe('offline');
    expect(store.groups.map((g) => g.name)).toEqual(['Lobby']);
  });

  it('discards stale out-of-order refresh responses', async () => {
    let resolveFirst!: (value: unknown) => void;
    let resolveSecond!: (value: unknown) => void;
    const getAdminRoomLayout = vi
      .fn()
      .mockImplementationOnce(() => new Promise((resolve) => (resolveFirst = resolve)))
      .mockImplementationOnce(() => new Promise((resolve) => (resolveSecond = resolve)));
    const client = {
      ...makeClient().client,
      getAdminRoomLayout
    } as unknown as AdminRoomLayoutWireClient;
    const store = new AdminRoomLayoutStore(client);

    void store.refresh();
    void store.refresh();

    resolveSecond(wireLayout([group('new', [room('new-room')])]));
    await settle();
    expect(store.groups.map((g) => g.id)).toEqual(['new']);

    resolveFirst(wireLayout([group('old', [room('old-room')])]));
    await settle();
    expect(store.groups.map((g) => g.id)).toEqual(['new']);
  });
});

describe('AdminRoomLayoutStore — mutations', () => {
  it('creates, renames, and deletes groups optimistically on success', async () => {
    const { client, createAdminRoomGroup, updateAdminRoomGroup, deleteAdminRoomGroup } = makeClient({
      createAdminRoomGroup: [
        { value: { group: wireGroup(group('g2', [], 'Projects')) } }
      ],
      updateAdminRoomGroup: [{ value: { group: wireGroup(group('g2', [], 'Renamed')) } }],
      deleteAdminRoomGroup: [{ value: { deleted: true } }]
    });
    const store = new AdminRoomLayoutStore(client);

    const createResult = await store.createGroup('Projects');
    expect(createResult).toEqual({
      ok: true,
      group: { id: 'g2', name: 'Projects', rooms: [] }
    });
    expect(store.groups.map((g) => g.name)).toEqual(['Projects']);

    await expect(store.renameGroup('g2', 'Renamed')).resolves.toEqual({ ok: true });
    expect(store.groups.map((g) => g.name)).toEqual(['Renamed']);

    await expect(store.deleteGroup('g2')).resolves.toEqual({ ok: true });
    expect(store.groups).toEqual([]);
    expect(createAdminRoomGroup.mock.calls[0][0]).toMatchObject({ name: 'Projects' });
    expect(updateAdminRoomGroup.mock.calls[0][0]).toMatchObject({
      groupId: 'g2',
      name: 'Renamed'
    });
    expect(deleteAdminRoomGroup.mock.calls[0][0]).toMatchObject({ groupId: 'g2' });
  });

  it('does not optimistically update a group when rename fails', async () => {
    const { client } = makeClient({
      updateAdminRoomGroup: [{ reject: new Error('nope') }]
    });
    const store = new AdminRoomLayoutStore(client);
    store.groups = [group('g1', [], 'Original')];

    await expect(store.renameGroup('g1', 'Changed')).resolves.toEqual({
      ok: false,
      error: 'nope'
    });
    expect(store.groups.map((g) => g.name)).toEqual(['Original']);
  });

  it('updates a room and refreshes for reconciliation', async () => {
    const { client, updateAdminRoom, getAdminRoomLayout } = makeClient({
      updateAdminRoom: [
        { value: { room: { id: 'r1', name: 'new-name', description: 'desc', archived: false } } }
      ],
      getAdminRoomLayout: [{ value: wireLayout([group('g1', [room('r1', { name: 'new-name' })])]) }]
    });
    const store = new AdminRoomLayoutStore(client);

    await expect(store.updateRoom('r1', 'new-name', 'desc')).resolves.toEqual({ ok: true });

    expect(updateAdminRoom.mock.calls[0][0]).toMatchObject({
      roomId: 'r1',
      name: 'new-name',
      description: 'desc'
    });
    expect(getAdminRoomLayout).toHaveBeenCalledTimes(1);
    expect(store.updatingRoom).toBe(false);
  });

  it('archives and unarchives rooms through matching mutations and refreshes', async () => {
    const { client, archiveAdminRoom, unarchiveAdminRoom, getAdminRoomLayout } = makeClient({
      archiveAdminRoom: [
        { value: { room: { id: 'r1', name: 'r1', description: '', archived: true } } }
      ],
      unarchiveAdminRoom: [
        { value: { room: { id: 'r1', name: 'r1', description: '', archived: false } } }
      ],
      getAdminRoomLayout: [
        { value: wireLayout([group('g1', [room('r1', { archived: true })])]) },
        { value: wireLayout([group('g1', [room('r1', { archived: false })])]) }
      ]
    });
    const store = new AdminRoomLayoutStore(client);

    await expect(store.archiveRoom('r1')).resolves.toEqual({ ok: true });
    await expect(store.unarchiveRoom('r1')).resolves.toEqual({ ok: true });

    expect(archiveAdminRoom.mock.calls[0][0]).toMatchObject({ roomId: 'r1' });
    expect(unarchiveAdminRoom.mock.calls[0][0]).toMatchObject({ roomId: 'r1' });
    expect(getAdminRoomLayout).toHaveBeenCalledTimes(2);
    expect(store.archivingRoomId).toBeNull();
  });
});

describe('AdminRoomLayoutStore — drag sequencing', () => {
  it('flushes room move mutations before room reorder mutations', async () => {
    const { client, moveAdminRoomToGroup, reorderAdminRoomsInGroup } = makeClient({
      moveAdminRoomToGroup: [{ value: { room: { id: 'b', name: 'b', archived: false } } }],
      reorderAdminRoomsInGroup: [
        { value: { group: wireGroup(group('g1', [room('a')])) } },
        { value: { group: wireGroup(group('g2', [room('c'), room('b'), room('d')])) } }
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
    expect(moveAdminRoomToGroup.mock.calls[0][0]).toMatchObject({ roomId: 'b', groupId: 'g2' });
    expect(reorderAdminRoomsInGroup.mock.calls.map((call) => call[0])).toMatchObject([
      { groupId: 'g1', orderedRoomIds: ['a'] },
      { groupId: 'g2', orderedRoomIds: ['c', 'b', 'd'] }
    ]);
  });

  it('requests a refresh when a room move or reorder fails', async () => {
    const { client, getAdminRoomLayout } = makeClient({
      moveAdminRoomToGroup: [{ reject: new Error('move denied') }],
      reorderAdminRoomsInGroup: [
        { value: { group: wireGroup(group('g1', [room('a')])) } },
        { value: { group: wireGroup(group('g2', [room('c'), room('b')])) } }
      ],
      getAdminRoomLayout: [{ value: wireLayout([group('g1', [room('a')])]) }]
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
    expect(getAdminRoomLayout).toHaveBeenCalledTimes(1);
  });

  it('does not call reorderRoomGroups when group order is unchanged', async () => {
    const { client, reorderAdminRoomGroups } = makeClient();
    const store = new AdminRoomLayoutStore(client);
    store.groups = [group('g1', []), group('g2', [])];

    store.handleGroupsConsider([group('g1', []), group('g2', [])], 'g1');
    await expect(store.handleGroupsFinalize([group('g1', []), group('g2', [])])).resolves.toEqual({
      ok: true,
      changed: false
    });
    expect(reorderAdminRoomGroups).not.toHaveBeenCalled();
  });

  it('calls reorderRoomGroups when group order changes', async () => {
    const { client, reorderAdminRoomGroups } = makeClient({
      reorderAdminRoomGroups: [{ value: { groups: [wireGroup(group('g2', [])), wireGroup(group('g1', []))] } }]
    });
    const store = new AdminRoomLayoutStore(client);
    store.groups = [group('g1', []), group('g2', [])];

    store.handleGroupsConsider([group('g2', []), group('g1', [])], 'g2');
    await expect(store.handleGroupsFinalize([group('g2', []), group('g1', [])])).resolves.toEqual({
      ok: true,
      changed: true
    });
    expect(reorderAdminRoomGroups.mock.calls[0][0]).toMatchObject({
      orderedGroupIds: ['g2', 'g1']
    });
  });
});

describe('AdminRoomLayoutStore — live events', () => {
  it('suppresses own room-layout echo events but refreshes later events', async () => {
    let now = 1000;
    const { client, getAdminRoomLayout } = makeClient({
      createAdminRoomGroup: [{ value: { group: wireGroup(group('g1', [], 'Lobby')) } }],
      getAdminRoomLayout: [{ value: wireLayout([group('g1', [])]) }]
    });
    const store = new AdminRoomLayoutStore(client, () => now);

    await store.createGroup('Lobby');
    now = 1500;
    expect(store.ingestRoomLayoutUpdated()).toBe(false);
    expect(getAdminRoomLayout).not.toHaveBeenCalled();

    now = 3100;
    expect(store.ingestRoomLayoutUpdated()).toBe(true);
    await settle();
    expect(getAdminRoomLayout).toHaveBeenCalledTimes(1);
  });

  it('refreshes on external room metadata/archive events', async () => {
    const { client, getAdminRoomLayout } = makeClient({
      getAdminRoomLayout: [
        { value: wireLayout([group('g1', [room('r1', { name: 'fresh' })])]) },
        { value: wireLayout([group('g1', [room('r1', { archived: true })])]) },
        { value: wireLayout([group('g1', [room('r1', { archived: false })])]) }
      ]
    });
    const store = new AdminRoomLayoutStore(client);

    expect(store.ingestServerEvent({ event: { __typename: 'RoomUpdatedEvent' } })).toBe(true);
    await settle();
    expect(store.ingestServerEvent({ event: { __typename: 'RoomArchivedEvent' } })).toBe(true);
    await settle();
    expect(store.ingestServerEvent({ event: { __typename: 'RoomUnarchivedEvent' } })).toBe(true);
    await settle();

    expect(getAdminRoomLayout).toHaveBeenCalledTimes(3);
  });
});
