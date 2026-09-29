import { describe, expect, it } from 'vitest';
import { buildRoomDetailsPatch, type RoomDetailsValues } from './roomSettings';

const original: RoomDetailsValues = { name: 'general', description: 'General discussion' };

describe('buildRoomDetailsPatch', () => {
  it('omits the description from a name-only update', () => {
    expect(buildRoomDetailsPatch({ ...original, name: 'announcements' }, original)).toEqual({
      name: 'announcements'
    });
  });

  it('clears an emptied description and omits the unchanged name', () => {
    expect(buildRoomDetailsPatch({ ...original, description: '   ' }, original)).toEqual({
      description: null
    });
  });

  it('normalizes the name before comparing it', () => {
    expect(buildRoomDetailsPatch({ ...original, name: 'Küche' }, original)).toEqual({
      name: 'Küche'
    });
    expect(buildRoomDetailsPatch({ ...original }, original)).toEqual({});
  });
});
