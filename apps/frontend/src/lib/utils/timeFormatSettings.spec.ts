import { describe, expect, it } from 'vitest';
import { hour12ForTimeFormat, timeFormatSettingsFor } from './formatTime';

describe('hour12ForTimeFormat', () => {
  it('maps explicit clock formats and leaves automatic formats to the locale', () => {
    expect(hour12ForTimeFormat('12h')).toBe(true);
    expect(hour12ForTimeFormat('24h')).toBe(false);
    expect(hour12ForTimeFormat('auto')).toBeUndefined();
  });
});

describe('timeFormatSettingsFor', () => {
  it('maps canonical client preferences into display formatting options', () => {
    expect(
      timeFormatSettingsFor({
        timezone: 'Europe/Berlin',
        timeFormat: '24h'
      })
    ).toEqual({
      effectiveTimezone: 'Europe/Berlin',
      effectiveHour12: false
    });
  });

  it('uses browser and locale defaults when client preferences request them', () => {
    expect(timeFormatSettingsFor({ timezone: null, timeFormat: 'auto' })).toEqual({
      effectiveTimezone: undefined,
      effectiveHour12: undefined
    });
  });
});
