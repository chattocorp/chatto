import { describe, expect, it } from 'vitest';
import { buildRoomSettingsUpdate, type RoomSettingsValues } from './roomSettings';

const original: RoomSettingsValues = {
  name: 'general',
  description: 'General discussion',
  universal: false,
  slowModeSeconds: 0
};

describe('buildRoomSettingsUpdate', () => {
  it('omits unchanged metadata from a Universal-only update', () => {
    expect(buildRoomSettingsUpdate('room-1', { ...original, universal: true }, original)).toEqual({
      roomId: 'room-1',
      universal: true
    });
  });

  it('omits Universal from a metadata-only update', () => {
    expect(
      buildRoomSettingsUpdate(
        'room-1',
        { ...original, name: 'announcements', description: '' },
        original
      )
    ).toEqual({
      roomId: 'room-1',
      name: 'announcements',
      description: null
    });
  });

  it('includes Slow Mode only when its value changes', () => {
    expect(
      buildRoomSettingsUpdate('room-1', { ...original, slowModeSeconds: 30 }, original)
    ).toEqual({ roomId: 'room-1', slowModeSeconds: 30 });
  });
});
