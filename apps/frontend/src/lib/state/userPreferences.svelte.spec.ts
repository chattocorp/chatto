import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
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
    document.documentElement.style.backgroundColor = '';
    document.documentElement.style.colorScheme = '';
  });

  it('uses the product defaults when no preferences are stored', () => {
    const state = new UserPreferencesState();

    expect(state.displayTheme).toBe('system');
    expect(state.effectiveDisplayTheme).toBe('light');
    expect(state.composerEditor).toBe('markdown');
    expect(state.composerSendMode).toBe('enter');
    expect(state.composerFormattingToolbarVisible).toBe(false);
    expect(state.threadPanePresentation).toBe('overlay');
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

  it('updates, persists, and applies the display theme', () => {
    const state = new UserPreferencesState();

    state.displayTheme = 'dark';

    expect(state.displayTheme).toBe('dark');
    expect(document.documentElement.dataset.theme).toBe('dark');
    expect(document.documentElement.style.backgroundColor).toBe('rgb(23, 23, 23)');
    expect(document.documentElement.style.colorScheme).toBe('dark');
    expect(JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}')).toMatchObject({
      displayTheme: 'dark'
    });
  });

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
