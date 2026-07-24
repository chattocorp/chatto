import { ImageFitMode } from '@chatto/api-types/api/v1/common_pb';
import { NotificationLevel } from '@chatto/api-types/api/v1/notification_preferences_pb';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';

/** Preserve the frontend's default when a notification level is absent or unknown. */
export function notificationLevelOrDefault(
  level: NotificationLevel | undefined
): NotificationLevel {
  switch (level) {
    case NotificationLevel.MUTED:
    case NotificationLevel.NORMAL:
    case NotificationLevel.ALL_MESSAGES:
    case NotificationLevel.DEFAULT:
      return level;
    case NotificationLevel.UNSPECIFIED:
    default:
      return NotificationLevel.DEFAULT;
  }
}

/** Treat absent or unknown presence values as offline instead of implying availability. */
export function presenceStatusOrOffline(status: PresenceStatus): PresenceStatus {
  switch (status) {
    case PresenceStatus.AWAY:
    case PresenceStatus.DO_NOT_DISTURB:
    case PresenceStatus.ONLINE:
    case PresenceStatus.OFFLINE:
      return status;
    case PresenceStatus.UNSPECIFIED:
    default:
      return PresenceStatus.OFFLINE;
  }
}

/** Keep unspecified or unknown room kinds on the non-DM path. */
export function roomKindOrChannel(kind: RoomKind): RoomKind {
  return kind === RoomKind.DM ? RoomKind.DM : RoomKind.CHANNEL;
}

/** Preserve the historical cover fallback for unsupported image-fit values. */
export function imageFitModeOrCover(fit: ImageFitMode): ImageFitMode {
  return fit === ImageFitMode.CONTAIN ? ImageFitMode.CONTAIN : ImageFitMode.COVER;
}
