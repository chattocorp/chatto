import { describe, expect, it } from 'vitest';
import { localeDisplayName, selectableLocales } from './locales';

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
});
