import { RoomKind } from '$lib/api-client/roomDirectory';
import { roomKindOrChannel } from '$lib/api-client/enumDefaults';
import { mapDirectoryRoom, mapRoomGroup } from '$lib/api-client/roomDirectory';
import { mapDirectoryMember } from '$lib/api-client/memberDirectory';
import type { UserAvatarUserView } from '$lib/render/users';
import type { ServerProjectionStore } from './projection.svelte';
import { SvelteSet } from 'svelte/reactivity';

type ProjectionReadiness = {
  hasUsableProjection: boolean;
};

type NotificationCountState = {
  roomUnreadCounts: Record<string, number>;
  roomImportantUnreadCounts: Record<string, number>;
};

export type RoomsListItem = {
  id: string;
  name: string;
  description?: string | null;
  type: RoomKind;
  isUniversal: boolean;
  viewerIsMember: boolean;
  viewerCanReadMessages?: boolean | null;
  viewerCanJoinRoom: boolean;
  viewerCanManageRoom: boolean;
  viewerNotificationCount: number;
  viewerImportantNotificationCount: number;
  hasMessageHistory?: boolean | null;
  members: UserAvatarUserView[];
};

export function isNavigationVisibleRoom(room: RoomsListItem): boolean {
  return room.type !== RoomKind.DM || room.hasMessageHistory !== false;
}

export type RoomsListGroup = {
  id: string;
  name: string;
  viewerCanCreateRoom?: boolean;
  viewerCanManageGroup: boolean;
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

export function avatarUserFromDirectoryMember(
  member: ReturnType<typeof mapDirectoryMember>
): UserAvatarUserView {
  return {
    id: member.id,
    login: member.login,
    displayName: member.displayName,
    deleted: member.deleted,
    isBot: member.isBot,
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

/**
 * Read-only navigation view over the canonical realtime server projection.
 *
 * The view owns no server-derived room, membership, group, profile, ordering,
 * or notification state. Getters translate the current protobuf projection
 * and the owning notification store at the presentation boundary.
 */
export class NavigationStore {
  readonly #rooms = $derived.by((): RoomsListItem[] => {
    if (!this.readiness.hasUsableProjection) return [];
    return [...this.projection.rooms.values()].flatMap((entry) => {
      const room = entry.room ? mapDirectoryRoom(entry.room) : null;
      if (!room || room.archived) return [];
      const members = entry.memberUserIds.flatMap((userId) => {
        const member = this.projection.users.get(userId);
        return member ? [avatarUserFromDirectoryMember(mapDirectoryMember(member))] : [];
      });
      const viewerNotificationCount = this.notificationCounts.roomUnreadCounts[room.id] ?? 0;
      const viewerImportantNotificationCount =
        this.notificationCounts.roomImportantUnreadCounts[room.id] ?? 0;
      return [
        {
          id: room.id,
          name: room.name,
          description: room.description,
          type: roomKindOrChannel(room.kind),
          isUniversal: room.isUniversal,
          viewerIsMember: room.isMember,
          viewerCanReadMessages: room.canReadMessages,
          viewerCanJoinRoom: room.canJoinRoom,
          viewerCanManageRoom: room.canManageRoom,
          viewerNotificationCount,
          viewerImportantNotificationCount,
          hasMessageHistory: room.kind === RoomKind.DM ? (entry.hasMessageHistory ?? null) : null,
          members
        }
      ];
    });
  });

  readonly #roomGroups = $derived.by((): RoomsListGroup[] => {
    if (!this.readiness.hasUsableProjection) return [];
    return this.projection.roomGroups.map((group) => {
      const mapped = mapRoomGroup(group);
      return {
        id: mapped.id,
        name: mapped.name,
        viewerCanCreateRoom: mapped.canCreateRoom,
        viewerCanManageGroup: mapped.canManageGroup,
        roomIds: mapped.roomIds,
        items: mapped.items.map((item) =>
          item.type === 'room'
            ? { id: item.id, type: 'room' as const, roomId: item.roomId }
            : { id: item.id, type: 'link' as const, link: item.link }
        )
      };
    });
  });

  readonly #memberRoomIds = $derived.by(
    () => new SvelteSet(this.#rooms.filter((room) => room.viewerIsMember).map((room) => room.id))
  );

  constructor(
    private readonly projection: ServerProjectionStore,
    private readonly readiness: ProjectionReadiness,
    private readonly notificationCounts: NotificationCountState
  ) {}

  get rooms(): RoomsListItem[] {
    return this.#rooms;
  }

  get roomGroups(): RoomsListGroup[] {
    return this.#roomGroups;
  }

  get currentUserId(): string | null {
    if (!this.readiness.hasUsableProjection) return null;
    return this.projection.viewer?.user?.profile?.id ?? null;
  }

  get isInitialLoading(): boolean {
    return !this.readiness.hasUsableProjection;
  }

  isRoomMember(roomId: string): boolean {
    return this.#memberRoomIds.has(roomId);
  }
}
