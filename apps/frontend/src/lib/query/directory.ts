import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';

type DirectoryQueryConnection = Pick<ServerConnection, 'queryScope'>;

function directoryRoot(serverId: string, connection: DirectoryQueryConnection) {
  return ['server', serverId, 'session', connection.queryScope, 'directory'] as const;
}

export const directoryQueryKeys = {
  root: directoryRoot,
  users(serverId: string, connection: DirectoryQueryConnection, search: string, limit: number) {
    return [...directoryRoot(serverId, connection), 'users', { search, limit }] as const;
  }
};
