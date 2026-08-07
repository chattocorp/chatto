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
  notificationPreferences(serverId: string, connection: SettingsQueryConnection) {
    return [...settingsRoot(serverId, connection), 'notification-preferences'] as const;
  }
};
