import type { MessageSearchAPI } from '$lib/api-client/messageSearch';
import { MessagesStore, RoomFilesStore, RoomMembersStore, RoomPinsStore } from '$lib/state/room';
import { clearRoomPinsSeenMarker } from '$lib/state/room/pins.svelte';
import { MessageSearchStore } from './messageSearch.svelte';
import type { ServerPresence } from './presence.svelte';
import type { ServerConnection } from './serverConnection.svelte';

/** Per-room searches kept at the same time. The least recently used one goes first. */
const MAX_RETAINED_ROOM_SEARCHES = 10;

/** The stores of one room that exist. A missing field has no store yet, or no longer. */
export type LoadedRoomStores = {
  readonly messages?: MessagesStore;
  readonly files?: RoomFilesStore;
  readonly pins?: RoomPinsStore;
  readonly members?: RoomMembersStore;
  readonly search?: MessageSearchStore;
  /** Thread timelines by thread root event ID. */
  readonly threads: Readonly<Record<string, MessagesStore>>;
};

type StoreKind = 'messages' | 'files' | 'pins' | 'members' | 'search';

type RoomEntry = {
  -readonly [K in keyof LoadedRoomStores]: LoadedRoomStores[K];
} & {
  threads: Record<string, MessagesStore>;
  /** Mounted consumers of each thread timeline. */
  threadRefs: Record<string, number>;
  /** Owner of the member store's derived fields. */
  disposeMembers?: () => void;
};

/** The part of {@link RoomStores} that components use: the store accessors. */
export type RoomStoreAccess = Pick<
  RoomStores,
  'messages' | 'thread' | 'retainThread' | 'releaseThread' | 'files' | 'pins' | 'search' | 'members'
>;

/** What {@link RoomStores} needs from its server. */
export type RoomStoresOptions = {
  serverId: string;
  connection: ServerConnection;
  presence: ServerPresence;
  messageSearchAPI: MessageSearchAPI;
  /** The viewer that timelines interpret realtime events for. */
  realtimeViewerId: () => string | null;
  /** The viewer ID for device-local pin markers. */
  viewerId: () => string | null;
  /** The complete member IDs of a room from the realtime projection, or null. */
  projectedMemberIds: (roomId: string) => readonly string[] | null;
  isAuthenticated: () => boolean;
};

/**
 * The room-scoped stores of one server: room and thread timelines, file lists,
 * pins, members, and per-room message search.
 *
 * Each accessor creates its store on first use and then returns the same
 * store until the registry is disposed, with two exceptions. Clearing a
 * room's message access with `forget` removes its timelines, file list, and
 * pin list. Only {@link MAX_RETAINED_ROOM_SEARCHES} per-room searches stay;
 * a new search removes the least recently used one.
 *
 * The registry is not reactive on purpose. The stores are reactive, and a
 * selector can create a store while Svelte evaluates a derived value.
 */
export class RoomStores {
  readonly #options: RoomStoresOptions;
  #rooms: Record<string, RoomEntry> = Object.create(null);
  /** Rooms with a search store, least recently used first. */
  #searchRecency: string[] = [];

  constructor(options: RoomStoresOptions) {
    this.#options = options;
  }

  #entry(roomId: string): RoomEntry {
    return (this.#rooms[roomId] ??= {
      threads: Object.create(null),
      threadRefs: Object.create(null)
    });
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
      entry.messages ||
      entry.files ||
      entry.pins ||
      entry.members ||
      Object.keys(entry.threads).length
        ? [roomId]
        : []
    );
  }

  /** The timeline of one room. */
  messages(roomId: string): MessagesStore {
    return (this.#entry(roomId).messages ??= new MessagesStore(
      this.#options.connection,
      this.#options.realtimeViewerId,
      { roomId }
    ));
  }

  /** The timeline of one thread. It receives projection updates after it is opened. */
  thread(roomId: string, threadRootEventId: string): MessagesStore {
    return (this.#entry(roomId).threads[threadRootEventId] ??= new MessagesStore(
      this.#options.connection,
      this.#options.realtimeViewerId,
      { roomId, threadRootEventId }
    ));
  }

  /** Keep a mounted thread timeline until its last consumer unmounts. */
  retainThread(roomId: string, threadRootEventId: string, store: MessagesStore): void {
    const entry = this.#rooms[roomId];
    if (entry?.threads[threadRootEventId] !== store) return;
    entry.threadRefs[threadRootEventId] = (entry.threadRefs[threadRootEventId] ?? 0) + 1;
  }

  /**
   * Release one consumer of a thread timeline. After the last one, the
   * timeline forgets its viewport but keeps its window for the next open.
   */
  releaseThread(roomId: string, threadRootEventId: string, store: MessagesStore): void {
    const entry = this.#rooms[roomId];
    if (entry?.threads[threadRootEventId] !== store) return;
    const remaining = (entry.threadRefs[threadRootEventId] ?? 1) - 1;
    if (remaining > 0) {
      entry.threadRefs[threadRootEventId] = remaining;
      return;
    }
    store.clearViewport();
    delete entry.threadRefs[threadRootEventId];
  }

  /** The file list of one room. It loads when a consumer retains it. */
  files(roomId: string): RoomFilesStore {
    return (this.#entry(roomId).files ??= new RoomFilesStore(this.#options.connection, roomId));
  }

  /** The pin list of one room. */
  pins(roomId: string): RoomPinsStore {
    return (this.#entry(roomId).pins ??= new RoomPinsStore(
      this.#options.connection,
      this.#options.serverId,
      this.#options.viewerId(),
      roomId
    ));
  }

  /** The message search of one room. */
  search(roomId: string): MessageSearchStore {
    const entry = this.#entry(roomId);
    const recency = this.#searchRecency.indexOf(roomId);
    if (recency >= 0) this.#searchRecency.splice(recency, 1);
    this.#searchRecency.push(roomId);
    if (entry.search) return entry.search;
    if (this.#searchRecency.length > MAX_RETAINED_ROOM_SEARCHES) {
      const oldest = this.#rooms[this.#searchRecency.shift()!];
      const evicted = oldest?.search;
      if (oldest) oldest.search = undefined;
      // A selector can create this store while Svelte renders. Release the old
      // store now and clear its reactive state after the render. The captured
      // store cannot be a replacement that a later call creates for that room.
      if (evicted) queueMicrotask(() => evicted.reset());
    }
    return (entry.search = new MessageSearchStore(
      this.#options.messageSearchAPI,
      this.#options.isAuthenticated
    ));
  }

  /** The member list of one room. It survives route changes. */
  members(roomId: string): RoomMembersStore {
    const entry = this.#entry(roomId);
    if (entry.members) return entry.members;
    // A route can create this store from a derived selector. Give its own
    // derived fields an owner that lasts until the registry is disposed.
    let created!: RoomMembersStore;
    entry.disposeMembers = $effect.root(() => {
      created = new RoomMembersStore(roomId, this.#options.connection, {
        presence: this.#options.presence,
        projectedMemberIds: () => this.#options.projectedMemberIds(roomId)
      });
    });
    return (entry.members = created);
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
    for (const thread of Object.values(entry.threads)) thread.dispose();
    entry.threads = Object.create(null);
    entry.threadRefs = Object.create(null);
  }

  /**
   * One handler for each store that a projection reset makes stale. Run them
   * with `runResetHandlers`, so that one failure does not stop the others.
   */
  resetHandlers(): (() => void)[] {
    return [
      ...this.all('members').map((store) => () => store.resetProjectionState()),
      ...this.timelines().map((store) => () => store.resetProjectionState()),
      ...this.all('files').map((store) => () => store.reset({ rehydrateRetained: true })),
      ...this.all('pins').map((store) => () => store.reset({ rehydrateRetained: true }))
    ];
  }

  /** Dispose every store. */
  dispose(): void {
    for (const entry of Object.values(this.#rooms)) {
      entry.members?.resetProjectionState();
      entry.disposeMembers?.();
      this.#disposeTimelines(entry);
      entry.files?.dispose();
      entry.search?.reset();
    }
    this.#rooms = Object.create(null);
    this.#searchRecency = [];
  }
}
