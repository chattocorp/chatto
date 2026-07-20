import { describe, expect, it } from 'vitest';
import { buildRoomGroupSettingsUpdate } from './roomGroupSettings';

describe('buildRoomGroupSettingsUpdate', () => {
  it('omits an unchanged description when only the name changes', () => {
    expect(
      buildRoomGroupSettingsUpdate(
        'g1',
        { name: 'Projects', description: 'Important rooms' },
        { name: 'Project', description: 'Important rooms' }
      )
    ).toEqual({ groupId: 'g1', name: 'Projects' });
  });

  it('omits an unchanged name when only the description changes', () => {
    expect(
      buildRoomGroupSettingsUpdate(
        'g1',
        { name: 'Project', description: 'Updated' },
        { name: 'Project', description: 'Original' }
      )
    ).toEqual({ groupId: 'g1', description: 'Updated' });
  });

  it('uses null to clear the description', () => {
    expect(
      buildRoomGroupSettingsUpdate(
        'g1',
        { name: 'Project', description: '   ' },
        { name: 'Project', description: 'Original' }
      )
    ).toEqual({ groupId: 'g1', description: null });
  });
});
