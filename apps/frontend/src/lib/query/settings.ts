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
  botsRoot(serverId: string, connection: SettingsQueryConnection) {
    return [...settingsRoot(serverId, connection), 'bots'] as const;
  },
  bots(serverId: string, connection: SettingsQueryConnection, search: string) {
    return [...settingsRoot(serverId, connection), 'bots', 'list', search] as const;
  },
  bot(serverId: string, connection: SettingsQueryConnection, botUserId: string) {
    return [...settingsRoot(serverId, connection), 'bots', 'detail', botUserId] as const;
  }
};
