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
import { Codecs, globalSlot } from '$lib/storage/slot';

interface Preferences {
  notificationSound: NotificationSoundId;
}

const defaultPreferences: Preferences = {
  notificationSound: defaultSoundId
};

const slot = globalSlot('preferences', defaultPreferences, Codecs.json<Preferences>());

function loadPreferences(): Preferences {
  const stored = slot.get();
  // Validate that the stored sound ID is still valid — silently fall back
  // to the default if the user migrated away from a sound we no longer ship.
  const isValidSound = notificationSounds.some((s) => s.id === stored.notificationSound);
  return {
    ...defaultPreferences,
    ...stored,
    notificationSound: isValidSound ? stored.notificationSound : defaultSoundId
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

  /**
   * Check if notifications are muted (sound set to silent).
   */
  get isMuted(): boolean {
    return this.#prefs.notificationSound === 'silent';
  }
}

export const userPreferences = new UserPreferencesState();
