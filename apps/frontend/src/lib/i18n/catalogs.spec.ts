import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const messagesRoot = fileURLToPath(new URL('../../../messages/', import.meta.url));
const settings = JSON.parse(
  readFileSync(new URL('../../../project.inlang/settings.json', import.meta.url), 'utf8')
) as { baseLocale: string; locales: string[] };
const sparseLocales = new Set(['en-US']);

function placeholders(value: string): string[] {
  return [...value.matchAll(/\{[^{}]+\}/g)].map(([match]) => match).sort();
}

function compareCatalogValue(source: unknown, translated: unknown, path: string): void {
  if (Array.isArray(source)) {
    expect(Array.isArray(translated), `${path} must remain an array`).toBe(true);
    expect(translated, `${path} must keep the same number of entries`).toHaveLength(source.length);
    source.forEach((value, index) =>
      compareCatalogValue(value, (translated as unknown[])[index], `${path}.${index}`)
    );
    return;
  }

  if (source && typeof source === 'object') {
    expect(translated, `${path} must remain an object`).toBeTypeOf('object');
    expect(Array.isArray(translated), `${path} must not become an array`).toBe(false);
    if (path.endsWith('.match')) {
      expect(Object.keys(translated as object), `${path} must keep every source branch`).toEqual(
        expect.arrayContaining(Object.keys(source))
      );
    } else {
      expect(Object.keys(translated as object), `${path} must keep the same keys`).toEqual(
        Object.keys(source)
      );
    }
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
  if (path.includes('.declarations.') || path.includes('.selectors.')) {
    expect(translated, `${path} is message syntax, not translated copy`).toBe(source);
  }
  if (source.includes('Chatto')) {
    expect(translated, `${path} must preserve the Chatto product name`).toContain('Chatto');
  }
}

describe('translated message catalogs', () => {
  it('keeps every complete locale structurally aligned with the source catalog', () => {
    const sourceFiles = readdirSync(join(messagesRoot, settings.baseLocale))
      .filter((filename) => filename.endsWith('.json'))
      .sort();

    for (const locale of settings.locales.filter((locale) => !sparseLocales.has(locale))) {
      const localeFiles = readdirSync(join(messagesRoot, locale))
        .filter((filename) => filename.endsWith('.json'))
        .sort();
      expect(localeFiles, `${locale} must contain every catalog file`).toEqual(sourceFiles);

      for (const filename of sourceFiles) {
        const source = JSON.parse(
          readFileSync(join(messagesRoot, settings.baseLocale, filename), 'utf8')
        );
        const translated = JSON.parse(readFileSync(join(messagesRoot, locale, filename), 'utf8'));
        compareCatalogValue(source, translated, `${locale}.${filename}`);
      }
    }
  });

  it('keeps functional account-deletion literals untranslated', () => {
    for (const locale of settings.locales) {
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

  it('keeps syntax-like examples valid', () => {
    for (const locale of settings.locales.filter((locale) => !sparseLocales.has(locale))) {
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
