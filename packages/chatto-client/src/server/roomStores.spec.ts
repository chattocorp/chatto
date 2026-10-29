import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ServerPresence } from './presence.js';
import type { ServerConnection } from './serverConnection.js';
import {
  RECENT_ROOM_LIMIT,
  RECENT_THREAD_LIMIT,
  RoomStores,
  type KeepRecent
} from './roomStores.js';

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

function makeRooms(viewerId: string | null = 'viewer', keepRecent?: KeepRecent): RoomStores {
  return new RoomStores({
    serverId: 'server',
    connection: {} as ServerConnection,
    presence: {} as ServerPresence,
    realtimeViewerId: () => viewerId,
    viewerId: () => viewerId,
    projectedMemberIds: () => null,
    keepRecent
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

  /** Use every store of a room, as a room view does. */
  function useRoom(rooms: RoomStores, roomId: string) {
    return [rooms.messages(roomId), rooms.files(roomId), rooms.pins(roomId), rooms.members(roomId)];
  }

  it('keeps the stores of only the most recently used rooms', () => {
    const rooms = makeRooms();
    const first = useRoom(rooms, 'R0');
    const second = useRoom(rooms, 'R1');
    for (let index = 2; index < RECENT_ROOM_LIMIT; index++) useRoom(rooms, `R${index}`);
    // Using a room again makes it the most recently used one.
    rooms.messages('R0');

    useRoom(rooms, 'new');
    for (const store of second) expect(store.dispose).toHaveBeenCalledOnce();
    for (const store of first) expect(store.dispose).not.toHaveBeenCalled();
    expect(rooms.loaded('R1')).toBeUndefined();
    expect(rooms.messages('R1')).not.toBe(second[0]);
  });

  it('keeps as many rooms and threads as keepRecent allows', () => {
    const rooms = makeRooms('viewer', { rooms: 1, threads: 2 });
    const first = useRoom(rooms, 'A');
    useRoom(rooms, 'B');
    expect(first[0].dispose).toHaveBeenCalledOnce();

    const [oldest, ...kept] = [
      rooms.thread('A', 'T0'),
      rooms.thread('A', 'T1'),
      rooms.thread('A', 'T2')
    ];
    expect(oldest.dispose).toHaveBeenCalledOnce();
    for (const thread of kept) expect(thread.dispose).not.toHaveBeenCalled();
  });

  it('keeps the thread timelines of a disposed room', () => {
    const rooms = makeRooms();
    const roomStores = useRoom(rooms, 'A');
    const thread = rooms.thread('A', 'T');
    for (let index = 0; index < RECENT_ROOM_LIMIT; index++) useRoom(rooms, `B${index}`);

    expect(roomStores[0].dispose).toHaveBeenCalledOnce();
    expect(thread.dispose).not.toHaveBeenCalled();
    expect(rooms.thread('A', 'T')).toBe(thread);
    expect(rooms.timelines('A')).toEqual([thread]);
  });

  it('does not use a room when only its threads are used', () => {
    const rooms = makeRooms();
    const roomStores = useRoom(rooms, 'A');
    for (let index = 0; index < RECENT_ROOM_LIMIT * 2; index++) rooms.thread(`B${index}`, 'T');
    expect(roomStores[0].dispose).not.toHaveBeenCalled();
  });

  it('keeps only the most recently used thread stores', () => {
    const rooms = makeRooms();
    const first = rooms.thread('A', 'T0');
    const second = rooms.thread('A', 'T1');
    for (let index = 2; index < RECENT_THREAD_LIMIT; index++) rooms.thread('B', `T${index}`);
    // Using a thread again makes it the most recently used one.
    rooms.thread('A', 'T0');

    rooms.thread('B', 'new');
    expect(second.dispose).toHaveBeenCalledOnce();
    expect(first.dispose).not.toHaveBeenCalled();
    expect(rooms.timelines('A')).toEqual([first]);
    const replacement = rooms.thread('A', 'T1');
    expect(replacement).not.toBe(second);
  });

  it('forgets a room when its last store is disposed', () => {
    const rooms = makeRooms();
    rooms.thread('X', 'T');
    for (let index = 0; index < RECENT_THREAD_LIMIT; index++) rooms.thread('B', `T${index}`);
    expect(rooms.loaded('X')).toBeUndefined();
    expect(rooms.roomIds()).toEqual(['B']);
  });

  it('forgets thread stores that a room disposes', () => {
    const rooms = makeRooms();
    const forgotten = rooms.thread('A', 'T');
    rooms.clearMessageAccess('A', true);
    for (let index = 0; index < RECENT_THREAD_LIMIT; index++) rooms.thread('B', `T${index}`);
    expect(forgotten.dispose).toHaveBeenCalledOnce();
    expect(rooms.timelines('B')).toHaveLength(RECENT_THREAD_LIMIT);
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

    for (const store of [...disposed, members]) expect(store.dispose).toHaveBeenCalled();
    // Disposal of the registry also clears the member list.
    expect(members.resetProjectionState).toHaveBeenCalledOnce();
    expect(rooms.entries()).toEqual([]);
    expect(rooms.messages('A')).not.toBe(disposed[0]);
  });
});
