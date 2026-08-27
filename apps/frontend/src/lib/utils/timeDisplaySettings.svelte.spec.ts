import { beforeEach, describe, expect, it } from 'vitest';
import { userPreferences } from '$lib/state/userPreferences.svelte';
import { hour12ForClockFormat, timeDisplaySettings } from './formatTime';

describe('hour12ForClockFormat', () => {
  it('maps explicit clock formats and leaves the device format to the locale', () => {
    expect(hour12ForClockFormat('12h')).toBe(true);
    expect(hour12ForClockFormat('24h')).toBe(false);
    expect(hour12ForClockFormat('device')).toBeUndefined();
  });
});

describe('timeDisplaySettings', () => {
  beforeEach(() => {
    localStorage.clear();
    userPreferences.timeZone = '';
    userPreferences.clockFormat = 'device';
  });

  it('maps app preferences into display formatting options', () => {
    userPreferences.timeZone = 'Europe/Berlin';
    userPreferences.clockFormat = '24h';

    expect(timeDisplaySettings()).toEqual({
      effectiveTimezone: 'Europe/Berlin',
      effectiveHour12: false
    });
  });

  it('falls back to device timezone and locale clock convention when unset', () => {
    expect(timeDisplaySettings()).toEqual({
      effectiveTimezone: undefined,
      effectiveHour12: undefined
    });
  });

  it('rejects unknown timezone names when set', () => {
    userPreferences.timeZone = 'Europe/Berlin';
    expect(timeDisplaySettings().effectiveTimezone).toBe('Europe/Berlin');

    userPreferences.timeZone = 'Mars/Olympus';
    expect(userPreferences.timeZone).toBe('');
    expect(timeDisplaySettings().effectiveTimezone).toBeUndefined();
  });
});
