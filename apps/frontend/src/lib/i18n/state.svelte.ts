import { getLocale as getParaglideLocale, type Locale } from '$lib/paraglide/runtime';

const i18nState = $state({
  locale: getParaglideLocale()
});

export function getReactiveLocale(): Locale {
  return i18nState.locale;
}

export function setReactiveLocale(locale: Locale): void {
  i18nState.locale = locale;
}
