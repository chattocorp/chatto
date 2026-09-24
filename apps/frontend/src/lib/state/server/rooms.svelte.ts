import { RoomKind } from '$lib/api-client/roomDirectory';
import { roomKindOrChannel } from '$lib/api-client/enumDefaults';
import { mapDirectoryRoom, mapRoomGroup } from '$lib/api-client/roomDirectory';
import { mapDirectoryMember } from '$lib/api-client/memberDirectory';
import type { UserAvatarUserView } from '$lib/render/users';
import type { ServerProjectionStore } from './projection.svelte';
import { SvelteSet } from 'svelte/reactivity';
import type { SavedRoom } from '$lib/storage/savedViews';

type ProjectionReadiness = {
  hasUsableProjection: boolean;
  isRecoveringSnapshot?: boolean;
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
  /** Null until live membership is known; saved labels do not grant membership. */
  viewerIsMember: boolean | null;
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
 * Read-only navigation over live rooms with saved labels as a display fallback.
 *
 * The view owns no server-derived room, membership, group, profile, ordering,
 * or notification state. Getters translate the current protobuf projection
 * and the owning notification store at the presentation boundary.
 */
export class NavigationStore {
  get #readable(): boolean {
    return (
      this.readiness.hasUsableProjection ||
      (this.readiness.isRecoveringSnapshot === true &&
        (this.projection.viewer !== null || this.getSavedRooms().length > 0))
    );
  }

  readonly #rooms = $derived.by((): RoomsListItem[] => {
    if (!this.#readable) return [];
    const live = [...this.projection.rooms.values()].flatMap((entry) => {
      const room = entry.room ? mapDirectoryRoom(entry) : null;
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
    return [
      ...live,
      ...this.getSavedRooms()
        .filter((room) => !this.projection.rooms.has(room.id))
        .map((room): RoomsListItem => ({
          id: room.id,
          name: room.name,
          type: roomKindOrChannel(room.kind ?? RoomKind.CHANNEL),
          isUniversal: room.universal ?? false,
          viewerIsMember: null,
          viewerCanReadMessages: null,
          viewerCanJoinRoom: false,
          viewerCanManageRoom: false,
          viewerNotificationCount: 0,
          viewerImportantNotificationCount: 0,
          members: []
        }))
    ];
  });

  readonly #roomGroups = $derived.by((): RoomsListGroup[] => {
    if (!this.#readable) return [];
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
    private readonly notificationCounts: NotificationCountState,
    private readonly getSavedRooms: () => readonly SavedRoom[] = () => []
  ) {}

  get rooms(): RoomsListItem[] {
    return this.#rooms;
  }

  get roomGroups(): RoomsListGroup[] {
    return this.#roomGroups;
  }

  get currentUserId(): string | null {
    if (!this.#readable) return null;
    return this.projection.viewer?.user?.profile?.id ?? null;
  }

  get isInitialLoading(): boolean {
    return !this.#readable;
  }

  isRoomMember(roomId: string): boolean {
    return this.#memberRoomIds.has(roomId);
  }
}
