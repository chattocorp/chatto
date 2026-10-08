/**
 * Cross-server summaries for the Home page: the viewer's direct messages and
 * the calls in progress on every signed-in server.
 *
 * The functions are pure. The Home components read each server's stores into a
 * `HomeServerSource` and pass the sources here.
 */

import { RoomKind } from '@chatto/client/api/roomDirectory';
import type { RoomsListItem } from '$lib/state/server/navigation';
import { isNavigationVisibleRoom } from '$lib/state/server/navigation';
import type { CallRoomParticipant } from '$lib/state/server/activeCallRooms';

/** The data of one signed-in server that the Home page needs. */
export type HomeServerSource = {
  serverId: string;
  serverName: string;
  /** The viewer on this server, which a direct message name leaves out. */
  viewerId: string | null;
  rooms: readonly RoomsListItem[];
  /** Whether the first room list of this server is still loading. */
  roomsLoading: boolean;
  /** Whether the room has unread messages, including optimistic local reads. */
  isUnread: (roomId: string) => boolean;
  /** Rooms with a call in progress. */
  callRoomIds: readonly string[];
  callParticipants: (roomId: string) => CallRoomParticipant[];
};

/** One direct message on the Home page. */
export type HomeDirectMessage = {
  serverId: string;
  serverName: string;
  viewerId: string | null;
  room: RoomsListItem;
  unread: boolean;
  hasActiveCall: boolean;
};

/** One call in progress on the Home page. */
export type HomeLiveCall = {
  serverId: string;
  serverName: string;
  viewerId: string | null;
  room: RoomsListItem;
  participants: CallRoomParticipant[];
};

/**
 * Return the visible direct messages of all servers. Direct messages that need
 * attention come first: Important notifications, then other notifications, then
 * unread messages. The sort is stable, so each server keeps its room order.
 */
export function homeDirectMessages(sources: readonly HomeServerSource[]): HomeDirectMessage[] {
  const entries = sources.flatMap((source) => {
    const callRooms = new Set(source.callRoomIds);
    return source.rooms
      .filter((room) => room.type === RoomKind.DM && isNavigationVisibleRoom(room))
      .map((room): HomeDirectMessage => ({
        serverId: source.serverId,
        serverName: source.serverName,
        viewerId: source.viewerId,
        room,
        unread: source.isUnread(room.id),
        hasActiveCall: callRooms.has(room.id)
      }));
  });
  return entries.sort(
    (a, b) =>
      b.room.viewerImportantNotificationCount - a.room.viewerImportantNotificationCount ||
      b.room.viewerNotificationCount - a.room.viewerNotificationCount ||
      Number(b.unread) - Number(a.unread)
  );
}

/**
 * Return the calls in progress that the viewer can see in the sidebar: calls
 * in direct messages and in rooms that the viewer is a member of.
 */
export function homeLiveCalls(sources: readonly HomeServerSource[]): HomeLiveCall[] {
  return sources.flatMap((source) => {
    const rooms = new Map(source.rooms.map((room) => [room.id, room]));
    return source.callRoomIds.flatMap((roomId): HomeLiveCall[] => {
      const room = rooms.get(roomId);
      if (!room || (room.type !== RoomKind.DM && !room.viewerIsMember)) return [];
      return [
        {
          serverId: source.serverId,
          serverName: source.serverName,
          viewerId: source.viewerId,
          room,
          participants: source.callParticipants(roomId)
        }
      ];
    });
  });
}

/**
 * Whether the direct-message list is still loading: no server can list its
 * rooms yet. One unreachable server must not keep the list loading.
 */
export function homeDirectMessagesLoading(sources: readonly HomeServerSource[]): boolean {
  return sources.length > 0 && sources.every((source) => source.roomsLoading);
}
