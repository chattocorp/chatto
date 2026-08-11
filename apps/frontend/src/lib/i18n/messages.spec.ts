import { afterEach, describe, expect, it } from 'vitest';
import { m } from './messages';
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

    expect(m('voice.screen_share_blocked')).toBe('Screen sharing was cancelled or blocked.');
    expect(m('admin.rooms_admin.subtitle')).toContain('organise');
    expect(m('settings.profile.status.template.vacation')).toBe('Holiday');
  });

  it('uses US overrides and falls back for shared messages', async () => {
    await selectLocale('en-US');

    expect(m('voice.screen_share_blocked')).toBe('Screen sharing was canceled or blocked.');
    expect(m('admin.rooms_admin.subtitle')).toContain('organize');
    expect(m('settings.profile.status.template.vacation')).toBe('Vacation');
    expect(m('common.cancel')).toBe('Cancel');
  });
});

describe('regional translated messages', () => {
  it('keeps Dutch and Flemish sign-in terminology distinct', async () => {
    await selectLocale('nl-NL');
    expect(m('common.sign_in')).toBe('Inloggen');

    await selectLocale('nl-BE');
    expect(m('common.sign_in')).toBe('Aanmelden');
  });

  it('uses Swiss German orthography', async () => {
    await selectLocale('de-DE');
    expect(m('common.close_sidebar')).toBe('Seitenleiste schließen');

    await selectLocale('de-CH');
    expect(m('common.close_sidebar')).toBe('Seitenleiste schliessen');
    expect(m('common.cancel')).toBe('Abbrechen');
    expect(m('ui.toggle_sidebar')).toBe('Seitenleiste umschalten');
  });

  it('keeps German and Austrian terminology distinct', async () => {
    await selectLocale('de-DE');
    expect(m('settings.profile.status.template.out_for_lunch')).toBe('Mittagspause');

    await selectLocale('de-AT');
    expect(m('settings.profile.status.template.out_for_lunch')).toBe('Auf Jause');
    expect(m('common.cancel')).toBe('Abbrechen');
    expect(m('admin.system.started')).toBe('Gestartet');
  });

  it('keeps European and Latin American Spanish terminology distinct', async () => {
    await selectLocale('es-ES');
    expect(m('common.password_confirm_placeholder')).toBe('Vuelve a introducir la contraseña');

    await selectLocale('es-419');
    expect(m('common.password_confirm_placeholder')).toBe('Ingresar contraseña nuevamente');
  });

  it('keeps Brazilian and European Portuguese terminology distinct', async () => {
    await selectLocale('pt-BR');
    expect(m('add_server.sign_in')).toBe('Faça login');

    await selectLocale('pt-PT');
    expect(m('add_server.sign_in')).toBe('Iniciar sessão');
  });

  it.each([
    ['pl-PL', 'dołączył do pokoju', 'dołączyli do pokoju', 'dołączyli do pokoju'],
    ['uk-UA', 'приєднався до кімнати', 'приєдналися до кімнати', 'приєдналися до кімнати']
  ] as const)('uses every plural category needed by %s', async (locale, one, few, many) => {
    await selectLocale(locale);

    expect(m('room.system_events.joined_count', { count: 1 })).toBe(one);
    expect(m('room.system_events.joined_count', { count: 2 })).toBe(few);
    expect(m('room.system_events.joined_count', { count: 5 })).toBe(many);
  });

  it('uses every Arabic plural category', async () => {
    await selectLocale('ar');

    expect(m('room.join.member_count', { count: 0 })).toBe('0 أعضاء');
    expect(m('room.join.member_count', { count: 1 })).toBe('1 عضو');
    expect(m('room.join.member_count', { count: 2 })).toBe('2 عضوان');
    expect(m('room.join.member_count', { count: 3 })).toBe('3 أعضاء');
    expect(m('room.join.member_count', { count: 11 })).toBe('11 عضوًا');
    expect(m('room.join.member_count', { count: 100 })).toBe('100 عضو');
  });

  it('uses every Hebrew plural category', async () => {
    await selectLocale('he-IL');

    expect(m('room.join.member_count', { count: 1 })).toBe('1 חבר');
    expect(m('room.join.member_count', { count: 2 })).toBe('2 חברים');
    expect(m('room.join.member_count', { count: 5 })).toBe('5 חברים');
  });

  it('keeps critical Arabic messages attached to the right keys', async () => {
    await selectLocale('ar');

    expect(m('auth.login.password_reset_success')).toBe(
      'تمت إعادة تعيين كلمة المرور بنجاح. يرجى تسجيل الدخول باستخدام كلمة المرور الجديدة.'
    );
    expect(m('auth.login.error.provider_denied')).toBe('أُلغي تسجيل الدخول لدى موفر الخدمة.');
    expect(m('admin.system.started')).toBe('بدأ');
    expect(m('admin.system.stopped')).toBe('توقف');
    expect(m('admin.system.failed')).toBe('فشل');
    expect(m('voice.microphone_denied')).toBe(
      'تم رفض الوصول إلى الميكروفون. تحقق من أذونات المتصفح وحاول مرة أخرى.'
    );
    expect(m('settings.account.delete_modal.warning_label')).toBe('تحذير:');
  });

  it('keeps critical Hebrew messages attached to the right keys', async () => {
    await selectLocale('he-IL');

    expect(m('chat.sign_out.description')).toBe(
      'צא רק מהשרת שנבחר, או נתק את כל השרתים מהלקוח הזה.'
    );
    expect(m('error_page.missing_media_description')).toBe(
      'לא ניתן לפתוח את הקובץ מהקישור הזה. ייתכן שהוא הוסר, או שקישור המדיה הפרטי אינו זמין עוד.'
    );
    expect(m('auth.callback.authorization_failed', { error: 'x' })).toBe('ההרשאה נכשלה: x');
    expect(m('voice.microphone_denied')).toBe(
      'הגישה למיקרופון נדחתה. בדוק את הרשאות הדפדפן ונסה שוב.'
    );
  });
});
