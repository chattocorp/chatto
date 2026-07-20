/**
 * Bundles all server-scoped stores into a single class per server.
 * Created and managed by the ServerRegistry — do not instantiate directly.
 */

import { CurrentUserState } from '$lib/auth/currentUser.svelte';
import { ServerInfoState } from './state.svelte';
import type { PublicServerInfo } from '$lib/api-client/server';
import type { ServerPermissions, ViewerData } from './permissions.svelte';
import { NotificationStore } from './notifications.svelte';
import { RoomUnreadStore } from './roomUnread.svelte';
import { NotificationLevelStore } from './notificationLevel.svelte';
import { PendingHighlightStore } from './pendingHighlight.svelte';
import { VoiceCallState } from './voiceCall.svelte';
import { ActiveCallRoomsState } from './activeCallRooms.svelte';
import { RoomsStore } from './rooms.svelte';
import { RoomDirectoryStore } from './roomDirectory.svelte';
import { AdminRoomLayoutStore } from './adminRoomLayout.svelte';
import { AdminEventLogStore } from './adminEventLog.svelte';
import { createRoomCommandAPI } from '$lib/api-client/rooms';
import { createNotificationAPI } from '$lib/api-client/notifications';
import { createVoiceCallAPI } from '$lib/api-client/voiceCalls';
import { createRoomDirectoryAPI } from '$lib/api-client/roomDirectory';
import { createAdminRoomLayoutAPI } from '$lib/api-client/adminRoomLayout';
import { createAdminEventLogAPI } from '$lib/api-client/adminEventLog';
import { createMessageSearchAPI } from '$lib/api-client/messageSearch';
import { createMemberDirectoryAPI } from '$lib/api-client/memberDirectory';
import { getViewerStateViaConnect } from '$lib/api-client/viewer';
import { eventBusManager } from './eventBus.svelte';
import type { ProjectionHandler } from '$lib/eventBus.svelte';
import type { ServerConnection } from './serverConnection.svelte';
import type { RegisteredServer } from './registry.svelte';
import { playCallSound } from '$lib/audio/callSounds';
import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { ServerProjectionStore } from './projection.svelte';
import { MessagesStore, RoomFilesStore } from '$lib/state/room';
import type { RoomMember } from '$lib/state/room';
import type { RealtimeProjectionEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import { mapDirectoryRoom, mapRoomGroup, RoomKind } from '$lib/api-client/roomDirectory';
import { mapDirectoryMember } from '$lib/api-client/memberDirectory';
import { viewerResponseToState, type ViewerState } from '$lib/api-client/viewer';
import { notifyUserSummaries } from '$lib/api-client/hooks';
import {
  clearUserSummaryCache,
  removeUserSummaryCacheEntry
} from '$lib/state/userSummaries.svelte';
import { avatarUserFromDirectoryMember } from './rooms.svelte';
import { mapNotificationPage } from '$lib/api-client/notifications';
import { RealtimeProjectionSyncState } from './realtimeSync.svelte';
import type { ActiveCall } from '@chatto/api-types/api/v1/voice_calls_pb';
import { MessageSearchStore } from './messageSearch.svelte';

/**
 * What kind of indicator a server (or the DM area) should display.
 * - 'notification' = warning badge, has a pending mention/reply/room-message
 * - 'unread' = grey dot, has unread rooms but no pending notification
 * - null = no indicator
 */
export type ServerIndicator = 'notification' | 'unread' | null;

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
  canAdminViewAudit: false
};

export class ServerStateStore {
  readonly serverId: string;
  readonly currentUser: CurrentUserState;
  readonly serverInfo: ServerInfoState;
  readonly notifications: NotificationStore;
  readonly roomUnread: RoomUnreadStore;
  readonly notificationLevels: NotificationLevelStore;
  readonly pendingHighlights: PendingHighlightStore;
  readonly voiceCall: VoiceCallState;
  readonly activeCallRooms: ActiveCallRoomsState;
  readonly rooms: RoomsStore;
  readonly roomDirectory: RoomDirectoryStore;
  readonly adminRoomLayout: AdminRoomLayoutStore;
  readonly adminEventLog: AdminEventLogStore;
  readonly messageSearch: MessageSearchStore;
  readonly projection = new ServerProjectionStore();
  /** Readiness and opaque resume position for this retained projection. */
  readonly realtimeSync = new RealtimeProjectionSyncState();

  /** Per-server viewer permissions (loaded by ServerSidebarEntry). */
  permissions = $state<ServerPermissions>(EMPTY_PERMISSIONS);

  /**
   * Live reference to the registered server. Reads pick up `updateServer`
   * mutations (e.g. token refresh, name change) because the registry stores
   * servers in $state.
   */
  readonly #registered: RegisteredServer;
  readonly #serverConnection: ServerConnection;
  // These registries are intentionally non-reactive. The stores they own are
  // reactive, while selector calls may occur during derived evaluation.
  #roomMessages: Record<string, MessagesStore> = Object.create(null);
  #roomFiles: Record<string, RoomFilesStore> = Object.create(null);
  #threadMessages: Record<string, MessagesStore> = Object.create(null);
  #threadMessageRefCounts: Record<string, number> = Object.create(null);
  #adminRoomLayoutSubscriptions = 0;

  /** Disposer for the internal effect root that wires lifecycle reactivity. */
  readonly #disposeEffects: () => void;
  readonly #playedCallSoundEventIds: string[] = [];

  constructor(
    registered: RegisteredServer,
    serverConnection: ServerConnection,
    publicServerInfoLoader?: (baseUrl: string) => Promise<PublicServerInfo>,
    onAuthenticationRequired?: () => void
  ) {
    this.serverId = registered.id;
    this.#registered = registered;
    this.#serverConnection = serverConnection;
    const cookieAuth = this.#cookieAuth;

    const connectAPIConfig = {
      serverId: serverConnection.serverId ?? registered.id,
      baseUrl: serverConnection.connectBaseUrl,
      bearerToken: serverConnection.bearerToken
    };
    const notificationAPI = createNotificationAPI(connectAPIConfig);
    const voiceCallAPI = createVoiceCallAPI(connectAPIConfig);
    const roomDirectoryAPI = createRoomDirectoryAPI(connectAPIConfig);
    const adminRoomLayoutAPI = createAdminRoomLayoutAPI(connectAPIConfig);
    const adminEventLogAPI = createAdminEventLogAPI(connectAPIConfig);
    const messageSearchAPI = createMessageSearchAPI(connectAPIConfig);
    const memberDirectoryAPI = createMemberDirectoryAPI(connectAPIConfig);
    this.currentUser = new CurrentUserState(
      cookieAuth,
      connectAPIConfig,
      undefined,
      onAuthenticationRequired
    );
    this.serverInfo = new ServerInfoState(registered.url, publicServerInfoLoader);
    this.notifications = new NotificationStore(notificationAPI);
    this.roomUnread = new RoomUnreadStore();
    this.notificationLevels = new NotificationLevelStore();
    const roomCommandAPI = createRoomCommandAPI({
      serverId: serverConnection.serverId ?? registered.id,
      baseUrl: serverConnection.connectBaseUrl,
      bearerToken: serverConnection.bearerToken
    });
    this.pendingHighlights = new PendingHighlightStore();
    this.voiceCall = new VoiceCallState(voiceCallAPI);
    this.activeCallRooms = new ActiveCallRoomsState(this.voiceCall);
    this.rooms = new RoomsStore(
      roomDirectoryAPI,
      memberDirectoryAPI,
      () => getViewerStateViaConnect(connectAPIConfig),
      this.notificationLevels,
      this.roomUnread,
      notificationAPI
    );
    this.roomDirectory = new RoomDirectoryStore(
      roomDirectoryAPI,
      memberDirectoryAPI,
      roomCommandAPI
    );
    this.adminRoomLayout = new AdminRoomLayoutStore(adminRoomLayoutAPI, roomCommandAPI);
    this.adminEventLog = new AdminEventLogStore(adminEventLogAPI);
    this.messageSearch = new MessageSearchStore(messageSearchAPI);

    // Apply the canonical projection delivered by this server's bus. Transient
    // envelopes are consumed only by components that need one-shot signals.
    this.#disposeEffects = $effect.root(() => {
      $effect(() => {
        const bus = eventBusManager.getBus(this.serverId);
        if (!bus) return;
        const projectionHandler: ProjectionHandler = (event) => this.ingestProjectionEvent(event);
        bus.projectionHandlers.add(projectionHandler);
        return () => {
          bus.projectionHandlers.delete(projectionHandler);
        };
      });
    });
  }

  /** Stable room timeline owner used by routes as a rendering selector. */
  messagesForRoom(roomId: string): MessagesStore {
    let store = this.#roomMessages[roomId];
    if (store) return store;
    store = new MessagesStore(this.#serverConnection, () => this.currentUser.user?.id ?? null);
    store.awaitRoomProjection(roomId);
    this.#roomMessages[roomId] = store;
    const page = this.projection.timelines.get(roomId);
    if (page) store.replaceRoomProjectionPage(roomId, page);
    return store;
  }

  /** Stable lazy file-list owner for one room on this server. */
  filesForRoom(roomId: string): RoomFilesStore {
    let store = this.#roomFiles[roomId];
    if (store) return store;
    store = new RoomFilesStore(this.#serverConnection, roomId);
    this.#roomFiles[roomId] = store;
    return store;
  }

  /** Restore the canonical latest window when a route selects this room. */
  restoreProjectedRoomWindow(roomId: string): void {
    const evictedRoomId = this.realtimeSync.retainRoom(roomId);
    if (evictedRoomId) this.evictRetainedRoom(evictedRoomId);
    const messages = this.messagesForRoom(roomId);
    // Route entry and cleanup both supersede an in-flight historical jump,
    // even when this room's first projection page has not arrived yet.
    const page = this.projection.timelines.get(roomId);
    if (page) messages.restoreRoomProjectionPage(roomId, page);
    else {
      messages.cancelPendingHistoricalJump();
      eventBusManager.hydrateRoom(this.serverId, roomId);
    }
  }

  private evictRetainedRoom(roomId: string): void {
    const room = this.projection.rooms.get(roomId)?.room;
    const clearMembership = room ? mapDirectoryRoom(room)?.kind !== RoomKind.DM : false;
    this.projection.evictRoomTimeline(roomId, clearMembership);
    const viewer = this.projection.viewer;
    if (viewer) this.synchronizeProjectedNavigation(viewerResponseToState(viewer));
    this.#roomMessages[roomId]?.dispose();
    delete this.#roomMessages[roomId];
    for (const [key, threadStore] of Object.entries(this.#threadMessages)) {
      if (!key.startsWith(`${roomId}\u0000`)) continue;
      threadStore.dispose();
      delete this.#threadMessages[key];
      delete this.#threadMessageRefCounts[key];
    }
  }

  /** Scrub every plaintext timeline mirror for a room at an authorization boundary. */
  private clearRoomAccess(roomId: string, forgetStores = false): void {
    this.voiceCall.handleRoomAccessRevoked(roomId);
    this.activeCallRooms.clearRoom(roomId);
    this.notifications.clearRoom(roomId);
    const roomStore = this.#roomMessages[roomId];
    roomStore?.clearForAccessRevocation();
    const filesStore = this.#roomFiles[roomId];
    filesStore?.reset();
    if (forgetStores) {
      roomStore?.dispose();
      delete this.#roomMessages[roomId];
      filesStore?.dispose();
      delete this.#roomFiles[roomId];
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
    this.#roomMessages[roomId]?.restoreAfterAccessGrant();
    this.#roomFiles[roomId]?.restoreAfterAccessGrant();
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

  private ingestProjectionEvent(event: RealtimeProjectionEvent): void {
    const existingTimelineRows = new SvelteSet<string>();
    for (const operation of event.operations) {
      if (operation.operation.case !== 'roomTimelineEventUpsert') continue;
      const update = operation.operation.value;
      if (
        update.event &&
        this.projection.timelines
          .get(update.roomId)
          ?.events.some((candidate) => candidate.id === update.event?.id)
      ) {
        existingTimelineRows.add(`${update.roomId}\u0000${update.event.id}`);
      }
    }
    this.projection.apply(event);
    let adminRoomLayoutChanged = false;
    for (const operation of event.operations) {
      switch (operation.operation.case) {
        case 'reset':
          this.resetProjectionMirrors();
          this.messageSearch.clearResults();
          adminRoomLayoutChanged = true;
          break;
        case 'serverUpsert':
          this.serverInfo.applyProjectionProfile(operation.operation.value);
          break;
        case 'serverStateUpsert':
          this.serverInfo.applyProjectionState(operation.operation.value);
          this.messageSearch.refreshRetainedResults();
          break;
        case 'viewerUpsert': {
          const viewer = viewerResponseToState(operation.operation.value);
          this.currentUser.user = viewer.user;
          this.currentUser.loading = false;
          this.setPermissions(viewer);
          this.synchronizeProjectedNavigation(viewer);
          break;
        }
        case 'userUpsert': {
          const member = mapDirectoryMember(operation.operation.value);
          if (!member.id) break;
          notifyUserSummaries(this.serverId, [member]);
          const viewerResponse = this.projection.viewer;
          if (viewerResponse)
            this.synchronizeProjectedNavigation(viewerResponseToState(viewerResponse));
          break;
        }
        case 'userRemove': {
          const userId = operation.operation.value.userId;
          this.messageSearch.invalidateAuthor(userId);
          removeUserSummaryCacheEntry(this.serverId, userId);
          this.notifications.scrubUser(userId);
          this.activeCallRooms.scrubUser(userId);
          for (const store of Object.values(this.#roomMessages)) {
            store.scrubUserReferences(userId);
          }
          for (const store of Object.values(this.#threadMessages)) {
            store.scrubUserReferences(userId);
          }
          const viewerResponse = this.projection.viewer;
          if (viewerResponse) {
            this.synchronizeProjectedNavigation(viewerResponseToState(viewerResponse));
          }
          break;
        }
        case 'roomUpsert': {
          adminRoomLayoutChanged = true;
          const viewerResponse = this.projection.viewer;
          if (viewerResponse)
            this.synchronizeProjectedNavigation(viewerResponseToState(viewerResponse));
          const roomId = operation.operation.value.room?.room?.id;
          if (!roomId) break;
          if (operation.operation.value.room?.viewerState?.isMember === false) {
            this.messageSearch.revokeRoom(roomId);
            this.clearRoomAccess(roomId);
          } else if (operation.operation.value.room?.viewerState?.isMember === true) {
            this.restoreRoomAccess(roomId);
          }
          break;
        }
        case 'roomRemove': {
          adminRoomLayoutChanged = true;
          const roomId = operation.operation.value.roomId;
          this.messageSearch.revokeRoom(roomId);
          this.clearRoomAccess(roomId, true);
          const viewerResponse = this.projection.viewer;
          if (viewerResponse)
            this.synchronizeProjectedNavigation(viewerResponseToState(viewerResponse));
          break;
        }
        case 'roomGroupsReplace': {
          adminRoomLayoutChanged = true;
          const viewerResponse = this.projection.viewer;
          if (viewerResponse)
            this.synchronizeProjectedNavigation(viewerResponseToState(viewerResponse));
          break;
        }
        case 'roomTimelineReplace': {
          const replacement = operation.operation.value;
          this.messageSearch.invalidateRoom(replacement.roomId);
          if (replacement.page) {
            this.#roomMessages[replacement.roomId]?.replaceRoomProjectionPage(
              replacement.roomId,
              replacement.page
            );
          }
          break;
        }
        case 'roomTimelineEventUpsert': {
          const update = operation.operation.value;
          if (update.event && !update.reactionChange) {
            this.messageSearch.invalidateMessage(
              update.roomId,
              update.event.id,
              existingTimelineRows.has(`${update.roomId}\u0000${update.event.id}`)
            );
          }
          if (update.event) {
            const retainedByProjection = Boolean(
              this.projection.timelines
                .get(update.roomId)
                ?.events.some((candidate) => candidate.id === update.event?.id)
            );
            this.#roomMessages[update.roomId]?.upsertRoomProjectionEvent(
              update.roomId,
              update.event,
              update.includes,
              update.retainDeletedRow,
              retainedByProjection
            );
            if (!update.reactionChange) {
              this.#roomFiles[update.roomId]?.applyTimelineEvent(update.event, event.id);
            }
            for (const [key, threadStore] of Object.entries(this.#threadMessages)) {
              if (!key.startsWith(`${update.roomId}\u0000`)) continue;
              threadStore.upsertRoomProjectionEvent(
                update.roomId,
                update.event,
                update.includes,
                update.retainDeletedRow
              );
            }
          }
          break;
        }
        case 'notificationsReplace': {
          const replacement = operation.operation.value;
          if (replacement.page) {
            this.notifications.replaceProjection(mapNotificationPage(replacement.page));
          }
          const viewerResponse = this.projection.viewer;
          if (viewerResponse) {
            this.synchronizeProjectedNavigation(viewerResponseToState(viewerResponse));
          }
          break;
        }
        case 'roomViewerStateReplace': {
          const replacement = operation.operation.value;
          if (replacement.viewerState?.isMember === false) {
            this.messageSearch.revokeRoom(replacement.roomId);
            this.clearRoomAccess(replacement.roomId);
          } else if (replacement.viewerState?.isMember === true) {
            this.restoreRoomAccess(replacement.roomId);
          }
          const viewerResponse = this.projection.viewer;
          if (viewerResponse) {
            this.synchronizeProjectedNavigation(viewerResponseToState(viewerResponse));
          }
          break;
        }
        case 'activeCallsReplace': {
          const calls = operation.operation.value.calls;
          this.reconcileActiveCallTransition(event, calls);
          this.activeCallRooms.replaceProjection(calls);
          break;
        }
        case 'presencesReplace': {
          const viewerResponse = this.projection.viewer;
          if (viewerResponse) {
            this.synchronizeProjectedNavigation(viewerResponseToState(viewerResponse));
          }
          break;
        }
        case 'threadViewerStatesReplace': {
          for (const [roomId, page] of this.projection.timelines) {
            for (const projectedEvent of page.events) {
              if (
                projectedEvent.event.case !== 'messagePosted' ||
                !projectedEvent.event.value.message?.thread
              ) {
                continue;
              }
              this.#roomMessages[roomId]?.upsertRoomProjectionEvent(
                roomId,
                projectedEvent,
                page.includes
              );
              for (const [key, threadStore] of Object.entries(this.#threadMessages)) {
                if (!key.startsWith(`${roomId}\u0000`)) continue;
                threadStore.upsertRoomProjectionEvent(roomId, projectedEvent, page.includes);
              }
            }
          }
          break;
        }
        case 'roomTimelineEventRemove': {
          const removal = operation.operation.value;
          this.messageSearch.invalidateMessage(removal.roomId, removal.eventId, true);
          this.#roomMessages[removal.roomId]?.removeRoomProjectionEvent(
            removal.roomId,
            removal.eventId
          );
          for (const [key, threadStore] of Object.entries(this.#threadMessages)) {
            if (!key.startsWith(`${removal.roomId}\u0000`)) continue;
            threadStore.removeRoomProjectionEvent(removal.roomId, removal.eventId);
          }
          break;
        }
        case 'roomActivity':
          this.rooms.bumpRoom(operation.operation.value.roomId);
          break;
        case undefined:
          // ServerProjectionStore validates the whole event before either
          // reducer mutates state, so this is unreachable for accepted input.
          throw new Error('unsupported realtime projection operation');
      }
    }
    if (adminRoomLayoutChanged) this.scheduleAdminRoomLayoutRefresh();
  }

  get #adminRoomLayoutActive(): boolean {
    return this.#adminRoomLayoutSubscriptions > 0;
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

  private synchronizeProjectedNavigation(viewer: ViewerState): void {
    const rooms = [...this.projection.rooms.values()].flatMap((entry) => {
      const room = entry.room ? mapDirectoryRoom(entry.room) : null;
      return room ? [room] : [];
    });
    const groups = this.projection.roomGroups.map(mapRoomGroup);
    const membersByRoomId = new SvelteMap<
      string,
      ReturnType<typeof avatarUserFromDirectoryMember>[]
    >();
    const notificationCountsByRoomId = new SvelteMap<string, number>();
    const messageHistoryByRoomId = new SvelteMap<string, boolean | null>();
    for (const entry of this.projection.rooms.values()) {
      const roomId = entry.room?.room?.id;
      if (!roomId) continue;
      const members = entry.memberUserIds.flatMap((userId) => {
        const user = this.projection.users.get(userId);
        return user ? [avatarUserFromDirectoryMember(mapDirectoryMember(user))] : [];
      });
      membersByRoomId.set(roomId, members);
      notificationCountsByRoomId.set(roomId, entry.viewerNotificationCount);
      messageHistoryByRoomId.set(roomId, entry.hasMessageHistory ?? null);
    }
    this.rooms.replaceProjection(
      viewer,
      rooms,
      groups,
      membersByRoomId,
      notificationCountsByRoomId,
      messageHistoryByRoomId
    );
    this.roomDirectory.replaceProjection(rooms);
  }

  /** Clear every mirror whose authority was invalidated by a reset frame. */
  private resetProjectionMirrors(): void {
    clearUserSummaryCache(this.serverId);
    for (const store of Object.values(this.#roomMessages)) store.resetProjectionState();
    for (const store of Object.values(this.#threadMessages)) store.resetProjectionState();
    for (const store of Object.values(this.#roomFiles)) {
      store.reset({ rehydrateRetained: true });
    }
    this.rooms.resetProjectionState();
    this.roomDirectory.resetProjectionState();
    this.notifications.resetProjectionState();
    this.notificationLevels.clear();
    this.roomUnread.clear();
    this.pendingHighlights.clear();
    this.activeCallRooms.clear();
    this.serverInfo.resetProjectionState();
    this.permissions = { ...EMPTY_PERMISSIONS };
    this.currentUser.loading = true;
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
    if (this.projection.timelines.has(roomId)) return true;
    const room = this.projection.rooms.get(roomId)?.room;
    return room ? mapDirectoryRoom(room)?.kind === RoomKind.DM : false;
  }

  /**
   * Whether this server uses cookie auth (origin) vs bearer auth (remote).
   * Read from the live registered server so it stays correct if the token
   * field is ever updated.
   */
  get #cookieAuth(): boolean {
    return this.#registered.token === null;
  }

  /**
   * Whether this server currently has an authenticated user.
   * - Cookie auth (origin): true when `currentUser.user` is set.
   * - Bearer auth (remote): true when an access token is registered.
   */
  get isAuthenticated(): boolean {
    if (this.#registered.reauthRequiredAt !== null) return false;
    if (this.#cookieAuth) {
      return this.currentUser.user != null;
    }
    return this.#registered.token != null;
  }

  /** Update permissions from viewer query data. */
  setPermissions(viewer: ViewerData): void {
    this.permissions = { ...viewer, loaded: true };
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
    if (this.notifications.hasSpaceNotification()) return 'notification';
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

  private reconcileActiveCallTransition(
    event: RealtimeProjectionEvent,
    calls: readonly ActiveCall[]
  ): void {
    const actorId = event.actorId;
    const previousActorCall = actorId ? this.activeCallRooms.findParticipantCall(actorId) : null;
    const nextActorCall = actorId ? projectedParticipantCall(calls, actorId) : null;

    if (!previousActorCall && nextActorCall) {
      this.playCallTransitionSound(
        event.id,
        'join',
        nextActorCall.roomId,
        nextActorCall.callId,
        actorId ?? null
      );
    } else if (
      previousActorCall &&
      !nextActorCall &&
      calls.some(
        (call) =>
          call.room?.id === previousActorCall.roomId &&
          (call.callId || null) === previousActorCall.callId
      )
    ) {
      this.playCallTransitionSound(
        event.id,
        'leave',
        previousActorCall.roomId,
        previousActorCall.callId,
        actorId ?? null
      );
      this.voiceCall.handleParticipantLeftEvent(
        previousActorCall.roomId,
        previousActorCall.callId,
        actorId ?? null,
        this.currentUserId()
      );
    }

    const connectedRoomId = this.voiceCall.roomId;
    if (!connectedRoomId) return;
    const previousCallId = this.activeCallRooms.getCallId(connectedRoomId);
    if (!previousCallId) return;
    const nextCallId = calls.find((call) => call.room?.id === connectedRoomId)?.callId ?? null;
    if (nextCallId !== previousCallId) {
      this.voiceCall.handleCallEndedEvent(connectedRoomId, previousCallId);
    }
  }

  private rememberPlayedCallSoundEvent(eventId: string): void {
    this.#playedCallSoundEventIds.push(eventId);
    if (this.#playedCallSoundEventIds.length > 500) {
      this.#playedCallSoundEventIds.shift();
    }
  }

  private currentUserId(): string | null {
    return this.rooms.currentUserId ?? this.currentUser.user?.id ?? this.#registered.userId;
  }

  /** Remove optimistic call UI state after a local join attempt fails. */
  handleVoiceCallJoinFailed(roomId: string): void {
    const currentUserId = this.rooms.currentUserId;
    this.activeCallRooms.handleLeave(roomId, null, currentUserId);
  }

  /** Clean up resources. */
  dispose(): void {
    this.#disposeEffects();
    this.adminRoomLayout.deactivateProjectionRefresh();
    this.#adminRoomLayoutSubscriptions = 0;
    this.realtimeSync.reset();
    for (const store of Object.values(this.#roomMessages)) store.dispose();
    this.#roomMessages = Object.create(null);
    for (const store of Object.values(this.#roomFiles)) store.dispose();
    this.#roomFiles = Object.create(null);
    for (const store of Object.values(this.#threadMessages)) store.dispose();
    this.#threadMessages = Object.create(null);
    this.#threadMessageRefCounts = Object.create(null);
    this.roomUnread.clear();
    this.notificationLevels.clear();
    this.pendingHighlights.clear();
    this.activeCallRooms.clear();
    this.messageSearch.reset();
  }
}

function projectedParticipantCall(
  calls: readonly ActiveCall[],
  userId: string
): { roomId: string; callId: string | null } | null {
  for (const call of calls) {
    const roomId = call.room?.id;
    if (!roomId) continue;
    if (call.participants.some((participant) => participant.user?.id === userId)) {
      return { roomId, callId: call.callId || null };
    }
  }
  return null;
}
