import { beforeEach, describe, expect, it } from 'vitest';
import { defaultNotificationSoundFilters, defaultSoundId } from '$lib/audio/notificationSounds';
import {
  ServerNotificationPreferencesState,
  getServerNotificationPreferences,
  resetServerNotificationPreferencesForTests
} from './serverNotificationPreferences.svelte';

function storageKey(serverId: string) {
  return `chatto:i:${serverId}:notificationPreferences`;
}

function setLegacyGlobalSound(
  notificationSound: string,
  notificationSoundFilters = defaultNotificationSoundFilters
) {
  localStorage.setItem(
    'chatto:preferences',
    JSON.stringify({ notificationSound, notificationSoundFilters })
  );
}

describe('ServerNotificationPreferencesState', () => {
  beforeEach(() => {
    localStorage.clear();
    resetServerNotificationPreferencesForTests();
  });

  it('persists independent choices for each server', () => {
    const first = getServerNotificationPreferences('first');
    const second = getServerNotificationPreferences('second');

    first.notificationSound = 'pop';
    second.notificationSound = 'chime-up';

    expect(first.notificationSound).toBe('pop');
    expect(second.notificationSound).toBe('chime-up');
    expect(JSON.parse(localStorage.getItem(storageKey('first')) ?? '{}')).toMatchObject({
      notificationSound: 'pop'
    });
    expect(JSON.parse(localStorage.getItem(storageKey('second')) ?? '{}')).toMatchObject({
      notificationSound: 'chime-up'
    });
  });

  it('seeds a new server slot from the former global sound preference', () => {
    setLegacyGlobalSound('pop', {
      ...defaultNotificationSoundFilters,
      volume: 1.5,
      echo: 40
    });

    const state = new ServerNotificationPreferencesState('migrated');

    expect(state.notificationSound).toBe('pop');
    expect(state.notificationSoundFilters).toEqual({
      ...defaultNotificationSoundFilters,
      volume: 1.5,
      echo: 40
    });
    expect(localStorage.getItem(storageKey('migrated'))).not.toBeNull();
  });

  it('prefers an existing server-specific choice over the migration seed', () => {
    setLegacyGlobalSound('pop');
    localStorage.setItem(
      storageKey('existing'),
      JSON.stringify({
        notificationSound: 'silent',
        notificationSoundFilters: { ...defaultNotificationSoundFilters, reverb: 25 }
      })
    );

    const state = new ServerNotificationPreferencesState('existing');

    expect(state.notificationSound).toBe('silent');
    expect(state.notificationSoundFilters.reverb).toBe(25);
  });

  it('normalizes unsupported server-specific values', () => {
    localStorage.setItem(
      storageKey('invalid'),
      JSON.stringify({
        notificationSound: 'missing',
        notificationSoundFilters: {
          volume: 10,
          highPassHz: -1,
          lowPassHz: 'loud',
          echo: 101,
          reverb: null,
          crunch: 50
        }
      })
    );

    const state = new ServerNotificationPreferencesState('invalid');

    expect(state.notificationSound).toBe(defaultSoundId);
    expect(state.notificationSoundFilters).toEqual({
      ...defaultNotificationSoundFilters,
      crunch: 50
    });
  });
});
