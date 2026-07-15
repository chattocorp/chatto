import type { Locale } from './runtime';

/** Locales presented to users. */
export const selectableLocales = [
  'en-GB',
  'en-US',
  'de-DE',
  'de-AT',
  'de-CH',
  'nl-NL',
  'nl-BE',
  'sv-SE',
  'fr-FR',
  'fr-CA',
  'es-ES',
  'es-419',
  'pt-BR',
  'pt-PT',
  'nb-NO',
  'pl-PL',
  'uk-UA',
  'ja-JP',
  'eo'
] as const satisfies readonly Locale[];

export type SelectableLocale = (typeof selectableLocales)[number];

/** Localise language names with the browser's CLDR data. */
export function localeDisplayName(locale: SelectableLocale, displayLocale: Locale): string {
  return new Intl.DisplayNames([displayLocale], { type: 'language' }).of(locale) ?? locale;
}
