import { baseLocale, isLocale, localeStorageKey, negotiateLocale, type Locale } from './locales';

function initialLocale(): Locale {
  const documentLocale = globalThis.document?.documentElement.lang;
  if (isLocale(documentLocale)) return documentLocale;

  try {
    const storedLocale = globalThis.localStorage?.getItem(localeStorageKey);
    if (isLocale(storedLocale)) return storedLocale;
  } catch {
    // Storage can be unavailable in privacy-restricted browser contexts.
  }
  if (typeof globalThis.document === 'undefined') return baseLocale;
  const browserLocales =
    globalThis.navigator?.languages ??
    (globalThis.navigator?.language ? [globalThis.navigator.language] : []);
  return browserLocales.length > 0 ? negotiateLocale(browserLocales) : baseLocale;
}

const i18nState = $state({
  locale: initialLocale(),
  revision: 0
});

export function getReactiveLocale(): Locale {
  return i18nState.locale;
}

export function setReactiveLocale(locale: Locale): void {
  i18nState.locale = locale;
}

export function getI18nRevision(): number {
  return i18nState.revision;
}

export function bumpI18nRevision(): void {
  i18nState.revision += 1;
}
