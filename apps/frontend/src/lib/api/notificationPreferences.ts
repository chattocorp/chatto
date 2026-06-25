import { Code, ConnectError, createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { NotificationPreferencesService } from '$lib/pb/chatto/api/v1/notification_preferences_connect';
import { NotificationLevel } from '$lib/pb/chatto/api/v1/notification_preferences_pb';
import { serverRegistry } from '$lib/state/server/registry.svelte';

export type ConnectAPIConfig = {
  serverId: string;
  baseUrl: string;
  bearerToken: string | null;
};

export type NotificationPreference = {
  level: NotificationLevel;
  effectiveLevel: NotificationLevel;
};

export async function setRoomNotificationLevel(
  config: ConnectAPIConfig,
  roomId: string,
  level: NotificationLevel
): Promise<NotificationPreference> {
  const transport = createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: true
  });
  const client = createClient(NotificationPreferencesService, transport);
  let response;
  try {
    response = await client.setRoomNotificationLevel(
      {
        roomId,
        level
      },
      {
        headers: config.bearerToken ? { Authorization: `Bearer ${config.bearerToken}` } : undefined
      }
    );
  } catch (err) {
    if (err instanceof ConnectError && err.code === Code.Unauthenticated) {
      serverRegistry.handleAuthenticationRequired(config.serverId);
    }
    throw err;
  }
  return {
    level: response.level,
    effectiveLevel: response.effectiveLevel
  };
}
