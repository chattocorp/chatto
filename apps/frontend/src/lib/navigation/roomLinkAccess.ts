import type { RoomsListItem } from '$lib/state/server/rooms.svelte';

export type RoomLinkAccessRedirect =
  | {
      kind: 'allow';
    }
  | {
      kind: 'redirect';
      path: string;
      state: App.PageState;
    };

export interface RoomLinkAccessOptions {
  rooms: readonly RoomsListItem[];
  roomId: string;
  targetPath: string;
  fallbackPath: string;
}

export function roomLinkAccessRedirect(options: RoomLinkAccessOptions): RoomLinkAccessRedirect {
  const room = options.rooms.find((candidate) => candidate.id === options.roomId);

  if (!room) {
    return { kind: 'allow' };
  }

  if (room.viewerIsMember) {
    return { kind: 'allow' };
  }

  return {
    kind: 'redirect',
    path: options.fallbackPath,
    state: {
      modal: {
        type: 'joinRoom',
        roomId: room.id,
        roomName: room.name,
        viewerCanJoinRoom: room.viewerCanJoinRoom,
        afterJoinPath: options.targetPath,
        closePath: options.fallbackPath
      }
    }
  };
}
