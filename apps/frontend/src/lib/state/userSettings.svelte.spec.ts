import { describe, expect, it } from 'vitest';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { hour12ForTimeFormat, UserSettingsState } from './userSettings.svelte';

describe('hour12ForTimeFormat', () => {
  it('maps explicit clock formats and leaves automatic formats to the locale', () => {
    expect(hour12ForTimeFormat(TimeFormat.TIME_FORMAT_12_HOUR)).toBe(true);
    expect(hour12ForTimeFormat(TimeFormat.TIME_FORMAT_24_HOUR)).toBe(false);
    expect(hour12ForTimeFormat(TimeFormat.TIME_FORMAT_AUTO)).toBeUndefined();
  });
});

describe('UserSettingsState', () => {
  it('stores protobuf-native settings and resets to automatic defaults', () => {
    const settings = new UserSettingsState();

    settings.updateFromData({
      timezone: 'Europe/Berlin',
      timeFormat: TimeFormat.TIME_FORMAT_24_HOUR
    });

    expect(settings.timezone).toBe('Europe/Berlin');
    expect(settings.timeFormat).toBe(TimeFormat.TIME_FORMAT_24_HOUR);
    expect(settings.effectiveHour12).toBe(false);

    settings.updateFromData(null);

    expect(settings.timezone).toBeNull();
    expect(settings.timeFormat).toBe(TimeFormat.TIME_FORMAT_AUTO);
    expect(settings.effectiveHour12).toBeUndefined();
  });
});
