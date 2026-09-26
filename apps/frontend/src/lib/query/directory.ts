import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import { serverSessionQueryRoot } from './keys';

type DirectoryQueryConnection = Pick<ServerConnection, 'queryScope'>;

function directoryRoot(serverId: string, connection: DirectoryQueryConnection) {
  return [...serverSessionQueryRoot(serverId, connection), 'directory'] as const;
}

export const directoryQueryKeys = {
  root: directoryRoot,
  users(serverId: string, connection: DirectoryQueryConnection, search: string, limit: number) {
    return [...directoryRoot(serverId, connection), 'users', { search, limit }] as const;
  },
  room(serverId: string, connection: DirectoryQueryConnection, roomId: string) {
    return [...directoryRoot(serverId, connection), 'room', roomId] as const;
  },
  roomMembers(serverId: string, connection: DirectoryQueryConnection, roomId: string) {
    return [...directoryQueryKeys.room(serverId, connection, roomId), 'members'] as const;
  },
  eligibleRoomMembers(
    serverId: string,
    connection: DirectoryQueryConnection,
    roomId: string,
    search: string,
    limit: number
  ) {
    return [
      ...directoryQueryKeys.room(serverId, connection, roomId),
      'eligible-members',
      { search, limit }
    ] as const;
  }
};
