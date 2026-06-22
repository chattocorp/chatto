import { readFileSync } from 'node:fs';
import { runInNewContext } from 'node:vm';
import { describe, expect, it } from 'vitest';

const appHtml = readFileSync(new URL('./app.html', import.meta.url), 'utf8');
const themeScript = appHtml.match(/<script>\s*([\s\S]*?)\s*<\/script>/i)?.[1];

function metaContent(name: string, mediaFragment: string): string | null {
  const tag = appHtml.match(
    new RegExp(`<meta\\s+[^>]*name="${name}"[^>]*media="[^"]*${mediaFragment}[^"]*"[^>]*>`, 'i')
  )?.[0];

  return tag?.match(/\bcontent="([^"]+)"/i)?.[1] ?? null;
}

function runThemeScript({
  preferences,
  legacyTheme,
  systemDark,
  storedLocale,
  browserLanguages
}: {
  preferences?: unknown;
  legacyTheme?: string;
  systemDark: boolean;
  storedLocale?: string;
  browserLanguages?: string[];
}) {
  if (!themeScript) throw new Error('theme script not found');

  const storage = new Map<string, string>();
  if (preferences !== undefined) {
    storage.set('chatto:preferences', JSON.stringify(preferences));
  }
  if (legacyTheme !== undefined) {
    storage.set('theme', legacyTheme);
  }
  if (storedLocale !== undefined) {
    storage.set('PARAGLIDE_LOCALE', storedLocale);
  }

  let dark = systemDark;
  let changeHandler: (() => void) | undefined;
  const root: {
    dataset: Record<string, string>;
    style: Record<string, string>;
    lang?: string;
    dir?: string;
  } = { dataset: {}, style: {} };

  runInNewContext(themeScript, {
    document: { documentElement: root },
    localStorage: {
      getItem: (key: string) => storage.get(key) ?? null
    },
    ...(browserLanguages
      ? { navigator: { languages: browserLanguages, language: browserLanguages[0] } }
      : {}),
    window: {
      matchMedia: () => ({
        get matches() {
          return dark;
        },
        addEventListener: (_type: string, handler: () => void) => {
          changeHandler = handler;
        }
      })
    }
  });

  return {
    root,
    changeSystemTheme(systemTheme: 'light' | 'dark') {
      dark = systemTheme === 'dark';
      changeHandler?.();
    }
  };
}

describe('app.html metadata', () => {
  it('defines theme colors matching the outer frame background colors', () => {
    expect(metaContent('theme-color', 'light')).toBe('#e5e7eb');
    expect(metaContent('theme-color', 'dark')).toBe('#262626');
  });
});

describe('app.html theme bootstrap', () => {
  it('reads chatto:preferences.displayTheme before legacy localStorage.theme', () => {
    const { root } = runThemeScript({
      preferences: { displayTheme: 'light' },
      legacyTheme: 'dark',
      systemDark: true
    });

    expect(root.dataset.theme).toBe('light');
    expect(root.style.backgroundColor).toBe('#f3f4f6');
  });

  it('uses legacy localStorage.theme when no display preference exists', () => {
    const { root } = runThemeScript({ legacyTheme: 'dark', systemDark: false });
    expect(root.dataset.theme).toBe('dark');
  });

  it('follows prefers-color-scheme when the display preference is system', () => {
    const { root } = runThemeScript({
      preferences: { displayTheme: 'system' },
      systemDark: true
    });

    expect(root.dataset.theme).toBe('dark');
  });

  it('follows prefers-color-scheme when no display preference exists', () => {
    const { root } = runThemeScript({ systemDark: true });
    expect(root.dataset.theme).toBe('dark');
  });

  it('only reacts to system theme changes while the display preference is system', () => {
    const system = runThemeScript({
      preferences: { displayTheme: 'system' },
      systemDark: false
    });
    system.changeSystemTheme('dark');
    expect(system.root.dataset.theme).toBe('dark');

    const explicit = runThemeScript({
      preferences: { displayTheme: 'light' },
      systemDark: false
    });
    explicit.changeSystemTheme('dark');
    expect(explicit.root.dataset.theme).toBe('light');
  });
});

describe('app.html locale bootstrap', () => {
  it('falls back to English when no browser locale is available', () => {
    const { root } = runThemeScript({ systemDark: false });
    expect(root.lang).toBe('en');
    expect(root.dir).toBe('ltr');
  });

  it('uses the stored Paraglide locale before browser languages', () => {
    const { root } = runThemeScript({
      systemDark: false,
      storedLocale: 'de',
      browserLanguages: ['en-US']
    });

    expect(root.lang).toBe('de');
  });

  it('matches supported browser language variants', () => {
    const { root } = runThemeScript({
      systemDark: false,
      browserLanguages: ['de-AT', 'en-US']
    });

    expect(root.lang).toBe('de');
  });
});
