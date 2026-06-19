import { NotificationLevel as WireNotificationLevel } from '$lib/pb/chatto/core/v1/user_preferences_pb';
import {
  NotificationLevel,
  notificationLevelFromWire as fromWire,
  notificationLevelToWire as toWire
} from '$lib/preferences/notificationLevel';

export function notificationLevelFromWire(level: WireNotificationLevel): NotificationLevel {
  return fromWire(level);
}

export function notificationLevelToWire(level: NotificationLevel): WireNotificationLevel {
  return toWire(level);
}
