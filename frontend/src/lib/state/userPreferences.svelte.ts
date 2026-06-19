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
type EffectiveTheme = 'light' | 'dark';

interface Preferences {
  displayTheme: DisplayTheme;
  notificationSound: NotificationSoundId;
  notificationSoundFilters: NotificationSoundFilters;
}

const defaultPreferences: Preferences = {
  displayTheme: 'system',
  notificationSound: defaultSoundId,
  notificationSoundFilters: defaultNotificationSoundFilters
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
    ...defaultPreferences,
    ...stored,
    displayTheme,
    notificationSound: isValidSound ? stored.notificationSound : defaultSoundId,
    notificationSoundFilters: normalizeNotificationSoundFilters(stored.notificationSoundFilters)
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
