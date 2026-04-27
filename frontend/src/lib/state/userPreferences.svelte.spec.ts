import { describe, it, expect, beforeEach } from 'vitest';
import { defaultSoundId } from '$lib/audio/notificationSounds';
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
    });

    it('hydrates a valid persisted sound', () => {
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ notificationSound: 'silent' }));
      const state = new UserPreferencesState();
      expect(state.notificationSound).toBe('silent');
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
  });
});
