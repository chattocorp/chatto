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

interface Preferences {
  notificationSound: NotificationSoundId;
  notificationSoundFilters: NotificationSoundFilters;
}

const defaultPreferences: Preferences = {
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
  return {
    ...defaultPreferences,
    ...stored,
    notificationSound: isValidSound ? stored.notificationSound : defaultSoundId,
    notificationSoundFilters: normalizeNotificationSoundFilters(stored.notificationSoundFilters)
  };
}

export class UserPreferencesState {
  #prefs = $state<Preferences>(loadPreferences());

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
