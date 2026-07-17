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
import { CallParticipantsState } from './callParticipants.svelte';
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
import { createMemberDirectoryAPI } from '$lib/api-client/memberDirectory';
import { getViewerStateViaConnect } from '$lib/api-client/viewer';
import { eventBusManager } from './eventBus.svelte';
import type { EventHandler, ProjectionHandler } from '$lib/eventBus.svelte';
import type { ServerConnection } from './serverConnection.svelte';
import type { RegisteredServer } from './registry.svelte';
import { playCallSound } from '$lib/audio/callSounds';
import { SvelteMap } from 'svelte/reactivity';
import { RoomEventKind, roomEventKind, type RoomEventKindSource } from '$lib/render/eventKinds';
import { useRenderData } from '$lib/render/data';
import { UserAvatarUserViewDocument } from '$lib/render/types';
import { ServerProjectionStore } from './projection.svelte';
import { MessagesStore } from '$lib/state/room';
import type { RoomMember } from '$lib/state/room';
import type { RealtimeProjectionEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import { mapDirectoryRoom, mapRoomGroup } from '$lib/api-client/roomDirectory';
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

type CallTransitionEventPayload = {
  roomId: string;
  callId: string | null;
};

function callTransitionEventPayload(event: RoomEventKindSource): CallTransitionEventPayload | null {
  if (!event || typeof event !== 'object') return null;
  const roomId = 'roomId' in event ? event.roomId : null;
  const callId = 'callId' in event ? event.callId : null;
  if (typeof roomId !== 'string') return null;
  return {
    roomId,
    callId: typeof callId === 'string' ? callId : null
  };
}

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
  readonly callParticipants: CallParticipantsState;
  readonly activeCallRooms: ActiveCallRoomsState;
  readonly rooms: RoomsStore;
  readonly roomDirectory: RoomDirectoryStore;
  readonly adminRoomLayout: AdminRoomLayoutStore;
  readonly adminEventLog: AdminEventLogStore;
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
  #threadMessages: Record<string, MessagesStore> = Object.create(null);

  /** Disposer for the internal effect root that wires lifecycle reactivity. */
  readonly #disposeEffects: () => void;
  readonly #playedCallSoundEventIds: string[] = [];
  #adminRoomLayoutSubscriptions = 0;

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
    this.callParticipants = new CallParticipantsState(voiceCallAPI);
    this.activeCallRooms = new ActiveCallRoomsState(voiceCallAPI, this.voiceCall);
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

    // Self-managed lifecycle for the substores that need fetch / event
    // wiring. Living here (in the per-server bundle) means consumers
    // don't have to scatter $effect + useEvent pairs through pages and
    // layouts — every server keeps itself in sync with its own bus, and
    // switching to a server only swaps which bundle's data the UI reads.
    this.#disposeEffects = $effect.root(() => {
      // Forward live events from this server's bus into the substores
      // that care. `eventBusManager.getBus` reads from a SvelteMap, so
      // this effect re-runs when the bus starts (post-auth for cookie
      // servers) or stops (sign-out / disconnect) and (de)registers
      // the handler accordingly.
      $effect(() => {
        const bus = eventBusManager.getBus(this.serverId);
        if (!bus) return;
        const handler: EventHandler = (event) => {
          this.rooms.ingestServerEvent(event);
          this.roomDirectory.ingestServerEvent(event);
          if (this.#adminRoomLayoutActive) {
            this.adminRoomLayout.ingestServerEvent(event);
          }
          const eventKind = roomEventKind(event.event);
          if (eventKind === RoomEventKind.CallParticipantJoined) {
            const callEvent = callTransitionEventPayload(event.event);
            if (!callEvent || !callEvent.callId) return;
            const actor = event.actor
              ? useRenderData(UserAvatarUserViewDocument, event.actor)
              : null;
            void this.activeCallRooms.handleJoin(callEvent.roomId, callEvent.callId, actor);
            this.playCallTransitionSound(
              event.id,
              'join',
              callEvent.roomId,
              callEvent.callId,
              event.actorId ?? null
            );
          } else if (eventKind === RoomEventKind.CallParticipantLeft) {
            const callEvent = callTransitionEventPayload(event.event);
            if (!callEvent) return;
            this.activeCallRooms.handleLeave(
              callEvent.roomId,
              callEvent.callId,
              event.actorId ?? null
            );
            this.playCallTransitionSound(
              event.id,
              'leave',
              callEvent.roomId,
              callEvent.callId,
              event.actorId ?? null
            );
            this.voiceCall.handleParticipantLeftEvent(
              callEvent.roomId,
              callEvent.callId,
              event.actorId ?? null,
              this.currentUserId()
            );
          } else if (eventKind === RoomEventKind.CallEnded) {
            const callEvent = callTransitionEventPayload(event.event);
            if (!callEvent) return;
            if (callEvent.callId) {
              this.activeCallRooms.handleEnd(callEvent.roomId, callEvent.callId);
            }
            this.voiceCall.handleCallEndedEvent(callEvent.roomId, callEvent.callId);
          }
        };
        const projectionHandler: ProjectionHandler = (event) => this.ingestProjectionEvent(event);
        bus.handlers.add(handler);
        bus.projectionHandlers.add(projectionHandler);
        return () => {
          bus.handlers.delete(handler);
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

  /** Restore the canonical latest window when a route selects this room. */
  restoreProjectedRoomWindow(roomId: string): void {
    const page = this.projection.timelines.get(roomId);
    if (page) this.messagesForRoom(roomId).replaceRoomProjectionPage(roomId, page);
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

  private ingestProjectionEvent(event: RealtimeProjectionEvent): void {
    this.projection.apply(event);
    for (const operation of event.operations) {
      switch (operation.operation.case) {
        case 'reset':
          clearUserSummaryCache(this.serverId);
          for (const store of Object.values(this.#roomMessages)) store.resetProjectionState();
          for (const store of Object.values(this.#threadMessages)) store.resetProjectionState();
          this.rooms.rooms = [];
          this.rooms.roomGroups = [];
          this.rooms.isInitialLoading = true;
          this.roomDirectory.allRooms = [];
          this.roomDirectory.isLoading = true;
          break;
        case 'serverUpsert':
          this.serverInfo.applyProjectionProfile(operation.operation.value);
          break;
        case 'serverStateUpsert':
          this.serverInfo.applyProjectionState(operation.operation.value);
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
        case 'userRemove':
          removeUserSummaryCacheEntry(this.serverId, operation.operation.value.userId);
          break;
        case 'roomUpsert':
        case 'roomRemove':
        case 'roomGroupsReplace': {
          const viewerResponse = this.projection.viewer;
          if (viewerResponse)
            this.synchronizeProjectedNavigation(viewerResponseToState(viewerResponse));
          if (operation.operation.case === 'roomRemove') {
            const store = this.#roomMessages[operation.operation.value.roomId];
            store?.dispose();
            delete this.#roomMessages[operation.operation.value.roomId];
            for (const [key, threadStore] of Object.entries(this.#threadMessages)) {
              if (!key.startsWith(`${operation.operation.value.roomId}\u0000`)) continue;
              threadStore.dispose();
              delete this.#threadMessages[key];
            }
          }
          break;
        }
        case 'roomTimelineReplace': {
          const replacement = operation.operation.value;
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
          if (update.event) {
            this.#roomMessages[update.roomId]?.upsertRoomProjectionEvent(
              update.roomId,
              update.event,
              update.includes,
              update.retainDeletedRow
            );
            for (const [key, threadStore] of Object.entries(this.#threadMessages)) {
              if (!key.startsWith(`${update.roomId}\u0000`)) continue;
              threadStore.upsertRoomProjectionEvent(
                update.roomId,
                update.event,
                update.includes,
                update.retainDeletedRow
              );
            }
            if (
              update.event.event.case === 'messagePosted' &&
              !update.event.event.value.message?.threadRootEventId
            ) {
              this.rooms.bumpRoom(update.roomId);
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
          const viewerResponse = this.projection.viewer;
          if (viewerResponse) {
            this.synchronizeProjectedNavigation(viewerResponseToState(viewerResponse));
          }
          break;
        }
        case 'activeCallsReplace': {
          const calls = operation.operation.value.calls;
          this.activeCallRooms.replaceProjection(calls);
          this.callParticipants.replaceProjection(calls);
          break;
        }
        case 'roomTimelineEventRemove': {
          const removal = operation.operation.value;
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
        case undefined:
          break;
      }
    }
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
    for (const entry of this.projection.rooms.values()) {
      const roomId = entry.room?.room?.id;
      if (!roomId) continue;
      const members = entry.memberUserIds.flatMap((userId) => {
        const user = this.projection.users.get(userId);
        return user ? [avatarUserFromDirectoryMember(mapDirectoryMember(user))] : [];
      });
      membersByRoomId.set(roomId, members);
      notificationCountsByRoomId.set(roomId, entry.viewerNotificationCount);
    }
    this.rooms.replaceProjection(
      viewer,
      rooms,
      groups,
      membersByRoomId,
      notificationCountsByRoomId
    );
    this.roomDirectory.replaceProjection(rooms);
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

  get #adminRoomLayoutActive(): boolean {
    return this.#adminRoomLayoutSubscriptions > 0;
  }

  activateAdminRoomLayout(): () => void {
    this.#adminRoomLayoutSubscriptions += 1;
    void this.adminRoomLayout.refresh();
    return () => {
      this.#adminRoomLayoutSubscriptions = Math.max(0, this.#adminRoomLayoutSubscriptions - 1);
    };
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
    this.callParticipants.handleLeave(roomId, null, currentUserId);
  }

  /** Clean up resources. */
  dispose(): void {
    this.#disposeEffects();
    this.realtimeSync.reset();
    for (const store of Object.values(this.#roomMessages)) store.dispose();
    this.#roomMessages = Object.create(null);
    for (const store of Object.values(this.#threadMessages)) store.dispose();
    this.#threadMessages = Object.create(null);
    this.roomUnread.clear();
    this.notificationLevels.clear();
    this.pendingHighlights.clear();
    this.activeCallRooms.clear();
    this.callParticipants.clear();
  }
}
