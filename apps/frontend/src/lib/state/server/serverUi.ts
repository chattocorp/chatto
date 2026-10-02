/**
 * Frontend state of one server that the Chatto client does not own: the
 * voice call, read views and notification attention, sidebar navigation,
 * optimistic unread and membership state, pending highlights, message search
 * sessions, and the admin room-layout editor.
 *
 * `serverUi(store)` returns the state of one server store. It lives as long as
 * the store: a new store, for example after an account change, gets new
 * state. `$lib/client` creates it for each store when the registry creates
 * the store, outside reactive reads, so that Svelte tracks its `$state`.
 * Search results and the layout editor copy server data, and the voice call
 * holds media, so the state subscribes to the store's boundary events and
 * clears or releases them itself.
 */

import { createAdminRoomLayoutAPI } from '$lib/api/adminRoomLayout';
import { createMemberDirectoryAPI } from '@chatto/client/api/memberDirectory';
import { createMessageSearchAPI, type MessageSearchAPI } from '@chatto/client/api/messageSearch';
import { createRoomCommandAPI } from '@chatto/client/api/rooms';
import { createVoiceCallAPI } from '@chatto/client/api/voiceCalls';
import type { RealtimeEvent } from '@chatto/client';
import type { ServerStateStore } from '@chatto/client/server/store';
import { ActiveCallRoomsState } from './activeCallRooms';
import { AdminRoomLayoutStore } from './adminRoomLayout';
import { CallPreferencesState } from './callPreferences.svelte';
import type { CallPermissions } from './callTypes';
import { NavigationStore } from './navigation';
import { NotificationAttention } from './notificationAttention';
import { ReadViewRegistry } from './readViews';
import { RoomDirectoryStore } from './roomDirectory';
import { RoomUnreadStore } from './roomUnread';
import { MessageSearchStore } from './messageSearch';
import { PendingHighlightStore } from './pendingHighlight';
import { VoiceCallState } from './voiceCall.svelte';

/**
 * What the server icon shows: `notification` for an occurrence that needs
 * attention, `unread` for unread rooms without one, or nothing.
 */
export type ServerIndicator = 'notification' | 'unread' | null;

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

/** The viewer's call permissions in a room. Missing permission data denies access. */
function callPermissions(store: ServerStateStore, roomId: string): CallPermissions {
  const state = store.projection.rooms.get(roomId)?.viewerState;
  const granted = (permission: string) =>
    state?.isMember === true &&
    state.permissions.some((grant) => grant.permission === permission && grant.granted);
  return {
    start: granted('call.start'),
    join: granted('call.join'),
    voice: granted('call.voice'),
    camera: granted('call.camera'),
    screenshare: granted('call.screenshare')
  };
}

/** The frontend state of one server store; see the module documentation. */
export class ServerUi {
  /** The LiveKit call of this server. */
  readonly voiceCall: VoiceCallState;
  /** Active calls from the projection, with this client's own call overlaid. */
  readonly activeCallRooms: ActiveCallRoomsState;
  /** Panes that the user reads now; they need no notification attention. */
  readonly readViews = new ReadViewRegistry();
  /** Notifications that need attention, without viewed ones. */
  readonly attention: NotificationAttention;
  /** Rooms and groups of the sidebar, with notification counts. */
  readonly navigation: NavigationStore;
  /** Optimistic unread state of rooms over the projection. */
  readonly roomUnread: RoomUnreadStore;
  /** Optimistic room membership commands and join previews. */
  readonly roomDirectory: RoomDirectoryStore;
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
    this.voiceCall = new VoiceCallState(
      connection.getAPI(createVoiceCallAPI),
      (roomId) => callPermissions(store, roomId),
      new CallPreferencesState(store.serverId)
    );
    this.activeCallRooms = new ActiveCallRoomsState(
      () => this.voiceCall,
      () => store.projection.activeCalls
    );
    this.attention = new NotificationAttention(store.notifications, this.readViews);
    this.navigation = new NavigationStore(store.roomList, () => this.attention.counts);
    this.roomUnread = new RoomUnreadStore(() => store.projection);
    this.roomDirectory = new RoomDirectoryStore(
      this.navigation,
      connection.getAPI(createMemberDirectoryAPI),
      connection.getAPI(createRoomCommandAPI)
    );
    this.#searchAPI = connection.getAPI(createMessageSearchAPI);
    this.messageSearch = new MessageSearchStore(this.#searchAPI, () => store.isAuthenticated);
    this.adminRoomLayout = new AdminRoomLayoutStore(
      connection.getAPI(createAdminRoomLayoutAPI),
      connection.getAPI(createRoomCommandAPI)
    );

    store.onReset(({ retainView }) => {
      if (retainView) return;
      this.voiceCall.handleProjectionReset();
      this.roomDirectory.resetOptimisticState();
      this.roomUnread.clear();
      this.pendingHighlights.clear();
      this.adminRoomLayout.resetProjectionState();
      this.#forEachSearch((search) => search.clearResults());
    });
    store.onRoomAccessLost(({ roomId, messagesOnly, removed }) => {
      if (!messagesOnly) this.voiceCall.handleRoomAccessRevoked(roomId);
      if (removed) {
        this.roomDirectory.removeMembershipProjection(roomId);
        this.roomUnread.removeRoomProjection(roomId);
      }
      this.#forRoomSearch(roomId, (search) => search.revokeRoom(roomId));
    });
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
      // A reset temporarily removes room data, not call access. Keep the media
      // session until fresh permissions arrive; the server also enforces
      // LiveKit access.
      if (resource === 'rooms') {
        void this.voiceCall.reconcilePermissions();
        // The projected rooms confirm or replace optimistic state.
        for (const [roomId, room] of store.projection.rooms) {
          this.roomDirectory.acknowledgeMembership(roomId, room.viewerState?.isMember);
          this.roomUnread.acknowledgeRoomProjection(roomId, room.viewerState?.hasUnread);
        }
      }
      if (resource === 'viewer') this.roomUnread.acknowledgeViewerProjection();
      if (update.reset || resource === 'rooms' || resource === 'roomGroups') {
        if (this.#adminRoomLayoutActive) this.adminRoomLayout.requestProjectionRefresh();
      }
      this.#handleCallEvent(update.event);
      const event = update.event?.event;
      if (!SEARCHABLE_MESSAGE_EVENTS.has(event?.case)) return;
      const roomId = (event?.value as { roomId?: string } | undefined)?.roomId;
      if (roomId) this.#forRoomSearch(roomId, (search) => search.invalidateRoom(roomId));
      else this.#forEachSearch((search) => search.clearResults());
    });
    store.onDispose(() => {
      // Release call media first; nothing waits for the server.
      this.voiceCall.dispose();
      this.readViews.clear();
      this.roomUnread.clear();
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

  /**
   * The server icon's indicator. Notifications take precedence over unread
   * rooms; direct messages count like rooms.
   */
  serverIndicator(): ServerIndicator {
    if (this.attention.counts.unreadNotificationCount > 0) return 'notification';
    if (this.attention.hasNonDMNotifications()) return 'notification';
    if (this.attention.hasDMNotifications()) return 'notification';
    if (this.roomUnread.hasAnyUnread) return 'unread';
    return null;
  }

  /** Forward call participant and end events to the voice call. */
  #handleCallEvent(event: RealtimeEvent | null): void {
    const payload = event?.event;
    if (!event || !payload) return;
    switch (payload.case) {
      case 'voiceCallParticipantJoined':
      case 'voiceCallParticipantLeft':
        this.voiceCall.handleParticipantTransition({
          eventId: event.id,
          kind: payload.case === 'voiceCallParticipantJoined' ? 'join' : 'leave',
          roomId: payload.value.roomId,
          callId: payload.value.callId || null,
          actorId: event.actorId || null,
          viewerId: this.#store.projectionViewerId ?? this.#store.viewerId
        });
        return;
      case 'voiceCallEnded':
        this.voiceCall.handleCallEndedEvent(payload.value.roomId, payload.value.callId || null);
        return;
    }
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

/**
 * Use `ui` as the state of `store`. Component specs use it to give a fake
 * store fake UI state; see `createTestServerScope`.
 */
export function setServerUiForTests(store: ServerStateStore, ui: object): void {
  uiByStore.set(store, ui as ServerUi);
}
