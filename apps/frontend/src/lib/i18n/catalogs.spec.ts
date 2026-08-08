import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';
import { baseLocale, selectableLocales } from './locales';

const messagesRoot = fileURLToPath(new URL('../../../messages/', import.meta.url));
const sourceRoot = fileURLToPath(new URL('../../../src/', import.meta.url));
const sparseLocales = new Set(['en-US']);
const pluralCategories = new Set(['zero', 'one', 'two', 'few', 'many', 'other']);

function placeholders(value: string): string[] {
  return [...value.matchAll(/\{[^{}]+\}/g)].map(([match]) => match).sort();
}

function isPlural(value: unknown): value is Record<string, string> {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return false;
  return (
    Object.keys(value).length > 0 &&
    Object.entries(value).every(
      ([key, branch]) => pluralCategories.has(key) && typeof branch === 'string'
    )
  );
}

function compareCatalogValue(source: unknown, translated: unknown, path: string): void {
  if (isPlural(source)) {
    expect(isPlural(translated), `${path} must remain a plural object`).toBe(true);
    expect((translated as Record<string, string>).other, `${path} must define other`).toBeTypeOf(
      'string'
    );
    const expectedPlaceholders = placeholders(source.other);
    for (const [category, branch] of Object.entries(translated as Record<string, string>)) {
      expect(placeholders(branch), `${path}.${category} must preserve placeholders`).toEqual(
        expectedPlaceholders
      );
    }
    return;
  }

  if (source && typeof source === 'object') {
    expect(translated, `${path} must remain an object`).toBeTypeOf('object');
    expect(Array.isArray(translated), `${path} must not become an array`).toBe(false);
    expect(Object.keys(translated as object), `${path} must keep the same keys`).toEqual(
      Object.keys(source)
    );
    for (const [key, value] of Object.entries(source)) {
      compareCatalogValue(value, (translated as Record<string, unknown>)[key], `${path}.${key}`);
    }
    return;
  }

  expect(typeof translated, `${path} must keep its scalar type`).toBe(typeof source);
  if (typeof source !== 'string' || typeof translated !== 'string') return;

  expect(placeholders(translated), `${path} must preserve placeholders`).toEqual(
    placeholders(source)
  );
  if (source.includes('Chatto')) {
    expect(translated, `${path} must preserve the Chatto product name`).toContain('Chatto');
  }
}

function sourceFiles(root: string): string[] {
  return readdirSync(root, { withFileTypes: true }).flatMap((entry) => {
    const path = join(root, entry.name);
    if (entry.isDirectory()) return sourceFiles(path);
    return /\.(?:svelte|ts)$/.test(entry.name) ? [path] : [];
  });
}

function collectMessageKeys(value: unknown, path = '', keys = new Set<string>()): Set<string> {
  if (typeof value === 'string' || isPlural(value)) {
    keys.add(path);
    return keys;
  }
  if (!value || typeof value !== 'object' || Array.isArray(value)) return keys;
  for (const [key, child] of Object.entries(value)) {
    collectMessageKeys(child, path ? `${path}.${key}` : key, keys);
  }
  return keys;
}

function expectNoEmptyMessages(value: unknown, path: string): void {
  if (typeof value === 'string') {
    expect(value.trim(), `${path} must not be empty`).not.toBe('');
    return;
  }
  if (!value || typeof value !== 'object' || Array.isArray(value)) return;
  for (const [key, child] of Object.entries(value)) {
    expectNoEmptyMessages(child, `${path}.${key}`);
  }
}

describe('translated message catalogs', () => {
  it('defines every literal message key used by frontend source', () => {
    const sourceKeys = new Set<string>();
    for (const filename of readdirSync(join(messagesRoot, baseLocale)).filter((name) =>
      name.endsWith('.json')
    )) {
      collectMessageKeys(
        JSON.parse(readFileSync(join(messagesRoot, baseLocale, filename), 'utf8')),
        '',
        sourceKeys
      );
    }

    for (const filename of sourceFiles(sourceRoot)) {
      const source = readFileSync(filename, 'utf8');
      for (const match of source.matchAll(/\bm(?:Html)?\(\s*(['"])([^'"\n]+)\1/g)) {
        expect(sourceKeys, `${filename} uses unknown message key ${match[2]}`).toContain(match[2]);
      }
    }
  });

  it('keeps every complete locale structurally aligned with the source catalog', () => {
    const sourceFiles = readdirSync(join(messagesRoot, baseLocale))
      .filter((filename) => filename.endsWith('.json'))
      .sort();

    for (const locale of selectableLocales.filter((locale) => !sparseLocales.has(locale))) {
      const localeFiles = readdirSync(join(messagesRoot, locale))
        .filter((filename) => filename.endsWith('.json'))
        .sort();
      expect(localeFiles, `${locale} must contain every catalog file`).toEqual(sourceFiles);

      for (const filename of sourceFiles) {
        const source = JSON.parse(readFileSync(join(messagesRoot, baseLocale, filename), 'utf8'));
        const translated = JSON.parse(readFileSync(join(messagesRoot, locale, filename), 'utf8'));
        compareCatalogValue(source, translated, `${locale}.${filename}`);
      }
    }
  });

  it('keeps functional account-deletion literals untranslated', () => {
    for (const locale of selectableLocales) {
      const catalog = JSON.parse(
        readFileSync(join(messagesRoot, locale, 'settings.json'), 'utf8')
      ) as {
        settings?: { account?: { delete_modal?: Record<string, string> } };
      };
      const modal = catalog.settings?.account?.delete_modal;
      if (!modal) continue;
      expect(modal.confirm_label, `${locale} must tell users to type DELETE`).toContain('DELETE');
      expect(modal.confirm_placeholder, `${locale} must preserve the DELETE token`).toBe('DELETE');
    }
  });

  it.each(['ar', 'he-IL'] as const)('keeps every %s message populated', (locale) => {
    for (const filename of readdirSync(join(messagesRoot, locale)).filter((name) =>
      name.endsWith('.json')
    )) {
      expectNoEmptyMessages(
        JSON.parse(readFileSync(join(messagesRoot, locale, filename), 'utf8')),
        `${locale}.${filename}`
      );
    }
  });

  it('keeps syntax-like examples valid', () => {
    for (const locale of selectableLocales.filter((locale) => !sparseLocales.has(locale))) {
      const common = JSON.parse(
        readFileSync(join(messagesRoot, locale, 'common.json'), 'utf8')
      ) as { common: { username_placeholder: string } };
      const preferences = (
        JSON.parse(readFileSync(join(messagesRoot, locale, 'settings.json'), 'utf8')) as {
          settings: {
            preferences: { time_format: Record<'12h' | '24h', { description: string }> };
          };
        }
      ).settings.preferences;

      expect(common.common.username_placeholder, `${locale} must show a valid ASCII username`).toBe(
        'your_username'
      );
      expect(
        preferences.time_format['12h'].description,
        `${locale} must show a 12-hour example`
      ).not.toContain('14:30');
      expect(
        preferences.time_format['12h'].description,
        `${locale} must distinguish 12-hour and 24-hour examples`
      ).not.toBe(preferences.time_format['24h'].description);
    }
  });
});
