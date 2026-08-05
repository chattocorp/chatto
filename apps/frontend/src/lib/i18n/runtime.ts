import { switchLocaleMessages } from '$lib/i18n/messages';

import { localeStorageKey, type Locale } from './locales';
import { getReactiveLocale, setReactiveLocale } from './state.svelte';

export { type Locale };

export function getLocale(): Locale {
  return getReactiveLocale();
}

export function getBrowserLocale(): string {
  return (
    globalThis.navigator?.languages?.[0] ??
    globalThis.navigator?.language ??
    new Intl.DateTimeFormat().resolvedOptions().locale
  );
}

/**
 * Combine Chatto's selected language with the browser's region for Intl formatting.
 *
 * Region-bearing content locales (for example, `en-GB`) are preserved. Language-only
 * locales inherit the browser's region when possible.
 */
export function getFormattingLocale(locale: string = getLocale()): string {
  if (typeof Intl.Locale !== 'function') return locale;

  try {
    const languageLocale = new Intl.Locale(locale);
    if (languageLocale.region) return languageLocale.toString();

    const browserLocale = getBrowserLocale();
    const browserRegion = new Intl.Locale(browserLocale).maximize().region;
    return browserRegion
      ? new Intl.Locale(languageLocale.baseName, { region: browserRegion }).toString()
      : languageLocale.toString();
  } catch {
    return locale;
  }
}

function getTextDirection(locale: Locale): 'ltr' | 'rtl' {
  const language = new Intl.Locale(locale).language;
  return ['ar', 'fa', 'he', 'ur'].includes(language) ? 'rtl' : 'ltr';
}

function applyDocumentLocale(locale: Locale): void {
  if (typeof document === 'undefined') return;
  document.documentElement.lang = locale;
  document.documentElement.dir = getTextDirection(locale);
}

export async function setLocale(locale: Locale): Promise<void> {
  await switchLocaleMessages(locale);
  try {
    localStorage.setItem(localeStorageKey, locale);
    localStorage.removeItem('PARAGLIDE_LOCALE');
  } catch {
    // Keep the in-memory preference when persistent storage is unavailable.
  }
  setReactiveLocale(locale);
  applyDocumentLocale(locale);
}
