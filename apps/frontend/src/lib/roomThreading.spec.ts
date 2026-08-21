import { describe, expect, it } from 'vitest';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { normalizeRoomThreadingMode, RoomThreadingMode } from './roomThreading';

describe('normalizeRoomThreadingMode', () => {
  it('preserves every explicit channel mode', () => {
    for (const mode of [
      RoomThreadingMode.REQUIRED,
      RoomThreadingMode.ENCOURAGED,
      RoomThreadingMode.ENABLED,
      RoomThreadingMode.DISABLED
    ]) {
      expect(normalizeRoomThreadingMode(RoomKind.CHANNEL, mode)).toBe(mode);
    }
  });

  it('maps omitted or unknown historical channel values to Enabled', () => {
    expect(normalizeRoomThreadingMode(RoomKind.CHANNEL, RoomThreadingMode.UNSPECIFIED)).toBe(
      RoomThreadingMode.ENABLED
    );
    expect(normalizeRoomThreadingMode(RoomKind.CHANNEL, 99 as RoomThreadingMode)).toBe(
      RoomThreadingMode.ENABLED
    );
  });

  it('keeps direct messages threadless regardless of the received value', () => {
    expect(normalizeRoomThreadingMode(RoomKind.DM, RoomThreadingMode.REQUIRED)).toBe(
      RoomThreadingMode.UNSPECIFIED
    );
  });
});
