/**
 * User preferences store.
 *
 * Stores user preferences in localStorage for persistence across sessions.
 * These are client-side preferences that don't need server sync.
 */

import {
  type NotificationSoundId,
  defaultSoundId,
  notificationSounds
} from '$lib/audio/notificationSounds';

const STORAGE_KEY = 'chatto:preferences';

interface Preferences {
  notificationSound: NotificationSoundId;
}

const defaultPreferences: Preferences = {
  notificationSound: defaultSoundId
};

function loadPreferences(): Preferences {
  if (typeof localStorage === 'undefined') {
    return defaultPreferences;
  }

  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) {
      const parsed = JSON.parse(stored);

      // Validate that the stored sound ID is still valid
      const soundId = parsed.notificationSound;
      const isValidSound = notificationSounds.some((s) => s.id === soundId);

      return {
        ...defaultPreferences,
        ...parsed,
        // Fall back to default if stored sound is invalid
        notificationSound: isValidSound ? soundId : defaultSoundId
      };
    }
  } catch {
    // Ignore parse errors, use defaults
  }
  return defaultPreferences;
}

function savePreferences(prefs: Preferences): void {
  if (typeof localStorage === 'undefined') {
    return;
  }

  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(prefs));
  } catch {
    // Ignore storage errors (quota exceeded, etc.)
  }
}

export class UserPreferencesState {
  #prefs = $state<Preferences>(loadPreferences());

  get notificationSound(): NotificationSoundId {
    return this.#prefs.notificationSound;
  }

  set notificationSound(value: NotificationSoundId) {
    this.#prefs.notificationSound = value;
    savePreferences(this.#prefs);
  }

  /**
   * Check if notifications are muted (sound set to silent).
   */
  get isMuted(): boolean {
    return this.#prefs.notificationSound === 'silent';
  }
}

export const userPreferences = new UserPreferencesState();
