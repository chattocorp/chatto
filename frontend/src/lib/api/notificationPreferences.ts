import { Code, ConnectError, createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { NotificationPreferencesService } from '$lib/pb/chatto/api/v1/notification_preferences_connect';
import { NotificationLevel } from '$lib/pb/chatto/api/v1/notification_preferences_pb';

export type ConnectAPIConfig = {
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
  const response = await client.setRoomNotificationLevel(
    {
      roomId,
      level
    },
    {
      headers: config.bearerToken ? { Authorization: `Bearer ${config.bearerToken}` } : undefined
    }
  );
  return {
    level: response.level,
    effectiveLevel: response.effectiveLevel
  };
}

export function shouldFallbackToGraphQL(err: unknown): boolean {
  if (!(err instanceof ConnectError)) {
    return true;
  }
  return err.code === Code.Unimplemented || err.code === Code.Unavailable;
}
