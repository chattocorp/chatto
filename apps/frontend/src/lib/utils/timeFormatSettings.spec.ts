import { describe, expect, it } from 'vitest';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { hour12ForTimeFormat, timeFormatSettingsFor } from './formatTime';

describe('hour12ForTimeFormat', () => {
  it('maps explicit clock formats and leaves automatic formats to the locale', () => {
    expect(hour12ForTimeFormat(TimeFormat.TIME_FORMAT_12_HOUR)).toBe(true);
    expect(hour12ForTimeFormat(TimeFormat.TIME_FORMAT_24_HOUR)).toBe(false);
    expect(hour12ForTimeFormat(TimeFormat.TIME_FORMAT_AUTO)).toBeUndefined();
  });
});

describe('timeFormatSettingsFor', () => {
  it('maps canonical viewer settings into display formatting options', () => {
    expect(
      timeFormatSettingsFor({
        timezone: 'Europe/Berlin',
        timeFormat: TimeFormat.TIME_FORMAT_24_HOUR
      })
    ).toEqual({
      effectiveTimezone: 'Europe/Berlin',
      effectiveHour12: false
    });
  });

  it('uses browser and locale defaults when viewer settings are absent', () => {
    expect(timeFormatSettingsFor(null)).toEqual({
      effectiveTimezone: undefined,
      effectiveHour12: undefined
    });
  });
});
