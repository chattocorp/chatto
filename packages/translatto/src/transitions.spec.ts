import { describe, expect, it, vi } from 'vitest';
import { createTranslatto } from './index.js';

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
  it('handles a synchronous loader failure alongside an asynchronous failure', async () => {
    const slow = deferred<{ common: { title: string } }>();
    const translatto = createTranslatto({
      baseLocale: 'en',
      loaders: {
        common: { en: () => slow.promise },
        admin: {
          en: () => {
            throw new Error('offline');
          }
        }
      }
    });
    await expect(translatto.setActiveSections(['common', 'admin'])).rejects.toThrow('offline');
    const retry = translatto.setActiveSections(['common']);
    slow.reject(new Error('connection closed'));
    await expect(retry).rejects.toThrow('connection closed');
    expect(translatto.snapshot.activeSections).toEqual([]);
  });

  it('starts the selected, parent, and English downloads together while validation waits for English', async () => {
    const english = deferred<{ admin: { title: string } }>();
    const loadEnglish = vi.fn(() => english.promise);
    const loadGerman = vi.fn(async () => ({ admin: { title: 'Verwaltung' } }));
    const loadAustrian = vi.fn(async () => ({ admin: {} }));
    const translatto = createTranslatto({
      baseLocale: 'en',
      initialLocale: 'de-AT',
      fallbackLocales: { 'de-AT': 'de' },
      loaders: { admin: { en: loadEnglish, de: loadGerman, 'de-AT': loadAustrian } }
    });
    const pending = translatto.setActiveSections(['admin']);
    expect(loadEnglish).toHaveBeenCalledOnce();
    expect(loadGerman).toHaveBeenCalledOnce();
    expect(loadAustrian).toHaveBeenCalledOnce();
    expect(translatto.t('admin.title')).toBe('⟦admin.title⟧');
    english.resolve({ admin: { title: 'Admin' } });
    await pending;
    expect(translatto.t('admin.title')).toBe('Verwaltung');
  });

  it('keeps preloaded and partially loaded sections invisible until a complete commit', async () => {
    const slow = deferred<{ admin: { title: string } }>();
    const translatto = createTranslatto({
      baseLocale: 'en',
      loaders: {
        common: { en: async () => ({ common: { title: 'Common' } }) },
        admin: { en: () => slow.promise }
      }
    });
    const listener = vi.fn();
    translatto.subscribe(listener);
    await translatto.preload('en', ['common']);
    expect(translatto.t('common.title')).toBe('⟦common.title⟧');
    const pending = translatto.setActiveSections(['common', 'admin']);
    expect(translatto.t('common.title')).toBe('⟦common.title⟧');
    expect(listener).toHaveBeenCalledOnce();
    slow.resolve({ admin: { title: 'Admin' } });
    await pending;
    expect(translatto.t('common.title')).toBe('Common');
    expect(translatto.t('admin.title')).toBe('Admin');
    expect(listener).toHaveBeenCalledTimes(2);
  });

  it('preserves eager fallback after invalid overlays and reloads the failed catalog on retry', async () => {
    const german = vi
      .fn()
      .mockResolvedValueOnce({ common: { title: 'Hallo {wrong}' } })
      .mockResolvedValue({ common: { title: 'Hallo {name}' } });
    const translatto = createTranslatto({
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
    const before = translatto.snapshot;
    await expect(translatto.setActiveSections(['common'])).rejects.toThrow('preserve');
    expect(translatto.snapshot).toEqual(before);
    expect(translatto.t('common.title', { name: 'Reader' })).toBe('Hello Reader');
    await translatto.setActiveSections(['common']);
    expect(translatto.t('common.title', { name: 'Reader' })).toBe('Hallo Reader');
    expect(german).toHaveBeenCalledTimes(2);
  });

  it('does not publish a successful sibling when another section fails', async () => {
    const translatto = createTranslatto({
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
    await expect(translatto.setActiveSections(['common', 'admin'])).rejects.toThrow('offline');
    expect(translatto.t('common.title')).toBe('⟦common.title⟧');
    expect(translatto.snapshot.activeSections).toEqual([]);
    await translatto.setActiveSections(['common']);
    expect(translatto.t('common.title')).toBe('Common');
  });

  it('retains published sections without loading inactive sections for new locales', async () => {
    const germanAdmin = vi.fn(async () => ({ admin: { title: 'Verwaltung' } }));
    const englishAdmin = vi.fn(async () => ({ admin: { title: 'Admin' } }));
    const translatto = createTranslatto({
      baseLocale: 'en',
      loaders: {
        common: {
          en: async () => ({ common: { title: 'Common' } }),
          de: async () => ({ common: { title: 'Allgemein' } })
        },
        admin: { en: englishAdmin, de: germanAdmin }
      }
    });
    await translatto.setActiveSections(['common', 'admin']);
    await translatto.setActiveSections(['common']);
    expect(translatto.t('admin.title')).toBe('Admin');
    await translatto.setLocale('de');
    expect(germanAdmin).not.toHaveBeenCalled();
    await translatto.setActiveSections(['common', 'admin']);
    expect(translatto.t('admin.title')).toBe('Verwaltung');
    expect(englishAdmin).toHaveBeenCalledOnce();
    expect(germanAdmin).toHaveBeenCalledOnce();
  });
});

describe('overlapping setters', () => {
  it.each(['locale first', 'sections first'])('composes independent updates: %s', async (order) => {
    const slow = deferred<{ admin: { title: string } }>();
    const translatto = createTranslatto({
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
    await translatto.setActiveSections(['common']);
    const locale = () => translatto.setLocale('de');
    const sections = () => translatto.setActiveSections(['common', 'admin']);
    const pending = order === 'locale first' ? [locale(), sections()] : [sections(), locale()];
    slow.resolve({ admin: { title: 'Admin' } });
    await Promise.all(pending);
    expect(translatto.snapshot.locale).toBe('de');
    expect(translatto.snapshot.activeSections).toEqual(['common', 'admin']);
    expect(translatto.t('admin.title')).toBe('Verwaltung');
  });

  it('does not cancel a pending valid transition when an unknown section is requested', async () => {
    const slow = deferred<{ common: { title: string } }>();
    const translatto = createTranslatto({
      baseLocale: 'en',
      loaders: { common: { en: () => slow.promise } }
    });
    const pending = translatto.setActiveSections(['common']);
    await expect(translatto.setActiveSections(['unknown'] as never)).rejects.toThrow('Unknown');
    slow.resolve({ common: { title: 'Common' } });
    await pending;
    expect(translatto.t('common.title')).toBe('Common');
  });

  it('does not clear a newer target when a superseded locale load fails', async () => {
    const german = deferred<{ common: { title: string } }>();
    const french = deferred<{ common: { title: string } }>();
    const translatto = createTranslatto({
      baseLocale: 'en',
      loaders: {
        common: {
          en: async () => ({ common: { title: 'Common' } }),
          de: () => german.promise,
          fr: () => french.promise
        }
      }
    });
    await translatto.setActiveSections(['common']);
    const old = translatto.setLocale('de');
    const newer = translatto.setLocale('fr');
    german.reject(new Error('offline'));
    await expect(old).rejects.toThrow('offline');
    const latest = translatto.setActiveSections(['common']);
    french.resolve({ common: { title: 'Commun' } });
    await Promise.all([newer, latest]);
    expect(translatto.snapshot.locale).toBe('fr');
  });
});

it('loads and escapes nested count.html plurals in base and translated catalogs', async () => {
  const translatto = createTranslatto({
    baseLocale: 'en',
    initialLocale: 'de',
    loaders: {
      common: {
        en: async () => ({ common: { count: { html: { other: '{count} items for {name}' } } } }),
        de: async () => ({ common: { count: { html: { other: '{count} Dinge für {name}' } } } })
      }
    }
  });
  await translatto.setActiveSections(['common']);
  expect(translatto.html('common.count.html', { count: 2, name: '<Reader>' })).toBe(
    '2 Dinge für &lt;Reader&gt;'
  );
});
