import { beforeEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { userPreferences } from '$lib/state/userPreferences.svelte';
import ComposerPage from './+page.svelte';

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

describe('App Preferences composer page', () => {
  beforeEach(() => {
    localStorage.clear();
    userPreferences.composerEditor = 'visual';
    userPreferences.composerSendMode = 'modifier-enter';
  });

  it('persists the composer choices immediately for this app', async () => {
    const { container, getByRole } = render(ComposerPage);
    await settle();

    expect(container.querySelectorAll('.panel-shell')).toHaveLength(2);
    const editorChoices = Array.from(
      container.querySelectorAll<HTMLButtonElement>('[aria-label="Message editor"] [role="radio"]')
    );
    expect(editorChoices.map((choice) => choice.textContent?.trim())).toEqual([
      expect.stringContaining('Markdown'),
      expect.stringContaining('Visual')
    ]);

    await getByRole('radio', { name: /^Markdown/ }).click();
    await getByRole('radio', { name: /^Return/ }).click();
    await settle();

    expect(userPreferences.composerEditor).toBe('markdown');
    expect(userPreferences.composerSendMode).toBe('enter');
    expect(JSON.parse(localStorage.getItem('chatto:preferences') ?? '{}')).toMatchObject({
      composerEditor: 'markdown',
      composerSendMode: 'enter'
    });
  });
});
