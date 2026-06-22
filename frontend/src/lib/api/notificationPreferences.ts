import { Code, ConnectError, createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { NotificationPreferencesService } from '$lib/pb/chatto/api/v1/notification_preferences_connect';
import { NotificationLevel as ApiNotificationLevel } from '$lib/pb/chatto/api/v1/notification_preferences_pb';
import { NotificationLevel } from '$lib/gql/graphql';

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
      level: notificationLevelToAPI(level)
    },
    {
      headers: config.bearerToken ? { Authorization: `Bearer ${config.bearerToken}` } : undefined
    }
  );
  return {
    level: notificationLevelFromAPI(response.level),
    effectiveLevel: notificationLevelFromAPI(response.effectiveLevel)
  };
}

export function shouldFallbackToGraphQL(err: unknown): boolean {
  if (!(err instanceof ConnectError)) {
    return true;
  }
  return ![
    Code.Unauthenticated,
    Code.PermissionDenied,
    Code.InvalidArgument,
    Code.FailedPrecondition
  ].includes(err.code);
}

function notificationLevelToAPI(level: NotificationLevel): ApiNotificationLevel {
  switch (level) {
    case NotificationLevel.Muted:
      return ApiNotificationLevel.MUTED;
    case NotificationLevel.Normal:
      return ApiNotificationLevel.NORMAL;
    case NotificationLevel.AllMessages:
      return ApiNotificationLevel.ALL_MESSAGES;
    case NotificationLevel.Default:
    default:
      return ApiNotificationLevel.DEFAULT;
  }
}

function notificationLevelFromAPI(level: ApiNotificationLevel): NotificationLevel {
  switch (level) {
    case ApiNotificationLevel.MUTED:
      return NotificationLevel.Muted;
    case ApiNotificationLevel.NORMAL:
      return NotificationLevel.Normal;
    case ApiNotificationLevel.ALL_MESSAGES:
      return NotificationLevel.AllMessages;
    case ApiNotificationLevel.DEFAULT:
    case ApiNotificationLevel.UNSPECIFIED:
    default:
      return NotificationLevel.Default;
  }
}
