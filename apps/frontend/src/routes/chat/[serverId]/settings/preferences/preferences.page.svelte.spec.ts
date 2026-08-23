import { beforeEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { q } from '$lib/test-utils';
import { userPreferences } from '$lib/state/userPreferences.svelte';

import PreferencesPage from './+page.svelte';

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

function buttonWithText(container: Element, text: string): HTMLButtonElement {
  const button = Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(
    (candidate) => candidate.textContent?.includes(text)
  );
  if (!button) throw new Error(`Button with text "${text}" not found`);
  return button;
}

describe('Preferences settings page', () => {
  beforeEach(() => {
    userPreferences.composerEditor = 'visual';
    userPreferences.composerSendMode = 'modifier-enter';
    userPreferences.setTimePreferences({ timezone: null, timeFormat: 'auto' });
  });

  it('hydrates time controls from client-wide preferences without waiting for a server', async () => {
    userPreferences.setTimePreferences({
      timezone: 'Pacific/Honolulu',
      timeFormat: '24h'
    });
    const { container } = render(PreferencesPage);
    await settle();

    const timezoneInput = q(container, '[data-testid="timezone-input"]') as HTMLInputElement;
    const saveButton = buttonWithText(container, 'Save');
    await expect.element(timezoneInput).toHaveValue('Pacific/Honolulu');
    await expect.element(timezoneInput).toBeEnabled();
    await expect
      .element(buttonWithText(container, '24-hour'))
      .toHaveAttribute('aria-checked', 'true');
    await expect.element(saveButton).toBeDisabled();
  });

  it('saves time format to the client-wide preference slot', async () => {
    const { container, getByRole } = render(PreferencesPage);
    await settle();

    await getByRole('radio', { name: /^24-hour/ }).click();
    await settle();
    await expect.element(buttonWithText(container, 'Save')).toBeEnabled();
    buttonWithText(container, 'Save').click();
    await settle();

    expect(userPreferences.timeFormat).toBe('24h');
    expect(JSON.parse(localStorage.getItem('chatto:preferences') ?? '{}')).toMatchObject({
      timezone: null,
      timeFormat: '24h'
    });
  });

  it('persists the editor choice immediately for this browser', async () => {
    const { container, getByRole } = render(PreferencesPage);
    await settle();

    expect(container.textContent).toContain('Choose how Chatto looks and behaves');
    expect(container.textContent).toContain(
      'This choice applies to every Chatto server in this browser.'
    );
    await getByRole('radio', { name: /^Markdown/ }).click();
    await settle();

    expect(userPreferences.composerEditor).toBe('markdown');
    expect(JSON.parse(localStorage.getItem('chatto:preferences') ?? '{}')).toMatchObject({
      composerEditor: 'markdown'
    });
  });

  it('persists the send-key choice immediately for this browser', async () => {
    const { container, getByRole } = render(PreferencesPage);
    await settle();

    expect(container.textContent).toContain('Send messages with');
    await getByRole('radio', { name: /^Return/ }).click();
    await settle();

    expect(userPreferences.composerSendMode).toBe('enter');
    expect(JSON.parse(localStorage.getItem('chatto:preferences') ?? '{}')).toMatchObject({
      composerSendMode: 'enter'
    });
  });
});
