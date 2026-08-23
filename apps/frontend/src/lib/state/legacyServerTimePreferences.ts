import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { userPreferences, type TimeFormatPreference } from '$lib/state/userPreferences.svelte';

export type LegacyServerTimePreferences = {
  timezone?: string | null;
  timeFormat: TimeFormat;
};

function clientTimeFormat(format: TimeFormat): TimeFormatPreference {
  if (format === TimeFormat.TIME_FORMAT_12_HOUR) return '12h';
  if (format === TimeFormat.TIME_FORMAT_24_HOUR) return '24h';
  return 'auto';
}

/** Import the first non-default retired server preference into client-local state. */
export function migrateLegacyServerTimePreferences(
  preferences: LegacyServerTimePreferences | null | undefined
): boolean {
  if (!preferences) return false;
  return userPreferences.migrateLegacyServerTimePreferences({
    timezone: preferences.timezone ?? null,
    timeFormat: clientTimeFormat(preferences.timeFormat)
  });
}
