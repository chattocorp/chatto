/**
 * The room navigation of one server: the client's room list with the
 * notification counts that the sidebar shows.
 */

import { computed } from '@chatto/client/reactivity';
import { RoomKind } from '@chatto/client/api/roomDirectory';
import type {
  RoomListGroup,
  RoomListGroupItem,
  RoomListItem,
  RoomListView,
  SidebarLinkListItem
} from '@chatto/client/server/rooms';
import type { AttentionCounts } from './notificationAttention';

/** A room of the sidebar, with its notification counts. */
export type RoomsListItem = RoomListItem & {
  /** Occurrences that need attention; see `NotificationAttention`. */
  viewerNotificationCount: number;
  /** Important occurrences that need attention. */
  viewerImportantNotificationCount: number;
};

export type RoomsListGroup = RoomListGroup;
export type RoomsListGroupItem = RoomListGroupItem;
export type { SidebarLinkListItem };

/** Whether the sidebar shows a room: direct messages appear once they have messages. */
export function isNavigationVisibleRoom(room: RoomListItem): boolean {
  return room.type !== RoomKind.DM || room.hasMessageHistory !== false;
}

/** Read-only navigation over the client's room list; see the module documentation. */
export class NavigationStore {
  readonly #roomList: RoomListView;
  readonly #counts: () => AttentionCounts;

  readonly #rooms = computed((): RoomsListItem[] => {
    const { roomUnreadCounts, roomImportantUnreadCounts } = this.#counts();
    return this.#roomList.rooms
      .filter((room) => room.type === RoomKind.DM || room.viewerCanListRoom !== false)
      .map((room) => ({
        ...room,
        viewerNotificationCount: roomUnreadCounts[room.id] ?? 0,
        viewerImportantNotificationCount: roomImportantUnreadCounts[room.id] ?? 0
      }));
  });

  // Management metadata stays available in the client. The chat sidebar hides
  // entries for which the server explicitly denies ordinary navigation.
  readonly #roomGroups = computed((): RoomsListGroup[] =>
    this.#roomList.roomGroups.map((group) => {
      const hidden = new Set(
        group.items?.flatMap((item) =>
          item.type === 'room' && item.viewerCanListRoom === false ? [item.roomId] : []
        )
      );
      return {
        ...group,
        roomIds: group.roomIds.filter((id) => !hidden.has(id)),
        items: group.items?.filter((item) => item.type !== 'room' || !hidden.has(item.roomId))
      };
    })
  );

  constructor(roomList: RoomListView, counts: () => AttentionCounts) {
    this.#roomList = roomList;
    this.#counts = counts;
  }

  get rooms(): RoomsListItem[] {
    return this.#rooms.get();
  }

  get roomGroups(): RoomsListGroup[] {
    return this.#roomGroups.get();
  }

  get currentUserId(): string | null {
    return this.#roomList.viewerId;
  }

  get isInitialLoading(): boolean {
    return this.#roomList.isLoading;
  }

  isRoomMember(roomId: string): boolean {
    return this.#roomList.isMember(roomId);
  }
}
