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
import type { EventBusCatchUpReason, EventHandler } from '$lib/eventBus.svelte';
import type { ServerConnection } from './serverConnection.svelte';
import type { RegisteredServer } from './registry.svelte';
import { playCallSound } from '$lib/audio/callSounds';
import { RoomEventKind, roomEventKind, type RoomEventKindSource } from '$lib/render/eventKinds';

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

const CATCH_UP_REFRESH_DEDUPE_MS = 1_000;

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

  /** Per-server viewer permissions (loaded by ServerSidebarEntry). */
  permissions = $state<ServerPermissions>(EMPTY_PERMISSIONS);

  /**
   * Live reference to the registered server. Reads pick up `updateServer`
   * mutations (e.g. token refresh, name change) because the registry stores
   * servers in $state.
   */
  readonly #registered: RegisteredServer;

  /** Disposer for the internal effect root that wires lifecycle reactivity. */
  readonly #disposeEffects: () => void;
  readonly #playedCallSoundEventIds: string[] = [];
  #adminRoomLayoutSubscriptions = 0;
  #lastSuccessfulCatchUpRefreshAt = 0;
  #catchUpRefreshInFlight = false;
  #queuedCatchUpRefreshReason: EventBusCatchUpReason | null = null;

  constructor(
    registered: RegisteredServer,
    serverConnection: ServerConnection,
    publicServerInfoLoader?: (baseUrl: string) => Promise<PublicServerInfo>
  ) {
    this.serverId = registered.id;
    this.#registered = registered;
    const cookieAuth = this.#cookieAuth;

    const connectAPIConfig = {
      baseUrl: serverConnection.connectBaseUrl,
      bearerToken: serverConnection.bearerToken
    };
    const notificationAPI = createNotificationAPI(connectAPIConfig);
    const voiceCallAPI = createVoiceCallAPI(connectAPIConfig);
    const roomDirectoryAPI = createRoomDirectoryAPI({
      serverId: serverConnection.serverId ?? registered.id,
      ...connectAPIConfig
    });
    const adminRoomLayoutAPI = createAdminRoomLayoutAPI({
      serverId: serverConnection.serverId ?? registered.id,
      ...connectAPIConfig
    });
    const adminEventLogAPI = createAdminEventLogAPI(connectAPIConfig);
    const memberDirectoryAPI = createMemberDirectoryAPI(connectAPIConfig);
    this.currentUser = new CurrentUserState(cookieAuth, connectAPIConfig);
    this.serverInfo = new ServerInfoState(registered.url, publicServerInfoLoader, connectAPIConfig);
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
    this.roomDirectory = new RoomDirectoryStore(roomDirectoryAPI, roomCommandAPI);
    this.adminRoomLayout = new AdminRoomLayoutStore(
      adminRoomLayoutAPI,
      roomDirectoryAPI,
      roomCommandAPI
    );
    this.adminEventLog = new AdminEventLogStore(adminEventLogAPI);

    // Self-managed lifecycle for the substores that need fetch / event
    // wiring. Living here (in the per-server bundle) means consumers
    // don't have to scatter $effect + useEvent pairs through pages and
    // layouts — every server keeps itself in sync with its own bus, and
    // switching to a server only swaps which bundle's data the UI reads.
    this.#disposeEffects = $effect.root(() => {
      // Refresh substores whose data depends on an authenticated viewer
      // when the user becomes available. Bearer-auth servers load the
      // user async; cookie-auth servers get it set by
      // AuthenticatedChatProvider after the SvelteKit load resolves.
      // Either way, this effect fires once on auth-flip and seeds the
      // initial data without the UI knowing.
      $effect(() => {
        if (this.currentUser.user) {
          this.serverInfo.refreshAuthenticatedSettings().catch((err) => {
            console.error(
              `[server:${this.#registered.url}] failed to load authenticated server settings`,
              err
            );
          });
          void this.rooms.refresh();
          void this.roomDirectory.refresh();
        }
      });

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
          if (eventKind === RoomEventKind.ServerUpdated) {
            void this.serverInfo.refreshProfile();
            if (this.currentUser.user) {
              this.serverInfo.refreshAuthenticatedSettings().catch((err) => {
                console.error(
                  `[server:${this.#registered.url}] failed to refresh authenticated server settings`,
                  err
                );
              });
            }
          } else if (eventKind === RoomEventKind.CallParticipantJoined) {
            const callEvent = callTransitionEventPayload(event.event);
            if (!callEvent) return;
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
            this.voiceCall.handleCallEndedEvent(callEvent.roomId, callEvent.callId);
          }
        };
        const catchUpHandler = (reason: EventBusCatchUpReason) => {
          void this.refreshProjectedStateAfterMissedEvents(reason);
        };
        bus.handlers.add(handler);
        bus.catchUpHandlers.add(catchUpHandler);
        return () => {
          bus.handlers.delete(handler);
          bus.catchUpHandlers.delete(catchUpHandler);
        };
      });
    });
  }

  private async refreshProjectedStateAfterMissedEvents(
    reason: EventBusCatchUpReason,
    force = false
  ): Promise<void> {
    if (!this.isAuthenticated) return;

    if (this.#catchUpRefreshInFlight) {
      this.#queuedCatchUpRefreshReason = reason;
      console.debug(
        `[server:${this.#registered.url}] queued catch-up refresh while one is running`,
        {
          reason
        }
      );
      return;
    }

    const now = Date.now();
    if (!force && now - this.#lastSuccessfulCatchUpRefreshAt < CATCH_UP_REFRESH_DEDUPE_MS) {
      console.debug(`[server:${this.#registered.url}] skipped duplicate catch-up refresh`, {
        reason
      });
      return;
    }

    this.#catchUpRefreshInFlight = true;
    let failed = false;

    try {
      console.debug(
        `[server:${this.#registered.url}] refreshing projected state after event bus gap`,
        {
          reason
        }
      );

      const run = async (label: string, task: () => Promise<unknown>) => {
        try {
          await task();
        } catch (err) {
          failed = true;
          console.error(`[server:${this.#registered.url}] catch-up refresh failed: ${label}`, err);
        }
      };

      const tasks: Promise<void>[] = [
        run('server profile', () => this.serverInfo.refreshProfile()),
        run('authenticated settings', () => this.serverInfo.refreshAuthenticatedSettings()),
        run('notifications', () => this.notifications.fetch()),
        run('rooms', () => this.rooms.refresh()),
        run('room directory', () => this.roomDirectory.refresh()),
        this.#adminRoomLayoutActive
          ? run('admin room layout', () => this.adminRoomLayout.refresh())
          : Promise.resolve(),
        this.serverInfo.livekitUrl
          ? run('active calls', () => this.activeCallRooms.load())
          : Promise.resolve()
      ];
      await Promise.all(tasks);

      if (!failed) {
        this.#lastSuccessfulCatchUpRefreshAt = Date.now();
        console.debug(
          `[server:${this.#registered.url}] projected state catch-up refresh completed`,
          {
            reason
          }
        );
      }
    } finally {
      this.#catchUpRefreshInFlight = false;
      const queuedReason = this.#queuedCatchUpRefreshReason;
      this.#queuedCatchUpRefreshReason = null;
      if (queuedReason) {
        console.debug(`[server:${this.#registered.url}] running queued catch-up refresh`, {
          reason: queuedReason
        });
        void this.refreshProjectedStateAfterMissedEvents(queuedReason, true);
      }
    }
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
    this.roomUnread.clear();
    this.notificationLevels.clear();
    this.pendingHighlights.clear();
    this.activeCallRooms.clear();
    this.callParticipants.clear();
  }
}
