import { beforeEach, describe, expect, it, vi } from 'vitest';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';

const mocks = vi.hoisted(() => ({ migrate: vi.fn() }));

vi.mock('./userPreferences.svelte', () => ({
  userPreferences: {
    migrateLegacyServerTimePreferences: mocks.migrate
  }
}));

import { migrateLegacyServerTimePreferences } from './legacyServerTimePreferences';

describe('migrateLegacyServerTimePreferences', () => {
  beforeEach(() => {
    mocks.migrate.mockReset();
  });

  it.each([
    [TimeFormat.TIME_FORMAT_UNSPECIFIED, 'auto'],
    [TimeFormat.TIME_FORMAT_AUTO, 'auto'],
    [TimeFormat.TIME_FORMAT_12_HOUR, '12h'],
    [TimeFormat.TIME_FORMAT_24_HOUR, '24h']
  ] as const)('maps legacy time format %s to %s', (timeFormat, expected) => {
    mocks.migrate.mockReturnValue(true);

    expect(
      migrateLegacyServerTimePreferences({
        timezone: 'Europe/Berlin',
        timeFormat
      })
    ).toBe(true);
    expect(mocks.migrate).toHaveBeenCalledWith({
      timezone: 'Europe/Berlin',
      timeFormat: expected
    });
  });

  it('ignores a missing legacy preference', () => {
    expect(migrateLegacyServerTimePreferences(null)).toBe(false);
    expect(mocks.migrate).not.toHaveBeenCalled();
  });
});
