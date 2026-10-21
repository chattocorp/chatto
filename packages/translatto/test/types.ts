import { createTranslatto, type LocalizedHtml } from '../src/index.js';

const translatto = createTranslatto({
  baseLocale: 'en-GB',
  loaders: {
    common: {
      'en-GB': async () => ({ common: { close: 'Close' } }),
      'de-DE': async () => ({ common: { close: 'Schließen' } })
    }
  }
});

translatto.t('common.close');
translatto.t('common.welcome', { name: 'Ada' });
translatto.t('common.member_count', { count: 3 });
translatto.t('common.member.count', { count: 3 });

// @ts-expect-error Count keys require parameters.
translatto.t('common.member_count');
// @ts-expect-error Count must be numeric.
translatto.t('common.member_count', { count: 'three' });
// @ts-expect-error HTML keys must use html().
translatto.t('common.legal_html');
// @ts-expect-error html() only accepts HTML keys.
translatto.html('common.close');

const html: LocalizedHtml = translatto.html('common.legal_html');
const pluralHtml: LocalizedHtml = translatto.html('common.member_count_html', {
  count: 3
});
void html;
void pluralHtml;

translatto.setLocale('de-DE');
translatto.setActiveSections(['common']);

// @ts-expect-error Locale names come from the loader registry.
translatto.setLocale('fr');
// @ts-expect-error Section names come from the loader registry.
translatto.setActiveSections(['room']);
