import {
  createTranslatto,
  type HtmlTranslationKey,
  type LocalizedHtml,
  type TranslationArguments
} from '@chatto/translatto';

import {
  catalogLoaders,
  catalogSections,
  chatCatalogSections,
  initialBaseCatalogs,
  publicCatalogSections,
  type CatalogSection
} from './catalogs';
import { isManagementCatalogSection } from './catalogSections.js';
import { baseLocale, fallbackLocales, type Locale } from './locales';
import {
  bumpI18nRevision,
  getI18nRevision,
  getReactiveLocale,
  setReactiveLocale
} from './state.svelte';

const translatto = createTranslatto({
  baseLocale,
  fallbackLocales,
  initialBaseCatalogs,
  loaders: catalogLoaders
});
let catalogTransition = Promise.resolve();

function scheduleCatalogTransition(operation: () => Promise<void>): Promise<void> {
  const scheduled = catalogTransition.then(operation);
  catalogTransition = scheduled.catch(() => undefined);
  return scheduled;
}

/** Translate a plain-text message from the currently committed locale. */
export function m<const Key extends string>(
  key: Key extends HtmlTranslationKey ? never : Key,
  ...args: TranslationArguments<Key>
): string {
  getI18nRevision();
  return translatto.t(key, ...args);
}

/** Resolve untrusted localized markup for the reviewed sanitizing renderer. */
export function mHtml<const Key extends HtmlTranslationKey>(
  key: Key,
  ...args: TranslationArguments<Key>
): LocalizedHtml {
  getI18nRevision();
  return translatto.html(key, ...args);
}

/** Load every catalog section; retained for focused tests and non-route consumers. */
export async function loadLocaleMessages(locale: Locale): Promise<void> {
  await scheduleCatalogTransition(async () => {
    await translatto.setActiveSections(catalogSections);
    await translatto.setLocale(locale);
    bumpI18nRevision();
  });
}

/** Switch locales while preserving the route's independently loaded sections. */
export async function switchLocaleMessages(locale: Locale): Promise<void> {
  await scheduleCatalogTransition(async () => {
    await translatto.setLocale(locale);
    bumpI18nRevision();
  });
}

/** Initialise the public shell catalogs for the locale selected before first paint. */
export async function preloadPublicLocaleMessages(): Promise<void> {
  const locale = getReactiveLocale();
  await scheduleCatalogTransition(async () => {
    await translatto.setActiveSections(publicCatalogSections);
    await translatto.setLocale(locale);
    setReactiveLocale(locale);
    bumpI18nRevision();
  });
}

/** Load the route's sections, with management catalogs only on management routes. */
export async function preloadChatLocaleMessages(management = false): Promise<void> {
  await scheduleCatalogTransition(async () => {
    const sections = management ? catalogSections : chatCatalogSections;
    const active = translatto.snapshot.activeSections;
    if (
      active.length === sections.length &&
      sections.every((section) => active.includes(section))
    ) {
      return;
    }
    await translatto.setActiveSections(sections);
    bumpI18nRevision();
  });
}

/**
 * Stop loading management sections on later language changes. Called only after
 * navigation leaves management. This removes active sections without downloads
 * or a revision bump: previously published messages remain readable.
 */
export async function deactivateManagementLocaleMessages(): Promise<void> {
  await scheduleCatalogTransition(async () => {
    const active = translatto.snapshot.activeSections;
    const sections = active.filter(
      (section) => !isManagementCatalogSection(section as CatalogSection)
    );
    if (sections.length !== active.length) await translatto.setActiveSections(sections);
  });
}
