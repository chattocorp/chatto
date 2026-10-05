import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ServerPresence } from './presence.js';
import type { ServerConnection } from './serverConnection.js';
import { RoomStores } from './roomStores.js';

const mocks = vi.hoisted(() => ({ clearRoomPinsSeenMarker: vi.fn() }));

const { FakeStore } = vi.hoisted(() => ({
  FakeStore: class {
    readonly args: unknown[];
    dispose = vi.fn();
    clearAnchor = vi.fn();
    clearForAccessRevocation = vi.fn();
    restoreAfterAccessGrant = vi.fn();
    reset = vi.fn();
    resetProjectionState = vi.fn();
    setRoom = vi.fn();
    constructor(...args: unknown[]) {
      this.args = args;
    }
  }
}));

vi.mock('../room/messages/MessagesStore.js', () => ({ MessagesStore: class extends FakeStore {} }));
vi.mock('../room/files.js', () => ({ RoomFilesStore: class extends FakeStore {} }));
vi.mock('../room/members.js', () => ({ RoomMembersStore: class extends FakeStore {} }));

vi.mock('../room/pins.js', () => ({
  RoomPinsStore: class extends FakeStore {},
  clearRoomPinsSeenMarker: mocks.clearRoomPinsSeenMarker
}));

function makeRooms(viewerId: string | null = 'viewer'): RoomStores {
  return new RoomStores({
    serverId: 'server',
    connection: {} as ServerConnection,
    presence: {} as ServerPresence,
    realtimeViewerId: () => viewerId,
    viewerId: () => viewerId,
    projectedMemberIds: () => null
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
  });

  it('selects existing timelines for one room or for all rooms', () => {
    const rooms = makeRooms();
    const roomA = rooms.messages('A');
    const threadA = rooms.thread('A', 'T');
    const threadB = rooms.thread('B', 'T');
    rooms.members('C');

    expect(rooms.loaded('missing')).toBeUndefined();
    expect(rooms.timelines('A')).toEqual([roomA, threadA]);
    expect(rooms.timelines('missing')).toEqual([]);
    expect(rooms.timelines()).toEqual([roomA, threadA, threadB]);
    expect(rooms.all('messages')).toEqual([roomA]);
    expect(rooms.roomIds()).toEqual(['A', 'B', 'C']);
  });

  it('clears a thread anchor only when its last consumer releases it', () => {
    const rooms = makeRooms();
    const thread = rooms.thread('A', 'T');
    rooms.retainThread('A', 'T', thread);
    rooms.retainThread('A', 'T', thread);

    rooms.releaseThread('A', 'T', thread);
    expect(thread.clearAnchor).not.toHaveBeenCalled();
    rooms.releaseThread('A', 'T', thread);
    expect(thread.clearAnchor).toHaveBeenCalledOnce();
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
    expect(stale.clearAnchor).not.toHaveBeenCalled();
    rooms.releaseThread('A', 'T', current);
    expect(current.clearAnchor).toHaveBeenCalledOnce();
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

  it('forgets the plaintext stores of a room but keeps its members', () => {
    const rooms = makeRooms();
    const messages = rooms.messages('A');
    const thread = rooms.thread('A', 'T');
    const files = rooms.files('A');
    const pins = rooms.pins('A');
    const members = rooms.members('A');

    rooms.clearMessageAccess('A', true);

    for (const store of [messages, thread, files, pins]) expect(store.dispose).toHaveBeenCalled();
    expect(rooms.loaded('A')).toMatchObject({ members, threads: {} });
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

    rooms.dispose();

    for (const store of disposed) expect(store.dispose).toHaveBeenCalled();
    expect(members.resetProjectionState).toHaveBeenCalledOnce();
    expect(rooms.entries()).toEqual([]);
    expect(rooms.messages('A')).not.toBe(disposed[0]);
  });
});
