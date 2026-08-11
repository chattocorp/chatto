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
  'it-IT',
  'lv-LV',
  'et-EE',
  'tr-TR',
  'cs-CZ',
  'ru-RU',
  'ar',
  'he-IL',
  'ja-JP',
  'zh-TW',
  'zh-CN',
  'eo'
] as const;

export type Locale = (typeof selectableLocales)[number];
export type SelectableLocale = Locale;
export const baseLocale = 'en-GB' satisfies Locale;
export const localeStorageKey = 'chatto:locale';

/** Sparse regional locales inherit their language's primary catalog before English. */
export const fallbackLocales = {
  'en-US': 'en-GB',
  'de-AT': 'de-DE',
  'de-CH': 'de-DE'
} as const satisfies Partial<Record<Locale, Locale>>;

export function isLocale(value: unknown): value is Locale {
  return typeof value === 'string' && selectableLocales.includes(value as Locale);
}

const defaultLocaleByLanguage: Readonly<Record<string, Locale>> = {
  en: 'en-GB',
  de: 'de-DE',
  nl: 'nl-NL',
  sv: 'sv-SE',
  fr: 'fr-FR',
  es: 'es-ES',
  pt: 'pt-BR',
  nb: 'nb-NO',
  no: 'nb-NO',
  pl: 'pl-PL',
  uk: 'uk-UA',
  it: 'it-IT',
  lv: 'lv-LV',
  et: 'et-EE',
  tr: 'tr-TR',
  cs: 'cs-CZ',
  ru: 'ru-RU',
  ar: 'ar',
  he: 'he-IL',
  ja: 'ja-JP',
  zh: 'zh-CN',
  eo: 'eo'
};

/** Resolve browser language preferences to a supported content locale. */
export function negotiateLocale(requestedLocales: readonly string[]): Locale {
  for (const requested of requestedLocales) {
    const parts = requested.toLowerCase().split('-');
    const exact = selectableLocales.find(
      (locale) => locale.toLowerCase() === requested.toLowerCase()
    );
    if (exact) return exact;

    const language = parts[0];
    if (language === 'zh') {
      if (parts.includes('hant') || parts.some((part) => ['tw', 'hk', 'mo'].includes(part))) {
        return 'zh-TW';
      }
      return 'zh-CN';
    }
    if (language && defaultLocaleByLanguage[language]) return defaultLocaleByLanguage[language];
  }
  return baseLocale;
}

/** Localise language names with the browser's CLDR data. */
export function localeDisplayName(locale: SelectableLocale, displayLocale: Locale): string {
  return new Intl.DisplayNames([displayLocale], { type: 'language' }).of(locale) ?? locale;
}
