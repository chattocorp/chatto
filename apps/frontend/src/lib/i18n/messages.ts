import {
  createLingua,
  type HtmlTranslationKey,
  type LocalizedHtml,
  type TranslationArguments
} from '@chatto/lingua';

import {
  catalogLoaders,
  catalogSections,
  catalogSectionsForRoute,
  initialBaseCatalogs
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

/** Load only the selected locale sections needed by a route. */
export async function preloadRouteMessages(routeId: string | null): Promise<void> {
  await scheduleCatalogTransition(async () => {
    await lingua.setActiveSections(catalogSectionsForRoute(routeId));
    bumpI18nRevision();
  });
}

/** Initialise route catalogs for the locale selected before first paint. */
export async function preloadActiveLocaleMessages(routeId: string | null): Promise<void> {
  const locale = getReactiveLocale();
  await scheduleCatalogTransition(async () => {
    await lingua.setActiveSections(catalogSectionsForRoute(routeId));
    await lingua.setLocale(locale);
    setReactiveLocale(locale);
    bumpI18nRevision();
  });
}
