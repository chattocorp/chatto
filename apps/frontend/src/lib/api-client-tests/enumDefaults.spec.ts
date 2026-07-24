import { describe, expect, it } from 'vitest';
import { ImageFitMode } from '@chatto/api-types/api/v1/common_pb';
import { NotificationLevel } from '@chatto/api-types/api/v1/notification_preferences_pb';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import {
  imageFitModeOrCover,
  notificationLevelOrDefault,
  presenceStatusOrOffline,
  roomKindOrChannel
} from '$lib/api-client/enumDefaults';

describe('protobuf enum defaults', () => {
  it('preserves supported values', () => {
    expect(notificationLevelOrDefault(NotificationLevel.ALL_MESSAGES)).toBe(
      NotificationLevel.ALL_MESSAGES
    );
    expect(presenceStatusOrOffline(PresenceStatus.AWAY)).toBe(PresenceStatus.AWAY);
    expect(roomKindOrChannel(RoomKind.DM)).toBe(RoomKind.DM);
    expect(imageFitModeOrCover(ImageFitMode.CONTAIN)).toBe(ImageFitMode.CONTAIN);
  });

  it('uses safe defaults for unspecified values', () => {
    expect(notificationLevelOrDefault(NotificationLevel.UNSPECIFIED)).toBe(
      NotificationLevel.DEFAULT
    );
    expect(presenceStatusOrOffline(PresenceStatus.UNSPECIFIED)).toBe(PresenceStatus.OFFLINE);
    expect(roomKindOrChannel(RoomKind.UNSPECIFIED)).toBe(RoomKind.CHANNEL);
    expect(imageFitModeOrCover(ImageFitMode.UNSPECIFIED)).toBe(ImageFitMode.COVER);
  });

  it('uses safe defaults for unknown future values', () => {
    expect(notificationLevelOrDefault(99 as NotificationLevel)).toBe(NotificationLevel.DEFAULT);
    expect(presenceStatusOrOffline(99 as PresenceStatus)).toBe(PresenceStatus.OFFLINE);
    expect(roomKindOrChannel(99 as RoomKind)).toBe(RoomKind.CHANNEL);
    expect(imageFitModeOrCover(99 as ImageFitMode)).toBe(ImageFitMode.COVER);
  });
});
