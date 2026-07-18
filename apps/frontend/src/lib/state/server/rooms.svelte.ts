import { untrack } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { RoomType, type UserAvatarUserView } from '$lib/render/types';
import type {
  DirectoryRoomGroup,
  DirectoryRoomGroupItem,
  DirectoryRoomSummary,
  RoomDirectoryAPI
} from '$lib/api-client/roomDirectory';
import { RoomDirectoryScope, RoomKind } from '$lib/api-client/roomDirectory';
import type { MemberDirectoryAPI, DirectoryMember } from '$lib/api-client/memberDirectory';
import type { ViewerState } from '$lib/api-client/viewer';
import type { NotificationLevelStore } from '$lib/state/server/notificationLevel.svelte';
import type { RoomUnreadStore } from '$lib/state/server/roomUnread.svelte';
import type { NotificationAPI } from '$lib/api-client/notifications';
import { ROOM_MEMBERS_PAGE_SIZE } from '$lib/state/room/members.svelte';

export type RoomsListItem = {
  id: string;
  name: string;
  description?: string | null;
  type: RoomType;
  isUniversal: boolean;
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

export function avatarUserFromDirectoryMember(member: DirectoryMember): UserAvatarUserView {
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

function sameStringArray(a: readonly string[], b: readonly string[]): boolean {
  if (a.length !== b.length) return false;
  return a.every((value, index) => value === b[index]);
}

function sameAvatarUser(a: UserAvatarUserView, b: UserAvatarUserView): boolean {
  return (
    a.id === b.id &&
    a.login === b.login &&
    a.displayName === b.displayName &&
    a.deleted === b.deleted &&
    a.avatarUrl === b.avatarUrl &&
    a.presenceStatus === b.presenceStatus &&
    a.customStatus?.emoji === b.customStatus?.emoji &&
    a.customStatus?.text === b.customStatus?.text &&
    a.customStatus?.expiresAt === b.customStatus?.expiresAt
  );
}

function sameAvatarUsers(
  a: readonly UserAvatarUserView[],
  b: readonly UserAvatarUserView[]
): boolean {
  if (a.length !== b.length) return false;
  return a.every((value, index) => {
    const other = b[index];
    return other !== undefined && sameAvatarUser(value, other);
  });
}

function sameRoomListItem(a: RoomsListItem, b: RoomsListItem): boolean {
  return (
    a.id === b.id &&
    a.name === b.name &&
    a.description === b.description &&
    a.type === b.type &&
    a.isUniversal === b.isUniversal &&
    a.viewerIsMember === b.viewerIsMember &&
    a.viewerCanJoinRoom === b.viewerCanJoinRoom &&
    a.viewerNotificationCount === b.viewerNotificationCount &&
    sameAvatarUsers(a.members, b.members)
  );
}

function sameSidebarLink(a: SidebarLinkListItem, b: SidebarLinkListItem): boolean {
  return a.id === b.id && a.label === b.label && a.url === b.url;
}

function sameRoomGroupItem(a: RoomsListGroupItem, b: RoomsListGroupItem): boolean {
  if (a.type !== b.type || a.id !== b.id) return false;
  if (a.type === 'room' && b.type === 'room') return a.roomId === b.roomId;
  if (a.type === 'link' && b.type === 'link') return sameSidebarLink(a.link, b.link);
  return false;
}

function sameRoomGroupItems(
  a: readonly RoomsListGroupItem[],
  b: readonly RoomsListGroupItem[]
): boolean {
  if (a.length !== b.length) return false;
  return a.every((value, index) => {
    const other = b[index];
    return other !== undefined && sameRoomGroupItem(value, other);
  });
}

function sameRoomGroup(a: RoomsListGroup, b: RoomsListGroup): boolean {
  return (
    a.id === b.id &&
    a.name === b.name &&
    sameStringArray(a.roomIds, b.roomIds) &&
    sameRoomGroupItems(a.items ?? [], b.items ?? [])
  );
}

function sameRoomGroups(
  a: readonly RoomsListGroup[] | null,
  b: readonly RoomsListGroup[]
): boolean {
  if (!a || a.length !== b.length) return false;
  return a.every((value, index) => {
    const other = b[index];
    return other !== undefined && sameRoomGroup(value, other);
  });
}

/**
 * Reactive store for a server's joined-room list, layout, and per-room
 * notification counts. One store per registered server, owned by
 * `ServerStateStore` — consumers (RoomList sidebar, the `/[serverId]` redirect
 * page, etc.) reach the active server's store via
 * `serverRegistry.getStore(activeServerId).rooms`, so the reactivity follows
 * the URL automatically when the user switches servers.
 *
 * Room read state is owned separately by `RoomUnreadStore` so the room list
 * and server indicator cannot maintain competing unread projections.
 *
 * Canonical room and viewer state is replaced by the server projection; the
 * Connect API remains available for explicit paginated reads and commands.
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
    const unreadSnapshotRevision = this.roomUnread.captureSnapshotRevision();
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
              (
                await this.memberDirectoryAPI.listRoomMembers(
                  room.id,
                  '',
                  ROOM_MEMBERS_PAGE_SIZE,
                  0
                )
              ).members.map(avatarUserFromDirectoryMember)
            ] as const
        )
      )
    );
    if (this.loadId !== thisLoad) return;

    const nextRooms = [
      ...visibleChannels.map((room) => this.roomListItem(room, [])),
      ...visibleDms.map((room) => this.roomListItem(room, dmMembersByRoomId.get(room.id) ?? []))
    ];
    this.applyRooms(nextRooms);
    this.roomUnread.initRooms([...visibleChannels, ...visibleDms], false, unreadSnapshotRevision);
    void this.refreshNotificationCounts();

    const nextRoomGroups = roomGroups.map((group) => ({
      id: group.id,
      name: group.name,
      roomIds: group.roomIds,
      items: group.items.map(roomGroupItem)
    }));
    if (!sameRoomGroups(this.roomGroups, nextRoomGroups)) {
      this.roomGroups = nextRoomGroups;
    }

    this.isInitialLoading = false;
  }

  /** Replace navigation state from the server-wide realtime projection. */
  replaceProjection(
    viewer: ViewerState,
    rooms: DirectoryRoomSummary[],
    roomGroups: DirectoryRoomGroup[],
    membersByRoomId: ReadonlyMap<string, UserAvatarUserView[]> = new SvelteMap(),
    notificationCountsByRoomId: ReadonlyMap<string, number> = new SvelteMap()
  ): void {
    this.loadId++;
    this.currentUserId = viewer.user.id;
    this.notificationLevels.setServerPreference(
      viewer.serverNotificationPreference.level,
      viewer.serverNotificationPreference.effectiveLevel
    );
    for (const pref of viewer.roomNotificationPreferences) {
      this.notificationLevels.setRoomPreference(pref.roomId, pref.level, pref.effectiveLevel);
    }

    const visibleRooms = uniqueById(rooms.filter((room) => !room.archived));
    this.applyRooms(
      visibleRooms.map((room) => ({
        ...this.roomListItem(room, membersByRoomId.get(room.id) ?? []),
        viewerNotificationCount: notificationCountsByRoomId.get(room.id) ?? 0
      })),
      false
    );
    this.roomUnread.initRooms(visibleRooms);
    this.roomGroups = roomGroups.map((group) => ({
      id: group.id,
      name: group.name,
      roomIds: group.roomIds,
      items: group.items.map(roomGroupItem)
    }));
    this.isInitialLoading = false;
  }

  /** Invalidate all projection-owned navigation state during a reset. */
  resetProjectionState(): void {
    this.loadId++;
    this.notificationCountsLoadId++;
    this.rooms = [];
    this.roomGroups = [];
    this.currentUserId = null;
    this.isInitialLoading = true;
  }

  private roomListItem(room: DirectoryRoomSummary, members: UserAvatarUserView[]): RoomsListItem {
    return {
      id: room.id,
      name: room.name,
      description: room.description,
      type: roomType(room.kind),
      isUniversal: room.isUniversal,
      viewerIsMember: room.isMember,
      viewerCanJoinRoom: room.canJoinRoom,
      viewerNotificationCount: 0,
      members
    };
  }

  private applyRooms(nextRooms: RoomsListItem[], preserveNotificationCounts = true): void {
    const previousById = new SvelteMap(this.rooms.map((room) => [room.id, room]));
    let changed = this.rooms.length !== nextRooms.length;
    const merged = nextRooms.map((room, index) => {
      const previous = previousById.get(room.id);
      const next = {
        ...room,
        viewerNotificationCount: preserveNotificationCounts
          ? (previous?.viewerNotificationCount ?? room.viewerNotificationCount)
          : room.viewerNotificationCount
      };
      if (!previous) {
        changed = true;
        return next;
      }
      if (this.rooms[index]?.id !== room.id || !sameRoomListItem(previous, next)) {
        changed = true;
        return next;
      }
      return previous;
    });

    if (changed) {
      this.rooms = merged;
    }
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
        let changed = false;
        const rooms = this.rooms.map((room) => {
          const viewerNotificationCount = countsByRoomId[room.id] ?? 0;
          if (room.viewerNotificationCount === viewerNotificationCount) return room;
          changed = true;
          return { ...room, viewerNotificationCount };
        });
        if (changed) this.rooms = rooms;
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
    // creating a read+write loop on `rooms`. Reactivity for other consumers
    // still fires from the assignment.
    untrack(() => {
      const idx = this.rooms.findIndex((r) => r.id === roomId);
      if (idx === -1) return;
      this.rooms[idx] = { ...this.rooms[idx], ...patch };
    });
  }
}

function roomGroupItem(item: DirectoryRoomGroupItem): RoomsListGroupItem {
  if (item.type === 'room') {
    return { id: item.id, type: 'room', roomId: item.roomId };
  }
  return { id: item.id, type: 'link', link: item.link };
}
