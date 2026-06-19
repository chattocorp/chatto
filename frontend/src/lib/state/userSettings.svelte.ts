/**
 * Server-side user display preferences (timezone, time format).
 *
 * Populated from the wire current-user response during app initialization.
 * Used by time formatting utilities to respect user preferences.
 */

import { createContext } from 'svelte';
import { TimeFormat, normalizeTimeFormat } from '$lib/preferences/timeFormat';

export class UserSettingsState {
  /** IANA timezone name, or null for browser default. */
  timezone = $state<string | null>(null);

  /** Time display format preference. */
  timeFormat = $state<TimeFormat>(TimeFormat.Auto);

  /**
   * Effective timezone for Intl.DateTimeFormat.
   * Returns undefined when unset, which tells Intl to use browser default.
   */
  get effectiveTimezone(): string | undefined {
    return this.timezone || undefined;
  }

  /**
   * Effective hour12 option for Intl.DateTimeFormat.
   * Returns undefined when unset, which tells Intl to use locale default.
   */
  get effectiveHour12(): boolean | undefined {
    if (this.timeFormat === TimeFormat.TwelveHour) return true;
    if (this.timeFormat === TimeFormat.TwentyFourHour) return false;
    return undefined;
  }

  /** Update from API settings data. */
  updateFromData(
    settings:
      | { timezone?: string | null; timeFormat?: TimeFormat | string | number | null }
      | null
      | undefined
  ) {
    if (settings) {
      this.timezone = settings.timezone ?? null;
      this.timeFormat = normalizeTimeFormat(settings.timeFormat);
    } else {
      this.timezone = null;
      this.timeFormat = TimeFormat.Auto;
    }
  }
}

export const [getUserSettings, setUserSettings] = createContext<UserSettingsState>();
