import { untrack } from 'svelte';
import { PresenceStatus, RoomType, type UserAvatarUserFragment } from '$lib/chatTypes';
import {
  ListMyRoomsRequest,
  type ListMyRoomsResponse,
  type RoomListItemView
} from '$lib/pb/chatto/api/v1/chat_pb';
import { RoomKind, type User } from '$lib/pb/chatto/core/v1/models_pb';
import type { NotificationLevelStore } from '$lib/state/server/notificationLevel.svelte';
import { notificationLevelFromWire } from '$lib/state/server/notificationLevelWire';
import type { RoomUnreadStore } from '$lib/state/server/roomUnread.svelte';
import { wireEventBusManager } from '$lib/state/server/wireEventBus.svelte';
import type { WireClient } from '$lib/wire/client';

export type RoomsListItem = {
  id: string;
  name: string;
  type: RoomType;
  hasUnread: boolean;
  // Populated for DM rooms only — used to derive the display name in the sidebar.
  members: UserAvatarUserFragment[];
};

export type RoomsListGroup = {
  id: string;
  name: string;
  roomIds: string[];
};

const roomStateRefreshEvents = new Set([
  'RoomCreatedEvent',
  'RoomDeletedEvent',
  'RoomGroupsUpdatedEvent',
  'RoomUpdatedEvent',
  'RoomArchivedEvent',
  'RoomUnarchivedEvent',
  'UserJoinedRoomEvent',
  'UserLeftRoomEvent'
]);

export function isRoomStateRefreshEvent(typename: string | undefined): boolean {
  return !!typename && roomStateRefreshEvents.has(typename);
}

/**
 * Reactive store for a server's joined-room list, layout, and per-room
 * unread/mention state. One store per registered server, owned by
 * `ServerStateStore` — consumers (RoomList sidebar, the `/[serverId]` redirect
 * page, etc.) reach the active server's store via
 * `serverRegistry.getStore(activeServerId).rooms`, so the reactivity follows
 * the URL automatically when the user switches servers.
 *
 * Per-room flag mutations (markRead, setUnread, ...) are exposed as methods
 * so components can react to local UI events (entering a room) and to other
 * subscriptions (mentions, marked-as-read across tabs).
 *
 * Subscription events are forwarded via {@link ingestServerEvent}; the
 * server bundle forwards events from every server's bus so each server's
 * store stays current regardless of which one is active.
 */
export class RoomsStore {
  rooms = $state<RoomsListItem[]>([]);
  roomGroups = $state<RoomsListGroup[] | null>(null);
  isInitialLoading = $state(true);
  // The viewer's user ID, captured from the same wire response that produced
  // `rooms`. Prefer this to a global auth context when filtering self out of
  // `room.members`, eliminating route-transition races where auth is briefly
  // empty while rooms are already rendered.
  currentUserId = $state<string | null>(null);

  private loadId = 0;

  constructor(
    private readonly serverId: string,
    private readonly notificationLevels: NotificationLevelStore,
    private readonly roomUnread: RoomUnreadStore,
    private readonly getWireClient: () => WireClient | undefined = () =>
      wireEventBusManager.getClient(serverId)
  ) {}

  // -------------------------------------------------------------------------
  // Loading
  // -------------------------------------------------------------------------

  async refresh(): Promise<void> {
    const thisLoad = ++this.loadId;
    const client = this.getWireClient();
    if (!client) {
      console.warn(`[server:${this.serverId}] cannot refresh rooms before wire client is ready`);
      return;
    }

    const [channelResponse, dmResponse] = await Promise.all([
      client.listMyRooms(new ListMyRoomsRequest({ kind: RoomKind.CHANNEL })),
      client.listMyRooms(new ListMyRoomsRequest({ kind: RoomKind.DM }))
    ]);
    if (this.loadId !== thisLoad) return;

    this.currentUserId = firstViewerUserId(channelResponse, dmResponse);
    const views = uniqueRoomViews([...channelResponse.roomViews, ...dmResponse.roomViews]);

    for (const view of views) {
      const room = view.room;
      const pref = view.viewerNotificationPreference;
      if (room && pref) {
        this.notificationLevels.setRoomPreference(
          room.id,
          notificationLevelFromWire(pref.level),
          notificationLevelFromWire(pref.effectiveLevel)
        );
      }
    }

    this.rooms = views.map(roomListItemFromWire).filter(isRoomsListItem);
    this.roomUnread.initRooms(this.rooms);
    this.roomGroups = channelResponse.roomGroups.length
      ? channelResponse.roomGroups.map((group) => ({
          id: group.id,
          name: group.name,
          roomIds: uniqueStrings(group.roomIds)
        }))
      : null;
    this.isInitialLoading = false;
  }

  // -------------------------------------------------------------------------
  // Per-room flag mutations
  // -------------------------------------------------------------------------

  markRead(roomId: string): void {
    this.patchRoom(roomId, { hasUnread: false });
  }

  setUnread(roomId: string): void {
    this.patchRoom(roomId, { hasUnread: true });
  }

  /**
   * Move a room to the front of the rooms array. RoomList renders DMs in
   * their store-array order, so this is what makes a freshly-active DM jump
   * to the top of the Direct Messages section. Channels render alphabetically
   * regardless of array order, so a bump is a no-op for them visually.
   */
  bumpRoom(roomId: string): void {
    untrack(() => {
      const idx = this.rooms.findIndex((r) => r.id === roomId);
      if (idx <= 0) return;
      const room = this.rooms[idx];
      this.rooms = [room, ...this.rooms.slice(0, idx), ...this.rooms.slice(idx + 1)];
    });
  }

  private patchRoom(roomId: string, patch: Partial<RoomsListItem>): void {
    // Wrapped in untrack so callers can invoke from within a $effect without
    // creating a read+write loop on `rooms` (e.g. `$effect(() =>
    // store.markRead(activeRoomId))`). Reactivity for other consumers still
    // fires from the assignment.
    untrack(() => {
      const idx = this.rooms.findIndex((r) => r.id === roomId);
      if (idx === -1) return;
      this.rooms[idx] = { ...this.rooms[idx], ...patch };
    });
  }

  // -------------------------------------------------------------------------
  // Subscription event ingestion
  // -------------------------------------------------------------------------

  /**
   * Refresh the room list when membership, room metadata, or group layout
   * changes. Other event types (messages, reactions, presence) are no-ops at
   * this level unless the message arrives for a room we don't yet know about —
   * that's how a freshly-created empty DM (filtered from the active
   * member-room DM list until its first message lands) shows up in the
   * sidebar without a manual reload.
   */
  ingestServerEvent(serverEvent: {
    event?: { __typename?: string; roomId?: string | null } | null;
  }): void {
    const event = serverEvent.event;
    if (!event) return;
    if (isRoomStateRefreshEvent(event.__typename)) {
      void this.refresh();
      return;
    }
    if (event.__typename === 'MessagePostedEvent') {
      const roomId = event.roomId;
      if (roomId && !this.rooms.some((r) => r.id === roomId)) {
        void this.refresh();
      }
    }
  }
}

function firstViewerUserId(...responses: ListMyRoomsResponse[]): string | null {
  return responses.find((response) => response.viewerUserId)?.viewerUserId ?? null;
}

function uniqueRoomViews(views: RoomListItemView[]): RoomListItemView[] {
  const seen: Record<string, true> = Object.create(null);
  return views.filter((view) => {
    const id = view.room?.id;
    if (!id || seen[id]) return false;
    seen[id] = true;
    return true;
  });
}

function uniqueStrings(values: string[]): string[] {
  return [...new Set(values)];
}

function roomListItemFromWire(view: RoomListItemView): RoomsListItem | null {
  const room = view.room;
  if (!room || room.archived) return null;
  return {
    id: room.id,
    name: room.name,
    type: roomTypeFromWire(room.kind),
    hasUnread: view.hasUnread,
    members: view.members.map(userToAvatarFragment)
  };
}

function isRoomsListItem(value: RoomsListItem | null): value is RoomsListItem {
  return value !== null;
}

function roomTypeFromWire(kind: RoomKind): RoomType {
  return kind === RoomKind.DM ? RoomType.Dm : RoomType.Channel;
}

function userToAvatarFragment(user: User): UserAvatarUserFragment {
  return {
    __typename: 'User',
    id: user.id,
    login: user.login,
    displayName: user.displayName,
    avatarUrl: null,
    presenceStatus: PresenceStatus.Offline
  };
}
