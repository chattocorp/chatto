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
    return this.#roomList.rooms.map((room) => ({
      ...room,
      viewerNotificationCount: roomUnreadCounts[room.id] ?? 0,
      viewerImportantNotificationCount: roomImportantUnreadCounts[room.id] ?? 0
    }));
  });

  constructor(roomList: RoomListView, counts: () => AttentionCounts) {
    this.#roomList = roomList;
    this.#counts = counts;
  }

  get rooms(): RoomsListItem[] {
    return this.#rooms.get();
  }

  get roomGroups(): RoomsListGroup[] {
    return this.#roomList.roomGroups;
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
