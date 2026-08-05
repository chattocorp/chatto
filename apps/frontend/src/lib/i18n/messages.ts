import {
  createLingua,
  type HtmlTranslationKey,
  type LocalizedHtml,
  type TranslationArguments
} from '@chatto/lingua';

import {
  catalogLoaders,
  catalogSections,
  initialBaseCatalogs,
  publicCatalogSections
} from './catalogs';
import { baseLocale, type Locale } from './locales';
import {
  bumpI18nRevision,
  getI18nRevision,
  getReactiveLocale,
  setReactiveLocale
} from './state.svelte';

const lingua = createLingua({
  baseLocale,
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
  return lingua.t(key, ...args);
}

/** Resolve untrusted localized markup for the reviewed sanitizing renderer. */
export function mHtml<const Key extends HtmlTranslationKey>(
  key: Key,
  ...args: TranslationArguments<Key>
): LocalizedHtml {
  getI18nRevision();
  return lingua.html(key, ...args);
}

/** Load every catalog section; retained for focused tests and non-route consumers. */
export async function loadLocaleMessages(locale: Locale): Promise<void> {
  await scheduleCatalogTransition(async () => {
    await lingua.setActiveSections(catalogSections);
    await lingua.setLocale(locale);
    bumpI18nRevision();
  });
}

/** Switch locales while preserving the route's independently loaded sections. */
export async function switchLocaleMessages(locale: Locale): Promise<void> {
  await scheduleCatalogTransition(async () => {
    await lingua.setLocale(locale);
    bumpI18nRevision();
  });
}

/** Initialise the public shell catalogs for the locale selected before first paint. */
export async function preloadPublicLocaleMessages(): Promise<void> {
  const locale = getReactiveLocale();
  await scheduleCatalogTransition(async () => {
    await lingua.setActiveSections(publicCatalogSections);
    await lingua.setLocale(locale);
    setReactiveLocale(locale);
    bumpI18nRevision();
  });
}

/** Load the complete selected locale before entering the authenticated chat shell. */
export async function preloadChatLocaleMessages(): Promise<void> {
  await scheduleCatalogTransition(async () => {
    await lingua.setActiveSections(catalogSections);
    bumpI18nRevision();
  });
}
