/**
 * Bundles all server-scoped stores into a single class per server.
 * Created and managed by the ServerRegistry — do not instantiate directly.
 */

import { CallPreferencesState } from './callPreferences.svelte';
import { TimelineSync, type LocalMessageMutation } from './timelineSync';
import { createMessageResourcesAPI } from '$lib/api-client/messageResources';
import { refreshPresencePreference } from '$lib/presenceTracking';
import { affectsViewerPermissions } from './permissionEvents';
import { runResetHandlers } from './resetHandlers';
import { CurrentUserState, type CurrentUser } from '$lib/auth/currentUser.svelte';
import { ServerInfoState } from './state.svelte';
import type { PublicServerInfo } from '$lib/api-client/server';
import {
  NO_SERVER_PERMISSIONS,
  serverPermissionsFromViewer,
  type ServerPermissions
} from './permissions';
import { NotificationStore } from './notifications.svelte';
import { RoomUnreadStore } from './roomUnread.svelte';
import { ReadViewRegistry } from './readViews.svelte';
import { PendingHighlightStore } from './pendingHighlight.svelte';
import { VoiceCallState } from './voiceCall.svelte';
import { ServerPresence } from './presence.svelte';
import { ActiveCallRoomsState } from './activeCallRooms.svelte';
import { NavigationStore } from './rooms.svelte';
import { RoomDirectoryStore } from './roomDirectory.svelte';
import { AdminRoomLayoutStore } from './adminRoomLayout.svelte';
import { createRoomCommandAPI } from '$lib/api-client/rooms';
import { createNotificationAPI } from '$lib/api-client/notifications';
import { createVoiceCallAPI } from '$lib/api-client/voiceCalls';
import { createAdminRoomLayoutAPI } from '$lib/api-client/adminRoomLayout';
import { createMessageSearchAPI } from '$lib/api-client/messageSearch';
import { createMemberDirectoryAPI } from '$lib/api-client/memberDirectory';
import { createRoleAPI } from '$lib/api-client/roles';
import {
  createRealtimeResourceAPI,
  RealtimeResourceUpdate,
  type RealtimeResourceAPI,
  type RealtimeResourceFamily
} from '$lib/api-client/realtimeResources';
import { eventBusManager } from './realtimeTransport.svelte';
import { RealtimeProjectionUpdate, type ProjectionHandler } from '$lib/eventBus.svelte';
import type { ServerConnection } from './serverConnection.svelte';
import type { ServerRegistration } from './catalog.svelte';
import type { ServerSession } from './sessions.svelte';
import { SvelteDate, SvelteMap, SvelteSet } from 'svelte/reactivity';
import { ServerProjectionStore } from './projection.svelte';
import { getUserStore } from './users.svelte';
import type { RoomMember } from '$lib/state/room';
import { RoomStores, type RoomStoreAccess } from './roomStores.svelte';
import { RoomWithViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
import { GetViewerResponse } from '@chatto/api-types/api/v1/viewer_pb';
import type { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import { RoomKind } from '$lib/api-client/roomDirectory';
import { mapDirectoryMember } from '$lib/api-client/memberDirectory';
import {
  createPrivilegedModeAPI,
  viewerResponseToState,
  type PrivilegedModeAPI
} from '$lib/api-client/viewer';
import { directMessageParticipant } from './rooms.svelte';
import { mapNotificationOccurrencePage } from '$lib/api-client/notifications';
import { RealtimeProjectionSyncState } from './realtimeSync.svelte';
import { PrivilegedModeState } from '@chatto/api-types/api/v1/viewer_pb';
import { MessageSearchStore } from './messageSearch.svelte';
import { MentionRolesStore } from './mentionRoles.svelte';
import { TimelineEventKind } from '$lib/render/timelineEvents';
import {
  queryCaches,
  refreshRegisteredAdminQueries,
  refreshRegisteredServerQueries,
  removeRegisteredAdminQueries,
  removeRegisteredAdminUserQueries,
  removeRegisteredServerQueries
} from '$lib/query/cacheRegistry';

/**
 * What kind of indicator a server (or the DM area) should display.
 * - 'notification' = warning badge, has a pending mention/reply/room-message
 * - 'unread' = grey dot, has unread rooms but no unread notification occurrence
 * - null = no indicator
 */
export type ServerIndicator = 'notification' | 'unread' | null;

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
  /**
   * The viewer's user ID for display and device-local keys: own-message
   * styling, "is this me" checks, and local storage names. Before the account
   * loads it falls back to the ID saved with this device's session. Never use
   * it to scope private data or requests; use {@link accountId} instead.
   */
  get viewerId(): string | null {
    return this.accountId ?? this.#getSession().userId ?? null;
  }

  /**
   * The account that `currentUser` accepted for this server, or null before it
   * loads. It stays set while the session reauthenticates. Use it to scope
   * private queries and requests, and to check that a response still belongs
   * to the same account. It never falls back to the saved session ID.
   */
  get accountId(): string | null {
    return this.currentUser.user?.id ?? null;
  }

  /**
   * The viewer of the realtime projection, or null until the projection is
   * readable. Compare it with projection rows, such as room members.
   */
  get projectionViewerId(): string | null {
    return this.navigation.currentUserId;
  }

  /** Viewer display data; authentication must use currentUser.verifiedUserId instead. */
  get viewerUser(): CurrentUser | undefined {
    return (
      this.currentUser.user ??
      (this.projection.viewer ? viewerResponseToState(this.projection.viewer).user : undefined)
    );
  }
  #privacyCleanupFailed = false;
  /** Stable canonical reducer installed before a projection transport starts. */
  readonly realtimeProjectionHandler: ProjectionHandler = (update) => {
    this.ingestProjectionEvent(update);
    const event = update.event;
    if (event?.event.case === 'presenceChanged' && event.actorId) {
      this.presence.set(event.actorId, event.event.value.status);
    }
  };

  /**
   * What the viewer may do on this server, derived from the viewer projection.
   * A projection for an account that `currentUser` did not accept grants
   * nothing, so the value stays unloaded until the accepted viewer arrives.
   */
  readonly permissions: ServerPermissions = $derived.by(() => {
    const response = this.projection.viewer;
    const accountId = this.accountId;
    if (!response || !accountId || response.user?.profile?.id !== accountId) {
      return NO_SERVER_PERMISSIONS;
    }
    return serverPermissionsFromViewer(viewerResponseToState(response));
  });

  /**
   * Live reference to the registered server. Reads pick up `updateServer`
   * mutations (e.g. token refresh, name change) because the registry stores
   * servers in $state.
   */
  readonly #getSession: () => ServerSession;
  readonly #originServer: boolean;
  readonly #serverConnection: ServerConnection;
  /** Observed presence of this server's users. Every presence reader uses it. */
  readonly presence = new ServerPresence();
  readonly #rooms: RoomStores;
  /**
   * The room-scoped stores: timelines, file lists, pins, members, and room
   * search. Access changes and resets go through this store, which also
   * updates the other owners of room data.
   */
  get rooms(): RoomStoreAccess {
    return this.#rooms;
  }
  #adminRoomLayoutSubscriptions = 0;

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
  readonly #projectionReconciliations = new SvelteSet<Promise<void>>();
  readonly #timelines: TimelineSync;

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
    this.serverInfo = new ServerInfoState(
      registration.url,
      publicServerInfoLoader,
      () => this.projection.serverState
    );
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
    this.activeCallRooms = new ActiveCallRoomsState(
      this.voiceCall,
      () => this.projection.activeCalls
    );
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
    this.#rooms = new RoomStores({
      serverId: this.serverId,
      connection: serverConnection,
      presence: this.presence,
      messageSearchAPI,
      realtimeViewerId: () => this.realtimeViewerId(),
      viewerId: () => this.viewerId,
      projectedMemberIds: (roomId) => {
        // Only a DM projection lists every member of its room.
        const room = this.projection.rooms.get(roomId);
        return room?.room?.kind === RoomKind.DM ? room.memberUserIds : null;
      },
      isAuthenticated: () => this.isAuthenticated
    });
    this.#timelines = new TimelineSync({
      rooms: this.#rooms,
      readMessages: (roomId, ids, cursor) =>
        serverConnection.getAPI(createMessageResourcesAPI).read(roomId, ids, cursor),
      generation: () => this.#realtimeProjectionGeneration,
      eventCursor: () => this.#currentEventMinimumCursor,
      track: (read, generation) => this.trackProjectionReconciliation(read, generation),
      actor: (userId) => {
        if (this.#deletedRealtimeUserIds.has(userId)) return { user: null, deleted: true };
        return { user: this.projection.users.view(userId) ?? null, deleted: false };
      }
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
    const previousViewer = this.projection.viewer;
    this.projection.viewer = response;
    if (!this.currentUser.apply(viewerResponseToState(response).user)) return;
    // Mutation and expiry responses are authoritative. Refresh snapshots now,
    // including room-only grants, without waiting for the realtime reconnect.
    if (viewerAuthorizationLost(previousViewer, response)) {
      removeRegisteredAdminQueries(this.serverId);
    } else {
      refreshRegisteredAdminQueries(this.serverId);
    }
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
          'notifications'
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
    for (const store of this.#rooms.all('members')) {
      for (const member of store.members) userIds.add(member.id);
    }
    const viewerId = this.realtimeViewerId();
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
    const presenceReadVersion = this.presence.version;
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
      this.publishProjectionUpdate(new RealtimeProjectionUpdate({ resource }), presenceReadVersion);
    }
    for (const [userId, member] of requestedCachedUsers) {
      if (returnedUserIds.has(userId) || this.projection.users.get(userId) !== member) continue;
      this.#deletedRealtimeUserIds.add(userId);
      this.projection.removeUser(userId);
      this.scrubRemovedUser(userId);
    }

    if (this.#realtimeSnapshotPending) {
      await Promise.all(
        this.#rooms
          .timelines()
          .map((store) =>
            store.hydrateRealtimeProjection(
              cursor,
              () => generation === this.#realtimeProjectionGeneration,
              true
            )
          )
      );
      this.requireCurrentRealtimeProjection(generation);
      // Retained channel membership can have been read before this snapshot.
      // Recheck it at the snapshot cursor before declaring the view current.
      await Promise.all(
        this.#rooms.all('members').map((store) => store.refresh({ minimumCursor: cursor }))
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
      this.#projectionReconciliations.size > 0
    ) {
      await Promise.all([
        ...this.#resourceRefreshes.values(),
        ...(this.#userRefresh ? [this.#userRefresh] : []),
        // Window refreshes are tracked reconciliations for their whole life.
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

  /** Return known follow state from a loaded canonical room timeline. */
  loadedThreadFollowState(roomId: string, threadRootEventId: string): boolean | null {
    const event = this.#rooms.loaded(roomId)?.messages?.getEventById(threadRootEventId);
    if (event?.event.kind !== TimelineEventKind.MessagePosted) return null;
    return event.event.viewerIsFollowingThread ?? null;
  }

  /** Check loaded canonical room timelines for one unread followed thread. */
  hasUnreadFollowedThreadInLoadedRooms(): boolean {
    return this.#rooms
      .entries()
      .some(
        ([roomId, { messages }]) =>
          messages?.rootEvents.some(
            (event) =>
              event.event.kind === TimelineEventKind.MessagePosted &&
              event.event.viewerIsFollowingThread === true &&
              event.event.viewerHasUnreadThread === true &&
              !this.readViews.covers(roomId, event.id)
          ) ?? false
      );
  }

  /** Reconcile a successful thread read even when its realtime hint is absent or a no-op. */
  reconcileThreadRead(roomId: string, threadRootEventId: string): void {
    if (this.#rooms.loaded(roomId)?.messages) {
      this.#timelines.reconcile(roomId, threadRootEventId);
    }
    queryCaches.followedThreads?.refresh(this.serverId);
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

  /** Load the latest room window at a route boundary when retained data needs it. */
  restoreProjectedRoomWindow(roomId: string): void {
    void this.#rooms.messages(roomId).restoreLatestWindow();
  }

  private updateRoomMembership(roomId: string, userId: string, joined: boolean): void {
    const store = this.#rooms.loaded(roomId)?.members;
    if (!store) return;
    this.trackProjectionReconciliation(
      store.applyMembership(userId, joined, this.#currentEventMinimumCursor),
      this.#realtimeProjectionGeneration
    );
  }

  /** Universal membership depends on server authorization, not only join facts. */
  private invalidateUniversalMembership(): void {
    for (const [id, room] of this.projection.rooms) {
      if (room.room?.universal) this.#rooms.loaded(id)?.members?.resetProjectionState();
    }
  }

  /** Scrub every plaintext timeline mirror for a room at an authorization boundary. */
  private clearRoomAccess(roomId: string, forgetStores = false): void {
    this.#rooms.loaded(roomId)?.members?.resetProjectionState();
    this.voiceCall.handleRoomAccessRevoked(roomId);
    this.projection.removeRoomCalls(roomId);
    this.notifications.clearRoom(roomId);
    this.clearRoomMessageAccess(roomId, forgetStores);
  }

  /** Message-read loss does not imply loss of voice or room membership. */
  private clearRoomMessageAccess(roomId: string, forgetStores = false): void {
    this.#timelines.invalidateRoom(roomId);
    queryCaches.followedThreads?.scrubRoom(this.serverId, roomId);
    this.forRoomMessageSearch(roomId, (store) => store.revokeRoom(roomId));
    this.#rooms.clearMessageAccess(roomId, forgetStores);
  }

  /** Reacquire only mounted stores that were previously scrubbed for access loss. */
  private restoreRoomAccess(roomId: string): void {
    this.notifications.restoreRoom(roomId);
    this.#rooms.restoreAccess(roomId);
  }

  /**
   * Apply one projection update. `presenceReadVersion` marks a user resource
   * that this client read itself; see {@link ServerPresence.applySnapshot}.
   */
  private ingestProjectionEvent(
    update: RealtimeProjectionUpdate,
    presenceReadVersion?: number
  ): void {
    if (
      update.event &&
      affectsViewerPermissions(
        update.event,
        this.realtimeViewerId(),
        this.projection.users.get(this.realtimeViewerId() ?? '')?.roles
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
      this.#timelines.reset();
      this.#permissionCheckGeneration++;
      this.checkingPermissions = false;
      if (update.privacyReset) {
        this.#serverConnection.invalidatePrivateData();
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
          if (!update.retainView) queryCaches.followedThreads?.reset(this.serverId);
        },
        () => {
          if (!update.retainView && !this.resetProjectionMirrors())
            throw new Error('Mirror cleanup incomplete');
        },
        ...[this.messageSearch, ...this.#rooms.all('search')].map((store) => () => {
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
        case 'viewer': {
          const response = resource.value;
          if (!this.checkingPermissions && viewerAuthorizationLost(previousViewer, response)) {
            removeRegisteredAdminQueries(this.serverId);
          }
          if (!this.currentUser.apply(viewerResponseToState(response).user)) return;
          this.roomUnread.acknowledgeViewerProjection();
          break;
        }
        case 'users': {
          // A partial read without presence keeps the known status. A complete
          // replacement, such as the snapshot, which never carries presence,
          // forgets all presence until catch-up reads the users again.
          this.presence.applySnapshot(
            resource.value.users.flatMap((member) =>
              member.user?.id ? [[member.user.id, member.user.presenceStatus] as const] : []
            ),
            update.replaceResource,
            presenceReadVersion
          );
          const members = resource.value.users.map(mapDirectoryMember);
          for (const store of this.#rooms.all('members')) store.updateUsers(members);
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
          queryCaches.server?.reconcileAdminRoomGroups(
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
        case undefined:
          break;
      }
    }

    if (sourceEvent?.event.case === 'messagePinned') {
      const pin = sourceEvent.event.value;
      this.#rooms.loaded(pin.roomId)?.pins?.applyRealtimeChange(pin, true, sourceEvent.id);
    } else if (sourceEvent?.event.case === 'messageUnpinned') {
      const pin = sourceEvent.event.value;
      this.#rooms.loaded(pin.roomId)?.pins?.applyRealtimeChange(pin, false, sourceEvent.id);
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
        this.#rooms.all('members').map((store) => store.refresh({ reauthorize: true }))
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
    const ids = new SvelteSet([...this.#rooms.roomIds(), ...this.projection.rooms.keys()]);
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
      // Snapshot catch-up owns the first full-access timeline read.
      if (!previous && canReadAllMessages && this.#realtimeSnapshotPending) continue;
      if (previous && messageAccess(previous) === messageAccess(next)) continue;
      // Rebuild only affected plaintext stores. Their owners and surrounding
      // page stay mounted, and their request generations fence old responses.
      this.clearRoomMessageAccess(roomId);
      this.restoreRoomAccess(roomId);
      const generation = this.#realtimeProjectionGeneration;
      for (const store of this.#rooms.timelines(roomId)) {
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
        break;
      case 'notifications':
        this.notifications.resetProjectionState();
        break;
      case 'activeCalls':
        this.projection.activeCalls = [];
        break;
    }
  }
  private scrubRemovedUser(userId: string): void {
    this.projection.users.delete(userId);
    for (const [roomId, { members }] of this.#rooms.entries())
      if (members) this.updateRoomMembership(roomId, userId, false);
    queryCaches.followedThreads?.reset(this.serverId);
    queryCaches.roomMembers?.scrubUser(this.serverId, userId);
    removeRegisteredAdminUserQueries(this.serverId, userId);
    this.forEachMessageSearch((store) => store.invalidateAuthor(userId));
    this.notifications.scrubUser(userId);
    for (const store of this.#rooms.timelines()) store.scrubUserReferences(userId);
  }

  private scrubRemovedRoom(roomId: string): void {
    this.roomDirectory.removeMembershipProjection(roomId);
    this.roomUnread.removeRoomProjection(roomId);
    this.forRoomMessageSearch(roomId, (store) => store.revokeRoom(roomId));
    queryCaches.roomMembers?.purgeRoom(this.serverId, roomId);
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
    const presenceReadVersion = this.presence.version;
    const resources = await this.#realtimeResources.readUsers(missingIds, minimumCursor);
    this.requireCurrentRealtimeProjection(generation);
    for (const resource of resources) {
      this.publishProjectionUpdate(new RealtimeProjectionUpdate({ resource }), presenceReadVersion);
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
        const presenceReadVersion = this.presence.version;
        const resources = await this.#realtimeResources.readUsers(ids, cursor);
        this.requireCurrentRealtimeProjection(readGeneration);
        for (const resource of resources) {
          this.publishProjectionUpdate(
            new RealtimeProjectionUpdate({ resource }),
            presenceReadVersion
          );
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

  /**
   * Apply a resource that this client read and notify every consumer of the
   * server bus. Give the presence version from before a user read, so a
   * presence change that arrived during the read is kept.
   */
  private publishProjectionUpdate(
    update: RealtimeProjectionUpdate,
    presenceReadVersion?: number
  ): void {
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
    this.ingestProjectionEvent(update, presenceReadVersion);
    eventBusManager.getBus(this.serverId)?.notify(update);
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
        this.setProjectedUserRole(
          payload.value.userId,
          payload.value.roleName,
          payload.case === 'roleAssigned'
        );
        this.refreshRealtimeUsers([payload.value.userId]);
        if (refreshQueries) queryCaches.server?.refreshRoles(this.serverId);
        return;
      }
      case 'roleDeleted':
        this.invalidateUniversalMembership();
        for (const [userId, member] of this.projection.users) {
          if (member.roles.includes(payload.value.roleName)) {
            this.setProjectedUserRole(userId, payload.value.roleName, false);
          }
        }
        this.mentionRoles.invalidate();
        void this.mentionRoles.load();
        if (refreshQueries) queryCaches.server?.refreshRoles(this.serverId);
        return;
      case 'roleCreated':
      case 'roleUpdated':
      case 'rolesReordered':
        this.mentionRoles.invalidate();
        void this.mentionRoles.load();
        if (refreshQueries) queryCaches.server?.refreshRoles(this.serverId);
        return;
      case 'rolePermissionsChanged':
        this.invalidateUniversalMembership();
        if (refreshQueries) queryCaches.server?.refreshRoles(this.serverId);
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
        if (event.actorId === this.realtimeViewerId() && roomId) this.clearRoomAccess(roomId);
        this.#timelines.refreshWindows(roomId, event.id || null);
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
        const anchorEventId =
          rawValue?.messageEventId ?? (payload.case === 'messagePosted' ? event.id : null);
        if (payload.case === 'messagePosted') this.#timelines.ingestPost(event);
        if (anchorEventId)
          this.#timelines.reconcile(
            roomId,
            anchorEventId,
            payload.case === 'messagePosted',
            payload.case === 'messagePosted' ? payload.value.threadRootEventId : undefined
          );
        if (payload.case === 'messageRetracted') {
          this.#timelines.retract(
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
          if (payload.value.threadRootEventId) queryCaches.followedThreads?.refresh(this.serverId);
        }
        if (payload.case === 'messageEdited') queryCaches.followedThreads?.refresh(this.serverId);
        if (payload.case === 'messageRetracted') {
          queryCaches.followedThreads?.retractMessage(
            this.serverId,
            payload.value.roomId,
            payload.value.messageEventId
          );
        }
        return;
      }
      case 'assetProcessingStarted':
      case 'assetProcessingSucceeded':
      case 'assetProcessingFailed':
        if (rawValue?.messageEventId) this.#timelines.reconcile(roomId, rawValue.messageEventId);
        return;
      case 'voiceCallParticipantJoined':
        this.voiceCall.playTransitionSound(
          event.id,
          'join',
          payload.value.roomId,
          payload.value.callId || null,
          event.actorId || null,
          this.realtimeViewerId()
        );
        this.refreshRealtimeResource('activeCalls');
        return;
      case 'voiceCallParticipantLeft':
        this.voiceCall.playTransitionSound(
          event.id,
          'leave',
          payload.value.roomId,
          payload.value.callId || null,
          event.actorId || null,
          this.realtimeViewerId()
        );
        this.voiceCall.handleParticipantLeftEvent(
          payload.value.roomId,
          payload.value.callId || null,
          event.actorId || null,
          this.realtimeViewerId()
        );
        this.refreshRealtimeResource('activeCalls');
        return;
      case 'voiceCallEnded':
        this.voiceCall.handleCallEndedEvent(payload.value.roomId, payload.value.callId || null);
        this.#timelines.refreshWindows(payload.value.roomId, event.id || null, true);
        this.refreshRealtimeResource('activeCalls');
        return;
      case 'voiceCallStarted':
        this.#timelines.refreshWindows(payload.value.roomId, event.id || null, true);
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
          event.actorId !== this.realtimeViewerId() ||
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
          this.#rooms.loaded(roomId)?.members?.resetProjectionState();
        if (payload.case === 'userJoinedRoom') {
          if (roomId && event.actorId) {
            this.updateRoomMembership(roomId, event.actorId, true);
            this.refreshRealtimeUsers([event.actorId]);
          }
          this.#timelines.refreshWindows(roomId, event.id || null);
        }
        if (payload.case === 'roomThreadingModeChanged') {
          this.#timelines.refreshWindows(roomId, event.id || null);
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
        queryCaches.server?.refreshAdmin(this.serverId);
        return;
      case 'viewerPresencePreferenceChanged':
        if (this.accountId) {
          refreshPresencePreference({ serverId: this.serverId, userId: this.accountId });
        }
        return;
      case 'viewerPreferencesChanged':
        this.refreshRealtimeResource('viewer');
        this.refreshRealtimeResource('rooms');
        if (this.accountId) this.refreshRealtimeUsers([this.accountId]);
        return;
      case 'roomReadStateChanged':
        if (this.roomAttentionMayChange(payload.value.roomId))
          this.refreshRealtimeResource('rooms');
        return;
      case 'threadCreated':
        this.#timelines.reconcile(roomId, payload.value.threadRootEventId);
        return;
      case 'threadViewerStateChanged': {
        this.#timelines.setThreadFollowState(
          roomId,
          payload.value.threadRootEventId,
          payload.value.isFollowing
        );
        queryCaches.followedThreads?.refresh(this.serverId);
        return;
      }
      default:
        return;
    }
  }

  /** Replace a projected user with a copy that has, or does not have, one role. */
  private setProjectedUserRole(userId: string, roleName: string, assigned: boolean): void {
    const member = this.projection.users.get(userId);
    if (!member) return;
    const updated = member.clone();
    updated.roles = updated.roles.filter((role) => role !== roleName);
    if (assigned) updated.roles.push(roleName);
    this.projection.users.set(userId, updated);
  }

  /**
   * Show a message change that this client made before its realtime event
   * arrives. See {@link TimelineSync.applyLocalMutation}.
   */
  applyLocalMessageMutation(roomId: string, eventId: string, kind: LocalMessageMutation): void {
    this.#timelines.applyLocalMutation(roomId, eventId, kind);
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

  get #adminRoomLayoutActive(): boolean {
    return this.#adminRoomLayoutSubscriptions > 0;
  }

  private forEachMessageSearch(callback: (store: MessageSearchStore) => void): void {
    callback(this.messageSearch);
    for (const store of this.#rooms.all('search')) callback(store);
  }

  private forRoomMessageSearch(
    roomId: string,
    callback: (store: MessageSearchStore) => void
  ): void {
    callback(this.messageSearch);
    const roomStore = this.#rooms.loaded(roomId)?.search;
    if (roomStore) callback(roomStore);
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
      () => this.presence.clear(),
      ...this.#rooms.resetHandlers(),
      () => this.roomDirectory.resetOptimisticState(),
      () => this.adminRoomLayout.resetProjectionState(),
      () => this.mentionRoles.invalidate(),
      () => this.notifications.resetProjectionState(),
      () => this.roomUnread.clear(),
      () => this.pendingHighlights.clear()
    ]);
    this.voiceCall.forgetTransitionSounds();
    return complete;
  }

  /**
   * Resolved member rows for DM presentation outside the room member store.
   * Deleted participants resolve to deleted placeholders.
   */
  projectedMembersForRoom(roomId: string): RoomMember[] {
    const memberIds = this.projection.rooms.get(roomId)?.memberUserIds ?? [];
    return memberIds.flatMap((userId) => directMessageParticipant(this.projection, userId));
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
    if (this.#cookieAuth) {
      return (
        this.currentUser.user != null &&
        this.currentUser.verifiedUserId === this.currentUser.user.id
      );
    }
    return this.#getSession().token != null;
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
   * The viewer to use when this store interprets realtime events: the
   * projection viewer, then the accepted account, then the saved session ID.
   */
  private realtimeViewerId(): string | null {
    return this.projectionViewerId ?? this.viewerId;
  }

  /** Remove optimistic call UI state after a local join attempt fails. */
  handleVoiceCallJoinFailed(roomId: string): void {
    const currentUserId = this.projectionViewerId;
    if (currentUserId) this.projection.removeCallParticipant(roomId, currentUserId);
  }

  /** Clean up resources. */
  dispose(): void {
    eventBusManager.getBus(this.serverId)?.clearReducer(this.realtimeProjectionHandler);
    this.currentUser.reset();
    this.#timelines.reset();
    this.projection.users.clear();
    this.readViews.clear();
    // In-flight destination and realtime reads must not revive a retired store.
    this.#realtimeProjectionGeneration++;
    this.#permissionCheckGeneration++;
    this.checkingPermissions = false;
    this.#serverConnection.invalidatePrivateData();
    removeRegisteredServerQueries(this.serverId);
    this.#rooms.dispose();
    this.presence.clear();
    this.adminRoomLayout.deactivateProjectionRefresh();
    this.#adminRoomLayoutSubscriptions = 0;
    this.realtimeSync.reset();
    this.roomUnread.clear();
    this.pendingHighlights.clear();
    this.messageSearch.reset();
  }
}
