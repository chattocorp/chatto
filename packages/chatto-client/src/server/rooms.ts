import { computed } from '../reactivity/index.js';
import { RoomKind } from '../api/roomDirectory.js';
import { roomKindOrChannel } from '../api/enumDefaults.js';
import { mapDirectoryRoom, mapRoomGroup } from '../api/roomDirectory.js';
import { deletedDirectMessageParticipant, type UserAvatarUserView } from '../timeline/users.js';
import type { ServerProjectionStore } from './projection.js';

type ProjectionReadiness = {
  hasUsableProjection: boolean;
  isRecoveringSnapshot?: boolean;
};

/** A room of the projection with the viewer's membership and permissions. */
export type RoomListItem = {
  id: string;
  name: string;
  description?: string | null;
  type: RoomKind;
  isUniversal: boolean;
  /** Displayed membership. The connection verifies the session before allowing commands. */
  viewerIsMember: boolean | null;
  viewerCanReadMessages?: boolean | null;
  viewerCanJoinRoom: boolean;
  viewerCanManageRoom: boolean;
  /** For a direct message: whether it has messages. Null for other rooms. */
  hasMessageHistory?: boolean | null;
  /** DM participants, with a deleted placeholder for each deleted account. */
  members: UserAvatarUserView[];
};

/** A room group of the projection, with the viewer's permissions in it. */
export type RoomListGroup = {
  id: string;
  name: string;
  viewerCanCreateRoom?: boolean;
  viewerCanManageGroup: boolean;
  roomIds: string[];
  items?: RoomListGroupItem[];
};

export type SidebarLinkListItem = {
  id: string;
  label: string;
  url: string;
};

export type RoomListGroupItem =
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

/**
 * Resolves one DM participant for presentation. Deleted accounts stay DM
 * participants and resolve to a deleted placeholder; unresolved profiles are
 * omitted until they load.
 */
export function directMessageParticipant(
  projection: ServerProjectionStore,
  userId: string
): UserAvatarUserView[] {
  const view = projection.users.view(userId);
  if (view) return [view];
  return projection.users.isDeleted(userId) ? [deletedDirectMessageParticipant(userId)] : [];
}

/**
 * The rooms and room groups of the retained projection, as plain values.
 *
 * The view owns no state: its getters translate the current protobuf
 * projection. It is empty until the projection is readable.
 */
export class RoomListView {
  get #readable(): boolean {
    return (
      this.readiness.hasUsableProjection ||
      (this.readiness.isRecoveringSnapshot === true &&
        (this.projection.viewer !== null || this.projection.rooms.size > 0))
    );
  }

  readonly #roomsComputed = computed((): RoomListItem[] => {
    if (!this.#readable) return [];
    const live = [...this.projection.rooms.values()].flatMap((entry) => {
      const room = entry.room ? mapDirectoryRoom(entry) : null;
      if (!room || room.archived) return [];
      const members = entry.memberUserIds.flatMap((userId) =>
        directMessageParticipant(this.projection, userId)
      );
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
          hasMessageHistory: room.kind === RoomKind.DM ? (entry.hasMessageHistory ?? null) : null,
          members
        }
      ];
    });
    return live;
  });
  get #rooms() {
    return this.#roomsComputed.get();
  }

  readonly #roomGroupsComputed = computed((): RoomListGroup[] => {
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
  get #roomGroups() {
    return this.#roomGroupsComputed.get();
  }

  readonly #memberRoomIdsComputed = computed(
    () => new Set(this.#rooms.filter((room) => room.viewerIsMember).map((room) => room.id))
  );
  get #memberRoomIds() {
    return this.#memberRoomIdsComputed.get();
  }

  constructor(
    private readonly projection: ServerProjectionStore,
    private readonly readiness: ProjectionReadiness
  ) {}

  /** Rooms that are not archived. Reactive. */
  get rooms(): RoomListItem[] {
    return this.#rooms;
  }

  /** Room groups in their configured order. Reactive. */
  get roomGroups(): RoomListGroup[] {
    return this.#roomGroups;
  }

  /** The viewer of the projection, or null until it is readable. Reactive. */
  get viewerId(): string | null {
    if (!this.#readable) return null;
    return this.projection.viewer?.user?.profile?.id ?? null;
  }

  /** Whether the projection is not readable yet. Reactive. */
  get isLoading(): boolean {
    return !this.#readable;
  }

  /** Whether the viewer is a member of the room. Reactive. */
  isMember(roomId: string): boolean {
    return this.#memberRoomIds.has(roomId);
  }
}
