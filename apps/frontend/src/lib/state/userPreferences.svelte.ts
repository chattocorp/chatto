/**
 * User preferences store.
 *
 * Stores user preferences in localStorage for persistence across sessions.
 * These are client-side preferences that don't need server sync.
 */

import {
  type NotificationSoundFilters,
  type NotificationSoundId,
  defaultNotificationSoundFilters,
  defaultSoundId,
  notificationSounds
} from '$lib/audio/notificationSounds';
import { Codecs, globalSlot } from '$lib/storage/slot';

export type DisplayTheme = 'system' | 'light' | 'dark';
export type ComposerEditorKind = 'visual' | 'markdown';
export type ComposerSendMode = 'enter' | 'modifier-enter';
export type TimeFormatPreference = 'auto' | '12h' | '24h';
export type TimePreferences = {
  timezone: string | null;
  timeFormat: TimeFormatPreference;
};
type EffectiveTheme = 'light' | 'dark';

interface Preferences extends TimePreferences {
  displayTheme: DisplayTheme;
  composerEditor: ComposerEditorKind;
  composerSendMode: ComposerSendMode;
  notificationSound: NotificationSoundId;
  notificationSoundFilters: NotificationSoundFilters;
  legacyServerTimePreferencesMigrated: boolean;
}

const defaultPreferences: Preferences = {
  displayTheme: 'system',
  composerEditor: 'markdown',
  composerSendMode: 'enter',
  timezone: null,
  timeFormat: 'auto',
  notificationSound: defaultSoundId,
  notificationSoundFilters: defaultNotificationSoundFilters,
  legacyServerTimePreferencesMigrated: false
};

const slot = globalSlot('preferences', defaultPreferences, Codecs.json<Preferences>());

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function clampNumber(value: unknown, min: number, max: number, fallback: number): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return fallback;
  if (value < min || value > max) return fallback;
  return value;
}

function isDisplayTheme(value: unknown): value is DisplayTheme {
  return value === 'system' || value === 'light' || value === 'dark';
}

function isComposerEditorKind(value: unknown): value is ComposerEditorKind {
  return value === 'visual' || value === 'markdown';
}

function isComposerSendMode(value: unknown): value is ComposerSendMode {
  return value === 'enter' || value === 'modifier-enter';
}

function isTimeFormatPreference(value: unknown): value is TimeFormatPreference {
  return value === 'auto' || value === '12h' || value === '24h';
}

function normalizeTimezone(value: unknown): string | null {
  if (typeof value !== 'string' || value.trim() === '') return null;
  const timezone = value.trim();
  try {
    new Intl.DateTimeFormat('en-US', { timeZone: timezone }).format(new Date());
    return timezone;
  } catch {
    return null;
  }
}

function getLegacyDisplayTheme(): DisplayTheme | null {
  if (typeof localStorage === 'undefined') return null;
  try {
    const legacy = localStorage.getItem('theme');
    return isDisplayTheme(legacy) && legacy !== 'system' ? legacy : null;
  } catch {
    return null;
  }
}

function getStoredDisplayTheme(): DisplayTheme | null {
  if (typeof localStorage === 'undefined') return null;
  try {
    const raw = localStorage.getItem(slot.key);
    if (!raw) return null;
    const parsed: unknown = JSON.parse(raw);
    if (!isRecord(parsed)) return null;
    return isDisplayTheme(parsed.displayTheme) ? parsed.displayTheme : null;
  } catch {
    return null;
  }
}

export function resolveDisplayTheme(theme: DisplayTheme): EffectiveTheme {
  if (theme === 'light' || theme === 'dark') return theme;
  if (typeof window === 'undefined') return 'light';
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

export function applyDisplayTheme(theme: DisplayTheme): void {
  if (typeof document === 'undefined') return;
  const effective = resolveDisplayTheme(theme);
  const root = document.documentElement;
  root.dataset.theme = effective;
  root.style.backgroundColor = effective === 'dark' ? '#171717' : '#f3f4f6';
  root.style.colorScheme = effective;
}

function normalizeNotificationSoundFilters(value: unknown): NotificationSoundFilters {
  const stored = isRecord(value) ? value : {};
  return {
    volume: clampNumber(stored.volume, 0, 2, defaultNotificationSoundFilters.volume),
    highPassHz: clampNumber(
      stored.highPassHz,
      20,
      2000,
      defaultNotificationSoundFilters.highPassHz
    ),
    lowPassHz: clampNumber(stored.lowPassHz, 800, 20000, defaultNotificationSoundFilters.lowPassHz),
    echo: clampNumber(stored.echo, 0, 100, defaultNotificationSoundFilters.echo),
    reverb: clampNumber(stored.reverb, 0, 100, defaultNotificationSoundFilters.reverb),
    crunch: clampNumber(stored.crunch, 0, 100, defaultNotificationSoundFilters.crunch)
  };
}

function loadPreferences(): Preferences {
  const stored = slot.get();
  // Validate that the stored sound ID is still valid — silently fall back
  // to the default if the user migrated away from a sound we no longer ship.
  const isValidSound = notificationSounds.some((s) => s.id === stored.notificationSound);
  const displayTheme =
    getStoredDisplayTheme() ?? getLegacyDisplayTheme() ?? defaultPreferences.displayTheme;
  return {
    displayTheme,
    composerEditor: isComposerEditorKind(stored.composerEditor)
      ? stored.composerEditor
      : defaultPreferences.composerEditor,
    composerSendMode: isComposerSendMode(stored.composerSendMode)
      ? stored.composerSendMode
      : defaultPreferences.composerSendMode,
    timezone: normalizeTimezone(stored.timezone),
    timeFormat: isTimeFormatPreference(stored.timeFormat)
      ? stored.timeFormat
      : defaultPreferences.timeFormat,
    notificationSound: isValidSound ? stored.notificationSound : defaultSoundId,
    notificationSoundFilters: normalizeNotificationSoundFilters(stored.notificationSoundFilters),
    legacyServerTimePreferencesMigrated: stored.legacyServerTimePreferencesMigrated === true
  };
}

export class UserPreferencesState {
  #prefs = $state<Preferences>(loadPreferences());

  get displayTheme(): DisplayTheme {
    return this.#prefs.displayTheme;
  }

  set displayTheme(value: DisplayTheme) {
    const displayTheme = isDisplayTheme(value) ? value : defaultPreferences.displayTheme;
    this.#prefs.displayTheme = displayTheme;
    slot.set(this.#prefs);
    applyDisplayTheme(displayTheme);
  }

  get effectiveDisplayTheme(): EffectiveTheme {
    return resolveDisplayTheme(this.#prefs.displayTheme);
  }

  get composerEditor(): ComposerEditorKind {
    return this.#prefs.composerEditor;
  }

  set composerEditor(value: ComposerEditorKind) {
    this.#prefs.composerEditor = isComposerEditorKind(value)
      ? value
      : defaultPreferences.composerEditor;
    slot.set(this.#prefs);
  }

  get composerSendMode(): ComposerSendMode {
    return this.#prefs.composerSendMode;
  }

  set composerSendMode(value: ComposerSendMode) {
    this.#prefs.composerSendMode = isComposerSendMode(value)
      ? value
      : defaultPreferences.composerSendMode;
    slot.set(this.#prefs);
  }

  get timezone(): string | null {
    return this.#prefs.timezone;
  }

  get timeFormat(): TimeFormatPreference {
    return this.#prefs.timeFormat;
  }

  /** Replace the client-wide time display preferences and make them authoritative. */
  setTimePreferences(value: TimePreferences): void {
    this.#prefs.timezone = normalizeTimezone(value.timezone);
    this.#prefs.timeFormat = isTimeFormatPreference(value.timeFormat)
      ? value.timeFormat
      : defaultPreferences.timeFormat;
    this.#prefs.legacyServerTimePreferencesMigrated = true;
    slot.set(this.#prefs);
  }

  /**
   * Import one non-default value from the retired per-server model. The first
   * non-default legacy value wins; an intentional local choice always wins over
   * later server loads.
   */
  migrateLegacyServerTimePreferences(value: TimePreferences): boolean {
    if (this.#prefs.legacyServerTimePreferencesMigrated) return false;
    const timezone = normalizeTimezone(value.timezone);
    const timeFormat = isTimeFormatPreference(value.timeFormat)
      ? value.timeFormat
      : defaultPreferences.timeFormat;
    if (timezone === null && timeFormat === 'auto') return false;

    this.#prefs.timezone = timezone;
    this.#prefs.timeFormat = timeFormat;
    this.#prefs.legacyServerTimePreferencesMigrated = true;
    slot.set(this.#prefs);
    return true;
  }

  get notificationSound(): NotificationSoundId {
    return this.#prefs.notificationSound;
  }

  set notificationSound(value: NotificationSoundId) {
    this.#prefs.notificationSound = value;
    slot.set(this.#prefs);
  }

  get notificationSoundFilters(): NotificationSoundFilters {
    return this.#prefs.notificationSoundFilters;
  }

  set notificationSoundFilters(value: NotificationSoundFilters) {
    this.#prefs.notificationSoundFilters = normalizeNotificationSoundFilters(value);
    slot.set(this.#prefs);
  }

  setNotificationSoundFilter(key: keyof NotificationSoundFilters, value: number) {
    this.notificationSoundFilters = {
      ...this.#prefs.notificationSoundFilters,
      [key]: value
    };
  }

  resetNotificationSoundFilters() {
    this.notificationSoundFilters = defaultNotificationSoundFilters;
  }

  /**
   * Check if notifications are muted (sound set to silent).
   */
  get isMuted(): boolean {
    return this.#prefs.notificationSound === 'silent';
  }
}

export const userPreferences = new UserPreferencesState();
