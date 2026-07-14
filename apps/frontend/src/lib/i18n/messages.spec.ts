import { afterEach, describe, expect, it } from 'vitest';
import * as m from './messages';
import { loadLocaleMessages } from './messages';
import type { Locale } from './runtime';
import { setReactiveLocale } from './state.svelte';

async function selectLocale(locale: Locale): Promise<void> {
  await loadLocaleMessages(locale);
  setReactiveLocale(locale);
}

afterEach(async () => {
  await selectLocale('en-GB');
});

describe('regional English messages', () => {
  it('uses British English in the base locale', async () => {
    await selectLocale('en-GB');

    expect(m['voice.screen_share_blocked']()).toBe('Screen sharing was cancelled or blocked.');
    expect(m['admin.rooms_admin.subtitle']()).toContain('organise');
    expect(m['settings.profile.status.template.vacation']()).toBe('Holiday');
  });

  it('uses US overrides and falls back for shared messages', async () => {
    await selectLocale('en-US');

    expect(m['voice.screen_share_blocked']()).toBe('Screen sharing was canceled or blocked.');
    expect(m['admin.rooms_admin.subtitle']()).toContain('organize');
    expect(m['settings.profile.status.template.vacation']()).toBe('Vacation');
    expect(m['common.cancel']()).toBe('Cancel');
  });
});

describe('regional translated messages', () => {
  it('keeps Dutch and Flemish sign-in terminology distinct', async () => {
    await selectLocale('nl-NL');
    expect(m['common.sign_in']()).toBe('Inloggen');

    await selectLocale('nl-BE');
    expect(m['common.sign_in']()).toBe('Aanmelden');
  });

  it('uses Swiss German orthography', async () => {
    await selectLocale('de-DE');
    expect(m['common.close_sidebar']()).toBe('Seitenleiste schließen');

    await selectLocale('de-CH');
    expect(m['common.close_sidebar']()).toBe('Seitenleiste schliessen');
  });

  it('keeps German and Austrian terminology distinct', async () => {
    await selectLocale('de-DE');
    expect(m['settings.profile.status.template.out_for_lunch']()).toBe('Mittagspause');

    await selectLocale('de-AT');
    expect(m['settings.profile.status.template.out_for_lunch']()).toBe('Auf Jause');
  });

  it('keeps European and Latin American Spanish terminology distinct', async () => {
    await selectLocale('es-ES');
    expect(m['common.password_confirm_placeholder']()).toBe(
      'Vuelve a introducir la contraseña'
    );

    await selectLocale('es-419');
    expect(m['common.password_confirm_placeholder']()).toBe('Ingresar contraseña nuevamente');
  });

  it('keeps Brazilian and European Portuguese terminology distinct', async () => {
    await selectLocale('pt-BR');
    expect(m['add_server.sign_in']()).toBe('Faça login');

    await selectLocale('pt-PT');
    expect(m['add_server.sign_in']()).toBe('Iniciar sessão');
  });

  it.each([
    ['pl-PL', 'dołączył do pokoju', 'dołączyli do pokoju', 'dołączyli do pokoju'],
    ['uk-UA', 'приєднався до кімнати', 'приєдналися до кімнати', 'приєдналися до кімнати']
  ] as const)('uses every plural category needed by %s', async (locale, one, few, many) => {
    await selectLocale(locale);

    expect(m['room.system_events.joined']({ count: 1 })).toBe(one);
    expect(m['room.system_events.joined']({ count: 2 })).toBe(few);
    expect(m['room.system_events.joined']({ count: 5 })).toBe(many);
  });
});
