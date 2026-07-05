import {
  authHeaders,
  createChattoClient,
  handleAuthError,
  type ConnectAPIConfig as BaseConnectAPIConfig
} from './connect.js';
import { NotificationPreferencesService } from '@chatto/api-types/api/v1/notification_preferences_connect';
import { NotificationLevel } from '@chatto/api-types/api/v1/notification_preferences_pb';

export type ConnectAPIConfig = BaseConnectAPIConfig & {
  serverId: string;
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
    level: response.preference?.level ?? NotificationLevel.UNSPECIFIED,
    effectiveLevel: response.preference?.effectiveLevel ?? NotificationLevel.UNSPECIFIED
  };
}

export async function updateServerNotificationPreference(
  config: ConnectAPIConfig,
  level: NotificationLevel
): Promise<NotificationPreference> {
  const client = createNotificationPreferencesClient(config);
  let response;
  try {
    response = await client.updateServerNotificationPreference(
      { level },
      {
        headers: connectHeaders(config)
      }
    );
  } catch (err) {
    handleAuthError(config, err);
  }
  return {
    level: response.preference?.level ?? NotificationLevel.UNSPECIFIED,
    effectiveLevel: response.preference?.effectiveLevel ?? NotificationLevel.UNSPECIFIED
  };
}

export async function updateRoomNotificationPreference(
  config: ConnectAPIConfig,
  roomId: string,
  level: NotificationLevel
): Promise<NotificationPreference> {
  const client = createNotificationPreferencesClient(config);
  let response;
  try {
    response = await client.updateRoomNotificationPreference(
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
    level: response.preference?.level ?? NotificationLevel.UNSPECIFIED,
    effectiveLevel: response.preference?.effectiveLevel ?? NotificationLevel.UNSPECIFIED
  };
}

function createNotificationPreferencesClient(config: ConnectAPIConfig) {
  return createChattoClient(NotificationPreferencesService, config);
}

function connectHeaders(config: ConnectAPIConfig) {
  return authHeaders(config);
}
