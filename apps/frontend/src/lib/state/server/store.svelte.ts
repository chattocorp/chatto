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
import { eventBusManager } from './eventBus.svelte';
import type { ProjectionHandler } from '$lib/eventBus.svelte';
import type { ServerConnection } from './serverConnection.svelte';
import type { ServerRegistration } from './catalog.svelte';
import type { ServerSession } from './sessions.svelte';
import { playCallSound } from '$lib/audio/callSounds';
import { SvelteSet } from 'svelte/reactivity';
import { ServerProjectionStore } from './projection.svelte';
import { MessagesStore, RoomFilesStore, RoomPinsStore } from '$lib/state/room';
import { clearRoomPinsSeenMarker } from '$lib/state/room/pins.svelte';
import type { RoomMember } from '$lib/state/room';
import type { RealtimeProjectionEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
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
import type { ActiveCall } from '@chatto/api-types/api/v1/voice_calls_pb';
import type { GetViewerResponse } from '@chatto/api-types/api/v1/viewer_pb';
import { MessageSearchStore } from './messageSearch.svelte';
import { MentionRolesStore } from './mentionRoles.svelte';
import {
  reconcileRegisteredAdminRoomGroupQueries,
  reconcileRegisteredAdminRoomQueries,
  reconcileRegisteredFollowedThreadQueries,
  invalidateRegisteredRoomMemberQueries,
  purgeRegisteredRoomMemberQueries,
  refreshRegisteredAdminQueries,
  removeRegisteredAdminQueries,
  removeRegisteredAdminUserQueries,
  removeRegisteredServerQueries,
  resetRegisteredFollowedThreadQueries,
  scrubRegisteredFollowedThreadMessage,
  scrubRegisteredFollowedThreadRoom,
  scrubRegisteredFollowedThreadUser,
  refreshRegisteredFollowedThreadQueries,
  scrubRegisteredRoomMemberUser,
  updateRegisteredFollowedThreadSummary
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

  const currentGrants = new Set([
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

  private ingestProjectionEvent(event: RealtimeProjectionEvent): void {
    const previousViewer = this.projection.viewer;
    const resetsProjection = event.operations.some(
      (operation) => operation.operation.case === 'reset'
    );
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
          resetRegisteredFollowedThreadQueries(this.serverId);
          this.resetProjectionMirrors();
          this.forEachMessageSearch((store) => store.clearResults());
          adminRoomLayoutChanged = true;
          break;
        case 'serverUpsert':
          this.serverInfo.applyProjectionProfile(operation.operation.value);
          break;
        case 'serverStateUpsert':
          this.serverInfo.applyProjectionState(operation.operation.value);
          this.forEachMessageSearch((store) => store.refreshRetainedResults());
          if (operation.operation.value.pinnedMessageChange) {
            this.#roomPins[
              operation.operation.value.pinnedMessageChange.roomId
            ]?.applyRealtimeChange(operation.operation.value.pinnedMessageChange, event.id);
          }
          break;
        case 'viewerUpsert': {
          if (viewerAuthorizationLost(previousViewer, operation.operation.value)) {
            removeRegisteredAdminQueries(this.serverId);
          }
          const viewer = viewerResponseToState(operation.operation.value);
          this.currentUser.user = viewer.user;
          this.currentUser.loading = false;
          this.setPermissions(viewer);
          this.roomUnread.acknowledgeViewerProjection();
          break;
        }
        case 'userUpsert': {
          const member = mapDirectoryMember(operation.operation.value);
          if (!member.id) break;
          notifyUserSummaries(this.serverId, [member]);
          break;
        }
        case 'userRemove': {
          const userId = operation.operation.value.userId;
          scrubRegisteredFollowedThreadUser(this.serverId);
          scrubRegisteredRoomMemberUser(this.serverId, userId);
          removeRegisteredAdminUserQueries(this.serverId, userId);
          this.forEachMessageSearch((store) => store.invalidateAuthor(userId));
          removeUserSummaryCacheEntry(this.serverId, userId);
          this.notifications.scrubUser(userId);
          this.activeCallRooms.scrubUser(userId);
          for (const store of Object.values(this.#roomMessages)) {
            store.scrubUserReferences(userId);
          }
          for (const store of Object.values(this.#threadMessages)) {
            store.scrubUserReferences(userId);
          }
          break;
        }
        case 'roomUpsert': {
          adminRoomLayoutChanged = true;
          const roomId = operation.operation.value.room?.room?.id;
          if (!roomId) break;
          reconcileRegisteredAdminRoomQueries(this.serverId, roomId);
          const viewerState = operation.operation.value.room?.viewerState;
          this.roomDirectory.acknowledgeMembership(roomId, viewerState?.isMember);
          this.roomUnread.acknowledgeRoomProjection(roomId, viewerState?.hasUnread);
          if (viewerState?.isMember === false) {
            this.forRoomMessageSearch(roomId, (store) => store.revokeRoom(roomId));
            this.clearRoomAccess(roomId);
          } else if (viewerState?.isMember === true) {
            this.restoreRoomAccess(roomId);
          }
          invalidateRegisteredRoomMemberQueries(this.serverId, roomId);
          break;
        }
        case 'roomRemove': {
          adminRoomLayoutChanged = true;
          const roomId = operation.operation.value.roomId;
          reconcileRegisteredAdminRoomQueries(this.serverId, roomId, true);
          this.roomDirectory.removeMembershipProjection(roomId);
          this.roomUnread.removeRoomProjection(roomId);
          this.forRoomMessageSearch(roomId, (store) => store.revokeRoom(roomId));
          purgeRegisteredRoomMemberQueries(this.serverId, roomId);
          this.clearRoomAccess(roomId, true);
          break;
        }
        case 'roomGroupsReplace': {
          adminRoomLayoutChanged = true;
          reconcileRegisteredAdminRoomGroupQueries(
            this.serverId,
            operation.operation.value.groups.map((group) => group.id)
          );
          break;
        }
        case 'roomTimelineReplace': {
          const replacement = operation.operation.value;
          this.forRoomMessageSearch(replacement.roomId, (store) =>
            store.invalidateRoom(replacement.roomId)
          );
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
          const projectedMessage =
            update.event?.event.case === 'messagePosted' ? update.event.event.value.message : null;
          if (update.event && projectedMessage?.deletedAt) {
            scrubRegisteredFollowedThreadMessage(this.serverId, update.roomId, update.event.id);
            this.#roomPins[update.roomId]?.applyMessageRetraction(update.event.id);
          } else if (update.event && projectedMessage) {
            this.#roomPins[update.roomId]?.applyMessageUpdate(update.event.id, projectedMessage);
          }
          const threadSummary = projectedMessage?.thread;
          if (projectedMessage?.threadRootEventId) {
            refreshRegisteredFollowedThreadQueries(this.serverId);
          }
          if (
            update.event &&
            projectedMessage &&
            !projectedMessage.threadRootEventId &&
            threadSummary
          ) {
            updateRegisteredFollowedThreadSummary(this.serverId, {
              roomId: update.roomId,
              threadRootEventId: update.event.id,
              replyCount: threadSummary.replyCount,
              lastReplyAt: threadSummary.lastReplyAt?.toDate().toISOString() ?? null,
              hasUnreadReplies: threadSummary.viewerState?.hasUnreadReplies
            });
          }
          if (update.event && !update.reactionChange) {
            const eventId = update.event.id;
            this.forRoomMessageSearch(update.roomId, (store) =>
              store.invalidateMessage(
                update.roomId,
                eventId,
                existingTimelineRows.has(`${update.roomId}\u0000${eventId}`)
              )
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
        case 'notificationOccurrencesReplace': {
          const replacement = operation.operation.value;
          if (replacement.occurrences) {
            this.notifications.replaceOccurrenceProjection(
              mapNotificationOccurrencePage(replacement.occurrences)
            );
          }
          break;
        }
        case 'roomViewerStateReplace': {
          const replacement = operation.operation.value;
          this.roomDirectory.acknowledgeMembership(
            replacement.roomId,
            replacement.viewerState?.isMember
          );
          this.roomUnread.acknowledgeRoomProjection(
            replacement.roomId,
            replacement.viewerState?.hasUnread
          );
          if (replacement.viewerState?.isMember === false) {
            this.forRoomMessageSearch(replacement.roomId, (store) =>
              store.revokeRoom(replacement.roomId)
            );
            this.clearRoomAccess(replacement.roomId);
          } else if (replacement.viewerState?.isMember === true) {
            this.restoreRoomAccess(replacement.roomId);
          }
          break;
        }
        case 'roomViewerActivityReplace': {
          const replacement = operation.operation.value;
          this.roomUnread.acknowledgeRoomProjection(replacement.roomId, replacement.hasUnread);
          break;
        }
        case 'activeCallsReplace': {
          const calls = operation.operation.value.calls;
          this.reconcileActiveCallTransition(event, calls);
          this.activeCallRooms.replaceProjection(calls);
          break;
        }
        case 'presencesReplace': {
          break;
        }
        case 'threadViewerStatesReplace': {
          reconcileRegisteredFollowedThreadQueries(
            this.serverId,
            this.projection.threadViewerStates
          );
          // A reply updates this projection even when the room timeline is not
          // retained. Refresh the feed so its preview, count, and activity order
          // do not depend on an open room or thread pane.
          if (!resetsProjection) refreshRegisteredFollowedThreadQueries(this.serverId);
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
          scrubRegisteredFollowedThreadMessage(this.serverId, removal.roomId, removal.eventId);
          this.forRoomMessageSearch(removal.roomId, (store) =>
            store.invalidateMessage(removal.roomId, removal.eventId, true)
          );
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
