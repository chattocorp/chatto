import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { setLocale } from '$lib/i18n/runtime';
import LanguagePage from './+page.svelte';

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

describe('App Preferences language page', () => {
  beforeEach(async () => {
    localStorage.clear();
    await setLocale('en-GB');
  });

  afterEach(async () => {
    await setLocale('en-GB');
  });

  it('changes and persists the app language', async () => {
    const { container, getByRole } = render(LanguagePage);
    await settle();

    expect(container.querySelectorAll('.panel-shell')).toHaveLength(1);
    await getByRole('radio', { name: 'German (Germany)' }).click();
    await vi.waitFor(() => {
      expect(document.documentElement.lang).toBe('de-DE');
      expect(localStorage.getItem('chatto:locale')).toBe('de-DE');
    });
  });
});
