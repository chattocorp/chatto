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
