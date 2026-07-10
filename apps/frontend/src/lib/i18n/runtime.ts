import { loadLocaleMessages } from '$lib/i18n/messages';
import {
  getTextDirection,
  setLocale as setParaglideLocale,
  type Locale
} from '$lib/paraglide/runtime';
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
 * Paraglide locales are language-only (for example, `en`), which would otherwise
 * make Intl fall back to US conventions instead of the browser's `en-GB` region.
 * Explicit region-bearing locales are preserved for deterministic callers and tests.
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

function applyDocumentLocale(locale: Locale): void {
  if (typeof document === 'undefined') return;
  document.documentElement.lang = locale;
  document.documentElement.dir = getTextDirection(locale);
}

export async function setLocale(
  locale: Locale,
  options?: Parameters<typeof setParaglideLocale>[1]
): Promise<void> {
  await loadLocaleMessages(locale);
  await setParaglideLocale(locale, { reload: false, ...options });
  setReactiveLocale(locale);
  applyDocumentLocale(locale);
}
