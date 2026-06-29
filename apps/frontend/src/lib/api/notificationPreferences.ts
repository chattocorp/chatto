export * from '@chatto/api-client/notificationPreferences';
import {
  getServerNotificationPreference as baseGetServerNotificationPreference,
  setRoomNotificationLevel as baseSetRoomNotificationLevel,
  setServerNotificationLevel as baseSetServerNotificationLevel
} from '@chatto/api-client/notificationPreferences';
import { withAuthenticationRequired } from './clientHooks';
import type { ConnectAPIConfig } from '@chatto/api-client/notificationPreferences';
import type { NotificationLevel } from '@chatto/api-types/api/v1/notification_preferences_pb';

export function getServerNotificationPreference(config: ConnectAPIConfig) {
  return baseGetServerNotificationPreference(withAuthenticationRequired(config));
}

export function setServerNotificationLevel(config: ConnectAPIConfig, level: NotificationLevel) {
  return baseSetServerNotificationLevel(withAuthenticationRequired(config), level);
}

export function setRoomNotificationLevel(
  config: ConnectAPIConfig,
  roomId: string,
  level: NotificationLevel
) {
  return baseSetRoomNotificationLevel(withAuthenticationRequired(config), roomId, level);
}
