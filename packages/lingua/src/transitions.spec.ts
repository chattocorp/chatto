import { describe, expect, it, vi } from 'vitest';
import { createLingua } from './index.js';

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  return { promise, resolve, reject };
}

describe('catalog publication', () => {
  it('starts the selected, parent, and English downloads together while validation waits for English', async () => {
    const english = deferred<{ admin: { title: string } }>();
    const loadEnglish = vi.fn(() => english.promise);
    const loadGerman = vi.fn(async () => ({ admin: { title: 'Verwaltung' } }));
    const loadAustrian = vi.fn(async () => ({ admin: {} }));
    const lingua = createLingua({
      baseLocale: 'en',
      initialLocale: 'de-AT',
      fallbackLocales: { 'de-AT': 'de' },
      loaders: { admin: { en: loadEnglish, de: loadGerman, 'de-AT': loadAustrian } }
    });
    const pending = lingua.setActiveSections(['admin']);
    expect(loadEnglish).toHaveBeenCalledOnce();
    expect(loadGerman).toHaveBeenCalledOnce();
    expect(loadAustrian).toHaveBeenCalledOnce();
    expect(lingua.t('admin.title')).toBe('⟦admin.title⟧');
    english.resolve({ admin: { title: 'Admin' } });
    await pending;
    expect(lingua.t('admin.title')).toBe('Verwaltung');
  });

  it('keeps preloaded and partially loaded sections invisible until a complete commit', async () => {
    const slow = deferred<{ admin: { title: string } }>();
    const lingua = createLingua({
      baseLocale: 'en',
      loaders: {
        common: { en: async () => ({ common: { title: 'Common' } }) },
        admin: { en: () => slow.promise }
      }
    });
    const listener = vi.fn();
    lingua.subscribe(listener);
    await lingua.preload('en', ['common']);
    expect(lingua.t('common.title')).toBe('⟦common.title⟧');
    const pending = lingua.setActiveSections(['common', 'admin']);
    expect(lingua.t('common.title')).toBe('⟦common.title⟧');
    expect(listener).toHaveBeenCalledOnce();
    slow.resolve({ admin: { title: 'Admin' } });
    await pending;
    expect(lingua.t('common.title')).toBe('Common');
    expect(lingua.t('admin.title')).toBe('Admin');
    expect(listener).toHaveBeenCalledTimes(2);
  });

  it('preserves eager fallback after invalid overlays and reloads the failed catalog on retry', async () => {
    const german = vi
      .fn()
      .mockResolvedValueOnce({ common: { title: 'Hallo {wrong}' } })
      .mockResolvedValue({ common: { title: 'Hallo {name}' } });
    const lingua = createLingua({
      baseLocale: 'en',
      initialLocale: 'de',
      initialBaseCatalogs: { common: { common: { title: 'Hello {name}' } } },
      loaders: {
        common: {
          en: async () => ({ common: { title: 'Hello {name}' } }),
          de: german
        }
      }
    });
    const before = lingua.snapshot;
    await expect(lingua.setActiveSections(['common'])).rejects.toThrow('preserve');
    expect(lingua.snapshot).toEqual(before);
    expect(lingua.t('common.title', { name: 'Reader' })).toBe('Hello Reader');
    await lingua.setActiveSections(['common']);
    expect(lingua.t('common.title', { name: 'Reader' })).toBe('Hallo Reader');
    expect(german).toHaveBeenCalledTimes(2);
  });

  it('does not publish a successful sibling when another section fails', async () => {
    const lingua = createLingua({
      baseLocale: 'en',
      loaders: {
        common: { en: async () => ({ common: { title: 'Common' } }) },
        admin: {
          en: async () => {
            throw new Error('offline');
          }
        }
      }
    });
    await expect(lingua.setActiveSections(['common', 'admin'])).rejects.toThrow('offline');
    expect(lingua.t('common.title')).toBe('⟦common.title⟧');
    expect(lingua.snapshot.activeSections).toEqual([]);
    await lingua.setActiveSections(['common']);
    expect(lingua.t('common.title')).toBe('Common');
  });

  it('retains published sections without loading inactive sections for new locales', async () => {
    const germanAdmin = vi.fn(async () => ({ admin: { title: 'Verwaltung' } }));
    const englishAdmin = vi.fn(async () => ({ admin: { title: 'Admin' } }));
    const lingua = createLingua({
      baseLocale: 'en',
      loaders: {
        common: {
          en: async () => ({ common: { title: 'Common' } }),
          de: async () => ({ common: { title: 'Allgemein' } })
        },
        admin: { en: englishAdmin, de: germanAdmin }
      }
    });
    await lingua.setActiveSections(['common', 'admin']);
    await lingua.setActiveSections(['common']);
    expect(lingua.t('admin.title')).toBe('Admin');
    await lingua.setLocale('de');
    expect(germanAdmin).not.toHaveBeenCalled();
    await lingua.setActiveSections(['common', 'admin']);
    expect(lingua.t('admin.title')).toBe('Verwaltung');
    expect(englishAdmin).toHaveBeenCalledOnce();
    expect(germanAdmin).toHaveBeenCalledOnce();
  });
});

describe('overlapping setters', () => {
  it.each(['locale first', 'sections first'])('composes independent updates: %s', async (order) => {
    const slow = deferred<{ admin: { title: string } }>();
    const lingua = createLingua({
      baseLocale: 'en',
      loaders: {
        common: {
          en: async () => ({ common: { title: 'Common' } }),
          de: async () => ({ common: { title: 'Allgemein' } })
        },
        admin: {
          en: () => slow.promise,
          de: async () => ({ admin: { title: 'Verwaltung' } })
        }
      }
    });
    await lingua.setActiveSections(['common']);
    const locale = () => lingua.setLocale('de');
    const sections = () => lingua.setActiveSections(['common', 'admin']);
    const pending = order === 'locale first' ? [locale(), sections()] : [sections(), locale()];
    slow.resolve({ admin: { title: 'Admin' } });
    await Promise.all(pending);
    expect(lingua.snapshot.locale).toBe('de');
    expect(lingua.snapshot.activeSections).toEqual(['common', 'admin']);
    expect(lingua.t('admin.title')).toBe('Verwaltung');
  });

  it('does not cancel a pending valid transition when an unknown section is requested', async () => {
    const slow = deferred<{ common: { title: string } }>();
    const lingua = createLingua({
      baseLocale: 'en',
      loaders: { common: { en: () => slow.promise } }
    });
    const pending = lingua.setActiveSections(['common']);
    await expect(lingua.setActiveSections(['unknown'] as never)).rejects.toThrow('Unknown');
    slow.resolve({ common: { title: 'Common' } });
    await pending;
    expect(lingua.t('common.title')).toBe('Common');
  });

  it('does not clear a newer target when a superseded locale load fails', async () => {
    const german = deferred<{ common: { title: string } }>();
    const french = deferred<{ common: { title: string } }>();
    const lingua = createLingua({
      baseLocale: 'en',
      loaders: {
        common: {
          en: async () => ({ common: { title: 'Common' } }),
          de: () => german.promise,
          fr: () => french.promise
        }
      }
    });
    await lingua.setActiveSections(['common']);
    const old = lingua.setLocale('de');
    const newer = lingua.setLocale('fr');
    german.reject(new Error('offline'));
    await expect(old).rejects.toThrow('offline');
    const latest = lingua.setActiveSections(['common']);
    french.resolve({ common: { title: 'Commun' } });
    await Promise.all([newer, latest]);
    expect(lingua.snapshot.locale).toBe('fr');
  });
});

it('loads and escapes nested count.html plurals in base and translated catalogs', async () => {
  const lingua = createLingua({
    baseLocale: 'en',
    initialLocale: 'de',
    loaders: {
      common: {
        en: async () => ({ common: { count: { html: { other: '{count} items for {name}' } } } }),
        de: async () => ({ common: { count: { html: { other: '{count} Dinge für {name}' } } } })
      }
    }
  });
  await lingua.setActiveSections(['common']);
  expect(lingua.html('common.count.html', { count: 2, name: '<Reader>' })).toBe(
    '2 Dinge für &lt;Reader&gt;'
  );
});
