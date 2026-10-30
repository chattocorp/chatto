import { LRUCache } from 'lru-cache';
import { MessagesStore } from '../room/messages/MessagesStore.js';
import { RoomFilesStore } from '../room/files.js';
import { RoomMembersStore } from '../room/members.js';
import { RoomPinsStore } from '../room/pins.js';
import { RoomNoticeStore } from '../room/notice.js';
import { clearRoomPinsSeenMarker } from '../room/pins.js';
import type { ServerPresence } from './presence.js';
import type { ServerConnection } from './serverConnection.js';

/** The stores of one room that exist. A missing field has no store yet, or no longer. */
export type LoadedRoomStores = {
  readonly messages?: MessagesStore;
  readonly files?: RoomFilesStore;
  readonly pins?: RoomPinsStore;
  readonly notice?: RoomNoticeStore;
  readonly members?: RoomMembersStore;
  /** Thread timelines by thread root event ID. */
  readonly threads: Readonly<Record<string, MessagesStore>>;
};

type StoreKind = 'messages' | 'files' | 'pins' | 'members' | 'notice';

/**
 * The number of most recently used rooms whose stores a registry keeps. The
 * stores stay current through realtime delivery, so a room used again needs
 * no read. The registry disposes the stores of less recently used rooms.
 */
export const RECENT_ROOM_LIMIT = 10;

/** Like {@link RECENT_ROOM_LIMIT}, for thread timelines. */
export const RECENT_THREAD_LIMIT = 20;

/**
 * How many recently used rooms and threads a client keeps live stores for;
 * it disposes the stores of older ones. The defaults are
 * {@link RECENT_ROOM_LIMIT} and {@link RECENT_THREAD_LIMIT}. Each value is a
 * positive integer. A host that uses more rooms or threads at the same time
 * needs a higher value.
 */
export type KeepRecent = { rooms?: number; threads?: number };

/** The limits of `keepRecent` with defaults. Throws for a value that is not a positive integer. */
export function keepRecentLimits(keepRecent: KeepRecent = {}): Required<KeepRecent> {
  const { rooms = RECENT_ROOM_LIMIT, threads = RECENT_THREAD_LIMIT } = keepRecent;
  for (const [name, value] of [
    ['rooms', rooms],
    ['threads', threads]
  ] as const) {
    if (!Number.isInteger(value) || value < 1) {
      throw new Error(`keepRecent.${name} must be a positive integer`);
    }
  }
  return { rooms, threads };
}

type RoomEntry = {
  -readonly [K in keyof LoadedRoomStores]: LoadedRoomStores[K];
} & {
  threads: Record<string, MessagesStore>;
};

type ThreadLocation = { roomId: string; threadRootEventId: string };

/** The part of {@link RoomStores} that components use: the store accessors. */
export type RoomStoreAccess = Pick<
  RoomStores,
  'messages' | 'thread' | 'files' | 'pins' | 'members' | 'notice'
>;

/** What {@link RoomStores} needs from its server. */
export type RoomStoresOptions = {
  serverId: string;
  connection: ServerConnection;
  presence: ServerPresence;
  /** The viewer that timelines interpret realtime events for. */
  realtimeViewerId: () => string | null;
  /** The viewer ID for device-local pin markers. */
  viewerId: () => string | null;
  /** The complete member IDs of a room from the realtime projection, or null. */
  projectedMemberIds: (roomId: string) => readonly string[] | null;
  /** Whether server recovery owns the first read of a newly created member store. */
  deferMemberLoading?: () => boolean;
  /** The number of recently used rooms and threads to keep; see {@link KeepRecent}. */
  keepRecent?: KeepRecent;
};

/**
 * The room-scoped stores of one server: room and thread timelines, file lists,
 * pins, and members.
 *
 * Each accessor creates its store on first use and then returns the same store.
 * A call to an accessor uses the room or thread. The registry keeps the stores
 * of the most recently used rooms and threads, by default
 * {@link RECENT_ROOM_LIMIT} and {@link RECENT_THREAD_LIMIT} (see
 * {@link KeepRecent}), and disposes the others. A disposed store gets no more realtime updates; the
 * accessor then returns a new store. So a host must not use more rooms or
 * threads at the same time than these limits. A host that keeps a timeline
 * across a long wait checks `isDisposed` and gets the timeline again. Use of a
 * thread does not keep its room's stores. Clearing a room's message access with
 * `forget` removes its timelines, file list, and pin list.
 *
 * The registry is not reactive on purpose. The stores are reactive, and a
 * selector can create a store while a UI framework evaluates a derived value.
 */
export class RoomStores {
  readonly #options: RoomStoresOptions;
  #rooms: Record<string, RoomEntry> = Object.create(null);
  /** The rooms whose own stores were used, by recency of use. */
  readonly #recentRooms: LRUCache<string, true>;
  /** Thread timelines, like {@link #recentRooms}. */
  readonly #recentThreads: LRUCache<MessagesStore, ThreadLocation>;

  constructor(options: RoomStoresOptions) {
    this.#options = options;
    const limits = keepRecentLimits(options.keepRecent);
    this.#recentRooms = new LRUCache({
      max: limits.rooms,
      dispose: (_, roomId, reason) => {
        if (reason === 'evict') this.#disposeRoom(roomId);
      }
    });
    this.#recentThreads = new LRUCache({
      max: limits.threads,
      dispose: (location, store, reason) => {
        if (reason === 'evict') this.#disposeThread(store, location);
      }
    });
  }

  #entry(roomId: string): RoomEntry {
    return (this.#rooms[roomId] ??= { threads: Object.create(null) });
  }

  /** The entry of a room whose own stores a caller uses now. */
  #usedRoom(roomId: string): RoomEntry {
    const entry = this.#entry(roomId);
    this.#recentRooms.set(roomId, true);
    return entry;
  }

  /** The stores of one room that exist, without creating any. */
  loaded(roomId: string): LoadedRoomStores | undefined {
    return this.#rooms[roomId];
  }

  /** The rooms that this registry knows, and the stores that exist for each. A room can have none. */
  entries(): [roomId: string, stores: LoadedRoomStores][] {
    return Object.entries(this.#rooms);
  }

  /** Every store of one kind that exists, in all rooms. */
  all<K extends StoreKind>(kind: K): NonNullable<RoomEntry[K]>[] {
    return Object.values(this.#rooms).flatMap((entry) => (entry[kind] ? [entry[kind]] : []));
  }

  /** The room and thread timelines that exist for one room, or for all rooms. */
  timelines(roomId?: string): MessagesStore[] {
    return this.#select(roomId).flatMap((entry) => [
      ...(entry.messages ? [entry.messages] : []),
      ...Object.values(entry.threads)
    ]);
  }

  #select(roomId: string | undefined): RoomEntry[] {
    if (roomId === undefined) return Object.values(this.#rooms);
    const entry = this.#rooms[roomId];
    return entry ? [entry] : [];
  }

  /** Rooms with a timeline, file list, pin list, or member list. */
  roomIds(): string[] {
    return Object.entries(this.#rooms).flatMap(([roomId, entry]) =>
      this.#isEmpty(entry) ? [] : [roomId]
    );
  }

  /** The timeline of one room. */
  messages(roomId: string): MessagesStore {
    return (this.#usedRoom(roomId).messages ??= new MessagesStore(
      this.#options.connection,
      this.#options.realtimeViewerId,
      { roomId }
    ));
  }

  /** The timeline of one thread. It receives projection updates after it is opened. */
  thread(roomId: string, threadRootEventId: string): MessagesStore {
    const store = (this.#entry(roomId).threads[threadRootEventId] ??= new MessagesStore(
      this.#options.connection,
      this.#options.realtimeViewerId,
      { roomId, threadRootEventId }
    ));
    this.#recentThreads.set(store, { roomId, threadRootEventId });
    return store;
  }

  #disposeThread(store: MessagesStore, { roomId, threadRootEventId }: ThreadLocation): void {
    store.dispose();
    const entry = this.#rooms[roomId];
    if (entry?.threads[threadRootEventId] !== store) return;
    delete entry.threads[threadRootEventId];
    if (this.#isEmpty(entry)) delete this.#rooms[roomId];
  }

  #isEmpty(entry: RoomEntry): boolean {
    return (
      !entry.messages &&
      !entry.files &&
      !entry.pins &&
      !entry.notice &&
      !entry.members &&
      !Object.keys(entry.threads).length
    );
  }

  /** Dispose the room's own stores. Its thread timelines have their own lifetime. */
  #disposeRoom(roomId: string): void {
    const entry = this.#rooms[roomId];
    if (!entry) return;
    entry.messages?.dispose();
    entry.messages = undefined;
    entry.files?.dispose();
    entry.files = undefined;
    entry.pins?.dispose();
    entry.pins = undefined;
    entry.members?.dispose();
    entry.members = undefined;
    entry.notice?.dispose();
    entry.notice = undefined;
    if (this.#isEmpty(entry)) delete this.#rooms[roomId];
  }

  /** The file list of one room. It loads when a consumer calls its `retain()`. */
  files(roomId: string): RoomFilesStore {
    return (this.#usedRoom(roomId).files ??= new RoomFilesStore(this.#options.connection, roomId));
  }

  /** The pin list of one room. */
  pins(roomId: string): RoomPinsStore {
    return (this.#usedRoom(roomId).pins ??= new RoomPinsStore(
      this.#options.connection,
      this.#options.serverId,
      this.#options.viewerId(),
      roomId
    ));
  }

  /** The current member-only notice. Loads while a consumer retains it. */
  notice(roomId: string): RoomNoticeStore {
    return (this.#usedRoom(roomId).notice ??= new RoomNoticeStore(
      this.#options.connection,
      roomId
    ));
  }

  /** The member list of one room. */
  members(roomId: string): RoomMembersStore {
    const entry = this.#usedRoom(roomId);
    if (entry.members) return entry.members;
    return (entry.members = new RoomMembersStore(roomId, this.#options.connection, {
      deferInitialLoad: this.#options.deferMemberLoading?.(),
      presence: this.#options.presence,
      projectedMemberIds: () => this.#options.projectedMemberIds(roomId)
    }));
  }

  /**
   * Remove the plaintext of one room at an authorization boundary. With
   * `forget`, also dispose the room's timelines, file list, and pin list.
   */
  clearMessageAccess(roomId: string, forget = false): void {
    clearRoomPinsSeenMarker(this.#options.serverId, this.#options.viewerId(), roomId);
    const entry = this.#rooms[roomId];
    if (!entry) return;
    entry.messages?.clearForAccessRevocation();
    entry.files?.reset();
    entry.pins?.reset({ accessRevoked: true });
    for (const thread of Object.values(entry.threads)) thread.clearForAccessRevocation();
    if (!forget) return;
    this.#disposeTimelines(entry);
    entry.files?.dispose();
    entry.files = undefined;
  }

  /** Load again the stores of one room that {@link clearMessageAccess} cleared. */
  restoreAccess(roomId: string): void {
    const entry = this.#rooms[roomId];
    if (!entry) return;
    entry.messages?.restoreAfterAccessGrant();
    entry.files?.restoreAfterAccessGrant();
    entry.pins?.restoreAfterAccessGrant();
    for (const thread of Object.values(entry.threads)) thread.restoreAfterAccessGrant();
  }

  #disposeTimelines(entry: RoomEntry): void {
    entry.messages?.dispose();
    entry.messages = undefined;
    entry.pins?.dispose();
    entry.pins = undefined;
    for (const thread of Object.values(entry.threads)) {
      thread.dispose();
      this.#recentThreads.delete(thread);
    }
    entry.threads = Object.create(null);
  }

  /**
   * One handler for each store that a projection reset makes stale. Run them
   * with `runResetHandlers`, so that one failure does not stop the others.
   */
  resetHandlers(): (() => void)[] {
    return [
      ...this.all('members').map((store) => () => store.resetProjectionState()),
      ...this.all('notice').map((store) => () => store.reset()),
      ...this.timelines().map((store) => () => store.resetProjectionState()),
      ...this.all('files').map((store) => () => store.reset({ rehydrateRetained: true })),
      ...this.all('pins').map((store) => () => store.reset({ rehydrateRetained: true }))
    ];
  }

  /** Dispose every store. */
  dispose(): void {
    for (const entry of Object.values(this.#rooms)) {
      // Unlike an eviction, disposal of the registry clears the member lists too.
      entry.members?.resetProjectionState();
      entry.members?.dispose();
      this.#disposeTimelines(entry);
      entry.files?.dispose();
      entry.notice?.dispose();
    }
    this.#rooms = Object.create(null);
    this.#recentRooms.clear();
    this.#recentThreads.clear();
  }
}
