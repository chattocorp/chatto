import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';

type SettingsQueryConnection = Pick<ServerConnection, 'queryScope'>;

function settingsRoot(serverId: string, connection: SettingsQueryConnection) {
  return ['server', serverId, 'session', connection.queryScope, 'settings'] as const;
}

export const settingsQueryKeys = {
  root: settingsRoot,
  externalIdentities(serverId: string, connection: SettingsQueryConnection) {
    return [...settingsRoot(serverId, connection), 'external-identities'] as const;
  },
  bots(serverId: string, connection: SettingsQueryConnection) {
    return [...settingsRoot(serverId, connection), 'bots'] as const;
  },
  botPermissions(serverId: string, connection: SettingsQueryConnection, botUserId: string) {
    return [...settingsRoot(serverId, connection), 'bots', botUserId, 'permissions'] as const;
  }
};
