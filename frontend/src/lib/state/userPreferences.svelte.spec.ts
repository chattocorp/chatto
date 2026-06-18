import { describe, it, expect, beforeEach } from 'vitest';
import { defaultNotificationSoundFilters, defaultSoundId } from '$lib/audio/notificationSounds';
import { UserPreferencesState } from './userPreferences.svelte';

const STORAGE_KEY = 'chatto:preferences';

describe('UserPreferencesState', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  describe('initial state', () => {
    it('uses the default sound when storage is empty', () => {
      const state = new UserPreferencesState();
      expect(state.notificationSound).toBe(defaultSoundId);
      expect(state.notificationSoundFilters).toEqual(defaultNotificationSoundFilters);
    });

    it('hydrates a valid persisted sound', () => {
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ notificationSound: 'silent' }));
      const state = new UserPreferencesState();
      expect(state.notificationSound).toBe('silent');
      expect(state.notificationSoundFilters).toEqual(defaultNotificationSoundFilters);
    });

    it('hydrates valid persisted notification sound filters', () => {
      localStorage.setItem(
        STORAGE_KEY,
        JSON.stringify({
          notificationSound: 'pop',
          notificationSoundFilters: {
            volume: 1.5,
            highPassHz: 500,
            lowPassHz: 8000,
            echo: 45,
            reverb: 30,
            crunch: 75
          }
        })
      );

      const state = new UserPreferencesState();
      expect(state.notificationSound).toBe('pop');
      expect(state.notificationSoundFilters).toEqual({
        volume: 1.5,
        highPassHz: 500,
        lowPassHz: 8000,
        echo: 45,
        reverb: 30,
        crunch: 75
      });
    });

    it('merges partial stored notification sound filters with defaults', () => {
      localStorage.setItem(
        STORAGE_KEY,
        JSON.stringify({
          notificationSound: 'pop',
          notificationSoundFilters: {
            volume: 0.35
          }
        })
      );

      const state = new UserPreferencesState();
      expect(state.notificationSoundFilters).toEqual({
        ...defaultNotificationSoundFilters,
        volume: 0.35
      });
    });

    it('falls back to default when stored sound id is invalid', () => {
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ notificationSound: 'no-such-sound' }));
      const state = new UserPreferencesState();
      expect(state.notificationSound).toBe(defaultSoundId);
    });

    it('ignores corrupt JSON', () => {
      localStorage.setItem(STORAGE_KEY, 'not-json');
      const state = new UserPreferencesState();
      expect(state.notificationSound).toBe(defaultSoundId);
    });

    it('falls back to safe filter values when stored filters are invalid', () => {
      localStorage.setItem(
        STORAGE_KEY,
        JSON.stringify({
          notificationSound: 'pop',
          notificationSoundFilters: {
            volume: 7,
            highPassHz: -1,
            lowPassHz: 'loud',
            echo: 101,
            reverb: Number.NaN,
            crunch: 'yes'
          }
        })
      );

      const state = new UserPreferencesState();
      expect(state.notificationSoundFilters).toEqual({
        volume: defaultNotificationSoundFilters.volume,
        highPassHz: 20,
        lowPassHz: defaultNotificationSoundFilters.lowPassHz,
        echo: defaultNotificationSoundFilters.echo,
        reverb: defaultNotificationSoundFilters.reverb,
        crunch: defaultNotificationSoundFilters.crunch
      });
    });
  });

  describe('isMuted', () => {
    it('is true when sound is silent', () => {
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ notificationSound: 'silent' }));
      const state = new UserPreferencesState();
      expect(state.isMuted).toBe(true);
    });

    it('is false for any non-silent sound', () => {
      const state = new UserPreferencesState();
      state.notificationSound = 'pop';
      expect(state.isMuted).toBe(false);
    });
  });

  describe('mutation', () => {
    it('updates and persists the notification sound', () => {
      const state = new UserPreferencesState();
      state.notificationSound = 'pop';
      expect(state.notificationSound).toBe('pop');

      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}');
      expect(stored.notificationSound).toBe('pop');
    });

    it('updates and persists individual notification sound filters', () => {
      const state = new UserPreferencesState();
      state.setNotificationSoundFilter('volume', 1.75);
      state.setNotificationSoundFilter('highPassHz', 900);
      state.setNotificationSoundFilter('lowPassHz', 5000);
      state.setNotificationSoundFilter('echo', 35);
      state.setNotificationSoundFilter('reverb', 45);
      state.setNotificationSoundFilter('crunch', 55);

      expect(state.notificationSoundFilters).toEqual({
        volume: 1.75,
        highPassHz: 900,
        lowPassHz: 5000,
        echo: 35,
        reverb: 45,
        crunch: 55
      });

      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}');
      expect(stored.notificationSoundFilters).toEqual({
        volume: 1.75,
        highPassHz: 900,
        lowPassHz: 5000,
        echo: 35,
        reverb: 45,
        crunch: 55
      });
    });

    it('resets notification sound filters to defaults', () => {
      const state = new UserPreferencesState();
      state.notificationSoundFilters = {
        volume: 0.25,
        highPassHz: 700,
        lowPassHz: 4000,
        echo: 35,
        reverb: 45,
        crunch: 55
      };

      state.resetNotificationSoundFilters();

      expect(state.notificationSoundFilters).toEqual(defaultNotificationSoundFilters);
      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}');
      expect(stored.notificationSoundFilters).toEqual(defaultNotificationSoundFilters);
    });
  });
});
