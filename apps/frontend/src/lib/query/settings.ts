import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';

type SettingsQueryConnection = Pick<ServerConnection, 'queryScope'>;

function settingsRoot(serverId: string, connection: SettingsQueryConnection) {
  return ['server', serverId, 'session', connection.queryScope, 'settings'] as const;
}

function accountSettingsRoot(
  serverId: string,
  connection: SettingsQueryConnection,
  userId: string
) {
  return [...settingsRoot(serverId, connection), 'account', userId] as const;
}

export const settingsQueryKeys = {
  root: settingsRoot,
  notificationPoliciesRoot(serverId: string, connection: SettingsQueryConnection) {
    return [...settingsRoot(serverId, connection), 'notification-policies'] as const;
  },
  notificationPolicies(
    serverId: string,
    connection: SettingsQueryConnection,
    scopes: readonly string[]
  ) {
    return [...settingsQueryKeys.notificationPoliciesRoot(serverId, connection), scopes] as const;
  },
  externalIdentities(serverId: string, connection: SettingsQueryConnection, userId: string) {
    return [...accountSettingsRoot(serverId, connection, userId), 'external-identities'] as const;
  },
  verifiedEmails(serverId: string, connection: SettingsQueryConnection, userId: string) {
    return [...accountSettingsRoot(serverId, connection, userId), 'verified-emails'] as const;
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
