/**
 * Frontend state of one server that the Chatto client does not own: pending
 * highlights, message search sessions, and the admin room-layout editor.
 *
 * `serverUi(store)` returns the state of one server store. It is created on
 * first use and lives as long as the store: a new store, for example after an
 * account change, gets new state. Search results and the layout editor copy
 * server data, so the state subscribes to the store's boundary events and
 * clears those copies itself.
 */

import { createAdminRoomLayoutAPI } from '@chatto/client/api/adminRoomLayout';
import { createMessageSearchAPI, type MessageSearchAPI } from '@chatto/client/api/messageSearch';
import { createRoomCommandAPI } from '@chatto/client/api/rooms';
import type { ServerStateStore } from '@chatto/client/server/store';
import { AdminRoomLayoutStore } from './adminRoomLayout';
import { MessageSearchStore } from './messageSearch';
import { PendingHighlightStore } from './pendingHighlight';

/** Per-room searches kept at the same time. The least recently used one goes first. */
const MAX_RETAINED_ROOM_SEARCHES = 10;

/** Realtime events that can change message search results. */
const SEARCHABLE_MESSAGE_EVENTS = new Set<string | undefined>([
  'messagePosted',
  'messageEdited',
  'messageRetracted',
  'reactionAdded',
  'reactionRemoved',
  'messagePinned',
  'messageUnpinned',
  'assetDeleted'
]);

/** The frontend state of one server store; see the module documentation. */
export class ServerUi {
  /** One-shot highlight targets for in-app navigation. */
  readonly pendingHighlights = new PendingHighlightStore();
  /** Server-wide message search. */
  readonly messageSearch: MessageSearchStore;
  /** The admin room-layout editor. Keep it current with {@link activateAdminRoomLayout}. */
  readonly adminRoomLayout: AdminRoomLayoutStore;
  readonly #store: ServerStateStore;
  readonly #searchAPI: MessageSearchAPI;
  /** Room searches, least recently used first. */
  readonly #roomSearches = new Map<string, MessageSearchStore>();
  #adminRoomLayoutSubscriptions = 0;

  constructor(store: ServerStateStore) {
    this.#store = store;
    const connection = store.connection;
    this.#searchAPI = connection.getAPI(createMessageSearchAPI);
    this.messageSearch = new MessageSearchStore(this.#searchAPI, () => store.isAuthenticated);
    this.adminRoomLayout = new AdminRoomLayoutStore(
      connection.getAPI(createAdminRoomLayoutAPI),
      connection.getAPI(createRoomCommandAPI)
    );

    store.onReset(({ retainView }) => {
      if (retainView) return;
      this.pendingHighlights.clear();
      this.adminRoomLayout.resetProjectionState();
      this.#forEachSearch((search) => search.clearResults());
    });
    store.onRoomAccessLost(({ roomId }) =>
      this.#forRoomSearch(roomId, (search) => search.revokeRoom(roomId))
    );
    store.onUserDeleted((userId) =>
      this.#forEachSearch((search) => search.invalidateAuthor(userId))
    );
    store.onPermissionsChanged(() => {
      // Search holds plaintext outside the projection: fence it at once.
      this.#forEachSearch((search) => search.refreshPermissions());
      if (this.#adminRoomLayoutActive) return this.adminRoomLayout.refreshPermissions();
      this.adminRoomLayout.resetProjectionState();
    });
    store.onUpdate((update) => {
      const resource = update.resource?.case;
      if (update.reset || resource === 'rooms' || resource === 'roomGroups') {
        if (this.#adminRoomLayoutActive) this.adminRoomLayout.requestProjectionRefresh();
      }
      const event = update.event?.event;
      if (!SEARCHABLE_MESSAGE_EVENTS.has(event?.case)) return;
      const roomId = (event?.value as { roomId?: string } | undefined)?.roomId;
      if (roomId) this.#forRoomSearch(roomId, (search) => search.invalidateRoom(roomId));
      else this.#forEachSearch((search) => search.clearResults());
    });
    store.onDispose(() => {
      this.adminRoomLayout.deactivateProjectionRefresh();
      this.#adminRoomLayoutSubscriptions = 0;
      this.pendingHighlights.clear();
      this.#forEachSearch((search) => search.reset());
      this.#roomSearches.clear();
    });
  }

  /**
   * The message search of one room. Only {@link MAX_RETAINED_ROOM_SEARCHES}
   * room searches stay; a new one removes the least recently used one.
   */
  roomSearch(roomId: string): MessageSearchStore {
    const existing = this.#roomSearches.get(roomId);
    if (existing) {
      this.#roomSearches.delete(roomId);
      this.#roomSearches.set(roomId, existing);
      return existing;
    }
    if (this.#roomSearches.size >= MAX_RETAINED_ROOM_SEARCHES) {
      const [oldestRoomId, evicted] = this.#roomSearches.entries().next().value!;
      this.#roomSearches.delete(oldestRoomId);
      // A selector can create this store while a UI renders. Release the old
      // store now and clear its reactive state after the render. The captured
      // store cannot be a replacement that a later call creates for that room.
      queueMicrotask(() => evicted.reset());
    }
    const search = new MessageSearchStore(this.#searchAPI, () => this.#store.isAuthenticated);
    this.#roomSearches.set(roomId, search);
    return search;
  }

  /** Keep the admin layout editor current while its route is mounted. Returns the release function. */
  activateAdminRoomLayout(): () => void {
    this.#adminRoomLayoutSubscriptions += 1;
    if (this.#adminRoomLayoutSubscriptions === 1) void this.adminRoomLayout.refresh();
    return () => {
      this.#adminRoomLayoutSubscriptions = Math.max(0, this.#adminRoomLayoutSubscriptions - 1);
      if (!this.#adminRoomLayoutActive) this.adminRoomLayout.deactivateProjectionRefresh();
    };
  }

  get #adminRoomLayoutActive(): boolean {
    return this.#adminRoomLayoutSubscriptions > 0;
  }

  #forEachSearch(callback: (search: MessageSearchStore) => void): void {
    callback(this.messageSearch);
    for (const search of this.#roomSearches.values()) callback(search);
  }

  #forRoomSearch(roomId: string, callback: (search: MessageSearchStore) => void): void {
    callback(this.messageSearch);
    const search = this.#roomSearches.get(roomId);
    if (search) callback(search);
  }
}

const uiByStore = new WeakMap<ServerStateStore, ServerUi>();

/** The frontend state of one server store; see {@link ServerUi}. */
export function serverUi(store: ServerStateStore): ServerUi {
  let ui = uiByStore.get(store);
  if (!ui) {
    ui = new ServerUi(store);
    uiByStore.set(store, ui);
  }
  return ui;
}
