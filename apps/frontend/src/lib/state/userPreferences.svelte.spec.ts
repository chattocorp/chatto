import { describe, it, expect, beforeEach, vi } from 'vitest';
import { defaultNotificationSoundFilters, defaultSoundId } from '$lib/audio/notificationSounds';
import { UserPreferencesState, resolveDisplayTheme } from './userPreferences.svelte';

const STORAGE_KEY = 'chatto:preferences';

function mockSystemTheme(theme: 'light' | 'dark') {
  vi.stubGlobal(
    'matchMedia',
    vi.fn((query: string) => ({
      matches: query === '(prefers-color-scheme: dark)' && theme === 'dark',
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn()
    }))
  );
}

describe('UserPreferencesState', () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    mockSystemTheme('light');
    localStorage.clear();
    delete document.documentElement.dataset.theme;
    document.documentElement.style.backgroundColor = '';
  });

  describe('initial state', () => {
    it('uses the system display theme when storage is empty', () => {
      const state = new UserPreferencesState();
      expect(state.displayTheme).toBe('system');
      expect(state.effectiveDisplayTheme).toBe('light');
    });

    it('resolves the system display theme from prefers-color-scheme', () => {
      mockSystemTheme('dark');
      const state = new UserPreferencesState();
      expect(resolveDisplayTheme(state.displayTheme)).toBe('dark');
      expect(state.effectiveDisplayTheme).toBe('dark');
    });

    it.each(['system', 'light', 'dark'] as const)(
      'hydrates a persisted %s display theme',
      (displayTheme) => {
        localStorage.setItem(STORAGE_KEY, JSON.stringify({ displayTheme }));
        const state = new UserPreferencesState();
        expect(state.displayTheme).toBe(displayTheme);
      }
    );

    it('falls back to system when the stored display theme is invalid', () => {
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ displayTheme: 'sepia' }));
      const state = new UserPreferencesState();
      expect(state.displayTheme).toBe('system');
    });

    it('hydrates the legacy localStorage.theme value when no preference exists', () => {
      localStorage.setItem('theme', 'dark');
      const state = new UserPreferencesState();
      expect(state.displayTheme).toBe('dark');
    });

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
    it.each([
      {
        displayTheme: 'light' as const,
        effectiveTheme: 'light' as const,
        background: 'rgb(243, 244, 246)'
      },
      {
        displayTheme: 'dark' as const,
        effectiveTheme: 'dark' as const,
        background: 'rgb(23, 23, 23)'
      },
      {
        displayTheme: 'system' as const,
        effectiveTheme: 'dark' as const,
        background: 'rgb(23, 23, 23)'
      }
    ])(
      'updates, persists, and applies the $displayTheme display theme',
      ({ displayTheme, effectiveTheme, background }) => {
        mockSystemTheme('dark');
        const state = new UserPreferencesState();

        state.displayTheme = displayTheme;

        expect(state.displayTheme).toBe(displayTheme);
        expect(document.documentElement.dataset.theme).toBe(effectiveTheme);
        expect(document.documentElement.style.backgroundColor).toBe(background);

        const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}');
        expect(stored.displayTheme).toBe(displayTheme);
      }
    );

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
