import { afterEach, describe, expect, it } from 'vitest';
import * as m from './messages';
import { loadLocaleMessages } from './messages';
import { setReactiveLocale } from './state.svelte';

async function selectLocale(locale: 'en-GB' | 'en-US'): Promise<void> {
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
