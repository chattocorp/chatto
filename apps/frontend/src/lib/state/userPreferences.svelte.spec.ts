import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  accentColors,
  surfaceTones,
  type SurfaceTone,
  type AccentColor,
  UserPreferencesState,
  getLegacyNotificationSoundPreferences,
  resolveDisplayTheme
} from './userPreferences.svelte';

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
    delete document.documentElement.dataset.accent;
    document.documentElement.style.removeProperty('--depth-level');
    delete document.documentElement.dataset.lightTone;
    delete document.documentElement.dataset.darkTone;
    document.documentElement.style.removeProperty('--contrast-soft-mix');
    document.documentElement.style.removeProperty('--contrast-strong-mix');
    document.documentElement.style.backgroundColor = '';
    document.documentElement.style.colorScheme = '';
    document.head.innerHTML = '<meta name="theme-color" content="#e5e7eb" />';
  });

  it('uses the product defaults when no preferences are stored', () => {
    const state = new UserPreferencesState();

    expect(state.displayTheme).toBe('system');
    expect(state.accentColor).toBe('cyan');
    expect(state.surfaceDepth).toBe(50);
    expect(state.contrastAge).toBe(30);
    expect(state.lightSurfaceTone).toBe('gray');
    expect(state.darkSurfaceTone).toBe('neutral');
    expect(document.documentElement.dataset.lightTone).toBe('gray');
    expect(document.documentElement.dataset.darkTone).toBe('neutral');
    expect(state.effectiveDisplayTheme).toBe('light');
    expect(state.composerEditor).toBe('markdown');
    expect(state.composerSendMode).toBe('enter');
    expect(state.composerFormattingToolbarVisible).toBe(false);
    expect(state.threadPanePresentation).toBe('overlay');
  });

  it.each([0, 10, 50, 70, 100])(
    'persists and restores %s% surface depth independently',
    (depth) => {
      const state = new UserPreferencesState();
      state.displayTheme = 'dark';
      state.accentColor = 'violet';
      state.surfaceDepth = depth;
      expect(document.documentElement.style.getPropertyValue('--depth-level')).toBe(String(depth));
      expect(document.documentElement.dataset.theme).toBe('dark');
      expect(document.documentElement.dataset.accent).toBe('violet');
      expect(new UserPreferencesState().surfaceDepth).toBe(depth);
      expect(JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}')).toMatchObject({
        surfaceDepth: depth,
        accentColor: 'violet',
        displayTheme: 'dark'
      });
    }
  );

  it.each([
    ['flat', 0],
    ['3d', 50],
    ['very-3d', 100]
  ])('migrates the former %s depth mode', (surfaceDepth, level) => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ surfaceDepth }));
    const state = new UserPreferencesState();
    expect(state.surfaceDepth).toBe(level);
    expect(document.documentElement.style.getPropertyValue('--depth-level')).toBe(String(level));
  });

  it.each([null, -10, 55, 110, 'unknown', {}, []])('rejects invalid stored depth %j', (depth) => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ surfaceDepth: depth }));
    const state = new UserPreferencesState();
    expect(state.surfaceDepth).toBe(50);
    state.surfaceDepth = 0;
    state.surfaceDepth = depth as number;
    expect(state.surfaceDepth).toBe(50);
    expect(document.documentElement.style.getPropertyValue('--depth-level')).toBe('50');
  });

  it.each([
    [25, 26],
    [26.5, 26],
    [33.5, 34]
  ])('snaps saved contrast %s from earlier versions to the 10%% step %s', (saved, step) => {
    // A half step would put the drawn grip and the native thumb in different places.
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ contrastAge: saved }));
    expect(new UserPreferencesState().contrastAge).toBe(step);
  });

  it('keeps preference fields saved by another app version', () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ futurePreference: 'kept' }));
    new UserPreferencesState().accentColor = 'violet';
    expect(JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}')).toMatchObject({
      futurePreference: 'kept',
      accentColor: 'violet'
    });
  });

  it('persists and applies contrast without changing the other appearance choices', () => {
    const state = new UserPreferencesState();
    state.displayTheme = 'dark';
    state.accentColor = 'violet';
    state.surfaceDepth = 0;
    state.contrastAge = 26;

    expect(state.contrastAge).toBe(26);
    expect(document.documentElement.style.getPropertyValue('--contrast-soft-mix')).toBe('40%');
    expect(document.documentElement.style.getPropertyValue('--contrast-strong-mix')).toBe('0%');
    expect(document.documentElement.dataset.theme).toBe('dark');
    expect(document.documentElement.dataset.accent).toBe('violet');
    expect(document.documentElement.style.getPropertyValue('--depth-level')).toBe('0');
    expect(new UserPreferencesState().contrastAge).toBe(26);
    expect(JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}')).toMatchObject({
      contrastAge: 26
    });

    state.contrastAge = 40;
    expect(document.documentElement.style.getPropertyValue('--contrast-soft-mix')).toBe('0%');
    expect(document.documentElement.style.getPropertyValue('--contrast-strong-mix')).toBe('100%');
    expect(document.documentElement.style.backgroundColor).toBe('var(--color-surface)');
    expect(document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')?.content).toBe(
      '#000000'
    );
    state.displayTheme = 'light';
    expect(document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')?.content).toBe(
      '#ffffff'
    );
    state.contrastAge = 30;
    expect(document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')?.content).toBe(
      '#e5e7eb'
    );
  });

  it('applies saved contrast when the client store starts after a reload', () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ contrastAge: 40 }));

    const state = new UserPreferencesState();

    expect(state.contrastAge).toBe(40);
    expect(document.documentElement.style.getPropertyValue('--contrast-soft-mix')).toBe('0%');
    expect(document.documentElement.style.getPropertyValue('--contrast-strong-mix')).toBe('100%');
  });

  it.each([null, '35', 19.5, 40.5, 30.25, {}, []])('rejects invalid contrast age %j', (age) => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ contrastAge: age }));
    const state = new UserPreferencesState();
    expect(state.contrastAge).toBe(30);
    state.contrastAge = 35;
    state.contrastAge = age as number;
    expect(state.contrastAge).toBe(30);
    expect(document.documentElement.style.getPropertyValue('--contrast-soft-mix')).toBe('0%');
    expect(document.documentElement.style.getPropertyValue('--contrast-strong-mix')).toBe('0%');
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
      expect(new UserPreferencesState().displayTheme).toBe(displayTheme);
    }
  );

  it('hydrates the legacy localStorage.theme value when no preference exists', () => {
    localStorage.setItem('theme', 'dark');
    expect(new UserPreferencesState().displayTheme).toBe('dark');
  });

  it('normalizes invalid independently stored app choices', () => {
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({
        displayTheme: 'sepia',
        composerEditor: 'plain-text',
        composerSendMode: 'spacebar',
        composerFormattingToolbarVisible: 'yes',
        threadPanePresentation: 'automatic'
      })
    );

    const state = new UserPreferencesState();

    expect(state.displayTheme).toBe('system');
    expect(state.composerEditor).toBe('markdown');
    expect(state.composerSendMode).toBe('enter');
    expect(state.composerFormattingToolbarVisible).toBe(false);
    expect(state.threadPanePresentation).toBe('overlay');
  });

  it.each(['overlay', 'split'] as const)(
    'hydrates a persisted %s thread pane presentation',
    (threadPanePresentation) => {
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ threadPanePresentation }));
      expect(new UserPreferencesState().threadPanePresentation).toBe(threadPanePresentation);
    }
  );

  it.each([
    {
      displayTheme: 'light' as const,
      effectiveTheme: 'light' as const,
      background: 'var(--color-surface)',
      themeColor: '#e5e7eb'
    },
    {
      displayTheme: 'dark' as const,
      effectiveTheme: 'dark' as const,
      background: 'var(--color-surface)',
      themeColor: '#262626'
    },
    {
      displayTheme: 'system' as const,
      effectiveTheme: 'dark' as const,
      background: 'var(--color-surface)',
      themeColor: '#262626'
    }
  ])(
    'updates, persists, and applies the $displayTheme display theme',
    ({ displayTheme, effectiveTheme, background, themeColor }) => {
      mockSystemTheme('dark');
      const state = new UserPreferencesState();

      state.displayTheme = displayTheme;

      expect(state.displayTheme).toBe(displayTheme);
      expect(document.documentElement.dataset.theme).toBe(effectiveTheme);
      expect(document.documentElement.style.backgroundColor).toBe(background);
      expect(document.documentElement.style.colorScheme).toBe(effectiveTheme);
      expect(document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')?.content).toBe(
        themeColor
      );
      expect(JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}')).toMatchObject({
        displayTheme
      });
    }
  );

  it('updates and persists composer choices', () => {
    const state = new UserPreferencesState();

    state.composerEditor = 'visual';
    state.composerSendMode = 'modifier-enter';
    state.composerFormattingToolbarVisible = true;

    expect(state.composerEditor).toBe('visual');
    expect(state.composerSendMode).toBe('modifier-enter');
    expect(state.composerFormattingToolbarVisible).toBe(true);
    expect(JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}')).toMatchObject({
      composerEditor: 'visual',
      composerSendMode: 'modifier-enter',
      composerFormattingToolbarVisible: true
    });
  });

  it.each(accentColors)(
    'persists and restores the %s accent without changing theme',
    (accentColor) => {
      const state = new UserPreferencesState();
      state.displayTheme = 'dark';
      state.accentColor = accentColor;
      expect(document.documentElement.dataset.accent).toBe(accentColor);
      expect(document.documentElement.dataset.theme).toBe('dark');
      expect(new UserPreferencesState().accentColor).toBe(accentColor);
      expect(JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}')).toMatchObject({
        accentColor,
        displayTheme: 'dark'
      });
    }
  );

  it.each(surfaceTones)('persists and restores the %s tone separately per theme', (tone) => {
    const state = new UserPreferencesState();
    state.lightSurfaceTone = tone;
    state.darkSurfaceTone = 'midnight';
    expect(document.documentElement.dataset.lightTone).toBe(tone);
    expect(document.documentElement.dataset.darkTone).toBe('midnight');

    const restored = new UserPreferencesState();
    expect(restored.lightSurfaceTone).toBe(tone);
    expect(restored.darkSurfaceTone).toBe('midnight');
    expect(JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}')).toMatchObject({
      lightSurfaceTone: tone,
      darkSurfaceTone: 'midnight'
    });
  });

  it('falls back to the default tones for invalid saved or assigned tones', () => {
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({ lightSurfaceTone: 'unknown', darkSurfaceTone: 42 })
    );
    const state = new UserPreferencesState();
    expect(state.lightSurfaceTone).toBe('gray');
    expect(state.darkSurfaceTone).toBe('neutral');
    state.lightSurfaceTone = 'invalid' as SurfaceTone;
    expect(state.lightSurfaceTone).toBe('gray');
    expect(document.documentElement.dataset.lightTone).toBe('gray');
    expect(document.documentElement.dataset.darkTone).toBe('neutral');
  });

  it('falls back to cyan for invalid saved or assigned accents', () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ accentColor: 'unknown' }));
    const state = new UserPreferencesState();
    expect(state.accentColor).toBe('cyan');
    state.accentColor = 'violet';
    state.accentColor = 'invalid' as AccentColor;
    expect(state.accentColor).toBe('cyan');
    expect(document.documentElement.dataset.accent).toBe('cyan');
  });

  it.each(['null', 'false', '123', '"violet"', '{broken'])(
    'recovers from an unusable preferences record: %s',
    (raw) => {
      localStorage.setItem(STORAGE_KEY, raw);
      expect(new UserPreferencesState().accentColor).toBe('cyan');
    }
  );

  it('hydrates a persisted formatting shelf choice', () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ composerFormattingToolbarVisible: true }));

    expect(new UserPreferencesState().composerFormattingToolbarVisible).toBe(true);
  });

  it('updates and persists the thread pane presentation', () => {
    const state = new UserPreferencesState();

    state.threadPanePresentation = 'split';

    expect(state.threadPanePresentation).toBe('split');
    expect(JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}')).toMatchObject({
      threadPanePresentation: 'split'
    });
  });

  it('keeps former global sound fields available only as a migration seed', () => {
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({
        displayTheme: 'system',
        composerEditor: 'markdown',
        composerSendMode: 'enter',
        notificationSound: 'pop',
        notificationSoundFilters: { volume: 1.5, echo: 30 }
      })
    );

    const legacy = getLegacyNotificationSoundPreferences();
    const state = new UserPreferencesState();
    state.threadPanePresentation = 'split';

    expect(legacy.notificationSound).toBe('pop');
    expect(legacy.notificationSoundFilters).toMatchObject({ volume: 1.5, echo: 30 });
    expect(JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}')).toMatchObject({
      threadPanePresentation: 'split',
      notificationSound: 'pop',
      notificationSoundFilters: { volume: 1.5, echo: 30 }
    });
    expect('notificationSound' in state).toBe(false);
  });
});
