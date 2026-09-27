import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { MessageSearchAPI } from '$lib/api-client/messageSearch';
import type { ServerPresence } from './presence.svelte';
import type { ServerConnection } from './serverConnection.svelte';
import { RoomStores } from './roomStores.svelte';

const mocks = vi.hoisted(() => ({ clearRoomPinsSeenMarker: vi.fn() }));

vi.mock('$lib/state/room', () => {
  class FakeStore {
    readonly args: unknown[];
    dispose = vi.fn();
    clearViewport = vi.fn();
    clearForAccessRevocation = vi.fn();
    restoreAfterAccessGrant = vi.fn();
    reset = vi.fn();
    resetProjectionState = vi.fn();
    setRoom = vi.fn();
    constructor(...args: unknown[]) {
      this.args = args;
    }
  }
  return {
    MessagesStore: class extends FakeStore {},
    RoomFilesStore: class extends FakeStore {},
    RoomPinsStore: class extends FakeStore {},
    RoomMembersStore: class extends FakeStore {}
  };
});

vi.mock('$lib/state/room/pins.svelte', () => ({
  clearRoomPinsSeenMarker: mocks.clearRoomPinsSeenMarker
}));

vi.mock('./messageSearch.svelte', () => ({
  MessageSearchStore: class {
    reset = vi.fn();
  }
}));

function makeRooms(viewerId: string | null = 'viewer'): RoomStores {
  return new RoomStores({
    serverId: 'server',
    connection: {} as ServerConnection,
    presence: {} as ServerPresence,
    messageSearchAPI: {} as MessageSearchAPI,
    realtimeViewerId: () => viewerId,
    viewerId: () => viewerId,
    projectedMemberIds: () => null,
    isAuthenticated: () => true
  });
}

describe('RoomStores', () => {
  beforeEach(() => {
    mocks.clearRoomPinsSeenMarker.mockReset();
  });

  it('returns one stable store for each room, thread, and kind', () => {
    const rooms = makeRooms();

    expect(rooms.messages('A')).toBe(rooms.messages('A'));
    expect(rooms.messages('A')).not.toBe(rooms.messages('B'));
    expect(rooms.thread('A', 'T1')).toBe(rooms.thread('A', 'T1'));
    expect(rooms.thread('A', 'T1')).not.toBe(rooms.thread('A', 'T2'));
    expect(rooms.thread('A', 'T1')).not.toBe(rooms.messages('A'));
    expect(rooms.files('A')).toBe(rooms.files('A'));
    expect(rooms.pins('A')).toBe(rooms.pins('A'));
    expect(rooms.members('A')).toBe(rooms.members('A'));
    expect(rooms.search('A')).toBe(rooms.search('A'));
  });

  it('selects existing timelines for one room or for all rooms', () => {
    const rooms = makeRooms();
    const roomA = rooms.messages('A');
    const threadA = rooms.thread('A', 'T');
    const threadB = rooms.thread('B', 'T');
    rooms.search('C');

    expect(rooms.loaded('missing')).toBeUndefined();
    expect(rooms.timelines('A')).toEqual([roomA, threadA]);
    expect(rooms.timelines('missing')).toEqual([]);
    expect(rooms.timelines()).toEqual([roomA, threadA, threadB]);
    expect(rooms.all('messages')).toEqual([roomA]);
    // A search alone does not hold plaintext that a permission change can revoke.
    expect(rooms.roomIds()).toEqual(['A', 'B']);
  });

  it('clears a thread viewport only when its last consumer releases it', () => {
    const rooms = makeRooms();
    const thread = rooms.thread('A', 'T');
    rooms.retainThread('A', 'T', thread);
    rooms.retainThread('A', 'T', thread);

    rooms.releaseThread('A', 'T', thread);
    expect(thread.clearViewport).not.toHaveBeenCalled();
    rooms.releaseThread('A', 'T', thread);
    expect(thread.clearViewport).toHaveBeenCalledOnce();
    expect(rooms.thread('A', 'T')).toBe(thread);
  });

  it('ignores consumers of a thread store that it no longer owns', () => {
    const rooms = makeRooms();
    const stale = rooms.thread('A', 'T');
    rooms.clearMessageAccess('A', true);
    const current = rooms.thread('A', 'T');
    rooms.retainThread('A', 'T', current);

    rooms.retainThread('A', 'T', stale);
    rooms.releaseThread('A', 'T', stale);
    expect(stale.clearViewport).not.toHaveBeenCalled();
    rooms.releaseThread('A', 'T', current);
    expect(current.clearViewport).toHaveBeenCalledOnce();
  });

  it('clears and restores the plaintext of one room', () => {
    const rooms = makeRooms();
    const messages = rooms.messages('A');
    const thread = rooms.thread('A', 'T');
    const files = rooms.files('A');
    const pins = rooms.pins('A');
    const other = rooms.messages('B');

    rooms.clearMessageAccess('A');
    expect(mocks.clearRoomPinsSeenMarker).toHaveBeenCalledWith('server', 'viewer', 'A');
    expect(messages.clearForAccessRevocation).toHaveBeenCalledOnce();
    expect(thread.clearForAccessRevocation).toHaveBeenCalledOnce();
    expect(files.reset).toHaveBeenCalledWith();
    expect(pins.reset).toHaveBeenCalledWith({ accessRevoked: true });
    expect(other.clearForAccessRevocation).not.toHaveBeenCalled();

    rooms.restoreAccess('A');
    for (const store of [messages, thread, files, pins]) {
      expect(store.restoreAfterAccessGrant).toHaveBeenCalledOnce();
    }
    expect(rooms.messages('A')).toBe(messages);

    // The pin marker is device-local, so it is cleared for a room without stores too.
    rooms.clearMessageAccess('unknown');
    expect(mocks.clearRoomPinsSeenMarker).toHaveBeenCalledWith('server', 'viewer', 'unknown');
  });

  it('removes the least recently used room search', async () => {
    const rooms = makeRooms();
    const searches = Array.from({ length: 10 }, (_, index) => rooms.search(`R${index + 1}`));

    expect(rooms.search('R1')).toBe(searches[0]);
    rooms.search('R11');
    await Promise.resolve();

    expect(rooms.loaded('R2')?.search).toBeUndefined();
    expect(searches[1].reset).toHaveBeenCalledOnce();
    expect(rooms.search('R1')).toBe(searches[0]);
    expect(searches[0].reset).not.toHaveBeenCalled();
  });

  it('forgets the plaintext stores of a room but keeps its members and search', () => {
    const rooms = makeRooms();
    const messages = rooms.messages('A');
    const thread = rooms.thread('A', 'T');
    const files = rooms.files('A');
    const pins = rooms.pins('A');
    const members = rooms.members('A');
    const search = rooms.search('A');

    rooms.clearMessageAccess('A', true);

    for (const store of [messages, thread, files, pins]) expect(store.dispose).toHaveBeenCalled();
    expect(rooms.loaded('A')).toMatchObject({ members, search, threads: {} });
    expect(rooms.timelines('A')).toEqual([]);
    expect(rooms.messages('A')).not.toBe(messages);
    expect(rooms.files('A')).not.toBe(files);
    expect(rooms.members('A')).toBe(members);
  });

  it('gives one reset handler to each store that a projection reset makes stale', () => {
    const rooms = makeRooms();
    const members = rooms.members('A');
    const timelines = [rooms.messages('A'), rooms.thread('A', 'T')];
    const files = rooms.files('A');
    const pins = rooms.pins('A');
    rooms.search('A');

    const handlers = rooms.resetHandlers();
    expect(handlers).toHaveLength(5);
    for (const handler of handlers) handler();

    expect(members.resetProjectionState).toHaveBeenCalledOnce();
    for (const store of timelines) expect(store.resetProjectionState).toHaveBeenCalledOnce();
    expect(files.reset).toHaveBeenCalledWith({ rehydrateRetained: true });
    expect(pins.reset).toHaveBeenCalledWith({ rehydrateRetained: true });
  });

  it('disposes every store and starts again after dispose', () => {
    const rooms = makeRooms();
    const disposed = [
      rooms.messages('A'),
      rooms.thread('A', 'T'),
      rooms.files('A'),
      rooms.pins('A')
    ];
    const members = rooms.members('A');
    const search = rooms.search('A');

    rooms.dispose();

    for (const store of disposed) expect(store.dispose).toHaveBeenCalled();
    expect(members.resetProjectionState).toHaveBeenCalledOnce();
    expect(search.reset).toHaveBeenCalledOnce();
    expect(rooms.entries()).toEqual([]);
    expect(rooms.messages('A')).not.toBe(disposed[0]);
  });
});
