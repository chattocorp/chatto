import { beforeEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { userPreferences } from '$lib/state/userPreferences.svelte';
import ClientPreferencesPage from './+page.svelte';

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

describe('Client Preferences page', () => {
  beforeEach(() => {
    localStorage.clear();
    userPreferences.displayTheme = 'system';
    userPreferences.composerEditor = 'visual';
    userPreferences.composerSendMode = 'modifier-enter';
  });

  it('identifies its cross-server client scope', async () => {
    const { container } = render(ClientPreferencesPage);
    await settle();

    expect(container.textContent).toContain('Client Preferences');
    expect(container.textContent).toContain(
      'Choices for this client that apply across all your registered servers'
    );
    expect(container.textContent).not.toContain('Timezone');
  });

  it('persists the editor choice immediately for this client', async () => {
    const { getByRole } = render(ClientPreferencesPage);
    await settle();

    await getByRole('radio', { name: /^Markdown/ }).click();
    await settle();

    expect(userPreferences.composerEditor).toBe('markdown');
    expect(JSON.parse(localStorage.getItem('chatto:preferences') ?? '{}')).toMatchObject({
      composerEditor: 'markdown'
    });
  });

  it('persists the send-key choice immediately for this client', async () => {
    const { getByRole } = render(ClientPreferencesPage);
    await settle();

    await getByRole('radio', { name: /^Return/ }).click();
    await settle();

    expect(userPreferences.composerSendMode).toBe('enter');
    expect(JSON.parse(localStorage.getItem('chatto:preferences') ?? '{}')).toMatchObject({
      composerSendMode: 'enter'
    });
  });
});
