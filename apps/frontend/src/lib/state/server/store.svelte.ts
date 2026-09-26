/**
 * Bundles all server-scoped stores into a single class per server.
 * Created and managed by the ServerRegistry — do not instantiate directly.
 */

import { CallPreferencesState } from './callPreferences.svelte';
import { MessageReconciler } from './messageReconciler';
import { createMessageResourcesAPI } from '$lib/api-client/messageResources';
import { refreshPresencePreference } from '$lib/presenceTracking';
import { affectsViewerPermissions } from './permissionEvents';
import { runResetHandlers } from './resetHandlers';
import { CurrentUserState, type CurrentUser } from '$lib/auth/currentUser.svelte';
import { ServerInfoState } from './state.svelte';
import type { PublicServerInfo } from '$lib/api-client/server';
import type { ServerPermissions, ViewerData } from './permissions';
import { NotificationStore } from './notifications.svelte';
import { RoomUnreadStore } from './roomUnread.svelte';
import { ReadViewRegistry } from './readViews.svelte';
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
  RealtimeResourceUpdate,
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
import { untrack } from 'svelte';
import { ServerProjectionStore } from './projection.svelte';
import { getUserStore } from './users.svelte';
import { MessagesStore, RoomFilesStore, RoomPinsStore, RoomMembersStore } from '$lib/state/room';
import type { RoomMember } from '$lib/state/room';
import { clearRoomPinsSeenMarker } from '$lib/state/room/pins.svelte';
import { RoomWithViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
import { ServerPublicProfile } from '@chatto/api-types/api/v1/server_pb';
import { decodePresentation } from '$lib/storage/decodeSavedView';
import { GetViewerResponse } from '@chatto/api-types/api/v1/viewer_pb';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import type { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import { mapDirectoryRoom, RoomKind } from '$lib/api-client/roomDirectory';
import { mapDirectoryMember } from '$lib/api-client/memberDirectory';
import {
  createPrivilegedModeAPI,
  viewerResponseToState,
  type PrivilegedModeAPI
} from '$lib/api-client/viewer';
import { avatarUserFromDirectoryMember } from './rooms.svelte';
import { mapNotificationOccurrencePage } from '$lib/api-client/notifications';
import { RealtimeProjectionSyncState } from './realtimeSync.svelte';
import { PrivilegedModeState } from '@chatto/api-types/api/v1/viewer_pb';
import { MessageSearchStore } from './messageSearch.svelte';
import { MentionRolesStore } from './mentionRoles.svelte';
import { TimelineEventKind, type TimelineEventView } from '$lib/render/timelineEvents';
import { clearSavedView, saveView, type SavedView } from '$lib/storage/savedViews';
import {
  reconcileRegisteredAdminRoomGroupQueries,
  purgeRegisteredRoomMemberQueries,
  refreshRegisteredAdminQueries,
  refreshRegisteredAdminProfileQueries,
  refreshRegisteredRoleQueries,
  removeRegisteredAdminQueries,
  removeRegisteredAdminUserQueries,
  removeRegisteredServerQueries,
  refreshRegisteredServerQueries,
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

/** One bounded timeline read. Different anchors or directions need separate reads. */
type MessageWindowRefresh = {
  anchorEventId: string | null;
  forward: boolean;
  minimumCursor?: string;
  generation: number;
};

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
  readonly readViews = new ReadViewRegistry();
  readonly roomUnread: RoomUnreadStore;
  readonly pendingHighlights: PendingHighlightStore;
  readonly voiceCall: VoiceCallState;
  readonly activeCallRooms: ActiveCallRoomsState;
  readonly navigation: NavigationStore;
  readonly roomDirectory: RoomDirectoryStore;
  readonly adminRoomLayout: AdminRoomLayoutStore;
  readonly messageSearch: MessageSearchStore;
  readonly mentionRoles: MentionRolesStore;
  readonly projection: ServerProjectionStore;
  /** Readiness and opaque resume position for this retained projection. */
  readonly realtimeSync = new RealtimeProjectionSyncState();
  /** Last complete store snapshot for reconnect and offline presentation. */
  savedView = $state.raw<SavedView | null>(null);
  /** Display identity is independent of whether this connection has verified its account. */
  get viewerId(): string | null {
    return this.currentUser.user?.id ?? this.#getSession().userId ?? null;
  }
  /** Viewer display data; authentication must use currentUser.verifiedUserId instead. */
  get viewerUser(): CurrentUser | undefined {
    return (
      this.currentUser.user ??
      (this.projection.viewer ? viewerResponseToState(this.projection.viewer).user : undefined)
    );
  }
  /** A cold disk view can render before its session has been verified. */
  startupPresentationOnly = $state(false);
  /** A registered background server has not started discovery or viewer checks. */
  networkStartupDeferred = $state(false);
  /** New owner allocation is deferred because selectors can allocate inside a derived. */
  #snapshotOwnersVersion = $state(0);
  #disposed = false;
  #snapshotTimer: ReturnType<typeof setTimeout> | undefined;
  #privacyCleanupFailed = false;
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
  #roomMembers: Record<string, RoomMembersStore> = Object.create(null);
  #roomMemberRoots: Record<string, () => void> = Object.create(null);
  #memberPresence = new SvelteMap<string, PresenceStatus>();

  /** Keep presence current for retained rooms and rooms first opened later. */
  readonly realtimePresenceHandler = (event: RealtimeEvent): void => {
    if (event.event.case !== 'presenceChanged' || !event.actorId) return;
    this.updateMemberPresence(event.actorId, event.event.value.status);
  };

  private updateMemberPresence(userId: string, status: PresenceStatus): void {
    this.#memberPresence.set(userId, status);
    for (const store of Object.values(this.#roomMembers)) store.setPresence(userId, status);
  }
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
  readonly #privilegedModeAPI: PrivilegedModeAPI;
  readonly #realtimeResources: RealtimeResourceAPI;
  #realtimeProjectionGeneration = 0;
  #realtimeSnapshotPending = false;
  /** Catch-up reads can replace retained state while live hints arrive. */
  #catchUpResourceReads = 0;
  #permissionCheckGeneration = 0;
  /** Block edits while authoritative permission reads are pending; retain the visible view. */
  checkingPermissions = $state(false);
  /** Deletions stay authoritative until the next exact snapshot resets this projection. */
  readonly #deletedRealtimeUserIds = new SvelteSet<string>();
  readonly #resourceRefreshes = new SvelteMap<RealtimeResourceFamily, Promise<boolean>>();
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
  /** Active reads stay owned until settlement, including reads started without a cursor. */
  readonly #messageWindowRefreshes = new SvelteMap<MessagesStore, Promise<void>>();
  readonly #pendingMessageWindowRefreshes = new WeakMap<MessagesStore, MessageWindowRefresh[]>();
  readonly #projectionReconciliations = new SvelteSet<Promise<void>>();
  readonly #messageReconciler = new MessageReconciler(
    (roomId, ids, cursor) =>
      this.#serverConnection.getAPI(createMessageResourcesAPI).read(roomId, ids, cursor),
    (roomId, cursor) => {
      const timelines = this.loadedMessageStores(roomId).map((store) =>
        store.captureMessageReconciliation()
      );
      const files = this.#roomFiles[roomId];
      const pins = this.#roomPins[roomId];
      return (id, resource, insert) => {
        for (const apply of timelines) apply(id, resource?.timeline ?? null, insert);
        files?.applyMessageUpdate(id, resource?.message ?? null, insert, cursor);
        pins?.applyMessageUpdate(id, resource?.message ?? null, cursor);
      };
    }
  );

  constructor(
    registration: ServerRegistration,
    getSession: () => ServerSession,
    originServer: boolean,
    serverConnection: ServerConnection,
    publicServerInfoLoader?: (baseUrl: string) => Promise<PublicServerInfo>,
    onAuthenticationRequired?: () => void,
    onViewerLoaded?: (user: CurrentUser) => void
  ) {
    this.serverId = registration.id;
    this.#getSession = getSession;
    this.#originServer = originServer;
    this.#serverConnection = serverConnection;
    this.projection = new ServerProjectionStore(
      getUserStore(this.serverId, serverConnection.queryScope)
    );

    const notificationAPI = serverConnection.getAPI(createNotificationAPI);
    const voiceCallAPI = serverConnection.getAPI(createVoiceCallAPI);
    const adminRoomLayoutAPI = serverConnection.getAPI(createAdminRoomLayoutAPI);
    const messageSearchAPI = serverConnection.getAPI(createMessageSearchAPI);
    this.#messageSearchAPI = messageSearchAPI;
    this.#realtimeResources = serverConnection.getAPI(createRealtimeResourceAPI);
    const memberDirectoryAPI = serverConnection.getAPI(createMemberDirectoryAPI);
    const roleAPI = serverConnection.getAPI(createRoleAPI);
    this.#privilegedModeAPI = serverConnection.getAPI(createPrivilegedModeAPI);
    this.currentUser = new CurrentUserState(
      originServer,
      serverConnection.apiConfig,
      undefined,
      onAuthenticationRequired,
      onViewerLoaded
    );
    this.serverInfo = new ServerInfoState(registration.url, publicServerInfoLoader);
    this.notifications = new NotificationStore(notificationAPI, (roomId, threadRootId) =>
      this.readViews.covers(roomId, threadRootId)
    );
    this.roomUnread = new RoomUnreadStore(() => this.projection);
    const roomCommandAPI = serverConnection.getAPI(createRoomCommandAPI);
    this.pendingHighlights = new PendingHighlightStore();
    this.voiceCall = new VoiceCallState(
      voiceCallAPI,
      (roomId) => {
        const state = this.projection.rooms.get(roomId)?.viewerState;
        const granted = (permission: string) =>
          state?.isMember === true &&
          (state.permissions.some((grant) => grant.permission === permission && grant.granted) ??
            false);
        return {
          start: granted('call.start'),
          join: granted('call.join'),
          voice: granted('call.voice'),
          camera: granted('call.camera'),
          screenshare: granted('call.screenshare')
        };
      },
      new CallPreferencesState(this.serverId)
    );
    this.activeCallRooms = new ActiveCallRoomsState(this.voiceCall);
    const notifications = this.notifications;
    this.navigation = new NavigationStore(this.projection, this.realtimeSync, {
      get roomUnreadCounts() {
        return notifications.attention.roomUnreadCounts;
      },
      get roomImportantUnreadCounts() {
        return notifications.attention.roomImportantUnreadCounts;
      }
    });
    this.roomDirectory = new RoomDirectoryStore(
      this.navigation,
      memberDirectoryAPI,
      roomCommandAPI
    );
    this.adminRoomLayout = new AdminRoomLayoutStore(adminRoomLayoutAPI, roomCommandAPI);
    this.messageSearch = new MessageSearchStore(messageSearchAPI, () => this.isAuthenticated);
    this.mentionRoles = new MentionRolesStore(roleAPI, () => this.isAuthenticated);

    // Apply the canonical projection delivered by this server's bus. Transient
    // envelopes are consumed only by components that need one-shot signals.
    this.#disposeEffects = $effect.root(() => {
      $effect(() => {
        void this.#snapshotOwnersVersion;
        void this.realtimeSync.checkpointAt;
        void this.realtimeSync.resumeCursor;
        void this.realtimeSync.lastCaughtUpAt;
        void this.realtimeSync.phase;
        void this.checkingPermissions;
        void this.#projectionReconciliations.size;
        void this.#pendingResourceRefreshes.size;
        void this.notifications.hasPendingMutations;
        void this.notifications.occurrences;
        void this.notifications.loading;
        // Track loaded-window changes without traversing or serializing their rows.
        for (const owner of [
          ...Object.values(this.#roomMessages),
          ...Object.values(this.#threadMessages)
        ]) {
          void owner.events.length;
          void owner.isInitialLoading;
          void owner.isLoadingMore;
          void owner.hasReachedStart;
          void owner.hasPendingMutations;
        }
        for (const owner of Object.values(this.#roomMembers)) owner.trackSnapshotChanges();
        untrack(() => this.scheduleSnapshot());
      });
      $effect(() => {
        const bus = eventBusManager.getBus(this.serverId);
        if (!bus) return;
        bus.projectionHandlers.add(this.realtimeProjectionHandler);
        bus.handlers.add(this.realtimePresenceHandler);
        return () => {
          bus.projectionHandlers.delete(this.realtimeProjectionHandler);
          bus.handlers.delete(this.realtimePresenceHandler);
        };
      });
    });
  }

  /** Change privilege activation and reconcile effective viewer permissions in place. */
  async setPrivilegedMode(active: boolean): Promise<void> {
    const update = active
      ? await this.#privilegedModeAPI.activate()
      : await this.#privilegedModeAPI.deactivate();
    const viewer = this.projection.viewer?.clone();
    if (!viewer) throw new Error('privileged-mode update has no viewer projection');
    viewer.privilegedMode = update.privilegedMode;
    viewer.capabilities = update.capabilities;
    viewer.viewerPermissions = update.viewerPermissions;
    this.applyViewerSnapshot(viewer);
    const authorizationRefreshGeneration = this.realtimeSync.invalidateAuthorization();
    const projectionRefreshed = this.realtimeSync.waitForAuthorizationRefresh(
      authorizationRefreshGeneration
    );
    this.#serverConnection.forceReconnect('privileged mode changed');
    await projectionRefreshed;
  }

  /** Reflect local expiry immediately; the reconnect obtains authoritative
   * effective permissions and catches role changes made during activation. */
  async expirePrivilegedMode(): Promise<void> {
    this.applyPrivilegedModeState(
      new PrivilegedModeState({
        available: this.projection.viewer?.privilegedMode?.available ?? false,
        active: false
      })
    );
    try {
      this.applyViewerSnapshot(await this.#privilegedModeAPI.refresh());
    } catch (error) {
      console.warn('[privileged-mode] failed to refresh effective permissions after expiry', error);
      // Reads must still recheck server authority when the viewer refresh fails.
      refreshRegisteredAdminQueries(this.serverId);
    } finally {
      this.realtimeSync.invalidateAuthorization();
      this.#serverConnection.forceReconnect('privileged mode expired');
    }
  }

  private applyPrivilegedModeState(state: PrivilegedModeState): void {
    this.#permissionCheckGeneration++;
    this.checkingPermissions = false;
    const viewer = this.projection.viewer?.clone();
    if (!viewer) return;
    viewer.privilegedMode = state;
    this.projection.viewer = viewer;
  }

  private applyViewerSnapshot(response: GetViewerResponse): void {
    // An explicit privilege response supersedes pending event-driven checks.
    this.#permissionCheckGeneration++;
    this.checkingPermissions = false;
    this.projection.viewer = response;
    const viewer = viewerResponseToState(response);
    if (!this.currentUser.apply(viewer.user)) return;
    // Mutation and expiry responses are authoritative. Refresh snapshots now,
    // including room-only grants, without waiting for the realtime reconnect.
    this.reconcilePermissions(viewer, true);
  }

  /** Reject work whose resource boundary was superseded by a newer reset. */
  private requireCurrentRealtimeProjection(generation: number): void {
    if (generation !== this.#realtimeProjectionGeneration) {
      throw new Error('realtime projection read was superseded by a newer reset');
    }
  }

  /** Complete auxiliary reads and event reconciliation through `cursor`.
   * Room groups must also be read: their viewer permissions can change when
   * privileged mode changes without a durable room-layout event. */
  async completeRealtimeCatchUp(cursor: string): Promise<void> {
    if (this.#privacyCleanupFailed) throw new Error('Private data cleanup did not complete');
    const generation = this.#realtimeProjectionGeneration;
    this.#catchUpResourceReads++;
    const batches = await Promise.all(
      (
        [
          'serverState',
          'viewer',
          'rooms',
          'roomGroups',
          'notifications',
          ...(this.realtimeSync.restoredFromDisk ? ['activeCalls' as const] : [])
        ] as RealtimeResourceFamily[]
      ).map((family) => this.#realtimeResources.read(family, cursor))
    ).finally(() => {
      this.#catchUpResourceReads--;
    });
    this.requireCurrentRealtimeProjection(generation);
    for (const resource of batches.flat()) {
      this.publishProjectionUpdate(new RealtimeProjectionUpdate({ resource }));
    }

    // Presence and other user current values are not durable replay events.
    // Refresh every user that the retained projection still references after
    // the authoritative room read has been applied.
    const userIds = new SvelteSet(this.projection.users.keys());
    for (const store of Object.values(this.#roomMembers)) {
      for (const member of store.members) userIds.add(member.id);
    }
    const viewerId = this.currentUserId();
    if (viewerId) userIds.add(viewerId);
    for (const room of this.projection.rooms.values()) {
      for (const userId of room.memberUserIds) userIds.add(userId);
    }
    // The snapshot user family is partial. Only this requested batch can
    // confirm an omitted cached account, and a newer write wins the race.
    const requestedCachedUsers = new SvelteMap(
      [...userIds].flatMap((id) => {
        const member = this.projection.users.get(id);
        return member ? [[id, member] as const] : [];
      })
    );
    const userResources = await this.#realtimeResources.readUsers(userIds, cursor);
    this.requireCurrentRealtimeProjection(generation);
    const returnedUserIds = new SvelteSet(
      userResources.flatMap((resource) =>
        resource.resource.case === 'users'
          ? resource.resource.value.users.flatMap((member) =>
              member.user?.id ? [member.user.id] : []
            )
          : []
      )
    );
    for (const resource of userResources) {
      this.publishProjectionUpdate(new RealtimeProjectionUpdate({ resource }));
    }
    for (const [userId, member] of requestedCachedUsers) {
      if (returnedUserIds.has(userId) || this.projection.users.get(userId) !== member) continue;
      this.#deletedRealtimeUserIds.add(userId);
      this.projection.removeUser(userId);
      this.scrubRemovedUser(userId);
    }

    if (this.#realtimeSnapshotPending) {
      await Promise.all(
        [...Object.values(this.#roomMessages), ...Object.values(this.#threadMessages)].map(
          (store) =>
            store.hydrateRealtimeProjection(
              cursor,
              () => generation === this.#realtimeProjectionGeneration,
              true
            )
        )
      );
      this.requireCurrentRealtimeProjection(generation);
    }
    if (this.#realtimeSnapshotPending || this.realtimeSync.restoredFromDisk) {
      // Retained channel membership can have been read before this snapshot.
      // Recheck it at the snapshot cursor before declaring the view current.
      await Promise.all(
        Object.values(this.#roomMembers).map((store) => store.refresh({ minimumCursor: cursor }))
      );
      this.requireCurrentRealtimeProjection(generation);
      this.#realtimeSnapshotPending = false;
    }
    await this.waitForRealtimeReconciliation();
    this.requireCurrentRealtimeProjection(generation);
  }

  /** Wait for queued event reads without starting catch-up resource reads. */
  async waitForRealtimeReconciliation(): Promise<void> {
    const generation = this.#realtimeProjectionGeneration;
    while (
      this.#resourceRefreshes.size > 0 ||
      this.#userRefresh ||
      this.#messageWindowRefreshes.size > 0 ||
      this.#projectionReconciliations.size > 0
    ) {
      await Promise.all([
        ...this.#resourceRefreshes.values(),
        ...(this.#userRefresh ? [this.#userRefresh] : []),
        ...this.#messageWindowRefreshes.values(),
        ...this.#projectionReconciliations
      ]);
      this.requireCurrentRealtimeProjection(generation);
    }
    if (this.#reconciliationError) {
      const error = this.#reconciliationError;
      this.#reconciliationError = null;
      throw error;
    }
  }

  /** Wait for this family's queued reads without consuming cursor-owner errors.
   * Returns false if a read failed. Does not start another resource request.
   */
  async waitForRealtimeResourceRefresh(family: RealtimeResourceFamily): Promise<boolean> {
    const generation = this.#realtimeProjectionGeneration;
    let succeeded = true;
    let refresh: Promise<boolean> | undefined;
    while ((refresh = this.#resourceRefreshes.get(family))) {
      const result = await refresh;
      this.requireCurrentRealtimeProjection(generation);
      succeeded = succeeded && result;
    }
    return succeeded;
  }

  /**
   * Make a command's room destination readable before mounting the room UI.
   * A successful command can precede its realtime event. Refresh missing rooms
   * through the canonical pipeline, which also hydrates DM users and fences resets.
   */
  async ensureRoomAvailable(roomId: string): Promise<void> {
    const generation = this.#realtimeProjectionGeneration;
    if (!this.projection.rooms.get(roomId)?.room) {
      this.refreshRealtimeResource('rooms');
    }
    const refreshed = await this.waitForRealtimeResourceRefresh('rooms');
    this.requireCurrentRealtimeProjection(generation);
    if (!refreshed) throw new Error('Could not load the conversation');
    const room = this.projection.rooms.get(roomId)?.room;
    if (!room || room.archived) throw new Error('Conversation is unavailable');
  }

  /** Stable timeline owner; saved restoration waits for verified live catch-up. */
  messagesForRoom(roomId: string, fromSavedProjection = false): MessagesStore {
    let store = this.#roomMessages[roomId];
    if (store) return store;
    store = new MessagesStore(this.#serverConnection, () => this.currentUser.user?.id ?? null);
    if (fromSavedProjection) store.awaitRoomProjection(roomId);
    else store.setRoom(roomId);
    this.#roomMessages[roomId] = store;
    this.snapshotOwnerAdded();
    return store;
  }

  private snapshotOwnerAdded(): void {
    queueMicrotask(() => {
      if (!this.#disposed) this.#snapshotOwnersVersion++;
    });
  }

  /** Return known follow state from a loaded canonical room timeline. */
  loadedThreadFollowState(roomId: string, threadRootEventId: string): boolean | null {
    const event = this.#roomMessages[roomId]?.getEventById(threadRootEventId);
    if (event?.event.kind !== TimelineEventKind.MessagePosted) return null;
    return event.event.viewerIsFollowingThread ?? null;
  }

  /** Check loaded canonical room timelines for one unread followed thread. */
  hasUnreadFollowedThreadInLoadedRooms(): boolean {
    return Object.entries(this.#roomMessages).some(([roomId, store]) =>
      store.rootEvents.some(
        (event) =>
          event.event.kind === TimelineEventKind.MessagePosted &&
          event.event.viewerIsFollowingThread === true &&
          event.event.viewerHasUnreadThread === true &&
          !this.readViews.covers(roomId, event.id)
      )
    );
  }

  /** Reconcile a successful thread read even when its realtime hint is absent or a no-op. */
  reconcileThreadRead(roomId: string, threadRootEventId: string): void {
    const roomStore = this.#roomMessages[roomId];
    if (roomStore) this.scheduleMessageReconciliation(roomId, threadRootEventId);
    refreshRegisteredFollowedThreadQueries(this.serverId);
    if (
      !this.notifications.hasLoaded ||
      this.notifications.loading ||
      this.notifications.error ||
      (this.notifications.roomUnreadCounts[roomId] ?? 0) > 0 ||
      this.resourceReadMayChange('notifications')
    ) {
      this.refreshRealtimeResource('notifications');
    }
    if (this.roomAttentionMayChange(roomId)) this.refreshRealtimeResource('rooms');
  }

  /** Do not skip a read based on state that an outstanding response can replace. */
  private resourceReadMayChange(family: RealtimeResourceFamily): boolean {
    return (
      this.#resourceRefreshes.has(family) ||
      this.#catchUpResourceReads > 0 ||
      this.#realtimeSnapshotPending ||
      this.checkingPermissions ||
      this.#reconciliationError !== null
    );
  }

  /** Reading an already-read room cannot clear more Badge attention. Unknown state must be read. */
  private roomAttentionMayChange(roomId: string): boolean {
    return (
      this.projection.rooms.get(roomId)?.viewerState?.hasUnread !== false ||
      this.resourceReadMayChange('rooms')
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
        const evicted = this.#roomMessageSearch[oldestRoomId];
        delete this.#roomMessageSearch[oldestRoomId];
        // Selectors can allocate this store during rendering. Release the
        // evicted store immediately, then clear its reactive state after render.
        // Capture the old owner so this cannot reset a replacement for that room.
        if (evicted) queueMicrotask(() => evicted.reset());
      }
    }
    store = new MessageSearchStore(this.#messageSearchAPI, () => this.isAuthenticated);
    this.#roomMessageSearch[roomId] = store;
    this.#roomMessageSearchRecency.push(roomId);
    return store;
  }

  /** Load the latest room window at a route boundary when retained data needs it. */
  restoreProjectedRoomWindow(roomId: string): void {
    const messages = this.messagesForRoom(roomId);
    void messages.restoreLatestWindow();
  }

  /** Membership survives route changes and receives server-level realtime updates. */
  membersForRoom(roomId: string): RoomMembersStore {
    let store = this.#roomMembers[roomId];
    if (!store) {
      // A route can create this store from a derived selector. Give its own
      // derived fields an owner that lasts until this server store is disposed.
      let created!: RoomMembersStore;
      this.#roomMemberRoots[roomId] = $effect.root(() => {
        created = new RoomMembersStore(this.#serverConnection);
        created.setRoom(roomId);
        // Initialize before exposing the store; selectors can run in a derived.
        created.livePresence = new SvelteMap(this.#memberPresence);
      });
      store = created;
      this.#roomMembers[roomId] = store;
      this.snapshotOwnerAdded();
    }
    return store;
  }

  private updateRoomMembership(roomId: string, userId: string, joined: boolean): void {
    const store = this.#roomMembers[roomId];
    if (!store) return;
    this.trackProjectionReconciliation(
      store.applyMembership(userId, joined, this.#currentEventMinimumCursor),
      this.#realtimeProjectionGeneration
    );
  }

  /** Universal membership depends on server authorization, not only join facts. */
  private invalidateUniversalMembership(): void {
    for (const [id, room] of this.projection.rooms) {
      if (room.room?.universal) this.#roomMembers[id]?.resetProjectionState();
    }
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
    this.discardSavedSnapshot();
    this.#roomMembers[roomId]?.resetProjectionState();
    this.voiceCall.handleRoomAccessRevoked(roomId);
    this.activeCallRooms.clearRoom(roomId);
    this.notifications.clearRoom(roomId);
    this.clearRoomMessageAccess(roomId, forgetStores);
  }

  /**
   * Restore a device snapshot only for the same local viewer. A session that
   * already needs reauthentication had its viewer rejected, so its saved view
   * is deleted instead of shown.
   */
  restoreSavedView(view: SavedView | null, beforeConnection = false): void {
    const session = this.#getSession();
    if (!view || view.serverId !== this.serverId || view.userId !== session.userId) return;
    if (session.reauthRequiredAt !== null) {
      this.discardSavedSnapshot();
      return;
    }
    // Decode everything before publishing so corrupt storage cannot partially restore state.
    let restored;
    try {
      restored = decodePresentation(view);
    } catch {
      void clearSavedView(this.serverId, view.userId);
      return;
    }
    if (!this.savedView || view.savedAt > this.savedView.savedAt) this.savedView = view;
    if (
      this.realtimeSync.phase === 'empty' &&
      (beforeConnection || (!this.currentUser.loading && !this.currentUser.user))
    ) {
      this.startupPresentationOnly = true;
      this.#serverConnection.pausePrivateRequests();
      this.serverInfo.name = view.serverName;
      this.projection.server = restored.server;
      this.serverInfo.version = view.presentation.serverVersion ?? '';
      if (view.presentation.searchStatus)
        this.messageSearch.restoreStatus(view.presentation.searchStatus);
      this.projection.activeCalls = restored.activeCalls;
      this.activeCallRooms.replaceProjection(restored.activeCalls);
      this.serverInfo.applyProjectionProfile(restored.server);
      this.projection.roomGroups = restored.groups;
      this.projection.viewer = restored.viewer;
      if (restored.notifications)
        this.notifications.replaceOccurrenceProjection(restored.notifications);
      if (restored.viewer)
        this.permissions = { ...viewerResponseToState(restored.viewer), loaded: true };
      this.projection.serverState = { runtime: restored.runtime, motd: view.presentation.motd };
      this.serverInfo.applyProjectionState(this.projection.serverState);
      for (const user of restored.users) {
        if (user.user?.id) this.projection.users.set(user.user.id, user);
      }
      for (const { saved, resource, events } of restored.rooms) {
        this.projection.rooms.set(saved.id, resource);
        if (saved.timeline)
          this.messagesForRoom(saved.id, true).restorePresentation(
            saved.id,
            events,
            saved.hasReachedStart,
            saved.timeline
          );
        if (saved.members) this.membersForRoom(saved.id).restorePresentation(saved.members);
        for (const thread of saved.threads ?? []) {
          const owner = new MessagesStore(
            this.#serverConnection,
            () => this.currentUser.user?.id ?? null
          );
          owner.restorePresentation(
            saved.id,
            thread.events,
            thread.hasReachedStart,
            thread.timeline,
            thread.rootId
          );
          this.#threadMessages[`${saved.id}\u0000${thread.rootId}`] = owner;
        }
      }
      this.realtimeSync.restoreSavedProjection(view.checkpoint);
    }
  }

  /** Permit transport work only after the server confirms the saved viewer. */
  verifyStartupViewer(userId: string): void {
    if (this.startupPresentationOnly && this.savedView?.userId === userId) {
      this.startupPresentationOnly = false;
      this.#serverConnection.resumePrivateRequests();
    }
  }

  /** Remove saved device content, including a normal view restored from that content. */
  clearSavedPresentation(): void {
    this.#serverConnection.cancelPrivateRequests();
    this.savedView = null;
    this.startupPresentationOnly = false;
    if (!this.realtimeSync.restoredFromDisk) return;
    this.#realtimeProjectionGeneration++;
    this.#permissionCheckGeneration++;
    this.#messageReconciler.reset();
    this.#serverConnection.invalidatePrivateData();
    this.permissions = EMPTY_PERMISSIONS;
    this.projection.reset();
    this.resetProjectionMirrors();
    this.realtimeSync.reset();
  }

  /** Coalesce dirty owners into a bounded-delay capture outside the update path. */
  private scheduleSnapshot(): void {
    if (this.#snapshotTimer !== undefined || this.#disposed || this.realtimeSync.phase !== 'ready')
      return;
    // A server-owned throttle coalesces bursts without postponing forever or
    // cancelling on room navigation. Capture is outside reactive dependency tracking.
    this.#snapshotTimer = setTimeout(() => {
      this.#snapshotTimer = undefined;
      const caughtUpAt = this.realtimeSync.lastCaughtUpAt;
      if (caughtUpAt) untrack(() => this.saveCurrentView(caughtUpAt));
    }, 100);
  }

  /** Persist every loaded owner at a completed, verified reconciliation checkpoint. */
  saveCurrentView(savedAt: number): void {
    if (this.realtimeSync.phase !== 'ready' || this.realtimeSync.lastCaughtUpAt !== savedAt) return;
    if (this.#projectionReconciliations.size > 0 || this.#pendingResourceRefreshes.size > 0) return;
    if (this.#reconciliationError || this.#catchUpResourceReads > 0 || this.checkingPermissions)
      return;
    const checkpoint = this.realtimeSync.resumeCursor;
    const checkpointAt = this.realtimeSync.checkpointAt;
    if (!checkpoint || !checkpointAt || this.notifications.hasPendingMutations) return;
    if (
      [...Object.values(this.#roomMessages), ...Object.values(this.#threadMessages)].some(
        (store) => store.hasPendingMutations
      )
    )
      return;
    const userId = this.currentUser.user?.id ?? this.#getSession().userId;
    if (
      !userId ||
      this.#deletedRealtimeUserIds.has(userId) ||
      !this.projection.viewer ||
      this.projection.viewer.user?.profile?.id !== userId
    )
      return;
    const rooms = [...this.projection.rooms.values()].flatMap((entry) => {
      const room = entry.room ? mapDirectoryRoom(entry) : null;
      if (!room || room.archived) return [];
      const timeline =
        room.isMember && room.canReadMessages === true
          ? this.#roomMessages[room.id]?.captureSnapshot()
          : undefined;
      const members = this.#roomMembers[room.id];
      return [
        {
          id: room.id,
          name: room.name,
          kind: room.kind,
          universal: room.isUniversal,
          resource: entry.toJsonString(),
          events: JSON.parse(JSON.stringify(timeline?.events ?? [])) as TimelineEventView[],
          timeline: timeline?.timeline,
          hasReachedStart: timeline?.hasReachedStart,
          threads:
            room.isMember && room.canReadMessages === true
              ? Object.entries(this.#threadMessages).flatMap(([key, owner]) => {
                  const [roomId, rootId] = key.split('\u0000');
                  if (roomId !== room.id) return [];
                  const snapshot = owner.captureSnapshot();
                  return snapshot ? [{ rootId, ...JSON.parse(JSON.stringify(snapshot)) }] : [];
                })
              : [],
          members: members?.hasFirstPage ? members.capturePresentation() : undefined
        }
      ];
    });
    const view: SavedView = {
      version: 3,
      checkpoint,
      checkpointAt,
      serverId: this.serverId,
      userId,
      viewerName: this.currentUser.user?.displayName ?? '',
      serverName: this.serverInfo.name,
      savedAt: Date.now(),
      rooms,
      presentation: {
        serverVersion: this.serverInfo.version,
        searchStatus: this.messageSearch.statusLoaded
          ? { ...this.messageSearch.status }
          : undefined,
        activeCalls: this.projection.activeCalls.map((call) => call.toJsonString()),
        server: (
          this.projection.server ?? new ServerPublicProfile({ name: this.serverInfo.name })
        ).toJsonString(),
        roomGroups: this.projection.roomGroups.map((group) => group.toJsonString()),
        users: [...this.projection.users.values()].map((member) => {
          // Presence events update a separate runtime owner. Persist its latest
          // value so restored profiles do not briefly show an older status.
          const copy = member.clone();
          if (copy.user) {
            copy.user.presenceStatus =
              this.#memberPresence.get(copy.user.id) ?? copy.user.presenceStatus;
          }
          return copy.toJsonString();
        }),
        viewer: this.projection.viewer.toJsonString(),
        runtime: this.projection.serverState?.runtime?.toJsonString(),
        motd: this.projection.serverState?.motd,
        notifications: this.notifications.capturePresentation()
      }
    };
    this.savedView = view;
    void saveView(view);
  }

  /** Fence pending disk writes whenever copied private content becomes invalid. */
  private discardSavedSnapshot(): void {
    this.savedView = null;
    void clearSavedView(this.serverId, this.currentUserId() ?? undefined);
  }

  /** Message-read loss does not imply loss of voice or room membership. */
  private clearRoomMessageAccess(roomId: string, forgetStores = false): void {
    this.#messageReconciler.invalidateRoom(roomId);
    clearRoomPinsSeenMarker(
      this.serverId,
      this.currentUser.user?.id ?? this.#getSession().userId ?? '',
      roomId
    );
    scrubRegisteredFollowedThreadRoom(this.serverId, roomId);
    this.forRoomMessageSearch(roomId, (store) => store.revokeRoom(roomId));
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
    this.snapshotOwnerAdded();
    return store;
  }

  /** Keep a mounted thread mirror alive until its final consumer unmounts. */
  retainMessagesForThread(roomId: string, threadRootEventId: string, store: MessagesStore): void {
    const key = `${roomId}\u0000${threadRootEventId}`;
    if (this.#threadMessages[key] !== store) return;
    this.#threadMessageRefCounts[key] = (this.#threadMessageRefCounts[key] ?? 0) + 1;
  }

  /** Release the UI consumer; retain the canonical window for replay and persistence. */
  releaseMessagesForThread(roomId: string, threadRootEventId: string, store: MessagesStore): void {
    const key = `${roomId}\u0000${threadRootEventId}`;
    if (this.#threadMessages[key] !== store) return;
    const remaining = (this.#threadMessageRefCounts[key] ?? 1) - 1;
    if (remaining > 0) {
      this.#threadMessageRefCounts[key] = remaining;
      return;
    }
    store.clearViewport();
    delete this.#threadMessageRefCounts[key];
  }

  private ingestProjectionEvent(update: RealtimeProjectionUpdate): void {
    if (
      update.event &&
      affectsViewerPermissions(
        update.event,
        this.currentUserId(),
        this.projection.users.get(this.currentUserId() ?? '')?.roles
      )
    ) {
      this.refreshViewerPermissions(update);
      return;
    }
    const previousViewer = this.projection.viewer;
    const previousRoomIds = new SvelteSet(this.projection.rooms.keys());
    const sourceEvent = update.event;
    let adminRoomLayoutChanged = update.reset;

    if (update.reset) {
      this.#messageReconciler.reset();
      this.#permissionCheckGeneration++;
      this.checkingPermissions = false;
      if (update.privacyReset) {
        this.#serverConnection.invalidatePrivateData();
        this.savedView = null;
        void clearSavedView(this.serverId, this.currentUserId() ?? undefined);
        this.permissions = EMPTY_PERMISSIONS;
        // Clear authority first; optional mirrors must not prevent this boundary.
        this.projection.reset();
      }
      const generation = ++this.#realtimeProjectionGeneration;
      this.#realtimeSnapshotPending = true;
      if (!update.retainView) this.#deletedRealtimeUserIds.clear();
      this.#reconciliationError = null;
      this.#pendingResourceRefreshes.clear();
      this.#pendingUserRefreshIds.clear();
      this.#pendingUserRefreshCursor = undefined;
      this.#pendingUserRefreshGeneration = generation;
      this.#privacyCleanupFailed = !runResetHandlers([
        () => {
          if (update.privacyReset && !removeRegisteredServerQueries(this.serverId))
            throw new Error('Query cleanup incomplete');
        },
        () => {
          if (!update.retainView) resetRegisteredFollowedThreadQueries(this.serverId);
        },
        () => {
          if (!update.retainView && !this.resetProjectionMirrors())
            throw new Error('Mirror cleanup incomplete');
        },
        ...[this.messageSearch, ...Object.values(this.#roomMessageSearch)].map((store) => () => {
          if (!update.retainView) store.clearResults();
        })
      ]);
    }

    if (update.resource?.case === 'rooms') {
      this.reconcileRoomPermissions(update.resource.value.rooms, update.cursor ?? undefined);
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
          if (!this.checkingPermissions && viewerAuthorizationLost(previousViewer, response)) {
            removeRegisteredAdminQueries(this.serverId);
          }
          const viewer = viewerResponseToState(response);
          if (!this.currentUser.apply(viewer.user)) return;
          this.setPermissions(viewer);
          this.roomUnread.acknowledgeViewerProjection();
          break;
        }
        case 'users': {
          const members = resource.value.users.map(mapDirectoryMember);
          for (const member of members) this.updateMemberPresence(member.id, member.presenceStatus);
          for (const store of Object.values(this.#roomMembers)) store.updateUsers(members);
          break;
        }
        case 'rooms':
          // A reset temporarily removes room data, not call access. Keep the
          // media session until fresh permissions arrive; LiveKit access is
          // also enforced independently by the server.
          void this.voiceCall.reconcilePermissions();
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

  /** Reauthorize retained resources in place. Permission events do not discard
   * the projection or its cursor. Individual resource owners remove denied data;
   * query observers and permitted route components retain their lifetime.
   */
  private refreshViewerPermissions(update: RealtimeProjectionUpdate): void {
    const check = ++this.#permissionCheckGeneration;
    const generation = this.#realtimeProjectionGeneration;
    this.checkingPermissions = true;
    const current = () =>
      check === this.#permissionCheckGeneration &&
      generation === this.#realtimeProjectionGeneration;
    const queries = refreshRegisteredServerQueries(this.serverId).catch((error) => {
      if (current()) this.#reconciliationError ??= error;
    });
    const layout = this.#adminRoomLayoutActive
      ? this.adminRoomLayout.refreshPermissions()
      : Promise.resolve(this.adminRoomLayout.resetProjectionState());
    // Search owns plaintext outside the room projection. Fence it immediately,
    // including when an unrelated authority read fails.
    this.forEachMessageSearch((store) => store.refreshPermissions());
    // Apply every semantic change before a later check can supersede this one.
    // The server-wide query refresh above already covers role queries.
    this.#currentEventMinimumCursor = update.cursor ?? undefined;
    try {
      if (update.event) this.invalidateRealtimeEvent(update.event, false);
    } finally {
      this.#currentEventMinimumCursor = undefined;
    }
    const refresh = (async () => {
      // Drain queued follow-up reads too. Waiting only for the current promises
      // lets a queued, older read overwrite the new authority later.
      await Promise.all(
        [...this.#resourceRefreshes.keys()].map((family) =>
          this.waitForRealtimeResourceRefresh(family)
        )
      );
      if (!current()) return;
      const families = [
        'viewer',
        'rooms',
        'roomGroups',
        'serverState',
        'notifications',
        'activeCalls'
      ] as const;
      const reads = await Promise.allSettled(
        families.map(async (family) => {
          try {
            const resources = await this.#realtimeResources.read(
              family,
              update.cursor ?? undefined
            );
            if (!current()) return;
            for (const resource of resources) {
              this.publishProjectionUpdate(
                new RealtimeProjectionUpdate({ resource, cursor: update.cursor })
              );
            }
          } catch (error) {
            if (!current()) return;
            this.clearFailedPermissionResource(family);
            throw error;
          }
        })
      );
      if (!current()) return;
      await Promise.all([queries, layout]);
      if (!current()) return;
      this.invalidateUniversalMembership();
      await Promise.all(
        Object.values(this.#roomMembers).map((store) => store.refresh({ reauthorize: true }))
      );
      // Each cache must complete its own check before a partial failure is
      // reported to the cursor owner for retry.
      const failure = reads.find((result) => result.status === 'rejected');
      if (failure?.status === 'rejected') throw failure.reason;
    })()
      .catch((error) => {
        // A failed read retries through normal cursor reconciliation. Retain the
        // shell and unaffected data; never request a new permission snapshot.
        if (current()) this.#reconciliationError ??= error;
      })
      .finally(() => {
        if (current()) this.checkingPermissions = false;
        this.#projectionReconciliations.delete(refresh);
      });
    this.#projectionReconciliations.add(refresh);
  }

  /** Read and posting changes require fresh message content and reply capabilities. */
  private reconcileRoomPermissions(rooms: RoomWithViewerState[], cursor?: string): void {
    const nextRooms = new SvelteMap(rooms.map((room) => [room.room?.id, room]));
    const ids = new SvelteSet([
      ...Object.keys(this.#roomMessages),
      ...Object.keys(this.#roomFiles),
      ...Object.keys(this.#roomPins),
      ...Object.keys(this.#roomMembers),
      ...this.projection.rooms.keys(),
      ...(this.savedView?.rooms.map((room) => room.id) ?? [])
    ]);
    for (const key of Object.keys(this.#threadMessages)) ids.add(key.split('\u0000')[0]);
    for (const roomId of ids) {
      const next = nextRooms.get(roomId);
      if (!next?.viewerState?.isMember) {
        this.clearRoomAccess(roomId);
        continue;
      }
      const previous = this.projection.rooms.get(roomId);
      const messageAccess = (room: RoomWithViewerState | undefined) =>
        [
          'message.read',
          'message.read-interactions',
          'message.post',
          'message.post-in-thread',
          'message.post-in-interactions'
        ]
          .map(
            (permission) =>
              room?.viewerState?.permissions.some(
                (grant) => grant.permission === permission && grant.granted
              ) ?? false
          )
          .join(',');
      const canReadAllMessages = next.viewerState.permissions.some(
        (grant) => grant.permission === 'message.read' && grant.granted
      );
      if (!canReadAllMessages) this.discardSavedSnapshot();
      // Snapshot catch-up owns the first full-access timeline read.
      if (!previous && canReadAllMessages && this.#realtimeSnapshotPending) continue;
      if (previous && messageAccess(previous) === messageAccess(next)) continue;
      // Rebuild only affected plaintext stores. Their owners and surrounding
      // page stay mounted, and their request generations fence old responses.
      this.clearRoomMessageAccess(roomId);
      this.restoreRoomAccess(roomId);
      const generation = this.#realtimeProjectionGeneration;
      for (const store of [
        this.#roomMessages[roomId],
        ...Object.entries(this.#threadMessages)
          .filter(([key]) => key.startsWith(`${roomId}\u0000`))
          .map(([, store]) => store)
      ]) {
        if (store)
          this.trackProjectionReconciliation(
            store.hydrateRealtimeProjection(
              cursor ?? '',
              () => generation === this.#realtimeProjectionGeneration
            ),
            generation
          );
      }
    }
  }

  /** Fail closed at the failed resource, without discarding unrelated state. */
  private clearFailedPermissionResource(family: RealtimeResourceFamily): void {
    switch (family) {
      case 'viewer':
        this.projection.viewer = null;
        this.permissions = EMPTY_PERMISSIONS;
        break;
      case 'rooms':
        this.reconcileRoomPermissions([]);
        this.projection.rooms.clear();
        break;
      case 'roomGroups':
        this.projection.roomGroups = [];
        break;
      case 'serverState':
        this.projection.serverState = null;
        this.serverInfo.resetProjectionState();
        break;
      case 'notifications':
        this.notifications.resetProjectionState();
        break;
      case 'activeCalls':
        this.projection.activeCalls = [];
        this.activeCallRooms.clear();
        break;
    }
  }
  private scrubRemovedUser(userId: string): void {
    this.discardSavedSnapshot();
    this.projection.users.delete(userId);
    for (const roomId of Object.keys(this.#roomMembers))
      this.updateRoomMembership(roomId, userId, false);
    scrubRegisteredFollowedThreadUser(this.serverId);
    scrubRegisteredRoomMemberUser(this.serverId, userId);
    removeRegisteredAdminUserQueries(this.serverId, userId);
    this.forEachMessageSearch((store) => store.invalidateAuthor(userId));
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
    const pending = this.#pendingResourceRefreshes.get(family);
    this.#pendingResourceRefreshes.set(family, {
      minimumCursor:
        minimumCursor ?? (pending?.generation === generation ? pending.minimumCursor : undefined),
      generation
    });
    if (this.#resourceRefreshes.has(family)) return;
    // Collect adjacent event frames before reading. Once a read starts, later
    // hints stay pending for a follow-up read at their own minimum boundary.
    const refresh = new Promise<void>((resolve) => setTimeout(resolve, 10))
      .then(() => {
        this.requireCurrentRealtimeProjection(generation);
        minimumCursor = this.#pendingResourceRefreshes.get(family)?.minimumCursor;
        this.#pendingResourceRefreshes.delete(family);
        return this.#realtimeResources.read(family, minimumCursor);
      })
      .then(async (resources) => {
        this.requireCurrentRealtimeProjection(generation);
        for (const resource of resources) {
          this.publishProjectionUpdate(new RealtimeProjectionUpdate({ resource }));
        }
        if (family === 'rooms') {
          await this.hydrateProjectedDMUsers(minimumCursor, generation);
        }
        return true;
      })
      .catch((error) => {
        if (generation !== this.#realtimeProjectionGeneration) return false;
        this.#reconciliationError ??= error;
        console.error(`[server:${this.serverId}] resource refresh failed`, family, error);
        return false;
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
    // Filter before both the local reducer and bus consumers see the response.
    // This covers profile refreshes, DM hydration, and catch-up user batches.
    if (update.resource?.case === 'users' && this.#deletedRealtimeUserIds.size > 0) {
      update = new RealtimeProjectionUpdate({
        resource: new RealtimeResourceUpdate({
          resource: {
            case: 'users',
            value: {
              users: update.resource.value.users.filter(
                (member) => !this.#deletedRealtimeUserIds.has(member.user?.id ?? '')
              )
            }
          },
          replace: update.replaceResource
        })
      });
    }
    this.ingestProjectionEvent(update);
    const bus = eventBusManager.getBus(this.serverId);
    if (!bus) return;
    for (const handler of bus.projectionHandlers) {
      if (handler !== this.realtimeProjectionHandler) handler(update);
    }
  }

  private invalidateRealtimeEvent(event: RealtimeEvent, refreshQueries = true): void {
    const payload = event.event;
    const rawValue = payload.value as
      { eventId?: string; messageEventId?: string; roomId?: string; userId?: string } | undefined;
    const roomId = rawValue?.roomId ?? '';

    switch (payload.case) {
      case 'roleAssigned':
      case 'roleRevoked': {
        this.invalidateUniversalMembership();
        const member = this.projection.users.get(payload.value.userId);
        if (member) {
          const updated = member.clone();
          updated.roles = updated.roles.filter((role) => role !== payload.value.roleName);
          if (payload.case === 'roleAssigned') updated.roles.push(payload.value.roleName);
          this.projection.users.set(payload.value.userId, updated);
        }
        this.refreshRealtimeUsers([payload.value.userId]);
        if (refreshQueries) refreshRegisteredRoleQueries(this.serverId);
        return;
      }
      case 'roleDeleted':
        this.invalidateUniversalMembership();
        for (const [userId, member] of this.projection.users) {
          if (!member.roles.includes(payload.value.roleName)) continue;
          const updated = member.clone();
          updated.roles = updated.roles.filter((role) => role !== payload.value.roleName);
          this.projection.users.set(userId, updated);
        }
        this.mentionRoles.invalidate();
        void this.mentionRoles.load();
        if (refreshQueries) refreshRegisteredRoleQueries(this.serverId);
        return;
      case 'roleCreated':
      case 'roleUpdated':
      case 'rolesReordered':
        this.mentionRoles.invalidate();
        void this.mentionRoles.load();
        if (refreshQueries) refreshRegisteredRoleQueries(this.serverId);
        return;
      case 'rolePermissionsChanged':
        this.invalidateUniversalMembership();
        if (refreshQueries) refreshRegisteredRoleQueries(this.serverId);
        return;
      case 'userAccountDeleted': {
        const userId = payload.value.userId;
        this.#deletedRealtimeUserIds.add(userId);
        this.#pendingUserRefreshIds.delete(userId);
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
        if (roomId && event.actorId) this.updateRoomMembership(roomId, event.actorId, false);
        if (event.actorId === this.currentUser.user?.id) {
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
      case 'messagePinned':
      case 'messageUnpinned':
      case 'assetDeleted': {
        if (
          payload.case === 'messageEdited' ||
          payload.case === 'messageRetracted' ||
          payload.case === 'assetDeleted'
        ) {
          this.discardSavedSnapshot();
        }
        const anchorEventId =
          rawValue?.messageEventId ?? (payload.case === 'messagePosted' ? event.id : null);
        if (payload.case === 'messagePosted') this.ingestRealtimeMessagePost(event);
        if (anchorEventId)
          this.scheduleMessageReconciliation(
            roomId,
            anchorEventId,
            payload.case === 'messagePosted',
            payload.case === 'messagePosted' ? payload.value.threadRootEventId : undefined
          );
        if (payload.case === 'messageRetracted') {
          this.applyLoadedMessageRetraction(
            roomId,
            payload.value.messageEventId,
            event.createdAt?.toDate().toISOString() ?? new SvelteDate().toISOString()
          );
        }
        if (roomId) this.forRoomMessageSearch(roomId, (store) => store.invalidateRoom(roomId));
        else this.forEachMessageSearch((store) => store.clearResults());
        if (payload.case === 'messagePosted') {
          // Posts do not establish viewer attention. Its user-scoped hints arrive
          // after the server applies Badge decisions and the poster's read state.
          // Known DM activity is already applied by the room projection.
          if (!this.projection.rooms.has(roomId)) this.refreshRealtimeResource('rooms');
          if (payload.value.threadRootEventId)
            refreshRegisteredFollowedThreadQueries(this.serverId);
        }
        if (payload.case === 'messageEdited' || payload.case === 'messageRetracted') {
          refreshRegisteredFollowedThreadQueries(this.serverId);
        }
        return;
      }
      case 'assetProcessingStarted':
      case 'assetProcessingSucceeded':
      case 'assetProcessingFailed':
        if (rawValue?.messageEventId)
          this.scheduleMessageReconciliation(roomId, rawValue.messageEventId);
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
      case 'notificationOccurrencesChanged':
        this.refreshRealtimeResource('notifications');
        return;
      case 'notificationUnreadStateChanged':
        // A self-authored hint can clear existing attention but cannot create it.
        // Post-commit hints also carry the invalidation for Slow Mode deadlines.
        // Occurrence changes have their own hint and must not be inferred here.
        if (
          !event.actorId ||
          event.actorId !== this.currentUserId() ||
          this.roomAttentionMayChange(payload.value.roomId) ||
          (this.projection.rooms.get(payload.value.roomId)?.room?.slowModeSeconds ?? 0) > 0
        ) {
          this.refreshRealtimeResource('rooms');
        }
        return;
      case 'roomCreated':
      case 'roomUpdated':
      case 'roomArchived':
      case 'roomUnarchived':
      case 'roomUniversalChanged':
      case 'roomSlowModeChanged':
      case 'roomThreadingModeChanged':
      case 'userJoinedRoom':
        if (payload.case === 'roomUniversalChanged' && roomId)
          this.#roomMembers[roomId]?.resetProjectionState();
        if (payload.case === 'userJoinedRoom') {
          if (roomId && event.actorId) {
            this.updateRoomMembership(roomId, event.actorId, true);
            this.refreshRealtimeUsers([event.actorId]);
          }
          this.refreshLoadedMessageWindows(roomId, event.id || null);
        }
        if (payload.case === 'roomThreadingModeChanged') {
          this.refreshLoadedMessageWindows(roomId, event.id || null);
        }
        this.refreshRealtimeResource('rooms');
        this.refreshRealtimeResource('roomGroups');
        return;
      case 'roomLayoutChanged':
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
        if (rawValue?.userId) this.projection.users.invalidate(rawValue.userId);
        if (payload.case === 'userAccountCreated' && rawValue?.userId) {
          this.invalidateUniversalMembership();
        }
        if (rawValue?.userId) this.refreshRealtimeUsers([rawValue.userId]);
        // Admin rows have a separate private cache; public profile hydration
        // cannot update its email, permission, or search snapshots.
        refreshRegisteredAdminProfileQueries(this.serverId);
        return;
      case 'viewerPresencePreferenceChanged':
        if (this.currentUser.user?.id) {
          refreshPresencePreference({ serverId: this.serverId, userId: this.currentUser.user.id });
        }
        return;
      case 'viewerPreferencesChanged':
        this.refreshRealtimeResource('viewer');
        this.refreshRealtimeResource('rooms');
        if (this.currentUser.user?.id) this.refreshRealtimeUsers([this.currentUser.user.id]);
        return;
      case 'roomReadStateChanged':
        if (this.roomAttentionMayChange(payload.value.roomId))
          this.refreshRealtimeResource('rooms');
        return;
      case 'threadCreated':
        this.scheduleMessageReconciliation(roomId, payload.value.threadRootEventId);
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
    const actorDeleted = !!event.actorId && this.#deletedRealtimeUserIds.has(event.actorId);
    const actor = actorDeleted
      ? null
      : actorMember
        ? avatarUserFromDirectoryMember(mapDirectoryMember(actorMember))
        : null;
    const timelineEvent: TimelineEventView = {
      id: event.id,
      createdAt: event.createdAt?.toDate().toISOString() ?? new SvelteDate().toISOString(),
      actorId: event.actorId || null,
      actor,
      actorResolution: actorDeleted ? 'deleted' : actor ? undefined : 'loading',
      event: {
        kind: TimelineEventKind.MessagePosted,
        roomId: posted.roomId,
        body: posted.bodyPlaintext,
        attachments: [],
        linkPreview: null,
        reactions: [],
        updatedAt: null,
        inReplyTo: posted.inReplyTo || null,
        threadRootEventId: posted.threadRootEventId || null,
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
    minimumCursor = this.#currentEventMinimumCursor
  ): void {
    for (const [candidateRoomId, store] of Object.entries(this.#roomMessages)) {
      if (roomId && candidateRoomId !== roomId) continue;
      const visibleAnchor = roomAnchorEventId
        ? (store.refreshAnchorForMessageMutation(roomAnchorEventId) ?? roomAnchorEventId)
        : null;
      this.scheduleMessageWindowRefresh(store, visibleAnchor, roomForward, minimumCursor);
    }
    for (const [key, store] of Object.entries(this.#threadMessages)) {
      if (roomId && !key.startsWith(`${roomId}\u0000`)) continue;
      const visibleAnchor = anchorEventId
        ? (store.refreshAnchorForMessageMutation(anchorEventId) ?? anchorEventId)
        : null;
      this.scheduleMessageWindowRefresh(store, visibleAnchor, threadForward, minimumCursor);
    }
  }

  private loadedMessageStores(roomId: string): MessagesStore[] {
    return [
      ...(this.#roomMessages[roomId] ? [this.#roomMessages[roomId]] : []),
      ...Object.entries(this.#threadMessages)
        .filter(([key]) => key.startsWith(`${roomId}\u0000`))
        .map(([, store]) => store)
    ];
  }

  /** One authoritative read serves every loaded view, including closed-thread files. */
  private scheduleMessageReconciliation(
    roomId: string,
    id: string,
    insert = false,
    threadRootEventId?: string
  ): void {
    if (!roomId || !id) return;
    const stores = this.loadedMessageStores(roomId);
    if (!stores.length && !this.#roomFiles[roomId] && !this.#roomPins[roomId]) return;
    const ids = new SvelteSet([id, ...stores.flatMap((store) => store.relatedMessageIds(id))]);
    for (const related of this.#roomFiles[roomId]?.relatedMessageIds(id) ?? []) ids.add(related);
    if (threadRootEventId) ids.add(threadRootEventId);
    const generation = this.#realtimeProjectionGeneration;
    const cursor = this.#currentEventMinimumCursor;
    let refresh: Promise<void> | undefined;
    for (const target of ids)
      refresh = this.#messageReconciler.enqueue(roomId, target, target === id && insert, cursor);
    if (refresh) this.trackProjectionReconciliation(refresh, generation);
  }

  private applyLoadedMessageRetraction(
    roomId: string,
    messageEventId: string,
    retractedAt: string
  ): void {
    if (!messageEventId) return;
    this.#roomFiles[roomId]?.applyMessageUpdate(messageEventId, null, false);
    this.#roomPins[roomId]?.applyMessageRetraction(messageEventId);
    for (const [candidateRoomId, store] of Object.entries(this.#roomMessages)) {
      if (roomId && candidateRoomId !== roomId) continue;
      store.applyMessageRetraction(messageEventId, retractedAt);
    }
    for (const [key, store] of Object.entries(this.#threadMessages)) {
      if (roomId && !key.startsWith(`${roomId}\u0000`)) continue;
      store.applyMessageRetraction(messageEventId, retractedAt);
    }
  }

  /** Keep every distinct pending read; a bounded page cannot cover other anchors. */
  private scheduleMessageWindowRefresh(
    store: MessagesStore,
    anchorEventId: string | null,
    forward = false,
    minimumCursor?: string
  ): void {
    const generation = this.#realtimeProjectionGeneration;
    if (this.#messageWindowRefreshes.has(store)) {
      const pending = (this.#pendingMessageWindowRefreshes.get(store) ?? []).filter(
        (request) => request.generation === generation
      );
      // Opaque cursors cannot be sorted. Only identical requests can be dropped.
      if (
        !pending.some(
          (request) =>
            request.anchorEventId === anchorEventId &&
            request.forward === forward &&
            request.minimumCursor === minimumCursor
        )
      ) {
        pending.push({ anchorEventId, forward, minimumCursor, generation });
      }
      this.#pendingMessageWindowRefreshes.set(store, pending);
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
        const queue = this.#pendingMessageWindowRefreshes
          .get(store)
          ?.filter((request) => request.generation === this.#realtimeProjectionGeneration);
        const pending = queue?.shift();
        if (queue?.length) this.#pendingMessageWindowRefreshes.set(store, queue);
        else this.#pendingMessageWindowRefreshes.delete(store);
        if (!pending) return;
        this.scheduleMessageWindowRefresh(
          store,
          pending.anchorEventId,
          pending.forward,
          pending.minimumCursor
        );
      });
    this.#messageWindowRefreshes.set(store, refresh);
    this.trackProjectionReconciliation(refresh, generation);
  }

  /** Keep the durable cursor behind every ConnectRPC read caused by its event. */
  private trackProjectionReconciliation(refresh: Promise<unknown>, generation: number): void {
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
    if (roomStore) this.scheduleMessageReconciliation(roomId, threadRootEventId);

    const threadStore = this.#threadMessages[`${roomId}\u0000${threadRootEventId}`];
    threadStore?.setThreadRootFollowState(threadRootEventId, isFollowing);
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
  private resetProjectionMirrors(): boolean {
    const complete = runResetHandlers([
      () => refreshRegisteredAdminQueries(this.serverId),
      () => this.projection.users.clear(),
      () => this.#memberPresence.clear(),
      ...Object.values(this.#roomMembers).map((store) => () => store.resetProjectionState()),
      ...Object.values(this.#roomMessages).map((store) => () => store.resetProjectionState()),
      ...Object.values(this.#threadMessages).map((store) => () => store.resetProjectionState()),
      ...Object.values(this.#roomFiles).map(
        (store) => () => store.reset({ rehydrateRetained: true })
      ),
      ...Object.values(this.#roomPins).map(
        (store) => () => store.reset({ rehydrateRetained: true })
      ),
      () => this.roomDirectory.resetOptimisticState(),
      () => this.adminRoomLayout.resetProjectionState(),
      () => this.mentionRoles.invalidate(),
      () => this.notifications.resetProjectionState(),
      () => this.roomUnread.clear(),
      () => this.pendingHighlights.clear(),
      () => this.activeCallRooms.clear(),
      () => this.serverInfo.resetProjectionState()
    ]);
    this.#playedCallSoundEventIds.length = 0;
    return complete;
  }

  /** Complete current room membership from the projection, including pending profiles. */
  projectedMemberIdsForRoom(roomId: string): string[] {
    return this.projection.rooms.get(roomId)?.memberUserIds ?? [];
  }

  /** Resolved member rows for DM presentation outside the room member store. */
  projectedMembersForRoom(roomId: string): RoomMember[] {
    return this.projectedMemberIdsForRoom(roomId).flatMap((userId) => {
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
   * - Cookie auth (origin): true when the current account is verified.
   * - Bearer auth (remote): true when an access token is registered.
   */
  get isAuthenticated(): boolean {
    if (this.#getSession().reauthRequiredAt !== null) return false;
    if (this.networkStartupDeferred) return false;
    if (this.startupPresentationOnly) return false;
    if (this.#cookieAuth) {
      return (
        this.currentUser.user != null &&
        this.currentUser.verifiedUserId === this.currentUser.user.id
      );
    }
    return this.#getSession().token != null;
  }

  /** Update permissions from viewer query data. */
  setPermissions(viewer: ViewerData): void {
    this.reconcilePermissions(viewer, false);
  }

  /** The permission refresh owns query reauthorization; avoid starting it twice. */
  private reconcilePermissions(viewer: ViewerData, refreshAdmin: boolean): void {
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
    if (this.checkingPermissions) return;
    if (lostAdminCapability) {
      removeRegisteredAdminQueries(this.serverId);
    } else if (refreshAdmin) {
      refreshRegisteredAdminQueries(this.serverId);
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
    if (this.notifications.attention.unreadNotificationCount > 0) return 'notification';
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
    this.#disposed = true;
    clearTimeout(this.#snapshotTimer);
    this.#snapshotTimer = undefined;
    this.currentUser.reset();
    this.#messageReconciler.reset();
    this.projection.users.clear();
    this.readViews.clear();
    // In-flight destination and realtime reads must not revive a retired store.
    this.#realtimeProjectionGeneration++;
    this.#permissionCheckGeneration++;
    this.checkingPermissions = false;
    this.#serverConnection.invalidatePrivateData();
    removeRegisteredServerQueries(this.serverId);
    for (const store of Object.values(this.#roomMembers)) store.resetProjectionState();
    this.#roomMembers = Object.create(null);
    for (const dispose of Object.values(this.#roomMemberRoots)) dispose();
    this.#roomMemberRoots = Object.create(null);
    this.#memberPresence.clear();
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
