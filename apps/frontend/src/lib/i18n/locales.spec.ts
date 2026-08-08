import { describe, expect, it } from 'vitest';
import { localeDisplayName, negotiateLocale, selectableLocales } from './locales';

describe('selectable locales', () => {
  it('lists every supported regional content locale explicitly', () => {
    expect(selectableLocales).toEqual([
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
    ]);
  });

  it('localises regional language names with Intl', () => {
    expect(localeDisplayName('fr-CA', 'en-GB')).toBe('Canadian French');
    expect(localeDisplayName('de-AT', 'de-DE')).toBe('Österreichisches Deutsch');
  });

  it('negotiates exact regions, language defaults, and Chinese scripts', () => {
    expect(negotiateLocale(['fr-CA', 'en-US'])).toBe('fr-CA');
    expect(negotiateLocale(['de'])).toBe('de-DE');
    expect(negotiateLocale(['zh-Hant-HK'])).toBe('zh-TW');
    expect(negotiateLocale(['zh-Hans'])).toBe('zh-CN');
    expect(negotiateLocale(['ar-EG'])).toBe('ar');
    expect(negotiateLocale(['he'])).toBe('he-IL');
    expect(negotiateLocale(['he-IL'])).toBe('he-IL');
    expect(negotiateLocale(['xx', 'en-US'])).toBe('en-US');
    expect(negotiateLocale(['xx'])).toBe('en-GB');
  });
});
