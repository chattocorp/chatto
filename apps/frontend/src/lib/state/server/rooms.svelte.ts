import { untrack } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { RoomType, type UserAvatarUserView } from '$lib/render/types';
import {
  isMessagePostedEvent,
  RoomEventKind,
  roomEventKind,
  type RoomEventKindSource
} from '$lib/render/eventKinds';
import type { RoomDirectoryAPI, DirectoryRoomSummary } from '@chatto/api-client/roomDirectory';
import { RoomDirectoryScope, RoomKind } from '@chatto/api-client/roomDirectory';
import type { MemberDirectoryAPI, DirectoryMember } from '@chatto/api-client/memberDirectory';
import type { ViewerState } from '@chatto/api-client/viewer';
import type { NotificationLevelStore } from '$lib/state/server/notificationLevel.svelte';
import type { RoomUnreadStore } from '$lib/state/server/roomUnread.svelte';
import type { NotificationAPI } from '@chatto/api-client/notifications';

export type RoomsListItem = {
  id: string;
  name: string;
  type: RoomType;
  isUniversal: boolean;
  hasUnread: boolean;
  viewerIsMember: boolean;
  viewerCanJoinRoom: boolean;
  viewerNotificationCount: number;
  // Populated for DM rooms only — used to derive the display name in the sidebar.
  members: UserAvatarUserView[];
};

export type RoomsListGroup = {
  id: string;
  name: string;
  roomIds: string[];
  items?: RoomsListGroupItem[];
};

export type SidebarLinkListItem = {
  id: string;
  label: string;
  url: string;
};

export type RoomsListGroupItem =
  | {
      id: string;
      type: 'room';
      roomId: string;
    }
  | {
      id: string;
      type: 'link';
      link: SidebarLinkListItem;
    };

export type ViewerStateLoader = () => Promise<ViewerState>;

function uniqueById<T extends { id: string }>(items: readonly T[] | null | undefined): T[] {
  const seen: Record<string, true> = Object.create(null);
  return (items ?? []).filter((item) => {
    if (seen[item.id]) return false;
    seen[item.id] = true;
    return true;
  });
}

function roomType(kind: RoomKind): RoomType {
  return kind === RoomKind.DM ? RoomType.Dm : RoomType.Channel;
}

function avatarUserFromDirectoryMember(member: DirectoryMember): UserAvatarUserView {
  return {
    id: member.id,
    login: member.login,
    displayName: member.displayName,
    deleted: member.deleted,
    avatarUrl: member.avatarUrl,
    presenceStatus: member.presenceStatus,
    customStatus: member.customStatus
      ? {
          emoji: member.customStatus.emoji,
          text: member.customStatus.text,
          expiresAt: member.customStatus.expiresAt
        }
      : null
  };
}

const roomStateRefreshEvents = new Set<RoomEventKind>([
  RoomEventKind.RoomCreated,
  RoomEventKind.RoomDeleted,
  RoomEventKind.RoomGroupsUpdated,
  RoomEventKind.RoomUpdated,
  RoomEventKind.RoomArchived,
  RoomEventKind.RoomUnarchived,
  RoomEventKind.RoomUniversalChanged,
  RoomEventKind.UserJoinedRoom,
  RoomEventKind.UserLeftRoom
]);

export function isRoomStateRefreshEvent(event: RoomEventKindSource): boolean {
  const kind = roomEventKind(event);
  return kind !== null && roomStateRefreshEvents.has(kind);
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
  // The viewer's user ID, captured from the same sidebar bootstrap query that
  // produced DM `room.members`. Use this in preference to a global auth
  // context when filtering self out of DM labels and avatars.
  currentUserId = $state<string | null>(null);

  private loadId = 0;
  private notificationCountsLoadId = 0;

  constructor(
    private readonly roomDirectoryAPI: RoomDirectoryAPI,
    private readonly memberDirectoryAPI: MemberDirectoryAPI,
    private readonly viewerStateLoader: ViewerStateLoader,
    private readonly notificationLevels: NotificationLevelStore,
    private readonly roomUnread: RoomUnreadStore,
    private readonly notificationAPI: NotificationAPI
  ) {}

  // -------------------------------------------------------------------------
  // Loading
  // -------------------------------------------------------------------------

  async refresh(): Promise<void> {
    const thisLoad = ++this.loadId;
    const [viewer, rooms, roomGroups] = await Promise.all([
      this.viewerStateLoader(),
      this.roomDirectoryAPI.listRooms(RoomDirectoryScope.ALL),
      this.roomDirectoryAPI.listRoomGroups()
    ]);
    if (this.loadId !== thisLoad) return;

    this.currentUserId = viewer.user.id;
    this.notificationLevels.setServerPreference(
      viewer.serverNotificationPreference.level,
      viewer.serverNotificationPreference.effectiveLevel
    );
    for (const pref of viewer.roomNotificationPreferences) {
      this.notificationLevels.setRoomPreference(pref.roomId, pref.level, pref.effectiveLevel);
    }

    const channelRooms = uniqueById(rooms.filter((room) => room.kind === RoomKind.CHANNEL));
    const dmRooms = uniqueById(rooms.filter((room) => room.kind === RoomKind.DM));
    const visibleChannels = channelRooms.filter((room) => !room.archived);
    const visibleDms = dmRooms.filter((room) => !room.archived);
    const dmMembersByRoomId = new SvelteMap<string, UserAvatarUserView[]>(
      await Promise.all(
        visibleDms.map(
          async (room) =>
            [
              room.id,
              (await this.memberDirectoryAPI.listRoomMembers(room.id, '', 100, 0)).members.map(
                avatarUserFromDirectoryMember
              )
            ] as const
        )
      )
    );
    if (this.loadId !== thisLoad) return;

    this.rooms = [
      ...visibleChannels.map((room) => this.roomListItem(room, [])),
      ...visibleDms.map((room) => this.roomListItem(room, dmMembersByRoomId.get(room.id) ?? []))
    ];
    this.roomUnread.initRooms([...visibleChannels, ...visibleDms]);
    void this.refreshNotificationCounts();

    this.roomGroups = roomGroups.map((group) => ({
      id: group.id,
      name: group.name,
      roomIds: group.roomIds,
      items: group.items
    }));

    this.isInitialLoading = false;
  }

  private roomListItem(room: DirectoryRoomSummary, members: UserAvatarUserView[]): RoomsListItem {
    return {
      id: room.id,
      name: room.name,
      type: roomType(room.kind),
      isUniversal: room.isUniversal,
      hasUnread: room.hasUnread,
      viewerIsMember: room.isMember,
      viewerCanJoinRoom: room.canJoinRoom,
      viewerNotificationCount: 0,
      members
    };
  }

  async refreshNotificationCounts(): Promise<void> {
    const loadId = this.loadId;
    const notificationCountsLoadId = ++this.notificationCountsLoadId;

    try {
      const countsByRoomId = await this.notificationAPI.listNotificationCounts();
      if (this.loadId !== loadId || this.notificationCountsLoadId !== notificationCountsLoadId) {
        return;
      }

      untrack(() => {
        this.rooms = this.rooms.map((room) => ({
          ...room,
          viewerNotificationCount: countsByRoomId[room.id] ?? 0
        }));
      });
    } catch (err) {
      if (this.loadId === loadId && this.notificationCountsLoadId === notificationCountsLoadId) {
        console.warn('failed to load room notification counts', err);
      }
    }
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

  incrementUnreadNotification(roomId: string): void {
    const room = this.rooms.find((r) => r.id === roomId);
    if (!room) return;
    this.patchRoom(roomId, { viewerNotificationCount: room.viewerNotificationCount + 1 });
  }

  decrementUnreadNotification(roomId: string, amount = 1): void {
    const room = this.rooms.find((r) => r.id === roomId);
    if (!room) return;
    this.patchRoom(roomId, {
      viewerNotificationCount: Math.max(0, room.viewerNotificationCount - amount)
    });
  }

  clearUnreadNotifications(roomId: string): void {
    this.patchRoom(roomId, { viewerNotificationCount: 0 });
  }

  clearAllUnreadNotifications(): void {
    untrack(() => {
      this.rooms = this.rooms.map((room) => ({
        ...room,
        viewerNotificationCount: 0
      }));
    });
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
  ingestServerEvent(serverEvent: { event?: RoomEventKindSource }): void {
    const event = serverEvent.event;
    if (!event) return;
    if (isRoomStateRefreshEvent(event)) {
      void this.refresh();
      return;
    }
    if (isMessagePostedEvent(event)) {
      const roomId = event.roomId;
      if (roomId && !this.rooms.some((r) => r.id === roomId)) {
        void this.refresh();
      }
    }
  }
}
