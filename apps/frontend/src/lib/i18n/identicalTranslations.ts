/**
 * Reviewed exceptions for the untranslated-message check in `catalogs.spec.ts`.
 *
 * The check fails when a complete locale uses the British English source text
 * for a message with two or more words. List a message key here only when the
 * identical text is correct in that locale, for example a loanword, a product
 * term, or a name. Add each locale separately. Remove entries that no longer
 * match the source text; the check fails on stale entries too.
 */
export const identicalTranslations: Record<string, readonly string[]> = {
  'add_server.url_label': ['nb-NO', 'nl-BE', 'nl-NL', 'sv-SE'],
  'admin.common.server_admin_page_title': ['de-DE', 'nb-NO', 'sv-SE'],
  'admin.event_log.event_page_title': ['de-DE'],
  'admin.system.broker': ['zh-CN', 'zh-TW'],
  'composer.link_url': ['nl-BE', 'nl-NL', 'pt-BR', 'pt-PT'],
  'preview.youtube_title': ['eo', 'nb-NO', 'sv-SE'],
  'room.thread.title': ['de-DE'],
  'settings.account.sso.title': ['de-DE'],
  'settings.notifications.sound.category.here_be_dragons': ['he-IL', 'nb-NO', 'sv-SE', 'uk-UA'],
  'settings.notifications.sound.name.beepboop': ['cs-CZ', 'de-DE'],
  'settings.notifications.sound.name.celesta': ['eo', 'he-IL', 'nb-NO', 'sv-SE'],
  'settings.notifications.sound.name.chime_up': ['lv-LV'],
  'settings.notifications.sound.name.dubstep': [
    'cs-CZ',
    'eo',
    'et-EE',
    'he-IL',
    'lv-LV',
    'nb-NO',
    'sv-SE',
    'uk-UA'
  ],
  'settings.notifications.sound.name.fanfare': ['he-IL', 'tr-TR'],
  'settings.notifications.sound.name.harp': ['uk-UA'],
  'settings.notifications.sound.name.la_cucaracha': [
    'cs-CZ',
    'de-DE',
    'eo',
    'es-419',
    'es-ES',
    'et-EE',
    'fr-CA',
    'fr-FR',
    'he-IL',
    'it-IT',
    'lv-LV',
    'nb-NO',
    'nl-BE',
    'nl-NL',
    'pl-PL',
    'pt-BR',
    'pt-PT',
    'sv-SE',
    'tr-TR'
  ],
  'settings.notifications.sound.name.laser': [
    'cs-CZ',
    'eo',
    'et-EE',
    'it-IT',
    'lv-LV',
    'nb-NO',
    'pt-BR',
    'pt-PT',
    'sv-SE',
    'uk-UA'
  ],
  'settings.notifications.sound.name.music_box': ['cs-CZ'],
  'settings.notifications.sound.name.pop': ['cs-CZ', 'eo'],
  'settings.notifications.sound.name.synth': ['eo']
};
