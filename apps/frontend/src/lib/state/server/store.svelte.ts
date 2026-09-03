/**
 * Bundles all server-scoped stores into a single class per server.
 * Created and managed by the ServerRegistry — do not instantiate directly.
 */

import { CurrentUserState } from '$lib/auth/currentUser.svelte';
import { ServerInfoState } from './state.svelte';
import type { PublicServerInfo } from '$lib/api-client/server';
import type { ServerPermissions, ViewerData } from './permissions';
import { NotificationStore } from './notifications.svelte';
import { RoomUnreadStore } from './roomUnread.svelte';
import { PendingHighlightStore } from './pendingHighlight.svelte';
import { VoiceCallState } from './voiceCall.svelte';
import { ActiveCallRoomsState } from './activeCallRooms.svelte';
import { NavigationStore } from './rooms.svelte';
import { RoomDirectoryStore } from './roomDirectory.svelte';
import { AdminRoomLayoutStore } from './adminRoomLayout.svelte';
import { createRoomCommandAPI } from '$lib/api-client/rooms';
import { createNotificationAPI } from '$lib/api-client/notifications';
import { createVoiceCallAPI } from '$lib/api-client/voiceCalls';
import { createAdminRoomLayoutAPI } from '$lib/api-client/adminRoomLayout';
import { createMessageSearchAPI, type MessageSearchAPI } from '$lib/api-client/messageSearch';
import { createMemberDirectoryAPI } from '$lib/api-client/memberDirectory';
import { createRoleAPI } from '$lib/api-client/roles';
import {
  createRealtimeResourceAPI,
  type RealtimeResourceAPI,
  type RealtimeResourceFamily
} from '$lib/api-client/realtimeResources';
import { eventBusManager } from './eventBus.svelte';
import { RealtimeProjectionUpdate, type ProjectionHandler } from '$lib/eventBus.svelte';
import type { ServerConnection } from './serverConnection.svelte';
import type { ServerRegistration } from './catalog.svelte';
import type { ServerSession } from './sessions.svelte';
import { playCallSound } from '$lib/audio/callSounds';
import { SvelteDate, SvelteMap, SvelteSet } from 'svelte/reactivity';
import { ServerProjectionStore } from './projection.svelte';
import { MessagesStore, RoomFilesStore, RoomPinsStore } from '$lib/state/room';
import { clearRoomPinsSeenMarker } from '$lib/state/room/pins.svelte';
import type { RoomMember } from '$lib/state/room';
import type { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import { mapDirectoryRoom, RoomKind } from '$lib/api-client/roomDirectory';
import { mapDirectoryMember } from '$lib/api-client/memberDirectory';
import { viewerResponseToState } from '$lib/api-client/viewer';
import { notifyUserSummaries } from '$lib/api-client/hooks';
import {
  clearUserSummaryCache,
  removeUserSummaryCacheEntry
} from '$lib/state/userSummaries.svelte';
import { avatarUserFromDirectoryMember } from './rooms.svelte';
import { mapNotificationOccurrencePage } from '$lib/api-client/notifications';
import { RealtimeProjectionSyncState } from './realtimeSync.svelte';
import type { GetViewerResponse } from '@chatto/api-types/api/v1/viewer_pb';
import { MessageSearchStore } from './messageSearch.svelte';
import { MentionRolesStore } from './mentionRoles.svelte';
import { TimelineEventKind, type TimelineEventView } from '$lib/render/timelineEvents';
import {
  reconcileRegisteredAdminRoomGroupQueries,
  purgeRegisteredRoomMemberQueries,
  refreshRegisteredAdminQueries,
  removeRegisteredAdminQueries,
  removeRegisteredAdminUserQueries,
  removeRegisteredServerQueries,
  resetRegisteredFollowedThreadQueries,
  scrubRegisteredFollowedThreadRoom,
  scrubRegisteredFollowedThreadUser,
  refreshRegisteredFollowedThreadQueries,
  scrubRegisteredRoomMemberUser
} from '$lib/query/cacheRegistry';

/**
 * What kind of indicator a server (or the DM area) should display.
 * - 'notification' = warning badge, has a pending mention/reply/room-message
 * - 'unread' = grey dot, has unread rooms but no unread notification occurrence
 * - null = no indicator
 */
export type ServerIndicator = 'notification' | 'unread' | null;

const MAX_RETAINED_ROOM_SEARCHES = 10;

function viewerAuthorizationLost(
  previous: GetViewerResponse | null,
  current: GetViewerResponse
): boolean {
  if (!previous) return false;
  if (previous.user?.profile?.id !== current.user?.profile?.id) return true;

  const currentGrants = new SvelteSet([
    ...(current.capabilities?.grants ?? [])
      .filter((grant) => grant.granted)
      .map((grant) => `capability:${grant.capability}`),
    ...(current.viewerPermissions?.permissions ?? [])
      .filter((grant) => grant.granted)
      .map((grant) => `permission:${grant.permission}`)
  ]);
  return [
    ...(previous.capabilities?.grants ?? [])
      .filter((grant) => grant.granted)
      .map((grant) => `capability:${grant.capability}`),
    ...(previous.viewerPermissions?.permissions ?? [])
      .filter((grant) => grant.granted)
      .map((grant) => `permission:${grant.permission}`)
  ].some((grant) => !currentGrants.has(grant));
}

const EMPTY_PERMISSIONS: ServerPermissions = {
  loaded: false,
  canViewAdmin: false,
  canStartDMs: false,
  canAdminViewUsers: false,
  canAdminManageAccounts: false,
  canAssignRoles: false,
  canAdminViewRoles: false,
  canAdminManageRoles: false,
  canAdminViewSystem: false,
  canAdminViewAudit: false,
  canManageInvites: false
};

export class ServerStateStore {
  readonly serverId: string;
  readonly currentUser: CurrentUserState;
  readonly serverInfo: ServerInfoState;
  readonly notifications: NotificationStore;
  readonly roomUnread: RoomUnreadStore;
  readonly pendingHighlights: PendingHighlightStore;
  readonly voiceCall: VoiceCallState;
  readonly activeCallRooms: ActiveCallRoomsState;
  readonly navigation: NavigationStore;
  readonly roomDirectory: RoomDirectoryStore;
  readonly adminRoomLayout: AdminRoomLayoutStore;
  readonly messageSearch: MessageSearchStore;
  readonly mentionRoles: MentionRolesStore;
  readonly projection = new ServerProjectionStore();
  /** Readiness and opaque resume position for this retained projection. */
  readonly realtimeSync = new RealtimeProjectionSyncState();
  /** Stable canonical reducer installed before a projection transport starts. */
  readonly realtimeProjectionHandler: ProjectionHandler = (event) =>
    this.ingestProjectionEvent(event);

  /** Per-server viewer permissions (loaded by ServerSidebarEntry). */
  permissions = $state<ServerPermissions>(EMPTY_PERMISSIONS);

  /**
   * Live reference to the registered server. Reads pick up `updateServer`
   * mutations (e.g. token refresh, name change) because the registry stores
   * servers in $state.
   */
  readonly #getSession: () => ServerSession;
  readonly #originServer: boolean;
  readonly #serverConnection: ServerConnection;
  // These registries are intentionally non-reactive. The stores they own are
  // reactive, while selector calls may occur during derived evaluation.
  #roomMessages: Record<string, MessagesStore> = Object.create(null);
  #roomFiles: Record<string, RoomFilesStore> = Object.create(null);
  #roomPins: Record<string, RoomPinsStore> = Object.create(null);
  #roomMessageSearch: Record<string, MessageSearchStore> = Object.create(null);
  #roomMessageSearchRecency: string[] = [];
  #threadMessages: Record<string, MessagesStore> = Object.create(null);
  #threadMessageRefCounts: Record<string, number> = Object.create(null);
  #adminRoomLayoutSubscriptions = 0;

  /** Disposer for the internal effect root that wires lifecycle reactivity. */
  readonly #disposeEffects: () => void;
  readonly #playedCallSoundEventIds: string[] = [];
  readonly #messageSearchAPI: MessageSearchAPI;
  readonly #realtimeResources: RealtimeResourceAPI;
  #realtimeProjectionGeneration = 0;
  #realtimeSnapshotPending = false;
  readonly #resourceRefreshes = new SvelteMap<RealtimeResourceFamily, Promise<void>>();
  readonly #pendingResourceRefreshes = new SvelteMap<
    RealtimeResourceFamily,
    { minimumCursor?: string; generation: number }
  >();
  #currentEventMinimumCursor: string | undefined;
  #userRefresh: Promise<void> | null = null;
  readonly #pendingUserRefreshIds = new SvelteSet<string>();
  #pendingUserRefreshCursor: string | undefined;
  #pendingUserRefreshGeneration = 0;
  #reconciliationError: unknown = null;
  readonly #messageWindowRefreshes = new WeakMap<MessagesStore, Promise<void>>();
  readonly #pendingMessageWindowRefreshes = new WeakMap<
    MessagesStore,
    { anchorEventId: string | null; forward: boolean; minimumCursor?: string; generation: number }
  >();
  readonly #projectionReconciliations = new SvelteSet<Promise<void>>();

  constructor(
    registration: ServerRegistration,
    getSession: () => ServerSession,
    originServer: boolean,
    serverConnection: ServerConnection,
    publicServerInfoLoader?: (baseUrl: string) => Promise<PublicServerInfo>,
    onAuthenticationRequired?: () => void
  ) {
    this.serverId = registration.id;
    this.#getSession = getSession;
    this.#originServer = originServer;
    this.#serverConnection = serverConnection;
    const cookieAuth = this.#cookieAuth;

    const connectAPIConfig = {
      serverId: serverConnection.serverId ?? registration.id,
      baseUrl: serverConnection.connectBaseUrl,
      bearerToken: serverConnection.bearerToken
    };
    const notificationAPI = serverConnection.getAPI(createNotificationAPI);
    const voiceCallAPI = serverConnection.getAPI(createVoiceCallAPI);
    const adminRoomLayoutAPI = serverConnection.getAPI(createAdminRoomLayoutAPI);
    const messageSearchAPI = serverConnection.getAPI(createMessageSearchAPI);
    this.#messageSearchAPI = messageSearchAPI;
    this.#realtimeResources = serverConnection.getAPI(createRealtimeResourceAPI);
    const memberDirectoryAPI = serverConnection.getAPI(createMemberDirectoryAPI);
    const roleAPI = serverConnection.getAPI(createRoleAPI);
    this.currentUser = new CurrentUserState(
      cookieAuth,
      connectAPIConfig,
      undefined,
      onAuthenticationRequired
    );
    this.serverInfo = new ServerInfoState(registration.url, publicServerInfoLoader);
    this.notifications = new NotificationStore(notificationAPI);
    this.roomUnread = new RoomUnreadStore(() => this.projection);
    const roomCommandAPI = serverConnection.getAPI(createRoomCommandAPI);
    this.pendingHighlights = new PendingHighlightStore();
    this.voiceCall = new VoiceCallState(voiceCallAPI);
    this.activeCallRooms = new ActiveCallRoomsState(this.voiceCall);
    this.navigation = new NavigationStore(this.projection, this.realtimeSync, this.notifications);
    this.roomDirectory = new RoomDirectoryStore(
      this.navigation,
      memberDirectoryAPI,
      roomCommandAPI
    );
    this.adminRoomLayout = new AdminRoomLayoutStore(adminRoomLayoutAPI, roomCommandAPI);
    this.messageSearch = new MessageSearchStore(messageSearchAPI);
    this.mentionRoles = new MentionRolesStore(roleAPI);

    // Apply the canonical projection delivered by this server's bus. Transient
    // envelopes are consumed only by components that need one-shot signals.
    this.#disposeEffects = $effect.root(() => {
      $effect(() => {
        const bus = eventBusManager.getBus(this.serverId);
        if (!bus) return;
        bus.projectionHandlers.add(this.realtimeProjectionHandler);
        return () => {
          bus.projectionHandlers.delete(this.realtimeProjectionHandler);
        };
      });
    });
  }

  /** Reject work whose resource boundary was superseded by a newer reset. */
  private requireCurrentRealtimeProjection(generation: number): void {
    if (generation !== this.#realtimeProjectionGeneration) {
      throw new Error('realtime projection read was superseded by a newer reset');
    }
  }

  /** Complete auxiliary reads and event reconciliation through `cursor`. */
  async completeRealtimeCatchUp(cursor: string): Promise<void> {
    if (this.#realtimeSnapshotPending) {
      const generation = this.#realtimeProjectionGeneration;
      const batches = await Promise.all(
        (['serverState', 'viewer', 'notifications'] as RealtimeResourceFamily[]).map((family) =>
          this.#realtimeResources.read(family, cursor)
        )
      );
      this.requireCurrentRealtimeProjection(generation);
      for (const resource of batches.flat()) {
        this.publishProjectionUpdate(new RealtimeProjectionUpdate({ resource }));
      }
      await Promise.all(
        [...Object.values(this.#roomMessages), ...Object.values(this.#threadMessages)].map(
          (store) =>
            store.hydrateRealtimeProjection(
              cursor,
              () => generation === this.#realtimeProjectionGeneration
            )
        )
      );
      this.requireCurrentRealtimeProjection(generation);
      this.#realtimeSnapshotPending = false;
    }
    while (
      this.#resourceRefreshes.size > 0 ||
      this.#userRefresh ||
      this.#projectionReconciliations.size > 0
    ) {
      await Promise.all([
        ...this.#resourceRefreshes.values(),
        ...(this.#userRefresh ? [this.#userRefresh] : []),
        ...this.#projectionReconciliations
      ]);
    }
    if (this.#reconciliationError) {
      const error = this.#reconciliationError;
      this.#reconciliationError = null;
      throw error;
    }
  }

  /** Stable room timeline owner used by routes as a rendering selector. */
  messagesForRoom(roomId: string): MessagesStore {
    let store = this.#roomMessages[roomId];
    if (store) return store;
    store = new MessagesStore(this.#serverConnection, () => this.currentUser.user?.id ?? null);
    store.setRoom(roomId);
    this.#roomMessages[roomId] = store;
    return store;
  }

  /** Return known follow state from a loaded canonical room timeline. */
  loadedThreadFollowState(roomId: string, threadRootEventId: string): boolean | null {
    const event = this.#roomMessages[roomId]?.getEventById(threadRootEventId);
    if (event?.event.kind !== TimelineEventKind.MessagePosted) return null;
    return event.event.viewerIsFollowingThread ?? null;
  }

  /** Check loaded canonical room timelines for one unread followed thread. */
  hasUnreadFollowedThreadInLoadedRooms(): boolean {
    return Object.values(this.#roomMessages).some((store) =>
      store.rootEvents.some(
        (event) =>
          event.event.kind === TimelineEventKind.MessagePosted &&
          event.event.viewerIsFollowingThread === true &&
          event.event.viewerHasUnreadThread === true
      )
    );
  }

  /** Stable lazy file-list owner for one room on this server. */
  filesForRoom(roomId: string): RoomFilesStore {
    let store = this.#roomFiles[roomId];
    if (store) return store;
    store = new RoomFilesStore(this.#serverConnection, roomId);
    this.#roomFiles[roomId] = store;
    return store;
  }

  /** Stable room pin owner, retained while its channel route is mounted. */
  pinsForRoom(roomId: string): RoomPinsStore {
    let store = this.#roomPins[roomId];
    if (store) return store;
    store = new RoomPinsStore(
      this.#serverConnection,
      this.serverId,
      this.currentUser.user?.id ?? this.#getSession().userId ?? '',
      roomId
    );
    this.#roomPins[roomId] = store;
    return store;
  }

  /** Stable transient message-search state scoped to one room. */
  messageSearchForRoom(roomId: string): MessageSearchStore {
    let store = this.#roomMessageSearch[roomId];
    if (store) {
      this.#touchRoomMessageSearch(roomId);
      return store;
    }
    if (this.#roomMessageSearchRecency.length >= MAX_RETAINED_ROOM_SEARCHES) {
      const oldestRoomId = this.#roomMessageSearchRecency.shift();
      if (oldestRoomId) {
        this.#roomMessageSearch[oldestRoomId]?.reset();
        delete this.#roomMessageSearch[oldestRoomId];
      }
    }
    store = new MessageSearchStore(this.#messageSearchAPI);
    this.#roomMessageSearch[roomId] = store;
    this.#roomMessageSearchRecency.push(roomId);
    return store;
  }

  /** Restore the canonical latest window when a route selects this room. */
  restoreProjectedRoomWindow(roomId: string): void {
    const messages = this.messagesForRoom(roomId);
    void messages.restoreLatestWindow();
  }

  private evictRetainedRoom(roomId: string): void {
    this.#roomMessages[roomId]?.dispose();
    delete this.#roomMessages[roomId];
    this.#roomPins[roomId]?.dispose();
    delete this.#roomPins[roomId];
    for (const [key, threadStore] of Object.entries(this.#threadMessages)) {
      if (!key.startsWith(`${roomId}\u0000`)) continue;
      threadStore.dispose();
      delete this.#threadMessages[key];
      delete this.#threadMessageRefCounts[key];
    }
  }

  /** Scrub every plaintext timeline mirror for a room at an authorization boundary. */
  private clearRoomAccess(roomId: string, forgetStores = false): void {
    clearRoomPinsSeenMarker(
      this.serverId,
      this.currentUser.user?.id ?? this.#getSession().userId ?? '',
      roomId
    );
    scrubRegisteredFollowedThreadRoom(this.serverId, roomId);
    this.voiceCall.handleRoomAccessRevoked(roomId);
    this.activeCallRooms.clearRoom(roomId);
    this.notifications.clearRoom(roomId);
    const roomStore = this.#roomMessages[roomId];
    roomStore?.clearForAccessRevocation();
    const filesStore = this.#roomFiles[roomId];
    filesStore?.reset();
    const pinsStore = this.#roomPins[roomId];
    pinsStore?.reset({ accessRevoked: true });
    if (forgetStores) {
      roomStore?.dispose();
      delete this.#roomMessages[roomId];
      filesStore?.dispose();
      delete this.#roomFiles[roomId];
      pinsStore?.dispose();
      delete this.#roomPins[roomId];
    }
    for (const [key, threadStore] of Object.entries(this.#threadMessages)) {
      if (!key.startsWith(`${roomId}\u0000`)) continue;
      threadStore.clearForAccessRevocation();
      if (forgetStores) {
        threadStore.dispose();
        delete this.#threadMessages[key];
        delete this.#threadMessageRefCounts[key];
      }
    }
  }

  /** Reacquire only mounted stores that were previously scrubbed for access loss. */
  private restoreRoomAccess(roomId: string): void {
    this.notifications.restoreRoom(roomId);
    this.#roomMessages[roomId]?.restoreAfterAccessGrant();
    this.#roomFiles[roomId]?.restoreAfterAccessGrant();
    this.#roomPins[roomId]?.restoreAfterAccessGrant();
    for (const [key, threadStore] of Object.entries(this.#threadMessages)) {
      if (key.startsWith(`${roomId}\u0000`)) threadStore.restoreAfterAccessGrant();
    }
  }

  /** Stable lazy thread timeline owner fed by the server projection once opened. */
  messagesForThread(roomId: string, threadRootEventId: string): MessagesStore {
    const key = `${roomId}\u0000${threadRootEventId}`;
    let store = this.#threadMessages[key];
    if (store) return store;
    store = new MessagesStore(this.#serverConnection, () => this.currentUser.user?.id ?? null);
    store.setThread(roomId, threadRootEventId);
    this.#threadMessages[key] = store;
    return store;
  }

  /** Keep a mounted thread mirror alive until its final consumer unmounts. */
  retainMessagesForThread(roomId: string, threadRootEventId: string, store: MessagesStore): void {
    const key = `${roomId}\u0000${threadRootEventId}`;
    if (this.#threadMessages[key] !== store) return;
    this.#threadMessageRefCounts[key] = (this.#threadMessageRefCounts[key] ?? 0) + 1;
  }

  /** Release and destroy an unmounted thread mirror and its decrypted rows. */
  releaseMessagesForThread(roomId: string, threadRootEventId: string, store: MessagesStore): void {
    const key = `${roomId}\u0000${threadRootEventId}`;
    if (this.#threadMessages[key] !== store) return;
    const remaining = (this.#threadMessageRefCounts[key] ?? 1) - 1;
    if (remaining > 0) {
      this.#threadMessageRefCounts[key] = remaining;
      return;
    }
    store.dispose();
    delete this.#threadMessages[key];
    delete this.#threadMessageRefCounts[key];
  }

  private ingestProjectionEvent(update: RealtimeProjectionUpdate): void {
    const previousViewer = this.projection.viewer;
    const previousUserIds = new SvelteSet(this.projection.users.keys());
    const previousRoomIds = new SvelteSet(this.projection.rooms.keys());
    const sourceEvent = update.event;
    let adminRoomLayoutChanged = update.reset;

    if (update.reset) {
      const generation = ++this.#realtimeProjectionGeneration;
      this.#realtimeSnapshotPending = true;
      this.#reconciliationError = null;
      this.#pendingResourceRefreshes.clear();
      this.#pendingUserRefreshIds.clear();
      this.#pendingUserRefreshCursor = undefined;
      this.#pendingUserRefreshGeneration = generation;
      resetRegisteredFollowedThreadQueries(this.serverId);
      this.resetProjectionMirrors();
      this.forEachMessageSearch((store) => store.clearResults());
    }

    this.projection.apply(update);
    const resource = update.resource;
    if (resource) {
      switch (resource.case) {
        case 'server':
          this.serverInfo.applyProjectionProfile(resource.value);
          break;
        case 'motd':
        case 'runtimeConfig':
          if (this.projection.serverState) {
            this.serverInfo.applyProjectionState(this.projection.serverState);
          }
          break;
        case 'viewer': {
          const response = resource.value;
          if (viewerAuthorizationLost(previousViewer, response)) {
            removeRegisteredAdminQueries(this.serverId);
          }
          const viewer = viewerResponseToState(response);
          this.currentUser.user = viewer.user;
          this.currentUser.loading = false;
          this.setPermissions(viewer);
          this.roomUnread.acknowledgeViewerProjection();
          break;
        }
        case 'users': {
          const members = resource.value.users.map(mapDirectoryMember);
          notifyUserSummaries(this.serverId, members);
          for (const userId of previousUserIds) {
            if (!this.projection.users.has(userId)) this.scrubRemovedUser(userId);
          }
          break;
        }
        case 'rooms':
          for (const [roomId, room] of this.projection.rooms) {
            this.roomDirectory.acknowledgeMembership(roomId, room.viewerState?.isMember);
            this.roomUnread.acknowledgeRoomProjection(roomId, room.viewerState?.hasUnread);
            if (room.viewerState?.isMember === false) this.clearRoomAccess(roomId);
            else if (room.viewerState?.isMember === true) this.restoreRoomAccess(roomId);
          }
          for (const roomId of previousRoomIds) {
            if (!this.projection.rooms.has(roomId)) this.scrubRemovedRoom(roomId);
          }
          adminRoomLayoutChanged = true;
          break;
        case 'roomGroups':
          reconcileRegisteredAdminRoomGroupQueries(
            this.serverId,
            resource.value.groups.map((group) => group.id)
          );
          adminRoomLayoutChanged = true;
          break;
        case 'notifications':
          this.notifications.replaceOccurrenceProjection(
            mapNotificationOccurrencePage(resource.value)
          );
          break;
        case 'activeCalls':
          this.activeCallRooms.replaceProjection(resource.value.calls);
          break;
        case undefined:
          break;
      }
    }

    if (sourceEvent?.event.case === 'messagePinned') {
      const pin = sourceEvent.event.value;
      this.#roomPins[pin.roomId]?.applyRealtimeChange(pin, true, sourceEvent.id);
    } else if (sourceEvent?.event.case === 'messageUnpinned') {
      const pin = sourceEvent.event.value;
      this.#roomPins[pin.roomId]?.applyRealtimeChange(pin, false, sourceEvent.id);
    }
    if (sourceEvent) {
      this.#currentEventMinimumCursor = update.cursor ?? undefined;
      try {
        this.invalidateRealtimeEvent(sourceEvent);
      } finally {
        this.#currentEventMinimumCursor = undefined;
      }
    }
    if (adminRoomLayoutChanged) this.scheduleAdminRoomLayoutRefresh();
  }
  private scrubRemovedUser(userId: string): void {
    scrubRegisteredFollowedThreadUser(this.serverId);
    scrubRegisteredRoomMemberUser(this.serverId, userId);
    removeRegisteredAdminUserQueries(this.serverId, userId);
    this.forEachMessageSearch((store) => store.invalidateAuthor(userId));
    removeUserSummaryCacheEntry(this.serverId, userId);
    this.notifications.scrubUser(userId);
    this.activeCallRooms.scrubUser(userId);
    for (const store of Object.values(this.#roomMessages)) store.scrubUserReferences(userId);
    for (const store of Object.values(this.#threadMessages)) store.scrubUserReferences(userId);
  }

  private scrubRemovedRoom(roomId: string): void {
    this.roomDirectory.removeMembershipProjection(roomId);
    this.roomUnread.removeRoomProjection(roomId);
    this.forRoomMessageSearch(roomId, (store) => store.revokeRoom(roomId));
    purgeRegisteredRoomMemberQueries(this.serverId, roomId);
    this.clearRoomAccess(roomId, true);
  }

  private refreshRealtimeResource(family: RealtimeResourceFamily, minimumCursor?: string): void {
    minimumCursor ??= this.#currentEventMinimumCursor;
    const generation = this.#realtimeProjectionGeneration;
    if (this.#resourceRefreshes.has(family)) {
      const pending = this.#pendingResourceRefreshes.get(family);
      this.#pendingResourceRefreshes.set(family, {
        minimumCursor:
          minimumCursor ?? (pending?.generation === generation ? pending.minimumCursor : undefined),
        generation
      });
      return;
    }
    const refresh = this.#realtimeResources
      .read(family, minimumCursor)
      .then(async (resources) => {
        this.requireCurrentRealtimeProjection(generation);
        for (const resource of resources) {
          this.publishProjectionUpdate(new RealtimeProjectionUpdate({ resource }));
        }
        if (family === 'rooms') {
          await this.hydrateProjectedDMUsers(minimumCursor, generation);
        }
      })
      .catch((error) => {
        if (generation !== this.#realtimeProjectionGeneration) return;
        this.#reconciliationError ??= error;
        console.error(`[server:${this.serverId}] resource refresh failed`, family, error);
      })
      .finally(() => {
        this.#resourceRefreshes.delete(family);
        const pending = this.#pendingResourceRefreshes.get(family);
        if (!pending) return;
        this.#pendingResourceRefreshes.delete(family);
        if (pending.generation !== this.#realtimeProjectionGeneration) return;
        this.refreshRealtimeResource(family, pending.minimumCursor);
      });
    this.#resourceRefreshes.set(family, refresh);
  }

  private async hydrateProjectedDMUsers(
    minimumCursor?: string,
    generation = this.#realtimeProjectionGeneration
  ): Promise<void> {
    this.requireCurrentRealtimeProjection(generation);
    const userIds = [...this.projection.rooms.values()].flatMap((room) => room.memberUserIds);
    const missingIds = userIds.filter((userId) => !this.projection.users.has(userId));
    const resources = await this.#realtimeResources.readUsers(missingIds, minimumCursor);
    this.requireCurrentRealtimeProjection(generation);
    for (const resource of resources) {
      this.publishProjectionUpdate(new RealtimeProjectionUpdate({ resource }));
    }
  }

  private refreshRealtimeUsers(userIds: Iterable<string>, minimumCursor?: string): void {
    for (const userId of userIds) if (userId) this.#pendingUserRefreshIds.add(userId);
    if (this.#pendingUserRefreshIds.size === 0) return;
    const nextCursor = minimumCursor ?? this.#currentEventMinimumCursor;
    const generation = this.#realtimeProjectionGeneration;
    if (this.#pendingUserRefreshGeneration !== generation) {
      this.#pendingUserRefreshCursor = undefined;
    }
    this.#pendingUserRefreshGeneration = generation;
    if (nextCursor || !this.#pendingUserRefreshCursor) {
      this.#pendingUserRefreshCursor = nextCursor;
    }
    if (this.#userRefresh) return;
    let failedGeneration = generation;
    this.#userRefresh = (async () => {
      while (this.#pendingUserRefreshIds.size > 0) {
        const ids = [...this.#pendingUserRefreshIds];
        this.#pendingUserRefreshIds.clear();
        const cursor = this.#pendingUserRefreshCursor;
        const readGeneration = this.#pendingUserRefreshGeneration;
        this.#pendingUserRefreshCursor = undefined;
        failedGeneration = readGeneration;
        const resources = await this.#realtimeResources.readUsers(ids, cursor);
        this.requireCurrentRealtimeProjection(readGeneration);
        for (const resource of resources) {
          this.publishProjectionUpdate(new RealtimeProjectionUpdate({ resource }));
        }
      }
    })()
      .catch((error) => {
        if (failedGeneration !== this.#realtimeProjectionGeneration) return;
        this.#reconciliationError ??= error;
        console.error(`[server:${this.serverId}] user resource refresh failed`, error);
      })
      .finally(() => {
        this.#userRefresh = null;
        if (this.#pendingUserRefreshIds.size > 0) this.refreshRealtimeUsers([]);
      });
  }

  /** Apply a refreshed resource and notify every consumer of the server bus. */
  private publishProjectionUpdate(update: RealtimeProjectionUpdate): void {
    this.ingestProjectionEvent(update);
    const bus = eventBusManager.getBus(this.serverId);
    if (!bus) return;
    for (const handler of bus.projectionHandlers) {
      if (handler !== this.realtimeProjectionHandler) handler(update);
    }
  }

  private invalidateRealtimeEvent(event: RealtimeEvent): void {
    const payload = event.event;
    const rawValue = payload.value as
      { eventId?: string; messageEventId?: string; roomId?: string; userId?: string } | undefined;
    const roomId = rawValue?.roomId ?? '';

    switch (payload.case) {
      case 'userAccountDeleted': {
        const userId = payload.value.userId;
        this.projection.removeUser(userId);
        this.scrubRemovedUser(userId);
        return;
      }
      case 'roomDeleted':
        this.projection.removeRoom(payload.value.roomId);
        this.scrubRemovedRoom(payload.value.roomId);
        this.refreshRealtimeResource('rooms');
        this.refreshRealtimeResource('roomGroups');
        return;
      case 'userLeftRoom':
      case 'roomMemberRemoved':
      case 'roomMemberBanned':
        if (
          (!!rawValue?.userId && rawValue.userId === this.currentUser.user?.id) ||
          (payload.case === 'userLeftRoom' && event.actorId === this.currentUser.user?.id)
        ) {
          if (roomId) this.clearRoomAccess(roomId);
        }
        if (payload.case === 'userLeftRoom') {
          this.refreshLoadedMessageWindows(roomId, event.id || null);
        }
        this.refreshRealtimeResource('rooms');
        this.refreshRealtimeResource('roomGroups');
        return;
      case 'messagePosted':
      case 'messageEdited':
      case 'messageRetracted':
      case 'reactionAdded':
      case 'reactionRemoved':
      case 'assetDeleted': {
        const anchorEventId =
          rawValue?.eventId ??
          rawValue?.messageEventId ??
          (payload.case === 'messagePosted' ? event.id : null);
        const roomAnchorEventId =
          payload.case === 'messagePosted' && payload.value.inThread
            ? payload.value.inThread
            : anchorEventId;
        if (payload.case === 'messagePosted') this.ingestRealtimeMessagePost(event);
        this.refreshLoadedMessageWindows(
          roomId,
          anchorEventId,
          roomAnchorEventId,
          payload.case === 'messagePosted' && !payload.value.inThread,
          payload.case === 'messagePosted' && !!payload.value.inThread,
          payload.case === 'messagePosted'
        );
        if (payload.case === 'messageRetracted') {
          this.applyLoadedMessageRetraction(
            roomId,
            payload.value.eventId,
            event.createdAt?.toDate().toISOString() ?? new SvelteDate().toISOString()
          );
        }
        this.refreshLoadedTimelineResources(roomId, {
          files: payload.case !== 'reactionAdded' && payload.case !== 'reactionRemoved',
          pins: payload.case !== 'messagePosted'
        });
        if (roomId) this.forRoomMessageSearch(roomId, (store) => store.invalidateRoom(roomId));
        else this.forEachMessageSearch((store) => store.clearResults());
        if (payload.case === 'messagePosted') {
          this.refreshRealtimeResource('rooms');
          if (payload.value.inThread) refreshRegisteredFollowedThreadQueries(this.serverId);
        }
        if (payload.case === 'messageRetracted') {
          refreshRegisteredFollowedThreadQueries(this.serverId);
        }
        return;
      }
      case 'assetProcessingStarted':
      case 'assetProcessingSucceeded':
      case 'assetProcessingFailed':
        this.refreshLoadedMessageWindows('', rawValue?.messageEventId ?? null);
        this.refreshLoadedTimelineResources('', { files: true, pins: true });
        return;
      case 'voiceCallParticipantJoined':
        this.playCallTransitionSound(
          event.id,
          'join',
          payload.value.roomId,
          payload.value.callId || null,
          event.actorId || null
        );
        this.refreshRealtimeResource('activeCalls');
        return;
      case 'voiceCallParticipantLeft':
        this.playCallTransitionSound(
          event.id,
          'leave',
          payload.value.roomId,
          payload.value.callId || null,
          event.actorId || null
        );
        this.voiceCall.handleParticipantLeftEvent(
          payload.value.roomId,
          payload.value.callId || null,
          event.actorId || null,
          this.currentUserId()
        );
        this.refreshRealtimeResource('activeCalls');
        return;
      case 'voiceCallEnded':
        this.voiceCall.handleCallEndedEvent(payload.value.roomId, payload.value.callId || null);
        this.refreshLoadedMessageWindows(
          payload.value.roomId,
          event.id || null,
          event.id || null,
          true
        );
        this.refreshRealtimeResource('activeCalls');
        return;
      case 'voiceCallStarted':
        this.refreshLoadedMessageWindows(
          payload.value.roomId,
          event.id || null,
          event.id || null,
          true
        );
        this.refreshRealtimeResource('activeCalls');
        return;
      case 'notificationOccurrencesInvalidated':
        this.refreshRealtimeResource('notifications');
        return;
      case 'notificationUnreadChanged':
        this.refreshRealtimeResource('notifications');
        this.refreshRealtimeResource('rooms');
        return;
      case 'roomCreated':
      case 'roomUpdated':
      case 'roomArchived':
      case 'roomUnarchived':
      case 'roomUniversalChanged':
      case 'roomSlowModeChanged':
      case 'roomThreadingModeChanged':
      case 'userJoinedRoom':
      case 'roomMemberAdded':
      case 'roomMemberUnbanned':
        if (payload.case === 'userJoinedRoom') {
          this.refreshLoadedMessageWindows(roomId, event.id || null);
        }
        if (payload.case === 'roomThreadingModeChanged') {
          this.refreshLoadedMessageWindows(roomId, event.id || null);
        }
        this.refreshRealtimeResource('rooms');
        this.refreshRealtimeResource('roomGroups');
        return;
      case 'roomGroupCreated':
      case 'roomGroupUpdated':
      case 'roomGroupDeleted':
      case 'roomAddedToGroup':
      case 'roomRemovedFromGroup':
      case 'roomsInGroupReordered':
      case 'sidebarLinkAddedToGroup':
      case 'sidebarLinkUpdated':
      case 'sidebarLinkRemovedFromGroup':
      case 'sidebarGroupEntriesReordered':
      case 'roomGroupsReordered':
        this.refreshRealtimeResource('roomGroups');
        return;
      case 'serverProfileChanged':
        this.refreshRealtimeResource('server');
        return;
      case 'serverMotdChanged':
        this.refreshRealtimeResource('serverState');
        return;
      case 'userProfileChanged':
      case 'userAccountCreated':
      case 'userLoginChanged':
      case 'userDisplayNameChanged':
      case 'userAvatarSet':
      case 'userAvatarCleared':
      case 'userCustomStatusSet':
      case 'userCustomStatusCleared':
      case 'userBioChanged':
        if (rawValue?.userId) this.refreshRealtimeUsers([rawValue.userId]);
        return;
      case 'viewerPreferencesChanged':
        this.refreshRealtimeResource('viewer');
        this.refreshRealtimeResource('rooms');
        return;
      case 'roomReadStateChanged':
        this.refreshRealtimeResource('rooms');
        return;
      case 'threadViewerStateChanged': {
        this.applyThreadFollowChange(
          roomId,
          payload.value.threadRootEventId,
          payload.value.isFollowing
        );
        refreshRegisteredFollowedThreadQueries(this.serverId);
        return;
      }
      default:
        return;
    }
  }

  /** Render an authorized public post while its resource hydration runs. */
  private ingestRealtimeMessagePost(event: RealtimeEvent): void {
    const posted = event.event.case === 'messagePosted' ? event.event.value : null;
    if (!posted || posted.bodyPlaintext === undefined || !event.id) return;
    const actorMember = event.actorId ? this.projection.users.get(event.actorId) : null;
    const timelineEvent: TimelineEventView = {
      id: event.id,
      createdAt: event.createdAt?.toDate().toISOString() ?? new SvelteDate().toISOString(),
      actorId: event.actorId || null,
      actor: actorMember ? avatarUserFromDirectoryMember(mapDirectoryMember(actorMember)) : null,
      event: {
        kind: TimelineEventKind.MessagePosted,
        roomId: posted.roomId,
        body: posted.bodyPlaintext,
        attachments: [],
        linkPreview: null,
        reactions: [],
        updatedAt: null,
        inReplyTo: posted.inReplyTo || null,
        threadRootEventId: posted.inThread || null,
        echoOfEventId: posted.echoOfEventId || null,
        echoFromThreadRootEventId: posted.echoFromThreadRootEventId || null,
        channelEchoEventId: null,
        deletedAt: null,
        pinned: false,
        threadExists: false,
        replyCount: 0,
        lastReplyAt: null,
        threadParticipantCount: 0,
        threadParticipants: [],
        viewerIsFollowingThread: null,
        viewerHasUnreadThread: null
      }
    };
    for (const [roomId, store] of Object.entries(this.#roomMessages)) {
      if (roomId === posted.roomId) store.ingestEvent(timelineEvent);
    }
    for (const [key, store] of Object.entries(this.#threadMessages)) {
      if (key.startsWith(`${posted.roomId}\u0000`)) store.ingestEvent(timelineEvent);
    }
  }

  private refreshLoadedMessageWindows(
    roomId: string,
    anchorEventId: string | null,
    roomAnchorEventId: string | null = anchorEventId,
    roomForward = false,
    threadForward = false,
    hydratePostedMessage = false,
    minimumCursor = this.#currentEventMinimumCursor
  ): void {
    for (const [candidateRoomId, store] of Object.entries(this.#roomMessages)) {
      if (roomId && candidateRoomId !== roomId) continue;
      const visibleAnchor = roomAnchorEventId
        ? (store.refreshAnchorForMessageMutation(roomAnchorEventId) ?? roomAnchorEventId)
        : null;
      if (hydratePostedMessage && roomForward) {
        this.schedulePostedMessageRefresh(store, visibleAnchor, minimumCursor);
      } else {
        this.scheduleMessageWindowRefresh(store, visibleAnchor, roomForward, minimumCursor);
      }
    }
    for (const [key, store] of Object.entries(this.#threadMessages)) {
      if (roomId && !key.startsWith(`${roomId}\u0000`)) continue;
      const visibleAnchor = anchorEventId
        ? (store.refreshAnchorForMessageMutation(anchorEventId) ?? anchorEventId)
        : null;
      if (hydratePostedMessage && threadForward) {
        this.schedulePostedMessageRefresh(store, visibleAnchor, minimumCursor);
      } else {
        this.scheduleMessageWindowRefresh(store, visibleAnchor, threadForward, minimumCursor);
      }
    }
  }

  /** Hydrate a new post first, then advance its retained timeline window. */
  private schedulePostedMessageRefresh(
    store: MessagesStore,
    anchorEventId: string | null,
    minimumCursor?: string
  ): void {
    if (!anchorEventId) {
      this.scheduleMessageWindowRefresh(store, anchorEventId, true, minimumCursor);
      return;
    }
    const generation = this.#realtimeProjectionGeneration;
    const refresh = store
      .refreshPostedMessage(
        anchorEventId,
        minimumCursor,
        () => generation === this.#realtimeProjectionGeneration
      )
      .then((timelineIsCurrent) => {
        if (timelineIsCurrent && generation === this.#realtimeProjectionGeneration) {
          this.scheduleMessageWindowRefresh(store, anchorEventId, true, minimumCursor);
        }
      });
    this.trackProjectionReconciliation(refresh, minimumCursor, generation);
  }

  private applyLoadedMessageRetraction(
    roomId: string,
    messageEventId: string,
    retractedAt: string
  ): void {
    if (!messageEventId) return;
    for (const [candidateRoomId, store] of Object.entries(this.#roomMessages)) {
      if (roomId && candidateRoomId !== roomId) continue;
      store.applyMessageRetraction(messageEventId, retractedAt);
    }
    for (const [key, store] of Object.entries(this.#threadMessages)) {
      if (roomId && !key.startsWith(`${roomId}\u0000`)) continue;
      store.applyMessageRetraction(messageEventId, retractedAt);
    }
  }

  /** Coalesce invalidations and run one follow-up read after an active read. */
  private scheduleMessageWindowRefresh(
    store: MessagesStore,
    anchorEventId: string | null,
    forward = false,
    minimumCursor?: string
  ): void {
    const generation = this.#realtimeProjectionGeneration;
    if (this.#messageWindowRefreshes.has(store)) {
      const pending = this.#pendingMessageWindowRefreshes.get(store);
      this.#pendingMessageWindowRefreshes.set(store, {
        anchorEventId,
        forward,
        minimumCursor:
          minimumCursor ?? (pending?.generation === generation ? pending.minimumCursor : undefined),
        generation
      });
      return;
    }
    const refresh = store
      .refreshCurrentWindow(
        anchorEventId,
        forward,
        minimumCursor,
        () => generation === this.#realtimeProjectionGeneration
      )
      .then(() => undefined)
      .catch((error) => {
        if (generation !== this.#realtimeProjectionGeneration) return;
        this.#reconciliationError ??= error;
      })
      .finally(() => {
        this.#messageWindowRefreshes.delete(store);
        const pending = this.#pendingMessageWindowRefreshes.get(store);
        if (!pending) return;
        this.#pendingMessageWindowRefreshes.delete(store);
        if (pending.generation !== this.#realtimeProjectionGeneration) return;
        this.scheduleMessageWindowRefresh(
          store,
          pending.anchorEventId,
          pending.forward,
          pending.minimumCursor
        );
      });
    this.#messageWindowRefreshes.set(store, refresh);
    this.trackProjectionReconciliation(refresh, minimumCursor, generation);
  }

  /** Keep the durable cursor behind every ConnectRPC read caused by its event. */
  private trackProjectionReconciliation(
    refresh: Promise<unknown>,
    minimumCursor: string | undefined,
    generation: number
  ): void {
    if (!minimumCursor) return;
    const tracked = refresh
      .then(() => undefined)
      .catch((error) => {
        if (generation !== this.#realtimeProjectionGeneration) return;
        this.#reconciliationError ??= error;
      })
      .finally(() => {
        this.#projectionReconciliations.delete(tracked);
      });
    this.#projectionReconciliations.add(tracked);
  }

  /** Apply user-scoped follow state without restarting an active thread read. */
  private applyThreadFollowChange(
    roomId: string,
    threadRootEventId: string,
    isFollowing: boolean
  ): void {
    if (!roomId || !threadRootEventId) return;
    const roomStore = this.#roomMessages[roomId];
    roomStore?.setThreadRootFollowState(threadRootEventId, isFollowing);
    if (roomStore) this.scheduleMessageWindowRefresh(roomStore, threadRootEventId);

    const threadStore = this.#threadMessages[`${roomId}\u0000${threadRootEventId}`];
    threadStore?.setThreadRootFollowState(threadRootEventId, isFollowing);
  }

  private refreshLoadedTimelineResources(
    roomId: string,
    resources: { files: boolean; pins: boolean }
  ): void {
    if (resources.files) {
      for (const [candidateRoomId, store] of Object.entries(this.#roomFiles)) {
        if (roomId && candidateRoomId !== roomId) continue;
        store.refreshRetained();
      }
    }
    if (resources.pins) {
      for (const [candidateRoomId, store] of Object.entries(this.#roomPins)) {
        if (roomId && candidateRoomId !== roomId) continue;
        store.retry();
      }
    }
  }

  get #adminRoomLayoutActive(): boolean {
    return this.#adminRoomLayoutSubscriptions > 0;
  }

  private forEachMessageSearch(callback: (store: MessageSearchStore) => void): void {
    callback(this.messageSearch);
    for (const store of Object.values(this.#roomMessageSearch)) callback(store);
  }

  private forRoomMessageSearch(
    roomId: string,
    callback: (store: MessageSearchStore) => void
  ): void {
    callback(this.messageSearch);
    const roomStore = this.#roomMessageSearch[roomId];
    if (roomStore) callback(roomStore);
  }

  #touchRoomMessageSearch(roomId: string): void {
    const currentIndex = this.#roomMessageSearchRecency.indexOf(roomId);
    if (currentIndex >= 0) this.#roomMessageSearchRecency.splice(currentIndex, 1);
    this.#roomMessageSearchRecency.push(roomId);
  }

  private scheduleAdminRoomLayoutRefresh(): void {
    if (!this.#adminRoomLayoutActive) return;
    this.adminRoomLayout.requestProjectionRefresh();
  }

  /** Keep the admin layout editor current while its route is mounted. */
  activateAdminRoomLayout(): () => void {
    this.#adminRoomLayoutSubscriptions += 1;
    if (this.#adminRoomLayoutSubscriptions === 1) void this.adminRoomLayout.refresh();
    return () => {
      this.#adminRoomLayoutSubscriptions = Math.max(0, this.#adminRoomLayoutSubscriptions - 1);
      if (!this.#adminRoomLayoutActive) this.adminRoomLayout.deactivateProjectionRefresh();
    };
  }

  /** Clear every mirror whose authority was invalidated by a reset frame. */
  private resetProjectionMirrors(): void {
    refreshRegisteredAdminQueries(this.serverId);
    clearUserSummaryCache(this.serverId);
    for (const store of Object.values(this.#roomMessages)) store.resetProjectionState();
    for (const store of Object.values(this.#threadMessages)) store.resetProjectionState();
    for (const store of Object.values(this.#roomFiles)) {
      store.reset({ rehydrateRetained: true });
    }
    for (const store of Object.values(this.#roomPins)) {
      store.reset({ rehydrateRetained: true });
    }
    this.roomDirectory.resetOptimisticState();
    this.notifications.resetProjectionState();
    this.roomUnread.clear();
    this.pendingHighlights.clear();
    this.activeCallRooms.clear();
    this.serverInfo.resetProjectionState();
    this.#playedCallSoundEventIds.length = 0;
  }

  /** Complete current room membership resolved through the warm user cache. */
  projectedMembersForRoom(roomId: string): RoomMember[] {
    const room = this.projection.rooms.get(roomId);
    if (!room) return [];
    return room.memberUserIds.flatMap((userId) => {
      const user = this.projection.users.get(userId);
      return user ? [avatarUserFromDirectoryMember(mapDirectoryMember(user))] : [];
    });
  }

  /** Whether membership references are authoritative for this projected room. */
  hasCompleteProjectedRoomMembership(roomId: string): boolean {
    const room = this.projection.rooms.get(roomId);
    return room ? mapDirectoryRoom(room)?.kind === RoomKind.DM : false;
  }

  /**
   * Whether this server uses cookie auth (origin) vs bearer auth (remote).
   * Read from the live registered server so it stays correct if the token
   * field is ever updated.
   */
  get #cookieAuth(): boolean {
    return this.#originServer && this.#getSession().token === null;
  }

  /**
   * Whether this server currently has an authenticated user.
   * - Cookie auth (origin): true when `currentUser.user` is set.
   * - Bearer auth (remote): true when an access token is registered.
   */
  get isAuthenticated(): boolean {
    if (this.#getSession().reauthRequiredAt !== null) return false;
    if (this.#cookieAuth) {
      return this.currentUser.user != null;
    }
    return this.#getSession().token != null;
  }

  /** Update permissions from viewer query data. */
  setPermissions(viewer: ViewerData): void {
    const previous = this.permissions;
    this.permissions = { ...viewer, loaded: true };
    const lostAdminCapability =
      previous.loaded &&
      ((previous.canViewAdmin && !viewer.canViewAdmin) ||
        (previous.canAdminViewUsers && !viewer.canAdminViewUsers) ||
        (previous.canAdminManageAccounts && !viewer.canAdminManageAccounts) ||
        (previous.canAssignRoles && !viewer.canAssignRoles) ||
        (previous.canAdminViewRoles && !viewer.canAdminViewRoles) ||
        (previous.canAdminManageRoles && !viewer.canAdminManageRoles) ||
        (previous.canAdminViewSystem && !viewer.canAdminViewSystem) ||
        (previous.canAdminViewAudit && !viewer.canAdminViewAudit) ||
        (previous.canManageInvites && !viewer.canManageInvites));
    if (lostAdminCapability) {
      removeRegisteredAdminQueries(this.serverId);
    }
  }

  /**
   * Single source of truth for the server-level indicator dot.
   * Notifications take precedence over plain unread.
   *
   * DMs are surfaced as rooms on the Server in the merged sidebar, so the
   * user expects the server icon to light up the same way it would for a
   * channel mention or unread.
   */
  serverIndicator(): ServerIndicator {
    // Channel + DM activity both roll up to the single server indicator.
    if (this.notifications.unreadNotificationCount > 0) return 'notification';
    if (this.notifications.hasNonDMNotifications()) return 'notification';
    if (this.notifications.hasDMNotifications()) return 'notification';
    if (this.roomUnread.hasAnyUnread) return 'unread';
    return null;
  }

  /**
   * Indicator for the DM area only. Kept for consumers that want a DM-only
   * answer instead of the combined server indicator.
   */
  dmIndicator(): ServerIndicator {
    if (this.notifications.hasDMNotifications()) return 'notification';
    // We no longer track DM unread separately — `hasAnyUnread` covers it.
    return null;
  }

  private playCallTransitionSound(
    eventId: string,
    kind: 'join' | 'leave',
    roomId: string,
    callId: string | null,
    actorId: string | null
  ): void {
    if (this.#playedCallSoundEventIds.includes(eventId)) return;

    const currentUserId = this.currentUserId();
    if (!actorId || !currentUserId) return;

    const decision = this.voiceCall.callTransitionSoundDecision(
      kind,
      roomId,
      callId,
      actorId === currentUserId
    );
    if (decision === 'skip') return;

    this.rememberPlayedCallSoundEvent(eventId);
    if (decision === 'defer') return;

    void playCallSound(kind);
  }

  private rememberPlayedCallSoundEvent(eventId: string): void {
    this.#playedCallSoundEventIds.push(eventId);
    if (this.#playedCallSoundEventIds.length > 500) {
      this.#playedCallSoundEventIds.shift();
    }
  }

  private currentUserId(): string | null {
    return this.navigation.currentUserId ?? this.currentUser.user?.id ?? this.#getSession().userId;
  }

  /** Remove optimistic call UI state after a local join attempt fails. */
  handleVoiceCallJoinFailed(roomId: string): void {
    const currentUserId = this.navigation.currentUserId;
    this.activeCallRooms.handleLeave(roomId, null, currentUserId);
  }

  /** Clean up resources. */
  dispose(): void {
    removeRegisteredServerQueries(this.serverId);
    this.#disposeEffects();
    this.adminRoomLayout.deactivateProjectionRefresh();
    this.#adminRoomLayoutSubscriptions = 0;
    this.realtimeSync.reset();
    for (const store of Object.values(this.#roomMessages)) store.dispose();
    this.#roomMessages = Object.create(null);
    for (const store of Object.values(this.#roomFiles)) store.dispose();
    this.#roomFiles = Object.create(null);
    for (const store of Object.values(this.#roomPins)) store.dispose();
    this.#roomPins = Object.create(null);
    for (const store of Object.values(this.#roomMessageSearch)) store.reset();
    this.#roomMessageSearch = Object.create(null);
    this.#roomMessageSearchRecency = [];
    for (const store of Object.values(this.#threadMessages)) store.dispose();
    this.#threadMessages = Object.create(null);
    this.#threadMessageRefCounts = Object.create(null);
    this.roomUnread.clear();
    this.pendingHighlights.clear();
    this.activeCallRooms.clear();
    this.messageSearch.reset();
  }
}
