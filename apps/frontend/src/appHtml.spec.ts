import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { runInNewContext } from 'node:vm';
import sharp from 'sharp';
import { describe, expect, it } from 'vitest';

const appHtml = readFileSync(new URL('./app.html', import.meta.url), 'utf8');
const configuredLocales = (
  JSON.parse(readFileSync(new URL('../project.inlang/settings.json', import.meta.url), 'utf8')) as {
    locales: string[];
  }
).locales;
const manifest = JSON.parse(
  readFileSync(new URL('../static/manifest.webmanifest', import.meta.url), 'utf8')
) as WebAppManifest;
const themeScript = appHtml.match(/<script>\s*([\s\S]*?)\s*<\/script>/i)?.[1];

type WebAppManifest = {
  icons?: Array<{ src?: string; sizes?: string; type?: string; purpose?: string }>;
};

function metaContent(name: string, mediaFragment: string): string | null {
  const tag = appHtml.match(
    new RegExp(`<meta\\s+[^>]*name="${name}"[^>]*media="[^"]*${mediaFragment}[^"]*"[^>]*>`, 'i')
  )?.[0];

  return tag?.match(/\bcontent="([^"]+)"/i)?.[1] ?? null;
}

function linkTag(rel: string): string | null {
  return (
    appHtml.match(new RegExp(`<link\\s+[^>]*rel="${rel}"[^>]*>`, 'i'))?.[0] ??
    appHtml.match(new RegExp(`<link\\s+[^>]*rel='${rel}'[^>]*>`, 'i'))?.[0] ??
    null
  );
}

function attributeValue(tag: string | null, attribute: string): string | null {
  return tag?.match(new RegExp(`\\b${attribute}=["']([^"']+)["']`, 'i'))?.[1] ?? null;
}

async function transparentPixelCount(path: string): Promise<number> {
  const { data } = await sharp(fileURLToPath(new URL(path, import.meta.url)))
    .ensureAlpha()
    .raw()
    .toBuffer({ resolveWithObject: true });

  let transparent = 0;
  for (let i = 3; i < data.length; i += 4) {
    if (data[i] < 255) transparent += 1;
  }
  return transparent;
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
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => storage.set(key, value)
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
    storedLocale: () => storage.get('PARAGLIDE_LOCALE'),
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

  it('declares the Safari apple touch icon with an explicit size', () => {
    const tag = linkTag('apple-touch-icon');

    expect(attributeValue(tag, 'href')).toBe('/apple-touch-icon');
    expect(attributeValue(tag, 'sizes')).toBe('180x180');
  });

  it('declares the browser favicon with an explicit size', () => {
    const tag = linkTag('icon');

    expect(attributeValue(tag, 'href')).toBe('/favicon');
    expect(attributeValue(tag, 'sizes')).toBe('32x32');
    expect(attributeValue(tag, 'type')).toBeNull();
  });

  it('keeps manifest icon paths pointed at the generated PWA assets', () => {
    expect(manifest.icons).toEqual([
      { src: '/icons/icon-192.png', sizes: '192x192', type: 'image/png' },
      { src: '/icons/icon-512.png', sizes: '512x512', type: 'image/png' },
      {
        src: '/icons/icon-maskable-192.png',
        sizes: '192x192',
        type: 'image/png',
        purpose: 'maskable'
      },
      {
        src: '/icons/icon-maskable-512.png',
        sizes: '512x512',
        type: 'image/png',
        purpose: 'maskable'
      }
    ]);
  });

  it.each([
    ['../static/icons/apple-touch-icon.png', 180],
    ['../static/icons/icon-192.png', 192],
    ['../static/icons/icon-512.png', 512],
    ['../static/icons/icon-maskable-192.png', 192],
    ['../static/icons/icon-maskable-512.png', 512]
  ])('keeps install-facing icon %s opaque at %i square', async (path, size) => {
    const metadata = await sharp(fileURLToPath(new URL(path, import.meta.url))).metadata();

    expect(metadata.width).toBe(size);
    expect(metadata.height).toBe(size);
    await expect(transparentPixelCount(path)).resolves.toBe(0);
  });

  it('keeps the favicon at browser tab size', async () => {
    const metadata = await sharp(
      fileURLToPath(new URL('../static/icons/favicon.png', import.meta.url))
    ).metadata();

    expect(metadata.width).toBe(32);
    expect(metadata.height).toBe(32);
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
    expect(root.style.colorScheme).toBe('light');
  });

  it('uses legacy localStorage.theme when no display preference exists', () => {
    const { root } = runThemeScript({ legacyTheme: 'dark', systemDark: false });
    expect(root.dataset.theme).toBe('dark');
    expect(root.style.colorScheme).toBe('dark');
  });

  it('follows prefers-color-scheme when the display preference is system', () => {
    const { root } = runThemeScript({
      preferences: { displayTheme: 'system' },
      systemDark: true
    });

    expect(root.dataset.theme).toBe('dark');
    expect(root.style.colorScheme).toBe('dark');
  });

  it('follows prefers-color-scheme when no display preference exists', () => {
    const { root } = runThemeScript({ systemDark: true });
    expect(root.dataset.theme).toBe('dark');
    expect(root.style.colorScheme).toBe('dark');
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
  it('keeps first-paint negotiation aligned with the configured locales', () => {
    const localeList = appHtml.match(/const locales = (\[[\s\S]*?\]);/)?.[1];
    expect(localeList).toBeTruthy();
    expect(JSON.parse(localeList!.replaceAll("'", '"'))).toEqual(configuredLocales);
  });

  it('falls back to British English when no browser locale is available', () => {
    const { root } = runThemeScript({ systemDark: false });
    expect(root.lang).toBe('en-GB');
    expect(root.dir).toBe('ltr');
  });

  it('uses the stored Paraglide locale before browser languages', () => {
    const { root } = runThemeScript({
      systemDark: false,
      storedLocale: 'fr-CA',
      browserLanguages: ['en-US']
    });

    expect(root.lang).toBe('fr-CA');
  });

  it('matches exact supported browser language variants', () => {
    const { root } = runThemeScript({
      systemDark: false,
      browserLanguages: ['de-AT', 'en-US']
    });

    expect(root.lang).toBe('de-AT');
  });

  it('maps a language-only browser preference to its default region', () => {
    const result = runThemeScript({
      systemDark: false,
      browserLanguages: ['pt']
    });

    expect(result.root.lang).toBe('pt-BR');
    expect(result.storedLocale()).toBe('pt-BR');
  });

  it('preserves an exact regional English browser locale', () => {
    const { root } = runThemeScript({
      systemDark: false,
      browserLanguages: ['en-US']
    });

    expect(root.lang).toBe('en-US');
  });

  it('falls back other English regions to British English', () => {
    const { root } = runThemeScript({
      systemDark: false,
      browserLanguages: ['en-AU']
    });

    expect(root.lang).toBe('en-GB');
  });

  it('migrates the legacy English preference to British English', () => {
    const result = runThemeScript({
      systemDark: false,
      storedLocale: 'en',
      browserLanguages: ['de-DE']
    });

    expect(result.root.lang).toBe('en-GB');
    expect(result.storedLocale()).toBe('en-GB');
  });

  it('migrates the legacy German preference to German for Germany', () => {
    const result = runThemeScript({
      systemDark: false,
      storedLocale: 'de',
      browserLanguages: ['en-US']
    });

    expect(result.root.lang).toBe('de-DE');
    expect(result.storedLocale()).toBe('de-DE');
  });

  it('ignores an unsupported stored locale when matching browser preferences', () => {
    const result = runThemeScript({
      systemDark: false,
      storedLocale: 'it-IT',
      browserLanguages: ['fr-CA']
    });

    expect(result.root.lang).toBe('fr-CA');
    expect(result.storedLocale()).toBe('fr-CA');
  });
});
