import type { RoomsListItem } from '$lib/state/server/rooms.svelte';

export type RoomRouteAccess =
  | {
      kind: 'unknown' | 'member';
    }
  | {
      kind: 'nonmember';
      room: RoomsListItem;
    };

export interface RoomLinkAccessOptions {
  rooms: readonly RoomsListItem[];
  roomId: string;
}

export function roomRouteAccess(options: RoomLinkAccessOptions): RoomRouteAccess {
  const room = options.rooms.find((candidate) => candidate.id === options.roomId);

  if (!room) {
    return { kind: 'unknown' };
  }

  if (room.viewerIsMember) {
    return { kind: 'member' };
  }

  return { kind: 'nonmember', room };
}
