import { NotificationLevel as WireNotificationLevel } from '$lib/pb/chatto/core/v1/user_preferences_pb';

export enum NotificationLevel {
  Default = 'DEFAULT',
  Muted = 'MUTED',
  Normal = 'NORMAL',
  AllMessages = 'ALL_MESSAGES'
}

export function notificationLevelFromWire(
  level: WireNotificationLevel | undefined | null
): NotificationLevel {
  switch (level) {
    case WireNotificationLevel.MUTED:
      return NotificationLevel.Muted;
    case WireNotificationLevel.ALL_MESSAGES:
      return NotificationLevel.AllMessages;
    case WireNotificationLevel.NORMAL:
      return NotificationLevel.Normal;
    case WireNotificationLevel.UNSPECIFIED:
    default:
      return NotificationLevel.Default;
  }
}

export function notificationLevelToWire(level: NotificationLevel): WireNotificationLevel {
  switch (level) {
    case NotificationLevel.Muted:
      return WireNotificationLevel.MUTED;
    case NotificationLevel.AllMessages:
      return WireNotificationLevel.ALL_MESSAGES;
    case NotificationLevel.Normal:
      return WireNotificationLevel.NORMAL;
    case NotificationLevel.Default:
    default:
      return WireNotificationLevel.UNSPECIFIED;
  }
}
