import { Code, ConnectError, createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { NotificationPreferencesService } from '@chatto/api-types/api/v1/notification_preferences_connect';
import { NotificationLevel } from '@chatto/api-types/api/v1/notification_preferences_pb';
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

export async function getServerNotificationPreference(
  config: ConnectAPIConfig
): Promise<NotificationPreference> {
  const client = createNotificationPreferencesClient(config);
  let response;
  try {
    response = await client.getServerNotificationPreference(
      {},
      {
        headers: connectHeaders(config)
      }
    );
  } catch (err) {
    handleAuthError(config, err);
  }
  return {
    level: response.level,
    effectiveLevel: response.effectiveLevel
  };
}

export async function setServerNotificationLevel(
  config: ConnectAPIConfig,
  level: NotificationLevel
): Promise<NotificationPreference> {
  const client = createNotificationPreferencesClient(config);
  let response;
  try {
    response = await client.setServerNotificationLevel(
      { level },
      {
        headers: connectHeaders(config)
      }
    );
  } catch (err) {
    handleAuthError(config, err);
  }
  return {
    level: response.level,
    effectiveLevel: response.effectiveLevel
  };
}

export async function setRoomNotificationLevel(
  config: ConnectAPIConfig,
  roomId: string,
  level: NotificationLevel
): Promise<NotificationPreference> {
  const client = createNotificationPreferencesClient(config);
  let response;
  try {
    response = await client.setRoomNotificationLevel(
      {
        roomId,
        level
      },
      {
        headers: connectHeaders(config)
      }
    );
  } catch (err) {
    handleAuthError(config, err);
  }
  return {
    level: response.level,
    effectiveLevel: response.effectiveLevel
  };
}

function createNotificationPreferencesClient(config: ConnectAPIConfig) {
  const transport = createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: true
  });
  return createClient(NotificationPreferencesService, transport);
}

function connectHeaders(config: ConnectAPIConfig) {
  return config.bearerToken ? { Authorization: `Bearer ${config.bearerToken}` } : undefined;
}

function handleAuthError(config: ConnectAPIConfig, err: unknown): never {
  if (err instanceof ConnectError && err.code === Code.Unauthenticated) {
    serverRegistry.handleAuthenticationRequired(config.serverId);
  }
  throw err;
}
